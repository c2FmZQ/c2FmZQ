package client

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"c2FmZQ/internal/crypto"
	"c2FmZQ/internal/secure"
)

func TestFindFilesToImport(t *testing.T) {
	c, err := newClient(t.TempDir())
	if err != nil {
		t.Fatalf("newClient: %v", err)
	}
	dest := "dest"

	testDir := t.TempDir()
	for _, f := range []string{
		"file1",
		"file2",
		"dirA/file3",
		"dirA/file4",
		"dirA/dirB/file5",
	} {
		fn := filepath.Join(testDir, f)
		dir, _ := filepath.Split(fn)
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(fn, []byte(dest), 0600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}

	want := []toImport{
		{src: testDir + "/dirA/dirB/file5", dst: "dest/dirA/dirB/file5"},
		{src: testDir + "/dirA/file3", dst: "dest/dirA/file3"},
		{src: testDir + "/dirA/file4", dst: "dest/dirA/file4"},
		{src: testDir + "/file1", dst: "dest/file1"},
		{src: testDir + "/file2", dst: "dest/file2"},
	}

	got, err := c.findFilesToImport([]string{filepath.Join(testDir, "*")}, dest, true)
	if err != nil {
		t.Fatalf("c.findFilesToImport('*'): %v", err)
	}
	if !reflect.DeepEqual(want, got) {
		t.Errorf("Found unexpected files. Want %v, got %v", want, got)
	}
}

func newClient(dir string) (*Client, error) {
	masterKey, err := crypto.CreateMasterKey()
	if err != nil {
		return nil, err
	}
	storage := secure.NewStorage(dir, masterKey.EncryptionKey)
	c, err := Create(masterKey, storage)
	if err != nil {
		return nil, err
	}
	return c, nil
}
