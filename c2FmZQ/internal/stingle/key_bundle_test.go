package stingle

import (
	"encoding/hex"
	"reflect"
	"testing"
)

func TestPublicKeyBundle(t *testing.T) {
	sk := MakeSecretKeyForTest()

	b := MakeKeyBundle(sk.PublicKey())
	t.Logf("bundle: %s", b)

	pk, err := DecodeKeyBundle(b)
	if err != nil {
		t.Fatalf("DecodeKeyBundle: %v", err)
	}

	if !reflect.DeepEqual(pk, sk.PublicKey()) {
		t.Errorf("Public keys don't match. Want %v, got %v", sk.PublicKey(), pk)
	}
}

func TestSecretKeyBundle(t *testing.T) {
	pass := []byte("foobar")
	want := MakeSecretKeyForTest()

	b := MakeSecretKeyBundle(pass, want)
	t.Logf("bundle: %s", b)

	got, err := DecodeSecretKeyBundle(pass, b)
	defer got.Wipe()
	if err != nil {
		t.Fatalf("DecodeKeyBundle: %v", err)
	}

	if !reflect.DeepEqual(want, got) {
		t.Errorf("Secret keys don't match. Want %v, got %v", want, got)
	}
}

func TestEncryptSecretKey(t *testing.T) {
	sk := MakeSecretKeyForTest()
	pass := []byte("foobar")

	enc := EncryptSecretKeyForExport(pass, sk)

	dec, err := DecryptSecretKeyFromBundle(pass, enc)
	defer dec.Wipe()
	if err != nil {
		t.Fatalf("DecryptSecretKeyFromBundle: %v", err)
	}
	if got, want := dec, sk; !reflect.DeepEqual(want, got) {
		t.Errorf("Unexpected result. Want %v, got %v", want, got)
	}
}

func TestPasswordForLogin(t *testing.T) {
	salt, _ := hex.DecodeString("19DE41D1BCB808221FA6D63777CCA7C2")
	want := "C2780F400FB0759543892B9409787118E3E1D7156428BA7C515C1637C700B668A4F588B5DCDD58DC43137F0CB40CC55BF3D2885E99B59B62454AAD8EC4E643EF"
	got := PasswordHashForLogin([]byte("foobar"), salt)
	if want != got {
		t.Errorf("PasswordHashForLogin: want %q, got %q", want, got)
	}
}
