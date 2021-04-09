// The kringle-server binary is an API server that's compatible with the
// Stingle Photos app (https://github.com/stingle/stingle-photos-android)
// published by stingle.org.
//
// For the app to connect to this server, it has to the recompiled with
// api_server_url set to point to this server.
package main

import (
	"flag"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"golang.org/x/sys/unix"
	"golang.org/x/term"
	"kringle-server/database"
	"kringle-server/log"
	"kringle-server/server"
)

var (
	allowNewAccounts = flag.Bool("allow-new-accounts", true, "Whether new account registration is allowed.")
	dbFlag           = flag.String("db", "", "The directory name of the database.")
	address          = flag.String("address", "127.0.0.1:8080", "The local address to use.")
	baseURL          = flag.String("base-url", "", "The base URL of the generated download links. If empty, the links will generated using the Host headers of the incoming requests, i.e. https://HOST/.")
	certFile         = flag.String("tlscert", "", "The name of the file containing the TLS cert to use. If neither -cert or -key is set, the server will not use TLS.")
	keyFile          = flag.String("tlskey", "", "The name of the file containing the TLS private key to use.")
	logLevel         = flag.Int("v", 2, "The level of logging verbosity: 1:Error 2:Info 3:Debug")

	encryptMetatada = flag.Bool("encrypt-metadata", true, "Whether to encrypt metadata.")
	passphraseFile  = flag.String("passphrase-file", "", "The name of the file containing the passphrase that protects the server's metadata. If left empty, the server will prompt for a passphrase when it starts.")
	htdigestFile    = flag.String("htdigest-file", "", "The name of the htdigest file to use for basic auth for some endpoints, e.g. /metrics")
)

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: %s [flags]\n\nFlags:\n", os.Args[0])
	flag.PrintDefaults()
	os.Exit(64)
}

func main() {
	rand.Seed(int64(time.Now().Nanosecond()))
	flag.Usage = usage
	flag.Parse()
	log.Level = *logLevel

	if *dbFlag == "" {
		log.Error("--db must be set")
		usage()
	}
	if *address == "" {
		log.Error("--address must be set")
		usage()
	}
	if (*certFile == "") != (*keyFile == "") {
		log.Error("--cert and --key must either both be set or unset.")
		usage()
	}
	var pp string
	if *encryptMetatada {
		pp = passphrase()
	}
	if pp == "" {
		log.Info("WARNING: Metadata encryption is DISABLED")
	}
	db := database.New(*dbFlag, pp)

	s := server.New(db, *address, *htdigestFile)
	s.AllowCreateAccount = *allowNewAccounts
	s.BaseURL = *baseURL

	done := make(chan struct{})
	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, unix.SIGINT)
		signal.Notify(ch, unix.SIGTERM)
		sig := <-ch
		log.Infof("Received signal %d (%s)", sig, sig)
		if err := s.Shutdown(); err != nil {
			log.Errorf("s.Shutdown: %v", err)
		}
		close(done)
	}()

	if *certFile == "" {
		log.Info("Starting server WITHOUT TLS")
		if err := s.Run(); err != http.ErrServerClosed {
			log.Fatalf("s.Run: %v", err)
		}
	} else {
		log.Info("Starting server with TLS")
		if err := s.RunWithTLS(*certFile, *keyFile); err != http.ErrServerClosed {
			log.Fatalf("s.RunWithTLS: %v", err)
		}
	}
	<-done
	log.Info("Server exited cleanly.")
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
