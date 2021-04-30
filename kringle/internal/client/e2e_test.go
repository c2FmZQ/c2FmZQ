// +build !nacl,!arm

package client_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-test/deep"
	"github.com/tyler-smith/go-bip39"

	"kringle/internal/client"
)

func TestLoginLogout(t *testing.T) {
	c, done := startServer(t)
	defer done()

	t.Log("CLIENT CreateAccount")
	if err := c.CreateAccount("alice@", "pass", true); err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	t.Log("CLIENT Login")
	if err := c.Login("alice@", "pass"); err != nil {
		t.Fatalf("Login: %v", err)
	}
	t.Log("CLIENT Logout")
	if err := c.Logout(); err != nil {
		t.Fatalf("c.Logout: %v", err)
	}
}

func TestRecovery(t *testing.T) {
	c, done := startServer(t)
	defer done()

	t.Log("CLIENT CreateAccount")
	if err := c.CreateAccount("alice@", "pass", false); err != nil {
		t.Fatalf("c.CreateAccount: %v", err)
	}
	phr, err := bip39.NewMnemonic(c.SecretKey.ToBytes())
	if err != nil {
		t.Fatalf("bip39.NewMnemonic: %v", err)
	}
	c.SetPrompt(func(string) (string, error) {
		return phr, nil
	})
	if err := c.Login("alice@", "pass"); err != nil {
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

	if err := c.RecoverAccount("alice@", "newpass", phr, true); err != nil {
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
	c, done := startServer(t)
	defer done()
	t.Log("CLIENT LOGIN")
	if err := c.CreateAccount("alice@", "pass", true); err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}

	testdir := t.TempDir()
	if err := makeImages(testdir, 0, 10); err != nil {
		t.Fatalf("makeImages: %v", err)
	}
	t.Log("CLIENT IMPORT *")
	if n, err := c.ImportFiles([]string{filepath.Join(testdir, "*")}, "gallery"); err != nil {
		t.Errorf("c.ImportFiles: %v", err)
	} else if want, got := 10, n; want != got {
		t.Errorf("Unexpected ImportFiles result. Want %d, got %d", want, got)
	}
	t.Log("CLIENT IMPORT *.jpg")
	if n, err := c.ImportFiles([]string{filepath.Join(testdir, "*0.jpg")}, "gallery"); err != nil {
		t.Errorf("c.ImportFiles: %v", err)
	} else if want, got := 0, n; want != got {
		t.Errorf("Unexpected ImportFiles result. Want %d, got %d", want, got)
	}

	t.Log("CLIENT LIST gallery/*")
	if err := c.ListFiles([]string{"gallery/*"}); err != nil {
		t.Errorf("c.ListFiles: %v", err)
	}

	exportDir := filepath.Join(testdir, "export")
	if err := os.Mkdir(exportDir, 0700); err != nil {
		t.Fatalf("os.Mkdir: %v", err)
	}
	t.Log("CLIENT EXPORT gallery/*")
	if n, err := c.ExportFiles([]string{"gallery/*"}, exportDir); err != nil {
		t.Errorf("c.ExportFiles: %v", err)
	} else if want, got := 10, n; want != got {
		t.Errorf("Unexpected ExportFiles result. Want %d, got %d", want, got)
	}

	t.Log("CLIENT SYNC dryrun")
	if err := c.Sync(true); err != nil {
		t.Errorf("c.Sync: %v", err)
	}
	t.Log("CLIENT SYNC")
	if err := c.Sync(false); err != nil {
		t.Errorf("c.Sync: %v", err)
	}

	t.Log("CLIENT GETUPDATES")
	if err := c.GetUpdates(false); err != nil {
		t.Errorf("c.GetUpdates: %v", err)
	}

	t.Log("CLIENT FREE gallery/*")
	if n, err := c.Free([]string{"gallery/*"}); err != nil {
		t.Errorf("c.Free: %v", err)
	} else if want, got := 10, n; want != got {
		t.Errorf("Unexpected Free result. Want %d, got %d", want, got)
	}

	t.Log("CLIENT PULL gallery/*0.jpg")
	if n, err := c.Pull([]string{"gallery/*0.jpg"}); err != nil {
		t.Errorf("c.Pull: %v", err)
	} else if want, got := 1, n; want != got {
		t.Errorf("Unexpected Pull result. Want %d, got %d", want, got)
	}
	t.Log("CLIENT PULL gallery/*")
	if n, err := c.Pull([]string{"gallery/*"}); err != nil {
		t.Errorf("c.Pull: %v", err)
	} else if want, got := 9, n; want != got {
		t.Errorf("Unexpected Pull result. Want %d, got %d", want, got)
	}
}

func TestCopyMoveDelete(t *testing.T) {
	c, done := startServer(t)
	defer done()
	t.Log("CLIENT LOGIN")
	if err := c.CreateAccount("alice@", "pass", true); err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}

	testdir := t.TempDir()
	if err := makeImages(testdir, 0, 5); err != nil {
		t.Fatalf("makeImages: %v", err)
	}
	t.Log("CLIENT IMPORT")
	if n, err := c.ImportFiles([]string{filepath.Join(testdir, "*")}, "gallery"); err != nil {
		t.Errorf("c.ImportFiles: %v", err)
	} else if want, got := 5, n; want != got {
		t.Errorf("Unexpected ImportFiles result. Want %d, got %d", want, got)
	}

	t.Log("CLIENT ADDALBUMS alpha beta charlie")
	if err := c.AddAlbums([]string{"alpha", "beta", "charlie"}); err != nil {
		t.Fatalf("AddAlbums: %v", err)
	}

	t.Log("CLIENT COPY gallery/image00[0-1].jpg -> alpha")
	if err := c.Copy([]string{"gallery/image00[0-1].jpg"}, "alpha"); err != nil {
		t.Fatalf("c.Copy: %v", err)
	}

	t.Log("CLIENT MOVE gallery/image00[2-3].jpg -> beta")
	if err := c.Move([]string{"gallery/image00[2-3].jpg"}, "beta"); err != nil {
		t.Fatalf("c.Move: %v", err)
	}

	want := []string{
		"alpha LOCAL",
		"beta LOCAL",
		"charlie LOCAL",
		"gallery",
		"trash",
		"alpha/image000.jpg LOCAL",
		"alpha/image001.jpg LOCAL",
		"beta/image002.jpg LOCAL",
		"beta/image003.jpg LOCAL",
		"gallery/image000.jpg LOCAL",
		"gallery/image001.jpg LOCAL",
		"gallery/image004.jpg LOCAL",
	}
	got, err := globAll(c)
	if err != nil {
		t.Fatalf("globAll: %v", err)
	}
	if diff := deep.Equal(want, got); diff != nil {
		t.Fatalf("Unexpected file list. Diff: %v", diff)
	}

	t.Log("CLIENT DELETE alpha/image000.jpg gallery/image004.jpg")
	if err := c.Delete([]string{"alpha/image000.jpg", "gallery/image004.jpg"}); err != nil {
		t.Fatalf("c.Delete: %v", err)
	}

	want = []string{
		"alpha LOCAL",
		"beta LOCAL",
		"charlie LOCAL",
		"gallery",
		"trash",
		"alpha/image001.jpg LOCAL",
		"beta/image002.jpg LOCAL",
		"beta/image003.jpg LOCAL",
		"gallery/image000.jpg LOCAL",
		"gallery/image001.jpg LOCAL",
		"trash/image000.jpg LOCAL",
		"trash/image004.jpg LOCAL",
	}
	if got, err = globAll(c); err != nil {
		t.Fatalf("globAll: %v", err)
	}
	if diff := deep.Equal(want, got); diff != nil {
		t.Fatalf("Unexpected file list. Diff: %v", diff)
	}

	t.Log("CLIENT DELETE trash/*")
	if err := c.Delete([]string{"trash/*"}); err != nil {
		t.Fatalf("c.Delete: %v", err)
	}

	want = []string{
		"alpha LOCAL",
		"beta LOCAL",
		"charlie LOCAL",
		"gallery",
		"trash",
		"alpha/image001.jpg LOCAL",
		"beta/image002.jpg LOCAL",
		"beta/image003.jpg LOCAL",
		"gallery/image000.jpg LOCAL",
		"gallery/image001.jpg LOCAL",
	}
	if got, err = globAll(c); err != nil {
		t.Fatalf("globAll: %v", err)
	}
	if diff := deep.Equal(want, got); diff != nil {
		t.Fatalf("Unexpected file list. Diff: %v", diff)
	}

	// Delete alpha should fail because it's not empty.
	t.Log("CLIENT DELETE alpha (should fail)")
	if err := c.Delete([]string{"alpha"}); err == nil {
		t.Fatal("c.Delete succeeded unexpectedly.")
	}
	t.Log("CLIENT DELETE charlie")
	// Delete charlie should succeed because it is empty.
	if err := c.Delete([]string{"charlie"}); err != nil {
		t.Fatalf("c.Delete: %v", err)
	}

	t.Log("CLIENT SYNC")
	if err := c.Sync(false); err != nil {
		t.Fatalf("c.Sync: %v", err)
	}

	want = []string{
		"alpha",
		"beta",
		"gallery",
		"trash",
		"alpha/image001.jpg",
		"beta/image002.jpg",
		"beta/image003.jpg",
		"gallery/image000.jpg",
		"gallery/image001.jpg",
	}
	if got, err = globAll(c); err != nil {
		t.Fatalf("globAll: %v", err)
	}
	if diff := deep.Equal(want, got); diff != nil {
		t.Fatalf("Unexpected file list. Diff: %v", diff)
	}
}

