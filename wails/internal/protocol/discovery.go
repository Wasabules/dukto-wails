package protocol

import (
	"crypto/ed25519"
	"encoding/binary"
	"errors"
	"fmt"
)

// MessageType is the single-byte tag at the start of a UDP discovery datagram.
type MessageType uint8

const (
	// MsgHelloBroadcast — sender is bound to the default port and broadcasts
	// its presence. No port field; signature required.
	MsgHelloBroadcast MessageType = 0x01
	// MsgHelloUnicast — unicast reply to a broadcast HELLO when the sender is
	// bound to the default port. No port field; signature required.
	MsgHelloUnicast MessageType = 0x02
	// MsgGoodbye — broadcast on shutdown. No port, no signature.
	MsgGoodbye MessageType = 0x03
	// MsgHelloPortBroadcast — sender uses a non-default port; port field
	// carries the listening port. Signature required.
	MsgHelloPortBroadcast MessageType = 0x04
	// MsgHelloPortUnicast — unicast reply when the sender uses a non-default
	// port. Signature required.
	MsgHelloPortUnicast MessageType = 0x05
	// MsgHelloPortKeyBroadcast — v2 broadcast HELLO with embedded Ed25519
	// pubkey + signature over (port_le ‖ utf-8 signature). Carries the same
	// information as MsgHelloPortBroadcast plus the long-term identity.
	// Legacy peers reject any byte > 0x05 and ignore the datagram silently,
	// so a v2 peer broadcasts both 0x04 and 0x06 every HELLO interval.
	MsgHelloPortKeyBroadcast MessageType = 0x06
	// MsgHelloPortKeyUnicast — v2 unicast reply, same payload shape as 0x06.
	MsgHelloPortKeyUnicast MessageType = 0x07
)

// Ed25519PublicKeySize is the on-the-wire pubkey length carried in 0x06/0x07.
const Ed25519PublicKeySize = ed25519.PublicKeySize

// Ed25519SignatureSize is the on-the-wire signature length carried in 0x06/0x07.
const Ed25519SignatureSize = ed25519.SignatureSize

// BuddyMessage is a decoded UDP discovery datagram.
//
// Port is meaningful only for port-carrying types (0x04, 0x05, 0x06, 0x07); it
// is ignored for other types on Serialize and always zero after Parse for them.
// Signature is meaningful for every type except MsgGoodbye.
//
// PubKey and Sig are populated only for v2 types (0x06, 0x07) and are ignored
// for legacy types. PubKey is a raw Ed25519 public key (32 B). Sig is an
// Ed25519 signature over `port_le_bytes ‖ utf-8(Signature)`.
type BuddyMessage struct {
	Type      MessageType
	Port      uint16
	Signature string
	PubKey    []byte
	Sig       []byte
}

// ErrInvalidMessage is returned by ParseBuddyMessage for any malformed datagram.
var ErrInvalidMessage = errors.New("dukto: invalid UDP discovery datagram")

// hasPort reports whether t carries a 2-byte port field.
func (t MessageType) hasPort() bool {
	return t == MsgHelloPortBroadcast || t == MsgHelloPortUnicast ||
		t == MsgHelloPortKeyBroadcast || t == MsgHelloPortKeyUnicast
}

// hasSignature reports whether t carries a signature payload.
func (t MessageType) hasSignature() bool {
	return t != MsgGoodbye
}

// hasKey reports whether t carries a pubkey + Ed25519 signature.
func (t MessageType) hasKey() bool {
	return t == MsgHelloPortKeyBroadcast || t == MsgHelloPortKeyUnicast
}

// Valid reports whether t is one of the seven defined types.
func (t MessageType) Valid() bool {
	return t >= MsgHelloBroadcast && t <= MsgHelloPortKeyUnicast
}

// Serialize encodes m to its on-the-wire bytes. Integers are little-endian, in
// line with Qt's native x86/ARM memory layout. Serialize does not validate m;
// use Validate to check input before sending.
//
// Wire layout for the v2 key-bearing types (0x06, 0x07):
//
//	type (1B) ‖ port (LE u16, 2B) ‖ pub_key (32B) ‖ sig (64B) ‖ utf-8 signature
func (m BuddyMessage) Serialize() []byte {
	n := 1
	if m.Type.hasPort() {
		n += 2
	}
	if m.Type.hasKey() {
		n += Ed25519PublicKeySize + Ed25519SignatureSize
	}
	sig := []byte(m.Signature)
	if m.Type.hasSignature() {
		n += len(sig)
	}
	buf := make([]byte, 0, n)
	buf = append(buf, byte(m.Type))
	if m.Type.hasPort() {
		buf = binary.LittleEndian.AppendUint16(buf, m.Port)
	}
	if m.Type.hasKey() {
		buf = append(buf, m.PubKey...)
		buf = append(buf, m.Sig...)
	}
	if m.Type.hasSignature() {
		buf = append(buf, sig...)
	}
	return buf
}

