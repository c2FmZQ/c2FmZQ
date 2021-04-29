// Package crypto implements a few abstractions around the go crypto packages
// to manage encryption keys, encrypt small data, and streams.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"golang.org/x/crypto/pbkdf2"
	"kringle/internal/log"
)

const (
	// The size of an encrypted key.
	encryptedKeySize = 93 // 1 (version) + 12 (nonce) + 64 (key) + 16 (mac)

	// The size of encrypted chunks in streams.
	streamChunkSize = 16 * 1024
)

var (
	ErrDecryptFailed = errors.New("decryption failed")
	ErrEncryptFailed = errors.New("encryption failed")
)

// MasterKey is an encryption key that is normally stored on disk encrypted with
// a passphrase. It is used to create file keys used to encrypt the content of
// files.
type MasterKey struct {
	EncryptionKey
}

// EncryptionKey is an encryption key that can be used to encrypt and decrypt
// data and streams.
type EncryptionKey struct {
	maskedKey    []byte
	encryptedKey []byte
	xor          func([]byte) []byte
}

// CreateMasterKey creates a new master key.
func CreateMasterKey() (*MasterKey, error) {
	b := make([]byte, 64)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return &MasterKey{encryptionKeyFromBytes(b)}, nil
}

// ReadMasterKey reads an encrypted master key from file and decrypts it.
func ReadMasterKey(passphrase, file string) (*MasterKey, error) {
	b, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	version, b := b[0], b[1:]
	if version != 1 {
		log.Debugf("ReadMasterKey: unexpected version: %d", version)
		return nil, ErrDecryptFailed
	}
	salt, b := b[:16], b[16:]
	numIter, b := int(binary.BigEndian.Uint32(b[:4])), b[4:]
	dk := pbkdf2.Key([]byte(passphrase), salt, numIter, 32, sha256.New)
	block, err := aes.NewCipher(dk)
	if err != nil {
		log.Debugf("aes.NewCipher: %v", err)
		return nil, ErrDecryptFailed
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		log.Debugf("cipher.NewGCM: %v", err)
		return nil, ErrDecryptFailed
	}
	nonce := b[:gcm.NonceSize()]
	encMasterKey := b[gcm.NonceSize():]
	mkBytes, err := gcm.Open(nil, nonce, encMasterKey, nil)
	if err != nil {
		log.Debugf("gcm.Open: %v", err)
		return nil, ErrDecryptFailed
	}
	return &MasterKey{EncryptionKey: encryptionKeyFromBytes(mkBytes)}, nil
}

// Save encrypts the key with passphrase and saves it to file.
func (mk MasterKey) Save(passphrase, file string) error {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return err
	}
	numIter := 200000
	if passphrase == "" {
		numIter = 10
	}
	numIterBin := make([]byte, 4)
	binary.BigEndian.PutUint32(numIterBin, uint32(numIter))
	dk := pbkdf2.Key([]byte(passphrase), salt, numIter, 32, sha256.New)
	block, err := aes.NewCipher(dk)
	if err != nil {
		log.Debugf("aes.NewCipher: %v", err)
		return ErrEncryptFailed
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		log.Debugf("cipher.NewGCM: %v", err)
		return ErrEncryptFailed
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		log.Debugf("io.ReadFull: %v", err)
		return ErrEncryptFailed
	}
	encMasterKey := gcm.Seal(nonce, nonce, mk.key(), nil)
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

func (k EncryptionKey) key() []byte {
	return k.xor(k.maskedKey)
}

// Hash returns the HMAC-SHA256 hash of b.
func (k EncryptionKey) Hash(b []byte) []byte {
	mac := hmac.New(sha256.New, k.key()[32:])
	mac.Write(b)
	return mac.Sum(nil)
}

