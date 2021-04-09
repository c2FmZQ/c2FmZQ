// Package secure stores arbitrary metadata in encrypted files.
package secure

import (
	"compress/gzip"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"time"

	"kringle-server/crypto"
	"kringle-server/log"
)

const (
	optEncrypted   = 1
	optCompressed  = 2
	optJSONEncoded = 4
	optGOBEncoded  = 8
)

var (
	// Indicates that the update was successfully rolled back.
	ErrRolledBack = errors.New("rolled back")
	// Indicates that the update was already rolled back by a previous call.
	ErrAlreadyRolledBack = errors.New("already rolled back")
	// Indicates that the update was already committed by a previous call.
	ErrAlreadyCommitted = errors.New("already committed")
)

// NewStorage returns a new Storage rooted at dir. The caller must provide an
// EncryptionKey that will be used to encrypt and decrypt per-file encryption
// keys.
func NewStorage(dir string, masterKey *crypto.EncryptionKey) *Storage {
	s := &Storage{
		dir:       dir,
		masterKey: masterKey,
	}
	s.useGOB = true
	if err := s.rollbackPendingOps(); err != nil {
		log.Fatalf("s.rollbackPendingOps: %v", err)
	}
	return s
}

// Offers the API to atomically read, write, and update encrypted files.
type Storage struct {
	dir       string
	masterKey *crypto.EncryptionKey
	compress  bool
	useGOB    bool
}

func createParentIfNotExist(filename string) error {
	dir, _ := filepath.Split(filename)
	return os.MkdirAll(dir, 0700)
}

