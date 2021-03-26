// Package stingle contains all the datastructures specific to the Stingle API.
package stingle

import (
	"encoding/json"
	"io"

	"stingle-server/log"
)

// The Stingle API version of Contact.
type Contact struct {
	UserID       string `json:"userId"`
	Email        string `json:"email"`
	PublicKey    string `json:"publicKey"`
	DateUsed     string `json:"dateUsed,omitempty"`
	DateModified string `json:"dateModified,omitempty"`
}

// The Stingle API representation of a File.
type File struct {
	File         string `json:"file"`
	Version      string `json:"version"`
	DateCreated  string `json:"dateCreated"`
	DateModified string `json:"dateModified"`
	Headers      string `json:"headers"`
	AlbumID      string `json:"albumId"`
}

// The Stingle API representation of an album.
type Album struct {
	AlbumID       string            `json:"albumId"`
	DateCreated   string            `json:"dateCreated"`
	DateModified  string            `json:"dateModified"`
	EncPrivateKey string            `json:"encPrivateKey"`
	Metadata      string            `json:"metadata"`
	PublicKey     string            `json:"publicKey"`
	IsShared      string            `json:"isShared"`
	IsHidden      string            `json:"isHidden"`
	IsOwner       string            `json:"isOwner"`
	Permissions   string            `json:"permissions"`
	IsLocked      string            `json:"isLocked"`
	Cover         string            `json:"cover"`
	Members       string            `json:"members"`
	SyncLocal     string            `json:"syncLocal,omitempty"`
	SharingKeys   map[string]string `json:"sharingKeys,omitempty"`
}

// Permissions that control what album members can do.
type Permissions string

func (p Permissions) AllowAdd() bool   { return len(p) == 4 && p[0] == '1' && p[1] == '1' }
func (p Permissions) AllowShare() bool { return len(p) == 4 && p[0] == '1' && p[2] == '1' }
func (p Permissions) AllowCopy() bool  { return len(p) == 4 && p[0] == '1' && p[3] == '1' }

const (
	// Delete event types.
	DeleteEventGallery     = 1 // A file is removed from the gallery.
	DeleteEventTrash       = 2 // A file is removed from the trash (and moved somewhere else).
	DeleteEventTrashDelete = 3 // A file is deleted from the trash.
	DeleteEventAlbum       = 4 // An album is deleted.
	DeleteEventAlbumFile   = 5 // A file is removed from an album.
	DeleteEventContact     = 6 // A contact is removed.
)

// The Stingle API representation of a Delete event.
type DeleteEvent struct {
	File    string `json:"file"`
	AlbumID string `json:"albumId"`
	Type    string `json:"type"`
	Date    string `json:"date"`
}

// ResponseOK returns a new Response with status OK.
func ResponseOK() *Response {
	return &Response{
		Status: "ok",
		Parts:  map[string]interface{}{},
		Infos:  []string{},
		Errors: []string{},
	}
}

// ResponseNOK returns a new Response with status NOK.
func ResponseNOK() *Response {
	return &Response{
		Status: "nok",
		Parts:  map[string]interface{}{},
		Infos:  []string{},
		Errors: []string{},
	}
}

// Response is the data structure used as return value for most API calls.
// 'Status' is set to ok when the request was successful, and nok otherwise.
// 'Parts' contains any data returned to the caller.
// 'Infos' and 'Errors' are messages displayed to the user.
type Response struct {
	Status string                 `json:"status"`
	Parts  map[string]interface{} `json:"parts"`
	Infos  []string               `json:"infos"`
	Errors []string               `json:"errors"`
}

// AddPart adds a value to Parts.
func (r *Response) AddPart(name string, value interface{}) *Response {
	r.Parts[name] = value
	return r
}

// AddPartList adds a list of values to Parts.
func (r *Response) AddPartList(name string, values ...interface{}) *Response {
	r.Parts[name] = values
	return r
}

// AddInfo adds a value to Infos.
func (r *Response) AddInfo(value string) *Response {
	r.Infos = append(r.Infos, value)
	return r
}

// AddError adds a value to Errors.
func (r *Response) AddError(value string) *Response {
	r.Errors = append(r.Errors, value)
	return r
}

// Send sends the .
func (r Response) Send(w io.Writer) error {
	if r.Status == "" {
		log.Panic("Response has empty status")
	}
	j, err := json.Marshal(r)
	if err != nil {
		return err
	}
	log.Infof("Response: %s", j)
	_, err = w.Write(j)
	return err
}
