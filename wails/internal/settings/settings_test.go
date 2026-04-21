package settings

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestOpen_NewFileUsesDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Values(); !reflect.DeepEqual(got, defaults()) {
		t.Fatalf("new store = %+v, want defaults %+v", got, defaults())
	}
	// Open must not create the file until first write.
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("settings file should not exist yet; stat err = %v", err)
	}
}

func TestUpdate_PersistsAtomically(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Update(func(v *Values) {
		v.BuddyName = "Alice"
		v.DarkMode = true
	}); err != nil {
		t.Fatal(err)
	}

	// Re-open in a fresh Store to confirm we actually round-tripped through
	// the filesystem rather than just through in-memory state.
	s2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	got := s2.Values()
	if got.BuddyName != "Alice" || !got.DarkMode {
		t.Fatalf("reopened values = %+v", got)
	}
	// Tmp file should not be left behind.
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("tmp file should have been renamed away; stat err = %v", err)
	}
}

func TestOpen_RejectsMalformedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path); err == nil {
		t.Fatal("expected error on malformed settings file")
	}
}

func TestWindowState_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s, _ := Open(path)
	want := &WindowState{X: 120, Y: 80, Width: 1100, Height: 720}
	if err := s.Update(func(v *Values) { v.Window = want }); err != nil {
		t.Fatal(err)
	}
	s2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	got := s2.Values().Window
	if got == nil {
		t.Fatal("Window was nil after reload")
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Window=%+v, want %+v", got, want)
	}
}

func TestSet_ReplacesValuesWholesale(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s, _ := Open(path)
	if err := s.Update(func(v *Values) { v.BuddyName = "A"; v.DarkMode = true }); err != nil {
		t.Fatal(err)
	}
	if err := s.Set(Values{BuddyName: "B"}); err != nil {
		t.Fatal(err)
	}
	got := s.Values()
	if got.BuddyName != "B" || got.DarkMode {
		t.Fatalf("Set did not replace wholesale: %+v", got)
	}
}
