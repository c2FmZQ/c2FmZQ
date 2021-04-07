package stingle

import (
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Holds signed, tamper-proof data used to authenticate requests.
type Token struct {
	// Who this token was issued to.
	Subject int64 `json:"sub"`
	// The user's current token seq number.
	Seq int `json:"seq"`
	// The reason/purpose of the token.
	Scope string `json:"scope"`
	// When the token was issued.
	IssuedAt int64 `json:"iat"`
	// When the token exipres.
	Expiration int64 `json:"exp"`
	// The file this token gives access to.
	File string `json:"file,omitempty"`
	// The set in which the file is.
	Set string `json:"set,omitempty"`
	// Whether the access is granted for the thumbnail.
	Thumb bool `json:"thumb,omitempty"`
	// The server's signature.
	Signature string `json:"sig,omitempty"`
}

// MintToken returns an encoded & signed token.
func MintToken(key SignSecretKey, tok Token, exp time.Duration) string {
	tok.IssuedAt = time.Now().Unix()
	tok.Expiration = time.Now().Add(exp).Unix()
	ser, _ := json.Marshal(tok)
	tok.Signature = hex.EncodeToString(key.Sign(ser))
	ser, _ = json.Marshal(tok)
	return base64.RawURLEncoding.EncodeToString(ser)
}

// DecodeToken decodes an encoded token. The returned token hasn't been validated yet.
func DecodeToken(t string) (Token, error) {
	ser, err := base64.RawURLEncoding.DecodeString(t)
	if err != nil {
		return Token{}, err
	}
	var tok Token
	if err := json.Unmarshal(ser, &tok); err != nil {
		return Token{}, err
	}
	return tok, nil
}

// ValidateToken validates the signature in the token. If an error is returns,
// the information in the token cannot be trusted.
func ValidateToken(sk SignSecretKey, tok Token) error {
	sig, err := hex.DecodeString(tok.Signature)
	if err != nil {
		return err
	}
	tok.Signature = ""
	ser, err := json.Marshal(tok)
	if err != nil {
		return err
	}
	if subtle.ConstantTimeCompare(sk.Sign(ser), sig) != 1 {
		return errors.New("signature doesn't match")
	}
	if now := time.Now().Unix(); tok.Expiration < now {
		return fmt.Errorf("token is expired (%d < %d)", tok.Expiration, now)
	}
	return nil
}
