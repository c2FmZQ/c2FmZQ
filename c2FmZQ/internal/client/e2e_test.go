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

package client_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-test/deep"
	"github.com/tyler-smith/go-bip39"

	"c2FmZQ/internal/client"
)

func TestLoginLogout(t *testing.T) {
	c, url, done := startServer(t)
	defer done()

	t.Log("CLIENT CreateAccount")
	if err := c.CreateAccount(url, "alice@", "pass", true); err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	t.Log("CLIENT Login")
	if err := c.Login(url, "alice@", "pass"); err != nil {
		t.Fatalf("Login: %v", err)
	}
	t.Log("CLIENT Logout")
	if err := c.Logout(); err != nil {
		t.Fatalf("c.Logout: %v", err)
	}
	t.Log("CLIENT Login")
	if err := c.Login(url, "alice@", "pass"); err != nil {
		t.Fatalf("Login: %v", err)
	}
	t.Log("CLIENT DeleteAccount")
	if err := c.DeleteAccount("pass"); err != nil {
		t.Fatalf("c.DeleteAccount: %v", err)
	}
}

func TestRecovery(t *testing.T) {
	c, url, done := startServer(t)
	defer done()

	t.Log("CLIENT CreateAccount")
	if err := c.CreateAccount(url, "alice@", "pass", false); err != nil {
		t.Fatalf("c.CreateAccount: %v", err)
	}
	sk := c.SecretKey()
	defer sk.Wipe()
	phr, err := bip39.NewMnemonic(sk.ToBytes())
	if err != nil {
		t.Fatalf("bip39.NewMnemonic: %v", err)
	}
	c.SetPrompt(func(string) (string, error) {
		return phr, nil
	})
	if err := c.Login(url, "alice@", "pass"); err != nil {
		t.Fatalf("c.Login: %v", err)
	}

	var buf bytes.Buffer
	c.SetWriter(&buf)
	if err := c.BackupPhrase("wrong-pass"); err == nil {
		t.Fatal("c.BackupPhrase succeeded unexpectedly")
	}
	if err := c.BackupPhrase("pass"); err != nil {
		t.Fatalf("c.BackupPhrase: %v", err)
	}
	if want, got := phr, buf.String(); !strings.Contains(got, want) {
		t.Errorf("c.BackupPhrase returned unexpected output. Want %q, got %q", want, got)
	}

	if err := c.RecoverAccount(url, "alice@", "newpass", phr, true); err != nil {
		t.Fatalf("c.RecoverAccount: %v", err)
	}
	if err := c.ChangePassword("newpass", "newnewpass", true); err != nil {
		t.Fatalf("c.ChangePassword: %v", err)
	}
	if err := c.UploadKeys("newnewpass", false); err != nil {
		t.Errorf("c.UploadKeys(false): %v", err)
	}
	if err := c.UploadKeys("newnewpass", true); err != nil {
		t.Errorf("c.UploadKeys(true): %v", err)
	}
}

