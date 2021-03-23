package database

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"stingle-server/log"
)

const (
	albumManifest = "album-manifest.json"
)

// Encapsulates a list of all of a user's albums, including albums shared with
// them.
type AlbumManifest struct {
	Albums  map[string]*AlbumRef `json:"albums"`
	Deletes []DeleteEvent        `json:"deletes"`
}

// Contains a reference to an album, i.e. its ID and where it is stored.
type AlbumRef struct {
	AlbumID string `json:"albumId"`
	File    string `json:"file"`
}

// Encapsulates all the information we know about an album.
type AlbumSpec struct {
	// The ID of the user account that owns the album.
	OwnerID int `json:"ownerId"`
	// The ID of the album.
	AlbumID string `json:"albumId"`
	// The time at which the album was created.
	DateCreated int64 `json:"dateCreated"`
	// The time at which the album was last modified.
	DateModified int64 `json:"dateModified"`
	// The private key of the album, encrypted for the owner.
	EncPrivateKey string `json:"encPrivateKey"`
	// Encrypted metadata, e.g. album name.
	Metadata string `json:"metadata"`
	// The public key of the album.
	PublicKey string `json:"publicKey"`
	// Whether the album is shared.
	IsShared bool `json:"isShared"`
	// Whether the album is hidden.
	IsHidden bool `json:"isHidden"`
	// The album's permissions settings.
	Permissions SharingPermissions `json:"permissions"`
	// Whether the album is locked (?)
	IsLocked bool `json:"isLocked"`
	// The file to use as album cover.
	Cover string `json:"cover"`
	// The set of members: key is member ID, value is always true.
	Members map[int]bool `json:"members"`
	// ?
	SyncLocal bool `json:"syncLocal"`
	// The private key of the album, encrypted for each member.
	SharingKeys map[int]string `json:"sharingKeys"`
}

// Permissions settings that control what album members can do.
type SharingPermissions string

func (p SharingPermissions) AllowAdd() bool   { return len(p) == 4 && p[0] == '1' && p[1] == '1' }
func (p SharingPermissions) AllowShare() bool { return len(p) == 4 && p[0] == '1' && p[2] == '1' }
func (p SharingPermissions) AllowCopy() bool  { return len(p) == 4 && p[0] == '1' && p[3] == '1' }

// The Stingle API representation of an album.
type StingleAlbum struct {
	AlbumID       string            `json:"albumId"`
	DateCreated   string            `json:"dateCreated"`
	DateModified  string            `json:"dateModified"`
	EncPrivateKey string            `json:"encPrivateKey"`
	Metadata      string            `json:"metadata"`
	PublicKey     string            `json:"publicKey"`
	IsShared      string            `json:"isShared"`
	IsHidden      string            `json:"isHidden"`
	IsOwner       string            `json:"isOwner"`
	Permissions   string            `json:"permissions"`
	IsLocked      string            `json:"isLocked"`
	Cover         string            `json:"cover"`
	Members       string            `json:"members"`
	SyncLocal     string            `json:"syncLocal,omitempty"`
	SharingKeys   map[string]string `json:"sharingKeys,omitempty"`
}

// Album returns a user's album information.
func (d *Database) Album(user User, albumID string) (*AlbumSpec, error) {
	fs, err := d.FileSet(user, AlbumSet, albumID)
	if err != nil {
		return nil, err
	}
	return fs.Album, nil
}

// makeAlbumPath returns a new random path for an album.
func (d *Database) makeAlbumPath() (string, error) {
	name := make([]byte, 32)
	if _, err := rand.Read(name); err != nil {
		return "", err
	}
	dir := filepath.Join(d.Dir(), "albums", fmt.Sprintf("%02X", name[0]), fmt.Sprintf("%02X", name[1]))
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	return filepath.Join(dir, base64.RawURLEncoding.EncodeToString(name)+".json"), nil
}

