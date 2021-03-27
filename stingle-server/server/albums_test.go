package server_test

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"testing"

	"stingle-server/database"
	"stingle-server/stingle"
)

func TestAddDeleteAlbum(t *testing.T) {
	sock, shutdown := startServer(t)
	defer shutdown()

	database.CurrentTimeForTesting = 1000

	c, err := createAccountAndLogin(sock, "alice")
	if err != nil {
		t.Fatalf("createAccountAndLogin failed: %v", err)
	}
	if err := c.addAlbum("album1", 1000); err != nil {
		t.Fatalf("c.addAlbum failed: %v", err)
	}

	got, err := c.getUpdates(0, 0, 0, 0, 0, 0)
	if err != nil {
		t.Fatalf("c.getUpdates failed: %v", err)
	}
	want := stingle.ResponseOK().
		AddPartList("albums", map[string]interface{}{
			"albumId":       "album1",
			"cover":         "",
			"dateCreated":   "1000",
			"dateModified":  "1000",
			"encPrivateKey": "album1 encPrivateKey",
			"isHidden":      "0",
			"isLocked":      "0",
			"isOwner":       "1",
			"isShared":      "0",
			"members":       "",
			"metadata":      "album1 metadata",
			"permissions":   "",
			"publicKey":     "album1 publicKey",
		})
	if diff := diffUpdates(want, got); diff != "" {
		t.Errorf("Unexpected updates:\n%v", diff)
	}

	database.CurrentTimeForTesting = 2000

	if err := c.deleteAlbum("album1"); err != nil {
		t.Fatalf("c.deleteAlbum failed: %v", err)
	}

	if got, err = c.getUpdates(0, 0, 0, 0, 0, 0); err != nil {
		t.Fatalf("c.getUpdates failed: %v", err)
	}
	want = stingle.ResponseOK().
		AddPartList("deletes", map[string]interface{}{
			"albumId": "album1", "date": "2000", "file": "", "type": "4",
		})
	if diff := diffUpdates(want, got); diff != "" {
		t.Errorf("Unexpected updates:\n%v", diff)
	}
}

