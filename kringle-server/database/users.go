package database

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"sort"

	"kringle-server/log"
	"kringle-server/stingle"
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
}

// Encapsulates all the information about a user account.
type User struct {
	UserID         int64                 `json:"userId"`
	Email          string                `json:"email"`
	HashedPassword string                `json:"hashedPassword"`
	Salt           string                `json:"salt"`
	HomeFolder     string                `json:"homeFolder"`
	KeyBundle      string                `json:"keyBundle"`
	IsBackup       string                `json:"isBackup"`
	ServerKey      stingle.SecretKey     `json:"serverKey"`
	ServerSignKey  stingle.SignSecretKey `json:"serverSignKey"`
	PublicKey      stingle.PublicKey     `json:"publicKey"`
	TokenSeq       int                   `json:"tokenSeq"`
}

// A user's contact list information.
type ContactList struct {
	Contacts map[int64]*Contact `json:"contacts"`
	In       map[int64]bool     `json:"in"`
	Deletes  []DeleteEvent      `json:"deletes"`
}

// Encapsulates the information about a user's contact (another user).
type Contact struct {
	UserID       int64  `json:"userId"`
	Email        string `json:"email"`
	PublicKey    string `json:"publicKey"`
	DateUsed     int64  `json:"dateUsed,omitempty"`
	DateModified int64  `json:"dateModified,omitempty"`
}

// ServerPublicKeyForExport returns the server's public key associated with this
// user.
func (u User) ServerPublicKeyForExport() string {
	return base64.StdEncoding.EncodeToString(u.ServerKey.PublicKey().Bytes)
}

func (u User) home(elems ...string) string {
	e := []string{"home", fmt.Sprintf("%d", u.UserID)}
	e = append(e, elems...)
	return filepath.Join(e...)
}

// AddUser creates a new user account for u.
func (d *Database) AddUser(u User) (retErr error) {
	var ul []userList
	commit, err := d.md.OpenForUpdate(d.filePath(userListFile), &ul)
	if err != nil {
		log.Errorf("d.md.OpenForUpdate: %v", err)
		return err
	}
	defer commit(true, &retErr)
	uids := make(map[int64]bool)
	for _, i := range ul {
		if i.Email == u.Email {
			return os.ErrExist
		}
		uids[i.UserID] = true
	}

	var uid int64
	for {
		bi, err := rand.Int(rand.Reader, big.NewInt(int64(math.MaxInt32-1000000)))
		if err != nil {
			commit(false, nil)
			return err
		}
		if uid = bi.Int64() + 1000000; !uids[uid] {
			break
		}
	}
	ul = append(ul, userList{UserID: uid, Email: u.Email})

	hf := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, hf); err != nil {
		return err
	}

	u.UserID = uid
	u.HomeFolder = hex.EncodeToString(hf)
	u.ServerKey = stingle.MakeSecretKey()
	u.ServerSignKey = stingle.MakeSignSecretKey()
	u.TokenSeq = 1
	return d.md.SaveDataFile(nil, d.filePath(u.home(userFile)), u)
}

// UpdateUser saves a user object.
func (d *Database) UpdateUser(u User) error {
	var f User
	commit, err := d.md.OpenForUpdate(d.filePath(u.home(userFile)), &f)
	if err != nil {
		return err
	}
	f = u
	return commit(true, nil)
}

// UserByID returns the User object with the given ID.
func (d *Database) UserByID(id int64) (User, error) {
	var u User
	_, err := d.md.ReadDataFile(d.filePath("home", fmt.Sprintf("%d", id), userFile), &u)
	return u, err
}

// User returns the User object with the given email address.
func (d *Database) User(email string) (User, error) {
	var ul []userList
	if _, err := d.md.ReadDataFile(d.filePath(userListFile), &ul); err != nil {
		return User{}, err
	}
	for _, u := range ul {
		if u.Email == email {
			return d.UserByID(u.UserID)
		}
	}
	return User{}, os.ErrNotExist
}

// SignKeyForUser returns the server's SignSecretKey associated with this user.
func (d *Database) SignKeyForUser(email string) stingle.SignSecretKey {
	if u, err := d.User(email); err == nil {
		return u.ServerSignKey
	}
	return stingle.SignSecretKey{}
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
	objects := []interface{}{
		&userContacts,
		&contactContacts,
	}
	commit, err := d.md.OpenManyForUpdate(files, objects)
	if err != nil {
		log.Errorf("d.md.OpenManyForUpdate: %v", err)
		return nil, err
	}
	defer commit(true, &retErr)

	if userContacts.Contacts == nil {
		userContacts.Contacts = make(map[int64]*Contact)
	}
	userContacts.Contacts[contact.UserID] = &Contact{
		UserID:       contact.UserID,
		Email:        contact.Email,
		PublicKey:    base64.StdEncoding.EncodeToString(contact.PublicKey.Bytes),
		DateModified: nowInMS(),
	}
	if contactContacts.In == nil {
		contactContacts.In = make(map[int64]bool)
	}
	contactContacts.In[user.UserID] = true

	return userContacts.Contacts[contact.UserID], nil
}

// AddContact adds the user with the given email address to user's contact list.
func (d *Database) AddContact(user User, contactEmail string) (*Contact, error) {
	c, err := d.User(contactEmail)
	if err != nil {
		return nil, err
	}
	return d.addContactToUser(user, c)
}

// lookupContacts returns a Contact for each UserIDs in the list.
func (d *Database) lookupContacts(uids map[int64]bool) []Contact {
	var ul []userList
	if _, err := d.md.ReadDataFile(d.filePath(userListFile), &ul); err != nil {
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
				PublicKey: base64.StdEncoding.EncodeToString(user.PublicKey.Bytes),
			})
		}
	}
	return out
}

// addCrossContacts adds contacts to each other.
func (d *Database) addCrossContacts(list []Contact) {
	files := make([]string, len(list))
	contactLists := make([]ContactList, len(list))
	objects := make([]interface{}, len(list))
	for i, c := range list {
		files[i] = d.filePath("home", fmt.Sprintf("%d", c.UserID), contactListFile)
		objects[i] = &contactLists[i]
	}
	commit, err := d.md.OpenManyForUpdate(files, objects)
	if err != nil {
		log.Errorf("d.md.OpenManyForUpdate: %v", err)
		return
	}
	count := 0
	for i, c1 := range list {
		contactList := &contactLists[i]
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
	var contactList ContactList
	if _, err := d.md.ReadDataFile(d.filePath(user.home(contactListFile)), &contactList); err != nil && !errors.Is(err, os.ErrNotExist) {
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
