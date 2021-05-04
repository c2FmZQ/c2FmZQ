// Package client implements the c2FmZQ Client functionality.
package client

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"c2FmZQ/internal/log"
	"c2FmZQ/internal/secure"
	"c2FmZQ/internal/stingle"
)

const (
	configFile   = "config"
	galleryFile  = "gallery"
	trashFile    = "trash"
	albumList    = "albums"
	albumPrefix  = "album/"
	contactsFile = "contacts"

	userAgent = "Dalvik/2.1.0 (Linux; U; Android 9; moto x4 Build/PPWS29.69-39-6-4)"
)

var (
	ErrNotLoggedIn = errors.New("not logged in")
)

// Create creates a new client configuration, if one doesn't exist already.
func Create(s *secure.Storage) (*Client, error) {
	var c Client
	c.hc = &http.Client{}
	c.storage = s
	c.writer = os.Stdout
	c.prompt = prompt
	c.LocalSecretKey = stingle.MakeSecretKey()

	if err := s.CreateEmptyFile(c.cfgFile(), &c); err != nil {
		return nil, err
	}
	if err := c.createEmptyFiles(); err != nil {
		return nil, err
	}
	return &c, nil
}

// Load loads the existing client configuration.
func Load(s *secure.Storage) (*Client, error) {
	var c Client
	c.storage = s
	if err := s.ReadDataFile(c.cfgFile(), &c); err != nil {
		return nil, err
	}
	c.hc = &http.Client{}
	c.writer = os.Stdout
	c.prompt = prompt
	c.createEmptyFiles()
	return &c, nil
}

// Client contains the metadata for a user account.
type Client struct {
	Account        *AccountInfo      `json:"accountInfo"`
	LocalSecretKey stingle.SecretKey `json:"localSecretKey"`

	hc *http.Client

	storage *secure.Storage
	writer  io.Writer
	prompt  func(msg string) (string, error)
}

// AccountInfo encapsulated the information for a logged in account.
type AccountInfo struct {
	Email           string            `json:"email"`
	Salt            []byte            `json:"salt"`
	HashedPassword  string            `json:"hashedPassword"`
	SecretKey       stingle.SecretKey `json:"secretKey"`
	IsBackedUp      bool              `json:"isBackedUp"`
	ServerBaseURL   string            `json:"serverBaseURL"`
	UserID          int64             `json:"userID"`
	ServerPublicKey stingle.PublicKey `json:"serverPublicKey"`
	Token           string            `json:"token"`
}

// Save saves the current client configuration.
func (c *Client) Save() error {
	return c.storage.SaveDataFile(c.cfgFile(), c)
}

func (c *Client) cfgFile() string {
	cfg := c.storage.HashString(configFile)
	return filepath.Join(cfg[:2], cfg)
}

// SecretKey returns the current secret key.
func (c *Client) SecretKey() stingle.SecretKey {
	if c.Account != nil {
		return c.Account.SecretKey
	}
	return c.LocalSecretKey
}

// Status returns the client's current status.
func (c *Client) Status() error {
	if c.Account == nil {
		c.Print("Not logged in.")
	} else {
		c.Printf("Logged in as %s on %s.\n", c.Account.Email, c.Account.ServerBaseURL)
		if c.Account.IsBackedUp {
			c.Printf("Secret key is backed up.\n")
		} else {
			c.Printf("Secret key is NOT backed up.\n")
		}
	}
	c.Printf("Public key: % X\n", c.SecretKey().PublicKey().ToBytes())
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
	n := c.storage.HashString(hex.EncodeToString(c.SecretKey().ToBytes()) + "/" + fn)
	return filepath.Join(n[:2], n)
}

func (c *Client) encodeParams(params map[string]string) string {
	j, _ := json.Marshal(params)
	return stingle.EncryptMessage(j, c.Account.ServerPublicKey, c.Account.SecretKey)
}

func (c *Client) sendRequest(uri string, form url.Values, server string) (*stingle.Response, error) {
	if server == "" && c.Account != nil {
		server = c.Account.ServerBaseURL
	}
	if server == "" {
		return nil, errors.New("ServerBaseURL is not set")
	}
	url := strings.TrimSuffix(server, "/") + uri

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
	if log.Level >= log.DebugLevel {
		var line []string
		line = append(line, fmt.Sprintf("Response: %s", sr.Status))
		if sr.Parts != nil {
			line = append(line, fmt.Sprintf(" Parts:%v", sr.Parts))
		}
		if len(sr.Infos) > 0 {
			line = append(line, fmt.Sprintf(" Infos:%v", sr.Infos))
		}
		if len(sr.Errors) > 0 {
			line = append(line, fmt.Sprintf(" Errors:%v", sr.Errors))
		}
		log.Debug(strings.Join(line, ""))
	}
	return &sr, nil
}

func (c *Client) download(file, set, thumb string) (io.ReadCloser, error) {
	if c.Account == nil {
		return nil, ErrNotLoggedIn
	}
	if c.Account.ServerBaseURL == "" {
		return nil, errors.New("ServerBaseURL is not set")
	}
	form := url.Values{}
	form.Set("token", c.Account.Token)
	form.Set("file", file)
	form.Set("set", set)
	form.Set("thumb", thumb)

	url := strings.TrimSuffix(c.Account.ServerBaseURL, "/") + "/v2/sync/download"

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

func (c *Client) createEmptyFiles() (err error) {
	if e := c.storage.CreateEmptyFile(c.fileHash(galleryFile), &FileSet{}); err == nil {
		err = e
	}
	if e := c.storage.CreateEmptyFile(c.fileHash(trashFile), &FileSet{}); err == nil {
		err = e
	}
	if e := c.storage.CreateEmptyFile(c.fileHash(albumList), &AlbumList{}); err == nil {
		err = e
	}
	if e := c.storage.CreateEmptyFile(c.fileHash(contactsFile), &ContactList{}); err == nil {
		err = e
	}
	return
}

func prompt(msg string) (reply string, err error) {
	fmt.Print(msg)
	reply, err = bufio.NewReader(os.Stdin).ReadString('\n')
	reply = strings.TrimSpace(reply)
	return
}