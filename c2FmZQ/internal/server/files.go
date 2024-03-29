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

package server

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"c2FmZQ/internal/database"
	"c2FmZQ/internal/log"
	"c2FmZQ/internal/stingle"
	"c2FmZQ/internal/stingle/token"
)

// handleUpload handles the /v2/sync/upload endpoint. It is used to upload
// new files. The incoming request is a multipart/form-data with two files:
// one for the image or video, and one for the thumbnail.
//
// Arguments:
//   - req: The http request.
//
// Form arguments
//   - token: The signed session token.
//   - headers: File metadata (encrypted key, etc)
//   - set: The file set where this file is being uploaded.
//   - albumId: The ID of the album where the file is being uploaded.
//   - dateCreated: A timestamp in milliseconds.
//   - dateModified: A timestamp in milliseconds.
//   - version: The file format version (opaque to the server).
//
// Returns:
//   - stingle.Response("ok")
func (s *Server) handleUpload(w http.ResponseWriter, req *http.Request) {
	up, err := s.receiveUpload("uploads", req)
	s.setDeadline(req.Context(), time.Now().Add(30*time.Second))
	if err != nil {
		log.Errorf("handleUpload: receiveUpload failed: %v", err)
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}
	_, user, err := s.checkToken(up.token, "session")
	if err != nil || !user.ValidTokens[token.Hash(up.token)] {
		log.Errorf("handleUpload: checkToken failed: %v", err)
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}
	log.Infof("%s %s %s (UserID:%d)", req.Proto, req.Method, req.URL, user.UserID)
	if user.NeedApproval {
		http.Error(w, "Account is not approved yet", http.StatusForbidden)
		return
	}

	if up.set == stingle.AlbumSet {
		albumSpec, err := s.db.Album(user, up.albumID)
		if err != nil {
			log.Errorf("db.Album(%q, %q) failed: %v", user.Email, up.albumID, err)
			http.Error(w, "Internal Error", http.StatusInternalServerError)
			return
		}
		if albumSpec.OwnerID != user.UserID && !albumSpec.Permissions.AllowAdd() {
			log.Error("handleUpload: permission denied on album")
			http.Error(w, "Adding to this album is not permitted", http.StatusForbidden)
			return
		}
	}

	if err := s.db.AddFile(user, up.FileSpec, up.name, up.set, up.albumID); err != nil {
		log.Errorf("AddFile: %v", err)
		if err == database.ErrQuotaExceeded {
			http.Error(w, "Quota exceeded", http.StatusForbidden)
			return
		}
		http.Error(w, "Internal Error", http.StatusInternalServerError)
		return
	}
	stingle.ResponseOK().Send(w)
}

