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
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"c2FmZQ/internal/database"
	"c2FmZQ/internal/log"
	"c2FmZQ/internal/stingle"
	"c2FmZQ/internal/stingle/token"
	"c2FmZQ/internal/webauthn"
)

// handleEnableMFA handles the /v2x/mfa/enable endpoint.
//
// Arguments:
//   - user: The authenticated user.
//   - req: The http request.
//
// Form arguments:
//   - token: The signed session token.
//   - params: Encrypted parameters:
//   - requireMFA: whether MFA is required
//
// Returns:
//   - stingle.Response(ok)
func (s *Server) handleEnableMFA(user database.User, req *http.Request) *stingle.Response {
	params, err := s.decodeParams(req.PostFormValue("params"), user)
	if err != nil {
		log.Errorf("decodeParams: %v", err)
		return stingle.ResponseNOK()
	}
	requireMFA := params["requireMFA"] == "1"
	passKey := params["passKey"] == "1"

	if !user.RequireMFA {
		user.WebAuthnConfig.UsePasskey = passKey
	}
	if resp, _ := s.requireMFA(&user, req, time.Duration(0)); resp != nil {
		return resp
	}

	if err := s.db.MutateUser(user.UserID, func(user *database.User) error {
		user.RequireMFA = requireMFA
		user.WebAuthnConfig.UsePasskey = passKey
		if user.RequireMFA && !mfaAvailableForUser(*user) {
			return errors.New("no MFA method")
		}
		return nil
	}); err != nil {
		log.Errorf("MutateUser: %v", err)
		return stingle.ResponseNOK()
	}
	resp := stingle.ResponseOK()
	if requireMFA {
		resp.AddInfo("MFA enabled")
	} else {
		resp.AddInfo("MFA disabled")
	}
	return resp
}

func mfaAvailableForUser(user database.User) bool {
	if user.OTPKey != "" {
		return true
	}
	if !user.WebAuthnConfig.UsePasskey && len(user.WebAuthnConfig.Keys) > 0 {
		return true
	}
	for _, k := range user.WebAuthnConfig.Keys {
		if k.Discoverable {
			return true
		}
	}
	return false
}

func (s *Server) tryRemoteMFA(ctx context.Context, user database.User) error {
	tok := make([]byte, 16)
	if _, err := rand.Read(tok); err != nil {
		log.Errorf("rand.Read: %v", err)
		return err
	}
	session := base64.RawURLEncoding.EncodeToString(tok)
	if err := s.db.RequestMFA(user, session); err != nil {
		return err
	}
	ch := make(chan struct{})
	s.remoteMFAMutex.Lock()
	s.remoteMFA[session] = remoteMFAReq{
		ch:     ch,
		userID: user.UserID,
	}
	s.remoteMFAMutex.Unlock()
	defer func() {
		s.remoteMFAMutex.Lock()
		defer s.remoteMFAMutex.Unlock()
		delete(s.remoteMFA, session)
	}()
	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(2 * time.Minute):
		return errors.New("timeout")
	}
}

// handleApproveMFA handles the /v2x/mfa/approve endpoint.
//
// Arguments:
//   - user: The authenticated user.
//   - req: The http request.
//
// Form arguments:
//   - token: The signed session token.
//   - params: Encrypted parameters:
//   - requireMFA: whether MFA is required
//
// Returns:
//   - stingle.Response(ok)
func (s *Server) handleApproveMFA(user database.User, req *http.Request) *stingle.Response {
	params, err := s.decodeParams(req.PostFormValue("params"), user)
	if err != nil {
		log.Errorf("decodeParams: %v", err)
		return stingle.ResponseNOK()
	}
	session := params["session"]
	s.remoteMFAMutex.Lock()
	defer s.remoteMFAMutex.Unlock()
	if r, ok := s.remoteMFA[session]; ok {
		if user.UserID == r.userID {
			close(r.ch)
			delete(s.remoteMFA, session)
		}
	}
	return stingle.ResponseOK()
}

