package client

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"kringle-server/stingle"
)

func (c *Client) ExportFiles(patterns []string, dir string) error {
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return fmt.Errorf("%s is not a directory", dir)
	}
	li, err := c.GlobFiles(patterns)
	if err != nil {
		return err
	}
	qCh := make(chan ListItem)
	eCh := make(chan error)
	for i := 0; i < 5; i++ {
		go c.exportWorker(qCh, eCh, dir)
	}
	go func() {
		for _, item := range li {
			qCh <- item
		}
		close(qCh)
	}()
	var errors []error
	for range li {
		if err := <-eCh; err != nil {
			errors = append(errors, err)
		}
	}
	if errors != nil {
		fmt.Fprintf(c.writer, "Files exported successfully: %d, %d with errors.\n", len(li)-len(errors), len(errors))
	} else {
		fmt.Fprintf(c.writer, "Files exported successfully: %d\n", len(li))
	}
	if errors != nil {
		return fmt.Errorf("%w %v", errors[0], errors[1:])
	}
	return nil
}

func (c *Client) exportWorker(ch <-chan ListItem, out chan<- error, dir string) {
	for i := range ch {
		out <- c.exportFile(i, dir)
	}
}

func (c *Client) exportFile(item ListItem, dir string) (err error) {
	var in io.ReadCloser
	if in, err = os.Open(item.FilePath); errors.Is(err, os.ErrNotExist) {
		in, err = c.download(item.FSFile.File, item.Set, "0")
	}
	if err != nil {
		return err
	}
	defer in.Close()
	if err := stingle.SkipHeader(in); err != nil {
		return err
	}
	_, fn := filepath.Split(string(item.Header.Filename))
	if fn == "" {
		_, fn = filepath.Split(string(item.FSFile.File))
		fn = "decrypted-" + fn
	}
	fn = filepath.Join(dir, fn)

	tmp := fmt.Sprintf("%s-tmp-%d", fn, time.Now().UnixNano())
	out, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL|os.O_SYNC, 0600)
	if err != nil {
		return err
	}
	r := stingle.DecryptFile(in, item.Header)
	if _, err := io.Copy(out, r); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, fn)
}
