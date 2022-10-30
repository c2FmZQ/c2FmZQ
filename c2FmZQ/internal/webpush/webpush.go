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

// Package webpush implements the Push API to send push notifications to users.
package webpush

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"c2FmZQ/internal/log"
)

// pushServiceID identifies a push service.
type pushServiceID int

const (
	google  pushServiceID = 1
	mozilla pushServiceID = 2
	windows pushServiceID = 3
	apple   pushServiceID = 4
)

// DefaultPushServiceConfiguration creates a default PushServiceConfiguration.
func DefaultPushServiceConfiguration() *PushServiceConfiguration {
	var c PushServiceConfiguration

	c.Enable = false
	c.JWTSubject = "https://..."

	c.Google.Enable = true
	c.Google.Regexp = `^https://fcm\.googleapis\.com/.*`
	c.Google.RateLimit = 10.0

	c.Mozilla.Enable = true
	c.Mozilla.Regexp = `^https://updates\.push\.services\.mozilla\.com/.*`
	c.Mozilla.RateLimit = 10.0

	c.Windows.Enable = false
	c.Windows.Regexp = `^https://[^/]*\.notify\.windows\.com/.*`
	c.Windows.RateLimit = 10.0

	c.Apple.Enable = true
	c.Apple.Regexp = `^https://[^/]*\.push\.apple\.com/.*`
	c.Apple.RateLimit = 10.0

	if err := c.Init(nil); err != nil {
		panic(err)
	}
	return &c
}

// PushServiceConfiguration encapsulates the push service options and the logic
// to send push notification via push services.
type PushServiceConfiguration struct {
	// Enable controls whether push notifications are enabled at all.
	Enable bool `json:"enable"`

	// JWTSubject is the value to assign to the "sub" claim in VAPID JWTs.
	// This is typically a URI, e.g. mailto: or https://...
	JWTSubject string `json:"jwtSubject"`

	// Google contains the options for Google's FCM service.
	Google struct {
		Enable    bool    `json:"enable"`
		Regexp    string  `json:"regexp"`
		RateLimit float64 `json:"rate_limit"`

		re *regexp.Regexp
		rl *rate.Limiter
	} `json:"google"`

	// Mozilla contains the options for Mozilla's push service.
	Mozilla struct {
		Enable    bool    `json:"enable"`
		Regexp    string  `json:"regexp"`
		RateLimit float64 `json:"rate_limit"`

		re *regexp.Regexp
		rl *rate.Limiter
	} `json:"mozilla"`

	// Windows contains the options for Azure's WNS push service.
	Windows struct {
		Enable          bool    `json:"enable"`
		Regexp          string  `json:"regexp"`
		RateLimit       float64 `json:"rate_limit"`
		PackageSID      string  `json:"packageSid"`
		SecretKey       string  `json:"secretKey"`
		AccessToken     string  `json:"accessToken,omitempty"`
		AccessTokenTime int64   `json:"accessTokenTime,omitempty"`

		re *regexp.Regexp
		mu sync.Mutex // guards AccessToken
		rl *rate.Limiter
	} `json:"windows"`

	// Apple contains the options for Apple's push service.
	Apple struct {
		Enable    bool    `json:"enable"`
		Regexp    string  `json:"regexp"`
		RateLimit float64 `json:"rate_limit"`

		re *regexp.Regexp
		rl *rate.Limiter
	} `json:"apple"`

	save     func(*PushServiceConfiguration) error
	client   *http.Client
	client11 *http.Client

	jwtCacheMu sync.Mutex
	jwtCache   map[string]jwtCacheEntry
}

type jwtCacheEntry struct {
	token        string
	refreshAfter time.Time
}

