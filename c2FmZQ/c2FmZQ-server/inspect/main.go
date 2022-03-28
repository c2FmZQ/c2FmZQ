package main

import (
	"bufio"
	"bytes"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/mdp/qrterminal"
	"github.com/pquerna/otp/totp"
	"github.com/urfave/cli/v2" // cli
	"golang.org/x/term"

	"c2FmZQ/internal/crypto"
	"c2FmZQ/internal/database"
	"c2FmZQ/internal/log"
	"c2FmZQ/internal/secure"
	"c2FmZQ/internal/stingle"
)

var (
	flagDatabase        string
	flagLogLevel        int
	flagEncryptMetadata bool
	flagPassphraseFile  string
	flagPassphraseCmd   string
	flagPassphraseEnv   string
)

func main() {
	var defaultDB string
	if home, err := os.UserHomeDir(); err == nil {
		defaultDB = filepath.Join(home, "c2FmZQ-server", "data")
	}
	app := &cli.App{
		Name:     "inspect",
		Usage:    "Access internal information from the c2FmZQ database.",
		HideHelp: true,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "database",
				Aliases:     []string{"db"},
				Value:       defaultDB,
				Usage:       "Use the database in `DIR`",
				EnvVars:     []string{"C2FMZQ_DATABASE"},
				TakesFile:   true,
				Destination: &flagDatabase,
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
				Usage:       "Whether the metadata is encrypted.",
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
				Name:        "passphrase-env",
				Value:       "",
				Usage:       "Use value of `ENV` as database passphrase.",
				EnvVars:     []string{"C2FMZQ_PASSPHRASE_ENV"},
				Destination: &flagPassphraseEnv,
			},
		},
		Commands: []*cli.Command{
			&cli.Command{
				Name:     "users",
				Category: "Users",
				Usage:    "Show the list of users.",
				Action:   showUsers,
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:    "long",
						Usage:   "Show details.",
						Aliases: []string{"l"},
					},
				},
			},
			&cli.Command{
				Name:     "cat",
				Category: "System",
				Aliases:  []string{"show", "dump"},
				Usage:    "Show the content of database files.",
				Action:   catFile,
			},
			&cli.Command{
				Name:     "edit",
				Category: "Users",
				Usage:    "Edit database files.",
				Subcommands: []*cli.Command{
					&cli.Command{
						Name:      "albums",
						Aliases:   []string{"album"},
						Usage:     "Edit a user's album list.",
						ArgsUsage: " ",
						Action:    editAlbums,
						Flags: []cli.Flag{
							&cli.Int64Flag{
								Name:    "userid",
								Usage:   "The userid of the user.",
								Aliases: []string{"u"},
							},
						},
					},
					&cli.Command{
						Name:      "contacts",
						Aliases:   []string{"contact"},
						Usage:     "Edit a user's contact list.",
						ArgsUsage: " ",
						Action:    editContacts,
						Flags: []cli.Flag{
							&cli.Int64Flag{
								Name:    "userid",
								Usage:   "The userid of the user.",
								Aliases: []string{"u"},
							},
						},
					},
					&cli.Command{
						Name:      "fileset",
						Aliases:   []string{"fs"},
						Usage:     "Edit a fileset.",
						ArgsUsage: "<file>",
						Action:    editFileset,
					},
					&cli.Command{
						Name:      "quotas",
						Aliases:   []string{"quota"},
						Usage:     "Edit quotas.",
						ArgsUsage: " ",
						Action:    editQuotas,
					},
					&cli.Command{
						Name:      "user",
						Usage:     "Edit a user file.",
						Action:    editUser,
						ArgsUsage: " ",
						Flags: []cli.Flag{
							&cli.Int64Flag{
								Name:    "userid",
								Usage:   "The userid of the user.",
								Aliases: []string{"u"},
							},
						},
					},
					&cli.Command{
						Name:    "userlist",
						Aliases: []string{"ul"},
						Usage:   "Edit the user list.",
						Action:  editUserList,
					},
				},
			},
			&cli.Command{
				Name:     "orphans",
				Category: "System",
				Usage:    "Find orphans files.",
				Action:   findOrphanFiles,
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "delete",
						Usage: "Delete the orphan files.",
					},
				},
			},
			&cli.Command{
				Name:     "change-passphrase",
				Category: "System",
				Usage:    "Change the passphrase that protects the database's master key.",
				Action:   changePassphrase,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "new-passphrase-command",
						Value: "",
						Usage: "Read the new database passphrase from the standard output of `COMMAND`.",
					},
					&cli.StringFlag{
						Name:  "new-passphrase-file",
						Value: "",
						Usage: "Read the new database passphrase from `FILE`.",
					},
					&cli.StringFlag{
						Name:  "new-passphrase-env",
						Value: "",
						Usage: "Change database passphrase to value of `ENV`.",
					},
				},
			},
			&cli.Command{
				Name:     "change-master-key",
				Category: "System",
				Usage:    "Re-encrypt all the data with a new master key. This can take a while.",
				Action:   changeMasterKey,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "format",
						Value: "auto",
						Usage: "The format of the new master key ('auto', 'aes', 'chacha20poly1305')",
					},
				},
			},
			&cli.Command{
				Name:     "rename-user",
				Category: "System",
				Usage:    "Change the email address of a user.",
				Action:   renameUser,
				Flags: []cli.Flag{
					&cli.Int64Flag{
						Name:  "userid",
						Usage: "The userid to update.",
					},
					&cli.StringFlag{
						Name:  "new-email",
						Usage: "The new email address of the user.",
					},
				},
			},
			&cli.Command{
				Name:     "otp",
				Category: "Users",
				Usage:    "Show, set, or clear a user's OTP key.",
				Action:   changeUserOTPKey,
				Flags: []cli.Flag{
					&cli.Int64Flag{
						Name:    "userid",
						Usage:   "The userid to update.",
						Aliases: []string{"u"},
					},
					&cli.BoolFlag{
						Name:  "set",
						Usage: "OTP should be enabled for this user.",
					},
					&cli.BoolFlag{
						Name:  "clear",
						Usage: "OTP should be disabled for this user.",
					},
				},
			},
			&cli.Command{
				Name:     "decoy",
				Category: "Users",
				Usage:    "Show, set, change, or clear a user's decoy password.",
				Action:   changeUserDecoy,
				Flags: []cli.Flag{
					&cli.Int64Flag{
						Name:    "userid",
						Usage:   "The userid to update.",
						Aliases: []string{"u"},
					},
					&cli.BoolFlag{
						Name:  "set",
						Usage: "Set a decoy password.",
					},
					&cli.BoolFlag{
						Name:  "change",
						Usage: "Change a decoy password.",
					},
					&cli.BoolFlag{
						Name:  "clear",
						Usage: "Clear a decoy password.",
					},
					&cli.BoolFlag{
						Name:  "show",
						Usage: "show decoy passwords.",
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
	var pp []byte
	if flagEncryptMetadata {
		var err error
		if pp, err = crypto.Passphrase(flagPassphraseCmd, flagPassphraseFile, flagPassphraseEnv); err != nil {
			return nil, err
		}
	}
	return database.New(flagDatabase, pp), nil
}

func createParent(filename string) {
	dir, _ := filepath.Split(filename)
	if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(dir, 0700); err != nil {
			log.Fatalf("os.MkdirAll(%q): %v", dir, err)
		}
	}
}

func prompt(msg string) string {
	fmt.Print(msg)
	reply, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	return strings.TrimSpace(reply)
}

func promptPassword(msg string) string {
	fmt.Print(msg)
	password, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		log.Errorf("term.ReadPassword: %v", err)
		return ""
	}
	return string(password)
}

