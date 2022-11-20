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
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
	"github.com/tebeka/selenium"
)

func TestMFAWithSecurityKey(t *testing.T) {
	wd, stop := startServer(t)
	defer stop()

	wd.enableWebauthn()

	t.Log("Setting passphrase")
	wd.sendKeys("#passphrase-input", "hello\n")

	t.Log("Creating new account")
	wd.click("#register-tab")
	wd.sendKeys("#email-input", "test@c2fmzq.org")
	wd.sendKeys("#password-input", "foobar")
	wd.sendKeys("#password-input2", "foobar")
	wd.click("#login-button")
	wd.waitFor("#gallery")

	t.Log("Adding security key")
	wd.click("#loggedin-account")
	wd.click("#account-menu-profile")
	wd.sendKeys("#profile-form-password", "foobar")
	wd.click("#profile-form-add-security-key-button")
	wd.clear(".prompt-input")
	wd.sendKeys(".prompt-input", "test key")
	wd.click(".prompt-confirm-button")
	wd.waitPopupMessage("Security key registered")

	wd.click("#profile-form-enable-mfa")
	wd.click("#profile-form-test-mfa")
	wd.waitPopupMessage("MFA OK")

	wd.click("#profile-form-button")
	wd.waitPopupMessage("MFA enabled")

	t.Log("Logging out")
	wd.click("#loggedin-account")
	wd.click("#account-menu-logout")

	t.Log("Logging in")
	wd.waitFor("#login-tab")
	wd.click("#login-tab")
	wd.sendKeys("#email-input", "test@c2fmzq.org")
	wd.sendKeys("#password-input", "foobar")
	wd.click("#login-button")
	wd.waitFor("#gallery")

	t.Log("Getting backup phrase")
	wd.click("#loggedin-account")
	wd.click("#account-menu-key-backup")
	wd.sendKeys("#key-backup-password", "foobar")
	wd.click("#backup-phrase-show-button")

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

	t.Log("Logging out")
	wd.click("#loggedin-account")
	wd.click("#account-menu-logout")

	t.Log("Recovering account")
	wd.click("#recover-tab")
	wd.sendKeys("#email-input", "test@c2fmzq.org")
	wd.sendKeys("#backup-phrase-input", backupPhrase)
	wd.sendKeys("#password-input", "foobar2")
	wd.sendKeys("#password-input2", "foobar2")
	wd.click("#login-button")
	wd.waitFor("#gallery")

	wd.disableWebauthn()

	t.Log("Logging out")
	wd.click("#loggedin-account")
	wd.click("#account-menu-logout")

	t.Log("Recovering account without webauthn")
	wd.click("#recover-tab")
	wd.sendKeys("#email-input", "test@c2fmzq.org")
	wd.sendKeys("#backup-phrase-input", backupPhrase)
	wd.sendKeys("#password-input", "foobar2")
	wd.sendKeys("#password-input2", "foobar2")
	wd.click("#login-button")

	wd.click(".prompt-cancel-button")
	wd.waitPopupMessage("Canceled")

	t.Log("Done")
}

func TestMFAWithOTP(t *testing.T) {
	wd, stop := startServer(t)
	defer stop()

	t.Log("Setting passphrase")
	wd.sendKeys("#passphrase-input", "hello\n")

	t.Log("Creating new account")
	wd.click("#register-tab")
	wd.sendKeys("#email-input", "test@c2fmzq.org")
	wd.sendKeys("#password-input", "foobar")
	wd.sendKeys("#password-input2", "foobar")
	wd.click("#login-button")
	wd.waitFor("#gallery")

	t.Log("Adding OTP")
	wd.click("#loggedin-account")
	wd.click("#account-menu-profile")
	wd.sendKeys("#profile-form-password", "foobar")
	wd.click("#profile-form-enable-otp")
	otpKey, err := wd.waitFor("#profile-form-otp-key").Text()
	if err != nil || !strings.HasPrefix(otpKey, "KEY: ") {
		t.Fatalf("Unexpected otpKey: (%q, %v)", otpKey, err)
	}
	otpKey = otpKey[5:]
	code := func() string {
		code, err := totp.GenerateCode(otpKey, time.Now())
		if err != nil {
			t.Fatalf("totp.GenerateCode: %v", err)
		}
		return code
	}
	wd.sendKeys("#profile-form-otp-code", code())

	wd.click("#profile-form-enable-mfa")
	wd.click("#profile-form-button")
	wd.sendKeys(".prompt-input", code())
	wd.click(".prompt-confirm-button")
	wd.waitPopupMessage("MFA enabled", "OTP enabled")

	t.Log("Logging out")
	wd.click("#loggedin-account")
	wd.click("#account-menu-logout")

	t.Log("Logging in")
	wd.waitFor("#login-tab")
	wd.click("#login-tab")
	wd.sendKeys("#email-input", "test@c2fmzq.org")
	wd.sendKeys("#password-input", "foobar")
	wd.click("#login-button")
	wd.sendKeys(".prompt-input", code())
	wd.click(".prompt-confirm-button")
	wd.waitFor("#gallery")

	t.Log("Turn off OTP and MFA")
	wd.click("#loggedin-account")
	wd.click("#account-menu-profile")
	wd.sendKeys("#profile-form-password", "foobar")
	wd.click("#profile-form-enable-mfa")
	wd.click("#profile-form-enable-otp")
	wd.click("#profile-form-button")
	wd.sendKeys(".prompt-input", code())
	wd.click(".prompt-confirm-button")
	wd.waitPopupMessage("MFA disabled", "OTP disabled")

	t.Log("Done")
}
