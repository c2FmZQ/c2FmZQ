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

package stingle

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"io/fs"

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
	for i := 0; i < len(b); i++ {
		b[i] = 0
	}
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
		lastChunkSize := (size - r.start) % int64(r.hdr.ChunkSize+chunkOverhead)
		if lastChunkSize > 0 {
			lastChunkSize -= chunkOverhead
		}
		decSize := nChunks*int64(r.hdr.ChunkSize) + lastChunkSize
		newOffset = decSize + offset
	default:
		return 0, fmt.Errorf("invalid whence: %d", whence)
	}
	if newOffset < 0 {
		return 0, fs.ErrInvalid
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
	chunkOffset := r.off % int64(r.hdr.ChunkSize)
	seekTo := r.start + r.off/int64(r.hdr.ChunkSize)*int64(r.hdr.ChunkSize+chunkOverhead)
	if _, err := seeker.Seek(seekTo, io.SeekStart); err != nil {
		return 0, err
	}
	r.buf = nil
	if err := r.readChunk(); err != nil && err != io.EOF {
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
	ck := DeriveKey(r.hdr.SymmetricKey, chacha20poly1305.KeySize, uint64(r.off/int64(r.hdr.ChunkSize)+1), context)
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
	if err == io.ErrUnexpectedEOF {
		err = io.EOF
	}
	if n > 0 && err == io.EOF {
		err = nil
	}
	return err
}

func (r *StreamReader) Read(b []byte) (n int, err error) {
	for err == nil {
		nn := copy(b[n:], r.buf)
		r.buf = r.buf[nn:]
		r.off += int64(nn)
		n += nn
		if n == len(b) {
			break
		}
		err = r.readChunk()
	}
	if n > 0 {
		return n, nil
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
