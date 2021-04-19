package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/urfave/cli/v2" // cli
	"golang.org/x/term"

	"kringle-server/client"
	"kringle-server/crypto"
	"kringle-server/log"
	"kringle-server/secure"
)

var (
	flagDataDir        string
	flagLogLevel       int
	flagPassphraseFile string
	flagAPIServer      string
)

func main() {
	app := &cli.App{
		Name:     "kringle",
		Usage:    "kringle client.",
		HideHelp: true,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "data-dir",
				Aliases:     []string{"d"},
				Value:       "",
				Usage:       "Save the data in `DIR`",
				Required:    true,
				EnvVars:     []string{"KRINGLE_DATADIR"},
				TakesFile:   true,
				Destination: &flagDataDir,
			},
			&cli.IntFlag{
				Name:        "verbose",
				Aliases:     []string{"v"},
				Value:       3,
				DefaultText: "3 (debug)",
				Usage:       "The level of logging verbosity: 1:Error 2:Info 3:Debug",
				Destination: &flagLogLevel,
			},
			&cli.StringFlag{
				Name:        "passphrase-file",
				Value:       "",
				Usage:       "Read the database passphrase from `FILE`.",
				EnvVars:     []string{"KRINGLE_PASSPHRASE_FILE"},
				Destination: &flagPassphraseFile,
			},
			&cli.StringFlag{
				Name:        "server",
				Value:       "",
				Usage:       "The API server base URL.",
				Destination: &flagAPIServer,
			},
		},
		Commands: []*cli.Command{
			&cli.Command{
				Name:   "create-account",
				Usage:  "Create an account",
				Action: createAccount,
			},
			&cli.Command{
				Name:   "login",
				Usage:  "Login to an account",
				Action: login,
			},
			&cli.Command{
				Name:   "updates",
				Usage:  "Request a list of updates",
				Action: updates,
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func initClient(ctx *cli.Context) (*client.Client, error) {
	log.Level = flagLogLevel
	var pp string
	var err error
	if pp, err = passphrase(ctx); err != nil {
		return nil, err
	}

	mkFile := filepath.Join(flagDataDir, "master.key")
	masterKey, err := crypto.ReadMasterKey(pp, mkFile)
	if errors.Is(err, os.ErrNotExist) {
		if masterKey, err = crypto.CreateMasterKey(); err != nil {
			log.Fatal("Failed to create master key")
		}
		err = masterKey.Save(pp, mkFile)
	}
	if err != nil {
		log.Fatalf("Failed to decrypt master key: %v", err)
	}
	storage := secure.NewStorage(flagDataDir, &masterKey.EncryptionKey)

	c, err := client.Load(storage)
	if err != nil {
		c, err = client.Create(storage)
	}
	if flagAPIServer != "" {
		c.ServerBaseURL = flagAPIServer
	}
	return c, err

}

func passphrase(ctx *cli.Context) (string, error) {
	if f := flagPassphraseFile; f != "" {
		p, err := os.ReadFile(f)
		if err != nil {
			return "", cli.Exit(err, 1)
		}
		return string(p), nil
	}
	fmt.Print("Enter database passphrase: ")
	passphrase, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return "", cli.Exit(err, 1)
	}
	return strings.TrimSpace(string(passphrase)), nil
}

func createAccount(ctx *cli.Context) error {
	c, err := initClient(ctx)
	if err != nil {
		return err
	}
	if ctx.Args().Len() != 1 {
		return errors.New("must specify email address")
	}

	email := ctx.Args().Get(0)
	fmt.Print("Enter password: ")
	password, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return err
	}
	return c.CreateAccount(email, string(password))
}

func login(ctx *cli.Context) error {
	c, err := initClient(ctx)
	if err != nil {
		return err
	}
	if ctx.Args().Len() != 1 {
		return errors.New("must specify email address")
	}

	email := ctx.Args().Get(0)
	fmt.Print("Enter password: ")
	password, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return err
	}
	return c.Login(email, string(password))
}

func updates(ctx *cli.Context) error {
	c, err := initClient(ctx)
	if err != nil {
		return err
	}

	return c.GetUpdates()
}
