package client

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"c2FmZQ/internal/stingle"
)

func (c *Client) AddAlbums(names []string) error {
	li, err := c.GlobFiles(names, GlobOptions{Quiet: true})
	if err != nil {
		return err
	}
	if len(li) > 0 {
		return fmt.Errorf("already exists: %s", li[0].Filename)
	}
	for _, n := range names {
		n := strings.TrimSuffix(n, "/")
		if _, err := c.addAlbum(n); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) addAlbum(name string) (*stingle.Album, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	albumID := base64.RawURLEncoding.EncodeToString(b)
	ask := stingle.MakeSecretKey()
	encPrivateKey := c.SecretKey().PublicKey().SealBoxBase64(ask.ToBytes())
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
		return nil, err
	}
	if al.Albums == nil {
		al.Albums = make(map[string]*stingle.Album)
	}
	if al.RemoteAlbums == nil {
		al.RemoteAlbums = make(map[string]*stingle.Album)
	}
	al.Albums[albumID] = &album
	if err := c.storage.CreateEmptyFile(c.fileHash(albumPrefix+albumID), &FileSet{}); err != nil {
		return nil, err
	}
	if err := commit(true, nil); err != nil {
		return nil, err
	}
	c.Printf("Created %s (not synced)\n", name)
	return &album, nil
}

