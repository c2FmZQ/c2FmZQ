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
	"bytes"
	"encoding/base64"
	"fmt"
	"image/png"
	"net/http"

	"github.com/pquerna/otp/totp"

	"c2FmZQ/internal/database"
	"c2FmZQ/internal/log"
	"c2FmZQ/internal/stingle"
)

// handleGenerateOTP handles the /c2/config/generateOTP endpoint.
//
// Arguments:
//  - user: The authenticated user.
//  - req: The http request.
//
// Form arguments:
//  - token: The signed session token.
//
// Returns:
//  - stingle.Response(ok)
//       Parts("key", OTP key)
//       Parts("img", base64-encoded QR code image)
func (s *Server) handleGenerateOTP(user database.User, req *http.Request) *stingle.Response {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      req.Host,
		AccountName: user.Email,
	})
	if err != nil {
		log.Errorf("totp.Generate: %v", err)
		return stingle.ResponseNOK()
	}
	img, err := key.Image(200, 200)
	if err != nil {
		log.Errorf("key.Image: %v", err)
		return stingle.ResponseNOK()
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		log.Errorf("png.Encode: %v", err)
		return stingle.ResponseNOK()
	}

	return stingle.ResponseOK().
		AddPart("key", key.Secret()).
		AddPart("img", fmt.Sprintf("data:image/png;base64,%s", base64.StdEncoding.EncodeToString(buf.Bytes())))
}

// handleSetOTP handles the /c2/config/setOTP endpoint.
//
// Arguments:
//  - user: The authenticated user.
//  - req: The http request.
//
// Form arguments:
//  - token: The signed session token.
//  - params: Encrypted parameters:
//     - key: The OTP key
//     - code: The current OTP code
//
// Returns:
//  - stingle.Response(ok)
func (s *Server) handleSetOTP(user database.User, req *http.Request) *stingle.Response {
	params, err := s.decodeParams(req.PostFormValue("params"), user)
	if err != nil {
		log.Errorf("decodeParams: %v", err)
		return stingle.ResponseNOK()
	}
	key := params["key"]
	code := params["code"]

	if !validateOTP(key, code) {
		return stingle.ResponseNOK().
			AddError("code is invalid")
	}
	user.OTPKey = key
	if err := s.db.UpdateUser(user); err != nil {
		log.Errorf("UpdateUser: %v", err)
		return stingle.ResponseNOK()
	}
	resp := stingle.ResponseOK()
	if user.OTPKey == "" {
		resp.AddInfo("OTP disabled")
	} else {
		resp.AddInfo("OTP enabled")
	}
	return resp
}

func validateOTP(key, passcode string) bool {
	if key == "" && passcode == "" {
		return true
	}
	return totp.Validate(passcode, key)
}
