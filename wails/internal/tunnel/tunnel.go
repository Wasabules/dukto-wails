// Package tunnel implements the v2 encrypted overlay on top of a TCP
// connection: an 8-byte magic prefix announces the upgrade, then a Noise
// XX handshake authenticates both peers and produces a transport that
// frames the legacy SessionHeader/ElementHeader stream inside
// ChaCha20-Poly1305 messages.
//
// See docs/SECURITY_v2.md for the full design. The wire format below is
// the contract between Wails and android-native — keep them aligned.
package tunnel

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"

	"github.com/flynn/noise"
)

// Magic is the 8-byte prefix a v2 sender writes before any handshake bytes.
// "DKTOv2" + two zero pad bytes — fixed forever, anchors the protocol
// upgrade detection at the receiver. Legacy senders never write these
// bytes, so a peek of the first 8 bytes is sufficient to route between
// the legacy SessionHeader parser and the Noise handshake.
var Magic = [8]byte{'D', 'K', 'T', 'O', 'v', '2', 0x00, 0x00}

// MagicLen is the length of the v2 magic prefix in bytes.
const MagicLen = 8

// maxFrame is the largest plaintext we encrypt in a single Noise transport
// message. Noise itself caps each message at 65535 bytes; we leave room for
// the AEAD tag (16 B) and the 2-byte length prefix.
const maxFrame = 65535 - 16

// Cipher suite for Dukto v2: X25519 DH, ChaCha20-Poly1305, SHA-256. Same
// suite as WireGuard. Stable forever — bumping requires a new magic value.
var cipherSuite = noise.NewCipherSuite(noise.DH25519, noise.CipherChaChaPoly, noise.HashSHA256)

// PeekMagic reads exactly 8 bytes from conn and reports whether they match
// [Magic]. The returned [PeekedConn] re-emits the peeked bytes on its first
// reads when isV2 is false, so a legacy fallback can parse the
// SessionHeader as if no peek had happened.
func PeekMagic(conn net.Conn) (isV2 bool, peeked PeekedConn, err error) {
	buf := make([]byte, MagicLen)
	if _, rerr := io.ReadFull(conn, buf); rerr != nil {
		return false, PeekedConn{}, fmt.Errorf("tunnel: read magic: %w", rerr)
	}
	pc := PeekedConn{Conn: conn}
	isV2 = string(buf) == string(Magic[:])
	if !isV2 {
		pc.unread = buf
	}
	return isV2, pc, nil
}

// PeekedConn is a [net.Conn] whose first reads replay bytes consumed by
// [PeekMagic] before falling through to the underlying connection. It
// transparently behaves like the original conn for everything else
// (Write, Close, deadlines, etc.).
type PeekedConn struct {
	net.Conn
	unread []byte
}

// Read drains the unread buffer first, then reads from the underlying
// connection. Writes are unaffected.
func (p *PeekedConn) Read(b []byte) (int, error) {
	if len(p.unread) > 0 {
		n := copy(b, p.unread)
		p.unread = p.unread[n:]
		return n, nil
	}
	return p.Conn.Read(b)
}

// HandshakeRole picks initiator vs. responder for [Handshake].
type HandshakeRole int

const (
	// RoleInitiator drives the Noise XX handshake (sender).
	RoleInitiator HandshakeRole = iota
	// RoleResponder follows the Noise XX handshake (receiver).
	RoleResponder
)

// Handshake performs a Noise XX handshake on conn and returns a [Session]
// ready to carry application bytes. The initiator writes Magic onto the
// wire automatically; the responder must already have consumed it via
// [PeekMagic] before calling Handshake.
//
// staticPriv / staticPub is this side's X25519 long-term keypair (derive
// it from the Ed25519 identity via identity.Identity.X25519Private).
//
// If psk is non-nil it must be exactly 32 bytes; the handshake then runs
// Noise XXpsk2 (PSK mixed before the third message). Used only for
// first-contact pairing — established peers handshake with psk == nil.
//
// On success the caller can read the remote X25519 pubkey via
// [Session.RemoteStatic]; comparing it against a pinned identity is the
// caller's responsibility.
func Handshake(conn net.Conn, role HandshakeRole, staticPriv, staticPub [32]byte, psk []byte) (*Session, error) {
	if psk != nil && len(psk) != 32 {
		return nil, fmt.Errorf("tunnel: psk length %d, expected 32 (or nil)", len(psk))
	}
	cfg := noise.Config{
		CipherSuite:   cipherSuite,
		Random:        nil, // crypto/rand
		Pattern:       noise.HandshakeXX,
		Initiator:     role == RoleInitiator,
		Prologue:      Magic[:],
		StaticKeypair: noise.DHKey{Private: staticPriv[:], Public: staticPub[:]},
	}
	if psk != nil {
		// XXpsk2 — PSK mixed before the third handshake message. Setting
		// PresharedKeyPlacement when psk == nil tells noise to treat this
		// as a PSK pattern, which then errors out on the second message.
		cfg.PresharedKey = psk
		cfg.PresharedKeyPlacement = 2
	}
	hs, err := noise.NewHandshakeState(cfg)
	if err != nil {
		return nil, fmt.Errorf("tunnel: handshake init: %w", err)
	}

	if role == RoleInitiator {
		if _, err := conn.Write(Magic[:]); err != nil {
			return nil, fmt.Errorf("tunnel: write magic: %w", err)
		}
	}

	var sendCS, recvCS *noise.CipherState
	// XX has 3 messages: ←e, →e,ee,s,es, ←s,se. As initiator we write 1, read
	// 2, write 3. As responder we read 1, write 2, read 3.
	steps := []struct{ writing bool }{
		{role == RoleInitiator},
		{role != RoleInitiator},
		{role == RoleInitiator},
	}
	// Per the noise library's noise_test.go conventions, the initiator
	// encrypts outgoing with cs0 and decrypts incoming with cs1; the
	// responder is the mirror: send with cs1, recv with cs0.
	mapToSendRecv := func(cs0, cs1 *noise.CipherState) {
		if role == RoleInitiator {
			sendCS, recvCS = cs0, cs1
		} else {
			sendCS, recvCS = cs1, cs0
		}
	}
	for i, st := range steps {
		if st.writing {
			out, cs0, cs1, werr := hs.WriteMessage(nil, nil)
			if werr != nil {
				return nil, fmt.Errorf("tunnel: handshake write %d: %w", i, werr)
			}
			if err := writeFrame(conn, out); err != nil {
				return nil, err
			}
			if cs0 != nil {
				mapToSendRecv(cs0, cs1)
			}
		} else {
			in, err := readFrame(conn)
			if err != nil {
				return nil, err
			}
			_, cs0, cs1, rerr := hs.ReadMessage(nil, in)
			if rerr != nil {
				return nil, fmt.Errorf("tunnel: handshake read %d: %w", i, rerr)
			}
			if cs0 != nil {
				mapToSendRecv(cs0, cs1)
			}
		}
	}
	if sendCS == nil || recvCS == nil {
		return nil, errors.New("tunnel: handshake did not yield transport keys")
	}
	remoteStatic := append([]byte(nil), hs.PeerStatic()...)
	return &Session{
		conn:         conn,
		send:         sendCS,
		recv:         recvCS,
		remoteStatic: remoteStatic,
	}, nil
}

