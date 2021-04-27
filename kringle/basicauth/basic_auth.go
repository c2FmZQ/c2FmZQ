package basicauth

import (
	"bytes"
	"crypto/md5"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"os"

	"kringle/log"
)

// New returns an initialized BasicAuth that uses the given htdigest file.
func New(filename string) (*BasicAuth, error) {
	basicAuth := &BasicAuth{
		htDigest: make(map[string][md5.Size]byte),
	}
	b, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	for i, line := range bytes.Split(b, []byte{'\n'}) {
		if len(line) == 0 {
			continue
		}
		parts := bytes.SplitN(line, []byte{':'}, 3)
		if len(parts) != 3 {
			log.Errorf("basic-auth: malformed line %s:%d", filename, i)
			continue
		}
		var pass [md5.Size]byte
		if sz, err := hex.Decode(pass[:], parts[2]); err != nil || sz != md5.Size {
			log.Errorf("basic-auth: malformed md5 hash %s:%d", filename, i)
			continue
		}
		key := string(bytes.Join(parts[:2], []byte{':'}))
		basicAuth.htDigest[key] = pass
	}
	return basicAuth, nil
}

// Handles basic auth for HTTP handlers using a htdigest file.
type BasicAuth struct {
	// key is user:realm, value is md5 password.
	htDigest map[string][md5.Size]byte
}

// Check checks the user's password using the preloaded htdigest file.
func (a *BasicAuth) Check(user, pass, realm string) bool {
	key := user + ":" + realm
	h := md5.Sum([]byte(key + ":" + pass))
	if p, ok := a.htDigest[key]; !ok || subtle.ConstantTimeCompare(h[:], p[:]) != 1 {
		return false
	}
	return true

}

// HandlerFunc wraps a http.HandlerFunc to require Basic Auth.
func (a *BasicAuth) HandlerFunc(realm string, h http.HandlerFunc) http.HandlerFunc {
	return a.Handler(realm, http.HandlerFunc(h))
}

// HandlerFunc wraps a http.Handler to require Basic Auth.
func (a *BasicAuth) Handler(realm string, h http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		user, pass, ok := req.BasicAuth()
		if !ok || !a.Check(user, pass, realm) {
			w.Header().Set("WWW-Authenticate", "Basic realm=\""+realm+"\"")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		h.ServeHTTP(w, req)
	}
}
