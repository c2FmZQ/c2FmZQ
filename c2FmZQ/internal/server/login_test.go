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
	"bytes"
	"encoding/base64"
	"fmt"
	"net/url"
	"reflect"
	"strconv"
	"testing"

	"c2FmZQ/internal/stingle"
)

func createAccountAndLogin(sock, email string) (*client, error) {
	c := newClient(sock)

	if err := c.createAccount(email); err != nil {
		return nil, fmt.Errorf("c.createAccount failed: %v", err)
	}
	if err := c.login(); err != nil {
		return nil, fmt.Errorf("c.login failed: %v", err)
	}
	return c, nil
}

func TestPreLoginFakeSalt(t *testing.T) {
	sock, shutdown := startServer(t)
	defer shutdown()
	c := newClient(sock)

	form := url.Values{}
	form.Set("email", "foo")
	sr, err := c.sendRequest("/v2/login/preLogin", form)
	if err != nil || sr.Status != "ok" {
		t.Fatalf("preLogin failed: %v %v", err, sr)
	}
	salt := sr.Part("salt").(string)
	if sr, err = c.sendRequest("/v2/login/preLogin", form); err != nil || sr.Status != "ok" {
		t.Fatalf("preLogin failed: %v %v", err, sr)
	}
	if want, got := salt, sr.Part("salt").(string); want != got {
		t.Errorf("Salt mismatch, want %s, got %s", want, got)
	}
}

func TestLogin(t *testing.T) {
	sock, shutdown := startServer(t)
	defer shutdown()

	c := newClient(sock)
	if err := c.createAccount("alice"); err != nil {
		t.Fatalf("c.createAccount failed: %v", err)
	}
	if err := c.preLogin(); err != nil {
		t.Fatalf("c.preLogin failed: %v", err)
	}
	if err := c.login(); err != nil {
		t.Fatalf("c.login failed: %v", err)
	}
	if err := c.getServerPK(); err != nil {
		t.Fatalf("c.getServerPK failed: %v", err)
	}
	if err := c.checkKey(); err != nil {
		t.Fatalf("c.checkKey failed: %v", err)
	}
	if err := c.changePass(); err != nil {
		t.Fatalf("c.changePass failed: %v", err)
	}
	if err := c.changeEmail(); err != nil {
		t.Fatalf("c.changeEmail failed: %v", err)
	}
	if err := c.recoverAccount(); err != nil {
		t.Fatalf("c.recoverAccount failed: %v", err)
	}

	// Negative tests.
	c.password = "WrongPassword"
	if err := c.login(); err == nil {
		t.Error("c.login should have failed but succeeded")
	}
	c.token = "BadToken"
	if err := c.changePass(); err == nil {
		t.Error("c.changePass should have failed but succeeded")
	}
	if err := c.changeEmail(); err == nil {
		t.Error("c.changeEmail should have failed but succeeded")
	}
	c.secretKey = stingle.MakeSecretKeyForTest()
	if err := c.recoverAccount(); err == nil {
		t.Error("c.recoverAccount should have failed but succeeded")
	}
	if err := c.checkKey(); err == nil {
		t.Error("c.checkKey should have failed but succeeded")
	}
	c.email = "bob"
	if err := c.preLogin(); err == nil {
		t.Error("c.preLogin should have failed but succeeded")
	}
	if err := c.getServerPK(); err == nil {
		t.Error("c.getServerPK should have failed but succeeded")
	}
}

func (c *client) createAccount(email string) error {
	c.email = email
	c.password = "PASSWORD"
	c.salt = "SALT"
	c.keyBundle = stingle.MakeKeyBundle(c.secretKey.PublicKey())
	c.isBackup = "0"
	form := url.Values{}
	form.Set("email", c.email)
	form.Set("password", c.password)
	form.Set("salt", c.salt)
	form.Set("keyBundle", c.keyBundle)
	form.Set("isBackup", c.isBackup)

	sr, err := c.sendRequest("/v2/register/createAccount", form)
	if err != nil {
		return err
	}
	if sr.Status != "ok" {
		return sr
	}
	return nil
}

func (c *client) preLogin() error {
	form := url.Values{}
	form.Set("email", c.email)
	sr, err := c.sendRequest("/v2/login/preLogin", form)
	if err != nil {
		return err
	}
	if sr.Status != "ok" {
		return sr
	}
	if want, got := c.salt, sr.Part("salt"); want != got {
		return fmt.Errorf("preLogin: unexpected salt: want %q, got %q", want, got)
	}
	return nil
}

