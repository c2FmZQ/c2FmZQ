package database

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"time"

	"stingle-server/log"
)

func number(n int64) json.Number {
	return json.Number(fmt.Sprintf("%d", n))
}

// lock atomically creates a lock file for the given filename. When this
// function returns without error, the lock is acquired and nobody else can
// acquire it until it is released.
//
// There is logic in place to remove stale locks after a while.
func lock(fn string) error {
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

// openForUpdate opens a json file with the expectation that the object will be
// modified and then saved again.
//
// Example:
//   var foo FooStruct
//   done, err := openForUpdate(filename, &foo)
//   if err != nil {
//     panic(err)
//   }
//   // modified foo
//   foo.Bar = X
//   return done()
func openForUpdate(f string, obj interface{}) (func(*error) error, error) {
	if err := lock(f); err != nil {
		return nil, err
	}
	if err := loadJSON(f, obj); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return func(errp *error) (err error) {
		if errp == nil || *errp != nil {
			errp = &err
		}
		if *errp = saveJSON(f, obj); *errp != nil {
			return *errp
		}
		*errp = unlock(f)
		return *errp
	}, nil
}

// loadJSON reads a json object from a file.
func loadJSON(flename string, obj interface{}) error {
	f, err := os.Open(flename)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewDecoder(f).Decode(obj)
}

// saveJSON atomically replace a json object in a file.
func saveJSON(filename string, obj interface{}) error {
	t := fmt.Sprintf("%s.tmp-%d", filename, time.Now().UnixNano())
	f, err := os.OpenFile(t, os.O_WRONLY|os.O_CREATE|os.O_EXCL|os.O_SYNC, 0600)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(obj); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(t, filename)
}