func showUsers(c *cli.Context) error {
	db, err := initDB(c)
	if err != nil {
		return err
	}
	db.DumpUsers(c.Bool("long"))
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

func findOrphanFiles(c *cli.Context) error {
	db, err := initDB(c)
	if err != nil {
		return err
	}
	return db.FindOrphanFiles(c.Bool("delete"))
}

func changeMasterKey(c *cli.Context) error {
	log.Level = flagLogLevel
	log.Infof("Working on %s", flagDatabase)

	pp, err := crypto.Passphrase(flagPassphraseCmd, flagPassphraseFile, flagPassphraseEnv)
	if err != nil {
		return err
	}
	mkFile := filepath.Join(flagDatabase, "master.key")
	mk1, err := crypto.ReadMasterKey(pp, mkFile)
	if err != nil {
		return err
	}

	var alg int
	switch f := strings.ToLower(c.String("format")); f {
	case "auto":
		alg = crypto.PickFastest
	case "aes", "aes256":
		alg = crypto.AES256
	case "chacha20poly1305":
		alg = crypto.Chacha20Poly1305
	default:
		log.Fatalf("Invalid format %q", f)
	}

	mk2, err := crypto.CreateMasterKey(alg)
	if err != nil {
		return err
	}

	if ans := prompt("\nMake sure you have a backup of the database before proceeding.\nType CHANGE-MASTER-KEY to continue: "); ans != "CHANGE-MASTER-KEY" {
		log.Fatal("Aborted.")
	}

	if err := mk2.Save(pp, mkFile+".new"); err != nil {
		return err
	}

	context := func(s string) []byte {
		h := sha1.Sum([]byte(s))
		return h[:]
	}

	db := database.New(flagDatabase, pp)

	reEncryptFile := func(path database.DFile) (err error) {
		defer func() {
			if err != nil {
				log.Infof("%s: %v", path, err)
			}
		}()
		rel := path.RelativePath
		newRel := rel
		if path.LogicalPath != "" {
			h := mk2.Hash([]byte(path.LogicalPath))
			dir := fmt.Sprintf("%02X", h[0])
			newRel = filepath.Join(dir, base64.RawURLEncoding.EncodeToString(h))
		}

		oldPath := filepath.Join(db.Dir(), rel)
		in, err := os.Open(oldPath)
		if err != nil {
			return err
		}
		defer in.Close()
		hdr := make([]byte, 5)
		if _, err := io.ReadFull(in, hdr); err != nil {
			log.Infof("%s Skipped", path)
			return nil
		}
		if string(hdr[:4]) != "KRIN" {
			log.Infof("%s Skipped", path)
			return nil
		}
		k1, err := mk1.ReadEncryptedKey(in)
		if err != nil {
			return err
		}
		defer k1.Wipe()
		r, err := k1.StartReader(context(rel), in)
		if err != nil {
			return err
		}
		defer r.Close()

		// Read the header again.
		h := make([]byte, 5)
		if _, err := io.ReadFull(r, h); err != nil {
			return err
		}
		if bytes.Compare(hdr, h) != 0 {
			return errors.New("wrong encrypted header")
		}
		if hdr[4]&0x40 != 0 {
			if err := secure.SkipPadding(r); err != nil {
				return err
			}
		}
		hdr[4] |= 0x40 // padded

		maxPadding := 64 * 1024
		if hdr[4]&0x04 != 0 { // raw bytes (blob)
			maxPadding = 1024 * 1024
		}

		newPath := filepath.Join(db.Dir(), newRel)
		createParent(newPath)
		out, err := os.OpenFile(newPath+".tmp", os.O_WRONLY|os.O_CREATE|os.O_EXCL|os.O_SYNC, 0600)
		if err != nil {
			return err
		}
		if _, err := out.Write(hdr); err != nil {
			out.Close()
			return err
		}

		k2, err := mk2.NewKey()
		if err != nil {
			out.Close()
			return err
		}
		defer k2.Wipe()
		if err := k2.WriteEncryptedKey(out); err != nil {
			out.Close()
			return err
		}
		w, err := k2.StartWriter(context(newRel), out)
		if err != nil {
			out.Close()
			return err
		}
		if _, err := w.Write(hdr); err != nil {
			out.Close()
			return err
		}
		if err := secure.AddPadding(w, maxPadding); err != nil {
			return err
		}
		var buf [4096]byte
		for {
			n, err := r.Read(buf[:])
			if n > 0 {
				if _, err := w.Write(buf[:n]); err != nil {
					w.Close()
					return err
				}
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				w.Close()
				return err
			}
		}
		if err := w.Close(); err != nil {
			return err
		}
		if err := os.Rename(newPath+".tmp", newPath); err != nil {
			return err
		}
		if oldPath != newPath {
			if err := os.Remove(oldPath); err != nil {
				return err
			}
		}
		return nil
	}

	type result struct {
		path string
		err  error
	}

	fileChan := db.FileIterator()
	errChan := make(chan result)
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(in <-chan database.DFile, out chan<- result) {
			defer wg.Done()
			for path := range in {
				err := reEncryptFile(path)
				out <- result{path.RelativePath, err}
			}
		}(fileChan, errChan)
	}

	go func() {
		wg.Wait()
		close(errChan)
	}()

	errCount := 0
	doneCount := 0
	for res := range errChan {
		if res.err != nil {
			errCount++
			log.Infof("%s: %v", res.path, res.err)
			continue
		}
		doneCount++
		if doneCount%100 == 0 {
			log.Infof("%d done", doneCount)
		}
	}
	log.Infof("%d done", doneCount)
	if errCount == 0 {
		log.Info("All files re-encrypted successfully")
	} else {
		log.Infof("Re-encryption error count: %d", errCount)
	}

	if err := os.Rename(mkFile+".new", mkFile); err != nil {
		return err
	}
	db.Wipe()
	db = database.New(flagDatabase, pp)
	defer db.Wipe()
	uids, err := db.UserIDs()
	if err != nil {
		return err
	}
	reEncryptString := func(s string) (string, error) {
		b, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			return "", err
		}
		k, err := mk1.Decrypt(b)
		if err != nil {
			return "", err
		}
		e, err := mk2.Encrypt(k)
		if err != nil {
			return "", err
		}
		return base64.StdEncoding.EncodeToString(e), nil
	}
	for _, uid := range uids {
		u, err := db.UserByID(uid)
		if err != nil {
			return err
		}
		u.ServerSecretKey, err = reEncryptString(u.ServerSecretKey)
		if err != nil {
			return err
		}
		u.TokenKey, err = reEncryptString(u.TokenKey)
		if err != nil {
			return err
		}
		for _, v := range u.Decoys {
			np, err := reEncryptString(v.Password)
			if err != nil {
				return err
			}
			v.Password = np
		}
		if err := db.UpdateUser(u); err != nil {
			return err
		}
		log.Infof("Updated user %d", uid)
	}
	return nil
}

