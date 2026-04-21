package settings

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestOpenWithMigration_NoFile verifies that a brand-new install with no Qt
// settings still produces a working default store rather than an error.
func TestOpenWithMigration_NoFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	s, migrated, err := OpenWithMigration(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if migrated {
		t.Errorf("expected migrated=false on a blank system")
	}
	if s == nil {
		t.Fatal("store is nil")
	}
	if !reflect.DeepEqual(s.Values(), defaults()) {
		t.Errorf("expected defaults, got %+v", s.Values())
	}
}

// TestOpenWithMigration_ExistingJSON verifies that once a JSON file exists,
// OpenWithMigration never clobbers it even if a Qt-era store is also around.
func TestOpenWithMigration_ExistingJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte(`{"destPath":"/tmp/keep","buddyName":"Keep"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	s, migrated, err := OpenWithMigration(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if migrated {
		t.Errorf("expected migrated=false when JSON already exists")
	}
	if got := s.Values().DestPath; got != "/tmp/keep" {
		t.Errorf("DestPath=%q, want /tmp/keep", got)
	}
}

func TestApplyQtValues(t *testing.T) {
	v := defaults()
	qt := QtValues{
		DestPath:        "/home/u/Downloads",
		BuddyName:       "Alice",
		AutoMode:        false,
		HasAutoMode:     true,
		Notification:    true,
		HasNotification: true,
		WindowGeometry:  []byte{0x01, 0x02, 0x03},
	}
	applyQtValues(&v, qt)

	if v.DestPath != "/home/u/Downloads" {
		t.Errorf("DestPath=%q", v.DestPath)
	}
	if v.BuddyName != "Alice" {
		t.Errorf("BuddyName=%q", v.BuddyName)
	}
	if v.AutoTheme != false {
		t.Errorf("AutoTheme=%v", v.AutoTheme)
	}
	if v.Notifications != true {
		t.Errorf("Notifications=%v", v.Notifications)
	}
	if !reflect.DeepEqual(v.WindowGeometry, []byte{0x01, 0x02, 0x03}) {
		t.Errorf("WindowGeometry=%v", v.WindowGeometry)
	}
}

// TestApplyQtValues_DefaultsPreservedWhenHasFlagsUnset: if a Qt install never
// wrote a given bool key, our own defaults must remain intact instead of
// silently flipping to zero.
func TestApplyQtValues_DefaultsPreservedWhenHasFlagsUnset(t *testing.T) {
	v := defaults()
	// Qt side has ShowTermsOnStart=false but we haven't marked it "Has".
	qt := QtValues{
		ShowTermsOnStart: false,
	}
	applyQtValues(&v, qt)
	if v.ShowTermsOnStart != true {
		t.Errorf("ShowTermsOnStart should keep default true when HasShowTerms is false, got %v", v.ShowTermsOnStart)
	}
}
