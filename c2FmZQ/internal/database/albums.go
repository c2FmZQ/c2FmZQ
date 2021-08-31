package database

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"c2FmZQ/internal/log"
	"c2FmZQ/internal/stingle"
)

const (
	albumManifest = "album-manifest"
)

// Encapsulates a list of all of a user's albums, including albums shared with
// them.
type AlbumManifest struct {
	Albums  map[string]*AlbumRef `json:"albums"`
	Deletes []DeleteEvent        `json:"deletes"`
	// The timestamp before which DeleteEvents were pruned.
	DeleteHorizon int64 `json:"deleteHorizon,omitempty"`
}

// Contains a reference to an album, i.e. its ID and where it is stored.
type AlbumRef struct {
	AlbumID string `json:"albumId"`
	File    string `json:"file"`
}

// Encapsulates all the information we know about an album.
type AlbumSpec struct {
	// The ID of the user account that owns the album.
	OwnerID int64 `json:"ownerId"`
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
	// Whether the album is locked.
	IsLocked bool `json:"isLocked"`
	// The album's permissions settings.
	Permissions stingle.Permissions `json:"permissions"`
	// The file to use as album cover.
	Cover string `json:"cover"`
	// The set of members: key is member ID, value is always true.
	Members map[int64]bool `json:"members"`
	// The private key of the album, encrypted for each member.
	SharingKeys map[int64]string `json:"sharingKeys"`
}

// Album returns a user's album information.
func (d *Database) Album(user User, albumID string) (*AlbumSpec, error) {
	defer recordLatency("Album")()

	fs, err := d.FileSet(user, stingle.AlbumSet, albumID)
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
	dir := filepath.Join("metadata", fmt.Sprintf("%02X", name[0]))
	return filepath.Join(dir, base64.RawURLEncoding.EncodeToString(name)), nil
}

// AddAlbum creates a new empty album with the given information.
func (d *Database) AddAlbum(owner User, album AlbumSpec) (retErr error) {
	defer recordLatency("AddAlbum")()

	ap, err := d.makeAlbumPath()
	if err != nil {
		log.Errorf("makeAlbumPath() failed: %v", err)
		return err
	}
	if err := d.addAlbumRef(owner.UserID, album.AlbumID, ap); err != nil {
		return err
	}
	if err := d.storage.CreateEmptyFile(ap, FileSet{}); err != nil {
		return err
	}
	commit, fs, err := d.fileSetForUpdate(owner, stingle.AlbumSet, album.AlbumID)
	if err != nil {
		return err
	}
	album.OwnerID = owner.UserID
	if album.Members == nil {
		album.Members = make(map[int64]bool)
	}
	if album.SharingKeys == nil {
		album.SharingKeys = make(map[int64]string)
	}
	fs.Album = &album
	return commit(true, &retErr)
}

