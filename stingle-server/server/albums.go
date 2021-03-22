package server

import (
	"encoding/json"
	"net/http"

	"stingle-server/database"
	"stingle-server/log"
)

// handleAddAlbum handles the /v2/sync/addAlbum endpoint. It is used to add a
// new album.
//
// Arguments:
//  - user: The authenticated user.
//  - req: The http request.
//
// Form arguments
//  - params: The encrypted parameters
//     - albumId: The ID of the album.
//     - dateCreated: A timestamp in milliseconds.
//     - dateModified: A timestamp in milliseconds.
//     - encPrivateKey: The encrypted private key for the album.
//     - metadata: The encrypted metadata of the album, e.g. it's name.
//     - publicKey: The public key of the album.
//
// Returns:
//  - StingleResponse(ok)
func (s *Server) handleAddAlbum(user database.User, req *http.Request) *StingleResponse {
	params, err := s.decodeParams(req.PostFormValue("params"), user)
	if err != nil {
		log.Errorf("decodeParams: %v", err)
		return NewResponse("nok")
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
		return NewResponse("nok")
	}
	return NewResponse("ok")
}

// handleDeleteAlbum handles the /v2/sync/deleteAlbum endpoint. It is used to
// delete an album.
//
// Arguments:
//  - user: The authenticated user.
//  - req: The http request.
//
// Form arguments
//  - params: The encrypted parameters
//     - albumId: The ID of the album.
//
// Returns:
//  - StingleResponse(ok)
func (s *Server) handleDeleteAlbum(user database.User, req *http.Request) *StingleResponse {
	params, err := s.decodeParams(req.PostFormValue("params"), user)
	if err != nil {
		log.Errorf("decodeParams: %v", err)
		return NewResponse("nok")
	}
	albumID := params["albumId"]
	albumSpec, err := s.db.Album(user, albumID)
	if err != nil {
		log.Errorf("db.Album(%q, %q) failed: %v", user.Email, albumID, err)
		return NewResponse("nok")
	}
	if albumSpec.OwnerID != user.UserID {
		return NewResponse("nok").AddError("You are not the owner of the album")
	}

	if err := s.db.DeleteAlbum(user, albumID); err != nil {
		log.Errorf("DeleteAlbum: %v", err)
		return NewResponse("nok")
	}
	return NewResponse("ok")
}

// handleChangeAlbumCover handles the /v2/sync/changeAlbumCover endpoint. It is used to
// change the album cover.
//
// Arguments:
//  - user: The authenticated user.
//  - req: The http request.
//
// Form arguments
//  - params: The encrypted parameters
//     - albumId: The ID of the album.
//     - cover: The filename to use as cover.
//
// Returns:
//  - StingleResponse(ok)
func (s *Server) handleChangeAlbumCover(user database.User, req *http.Request) *StingleResponse {
	params, err := s.decodeParams(req.PostFormValue("params"), user)
	if err != nil {
		log.Errorf("decodeParams: %v", err)
		return NewResponse("nok")
	}
	albumID := params["albumId"]
	cover := params["cover"]

	albumSpec, err := s.db.Album(user, albumID)
	if err != nil {
		log.Errorf("db.Album(%q, %q) failed: %v", user.Email, albumID, err)
		return NewResponse("nok")
	}
	if albumSpec.OwnerID != user.UserID {
		return NewResponse("nok").AddError("You are not the owner of the album")
	}

	if err := s.db.ChangeAlbumCover(user, albumID, cover); err != nil {
		log.Errorf("ChangeAlbumCover: %v", err)
		return NewResponse("nok")
	}
	return NewResponse("ok")
}

// handleRenameAlbum handles the /v2/sync/renameAlbum endpoint. It is used to
// rename an album.
//
// Arguments:
//  - user: The authenticated user.
//  - req: The http request.
//
// Form arguments
//  - params: The encrypted parameters
//     - albumId: The ID of the album.
//     - metadata: The encrypted metadata of the album.
//
// Returns:
//  - StingleResponse(ok)
func (s *Server) handleRenameAlbum(user database.User, req *http.Request) *StingleResponse {
	params, err := s.decodeParams(req.PostFormValue("params"), user)
	if err != nil {
		log.Errorf("decodeParams: %v", err)
		return NewResponse("nok")
	}
	albumID := params["albumId"]
	metadata := params["metadata"]

	albumSpec, err := s.db.Album(user, albumID)
	if err != nil {
		log.Errorf("db.Album(%q, %q) failed: %v", user.Email, albumID, err)
		return NewResponse("nok")
	}
	if albumSpec.OwnerID != user.UserID {
		return NewResponse("nok").AddError("You are not the owner of the album")
	}

	if err := s.db.ChangeMetadata(user, albumID, metadata); err != nil {
		log.Errorf("ChangeMetadata: %v", err)
		return NewResponse("nok")
	}
	return NewResponse("ok")
}