func TestImportExportSync(t *testing.T) {
	c, url, done := startServer(t)
	defer done()
	t.Log("CLIENT CreateAccount")
	if err := c.CreateAccount(url, "alice@", "pass", true); err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}

	testdir := t.TempDir()
	if err := makeImages(testdir, 0, 10); err != nil {
		t.Fatalf("makeImages: %v", err)
	}
	t.Log("CLIENT Import *")
	if n, err := c.ImportFiles([]string{filepath.Join(testdir, "*")}, "gallery", true); err != nil {
		t.Errorf("c.ImportFiles: %v", err)
	} else if want, got := 10, n; want != got {
		t.Errorf("Unexpected ImportFiles result. Want %d, got %d", want, got)
	}
	t.Log("CLIENT Import *0.jpg")
	if n, err := c.ImportFiles([]string{filepath.Join(testdir, "*0.jpg")}, "gallery", true); err != nil {
		t.Errorf("c.ImportFiles: %v", err)
	} else if want, got := 0, n; want != got {
		t.Errorf("Unexpected ImportFiles result. Want %d, got %d", want, got)
	}

	t.Log("CLIENT ListFiles gallery/*")
	if err := c.ListFiles([]string{"gallery/*"}, client.GlobOptions{}); err != nil {
		t.Errorf("c.ListFiles: %v", err)
	}

	exportDir := filepath.Join(testdir, "export")
	if err := os.Mkdir(exportDir, 0700); err != nil {
		t.Fatalf("os.Mkdir: %v", err)
	}
	t.Log("CLIENT Export gallery/*")
	if n, err := c.ExportFiles([]string{"gallery/*"}, exportDir, true); err != nil {
		t.Errorf("c.ExportFiles: %v", err)
	} else if want, got := 10, n; want != got {
		t.Errorf("Unexpected ExportFiles result. Want %d, got %d", want, got)
	}

	t.Log("CLIENT Sync dryrun")
	if err := c.Sync(true); err != nil {
		t.Errorf("c.Sync: %v", err)
	}
	t.Log("CLIENT Sync")
	if err := c.Sync(false); err != nil {
		t.Errorf("c.Sync: %v", err)
	}

	t.Log("CLIENT GetUpdates")
	if err := c.GetUpdates(false); err != nil {
		t.Errorf("c.GetUpdates: %v", err)
	}

	t.Log("CLIENT Free gallery/*")
	if n, err := c.Free([]string{"gallery/*"}, client.GlobOptions{}); err != nil {
		t.Errorf("c.Free: %v", err)
	} else if want, got := 10, n; want != got {
		t.Errorf("Unexpected Free result. Want %d, got %d", want, got)
	}

	t.Log("CLIENT Pull gallery/*0.jpg")
	if n, err := c.Pull([]string{"gallery/*0.jpg"}, client.GlobOptions{}); err != nil {
		t.Errorf("c.Pull: %v", err)
	} else if want, got := 1, n; want != got {
		t.Errorf("Unexpected Pull result. Want %d, got %d", want, got)
	}
	t.Log("CLIENT Pull gallery/*")
	if n, err := c.Pull([]string{"gallery/*"}, client.GlobOptions{}); err != nil {
		t.Errorf("c.Pull: %v", err)
	} else if want, got := 9, n; want != got {
		t.Errorf("Unexpected Pull result. Want %d, got %d", want, got)
	}
}

