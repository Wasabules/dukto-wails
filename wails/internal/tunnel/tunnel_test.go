package tunnel

import (
	"bytes"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"dukto/internal/identity"
)

// inMemoryPipe wraps net.Pipe to expose net.Conn ends with deterministic
// timing characteristics for our handshake tests.
func inMemoryPipe() (net.Conn, net.Conn) { return net.Pipe() }

// loadIdent generates an in-memory ephemeral identity and returns the
// X25519 keypair derived from it. Used as a test fixture so each test
// gets a fresh long-term key without touching disk.
func loadIdent(t *testing.T) ([32]byte, [32]byte) {
	t.Helper()
	dir := t.TempDir()
	id, err := identity.LoadOrGenerate(dir + "/identity.key")
	if err != nil {
		t.Fatal(err)
	}
	priv := id.X25519Private()
	pub, err := id.X25519Public()
	if err != nil {
		t.Fatal(err)
	}
	return priv, pub
}

func TestPeekMagic_DetectsV2(t *testing.T) {
	a, b := inMemoryPipe()
	go func() {
		_, _ = a.Write(Magic[:])
		_ = a.Close()
	}()
	v2, peeked, err := PeekMagic(b)
	if err != nil {
		t.Fatal(err)
	}
	if !v2 {
		t.Fatal("expected v2 detection")
	}
	// Reading more should return EOF (the writer closed).
	buf := make([]byte, 1)
	if _, err := peeked.Read(buf); !errors.Is(err, io.EOF) {
		t.Fatalf("expected EOF, got %v", err)
	}
}

func TestPeekMagic_LegacyReplaysBytes(t *testing.T) {
	a, b := inMemoryPipe()
	// Legacy senders open with a SessionHeader directly. The first 8 bytes
	// happen to be the LE encoding of TotalElements + start of TotalSize —
	// not equal to the magic.
	wantPrefix := []byte{0x05, 0, 0, 0, 0, 0, 0, 0} // TotalElements=5
	body := []byte("trailing")
	go func() {
		_, _ = a.Write(append(append([]byte{}, wantPrefix...), body...))
		_ = a.Close()
	}()
	v2, peeked, err := PeekMagic(b)
	if err != nil {
		t.Fatal(err)
	}
	if v2 {
		t.Fatal("non-magic prefix should not be detected as v2")
	}
	got, err := io.ReadAll(&peeked)
	if err != nil {
		t.Fatal(err)
	}
	want := append(append([]byte{}, wantPrefix...), body...)
	if !bytes.Equal(got, want) {
		t.Fatalf("legacy fallback dropped bytes\n got % x\n want % x", got, want)
	}
}

