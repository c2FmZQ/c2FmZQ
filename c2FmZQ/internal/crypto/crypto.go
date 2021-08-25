// Package crypto implements a few abstractions around the go crypto packages
// to manage encryption keys, encrypt small data, and streams.
package crypto

import (
	"errors"
	"io"
	"os"
)

const (
	AES256           int = iota // AES256-GCM, AES256-CBC+HMAC-SHA256, PBKDF2.
	Chacha20Poly1305            // Chacha20Poly1305, Argon2.

	DefaultAlgo = AES256
	PickFastest = -1
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
	if alg == PickFastest {
		var err error
		if alg, err = Fastest(); err != nil {
			alg = DefaultAlgo
		}
	}
	switch alg {
	case AES256:
		return CreateAESMasterKey()
	case Chacha20Poly1305:
		return CreateChacha20Poly1305MasterKey()
	default:
		return nil, ErrUnexpectedAlgo
	}
}

// ReadMasterKey reads an encrypted master key from file and decrypts it.
func ReadMasterKey(passphrase []byte, file string) (MasterKey, error) {
	b, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	switch b[0] {
	case 1: // AES256
		return ReadAESMasterKey(passphrase, file)
	case 2: // Chacha20Poly1305
		return ReadChacha20Poly1305MasterKey(passphrase, file)
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
	StartReader(ctx []byte, r io.Reader) (StreamReader, error)
	// StartWriter opens a writer to encrypt a stream of data.
	StartWriter(ctx []byte, w io.Writer) (StreamWriter, error)
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
	io.Reader
	io.Seeker
	io.Closer
}

// StreamWriter encrypts a stream.
type StreamWriter interface {
	io.Writer
	io.Closer
}
