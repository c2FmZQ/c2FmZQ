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

// Package licenses embeds the software licenses of all the libraries used
// in this project.
package licenses

import (
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

//go:embed *
var content embed.FS

func Show() {
	files := make(map[string]string)
	fs.WalkDir(content, ".", func(path string, d fs.DirEntry, err error) error {
		if !d.IsDir() && strings.Contains(path, "/") {
			p := path[strings.Index(path, "/")+1:]
			b, _ := content.ReadFile(path)
			files[p] = string(b)
		}
		return nil
	})
	var sorted []string
	for path := range files {
		sorted = append(sorted, path)
	}
	sort.Strings(sorted)
	for _, p := range sorted {
		fmt.Println(strings.Repeat("~", len(p)))
		fmt.Println(p)
		fmt.Println(strings.Repeat("~", len(p)))
		fmt.Println()
		fmt.Println(files[p])
	}
}
