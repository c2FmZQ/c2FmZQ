// +build nacl arm

package stingle

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"

	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/ed25519"
	"golang.org/x/crypto/nacl/box"
	"golang.org/x/crypto/nacl/secretbox"
	"golang.org/x/crypto/nacl/sign"
)

// MakeSecretKey returns a new SecretKey.
func MakeSecretKey() SecretKey {
	sk := SecretKey{B: new([32]byte)}
	if _, err := io.ReadFull(rand.Reader, sk.B[:]); err != nil {
		panic(err)
	}
	return sk
}

type SecretKey struct {
	B *[32]byte
}
type PublicKey struct {
	B *[32]byte
}

func (k SecretKey) ToBytes() []byte {
	return k.B[:]
}

func (k PublicKey) ToBytes() []byte {
	return k.B[:]
}

func SecretKeyFromBytes(b []byte) SecretKey {
	sk := SecretKey{B: new([32]byte)}
	copy(sk.B[:], b)
	return SecretKey(sk)
}

func PublicKeyFromBytes(b []byte) PublicKey {
	pk := PublicKey{B: new([32]byte)}
	copy(pk.B[:], b)
	return PublicKey(pk)
}

func (k SecretKey) PublicKey() PublicKey {
	pk := PublicKey{B: new([32]byte)}
	curve25519.ScalarBaseMult(pk.B, k.B)
	return pk
}

func (k *SecretKey) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return err
	}
	k.B = new([32]byte)
	copy(k.B[:], b)
	return nil
}

func (k SecretKey) MarshalJSON() ([]byte, error) {
	return json.Marshal(base64.RawURLEncoding.EncodeToString(k.B[:]))
}

func (k *PublicKey) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return err
	}
	k.B = new([32]byte)
	copy(k.B[:], b)
	return nil
}

func (k PublicKey) MarshalJSON() ([]byte, error) {
	return json.Marshal(base64.RawURLEncoding.EncodeToString(k.B[:]))
}

// MakeSignSecretKey returns a new SignSecretKey.
func MakeSignSecretKey() SignSecretKey {
	_, sk, err := sign.GenerateKey(rand.Reader)
	if err != nil {
		panic(err)
	}
	return SignSecretKey{B: sk}
}

type SignSecretKey struct {
	B *[64]byte
}

func (k SignSecretKey) Sign(msg []byte) []byte {
	return ed25519.Sign(ed25519.PrivateKey((*k.B)[:]), msg)
}

func (k *SignSecretKey) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return err
	}
	k.B = new([64]byte)
	copy(k.B[:], b)
	return nil
}

func (k SignSecretKey) MarshalJSON() ([]byte, error) {
	return json.Marshal(base64.RawURLEncoding.EncodeToString(k.B[:]))
}

type SignPublicKey struct {
	B *[32]byte
}

func (k SignSecretKey) PublicKey() SignPublicKey {
	pk := new([32]byte)
	copy((*pk)[:], k.B[32:])
	return SignPublicKey{B: pk}
}

// EncryptMessage encrypts a message using Authenticated Public Key Encryption.
// https://pkg.go.dev/github.com/jamesruan/sodium#hdr-Authenticated_Public_Key_Encryption
func EncryptMessage(msg []byte, pk PublicKey, sk SecretKey) string {
	nonce := new([24]byte)
	if _, err := io.ReadFull(rand.Reader, nonce[:]); err != nil {
		panic(err)
	}
	out := make([]byte, len(nonce))
	copy(out, nonce[:])
	ret := box.Seal(out, msg, nonce, pk.B, sk.B)
	return base64.StdEncoding.EncodeToString(ret)
}

// DecryptMessage decrypts a message using Authenticated Public Key Encryption.
// https://pkg.go.dev/github.com/jamesruan/sodium#hdr-Authenticated_Public_Key_Encryption
func DecryptMessage(msg string, pk PublicKey, sk SecretKey) ([]byte, error) {
	b, err := base64.StdEncoding.DecodeString(msg)
	if err != nil {
		return nil, err
	}
	nonce := new([24]byte)
	copy((*nonce)[:], b[:24])

	ret, ok := box.Open(nil, b[len(nonce):], nonce, pk.B, sk.B)
	if !ok {
		return nil, errors.New("box.Open failed")
	}
	return ret, nil
}

// SealBox encrypts a message using Anonymous Public Key Encryption.
func SealBox(msg []byte, pk PublicKey) string {
	ret, err := box.SealAnonymous(nil, msg, pk.B, rand.Reader)
	if err != nil {
		panic(err)
	}
	return base64.StdEncoding.EncodeToString(ret)
}

// SealBoxOpen decrypts a message encrypted by SealBox.
func SealBoxOpen(msg string, sk SecretKey) ([]byte, error) {
	b, err := base64.StdEncoding.DecodeString(msg)
	if err != nil {
		return nil, err
	}
	ret, ok := box.OpenAnonymous(nil, b, sk.PublicKey().B, sk.B)
	if !ok {
		return nil, errors.New("box.OpenAnonymous failed")
	}
	return ret, nil
}

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