func TestShareAlbum(t *testing.T) {
	sock, shutdown := startServer(t)
	defer shutdown()

	database.CurrentTimeForTesting = 1000

	alice, err := createAccountAndLogin(sock, "alice")
	if err != nil {
		t.Fatalf("createAccountAndLogin failed: %v", err)
	}
	bob, err := createAccountAndLogin(sock, "bob")
	if err != nil {
		t.Fatalf("createAccountAndLogin failed: %v", err)
	}
	carol, err := createAccountAndLogin(sock, "carol")
	if err != nil {
		t.Fatalf("createAccountAndLogin failed: %v", err)
	}

	if err := alice.addAlbum("album", 1000); err != nil {
		t.Fatalf("alice.addAlbum failed: %v", err)
	}

	album := stingle.Album{
		AlbumID:     "album",
		IsShared:    "1",
		Permissions: "1111",
		Members:     fmt.Sprintf("%d,%d", alice.userID, bob.userID),
		SharingKeys: map[string]string{
			fmt.Sprintf("%d", bob.userID): "Bob's Sharing Key",
		},
	}

	database.CurrentTimeForTesting = 2000

	if err := alice.shareAlbum(album); err != nil {
		t.Fatalf("alice.shareAlbum failed: %v", err)
	}

	got, err := alice.getUpdates(0, 0, 0, 0, 0, 0)
	if err != nil {
		t.Fatalf("alice.getUpdates failed: %v", err)
	}
	want := stingle.ResponseOK().
		AddPartList("albums", map[string]interface{}{
			"albumId":       "album",
			"cover":         "",
			"dateCreated":   "1000",
			"dateModified":  "2000",
			"encPrivateKey": "album encPrivateKey",
			"isHidden":      "0",
			"isLocked":      "0",
			"isOwner":       "1",
			"isShared":      "1",
			"members":       "1,2",
			"metadata":      "album metadata",
			"permissions":   "1111",
			"publicKey":     "album publicKey",
		}).
		AddPartList("contacts", map[string]interface{}{
			"dateModified": "2000", "email": "bob", "publicKey": base64.StdEncoding.EncodeToString(bob.secretKey.PublicKey().Bytes), "userId": fmt.Sprintf("%d", bob.userID),
		})

	if diff := diffUpdates(want, got); diff != "" {
		t.Errorf("Unexpected updates:\n%v", diff)
	}

	if got, err = bob.getUpdates(0, 0, 0, 0, 0, 0); err != nil {
		t.Fatalf("bob.getUpdates failed: %v", err)
	}
	want = stingle.ResponseOK().
		AddPartList("albums", map[string]interface{}{
			"albumId":       "album",
			"cover":         "",
			"dateCreated":   "1000",
			"dateModified":  "2000",
			"encPrivateKey": "Bob's Sharing Key",
			"isHidden":      "0",
			"isLocked":      "0",
			"isOwner":       "0",
			"isShared":      "1",
			"members":       "1,2",
			"metadata":      "album metadata",
			"permissions":   "1111",
			"publicKey":     "album publicKey",
		}).
		AddPartList("contacts", map[string]interface{}{
			"dateModified": "2000", "email": "alice", "publicKey": base64.StdEncoding.EncodeToString(alice.secretKey.PublicKey().Bytes), "userId": fmt.Sprintf("%d", alice.userID),
		})

	if diff := diffUpdates(want, got); diff != "" {
		t.Errorf("Unexpected updates:\n%v", diff)
	}

	album = stingle.Album{
		AlbumID: "album",
		Members: fmt.Sprintf("%d", carol.userID),
		SharingKeys: map[string]string{
			fmt.Sprintf("%d", carol.userID): "Carol's Sharing Key",
		},
	}

	database.CurrentTimeForTesting = 3000

	if err := bob.shareAlbum(album); err != nil {
		t.Fatalf("bob.shareAlbum failed: %v", err)
	}

	if got, err = carol.getUpdates(0, 0, 0, 0, 0, 0); err != nil {
		t.Fatalf("carol.getUpdates failed: %v", err)
	}
	want = stingle.ResponseOK().
		AddPartList("albums", map[string]interface{}{
			"albumId":       "album",
			"cover":         "",
			"dateCreated":   "1000",
			"dateModified":  "3000",
			"encPrivateKey": "Carol's Sharing Key",
			"isHidden":      "0",
			"isLocked":      "0",
			"isOwner":       "0",
			"isShared":      "1",
			"members":       "1,2,3",
			"metadata":      "album metadata",
			"permissions":   "1111",
			"publicKey":     "album publicKey",
		}).
		AddPartList("contacts",
			map[string]interface{}{
				"dateModified": "3000", "email": "alice", "publicKey": base64.StdEncoding.EncodeToString(alice.secretKey.PublicKey().Bytes), "userId": fmt.Sprintf("%d", alice.userID),
			},
			map[string]interface{}{
				"dateModified": "3000", "email": "bob", "publicKey": base64.StdEncoding.EncodeToString(bob.secretKey.PublicKey().Bytes), "userId": fmt.Sprintf("%d", bob.userID),
			})

	if diff := diffUpdates(want, got); diff != "" {
		t.Errorf("Unexpected updates:\n%v", diff)
	}
}

func (c *client) addAlbum(albumID string, ts int64) error {
	params := make(map[string]string)
	params["albumId"] = albumID
	params["dateCreated"] = fmt.Sprintf("%d", ts)
	params["dateModified"] = fmt.Sprintf("%d", ts)
	params["encPrivateKey"] = albumID + " encPrivateKey"
	params["metadata"] = albumID + " metadata"
	params["publicKey"] = albumID + " publicKey"

	form := url.Values{}
	form.Set("token", c.token)
	form.Set("params", c.encodeParams(params))

	sr, err := c.sendRequest("/v2/sync/addAlbum", form)
	if err != nil {
		return err
	}
	if sr.Status != "ok" {
		return fmt.Errorf("status:nok %+v", sr)
	}
	return nil
}

func (c *client) deleteAlbum(albumID string) error {
	params := make(map[string]string)
	params["albumId"] = albumID

	form := url.Values{}
	form.Set("token", c.token)
	form.Set("params", c.encodeParams(params))

	sr, err := c.sendRequest("/v2/sync/deleteAlbum", form)
	if err != nil {
		return err
	}
	if sr.Status != "ok" {
		return fmt.Errorf("status:nok %+v", sr)
	}
	return nil
}

func (c *client) shareAlbum(album stingle.Album) error {
	ja, err := json.Marshal(album)
	if err != nil {
		return err
	}
	jk, err := json.Marshal(album.SharingKeys)
	if err != nil {
		return err
	}
	params := make(map[string]string)
	params["album"] = string(ja)
	params["sharingKeys"] = string(jk)

	form := url.Values{}
	form.Set("token", c.token)
	form.Set("params", c.encodeParams(params))

	sr, err := c.sendRequest("/v2/sync/share", form)
	if err != nil {
		return err
	}
	if sr.Status != "ok" {
		return fmt.Errorf("status:nok %+v", sr)
	}
	return nil
}
