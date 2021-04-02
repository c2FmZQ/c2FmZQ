package md

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
)

// The header of a metadata file. It contains random bytes and an encrypted key.
type Header [32 + aes.BlockSize]byte

// An AES encryption key.
type SecretKey [32]byte

// A HeaderDecrypter decrypts the secret key from a Header.
type HeaderDecrypter func(Header) (SecretKey, error)

// Encapsulates the data and logic to encrypt and decrypt a file.
type Crypter struct {
	decryptKey HeaderDecrypter
	header     []byte
}

func newCrypter(f HeaderDecrypter) *Crypter {
	return &Crypter{decryptKey: f}
}

func (c *Crypter) beginRead(r io.Reader) (*cipher.StreamReader, error) {
	c.header = make([]byte, 32+aes.BlockSize)
	if _, err := io.ReadFull(r, c.header); err != nil {
		return nil, fmt.Errorf("reading header: %w", err)
	}
	var hdr Header
	copy(hdr[:], c.header)
	key, err := c.decryptKey(hdr)
	if err != nil {
		return nil, fmt.Errorf("decrypting file key: %w", err)
	}
	block, err := aes.NewCipher(key[:])
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
		c.header = make([]byte, 32+aes.BlockSize)
		if _, err := io.ReadFull(rand.Reader, c.header); err != nil {
			return nil, fmt.Errorf("creating header: %w", err)
		}
	}
	var hdr Header
	copy(hdr[:], c.header)
	key, err := c.decryptKey(hdr)
	if err != nil {
		return nil, fmt.Errorf("decrypting file key: %w", err)
	}
	block, err := aes.NewCipher(key[:])
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
