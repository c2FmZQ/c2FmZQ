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
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/disintegration/imaging"
	"github.com/rwcarlsen/goexif/exif"

	"kringle/log"
	"kringle/stingle"
)

// ImportFiles encrypts and imports files. Returns the number of files imported.
func (c *Client) ImportFiles(patterns []string, dir string) (int, error) {
	dir = strings.TrimSuffix(dir, "/")
	li, err := c.glob(dir)
	if err != nil {
		return 0, err
	}
	if len(li) != 1 || !li[0].IsDir {
		return 0, fmt.Errorf("%s is not a directory", dir)
	}
	dst := li[0]
	pk := c.SecretKey.PublicKey()
	if dst.Album != nil {
		if dst.Album.IsOwner != "1" && !stingle.Permissions(dst.Album.Permissions).AllowAdd() {
			return 0, fmt.Errorf("adding is not allowed: %s", dir)
		}
		if pk, err = dst.Album.PK(); err != nil {
			return 0, err
		}
	}

	existingItems, err := c.glob(dir + "/*")
	if err != nil {
		return 0, err
	}
	exist := make(map[string]bool)
	for _, item := range existingItems {
		_, fn := filepath.Split(item.Filename)
		exist[fn] = true
	}

	var files []string
	for _, p := range patterns {
		m, err := filepath.Glob(p)
		if err != nil {
			return 0, err
		}
		files = append(files, m...)
	}
	count := 0
	for _, file := range files {
		_, fn := filepath.Split(file)
		if exist[fn] {
			fmt.Fprintf(c.writer, "Skipping %s (already exists in %s)\n", file, dir)
			continue
		}
		fmt.Fprintf(c.writer, "Importing %s -> %s\n", file, dir)
		if err := c.importFile(file, dst, pk); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func (c *Client) importFile(file string, dst ListItem, pk stingle.PublicKey) error {
	fi, err := os.Stat(file)
	if err != nil {
		return err
	}
	_, fn := filepath.Split(file)
	hdrs := stingle.NewHeaders(fn)
	hdrs[0].DataSize = fi.Size()

	creationTime := time.Now()
	switch ext := strings.ToLower(filepath.Ext(file)); ext {
	case ".jpg", ".jpeg", ".png", ".gif":
		hdrs[0].FileType = stingle.FileTypePhoto
	case ".mp4", ".mov":
		hdrs[0].FileType = stingle.FileTypeVideo
	default:
		hdrs[0].FileType = stingle.FileTypeGeneral
	}
	if hdrs[0].FileType == stingle.FileTypeVideo {
		if dur, ct, err := videoMetadata(file); err == nil {
			hdrs[0].VideoDuration = dur
			if !ct.IsZero() {
				creationTime = ct
			}
		}
	}
	var thumbnail []byte
	switch hdrs[0].FileType {
	case stingle.FileTypeVideo:
		thumbnail, err = c.videoThumbnail(file)
	case stingle.FileTypePhoto:
		thumbnail, err = c.photoThumbnail(file)
	default:
		thumbnail, err = c.genericThumbnail(file)
	}
	if err != nil {
		// Fallback to a genetic thumbnail.
		thumbnail, err = c.genericThumbnail(file)
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
	if x, err := c.importExif(file); err == nil {
		if t, err := x.DateTime(); err == nil {
			sFile.DateCreated = json.Number(strconv.FormatInt(t.UnixNano()/1000000, 10))
		}
	}
	in, err := os.Open(file)
	if err != nil {
		return err
	}
	defer in.Close()
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
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
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

func (c *Client) genericThumbnail(filename string) ([]byte, error) {
	_, filename = filepath.Split(filename)
	ext := filepath.Ext(filename)
	filename = filename[:len(filename)-len(ext)]
	img := image.NewRGBA(image.Rect(0, 0, 120, 120))

	for _, label := range []struct {
		txt  string
		x, y int
		col  color.RGBA
	}{
		{filename, 10, 10, color.RGBA{200, 200, 200, 255}},
		{ext, 10, 30, color.RGBA{200, 200, 200, 255}},
	} {
		point := fixed.Point26_6{fixed.Int26_6(label.x * 64), fixed.Int26_6(label.y * 64)}
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

func (c *Client) photoThumbnail(filename string) ([]byte, error) {
	img, err := imaging.Open(filename, imaging.AutoOrientation(true))
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

func (c *Client) videoThumbnail(file string) ([]byte, error) {
	bin, err := exec.LookPath("ffmpeg")
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(bin, "-i", file, "-frames:v", "1", "-an", "-vf", "scale=320:240", "-f", "apng", "pipe:1")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	b, err := cmd.Output()
	if err != nil {
		log.Errorf("ffmpeg: %s", stderr.String())
		return nil, err
	}
	return b, nil
}

func videoMetadata(file string) (duration int32, creationTime time.Time, err error) {
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
	cmd := exec.Command(bin, "-show_streams", "-print_format", "json", file)
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
		duration = int32(math.Ceil(d))
		// Format: 2021-03-28T17:02:12.000000Z
		creationTime, _ = time.Parse("2006-01-02T15:04:05.000000Z", streamInfo.Streams[0].Tags.CreationTime)
	}
	return
}

func (c *Client) encryptFile(in io.Reader, file string, hdr stingle.Header, pk stingle.PublicKey, thumb bool) error {
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
