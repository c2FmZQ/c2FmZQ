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

package client

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/tyler-smith/go-bip39"

	"c2FmZQ/internal/log"
	"c2FmZQ/internal/stingle"
)

const (
	backupWarning = `
	WARNING: Secret key backup is disabled. Make sure to save a copy of the
	backup phrase. You will need it if you forget your password, or if you
	need to login again.
`
)

// CreateAccount creates a new account on the remote server.
func (c *Client) CreateAccount(server, email, password string, doBackup bool) error {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return err
	}
	sk := stingle.MakeSecretKey()
	defer sk.Wipe()
	bundle := stingle.MakeKeyBundle(sk.PublicKey())
	if doBackup {
		bundle = stingle.MakeSecretKeyBundle([]byte(password), sk)
	}
	pw := stingle.PasswordHashForLogin([]byte(password), salt)
	form := url.Values{}
	form.Set("email", email)
	form.Set("password", pw)
	form.Set("salt", strings.ToUpper(hex.EncodeToString(salt)))
	form.Set("keyBundle", bundle)
	form.Set("isBackup", "0")
	if doBackup {
		form.Set("isBackup", "1")
	}

	sr, err := c.sendRequest("/v2/register/createAccount", form, server)
	if err != nil {
		return err
	}
	if sr.Status != "ok" {
		return sr
	}

	c.Account = &AccountInfo{
		Email:          email,
		SecretKey:      c.encryptSK(sk),
		Salt:           salt,
		HashedPassword: pw,
		IsBackedUp:     doBackup,
		ServerBaseURL:  server,
	}
	c.createEmptyFiles()

	if err := c.Save(); err != nil {
		return err
	}
	c.Print("Account created successfully.")
	if !doBackup {
		c.Print(backupWarning)
	}
	if _, err := c.sendLogin(email, pw); err != nil {
		return err
	}
	if err := c.Save(); err != nil {
		return err
	}
	c.Print("Logged in successfully.")
	return nil
}

// BackupPhrase returns the backup phrase for the secret key. This is
// effectively *the* secret key.
func (c *Client) BackupPhrase(password string) error {
	if c.Account == nil {
		c.Print("Not logged in.")
		return nil
	}
	if err := c.checkPassword(password); err != nil {
		return err
	}
	sk := c.SecretKey()
	defer sk.Wipe()
	phr, err := bip39.NewMnemonic(sk.ToBytes())
	if err != nil {
		return err
	}
	c.Printf("Backup phrase:\n\n%s\n\n", phr)
	return nil
}

// Login logs in to the remote server.
func (c *Client) Login(server, email, password string) error {
	form := url.Values{}
	form.Set("email", email)
	sr, err := c.sendRequest("/v2/login/preLogin", form, server)
	if err != nil {
		return err
	}
	if sr.Status != "ok" {
		return sr
	}
	eSalt, ok := sr.Part("salt").(string)
	if !ok {
		return fmt.Errorf("salt has unexpected type: %T", sr.Part("salt"))
	}
	salt, err := hex.DecodeString(eSalt)
	if err != nil {
		return err
	}
	pw := stingle.PasswordHashForLogin([]byte(password), salt)

	c.Account = &AccountInfo{
		Email:          email,
		Salt:           salt,
		HashedPassword: pw,
		ServerBaseURL:  server,
	}

	if sr, err = c.sendLogin(email, pw); err != nil {
		return err
	}
	keyBundle, ok := sr.Part("keyBundle").(string)
	if !ok {
		return fmt.Errorf("keyBundle has unexpected type: %T", sr.Part("keyBundle"))
	}
	sk, err := stingle.DecodeSecretKeyBundle([]byte(password), keyBundle)
	defer sk.Wipe()
	if err != nil {
		c.Account.IsBackedUp = false
		phr, err := c.prompt("Enter backup phrase: ")
		if err != nil {
			return err
		}
		b, err := bip39.EntropyFromMnemonic(phr)
		if err != nil {
			return err
		}
		sk = stingle.SecretKeyFromBytes(b)
		if err := c.checkKey(server, email, sk); err != nil {
			return err
		}
	}

	c.Account.SecretKey = c.encryptSK(sk)
	c.createEmptyFiles()

	if err := c.Save(); err != nil {
		return err
	}
	c.Print("Logged in successfully.")
	return nil
}

