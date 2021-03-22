package server

import (
	"crypto/rand"
	"fmt"
	"net/http"
	"time"

	"stingle-server/crypto"
	"stingle-server/database"
	"stingle-server/log"
)

// handleCreateAccount handles the /v2/register/createAccount endpoint.
//
// Argument:
//  - req: The http request.
//
// The form arguments:
//  - email:     The email address to use for the account.
//  - password:  The hashed password.
//  - salt:      The salt used to hash the password.
//  - keyBundle: A binary representation of the public and (optionally) encrypted
//               secret keys of the user.
//  - isBackup:  Whether the user's secret key is included in the keyBundle.
//
// Returns:
//  - StingleResponse(ok)
func (s *Server) handleCreateAccount(req *http.Request) *StingleResponse {
	email := req.PostFormValue("email")
	if _, err := s.db.User(email); err == nil {
		return NewResponse("nok").AddError("User already exists")
	}
	pk, err := crypto.DecodeKeyBundle(req.PostFormValue("keyBundle"))
	if err != nil {
		return NewResponse("nok").AddError("KeyBundle cannot be parsed")
	}
	if err := s.db.AddUser(
		database.User{
			Email:     email,
			Password:  req.PostFormValue("password"),
			Salt:      req.PostFormValue("salt"),
			KeyBundle: req.PostFormValue("keyBundle"),
			IsBackup:  req.PostFormValue("isBackup"),
			PublicKey: pk,
		}); err != nil {
		log.Errorf("AddUser: %v", err)
		return NewResponse("nok")
	}
	return NewResponse("ok")
}

// handlePreLogin handles the /v2/login/preLogin endpoint.
//
// Arguments:
//  - req: The http request.
//
// The form arguments:
//  - email: The email address of the account.
//
// Returns:
//  - StingleResponse(ok)
//     Part(salt, The salt used to hash the password)
func (s *Server) handlePreLogin(req *http.Request) *StingleResponse {
	email := req.PostFormValue("email")
	u, err := s.db.User(email)
	if err != nil {
		return NewResponse("nok").AddError("User doesn't exist")
	}
	return NewResponse("ok").AddPart("salt", u.Salt)
}

// handleLogin handles the /v2/login/login endpoint.
//
// Argument:
//  - req: The http request.
//
// The form arguments:
//  - email: The email address of the account.
//  - password: The hashed password.
//
// Returns:
//  - StingleResponse(ok)
//      Part(userId, The numeric ID of the account)
//      Part(keyBundle, The encoded keys of the user)
//      Part(serverPublicKey, The server's public key that is associated with this account)
//      Part(token, The session token signed by the server)
//      Part(isKeyBackedUp, Whether the user's secret key is in keyBundle)
//      Part(homeFolder, A "Home folder" used on the app's device)
func (s *Server) handleLogin(req *http.Request) *StingleResponse {
	email := req.PostFormValue("email")
	pass := req.PostFormValue("password")
	u, err := s.db.User(email)
	if err != nil || u.Password != pass {
		return NewResponse("nok").AddError("Invalid credentials")
	}
	return NewResponse("ok").
		AddPart("keyBundle", u.KeyBundle).
		AddPart("serverPublicKey", u.ServerPublicKeyForExport()).
		AddPart("token", crypto.MintToken(u.ServerSignKey,
			crypto.Token{Scope: "session", Subject: u.Email}, 30*24*time.Hour)).
		AddPart("userId", fmt.Sprintf("%d", u.UserID)).
		AddPart("isKeyBackedUp", u.IsBackup).
		AddPart("homeFolder", u.HomeFolder)
}

// handleLogout handles the /v2/login/logout endpoint. This server doesn't
// actually do anything with it.
//
// Arguments:
//  - user: The authenticated user.
//  - req: The http request.
//
// Returns:
//  - StringleResponse(ok)
func (s *Server) handleLogout(user database.User, req *http.Request) *StingleResponse {
	return NewResponse("ok")
}

// handleChangePass handles the /v2/login/changePass endpoint.
//
// Arguments:
//  - user: The authenticated user.
//  - req: The http request.
//
// Form arguments:
//  - params - Encrypted parameters:
//     - newPassword: The new hashed password.
//     - newSalt: The salt used to hash the new password.
//     - keyBundle: The new keyBundle.
//
// Returns:
//  - StingleResponse(ok)
//      Part(token, A new signed session token)
func (s *Server) handleChangePass(user database.User, req *http.Request) *StingleResponse {
	params, err := s.decodeParams(req.PostFormValue("params"), user)
	if err != nil {
		log.Errorf("decodeParams: %v", err)
		return NewResponse("nok")
	}
	user.Password = params["newPassword"]
	user.Salt = params["newSalt"]
	user.KeyBundle = params["keyBundle"]
	pk, err := crypto.DecodeKeyBundle(user.KeyBundle)
	if err != nil {
		log.Errorf("DecodeKeyBundle: %v", err)
		return NewResponse("nok")
	}
	user.PublicKey = pk

	if err := s.db.UpdateUser(user); err != nil {
		log.Errorf("UpdateUser: %v", err)
		return NewResponse("nok")
	}
	return NewResponse("ok").
		AddPart("token", crypto.MintToken(s.db.SignKeyForUser(user.Email),
			crypto.Token{Scope: "session", Subject: user.Email}, 30*24*time.Hour))
}

