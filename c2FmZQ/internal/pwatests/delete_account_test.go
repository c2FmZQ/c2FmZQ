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
	"time"
)

func TestDeleteAccount(t *testing.T) {
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

	t.Log("Deleting account")
	wd.click("#loggedin-account")
	wd.click("#account-menu-profile")
	wd.click("#profile-form-delete-button")
	wd.sendKeys(".prompt-input", "foobar\n")

	wd.sleep(2 * time.Second)

	wd.waitFor("#loggedout-div")
	wd.click("#login-tab")
	wd.sendKeys("#email-input", "test@c2fmzq.org")
	wd.sendKeys("#password-input", "foobar")
	wd.click("#login-button")

	wd.waitPopupMessage("Login failed", "Invalid credentials")

	t.Log("Done")
}
