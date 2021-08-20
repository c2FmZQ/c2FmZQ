package main

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"github.com/urfave/cli/v2" // cli
	"golang.org/x/term"

	"c2FmZQ/internal/crypto"
	"c2FmZQ/internal/database"
	"c2FmZQ/internal/log"
	"c2FmZQ/internal/stingle"
)

var (
	flagDatabase        string
	flagLogLevel        int
	flagEncryptMetadata bool
	flagPassphraseFile  string
	flagPassphraseCmd   string
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
			&cli.Command{
				Name:    "quotas",
				Aliases: []string{"quota"},
				Usage:   "Edit quotas.",
				Action:  editQuotas,
			},
			&cli.Command{
				Name:   "orphans",
				Usage:  "Find orphans files.",
				Action: findOrphanFiles,
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "delete",
						Usage: "Delete the orphan files.",
					},
				},
			},
			&cli.Command{
				Name:   "convert-aes-chacha20poly1305",
				Usage:  "Convert between AES and Chacha20Poly1305 encryption.",
				Action: convertAESChacha20Poly1305,
			},
			&cli.Command{
				Name:   "rename-user",
				Usage:  "Change the email address of a user.",
				Action: renameUser,
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
		if pp, err = crypto.Passphrase(flagPassphraseCmd, flagPassphraseFile); err != nil {
			return nil, err
		}
	}
	return database.New(flagDatabase, pp), nil
}

func prompt(msg string) string {
	fmt.Print(msg)
	reply, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	return strings.TrimSpace(reply)
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
	return sk, nil
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
		hdrs, err := stingle.DecryptBase64Headers(h, sk)
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
		hdr, err := stingle.DecryptHeader(in, sk)
		if err != nil {
			return err
		}
		defer hdr.Wipe()
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

func editQuotas(c *cli.Context) error {
	db, err := initDB(c)
	if err != nil {
		return err
	}
	return db.EditQuotas()
}

func findOrphanFiles(c *cli.Context) error {
	db, err := initDB(c)
	if err != nil {
		return err
	}
	return db.FindOrphanFiles(c.Bool("delete"))
}

func convertAESChacha20Poly1305(c *cli.Context) error {
	log.Level = flagLogLevel
	pp, err := crypto.Passphrase(flagPassphraseCmd, flagPassphraseFile)
	if err != nil {
		return err
	}
	mkFile := filepath.Join(flagDatabase, "master.key")
	mk1, err := crypto.ReadMasterKey(pp, mkFile)
	if err != nil {
		return err
	}
	var mk2 crypto.MasterKey
	switch mk := mk1.(type) {
	case *crypto.AESMasterKey:
		log.Infof("MasterKey is AES. Converting to Chacha20Poly1305.")
		mk2 = &crypto.Chacha20Poly1305MasterKey{
			Chacha20Poly1305Key: (*crypto.Chacha20Poly1305Key)(unsafe.Pointer(mk.AESKey)),
		}
	case *crypto.Chacha20Poly1305MasterKey:
		log.Infof("MasterKey is Chacha20Poly1305. Converting to AES.")
		mk2 = &crypto.AESMasterKey{
			AESKey: (*crypto.AESKey)(unsafe.Pointer(mk.Chacha20Poly1305Key)),
		}
	default:
		log.Fatalf("MasterKey is %T", mk1)
	}
	if ans := prompt("\nMake sure you have a backup of the database before proceeding.\nType CONVERT to continue: "); ans != "CONVERT" {
		log.Fatal("Aborted.")
	}

	if err := mk2.Save(pp, mkFile+".new"); err != nil {
		return err
	}

	context := func(s string) []byte {
		h := sha1.Sum([]byte(s))
		return h[:]
	}

	if err := filepath.WalkDir(flagDatabase, func(path string, d fs.DirEntry, err error) (retErr error) {
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(flagDatabase, path)
		if err != nil {
			return err
		}
		defer func() {
			log.Infof("%s, err=%v", rel, retErr)
		}()
		ctx := context(rel)

		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		hdr := make([]byte, 5)
		if _, err := io.ReadFull(in, hdr); err != nil {
			return nil
		}
		if string(hdr[:4]) != "KRIN" {
			return nil
		}
		k1, err := mk1.ReadEncryptedKey(in)
		if err != nil {
			return err
		}
		defer k1.Wipe()
		r, err := k1.StartReader(ctx, in)
		if err != nil {
			return err
		}
		defer r.Close()

		out, err := os.OpenFile(path+".tmp", os.O_WRONLY|os.O_CREATE|os.O_EXCL|os.O_SYNC, 0600)
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
		w, err := k2.StartWriter(ctx, out)
		if err != nil {
			out.Close()
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
		return os.Rename(path+".tmp", path)

	}); err != nil {
		return err
	}
	if err := os.Rename(mkFile+".new", mkFile); err != nil {
		return err
	}
	db := database.New(flagDatabase, pp)
	uids, err := db.UserIDs()
	if err != nil {
		return err
	}
	reencrypt := func(s string) (string, error) {
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
		u.ServerSecretKey, err = reencrypt(u.ServerSecretKey)
		if err != nil {
			return err
		}
		u.TokenKey, err = reencrypt(u.TokenKey)
		if err != nil {
			return err
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
