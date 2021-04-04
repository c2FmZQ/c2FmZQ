package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"golang.org/x/crypto/pbkdf2"
	"kringle-server/log"
)

const (
	// The size of an encrypted key.
	EncryptedKeySize = 96
)

type MasterKey struct {
	EncryptionKey
}

type EncryptionKey struct {
	key           []byte
	encrypted_key []byte
}

// CreateMasterKey creates a new master key.
func CreateMasterKey() (*MasterKey, error) {
	ek := EncryptionKey{
		key: make([]byte, 32),
	}
	if _, err := io.ReadFull(rand.Reader, ek.key); err != nil {
		return nil, err
	}
	return &MasterKey{ek}, nil
}

// ReadMasterKey reads an encrypted master key from file and decrypts it.
func ReadMasterKey(passphrase, file string) (*MasterKey, error) {
	b, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	version, b := b[0], b[1:]
	if version != 1 {
		log.Fatalf("unexpected master key version %d", version)
	}
	salt, b := b[:16], b[16:]
	numIter, b := int(binary.LittleEndian.Uint32(b[:4])), b[4:]
	dk := pbkdf2.Key([]byte(passphrase), salt, numIter, 32, sha256.New)
	block, err := aes.NewCipher(dk)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := b[:gcm.NonceSize()]
	encMasterKey := b[gcm.NonceSize():]
	masterKey, err := gcm.Open(nil, nonce, encMasterKey, nil)
	if err != nil {
		return nil, err
	}
	return &MasterKey{EncryptionKey: EncryptionKey{key: masterKey}}, nil
}

// Save encrypts the key with passphrase and saves it to file.
func (mk MasterKey) Save(passphrase, file string) error {
	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return err
	}
	numIter := 200000
	if passphrase == "" {
		numIter = 10
	}
	numIterBin := make([]byte, 4)
	binary.LittleEndian.PutUint32(numIterBin, uint32(numIter))
	dk := pbkdf2.Key([]byte(passphrase), salt, numIter, 32, sha256.New)
	block, err := aes.NewCipher(dk)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return err
	}
	encMasterKey := gcm.Seal(nonce, nonce, mk.key, nil)
	data := []byte{1} // version
	data = append(data, salt...)
	data = append(data, numIterBin...)
	data = append(data, encMasterKey...)
	dir, _ := filepath.Split(file)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if err := ioutil.WriteFile(file, data, 0600); err != nil {
		return err
	}
	return nil
}

// Hash returns a hash of b.
func (k EncryptionKey) Hash(b []byte) []byte {
	mac := hmac.New(sha256.New, k.key)
	mac.Write(b)
	return mac.Sum(nil)
}

// Decrypt decrypts data that was encrypted with Encrypt and the same key.
func (k EncryptionKey) Decrypt(data []byte) ([]byte, error) {
	if len(k.key) == 0 {
		log.Fatal("key is not set")
	}
	iv := data[:aes.BlockSize]
	encData := data[aes.BlockSize : len(data)-32]
	hm := data[len(data)-32:]
	if !hmac.Equal(hm, k.Hash(encData)) {
		return nil, errors.New("invalid hmac")
	}
	block, err := aes.NewCipher(k.key)
	if err != nil {
		return nil, fmt.Errorf("aes.NewCipher failed: %w", err)
	}
	mode := cipher.NewCBCDecrypter(block, iv)
	dec := make([]byte, len(data)-aes.BlockSize-32)
	mode.CryptBlocks(dec, encData)
	padSize := int(dec[0])
	return dec[1 : len(dec)-padSize], nil
}

// Encrypt encrypts data using the key.
func (k EncryptionKey) Encrypt(data []byte) ([]byte, error) {
	if len(k.key) == 0 {
		log.Fatal("key is not set")
	}
	block, err := aes.NewCipher(k.key)
	if err != nil {
		return nil, fmt.Errorf("aes.NewCipher failed: %w", err)
	}
	iv := make([]byte, aes.BlockSize)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, err
	}
	padSize := 16 - (len(data)+1)%16
	pData := make([]byte, len(data)+padSize+1)
	pData[0] = byte(padSize)
	copy(pData[1:], data)
	if _, err := io.ReadFull(rand.Reader, pData[len(data)+1:]); err != nil {
		return nil, fmt.Errorf("padding data: %w", err)
	}

	mode := cipher.NewCBCEncrypter(block, iv)
	encData := make([]byte, len(pData))
	mode.CryptBlocks(encData, pData)
	hmac := k.Hash(encData)

	out := make([]byte, len(iv)+len(encData)+len(hmac))
	copy(out, iv)
	copy(out[len(iv):], encData)
	copy(out[len(iv)+len(encData):], hmac)
	return out, nil
}

// NewEncryptionKey creates a new AES-256 key.
func (k EncryptionKey) NewEncryptionKey() (*EncryptionKey, error) {
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("creating key: %w", err)
	}
	enc, err := k.Encrypt(key)
	if err != nil {
		return nil, err
	}
	return &EncryptionKey{key: key, encrypted_key: enc}, nil
}

// DecryptKey decrypts an encrypted key.
func (k EncryptionKey) DecryptKey(b []byte) (*EncryptionKey, error) {
	if len(b) != EncryptedKeySize {
		return nil, fmt.Errorf("invalid encrypted key size: %d", len(b))
	}
	key, err := k.Decrypt(b)
	if err != nil {
		return nil, err
	}
	if len(key) != 32 {
		return nil, errors.New("invalid key")
	}
	encrypted_key := make([]byte, len(b))
	copy(encrypted_key, b)
	return &EncryptionKey{key: key, encrypted_key: encrypted_key}, nil
}

// StartReader opens a reader to decrypt data.
func (k EncryptionKey) StartReader(r io.Reader) (*cipher.StreamReader, error) {
	block, err := aes.NewCipher(k.key)
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

// StartWriter opens a writer to encrypt data.
func (k EncryptionKey) StartWriter(w io.Writer) (*cipher.StreamWriter, error) {
	block, err := aes.NewCipher(k.key)
	if err != nil {
		return nil, fmt.Errorf("aes.NewCipher: %w", err)
	}
	iv := make([]byte, block.BlockSize())
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, fmt.Errorf("creating file iv: %w", err)
	}
	if _, err := w.Write(iv); err != nil {
		return nil, fmt.Errorf("writing file iv: %w", err)
	}
	return &cipher.StreamWriter{
		S: cipher.NewCTR(block, iv),
		W: w,
	}, nil
}

// ReadEncryptedKey reads an encrypted key and decrypts it.
func (k EncryptionKey) ReadEncryptedKey(r io.Reader) (*EncryptionKey, error) {
	key := make([]byte, EncryptedKeySize)
	if _, err := io.ReadFull(r, key); err != nil {
		return nil, fmt.Errorf("reading enc file key: %w", err)
	}
	return k.DecryptKey(key)
}

// WriteEncryptedKey writes the encrypted key to the writer.
func (k EncryptionKey) WriteEncryptedKey(w io.Writer) error {
	_, err := w.Write(k.encrypted_key)
	return err
}
