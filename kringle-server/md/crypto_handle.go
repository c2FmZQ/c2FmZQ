package md

import (
	"fmt"
	"io"

	"kringle-server/crypto/aes"
)

// Encapsulates the data and logic to encrypt and decrypt a file.
type CryptoHandle struct {
	ek         aes.EncryptionKey
	encFileKey []byte
}

func newCryptoHandle(ek aes.EncryptionKey) *CryptoHandle {
	return &CryptoHandle{ek: ek}
}

func (c *CryptoHandle) beginRead(r io.Reader) (io.Reader, error) {
	c.encFileKey = make([]byte, aes.EncryptedKeySize)
	if _, err := io.ReadFull(r, c.encFileKey); err != nil {
		return nil, fmt.Errorf("reading enc file key: %w", err)
	}
	return c.ek.DecryptKeyAndStartReader(c.encFileKey, r)
}

func (c *CryptoHandle) beginWrite(w io.Writer) (io.WriteCloser, error) {
	if len(c.encFileKey) == 0 {
		encKey, err := c.ek.NewEncryptedKey()
		if err != nil {
			return nil, err
		}
		c.encFileKey = encKey
	}
	if _, err := w.Write(c.encFileKey); err != nil {
		return nil, fmt.Errorf("writing enc file key: %w", err)
	}
	return c.ek.DecryptKeyAndStartWriter(c.encFileKey, w)
}