func TestCopyMoveDeleteFiles(t *testing.T) {
	c, url, done := startServer(t)
	defer done()
	t.Log("CLIENT CreateAccount")
	if err := c.CreateAccount(url, "alice@", "pass", true); err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}

	testdir := t.TempDir()
	if err := makeImages(testdir, 0, 5); err != nil {
		t.Fatalf("makeImages: %v", err)
	}
	t.Log("CLIENT Import")
	if n, err := c.ImportFiles([]string{filepath.Join(testdir, "*")}, "gallery", true); err != nil {
		t.Errorf("c.ImportFiles: %v", err)
	} else if want, got := 5, n; want != got {
		t.Errorf("Unexpected ImportFiles result. Want %d, got %d", want, got)
	}

	t.Log("CLIENT AddAlbums alpha beta charlie")
	if err := c.AddAlbums([]string{"alpha", "beta", "charlie"}); err != nil {
		t.Fatalf("AddAlbums: %v", err)
	}

	t.Log("CLIENT Copy gallery/image00[0-1].jpg -> alpha")
	if err := c.Copy([]string{"gallery/image00[0-1].jpg"}, "alpha", false); err != nil {
		t.Fatalf("c.Copy: %v", err)
	}

	t.Log("CLIENT Move gallery/image00[2-3].jpg -> beta")
	if err := c.Move([]string{"gallery/image00[2-3].jpg"}, "beta", false); err != nil {
		t.Fatalf("c.Move: %v", err)
	}

	want := []string{
		".trash",
		"alpha LOCAL",
		"alpha/image000.jpg LOCAL",
		"alpha/image001.jpg LOCAL",
		"beta LOCAL",
		"beta/image002.jpg LOCAL",
		"beta/image003.jpg LOCAL",
		"charlie LOCAL",
		"gallery",
		"gallery/image000.jpg LOCAL",
		"gallery/image001.jpg LOCAL",
		"gallery/image004.jpg LOCAL",
	}
	got, err := globAll(c)
	if err != nil {
		t.Fatalf("globAll: %v", err)
	}
	if diff := deep.Equal(want, got); diff != nil {
		t.Fatalf("Unexpected file list. Want %#v, got %#v, diff: %v", want, got, diff)
	}

	t.Log("CLIENT Delete alpha/image000.jpg gallery/image004.jpg")
	if err := c.Delete([]string{"alpha/image000.jpg", "gallery/image004.jpg"}, false); err != nil {
		t.Fatalf("c.Delete: %v", err)
	}

	want = []string{
		".trash",
		".trash/image000.jpg LOCAL",
		".trash/image004.jpg LOCAL",
		"alpha LOCAL",
		"alpha/image001.jpg LOCAL",
		"beta LOCAL",
		"beta/image002.jpg LOCAL",
		"beta/image003.jpg LOCAL",
		"charlie LOCAL",
		"gallery",
		"gallery/image000.jpg LOCAL",
		"gallery/image001.jpg LOCAL",
	}
	if got, err = globAll(c); err != nil {
		t.Fatalf("globAll: %v", err)
	}
	if diff := deep.Equal(want, got); diff != nil {
		t.Fatalf("Unexpected file list. Want %#v, got %#v, diff: %v", want, got, diff)
	}

	t.Log("CLIENT Delete .trash/*")
	if err := c.Delete([]string{".trash/*"}, false); err != nil {
		t.Fatalf("c.Delete: %v", err)
	}

	want = []string{
		".trash",
		"alpha LOCAL",
		"alpha/image001.jpg LOCAL",
		"beta LOCAL",
		"beta/image002.jpg LOCAL",
		"beta/image003.jpg LOCAL",
		"charlie LOCAL",
		"gallery",
		"gallery/image000.jpg LOCAL",
		"gallery/image001.jpg LOCAL",
	}
	if got, err = globAll(c); err != nil {
		t.Fatalf("globAll: %v", err)
	}
	if diff := deep.Equal(want, got); diff != nil {
		t.Fatalf("Unexpected file list. Want %#v, got %#v, diff: %v", want, got, diff)
	}

	// Delete alpha should fail because it's not empty.
	t.Log("CLIENT Delete alpha (should fail)")
	if err := c.Delete([]string{"alpha"}, false); err == nil {
		t.Fatal("c.Delete succeeded unexpectedly.")
	}
	t.Log("CLIENT Delete charlie")
	// Delete charlie should succeed because it is empty.
	if err := c.Delete([]string{"charlie"}, false); err != nil {
		t.Fatalf("c.Delete: %v", err)
	}

	t.Log("CLIENT Sync")
	if err := c.Sync(false); err != nil {
		t.Fatalf("c.Sync: %v", err)
	}

	want = []string{
		".trash",
		"alpha",
		"alpha/image001.jpg",
		"beta",
		"beta/image002.jpg",
		"beta/image003.jpg",
		"gallery",
		"gallery/image000.jpg",
		"gallery/image001.jpg",
	}
	if got, err = globAll(c); err != nil {
		t.Fatalf("globAll: %v", err)
	}
	if diff := deep.Equal(want, got); diff != nil {
		t.Fatalf("Unexpected file list. Want %#v, got %#v, diff: %v", want, got, diff)
	}

	t.Log("CLIENT Move gallery/image000.jpg -> gallery/foo.jpg")
	if err := c.Move([]string{"gallery/image000.jpg"}, "gallery/foo.jpg", false); err != nil {
		t.Fatalf("c.Move: %v", err)
	}
	t.Log("CLIENT Move gallery/foo.jpg -> beta/bar.jpg")
	if err := c.Move([]string{"gallery/foo.jpg"}, "beta/bar.jpg", false); err != nil {
		t.Fatalf("c.Move: %v", err)
	}
	t.Log("CLIENT Copy gallery/image001.jpg -> gallery/abc.jpg (should fail)")
	if err := c.Copy([]string{"gallery/image001.jpg"}, "gallery/abc.jpg", false); err == nil {
		t.Fatal("c.Copy succeeded unexpectedly")
	}
	t.Log("CLIENT Copy gallery/image001.jpg -> beta/xyz.jpg")
	if err := c.Copy([]string{"gallery/image001.jpg"}, "beta/xyz.jpg", false); err != nil {
		t.Fatalf("c.Copy: %v", err)
	}

	t.Log("CLIENT Sync")
	if err := c.Sync(false); err != nil {
		t.Fatalf("c.Sync: %v", err)
	}

	want = []string{
		".trash",
		"alpha",
		"alpha/image001.jpg",
		"beta",
		"beta/bar.jpg",
		"beta/image002.jpg",
		"beta/image003.jpg",
		"beta/xyz.jpg",
		"gallery",
		"gallery/image001.jpg",
	}
	if got, err = globAll(c); err != nil {
		t.Fatalf("globAll: %v", err)
	}
	if diff := deep.Equal(want, got); diff != nil {
		t.Fatalf("Unexpected file list. Want %#v, got %#v, Diff: %v", want, got, diff)
	}
}

