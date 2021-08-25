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
	"io/fs"
	"os"
	"path/filepath"
	"runtime"

	"c2FmZQ/internal/log"
	"golang.org/x/crypto/pbkdf2"
)

const (
	// The size of an encrypted key.
	aesEncryptedKeySize = 129 // 1 (version) + 16 (iv) + 64 (key) + 16 (pad) + 32 (mac)

	// The size of encrypted chunks in streams.
	aesFileChunkSize = 1 << 20
)

// AESKey is an encryption key that can be used to encrypt and decrypt
// data and streams.
type AESKey struct {
	maskedKey    []byte
	encryptedKey []byte
	xor          func([]byte) []byte
}

// Wipe zeros the key material.
func (k *AESKey) Wipe() {
	for i := range k.maskedKey {
		k.maskedKey[i] = 0
	}
	runtime.SetFinalizer(k, nil)
}

func (k *AESKey) setFinalizer() {
	stack := log.Stack()
	runtime.SetFinalizer(k, func(obj interface{}) {
		key := obj.(*AESKey)
		for i := range key.maskedKey {
			if key.maskedKey[i] != 0 {
				if log.Level >= log.DebugLevel {
					log.Panicf("WIPEME: AESKey not wiped. Call stack: %s", stack)
				}
				log.Errorf("WIPEME: AESKey not wiped. Call stack: %s", stack)
				key.Wipe()
				return
			}
		}
	})
}

type AESMasterKey struct {
	*AESKey
}

// CreateAESMasterKey creates a new master key.
func CreateAESMasterKey() (MasterKey, error) {
	b := make([]byte, 64)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return &AESMasterKey{aesKeyFromBytes(b)}, nil
}

// CreateAESMasterKeyForTest creates a new master key to tests.
func CreateAESMasterKeyForTest() (MasterKey, error) {
	b := make([]byte, 64)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	mk := &AESMasterKey{aesKeyFromBytes(b)}
	runtime.SetFinalizer(mk.AESKey, nil)
	return mk, nil
}

// ReadAESMasterKey reads an encrypted master key from file and decrypts it.
func ReadAESMasterKey(passphrase []byte, file string) (MasterKey, error) {
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
	dk := pbkdf2.Key(passphrase, salt, numIter, 32, sha256.New)
	block, err := aes.NewCipher(dk)
	if err != nil {
		log.Debug(err)
		return nil, ErrDecryptFailed
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		log.Debug(err)
		return nil, ErrDecryptFailed
	}
	nonce := b[:gcm.NonceSize()]
	encMasterKey := b[gcm.NonceSize():]
	mkBytes, err := gcm.Open(nil, nonce, encMasterKey, nil)
	if err != nil {
		log.Debug(err)
		return nil, ErrDecryptFailed
	}
	return &AESMasterKey{AESKey: aesKeyFromBytes(mkBytes)}, nil
}

// Save encrypts the key with passphrase and saves it to file.
func (mk AESMasterKey) Save(passphrase []byte, file string) error {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return err
	}
	numIter := 200000
	if len(passphrase) == 0 {
		numIter = 10
	}
	numIterBin := make([]byte, 4)
	binary.BigEndian.PutUint32(numIterBin, uint32(numIter))
	dk := pbkdf2.Key(passphrase, salt, numIter, 32, sha256.New)
	block, err := aes.NewCipher(dk)
	if err != nil {
		log.Debug(err)
		return ErrEncryptFailed
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		log.Debug(err)
		return ErrEncryptFailed
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		log.Debug(err)
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
	if err := os.WriteFile(file, data, 0600); err != nil {
		return err
	}
	return nil
}

func (k AESKey) key() []byte {
	return k.xor(k.maskedKey)
}

// Hash returns the HMAC-SHA256 hash of b.
func (k AESKey) Hash(b []byte) []byte {
	mac := hmac.New(sha256.New, k.key()[32:])
	mac.Write(b)
	return mac.Sum(nil)
}

// Decrypt decrypts data that was encrypted with Encrypt and the same key.
func (k AESKey) Decrypt(data []byte) ([]byte, error) {
	if len(k.maskedKey) == 0 {
		log.Fatal("key is not set")
	}
	if (len(data)-1)%aes.BlockSize != 0 || len(data)-1 < aes.BlockSize+32 {
		return nil, ErrDecryptFailed
	}
	version, data := data[0], data[1:]
	if version != 1 {
		return nil, ErrDecryptFailed
	}
	iv, data := data[:aes.BlockSize], data[aes.BlockSize:]
	encData, data := data[:len(data)-32], data[len(data)-32:]
	hm := data[:32]
	if !hmac.Equal(hm, k.Hash(encData)) {
		return nil, ErrDecryptFailed
	}
	block, err := aes.NewCipher(k.key()[:32])
	if err != nil {
		return nil, ErrDecryptFailed
	}
	mode := cipher.NewCBCDecrypter(block, iv)
	dec := make([]byte, len(encData))
	mode.CryptBlocks(dec, encData)
	padSize := int(dec[len(dec)-1])
	if padSize > len(encData) || padSize > aes.BlockSize {
		return nil, ErrDecryptFailed
	}
	for i := 0; i < padSize; i++ {
		if dec[len(dec)-i-1] != byte(padSize) {
			return nil, ErrDecryptFailed
		}
	}
	return dec[:len(dec)-padSize], nil
}

