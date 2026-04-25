package main

import (
	"crypto/ed25519"
	"fmt"
	"log"
	"net/netip"
	"strings"

	"dukto/internal/discovery"
	"dukto/internal/identity"
	"dukto/internal/protocol"
)

// Peers returns the current peer snapshot.
func (a *App) Peers() []PeerView {
	if a.messenger == nil {
		return nil
	}
	raw := a.messenger.Peers()
	out := make([]PeerView, 0, len(raw))
	for _, p := range raw {
		out = append(out, peerView(p))
	}
	return out
}

// peerView converts a discovery.Peer to its Wails-facing form, including the
// v2 fingerprint derived from the advertised pubkey.
func peerView(p discovery.Peer) PeerView {
	view := PeerView{
		Address:   p.Addr.String(),
		Port:      p.Port,
		Signature: p.Signature,
		V2Capable: p.V2Capable,
	}
	if len(p.PubKey) == ed25519.PublicKeySize {
		view.Fingerprint = identity.Fingerprint(ed25519.PublicKey(p.PubKey))
	}
	return view
}

// Signature returns the signature this app currently advertises. Useful for
// the UI to show "you appear to others as …".
func (a *App) Signature() string {
	return a.currentSignature()
}

// LocalAddresses returns the IPv4 addresses we're listening on. Mirrors Qt's
// "your IPs" debug panel and is invaluable when peers don't see each other:
// they're usually on different subnets or VPNs.
func (a *App) LocalAddresses() []string {
	ifs, err := discovery.SystemInterfaces()
	if err != nil {
		log.Printf("dukto: enumerate interfaces: %v", err)
		return nil
	}
	out := make([]string, 0, len(ifs))
	for _, iface := range ifs {
		out = append(out, fmt.Sprintf("%s (%s)", iface.IP.String(), iface.Name))
	}
	return out
}

// pokeManualPeers unicasts a HELLO to each configured manual peer IP. Runs
// on the same cadence as the broadcast HELLO ticker. Bad entries are logged
// once and skipped; bad format errors are silent (the setter validates).
func (a *App) pokeManualPeers() {
	for _, s := range a.settings.Values().ManualPeers {
		addr, port, err := parseManualPeer(s)
		if err != nil {
			continue
		}
		a.messenger.UnicastHello(addr, port)
	}
}

// parseManualPeer accepts either "1.2.3.4" or "1.2.3.4:port".
func parseManualPeer(s string) (netip.Addr, uint16, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return netip.Addr{}, 0, fmt.Errorf("empty address")
	}
	if ap, err := netip.ParseAddrPort(s); err == nil {
		return ap.Addr().Unmap(), ap.Port(), nil
	}
	addr, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Addr{}, 0, err
	}
	return addr.Unmap(), protocol.DefaultPort, nil
}

// parseAddrPort coerces an "ip:port" string into a netip.AddrPort, falling
// back to the default TCP port if only an address is given.
func parseAddrPort(s string) (netip.AddrPort, error) {
	if ap, err := netip.ParseAddrPort(s); err == nil {
		return ap, nil
	}
	if addr, err := netip.ParseAddr(s); err == nil {
		return netip.AddrPortFrom(addr, protocol.DefaultPort), nil
	}
	return netip.AddrPort{}, fmt.Errorf("invalid address %q", s)
}
