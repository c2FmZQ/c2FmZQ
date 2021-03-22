package server

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"

	"stingle-server/database"
	"stingle-server/log"
)

// The return value of receiveUpload.
type Upload struct {
	database.FileSpec
	Token string
}

// receiveUpload processes a multipart/form-data.
func receiveUpload(dir string, req *http.Request) (*Upload, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	mr, err := req.MultipartReader()
	if err != nil {
		return nil, err
	}

	var upload Upload

	for {
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
			size, err := io.Copy(f, p)
			if err != nil {
				return nil, err
			}

			upload.FileSpec.File = p.FileName()
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
			buf := make([]byte, 1024)
			sz, err := io.ReadFull(p, buf)
			if err != io.ErrUnexpectedEOF && err != io.EOF {
				return nil, fmt.Errorf("received input is more than 1KB in size: sz=%d,%q=%q", sz, p.FormName(), string(buf[:sz]))
			}
			slurp := string(buf[:sz])

			switch p.FormName() {
			case "headers":
				upload.FileSpec.Headers = slurp
			case "set":
				upload.FileSpec.Set = slurp
			case "dateCreated":
				if upload.FileSpec.DateCreated, err = strconv.ParseInt(slurp, 10, 64); err != nil {
					return nil, err
				}
			case "albumId":
				upload.FileSpec.AlbumID = slurp
			case "dateModified":
				if upload.FileSpec.DateModified, err = strconv.ParseInt(slurp, 10, 64); err != nil {
					return nil, err
				}
			case "version":
				upload.FileSpec.Version = slurp
			case "token":
				upload.Token = slurp
			default:
				log.Errorf("receiveUpload: unexpected form input: %q=%q", p.FormName(), slurp)
			}
		}
	}

	return &upload, nil
}
