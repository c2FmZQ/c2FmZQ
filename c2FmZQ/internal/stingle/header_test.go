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