// Init initialized PushServiceConfiguration's internal fields.
func (c *PushServiceConfiguration) Init(saveFunc func(*PushServiceConfiguration) error) error {
	c.save = saveFunc
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ForceAttemptHTTP2 = false
	c.client = &http.Client{}
	c.client11 = &http.Client{
		Transport: &http.Transport{
			TLSNextProto: map[string]func(string, *tls.Conn) http.RoundTripper{},
		},
	}
	c.jwtCache = make(map[string]jwtCacheEntry)

	if c.Google.Enable {
		re, err := regexp.Compile(c.Google.Regexp)
		if err != nil {
			return err
		}
		c.Google.re = re
		c.Google.rl = rate.NewLimiter(rate.Limit(c.Google.RateLimit), int(math.Ceil(c.Google.RateLimit)))
	}
	if c.Mozilla.Enable {
		re, err := regexp.Compile(c.Mozilla.Regexp)
		if err != nil {
			return err
		}
		c.Mozilla.re = re
		c.Mozilla.rl = rate.NewLimiter(rate.Limit(c.Mozilla.RateLimit), int(math.Ceil(c.Mozilla.RateLimit)))
	}
	if c.Windows.Enable {
		re, err := regexp.Compile(c.Windows.Regexp)
		if err != nil {
			return err
		}
		c.Windows.re = re
		c.Windows.rl = rate.NewLimiter(rate.Limit(c.Windows.RateLimit), int(math.Ceil(c.Windows.RateLimit)))
	}
	if c.Apple.Enable {
		re, err := regexp.Compile(c.Apple.Regexp)
		if err != nil {
			return err
		}
		c.Apple.re = re
		c.Apple.rl = rate.NewLimiter(rate.Limit(c.Apple.RateLimit), int(math.Ceil(c.Apple.RateLimit)))
	}
	return nil
}

// matchService determines which push service to use for an endpoint.
func (c *PushServiceConfiguration) matchService(endpoint string) (pushServiceID, *rate.Limiter, error) {
	if c.Google.Enable && c.Google.re.MatchString(endpoint) {
		return google, c.Google.rl, nil
	}
	if c.Mozilla.Enable && c.Mozilla.re.MatchString(endpoint) {
		return mozilla, c.Mozilla.rl, nil
	}
	// Windows / Edge / WNS probably doesn't work correctly.
	if c.Windows.Enable && c.Windows.re.MatchString(endpoint) {
		return windows, c.Windows.rl, nil
	}
	if c.Apple.Enable && c.Apple.re.MatchString(endpoint) {
		return apple, c.Apple.rl, nil
	}
	return 0, nil, errors.New("no match")
}

// Params encapsulates the information needed to send the push notification.
type Params struct {
	// The notification service endpoint where the notification will be sent.
	Endpoint string
	// The server's base64-encoded ECDSA private key.
	ApplicationServerPrivateKey string
	// The server's base64-encoded ECDSA public key.
	ApplicationServerPublicKey string
	// The auth secret from the client's subscribe response.
	Auth string
	// Peer's ECDH base64-encoded public key.
	P256dh string
	// The payload to attach to the notification.
	Payload []byte
}

// Send sends a push notification.
func (c *PushServiceConfiguration) Send(ctx context.Context, params Params) (*http.Response, error) {
	service, rl, err := c.matchService(params.Endpoint)
	if err != nil {
		return nil, err
	}
	if err := rl.Wait(ctx); err != nil {
		return nil, err
	}
	req, err := c.makeRequest(ctx, params)
	if err != nil {
		return nil, err
	}
	if service == windows {
		// WNS requires HTTP/1.1
		return c.client11.Do(req)
	}
	return c.client.Do(req)
}

