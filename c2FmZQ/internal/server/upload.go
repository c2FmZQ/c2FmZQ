package server

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"

	"c2FmZQ/internal/database"
	"c2FmZQ/internal/log"
)

// The return value of receiveUpload.
type upload struct {
	database.FileSpec
	token   string
	name    string
	set     string
	albumID string
}

// receiveUpload processes a multipart/form-data.
func (s *Server) receiveUpload(dir string, req *http.Request) (*upload, error) {
	ctx := req.Context()

	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	mr, err := req.MultipartReader()
	if err != nil {
		return nil, err
	}

	var upload upload

	for {
		s.setDeadline(ctx, time.Now().Add(time.Minute))
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if p.FileName() != "" {
			f, err := os.CreateTemp(dir, "upload-*")
			if err != nil {
				return nil, err
			}
			size, err := s.copyWithCtx(ctx, f, p)
			if err != nil {
				return nil, err
			}

			upload.name = p.FileName()
			if p.FormName() == "file" {
				upload.FileSpec.StoreFile = f.Name()
				upload.FileSpec.StoreFileSize = size
			} else if p.FormName() == "thumb" {
				upload.FileSpec.StoreThumb = f.Name()
				upload.FileSpec.StoreThumbSize = size
			}

			if err := f.Close(); err != nil {
				return nil, err
			}
			if err := p.Close(); err != nil {
				return nil, err
			}
		} else {
			buf := make([]byte, 2048)
			sz, err := io.ReadFull(p, buf)
			if err != io.ErrUnexpectedEOF && err != io.EOF {
				return nil, fmt.Errorf("received input is more than 2KB in size: sz=%d,%q=%q", sz, p.FormName(), string(buf[:sz]))
			}
			slurp := string(buf[:sz])

			switch p.FormName() {
			case "headers":
				upload.FileSpec.Headers = slurp
			case "set":
				upload.set = slurp
			case "dateCreated":
				if upload.FileSpec.DateCreated, err = strconv.ParseInt(slurp, 10, 64); err != nil {
					return nil, err
				}
			case "albumId":
				upload.albumID = slurp
			case "dateModified":
				if upload.FileSpec.DateModified, err = strconv.ParseInt(slurp, 10, 64); err != nil {
					return nil, err
				}
			case "version":
				upload.FileSpec.Version = slurp
			case "token":
				upload.token = slurp
			default:
				log.Errorf("receiveUpload: unexpected form input: %q=%q", p.FormName(), slurp)
			}
		}
	}

	return &upload, nil
}
