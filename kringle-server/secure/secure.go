// Package secure stores arbitrary metadata in encrypted files.
package secure

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"kringle-server/crypto"
	"kringle-server/log"
)

// NewStorage returns a new Storage rooted at dir. The caller must provide an
// EncryptionKey that will be used to encrypt and decrypt per-file encryption
// keys.
func NewStorage(dir string, masterKey crypto.EncryptionKey) *Storage {
	s := &Storage{
		dir:       dir,
		masterKey: masterKey,
	}
	if err := s.rollbackPendingOps(); err != nil {
		log.Fatalf("s.rollbackPendingOps: %v", err)
	}
	return s
}

// Offers the API to atomically read, write, and update encrypted files.
type Storage struct {
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
