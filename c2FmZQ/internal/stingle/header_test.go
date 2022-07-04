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
	"reflect"
	"testing"
)

func TestDecryptHeader(t *testing.T) {
	sk := MakeSecretKeyForTest()

	header := &Header{
		FileID:        []byte("ABCDEFGHIJKLMNOPQRSTUVWXYZ123456"),
		Version:       1,
		ChunkSize:     1024,
		DataSize:      2048,
		SymmetricKey:  []byte("01234567890123456789012345678901"),
		FileType:      2,
		Filename:      []byte("FOOBAR"),
		VideoDuration: 1234,
	}
	var enc bytes.Buffer
	if err := EncryptHeader(&enc, header, sk.PublicKey()); err != nil {
		t.Fatalf("EncryptHeader: %v", err)
	}

	dec, err := DecryptHeader(&enc, sk)
	defer dec.Wipe()
	if err != nil {
		t.Fatalf("DecryptHeader: %v", err)
	}

	if want, got := header, dec; !reflect.DeepEqual(want, got) {
		t.Errorf("DecryptHeader returned unexpected result. Want %#v, got %#v", want, got)
	}
}
