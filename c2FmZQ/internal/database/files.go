package database

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"c2FmZQ/internal/log"
	"c2FmZQ/internal/stingle"
)

const (
	fileSetPattern = "fileset-%s"
)

var (
	ErrQuotaExceeded = errors.New("quota exceeded")
)

// FileSet encapsulates to information of a file set, i.e. a group of files
// like the Gallery, the Trash, or albums.
type FileSet struct {
	// If the file set is an album, Album points to the album spec.
	Album *AlbumSpec `json:"album,omitempty"`
	// All the files in the file set, keyed by file name.
	Files map[string]*FileSpec `json:"files"`
	// The deletion events for the file set.
	Deletes []DeleteEvent `json:"deletes,omitempty"`
	// The timestamp before which DeleteEvents were pruned.
	DeleteHorizon int64 `json:"deleteHorizon,omitempty"`
}

// FileSpec encapsulates the information of a file.
type FileSpec struct {
	// The file headers, i.e. encrypted file key.
	Headers string `json:"headers"`
	// The time when the file was created.
	DateCreated int64 `json:"dateCreated"`
	// The time when the file was modified, e.g. added to a set.
	DateModified int64 `json:"dateModified"`
	// Version?
	Version string `json:"version"`
	// The file path where the file content is stored.
	StoreFile string `json:"storeFile"`
	// The size of the file content.
	StoreFileSize int64 `json:"storeFilesize"`
	// The file path where the file thumbnail is stored.
	StoreThumb string `json:"storeThumb"`
	// The size of the file thumbnail.
	StoreThumbSize int64 `json:"storeThumbSize"`
}

// BlobSpec encapsulated the information of a blob (the content of a file).
type BlobSpec struct {
	// The number of FileSpecs that point to this blob.
	RefCount int `json:"refCount"`
}

// incRefCount increases the RefCount of a blob by delta, which can be negative.
func (d *Database) incRefCount(blob string, delta int) int {
	var blobSpec BlobSpec
	commit, err := d.storage.OpenForUpdate(blob+".ref", &blobSpec)
	if err != nil {
		log.Fatalf("incRefCount(%q, %d) failed: %v", blob, delta, err)
	}
	blobSpec.RefCount += delta
	if err := commit(true, nil); err != nil {
		log.Fatalf("incRefCount(%q, %d) failed: %v", blob, delta, err)
	}
	log.Debugf("RefCount(%q)%+d -> %d", blob, delta, blobSpec.RefCount)
	if blobSpec.RefCount == 0 {
		fn := filepath.Join(d.dir, blob)
		if err := os.Remove(fn); err != nil {
			log.Errorf("os.Remove(%q) failed: %v", blob, err)
		}
		if err := os.Remove(fn + ".ref"); err != nil {
			log.Errorf("os.Remove(%q) failed: %v", blob+".ref", err)
		}
	}
	return blobSpec.RefCount
}

// fileSetPath returns the path where a file set is stored.
func (d *Database) fileSetPath(user User, set string) string {
	return d.filePath(user.home(fmt.Sprintf(fileSetPattern, set)))
}

// addFileToFileSet adds file to one of user's file sets.
func (d *Database) addFileToFileSet(user User, file FileSpec, name, set, albumID string) (retErr error) {
	var fileName string
	if set == stingle.AlbumSet {
		albumRef, err := d.albumRef(user, albumID)
		if err != nil {
			return err
		}
		fileName = albumRef.File
	} else {
		fileName = d.fileSetPath(user, set)
	}
	var fileSet FileSet
	commit, err := d.storage.OpenForUpdate(fileName, &fileSet)
	if err != nil {
		log.Errorf("d.storage.OpenForUpdate(%q): %v", fileName, err)
		return err
	}
	defer commit(true, &retErr)

	if fileSet.Files == nil {
		fileSet.Files = make(map[string]*FileSpec)
	}
	if fileSet.Deletes == nil {
		fileSet.Deletes = []DeleteEvent{}
	}
	fileSet.Files[name] = &file
	d.storage.CreateEmptyFile(file.StoreFile+".ref", BlobSpec{})
	d.storage.CreateEmptyFile(file.StoreThumb+".ref", BlobSpec{})
	d.incRefCount(file.StoreFile, 1)
	d.incRefCount(file.StoreThumb, 1)
	return nil
}