func renameUser(c *cli.Context) error {
	db, err := initDB(c)
	if err != nil {
		return err
	}
	id := c.Int64("userid")
	email := c.String("new-email")
	if id <= 0 || len(email) == 0 {
		return cli.ShowSubcommandHelp(c)
	}
	return db.RenameUser(id, email)
}

func editUserList(c *cli.Context) error {
	db, err := initDB(c)
	if err != nil {
		return err
	}
	return db.EditUserList()
}

func editQuotas(c *cli.Context) error {
	db, err := initDB(c)
	if err != nil {
		return err
	}
	return db.EditQuotas()
}

func editUser(c *cli.Context) error {
	db, err := initDB(c)
	if err != nil {
		return err
	}
	uid := c.Int64("userid")
	if uid <= 0 {
		cli.ShowSubcommandHelp(c)
		return nil
	}
	return db.EditUser(uid)
}

func editAlbums(c *cli.Context) error {
	db, err := initDB(c)
	if err != nil {
		return err
	}
	uid := c.Int64("userid")
	if uid <= 0 {
		cli.ShowSubcommandHelp(c)
		return nil
	}
	return db.EditAlbums(uid)
}

func editContacts(c *cli.Context) error {
	db, err := initDB(c)
	if err != nil {
		return err
	}
	uid := c.Int64("userid")
	if uid <= 0 {
		cli.ShowSubcommandHelp(c)
		return nil
	}
	return db.EditContacts(uid)
}

