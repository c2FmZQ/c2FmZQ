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
	"time"

	"c2FmZQ/internal/client"
	"c2FmZQ/internal/pwa"
	"c2FmZQ/internal/stingle"
)

func TestCreateDeleteAlbum(t *testing.T) {
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

	t.Log("Create collection")
	wd.click("#add-button")
	wd.click("#menu-create-collection")
	wd.sendKeys("#collection-properties-name", "my pix")
	wd.click("#collection-properties-apply-button")

	wd.sleep(2 * time.Second) // for refresh
	wd.waitText(".collectionThumbLabel", "my pix")

	wd.click("#settings-button")
	wd.click("#collection-properties-delete")
	wd.click(".prompt-confirm-button")

	t.Log("Done")
}

func TestSharing(t *testing.T) {
	wd, stop := startServer(t)
	defer stop()

	alice := newClient(t)
	if err := alice.CreateAccount(wd.ServerURL(), "alice@c2fmzq.org", "foobar", true); err != nil {
		t.Fatalf("alice.CreateAccount: %v", err)
	}
	bob := newClient(t)
	if err := bob.CreateAccount(wd.ServerURL(), "bob@c2fmzq.org", "foobar", true); err != nil {
		t.Fatalf("bob.CreateAccount: %v", err)
	}

	t.Log("Setting passphrase")
	wd.sendKeys("#passphrase-input", "hello\n")

	t.Log("Logging in")
	wd.click("#login-tab")
	wd.sendKeys("#email-input", "alice@c2fmzq.org")
	wd.sendKeys("#password-input", "foobar")
	wd.click("#login-button")
	wd.waitFor("#gallery")

	t.Log("Create collection")
	wd.click("#add-button")
	wd.click("#menu-create-collection")
	wd.sendKeys("#collection-properties-name", "my pix")
	wd.click("#collection-properties-shared")
	wd.sendKeys("#collection-properties-members-input", "bob@c2fmzq.org")
	wd.click("#collection-properties-members-add-button")
	wd.click("#collection-properties-perm-add")
	wd.click("#collection-properties-perm-copy")
	wd.click("#collection-properties-perm-share")
	wd.click("#collection-properties-apply-button")

	wd.sleep(2 * time.Second) // for refresh

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

	wd.click("#refresh-button")
	wd.sleep(2 * time.Second) // for refresh

	wd.click("#collections>div:nth-child(1)")
	wd.rightClick(".thumbdiv")
	wd.click("#context-menu-select")
	wd.rightClick("#collections>div:nth-child(2)")
	wd.click("#context-menu-move")
	wd.waitPopupMessage("Moved 1 file")

	if err := bob.GetUpdates(true); err != nil {
		t.Fatalf("bob.GetUpdates: %v", err)
	}
	items, err := bob.GlobFiles([]string{"shared/*"}, client.GlobOptions{MatchDot: true, Recursive: true})
	if err != nil {
		t.Fatalf("bob.GlobFiles: %v", err)
	}
	if got, want := len(items), 2; got != want {
		t.Fatalf("Unexpected number of shared albums: Got %d, want %d", got, want)
	}
	if got, want := items[0].Filename, "shared/my pix"; got != want {
		t.Errorf("Unexpected shared album: Got %q, want %q", got, want)
	}
	if got, want := items[1].Filename, "shared/my pix/c2.png"; got != want {
		t.Errorf("Unexpected shared album: Got %q, want %q", got, want)
	}
	if got, want := stingle.Permissions(items[0].Album.Permissions).Human(), "+Add,+Copy,+Share"; got != want {
		t.Errorf("Unexpected album permissions: Got %q, want %q", got, want)
	}
	if got, want := items[0].Album.Cover, ""; got != want {
		t.Errorf("Unexpected album cover: Got %q, want %q", got, want)
	}

	t.Log("Change the name and the permissions")
	wd.click("#collections>div:nth-child(2)")
	wd.click("#settings-button")
	wd.waitFor("#collection-properties-name").Clear()
	wd.sendKeys("#collection-properties-name", "MY PIX")
	wd.click("#collection-properties-perm-add")
	wd.click("#collection-properties-perm-share")
	wd.click("#collection-properties-apply-button")

	wd.sleep(2 * time.Second) // for refresh

	if err := bob.GetUpdates(true); err != nil {
		t.Fatalf("bob.GetUpdates: %v", err)
	}
	items, err = bob.GlobFiles([]string{"shared/*"}, client.MatchAll)
	if err != nil {
		t.Fatalf("bob.GlobFiles: %v", err)
	}
	if got, want := len(items), 1; got != want {
		t.Fatalf("Unexpected number of shared albums: Got %d, want %d", got, want)
	}
	if got, want := items[0].Filename, "shared/MY PIX"; got != want {
		t.Errorf("Unexpected shared album: Got %q, want %q", got, want)
	}
	if got, want := stingle.Permissions(items[0].Album.Permissions).Human(), "-Add,+Copy,-Share"; got != want {
		t.Errorf("Unexpected album permissions: Got %q, want %q", got, want)
	}

	t.Log("Unshare")
	wd.click("#settings-button")
	wd.click("#collection-properties-shared")
	wd.click("#collection-properties-apply-button")

	wd.sleep(2 * time.Second) // for refresh

	if err := bob.GetUpdates(true); err != nil {
		t.Fatalf("bob.GetUpdates: %v", err)
	}
	items, err = bob.GlobFiles([]string{"shared/*"}, client.MatchAll)
	if err != nil {
		t.Fatalf("bob.GlobFiles: %v", err)
	}
	if got, want := len(items), 0; got != want {
		t.Fatalf("Unexpected number of shared albums: Got %d, want %d", got, want)
	}

	t.Log("Done")
}
