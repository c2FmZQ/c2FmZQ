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
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"math/big"
	"os"
	"path"
	"path/filepath"
	"sort"

	"c2FmZQ/internal/log"
	"c2FmZQ/internal/stingle"
	"c2FmZQ/internal/stingle/token"
)

const (
	userListFile    = "users.dat"
	userFile        = "user.dat"
	contactListFile = "contact-list.dat"
)

// This is used internally for the list of all users in the system.
type userList struct {
	UserID int64  `json:"userId"`
	Email  string `json:"email"`
	Admin  bool   `json:"admin,omitempty"`
}

// Encapsulates all the information about a user account.
type User struct {
	// Whether login with this account is disabled.
	LoginDisabled bool `json:"loginDisabled"`
	// Whether this user account needs to be approved. Accounts that need
	// approval can't upload or share files.
	NeedApproval bool `json:"needApproval"`
	// Whether this user is an administrator of the system.
	Admin bool `json:"admin"`
	// The unique user ID of the user.
	UserID int64 `json:"userId"`
	// The unique email address of the user.
	Email string `json:"email"`
	// A hash of the user's password.
	HashedPassword string `json:"hashedPassword"`
	// The salt used by the user to create the password.
	Salt string `json:"salt"`
	// The user's home folder on the app. Not used by the server.
	HomeFolder string `json:"homeFolder"`
	// The user's key bundle. It contains the user's public key, and
	// optionally, their encrypted secret key.
	KeyBundle string `json:"keyBundle"`
	// Whether KeyBundle contains the encrypted secret key.
	IsBackup string `json:"isBackup"`
	// The server's secret key used with this user, encrypted with master key.
	ServerSecretKey string `json:"serverSecretKey"`
	// The server's public key used with this user.
	ServerPublicKey stingle.PublicKey `json:"serverPublicKey"`
	// The user's public key, extracted from the key bundle.
	PublicKey stingle.PublicKey `json:"publicKey"`
	// The server's secret key used for encrypting tokens for this user,
	// encrypted with master key.
	TokenKey string `json:"serverTokenKey"`
	// A set of valid tokens. Each Login adds a token. Each logout remove one.
	ValidTokens map[string]bool `json:"validTokens"`
	// The OTP key for this user.
	OTPKey string `json:"otpKey,omitempty"`
	// Decoy accounts that the user can access with different passwords.
	Decoys []*Decoy `json:"decoys,omitempty"`
}

// A decoy account's information.
type Decoy struct {
	// The UserID of the decoy account.
	UserID int64 `json:"userId"`
	// The encrypted password.
	Password string `json:"password"`
}

// A user's contact list information.
type ContactList struct {
	// All the user's contacts, keyed by UserID.
	Contacts map[int64]*Contact `json:"contacts"`
	// All users who have this user in their contact list.
	In map[int64]bool `json:"in"`
	// Delete events for contacts.
	Deletes []DeleteEvent `json:"deletes"`
	// The timestamp before which DeleteEvents were pruned.
	DeleteHorizon int64 `json:"deleteHorizon,omitempty"`
}

// Encapsulates the information about a user's contact (another user).
type Contact struct {
	// The contact's UserID.
	UserID int64 `json:"userId"`
	// The contact's email address.
	Email string `json:"email"`
	// The contact's public key.
	PublicKey string `json:"publicKey"`
	// ?
	DateUsed int64 `json:"dateUsed,omitempty"`
	// The time when the contact was added or modified.
	DateModified int64 `json:"dateModified,omitempty"`
}

// ServerPublicKeyForExport returns the server's public key associated with this
// user.
func (u User) ServerPublicKeyForExport() string {
	defer recordLatency("ServerPublicKeyForExport")()

	return base64.StdEncoding.EncodeToString(u.ServerPublicKey.ToBytes())
}

func (u User) home(elems ...string) string {
	return homeByUserID(u.UserID, elems...)
}

func homeByUserID(userID int64, elems ...string) string {
	e := []string{"home", fmt.Sprintf("%d", userID)}
	e = append(e, elems...)
	return path.Join(e...)
}

