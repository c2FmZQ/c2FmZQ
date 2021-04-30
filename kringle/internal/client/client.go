// Package client implements the Kringle Client functionality.
package client

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"kringle/internal/log"
	"kringle/internal/secure"
	"kringle/internal/stingle"
)

const (
	configFile   = "config"
	galleryFile  = "gallery"
	trashFile    = "trash"
	albumList    = "albums"
	albumPrefix  = "album/"
	contactsFile = "contacts"
	blobsDir     = "blobs"

	userAgent = "Dalvik/2.1.0 (Linux; U; Android 9; moto x4 Build/PPWS29.69-39-6-4)"
)

// Create creates a new client configuration, if one doesn't exist already.
func Create(s *secure.Storage) (*Client, error) {
	var c Client
	c.hc = &http.Client{}
	c.storage = s
	c.writer = os.Stdout
	c.prompt = prompt
	c.LocalSecretKey = stingle.MakeSecretKey()
	c.SecretKey = c.LocalSecretKey

	if err := s.CreateEmptyFile(s.HashString(configFile), &c); err != nil {
		return nil, err
	}
	if err := c.storage.CreateEmptyFile(c.fileHash(galleryFile), &FileSet{}); err != nil {
		return nil, err
	}
	if err := c.storage.CreateEmptyFile(c.fileHash(trashFile), &FileSet{}); err != nil {
		return nil, err
	}
	if err := c.storage.CreateEmptyFile(c.fileHash(albumList), &AlbumList{}); err != nil {
		return nil, err
	}
	if err := c.storage.CreateEmptyFile(c.fileHash(contactsFile), &ContactList{}); err != nil {
		return nil, err
	}
	return &c, nil
}

// Load loads the existing client configuration.
func Load(s *secure.Storage) (*Client, error) {
	var c Client
	if _, err := s.ReadDataFile(s.HashString(configFile), &c); err != nil {
		return nil, err
	}
	c.hc = &http.Client{}
	c.storage = s
	c.writer = os.Stdout
	c.prompt = prompt
	c.storage.CreateEmptyFile(c.fileHash(galleryFile), &FileSet{})
	c.storage.CreateEmptyFile(c.fileHash(trashFile), &FileSet{})
	c.storage.CreateEmptyFile(c.fileHash(albumList), &AlbumList{})
	c.storage.CreateEmptyFile(c.fileHash(contactsFile), &ContactList{})
	return &c, nil
}

// Client contains the metadata for a user account.
type Client struct {
	UserID          int64             `json:"userID"`
	Email           string            `json:"email"`
	Salt            []byte            `json:"salt"`
	HashedPassword  string            `json:"hashedPassword"`
	SecretKey       stingle.SecretKey `json:"secretKey"`
	ServerPublicKey stingle.PublicKey `json:"serverPublicKey"`
	IsBackedUp      bool              `json:"isBackedUp"`
	Token           string            `json:"token"`

	ServerBaseURL  string            `json:"serverBaseURL"`
	LocalSecretKey stingle.SecretKey `json:"localSecretKey"`

	hc *http.Client

	storage *secure.Storage
	writer  io.Writer
	prompt  func(msg string) (string, error)
}

// Save saves the current client configuration.
func (c *Client) Save() error {
	return c.storage.SaveDataFile(nil, c.storage.HashString(configFile), c)
}

// Status returns the client's current status.
func (c *Client) Status() error {
	if c.Email == "" {
		c.Print("Not logged in.")
		return nil
	}
	c.Printf("Logged in as %s on %s. ", c.Email, c.ServerBaseURL)
	if c.IsBackedUp {
		c.Printf("Secret key is backed up.\n")
	} else {
		c.Printf("Secret key is NOT backed up.\n")
	}
	return nil
}

func (c *Client) SetWriter(w io.Writer) {
	c.writer = w
}

func (c *Client) SetPrompt(f func(msg string) (string, error)) {
	c.prompt = f
}

func (c *Client) SetHTTPClient(hc *http.Client) {
	c.hc = hc
}

func (c *Client) Printf(format string, args ...interface{}) {
	fmt.Fprintf(c.writer, format, args...)
}

func (c *Client) Print(args ...interface{}) {
	fmt.Fprintln(c.writer, args...)
}

func nowString() string {
	return fmt.Sprintf("%d", time.Now().UnixNano()/1000000)
}

func nowJSON() json.Number {
	return json.Number(nowString())
}

func (c *Client) fileHash(fn string) string {
	if c.Email == "" {
		return c.storage.HashString("local/" + fn)
	}
	return c.storage.HashString(c.ServerBaseURL + "/" + c.Email + "/" + fn)
}

func (c *Client) encodeParams(params map[string]string) string {
	j, _ := json.Marshal(params)
	return stingle.EncryptMessage(j, c.ServerPublicKey, c.SecretKey)
}

func (c *Client) sendRequest(uri string, form url.Values) (*stingle.Response, error) {
	c.ServerBaseURL = strings.TrimSuffix(c.ServerBaseURL, "/")
	if c.ServerBaseURL == "" {
		return nil, errors.New("ServerBaseURL is not set")
	}
	url := c.ServerBaseURL + uri

	log.Debugf("SEND POST %s", url)
	log.Debugf(" %v", form)

	req, err := http.NewRequest("POST", url, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.hc.Do(req)
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
	log.Debugf("Response: %v", sr)
	return &sr, nil
}

func (c *Client) download(file, set, thumb string) (io.ReadCloser, error) {
	c.ServerBaseURL = strings.TrimSuffix(c.ServerBaseURL, "/")
	if c.ServerBaseURL == "" {
		return nil, errors.New("ServerBaseURL is not set")
	}
	form := url.Values{}
	form.Set("token", c.Token)
	form.Set("file", file)
	form.Set("set", set)
	form.Set("thumb", thumb)

	url := c.ServerBaseURL + "/v2/sync/download"

	log.Debugf("SEND POST %v", url)
	log.Debugf(" %v", form)

	req, err := http.NewRequest("POST", url, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("request returned status code %d", resp.StatusCode)
	}
	return resp.Body, nil
}

func prompt(msg string) (reply string, err error) {
	fmt.Print(msg)
	reply, err = bufio.NewReader(os.Stdin).ReadString('\n')
	reply = strings.TrimSpace(reply)
	return
}