func TestConcurrentMutations(t *testing.T) {
	c1, done := startServer(t)
	defer done()
	t.Log("CLIENT 1 CreateAccount")
	if err := c1.CreateAccount("alice@", "pass", true); err != nil {
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
	if n, err := c1.ImportFiles([]string{filepath.Join(testdir, "*")}, "alpha"); err != nil {
		t.Errorf("c1.ImportFiles: %v", err)
	} else if want, got := 5, n; want != got {
		t.Errorf("Unexpected ImportFiles result. Want %d, got %d", want, got)
	}
	t.Log("CLIENT 1 Sync")
	if err := c1.Sync(false); err != nil {
		t.Fatalf("c1.Sync: %v", err)
	}
	want := []string{
		"alpha",
		"beta",
		"delta",
		"gallery",
		"trash",
		"alpha/image000.jpg",
		"alpha/image001.jpg",
		"alpha/image002.jpg",
		"alpha/image003.jpg",
		"alpha/image004.jpg",
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
	if err := c2.Login("alice@", "pass"); err != nil {
		t.Fatalf("c2.Login: %v", err)
	}
	t.Log("CLIENT 2 GetUpdates")
	if err := c2.GetUpdates(false); err != nil {
		t.Fatalf("c2.GetUpdates: %v", err)
	}
	t.Log("CLIENT 2 Pull */*")
	if _, err := c2.Pull([]string{"*/*"}); err != nil {
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
	if err := c2.Delete([]string{"delta"}); err != nil {
		t.Fatalf("c2.Delete: %v", err)
	}
	t.Log("CLIENT 2 Import -> charlie")
	if n, err := c2.ImportFiles([]string{filepath.Join(testdir, "*")}, "charlie"); err != nil {
		t.Errorf("c2.ImportFiles: %v", err)
	} else if want, got := 5, n; want != got {
		t.Errorf("Unexpected ImportFiles result. Want %d, got %d", want, got)
	}
	t.Log("CLIENT 2 Move alpha/image000.jpg charlie/image100.jpg -> beta")
	if err := c2.Move([]string{"alpha/image000.jpg", "charlie/image100.jpg"}, "beta"); err != nil {
		t.Fatalf("c2.Move: %v", err)
	}
	want = []string{
		"alpha",
		"beta",
		"charlie LOCAL",
		"gallery",
		"trash",
		"alpha/image001.jpg",
		"alpha/image002.jpg",
		"alpha/image003.jpg",
		"alpha/image004.jpg",
		"beta/image000.jpg LOCAL",
		"beta/image100.jpg LOCAL",
		"charlie/image101.jpg LOCAL",
		"charlie/image102.jpg LOCAL",
		"charlie/image103.jpg LOCAL",
		"charlie/image104.jpg LOCAL",
	}
	if got, err = globAll(c2); err != nil {
		t.Fatalf("globAll: %v", err)
	}
	if diff := deep.Equal(want, got); diff != nil {
		t.Fatalf("Unexpected file list. Diff: %v", diff)
	}

	t.Log("CLIENT 1 Move alpha/* -> delta")
	if err := c1.Move([]string{"alpha/*"}, "delta"); err != nil {
		t.Fatalf("c1.Move: %v", err)
	}
	t.Log("CLIENT 1 Delete alpha beta")
	if err := c1.Delete([]string{"alpha", "beta"}); err != nil {
		t.Fatalf("c1.Delete: %v", err)
	}
	t.Log("CLIENT 1 Sync")
	if err := c1.Sync(false); err != nil {
		t.Fatalf("c1.Sync: %v", err)
	}
	want = []string{
		"delta",
		"gallery",
		"trash",
		"delta/image000.jpg",
		"delta/image001.jpg",
		"delta/image002.jpg",
		"delta/image003.jpg",
		"delta/image004.jpg",
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
		"beta",
		"charlie",
		"delta",
		"gallery",
		"trash",
		"beta/image000.jpg",
		"beta/image100.jpg",
		"charlie/image101.jpg",
		"charlie/image102.jpg",
		"charlie/image103.jpg",
		"charlie/image104.jpg",
		"delta/image000.jpg",
		"delta/image001.jpg",
		"delta/image002.jpg",
		"delta/image003.jpg",
		"delta/image004.jpg",
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
	_, done := startServer(t)
	defer done()

	c := make(map[string]*client.Client)
	for _, n := range []string{"alice", "bob", "carol", "dave"} {
		t.Logf("%s Login", n)
		var err error
		if c[n], err = newClient(t.TempDir()); err != nil {
			t.Fatalf("newClient: %v", err)
		}
		if err := c[n].CreateAccount(n+"@", n+"-pass", true); err != nil {
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
	if n, err := c["alice"].ImportFiles([]string{filepath.Join(testdir, "*")}, "alpha"); err != nil {
		t.Errorf("alice.ImportFiles: %v", err)
	} else if want, got := 5, n; want != got {
		t.Errorf("Unexpected ImportFiles result. Want %d, got %d", want, got)
	}
	t.Log("alice Sync")
	if err := c["alice"].Sync(false); err != nil {
		t.Fatalf("alice.Sync: %v", err)
	}
	t.Log("alice Share")
	if err := c["alice"].Share("alpha", []string{"bob@", "carol@", "dave@"}, nil); err != nil {
		t.Fatalf("alice.Share: %v", err)
	}

	for n, client := range c {
		t.Logf("%s GetUpdates", n)
		if err := client.GetUpdates(false); err != nil {
			t.Fatalf("%s.GetUpdates: %v", n, err)
		}
		want := []string{
			"alpha",
			"gallery",
			"trash",
			"alpha/image000.jpg",
			"alpha/image001.jpg",
			"alpha/image002.jpg",
			"alpha/image003.jpg",
			"alpha/image004.jpg",
		}
		got, err := globAll(client)
		if err != nil {
			t.Fatalf("globAll: %v", err)
		}
		if diff := deep.Equal(want, got); diff != nil {
			t.Fatalf("Unexpected file list. Diff: %v", diff)
		}
	}

	t.Log("bob Leave")
	if err := c["bob"].Leave([]string{"alpha"}); err != nil {
		t.Fatalf("bob.Leave: %v", err)
	}

	t.Log("bob GetUpdates")
	if err := c["bob"].GetUpdates(false); err != nil {
		t.Fatalf("bob.GetUpdates: %v", err)
	}
	want := []string{
		"gallery",
		"trash",
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
		"gallery",
		"trash",
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
		"gallery",
		"trash",
	}
	if got, err = globAll(c["dave"]); err != nil {
		t.Fatalf("globAll: %v", err)
	}
	if diff := deep.Equal(want, got); diff != nil {
		t.Fatalf("Unexpected file list. Diff: %v", diff)
	}
}