// makeRequest creates an http.Request for sending a push notification.
func (c *PushServiceConfiguration) makeRequest(ctx context.Context, params Params) (*http.Request, error) {
	serviceID, _, err := c.matchService(params.Endpoint)
	if err != nil {
		return nil, err
	}

	payload, salt, localKey, err := c.encryptPayload(params)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, params.Endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	if serviceID == windows {
		accessToken, err := c.getAccessToken(params)
		if err != nil {
			return nil, err
		}
		req.Header.Add("Authorization", "Bearer "+accessToken)
		req.Header.Set("X-WNS-Type", "wns/tile")
	} else {
		jwt, err := c.makeJWT(params)
		if err != nil {
			return nil, err
		}
		req.Header.Add("Authorization", "Bearer "+jwt)
	}
	req.Header.Set("Encryption", "salt="+salt)
	req.Header.Set("Crypto-Key", "p256ecdsa="+params.ApplicationServerPublicKey+";dh="+localKey)
	req.Header.Set("TTL", "86400")
	req.Header.Set("Urgency", "normal")
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Content-Encoding", "aesgcm")

	log.Debugf("URL: %s", req.URL.String())
	for k, vv := range req.Header {
		for _, v := range vv {
			log.Debugf("   %s: %s", k, v)
		}
	}

	return req, nil
}

// makeJWT creates a VAPID JWT auth token. Some push services don't like tokens
// that refresh more than once per hour. So, the tokens are valid for 2 hours,
// and refreshed after 1 hour.
func (c *PushServiceConfiguration) makeJWT(params Params) (string, error) {
	c.jwtCacheMu.Lock()
	defer c.jwtCacheMu.Unlock()
	cached, ok := c.jwtCache[params.ApplicationServerPublicKey]
	if ok && time.Now().Before(cached.refreshAfter) {
		return cached.token, nil
	}
	url, err := url.Parse(params.Endpoint)
	if err != nil {
		return "", err
	}
	url.Path = "/"
	hdrs, _ := json.Marshal(map[string]interface{}{
		"typ": "JWT",
		"alg": "ES256",
	})
	now := time.Now()
	claims, _ := json.Marshal(map[string]interface{}{
		"aud": url.String(),
		"iat": now.Unix(),
		"exp": now.Add(2 * time.Hour).Unix(),
		"sub": c.JWTSubject,
	})
	der, err := base64.RawURLEncoding.DecodeString(params.ApplicationServerPrivateKey)
	if err != nil {
		return "", err
	}
	key, err := x509.ParseECPrivateKey(der)
	if err != nil {
		return "", err
	}
	toSign := base64.RawURLEncoding.EncodeToString(hdrs) + "." + base64.RawURLEncoding.EncodeToString(claims)
	h := sha256.Sum256([]byte(toSign))
	r, s, err := ecdsa.Sign(rand.Reader, key, h[:])
	if err != nil {
		return "", err
	}
	nb := key.Curve.Params().BitSize / 8
	sig := make([]byte, 2*nb)
	r.FillBytes(sig[:nb])
	s.FillBytes(sig[nb:])

	cached.token = toSign + "." + base64.RawURLEncoding.EncodeToString(sig)
	cached.refreshAfter = time.Now().Add(time.Hour)
	c.jwtCache[params.ApplicationServerPublicKey] = cached
	return cached.token, nil
}