// handleMoveFile handles the /v2/sync/moveFile endpoint. It is used to move
// or copy files between filesets/albums.
//
// Arguments:
//   - user: The authenticated user.
//   - req: The http request.
//
// Form arguments
//   - token: The signed session token.
//   - params: The encrypted parameters
//   - setFrom: The set from which the files are moving (or being copied)
//   - setTo: The set to which the files are moving (or being copied)
//   - albumIdFrom: The ID of the album from which the files are moving, or ""
//     if moving from Trash or Gallery.
//   - albumIdTo: The ID of the album to which the files are moving, or "" if
//     moving to Trash or Gallery.
//   - isMoving: "0" if the files are being copied, "1" if they are moving.
//   - count: The number of files being copied or moved.
//   - filename<int>: The filenames affected (filename0, filename1, etc)
//   - headers<int>: The file headers, present only if the headers are
//     changing, i.e. when moving to/from albums.
//
// Returns:
//   - stingle.Response(ok)
func (s *Server) handleMoveFile(user database.User, req *http.Request) *stingle.Response {
	params, err := s.decodeParams(req.PostFormValue("params"), user)
	if err != nil {
		log.Errorf("decodeParams: %v", err)
		return stingle.ResponseNOK()
	}

	p := database.MoveFileParams{
		SetFrom:     params["setFrom"],
		SetTo:       params["setTo"],
		AlbumIDFrom: params["albumIdFrom"],
		AlbumIDTo:   params["albumIdTo"],
		IsMoving:    params["isMoving"] == "1",
	}
	count := parseInt(params["count"], 0)
	if count > 100000 {
		return stingle.ResponseNOK().AddError("Too many files")
	}

	for i := int64(0); i < count; i++ {
		filename := params[fmt.Sprintf("filename%d", i)]
		p.Filenames = append(p.Filenames, filename)
		hdr := params[fmt.Sprintf("headers%d", i)]
		if hdr != "" {
			p.Headers = append(p.Headers, hdr)
		}
	}
	if p.SetFrom == stingle.TrashSet {
		if p.SetTo != stingle.GallerySet || !p.IsMoving {
			return stingle.ResponseNOK().AddError("Can only move from trash to gallery")
		}
	}
	if p.SetTo == stingle.TrashSet {
		if !p.IsMoving {
			return stingle.ResponseNOK().AddError("Can only move to trash, not copy")
		}
	}
	if p.AlbumIDFrom != "" {
		albumSpec, err := s.db.Album(user, p.AlbumIDFrom)
		if err != nil {
			log.Errorf("db.Album(%q, %q) failed: %v", user.Email, p.AlbumIDFrom, err)
			return stingle.ResponseNOK()
		}
		if albumSpec.OwnerID != user.UserID && !albumSpec.Permissions.AllowCopy() {
			return stingle.ResponseNOK().AddError("Copying from this album is not permitted")
		}
		if albumSpec.OwnerID != user.UserID && p.IsMoving {
			return stingle.ResponseNOK().AddError("Removing items from this album is not permitted")
		}
	}
	if p.AlbumIDTo != "" {
		albumSpec, err := s.db.Album(user, p.AlbumIDTo)
		if err != nil {
			log.Errorf("db.Album(%q, %q) failed: %v", user.Email, p.AlbumIDTo, err)
			return stingle.ResponseNOK()
		}
		if albumSpec.OwnerID != user.UserID && !albumSpec.Permissions.AllowAdd() {
			return stingle.ResponseNOK().AddError("Adding to this album is not permitted")
		}
	}

	if err := s.db.MoveFile(user, p); err != nil {
		log.Errorf("MoveFile(%+v): %v", p, err)
		if err == database.ErrQuotaExceeded {
			return stingle.ResponseNOK().AddError("Quota exceeded")
		}
		return stingle.ResponseNOK()
	}
	return stingle.ResponseOK()
}

// handleEmptyTrash handles the /v2/sync/emptyTrash endpoint. It is used to
// delete all the files in the Trash set.
//
// Arguments:
//   - user: The authenticated user.
//   - req: The http request
//
// Form arguments
//   - params: The encrypted parameters
//   - time: A timestamp in milliseconds. All files added until that time
//     should be removed.
//
// Returns:
//   - stingle.Response(ok)
func (s *Server) handleEmptyTrash(user database.User, req *http.Request) *stingle.Response {
	params, err := s.decodeParams(req.PostFormValue("params"), user)
	if err != nil {
		log.Errorf("decodeParams: %v", err)
		return stingle.ResponseNOK()
	}
	if err := s.db.EmptyTrash(user, parseInt(params["time"], 0)); err != nil {
		log.Errorf("EmptyTrash: %v", err)
		return stingle.ResponseNOK()
	}
	return stingle.ResponseOK()
}

// handleDelete handles the /v2/sync/delete endpoint. It is used to delete some
// the files in the Trash set.
//
// Arguments:
//   - user: The authenticated user.
//   - req: The http request
//
// Form arguments
//   - params: The encrypted parameters
//   - count: The number of files being deleted.
//   - filename<int>: The filenames being deleted.
//
// Returns:
//   - stingle.Response(ok)
func (s *Server) handleDelete(user database.User, req *http.Request) *stingle.Response {
	params, err := s.decodeParams(req.PostFormValue("params"), user)
	if err != nil {
		log.Errorf("decodeParams: %v", err)
		return stingle.ResponseNOK()
	}
	count := parseInt(params["count"], 0)
	if count > 100000 {
		stingle.ResponseNOK().AddError("Too many files")
	}
	files := []string{}
	for i := int64(0); i < count; i++ {
		files = append(files, params[fmt.Sprintf("filename%d", i)])
	}
	if err := s.db.DeleteFiles(user, files); err != nil {
		log.Errorf("DeleteFiles: %v", err)
		return stingle.ResponseNOK()
	}
	return stingle.ResponseOK()
}

