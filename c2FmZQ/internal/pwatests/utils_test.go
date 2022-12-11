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

//go:build selenium
// +build selenium

package pwa_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tebeka/selenium"
	"github.com/tebeka/selenium/chrome"
	slog "github.com/tebeka/selenium/log"

	"c2FmZQ/internal/database"
	"c2FmZQ/internal/log"
	"c2FmZQ/internal/server"
)

func startServer(t *testing.T) (*wrapper, func()) {
	testdir := t.TempDir()
	log.Record = t.Log
	log.Level = 3
	db := database.New(filepath.Join(testdir, "data"), []byte("secret"))
	s := server.New(db, "", "", "")
	s.AllowCreateAccount = true
	s.AutoApproveNewAccounts = true
	s.EnableWebApp = true
	l, err := net.Listen("tcp", "devtest:0")
	if err != nil {
		t.Fatalf("net.Listen failed: %v", err)
	}
	go s.RunWithListener(l)
	url := fmt.Sprintf("http://devtest:%d/", l.Addr().(*net.TCPAddr).Port)
	t.Logf("Server running on %s", url)
	wd := newWebDriver(t, url)
	if err := wd.ResizeWindow("", 1000, 800); err != nil {
		t.Fatalf("wd.ResizeWindow: %v", err)
	}
	if err := wd.Get(url); err != nil {
		t.Fatalf("wd.Get: %v", err)
	}
	return wd, func() {
		s.Shutdown()
		db.Wipe()
		log.Record = nil
		wd.getLogs(slog.Browser)
		wd.Quit()
	}
}

func newWebDriver(t *testing.T, url string) *wrapper {
	caps := selenium.Capabilities{"browserName": "chrome"}
	caps.AddChrome(chrome.Capabilities{
		Path: "/usr/bin/google-chrome",
		Args: []string{
			"--no-sandbox",
			"--allow-insecure-localhost",
			"--unsafely-treat-insecure-origin-as-secure=" + url,
		},
	})
	caps.SetLogLevel(slog.Browser, slog.Info)
	prefix := "http://chrome:4444/wd/hub"
	wd, err := selenium.NewRemote(caps, prefix)
	if err != nil {
		t.Fatalf("selenium.NewRemote: %v", err)
	}
	return &wrapper{
		WebDriver: wd,
		t:         t,
		urlPrefix: prefix,
	}
}

type wrapper struct {
	selenium.WebDriver

	t               *testing.T
	urlPrefix       string
	authenticatorID string
}

func (w *wrapper) enableWebauthn() error {
	url := fmt.Sprintf("%s/session/%s/webauthn/authenticator", w.urlPrefix, w.SessionID())
	data, err := json.Marshal(map[string]interface{}{
		"protocol":            "ctap2",
		"transport":           "internal",
		"hasResidentKey":      true,
		"hasUserVerification": true,
		"isUserConsenting":    true,
		"isUserVerified":      true,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	var response struct {
		SessionID string `json:"sessionId"`
		Status    int    `json:"status"`
		Value     string `json:"value"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return err
	}
	w.authenticatorID = response.Value
	log.Infof("Authenticator ID: %s", w.authenticatorID)
	return nil
}

func (w *wrapper) disableWebauthn() error {
	url := fmt.Sprintf("%s/session/%s/webauthn/authenticator/%s", w.urlPrefix, w.SessionID(), w.authenticatorID)
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	log.Infof("disableWebauthn: %s", string(body))
	return nil
}

func (w *wrapper) getLogs(logType slog.Type) {
	messages, err := w.Log(logType)
	if err != nil {
		w.t.Logf("getLogs: %v", err)
		return
	}
	url, err := w.CurrentURL()
	if err != nil {
		w.t.Logf("CurrentURL: %v", err)
		return
	}
	for _, m := range messages {
		msg := strings.Replace(m.Message, url, "", 1)
		w.t.Logf("%s.%.4s: %s", logType, m.Level, msg)
	}
}

func (w *wrapper) css(s string) selenium.WebElement {
	e, err := w.FindElement(selenium.ByCSSSelector, s)
	if err != nil {
		w.t.Fatalf("%s: %v", s, err)
	}
	return e
}

func (w *wrapper) waitFor(sel string) selenium.WebElement {
	var e selenium.WebElement
	if err := w.Wait(func(wd selenium.WebDriver) (bool, error) {
		var err error
		if e, err = wd.FindElement(selenium.ByCSSSelector, sel); err != nil {
			return false, nil
		}
		return e.IsDisplayed()
	}); err != nil || e == nil {
		w.t.Fatalf("waitFor(%q): %v", sel, err)
	}
	return e
}

func (w *wrapper) waitGone(sel string) {
	if err := w.Wait(func(wd selenium.WebDriver) (bool, error) {
		if _, err := wd.FindElement(selenium.ByCSSSelector, sel); err == nil {
			return false, nil
		}
		return true, nil
	}); err != nil {
		w.t.Fatalf("waitGone(%q): %v", sel, err)
	}
}

func (w *wrapper) sendKeys(sel, keys string) {
	if err := w.waitFor(sel).SendKeys(keys); err != nil {
		w.t.Fatalf("%s: %v", sel, err)
	}
}

func (w *wrapper) click(sel string) {
	if err := w.waitFor(sel).Click(); err != nil {
		w.t.Fatalf("%s: %v", sel, err)
	}
}

func (w *wrapper) clear(sel string) {
	if err := w.waitFor(sel).Clear(); err != nil {
		w.t.Fatalf("%s: %v", sel, err)
	}
}

func (w *wrapper) waitPopupMessage(messages ...string) {
	want := make(map[string]bool)
	for _, m := range messages {
		want[m] = true
	}
	var elems []selenium.WebElement
	if err := w.Wait(func(wd selenium.WebDriver) (bool, error) {
		var err error
		if elems, err = wd.FindElements(selenium.ByCSSSelector, ".popup-message"); err != nil || len(elems) != len(want) {
			return false, nil
		}
		for _, e := range elems {
			if t, err := e.Text(); err != nil || !want[t] {
				return false, nil
			}
		}
		return true, nil
	}); err != nil || len(elems) == 0 {
		w.t.Fatalf("waitPopupMessage(%v): %v", messages, err)
	}
	w.waitGone(".popup-message")
}
