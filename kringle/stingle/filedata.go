package stingle

import (
	"crypto/rand"
	"io"

	"golang.org/x/crypto/chacha20poly1305"
)

const (
	poly1305Overhead = 16
	context          = "__data__"
)

// EncryptFile encrypts the plaintext from the reader using the SymmetricKey in
// header, and writes the ciphertext to the writer.
func EncryptFile(w io.Writer, header Header) *StreamWriter {
	return &StreamWriter{hdr: header, w: w}
}

// DecryptFile decrypts the ciphertext from the reader using the SymmetricKey
// in header, and write the plaintext to the writer.
func DecryptFile(r io.Reader, header Header) *StreamReader {
	return &StreamReader{hdr: header, r: r}
}

// StreamWriter encrypts a stream of data.
type StreamWriter struct {
	hdr Header
	w   io.Writer
	c   uint64
	buf []byte
}

func (w *StreamWriter) writeChunk(b []byte) (int, error) {
	w.c++
	ck := DeriveKey(w.hdr.SymmetricKey, chacha20poly1305.KeySize, w.c, context)

	nonce := make([]byte, chacha20poly1305.NonceSizeX)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
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
	if c, ok := w.w.(io.Closer); ok {
		if e := c.Close(); err == nil {
			err = e
		}
	}
	return
}

// StreamReader decrypts an input stream.
type StreamReader struct {
	hdr Header
	r   io.Reader
	c   uint64
	buf []byte
}

func (r *StreamReader) Read(b []byte) (n int, err error) {
	for err == nil {
		nn := copy(b[n:], r.buf)
		r.buf = r.buf[nn:]
		n += nn
		if n == len(b) {
			break
		}
		r.c++
		ck := DeriveKey(r.hdr.SymmetricKey, chacha20poly1305.KeySize, r.c, context)
		in := make([]byte, chacha20poly1305.NonceSizeX+r.hdr.ChunkSize+poly1305Overhead)
		if nn, err = io.ReadFull(r.r, in); nn > 0 {
			nonce := in[:chacha20poly1305.NonceSizeX]
			enc := in[chacha20poly1305.NonceSizeX:nn]

			ae, err := chacha20poly1305.NewX(ck)
			if err != nil {
				return n, err
			}
			dec, err := ae.Open(enc[:0], nonce, enc, nil)
			if err != nil {
				return n, err
			}
			r.buf = append(r.buf, dec...)
		}
		if len(r.buf) > 0 && (err == io.EOF || err == io.ErrUnexpectedEOF) {
			err = nil
		}
	}
	if n > 0 {
		return n, nil
	}
	if err == io.ErrUnexpectedEOF {
		err = io.EOF
	}
	return n, err
}
