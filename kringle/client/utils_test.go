package client_test

import (
	"fmt"
	"image"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"kringle/client"
	"kringle/crypto"
	"kringle/database"
	"kringle/log"
	"kringle/secure"
	"kringle/server"
)

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

func makeImages(dir string, start, n int) error {
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for i := start; i < start+n; i++ {
		fn := filepath.Join(dir, fmt.Sprintf("image%03d.jpg", i))
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

func globAll(c *client.Client) ([]string, error) {
	var out []string
	for _, p := range []string{"*", "*/*"} {
		li, err := c.GlobFiles([]string{p})
		if err != nil {
			return nil, err
		}
		var list []string
		for _, item := range li {
			line := item.Filename
			if item.LocalOnly {
				line += " LOCAL"
			}
			list = append(list, line)
		}
		sort.Strings(list)
		out = append(out, list...)
	}
	return out, nil
}