// AddUser creates a new user account for u.
func (d *Database) AddUser(u User) (userID int64, retErr error) {
	defer recordLatency("AddUser")()

	var ul []userList
	commit, err := d.storage.OpenForUpdate(d.filePath(userListFile), &ul)
	if err != nil {
		log.Errorf("d.storage.OpenForUpdate: %v", err)
		return 0, err
	}
	defer commit(false, &retErr)
	uids := make(map[int64]bool)
	for _, i := range ul {
		if i.Email == u.Email {
			return 0, os.ErrExist
		}
		uids[i.UserID] = true
	}

	var uid int64
	for {
		bi, err := rand.Int(rand.Reader, big.NewInt(int64(math.MaxInt32-1000000)))
		if err != nil {
			return 0, err
		}
		if uid = bi.Int64() + 1000000; !uids[uid] {
			break
		}
	}
	if len(ul) == 0 {
		// First user is an admin.
		u.Admin = true
	}
	ul = append(ul, userList{UserID: uid, Email: u.Email, Admin: u.Admin})

	u.UserID = uid
	hf := make([]byte, 16)
	if _, err := rand.Read(hf); err != nil {
		return 0, err
	}
	u.HomeFolder = hex.EncodeToString(hf)
	ssk := stingle.MakeSecretKey()
	defer ssk.Wipe()
	essk, err := d.EncryptSecretKey(ssk)
	if err != nil {
		return 0, err
	}
	u.ServerSecretKey = essk
	u.ServerPublicKey = ssk.PublicKey()
	tk := token.MakeKey()
	defer tk.Wipe()
	if u.TokenKey, err = d.EncryptTokenKey(tk); err != nil {
		return 0, err
	}
	if err := d.storage.SaveDataFile(d.filePath(u.home(userFile)), u); err != nil {
		return 0, err
	}

	if err := d.storage.CreateEmptyFile(d.fileSetPath(u, stingle.TrashSet), FileSet{}); err != nil {
		return 0, err
	}
	if err := d.storage.CreateEmptyFile(d.fileSetPath(u, stingle.GallerySet), FileSet{}); err != nil {
		return 0, err
	}
	if err := d.storage.CreateEmptyFile(d.filePath(u.home(albumManifest)), AlbumManifest{}); err != nil {
		return 0, err
	}
	if err := d.storage.CreateEmptyFile(d.filePath(u.home(contactListFile)), ContactList{}); err != nil {
		return 0, err
	}
	return u.UserID, commit(true, nil)
}

// UpdateUser adds or updates a user object.
func (d *Database) UpdateUser(u User) error {
	defer recordLatency("UpdateUser")()

	var f User
	commit, err := d.storage.OpenForUpdate(d.filePath(u.home(userFile)), &f)
	if err != nil {
		return err
	}
	f = u
	return commit(true, nil)
}

// ApproveUser approves a new user account.
func (d *Database) ApproveUser(id int64) error {
	defer recordLatency("ApproveUser")()

	var u User
	commit, err := d.storage.OpenForUpdate(d.filePath(homeByUserID(id, userFile)), &u)
	if err != nil {
		return err
	}
	u.NeedApproval = false
	return commit(true, nil)
}

// RenameUser changes a user's email address.
func (d *Database) RenameUser(id int64, newEmail string) (retErr error) {
	defer recordLatency("RenameUser")()
	if newEmail == "" {
		return fs.ErrInvalid
	}

	files := []string{
		d.filePath(userListFile),
		d.filePath(homeByUserID(id, userFile)),
	}
	var ul []userList
	var u User
	objects := []interface{}{&ul, &u}

	var cl ContactList
	if err := d.storage.ReadDataFile(d.filePath(homeByUserID(id, contactListFile)), &cl); err != nil {
		return err
	}
	var contactlists []*ContactList
	for cid := range cl.In {
		files = append(files, d.filePath(homeByUserID(cid, contactListFile)))
		t := &ContactList{}
		contactlists = append(contactlists, t)
		objects = append(objects, t)
	}

	commit, err := d.storage.OpenManyForUpdate(files, objects)
	if err != nil {
		log.Errorf("d.storage.OpenManyForUpdate: %v", err)
		return err
	}
	defer commit(false, &retErr)
	for _, u := range ul {
		if u.Email == newEmail {
			return fs.ErrExist
		}
	}
	for _, cl := range contactlists {
		if c, ok := cl.Contacts[id]; ok {
			c.Email = newEmail
			c.DateModified = nowInMS()
		}
	}
	for i := range ul {
		if ul[i].UserID == id {
			ul[i].Email = newEmail
			u.Email = newEmail
			return commit(true, nil)
		}
	}
	return os.ErrNotExist
}

