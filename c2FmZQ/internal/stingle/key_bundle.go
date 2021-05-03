package stingle

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"c2FmZQ/internal/stingle/pwhash"
)

// MakeKeyBundle creates a KeyBundle with the public key.
func MakeKeyBundle(pk PublicKey) string {
	b := []byte{'S', 'P', 'K', 1, 2}
	b = append(b, pk.ToBytes()...)
	return base64.StdEncoding.EncodeToString(b)
}

// DecodeKeyBundle extracts the PublicKey from a KeyBundle.
func DecodeKeyBundle(bundle string) (pk PublicKey, err error) {
	key := make([]byte, 32)

	b, err := base64.StdEncoding.DecodeString(bundle)
	if err != nil {
		return pk, err
	}
	if len(b) < len(key)+5 {
		return pk, fmt.Errorf("bundle is too short: %d", len(b))
	}

	// Header
	if !bytes.Equal(b[:4], []byte{'S', 'P', 'K', 1}) {
		return pk, fmt.Errorf("unexpected bundle header %v", b[:4])
	}
	b = b[4:]

	// Key file type
	kfType := b[0]
	b = b[1:]

	switch kfType {
	case 0: // Bundle encrypted
		copy(key, b[:len(key)])
	case 2: // Public plain
		copy(key, b[:len(key)])
	default:
		return pk, errors.New("unexpected key file type")
	}
	return PublicKeyFromBytes(key), nil

}

// MakeKeyBundle creates a KeyBundle with the public key.
func MakeSecretKeyBundle(password []byte, sk SecretKey) string {
	pk := sk.PublicKey()
	b := []byte{'S', 'P', 'K', 1, 0}
	b = append(b, pk.ToBytes()...)
	b = append(b, EncryptSecretKeyForExport(password, sk)...)
	return base64.StdEncoding.EncodeToString(b)
}

// DecodeSecretKeyBundle extracts the SecretKey from a KeyBundle.
func DecodeSecretKeyBundle(password []byte, bundle string) (sk SecretKey, err error) {
	b, err := base64.StdEncoding.DecodeString(bundle)
	if err != nil {
		return sk, err
	}
	if len(b) != 125 { // hdr(5) + pk(32) + esk(88)
		return sk, fmt.Errorf("bundle is too short: %d", len(b))
	}

	// Header
	if !bytes.Equal(b[:4], []byte{'S', 'P', 'K', 1}) {
		return sk, fmt.Errorf("unexpected bundle header %v", b[:4])
	}
	b = b[4:]

	// Key file type
	kfType := b[0]
	b = b[1:]

	if kfType != 0 {
		return sk, errors.New("secret key is not in bundle")
	}
	// public key.
	pk := b[:32]
	b = b[32:]
	if sk, err = DecryptSecretKeyFromBundle(password, b); err != nil {
		return sk, err
	}
	// Sanity check that the public key and secret key match.
	if bytes.Compare(sk.PublicKey().ToBytes(), pk) != 0 {
		return sk, fmt.Errorf("encoded public key doesn't match secret key")
	}
	return sk, nil
}

// EncryptSecretKeyForExport encrypts the secret key with password.
func EncryptSecretKeyForExport(password []byte, sk SecretKey) []byte {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		panic(err)
	}
	key := pwhash.KeyFromPassword(password, salt, pwhash.Moderate, 32)
	nonce := make([]byte, 24)
	if _, err := rand.Read(nonce); err != nil {
		panic(err)
	}
	out := EncryptSymmetric(sk.ToBytes(), nonce, key)
	out = append(out, salt...)
	out = append(out, nonce...)
	return out
}

// DecryptSecretKeyFromBundle decrypts the secret key encoded in a bundle.
func DecryptSecretKeyFromBundle(password, encryptedKey []byte) (sk SecretKey, err error) {
	if len(encryptedKey) != 88 {
		return sk, fmt.Errorf("expected encrypted key size 88, got %d", len(encryptedKey))
	}
	nonce := encryptedKey[len(encryptedKey)-24:]
	salt := encryptedKey[len(encryptedKey)-40 : len(encryptedKey)-24]
	key := pwhash.KeyFromPassword(password, salt, pwhash.Moderate, 32)
	b, err := DecryptSymmetric(encryptedKey[:len(encryptedKey)-40], nonce, key)
	if err != nil {
		return sk, err
	}
	return SecretKeyFromBytes(b), nil
}

// PasswordHashForLogin returns a hash of password used for login. salt is 16 bytes.
func PasswordHashForLogin(password, salt []byte) string {
	hash := pwhash.KeyFromPassword(password, salt, pwhash.Moderate, 64)
	return strings.ToUpper(hex.EncodeToString(hash))
}
