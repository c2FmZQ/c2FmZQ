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

//go:build sodium
// +build sodium

package stingle

import (
	"github.com/jamesruan/sodium"
)

// Derive subkey from masterKey.
func DeriveKey(masterKey []byte, length int, id uint64, ctx string) []byte {
	mk := sodium.MasterKey{Bytes: sodium.Bytes(masterKey)}
	dk := mk.Derive(length, id, sodium.KeyContext(ctx))
	return []byte(dk.Bytes)
}
