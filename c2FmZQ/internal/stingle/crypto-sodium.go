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

// +build sodium

package stingle

import (
	"encoding/base64"
	"errors"
	"runtime"

	"github.com/jamesruan/sodium"

	"c2FmZQ/internal/log"
)

// MakeSecretKey returns a new SecretKey.
func MakeSecretKey() *SecretKey {
	kp := sodium.MakeBoxKP()
	sk := SecretKey(kp.SecretKey)
	sk.setFinalizer()
	return &sk
}

// MakeSecretKeyForTest returns a new SecretKey for tests. It should only be
// called from a _test.go file.
func MakeSecretKeyForTest() *SecretKey {
	sk := MakeSecretKey()
	runtime.SetFinalizer(sk, nil)
	return sk
}

// A secret key for asymmetric key encryption.
type SecretKey sodium.BoxSecretKey

// SecretKeyFromBytes returns a SecretKey from raw bytes.
func SecretKeyFromBytes(b []byte) *SecretKey {
	c := make([]byte, len(b))
	copy(c, b)
	for i := 0; i < len(b); i++ {
		b[i] = 0
	}
	sk := SecretKey(sodium.BoxSecretKey{Bytes: sodium.Bytes(c)})
	sk.setFinalizer()
	return &sk
}

func (k *SecretKey) setFinalizer() {
	stack := log.Stack()
	runtime.SetFinalizer(k, func(obj interface{}) {
		sk := obj.(*SecretKey)
		for i := range sk.Bytes {
			if sk.Bytes[i] != 0 {
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

func (k SecretKey) ToBytes() []byte {
	return []byte(sodium.BoxSecretKey(k).Bytes)
}

func (k SecretKey) Empty() bool {
	return sodium.BoxSecretKey(k).Bytes == nil
}

func (k SecretKey) PublicKey() PublicKey {
	pk := sodium.BoxSecretKey(k).PublicKey()
	return PublicKeyFromBytes(pk.Bytes)
}

func (k *SecretKey) Wipe() {
	if k == nil {
		return
	}
	for i := range k.Bytes {
		k.Bytes[i] = 0
	}
	runtime.SetFinalizer(k, nil)
}

func (k PublicKey) sodium() sodium.BoxPublicKey {
	return sodium.BoxPublicKey{Bytes: sodium.Bytes(k.B[:])}
}

// EncryptMessage encrypts a message using Authenticated Public Key Encryption.
// https://pkg.go.dev/github.com/jamesruan/sodium#hdr-Authenticated_Public_Key_Encryption
func EncryptMessage(msg []byte, pk PublicKey, sk *SecretKey) string {
	var n sodium.BoxNonce
	sodium.Randomize(&n)

	m := []byte(n.Bytes)
	m = append(m, []byte(sodium.Bytes(msg).Box(n, pk.sodium(), sodium.BoxSecretKey(*sk)))...)
	return base64.StdEncoding.EncodeToString(m)
}

// DecryptMessage decrypts a message using Authenticated Public Key Encryption.
// https://pkg.go.dev/github.com/jamesruan/sodium#hdr-Authenticated_Public_Key_Encryption
func DecryptMessage(msg string, pk PublicKey, sk *SecretKey) ([]byte, error) {
	b, err := base64.StdEncoding.DecodeString(msg)
	if err != nil {
		return nil, err
	}
	var n sodium.BoxNonce
	if len(b) < n.Size() {
		return nil, errors.New("msg too short")
	}
	n.Bytes = make([]byte, n.Size())
	copy(n.Bytes, b[:n.Size()])
	b = b[n.Size():]
	m, err := sodium.Bytes(b).BoxOpen(n, pk.sodium(), sodium.BoxSecretKey(*sk))
	if err != nil {
		return nil, err
	}
	return []byte(m), nil
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
	b := sodium.Bytes(msg)
	return []byte(b.SealedBox(pk.sodium()))
}

// SealBoxOpen decrypts a message encrypted by SealBox.
func (sk SecretKey) SealBoxOpen(msg []byte) ([]byte, error) {
	ssk := sodium.BoxSecretKey(sk)
	kp := sodium.BoxKP{PublicKey: ssk.PublicKey(), SecretKey: ssk}
	d, err := sodium.Bytes(msg).SealedBoxOpen(kp)
	if err != nil {
		return nil, err
	}
	return []byte(d), nil
}

// EncryptSymmetric encrypts msg with a symmetric key.
func EncryptSymmetric(msg, nonce, key []byte) []byte {
	n := sodium.SecretBoxNonce{Bytes: sodium.Bytes(nonce)}
	k := sodium.SecretBoxKey{Bytes: sodium.Bytes(key)}
	return []byte(sodium.Bytes(msg).SecretBox(n, k))
}

// DecryptSymmetric decrypts msg with a symmetric key.
func DecryptSymmetric(msg, nonce, key []byte) ([]byte, error) {
	n := sodium.SecretBoxNonce{Bytes: sodium.Bytes(nonce)}
	k := sodium.SecretBoxKey{Bytes: sodium.Bytes(key)}
	ret, err := sodium.Bytes(msg).SecretBoxOpen(n, k)
	if err != nil {
		return nil, err
	}
	return []byte(ret), nil
}
