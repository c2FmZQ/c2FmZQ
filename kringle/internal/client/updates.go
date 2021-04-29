package client

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"

	"kringle/internal/log"
	"kringle/internal/stingle"
)

// AlbumList represents a list of albums.
type AlbumList struct {
	UpdateTimestamps
	Albums       map[string]*stingle.Album `json:"albums"`
	RemoteAlbums map[string]*stingle.Album `json:"remoteAlbums"`
}

// FileSet represents a file set.
type FileSet struct {
	UpdateTimestamps
	Files       map[string]*stingle.File `json:"files"`
	RemoteFiles map[string]*stingle.File `json:"remoteFiles"`
}

// ContactList represents a list of contacts.
type ContactList struct {
	UpdateTimestamps
	Contacts map[int64]*stingle.Contact `json:"contacts"`
}

// UpdateTimestamps represents update/delete timestamps.
type UpdateTimestamps struct {
	LastUpdateTime int64 `json:"lastUpdateTime"`
	LastDeleteTime int64 `json:"lastDeleteTime"`
}

// fileSetForUpdate retrieves a file sets for update.
func (c *Client) fileSetForUpdate(name string) (func(bool, *error) error, *FileSet, error) {
	commit, fs, err := c.fileSetsForUpdate([]string{name})
	if err != nil {
		log.Errorf("fileSetForUpdate(%q): %v", name, err)
		return nil, nil, err
	}
	return commit, fs[0], nil
}

// fileSetsForUpdate retrieves any number of file sets for update.
func (c *Client) fileSetsForUpdate(names []string) (func(bool, *error) error, []*FileSet, error) {
	var filenames []string
	for i := range names {
		filenames = append(filenames, c.fileHash(names[i]))
	}

	fileSets := make([]*FileSet, len(filenames))
	for i := range fileSets {
		fileSets[i] = &FileSet{}
	}
	commit, err := c.storage.OpenManyForUpdate(filenames, fileSets)
	if err != nil {
		return nil, nil, err
	}
	for _, fs := range fileSets {
		if fs.Files == nil {
			fs.Files = make(map[string]*stingle.File)
		}
		if fs.RemoteFiles == nil {
			fs.RemoteFiles = make(map[string]*stingle.File)
		}
	}
	return commit, fileSets, nil
}

func copyJSON(src interface{}, dst interface{}) error {
	b, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dst)
}

func (c *Client) processAlbumUpdates(updates []stingle.Album) (retErr error) {
	if len(updates) == 0 {
		return nil
	}
	var al AlbumList
	commit, err := c.storage.OpenForUpdate(c.fileHash(albumList), &al)
	if err != nil {
		return err
	}
	defer commit(true, &retErr)
	if al.Albums == nil {
		al.Albums = make(map[string]*stingle.Album)
	}
	if al.RemoteAlbums == nil {
		al.RemoteAlbums = make(map[string]*stingle.Album)
	}
	for _, up := range updates {
		if up.AlbumID == "" {
			continue
		}
		// Update remote album.
		if _, ok := al.RemoteAlbums[up.AlbumID]; !ok {
			c.storage.CreateEmptyFile(c.fileHash(albumPrefix+up.AlbumID), &FileSet{})
		}
		na := up
		al.RemoteAlbums[up.AlbumID] = &na

		// Update local album.
		la, ok := al.Albums[up.AlbumID]
		if !ok {
			c.storage.CreateEmptyFile(c.fileHash(albumPrefix+up.AlbumID), &FileSet{})
			al.Albums[up.AlbumID] = &na
		} else {
			nad, _ := na.DateModified.Int64()
			lad, _ := la.DateModified.Int64()
			if nad > lad {
				al.Albums[up.AlbumID] = &na
			}
		}

		d, _ := up.DateModified.Int64()
		if d > al.LastUpdateTime {
			al.LastUpdateTime = d
		}
	}
	return nil
}

