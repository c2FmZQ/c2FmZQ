package stingle

import (
	"encoding/base64"
	"encoding/json"
	"errors"
)

type PublicKey struct {
	B [32]byte
}

func PublicKeyFromBytes(b []byte) (pk PublicKey) {
	copy(pk.B[:], b)
	return
}

func (pk PublicKey) ToBytes() []byte {
	return pk.B[:]
}

func (pk *PublicKey) UnmarshalBinary(b []byte) error {
	if len(b) != 32 {
		return errors.New("invalid public key")
	}
	copy(pk.B[:], b)
	return nil
}

func (pk PublicKey) MarshalBinary() ([]byte, error) {
	return pk.B[:], nil
}

func (pk *PublicKey) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return err
	}
	if len(b) != 32 {
		return errors.New("invalid public key")
	}
	copy(pk.B[:], b)
	return nil
}

func (pk PublicKey) MarshalJSON() ([]byte, error) {
	return json.Marshal(base64.RawURLEncoding.EncodeToString(pk.B[:]))
}
