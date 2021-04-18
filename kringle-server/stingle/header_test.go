package stingle

import (
	"bytes"
	"reflect"
	"testing"
)

func TestDecodeHeader(t *testing.T) {
	sk := MakeSecretKey()

	header := Header{
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
	if err := EncodeHeader(&enc, header, sk.PublicKey()); err != nil {
		t.Fatalf("EncodeHeader: %v", err)
	}

	dec, err := DecodeHeader(&enc, sk)
	if err != nil {
		t.Fatalf("DecodeHeader: %v", err)
	}

	if want, got := header, dec; !reflect.DeepEqual(want, got) {
		t.Errorf("DecodeHeader returned unexpected result. Want %#v, got %#v", want, got)
	}

}
