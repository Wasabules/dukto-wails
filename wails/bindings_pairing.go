package main

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net"
	"net/netip"
	"sort"
	"strings"
	"time"

	"github.com/skip2/go-qrcode"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"dukto/internal/audit"
	"dukto/internal/discovery"
	"dukto/internal/eff"
	"dukto/internal/identity"
	"dukto/internal/protocol"
	"dukto/internal/settings"
	"dukto/internal/tunnel"
)

// upgradeServerConn is wired into transfer.Server. It peeks the first 8
// bytes of every accepted connection and routes the session through the
// Noise XX responder when they match the v2 magic; otherwise it returns
// the peeked-but-replayed conn so the legacy SessionHeader parser sees an
// unchanged byte stream.
//
// When a pairing session is in flight (StartPairing called within the
// last 30 seconds with a generated PSK), the handshake runs Noise XXpsk2
// instead of plain XX so a first-contact peer can authenticate us via
// the shared one-shot passphrase.
func (a *App) upgradeServerConn(conn net.Conn) (net.Conn, bool, error) {
	refuseCleartext := a.settings.Values().RefuseCleartext
	if a.identity.Public == nil {
		// No long-term identity loaded — we can't do v2 anyway. If
		// the user demands encryption, drop the connection.
		if refuseCleartext {
			return nil, false, errors.New("refuseCleartext: no v2 identity loaded")
		}
		return conn, false, nil
	}
	isV2, peeked, err := tunnel.PeekMagic(conn)
	if err != nil {
		return nil, false, fmt.Errorf("v2 peek: %w", err)
	}
	if !isV2 {
		if refuseCleartext {
			return nil, false, errors.New("refuseCleartext: peer used legacy session header")
		}
		// Returning the PeekedConn (which replays the 8 bytes on first
		// reads) means the legacy parser sees an unmodified stream.
		return &peeked, false, nil
	}

	priv, pub := a.identity.X25519Private(), mustX25519Pub(a.identity)
	psk := a.consumePairingPSK()
	sess, err := tunnel.Handshake(&peeked, tunnel.RoleResponder, priv, pub, psk)
	if err != nil {
		return nil, false, fmt.Errorf("v2 handshake: %w", err)
	}
	// Pairing branch: when the handshake used a PSK, automatically pin
	// the peer's identity. The PSK already proved both sides know the
	// passphrase, so the remote_static is trustworthy at this point.
	remote := sess.RemoteStatic()
	if psk != nil {
		_ = a.autoPinFromX25519(conn.RemoteAddr(), remote)
	} else {
		// TOFU mismatch detector: if we have a pinning record for this
		// peer's IP and the new remote_static doesn't match, kill the
		// session and surface a UI modal so the user can re-pair.
		if mismatch := a.checkTOFUMismatch(conn.RemoteAddr(), remote); mismatch != nil {
			_ = sess.Close()
			a.emitTOFUMismatch(*mismatch)
			return nil, false, fmt.Errorf("v2 fingerprint mismatch: %s", mismatch.NewFingerprint)
		}
		if refuseCleartext {
			// Encrypted-only mode also requires the responder side to
			// know the peer in advance: an unpinned v2 connection means
			// we have no way to authenticate the remote_static beyond
			// "it ran the handshake". Drop unless the peer is paired.
			if !a.isPubKeyPinned(remote) {
				_ = sess.Close()
				return nil, false, errors.New("refuseCleartext: peer not paired")
			}
		}
	}
	a.recordEncryptedHandshake(conn.RemoteAddr(), remote)
	return sess, true, nil
}

// ── pairing flow ─────────────────────────────────────────────────────────

// PinnedAddressTTL is how long a pinned peer's LastSeenAddr stays
// usable for the unicast poke loop. After this delay without a fresh
// HELLO sighting, we stop probing the address — protects against
// leaking our presence to a peer that's now behind a different IP
// (DHCP renewal of a long-disconnected friend, recycled cafe Wi-Fi,
// etc.).
const PinnedAddressTTL = 7 * 24 * time.Hour