func (s *Server) requireMFA(user *database.User, req *http.Request, gracePeriod time.Duration) (*stingle.Response, bool) {
	if _, passcode := parseOTP(req.PostFormValue("email")); passcode != "" {
		if !validateOTP(user.OTPKey, passcode) {
			return stingle.ResponseNOK(), false
		}
		return nil, false
	}
	tokHash := token.Hash(req.PostFormValue("token"))
	if user.WebAuthnConfig.LastAuthTimes[tokHash].Add(gracePeriod).After(time.Now()) {
		return nil, false
	}

	if mfa := req.PostFormValue("mfa"); mfa != "" {
		return s.checkMFAResponse(user, req), false
	}

	if c := req.Header.Get("X-c2FmZQ-capabilities"); !strings.Contains(c, "mfa") {
		ctx := req.Context()
		s.setDeadline(ctx, time.Now().Add(3*time.Minute))
		if err := s.tryRemoteMFA(ctx, *user); err != nil {
			log.Errorf("tryRemoteMFA: %v", err)
			return stingle.ResponseNOK(), false
		}
		return nil, false
	}
	var opts *webauthn.AssertionOptions
	if len(user.WebAuthnConfig.Keys) > 0 {
		var err error
		if opts, err = webauthn.NewAssertionOptions(); err != nil {
			log.Errorf("webauthn.NewAssertionOptions: %v", err)
			return stingle.ResponseNOK(), false
		}
		if user.WebAuthnConfig.UsePasskey {
			opts.UserVerification = "required"
			opts.AllowCredentials = make([]webauthn.CredentialID, 0)
		} else {
			for _, key := range user.WebAuthnConfig.Keys {
				opts.AllowCredentials = append(opts.AllowCredentials, webauthn.CredentialID{
					Type:       "public-key",
					ID:         key.ID,
					Transports: key.Transports,
				})
			}
			sort.Slice(opts.AllowCredentials, func(i, j int) bool {
				a := user.WebAuthnConfig.Keys[opts.AllowCredentials[i].ID]
				b := user.WebAuthnConfig.Keys[opts.AllowCredentials[j].ID]
				return a.LastSeen.After(b.LastSeen)
			})
		}
		if err := s.db.MutateUser(user.UserID, func(u *database.User) error {
			u.WebAuthnConfig.AddChallenge(opts.Challenge)
			*user = *u
			return nil
		}); err != nil {
			log.Errorf("MutateUser: %v", err)
			return stingle.ResponseNOK(), false
		}
	}
	return stingle.ResponseNOK().
		AddPart("mfa", struct {
			Options *webauthn.AssertionOptions `json:"webauthn"`
		}{opts}), true
}

