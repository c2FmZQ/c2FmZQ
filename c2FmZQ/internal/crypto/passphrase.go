package crypto

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"

	"golang.org/x/term"
)

// Passphrase retrieves a passphrase. If cmd is set, the passphrase is the
// output of the command. Or, if file is set, the passphrase is the content
// of the file. Otherwise, the passphrase is read from the terminal.
func Passphrase(cmd, file string) ([]byte, error) {
	if cmd != "" {
		return exec.Command("/bin/sh", "-c", cmd).Output()
	}
	if file != "" {
		return os.ReadFile(file)
	}
	fmt.Print("Enter database passphrase: ")
	p, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	return bytes.TrimSpace(p), err
}
