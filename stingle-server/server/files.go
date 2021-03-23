package server

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"stingle-server/crypto"
	"stingle-server/database"
	"stingle-server/log"
)

// handleUpload handles the /v2/sync/upload endpoint. It is used to upload
// new files. The incoming request is a multipart/form-data with two files:
// one for the image or video, and one for the thumbnail.
//
// Arguments:
//  - req: The http request.
//
// Form arguments
//  - token: The signed session token.
//  - headers: File metadata (encrypted key, etc)
//  - set: The file set where this file is being uploaded.
//  - albumId: The ID of the album where the file is being uploaded.
//  - dateCreated: A timestamp in milliseconds.
//  - dateModified: A timestamp in milliseconds.
//  - version: The file format version (opaque to the server).
//
// Returns:
//  - StingleResponse("ok")
func (s *Server) handleUpload(req *http.Request) *StingleResponse {
	up, err := receiveUpload(filepath.Join(s.db.Dir(), "uploads"), req)
	if err != nil {
		log.Errorf("receiveUpload: %v", err)
		return NewResponse("nok")
	}
	_, user, err := s.checkToken(up.Token, "session")
	if err != nil {
		return NewResponse("nok")
	}

	if up.Set == database.AlbumSet {
		albumSpec, err := s.db.Album(user, up.AlbumID)
		if err != nil {
			log.Errorf("db.Album(%q, %q) failed: %v", user.Email, up.AlbumID, err)
			return NewResponse("nok")
		}
		if albumSpec.OwnerID != user.UserID && !albumSpec.Permissions.AllowAdd() {
			return NewResponse("nok").AddError("Adding to this album is not permitted")
		}
	}

	if err := s.db.AddFile(user, up.FileSpec); err != nil {
		log.Errorf("AddFile: %v", err)
		return NewResponse("nok")
	}
	return NewResponse("ok")
}

// handleMoveFile handles the /v2/sync/moveFile endpoint. It is used to move
// or copy files between filesets/albums.
//
// Arguments:
//  - user: The authenticated user.
//  - req: The http request.
//
// Form arguments
//  - token: The signed session token.
//  - params: The encrypted parameters
//     - setFrom: The set from which the files are moving (or being copied)
//     - setTo: The set to which the files are moving (or being copied)
//     - albumIdFrom: The ID of the album from which the files are moving, or ""
//                    if moving from Trash or Gallery.
//     - albumIdTo: The IS of the album to which the files are moving, or "" if
//                  moving to Trash or Gallery.
//     - isMoving: "0" if the files are being copied, "1" if they are moving.
//     - count: The number of files being copied or moved.
//     - filename<int>: The filenames affected (filename0, filename1, etc)
//     - headers<int>: The file headers, present only if the headers are
//                     changing, i.e. when moving to/from albums.
//
// Returns:
//  - StingleResponse(ok)
func (s *Server) handleMoveFile(user database.User, req *http.Request) *StingleResponse {
	params, err := s.decodeParams(req.PostFormValue("params"), user)
	if err != nil {
		log.Errorf("decodeParams: %v", err)
		return NewResponse("nok")
	}

	p := database.MoveFileParams{
		SetFrom:     params["setFrom"],
		SetTo:       params["setTo"],
		AlbumIDFrom: params["albumIdFrom"],
		AlbumIDTo:   params["albumIdTo"],
		IsMoving:    params["isMoving"] == "1",
	}
	count := parseInt(params["count"], 0)

	for i := 0; i < int(count); i++ {
		filename := params[fmt.Sprintf("filename%d", i)]
		p.Filenames = append(p.Filenames, filename)
		hdr := params[fmt.Sprintf("headers%d", i)]
		if hdr != "" {
			p.Headers = append(p.Headers, hdr)
		}
	}
	if p.AlbumIDFrom != "" {
		albumSpec, err := s.db.Album(user, p.AlbumIDFrom)
		if err != nil {
			log.Errorf("db.Album(%q, %q) failed: %v", user.Email, p.AlbumIDFrom, err)
			return NewResponse("nok")
		}
		if albumSpec.OwnerID != user.UserID && !albumSpec.Permissions.AllowCopy() {
			return NewResponse("nok").AddError("Copying from this album is not permitted")
		}
		if albumSpec.OwnerID != user.UserID && p.IsMoving {
			return NewResponse("nok").AddError("Removing items from this album is not permitted")
		}
	}
	if p.AlbumIDTo != "" {
		albumSpec, err := s.db.Album(user, p.AlbumIDTo)
		if err != nil {
			log.Errorf("db.Album(%q, %q) failed: %v", user.Email, p.AlbumIDTo, err)
			return NewResponse("nok")
		}
		if albumSpec.OwnerID != user.UserID && !albumSpec.Permissions.AllowAdd() {
			return NewResponse("nok").AddError("Adding to this album is not permitted")
		}
	}

	if err := s.db.MoveFile(user, p); err != nil {
		log.Errorf("MoveFile(%+v): %v", p, err)
		return NewResponse("nok")
	}
	return NewResponse("ok")
}

