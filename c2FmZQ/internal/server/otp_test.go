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
	"time"

	"github.com/pquerna/otp/totp"
)

func TestOTP(t *testing.T) {
	sock, shutdown := startServer(t)
	defer shutdown()

	c, err := createAccountAndLogin(sock, "alice")
	if err != nil {
		t.Fatalf("createAccountAndLogin failed: %v", err)
	}

	if err := c.registerOTP(); err != nil {
		t.Fatalf("c.registerOTP failed: %v", err)
	}
}

func (c *client) registerOTP() error {
	key, err := c.generateOTP()
	if err != nil {
		return err
	}
	code, err := totp.GenerateCode(key, time.Now())
	if err != nil {
		return err
	}
	return c.setOTP(key, code)
}

func (c *client) generateOTP() (string, error) {
	form := url.Values{}
	form.Set("token", c.token)
	sr, err := c.sendRequest("/v2x/config/generateOTP", form)
	if err != nil {
		return "", err
	}
	return sr.Part("key").(string), nil
}

func (c *client) setOTP(key, code string) error {
	params := map[string]string{
		"key":  key,
		"code": code,
	}
	form := url.Values{}
	form.Set("token", c.token)
	form.Set("params", c.encodeParams(params))
	sr, err := c.sendRequest("/v2x/config/setOTP", form)
	if err != nil {
		return err
	}
	if sr.Status != "ok" {
		return sr
	}
	if len(sr.Infos) != 1 || sr.Infos[0] != "OTP enabled" {
		return errors.New("Expected OTP Enabled")
	}
	c.otpKey = key
	return nil
}