// RemoveAlbums deletes albums.
func (c *Client) RemoveAlbums(patterns []string) error {
	li, err := c.GlobFiles(patterns, GlobOptions{})
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
	if !item.IsDir || item.Album == nil {
		return fmt.Errorf("cannot remove: %s", item.Filename)
	}
	c.Printf("Removing %s (not synced)\n", item.Filename)
	var al AlbumList
	commit, err := c.storage.OpenForUpdate(c.fileHash(albumList), &al)
	if err != nil {
		return err
	}
	defer commit(false, &retErr)
	delete(al.Albums, item.Album.AlbumID)
	var fs FileSet
	if err := c.storage.ReadDataFile(c.fileHash(albumPrefix+item.Album.AlbumID), &fs); err != nil {
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

// Copy copies files to an existing album.
func (c *Client) Copy(patterns []string, dest string) error {
	dest = strings.TrimSuffix(dest, "/")
	si, err := c.GlobFiles(patterns, GlobOptions{})
	if err != nil {
		return err
	}
	if len(si) == 0 {
		return fmt.Errorf("no match for: %s", strings.Join(patterns, " "))
	}
	for _, item := range si {
		if item.IsDir {
			return fmt.Errorf("cannot move a directory to another directory: %s", item.Filename)
		}
		if item.Set == stingle.TrashSet {
			return fmt.Errorf("cannot copy from trash, only move: %s", item.Filename)
		}
		if item.Album != nil && item.Album.IsOwner != "1" && !stingle.Permissions(item.Album.Permissions).AllowCopy() {
			return fmt.Errorf("copying is not allowed: %s", item.Filename)
		}
	}

	di, err := c.glob(dest, GlobOptions{})
	if err != nil {
		return err
	}
	if len(di) == 0 {
		return fmt.Errorf("no match for: %s", dest)
	}
	if len(di) != 1 || !di[0].IsDir {
		return fmt.Errorf("destination must be a directory: %s", dest)
	}
	dst := di[0]
	if dst.Set == "" {
		album, err := c.addAlbum(dst.Filename)
		if err != nil {
			return err
		}
		dst.Set = stingle.AlbumSet
		dst.Album = album
		dst.FileSet = albumPrefix + album.AlbumID
	}

	if dst.Album != nil && dst.Album.IsOwner != "1" && !stingle.Permissions(dst.Album.Permissions).AllowAdd() {
		return fmt.Errorf("adding is not allowed: %s", dest)
	}
	if dst.Set == stingle.TrashSet {
		return fmt.Errorf("cannot copy to trash, only move: %s", dst.Filename)
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
	di, err := c.glob(dest, GlobOptions{})
	if err != nil {
		return err
	}
	si, err := c.GlobFiles(patterns, GlobOptions{})
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
	return c.renameDir(si[0], dest, true)
}

// Move moves files to an existing album, or renames an album.
func (c *Client) Move(patterns []string, dest string) error {
	dest = strings.TrimSuffix(dest, "/")
	si, err := c.GlobFiles(patterns, GlobOptions{})
	if err != nil {
		return err
	}
	if len(si) == 0 {
		return fmt.Errorf("no match for: %s", strings.Join(patterns, " "))
	}
	for _, item := range si {
		if item.Album != nil && item.Album.IsOwner != "1" {
			return fmt.Errorf("moving is not allowed: %s", item.Filename)
		}
	}

	di, err := c.glob(dest, GlobOptions{})
	if err != nil {
		return err
	}
	// Rename an album.
	if len(si) == 1 && si[0].IsDir && len(di) == 0 {
		return c.renameDir(si[0], dest, true)
	}
	// Move to a different directory.
	if len(di) != 1 || !di[0].IsDir {
		return fmt.Errorf("destination must be a directory: %s", dest)
	}
	dst := di[0]
	if dst.Set == "" {
		album, err := c.addAlbum(dst.Filename)
		if err != nil {
			return err
		}
		dst.Set = stingle.AlbumSet
		dst.Album = album
		dst.FileSet = albumPrefix + album.AlbumID
	}

	if dst.Album != nil && dst.Album.IsOwner != "1" && !stingle.Permissions(dst.Album.Permissions).AllowAdd() {
		return fmt.Errorf("adding is not allowed: %s", dest)
	}
	groups := make(map[string][]ListItem)
	for _, item := range si {
		if item.IsDir {
			_, n := path.Split(item.Filename)
			newName := path.Join(dst.Filename, n)
			di, err := c.glob(newName, GlobOptions{ExactMatch: true})
			if err != nil {
				return err
			}
			if len(di) > 0 {
				return fmt.Errorf("already exists: %v", newName)
			}
			if err := c.renameDir(item, newName, true); err != nil {
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
		if err := c.moveFiles(li, dst, true); err != nil {
			return err
		}
	}
	return nil
}

// Delete moves files trash, or deletes them from trash.
func (c *Client) Delete(patterns []string) error {
	si, err := c.GlobFiles(patterns, GlobOptions{})
	if err != nil {
		return err
	}
	if len(si) == 0 {
		return nil
	}
	di, err := c.glob(".trash", GlobOptions{})
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

func (c *Client) renameDir(item ListItem, name string, recursive bool) (retErr error) {
	name = strings.TrimSuffix(name, "/")
	if name == "" {
		return fmt.Errorf("illegal name: %q", name)
	}
	if item.Album != nil {
		if item.Album.IsOwner != "1" {
			return fmt.Errorf("only the album owner can rename it: %s", item.Filename)
		}
		pk, err := item.Album.PK()
		if err != nil {
			return err
		}

		c.Printf("Renaming %s -> %s (not synced)\n", strings.TrimSuffix(item.Filename, "/"), name)

		var al AlbumList
		commit, err := c.storage.OpenForUpdate(c.fileHash(albumList), &al)
		if err != nil {
			return err
		}
		md := stingle.EncryptAlbumMetadata(stingle.AlbumMetadata{Name: name}, pk)
		al.Albums[item.Album.AlbumID].Metadata = md
		al.Albums[item.Album.AlbumID].DateModified = nowJSON()
		if err := commit(true, nil); err != nil {
			return err
		}
	}
	if !recursive {
		return nil
	}

	oldPrefix := item.Filename + "/"
	newPrefix := name + "/"
	li, err := c.glob(oldPrefix+"*", GlobOptions{MatchDot: true, Recursive: true})
	if err != nil {
		return err
	}
	var errList []error
	for _, item := range li {
		if !item.IsDir || item.Album == nil {
			continue
		}
		newName := newPrefix + item.Filename[len(oldPrefix):]
		if err := c.renameDir(item, newName, false); err != nil {
			errList = append(errList, err)
		}
	}
	if errList != nil {
		return fmt.Errorf("%w [%v]", errList[0], errList[1:])
	}
	return nil
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

	sk, pk := c.SecretKey(), c.SecretKey().PublicKey()
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
			c.Printf("Moving %s -> %s (not synced)\n", item.Filename, toItem.Filename)
			delete(fs[0].Files, ff.File)
		} else {
			c.Printf("Copying %s -> %s (not synced)\n", item.Filename, toItem.Filename)
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
			c.Printf("Deleting %s (not synced)\n", item.Filename)
			delete(fs.Files, item.FSFile.File)
		}
	}
	return commit(true, nil)
}
