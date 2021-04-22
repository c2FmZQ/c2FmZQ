package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"kringle-server/log"
	"kringle-server/stingle"
)

// Pull downloads all the files matching pattern that are not already present
// in the local storage.
func (c *Client) Pull(patterns []string) error {
	list, err := c.GlobFiles(patterns)
	if err != nil {
		return err
	}
	files := make(map[string]ListItem)
	for _, item := range list {
		if item.FSFile.LocalOnly {
			continue
		}
		fn := c.blobPath(item.FSFile.File, false)
		if _, err := os.Stat(fn); errors.Is(err, os.ErrNotExist) {
			files[item.FSFile.File] = item
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
	if len(files) == 0 {
		fmt.Println("No files to download.")
	} else if errors == nil {
		fmt.Printf("Successfully downloaded %d file(s).\n", len(files))
	} else {
		fmt.Printf("Successfully downloaded %d file(s), %d failed.\n", len(files)-len(errors), len(errors))
	}
	if errors != nil {
		return fmt.Errorf("%w %v", errors[0], errors[1:])
	}
	return nil
}

// Push uploads all the files matching pattern that have not yet been uploaded.
func (c *Client) Push(patterns []string) error {
	list, err := c.GlobFiles(patterns)
	if err != nil {
		return err
	}
	files := make(map[string]ListItem)
	for _, item := range list {
		if !item.FSFile.LocalOnly {
			continue
		}
		files[item.FSFile.File] = item
	}

	qCh := make(chan ListItem)
	eCh := make(chan error)
	for i := 0; i < 5; i++ {
		go c.uploadWorker(qCh, eCh)
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

	if len(files) == 0 {
		fmt.Println("No files to upload.")
	} else if errors == nil {
		fmt.Printf("Successfully uploaded %d file(s).\n", len(files))
	} else {
		fmt.Printf("Successfully uploaded %d file(s), %d failed.\n", len(files)-len(errors), len(errors))
	}
	if errors != nil {
		return fmt.Errorf("%w %v", errors[0], errors[1:])
	}
	return nil
}

// Free deletes all the files matching pattern that are already present in the
// remote storage.
func (c *Client) Free(patterns []string) error {
	list, err := c.GlobFiles(patterns)
	if err != nil {
		return err
	}
	count := 0
	for _, item := range list {
		if item.FSFile.LocalOnly {
			continue
		}
		fn := c.blobPath(item.FSFile.File, false)
		if _, err := os.Stat(fn); errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err := os.Remove(fn); err != nil {
			return err
		}
		count++
	}
	if count == 0 {
		fmt.Println("There are no files to delete.")
	} else {
		fmt.Printf("Successfully freed %d file(s).\n", count)
	}
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

func (c *Client) uploadWorker(ch <-chan ListItem, out chan<- error) {
	for i := range ch {
		out <- c.uploadFile(i)
	}
}

func (c *Client) downloadFile(li ListItem) error {
	r, err := c.download(li.FSFile.File, li.Set, "0")
	if err != nil {
		return err
	}
	defer r.Close()
	fn := c.blobPath(li.FSFile.File, false)
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

func (c *Client) uploadFile(item ListItem) error {
	hc := http.Client{}

	pr, pw := io.Pipe()
	w := multipart.NewWriter(pw)

	go func() {
		defer pw.Close()
		for _, f := range []string{"file", "thumb"} {
			pw, err := w.CreateFormFile(f, item.FSFile.File)
			if err != nil {
				log.Errorf("multipart.CreateFormFile(%s): %v", item.FSFile.File, err)
				return
			}
			in, err := os.Open(c.blobPath(item.FSFile.File, f == "thumb"))
			if err != nil {
				log.Errorf("Open(%s): %v", item.FSFile.File, err)
				return
			}
			if _, err := io.Copy(pw, in); err != nil {
				log.Errorf("Read(%s): %v", item.FSFile.File, err)
				return
			}
			if err := in.Close(); err != nil {
				log.Errorf("Close(%s): %v", item.FSFile.File, err)
				return
			}
		}
		for _, f := range []struct{ name, value string }{
			{"headers", item.FSFile.Headers},
			{"set", item.Set},
			{"albumId", item.AlbumID},
			{"dateCreated", item.FSFile.DateCreated.String()},
			{"dateModified", item.FSFile.DateModified.String()},
			{"version", item.FSFile.Version},
			{"token", c.Token},
		} {
			pw, err := w.CreateFormField(f.name)
			if err != nil {
				log.Errorf("Metadata(%s): %v", item.FSFile.File, err)
				return
			}
			if _, err := pw.Write([]byte(f.value)); err != nil {
				log.Errorf("Metadata(%s): %v", item.FSFile.File, err)
				return
			}
		}
		if err := w.Close(); err != nil {
			log.Errorf("multipart.Writer(%s): %v", item.FSFile.File, err)
			return
		}
	}()

	url := c.ServerBaseURL + "/v2/sync/upload"

	resp, err := hc.Post(url, w.FormDataContentType(), pr)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("request returned status code %d", resp.StatusCode)
	}
	dec := json.NewDecoder(resp.Body)
	dec.UseNumber()
	var sr stingle.Response
	if err := dec.Decode(&sr); err != nil {
		return err
	}
	log.Debugf("Response: %v", sr)
	if sr.Status != "ok" {
		return sr
	}
	return nil
}
