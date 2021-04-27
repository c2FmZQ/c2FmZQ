// +build !nacl,!arm

package client_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-test/deep"
)

func TestLoginLogout(t *testing.T) {
	c, done := startServer(t)
	defer done()

	if err := login(c, "alice@", "pass"); err != nil {
		t.Fatalf("login: %v", err)
	}
	if err := c.Logout(); err != nil {
		t.Fatalf("c.Logout: %v", err)
	}
}

func TestImportExportSync(t *testing.T) {
	c, done := startServer(t)
	defer done()
	if err := login(c, "alice@", "pass"); err != nil {
		t.Fatalf("login: %v", err)
	}

	testdir := t.TempDir()
	if err := makeImages(testdir, 0, 10); err != nil {
		t.Fatalf("makeImages: %v", err)
	}
	if n, err := c.ImportFiles([]string{filepath.Join(testdir, "*")}, "gallery"); err != nil {
		t.Errorf("c.ImportFiles: %v", err)
	} else if want, got := 10, n; want != got {
		t.Errorf("Unexpected ImportFiles result. Want %d, got %d", want, got)
	}
	if n, err := c.ImportFiles([]string{filepath.Join(testdir, "*0.jpg")}, "gallery"); err != nil {
		t.Errorf("c.ImportFiles: %v", err)
	} else if want, got := 0, n; want != got {
		t.Errorf("Unexpected ImportFiles result. Want %d, got %d", want, got)
	}

	if err := c.ListFiles([]string{"gallery/*"}); err != nil {
		t.Errorf("c.ListFiles: %v", err)
	}

	exportDir := filepath.Join(testdir, "export")
	if err := os.Mkdir(exportDir, 0700); err != nil {
		t.Fatalf("os.Mkdir: %v", err)
	}
	if n, err := c.ExportFiles([]string{"gallery/*"}, exportDir); err != nil {
		t.Errorf("c.ExportFiles: %v", err)
	} else if want, got := 10, n; want != got {
		t.Errorf("Unexpected ExportFiles result. Want %d, got %d", want, got)
	}

	if err := c.Sync(true); err != nil {
		t.Errorf("c.Sync: %v", err)
	}
	if err := c.Sync(false); err != nil {
		t.Errorf("c.Sync: %v", err)
	}

	if err := c.GetUpdates(false); err != nil {
		t.Errorf("c.GetUpdates: %v", err)
	}

	if n, err := c.Free([]string{"gallery/*"}); err != nil {
		t.Errorf("c.Free: %v", err)
	} else if want, got := 10, n; want != got {
		t.Errorf("Unexpected Free result. Want %d, got %d", want, got)
	}

	if n, err := c.Pull([]string{"gallery/*0.jpg"}); err != nil {
		t.Errorf("c.Pull: %v", err)
	} else if want, got := 1, n; want != got {
		t.Errorf("Unexpected Pull result. Want %d, got %d", want, got)
	}
	if n, err := c.Pull([]string{"gallery/*"}); err != nil {
		t.Errorf("c.Pull: %v", err)
	} else if want, got := 9, n; want != got {
		t.Errorf("Unexpected Pull result. Want %d, got %d", want, got)
	}
}

func TestCopyMoveDelete(t *testing.T) {
	c, done := startServer(t)
	defer done()
	if err := login(c, "alice@", "pass"); err != nil {
		t.Fatalf("login: %v", err)
	}

	testdir := t.TempDir()
	if err := makeImages(testdir, 0, 5); err != nil {
		t.Fatalf("makeImages: %v", err)
	}
	if n, err := c.ImportFiles([]string{filepath.Join(testdir, "*")}, "gallery"); err != nil {
		t.Errorf("c.ImportFiles: %v", err)
	} else if want, got := 5, n; want != got {
		t.Errorf("Unexpected ImportFiles result. Want %d, got %d", want, got)
	}

	if err := c.AddAlbums([]string{"alpha", "beta", "charlie"}); err != nil {
		t.Fatalf("AddAlbums: %v", err)
	}

	if err := c.Copy([]string{"gallery/image00[0-1].jpg"}, "alpha"); err != nil {
		t.Fatalf("c.Copy: %v", err)
	}

	if err := c.Move([]string{"gallery/image00[2-3].jpg"}, "beta"); err != nil {
		t.Fatalf("c.Move: %v", err)
	}

	want := []string{
		"alpha/ LOCAL",
		"beta/ LOCAL",
		"charlie/ LOCAL",
		"gallery/",
		"trash/",
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

	if err := c.Delete([]string{"alpha/image000.jpg", "gallery/image004.jpg"}); err != nil {
		t.Fatalf("c.Delete: %v", err)
	}

	want = []string{
		"alpha/ LOCAL",
		"beta/ LOCAL",
		"charlie/ LOCAL",
		"gallery/",
		"trash/",
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

	if err := c.Delete([]string{"trash/*"}); err != nil {
		t.Fatalf("c.Delete: %v", err)
	}

	want = []string{
		"alpha/ LOCAL",
		"beta/ LOCAL",
		"charlie/ LOCAL",
		"gallery/",
		"trash/",
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
	if err := c.Delete([]string{"alpha"}); err == nil {
		t.Fatal("c.Delete succeeded unexpectedly.")
	}
	// Delete charlie should succeed because it is empty.
	if err := c.Delete([]string{"charlie"}); err != nil {
		t.Fatalf("c.Delete: %v", err)
	}

	if err := c.Sync(false); err != nil {
		t.Fatalf("c.Sync: %v", err)
	}

	want = []string{
		"alpha/",
		"beta/",
		"gallery/",
		"trash/",
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
