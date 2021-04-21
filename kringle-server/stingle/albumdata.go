package stingle

import (
	"encoding/binary"
	"errors"
)

type AlbumMetadata struct {
	Name string `json:"name"`
}

// DecryptAlbumMetadata decrypts an album's metadata.
func DecryptAlbumMetadata(md string, sk SecretKey) (*AlbumMetadata, error) {
	b, err := sk.SealBoxOpenBase64(md)
	if err != nil {
		return nil, err
	}
	if b[0] != 1 {
		return nil, errors.New("unexpected version")
	}
	b = b[1:]
	l := int(binary.BigEndian.Uint32(b[:4]))
	b = b[4:]
	if l < 0 || l > len(b) {
		return nil, errors.New("invalid name length")
	}
	name := string(b[:l])
	return &AlbumMetadata{Name: name}, nil
}
