// Copyright 2021-2022 TTBT Enterprises LLC
//
// This file is part of c2FmZQ (https://c2FmZQ.org/).
//
// c2FmZQ is free software: you can redistribute it and/or modify it under the
// terms of the GNU General Public License as published by the Free Software
// Foundation, either version 3 of the License, or (at your option) any later
// version.
//
// c2FmZQ is distributed in the hope that it will be useful, but WITHOUT ANY
// WARRANTY; without even the implied warranty of MERCHANTABILITY or FITNESS FOR
// A PARTICULAR PURPOSE. See the GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License along with
// c2FmZQ. If not, see <https://www.gnu.org/licenses/>.

// Package webauthn implements the server side of WebAuthn.
package webauthn

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"

	cbor "github.com/fxamacker/cbor/v2"
)

const (
	// https://w3c.github.io/webauthn/#sctn-alg-identifier
	algES256 = -7
	algRS256 = -257
)

// ErrTooShort indicates that the message is too short and can't be decoded.
var ErrTooShort = errors.New("too short")

// AttestationOptions encapsulates the options to navigator.credentials.create().
type AttestationOptions struct {
	// The cryptographic challenge is 32 random bytes.
	Challenge string `json:"challenge"`
	// The name of the relying party, i.e. c2FmZQ. The ID is optional.
	RelyingParty struct {
		Name string `json:"name"`
		ID   string `json:"id,omitempty"`
	} `json:"rp"`
	// The user information.
	User struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		DisplayName string `json:"displayName"`
	} `json:"user"`
	// The acceptable public key params.
	PubKeyCredParams []PubKeyCredParam `json:"pubKeyCredParams,omitempty"`
	// Timeout in milliseconds.
	Timeout int `json:"timeout,omitempty"`
	// A list of credentials already registered for this user.
	ExcludeCredentials []CredentialID `json:"excludeCredentials,omitempty"`
	// The type of attestation
	Attestation string `json:"attestation,omitempty"`
	// Authticator selection parameters.
	AuthenticatorSelection struct {
		// required, preferred, or discouraged
		UserVerification string `json:"userVerification"`
	} `json:"authenticatorSelection"`
	// Extensions.
	Extensions map[string]interface{} `json:"extensions,omitempty"`
}

// NewAttestationOptions returns a new AttestationOptions with Challenge,
// PubKeyCredParams, and Timeout already populated.
func NewAttestationOptions() (*AttestationOptions, error) {
	ao := &AttestationOptions{
		PubKeyCredParams: []PubKeyCredParam{
			{
				Type: "public-key",
				Alg:  algES256,
			},
			{
				Type: "public-key",
				Alg:  algRS256,
			},
		},
		Timeout:     60000, // 60 sec
		Attestation: "none",
	}
	ao.AuthenticatorSelection.UserVerification = "discouraged"

	challenge := make([]byte, 32)
	if _, err := rand.Read(challenge); err != nil {
		return nil, err
	}
	ao.Challenge = base64.RawURLEncoding.EncodeToString(challenge)
	return ao, nil
}

// AssertionOptions encapsulates the options to navigator.credentials.get().
type AssertionOptions struct {
	// The cryptographic challenge is 32 random bytes.
	Challenge string `json:"challenge"`
	// Timeout in milliseconds.
	Timeout int `json:"timeout,omitempty"`
	// A list of credentials already registered for this user.
	AllowCredentials []CredentialID `json:"allowCredentials"`
	// UserVerification: required, preferred, discouraged
	UserVerification string `json:"userVerification"`
}

// NewAssertionOptions returns a new AssertionOptions with Challenge,
// and Timeout already populated.
func NewAssertionOptions() (*AssertionOptions, error) {
	ao := &AssertionOptions{
		Timeout:          20000, // 20 sec
		UserVerification: "discouraged",
	}
	challenge := make([]byte, 32)
	if _, err := rand.Read(challenge); err != nil {
		return nil, err
	}
	ao.Challenge = base64.RawURLEncoding.EncodeToString(challenge)
	return ao, nil
}

