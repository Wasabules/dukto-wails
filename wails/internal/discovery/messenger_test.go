package discovery

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"net/netip"
	"testing"
	"time"

	"dukto/internal/protocol"
)

// newTestMessenger builds a Messenger wired up for unit testing: no socket is
// opened, so only the decision logic (handleDatagram / dispatch / emit) is
// exercised. Callers can still poll Events() and inspect Peers().
func newTestMessenger(port uint16, sig string) *Messenger {
	ifaces := func() ([]Interface, error) { return nil, nil }
	m := New(Config{
		Port:          port,
		SignatureFunc: func() string { return sig },
		Interfaces:    ifaces,
	})
	m.now = func() time.Time { return time.Unix(0, 0) }
	return m
}

// drainEvent reads one event with a short timeout. Fails the test if none
// arrives — the discovery path under test should always emit synchronously.
func drainEvent(t *testing.T, m *Messenger) Event {
	t.Helper()
	select {
	case ev := <-m.Events():
		return ev
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected an event, got none")
		return Event{}
	}
}

func assertNoEvent(t *testing.T, m *Messenger) {
	t.Helper()
	select {
	case ev := <-m.Events():
		t.Fatalf("unexpected event: %+v", ev)
	case <-time.After(20 * time.Millisecond):
	}
}

func TestHandleDatagram_HelloBroadcast_AddsPeerAndEmits(t *testing.T) {
	m := newTestMessenger(protocol.DefaultPort, "me at host (Linux)")
	src := netip.MustParseAddr("10.0.0.2")
	data := protocol.BuddyMessage{Type: protocol.MsgHelloBroadcast, Signature: "alice at box (Linux)"}.Serialize()

	m.handleDatagram(data, src)

	ev := drainEvent(t, m)
	if ev.Kind != EventFound {
		t.Fatalf("Kind = %v, want EventFound", ev.Kind)
	}
	if ev.Peer.Addr != src || ev.Peer.Port != protocol.DefaultPort || ev.Peer.Signature != "alice at box (Linux)" {
		t.Fatalf("unexpected peer: %+v", ev.Peer)
	}
	peers := m.Peers()
	if len(peers) != 1 || peers[0].Addr != src {
		t.Fatalf("peers map not updated: %+v", peers)
	}
}

func TestHandleDatagram_HelloUnicast_AddsPeerNoReply(t *testing.T) {
	// We can't easily observe that *no* unicast reply is sent without a real
	// socket, but we can at least confirm the peer is recorded and an event is
	// emitted exactly like a broadcast HELLO.
	m := newTestMessenger(protocol.DefaultPort, "me at host (Linux)")
	src := netip.MustParseAddr("10.0.0.3")
	data := protocol.BuddyMessage{Type: protocol.MsgHelloUnicast, Signature: "bob at bx (Macintosh)"}.Serialize()

	m.handleDatagram(data, src)

	ev := drainEvent(t, m)
	if ev.Kind != EventFound || ev.Peer.Signature != "bob at bx (Macintosh)" {
		t.Fatalf("unexpected event: %+v", ev)
	}
}

func TestHandleDatagram_Goodbye_RemovesPeer(t *testing.T) {
	m := newTestMessenger(protocol.DefaultPort, "me at host (Linux)")
	src := netip.MustParseAddr("10.0.0.4")

	m.handleDatagram(protocol.BuddyMessage{Type: protocol.MsgHelloBroadcast, Signature: "c at d (Linux)"}.Serialize(), src)
	_ = drainEvent(t, m) // EventFound

	m.handleDatagram(protocol.Goodbye().Serialize(), src)
	ev := drainEvent(t, m)
	if ev.Kind != EventGone || ev.Peer.Addr != src {
		t.Fatalf("unexpected event: %+v", ev)
	}
	if len(m.Peers()) != 0 {
		t.Fatalf("peer should have been removed, have %+v", m.Peers())
	}
}