// handleEmptyTrash handles the /v2/sync/emptyTrash endpoint. It is used to
// delete all the files in the Trash set.
//
// Arguments:
//  - user: The authenticated user.
//  - req: The http request
//
// Form arguments
//  - params: The encrypted parameters
//     - time: A timestamp in milliseconds. All files added until that time
//             should be removed.
//
// Returns:
//  - StingleResponse(ok)
func (s *Server) handleEmptyTrash(user database.User, req *http.Request) *StingleResponse {
	params, err := s.decodeParams(req.PostFormValue("params"), user)
	if err != nil {
		log.Errorf("decodeParams: %v", err)
		return NewResponse("nok")
	}
	if err := s.db.EmptyTrash(user, parseInt(params["time"], 0)); err != nil {
		log.Errorf("EmptyTrash: %v", err)
		return NewResponse("nok")
	}
	return NewResponse("ok")
}

// handleDelete handles the /v2/sync/delete endpoint. It is used to delete some
// the files in the Trash set.
//
// Arguments:
//  - user: The authenticated user.
//  - req: The http request
//
// Form arguments
//  - params: The encrypted parameters
//     - count: The number of files being deleted.
//     - filename<int>: The filenames being deleted.
//
// Returns:
//  - StingleResponse(ok)
func (s *Server) handleDelete(user database.User, req *http.Request) *StingleResponse {
	params, err := s.decodeParams(req.PostFormValue("params"), user)
	if err != nil {
		log.Errorf("decodeParams: %v", err)
		return NewResponse("nok")
	}
	count := int(parseInt(params["count"], 0))
	files := []string{}
	for i := 0; i < count; i++ {
		files = append(files, params[fmt.Sprintf("filename%d", i)])
	}
	if err := s.db.DeleteFiles(user, files); err != nil {
		log.Errorf("DeleteFiles: %v", err)
		return NewResponse("nok")
	}
	return NewResponse("ok")
}

// handleDownload handles the /v2/sync/download endpoint. It is used to download
// the content of a file.
//
// Arguments:
//  - user: The authenticated user.
//  - req: The http request
//
// Form arguments
//  - file: The filename to download.
//  - set: The file set where the file is.
//  - thumb: "1" if downloading the thumbnail, "0" otherwise.
//
// Returns:
//   - The content of the file is streamed.
func (s *Server) handleDownload(w http.ResponseWriter, req *http.Request) {
	req.ParseForm()

	_, user, err := s.checkToken(req.PostFormValue("token"), "session")
	if err != nil {
		log.Errorf("%s %s (INVALID TOKEN: %v)", req.Method, req.URL, err)
		NewResponse("ok").AddPart("logout", "1").Send(w)
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
		return
	}
	if _, err := io.Copy(w, f); err != nil {
		log.Errorf("Copy failed: %v", err)
	}
	if err := f.Close(); err != nil {
		log.Errorf("Close failed: %v", err)
	}
}