// AddAlbum creates a new empty album with the given information.
func (d *Database) AddAlbum(owner User, album AlbumSpec) (retErr error) {
	ap, err := d.makeAlbumPath()
	if err != nil {
		log.Errorf("makeAlbumPath() failed: %v", err)
		return err
	}
	if err := d.addAlbumRef(owner.UserID, album.AlbumID, ap); err != nil {
		return err
	}
	done, fs, err := d.fileSetForUpdate(owner, AlbumSet, album.AlbumID)
	if err != nil {
		return err
	}
	album.OwnerID = owner.UserID
	if album.Members == nil {
		album.Members = make(map[int]bool)
	}
	if album.SharingKeys == nil {
		album.SharingKeys = make(map[int]string)
	}
	fs.Album = &album
	return done(&retErr)
}

// DeleteAlbum deletes an album.
func (d *Database) DeleteAlbum(owner User, albumID string) error {
	albumRef, err := d.albumRef(owner, albumID)
	if err != nil {
		return err
	}
	fs, err := d.FileSet(owner, AlbumSet, albumID)
	if err != nil {
		return err
	}
	if err := lock(albumRef.File); err != nil {
		return err
	}
	defer unlock(albumRef.File)
	if err := os.Remove(albumRef.File); err != nil {
		log.Errorf("os.Remove(%q) failed: %v", albumRef.File, err)
	}

	if err := d.removeAlbumRef(owner.UserID, albumID); err != nil {
		return err
	}
	for m, _ := range fs.Album.Members {
		if err := d.removeAlbumRef(m, albumID); err != nil {
			log.Errorf("removeAlbumRef(%d, %q failed: %v", m, albumID, err)
		}
	}
	for _, f := range fs.Files {
		d.incRefCount(f.StoreFile, -1)
		d.incRefCount(f.StoreThumb, -1)
	}
	return nil
}

// ChangeAlbumCover changes the file uses as cover for the album.
func (d *Database) ChangeAlbumCover(user User, albumID, cover string) (retErr error) {
	done, fs, err := d.fileSetForUpdate(user, AlbumSet, albumID)
	if err != nil {
		return err
	}
	defer done(&retErr)

	fs.Album.Cover = cover
	fs.Album.DateModified = nowInMS()
	return nil
}

// ChangeMetadata updates the album's metadata.
func (d *Database) ChangeMetadata(user User, albumID, metadata string) (retErr error) {
	done, fs, err := d.fileSetForUpdate(user, AlbumSet, albumID)
	if err != nil {
		return err
	}
	defer done(&retErr)

	fs.Album.Metadata = metadata
	fs.Album.DateModified = nowInMS()
	return nil
}

// AlbumPermissions returns the album's permissions.
func (d *Database) AlbumPermissions(user User, albumID string) (SharingPermissions, error) {
	fs, err := d.FileSet(user, AlbumSet, albumID)
	if err != nil {
		return SharingPermissions(""), err
	}
	return fs.Album.Permissions, nil
}

// AlbumRefs returns a list of all the user's albums.
func (d *Database) AlbumRefs(user User) (map[string]*AlbumRef, error) {
	var manifest AlbumManifest
	if err := loadJSON(filepath.Join(d.Home(user.Email), albumManifest), &manifest); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if manifest.Albums == nil {
		manifest.Albums = make(map[string]*AlbumRef)
	}
	return manifest.Albums, nil
}

// convertAlbumSpecToStingleAlbum converts a AlbumSpec to StingleAlbum.
func convertAlbumSpecToStingleAlbum(album *AlbumSpec) StingleAlbum {
	members := []string{}
	for k, _ := range album.Members {
		members = append(members, fmt.Sprintf("%d", k))
	}
	sort.Strings(members)
	return StingleAlbum{
		AlbumID:       album.AlbumID,
		DateCreated:   fmt.Sprintf("%d", album.DateCreated),
		DateModified:  fmt.Sprintf("%d", album.DateModified),
		EncPrivateKey: album.EncPrivateKey,
		Metadata:      album.Metadata,
		PublicKey:     album.PublicKey,
		IsShared:      boolToString(album.IsShared),
		IsHidden:      boolToString(album.IsHidden),
		IsOwner:       "1",
		Permissions:   string(album.Permissions),
		IsLocked:      boolToString(album.IsLocked),
		Cover:         album.Cover,
		Members:       strings.Join(members, ","),
	}
}

