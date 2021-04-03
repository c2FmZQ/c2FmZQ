package md

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
)

// Encapsulates the data and logic to encrypt and decrypt a file.
type Crypter struct {
	ed     EncrypterDecrypter
	header []byte
}

func newCrypter(ed EncrypterDecrypter) *Crypter {
	return &Crypter{ed: ed}
}

func (c *Crypter) beginRead(r io.Reader) (*cipher.StreamReader, error) {
	c.header = make([]byte, 64+aes.BlockSize)
	if _, err := io.ReadFull(r, c.header); err != nil {
		return nil, fmt.Errorf("reading header: %w", err)
	}
	key, err := c.ed.Decrypt(c.header)
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
	if len(c.header) == 0 {
		key, err := c.ed.NewEncryptedKey()
		if err != nil {
			return nil, err
		}
		c.header = key
	}
	key, err := c.ed.Decrypt(c.header)
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
	if _, err := w.Write(c.header); err != nil {
		return nil, fmt.Errorf("writing file header: %w", err)
	}
	if _, err := w.Write(iv); err != nil {
		return nil, fmt.Errorf("writing file iv: %w", err)
	}
	return &cipher.StreamWriter{
		S: cipher.NewCTR(block, iv),
		W: w,
	}, nil
}
