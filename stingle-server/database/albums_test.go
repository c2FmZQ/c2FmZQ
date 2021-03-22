package database_test

import (
	"fmt"
	"github.com/go-test/deep"
	"testing"

	"stingle-server/crypto"
	"stingle-server/database"
)

func addAlbum(db *database.Database, user database.User, albumID string) error {
	as := database.AlbumSpec{
		OwnerID:       user.UserID,
		AlbumID:       albumID,
		DateCreated:   1000,
		DateModified:  1001,
		EncPrivateKey: "album-key",
		Metadata:      "album-metadata",
		PublicKey:     "album-publickey",
	}
	return db.AddAlbum(user, as)
}

func TestAlbums(t *testing.T) {
	dir := t.TempDir()
	db := database.New(dir)
	email := "alice@"
	key := crypto.MakeSecretKey()
	database.CurrentTimeForTesting = 10000

	if err := addUser(db, email, key.PublicKey()); err != nil {
		t.Fatalf("addUser(%q, pk) failed: %v", email, err)
	}
	user, err := db.User(email)
	if err != nil {
		t.Fatalf("db.User(%q) failed: %v", email, err)
	}
	if err := addAlbum(db, user, "my-album"); err != nil {
		t.Fatalf("addAlbum(%q, %q) failed: %v", user.Email, "my-album", err)
	}

	// Add 10 files in Gallery.
	for i := 0; i < 10; i++ {
		f := fmt.Sprintf("file%d", i)
		if err := addFile(db, user, f, database.GallerySet, ""); err != nil {
			t.Errorf("addFile(%q, %q, %q) failed: %v", f, database.GallerySet, "", err)
		}
	}

	// Move 4 files from Gallery to Trash.
	mvp := database.MoveFileParams{
		SetFrom:   database.GallerySet,
		SetTo:     database.AlbumSet,
		AlbumIDTo: "my-album",
		IsMoving:  true,
		Filenames: []string{"file1", "file2", "file3", "file4"},
		Headers:   []string{"hdr1", "hdr2", "hdr3", "hdr4"},
	}
	if err := db.MoveFile(user, mvp); err != nil {
		t.Fatalf("db.MoveFile(%q, %v) failed: %v", user.Email, mvp, err)
	}

	// Check the new number of files in Gallery and Album.
	gallerySize := numFilesInSet(t, db, user, database.GallerySet, "")
	if want, got := 6, gallerySize; want != got {
		t.Errorf("Unexpected number of files in Gallery: Want %d, got %d", want, got)
	}
	albumSize := numFilesInSet(t, db, user, database.AlbumSet, "my-album")
	if want, got := 4, albumSize; want != got {
		t.Errorf("Unexpected number of files in Album: Want %d, got %d", want, got)
	}

	// Add Bob.
	bobKey := crypto.MakeSecretKey()
	if err := addUser(db, "bob@", bobKey.PublicKey()); err != nil {
		t.Fatalf("addUser(%q, pk) failed: %v", "bob@", err)
	}
	bobUser, err := db.User("bob@")
	if err != nil {
		t.Fatalf("db.User(%q) failed: %v", "bob@", err)
	}

	// Share album with Bob.
	stingleAlbum := database.StingleAlbum{
		AlbumID:     "my-album",
		IsShared:    "1",
		Permissions: "1111",
		Members:     fmt.Sprintf("%d,%d", user.UserID, bobUser.UserID),
	}
	sharingKeys := map[string]string{fmt.Sprintf("%d", bobUser.UserID): "bob's sharing key"}
	if err := db.ShareAlbum(user, &stingleAlbum, sharingKeys); err != nil {
		t.Errorf("db.ShareAlbum(%q, %v) failed: %v", user.Email, stingleAlbum, err)
	}
	fs, err := db.FileSet(bobUser, database.AlbumSet, "my-album")
	if err != nil {
		t.Fatalf("d.FileSet(%q, %q, %q) failed: %v", bobUser.Email, database.AlbumSet, "my-album", err)
	}

	expAlbumSpec := database.AlbumSpec{
		OwnerID:       1,
		AlbumID:       "my-album",
		DateCreated:   1000,
		DateModified:  10000,
		EncPrivateKey: "album-key",
		Metadata:      "album-metadata",
		PublicKey:     "album-publickey",
		IsShared:      true,
		IsHidden:      false,
		Permissions:   "1111",
		IsLocked:      false,
		Cover:         "",
		Members:       map[int]bool{1: true, 2: true},
		SyncLocal:     false,
		SharingKeys:   map[int]string{2: "bob's sharing key"},
	}
	if diff := deep.Equal(expAlbumSpec, *fs.Album); diff != nil {
		t.Errorf("Album data has unexpected value: %v", diff)
	}

	aliceUpdates, err := db.AlbumUpdates(user, 0)
	if err != nil {
		t.Fatalf("db.AlbumUpdates(%q, 0) failed: %v", user.Email, err)
	}
	expAliceUpdates := []database.StingleAlbum{
		database.StingleAlbum{
			AlbumID:       "my-album",
			DateCreated:   "1000",
			DateModified:  "10000",
			EncPrivateKey: "album-key",
			Metadata:      "album-metadata",
			PublicKey:     "album-publickey",
			IsShared:      "1",
			IsHidden:      "0",
			IsOwner:       "1",
			Permissions:   "1111",
			IsLocked:      "0",
			Cover:         "",
			Members:       "1,2",
			SyncLocal:     "",
		},
	}
	if diff := deep.Equal(expAliceUpdates, aliceUpdates); diff != nil {
		t.Errorf("Alice's updates have unexpected value: %v", diff)
	}

	bobUpdates, err := db.AlbumUpdates(bobUser, 0)
	if err != nil {
		t.Fatalf("db.AlbumUpdates(%q, 0) failed: %v", bobUser.Email, err)
	}
	expBobUpdates := []database.StingleAlbum{
		database.StingleAlbum{
			AlbumID:       "my-album",
			DateCreated:   "1000",
			DateModified:  "10000",
			EncPrivateKey: "bob's sharing key",
			Metadata:      "album-metadata",
			PublicKey:     "album-publickey",
			IsShared:      "1",
			IsHidden:      "0",
			IsOwner:       "0",
			Permissions:   "1111",
			IsLocked:      "0",
			Cover:         "",
			Members:       "1,2",
			SyncLocal:     "",
		},
	}
	if diff := deep.Equal(expBobUpdates, bobUpdates); diff != nil {
		t.Errorf("Bob's updates have unexpected value: %v", diff)
	}
}
