package fuse_test

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/go-test/deep"

	"c2FmZQ/internal/client"
	"c2FmZQ/internal/client/fuse"
	"c2FmZQ/internal/crypto"
	"c2FmZQ/internal/log"
	"c2FmZQ/internal/secure"
)

func TestFuse(t *testing.T) {
	log.Level = 2
	c, err := newClient(t.TempDir())
	if err != nil {
		t.Fatalf("newClient: %v", err)
	}
	fuseDir := t.TempDir()
	go func() {
		if err := fuse.Mount(c, fuseDir); err != nil {
			t.Fatalf("fuse.Mount(%q): %v", fuseDir, err)
		}
	}()
	var unmount func()
	unmount = func() {
		if err := fuse.Unmount(fuseDir); err != nil {
			t.Fatalf("fuse.Unmount(%q): %v", fuseDir, err)
		}
		unmount = nil
	}
	defer func() {
		if unmount != nil {
			unmount()
		}
	}()
	time.Sleep(1 * time.Second)

	galleryDir := filepath.Join(fuseDir, "gallery")
	if err := run("echo 'Hello World!' > %s", filepath.Join(galleryDir, "hello.txt")); err != nil {
		t.Fatalf("echo: %v", err)
	}
	if err := run("echo 'Hello World!' > %s", filepath.Join(galleryDir, "hello.txt")); err == nil {
		t.Fatal("echo succeeded unexpectedly")
	}
	fooDir := filepath.Join(fuseDir, "foo")
	if err := os.Mkdir(fooDir, 0700); err != nil {
		t.Fatalf("Mkdir(%q): %v", fooDir, err)
	}
	if err := makeImages(fooDir, 0, 10, false); err != nil {
		t.Fatalf("makeImages(%q, false): %v", fooDir, err)
	}
	if err := makeImages(fooDir, 10, 10, true); err != nil {
		t.Fatalf("makeImages(%q, true): %v", fooDir, err)
	}
	barDir := filepath.Join(fuseDir, "bar")
	if err := run("cp -r '%s' '%s'", fooDir, barDir); err != nil {
		t.Fatalf("cp -r: %v", err)
	}
	bazDir := filepath.Join(fuseDir, "baz")
	if err := os.Mkdir(bazDir, 0700); err != nil {
		t.Fatalf("Mkdir(%q): %v", bazDir, err)
	}
	if err := run("tar -C '%s' -cf - . | tar -C '%s' -xvf -", fooDir, bazDir); err != nil {
		t.Fatalf("tar: %v", err)
	}
	bizDir := filepath.Join(fuseDir, "biz")
	// Fuse doesn't seem to like the utimensat / futimens system calls.
	if err := run("rsync -avh --no-times '%s/' '%s'", fooDir, bizDir); err != nil {
		t.Errorf("rsync: %v", err)
	}

	if err := run("ls -alR %s", fuseDir); err != nil {
		t.Fatalf("ls -alR: %v", err)
	}

	// Blocks until ongoing mutations are finished.
	unmount()

	got, err := glob(c)
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	want := []string{
		".trash 0",
		"bar 0",
		"bar/image000.jpg 597",
		"bar/image001.jpg 597",
		"bar/image002.jpg 597",
		"bar/image003.jpg 597",
		"bar/image004.jpg 597",
		"bar/image005.jpg 597",
		"bar/image006.jpg 597",
		"bar/image007.jpg 597",
		"bar/image008.jpg 597",
		"bar/image009.jpg 597",
		"bar/image010.jpg 597",
		"bar/image011.jpg 597",
		"bar/image012.jpg 597",
		"bar/image013.jpg 597",
		"bar/image014.jpg 597",
		"bar/image015.jpg 597",
		"bar/image016.jpg 597",
		"bar/image017.jpg 597",
		"bar/image018.jpg 597",
		"bar/image019.jpg 597",
		"baz 0",
		"baz/image000.jpg 597",
		"baz/image001.jpg 597",
		"baz/image002.jpg 597",
		"baz/image003.jpg 597",
		"baz/image004.jpg 597",
		"baz/image005.jpg 597",
		"baz/image006.jpg 597",
		"baz/image007.jpg 597",
		"baz/image008.jpg 597",
		"baz/image009.jpg 597",
		"baz/image010.jpg 597",
		"baz/image011.jpg 597",
		"baz/image012.jpg 597",
		"baz/image013.jpg 597",
		"baz/image014.jpg 597",
		"baz/image015.jpg 597",
		"baz/image016.jpg 597",
		"baz/image017.jpg 597",
		"baz/image018.jpg 597",
		"baz/image019.jpg 597",
		"biz 0",
		"biz/image000.jpg 597",
		"biz/image001.jpg 597",
		"biz/image002.jpg 597",
		"biz/image003.jpg 597",
		"biz/image004.jpg 597",
		"biz/image005.jpg 597",
		"biz/image006.jpg 597",
		"biz/image007.jpg 597",
		"biz/image008.jpg 597",
		"biz/image009.jpg 597",
		"biz/image010.jpg 597",
		"biz/image011.jpg 597",
		"biz/image012.jpg 597",
		"biz/image013.jpg 597",
		"biz/image014.jpg 597",
		"biz/image015.jpg 597",
		"biz/image016.jpg 597",
		"biz/image017.jpg 597",
		"biz/image018.jpg 597",
		"biz/image019.jpg 597",
		"foo 0",
		"foo/image000.jpg 597",
		"foo/image001.jpg 597",
		"foo/image002.jpg 597",
		"foo/image003.jpg 597",
		"foo/image004.jpg 597",
		"foo/image005.jpg 597",
		"foo/image006.jpg 597",
		"foo/image007.jpg 597",
		"foo/image008.jpg 597",
		"foo/image009.jpg 597",
		"foo/image010.jpg 597",
		"foo/image011.jpg 597",
		"foo/image012.jpg 597",
		"foo/image013.jpg 597",
		"foo/image014.jpg 597",
		"foo/image015.jpg 597",
		"foo/image016.jpg 597",
		"foo/image017.jpg 597",
		"foo/image018.jpg 597",
		"foo/image019.jpg 597",
		"gallery 0",
		"gallery/hello.txt 13",
	}
	if diff := deep.Equal(want, got); diff != nil {
		t.Errorf("Unexpected file list. Want %#v, got %#v, diff %v", want, got, diff)
	}
}