// Decrypt decrypts data that was encrypted with Encrypt and the same key.
func (k EncryptionKey) Decrypt(data []byte) ([]byte, error) {
	if len(k.maskedKey) == 0 {
		log.Fatal("key is not set")
	}
	version, data := data[0], data[1:]
	if version != 1 {
		log.Debugf("Decrypt: unexpected version %d", version)
		return nil, ErrDecryptFailed
	}
	block, err := aes.NewCipher(k.key()[:32])
	if err != nil {
		log.Debugf("aes.NewCipher: %v", err)
		return nil, ErrDecryptFailed
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		log.Debugf("cipher.NewGCM: %v", err)
		return nil, ErrDecryptFailed
	}
	nonce := data[:gcm.NonceSize()]
	encData := data[gcm.NonceSize():]
	dec, err := gcm.Open(nil, nonce, encData, nil)
	if err != nil {
		log.Debugf("gcm.Open: %v", err)
		return nil, ErrDecryptFailed
	}
	return dec, nil
}

// Encrypt encrypts data using the key.
func (k EncryptionKey) Encrypt(data []byte) ([]byte, error) {
	if len(k.maskedKey) == 0 {
		log.Fatal("key is not set")
	}
	block, err := aes.NewCipher(k.key()[:32])
	if err != nil {
		log.Debugf("aes.NewCipher: %v", err)
		return nil, ErrEncryptFailed
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		log.Debugf("cipher.NewGCM: %v", err)
		return nil, ErrEncryptFailed
	}
	out := make([]byte, 1+gcm.NonceSize())
	out[0] = 1 // version
	if _, err := rand.Read(out[1:]); err != nil {
		log.Debugf("io.ReadFull: %v", err)
		return nil, ErrEncryptFailed
	}
	return gcm.Seal(out, out[1:1+gcm.NonceSize()], data, nil), nil
}

// encryptionKeyFromBytes returns an EncryptionKey with the raw bytes provided.
// Internally, the key is masked with a ephemeral key in memory.
func encryptionKeyFromBytes(b []byte) EncryptionKey {
	mask := make([]byte, len(b))
	if _, err := rand.Read(mask); err != nil {
		panic(err)
	}
	xor := func(in []byte) []byte {
		out := make([]byte, len(mask))
		for i := range mask {
			out[i] = in[i] ^ mask[i]
		}
		return out
	}
	ek := EncryptionKey{maskedKey: xor(b), xor: xor}
	for i := range b {
		b[i] = 0
	}
	return ek
}

// NewEncryptionKey creates a new encryption key.
func (k EncryptionKey) NewEncryptionKey() (*EncryptionKey, error) {
	b := make([]byte, 64)
	if _, err := rand.Read(b); err != nil {
		log.Debugf("io.ReadFull: %v", err)
		return nil, ErrEncryptFailed
	}
	enc, err := k.Encrypt(b)
	if err != nil {
		return nil, err
	}
	ek := encryptionKeyFromBytes(b)
	ek.encryptedKey = enc
	return &ek, nil
}

// DecryptKey decrypts an encrypted key.
func (k EncryptionKey) DecryptKey(encryptedKey []byte) (*EncryptionKey, error) {
	if len(encryptedKey) != encryptedKeySize {
		log.Debugf("DecryptKey: unexpected encrypted key size %d != %d", len(encryptedKey), encryptedKeySize)
		return nil, ErrDecryptFailed
	}
	b, err := k.Decrypt(encryptedKey)
	if err != nil {
		return nil, err
	}
	if len(b) != 64 {
		log.Debugf("DecryptKey: unexpected decrypted key size %d != %d", len(b), 64)
		return nil, ErrDecryptFailed
	}
	ek := encryptionKeyFromBytes(b)
	ek.encryptedKey = make([]byte, len(encryptedKey))
	copy(ek.encryptedKey, encryptedKey)
	return &ek, nil
}

// StreamReader decrypts an input stream.
type StreamReader struct {
	gcm cipher.AEAD
	r   io.Reader
	c   uint64
	buf []byte
}

