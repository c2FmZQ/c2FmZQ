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

package webauthn

import (
	"testing"
)

func TestWithRawBrowserData(t *testing.T) {
	var (
		clientDataJSON = []byte{
			123, 34, 116, 121, 112, 101, 34, 58, 34, 119, 101, 98, 97, 117,
			116, 104, 110, 46, 99, 114, 101, 97, 116, 101, 34, 44, 34, 99,
			104, 97, 108, 108, 101, 110, 103, 101, 34, 58, 34, 65, 65, 69,
			67, 65, 119, 81, 70, 66, 103, 99, 73, 67, 81, 111, 76, 68, 65,
			48, 79, 68, 120, 65, 82, 69, 104, 77, 85, 70, 82, 89, 88, 71,
			66, 107, 97, 71, 120, 119, 100, 72, 104, 56, 34, 44, 34, 111,
			114, 105, 103, 105, 110, 34, 58, 34, 104, 116, 116, 112, 115,
			58, 47, 47, 112, 102, 102, 116, 46, 110, 101, 116, 34, 44, 34,
			99, 114, 111, 115, 115, 79, 114, 105, 103, 105, 110, 34, 58,
			102, 97, 108, 115, 101, 125,
		}
		attestationObject = []byte{
			163, 99, 102, 109, 116, 104, 102, 105, 100, 111, 45, 117, 50,
			102, 103, 97, 116, 116, 83, 116, 109, 116, 162, 99, 115, 105,
			103, 88, 72, 48, 70, 2, 33, 0, 206, 172, 115, 184, 201, 72,
			130, 231, 198, 68, 49, 220, 200, 94, 115, 84, 183, 223, 223,
			181, 86, 213, 152, 52, 204, 47, 46, 74, 28, 21, 243, 55, 2,
			33, 0, 197, 18, 140, 110, 215, 146, 61, 20, 119, 12, 229, 155,
			85, 28, 105, 90, 24, 65, 106, 52, 231, 210, 69, 46, 62, 212,
			189, 240, 120, 50, 131, 35, 99, 120, 53, 99, 129, 89, 1, 221,
			48, 130, 1, 217, 48, 130, 1, 125, 160, 3, 2, 1, 2, 2, 1, 1, 48,
			13, 6, 9, 42, 134, 72, 134, 247, 13, 1, 1, 11, 5, 0, 48, 96,
			49, 11, 48, 9, 6, 3, 85, 4, 6, 19, 2, 85, 83, 49, 17, 48, 15, 6,
			3, 85, 4, 10, 12, 8, 67, 104, 114, 111, 109, 105, 117, 109, 49,
			34, 48, 32, 6, 3, 85, 4, 11, 12, 25, 65, 117, 116, 104, 101,
			110, 116, 105, 99, 97, 116, 111, 114, 32, 65, 116, 116, 101,
			115, 116, 97, 116, 105, 111, 110, 49, 26, 48, 24, 6, 3, 85, 4,
			3, 12, 17, 66, 97, 116, 99, 104, 32, 67, 101, 114, 116, 105,
			102, 105, 99, 97, 116, 101, 48, 30, 23, 13, 49, 55, 48, 55, 49,
			52, 48, 50, 52, 48, 48, 48, 90, 23, 13, 52, 50, 49, 49, 49, 55,
			50, 49, 52, 49, 49, 52, 90, 48, 96, 49, 11, 48, 9, 6, 3, 85, 4,
			6, 19, 2, 85, 83, 49, 17, 48, 15, 6, 3, 85, 4, 10, 12, 8, 67,
			104, 114, 111, 109, 105, 117, 109, 49, 34, 48, 32, 6, 3, 85, 4,
			11, 12, 25, 65, 117, 116, 104, 101, 110, 116, 105, 99, 97, 116,
			111, 114, 32, 65, 116, 116, 101, 115, 116, 97, 116, 105, 111,
			110, 49, 26, 48, 24, 6, 3, 85, 4, 3, 12, 17, 66, 97, 116, 99,
			104, 32, 67, 101, 114, 116, 105, 102, 105, 99, 97, 116, 101, 48,
			89, 48, 19, 6, 7, 42, 134, 72, 206, 61, 2, 1, 6, 8, 42, 134, 72,
			206, 61, 3, 1, 7, 3, 66, 0, 4, 141, 97, 126, 101, 201, 80, 142,
			100, 188, 197, 103, 58, 200, 42, 103, 153, 218, 60, 20, 70, 104,
			44, 37, 140, 70, 63, 255, 223, 88, 223, 210, 250, 62, 108, 55,
			139, 83, 215, 149, 196, 164, 223, 251, 65, 153, 237, 215, 134,
			47, 35, 171, 175, 2, 3, 180, 184, 145, 27, 160, 86, 153, 148,
			225, 1, 163, 37, 48, 35, 48, 12, 6, 3, 85, 29, 19, 1, 1, 255, 4,
			2, 48, 0, 48, 19, 6, 11, 43, 6, 1, 4, 1, 130, 229, 28, 2, 1, 1,
			4, 4, 3, 2, 5, 32, 48, 13, 6, 9, 42, 134, 72, 134, 247, 13, 1,
			1, 11, 5, 0, 3, 71, 0, 48, 68, 2, 32, 110, 49, 252, 55, 238,
			119, 141, 29, 27, 125, 75, 232, 103, 146, 197, 2, 229, 163, 237,
			228, 90, 129, 140, 198, 130, 105, 199, 28, 196, 46, 25, 4, 2,
			32, 127, 75, 238, 41, 183, 177, 29, 102, 154, 202, 191, 189,
			245, 16, 158, 46, 24, 96, 245, 180, 107, 134, 72, 16, 46, 227,
			198, 14, 141, 214, 38, 149, 104, 97, 117, 116, 104, 68, 97, 116,
			97, 88, 164, 59, 173, 244, 133, 130, 181, 29, 207, 214, 72, 18,
			138, 31, 63, 249, 128, 104, 87, 82, 35, 83, 189, 56, 165, 215,
			183, 249, 127, 162, 220, 237, 110, 65, 0, 0, 0, 0, 0, 0, 0, 0,
			0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 32, 221, 85, 140, 104,
			176, 38, 21, 82, 83, 107, 113, 146, 112, 106, 158, 15, 37, 108,
			181, 127, 53, 192, 192, 7, 212, 230, 215, 151, 243, 175, 222,
			88, 165, 1, 2, 3, 38, 32, 1, 33, 88, 32, 80, 137, 11, 175, 56,
			246, 27, 157, 96, 74, 188, 102, 243, 100, 218, 2, 99, 10, 241,
			171, 76, 122, 90, 50, 33, 210, 174, 194, 242, 198, 21, 139, 34,
			88, 32, 207, 17, 25, 199, 194, 225, 116, 13, 169, 195, 3, 52,
			180, 2, 215, 135, 212, 172, 5, 237, 7, 61, 217, 21, 209, 39,
			126, 139, 30, 104, 99, 242,
		}
		clientDataJSON2 = []byte{
			123, 34, 116, 121, 112, 101, 34, 58, 34, 119, 101, 98, 97, 117,
			116, 104, 110, 46, 103, 101, 116, 34, 44, 34, 99, 104, 97, 108,
			108, 101, 110, 103, 101, 34, 58, 34, 65, 65, 69, 67, 65, 119, 81,
			70, 66, 103, 99, 73, 67, 81, 111, 76, 68, 65, 48, 79, 68, 120,
			65, 82, 69, 104, 77, 85, 70, 82, 89, 88, 71, 66, 107, 97, 71,
			120, 119, 100, 72, 104, 56, 34, 44, 34, 111, 114, 105, 103, 105,
			110, 34, 58, 34, 104, 116, 116, 112, 115, 58, 47, 47, 112, 102,
			102, 116, 46, 110, 101, 116, 34, 44, 34, 99, 114, 111, 115, 115,
			79, 114, 105, 103, 105, 110, 34, 58, 102, 97, 108, 115, 101, 125,
		}
		authenticatorData = []byte{
			59, 173, 244, 133, 130, 181, 29, 207, 214, 72, 18, 138, 31, 63,
			249, 128, 104, 87, 82, 35, 83, 189, 56, 165, 215, 183, 249, 127,
			162, 220, 237, 110, 1, 0, 0, 0, 2,
		}
		signature = []byte{
			48, 68, 2, 32, 21, 41, 57, 157, 176, 112, 230, 228, 91, 125, 8,
			141, 56, 88, 109, 132, 34, 221, 245, 158, 45, 197, 234, 38, 61,
			70, 234, 31, 104, 115, 184, 198, 2, 32, 42, 99, 185, 185, 22,
			58, 251, 37, 98, 223, 206, 117, 40, 60, 227, 199, 58, 194, 97,
			216, 252, 247, 201, 218, 18, 237, 37, 133, 159, 252, 176, 145,
		}
	)

	if _, err := ParseClientData(clientDataJSON); err != nil {
		t.Fatalf("ParseClientData: %v", err)
	}
	if _, err := ParseClientData(clientDataJSON2); err != nil {
		t.Fatalf("ParseClientData: %v", err)
	}
	ao, err := ParseAttestationObject(attestationObject, clientDataJSON)
	if err != nil {
		t.Fatalf("ParseAttestationObject: %v", err)
	}
	if !ao.AuthData.UserPresence {
		t.Error("Expected UserPresence to be true")
	}
	if ao.AuthData.UserVerification {
		t.Error("Expected UserVerification to be false")
	}
	if ao.AuthData.AttestedCredentials == nil {
		t.Fatal("no AttestedCredentials")
	}
	if err := VerifySignature(ao.AuthData.AttestedCredentials.COSEKey, authenticatorData, clientDataJSON2, signature); err != nil {
		t.Fatalf("VerifySignature: %v", err)
	}
}

