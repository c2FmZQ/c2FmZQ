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
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"c2FmZQ/internal/client"
)

func TestList(t *testing.T) {
	c, err := newClient(t.TempDir())
	if err != nil {
		t.Fatalf("newClient: %v", err)
	}
	testdir := t.TempDir()
	if err := makeImages(testdir, 1, 2); err != nil {
		t.Fatalf("makeImages: %v", err)
	}
	if err := os.Rename(filepath.Join(testdir, "image002.jpg"), filepath.Join(testdir, ".image002.jpg")); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	t.Log("Import *")
	if n, err := c.ImportFiles([]string{filepath.Join(testdir, "*")}, "gallery", true); err != nil {
		t.Errorf("c.ImportFiles: %v", err)
	} else if want, got := 2, n; want != got {
		t.Errorf("Unexpected ImportFiles result. Want %d, got %d", want, got)
	}

	var buf bytes.Buffer
	c.SetWriter(&buf)

	testcases := []struct {
		name     string
		patterns []string
		opt      client.GlobOptions
		expected string
	}{
		{
			"ls",
			[]string{""}, client.GlobOptions{},
			"gallery/\n",
		},
		{
			"ls -l",
			[]string{""}, client.GlobOptions{Long: true},
			"gallery/ 2 files\n",
		},
		{
			"ls *",
			[]string{"*"}, client.GlobOptions{},
			"gallery:\nimage001.jpg\n",
		},
		{
			"ls -d *",
			[]string{"*"}, client.GlobOptions{Directory: true},
			"gallery/\n",
		},
		{
			"ls -ld *",
			[]string{"*"}, client.GlobOptions{Long: true, Directory: true},
			"gallery/ 2 files\n",
		},
		{
			"ls -a",
			[]string{""}, client.GlobOptions{MatchDot: true},
			".trash/\ngallery/\n",
		},
		{
			"ls -a *",
			[]string{"*"}, client.GlobOptions{MatchDot: true},
			".trash:\n\ngallery:\n.image002.jpg\nimage001.jpg\n",
		},
		{
			"ls -l gallery",
			[]string{"gallery"}, client.GlobOptions{Long: true},
			"gallery:\nimage001.jpg  789 XXXX-XX-XX XX:XX:XX photo Local\n",
		},
		{
			"ls */*",
			[]string{"*/*"}, client.GlobOptions{},
			"gallery/image001.jpg\n",
		},
		{
			"ls -l */*",
			[]string{"*/*"}, client.GlobOptions{Long: true},
			"gallery/image001.jpg  789 XXXX-XX-XX XX:XX:XX photo Local\n",
		},
		{
			"ls -lR",
			[]string{""}, client.GlobOptions{Long: true, Recursive: true},
			"gallery/                 2 files\n" +
				"gallery/.image002.jpg  789 XXXX-XX-XX XX:XX:XX photo Local\n" +
				"gallery/image001.jpg   789 XXXX-XX-XX XX:XX:XX photo Local\n",
		},
		{
			"ls -alR",
			[]string{""}, client.GlobOptions{MatchDot: true, Long: true, Recursive: true},
			".trash/                  0 files\n" +
				"gallery/                 2 files\n" +
				"gallery/.image002.jpg  789 XXXX-XX-XX XX:XX:XX photo Local\n" +
				"gallery/image001.jpg   789 XXXX-XX-XX XX:XX:XX photo Local\n",
		},
	}
	dateRE := regexp.MustCompile(`....-..-.. ..:..:..`)
	for _, tc := range testcases {
		buf.Reset()
		if err := c.ListFiles(tc.patterns, tc.opt); err != nil {
			t.Errorf("c.ListFiles: %v", err)
		}
		if want, got := tc.expected, dateRE.ReplaceAllString(buf.String(), "XXXX-XX-XX XX:XX:XX"); want != got {
			t.Errorf("[%s] Unexpected output. Want %q, got %q", tc.name, want, got)
		}
	}
}
