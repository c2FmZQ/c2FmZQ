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
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"time"

	"c2FmZQ/internal/database"
	"c2FmZQ/internal/log"
	"c2FmZQ/internal/stingle"
	"c2FmZQ/internal/webauthn"
)

// handleWebAuthnRegister handles the /v2x/config/webauthn/register endpoint.
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
func (s *Server) handleWebAuthnRegister(user database.User, req *http.Request) *stingle.Response {
	var passKey bool
	var keyName string
	var discoverable bool
	var clientDataJSON string
	var attestationObject string
	var transports []string
	if v := req.PostFormValue("params"); v != "" {
		params, err := s.decodeParams(v, user)
		if err != nil {
			log.Errorf("decodeParams: %v", err)
			return stingle.ResponseNOK()
		}
		passKey = params["passKey"] == "1"
		keyName = params["keyName"]
		discoverable = params["discoverable"] == "1"
		clientDataJSON = params["clientDataJSON"]
		attestationObject = params["attestationObject"]
		if tr := params["transports"]; tr != "" {
			if err := json.Unmarshal([]byte(tr), &transports); err != nil {
				log.Errorf("json.Unmarshal: %v", err)
				return stingle.ResponseNOK()
			}
		}
	}
	if user.NeedApproval && (clientDataJSON != "" || attestationObject != "") {
		return stingle.ResponseNOK().
			AddError("Account is not approved yet")
	}

	// This is implementing the registration of new credentials as described
	// in https://w3c.github.io/webauthn/#sctn-registering-a-new-credential
	//
	// Most of the steps are implemented in the webauthn package. Generally,
	// speaking, we don't care what kind of device the user has, or whether
	// it is attested by some third party. Self-attestation is OK.
	var opts *webauthn.AttestationOptions
	if err := s.db.MutateUser(user.UserID, func(user *database.User) error {
		if attestationObject == "" {
			var err error
			if opts, err = webauthn.NewAttestationOptions(); err != nil {
				return err
			}
			opts.RelyingParty.Name = "c2FmZQ"
			if id := user.WebAuthnConfig.UserID; id == "" {
				id := make([]byte, 32)
				if _, err := rand.Read(id); err != nil {
					return err
				}
				user.WebAuthnConfig.UserID = base64.RawURLEncoding.EncodeToString(id)
			}
			opts.User.ID = user.WebAuthnConfig.UserID
			opts.User.Name = user.Email
			opts.User.DisplayName = user.Email
			for _, key := range user.WebAuthnConfig.Keys {
				opts.ExcludeCredentials = append(opts.ExcludeCredentials, webauthn.CredentialID{
					Type:       "public-key",
					ID:         key.ID,
					Transports: key.Transports,
				})
			}
			user.WebAuthnConfig.AddChallenge(opts.Challenge)
			if passKey {
				opts.AuthenticatorSelection.UserVerification = "required"
				opts.AuthenticatorSelection.RequireResidentKey = true
			}
			return nil
		}
		rawClientDataJSON, err := base64.RawURLEncoding.DecodeString(clientDataJSON)
		if err != nil {
			return err
		}
		cd, err := webauthn.ParseClientData(rawClientDataJSON)
		if err != nil {
			return err
		}
		if cd.Type != "webauthn.create" {
			return errors.New("unexpected clientData.type")
		}
		if !user.WebAuthnConfig.CheckChallenge(cd.Challenge) {
			return errors.New("unexpected clientData.challenge")
		}
		rawAttestationObject, err := base64.RawURLEncoding.DecodeString(attestationObject)
		if err != nil {
			return err
		}
		ao, err := webauthn.ParseAttestationObject(rawAttestationObject, rawClientDataJSON)
		if err != nil {
			return err
		}
		if !ao.AuthData.UserPresence {
			return errors.New("user presence is false")
		}
		creds := ao.AuthData.AttestedCredentials
		if creds == nil {
			return errors.New("no attested credentials")
		}
		if user.WebAuthnConfig.Keys == nil {
			user.WebAuthnConfig.Keys = make(map[string]*database.WebAuthnKey)
		}
		if keyName == "" {
			keyName = creds.ID
		}
		now := time.Now().UTC()
		user.WebAuthnConfig.Keys[creds.ID] = &database.WebAuthnKey{
			Name:           keyName,
			ID:             creds.ID,
			PublicKey:      creds.COSEKey,
			RPIDHash:       ao.AuthData.RPIDHash,
			SignCount:      ao.AuthData.SignCount,
			BackupEligible: ao.AuthData.BackupEligible,
			BackupState:    ao.AuthData.BackupState,
			Transports:     transports,
			Discoverable:   discoverable,
			CreatedAt:      now,
			LastSeen:       now,
		}
		return nil
	}); err != nil {
		log.Errorf("MutateUser: %v", err)
		return stingle.ResponseNOK()
	}
	resp := stingle.ResponseOK()
	if opts != nil {
		resp.AddPart("attestationOptions", opts)
	} else {
		resp.AddInfo("Security device registered")
	}
	return resp
}

// handleWebAuthnKeys handles the /v2x/config/webauthn/keys endpoint.
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
func (s *Server) handleWebAuthnKeys(user database.User, req *http.Request) *stingle.Response {
	type key struct {
		Name         string `json:"name"`
		ID           string `json:"id"`
		Discoverable bool   `json:"discoverable"`
		CreatedAt    int64  `json:"createdAt"`
		LastSeen     int64  `json:"lastSeen"`
	}
	keys := make([]key, 0, len(user.WebAuthnConfig.Keys))
	if user.WebAuthnConfig != nil {
		for _, k := range user.WebAuthnConfig.Keys {
			keys = append(keys, key{
				Name:         k.Name,
				ID:           k.ID,
				Discoverable: k.Discoverable,
				CreatedAt:    k.CreatedAt.UnixMilli(),
				LastSeen:     k.LastSeen.UnixMilli(),
			})
		}
		sort.Slice(keys, func(i, j int) bool {
			return keys[i].LastSeen > keys[j].LastSeen
		})
	}
	return stingle.ResponseOK().
		AddPart("keys", keys)
}

// handleWebAuthnUpdateKeys handles the /v2x/config/webauthn/updateKeys endpoint.
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
func (s *Server) handleWebAuthnUpdateKeys(user database.User, req *http.Request) *stingle.Response {
	params, err := s.decodeParams(req.PostFormValue("params"), user)
	if err != nil {
		log.Errorf("decodeParams: %v", err)
		return stingle.ResponseNOK()
	}
	var updates []struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Deleted bool   `json:"deleted"`
	}
	if err := json.Unmarshal([]byte(params["updates"]), &updates); err != nil {
		log.Errorf("handleWebAuthnUpdateKeys: %v", err)
		return stingle.ResponseNOK()
	}
	if user.WebAuthnConfig == nil {
		return stingle.ResponseNOK()
	}
	if err := s.db.MutateUser(user.UserID, func(user *database.User) error {
		for _, u := range updates {
			key, ok := user.WebAuthnConfig.Keys[u.ID]
			if !ok {
				continue
			}
			if u.Deleted {
				delete(user.WebAuthnConfig.Keys, u.ID)
				continue
			}
			key.Name = u.Name
		}
		if user.RequireMFA && !mfaAvailableForUser(*user) {
			return errors.New("no MFA method left")
		}
		return nil
	}); err != nil {
		log.Errorf("MutateUser: %v", err)
		return stingle.ResponseNOK()
	}
	return stingle.ResponseOK().
		AddInfo("Security devices updated")
}
