package stingle

import (
	"bytes"
	"reflect"
	"testing"
)

func TestEncryptDecrypt(t *testing.T) {
	senderKey := MakeSecretKey()
	receiverKey := MakeSecretKey()

	msg := []byte("blah blah blah 123")
	encrypted := EncryptMessage(msg, receiverKey.PublicKey(), senderKey)

	if got, err := DecryptMessage(encrypted, senderKey.PublicKey(), receiverKey); err != nil {
		t.Errorf("DecryptMessage failed, err: %v", err)
	} else if !bytes.Equal(got, msg) {
		t.Errorf("DecryptMessage got %q, want %q", got, msg)
	}
}

func TestSealBox(t *testing.T) {
	key := MakeSecretKey()
	msg := []byte("foo bar")
	enc := SealBox(msg, key.PublicKey())

	dec, err := SealBoxOpen(enc, key)
	if err != nil {
		t.Fatalf("SealBoxOpen failed: %v", err)
	}
	if want, got := msg, dec; !reflect.DeepEqual(want, got) {
		t.Errorf("SealBoxOpen returned unexpected result: Want %q, got %q", want, got)
	}
}