// AlbumUpdates returns all the changes to the user's album list since ts.
func (d *Database) AlbumUpdates(user User, ts int64) ([]StingleAlbum, error) {
	albumRefs, err := d.AlbumRefs(user)
	if err != nil {
		return nil, err
	}
	out := []StingleAlbum{}
	for _, v := range albumRefs {
		fs, err := d.FileSet(user, AlbumSet, v.AlbumID)
		if err != nil {
			log.Errorf("d.FileSet(%q, %q, %q) failed: %v", user.Email, AlbumSet, v.AlbumID, err)
			continue
		}
		if fs.Album.DateModified > ts {
			sa := convertAlbumSpecToStingleAlbum(fs.Album)
			if fs.Album.OwnerID != user.UserID {
				sa.EncPrivateKey = fs.Album.SharingKeys[user.UserID]
				sa.IsOwner = "0"
			}
			out = append(out, sa)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DateModified < out[j].DateModified })

	return out, nil
}

// ShareAlbum turns on sharing on an album and adds members.
func (d *Database) ShareAlbum(user User, sharing *StingleAlbum, sharingKeys map[string]string) (retErr error) {
	albumRef, err := d.albumRef(user, sharing.AlbumID)
	if err != nil {
		return err
	}
	done, fs, err := d.fileSetForUpdate(user, AlbumSet, sharing.AlbumID)
	if err != nil {
		return err
	}
	defer done(&retErr)
	if fs.Album.Members == nil {
		fs.Album.Members = make(map[int]bool)
	}
	if fs.Album.SharingKeys == nil {
		fs.Album.SharingKeys = make(map[int]string)
	}
	if fs.Album.OwnerID != user.UserID && (!fs.Album.IsShared || !fs.Album.Members[user.UserID] || !fs.Album.Permissions.AllowShare()) {
		return fmt.Errorf("user %d is not allowed to share this album", user.UserID)
	}
	if fs.Album.OwnerID == user.UserID {
		fs.Album.IsShared = sharing.IsShared == "1"
		fs.Album.IsHidden = sharing.IsHidden == "1"
		fs.Album.IsLocked = sharing.IsLocked == "1"
		fs.Album.SyncLocal = sharing.SyncLocal == "1"
		fs.Album.Permissions = SharingPermissions(sharing.Permissions)
	}
	for _, m := range strings.Split(sharing.Members, ",") {
		sid, err := strconv.ParseInt(m, 10, 32)
		if err != nil {
			log.Errorf("Invalid members: %q", sharing.Members)
			continue
		}
		id := int(sid)
		if id != fs.Album.OwnerID && sharingKeys[m] == "" {
			log.Errorf("Sharing album with %d but no sharing key", id)
			continue
		}
		fs.Album.Members[id] = true
		if err := d.addAlbumRef(id, fs.Album.AlbumID, albumRef.File); err != nil {
			log.Errorf("addAlbumRef(%d, %q, %q) failed: %v", id, fs.Album.AlbumID, albumRef.File, err)
		}
	}
	for k, v := range sharingKeys {
		sid, err := strconv.ParseInt(k, 10, 32)
		if err != nil {
			log.Errorf("Invalid sharingKeys: %v", sharingKeys)
			continue
		}
		id := int(sid)
		if _, ok := fs.Album.SharingKeys[id]; ok && fs.Album.OwnerID != user.UserID {
			log.Errorf("Non-owner %d trying to overwrite sharing key for %d", user.UserID, id)
			continue
		}
		fs.Album.SharingKeys[id] = v
	}
	fs.Album.DateModified = nowInMS()
	d.addCrossContacts(d.lookupContacts(fs.Album.Members))
	return nil
}

