package client

import (
	"encoding/json"
	"fmt"
	"net/url"

	"kringle-server/stingle"
)

func (c *Client) Hide(names []string, hidden bool) (retErr error) {
	li, err := c.GlobFiles(names)
	if err != nil {
		return err
	}
	var al AlbumList
	commit, err := c.storage.OpenForUpdate(c.fileHash(albumList), &al)
	if err != nil {
		return err
	}
	defer commit(true, &retErr)
	count := 0
	for _, item := range li {
		if item.AlbumID == "" {
			continue
		}
		album, ok := al.Albums[item.AlbumID]
		if !ok {
			continue
		}
		if hidden {
			album.IsHidden = "1"
		} else {
			album.IsHidden = "0"
		}
		if err := c.editPerms(album); err != nil {
			return err
		}
		count++
	}
	if hidden && count > 0 {
		fmt.Printf("Successfully hidden %d album(s).\n", count)
	}
	if !hidden && count > 0 {
		fmt.Printf("Successfully unhidden %d album(s).\n", count)
	}
	return nil
}

func (c *Client) editPerms(album *stingle.Album) error {
	ja, err := json.Marshal(album)
	if err != nil {
		return err
	}
	params := make(map[string]string)
	params["album"] = string(ja)

	form := url.Values{}
	form.Set("token", c.Token)
	form.Set("params", c.encodeParams(params))

	sr, err := c.sendRequest("/v2/sync/editPerms", form)
	if err != nil {
		return err
	}
	if sr.Status != "ok" {
		return sr
	}
	return nil
}
