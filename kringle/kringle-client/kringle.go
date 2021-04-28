package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mattn/go-shellwords" // shellwords
	"github.com/urfave/cli/v2"       // cli
	"golang.org/x/term"

	"kringle/client"
	"kringle/crypto"
	"kringle/log"
	"kringle/secure"
)

type kringle struct {
	client *client.Client
	cli    *cli.App
	term   *term.Terminal

	// flags
	flagDataDir        string
	flagLogLevel       int
	flagPassphraseFile string
	flagAPIServer      string
}

func makeKringle() *kringle {
	var app kringle
	app.cli = &cli.App{
		Name:     "kringle",
		Usage:    "kringle client.",
		HideHelp: true,
		CommandNotFound: func(ctx *cli.Context, cmd string) {
			fmt.Fprintf(app.cli.Writer, "Unknown command %q. Try \"help\"\n", cmd)
		},
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "data-dir",
				Aliases:     []string{"d"},
				Value:       "",
				Usage:       "Save the data in `DIR`",
				Required:    true,
				EnvVars:     []string{"KRINGLE_DATADIR"},
				TakesFile:   true,
				Destination: &app.flagDataDir,
			},
			&cli.IntFlag{
				Name:        "verbose",
				Aliases:     []string{"v"},
				Value:       2,
				DefaultText: "2 (info)",
				Usage:       "The level of logging verbosity: 1:Error 2:Info 3:Debug",
				Destination: &app.flagLogLevel,
			},
			&cli.StringFlag{
				Name:        "passphrase-file",
				Value:       "",
				Usage:       "Read the database passphrase from `FILE`.",
				EnvVars:     []string{"KRINGLE_PASSPHRASE_FILE"},
				Destination: &app.flagPassphraseFile,
			},
			&cli.StringFlag{
				Name:        "server",
				Value:       "",
				Usage:       "The API server base URL.",
				Destination: &app.flagAPIServer,
			},
		},
		Commands: []*cli.Command{
			&cli.Command{
				Name:     "shell",
				Usage:    "Run in shell mode.",
				Action:   app.shell,
				Category: "Mode",
			},
			&cli.Command{
				Name:      "create-account",
				Usage:     "Create an account.",
				ArgsUsage: "<email>",
				Action:    app.createAccount,
				Category:  "Account",
			},
			&cli.Command{
				Name:      "login",
				Usage:     "Login to an account.",
				ArgsUsage: "<email>",
				Action:    app.login,
				Category:  "Account",
			},
			&cli.Command{
				Name:      "logout",
				Usage:     "Logout.",
				ArgsUsage: " ",
				Action:    app.logout,
				Category:  "Account",
			},
			&cli.Command{
				Name:      "updates",
				Aliases:   []string{"update"},
				Usage:     "Pull metadata updates from remote server.",
				ArgsUsage: " ",
				Action:    app.updates,
				Category:  "Sync",
			},
			&cli.Command{
				Name:      "download",
				Aliases:   []string{"pull"},
				Usage:     "Download files that aren't already downloaded.",
				ArgsUsage: `["glob"] ... (default "*/*")`,
				Action:    app.pullFiles,
				Category:  "Sync",
			},
			&cli.Command{
				Name:      "sync",
				Usage:     "Send changes to remote server.",
				ArgsUsage: " ",
				Action:    app.syncFiles,
				Category:  "Sync",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "dryrun",
						Value: false,
						Usage: "Show what would be synced without actually syncing.",
					},
				},
			},
			&cli.Command{
				Name:      "free",
				Usage:     "Remove local files that are backed up.",
				ArgsUsage: `["glob"] ... (default "*/*")`,
				Action:    app.freeFiles,
				Category:  "Sync",
			},
			&cli.Command{
				Name:      "create-album",
				Aliases:   []string{"mkdir"},
				Usage:     "Create new directory (album).",
				ArgsUsage: `<name> ...`,
				Action:    app.createAlbum,
				Category:  "Albums",
			},
			&cli.Command{
				Name:      "delete-album",
				Aliases:   []string{"rmdir"},
				Usage:     "Remove a directory (album).",
				ArgsUsage: `<name> ...`,
				Action:    app.removeAlbum,
				Category:  "Albums",
			},
			&cli.Command{
				Name:      "rename",
				Usage:     "Rename a directory (album).",
				ArgsUsage: `<old name> <new name>`,
				Action:    app.renameAlbum,
				Category:  "Albums",
			},
			&cli.Command{
				Name:      "hide",
				Usage:     "Hide directory.",
				ArgsUsage: `["glob"] ...`,
				Action:    app.hideAlbums,
				Category:  "Albums",
			},
			&cli.Command{
				Name:      "unhide",
				Usage:     "Unhide directory.",
				ArgsUsage: "[name] ...",
				Action:    app.unhideAlbums,
				Category:  "Albums",
			},
			&cli.Command{
				Name:      "list",
				Aliases:   []string{"ls"},
				Usage:     "List files and directories.",
				ArgsUsage: `["glob"] ... (default "*")`,
				Action:    app.listFiles,
				Category:  "Files",
			},
			&cli.Command{
				Name:      "copy",
				Aliases:   []string{"cp"},
				Usage:     "Copy files to a different directory.",
				ArgsUsage: `<"glob"> ... <dest>`,
				Action:    app.copyFiles,
				Category:  "Files",
			},
			&cli.Command{
				Name:      "move",
				Aliases:   []string{"mv"},
				Usage:     "Move files to a different directory, or rename a directory.",
				ArgsUsage: `<"glob"> ... <dest>`,
				Action:    app.moveFiles,
				Category:  "Files",
			},
			&cli.Command{
				Name:      "delete",
				Aliases:   []string{"rm", "remove"},
				Usage:     "Delete files (move them to trash, or delete them from trash).",
				ArgsUsage: `<"glob"> ...`,
				Action:    app.deleteFiles,
				Category:  "Files",
			},
			&cli.Command{
				Name:      "export",
				Usage:     "Decrypt and export files.",
				ArgsUsage: `"<glob>" ... <output directory>`,
				Action:    app.exportFiles,
				Category:  "Import/Export",
			},
			&cli.Command{
				Name:      "import",
				Usage:     "Encrypt and import files.",
				ArgsUsage: `"<glob>" ... <directory>`,
				Action:    app.importFiles,
				Category:  "Import/Export",
			},
		},
	}
	sort.Sort(cli.CommandsByName(app.cli.Commands))

	return &app
}

