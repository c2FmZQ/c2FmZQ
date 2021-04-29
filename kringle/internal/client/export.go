package client

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"kringle/internal/stingle"
)

// ExportFiles decrypts and exports files to dir. Returns the number of files exported.
func (c *Client) ExportFiles(patterns []string, dir string) (int, error) {
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return 0, fmt.Errorf("%s is not a directory", dir)
	}
	li, err := c.GlobFiles(patterns)
	if err != nil {
		return 0, err
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
	count := len(li) - len(errors)
	if errors != nil {
		return count, fmt.Errorf("%w %v", errors[0], errors[1:])
	}
	return count, nil
}

// Cat decrypts and sends the plaintext to stdout.
func (c *Client) Cat(patterns []string) error {
	li, err := c.GlobFiles(patterns)
	if err != nil {
		return err
	}
	for _, item := range li {
		if err := c.catFile(item); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) catFile(item ListItem) error {
	f, err := os.Open(item.FilePath)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := stingle.SkipHeader(f); err != nil {
		return err
	}
	_, err = io.Copy(os.Stdout, stingle.DecryptFile(f, item.Header))
	return err
}

func (c *Client) exportWorker(ch <-chan ListItem, out chan<- error, dir string) {
	for i := range ch {
		_, fn := filepath.Split(string(i.Header.Filename))
		c.Printf("Exporting %s\n", filepath.Join(dir, fn))
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
