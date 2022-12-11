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
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode"

	"golang.org/x/crypto/bcrypt"

	"c2FmZQ/internal/database"
	"c2FmZQ/internal/log"
	"c2FmZQ/internal/stingle"
	"c2FmZQ/internal/stingle/token"
)

const (
	// Login tokens are good for 180 days.
	tokenDuration = 180 * 24 * time.Hour
)

// handleCreateAccount handles the /v2/register/createAccount endpoint.
//
// Argument:
//   - req: The http request.
//
// The form arguments:
//   - email:     The email address to use for the account.
//   - password:  The hashed password.
//   - salt:      The salt used to hash the password.
//   - keyBundle: A binary representation of the public and (optionally) encrypted
//     secret keys of the user.
//   - isBackup:  Whether the user's secret key is included in the keyBundle.
//
// Returns:
//   - stingle.Response(ok)
func (s *Server) handleCreateAccount(req *http.Request) *stingle.Response {
	defer time.Sleep(time.Duration(time.Now().UnixNano()%200) * time.Millisecond)
	pk, _, err := stingle.DecodeKeyBundle(req.PostFormValue("keyBundle"))
	if err != nil {
		return stingle.ResponseNOK()
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(req.PostFormValue("password")), 12)
	if err != nil {
		log.Errorf("bcrypt.GenerateFromPassword: %v", err)
		return stingle.ResponseNOK()
	}
	email := req.PostFormValue("email")
	if !validateEmail(email) {
		return stingle.ResponseNOK()
	}
	if _, err := s.db.User(email); err == nil {
		return stingle.ResponseNOK()
	}
	if !s.AllowCreateAccount {
		return stingle.ResponseNOK()
	}
	if _, err := s.db.AddUser(
		database.User{
			Email:          email,
			HashedPassword: base64.StdEncoding.EncodeToString(hashed),
			Salt:           req.PostFormValue("salt"),
			KeyBundle:      req.PostFormValue("keyBundle"),
			IsBackup:       req.PostFormValue("isBackup"),
			PublicKey:      pk,
			NeedApproval:   !s.AutoApproveNewAccounts,
		}); err != nil {
		log.Errorf("AddUser: %v", err)
		return stingle.ResponseNOK()
	}
	return stingle.ResponseOK()
}

// handlePreLogin handles the /v2/login/preLogin endpoint.
//
// Arguments:
//   - req: The http request.
//
// The form arguments:
//   - email: The email address of the account.
//
// Returns:
//   - stingle.Response(ok)
//     Part(salt, The salt used to hash the password)
func (s *Server) handlePreLogin(req *http.Request) *stingle.Response {
	defer time.Sleep(time.Duration(time.Now().UnixNano()%200) * time.Millisecond)
	email, _ := parseOTP(req.PostFormValue("email"))
	if u, err := s.db.User(email); err == nil && !u.LoginDisabled {
		return stingle.ResponseOK().AddPart("salt", u.Salt)
	}
	if v, ok := s.preLoginCache.Get(email); ok {
		return stingle.ResponseOK().AddPart("salt", v.(string))
	}
	fakeSalt := make([]byte, 16)
	if _, err := rand.Read(fakeSalt); err != nil {
		return stingle.ResponseNOK()
	}
	v := strings.ToUpper(hex.EncodeToString(fakeSalt))
	s.preLoginCache.Add(email, v)
	return stingle.ResponseOK().AddPart("salt", v)
}