func editFileset(c *cli.Context) error {
	db, err := initDB(c)
	if err != nil {
		return err
	}
	if c.Args().Len() != 1 {
		cli.ShowSubcommandHelp(c)
		return nil
	}
	return db.EditFileset(c.Args().Get(0))
}

func changeUserOTPKey(c *cli.Context) error {
	db, err := initDB(c)
	if err != nil {
		return err
	}
	defer db.Wipe()
	id := c.Int64("userid")
	if id <= 0 {
		return cli.ShowSubcommandHelp(c)
	}
	user, err := db.UserByID(id)
	if err != nil {
		return err
	}
	changed := false
	if c.Bool("clear") {
		if user.OTPKey == "" {
			log.Infof("User's OTP Key is not set")
		} else {
			user.OTPKey = ""
			changed = true
		}
	}
	if c.Bool("set") {
		if user.OTPKey != "" {
			log.Infof("User's OTP Key is already set")
		} else {
			key, err := totp.Generate(totp.GenerateOpts{
				Issuer:      "c2FmZQ",
				AccountName: user.Email,
			})
			if err != nil {
				return err
			}
			user.OTPKey = key.Secret()
			changed = true
		}
	}
	if changed {
		if err := db.UpdateUser(user); err != nil {
			return err
		}
	}

	if user.OTPKey == "" {
		log.Infof("User's OTP is disabled")
		return nil
	}

	opts := totp.GenerateOpts{
		Issuer:      "c2FmZQ",
		AccountName: user.Email,
	}
	if opts.Secret, err = base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(user.OTPKey); err != nil {
		return err
	}
	key, err := totp.Generate(opts)
	if err != nil {
		return err
	}

	var buf strings.Builder
	qrterminal.GenerateHalfBlock(key.URL(), qrterminal.L, &buf)
	qr := buf.String()
	// The generated QR code is designed for a terminal with a dark
	// background. If the terminal has a light background, it looks
	// awkward (but still works). For convenience, we'll show the
	// QR code in its original version and with reversed colors.
	rev := strings.Map(func(r rune) rune {
		switch r {
		case '█':
			return ' '
		case ' ':
			return '█'
		case '▀':
			return '▄'
		case '▄':
			return '▀'
		default:
			return r
		}
	}, qr)
	// Show QR code with original and reverse color side by side.
	s1, s2 := strings.Split(qr, "\n"), strings.Split(rev, "\n")
	for i := range s1 {
		fmt.Println(s1[i], s2[i])
	}
	fmt.Printf("TOTP KEY: %s\n\n", user.OTPKey)
	return nil
}

