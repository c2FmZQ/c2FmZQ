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
