// Package database implements all the storage requirement of the c2FmZQ
// server.
package database

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"c2FmZQ/internal/crypto"
	"c2FmZQ/internal/log"
	"c2FmZQ/internal/secure"
	"c2FmZQ/internal/stingle"
)

var (
	// Set this only for tests.
	CurrentTimeForTesting int64 = 0

	funcLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "database_response_time",
			Help:    "The database's response time",
			Buckets: []float64{0.01, 0.05, 0.1, 0.2, 0.3, 0.4, 0.5, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 20, 30, 45, 60, 90, 120},
		},
		[]string{"func"},
	)
)

func init() {
	prometheus.MustRegister(funcLatency)

}

func recordLatency(name string) func() time.Duration {
	timer := prometheus.NewTimer(funcLatency.WithLabelValues(name))
	return timer.ObserveDuration
}

// New returns an initialized database that uses dir for storage.
func New(dir string, passphrase []byte) *Database {
	db := &Database{dir: dir}
	mkFile := filepath.Join(dir, "master.key")
	if len(passphrase) > 0 {
		if _, err := os.Stat(filepath.Join(dir, "metadata", "users.dat")); err == nil {
			log.Fatal("Passphrase is set, but metadata/users.dat exists.")
		}
		var err error
		if db.masterKey, err = crypto.ReadMasterKey(passphrase, mkFile); errors.Is(err, os.ErrNotExist) {
			if db.masterKey, err = crypto.CreateMasterKey(); err != nil {
				log.Fatal("Failed to create master key")
			}
			err = db.masterKey.Save(passphrase, mkFile)
		}
		if err != nil {
			log.Fatalf("Failed to decrypt master key: %v", err)
		}
		db.storage = secure.NewStorage(dir, &db.masterKey.EncryptionKey)
	} else {
		if _, err := os.Stat(mkFile); err == nil {
			log.Fatal("Passphrase is empty, but master.key exists.")
		}
		db.storage = secure.NewStorage(dir, nil)
	}

	// Fail silently if it already exists.
	db.storage.CreateEmptyFile(db.filePath(userListFile), []userList{})
	db.CreateEmptyQuotaFile()
	return db
}

// Database implements all the storage requirements of the c2FmZQ server using
// encrypted storage on a local filesystem.
type Database struct {
	dir       string
	masterKey *crypto.MasterKey
	storage   *secure.Storage
}

// Dir returns the directory where the database stores its data.
func (d Database) Dir() string {
	return d.dir
}

func (d *Database) Hash(in []byte) []byte {
	if d.masterKey != nil {
		return d.masterKey.Hash(in)
	}
	h := sha256.Sum256(in)
	return h[:]
}

// filePath returns a cryptographically secure hash of a logical file name.
func (d *Database) filePath(elems ...string) string {
	if d.masterKey != nil {
		name := d.masterKey.Hash([]byte(path.Join(elems...)))
		dir := filepath.Join("metadata", fmt.Sprintf("%02X", name[0]))
		return filepath.Join(dir, base64.RawURLEncoding.EncodeToString(name))
	}
	return filepath.Join("metadata", filepath.Join(elems...))
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

// number converts an integer to a json.Number.
func number(n int64) json.Number {
	return json.Number(fmt.Sprintf("%d", n))
}

// createParentIfNotExist creates filename's parent directory if it doesn't
// already exist.
func createParentIfNotExist(filename string) error {
	dir, _ := filepath.Split(filename)
	if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("os.MkdirAll(%q): %w", dir, err)
		}
	}
	return nil
}

// showCallStack logs the current call stack, for debugging.
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
		if !strings.Contains(frame.File, "c2FmZQ-server") {
			break
		}
		fl := fmt.Sprintf("%s:%d", filepath.Base(frame.File), frame.Line)
		log.Debugf("   %-15s %s", fl, frame.Function)
		if !more {
			break
		}
	}
}

