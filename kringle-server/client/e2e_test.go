package client_test

import (
	"fmt"
	"image"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"kringle-server/client"
	"kringle-server/crypto"
	"kringle-server/database"
	"kringle-server/log"
	"kringle-server/secure"
	"kringle-server/server"
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
	if err := makeImages(testdir, 10); err != nil {
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

func startServer(t *testing.T) (*client.Client, func()) {
	testdir := t.TempDir()
	log.Record = t.Log
	log.Level = 3
	db := database.New(filepath.Join(testdir, "data"), "")
	s := server.New(db, "", "")
	s.AllowCreateAccount = true

	srv := httptest.NewServer(s.Handler())
	c := newClient(t, srv.Client())
	c.ServerBaseURL = srv.URL
	return c, srv.Close
}

func newClient(t *testing.T, hc *http.Client) *client.Client {
	testdir := t.TempDir()
	masterKey, err := crypto.CreateMasterKey()
	if err != nil {
		t.Fatalf("Failed to create master key: %v", err)
	}
	storage := secure.NewStorage(testdir, &masterKey.EncryptionKey)
	c, err := client.Create(storage)
	if err != nil {
		t.Fatalf("client.Create: %v", err)
	}
	c.SetHTTPClient(hc)
	return c
}

func login(c *client.Client, email, password string) error {
	if err := c.CreateAccount(email, password); err != nil {
		return err
	}
	return c.Login(email, password)
}

func makeImages(dir string, n int) error {
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for i := 0; i < n; i++ {
		fn := filepath.Join(dir, fmt.Sprintf("image%d.jpg", i))
		f, err := os.Create(fn)
		if err != nil {
			return err
		}
		if err := jpeg.Encode(f, img, &jpeg.Options{Quality: 70}); err != nil {
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
	}
	return nil
}