func changeUserDecoy(c *cli.Context) error {
	db, err := initDB(c)
	if err != nil {
		return err
	}
	defer db.Wipe()
	id := c.Int64("userid")
	if id <= 0 {
		return cli.ShowSubcommandHelp(c)
	}
	user, err := db.UserByID(id)
	if err != nil {
		return err
	}
	find := func(pass string) (int, *database.Decoy) {
		for i, d := range user.Decoys {
			p, err := db.Decrypt(d.Password)
			if err != nil {
				log.Errorf("Decrypt: %v", err)
				continue
			}
			if subtle.ConstantTimeCompare(p, []byte(pass)) == 1 {
				return i, d
			}
		}
		return 0, nil
	}
	changed := false
	if c.Bool("clear") {
		pass := promptPassword("Enter password to DELETE: ")
		if index, decoy := find(pass); decoy != nil {
			del, err := db.UserByID(decoy.UserID)
			if err != nil {
				return err
			}
			if !del.LoginDisabled {
				return errors.New("active account")
			}
			if err := db.DeleteUser(del); err != nil {
				return err
			}
			user.Decoys = append(user.Decoys[:index], user.Decoys[index+1:]...)
			changed = true
		} else {
			return errors.New("invalid password")
		}
	}
	if c.Bool("change") {
		oldpass := promptPassword("Enter OLD password: ")
		if _, decoy := find(oldpass); decoy != nil {
			newpass := promptPassword("Enter NEW password: ")
			if _, d := find(newpass); d != nil {
				return errors.New("password already exists")
			}
			p, err := db.Encrypt([]byte(newpass))
			if err != nil {
				return err
			}
			decoy.Password = p
			du, err := db.UserByID(decoy.UserID)
			if err != nil {
				return err
			}
			sk, err := stingle.DecodeSecretKeyBundle([]byte(oldpass), du.KeyBundle)
			if err != nil {
				return err
			}
			du.KeyBundle = stingle.MakeSecretKeyBundle([]byte(newpass), sk)
			sk.Wipe()
			etk, err := db.NewEncryptedTokenKey()
			if err != nil {
				return err
			}
			du.TokenKey = etk
			if err := db.UpdateUser(du); err != nil {
				return err
			}
			changed = true
		} else {
			return errors.New("invalid password")
		}
	}
	if c.Bool("set") {
		pass := promptPassword("Enter NEW password: ")
		if _, decoy := find(pass); decoy != nil {
			return errors.New("password already exists")
		}
		sk := stingle.MakeSecretKey()
		pk := sk.PublicKey()
		bundle := stingle.MakeSecretKeyBundle([]byte(pass), sk)
		sk.Wipe()
		uid, err := db.AddUser(
			database.User{
				LoginDisabled: true,
				Email:         fmt.Sprintf("!%x", sha1.Sum(pk.ToBytes())),
				KeyBundle:     bundle,
				IsBackup:      "1",
				PublicKey:     pk,
			})
		if err != nil {
			return err
		}
		du, err := db.UserByID(uid)
		if err != nil {
			return err
		}
		du.Email = user.Email
		if err := db.UpdateUser(du); err != nil {
			return err
		}
		epass, err := db.Encrypt([]byte(pass))
		if err != nil {
			return err
		}
		user.Decoys = append(user.Decoys, &database.Decoy{
			UserID:   uid,
			Password: epass,
		})
		changed = true
	}
	if changed {
		if err := db.UpdateUser(user); err != nil {
			return err
		}
	}

	switch n := len(user.Decoys); n {
	case 0:
		fmt.Printf("User %d has no decoy passwords.\n", id)
	case 1:
		fmt.Printf("User %d has 1 decoy password.\n", id)
	default:
		fmt.Printf("User %d has %d decoy passwords.\n", id, n)
	}
	if c.Bool("show") {
		for _, d := range user.Decoys {
			p, _ := db.Decrypt(d.Password)
			fmt.Printf("  %s (%s): %d\n", d.Password, p, d.UserID)
		}
	}
	return nil
}

func changePassphrase(c *cli.Context) error {
	log.Level = flagLogLevel

	if !flagEncryptMetadata {
		return errors.New("-encrypt-metadata must be true")
	}
	mkFile := filepath.Join(flagDatabase, "master.key")

	oldPass, err := crypto.Passphrase(flagPassphraseCmd, flagPassphraseFile, flagPassphraseEnv)
	if err != nil {
		return err
	}
	mk, err := crypto.ReadMasterKey(oldPass, mkFile)
	if err != nil {
		return err
	}
	defer mk.Wipe()

	newPass, err := crypto.NewPassphrase(c.String("new-passphrase-command"), c.String("new-passphrase-file"), c.String("new-passphrase-env"))
	if err != nil {
		return err
	}
	if err := mk.Save(newPass, mkFile+".new"); err != nil {
		return err
	}
	if err := os.Rename(mkFile+".new", mkFile); err != nil {
		return err
	}
	log.Infof("Passphrase changed successfully [%s].", mkFile)
	return nil
}