func TestNestedDirectories(t *testing.T) {
	c, url, done := startServer(t)
	defer done()
	t.Log("CreateAccount")
	if err := c.CreateAccount(url, "alice@", "pass", true); err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}

	testdir := t.TempDir()
	if err := makeImages(testdir, 0, 1); err != nil {
		t.Fatalf("makeImages: %v", err)
	}
	t.Log("Import")
	if n, err := c.ImportFiles([]string{filepath.Join(testdir, "*")}, "gallery", true); err != nil {
		t.Errorf("c.ImportFiles: %v", err)
	} else if want, got := 1, n; want != got {
		t.Errorf("Unexpected ImportFiles result. Want %d, got %d", want, got)
	}

	t.Log("AddAlbums a/b/c/d")
	if err := c.AddAlbums([]string{"a/b/c/d", "a/b/e/f"}); err != nil {
		t.Fatalf("AddAlbums: %v", err)
	}

	t.Log("Copy gallery/* -> a/b/c/d")
	if err := c.Copy([]string{"gallery/*"}, "a/b/c/d", false); err != nil {
		t.Fatalf("c.Copy: %v", err)
	}

	t.Log("Move a/b/c/d/* -> a/b")
	if err := c.Move([]string{"a/b/c/d/*"}, "a/b", false); err != nil {
		t.Fatalf("c.Move: %v", err)
	}

	t.Log("Sync")
	if err := c.Sync(false); err != nil {
		t.Fatalf("c.Sync: %v", err)
	}

	want := []string{
		".trash",
		"a LOCAL",
		"a/b",
		"a/b/c LOCAL",
		"a/b/c/d",
		"a/b/e LOCAL",
		"a/b/e/f",
		"a/b/image000.jpg",
		"gallery",
		"gallery/image000.jpg",
	}
	got, err := globAll(c)
	if err != nil {
		t.Fatalf("globAll: %v", err)
	}
	if diff := deep.Equal(want, got); diff != nil {
		t.Fatalf("Unexpected file list. Want %#v, got %#v, diff: %v", want, got, diff)
	}

	t.Log("Move a/b/c/d -> a/b/e")
	if err := c.Move([]string{"a/b/c/d"}, "a/b/e", false); err != nil {
		t.Fatalf("c.Move: %v", err)
	}
	t.Log("Copy gallery/* -> a")
	if err := c.Copy([]string{"gallery/*"}, "a", false); err != nil {
		t.Fatalf("c.Move: %v", err)
	}

	t.Log("Sync")
	if err := c.Sync(false); err != nil {
		t.Fatalf("c.Sync: %v", err)
	}

	want = []string{
		".trash",
		"a",
		"a/b",
		"a/b/e",
		"a/b/e/d",
		"a/b/e/f",
		"a/b/image000.jpg",
		"a/image000.jpg",
		"gallery",
		"gallery/image000.jpg",
	}
	if got, err = globAll(c); err != nil {
		t.Fatalf("globAll: %v", err)
	}
	if diff := deep.Equal(want, got); diff != nil {
		t.Fatalf("Unexpected file list. Want %#v, got %#v, diff: %v", want, got, diff)
	}
}

