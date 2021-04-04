package aes

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
	key []byte
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
	return &MasterKey{EncryptionKey{masterKey}}, nil
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

// Decrypt decrypts data that was encrypted with Encrypt and the same master key.
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

// Encrypt encrypts data using the master key.
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

// NewEncryptedKey creates a new encrypted AES-256 key. The size of the
// encrypted key is EncryptedKeySize.
func (k EncryptionKey) NewEncryptedKey() ([]byte, error) {
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("creating key: %w", err)
	}
	return k.Encrypt(key)
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
	return &EncryptionKey{key: key}, nil
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

// DecryptKeyAndStartReader combines DecryptKey and StartReader.
func (k EncryptionKey) DecryptKeyAndStartReader(key []byte, r io.Reader) (*cipher.StreamReader, error) {
	fileKey, err := k.DecryptKey(key)
	if err != nil {
		return nil, fmt.Errorf("decrypting file key: %w", err)
	}
	return fileKey.StartReader(r)
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

// DecryptKeyAndStartWriter combines DecryptKey and StartWriter.
func (k EncryptionKey) DecryptKeyAndStartWriter(key []byte, w io.Writer) (*cipher.StreamWriter, error) {
	fileKey, err := k.DecryptKey(key)
	if err != nil {
		return nil, fmt.Errorf("decrypting file key: %w", err)
	}
	return fileKey.StartWriter(w)
}