// DeleteAlbum deletes an album.
func (d *Database) DeleteAlbum(owner User, albumID string) error {
	defer recordLatency("DeleteAlbum")()

	albumRef, err := d.albumRef(owner, albumID)
	if err != nil {
		return err
	}
	fs, err := d.FileSet(owner, stingle.AlbumSet, albumID)
	if err != nil {
		return err
	}
	if err := d.storage.Lock(albumRef.File); err != nil {
		return err
	}
	defer d.storage.Unlock(albumRef.File)
	if err := os.Remove(filepath.Join(d.Dir(), albumRef.File)); err != nil {
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
	defer recordLatency("ChangeAlbumCover")()

	commit, fs, err := d.fileSetForUpdate(user, stingle.AlbumSet, albumID)
	if err != nil {
		return err
	}
	defer commit(true, &retErr)

	fs.Album.Cover = cover
	fs.Album.DateModified = nowInMS()
	return nil
}

// ChangeMetadata updates the album's metadata.
func (d *Database) ChangeMetadata(user User, albumID, metadata string) (retErr error) {
	defer recordLatency("ChangeMetadata")()

	commit, fs, err := d.fileSetForUpdate(user, stingle.AlbumSet, albumID)
	if err != nil {
		return err
	}
	defer commit(true, &retErr)

	fs.Album.Metadata = metadata
	fs.Album.DateModified = nowInMS()
	return nil
}

// AlbumPermissions returns the album's permissions.
func (d *Database) AlbumPermissions(user User, albumID string) (stingle.Permissions, error) {
	defer recordLatency("AlbumPermissions")()

	fs, err := d.FileSet(user, stingle.AlbumSet, albumID)
	if err != nil {
		return stingle.Permissions(""), err
	}
	return fs.Album.Permissions, nil
}

// AlbumRefs returns a list of all the user's albums.
func (d *Database) AlbumRefs(user User) (map[string]*AlbumRef, error) {
	defer recordLatency("AlbumRefs")()

	type cacheValue struct {
		ts int64
		sz int64
		ar map[string]*AlbumRef
	}
	d.albumRefCacheMutex.Lock()
	defer d.albumRefCacheMutex.Unlock()

	fileName := d.filePath(user.home(albumManifest))
	ts, sz := d.stat(fileName)
	if v, ok := d.albumRefCache.Get(fileName); ok {
		if cv := v.(cacheValue); cv.ts == ts && cv.sz == sz {
			log.Debugf("AlbumRef cache hit %s %d %d", fileName, ts, sz)
			return cv.ar, nil
		}
	}
	log.Debugf("AlbumRef cache miss %s %d %d", fileName, ts, sz)

	var manifest AlbumManifest
	if err := d.storage.ReadDataFile(fileName, &manifest); err != nil {
		return nil, err
	}
	if manifest.Albums == nil {
		manifest.Albums = make(map[string]*AlbumRef)
	}
	if ts2, sz2 := d.stat(fileName); ts == ts2 && sz == sz2 {
		d.albumRefCache.Add(fileName, cacheValue{ts, sz, manifest.Albums})
	}
	return manifest.Albums, nil
}

// convertAlbumSpecToStingleAlbum converts a AlbumSpec to stingle.Album.
func convertAlbumSpecToStingleAlbum(album *AlbumSpec) stingle.Album {
	members := []string{}
	for k, _ := range album.Members {
		members = append(members, fmt.Sprintf("%d", k))
	}
	sort.Strings(members)
	return stingle.Album{
		AlbumID:       album.AlbumID,
		DateCreated:   number(album.DateCreated),
		DateModified:  number(album.DateModified),
		EncPrivateKey: album.EncPrivateKey,
		Metadata:      album.Metadata,
		PublicKey:     album.PublicKey,
		IsShared:      boolToNumber(album.IsShared),
		IsHidden:      boolToNumber(album.IsHidden),
		IsOwner:       "1",
		Permissions:   string(album.Permissions),
		IsLocked:      "0",
		Cover:         album.Cover,
		Members:       strings.Join(members, ","),
	}
}

// AlbumUpdates returns all the changes to the user's album list since ts.
func (d *Database) AlbumUpdates(user User, ts int64) ([]stingle.Album, error) {
	defer recordLatency("AlbumUpdates")()

	albumRefs, err := d.AlbumRefs(user)
	if err != nil {
		return nil, err
	}
	out := []stingle.Album{}
	for _, v := range albumRefs {
		fs, err := d.FileSet(user, stingle.AlbumSet, v.AlbumID)
		if err != nil {
			log.Errorf("d.FileSet(%q, %q, %q) failed: %v", user.Email, stingle.AlbumSet, v.AlbumID, err)
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
	sort.Slice(out, func(i, j int) bool {
		if out[i].DateModified == out[j].DateModified {
			return out[i].AlbumID < out[j].AlbumID
		}
		return out[i].DateModified < out[j].DateModified
	})

	return out, nil
}

// ShareAlbum turns on sharing on an album and adds members.
func (d *Database) ShareAlbum(user User, sharing *stingle.Album, sharingKeys map[string]string) (retErr error) {
	defer recordLatency("ShareAlbums")()

	albumRef, err := d.albumRef(user, sharing.AlbumID)
	if err != nil {
		return err
	}
	commit, fs, err := d.fileSetForUpdate(user, stingle.AlbumSet, sharing.AlbumID)
	if err != nil {
		return err
	}
	defer commit(true, &retErr)
	if fs.Album.Members == nil {
		fs.Album.Members = make(map[int64]bool)
	}
	if fs.Album.SharingKeys == nil {
		fs.Album.SharingKeys = make(map[int64]string)
	}
	if fs.Album.OwnerID != user.UserID && (!fs.Album.IsShared || !fs.Album.Members[user.UserID] || !fs.Album.Permissions.AllowShare()) {
		return fmt.Errorf("user %d is not allowed to share this album", user.UserID)
	}
	if fs.Album.OwnerID == user.UserID {
		fs.Album.IsShared = true
		fs.Album.IsHidden = sharing.IsHidden == "1"
		fs.Album.IsLocked = sharing.IsLocked == "1"
		fs.Album.Permissions = stingle.Permissions(sharing.Permissions)
	}
	for _, m := range strings.Split(sharing.Members, ",") {
		id, err := strconv.ParseInt(m, 10, 64)
		if err != nil {
			log.Errorf("Invalid members: %q", sharing.Members)
			continue
		}
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
		id, err := strconv.ParseInt(k, 10, 64)
		if err != nil {
			log.Errorf("Invalid sharingKeys: %v", sharingKeys)
			continue
		}
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
	defer recordLatency("UnshareAlbums")()

	commit, fs, err := d.fileSetForUpdate(owner, stingle.AlbumSet, albumID)
	if err != nil {
		return err
	}
	defer commit(true, &retErr)

	fs.Album.IsShared = false
	for m, _ := range fs.Album.Members {
		if m == owner.UserID {
			continue
		}
		if err := d.removeAlbumRef(m, albumID); err != nil {
			log.Errorf("removeAlbumRef(%d, %q) failed: %v", m, albumID, err)
		}
	}
	fs.Album.Members = make(map[int64]bool)
	fs.Album.SharingKeys = make(map[int64]string)
	fs.Album.DateModified = nowInMS()
	return nil
}

// albumRef returns a reference to an album file, i.e. where it is stored.
func (d *Database) albumRef(user User, albumID string) (*AlbumRef, error) {
	ar, err := d.AlbumRefs(user)
	if err != nil {
		return nil, err
	}
	a := ar[albumID]
	if a == nil {
		return nil, os.ErrNotExist
	}
	return a, nil
}

// addAlbumRef adds an album reference the a user's album list.
func (d *Database) addAlbumRef(memberID int64, albumID, file string) (retErr error) {
	user, err := d.UserByID(memberID)
	if err != nil {
		return err
	}

	var manifest AlbumManifest
	commit, err := d.storage.OpenForUpdate(d.filePath(user.home(albumManifest)), &manifest)
	if err != nil {
		log.Errorf("d.storage.OpenForUpdate: %v", err)
		return err
	}
	defer commit(true, &retErr)

	if manifest.Albums == nil {
		manifest.Albums = make(map[string]*AlbumRef)
	}
	manifest.Albums[albumID] = &AlbumRef{
		AlbumID: albumID,
		File:    file,
	}
	pruneDeleteEvents(&manifest.Deletes, &manifest.DeleteHorizon)
	return nil
}

// removeAlbumRef removes an album reference from a user's album list.
func (d *Database) removeAlbumRef(memberID int64, albumID string) (retErr error) {
	user, err := d.UserByID(memberID)
	if err != nil {
		return err
	}

	var manifest AlbumManifest
	commit, err := d.storage.OpenForUpdate(d.filePath(user.home(albumManifest)), &manifest)
	if err != nil {
		log.Errorf("d.storage.OpenForUpdate: %v", err)
		return err
	}
	defer commit(true, &retErr)

	if manifest.Albums == nil {
		return nil
	}
	delete(manifest.Albums, albumID)

	manifest.Deletes = append(manifest.Deletes, DeleteEvent{
		AlbumID: albumID,
		Type:    stingle.DeleteEventAlbum,
		Date:    nowInMS(),
	})
	pruneDeleteEvents(&manifest.Deletes, &manifest.DeleteHorizon)
	return nil
}

// UpdatePerms updates the permissions on an album.
func (d *Database) UpdatePerms(owner User, albumID string, permissions stingle.Permissions, isHidden, isLocked bool) (retErr error) {
	defer recordLatency("UpdatePerms")()

	commit, fs, err := d.fileSetForUpdate(owner, stingle.AlbumSet, albumID)
	if err != nil {
		return err
	}
	defer commit(true, &retErr)
	fs.Album.Permissions = permissions
	fs.Album.IsHidden = isHidden
	fs.Album.IsLocked = isLocked
	fs.Album.DateModified = nowInMS()
	return nil
}

// RemoveAlbumMember removes a member from the album.
func (d *Database) RemoveAlbumMember(user User, albumID string, memberID int64) (retErr error) {
	defer recordLatency("RemoveAlbumMember")()

	commit, fs, err := d.fileSetForUpdate(user, stingle.AlbumSet, albumID)
	if err != nil {
		return err
	}
	if fs.Album.OwnerID == memberID {
		return nil
	}
	defer commit(true, &retErr)
	delete(fs.Album.Members, memberID)
	delete(fs.Album.SharingKeys, memberID)
	fs.Album.DateModified = nowInMS()
	return d.removeAlbumRef(memberID, albumID)
}