func (s *Server) checkMFAResponse(user *database.User, req *http.Request) *stingle.Response {
	failResp := stingle.ResponseNOK().AddError("MFA failed")

	var data struct {
		OTP      string `json:"otp"`
		WebAuthn struct {
			ID                string `json:"id"`
			ClientDataJSON    string `json:"clientDataJSON"`
			AuthenticatorData string `json:"authenticatorData"`
			Signature         string `json:"signature"`
			UserHandle        string `json:"userHandle"`
		} `json:"webauthn"`
	}
	if err := json.Unmarshal([]byte(req.PostFormValue("mfa")), &data); err != nil {
		log.Errorf("json.Unmarshal: %q %v", req.PostFormValue("mfa"), err)
		return failResp
	}
	if data.OTP != "" {
		if !validateOTP(user.OTPKey, data.OTP) {
			log.Info("checkMFAResponse: OTP check failed")
			return failResp
		}
		return nil
	}

	if data.WebAuthn.Signature == "" {
		return failResp
	}

	// https://w3c.github.io/webauthn/#sctn-verifying-assertion
	clientDataJSON, err := base64.RawURLEncoding.DecodeString(data.WebAuthn.ClientDataJSON)
	if err != nil {
		log.Errorf("data.WebAuthn.ClientDataJSON: %v", err)
		return failResp
	}
	rawAuthData, err := base64.RawURLEncoding.DecodeString(data.WebAuthn.AuthenticatorData)
	if err != nil {
		log.Errorf("data.WebAuthn.AuthenticatorData: %v", err)
		return failResp
	}
	sig, err := base64.RawURLEncoding.DecodeString(data.WebAuthn.Signature)
	if err != nil {
		log.Errorf("data.WebAuthn.Signature: %v", err)
		return failResp
	}

	cd, err := webauthn.ParseClientData(clientDataJSON)
	if err != nil {
		log.Errorf("webauthn.ParseClientData: %v", err)
		return failResp
	}
	if cd.Type != "webauthn.get" {
		log.Error("unexpected clientData.type")
		return failResp
	}
	var authData webauthn.AuthenticatorData
	if err := webauthn.ParseAuthenticatorData(rawAuthData, &authData); err != nil {
		log.Errorf("webauthn.ParseAuthenticatorData: %v", err)
		return failResp
	}
	if !authData.UserPresence {
		log.Error("UserPresence is false")
		return failResp
	}
	if user.WebAuthnConfig.UsePasskey {
		if !authData.UserVerification {
			log.Error("UserVerification is false")
			return failResp
		}
		if data.WebAuthn.UserHandle != user.WebAuthnConfig.UserID {
			log.Errorf("userHandle mismatch %q != %q", data.WebAuthn.UserHandle, user.WebAuthnConfig.UserID)
			return failResp
		}
	}
	creds, ok := user.WebAuthnConfig.Keys[data.WebAuthn.ID]
	if !ok {
		log.Errorf("Unknown key %q", data.WebAuthn.ID)
		return failResp
	}
	if authData.RPIDHash != creds.RPIDHash {
		log.Error("rpIdHash mismatch")
		return failResp
	}
	if (authData.SignCount > 0 || creds.SignCount > 0) && authData.SignCount <= creds.SignCount {
		// Log it, but don't fail.
		log.Infof("SignCount: %d <= %d", authData.SignCount, creds.SignCount)
	}
	if err := s.db.MutateUser(user.UserID, func(u *database.User) error {
		if !u.WebAuthnConfig.CheckChallenge(cd.Challenge) {
			return errors.New("unexpected clientData.challenge")
		}
		if err := webauthn.VerifySignature(creds.PublicKey, rawAuthData, clientDataJSON, sig); err != nil {
			return err
		}
		now := time.Now().UTC()
		if creds, ok := u.WebAuthnConfig.Keys[data.WebAuthn.ID]; ok {
			creds.SignCount = authData.SignCount
			creds.LastSeen = now
			if tok := req.PostFormValue("token"); tok != "" {
				if u.WebAuthnConfig.LastAuthTimes == nil {
					u.WebAuthnConfig.LastAuthTimes = make(map[string]time.Time)
				}
				u.WebAuthnConfig.LastAuthTimes[token.Hash(tok)] = now
			}
			for k, v := range u.WebAuthnConfig.LastAuthTimes {
				if v.Add(5 * time.Minute).Before(now) {
					delete(u.WebAuthnConfig.LastAuthTimes, k)
				}
			}
		}
		*user = *u
		return nil
	}); err != nil {
		log.Errorf("MutateUser: %v", err)
		return failResp
	}
	return nil
}

// handleMFACheck handles the /v2x/mfa/check endpoint.
//
// Arguments:
//   - user: The authenticated user.
//   - req: The http request.
//
// Returns:
//   - stingle.Response(ok)
func (s *Server) handleMFACheck(user database.User, req *http.Request) *stingle.Response {
	params, err := s.decodeParams(req.PostFormValue("params"), user)
	if err != nil {
		log.Errorf("decodeParams: %v", err)
		return stingle.ResponseNOK()
	}
	user.WebAuthnConfig.UsePasskey = params["passKey"] == "1"
	if resp, _ := s.requireMFA(&user, req, time.Duration(0)); resp != nil {
		return resp
	}
	return stingle.ResponseOK().AddInfo("MFA OK")
}

// handleMFAStatus handles the /v2x/mfa/status endpoint.
//
// Arguments:
//   - user: The authenticated user.
//   - req: The http request.
//
// Returns:
//   - stingle.Response(ok)
func (s *Server) handleMFAStatus(user database.User, req *http.Request) *stingle.Response {
	return stingle.ResponseOK().
		AddPart("mfaEnabled", user.RequireMFA).
		AddPart("otpEnabled", user.OTPKey != "").
		AddPart("passKey", user.WebAuthnConfig.UsePasskey)
}
