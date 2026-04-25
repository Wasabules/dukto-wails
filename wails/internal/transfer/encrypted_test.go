package transfer_test

// End-to-end integration test: a Server with a v2 Upgrade hook accepts an
// encrypted session from a Sender with a matching Upgrade hook, and the
// plaintext text snippet round-trips. Lives in a `_test` package so the
// import of internal/tunnel is allowed.

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"sync"
	"testing"
	"time"

	"dukto/internal/tunnel"

	"dukto/internal/identity"
	"dukto/internal/transfer"
)

func TestEncryptedRoundTripWithUpgradeHooks(t *testing.T) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	dst := t.TempDir()

	// One ephemeral identity per side. Keypairs are generated in-memory
	// with crypto/rand to keep the test hermetic.
	clientID, serverID := newTestIdentity(t), newTestIdentity(t)

	var got string
	var encrypted bool
	var mu sync.Mutex
	srv := &transfer.Server{
		NewReceiver: func() *transfer.Receiver {
			return &transfer.Receiver{Dest: dst, OnEvent: func(ev transfer.ReceiveEvent) error {
				if ev.Kind == transfer.EventTextReceived {
					mu.Lock()
					got = ev.Text
					mu.Unlock()
				}
				return nil
			}}
		},
		Upgrade: func(c net.Conn) (net.Conn, bool, error) {
			isV2, peeked, err := tunnel.PeekMagic(c)
			if err != nil {
				return nil, false, err
			}
			if !isV2 {
				return &peeked, false, nil
			}
			priv, pub := serverID.X25519Private(), mustPub(t, serverID)
			sess, err := tunnel.Handshake(&peeked, tunnel.RoleResponder, priv, pub, nil)
			if err != nil {
				return nil, false, err
			}
			return sess, true, nil
		},
		OnSessionMode: func(enc bool) {
			mu.Lock()
			encrypted = enc
			mu.Unlock()
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	serveDone := make(chan error, 1)
	go func() { serveDone <- srv.Serve(ctx, ln) }()

	peerAddr := ln.Addr().(*net.TCPAddr).AddrPort()
	srcs, hdr := transfer.TextSource("encrypted hi")
	sender := &transfer.Sender{
		Upgrade: func(c net.Conn) (net.Conn, error) {
			priv, pub := clientID.X25519Private(), mustPub(t, clientID)
			return tunnel.Handshake(c, tunnel.RoleInitiator, priv, pub, nil)
		},
	}
	if err := sender.Dial(ctx, peerAddr, srcs, hdr); err != nil {
		t.Fatalf("encrypted Dial: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		if got == "encrypted hi" && encrypted {
			mu.Unlock()
			cancel()
			<-serveDone
			return
		}
		mu.Unlock()
		time.Sleep(5 * time.Millisecond)
	}
	mu.Lock()
	t.Fatalf("server never delivered the encrypted snippet (got=%q encrypted=%v)", got, encrypted)
}

// newTestIdentity generates an ephemeral Ed25519 identity. Persisting to
// disk (the production path) would force every test run to mkdir/rename,
// which isn't worth it for a deterministic in-memory generation.
func newTestIdentity(t *testing.T) identity.Identity {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return identity.Identity{Public: pub, Private: priv}
}

func mustPub(t *testing.T, id identity.Identity) [32]byte {
	t.Helper()
	pub, err := id.X25519Public()
	if err != nil {
		t.Fatal(err)
	}
	return pub
}
