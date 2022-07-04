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

func TestEncryptDecrypt(t *testing.T) {
	senderKey := MakeSecretKeyForTest()
	receiverKey := MakeSecretKeyForTest()

	msg := []byte("blah blah blah 123")
	encrypted := EncryptMessage(msg, receiverKey.PublicKey(), senderKey)

	if got, err := DecryptMessage(encrypted, senderKey.PublicKey(), receiverKey); err != nil {
		t.Errorf("DecryptMessage failed, err: %v", err)
	} else if !bytes.Equal(got, msg) {
		t.Errorf("DecryptMessage got %q, want %q", got, msg)
	}
}

func TestSealBox(t *testing.T) {
	key := MakeSecretKeyForTest()
	msg := []byte("foo bar")
	enc := key.PublicKey().SealBox(msg)

	dec, err := key.SealBoxOpen(enc)
	if err != nil {
		t.Fatalf("SealBoxOpen failed: %v", err)
	}
	if want, got := msg, dec; !reflect.DeepEqual(want, got) {
		t.Errorf("SealBoxOpen returned unexpected result: Want %q, got %q", want, got)
	}
}

func TestSymmetric(t *testing.T) {
	nonce := []byte("abcdefghijklmnopqrstuvwx")
	key := []byte("ABCDEFGHIJKLMNOPQRSTUVWXYZ123456")
	msg := []byte("Hello world!")

	enc := EncryptSymmetric(msg, nonce, key)
	dec, err := DecryptSymmetric(enc, nonce, key)
	if err != nil {
		t.Fatalf("DecryptSymmetric: %v", err)
	}
	if want, got := msg, dec; !bytes.Equal(want, got) {
		t.Errorf("DecryptSymmetric: want %q, got %q", want, got)
	}
}
