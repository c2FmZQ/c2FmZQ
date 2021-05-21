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

	"c2FmZQ/internal/client"
	"c2FmZQ/internal/client/fuse"
	"c2FmZQ/internal/crypto"
	"c2FmZQ/internal/log"
	"c2FmZQ/internal/secure"
	"c2FmZQ/licenses"
)

type App struct {
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

func New() *App {
	dataDir, err := os.UserConfigDir()
	if err != nil {
		dataDir, _ = os.UserHomeDir()
	}
	dataDir = filepath.Join(dataDir, ".c2FmZQ")

	var app App
	app.cli = &cli.App{
		Name:     "c2FmZQ",
		Usage:    "Keep your files away from prying eyes.",
		HideHelp: true,
		CommandNotFound: func(ctx *cli.Context, cmd string) {
			fmt.Fprintf(app.cli.Writer, "Unknown command %q. Try \"help\"\n", cmd)
		},
		UseShortOptionHandling: true,
	}
	app.cli.Flags = []cli.Flag{
		&cli.StringFlag{
			Name:        "data-dir",
			Aliases:     []string{"d"},
			Value:       dataDir,
			Usage:       "Save the data in `DIR`",
			EnvVars:     []string{"C2FMZQ_DATADIR"},
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
			EnvVars:     []string{"C2FMZQ_PASSPHRASE_FILE"},
			Destination: &app.flagPassphraseFile,
		},
		&cli.StringFlag{
			Name:        "server",
			Value:       "",
			Usage:       "The API server base URL.",
			EnvVars:     []string{"C2FMZQ_API_SERVER"},
			Destination: &app.flagAPIServer,
		},
		&cli.BoolFlag{
			Name:        "auto-update",
			Value:       true,
			Usage:       "Automatically fetch metadata updates from the remote server before each command.",
			Destination: &app.flagAutoUpdate,
		},
	}
	app.cli.Commands = []*cli.Command{
		&cli.Command{
			Name:     "licenses",
			Usage:    "Show the software licenses.",
			Action:   app.licenses,
			Category: "Misc",
		},
		&cli.Command{
			Name:     "shell",
			Usage:    "Run in shell mode.",
			Action:   app.shell,
			Category: "Mode",
		},
		&cli.Command{
			Name:      "mount",
			Usage:     "Mount as a fuse filesystem.",
			ArgsUsage: "<dir>",
			Action:    app.mount,
			Category:  "Mode",
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
			Name:      "delete-account",
			Usage:     "Delete the account and wipe all data.",
			ArgsUsage: " ",
			Action:    app.deleteAccount,
			Category:  "Account",
		},
		&cli.Command{
			Name:      "wipe-account",
			Usage:     "Wipe all local files associated with the current account.",
			ArgsUsage: " ",
			Action:    app.wipeAccount,
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
			Usage:     "Download a local copy of encrypted files.",
			ArgsUsage: `["glob"] ... (default "*")`,
			Action:    app.pullFiles,
			Category:  "Sync",
			Flags: []cli.Flag{
				&cli.BoolFlag{
					Name:    "recursive",
					Aliases: []string{"R"},
					Value:   true,
					Usage:   "Pull files recursively.",
				},
			},
		},
		&cli.Command{
			Name:      "sync",
			Usage:     "Upload changes to remote server.",
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
			Usage:     "Remove the local copy of encrypted files that are backed up.",
			ArgsUsage: `["glob"] ... (default "*")`,
			Action:    app.freeFiles,
			Category:  "Sync",
			Flags: []cli.Flag{
				&cli.BoolFlag{
					Name:    "recursive",
					Aliases: []string{"R"},
					Value:   true,
					Usage:   "Remove files recursively.",
				},
			},
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
			Name:      "list",
			Aliases:   []string{"ls"},
			Usage:     "List files and directories.",
			ArgsUsage: `["glob"] ... (default "*")`,
			Action:    app.listFiles,
			Category:  "Files",
			Flags: []cli.Flag{
				&cli.BoolFlag{
					Name:    "all",
					Aliases: []string{"a"},
					Value:   false,
					Usage:   "Show hidden files.",
				},
				&cli.BoolFlag{
					Name:    "long",
					Aliases: []string{"l"},
					Value:   false,
					Usage:   "Show long format.",
				},
				&cli.BoolFlag{
					Name:    "recursive",
					Aliases: []string{"R"},
					Value:   false,
					Usage:   "Show files recursively.",
				},
				&cli.BoolFlag{
					Name:    "directory",
					Aliases: []string{"d"},
					Value:   false,
					Usage:   "Show directories, not their content.",
				},
			},
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
			Flags: []cli.Flag{
				&cli.BoolFlag{
					Name:    "recursive",
					Aliases: []string{"R"},
					Value:   true,
					Usage:   "Export files recursively.",
				},
			},
		},
		&cli.Command{
			Name:      "import",
			Usage:     "Encrypt and import files.",
			ArgsUsage: `"<glob>" ... <directory>`,
			Action:    app.importFiles,
			Category:  "Import/Export",
			Flags: []cli.Flag{
				&cli.BoolFlag{
					Name:    "recursive",
					Aliases: []string{"R"},
					Value:   true,
					Usage:   "Import files recursively.",
				},
			},
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
			Usage:     "Remove a directory (album) that is shared with us.",
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
	}
	sort.Sort(cli.CommandsByName(app.cli.Commands))

	return &app
}

func (a *App) Run(args []string) error {
	return a.cli.Run(args)
}

func (a *App) init(ctx *cli.Context, update bool) error {
	if a.client == nil {
		log.Level = a.flagLogLevel
		var pp string
		var err error
		if pp, err = a.passphrase(ctx); err != nil {
			return err
		}

		mkFile := filepath.Join(a.flagDataDir, "master.key")
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
		storage := secure.NewStorage(a.flagDataDir, &masterKey.EncryptionKey)

		c, err := client.Load(masterKey, storage)
		if err != nil {
			if c, err = client.Create(masterKey, storage); err != nil {
				log.Fatalf("client.Create: %v", err)
			}
		}
		a.client = c
		a.client.SetPrompt(a.prompt)
	}
	if update && a.flagAutoUpdate && a.client.Account != nil {
		if err := a.client.GetUpdates(true); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) setupTerminal() (*term.Terminal, func()) {
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		panic(err)
	}

	screen := struct {
		io.Reader
		io.Writer
	}{os.Stdin, os.Stdout}
	t := term.NewTerminal(screen, "> ")
	return t, func() { term.Restore(int(os.Stdin.Fd()), oldState) }
}

func (a *App) shell(ctx *cli.Context) error {
	if err := a.init(ctx, false); err != nil {
		return err
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return errors.New("not a terminal")
	}
	p := shellwords.NewParser()

	width, height, err := term.GetSize(int(os.Stdin.Fd()))
	if err != nil {
		return err
	}
	t, reset := a.setupTerminal()
	defer reset()
	t.SetSize(width, height)
	t.AutoCompleteCallback = func(line string, pos int, key rune) (newLine string, newPos int, ok bool) {
		defer func() {
			if r := recover(); r != nil {
				fmt.Fprintln(t, "Recovered in auto-complete", r)
			}
		}()
		if key == '\t' {
			args, err := p.Parse(line[:pos])
			if err != nil {
				return
			}
			var currentWord string
			if len(args) > 0 {
				currentWord = args[len(args)-1]
			}
			if pos == 0 || (line[pos-1] == ' ' && currentWord[len(currentWord)-1] != ' ') {
				args = append(args, "")
				currentWord = ""
			}
			var options []autoCompleteOption
			if len(args) == 1 {
				options = a.commandOptions(a.cli.Commands, currentWord)
			}
			if len(args) > 1 {
				options = a.fileOptions(currentWord)
			}
			if len(options) == 0 {
				return
			}
			if len(options) > 1 {
				a.displayOptions(t, width, options)
			}
			prefix := a.commonPrefix(options)
			replace := line[:pos-len(escape(currentWord))] + options[0].name[:prefix]
			if prefix == len(options[0].name) && options[0].name[len(options[0].name)-1] != '/' {
				replace += " "
			}
			newLine = replace + line[pos:]
			newPos = len(replace)
			ok = true
		}
		return
	}

	a.cli.Writer = t
	a.client.SetWriter(t)
	a.term = t

	for {
		prompt := "local> "
		if a.client.Account != nil {
			prompt = "[" + a.client.Account.ServerBaseURL + "] " + a.client.Account.Email + "> "
		}
		t.SetPrompt(string(t.Escape.Green) + prompt + string(t.Escape.Reset))
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
			args = append([]string{"c2FmZQ"}, args...)
			if err := a.cli.Run(args); err != nil {
				fmt.Fprintf(t, "%s%v%s\n", t.Escape.Red, err, t.Escape.Reset)
			}
		}
	}
}

func (a *App) mount(ctx *cli.Context) error {
	if err := a.init(ctx, false); err != nil {
		return err
	}
	if ctx.Args().Len() != 1 {
		cli.ShowSubcommandHelp(ctx)
		return nil
	}
	return fuse.Mount(a.client, ctx.Args().Get(0))
}

func (a *App) passphrase(ctx *cli.Context) (string, error) {
	if f := a.flagPassphraseFile; f != "" {
		p, err := os.ReadFile(f)
		if err != nil {
			return "", cli.Exit(err, 1)
		}
		return string(p), nil
	}
	return a.promptPass("Enter database passphrase: ")
}

func (a *App) promptPass(msg string) (string, error) {
	t := a.term
	if t == nil {
		tt, reset := a.setupTerminal()
		defer reset()
		t = tt
	}
	return t.ReadPassword(string(t.Escape.Red) + msg + string(t.Escape.Reset))
}

func (a *App) prompt(msg string) (reply string, err error) {
	if a.term != nil {
		a.term.SetPrompt(string(a.term.Escape.Green) + msg + string(a.term.Escape.Reset))
		return a.term.ReadLine()
	}
	fmt.Print(msg)
	reply, err = bufio.NewReader(os.Stdin).ReadString('\n')
	reply = strings.TrimSpace(reply)
	return
}

func (a *App) createAccount(ctx *cli.Context) error {
	if err := a.init(ctx, false); err != nil {
		return err
	}
	server := a.flagAPIServer
	if server == "" {
		var err error
		if server, err = a.prompt("Enter server URL: "); err != nil {
			return err
		}
	}
	var email string
	if ctx.Args().Len() != 1 {
		var err error
		if email, err = a.prompt("Enter email: "); err != nil {
			return err
		}
	} else {
		email = ctx.Args().Get(0)
	}

	password, err := a.promptPass("Enter password: ")
	if err != nil {
		return err
	}
	return a.client.CreateAccount(server, email, password, ctx.Bool("backup"))
}

func (a *App) recoverAccount(ctx *cli.Context) error {
	if err := a.init(ctx, false); err != nil {
		return err
	}
	server := a.flagAPIServer
	if server == "" {
		var err error
		if server, err = a.prompt("Enter server URL: "); err != nil {
			return err
		}
	}
	var email string
	if ctx.Args().Len() != 1 {
		var err error
		if email, err = a.prompt("Enter email: "); err != nil {
			return err
		}
	} else {
		email = ctx.Args().Get(0)
	}
	phrase, err := a.prompt("Enter backup phrase: ")
	if err != nil {
		return err
	}

	password, err := a.promptPass("Enter new password: ")
	if err != nil {
		return err
	}
	return a.client.RecoverAccount(server, email, password, phrase, ctx.Bool("backup"))
}

func (a *App) changePassword(ctx *cli.Context) error {
	if err := a.init(ctx, false); err != nil {
		return err
	}
	if ctx.Args().Len() > 0 {
		cli.ShowSubcommandHelp(ctx)
		return nil
	}
	password, err := a.promptPass("Enter current password: ")
	if err != nil {
		return err
	}
	newPassword, err := a.promptPass("Enter new password: ")
	if err != nil {
		return err
	}
	newPassword2, err := a.promptPass("Re-enter new password: ")
	if err != nil {
		return err
	}
	if newPassword != newPassword2 {
		return errors.New("passwords do not match")
	}
	return a.client.ChangePassword(password, newPassword, ctx.Bool("backup"))
}

func (a *App) setKeyBackup(ctx *cli.Context) error {
	if err := a.init(ctx, false); err != nil {
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
	if a.client.Account == nil {
		a.client.Print("Not logged in.")
		return nil
	}
	password, err := a.promptPass("Enter password: ")
	if err != nil {
		return err
	}
	return a.client.UploadKeys(password, doBackup)
}

func (a *App) login(ctx *cli.Context) error {
	if err := a.init(ctx, false); err != nil {
		return err
	}
	server := a.flagAPIServer
	if server == "" {
		var err error
		if server, err = a.prompt("Enter server URL: "); err != nil {
			return err
		}
	}
	var email string
	if ctx.Args().Len() != 1 {
		var err error
		if email, err = a.prompt("Enter email: "); err != nil {
			return err
		}
	} else {
		email = ctx.Args().Get(0)
	}

	password, err := a.promptPass("Enter password: ")
	if err != nil {
		return err
	}
	if err := a.client.Login(server, email, password); err != nil {
		return err
	}
	return a.client.GetUpdates(true)
}

func (a *App) logout(ctx *cli.Context) error {
	if err := a.init(ctx, false); err != nil {
		return err
	}
	return a.client.Logout()
}

func (a *App) status(ctx *cli.Context) error {
	if err := a.init(ctx, false); err != nil {
		return err
	}
	return a.client.Status()
}

func (a *App) backupPhrase(ctx *cli.Context) error {
	if err := a.init(ctx, false); err != nil {
		return err
	}
	if a.client.Account == nil {
		a.client.Print("Not logged in.")
		return nil
	}
	a.client.Print("\nWARNING: The backup phrase must be kept secret. It can be used to access all your data.\n")
	password, err := a.promptPass("Enter password: ")
	if err != nil {
		return err
	}
	return a.client.BackupPhrase(password)
}

func (a *App) deleteAccount(ctx *cli.Context) error {
	if err := a.init(ctx, false); err != nil {
		return err
	}
	if a.client.Account == nil {
		a.client.Print("Not logged in.")
		return nil
	}
	if err := a.client.Status(); err != nil {
		return err
	}
	a.client.Print("\n*********************************************************************")
	a.client.Print("WARNING: You are about to delete your account and wipe all your data.")
	a.client.Print("*********************************************************************\n")
	password, err := a.promptPass("Enter password: ")
	if err != nil {
		return err
	}
	return a.client.DeleteAccount(password)
}

func (a *App) wipeAccount(ctx *cli.Context) error {
	if err := a.init(ctx, false); err != nil {
		return err
	}
	if err := a.client.Status(); err != nil {
		return err
	}
	a.client.Print("\n*********************************************")
	a.client.Print("WARNING: You are about to wipe all your data.")
	a.client.Print("***********************************************\n")
	if a.client.Account != nil {
		password, err := a.promptPass("Enter password: ")
		if err != nil {
			return err
		}
		return a.client.WipeAccount(password)
	}
	if reply, err := a.prompt("Type WIPE to confirm: "); err != nil || reply != "WIPE" {
		return errors.New("not confirmed")
	}
	return a.client.WipeAccount("")
}

func (a *App) updates(ctx *cli.Context) error {
	if err := a.init(ctx, false); err != nil {
		return err
	}
	if a.client.Account == nil {
		a.client.Print("Updates requires logging in to a remote server.")
		return nil
	}
	return a.client.GetUpdates(false)
}

func (a *App) pullFiles(ctx *cli.Context) error {
	if err := a.init(ctx, true); err != nil {
		return err
	}
	if a.client.Account == nil {
		a.client.Print("Pull requires logging in to a remote server.")
		return nil
	}
	patterns := []string{"*"}
	if ctx.Args().Len() > 0 {
		patterns = ctx.Args().Slice()
	}
	opt := client.GlobOptions{}
	if ctx.Bool("recursive") {
		opt.Recursive = true
	}
	_, err := a.client.Pull(patterns, opt)
	return err
}

func (a *App) syncFiles(ctx *cli.Context) error {
	if err := a.init(ctx, true); err != nil {
		return err
	}
	if a.client.Account == nil {
		a.client.Print("Sync requires logging in to a remote server.")
		return nil
	}
	return a.client.Sync(ctx.Bool("dryrun"))
}

func (a *App) freeFiles(ctx *cli.Context) error {
	if err := a.init(ctx, true); err != nil {
		return err
	}
	patterns := []string{"*"}
	if ctx.Args().Len() > 0 {
		patterns = ctx.Args().Slice()
	}
	opt := client.GlobOptions{}
	if ctx.Bool("recursive") {
		opt.Recursive = true
	}
	_, err := a.client.Free(patterns, opt)
	return err
}

func (a *App) createAlbum(ctx *cli.Context) error {
	if err := a.init(ctx, true); err != nil {
		return err
	}
	names := ctx.Args().Slice()
	if len(names) == 0 {
		cli.ShowSubcommandHelp(ctx)
		return nil
	}
	return a.client.AddAlbums(names)
}

func (a *App) removeAlbum(ctx *cli.Context) error {
	if err := a.init(ctx, true); err != nil {
		return err
	}
	patterns := ctx.Args().Slice()
	if len(patterns) == 0 {
		cli.ShowSubcommandHelp(ctx)
		return nil
	}
	return a.client.RemoveAlbums(patterns)
}

func (a *App) renameAlbum(ctx *cli.Context) error {
	if err := a.init(ctx, true); err != nil {
		return err
	}
	args := ctx.Args().Slice()
	if len(args) < 2 {
		cli.ShowSubcommandHelp(ctx)
		return nil
	}
	return a.client.RenameAlbum(args[:len(args)-1], args[len(args)-1])
}

func (a *App) listFiles(ctx *cli.Context) error {
	if err := a.init(ctx, true); err != nil {
		return err
	}
	patterns := []string{""}
	if ctx.Args().Len() > 0 {
		patterns = ctx.Args().Slice()
	}
	opt := client.GlobOptions{}
	if ctx.Bool("all") {
		opt.MatchDot = true
	}
	if ctx.Bool("long") {
		opt.Long = true
	}
	if ctx.Bool("recursive") {
		opt.Recursive = true
	}
	if ctx.Bool("directory") {
		opt.Directory = true
	}
	return a.client.ListFiles(patterns, opt)
}

func (a *App) copyFiles(ctx *cli.Context) error {
	if err := a.init(ctx, true); err != nil {
		return err
	}
	args := ctx.Args().Slice()
	if len(args) < 2 {
		cli.ShowSubcommandHelp(ctx)
		return nil
	}
	return a.client.Copy(args[:len(args)-1], args[len(args)-1], false)
}

func (a *App) moveFiles(ctx *cli.Context) error {
	if err := a.init(ctx, true); err != nil {
		return err
	}
	args := ctx.Args().Slice()
	if len(args) < 2 {
		cli.ShowSubcommandHelp(ctx)
		return nil
	}
	return a.client.Move(args[:len(args)-1], args[len(args)-1], false)
}

func (a *App) deleteFiles(ctx *cli.Context) error {
	if err := a.init(ctx, true); err != nil {
		return err
	}
	args := ctx.Args().Slice()
	if len(args) == 0 {
		cli.ShowSubcommandHelp(ctx)
		return nil
	}
	return a.client.Delete(args, false)
}

func (a *App) catFiles(ctx *cli.Context) error {
	if err := a.init(ctx, true); err != nil {
		return err
	}
	args := ctx.Args().Slice()
	if len(args) == 0 {
		cli.ShowSubcommandHelp(ctx)
		return nil
	}
	return a.client.Cat(args)
}

func (a *App) exportFiles(ctx *cli.Context) error {
	if err := a.init(ctx, true); err != nil {
		return err
	}
	args := ctx.Args().Slice()
	if len(args) < 2 {
		cli.ShowSubcommandHelp(ctx)
		return nil
	}
	patterns := args[:len(args)-1]
	dir := args[len(args)-1]
	_, err := a.client.ExportFiles(patterns, dir, ctx.Bool("recursive"))
	return err
}

func (a *App) importFiles(ctx *cli.Context) error {
	if err := a.init(ctx, true); err != nil {
		return err
	}
	args := ctx.Args().Slice()
	if len(args) < 2 {
		cli.ShowSubcommandHelp(ctx)
		return nil
	}
	patterns := args[:len(args)-1]
	dir := args[len(args)-1]
	_, err := a.client.ImportFiles(patterns, dir, ctx.Bool("recursive"))
	return err
}

func (a *App) shareAlbum(ctx *cli.Context) error {
	if err := a.init(ctx, true); err != nil {
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
	return a.client.Share(pattern, emails, perms)
}

func (a *App) unshareAlbum(ctx *cli.Context) error {
	if err := a.init(ctx, true); err != nil {
		return err
	}
	args := ctx.Args().Slice()
	if len(args) == 0 {
		cli.ShowSubcommandHelp(ctx)
		return nil
	}
	return a.client.Unshare(args)
}

func (a *App) leaveAlbum(ctx *cli.Context) error {
	if err := a.init(ctx, true); err != nil {
		return err
	}
	args := ctx.Args().Slice()
	if len(args) == 0 {
		cli.ShowSubcommandHelp(ctx)
		return nil
	}
	return a.client.Leave(args)
}

func (a *App) removeMember(ctx *cli.Context) error {
	if err := a.init(ctx, true); err != nil {
		return err
	}
	args := ctx.Args().Slice()
	if len(args) < 2 {
		cli.ShowSubcommandHelp(ctx)
		return nil
	}
	pattern := args[0]
	emails := args[1:]
	return a.client.RemoveMembers(pattern, emails)
}

func (a *App) changePermissions(ctx *cli.Context) error {
	if err := a.init(ctx, true); err != nil {
		return err
	}
	args := ctx.Args().Slice()
	if len(args) < 2 {
		cli.ShowSubcommandHelp(ctx)
		return nil
	}
	perms := strings.Split(args[0], ",")
	patterns := args[1:]
	return a.client.ChangePermissions(patterns, perms)
}

func (a *App) listContacts(ctx *cli.Context) error {
	if err := a.init(ctx, true); err != nil {
		return err
	}
	patterns := ctx.Args().Slice()
	if len(patterns) == 0 {
		patterns = []string{"*"}
	}
	return a.client.Contacts(patterns)
}

func (a *App) licenses(ctx *cli.Context) error {
	licenses.Show()
	return nil
}
