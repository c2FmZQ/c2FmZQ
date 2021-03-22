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

	type Foo struct {
		Foo string `json:"foo"`
	}
	foo := Foo{"foo"}
	if err := saveJSON(fn, foo); err != nil {
		t.Fatalf("saveJSON failed: %v", err)
	}
	var bar Foo
	done, err := openForUpdate(fn, &bar)
	if err != nil {
		t.Fatalf("openForUpdate failed: %v", err)
	}
	if !reflect.DeepEqual(foo, bar) {
		t.Fatalf("openForUpdate() got %+v, want %+v", bar, foo)
	}
	bar.Foo = "bar"
	if err := done(nil); err != nil {
		t.Errorf("done() failed: %v", err)
	}

	if err := loadJSON(fn, &foo); err != nil {
		t.Fatalf("loadJSON() failed: %v", err)
	}
	if !reflect.DeepEqual(foo, bar) {
		t.Fatalf("openForUpdate() got %+v, want %+v", foo, bar)
	}
}

func TestOpenForUpdateDeferredDone(t *testing.T) {
	dir := t.TempDir()

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
		done, err := openForUpdate(fn, &foo)
		if err != nil {
			t.Fatalf("openForUpdate failed: %v", err)
		}
		defer done(&retErr)
		if err := os.RemoveAll(sub); err != nil {
			t.Fatalf("of.RemoveAll(%q): %v", sub, err)
		}
		return nil
	}

	if err := f(); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("f returned unexpected error: %v", err)
	}
}