func TestSyncTrash(t *testing.T) {
	c, url, done := startServer(t)
	defer done()
	t.Log("CLIENT CreateAccount")
	if err := c.CreateAccount(url, "alice@", "pass", true); err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}

	testdir := t.TempDir()
	if err := makeImages(testdir, 0, 5); err != nil {
		t.Fatalf("makeImages: %v", err)
	}
	t.Log("CLIENT Import")
	if n, err := c.ImportFiles([]string{filepath.Join(testdir, "*")}, "gallery", true); err != nil {
		t.Errorf("c.ImportFiles: %v", err)
	} else if want, got := 5, n; want != got {
		t.Errorf("Unexpected ImportFiles result. Want %d, got %d", want, got)
	}
	t.Log("CLIENT Copy gallery/* -> .trash")
	if err := c.Copy([]string{"gallery/*"}, ".trash", false); err == nil {
		t.Fatalf("c.Copy to trash succeeded unexpectedly")
	}
	t.Log("CLIENT Move gallery/* -> .trash")
	if err := c.Move([]string{"gallery/*"}, ".trash", false); err != nil {
		t.Fatalf("Move to trash: %v", err)
	}
	t.Log("CLIENT Sync")
	if err := c.Sync(false); err != nil {
		t.Fatalf("c.Sync: %v", err)
	}
	t.Log("CLIENT Copy trash/* -> gallery")
	if err := c.Copy([]string{".trash/*"}, "gallery", false); err == nil {
		t.Fatalf("c.Copy from trash succeeded unexpectedly")
	}
	t.Log("CLIENT AddAlbums alpha beta")
	if err := c.AddAlbums([]string{"alpha", "beta"}); err != nil {
		t.Fatalf("AddAlbums: %v", err)
	}
	t.Log("CLIENT Move trash/* -> alpha")
	if err := c.Move([]string{".trash/*"}, "alpha", false); err != nil {
		t.Fatalf("Move from trash to alpha: %v", err)
	}
	t.Log("CLIENT Copy alpha/* -> beta")
	if err := c.Copy([]string{"alpha/*"}, "beta", false); err != nil {
		t.Fatalf("Copy from alpha to beta: %v", err)
	}
	t.Log("CLIENT Sync")
	if err := c.Sync(false); err != nil {
		t.Fatalf("c.Sync: %v", err)
	}
	t.Log("CLIENT Delete */image000.jpg")
	if err := c.Delete([]string{"*/image000.jpg"}, false); err != nil {
		t.Fatalf("c.Delete: %v", err)
	}
	t.Log("CLIENT Delete .trash/image000.jpg")
	if err := c.Delete([]string{".trash/image000.jpg"}, false); err != nil {
		t.Fatalf("c.Delete: %v", err)
	}
	t.Log("CLIENT Sync")
	if err := c.Sync(false); err != nil {
		t.Fatalf("c.Sync: %v", err)
	}

	want := []string{
		".trash",
		"alpha",
		"alpha/image001.jpg",
		"alpha/image002.jpg",
		"alpha/image003.jpg",
		"alpha/image004.jpg",
		"beta",
		"beta/image001.jpg",
		"beta/image002.jpg",
		"beta/image003.jpg",
		"beta/image004.jpg",
		"gallery",
	}
	got, err := globAll(c)
	if err != nil {
		t.Fatalf("globAll: %v", err)
	}
	if diff := deep.Equal(want, got); diff != nil {
		t.Fatalf("Unexpected file list. Diff: %v", diff)
	}
}

