package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
	"kringle-server/database"
	"kringle-server/log"
)

var (
	dbFlag   = flag.String("db", "", "The directory name of the database.")
	logLevel = flag.Int("v", 3, "The level of logging verbosity: 1:Error 2:Info 3:Debug")

	passphraseFile = flag.String("passphrase_file", "", "The name of the file containing the passphrase that protects the server's metadata. If left empty, the server will prompt for a passphrase when it starts.")
)

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: %s [flags]\n\nFlags:\n", os.Args[0])
	flag.PrintDefaults()
	os.Exit(64)
}

func main() {
	flag.Usage = usage
	flag.Parse()
	log.Level = *logLevel

	if *dbFlag == "" {
		log.Error("--db must be set")
		usage()
	}
	db := database.New(*dbFlag, passphrase())
	for _, f := range flag.Args() {
		if err := db.DumpFile(f); err != nil {
			log.Errorf("Error: %v", err)
		}
	}
}

func passphrase() string {
	if *passphraseFile != "" {
		p, err := os.ReadFile(*passphraseFile)
		if err != nil {
			log.Fatalf("passphrase: %v", err)
		}
		return string(p)
	}
	fmt.Print("Enter passphrase: ")
	passphrase, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		log.Fatalf("passphrase: %v", err)
	}
	return strings.TrimSpace(string(passphrase))
}
