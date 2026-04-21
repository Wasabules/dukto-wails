package main

import (
	"fmt"
	"net"
	"net/netip"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"dukto/internal/audit"
	"dukto/internal/protocol"
	"dukto/internal/settings"
)

// allowConn is transfer.Server.Allow. It walks every security check in
// priority order (master switch → block list → rate limit → interface →
// whitelist → confirm-unknown) and writes an audit entry for each rejection
// or acceptance. Returning false closes the socket immediately; callers have
// no way to surface the reason to the sender, which is intentional — a
// would-be attacker should not learn why they were blocked.
func (a *App) allowConn(conn net.Conn) bool {
	v := a.settings.Values()
	remote := conn.RemoteAddr()
	ip := remoteIP(remote)
	sig := a.signatureFor(ip)

	// Master switch: refuse everyone when the UI has turned reception off.
	// Evaluated before everything else so an empty whitelist isn't the only
	// way to lock down a running instance.
	if !v.ReceivingEnabled {
		a.auditReject(ip, sig, "receiving.disabled", "")
		return false
	}
	// Block list wins over whitelist. A blocked peer is always refused even
	// if the user just added them to the whitelist by accident.
	if sig != "" && containsString(v.BlockedPeers, sig) {
		a.auditReject(ip, sig, "blocklist.hit", "")
		return false
	}
	// Per-IP TCP accept rate-limit.
	if v.TCPAcceptCooldownSeconds > 0 && !a.reserveAcceptSlot(ip, time.Duration(v.TCPAcceptCooldownSeconds)*time.Second) {
		a.auditReject(ip, sig, "ratelimit.tcp", "")
		return false
	}
	// Interface allow-list. `""` empty list → accept any interface.
	if len(v.AllowedInterfaces) > 0 && !a.localIfaceAllowed(conn, v.AllowedInterfaces) {
		a.auditReject(ip, sig, "interface.denied", conn.LocalAddr().String())
		return false
	}
	// Whitelist mode: only previously-trusted signatures pass.
	if v.WhitelistEnabled {
		if len(v.Whitelist) == 0 || sig == "" || !containsString(v.Whitelist, sig) {
			a.auditReject(ip, sig, "whitelist.deny", "")
			return false
		}
		a.auditAccept(ip, sig, "whitelist.allow")
		return true
	}
	// Confirm-unknown: if the peer hasn't been approved before, ask the UI.
	if v.ConfirmUnknownPeers && sig != "" && !containsString(v.ApprovedPeerSigs, sig) {
		approved := a.awaitSessionApproval(ip, sig)
		if !approved {
			a.auditReject(ip, sig, "confirm.denied", "")
			return false
		}
		// Remember the approval so subsequent sessions don't re-prompt.
		_ = a.settings.Update(func(v *settings.Values) {
			if !containsString(v.ApprovedPeerSigs, sig) {
				v.ApprovedPeerSigs = append(v.ApprovedPeerSigs, sig)
			}
		})
		a.auditAccept(ip, sig, "confirm.approved")
		return true
	}
	a.auditAccept(ip, sig, "accept")
	return true
}

// signatureFor looks up the peer's advertised signature by IP via the
// messenger's discovery map. Empty string if the peer hasn't sent a HELLO,
// or if the messenger isn't ready yet.
func (a *App) signatureFor(ip netip.Addr) string {
	if a.messenger == nil || !ip.IsValid() {
		return ""
	}
	for _, p := range a.messenger.Peers() {
		if p.Addr == ip {
			return p.Signature
		}
	}
	return ""
}

// reserveAcceptSlot implements the per-IP accept cooldown. Returns true if
// the caller may proceed, updating the last-accepted timestamp. Returns
// false if the previous accept was within `cool` of now.
func (a *App) reserveAcceptSlot(ip netip.Addr, cool time.Duration) bool {
	if !ip.IsValid() {
		return true
	}
	a.rateMu.Lock()
	defer a.rateMu.Unlock()
	now := time.Now()
	if last, ok := a.lastAccept[ip]; ok && now.Sub(last) < cool {
		return false
	}
	a.lastAccept[ip] = now
	return true
}

