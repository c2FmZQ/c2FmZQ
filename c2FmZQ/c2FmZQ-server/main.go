// The c2FmZQ-server binary is an API server that can securely encrypt, store,
// and share files, including but not limited to pictures and videos.
//
// It is compatible with the Stingle Photos app (https://github.com/stingle/stingle-photos-android)
// published by stingle.org.
package main

import (
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/urfave/cli/v2" // cli

	"c2FmZQ/internal/crypto"
	"c2FmZQ/internal/database"
	"c2FmZQ/internal/log"
	"c2FmZQ/internal/server"
	"c2FmZQ/licenses"
)

var (
	flagDatabase              string
	flagAddress               string
	flagBaseURL               string
	flagRedirect404           string
	flagPathPrefix            string
	flagTLSCert               string
	flagTLSKey                string
	flagAllowNewAccounts      bool
	flagLogLevel              int
	flagEncryptMetadata       bool
	flagPassphraseFile        string
	flagPassphraseCmd         string
	flagHTDigestFile          string
	flagAutocertDomain        string
	flagAutocertAddr          string
	flagMaxConcurrentRequests int
)

func main() {
	rand.Seed(int64(time.Now().Nanosecond()))
	var defaultDB string
	if home, err := os.UserHomeDir(); err == nil {
		defaultDB = filepath.Join(home, "c2FmZQ-server", "data")
	}
	app := &cli.App{
		Name:      "c2FmZQ-server",
		Usage:     "Run the c2FmZQ server",
		HideHelp:  true,
		ArgsUsage: " ",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "database",
				Aliases:     []string{"db"},
				Value:       defaultDB,
				Usage:       "Use the database in `DIR`",
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
				Name:        "path-prefix",
				Value:       "",
				Usage:       "The API endpoints are <path-prefix>/v2/...",
				Destination: &flagPathPrefix,
			},
			&cli.StringFlag{
				Name:        "base-url",
				Value:       "",
				Usage:       "The base URL of the generated download links. If empty, the links will generated using the Host headers of the incoming requests, i.e. https://HOST/.",
				Destination: &flagBaseURL,
			},
			&cli.StringFlag{
				Name:        "redirect-404",
				Value:       "",
				Usage:       "Requests to unknown endpoints are redirected to this URL.",
				Destination: &flagRedirect404,
			},
			&cli.StringFlag{
				Name:        "tlscert",
				Value:       "",
				Usage:       "The name of the `FILE` containing the TLS cert to use.",
				TakesFile:   true,
				Destination: &flagTLSCert,
			},
			&cli.StringFlag{
				Name:        "tlskey",
				Value:       "",
				Usage:       "The name of the `FILE` containing the TLS private key to use.",
				Destination: &flagTLSKey,
			},
			&cli.StringFlag{
				Name:        "autocert-domain",
				Value:       "",
				Usage:       "Use autocert (letsencrypt.org) to get TLS credentials for this `domain`. The special value 'any' means accept any domain. The credentials are saved in the database.",
				EnvVars:     []string{"C2FMZQ_DOMAIN"},
				Destination: &flagAutocertDomain,
			},
			&cli.StringFlag{
				Name:        "autocert-address",
				Value:       ":http",
				Usage:       "The autocert http server will listen on this address. It must be reachable externally on port 80.",
				Destination: &flagAutocertAddr,
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
				Value:       2,
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
				Name:        "passphrase-command",
				Value:       "",
				Usage:       "Read the database passphrase from the standard output of `COMMAND`.",
				EnvVars:     []string{"C2FMZQ_PASSPHRASE_CMD"},
				Destination: &flagPassphraseCmd,
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
			&cli.IntFlag{
				Name:        "max-concurrent-requests",
				Value:       10,
				Usage:       "The maximum number of concurrent requests.",
				Destination: &flagMaxConcurrentRequests,
			},
			&cli.BoolFlag{
				Name:  "licenses",
				Usage: "Show the software licenses.",
			},
		},
		Action: startServer,
	}
	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func startServer(c *cli.Context) error {
	if c.Bool("licenses") {
		licenses.Show()
		return nil
	}
	if c.Args().Len() > 0 {
		cli.ShowSubcommandHelp(c)
		return nil
	}
	log.Level = flagLogLevel
	if (flagTLSCert == "") != (flagTLSKey == "") {
		log.Fatal("--tlscert and --tlskey must either both be set or unset.")
	}
	var pp []byte
	if flagEncryptMetadata {
		var err error
		if pp, err = crypto.Passphrase(flagPassphraseCmd, flagPassphraseFile); err != nil {
			return err
		}
	}
	if pp == nil {
		log.Info("WARNING: Metadata encryption is DISABLED")
	}
	db := database.New(flagDatabase, pp)

	s := server.New(db, flagAddress, flagHTDigestFile, flagPathPrefix)
	s.AllowCreateAccount = flagAllowNewAccounts
	s.BaseURL = flagBaseURL
	s.Redirect404 = flagRedirect404
	s.MaxConcurrentRequests = flagMaxConcurrentRequests

	done := make(chan struct{})
	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGINT)
		signal.Notify(ch, syscall.SIGTERM)
		sig := <-ch
		log.Infof("Received signal %d (%s)", sig, sig)
		if err := s.Shutdown(); err != nil {
			log.Errorf("s.Shutdown: %v", err)
		}
		close(done)
	}()

	if flagTLSCert == "" && flagAutocertDomain == "" {
		log.Info("Starting server WITHOUT TLS")
		if err := s.Run(); err != http.ErrServerClosed {
			log.Fatalf("s.Run: %v", err)
		}
	} else if flagAutocertDomain == "" {
		log.Info("Starting server with TLS")
		if err := s.RunWithTLS(flagTLSCert, flagTLSKey); err != http.ErrServerClosed {
			log.Fatalf("s.RunWithTLS: %v", err)
		}
	} else {
		log.Info("Starting server with Autocert")
		if err := s.RunWithAutocert(flagAutocertDomain, flagAutocertAddr); err != http.ErrServerClosed {
			log.Fatalf("s.RunWithAutocert: %v", err)
		}
	}
	<-done
	log.Info("Server exited cleanly.")
	return nil
}
