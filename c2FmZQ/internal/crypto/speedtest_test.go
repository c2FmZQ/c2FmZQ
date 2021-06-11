package crypto

import (
	"testing"

	"c2FmZQ/internal/log"
)

func TestFastest(t *testing.T) {
	log.Level = 3
	f, err := Fastest()
	if err != nil {
		t.Fatalf("Fastest failed: %v", err)
	}
	t.Logf("Fastest: %d", f)
}