func (c *Client) sendLogin(email, hashedPassword string) (*stingle.Response, error) {
	form := url.Values{}
	form.Set("email", email)
	form.Set("password", hashedPassword)
	sr, err := c.sendRequest("/v2/login/login", form, "")
	if err != nil {
		return nil, err
	}
	if sr.Status != "ok" {
		return nil, sr
	}
	userID, ok := sr.Part("userId").(string)
	if !ok {
		log.Errorf("userId has unexpected type: %T", sr.Part("userId"))
	}
	id, err := strconv.ParseInt(userID, 10, 32)
	if err != nil {
		return nil, err
	}
	serverPublicKey, ok := sr.Part("serverPublicKey").(string)
	if !ok {
		log.Errorf("serverPublicKey has unexpected type: %T", sr.Part("serverPublicKey"))
	}
	pk, err := base64.StdEncoding.DecodeString(serverPublicKey)
	if err != nil {
		return nil, err
	}
	token, ok := sr.Part("token").(string)
	if !ok || token == "" {
		return nil, fmt.Errorf("login: invalid token: %#v", sr.Part("token"))
	}

	c.Account.Email = email
	c.Account.HashedPassword = hashedPassword
	c.Account.Token = token
	c.Account.UserID = id
	c.Account.ServerPublicKey = stingle.PublicKeyFromBytes(pk)
	c.Account.IsBackedUp = true
	return sr, nil
}

// Logout logs out from the remote server.
func (c *Client) Logout() error {
	if c.Account == nil {
		return ErrNotLoggedIn
	}
	form := url.Values{}
	form.Set("token", c.Account.Token)
	sr, err := c.sendRequest("/v2/login/logout", form, "")
	if err != nil {
		return err
	}
	if sr.Status != "ok" {
		return sr
	}
	c.Account = nil
	if err := c.Save(); err != nil {
		return err
	}
	c.Print("Logged out successfully.")
	return nil
}

func (c *Client) checkPassword(password string) error {
	if c.Account == nil {
		return ErrNotLoggedIn
	}
	if c.Account.HashedPassword != stingle.PasswordHashForLogin([]byte(password), c.Account.Salt) {
		return errors.New("invalid password")
	}
	return nil
}

// ChangePassword changes the user's password.
func (c *Client) ChangePassword(password, newPassword string, doBackup bool) error {
	if err := c.checkPassword(password); err != nil {
		return err
	}
	sk := c.SecretKey()
	defer sk.Wipe()
	bundle := stingle.MakeKeyBundle(sk.PublicKey())
	if doBackup {
		bundle = stingle.MakeSecretKeyBundle([]byte(newPassword), sk)
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return err
	}
	pw := stingle.PasswordHashForLogin([]byte(newPassword), salt)

	params := make(map[string]string)
	params["newPassword"] = pw
	params["newSalt"] = strings.ToUpper(hex.EncodeToString(salt))
	params["keyBundle"] = bundle
	form := url.Values{}
	form.Set("token", c.Account.Token)
	form.Set("params", c.encodeParams(params))

	sr, err := c.sendRequest("/v2/login/changePass", form, "")
	if err != nil {
		return err
	}
	if sr.Status != "ok" {
		return sr
	}
	tok, ok := sr.Part("token").(string)
	if !ok || tok == "" {
		return errors.New("missing new token")
	}

	c.Account.Token = tok
	c.Account.Salt = salt
	c.Account.HashedPassword = pw
	c.Account.SecretKey = c.encryptSK(sk)
	c.Account.IsBackedUp = doBackup

	if err := c.Save(); err != nil {
		return err
	}
	c.Print("Password changed successfully.")
	if !doBackup {
		c.Print(backupWarning)
	}
	return nil
}

