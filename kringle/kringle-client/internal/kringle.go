package internal

import (
	"bufio"
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

	"kringle/internal/client"
	"kringle/internal/crypto"
	"kringle/internal/log"
	"kringle/internal/secure"
)

type kringle struct {
	cli    *cli.App
	client *client.Client
	term   *term.Terminal

	// flags
	flagDataDir        string
	flagLogLevel       int
	flagPassphraseFile string
	flagAPIServer      string
	flagAutoUpdate     bool
}

func New() *kringle {
	dataDir, err := os.UserConfigDir()
	if err != nil {
		dataDir, _ = os.UserHomeDir()
	}
	dataDir = filepath.Join(dataDir, ".kringle")

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
				Value:       dataDir,
				Usage:       "Save the data in `DIR`",
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
				EnvVars:     []string{"KRINGLE_API_SERVER"},
				Destination: &app.flagAPIServer,
			},
			&cli.BoolFlag{
				Name:        "auto-update",
				Value:       true,
				Usage:       "Automatically fetch metadata updates from the remote server before each command.",
				Destination: &app.flagAutoUpdate,
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
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "backup",
						Value: true,
						Usage: "Backup encrypted secret key on remote server.",
					},
				},
			},
			&cli.Command{
				Name:      "recover-account",
				Usage:     "Recover an account with backup phrase.",
				ArgsUsage: "<email>",
				Action:    app.recoverAccount,
				Category:  "Account",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "backup",
						Value: true,
						Usage: "Backup encrypted secret key on remote server.",
					},
				},
			},
			&cli.Command{
				Name:     "change-password",
				Usage:    "Change the user's password.",
				Action:   app.changePassword,
				Category: "Account",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "backup",
						Value: true,
						Usage: "Backup encrypted secret key on remote server.",
					},
				},
			},
			&cli.Command{
				Name:      "set-key-backup",
				Usage:     "Enable or disable secret key backup.",
				ArgsUsage: "<on|off>",
				Action:    app.setKeyBackup,
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
				Name:      "status",
				Usage:     "Show the client's status.",
				ArgsUsage: " ",
				Action:    app.status,
				Category:  "Account",
			},
			&cli.Command{
				Name:      "backup-phrase",
				Usage:     "Show the backup phrase for the current account. The backup phrase must be kept secret.",
				ArgsUsage: " ",
				Action:    app.backupPhrase,
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
				Usage:     "Hide directory (album).",
				ArgsUsage: `["glob"] ...`,
				Action:    app.hideAlbums,
				Category:  "Albums",
			},
			&cli.Command{
				Name:      "unhide",
				Usage:     "Unhide directory (album).",
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
				Name:      "cat",
				Aliases:   []string{"show"},
				Usage:     "Decrypt files and send their content to standard output.",
				ArgsUsage: `<"glob"> ...`,
				Action:    app.catFiles,
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
			&cli.Command{
				Name:      "share",
				Usage:     "Share a directory (album) with other people.",
				ArgsUsage: `"<glob>" <email> ...`,
				Action:    app.shareAlbum,
				Category:  "Share",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "perm",
						Aliases: []string{"p", "perms", "permissions"},
						Value:   "",
						Usage:   "Comma-separated list of album permissions: 'Add', 'Share', 'Copy', e.g. --perm=Add,Share .",
					},
				},
			},
			&cli.Command{
				Name:      "unshare",
				Usage:     "Stop sharing a directory (album).",
				ArgsUsage: `"<glob>" ...`,
				Action:    app.unshareAlbum,
				Category:  "Share",
			},
			&cli.Command{
				Name:      "leave",
				Usage:     "Remove a directory that is shared with us.",
				ArgsUsage: `"<glob>" ...`,
				Action:    app.leaveAlbum,
				Category:  "Share",
			},
			&cli.Command{
				Name:      "remove-member",
				Usage:     "Remove members from a directory (album).",
				ArgsUsage: `"<glob>" <email> ...`,
				Action:    app.removeMember,
				Category:  "Share",
			},
			&cli.Command{
				Name:      "change-permissions",
				Aliases:   []string{"chmod"},
				Usage:     "Change the permissions on a shared directory (album).",
				ArgsUsage: `<comma-separated permissions> "<glob>" ...   e.g. +Add,-Share MyAlbum`,
				Action:    app.changePermissions,
				Category:  "Share",
			},
			&cli.Command{
				Name:      "contacts",
				Usage:     "List contacts.",
				ArgsUsage: `"<glob>" ...`,
				Action:    app.listContacts,
				Category:  "Share",
			},
		},
	}
	sort.Sort(cli.CommandsByName(app.cli.Commands))

	return &app
}

func (k *kringle) Run(args []string) error {
	return k.cli.Run(args)
}

func (k *kringle) init(ctx *cli.Context, update bool) error {
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
		k.client = c
		k.client.SetPrompt(k.prompt)
	}
	if update && k.flagAutoUpdate && k.client.Account != nil {
		if err := k.client.GetUpdates(true); err != nil {
			return err
		}
	}
	return nil
}

