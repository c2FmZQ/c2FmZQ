package client

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"

	"kringle-server/stingle"
)

func (c *Client) AddAlbums(names []string) error {
	for _, n := range names {
		n := strings.TrimSuffix(n, "/")
		if strings.Contains(n, "/") {
			return fmt.Errorf("album name may not contain a slash: %q", n)
		}
		if err := c.addAlbum(n); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) addAlbum(name string) error {
	b := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return err
	}
	albumID := base64.RawURLEncoding.EncodeToString(b)
	ask := stingle.MakeSecretKey()
	encPrivateKey := c.SecretKey.PublicKey().SealBoxBase64(ask.ToBytes())
	metadata := stingle.EncryptAlbumMetadata(stingle.AlbumMetadata{Name: name}, ask.PublicKey())
	publicKey := base64.StdEncoding.EncodeToString(ask.PublicKey().ToBytes())

	album := stingle.Album{
		AlbumID:       albumID,
		DateCreated:   nowJSON(),
		DateModified:  nowJSON(),
		EncPrivateKey: encPrivateKey,
		Metadata:      metadata,
		PublicKey:     publicKey,
		IsShared:      "0",
		IsHidden:      "0",
		IsOwner:       "1",
		IsLocked:      "0",
		LocalOnly:     true,
	}

	var al AlbumList
	commit, err := c.storage.OpenForUpdate(c.fileHash(albumList), &al)
	if err != nil {
		return err
	}
	al.Albums[albumID] = &album
	if err := c.storage.CreateEmptyFile(c.fileHash(albumPrefix+albumID), &FileSet{}); err != nil {
		return err
	}
	if err := commit(true, nil); err != nil {
		return err
	}
	fmt.Fprintf(c.writer, "Created %s\n", name)
	//if err := c.createRemoteAlbum(&album); err != nil {
	//	fmt.Fprintf(c.writer, "Failed to create %s remotely\n", name)
	//	return err
	//}
	return nil
}

func (c *Client) Hide(names []string, hidden bool) (retErr error) {
	li, err := c.GlobFiles(names)
	if err != nil {
		return err
	}
	var al AlbumList
	commit, err := c.storage.OpenForUpdate(c.fileHash(albumList), &al)
	if err != nil {
		return err
	}
	defer commit(true, &retErr)
	for _, item := range li {
		if item.Album == nil {
			continue
		}
		album, ok := al.Albums[item.Album.AlbumID]
		if !ok {
			continue
		}
		if hidden {
			album.IsHidden = "1"
		} else {
			album.IsHidden = "0"
		}
		if !album.LocalOnly {
			if err := c.editPerms(album); err != nil {
				return err
			}
		}
		if hidden {
			fmt.Fprintf(c.writer, "Hid %s\n", item.Filename)
		} else {
			fmt.Fprintf(c.writer, "Unhid %s\n", item.Filename)
		}
	}
	return nil
}