// UserByID returns the User object with the given ID.
func (d *Database) UserByID(id int64) (User, error) {
	defer recordLatency("UserByID")()

	var u User
	err := d.storage.ReadDataFile(d.filePath("home", fmt.Sprintf("%d", id), userFile), &u)
	if u.ValidTokens == nil {
		u.ValidTokens = make(map[string]bool)
	}
	return u, err
}

// User returns the User object with the given email address.
func (d *Database) User(email string) (User, error) {
	defer recordLatency("User")()

	var ul []userList
	if err := d.storage.ReadDataFile(d.filePath(userListFile), &ul); err != nil {
		return User{}, err
	}
	for _, u := range ul {
		if u.Email == email {
			return d.UserByID(u.UserID)
		}
	}
	return User{}, os.ErrNotExist
}

// NewEncryptedTokenKey returns a new encrypted TokenKey.
func (d *Database) NewEncryptedTokenKey() (string, error) {
	tk := token.MakeKey()
	defer tk.Wipe()
	return d.EncryptTokenKey(tk)
}

// EncryptTokenKey encrypts a TokenKey.
func (d *Database) EncryptTokenKey(key *token.Key) (string, error) {
	return d.Encrypt(key[:])
}

// DecryptTokenKey decrypts an encrypted TokenKey.
func (d *Database) DecryptTokenKey(key string) (*token.Key, error) {
	k, err := d.Decrypt(key)
	if err != nil {
		return nil, err
	}
	return token.KeyFromBytes(k), nil
}

// EncryptSecretKey encrypts a SecretKey.
func (d *Database) EncryptSecretKey(sk *stingle.SecretKey) (string, error) {
	return d.Encrypt(sk.ToBytes())
}

// DecryptSecretKey decrypts an encrypted SecretKey.
func (d *Database) DecryptSecretKey(key string) (*stingle.SecretKey, error) {
	k, err := d.Decrypt(key)
	if err != nil {
		return nil, err
	}
	return stingle.SecretKeyFromBytes(k), nil
}

// TokenKeyForUser returns the server's TokenKey associated with this user.
func (d *Database) TokenKeyForUser(email string) (*token.Key, error) {
	defer recordLatency("TokenKeyForUser")()

	u, err := d.User(email)
	if err != nil {
		return nil, err
	}
	return d.DecryptTokenKey(u.TokenKey)
}

// DeleteUser deletes a user object and all resources attached to it.
func (d *Database) DeleteUser(u User) error {
	defer recordLatency("DeleteUser")()

	var ul []userList
	commit, err := d.storage.OpenForUpdate(d.filePath(userListFile), &ul)
	if err != nil {
		log.Errorf("d.storage.OpenForUpdate: %v", err)
		return err
	}
	for i := range ul {
		if ul[i].UserID == u.UserID {
			ul[i] = ul[len(ul)-1]
			ul = ul[:len(ul)-1]
			break
		}
	}
	if err := commit(true, nil); err != nil {
		return err
	}
	if err := d.removeAllContacts(u); err != nil {
		return err
	}

	albumRefs, err := d.AlbumRefs(u)
	if err != nil {
		return err
	}

	for albumID := range albumRefs {
		album, err := d.Album(u, albumID)
		if err != nil {
			return err
		}
		if album.OwnerID == u.UserID {
			if err := d.DeleteAlbum(u, albumID); err != nil {
				return err
			}
			continue
		}
		if err := d.RemoveAlbumMember(u, albumID, u.UserID); err != nil {
			return err
		}
	}
	commit, filesets, err := d.fileSetsForUpdate(u, []string{stingle.GallerySet, stingle.TrashSet}, []string{"", ""})
	if err != nil {
		return err
	}
	for _, fs := range filesets {
		for _, f := range fs.Files {
			d.incRefCount(f.StoreFile, -1)
			d.incRefCount(f.StoreThumb, -1)
		}
	}
	if err := commit(true, nil); err != nil {
		return err
	}
	for _, f := range []string{
		d.filePath(u.home(userFile)),
		d.fileSetPath(u, stingle.TrashSet),
		d.fileSetPath(u, stingle.GallerySet),
		d.filePath(u.home(albumManifest)),
		d.filePath(u.home(contactListFile)),
	} {
		if err := os.Remove(filepath.Join(d.Dir(), f)); err != nil {
			return err
		}
	}
	return nil
}

