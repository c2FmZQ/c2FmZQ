// +build !nacl,!arm

package stingle

import (
	"encoding/base64"
	"encoding/json"
	"errors"

	"github.com/jamesruan/sodium"
)

// MakeSecretKey returns a new SecretKey.
func MakeSecretKey() SecretKey {
	kp := sodium.MakeBoxKP()
	return SecretKey(kp.SecretKey)
}

type SecretKey sodium.BoxSecretKey
type PublicKey sodium.BoxPublicKey

func SecretKeyFromBytes(b []byte) SecretKey {
	return SecretKey(sodium.BoxSecretKey{Bytes: sodium.Bytes(b)})
}

func PublicKeyFromBytes(b []byte) PublicKey {
	return PublicKey(sodium.BoxPublicKey{Bytes: sodium.Bytes(b)})
}

func (k SecretKey) ToBytes() []byte {
	return []byte(sodium.BoxSecretKey(k).Bytes)
}

func (k PublicKey) ToBytes() []byte {
	return []byte(sodium.BoxPublicKey(k).Bytes)
}

func (k SecretKey) Empty() bool {
	return sodium.BoxSecretKey(k).Bytes == nil
}

func (k SecretKey) PublicKey() PublicKey {
	return PublicKey(sodium.BoxSecretKey(k).PublicKey())
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
	k.Bytes = sodium.Bytes(b)
	return nil
}

func (k SecretKey) MarshalJSON() ([]byte, error) {
	return json.Marshal(base64.RawURLEncoding.EncodeToString(k.Bytes))
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
	k.Bytes = sodium.Bytes(b)
	return nil
}

func (k PublicKey) MarshalJSON() ([]byte, error) {
	return json.Marshal(base64.RawURLEncoding.EncodeToString(k.Bytes))
}

// MakeSignSecretKey returns a new SignSecretKey.
func MakeSignSecretKey() SignSecretKey {
	kp := sodium.MakeSignKP()
	return SignSecretKey(kp.SecretKey)
}

type SignSecretKey sodium.SignSecretKey
type SignPublicKey sodium.SignPublicKey

func (k SignSecretKey) Empty() bool {
	return sodium.SignSecretKey(k).Bytes == nil
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
	k.Bytes = sodium.Bytes(b)
	return nil
}

func (k SignSecretKey) MarshalJSON() ([]byte, error) {
	return json.Marshal(base64.RawURLEncoding.EncodeToString(k.Bytes))
}

func (k SignSecretKey) PublicKey() SignPublicKey {
	return SignPublicKey(sodium.SignSecretKey(k).PublicKey())
}

func (k SignSecretKey) Sign(msg []byte) []byte {
	return sodium.Bytes(msg).SignDetached(sodium.SignSecretKey(k)).Bytes
}

// EncryptMessage encrypts a message using Authenticated Public Key Encryption.
// https://pkg.go.dev/github.com/jamesruan/sodium#hdr-Authenticated_Public_Key_Encryption
func EncryptMessage(msg []byte, pk PublicKey, sk SecretKey) string {
	var n sodium.BoxNonce
	sodium.Randomize(&n)

	m := []byte(n.Bytes)
	m = append(m, []byte(sodium.Bytes(msg).Box(n, sodium.BoxPublicKey(pk), sodium.BoxSecretKey(sk)))...)
	return base64.StdEncoding.EncodeToString(m)
}

// DecryptMessage decrypts a message using Authenticated Public Key Encryption.
// https://pkg.go.dev/github.com/jamesruan/sodium#hdr-Authenticated_Public_Key_Encryption
func DecryptMessage(msg string, pk PublicKey, sk SecretKey) ([]byte, error) {
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
	m, err := sodium.Bytes(b).BoxOpen(n, sodium.BoxPublicKey(pk), sodium.BoxSecretKey(sk))
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
	return []byte(b.SealedBox(sodium.BoxPublicKey(pk)))
}

// SealBoxOpen decrypts a message encrypted by SealBox.
func (sk SecretKey) SealBoxOpen(msg []byte) ([]byte, error) {
	kp := sodium.BoxKP{PublicKey: sodium.BoxPublicKey(sk.PublicKey()), SecretKey: sodium.BoxSecretKey(sk)}
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
