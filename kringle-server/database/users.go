package database

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"sort"

	"kringle-server/crypto/stinglecrypto"
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
	UserID        int64                       `json:"userId"`
	Email         string                      `json:"email"`
	Password      string                      `json:"password"`
	Salt          string                      `json:"salt"`
	HomeFolder    string                      `json:"homeFolder"`
	KeyBundle     string                      `json:"keyBundle"`
	IsBackup      string                      `json:"isBackup"`
	ServerKey     stinglecrypto.SecretKey     `json:"serverKey"`
	ServerSignKey stinglecrypto.SignSecretKey `json:"serverSignKey"`
	PublicKey     stinglecrypto.PublicKey     `json:"publicKey"`
	TokenSeq      int                         `json:"tokenSeq"`
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

// AddUser creates a new user account for u.
func (d *Database) AddUser(u User) (retErr error) {
	var ul []userList
	commit, err := d.md.OpenForUpdate(d.filePath(userListFile), &ul)
	if err != nil {
		log.Errorf("d.md.OpenForUpdate: %v", err)
		return err
	}
	defer commit(true, &retErr)
	var uid int64
	for _, i := range ul {
		if i.Email == u.Email {
			return os.ErrExist
		}
		if i.UserID > uid {
			uid = i.UserID
		}
	}
	uid += 1
	ul = append(ul, userList{UserID: uid, Email: u.Email})

	hf := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, hf); err != nil {
		return err
	}

	u.UserID = uid
	u.HomeFolder = hex.EncodeToString(hf)
	u.ServerKey = stinglecrypto.MakeSecretKey()
	u.ServerSignKey = stinglecrypto.MakeSignSecretKey()
	u.TokenSeq = 1
	return d.md.SaveDataFile(nil, d.filePath("home", u.Email, userFile), u)
}

// UpdateUser saves a user object.
func (d *Database) UpdateUser(u User) error {
	var f User
	commit, err := d.md.OpenForUpdate(d.filePath("home", u.Email, userFile), &f)
	if err != nil {
		return err
	}
	f = u
	return commit(true, nil)
}

// UserByID returns the User object with the given ID.
func (d *Database) UserByID(id int64) (User, error) {
	var ul []userList
	if _, err := d.md.ReadDataFile(d.filePath(userListFile), &ul); err != nil {
		return User{}, err
	}
	for _, u := range ul {
		if u.UserID == id {
			return d.User(u.Email)
		}
	}
	return User{}, os.ErrNotExist
}

// User returns the User object with the given email address.
func (d *Database) User(email string) (User, error) {
	var u User
	_, err := d.md.ReadDataFile(d.filePath("home", email, userFile), &u)
	return u, err
}

// SignKeyForUser returns the server's SignSecretKey associated with this user.
func (d *Database) SignKeyForUser(email string) stinglecrypto.SignSecretKey {
	if u, err := d.User(email); err == nil {
		return u.ServerSignKey
	}
	return stinglecrypto.SignSecretKey{}
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
	var contactList map[string]*Contact
	commit, err := d.md.OpenForUpdate(d.filePath("home", user.Email, contactListFile), &contactList)
	if err != nil {
		log.Errorf("d.md.OpenForUpdate: %v", err)
		return nil, err
	}
	defer commit(true, &retErr)

	if contactList == nil {
		contactList = make(map[string]*Contact)
	}
	contactList[contact.Email] = &Contact{
		UserID:       contact.UserID,
		Email:        contact.Email,
		PublicKey:    base64.StdEncoding.EncodeToString(contact.PublicKey.Bytes),
		DateModified: nowInMS(),
	}
	return contactList[contact.Email], nil
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
			user, err := d.User(u.Email)
			if err != nil {
				log.Errorf("d.User(%q) failed, but user in %q: %v", u.Email, userListFile, err)
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
	for _, c1 := range list {
		var contactList map[string]*Contact
		commit, err := d.md.OpenForUpdate(d.filePath("home", c1.Email, contactListFile), &contactList)
		if err != nil {
			log.Errorf("d.md.OpenForUpdate: %v", err)
			continue
		}
		if contactList == nil {
			contactList = make(map[string]*Contact)
		}
		count := 0
		for _, c2 := range list {
			if c1.UserID == c2.UserID {
				continue
			}
			if contactList[c2.Email] == nil {
				count++
				c := c2
				c.DateModified = nowInMS()
				contactList[c2.Email] = &c
			}
		}
		if err := commit(true, nil); err != nil {
			log.Errorf("Failed to save user %d's contact list: %v", c1.UserID, err)
		} else {
			log.Debugf("Added %d contact(s) to user %d", count, c1.UserID)
		}
	}
}

// ContactUpdates returns changes to a user's contact list that are more recent
// than ts.
func (d *Database) ContactUpdates(email string, ts int64) ([]stingle.Contact, error) {
	var contacts map[string]Contact
	if _, err := d.md.ReadDataFile(d.filePath("home", email, contactListFile), &contacts); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if contacts == nil {
		contacts = make(map[string]Contact)
	}
	out := []stingle.Contact{}
	for _, v := range contacts {
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
