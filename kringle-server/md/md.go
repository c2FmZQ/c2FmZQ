// Package md stores arbitrary metadata in encrypted files.
package md

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"kringle-server/crypto"
	"kringle-server/log"
)

// New returns a new Metadata rooted at dir. The caller must provide an
// EncryptionKey that will be used to encrypt and decrypt per-file encryption
// keys.
func New(dir string, masterKey crypto.EncryptionKey) *Metadata {
	md := &Metadata{
		dir:       dir,
		masterKey: masterKey,
	}
	if err := md.rollbackPendingOps(); err != nil {
		log.Fatalf("md.rollbackPendingOps: %v", err)
	}
	return md
}

// Offers the API to atomically read, write, and update encrypted files.
type Metadata struct {
	dir       string
	masterKey crypto.EncryptionKey
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
