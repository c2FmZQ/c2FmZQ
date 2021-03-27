package server_test

import (
	"fmt"
	"net/url"
	"testing"

	"stingle-server/database"
	"stingle-server/stingle"
)

func TestUploadDownload(t *testing.T) {
	sock, shutdown := startServer(t)
	defer shutdown()

	c, err := createAccountAndLogin(sock, "alice")
	if err != nil {
		t.Fatalf("createAccountAndLogin failed: %v", err)
	}
	if err := c.addAlbum("album1", 1000); err != nil {
		t.Fatalf("c.addAlbum failed: %v", err)
	}

	// Upload to gallery.
	sr, err := c.uploadFile("filename1", database.GallerySet, "", 1000)
	if err != nil {
		t.Errorf("c.uploadFile failed: %v", err)
	}
	if want, got := "ok", sr.Status; want != got {
		t.Errorf("c.uploadFile returned unexpected status: Want %q, got %q", want, got)
	}

	// Upload album.
	if sr, err = c.uploadFile("filename2", database.AlbumSet, "album1", 1000); err != nil {
		t.Errorf("c.uploadFile failed: %v", err)
	}
	if want, got := "ok", sr.Status; want != got {
		t.Errorf("c.uploadFile returned unexpected status: Want %q, got %q", want, got)
	}

	// Upload to a non-existent album should fail.
	if sr, err = c.uploadFile("filename3", database.AlbumSet, "DoesNotExist", 1000); err != nil {
		t.Errorf("c.uploadFile failed: %v", err)
	}
	if want, got := "nok", sr.Status; want != got {
		t.Errorf("c.uploadFile returned unexpected status: Want %q, got %q", want, got)
	}

	// Download with /v2/sync/download
	for _, f := range []struct{ filename, set, thumb, body string }{
		{"filename1", database.GallerySet, "0", `Content of "file" filename "filename1"`},
		{"filename1", database.GallerySet, "1", `Content of "thumb" filename "filename1"`},
		{"filename2", database.AlbumSet, "0", `Content of "file" filename "filename2"`},
		{"filename2", database.AlbumSet, "1", `Content of "thumb" filename "filename2"`},
	} {
		body, err := c.downloadPost(f.filename, f.set, f.thumb)
		if err != nil {
			t.Fatalf("c.downloadPost(%q, %q, %q) failed: %v", f.filename, f.set, f.thumb, err)
		}
		if want, got := f.body, body; want != got {
			t.Errorf("c.downloadPost returned unexpected body: Want %q, got %q", want, got)
		}
	}

	// Download with /v2/sync/getUrl
	for _, f := range []struct{ filename, set, body string }{
		{"filename1", database.GallerySet, `Content of "file" filename "filename1"`},
		{"filename2", database.AlbumSet, `Content of "file" filename "filename2"`},
	} {
		url, err := c.getURL(f.filename, f.set)
		if err != nil {
			t.Fatalf("c.getURL(%q, %q) failed: %v", f.filename, f.set, err)
		}
		body, err := c.downloadGet(url)
		if err != nil {
			t.Fatalf("c.downloadGet(%q) failed: %v", url, err)
		}
		if want, got := f.body, body; want != got {
			t.Errorf("c.downloadGet returned unexpected body: Want %q, got %q", want, got)
		}
	}

	// Download with /v2/sync/getDownloadUrls
	files := []string{"filename1", "filename2"}
	sets := []string{database.GallerySet, database.AlbumSet}
	urls, err := c.getDownloadURLs(files, sets, false)
	if err != nil {
		t.Errorf("c.getDownloadURLs(%v, %v, false) failed: %v", files, sets, err)
	}
	for _, f := range []struct{ filename, body string }{
		{"filename1", `Content of "file" filename "filename1"`},
		{"filename2", `Content of "file" filename "filename2"`},
	} {
		body, err := c.downloadGet(urls[f.filename])
		if err != nil {
			t.Fatalf("c.downloadGet(%q) failed: %v", urls[f.filename], err)
		}
		if want, got := f.body, body; want != got {
			t.Errorf("c.downloadGet returned unexpected body: Want %q, got %q", want, got)
		}
	}

	if urls, err = c.getDownloadURLs(files, sets, true); err != nil {
		t.Errorf("c.getDownloadURLs(%v, %v, true) failed: %v", files, sets, err)
	}
	for _, f := range []struct{ filename, body string }{
		{"filename1", `Content of "thumb" filename "filename1"`},
		{"filename2", `Content of "thumb" filename "filename2"`},
	} {
		body, err := c.downloadGet(urls[f.filename])
		if err != nil {
			t.Fatalf("c.downloadGet(%q) failed: %v", urls[f.filename], err)
		}
		if want, got := f.body, body; want != got {
			t.Errorf("c.downloadGet returned unexpected body: Want %q, got %q", want, got)
		}
	}
}

