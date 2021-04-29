package secure

import (
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"kringle/crypto"
	"kringle/log"
)

func init() {
	log.Level = 3
}

func encryptionKey() *crypto.EncryptionKey {
	mk, err := crypto.CreateMasterKey()
	if err != nil {
		panic(err)
	}
	return &mk.EncryptionKey
}

func TestLock(t *testing.T) {
	dir := t.TempDir()
	s := NewStorage(dir, encryptionKey())
	fn := "foo"

	if err := s.Lock(fn); err != nil {
		t.Fatalf("Lock() failed: %v", err)
	}
	go func() {
		time.Sleep(100 * time.Millisecond)
		s.Unlock(fn)
	}()
	if err := s.Lock(fn); err != nil {
		t.Errorf("Lock() failed: %v", err)
	}
	if err := s.Unlock(fn); err != nil {
		t.Errorf("Unlock() failed: %v", err)
	}
}

func TestOpenForUpdate(t *testing.T) {
	dir := t.TempDir()
	fn := "test.json"
	s := NewStorage(dir, encryptionKey())

	type Foo struct {
		Foo string `json:"foo"`
	}
	foo := Foo{"foo"}
	if err := s.SaveDataFile(nil, fn, foo); err != nil {
		t.Fatalf("s.SaveDataFile failed: %v", err)
	}
	var bar Foo
	commit, err := s.OpenForUpdate(fn, &bar)
	if err != nil {
		t.Fatalf("s.OpenForUpdate failed: %v", err)
	}
	if !reflect.DeepEqual(foo, bar) {
		t.Fatalf("s.OpenForUpdate() got %+v, want %+v", bar, foo)
	}
	bar.Foo = "bar"
	if err := commit(true, nil); err != nil {
		t.Errorf("done() failed: %v", err)
	}
	if err := commit(false, nil); err != ErrAlreadyCommitted {
		t.Errorf("unexpected error. Want %v, got %v", ErrAlreadyCommitted, err)
	}

	if _, err := s.ReadDataFile(fn, &foo); err != nil {
		t.Fatalf("s.ReadDataFile() failed: %v", err)
	}
	if !reflect.DeepEqual(foo, bar) {
		t.Fatalf("d.openForUpdate() got %+v, want %+v", foo, bar)
	}
}

func TestRollback(t *testing.T) {
	dir := t.TempDir()
	fn := "test.json"
	s := NewStorage(dir, encryptionKey())

	type Foo struct {
		Foo string `json:"foo"`
	}
	foo := Foo{"foo"}
	if err := s.SaveDataFile(nil, fn, foo); err != nil {
		t.Fatalf("s.SaveDataFile failed: %v", err)
	}
	var bar Foo
	commit, err := s.OpenForUpdate(fn, &bar)
	if err != nil {
		t.Fatalf("s.OpenForUpdate failed: %v", err)
	}
	if !reflect.DeepEqual(foo, bar) {
		t.Fatalf("s.OpenForUpdate() got %+v, want %+v", bar, foo)
	}
	bar.Foo = "bar"
	if err := commit(false, nil); err != ErrRolledBack {
		t.Errorf("unexpected error. Want %v, got %v", ErrRolledBack, err)
	}
	if err := commit(true, nil); err != ErrAlreadyRolledBack {
		t.Errorf("unexpected error. Want %v, got %v", ErrAlreadyRolledBack, err)
	}

	var foo2 Foo
	if _, err := s.ReadDataFile(fn, &foo2); err != nil {
		t.Fatalf("s.ReadDataFile() failed: %v", err)
	}
	if !reflect.DeepEqual(foo, foo2) {
		t.Fatalf("s.OpenForUpdate() got %+v, want %+v", foo2, foo)
	}
}

func TestOpenForUpdateDeferredDone(t *testing.T) {
	dir := t.TempDir()
	s := NewStorage(dir, encryptionKey())

	// This function should return os.ErrNotExist because the file open for
	// update can't be saved.
	f := func() (retErr error) {
		fn := filepath.Join("sub", "test.json")
		type Foo struct {
			Foo string `json:"foo"`
		}
		if err := s.CreateEmptyFile(fn, Foo{}); err != nil {
			t.Fatalf("s.CreateEmptyFile failed: %v", err)
		}
		var foo Foo
		commit, err := s.OpenForUpdate(fn, &foo)
		if err != nil {
			t.Fatalf("s.OpenForUpdate failed: %v", err)
		}
		defer commit(true, &retErr)
		if err := os.RemoveAll(filepath.Join(dir, "sub")); err != nil {
			t.Fatalf("of.RemoveAll(sub): %v", err)
		}
		return nil
	}

	if err := f(); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("f returned unexpected error: %v", err)
	}
}

func TestEncodeByteSlice(t *testing.T) {
	want := []byte("Hello world")
	dir := t.TempDir()
	s := NewStorage(dir, encryptionKey())
	if err := s.CreateEmptyFile("file", (*[]byte)(nil)); err != nil {
		t.Fatalf("s.CreateEmptyFile failed: %v", err)
	}
	if err := s.SaveDataFile(nil, "file", &want); err != nil {
		t.Fatalf("s.WriteDataFile() failed: %v", err)
	}
	var got []byte
	if _, err := s.ReadDataFile("file", &got); err != nil {
		t.Fatalf("s.ReadDataFile() failed: %v", err)
	}
	if !reflect.DeepEqual(want, got) {
		t.Errorf("Unexpected msg. Want %q, got %q", want, got)
	}
}

