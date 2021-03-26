// Package server implements the Stingle server API.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"

	"github.com/NYTimes/gziphandler"
	"stingle-server/crypto"
	"stingle-server/database"
	"stingle-server/log"
	"stingle-server/stingle"
)

// An HTTP server that implements the Stingle server API.
type Server struct {
	BaseURL string
	mux     *http.ServeMux
	srv     *http.Server
	db      *database.Database
	addr    string
}

// New returns an instance of Server that's fully initialized and ready to run.
func New(db *database.Database, addr string) *Server {
	s := &Server{
		mux:  http.NewServeMux(),
		db:   db,
		addr: addr,
	}

	s.mux.HandleFunc("/", s.handleNotFound)
	s.mux.HandleFunc("/v2/register/createAccount", s.noauth(s.handleCreateAccount))
	s.mux.HandleFunc("/v2/login/preLogin", s.noauth(s.handlePreLogin))
	s.mux.HandleFunc("/v2/login/login", s.noauth(s.handleLogin))
	s.mux.HandleFunc("/v2/login/logout", s.auth(s.handleLogout))
	s.mux.HandleFunc("/v2/login/changePass", s.auth(s.handleChangePass))
	s.mux.HandleFunc("/v2/login/checkKey", s.noauth(s.handleCheckKey))
	s.mux.HandleFunc("/v2/login/recoverAccount", s.noauth(s.handleRecoverAccount))
	s.mux.HandleFunc("/v2/login/deleteUser", s.auth(s.handleDeleteUser))
	s.mux.HandleFunc("/v2/keys/getServerPK", s.auth(s.handleGetServerPK))
	s.mux.HandleFunc("/v2/keys/reuploadKeys", s.auth(s.handleReuploadKeys))

	s.mux.HandleFunc("/v2/sync/getUpdates", s.auth(s.handleGetUpdates))
	s.mux.HandleFunc("/v2/sync/upload", s.noauth(s.handleUpload))
	s.mux.HandleFunc("/v2/sync/moveFile", s.auth(s.handleMoveFile))
	s.mux.HandleFunc("/v2/sync/emptyTrash", s.auth(s.handleEmptyTrash))
	s.mux.HandleFunc("/v2/sync/delete", s.auth(s.handleDelete))
	s.mux.HandleFunc("/v2/sync/download", s.handleDownload)
	s.mux.HandleFunc("/v2/signedDownload/", s.handleSignedDownload)
	s.mux.HandleFunc("/v2/sync/getDownloadUrls", s.auth(s.handleGetDownloadUrls))
	s.mux.HandleFunc("/v2/sync/getUrl", s.auth(s.handleGetURL))

	s.mux.HandleFunc("/v2/sync/addAlbum", s.auth(s.handleAddAlbum))
	s.mux.HandleFunc("/v2/sync/deleteAlbum", s.auth(s.handleDeleteAlbum))
	s.mux.HandleFunc("/v2/sync/changeAlbumCover", s.auth(s.handleChangeAlbumCover))
	s.mux.HandleFunc("/v2/sync/renameAlbum", s.auth(s.handleRenameAlbum))
	s.mux.HandleFunc("/v2/sync/getContact", s.auth(s.handleGetContact))
	s.mux.HandleFunc("/v2/sync/share", s.auth(s.handleShare))
	s.mux.HandleFunc("/v2/sync/editPerms", s.auth(s.handleEditPerms))
	s.mux.HandleFunc("/v2/sync/removeAlbumMember", s.auth(s.handleRemoveAlbumMember))
	s.mux.HandleFunc("/v2/sync/unshareAlbum", s.auth(s.handleUnshareAlbum))
	s.mux.HandleFunc("/v2/sync/leaveAlbum", s.auth(s.handleLeaveAlbum))

	return s
}

// Run runs the HTTP server on the configured address.
func (s *Server) Run() error {
	s.srv = &http.Server{
		Addr:    s.addr,
		Handler: gziphandler.GzipHandler(s.mux),
	}
	return s.srv.ListenAndServe()
}

