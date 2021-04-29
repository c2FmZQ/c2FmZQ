// Package stingle contains datastructures and code specific to the Stingle API.
package stingle

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"

	"kringle/internal/log"
)

// The Stingle API version of Contact.
type Contact struct {
	UserID       json.Number `json:"userId"`
	Email        string      `json:"email"`
	PublicKey    string      `json:"publicKey"`
	DateUsed     json.Number `json:"dateUsed,omitempty"`
	DateModified json.Number `json:"dateModified,omitempty"`
}

// The Stingle API representation of a File.
type File struct {
	File         string      `json:"file"`
	Version      string      `json:"version"`
	DateCreated  json.Number `json:"dateCreated"`
	DateModified json.Number `json:"dateModified"`
	Headers      string      `json:"headers"`
	AlbumID      string      `json:"albumId"`
}

// The Stingle API representation of an album.
type Album struct {
	AlbumID       string            `json:"albumId"`
	DateCreated   json.Number       `json:"dateCreated"`
	DateModified  json.Number       `json:"dateModified"`
	EncPrivateKey string            `json:"encPrivateKey"`
	Metadata      string            `json:"metadata"`
	PublicKey     string            `json:"publicKey"`
	IsShared      json.Number       `json:"isShared"`
	IsHidden      json.Number       `json:"isHidden"`
	IsOwner       json.Number       `json:"isOwner"`
	Permissions   string            `json:"permissions"`
	IsLocked      json.Number       `json:"isLocked"`
	Cover         string            `json:"cover"`
	Members       string            `json:"members"`
	SyncLocal     json.Number       `json:"syncLocal,omitempty"`
	SharingKeys   map[string]string `json:"sharingKeys,omitempty"`
}

// PK returns the contact's decoded PublicKey.
func (c Contact) PK() (pk PublicKey, err error) {
	b, err := base64.StdEncoding.DecodeString(c.PublicKey)
	if err != nil {
		return
	}
	pk = PublicKeyFromBytes(b)
	return
}

// Name returns the decrypted file name.
func (f File) Name(sk SecretKey) (string, error) {
	hdrs, err := DecryptBase64Headers(f.Headers, sk)
	if err != nil {
		return "", err
	}
	return string(hdrs[0].Filename), nil
}

// SK returns the album's decrypted SecretKey.
func (a Album) SK(sk SecretKey) (ask SecretKey, err error) {
	b, err := sk.SealBoxOpenBase64(a.EncPrivateKey)
	if err != nil {
		return
	}
	ask = SecretKeyFromBytes(b)
	return
}

// PK returns the album's decoded PublicKey.
func (a Album) PK() (pk PublicKey, err error) {
	b, err := base64.StdEncoding.DecodeString(a.PublicKey)
	if err != nil {
		return
	}
	pk = PublicKeyFromBytes(b)
	return
}

// Name returns the decrypted album name.
func (a Album) Name(sk SecretKey) (string, error) {
	ask, err := a.SK(sk)
	if err != nil {
		return "", err
	}
	md, err := DecryptAlbumMetadata(a.Metadata, ask)
	if err != nil {
		return "", err
	}
	return md.Name, nil
}

func (a *Album) Equals(b *Album) bool {
	if b == nil {
		return false
	}
	return reflect.DeepEqual(*a, *b)
}

// Permissions that control what album members can do.
type Permissions string

func (p Permissions) AllowAdd() bool   { return len(p) == 4 && p[0] == '1' && p[1] == '1' }
func (p Permissions) AllowShare() bool { return len(p) == 4 && p[0] == '1' && p[2] == '1' }
func (p Permissions) AllowCopy() bool  { return len(p) == 4 && p[0] == '1' && p[3] == '1' }

func (p Permissions) Human() string {
	var out []string
	if p.AllowAdd() {
		out = append(out, "+Add")
	} else {
		out = append(out, "-Add")
	}
	if p.AllowCopy() {
		out = append(out, "+Copy")
	} else {
		out = append(out, "-Copy")
	}
	if p.AllowShare() {
		out = append(out, "+Share")
	} else {
		out = append(out, "-Share")
	}
	return strings.Join(out, ",")
}

const (
	GallerySet = "0"
	TrashSet   = "1"
	AlbumSet   = "2"

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
	File    string      `json:"file"`
	AlbumID string      `json:"albumId"`
	Type    json.Number `json:"type"`
	Date    json.Number `json:"date"`
}

const (
	// Values of Header.FileType.
	FileTypeGeneral = 1
	FileTypePhoto   = 2
	FileTypeVideo   = 3
)

func FileType(t uint8) string {
	switch t {
	case FileTypeGeneral:
		return "general file type"
	case FileTypePhoto:
		return "photo"
	case FileTypeVideo:
		return "video"
	default:
		return "unknown file type"
	}
}

// Header is a the header of an encrypted file.
type Header struct {
	FileID        []byte
	Version       uint8
	ChunkSize     int32
	DataSize      int64
	SymmetricKey  []byte
	FileType      uint8
	Filename      []byte
	VideoDuration int32
}

// ResponseOK returns a new Response with status OK.
func ResponseOK() *Response {
	return &Response{
		Status: "ok",
		Errors: []string{},
	}
}

// ResponseNOK returns a new Response with status NOK.
func ResponseNOK() *Response {
	return &Response{
		Status: "nok",
		Errors: []string{},
	}
}

// Response is the data structure used as return value for most API calls.
// 'Status' is set to ok when the request was successful, and nok otherwise.
// 'Parts' contains any data returned to the caller.
// 'Infos' and 'Errors' are messages displayed to the user.
type Response struct {
	Status string      `json:"status"`
	Parts  interface{} `json:"parts"`
	Infos  []string    `json:"infos"`
	Errors []string    `json:"errors"`
}

// Error makes it so that Response can be returned as an error.
func (r Response) Error() string {
	return fmt.Sprintf("status:%q errors:%v", r.Status, r.Errors)
}

// AddPart adds a value to Parts.
func (r *Response) AddPart(name string, value interface{}) *Response {
	if r.Parts == nil {
		r.Parts = make(map[string]interface{})
	}
	r.Parts.(map[string]interface{})[name] = value
	return r
}

// Part returns the value of the named part of the response.
func (r *Response) Part(name string) interface{} {
	parts, ok := r.Parts.(map[string]interface{})
	if !ok {
		log.Errorf("Response.Parts has unexpected type: %T", r.Parts)
		return ""
	}
	return parts[name]
}

// AddPartList adds a list of values to Parts.
func (r *Response) AddPartList(name string, values ...interface{}) *Response {
	if r.Parts == nil {
		r.Parts = make(map[string]interface{})
	}
	r.Parts.(map[string]interface{})[name] = values
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

// Send sends the Response.
func (r Response) Send(w io.Writer) error {
	if r.Status == "" {
		log.Panic("Response has empty status")
	}
	if r.Parts == nil {
		r.Parts = []string{}
	}
	if r.Infos == nil {
		r.Infos = []string{}
	}
	log.Debugf("Response: %#v", r)
	return json.NewEncoder(w).Encode(r)
}