// handleDownload handles the /v2/sync/download endpoint. It is used to download
// the content of a file.
//
// Arguments:
//   - user: The authenticated user.
//   - req: The http request
//
// Form arguments
//   - file: The filename to download.
//   - set: The file set where the file is.
//   - thumb: "1" if downloading the thumbnail, "0" otherwise.
//
// Returns:
//   - The content of the file is streamed.
func (s *Server) handleDownload(w http.ResponseWriter, req *http.Request) {
	timer := prometheus.NewTimer(reqLatency.WithLabelValues(req.Method, req.URL.String()))
	defer timer.ObserveDuration()
	req.ParseForm()

	_, user, err := s.checkToken(req.PostFormValue("token"), "session")
	if err != nil {
		log.Errorf("%s %s (INVALID TOKEN: %v)", req.Method, req.URL, err)
		stingle.ResponseOK().AddPart("logout", "1").Send(w)
		reqStatus.WithLabelValues(req.Method, req.URL.String(), "nok").Inc()
		return
	}
	log.Infof("%s %s (UserID:%d)", req.Method, req.URL, user.UserID)
	filename := req.PostFormValue("file")
	set := req.PostFormValue("set")
	thumb := req.PostFormValue("thumb") == "1"

	f, err := s.db.DownloadFile(user, set, filename, thumb)
	if err != nil {
		log.Errorf("DownloadFile failed: %v", err)
		w.WriteHeader(http.StatusNotFound)
		reqStatus.WithLabelValues(req.Method, req.URL.String(), "nok").Inc()
		return
	}
	if _, err := s.copyWithCtx(req.Context(), w, f); err != nil {
		log.Debugf("Copy failed: %v", err)
	}
	if err := f.Close(); err != nil {
		log.Errorf("Close failed: %v", err)
	}
	reqStatus.WithLabelValues(req.Method, req.URL.String(), "ok").Inc()
}

// tryToHandleRange implements minimal support for RFC 7233, section 3.1: Range.
// Streaming videos doesn't work very well without it.
func (s *Server) tryToHandleRange(w http.ResponseWriter, rangeHdr string, f io.ReadSeekCloser) {
	log.Debugf("Requested range: %s", rangeHdr)
	m := regexp.MustCompile(`^bytes=(\d+)-$`).FindStringSubmatch(rangeHdr)
	if len(m) != 2 {
		return
	}
	size, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		log.Errorf("f.Seek(0, SeekEnd) failed: %v", err)
		return
	}
	offset := parseInt(m[1], 0)
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		log.Errorf("f.Seek(%d, SeekStart) failed: %v", offset, err)
		return
	}
	cr := fmt.Sprintf("bytes %d-%d/%d", offset, size-1, size)
	log.Debugf("Sending %s", cr)
	w.Header().Set("Content-Range", cr)
	w.WriteHeader(http.StatusPartialContent)
}

// handleTokenDownload handles the /v2/download endpoint. It is used to
// download a file with a client that can't use the authenticated API calls,
// e.g. a video player. The URL contains a token that's encrypted by this server
// and contains all the information to authenticate the request and find the
// requested file.
//
// Arguments:
//   - w: The http response writer.
//   - req: The http request.
//
// Returns:
//   - The content of the file is streamed.
func (s *Server) handleTokenDownload(w http.ResponseWriter, req *http.Request) {
	baseURI, tok := path.Split(req.URL.RequestURI())
	timer := prometheus.NewTimer(reqLatency.WithLabelValues(req.Method, baseURI))
	defer timer.ObserveDuration()

	token, user, err := s.checkToken(tok, "download")
	if err != nil {
		log.Errorf("%s %s (INVALID TOKEN: %v)", req.Method, req.URL, err)
		w.WriteHeader(http.StatusUnauthorized)
		reqStatus.WithLabelValues(req.Method, baseURI, "nok").Inc()
		return
	}
	log.Infof("%s %s %s[...] (UserID:%d)", req.Proto, req.Method, baseURI, user.UserID)

	f, err := s.db.DownloadFile(user, token.Set, token.File, token.Thumb)
	if err != nil {
		log.Errorf("DownloadFile(%q, %q, %q, %v) failed: %v", user.Email, token.Set, token.File, token.Thumb, err)
		w.WriteHeader(http.StatusNotFound)
		reqStatus.WithLabelValues(req.Method, baseURI, "nok").Inc()
		return
	}
	if r := req.Header.Get("Range"); r != "" {
		s.tryToHandleRange(w, r, f)
	}
	if _, err := s.copyWithCtx(req.Context(), w, f); err != nil {
		log.Debugf("Copy failed: %v", err)
	}
	if err := f.Close(); err != nil {
		log.Errorf("Close failed: %v", err)
	}
	reqStatus.WithLabelValues(req.Method, baseURI, "ok").Inc()
}