// PubKeyCredParam: Public key credential parameters.
type PubKeyCredParam struct {
	// The type of credentials. Always "public-key"
	Type string `json:"type"`
	// The encryption algorythm: -7 for ES256, -257 for RS256.
	Alg int `json:"alg"`
}

// CredentialID is a credential ID from an anthenticator.
type CredentialID struct {
	// The type of credentials. Always "public-key"
	Type string `json:"type"`
	// The credential ID, base64url encoded.
	ID string `json:"id"`
	// The available transports for this credential.
	Transports []string `json:"transports,omitempty"`
}

// ClientData is a decoded ClientDataJSON object.
type ClientData struct {
	Type      string `json:"type"`
	Challenge string `json:"challenge"`
	Origin    string `json:"origin"`
}

// Attestation. https://w3c.github.io/webauthn/#sctn-attestation
type Attestation struct {
	Format      string          `cbor:"fmt"`
	AttStmt     cbor.RawMessage `cbor:"attStmt"`
	RawAuthData []byte          `cbor:"authData"`

	AuthData AuthenticatorData `cbor:"-"`
}

// AuthenticatorData is the authenticator data provided during attestation and
// assertion. https://w3c.github.io/webauthn/#sctn-authenticator-data
type AuthenticatorData struct {
	RPIDHash               string               `json:"rpIdHash"`
	UserPresence           bool                 `json:"up"`
	BackupEligible         bool                 `json:"be"`
	BackupState            bool                 `json:"bs"`
	UserVerification       bool                 `json:"uv"`
	AttestedCredentialData bool                 `json:"at"`
	ExtensionData          bool                 `json:"ed"`
	SignCount              uint32               `json:"signCount"`
	AttestedCredentials    *AttestedCredentials `json:"attestedCredentialData"`
}

// AttestedCredentials. https://w3c.github.io/webauthn/#sctn-attested-credential-data
type AttestedCredentials struct {
	AAGUID  string `json:"AAGUID"`
	ID      string `json:"credentialId"`
	COSEKey string `json:"credentialPublicKey"`
}

// ParseAttestationObject parses an attestationObject, and performs a minimal
// amount of validation on packed and fido-u2f attestation statements.
func ParseAttestationObject(attestationObject, clientDataJSON []byte) (*Attestation, error) {
	var att Attestation
	if err := cbor.Unmarshal(attestationObject, &att); err != nil {
		return nil, fmt.Errorf("cbor.Unmarshal: %w", err)
	}
	if err := ParseAuthenticatorData(att.RawAuthData, &att.AuthData); err != nil {
		return nil, fmt.Errorf("ParseAuthenticatorData: %w", err)
	}
	// We don't actually verify the authenticator attestation. Users can use whatever
	// they want.
	return &att, nil
}

func ParseAuthenticatorData(raw []byte, ad *AuthenticatorData) error {
	// https://w3c.github.io/webauthn/#sctn-authenticator-data
	if len(raw) < 37 {
		return ErrTooShort
	}
	ad.RPIDHash = base64.RawURLEncoding.EncodeToString(raw[:32])
	raw = raw[32:]
	ad.UserPresence = raw[0]&1 != 0
	ad.UserVerification = (raw[0]>>2)&1 != 0
	ad.BackupEligible = (raw[0]>>3)&1 != 0
	ad.BackupState = (raw[0]>>4)&1 != 0
	ad.AttestedCredentialData = (raw[0]>>6)&1 != 0
	ad.ExtensionData = (raw[0]>>7)&1 != 0
	raw = raw[1:]
	ad.SignCount = binary.BigEndian.Uint32(raw[:4])
	raw = raw[4:]

	if ad.AttestedCredentialData {
		// https://w3c.github.io/webauthn/#sctn-attested-credential-data
		if len(raw) < 18 {
			return ErrTooShort
		}
		ad.AttestedCredentials = &AttestedCredentials{}
		ad.AttestedCredentials.AAGUID = base64.RawURLEncoding.EncodeToString(raw[:16])
		raw = raw[16:]

		sz := binary.BigEndian.Uint16(raw[:2])
		raw = raw[2:]
		if sz > 1023 {
			return errors.New("invalid credentialId length")
		}
		if len(raw) < int(sz) {
			return ErrTooShort
		}
		ad.AttestedCredentials.ID = base64.RawURLEncoding.EncodeToString(raw[:int(sz)])
		raw = raw[int(sz):]

		var coseKey cbor.RawMessage
		if err := cbor.Unmarshal(raw, &coseKey); err != nil {
			return err
		}
		ad.AttestedCredentials.COSEKey = base64.RawURLEncoding.EncodeToString(coseKey)
	}
	if ad.ExtensionData {
		// Parse extensions
	}
	return nil
}

