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
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestBackupRestore(t *testing.T) {
	dir := t.TempDir()
	s := NewStorage(dir, aesEncryptionKey())

	if err := os.Mkdir(filepath.Join(dir, "data"), 0700); err != nil {
		t.Fatalf("os.Mkdir: %v", err)
	}
	var files []string
	for i := 1; i <= 10; i++ {
		file := filepath.Join("data", fmt.Sprintf("file%d", i))
		if err := os.WriteFile(filepath.Join(dir, file), []byte(fmt.Sprintf("This is file %d", i)), 0600); err != nil {
			t.Fatalf("os.WriteFile: %v", err)
		}
		files = append(files, file)
	}
	bck, err := s.createBackup(files)
	if err != nil {
		t.Fatalf("s.createBackup: %v", err)
	}

	var got backup
	if err := s.ReadDataFile(filepath.Join("pending", fmt.Sprintf("%d", bck.TS.UnixNano())), &got); err != nil {
		t.Fatalf("s.ReadDataFile: %v", err)
	}
	if want := files; !reflect.DeepEqual(want, got.Files) {
		t.Errorf("Unexpected pending op files. Want %+v, got %+v", want, got)
	}

	for i := 1; i <= 10; i++ {
		file := filepath.Join(dir, "data", fmt.Sprintf("file%d", i))
		if err := os.WriteFile(file+".tmp", []byte("XXXXXX"), 0600); err != nil {
			t.Fatalf("os.WriteFile: %v", err)
		}
		if err := os.Rename(file+".tmp", file); err != nil {
			t.Fatalf("os.Rename: %v", err)
		}
		files = append(files, file)
	}
	bck.restore()
	if err := s.ReadDataFile(filepath.Join("pending", fmt.Sprintf("%d", bck.TS.UnixNano())), &got); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("pending ops file should have been deleted: %v", err)
	}

	for i := 1; i <= 10; i++ {
		file := filepath.Join("data", fmt.Sprintf("file%d", i))
		b, err := os.ReadFile(filepath.Join(dir, file))
		if err != nil {
			t.Fatalf("os.ReadFile: %v", err)
		}
		if want, got := fmt.Sprintf("This is file %d", i), string(b); want != got {
			t.Errorf("Unexpected file content after restore. Want %q, got %q", want, got)
		}

	}
}

func TestBackupDelete(t *testing.T) {
	dir := t.TempDir()
	s := NewStorage(dir, aesEncryptionKey())

	if err := os.Mkdir(filepath.Join(dir, "data"), 0700); err != nil {
		t.Fatalf("os.Mkdir: %v", err)
	}
	var files []string
	for i := 1; i <= 10; i++ {
		file := filepath.Join("data", fmt.Sprintf("file%d", i))
		if err := os.WriteFile(filepath.Join(dir, file), []byte(fmt.Sprintf("This is file %d", i)), 0600); err != nil {
			t.Fatalf("os.WriteFile: %v", err)
		}
		files = append(files, file)
	}
	bck, err := s.createBackup(files)
	if err != nil {
		t.Fatalf("s.createBackup: %v", err)
	}

	var got backup
	if err := s.ReadDataFile(filepath.Join("pending", fmt.Sprintf("%d", bck.TS.UnixNano())), &got); err != nil {
		t.Fatalf("s.ReadDataFile: %v", err)
	}
	if want := files; !reflect.DeepEqual(want, got.Files) {
		t.Errorf("Unexpected pending op files. Want %+v, got %+v", want, got)
	}

	for i := 1; i <= 10; i++ {
		file := filepath.Join("data", fmt.Sprintf("file%d", i))
		if err := os.WriteFile(filepath.Join(dir, file), []byte("XXXXXX"), 0600); err != nil {
			t.Fatalf("os.WriteFile: %v", err)
		}
		files = append(files, file)
	}
	bck.delete()
	if err := s.ReadDataFile(filepath.Join("pending", fmt.Sprintf("%d", bck.TS.UnixNano())), &got); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("pending ops file should have been deleted: %v", err)
	}

	for i := 1; i <= 10; i++ {
		file := filepath.Join("data", fmt.Sprintf("file%d", i))
		b, err := os.ReadFile(filepath.Join(dir, file))
		if err != nil {
			t.Fatalf("os.ReadFile: %v", err)
		}
		if want, got := "XXXXXX", string(b); want != got {
			t.Errorf("Unexpected file content after restore. Want %q, got %q", want, got)
		}

	}
}

func TestRestorePendingOps(t *testing.T) {
	dir := t.TempDir()
	ed := aesEncryptionKey()
	s := NewStorage(dir, ed)

	if err := os.Mkdir(filepath.Join(dir, "data"), 0700); err != nil {
		t.Fatalf("os.Mkdir: %v", err)
	}
	var files []string
	for i := 1; i <= 10; i++ {
		file := filepath.Join("data", fmt.Sprintf("file%d", i))
		if err := os.WriteFile(filepath.Join(dir, file), []byte(fmt.Sprintf("This is file %d", i)), 0600); err != nil {
			t.Fatalf("os.WriteFile: %v", err)
		}
		files = append(files, file)
	}
	if _, err := s.createBackup(files); err != nil {
		t.Fatalf("s.createBackup: %v", err)
	}
	for i := 1; i <= 10; i++ {
		file := filepath.Join(dir, "data", fmt.Sprintf("file%d", i))
		if err := os.WriteFile(file+".tmp", []byte("XXXXXX"), 0600); err != nil {
			t.Fatalf("os.WriteFile: %v", err)
		}
		if err := os.Rename(file+".tmp", file); err != nil {
			t.Fatalf("os.Rename: %v", err)
		}
		files = append(files, file)
	}

	// NewStorage will notice the aborted operation and roll it back.
	s = NewStorage(dir, ed)

	for i := 1; i <= 10; i++ {
		file := filepath.Join("data", fmt.Sprintf("file%d", i))
		b, err := os.ReadFile(filepath.Join(dir, file))
		if err != nil {
			t.Fatalf("os.ReadFile: %v", err)
		}
		if want, got := fmt.Sprintf("This is file %d", i), string(b); want != got {
			t.Errorf("Unexpected file content after restore. Want %q, got %q", want, got)
		}

	}
}
