package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"

	"c2FmZQ/internal/stingle"
)

// Share sharing albums matching pattern with contacts.
func (c *Client) Share(pattern string, shareWith []string, permissions []string) error {
	if c.Account == nil {
		return ErrNotLoggedIn
	}
	li, err := c.GlobFiles([]string{pattern}, GlobOptions{})
	if err != nil {
		return err
	}
	for _, item := range li {
		if item.Album == nil {
			return fmt.Errorf("not an album: %s", item.Filename)
		}
		if item.Album.IsOwner != "1" && !stingle.Permissions(item.Album.Permissions).AllowShare() {
			return fmt.Errorf("resharing is not permitted: %s", item.Filename)
		}
	}
	var cl ContactList
	if err := c.storage.ReadDataFile(c.fileHash(contactsFile), &cl); err != nil {
		return err
	}
	var members []*stingle.Contact
	maxSize := 5
	for _, email := range shareWith {
		if email == c.Account.Email {
			continue
		}
		if len(email) > maxSize {
			maxSize = len(email)
		}
		found := false
		for _, c := range cl.Contacts {
			if c.Email == email {
				members = append(members, c)
				found = true
				break
			}
		}
		if found {
			continue
		}
		c, err := c.sendGetContact(email)
		if err != nil {
			return err
		}
		members = append(members, c)
	}
	if len(members) == 0 {
		return fmt.Errorf("no match: %s", shareWith)
	}
	c.Print("Sharing with:\n")
	c.Printf("%*s %s\n", -maxSize, "Email", "Public Key")
	var list []string
	for _, m := range members {
		pk, _ := m.PK()
		list = append(list, fmt.Sprintf("%*s % X", -maxSize, m.Email, pk.ToBytes()))
	}
	sort.Strings(list)
	for _, l := range list {
		c.Print(l)
	}
	c.Print("\nWARNING: Verify the public keys of your contacts, then confirm.\n")
	if reply, err := c.prompt("Type YES to confirm: "); err != nil || reply != "YES" {
		return errors.New("not confirmed")
	}
	for _, item := range li {
		album := item.Album
		sharingKeys := make(map[string]string)
		sk, err := album.SK(c.SecretKey())
		if err != nil {
			return err
		}
		ids := []string{fmt.Sprintf("%d", c.Account.UserID)}
		for _, m := range members {
			id := m.UserID.String()
			pk, err := m.PK()
			if err != nil {
				return err
			}
			sharingKeys[id] = pk.SealBoxBase64(sk.ToBytes())
			ids = append(ids, id)
		}
		album.Members = strings.Join(ids, ",")
		if album.Permissions, err = c.parsePermissions("1000", permissions); err != nil {
			return err
		}

		if err := c.sendShare(album, sharingKeys); err != nil {
			return err
		}
		c.Printf("Now sharing %s with %s. (synced)\n", item.Filename, strings.Join(shareWith, ", "))
	}
	return nil
}

// Unshare stops sharing albums.
func (c *Client) Unshare(patterns []string) error {
	if c.Account == nil {
		return ErrNotLoggedIn
	}
	li, err := c.GlobFiles(patterns, GlobOptions{})
	if err != nil {
		return err
	}
	for _, item := range li {
		if item.Album == nil {
			return fmt.Errorf("not an album: %s", item.Filename)
		}
		if item.Album.IsOwner != "1" {
			return fmt.Errorf("not owner: %s", item.Filename)
		}
	}
	for _, item := range li {
		if err := c.sendUnshareAlbum(item.Album.AlbumID); err != nil {
			return err
		}
		c.Printf("Stopped sharing %s. (synced)\n", item.Filename)
	}
	return nil
}

