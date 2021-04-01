package basicauth

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func encodeHTDigest(user, pass, realm string) string {
	hash := md5.Sum([]byte(user + ":" + realm + ":" + pass))
	return fmt.Sprintf("%s:%s:%s\n", user, realm, hex.EncodeToString(hash[:]))
}

func TestBasicAuth(t *testing.T) {
	testdir := t.TempDir()
	htdigest := filepath.Join(testdir, "htdigest")

	content := encodeHTDigest("foo", "bar", "World") + encodeHTDigest("foo", "bork", "Town")
	if err := os.WriteFile(htdigest, []byte(content), 0600); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}

	ba, err := New(htdigest)
	if err != nil {
		t.Fatalf("newBasicAuth: %v", err)
	}

	if want, got := true, ba.Check("foo", "bar", "World"); want != got {
		t.Errorf("ba.Check(foo, bar, World) returned unexpected result. Want %v, got %v", want, got)
	}
	if want, got := true, ba.Check("foo", "bork", "Town"); want != got {
		t.Errorf("ba.Check(foo, bar, World) returned unexpected result. Want %v, got %v", want, got)
	}

	if want, got := false, ba.Check("foo", "bar", "Town"); want != got {
		t.Errorf("ba.Check(foo, bar, World) returned unexpected result. Want %v, got %v", want, got)
	}
	if want, got := false, ba.Check("foo", "bork", "World"); want != got {
		t.Errorf("ba.Check(foo, bar, World) returned unexpected result. Want %v, got %v", want, got)
	}
	if want, got := false, ba.Check("food", "bar", "World"); want != got {
		t.Errorf("ba.Check(foo, bar, World) returned unexpected result. Want %v, got %v", want, got)
	}
	if want, got := false, ba.Check("", "", ""); want != got {
		t.Errorf("ba.Check(foo, bar, World) returned unexpected result. Want %v, got %v", want, got)
	}
}
