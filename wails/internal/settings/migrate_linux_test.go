//go:build linux

package settings

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadQtValues_ParsesINI(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	confDir := filepath.Join(dir, "msec.it")
	if err := os.MkdirAll(confDir, 0o755); err != nil {
		t.Fatal(err)
	}
	conf := `
[General]
DestPath=/home/alice/Downloads
BuddyName=Alice
ThemeColor=#ff00aa
AutoMode=false
DarkMode=true
Notification=1
CloseToTray=0

[R5]
ShowTermsOnStart=false
`
	if err := os.WriteFile(filepath.Join(confDir, "Dukto.conf"), []byte(conf), 0o600); err != nil {
		t.Fatal(err)
	}

	qt, found, err := loadQtValues()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected found=true")
	}
	want := QtValues{
		DestPath:         "/home/alice/Downloads",
		BuddyName:        "Alice",
		ThemeColor:       "#ff00aa",
		AutoMode:         false,
		HasAutoMode:      true,
		DarkMode:         true,
		HasDarkMode:      true,
		ShowTermsOnStart: false,
		HasShowTerms:     true,
		Notification:     true,
		HasNotification:  true,
		CloseToTray:      false,
		HasCloseToTray:   true,
	}
	if !reflect.DeepEqual(qt, want) {
		t.Errorf("got:\n%+v\nwant:\n%+v", qt, want)
	}
}

func TestLoadQtValues_MissingFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	qt, found, err := loadQtValues()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found {
		t.Errorf("expected found=false when file is absent, got %+v", qt)
	}
}
