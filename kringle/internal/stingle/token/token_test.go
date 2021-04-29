package token

import (
	"testing"
	"time"
)

func TestTokens(t *testing.T) {
	key := MakeKey()
	tok := Mint(key, Token{Scope: "foo", Subject: 44545}, time.Hour)

	dec, err := Decrypt(key, tok)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}
	if dec.Scope != "foo" || dec.Subject != 44545 {
		t.Errorf("Unexpected token. Got %+v, want {'foo', 'blah blah'}", dec)
	}
}
