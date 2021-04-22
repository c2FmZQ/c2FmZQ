package client

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// Sync downloads all the files matching pattern that are not already present
// in the local storage.
func (c *Client) Sync(pattern string) error {
	list, err := c.GlobFiles(pattern)
	if err != nil {
		return err
	}
	files := make(map[string]ListItem)
	for _, item := range list {
		fn := c.blobPath(item.File, false)
		if _, err := os.Stat(fn); errors.Is(err, os.ErrNotExist) {
			files[item.File] = item
		}
	}

	qCh := make(chan ListItem)
	eCh := make(chan error)
	for i := 0; i < 5; i++ {
		go c.downloadWorker(qCh, eCh)
	}
	go func() {
		for _, li := range files {
			qCh <- li
		}
		close(qCh)
	}()
	var errors []error
	for range files {
		if err := <-eCh; err != nil {
			errors = append(errors, err)
		}
	}
	fmt.Printf("Files downloaded: %d Errors: %d\n", len(files)-len(errors), len(errors))
	if errors != nil {
		return fmt.Errorf("%w %v", errors[0], errors[1:])
	}
	return nil
}

// Free deletes all the files matching pattern that are already present in the
// remote storage.
func (c *Client) Free(pattern string) error {
	list, err := c.GlobFiles(pattern)
	if err != nil {
		return err
	}
	count := 0
	for _, item := range list {
		fn := c.blobPath(item.File, false)
		if _, err := os.Stat(fn); errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err := os.Remove(fn); err != nil {
			return err
		}
		count++
	}
	fmt.Printf("Successfully freed %d file(s).\n", count)
	return nil
}

func (c *Client) blobPath(name string, thumb bool) string {
	if thumb {
		name = name + "-thumb"
	}
	n := c.storage.HashString(name)
	return filepath.Join(c.storage.Dir(), blobsDir, n[:2], n)
}

func (c *Client) downloadWorker(ch <-chan ListItem, out chan<- error) {
	for i := range ch {
		out <- c.downloadFile(i)
	}
}

func (c *Client) downloadFile(li ListItem) error {
	r, err := c.download(li.File, li.Set, "0")
	if err != nil {
		return err
	}
	defer r.Close()
	fn := c.blobPath(li.File, false)
	dir, _ := filepath.Split(fn)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	tmp := fmt.Sprintf("%s-tmp-%d", fn, time.Now().UnixNano())
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL|os.O_SYNC, 0600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, fn)
}