func TestHandleDatagram_GoodbyeUnknownPeer_NoEvent(t *testing.T) {
	m := newTestMessenger(protocol.DefaultPort, "me at host (Linux)")
	src := netip.MustParseAddr("10.0.0.99")

	m.handleDatagram(protocol.Goodbye().Serialize(), src)
	assertNoEvent(t, m)
}

func TestHandleDatagram_HelloPort_NotStoredButPortTracked(t *testing.T) {
	// Qt parity: 0x04/0x05 peers never land in the peers map; only their port
	// is recorded so our next broadcast reaches them.
	m := newTestMessenger(protocol.DefaultPort, "me at host (Linux)")
	src := netip.MustParseAddr("10.0.0.5")
	data := protocol.BuddyMessage{Type: protocol.MsgHelloPortBroadcast, Port: 5000, Signature: "e at f (Windows)"}.Serialize()

	m.handleDatagram(data, src)

	ev := drainEvent(t, m)
	if ev.Kind != EventFound || ev.Peer.Port != 5000 {
		t.Fatalf("unexpected event: %+v", ev)
	}
	if got := m.Peers(); len(got) != 0 {
		t.Fatalf("PORT peer must not be stored, got %+v", got)
	}
	m.mu.Lock()
	_, tracked := m.ports[5000]
	m.mu.Unlock()
	if !tracked {
		t.Fatal("port 5000 should be tracked after HELLO_PORT")
	}
}

func TestHandleDatagram_MalformedDatagramIgnored(t *testing.T) {
	m := newTestMessenger(protocol.DefaultPort, "me at host (Linux)")
	m.handleDatagram([]byte{0x99, 'x'}, netip.MustParseAddr("10.0.0.6"))
	assertNoEvent(t, m)
	if len(m.Peers()) != 0 {
		t.Fatalf("no peer expected, got %+v", m.Peers())
	}
}

func TestHandleDatagram_SelfEchoSuppression(t *testing.T) {
	m := newTestMessenger(protocol.DefaultPort, "me at host (Linux)")
	self := netip.MustParseAddr("10.0.0.7")
	// Mark self as a local IP (as broadcast() would).
	m.mu.Lock()
	m.localIPs[self] = 0
	m.mu.Unlock()

	data := protocol.BuddyMessage{Type: protocol.MsgHelloBroadcast, Signature: "me at me (Linux)"}.Serialize()
	m.handleDatagram(data, self)

	assertNoEvent(t, m)
	if len(m.Peers()) != 0 {
		t.Fatalf("self echo must not add a peer, got %+v", m.Peers())
	}
	m.mu.Lock()
	count := m.localIPs[self]
	m.mu.Unlock()
	if count != 1 {
		t.Fatalf("self-echo counter = %d, want 1", count)
	}
}

func TestHandleDatagram_BroadcastStormGuard(t *testing.T) {
	// After > broadcastStormThreshold (5) echoes from a self-address, the
	// source is quarantined in badIPs. Further datagrams are dropped even
	// before parsing.
	m := newTestMessenger(protocol.DefaultPort, "me at host (Linux)")
	self := netip.MustParseAddr("10.0.0.8")
	m.mu.Lock()
	m.localIPs[self] = 0
	m.mu.Unlock()

	data := protocol.BuddyMessage{Type: protocol.MsgHelloBroadcast, Signature: "x at y (Linux)"}.Serialize()
	for range broadcastStormThreshold + 2 {
		m.handleDatagram(data, self)
	}

	m.mu.Lock()
	_, quarantined := m.badIPs[self]
	m.mu.Unlock()
	if !quarantined {
		t.Fatal("source should be quarantined after exceeding storm threshold")
	}
	assertNoEvent(t, m)

	// Even a "legitimate" datagram from a quarantined address is now dropped.
	m.handleDatagram(data, self)
	assertNoEvent(t, m)
}

