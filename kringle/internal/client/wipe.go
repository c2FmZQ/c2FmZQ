package client

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// WipeAccount deletes all the files associated with the current account.
func (c *Client) WipeAccount(password string) error {
	if c.Account != nil {
		if err := c.checkPassword(password); err != nil {
			return err
		}
	}
	var errList []error

	if errs := c.wipeFileSet(galleryFile); errs != nil {
		errList = append(errList, errs...)
	}
	if errs := c.wipeFileSet(trashFile); errs != nil {
		errList = append(errList, errs...)
	}
	var al AlbumList
	if _, err := c.storage.ReadDataFile(c.fileHash(albumList), &al); err != nil {
		return err
	}
	for _, album := range al.Albums {
		if errs := c.wipeFileSet(albumPrefix + album.AlbumID); errs != nil {
			errList = append(errList, errs...)
		}
	}
	if err := os.Remove(filepath.Join(c.storage.Dir(), c.fileHash(albumList))); err != nil {
		errList = append(errList, err)
	}
	if err := os.Remove(filepath.Join(c.storage.Dir(), c.fileHash(contactsFile))); err != nil {
		errList = append(errList, err)
	}
	if c.Account != nil {
		c.Account = nil
		if err := c.Save(); err != nil {
			errList = append(errList, err)
		}
	} else {
		if err := os.Remove(filepath.Join(c.storage.Dir(), c.cfgFile())); err != nil {
			errList = append(errList, err)
		}
	}
	if errList != nil {
		for _, err := range errList {
			c.Printf("ERR: %v\n", err)
		}
		return fmt.Errorf("wipe errors: %w (%v)", errList[0], errList[1:])
	}
	c.Print("All data was deleted.")
	return nil
}

func (c *Client) wipeFileSet(name string) (errList []error) {
	fn := c.fileHash(name)
	var fs FileSet
	if _, err := c.storage.ReadDataFile(fn, &fs); err != nil {
		errList = append(errList, err)
	}
	for _, f := range fs.Files {
		if err := os.Remove(c.blobPath(f.File, false)); err != nil && !errors.Is(err, os.ErrNotExist) {
			errList = append(errList, err)
		}
		if err := os.Remove(c.blobPath(f.File, true)); err != nil && !errors.Is(err, os.ErrNotExist) {
			errList = append(errList, err)
		}

	}
	if err := os.Remove(filepath.Join(c.storage.Dir(), fn)); err != nil {
		errList = append(errList, err)
	}
	return errList
}
