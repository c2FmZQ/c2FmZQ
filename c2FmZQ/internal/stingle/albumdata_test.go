package stingle

import (
	"testing"
)

func TestAlbumMetadata(t *testing.T) {
	sk := MakeSecretKeyForTest()
	md := AlbumMetadata{Name: "foobar"}
	enc := EncryptAlbumMetadata(md, sk.PublicKey())

	dec, err := DecryptAlbumMetadata(enc, sk)
	if err != nil {
		t.Fatalf("DecryptAlbumMetadata: %v", err)
	}
	if want, got := md.Name, dec.Name; want != got {
		t.Errorf("unexpected result. Want %q, got %q", want, got)
	}
}
