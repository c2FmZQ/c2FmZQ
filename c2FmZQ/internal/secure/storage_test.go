//
// Copyright 2021-2022 TTBT Enterprises LLC
//
// This file is part of c2FmZQ (https://c2FmZQ.org/).
//
// c2FmZQ is free software: you can redistribute it and/or modify it under the
// terms of the GNU General Public License as published by the Free Software
// Foundation, either version 3 of the License, or (at your option) any later
// version.
//
// c2FmZQ is distributed in the hope that it will be useful, but WITHOUT ANY
// WARRANTY; without even the implied warranty of MERCHANTABILITY or FITNESS FOR
// A PARTICULAR PURPOSE. See the GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License along with
// c2FmZQ. If not, see <https://www.gnu.org/licenses/>.

package secure

import (
	"crypto/rand"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"c2FmZQ/internal/crypto"
	"c2FmZQ/internal/log"
)

func init() {
	log.Level = 2
}

func aesEncryptionKey() crypto.EncryptionKey {
	mk, err := crypto.CreateAESMasterKeyForTest()
	if err != nil {
		panic(err)
	}
	return mk.(crypto.EncryptionKey)
}

func ccEncryptionKey() crypto.EncryptionKey {
	mk, err := crypto.CreateChacha20Poly1305MasterKeyForTest()
	if err != nil {
		panic(err)
	}
	return mk.(crypto.EncryptionKey)
}