func (c *Client) processContactUpdates(updates []stingle.Contact) (retErr error) {
	if len(updates) == 0 {
		return nil
	}
	var cl ContactList
	commit, err := c.storage.OpenForUpdate(c.fileHash(contactsFile), &cl)
	if err != nil {
		return err
	}
	defer commit(true, &retErr)
	if cl.Contacts == nil {
		cl.Contacts = make(map[int64]*stingle.Contact)
	}
	for _, up := range updates {
		id, _ := up.UserID.Int64()
		nc := up
		cl.Contacts[id] = &nc
		d, _ := up.DateModified.Int64()
		if d > cl.LastUpdateTime {
			cl.LastUpdateTime = d
		}
	}
	return nil
}

func (c *Client) processFileUpdates(name string, updates []stingle.File) (n int, retErr error) {
	if len(updates) == 0 {
		return 0, nil
	}
	commit, fs, err := c.fileSetForUpdate(name)
	if err != nil {
		return 0, err
	}
	defer commit(true, &retErr)
	for _, up := range updates {
		if _, ok := fs.RemoteFiles[up.File]; !ok {
			n++
		}
		nf := up
		fs.RemoteFiles[up.File] = &nf
		fs.Files[up.File] = &nf
		d, _ := up.DateModified.Int64()
		if d > fs.LastUpdateTime {
			fs.LastUpdateTime = d
		}
	}
	return n, nil
}

func (c *Client) processAlbumFileUpdates(updates []stingle.File) (retErr error) {
	var al AlbumList
	commit, err := c.storage.OpenForUpdate(c.fileHash(albumList), &al)
	if err != nil {
		return err
	}
	defer commit(true, &retErr)

	albums := make(map[string]struct{})
	for _, f := range updates {
		albums[f.AlbumID] = struct{}{}
	}
	for a := range albums {
		var u []stingle.File
		for _, f := range updates {
			if f.AlbumID == a {
				u = append(u, f)
			}
		}

		n, err := c.processFileUpdates(albumPrefix+a, u)
		if err != nil {
			return err
		}
		if n == 0 {
			continue
		}
		// If the album was deleted locally, bring it back since there
		// are new files.
		if _, ok := al.Albums[a]; !ok {
			al.Albums[a] = al.RemoteAlbums[a]
			if album := al.Albums[a]; album != nil {
				name, _ := album.Name(c.SecretKey)
				log.Debugf("Album recovered %s (%s)", name, a)
			}
		}
	}
	return nil
}

func (c *Client) processDeleteFiles(name string, deletes []stingle.DeleteEvent) (retErr error) {
	commit, fs, err := c.fileSetForUpdate(name)
	if err != nil {
		return err
	}
	defer commit(true, &retErr)
	for _, del := range deletes {
		d, _ := del.Date.Int64()
		if f, ok := fs.Files[del.File]; ok {
			fd, _ := f.DateModified.Int64()
			if d > fd {
				delete(fs.Files, del.File)
			}
		}
		if f, ok := fs.RemoteFiles[del.File]; ok {
			fd, _ := f.DateModified.Int64()
			if d > fd {
				delete(fs.RemoteFiles, del.File)
			}
		}
		if d > fs.LastDeleteTime {
			fs.LastDeleteTime = d
		}
	}
	return nil
}

func (c *Client) albumHasLocalFileChanges(albumID string) (bool, error) {
	var fs FileSet
	if _, err := c.storage.ReadDataFile(c.fileHash(albumPrefix+albumID), &fs); err != nil {
		return false, err
	}
	for f := range fs.Files {
		if _, ok := fs.RemoteFiles[f]; !ok {
			return true, nil
		}
	}
	return false, nil
}