func TestConcurrentMutations(t *testing.T) {
	c1, url, done := startServer(t)
	defer done()
	t.Log("CLIENT 1 CreateAccount")
	if err := c1.CreateAccount(url, "alice@", "pass", true); err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}

	testdir := t.TempDir()
	if err := makeImages(testdir, 0, 5); err != nil {
		t.Fatalf("makeImages: %v", err)
	}

	t.Log("CLIENT 1 AddAlbum alpha beta delta")
	if err := c1.AddAlbums([]string{"alpha", "beta", "delta"}); err != nil {
		t.Fatalf("c1.AddAlbums: %v", err)
	}
	t.Log("CLIENT 1 Import -> alpha")
	if n, err := c1.ImportFiles([]string{filepath.Join(testdir, "*")}, "alpha", true); err != nil {
		t.Errorf("c1.ImportFiles: %v", err)
	} else if want, got := 5, n; want != got {
		t.Errorf("Unexpected ImportFiles result. Want %d, got %d", want, got)
	}
	t.Log("CLIENT 1 Sync")
	if err := c1.Sync(false); err != nil {
		t.Fatalf("c1.Sync: %v", err)
	}
	want := []string{
		".trash",
		"alpha",
		"alpha/image000.jpg",
		"alpha/image001.jpg",
		"alpha/image002.jpg",
		"alpha/image003.jpg",
		"alpha/image004.jpg",
		"beta",
		"delta",
		"gallery",
	}
	got, err := globAll(c1)
	if err != nil {
		t.Fatalf("globAll: %v", err)
	}
	if diff := deep.Equal(want, got); diff != nil {
		t.Fatalf("Unexpected file list. Diff: %v", diff)
	}

	t.Log("CLIENT 2")

	c2, err := newClient(t.TempDir())
	if err != nil {
		t.Fatalf("newClient: %v", err)
	}
	t.Log("CLIENT 2 Login")
	if err := c2.Login(url, "alice@", "pass"); err != nil {
		t.Fatalf("c2.Login: %v", err)
	}
	t.Log("CLIENT 2 GetUpdates")
	if err := c2.GetUpdates(false); err != nil {
		t.Fatalf("c2.GetUpdates: %v", err)
	}
	t.Log("CLIENT 2 Pull */*")
	if _, err := c2.Pull([]string{"*/*"}, client.GlobOptions{}); err != nil {
		t.Fatalf("c2.Pull: %v", err)
	}
	testdir = t.TempDir()
	if err := makeImages(testdir, 100, 5); err != nil {
		t.Fatalf("makeImages: %v", err)
	}
	t.Log("CLIENT 2 AddAlbum charlie")
	if err := c2.AddAlbums([]string{"charlie"}); err != nil {
		t.Fatalf("c2.AddAlbums: %v", err)
	}
	t.Log("CLIENT 2 Delete delta")
	if err := c2.Delete([]string{"delta"}, false); err != nil {
		t.Fatalf("c2.Delete: %v", err)
	}
	t.Log("CLIENT 2 Import -> charlie")
	if n, err := c2.ImportFiles([]string{filepath.Join(testdir, "*")}, "charlie", true); err != nil {
		t.Errorf("c2.ImportFiles: %v", err)
	} else if want, got := 5, n; want != got {
		t.Errorf("Unexpected ImportFiles result. Want %d, got %d", want, got)
	}
	t.Log("CLIENT 2 Move alpha/image000.jpg charlie/image100.jpg -> beta")
	if err := c2.Move([]string{"alpha/image000.jpg", "charlie/image100.jpg"}, "beta", false); err != nil {
		t.Fatalf("c2.Move: %v", err)
	}
	want = []string{
		".trash",
		"alpha",
		"alpha/image001.jpg",
		"alpha/image002.jpg",
		"alpha/image003.jpg",
		"alpha/image004.jpg",
		"beta",
		"beta/image000.jpg LOCAL",
		"beta/image100.jpg LOCAL",
		"charlie LOCAL",
		"charlie/image101.jpg LOCAL",
		"charlie/image102.jpg LOCAL",
		"charlie/image103.jpg LOCAL",
		"charlie/image104.jpg LOCAL",
		"gallery",
	}
	if got, err = globAll(c2); err != nil {
		t.Fatalf("globAll: %v", err)
	}
	if diff := deep.Equal(want, got); diff != nil {
		t.Fatalf("Unexpected file list. Diff: %v", diff)
	}

	t.Log("CLIENT 1 Move alpha/* -> delta")
	if err := c1.Move([]string{"alpha/*"}, "delta", false); err != nil {
		t.Fatalf("c1.Move: %v", err)
	}
	t.Log("CLIENT 1 Delete alpha beta")
	if err := c1.Delete([]string{"alpha", "beta"}, false); err != nil {
		t.Fatalf("c1.Delete: %v", err)
	}
	t.Log("CLIENT 1 Sync")
	if err := c1.Sync(false); err != nil {
		t.Fatalf("c1.Sync: %v", err)
	}
	want = []string{
		".trash",
		"delta",
		"delta/image000.jpg",
		"delta/image001.jpg",
		"delta/image002.jpg",
		"delta/image003.jpg",
		"delta/image004.jpg",
		"gallery",
	}
	if got, err = globAll(c1); err != nil {
		t.Fatalf("globAll: %v", err)
	}
	if diff := deep.Equal(want, got); diff != nil {
		t.Fatalf("Unexpected file list. Diff: %v", diff)
	}

	t.Log("CLIENT 2 Sync")
	if err := c2.Sync(false); err != nil {
		t.Fatalf("c2.Sync: %v", err)
	}
	want = []string{
		".trash",
		"beta",
		"beta/image000.jpg",
		"beta/image100.jpg",
		"charlie",
		"charlie/image101.jpg",
		"charlie/image102.jpg",
		"charlie/image103.jpg",
		"charlie/image104.jpg",
		"delta",
		"delta/image000.jpg",
		"delta/image001.jpg",
		"delta/image002.jpg",
		"delta/image003.jpg",
		"delta/image004.jpg",
		"gallery",
	}
	if got, err = globAll(c2); err != nil {
		t.Fatalf("globAll: %v", err)
	}
	if diff := deep.Equal(want, got); diff != nil {
		t.Fatalf("Unexpected file list. Diff: %v", diff)
	}

	t.Log("CLIENT 1 Sync")
	if err := c1.Sync(false); err != nil {
		t.Fatalf("c1.Sync: %v", err)
	}

	if got, err = globAll(c1); err != nil {
		t.Fatalf("globAll: %v", err)
	}
	// Same state as client 2.
	if diff := deep.Equal(want, got); diff != nil {
		t.Fatalf("Unexpected file list. Diff: %v", diff)
	}
}

