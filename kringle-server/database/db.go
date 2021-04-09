// Package database implements all the storage requirement of the kringle
// server.
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

	"github.com/prometheus/client_golang/prometheus"

	"kringle-server/crypto"
	"kringle-server/log"
	"kringle-server/secure"
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
		log.Fatalf("Failed to decrypt master key: %v", err)
	}
	db.storage = secure.NewStorage(dir, db.masterKey.EncryptionKey)
	// Fail silently if it already exists.
	db.storage.CreateEmptyFile(db.filePath(userListFile))
	return db
}

// Database implements all the storage requirements of the kringle server using
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

// filePath returns a cryptographically secure hash of a logical file name.
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

// DumpFile shows the content of a file to stdout.
func (d Database) DumpFile(filename string) error {
	return d.storage.DumpFile(filename)
}

// DumpUsers shows information about all the users to stdout.
func (d Database) DumpUsers() {
	var ul []userList
	if _, err := d.storage.ReadDataFile(d.filePath(userListFile), &ul); err != nil {
		log.Errorf("ReadDataFile: %v", err)
	}
	for _, u := range ul {
		user, err := d.UserByID(u.UserID)
		if err != nil {
			log.Errorf("User(%q): %v", u.Email, err)
		}
		fmt.Printf("ID %d [%s]: %s\n", u.UserID, u.Email, d.filePath(user.home(userFile)))
		fmt.Printf("  -contacts: %s\n", d.filePath(user.home(contactListFile)))
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
