package transfer

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestServer_Serve_AcceptsSession(t *testing.T) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	dst := t.TempDir()

	var got string
	var mu sync.Mutex
	srv := &Server{
		NewReceiver: func() *Receiver {
			return &Receiver{Dest: dst, OnEvent: func(ev ReceiveEvent) error {
				if ev.Kind == EventTextReceived {
					mu.Lock()
					got = ev.Text
					mu.Unlock()
				}
				return nil
			}}
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	serveDone := make(chan error, 1)
	go func() { serveDone <- srv.Serve(ctx, ln) }()

	peer := ln.Addr().(*net.TCPAddr).AddrPort()
	if err := SendText(ctx, peer, "hi"); err != nil {
		t.Fatal(err)
	}
	// Give the server goroutine a moment to finalize the receive.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		if got == "hi" {
			mu.Unlock()
			break
		}
		mu.Unlock()
		time.Sleep(5 * time.Millisecond)
	}
	if got != "hi" {
		t.Fatalf("server did not deliver text, got %q", got)
	}
	cancel()
	<-serveDone
}

func TestServer_RejectsSecondConcurrentConnection(t *testing.T) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	// Block the first receive indefinitely so the in-flight guard stays
	// engaged while we open a second connection.
	release := make(chan struct{})
	var started atomic.Int32
	newRx := func() *Receiver {
		return &Receiver{Dest: t.TempDir(), OnEvent: func(ev ReceiveEvent) error {
			if ev.Kind == EventSessionStart {
				started.Add(1)
				<-release
			}
			return nil
		}}
	}
	srv := &Server{NewReceiver: newRx}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.Serve(ctx, ln)

	peer := ln.Addr().(*net.TCPAddr).AddrPort()

	// First connection: send enough that SessionStart fires, then the
	// handler stalls on release. We have to keep the socket open so the
	// Receiver doesn't see EOF.
	firstConn, err := net.DialTimeout("tcp4", peer.String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer firstConn.Close()
	// Write a session header so the receiver enters EventSessionStart.
	if _, err := firstConn.Write(sessionHeaderBytes(1, 0)); err != nil {
		t.Fatal(err)
	}

	// Wait for the handler to reach the blocking point.
	deadline := time.Now().Add(time.Second)
	for started.Load() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("first handler never started")
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Second connection: should be dropped immediately by the server.
	secondConn, err := net.DialTimeout("tcp4", peer.String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer secondConn.Close()
	// The server closes the second connection before reading anything, so
	// a read on it returns EOF quickly. Give the server a moment to do so.
	_ = secondConn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	buf := make([]byte, 1)
	_, err = secondConn.Read(buf)
	if err == nil {
		t.Fatal("expected second connection to be closed by server")
	}
	close(release)
}

// sessionHeaderBytes builds a 16-byte little-endian session header for tests.
func sessionHeaderBytes(elements uint64, size int64) []byte {
	out := make([]byte, 16)
	for i := range 8 {
		out[i] = byte(elements >> (8 * i))
	}
	u := uint64(size)
	for i := range 8 {
		out[8+i] = byte(u >> (8 * i))
	}
	return out
}
