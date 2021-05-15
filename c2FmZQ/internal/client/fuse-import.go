package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/rwcarlsen/goexif/exif"

	"c2FmZQ/internal/log"
	"c2FmZQ/internal/stingle"
)

// StreamImport allows importing files via a stream, like writes from a fuse
// filesystem.
//
// First, it creates an initial entry in the fileset, and returns a
// io.WriteCloser. The io.WriteCloser is used to import the plaintext
// and encrypt it.
//
// When Close is called, the file is reprocessed to create a thumbnail,
// record its size, etc.
func (c *Client) StreamImport(name string, dst ListItem) (*FuseImportWriter, error) {
	if dst.Set == "" {
		album, err := c.addAlbum(dst.Filename)
		if err != nil {
			return nil, err
		}
		dst.Set = stingle.AlbumSet
		dst.Album = album
		dst.FileSet = albumPrefix + album.AlbumID
	}
	pk := c.SecretKey().PublicKey()
	if dst.Album != nil {
		if dst.Album.IsOwner != "1" && !stingle.Permissions(dst.Album.Permissions).AllowAdd() {
			return nil, syscall.EPERM
		}
		var err error
		if pk, err = dst.Album.PK(); err != nil {
			return nil, err
		}
	}

	thumbName := name
	ft := fileTypeForExt(strings.ToLower(filepath.Ext(name)))
	if ft == stingle.FileTypeGeneral {
		// See if the filename is a rsync temporary name, e.g.
		// .IMG_20191227_113708553.jpg.fbscSK
		if m := regexp.MustCompile(`^\.(.+\.[^.]+)\.[^.]{6}$`).FindStringSubmatch(name); len(m) == 2 {
			thumbName = m[1]
			ft = fileTypeForExt(strings.ToLower(filepath.Ext(m[1])))
		}
	}
	hdrs := stingle.NewHeaders(name)
	hdrs[0].FileType = ft
	hdrs[1].FileType = hdrs[0].FileType
	encHdrs, err := stingle.EncryptBase64Headers(hdrs[:], pk)
	if err != nil {
		return nil, err
	}
	ts := strconv.FormatInt(time.Now().UnixNano()/1000000, 10)
	sFile := stingle.File{
		File:         makeSPFilename(),
		Version:      "1",
		DateCreated:  json.Number(ts),
		DateModified: json.Number(ts),
		Headers:      encHdrs,
	}
	if dst.Album != nil {
		sFile.AlbumID = dst.Album.AlbumID
	}

	commit, fs, err := c.fileSetForUpdate(dst.FileSet)
	if err != nil {
		return nil, err
	}
	fs.Files[sFile.File] = &sFile
	if err := commit(true, nil); err != nil {
		return nil, err
	}

	fn := c.blobPath(sFile.File, false)
	dir, _ := filepath.Split(fn)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	thumbnail, err := c.genericThumbnail(thumbName)
	if err != nil {
		return nil, err
	}
	if err := c.encryptFile(bytes.NewBuffer(thumbnail), sFile.File, hdrs[1], pk, true); err != nil {
		return nil, err
	}

	out, err := os.OpenFile(fn, os.O_WRONLY|os.O_CREATE|os.O_EXCL|os.O_SYNC, 0600)
	if err != nil {
		return nil, err
	}
	if err := stingle.EncryptHeader(out, hdrs[0], pk); err != nil {
		out.Close()
		return nil, err
	}
	return &FuseImportWriter{
		c:            c,
		w:            stingle.EncryptFile(out, hdrs[0]),
		sfile:        sFile.File,
		albumID:      sFile.AlbumID,
		fs:           dst.FileSet,
		origFilename: hdrs[0].Filename,
	}, nil
}

// FuseImportWriter encrypts a file as it is being written.
type FuseImportWriter struct {
	c            *Client
	w            *stingle.StreamWriter
	size         int64
	sfile        string
	albumID      string
	fs           string
	origFilename []byte
}

func (iw *FuseImportWriter) Write(b []byte) (n int, err error) {
	iw.size += int64(len(b))
	return iw.w.Write(b)
}

