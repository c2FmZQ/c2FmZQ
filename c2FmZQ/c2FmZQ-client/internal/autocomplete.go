package internal

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/urfave/cli/v2"
	"golang.org/x/term"

	"c2FmZQ/internal/client"
)

type autoCompleteOption struct {
	name    string
	display string
}

func (a *App) commandOptions(cmds []*cli.Command, currentWord string) []autoCompleteOption {
	var options []autoCompleteOption
	for _, cmd := range []string{"exit", "help"} {
		if strings.HasPrefix(cmd, currentWord) {
			options = append(options, autoCompleteOption{name: cmd, display: cmd})
		}
	}
	for _, cmd := range cmds {
		if strings.HasPrefix(cmd.Name, currentWord) {
			options = append(options, autoCompleteOption{name: cmd.Name, display: cmd.Name})
		}
		for _, alias := range cmd.Aliases {
			if strings.HasPrefix(alias, currentWord) {
				options = append(options, autoCompleteOption{name: alias, display: alias})
			}
		}
	}
	sort.Slice(options, func(i, j int) bool {
		return options[i].display < options[j].display
	})
	return options
}

func (a *App) fileOptions(currentWord string) []autoCompleteOption {
	li, err := a.client.GlobFiles([]string{currentWord + "*"}, client.GlobOptions{Quiet: true})
	if err != nil {
		return nil
	}
	if len(li) == 0 {
		return nil
	}
	var options []autoCompleteOption
	for _, item := range li {
		n := item.Filename
		_, d := filepath.Split(n)
		if item.IsDir {
			n += "/"
			d += "/"
		}
		options = append(options, autoCompleteOption{name: n, display: d})
	}
	return options
}

func (a *App) commonPrefix(options []autoCompleteOption) int {
	p := 0
	for {
		same := true
		for _, n := range options {
			if p >= len(n.name) || p >= len(options[0].name) || n.name[p] != options[0].name[p] {
				same = false
				break
			}
		}
		if !same {
			break
		}
		p++
	}
	return p
}

func (a *App) displayOptions(t *term.Terminal, width int, options []autoCompleteOption) {
	size := 0
	for _, n := range options {
		if len(n.display) > size {
			size = len(n.display)
		}
	}
	fmt.Fprintln(t, "\nOptions:")
	var out []string
	line := "  "
	for _, n := range options {
		line = fmt.Sprintf("%s%*s ", line, -size, n.display)
		if len(line) >= width-size {
			out = append(out, string(t.Escape.Blue)+line+string(t.Escape.Reset))
			line = "  "
		}
	}
	if len(line) > 2 {
		out = append(out, string(t.Escape.Blue)+line+string(t.Escape.Reset))
	}
	fmt.Fprintln(t, strings.Join(out, "\n"))
}
