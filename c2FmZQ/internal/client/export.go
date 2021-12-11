package client

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"c2FmZQ/internal/stingle"
)

// ExportFiles decrypts and exports files to dir. Returns the number of files exported.
func (c *Client) ExportFiles(patterns []string, dir string, recursive bool) (int, error) {
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return 0, fmt.Errorf("%s is not a directory", dir)
	}
	li, err := c.GlobFiles(patterns, GlobOptions{})
	if err != nil {
		return 0, err
	}

	type srcdst struct {
		src ListItem
		dst string
	}

	var toExport []srcdst
	for _, item := range li {
		if !item.IsDir {
			toExport = append(toExport, srcdst{item, dir})
			continue
		}
		if !recursive {
			continue
		}
		si, err := c.glob(filepath.Join(item.Filename, "*"), GlobOptions{ExactMatchExceptLast: true, Recursive: true})
		if err != nil {
			return 0, err
		}
		parent, _ := filepath.Split(item.Filename)
		for _, item2 := range si {
			if item2.IsDir {
				continue
			}
			d, _ := filepath.Split(item2.Filename)
			rel, err := filepath.Rel(parent, d)
			if err != nil {
				return 0, err
			}
			toExport = append(toExport, srcdst{item2, filepath.Join(dir, rel)})
		}
	}
	qCh := make(chan srcdst)
	eCh := make(chan error)
	for i := 0; i < 5; i++ {
		go func() {
			for i := range qCh {
				sk := c.SecretKey()
				hdr, err := i.src.Header(sk)
				sk.Wipe()
				if err != nil {
					eCh <- err
					continue
				}
				_, fn := filepath.Split(string(hdr.Filename))
				c.Printf("Exporting %s -> %s\n", i.src.Filename, filepath.Join(i.dst, sanitize(fn)))
				eCh <- c.exportFile(i.src, i.dst, hdr)
				hdr.Wipe()
			}
		}()
	}
	go func() {
		for _, i := range toExport {
			qCh <- i
		}
		close(qCh)
	}()
	var errors []error
	for range toExport {
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
	li, err := c.GlobFiles(patterns, GlobOptions{})
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
	var f io.ReadCloser
	var err error
	if f, err = os.Open(item.FilePath); errors.Is(err, os.ErrNotExist) {
		f, err = c.download(item.FSFile.File, item.Set, "0")
	}
	if err != nil {
		return err
	}
	defer f.Close()
	if err := stingle.SkipHeader(f); err != nil {
		return err
	}
	sk := c.SecretKey()
	hdr, err := item.Header(sk)
	sk.Wipe()
	defer hdr.Wipe()
	if err != nil {
		return err
	}
	_, err = io.Copy(os.Stdout, stingle.DecryptFile(f, hdr))
	return err
}

func (c *Client) exportFile(item ListItem, dir string, hdr *stingle.Header) (err error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
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
	_, fn := filepath.Split(sanitize(string(hdr.Filename)))
	if fn == "" {
		_, fn = filepath.Split(sanitize(string(item.FSFile.File)))
		fn = "decrypted-" + fn
	}
	fn = filepath.Join(dir, fn)
	tmp := fmt.Sprintf("%s-tmp-%d", fn, time.Now().UnixNano())
	out, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL|os.O_SYNC, 0600)
	if err != nil {
		return err
	}
	r := stingle.DecryptFile(in, hdr)
	if _, err := io.Copy(out, r); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, fn)
}