// handleGetContact handles the /v2/sync/getContact endpoint. It is used to
// get the contact information of another user.
//
// Arguments:
//  - user: The authenticated user.
//  - req: The http request.
//
// Form arguments
//  - params: The encrypted parameters
//     - email: The email of the contact.
//
// Returns:
//  - StingleResponse(ok).
//      Part(contact, contact object)
func (s *Server) handleGetContact(user database.User, req *http.Request) *StingleResponse {
	params, err := s.decodeParams(req.PostFormValue("params"), user)
	if err != nil {
		log.Errorf("decodeParams: %v", err)
		return NewResponse("nok")
	}
	contact, err := s.db.AddContact(user, params["email"])
	if err != nil {
		log.Errorf("AddContact: %v", err)
		return NewResponse("nok")
	}
	return NewResponse("ok").AddPart("contact", contact)
}

func (s *Server) parseAlbumJSON(b []byte) (*database.StingleAlbum, error) {
	var album database.StingleAlbum
	if err := json.Unmarshal(b, &album); err != nil {
		return nil, err
	}
	return &album, nil
}

// handleShare handles the /v2/sync/share endpoint. It is used to share an
// album with some contacts.
//
// Arguments:
//  - user: The authenticated user.
//  - req: The http request.
//
// Form arguments
//  - params: The encrypted parameters
//     - album: A JSON-encoded album object.
//     - sharingKeys: A JSON-encoded map of UserID:SharingKey. The SharingKey is
//                    the encPrivateKey to share with each member.
//
// Returns:
//  - StingleResponse(ok).
func (s *Server) handleShare(user database.User, req *http.Request) *StingleResponse {
	params, err := s.decodeParams(req.PostFormValue("params"), user)
	if err != nil {
		log.Errorf("decodeParams: %v", err)
		return NewResponse("nok")
	}

	album, err := s.parseAlbumJSON([]byte(params["album"]))
	if err != nil {
		return NewResponse("nok")
	}

	var sharingKeys map[string]string
	if err := json.Unmarshal([]byte(params["sharingKeys"]), &sharingKeys); err != nil {
		log.Errorf("json.Unmarshal sharingKeys failed: %v", err)
		return NewResponse("nok")
	}

	albumSpec, err := s.db.Album(user, album.AlbumID)
	if err != nil {
		log.Errorf("db.Album(%q, %q) failed: %v", user.Email, album.AlbumID, err)
		return NewResponse("nok")
	}
	if albumSpec.OwnerID == user.UserID || (albumSpec.Members[user.UserID] && albumSpec.Permissions.AllowShare()) {
		if err := s.db.ShareAlbum(user, album, sharingKeys); err != nil {
			log.Errorf("ShareAlbum: %v", err)
			return NewResponse("nok")
		}
		return NewResponse("ok")
	}
	return NewResponse("nok").AddError("You are not allow to share the album")

}

// handleEditPerms handles the /v2/sync/editPerms endpoint. It is used to
// change the album permissions.
//
// Arguments:
//  - user: The authenticated user.
//  - req: The http request.
//
// Form arguments
//  - params: The encrypted parameters
//     - album: A JSON-encoded album object.
//
// Returns:
//  - StingleResponse(ok)
func (s *Server) handleEditPerms(user database.User, req *http.Request) *StingleResponse {
	params, err := s.decodeParams(req.PostFormValue("params"), user)
	if err != nil {
		log.Errorf("decodeParams: %v", err)
		return NewResponse("nok")
	}

	album, err := s.parseAlbumJSON([]byte(params["album"]))
	if err != nil {
		return NewResponse("nok")
	}

	albumSpec, err := s.db.Album(user, album.AlbumID)
	if err != nil {
		log.Errorf("db.Album(%q, %q) failed: %v", user.Email, album.AlbumID, err)
		return NewResponse("nok")
	}
	if albumSpec.OwnerID != user.UserID {
		return NewResponse("nok").AddError("You are not the owner of the album")
	}

	if err := s.db.UpdatePerms(user, album.AlbumID, database.SharingPermissions(album.Permissions)); err != nil {
		log.Errorf("UpdatePerms(%q, %q): %v", album.AlbumID, album.Permissions, err)
		return NewResponse("nok")
	}
	return NewResponse("ok")
}

