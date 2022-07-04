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

package client

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"golang.org/x/image/font"
	"golang.org/x/image/font/inconsolata"
	"golang.org/x/image/math/fixed"
	"image"
	"image/color"
	"image/png"
	"io"
	"io/fs"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/disintegration/imaging"
	"github.com/rwcarlsen/goexif/exif"

	"c2FmZQ/internal/log"
	"c2FmZQ/internal/stingle"
)

type toImport struct {
	src string
	dst string
}

// ImportFiles encrypts and imports files. Returns the number of files imported.
func (c *Client) ImportFiles(patterns []string, dest string, recursive bool) (int, error) {
	files, err := c.findFilesToImport(patterns, dest, recursive)
	if err != nil {
		return 0, err
	}
	dirs := make(map[string][]ListItem)
	for _, f := range files {
		dir, _ := filepath.Split(f.dst)
		dir = strings.TrimSuffix(dir, "/")
		dirs[dir] = nil
	}
	var sorted []string
	for dir := range dirs {
		sorted = append(sorted, dir)
	}
	sort.Strings(sorted)
	// The first loop on sorted catches most errors without mutating
	// anything.
	for _, dir := range sorted {
		li, err := c.glob(dir, GlobOptions{ExactMatch: true})
		if err != nil {
			return 0, err
		}
		dirs[dir] = li
		if len(li) > 1 {
			// Should not happen.
			return 0, fmt.Errorf("%s is not a directory", dir)
		}
		if len(li) == 1 && !li[0].IsDir {
			return 0, fmt.Errorf("%s is not a directory", dir)
		}
		if len(li) == 0 {
			continue
		}
		if li[0].Set == stingle.TrashSet {
			return 0, fmt.Errorf("cannot import to trash: %s", dir)
		}
		if li[0].Album != nil && li[0].Album.IsOwner != "1" && !stingle.Permissions(li[0].Album.Permissions).AllowAdd() {
			return 0, fmt.Errorf("adding is not allowed: %s", dir)
		}
	}
	count := 0
	for _, dir := range sorted {
		li := dirs[dir]
		if len(li) == 0 || (len(li) == 1 && li[0].Set == "") {
			name := dir
			if len(li) == 1 {
				name = li[0].Filename
			}
			if _, err := c.addAlbum(name); err != nil {
				return 0, err
			}
			if li, err = c.glob(name, GlobOptions{ExactMatch: true}); err != nil {
				return 0, err
			}
		}
		pk := c.PublicKey()
		if li[0].Album != nil {
			if pk, err = li[0].Album.PK(); err != nil {
				return 0, err
			}
		}
		for _, f := range files {
			if dd, _ := filepath.Split(f.dst); dir != strings.TrimSuffix(dd, "/") {
				continue
			}
			c.Printf("Importing %s -> %s (not synced)\n", f.src, f.dst)
			if err := c.importFile(f.src, li[0], pk); err != nil {
				return count, err
			}
			count++
		}
	}

	return count, nil
}

func importedFileName(s string) string {
	s = strings.ReplaceAll(s, "\\", "/")
	parts := strings.Split(s, "/")
	for i := range parts {
		parts[i] = sanitize(parts[i])
	}
	return filepath.Join(parts...)
}