// handleLogin handles the /v2/login/login endpoint.
//
// Argument:
//   - req: The http request.
//
// The form arguments:
//   - email: The email address of the account.
//   - password: The hashed password.
//
// Returns:
//   - stingle.Response(ok)
//     Part(userId, The numeric ID of the account)
//     Part(keyBundle, The encoded keys of the user)
//     Part(serverPublicKey, The server's public key that is associated with this account)
//     Part(token, The session token signed by the server)
//     Part(isKeyBackedUp, Whether the user's secret key is in keyBundle)
//     Part(homeFolder, A "Home folder" used on the app's device)
func (s *Server) handleLogin(req *http.Request) *stingle.Response {
	email, _ := parseOTP(req.PostFormValue("email"))
	pass := req.PostFormValue("password")
	u, err := s.db.User(email)
	if err != nil {
		return stingle.ResponseNOK().AddError("Invalid credentials")
	}
	if u.LoginDisabled {
		return stingle.ResponseNOK().AddError("Invalid credentials")
	}
	var mfaFailed bool
	if u.RequireMFA {
		resp, redirect := s.requireMFA(&u, req, time.Duration(0))
		if resp != nil && redirect {
			return resp
		}
		mfaFailed = resp != nil
	}
	hashed, err := base64.StdEncoding.DecodeString(u.HashedPassword)
	if err != nil {
		log.Errorf("base64.StdEncoding.DecodeString: %v", err)
		return stingle.ResponseNOK().AddError("Invalid credentials")
	}
	pwCh := make(chan bool)
	decoyCh := make(chan *database.User)
	go func() { pwCh <- bcrypt.CompareHashAndPassword(hashed, []byte(pass)) == nil }()
	go func() { decoyCh <- s.decoyLogin(u, pass) }()
	pwOK := <-pwCh
	decoyUser := <-decoyCh

	log.Debugf("UserID:%d pwOK:%v", u.UserID, pwOK)
	if !pwOK || mfaFailed {
		if decoyUser == nil {
			return stingle.ResponseNOK().AddError("Invalid credentials")
		}
		u = *decoyUser
	}
	tk, err := s.db.DecryptTokenKey(u.TokenKey)
	if err != nil {
		return stingle.ResponseNOK()
	}
	defer tk.Wipe()
	tok := token.Mint(tk, token.Token{Scope: "session", Subject: u.UserID}, tokenDuration)
	if err := s.db.MutateUser(u.UserID, func(u *database.User) error {
		u.ValidTokens[token.Hash(tok)] = true
		return nil
	}); err != nil {
		log.Errorf("MutateUser: %v", err)
		return stingle.ResponseNOK()
	}
	resp := stingle.ResponseOK().
		AddPart("keyBundle", u.KeyBundle).
		AddPart("serverPublicKey", u.ServerPublicKeyForExport()).
		AddPart("token", tok).
		AddPart("userId", fmt.Sprintf("%d", u.UserID)).
		AddPart("isKeyBackedUp", u.IsBackup).
		AddPart("homeFolder", u.HomeFolder)
	if u.RequireMFA {
		resp.AddPart("_mfaEnabled", "1")
	}
	if u.WebAuthnConfig.UsePasskey {
		resp.AddPart("_passKey", "1")
	}
	if u.OTPKey != "" {
		resp.AddPart("_otpEnabled", "1")
	}
	if u.Admin {
		resp.AddPart("_admin", "1")
	}
	if u.NeedApproval {
		resp.AddInfo("Your account hasn't been approved yet. Some features are disabled.")
	}
	return resp
}

func (s *Server) decoyLogin(user database.User, hash string) *database.User {
	salt, err := hex.DecodeString(user.Salt)
	if err != nil {
		return nil
	}
	ch := make(chan int64)
	var wg sync.WaitGroup
	for _, decoy := range user.Decoys {
		wg.Add(1)
		go func(decoy *database.Decoy) {
			defer wg.Done()
			pw, err := s.db.Decrypt(decoy.Password)
			if err != nil {
				log.Errorf("Decrypt: %v", err)
				return
			}
			if stingle.PasswordHashForLogin(pw, salt) == hash {
				ch <- decoy.UserID
			}
		}(decoy)
	}
	go func() {
		wg.Wait()
		close(ch)
	}()
	var uid int64
	for uid = range ch {
		continue
	}
	if uid == 0 {
		return nil
	}
	u, err := s.db.UserByID(uid)
	if err != nil || !u.LoginDisabled {
		return nil
	}
	return &u
}

