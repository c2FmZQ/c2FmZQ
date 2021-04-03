package masterkey

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

type MasterKey struct {
	key []byte
}

// Create creates a new master key.
func Create() (*MasterKey, error) {
	mk := &MasterKey{
		key: make([]byte, 32),
	}
	if _, err := io.ReadFull(rand.Reader, mk.key); err != nil {
		return nil, err
	}
	return mk, nil
}

// Read reads an encrypted master key from file and decrypts it.
func Read(passphrase, file string) (*MasterKey, error) {
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
	return &MasterKey{key: masterKey}, nil
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
func (mk MasterKey) Hash(b []byte) []byte {
	mac := hmac.New(sha256.New, mk.key)
	mac.Write(b)
	return mac.Sum(nil)
}

// Decrypt decrypts data using the master key. The data must begin with the
// 16-byte iv and 32-byte hmac, and it must be a multiple of 16 bytes in total size.
func (mk MasterKey) Decrypt(data []byte) ([]byte, error) {
	if len(mk.key) == 0 {
		log.Fatal("master key is not set")
	}
	if len(data)%16 != 0 || len(data) < aes.BlockSize+32 {
		return nil, fmt.Errorf("invalid data size: %d", len(data))
	}
	iv := data[:aes.BlockSize]
	hm := data[aes.BlockSize : aes.BlockSize+32]
	encData := data[aes.BlockSize+32:]
	if !hmac.Equal(hm, mk.Hash(encData)) {
		return nil, errors.New("invalid hmac")
	}
	block, err := aes.NewCipher(mk.key)
	if err != nil {
		return nil, fmt.Errorf("aes.NewCipher failed: %w", err)
	}
	mode := cipher.NewCBCDecrypter(block, iv)
	out := make([]byte, len(data)-aes.BlockSize-32)
	mode.CryptBlocks(out, encData)
	return out, nil
}

// Encrypt encrypts data using the master key. The data must be a multiple
// of 16 bytes in size. The encrypted output includes the 16-byte iv, and hmac
// at the beginning.
func (mk MasterKey) Encrypt(data []byte) ([]byte, error) {
	if len(mk.key) == 0 {
		log.Fatal("master key is not set")
	}
	if len(data)%16 != 0 {
		return nil, fmt.Errorf("invalid data size: %d", len(data))
	}
	block, err := aes.NewCipher(mk.key)
	if err != nil {
		return nil, fmt.Errorf("aes.NewCipher failed: %w", err)
	}
	iv := make([]byte, aes.BlockSize)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, err
	}
	mode := cipher.NewCBCEncrypter(block, iv)
	encData := make([]byte, len(data))
	mode.CryptBlocks(encData, data)
	hmac := mk.Hash(encData)

	out := make([]byte, len(data)+aes.BlockSize+32)
	copy(out, iv)
	copy(out[aes.BlockSize:], hmac)
	copy(out[aes.BlockSize+32:], encData)
	return out, nil
}

// NewEncryptedKey creates a new encrypted AES-256 key.
func (mk MasterKey) NewEncryptedKey() ([]byte, error) {
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("creating key: %w", err)
	}
	return mk.Encrypt(key)
}
