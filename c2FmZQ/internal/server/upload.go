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
	mr, err := req.MultipartReader()
	if err != nil {
		return nil, err
	}
	var upload upload

	for {
		s.setDeadline(ctx, time.Now().Add(10*time.Minute))
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if p.FileName() != "" {
			f, name, err := s.db.TempFile(dir)
			if err != nil {
				return nil, err
			}
			size, err := s.copyWithCtx(ctx, f, p)
			if err != nil {
				if err := os.Remove(name); err != nil {
					log.Errorf("os.Remove(%q): %v", name, err)
				}
				return nil, err
			}

			upload.name = p.FileName()
			if p.FormName() == "file" {
				upload.FileSpec.StoreFile = name
				upload.FileSpec.StoreFileSize = size
			} else if p.FormName() == "thumb" {
				upload.FileSpec.StoreThumb = name
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