func (r *StreamReader) Read(b []byte) (n int, err error) {
	for err == nil {
		nn := copy(b[n:], r.buf)
		r.buf = r.buf[nn:]
		n += nn
		if n == len(b) {
			break
		}
		in := make([]byte, 8+r.gcm.NonceSize()+streamChunkSize+r.gcm.Overhead())
		if nn, err = io.ReadFull(r.r, in); nn > 0 {
			r.c++
			if nn <= 36 {
				log.Debugf("StreamReader.Read: short chunk %d", nn)
				return n, ErrDecryptFailed
			}
			cc := binary.BigEndian.Uint64(in[:8])
			if r.c != cc {
				log.Debugf("StreamReader.Read: wrong chunk number %d", cc)
				return n, ErrDecryptFailed
			}
			dec, err := r.gcm.Open(nil, in[8:8+r.gcm.NonceSize()], in[8+r.gcm.NonceSize():nn], in[:8])
			if err != nil {
				log.Debugf("gcm.Open: %v", err)
				return n, ErrDecryptFailed
			}
			r.buf = append(r.buf, dec...)
		}
		if len(r.buf) > 0 && (err == io.EOF || err == io.ErrUnexpectedEOF) {
			err = nil
		}
	}
	if n > 0 {
		return n, nil
	}
	if err == io.ErrUnexpectedEOF {
		err = io.EOF
	}
	return n, err
}

func (r *StreamReader) Close() error {
	if c, ok := r.r.(io.Closer); ok {
		if err := c.Close(); err != nil {
			return err
		}
	}
	return nil
}

// StartReader opens a reader to decrypt a stream of data.
func (k EncryptionKey) StartReader(r io.Reader) (*StreamReader, error) {
	block, err := aes.NewCipher(k.key()[:32])
	if err != nil {
		log.Debugf("aes.NewCipher: %v", err)
		return nil, ErrDecryptFailed
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		log.Debugf("cipher.NewGCM: %v", err)
		return nil, ErrDecryptFailed
	}
	return &StreamReader{gcm: gcm, r: r}, nil
}

// StreamWriter encrypts a stream of data.
type StreamWriter struct {
	gcm cipher.AEAD
	w   io.Writer
	c   uint64
	buf []byte
}

func (w *StreamWriter) writeChunk(b []byte) (int, error) {
	w.c++
	out := make([]byte, 8+w.gcm.NonceSize())
	binary.BigEndian.PutUint64(out[:8], w.c)
	if _, err := rand.Read(out[8:]); err != nil {
		log.Debugf("io.ReadFull: %v", err)
		return 0, ErrEncryptFailed
	}
	out = w.gcm.Seal(out, out[8:8+w.gcm.NonceSize()], b, out[:8])
	return w.w.Write(out)
}

func (w *StreamWriter) Write(b []byte) (n int, err error) {
	w.buf = append(w.buf, b...)
	n = len(b)
	for len(w.buf) >= streamChunkSize {
		_, err = w.writeChunk(w.buf[:streamChunkSize])
		w.buf = w.buf[streamChunkSize:]
		if err != nil {
			break
		}
	}
	return
}

func (w *StreamWriter) Close() (err error) {
	if len(w.buf) > 0 {
		_, err = w.writeChunk(w.buf)
	}
	if c, ok := w.w.(io.Closer); ok {
		if e := c.Close(); err == nil {
			err = e
		}
	}
	return
}

// StartWriter opens a writer to encrypt a stream of data.
func (k EncryptionKey) StartWriter(w io.Writer) (*StreamWriter, error) {
	block, err := aes.NewCipher(k.key()[:32])
	if err != nil {
		log.Debugf("aes.NewCipher: %v", err)
		return nil, ErrEncryptFailed
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		log.Debugf("cipher.NewGCM: %v", err)
		return nil, ErrEncryptFailed
	}
	return &StreamWriter{gcm: gcm, w: w}, nil
}

// ReadEncryptedKey reads an encrypted key and decrypts it.
func (k EncryptionKey) ReadEncryptedKey(r io.Reader) (*EncryptionKey, error) {
	buf := make([]byte, encryptedKeySize)
	if _, err := io.ReadFull(r, buf); err != nil {
		log.Debugf("ReadEncryptedKey: %v", err)
		return nil, ErrDecryptFailed
	}
	return k.DecryptKey(buf)
}

// WriteEncryptedKey writes the encrypted key to the writer.
func (k EncryptionKey) WriteEncryptedKey(w io.Writer) error {
	n, err := w.Write(k.encryptedKey)
	if n != encryptedKeySize {
		log.Debugf("WriteEncryptedKey: unexpected key size: %d != %d", n, encryptedKeySize)
		return ErrEncryptFailed
	}
	return err
}
