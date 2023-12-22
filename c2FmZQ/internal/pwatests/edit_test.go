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
	"os"
	"path/filepath"
	"testing"

	"c2FmZQ/internal/pwa"
)

func TestEdit(t *testing.T) {
	wd, stop := startServer(t)
	defer stop()

	alice := newClient(t)
	if err := alice.CreateAccount(wd.ServerURL(), "alice@c2fmzq.org", "foobar", true); err != nil {
		t.Fatalf("alice.CreateAccount: %v", err)
	}
	dir := t.TempDir()
	b, err := pwa.FS.ReadFile("c2.png")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "c2.png"), b, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := alice.GetUpdates(false); err != nil {
		t.Fatalf("GetUpdates: %v", err)
	}
	if _, err := alice.ImportFiles([]string{filepath.Join(dir, "*")}, "gallery", false); err != nil {
		t.Fatalf("ImportFiles: %v", err)
	}
	if err := alice.Sync(false); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	wd.setPassphrase("hello")

	t.Log("Logging in")
	wd.click("#login-tab")
	wd.sendKeys("#email-input", "alice@c2fmzq.org")
	wd.sendKeys("#password-input", "foobar")
	wd.click("#login-button")
	wd.waitFor("#gallery")

	wd.rightClick(".thumbdiv")
	wd.click("#context-menu-edit")
	wd.click(".FIE_topbar-save-button")
	wd.sendKeys(".FIE_save-file-name-input>div>input", "blah")
	wd.click(".FIE_save-modal button[color=primary]")

	wd.waitPopupMessage("Upload: 1/1 [100%]")

	t.Log("Done")
}