func (k *kringle) initClient(ctx *cli.Context, update bool) error {
	if k.client == nil {
		log.Level = k.flagLogLevel
		var pp string
		var err error
		if pp, err = k.passphrase(ctx); err != nil {
			return err
		}

		mkFile := filepath.Join(k.flagDataDir, "master.key")
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
		storage := secure.NewStorage(k.flagDataDir, &masterKey.EncryptionKey)

		c, err := client.Load(storage)
		if err != nil {
			c, err = client.Create(storage)
		}
		if k.flagAPIServer != "" {
			c.ServerBaseURL = k.flagAPIServer
		}
		k.client = c
	}
	if update && k.client.Token != "" {
		if err := k.client.GetUpdates(true); err != nil {
			return err
		}
	}
	return nil
}

func (k *kringle) shell(ctx *cli.Context) error {
	if err := k.initClient(ctx, false); err != nil {
		return err
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return errors.New("not a terminal")
	}
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		panic(err)
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	screen := struct {
		io.Reader
		io.Writer
	}{os.Stdin, os.Stdout}
	t := term.NewTerminal(screen, "kringle> ")
	/*
		t.AutoCompleteCallback = func(line string, pos int, key rune) (newLine string, newPos int, ok bool) {
			if key == '\t' {
				// Do something
			}
			return
		}
	*/
	k.cli.Writer = t
	k.client.SetWriter(t)
	k.term = t

	p := shellwords.NewParser()

	for {
		t.SetPrompt(string(t.Escape.Green) + "kringle> " + string(t.Escape.Reset))
		line, err := t.ReadLine()
		if err == io.EOF {
			return nil
		}
		line = strings.TrimSpace(line)
		args, err := p.Parse(line)
		if err != nil {
			fmt.Fprintf(t, "p.Parse: %v\n", err)
		}
		if len(args) == 0 {
			continue
		}
		switch args[0] {
		case "exit":
			return nil
		case "help":
			if len(args) > 1 {
				t.Write(t.Escape.Blue)
				cli.ShowCommandHelp(ctx, args[1])
				t.Write(t.Escape.Reset)
			} else {
				t.Write(t.Escape.Blue)
				cli.ShowCommandHelp(ctx, "")
				t.Write(t.Escape.Reset)
			}
		case "shell":
			fmt.Fprintf(t, "%sWe Need To Go Deeper%s\n", t.Escape.Red, t.Escape.Reset)
			fallthrough
		default:
			args = append([]string{"kringle"}, args...)
			if err := k.cli.Run(args); err != nil {
				fmt.Fprintf(t, "%s%v%s\n", t.Escape.Red, err, t.Escape.Reset)
			}
		}
	}
}

