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

//go:build selenium
// +build selenium

package jsclient_test

import (
	"fmt"
	"net"
	"path/filepath"
	"testing"

	"github.com/tebeka/selenium"
	"github.com/tebeka/selenium/chrome"

	"c2FmZQ/internal/database"
	"c2FmZQ/internal/log"
	"c2FmZQ/internal/server"
)

func startServer(t *testing.T) (string, func()) {
	testdir := t.TempDir()
	log.Record = t.Log
	log.Level = 3
	db := database.New(filepath.Join(testdir, "data"), []byte("secret"))
	s := server.New(db, "", "", "")
	s.AllowCreateAccount = true
	s.AutoApproveNewAccounts = true
	s.EnableWebApp = true
	l, err := net.Listen("tcp", "devtest:0")
	if err != nil {
		t.Fatalf("net.Listen failed: %v", err)
	}
	go s.RunWithListener(l)
	return l.Addr().String(), func() {
		s.Shutdown()
		log.Record = nil
	}
}

func TestRegisterRecoverLogin(t *testing.T) {
	addr, stop := startServer(t)
	defer stop()
	url := fmt.Sprintf("http://%s/", addr)
	t.Logf("Server running on %s", url)

	caps := selenium.Capabilities{"browserName": "chrome"}
	caps.AddChrome(chrome.Capabilities{
		Path: "/usr/bin/google-chrome",
		Args: []string{
			"--no-sandbox",
			"--allow-insecure-localhost",
			"--unsafely-treat-insecure-origin-as-secure=" + url,
		},
	})
	wd, err := selenium.NewRemote(caps, "http://chrome:4444/wd/hub")
	if err != nil {
		t.Fatalf("selenium.NewRemote: %v", err)
	}
	defer wd.Quit()
	if err := wd.Get(url); err != nil {
		t.Fatalf("wd.Get: %v", err)
	}

	h := helper{t, wd}

	t.Log("Setting passphrase")
	h.sendKeys("#passphrase-input", "hello\n")

	t.Log("Creating new account")
	h.click("#register-tab")
	h.sendKeys("#email-input", "test@c2fmzq.org")
	h.sendKeys("#password-input", "foobar")
	h.sendKeys("#password-input2", "foobar")
	h.click("#login-button")
	h.waitFor("#gallery")

	t.Log("Getting backup phrase")
	h.click("#loggedin-account")
	h.click("#account-menu-key-backup")
	h.sendKeys("#key-backup-password", "foobar")
	h.click("#backup-phrase-show-button")

	var backupPhrase string
	wd.Wait(func(wd selenium.WebDriver) (bool, error) {
		v, err := h.css("#backup-phrase-value").Text()
		if err != nil || v == "" {
			return false, nil
		}
		backupPhrase = v
		return true, nil
	})
	h.click(".popup-close")

	t.Log("Logging out")
	h.click("#loggedin-account")
	h.click("#account-menu-logout")

	t.Log("Recovering account")
	h.click("#recover-tab")
	h.sendKeys("#email-input", "test@c2fmzq.org")
	h.sendKeys("#backup-phrase-input", backupPhrase)
	h.sendKeys("#password-input", "foobar2")
	h.sendKeys("#password-input2", "foobar2")
	h.click("#login-button")
	h.waitFor("#gallery")

	t.Log("Logging out")
	h.click("#loggedin-account")
	h.click("#account-menu-logout")

	t.Log("Logging in")
	h.click("#login-tab")
	h.sendKeys("#email-input", "test@c2fmzq.org")
	h.sendKeys("#password-input", "foobar2")
	h.click("#login-button")
	h.waitFor("#gallery")

	t.Log("Done")
}

type helper struct {
	t  *testing.T
	wd selenium.WebDriver
}

func (h *helper) css(s string) selenium.WebElement {
	e, err := h.wd.FindElement(selenium.ByCSSSelector, s)
	if err != nil {
		h.t.Fatalf("%s: %v", s, err)
	}
	return e
}

func (h *helper) waitFor(sel string) selenium.WebElement {
	var e selenium.WebElement
	h.wd.Wait(func(wd selenium.WebDriver) (bool, error) {
		var err error
		if e, err = wd.FindElement(selenium.ByCSSSelector, sel); err != nil {
			return false, nil
		}
		return e.IsDisplayed()
	})
	return e
}

func (h *helper) sendKeys(sel, keys string) {
	if err := h.waitFor(sel).SendKeys(keys); err != nil {
		h.t.Fatalf("%s: %v", sel, err)
	}
}

func (h *helper) click(sel string) {
	if err := h.waitFor(sel).Click(); err != nil {
		h.t.Fatalf("%s: %v", sel, err)
	}
}