// makeFilePath creates a random file name for a new file.
func (d *Database) makeFilePath() (string, error) {
	name := make([]byte, 32)
	if _, err := rand.Read(name); err != nil {
		return "", err
	}
	dir := filepath.Join("blobs", fmt.Sprintf("%02X", name[0]))
	return filepath.Join(dir, base64.RawURLEncoding.EncodeToString(name)), nil
}

// AddFile adds a new file to the database. The file content and thumbnail are
// already on disk in temporary files (file.StoreFile and file.StoreThumb). They
// will be moved to random file names.
func (d *Database) AddFile(user User, file FileSpec, name, set, albumID string) error {
	defer recordLatency("AddFile")()

	spaceUsed, err := d.SpaceUsed(user)
	if err != nil {
		return err
	}
	quota, err := d.Quota(user.UserID)
	if err != nil {
		return err
	}
	if total := spaceUsed + file.StoreFileSize + file.StoreThumbSize; total > quota {
		log.Errorf("User quota exceeded: %d > %d", total, quota)
		os.Remove(file.StoreFile)
		os.Remove(file.StoreThumb)
		return ErrQuotaExceeded
	}

	fn, err := d.makeFilePath()
	if err != nil {
		log.Errorf("makeFilePath() failed: %v", err)
		return err
	}
	tn, err := d.makeFilePath()
	if err != nil {
		log.Errorf("makeFilePath() failed: %v", err)
		return err
	}

	if err := createParentIfNotExist(filepath.Join(filepath.Join(d.Dir(), fn))); err != nil {
		return err
	}
	if err := os.Rename(file.StoreFile, filepath.Join(d.Dir(), fn)); err != nil {
		return err
	}
	file.StoreFile = fn
	if err := createParentIfNotExist(filepath.Join(filepath.Join(d.Dir(), tn))); err != nil {
		return err
	}
	if err := os.Rename(file.StoreThumb, filepath.Join(d.Dir(), tn)); err != nil {
		return err
	}
	file.StoreThumb = tn
	file.DateModified = nowInMS()

	if err := d.addFileToFileSet(user, file, name, set, albumID); err != nil {
		for _, f := range []string{fn, tn, fn + ".ref", tn + ".ref"} {
			if err := os.Remove(filepath.Join(d.Dir(), f)); err != nil {
				log.Errorf("os.Remove(%q) failed: %v", f, err)
			}
		}
		return err
	}
	return nil
}

func (d *Database) mtime(f string) int64 {
	fi, err := os.Stat(filepath.Join(d.Dir(), f))
	if err != nil {
		return 0
	}
	return fi.ModTime().UnixNano()
}

// FileSet retrives a given file set, for reading only.
func (d *Database) FileSet(user User, set, albumID string) (*FileSet, error) {
	defer recordLatency("FileSet")()

	var fileName string
	if set == stingle.AlbumSet {
		albumRef, err := d.albumRef(user, albumID)
		if err != nil {
			return nil, err
		}
		fileName = albumRef.File
	} else {
		fileName = d.fileSetPath(user, set)
	}

	key := struct {
		name string
		ts   int64
	}{fileName, d.mtime(fileName)}
	d.fileSetCacheMutex.Lock()
	defer d.fileSetCacheMutex.Unlock()
	var fileSet FileSet

	if fs, ok := d.fileSetCache.Get(key); ok {
		log.Debugf("FileSet cache hit %v", key)
		return fs.(*FileSet), nil
	}
	log.Debugf("FileSet cache miss %v", key)

	if err := d.storage.ReadDataFile(fileName, &fileSet); err != nil {
		return nil, err
	}
	if fileSet.Files == nil {
		fileSet.Files = make(map[string]*FileSpec)
	}
	if fileSet.Deletes == nil {
		fileSet.Deletes = []DeleteEvent{}
	}
	if d.mtime(fileName) == key.ts {
		d.fileSetCache.Add(key, &fileSet)
	}
	return &fileSet, nil
}

// fileSetForUpdate retrieves a file set for update.
func (d *Database) fileSetForUpdate(user User, set, albumID string) (func(bool, *error) error, *FileSet, error) {
	commit, fileSets, err := d.fileSetsForUpdate(user, []string{set}, []string{albumID})
	if err != nil {
		return nil, nil, err
	}
	return commit, fileSets[0], nil
}

