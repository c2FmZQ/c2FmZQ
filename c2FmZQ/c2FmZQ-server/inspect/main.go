package main

import (
	"bufio"
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

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
				Name:   "change-passphrase",
				Usage:  "Change the passphrase that protects the database's master key.",
				Action: changePassphrase,
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
				},
			},
			&cli.Command{
				Name:   "change-master-key",
				Usage:  "Re-encrypt all the data with a new master key. This can take a while.",
				Action: changeMasterKey,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "format",
						Value: "auto",
						Usage: "The format of the new master key ('auto', 'aes', 'chacha20poly1305')",
					},
				},
			},
			&cli.Command{
				Name:   "encrypt-blobs",
				Usage:  "Encrypt blobs that were left unencrypted by older code.",
				Action: encryptBlobs,
			},
			&cli.Command{
				Name:   "rename-ref-files",
				Usage:  "Rename old ref files (now deprecated)",
				Action: renameRefFiles,
			},
			&cli.Command{
				Name:   "flatten-data-dir",
				Usage:  "Flatten the blobs and metadata directories under the data dir.",
				Action: flattenDataDir,
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

func changeMasterKey(c *cli.Context) error {
	log.Level = flagLogLevel
	log.Infof("Working on %s", flagDatabase)

	pp, err := crypto.Passphrase(flagPassphraseCmd, flagPassphraseFile)
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
		if err := db.UpdateUser(u); err != nil {
			return err
		}
		log.Infof("Updated user %d", uid)
	}
	return nil
}

func renameRefFiles(c *cli.Context) error {
	log.Level = flagLogLevel
	log.Infof("Working on %s", flagDatabase)

	pp, err := crypto.Passphrase(flagPassphraseCmd, flagPassphraseFile)
	if err != nil {
		return err
	}
	mkFile := filepath.Join(flagDatabase, "master.key")
	mk, err := crypto.ReadMasterKey(pp, mkFile)
	if err != nil {
		return err
	}
	s := secure.NewStorage(flagDatabase, mk)

	reEncryptFile := func(path database.DFile) (err error) {
		defer func() {
			if err != nil {
				log.Infof("%s: %v", path, err)
			}
		}()
		if !strings.HasSuffix(path.RelativePath, ".ref") {
			return nil
		}

		h := mk.Hash([]byte(path.RelativePath))
		dir := filepath.Join("metadata", fmt.Sprintf("%02X", h[0]))
		newRel := filepath.Join(dir, base64.RawURLEncoding.EncodeToString(h))

		var blobSpec database.BlobSpec
		if err := s.ReadDataFile(path.RelativePath, &blobSpec); err != nil {
			return err
		}
		if err := s.SaveDataFile(newRel, &blobSpec); err != nil {
			return err
		}
		return os.Remove(filepath.Join(flagDatabase, path.RelativePath))
	}

	type result struct {
		path string
		err  error
	}

	db := database.New(flagDatabase, pp)
	defer db.Wipe()
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
			log.Infof("%d scanned", doneCount)
		}
	}
	log.Infof("%d scanned", doneCount)
	if errCount == 0 {
		log.Info("All files renamed successfully")
	} else {
		log.Infof("Renaming error count: %d", errCount)
	}

	return nil
}

