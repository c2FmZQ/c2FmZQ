package crypto

import (
	"bytes"
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
		t.Errorf("DecryptMessage got %v, want %v", got, msg)
	}
}
