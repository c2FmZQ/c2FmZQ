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

	"kringle/internal/crypto"
	"kringle/internal/database"
	"kringle/internal/log"
	"kringle/internal/secure"
	"kringle/internal/client"
	"kringle/internal/server"
)

var (
	hc  *http.Client
	url string
)

func startServer(t *testing.T) (*client.Client, func()) {
	testdir := t.TempDir()
	log.Record = t.Log
	log.Level = 2
	db := database.New(filepath.Join(testdir, "data"), "")
	s := server.New(db, "", "")
	s.AllowCreateAccount = true

	srv := httptest.NewServer(s.Handler())
	hc = srv.Client()
	url = srv.URL
	c, err := newClient(t.TempDir())
	if err != nil {
		t.Fatalf("newClient: %v", err)
	}
	return c, srv.Close
}

func newClient(dir string) (*client.Client, error) {
	masterKey, err := crypto.CreateMasterKey()
	if err != nil {
		return nil, err
	}
	storage := secure.NewStorage(dir, &masterKey.EncryptionKey)
	c, err := client.Create(storage)
	if err != nil {
		return nil, err
	}
	c.SetHTTPClient(hc)
	c.ServerBaseURL = url
	return c, nil
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