// handleRemoveAlbumMember handles the /v2/sync/removeAlbumMember endpoint. It
// is used to remove a member from the album.
//
// Arguments:
//  - user: The authenticated user.
//  - req: The http request.
//
// Form arguments
//  - params: The encrypted parameters
//     - album: A JSON-encoded album object.
//     - memberUserId: The user ID to remove.
//
// Returns:
//  - StingleResponse(ok)
func (s *Server) handleRemoveAlbumMember(user database.User, req *http.Request) *StingleResponse {
	params, err := s.decodeParams(req.PostFormValue("params"), user)
	if err != nil {
		log.Errorf("decodeParams: %v", err)
		return NewResponse("nok")
	}

	album, err := s.parseAlbumJSON([]byte(params["album"]))
	if err != nil {
		return NewResponse("nok")
	}
	memberID := int(parseInt(params["memberUserId"], 0))

	albumSpec, err := s.db.Album(user, album.AlbumID)
	if err != nil {
		log.Errorf("db.Album(%q, %q) failed: %v", user.Email, album.AlbumID, err)
		return NewResponse("nok")
	}
	if albumSpec.OwnerID != user.UserID {
		return NewResponse("nok").AddError("You are not the owner of the album")
	}

	if err := s.db.RemoveAlbumMember(user, album.AlbumID, memberID); err != nil {
		log.Errorf("RemoveAlbumMember(%q, %q): %v", album.AlbumID, memberID, err)
		return NewResponse("nok")
	}
	return NewResponse("ok")
}

// handleUnshareAlbum handles the /v2/sync/unshareAlbum endpoint. It is used to
// stop sharing an album.
//
// Arguments:
//  - user: The authenticated user.
//  - req: The http request.
//
// Form arguments
//  - params: The encrypted parameters
//     - albumId: The ID of the album to stop sharing.
//
// Returns:
//  - StingleResponse(ok)
func (s *Server) handleUnshareAlbum(user database.User, req *http.Request) *StingleResponse {
	params, err := s.decodeParams(req.PostFormValue("params"), user)
	if err != nil {
		log.Errorf("decodeParams: %v", err)
		return NewResponse("nok")
	}

	albumID := params["albumId"]

	albumSpec, err := s.db.Album(user, albumID)
	if err != nil {
		log.Errorf("db.Album(%q, %q) failed: %v", user.Email, albumID, err)
		return NewResponse("nok")
	}
	if albumSpec.OwnerID != user.UserID {
		return NewResponse("nok").AddError("You are not the owner of the album")
	}

	if err := s.db.UnshareAlbum(user, albumID); err != nil {
		log.Errorf("UnshareAlbum(%q): %v", albumID, err)
		return NewResponse("nok")
	}
	return NewResponse("ok")
}

// handleLeaveAlbum handles the /v2/sync/leaveAlbum endpoint. It is used to
// remove oneself from an album that was shared.
//
// Arguments:
//  - user: The authenticated user.
//  - req: The http request.
//
// Form arguments
//  - params: The encrypted parameters
//     - albumId: The ID of the album to leave.
//
// Returns:
//  - StingleResponse(ok)
func (s *Server) handleLeaveAlbum(user database.User, req *http.Request) *StingleResponse {
	params, err := s.decodeParams(req.PostFormValue("params"), user)
	if err != nil {
		log.Errorf("decodeParams: %v", err)
		return NewResponse("nok")
	}
	albumID := params["albumId"]

	albumSpec, err := s.db.Album(user, albumID)
	if err != nil {
		log.Errorf("db.Album(%q, %q) failed: %v", user.Email, albumID, err)
		return NewResponse("nok")
	}
	if albumSpec.OwnerID == user.UserID {
		return NewResponse("nok").AddError("You can't leave your own album")
	}
	if !albumSpec.Members[user.UserID] {
		return NewResponse("nok").AddError("You are not a member of this album")
	}

	if err := s.db.RemoveAlbumMember(user, albumID, user.UserID); err != nil {
		log.Errorf("RemoveAlbumMember(%q, %q): %v", albumID, user.UserID, err)
		return NewResponse("nok")
	}
	return NewResponse("ok")
}
