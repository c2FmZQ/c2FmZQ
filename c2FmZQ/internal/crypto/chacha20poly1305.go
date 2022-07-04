//
// Copyright 2021-2022 TTBT Enterprises LLC
//
// This file is part of c2FmZQ (https://c2FmZQ.org/).
//
// c2FmZQ is free software: you can redistribute it and/or modify it under the
// terms of the GNU General Public License as published by the Free Software
// Foundation, either version 3 of the License, or (at your option) any later
// version.
//
// c2FmZQ is distributed in the hope that it will be useful, but WITHOUT ANY
// WARRANTY; without even the implied warranty of MERCHANTABILITY or FITNESS FOR
// A PARTICULAR PURPOSE. See the GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License along with
// c2FmZQ. If not, see <https://www.gnu.org/licenses/>.

package crypto

import (
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
	"time"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/chacha20poly1305"

	"c2FmZQ/internal/log"
)

const (
	// The size of an encrypted key.
	chachaEncryptedKeySize = 105 // 1 (version) + 24 (nonce) + 64 (key) + 16 (tag)

	// The size of encrypted chunks in streams.
	chachaFileChunkSize = 1 << 20
)

// Chacha20Poly1305Key is an encryption key that can be used to encrypt and
// decrypt data and streams.
type Chacha20Poly1305Key struct {
	maskedKey    []byte
	encryptedKey []byte
	xor          func([]byte) []byte
}

// Wipe zeros the key material.
func (k *Chacha20Poly1305Key) Wipe() {
	for i := range k.maskedKey {
		k.maskedKey[i] = 0
	}
	runtime.SetFinalizer(k, nil)
}

func (k *Chacha20Poly1305Key) setFinalizer() {
	stack := log.Stack()
	runtime.SetFinalizer(k, func(obj interface{}) {
		key := obj.(*Chacha20Poly1305Key)
		for i := range key.maskedKey {
			if key.maskedKey[i] != 0 {
				if log.Level >= log.DebugLevel {
					log.Panicf("WIPEME: Chacha20Poly1305Key not wiped. Call stack: %s", stack)
				}
				log.Errorf("WIPEME: Chacha20Poly1305Key not wiped. Call stack: %s", stack)
				key.Wipe()
				return
			}
		}
	})
}

type Chacha20Poly1305MasterKey struct {
	*Chacha20Poly1305Key
}

// CreateChacha20Poly1305MasterKey creates a new master key.
func CreateChacha20Poly1305MasterKey() (MasterKey, error) {
	b := make([]byte, 64)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return &Chacha20Poly1305MasterKey{chacha20poly1305KeyFromBytes(b)}, nil
}

// CreateChacha20Poly1305MasterKeyForTest creates a new master key to tests.
func CreateChacha20Poly1305MasterKeyForTest() (MasterKey, error) {
	mk, err := CreateChacha20Poly1305MasterKey()
	if err != nil {
		return nil, err
	}
	runtime.SetFinalizer(mk.(*Chacha20Poly1305MasterKey).Chacha20Poly1305Key, nil)
	return mk, nil
}

// ReadChacha20Poly1305MasterKey reads an encrypted master key from file and decrypts it.
func ReadChacha20Poly1305MasterKey(passphrase []byte, file string) (MasterKey, error) {
	b, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	version, b := b[0], b[1:]
	if version != 2 {
		log.Debugf("ReadMasterKey: unexpected version: %d", version)
		return nil, ErrDecryptFailed
	}
	salt, b := b[:16], b[16:]
	time, b := uint32(b[0]), b[1:]
	memory, b := binary.LittleEndian.Uint32(b[:4]), b[4:]
	dk := argon2.IDKey(passphrase, salt, time, memory, 1, 32)
	ccp, err := chacha20poly1305.NewX(dk)
	if err != nil {
		log.Debug(err)
		return nil, ErrEncryptFailed
	}
	nonce := b[:ccp.NonceSize()]
	encMasterKey := b[ccp.NonceSize():]
	mkBytes, err := ccp.Open(nil, nonce, encMasterKey, nil)
	if err != nil {
		log.Debug(err)
		return nil, ErrDecryptFailed
	}
	return &Chacha20Poly1305MasterKey{Chacha20Poly1305Key: chacha20poly1305KeyFromBytes(mkBytes)}, nil
}

// Save encrypts the key with passphrase and saves it to file.
func (mk Chacha20Poly1305MasterKey) Save(passphrase []byte, file string) error {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return err
	}
	time := uint32(2)
	memory := uint32(128 * 1024)
	dk := argon2.IDKey(passphrase, salt, time, memory, 1, 32)
	ccp, err := chacha20poly1305.NewX(dk)
	if err != nil {
		log.Debug(err)
		return ErrEncryptFailed
	}

	nonce := make([]byte, ccp.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		log.Debug(err)
		return ErrEncryptFailed
	}
	encMasterKey := ccp.Seal(nonce, nonce, mk.key(), nil)
	memoryb := make([]byte, 4)
	binary.LittleEndian.PutUint32(memoryb, memory)
	data := []byte{2} // version
	data = append(data, salt...)
	data = append(data, byte(time))
	data = append(data, memoryb...)
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

