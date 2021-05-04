package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"c2FmZQ/internal/log"
	"c2FmZQ/internal/stingle"
)

type albumDiffs struct {
	AlbumsToAdd        []*stingle.Album
	AlbumsToRemove     []*stingle.Album
	AlbumsToRename     []*stingle.Album
	AlbumPermsToChange []*stingle.Album

	FilesToAdd    []FileLoc
	FilesToMove   []MoveItem
	FilesToDelete []string
}

type FileLoc struct {
	File    *stingle.File
	Set     string
	AlbumID string
}

type MoveKey struct {
	SetFrom     string
	AlbumIDFrom string
	SetTo       string
	AlbumIDTo   string
	Moving      bool
}

type MoveItem struct {
	key   MoveKey
	files []*stingle.File
}

// Sync synchronizes all metadata changes that have been made locally with the
// remote server.
func (c *Client) Sync(dryrun bool) error {
	if err := c.GetUpdates(true); err != nil {
		return err
	}
	d, err := c.diff()
	if err != nil {
		return err
	}
	if d.AlbumsToAdd == nil && d.AlbumsToRemove == nil && d.AlbumsToRename == nil && d.AlbumPermsToChange == nil &&
		d.FilesToAdd == nil && d.FilesToMove == nil && d.FilesToDelete == nil {
		c.Print("Already synced.")
		return nil
	}
	if err := c.applyDiffs(d, dryrun); err != nil {
		return err
	}
	if dryrun {
		return nil
	}
	return c.GetUpdates(true)
}

