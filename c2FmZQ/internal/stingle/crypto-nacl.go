// +build nacl arm

package stingle

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"

	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/ed25519"
	"golang.org/x/crypto/nacl/box"
	"golang.org/x/crypto/nacl/secretbox"
	"golang.org/x/crypto/nacl/sign"

	"c2FmZQ/internal/log"
)

// MakeSecretKey returns a new SecretKey.
func MakeSecretKey() *SecretKey {
	sk := SecretKey{B: new([32]byte)}
	if _, err := rand.Read(sk.B[:]); err != nil {
		panic(err)
	}
	return &sk
}

func MakeSecretKeyForTest() *SecretKey {
	return MakeSecretKey()
}

func SecretKeyFromBytes(b []byte) *SecretKey {
	sk := SecretKey{B: new([32]byte)}
	copy(sk.B[:], b)
	for i := 0; i < len(b); i++ {
		b[i] = 0
	}
	return &sk
}

type SecretKey struct {
	B *[32]byte
}

func (k *SecretKey) Wipe() {
	if k == nil {
		return
	}
	for i := range *k.B {
		(*k.B)[i] = 0
	}
	if log.Level > log.DebugLevel {
		log.Debugf("Wiped %#v", *k)
	}
}

func (k SecretKey) ToBytes() []byte {
	return k.B[:]
}

func (k SecretKey) PublicKey() (pk PublicKey) {
	curve25519.ScalarBaseMult(&pk.B, k.B)
	return
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

func (k PublicKey) nacl() *[32]byte {
	return &k.B
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
// https://pkg.go.dev/github.com/jamesruan/sodium#hdr-Authenticated_Public_Key_Encryption
func DecryptMessage(msg string, pk PublicKey, sk *SecretKey) ([]byte, error) {
	b, err := base64.StdEncoding.DecodeString(msg)
	if err != nil {
		return nil, err
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