func (c *Client) findFilesToImport(patterns []string, dest string, recursive bool) ([]toImport, error) {
	dest = strings.TrimSuffix(dest, "/")
	li, err := c.glob(dest, GlobOptions{})
	if err != nil {
		return nil, err
	}
	if len(li) > 1 || (len(li) == 1 && !li[0].IsDir) {
		return nil, fmt.Errorf("destination must be a directory: %s", dest)
	}
	if len(li) == 1 {
		dest = li[0].Filename
	}

	existingItems, err := c.glob(filepath.Join(dest, "*"), GlobOptions{MatchDot: true, Recursive: recursive})
	if err != nil {
		return nil, err
	}
	exist := make(map[string]bool)
	for _, item := range existingItems {
		exist[item.Filename] = true
	}

	var files []toImport
	for _, p := range patterns {
		m, err := filepath.Glob(p)
		if err != nil {
			return nil, err
		}
		for _, f := range m {
			fi, err := os.Stat(f)
			if err != nil {
				log.Errorf("%s: %v", f, err)
				continue
			}
			if !fi.IsDir() {
				_, file := filepath.Split(f)
				df := filepath.Join(dest, importedFileName(file))
				if exist[df] {
					c.Printf("Skipping %s (already exists)\n", df)
					continue
				}
				files = append(files, toImport{src: f, dst: df})
				continue
			}
			if !recursive {
				continue
			}
			baseDir, _ := filepath.Split(f)
			filepath.WalkDir(f, func(p string, d fs.DirEntry, err error) error {
				if err != nil {
					log.Errorf("%s: %v", p, err)
					return nil
				}
				if d.IsDir() {
					return nil
				}
				rel, err := filepath.Rel(baseDir, p)
				if err != nil {
					log.Errorf("%s: %v", p, err)
					return nil
				}
				df := filepath.Join(dest, importedFileName(rel))
				if exist[df] {
					c.Printf("Skipping %s (already exists)\n", df)
					return nil
				}
				files = append(files, toImport{src: p, dst: df})
				return nil
			})
		}
	}

	return files, nil
}

func fileTypeForExt(ext string) uint8 {
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".tiff", ".bmp", ".webp", ".svg":
		return stingle.FileTypePhoto
	case ".mp4", ".mov", ".webm", ".mkv", ".flv", ".vob", ".ogv", ".ogg", ".avi", ".mts",
		".m2ts", ".ts", ".qt", ".wmv", ".yuv", ".rm", ".rmvb", ".m4p", ".m4v", ".mpg",
		".mp2", ".mpeg", ".mpe", ".mpv", ".m2v", ".svi", ".3gp", ".3g2":
		return stingle.FileTypeVideo
	default:
		return stingle.FileTypeGeneral
	}
}

func (c *Client) importFile(file string, dst ListItem, pk stingle.PublicKey) error {
	fi, err := os.Stat(file)
	if err != nil {
		return err
	}

	in, err := os.Open(file)
	if err != nil {
		return err
	}
	defer in.Close()

	_, fn := filepath.Split(file)
	creationTime := time.Now()

	hdrs := stingle.NewHeaders(fn)
	defer hdrs[0].Wipe()
	defer hdrs[1].Wipe()
	hdrs[0].DataSize = fi.Size()
	hdrs[0].FileType = fileTypeForExt(strings.ToLower(filepath.Ext(file)))
	if hdrs[0].FileType == stingle.FileTypeVideo {
		if dur, ct, err := videoMetadata(in); err == nil {
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
			creationTime = t
		}
	}
	if _, err := in.Seek(0, io.SeekStart); err != nil {
		return err
	}

	var thumbnail []byte
	switch hdrs[0].FileType {
	case stingle.FileTypeVideo:
		thumbnail, err = c.videoThumbnail(in)
	case stingle.FileTypePhoto:
		thumbnail, err = c.photoThumbnail(in)
	default:
		thumbnail, err = c.GenericThumbnail(file)
	}
	if err != nil {
		// Fallback to a genetic thumbnail.
		thumbnail, err = c.GenericThumbnail(file)
	}
	if err != nil {
		return err
	}
	hdrs[1].DataSize = int64(len(thumbnail))
	hdrs[1].FileType = hdrs[0].FileType
	hdrs[1].VideoDuration = hdrs[0].VideoDuration

	encHdrs, err := stingle.EncryptBase64Headers(hdrs[:], pk)
	if err != nil {
		return err
	}
	sFile := stingle.File{
		File:         makeSPFilename(),
		Version:      "1",
		DateCreated:  json.Number(strconv.FormatInt(creationTime.UnixNano()/1000000, 10)),
		DateModified: json.Number(strconv.FormatInt(time.Now().UnixNano()/1000000, 10)),
		Headers:      encHdrs,
	}
	if dst.Album != nil {
		sFile.AlbumID = dst.Album.AlbumID
	}

	if _, err := in.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if err := c.encryptFile(in, sFile.File, hdrs[0], pk, false); err != nil {
		return err
	}
	if err := c.encryptFile(bytes.NewBuffer(thumbnail), sFile.File, hdrs[1], pk, true); err != nil {
		return err
	}
	commit, fs, err := c.fileSetForUpdate(dst.FileSet)
	if err != nil {
		return err
	}
	fs.Files[sFile.File] = &sFile
	return commit(true, nil)
}

