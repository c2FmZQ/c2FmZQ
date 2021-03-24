package server_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"testing"

	"stingle-server/crypto"
	"stingle-server/database"
	"stingle-server/log"
	"stingle-server/server"
)

// startServer starts a server listening on a unix socket. Returns the unix socket
// and a function to shutdown the server.
func startServer(t *testing.T) (string, func() error) {
	testdir := t.TempDir()
	sock := filepath.Join(testdir, "server.sock")
	log.Level = 3
	db := database.New(filepath.Join(testdir, "data"))
	s := server.New(db, "")
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("net.Listen failed: %v", err)
	}
	go s.RunWithListener(l)
	return sock, s.Shutdown
}

// newClient returns a new test client that uses sock to connect to the server.
func newClient(t *testing.T, sock string) *client {
	sk := crypto.MakeSecretKey()
	return &client{
		t:         t,
		sock:      sock,
		secretKey: sk,
	}
}

type client struct {
	t    *testing.T
	sock string

	userID          int
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

// A Dialer that always connects to the same unix socket.
type dialer struct {
	net.Dialer
	sock string
}

func (d dialer) DialContext(ctx context.Context, _, _ string) (net.Conn, error) {
	return d.Dialer.DialContext(ctx, "unix", d.sock)
}

func (c *client) sendRequest(uri string, form url.Values) (server.StingleResponse, error) {
	dialer := dialer{sock: c.sock}
	hc := http.Client{Transport: &http.Transport{DialContext: dialer.DialContext}}

	c.t.Logf("POST %s", uri)
	c.t.Logf(" %v", form)
	var sr server.StingleResponse
	resp, err := hc.PostForm("http://unix"+uri, form)
	if err != nil {
		return sr, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return sr, fmt.Errorf("request returned status code %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return sr, err
	}
	if err := json.Unmarshal(body, &sr); err != nil {
		return sr, err
	}
	return sr, nil
}
