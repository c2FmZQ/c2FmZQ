// Package limit implements a mechamism to limit the number of concurrent
// connections.
package limit

import (
	"container/list"
	"errors"
	"net/http"
	"strings"
	"sync"

	"c2FmZQ/internal/log"
)

// ConnLimiter implements a connection limiter.
type ConnLimiter struct {
	maxInQueue  int
	maxInFlight int
	next        http.Handler

	mu       sync.Mutex
	queue    *list.List
	inFlight int
}

// New returns a new http.Handler that limits connections to the given number of
// concurrent requests  before passing the request the next http.Handler.
//
// The new http.Handler also limit the number of connections in queue at any
// time to 50 times the maximum number of concurrent requests.
func New(max int, next http.Handler) *ConnLimiter {
	return &ConnLimiter{
		maxInQueue:  max * 50,
		maxInFlight: max,
		next:        next,
		queue:       list.New(),
	}
}

// Ticket returns a channel that will become ready when it is the caller's turn
// to proceed, or an error if there are too many connections in the queue.
func (c *ConnLimiter) Ticket() (<-chan struct{}, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.queue.Len() >= c.maxInQueue {
		return nil, errors.New("too many connections")
	}
	ch := make(chan struct{})
	if c.inFlight < c.maxInFlight {
		close(ch)
		c.inFlight++
	} else {
		c.queue.PushBack(ch)
	}
	return ch, nil
}

// Done must be called when Ticket returned successfully and the caller is done
// executing.
func (c *ConnLimiter) Done() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e := c.queue.Front(); e != nil {
		close(c.queue.Remove(e).(chan struct{}))
	} else {
		c.inFlight--
		if c.inFlight < 0 {
			log.Fatalf("inFlight = %d", c.inFlight)
		}
	}
}

// ServeHTTP handles an HTTP request.
func (c *ConnLimiter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasSuffix(r.URL.Path, "/metrics") {
		c.next.ServeHTTP(w, r)
		return
	}
	ready, err := c.Ticket()
	if err != nil {
		log.Debugf("Too Many Requests: %s %.10s...", r.Method, r.URL)
		w.Header().Set("Retry-After", "30")
		http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
		return
	}
	defer c.Done()
	select {
	case <-r.Context().Done():
		return
	case <-ready:
		c.next.ServeHTTP(w, r)
	}
}
