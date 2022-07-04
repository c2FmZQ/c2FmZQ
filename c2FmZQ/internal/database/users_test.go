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
	_, err := db.AddUser(u)
	return err
}

func TestUsers(t *testing.T) {
	dir := t.TempDir()
	db := database.New(dir, nil)
	database.CurrentTimeForTesting = 10000

	// Add, lookup, modify users.
	emails := []string{"alice@", "bob@", "charlie@"}
	keys := make(map[string]*stingle.SecretKey)
	users := make(map[string]database.User)
	for _, e := range emails {
		keys[e] = stingle.MakeSecretKeyForTest()
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

func TestRenameUser(t *testing.T) {
	dir := t.TempDir()
	db := database.New(dir, nil)
	database.CurrentTimeForTesting = 10000

	// Add, lookup, modify users.
	emails := []string{"alice@", "bob@", "carol@"}
	keys := make(map[string]*stingle.SecretKey)
	users := make(map[string]database.User)
	for _, e := range emails {
		keys[e] = stingle.MakeSecretKeyForTest()
		if err := addUser(db, e, keys[e].PublicKey()); err != nil {
			t.Fatalf("addUser(%q, pk) failed: %v", e, err)
		}
		u, err := db.User(e)
		if err != nil {
			t.Fatalf("User(%q) failed: %v", e, err)
		}
		users[e] = u
	}

	for _, e := range emails[1:] {
		if _, err := db.AddContact(users[e], "alice@"); err != nil {
			t.Fatalf("AddContact(%q, %q) failed: %v", e, "alice@", err)
		}
	}
	database.CurrentTimeForTesting = 20000

	alice := users["alice@"]
	if err := db.RenameUser(alice.UserID, "notalice@"); err != nil {
		t.Fatalf("db.RenameUser failed: %v", err)
	}
	if u, err := db.User("notalice@"); err != nil {
		t.Fatalf("db.User(notalice) failed: %v", err)
	} else if want, got := alice.UserID, u.UserID; want != got {
		t.Errorf("Unexpected userID after rename. Want %d, got %d", want, got)
	}

	cu, err := db.ContactUpdates(users["bob@"], 0)
	if err != nil {
		t.Errorf("ContactUpdates(%q) failed: %v", "bob@", err)
	}
	if want, got := 1, len(cu); want != got {
		t.Fatalf("Unexpected number of contacts: want %d, got %d", want, got)
	}
	if want, got := "notalice@", cu[0].Email; want != got {
		t.Errorf("Unexpected contact email. want %q, got %q", want, got)
	}
	if want, got := "20000", cu[0].DateModified.String(); want != got {
		t.Errorf("Unexpected contact date modified. want %v, got %v", want, got)
	}

}