func TestEncodeBinary(t *testing.T) {
	want := time.Now()
	dir := t.TempDir()
	s := NewStorage(dir, encryptionKey())
	if err := s.CreateEmptyFile("file", &time.Time{}); err != nil {
		t.Fatalf("s.CreateEmptyFile failed: %v", err)
	}
	if err := s.SaveDataFile(nil, "file", &want); err != nil {
		t.Fatalf("s.WriteDataFile() failed: %v", err)
	}
	var got time.Time
	if _, err := s.ReadDataFile("file", &got); err != nil {
		t.Fatalf("s.ReadDataFile() failed: %v", err)
	}
	if got.UnixNano() != want.UnixNano() {
		t.Errorf("Unexpected time. Want %q, got %q", want, got)
	}
}

func RunBenchmarkOpenForUpdate(b *testing.B, kb int, k *crypto.EncryptionKey, compress, useGOB bool) {
	dir := b.TempDir()
	file := filepath.Join(dir, "testfile")
	s := NewStorage(dir, k)
	s.compress = compress
	s.useGOB = useGOB

	obj := struct {
		M map[string]string `json:"m"`
	}{}
	obj.M = make(map[string]string)
	for i := 0; i < kb; i++ {
		key := make([]byte, 32)
		value := make([]byte, 1024)
		if _, err := rand.Read(key); err != nil {
			b.Fatalf("io.ReadFull: %v", err)
		}
		if _, err := rand.Read(value); err != nil {
			b.Fatalf("io.ReadFull: %v", err)
		}
		obj.M[string(key)] = string(value)
	}
	if err := s.writeFile(nil, "testfile", &obj); err != nil {
		b.Fatalf("s.writeFile: %v", err)
	}
	fi, err := os.Stat(file)
	if err != nil {
		b.Fatalf("os.Stat: %v", err)
	}
	b.ResetTimer()
	b.SetBytes(fi.Size())
	for i := 0; i < b.N; i++ {
		commit, err := s.OpenForUpdate("testfile", &obj)
		if err != nil {
			b.Fatalf("s.OpenForUpdate: %v", err)
		}
		if err := commit(true, nil); err != nil {
			b.Fatalf("commit: %v", err)
		}
	}
}

func BenchmarkOpenForUpdate_JSON_1KB_AES(b *testing.B) {
	RunBenchmarkOpenForUpdate(b, 1, encryptionKey(), false, false)
}

func BenchmarkOpenForUpdate_JSON_1MB_AES(b *testing.B) {
	RunBenchmarkOpenForUpdate(b, 1024, encryptionKey(), false, false)
}

func BenchmarkOpenForUpdate_JSON_10MB_AES(b *testing.B) {
	RunBenchmarkOpenForUpdate(b, 10240, encryptionKey(), false, false)
}

func BenchmarkOpenForUpdate_JSON_20MB_AES(b *testing.B) {
	RunBenchmarkOpenForUpdate(b, 20480, encryptionKey(), false, false)
}

func BenchmarkOpenForUpdate_JSON_1KB_PlainText(b *testing.B) {
	RunBenchmarkOpenForUpdate(b, 1, nil, false, false)
}

func BenchmarkOpenForUpdate_JSON_1MB_PlainText(b *testing.B) {
	RunBenchmarkOpenForUpdate(b, 1024, nil, false, false)
}

func BenchmarkOpenForUpdate_JSON_10MB_PlainText(b *testing.B) {
	RunBenchmarkOpenForUpdate(b, 10240, nil, false, false)
}

func BenchmarkOpenForUpdate_JSON_20MB_PlainText(b *testing.B) {
	RunBenchmarkOpenForUpdate(b, 20480, nil, false, false)
}

func BenchmarkOpenForUpdate_GOB_1KB_AES(b *testing.B) {
	RunBenchmarkOpenForUpdate(b, 1, encryptionKey(), false, true)
}

func BenchmarkOpenForUpdate_GOB_1MB_AES(b *testing.B) {
	RunBenchmarkOpenForUpdate(b, 1024, encryptionKey(), false, true)
}

func BenchmarkOpenForUpdate_GOB_10MB_AES(b *testing.B) {
	RunBenchmarkOpenForUpdate(b, 10240, encryptionKey(), false, true)
}

func BenchmarkOpenForUpdate_GOB_20MB_AES(b *testing.B) {
	RunBenchmarkOpenForUpdate(b, 20480, encryptionKey(), false, true)
}

func BenchmarkOpenForUpdate_GOB_1KB_PlainText(b *testing.B) {
	RunBenchmarkOpenForUpdate(b, 1, nil, false, true)
}

func BenchmarkOpenForUpdate_GOB_1MB_PlainText(b *testing.B) {
	RunBenchmarkOpenForUpdate(b, 1024, nil, false, true)
}

func BenchmarkOpenForUpdate_GOB_10MB_PlainText(b *testing.B) {
	RunBenchmarkOpenForUpdate(b, 10240, nil, false, true)
}

func BenchmarkOpenForUpdate_GOB_20MB_PlainText(b *testing.B) {
	RunBenchmarkOpenForUpdate(b, 20480, nil, false, true)
}

func BenchmarkOpenForUpdate_GOB_1KB_PlainText_GZIP(b *testing.B) {
	RunBenchmarkOpenForUpdate(b, 1, nil, true, true)
}

func BenchmarkOpenForUpdate_GOB_1MB_PlainText_GZIP(b *testing.B) {
	RunBenchmarkOpenForUpdate(b, 1024, nil, true, true)
}

func BenchmarkOpenForUpdate_GOB_10MB_PlainText_GZIP(b *testing.B) {
	RunBenchmarkOpenForUpdate(b, 10240, nil, true, true)
}

func BenchmarkOpenForUpdate_GOB_20MB_PlainText_GZIP(b *testing.B) {
	RunBenchmarkOpenForUpdate(b, 20480, nil, true, true)
}
