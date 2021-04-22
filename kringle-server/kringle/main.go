package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
				Value:       2,
				DefaultText: "2 (info)",
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
				Name:      "create-account",
				Usage:     "Create an account.",
				ArgsUsage: "<email>",
				Action:    createAccount,
			},
			&cli.Command{
				Name:      "login",
				Usage:     "Login to an account.",
				ArgsUsage: "<email>",
				Action:    login,
			},
			&cli.Command{
				Name:      "logout",
				Usage:     "Logout.",
				ArgsUsage: " ",
				Action:    logout,
			},
			&cli.Command{
				Name:      "updates",
				Aliases:   []string{"up", "update"},
				Usage:     "Pull metadata updates.",
				ArgsUsage: " ",
				Action:    updates,
			},
			&cli.Command{
				Name:      "sync",
				Aliases:   []string{"pull", "download"},
				Usage:     "Download all the files that aren't already downloaded.",
				ArgsUsage: `["glob"] ... (default "*/*")`,
				Action:    syncFiles,
			},
			&cli.Command{
				Name:      "free",
				Usage:     "Remove all the files that are backed up.",
				ArgsUsage: `["glob"] ... (default "*/*")`,
				Action:    freeFiles,
			},
			&cli.Command{
				Name:      "hide",
				Usage:     "Hide albums.",
				ArgsUsage: `["glob"] ...`,
				Action:    hideAlbums,
			},
			&cli.Command{
				Name:      "unhide",
				Usage:     "Unhide albums.",
				ArgsUsage: "[name] ...",
				Action:    unhideAlbums,
			},
			&cli.Command{
				Name:      "list",
				Aliases:   []string{"ls"},
				Usage:     "List the files in a file set.",
				ArgsUsage: `["glob"] ... (default "*")`,
				Action:    listFiles,
			},
			&cli.Command{
				Name:      "export",
				Usage:     "Decrypt and export files.",
				ArgsUsage: `"<glob>" ... <output directory>`,
				Action:    exportFiles,
			},
			&cli.Command{
				Name:      "import",
				Usage:     "Encrypts and import files.",
				ArgsUsage: `"<glob>" ... <album>`,
				Action:    importFiles,
			},
		},
	}
	sort.Sort(cli.CommandsByName(app.Commands))

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

func logout(ctx *cli.Context) error {
	c, err := initClient(ctx)
	if err != nil {
		return err
	}
	return c.Logout()
}

func updates(ctx *cli.Context) error {
	c, err := initClient(ctx)
	if err != nil {
		return err
	}

	return c.GetUpdates()
}

func syncFiles(ctx *cli.Context) error {
	c, err := initClient(ctx)
	if err != nil {
		return err
	}
	patterns := []string{"*/*"}
	if ctx.Args().Len() > 0 {
		patterns = ctx.Args().Slice()
	}
	if err := c.GetUpdates(); err != nil {
		return err
	}
	return c.Sync(patterns)
}

func freeFiles(ctx *cli.Context) error {
	c, err := initClient(ctx)
	if err != nil {
		return err
	}
	patterns := []string{"*/*"}
	if ctx.Args().Len() > 0 {
		patterns = ctx.Args().Slice()
	}
	if err := c.GetUpdates(); err != nil {
		return err
	}
	return c.Free(patterns)
}

func hideAlbums(ctx *cli.Context) error {
	c, err := initClient(ctx)
	if err != nil {
		return err
	}
	patterns := []string{"*"}
	if ctx.Args().Len() > 0 {
		patterns = ctx.Args().Slice()
	}
	return c.Hide(patterns, true)
}

func unhideAlbums(ctx *cli.Context) error {
	c, err := initClient(ctx)
	if err != nil {
		return err
	}
	patterns := []string{"*"}
	if ctx.Args().Len() > 0 {
		patterns = ctx.Args().Slice()
	}
	return c.Hide(patterns, false)
}

func listFiles(ctx *cli.Context) error {
	c, err := initClient(ctx)
	if err != nil {
		return err
	}
	patterns := []string{"*"}
	if ctx.Args().Len() > 0 {
		patterns = ctx.Args().Slice()
	}
	return c.ListFiles(patterns)
}

func exportFiles(ctx *cli.Context) error {
	c, err := initClient(ctx)
	if err != nil {
		return err
	}
	args := ctx.Args().Slice()
	if len(args) < 2 {
		return errors.New("missing argument")
	}
	patterns := args[:len(args)-1]
	dir := args[len(args)-1]
	return c.ExportFiles(patterns, dir)
}

func importFiles(ctx *cli.Context) error {
	c, err := initClient(ctx)
	if err != nil {
		return err
	}
	args := ctx.Args().Slice()
	if len(args) < 2 {
		return errors.New("missing argument")
	}
	patterns := args[:len(args)-1]
	dir := args[len(args)-1]
	return c.ImportFiles(patterns, dir)
}
