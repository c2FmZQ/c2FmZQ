// Copyright 2021-2023 TTBT Enterprises LLC
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
)

func TestLock(t *testing.T) {
	wd, stop := startServer(t)
	defer stop()

	wd.setPassphrase("hello")
	wd.createAccount("test@c2fmzq.org", "foobar")

	wd.click("#account-button")
	wd.click("#account-menu-lock")

	wd.sendKeys("#passphrase-input", "hullo\n")
	wd.waitPopupMessage("Wrong passphrase")

	wd.sendKeys("#passphrase-input", "hello\n")
	wd.waitFor("#gallery")

	t.Log("Done")
}

func TestChangePassphrase(t *testing.T) {
	wd, stop := startServer(t)
	defer stop()

	wd.setPassphrase("hello")
	wd.createAccount("test@c2fmzq.org", "foobar")

	wd.logout()

	wd.setPassphrase("hullo")

	t.Log("Login")
	wd.click("#login-tab")
	wd.sendKeys("#email-input", "test@c2fmzq.org")
	wd.sendKeys("#password-input", "foobar")
	wd.click("#login-button")
	wd.waitFor("#gallery")

	t.Log("Done")
}
