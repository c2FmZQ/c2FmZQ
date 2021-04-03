// Package md stores arbitrary metadata in encrypted files.
package md

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"kringle-server/log"
)

// Provides a way to create a new encrypted key, and to decrypt them.
type EncrypterDecrypter interface {
	NewEncryptedKey() ([]byte, error)
	Decrypt([]byte) ([]byte, error)
}

// New returns a new Metadata rooted at dir. The caller must provide a
// HeaderDecrypter that will be used to decrypt each file header and return
// the SecretKey.
func New(dir string, ed EncrypterDecrypter) *Metadata {
	md := &Metadata{
		dir: dir,
		ed:  ed,
	}
	if err := md.rollbackPendingOps(); err != nil {
		log.Fatalf("md.rollbackPendingOps: %v", err)
	}
	return md
}

// Offers the API to atomically read, write, and update encrypted files.
type Metadata struct {
	dir string
	ed  EncrypterDecrypter
}

func createParentIfNotExist(filename string) error {
	dir, _ := filepath.Split(filename)
	if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("os.MkdirAll(%q): %w", dir, err)
		}
	}
	return nil
}
