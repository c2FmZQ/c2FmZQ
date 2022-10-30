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

//go:build !sodium
// +build !sodium

package stingle

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"runtime"

	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/nacl/box"
	"golang.org/x/crypto/nacl/secretbox"

	"c2FmZQ/internal/log"
)

// MakeSecretKey returns a new SecretKey.
func MakeSecretKey() *SecretKey {
	sk := &SecretKey{B: new([32]byte)}
	if _, err := rand.Read(sk.B[:]); err != nil {
		panic(err)
	}
	sk.setFinalizer()
	return sk
}

// MakeSecretKey returns a new SecretKey for tests.
func MakeSecretKeyForTest() *SecretKey {
	sk := MakeSecretKey()
	runtime.SetFinalizer(sk, nil)
	return sk
}

// SecretKeyFromBytes returns a SecretKey from raw bytes.
func SecretKeyFromBytes(b []byte) *SecretKey {
	sk := &SecretKey{B: new([32]byte)}
	copy(sk.B[:], b)
	for i := 0; i < len(b); i++ {
		b[i] = 0
	}
	sk.setFinalizer()
	return sk
}

// A secret key for asymmetric key encryption.
type SecretKey struct {
	B *[32]byte
}

// Wipe zeros the secret key.
func (sk *SecretKey) Wipe() {
	if sk == nil {
		return
	}
	for i := range *sk.B {
		(*sk.B)[i] = 0
	}
	if log.Level > log.DebugLevel {
		log.Debugf("Wiped %#v", *sk)
	}
	runtime.SetFinalizer(sk, nil)
}

func (k *SecretKey) setFinalizer() {
	stack := log.Stack()
	runtime.SetFinalizer(k, func(obj interface{}) {
		sk := obj.(*SecretKey)
		for i := range *sk.B {
			if (*sk.B)[i] != 0 {
				if log.Level >= log.DebugLevel {
					log.Panicf("WIPEME: SecretKey not wiped. Call stack: %s", stack)
				}
				log.Errorf("WIPEME: SecretKey not wiped. Call stack: %s", stack)
				sk.Wipe()
				return
			}
		}
	})
}

// ToBytes returns the raw bytes of the secret key.
func (sk SecretKey) ToBytes() []byte {
	return sk.B[:]
}

// PublicKey returns the public key associated with this secret key.
func (sk SecretKey) PublicKey() (pk PublicKey) {
	curve25519.ScalarBaseMult(&pk.B, sk.B)
	return
}

// nacl returns the public key in nacl format.
func (pk PublicKey) nacl() *[32]byte {
	return &pk.B
}

// EncryptMessage encrypts a message using Authenticated Public Key Encryption.
func EncryptMessage(msg []byte, pk PublicKey, sk *SecretKey) string {
	nonce := new([24]byte)
	if _, err := rand.Read(nonce[:]); err != nil {
		panic(err)
	}
	out := make([]byte, len(nonce))
	copy(out, nonce[:])
	ret := box.Seal(out, msg, nonce, pk.nacl(), sk.B)
	return base64.StdEncoding.EncodeToString(ret)
}

// DecryptMessage decrypts a message using Authenticated Public Key Encryption.
func DecryptMessage(msg string, pk PublicKey, sk *SecretKey) ([]byte, error) {
	b, err := base64.StdEncoding.DecodeString(msg)
	if err != nil {
		return nil, err
	}
	if len(b) < 24 {
		return nil, errors.New("message is too short")
	}
	nonce := new([24]byte)
	copy((*nonce)[:], b[:24])

	ret, ok := box.Open(nil, b[len(nonce):], nonce, pk.nacl(), sk.B)
	if !ok {
		return nil, errors.New("box.Open failed")
	}
	return ret, nil
}

// SealBoxBase64 encrypts a message using Anonymous Public Key Encryption.
func (pk PublicKey) SealBoxBase64(msg []byte) string {
	return base64.StdEncoding.EncodeToString(pk.SealBox(msg))
}

// SealBoxOpenBase64 decrypts a message encrypted by SealBoxBase64.
func (sk SecretKey) SealBoxOpenBase64(msg string) ([]byte, error) {
	b, err := base64.StdEncoding.DecodeString(msg)
	if err != nil {
		return nil, err
	}
	return sk.SealBoxOpen(b)
}

// SealBox encrypts a message using Anonymous Public Key Encryption.
func (pk PublicKey) SealBox(msg []byte) []byte {
	ret, err := box.SealAnonymous(nil, msg, pk.nacl(), rand.Reader)
	if err != nil {
		panic(err)
	}
	return ret
}

// SealBoxOpen decrypts a message encrypted by SealBox.
func (sk SecretKey) SealBoxOpen(msg []byte) ([]byte, error) {
	ret, ok := box.OpenAnonymous(nil, msg, sk.PublicKey().nacl(), sk.B)
	if !ok {
		return nil, errors.New("box.OpenAnonymous failed")
	}
	return ret, nil
}

// EncryptSymmetric encrypts a message using symmetric key encryption.
func EncryptSymmetric(msg, nonce, key []byte) []byte {
	if len(nonce) != 24 || len(key) != 32 {
		panic("invalid arguments")
	}
	n := new([24]byte)
	copy((*n)[:], nonce)
	k := new([32]byte)
	copy((*k)[:], key)
	out := []byte{}
	return secretbox.Seal(out, msg, n, k)
}

// DecryptSymmetric decrypts a message using symmetric key encryption.
func DecryptSymmetric(box, nonce, key []byte) ([]byte, error) {
	if len(nonce) != 24 || len(key) != 32 {
		panic("invalid arguments")
	}
	n := new([24]byte)
	copy((*n)[:], nonce)
	k := new([32]byte)
	copy((*k)[:], key)
	out := []byte{}
	ret, ok := secretbox.Open(out, box, n, k)
	if !ok {
		return nil, errors.New("secretbox.Open failed")
	}
	return ret, nil
}