func (k *kringle) passphrase(ctx *cli.Context) (string, error) {
	if f := k.flagPassphraseFile; f != "" {
		p, err := os.ReadFile(f)
		if err != nil {
			return "", cli.Exit(err, 1)
		}
		return string(p), nil
	}
	return k.promptPass("Enter database passphrase: ")
}

func (k *kringle) promptPass(msg string) (string, error) {
	if k.term != nil {
		return k.term.ReadPassword(string(k.term.Escape.Green) + msg + string(k.term.Escape.Reset))
	}
	fmt.Print(msg)
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	return string(b), err
}

func (k *kringle) prompt(msg string) (reply string, err error) {
	if k.term != nil {
		k.term.SetPrompt(string(k.term.Escape.Green) + msg + string(k.term.Escape.Reset))
		return k.term.ReadLine()
	}
	fmt.Print(msg)
	_, err = fmt.Scanln(&reply)
	return
}

func (k *kringle) createAccount(ctx *cli.Context) error {
	if err := k.initClient(ctx, false); err != nil {
		return err
	}
	if k.client.ServerBaseURL == "" {
		var err error
		if k.client.ServerBaseURL, err = k.prompt("Enter server URL: "); err != nil {
			return err
		}
	}
	var email string
	if ctx.Args().Len() != 1 {
		var err error
		if email, err = k.prompt("Enter email: "); err != nil {
			return err
		}
	} else {
		email = ctx.Args().Get(0)
	}

	password, err := k.promptPass("Enter password: ")
	if err != nil {
		return err
	}
	return k.client.CreateAccount(email, password)
}

func (k *kringle) login(ctx *cli.Context) error {
	if err := k.initClient(ctx, false); err != nil {
		return err
	}
	if k.client.ServerBaseURL == "" {
		var err error
		if k.client.ServerBaseURL, err = k.prompt("Enter server URL: "); err != nil {
			return err
		}
	}
	var email string
	if ctx.Args().Len() != 1 {
		var err error
		if email, err = k.prompt("Enter email: "); err != nil {
			return err
		}
	} else {
		email = ctx.Args().Get(0)
	}

	password, err := k.promptPass("Enter password: ")
	if err != nil {
		return err
	}
	if err := k.client.Login(email, password); err != nil {
		return err
	}
	return k.client.GetUpdates(true)
}

func (k *kringle) logout(ctx *cli.Context) error {
	if err := k.initClient(ctx, false); err != nil {
		return err
	}
	return k.client.Logout()
}

func (k *kringle) updates(ctx *cli.Context) error {
	if err := k.initClient(ctx, false); err != nil {
		return err
	}
	if k.client.Email == "" {
		k.client.Print("Updates requires logging in to a remote server.")
		return nil
	}
	return k.client.GetUpdates(false)
}

func (k *kringle) pullFiles(ctx *cli.Context) error {
	if err := k.initClient(ctx, true); err != nil {
		return err
	}
	if k.client.Email == "" {
		k.client.Print("Pull requires logging in to a remote server.")
		return nil
	}
	patterns := []string{"*/*"}
	if ctx.Args().Len() > 0 {
		patterns = ctx.Args().Slice()
	}
	_, err := k.client.Pull(patterns)
	return err
}

func (k *kringle) syncFiles(ctx *cli.Context) error {
	if err := k.initClient(ctx, true); err != nil {
		return err
	}
	if k.client.Email == "" {
		k.client.Print("Sync requires logging in to a remote server.")
		return nil
	}
	return k.client.Sync(ctx.Bool("dryrun"))
}

