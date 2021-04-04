package stinglecrypto

import (
	"encoding/base64"
	"errors"

	"github.com/jamesruan/sodium"
)

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

// SealBox encrypts a message using Anonymous Public Key Encryption.
// https://pkg.go.dev/github.com/jamesruan/sodium#hdr-Anonymous_Public_Key_Encryption
func SealBox(msg []byte, pk PublicKey) string {
	b := sodium.Bytes(msg)
	return base64.StdEncoding.EncodeToString([]byte(b.SealedBox(sodium.BoxPublicKey(pk))))
}

// SealBoxOpen decrypts a message encrypted by SealBox.
func SealBoxOpen(msg string, sk SecretKey) ([]byte, error) {
	b, err := base64.StdEncoding.DecodeString(msg)
	if err != nil {
		return nil, err
	}
	kp := sodium.BoxKP{sodium.BoxPublicKey(sk.PublicKey()), sodium.BoxSecretKey(sk)}
	d, err := sodium.Bytes(b).SealedBoxOpen(kp)
	if err != nil {
		return nil, err
	}
	return []byte(d), nil
}
