package settings

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// QtValues carries only the subset of fields the Qt codebase persisted. We
// keep it separate from Values so the migrator can signal "value not set"
// via the bool companions rather than silently overwriting our defaults.
type QtValues struct {
	DestPath         string
	BuddyName        string
	ThemeColor       string
	AutoMode         bool
	HasAutoMode      bool
	DarkMode         bool
	HasDarkMode      bool
	ShowTermsOnStart bool
	HasShowTerms     bool
	Notification     bool
	HasNotification  bool
	CloseToTray      bool
	HasCloseToTray   bool
	WindowGeometry   []byte
}

// LoadQtValues reads whatever Qt-era settings exist in the platform-native
// store ("msec.it" / "Dukto") and returns them as QtValues. Returns
// (zero, false, nil) when no Qt settings are present — the common case for
// fresh installs. Errors are surfaced so the caller can log them; they should
// never block startup since migration is best-effort.
func LoadQtValues() (QtValues, bool, error) {
	return loadQtValues()
}

// OpenWithMigration behaves like Open but, if the JSON file is absent, tries
// to seed the store from a Qt-era QSettings store on first run. Migration
// errors are non-fatal: a best-effort migration is always better than none.
//
// The returned *Store already has any migrated values flushed to disk so the
// next launch reads from the JSON directly.
func OpenWithMigration(path string) (*Store, bool, error) {
	if _, err := os.Stat(path); err == nil {
		s, err := Open(path)
		return s, false, err
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, false, fmt.Errorf("settings: stat %q: %w", path, err)
	}

	qt, found, err := loadQtValues()
	s, openErr := Open(path)
	if openErr != nil {
		return nil, false, openErr
	}
	if !found {
		return s, false, err
	}
	if err := s.Update(func(v *Values) { applyQtValues(v, qt) }); err != nil {
		return s, false, err
	}
	return s, true, err
}

// applyQtValues overlays non-zero Qt fields onto v. Booleans are only applied
// when the migrator explicitly recorded them (HasXxx), so a Qt install that
// never opened the relevant dialog won't flip our own defaults.
func applyQtValues(v *Values, qt QtValues) {
	if qt.DestPath != "" {
		v.DestPath = qt.DestPath
	}
	if qt.BuddyName != "" {
		v.BuddyName = qt.BuddyName
	}
	if qt.ThemeColor != "" {
		v.ThemeColor = qt.ThemeColor
	}
	if qt.HasAutoMode {
		v.AutoTheme = qt.AutoMode
	}
	if qt.HasDarkMode {
		v.DarkMode = qt.DarkMode
	}
	if qt.HasShowTerms {
		v.ShowTermsOnStart = qt.ShowTermsOnStart
	}
	if qt.HasNotification {
		v.Notifications = qt.Notification
	}
	if qt.HasCloseToTray {
		v.CloseToTray = qt.CloseToTray
	}
	if len(qt.WindowGeometry) > 0 {
		v.WindowGeometry = append([]byte(nil), qt.WindowGeometry...)
	}
}