// handleLogout handles the /v2/login/logout endpoint.
//
// Arguments:
//   - user: The authenticated user.
//   - req: The http request.
//
// Returns:
//   - StringleResponse(ok)
func (s *Server) handleLogout(user database.User, req *http.Request) *stingle.Response {
	if err := s.db.MutateUser(user.UserID, func(user *database.User) error {
		delete(user.ValidTokens, token.Hash(req.PostFormValue("token")))
		return nil
	}); err != nil {
		log.Errorf("MutateUser: %v", err)
		return stingle.ResponseNOK()
	}
	return stingle.ResponseOK().AddPart("logout", "1")
}

// handleChangePass handles the /v2/login/changePass endpoint.
//
// Arguments:
//   - user: The authenticated user.
//   - req: The http request.
//
// Form arguments:
//   - params - Encrypted parameters:
//   - newPassword: The new hashed password.
//   - newSalt: The salt used to hash the new password.
//   - keyBundle: The new keyBundle.
//
// Returns:
//   - stingle.Response(ok)
//     Part(token, A new signed session token)
func (s *Server) handleChangePass(user database.User, req *http.Request) *stingle.Response {
	if user.LoginDisabled {
		// Changing the password of a decoy account doesn't work.
		return stingle.ResponseNOK()
	}
	params, err := s.decodeParams(req.PostFormValue("params"), user)
	if err != nil {
		log.Errorf("decodeParams: %v", err)
		return stingle.ResponseNOK()
	}

	var tok string
	if err := s.db.MutateUser(user.UserID, func(user *database.User) error {
		hashed, err := bcrypt.GenerateFromPassword([]byte(params["newPassword"]), 12)
		if err != nil {
			log.Errorf("bcrypt.GenerateFromPassword: %v", err)
			return err
		}
		user.HashedPassword = base64.StdEncoding.EncodeToString(hashed)
		user.Salt = params["newSalt"]
		user.KeyBundle = params["keyBundle"]
		etk, err := s.db.NewEncryptedTokenKey()
		if err != nil {
			log.Errorf("NewEncryptedTokenKey: %v", err)
			return err
		}
		user.TokenKey = etk
		pk, hasSK, err := stingle.DecodeKeyBundle(user.KeyBundle)
		if err != nil {
			log.Errorf("DecodeKeyBundle: %v", err)
			return err
		}
		user.PublicKey = pk
		if hasSK {
			user.IsBackup = "1"
		} else {
			user.IsBackup = "0"
		}
		tk, err := s.db.DecryptTokenKey(user.TokenKey)
		if err != nil {
			log.Errorf("DecryptTokenKey: %v", err)
			return err
		}
		defer tk.Wipe()
		tok = token.Mint(tk, token.Token{Scope: "session", Subject: user.UserID}, tokenDuration)
		user.ValidTokens = map[string]bool{token.Hash(tok): true}
		return nil
	}); err != nil {
		log.Errorf("MutateUser: %v", err)
		return stingle.ResponseNOK()
	}
	return stingle.ResponseOK().
		AddPart("token", tok).
		AddInfo("Password updated")

}

// handleGetServerPK handles the /v2/keys/getServerPK endpoint. The server's
// public key is used to encrypt the "params" arguments.
//
// Arguments:
//   - user: The authenticated user.
//   - req: The http request.
//
// Returns:
//   - stingle.Response(ok)
//     Part(serverPK, server's public key)
func (s *Server) handleGetServerPK(user database.User, req *http.Request) *stingle.Response {
	return stingle.ResponseOK().AddPart("serverPK", user.ServerPublicKeyForExport())
}

