package database

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"kringle-server/log"
	"kringle-server/stingle"
)

const (
	fileSetPattern = "fileset-%s"

	GallerySet = "0"
	TrashSet   = "1"
	AlbumSet   = "2"
)

type FileSet struct {
	Album   *AlbumSpec           `json:"album,omitempty"`
	Files   map[string]*FileSpec `json:"files"`
	Deletes []DeleteEvent        `json:"deletes,omitempty"`
}

type FileSpec struct {
	File           string `json:"file"`
	Headers        string `json:"headers"`
	Set            string `json:"set"`
	DateCreated    int64  `json:"dateCreated"`
	AlbumID        string `json:"albumId,omitempty"`
	DateModified   int64  `json:"dateModified"`
	Version        string `json:"version"`
	StoreFile      string `json:"storeFile"`
	StoreFileSize  int64  `json:"storeFilesize"`
	StoreThumb     string `json:"storeThumb"`
	StoreThumbSize int64  `json:"storeThumbSize"`
}

type BlobSpec struct {
	RefCount int `json:"refCount"`
}

func (d *Database) incRefCount(blob string, delta int) int {
	var blobSpec BlobSpec
	commit, err := d.storage.OpenForUpdate(blob+".ref", &blobSpec)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Fatalf("incRefCount(%q, %d) failed: %v", blob, delta, err)
	}
	blobSpec.RefCount += delta
	if err := commit(true, nil); err != nil {
		log.Fatalf("incRefCount(%q, %d) failed: %v", blob, delta, err)
	}
	log.Debugf("RefCount(%q)%+d -> %d", blob, delta, blobSpec.RefCount)
	if blobSpec.RefCount == 0 {
		if err := os.Remove(blob); err != nil {
			log.Errorf("os.Remove(%q) failed: %v", blob, err)
		}
		if err := os.Remove(blob + ".ref"); err != nil {
			log.Errorf("os.Remove(%q) failed: %v", blob+".ref", err)
		}
	}
	return blobSpec.RefCount
}

func (d *Database) fileSetPath(user User, set string) string {
	return d.filePath(user.home(fmt.Sprintf(fileSetPattern, set)))
}

func (d *Database) addFileToFileSet(user User, file FileSpec) (retErr error) {
	var fileName string
	if file.Set == AlbumSet {
		albumRef, err := d.albumRef(user, file.AlbumID)
		if err != nil {
			return err
		}
		fileName = albumRef.File
	} else {
		fileName = d.fileSetPath(user, file.Set)
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
	fileSet.Files[file.File] = &file
	d.incRefCount(file.StoreFile, 1)
	d.incRefCount(file.StoreThumb, 1)
	return nil
}

func (d *Database) makeFilePath() (string, error) {
	name := make([]byte, 32)
	if _, err := rand.Read(name); err != nil {
		return "", err
	}
	dir := filepath.Join("blobs", fmt.Sprintf("%02X", name[0]), fmt.Sprintf("%02X", name[1]))
	return filepath.Join(dir, base64.RawURLEncoding.EncodeToString(name)), nil
}

func (d *Database) AddFile(user User, file FileSpec) error {
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

	if err := d.addFileToFileSet(user, file); err != nil {
		if err := os.Remove(filepath.Join(d.Dir(), fn)); err != nil {
			log.Errorf("os.Remove(%q) failed: %v", fn, err)
		}
		if err := os.Remove(filepath.Join(d.Dir(), tn)); err != nil {
			log.Errorf("os.Remove(%q) failed: %v", tn, err)
		}
		return err
	}
	return nil
}

func (d *Database) FileSet(user User, set, albumID string) (*FileSet, error) {
	var fileName string
	if set == AlbumSet {
		albumRef, err := d.albumRef(user, albumID)
		if err != nil {
			return nil, err
		}
		fileName = albumRef.File
	} else {
		fileName = d.fileSetPath(user, set)
	}
	var fileSet FileSet
	if _, err := d.storage.ReadDataFile(fileName, &fileSet); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if fileSet.Files == nil {
		fileSet.Files = make(map[string]*FileSpec)
	}
	if fileSet.Deletes == nil {
		fileSet.Deletes = []DeleteEvent{}
	}
	return &fileSet, nil
}

func (d *Database) fileSetForUpdate(user User, set, albumID string) (func(bool, *error) error, *FileSet, error) {
	commit, fileSets, err := d.fileSetsForUpdate(user, []string{set}, []string{albumID})
	if err != nil {
		return nil, nil, err
	}
	return commit, fileSets[0], nil
}

func (d *Database) fileSetsForUpdate(user User, sets, albumIDs []string) (func(bool, *error) error, []*FileSet, error) {
	var filenames []string
	for i := range sets {
		if sets[i] == AlbumSet {
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

type MoveFileParams struct {
	SetFrom     string
	SetTo       string
	AlbumIDFrom string
	AlbumIDTo   string
	IsMoving    bool
	Filenames   []string
	Headers     []string
}

func (d *Database) MoveFile(user User, p MoveFileParams) (retErr error) {
	if p.SetTo == p.SetFrom && p.AlbumIDTo == p.AlbumIDFrom {
		return errors.New("src and dest are the same")
	}
	commit, fileSets, err := d.fileSetsForUpdate(user, []string{p.SetTo, p.SetFrom}, []string{p.AlbumIDTo, p.AlbumIDFrom})
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
		toFile.Set = p.SetTo
		toFile.AlbumID = p.AlbumIDTo
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
			if p.SetFrom == GallerySet {
				de.Type = stingle.DeleteEventGallery
			} else if p.SetFrom == TrashSet {
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
	return nil
}

func (d *Database) EmptyTrash(user User, t int64) (retErr error) {
	commit, fs, err := d.fileSetForUpdate(user, TrashSet, "")
	if err != nil {
		log.Errorf("fileSetForUpdate(%q, %q, %q) failed: %v", user.Email, TrashSet, "", err)
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
	return nil
}

func (d *Database) DeleteFiles(user User, files []string) (retErr error) {
	commit, fs, err := d.fileSetForUpdate(user, TrashSet, "")
	if err != nil {
		log.Errorf("fileSetForUpdate(%q, %q, %q) failed: %v", user.Email, TrashSet, "", err)
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
	return nil
}

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

func (d *Database) downloadFileSpec(fileSpec *FileSpec, thumb bool) (*os.File, error) {
	if thumb {
		return os.Open(filepath.Join(d.Dir(), fileSpec.StoreThumb))
	}
	return os.Open(filepath.Join(d.Dir(), fileSpec.StoreFile))
}

func (d *Database) DownloadFile(user User, set, filename string, thumb bool) (*os.File, error) {
	if set != AlbumSet {
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
	for _, album := range albumRefs {
		fileSpec, err := d.findFileInSet(user, AlbumSet, album.AlbumID, filename)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			log.Errorf("findFileInSet(%q, %q, %q, %q, %v) failed: %v", user.Email, AlbumSet, album.AlbumID, filename, thumb, err)
			return nil, err
		}
		return d.downloadFileSpec(fileSpec, thumb)
	}
	return nil, os.ErrNotExist
}
