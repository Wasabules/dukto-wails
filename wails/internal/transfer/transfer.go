// Package transfer streams Dukto TCP sessions on top of internal/protocol.
//
// The package is split in two layers:
//
//   - Writer / Reader are streaming codecs over a raw io.Writer / io.Reader.
//     They know nothing about sockets or the filesystem, which makes them
//     trivial to unit-test with bytes.Buffer.
//   - Sources builds a Source list from local filesystem paths by flattening
//     directory trees the way the Qt reference does (directory header before
//     children, lexical order, '/' separators on the wire regardless of OS).
//
// Socket-level send/receive helpers live in send.go / receive.go.
package transfer

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"

	"dukto/internal/protocol"
)

// ErrStreamOverrun is returned when the caller writes more elements than the
// session header declared. Mixing that up would desynchronize the receiver
// halfway through the stream with no way to recover, so we refuse to write
// the extra header.
var ErrStreamOverrun = errors.New("transfer: element count exceeds session header")

// Writer streams one Dukto TCP session to an io.Writer. Create one with
// NewWriter, write exactly TotalElements elements via WriteDir / WriteFile /
// WriteText, then call Done. Writer is not safe for concurrent use.
type Writer struct {
	w         io.Writer
	remaining uint64
}

// NewWriter writes the 16-byte session header and returns a Writer scoped to
// hdr.TotalElements. The caller is responsible for having computed the header
// with Sources() or equivalently.
func NewWriter(w io.Writer, hdr protocol.SessionHeader) (*Writer, error) {
	if err := protocol.WriteSessionHeader(w, hdr); err != nil {
		return nil, err
	}
	return &Writer{w: w, remaining: hdr.TotalElements}, nil
}

// WriteDir emits a directory element. No data bytes follow.
func (sw *Writer) WriteDir(name string) error {
	if err := sw.reserve(); err != nil {
		return err
	}
	return protocol.WriteElementHeader(sw.w, protocol.ElementHeader{
		Name: name,
		Size: protocol.DirectorySizeMarker,
	})
}

// WriteFile emits a file element. size must be ≥ 0; exactly size bytes are
// copied from data. For size == 0 no data is read and data may be nil.
func (sw *Writer) WriteFile(name string, size int64, data io.Reader) error {
	if size < 0 {
		return fmt.Errorf("transfer: file %q has negative size %d", name, size)
	}
	if err := sw.reserve(); err != nil {
		return err
	}
	if err := protocol.WriteElementHeader(sw.w, protocol.ElementHeader{Name: name, Size: size}); err != nil {
		return err
	}
	if size == 0 {
		return nil
	}
	if data == nil {
		return fmt.Errorf("transfer: file %q declared %d bytes but reader is nil", name, size)
	}
	_, err := io.CopyN(sw.w, data, size)
	return err
}

// WriteText emits the magic text-snippet element. Per the protocol this
// element must be the only one in the session.
func (sw *Writer) WriteText(text string) error {
	return sw.WriteFile(protocol.TextElementName, int64(len(text)), strings.NewReader(text))
}

// Done verifies that the declared number of elements has been written.
// Callers that forget this will silently ship short sessions; receivers tend
// to hang waiting for the missing elements.
func (sw *Writer) Done() error {
	if sw.remaining != 0 {
		return fmt.Errorf("transfer: %d elements not written", sw.remaining)
	}
	return nil
}

func (sw *Writer) reserve() error {
	if sw.remaining == 0 {
		return ErrStreamOverrun
	}
	sw.remaining--
	return nil
}

// Element is one parsed element from a session. Data is a bounded reader
// scoped to Header.Size bytes — the caller must drain or discard it before
// asking the Reader for the next Element (the Reader will auto-drain leftover
// bytes if the caller forgets, but this defeats the streaming model and wastes
// memory on large files). Data is nil for directory entries.
type Element struct {
	Header protocol.ElementHeader
	Data   io.Reader
}

// IsDirectory mirrors protocol.ElementHeader.IsDirectory.
func (e Element) IsDirectory() bool { return e.Header.IsDirectory() }

// IsText mirrors protocol.ElementHeader.IsText.
func (e Element) IsText() bool { return e.Header.IsText() }

// Reader streams elements from a session. Create with NewReader, call Next()
// until it returns io.EOF.
type Reader struct {
	r         *bufio.Reader
	Header    protocol.SessionHeader
	remaining uint64
	current   *io.LimitedReader
}

// NewReader reads the 16-byte session header and returns a Reader positioned
// just before the first element. If r is not already a *bufio.Reader one is
// wrapped around it.
func NewReader(r io.Reader) (*Reader, error) {
	br, ok := r.(*bufio.Reader)
	if !ok {
		br = bufio.NewReader(r)
	}
	hdr, err := protocol.ReadSessionHeader(br)
	if err != nil {
		return nil, err
	}
	return &Reader{r: br, Header: hdr, remaining: hdr.TotalElements}, nil
}

// Next returns the next element or io.EOF when all declared elements have been
// consumed. If the previous element's Data was not fully read, it is drained
// up to the next element boundary first.
func (sr *Reader) Next() (Element, error) {
	if sr.current != nil && sr.current.N > 0 {
		if _, err := io.Copy(io.Discard, sr.current); err != nil {
			return Element{}, err
		}
	}
	sr.current = nil
	if sr.remaining == 0 {
		return Element{}, io.EOF
	}
	hdr, err := protocol.ReadElementHeader(sr.r)
	if err != nil {
		return Element{}, err
	}
	sr.remaining--
	if hdr.IsDirectory() {
		return Element{Header: hdr}, nil
	}
	sr.current = &io.LimitedReader{R: sr.r, N: hdr.Size}
	return Element{Header: hdr, Data: sr.current}, nil
}

// Remaining reports the number of elements not yet returned by Next.
func (sr *Reader) Remaining() uint64 { return sr.remaining }