func TestMessenger_EventsBuffer_DropsUnderBackpressure(t *testing.T) {
	// emit() uses a drop-oldest policy so that a slow consumer can never stall
	// the receive loop. Fill the buffer, push one more, then verify the
	// oldest event was dropped.
	m := newTestMessenger(protocol.DefaultPort, "me at host (Linux)")
	for i := range cap(m.events) {
		m.events <- Event{Kind: EventFound, Peer: Peer{Port: uint16(i)}}
	}
	m.emit(Event{Kind: EventFound, Peer: Peer{Port: 9999}})

	if len(m.events) != cap(m.events) {
		t.Fatalf("buffer len = %d, want %d", len(m.events), cap(m.events))
	}
	first := <-m.events
	if first.Peer.Port == 0 {
		t.Fatalf("oldest event should have been dropped; got Port=0 still present")
	}
}

func TestDirectedBroadcast(t *testing.T) {
	cases := []struct {
		ip, mask, want string
	}{
		{"192.168.1.10", "255.255.255.0", "192.168.1.255"},
		{"10.0.0.1", "255.0.0.0", "10.255.255.255"},
		{"172.16.5.4", "255.255.0.0", "172.16.255.255"},
	}
	for _, c := range cases {
		ip := netip.MustParseAddr(c.ip).As4()
		want := netip.MustParseAddr(c.want).As4()
		got := directedBroadcast(ip[:], parseMaskV4(t, c.mask))
		if netip.AddrFrom4([4]byte(got)) != netip.AddrFrom4(want) {
			t.Errorf("%s/%s: got %v, want %v", c.ip, c.mask, got, c.want)
		}
	}
}

func parseMaskV4(t *testing.T, s string) []byte {
	t.Helper()
	a := netip.MustParseAddr(s).As4()
	return a[:]
}

func TestHandleDatagram_HelloPortKey_VerifiesAndStampsV2(t *testing.T) {
	m := newTestMessenger(protocol.DefaultPort, "me at host (Linux)")
	src := netip.MustParseAddr("10.0.0.7")
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	msg := protocol.SignBuddyMessage(
		protocol.BuddyMessage{Type: protocol.MsgHelloPortKeyBroadcast, Port: 4644, Signature: "bob at phone (Android)"},
		pub, priv,
	)

	m.handleDatagram(msg.Serialize(), src)

	ev := drainEvent(t, m)
	if ev.Kind != EventFound {
		t.Fatalf("kind = %v", ev.Kind)
	}
	if !ev.Peer.V2Capable {
		t.Fatal("expected V2Capable to be true on a verified 0x06 datagram")
	}
	if !bytes.Equal(ev.Peer.PubKey, pub) {
		t.Fatal("expected emitted PubKey to match the signing key")
	}

	// A subsequent legacy HELLO from the same source should now be stamped v2 too.
	legacy := protocol.BuddyMessage{Type: protocol.MsgHelloBroadcast, Signature: "bob at phone (Android)"}.Serialize()
	m.handleDatagram(legacy, src)
	ev2 := drainEvent(t, m)
	if !ev2.Peer.V2Capable || !bytes.Equal(ev2.Peer.PubKey, pub) {
		t.Fatalf("legacy HELLO from a v2-known IP should inherit the key, got %+v", ev2.Peer)
	}
}

func TestHandleDatagram_HelloPortKey_RejectsBadSignature(t *testing.T) {
	m := newTestMessenger(protocol.DefaultPort, "me at host (Linux)")
	src := netip.MustParseAddr("10.0.0.8")
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	msg := protocol.SignBuddyMessage(
		protocol.BuddyMessage{Type: protocol.MsgHelloPortKeyBroadcast, Port: 4644, Signature: "bob at phone (Android)"},
		pub, priv,
	)
	wire := msg.Serialize()
	// Flip a bit inside the signature payload (last byte of sig field). The
	// Ed25519 signature is at offset 1 + 2 + 32 = 35; the field is 64 bytes.
	wire[35+63] ^= 0x01

	m.handleDatagram(wire, src)
	assertNoEvent(t, m)
	// And no v2 record should have been written.
	if m.lookupV2Key(src) != nil {
		t.Fatal("rejected datagrams should not register a v2 key")
	}
}
