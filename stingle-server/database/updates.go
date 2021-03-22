package database

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"stingle-server/log"
)

type DeleteEvent struct {
	File    string `json:"file"`
	AlbumID string `json:"albumId"`
	// 1: Gallery 2: Trash 3: Trash&delete 4: Album 5: Album file 6: Contact
	Type int   `json:"type"`
	Date int64 `json:"date"`
}

type StingleDelete struct {
	File    string `json:"file"`
	AlbumID string `json:"albumId"`
	// 1: Gallery 2: Trash 3: Trash&delete 4: Album 5: Album file 6: Contact
	Type string `json:"type"`
	Date string `json:"date"`
}

func (d *Database) fileUpdatesForSet(user User, set, albumID string, ts int64, ch chan<- StingleFile, wg *sync.WaitGroup) {
	defer wg.Done()
	fs, err := d.FileSet(user, set, albumID)
	if err != nil {
		log.Errorf("d.FileSet(%q, %q, %q) failed: %v", user.Email, set, albumID, err)
		return
	}

	for _, v := range fs.Files {
		if v.DateModified > ts {
			ch <- StingleFile{
				File:         v.File,
				Version:      v.Version,
				DateCreated:  fmt.Sprintf("%d", v.DateCreated),
				DateModified: fmt.Sprintf("%d", v.DateModified),
				Headers:      v.Headers,
				AlbumID:      v.AlbumID,
			}
		}
	}
}

func (d *Database) FileUpdates(user User, set string, ts int64) ([]StingleFile, error) {
	ch := make(chan StingleFile)
	var wg sync.WaitGroup

	if set != AlbumSet {
		wg.Add(1)
		go d.fileUpdatesForSet(user, set, "", ts, ch, &wg)
	} else {
		albumRefs, err := d.AlbumRefs(user)
		if err != nil {
			log.Errorf("AlbumRefs(%q) failed: %v", user.Email, err)
			return nil, err
		}

		for _, album := range albumRefs {
			wg.Add(1)
			go d.fileUpdatesForSet(user, AlbumSet, album.AlbumID, ts, ch, &wg)
		}
	}
	go func(ch chan<- StingleFile, wg *sync.WaitGroup) {
		wg.Wait()
		close(ch)
	}(ch, &wg)

	out := []StingleFile{}
	for sf := range ch {
		out = append(out, sf)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DateModified < out[j].DateModified })
	return out, nil
}

func (d *Database) deleteUpdatesForSet(user User, set, albumID string, ts int64, ch chan<- StingleDelete, wg *sync.WaitGroup) {
	defer wg.Done()
	fs, err := d.FileSet(user, set, albumID)
	if err != nil {
		log.Errorf("d.FileSet(%q, %q, %q failed: %v", user.Email, set, albumID, err)
		return
	}
	for _, d := range fs.Deletes {
		if d.Date > ts {
			ch <- StingleDelete{
				File:    d.File,
				AlbumID: d.AlbumID,
				Type:    fmt.Sprintf("%d", d.Type),
				Date:    fmt.Sprintf("%d", d.Date),
			}
		}
	}
}

func (d *Database) DeleteUpdates(user User, ts int64) ([]StingleDelete, error) {
	out := []StingleDelete{}

	var manifest AlbumManifest
	if err := loadJSON(filepath.Join(d.Home(user.Email), albumManifest), &manifest); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	for _, d := range manifest.Deletes {
		if d.Date > ts {
			out = append(out, StingleDelete{
				File:    d.File,
				AlbumID: d.AlbumID,
				Type:    fmt.Sprintf("%d", d.Type),
				Date:    fmt.Sprintf("%d", d.Date),
			})
		}
	}

	ch := make(chan StingleDelete)
	var wg sync.WaitGroup
	for _, set := range []string{GallerySet, TrashSet, AlbumSet} {
		if set == AlbumSet {
			for _, a := range manifest.Albums {
				wg.Add(1)
				go d.deleteUpdatesForSet(user, set, a.AlbumID, ts, ch, &wg)
			}
		} else {
			wg.Add(1)
			go d.deleteUpdatesForSet(user, set, "", ts, ch, &wg)
		}
	}
	go func(ch chan<- StingleDelete, wg *sync.WaitGroup) {
		wg.Wait()
		close(ch)
	}(ch, &wg)

	for de := range ch {
		out = append(out, de)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date < out[j].Date })
	return out, nil
}

type fileSize struct {
	name string
	size int64
}

func (d *Database) getFileSizes(user User, set, albumID string, ch chan<- fileSize, wg *sync.WaitGroup) {
	defer wg.Done()
	fs, err := d.FileSet(user, set, albumID)
	if err != nil {
		log.Errorf("d.FileSet(%q, %q, %q failed: %v", user.Email, set, albumID, err)
		return
	}
	if fs.Album != nil && fs.Album.OwnerID != user.UserID {
		// Only charge file size to owner of the album.
		return
	}
	for _, f := range fs.Files {
		ch <- fileSize{f.File, f.StoreFileSize + f.StoreThumbSize}
	}
}

func (d *Database) SpaceUsed(user User) (int64, error) {
	var manifest AlbumManifest
	if err := loadJSON(filepath.Join(d.Home(user.Email), albumManifest), &manifest); err != nil && !errors.Is(err, os.ErrNotExist) {
		return 0, err
	}

	ch := make(chan fileSize)
	var wg sync.WaitGroup
	for _, set := range []string{GallerySet, TrashSet, AlbumSet} {
		if set == AlbumSet {
			for _, a := range manifest.Albums {
				wg.Add(1)
				go d.getFileSizes(user, set, a.AlbumID, ch, &wg)
			}
		} else {
			wg.Add(1)
			go d.getFileSizes(user, set, "", ch, &wg)
		}
	}
	go func(ch chan<- fileSize, wg *sync.WaitGroup) {
		wg.Wait()
		close(ch)
	}(ch, &wg)

	files := make(map[string]int64)
	for fs := range ch {
		files[fs.name] = fs.size
	}
	var total int64
	for _, v := range files {
		total += v
	}
	return total, nil
}
