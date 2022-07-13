package database_test

import (
	"encoding/json"
	"testing"

	"c2FmZQ/internal/database"
	"github.com/go-test/deep"
)

func TestTag(t *testing.T) {
	dir := t.TempDir()
	db := database.New(dir, nil)
	database.CurrentTimeForTesting = 10000

	id, err := db.AddUser(database.User{Email: "1@", NeedApproval: false, Admin: false})
	if err != nil {
		t.Fatalf("db.AddUser: %v", err)
	}
	data, err := db.AdminData(nil)
	if err != nil {
		t.Fatalf("db.AdminData: %v", err)
	}
	data2, err := db.AdminData(nil)
	if err != nil {
		t.Fatalf("db.AdminData: %v", err)
	}
	user, err := db.UserByID(id)
	if err != nil {
		t.Fatalf("db.UserByID: %v", err)
	}
	user.Admin = true
	if err := db.UpdateUser(user); err != nil {
		t.Fatalf("db.UpdateUser: %v", err)
	}
	data3, err := db.AdminData(nil)
	if err != nil {
		t.Fatalf("db.AdminData: %v", err)
	}

	ser := func(v interface{}) string {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("json.Marshal: %v", err)
		}
		return string(b)
	}
	if got, want := ser(data), ser(data2); got != want {
		t.Errorf("Unexpected AdminData. Got %#v, want %#v", got, want)
	}
	if data.Tag == data3.Tag {
		t.Errorf("Tags unexpectedly equal: %s == %s", data.Tag, data3.Tag)
	}
}

func ptr[T any](v T) *T {
	return &v
}

func TestUpdates(t *testing.T) {
	dir := t.TempDir()
	db := database.New(dir, nil)
	database.CurrentTimeForTesting = 10000

	emails := []string{"alice", "bob", "carol"}
	var userIDs []int64
	for _, e := range emails {
		id, err := db.AddUser(database.User{Email: e})
		if err != nil {
			t.Fatalf("db.AddUser: %v", err)
		}
		userIDs = append(userIDs, id)
	}
	data, err := db.AdminData(nil)
	if err != nil {
		t.Fatalf("db.AdminData: %v", err)
	}
	updates := database.AdminData{
		Tag:              data.Tag,
		DefaultQuota:     ptr(int64(10)),
		DefaultQuotaUnit: ptr("MB"),
		Users: []database.AdminUser{
			{
				UserID:    userIDs[0],
				Admin:     ptr(true),
				Locked:    ptr(true),
				Approved:  ptr(false),
				Quota:     ptr(int64(1)),
				QuotaUnit: ptr("GB"),
			},
			{
				UserID:    userIDs[2],
				Quota:     ptr(int64(100)),
				QuotaUnit: ptr("MB"),
			},
		},
	}
	if data, err = db.AdminData(&updates); err != nil {
		t.Fatalf("db.AdminData: %v", err)
	}
	exp := &database.AdminData{
		Tag:              data.Tag,
		DefaultQuota:     ptr(int64(10)),
		DefaultQuotaUnit: ptr("MB"),
		Users: []database.AdminUser{
			{
				UserID:    userIDs[0],
				Email:     ptr("alice"),
				Admin:     ptr(true),
				Locked:    ptr(true),
				Approved:  ptr(false),
				Quota:     ptr(int64(1)),
				QuotaUnit: ptr("GB"),
			},
			{
				UserID:   userIDs[1],
				Email:    ptr("bob"),
				Admin:    ptr(false),
				Locked:   ptr(false),
				Approved: ptr(true),
			},
			{
				UserID:    userIDs[2],
				Email:     ptr("carol"),
				Admin:     ptr(false),
				Locked:    ptr(false),
				Approved:  ptr(true),
				Quota:     ptr(int64(100)),
				QuotaUnit: ptr("MB"),
			},
		},
	}

	if diff := deep.Equal(exp, data); diff != nil {
		t.Errorf("Unexpected data: %s", diff)
	}
}
