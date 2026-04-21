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
		AutoTheme:        true,
		DarkMode:         false,
		ShowTermsOnStart: true,
		Notifications:    false,
		CloseToTray:      false,
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