// handleCheckKey handles the /v2/login/checkKey endpoint. This is part of the
// password recovery flow. The user has to enter their secret "passphrase" in
// the app, and the app uses this endpoint to verify that the key/passphrase is
// correct.
//
// Argument:
//   - req: The http request.
//
// Form arguments:
//   - email: The email address of the account.
//
// Returns:
//   - stingle.Response(ok)
//     Part(challenge, A message that can only be read with the right secret key)
//     Part(isKeyBackedUp, Whether the encrypted secret of the user in on the server)
//     Part(serverPK, The public key of the server associated with this account)
func (s *Server) handleCheckKey(req *http.Request) *stingle.Response {
	defer time.Sleep(time.Duration(time.Now().UnixNano()%200) * time.Millisecond)
	email := req.PostFormValue("email")
	rnd := make([]byte, 64)
	if _, err := rand.Read(rnd); err != nil {
		return stingle.ResponseNOK()
	}
	var (
		isBackup string
		pk       stingle.PublicKey
		serverPK stingle.PublicKey
	)
	if u, err := s.db.User(email); err == nil {
		pk = u.PublicKey
		serverPK = u.ServerPublicKey
		isBackup = u.IsBackup
		if u.RequireMFA {
			resp, _ := s.requireMFA(&u, req, time.Duration(0))
			if resp != nil {
				return resp
			}
		}
	} else {
		isBackup = "1"
		pk = stingle.PublicKeyFromBytes(rnd[:32])
		if v, ok := s.checkKeyCache.Get(email); ok {
			serverPK = v.(stingle.PublicKey)
		} else {
			serverPK = stingle.PublicKeyFromBytes(rnd[32:])
			s.checkKeyCache.Add(email, serverPK)
		}
	}
	return stingle.ResponseOK().
		AddPart("challenge", pk.SealBox(append([]byte("validkey_"), rnd[:16]...))).
		AddPart("isKeyBackedUp", isBackup).
		AddPart("serverPK", base64.StdEncoding.EncodeToString(serverPK.ToBytes()))
}

// handleRecoverAccount handles the /v2/login/recoverAccount endpoint, which
// is pretty much same as /v2/login/changePass.
// Form arguments:
//
// Argument:
//   - req: The http request.
//
// Form arguments:
//   - email: The email address of the account.
//   - params - Encrypted parameters:
//   - newPassword: The new hashed password.
//   - newSalt: The salt used to hash the new password.
//   - keyBundle: The new keyBundle.
//
// Returns:
//   - stingle.Response(ok)
//     Part(result, OK)
func (s *Server) handleRecoverAccount(req *http.Request) *stingle.Response {
	defer time.Sleep(time.Duration(time.Now().UnixNano()%200) * time.Millisecond)
	email := req.PostFormValue("email")
	user, err := s.db.User(email)
	if err != nil {
		return stingle.ResponseNOK()
	}
	if user.LoginDisabled {
		return stingle.ResponseNOK()
	}
	if user.RequireMFA {
		resp, _ := s.requireMFA(&user, req, time.Duration(0))
		if resp != nil {
			return resp
		}
	}
	params, err := s.decodeParams(req.PostFormValue("params"), user)
	if err != nil {
		log.Errorf("decodeParams: %v", err)
		return stingle.ResponseNOK()
	}

	if err := s.db.MutateUser(user.UserID, func(user *database.User) error {
		hashed, err := bcrypt.GenerateFromPassword([]byte(params["newPassword"]), 12)
		if err != nil {
			log.Errorf("bcrypt.GenerateFromPassword: %v", err)
			return err
		}
		user.HashedPassword = base64.StdEncoding.EncodeToString(hashed)
		user.Salt = params["newSalt"]
		user.KeyBundle = params["keyBundle"]
		etk, err := s.db.NewEncryptedTokenKey()
		if err != nil {
			log.Errorf("s.db.NewEncryptedTokenKey: %v", err)
			return err
		}
		user.TokenKey = etk
		pk, hasSK, err := stingle.DecodeKeyBundle(user.KeyBundle)
		if err != nil {
			log.Errorf("DecodeKeyBundle: %v", err)
			return err
		}
		user.PublicKey = pk
		if hasSK {
			user.IsBackup = "1"
		} else {
			user.IsBackup = "0"
		}
		return nil
	}); err != nil {
		log.Errorf("MutateUser: %v", err)
		return stingle.ResponseNOK()
	}
	return stingle.ResponseOK().AddPart("result", "OK")
}

