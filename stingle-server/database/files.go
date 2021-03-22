package database

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"stingle-server/log"
)

const (
	fileSetPattern = "fileset-%s.json"

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

type StingleFile struct {
	File         string `json:"file"`
	Version      string `json:"version"`
	DateCreated  string `json:"dateCreated"`
	DateModified string `json:"dateModified"`
	Headers      string `json:"headers"`
	AlbumID      string `json:"albumId"`
}

type BlobSpec struct {
	RefCount int `json:"refCount"`
}

func (d *Database) incRefCount(blob string, delta int) int {
	var blobSpec BlobSpec
	done, err := openForUpdate(blob+".json", &blobSpec)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Fatalf("incRefCount(%q, %d) failed: %v", blob, delta, err)
	}
	blobSpec.RefCount += delta
	if err := done(nil); err != nil {
		log.Fatalf("incRefCount(%q, %d) failed: %v", blob, delta, err)
	}
	log.Infof("RefCount(%q)%+d -> %d", blob, delta, blobSpec.RefCount)
	showCallStack()
	if blobSpec.RefCount == 0 {
		if err := os.Remove(blob); err != nil {
			log.Errorf("os.Remove(%q) failed: %v", blob, err)
		}
		if err := os.Remove(blob + ".json"); err != nil {
			log.Errorf("os.Remove(%q) failed: %v", blob+".json", err)
		}
	}
	return blobSpec.RefCount
}

func (d *Database) fileSetPath(user User, set string) string {
	return filepath.Join(d.Home(user.Email), fmt.Sprintf(fileSetPattern, set))
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
	done, err := openForUpdate(fileName, &fileSet)
	if err != nil {
		log.Errorf("openForUpdate(%q): %v", fileName, err)
		return err
	}
	defer done(&retErr)

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
	dir := filepath.Join(d.Dir(), "blobs", fmt.Sprintf("%02X", name[0]), fmt.Sprintf("%02X", name[1]))
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
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

	if err := os.Rename(file.StoreFile, fn); err != nil {
		return err
	}
	file.StoreFile = fn
	if err := os.Rename(file.StoreThumb, tn); err != nil {
		return err
	}
	file.StoreThumb = tn
	file.DateModified = nowInMS()

	if err := d.addFileToFileSet(user, file); err != nil {
		if err := os.Remove(fn); err != nil {
			log.Errorf("os.Remove(%q) failed: %v", fn, err)
		}
		if err := os.Remove(tn); err != nil {
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
	if err := loadJSON(fileName, &fileSet); err != nil && !errors.Is(err, os.ErrNotExist) {
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

func (d *Database) fileSetForUpdate(user User, set, albumID string) (func(*error) error, *FileSet, error) {
	var fileName string
	if set == AlbumSet {
		albumRef, err := d.albumRef(user, albumID)
		if err != nil {
			return nil, nil, err
		}
		fileName = albumRef.File
	} else {
		fileName = d.fileSetPath(user, set)
	}
	var fileSet FileSet
	done, err := openForUpdate(fileName, &fileSet)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, nil, err
	}
	if fileSet.Files == nil {
		fileSet.Files = make(map[string]*FileSpec)
	}
	if fileSet.Deletes == nil {
		fileSet.Deletes = []DeleteEvent{}
	}
	return done, &fileSet, nil
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
	done1, fsTo, err := d.fileSetForUpdate(user, p.SetTo, p.AlbumIDTo)
	if err != nil {
		log.Errorf("fileSetForUpdate(%q, %q, %q) failed: %v", user.Email, p.SetTo, p.AlbumIDTo, err)
		return err
	}
	defer done1(&retErr)

	done2, fsFrom, err := d.fileSetForUpdate(user, p.SetFrom, p.AlbumIDFrom)
	if err != nil {
		log.Errorf("fileSetForUpdate(%q, %q, %q) failed: %v", user.Email, p.SetFrom, p.AlbumIDFrom, err)
		return err
	}
	defer done2(&retErr)

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

		fsTo.Files[fn] = &toFile

		if p.IsMoving {
			delete(fsFrom.Files, fn)
			de := DeleteEvent{
				File:    fn,
				AlbumID: p.AlbumIDFrom,
				Date:    nowInMS(),
			}
			// 1: Gallery 2: Trash 3: Trash&delete 4: Album 5: Album file 6: Contact
			if p.SetFrom == GallerySet {
				de.Type = 1
			} else if p.SetFrom == TrashSet {
				de.Type = 2
			} else {
				de.Type = 5
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
	done, fs, err := d.fileSetForUpdate(user, TrashSet, "")
	if err != nil {
		log.Errorf("fileSetForUpdate(%q, %q, %q) failed: %v", user.Email, TrashSet, "", err)
		return err
	}
	defer done(&retErr)
	for k, v := range fs.Files {
		if v.DateModified <= t {
			if file, ok := fs.Files[k]; ok {
				d.incRefCount(file.StoreFile, -1)
				d.incRefCount(file.StoreThumb, -1)
			}
			delete(fs.Files, k)
			de := DeleteEvent{
				File: k,
				Type: 3,
				Date: t,
			}
			fs.Deletes = append(fs.Deletes, de)
		}
	}
	return nil
}

func (d *Database) DeleteFiles(user User, files []string) (retErr error) {
	done, fs, err := d.fileSetForUpdate(user, TrashSet, "")
	if err != nil {
		log.Errorf("fileSetForUpdate(%q, %q, %q) failed: %v", user.Email, TrashSet, "", err)
		return err
	}
	defer done(&retErr)
	for _, f := range files {
		if file, ok := fs.Files[f]; ok {
			d.incRefCount(file.StoreFile, -1)
			d.incRefCount(file.StoreThumb, -1)
		}
		delete(fs.Files, f)
		de := DeleteEvent{
			File: f,
			Type: 3,
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
		return os.Open(fileSpec.StoreThumb)
	}
	return os.Open(fileSpec.StoreFile)
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