func TestSharing(t *testing.T) {
	_, url, done := startServer(t)
	defer done()

	c := make(map[string]*client.Client)
	for _, n := range []string{"alice", "bob", "carol", "dave"} {
		t.Logf("%s Login", n)
		var err error
		if c[n], err = newClient(t.TempDir()); err != nil {
			t.Fatalf("newClient: %v", err)
		}
		if err := c[n].CreateAccount(url, n+"@", n+"-pass", true); err != nil {
			t.Fatalf("CreateAccount(%s): %v", n, err)
		}
	}

	testdir := t.TempDir()
	if err := makeImages(testdir, 0, 5); err != nil {
		t.Fatalf("makeImages: %v", err)
	}

	t.Log("alice AddAlbum alpha")
	if err := c["alice"].AddAlbums([]string{"alpha"}); err != nil {
		t.Fatalf("alice.AddAlbums: %v", err)
	}
	t.Log("alice Import -> alpha")
	if n, err := c["alice"].ImportFiles([]string{filepath.Join(testdir, "*")}, "alpha", true); err != nil {
		t.Errorf("alice.ImportFiles: %v", err)
	} else if want, got := 5, n; want != got {
		t.Errorf("Unexpected ImportFiles result. Want %d, got %d", want, got)
	}
	t.Log("alice Sync")
	if err := c["alice"].Sync(false); err != nil {
		t.Fatalf("alice.Sync: %v", err)
	}
	c["alice"].SetPrompt(func(string) (string, error) { return "YES", nil })
	t.Log("alice Share")
	if err := c["alice"].Share("alpha", []string{"bob@", "carol@", "dave@"}, nil); err != nil {
		t.Fatalf("alice.Share: %v", err)
	}

	for n, client := range c {
		t.Logf("%s GetUpdates", n)
		if err := client.GetUpdates(false); err != nil {
			t.Fatalf("%s.GetUpdates: %v", n, err)
		}
		var want []string
		if n == "alice" {
			want = []string{
				".trash",
				"alpha",
				"alpha/image000.jpg",
				"alpha/image001.jpg",
				"alpha/image002.jpg",
				"alpha/image003.jpg",
				"alpha/image004.jpg",
				"gallery",
			}
		} else {
			want = []string{
				".trash",
				"gallery",
				"shared LOCAL",
				"shared/alpha",
				"shared/alpha/image000.jpg",
				"shared/alpha/image001.jpg",
				"shared/alpha/image002.jpg",
				"shared/alpha/image003.jpg",
				"shared/alpha/image004.jpg",
			}
		}
		got, err := globAll(client)
		if err != nil {
			t.Fatalf("globAll: %v", err)
		}
		if diff := deep.Equal(want, got); diff != nil {
			t.Fatalf("Unexpected file list. Want %#v, got %#v, diff: %v", want, got, diff)
		}
	}

	t.Log("bob Leave")
	if err := c["bob"].Leave([]string{"shared/alpha"}); err != nil {
		t.Fatalf("bob.Leave: %v", err)
	}

	t.Log("bob GetUpdates")
	if err := c["bob"].GetUpdates(false); err != nil {
		t.Fatalf("bob.GetUpdates: %v", err)
	}
	want := []string{
		".trash",
		"gallery",
	}
	got, err := globAll(c["bob"])
	if err != nil {
		t.Fatalf("globAll: %v", err)
	}
	if diff := deep.Equal(want, got); diff != nil {
		t.Fatalf("Unexpected file list. Diff: %v", diff)
	}

	t.Log("alice RemoveMember carol")
	if err := c["alice"].RemoveMembers("alpha", []string{"carol@"}); err != nil {
		t.Fatalf("alice.RemoveMembers: %v", err)
	}

	t.Log("carol GetUpdates")
	if err := c["carol"].GetUpdates(false); err != nil {
		t.Fatalf("carol.GetUpdates: %v", err)
	}
	want = []string{
		".trash",
		"gallery",
	}
	if got, err = globAll(c["carol"]); err != nil {
		t.Fatalf("globAll: %v", err)
	}
	if diff := deep.Equal(want, got); diff != nil {
		t.Fatalf("Unexpected file list. Diff: %v", diff)
	}

	t.Log("alice Unshare")
	if err := c["alice"].Unshare([]string{"alpha"}); err != nil {
		t.Fatalf("alice.Unshare: %v", err)
	}
	t.Log("dave GetUpdates")
	if err := c["dave"].GetUpdates(false); err != nil {
		t.Fatalf("dave.GetUpdates: %v", err)
	}
	want = []string{
		".trash",
		"gallery",
	}
	if got, err = globAll(c["dave"]); err != nil {
		t.Fatalf("globAll: %v", err)
	}
	if diff := deep.Equal(want, got); diff != nil {
		t.Fatalf("Unexpected file list. Diff: %v", diff)
	}
}