func (c *Client) applyDiffs(d *albumDiffs, dryrun bool) error {
	var al AlbumList
	if err := c.storage.ReadDataFile(c.fileHash(albumList), &al); err != nil {
		return err
	}
	if len(d.AlbumsToAdd) > 0 {
		if err := c.applyAlbumsToAdd(d.AlbumsToAdd, dryrun); err != nil {
			return err
		}
	}
	if len(d.AlbumsToRename) > 0 {
		if err := c.applyAlbumsToRename(d.AlbumsToRename, dryrun); err != nil {
			return err
		}
	}
	if len(d.AlbumPermsToChange) > 0 {
		if err := c.applyAlbumPermsToChange(d.AlbumPermsToChange, dryrun); err != nil {
			return err
		}
	}
	if len(d.FilesToAdd) > 0 {
		if err := c.applyFilesToAdd(d.FilesToAdd, al, dryrun); err != nil {
			return err
		}
	}
	if len(d.FilesToMove) > 0 {
		if err := c.applyFilesToMove(d.FilesToMove, al, dryrun); err != nil {
			return err
		}
	}
	if len(d.FilesToDelete) > 0 {
		if err := c.applyFilesToDelete(d.FilesToDelete, al, dryrun); err != nil {
			return err
		}
	}
	if len(d.AlbumsToRemove) > 0 {
		if err := c.applyAlbumsToRemove(d.AlbumsToRemove, dryrun); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) applyAlbumsToAdd(albums []*stingle.Album, dryrun bool) error {
	c.showAlbumsToSync("Albums to add:", albums)
	if dryrun {
		return nil
	}
	for _, album := range albums {
		if err := c.sendAddAlbum(album); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) applyAlbumsToRename(albums []*stingle.Album, dryrun bool) error {
	c.showAlbumsToSync("Albums to rename:", albums)
	if dryrun {
		return nil
	}
	for _, album := range albums {
		if err := c.sendRenameAlbum(album); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) applyAlbumPermsToChange(albums []*stingle.Album, dryrun bool) error {
	c.showAlbumsToSync("Album permissions to change:", albums)
	if dryrun {
		return nil
	}
	for _, album := range albums {
		if err := c.sendEditPerms(album); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) applyFilesToAdd(files []FileLoc, al AlbumList, dryrun bool) error {
	c.showFilesToSync("Files to add:", files, al)
	if dryrun {
		return nil
	}
	qCh := make(chan FileLoc)
	eCh := make(chan error)
	for i := 0; i < 5; i++ {
		go c.uploadWorker(qCh, eCh)
	}
	go func() {
		for _, f := range files {
			qCh <- f
		}
		close(qCh)
	}()
	var errors []error
	for range files {
		if err := <-eCh; err != nil {
			errors = append(errors, err)
		}
	}
	if errors != nil {
		return fmt.Errorf("%w %v", errors[0], errors[1:])
	}
	return nil
}

func (c *Client) applyFilesToMove(moves []MoveItem, al AlbumList, dryrun bool) error {
	c.Print("Files to move:")
	for _, i := range moves {
		src, err := c.translateSetAlbumIDToName(i.key.SetFrom, i.key.AlbumIDFrom, al)
		if err != nil {
			src = fmt.Sprintf("Set:%s Album:%s", i.key.SetFrom, i.key.AlbumIDFrom)
		}
		dst, err := c.translateSetAlbumIDToName(i.key.SetTo, i.key.AlbumIDTo, al)
		if err != nil {
			dst = fmt.Sprintf("Set:%s Album:%s", i.key.SetTo, i.key.AlbumIDTo)
		}
		op := "Moving"
		if !i.key.Moving {
			op = "Copying"
		}
		var files []string
		for _, f := range i.files {
			sk := c.SecretKey()
			if i.key.AlbumIDTo != "" {
				sk, err = al.Albums[i.key.AlbumIDTo].SK(sk)
				if err != nil {
					return err
				}
			}
			n, err := f.Name(sk)
			if err != nil {
				n = f.File
			}
			files = append(files, n)
		}
		c.Printf("* %s %s -> %s: %s\n", op, src, dst, strings.Join(files, ","))
	}
	if dryrun {
		return nil
	}
	for _, i := range moves {
		if err := c.sendMoveFiles(i.key, i.files); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) applyFilesToDelete(files []string, al AlbumList, dryrun bool) error {
	c.Print("Files to delete:")
	for _, f := range files {
		c.Printf("* trash/%s\n", f)
	}
	if dryrun {
		return nil
	}
	if err := c.sendDelete(files); err != nil {
		return err
	}
	return nil
}

func (c *Client) applyAlbumsToRemove(albums []*stingle.Album, dryrun bool) error {
	c.showAlbumsToSync("Albums to remove:", albums)
	if dryrun {
		return nil
	}
	for _, album := range albums {
		if album.IsOwner == "1" {
			if err := c.sendDeleteAlbum(album.AlbumID); err != nil {
				return err
			}
			continue
		}
		if err := c.sendLeaveAlbum(album.AlbumID); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) showAlbumsToSync(label string, albums []*stingle.Album) error {
	c.Print(label)
	for _, a := range albums {
		name, err := a.Name(c.SecretKey())
		if err != nil {
			return err
		}
		c.Printf("* %s\n", name)
	}
	return nil
}

func (c *Client) showFilesToSync(label string, files []FileLoc, al AlbumList) error {
	c.Print(label)
	for _, f := range files {
		sk := c.SecretKey()
		if album, ok := al.Albums[f.AlbumID]; ok {
			ask, err := album.SK(sk)
			if err != nil {
				return err
			} else {
				sk = ask
			}
		} else if album, ok := al.RemoteAlbums[f.AlbumID]; ok {
			ask, err := album.SK(sk)
			if err != nil {
				return err
			} else {
				sk = ask
			}
		}
		n, err := f.File.Name(sk)
		if err != nil {
			n = f.File.File
		}
		d, err := c.translateSetAlbumIDToName(f.Set, f.AlbumID, al)
		if err != nil {
			return err
		}
		c.Printf("* %s/%s\n", d, n)
	}
	return nil
}

func (c *Client) translateSetAlbumIDToName(set, albumID string, al AlbumList) (string, error) {
	switch set {
	case stingle.GallerySet:
		return "gallery", nil
	case stingle.TrashSet:
		return ".trash", nil
	case stingle.AlbumSet:
		if album, ok := al.Albums[albumID]; ok {
			return album.Name(c.SecretKey())
		}
		if album, ok := al.RemoteAlbums[albumID]; ok {
			return album.Name(c.SecretKey())
		}
		return "", fmt.Errorf("album not found: %s", albumID)
	default:
		return "", fmt.Errorf("invalid set: %s", set)
	}
}

func (c *Client) diff() (*albumDiffs, error) {
	var diffs albumDiffs

	// Diff album metadata.
	var al AlbumList
	if err := c.storage.ReadDataFile(c.fileHash(albumList), &al); err != nil {
		return nil, err
	}
	for albumID, album := range al.Albums {
		ra, ok := al.RemoteAlbums[albumID]
		if !ok {
			diffs.AlbumsToAdd = append(diffs.AlbumsToAdd, album)
			continue
		}
		if album.Metadata != ra.Metadata {
			diffs.AlbumsToRename = append(diffs.AlbumsToRename, album)
		}
		if album.IsHidden != ra.IsHidden || album.Permissions != ra.Permissions {
			diffs.AlbumPermsToChange = append(diffs.AlbumPermsToChange, album)
		}
	}
	for albumID, album := range al.RemoteAlbums {
		if _, ok := al.Albums[albumID]; !ok {
			diffs.AlbumsToRemove = append(diffs.AlbumsToRemove, album)
		}
	}
	// TODO: Add sharing diffs.

	// Diff files.
	//
	// The strategy is to make a list of where each file is, locally and remotely.
	// Then, find out where it was added or removed so that we can infer copy or
	// move operations.
	type dir struct {
		fileSet string
		set     string
		album   *stingle.Album
	}
	dirs := []dir{
		{galleryFile, stingle.GallerySet, nil},
		{trashFile, stingle.TrashSet, nil},
	}
	for _, album := range al.Albums {
		dirs = append(dirs, dir{albumPrefix + album.AlbumID, stingle.AlbumSet, album})
	}
	type setAlbum struct {
		set     string
		albumID string
		file    *stingle.File
	}
	fileLocations := make(map[string]map[setAlbum]*stingle.File)
	remoteFileLocations := make(map[string]map[setAlbum]*stingle.File)

	for _, d := range dirs {
		var fs FileSet
		if err := c.storage.ReadDataFile(c.fileHash(d.fileSet), &fs); err != nil {
			return nil, err
		}
		for fn, f := range fs.Files {
			sa := setAlbum{set: d.set}
			if d.album != nil {
				sa.albumID = d.album.AlbumID
			}
			if fileLocations[fn] == nil {
				fileLocations[fn] = make(map[setAlbum]*stingle.File)
			}
			fileLocations[fn][sa] = f
		}
		for fn, f := range fs.RemoteFiles {
			sa := setAlbum{set: d.set}
			if d.album != nil {
				sa.albumID = d.album.AlbumID
			}
			if remoteFileLocations[fn] == nil {
				remoteFileLocations[fn] = make(map[setAlbum]*stingle.File)
			}
			remoteFileLocations[fn][sa] = f
		}
	}

	type fileChange struct {
		add    []setAlbum
		remove []setAlbum
	}
	fileChanges := make(map[string]*fileChange)

	for fn, l := range fileLocations {
		rl := remoteFileLocations[fn]
		for sa, f := range l {
			if rl == nil || rl[sa] == nil {
				if fileChanges[fn] == nil {
					fileChanges[fn] = &fileChange{}
				}
				sa.file = f
				fileChanges[fn].add = append(fileChanges[fn].add, sa)
			}
		}
	}
	for fn, rl := range remoteFileLocations {
		l := fileLocations[fn]
		for sa, f := range rl {
			if l[sa] == nil {
				if fileChanges[fn] == nil {
					fileChanges[fn] = &fileChange{}
				}
				sa.file = f
				fileChanges[fn].remove = append(fileChanges[fn].remove, sa)
			}
		}
	}

	moves := make(map[MoveKey][]*stingle.File)
	deletes := make(map[string]struct{})

	for fn, changes := range fileChanges {
		var loc []setAlbum
		for l, f := range remoteFileLocations[fn] {
			l.file = f
			loc = append(loc, l)
		}
		sort.Slice(loc, func(i, j int) bool {
			if loc[i].set == loc[j].set {
				return loc[i].albumID < loc[j].albumID
			}
			return loc[i].set < loc[i].albumID
		})
		for _, add := range changes.add {
			if loc == nil {
				// File is added to the gallery or an album.
				if add.set != stingle.TrashSet {
					diffs.FilesToAdd = append(diffs.FilesToAdd, FileLoc{add.file, add.set, add.albumID})
					loc = []setAlbum{{set: add.set, albumID: add.albumID}}
					continue
				}
				// File is added to trash. First add to gallery, then move it to trash.
				diffs.FilesToAdd = append(diffs.FilesToAdd, FileLoc{add.file, stingle.GallerySet, ""})
				mk := MoveKey{
					SetFrom: stingle.GallerySet,
					SetTo:   stingle.TrashSet,
					Moving:  true,
				}
				moves[mk] = append(moves[mk], add.file)
				loc = []setAlbum{{set: stingle.TrashSet}}
				continue
			}
			from := loc[0]
			moving := false
			if len(changes.remove) > 0 {
				from = changes.remove[0]
				changes.remove = changes.remove[1:]
				moving = true
			}
			// File is moving from the gallery or an album.
			if from.set != stingle.TrashSet {
				if add.set == stingle.TrashSet {
					moving = true
				}
				mk := MoveKey{
					SetFrom:     from.set,
					AlbumIDFrom: from.albumID,
					SetTo:       add.set,
					AlbumIDTo:   add.albumID,
					Moving:      moving,
				}
				moves[mk] = append(moves[mk], add.file)
				continue
			}
			// File is moving from trash to gallery.
			if add.set == stingle.GallerySet {
				mk := MoveKey{
					SetFrom: stingle.TrashSet,
					SetTo:   stingle.GallerySet,
					Moving:  true,
				}
				moves[mk] = append(moves[mk], add.file)
				continue
			}
			// File is moving from trash to an album. First, move to gallery,
			// then move to album.
			mk := MoveKey{
				SetFrom: stingle.TrashSet,
				SetTo:   stingle.GallerySet,
				Moving:  true,
			}
			moves[mk] = append(moves[mk], from.file)

			loc = append([]setAlbum{setAlbum{set: stingle.GallerySet}}, loc...)

			mk = MoveKey{
				SetFrom:   stingle.GallerySet,
				SetTo:     add.set,
				AlbumIDTo: add.albumID,
				Moving:    true,
			}
			moves[mk] = append(moves[mk], add.file)
		}
		for _, remove := range changes.remove {
			if remove.set != stingle.TrashSet {
				// XXX: We should really update the headers when moving
				// from album to trash. But the file will be deleted
				// immediately after moving. So, it doesn't matter too
				// much.
				mk := MoveKey{
					SetFrom:     remove.set,
					AlbumIDFrom: remove.albumID,
					SetTo:       stingle.TrashSet,
					Moving:      true,
				}
				moves[mk] = append(moves[mk], remove.file)
			}
			deletes[remove.file.File] = struct{}{}
		}
	}

	for k, v := range moves {
		if k.SetFrom == stingle.TrashSet {
			diffs.FilesToMove = append(diffs.FilesToMove, MoveItem{key: k, files: v})
		}
	}
	for k, v := range moves {
		if k.SetFrom != stingle.TrashSet && k.SetTo != stingle.TrashSet && !k.Moving {
			diffs.FilesToMove = append(diffs.FilesToMove, MoveItem{key: k, files: v})
		}
	}
	for k, v := range moves {
		if k.SetFrom != stingle.TrashSet && k.SetTo != stingle.TrashSet && k.Moving {
			diffs.FilesToMove = append(diffs.FilesToMove, MoveItem{key: k, files: v})
		}
	}
	for k, v := range moves {
		if k.SetTo == stingle.TrashSet {
			diffs.FilesToMove = append(diffs.FilesToMove, MoveItem{key: k, files: v})
		}
	}
	for k := range deletes {
		diffs.FilesToDelete = append(diffs.FilesToDelete, k)
	}

	return &diffs, nil
}

// Pull downloads all the files matching pattern that are not already present
// in the local storage. Returns the number of files downloaded.
func (c *Client) Pull(patterns []string) (int, error) {
	list, err := c.GlobFiles(patterns, GlobOptions{})
	if err != nil {
		return 0, err
	}
	files := make(map[string]ListItem)
	for _, item := range list {
		if item.LocalOnly {
			continue
		}
		fn := c.blobPath(item.FSFile.File, false)
		if _, err := os.Stat(fn); errors.Is(err, os.ErrNotExist) {
			files[item.FSFile.File] = item
		}
	}

	qCh := make(chan ListItem)
	eCh := make(chan error)
	for i := 0; i < 5; i++ {
		go c.downloadWorker(qCh, eCh)
	}
	go func() {
		for _, li := range files {
			qCh <- li
		}
		close(qCh)
	}()
	var errors []error
	for range files {
		if err := <-eCh; err != nil {
			errors = append(errors, err)

		}
	}
	if len(files) == 0 {
		fmt.Fprintln(c.writer, "No files to download.")
	}
	count := len(files) - len(errors)
	if errors != nil {
		return count, fmt.Errorf("%w %v", errors[0], errors[1:])
	}
	return count, nil
}

// Free deletes all the files matching pattern that are already present in the
// remote storage. Returns the number of files freed.
func (c *Client) Free(patterns []string) (int, error) {
	list, err := c.GlobFiles(patterns, GlobOptions{})
	if err != nil {
		return 0, err
	}
	count := 0
	for _, item := range list {
		if item.LocalOnly {
			continue
		}
		fn := c.blobPath(item.FSFile.File, false)
		if _, err := os.Stat(fn); errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err := os.Remove(fn); err != nil {
			return count, err
		}
		c.Printf("Freed %s\n", item.Filename)
		count++
	}
	if count == 0 {
		fmt.Fprintln(c.writer, "There are no files to free.")
	}
	return count, nil
}

func (c *Client) blobPath(name string, thumb bool) string {
	if thumb {
		name = name + "-thumb"
	}
	return filepath.Join(c.storage.Dir(), c.fileHash(name))
}

func (c *Client) downloadWorker(ch <-chan ListItem, out chan<- error) {
	for i := range ch {
		c.Printf("Downloading %s\n", i.Filename)
		out <- c.downloadFile(i)
	}
}

func (c *Client) uploadWorker(ch <-chan FileLoc, out chan<- error) {
	for l := range ch {
		out <- c.uploadFile(l)
	}
}

func (c *Client) downloadFile(li ListItem) error {
	r, err := c.download(li.FSFile.File, li.Set, "0")
	if err != nil {
		return err
	}
	defer r.Close()
	fn := c.blobPath(li.FSFile.File, false)
	dir, _ := filepath.Split(fn)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	tmp := fmt.Sprintf("%s-tmp-%d", fn, time.Now().UnixNano())
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL|os.O_SYNC, 0600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, fn)
}

func (c *Client) uploadFile(item FileLoc) error {
	if c.Account == nil {
		return ErrNotLoggedIn
	}
	pr, pw := io.Pipe()
	w := multipart.NewWriter(pw)

	go func() {
		defer pw.Close()
		for _, f := range []string{"file", "thumb"} {
			pw, err := w.CreateFormFile(f, item.File.File)
			if err != nil {
				log.Errorf("multipart.CreateFormFile(%s): %v", item.File.File, err)
				return
			}
			in, err := os.Open(c.blobPath(item.File.File, f == "thumb"))
			if err != nil {
				log.Errorf("Open(%s): %v", item.File.File, err)
				return
			}
			if _, err := io.Copy(pw, in); err != nil {
				log.Errorf("Read(%s): %v", item.File.File, err)
				return
			}
			if err := in.Close(); err != nil {
				log.Errorf("Close(%s): %v", item.File.File, err)
				return
			}
		}
		for _, f := range []struct{ name, value string }{
			{"headers", item.File.Headers},
			{"set", item.Set},
			{"albumId", item.AlbumID},
			{"dateCreated", item.File.DateCreated.String()},
			{"dateModified", item.File.DateModified.String()},
			{"version", item.File.Version},
			{"token", c.Account.Token},
		} {
			pw, err := w.CreateFormField(f.name)
			if err != nil {
				log.Errorf("Metadata(%s): %v", item.File.File, err)
				return
			}
			if _, err := pw.Write([]byte(f.value)); err != nil {
				log.Errorf("Metadata(%s): %v", item.File.File, err)
				return
			}
		}
		if err := w.Close(); err != nil {
			log.Errorf("multipart.Writer(%s): %v", item.File.File, err)
			return
		}
	}()

	url := strings.TrimSuffix(c.Account.ServerBaseURL, "/") + "/v2/sync/upload"

	req, err := http.NewRequest("POST", url, pr)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("request returned status code %d", resp.StatusCode)
	}
	dec := json.NewDecoder(resp.Body)
	dec.UseNumber()
	var sr stingle.Response
	if err := dec.Decode(&sr); err != nil {
		return err
	}
	log.Debugf("Response: %v", sr)
	if sr.Status != "ok" {
		return sr
	}
	return nil
}

func (c *Client) sendAddAlbum(album *stingle.Album) error {
	if c.Account == nil {
		return ErrNotLoggedIn
	}
	params := make(map[string]string)
	params["albumId"] = album.AlbumID
	params["dateCreated"] = album.DateCreated.String()
	params["dateModified"] = nowString()
	params["encPrivateKey"] = album.EncPrivateKey
	params["metadata"] = album.Metadata
	params["publicKey"] = album.PublicKey
	form := url.Values{}
	form.Set("token", c.Account.Token)
	form.Set("params", c.encodeParams(params))

	sr, err := c.sendRequest("/v2/sync/addAlbum", form, "")
	if err != nil {
		return err
	}
	if sr.Status != "ok" {
		return sr
	}
	return nil
}

func (c *Client) sendDeleteAlbum(albumID string) error {
	if c.Account == nil {
		return ErrNotLoggedIn
	}
	params := make(map[string]string)
	params["albumId"] = albumID
	form := url.Values{}
	form.Set("token", c.Account.Token)
	form.Set("params", c.encodeParams(params))

	sr, err := c.sendRequest("/v2/sync/deleteAlbum", form, "")
	if err != nil {
		return err
	}
	if sr.Status != "ok" {
		return sr
	}
	return nil
}

func (c *Client) sendRenameAlbum(album *stingle.Album) error {
	if c.Account == nil {
		return ErrNotLoggedIn
	}
	params := make(map[string]string)
	params["albumId"] = album.AlbumID
	params["metadata"] = album.Metadata

	form := url.Values{}
	form.Set("token", c.Account.Token)
	form.Set("params", c.encodeParams(params))

	sr, err := c.sendRequest("/v2/sync/renameAlbum", form, "")
	if err != nil {
		return err
	}
	if sr.Status != "ok" {
		return sr
	}
	return nil
}

func (c *Client) sendEditPerms(album *stingle.Album) error {
	if c.Account == nil {
		return ErrNotLoggedIn
	}
	ja, err := json.Marshal(album)
	if err != nil {
		return err
	}
	params := make(map[string]string)
	params["album"] = string(ja)

	form := url.Values{}
	form.Set("token", c.Account.Token)
	form.Set("params", c.encodeParams(params))

	sr, err := c.sendRequest("/v2/sync/editPerms", form, "")
	if err != nil {
		return err
	}
	if sr.Status != "ok" {
		return sr
	}
	return nil
}

func (c *Client) sendMoveFiles(key MoveKey, files []*stingle.File) error {
	if c.Account == nil {
		return ErrNotLoggedIn
	}
	if key.SetFrom == stingle.TrashSet {
		if key.SetTo != stingle.GallerySet || !key.Moving {
			return fmt.Errorf("can only move from trash to gallery: %v", key)
		}
	}
	if key.SetTo == stingle.TrashSet {
		if !key.Moving {
			return fmt.Errorf("can only move to trash: %v", key)
		}
	}
	params := make(map[string]string)
	params["setFrom"] = key.SetFrom
	params["setTo"] = key.SetTo
	params["albumIdFrom"] = key.AlbumIDFrom
	params["albumIdTo"] = key.AlbumIDTo
	params["isMoving"] = "0"
	if key.Moving {
		params["isMoving"] = "1"
	}
	for i, f := range files {
		params[fmt.Sprintf("headers%d", i)] = f.Headers
		params[fmt.Sprintf("filename%d", i)] = f.File
	}
	params["count"] = fmt.Sprintf("%d", len(files))

	form := url.Values{}
	form.Set("token", c.Account.Token)
	form.Set("params", c.encodeParams(params))

	sr, err := c.sendRequest("/v2/sync/moveFile", form, "")
	if err != nil {
		return err
	}
	if sr.Status != "ok" {
		return sr
	}
	return nil
}

func (c *Client) sendDelete(files []string) error {
	if c.Account == nil {
		return ErrNotLoggedIn
	}
	params := make(map[string]string)
	for i, f := range files {
		params[fmt.Sprintf("filename%d", i)] = f
	}
	params["count"] = fmt.Sprintf("%d", len(files))

	form := url.Values{}
	form.Set("token", c.Account.Token)
	form.Set("params", c.encodeParams(params))

	sr, err := c.sendRequest("/v2/sync/delete", form, "")
	if err != nil {
		return err
	}
	if sr.Status != "ok" {
		return sr
	}
	return nil
}
