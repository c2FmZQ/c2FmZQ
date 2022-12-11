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

package webauthn

import (
	"bytes"
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

	cbor "github.com/fxamacker/cbor/v2"
)

// FakeAuthenticator mimics the behavior of a WebAuthn authenticator for testing.
type FakeAuthenticator struct {
	keys     map[string]fakeAuthKey
	rpIDHash []byte
}

type fakeAuthKey struct {
	id         []byte
	uid        []byte
	rk         bool
	privateKey crypto.Signer
	signCount  uint32
}

// NewFakeAuthenticator returns a new FakeAuthenticator for testing.
func NewFakeAuthenticator() (*FakeAuthenticator, error) {
	return &FakeAuthenticator{
		keys: make(map[string]fakeAuthKey),
	}, nil
}

// Create mimics the behavior of the WebAuthn create call.
func (a *FakeAuthenticator) Create(options *AttestationOptions) (clientDataJSON, attestationObject []byte, err error) {
	var authKey fakeAuthKey
	var coseKey []byte
	switch options.PubKeyCredParams[0].Alg {
	case algES256:
		privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return nil, nil, err
		}
		if coseKey, err = es256CoseKey(privKey.PublicKey); err != nil {
			return nil, nil, err
		}
		authKey.privateKey = privKey
	case algRS256:
		privKey, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			return nil, nil, err
		}
		if coseKey, err = rs256CoseKey(privKey.PublicKey); err != nil {
			return nil, nil, err
		}
		authKey.privateKey = privKey
	default:
		return nil, nil, errors.New("unexpected options.PubKeyCredParams alg")
	}
	cd := ClientData{
		Type:      "webauthn.create",
		Challenge: options.Challenge,
		Origin:    "https://example.com/",
	}
	if clientDataJSON, err = json.Marshal(cd); err != nil {
		return nil, nil, err
	}

	uid, err := base64.RawURLEncoding.DecodeString(options.User.ID)
	if err != nil {
		return nil, nil, err
	}
	authKey.uid = uid
	authKey.rk = options.AuthenticatorSelection.RequireResidentKey

	authKey.id = make([]byte, 32)
	if _, err := rand.Read(authKey.id); err != nil {
		return nil, nil, err
	}
	rpIDHash := sha256.Sum256([]byte(options.RelyingParty.ID))
	a.rpIDHash = rpIDHash[:]

	authData, err := authKey.makeAuthData(a.rpIDHash, coseKey)
	if err != nil {
		return nil, nil, err
	}
	att := Attestation{
		Format:      "none",
		RawAuthData: authData,
	}
	if attestationObject, err = cbor.Marshal(att); err != nil {
		return nil, nil, err
	}
	a.keys[base64.RawURLEncoding.EncodeToString(authKey.id)] = authKey
	return
}

// Get mimics the behavior of the WebAuthn create call.
func (a *FakeAuthenticator) Get(options *AssertionOptions) (id string, clientDataJSON, authData, signature, userHandle []byte, err error) {
	var authKey fakeAuthKey
	if len(options.AllowCredentials) > 0 {
		for _, k := range options.AllowCredentials {
			if ak, ok := a.keys[k.ID]; ok {
				id = k.ID
				authKey = ak
				break
			}
		}
	} else {
		for kid, key := range a.keys {
			if key.rk {
				id = kid
				authKey = key
				userHandle = key.uid
				break
			}
		}
	}
	if id == "" {
		err = errors.New("key not found")
		return
	}
	cd := ClientData{
		Type:      "webauthn.get",
		Challenge: options.Challenge,
		Origin:    "https://example.com/",
	}
	if clientDataJSON, err = json.Marshal(cd); err != nil {
		return
	}
	authKey.signCount++
	if authData, err = authKey.makeAuthData(a.rpIDHash, nil); err != nil {
		return
	}
	signature, err = sign(authKey, authData, clientDataJSON)
	return
}

func (a *FakeAuthenticator) RotateKeys() error {
	for k, v := range a.keys {
		privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return err
		}
		v.privateKey = privKey
		a.keys[k] = v
	}
	return nil
}

func (k *fakeAuthKey) makeAuthData(rpIDHash, coseKey []byte) ([]byte, error) {
	var buf bytes.Buffer
	buf.Write(rpIDHash)

	var bits uint8
	bits |= 1      // UP
	bits |= 1 << 2 // UV
	if coseKey != nil {
		bits |= 1 << 6 // AT
	}
	buf.Write([]byte{bits})
	binary.Write(&buf, binary.BigEndian, k.signCount)

	if coseKey != nil {
		var aaguid [16]byte
		buf.Write(aaguid[:])
		binary.Write(&buf, binary.BigEndian, uint16(len(k.id)))
		buf.Write(k.id)
		buf.Write(coseKey)
	}
	return buf.Bytes(), nil
}

// es256CoseKey converts a ECDSA public key to COSE.
func es256CoseKey(publicKey ecdsa.PublicKey) ([]byte, error) {
	if publicKey.Curve != elliptic.P256() {
		return nil, errors.New("unexpected EC curve")
	}
	ecKey := struct {
		KTY   int    `cbor:"1,keyasint"`
		ALG   int    `cbor:"3,keyasint"`
		Curve int    `cbor:"-1,keyasint"`
		X     []byte `cbor:"-2,keyasint"`
		Y     []byte `cbor:"-3,keyasint"`
	}{
		KTY:   2,
		ALG:   algES256,
		Curve: 1, // P-256
		X:     publicKey.X.Bytes(),
		Y:     publicKey.Y.Bytes(),
	}
	return cbor.Marshal(ecKey)
}

// rs256CoseKey converts a RSA public key to COSE.
func rs256CoseKey(publicKey rsa.PublicKey) ([]byte, error) {
	rsaKey := struct {
		KTY int    `cbor:"1,keyasint"`
		ALG int    `cbor:"3,keyasint"`
		N   []byte `cbor:"-1,keyasint"`
		E   int    `cbor:"-2,keyasint"`
	}{
		KTY: 3,
		ALG: algRS256,
		N:   publicKey.N.Bytes(),
		E:   publicKey.E,
	}
	return cbor.Marshal(rsaKey)
}

func sign(authKey fakeAuthKey, authData, clientDataJSON []byte) ([]byte, error) {
	signedBytes := signedBytes(authData, clientDataJSON)
	hashed := sha256.Sum256(signedBytes)
	return authKey.privateKey.Sign(rand.Reader, hashed[:], crypto.SHA256)
}