// DumpFile shows the content of a file to stdout.
func (d Database) DumpFile(filename string) error {
	var (
		user          User
		blob          BlobSpec
		userList      []userList
		albumManifest AlbumManifest
		contactList   ContactList
		fileSet       FileSet
	)

	out := func(obj interface{}) error {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(obj)
	}

	if err := d.storage.ReadDataFile(filename, &user); err == nil && user.UserID != 0 {
		return out(user)
	}
	if err := d.storage.ReadDataFile(filename, &blob); err == nil && blob.RefCount > 0 {
		return out(blob)
	}
	if err := d.storage.ReadDataFile(filename, &userList); err == nil && userList != nil {
		return out(userList)
	}
	if err := d.storage.ReadDataFile(filename, &albumManifest); err == nil && albumManifest.Albums != nil {
		return out(albumManifest)
	}
	if err := d.storage.ReadDataFile(filename, &contactList); err == nil && contactList.Contacts != nil {
		return out(contactList)
	}
	if err := d.storage.ReadDataFile(filename, &fileSet); err == nil && fileSet.Files != nil {
		return out(fileSet)
	} else {
		return err
	}
}

// DumpUsers shows information about all the users to stdout.
func (d Database) DumpUsers() {
	var ul []userList
	if err := d.storage.ReadDataFile(d.filePath(userListFile), &ul); err != nil {
		log.Errorf("ReadDataFile: %v", err)
	}
	for _, u := range ul {
		user, err := d.UserByID(u.UserID)
		if err != nil {
			log.Errorf("User(%q): %v", u.Email, err)
			continue
		}
		fmt.Printf("ID %d [%s]: %s\n", u.UserID, u.Email, d.filePath(user.home(userFile)))
		fmt.Printf("  -contacts: %s\n", d.filePath(user.home(contactListFile)))
		fmt.Printf("  -trash: %s\n", d.fileSetPath(user, stingle.TrashSet))
		fmt.Printf("  -gallery: %s\n", d.fileSetPath(user, stingle.GallerySet))
		albums, err := d.AlbumRefs(user)
		if err != nil {
			log.Errorf("AlbumRefs(%q): %v", u.Email, err)
			continue
		}
		for k, v := range albums {
			fmt.Printf("  -album %s: %s\n", k, v.File)
		}
	}
}

func (d Database) FindOrphanFiles(del bool) error {
	exist := make(map[string]struct{})
	err := filepath.WalkDir(d.Dir(), func(path string, de fs.DirEntry, err error) error {
		if err != nil {
			log.Errorf("%s: %s", path, err)
			return err
		}
		if de.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(d.Dir(), path)
		exist[rel] = struct{}{}
		return nil
	})
	if err != nil {
		return err
	}
	delete(exist, "master.key")
	delete(exist, d.filePath(userListFile))
	delete(exist, d.filePath(quotaFile))

	var ul []userList
	if err := d.storage.ReadDataFile(d.filePath(userListFile), &ul); err != nil {
		log.Errorf("ReadDataFile: %v", err)
		return err
	}
	for _, u := range ul {
		user, err := d.UserByID(u.UserID)
		if err != nil {
			log.Errorf("User(%q): %v", u.Email, err)
			return err
		}
		delete(exist, d.filePath(user.home(userFile)))
		delete(exist, d.filePath(user.home(contactListFile)))
		delete(exist, d.filePath(user.home(albumManifest)))
		delete(exist, d.fileSetPath(user, stingle.TrashSet))
		delete(exist, d.fileSetPath(user, stingle.GallerySet))
		albums, err := d.AlbumRefs(user)
		if err != nil {
			log.Errorf("AlbumRefs(%q): %v", u.Email, err)
			return err
		}
		fsList := []string{
			d.fileSetPath(user, stingle.TrashSet),
			d.fileSetPath(user, stingle.GallerySet),
		}
		for _, v := range albums {
			delete(exist, v.File)
			fsList = append(fsList, v.File)
		}
		for _, f := range fsList {
			var fs FileSet
			if err := d.storage.ReadDataFile(f, &fs); err != nil {
				log.Errorf("FileSet: %v", err)
				return err
			}
			for _, file := range fs.Files {
				delete(exist, file.StoreFile)
				delete(exist, file.StoreThumb)
				delete(exist, file.StoreFile+".ref")
				delete(exist, file.StoreThumb+".ref")
			}
		}
	}
	var sorted []string
	for e := range exist {
		sorted = append(sorted, e)
	}
	sort.Strings(sorted)
	for _, e := range sorted {
		if del {
			log.Infof("Deleting orphan file: %s", e)
			if err := os.Remove(filepath.Join(d.Dir(), e)); err != nil {
				return err
			}
			continue
		}
		log.Infof("Orphan file: %s", e)
	}
	return nil
}