func (c *client) login() error {
	form := url.Values{}
	form.Set("email", c.email)
	form.Set("password", c.password)
	sr, err := c.sendRequest("/v2/login/login", form)
	if err != nil {
		return err
	}
	if sr.Status != "ok" {
		return sr
	}
	if want, got := c.keyBundle, sr.Part("keyBundle"); want != got {
		return fmt.Errorf("login: unexpected keyBundle: want %q, got %q", want, got)
	}
	if want, got := c.isBackup, sr.Part("isKeyBackedUp"); want != got {
		return fmt.Errorf("login: unexpected isKeyBackedUp: want %q, got %q", want, got)
	}
	id, err := strconv.ParseInt(sr.Part("userId").(string), 10, 32)
	if err != nil {
		return err
	}
	c.userID = id
	pk, err := base64.StdEncoding.DecodeString(sr.Part("serverPublicKey").(string))
	if err != nil {
		return err
	}
	c.serverPublicKey = stingle.PublicKeyFromBytes(pk)
	token, ok := sr.Part("token").(string)
	if !ok || token == "" {
		return fmt.Errorf("login: invalid token: %#v", sr.Part("token"))
	}
	c.token = token
	return nil
}

func (c *client) getServerPK() error {
	form := url.Values{}
	form.Set("token", c.token)
	sr, err := c.sendRequest("/v2/keys/getServerPK", form)
	if err != nil {
		return err
	}
	if sr.Status != "ok" {
		return sr
	}
	if sr.Part("serverPK") == nil {
		return fmt.Errorf("server did not return serverPK")
	}
	pk, err := base64.StdEncoding.DecodeString(sr.Part("serverPK").(string))
	if err != nil {
		return err
	}
	if want, got := []byte(c.serverPublicKey.ToBytes()), pk; !reflect.DeepEqual(want, got) {
		return fmt.Errorf("login: unexpected serverPK: want %#v, got %#v", want, got)
	}
	return nil
}

func (c *client) checkKey() error {
	form := url.Values{}
	form.Set("email", c.email)
	sr, err := c.sendRequest("/v2/login/checkKey", form)
	if err != nil {
		return err
	}
	if sr.Status != "ok" {
		return sr
	}
	if want, got := c.isBackup, sr.Part("isKeyBackedUp"); want != got {
		return fmt.Errorf("checkKey: unexpected isKeyBackedUp: want %q, got %q", want, got)
	}
	pk, err := base64.StdEncoding.DecodeString(sr.Part("serverPK").(string))
	if err != nil {
		return err
	}
	if want, got := []byte(c.serverPublicKey.ToBytes()), pk; !reflect.DeepEqual(want, got) {
		return fmt.Errorf("checkKey: unexpected serverPK: want %#v, got %#v", want, got)
	}
	dec, err := c.secretKey.SealBoxOpenBase64(sr.Part("challenge").(string))
	if err != nil {
		return fmt.Errorf("checkKey challenge error: %v", err)
	}
	if prefix := []byte("validkey_"); !bytes.HasPrefix(dec, prefix) {
		return fmt.Errorf("checkKey challenge has unexpected prefix: %v", dec)
	}
	return nil
}

func (c *client) changePass() error {
	params := make(map[string]string)
	params["newPassword"] = "NEWPASSWORD"
	params["newSalt"] = "NEWSALT"
	params["keyBundle"] = c.keyBundle

	form := url.Values{}
	form.Set("token", c.token)
	form.Set("params", c.encodeParams(params))

	sr, err := c.sendRequest("/v2/login/changePass", form)
	if err != nil {
		return err
	}
	if sr.Status != "ok" {
		return sr
	}
	token, ok := sr.Part("token").(string)
	if !ok || token == "" {
		return fmt.Errorf("login: invalid token: %#v", sr.Part("token"))
	}
	c.token = token
	return nil
}

func (c *client) changeEmail() error {
	params := make(map[string]string)
	params["newEmail"] = "NEWEMAIL"

	form := url.Values{}
	form.Set("token", c.token)
	form.Set("params", c.encodeParams(params))

	sr, err := c.sendRequest("/v2/login/changeEmail", form)
	if err != nil {
		return err
	}
	if sr.Status != "ok" {
		return sr
	}
	email, ok := sr.Part("email").(string)
	if !ok || email == "" {
		return fmt.Errorf("login: invalid email: %#v", sr.Part("email"))
	}
	c.email = email
	return nil
}

func (c *client) recoverAccount() error {
	params := make(map[string]string)
	params["newPassword"] = "NEWPASSWORD"
	params["newSalt"] = "NEWSALT"
	params["keyBundle"] = c.keyBundle

	form := url.Values{}
	form.Set("email", c.email)
	form.Set("params", c.encodeParams(params))

	sr, err := c.sendRequest("/v2/login/recoverAccount", form)
	if err != nil {
		return err
	}
	if sr.Status != "ok" {
		return sr
	}
	if want, got := "OK", sr.Part("result"); want != got {
		return fmt.Errorf("recoverAccount: unexpected result: want %v, got %v", want, got)
	}
	return nil
}