func (iw *FuseImportWriter) Close() error {
	if err := iw.w.Close(); err != nil {
		return err
	}
	return iw.processNewFile()
}

func (iw *FuseImportWriter) sk() (sk stingle.SecretKey, err error) {
	sk = iw.c.SecretKey()
	if iw.albumID != "" {
		var al AlbumList
		if err := iw.c.storage.ReadDataFile(iw.c.fileHash(albumList), &al); err != nil {
			return sk, err
		}
		album, ok := al.Albums[iw.albumID]
		if !ok {
			return sk, fmt.Errorf("album doesn't exist anymore: %s", iw.albumID)
		}
		ask, err := album.SK(sk)
		if err != nil {
			return sk, err
		}
		sk = ask
	}
	return sk, nil
}

func (iw *FuseImportWriter) processNewFile() (retErr error) {
	sk, err := iw.sk()
	if err != nil {
		return err
	}
	pk := sk.PublicKey()

	commit, fs, err := iw.c.fileSetForUpdate(iw.fs)
	if err != nil {
		return err
	}
	file, ok := fs.Files[iw.sfile]
	if !ok {
		return fmt.Errorf("file is not in the fileset anymore: %s", iw.origFilename)
	}
	defer commit(false, &retErr)

	hdrs, err := stingle.DecryptBase64Headers(file.Headers, sk)
	if err != nil {
		return err
	}
	filename := string(hdrs[0].Filename)
	log.Debugf("FuseImportWriter.Close: %s", filename)

	encFile, err := os.Open(iw.c.blobPath(file.File, false))
	if err != nil {
		return err
	}
	if err := stingle.SkipHeader(encFile); err != nil {
		encFile.Close()
		return err
	}
	in := stingle.DecryptFile(encFile, hdrs[0])
	defer in.Close()

	hdrs[0].DataSize = iw.size
	creationTime := time.Now()
	if hdrs[0].FileType == stingle.FileTypeVideo {
		if dur, ct, err := videoMetadata(in); err == nil {
			log.Debugf("FuseImportWriter.Close: video duration %d", dur)
			hdrs[0].VideoDuration = dur
			if !ct.IsZero() {
				creationTime = ct
			}
		}
	}

	if _, err := in.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if x, err := exif.Decode(in); err == nil {
		if t, err := x.DateTime(); err == nil {
			log.Debugf("FuseImportWriter.Close: exif DateTime %s", t)
			creationTime = t
		}
	}
	if _, err := in.Seek(0, io.SeekStart); err != nil {
		return err
	}
	var thumbnail []byte
	switch hdrs[0].FileType {
	case stingle.FileTypeVideo:
		thumbnail, err = iw.c.videoThumbnail(in)
	case stingle.FileTypePhoto:
		thumbnail, err = iw.c.photoThumbnail(in)
	default:
		thumbnail, err = iw.c.genericThumbnail(filename)
	}
	if err != nil {
		// Fallback to a generic thumbnail.
		thumbnail, err = iw.c.genericThumbnail(filename)
	}
	if err != nil {
		return err
	}
	hdrs[1].DataSize = int64(len(thumbnail))
	hdrs[1].FileType = hdrs[0].FileType
	hdrs[1].VideoDuration = hdrs[0].VideoDuration

	encHdrs, err := stingle.EncryptBase64Headers(hdrs, pk)
	if err != nil {
		return err
	}
	file.Headers = encHdrs
	file.DateCreated = json.Number(strconv.FormatInt(creationTime.UnixNano()/1000000, 10))

	// Rewrite the thumbnail.
	if err := iw.c.encryptFile(bytes.NewBuffer(thumbnail), file.File, hdrs[1], pk, true); err != nil {
		return err
	}
	if err := commit(true, nil); err != nil {
		return err
	}

	// Rewrite the file header. The header should be the same size because
	// we use the original filename.
	out, err := os.OpenFile(iw.c.blobPath(file.File, false), os.O_WRONLY|os.O_SYNC, 0600)
	if err != nil {
		return err
	}
	hdrs[0].Filename = iw.origFilename
	if err := stingle.EncryptHeader(out, hdrs[0], pk); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
