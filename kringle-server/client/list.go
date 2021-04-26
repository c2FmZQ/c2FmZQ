package client

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rwcarlsen/goexif/exif"

	"kringle-server/stingle"
)

// ListItem is the information returned by GlobFiles() for each item.
type ListItem struct {
	Filename  string         // kringle representation, e.g. album/ or album/file.jpg
	IsDir     bool           // Whether this is a directory, i.e. gallery, trash, or album.
	Header    stingle.Header // The decrypted file header.
	FilePath  string         // Path where the file content is stored.
	FileSet   string         // Path where the FileSet is stored.
	FSFile    stingle.File   // The stingle.File object for this item.
	DirSize   int            // The number of items in the directory.
	Set       string         // The Set value, i.e. "0" for gallery, "1" for trash, "2" for albums.
	Album     *stingle.Album // Pointer to stingle.Album if this is part of an album.
	LocalOnly bool           // Indicates that this item only exists locally.
}

// GlobFiles returns files that match the glob patterns.
func (c *Client) GlobFiles(patterns []string) ([]ListItem, error) {
	var li []ListItem
	for _, p := range patterns {
		items, err := c.glob(p)
		if err != nil {
			return nil, err
		}
		if len(items) == 0 {
			fmt.Fprintf(c.writer, "no match for: %s\n", p)
		}
		li = append(li, items...)
	}
	return li, nil
}

// glob returns files that match the glob pattern.
func (c *Client) glob(pattern string) ([]ListItem, error) {
	// Sanity check the pattern.
	if _, err := path.Match(pattern, ""); err != nil {
		return nil, err
	}
	pattern = strings.TrimSuffix(pattern, "/")
	// The directory structure can only be 2 deep.
	pathElems := strings.SplitN(pattern, "/", 2)
	if len(pathElems) > 2 {
		return nil, nil
	}

	type dir struct {
		name    string
		fileSet string
		set     string
		sk      stingle.SecretKey
		album   *stingle.Album
		local   bool
	}
	dirs := []dir{
		{"gallery", galleryFile, stingle.GallerySet, c.SecretKey, nil, false},
		{"trash", trashFile, stingle.TrashSet, c.SecretKey, nil, false},
	}
	var al AlbumList
	if _, err := c.storage.ReadDataFile(c.fileHash(albumList), &al); err != nil {
		return nil, err
	}
	for _, album := range al.Albums {
		local := al.RemoteAlbums[album.AlbumID] == nil
		ask, err := album.SK(c.SecretKey)
		if err != nil {
			return nil, err
		}
		md, err := stingle.DecryptAlbumMetadata(album.Metadata, ask)
		if err != nil {
			return nil, err
		}
		dirs = append(dirs, dir{md.Name, albumPrefix + album.AlbumID, stingle.AlbumSet, ask, album, local})
	}

	var out []ListItem
	for _, d := range dirs {
		if d.album != nil && d.album.IsHidden == "1" && pathElems[0] != d.name {
			continue
		} else if matched, _ := path.Match(pathElems[0], d.name); !matched {
			continue
		}
		var fs FileSet
		if _, err := c.storage.ReadDataFile(c.fileHash(d.fileSet), &fs); err != nil {
			return nil, err
		}
		// Only show directories.
		if len(pathElems) == 1 {
			li := ListItem{
				Filename:  d.name + "/",
				FileSet:   d.fileSet,
				IsDir:     true,
				DirSize:   len(fs.Files),
				Set:       d.set,
				Album:     d.album,
				LocalOnly: d.local,
			}
			out = append(out, li)
			continue
		}
		// Look for matching files.
		for _, f := range fs.Files {
			local := fs.RemoteFiles[f.File] == nil
			hdrs, err := stingle.DecryptBase64Headers(f.Headers, d.sk)
			if err != nil {
				return nil, err
			}
			fn := string(hdrs[0].Filename)
			if matched, _ := path.Match(pathElems[1], fn); matched {
				out = append(out, ListItem{
					Filename:  d.name + "/" + fn,
					Header:    hdrs[0],
					FilePath:  c.blobPath(f.File, false),
					FileSet:   d.fileSet,
					FSFile:    *f,
					Set:       d.set,
					Album:     d.album,
					LocalOnly: local,
				})
			}
		}
	}
	return out, nil
}

func (c *Client) ListFiles(patterns []string) error {
	li, err := c.GlobFiles(patterns)
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
	var cl ContactList
	if _, err := c.storage.ReadDataFile(c.fileHash(contactsFile), &cl); err != nil {
		return err
	}

	var out []string
	for _, item := range li {
		if item.IsDir {
			s := fmt.Sprintf("%*s %6d file", -maxFilenameWidth, item.Filename, item.DirSize)
			if item.DirSize != 1 {
				s += "s"
			}
			if item.Album != nil && item.Album.IsShared == "1" {
				if item.Album.IsOwner == "1" {
					s += ", shared by me"
				} else {
					s += ", shared with me"
				}
				p := strings.Split(item.Album.Members, ",")
				s += fmt.Sprintf(", %d members: ", len(p))
				var ml []string
				for _, m := range p {
					id, _ := strconv.ParseInt(m, 10, 64)
					if id == c.UserID {
						ml = append(ml, c.Email)
						continue
					}
					ml = append(ml, cl.Contacts[id].Email)
				}
				sort.Strings(ml)
				s += strings.Join(ml, ", ")
			}
			if item.LocalOnly {
				s += ", Local"
			}
			s += "\n"
			out = append(out, s)
			continue
		}
		duration := ""
		if item.Header.FileType == stingle.FileTypeVideo {
			duration = fmt.Sprintf(" %s", time.Duration(item.Header.VideoDuration)*time.Second)
		}

		exifData := ""
		if x, err := c.getExif(item); err == nil {
			sizeX, _ := x.Get("PixelXDimension")
			sizeY, _ := x.Get("PixelYDimension")
			if sizeX != nil && sizeY != nil {
				exifData = fmt.Sprintf(" Size: %sx%s", sizeX, sizeY)
			}
			if lat, lon, err := x.LatLong(); err == nil {
				exifData = exifData + fmt.Sprintf(" GPS: %f,%f", lat, lon)
			}
		}
		local := ""
		if item.LocalOnly {
			local = " Local"
		}
		ms, _ := item.FSFile.DateCreated.Int64()
		out = append(out, fmt.Sprintf("%*s %*d %s %s%s%s%s\n", -maxFilenameWidth, item.Filename, maxSizeWidth, item.Header.DataSize,
			time.Unix(ms/1000, 0).Format("2006-01-02 15:04:05"), stingle.FileType(item.Header.FileType),
			exifData, duration, local))
	}
	sort.Strings(out)
	for _, l := range out {
		fmt.Fprint(c.writer, l)
	}
	return nil
}

func (c *Client) getExif(item ListItem) (x *exif.Exif, err error) {
	if item.Header.FileType != stingle.FileTypePhoto {
		return nil, errors.New("not a photo")
	}
	var f io.ReadCloser
	if f, err = os.Open(item.FilePath); errors.Is(err, os.ErrNotExist) {
		//f, err = c.download(item.File, item.Set, "0")
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if err := stingle.SkipHeader(f); err != nil {
		return nil, err
	}
	return exif.Decode(stingle.DecryptFile(f, item.Header))
}