func (k Chacha20Poly1305Key) key() []byte {
	return k.xor(k.maskedKey)
}

// Hash returns the HMAC-SHA256 hash of b.
func (k Chacha20Poly1305Key) Hash(b []byte) []byte {
	mac := hmac.New(sha256.New, k.key()[32:])
	mac.Write(b)
	return mac.Sum(nil)
}

// Decrypt decrypts data that was encrypted with Encrypt and the same key.
func (k Chacha20Poly1305Key) Decrypt(data []byte) ([]byte, error) {
	if len(k.maskedKey) == 0 {
		log.Fatal("key is not set")
	}
	version, data := data[0], data[1:]
	if version != 2 {
		return nil, ErrDecryptFailed
	}
	ccp, err := chacha20poly1305.NewX(k.key()[:32])
	if err != nil {
		log.Debug(err)
		return nil, ErrEncryptFailed
	}
	nonce := data[:ccp.NonceSize()]
	b, err := ccp.Open(nil, nonce, data[ccp.NonceSize():], nil)
	if err != nil {
		log.Debug(err)
		return nil, ErrDecryptFailed
	}
	return b, nil
}

// Encrypt encrypts data using the key.
func (k Chacha20Poly1305Key) Encrypt(data []byte) ([]byte, error) {
	if len(k.maskedKey) == 0 {
		log.Fatal("key is not set")
	}
	ccp, err := chacha20poly1305.NewX(k.key()[:32])
	if err != nil {
		log.Debug(err)
		return nil, ErrEncryptFailed
	}
	out := make([]byte, 1+ccp.NonceSize(), 1+ccp.NonceSize()+len(data)+ccp.Overhead())
	out[0] = 2 // version
	// nonce cannot be repeated with the same key. Use a combination of
	// current time and random number.
	binary.LittleEndian.PutUint64(out[1:9], uint64(time.Now().UnixNano()))
	if _, err := rand.Read(out[9 : 1+ccp.NonceSize()]); err != nil {
		log.Debug(err)
		return nil, ErrEncryptFailed
	}
	return ccp.Seal(out, out[1:1+ccp.NonceSize()], data, nil), nil
}

// chacha20poly1305KeyFromBytes returns an Chacha20Poly1305Key with the raw
// bytes provided. Internally, the key is masked with a ephemeral key in memory.
func chacha20poly1305KeyFromBytes(b []byte) *Chacha20Poly1305Key {
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
	ek := &Chacha20Poly1305Key{maskedKey: xor(b), xor: xor}
	for i := range b {
		b[i] = 0
	}
	ek.setFinalizer()
	return ek
}

// NewKey creates a new encryption key.
func (k Chacha20Poly1305Key) NewKey() (EncryptionKey, error) {
	b := make([]byte, 64)
	if _, err := rand.Read(b); err != nil {
		log.Debug(err)
		return nil, ErrEncryptFailed
	}
	enc, err := k.Encrypt(b)
	if err != nil {
		return nil, err
	}
	ek := chacha20poly1305KeyFromBytes(b)
	ek.encryptedKey = enc
	return ek, nil
}

