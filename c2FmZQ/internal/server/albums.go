//
// Copyright 2021-2022 TTBT Enterprises LLC
//
// This file is part of c2FmZQ (https://c2FmZQ.org/).
//
// c2FmZQ is free software: you can redistribute it and/or modify it under the
// terms of the GNU General Public License as published by the Free Software
// Foundation, either version 3 of the License, or (at your option) any later
// version.
//
// c2FmZQ is distributed in the hope that it will be useful, but WITHOUT ANY
// WARRANTY; without even the implied warranty of MERCHANTABILITY or FITNESS FOR
// A PARTICULAR PURPOSE. See the GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License along with
// c2FmZQ. If not, see <https://www.gnu.org/licenses/>.

package server

import (
	"encoding/json"
	"net/http"

	"c2FmZQ/internal/database"
	"c2FmZQ/internal/log"
	"c2FmZQ/internal/stingle"
)

// handleAddAlbum handles the /v2/sync/addAlbum endpoint. It is used to add a
// new album.
//
// Arguments:
//   - user: The authenticated user.
//   - req: The http request.
//
// Form arguments
//   - params: The encrypted parameters
//   - albumId: The ID of the album.
//   - dateCreated: A timestamp in milliseconds.
//   - dateModified: A timestamp in milliseconds.
//   - encPrivateKey: The encrypted private key for the album.
//   - metadata: The encrypted metadata of the album, e.g. it's name.
//   - publicKey: The public key of the album.
//
// Returns:
//   - stingle.Response(ok)
func (s *Server) handleAddAlbum(user database.User, req *http.Request) *stingle.Response {
	params, err := s.decodeParams(req.PostFormValue("params"), user)
	if err != nil {
		log.Errorf("decodeParams: %v", err)
		return stingle.ResponseNOK()
	}
	album := database.AlbumSpec{
		AlbumID:       params["albumId"],
		DateCreated:   parseInt(params["dateCreated"], 0),
		DateModified:  parseInt(params["dateModified"], 0),
		EncPrivateKey: params["encPrivateKey"],
		Metadata:      params["metadata"],
		PublicKey:     params["publicKey"],
	}
	if err := s.db.AddAlbum(user, album); err != nil {
		log.Errorf("AddAlbum: %v", err)
		return stingle.ResponseNOK()
	}
	return stingle.ResponseOK()
}

// handleDeleteAlbum handles the /v2/sync/deleteAlbum endpoint. It is used to
// delete an album.
//
// Arguments:
//   - user: The authenticated user.
//   - req: The http request.
//
// Form arguments
//   - params: The encrypted parameters
//   - albumId: The ID of the album.
//
// Returns:
//   - stingle.Response(ok)
func (s *Server) handleDeleteAlbum(user database.User, req *http.Request) *stingle.Response {
	params, err := s.decodeParams(req.PostFormValue("params"), user)
	if err != nil {
		log.Errorf("decodeParams: %v", err)
		return stingle.ResponseNOK()
	}
	albumID := params["albumId"]
	albumSpec, err := s.db.Album(user, albumID)
	if err != nil {
		log.Errorf("db.Album(%q, %q) failed: %v", user.Email, albumID, err)
		return stingle.ResponseNOK()
	}
	if albumSpec.OwnerID != user.UserID {
		return stingle.ResponseNOK().AddError("You are not the owner of the album")
	}

	if err := s.db.DeleteAlbum(user, albumID); err != nil {
		log.Errorf("DeleteAlbum: %v", err)
		return stingle.ResponseNOK()
	}
	return stingle.ResponseOK()
}

// handleChangeAlbumCover handles the /v2/sync/changeAlbumCover endpoint. It is used to
// change the album cover.
//
// Arguments:
//   - user: The authenticated user.
//   - req: The http request.
//
// Form arguments
//   - params: The encrypted parameters
//   - albumId: The ID of the album.
//   - cover: The filename to use as cover.
//
// Returns:
//   - stingle.Response(ok)
func (s *Server) handleChangeAlbumCover(user database.User, req *http.Request) *stingle.Response {
	params, err := s.decodeParams(req.PostFormValue("params"), user)
	if err != nil {
		log.Errorf("decodeParams: %v", err)
		return stingle.ResponseNOK()
	}
	albumID := params["albumId"]
	cover := params["cover"]

	albumSpec, err := s.db.Album(user, albumID)
	if err != nil {
		log.Errorf("db.Album(%q, %q) failed: %v", user.Email, albumID, err)
		return stingle.ResponseNOK()
	}
	if albumSpec.OwnerID != user.UserID {
		return stingle.ResponseNOK().AddError("You are not the owner of the album")
	}

	if err := s.db.ChangeAlbumCover(user, albumID, cover); err != nil {
		log.Errorf("ChangeAlbumCover: %v", err)
		return stingle.ResponseNOK()
	}
	return stingle.ResponseOK()
}

