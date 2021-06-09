package crypto

import (
	"bytes"
	"crypto/rand"
	"io"
	"path/filepath"
	"reflect"
	"testing"

	"c2FmZQ/internal/log"
)

func init() {
	log.Level = 3
}

func TestMasterKey(t *testing.T) {
	dir := t.TempDir()
	keyFile := filepath.Join(dir, "key")
	mk, err := CreateAESMasterKey()
	if err != nil {
		t.Fatalf("CreateMasterKey: %v", err)
	}
	defer mk.Wipe()
	if err := mk.Save([]byte("foo"), keyFile); err != nil {
		t.Fatalf("mk.Save: %v", err)
	}

	got, err := ReadAESMasterKey([]byte("foo"), keyFile)
	if err != nil {
		t.Fatalf("ReadMasterKey('foo'): %v", err)
	}
	defer got.Wipe()
	if want := mk; !reflect.DeepEqual(want.(*AESMasterKey).key(), got.(*AESMasterKey).key()) {
		t.Errorf("Mismatch keys: %v != %v", want.(*AESMasterKey).key(), got.(*AESMasterKey).key())
	}
	if _, err := ReadAESMasterKey([]byte("bar"), keyFile); err == nil {
		t.Errorf("ReadMasterKey('bar') should have failed, but didn't")
	}
}

func TestEncryptDecrypt(t *testing.T) {
	mk, err := CreateAESMasterKey()
	if err != nil {
		t.Fatalf("CreateMasterKey: %v", err)
	}
	defer mk.Wipe()

	m := []byte("ABCDEFGHIJKLMNOPQRSTUVWXYZ")
	for i := 1; i < len(m); i++ {
		enc, err := mk.Encrypt(m[:i])
		if err != nil {
			t.Fatalf("mk.Encrypt: %v", err)
		}
		dec, err := mk.Decrypt(enc)
		if err != nil {
			t.Fatalf("mk.Decrypt: %v", err)
		}
		if !reflect.DeepEqual(m[:i], dec) {
			t.Errorf("Decrypted data[%d] doesn't match. Want %#v, got %#v", i, m[:i], dec)
		}
	}
}

func TestEncryptedKey(t *testing.T) {
	mk, err := CreateAESMasterKey()
	if err != nil {
		t.Fatalf("CreateMasterKey: %v", err)
	}
	defer mk.Wipe()

	ek, err := mk.NewKey()
	if err != nil {
		t.Fatalf("mk.NewKey: %v", err)
	}
	defer ek.Wipe()

	var buf bytes.Buffer
	if err := ek.WriteEncryptedKey(&buf); err != nil {
		t.Fatalf("ek.WriteEncryptedKey: %v", err)
	}

	ek2, err := mk.ReadEncryptedKey(&buf)
	if err != nil {
		t.Fatalf("mk.ReadEncryptedKey: %v", err)
	}
	defer ek2.Wipe()
	if want, got := ek.(*AESKey).key(), ek2.(*AESKey).key(); !reflect.DeepEqual(want, got) {
		t.Errorf("Unexpected key. Want %+v, got %+v", want, got)
	}
}

func TestStream(t *testing.T) {
	mk, err := CreateAESMasterKeyForTest()
	if err != nil {
		t.Fatalf("CreateMasterKey: %v", err)
	}
	var buf bytes.Buffer
	content := make([]byte, 10000)
	if _, err := rand.Read(content); err != nil {
		t.Fatalf("rand: %v", err)
	}
	ctx := uint32(0x12121212)
	w, err := mk.StartWriter(ctx, &buf)
	if err != nil {
		t.Fatalf("StartWriter: %v", err)
	}
	if _, err := w.Write(content); err != nil {
		t.Fatalf("StartWriter.Write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("StartWriter.Close: %v", err)
	}

	r, err := mk.StartReader(ctx, &buf)
	if err != nil {
		t.Fatalf("StartReader: %v", err)
	}
	var got []byte
	for s := 0; s < 1000; s++ {
		b := make([]byte, s)
		n, err := r.Read(b)
		got = append(got, b[:n]...)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("StartReader.Read: %v", err)
		}
	}
	if err := r.Close(); err != nil {
		t.Fatalf("StartReader.Close: %v", err)
	}
	if want := content; !reflect.DeepEqual(want, got) {
		t.Errorf("Read different content. Want %v, got %v", want, got)
	}
}

func TestStreamInvalidMAC(t *testing.T) {
	mk, err := CreateAESMasterKey()
	if err != nil {
		t.Fatalf("CreateMasterKey: %v", err)
	}
	var buf bytes.Buffer
	content := make([]byte, 10000)
	if _, err := rand.Read(content); err != nil {
		t.Fatalf("rand: %v", err)
	}
	ctx := uint32(0x44332211)
	w, err := mk.StartWriter(ctx, &buf)
	if err != nil {
		t.Fatalf("StartWriter: %v", err)
	}
	if _, err := w.Write(content); err != nil {
		t.Fatalf("StartWriter.Write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("StartWriter.Close: %v", err)
	}

	c := buf.Bytes()[buf.Len()-1]
	buf.Bytes()[buf.Len()-1] = ^c

	r, err := mk.StartReader(ctx, &buf)
	if err != nil {
		t.Fatalf("StartReader: %v", err)
	}
	b := make([]byte, 10000)
	if n, err := r.Read(b); err != ErrDecryptFailed {
		t.Errorf("StartReader.Read: %d, %v", n, err)
	}
}
