package protocol

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// TextElementName is the magic filename that designates a text snippet in the
// TCP transfer stream. A session containing this name must contain exactly
// that one element — mixing text with files in a single session is not part of
// the protocol.
const TextElementName = "___DUKTO___TEXT___"

// DirectorySizeMarker is the ElementHeader.Size value that marks a directory
// entry. No data bytes follow a directory header.
const DirectorySizeMarker int64 = -1

// SessionHeader is the first 16 bytes of a TCP transfer stream.
//
// TotalElements is the number of elements in the session (files + directories
// + the synthetic text element). It must be > 0.
//
// TotalSize is the sum of file sizes in bytes. Directories contribute 0. Text
// snippets contribute the UTF-8 byte length of the text. It must be ≥ 0 and is
// used by receivers for progress bars only.
type SessionHeader struct {
	TotalElements uint64
	TotalSize     int64
}

// ElementHeader frames a single element in the transfer stream as
// "<utf8-name>\x00<size-le-i64>".
//
// Name uses '/' as a path separator regardless of host OS. It must be non-empty.
//
// Size:
//   - -1: directory entry (no data follows)
//   -  0: empty file OR zero-length text snippet (no data follows)
//   - >0: file byte count or UTF-8 text byte length (that many data bytes follow)
//   - values < -1 are invalid and cause a receiver to abort the session
type ElementHeader struct {
	Name string
	Size int64
}

// ErrInvalidStream is returned by the decoding helpers for malformed TCP data.
var ErrInvalidStream = errors.New("dukto: invalid TCP transfer stream")

// WriteSessionHeader writes the 16-byte little-endian session header.
func WriteSessionHeader(w io.Writer, h SessionHeader) error {
	var buf [16]byte
	binary.LittleEndian.PutUint64(buf[0:8], h.TotalElements)
	binary.LittleEndian.PutUint64(buf[8:16], uint64(h.TotalSize))
	_, err := w.Write(buf[:])
	return err
}

// ReadSessionHeader reads the 16-byte little-endian session header. Returns
// ErrInvalidStream if TotalElements is 0 or TotalSize is negative.
func ReadSessionHeader(r io.Reader) (SessionHeader, error) {
	var buf [16]byte
	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return SessionHeader{}, err
	}
	h := SessionHeader{
		TotalElements: binary.LittleEndian.Uint64(buf[0:8]),
		TotalSize:     int64(binary.LittleEndian.Uint64(buf[8:16])),
	}
	if h.TotalElements == 0 {
		return h, fmt.Errorf("%w: zero TotalElements", ErrInvalidStream)
	}
	if h.TotalSize < 0 {
		return h, fmt.Errorf("%w: negative TotalSize %d", ErrInvalidStream, h.TotalSize)
	}
	return h, nil
}

// WriteElementHeader writes "<utf8(Name)>\x00<size-le-i64>".
func WriteElementHeader(w io.Writer, h ElementHeader) error {
	if h.Name == "" {
		return fmt.Errorf("%w: empty element name", ErrInvalidStream)
	}
	if _, err := w.Write([]byte(h.Name)); err != nil {
		return err
	}
	if _, err := w.Write([]byte{0}); err != nil {
		return err
	}
	var sz [8]byte
	binary.LittleEndian.PutUint64(sz[:], uint64(h.Size))
	_, err := w.Write(sz[:])
	return err
}

// ReadElementHeader reads a NUL-terminated UTF-8 name followed by the 8-byte
// little-endian size. The reader should be a *bufio.Reader for efficient
// byte-by-byte name scanning; any io.Reader works if wrapped. Returns
// ErrInvalidStream on empty name or Size < -1.
func ReadElementHeader(r *bufio.Reader) (ElementHeader, error) {
	name, err := r.ReadString(0)
	if err != nil {
		return ElementHeader{}, err
	}
	// Strip the trailing NUL terminator that ReadString includes.
	name = name[:len(name)-1]
	if name == "" {
		return ElementHeader{}, fmt.Errorf("%w: empty element name", ErrInvalidStream)
	}
	var sz [8]byte
	if _, err := io.ReadFull(r, sz[:]); err != nil {
		return ElementHeader{}, err
	}
	size := int64(binary.LittleEndian.Uint64(sz[:]))
	if size < DirectorySizeMarker {
		return ElementHeader{}, fmt.Errorf("%w: element size %d below -1", ErrInvalidStream, size)
	}
	return ElementHeader{Name: name, Size: size}, nil
}

// IsDirectory reports whether h describes a directory (Size == -1).
func (h ElementHeader) IsDirectory() bool { return h.Size == DirectorySizeMarker }

// IsText reports whether h describes the magic text-snippet element.
func (h ElementHeader) IsText() bool { return h.Name == TextElementName }
