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

// Package web implements the HTTP server API.
package web

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"net"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/crypto/acme/autocert"

	"c2FmZQ/internal/client"
	"c2FmZQ/internal/log"
	"c2FmZQ/internal/server/limit"
	"c2FmZQ/internal/stingle/token"
)

type ctxKey int

var (
	connKey   ctxKey = 1
	tokenKey  ctxKey = 2
	cookieKey ctxKey = 3
)

type Server struct {
	mux *http.ServeMux
	srv *http.Server
	c   *client.Client
}

// NewServer returns an instance of Server that's fully initialized and ready to run.
func NewServer(c *client.Client) *Server {
	s := &Server{
		mux: http.NewServeMux(),
		c:   c,
	}

	s.mux.HandleFunc(s.c.WebServerConfig.URLPrefix, s.handleIndex)
	s.mux.HandleFunc(s.c.WebServerConfig.URLPrefix+"view/", s.method("GET", s.auth(s.handleView)))
	s.mux.HandleFunc(s.c.WebServerConfig.URLPrefix+"raw/", s.method("GET", s.auth(s.handleRaw)))
	if s.c.WebServerConfig.EnableEdit {
		s.mux.HandleFunc(s.c.WebServerConfig.URLPrefix+"edit/", s.method("GET", s.auth(s.handleEdit)))
		s.mux.HandleFunc(s.c.WebServerConfig.URLPrefix+"upload/", s.method("POST", s.auth(s.handleUpload)))
	}

	return s
}

func (s *Server) httpServer() *http.Server {
	s.srv = &http.Server{
		Addr:              s.c.WebServerConfig.Address,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 30 * time.Second,
		IdleTimeout:       10 * time.Second,
		ConnContext: func(ctx context.Context, c net.Conn) context.Context {
			return context.WithValue(ctx, connKey, c)
		},
		ErrorLog: log.Logger(),
		TLSConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
	}
	return s.srv
}

// Run runs the HTTP server on the configured address.
func (s *Server) Run() error {
	if s.c.WebServerConfig.AutocertDomain == "" {
		return s.httpServer().ListenAndServe()
	}

	certManager := autocert.Manager{
		Prompt: autocert.AcceptTOS,
		Cache:  s.c.AutocertCache(),
	}
	if dom := s.c.WebServerConfig.AutocertDomain; dom != "any" && dom != "*" {
		certManager.HostPolicy = autocert.HostWhitelist(dom)
	}
	go func() {
		addr := s.c.WebServerConfig.AutocertAddress
		if addr == "" {
			addr = ":http"
		}
		log.Fatalf("autocert.Manager failed: %v", http.ListenAndServe(addr, certManager.HTTPHandler(nil)))
	}()

	srv := s.httpServer()
	srv.TLSConfig.GetCertificate = certManager.GetCertificate
	return s.srv.ListenAndServeTLS("", "")
}

// RunWithTLS runs the HTTP server with TLS.
func (s *Server) RunWithTLS(certFile, keyFile string) error {
	return s.httpServer().ListenAndServeTLS(certFile, keyFile)
}

// Shutdown cleanly shuts down the http server.
func (s *Server) Shutdown() error {
	return s.srv.Shutdown(context.Background())
}

// Handler returns the server's http.Handler. Used for testing.
func (s *Server) Handler() http.Handler {
	handler := http.Handler(s.mux)
	handler = setTag(handler)
	handler = limit.New(s.c.WebServerConfig.MaxConcurrentRequests, handler)
	return handler
}

func (s *Server) setDeadline(ctx context.Context, t time.Time) {
	c, ok := ctx.Value(connKey).(net.Conn)
	if !ok {
		log.Debugf("ctx doesn't have connKey")
		return
	}
	c.SetDeadline(t)
}

// method wraps handlers to enforce a specific method.
func (s *Server) method(method string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.Method != method {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		next(w, req)
	}
}

// auth wraps handlers that require authentication.
func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()
		s.setDeadline(ctx, time.Now().Add(30*time.Second))
		defer s.setDeadline(ctx, time.Time{})

		q := req.URL.Query()
		tok, err := token.Decrypt(s.c.WebServerConfig.TokenKey, q.Get("tok"))
		if err != nil || tok.File != tagFromCtx(ctx) {
			http.Redirect(w, req, s.c.WebServerConfig.URLPrefix+"?redir="+url.QueryEscape(req.URL.Path), http.StatusFound)
			return
		}
		ctx = context.WithValue(ctx, tokenKey, tok)

		if s.c.WebServerConfig.AllowCaching {
			w.Header().Set("Cache-Control", "private")
		} else {
			w.Header().Set("Cache-Control", "no-store")
		}

		next(w, req.WithContext(ctx))
	}
}

func tagFromCtx(ctx context.Context) string {
	if c, ok := ctx.Value(cookieKey).(*http.Cookie); ok {
		return c.Value
	}
	return ""
}

// setTag creates or refreshes the TAG cookie.
func setTag(next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		ctx := req.Context()

		cookie, err := req.Cookie("TAG")
		if err != nil {
			b := make([]byte, 8)
			if _, err := rand.Read(b); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			cookie = &http.Cookie{
				Name:  "TAG",
				Value: hex.EncodeToString(b),
			}
		}
		cookie.Expires = time.Now().Add(24 * time.Hour)
		cookie.Path = "/"
		cookie.SameSite = http.SameSiteStrictMode
		cookie.HttpOnly = true
		http.SetCookie(w, cookie)
		ctx = context.WithValue(ctx, cookieKey, cookie)

		next.ServeHTTP(w, req.WithContext(ctx))
	}
}
