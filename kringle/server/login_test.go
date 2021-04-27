package server_test

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net/url"
	"reflect"
	"strconv"
	"testing"

	"kringle/stingle"
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
	c.secretKey = stingle.MakeSecretKey()
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
	if want, got := c.salt, sr.Parts["salt"]; want != got {
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
	if want, got := c.keyBundle, sr.Parts["keyBundle"]; want != got {
		return fmt.Errorf("login: unexpected keyBundle: want %q, got %q", want, got)
	}
	if want, got := c.isBackup, sr.Parts["isKeyBackedUp"]; want != got {
		return fmt.Errorf("login: unexpected isKeyBackedUp: want %q, got %q", want, got)
	}
	id, err := strconv.ParseInt(sr.Parts["userId"].(string), 10, 32)
	if err != nil {
		return err
	}
	c.userID = id
	pk, err := base64.StdEncoding.DecodeString(sr.Parts["serverPublicKey"].(string))
	if err != nil {
		return err
	}
	c.serverPublicKey = stingle.PublicKeyFromBytes(pk)
	token, ok := sr.Parts["token"].(string)
	if !ok || token == "" {
		return fmt.Errorf("login: invalid token: %#v", sr.Parts["token"])
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
	if sr.Parts["serverPK"] == nil {
		return fmt.Errorf("server did not return serverPK")
	}
	pk, err := base64.StdEncoding.DecodeString(sr.Parts["serverPK"].(string))
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
	if want, got := c.isBackup, sr.Parts["isKeyBackedUp"]; want != got {
		return fmt.Errorf("checkKey: unexpected isKeyBackedUp: want %q, got %q", want, got)
	}
	pk, err := base64.StdEncoding.DecodeString(sr.Parts["serverPK"].(string))
	if err != nil {
		return err
	}
	if want, got := []byte(c.serverPublicKey.ToBytes()), pk; !reflect.DeepEqual(want, got) {
		return fmt.Errorf("checkKey: unexpected serverPK: want %#v, got %#v", want, got)
	}
	dec, err := c.secretKey.SealBoxOpenBase64(sr.Parts["challenge"].(string))
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
	token, ok := sr.Parts["token"].(string)
	if !ok || token == "" {
		return fmt.Errorf("login: invalid token: %#v", sr.Parts["token"])
	}
	c.token = token
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
	if want, got := "OK", sr.Parts["result"]; want != got {
		return fmt.Errorf("recoverAccount: unexpected result: want %v, got %v", want, got)
	}
	return nil
}