// notePinnedPeerSeen refreshes LastSeenAddr on the matching PinnedPeer
// record. No-op when the peer isn't pinned, when the address is
// unchanged AND less than 1h old (no need to churn the JSON store on
// every 10s broadcast), or when a non-paired peer happens to share an
// IP — the lookup is by fingerprint, computed from the verified
// pubkey, so impostors can't poison the cache.
func (a *App) notePinnedPeerSeen(p discovery.Peer) {
	if len(p.PubKey) != ed25519.PublicKeySize {
		return
	}
	fp := identity.Fingerprint(ed25519.PublicKey(p.PubKey))
	current := a.settings.Values().PinnedPeers
	rec, ok := current[fp]
	if !ok {
		return
	}
	addr := fmt.Sprintf("%s:%d", p.Addr.String(), p.Port)
	now := time.Now()
	// Skip the disk write when nothing material changed and the
	// timestamp is still fresh — saves a JSON rewrite on every tick.
	if rec.LastSeenAddr == addr && now.Sub(rec.LastSeenAt) < time.Hour {
		return
	}
	_ = a.settings.Update(func(v *settings.Values) {
		r, ok := v.PinnedPeers[fp]
		if !ok {
			return
		}
		r.LastSeenAddr = addr
		r.LastSeenAt = now
		v.PinnedPeers[fp] = r
	})
}

// isEd25519PubKeyPinned answers the messenger's IsPubKeyPinned hook
// so paired peers' inbound probes get a unicast reply even when our
// own broadcast is stealthed.
func (a *App) isEd25519PubKeyPinned(pub ed25519.PublicKey) bool {
	if len(pub) != ed25519.PublicKeySize {
		return false
	}
	fp := identity.Fingerprint(pub)
	_, ok := a.settings.Values().PinnedPeers[fp]
	return ok
}

// pairingTTL is how long a generated passphrase stays armed on the
// "responder" side. After expiry the PSK is discarded and the handshake
// falls back to plain XX. Long enough to give the user time to read the
// words out and type them in; short enough that a forgotten flow can't
// linger as a permanent low-entropy backdoor.
const pairingTTL = 60 * time.Second

// pendingPairing holds the responder-side PSK during an in-flight
// pairing. Cleared on a successful handshake or after [pairingTTL].
type pendingPairing struct {
	psk     []byte
	expires time.Time
}

// PairingCodeQR returns a PNG QR-code data URL encoding [passphrase].
// The Android camera-pairing flow scans this directly. Caller is the UI
// layer right after StartPairing — the passphrase plaintext is what's
// encoded; the QR is just an alternate visual representation.
func (a *App) PairingCodeQR(passphrase string) (string, error) {
	if passphrase == "" {
		return "", errors.New("PairingCodeQR: empty passphrase")
	}
	png, err := qrcode.Encode(passphrase, qrcode.Medium, 240)
	if err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(png), nil
}

// StartPairing generates a fresh 5-word EFF passphrase, derives the PSK,
// and arms the server for the next [pairingTTL]. The passphrase is
// returned for the user to read out / scan; the PSK never leaves the
// process.
func (a *App) StartPairing() (string, error) {
	pass, err := eff.Generate(5)
	if err != nil {
		return "", err
	}
	psk, err := eff.DerivePSK(pass)
	if err != nil {
		return "", err
	}
	a.modeMu.Lock()
	a.pendingPair = &pendingPairing{psk: psk, expires: time.Now().Add(pairingTTL)}
	a.modeMu.Unlock()
	return pass, nil
}

// CancelPairing clears any in-flight pairing PSK. Safe to call when no
// pairing is active.
func (a *App) CancelPairing() {
	a.modeMu.Lock()
	a.pendingPair = nil
	a.modeMu.Unlock()
}

// consumePairingPSK returns the armed PSK and clears it atomically. Used
// by the Server.Upgrade hook to feed the PSK into one handshake exactly.
func (a *App) consumePairingPSK() []byte {
	a.modeMu.Lock()
	defer a.modeMu.Unlock()
	pp := a.pendingPair
	if pp == nil || time.Now().After(pp.expires) {
		a.pendingPair = nil
		return nil
	}
	a.pendingPair = nil
	return pp.psk
}

