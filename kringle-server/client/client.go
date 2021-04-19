package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"kringle-server/log"
	"kringle-server/secure"
	"kringle-server/stingle"
)

const (
	configFile = ".config"
)

// Create creates a new client configuration, if one doesn't exist already.
func Create(s *secure.Storage) (*Client, error) {
	var c Client
	if err := s.CreateEmptyFile(configFile, &c); err != nil {
		return nil, err
	}
	c.storage = s
	return &c, nil
}

// Load loads the existing client configuration.
func Load(s *secure.Storage) (*Client, error) {
	var c Client
	if _, err := s.ReadDataFile(configFile, &c); err != nil {
		return nil, err
	}
	c.storage = s
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

	storage *secure.Storage
}

func (c *Client) encodeParams(params map[string]string) string {
	j, _ := json.Marshal(params)
	return stingle.EncryptMessage(j, c.ServerPublicKey, c.SecretKey)
}

func (c *Client) sendRequest(uri string, form url.Values) (*stingle.Response, error) {
	if c.ServerBaseURL == "" {
		return nil, errors.New("ServerBaseURL is not set")
	}
	hc := http.Client{}
	url := c.ServerBaseURL + uri

	log.Debugf("SEND POST %v", url)
	log.Debugf(" %v", form)
	resp, err := hc.PostForm(url, form)
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
	log.Infof("Response: %v", sr)
	return &sr, nil
}
