package osint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsUnder(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	nested := filepath.Join(root, "sub", "file.txt")
	outside := t.TempDir()

	cases := []struct {
		name   string
		root   string
		target string
		want   bool
	}{
		{"empty root", "", nested, false},
		{"nested path", root, nested, true},
		{"root itself", root, root, true},
		{"parent via dotdot", root, filepath.Join(root, "..", "escape"), false},
		{"unrelated dir", root, outside, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := IsUnder(tc.root, tc.target); got != tc.want {
				t.Fatalf("IsUnder(%q, %q) = %v, want %v", tc.root, tc.target, got, tc.want)
			}
		})
	}
}

func TestFileHashUnder(t *testing.T) {
	root := t.TempDir()
	f := filepath.Join(root, "x.txt")
	if err := os.WriteFile(f, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}

	h, err := FileHashUnder(root, f)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	// sha256("hello")
	const want = "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if h != want {
		t.Fatalf("hash = %s, want %s", h, want)
	}

	outside := filepath.Join(t.TempDir(), "y.txt")
	if err := os.WriteFile(outside, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := FileHashUnder(root, outside); err == nil || !strings.Contains(err.Error(), "outside") {
		t.Fatalf("expected outside-destination error, got %v", err)
	}
}