func TestCopyPermission(t *testing.T) {
	_, url, done := startServer(t)
	defer done()

	c := make(map[string]*client.Client)
	for _, n := range []string{"alice", "bob"} {
		t.Logf("%s Login", n)
		var err error
		if c[n], err = newClient(t.TempDir()); err != nil {
			t.Fatalf("newClient: %v", err)
		}
		if err := c[n].CreateAccount(url, n+"@", n+"-pass", true); err != nil {
			t.Fatalf("CreateAccount(%s): %v", n, err)
		}
	}

	t.Log("alice AddAlbum alpha beta")
	if err := c["alice"].AddAlbums([]string{"alpha", "beta"}); err != nil {
		t.Fatalf("alice.AddAlbums: %v", err)
	}
	testdir := t.TempDir()
	if err := makeImages(testdir, 0, 1); err != nil {
		t.Fatalf("makeImages: %v", err)
	}

	t.Log("alice Import -> alpha")
	if _, err := c["alice"].ImportFiles([]string{filepath.Join(testdir, "*")}, "alpha", true); err != nil {
		t.Errorf("alice.ImportFiles: %v", err)
	}
	t.Log("alice Copy alpha/* -> beta")
	if err := c["alice"].Copy([]string{"alpha/*"}, "beta", false); err != nil {
		t.Fatalf("alice.Copy: %v", err)
	}
	t.Log("alice Sync")
	if err := c["alice"].Sync(false); err != nil {
		t.Fatalf("alice.Sync: %v", err)
	}
	c["alice"].SetPrompt(func(string) (string, error) { return "YES", nil })
	t.Log("alice Share alpha")
	if err := c["alice"].Share("alpha", []string{"bob@"}, []string{"+copy"}); err != nil {
		t.Fatalf("alice.Share: %v", err)
	}
	t.Log("alice Share beta")
	if err := c["alice"].Share("beta", []string{"bob@"}, nil); err != nil {
		t.Fatalf("alice.Share: %v", err)
	}

	for n, client := range c {
		t.Logf("%s GetUpdates", n)
		if err := client.GetUpdates(false); err != nil {
			t.Fatalf("%s.GetUpdates: %v", n, err)
		}
	}
	t.Log("bob Copy shared/beta/* -> gallery   Should fail")
	if err := c["bob"].Copy([]string{"shared/beta/*"}, "gallery", false); err == nil {
		t.Fatal("bob.Copy succeeded unexpectedly")
	} else {
		t.Logf("Copy error: %v", err)
	}
	t.Log("bob Copy shared/alpha/* -> gallery")
	if err := c["bob"].Copy([]string{"shared/alpha/*"}, "gallery", false); err != nil {
		t.Fatalf("bob.Copy: %v", err)
	}
	t.Log("bob Sync")
	if err := c["bob"].Sync(false); err != nil {
		t.Fatalf("bob.Sync: %v", err)
	}
}