// fileSetsForUpdate retrieves any number of file sets for atomic update.
func (d *Database) fileSetsForUpdate(user User, sets, albumIDs []string) (func(bool, *error) error, []*FileSet, error) {
	var filenames []string
	for i := range sets {
		if sets[i] == stingle.AlbumSet {
			albumRef, err := d.albumRef(user, albumIDs[i])
			if err != nil {
				return nil, nil, err
			}
			filenames = append(filenames, albumRef.File)
			continue
		}
		filenames = append(filenames, d.fileSetPath(user, sets[i]))
	}

	fileSets := make([]*FileSet, len(filenames))
	for i := range fileSets {
		fileSets[i] = &FileSet{}
	}
	commit, err := d.storage.OpenManyForUpdate(filenames, fileSets)
	if err != nil {
		return nil, nil, err
	}
	for _, fs := range fileSets {
		if fs.Files == nil {
			fs.Files = make(map[string]*FileSpec)
		}
		if fs.Deletes == nil {
			fs.Deletes = []DeleteEvent{}
		}
	}
	return commit, fileSets, nil
}

// MoveFileParams specifies what a move operation does.
type MoveFileParams struct {
	// The set where the files originate.
	SetFrom string
	// The set where the files are going.
	SetTo string
	// The album where the files originate, or empty if not an album.
	AlbumIDFrom string
	// The album where the files are going, or empty if not an album.
	AlbumIDTo string
	// True is the files are moving. False if they are being copied.
	IsMoving bool
	// The files moving or being copied.
	Filenames []string
	// The new headers for the files, or empty if the headers aren't
	// changing.
	Headers []string
}

// MoveFile moves or copies files between file sets.
func (d *Database) MoveFile(user User, p MoveFileParams) (retErr error) {
	defer recordLatency("MoveFile")()

	var (
		commit   func(bool, *error) error
		fileSets []*FileSet
		err      error
	)
	if p.SetTo == p.SetFrom && p.AlbumIDTo == p.AlbumIDFrom {
		p.IsMoving = false
		c, fs, e := d.fileSetForUpdate(user, p.SetFrom, p.AlbumIDFrom)
		commit, fileSets, err = c, []*FileSet{fs, fs}, e
	} else {
		commit, fileSets, err = d.fileSetsForUpdate(user, []string{p.SetTo, p.SetFrom}, []string{p.AlbumIDTo, p.AlbumIDFrom})
	}
	if err != nil {
		log.Errorf("fileSetsForUpdate(%q, {%q, %q}, {%q, %q}) failed: %v",
			user.Email, p.SetTo, p.SetFrom, p.AlbumIDTo, p.AlbumIDFrom, err)
		return err
	}
	defer commit(true, &retErr)
	fsTo, fsFrom := fileSets[0], fileSets[1]

	for i := range p.Filenames {
		fn := p.Filenames[i]
		fromFile := fsFrom.Files[fn]
		if fromFile == nil {
			continue
		}
		toFile := *fromFile
		if len(p.Headers) == len(p.Filenames) {
			toFile.Headers = p.Headers[i]
		}
		var refCountAdj int
		_, alreadyExists := fsTo.Files[fn]
		switch {
		case alreadyExists && p.IsMoving:
			refCountAdj = -1
		case !alreadyExists && !p.IsMoving:
			refCountAdj = 1
		}

		toFile.DateModified = nowInMS()
		fsTo.Files[fn] = &toFile

		if p.IsMoving {
			delete(fsFrom.Files, fn)
			de := DeleteEvent{
				File:    fn,
				AlbumID: p.AlbumIDFrom,
				Date:    nowInMS(),
			}
			if p.SetFrom == stingle.GallerySet {
				de.Type = stingle.DeleteEventGallery
			} else if p.SetFrom == stingle.TrashSet {
				de.Type = stingle.DeleteEventTrash
			} else {
				de.Type = stingle.DeleteEventAlbumFile
			}
			fsFrom.Deletes = append(fsFrom.Deletes, de)
		}
		if refCountAdj != 0 {
			d.incRefCount(toFile.StoreFile, refCountAdj)
			d.incRefCount(toFile.StoreThumb, refCountAdj)
		}
	}
	pruneDeleteEvents(&fsFrom.Deletes, &fsFrom.DeleteHorizon)
	pruneDeleteEvents(&fsTo.Deletes, &fsTo.DeleteHorizon)
	return nil
}