// Lock atomically creates a lock file for the given filename. When this
// function returns without error, the lock is acquired and nobody else can
// acquire it until it is released.
//
// There is logic in place to remove stale locks after a while.
func (s *Storage) Lock(fn string) error {
	lockf := filepath.Join(s.dir, fn) + ".lock"
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
func (s *Storage) LockMany(filenames []string) error {
	sorted := make([]string, len(filenames))
	copy(sorted, filenames)
	sort.Strings(sorted)
	var locks []string
	for _, f := range sorted {
		if err := s.Lock(f); err != nil {
			s.UnlockMany(locks)
			return err
		}
		locks = append(locks, f)
	}
	return nil
}

// Unlock released the lock file for the given filename.
func (s *Storage) Unlock(fn string) error {
	lockf := filepath.Join(s.dir, fn) + ".lock"
	if err := os.Remove(lockf); err != nil {
		return err
	}
	return nil
}

// UnlockMany unlocks multiples files locked by LockMany().
func (s *Storage) UnlockMany(filenames []string) error {
	sorted := make([]string, len(filenames))
	copy(sorted, filenames)
	sort.Sort(sort.Reverse(sort.StringSlice(filenames)))
	for _, f := range sorted {
		if err := s.Unlock(f); err != nil {
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
//     commit, err := s.OpenForUpdate(filename, &foo)
//     if err != nil {
//       panic(err)
//     }
//     defer commit(false, &retErr) // rollback unless first committed.
//     // modify foo
//     foo.Bar = X
//     return commit(true, nil) // commit
//  }
func (s *Storage) OpenForUpdate(f string, obj interface{}) (func(commit bool, errp *error) error, error) {
	return s.OpenManyForUpdate([]string{f}, []interface{}{obj})
}

// OpenManyForUpdate is like OpenForUpdate, but for multiple files.
//
// Example:
//   func foo() (retErr error) {
//     file1, file2 := "file1", "file2"
//     var foo FooStruct
//     var bar BarStruct
//     // foo is read from file1, bar is read from file2.
//     commit, err := s.OpenManyForUpdate([]string{file1, file2}, []interface{}{&foo, &bar})
//     if err != nil {
//       panic(err)
//     }
//     defer commit(false, &retErr) // rollback unless first committed.
//     // modify foo and bar
//     foo.X = "new X"
//     bar.Y = "new Y"
//     return commit(true, nil) // commit
//  }
func (s *Storage) OpenManyForUpdate(files []string, objects interface{}) (func(commit bool, errp *error) error, error) {
	if reflect.TypeOf(objects).Kind() != reflect.Slice {
		log.Panic("objects must be a slice")
	}
	objValue := reflect.ValueOf(objects)
	if len(files) != objValue.Len() {
		log.Panicf("len(files) != len(objects), %d != %d", len(files), objValue.Len())
	}
	if err := s.LockMany(files); err != nil {
		return nil, err
	}
	type readValue struct {
		i   int
		k   *crypto.EncryptionKey
		err error
	}
	ch := make(chan readValue)
	keys := make([]*crypto.EncryptionKey, len(files))
	for i := range files {
		go func(i int, file string, obj interface{}) {
			k, err := s.ReadDataFile(file, obj)
			ch <- readValue{i, k, err}
		}(i, files[i], objValue.Index(i).Interface())
	}

	var errorList []error
	for _ = range files {
		v := <-ch
		if v.err != nil {
			errorList = append(errorList, v.err)
		}
		keys[v.i] = v.k
	}
	if errorList != nil {
		s.UnlockMany(files)
		return nil, fmt.Errorf("s.ReadDataFile: %w %v", errorList[0], errorList[1:])
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
			// restored automatically when the process restarts. See NewStorage().
			var backup *backup
			if len(files) > 1 {
				var err error
				if backup, err = s.createBackup(files); err != nil {
					*errp = err
					return *errp
				}
			}
			ch := make(chan error)
			for i := range files {
				go func(k *crypto.EncryptionKey, file string, obj interface{}) {
					ch <- s.SaveDataFile(k, file, obj)
				}(keys[i], files[i], objValue.Index(i).Interface())
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
					*errp = fmt.Errorf("s.SaveDataFile: %w %v", errorList[0], errorList[1:])
				}
			} else {
				if backup != nil {
					backup.delete()
				}
				committed = true
			}
		}
		if err := s.UnlockMany(files); err != nil && *errp == nil {
			*errp = err
		}
		if !commit && *errp == nil {
			*errp = ErrRolledBack
		}
		return *errp
	}, nil
}

// ReadDataFile reads a json object from a file.
func (s *Storage) ReadDataFile(filename string, obj interface{}) (*crypto.EncryptionKey, error) {
	f, err := os.Open(filepath.Join(s.dir, filename))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	hdr := make([]byte, 5)
	if _, err := io.ReadFull(f, hdr); err != nil {
		return nil, err
	}
	if string(hdr[:4]) != "KRIN" {
		return nil, errors.New("wrong file type")
	}
	flags := hdr[4]
	if flags&optEncrypted != 0 && s.masterKey == nil {
		return nil, errors.New("file is encrypted, but we don't have a decryption key")
	}

	var r io.Reader = f
	var k *crypto.EncryptionKey
	if flags&optEncrypted != 0 {
		// Read the encrypted file key.
		k, err := s.masterKey.ReadEncryptedKey(f)
		if err != nil {
			return nil, err
		}
		// Use the file key to decrypt the rest of the file.
		if r, err = k.StartReader(f); err != nil {
			return nil, err
		}
	}
	var rc io.Reader = r
	if flags&optCompressed != 0 {
		// Decompress the content of the file.
		gz, err := gzip.NewReader(r)
		if err != nil {
			return nil, err
		}
		defer gz.Close()
		rc = gz
	}

	if flags&optGOBEncoded != 0 {
		// Decode with GOB.
		if err := gob.NewDecoder(rc).Decode(obj); err != nil {
			return nil, err
		}
	} else if flags&optJSONEncoded != 0 {
		// Decode JSON object.
		if err := json.NewDecoder(rc).Decode(obj); err != nil {
			return nil, err
		}
	}
	return k, nil
}

// SaveDataFile atomically replace a json object in a file.
func (s *Storage) SaveDataFile(k *crypto.EncryptionKey, filename string, obj interface{}) error {
	t := fmt.Sprintf("%s.tmp-%d", filename, time.Now().UnixNano())
	if err := s.writeFile(k, t, obj); err != nil {
		return err
	}
	// Atomcically replace the file.
	return os.Rename(filepath.Join(s.dir, t), filepath.Join(s.dir, filename))
}

// CreateEmptyFile creates an empty file.
func (s *Storage) CreateEmptyFile(filename string, obj interface{}) error {
	return s.writeFile(nil, filename, obj)
}

// writeFile writes obj to a file.
func (s *Storage) writeFile(k *crypto.EncryptionKey, filename string, obj interface{}) error {
	fn := filepath.Join(s.dir, filename)
	if err := createParentIfNotExist(fn); err != nil {
		return err
	}
	var flags byte
	if s.masterKey != nil {
		flags |= optEncrypted
	}
	if s.compress {
		flags |= optCompressed
	}
	if s.useGOB {
		flags |= optGOBEncoded
	} else {
		flags |= optJSONEncoded
	}

	if k == nil && s.masterKey != nil {
		var err error
		if k, err = s.masterKey.NewEncryptionKey(); err != nil {
			return err
		}
	}
	f, err := os.OpenFile(fn, os.O_WRONLY|os.O_CREATE|os.O_EXCL|os.O_SYNC, 0600)
	if err != nil {
		return err
	}
	if _, err := f.Write([]byte{'K', 'R', 'I', 'N', flags}); err != nil {
		f.Close()
		return err
	}
	var w io.WriteCloser = f
	if s.masterKey != nil {
		// Write the encrypted file key first.
		if err := k.WriteEncryptedKey(f); err != nil {
			f.Close()
			return err
		}
		// Use the file key to encrypt the rest of the file.
		var err error
		if w, err = k.StartWriter(f); err != nil {
			f.Close()
			return err
		}
	}
	var rc io.WriteCloser = w
	if s.compress {
		// Compress the content.
		gz, err := gzip.NewWriterLevel(w, gzip.BestSpeed)
		if err != nil {
			return err
		}
		rc = gz
	}

	if s.useGOB {
		// Encode with GOB.
		if err := gob.NewEncoder(rc).Encode(obj); err != nil {
			if rc != w {
				rc.Close()
			}
			w.Close()
			return err
		}
	} else {
		// Encode as JSON object.
		enc := json.NewEncoder(rc)
		enc.SetIndent("", "  ")
		if err := enc.Encode(obj); err != nil {
			if rc != w {
				rc.Close()
			}
			w.Close()
			return err
		}
	}
	if rc != w {
		if err := rc.Close(); err != nil {
			w.Close()
			return err
		}
	}
	if err := w.Close(); err != nil {
		return err
	}
	return nil
}