func (k *kringle) setupTerminal() (*term.Terminal, func()) {
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		panic(err)
	}

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
	return t, func() { term.Restore(int(os.Stdin.Fd()), oldState) }
}

func (k *kringle) shell(ctx *cli.Context) error {
	if err := k.init(ctx, false); err != nil {
		return err
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return errors.New("not a terminal")
	}
	t, reset := k.setupTerminal()
	defer reset()

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
	t := k.term
	if t == nil {
		tt, reset := k.setupTerminal()
		defer reset()
		t = tt
	}
	return t.ReadPassword(string(t.Escape.Red) + msg + string(t.Escape.Reset))
}

func (k *kringle) prompt(msg string) (reply string, err error) {
	if k.term != nil {
		k.term.SetPrompt(string(k.term.Escape.Green) + msg + string(k.term.Escape.Reset))
		return k.term.ReadLine()
	}
	fmt.Print(msg)
	reply, err = bufio.NewReader(os.Stdin).ReadString('\n')
	reply = strings.TrimSpace(reply)
	return
}

func (k *kringle) createAccount(ctx *cli.Context) error {
	if err := k.init(ctx, false); err != nil {
		return err
	}
	server := k.flagAPIServer
	if server == "" {
		var err error
		if server, err = k.prompt("Enter server URL: "); err != nil {
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
	return k.client.CreateAccount(server, email, password, ctx.Bool("backup"))
}

func (k *kringle) recoverAccount(ctx *cli.Context) error {
	if err := k.init(ctx, false); err != nil {
		return err
	}
	server := k.flagAPIServer
	if server == "" {
		var err error
		if server, err = k.prompt("Enter server URL: "); err != nil {
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
	phrase, err := k.prompt("Enter backup phrase: ")
	if err != nil {
		return err
	}

	password, err := k.promptPass("Enter new password: ")
	if err != nil {
		return err
	}
	return k.client.RecoverAccount(server, email, password, phrase, ctx.Bool("backup"))
}

func (k *kringle) changePassword(ctx *cli.Context) error {
	if err := k.init(ctx, false); err != nil {
		return err
	}
	if ctx.Args().Len() > 0 {
		cli.ShowSubcommandHelp(ctx)
		return nil
	}
	password, err := k.promptPass("Enter current password: ")
	if err != nil {
		return err
	}
	newPassword, err := k.promptPass("Enter new password: ")
	if err != nil {
		return err
	}
	newPassword2, err := k.promptPass("Re-enter new password: ")
	if err != nil {
		return err
	}
	if newPassword != newPassword2 {
		return errors.New("passwords do not match")
	}
	return k.client.ChangePassword(password, newPassword, ctx.Bool("backup"))
}

func (k *kringle) setKeyBackup(ctx *cli.Context) error {
	if err := k.init(ctx, false); err != nil {
		return err
	}
	if ctx.Args().Len() != 1 {
		cli.ShowSubcommandHelp(ctx)
		return nil
	}
	var doBackup bool
	switch arg := ctx.Args().Get(0); strings.ToLower(arg) {
	case "on":
		doBackup = true
	case "off":
		doBackup = false
	default:
		cli.ShowSubcommandHelp(ctx)
		return nil
	}
	if k.client.Account == nil {
		k.client.Print("Not logged in.")
		return nil
	}
	password, err := k.promptPass("Enter password: ")
	if err != nil {
		return err
	}
	return k.client.UploadKeys(password, doBackup)
}

func (k *kringle) login(ctx *cli.Context) error {
	if err := k.init(ctx, false); err != nil {
		return err
	}
	server := k.flagAPIServer
	if server == "" {
		var err error
		if server, err = k.prompt("Enter server URL: "); err != nil {
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
	if err := k.client.Login(server, email, password); err != nil {
		return err
	}
	return k.client.GetUpdates(true)
}

func (k *kringle) logout(ctx *cli.Context) error {
	if err := k.init(ctx, false); err != nil {
		return err
	}
	return k.client.Logout()
}

func (k *kringle) status(ctx *cli.Context) error {
	if err := k.init(ctx, false); err != nil {
		return err
	}
	return k.client.Status()
}

func (k *kringle) backupPhrase(ctx *cli.Context) error {
	if err := k.init(ctx, false); err != nil {
		return err
	}
	if k.client.Account == nil {
		k.client.Print("Not logged in.")
		return nil
	}
	k.client.Print("\nWARNING: The backup phrase must be kept secret. It can be used to access all your data.\n")
	password, err := k.promptPass("Enter password: ")
	if err != nil {
		return err
	}
	return k.client.BackupPhrase(password)
}

func (k *kringle) updates(ctx *cli.Context) error {
	if err := k.init(ctx, false); err != nil {
		return err
	}
	if k.client.Account == nil {
		k.client.Print("Updates requires logging in to a remote server.")
		return nil
	}
	return k.client.GetUpdates(false)
}

func (k *kringle) pullFiles(ctx *cli.Context) error {
	if err := k.init(ctx, true); err != nil {
		return err
	}
	if k.client.Account == nil {
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
	if err := k.init(ctx, true); err != nil {
		return err
	}
	if k.client.Account == nil {
		k.client.Print("Sync requires logging in to a remote server.")
		return nil
	}
	return k.client.Sync(ctx.Bool("dryrun"))
}

func (k *kringle) freeFiles(ctx *cli.Context) error {
	if err := k.init(ctx, true); err != nil {
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
	if err := k.init(ctx, true); err != nil {
		return err
	}
	names := ctx.Args().Slice()
	if len(names) == 0 {
		cli.ShowSubcommandHelp(ctx)
		return nil
	}
	return k.client.AddAlbums(names)
}

func (k *kringle) removeAlbum(ctx *cli.Context) error {
	if err := k.init(ctx, true); err != nil {
		return err
	}
	patterns := ctx.Args().Slice()
	if len(patterns) == 0 {
		cli.ShowSubcommandHelp(ctx)
		return nil
	}
	return k.client.RemoveAlbums(patterns)
}

func (k *kringle) renameAlbum(ctx *cli.Context) error {
	if err := k.init(ctx, true); err != nil {
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
	if err := k.init(ctx, true); err != nil {
		return err
	}
	if ctx.Args().Len() == 0 {
		cli.ShowSubcommandHelp(ctx)
	}
	patterns := ctx.Args().Slice()
	return k.client.Hide(patterns, true)
}

func (k *kringle) unhideAlbums(ctx *cli.Context) error {
	if err := k.init(ctx, true); err != nil {
		return err
	}
	if ctx.Args().Len() == 0 {
		cli.ShowSubcommandHelp(ctx)
	}
	patterns := ctx.Args().Slice()
	return k.client.Hide(patterns, false)
}

func (k *kringle) listFiles(ctx *cli.Context) error {
	if err := k.init(ctx, true); err != nil {
		return err
	}
	patterns := []string{"*"}
	if ctx.Args().Len() > 0 {
		patterns = ctx.Args().Slice()
	}
	return k.client.ListFiles(patterns)
}

func (k *kringle) copyFiles(ctx *cli.Context) error {
	if err := k.init(ctx, true); err != nil {
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
	if err := k.init(ctx, true); err != nil {
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
	if err := k.init(ctx, true); err != nil {
		return err
	}
	args := ctx.Args().Slice()
	if len(args) == 0 {
		cli.ShowSubcommandHelp(ctx)
		return nil
	}
	return k.client.Delete(args)
}

func (k *kringle) catFiles(ctx *cli.Context) error {
	if err := k.init(ctx, true); err != nil {
		return err
	}
	args := ctx.Args().Slice()
	if len(args) == 0 {
		cli.ShowSubcommandHelp(ctx)
		return nil
	}
	return k.client.Cat(args)
}

func (k *kringle) exportFiles(ctx *cli.Context) error {
	if err := k.init(ctx, true); err != nil {
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
	if err := k.init(ctx, true); err != nil {
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

func (k *kringle) shareAlbum(ctx *cli.Context) error {
	if err := k.init(ctx, true); err != nil {
		return err
	}
	args := ctx.Args().Slice()
	if len(args) < 2 {
		cli.ShowSubcommandHelp(ctx)
		return nil
	}
	pattern := args[0]
	emails := args[1:]
	perms := strings.Split(ctx.String("perm"), ",")
	if len(perms) == 1 && perms[0] == "" {
		perms = nil
	}
	return k.client.Share(pattern, emails, perms)
}

func (k *kringle) unshareAlbum(ctx *cli.Context) error {
	if err := k.init(ctx, true); err != nil {
		return err
	}
	args := ctx.Args().Slice()
	if len(args) == 0 {
		cli.ShowSubcommandHelp(ctx)
		return nil
	}
	return k.client.Unshare(args)
}

func (k *kringle) leaveAlbum(ctx *cli.Context) error {
	if err := k.init(ctx, true); err != nil {
		return err
	}
	args := ctx.Args().Slice()
	if len(args) == 0 {
		cli.ShowSubcommandHelp(ctx)
		return nil
	}
	return k.client.Leave(args)
}

func (k *kringle) removeMember(ctx *cli.Context) error {
	if err := k.init(ctx, true); err != nil {
		return err
	}
	args := ctx.Args().Slice()
	if len(args) < 2 {
		cli.ShowSubcommandHelp(ctx)
		return nil
	}
	pattern := args[0]
	emails := args[1:]
	return k.client.RemoveMembers(pattern, emails)
}

func (k *kringle) changePermissions(ctx *cli.Context) error {
	if err := k.init(ctx, true); err != nil {
		return err
	}
	args := ctx.Args().Slice()
	if len(args) < 2 {
		cli.ShowSubcommandHelp(ctx)
		return nil
	}
	perms := strings.Split(args[0], ",")
	patterns := args[1:]
	return k.client.ChangePermissions(patterns, perms)
}

func (k *kringle) listContacts(ctx *cli.Context) error {
	if err := k.init(ctx, true); err != nil {
		return err
	}
	patterns := ctx.Args().Slice()
	if len(patterns) == 0 {
		patterns = []string{"*"}
	}
	return k.client.Contacts(patterns)
}
