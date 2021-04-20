package client

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
)

func (c *Client) filepath(name string, thumb bool) string {
	if thumb {
		name = name + "-thumb"
	}
	n := c.storage.HashString(name)
	return filepath.Join(c.storage.Dir(), blobsDir, n[:2], n)
}

type downloadItem struct {
	url  string
	path string
}

func (c *Client) downloadWorker(ch <-chan downloadItem, out chan<- error) {
	for i := range ch {
		out <- c.downloadFile(i.url, i.path)
	}
}

func (c *Client) downloadFile(url, path string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status code %d", resp.StatusCode)
	}
	dir, _ := filepath.Split(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	tmp := path + "-new"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL|os.O_SYNC, 0600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (c *Client) syncFileSet(name string, urls map[string]string) error {
	var fs FileSet
	if _, err := c.storage.ReadDataFile(c.storage.HashString(name), &fs); err != nil {
		return err
	}
	var queue []string
	for f := range fs.Files {
		fn := c.filepath(f, false)
		if _, err := os.Stat(fn); errors.Is(err, os.ErrNotExist) {
			queue = append(queue, f)
		}
	}
	if len(queue) == 0 {
		return nil
	}
	var set string
	switch name {
	case galleryFile:
		set = "0"
	case trashFile:
		set = "1"
	default:
		set = "2"
	}
	form := url.Values{}
	form.Set("token", c.Token)
	form.Set("is_thumb", "0")
	for i, f := range queue {
		form.Set(fmt.Sprintf("files[%d][filename]", i), f)
		form.Set(fmt.Sprintf("files[%d][set]", i), set)
	}
	sr, err := c.sendRequest("/v2/sync/getDownloadUrls", form)
	if err != nil {
		return err
	}
	u := make(map[string]string)
	copyJSON(sr.Parts["urls"], &u)
	for k, v := range u {
		urls[k] = v
	}
	return nil
}

// Sync downloads all the files that are not already present in the local
// storage.
func (c *Client) Sync() error {
	urls := make(map[string]string)
	if err := c.syncFileSet(galleryFile, urls); err != nil {
		return err
	}
	if err := c.syncFileSet(trashFile, urls); err != nil {
		return err
	}
	var al AlbumList
	_, err := c.storage.ReadDataFile(c.storage.HashString(albumList), &al)
	if err != nil {
		return err
	}
	for album := range al.Albums {
		if err := c.syncFileSet(albumPrefix+album, urls); err != nil {
			return err
		}
	}

	qCh := make(chan downloadItem)
	eCh := make(chan error)
	for i := 0; i < 5; i++ {
		go c.downloadWorker(qCh, eCh)
	}
	go func() {
		for f, url := range urls {
			qCh <- downloadItem{url, c.filepath(f, false)}
		}
		close(qCh)
	}()
	var errors []error
	for range urls {
		if err := <-eCh; err != nil {
			errors = append(errors, err)
		}
	}
	fmt.Printf("Files downloaded: %d Errors: %d\n", len(urls)-len(errors), len(errors))
	if errors != nil {
		return fmt.Errorf("%w %v", errors[0], errors[1:])
	}
	return nil
}