func flattenDataDir(c *cli.Context) error {
	log.Level = flagLogLevel
	log.Infof("Working on %s", flagDatabase)

	pp, err := crypto.Passphrase(flagPassphraseCmd, flagPassphraseFile)
	if err != nil {
		return err
	}
	mkFile := filepath.Join(flagDatabase, "master.key")
	mk, err := crypto.ReadMasterKey(pp, mkFile)
	if err != nil {
		return err
	}
	s := secure.NewStorage(flagDatabase, mk)

	isBlob := func(file string) bool {
		in, err := os.Open(filepath.Join(flagDatabase, file))
		if err != nil {
			return false
		}
		defer in.Close()
		hdr := make([]byte, 5)
		if _, err := io.ReadFull(in, hdr); err != nil {
			log.Infof("%s bad header", file)
			return false
		}
		if string(hdr[:4]) != "KRIN" {
			log.Infof("%s bad header", file)
			return false
		}
		return (hdr[4] & 0x04) != 0
	}

	context := func(s string) []byte {
		h := sha1.Sum([]byte(s))
		return h[:]
	}

	moveFile := func(path database.DFile) (err error) {
		defer func() {
			if err != nil {
				log.Infof("%s: %v", path, err)
			}
		}()

		var newRel string
		if path.LogicalPath == "" {
			newRel = strings.TrimPrefix(path.RelativePath, "metadata/")
			newRel = strings.TrimPrefix(newRel, "blobs/")
		} else {
			p := strings.TrimPrefix(path.LogicalPath, "blobs/")
			h := mk.Hash([]byte(p))
			dir := fmt.Sprintf("%02X", h[0])
			newRel = filepath.Join(dir, base64.RawURLEncoding.EncodeToString(h))
		}
		if path.RelativePath == newRel {
			return nil
		}

		var typ string

		if strings.HasSuffix(path.LogicalPath, "album-manifest") {
			typ = "manifest"
		} else if (path.LogicalPath == "" && !isBlob(path.RelativePath)) ||
			strings.HasSuffix(path.LogicalPath, "/fileset-0") ||
			strings.HasSuffix(path.LogicalPath, "/fileset-1") {
			typ = "fileset"
		}

		switch typ {
		case "manifest":
			// Rewrite AlbumRef.File
			var m database.AlbumManifest
			if err := s.ReadDataFile(path.RelativePath, &m); err != nil {
				return err
			}
			for _, v := range m.Albums {
				v.File = strings.TrimPrefix(v.File, "metadata/")
			}

			if err := s.SaveDataFile(newRel, &m); err != nil {
				return err
			}
		case "fileset":
			// Rewrite FileSpec.StoreFile & FileSpec.StoreThumb
			var fs database.FileSet
			if err := s.ReadDataFile(path.RelativePath, &fs); err != nil {
				return err
			}
			for _, v := range fs.Files {
				v.StoreFile = strings.TrimPrefix(v.StoreFile, "blobs/")
				v.StoreThumb = strings.TrimPrefix(v.StoreThumb, "blobs/")
			}

			if err := s.SaveDataFile(newRel, &fs); err != nil {
				return err
			}
		default:
			// Copy content as is.
			in, err := os.Open(filepath.Join(flagDatabase, path.RelativePath))
			if err != nil {
				return err
			}
			defer in.Close()
			hdr := make([]byte, 5)
			if _, err := io.ReadFull(in, hdr); err != nil {
				return fmt.Errorf("%s: bad header", path.RelativePath)
			}
			if string(hdr[:4]) != "KRIN" {
				return fmt.Errorf("%s: bad header", path.RelativePath)
			}
			k1, err := mk.ReadEncryptedKey(in)
			if err != nil {
				return err
			}
			defer k1.Wipe()
			r, err := k1.StartReader(context(path.RelativePath), in)
			if err != nil {
				return err
			}
			defer r.Close()

			newPath := filepath.Join(flagDatabase, newRel)
			createParent(newPath)
			out, err := os.OpenFile(newPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL|os.O_SYNC, 0600)
			if err != nil {
				return err
			}
			if _, err := out.Write(hdr); err != nil {
				out.Close()
				return err
			}

			k2, err := mk.NewKey()
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
			if _, err := io.Copy(w, r); err != nil {
				return err
			}
			if err := w.Close(); err != nil {
				return err
			}
		}

		return os.Remove(filepath.Join(flagDatabase, path.RelativePath))
	}

	type result struct {
		path string
		err  error
	}

	db := database.New(flagDatabase, pp)
	defer db.Wipe()
	fileChan := db.FileIterator()
	errChan := make(chan result)
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(in <-chan database.DFile, out chan<- result) {
			defer wg.Done()
			for path := range in {
				err := moveFile(path)
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
			log.Infof("%d scanned", doneCount)
		}
	}
	for i := 0; i <= 255; i++ {
		hex := fmt.Sprintf("%02X", i)
		if err := os.Remove(filepath.Join(flagDatabase, "blobs", hex)); err != nil && !errors.Is(err, os.ErrNotExist) {
			log.Errorf("blobs/%s: %v", hex, err)
		}
		if err := os.Remove(filepath.Join(flagDatabase, "metadata", hex)); err != nil && !errors.Is(err, os.ErrNotExist) {
			log.Errorf("metadata/%s: %v", hex, err)
		}
	}
	if err := os.Remove(filepath.Join(flagDatabase, "blobs")); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Errorf("blobs: %v", err)
	}
	if err := os.Remove(filepath.Join(flagDatabase, "metadata")); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Errorf("metadata: %v", err)
	}

	log.Infof("%d scanned", doneCount)
	if errCount == 0 {
		log.Info("All files renamed successfully")
	} else {
		log.Infof("Renaming error count: %d", errCount)
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

func encryptBlobs(c *cli.Context) error {
	log.Level = flagLogLevel
	pp, err := crypto.Passphrase(flagPassphraseCmd, flagPassphraseFile)
	if err != nil {
		return err
	}
	mk, err := crypto.ReadMasterKey(pp, filepath.Join(flagDatabase, "master.key"))
	if err != nil {
		return err
	}
	s := secure.NewStorage(flagDatabase, mk)

	if ans := prompt("\nMake sure you have a backup of the database before proceeding.\nType ENCRYPT to continue: "); ans != "ENCRYPT" {
		log.Fatal("Aborted.")
	}

	if err := filepath.WalkDir(filepath.Join(flagDatabase, "blobs"), func(path string, d fs.DirEntry, err error) (retErr error) {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(flagDatabase, path)
		if err != nil {
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		hdr := make([]byte, 5)
		if _, err := io.ReadFull(in, hdr); err == nil && string(hdr[:4]) == "KRIN" {
			return nil
		}
		log.Infof("Encrypting %s", rel)
		if _, err := in.Seek(0, io.SeekStart); err != nil {
			return err
		}
		w, err := s.OpenBlobWrite(rel+".encrypted", rel)
		if err != nil {
			return err
		}
		n, err := io.Copy(w, in)
		if err != nil {
			return err
		}
		if err := w.Close(); err != nil {
			return err
		}
		log.Infof("Encrypted %d bytes", n)

		return os.Rename(path+".encrypted", path)
	}); err != nil {
		return err
	}

	return nil
}

func changePassphrase(c *cli.Context) error {
	log.Level = flagLogLevel

	if !flagEncryptMetadata {
		return errors.New("-encrypt-metadata must be true")
	}
	mkFile := filepath.Join(flagDatabase, "master.key")

	oldPass, err := crypto.Passphrase(flagPassphraseCmd, flagPassphraseFile)
	if err != nil {
		return err
	}
	mk, err := crypto.ReadMasterKey(oldPass, mkFile)
	if err != nil {
		return err
	}
	defer mk.Wipe()

	newPass, err := crypto.NewPassphrase(c.String("new-passphrase-command"), c.String("new-passphrase-file"))
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
