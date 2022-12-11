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
	"errors"
	"net/url"
	"testing"
)

func TestMFACheckOTP(t *testing.T) {
	sock, shutdown := startServer(t)
	defer shutdown()

	c, err := createAccountAndLogin(sock, "alice")
	if err != nil {
		t.Fatalf("createAccountAndLogin failed: %v", err)
	}
	if err := c.mfaCheck(false); err == nil {
		t.Fatal("mfaCheck should have failed")
	}
	if err := c.registerOTP(); err != nil {
		t.Fatalf("c.registerOTP failed: %v", err)
	}
	if err := c.mfaCheck(false); err != nil {
		t.Fatalf("mfaCheck: %v", err)
	}
}

func TestMFACheckWebAuthn(t *testing.T) {
	sock, shutdown := startServer(t)
	defer shutdown()

	c, err := createAccountAndLogin(sock, "alice")
	if err != nil {
		t.Fatalf("createAccountAndLogin failed: %v", err)
	}
	if err := c.mfaCheck(false); err == nil {
		t.Fatal("mfaCheck should have failed")
	}
	if err := c.registerSecurityKey("testkey", false); err != nil {
		t.Fatalf("c.registerSecurityKey failed: %v", err)
	}
	if err := c.mfaCheck(false); err != nil {
		t.Fatalf("mfaCheck: %v", err)
	}
}

func TestMFACheckWebAuthnPasskey(t *testing.T) {
	sock, shutdown := startServer(t)
	defer shutdown()

	c, err := createAccountAndLogin(sock, "alice")
	if err != nil {
		t.Fatalf("createAccountAndLogin failed: %v", err)
	}
	if err := c.mfaCheck(true); err == nil {
		t.Fatal("mfaCheck should have failed")
	}
	if err := c.registerSecurityKey("testkey", true); err != nil {
		t.Fatalf("c.registerSecurityKey failed: %v", err)
	}
	if err := c.mfaCheck(true); err != nil {
		t.Fatalf("mfaCheck: %v", err)
	}
}

func TestMFALoginOK(t *testing.T) {
	sock, shutdown := startServer(t)
	defer shutdown()

	c, err := createAccountAndLogin(sock, "alice")
	if err != nil {
		t.Fatalf("createAccountAndLogin failed: %v", err)
	}
	if err := c.registerSecurityKey("testkey", false); err != nil {
		t.Fatalf("c.registerSecurityKey failed: %v", err)
	}
	if err := c.mfaCheck(false); err != nil {
		t.Fatalf("mfaCheck: %v", err)
	}
	if err := c.mfaEnable(true, false); err != nil {
		t.Fatalf("mfaEnable: %v", err)
	}
	if err := c.login(); err != nil {
		t.Fatalf("login failed: %v", err)
	}

	c.authenticator.RotateKeys()
	// Now the signature should be invalid.
	if err := c.login(); err == nil {
		t.Fatal("login should have failed")
	}
}

func TestEnableMFARequiresMFA(t *testing.T) {
	sock, shutdown := startServer(t)
	defer shutdown()

	c, err := createAccountAndLogin(sock, "alice")
	if err != nil {
		t.Fatalf("createAccountAndLogin failed: %v", err)
	}
	if err := c.mfaEnable(true, false); err == nil {
		t.Fatal("mfaEnable should have failed")
	}
}

func (c *client) mfaCheck(passKey bool) error {
	params := map[string]string{
		"passKey": "0",
	}
	if passKey {
		params["passKey"] = "1"
	}
	form := url.Values{}
	form.Set("token", c.token)
	form.Set("params", c.encodeParams(params))
	sr, err := c.sendRequest("/v2x/mfa/check", form)
	if err != nil {
		return err
	}
	if sr.Status != "ok" {
		return sr
	}
	if len(sr.Infos) == 0 || sr.Infos[0] != "MFA OK" {
		return errors.New("Expected MFA OK")
	}
	return nil
}

func (c *client) mfaEnable(v, passKey bool) error {
	params := map[string]string{
		"requireMFA": "1",
		"passKey":    "1",
	}
	if !v {
		params["requireMFA"] = "0"
	}
	if !passKey {
		params["passKey"] = "0"
	}
	form := url.Values{}
	form.Set("token", c.token)
	form.Set("params", c.encodeParams(params))
	sr, err := c.sendRequest("/v2x/mfa/enable", form)
	if err != nil {
		return err
	}
	if sr.Status != "ok" {
		return sr
	}
	return nil
}