// Export converts a Contact to stingle.Contact.
func (c Contact) Export() stingle.Contact {
	return stingle.Contact{
		UserID:       number(c.UserID),
		Email:        c.Email,
		PublicKey:    c.PublicKey,
		DateUsed:     number(c.DateUsed),
		DateModified: number(c.DateModified),
	}
}

// addContactToUser adds contact to user's contact list.
func (d *Database) addContactToUser(user, contact User) (c *Contact, retErr error) {
	var (
		userContacts    ContactList
		contactContacts ContactList
	)
	files := []string{
		d.filePath(user.home(contactListFile)),
		d.filePath(contact.home(contactListFile)),
	}
	contactLists := []*ContactList{
		&userContacts,
		&contactContacts,
	}
	commit, err := d.storage.OpenManyForUpdate(files, contactLists)
	if err != nil {
		log.Errorf("d.storage.OpenManyForUpdate: %v", err)
		return nil, err
	}
	defer commit(true, &retErr)

	if userContacts.Contacts == nil {
		userContacts.Contacts = make(map[int64]*Contact)
	}
	userContacts.Contacts[contact.UserID] = &Contact{
		UserID:       contact.UserID,
		Email:        contact.Email,
		PublicKey:    base64.StdEncoding.EncodeToString(contact.PublicKey.ToBytes()),
		DateModified: nowInMS(),
	}
	if contactContacts.In == nil {
		contactContacts.In = make(map[int64]bool)
	}
	contactContacts.In[user.UserID] = true

	pruneDeleteEvents(&contactLists[0].Deletes, &contactLists[0].DeleteHorizon)
	pruneDeleteEvents(&contactLists[1].Deletes, &contactLists[1].DeleteHorizon)
	return userContacts.Contacts[contact.UserID], nil
}

// AddContact adds the user with the given email address to user's contact list.
func (d *Database) AddContact(user User, contactEmail string) (*Contact, error) {
	defer recordLatency("AddContact")()

	c, err := d.User(contactEmail)
	if err != nil {
		return nil, err
	}
	if c.NeedApproval {
		return nil, errors.New("account is not approved yet")
	}
	return d.addContactToUser(user, c)
}

// removeAllContacts removes all contacts.
func (d *Database) removeAllContacts(user User) (retErr error) {
	var contacts ContactList
	if err := d.storage.ReadDataFile(d.filePath(user.home(contactListFile)), &contacts); err != nil {
		return err
	}
	uids := make(map[int64]struct{})
	uids[user.UserID] = struct{}{}
	for uid := range contacts.Contacts {
		uids[uid] = struct{}{}
	}
	for uid := range contacts.In {
		uids[uid] = struct{}{}
	}
	uidSlice := make([]int64, 0, len(uids))
	fileSlice := make([]string, 0, len(uids))
	contactListSlice := make([]*ContactList, 0, len(uids))
	for uid := range uids {
		uidSlice = append(uidSlice, uid)
		fileSlice = append(fileSlice, d.filePath(homeByUserID(uid, contactListFile)))
		contactListSlice = append(contactListSlice, new(ContactList))
	}
	commit, err := d.storage.OpenManyForUpdate(fileSlice, contactListSlice)
	if err != nil {
		log.Errorf("d.storage.OpenManyForUpdate: %v", err)
		return err
	}
	defer commit(false, &retErr)
	uc := make(map[int64]*ContactList)
	for i, uid := range uidSlice {
		uc[uid] = contactListSlice[i]
	}
	for uid, cl := range uc {
		// Remove user from contact's list.
		delete(uc[user.UserID].In, uid)
		delete(cl.Contacts, user.UserID)
		cl.Deletes = append(cl.Deletes, DeleteEvent{
			File: fmt.Sprintf("%d", user.UserID),
			Date: nowInMS(),
			Type: stingle.DeleteEventContact,
		})
		// Remove contact from user's list.
		delete(uc[uid].In, user.UserID)
		delete(uc[uid].Contacts, uid)
		uc[user.UserID].Deletes = append(uc[user.UserID].Deletes, DeleteEvent{
			File: fmt.Sprintf("%d", uid),
			Date: nowInMS(),
			Type: stingle.DeleteEventContact,
		})
	}
	for i := range contactListSlice {
		pruneDeleteEvents(&contactListSlice[i].Deletes, &contactListSlice[i].DeleteHorizon)
	}
	return commit(true, nil)
}

