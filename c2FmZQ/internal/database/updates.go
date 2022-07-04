//
// Copyright 2021-2022 TTBT Enterprises LLC
//
// This file is part of c2FmZQ (https://c2FmZQ.org/).
//
// c2FmZQ is free software: you can redistribute it and/or modify it under the
// terms of the GNU General Public License as published by the Free Software
// Foundation, either version 3 of the License, or (at your option) any later
// version.
//
// c2FmZQ is distributed in the hope that it will be useful, but WITHOUT ANY
// WARRANTY; without even the implied warranty of MERCHANTABILITY or FITNESS FOR
// A PARTICULAR PURPOSE. See the GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License along with
// c2FmZQ. If not, see <https://www.gnu.org/licenses/>.

package database

import (
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"c2FmZQ/internal/log"
	"c2FmZQ/internal/stingle"
)

const (
	deleteEventHorizon = 180 * 24 * time.Hour
)

var (
	// Indicates that some delete events were pruned sinced the client's
	// last update. This client can still upload its data, but they should
	// wipe the app and login again.
	ErrUpdateTimestampTooOld = errors.New("update timestamp is too old")
)

// DeleteEvent encapsulates a deletion event. The File and AlbumID fields are
// different meanings depending on the value of Type.
type DeleteEvent struct {
	File    string `json:"file,omitempty"`
	AlbumID string `json:"albumId,omitempty"`
	Type    int    `json:"type"` // See stingle/types.go
	Date    int64  `json:"date"` // The time of the deletion.
}

func pruneDeleteEvents(events *[]DeleteEvent, horizonTS *int64) {
	ts := nowInMS() - int64(deleteEventHorizon/time.Millisecond)
	off := 0
	for off = 0; off < len(*events) && (*events)[off].Date < ts; off++ {
		continue
	}
	if off > 0 {
		*events = (*events)[off:]
		*horizonTS = ts
	}
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
func (d *Database) deleteUpdatesForSet(user User, set, albumID string, ts int64, ch chan<- stingle.DeleteEvent, eCh chan<- error) {
	fs, err := d.FileSet(user, set, albumID)
	if err != nil {
		log.Errorf("d.FileSet(%q, %q, %q failed: %v", user.Email, set, albumID, err)
		eCh <- err
		return
	}
	if ts > 0 && ts < fs.DeleteHorizon {
		eCh <- ErrUpdateTimestampTooOld
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
	eCh <- nil
}

// DeleteUpdates returns all the files that were deleted from a file set since
// time ts.
func (d *Database) DeleteUpdates(user User, ts int64) ([]stingle.DeleteEvent, error) {
	defer recordLatency("DeleteUpdates")()

	out := []stingle.DeleteEvent{}

	var manifest AlbumManifest
	if err := d.storage.ReadDataFile(d.filePath(user.home(albumManifest)), &manifest); err != nil {
		return nil, err
	}
	if ts > 0 && ts < manifest.DeleteHorizon {
		return nil, ErrUpdateTimestampTooOld
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
	var contactList ContactList
	if err := d.storage.ReadDataFile(d.filePath(user.home(contactListFile)), &contactList); err != nil {
		return nil, err
	}
	if ts > 0 && ts < contactList.DeleteHorizon {
		return nil, ErrUpdateTimestampTooOld
	}
	for _, d := range contactList.Deletes {
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
	eCh := make(chan error)
	count := 0
	for _, set := range []string{stingle.GallerySet, stingle.TrashSet, stingle.AlbumSet} {
		if set == stingle.AlbumSet {
			for _, a := range manifest.Albums {
				count++
				go d.deleteUpdatesForSet(user, set, a.AlbumID, ts, ch, eCh)
			}
		} else {
			count++
			go d.deleteUpdatesForSet(user, set, "", ts, ch, eCh)
		}
	}
	var errorList []error
	go func() {
		for i := 0; i < count; i++ {
			if err := <-eCh; err != nil {
				errorList = append(errorList, err)
			}
		}
		close(ch)
	}()

	for de := range ch {
		out = append(out, de)
	}
	for _, err := range errorList {
		if err == ErrUpdateTimestampTooOld {
			return nil, err
		}
	}
	if errorList != nil {
		return nil, fmt.Errorf("%w %v", errorList[0], errorList[1:])
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
	if err := d.storage.ReadDataFile(d.filePath(user.home(albumManifest)), &manifest); err != nil {
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
