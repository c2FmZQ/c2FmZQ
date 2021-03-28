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

	"kringle-server/log"
)

var (
	// Set this only for tests.
	CurrentTimeForTesting int64 = 0
)

// New returns an initialized database that uses dir for storage.
func New(dir, passphrase string) *Database {
	db := &Database{dir: dir}
	var err error
	if db.masterKey, err = db.readMasterKey(passphrase); errors.Is(err, os.ErrNotExist) {
		db.masterKey, err = db.createMasterKey(passphrase)
	}
	if err != nil {
		log.Fatal("Failed to decrypt master key")
	}
	return db
}

// Implements all the storage requirements of the kringle server using a local
// filesystem.
type Database struct {
	dir       string
	masterKey []byte
}

// Dir returns the directory where the database stores its data.
func (d Database) Dir() string {
	return d.dir
}

func (d *Database) filePath(elems ...string) string {
	name := d.masterHash([]byte(path.Join(elems...)))
	dir := filepath.Join(d.Dir(), "metadata", fmt.Sprintf("%02X", name[0]), fmt.Sprintf("%02X", name[1]))
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
