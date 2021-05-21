package stingle

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/chacha20poly1305"
)

const (
	poly1305Overhead = 16
	context          = "__data__"

	chunkOverhead = chacha20poly1305.NonceSizeX + poly1305Overhead
)

// EncryptFile encrypts the plaintext from the reader using the SymmetricKey in
// header, and writes the ciphertext to the writer.
func EncryptFile(w io.Writer, header *Header) *StreamWriter {
	return &StreamWriter{hdr: header, w: w}
}

// DecryptFile decrypts the ciphertext from the reader using the SymmetricKey
// in header, and write the plaintext to the writer.
func DecryptFile(r io.Reader, header *Header) *StreamReader {
	var start int64
	if seeker, ok := r.(io.Seeker); ok {
		off, err := seeker.Seek(0, io.SeekCurrent)
		if err != nil {
			panic(err)
		}
		start = off
	}
	return &StreamReader{hdr: header, r: r, start: start}
}

// StreamWriter encrypts a stream of data.
type StreamWriter struct {
	hdr *Header
	w   io.Writer
	c   uint64
	buf []byte
}

func (w *StreamWriter) writeChunk(b []byte) (int, error) {
	w.c++
	ck := DeriveKey(w.hdr.SymmetricKey, chacha20poly1305.KeySize, w.c, context)

	nonce := make([]byte, chacha20poly1305.NonceSizeX)
	if _, err := rand.Read(nonce); err != nil {
		return 0, err
	}
	ae, err := chacha20poly1305.NewX(ck)
	if err != nil {
		return 0, err
	}
	enc := ae.Seal(nonce, nonce, b, nil)
	return w.w.Write(enc)
}

func (w *StreamWriter) Write(b []byte) (n int, err error) {
	w.buf = append(w.buf, b...)
	n = len(b)
	for int32(len(w.buf)) >= w.hdr.ChunkSize {
		_, err = w.writeChunk(w.buf[:w.hdr.ChunkSize])
		w.buf = w.buf[w.hdr.ChunkSize:]
		if err != nil {
			break
		}
	}
	return
}

func (w *StreamWriter) Close() (err error) {
	if len(w.buf) > 0 {
		_, err = w.writeChunk(w.buf)
	}
	w.hdr.Wipe()
	if c, ok := w.w.(io.Closer); ok {
		if e := c.Close(); err == nil {
			err = e
		}
	}
	return
}

// StreamReader decrypts an input stream.
type StreamReader struct {
	hdr   *Header
	r     io.Reader
	c     int64
	start int64
	off   int64
	buf   []byte
}

// Seek moves the next read to a new offset. The offset is in the decrypted
// stream.
func (r *StreamReader) Seek(offset int64, whence int) (int64, error) {
	var newOffset int64
	switch whence {
	case io.SeekStart:
		newOffset = offset
	case io.SeekCurrent:
		newOffset = r.off + offset
	case io.SeekEnd:
		seeker, ok := r.r.(io.Seeker)
		if !ok {
			return 0, errors.New("SeekEnd not implemented")
		}
		size, err := seeker.Seek(0, io.SeekEnd)
		if err != nil {
			return 0, err
		}
		nChunks := (size - r.start) / int64(r.hdr.ChunkSize+chunkOverhead)
		lastChunkSize := (size-r.start)%int64(r.hdr.ChunkSize+chunkOverhead) - chunkOverhead
		decSize := nChunks*int64(r.hdr.ChunkSize) + lastChunkSize
		newOffset = decSize + offset
	default:
		return 0, fmt.Errorf("invalid whence: %d", whence)
	}
	if newOffset == r.off {
		return r.off, nil
	}
	seeker, ok := r.r.(io.Seeker)
	if !ok {
		return 0, errors.New("input is not seekable")
	}
	// Move to new offset.
	r.off = newOffset
	r.c = r.off / int64(r.hdr.ChunkSize)
	chunkOffset := r.off % int64(r.hdr.ChunkSize)
	seekTo := r.start + r.c*int64(r.hdr.ChunkSize+chunkOverhead)
	if _, err := seeker.Seek(seekTo, io.SeekStart); err != nil {
		return 0, err
	}
	r.buf = nil
	if err := r.readChunk(); err != nil {
		return 0, err
	}
	if chunkOffset < int64(len(r.buf)) {
		r.buf = r.buf[chunkOffset:]
	} else {
		r.buf = nil
	}
	return r.off, nil
}

func (r *StreamReader) readChunk() error {
	r.c++
	ck := DeriveKey(r.hdr.SymmetricKey, chacha20poly1305.KeySize, uint64(r.c), context)
	in := make([]byte, r.hdr.ChunkSize+chunkOverhead)
	n, err := io.ReadFull(r.r, in)
	if n > 0 {
		nonce := in[:chacha20poly1305.NonceSizeX]
		enc := in[chacha20poly1305.NonceSizeX:n]

		ae, err := chacha20poly1305.NewX(ck)
		if err != nil {
			return err
		}
		dec, err := ae.Open(enc[:0], nonce, enc, nil)
		if err != nil {
			return err
		}
		r.buf = append(r.buf, dec...)
	}
	if n > 0 && (err == io.EOF || err == io.ErrUnexpectedEOF) {
		err = nil
	}
	return err
}

func (r *StreamReader) Read(b []byte) (n int, err error) {
	for err == nil {
		nn := copy(b[n:], r.buf)
		r.buf = r.buf[nn:]
		n += nn
		if n == len(b) {
			break
		}
		if err = r.readChunk(); len(r.buf) > 0 && (err == io.EOF || err == io.ErrUnexpectedEOF) {
			err = nil
		}
	}
	r.off += int64(n)
	if n > 0 {
		return n, nil
	}
	if err == io.ErrUnexpectedEOF {
		err = io.EOF
	}
	return n, err
}

func (r *StreamReader) Close() error {
	r.hdr.Wipe()
	if c, ok := r.r.(io.Closer); ok {
		return c.Close()
	}
	return nil
}
