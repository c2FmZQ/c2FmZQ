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

// handleAdminUsers handles the /c2/admin/users endpoint.
//
// Arguments:
//  - user: The authenticated user.
//  - req: The http request.
//
// Form arguments:
//  - token: The signed session token.
//  - params: The encrypted parameters
//     - changes: changes to apply
//
// Returns:
//  - stingle.Response(ok)
//        Parts("users", encrypted list of user data)
func (s *Server) handleAdminUsers(user database.User, req *http.Request) *stingle.Response {
	if !user.Admin {
		return stingle.ResponseNOK()
	}
	data, err := s.db.AdminData(nil)
	if err != nil {
		log.Errorf("AdminData: %v", err)
		return stingle.ResponseNOK()
	}
	params, err := s.decodeParams(req.PostFormValue("params"), user)
	if err != nil {
		log.Errorf("decodeParams: %v", err)
		return stingle.ResponseNOK()
	}
	if v, ok := params["changes"]; ok {
		var changes database.AdminData
		if err := json.Unmarshal([]byte(v), &changes); err != nil {
			log.Errorf("json.Unmarshal: %v", err)
			return stingle.ResponseNOK()
		}
		data, err = s.db.AdminData(&changes)
		if err == database.ErrOutdated {
			return stingle.ResponseNOK().AddError("Data outdated")
		}
		if err != nil {
			log.Errorf("AdminData: %v", err)
			return stingle.ResponseNOK()
		}
	}
	b, err := json.Marshal(data)
	if err != nil {
		log.Errorf("json.Marshal: %v", err)
		return stingle.ResponseNOK()
	}
	return stingle.ResponseOK().
		AddPart("users", user.PublicKey.SealBox(b))
}
