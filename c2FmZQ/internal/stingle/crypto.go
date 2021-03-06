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
