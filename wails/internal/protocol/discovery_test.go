package protocol

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"testing"
)

// Fixed signature used across tests. Includes non-ASCII to catch any UTF-8
// handling regression. The reference app builds this with ` at ` and
// parentheses as literals; we rely on both on parsing.
const testSig = "Tëst at test-host (Linux)"

func TestBuddyMessage_SerializeHelloBroadcast(t *testing.T) {
	m := BuddyMessage{Type: MsgHelloBroadcast, Signature: testSig}
	got := m.Serialize()
	want := append([]byte{0x01}, []byte(testSig)...)
	if !bytes.Equal(got, want) {
		t.Fatalf("serialize = % x\nwant      = % x", got, want)
	}
}

func TestBuddyMessage_SerializeHelloUnicast(t *testing.T) {
	m := BuddyMessage{Type: MsgHelloUnicast, Signature: testSig}
	got := m.Serialize()
	want := append([]byte{0x02}, []byte(testSig)...)
	if !bytes.Equal(got, want) {
		t.Fatalf("serialize = % x\nwant      = % x", got, want)
	}
}

func TestBuddyMessage_SerializeGoodbye(t *testing.T) {
	m := Goodbye()
	got := m.Serialize()
	want := []byte{0x03}
	if !bytes.Equal(got, want) {
		t.Fatalf("serialize = % x\nwant      = % x", got, want)
	}
}

func TestBuddyMessage_SerializeHelloPortBroadcastLittleEndian(t *testing.T) {
	// 5000 = 0x1388 ⇒ LE bytes 0x88 0x13.
	m := BuddyMessage{Type: MsgHelloPortBroadcast, Port: 5000, Signature: testSig}
	got := m.Serialize()
	want := append([]byte{0x04, 0x88, 0x13}, []byte(testSig)...)
	if !bytes.Equal(got, want) {
		t.Fatalf("serialize = % x\nwant      = % x", got, want)
	}
}

func TestBuddyMessage_SerializeHelloPortUnicastLittleEndian(t *testing.T) {
	// 0xABCD ⇒ LE bytes 0xCD 0xAB. Catches a byte-swap regression.
	m := BuddyMessage{Type: MsgHelloPortUnicast, Port: 0xABCD, Signature: testSig}
	got := m.Serialize()
	want := append([]byte{0x05, 0xCD, 0xAB}, []byte(testSig)...)
	if !bytes.Equal(got, want) {
		t.Fatalf("serialize = % x\nwant      = % x", got, want)
	}
}

func TestBuddyMessage_RoundTrip(t *testing.T) {
	cases := []BuddyMessage{
		{Type: MsgHelloBroadcast, Signature: testSig},
		{Type: MsgHelloUnicast, Signature: testSig},
		{Type: MsgGoodbye},
		{Type: MsgHelloPortBroadcast, Port: 5000, Signature: testSig},
		{Type: MsgHelloPortUnicast, Port: 65535, Signature: testSig},
	}
	for _, want := range cases {
		got, err := ParseBuddyMessage(want.Serialize())
		if err != nil {
			t.Errorf("parse after serialize %+v: %v", want, err)
			continue
		}
		if !buddyEqual(got, want) {
			t.Errorf("round-trip mismatch\n got  %+v\n want %+v", got, want)
		}
	}
}

func buddyEqual(a, b BuddyMessage) bool {
	return a.Type == b.Type && a.Port == b.Port && a.Signature == b.Signature &&
		bytes.Equal(a.PubKey, b.PubKey) && bytes.Equal(a.Sig, b.Sig)
}

func TestBuddyMessage_SerializeHelloPortKeyBroadcast(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	m := SignBuddyMessage(
		BuddyMessage{Type: MsgHelloPortKeyBroadcast, Port: 5000, Signature: testSig},
		pub, priv,
	)
	got := m.Serialize()
	// type ‖ port_le ‖ pubkey ‖ sig ‖ utf-8(signature)
	want := []byte{0x06, 0x88, 0x13}
	want = append(want, pub...)
	want = append(want, m.Sig...)
	want = append(want, []byte(testSig)...)
	if !bytes.Equal(got, want) {
		t.Fatalf("serialize length=%d want=%d", len(got), len(want))
	}
	// And the layout should round-trip + verify.
	parsed, err := ParseBuddyMessage(got)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Port != 5000 || parsed.Signature != testSig {
		t.Fatalf("round-trip changed semantic fields: %+v", parsed)
	}
	if !bytes.Equal(parsed.PubKey, pub) {
		t.Fatal("pubkey mangled by round-trip")
	}
	if err := parsed.VerifyKey(); err != nil {
		t.Fatalf("verify after round-trip: %v", err)
	}
}

func TestBuddyMessage_VerifyKey_RejectsTamperedSignature(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	m := SignBuddyMessage(
		BuddyMessage{Type: MsgHelloPortKeyBroadcast, Port: 5000, Signature: testSig},
		pub, priv,
	)
	// Flip a single bit in the signed text — pubkey claims to control "...test-host..."
	// but the wire payload says something else, so verification must fail.
	tampered := m
	tampered.Signature = "Mallory at evil-host (Linux)"
	if err := tampered.VerifyKey(); err == nil {
		t.Fatal("expected verify to fail on tampered signature string")
	}
	// Same when the port lies.
	tampered = m
	tampered.Port = 5001
	if err := tampered.VerifyKey(); err == nil {
		t.Fatal("expected verify to fail on tampered port")
	}
}

