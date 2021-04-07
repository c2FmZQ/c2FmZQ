package stingle

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
)

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
func MakeKeyBundle(pk PublicKey) string {
	b := []byte{'S', 'P', 'K', 1, 2}
	b = append(b, pk.ToBytes()...)
	return base64.StdEncoding.EncodeToString(b)
}
