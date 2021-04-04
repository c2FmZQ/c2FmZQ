package md

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"kringle-server/crypto"
)

func encrypterDecrypter() crypto.EncryptionKey {
	mk, err := crypto.CreateMasterKey()
	if err != nil {
		panic(err)
	}
	return mk.EncryptionKey
}

func TestLock(t *testing.T) {
	dir := t.TempDir()
	md := New(dir, encrypterDecrypter())
	fn := "foo"

	if err := md.Lock(fn); err != nil {
		t.Fatalf("Lock() failed: %v", err)
	}
	go func() {
		time.Sleep(100 * time.Millisecond)
		md.Unlock(fn)
	}()
	if err := md.Lock(fn); err != nil {
		t.Errorf("Lock() failed: %v", err)
	}
	if err := md.Unlock(fn); err != nil {
		t.Errorf("Unlock() failed: %v", err)
	}
}

func TestOpenForUpdate(t *testing.T) {
	dir := t.TempDir()
	fn := "test.json"
	md := New(dir, encrypterDecrypter())

	type Foo struct {
		Foo string `json:"foo"`
	}
	foo := Foo{"foo"}
	if err := md.SaveDataFile(nil, fn, foo); err != nil {
		t.Fatalf("md.SaveDataFile failed: %v", err)
	}
	var bar Foo
	commit, err := md.OpenForUpdate(fn, &bar)
	if err != nil {
		t.Fatalf("md.OpenForUpdate failed: %v", err)
	}
	if !reflect.DeepEqual(foo, bar) {
		t.Fatalf("md.OpenForUpdate() got %+v, want %+v", bar, foo)
	}
	bar.Foo = "bar"
	if err := commit(true, nil); err != nil {
		t.Errorf("done() failed: %v", err)
	}
	if err := commit(false, nil); err != ErrAlreadyCommitted {
		t.Errorf("unexpected error. Want %v, got %v", ErrAlreadyCommitted, err)
	}

	if _, err := md.ReadDataFile(fn, &foo); err != nil {
		t.Fatalf("md.ReadDataFile() failed: %v", err)
	}
	if !reflect.DeepEqual(foo, bar) {
		t.Fatalf("d.openForUpdate() got %+v, want %+v", foo, bar)
	}
}

func TestRollback(t *testing.T) {
	dir := t.TempDir()
	fn := "test.json"
	md := New(dir, encrypterDecrypter())

	type Foo struct {
		Foo string `json:"foo"`
	}
	foo := Foo{"foo"}
	if err := md.SaveDataFile(nil, fn, foo); err != nil {
		t.Fatalf("md.SaveDataFile failed: %v", err)
	}
	var bar Foo
	commit, err := md.OpenForUpdate(fn, &bar)
	if err != nil {
		t.Fatalf("md.OpenForUpdate failed: %v", err)
	}
	if !reflect.DeepEqual(foo, bar) {
		t.Fatalf("md.OpenForUpdate() got %+v, want %+v", bar, foo)
	}
	bar.Foo = "bar"
	if err := commit(false, nil); err != ErrRolledBack {
		t.Errorf("unexpected error. Want %v, got %v", ErrRolledBack, err)
	}
	if err := commit(true, nil); err != ErrAlreadyRolledBack {
		t.Errorf("unexpected error. Want %v, got %v", ErrAlreadyRolledBack, err)
	}

	var foo2 Foo
	if _, err := md.ReadDataFile(fn, &foo2); err != nil {
		t.Fatalf("md.ReadDataFile() failed: %v", err)
	}
	if !reflect.DeepEqual(foo, foo2) {
		t.Fatalf("md.OpenForUpdate() got %+v, want %+v", foo2, foo)
	}
}

func TestOpenForUpdateDeferredDone(t *testing.T) {
	dir := t.TempDir()
	md := New(dir, encrypterDecrypter())

	// This function should return os.ErrNotExist because the file open for
	// update can't be saved.
	f := func() (retErr error) {
		fn := filepath.Join("sub", "test.json")
		type Foo struct {
			Foo string `json:"foo"`
		}
		var foo Foo
		commit, err := md.OpenForUpdate(fn, &foo)
		if err != nil {
			t.Fatalf("md.OpenForUpdate failed: %v", err)
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