// PairWithPassphrase is the initiator-side counterpart of [StartPairing]:
// the user types the 5-word code; this function dials the peer and runs
// Noise XXpsk2 with the same derived PSK. On success both peers have
// pinned each other's long-term key in their TOFU table.
func (a *App) PairWithPassphrase(addrPort, passphrase string) error {
	if a.identity.Public == nil {
		return errors.New("PairWithPassphrase: no v2 identity loaded")
	}
	psk, err := eff.DerivePSK(passphrase)
	if err != nil {
		return err
	}
	peer, err := parseAddrPort(addrPort)
	if err != nil {
		return err
	}
	priv, pub := a.identity.X25519Private(), mustX25519Pub(a.identity)
	dialer := net.Dialer{Timeout: 5 * time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := dialer.DialContext(ctx, "tcp4", peer.String())
	if err != nil {
		return fmt.Errorf("dial %s: %w", peer, err)
	}
	defer conn.Close()
	sess, err := tunnel.Handshake(conn, tunnel.RoleInitiator, priv, pub, psk)
	if err != nil {
		return fmt.Errorf("pair handshake: %w", err)
	}
	defer sess.Close()
	// PSK match → both sides know the passphrase → remote_static is
	// trustworthy. Pin it.
	if err := a.autoPinFromX25519(conn.RemoteAddr(), sess.RemoteStatic()); err != nil {
		return fmt.Errorf("pair pin: %w", err)
	}
	// Send a tiny "pair complete" sentinel so the responder's Receiver
	// gets a clean session start/end and the audit log records the pair.
	hdr := protocol.SessionHeader{TotalElements: 1, TotalSize: 0}
	if err := sendPairingSentinel(sess, hdr); err != nil {
		return fmt.Errorf("pair sentinel: %w", err)
	}
	return nil
}

// sendPairingSentinel writes a zero-byte text element through the Noise
// session so the receiver sees a proper SessionHeader+ElementHeader+EOF
// flow. Used as the "ack" that closes a pairing handshake.
func sendPairingSentinel(sess *tunnel.Session, hdr protocol.SessionHeader) error {
	if err := protocol.WriteSessionHeader(sess, hdr); err != nil {
		return err
	}
	return protocol.WriteElementHeader(sess, protocol.ElementHeader{
		Name: protocol.TextElementName,
		Size: 0,
	})
}

// autoPinFromX25519 records a peer's long-term identity in the TOFU
// table after a PSK-authenticated handshake. The Ed25519 fingerprint is
// recovered from the discovery messenger's verified peer table by
// matching the X25519 → Ed25519 (via the Edwards-to-Montgomery
// equivalence we verified at startup).
func (a *App) autoPinFromX25519(remote net.Addr, x25519 []byte) error {
	addrStr := ""
	if udp, ok := remote.(*net.TCPAddr); ok {
		addrStr = udp.IP.String()
	} else {
		addrStr = remote.String()
		if i := strings.LastIndex(addrStr, ":"); i >= 0 {
			addrStr = addrStr[:i]
		}
	}
	pub, err := a.findPubKeyForAddress(addrStr)
	if err != nil {
		return fmt.Errorf("autoPin: %w", err)
	}
	expectedX, err := identity.Ed25519PubToX25519Pub(pub)
	if err != nil || !bytesEqual(expectedX[:], x25519) {
		return fmt.Errorf("autoPin: x25519 doesn't match advertised ed25519 for %s", addrStr)
	}
	fp := identity.Fingerprint(pub)
	rec := settings.PinnedPeer{
		Fingerprint:   fp,
		Ed25519PubHex: hex.EncodeToString(pub),
		Label:         a.labelForAddress(addrStr),
		PinnedAt:      time.Now(),
	}
	if err := a.settings.Update(func(v *settings.Values) {
		if v.PinnedPeers == nil {
			v.PinnedPeers = map[string]settings.PinnedPeer{}
		}
		v.PinnedPeers[fp] = rec
	}); err != nil {
		return err
	}
	if a.audit != nil {
		a.audit.Append(audit.Entry{
			Time: time.Now(), Kind: "peer_pinned",
			Peer: rec.Label, Reason: "psk-pairing/" + fp,
		})
	}
	return nil
}

// _ silences the "unused netip" import when someone removes the
// parseAddrPort helper later. parseAddrPort lives in bindings_peers.go
// already, so keep the import linked here too.
var _ = netip.AddrPort{}

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

// TOFUMismatch is the event payload sent to the frontend when an inbound
// v2 handshake produces a remote_static that doesn't match the X25519
// derived from the peer's already-pinned Ed25519 fingerprint.
type TOFUMismatch struct {
	Address           string `json:"address"`
	OldFingerprint    string `json:"oldFingerprint"`
	NewFingerprint    string `json:"newFingerprint"`
	Label             string `json:"label,omitempty"`
}

// checkTOFUMismatch returns a non-nil mismatch description when:
//   - the peer at remote already has a pinned record (matched by IP →
//     advertised Ed25519 pubkey), AND
//   - the X25519 just received via Noise doesn't match the X25519 derived
//     from that pinned Ed25519 pubkey.
//
// Returns nil when the peer isn't pinned (legitimate first-contact) or
// when the keys agree (legitimate paired session).
func (a *App) checkTOFUMismatch(remote net.Addr, gotX25519 []byte) *TOFUMismatch {
	addrStr := stripPort(remote.String())
	pub, err := a.findPubKeyForAddress(addrStr)
	if err != nil {
		return nil // peer's HELLO not seen yet — first-contact, not a mismatch
	}
	pinned := a.settings.Values().PinnedPeers
	advertisedFP := identity.Fingerprint(pub)
	rec, ok := pinned[advertisedFP]
	if !ok {
		return nil // not previously pinned — no mismatch possible
	}
	pinPub, err := hex.DecodeString(rec.Ed25519PubHex)
	if err != nil {
		return nil
	}
	expected, err := identity.Ed25519PubToX25519Pub(ed25519.PublicKey(pinPub))
	if err != nil {
		return nil
	}
	if bytesEqual(expected[:], gotX25519) {
		return nil
	}
	return &TOFUMismatch{
		Address:        addrStr,
		OldFingerprint: rec.Fingerprint,
		NewFingerprint: identity.Fingerprint(pub),
		Label:          rec.Label,
	}
}

// onPeerIdentityRotation is the discovery-layer hook called when an IP
// that previously announced ed_pub_old in 0x06/0x07 now announces
// ed_pub_new. Fires the existing tofu_mismatch UI event when (and only
// when) the OLD fingerprint was in our pinned table — otherwise the
// rotation is just a normal "this peer reinstalled" or a brand-new
// peer at a recycled IP, and warning would be noise.
func (a *App) onPeerIdentityRotation(addr netip.Addr, oldPub, newPub []byte) {
	oldFP := identity.Fingerprint(ed25519.PublicKey(oldPub))
	newFP := identity.Fingerprint(ed25519.PublicKey(newPub))
	pinned := a.settings.Values().PinnedPeers
	rec, ok := pinned[oldFP]
	if !ok {
		return // not previously pinned → silent
	}
	mismatch := TOFUMismatch{
		Address:        addr.String(),
		OldFingerprint: oldFP,
		NewFingerprint: newFP,
		Label:          rec.Label,
	}
	a.emitTOFUMismatch(mismatch)
}

// emitTOFUMismatch ships the mismatch payload to the frontend so the UI
// can pop a "Identity changed" modal. Also writes to the audit log so
// the event is preserved across UI restarts.
func (a *App) emitTOFUMismatch(m TOFUMismatch) {
	if a.audit != nil {
		a.audit.Append(audit.Entry{
			Time: time.Now(),
			Kind: "tofu_mismatch",
			Peer: m.Label,
			Reason: fmt.Sprintf("pinned=%s advertised=%s", m.OldFingerprint, m.NewFingerprint),
		})
	}
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, evtTOFUMismatch, m)
	}
}

// isPubKeyPinned reports whether the X25519 pubkey [x25519] corresponds
// to a peer in the TOFU table. Used by the refuseCleartext gate on the
// responder side.
func (a *App) isPubKeyPinned(x25519 []byte) bool {
	pinned := a.settings.Values().PinnedPeers
	for _, rec := range pinned {
		pub, err := hex.DecodeString(rec.Ed25519PubHex)
		if err != nil {
			continue
		}
		expected, err := identity.Ed25519PubToX25519Pub(ed25519.PublicKey(pub))
		if err != nil {
			continue
		}
		if bytesEqual(expected[:], x25519) {
			return true
		}
	}
	return false
}

// stripPort returns the IP portion of an "ip:port" or "[v6]:port" string.
func stripPort(s string) string {
	if i := strings.LastIndex(s, ":"); i >= 0 {
		return s[:i]
	}
	return s
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