func makeSPFilename() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b) + ".sp"
}

func (c *Client) importExif(file string) (x *exif.Exif, err error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return exif.Decode(f)
}

func (c *Client) GenericThumbnail(filename string) ([]byte, error) {
	_, filename = filepath.Split(filename)
	var ext string
	if filename[0] == '.' {
		ext = filepath.Ext(filename[1:])
	} else {
		ext = filepath.Ext(filename)
	}
	filename = filename[:len(filename)-len(ext)]
	img := image.NewRGBA(image.Rect(0, 0, 120, 120))

	for _, label := range []struct {
		txt  string
		x, y int
		col  color.RGBA
	}{
		{filename, 10, 20, color.RGBA{200, 200, 200, 255}},
		{ext, 10, 40, color.RGBA{200, 200, 200, 255}},
	} {
		point := fixed.Point26_6{X: fixed.Int26_6(label.x * 64), Y: fixed.Int26_6(label.y * 64)}
		d := &font.Drawer{
			Dst:  img,
			Src:  image.NewUniform(label.col),
			Face: inconsolata.Bold8x16,
			Dot:  point,
		}
		d.DrawString(label.txt)
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (c *Client) photoThumbnail(file io.Reader) ([]byte, error) {
	img, err := imaging.Decode(file, imaging.AutoOrientation(true))
	if err != nil {
		return nil, err
	}
	img = imaging.Fill(img, 240, 320, imaging.Center, imaging.Lanczos)

	var buf bytes.Buffer
	if err := imaging.Encode(&buf, img, imaging.PNG); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (c *Client) videoThumbnail(file io.Reader) ([]byte, error) {
	bin, err := exec.LookPath("ffmpeg")
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(bin, "-i", "pipe:0", "-frames:v", "1", "-an", "-vf", "thumbnail,scale=320:240", "-f", "apng", "pipe:1")
	cmd.Stdin = file
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	b, err := cmd.Output()
	if err != nil {
		log.Errorf("ffmpeg: %s", stderr.String())
		return nil, err
	}
	return b, nil
}

func videoMetadata(file io.Reader) (duration int32, creationTime time.Time, err error) {
	bin, err := exec.LookPath("ffprobe")
	if err != nil {
		return
	}
	var streamInfo struct {
		Streams []struct {
			Duration json.Number `json:"duration"`
			Tags     struct {
				CreationTime string `json:"creation_time"`
			} `json:"tags"`
		} `json:"streams"`
	}
	cmd := exec.Command(bin, "-show_streams", "-print_format", "json", "-")
	cmd.Stdin = file
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	b, err := cmd.Output()
	if err != nil {
		log.Errorf("ffprobe: %s", stderr.String())
		return
	}
	if err = json.Unmarshal(b, &streamInfo); err != nil {
		log.Errorf("ffprobe json: %v", err)
		return
	}
	if len(streamInfo.Streams) > 0 {
		d, _ := streamInfo.Streams[0].Duration.Float64()
		duration = int32(math.Floor(d))
		// Format: 2021-03-28T17:02:12.000000Z
		creationTime, _ = time.Parse("2006-01-02T15:04:05.000000Z", streamInfo.Streams[0].Tags.CreationTime)
	}
	return
}

func (c *Client) encryptFile(in io.Reader, file string, hdr *stingle.Header, pk stingle.PublicKey, thumb bool) error {
	fn := c.blobPath(file, thumb)
	dir, _ := filepath.Split(fn)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	tmp := fmt.Sprintf("%s-tmp-%d", fn, time.Now().UnixNano())
	out, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL|os.O_SYNC, 0600)
	if err != nil {
		return err
	}
	if err := stingle.EncryptHeader(out, hdr, pk); err != nil {
		out.Close()
		return err
	}
	w := stingle.EncryptFile(out, hdr)
	if _, err := io.Copy(w, in); err != nil {
		w.Close()
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, fn)
}
