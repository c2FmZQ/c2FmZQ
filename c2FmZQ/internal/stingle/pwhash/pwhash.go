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

package pwhash

import (
	"golang.org/x/crypto/argon2"
)

const (
	// Argon2ID constants.
	memLimitInteractive = 65536
	memLimitModerate    = 262144
	memLimitSensitive   = 1048576
	opsLimitInteractive = 2
	opsLimitModerate    = 3
	opsLimitSensitive   = 4

	Interactive = iota // not used
	Moderate           // for login, key bundle
	Sensitive          // not used
)

func KeyFromPassword(password, salt []byte, level, length uint32) []byte {
	var memLimit, opsLimit uint32
	switch level {
	case Interactive:
		memLimit, opsLimit = memLimitInteractive, opsLimitInteractive
	case Moderate:
		memLimit, opsLimit = memLimitModerate, opsLimitModerate
	case Sensitive:
		memLimit, opsLimit = memLimitSensitive, opsLimitSensitive
	default:
		panic("unknown level")
	}
	return argon2.IDKey(password, salt, opsLimit, memLimit, 1, length)
}
