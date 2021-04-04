package stingle

import (
	"testing"
	"time"
)

func TestTokens(t *testing.T) {
	key := MakeSignSecretKey()
	tok := MintToken(key, Token{Scope: "foo", Subject: 44545}, time.Hour)

	decoded, err := DecodeToken(tok)
	if err != nil {
		t.Fatalf("DecodeToken failed: %v", err)
	}
	if decoded.Scope != "foo" || decoded.Subject != 44545 {
		t.Errorf("Unexpected token. Got %+v, want {'foo', 'blah blah'}", decoded)
	}

	if err := ValidateToken(key, decoded); err != nil {
		t.Errorf("ValidateToken failed. err = %v", err)
	}

	decoded.Scope = "bar" // Invalidates the signature
	if err := ValidateToken(key, decoded); err == nil {
		t.Errorf("ValidateToken(invTok) succeeded unexpectedly, err=%v", err)
	}
}