func (s *Server) copyWithCtx(ctx context.Context, dst io.Writer, src io.Reader) (n int64, err error) {
	buf := make([]byte, 4096)
	for {
		select {
		case <-ctx.Done():
			log.Debugf("copy: canceled after %d bytes", n)
			return
		default:
		}
		s.setDeadline(ctx, time.Now().Add(10*time.Minute))
		t := time.Now()
		nr, err := src.Read(buf)
		readDur := time.Since(t)
		if nr > 0 {
			s.setDeadline(ctx, time.Now().Add(10*time.Minute))
			nw, err := dst.Write(buf[:nr])
			n += int64(nw)
			if nw != nr {
				log.Debugf("copy: short write after %d bytes", n)
				return n, io.ErrShortWrite
			}
			if err != nil {
				return n, err
			}
		}
		if err == io.EOF {
			log.Debugf("copy: finished: %d bytes", n)
			return n, nil
		}
		if err != nil {
			log.Debugf("copy: read error: %v (read duration: %s)", err, readDur)
			return n, err
		}
	}
}

// makeDownloadURL creates a signed URL to download a file.
func (s *Server) makeDownloadURL(user database.User, host, file, set string, isThumb bool) (string, error) {
	tk, err := s.db.DecryptTokenKey(user.TokenKey)
	if err != nil {
		return "", err
	}
	defer tk.Wipe()
	tok := token.Mint(
		tk,
		token.Token{
			Scope:   "download",
			Subject: user.UserID,
			Set:     set,
			File:    file,
			Thumb:   isThumb,
		},
		12*time.Hour,
	)
	b := s.BaseURL
	if b == "" {
		b = fmt.Sprintf("https://%s%s/", host, s.pathPrefix)
	}
	return fmt.Sprintf("%sv2/download/%s", b, tok), nil
}

// handleGetDownloadUrls handles the /v2/sync/getDownloadUrls endpoint. It is
// used to created multiple signed URLs to download files.
//
// Arguments:
//   - user: The authenticated user.
//   - req: The http request
//
// Form arguments
//   - is_thumb: "1" if downloading thumbnails, "0" otherwise.
//   - files[<int>][filename]: The filenames to download.
//   - files[<int>][set]: The file sets where the files are.
//
// Returns:
//   - StringleResponse(ok).
//     Parts("urls", list of signed urls)
func (s *Server) handleGetDownloadUrls(user database.User, req *http.Request) *stingle.Response {
	isThumb := req.PostFormValue("is_thumb") == "1"
	urls := make(map[string]string)
	re := regexp.MustCompile(`^files\[\d+\]\[filename\]$`)

	for k, v := range req.PostForm {
		if !re.MatchString(k) || len(v) == 0 {
			continue
		}
		set := req.PostFormValue(strings.Replace(k, "filename", "set", 1))
		url, err := s.makeDownloadURL(user, req.Host, v[0], set, isThumb)
		if err != nil {
			return stingle.ResponseNOK()
		}
		urls[v[0]] = url
	}
	return stingle.ResponseOK().AddPart("urls", urls)
}

// handleGetURL handles the /v2/sync/getUrl endpoint. It is used to created
// a single signed URL to download a file.
//
// Arguments:
//   - user: The authenticated user.
//   - req: The http request
//
// Form arguments
//   - file: The filename to download.
//   - set: The file set where the file is.
//
// Returns:
//   - StringleResponse(ok).
//     Parts("url", signed url)
func (s *Server) handleGetURL(user database.User, req *http.Request) *stingle.Response {
	url, err := s.makeDownloadURL(user, req.Host, req.PostFormValue("file"), req.PostFormValue("set"), req.PostFormValue("thumb") == "1")
	if err != nil {
		return stingle.ResponseNOK()
	}
	return stingle.ResponseOK().AddPart("url", url)
}
