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

package pwa_test

import (
	"testing"

	"github.com/tebeka/selenium"
)

func TestRegisterRecoverLogin(t *testing.T) {
	wd, stop := startServer(t)
	defer stop()

	wd.setPassphrase("hello")
	wd.createAccount("test@c2fmzq.org", "foobar")

	t.Log("Getting backup phrase")
	wd.click("#account-button")
	wd.click("#account-menu-key-backup")
	wd.click("#backup-phrase-show-button")
	wd.sendKeys(".prompt-input", "foobar\n")
	wd.click(".prompt-confirm-button")

	var backupPhrase string
	wd.Wait(func(selenium.WebDriver) (bool, error) {
		v, err := wd.css("#backup-phrase-value").Text()
		if err != nil || v == "" {
			return false, nil
		}
		backupPhrase = v
		return true, nil
	})
	wd.click(".popup-close")

	wd.logout()

	wd.setPassphrase("hello")

	t.Log("Recovering account")
	wd.click("#recover-tab")
	wd.sendKeys("#email-input", "test@c2fmzq.org")
	wd.sendKeys("#backup-phrase-input", backupPhrase)
	wd.sendKeys("#password-input", "foobar2")
	wd.sendKeys("#password-input2", "foobar2")
	wd.click("#login-button")
	wd.waitFor("#gallery")

	wd.logout()

	wd.setPassphrase("hello")

	t.Log("Logging in")
	wd.click("#login-tab")
	wd.sendKeys("#email-input", "test@c2fmzq.org")
	wd.sendKeys("#password-input", "foobar2")
	wd.click("#login-button")
	wd.waitFor("#gallery")

	t.Log("Done")
}

func TestNoBackupKeys(t *testing.T) {
	wd, stop := startServer(t)
	defer stop()

	wd.setPassphrase("hello")
	t.Log("Creating new account")
	wd.click("#register-tab")
	wd.sendKeys("#email-input", "test@c2fmzq.org")
	wd.sendKeys("#password-input", "foobar")
	wd.sendKeys("#password-input2", "foobar")
	wd.click("#backup-keys-checkbox") // <==
	wd.click("#login-button")
	wd.waitFor("#gallery")

	wd.waitPopupMessage("Your secret key is NOT backed up. You will need a backup phrase next time you login.")

	t.Log("Getting backup phrase")
	wd.click("#account-button")
	wd.click("#account-menu-key-backup")
	wd.click("#backup-phrase-show-button")
	wd.sendKeys(".prompt-input", "foobar\n")
	wd.click(".prompt-confirm-button")

	var backupPhrase string
	wd.Wait(func(selenium.WebDriver) (bool, error) {
		v, err := wd.css("#backup-phrase-value").Text()
		if err != nil || v == "" {
			return false, nil
		}
		backupPhrase = v
		return true, nil
	})
	wd.click(".popup-close")

	wd.logout()

	wd.setPassphrase("hello")

	t.Log("Logging in")
	wd.click("#login-tab")
	wd.sendKeys("#email-input", "test@c2fmzq.org")
	wd.sendKeys("#password-input", "foobar")
	wd.click("#login-button")

	wd.sendKeys(".prompt-input", backupPhrase)
	wd.click(".prompt-confirm-button")
	wd.waitFor("#gallery")

	t.Log("Enable key backup")
	wd.click("#account-button")
	wd.click("#account-menu-key-backup")
	wd.click("#choose-key-backup-yes")
	wd.sendKeys(".prompt-input", "foobar\n")
	wd.click(".prompt-confirm-button")
	wd.waitPopupMessage("Enabled")
	wd.click(".popup-close")

	wd.logout()

	wd.setPassphrase("hello")

	t.Log("Logging in")
	wd.click("#login-tab")
	wd.sendKeys("#email-input", "test@c2fmzq.org")
	wd.sendKeys("#password-input", "foobar")
	wd.click("#login-button")
	wd.waitFor("#gallery")

	t.Log("Done")
}
