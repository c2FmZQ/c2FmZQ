//
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

package webpush

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"io"
	"regexp"
	"strings"
	"testing"
)

func TestMatchService(t *testing.T) {
	cfg := DefaultPushServiceConfiguration()
	cfg.Windows.Enable = true
	cfg.Init(nil)
	testcases := []struct {
		ep   string
		want pushServiceID
	}{
		{"https://fcm.googleapis.com/...", google},
		{"https://updates.push.services.mozilla.com/...", mozilla},
		{"https://whatever.notify.windows.com/...", windows},
	}
	for _, tc := range testcases {
		got, _, err := cfg.matchService(tc.ep)
		if err != nil {
			t.Errorf("matchService(%q) failed: %v", tc.ep, err)
		}
		if got != tc.want {
			t.Errorf("matchService(%q) = %#v, want %#v", tc.ep, got, tc.want)
		}
	}
}

func TestMakeRequest(t *testing.T) {
	cfg := DefaultPushServiceConfiguration()
	cfg.Google.Regexp = "^https://"
	cfg.Init(nil)
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa.GenerateKey: %v", err)
	}
	aspk, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("x509.MarshalECPrivateKey: %v", err)
	}

	curve := ecdh.P256()
	clientKey, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("curve.GenerateKey: %v", err)
	}

	authBytes := make([]byte, 12)
	if _, err := rand.Read(authBytes); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}

	params := Params{
		Endpoint:                    "https://foo/",
		ApplicationServerPrivateKey: base64.RawURLEncoding.EncodeToString(aspk),
		ApplicationServerPublicKey:  base64.RawURLEncoding.EncodeToString(elliptic.Marshal(key.PublicKey.Curve, key.PublicKey.X, key.PublicKey.Y)),
		Auth:                        base64.RawURLEncoding.EncodeToString(authBytes),
		P256dh:                      base64.RawURLEncoding.EncodeToString(clientKey.PublicKey().Bytes()),
		Payload:                     []byte("Hello World!"),
	}

	req, err := cfg.makeRequest(context.Background(), params)
	if err != nil {
		t.Fatalf("makeRequest: %v", err)
	}

	// Decrypt payload.
	payload, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("body: %v", err)
	}

	var salt []byte
	if h := req.Header.Get("Encryption"); strings.HasPrefix(h, "salt=") {
		if salt, err = base64.RawURLEncoding.DecodeString(h[5:]); err != nil {
			t.Fatalf("salt: %v", err)
		}
	} else {
		t.Fatalf("cannot find salt: %q", h)
	}
	var pubKeyBytes []byte
	if m := regexp.MustCompile(`dh=([^;]*);?`).FindStringSubmatch(req.Header.Get("Crypto-Key")); len(m) == 2 {
		if pubKeyBytes, err = base64.RawURLEncoding.DecodeString(m[1]); err != nil {
			t.Fatalf("pk: %v", err)
		}
	} else {
		t.Fatalf("cannot find pubKeyBytes: %q", m)
	}
	pubKey, err := curve.NewPublicKey(pubKeyBytes)
	if err != nil {
		t.Fatalf("curve.NewPublicKey: %v", err)
	}
	sharedSecret, err := curve.ECDH(clientKey, pubKey)
	if err != nil {
		t.Fatalf("shareSecret: %v", err)
	}

	prk := hkdf(authBytes, sharedSecret, []byte("Content-Encoding: auth\x00"), 32)
	cekInfo := createInfo("aesgcm", clientKey.PublicKey().Bytes(), pubKeyBytes)
	cek := hkdf(salt, prk, cekInfo, 16)
	nonceInfo := createInfo("nonce", clientKey.PublicKey().Bytes(), pubKeyBytes)
	nonce := hkdf(salt, prk, nonceInfo, 12)

	block, err := aes.NewCipher(cek)
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatalf("NewGCM: %v", err)
	}
	decPayload, err := gcm.Open(nil, nonce, payload, nil)
	if err != nil {
		t.Fatalf("gcm.Open: %v", err)
	}
	l := binary.BigEndian.Uint16(decPayload[:2])
	decPayload = decPayload[2+l:]
	if !bytes.Equal(decPayload, []byte("Hello World!")) {
		t.Fatalf("Unexpected decrypted message: %v", decPayload)
	}
}
