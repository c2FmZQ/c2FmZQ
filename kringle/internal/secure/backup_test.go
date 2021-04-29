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
	s := NewStorage(dir, encryptionKey())

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
	if _, err := s.ReadDataFile(filepath.Join("pending", fmt.Sprintf("%d", bck.TS.UnixNano())), &got); err != nil {
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
	bck.restore()
	if _, err := s.ReadDataFile(filepath.Join("pending", fmt.Sprintf("%d", bck.TS.UnixNano())), &got); !errors.Is(err, os.ErrNotExist) {
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
	s := NewStorage(dir, encryptionKey())

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
	if _, err := s.ReadDataFile(filepath.Join("pending", fmt.Sprintf("%d", bck.TS.UnixNano())), &got); err != nil {
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
	if _, err := s.ReadDataFile(filepath.Join("pending", fmt.Sprintf("%d", bck.TS.UnixNano())), &got); !errors.Is(err, os.ErrNotExist) {
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
	ed := encryptionKey()
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
		file := filepath.Join("data", fmt.Sprintf("file%d", i))
		if err := os.WriteFile(filepath.Join(dir, file), []byte("XXXXXX"), 0600); err != nil {
			t.Fatalf("os.WriteFile: %v", err)
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
