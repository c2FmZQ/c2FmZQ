package database

import (
	"reflect"
	"testing"
)

func TestMasterKey(t *testing.T) {
	db := &Database{dir: t.TempDir()}

	a, err := db.createMasterKey("foo")
	if err != nil {
		t.Fatalf("db.createMasterKey('foo'): %v", err)
	}
	b, err := db.readMasterKey("foo")
	if err != nil {
		t.Fatalf("db.readMasterKey('foo'): %v", err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Errorf("Mismatch keys: %v != %v", a, b)
	}
	if _, err := db.readMasterKey("bar"); err == nil {
		t.Errorf("db.readMasterKey('bar') should have failed, but didn't")
	}
}