// Validate reports whether m is safe to Serialize and would be accepted by a
// conformant receiver. Port==0 is rejected for port-carrying types. Empty
// signature is rejected for every non-goodbye type. Key-bearing types must
// carry a 32-byte pubkey and a 64-byte Ed25519 signature.
func (m BuddyMessage) Validate() error {
	if !m.Type.Valid() {
		return fmt.Errorf("%w: unknown type 0x%02x", ErrInvalidMessage, byte(m.Type))
	}
	if m.Type.hasPort() && m.Port == 0 {
		return fmt.Errorf("%w: port-carrying type 0x%02x with zero port", ErrInvalidMessage, byte(m.Type))
	}
	if m.Type.hasSignature() && m.Signature == "" {
		return fmt.Errorf("%w: type 0x%02x requires a signature", ErrInvalidMessage, byte(m.Type))
	}
	if m.Type.hasKey() {
		if len(m.PubKey) != Ed25519PublicKeySize {
			return fmt.Errorf("%w: type 0x%02x pubkey must be %d bytes", ErrInvalidMessage, byte(m.Type), Ed25519PublicKeySize)
		}
		if len(m.Sig) != Ed25519SignatureSize {
			return fmt.Errorf("%w: type 0x%02x sig must be %d bytes", ErrInvalidMessage, byte(m.Type), Ed25519SignatureSize)
		}
	}
	return nil
}

// ParseBuddyMessage decodes a UDP datagram. It returns ErrInvalidMessage for:
// empty input, unknown type byte, port-carrying types with zero port, and
// non-goodbye types with empty signature. These rejection rules match the Qt
// reference implementation.
func ParseBuddyMessage(data []byte) (BuddyMessage, error) {
	if len(data) < 1 {
		return BuddyMessage{}, fmt.Errorf("%w: empty datagram", ErrInvalidMessage)
	}
	t := MessageType(data[0])
	if !t.Valid() {
		return BuddyMessage{}, fmt.Errorf("%w: unknown type 0x%02x", ErrInvalidMessage, byte(t))
	}
	payload := data[1:]
	var port uint16
	if t.hasPort() {
		if len(payload) < 2 {
			return BuddyMessage{}, fmt.Errorf("%w: port-carrying type truncated", ErrInvalidMessage)
		}
		port = binary.LittleEndian.Uint16(payload[:2])
		payload = payload[2:]
		if port == 0 {
			return BuddyMessage{}, fmt.Errorf("%w: zero port in type 0x%02x", ErrInvalidMessage, byte(t))
		}
	}
	var pub, sigBytes []byte
	if t.hasKey() {
		need := Ed25519PublicKeySize + Ed25519SignatureSize
		if len(payload) < need {
			return BuddyMessage{}, fmt.Errorf("%w: type 0x%02x truncated key/sig", ErrInvalidMessage, byte(t))
		}
		pub = append([]byte(nil), payload[:Ed25519PublicKeySize]...)
		sigBytes = append([]byte(nil), payload[Ed25519PublicKeySize:Ed25519PublicKeySize+Ed25519SignatureSize]...)
		payload = payload[need:]
	}
	var sig string
	if t.hasSignature() {
		if len(payload) == 0 {
			return BuddyMessage{}, fmt.Errorf("%w: missing signature in type 0x%02x", ErrInvalidMessage, byte(t))
		}
		sig = string(payload)
	}
	return BuddyMessage{Type: t, Port: port, Signature: sig, PubKey: pub, Sig: sigBytes}, nil
}

// SignedPayload returns the byte string that the Ed25519 signature in a v2
// HELLO covers: little-endian port followed by the utf-8 signature string.
// Returned even for non-key types (callers usually only invoke it for 0x06/0x07).
func (m BuddyMessage) SignedPayload() []byte {
	out := make([]byte, 0, 2+len(m.Signature))
	out = binary.LittleEndian.AppendUint16(out, m.Port)
	out = append(out, []byte(m.Signature)...)
	return out
}

// VerifyKey checks the embedded Ed25519 signature against the embedded pubkey
// over SignedPayload(). Returns an ErrInvalidMessage-wrapped error on
// mismatch. Callers must only invoke VerifyKey on key-bearing types.
func (m BuddyMessage) VerifyKey() error {
	if !m.Type.hasKey() {
		return fmt.Errorf("%w: type 0x%02x carries no key", ErrInvalidMessage, byte(m.Type))
	}
	if len(m.PubKey) != Ed25519PublicKeySize || len(m.Sig) != Ed25519SignatureSize {
		return fmt.Errorf("%w: malformed key/sig fields", ErrInvalidMessage)
	}
	if !ed25519.Verify(ed25519.PublicKey(m.PubKey), m.SignedPayload(), m.Sig) {
		return fmt.Errorf("%w: bad ed25519 signature", ErrInvalidMessage)
	}
	return nil
}

// Goodbye returns a ready-to-send MsgGoodbye datagram.
func Goodbye() BuddyMessage {
	return BuddyMessage{Type: MsgGoodbye}
}

// HelloBroadcastType returns the broadcast HELLO type appropriate for a peer
// listening on localPort. Matches the Qt selection rule: default port → 0x01,
// non-default port → 0x04 (which carries the port).
func HelloBroadcastType(localPort uint16) MessageType {
	if localPort == DefaultPort {
		return MsgHelloBroadcast
	}
	return MsgHelloPortBroadcast
}

// HelloUnicastType returns the unicast-reply HELLO type for a peer listening
// on localPort. Mirrors HelloBroadcastType.
func HelloUnicastType(localPort uint16) MessageType {
	if localPort == DefaultPort {
		return MsgHelloUnicast
	}
	return MsgHelloPortUnicast
}

// SignBuddyMessage stamps a v2 HELLO with the caller's identity. It populates
// PubKey and computes Sig as Ed25519(priv, port_le ‖ utf-8(Signature)). The
// returned message can be Serialize()d directly. Caller must ensure m.Type is
// a key-bearing type before invoking.
func SignBuddyMessage(m BuddyMessage, pub ed25519.PublicKey, priv ed25519.PrivateKey) BuddyMessage {
	m.PubKey = append([]byte(nil), pub...)
	m.Sig = ed25519.Sign(priv, m.SignedPayload())
	return m
}
