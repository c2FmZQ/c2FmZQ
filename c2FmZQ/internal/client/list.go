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
	"unicode"

	"github.com/rwcarlsen/goexif/exif"

	"c2FmZQ/internal/stingle"
)

// ListItem is the information returned by GlobFiles() for each item.
type ListItem struct {
	Filename  string         // c2FmZQ representation, e.g. album/ or album/file.jpg
	IsDir     bool           // Whether this is a directory, i.e. gallery, trash, or album.
	FilePath  string         // Path where the file content is stored.
	FileSet   string         // Path where the FileSet is stored.
	FSFile    stingle.File   // The stingle.File object for this item.
	Size      int64          // The file size.
	DirSize   int            // The number of items in the directory.
	Set       string         // The Set value, i.e. "0" for gallery, "1" for trash, "2" for albums.
	Album     *stingle.Album // Pointer to stingle.Album if this is part of an album.
	LocalOnly bool           // Indicates that this item only exists locally.
}

// GlobOptions contains options for GlobFiles and ListFiles.
type GlobOptions struct {
	// Glob options
	MatchDot             bool // Wildcards match dot at the beginning of dir/file names.
	Quiet                bool // Don't show errors.
	Recursive            bool // Traverse tree recursively.
	ExactMatch           bool // pattern is an exact name to match, i.e. no wildcards.
	ExactMatchExceptLast bool // pattern is an exact match except for the last element.

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
	album   *stingle.Album
}

type file struct {
	f       *stingle.File
	size    int64
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
	if g.opt.ExactMatch || (g.opt.ExactMatchExceptLast && len(g.elems) > 1) {
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

func (n *node) insertDir(name, fileSet, set string, album *stingle.Album, local bool) {
	var nn *node
	for i := 0; ; i++ {
		nodeName := name
		if i > 0 {
			nodeName = fmt.Sprintf("%s (%d)", name, i)
		}
		nn = n.find(nodeName, true)
		if nn.dir == nil && nn.file == nil {
			break
		}
	}
	nn.local = local
	nn.dir = &dir{
		fileSet: fileSet,
		set:     set,
		album:   album,
	}
}

func (n *node) insertFile(name string, size int64, f *stingle.File, fileSet, set string, album *stingle.Album, local bool) {
	var nn *node
	for i := 0; ; i++ {
		nodeName := name
		if i > 0 {
			nodeName = fmt.Sprintf("%s (%d)", name, i)
		}
		nn = n.find(nodeName, true)
		if nn.dir == nil && nn.file == nil {
			break
		}
	}
	nn.local = local
	nn.file = &file{
		f:       f,
		size:    size,
		fileSet: fileSet,
		set:     set,
		album:   album,
	}
}

func sanitize(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		s = "(noname)"
	} else if s == "." {
		s = "(dot)"
	} else if s == ".." {
		s = "(dotdot)"
	}
	return strings.Map(func(r rune) rune {
		if !unicode.IsPrint(r) {
			return unicode.ReplacementChar
		}
		return r
	}, s)
}

// Header returns the decrypted Header.
func (i ListItem) Header(sk *stingle.SecretKey) (*stingle.Header, error) {
	if a := i.Album; a != nil {
		ask, err := a.SK(sk)
		if err != nil {
			return nil, err
		}
		defer ask.Wipe()
		sk = ask
	}
	hdrs, err := stingle.DecryptBase64Headers(i.FSFile.Headers, sk)
	if err != nil {
		return nil, err
	}
	hdrs[1].Wipe()
	return hdrs[0], nil
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
	root.insertDir("gallery", galleryFile, stingle.GallerySet, nil, false)
	root.insertDir(".trash", trashFile, stingle.TrashSet, nil, false)
	var al AlbumList
	if err := c.storage.ReadDataFile(c.fileHash(albumList), &al); err != nil {
		return nil, fmt.Errorf("albumList: %w", err)
	}
	var albumIDs []string
	for albumID := range al.Albums {
		albumIDs = append(albumIDs, albumID)
	}
	sort.Strings(albumIDs)
	for _, albumID := range albumIDs {
		album := al.Albums[albumID]
		local := al.RemoteAlbums[albumID] == nil
		ask, err := c.SKForAlbum(album)
		if err != nil {
			return nil, err
		}
		md, err := stingle.DecryptAlbumMetadata(album.Metadata, ask)
		ask.Wipe()
		if err != nil {
			return nil, err
		}
		name := sanitize(md.Name)
		if album.IsShared == "1" && album.IsOwner != "1" {
			name = path.Join("shared", name)
		}
		root.insertDir(name, albumPrefix+album.AlbumID, stingle.AlbumSet, album, local)
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
		var files []string
		for file := range fs.Files {
			files = append(files, file)
		}
		sort.Strings(files)
		for _, file := range files {
			f := fs.Files[file]
			local := fs.RemoteFiles[f.File] == nil
			sk, err := c.SKForAlbum(n.dir.album)
			if err != nil {
				return err
			}
			hdrs, err := stingle.DecryptBase64Headers(f.Headers, sk)
			sk.Wipe()
			if err != nil {
				return err
			}
			fn := sanitize(string(hdrs[0].Filename))
			n.insertFile(fn, hdrs[0].DataSize, f, n.dir.fileSet, n.dir.set, n.dir.album, local)
			hdrs[0].Wipe()
			hdrs[1].Wipe()
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
				Size:      n.file.size,
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
		w := len(fmt.Sprintf("%d", item.Size))
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
		sk := c.SecretKey()
		hdr, err := item.Header(sk)
		sk.Wipe()
		if err != nil {
			return err
		}

		if !opt.Long {
			c.Print(strings.TrimPrefix(item.Filename, opt.trimPrefix))
			hdr.Wipe()
			continue
		}
		duration := ""
		if hdr.FileType == stingle.FileTypeVideo {
			duration = fmt.Sprintf(" %s", time.Duration(hdr.VideoDuration)*time.Second)
		}

		exifData := ""
		if x, err := c.getExif(item, hdr); err == nil {
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
			strings.TrimPrefix(item.Filename, opt.trimPrefix), maxSizeWidth, item.Size,
			time.Unix(ms/1000, 0).Format("2006-01-02 15:04:05"), stingle.FileType(hdr.FileType),
			exifData, duration, local)
		hdr.Wipe()
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

func (c *Client) getExif(item ListItem, hdr *stingle.Header) (x *exif.Exif, err error) {
	if hdr.FileType != stingle.FileTypePhoto {
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
	return exif.Decode(stingle.DecryptFile(f, hdr))
}
