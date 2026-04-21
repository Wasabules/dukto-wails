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
	if err := rc.Handle(ctx, conn); err != nil && s.OnAcceptError != nil {
		s.OnAcceptError(err)
	}
}
