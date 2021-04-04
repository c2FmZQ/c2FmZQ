// Package database implements all the storage requirement of the kringle server
// using a local filesystem. It doesn't use any external database server.
package database

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"kringle-server/crypto"
	"kringle-server/log"
	"kringle-server/md"
)

var (
	// Set this only for tests.
	CurrentTimeForTesting int64 = 0
)

// New returns an initialized database that uses dir for storage.
func New(dir, passphrase string) *Database {
	db := &Database{dir: dir}
	mkFile := filepath.Join(dir, "master.key")
	var err error
	if db.masterKey, err = crypto.ReadMasterKey(passphrase, mkFile); errors.Is(err, os.ErrNotExist) {
		if db.masterKey, err = crypto.CreateMasterKey(); err != nil {
			log.Fatal("Failed to create master key")
		}
		err = db.masterKey.Save(passphrase, mkFile)
	}
	if err != nil {
		log.Fatal("Failed to decrypt master key")
	}
	db.md = md.New(dir, db.masterKey.EncryptionKey)
	return db
}

// Implements all the storage requirements of the kringle server using a local
// filesystem.
type Database struct {
	dir       string
	masterKey *crypto.MasterKey
	md        *md.Metadata
}

// Dir returns the directory where the database stores its data.
func (d Database) Dir() string {
	return d.dir
}

func (d *Database) filePath(elems ...string) string {
	name := d.masterKey.Hash([]byte(path.Join(elems...)))
	dir := filepath.Join("metadata", fmt.Sprintf("%02X", name[0]), fmt.Sprintf("%02X", name[1]))
	return filepath.Join(dir, base64.RawURLEncoding.EncodeToString(name))
}

// nowInMS returns the current time in ms.
func nowInMS() int64 {
	if CurrentTimeForTesting != 0 {
		return CurrentTimeForTesting
	}
	return time.Now().UnixNano() / 1000000 // ms
}

// boolToNumber converts a bool to json.Number "0" or "1".
func boolToNumber(b bool) json.Number {
	if b {
		return json.Number("1")
	}
	return json.Number("0")
}

func number(n int64) json.Number {
	return json.Number(fmt.Sprintf("%d", n))
}

func createParentIfNotExist(filename string) error {
	dir, _ := filepath.Split(filename)
	if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("os.MkdirAll(%q): %w", dir, err)
		}
	}
	return nil
}

func showCallStack() {
	pc := make([]uintptr, 10)
	n := runtime.Callers(2, pc)
	if n == 0 {
		return
	}
	frames := runtime.CallersFrames(pc[:n])

	log.Debug("Call Stack")
	for {
		frame, more := frames.Next()
		if !strings.Contains(frame.File, "kringle-server") {
			break
		}
		fl := fmt.Sprintf("%s:%d", filepath.Base(frame.File), frame.Line)
		log.Debugf("   %-15s %s", fl, frame.Function)
		if !more {
			break
		}
	}
}

func (d Database) DumpFile(filename string) error {
	return d.md.DumpFile(filename)
}

func (d Database) DumpUsers() {
	var ul []userList
	if _, err := d.md.ReadDataFile(d.filePath(userListFile), &ul); err != nil {
		log.Errorf("ReadDataFile: %v", err)
	}
	for _, u := range ul {
		user, err := d.User(u.Email)
		if err != nil {
			log.Errorf("User(%q): %v", u.Email, err)
		}
		fmt.Printf("ID %d [%s]: %s\n", u.UserID, u.Email, d.filePath("home", u.Email, userFile))
		fmt.Printf("  -contacts: %s\n", d.filePath("home", user.Email, contactListFile))
		fmt.Printf("  -trash: %s\n", d.fileSetPath(user, TrashSet))
		fmt.Printf("  -gallery: %s\n", d.fileSetPath(user, GallerySet))
		albums, err := d.AlbumRefs(user)
		if err != nil {
			log.Errorf("AlbumRefs(%q): %v", u.Email, err)
		}
		for k, v := range albums {
			fmt.Printf("  -album %s: %s\n", k, v.File)
		}
	}
}
