package main

import (
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net"
	"sort"
	"strings"
	"time"

	"dukto/internal/audit"
	"dukto/internal/identity"
	"dukto/internal/settings"
	"dukto/internal/tunnel"
)

// upgradeServerConn is wired into transfer.Server. It peeks the first 8
// bytes of every accepted connection and routes the session through the
// Noise XX responder when they match the v2 magic; otherwise it returns
// the peeked-but-replayed conn so the legacy SessionHeader parser sees an
// unchanged byte stream.
func (a *App) upgradeServerConn(conn net.Conn) (net.Conn, bool, error) {
	if a.identity.Public == nil {
		// No long-term identity loaded — we can't do v2 anyway, so always
		// fall through to legacy.
		return conn, false, nil
	}
	isV2, peeked, err := tunnel.PeekMagic(conn)
	if err != nil {
		return nil, false, fmt.Errorf("v2 peek: %w", err)
	}
	if !isV2 {
		// Returning the PeekedConn (which replays the 8 bytes on first
		// reads) means the legacy parser sees an unmodified stream.
		return &peeked, false, nil
	}

	priv, pub := a.identity.X25519Private(), mustX25519Pub(a.identity)
	sess, err := tunnel.Handshake(&peeked, tunnel.RoleResponder, priv, pub, nil)
	if err != nil {
		return nil, false, fmt.Errorf("v2 handshake: %w", err)
	}
	// Trust gate: session is encrypted but the remote_static must match a
	// pinned peer. The Allow callback already audits unknown / blocked
	// peers; this layer adds the second gate "v2 session from a peer we
	// haven't pinned yet". We let the session through (pinning can happen
	// from the UI after this exchange) and emit an audit entry capturing
	// who connected so the user can decide.
	remote := sess.RemoteStatic()
	a.recordEncryptedHandshake(conn.RemoteAddr(), remote)
	return sess, true, nil
}

// senderUpgrade is the Sender.Upgrade hook used by bindings_files when
// dialling a peer that the user has marked as pinned. Returns the raw
// conn unchanged when the peer isn't pinned (cleartext fallback).
func (a *App) senderUpgrade(expectedFingerprint string) func(net.Conn) (net.Conn, error) {
	return func(conn net.Conn) (net.Conn, error) {
		if expectedFingerprint == "" || a.identity.Public == nil {
			return conn, nil
		}
		expected, err := a.lookupPinnedX25519(expectedFingerprint)
		if err != nil {
			// Fall back to cleartext rather than refuse the send — the
			// pinning record is local UX, not a hard policy gate yet.
			log.Printf("dukto: send v2 lookup %s: %v", expectedFingerprint, err)
			return conn, nil
		}
		priv, pub := a.identity.X25519Private(), mustX25519Pub(a.identity)
		sess, err := tunnel.Handshake(conn, tunnel.RoleInitiator, priv, pub, nil)
		if err != nil {
			return nil, fmt.Errorf("noise handshake: %w", err)
		}
		// Verify the remote_static matches the pinned X25519 derived from
		// the Ed25519 fingerprint. If not, kill the session — this is the
		// primary defence against a peer at the same IP swapping identity.
		got := sess.RemoteStatic()
		if !bytesEqual(got, expected[:]) {
			_ = sess.Close()
			return nil, fmt.Errorf("v2 fingerprint mismatch: pinned=%s", expectedFingerprint)
		}
		return sess, nil
	}
}

// recordSessionMode is the Server.OnSessionMode hook. It stashes the
// encrypted/cleartext flag so the receive-event handler can stamp the
// audit/history entry correctly.
func (a *App) recordSessionMode(encrypted bool) {
	a.modeMu.Lock()
	a.lastSessionEncrypted = encrypted
	a.modeMu.Unlock()
}

// sessionEncrypted returns the latched encrypted flag for the session
// currently being handled.
func (a *App) sessionEncrypted() bool {
	a.modeMu.Lock()
	defer a.modeMu.Unlock()
	return a.lastSessionEncrypted
}

// recordEncryptedHandshake writes an audit entry capturing the remote_static
// of an inbound v2 handshake. Used by the UI to surface "peer X with
// fingerprint Y just connected" so the user can pin them after seeing
// the fingerprint match.
func (a *App) recordEncryptedHandshake(remote net.Addr, remoteX25519 []byte) {
	if a.audit == nil {
		return
	}
	a.audit.Append(audit.Entry{
		Time:   time.Now(),
		Kind:   "session_encrypted",
		Peer:   remote.String(),
		Reason: hex.EncodeToString(remoteX25519),
	})
}

