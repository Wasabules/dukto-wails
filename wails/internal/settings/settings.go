// Package settings holds the user-visible preferences that must survive
// restarts: destination directory, buddy name, theme, tray behaviour, and
// similar. Values are persisted as JSON under os.UserConfigDir()/dukto.
//
// The store is concurrency-safe. Reads are cheap (they return a copy of an
// in-memory struct); writes go through Update(), which persists synchronously
// so a crash moments later doesn't lose the change.
//
// Migration from the Qt-era QSettings store ("msec.it"/"Dukto") is intentionally
// handled in a separate file (migrate.go) so this one stays platform-agnostic.
package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// fileName is the settings JSON file, relative to the app config dir.
const fileName = "settings.json"

// Values is the plain-data snapshot of user preferences. Field tags match the
// Qt key names verbatim where practical — that makes the migrator's job
// straightforward — except where the Qt name is genuinely awkward
// ("R5/ShowTermsOnStart", ThemeColor as a string-encoded Qt color).
type Values struct {
	DestPath         string       `json:"destPath,omitempty"`
	BuddyName        string       `json:"buddyName,omitempty"`
	ThemeColor       string       `json:"themeColor,omitempty"`
	AutoTheme        bool         `json:"autoTheme"`
	DarkMode         bool         `json:"darkMode"`
	ShowTermsOnStart bool         `json:"showTermsOnStart"`
	Notifications    bool         `json:"notifications"`
	CloseToTray      bool         `json:"closeToTray"`
	Window           *WindowState `json:"window,omitempty"`
	// WindowGeometry holds the raw Qt-era window blob captured during
	// migration. We don't know the format (Qt's `saveGeometry()` output is
	// private), so it's kept verbatim for forensic value only. The Window
	// field above is the structured replacement we actually use.
	WindowGeometry []byte `json:"windowGeometry,omitempty"`

	// History is the receive log shown in the UI's threaded Received panel.
	// Capped by the caller (see app.appendHistory); we persist whatever the
	// caller passes without trimming here, so tests can round-trip arbitrary
	// shapes. A missing history field deserialises to the empty slice.
	History []HistoryItem `json:"history,omitempty"`

	// Whitelist-related fields. When WhitelistEnabled is true, the transfer
	// server drops connections whose signature (looked up via the messenger's
	// IP→peer map) is not in Whitelist. Entries are full signatures
	// ("User at Host (Platform)") — chosen over IPs so DHCP churn doesn't
	// invalidate the list. The tradeoff is that a malicious LAN peer could
	// spoof a trusted signature, but Dukto is already documented as
	// trusted-LAN-only so this is acceptable.
	WhitelistEnabled bool     `json:"whitelistEnabled"`
	Whitelist        []string `json:"whitelist,omitempty"`

	// RejectedExtensions is a list of lowercase extensions (without the leading
	// dot) that the receiver refuses on sight. Defaults seed common
	// executable/script extensions; users can override in the settings drawer.
	RejectedExtensions []string `json:"rejectedExtensions,omitempty"`

	// LargeFileThresholdMB, if > 0, causes the receiver to refuse any session
	// whose total size exceeds the threshold from a non-whitelisted peer. Set
	// to 0 to disable the cap entirely.
	LargeFileThresholdMB int `json:"largeFileThresholdMB"`

	// Aliases maps a peer signature to a friendly name used only locally in
	// the UI. The wire signature is not rewritten — this is presentation-only.
	Aliases map[string]string `json:"aliases,omitempty"`

	// ManualPeers is a list of "ip" or "ip:port" strings that the messenger
	// unicasts HELLO to on every tick, complementing the broadcast discovery
	// path. Useful across subnets where UDP broadcast doesn't carry.
	ManualPeers []string `json:"manualPeers,omitempty"`

	// ReceivingEnabled is the user-facing "accept incoming transfers" master
	// switch. When false, the TCP accept callback refuses every connection
	// regardless of whitelist. Gated server-side so a compromised/buggy
	// frontend cannot bypass it.
	ReceivingEnabled bool `json:"receivingEnabled"`

	// IdleAutoDisableMinutes, if > 0, turns ReceivingEnabled off after the
	// given number of minutes without a received session. 0 disables the
	// timer (the "always on" behaviour most users will want).
	IdleAutoDisableMinutes int `json:"idleAutoDisableMinutes"`

	// BlockedPeers is the inverse of Whitelist. Signatures listed here are
	// always denied at accept time, regardless of WhitelistEnabled. Useful
	// for permanently muting a specific spammer without having to lock the
	// app down to an allow-list.
	BlockedPeers []string `json:"blockedPeers,omitempty"`

	// TCPAcceptCooldownSeconds is the minimum delay between two accepted
	// sessions from the same remote IP. 0 disables the rate-limit. When
	// active, the second burst hit inside the window is rejected and
	// audit-logged.
	TCPAcceptCooldownSeconds int `json:"tcpAcceptCooldownSeconds"`

	// UDPHelloCooldownSeconds rate-limits incoming HELLO packets per source
	// IP in discovery. 0 disables. Small values (1–2 s) are usually enough
	// to block a broadcast-storm attacker without dropping legitimate peers.
	UDPHelloCooldownSeconds int `json:"udpHelloCooldownSeconds"`

	// ConfirmUnknownPeers, when true, makes the receiver block every
	// session from a peer we've never previously accepted until the user
	// approves it through a UI modal. Timeout (60 s) auto-rejects.
	ConfirmUnknownPeers bool `json:"confirmUnknownPeers"`

	// ApprovedPeerSigs is the persisted "seen and approved" set used by
	// ConfirmUnknownPeers. A peer's first accepted session adds its
	// signature here so subsequent transfers don't re-prompt.
	ApprovedPeerSigs []string `json:"approvedPeerSigs,omitempty"`

	// MaxFilesPerSession caps the element count inside one session.
	// 0 disables. Protects against "10 million tiny files" attacks that
	// would otherwise exhaust inodes or the file descriptor table.
	MaxFilesPerSession int `json:"maxFilesPerSession"`

	// MaxPathDepth caps the number of '/' segments in any incoming element
	// name. 0 disables. A low value (10–20) mitigates path-blowup tricks.
	MaxPathDepth int `json:"maxPathDepth"`

	// MinFreeDiskPercent, if > 0, causes the receiver to refuse a session
	// when the destination filesystem would drop below this free-space
	// percentage after the transfer. 0 disables.
	MinFreeDiskPercent int `json:"minFreeDiskPercent"`

	// AllowedInterfaces filters which local network interfaces the server
	// will accept connections on. Empty = accept everywhere. Matches
	// against the interface's display name (e.g. "eth0", "Wi-Fi").
	AllowedInterfaces []string `json:"allowedInterfaces,omitempty"`
}

