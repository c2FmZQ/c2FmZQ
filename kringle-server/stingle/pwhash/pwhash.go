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
	Moderate           // for login
	Difficult          // for key bundle
)

func KeyFromPassword(password, salt []byte, mode, length int) []byte {
	var memLimit, opsLimit uint32
	switch mode {
	case Interactive:
		memLimit, opsLimit = memLimitInteractive, opsLimitInteractive
	case Moderate:
		memLimit, opsLimit = memLimitModerate, opsLimitModerate
	case Difficult:
		memLimit, opsLimit = memLimitSensitive, opsLimitSensitive
	default:
		panic("unknown mode")
	}
	return argon2.IDKey(password, salt, opsLimit, memLimit, 1, uint32(length))
}