// localIfaceAllowed reports whether conn.LocalAddr() lives on one of the
// interfaces listed in allowed (by interface name). The lookup walks
// net.Interfaces() once per check — at ~5 interfaces per host this is
// cheap relative to accepting a TCP session.
func (a *App) localIfaceAllowed(conn net.Conn, allowed []string) bool {
	localHost, _, err := net.SplitHostPort(conn.LocalAddr().String())
	if err != nil {
		return false
	}
	localIP, err := netip.ParseAddr(localHost)
	if err != nil {
		return false
	}
	localIP = localIP.Unmap()
	ifaces, err := net.Interfaces()
	if err != nil {
		return false
	}
	for _, iface := range ifaces {
		if !containsString(allowed, iface.Name) {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipn, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ia, ok := netip.AddrFromSlice(ipn.IP)
			if !ok {
				continue
			}
			if ia.Unmap() == localIP {
				return true
			}
		}
	}
	return false
}

// awaitSessionApproval blocks until either the UI approves the session, the
// user rejects it, or 60 s elapses. Each pending request has a unique ID
// shared with the frontend so concurrent connect attempts are resolvable
// one-by-one. A rejection (timeout included) returns false.
const sessionConfirmTimeout = 60 * time.Second

func (a *App) awaitSessionApproval(ip netip.Addr, sig string) bool {
	a.pendingMu.Lock()
	a.pendingSeq++
	id := fmt.Sprintf("req-%d", a.pendingSeq)
	ch := make(chan bool, 1)
	a.pendingSessions[id] = ch
	a.pendingMu.Unlock()

	defer func() {
		a.pendingMu.Lock()
		delete(a.pendingSessions, id)
		a.pendingMu.Unlock()
	}()

	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, evtPendingSession, map[string]any{
			"id":        id,
			"remote":    ip.String(),
			"signature": sig,
			"timeout":   int(sessionConfirmTimeout / time.Second),
		})
	}

	select {
	case ok := <-ch:
		return ok
	case <-time.After(sessionConfirmTimeout):
		return false
	}
}

// ResolvePendingSession is the RPC the UI calls from its confirm modal.
// Passing an unknown id is a no-op so the user-facing code doesn't need to
// worry about races with the timeout.
func (a *App) ResolvePendingSession(id string, allow bool) {
	a.pendingMu.Lock()
	ch, ok := a.pendingSessions[id]
	a.pendingMu.Unlock()
	if !ok {
		return
	}
	select {
	case ch <- allow:
	default:
	}
}

// auditAccept / auditReject are thin helpers around the audit Log so call
// sites stay readable. Both swallow the underlying write error.
func (a *App) auditAccept(ip netip.Addr, sig, reason string) {
	if a.audit == nil {
		return
	}
	a.audit.Append(audit.Entry{
		Kind:   "accept",
		Reason: reason,
		Remote: ip.String(),
		Peer:   sig,
	})
}

func (a *App) auditReject(ip netip.Addr, sig, reason, detail string) {
	if a.audit == nil {
		return
	}
	a.audit.Append(audit.Entry{
		Kind:   "reject",
		Reason: reason,
		Remote: ip.String(),
		Peer:   sig,
		Detail: detail,
	})
}

// auditDiskReject is called by the disk-guard hook when an incoming session
// would exceed the configured free-space budget. Split out because the
// remote/peer fields aren't known at the hook site (the session header has
// already been parsed but not associated with a signature).
func (a *App) auditDiskReject(dest string, free, total uint64, needed int64) {
	if a.audit == nil {
		return
	}
	a.audit.Append(audit.Entry{
		Kind:   "reject",
		Reason: "disk.low",
		Detail: fmt.Sprintf("dest=%q free=%d total=%d needed=%d", dest, free, total, needed),
	})
}

// remoteIP extracts the sender IP from a net.Addr, returning the zero addr
// if parsing fails (which makes downstream IsValid() checks short-circuit).
func remoteIP(a net.Addr) netip.Addr {
	if a == nil {
		return netip.Addr{}
	}
	host, _, err := net.SplitHostPort(a.String())
	if err != nil {
		return netip.Addr{}
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}
	}
	return addr.Unmap()
}