// handleRenameAlbum handles the /v2/sync/renameAlbum endpoint. It is used to
// rename an album.
//
// Arguments:
//   - user: The authenticated user.
//   - req: The http request.
//
// Form arguments
//   - params: The encrypted parameters
//   - albumId: The ID of the album.
//   - metadata: The encrypted metadata of the album.
//
// Returns:
//   - stingle.Response(ok)
func (s *Server) handleRenameAlbum(user database.User, req *http.Request) *stingle.Response {
	params, err := s.decodeParams(req.PostFormValue("params"), user)
	if err != nil {
		log.Errorf("decodeParams: %v", err)
		return stingle.ResponseNOK()
	}
	albumID := params["albumId"]
	metadata := params["metadata"]

	albumSpec, err := s.db.Album(user, albumID)
	if err != nil {
		log.Errorf("db.Album(%q, %q) failed: %v", user.Email, albumID, err)
		return stingle.ResponseNOK()
	}
	if albumSpec.OwnerID != user.UserID {
		return stingle.ResponseNOK().AddError("You are not the owner of the album")
	}

	if err := s.db.ChangeMetadata(user, albumID, metadata); err != nil {
		log.Errorf("ChangeMetadata: %v", err)
		return stingle.ResponseNOK()
	}
	return stingle.ResponseOK()
}

// handleGetContact handles the /v2/sync/getContact endpoint. It is used to
// get the contact information of another user.
//
// Arguments:
//   - user: The authenticated user.
//   - req: The http request.
//
// Form arguments
//   - params: The encrypted parameters
//   - email: The email of the contact.
//
// Returns:
//   - stingle.Response(ok).
//     Part(contact, contact object)
func (s *Server) handleGetContact(user database.User, req *http.Request) *stingle.Response {
	if user.NeedApproval {
		return stingle.ResponseNOK().
			AddError("Account is not approved yet")
	}
	params, err := s.decodeParams(req.PostFormValue("params"), user)
	if err != nil {
		log.Errorf("decodeParams: %v", err)
		return stingle.ResponseNOK()
	}
	contact, err := s.db.AddContact(user, params["email"])
	if err != nil {
		log.Errorf("AddContact: %v", err)
		return stingle.ResponseNOK()
	}
	return stingle.ResponseOK().AddPart("contact", contact)
}

func (s *Server) parseAlbumJSON(b []byte) (*stingle.Album, error) {
	var album stingle.Album
	if err := json.Unmarshal(b, &album); err != nil {
		return nil, err
	}
	return &album, nil
}

// handleShare handles the /v2/sync/share endpoint. It is used to share an
// album with some contacts.
//
// Arguments:
//   - user: The authenticated user.
//   - req: The http request.
//
// Form arguments
//   - params: The encrypted parameters
//   - album: A JSON-encoded album object.
//   - sharingKeys: A JSON-encoded map of UserID:SharingKey. The SharingKey is
//     the encPrivateKey to share with each member.
//
// Returns:
//   - stingle.Response(ok).
func (s *Server) handleShare(user database.User, req *http.Request) *stingle.Response {
	if user.NeedApproval {
		return stingle.ResponseNOK().
			AddError("Account is not approved yet")
	}
	params, err := s.decodeParams(req.PostFormValue("params"), user)
	if err != nil {
		log.Errorf("decodeParams: %v", err)
		return stingle.ResponseNOK()
	}

	album, err := s.parseAlbumJSON([]byte(params["album"]))
	if err != nil {
		return stingle.ResponseNOK()
	}

	var sharingKeys map[string]string
	if err := json.Unmarshal([]byte(params["sharingKeys"]), &sharingKeys); err != nil {
		log.Errorf("json.Unmarshal sharingKeys failed: %v", err)
		return stingle.ResponseNOK()
	}

	albumSpec, err := s.db.Album(user, album.AlbumID)
	if err != nil {
		log.Errorf("db.Album(%q, %q) failed: %v", user.Email, album.AlbumID, err)
		return stingle.ResponseNOK()
	}
	if albumSpec.OwnerID == user.UserID || (albumSpec.Members[user.UserID] && albumSpec.Permissions.AllowShare()) {
		if err := s.db.ShareAlbum(user, album, sharingKeys); err != nil {
			log.Errorf("ShareAlbum: %v", err)
			return stingle.ResponseNOK()
		}
		return stingle.ResponseOK()
	}
	return stingle.ResponseNOK().AddError("You are not allow to share the album")

}

