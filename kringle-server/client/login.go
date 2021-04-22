package client

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"

	"kringle-server/stingle"
)

func (c *Client) CreateAccount(email, password string) error {
	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
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
	if err := c.storage.SaveDataFile(nil, c.storage.HashString(configFile), c); err != nil {
		return err
	}
	fmt.Println("Account created successfully.")
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
	salt, err := hex.DecodeString(sr.Parts["salt"].(string))
	if err != nil {
		return err
	}

	form.Set("password", stingle.PasswordHashForLogin([]byte(password), salt))
	if sr, err = c.sendRequest("/v2/login/login", form); err != nil {
		return err
	}
	if sr.Status != "ok" {
		return sr
	}
	id, err := strconv.ParseInt(sr.Parts["userId"].(string), 10, 32)
	if err != nil {
		return err
	}
	c.UserID = id
	c.Email = email
	pk, err := base64.StdEncoding.DecodeString(sr.Parts["serverPublicKey"].(string))
	if err != nil {
		return err
	}
	c.ServerPublicKey = stingle.PublicKeyFromBytes(pk)
	token, ok := sr.Parts["token"].(string)
	if !ok || token == "" {
		return fmt.Errorf("login: invalid token: %#v", sr.Parts["token"])
	}
	c.Token = token
	if sk, err := stingle.DecodeSecretKeyBundle([]byte(password), sr.Parts["keyBundle"].(string)); err == nil {
		c.SecretKey = sk
	}
	if err := c.storage.SaveDataFile(nil, c.storage.HashString(configFile), c); err != nil {
		return err
	}
	c.storage.CreateEmptyFile(c.fileHash(galleryFile), &FileSet{})
	c.storage.CreateEmptyFile(c.fileHash(trashFile), &FileSet{})
	c.storage.CreateEmptyFile(c.fileHash(albumList), &AlbumList{})
	c.storage.CreateEmptyFile(c.fileHash(contactsFile), &ContactList{})
	fmt.Println("Logged in successfully.")
	return c.GetUpdates()
}