// Encrypt encrypts data using the key.
func (k AESKey) Encrypt(data []byte) ([]byte, error) {
	if len(k.maskedKey) == 0 {
		log.Fatal("key is not set")
	}
	block, err := aes.NewCipher(k.key()[:32])
	if err != nil {
		return nil, ErrEncryptFailed
	}
	iv := make([]byte, aes.BlockSize)
	if _, err := rand.Read(iv); err != nil {
		return nil, ErrEncryptFailed
	}
	padSize := aes.BlockSize - len(data)%aes.BlockSize
	pData := make([]byte, len(data)+padSize)
	copy(pData, data)
	for i := 0; i < padSize; i++ {
		pData[len(data)+i] = byte(padSize)
	}

	mode := cipher.NewCBCEncrypter(block, iv)
	encData := make([]byte, len(pData))
	mode.CryptBlocks(encData, pData)
	for i := range pData {
		pData[i] = 0
	}
	hmac := k.Hash(encData)

	out := make([]byte, 1+len(iv)+len(encData)+len(hmac))
	out[0] = 1 // version
	copy(out[1:], iv)
	copy(out[1+len(iv):], encData)
	copy(out[1+len(iv)+len(encData):], hmac)
	return out, nil
}

// aesKeyFromBytes returns an AESKey with the raw bytes provided.
// Internally, the key is masked with a ephemeral key in memory.
func aesKeyFromBytes(b []byte) *AESKey {
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
	ek := &AESKey{maskedKey: xor(b), xor: xor}
	for i := range b {
		b[i] = 0
	}
	ek.setFinalizer()
	return ek
}

// NewKey creates a new encryption key.
func (k AESKey) NewKey() (EncryptionKey, error) {
	b := make([]byte, 64)
	if _, err := rand.Read(b); err != nil {
		log.Debug(err)
		return nil, ErrEncryptFailed
	}
	enc, err := k.Encrypt(b)
	if err != nil {
		return nil, err
	}
	ek := aesKeyFromBytes(b)
	ek.encryptedKey = enc
	return ek, nil
}