func (k *kringle) freeFiles(ctx *cli.Context) error {
	if err := k.initClient(ctx, true); err != nil {
		return err
	}
	patterns := []string{"*/*"}
	if ctx.Args().Len() > 0 {
		patterns = ctx.Args().Slice()
	}
	_, err := k.client.Free(patterns)
	return err
}

func (k *kringle) createAlbum(ctx *cli.Context) error {
	if err := k.initClient(ctx, true); err != nil {
		return err
	}
	names := ctx.Args().Slice()
	if len(names) == 0 {
		return nil
	}
	return k.client.AddAlbums(names)
}

func (k *kringle) removeAlbum(ctx *cli.Context) error {
	if err := k.initClient(ctx, true); err != nil {
		return err
	}
	patterns := ctx.Args().Slice()
	if len(patterns) == 0 {
		return nil
	}
	return k.client.RemoveAlbums(patterns)
}

func (k *kringle) renameAlbum(ctx *cli.Context) error {
	if err := k.initClient(ctx, true); err != nil {
		return err
	}
	args := ctx.Args().Slice()
	if len(args) < 2 {
		cli.ShowSubcommandHelp(ctx)
		return nil
	}
	return k.client.RenameAlbum(args[:len(args)-1], args[len(args)-1])
}

func (k *kringle) hideAlbums(ctx *cli.Context) error {
	if err := k.initClient(ctx, true); err != nil {
		return err
	}
	patterns := []string{"*"}
	if ctx.Args().Len() > 0 {
		patterns = ctx.Args().Slice()
	}
	return k.client.Hide(patterns, true)
}

func (k *kringle) unhideAlbums(ctx *cli.Context) error {
	if err := k.initClient(ctx, true); err != nil {
		return err
	}
	patterns := []string{"*"}
	if ctx.Args().Len() > 0 {
		patterns = ctx.Args().Slice()
	}
	return k.client.Hide(patterns, false)
}

func (k *kringle) listFiles(ctx *cli.Context) error {
	if err := k.initClient(ctx, true); err != nil {
		return err
	}
	patterns := []string{"*"}
	if ctx.Args().Len() > 0 {
		patterns = ctx.Args().Slice()
	}
	return k.client.ListFiles(patterns)
}

func (k *kringle) copyFiles(ctx *cli.Context) error {
	if err := k.initClient(ctx, true); err != nil {
		return err
	}
	args := ctx.Args().Slice()
	if len(args) < 2 {
		cli.ShowSubcommandHelp(ctx)
		return nil
	}
	return k.client.Copy(args[:len(args)-1], args[len(args)-1])
}

func (k *kringle) moveFiles(ctx *cli.Context) error {
	if err := k.initClient(ctx, true); err != nil {
		return err
	}
	args := ctx.Args().Slice()
	if len(args) < 2 {
		cli.ShowSubcommandHelp(ctx)
		return nil
	}
	return k.client.Move(args[:len(args)-1], args[len(args)-1])
}

func (k *kringle) deleteFiles(ctx *cli.Context) error {
	if err := k.initClient(ctx, true); err != nil {
		return err
	}
	args := ctx.Args().Slice()
	if len(args) == 0 {
		cli.ShowSubcommandHelp(ctx)
		return nil
	}
	return k.client.Delete(args)
}

func (k *kringle) exportFiles(ctx *cli.Context) error {
	if err := k.initClient(ctx, true); err != nil {
		return err
	}
	args := ctx.Args().Slice()
	if len(args) < 2 {
		cli.ShowSubcommandHelp(ctx)
		return nil
	}
	patterns := args[:len(args)-1]
	dir := args[len(args)-1]
	_, err := k.client.ExportFiles(patterns, dir)
	return err
}

func (k *kringle) importFiles(ctx *cli.Context) error {
	if err := k.initClient(ctx, true); err != nil {
		return err
	}
	args := ctx.Args().Slice()
	if len(args) < 2 {
		cli.ShowSubcommandHelp(ctx)
		return nil
	}
	patterns := args[:len(args)-1]
	dir := args[len(args)-1]
	_, err := k.client.ImportFiles(patterns, dir)
	return err
}