// Session is the encrypted transport produced by a successful Handshake.
// Read and Write transparently encrypt/decrypt against ChaCha20-Poly1305
// using the keys derived from the handshake.
type Session struct {
	conn net.Conn
	send *noise.CipherState
	recv *noise.CipherState

	remoteStatic []byte

	// rxBuf holds plaintext left over from the previous frame when the
	// caller's Read() buffer was smaller than a single frame.
	rxBuf []byte
}

// RemoteStatic returns the peer's long-term X25519 public key, as seen by
// the handshake. Callers must verify it against a pinned identity before
// trusting the session — TOFU pinning is enforced by the caller, not the
// tunnel package.
func (s *Session) RemoteStatic() []byte {
	out := make([]byte, len(s.remoteStatic))
	copy(out, s.remoteStatic)
	return out
}

// Write encrypts p and frames it on the wire as `length_le_u16 || ciphertext`.
// p is split into multiple Noise transport messages if it exceeds [maxFrame].
func (s *Session) Write(p []byte) (int, error) {
	written := 0
	for len(p) > 0 {
		chunk := p
		if len(chunk) > maxFrame {
			chunk = chunk[:maxFrame]
		}
		ct, err := s.send.Encrypt(nil, nil, chunk)
		if err != nil {
			return written, fmt.Errorf("tunnel: encrypt: %w", err)
		}
		if err := writeFrame(s.conn, ct); err != nil {
			return written, err
		}
		written += len(chunk)
		p = p[len(chunk):]
	}
	return written, nil
}

// Read decrypts the next frame and copies up to len(p) bytes of its
// plaintext into p. If the plaintext is larger than p, the leftover is
// retained for the next call.
func (s *Session) Read(p []byte) (int, error) {
	if len(s.rxBuf) > 0 {
		n := copy(p, s.rxBuf)
		s.rxBuf = s.rxBuf[n:]
		return n, nil
	}
	frame, err := readFrame(s.conn)
	if err != nil {
		return 0, err
	}
	pt, err := s.recv.Decrypt(nil, nil, frame)
	if err != nil {
		return 0, fmt.Errorf("tunnel: decrypt: %w", err)
	}
	n := copy(p, pt)
	if n < len(pt) {
		s.rxBuf = pt[n:]
	}
	return n, nil
}

// Close shuts down the underlying connection.
func (s *Session) Close() error { return s.conn.Close() }

// writeFrame writes a 2-byte LE length prefix followed by data.
func writeFrame(conn net.Conn, data []byte) error {
	if len(data) > 65535 {
		return fmt.Errorf("tunnel: frame %d > 65535", len(data))
	}
	var hdr [2]byte
	binary.LittleEndian.PutUint16(hdr[:], uint16(len(data)))
	if _, err := conn.Write(hdr[:]); err != nil {
		return fmt.Errorf("tunnel: write frame header: %w", err)
	}
	if _, err := conn.Write(data); err != nil {
		return fmt.Errorf("tunnel: write frame body: %w", err)
	}
	return nil
}

// readFrame reads a 2-byte LE length prefix and returns the following
// `length` bytes.
func readFrame(conn net.Conn) ([]byte, error) {
	var hdr [2]byte
	if _, err := io.ReadFull(conn, hdr[:]); err != nil {
		return nil, fmt.Errorf("tunnel: read frame header: %w", err)
	}
	n := binary.LittleEndian.Uint16(hdr[:])
	buf := make([]byte, n)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return nil, fmt.Errorf("tunnel: read frame body: %w", err)
	}
	return buf, nil
}