// EmptyTrash deletes the files in the Trash set that were added up to time t.
func (d *Database) EmptyTrash(user User, t int64) (retErr error) {
	defer recordLatency("EmptyTrash")()

	commit, fs, err := d.fileSetForUpdate(user, stingle.TrashSet, "")
	if err != nil {
		log.Errorf("fileSetForUpdate(%q, %q, %q) failed: %v", user.Email, stingle.TrashSet, "", err)
		return err
	}
	defer commit(true, &retErr)
	for k, v := range fs.Files {
		if v.DateModified <= t {
			if file, ok := fs.Files[k]; ok {
				d.incRefCount(file.StoreFile, -1)
				d.incRefCount(file.StoreThumb, -1)
			}
			delete(fs.Files, k)
			de := DeleteEvent{
				File: k,
				Type: stingle.DeleteEventTrashDelete,
				Date: t,
			}
			fs.Deletes = append(fs.Deletes, de)
		}
	}
	pruneDeleteEvents(&fs.Deletes, &fs.DeleteHorizon)
	return nil
}

// DeleteFiles deletes specific files from the Trash set.
func (d *Database) DeleteFiles(user User, files []string) (retErr error) {
	defer recordLatency("DeleteFiles")()

	commit, fs, err := d.fileSetForUpdate(user, stingle.TrashSet, "")
	if err != nil {
		log.Errorf("fileSetForUpdate(%q, %q, %q) failed: %v", user.Email, stingle.TrashSet, "", err)
		return err
	}
	defer commit(true, &retErr)
	for _, f := range files {
		if file, ok := fs.Files[f]; ok {
			d.incRefCount(file.StoreFile, -1)
			d.incRefCount(file.StoreThumb, -1)
		}
		delete(fs.Files, f)
		de := DeleteEvent{
			File: f,
			Type: stingle.DeleteEventTrashDelete,
			Date: nowInMS(),
		}
		fs.Deletes = append(fs.Deletes, de)
	}
	pruneDeleteEvents(&fs.Deletes, &fs.DeleteHorizon)
	return nil
}

// findFileInSet retrieves a given file from a user's file set.
func (d *Database) findFileInSet(user User, set, albumID, filename string) (*FileSpec, error) {
	fs, err := d.FileSet(user, set, albumID)
	if err != nil {
		return nil, err
	}
	if f := fs.Files[filename]; f != nil {
		return f, nil
	}
	return nil, os.ErrNotExist
}

// downloadFileSpec opens a file for reading.
func (d *Database) downloadFileSpec(fileSpec *FileSpec, thumb bool) (*os.File, error) {
	if thumb {
		return os.Open(filepath.Join(d.Dir(), fileSpec.StoreThumb))
	}
	return os.Open(filepath.Join(d.Dir(), fileSpec.StoreFile))
}

// DownloadFile locates a file and opens it for reading.
func (d *Database) DownloadFile(user User, set, filename string, thumb bool) (*os.File, error) {
	defer recordLatency("DownloadFile")()

	if set != stingle.AlbumSet {
		fileSpec, err := d.findFileInSet(user, set, "", filename)
		if err != nil {
			return nil, err
		}
		return d.downloadFileSpec(fileSpec, thumb)
	}

	albumRefs, err := d.AlbumRefs(user)
	if err != nil {
		log.Errorf("AlbumRefs(%q) failed: %v", user.Email, err)
		return nil, err
	}
	// Make sure the cache is big enough for all the filesets. Use 2x to
	// allow two concurrent users without causing evictions.
	if n := 2 * len(albumRefs); n > d.fileSetCacheSize {
		d.fileSetCacheMutex.Lock()
		d.fileSetCacheSize = n
		d.fileSetCache.Resize(n)
		d.fileSetCacheMutex.Unlock()
	}
	for _, album := range albumRefs {
		fileSpec, err := d.findFileInSet(user, stingle.AlbumSet, album.AlbumID, filename)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			log.Errorf("findFileInSet(%q, %q, %q, %q, %v) failed: %v", user.Email, stingle.AlbumSet, album.AlbumID, filename, thumb, err)
			return nil, err
		}
		return d.downloadFileSpec(fileSpec, thumb)
	}
	return nil, os.ErrNotExist
}
