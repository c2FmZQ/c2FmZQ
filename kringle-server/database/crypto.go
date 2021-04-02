package database

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"golang.org/x/crypto/pbkdf2"
	"kringle-server/log"
	"kringle-server/md"
)

const (
	masterKeyFile = "master.key"
)

func (d Database) readMasterKey(passphrase string) ([]byte, error) {
	b, err := os.ReadFile(filepath.Join(d.Dir(), masterKeyFile))
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
	return masterKey, nil
}

func (d Database) createMasterKey(passphrase string) ([]byte, error) {
	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, err
	}
	numIter := 100000
	if passphrase == "" {
		numIter = 10
	}
	numIterBin := make([]byte, 4)
	binary.LittleEndian.PutUint32(numIterBin, uint32(numIter))
	dk := pbkdf2.Key([]byte(passphrase), salt, numIter, 32, sha256.New)
	block, err := aes.NewCipher(dk)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	masterKey := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, masterKey); err != nil {
		return nil, err
	}
	encMasterKey := gcm.Seal(nonce, nonce, masterKey, nil)
	if err := os.MkdirAll(d.Dir(), 0700); err != nil {
		return nil, err
	}
	data := []byte{1} // version
	data = append(data, salt...)
	data = append(data, numIterBin...)
	data = append(data, encMasterKey...)
	if err := ioutil.WriteFile(filepath.Join(d.Dir(), masterKeyFile), data, 0600); err != nil {
		return nil, err
	}
	return masterKey, nil
}

func (d Database) masterHash(in []byte) []byte {
	var b []byte
	b = append(b, d.masterKey...)
	b = append(b, in...)
	out := sha256.Sum256(b)
	return out[:]
}

func (d *Database) decryptWithMasterKey(header md.Header) (sk md.SecretKey, err error) {
	if len(d.masterKey) == 0 {
		log.Fatal("master key is not set")
	}
	if len(header) != 32+aes.BlockSize {
		return sk, fmt.Errorf("wrong header size: %d", len(header))
	}
	block, err := aes.NewCipher(d.masterKey)
	if err != nil {
		return sk, fmt.Errorf("aes.NewCipher failed: %w", err)
	}
	mode := cipher.NewCBCDecrypter(block, header[:aes.BlockSize])
	mode.CryptBlocks(sk[:], header[aes.BlockSize:])
	return
}
