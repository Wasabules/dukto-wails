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

// Send opens sources in order and streams a full session to w. It does not
// dial or close a socket; the caller supplies w — typically a *net.TCPConn.
// sendBufSize controls the io.Copy staging buffer (0 uses the default).
//
// The header must describe sources exactly — if it declares more or fewer
// elements than len(sources), Send returns an error after the mismatch is
// detected (either from Writer.Done or mid-stream).
func Send(w io.Writer, sources []Source, hdr protocol.SessionHeader) error {
	sw, err := NewWriter(w, hdr)
	if err != nil {
		return fmt.Errorf("transfer: write session header: %w", err)
	}
	for _, s := range sources {
		switch {
		case s.IsText():
			if err := sw.WriteText(s.Text); err != nil {
				return fmt.Errorf("transfer: write text element: %w", err)
			}
		case s.IsDirectory():
			if err := sw.WriteDir(s.Name); err != nil {
				return fmt.Errorf("transfer: write dir %q: %w", s.Name, err)
			}
		default:
			if err := sendFile(sw, s); err != nil {
				return err
			}
		}
	}
	return sw.Done()
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
// half-closes writes. Cancelling ctx closes the connection.
func Dial(ctx context.Context, peer netip.AddrPort, sources []Source, hdr protocol.SessionHeader) error {
	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp4", peer.String())
	if err != nil {
		return fmt.Errorf("transfer: dial %s: %w", peer, err)
	}
	defer conn.Close()

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
	if err := Send(bw, sources, hdr); err != nil {
		return err
	}
	if err := bw.Flush(); err != nil {
		return fmt.Errorf("transfer: flush: %w", err)
	}
	// The Qt receiver stops after byte accounting (§3.5), so we can half-close
	// to signal end-of-send. Ignore errors — the receiver may have closed
	// first.
	if tcp, ok := conn.(*net.TCPConn); ok {
		_ = tcp.CloseWrite()
	}
	return nil
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
