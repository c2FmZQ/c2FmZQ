package web

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"c2FmZQ/internal/client"
	"c2FmZQ/internal/log"
	"c2FmZQ/internal/stingle"
	"c2FmZQ/internal/stingle/token"
)

var (
	//go:embed static/*
	staticContent embed.FS

	//go:embed index.template
	indexTemplateSource string
	indexTemplate       *template.Template

	//go:embed gallery.template
	galleryTemplateSource string
	galleryTemplate       *template.Template

	//go:embed edit.template
	editTemplateSource string
	editTemplate       *template.Template
)

func init() {
	funcs := template.FuncMap{
		"basename": path.Base,
	}
	indexTemplate = template.Must(template.New("index").Funcs(funcs).Parse(indexTemplateSource))
	galleryTemplate = template.Must(template.New("gallery").Funcs(funcs).Parse(galleryTemplateSource))
	editTemplate = template.Must(template.New("edit").Funcs(funcs).Parse(editTemplateSource))
}

func (s *Server) reqPath(req *http.Request, endpoint string) string {
	p := strings.TrimPrefix(req.URL.Path, s.c.WebServerConfig.URLPrefix)
	p = strings.TrimPrefix(p, endpoint)
	return p
}

func (s *Server) handleIndex(w http.ResponseWriter, req *http.Request) {
	log.Infof("%s %s", req.Method, req.RequestURI)
	ctx := req.Context()

	p := s.reqPath(req, "/")
	if b, err := staticContent.ReadFile(filepath.Join("static", p)); err == nil {
		w.Header().Set("Cache-Control", "public, immutable")
		http.ServeContent(w, req, p, time.Time{}, bytes.NewReader(b))
		return
	}
	if p != "" {
		http.NotFound(w, req)
		return
	}

	if req.Method == http.MethodPost {
		if err := req.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if pass := req.PostForm.Get("password"); pass != s.c.WebServerConfig.Password {
			http.Error(w, "Coffee?", http.StatusTeapot)
		}
		redirect := req.PostForm.Get("redir")
		if redirect == "" {
			redirect = s.c.WebServerConfig.URLPrefix + "view/"
		}
		tok := token.Mint(s.c.WebServerConfig.TokenKey, token.Token{Scope: "web", File: tagFromCtx(ctx)}, 24*time.Hour)
		redirect = redirect + "?tok=" + url.QueryEscape(tok)

		http.Redirect(w, req, redirect, http.StatusFound)
		return
	}
	data := struct {
		Redirect string
		Prefix   string
	}{
		Redirect: req.URL.Query().Get("redir"),
		Prefix:   s.c.WebServerConfig.URLPrefix,
	}
	w.Header().Set("Content-Type", "text/html;charset=UTF-8")
	if err := indexTemplate.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (s *Server) glob(pattern string, exact bool) ([]client.ListItem, error) {
	li, err := s.c.GlobFiles([]string{s.c.WebServerConfig.ExportPath + pattern}, client.GlobOptions{ExactMatch: exact, Quiet: true})
	if err != nil {
		return nil, err
	}
	for i := range li {
		li[i].Filename = strings.TrimPrefix(li[i].Filename, s.c.WebServerConfig.ExportPath)
	}
	sort.Slice(li, func(i, j int) bool {
		return li[i].FSFile.DateCreated > li[j].FSFile.DateCreated
	})
	return li, nil
}

func (s *Server) handleView(w http.ResponseWriter, req *http.Request) {
	log.Infof("%s %s", req.Method, req.RequestURI)
	page, _ := strconv.Atoi(req.URL.Query().Get("page"))
	if page == 0 {
		page = 1
	}
	type albumData struct {
		Name  string
		Cover string
	}
	type fileData struct {
		Name     string
		Date     string
		Duration string
	}
	data := struct {
		Token    string
		Prefix   string
		Page     int
		NextPage int
		Parent   string
		Current  string
		Albums   []albumData
		Files    []fileData
		// If it's a single file.
		Name     string
		Date     string
		IsVideo  bool
		PrevFile string
		NextFile string
	}{
		Token:    req.URL.Query().Get("tok"),
		Prefix:   s.c.WebServerConfig.URLPrefix,
		Page:     page,
		NextPage: page + 1,
	}
	var li []client.ListItem
	var isDir bool
	if path := s.reqPath(req, "view/"); path == "" {
		isDir = true
		data.Current = ""
	} else {
		var err error
		if li, err = s.glob(path, true); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if len(li) == 0 {
			http.NotFound(w, req)
			return
		}
		isDir = li[0].IsDir
		data.Current = li[0].Filename
	}
	if isDir {
		if req.URL.Path[len(req.URL.Path)-1] != '/' {
			url := req.URL
			url.Path = url.Path + "/"
			http.Redirect(w, req, url.String(), http.StatusFound)
			return
		}
		if data.Current != "" {
			data.Parent = path.Dir(data.Current) + "/"
		}
		pattern := filepath.Join(data.Current, "*")
		li, err := s.glob(pattern, false)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		sk := s.c.SecretKey()
		defer sk.Wipe()
		var files []fileData
		for _, item := range li {
			if item.IsDir {
				cover, _ := s.c.AlbumCover(item)
				if cover == "" {
					cover = item.Filename
				}
				data.Albums = append(data.Albums, albumData{
					Name:  fixSlashes(item.Filename),
					Cover: fixSlashes(cover),
				})
			} else {
				t, _ := item.FSFile.DateCreated.Int64()
				hdr, err := item.Header(sk)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				var dur string
				if d := hdr.VideoDuration; d > 0 {
					dur = fmt.Sprintf("%s", time.Duration(d)*time.Second)
				}
				hdr.Wipe()
				files = append(files, fileData{
					Name:     fixSlashes(item.Filename),
					Date:     time.Unix(t/1000, 0).Format("Monday, 2 January 2006"),
					Duration: dur,
				})
			}
		}
		const itemsPerPage = 10
		start := (page - 1) * itemsPerPage
		if end := len(files); start < end {
			if end-start > itemsPerPage {
				end = start + itemsPerPage
			}
			data.Files = files[start:end]
		}
	} else {
		item := li[0]
		t, _ := item.FSFile.DateCreated.Int64()
		data.Date = time.Unix(t/1000, 0).Format("Monday, 2 January 2006")
		data.Name = fixSlashes(item.Filename)
		data.Parent = fixSlashes(filepath.Dir(item.Filename)) + "/"
		sk := s.c.SecretKey()
		hdr, err := item.Header(sk)
		sk.Wipe()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer hdr.Wipe()
		data.IsVideo = hdr.FileType == stingle.FileTypeVideo

		// Find Previous and Next files
		pattern := filepath.Join(data.Parent, "*")
		li, err := s.glob(pattern, false)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for i := range li {
			if li[i].Filename == item.Filename {
				if i > 0 {
					data.PrevFile = li[i-1].Filename
				}
				if i < len(li)-1 {
					data.NextFile = li[i+1].Filename
				}
				break
			}
		}
	}
	w.Header().Set("Content-Type", "text/html;charset=UTF-8")
	if err := galleryTemplate.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (s *Server) handleEdit(w http.ResponseWriter, req *http.Request) {
	log.Infof("%s %s", req.Method, req.RequestURI)
	data := struct {
		Token   string
		Prefix  string
		Current string
		Parent  string
		Name    string
	}{
		Token:  req.URL.Query().Get("tok"),
		Prefix: s.c.WebServerConfig.URLPrefix,
	}
	var li []client.ListItem
	file := s.reqPath(req, "edit/")
	li, err := s.glob(file, true)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(li) == 0 {
		http.NotFound(w, req)
		return
	}
	if li[0].IsDir {
		http.Error(w, "Edit directory", http.StatusInternalServerError)
	}

	item := li[0]
	data.Current = fixSlashes(item.Filename)
	data.Parent = path.Dir(data.Current) + "/"
	data.Name = fixSlashes(item.Filename)

	w.Header().Set("Content-Type", "text/html;charset=UTF-8")
	if err := editTemplate.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (s *Server) handleUpload(w http.ResponseWriter, req *http.Request) {
	log.Infof("%s %s", req.Method, req.RequestURI)
	var li []client.ListItem
	file := s.reqPath(req, "upload/")
	li, err := s.glob(file, true)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(li) == 0 {
		http.NotFound(w, req)
		return
	}
	if !li[0].IsDir {
		http.Error(w, "not a folder", http.StatusInternalServerError)
		return
	}

	ctx := req.Context()
	mr, err := req.MultipartReader()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	for {
		s.setDeadline(ctx, time.Now().Add(time.Minute))
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if p.FormName() == "file" {
			name := path.Base(fixSlashes(p.FileName()))
			f, err := s.c.StreamImport(name, li[0])
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if _, err := io.Copy(f, p); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if err := f.Close(); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if err := p.Close(); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			continue
		}
		http.Error(w, "invalid upload", http.StatusInternalServerError)
		return
	}
}

func (s *Server) handleRaw(w http.ResponseWriter, req *http.Request) {
	log.Infof("%s %s", req.Method, req.RequestURI)
	path := s.reqPath(req, "raw/")
	thumb := req.URL.Query().Get("thumb") == "1"

	li, err := s.glob(path, true)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(li) == 0 {
		http.NotFound(w, req)
		return
	}
	item := li[0]
	if item.IsDir {
		if thumb {
			s.sendThumbnail(w, item.Filename)
			return
		}
		li, err := s.glob(filepath.Join(path, "*"), false)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		for _, item := range li {
			w.Write([]byte(item.Filename + "\n"))
		}
		return
	}

	var f io.ReadSeekCloser
	if thumb {
		if f, err = os.Open(item.ThumbPath); errors.Is(err, os.ErrNotExist) {
			if item.FSFile.File != "" {
				f, err = s.c.DownloadGet(item.FSFile.File, item.Set, true)
			}
		}
		if err != nil {
			s.sendThumbnail(w, item.Filename)
			return
		}
	} else {
		if f, err = os.Open(item.FilePath); errors.Is(err, os.ErrNotExist) {
			if item.FSFile.File != "" {
				f, err = s.c.DownloadGet(item.FSFile.File, item.Set, false)
			}
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	defer f.Close()
	if err := stingle.SkipHeader(f); err != nil {
		s.sendThumbnail(w, item.Filename)
		return
	}
	sk := s.c.SecretKey()
	var hdr *stingle.Header
	if thumb {
		hdr, err = item.ThumbHeader(sk)
	} else {
		hdr, err = item.Header(sk)
	}
	sk.Wipe()
	defer hdr.Wipe()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if s.c.WebServerConfig.AllowCaching {
		w.Header().Set("Cache-Control", "private, max-age=86400, immutable")
	}

	in := stingle.DecryptFile(f, hdr)
	buf := make([]byte, 512)
	n, _ := io.ReadFull(in, buf)
	w.Header().Set("Content-Type", http.DetectContentType(buf[:n]))
	w.Header().Set("Accept-Ranges", "bytes")

	// Handle Range header. We could use http.ServeContent, but it uses io.Seek a
	// lot, which can be expensive here.
	start, end, ok := parseRangeHeader(req, hdr.DataSize)
	if !ok {
		w.Write(buf[:n])
		io.Copy(w, in)
		return
	}

	if _, err := in.Seek(start, io.SeekStart); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, hdr.DataSize))
	w.WriteHeader(http.StatusPartialContent)
	if req.Method != http.MethodHead {
		io.CopyN(w, in, end-start+1)
	}
	return
}

func (s *Server) sendThumbnail(w http.ResponseWriter, filename string) {
	b, err := s.c.GenericThumbnail(filename)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Write(b)
}

func parseRangeHeader(req *http.Request, size int64) (int64, int64, bool) {
	r := req.Header.Get("Range")
	if r == "" {
		return 0, 0, false
	}
	m := regexp.MustCompile(`^bytes=(\d*)-(\d*)$`).FindStringSubmatch(r)
	if len(m) != 3 {
		return 0, 0, false
	}
	if len(m[1]) > 0 {
		start, err := strconv.ParseInt(m[1], 10, 64)
		if err != nil {
			return 0, 0, false
		}
		end := size - 1
		if len(m[2]) > 0 {
			if end, err = strconv.ParseInt(m[2], 10, 64); err != nil {
				return 0, 0, false
			}
		}
		if end < start {
			return 0, 0, false
		}
		return start, end, true
	}
	length, err := strconv.ParseInt(m[2], 10, 64)
	if err != nil || length > size {
		return 0, 0, false
	}
	return size - length, size - 1, true
}

func fixSlashes(s string) string {
	if filepath.Separator == '\\' {
		s = strings.Map(func(r rune) rune {
			if r == filepath.Separator {
				return '/'
			}
			return r
		}, s)
	}
	return s
}
