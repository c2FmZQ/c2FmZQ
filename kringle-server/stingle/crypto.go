package stingle

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jamesruan/sodium"
)

// MakeSecretKey returns a new SecretKey.
func MakeSecretKey() SecretKey {
	kp := sodium.MakeBoxKP()
	return SecretKey(kp.SecretKey)
}

type SecretKey sodium.BoxSecretKey
type PublicKey sodium.BoxPublicKey

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

type SignPublicKey sodium.SignPublicKey

// DecodeKeyBundle extracts the PublicKey from a KeyBundle.
func DecodeKeyBundle(bundle string) (PublicKey, error) {
	var pk sodium.BoxPublicKey

	b, err := base64.StdEncoding.DecodeString(bundle)
	if err != nil {
		return PublicKey(pk), err
	}
	if len(b) < pk.Size()+5 {
		return PublicKey(pk), fmt.Errorf("bundle is too short: %d", len(b))
	}

	// Header
	if !bytes.Equal(b[:4], []byte{'S', 'P', 'K', 1}) {
		return PublicKey(pk), fmt.Errorf("unexpected bundle header %v", b[:4])
	}
	b = b[4:]

	// Key file type
	kfType := b[0]
	b = b[1:]

	switch kfType {
	case 0: // Bundle encrypted
		pk.Bytes = make([]byte, pk.Size())
		copy(pk.Bytes, b[:pk.Size()])
	case 2: // Public plain
		pk.Bytes = make([]byte, pk.Size())
		copy(pk.Bytes, b[:pk.Size()])
	default:
		return PublicKey(pk), errors.New("unexpected key file type")
	}
	return PublicKey(pk), nil

}

func MakeKeyBundle(pk PublicKey) string {
	b := []byte{'S', 'P', 'K', 1, 2}
	b = append(b, []byte(sodium.BoxPublicKey(pk).Bytes)...)
	return base64.StdEncoding.EncodeToString(b)
}
