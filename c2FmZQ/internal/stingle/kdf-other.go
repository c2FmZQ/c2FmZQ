// +build !sodium

package stingle

import (
	"encoding/binary"

	"github.com/minio/blake2b-simd"
)

// Derive subkey from masterKey.
func DeriveKey(masterKey []byte, length, id uint64, ctx string) (dk []byte) {
	salt := make([]byte, 8)
	binary.LittleEndian.PutUint64(salt, id)

	h, err := blake2b.New(&blake2b.Config{
		Size:   uint8(length),
		Key:    masterKey,
		Salt:   salt,
		Person: []byte(ctx),
	})
	if err != nil {
		panic(err)
	}
	dk = make([]byte, int(length))
	h.Sum(dk[:0])
	return
}