// Leave removes an album that was shared with us.
func (c *Client) Leave(patterns []string) error {
	if c.Account == nil {
		return ErrNotLoggedIn
	}
	li, err := c.GlobFiles(patterns, GlobOptions{})
	if err != nil {
		return err
	}
	for _, item := range li {
		if item.Album == nil {
			return fmt.Errorf("not an album: %s", item.Filename)
		}
		if item.Album.IsOwner == "1" {
			return fmt.Errorf("is owner: %s", item.Filename)
		}
	}
	for _, item := range li {
		if err := c.sendLeaveAlbum(item.Album.AlbumID); err != nil {
			return err
		}
		c.Printf("Left %s. (synced)\n", item.Filename)
	}
	return nil
}

// RemoveMember removes members of an album.
func (c *Client) RemoveMembers(pattern string, toRemove []string) error {
	if c.Account == nil {
		return ErrNotLoggedIn
	}
	li, err := c.GlobFiles([]string{pattern}, GlobOptions{})
	if err != nil {
		return err
	}
	for _, item := range li {
		if item.Album == nil {
			return fmt.Errorf("not an album: %s", item.Filename)
		}
		if item.Album.IsOwner != "1" {
			return fmt.Errorf("not owner: %s", item.Filename)
		}
	}
	var cl ContactList
	if err := c.storage.ReadDataFile(c.fileHash(contactsFile), &cl); err != nil {
		return err
	}
	var ids []string
	for _, email := range toRemove {
		if email == c.Account.Email {
			continue
		}
		found := false
		for _, c := range cl.Contacts {
			if c.Email == email {
				ids = append(ids, c.UserID.String())
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("not a contact: %s", email)
		}
	}

	for _, item := range li {
		album := item.Album
		members := make(map[string]bool)
		for _, m := range strings.Split(album.Members, ",") {
			members[m] = true
		}
		for _, sid := range ids {
			if !members[sid] {
				continue
			}
			id, _ := strconv.ParseInt(sid, 10, 64)
			if err := c.sendRemoveAlbumMember(album, id); err != nil {
				return err
			}
			c.Printf("Removed %s from %s. (synced)\n", cl.Contacts[id].Email, item.Filename)
		}
	}
	return nil
}

// ChangePermissions changes the permissions on albums.
func (c *Client) ChangePermissions(patterns, perms []string) (retErr error) {
	if c.Account == nil {
		return ErrNotLoggedIn
	}
	li, err := c.GlobFiles(patterns, GlobOptions{})
	if err != nil {
		return err
	}
	for _, item := range li {
		if item.Album == nil {
			return fmt.Errorf("not an album: %s", item.Filename)
		}
		if item.Album.IsOwner != "1" {
			return fmt.Errorf("not owner: %s", item.Filename)
		}
	}
	var al AlbumList
	commit, err := c.storage.OpenForUpdate(c.fileHash(albumList), &al)
	if err != nil {
		return err
	}
	defer commit(false, &retErr)
	for _, item := range li {
		p := item.Album.Permissions
		if len(p) != 4 {
			p = "1000"
		}
		if p, err = c.parsePermissions(p, perms); err != nil {
			return err
		}
		al.Albums[item.Album.AlbumID].Permissions = p
		al.Albums[item.Album.AlbumID].DateModified = nowJSON()
		c.Printf("Set permissions on %s to %s (%s). (not synced)\n", item.Filename, stingle.Permissions(p).Human(), p)
	}
	return commit(true, nil)
}

// Contacts displays the contacts matching the patterns.
func (c *Client) Contacts(patterns []string) error {
	var cl ContactList
	if err := c.storage.ReadDataFile(c.fileHash(contactsFile), &cl); err != nil {
		return err
	}
	var out []string
L:
	for _, c := range cl.Contacts {
		for _, p := range patterns {
			if m, err := path.Match(p, c.Email); err == nil && m {
				pk, _ := c.PK()
				out = append(out, fmt.Sprintf("%s (% X)", c.Email, pk.ToBytes()))
				continue L
			}
		}
	}
	if out == nil {
		c.Printf("No match.\n")
		return nil
	}
	sort.Strings(out)
	c.Printf("Contacts:\n")
	for _, e := range out {
		c.Printf("* %s\n", e)
	}
	return nil
}

func (c *Client) parsePermissions(p string, changes []string) (string, error) {
	b := []byte(p)
	if b[0] != '1' {
		return "", fmt.Errorf("unknown version: %c", b[0])
	}
	for _, c := range changes {
		switch c := strings.TrimSpace(strings.ToLower(c)); c {
		case "add", "+add", "allowadd", "+allowadd", "a", "+a":
			b[1] = '1'
		case "share", "+share", "allowshare", "+allowshare", "s", "+s":
			b[2] = '1'
		case "copy", "+copy", "allowcopy", "+allowcopy", "c", "+c":
			b[3] = '1'
		case "-add", "-allowadd", "-a":
			b[1] = '0'
		case "-share", "-allowshare", "-s":
			b[2] = '0'
		case "-copy", "-allowcopy", "-c":
			b[3] = '0'
		default:
			return "", fmt.Errorf("invalid permission value: %s", c)
		}
	}
	return string(b), nil
}

func (c *Client) sendGetContact(email string) (*stingle.Contact, error) {
	if c.Account == nil {
		return nil, ErrNotLoggedIn
	}
	params := make(map[string]string)
	params["email"] = email

	form := url.Values{}
	form.Set("token", c.Account.Token)
	form.Set("params", c.encodeParams(params))

	sr, err := c.sendRequest("/v2/sync/getContact", form, "")
	if err != nil {
		return nil, err
	}
	if sr.Status != "ok" {
		return nil, sr
	}
	var contact stingle.Contact
	if err := copyJSON(sr.Part("contact"), &contact); err != nil {
		return nil, err
	}
	return &contact, nil
}

func (c *Client) sendShare(album *stingle.Album, sharingKeys map[string]string) error {
	if c.Account == nil {
		return ErrNotLoggedIn
	}
	aj, err := json.Marshal(album)
	if err != nil {
		return err
	}
	kj, err := json.Marshal(sharingKeys)
	if err != nil {
		return err
	}
	params := make(map[string]string)
	params["album"] = string(aj)
	params["sharingKeys"] = string(kj)

	form := url.Values{}
	form.Set("token", c.Account.Token)
	form.Set("params", c.encodeParams(params))

	sr, err := c.sendRequest("/v2/sync/share", form, "")
	if err != nil {
		return err
	}
	if sr.Status != "ok" {
		return sr
	}
	return nil
}

func (c *Client) sendUnshareAlbum(albumID string) error {
	if c.Account == nil {
		return ErrNotLoggedIn
	}
	params := make(map[string]string)
	params["albumId"] = albumID

	form := url.Values{}
	form.Set("token", c.Account.Token)
	form.Set("params", c.encodeParams(params))

	sr, err := c.sendRequest("/v2/sync/unshareAlbum", form, "")
	if err != nil {
		return err
	}
	if sr.Status != "ok" {
		return sr
	}
	return nil
}

func (c *Client) sendLeaveAlbum(albumID string) error {
	if c.Account == nil {
		return ErrNotLoggedIn
	}
	params := make(map[string]string)
	params["albumId"] = albumID

	form := url.Values{}
	form.Set("token", c.Account.Token)
	form.Set("params", c.encodeParams(params))

	sr, err := c.sendRequest("/v2/sync/leaveAlbum", form, "")
	if err != nil {
		return err
	}
	if sr.Status != "ok" {
		return sr
	}
	return nil
}

func (c *Client) sendRemoveAlbumMember(album *stingle.Album, id int64) error {
	if c.Account == nil {
		return ErrNotLoggedIn
	}
	aj, err := json.Marshal(album)
	if err != nil {
		return err
	}
	params := make(map[string]string)
	params["album"] = string(aj)
	params["memberUserId"] = fmt.Sprintf("%d", id)

	form := url.Values{}
	form.Set("token", c.Account.Token)
	form.Set("params", c.encodeParams(params))

	sr, err := c.sendRequest("/v2/sync/removeAlbumMember", form, "")
	if err != nil {
		return err
	}
	if sr.Status != "ok" {
		return sr
	}
	return nil
}
