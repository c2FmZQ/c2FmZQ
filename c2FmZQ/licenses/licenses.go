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