// tryToHandleRange implements minimal support for RFC 7233, section 3.1: Range.
// Streaming videos doesn't work very well without it.
func (s *Server) tryToHandleRange(w http.ResponseWriter, rangeHdr string, f *os.File) {
	log.Infof("Requested range: %s", rangeHdr)
	m := regexp.MustCompile(`^bytes=(\d+)-$`).FindStringSubmatch(rangeHdr)
	if len(m) != 2 {
		return
	}
	offset := parseInt(m[1], 0)
	if _, err := f.Seek(offset, 0); err != nil {
		log.Errorf("f.Seek(%d, 0) failed: %v", offset, err)
		return
	}
	fi, err := f.Stat()
	if err != nil {
		log.Errorf("f.Stat() failed: %v", err)
		return
	}
	cr := fmt.Sprintf("bytes %d-%d/%d", offset, fi.Size()-1, fi.Size())
	log.Infof("Sending %s", cr)
	w.Header().Set("Content-Range", cr)
	w.WriteHeader(http.StatusPartialContent)
}

// handleSignedDownload handles the /v2/signedDownload endpoint. It is used to
// download a file with a client that can't use the authenticated API calls,
// e.g. a video player. The URL contains a token that's signed by this server
// and contains all the information to authenticate the request and find the
// requested file.
//
// Arguments:
//  - w: The http response writer.
//  - req: The http request.
//
// Returns:
//   - The content of the file is streamed.
func (s *Server) handleSignedDownload(w http.ResponseWriter, req *http.Request) {
	_, tok := path.Split(req.URL.RequestURI())
	token, user, err := s.checkToken(tok, "download")
	if err != nil {
		log.Errorf("%s %s (INVALID TOKEN: %v)", req.Method, req.URL, err)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	log.Infof("%s %s (UserID:%d)", req.Method, req.URL, user.UserID)

	f, err := s.db.DownloadFile(user, token.Set, token.File, token.Thumb)
	if err != nil {
		log.Errorf("DownloadFile failed: %v", err)
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if r := req.Header.Get("Range"); r != "" {
		s.tryToHandleRange(w, r, f)
	}
	if _, err := io.Copy(w, f); err != nil {
		log.Errorf("Copy failed: %v", err)
	}
	if err := f.Close(); err != nil {
		log.Errorf("Close failed: %v", err)
	}
}

// makeDownloadURL creates a signed URL to download a file.
func (s *Server) makeDownloadURL(user database.User, host, file, set string, isThumb bool) string {
	tok := crypto.MintToken(
		user.ServerSignKey,
		crypto.Token{
			Scope:   "download",
			Subject: user.Email,
			Seq:     user.TokenSeq,
			Set:     set,
			File:    file,
			Thumb:   isThumb,
		},
		1*time.Hour,
	)
	b := s.BaseURL
	if b == "" {
		b = fmt.Sprintf("https://%s/", host)
	}
	return fmt.Sprintf("%sv2/signedDownload/%s", b, tok)
}

// handleGetDownloadUrls handles the /v2/sync/getDownloadUrls endpoint. It is
// used to created multiple signed URLs to download files.
//
// Arguments:
//  - user: The authenticated user.
//  - req: The http request
//
// Form arguments
//  - is_thumb: "1" if downloading thumbnails, "0" otherwise.
//  - files[<int>][filename]: The filenames to download.
//  - files[<int>][set]: The file sets where the files are.
//
// Returns:
//   - StringleResponse(ok).
//        Parts("urls", list of signed urls)
func (s *Server) handleGetDownloadUrls(user database.User, req *http.Request) *StingleResponse {
	isThumb := req.PostFormValue("is_thumb") == "1"
	urls := make(map[string]string)
	re := regexp.MustCompile(`^files\[\d+\]\[filename\]$`)

	for k, v := range req.PostForm {
		if !re.MatchString(k) || len(v) == 0 {
			continue
		}
		set := req.PostFormValue(strings.Replace(k, "filename", "set", 1))
		urls[v[0]] = s.makeDownloadURL(user, req.Host, v[0], set, isThumb)
	}
	return NewResponse("ok").AddPart("urls", urls)
}

// handleGetUrl handles the /v2/sync/getUrl endpoint. It is used to created
// a single signed URL to download a file.
//
// Arguments:
//  - user: The authenticated user.
//  - req: The http request
//
// Form arguments
//  - file: The filename to download.
//  - set: The file set where the file is.
//
// Returns:
//   - StringleResponse(ok).
//        Parts("url", signed url)
func (s *Server) handleGetUrl(user database.User, req *http.Request) *StingleResponse {
	return NewResponse("ok").
		AddPart("url", s.makeDownloadURL(user, req.Host, req.PostFormValue("file"), req.PostFormValue("set"), false))
}
