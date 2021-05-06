// Package secure stores arbitrary metadata in encrypted files.
package secure

import (
	"bytes"
	"compress/gzip"
	"crypto/sha1"
	"encoding"
	"encoding/binary"
	"encoding/gob"
	"encoding/hex"
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

	"c2FmZQ/internal/crypto"
	"c2FmZQ/internal/log"
)

const (
	optJSONEncoded   = 0x01 // encoding/json
	optGOBEncoded    = 0x02 // encoding/gob
	optBinaryEncoded = 0x03 // with encoding.BinaryMarshaler
	optRawBytes      = 0x04 // []byte
	optEncodingMask  = 0x0F

	optEncrypted  = 0x10
	optCompressed = 0x20
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

// Storage offers the API to atomically read, write, and update encrypted files.
type Storage struct {
	dir       string
	masterKey *crypto.EncryptionKey
	compress  bool
	useGOB    bool
}

// Dir returns the root directory of the storage.
func (s *Storage) Dir() string {
	return s.dir
}

// HashString returns a cryptographically secure hash of a string.
func (s *Storage) HashString(str string) string {
	return hex.EncodeToString(s.masterKey.Hash([]byte(str)))
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

// OpenForUpdate opens a file with the expectation that the object will be
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
		err error
	}
	ch := make(chan readValue)
	for i := range files {
		go func(i int, file string, obj interface{}) {
			err := s.ReadDataFile(file, obj)
			ch <- readValue{i, err}
		}(i, files[i], objValue.Index(i).Interface())
	}

	var errorList []error
	for _ = range files {
		v := <-ch
		if v.err != nil {
			errorList = append(errorList, v.err)
		}
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
				go func(file string, obj interface{}) {
					ch <- s.SaveDataFile(file, obj)
				}(files[i], objValue.Index(i).Interface())
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

func context(s string) (ctx uint32) {
	h := sha1.Sum([]byte(s))
	ctx = binary.BigEndian.Uint32(h[:4])
	return
}

// ReadDataFile reads an object from a file.
func (s *Storage) ReadDataFile(filename string, obj interface{}) error {
	f, err := os.Open(filepath.Join(s.dir, filename))
	if err != nil {
		return err
	}
	defer f.Close()

	hdr := make([]byte, 5)
	if _, err := io.ReadFull(f, hdr); err != nil {
		return err
	}
	if string(hdr[:4]) != "KRIN" {
		return errors.New("wrong file type")
	}
	flags := hdr[4]
	if flags&optEncrypted != 0 && s.masterKey == nil {
		return errors.New("file is encrypted, but a master key was not provided")
	}

	var r io.ReadCloser = f
	if flags&optEncrypted != 0 {
		// Read the encrypted file key.
		k, err := s.masterKey.ReadEncryptedKey(f)
		if err != nil {
			return err
		}
		// Use the file key to decrypt the rest of the file.
		if r, err = k.StartReader(context(filename), f); err != nil {
			return err
		}
		// Read the header again.
		h := make([]byte, 5)
		if _, err := io.ReadFull(r, h); err != nil {
			return err
		}
		if bytes.Compare(hdr, h) != 0 {
			return errors.New("wrong encrypted header")
		}
	}
	var rc io.Reader = r
	if flags&optCompressed != 0 {
		// Decompress the content of the file.
		gz, err := gzip.NewReader(r)
		if err != nil {
			return err
		}
		defer gz.Close()
		rc = gz
	}

	switch enc := flags & optEncodingMask; enc {
	case optGOBEncoded:
		// Decode with GOB.
		if err := gob.NewDecoder(rc).Decode(obj); err != nil {
			log.Debugf("gob Decode: %v", err)
			return err
		}
	case optJSONEncoded:
		// Decode JSON object.
		if err := json.NewDecoder(rc).Decode(obj); err != nil {
			log.Debugf("json Decode: %v", err)
			return err
		}
	case optBinaryEncoded:
		// Decode with UnmarshalBinary.
		u, ok := obj.(encoding.BinaryUnmarshaler)
		if !ok {
			return fmt.Errorf("obj doesn't implement encoding.BinaryUnmarshaler: %T", obj)
		}
		b, err := io.ReadAll(rc)
		if err != nil {
			return err
		}
		if err := u.UnmarshalBinary(b); err != nil {
			return err
		}
	case optRawBytes:
		// Read raw bytes.
		b, ok := obj.(*[]byte)
		if !ok {
			return fmt.Errorf("obj isn't *[]byte: %T", obj)
		}
		buf := make([]byte, 1024)
		for {
			n, err := rc.Read(buf)
			if n > 0 {
				*b = append(*b, buf[:n]...)
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unexpected encoding %x", enc)
	}
	if r != f {
		if err := r.Close(); err != nil {
			return err
		}
	}
	return nil
}

// SaveDataFile atomically replace an object in a file.
func (s *Storage) SaveDataFile(filename string, obj interface{}) error {
	t := fmt.Sprintf("%s.tmp-%d", filename, time.Now().UnixNano())
	if err := s.writeFile(context(filename), t, obj); err != nil {
		return err
	}
	// Atomically replace the file.
	return os.Rename(filepath.Join(s.dir, t), filepath.Join(s.dir, filename))
}

// CreateEmptyFile creates an empty file.
func (s *Storage) CreateEmptyFile(filename string, empty interface{}) error {
	return s.writeFile(context(filename), filename, empty)
}

// writeFile writes obj to a file.
func (s *Storage) writeFile(ctx uint32, filename string, obj interface{}) (retErr error) {
	fn := filepath.Join(s.dir, filename)
	if err := createParentIfNotExist(fn); err != nil {
		return err
	}

	var flags byte
	if _, ok := obj.(encoding.BinaryMarshaler); ok {
		flags = optBinaryEncoded
	} else if _, ok := obj.(*[]byte); ok {
		flags = optRawBytes
	} else if s.useGOB {
		flags = optGOBEncoded
	} else {
		flags = optJSONEncoded
	}
	if s.masterKey != nil {
		flags |= optEncrypted
	}
	if s.compress {
		flags |= optCompressed
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
		k, err := s.masterKey.NewEncryptionKey()
		if err != nil {
			return err
		}
		// Write the encrypted file key first.
		if err := k.WriteEncryptedKey(f); err != nil {
			f.Close()
			return err
		}
		// Use the file key to encrypt the rest of the file.
		if w, err = k.StartWriter(ctx, f); err != nil {
			f.Close()
			return err
		}
		// Write the header again.
		if _, err := w.Write([]byte{'K', 'R', 'I', 'N', flags}); err != nil {
			w.Close()
			return err
		}
	}
	var wc io.WriteCloser = w
	if s.compress {
		// Compress the content.
		gz, err := gzip.NewWriterLevel(w, gzip.BestSpeed)
		if err != nil {
			return err
		}
		wc = gz
	}

	defer func() {
		if wc != w {
			if err := wc.Close(); err != nil && retErr == nil {
				retErr = err
			}
		}
		if err := w.Close(); err != nil && retErr == nil {
			retErr = err
		}
	}()

	switch enc := flags & optEncodingMask; enc {
	case optGOBEncoded:
		// Encode with GOB.
		if err := gob.NewEncoder(wc).Encode(obj); err != nil {
			return err
		}
	case optJSONEncoded:
		// Encode as JSON object.
		enc := json.NewEncoder(wc)
		enc.SetIndent("", "  ")
		if err := enc.Encode(obj); err != nil {
			return err
		}
	case optBinaryEncoded:
		// Encode with BinaryMarshaler.
		m, ok := obj.(encoding.BinaryMarshaler)
		if !ok {
			return fmt.Errorf("obj doesn't implement encoding.BinaryMarshaler: %T", obj)
		}
		b, err := m.MarshalBinary()
		if err != nil {
			return err
		}
		if _, err := wc.Write(b); err != nil {
			return err
		}
	case optRawBytes:
		// Write raw bytes.
		b, ok := obj.(*[]byte)
		if !ok {
			return fmt.Errorf("obj isn't *[]byte: %T", obj)
		}
		if b != nil {
			if _, err := wc.Write(*b); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unexpected encoding %x", enc)
	}

	return nil
}