func TestWithFakeAuthenticator(t *testing.T) {
	auth, err := NewFakeAuthenticator()
	if err != nil {
		t.Fatalf("NewFakeAuthenticator: %v", err)
	}
	attestOpts, err := NewAttestationOptions()
	if err != nil {
		t.Fatalf("NewAttestationOptions: %v", err)
	}
	keys := make(map[string]string)
	for _, alg := range []int{algES256, algRS256} {
		attestOpts.PubKeyCredParams = []PubKeyCredParam{
			{
				Type: "public-key",
				Alg:  alg,
			},
		}
		clientDataJSON, attestationObject, err := auth.Create(attestOpts)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		ao, err := ParseAttestationObject(attestationObject, clientDataJSON)
		if err != nil {
			t.Fatalf("ParseAttestationObject: %v", err)
		}
		if !ao.AuthData.UserPresence {
			t.Error("Expected UserPresence to be true")
		}
		if ao.AuthData.UserVerification {
			t.Error("Expected UserVerification to be false")
		}
		if ao.AuthData.AttestedCredentials == nil {
			t.Fatal("no AttestedCredentials")
		}
		keys[ao.AuthData.AttestedCredentials.ID] = ao.AuthData.AttestedCredentials.COSEKey
	}

	for keyID, coseKey := range keys {
		assertOpts, err := NewAssertionOptions()
		if err != nil {
			t.Fatalf("NewAssertionOptions: %v", err)
		}
		assertOpts.AllowCredentials = append(assertOpts.AllowCredentials, CredentialID{
			ID: keyID,
		})
		id, clientDataJSON, authData, signature, err := auth.Get(assertOpts)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if id != keyID {
			t.Errorf("Unexpected key ID. Got %q, want %q", id, keyID)
		}
		if err := VerifySignature(coseKey, authData, clientDataJSON, signature); err != nil {
			t.Fatalf("VerifySignature: %v", err)
		}
	}

	auth.RotateKeys()
	// All signatures below should be invalid.
	for keyID, coseKey := range keys {
		assertOpts, err := NewAssertionOptions()
		if err != nil {
			t.Fatalf("NewAssertionOptions: %v", err)
		}
		assertOpts.AllowCredentials = append(assertOpts.AllowCredentials, CredentialID{
			ID: keyID,
		})
		id, clientDataJSON, authData, signature, err := auth.Get(assertOpts)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if id != keyID {
			t.Errorf("Unexpected key ID. Got %q, want %q", id, keyID)
		}
		if err := VerifySignature(coseKey, authData, clientDataJSON, signature); err == nil {
			t.Fatal("VerifySignature should have failed")
		}
	}
}