// HistoryItem is one received file or text snippet, persisted so the threaded
// Received panel survives restarts. Kept deliberately small — no thumbnails,
// no payload bytes for files — so the settings JSON stays cheap to read.
type HistoryItem struct {
	Kind string    `json:"kind"`           // "file" or "text"
	Name string    `json:"name,omitempty"` // filename for kind=file, empty for text
	Path string    `json:"path,omitempty"` // local absolute path for kind=file
	Text string    `json:"text,omitempty"` // snippet body for kind=text
	At   time.Time `json:"at"`
	From string    `json:"from,omitempty"` // "ip:port" of the sender
}

// WindowState is the persisted window placement. All four fields are required;
// an absent Window field is treated as "first run, use defaults".
type WindowState struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

// defaults returns the initial values for a brand-new install. DestPath is
// left empty so that the caller can decide the platform-specific default
// (typically ~/Downloads) and keep the logic out of this package.
func defaults() Values {
	return Values{
		AutoTheme:            true,
		DarkMode:             false,
		ShowTermsOnStart:     true,
		Notifications:        false,
		CloseToTray:          false,
		WhitelistEnabled:       false,
		LargeFileThresholdMB:   0,
		ReceivingEnabled:       true,
		IdleAutoDisableMinutes: 0,
		RejectedExtensions: []string{
			"exe", "bat", "cmd", "com", "scr", "msi", "ps1", "vbs", "jse", "lnk",
		},
	}
}

// Store is a concurrency-safe, JSON-backed settings store.
type Store struct {
	path string

	mu  sync.RWMutex
	val Values
}

// Open loads the settings file at path. If the file does not exist, a default
// Store is returned and the file is NOT created until the first Update() call
// — this avoids leaving empty settings behind for a user who ran Dukto once
// and uninstalled it.
//
// Malformed existing files surface as an error rather than being silently
// overwritten, so the user can recover by hand if needed.
func Open(path string) (*Store, error) {
	s := &Store{path: path, val: defaults()}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return s, nil
		}
		return nil, fmt.Errorf("settings: read %q: %w", path, err)
	}
	if err := json.Unmarshal(data, &s.val); err != nil {
		return nil, fmt.Errorf("settings: parse %q: %w", path, err)
	}
	return s, nil
}

// DefaultPath returns the location where the settings file should live, using
// the OS-appropriate config dir. Callers typically pass this to Open.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("settings: resolve config dir: %w", err)
	}
	return filepath.Join(dir, "dukto", fileName), nil
}

// Values returns a snapshot of the current settings. Safe for concurrent use
// because it returns a value copy.
func (s *Store) Values() Values {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.val
}

// Update atomically mutates the settings via fn and persists the result.
// Callers receive a pointer to a copy of the current state; edits made
// through fn are applied under the write lock before being flushed to disk.
func (s *Store) Update(fn func(*Values)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(&s.val)
	return s.persist()
}

// Set overwrites the entire Values atom and persists it.
func (s *Store) Set(v Values) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.val = v
	return s.persist()
}

// persist writes the current values to disk as JSON. It writes to a sibling
// temp file then renames, so a crash mid-write cannot corrupt the settings.
func (s *Store) persist() error {
	if s.path == "" {
		return errors.New("settings: no path configured")
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("settings: mkdir config dir: %w", err)
	}
	data, err := json.MarshalIndent(s.val, "", "  ")
	if err != nil {
		return fmt.Errorf("settings: marshal: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("settings: write temp: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("settings: atomic rename: %w", err)
	}
	return nil
}
