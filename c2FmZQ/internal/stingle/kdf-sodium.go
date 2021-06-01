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
