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
	"bytes"
	"encoding/binary"
	"errors"
)

type AlbumMetadata struct {
	Name string `json:"name"`
}

// DecryptAlbumMetadata decrypts an album's metadata.
func DecryptAlbumMetadata(md string, sk *SecretKey) (*AlbumMetadata, error) {
	b, err := sk.SealBoxOpenBase64(md)
	if err != nil {
		return nil, err
	}
	if len(b) < 5 {
		return nil, errors.New("invalid metadata")
	}
	if b[0] != 1 {
		return nil, errors.New("unexpected version")
	}
	b = b[1:]
	l := int(binary.BigEndian.Uint32(b[:4]))
	b = b[4:]
	if l < 0 || l > len(b) {
		return nil, errors.New("invalid name length")
	}
	name := string(b[:l])
	return &AlbumMetadata{Name: name}, nil
}

// EncryptAlbumMetadata encrypts an album's metadata.
func EncryptAlbumMetadata(md AlbumMetadata, pk PublicKey) string {
	var buf bytes.Buffer
	buf.Write([]byte{1}) // version
	binary.Write(&buf, binary.BigEndian, uint32(len(md.Name)))
	buf.Write([]byte(md.Name))
	return pk.SealBoxBase64(buf.Bytes())
}
