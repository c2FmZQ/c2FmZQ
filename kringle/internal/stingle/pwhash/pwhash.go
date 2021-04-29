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
