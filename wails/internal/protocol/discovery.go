package protocol

import (
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
)

// BuddyMessage is a decoded UDP discovery datagram.
//
// Port is meaningful only for MsgHelloPortBroadcast / MsgHelloPortUnicast; it
// is ignored for other types on Serialize and always zero after Parse for them.
// Signature is meaningful for every type except MsgGoodbye.
type BuddyMessage struct {
	Type      MessageType
	Port      uint16
	Signature string
}

// ErrInvalidMessage is returned by ParseBuddyMessage for any malformed datagram.
var ErrInvalidMessage = errors.New("dukto: invalid UDP discovery datagram")

// hasPort reports whether t carries a 2-byte port field.
func (t MessageType) hasPort() bool {
	return t == MsgHelloPortBroadcast || t == MsgHelloPortUnicast
}

// hasSignature reports whether t carries a signature payload.
func (t MessageType) hasSignature() bool {
	return t != MsgGoodbye
}

// Valid reports whether t is one of the five defined types.
func (t MessageType) Valid() bool {
	return t >= MsgHelloBroadcast && t <= MsgHelloPortUnicast
}

// Serialize encodes m to its on-the-wire bytes. Integers are little-endian, in
// line with Qt's native x86/ARM memory layout. Serialize does not validate m;
// use Validate to check input before sending.
func (m BuddyMessage) Serialize() []byte {
	n := 1
	if m.Type.hasPort() {
		n += 2
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
	if m.Type.hasSignature() {
		buf = append(buf, sig...)
	}
	return buf
}

// Validate reports whether m is safe to Serialize and would be accepted by a
// conformant receiver. Port==0 is rejected for MsgHelloPort* types (matches Qt
// BuddyMessage::parse). Empty signature is rejected for every non-goodbye type.
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
	var sig string
	if t.hasSignature() {
		if len(payload) == 0 {
			return BuddyMessage{}, fmt.Errorf("%w: missing signature in type 0x%02x", ErrInvalidMessage, byte(t))
		}
		sig = string(payload)
	}
	return BuddyMessage{Type: t, Port: port, Signature: sig}, nil
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
