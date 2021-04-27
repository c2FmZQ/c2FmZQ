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

	"kringle/log"
	"kringle/stingle"
)

type albumDiffs struct {
	AlbumsToAdd        []*stingle.Album
	AlbumsToRemove     []*stingle.Album
	AlbumsToRename     []*stingle.Album
	AlbumPermsToChange []*stingle.Album

	FilesToAdd    []FileLoc
	FilesToMove   map[MoveKey][]*stingle.File
	FilesToDelete []FileLoc
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
	return c.applyDiffs(d, dryrun)
}

func (c *Client) applyDiffs(d *albumDiffs, dryrun bool) error {
	var al AlbumList
	if _, err := c.storage.ReadDataFile(c.fileHash(albumList), &al); err != nil {
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
		if err := c.applyAlbumsToRemove(d.AlbumPermsToChange, dryrun); err != nil {
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

func (c *Client) applyFilesToMove(moves map[MoveKey][]*stingle.File, al AlbumList, dryrun bool) error {
	c.Print("Files to move:")
	for k, v := range moves {
		src, err := c.translateSetAlbumIDToName(k.SetFrom, k.AlbumIDFrom, al)
		if err != nil {
			src = fmt.Sprintf("Set:%s Album:%s", k.SetFrom, k.AlbumIDFrom)
		}
		dst, err := c.translateSetAlbumIDToName(k.SetTo, k.AlbumIDTo, al)
		if err != nil {
			dst = fmt.Sprintf("Set:%s Album:%s", k.SetTo, k.AlbumIDTo)
		}
		op := "Moving"
		if !k.Moving {
			op = "Copying"
		}
		var files []string
		for _, f := range v {
			sk := c.SecretKey
			if k.AlbumIDTo != "" {
				sk, err = al.Albums[k.AlbumIDTo].SK(sk)
				if err != nil {
					c.Printf("SK: %v\n", err)
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
	for k, files := range moves {
		if err := c.sendMoveFiles(k, files); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) applyFilesToDelete(toDelete []FileLoc, al AlbumList, dryrun bool) error {
	c.showFilesToSync("Files to delete:", toDelete, al)
	if dryrun {
		return nil
	}
	var files []*stingle.File
	for _, f := range toDelete {
		files = append(files, f.File)
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
		if err := c.sendDeleteAlbum(album.AlbumID); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) showAlbumsToSync(label string, albums []*stingle.Album) {
	c.Print(label)
	for _, a := range albums {
		name, err := a.Name(c.SecretKey)
		if err != nil {
			c.Printf("Name: %v\n", err)
			name = a.AlbumID
		}
		c.Printf("* %s\n", name)
	}
}

func (c *Client) showFilesToSync(label string, files []FileLoc, al AlbumList) {
	c.Print(label)
	for _, f := range files {
		sk := c.SecretKey
		if album, ok := al.Albums[f.AlbumID]; ok {
			ask, err := album.SK(sk)
			if err != nil {
				c.Printf("sk: %v\n", err)
			} else {
				sk = ask
			}
		} else if album, ok := al.RemoteAlbums[f.AlbumID]; ok {
			ask, err := album.SK(sk)
			if err != nil {
				c.Printf("sk: %v\n", err)
			} else {
				sk = ask
			}
		}
		n, err := f.File.Name(sk)
		if err != nil {
			c.Printf("Name: %v\n", err)
			n = f.File.File
		}
		d, err := c.translateSetAlbumIDToName(f.Set, f.AlbumID, al)
		if err != nil {
			c.Printf("translate: %v\n", err)
		}
		c.Printf("* %s / %s\n", d, n)
	}
}

func (c *Client) translateSetAlbumIDToName(set, albumID string, al AlbumList) (string, error) {
	switch set {
	case stingle.GallerySet:
		return "gallery", nil
	case stingle.TrashSet:
		return "trash", nil
	case stingle.AlbumSet:
		if album, ok := al.Albums[albumID]; ok {
			return album.Name(c.SecretKey)
		}
		if album, ok := al.RemoteAlbums[albumID]; ok {
			return album.Name(c.SecretKey)
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
	if _, err := c.storage.ReadDataFile(c.fileHash(albumList), &al); err != nil {
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
		if _, err := c.storage.ReadDataFile(c.fileHash(d.fileSet), &fs); err != nil {
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
				diffs.FilesToAdd = append(diffs.FilesToAdd, FileLoc{add.file, add.set, add.albumID})
				continue
			}
			from := loc[0]
			moving := false
			if len(changes.remove) > 0 {
				from = changes.remove[0]
				changes.remove = changes.remove[1:]
				moving = true
			}
			mk := MoveKey{
				SetFrom:     from.set,
				AlbumIDFrom: from.albumID,
				SetTo:       add.set,
				AlbumIDTo:   add.albumID,
				Moving:      moving,
			}
			if diffs.FilesToMove == nil {
				diffs.FilesToMove = make(map[MoveKey][]*stingle.File)
			}
			diffs.FilesToMove[mk] = append(diffs.FilesToMove[mk], add.file)
		}
		for _, remove := range changes.remove {
			mk := MoveKey{
				SetFrom:     remove.set,
				AlbumIDFrom: remove.albumID,
				SetTo:       stingle.TrashSet,
				AlbumIDTo:   "",
				Moving:      true,
			}
			if diffs.FilesToMove == nil {
				diffs.FilesToMove = make(map[MoveKey][]*stingle.File)
			}
			diffs.FilesToMove[mk] = append(diffs.FilesToMove[mk], remove.file)

			diffs.FilesToDelete = append(diffs.FilesToDelete, FileLoc{remove.file, stingle.TrashSet, ""})
		}
	}
	return &diffs, nil
}

// Pull downloads all the files matching pattern that are not already present
// in the local storage. Returns the number of files downloaded.
func (c *Client) Pull(patterns []string) (int, error) {
	list, err := c.GlobFiles(patterns)
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
	list, err := c.GlobFiles(patterns)
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
		fmt.Fprintf(c.writer, "Freeing %s\n", item.Filename)
		if err := os.Remove(fn); err != nil {
			return count, err
		}
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
	n := c.storage.HashString(name)
	return filepath.Join(c.storage.Dir(), blobsDir, n[:2], n)
}

func (c *Client) downloadWorker(ch <-chan ListItem, out chan<- error) {
	for i := range ch {
		fmt.Fprintf(c.writer, "Downloading %s\n", i.Filename)
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
			{"token", c.Token},
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

	url := c.ServerBaseURL + "/v2/sync/upload"

	resp, err := c.hc.Post(url, w.FormDataContentType(), pr)
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
	params := make(map[string]string)
	params["albumId"] = album.AlbumID
	params["dateCreated"] = album.DateCreated.String()
	params["dateModified"] = nowString()
	params["encPrivateKey"] = album.EncPrivateKey
	params["metadata"] = album.Metadata
	params["publicKey"] = album.PublicKey
	form := url.Values{}
	form.Set("token", c.Token)
	form.Set("params", c.encodeParams(params))

	sr, err := c.sendRequest("/v2/sync/addAlbum", form)
	if err != nil {
		return err
	}
	if sr.Status != "ok" {
		return sr
	}
	return nil
}

func (c *Client) sendDeleteAlbum(albumID string) error {
	params := make(map[string]string)
	params["albumId"] = albumID
	form := url.Values{}
	form.Set("token", c.Token)
	form.Set("params", c.encodeParams(params))

	sr, err := c.sendRequest("/v2/sync/deleteAlbum", form)
	if err != nil {
		return err
	}
	if sr.Status != "ok" {
		return sr
	}
	return nil
}

func (c *Client) sendRenameAlbum(album *stingle.Album) error {
	params := make(map[string]string)
	params["albumId"] = album.AlbumID
	params["metadata"] = album.Metadata

	form := url.Values{}
	form.Set("token", c.Token)
	form.Set("params", c.encodeParams(params))

	sr, err := c.sendRequest("/v2/sync/renameAlbum", form)
	if err != nil {
		return err
	}
	if sr.Status != "ok" {
		return sr
	}
	return nil
}

func (c *Client) sendEditPerms(album *stingle.Album) error {
	ja, err := json.Marshal(album)
	if err != nil {
		return err
	}
	params := make(map[string]string)
	params["album"] = string(ja)

	form := url.Values{}
	form.Set("token", c.Token)
	form.Set("params", c.encodeParams(params))

	sr, err := c.sendRequest("/v2/sync/editPerms", form)
	if err != nil {
		return err
	}
	if sr.Status != "ok" {
		return sr
	}
	return nil
}

func (c *Client) sendMoveFiles(key MoveKey, files []*stingle.File) error {
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
	form.Set("token", c.Token)
	form.Set("params", c.encodeParams(params))

	sr, err := c.sendRequest("/v2/sync/moveFile", form)
	if err != nil {
		return err
	}
	if sr.Status != "ok" {
		return sr
	}
	return nil
}

func (c *Client) sendDelete(files []*stingle.File) error {
	params := make(map[string]string)
	for i, f := range files {
		params[fmt.Sprintf("filename%d", i)] = f.File
	}
	params["count"] = fmt.Sprintf("%d", len(files))

	form := url.Values{}
	form.Set("token", c.Token)
	form.Set("params", c.encodeParams(params))

	sr, err := c.sendRequest("/v2/sync/delete", form)
	if err != nil {
		return err
	}
	if sr.Status != "ok" {
		return sr
	}
	return nil
}