// RunWithTLS runs the HTTP server with TLS.
func (s *Server) RunWithTLS(certFile, keyFile string) error {
	s.srv = &http.Server{
		Addr:    s.addr,
		Handler: gziphandler.GzipHandler(s.mux),
	}
	return s.srv.ListenAndServeTLS(certFile, keyFile)
}

// RunWithListener runs the server using a pre-existing Listener. Used for testing.
func (s *Server) RunWithListener(l net.Listener) error {
	s.srv = &http.Server{
		Addr:    s.addr,
		Handler: gziphandler.GzipHandler(s.mux),
	}
	return s.srv.Serve(l)
}

// Shutdown cleanly shutdowns the http server.
func (s *Server) Shutdown() error {
	return s.srv.Shutdown(context.Background())
}

// decodeParams decodes the params value that's parsed to most API endpoints.
// It is an encrypted json object representing key:value pairs.
// Returns the decrypted key:value pairs as a map.
func (s *Server) decodeParams(params string, user database.User) (map[string]string, error) {
	m, err := crypto.DecryptMessage(params, user.PublicKey, user.ServerKey)
	if err != nil {
		return nil, err
	}
	var p map[string]string
	if err := json.Unmarshal(m, &p); err != nil {
		return nil, err
	}
	log.Infof("Params: %#v", p)
	return p, nil
}

// parseInt converts a string to int64, mapping any errors to a default return
// value.
func parseInt(s string, def int64) int64 {
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return def
	}
	return v
}

// noauth wraps handlers that don't require authentication.
func (s *Server) noauth(f func(*http.Request) *stingle.Response) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		log.Infof("%s %s", req.Method, req.URL)
		req.ParseForm()
		if err := f(req).Send(w); err != nil {
			log.Errorf("Send: %v", err)
		}
	}
}

// checkToken validates the signed token that was given to the client when it
// logged in. The client presents this token with most API requests.
// Returns the decoded token, and the authenticated user.
func (s *Server) checkToken(tok, scope string) (crypto.Token, database.User, error) {
	token, err := crypto.DecodeToken(tok)
	if err != nil {
		return crypto.Token{}, database.User{}, err
	}
	user, err := s.db.User(token.Subject)
	if err != nil {
		return token, database.User{}, err
	}
	if err := crypto.ValidateToken(user.ServerSignKey, token); err != nil {
		return token, database.User{}, err
	}
	if token.Seq != user.TokenSeq {
		return token, user, fmt.Errorf("unexpected token Seq (%d != %d)", token.Seq, user.TokenSeq)
	}
	if token.Scope != scope {
		return token, user, fmt.Errorf("unexpected token Scope (%q != %q)", token.Scope, scope)
	}
	return token, user, nil
}

// auth wraps handlers that require authentication, checking the token, and
// passing the authenticated user to the underlying handler.
func (s *Server) auth(f func(database.User, *http.Request) *stingle.Response) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		req.ParseForm()

		_, user, err := s.checkToken(req.PostFormValue("token"), "session")
		if err != nil {
			log.Errorf("%s %s (INVALID TOKEN: %v)", req.Method, req.URL, err)
			if err := stingle.ResponseOK().AddPart("logout", "1").Send(w); err != nil {
				log.Errorf("Send: %v", err)
			}
			return
		}
		log.Infof("%s %s (UserID:%d)", req.Method, req.URL, user.UserID)
		if err := f(user, req).Send(w); err != nil {
			log.Errorf("Send: %v", err)
		}
	}
}

// handleNotFound handles requests for undefined endpoints.
func (s *Server) handleNotFound(w http.ResponseWriter, req *http.Request) {
	log.Infof("!!! (404) %s %s", req.Method, req.URL)
	if log.Level >= log.DebugLevel {
		req.ParseForm()
		if req.PostForm != nil {
			for k, v := range req.PostForm {
				log.Debugf("> %s: %v", k, v)
			}
		}
	}
	w.WriteHeader(http.StatusNotFound)
}
