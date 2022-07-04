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

package token

import (
	"testing"
	"time"
)

func TestTokens(t *testing.T) {
	key := MakeKey()
	tok := Mint(key, Token{Scope: "foo", Subject: 44545}, time.Hour)

	dec, err := Decrypt(key, tok)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}
	if dec.Scope != "foo" || dec.Subject != 44545 {
		t.Errorf("Unexpected token. Got %+v, want {'foo', 'blah blah'}", dec)
	}
}
