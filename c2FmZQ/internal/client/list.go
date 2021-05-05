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

	"c2FmZQ/internal/stingle"
)

// ListItem is the information returned by GlobFiles() for each item.
type ListItem struct {
	Filename  string         // c2FmZQ representation, e.g. album/ or album/file.jpg
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

// GlobOptions contains options for GlobFiles and ListFiles.
type GlobOptions struct {
	// Glob options
	MatchDot   bool // Wildcards match dot at the beginning of dir/file names.
	Quiet      bool // Don't show errors.
	Recursive  bool // Traverse tree recursively.
	ExactMatch bool // pattern is an exact name to match, i.e. no wildcards.

	// List options
	Long      bool // Show long output.
	Directory bool // Show directories themselves.

	trimPrefix string
}

var MatchAll = GlobOptions{MatchDot: true}

type node struct {
	name  string
	local bool
	dir   *dir
	file  *file

	children map[string]*node
}

type dir struct {
	fileSet string
	set     string
	sk      stingle.SecretKey
	album   *stingle.Album
}

type file struct {
	f       *stingle.File
	hdrs    []stingle.Header
	fileSet string
	set     string
	album   *stingle.Album
}

type glob struct {
	elems []string
	opt   GlobOptions
}

func (g *glob) matchFirstElem(n string) bool {
	if len(g.elems) == 0 {
		return g.opt.Recursive
	}
	if g.elems[0] == n {
		return true
	}
	if !g.opt.MatchDot && !strings.HasPrefix(g.elems[0], ".") && strings.HasPrefix(n, ".") {
		return false
	}
	if g.opt.ExactMatch {
		return g.elems[0] == n
	}
	matched, _ := path.Match(g.elems[0], n)
	return matched
}

func newNode(name string) *node {
	return &node{
		name:     name,
		children: make(map[string]*node),
	}
}

func (n *node) find(name string, create bool) *node {
	elems := strings.Split(name, "/")
	for {
		if len(elems) == 0 {
			return n
		}
		if len(elems[0]) == 0 {
			elems = elems[1:]
			continue
		}
		if next, ok := n.children[elems[0]]; ok {
			n = next
			elems = elems[1:]
			continue
		}
		if create {
			n.children[elems[0]] = newNode(elems[0])
			n = n.children[elems[0]]
			elems = elems[1:]
			continue
		}
		return nil
	}
}

func (n *node) insertDir(name, fileSet, set string, sk stingle.SecretKey, album *stingle.Album, local bool) {
	nn := n.find(name, true)
	nn.local = local
	nn.dir = &dir{
		fileSet: fileSet,
		set:     set,
		sk:      sk,
		album:   album,
	}
}

func (n *node) insertFile(name string, f *stingle.File, hdrs []stingle.Header, fileSet, set string, album *stingle.Album, local bool) {
	nn := n.find(name, true)
	nn.local = local
	nn.file = &file{
		f:       f,
		hdrs:    hdrs,
		fileSet: fileSet,
		set:     set,
		album:   album,
	}
}

// GlobFiles returns files that match the glob patterns.
func (c *Client) GlobFiles(patterns []string, opt GlobOptions) ([]ListItem, error) {
	var li []ListItem
	for _, p := range patterns {
		items, err := c.glob(p, opt)
		if err != nil {
			return nil, err
		}
		if len(items) == 0 && !opt.Quiet {
			fmt.Fprintf(c.writer, "no match for: %s\n", p)
		}
		li = append(li, items...)
	}
	sort.Slice(li, func(i, j int) bool {
		if li[i].Filename == li[j].Filename {
			if li[i].IsDir {
				return true
			}
			return false
		}
		return li[i].Filename < li[j].Filename
	})
	return li, nil
}

// glob returns files that match the glob pattern.
func (c *Client) glob(pattern string, opt GlobOptions) ([]ListItem, error) {
	// Sanity check the pattern.
	if _, err := path.Match(pattern, ""); err != nil {
		return nil, err
	}
	pattern = strings.TrimSuffix(pattern, "/")
	g := &glob{opt: opt}
	g.elems = strings.Split(pattern, "/")

	root := newNode("")
	root.insertDir("gallery", galleryFile, stingle.GallerySet, c.SecretKey(), nil, false)
	root.insertDir(".trash", trashFile, stingle.TrashSet, c.SecretKey(), nil, false)
	var al AlbumList
	if err := c.storage.ReadDataFile(c.fileHash(albumList), &al); err != nil {
		return nil, fmt.Errorf("albumList: %w", err)
	}
	for _, album := range al.Albums {
		local := al.RemoteAlbums[album.AlbumID] == nil
		ask, err := album.SK(c.SecretKey())
		if err != nil {
			return nil, err
		}
		md, err := stingle.DecryptAlbumMetadata(album.Metadata, ask)
		if err != nil {
			return nil, err
		}
		root.insertDir(md.Name, albumPrefix+album.AlbumID, stingle.AlbumSet, ask, album, local)
	}

	var out []ListItem
	if err := c.globStep("", g, root, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) globStep(parent string, g *glob, n *node, li *[]ListItem) error {
	if n.dir != nil {
		var fs FileSet
		if err := c.storage.ReadDataFile(c.fileHash(n.dir.fileSet), &fs); err != nil {
			return err
		}
		for _, f := range fs.Files {
			local := fs.RemoteFiles[f.File] == nil
			hdrs, err := stingle.DecryptBase64Headers(f.Headers, n.dir.sk)
			if err != nil {
				return err
			}
			fn := string(hdrs[0].Filename)
			n.insertFile(fn, f, hdrs, n.dir.fileSet, n.dir.set, n.dir.album, local)
		}
	}
	if len(g.elems) == 0 {
		if n.dir != nil {
			*li = append(*li, ListItem{
				Filename:  path.Join(parent, n.name),
				FileSet:   n.dir.fileSet,
				IsDir:     true,
				DirSize:   len(n.children),
				Set:       n.dir.set,
				Album:     n.dir.album,
				LocalOnly: n.local,
			})
		} else if n.file != nil {
			*li = append(*li, ListItem{
				Filename:  path.Join(parent, n.name),
				Header:    n.file.hdrs[0],
				FilePath:  c.blobPath(n.file.f.File, false),
				FileSet:   n.file.fileSet,
				FSFile:    *n.file.f,
				Set:       n.file.set,
				Album:     n.file.album,
				LocalOnly: n.local,
			})
		} else {
			*li = append(*li, ListItem{
				Filename:  path.Join(parent, n.name),
				IsDir:     true,
				LocalOnly: true,
			})
		}
		if !g.opt.Recursive {
			return nil
		}
	}

	gg := &glob{opt: g.opt}
	if len(g.elems) > 0 {
		gg.elems = g.elems[1:]
	}
	for _, child := range n.children {
		if g.matchFirstElem(child.name) {
			if err := c.globStep(path.Join(parent, n.name), gg, child, li); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *Client) ListFiles(patterns []string, opt GlobOptions) error {
	for i, p := range patterns {
		if p == "" {
			p = "*"
			opt.Directory = true
			patterns[i] = p
		}
	}
	li, err := c.GlobFiles(patterns, opt)
	if err != nil {
		return err
	}
	maxFilenameWidth, maxSizeWidth := 0, 0
	for _, item := range li {
		fn := strings.TrimPrefix(item.Filename+"/", opt.trimPrefix)
		if len(fn) > maxFilenameWidth {
			maxFilenameWidth = len(fn)
		}
		w := len(fmt.Sprintf("%d", item.Header.DataSize))
		if w > maxSizeWidth {
			maxSizeWidth = w
		}
	}
	var cl ContactList
	if err := c.storage.ReadDataFile(c.fileHash(contactsFile), &cl); err != nil {
		return err
	}

	var expand []string
	fileCount := 0
	for _, item := range li {
		if item.IsDir {
			if !opt.Directory && !opt.Recursive {
				expand = append(expand, item.Filename)
			} else if !opt.Long {
				c.Print(strings.TrimPrefix(item.Filename+"/", opt.trimPrefix))
			} else if item.Set == "" {
				c.Printf("%*s %*s -\n", -maxFilenameWidth, strings.TrimPrefix(item.Filename+"/", opt.trimPrefix), maxSizeWidth, "")
			} else {
				s := fmt.Sprintf("%*s %*d file", -maxFilenameWidth, strings.TrimPrefix(item.Filename+"/", opt.trimPrefix), maxSizeWidth, item.DirSize)
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
						if c.Account != nil && id == c.Account.UserID {
							ml = append(ml, c.Account.Email)
							continue
						}
						ml = append(ml, cl.Contacts[id].Email)
					}
					sort.Strings(ml)
					s += strings.Join(ml, ",")
					s += ", Permissions: " + stingle.Permissions(item.Album.Permissions).Human()
				}
				if item.LocalOnly {
					s += ", Local"
				}
				c.Print(s)
			}
			continue
		}
		fileCount++
		if !opt.Long {
			c.Print(strings.TrimPrefix(item.Filename, opt.trimPrefix))
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
		c.Printf("%*s %*d %s %s%s%s%s\n", -maxFilenameWidth,
			strings.TrimPrefix(item.Filename, opt.trimPrefix), maxSizeWidth, item.Header.DataSize,
			time.Unix(ms/1000, 0).Format("2006-01-02 15:04:05"), stingle.FileType(item.Header.FileType),
			exifData, duration, local)
	}
	if fileCount > 0 && len(expand) > 0 {
		c.Print()
	}
	opt.Quiet = true
	opt.Directory = true
	for i, d := range expand {
		if i > 0 {
			c.Print()
		}
		opt.trimPrefix = d + "/"
		c.Printf("%s:\n", d)
		c.ListFiles([]string{path.Join(d, "*")}, opt)
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
