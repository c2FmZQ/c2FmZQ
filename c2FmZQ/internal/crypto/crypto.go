// Package crypto implements a few abstractions around the go crypto packages
// to manage encryption keys, encrypt small data, and streams.
package crypto

import (
	"errors"
	"io"
)

const (
	AES int = iota // AES256-GCM and AES256-CBC+HMAC-SHA256.
)

var (
	// Indicates that the ciphertext could not be decrypted.
	ErrDecryptFailed = errors.New("decryption failed")
	// Indicates that the plaintext could not be encrypted.
	ErrEncryptFailed = errors.New("encryption failed")
	// Indicates an invalid alg value.
	ErrUnexpectedAlgo = errors.New("unexpected algorithm")
)

// MasterKey is an encryption key that is normally stored on disk encrypted with
// a passphrase. It is used to create file keys used to encrypt the content of
// files.
type MasterKey interface {
	EncryptionKey

	// Save encrypted the MasterKey with passphrase and saves it to file.
	Save(passphrase []byte, file string) error
}

// CreateMasterKey creates a new master key.
func CreateMasterKey(alg int) (MasterKey, error) {
	switch alg {
	case AES:
		return CreateAESMasterKey()
	default:
		return nil, ErrUnexpectedAlgo
	}
}

// ReadMasterKey reads an encrypted master key from file and decrypts it.
func ReadMasterKey(alg int, passphrase []byte, file string) (MasterKey, error) {
	switch alg {
	case AES:
		return ReadAESMasterKey(passphrase, file)
	default:
		return nil, ErrUnexpectedAlgo
	}
}

// EncryptionKey is an encryption key that can be used to encrypt and decrypt
// data and streams.
type EncryptionKey interface {
	// Encrypt encrypts data using the key.
	Encrypt(data []byte) ([]byte, error)
	// Decrypt decrypts data that was encrypted with Encrypt and the same key.
	Decrypt(data []byte) ([]byte, error)
	// Hash returns a cryptographially secure hash of b.
	Hash(b []byte) []byte
	// StartReader opens a reader to decrypt a stream of data.
	StartReader(ctx uint32, r io.Reader) (StreamReader, error)
	// StartWriter opens a writer to encrypt a stream of data.
	StartWriter(ctx uint32, w io.Writer) (StreamWriter, error)
	// NewKey creates a new encryption key.
	NewKey() (EncryptionKey, error)
	// DecryptKey decrypts an encrypted key.
	DecryptKey(encryptedKey []byte) (EncryptionKey, error)
	// ReadEncryptedKey reads an encrypted key and decrypts it.
	ReadEncryptedKey(r io.Reader) (EncryptionKey, error)
	// WriteEncryptedKey writes the encrypted key to the writer.
	WriteEncryptedKey(w io.Writer) error
	// Wipe zeros the key material.
	Wipe()
}

// StreamReader decrypts a stream.
type StreamReader interface {
	Read(b []byte) (n int, err error)
	Close() error
}

// StreamWriter encrypts a stream.
type StreamWriter interface {
	Write(b []byte) (n int, err error)
	Close() error
}
