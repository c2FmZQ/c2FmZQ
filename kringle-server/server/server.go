// Package server implements the Stingle server API.
package server

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strconv"

	"github.com/NYTimes/gziphandler"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"kringle-server/basicauth"
	"kringle-server/database"
	"kringle-server/log"
	"kringle-server/stingle"
	"kringle-server/stingle/token"
)

var (
	reqLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "server_response_time",
			Help:    "The server's response time",
			Buckets: []float64{0.01, 0.05, 0.1, 0.2, 0.3, 0.4, 0.5, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 20, 30, 45, 60, 90, 120},
		},
		[]string{"method", "uri"},
	)
	reqStatus = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "server_response_status_total",
			Help: "Number of requests",
		},
		[]string{"method", "uri", "status"},
	)
	reqSize = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "server_request_size",
			Help:    "The size of requests",
			Buckets: prometheus.ExponentialBuckets(1, 2, 32),
		},
		[]string{"code"},
	)
	respSize = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "server_response_size",
			Help:    "The size of responses",
			Buckets: prometheus.ExponentialBuckets(1, 2, 32),
		},
		[]string{"code"},
	)
)

func init() {
	prometheus.MustRegister(reqLatency)
	prometheus.MustRegister(reqStatus)
	prometheus.MustRegister(reqSize)
	prometheus.MustRegister(respSize)
}

// An HTTP server that implements the Stingle server API.
type Server struct {
	AllowCreateAccount bool
	BaseURL            string
	mux                *http.ServeMux
	srv                *http.Server
	db                 *database.Database
	addr               string
	basicAuth          *basicauth.BasicAuth
}

// New returns an instance of Server that's fully initialized and ready to run.
func New(db *database.Database, addr, htdigest string) *Server {
	s := &Server{
		mux:  http.NewServeMux(),
		db:   db,
		addr: addr,
	}
	if htdigest != "" {
		var err error
		if s.basicAuth, err = basicauth.New(htdigest); err != nil {
			log.Errorf("htdigest: %v", err)
		}
	}
	if s.basicAuth != nil {
		s.mux.HandleFunc("/metrics", s.basicAuth.Handler("Metrics", promhttp.Handler()))
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
	s.mux.HandleFunc("/v2/download/", s.handleTokenDownload)
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

func (s *Server) wrapHandler() http.Handler {
	handler := http.Handler(s.mux)
	handler = gziphandler.GzipHandler(handler)
	handler = promhttp.InstrumentHandlerRequestSize(reqSize, handler)
	handler = promhttp.InstrumentHandlerResponseSize(respSize, handler)
	return handler
}

// Run runs the HTTP server on the configured address.
func (s *Server) Run() error {
	s.srv = &http.Server{
		Addr:    s.addr,
		Handler: s.wrapHandler(),
	}
	return s.srv.ListenAndServe()
}

// RunWithTLS runs the HTTP server with TLS.
func (s *Server) RunWithTLS(certFile, keyFile string) error {
	s.srv = &http.Server{
		Addr:    s.addr,
		Handler: s.wrapHandler(),
	}
	return s.srv.ListenAndServeTLS(certFile, keyFile)
}

// RunWithListener runs the server using a pre-existing Listener. Used for testing.
func (s *Server) RunWithListener(l net.Listener) error {
	s.srv = &http.Server{
		Addr:    s.addr,
		Handler: s.wrapHandler(),
	}
	return s.srv.Serve(l)
}

// Shutdown cleanly shuts down the http server.
func (s *Server) Shutdown() error {
	return s.srv.Shutdown(context.Background())
}

// decodeParams decodes the params value that's parsed to most API endpoints.
// It is an encrypted json object representing key:value pairs.
// Returns the decrypted key:value pairs as a map.
func (s *Server) decodeParams(params string, user database.User) (map[string]string, error) {
	m, err := stingle.DecryptMessage(params, user.PublicKey, user.ServerKey)
	if err != nil {
		return nil, err
	}
	var p map[string]string
	if err := json.Unmarshal(m, &p); err != nil {
		return nil, err
	}
	log.Debugf("Params: %#v", p)
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
		timer := prometheus.NewTimer(reqLatency.WithLabelValues(req.Method, req.URL.String()))
		defer timer.ObserveDuration()
		log.Infof("%s %s", req.Method, req.URL)
		req.ParseForm()
		sr := f(req)
		if err := sr.Send(w); err != nil {
			log.Errorf("Send: %v", err)
		}
		reqStatus.WithLabelValues(req.Method, req.URL.String(), sr.Status).Inc()
	}
}

// checkToken validates the signed token that was given to the client when it
// logged in. The client presents this token with most API requests.
// Returns the decoded token, and the authenticated user.
func (s *Server) checkToken(tok, scope string) (token.Token, database.User, error) {
	id, err := token.Subject(tok)
	if err != nil {
		return token.Token{}, database.User{}, err
	}
	user, err := s.db.UserByID(id)
	if err != nil {
		return token.Token{}, database.User{}, err
	}
	t, err := token.Decrypt(user.TokenKey, tok)
	if err != nil {
		return token.Token{}, database.User{}, err
	}
	if t.Scope != scope {
		return token.Token{}, database.User{}, token.ErrValidationFailed
	}
	return t, user, nil
}

// auth wraps handlers that require authentication, checking the token, and
// passing the authenticated user to the underlying handler.
func (s *Server) auth(f func(database.User, *http.Request) *stingle.Response) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		timer := prometheus.NewTimer(reqLatency.WithLabelValues(req.Method, req.URL.String()))
		defer timer.ObserveDuration()

		req.ParseForm()

		_, user, err := s.checkToken(req.PostFormValue("token"), "session")
		if err != nil {
			log.Errorf("%s %s (INVALID TOKEN: %v)", req.Method, req.URL, err)
			sr := stingle.ResponseNOK().AddPart("logout", "1").AddError("You are not logged in")
			if err := sr.Send(w); err != nil {
				log.Errorf("Send: %v", err)
			}
			return
		}
		log.Infof("%s %s (UserID:%d)", req.Method, req.URL, user.UserID)
		sr := f(user, req)
		if err := sr.Send(w); err != nil {
			log.Errorf("Send: %v", err)
		}
		reqStatus.WithLabelValues(req.Method, req.URL.String(), sr.Status).Inc()
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
