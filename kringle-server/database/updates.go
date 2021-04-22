package database

import (
	"sort"
	"sync"

	"kringle-server/log"
	"kringle-server/stingle"
)

// DeleteEvent encapsulates a deletion event. The File and AlbumID fields are
// different meanings depending on the value of Type.
type DeleteEvent struct {
	File    string `json:"file"`
	AlbumID string `json:"albumId"`
	Type    int    `json:"type"` // See stingle/types.go
	Date    int64  `json:"date"` // The time of the deletion.
}

// fileUpdatesForSet finds which files were added to the file set since ts.
func (d *Database) fileUpdatesForSet(user User, set, albumID string, ts int64, ch chan<- stingle.File, wg *sync.WaitGroup) {
	defer wg.Done()
	fs, err := d.FileSet(user, set, albumID)
	if err != nil {
		log.Errorf("d.FileSet(%q, %q, %q) failed: %v", user.Email, set, albumID, err)
		return
	}

	for k, v := range fs.Files {
		if v.DateModified > ts {
			ch <- stingle.File{
				File:         k,
				Version:      v.Version,
				DateCreated:  number(v.DateCreated),
				DateModified: number(v.DateModified),
				Headers:      v.Headers,
				AlbumID:      albumID,
			}
		}
	}
}

// FileUpdates returns all the files that were added to a file set since time
// ts.
func (d *Database) FileUpdates(user User, set string, ts int64) ([]stingle.File, error) {
	defer recordLatency("FileUpdates")()

	ch := make(chan stingle.File)
	var wg sync.WaitGroup

	if set != stingle.AlbumSet {
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
			go d.fileUpdatesForSet(user, stingle.AlbumSet, album.AlbumID, ts, ch, &wg)
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

// deleteUpdatesForSet finds which files were deleted from the file set since
// ts.
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

// DeleteUpdates returns all the files that were deleted from a file set since
// time ts.
func (d *Database) DeleteUpdates(user User, ts int64) ([]stingle.DeleteEvent, error) {
	defer recordLatency("DeleteUpdates")()

	out := []stingle.DeleteEvent{}

	var manifest AlbumManifest
	if _, err := d.storage.ReadDataFile(d.filePath(user.home(albumManifest)), &manifest); err != nil {
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
	for _, set := range []string{stingle.GallerySet, stingle.TrashSet, stingle.AlbumSet} {
		if set == stingle.AlbumSet {
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
	for k, f := range fs.Files {
		ch <- fileSize{k, f.StoreFileSize + f.StoreThumbSize}
	}
}

// SpaceUsed calculates the sum of all the file sizes in a user's file sets,
// counting each file only once, even if it is in multiple sets.
func (d *Database) SpaceUsed(user User) (int64, error) {
	defer recordLatency("SpaceUsed")()

	var manifest AlbumManifest
	if _, err := d.storage.ReadDataFile(d.filePath(user.home(albumManifest)), &manifest); err != nil {
		return 0, err
	}

	ch := make(chan fileSize)
	var wg sync.WaitGroup
	for _, set := range []string{stingle.GallerySet, stingle.TrashSet, stingle.AlbumSet} {
		if set == stingle.AlbumSet {
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
