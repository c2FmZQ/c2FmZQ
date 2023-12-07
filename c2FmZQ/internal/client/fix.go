package client

import (
	"c2FmZQ/internal/log"
	"c2FmZQ/internal/stingle"
)

// FixFiles
func (c *Client) FixFiles(dir string) error {
	li, err := c.GlobFiles([]string{dir}, GlobOptions{})
	if err != nil {
		return err
	}
	for _, item := range li {
		if !item.IsDir || item.Album == nil {
			c.Print("nothing to fix")
			return nil
		}
	}
	for _, item := range li {
		c.fixFileSet(item.Album, item.FileSet)
	}
	return nil
}

func (c *Client) fixFileSet(album *stingle.Album, name string) (retErr error) {
	commit, fs, err := c.fileSetForUpdate(name)
	if err != nil {
		return err
	}
	defer commit(true, &retErr)
	sk, err := c.SKForAlbum(album)
	if err != nil {
		return err
	}
	defer sk.Wipe()
	for k, v := range fs.Files {
		if _, err := stingle.DecryptBase64Headers(v.Headers, sk); err == nil {
			continue
		}
		log.Infof("Deleting %s (not synced)", k)
		delete(fs.Files, k)
	}
	return nil
}
