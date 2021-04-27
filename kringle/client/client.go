package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"kringle/log"
	"kringle/secure"
	"kringle/stingle"
)

const (
	configFile   = "config"
	galleryFile  = "gallery"
	trashFile    = "trash"
	albumList    = "albums"
	albumPrefix  = "album/"
	contactsFile = "contacts"
	blobsDir     = "blobs"
)

// Create creates a new client configuration, if one doesn't exist already.
func Create(s *secure.Storage) (*Client, error) {
	var c Client
	if err := s.CreateEmptyFile(s.HashString(configFile), &c); err != nil {
		return nil, err
	}
	c.hc = &http.Client{}
	c.storage = s
	c.writer = os.Stdout
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
	return &c, nil
}

// Client contains the metadata for a user account.
type Client struct {
	UserID          int64             `json:"userID"`
	Email           string            `json:"email"`
	SecretKey       stingle.SecretKey `json:"secretKey"`
	ServerPublicKey stingle.PublicKey `json:"serverPublicKey"`
	Token           string            `json:"token"`

	ServerBaseURL string `json:"serverBaseURL"`
	HomeDir       string `json:"homeDir"`

	hc *http.Client

	storage *secure.Storage
	writer  io.Writer
}

func (c *Client) SetWriter(w io.Writer) {
	c.writer = w
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

	log.Debugf("SEND POST %v", url)
	log.Debugf(" %v", form)
	resp, err := c.hc.PostForm(url, form)
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
	resp, err := c.hc.PostForm(url, form)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("request returned status code %d", resp.StatusCode)
	}
	return resp.Body, nil
}
