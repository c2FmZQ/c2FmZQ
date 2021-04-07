package stingle

import (
	"reflect"
	"testing"
)

func TestBundle(t *testing.T) {
	sk := MakeSecretKey()

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