func (c *Client) processDeleteAlbums(deletes []stingle.DeleteEvent) (retErr error) {
	var al AlbumList
	commit, err := c.storage.OpenForUpdate(c.fileHash(albumList), &al)
	if err != nil {
		return err
	}
	defer commit(true, &retErr)
	for _, del := range deletes {
		d, _ := del.Date.Int64()
		if a, ok := al.Albums[del.AlbumID]; ok {
			ad, _ := a.DateModified.Int64()
			name, _ := a.Name(c.SecretKey)
			localChanges, err := c.albumHasLocalFileChanges(del.AlbumID)
			if err != nil {
				return err
			}
			if d > ad && (a.IsOwner != "1" || (a.Equals(al.RemoteAlbums[del.AlbumID]) && !localChanges)) {
				log.Debugf("Album deleted: %s (%s)", name, a.AlbumID)
				delete(al.Albums, del.AlbumID)
			} else {
				log.Debugf("Album NOT deleted: %s (%s)", name, a.AlbumID)
			}
		}
		if a, ok := al.RemoteAlbums[del.AlbumID]; ok {
			ad, _ := a.DateModified.Int64()
			if d > ad {
				delete(al.RemoteAlbums, del.AlbumID)
			}
		}
		if al.Albums[del.AlbumID] == nil && al.RemoteAlbums[del.AlbumID] == nil {
			if err := os.Remove(filepath.Join(c.storage.Dir(), c.fileHash(albumPrefix+del.AlbumID))); err != nil {
				return err
			}
		}
		if d > al.LastDeleteTime {
			al.LastDeleteTime = d
		}
	}
	return nil
}

func (c *Client) processDeleteContacts(deletes []stingle.DeleteEvent) (retErr error) {
	var cl ContactList
	commit, err := c.storage.OpenForUpdate(c.fileHash(contactsFile), &cl)
	if err != nil {
		return err
	}
	defer commit(true, &retErr)
	for _, del := range deletes {
		d, _ := del.Date.Int64()
		id, _ := strconv.ParseInt(del.File, 10, 64)
		if contact, ok := cl.Contacts[id]; ok {
			cd, _ := contact.DateModified.Int64()
			if d > cd {
				delete(cl.Contacts, id)
			}
		}
		if d > cl.LastDeleteTime {
			cl.LastDeleteTime = d
		}
	}
	log.Debugf("Contacts: [%d] %#v", len(cl.Contacts), cl)
	return nil
}

