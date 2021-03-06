//
// Copyright 2021-2022 TTBT Enterprises LLC
//
// This file is part of c2FmZQ (https://c2FmZQ.org/).
//
// c2FmZQ is free software: you can redistribute it and/or modify it under the
// terms of the GNU General Public License as published by the Free Software
// Foundation, either version 3 of the License, or (at your option) any later
// version.
//
// c2FmZQ is distributed in the hope that it will be useful, but WITHOUT ANY
// WARRANTY; without even the implied warranty of MERCHANTABILITY or FITNESS FOR
// A PARTICULAR PURPOSE. See the GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License along with
// c2FmZQ. If not, see <https://www.gnu.org/licenses/>.

package secure

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"c2FmZQ/internal/log"
)

func (s *Storage) createBackup(files []string) (*backup, error) {
	b := &backup{dir: s.dir, TS: time.Now(), Files: files}
	if err := b.backup(); err != nil {
		return nil, err
	}
	b.pending = filepath.Join("pending", fmt.Sprintf("%d", b.TS.UnixNano()))
	if err := s.SaveDataFile(b.pending, b); err != nil {
		return nil, err
	}
	return b, nil
}

func (s *Storage) rollbackPendingOps() error {
	m, err := filepath.Glob(filepath.Join(s.dir, "pending", "*"))
	if err != nil {
		return err
	}
	for _, f := range m {
		rel, err := filepath.Rel(s.dir, f)
		if err != nil {
			return err
		}
		var b backup
		if err := s.ReadDataFile(rel, &b); err != nil {
			return err
		}
		b.dir = s.dir
		b.pending = rel
		// Make sure pending is this backup is really abandoned.
		time.Sleep(time.Until(b.TS.Add(5 * time.Second)))
		if err := b.restore(); err != nil {
			return err
		}
		log.Infof("Rolled back pending operation %d [%v]", b.TS.UnixNano(), b.Files)
		// The abandoned files were most likely locked.
		s.UnlockMany(b.Files)
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
	ch := make(chan error)
	for _, f := range b.Files {
		go func(fn string) { ch <- copyFile(b.backupFileName(fn), fn) }(filepath.Join(b.dir, f))
	}
	var errList []error
	for _ = range b.Files {
		if err := <-ch; err != nil && !errors.Is(err, os.ErrNotExist) {
			errList = append(errList, err)
		}
	}
	if errList != nil {
		return fmt.Errorf("%w %v", errList[0], errList[1:])
	}
	return nil
}

func (b *backup) restore() error {
	ch := make(chan error)
	for _, f := range b.Files {
		go func(fn string) { ch <- os.Rename(b.backupFileName(fn), fn) }(filepath.Join(b.dir, f))
	}
	var errList []error
	for _ = range b.Files {
		if err := <-ch; err != nil && !errors.Is(err, os.ErrNotExist) {
			errList = append(errList, err)
		}
	}
	if errList != nil {
		return fmt.Errorf("%w %v", errList[0], errList[1:])
	}
	if err := os.Remove(filepath.Join(b.dir, b.pending)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (b *backup) delete() error {
	ch := make(chan error)
	for _, f := range b.Files {
		go func(fn string) { ch <- os.Remove(b.backupFileName(fn)) }(filepath.Join(b.dir, f))
	}
	var errList []error
	for _ = range b.Files {
		if err := <-ch; err != nil && !errors.Is(err, os.ErrNotExist) {
			errList = append(errList, err)
		}
	}
	if errList != nil {
		return fmt.Errorf("%w %v", errList[0], errList[1:])
	}
	if err := os.Remove(filepath.Join(b.dir, b.pending)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (b *backup) backupFileName(f string) string {
	return fmt.Sprintf("%s.bck-%d", f, b.TS.UnixNano())
}

func copyFile(dst, src string) error {
	if err := os.Link(src, dst); err == nil {
		return nil
	}
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