// handleEditPerms handles the /v2/sync/editPerms endpoint. It is used to
// change the album permissions.
//
// Arguments:
//   - user: The authenticated user.
//   - req: The http request.
//
// Form arguments
//   - params: The encrypted parameters
//   - album: A JSON-encoded album object.
//
// Returns:
//   - stingle.Response(ok)
func (s *Server) handleEditPerms(user database.User, req *http.Request) *stingle.Response {
	params, err := s.decodeParams(req.PostFormValue("params"), user)
	if err != nil {
		log.Errorf("decodeParams: %v", err)
		return stingle.ResponseNOK()
	}

	album, err := s.parseAlbumJSON([]byte(params["album"]))
	if err != nil {
		return stingle.ResponseNOK()
	}

	albumSpec, err := s.db.Album(user, album.AlbumID)
	if err != nil {
		log.Errorf("db.Album(%q, %q) failed: %v", user.Email, album.AlbumID, err)
		return stingle.ResponseNOK()
	}
	if albumSpec.OwnerID != user.UserID {
		return stingle.ResponseNOK().AddError("You are not the owner of the album")
	}

	if err := s.db.UpdatePerms(user, album.AlbumID, stingle.Permissions(album.Permissions), album.IsHidden == "1", album.IsLocked == "1"); err != nil {
		log.Errorf("UpdatePerms(%q, %q): %v", album.AlbumID, album.Permissions, err)
		return stingle.ResponseNOK()
	}
	return stingle.ResponseOK()
}

// handleRemoveAlbumMember handles the /v2/sync/removeAlbumMember endpoint. It
// is used to remove a member from the album.
//
// Arguments:
//   - user: The authenticated user.
//   - req: The http request.
//
// Form arguments
//   - params: The encrypted parameters
//   - album: A JSON-encoded album object.
//   - memberUserId: The user ID to remove.
//
// Returns:
//   - stingle.Response(ok)
func (s *Server) handleRemoveAlbumMember(user database.User, req *http.Request) *stingle.Response {
	params, err := s.decodeParams(req.PostFormValue("params"), user)
	if err != nil {
		log.Errorf("decodeParams: %v", err)
		return stingle.ResponseNOK()
	}

	album, err := s.parseAlbumJSON([]byte(params["album"]))
	if err != nil {
		return stingle.ResponseNOK()
	}
	memberID := parseInt(params["memberUserId"], 0)

	albumSpec, err := s.db.Album(user, album.AlbumID)
	if err != nil {
		log.Errorf("db.Album(%q, %q) failed: %v", user.Email, album.AlbumID, err)
		return stingle.ResponseNOK()
	}
	if albumSpec.OwnerID != user.UserID {
		return stingle.ResponseNOK().AddError("You are not the owner of the album")
	}

	if err := s.db.RemoveAlbumMember(user, album.AlbumID, memberID); err != nil {
		log.Errorf("RemoveAlbumMember(%q, %q): %v", album.AlbumID, memberID, err)
		return stingle.ResponseNOK()
	}
	return stingle.ResponseOK()
}

// handleUnshareAlbum handles the /v2/sync/unshareAlbum endpoint. It is used to
// stop sharing an album.
//
// Arguments:
//   - user: The authenticated user.
//   - req: The http request.
//
// Form arguments
//   - params: The encrypted parameters
//   - albumId: The ID of the album to stop sharing.
//
// Returns:
//   - stingle.Response(ok)
func (s *Server) handleUnshareAlbum(user database.User, req *http.Request) *stingle.Response {
	params, err := s.decodeParams(req.PostFormValue("params"), user)
	if err != nil {
		log.Errorf("decodeParams: %v", err)
		return stingle.ResponseNOK()
	}

	albumID := params["albumId"]

	albumSpec, err := s.db.Album(user, albumID)
	if err != nil {
		log.Errorf("db.Album(%q, %q) failed: %v", user.Email, albumID, err)
		return stingle.ResponseNOK()
	}
	if albumSpec.OwnerID != user.UserID {
		return stingle.ResponseNOK().AddError("You are not the owner of the album")
	}

	if err := s.db.UnshareAlbum(user, albumID); err != nil {
		log.Errorf("UnshareAlbum(%q): %v", albumID, err)
		return stingle.ResponseNOK()
	}
	return stingle.ResponseOK()
}

// handleLeaveAlbum handles the /v2/sync/leaveAlbum endpoint. It is used to
// remove oneself from an album that was shared.
//
// Arguments:
//   - user: The authenticated user.
//   - req: The http request.
//
// Form arguments
//   - params: The encrypted parameters
//   - albumId: The ID of the album to leave.
//
// Returns:
//   - stingle.Response(ok)
func (s *Server) handleLeaveAlbum(user database.User, req *http.Request) *stingle.Response {
	params, err := s.decodeParams(req.PostFormValue("params"), user)
	if err != nil {
		log.Errorf("decodeParams: %v", err)
		return stingle.ResponseNOK()
	}
	albumID := params["albumId"]

	albumSpec, err := s.db.Album(user, albumID)
	if err != nil {
		log.Errorf("db.Album(%q, %q) failed: %v", user.Email, albumID, err)
		return stingle.ResponseNOK()
	}
	if albumSpec.OwnerID == user.UserID {
		return stingle.ResponseNOK().AddError("You can't leave your own album")
	}
	if !albumSpec.Members[user.UserID] {
		return stingle.ResponseNOK().AddError("You are not a member of this album")
	}

	if err := s.db.RemoveAlbumMember(user, albumID, user.UserID); err != nil {
		log.Errorf("RemoveAlbumMember(%q, %q): %v", albumID, user.UserID, err)
		return stingle.ResponseNOK()
	}
	return stingle.ResponseOK()
}
