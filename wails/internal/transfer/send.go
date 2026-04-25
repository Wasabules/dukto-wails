package transfer

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"strings"

	"dukto/internal/protocol"
)

// Sender carries per-send optional behavior. The zero value is valid and
// matches the previous package-level Dial/Send semantics.
type Sender struct {
	// OnProgress, if set, reports cumulative bytes sent for this session.
	// Header bytes are not counted; only element payloads are. The callback
	// is throttled — see progressStride.
	OnProgress ProgressFunc

	// Upgrade, if set, is invoked by Dial right after dialing and before
	// any session bytes are written. It can return a wrapped net.Conn —
	// the Dukto v2 tunnel installs a Noise XX session here so subsequent
	// writes go through ChaCha20-Poly1305. Returning a non-nil error
	// closes the dialled conn and aborts Dial.
	Upgrade func(net.Conn) (net.Conn, error)

	// bytesSent is the running counter shared across counting writers during
	// a single Send() call. Reset per session in Send.
	bytesSent int64
}

// Send opens sources in order and streams a full session to w. It does not
// dial or close a socket; the caller supplies w — typically a *net.TCPConn.
//
// The header must describe sources exactly — if it declares more or fewer
// elements than len(sources), Send returns an error after the mismatch is
// detected (either from Writer.Done or mid-stream).
func (s *Sender) Send(w io.Writer, sources []Source, hdr protocol.SessionHeader) error {
	s.bytesSent = 0
	stride := progressStride(hdr.TotalSize)
	pw := s.wrapProgress(w, hdr.TotalSize, stride)
	sw, err := NewWriter(pw, hdr)
	if err != nil {
		return fmt.Errorf("transfer: write session header: %w", err)
	}
	for _, src := range sources {
		switch {
		case src.IsText():
			if err := sw.WriteText(src.Text); err != nil {
				return fmt.Errorf("transfer: write text element: %w", err)
			}
		case src.IsDirectory():
			if err := sw.WriteDir(src.Name); err != nil {
				return fmt.Errorf("transfer: write dir %q: %w", src.Name, err)
			}
		default:
			if err := sendFile(sw, src); err != nil {
				return err
			}
		}
	}
	if err := sw.Done(); err != nil {
		return err
	}
	// Terminal tick so the UI bar reaches 100% even if the last file's stride
	// didn't align with the session end.
	if s.OnProgress != nil && hdr.TotalSize > 0 {
		s.OnProgress(s.bytesSent, hdr.TotalSize)
	}
	return nil
}

// wrapProgress returns w, possibly wrapped in a counter. See receive.wrapProgress.
func (s *Sender) wrapProgress(w io.Writer, total, stride int64) io.Writer {
	if s.OnProgress == nil {
		return w
	}
	return &countingWriter{
		w:       w,
		counter: &s.bytesSent,
		total:   total,
		cb:      s.OnProgress,
		stride:  stride,
	}
}

// Send is the progress-less convenience wrapper for the Sender.Send method.
// Existing callers (and tests) stay on this signature.
func Send(w io.Writer, sources []Source, hdr protocol.SessionHeader) error {
	return (&Sender{}).Send(w, sources, hdr)
}

func sendFile(sw *Writer, s Source) error {
	if s.Size == 0 {
		return sw.WriteFile(s.Name, 0, nil)
	}
	f, err := os.Open(s.LocalPath)
	if err != nil {
		return fmt.Errorf("transfer: open %q: %w", s.LocalPath, err)
	}
	defer f.Close()
	if err := sw.WriteFile(s.Name, s.Size, f); err != nil {
		return fmt.Errorf("transfer: stream %q: %w", s.Name, err)
	}
	return nil
}

// Dial connects to peer and streams the session to the TCP socket, then
// half-closes writes. Cancelling ctx closes the connection. When Upgrade
// is set on the Sender it runs after the dial completes, wrapping the
// raw conn in a v2 Noise session before Send is called.
func (s *Sender) Dial(ctx context.Context, peer netip.AddrPort, sources []Source, hdr protocol.SessionHeader) error {
	dialer := net.Dialer{}
	rawConn, err := dialer.DialContext(ctx, "tcp4", peer.String())
	if err != nil {
		return fmt.Errorf("transfer: dial %s: %w", peer, err)
	}
	defer rawConn.Close()

	conn := rawConn
	if s.Upgrade != nil {
		upgraded, uerr := s.Upgrade(rawConn)
		if uerr != nil {
			return fmt.Errorf("transfer: upgrade: %w", uerr)
		}
		conn = upgraded
		// If Upgrade returned a different conn (e.g. tunnel.Session
		// wrapping the raw socket), make sure we close it on exit too.
		if upgraded != rawConn {
			defer upgraded.Close()
		}
	}

	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-done:
		}
	}()

	bw := bufio.NewWriter(conn)
	if err := s.Send(bw, sources, hdr); err != nil {
		return err
	}
	if err := bw.Flush(); err != nil {
		return fmt.Errorf("transfer: flush: %w", err)
	}
	// The Qt receiver stops after byte accounting (§3.5), so we can half-close
	// to signal end-of-send. Ignore errors — the receiver may have closed
	// first. The half-close has to be done on the *raw* TCP socket, not on
	// the v2 session wrapper which has no notion of FIN.
	if tcp, ok := rawConn.(*net.TCPConn); ok {
		_ = tcp.CloseWrite()
	}
	return nil
}

// Dial is the progress-less convenience wrapper for Sender.Dial.
func Dial(ctx context.Context, peer netip.AddrPort, sources []Source, hdr protocol.SessionHeader) error {
	return (&Sender{}).Dial(ctx, peer, sources, hdr)
}

// SendText is a convenience that dials peer and sends a single text snippet.
func SendText(ctx context.Context, peer netip.AddrPort, text string) error {
	sources, hdr := TextSource(text)
	return Dial(ctx, peer, sources, hdr)
}

// QuoteSourceNames returns a comma-separated preview of source wire names,
// handy for log lines. Long lists are truncated with an ellipsis.
func QuoteSourceNames(sources []Source, max int) string {
	if max <= 0 {
		max = 3
	}
	var names []string
	for i, s := range sources {
		if i >= max {
			names = append(names, fmt.Sprintf("…(+%d more)", len(sources)-max))
			break
		}
		names = append(names, s.Name)
	}
	return strings.Join(names, ", ")
}