// handleDeleteUser handles the /v2/login/deleteUser endpoint. It is used
// to delete the user's account, but it is not currently implemented.
//
// Arguments:
//   - user: The authenticated user.
//   - req: The http request.
//
// Form arguments:
//   - token: The signed session token.
//   - params: Encrypted parameters:
//   - password: The user's hashed password.
//
// Returns:
//   - stingle.Response(ok)
func (s *Server) handleDeleteUser(user database.User, req *http.Request) *stingle.Response {
	params, err := s.decodeParams(req.PostFormValue("params"), user)
	if err != nil {
		log.Errorf("decodeParams: %v", err)
		return stingle.ResponseNOK()
	}
	pass := params["password"]
	hashed, err := base64.StdEncoding.DecodeString(user.HashedPassword)
	if err != nil {
		log.Errorf("base64.StdEncoding.DecodeString: %v", err)
		return stingle.ResponseNOK().AddError("Invalid credentials")
	}
	if err != nil || bcrypt.CompareHashAndPassword(hashed, []byte(pass)) != nil {
		return stingle.ResponseNOK().AddError("Invalid credentials")
	}
	if err := s.db.DeleteUser(user); err != nil {
		log.Errorf("DeleteUser: %v", err)
		return stingle.ResponseNOK()
	}
	return stingle.ResponseOK()
}

// handleChangeEmail handles the /v2/login/changeEmail endpoint.
//
// Arguments:
//   - user: The authenticated user.
//   - req: The http request.
//
// Form arguments:
//   - token: The signed session token.
//   - params: Encrypted parameters:
//   - newEmail: The new email address.
//
// Returns:
//   - stingle.Response(ok)
func (s *Server) handleChangeEmail(user database.User, req *http.Request) *stingle.Response {
	if user.LoginDisabled {
		// Changing the email of a decoy account doesn't work.
		return stingle.ResponseNOK()
	}
	params, err := s.decodeParams(req.PostFormValue("params"), user)
	if err != nil {
		log.Errorf("decodeParams: %v", err)
		return stingle.ResponseNOK()
	}
	newEmail := params["newEmail"]
	if !validateEmail(newEmail) {
		return stingle.ResponseNOK()
	}
	if err := s.db.RenameUser(user.UserID, newEmail); err != nil {
		log.Errorf("RenameUser: %v", err)
		return stingle.ResponseNOK()
	}
	return stingle.ResponseOK().
		AddPart("email", newEmail).
		AddInfo("Email updated")
}

// handleReuploadKeys handles the /v2/keys/reuploadKeys endpoint. It is used
// when the user changes the "Backup my keys" setting.
//
// Arguments:
//   - user: The authenticated user.
//   - req: The http request.
//
// Form arguments:
//   - token: The signed session token.
//   - params: Encrypted parameters:
//   - keyBundle: The new keyBundle.
//
// Returns:
//   - stingle.Response(ok)
func (s *Server) handleReuploadKeys(user database.User, req *http.Request) *stingle.Response {
	params, err := s.decodeParams(req.PostFormValue("params"), user)
	if err != nil {
		log.Errorf("decodeParams: %v", err)
		return stingle.ResponseNOK()
	}
	if err := s.db.MutateUser(user.UserID, func(user *database.User) error {
		user.KeyBundle = params["keyBundle"]
		pk, hasSK, err := stingle.DecodeKeyBundle(user.KeyBundle)
		if err != nil {
			log.Errorf("DecodeKeyBundle: %v", err)
			return stingle.ResponseNOK()
		}
		user.PublicKey = pk
		if hasSK {
			user.IsBackup = "1"
		} else {
			user.IsBackup = "0"
		}
		return nil
	}); err != nil {
		log.Errorf("MutateUser: %v", err)
		return stingle.ResponseNOK()
	}
	return stingle.ResponseOK()
}

// parseOTP parses an email address in the form of passcode%user@domain and
// returns the email address and the passcode.
func parseOTP(v string) (email string, passcode string) {
	p := strings.SplitN(v, "%", 2)
	if len(p) == 2 {
		return p[1], p[0]
	}
	return v, ""
}

func validateEmail(email string) bool {
	return len(email) > 0 && len(email) <= 64 && email == cleanUnicode(email)
}

func cleanUnicode(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsPrint(r) {
			return r
		}
		return -1
	}, s)
}