// TestHandshake_RoundTrip is the canonical happy path: two peers with
// independent identities complete a Noise XX handshake and exchange
// encrypted application data both ways.
func TestHandshake_RoundTrip(t *testing.T) {
	clientPriv, clientPub := loadIdent(t)
	serverPriv, serverPub := loadIdent(t)

	ca, cb := inMemoryPipe()

	var serverSess *Session
	var serverPeerStatic []byte
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Responder side mirrors what transfer.Server will do: peek the
		// magic, then run the handshake on the peeked connection.
		isV2, peeked, err := PeekMagic(cb)
		if err != nil || !isV2 {
			t.Errorf("server: peek failed (v2=%v err=%v)", isV2, err)
			return
		}
		s, err := Handshake(&peeked, RoleResponder, serverPriv, serverPub, nil)
		if err != nil {
			t.Errorf("server handshake: %v", err)
			return
		}
		serverSess = s
		serverPeerStatic = s.RemoteStatic()
	}()

	clientSess, err := Handshake(ca, RoleInitiator, clientPriv, clientPub, nil)
	if err != nil {
		t.Fatalf("client handshake: %v", err)
	}
	wg.Wait()
	if serverSess == nil {
		t.Fatal("server handshake did not complete")
	}

	if !bytes.Equal(clientSess.RemoteStatic(), serverPub[:]) {
		t.Fatal("client RemoteStatic() does not match the server's pub")
	}
	if !bytes.Equal(serverPeerStatic, clientPub[:]) {
		t.Fatal("server RemoteStatic() does not match the client's pub")
	}

	// Bidirectional transport: payload is large enough to span multiple
	// Noise frames so we exercise frame splitting on the way out and
	// reassembly on the way in.
	payload := bytes.Repeat([]byte("DUKTO"), 4000) // ~20 KiB > one Noise frame
	got := make([]byte, len(payload))
	var rwg sync.WaitGroup
	rwg.Add(1)
	go func() {
		defer rwg.Done()
		if _, err := io.ReadFull(structAsReader{serverSess}, got); err != nil {
			t.Errorf("server read: %v", err)
		}
	}()
	if _, err := clientSess.Write(payload); err != nil {
		t.Fatalf("client write: %v", err)
	}
	rwg.Wait()
	if !bytes.Equal(got, payload) {
		t.Fatal("plaintext mangled by the tunnel")
	}

	reply := []byte("OK")
	got2 := make([]byte, len(reply))
	rwg.Add(1)
	go func() {
		defer rwg.Done()
		if _, err := io.ReadFull(structAsReader{clientSess}, got2); err != nil {
			t.Errorf("client read: %v", err)
		}
	}()
	if _, err := serverSess.Write(reply); err != nil {
		t.Fatalf("server write: %v", err)
	}
	rwg.Wait()
	if !bytes.Equal(got2, reply) {
		t.Fatal("reply plaintext mangled")
	}
}

// TestHandshake_PSKRequiresMatch covers the XXpsk2 first-pairing path: a
// matching PSK on both sides completes the handshake; mismatched PSKs make
// it fail.
func TestHandshake_PSKRequiresMatch(t *testing.T) {
	priv1, pub1 := loadIdent(t)
	priv2, pub2 := loadIdent(t)

	good := bytes.Repeat([]byte{0xAB}, 32)

	t.Run("matching", func(t *testing.T) {
		ca, cb := inMemoryPipe()
		var serverErr error
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			isV2, peeked, err := PeekMagic(cb)
			if err != nil || !isV2 {
				serverErr = err
				return
			}
			_, serverErr = Handshake(&peeked, RoleResponder, priv2, pub2, good)
		}()
		_, err := Handshake(ca, RoleInitiator, priv1, pub1, good)
		wg.Wait()
		if err != nil {
			t.Fatalf("client: %v", err)
		}
		if serverErr != nil {
			t.Fatalf("server: %v", serverErr)
		}
	})

	t.Run("mismatch", func(t *testing.T) {
		// Set a deadline on both ends so the test can't deadlock if the
		// handshake breaks down at a step that needs both sides to make
		// progress (XXpsk2 mismatch surfaces on the responder's third-
		// message read, but the initiator may already have committed
		// keys at that point).
		ca, cb := inMemoryPipe()
		t.Cleanup(func() { _ = ca.Close(); _ = cb.Close() })
		deadline := time.Now().Add(2 * time.Second)
		_ = ca.SetDeadline(deadline)
		_ = cb.SetDeadline(deadline)

		bad := bytes.Repeat([]byte{0xCD}, 32)
		var serverErr error
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			isV2, peeked, err := PeekMagic(cb)
			if err != nil || !isV2 {
				serverErr = err
				return
			}
			_, serverErr = Handshake(&peeked, RoleResponder, priv2, pub2, bad)
		}()
		_, err := Handshake(ca, RoleInitiator, priv1, pub1, good)
		wg.Wait()
		if err == nil && serverErr == nil {
			t.Fatal("PSK mismatch should reject the handshake on at least one side")
		}
	})
}

// structAsReader exposes a *Session as an io.Reader for use with
// io.ReadFull. Session.Read is already an io.Reader-shaped method but Go's
// type system requires the explicit indirection in tests where we hand a
// pointer to multiple test variables.
type structAsReader struct{ s *Session }

func (r structAsReader) Read(p []byte) (int, error) { return r.s.Read(p) }
