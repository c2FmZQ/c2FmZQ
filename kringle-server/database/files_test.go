package database_test

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"kringle-server/database"
	"kringle-server/stingle"
)

func addFile(db *database.Database, user database.User, name, set, albumID string) error {
	fs := database.FileSpec{
		File:           name,
		Headers:        name + "-headers",
		Set:            set,
		AlbumID:        albumID,
		DateCreated:    1,
		DateModified:   2,
		Version:        "1",
		StoreFile:      filepath.Join(db.Dir(), "upload-"+name),
		StoreFileSize:  1000,
		StoreThumb:     filepath.Join(db.Dir(), "upload-thumb-"+name),
		StoreThumbSize: 100,
	}
	if err := os.WriteFile(fs.StoreFile, []byte("file content"), 0600); err != nil {
		return err
	}
	if err := os.WriteFile(fs.StoreThumb, []byte("thumb content"), 0600); err != nil {
		return err
	}
	return db.AddFile(user, fs)
}

func numFilesInSet(t *testing.T, db *database.Database, user database.User, set, albumID string) int {
	fs, err := db.FileSet(user, set, albumID)
	if err != nil {
		t.Fatalf("db.FileSet(%q, %q, %q) failed: %v", user.Email, set, albumID, err)
	}
	return len(fs.Files)
}

func TestFiles(t *testing.T) {
	dir := t.TempDir()
	db := database.New(dir, "")
	email := "alice@"
	key := stingle.MakeSecretKey()
	database.CurrentTimeForTesting = 10000

	if err := addUser(db, email, key.PublicKey()); err != nil {
		t.Fatalf("addUser(%q, pk) failed: %v", email, err)
	}
	user, err := db.User(email)
	if err != nil {
		t.Fatalf("db.User(%q) failed: %v", email, err)
	}

	// Add 10 files in Gallery.
	for i := 0; i < 10; i++ {
		f := fmt.Sprintf("file%d", i)
		if err := addFile(db, user, f, database.GallerySet, ""); err != nil {
			t.Errorf("addFile(%q, %q, %q) failed: %v", f, database.GallerySet, "", err)
		}
	}
	// Adding a file to a non-existent album should fail.
	if err := addFile(db, user, "fileX", database.AlbumSet, "NonExistenAlbum"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("addFile(fileX, AlbumSet, 'NonExistenAlbum') returned unexpected error: want %v, got %v", os.ErrNotExist, err)
	}

	f, err := db.DownloadFile(user, database.GallerySet, "file1", false)
	if err != nil {
		t.Fatalf("db.DownloadFile(%q, %q, %q) failed: %v", user.Email, database.GallerySet, "file1", false)
	}
	slurp, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("io.ReadAll(f) failed: %v", err)
	}
	f.Close()
	if want, got := "file content", string(slurp); want != got {
		t.Errorf("Unexpected file content: want %q, got %q", want, got)
	}

	// Check the number of files in Gallery and Trash.
	gallerySize := numFilesInSet(t, db, user, database.GallerySet, "")
	if want, got := 10, gallerySize; want != got {
		t.Errorf("Unexpected number of files in Gallery: Want %d, got %d", want, got)
	}
	trashSize := numFilesInSet(t, db, user, database.TrashSet, "")
	if want, got := 0, trashSize; want != got {
		t.Errorf("Unexpected number of files in Trash: Want %d, got %d", want, got)
	}

	// Move 4 files from Gallery to Trash.
	mvp := database.MoveFileParams{
		SetFrom:   database.GallerySet,
		SetTo:     database.TrashSet,
		IsMoving:  true,
		Filenames: []string{"file1", "file2", "file3", "file4"},
	}
	if err := db.MoveFile(user, mvp); err != nil {
		t.Fatalf("db.MoveFile(%q, %v) failed: %v", user.Email, mvp, err)
	}

	// Check the new number of files in Gallery and Trash.
	gallerySize = numFilesInSet(t, db, user, database.GallerySet, "")
	if want, got := 6, gallerySize; want != got {
		t.Errorf("Unexpected number of files in Gallery: Want %d, got %d", want, got)
	}
	trashSize = numFilesInSet(t, db, user, database.TrashSet, "")
	if want, got := 4, trashSize; want != got {
		t.Errorf("Unexpected number of files in Trash: Want %d, got %d", want, got)
	}

	// Delete 2 files from Trash.
	toDelete := []string{"file1", "file2"}
	if err := db.DeleteFiles(user, toDelete); err != nil {
		t.Fatalf("db.DeleteFiles(%q, %v) failed: %v", user.Email, toDelete, err)
	}

	// Check the new number of files in Gallery and Trash.
	gallerySize = numFilesInSet(t, db, user, database.GallerySet, "")
	if want, got := 6, gallerySize; want != got {
		t.Errorf("Unexpected number of files in Gallery: Want %d, got %d", want, got)
	}
	trashSize = numFilesInSet(t, db, user, database.TrashSet, "")
	if want, got := 2, trashSize; want != got {
		t.Errorf("Unexpected number of files in Trash: Want %d, got %d", want, got)
	}

	// Empty the Trash.
	now := time.Now().UnixNano() / 1000000
	if err := db.EmptyTrash(user, now); err != nil {
		t.Fatalf("db.EmptyTrash(%q, %d) failed: %v", user.Email, now, err)
	}

	// Check the new number of files in Gallery and Trash.
	gallerySize = numFilesInSet(t, db, user, database.GallerySet, "")
	if want, got := 6, gallerySize; want != got {
		t.Errorf("Unexpected number of files in Gallery: Want %d, got %d", want, got)
	}
	trashSize = numFilesInSet(t, db, user, database.TrashSet, "")
	if want, got := 0, trashSize; want != got {
		t.Errorf("Unexpected number of files in Trash: Want %d, got %d", want, got)
	}
}
