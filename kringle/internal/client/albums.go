package client

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"kringle/internal/stingle"
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
	if _, err := rand.Read(b); err != nil {
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
	}

	var al AlbumList
	commit, err := c.storage.OpenForUpdate(c.fileHash(albumList), &al)
	if err != nil {
		return err
	}
	if al.Albums == nil {
		al.Albums = make(map[string]*stingle.Album)
	}
	if al.RemoteAlbums == nil {
		al.RemoteAlbums = make(map[string]*stingle.Album)
	}
	al.Albums[albumID] = &album
	if err := c.storage.CreateEmptyFile(c.fileHash(albumPrefix+albumID), &FileSet{}); err != nil {
		return err
	}
	if err := commit(true, nil); err != nil {
		return err
	}
	fmt.Fprintf(c.writer, "Created %s\n", name)
	return nil
}

// RemoveAlbums deletes albums.
func (c *Client) RemoveAlbums(patterns []string) error {
	li, err := c.GlobFiles(patterns)
	if err != nil {
		return err
	}
	for _, item := range li {
		if !item.IsDir || item.Album == nil {
			return fmt.Errorf("cannot remove %s", item.Filename)
		}
	}
	for _, item := range li {
		if err := c.removeAlbum(item); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) removeAlbum(item ListItem) (retErr error) {
	c.Printf("Removing %s\n", item.Filename)
	var al AlbumList
	commit, err := c.storage.OpenForUpdate(c.fileHash(albumList), &al)
	if err != nil {
		return err
	}
	defer commit(false, &retErr)
	if item.Album == nil {
		return fmt.Errorf("not an album: %s", item.Filename)
	}
	delete(al.Albums, item.Album.AlbumID)
	var fs FileSet
	if _, err := c.storage.ReadDataFile(c.fileHash(albumPrefix+item.Album.AlbumID), &fs); err != nil {
		return err
	}
	if len(fs.Files) > 0 {
		return fmt.Errorf("album is not empty: %s", item.Filename)
	}
	if _, ok := al.RemoteAlbums[item.Album.AlbumID]; !ok {
		if err := os.Remove(filepath.Join(c.storage.Dir(), c.fileHash(albumPrefix+item.Album.AlbumID))); err != nil {
			return err
		}
	}
	return commit(true, nil)
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
	defer commit(false, &retErr)
	for _, item := range li {
		if !item.IsDir || item.Album == nil {
			continue
		}
		album, ok := al.Albums[item.Album.AlbumID]
		if !ok {
			continue
		}
		if hidden {
			album.IsHidden = "1"
			fmt.Fprintf(c.writer, "Hid %s\n", item.Filename)
		} else {
			album.IsHidden = "0"
			fmt.Fprintf(c.writer, "Unhid %s\n", item.Filename)
		}
	}
	return commit(true, nil)
}

// Copy copies files to an existing album.
func (c *Client) Copy(patterns []string, dest string) error {
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
	if len(di) == 0 {
		return fmt.Errorf("no match for: %s", dest)
	}
	if len(di) != 1 || !di[0].IsDir {
		return fmt.Errorf("destination must be a directory: %s", dest)
	}
	dst := di[0]
	if dst.Album != nil && dst.Album.IsOwner != "1" && !stingle.Permissions(dst.Album.Permissions).AllowAdd() {
		return fmt.Errorf("adding is not allowed: %s", dest)
	}
	for _, item := range si {
		if item.IsDir {
			return fmt.Errorf("cannot move a directory to another directory: %s", item.Filename)
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
		if err := c.moveFiles(li, dst, false); err != nil {
			return err
		}
	}
	return nil
}

// RenameAlbum renames an album.
func (c *Client) RenameAlbum(patterns []string, dest string) error {
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
	if len(si) > 1 {
		return fmt.Errorf("more than one match for: %s", strings.Join(patterns, " "))
	}
	if len(di) != 0 {
		return fmt.Errorf("destination already exists: %s", di[0].Filename)
	}
	return c.renameAlbum(si[0], dest)
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
		return fmt.Errorf("destination must be a directory: %s", dest)
	}
	dst := di[0]
	if dst.Album != nil && dst.Album.IsOwner != "1" && !stingle.Permissions(dst.Album.Permissions).AllowAdd() {
		return fmt.Errorf("adding is not allowed: %s", dest)
	}
	for _, item := range si {
		if item.IsDir {
			return fmt.Errorf("cannot move a directory to another directory: %s", item.Filename)
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
		if err := c.moveFiles(li, dst, true); err != nil {
			return err
		}
	}
	return nil
}

// Delete moves files trash, or deletes them from trash.
func (c *Client) Delete(patterns []string) error {
	si, err := c.GlobFiles(patterns)
	if err != nil {
		return err
	}
	if len(si) == 0 {
		return nil
	}
	di, err := c.glob("trash")
	if err != nil || len(di) != 1 {
		return err
	}
	groups := make(map[string][]ListItem)
	for _, item := range si {
		if item.IsDir {
			if err := c.removeAlbum(item); err != nil {
				return err
			}
			continue
		}
		key := item.Set + "/"
		if item.Album != nil {
			key += item.Album.AlbumID
		}
		groups[key] = append(groups[key], item)
	}
	for _, li := range groups {
		if li[0].Set == stingle.TrashSet {
			if err := c.deleteFiles(li); err != nil {
				return err
			}
			continue
		}
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
	if li.Album != nil && li.Album.IsOwner != "1" {
		return fmt.Errorf("only the album owner can rename it: %s", li.Filename)
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

	for _, item := range fromItems {
		ff := item.FSFile
		if moving {
			if item.Album != nil && item.Album.IsOwner != "1" {
				return fmt.Errorf("only the album owner can move files: %s", item.Filename)
			}
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
		}
		ff.DateModified = nowJSON()
		ff.AlbumID = toAlbumID
		fs[1].Files[ff.File] = &ff
	}
	return commit(true, nil)
}

func (c *Client) deleteFiles(li []ListItem) (retErr error) {
	commit, fs, err := c.fileSetForUpdate(trashFile)
	if err != nil {
		return err
	}
	defer commit(false, &retErr)

	for _, item := range li {
		if item.Album != nil && item.Album.IsOwner != "1" {
			return fmt.Errorf("only the album owner can delete files: %s", item.Filename)
		}
		if _, ok := fs.Files[item.FSFile.File]; ok {
			fmt.Fprintf(c.writer, "Deleting %s\n", item.Filename)
			delete(fs.Files, item.FSFile.File)
		}
	}
	return commit(true, nil)
}
