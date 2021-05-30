package database_test

import (
	"fmt"
	"github.com/go-test/deep"
	"sort"
	"strings"
	"testing"

	"c2FmZQ/internal/database"
	"c2FmZQ/internal/stingle"
)

func membersString(ids ...int64) string {
	members := []string{}
	for _, v := range ids {
		members = append(members, fmt.Sprintf("%d", v))
	}
	sort.Strings(members)
	return strings.Join(members, ",")
}

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
	db := database.New(dir, nil)
	email := "alice@"
	key := stingle.MakeSecretKeyForTest()
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
		if err := addFile(db, user, f, stingle.GallerySet, ""); err != nil {
			t.Errorf("addFile(%q, %q, %q) failed: %v", f, stingle.GallerySet, "", err)
		}
	}

	// Move 4 files from Gallery to Trash.
	mvp := database.MoveFileParams{
		SetFrom:   stingle.GallerySet,
		SetTo:     stingle.AlbumSet,
		AlbumIDTo: "my-album",
		IsMoving:  true,
		Filenames: []string{"file1", "file2", "file3", "file4"},
		Headers:   []string{"hdr1", "hdr2", "hdr3", "hdr4"},
	}
	if err := db.MoveFile(user, mvp); err != nil {
		t.Fatalf("db.MoveFile(%q, %v) failed: %v", user.Email, mvp, err)
	}

	// Check the new number of files in Gallery and Album.
	gallerySize := numFilesInSet(t, db, user, stingle.GallerySet, "")
	if want, got := 6, gallerySize; want != got {
		t.Errorf("Unexpected number of files in Gallery: Want %d, got %d", want, got)
	}
	albumSize := numFilesInSet(t, db, user, stingle.AlbumSet, "my-album")
	if want, got := 4, albumSize; want != got {
		t.Errorf("Unexpected number of files in Album: Want %d, got %d", want, got)
	}

	// Add Bob.
	bobKey := stingle.MakeSecretKeyForTest()
	if err := addUser(db, "bob@", bobKey.PublicKey()); err != nil {
		t.Fatalf("addUser(%q, pk) failed: %v", "bob@", err)
	}
	bobUser, err := db.User("bob@")
	if err != nil {
		t.Fatalf("db.User(%q) failed: %v", "bob@", err)
	}

	// Share album with Bob.
	stingleAlbum := stingle.Album{
		AlbumID:     "my-album",
		IsShared:    "1",
		Permissions: "1111",
		Members:     membersString(user.UserID, bobUser.UserID),
	}
	sharingKeys := map[string]string{fmt.Sprintf("%d", bobUser.UserID): "bob's sharing key"}
	if err := db.ShareAlbum(user, &stingleAlbum, sharingKeys); err != nil {
		t.Errorf("db.ShareAlbum(%q, %v) failed: %v", user.Email, stingleAlbum, err)
	}
	fs, err := db.FileSet(bobUser, stingle.AlbumSet, "my-album")
	if err != nil {
		t.Fatalf("d.FileSet(%q, %q, %q) failed: %v", bobUser.Email, stingle.AlbumSet, "my-album", err)
	}

	expAlbumSpec := database.AlbumSpec{
		OwnerID:       user.UserID,
		AlbumID:       "my-album",
		DateCreated:   1000,
		DateModified:  10000,
		EncPrivateKey: "album-key",
		Metadata:      "album-metadata",
		PublicKey:     "album-publickey",
		IsShared:      true,
		Permissions:   "1111",
		Cover:         "",
		Members:       map[int64]bool{user.UserID: true, bobUser.UserID: true},
		SharingKeys:   map[int64]string{bobUser.UserID: "bob's sharing key"},
	}
	if diff := deep.Equal(expAlbumSpec, *fs.Album); diff != nil {
		t.Errorf("Album data has unexpected value: %v", diff)
	}

	aliceUpdates, err := db.AlbumUpdates(user, 0)
	if err != nil {
		t.Fatalf("db.AlbumUpdates(%q, 0) failed: %v", user.Email, err)
	}
	expAliceUpdates := []stingle.Album{
		stingle.Album{
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
			Members:       membersString(user.UserID, bobUser.UserID),
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
	expBobUpdates := []stingle.Album{
		stingle.Album{
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
			Members:       membersString(user.UserID, bobUser.UserID),
			SyncLocal:     "",
		},
	}
	if diff := deep.Equal(expBobUpdates, bobUpdates); diff != nil {
		t.Errorf("Bob's updates have unexpected value: %v", diff)
	}

	if err := db.DeleteUser(user); err != nil {
		t.Fatalf("DeleteUser(alice) failed: %v", err)
	}
	deleteUpdates, err := db.DeleteUpdates(bobUser, 0)
	if err != nil {
		t.Fatalf("db.DeleteUpdates(%q, 0) failed: %v", bobUser.Email, err)
	}
	expDeleteUpdates := []stingle.DeleteEvent{
		stingle.DeleteEvent{
			File: "", AlbumID: "my-album", Type: "4", Date: "10000",
		},
		stingle.DeleteEvent{
			File: fmt.Sprintf("%d", user.UserID), AlbumID: "", Type: "6", Date: "10000",
		},
	}
	if diff := deep.Equal(expDeleteUpdates, deleteUpdates); diff != nil {
		t.Errorf("Bob's delete updates have unexpected value: %v", diff)

	}
}
