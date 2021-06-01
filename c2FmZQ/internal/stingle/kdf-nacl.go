// +build nacl arm windows darwin

package stingle

func DeriveKey(masterKey []byte, length, id uint64, ctx string) []byte {
	panic("DeriveKey is not implemented")
	return nil
}
