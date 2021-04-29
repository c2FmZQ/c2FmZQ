package server_test

import (
	"fmt"
	"net/url"
	"strings"

	"kringle/internal/stingle"
)

func (c *client) getUpdates(fileST, trashST, albumsST, albumFilesST, cntST, delST int64) (*stingle.Response, error) {
	form := url.Values{}
	form.Set("token", c.token)
	form.Set("fileST", fmt.Sprintf("%d", fileST))
	form.Set("trashST", fmt.Sprintf("%d", trashST))
	form.Set("albumsST", fmt.Sprintf("%d", albumsST))
	form.Set("albumFilesST", fmt.Sprintf("%d", albumFilesST))
	form.Set("cntST", fmt.Sprintf("%d", cntST))
	form.Set("delST", fmt.Sprintf("%d", delST))

	sr, err := c.sendRequest("/v2/sync/getUpdates", form)
	if err != nil {
		return nil, err
	}
	if sr.Status != "ok" {
		return nil, sr
	}
	return sr, nil
}

func addMissingFields(sr *stingle.Response) {
	if sr.Parts == nil {
		sr.Parts = make(map[string]interface{})
	}
	for _, t := range []string{"files", "trash", "albums", "albumFiles", "contacts", "deletes"} {
		if sr.Parts[t] == nil {
			sr.Parts[t] = []interface{}{}
		}
	}
}

func compareLists(s1, s2 []interface{}) []string {
	m1 := make(map[string]bool)
	for _, v := range s1 {
		m1[fmt.Sprintf("%#v", v)] = true
	}
	m2 := make(map[string]bool)
	for _, v := range s2 {
		m2[fmt.Sprintf("%#v", v)] = true
	}
	var out []string
	for k, _ := range m1 {
		if !m2[k] {
			out = append(out, fmt.Sprintf("  - %s", k))
		}
	}
	for k, _ := range m2 {
		if !m1[k] {
			out = append(out, fmt.Sprintf("  + %s", k))
		}
	}
	return out
}

func diffUpdates(u1, u2 *stingle.Response) string {
	addMissingFields(u1)
	addMissingFields(u2)
	var out []string
	if u1.Status != u2.Status {
		out = append(out, fmt.Sprintf("Status %q != %q", u1.Status, u2.Status))
	}
	for _, f := range []string{"files", "trash", "albums", "albumFiles", "contacts", "deletes"} {
		if diff := compareLists(u1.Parts[f].([]interface{}), u2.Parts[f].([]interface{})); diff != nil {
			out = append(out, fmt.Sprintf("In %s:", f))
			out = append(out, diff...)
		}
	}
	return strings.Join(out, "\n")
}