// getAccessToken fetches a new access token to use with WNS.
func (c *PushServiceConfiguration) getAccessToken(params Params) (string, error) {
	c.Windows.mu.Lock()
	defer c.Windows.mu.Unlock()
	if t := c.Windows.AccessToken; t != "" {
		return t, nil
	}
	resp, err := http.DefaultClient.PostForm("https://login.live.com/accesstoken.srf", url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {c.Windows.PackageSID},
		"client_secret": {c.Windows.SecretKey},
		"scope":         {"notify.windows.com"},
	})
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		var buf [1024]byte
		n, _ := io.ReadFull(resp.Body, buf[:])
		log.Errorf("access token response: %s %s", resp.Status, string(buf[:n]))
		return "", errors.New("request failed")
	}
	if ct := resp.Header.Get("content-type"); ct != "application/json" {
		log.Errorf("access token response content-type: %s", ct)
	}
	var data struct {
		AccessToken string `json:"access_token"`
		TokeyType   string `json:"token_type"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		log.Errorf("json decode: %v", err)
		return "", err
	}
	c.Windows.AccessToken = data.AccessToken
	c.Windows.AccessTokenTime = time.Now().Unix()
	if c.save != nil {
		if err := c.save(c); err != nil {
			log.Errorf("save: %v", err)
		}
	}
	return c.Windows.AccessToken, nil
}

// encryptPayload encrypts the notification's data payload. This is mostly based on
// https://developer.chrome.com/blog/web-push-encryption/
func (c *PushServiceConfiguration) encryptPayload(params Params) (encryptedPayload []byte, base64Salt, base64LocalKey string, err error) {
	peerKeyBytes, err := base64.RawURLEncoding.DecodeString(params.P256dh)
	if err != nil {
		return nil, "", "", err
	}
	authBytes, err := base64.RawURLEncoding.DecodeString(params.Auth)
	if err != nil {
		return nil, "", "", err
	}

	curve := ecdh.P256()
	peerKey, err := curve.NewPublicKey(peerKeyBytes)
	if err != nil {
		return nil, "", "", err
	}
	localKey, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		return nil, "", "", err
	}
	sharedSecret, err := curve.ECDH(localKey, peerKey)
	if err != nil {
		return nil, "", "", err
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, "", "", err
	}
	prk := hkdf(authBytes, sharedSecret, []byte("Content-Encoding: auth\x00"), 32)
	cekInfo := createInfo("aesgcm", peerKeyBytes, localKey.PublicKey().Bytes())
	cek := hkdf(salt, prk, cekInfo, 16)
	nonceInfo := createInfo("nonce", peerKeyBytes, localKey.PublicKey().Bytes())
	nonce := hkdf(salt, prk, nonceInfo, 12)

	paddingLen := uint16(16 - len(params.Payload)%16)
	payload := make([]byte, 2+paddingLen, 2+int(paddingLen)+len(params.Payload))
	binary.BigEndian.PutUint16(payload[:2], paddingLen)

	block, err := aes.NewCipher(cek)
	if err != nil {
		return nil, "", "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, "", "", err
	}
	payload = append(payload, params.Payload...)
	encPayload := gcm.Seal(nil, nonce, payload, nil)

	if sz := len(encPayload); sz >= 4000 {
		return nil, "", "", fmt.Errorf("message too large: %d", sz)
	}

	return encPayload, base64.RawURLEncoding.EncodeToString(salt), base64.RawURLEncoding.EncodeToString(localKey.PublicKey().Bytes()), nil
}

func hkdf(salt, ikm, info []byte, length int) []byte {
	if length < 0 || length > 32 {
		panic("cannot return keys longer than 32 bytes")
	}
	// Equivalent to:
	//  r := hkdf.New(sha256.New, ikm, salt, info)
	//  buf := make([]byte, length)
	//  io.ReadFull(r, buf)
	//  return buf
	keyHMAC := hmac.New(sha256.New, salt)
	keyHMAC.Write(ikm)
	key := keyHMAC.Sum(nil)
	infoHMAC := hmac.New(sha256.New, key)
	infoHMAC.Write(info)
	infoHMAC.Write([]byte{0x1})
	out := infoHMAC.Sum(nil)
	return out[:length]
}

func createInfo(typ string, peerPubKey, localPubKey []byte) []byte {
	var buf bytes.Buffer
	buf.Write([]byte("Content-Encoding: " + typ))
	buf.Write([]byte{0x0})
	buf.Write([]byte("P-256"))
	buf.Write([]byte{0x0})
	binary.Write(&buf, binary.BigEndian, uint16(len(peerPubKey)))
	buf.Write(peerPubKey)
	binary.Write(&buf, binary.BigEndian, uint16(len(localPubKey)))
	buf.Write(localPubKey)
	return buf.Bytes()
}
