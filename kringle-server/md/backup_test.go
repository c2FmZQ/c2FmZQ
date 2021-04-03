package md

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
	md := New(dir, encrypterDecrypter())

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
	bck, err := md.createBackup(files)
	if err != nil {
		t.Fatalf("md.createBackup: %v", err)
	}

	var got backup
	if _, err := md.ReadDataFile(filepath.Join("pending", fmt.Sprintf("%d", bck.TS.UnixNano())), &got); err != nil {
		t.Fatalf("md.ReadDataFile: %v", err)
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
	if _, err := md.ReadDataFile(filepath.Join("pending", fmt.Sprintf("%d", bck.TS.UnixNano())), &got); !errors.Is(err, os.ErrNotExist) {
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
	md := New(dir, encrypterDecrypter())

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
	bck, err := md.createBackup(files)
	if err != nil {
		t.Fatalf("md.createBackup: %v", err)
	}

	var got backup
	if _, err := md.ReadDataFile(filepath.Join("pending", fmt.Sprintf("%d", bck.TS.UnixNano())), &got); err != nil {
		t.Fatalf("md.ReadDataFile: %v", err)
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
	if _, err := md.ReadDataFile(filepath.Join("pending", fmt.Sprintf("%d", bck.TS.UnixNano())), &got); !errors.Is(err, os.ErrNotExist) {
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
	ed := encrypterDecrypter()
	md := New(dir, ed)

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
	if _, err := md.createBackup(files); err != nil {
		t.Fatalf("md.createBackup: %v", err)
	}
	for i := 1; i <= 10; i++ {
		file := filepath.Join("data", fmt.Sprintf("file%d", i))
		if err := os.WriteFile(filepath.Join(dir, file), []byte("XXXXXX"), 0600); err != nil {
			t.Fatalf("os.WriteFile: %v", err)
		}
		files = append(files, file)
	}

	// New will notice the aborted operation and roll it back.
	md = New(dir, ed)

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
