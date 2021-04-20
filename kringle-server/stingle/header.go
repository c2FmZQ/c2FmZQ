package stingle

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Header is a the header of an encrypted file.
type Header struct {
	FileID        []byte
	Version       uint8
	ChunkSize     int32
	DataSize      int64
	SymmetricKey  []byte
	FileType      uint8
	Filename      []byte
	VideoDuration int32
}

// DecryptBase64Headers decrypts base64-encoded headers.
func DecryptBase64Headers(hdrs string, sk SecretKey) ([]Header, error) {
	var out []Header
	for _, hdr := range strings.Split(hdrs, "*") {
		b, err := base64.RawURLEncoding.DecodeString(hdr)
		if err != nil {
			return nil, err
		}
		h, err := DecryptHeader(bytes.NewBuffer(b), sk)
		if err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, nil
}

// DecryptHeader decrypts a file header from the reader.
func DecryptHeader(in io.Reader, sk SecretKey) (hdr Header, err error) {
	b := make([]byte, 3)
	if _, err = io.ReadFull(in, b); err != nil {
		return
	}
	// 2 bytes {'S','P'}
	if b[0] != 'S' || b[1] != 'P' {
		err = errors.New("unexpected file type")
		return
	}
	// 1 byte version
	if b[2] != 1 {
		err = errors.New("unexpected file version")
		return
	}

	// 32-byte file ID
	hdr.FileID = make([]byte, 32)
	if _, err = io.ReadFull(in, hdr.FileID); err != nil {
		return
	}

	// 4-byte header size
	var headerSize int32
	if err = binary.Read(in, binary.BigEndian, &headerSize); err != nil {
		return
	}
	if headerSize < 0 || headerSize > 64*1024 {
		err = errors.New("invalid header size")
		return
	}

	// header-size bytes (encHeader)
	encHeader := make([]byte, headerSize)
	if _, err = io.ReadFull(in, encHeader); err != nil {
		return
	}

	d, err := sk.SealBoxOpen(encHeader)
	if err != nil {
		return hdr, err
	}

	// 1-byte header.headerVersion
	hdr.Version, d = d[0], d[1:]
	// 4-byte header.chunkSize
	hdr.ChunkSize, d = int32(binary.BigEndian.Uint32(d[:4])), d[4:]
	if hdr.ChunkSize < 1 || hdr.ChunkSize > 64*1024*1024 {
		err = errors.New("invalid chunk size")
		return
	}

	// 8-byte header.dataSize
	if len(d) < 8 {
		err = errors.New("invalid data size")
		return
	}
	hdr.DataSize, d = int64(binary.BigEndian.Uint64(d[:8])), d[8:]

	// 32-byte SymmetricKey
	if len(d) < 32 {
		err = errors.New("invalid symmetric key")
		return
	}
	hdr.SymmetricKey = make([]byte, 32)
	copy(hdr.SymmetricKey, d)
	d = d[32:]

	// 1-byte FileType
	if len(d) == 0 {
		err = errors.New("invalid file type")
		return
	}
	hdr.FileType, d = d[0], d[1:]

	// 1-byte filenameSize
	if len(d) == 0 {
		err = errors.New("invalid filename size")
		return
	}
	filenameSize := d[0]
	d = d[1:]
	if filenameSize < 0 || int(filenameSize) > len(d) {
		err = fmt.Errorf("invalid filename size: %d", filenameSize)
		return
	}

	// filenameSize-byte Filename
	hdr.Filename, d = d[:filenameSize], d[filenameSize:]

	// 4-byte VideoDuration
	if len(d) < 4 {
		err = errors.New("invalid video duration")
		return
	}
	hdr.VideoDuration = int32(binary.BigEndian.Uint32(d[:4]))
	d = d[4:]

	return
}

// EncryptHeader encrypts and write the file header to the writer.
func EncryptHeader(out io.Writer, hdr Header, pk PublicKey) (err error) {
	if len(hdr.FileID) != 32 {
		return errors.New("invalid file id")
	}
	if len(hdr.SymmetricKey) != 32 {
		return errors.New("invalid symmetric key")
	}

	var h bytes.Buffer
	binary.Write(&h, binary.BigEndian, hdr.Version)              // 1 byte
	binary.Write(&h, binary.BigEndian, hdr.ChunkSize)            // 4 bytes
	binary.Write(&h, binary.BigEndian, hdr.DataSize)             // 8 bytes
	binary.Write(&h, binary.BigEndian, hdr.SymmetricKey)         // 32 bytes
	binary.Write(&h, binary.BigEndian, hdr.FileType)             // 1 byte
	binary.Write(&h, binary.BigEndian, uint8(len(hdr.Filename))) // 1 byte
	binary.Write(&h, binary.BigEndian, hdr.Filename)             // n bytes
	binary.Write(&h, binary.BigEndian, hdr.VideoDuration)        // 4 bytes

	encHdr := pk.SealBox(h.Bytes())
	hdrSize := make([]byte, 4)
	binary.BigEndian.PutUint32(hdrSize, uint32(len(encHdr)))
	if _, err = out.Write([]byte{'S', 'P', 1}); err != nil {
		return err
	}
	if _, err = out.Write(hdr.FileID); err != nil {
		return err
	}
	if _, err = out.Write(hdrSize); err != nil {
		return err
	}
	if _, err = out.Write(encHdr); err != nil {
		return err
	}
	return nil
}