// lookupContacts returns a Contact for each UserIDs in the list.
func (d *Database) lookupContacts(uids map[int64]bool) []Contact {
	var ul []userList
	if err := d.storage.ReadDataFile(d.filePath(userListFile), &ul); err != nil {
		return nil
	}
	var out []Contact
	for _, u := range ul {
		if uids[u.UserID] {
			user, err := d.UserByID(u.UserID)
			if err != nil {
				log.Errorf("d.UserByID(%q) failed, but user in %q: %v", u.UserID, userListFile, err)
				continue
			}
			out = append(out, Contact{
				UserID:    user.UserID,
				Email:     user.Email,
				PublicKey: base64.StdEncoding.EncodeToString(user.PublicKey.ToBytes()),
			})
		}
	}
	return out
}

// addCrossContacts adds contacts to each other.
func (d *Database) addCrossContacts(list []Contact) {
	files := make([]string, len(list))
	contactLists := make([]*ContactList, len(list))
	for i, c := range list {
		files[i] = d.filePath("home", fmt.Sprintf("%d", c.UserID), contactListFile)
		contactLists[i] = &ContactList{}
	}
	commit, err := d.storage.OpenManyForUpdate(files, contactLists)
	if err != nil {
		log.Errorf("d.storage.OpenManyForUpdate: %v", err)
		return
	}
	count := 0
	for i, c1 := range list {
		contactList := contactLists[i]
		if contactList.Contacts == nil {
			contactList.Contacts = make(map[int64]*Contact)
		}
		if contactList.In == nil {
			contactList.In = make(map[int64]bool)
		}
		for _, c2 := range list {
			if c1.UserID == c2.UserID {
				continue
			}
			if contactList.Contacts[c2.UserID] == nil {
				count++
				c := c2
				c.DateModified = nowInMS()
				contactList.Contacts[c2.UserID] = &c
			}
			contactList.In[c2.UserID] = true
		}
	}
	if err := commit(true, nil); err != nil {
		log.Errorf("Failed to save user contact lists: %v", err)
	} else {
		log.Debugf("Added %d contact(s) to %d user(s)", count, len(list))
	}
}

// ContactUpdates returns changes to a user's contact list that are more recent
// than ts.
func (d *Database) ContactUpdates(user User, ts int64) ([]stingle.Contact, error) {
	defer recordLatency("ContactUpdates")()

	var contactList ContactList
	if err := d.storage.ReadDataFile(d.filePath(user.home(contactListFile)), &contactList); err != nil {
		return nil, err
	}
	if contactList.Contacts == nil {
		contactList.Contacts = make(map[int64]*Contact)
	}
	out := []stingle.Contact{}
	for _, v := range contactList.Contacts {
		if v.DateModified > ts {
			sc := stingle.Contact{
				UserID:       number(v.UserID),
				Email:        v.Email,
				PublicKey:    v.PublicKey,
				DateModified: number(v.DateModified),
			}

			out = append(out, sc)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].DateModified == out[j].DateModified {
			return out[i].Email < out[j].Email
		}
		return out[i].DateModified < out[j].DateModified
	})
	return out, nil
}
