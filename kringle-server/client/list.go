package client

import (
	"errors"
	"fmt"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/rwcarlsen/goexif/exif"

	"kringle-server/stingle"
)

type ListItem struct {
	Filename    string
	Header      stingle.Header
	FilePath    string
	DateCreated time.Time
	File        string
	AlbumID     string
	IsDir       bool
	DirSize     int
}

// GlobFiles returns files that match the glob pattern.
func (c *Client) GlobFiles(pattern string) ([]ListItem, error) {
	if _, err := path.Match(pattern, ""); err != nil {
		return nil, err
	}
	pathElems := strings.SplitN(pattern, "/", 2)
	if len(pathElems) > 2 {
		return nil, nil
	}
	if len(pathElems) == 0 || (len(pathElems) == 1 && pathElems[0] == "") {
		pathElems = []string{"*"}
	}

	var al AlbumList
	if _, err := c.storage.ReadDataFile(c.storage.HashString(albumList), &al); err != nil {
		return nil, err
	}
	type dir struct {
		name string
		file string
		sk   stingle.SecretKey
	}
	dirs := []dir{
		{"gallery", galleryFile, c.SecretKey},
		{"trash", trashFile, c.SecretKey},
	}
	for _, album := range al.Albums {
		askBytes, err := c.SecretKey.SealBoxOpenBase64(album.EncPrivateKey)
		if err != nil {
			return nil, err
		}
		ask := stingle.SecretKeyFromBytes(askBytes)
		md, err := stingle.DecryptAlbumMetadata(album.Metadata, ask)
		if err != nil {
			return nil, err
		}
		dirs = append(dirs, dir{md.Name, albumPrefix + album.AlbumID, ask})
	}
	var out []ListItem
	for _, d := range dirs {
		if matched, _ := path.Match(pathElems[0], d.name); !matched {
			continue
		}
		var fs FileSet
		if _, err := c.storage.ReadDataFile(c.storage.HashString(d.file), &fs); err != nil {
			return nil, err
		}
		if len(pathElems) == 1 {
			out = append(out, ListItem{Filename: d.name + "/", IsDir: true, DirSize: len(fs.Files)})
			continue
		}
		for _, f := range fs.Files {
			hdrs, err := stingle.DecryptBase64Headers(f.Headers, d.sk)
			if err != nil {
				return nil, err
			}
			fn := string(hdrs[0].Filename)
			if matched, _ := path.Match(pathElems[1], fn); matched {
				ts, _ := f.DateCreated.Int64()
				out = append(out, ListItem{
					Filename:    d.name + "/" + fn,
					Header:      hdrs[0],
					FilePath:    c.filepath(f.File, false),
					DateCreated: time.Unix(ts/1000, 0),
					File:        f.File,
					AlbumID:     f.AlbumID,
				})
			}
		}
	}
	return out, nil
}

func (c *Client) ListFiles(pattern string) error {
	li, err := c.GlobFiles(pattern)
	if err != nil {
		return err
	}
	maxFilenameWidth, maxSizeWidth := 0, 0
	for _, item := range li {
		if len(item.Filename) > maxFilenameWidth {
			maxFilenameWidth = len(item.Filename)
		}
		w := len(fmt.Sprintf("%d", item.Header.DataSize))
		if w > maxSizeWidth {
			maxSizeWidth = w
		}
	}

	var out []string
	for _, item := range li {
		if item.IsDir {
			s := ""
			if item.DirSize != 1 {
				s = "s"
			}
			out = append(out, fmt.Sprintf("%*s %6d file%s\n", -maxFilenameWidth, item.Filename, item.DirSize, s))
			continue
		}
		duration := ""
		if item.Header.FileType == stingle.FileTypeVideo {
			duration = fmt.Sprintf(" %s", time.Duration(item.Header.VideoDuration)*time.Second)
		}

		exifData := ""
		if x, err := getExif(item.FilePath, item.Header); err == nil {
			sizeX, _ := x.Get("PixelXDimension")
			sizeY, _ := x.Get("PixelYDimension")
			if sizeX != nil && sizeY != nil {
				exifData = fmt.Sprintf(" Size: %sx%s", sizeX, sizeY)
			}
			if lat, lon, err := x.LatLong(); err == nil {
				exifData = exifData + fmt.Sprintf(" GPS: %f,%f", lat, lon)
			}
		} else if errors.Is(err, os.ErrNotExist) {
			exifData = " (remote only)"
		}
		out = append(out, fmt.Sprintf("%*s %*d %s %s%s%s\n", -maxFilenameWidth, item.Filename, maxSizeWidth, item.Header.DataSize,
			item.DateCreated.Format("2006-01-02 15:04:05"), stingle.FileType(item.Header.FileType),
			exifData, duration))
	}
	sort.Strings(out)
	for _, l := range out {
		fmt.Print(l)
	}
	return nil
}

func getExif(filepath string, hdr stingle.Header) (*exif.Exif, error) {
	if hdr.FileType != stingle.FileTypePhoto {
		return nil, errors.New("not a photo")
	}
	f, err := os.Open(filepath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if err := stingle.SkipHeader(f); err != nil {
		return nil, err
	}
	return exif.Decode(stingle.DecryptFile(f, hdr))
}
