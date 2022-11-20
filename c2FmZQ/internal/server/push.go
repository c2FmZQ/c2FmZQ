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
	"net/http"

	"c2FmZQ/internal/database"
	"c2FmZQ/internal/log"
	"c2FmZQ/internal/stingle"
)

// handlePush handles the /c2/config/push endpoint.
//
// Arguments:
//   - user: The authenticated user.
//   - req: The http request.
//
// Form arguments:
//   - token: The signed session token.
//
// Returns:
//   - stingle.Response(ok)
func (s *Server) handlePush(user database.User, req *http.Request) *stingle.Response {
	var ep string
	var auth string
	var p256dh string
	if v := req.PostFormValue("params"); v != "" {
		params, err := s.decodeParams(v, user)
		if err != nil {
			log.Errorf("decodeParams: %v", err)
			return stingle.ResponseNOK()
		}
		ep = params["endpoint"]
		auth = params["auth"]
		p256dh = params["p256dh"]
	}
	if user.NeedApproval && (auth != "" || p256dh != "") {
		return stingle.ResponseNOK().
			AddError("Account is not approved yet")
	}

	if err := s.db.MutateUser(user.UserID, func(u *database.User) error {
		if pc := u.PushConfig; pc == nil {
			pc, err := database.NewPushConfig()
			if err != nil {
				log.Errorf("NewPushConfig: %v", err)
				return err
			}
			u.PushConfig = pc
		}
		if ep != "" {
			_, exists := u.PushConfig.Endpoints[ep]
			if exists && auth == "" {
				delete(u.PushConfig.Endpoints, ep)
			}
			if auth != "" && p256dh != "" {
				if u.PushConfig.Endpoints == nil {
					u.PushConfig.Endpoints = make(map[string]*database.EndpointData)
				}
				u.PushConfig.Endpoints[ep] = &database.EndpointData{
					Auth:   auth,
					P256dh: p256dh,
				}
				if err := s.db.TestPushEndpoint(*u, ep); err != nil {
					log.Errorf("TestPushEndpoint: %v", err)
					return err
				}
			}
		}
		user = *u
		return nil
	}); err != nil {
		log.Errorf("MutateUser: %v", err)
		return stingle.ResponseNOK()
	}
	return stingle.ResponseOK().
		AddPart("applicationServerKey", user.PushConfig.ApplicationServerPublicKey)
}