func (c *Client) processDeleteUpdates(updates []stingle.DeleteEvent) (retErr error) {
	if len(updates) == 0 {
		return nil
	}
	types := make(map[int64]struct{})
	albums := make(map[string]struct{})
	for _, up := range updates {
		t, _ := up.Type.Int64()
		types[t] = struct{}{}
		if t == stingle.DeleteEventAlbumFile {
			albums[up.AlbumID] = struct{}{}
		}
	}
	for t := range types {
		var de []stingle.DeleteEvent
		for _, up := range updates {
			tt, _ := up.Type.Int64()
			if tt == t {
				de = append(de, up)
			}
		}
		var err error
		switch t {
		case stingle.DeleteEventGallery:
			err = c.processDeleteFiles(galleryFile, de)
		case stingle.DeleteEventTrash:
			err = c.processDeleteFiles(trashFile, de)
		case stingle.DeleteEventTrashDelete:
			// TODO: handle actual file deletions.
			err = c.processDeleteFiles(trashFile, de)
		case stingle.DeleteEventAlbum:
			err = c.processDeleteAlbums(de)
		case stingle.DeleteEventAlbumFile:
			for album := range albums {
				var ade []stingle.DeleteEvent
				for _, d := range de {
					if d.AlbumID == album {
						ade = append(ade, d)
					}
				}
				if e := c.processDeleteFiles(albumPrefix+album, ade); err == nil {
					err = e
				}
			}
		case stingle.DeleteEventContact:
			err = c.processDeleteContacts(de)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) getTimestamps(name string) (ts UpdateTimestamps, err error) {
	foo := struct{ UpdateTimestamps }{}
	_, err = c.storage.ReadDataFile(c.fileHash(name), &foo)
	ts = foo.UpdateTimestamps
	return
}

func (c *Client) getAlbumTimestamps() (ts UpdateTimestamps, err error) {
	var al AlbumList
	_, err = c.storage.ReadDataFile(c.fileHash(albumList), &al)
	for album := range al.Albums {
		t, err := c.getTimestamps(albumPrefix + album)
		if err != nil {
			return ts, err
		}
		if t.LastUpdateTime > ts.LastUpdateTime {
			ts.LastUpdateTime = t.LastUpdateTime
		}
		if t.LastDeleteTime > ts.LastDeleteTime {
			ts.LastDeleteTime = t.LastDeleteTime
		}
	}
	return ts, nil
}

func max(values ...int64) (m int64) {
	for _, v := range values {
		if v > m {
			m = v
		}
	}
	return
}

func (c *Client) GetUpdates(quiet bool) error {
	galleryTS, err := c.getTimestamps(galleryFile)
	if err != nil {
		return err
	}
	trashTS, err := c.getTimestamps(trashFile)
	if err != nil {
		return err
	}
	albumsTS, err := c.getTimestamps(albumList)
	if err != nil {
		return err
	}
	contactsTS, err := c.getTimestamps(contactsFile)
	if err != nil {
		return err
	}
	albumFilesTS, err := c.getAlbumTimestamps()
	if err != nil {
		return err
	}
	deleteTS := max(galleryTS.LastDeleteTime, trashTS.LastDeleteTime, albumsTS.LastDeleteTime, contactsTS.LastDeleteTime, albumFilesTS.LastDeleteTime)

	form := url.Values{}
	form.Set("token", c.Token)
	form.Set("filesST", strconv.FormatInt(galleryTS.LastUpdateTime, 10))
	form.Set("trashST", strconv.FormatInt(trashTS.LastUpdateTime, 10))
	form.Set("albumsST", strconv.FormatInt(albumsTS.LastUpdateTime, 10))
	form.Set("albumFilesST", strconv.FormatInt(albumFilesTS.LastUpdateTime, 10))
	form.Set("cntST", strconv.FormatInt(contactsTS.LastUpdateTime, 10))
	form.Set("delST", strconv.FormatInt(deleteTS, 10))
	sr, err := c.sendRequest("/v2/sync/getUpdates", form)
	if err != nil {
		return err
	}
	if sr.Status != "ok" {
		return sr
	}

	var albums []stingle.Album
	if err := copyJSON(sr.Parts["albums"], &albums); err != nil {
		return err
	}
	if err := c.processAlbumUpdates(albums); err != nil {
		return err
	}

	var gallery []stingle.File
	if err := copyJSON(sr.Parts["files"], &gallery); err != nil {
		return err
	}
	if _, err := c.processFileUpdates(galleryFile, gallery); err != nil {
		return err
	}

	var trash []stingle.File
	if err := copyJSON(sr.Parts["trash"], &trash); err != nil {
		return err
	}
	if _, err := c.processFileUpdates(trashFile, trash); err != nil {
		return err
	}

	var albumFiles []stingle.File
	if err := copyJSON(sr.Parts["albumFiles"], &albumFiles); err != nil {
		return err
	}
	if err := c.processAlbumFileUpdates(albumFiles); err != nil {
		return err
	}

	var contacts []stingle.Contact
	if err := copyJSON(sr.Parts["contacts"], &contacts); err != nil {
		return err
	}
	if err := c.processContactUpdates(contacts); err != nil {
		return err
	}

	var deletes []stingle.DeleteEvent
	if err := copyJSON(sr.Parts["deletes"], &deletes); err != nil {
		return err
	}
	if err := c.processDeleteUpdates(deletes); err != nil {
		return err
	}

	if !quiet {
		fmt.Fprintln(c.writer, "Metadata synced successfully.")
	}
	return nil
}