func TestLock(t *testing.T) {
	dir := t.TempDir()
	s := NewStorage(dir, aesEncryptionKey())
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
	s := NewStorage(dir, aesEncryptionKey())

	type Foo struct {
		Foo string `json:"foo"`
	}
	foo := Foo{"foo"}
	if err := s.SaveDataFile(fn, foo); err != nil {
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

	if err := s.ReadDataFile(fn, &foo); err != nil {
		t.Fatalf("s.ReadDataFile() failed: %v", err)
	}
	if !reflect.DeepEqual(foo, bar) {
		t.Fatalf("d.openForUpdate() got %+v, want %+v", foo, bar)
	}
}

func TestRollback(t *testing.T) {
	dir := t.TempDir()
	fn := "test.json"
	s := NewStorage(dir, aesEncryptionKey())

	type Foo struct {
		Foo string `json:"foo"`
	}
	foo := Foo{"foo"}
	if err := s.SaveDataFile(fn, foo); err != nil {
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
	if err := s.ReadDataFile(fn, &foo2); err != nil {
		t.Fatalf("s.ReadDataFile() failed: %v", err)
	}
	if !reflect.DeepEqual(foo, foo2) {
		t.Fatalf("s.OpenForUpdate() got %+v, want %+v", foo2, foo)
	}
}

func TestOpenForUpdateDeferredDone(t *testing.T) {
	dir := t.TempDir()
	s := NewStorage(dir, aesEncryptionKey())

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
	s := NewStorage(dir, aesEncryptionKey())
	if err := s.CreateEmptyFile("file", (*[]byte)(nil)); err != nil {
		t.Fatalf("s.CreateEmptyFile failed: %v", err)
	}
	if err := s.SaveDataFile("file", &want); err != nil {
		t.Fatalf("s.WriteDataFile() failed: %v", err)
	}
	var got []byte
	if err := s.ReadDataFile("file", &got); err != nil {
		t.Fatalf("s.ReadDataFile() failed: %v", err)
	}
	if !reflect.DeepEqual(want, got) {
		t.Errorf("Unexpected msg. Want %q, got %q", want, got)
	}
}

func TestEncodeBinary(t *testing.T) {
	want := time.Now()
	dir := t.TempDir()
	s := NewStorage(dir, aesEncryptionKey())
	if err := s.CreateEmptyFile("file", &time.Time{}); err != nil {
		t.Fatalf("s.CreateEmptyFile failed: %v", err)
	}
	if err := s.SaveDataFile("file", &want); err != nil {
		t.Fatalf("s.WriteDataFile() failed: %v", err)
	}
	var got time.Time
	if err := s.ReadDataFile("file", &got); err != nil {
		t.Fatalf("s.ReadDataFile() failed: %v", err)
	}
	if got.UnixNano() != want.UnixNano() {
		t.Errorf("Unexpected time. Want %q, got %q", want, got)
	}
}

func TestBlobs(t *testing.T) {
	dir := t.TempDir()
	s := NewStorage(dir, aesEncryptionKey())
	//s := NewStorage(dir, ccEncryptionKey())
	const (
		temp    = "tempfile"
		final   = "finalfile"
		content = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	)

	w, err := s.OpenBlobWrite(temp, final)
	if err != nil {
		t.Fatalf("s.OpenBlobWrite failed: %v", err)
	}
	if _, err := w.Write([]byte(content)); err != nil {
		t.Fatalf("w.Write failed: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("w.Close failed: %v", err)
	}

	var buf []byte
	if err := s.ReadDataFile(temp, &buf); err == nil {
		t.Fatalf("s.ReadDataFile() didn't fail. Got: %s", buf)
	}
	if err := os.Rename(filepath.Join(dir, temp), filepath.Join(dir, final)); err != nil {
		t.Fatalf("os.Rename failed: %v", err)
	}
	if err := s.ReadDataFile(final, &buf); err != nil {
		t.Fatalf("s.ReadDataFile() failed: %v", err)
	}
	if want, got := content, string(buf); want != got {
		t.Errorf("Unexpected content. Want %q, got %q", want, got)
	}

	r, err := s.OpenBlobRead(final)
	if err != nil {
		t.Fatalf("s.OpenBlobRead failed: %v", err)
	}

	// Test SeekStart.
	off, err := r.Seek(5, io.SeekStart)
	if err != nil {
		t.Fatalf("r.Seek(5, io.SeekStart) failed: %v", err)
	}
	if want, got := int64(5), off; want != got {
		t.Errorf("Unexpected seek offset. Want %d, got %d", want, got)
	}
	if got, err := io.ReadAll(r); err != nil || string(got) != content[5:] {
		t.Errorf("Unexpected content. Want %q, got %s", content[5:], got)
	}

	// Test SeekCurrent.
	if _, err := r.Seek(5, io.SeekStart); err != nil {
		t.Fatalf("r.Seek(5, io.SeekStart) failed: %v", err)
	}
	if off, err = r.Seek(10, io.SeekCurrent); err != nil {
		t.Fatalf("r.Seek(5, io.SeekCurrent) failed: %v", err)
	}
	if want, got := int64(15), off; want != got {
		t.Errorf("Unexpected seek offset. Want %d, got %d", want, got)
	}
	if got, err := io.ReadAll(r); err != nil || string(got) != content[15:] {
		t.Errorf("Unexpected content. Want %q, got %s", content[15:], got)
	}

	// Test SeekEnd.
	if off, err = r.Seek(-3, io.SeekEnd); err != nil {
		t.Fatalf("r.Seek(-3, io.SeekEnd) failed: %v", err)
	}
	if want, got := int64(len(content)-3), off; want != got {
		t.Errorf("Unexpected seek offset. Want %d, got %d", want, got)
	}
	if got, err := io.ReadAll(r); err != nil || string(got) != "XYZ" {
		t.Errorf("Unexpected content. Want %q, got %s", "XYZ", got)
	}

	// Test SeekEnd.
	if off, err = r.Seek(0, io.SeekEnd); err != nil {
		t.Fatalf("r.Seek(0, io.SeekEnd) failed: %v", err)
	}
	if want, got := int64(len(content)), off; want != got {
		t.Errorf("Unexpected seek offset. Want %d, got %d", want, got)
	}
	if got, err := io.ReadAll(r); err != nil || string(got) != "" {
		t.Errorf("Unexpected content. Want %q, got %s", "", got)
	}

	if err := r.Close(); err != nil {
		t.Fatalf("r.Close failed: %v", err)
	}
}

func RunBenchmarkOpenForUpdate(b *testing.B, kb int, k crypto.EncryptionKey, compress, useGOB bool) {
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
	if err := s.writeFile(context("testfile"), "testfile", &obj); err != nil {
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
	RunBenchmarkOpenForUpdate(b, 1, aesEncryptionKey(), false, false)
}

func BenchmarkOpenForUpdate_JSON_1MB_AES(b *testing.B) {
	RunBenchmarkOpenForUpdate(b, 1024, aesEncryptionKey(), false, false)
}

func BenchmarkOpenForUpdate_JSON_10MB_AES(b *testing.B) {
	RunBenchmarkOpenForUpdate(b, 10240, aesEncryptionKey(), false, false)
}

func BenchmarkOpenForUpdate_JSON_20MB_AES(b *testing.B) {
	RunBenchmarkOpenForUpdate(b, 20480, aesEncryptionKey(), false, false)
}

func BenchmarkOpenForUpdate_JSON_1KB_CHACHA20POLY1305(b *testing.B) {
	RunBenchmarkOpenForUpdate(b, 1, ccEncryptionKey(), false, false)
}

func BenchmarkOpenForUpdate_JSON_1MB_CHACHA20POLY1305(b *testing.B) {
	RunBenchmarkOpenForUpdate(b, 1024, ccEncryptionKey(), false, false)
}

func BenchmarkOpenForUpdate_JSON_10MB_CHACHA20POLY1305(b *testing.B) {
	RunBenchmarkOpenForUpdate(b, 10240, ccEncryptionKey(), false, false)
}

func BenchmarkOpenForUpdate_JSON_20MB_CHACHA20POLY1305(b *testing.B) {
	RunBenchmarkOpenForUpdate(b, 20480, ccEncryptionKey(), false, false)
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
	RunBenchmarkOpenForUpdate(b, 1, aesEncryptionKey(), false, true)
}

func BenchmarkOpenForUpdate_GOB_1MB_AES(b *testing.B) {
	RunBenchmarkOpenForUpdate(b, 1024, aesEncryptionKey(), false, true)
}

func BenchmarkOpenForUpdate_GOB_10MB_AES(b *testing.B) {
	RunBenchmarkOpenForUpdate(b, 10240, aesEncryptionKey(), false, true)
}

func BenchmarkOpenForUpdate_GOB_20MB_AES(b *testing.B) {
	RunBenchmarkOpenForUpdate(b, 20480, aesEncryptionKey(), false, true)
}

func BenchmarkOpenForUpdate_GOB_1KB_CHACHA20POLY1305(b *testing.B) {
	RunBenchmarkOpenForUpdate(b, 1, ccEncryptionKey(), false, true)
}

func BenchmarkOpenForUpdate_GOB_1MB_CHACHA20POLY1305(b *testing.B) {
	RunBenchmarkOpenForUpdate(b, 1024, ccEncryptionKey(), false, true)
}

func BenchmarkOpenForUpdate_GOB_10MB_CHACHA20POLY1305(b *testing.B) {
	RunBenchmarkOpenForUpdate(b, 10240, ccEncryptionKey(), false, true)
}

func BenchmarkOpenForUpdate_GOB_20MB_CHACHA20POLY1305(b *testing.B) {
	RunBenchmarkOpenForUpdate(b, 20480, ccEncryptionKey(), false, true)
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
