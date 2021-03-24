package server_test

import (
	"fmt"
	"net/url"
)

func (c *client) addAlbum(albumID string) error {
	params := make(map[string]string)
	params["albumId"] = albumID
	params["dateCreated"] = nowString()
	params["dateModified"] = nowString()
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
