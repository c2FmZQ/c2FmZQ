package client

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"

	"kringle-server/log"
	"kringle-server/stingle"
)

// AlbumList represents a list of albums.
type AlbumList struct {
	UpdateTimestamps
	Albums map[string]stingle.Album `json:"albums"`
}

// FileSet represents a file set.
type FileSet struct {
	UpdateTimestamps
	Files map[string]stingle.File `json:"files"`
}

// ContactList represents a list of contacts.
type ContactList struct {
	UpdateTimestamps
	Contacts map[string]stingle.Contact `json:"contacts"`
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
		filenames = append(filenames, c.storage.HashString(names[i]))
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
			fs.Files = make(map[string]stingle.File)
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
	commit, err := c.storage.OpenForUpdate(c.storage.HashString(albumList), &al)
	if err != nil {
		return err
	}
	defer commit(true, &retErr)
	if al.Albums == nil {
		al.Albums = make(map[string]stingle.Album)
	}
	for _, up := range updates {
		if _, exists := al.Albums[up.AlbumID]; !exists {
			c.storage.CreateEmptyFile(c.storage.HashString(albumPrefix+up.AlbumID), &FileSet{})
		}
		al.Albums[up.AlbumID] = up
		d, _ := up.DateModified.Int64()
		if d > al.LastUpdateTime {
			al.LastUpdateTime = d
		}
	}
	log.Debugf("AlbumList: [%d] %#v", len(al.Albums), al)
	return nil
}

func (c *Client) processContactUpdates(updates []stingle.Contact) (retErr error) {
	if len(updates) == 0 {
		return nil
	}
	var cl ContactList
	commit, err := c.storage.OpenForUpdate(c.storage.HashString(contactsFile), &cl)
	if err != nil {
		return err
	}
	defer commit(true, &retErr)
	if cl.Contacts == nil {
		cl.Contacts = make(map[string]stingle.Contact)
	}
	for _, up := range updates {
		cl.Contacts[up.Email] = up
		d, _ := up.DateModified.Int64()
		if d > cl.LastUpdateTime {
			cl.LastUpdateTime = d
		}
	}
	log.Debugf("Contacts: [%d] %#v", len(cl.Contacts), cl)
	return nil
}

func (c *Client) processFileUpdates(name string, updates []stingle.File) (retErr error) {
	if len(updates) == 0 {
		return nil
	}
	commit, fs, err := c.fileSetForUpdate(name)
	if err != nil {
		return err
	}
	defer commit(true, &retErr)
	for _, up := range updates {
		fs.Files[up.File] = up
		d, _ := up.DateModified.Int64()
		if d > fs.LastUpdateTime {
			fs.LastUpdateTime = d
		}
	}
	log.Debugf("FileSet(%q): [%d] %#v", name, len(fs.Files), fs)
	return nil
}

func (c *Client) processAlbumFileUpdates(updates []stingle.File) (retErr error) {
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
		if err := c.processFileUpdates(albumPrefix+a, u); err != nil {
			return err
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
		if d > fs.LastDeleteTime {
			fs.LastDeleteTime = d
		}
	}
	log.Debugf("FileSet(%q): [%d] %#v", name, len(fs.Files), fs)
	return nil
}

func (c *Client) processDeleteAlbums(deletes []stingle.DeleteEvent) (retErr error) {
	var al AlbumList
	commit, err := c.storage.OpenForUpdate(c.storage.HashString(albumList), &al)
	if err != nil {
		return err
	}
	defer commit(true, &retErr)
	for _, del := range deletes {
		d, _ := del.Date.Int64()
		if a, ok := al.Albums[del.AlbumID]; ok {
			ad, _ := a.DateModified.Int64()
			if d > ad {
				delete(al.Albums, del.AlbumID)
				if err := os.Remove(filepath.Join(c.storage.Dir(), c.storage.HashString(albumPrefix+del.AlbumID))); err != nil {
					return err
				}
			}
		}
		if d > al.LastDeleteTime {
			al.LastDeleteTime = d
		}
	}
	log.Debugf("Albums: [%d] %#v", len(al.Albums), al)
	return nil
}

func (c *Client) processDeleteContacts(deletes []stingle.DeleteEvent) (retErr error) {
	var cl ContactList
	commit, err := c.storage.OpenForUpdate(c.storage.HashString(contactsFile), &cl)
	if err != nil {
		return err
	}
	defer commit(true, &retErr)
	for _, del := range deletes {
		d, _ := del.Date.Int64()
		for email, contact := range cl.Contacts {
			cd, _ := contact.DateModified.Int64()
			if contact.UserID.String() == del.File && d > cd {
				delete(cl.Contacts, email)
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
	_, err = c.storage.ReadDataFile(c.storage.HashString(name), &foo)
	ts = foo.UpdateTimestamps
	return
}

func (c *Client) getAlbumTimestamps() (ts UpdateTimestamps, err error) {
	var al AlbumList
	_, err = c.storage.ReadDataFile(c.storage.HashString(albumList), &al)
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

func (c *Client) GetUpdates() error {
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
	if err := c.processFileUpdates(galleryFile, gallery); err != nil {
		return err
	}

	var trash []stingle.File
	if err := copyJSON(sr.Parts["trash"], &trash); err != nil {
		return err
	}
	if err := c.processFileUpdates(trashFile, trash); err != nil {
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

	fmt.Println("Metadata synced successfully.")
	return nil
}