// UnshareAlbum turns off sharing and removes all the members of an album.
func (d *Database) UnshareAlbum(owner User, albumID string) (retErr error) {
	done, fs, err := d.fileSetForUpdate(owner, AlbumSet, albumID)
	if err != nil {
		return err
	}
	defer done(&retErr)

	fs.Album.IsShared = false
	for m, _ := range fs.Album.Members {
		if m == owner.UserID {
			continue
		}
		if err := d.removeAlbumRef(m, albumID); err != nil {
			log.Errorf("removeAlbumRef(%d, %q) failed: %v", m, albumID, err)
		}
	}
	fs.Album.Members = make(map[int]bool)
	fs.Album.SharingKeys = make(map[int]string)
	fs.Album.DateModified = nowInMS()
	return nil
}

// albumRef returns a reference to an album file, i.e. where it is stored.
func (d *Database) albumRef(user User, albumID string) (*AlbumRef, error) {
	home := d.Home(user.Email)
	var manifest AlbumManifest
	if err := loadJSON(filepath.Join(home, albumManifest), &manifest); err != nil {
		return nil, err
	}
	a := manifest.Albums[albumID]
	if a == nil {
		return nil, os.ErrNotExist
	}
	return a, nil
}

// addAlbumRef adds an album reference the a user's album list.
func (d *Database) addAlbumRef(memberID int, albumID, file string) (retErr error) {
	home, err := d.HomeByID(memberID)
	if err != nil {
		return err
	}

	var manifest AlbumManifest
	done, err := openForUpdate(filepath.Join(home, albumManifest), &manifest)
	if err != nil {
		log.Errorf("openForUpdate: %v", err)
		return err
	}
	defer done(&retErr)

	if manifest.Albums == nil {
		manifest.Albums = make(map[string]*AlbumRef)
	}
	manifest.Albums[albumID] = &AlbumRef{
		AlbumID: albumID,
		File:    file,
	}
	return nil
}

// removeAlbumRef removes an album reference from a user's album list.
func (d *Database) removeAlbumRef(memberID int, albumID string) (retErr error) {
	home, err := d.HomeByID(memberID)
	if err != nil {
		return err
	}

	var manifest AlbumManifest
	done, err := openForUpdate(filepath.Join(home, albumManifest), &manifest)
	if err != nil {
		log.Errorf("openForUpdate: %v", err)
		return err
	}
	defer done(&retErr)

	if manifest.Albums == nil {
		return nil
	}
	delete(manifest.Albums, albumID)

	manifest.Deletes = append(manifest.Deletes, DeleteEvent{
		AlbumID: albumID,
		Type:    4,
		Date:    nowInMS(),
	})
	return nil
}

// UpdatePerms updates the permissions on an album.
func (d *Database) UpdatePerms(owner User, albumID string, permissions SharingPermissions) (retErr error) {
	done, fs, err := d.fileSetForUpdate(owner, AlbumSet, albumID)
	if err != nil {
		return err
	}
	defer done(&retErr)
	fs.Album.Permissions = permissions
	fs.Album.DateModified = nowInMS()
	return nil
}

// RemoveAlbumMember removes a member from the album.
func (d *Database) RemoveAlbumMember(owner User, albumID string, memberID int) (retErr error) {
	if owner.UserID == memberID {
		return nil
	}
	done, fs, err := d.fileSetForUpdate(owner, AlbumSet, albumID)
	if err != nil {
		return err
	}
	defer done(&retErr)
	delete(fs.Album.Members, memberID)
	delete(fs.Album.SharingKeys, memberID)
	fs.Album.DateModified = nowInMS()
	return d.removeAlbumRef(memberID, albumID)
}
