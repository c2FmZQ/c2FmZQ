// Package md stores arbitrary metadata in encrypted files.
package md

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"kringle-server/log"
)

// New returns a new Metadata rooted at dir. The caller must provide a
// HeaderDecrypter that will be used to decrypt each file header and return
// the SecretKey.
func New(dir string, keyFromHeader HeaderDecrypter) *Metadata {
	md := &Metadata{
		dir:           dir,
		keyFromHeader: keyFromHeader,
	}
	if err := md.recoverPendingOps(); err != nil {
		log.Fatalf("md.recoverPendingOps: %v", err)
	}
	return md
}

// Offers the API to atomically read, write, and update encrypted files.
type Metadata struct {
	dir           string
	keyFromHeader HeaderDecrypter
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