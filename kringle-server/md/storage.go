package md

import (
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"time"

	"kringle-server/log"
)

var (
	// Indicates that the update was successfully rolled back.
	ErrRolledBack = errors.New("rolled back")
	// Indicates that the update was already rolled back by a previous call.
	ErrAlreadyRolledBack = errors.New("already rolled back")
	// Indicates that the update was already committed by a previous call.
	ErrAlreadyCommitted = errors.New("already committed")
)

// Lock atomically creates a lock file for the given filename. When this
// function returns without error, the lock is acquired and nobody else can
// acquire it until it is released.
//
// There is logic in place to remove stale locks after a while.
func (md *Metadata) Lock(fn string) error {
	lockf := filepath.Join(md.dir, fn) + ".lock"
	if err := createParentIfNotExist(lockf); err != nil {
		return err
	}
	deadline := time.Duration(600+rand.Int()%60) * time.Second
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

// LockMany locks multiple files such that if the exact same files are locked
// concurrently, there won't be any deadlock.
//
// When the function returns successfully, all the files are locked.
func (md *Metadata) LockMany(filenames []string) error {
	sorted := make([]string, len(filenames))
	copy(sorted, filenames)
	sort.Strings(sorted)
	var locks []string
	for _, f := range sorted {
		if err := md.Lock(f); err != nil {
			md.UnlockMany(locks)
			return err
		}
		locks = append(locks, f)
	}
	return nil
}

// Unlock released the lock file for the given filename.
func (md *Metadata) Unlock(fn string) error {
	lockf := filepath.Join(md.dir, fn) + ".lock"
	if err := os.Remove(lockf); err != nil {
		return err
	}
	return nil
}

// UnlockMany unlocks multiples files locked by LockMany().
func (md *Metadata) UnlockMany(filenames []string) error {
	sorted := make([]string, len(filenames))
	copy(sorted, filenames)
	sort.Sort(sort.Reverse(sort.StringSlice(filenames)))
	for _, f := range sorted {
		if err := md.Unlock(f); err != nil {
			return err
		}
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

// OpenForUpdate opens a json file with the expectation that the object will be
// modified and then saved again.
//
// Example:
//   func foo() (retErr error) {
//     var foo FooStruct
//     commit, err := md.OpenForUpdate(filename, &foo)
//     if err != nil {
//       panic(err)
//     }
//     defer commit(false, &retErr) // rollback unless first committed.
//     // modify foo
//     foo.Bar = X
//     return commit(true, nil) // commit
//  }
func (md *Metadata) OpenForUpdate(f string, obj interface{}) (func(commit bool, errp *error) error, error) {
	return md.OpenManyForUpdate([]string{f}, []interface{}{obj})
}

// OpenManyForUpdate is like OpenForUpdate, but for multiple files.
//
// Example:
//   func foo() (retErr error) {
//     file1, file2 := "file1", "file2"
//     var foo FooStruct
//     var bar BarStruct
//     // foo is read from file1, bar is read from file2.
//     commit, err := md.OpenManyForUpdate([]string{file1, file2}, []interface{}{&foo, &bar})
//     if err != nil {
//       panic(err)
//     }
//     defer commit(false, &retErr) // rollback unless first committed.
//     // modify foo and bar
//     foo.X = "new X"
//     bar.Y = "new Y"
//     return commit(true, nil) // commit
//  }
func (md *Metadata) OpenManyForUpdate(files []string, objects []interface{}) (func(commit bool, errp *error) error, error) {
	if len(files) != len(objects) {
		log.Panicf("len(files) != len(objects), %d != %d", len(files), len(objects))
	}
	if err := md.LockMany(files); err != nil {
		return nil, err
	}
	type readValue struct {
		i   int
		c   *Crypter
		err error
	}
	ch := make(chan readValue)
	crypters := make([]*Crypter, len(files))
	for i := range files {
		go func(i int, file string, obj interface{}) {
			crypter, err := md.ReadDataFile(file, obj)
			ch <- readValue{i, crypter, err}
		}(i, files[i], objects[i])
	}

	var errorList []error
	for _ = range files {
		v := <-ch
		if v.err != nil && !errors.Is(v.err, os.ErrNotExist) {
			errorList = append(errorList, v.err)
		}
		crypters[v.i] = v.c
	}
	if errorList != nil {
		md.UnlockMany(files)
		return nil, fmt.Errorf("md.ReadDataFile: %v", errorList)
	}

	var called, committed bool
	return func(commit bool, errp *error) (retErr error) {
		if called {
			if committed {
				return ErrAlreadyCommitted
			}
			return ErrAlreadyRolledBack
		}
		called = true
		if errp == nil || *errp != nil {
			errp = &retErr
		}
		if commit {
			// If some of the SaveDataFile calls fails and some succeed, the data could
			// be inconsistent. When we have more then one file, make a backup of the
			// original data, and restore it if anything goes wrong.
			//
			// If the process dies in the middle of saving the data, the backup will be
			// restored automatically when the process restarts. See md.New().
			var backup *backup
			if len(files) > 1 {
				var err error
				if backup, err = md.createBackup(files); err != nil {
					*errp = err
					return *errp
				}
			}
			ch := make(chan error)
			for i := range files {
				go func(c *Crypter, file string, obj interface{}) {
					ch <- md.SaveDataFile(c, file, obj)
				}(crypters[i], files[i], objects[i])
			}
			var errorList []error
			for _ = range files {
				if err := <-ch; err != nil {
					errorList = append(errorList, err)
				}
			}
			if errorList != nil {
				if backup != nil {
					backup.restore()
				}
				if *errp == nil {
					*errp = fmt.Errorf("md.SaveDataFile: %v", errorList)
				}
			} else {
				if backup != nil {
					backup.delete()
				}
				committed = true
			}
		}
		if err := md.UnlockMany(files); err != nil && *errp == nil {
			*errp = err
		}
		if !commit && *errp == nil {
			*errp = ErrRolledBack
		}
		return *errp
	}, nil
}

// DumpFile writes the decrypted content of a file to stdout.
func (md *Metadata) DumpFile(filename string) error {
	f, err := os.Open(filepath.Join(md.dir, filename))
	if err != nil {
		return err
	}
	defer f.Close()
	c := newCrypter(md.keyFromHeader)
	r, err := c.beginRead(f)
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

// ReadDataFile reads a json object from a file.
func (md *Metadata) ReadDataFile(filename string, obj interface{}) (*Crypter, error) {
	f, err := os.Open(filepath.Join(md.dir, filename))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	c := newCrypter(md.keyFromHeader)
	r, err := c.beginRead(f)
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

// SaveDataFile atomically replace a json object in a file.
func (md *Metadata) SaveDataFile(c *Crypter, filename string, obj interface{}) error {
	filename = filepath.Join(md.dir, filename)
	if c == nil {
		c = newCrypter(md.keyFromHeader)
		if err := createParentIfNotExist(filename); err != nil {
			return err
		}
	}
	t := fmt.Sprintf("%s.tmp-%d", filename, time.Now().UnixNano())
	f, err := os.OpenFile(t, os.O_WRONLY|os.O_CREATE|os.O_EXCL|os.O_SYNC, 0600)
	if err != nil {
		return err
	}
	w, err := c.beginWrite(f)
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
