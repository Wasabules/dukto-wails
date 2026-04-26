// Package main is the Wails v2 host for Dukto. The App struct is the single
// type Wails binds to the Svelte frontend; behaviour is split across several
// files in this package (lifecycle.go for startup/shutdown, events.go for
// the wire-level event catalogue, and bindings_*.go for the RPCs the UI
// calls).
package main

import (
	"context"
	"log"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"sync"
	"time"

	"dukto/internal/audit"
	"dukto/internal/avatar"
	"dukto/internal/discovery"
	"dukto/internal/identity"
	"dukto/internal/settings"
	"dukto/internal/transfer"
)

// App is the Wails application struct bound to the Svelte frontend.
type App struct {
	ctx context.Context

	messenger    *discovery.Messenger
	tcpServer    *transfer.Server
	tcpLn        net.Listener
	avatarServer *avatar.Server
	helloStop    chan struct{}
	eventsStop   chan struct{}

	settings *settings.Store

	// identity is this install's long-term Ed25519 keypair. Currently only
	// used to surface a fingerprint in Settings (M1 of the v2 encrypted
	// overlay — see docs/SECURITY_v2.md). Future milestones use it to sign
	// discovery datagrams (M2) and authenticate Noise XX handshakes (M3).
	identity identity.Identity

	// cancelMu guards sendCancel / receiveCancel, which are set while a
	// transfer is in flight and cleared when it finishes. The UI's cancel
	// button fires these via CancelTransfer.
	cancelMu      sync.Mutex
	sendCancel    context.CancelFunc
	receiveCancel context.CancelFunc

	// activityMu guards lastReceiveActivity, which tracks the last moment
	// a receive session made progress. The idle auto-disable ticker reads
	// it to decide whether to flip ReceivingEnabled off.
	activityMu          sync.Mutex
	lastReceiveActivity time.Time
	idleStop            chan struct{}

	// audit is the append-only security event log.
	audit *audit.Log

	// rateMu guards lastAccept, the per-IP cooldown map used to rate-limit
	// TCP session accepts. Kept on the App so the server callback has
	// direct access without another layer of indirection.
	rateMu     sync.Mutex
	lastAccept map[netip.Addr]time.Time

	// pendingMu guards pendingSessions, the table of session-confirm
	// requests awaiting user approval from the UI modal. Keys are unique
	// request IDs; values are channels the Allow callback blocks on.
	pendingMu       sync.Mutex
	pendingSessions map[string]chan bool
	pendingSeq      uint64

	// modeMu guards lastSessionEncrypted — set right after a Server.Upgrade
	// hook runs, read by handleReceiveEvent so the audit log can stamp the
	// session with kind=ENCRYPTED vs CLEARTEXT. Reset on session end.
	// Also guards pendingPair, the in-flight v2 pairing PSK; consuming it
	// is a one-shot operation that the upgrade hook does atomically.
	modeMu               sync.Mutex
	lastSessionEncrypted bool
	pendingPair          *pendingPairing
}

// NewApp creates the App and loads persistent settings. If loading fails
// (malformed file on disk) the app continues with an in-memory default store
// so the user can at least see the error and reset via the UI.
func NewApp() *App {
	a := &App{
		lastAccept:      map[netip.Addr]time.Time{},
		pendingSessions: map[string]chan bool{},
	}
	path, err := settings.DefaultPath()
	if err != nil {
		log.Printf("dukto: settings path: %v — falling back to in-memory store", err)
		a.settings = mustMemStore()
		a.audit = audit.Open(filepath.Join(os.TempDir(), "dukto-audit.log"))
		return a
	}
	store, migrated, err := settings.OpenWithMigration(path)
	if err != nil {
		log.Printf("dukto: load settings: %v — falling back to in-memory store", err)
		a.settings = mustMemStore()
		a.audit = audit.Open(filepath.Join(os.TempDir(), "dukto-audit.log"))
		return a
	}
	if migrated {
		log.Printf("dukto: migrated settings from Qt build (%s)", path)
	}
	a.settings = store
	// Seed the destination directory if the user never picked one.
	if store.Values().DestPath == "" {
		_ = store.Update(func(v *settings.Values) { v.DestPath = defaultDestDir() })
	}
	a.audit = audit.Open(filepath.Join(filepath.Dir(path), "audit.log"))

	// Identity. Failure here is non-fatal at this milestone: M1 only uses
	// the fingerprint for display, so the rest of the app stays usable
	// even if we couldn't load/generate the key. Logged loudly so a real
	// production build won't silently lose the key.
	id, err := identity.LoadOrGenerate(filepath.Join(filepath.Dir(path), "identity.key"))
	if err != nil {
		log.Printf("dukto: identity: %v — running without an Ed25519 keypair", err)
	} else {
		a.identity = id
	}
	return a
}

// mustMemStore returns a store rooted at a throwaway path. Used only as a
// last-resort fallback so NewApp can always succeed.
func mustMemStore() *settings.Store {
	s, _ := settings.Open(filepath.Join(os.TempDir(), "dukto-fallback-settings.json"))
	_ = s.Update(func(v *settings.Values) { v.DestPath = defaultDestDir() })
	return s
}

// defaultDestDir picks a sensible default: ~/Downloads if it exists, else the
// user's home directory, else the current working directory.
func defaultDestDir() string {
	home, err := os.UserHomeDir()
	if err == nil {
		dl := filepath.Join(home, "Downloads")
		if info, err := os.Stat(dl); err == nil && info.IsDir() {
			return dl
		}
		return home
	}
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return "."
}
