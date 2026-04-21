package main

import (
	"fmt"
	"net"
	"strings"
	"time"

	"dukto/internal/audit"
	"dukto/internal/settings"
)

// ----- Block list ------------------------------------------------------------

// BlockedPeers returns the signatures currently on the block list.
func (a *App) BlockedPeers() []string {
	src := a.settings.Values().BlockedPeers
	out := make([]string, len(src))
	copy(out, src)
	return out
}

// BlockPeer adds a signature to the block list. De-duplicates so repeat
// clicks are a no-op. An empty signature is rejected — the UI passes
// Peer.Signature which is only empty before the first HELLO.
func (a *App) BlockPeer(sig string) error {
	sig = strings.TrimSpace(sig)
	if sig == "" {
		return fmt.Errorf("signature required")
	}
	return a.settings.Update(func(v *settings.Values) {
		for _, s := range v.BlockedPeers {
			if s == sig {
				return
			}
		}
		v.BlockedPeers = append(v.BlockedPeers, sig)
	})
}

// UnblockPeer removes a signature from the block list.
func (a *App) UnblockPeer(sig string) error {
	return a.settings.Update(func(v *settings.Values) {
		out := v.BlockedPeers[:0]
		for _, s := range v.BlockedPeers {
			if s != sig {
				out = append(out, s)
			}
		}
		v.BlockedPeers = out
	})
}

// ----- Rate limits -----------------------------------------------------------

// TCPAcceptCooldownSeconds returns the current per-IP TCP cooldown setting.
func (a *App) TCPAcceptCooldownSeconds() int {
	return a.settings.Values().TCPAcceptCooldownSeconds
}

// SetTCPAcceptCooldownSeconds updates the per-IP TCP accept cooldown. 0 turns
// the gate off. Clamped to [0, 600] so a rogue UI value can't strand
// legitimate peers for an hour.
func (a *App) SetTCPAcceptCooldownSeconds(s int) error {
	if s < 0 {
		s = 0
	}
	if s > 600 {
		s = 600
	}
	return a.settings.Update(func(v *settings.Values) { v.TCPAcceptCooldownSeconds = s })
}

// UDPHelloCooldownSeconds returns the current HELLO cooldown.
func (a *App) UDPHelloCooldownSeconds() int {
	return a.settings.Values().UDPHelloCooldownSeconds
}

// SetUDPHelloCooldownSeconds updates the messenger's per-IP HELLO gate and
// persists. The change takes effect immediately via SetHelloCooldown.
func (a *App) SetUDPHelloCooldownSeconds(s int) error {
	if s < 0 {
		s = 0
	}
	if s > 600 {
		s = 600
	}
	if err := a.settings.Update(func(v *settings.Values) { v.UDPHelloCooldownSeconds = s }); err != nil {
		return err
	}
	if a.messenger != nil {
		a.messenger.SetHelloCooldown(time.Duration(s) * time.Second)
	}
	return nil
}

// ----- Confirm unknown peers -------------------------------------------------

// ConfirmUnknownPeers returns whether the UI modal gate is active.
func (a *App) ConfirmUnknownPeers() bool {
	return a.settings.Values().ConfirmUnknownPeers
}

// SetConfirmUnknownPeers toggles the confirm-unknown gate. Approvals already
// cached in ApprovedPeerSigs are kept; turning the gate back on later will
// reuse them so a previously-approved peer isn't re-prompted.
func (a *App) SetConfirmUnknownPeers(on bool) error {
	return a.settings.Update(func(v *settings.Values) { v.ConfirmUnknownPeers = on })
}

// ForgetApprovedPeers drops every cached approval so the next session from
// each peer re-prompts.
func (a *App) ForgetApprovedPeers() error {
	return a.settings.Update(func(v *settings.Values) { v.ApprovedPeerSigs = nil })
}

// ----- Session caps ----------------------------------------------------------

// MaxFilesPerSession returns the current per-session file-count cap.
func (a *App) MaxFilesPerSession() int { return a.settings.Values().MaxFilesPerSession }

// SetMaxFilesPerSession updates the file-count cap. Clamped to [0, 1_000_000].
func (a *App) SetMaxFilesPerSession(n int) error {
	if n < 0 {
		n = 0
	}
	if n > 1_000_000 {
		n = 1_000_000
	}
	return a.settings.Update(func(v *settings.Values) { v.MaxFilesPerSession = n })
}

// MaxPathDepth returns the current per-element depth cap.
func (a *App) MaxPathDepth() int { return a.settings.Values().MaxPathDepth }

// SetMaxPathDepth updates the per-element '/' segment cap. Clamped to [0, 64];
// 0 disables the gate.
func (a *App) SetMaxPathDepth(n int) error {
	if n < 0 {
		n = 0
	}
	if n > 64 {
		n = 64
	}
	return a.settings.Update(func(v *settings.Values) { v.MaxPathDepth = n })
}

// MinFreeDiskPercent returns the disk-free guard threshold.
func (a *App) MinFreeDiskPercent() int { return a.settings.Values().MinFreeDiskPercent }

// SetMinFreeDiskPercent clamps to [0, 99] and persists. 0 disables the guard.
func (a *App) SetMinFreeDiskPercent(n int) error {
	if n < 0 {
		n = 0
	}
	if n > 99 {
		n = 99
	}
	return a.settings.Update(func(v *settings.Values) { v.MinFreeDiskPercent = n })
}

// ----- Network interface allow-list ------------------------------------------

// AllowedInterfaces returns the interface names the server is restricted to.
func (a *App) AllowedInterfaces() []string {
	src := a.settings.Values().AllowedInterfaces
	out := make([]string, len(src))
	copy(out, src)
	return out
}

// SetAllowedInterfaces replaces the list. An empty list means "accept on
// every interface".
func (a *App) SetAllowedInterfaces(names []string) error {
	cleaned := make([]string, 0, len(names))
	seen := map[string]struct{}{}
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		if _, dup := seen[n]; dup {
			continue
		}
		seen[n] = struct{}{}
		cleaned = append(cleaned, n)
	}
	return a.settings.Update(func(v *settings.Values) { v.AllowedInterfaces = cleaned })
}

// InterfaceView is the UI-facing projection of a local network interface. The
// Active flag mirrors FlagUp & !FlagLoopback so the frontend can grey out
// offline adapters without re-implementing the logic.
type InterfaceView struct {
	Name    string   `json:"name"`
	Active  bool     `json:"active"`
	Address []string `json:"addresses"`
}

// AvailableInterfaces returns the list of local network interfaces. Loopback
// is filtered out — Dukto never binds there and showing it in the UI would
// only be noise.
func (a *App) AvailableInterfaces() ([]InterfaceView, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	out := make([]InterfaceView, 0, len(ifaces))
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		v := InterfaceView{
			Name:   iface.Name,
			Active: iface.Flags&net.FlagUp != 0,
		}
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			if ipn, ok := addr.(*net.IPNet); ok && ipn.IP.To4() != nil {
				v.Address = append(v.Address, ipn.IP.String())
			}
		}
		out = append(out, v)
	}
	return out, nil
}

// ----- Audit log -------------------------------------------------------------

// AuditEntries returns the tail of the audit log. Bounded by the audit
// package itself to 2000 lines.
func (a *App) AuditEntries() ([]audit.Entry, error) {
	if a.audit == nil {
		return nil, nil
	}
	return a.audit.Read()
}

// ClearAudit truncates the audit log on disk.
func (a *App) ClearAudit() error {
	if a.audit == nil {
		return nil
	}
	return a.audit.Clear()
}
