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

package crypto

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"

	"golang.org/x/term"
)

// Passphrase retrieves a passphrase. If cmd is set, the passphrase is the
// output of the command. Or, if file is set, the passphrase is the content
// of the file. Otherwise, the passphrase is read from the terminal.
func Passphrase(cmd, file, passphrase string) ([]byte, error) {
	if cmd != "" {
		c := exec.Command("/bin/sh", "-c", cmd)
		c.Stderr = os.Stderr
		return c.Output()
	}
	if file != "" {
		return os.ReadFile(file)
	}
	if passphrase != "" {
		return []byte(passphrase), nil
	}
	fmt.Print("Enter database passphrase: ")
	p, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	return bytes.TrimSpace(p), err
}

// NewPassphrase is like Passphrase but will prompt for a 'new' passphrase twice
// if it is coming from a terminal.
func NewPassphrase(cmd, file, passphrase string) ([]byte, error) {
	if cmd != "" {
		c := exec.Command("/bin/sh", "-c", cmd)
		c.Stderr = os.Stderr
		return c.Output()
	}
	if file != "" {
		return os.ReadFile(file)
	}
	if passphrase != "" {
		return []byte(passphrase), nil
	}
	fmt.Print("Enter NEW database passphrase: ")
	p, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	fmt.Print("Re-enter NEW database passphrase: ")
	p2, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if bytes.Compare(p, p2) != 0 {
		return nil, errors.New("new passphrase doesn't match")
	}
	return bytes.TrimSpace(p), err
}
