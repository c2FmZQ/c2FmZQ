package server_test

import (
	"fmt"
	"net/url"
	"testing"

	"stingle-server/database"
)

func TestUploadDownload(t *testing.T) {
	sock, shutdown := startServer(t)
	defer shutdown()

	c, err := createAccountAndLogin(sock, "alice")
	if err != nil {
		t.Fatalf("createAccountAndLogin failed: %v", err)
	}
	if err := c.addAlbum("album1"); err != nil {
		t.Fatalf("c.addAlbum failed: %v", err)
	}

	// Upload to gallery.
	sr, err := c.uploadFile("filename1", database.GallerySet, "")
	if err != nil {
		t.Errorf("c.uploadFile failed: %v", err)
	}
	if want, got := "ok", sr.Status; want != got {
		t.Errorf("c.uploadFile returned unexpected status: Want %q, got %q", want, got)
	}

	// Upload album.
	if sr, err = c.uploadFile("filename2", database.AlbumSet, "album1"); err != nil {
		t.Errorf("c.uploadFile failed: %v", err)
	}
	if want, got := "ok", sr.Status; want != got {
		t.Errorf("c.uploadFile returned unexpected status: Want %q, got %q", want, got)
	}

	// Upload to a non-existent album should fail.
	if sr, err = c.uploadFile("filename3", database.AlbumSet, "DoesNotExist"); err != nil {
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
		return "", fmt.Errorf("status:nok %+v", sr)
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
		return nil, fmt.Errorf("status:nok %+v", sr)
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
