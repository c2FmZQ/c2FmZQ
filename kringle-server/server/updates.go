package server

import (
	"fmt"
	"net/http"

	"kringle-server/database"
	"kringle-server/log"
	"kringle-server/stingle"
)

// handleGetUpdates handles the /v2/sync/getUpdates endpoint. This is the
// mechanism by which the user learns about changes in files, albums, etc.
// Form arguments:
//  - token  - The signed session token.
//  - params - Encrypted parameters:
//     - filesST - The timestamp of the last seen changes to the Gallery.
//     - trashST - The timestamp of the last seen changes to the Trash.
//     - albumsST - The timestamp of the last seen to albums.
//     - albumFilesST - The timestamp of the last seen changes to any album
//                      files.
//     - cntST - The timestamp of the last seen changes to contacts.
//     - delST - The timestamp of the last seen delete events.
// Returns:
//  - files: unseen changes in Gallery
//  - trash: unseen changes in Trash
//  - albums: unseen changes in albums
//  - albumFiles: unseen changes in album files
//  - contacts: unseen changes in contacts
//  - deletes: unseen deletions (files, albums, contacts, etc)
//  - spacedUsed: the number of megabytes of storage used.
//  - spaceQuota: the user's quota in megabytes.
func (s *Server) handleGetUpdates(user database.User, req *http.Request) *stingle.Response {
	fileST := parseInt(req.PostFormValue("filesST"), 0)
	trashST := parseInt(req.PostFormValue("trashST"), 0)
	albumsST := parseInt(req.PostFormValue("albumsST"), 0)
	albumFilesST := parseInt(req.PostFormValue("albumFilesST"), 0)
	cntST := parseInt(req.PostFormValue("cntST"), 0)
	delST := parseInt(req.PostFormValue("delST"), 0)

	files, err := s.db.FileUpdates(user, stingle.GallerySet, fileST)
	if err != nil {
		log.Errorf("FileUpdates() failed: %v", err)
		return stingle.ResponseNOK()
	}
	trash, err := s.db.FileUpdates(user, stingle.TrashSet, trashST)
	if err != nil {
		log.Errorf("FileUpdates() failed: %v", err)
		return stingle.ResponseNOK()
	}
	albums, err := s.db.AlbumUpdates(user, albumsST)
	if err != nil {
		log.Errorf("AlbumUpdates() failed: %v", err)
		return stingle.ResponseNOK()
	}
	albumFiles, err := s.db.FileUpdates(user, stingle.AlbumSet, albumFilesST)
	if err != nil {
		log.Errorf("FileUpdates() failed: %v", err)
		return stingle.ResponseNOK()
	}
	contacts, err := s.db.ContactUpdates(user, cntST)
	if err != nil {
		log.Errorf("ContactUpdates() failed: %v", err)
		return stingle.ResponseNOK()
	}
	deletes, err := s.db.DeleteUpdates(user, delST)
	if err != nil {
		log.Errorf("DeleteUpdates() failed: %v", err)
		return stingle.ResponseNOK()
	}
	spaceUsed, err := s.db.SpaceUsed(user)
	if err != nil {
		log.Errorf("SpaceUSed() failed: %v", err)
	}
	return stingle.ResponseOK().
		AddPart("files", files).
		AddPart("trash", trash).
		AddPart("albums", albums).
		AddPart("albumFiles", albumFiles).
		AddPart("contacts", contacts).
		AddPart("deletes", deletes).
		AddPart("spaceUsed", fmt.Sprintf("%d", spaceUsed>>20)).
		AddPart("spaceQuota", "104857600") // 100 TB. Arbitrary large value.
}
