package md

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"

	"kringle-server/masterkey"
)

// Encapsulates the data and logic to encrypt and decrypt a file.
type Crypter struct {
	ed         EncrypterDecrypter
	encFileKey []byte
}

func newCrypter(ed EncrypterDecrypter) *Crypter {
	return &Crypter{ed: ed}
}

func (c *Crypter) beginRead(r io.Reader) (*cipher.StreamReader, error) {
	c.encFileKey = make([]byte, masterkey.EncryptedKeySize)
	if _, err := io.ReadFull(r, c.encFileKey); err != nil {
		return nil, fmt.Errorf("reading encFileKey: %w", err)
	}
	key, err := c.ed.Decrypt(c.encFileKey)
	if err != nil {
		return nil, fmt.Errorf("decrypting file key: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("aes.NewCipher: %w", err)
	}
	iv := make([]byte, block.BlockSize())
	if _, err := io.ReadFull(r, iv); err != nil {
		return nil, fmt.Errorf("reading file iv: %w", err)
	}
	return &cipher.StreamReader{
		S: cipher.NewCTR(block, iv),
		R: r,
	}, nil
}

func (c *Crypter) beginWrite(w io.Writer) (*cipher.StreamWriter, error) {
	if len(c.encFileKey) == 0 {
		encKey, err := c.ed.NewEncryptedKey()
		if err != nil {
			return nil, err
		}
		c.encFileKey = encKey
	}
	key, err := c.ed.Decrypt(c.encFileKey)
	if err != nil {
		return nil, fmt.Errorf("decrypting file key: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("aes.NewCipher: %w", err)
	}
	iv := make([]byte, block.BlockSize())
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, fmt.Errorf("creating file iv: %w", err)
	}
	if _, err := w.Write(c.encFileKey); err != nil {
		return nil, fmt.Errorf("writing file encFileKey: %w", err)
	}
	if _, err := w.Write(iv); err != nil {
		return nil, fmt.Errorf("writing file iv: %w", err)
	}
	return &cipher.StreamWriter{
		S: cipher.NewCTR(block, iv),
		W: w,
	}, nil
}
