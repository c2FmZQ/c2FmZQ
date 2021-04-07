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
	"golang.org/x/crypto/nacl/sign"
)

// MakeSecretKey returns a new SecretKey.
func MakeSecretKey() SecretKey {
	sk := SecretKey{b: new([32]byte)}
	if _, err := io.ReadFull(rand.Reader, sk.b[:]); err != nil {
		panic(err)
	}
	return sk
}

type SecretKey struct {
	b *[32]byte
}
type PublicKey struct {
	b *[32]byte
}

func (k PublicKey) ToBytes() []byte {
	return k.b[:]
}

func PublicKeyFromBytes(b []byte) PublicKey {
	pk := PublicKey{b: new([32]byte)}
	copy(pk.b[:], b)
	return PublicKey(pk)
}

func (k SecretKey) PublicKey() PublicKey {
	pk := PublicKey{b: new([32]byte)}
	curve25519.ScalarBaseMult(pk.b, k.b)
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
	k.b = new([32]byte)
	copy(k.b[:], b)
	return nil
}

func (k SecretKey) MarshalJSON() ([]byte, error) {
	return json.Marshal(base64.RawURLEncoding.EncodeToString(k.b[:]))
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
	k.b = new([32]byte)
	copy(k.b[:], b)
	return nil
}

func (k PublicKey) MarshalJSON() ([]byte, error) {
	return json.Marshal(base64.RawURLEncoding.EncodeToString(k.b[:]))
}

// MakeSignSecretKey returns a new SignSecretKey.
func MakeSignSecretKey() SignSecretKey {
	_, sk, err := sign.GenerateKey(rand.Reader)
	if err != nil {
		panic(err)
	}
	return SignSecretKey{b: sk}
}

type SignSecretKey struct {
	b *[64]byte
}

func (k SignSecretKey) Sign(msg []byte) []byte {
	return ed25519.Sign(ed25519.PrivateKey((*k.b)[:]), msg)
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
	k.b = new([64]byte)
	copy(k.b[:], b)
	return nil
}

func (k SignSecretKey) MarshalJSON() ([]byte, error) {
	return json.Marshal(base64.RawURLEncoding.EncodeToString(k.b[:]))
}

type SignPublicKey struct {
	b *[32]byte
}

func (k SignSecretKey) PublicKey() SignPublicKey {
	pk := new([32]byte)
	copy((*pk)[:], k.b[32:])
	return SignPublicKey{b: pk}
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
	ret := box.Seal(out, msg, nonce, pk.b, sk.b)
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

	ret, ok := box.Open(nil, b[len(nonce):], nonce, pk.b, sk.b)
	if !ok {
		return nil, errors.New("box.Open failed")
	}
	return ret, nil
}

// SealBox encrypts a message using Anonymous Public Key Encryption.
func SealBox(msg []byte, pk PublicKey) string {
	ret, err := box.SealAnonymous(nil, msg, pk.b, rand.Reader)
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
	ret, ok := box.OpenAnonymous(nil, b, sk.PublicKey().b, sk.b)
	if !ok {
		return nil, errors.New("box.OpenAnonymous failed")
	}
	return ret, nil
}
