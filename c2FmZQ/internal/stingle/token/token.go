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

// Package token provides a mechanism for securely authenticating future
// requests from a user.
//
// key := MakeKey()
//
// // tok is valid for 1 hour and assigned to Subject 44545.
// encryptedToken := Mint(key, Token{Subject: 44545, Scope: "scope"}, time.Hour)
//
// // Subject can be used to find the right key for the subject.
// Subject(encryptedToken) == 44545
//
// // Check returns err=nil iff encryptedToken is valid.
// tok, err := Check(key, encryptedToken)
//
package token

import (
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"runtime"
	"time"

	"golang.org/x/crypto/chacha20poly1305"

	"c2FmZQ/internal/log"
)

var (
	ErrValidationFailed = errors.New("token validation failed")
)

// A secret key used to encrypt tokens.
type Key [chacha20poly1305.KeySize]byte

func KeyFromBytes(b []byte) *Key {
	if len(b) != chacha20poly1305.KeySize {
		panic("invalid token key size")
	}
	var k Key
	copy(k[:], b)
	for i := 0; i < chacha20poly1305.KeySize; i++ {
		b[i] = 0
	}
	k.setFinalizer()
	return &k
}

func (k *Key) Wipe() {
	if k == nil {
		return
	}
	for i := range k {
		k[i] = 0
	}
	runtime.SetFinalizer(k, nil)
}

func (k *Key) setFinalizer() {
	stack := log.Stack()
	runtime.SetFinalizer(k, func(obj interface{}) {
		tk := obj.(*Key)
		for i := range tk {
			if tk[i] != 0 {
				if log.Level >= log.DebugLevel {
					log.Panicf("WIPEME: TokenKey not wiped. Call stack: %s", stack)
				}
				log.Errorf("WIPEME: TokenKey not wiped. Call stack: %s", stack)
				tk.Wipe()
				return
			}
		}
	})
}

func (k *Key) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return err
	}
	if len(b) != chacha20poly1305.KeySize {
		return errors.New("invalid key size")
	}
	copy((*k)[:], b)
	k.setFinalizer()
	return nil
}

func (k Key) MarshalJSON() ([]byte, error) {
	return json.Marshal(base64.RawURLEncoding.EncodeToString(k[:]))
}

// Holds the information contained in the encrypted token.
type Token struct {
	// Who this token was issued to.
	Subject int64 `json:"sub"`
	// The reason/purpose of the token.
	Scope string `json:"scope"`
	// When the token was issued.
	IssuedAt int64 `json:"iat"`
	// When the token exipres.
	Expiration int64 `json:"exp"`
	// The file this token gives access to.
	File string `json:"file,omitempty"`
	// The set in which the file is.
	Set string `json:"set,omitempty"`
	// Whether the access is granted for the thumbnail.
	Thumb bool `json:"thumb,omitempty"`
}

// MakeKey returns a new encryption key.
func MakeKey() *Key {
	var key Key
	if _, err := rand.Read(key[:]); err != nil {
		panic(err)
	}
	return &key
}

// Mint returns an encrypted token.
func Mint(key *Key, tok Token, exp time.Duration) string {
	tok.IssuedAt = time.Now().Unix()
	tok.Expiration = time.Now().Add(exp).Unix()
	ser, _ := json.Marshal(tok)

	cc, err := chacha20poly1305.New(key[:])
	if err != nil {
		panic(err)
	}

	enc := make([]byte, 8)
	binary.BigEndian.PutUint64(enc, uint64(tok.Subject))

	nonce := make([]byte, cc.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		panic(err)
	}
	enc = append(enc, nonce...)
	enc = cc.Seal(enc, nonce, ser, enc[:8])

	return base64.RawURLEncoding.EncodeToString(enc)
}

// Subject returns the plaintext Subject ID from an encrypted token.
func Subject(t string) (int64, error) {
	enc, err := base64.RawURLEncoding.DecodeString(t)
	if err != nil {
		return 0, ErrValidationFailed
	}
	if len(enc) <= 8+chacha20poly1305.NonceSize {
		return 0, ErrValidationFailed
	}
	return int64(binary.BigEndian.Uint64(enc[:8])), nil
}

// Decrypt returns a decrypted and validated token.
func Decrypt(key *Key, t string) (Token, error) {
	enc, err := base64.RawURLEncoding.DecodeString(t)
	if err != nil {
		return Token{}, ErrValidationFailed
	}
	if len(enc) <= 8+chacha20poly1305.NonceSize {
		return Token{}, ErrValidationFailed
	}
	cc, err := chacha20poly1305.New(key[:])
	if err != nil {
		return Token{}, ErrValidationFailed
	}
	ser, err := cc.Open(nil, enc[8:8+cc.NonceSize()], enc[8+cc.NonceSize():], enc[:8])
	if err != nil {
		return Token{}, ErrValidationFailed
	}
	var tok Token
	if err := json.Unmarshal(ser, &tok); err != nil {
		return Token{}, ErrValidationFailed
	}
	if int64(binary.BigEndian.Uint64(enc[:8])) != tok.Subject {
		return Token{}, ErrValidationFailed
	}
	if now := time.Now().Unix(); tok.IssuedAt > now || tok.Expiration < now {
		return Token{}, ErrValidationFailed
	}
	return tok, nil
}

// Hash returns a token.
func Hash(token string) string {
	h := sha1.Sum([]byte(token))
	return base64.RawURLEncoding.EncodeToString(h[:])
}