// containsString is a small helper we use in a handful of allow-list checks;
// inlining `slices.Contains` would be tidier but staff hasn't bumped the
// module's go.mod to 1.21+ yet, so keep this local.
func containsString(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// ----- Whitelist RPCs --------------------------------------------------------

// WhitelistEnabled returns whether the allow-list gate is active.
func (a *App) WhitelistEnabled() bool { return a.settings.Values().WhitelistEnabled }

// SetWhitelistEnabled toggles the allow-list. Turning it on with an empty
// Whitelist will refuse every transfer until entries are added — the UI
// should warn before flipping this on.
func (a *App) SetWhitelistEnabled(on bool) error {
	return a.settings.Update(func(v *settings.Values) { v.WhitelistEnabled = on })
}

// Whitelist returns the signatures currently allowed when WhitelistEnabled.
func (a *App) Whitelist() []string {
	src := a.settings.Values().Whitelist
	out := make([]string, len(src))
	copy(out, src)
	return out
}

// AddWhitelist appends a signature, de-duping so repeated UI clicks are safe.
func (a *App) AddWhitelist(sig string) error {
	sig = strings.TrimSpace(sig)
	if sig == "" {
		return fmt.Errorf("signature required")
	}
	return a.settings.Update(func(v *settings.Values) {
		for _, s := range v.Whitelist {
			if s == sig {
				return
			}
		}
		v.Whitelist = append(v.Whitelist, sig)
	})
}

// RemoveWhitelist drops a signature from the allow-list.
func (a *App) RemoveWhitelist(sig string) error {
	return a.settings.Update(func(v *settings.Values) {
		out := v.Whitelist[:0]
		for _, s := range v.Whitelist {
			if s != sig {
				out = append(out, s)
			}
		}
		v.Whitelist = out
	})
}

// ----- Extension policy RPCs -------------------------------------------------

// RejectedExtensions returns the current auto-reject list.
func (a *App) RejectedExtensions() []string {
	src := a.settings.Values().RejectedExtensions
	out := make([]string, len(src))
	copy(out, src)
	return out
}

// SetRejectedExtensions replaces the auto-reject list. The input is
// normalised (lowercase, dot-stripped) so the receiver's set lookup is O(1).
func (a *App) SetRejectedExtensions(exts []string) error {
	normalised := make([]string, 0, len(exts))
	seen := map[string]struct{}{}
	for _, e := range exts {
		e = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(e, ".")))
		if e == "" {
			continue
		}
		if _, dup := seen[e]; dup {
			continue
		}
		seen[e] = struct{}{}
		normalised = append(normalised, e)
	}
	return a.settings.Update(func(v *settings.Values) { v.RejectedExtensions = normalised })
}

// ----- Aliases RPCs ----------------------------------------------------------

// Aliases returns the full signature→alias map. Safe to mutate; returns a copy.
func (a *App) Aliases() map[string]string {
	src := a.settings.Values().Aliases
	out := make(map[string]string, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

// SetAlias assigns a local alias to a peer signature. Empty alias removes
// the entry so the UI falls back to the raw signature's user component.
func (a *App) SetAlias(sig, alias string) error {
	sig = strings.TrimSpace(sig)
	if sig == "" {
		return fmt.Errorf("signature required")
	}
	alias = strings.TrimSpace(alias)
	return a.settings.Update(func(v *settings.Values) {
		if v.Aliases == nil {
			v.Aliases = map[string]string{}
		}
		if alias == "" {
			delete(v.Aliases, sig)
			return
		}
		v.Aliases[sig] = alias
	})
}

// ----- Manual peer RPCs ------------------------------------------------------

// ManualPeers returns the configured cross-subnet peer addresses.
func (a *App) ManualPeers() []string {
	src := a.settings.Values().ManualPeers
	out := make([]string, len(src))
	copy(out, src)
	return out
}

// AddManualPeer validates the address and adds it, firing a HELLO
// immediately so the UI gets a discovery event without waiting for the
// next tick.
func (a *App) AddManualPeer(addr string) error {
	parsed, port, err := parseManualPeer(addr)
	if err != nil {
		return fmt.Errorf("invalid address: %w", err)
	}
	canonical := parsed.String()
	if port != protocol.DefaultPort {
		canonical = netip.AddrPortFrom(parsed, port).String()
	}
	if err := a.settings.Update(func(v *settings.Values) {
		for _, s := range v.ManualPeers {
			if s == canonical {
				return
			}
		}
		v.ManualPeers = append(v.ManualPeers, canonical)
	}); err != nil {
		return err
	}
	if a.messenger != nil {
		a.messenger.UnicastHello(parsed, port)
	}
	return nil
}

// RemoveManualPeer drops a manual peer by its canonical string.
func (a *App) RemoveManualPeer(addr string) error {
	return a.settings.Update(func(v *settings.Values) {
		out := v.ManualPeers[:0]
		for _, s := range v.ManualPeers {
			if s != addr {
				out = append(out, s)
			}
		}
		v.ManualPeers = out
	})
}
