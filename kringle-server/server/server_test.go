package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"kringle-server/crypto"
	"kringle-server/database"
	"kringle-server/log"
	"kringle-server/server"
	"kringle-server/stingle"
)

// startServer starts a server listening on a unix socket. Returns the unix socket
// and a function to shutdown the server.
func startServer(t *testing.T) (string, func()) {
	testdir := t.TempDir()
	sock := filepath.Join(testdir, "server.sock")
	log.Record = t.Log
	log.Level = 3
	db := database.New(filepath.Join(testdir, "data"), "")
	s := server.New(db, "")
	s.AllowCreateAccount = true
	s.BaseURL = "http://unix/"
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("net.Listen failed: %v", err)
	}
	go s.RunWithListener(l)
	return sock, func() {
		s.Shutdown()
		log.Record = nil
	}
}

// newClient returns a new test client that uses sock to connect to the server.
func newClient(sock string) *client {
	sk := crypto.MakeSecretKey()
	return &client{
		sock:      sock,
		secretKey: sk,
	}
}

type client struct {
	sock string

	userID          int64
	email           string
	password        string
	salt            string
	isBackup        string
	secretKey       crypto.SecretKey
	serverPublicKey crypto.PublicKey
	keyBundle       string
	token           string
}

func (c *client) encodeParams(params map[string]string) string {
	j, _ := json.Marshal(params)
	return crypto.EncryptMessage(j, c.serverPublicKey, c.secretKey)
}

func nowString() string {
	return fmt.Sprintf("%d", time.Now().UnixNano()/1000000)
}

// A Dialer that always connects to the same unix socket.
type dialer struct {
	net.Dialer
	sock string
}

func (d dialer) DialContext(ctx context.Context, _, _ string) (net.Conn, error) {
	return d.Dialer.DialContext(ctx, "unix", d.sock)
}

func (c *client) sendRequest(uri string, form url.Values) (*stingle.Response, error) {
	dialer := dialer{sock: c.sock}
	hc := http.Client{Transport: &http.Transport{DialContext: dialer.DialContext}}

	log.Debugf("SEND POST %s", uri)
	log.Debugf(" %v", form)
	resp, err := hc.PostForm("http://unix"+uri, form)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request returned status code %d", resp.StatusCode)
	}
	dec := json.NewDecoder(resp.Body)
	dec.UseNumber()
	var sr stingle.Response
	if err := dec.Decode(&sr); err != nil {
		return nil, err
	}
	return &sr, nil
}

func (c *client) uploadFile(filename, set, albumID string, t int64) (*stingle.Response, error) {
	dialer := dialer{sock: c.sock}
	hc := http.Client{Transport: &http.Transport{DialContext: dialer.DialContext}}

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for _, f := range []string{"file", "thumb"} {
		pw, err := w.CreateFormFile(f, filename)
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(pw, "Content of %q filename %q", f, filename)
	}
	ts := fmt.Sprintf("%d", t)
	for _, f := range []struct{ name, value string }{
		{"headers", fmt.Sprintf("%s headers %s", filename, albumID)},
		{"set", set},
		{"albumId", albumID},
		{"dateCreated", ts},
		{"dateModified", ts},
		{"version", "1"},
		{"token", c.token},
	} {
		pw, err := w.CreateFormField(f.name)
		if err != nil {
			return nil, err
		}
		fmt.Fprint(pw, f.value)
	}
	if err := w.Close(); err != nil {
		return nil, err
	}

	log.Debugf("SEND POST /v2/sync/upload (%q, %q, %q)", filename, set, albumID)

	resp, err := hc.Post("http://unix/v2/sync/upload", w.FormDataContentType(), &buf)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request returned status code %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var sr stingle.Response
	if err := json.Unmarshal(body, &sr); err != nil {
		return nil, err
	}

	return &sr, nil
}

func (c *client) downloadPost(file, set, isThumb string) (string, error) {
	form := url.Values{}
	form.Set("token", c.token)
	form.Set("file", file)
	form.Set("set", set)
	form.Set("thumb", isThumb)

	dialer := dialer{sock: c.sock}
	hc := http.Client{Transport: &http.Transport{DialContext: dialer.DialContext}}

	log.Debug("SEND POST /v2/sync/download")
	log.Debugf(" %v", form)
	resp, err := hc.PostForm("http://unix/v2/sync/download", form)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("request returned status code %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func (c *client) downloadGet(url string) (string, error) {
	dialer := dialer{sock: c.sock}
	hc := http.Client{Transport: &http.Transport{DialContext: dialer.DialContext}}

	log.Debugf("SEND GET %s", url)
	resp, err := hc.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("request returned status code %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}