// handleGetServerPK handles the /v2/login/getServerPK endpoint. The server's
// public key is used to encrypt the "params" arguments.
//
// Arguments:
//  - user: The authenticated user.
//  - req: The http request.
//
// Returns:
//  - StingleResponse(ok)
//     Part(serverPK, server's public key)
func (s *Server) handleGetServerPK(user database.User, req *http.Request) *StingleResponse {
	return NewResponse("ok").AddPart("serverPK", user.ServerPublicKeyForExport())
}

// handleCheckKey handles the /v2/login/checkKey endpoint. This is part of the
// password recovery flow. The user has to enter their secret "passphrase" in
// the app, and the app uses this endpoint to verify that the key/passphrase is
// correct.
//
// Argument:
//  - req: The http request.
//
// Form arguments:
//  - email: The email address of the account.
//
// Returns:
//  - StingleResponse(ok)
//      Part(challenge, A message that can only be read with the right secret key)
//      Part(isKeyBackedUp, Whether the encrypted secrey of the user in on the server)
//      Part(serverPK, The public key of the server associated with this account)
func (s *Server) handleCheckKey(req *http.Request) *StingleResponse {
	email := req.PostFormValue("email")
	u, err := s.db.User(email)
	if err != nil {
		return NewResponse("nok").AddError("User doesn't exist")
	}
	rnd := make([]byte, 32)
	if _, err := rand.Read(rnd); err != nil {
		return NewResponse("nok")
	}
	return NewResponse("ok").
		AddPart("challenge", crypto.SealBox(append([]byte("validkey_"), rnd...), u.PublicKey)).
		AddPart("isKeyBackedUp", u.IsBackup).
		AddPart("serverPK", u.ServerPublicKeyForExport())
}

// handleRecoverAccount handles the /v2/login/recoverAccount endpoint, which
// is pretty much same as /v2/login/changePass.
// Form arguments:
//
// Argument:
//  - req: The http request.
//
// Form arguments:
//  - email: The email address of the account.
//  - params - Encrypted parameters:
//     - newPassword: The new hashed password.
//     - newSalt: The salt used to hash the new password.
//     - keyBundle: The new keyBundle.
//
// Returns:
//  - StingleResponse(ok)
//      Part(result, OK)
func (s *Server) handleRecoverAccount(req *http.Request) *StingleResponse {
	email := req.PostFormValue("email")
	user, err := s.db.User(email)
	if err != nil {
		return NewResponse("nok").AddError("User doesn't exist")
	}
	params, err := s.decodeParams(req.PostFormValue("params"), user)
	if err != nil {
		log.Errorf("decodeParams: %v", err)
		return NewResponse("nok")
	}
	user.Password = params["newPassword"]
	user.Salt = params["newSalt"]
	user.KeyBundle = params["keyBundle"]
	pk, err := crypto.DecodeKeyBundle(user.KeyBundle)
	if err != nil {
		log.Errorf("DecodeKeyBundle: %v", err)
		return NewResponse("nok")
	}
	user.PublicKey = pk

	if err := s.db.UpdateUser(user); err != nil {
		log.Errorf("UpdateUser: %v", err)
		return NewResponse("nok")
	}
	return NewResponse("ok").AddPart("result", "OK")
}

// handleDeleteUser handles the /v2/keys/deleteUser endpoint. It is used
// to delete the user's account, but it is not currently implemented.
//
// Arguments:
//  - user: The authenticated user.
//  - req: The http request.
//
// Form arguments:
//  - token: The signed session token.
//  - params: Encrypted parameters:
//     - password: The user's hashed password.
//
// Returns:
//  - StingleResponse(ok)
func (s *Server) handleDeleteUser(user database.User, req *http.Request) *StingleResponse {
	if _, err := s.decodeParams(req.PostFormValue("params"), user); err != nil {
		log.Errorf("decodeParams: %v", err)
		return NewResponse("nok")
	}
	return NewResponse("nok").AddError("Account deletion is not implemented")
}

// handleReuploadKeys handles the /v2/keys/reuploadKeys endpoint. It is used
// when the user changes the "Backup my keys" setting.
//
// Arguments:
//  - user: The authenticated user.
//  - req: The http request.
//
// Form arguments:
//  - token: The signed session token.
//  - params: Encrypted parameters:
//     - keyBundle: The new keyBundle.
//
// Returns:
//  - StingleResponse(ok)
func (s *Server) handleReuploadKeys(user database.User, req *http.Request) *StingleResponse {
	params, err := s.decodeParams(req.PostFormValue("params"), user)
	if err != nil {
		log.Errorf("decodeParams: %v", err)
		return NewResponse("nok")
	}
	user.KeyBundle = params["keyBundle"]
	pk, err := crypto.DecodeKeyBundle(user.KeyBundle)
	if err != nil {
		log.Errorf("DecodeKeyBundle: %v", err)
		return NewResponse("nok")
	}
	user.PublicKey = pk

	if err := s.db.UpdateUser(user); err != nil {
		log.Errorf("UpdateUser: %v", err)
		return NewResponse("nok")
	}
	return NewResponse("ok")
}