func ParseClientData(js []byte) (*ClientData, error) {
	var out ClientData
	err := json.Unmarshal(js, &out)
	return &out, err
}

func signedBytes(authData, clientDataJSON []byte) []byte {
	clientDataHash := sha256.Sum256(clientDataJSON)
	signedBytes := make([]byte, len(authData)+len(clientDataHash))
	copy(signedBytes, authData)
	copy(signedBytes[len(authData):], clientDataHash[:])
	return signedBytes
}

// VerifySignature verifies the webauthn signature.
func VerifySignature(coseKey string, authData, clientDataJSON, signature []byte) error {
	key, err := base64.RawURLEncoding.DecodeString(coseKey)
	if err != nil {
		return err
	}
	signedBytes := signedBytes(authData, clientDataJSON)
	hashed := sha256.Sum256(signedBytes)

	var kty struct {
		KTY int `cbor:"1,keyasint"`
	}
	if err := cbor.Unmarshal(key, &kty); err != nil {
		return fmt.Errorf("cbor.Unmarshal(%q): %w", key, err)
	}
	switch kty.KTY {
	case 2: // ECDSA public key
		var ecKey struct {
			KTY   int    `cbor:"1,keyasint"`
			ALG   int    `cbor:"3,keyasint"`
			Curve int    `cbor:"-1,keyasint"`
			X     []byte `cbor:"-2,keyasint"`
			Y     []byte `cbor:"-3,keyasint"`
		}
		if err := cbor.Unmarshal(key, &ecKey); err != nil {
			return err
		}
		if ecKey.ALG != algES256 {
			return errors.New("unexpected EC key alg")
		}
		if ecKey.Curve != 1 { // P-256
			return errors.New("unexpected EC key curve")
		}
		publicKey := &ecdsa.PublicKey{
			Curve: elliptic.P256(),
			X:     new(big.Int).SetBytes(ecKey.X),
			Y:     new(big.Int).SetBytes(ecKey.Y),
		}
		if !publicKey.Curve.IsOnCurve(publicKey.X, publicKey.Y) {
			return errors.New("invalid public key")
		}
		if !ecdsa.VerifyASN1(publicKey, hashed[:], signature) {
			return errors.New("invalid signature")
		}
		return nil
	case 3: // RSA public key, RSASSA-PKCS1-v1_5
		var rsaKey struct {
			KTY int    `cbor:"1,keyasint"`
			ALG int    `cbor:"3,keyasint"`
			N   []byte `cbor:"-1,keyasint"`
			E   int    `cbor:"-2,keyasint"`
		}
		if err := cbor.Unmarshal(key, &rsaKey); err != nil {
			return err
		}
		if rsaKey.ALG != algRS256 {
			return errors.New("unexpected RSA key alg")
		}
		publicKey := &rsa.PublicKey{
			N: new(big.Int).SetBytes(rsaKey.N),
			E: rsaKey.E,
		}
		if err := rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, hashed[:], signature); err != nil {
			return err
		}
		return nil
	default:
		return errors.New("unsupported key type")
	}
}
