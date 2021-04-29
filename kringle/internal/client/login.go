package client

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"kringle/internal/log"
	"kringle/internal/stingle"
)

func (c *Client) CreateAccount(email, password string) error {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return err
	}
	sk := stingle.MakeSecretKey()

	form := url.Values{}
	form.Set("email", email)
	form.Set("password", stingle.PasswordHashForLogin([]byte(password), salt))
	form.Set("salt", strings.ToUpper(hex.EncodeToString(salt)))
	form.Set("keyBundle", stingle.MakeSecretKeyBundle([]byte(password), sk))
	form.Set("isBackup", "1")

	sr, err := c.sendRequest("/v2/register/createAccount", form)
	if err != nil {
		return err
	}
	if sr.Status != "ok" {
		return sr
	}
	c.Email = email
	c.SecretKey = sk
	c.Salt = salt
	if err := c.Save(); err != nil {
		return err
	}
	fmt.Fprintln(c.writer, "Account created successfully.")
	return nil
}

func (c *Client) Login(email, password string) error {
	form := url.Values{}
	form.Set("email", email)
	sr, err := c.sendRequest("/v2/login/preLogin", form)
	if err != nil {
		return err
	}
	if sr.Status != "ok" {
		return sr
	}
	salt, err := hex.DecodeString(sr.Part("salt").(string))
	if err != nil {
		return err
	}
	if len(salt) == 0 {
		log.Debugf("PreLogin: salt is empty: %#v", sr)
		salt = c.Salt
	}

	form.Set("password", stingle.PasswordHashForLogin([]byte(password), salt))
	if sr, err = c.sendRequest("/v2/login/login", form); err != nil {
		return err
	}
	if sr.Status != "ok" {
		return sr
	}
	id, err := strconv.ParseInt(sr.Part("userId").(string), 10, 32)
	if err != nil {
		return err
	}
	c.UserID = id
	c.Email = email
	pk, err := base64.StdEncoding.DecodeString(sr.Part("serverPublicKey").(string))
	if err != nil {
		return err
	}
	c.ServerPublicKey = stingle.PublicKeyFromBytes(pk)
	token, ok := sr.Part("token").(string)
	if !ok || token == "" {
		return fmt.Errorf("login: invalid token: %#v", sr.Part("token"))
	}
	c.Token = token
	if sk, err := stingle.DecodeSecretKeyBundle([]byte(password), sr.Part("keyBundle").(string)); err == nil {
		c.SecretKey = sk
	}
	c.storage.CreateEmptyFile(c.fileHash(galleryFile), &FileSet{})
	c.storage.CreateEmptyFile(c.fileHash(trashFile), &FileSet{})
	c.storage.CreateEmptyFile(c.fileHash(albumList), &AlbumList{})
	c.storage.CreateEmptyFile(c.fileHash(contactsFile), &ContactList{})
	if err := c.Save(); err != nil {
		return err
	}
	fmt.Fprintln(c.writer, "Logged in successfully.")
	return nil
}

func (c *Client) Logout() error {
	form := url.Values{}
	form.Set("token", c.Token)
	sr, err := c.sendRequest("/v2/login/logout", form)
	if err != nil {
		return err
	}
	if sr.Status != "ok" {
		return sr
	}
	c.Email = ""
	c.Token = ""
	if err := c.Save(); err != nil {
		return err
	}
	fmt.Fprintln(c.writer, "Logged out successfully.")
	return nil
}
