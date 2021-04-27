package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/urfave/cli/v2" // cli
	"golang.org/x/term"

	"kringle/database"
	"kringle/log"
	"kringle/stingle"
)

var (
	flagDatabase        string
	flagLogLevel        int
	flagEncryptMetadata bool
	flagPassphraseFile  string
)

func main() {
	app := &cli.App{
		Name:     "debug",
		Usage:    "Access debug information from the kringle database.",
		HideHelp: true,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "database",
				Aliases:     []string{"db"},
				Value:       "",
				Usage:       "Use the database in `DIR`",
				Required:    true,
				EnvVars:     []string{"KRINGLE_DATABASE"},
				TakesFile:   true,
				Destination: &flagDatabase,
			},
			&cli.IntFlag{
				Name:        "verbose",
				Aliases:     []string{"v"},
				Value:       3,
				DefaultText: "3 (debug)",
				Usage:       "The level of logging verbosity: 1:Error 2:Info 3:Debug",
				Destination: &flagLogLevel,
			},
			&cli.BoolFlag{
				Name:        "encrypt-metadata",
				Value:       true,
				Usage:       "Whether the metadata is encrypted.",
				Destination: &flagEncryptMetadata,
			},
			&cli.StringFlag{
				Name:        "passphrase-file",
				Value:       "",
				Usage:       "Read the database passphrase from `FILE`.",
				EnvVars:     []string{"KRINGLE_PASSPHRASE_FILE"},
				Destination: &flagPassphraseFile,
			},
		},
		Commands: []*cli.Command{
			&cli.Command{
				Name:   "users",
				Usage:  "Show the list of users.",
				Action: showUsers,
			},
			&cli.Command{
				Name:    "cat",
				Aliases: []string{"show", "dump"},
				Usage:   "Show the content of files.",
				Action:  catFile,
			},
			&cli.Command{
				Name:   "key",
				Usage:  "Decrypt a user's secret key.",
				Action: decrypt,
			},
			&cli.Command{
				Name:   "header",
				Usage:  "Decrypt a file header.",
				Action: decryptHeader,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "user",
						Usage:    "user's email address",
						Required: true,
					},
				},
			},
			&cli.Command{
				Name:   "file",
				Usage:  "Decrypt a file.",
				Action: decryptFile,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "user",
						Usage:    "user's email address",
						Required: true,
					},
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func initDB(c *cli.Context) (*database.Database, error) {
	log.Level = flagLogLevel
	var pp string
	if flagEncryptMetadata {
		var err error
		if pp, err = passphrase(c); err != nil {
			return nil, err
		}
	}
	return database.New(flagDatabase, pp), nil
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
	fmt.Println()
	if err != nil {
		return "", cli.Exit(err, 1)
	}
	return strings.TrimSpace(string(passphrase)), nil
}

func userSK(db *database.Database, email string) (*stingle.SecretKey, error) {
	user, err := db.User(email)
	if err != nil {
		return nil, err
	}
	fmt.Print("Enter user's password: ")
	password, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return nil, err
	}
	sk, err := stingle.DecodeSecretKeyBundle(password, user.KeyBundle)
	if err != nil {
		return nil, err
	}
	return &sk, nil
}

func showUsers(c *cli.Context) error {
	db, err := initDB(c)
	if err != nil {
		return err
	}
	db.DumpUsers()
	return nil
}

func catFile(c *cli.Context) error {
	db, err := initDB(c)
	if err != nil {
		return err
	}
	for _, f := range c.Args().Slice() {
		if err := db.DumpFile(f); err != nil {
			log.Errorf("%s: %v", f, err)
		}
	}
	return nil
}

func decrypt(c *cli.Context) error {
	db, err := initDB(c)
	if err != nil {
		return err
	}
	for _, u := range c.Args().Slice() {
		if _, err := userSK(db, u); err != nil {
			return err
		}
		log.Info("user's secret key successfully decrypted")
	}
	return nil
}

func decryptHeader(c *cli.Context) error {
	db, err := initDB(c)
	if err != nil {
		return err
	}
	sk, err := userSK(db, c.String("user"))
	if err != nil {
		log.Errorf("%s: %v", c.String("user"), err)
	}
	for _, h := range c.Args().Slice() {
		log.Infof("Decoding %s", h)
		hdrs, err := stingle.DecryptBase64Headers(h, *sk)
		if err != nil {
			return cli.Exit(err, 1)
		}
		log.Infof("File: %#v", hdrs[0])
		log.Infof("Thumb: %#v", hdrs[1])
	}
	return nil
}

func decryptFile(c *cli.Context) error {
	db, err := initDB(c)
	if err != nil {
		return err
	}
	sk, err := userSK(db, c.String("user"))
	if err != nil {
		log.Errorf("%s: %v", c.String("user"), err)
	}
	for _, f := range c.Args().Slice() {
		fn := filepath.Join(db.Dir(), f)

		in, err := os.Open(fn)
		if err != nil {
			return err
		}
		hdr, err := stingle.DecryptHeader(in, *sk)
		if err != nil {
			return err
		}
		out, err := os.CreateTemp("", "decryptedfile-*")
		if err != nil {
			in.Close()
			return err
		}
		r := stingle.DecryptFile(in, hdr)
		if _, err := io.Copy(out, r); err != nil {
			in.Close()
			out.Close()
			return err
		}
		if err := in.Close(); err != nil {
			return err
		}
		if err := out.Close(); err != nil {
			return err
		}
		log.Infof("Decrypted %s to %s", fn, out.Name())
	}
	return nil
}