func TestEmptyTrash(t *testing.T) {
	sock, shutdown := startServer(t)
	defer shutdown()

	c, err := createAccountAndLogin(sock, "alice")
	if err != nil {
		t.Fatalf("createAccountAndLogin failed: %v", err)
	}

	// Upload to trash.
	for i := 0; i < 10; i++ {
		f := fmt.Sprintf("filename%d", i)
		sr, err := c.uploadFile(f, database.TrashSet, "", 1000)
		if err != nil {
			t.Errorf("c.uploadFile(%q) failed: %v", f, err)
		}
		if want, got := "ok", sr.Status; want != got {
			t.Errorf("c.uploadFile returned unexpected status: Want %q, got %q", want, got)
		}
	}

	// Empty trash.
	if err := c.emptyTrash(nowString()); err != nil {
		t.Errorf("c.emptyTrash failed: %v", err)
	}

	// Attempts to download the deleted files should fail.
	for i := 0; i < 10; i++ {
		f := fmt.Sprintf("filename%d", i)
		url, err := c.getURL(f, database.TrashSet)
		if err != nil {
			t.Errorf("c.getURL(%q, %q) failed: %v", f, database.TrashSet, err)
		}
		if _, err := c.downloadGet(url); err == nil {
			t.Errorf("c.downloadGet(%q) should have failed, but didn't", url)
		}
	}
}

func TestDelete(t *testing.T) {
	sock, shutdown := startServer(t)
	defer shutdown()

	c, err := createAccountAndLogin(sock, "alice")
	if err != nil {
		t.Fatalf("createAccountAndLogin failed: %v", err)
	}

	// Upload to trash.
	for i := 0; i < 10; i++ {
		f := fmt.Sprintf("filename%d", i)
		sr, err := c.uploadFile(f, database.TrashSet, "", 1000)
		if err != nil {
			t.Errorf("c.uploadFile(%q) failed: %v", f, err)
		}
		if want, got := "ok", sr.Status; want != got {
			t.Errorf("c.uploadFile returned unexpected status: Want %q, got %q", want, got)
		}
	}

	// Empty trash.
	files := []string{"filename0", "filename1"}
	if err := c.deleteFiles(files); err != nil {
		t.Errorf("c.deleteFile(%v) failed: %v", files, err)
	}

	// Attempts to download the deleted files should fail.
	for i := 0; i < 10; i++ {
		f := fmt.Sprintf("filename%d", i)
		url, err := c.getURL(f, database.TrashSet)
		if err != nil {
			t.Errorf("c.getURL(%q, %q) failed: %v", f, database.TrashSet, err)
		}
		if i < 2 {
			// These are deleted.
			if _, err := c.downloadGet(url); err == nil {
				t.Errorf("c.downloadGet(%q) should have failed, but didn't", url)
			}
			continue
		}
		// These are still there.
		if _, err := c.downloadGet(url); err != nil {
			t.Errorf("c.downloadGet(%q) failed unexpectedly: %v", url, err)
		}
	}
}

