package limit_test

import (
	"testing"

	"c2FmZQ/internal/server/limit"
)

func TestTicket(t *testing.T) {
	l := limit.New(1, nil)

	var ch []<-chan struct{}
	for i := 0; i < 5; i++ {
		c, err := l.Ticket()
		if err != nil {
			t.Fatalf("Ticket failed: %v", err)
		}
		ch = append(ch, c)
	}

	var exp [5]bool
	for i := 0; i < 5; i++ {
		exp[i] = true
		t.Logf("Loop %d - Exp %v", i, exp)
		for j := range ch {
			if got, want := ready(ch[j]), exp[j]; got != want {
				t.Errorf("%d: ready(ch[%d]) Got %v, want %v", i, j, got, want)
			}
		}
		l.Done()
	}
}

func ready(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}
