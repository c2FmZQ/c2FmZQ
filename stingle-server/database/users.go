package database

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"stingle-server/crypto"
	"stingle-server/log"
	"stingle-server/stingle"
)

const (
	userListFile    = "users.json"
	userFile        = "user.json"
	contactListFile = "contact-list.json"
)

// This is used internally for the list of all users in the system.
type userList struct {
	UserID int    `json:"userId"`
	Email  string `json:"email"`
}

// Encapsulates all the information about a user account.
type User struct {
	UserID        int                  `json:"userId"`
	Email         string               `json:"email"`
	Password      string               `json:"password"`
	Salt          string               `json:"salt"`
	HomeFolder    string               `json:"homeFolder"`
	KeyBundle     string               `json:"keyBundle"`
	IsBackup      string               `json:"isBackup"`
	ServerKey     crypto.SecretKey     `json:"serverKey"`
	ServerSignKey crypto.SignSecretKey `json:"serverSignKey"`
	PublicKey     crypto.PublicKey     `json:"publicKey"`
	TokenSeq      int                  `json:"tokenSeq"`
}

// Encapsulates the information about a user's contact (another user).
type Contact struct {
	UserID       int    `json:"userId"`
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

// Home returns the directory where the user's data is stored.
func (d Database) Home(email string) string {
	return filepath.Join(d.dir, "users", base64.RawURLEncoding.EncodeToString([]byte(email)))
}

// HomeByID is like Home() except that it uses a User ID instead of email.
func (d Database) HomeByID(userID int) (string, error) {
	var ul []userList
	if err := loadJSON(filepath.Join(d.Dir(), userListFile), &ul); err != nil {
		return "", err
	}
	for _, u := range ul {
		if u.UserID == userID {
			return d.Home(u.Email), nil
		}
	}
	return "", os.ErrNotExist
}

// AddUser creates a new user account for u.
func (d *Database) AddUser(u User) (retErr error) {
	home := d.Home(u.Email)
	if err := os.MkdirAll(home, 0700); err != nil {
		return err
	}

	var ul []userList
	done, err := openForUpdate(filepath.Join(d.Dir(), userListFile), &ul)
	if err != nil {
		log.Errorf("openForUpdate: %v", err)
		return err
	}
	defer done(&retErr)
	uid := 0
	for _, i := range ul {
		if i.Email == u.Email {
			return os.ErrExist
		}
		if i.UserID > uid {
			uid = i.UserID
		}
	}
	uid += 1
	hf := base64.RawURLEncoding.EncodeToString([]byte(filepath.Join("users", u.Email)))
	ul = append(ul, userList{UserID: uid, Email: u.Email})

	u.UserID = uid
	u.HomeFolder = hf
	u.ServerKey = crypto.MakeSecretKey()
	u.ServerSignKey = crypto.MakeSignSecretKey()
	u.TokenSeq = 1
	return saveJSON(filepath.Join(home, userFile), u)
}

// UpdateUser saves a user object.
func (d *Database) UpdateUser(u User) error {
	return saveJSON(filepath.Join(d.Home(u.Email), userFile), u)
}

// UserByID returns the User object with the given ID.
func (d *Database) UserByID(id int) (User, error) {
	var ul []userList
	if err := loadJSON(filepath.Join(d.Dir(), userListFile), &ul); err != nil {
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
	err := loadJSON(filepath.Join(d.Home(email), userFile), &u)
	return u, err
}

// SignKeyForUser returns the server's SignSecretKey associated with this user.
func (d *Database) SignKeyForUser(email string) crypto.SignSecretKey {
	if u, err := d.User(email); err == nil {
		return u.ServerSignKey
	}
	return crypto.SignSecretKey{}
}

// Export converts a Contact to stingle.Contact.
func (c Contact) Export() stingle.Contact {
	return stingle.Contact{
		UserID:       fmt.Sprintf("%d", c.UserID),
		Email:        c.Email,
		PublicKey:    c.PublicKey,
		DateUsed:     fmt.Sprintf("%d", c.DateUsed),
		DateModified: fmt.Sprintf("%d", c.DateModified),
	}
}

// addContactToUser adds contact to user's contact list.
func (d *Database) addContactToUser(user, contact User) (c *Contact, retErr error) {
	home := d.Home(user.Email)

	var contactList map[string]*Contact
	done, err := openForUpdate(filepath.Join(home, contactListFile), &contactList)
	if err != nil {
		log.Errorf("openForUpdate: %v", err)
		return nil, err
	}
	defer done(&retErr)

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
func (d *Database) lookupContacts(uids map[int]bool) []Contact {
	var ul []userList
	if err := loadJSON(filepath.Join(d.Dir(), userListFile), &ul); err != nil {
		return nil
	}
	var out []Contact
	for _, u := range ul {
		if uids[u.UserID] {
			user, err := d.User(u.Email)
			if err != nil {
				log.Infof("d.User(%q) failed, but user in %q: %v", u.Email, userListFile, err)
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
		home := d.Home(c1.Email)

		var contactList map[string]*Contact
		done, err := openForUpdate(filepath.Join(home, contactListFile), &contactList)
		if err != nil {
			log.Errorf("openForUpdate: %v", err)
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
		if err := done(nil); err != nil {
			log.Errorf("Failed to save user %d's contact list: %v", c1.UserID, err)
		} else {
			log.Infof("Added %d contact(s) to user %d", count, c1.UserID)
		}
	}
}

// ContactUpdates returns changes to a user's contact list that are more recent
// than ts.
func (d *Database) ContactUpdates(email string, ts int64) ([]stingle.Contact, error) {
	home := d.Home(email)
	var contacts map[string]Contact
	if err := loadJSON(filepath.Join(home, contactListFile), &contacts); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if contacts == nil {
		contacts = make(map[string]Contact)
	}
	out := []stingle.Contact{}
	for _, v := range contacts {
		if v.DateModified > ts {
			sc := stingle.Contact{
				UserID:       fmt.Sprintf("%d", v.UserID),
				Email:        v.Email,
				PublicKey:    v.PublicKey,
				DateModified: fmt.Sprintf("%d", v.DateModified),
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