// RecoverAccount recovers an account using the backup phrase.
func (c *Client) RecoverAccount(server, email, newPassword, backupPhrase string, doBackup bool) error {
	b, err := bip39.EntropyFromMnemonic(backupPhrase)
	if err != nil {
		return err
	}
	sk := stingle.SecretKeyFromBytes(b)
	defer sk.Wipe()
	if err := c.checkKey(server, email, sk); err != nil {
		return err
	}
	bundle := stingle.MakeKeyBundle(sk.PublicKey())
	if doBackup {
		bundle = stingle.MakeSecretKeyBundle([]byte(newPassword), sk)
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return err
	}
	pw := stingle.PasswordHashForLogin([]byte(newPassword), salt)

	params := make(map[string]string)
	params["newPassword"] = pw
	params["newSalt"] = strings.ToUpper(hex.EncodeToString(salt))
	params["keyBundle"] = bundle

	form := url.Values{}
	form.Set("email", email)
	form.Set("params", c.encodeParams(params))

	sr, err := c.sendRequest("/v2/login/recoverAccount", form, "")
	if err != nil {
		return err
	}
	if sr.Status != "ok" {
		return sr
	}
	if sr.Part("result") != "OK" {
		return errors.New("result not OK")
	}
	c.Account = &AccountInfo{
		Email:          email,
		Salt:           salt,
		HashedPassword: pw,
		SecretKey:      c.encryptSK(sk),
		IsBackedUp:     doBackup,
		ServerBaseURL:  server,
	}
	c.createEmptyFiles()
	if err := c.Save(); err != nil {
		return err
	}
	c.Print("Account recovered successfully.")

	if _, err := c.sendLogin(email, pw); err != nil {
		return err
	}
	if err := c.Save(); err != nil {
		return err
	}
	c.Print("Logged in successfully.")
	if !doBackup {
		c.Print(backupWarning)
	}
	return nil
}

// DeleteAccount deletes the account on the remote server.
func (c *Client) DeleteAccount(password string) error {
	if err := c.checkPassword(password); err != nil {
		return err
	}
	params := make(map[string]string)
	params["password"] = stingle.PasswordHashForLogin([]byte(password), c.Account.Salt)

	form := url.Values{}
	form.Set("token", c.Account.Token)
	form.Set("params", c.encodeParams(params))

	sr, err := c.sendRequest("/v2/login/deleteUser", form, "")
	if err != nil {
		return err
	}
	if sr.Status != "ok" {
		return sr
	}
	if err := c.WipeAccount(password); err != nil {
		return err
	}
	c.Account = nil
	if err := c.Save(); err != nil {
		return err
	}
	c.Print("Account deleted successfully.")
	return nil
}

// UploadKeys uploads the users keybundle again.
func (c *Client) UploadKeys(password string, doBackup bool) error {
	if err := c.checkPassword(password); err != nil {
		return err
	}
	sk := c.SecretKey()
	defer sk.Wipe()
	bundle := stingle.MakeKeyBundle(sk.PublicKey())
	if doBackup {
		bundle = stingle.MakeSecretKeyBundle([]byte(password), sk)
	}
	c.Account.IsBackedUp = doBackup

	params := make(map[string]string)
	params["keyBundle"] = bundle

	form := url.Values{}
	form.Set("token", c.Account.Token)
	form.Set("params", c.encodeParams(params))

	sr, err := c.sendRequest("/v2/keys/reuploadKeys", form, "")
	if err != nil {
		return err
	}
	if sr.Status != "ok" {
		return sr
	}
	if err := c.Save(); err != nil {
		return err
	}
	if doBackup {
		c.Print("Secret key backup enabled.")
	} else {
		c.Print("Secret key backup disabled.")
		c.Print(backupWarning)
	}
	return nil
}

func (c *Client) checkKey(server, email string, sk *stingle.SecretKey) error {
	form := url.Values{}
	form.Set("email", email)
	sr, err := c.sendRequest("/v2/login/checkKey", form, server)
	if err != nil {
		return err
	}
	if sr.Status != "ok" {
		return sr
	}
	challenge, ok := sr.Part("challenge").(string)
	if !ok {
		return errors.New("invalid challenge")
	}
	dec, err := sk.SealBoxOpenBase64(challenge)
	if err != nil || !bytes.HasPrefix(dec, []byte("validkey_")) {
		return errors.New("wrong key")
	}
	return nil
}