func newClient(dir string) (*client.Client, error) {
	masterKey, err := crypto.CreateMasterKey()
	if err != nil {
		return nil, err
	}
	storage := secure.NewStorage(dir, &masterKey.EncryptionKey)
	c, err := client.Create(masterKey, storage)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func makeImages(dir string, start, n int, useRename bool) error {
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	for i := start; i < start+n; i++ {
		fn := fmt.Sprintf("image%03d.jpg", i)
		tmp := fn
		if useRename {
			tmp = tmpName(fn)
		}
		f, err := os.Create(filepath.Join(dir, tmp))
		if err != nil {
			return err
		}
		if err := jpeg.Encode(f, img, &jpeg.Options{Quality: 70}); err != nil {
			return err
		}
		if useRename {
			if err := os.Rename(filepath.Join(dir, tmp), filepath.Join(dir, fn)); err != nil {
				return err
			}
		}
		if err := f.Close(); err != nil {
			return err
		}
	}
	return nil
}

func tmpName(s string) string {
	b := "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	out := []byte(fmt.Sprintf(".%s.", s))
	for i := 0; i < 6; i++ {
		out = append(out, b[rand.Intn(len(b))])
	}
	return string(out)
}

func run(format string, args ...interface{}) error {
	c := fmt.Sprintf(format, args...)
	fmt.Printf("Running: %s\n", c)
	cmd := exec.Command("/bin/bash", "-c", c)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	fmt.Printf("OUTPUT: %s\n", buf.String())
	return err
}

func glob(c *client.Client) ([]string, error) {
	var out []string
	li, err := c.GlobFiles([]string{"*"}, client.GlobOptions{MatchDot: true, Recursive: true})
	if err != nil {
		return nil, err
	}
	var list []string
	for _, item := range li {
		list = append(list, fmt.Sprintf("%s %d", item.Filename, item.Size))
	}
	sort.Strings(list)
	out = append(out, list...)
	return out, nil
}
