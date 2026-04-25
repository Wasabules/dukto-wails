package transfer

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
)

// Server accepts inbound Dukto TCP sessions and feeds each one to a Receiver.
//
// Qt's reference implementation rejects a second inbound connection while one
// is already in progress (network/receiver.cpp). We preserve that invariant:
// once a transfer is active, any further Accept returns an immediately-closed
// connection. Without the guard, the Receiver would interleave element data
// from two streams and produce corrupted output.
type Server struct {
	// NewReceiver is called once per accepted connection and must return a
	// ready-to-use Receiver. Must not be nil.
	NewReceiver func() *Receiver

	// OnAcceptError, if non-nil, is invoked when an Accept or Handle call
	// fails. Errors are otherwise silently dropped, because the listener loop
	// must keep running across transient failures (client abandoning a
	// connection mid-handshake, reset sockets on sleep/wake, etc).
	OnAcceptError func(error)

	// OnSessionStart, if non-nil, is invoked right before Receiver.Handle
	// starts. The callback receives a cancel fn that, when called, aborts
	// the in-flight session (closes the connection and returns ctx.Err()
	// from Handle). Used by the UI to offer a "cancel transfer" button.
	OnSessionStart func(cancel context.CancelFunc)
	// OnSessionEnd, if non-nil, is invoked after Handle returns, regardless
	// of error. Used to clear the UI cancel state.
	OnSessionEnd func()

	// Allow, if non-nil, is consulted before Handle runs. Returning false
	// causes the connection to be closed immediately without any Receiver
	// work. It receives the full net.Conn so checks can examine both the
	// remote and the local address (used for per-interface gating, block
	// list, rate-limiting, audit logging).
	Allow func(conn net.Conn) bool

	// Upgrade, if non-nil, runs right after Allow and before Receiver.Handle.
	// It is given the raw conn and may either:
	//   - return the same conn (legacy session — bytes parsed as a
	//     SessionHeader directly), or
	//   - return a wrapped conn carrying the encrypted v2 transport, or
	//   - return a non-nil error to abort the session (e.g. a refused
	//     handshake or a fingerprint mismatch).
	// The bool out-param is reported on the audit log via the OnSessionMode
	// hook (true == encrypted v2, false == legacy cleartext) so the UI
	// can render the correct icon for that activity entry.
	Upgrade func(conn net.Conn) (net.Conn, bool, error)

	// OnSessionMode, if non-nil, is invoked once after Upgrade and before
	// Handle, communicating whether the accepted session is encrypted.
	// Defaults to a no-op.
	OnSessionMode func(encrypted bool)

	inFlight atomic.Bool

	mu sync.Mutex
	ln net.Listener
}

// Serve listens on ln and accepts sessions until ln.Close is called or ctx
// expires. It returns nil on a clean shutdown.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	if s.NewReceiver == nil {
		return errors.New("transfer: Server.NewReceiver must be set")
	}
	s.mu.Lock()
	s.ln = ln
	s.mu.Unlock()

	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = ln.Close()
		case <-done:
		}
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return fmt.Errorf("transfer: accept: %w", err)
		}
		go s.handle(ctx, conn)
	}
}

// Close stops the accept loop. Subsequent Serve returns nil.
func (s *Server) Close() error {
	s.mu.Lock()
	ln := s.ln
	s.mu.Unlock()
	if ln == nil {
		return nil
	}
	return ln.Close()
}

func (s *Server) handle(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	// Policy gate. Closes the socket immediately if the peer isn't
	// allowed — from the sender's perspective this looks like any other
	// refused transfer.
	if s.Allow != nil && !s.Allow(conn) {
		return
	}
	// Single-transfer-at-a-time guard — matches Qt behavior.
	if !s.inFlight.CompareAndSwap(false, true) {
		return
	}
	defer s.inFlight.Store(false)

	rc := s.NewReceiver()
	if rc == nil {
		if s.OnAcceptError != nil {
			s.OnAcceptError(errors.New("transfer: NewReceiver returned nil"))
		}
		return
	}

	sessConn := conn
	encrypted := false
	if s.Upgrade != nil {
		upgraded, isEncrypted, uerr := s.Upgrade(conn)
		if uerr != nil {
			if s.OnAcceptError != nil {
				s.OnAcceptError(uerr)
			}
			return
		}
		if upgraded != nil {
			sessConn = upgraded
			// Make sure the wrapped session also closes when handle returns.
			if upgraded != conn {
				defer upgraded.Close()
			}
		}
		encrypted = isEncrypted
	}
	if s.OnSessionMode != nil {
		s.OnSessionMode(encrypted)
	}

	// Scope a cancellable context to this session so the UI can abort mid
	// transfer without closing the listener. Handle already closes the
	// connection when its ctx is cancelled.
	sessCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	if s.OnSessionStart != nil {
		s.OnSessionStart(cancel)
	}
	if s.OnSessionEnd != nil {
		defer s.OnSessionEnd()
	}
	if err := rc.Handle(sessCtx, sessConn); err != nil && s.OnAcceptError != nil {
		s.OnAcceptError(err)
	}
}
