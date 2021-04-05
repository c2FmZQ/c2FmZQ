package database

import (
	"errors"
	"os"
	"sort"
	"sync"

	"kringle-server/log"
	"kringle-server/stingle"
)

type DeleteEvent struct {
	File    string `json:"file"`
	AlbumID string `json:"albumId"`
	Type    int    `json:"type"`
	Date    int64  `json:"date"`
}

func (d *Database) fileUpdatesForSet(user User, set, albumID string, ts int64, ch chan<- stingle.File, wg *sync.WaitGroup) {
	defer wg.Done()
	fs, err := d.FileSet(user, set, albumID)
	if err != nil {
		log.Errorf("d.FileSet(%q, %q, %q) failed: %v", user.Email, set, albumID, err)
		return
	}

	for _, v := range fs.Files {
		if v.DateModified > ts {
			ch <- stingle.File{
				File:         v.File,
				Version:      v.Version,
				DateCreated:  number(v.DateCreated),
				DateModified: number(v.DateModified),
				Headers:      v.Headers,
				AlbumID:      v.AlbumID,
			}
		}
	}
}

func (d *Database) FileUpdates(user User, set string, ts int64) ([]stingle.File, error) {
	ch := make(chan stingle.File)
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
	go func(ch chan<- stingle.File, wg *sync.WaitGroup) {
		wg.Wait()
		close(ch)
	}(ch, &wg)

	out := []stingle.File{}
	for sf := range ch {
		out = append(out, sf)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].DateModified == out[j].DateModified {
			return out[i].File < out[j].File
		}
		return out[i].DateModified < out[j].DateModified
	})
	return out, nil
}

func (d *Database) deleteUpdatesForSet(user User, set, albumID string, ts int64, ch chan<- stingle.DeleteEvent, wg *sync.WaitGroup) {
	defer wg.Done()
	fs, err := d.FileSet(user, set, albumID)
	if err != nil {
		log.Errorf("d.FileSet(%q, %q, %q failed: %v", user.Email, set, albumID, err)
		return
	}
	for _, d := range fs.Deletes {
		if d.Date > ts {
			ch <- stingle.DeleteEvent{
				File:    d.File,
				AlbumID: d.AlbumID,
				Type:    number(int64(d.Type)),
				Date:    number(d.Date),
			}
		}
	}
}

func (d *Database) DeleteUpdates(user User, ts int64) ([]stingle.DeleteEvent, error) {
	out := []stingle.DeleteEvent{}

	var manifest AlbumManifest
	if _, err := d.storage.ReadDataFile(d.filePath(user.home(albumManifest)), &manifest); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	for _, d := range manifest.Deletes {
		if d.Date > ts {
			out = append(out, stingle.DeleteEvent{
				File:    d.File,
				AlbumID: d.AlbumID,
				Type:    number(int64(d.Type)),
				Date:    number(d.Date),
			})
		}
	}

	ch := make(chan stingle.DeleteEvent)
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
	go func(ch chan<- stingle.DeleteEvent, wg *sync.WaitGroup) {
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
	if _, err := d.storage.ReadDataFile(d.filePath(user.home(albumManifest)), &manifest); err != nil && !errors.Is(err, os.ErrNotExist) {
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
