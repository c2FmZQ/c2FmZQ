package client

import (
	"encoding/json"
	"fmt"
	"net/url"

	"kringle-server/stingle"
)

func copyJSON(src interface{}, dst interface{}) error {
	b, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dst)
}

func (c *Client) GetUpdates() error {
	form := url.Values{}
	form.Set("token", c.Token)
	form.Set("fileST", "0")
	form.Set("trashST", "0")
	form.Set("albumsST", "0")
	form.Set("albumFilesST", "0")
	form.Set("cntST", "0")
	form.Set("delST", "0")
	sr, err := c.sendRequest("/v2/sync/getUpdates", form)
	if err != nil {
		return err
	}
	if sr.Status != "ok" {
		return sr
	}

	var albums []stingle.Album
	if err := copyJSON(sr.Parts["albums"], &albums); err != nil {
		return err
	}
	fmt.Printf("Albums: %+v\n", albums)

	var gallery []stingle.File
	if err := copyJSON(sr.Parts["files"], &gallery); err != nil {
		return err
	}
	fmt.Printf("Gallery: %+v\n", gallery)

	var trash []stingle.File
	if err := copyJSON(sr.Parts["trash"], &trash); err != nil {
		return err
	}
	fmt.Printf("Trash: %+v\n", trash)

	var albumFiles []stingle.File
	if err := copyJSON(sr.Parts["albumFiles"], &albumFiles); err != nil {
		return err
	}
	fmt.Printf("Album files: %+v\n", albumFiles)

	var contacts []stingle.Contact
	if err := copyJSON(sr.Parts["contacts"], &contacts); err != nil {
		return err
	}
	fmt.Printf("Contacts: %+v\n", contacts)

	var deletes []stingle.DeleteEvent
	if err := copyJSON(sr.Parts["deletes"], &deletes); err != nil {
		return err
	}
	fmt.Printf("Deletes: %+v\n", deletes)

	return nil
}