// DecryptKey decrypts an encrypted key.
func (k AESKey) DecryptKey(encryptedKey []byte) (EncryptionKey, error) {
	if len(encryptedKey) != aesEncryptedKeySize {
		log.Debugf("DecryptKey: unexpected encrypted key size %d != %d", len(encryptedKey), aesEncryptedKeySize)
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
	ek := aesKeyFromBytes(b)
	ek.encryptedKey = make([]byte, len(encryptedKey))
	copy(ek.encryptedKey, encryptedKey)
	return ek, nil
}

// AESStreamReader decrypts an input stream.
type AESStreamReader struct {
	gcm   cipher.AEAD
	r     io.Reader
	ctx   []byte
	start int64
	off   int64
	buf   []byte
}

func gcmNonce(ctx []byte, counter int64) []byte {
	var n [12]byte
	copy(n[:4], ctx)
	binary.BigEndian.PutUint64(n[4:], uint64(counter))
	return n[:]
}

// Seek moves the next read to a new offset. The offset is in the decrypted
// stream.
func (r *AESStreamReader) Seek(offset int64, whence int) (int64, error) {
	var newOffset int64
	switch whence {
	case io.SeekStart:
		newOffset = offset
	case io.SeekCurrent:
		newOffset = r.off + offset
	case io.SeekEnd:
		seeker, ok := r.r.(io.Seeker)
		if !ok {
			return 0, errors.New("input is not seekable")
		}
		size, err := seeker.Seek(0, io.SeekEnd)
		if err != nil {
			return 0, err
		}
		nChunks := (size - r.start) / int64(aesFileChunkSize+r.gcm.Overhead())
		lastChunkSize := (size - r.start) % int64(aesFileChunkSize+r.gcm.Overhead())
		if lastChunkSize > 0 {
			lastChunkSize -= int64(r.gcm.Overhead())
		}
		if lastChunkSize < 0 {
			return 0, errors.New("invalid last chunk")
		}
		decSize := nChunks*int64(aesFileChunkSize) + lastChunkSize
		newOffset = decSize + offset
	default:
		return 0, fmt.Errorf("invalid whence: %d", whence)
	}
	if newOffset < 0 {
		return 0, fs.ErrInvalid
	}
	if newOffset == r.off {
		return r.off, nil
	}
	seeker, ok := r.r.(io.Seeker)
	if !ok {
		return 0, errors.New("input is not seekable")
	}
	// Move to new offset.
	r.off = newOffset
	chunkOffset := r.off % int64(aesFileChunkSize)
	seekTo := r.start + r.off/int64(aesFileChunkSize)*int64(aesFileChunkSize+r.gcm.Overhead())
	if _, err := seeker.Seek(seekTo, io.SeekStart); err != nil {
		return 0, err
	}
	r.buf = nil
	if err := r.readChunk(); err != nil && err != io.EOF {
		return 0, err
	}
	if chunkOffset < int64(len(r.buf)) {
		r.buf = r.buf[chunkOffset:]
	} else {
		r.buf = nil
	}
	return r.off, nil
}

func (r *AESStreamReader) readChunk() error {
	in := make([]byte, aesFileChunkSize+r.gcm.Overhead())
	n, err := io.ReadFull(r.r, in)
	if n > 0 {
		nonce := gcmNonce(r.ctx, r.off/int64(aesFileChunkSize)+1)
		if n <= r.gcm.Overhead() {
			log.Debugf("StreamReader.Read: short chunk %d", n)
			return ErrDecryptFailed
		}
		dec, err := r.gcm.Open(nil, nonce, in[:n], nil)
		if err != nil {
			log.Debug(err)
			return ErrDecryptFailed
		}
		r.buf = append(r.buf, dec...)
	}
	if err == io.ErrUnexpectedEOF {
		err = io.EOF
	}
	if len(r.buf) > 0 && err == io.EOF {
		err = nil
	}
	return err
}

func (r *AESStreamReader) Read(b []byte) (n int, err error) {
	for err == nil {
		nn := copy(b[n:], r.buf)
		r.buf = r.buf[nn:]
		r.off += int64(nn)
		n += nn
		if n == len(b) {
			break
		}
		err = r.readChunk()
	}
	if n > 0 {
		return n, nil
	}
	return n, err
}

func (r *AESStreamReader) Close() error {
	if c, ok := r.r.(io.Closer); ok {
		if err := c.Close(); err != nil {
			return err
		}
	}
	return nil
}

// StartReader opens a reader to decrypt a stream of data.
func (k AESKey) StartReader(ctx []byte, r io.Reader) (StreamReader, error) {
	var start int64
	if seeker, ok := r.(io.Seeker); ok {
		off, err := seeker.Seek(0, io.SeekCurrent)
		if err != nil {
			panic(err)
		}
		start = off
	}

	block, err := aes.NewCipher(k.key()[:32])
	if err != nil {
		log.Debug(err)
		return nil, ErrDecryptFailed
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		log.Debug(err)
		return nil, ErrDecryptFailed
	}
	return &AESStreamReader{gcm: gcm, r: r, ctx: ctx, start: start}, nil
}

// AESStreamWriter encrypts a stream of data.
type AESStreamWriter struct {
	gcm cipher.AEAD
	w   io.Writer
	ctx []byte
	c   int64
	buf []byte
}

func (w *AESStreamWriter) writeChunk(b []byte) (int, error) {
	w.c++
	nonce := gcmNonce(w.ctx, w.c)
	out := w.gcm.Seal(nil, nonce, b, nil)
	for i := range b {
		b[i] = 0
	}
	return w.w.Write(out)
}

func (w *AESStreamWriter) Write(b []byte) (n int, err error) {
	w.buf = append(w.buf, b...)
	n = len(b)
	for len(w.buf) >= aesFileChunkSize {
		_, err = w.writeChunk(w.buf[:aesFileChunkSize])
		w.buf = w.buf[aesFileChunkSize:]
		if err != nil {
			break
		}
	}
	return
}

func (w *AESStreamWriter) Close() (err error) {
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
func (k AESKey) StartWriter(ctx []byte, w io.Writer) (StreamWriter, error) {
	block, err := aes.NewCipher(k.key()[:32])
	if err != nil {
		log.Debug(err)
		return nil, ErrEncryptFailed
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		log.Debug(err)
		return nil, ErrEncryptFailed
	}
	return &AESStreamWriter{gcm: gcm, w: w, ctx: ctx}, nil
}

// ReadEncryptedKey reads an encrypted key and decrypts it.
func (k AESKey) ReadEncryptedKey(r io.Reader) (EncryptionKey, error) {
	buf := make([]byte, aesEncryptedKeySize)
	if _, err := io.ReadFull(r, buf); err != nil {
		log.Debug(err)
		return nil, ErrDecryptFailed
	}
	return k.DecryptKey(buf)
}

// WriteEncryptedKey writes the encrypted key to the writer.
func (k AESKey) WriteEncryptedKey(w io.Writer) error {
	n, err := w.Write(k.encryptedKey)
	if n != aesEncryptedKeySize {
		log.Debugf("WriteEncryptedKey: unexpected key size: %d != %d", n, aesEncryptedKeySize)
		return ErrEncryptFailed
	}
	return err
}