func TestMoveFile(t *testing.T) {
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
	if err := c.addAlbum("album2", 1000); err != nil {
		t.Fatalf("c.addAlbum failed: %v", err)
	}

	database.CurrentTimeForTesting = 2000

	// Upload to gallery.
	for i := 0; i < 10; i++ {
		f := fmt.Sprintf("filename%d", i)
		sr, err := c.uploadFile(f, database.GallerySet, "", 1000)
		if err != nil {
			t.Errorf("c.uploadFile(%q) failed: %v", f, err)
		}
		if want, got := "ok", sr.Status; want != got {
			t.Errorf("c.uploadFile returned unexpected status: Want %q, got %q", want, got)
		}
	}

	database.CurrentTimeForTesting = 3000

	// Move 2 files to trash.
	if err := c.moveFiles(database.MoveFileParams{
		SetFrom:   database.GallerySet,
		SetTo:     database.TrashSet,
		Filenames: []string{"filename0", "filename1"},
		IsMoving:  true,
	}); err != nil {
		t.Errorf("c.moveFiles failed: %v", err)
	}

	database.CurrentTimeForTesting = 4000

	// Move 2 files to album1.
	if err := c.moveFiles(database.MoveFileParams{
		SetFrom:   database.GallerySet,
		SetTo:     database.AlbumSet,
		AlbumIDTo: "album1",
		Filenames: []string{"filename2", "filename3"},
		Headers:   []string{"filename2 headers album1", "filename3 headers album1"},
		IsMoving:  true,
	}); err != nil {
		t.Errorf("c.moveFiles failed: %v", err)
	}

	database.CurrentTimeForTesting = 5000

	// Copy 2 files to album2.
	if err := c.moveFiles(database.MoveFileParams{
		SetFrom:   database.GallerySet,
		SetTo:     database.AlbumSet,
		AlbumIDTo: "album2",
		Filenames: []string{"filename4", "filename5"},
		Headers:   []string{"filename4 headers album2", "filename5 headers album2"},
		IsMoving:  false,
	}); err != nil {
		t.Errorf("c.moveFiles failed: %v", err)
	}

	got, err := c.getUpdates(0, 0, 0, 0, 0, 0)
	if err != nil {
		t.Fatalf("c.getUpdates failed: %v", err)
	}

	want := stingle.ResponseOK().
		AddPartList("trash",
			map[string]interface{}{"albumId": "", "dateCreated": "1000", "dateModified": "3000", "file": "filename0", "headers": "filename0 headers ", "version": "1"},
			map[string]interface{}{"albumId": "", "dateCreated": "1000", "dateModified": "3000", "file": "filename1", "headers": "filename1 headers ", "version": "1"},
		).
		AddPartList("files",
			map[string]interface{}{"albumId": "", "dateCreated": "1000", "dateModified": "2000", "file": "filename4", "headers": "filename4 headers ", "version": "1"},
			map[string]interface{}{"albumId": "", "dateCreated": "1000", "dateModified": "2000", "file": "filename5", "headers": "filename5 headers ", "version": "1"},
			map[string]interface{}{"albumId": "", "dateCreated": "1000", "dateModified": "2000", "file": "filename6", "headers": "filename6 headers ", "version": "1"},
			map[string]interface{}{"albumId": "", "dateCreated": "1000", "dateModified": "2000", "file": "filename7", "headers": "filename7 headers ", "version": "1"},
			map[string]interface{}{"albumId": "", "dateCreated": "1000", "dateModified": "2000", "file": "filename8", "headers": "filename8 headers ", "version": "1"},
			map[string]interface{}{"albumId": "", "dateCreated": "1000", "dateModified": "2000", "file": "filename9", "headers": "filename9 headers ", "version": "1"},
		).
		AddPartList("albums",
			map[string]interface{}{"albumId": "album1", "cover": "", "dateCreated": "1000", "dateModified": "1000", "encPrivateKey": "album1 encPrivateKey", "isHidden": "0", "isLocked": "0", "isOwner": "1", "isShared": "0", "members": "", "metadata": "album1 metadata", "permissions": "", "publicKey": "album1 publicKey"},
			map[string]interface{}{"albumId": "album2", "cover": "", "dateCreated": "1000", "dateModified": "1000", "encPrivateKey": "album2 encPrivateKey", "isHidden": "0", "isLocked": "0", "isOwner": "1", "isShared": "0", "members": "", "metadata": "album2 metadata", "permissions": "", "publicKey": "album2 publicKey"},
		).
		AddPartList("albumFiles",
			map[string]interface{}{"albumId": "album1", "dateCreated": "1000", "dateModified": "4000", "file": "filename2", "headers": "filename2 headers album1", "version": "1"},
			map[string]interface{}{"albumId": "album1", "dateCreated": "1000", "dateModified": "4000", "file": "filename3", "headers": "filename3 headers album1", "version": "1"},
			map[string]interface{}{"albumId": "album2", "dateCreated": "1000", "dateModified": "5000", "file": "filename4", "headers": "filename4 headers album2", "version": "1"},
			map[string]interface{}{"albumId": "album2", "dateCreated": "1000", "dateModified": "5000", "file": "filename5", "headers": "filename5 headers album2", "version": "1"},
		).
		AddPartList("deletes",
			map[string]interface{}{"albumId": "", "date": "3000", "file": "filename0", "type": "1"},
			map[string]interface{}{"albumId": "", "date": "3000", "file": "filename1", "type": "1"},
			map[string]interface{}{"albumId": "", "date": "4000", "file": "filename2", "type": "1"},
			map[string]interface{}{"albumId": "", "date": "4000", "file": "filename3", "type": "1"},
		)
	if diff := diffUpdates(want, got); diff != "" {
		t.Errorf("Unexpected updates:\n%s", diff)
	}
}

