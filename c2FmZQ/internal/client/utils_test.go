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
	"fmt"
	"image"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"c2FmZQ/internal/client"
	"c2FmZQ/internal/crypto"
	"c2FmZQ/internal/database"
	"c2FmZQ/internal/log"
	"c2FmZQ/internal/secure"
	"c2FmZQ/internal/server"
)

var (
	hc *http.Client
)

func startServer(t *testing.T) (*client.Client, string, func()) {
	testdir := t.TempDir()
	log.Record = t.Log
	log.Level = 2
	db := database.New(filepath.Join(testdir, "data"), nil)
	s := server.New(db, "", "", "")
	s.AllowCreateAccount = true

	srv := httptest.NewServer(s.Handler())
	hc = srv.Client()
	c, err := newClient(t.TempDir())
	if err != nil {
		t.Fatalf("newClient: %v", err)
	}
	return c, srv.URL, srv.Close
}

func newClient(dir string) (*client.Client, error) {
	masterKey, err := crypto.CreateAESMasterKeyForTest()
	if err != nil {
		return nil, err
	}
	storage := secure.NewStorage(dir, masterKey)
	c, err := client.Create(masterKey, storage)
	if err != nil {
		return nil, err
	}
	c.SetHTTPClient(hc)
	return c, nil
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
	li, err := c.GlobFiles([]string{"*"}, client.GlobOptions{MatchDot: true, Recursive: true})
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
	return out, nil
}
