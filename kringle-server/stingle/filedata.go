package stingle

import (
	"crypto/rand"
	"io"

	"golang.org/x/crypto/chacha20poly1305"
)

const (
	poly1305Overhead = 16
	context          = "__data__"
)

// EncryptFile encrypts the plaintext from the reader using the SymmetricKey in
// header, and writes the ciphertext to the writer.
func EncryptFile(r io.Reader, w io.Writer, header Header) error {
	buf := make([]byte, header.ChunkSize)

	for c := uint64(1); ; c++ {
		ck := DeriveKey(header.SymmetricKey, chacha20poly1305.KeySize, c, context)

		n, err := io.ReadFull(r, buf)
		if n > 0 {
			nonce := make([]byte, chacha20poly1305.NonceSizeX)
			if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
				return err
			}
			ae, err := chacha20poly1305.NewX(ck)
			if err != nil {
				return err
			}
			enc := ae.Seal(nonce, nonce, buf[:n], nil)
			if _, err := w.Write(enc); err != nil {
				return err
			}
		}
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// DecryptFile decrypts the ciphertext from the reader using the SymmetricKey
// in header, and write the plaintext to the writer.
func DecryptFile(r io.Reader, w io.Writer, header Header) error {
	buf := make([]byte, chacha20poly1305.NonceSizeX+header.ChunkSize+poly1305Overhead)

	for c := uint64(1); ; c++ {
		ck := DeriveKey(header.SymmetricKey, chacha20poly1305.KeySize, c, context)

		n, err := io.ReadFull(r, buf)
		if n > chacha20poly1305.NonceSizeX {
			nonce := buf[:chacha20poly1305.NonceSizeX]
			enc := buf[chacha20poly1305.NonceSizeX:n]

			ae, err := chacha20poly1305.NewX(ck)
			if err != nil {
				return err
			}
			dec, err := ae.Open(enc[:0], nonce, enc, nil)
			if err != nil {
				return err
			}
			if _, err := w.Write(dec); err != nil {
				return err
			}
		}
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
		if err != nil {
			return err
		}
	}
	return nil
}
