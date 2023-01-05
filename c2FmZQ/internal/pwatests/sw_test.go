//
// Copyright 2021-2023 TTBT Enterprises LLC
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
	"encoding/json"
	"net/url"
	"testing"
)

func TestServiceWorkerTests(t *testing.T) {
	wd, stop := startServer(t)
	defer stop()

	u, err := wd.CurrentURL()
	if err != nil {
		t.Fatalf("CurrentURL: %v", err)
	}
	url, err := url.Parse(u)
	if err != nil {
		t.Fatalf("ParseURL: %v", err)
	}
	url.Path = "sw-tests.html"
	url.RawQuery = ""

	if err := wd.Get(url.String()); err != nil {
		t.Fatalf("sw-tests.html: %v", err)
	}

	js, err := wd.waitFor("#results").Text()
	if err != nil {
		t.Fatalf("#results: %v", err)
	}
	var results []struct {
		Test   string `json:"test"`
		Result string `json:"result"`
		Error  string `json:"err"`
	}
	if err := json.Unmarshal([]byte(js), &results); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	for _, r := range results {
		if r.Result == "PASS" {
			t.Logf("TEST %s PASS", r.Test)
		} else {
			t.Errorf("TEST %s %s %s", r.Test, r.Result, r.Error)
		}
	}
}
