package md

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

func (md *Metadata) createBackup(files []string) (*backup, error) {
	b := &backup{dir: md.dir, TS: time.Now(), Files: files}
	if err := b.backup(); err != nil {
		return nil, err
	}
	b.pending = filepath.Join("pending", fmt.Sprintf("%d", b.TS.UnixNano()))
	if err := md.SaveDataFile(nil, b.pending, b); err != nil {
		return nil, err
	}
	return b, nil
}

func (md *Metadata) recoverPendingOps() error {
	m, err := filepath.Glob(filepath.Join(md.dir, "pending", "*"))
	if err != nil {
		return err
	}
	for _, f := range m {
		rel, err := filepath.Rel(md.dir, f)
		if err != nil {
			return err
		}
		var b backup
		if _, err := md.ReadDataFile(rel, &b); err != nil {
			return err
		}
		b.dir = md.dir
		b.pending = rel
		if err := b.restore(); err != nil {
			return err
		}
		md.UnlockMany(b.Files)
	}
	return nil
}

type backup struct {
	// The timestamp of the backup.
	TS time.Time `json:"ts"`
	// Relative file names.
	Files []string `json:"files"`

	// The root of the data directory.
	dir string
	// The relative file name of the pending ops file.
	pending string
}

func (b *backup) backup() error {
	for _, f := range b.Files {
		fn := filepath.Join(b.dir, f)
		if err := copyFile(b.fileName(fn), fn); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func (b *backup) restore() error {
	for _, f := range b.Files {
		fn := filepath.Join(b.dir, f)
		if err := os.Rename(b.fileName(fn), fn); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return os.Remove(filepath.Join(b.dir, b.pending))
}

func (b *backup) delete() error {
	for _, f := range b.Files {
		fn := filepath.Join(b.dir, f)
		if err := os.Remove(b.fileName(fn)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return os.Remove(filepath.Join(b.dir, b.pending))
}

func (b *backup) fileName(f string) string {
	return fmt.Sprintf("%s.bck-%d", f, b.TS.UnixNano())
}

func copyFile(dst, src string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		in.Close()
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		in.Close()
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		in.Close()
		return err
	}
	return in.Close()
}