// DecryptKey decrypts an encrypted key.
func (k Chacha20Poly1305Key) DecryptKey(encryptedKey []byte) (EncryptionKey, error) {
	if len(encryptedKey) != chachaEncryptedKeySize {
		log.Debugf("DecryptKey: unexpected encrypted key size %d != %d", len(encryptedKey), chachaEncryptedKeySize)
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
	ek := chacha20poly1305KeyFromBytes(b)
	ek.encryptedKey = make([]byte, len(encryptedKey))
	copy(ek.encryptedKey, encryptedKey)
	return ek, nil
}

func chachaNonce(ctx []byte, counter int64) []byte {
	var n [24]byte
	copy(n[:16], ctx)
	binary.LittleEndian.PutUint64(n[16:], uint64(counter))
	return n[:]
}

// Chacha20Poly1305StreamReader decrypts an input stream.
type Chacha20Poly1305StreamReader struct {
	ccp   cipher.AEAD
	k     Chacha20Poly1305Key
	r     io.Reader
	ctx   []byte
	start int64
	off   int64
	buf   []byte
}

// Seek moves the next read to a new offset. The offset is in the decrypted
// stream.
func (r *Chacha20Poly1305StreamReader) Seek(offset int64, whence int) (int64, error) {
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
		nChunks := (size - r.start) / int64(chachaFileChunkSize+r.ccp.Overhead())
		lastChunkSize := (size - r.start) % int64(chachaFileChunkSize+r.ccp.Overhead())
		if lastChunkSize > 0 {
			lastChunkSize -= int64(r.ccp.Overhead())
		}
		if lastChunkSize < 0 {
			return 0, errors.New("invalid last chunk")
		}
		decSize := nChunks*int64(chachaFileChunkSize) + lastChunkSize
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
	// Move to new offset. Fast path if we already have enough data in the
	// buffer.
	if d := newOffset - r.off; d > 0 && d < int64(len(r.buf)) {
		r.buf = r.buf[int(d):]
		r.off = newOffset
		return r.off, nil
	}

	// Move to new offset. Slow path. Seek to new position and read a new
	// chunk.
	seeker, ok := r.r.(io.Seeker)
	if !ok {
		return 0, errors.New("input is not seekable")
	}
	r.off = newOffset
	chunkOffset := r.off % int64(chachaFileChunkSize)
	seekTo := r.start + r.off/int64(chachaFileChunkSize)*int64(chachaFileChunkSize+r.ccp.Overhead())
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

func (r *Chacha20Poly1305StreamReader) readChunk() error {
	in := make([]byte, chachaFileChunkSize+r.ccp.Overhead())
	n, err := io.ReadFull(r.r, in)
	if n > 0 {
		dec, err := r.ccp.Open(in[:0], chachaNonce(r.ctx, r.off/int64(chachaFileChunkSize)+1), in[:n], nil)
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

func (r *Chacha20Poly1305StreamReader) Read(b []byte) (n int, err error) {
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

func (r *Chacha20Poly1305StreamReader) Close() error {
	if c, ok := r.r.(io.Closer); ok {
		if err := c.Close(); err != nil {
			return err
		}
	}
	return nil
}

// StartReader opens a reader to decrypt a stream of data.
func (k Chacha20Poly1305Key) StartReader(ctx []byte, r io.Reader) (StreamReader, error) {
	var start int64
	if seeker, ok := r.(io.Seeker); ok {
		off, err := seeker.Seek(0, io.SeekCurrent)
		if err != nil {
			panic(err)
		}
		start = off
	}

	ccp, err := chacha20poly1305.NewX(k.key()[:32])
	if err != nil {
		return nil, err
	}
	return &Chacha20Poly1305StreamReader{ccp: ccp, r: r, ctx: ctx, start: start}, nil
}

// Chacha20Poly1305StreamWriter encrypts a stream of data.
type Chacha20Poly1305StreamWriter struct {
	ccp cipher.AEAD
	w   io.Writer
	ctx []byte
	c   int64
	buf []byte
}

func (w *Chacha20Poly1305StreamWriter) writeChunk(b []byte) (int, error) {
	w.c++
	enc := w.ccp.Seal(nil, chachaNonce(w.ctx, w.c), b, nil)
	for i := 0; i < len(b); i++ {
		b[i] = 0
	}
	return w.w.Write(enc)
}

func (w *Chacha20Poly1305StreamWriter) Write(b []byte) (n int, err error) {
	w.buf = append(w.buf, b...)
	n = len(b)
	for len(w.buf) >= chachaFileChunkSize {
		_, err = w.writeChunk(w.buf[:chachaFileChunkSize])
		w.buf = w.buf[chachaFileChunkSize:]
		if err != nil {
			break
		}
	}
	return
}

func (w *Chacha20Poly1305StreamWriter) Close() (err error) {
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
func (k Chacha20Poly1305Key) StartWriter(ctx []byte, w io.Writer) (StreamWriter, error) {
	ccp, err := chacha20poly1305.NewX(k.key()[:32])
	if err != nil {
		return nil, err
	}
	return &Chacha20Poly1305StreamWriter{ccp: ccp, w: w, ctx: ctx}, nil
}

// ReadEncryptedKey reads an encrypted key and decrypts it.
func (k Chacha20Poly1305Key) ReadEncryptedKey(r io.Reader) (EncryptionKey, error) {
	buf := make([]byte, chachaEncryptedKeySize)
	if _, err := io.ReadFull(r, buf); err != nil {
		log.Debugf("ReadEncryptedKey: %v", err)
		return nil, ErrDecryptFailed
	}
	return k.DecryptKey(buf)
}

// WriteEncryptedKey writes the encrypted key to the writer.
func (k Chacha20Poly1305Key) WriteEncryptedKey(w io.Writer) error {
	n, err := w.Write(k.encryptedKey)
	if n != chachaEncryptedKeySize {
		log.Debugf("WriteEncryptedKey: unexpected key size: %d != %d", n, chachaEncryptedKeySize)
		return ErrEncryptFailed
	}
	return err
}
