// The c2FmZQ-server binary is an API server that can securely encrypt, store,
// and share files, including but not limited to pictures and videos.
//
// It is compatible with the Stingle Photos app (https://github.com/stingle/stingle-photos-android)
// published by stingle.org.
package main

import (
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/urfave/cli/v2" // cli
	"golang.org/x/sys/unix"
	"golang.org/x/term"

	"c2FmZQ/internal/database"
	"c2FmZQ/internal/log"
	"c2FmZQ/internal/server"
)

var (
	flagDatabase         string
	flagAddress          string
	flagBaseURL          string
	flagTLSCert          string
	flagTLSKey           string
	flagAllowNewAccounts bool
	flagLogLevel         int
	flagEncryptMetadata  bool
	flagPassphraseFile   string
	flagHTDigestFile     string
)

func main() {
	rand.Seed(int64(time.Now().Nanosecond()))

	app := &cli.App{
		Name:      "c2FmZQ-server",
		Usage:     "Runs the c2FmZQ server",
		HideHelp:  true,
		ArgsUsage: " ",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "database",
				Aliases:     []string{"db"},
				Value:       "",
				Usage:       "Use the database in `DIR`",
				Required:    true,
				EnvVars:     []string{"C2FMZQ_DATABASE"},
				Destination: &flagDatabase,
			},
			&cli.StringFlag{
				Name:        "address",
				Aliases:     []string{"addr"},
				Value:       "127.0.0.1:8080",
				Usage:       "The local address to use.",
				Destination: &flagAddress,
			},
			&cli.StringFlag{
				Name:        "base-url",
				Value:       "",
				Usage:       "The base URL of the generated download links. If empty, the links will generated using the Host headers of the incoming requests, i.e. https://HOST/.",
				Destination: &flagBaseURL,
			},
			&cli.StringFlag{
				Name:        "tlscert",
				Value:       "",
				Usage:       "The name of the `FILE` containing the TLS cert to use. If neither -tlscert nor -tlskey is set, the server will not use TLS.",
				TakesFile:   true,
				Destination: &flagTLSCert,
			},
			&cli.StringFlag{
				Name:        "tlskey",
				Value:       "",
				Usage:       "The name of the `FILE` containing the TLS private key to use.",
				Destination: &flagTLSKey,
			},
			&cli.BoolFlag{
				Name:        "allow-new-accounts",
				Value:       true,
				Usage:       "Allow new account registrations.",
				Destination: &flagAllowNewAccounts,
			},
			&cli.IntFlag{
				Name:        "verbose",
				Aliases:     []string{"v"},
				Value:       3,
				DefaultText: "2 (info)",
				Usage:       "The level of logging verbosity: 1:Error 2:Info 3:Debug",
				Destination: &flagLogLevel,
			},
			&cli.BoolFlag{
				Name:        "encrypt-metadata",
				Value:       true,
				Usage:       "Encrypt the server metadata (strongly recommended).",
				Destination: &flagEncryptMetadata,
			},
			&cli.StringFlag{
				Name:        "passphrase-file",
				Value:       "",
				Usage:       "Read the database passphrase from `FILE`.",
				EnvVars:     []string{"C2FMZQ_PASSPHRASE_FILE"},
				Destination: &flagPassphraseFile,
			},
			&cli.StringFlag{
				Name:        "htdigest-file",
				Value:       "",
				Usage:       "The name of the htdigest `FILE` to use for basic auth for some endpoints, e.g. /metrics",
				EnvVars:     []string{"C2FMZQ_HTDIGEST_FILE"},
				Destination: &flagHTDigestFile,
			},
		},
		Action: startServer,
	}
	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func startServer(c *cli.Context) error {
	if c.Args().Len() > 0 {
		cli.ShowSubcommandHelp(c)
		return nil
	}
	log.Level = flagLogLevel
	if (flagTLSCert == "") != (flagTLSKey == "") {
		log.Fatal("--tlscert and --tlskey must either both be set or unset.")
	}
	var pp string
	if flagEncryptMetadata {
		var err error
		if pp, err = passphrase(c); err != nil {
			return err
		}
	}
	if pp == "" {
		log.Info("WARNING: Metadata encryption is DISABLED")
	}
	db := database.New(flagDatabase, pp)

	s := server.New(db, flagAddress, flagHTDigestFile)
	s.AllowCreateAccount = flagAllowNewAccounts
	s.BaseURL = flagBaseURL

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

	if flagTLSCert == "" {
		log.Info("Starting server WITHOUT TLS")
		if err := s.Run(); err != http.ErrServerClosed {
			log.Fatalf("s.Run: %v", err)
		}
	} else {
		log.Info("Starting server with TLS")
		if err := s.RunWithTLS(flagTLSCert, flagTLSKey); err != http.ErrServerClosed {
			log.Fatalf("s.RunWithTLS: %v", err)
		}
	}
	<-done
	log.Info("Server exited cleanly.")
	return nil
}

func passphrase(c *cli.Context) (string, error) {
	if f := flagPassphraseFile; f != "" {
		p, err := os.ReadFile(f)
		if err != nil {
			return "", cli.Exit(err, 1)
		}
		return string(p), nil
	}
	fmt.Print("Enter database passphrase: ")
	passphrase, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		return "", cli.Exit(err, 1)
	}
	return strings.TrimSpace(string(passphrase)), nil
}
