package database_test

import (
	"fmt"
	"github.com/go-test/deep"
	"testing"

	"c2FmZQ/internal/database"
	"c2FmZQ/internal/stingle"
)

func addUser(db *database.Database, email string, pk stingle.PublicKey) error {
	u := database.User{
		Email:          email,
		HashedPassword: email + "-Password",
		Salt:           email + "-Salt",
		KeyBundle:      email + "KeyBundle",
		IsBackup:       "0",
		PublicKey:      pk,
	}
	return db.AddUser(u)
}

func TestUsers(t *testing.T) {
	dir := t.TempDir()
	db := database.New(dir, "")
	database.CurrentTimeForTesting = 10000

	// Add, lookup, modify users.
	emails := []string{"alice@", "bob@", "charlie@"}
	keys := make(map[string]stingle.SecretKey)
	users := make(map[string]database.User)
	for _, e := range emails {
		keys[e] = stingle.MakeSecretKey()
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
	alice.HashedPassword = "Alice's new password"
	if err := db.UpdateUser(alice); err != nil {
		t.Errorf("UpdateUser(%v) failed: %v", alice, err)
	}
	if u, err := db.User("alice@"); err != nil {
		t.Errorf("User(alice@q) failed: %v", err)
	} else if want, got := alice.HashedPassword, u.HashedPassword; want != got {
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

	cu, err := db.ContactUpdates(alice, 0)
	if err != nil {
		t.Errorf("ContactUpdates(%q) failed: %v", alice.Email, err)
	}
	if want, got := 2, len(cu); want != got {
		t.Errorf("Unexpected number of contacts: want %d, got %d", want, got)
	}

	if err := db.DeleteUser(users["bob@"]); err != nil {
		t.Fatalf("DeleteUser(%q) failed: %v", users["bob@"].Email, err)
	}
	du, err := db.DeleteUpdates(alice, 0)
	if err != nil {
		t.Fatalf("DeleteUpdates(%q) failed: %v", alice.Email, err)
	}
	if want, got := 1, len(du); want != got {
		t.Fatalf("Unexpected number of deleted contacts: want %d, got %d", want, got)
	}
	if want, got := fmt.Sprintf("%d", users["bob@"].UserID), du[0].File; want != got {
		t.Fatalf("Unexpected deleted contact id: want %s, got %s", want, got)
	}
}