// Move moves files to an existing album, or renames an album.
func (c *Client) Move(patterns []string, dest string) error {
	dest = strings.TrimSuffix(dest, "/")
	di, err := c.glob(dest)
	if err != nil {
		return err
	}
	si, err := c.GlobFiles(patterns)
	if err != nil {
		return err
	}
	if len(si) == 0 {
		return fmt.Errorf("no match for: %s", strings.Join(patterns, " "))
	}
	// Rename an album.
	if len(si) == 1 && si[0].IsDir && len(di) == 0 {
		return c.renameAlbum(si[0], dest)
	}
	// Move files to a different album.
	if len(di) != 1 || !di[0].IsDir {
		return fmt.Errorf("destination must be an album: %s", dest)
	}
	for _, item := range si {
		if item.IsDir {
			return fmt.Errorf("cannot move albums to another album: %s", item.Filename)
		}
	}
	groups := make(map[string][]ListItem)
	for _, item := range si {
		key := item.Set + "/"
		if item.Album != nil {
			key += item.Album.AlbumID
		}
		groups[key] = append(groups[key], item)
	}
	for _, li := range groups {
		if err := c.moveFiles(li, di[0], true); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) renameAlbum(li ListItem, name string) (retErr error) {
	var albumID string
	if li.Album != nil {
		albumID = li.Album.AlbumID
	}
	name = strings.TrimSuffix(name, "/")
	if albumID == "" {
		return fmt.Errorf("cannot rename gallery or trash")
	}
	if name == "" || strings.Contains(name, "/") {
		return fmt.Errorf("illegal name: %q", name)
	}
	pk, err := li.Album.PK()
	if err != nil {
		return err
	}

	fmt.Fprintf(c.writer, "Renaming %s -> %s\n", strings.TrimSuffix(li.Filename, "/"), name)

	var al AlbumList
	commit, err := c.storage.OpenForUpdate(c.fileHash(albumList), &al)
	if err != nil {
		return err
	}
	defer commit(false, &retErr)
	md := stingle.EncryptAlbumMetadata(stingle.AlbumMetadata{Name: name}, pk)
	al.Albums[albumID].Metadata = md
	al.Albums[albumID].DateModified = nowJSON()

	if !li.Album.LocalOnly {
		params := make(map[string]string)
		params["albumId"] = albumID
		params["metadata"] = md

		form := url.Values{}
		form.Set("token", c.Token)
		form.Set("params", c.encodeParams(params))

		sr, err := c.sendRequest("/v2/sync/renameAlbum", form)
		if err != nil {
			return err
		}
		if sr.Status != "ok" {
			return sr
		}
	}
	return commit(true, nil)
}

func (c *Client) moveFiles(fromItems []ListItem, toItem ListItem, moving bool) (retErr error) {
	var (
		fromSet, toSet         string = fromItems[0].Set, toItem.Set
		fromAlbumID, toAlbumID string
		fromAlbum, toAlbum     *stingle.Album = fromItems[0].Album, toItem.Album
	)
	if fromAlbum != nil {
		fromAlbumID = fromAlbum.AlbumID
	}
	if toAlbum != nil {
		toAlbumID = toAlbum.AlbumID
	}

	if fromSet == toSet && fromAlbumID == toAlbumID {
		return fmt.Errorf("source and destination are the same: %s", toItem.Filename)
	}
	if fromAlbum != nil && fromAlbum.LocalOnly {
		if err := c.createRemoteAlbum(fromAlbum); err != nil {
			return err
		}
	}
	if toAlbum != nil && toAlbum.LocalOnly {
		if err := c.createRemoteAlbum(toAlbum); err != nil {
			return err
		}
	}

	sk, pk := c.SecretKey, c.SecretKey.PublicKey()
	needHeaders := fromAlbum != nil || toAlbum != nil
	if needHeaders {
		var err error
		if fromAlbum != nil {
			if sk, err = fromAlbum.SK(sk); err != nil {
				return err
			}
		}
		if toAlbum != nil {
			if pk, err = toAlbum.PK(); err != nil {
				return err
			}
		}
	}
	commit, fs, err := c.fileSetsForUpdate([]string{fromItems[0].FileSet, toItem.FileSet})
	if err != nil {
		return err
	}
	defer commit(false, &retErr)

	params := make(map[string]string)
	params["setFrom"] = fromSet
	params["setTo"] = toSet
	params["albumIdFrom"] = fromAlbumID
	params["albumIdTo"] = toAlbumID
	params["isMoving"] = "0"
	if moving {
		params["isMoving"] = "1"
	}
	count := 0
	for _, item := range fromItems {
		ff := item.FSFile
		if moving {
			fmt.Fprintf(c.writer, "Moving %s -> %s\n", item.Filename, toItem.Filename)
			delete(fs[0].Files, ff.File)
		} else {
			fmt.Fprintf(c.writer, "Copying %s -> %s\n", item.Filename, toItem.Filename)
		}
		if needHeaders {
			// Re-encrypt headers for destination.
			hdrs, err := stingle.DecryptBase64Headers(ff.Headers, sk)
			if err != nil {
				return err
			}
			h, err := stingle.EncryptBase64Headers(hdrs, pk)
			if err != nil {
				return err
			}
			ff.Headers = h
			if !ff.LocalOnly {
				params[fmt.Sprintf("headers%d", count)] = h
			}
		}
		if !ff.LocalOnly {
			params[fmt.Sprintf("filename%d", count)] = ff.File
			count++
		}
		ff.DateModified = nowJSON()
		ff.AlbumID = toAlbumID
		fs[1].Files[ff.File] = &ff
	}
	params["count"] = fmt.Sprintf("%d", count)

	form := url.Values{}
	form.Set("token", c.Token)
	form.Set("params", c.encodeParams(params))

	sr, err := c.sendRequest("/v2/sync/moveFile", form)
	if err != nil {
		return err
	}
	if sr.Status != "ok" {
		return sr
	}
	return commit(true, nil)
}

func (c *Client) editPerms(album *stingle.Album) error {
	ja, err := json.Marshal(album)
	if err != nil {
		return err
	}
	params := make(map[string]string)
	params["album"] = string(ja)

	form := url.Values{}
	form.Set("token", c.Token)
	form.Set("params", c.encodeParams(params))

	sr, err := c.sendRequest("/v2/sync/editPerms", form)
	if err != nil {
		return err
	}
	if sr.Status != "ok" {
		return sr
	}
	return nil
}
