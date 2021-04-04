package database_test

import (
	"github.com/go-test/deep"
	"testing"

	"kringle-server/crypto/stinglecrypto"
	"kringle-server/database"
)

func addUser(db *database.Database, email string, pk stinglecrypto.PublicKey) error {
	u := database.User{
		Email:     email,
		Password:  email + "-Password",
		Salt:      email + "-Salt",
		KeyBundle: email + "KeyBundle",
		IsBackup:  "0",
		PublicKey: pk,
	}
	return db.AddUser(u)
}

func TestUsers(t *testing.T) {
	dir := t.TempDir()
	db := database.New(dir, "")
	database.CurrentTimeForTesting = 10000

	// Add, lookup, modify users.
	emails := []string{"alice@", "bob@", "charlie@"}
	keys := make(map[string]stinglecrypto.SecretKey)
	users := make(map[string]database.User)
	for _, e := range emails {
		keys[e] = stinglecrypto.MakeSecretKey()
		if err := addUser(db, e, keys[e].PublicKey()); err != nil {
			t.Fatalf("addUser(%q, pk) failed: %v", e, err)
		}
		u, err := db.User(e)
		if err != nil {
			t.Fatalf("User(%q) failed: %v", e, err)
		}
		users[e] = u

		u2, err := db.UserByID(u.UserID)
		if err != nil {
			t.Fatalf("UserByID(%q) failed: %v", e, err)
		}
		if diff := deep.Equal(u, u2); diff != nil {
			t.Errorf("User and UserByID returned different results: %v", diff)
		}

	}

	alice := users["alice@"]
	alice.Password = "Alice's new password"
	if err := db.UpdateUser(alice); err != nil {
		t.Errorf("UpdateUser(%v) failed: %v", alice, err)
	}
	if u, err := db.User("alice@"); err != nil {
		t.Errorf("User(alice@q) failed: %v", err)
	} else if want, got := alice.Password, u.Password; want != got {
		t.Errorf("UpdateUser() failed to update the user: want %q, got %q", want, got)
	}

	// Contacts
	for _, e := range emails[1:] {
		c, err := db.AddContact(alice, e)
		if err != nil {
			t.Errorf("AddContact(%v, %q) failed: %v", alice, e, err)
		}
		if want, got := users[e].UserID, c.UserID; want != got {
			t.Errorf("Unexpected contact UserID: want %q, got %q", want, got)
		}
		if want, got := users[e].Email, c.Email; want != got {
			t.Errorf("Unexpected contact Email: want %q, got %q", want, got)
		}
	}

	cu, err := db.ContactUpdates(alice.Email, 0)
	if err != nil {
		t.Errorf("ContactUpdates(%q) failed: %v", alice.Email, err)
	}
	if want, got := 2, len(cu); want != got {
		t.Errorf("Unexpected number of contacts: want %d, got %d", want, got)
	}
}
