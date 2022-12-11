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

package server_test

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"testing"

	"c2FmZQ/internal/stingle"
	"c2FmZQ/internal/webauthn"
)

func TestWebAuthn(t *testing.T) {
	sock, shutdown := startServer(t)
	defer shutdown()

	c, err := createAccountAndLogin(sock, "alice")
	if err != nil {
		t.Fatalf("createAccountAndLogin failed: %v", err)
	}

	if err := c.registerSecurityKey("testkey", false); err != nil {
		t.Fatalf("c.registerSecurityKey failed: %v", err)
	}

	keys, err := c.webAuthnKeys()
	if err != nil {
		t.Fatalf("c.webAuthnKeys: %v", keys)
	}
	if len(keys) != 1 {
		t.Fatalf("Expected one key: %#v", keys)
	}
	if want, got := "testkey", keys[0].Name; want != got {
		t.Fatalf("Unexpected key name. Got %q, want %q", got, want)
	}
}

func (c *client) registerSecurityKey(name string, passKey bool) error {
	params := map[string]string{
		"passKey": "0",
	}
	if passKey {
		params["passKey"] = "1"
	}
	res := c.webAuthnRegister(params)
	sr, ok := res.(*stingle.Response)
	if !ok || sr.Status != "ok" {
		return fmt.Errorf("c.webAuthnRegister: %v", res)
	}
	b, err := json.Marshal(sr.Part("attestationOptions"))
	if err != nil {
		return err
	}
	var ao webauthn.AttestationOptions
	if err := json.Unmarshal(b, &ao); err != nil {
		return err
	}
	clientDataJSON, attestationObject, err := c.authenticator.Create(&ao)
	if err != nil {
		return err
	}
	params = map[string]string{
		"keyName":           name,
		"clientDataJSON":    base64.RawURLEncoding.EncodeToString(clientDataJSON),
		"attestationObject": base64.RawURLEncoding.EncodeToString(attestationObject),
		"discoverable":      "0",
	}
	if passKey {
		params["discoverable"] = "1"
	}
	res = c.webAuthnRegister(params)
	if _, ok := res.(*stingle.Response); !ok || sr.Status != "ok" {
		return fmt.Errorf("c.webAuthnRegister: %#v", res)
	}
	return nil
}

func (c *client) webAuthnRegister(params map[string]string) error {
	form := url.Values{}
	form.Set("token", c.token)
	if params != nil {
		form.Set("params", c.encodeParams(params))
	}
	sr, err := c.sendRequest("/v2x/config/webauthn/register", form)
	if err != nil {
		return err
	}
	return sr
}

type webAuthnKey struct {
	Name string `json:"name"`
	ID   string `json:"id"`
}

func (c *client) webAuthnKeys() ([]webAuthnKey, error) {
	form := url.Values{}
	form.Set("token", c.token)
	sr, err := c.sendRequest("/v2x/config/webauthn/keys", form)
	if err != nil {
		return nil, err
	}
	raw := sr.Part("keys")
	b, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	var keys []webAuthnKey
	if err := json.Unmarshal(b, &keys); err != nil {
		return nil, err
	}
	return keys, nil
}