func TestBuddyMessage_VerifyKey_RejectsMismatchedKey(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	otherPub, _, _ := ed25519.GenerateKey(rand.Reader)
	m := SignBuddyMessage(
		BuddyMessage{Type: MsgHelloPortKeyBroadcast, Port: 5000, Signature: testSig},
		pub, priv,
	)
	m.PubKey = otherPub
	if err := m.VerifyKey(); err == nil {
		t.Fatal("expected verify to fail when pubkey doesn't match the signing key")
	}
}

func TestParseBuddyMessage_KeyBearingTruncation(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	m := SignBuddyMessage(
		BuddyMessage{Type: MsgHelloPortKeyBroadcast, Port: 5000, Signature: testSig},
		pub, priv,
	)
	wire := m.Serialize()
	// Truncate inside the pubkey field (1 + 2 + 16 = 19 bytes — half a pubkey).
	if _, err := ParseBuddyMessage(wire[:19]); !errors.Is(err, ErrInvalidMessage) {
		t.Fatalf("expected ErrInvalidMessage on truncated key, got %v", err)
	}
	// Truncate inside the sig field (1 + 2 + 32 + 30 = 65 bytes).
	if _, err := ParseBuddyMessage(wire[:65]); !errors.Is(err, ErrInvalidMessage) {
		t.Fatalf("expected ErrInvalidMessage on truncated sig, got %v", err)
	}
}

func TestBuddyMessage_SignedPayload(t *testing.T) {
	m := BuddyMessage{Type: MsgHelloPortKeyBroadcast, Port: 5000, Signature: testSig}
	got := m.SignedPayload()
	want := []byte{0x88, 0x13}
	want = append(want, []byte(testSig)...)
	if !bytes.Equal(got, want) {
		t.Fatalf("signed payload\n got  % x\n want % x", got, want)
	}
	// Sanity: verify the LE encoder matches binary.LittleEndian directly.
	port := binary.LittleEndian.Uint16(got[:2])
	if port != 5000 {
		t.Fatalf("port LE decode = %d", port)
	}
}

func TestParseBuddyMessage_RejectsInvalid(t *testing.T) {
	cases := []struct {
		name string
		data []byte
	}{
		{"empty", nil},
		{"unknown type", []byte{0x09, 'x'}},
		{"zero type", []byte{0x00, 'x'}},
		{"hello without signature", []byte{0x01}},
		{"hello_unicast without signature", []byte{0x02}},
		{"hello_port without port", []byte{0x04}},
		{"hello_port truncated port", []byte{0x04, 0x88}},
		{"hello_port zero port", append([]byte{0x04, 0x00, 0x00}, []byte(testSig)...)},
		{"hello_port_unicast zero port", append([]byte{0x05, 0x00, 0x00}, []byte(testSig)...)},
		{"hello_port missing signature", []byte{0x04, 0x88, 0x13}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := ParseBuddyMessage(c.data); !errors.Is(err, ErrInvalidMessage) {
				t.Fatalf("expected ErrInvalidMessage, got %v", err)
			}
		})
	}
}

func TestParseBuddyMessage_GoodbyeIgnoresTrailingBytes(t *testing.T) {
	// The Qt parser doesn't inspect bytes after the type byte for GOODBYE.
	// We match: accept but drop.
	got, err := ParseBuddyMessage([]byte{0x03, 'j', 'u', 'n', 'k'})
	if err != nil {
		t.Fatal(err)
	}
	if got.Type != MsgGoodbye || got.Signature != "" || got.Port != 0 {
		t.Fatalf("got %+v", got)
	}
}

func TestBuddyMessage_Validate(t *testing.T) {
	cases := []struct {
		name string
		msg  BuddyMessage
		ok   bool
	}{
		{"valid hello", BuddyMessage{Type: MsgHelloBroadcast, Signature: "x"}, true},
		{"valid goodbye", BuddyMessage{Type: MsgGoodbye}, true},
		{"valid hello_port", BuddyMessage{Type: MsgHelloPortBroadcast, Port: 1, Signature: "x"}, true},
		{"unknown type", BuddyMessage{Type: 0x99, Signature: "x"}, false},
		{"hello no sig", BuddyMessage{Type: MsgHelloBroadcast}, false},
		{"hello_port zero port", BuddyMessage{Type: MsgHelloPortBroadcast, Port: 0, Signature: "x"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.msg.Validate()
			if (err == nil) != c.ok {
				t.Fatalf("Validate err=%v ok=%v", err, c.ok)
			}
		})
	}
}

func TestHelloTypeSelectors(t *testing.T) {
	if HelloBroadcastType(DefaultPort) != MsgHelloBroadcast {
		t.Error("broadcast on default port should be 0x01")
	}
	if HelloBroadcastType(5000) != MsgHelloPortBroadcast {
		t.Error("broadcast on non-default port should be 0x04")
	}
	if HelloUnicastType(DefaultPort) != MsgHelloUnicast {
		t.Error("unicast on default port should be 0x02")
	}
	if HelloUnicastType(5000) != MsgHelloPortUnicast {
		t.Error("unicast on non-default port should be 0x05")
	}
}

func TestBuildSignature_MatchesQtFormat(t *testing.T) {
	// Exact format the Qt Messenger::getSystemSignature produces.
	got := BuildSignature("Alice", "laptop", PlatformWindows)
	want := "Alice at laptop (Windows)"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
