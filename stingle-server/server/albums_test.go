package server_test

import (
	"fmt"
	"net/url"
	"testing"

	"stingle-server/database"
	"stingle-server/server"
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
	want := server.StingleResponse{
		Status: "ok",
		Parts: map[string]interface{}{
			"albums": []interface{}{
				map[string]interface{}{
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
				},
			},
		},
	}
	if diff := compareUpdates(want, got); diff != "" {
		t.Errorf("Unexpected updates:\n%v", diff)
	}

	database.CurrentTimeForTesting = 2000

	if err := c.deleteAlbum("album1"); err != nil {
		t.Fatalf("c.deleteAlbum failed: %v", err)
	}

	if got, err = c.getUpdates(0, 0, 0, 0, 0, 0); err != nil {
		t.Fatalf("c.getUpdates failed: %v", err)
	}
	want = server.StingleResponse{
		Status: "ok",
		Parts: map[string]interface{}{
			"deletes": []interface{}{
				map[string]interface{}{"albumId": "album1", "date": "2000", "file": "", "type": "4"},
			},
		},
	}
	if diff := compareUpdates(want, got); diff != "" {
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