func (c *client) getURL(file, set string) (string, error) {
	form := url.Values{}
	form.Set("token", c.token)
	form.Set("file", file)
	form.Set("set", set)
	sr, err := c.sendRequest("/v2/sync/getUrl", form)
	if err != nil {
		return "", err
	}
	if sr.Status != "ok" {
		return "", sr
	}
	url, ok := sr.Parts["url"].(string)
	if !ok {
		return "", fmt.Errorf("server did not return a url: %v", sr.Parts["url"])
	}
	return url, nil
}

func (c *client) getDownloadURLs(files, sets []string, isThumb bool) (map[string]string, error) {
	form := url.Values{}
	form.Set("token", c.token)
	if isThumb {
		form.Set("is_thumb", "1")
	}
	for i := range files {
		form.Set(fmt.Sprintf("files[%d][filename]", i), files[i])
		form.Set(fmt.Sprintf("files[%d][set]", i), sets[i])
	}
	sr, err := c.sendRequest("/v2/sync/getDownloadUrls", form)
	if err != nil {
		return nil, err
	}
	if sr.Status != "ok" {
		return nil, sr
	}
	urls, ok := sr.Parts["urls"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("server did not return a list of urls: %#v", sr.Parts["urls"])
	}
	out := make(map[string]string)
	for k, v := range urls {
		out[k] = v.(string)
	}
	return out, nil
}

func (c *client) emptyTrash(ts string) error {
	params := map[string]string{"time": ts}
	form := url.Values{}
	form.Set("token", c.token)
	form.Set("params", c.encodeParams(params))

	sr, err := c.sendRequest("/v2/sync/emptyTrash", form)
	if err != nil {
		return err
	}
	if sr.Status != "ok" {
		return sr
	}
	return nil
}

func (c *client) deleteFiles(files []string) error {
	params := make(map[string]string)
	params["count"] = fmt.Sprintf("%d", len(files))
	for i, f := range files {
		params[fmt.Sprintf("filename%d", i)] = f
	}
	form := url.Values{}
	form.Set("token", c.token)
	form.Set("params", c.encodeParams(params))

	sr, err := c.sendRequest("/v2/sync/delete", form)
	if err != nil {
		return err
	}
	if sr.Status != "ok" {
		return sr
	}
	return nil
}

func (c *client) moveFiles(p database.MoveFileParams) error {
	params := make(map[string]string)
	params["setFrom"] = p.SetFrom
	params["setTo"] = p.SetTo
	params["albumIdFrom"] = p.AlbumIDFrom
	params["albumIdTo"] = p.AlbumIDTo
	if p.IsMoving {
		params["isMoving"] = "1"
	} else {
		params["isMoving"] = "0"
	}
	params["count"] = fmt.Sprintf("%d", len(p.Filenames))
	for i, f := range p.Filenames {
		params[fmt.Sprintf("filename%d", i)] = f
		if len(p.Headers) == len(p.Filenames) {
			params[fmt.Sprintf("headers%d", i)] = p.Headers[i]
		}
	}
	form := url.Values{}
	form.Set("token", c.token)
	form.Set("params", c.encodeParams(params))

	sr, err := c.sendRequest("/v2/sync/moveFile", form)
	if err != nil {
		return err
	}
	if sr.Status != "ok" {
		return sr
	}
	return nil
}