// PinPeer pins the peer identified by fingerprint as a trusted v2 endpoint.
// The peer's pubkey must already be known (from a prior 0x06/0x07 HELLO);
// callers pass the address discovered for that peer so we can look the
// pubkey up. Returns the persisted PinnedPeer record or an error.
func (a *App) PinPeer(fingerprint, address string) (*settings.PinnedPeer, error) {
	if fingerprint == "" || address == "" {
		return nil, errors.New("PinPeer: fingerprint and address are required")
	}
	pub, err := a.findPubKeyForAddress(address)
	if err != nil {
		return nil, err
	}
	gotFP := identity.Fingerprint(pub)
	if !strings.EqualFold(gotFP, fingerprint) {
		return nil, fmt.Errorf("fingerprint mismatch: expected %s, peer at %s advertises %s", fingerprint, address, gotFP)
	}
	label := a.labelForAddress(address)
	rec := settings.PinnedPeer{
		Fingerprint:   gotFP,
		Ed25519PubHex: hex.EncodeToString(pub),
		Label:         label,
		PinnedAt:      time.Now(),
	}
	if err := a.settings.Update(func(v *settings.Values) {
		if v.PinnedPeers == nil {
			v.PinnedPeers = map[string]settings.PinnedPeer{}
		}
		v.PinnedPeers[gotFP] = rec
	}); err != nil {
		return nil, err
	}
	if a.audit != nil {
		a.audit.Append(audit.Entry{
			Time:   time.Now(),
			Kind:   "peer_pinned",
			Peer:   label,
			Reason: gotFP,
		})
	}
	return &rec, nil
}

// UnpinPeer removes the pinning for fingerprint. Subsequent sessions with
// that peer fall back to cleartext (the peer's encryption capability
// advertisement still shows in the UI as 🔓 unpaired).
func (a *App) UnpinPeer(fingerprint string) error {
	if fingerprint == "" {
		return errors.New("UnpinPeer: fingerprint required")
	}
	return a.settings.Update(func(v *settings.Values) {
		delete(v.PinnedPeers, fingerprint)
	})
}

// PinnedPeers returns the TOFU table sorted by PinnedAt descending so the
// settings list shows the most recent pairings first.
func (a *App) PinnedPeers() []settings.PinnedPeer {
	pinned := a.settings.Values().PinnedPeers
	out := make([]settings.PinnedPeer, 0, len(pinned))
	for _, p := range pinned {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PinnedAt.After(out[j].PinnedAt) })
	return out
}

// IsPeerPinned is the small helper the bindings layer uses to decide
// whether to install the v2 Upgrade hook on an outbound Sender.
func (a *App) IsPeerPinned(fingerprint string) bool {
	if fingerprint == "" {
		return false
	}
	_, ok := a.settings.Values().PinnedPeers[fingerprint]
	return ok
}

// findPubKeyForAddress resolves the most recently advertised Ed25519
// pubkey for a peer at the given "ip:port" or "ip" string, drawing from
// the discovery messenger's verified-peer table.
func (a *App) findPubKeyForAddress(address string) (ed25519.PublicKey, error) {
	if a.messenger == nil {
		return nil, errors.New("messenger not started")
	}
	addr := address
	if i := strings.Index(addr, ":"); i >= 0 {
		addr = addr[:i]
	}
	for _, p := range a.messenger.Peers() {
		if p.Addr.String() == addr {
			if len(p.PubKey) == ed25519.PublicKeySize {
				return ed25519.PublicKey(append([]byte(nil), p.PubKey...)), nil
			}
		}
	}
	return nil, fmt.Errorf("no v2 pubkey advertised for %s yet — wait for a HELLO", addr)
}

// fingerprintForAddress returns the Ed25519 fingerprint of the v2-capable
// peer at address (an "ip" or "ip:port"), or "" if no v2 HELLO has been
// received from that peer yet. Used by the sender path to decide whether
// to opt into the encrypted upgrade.
func (a *App) fingerprintForAddress(address string) string {
	pub, err := a.findPubKeyForAddress(address)
	if err != nil {
		return ""
	}
	return identity.Fingerprint(pub)
}

// labelForAddress returns the buddy-name or signature for address, used
// as the persisted Label on the PinnedPeer entry.
func (a *App) labelForAddress(address string) string {
	if a.messenger == nil {
		return address
	}
	addr := address
	if i := strings.Index(addr, ":"); i >= 0 {
		addr = addr[:i]
	}
	for _, p := range a.messenger.Peers() {
		if p.Addr.String() == addr {
			if p.Signature != "" {
				return p.Signature
			}
		}
	}
	return address
}

// lookupPinnedX25519 reads the X25519 pubkey for fingerprint from the
// pinned table by converting its stored Ed25519 pubkey via the Edwards-
// to-Montgomery transform.
func (a *App) lookupPinnedX25519(fingerprint string) ([32]byte, error) {
	pinned := a.settings.Values().PinnedPeers
	rec, ok := pinned[fingerprint]
	if !ok {
		return [32]byte{}, fmt.Errorf("not pinned: %s", fingerprint)
	}
	pubBytes, err := hex.DecodeString(rec.Ed25519PubHex)
	if err != nil {
		return [32]byte{}, fmt.Errorf("decode pinned pubkey: %w", err)
	}
	return identity.Ed25519PubToX25519Pub(ed25519.PublicKey(pubBytes))
}

// mustX25519Pub returns the X25519 pubkey for an Identity, ignoring the
// error (it can only fail if curve25519 itself fails — a logic bug).
func mustX25519Pub(id identity.Identity) [32]byte {
	pub, err := id.X25519Public()
	if err != nil {
		log.Printf("dukto: x25519 derivation: %v", err)
	}
	return pub
}

// bytesEqual is a constant-time-ish helper. The actual constant-time
// comparison happens in noise's authenticated-encryption layer; this one
// is just for the post-handshake fingerprint check, where timing leaks
// would only reveal "is the peer pinned" to a peer that's already on
// the LAN.
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
