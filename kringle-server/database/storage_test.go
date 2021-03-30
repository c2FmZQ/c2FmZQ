package database

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestLock(t *testing.T) {
	dir := t.TempDir()
	fn := filepath.Join(dir, "foo")

	if err := lock(fn); err != nil {
		t.Fatalf("lock() failed: %v", err)
	}
	go func() {
		time.Sleep(100 * time.Millisecond)
		unlock(fn)
	}()
	if err := lock(fn); err != nil {
		t.Errorf("lock() failed: %v", err)
	}
	if err := unlock(fn); err != nil {
		t.Errorf("unlock() failed: %v", err)
	}
}

func TestOpenForUpdate(t *testing.T) {
	dir := t.TempDir()
	fn := filepath.Join(dir, "test.json")
	db := New(dir, "")

	type Foo struct {
		Foo string `json:"foo"`
	}
	foo := Foo{"foo"}
	if err := db.saveDataFile(nil, fn, foo); err != nil {
		t.Fatalf("d.saveDataFile failed: %v", err)
	}
	var bar Foo
	commit, err := db.openForUpdate(fn, &bar)
	if err != nil {
		t.Fatalf("db.openForUpdate failed: %v", err)
	}
	if !reflect.DeepEqual(foo, bar) {
		t.Fatalf("db.openForUpdate() got %+v, want %+v", bar, foo)
	}
	bar.Foo = "bar"
	if err := commit(true, nil); err != nil {
		t.Errorf("done() failed: %v", err)
	}
	if err := commit(false, nil); err == nil {
		t.Error("second commit should have failed")
	}

	if _, err := db.readDataFile(fn, &foo); err != nil {
		t.Fatalf("db.readDataFile() failed: %v", err)
	}
	if !reflect.DeepEqual(foo, bar) {
		t.Fatalf("d.openForUpdate() got %+v, want %+v", foo, bar)
	}
}

func TestRollback(t *testing.T) {
	dir := t.TempDir()
	fn := filepath.Join(dir, "test.json")
	db := New(dir, "")

	type Foo struct {
		Foo string `json:"foo"`
	}
	foo := Foo{"foo"}
	if err := db.saveDataFile(nil, fn, foo); err != nil {
		t.Fatalf("d.saveDataFile failed: %v", err)
	}
	var bar Foo
	commit, err := db.openForUpdate(fn, &bar)
	if err != nil {
		t.Fatalf("db.openForUpdate failed: %v", err)
	}
	if !reflect.DeepEqual(foo, bar) {
		t.Fatalf("db.openForUpdate() got %+v, want %+v", bar, foo)
	}
	bar.Foo = "bar"
	if err := commit(false, nil); err == nil {
		t.Error("second commit should have failed")
	}

	var foo2 Foo
	if _, err := db.readDataFile(fn, &foo2); err != nil {
		t.Fatalf("db.readDataFile() failed: %v", err)
	}
	if !reflect.DeepEqual(foo, foo2) {
		t.Fatalf("d.openForUpdate() got %+v, want %+v", foo2, foo)
	}
}

func TestOpenForUpdateDeferredDone(t *testing.T) {
	dir := t.TempDir()
	db := New(dir, "")

	// This function should return os.ErrNotExist because the file open for
	// update can't be saved.
	f := func() (retErr error) {
		sub := filepath.Join(dir, "sub")
		if err := os.Mkdir(sub, 0700); err != nil {
			t.Fatalf("os.Mkdir(%q, 0700): %v", sub, err)
		}
		fn := filepath.Join(sub, "test.json")
		type Foo struct {
			Foo string `json:"foo"`
		}
		var foo Foo
		commit, err := db.openForUpdate(fn, &foo)
		if err != nil {
			t.Fatalf("db.openForUpdate failed: %v", err)
		}
		defer commit(true, &retErr)
		if err := os.RemoveAll(sub); err != nil {
			t.Fatalf("of.RemoveAll(%q): %v", sub, err)
		}
		return nil
	}

	if err := f(); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("f returned unexpected error: %v", err)
	}
}
