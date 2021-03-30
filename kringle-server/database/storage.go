package database

import (
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"kringle-server/log"
)

var (
	errRolledBack        = errors.New("rolled back")
	errAlreadyRolledBack = errors.New("already rolled back")
	errAlreadyCommitted  = errors.New("already committed")
)

// lock atomically creates a lock file for the given filename. When this
// function returns without error, the lock is acquired and nobody else can
// acquire it until it is released.
//
// There is logic in place to remove stale locks after a while.
func lock(fn string) error {
	if err := createParentIfNotExist(fn); err != nil {
		return err
	}
	deadline := time.Duration(60+rand.Int()%60) * time.Second
	lockf := fn + ".lock"
	for {
		f, err := os.OpenFile(lockf, os.O_WRONLY|os.O_CREATE|os.O_EXCL|os.O_SYNC, 0600)
		if errors.Is(err, os.ErrExist) {
			tryToRemoveStaleLock(lockf, deadline)
			time.Sleep(time.Duration(50+rand.Int()%100) * time.Millisecond)
			continue
		}
		if err != nil {
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
		return nil
	}
}

// unlock released the lock file for the given filename.
func unlock(fn string) error {
	lockf := fn + ".lock"
	if err := os.Remove(lockf); err != nil {
		return err
	}
	return nil
}

func tryToRemoveStaleLock(lockf string, deadline time.Duration) {
	fi, err := os.Stat(lockf)
	if err != nil {
		return
	}
	if time.Since(fi.ModTime()) > deadline {
		if err := os.Remove(lockf); err == nil {
			log.Errorf("Removed stale lock %q", lockf)
		}
	}
}

// d.openForUpdate opens a json file with the expectation that the object will be
// modified and then saved again.
//
// Example:
//   func foo() (retErr error) {
//     var foo FooStruct
//     commit, err := d.openForUpdate(filename, &foo)
//     if err != nil {
//       panic(err)
//     }
//     defer commit(false, &retErr) // rollback unless first committed.
//     // modified foo
//     foo.Bar = X
//     return commit(true, &retError) // commit
//  }
func (d *Database) openForUpdate(f string, obj interface{}) (func(bool, *error) error, error) {
	if err := lock(f); err != nil {
		return nil, err
	}
	crypter, err := d.readDataFile(f, obj)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	var called, committed bool
	return func(commit bool, errp *error) (err error) {
		if called {
			if committed {
				return errAlreadyCommitted
			}
			return errAlreadyRolledBack
		}
		called = true
		if errp == nil || *errp != nil {
			errp = &err
		}
		if commit {
			if *errp = d.saveDataFile(crypter, f, obj); *errp != nil {
				return *errp
			}
			committed = true
		}
		*errp = unlock(f)
		if !commit && *errp == nil {
			*errp = errRolledBack
		}
		return *errp
	}, nil
}

// DumpFile writes the decrypted content of a file to stdout.
func (d *Database) DumpFile(filename string) error {
	f, err := os.Open(filepath.Join(d.Dir(), filename))
	if err != nil {
		return err
	}
	defer f.Close()
	c := newCrypter(d.decryptWithMasterKey)
	r, err := c.BeginRead(f)
	if err != nil {
		return err
	}
	gz, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer gz.Close()
	_, err = io.Copy(os.Stdout, gz)
	return err
}

// readDataFile reads a json object from a file.
func (d *Database) readDataFile(filename string, obj interface{}) (*crypter, error) {
	f, err := os.Open(filepath.Join(d.Dir(), filename))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	c := newCrypter(d.decryptWithMasterKey)
	r, err := c.BeginRead(f)
	if err != nil {
		return nil, err
	}
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	if err := json.NewDecoder(gz).Decode(obj); err != nil {
		return nil, err
	}
	return c, nil
}

// saveDataFile atomically replace a json object in a file.
func (d *Database) saveDataFile(c *crypter, filename string, obj interface{}) error {
	filename = filepath.Join(d.Dir(), filename)
	if c == nil {
		c = newCrypter(d.decryptWithMasterKey)
		if err := createParentIfNotExist(filename); err != nil {
			return err
		}
	}
	t := fmt.Sprintf("%s.tmp-%d", filename, time.Now().UnixNano())
	f, err := os.OpenFile(t, os.O_WRONLY|os.O_CREATE|os.O_EXCL|os.O_SYNC, 0600)
	if err != nil {
		return err
	}
	w, err := c.BeginWrite(f)
	if err != nil {
		f.Close()
		return err
	}
	gz, err := gzip.NewWriterLevel(w, gzip.BestCompression)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(gz)
	enc.SetIndent("", "  ")
	if err := enc.Encode(obj); err != nil {
		gz.Close()
		w.Close()
		return err
	}
	if err := gz.Close(); err != nil {
		w.Close()
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return os.Rename(t, filename)
}
