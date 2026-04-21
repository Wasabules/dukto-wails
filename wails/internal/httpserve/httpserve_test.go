package httpserve

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFilesServesUnderRoot(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "a.txt")
	if err := os.WriteFile(target, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}

	h := Files(func() string { return root })
	req := httptest.NewRequest(http.MethodGet, "/files?path="+url.QueryEscape(target), nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "content" {
		t.Fatalf("body = %q, want %q", rec.Body.String(), "content")
	}
}

func TestFilesRefusesOutsideRoot(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "b.txt")
	if err := os.WriteFile(outside, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	h := Files(func() string { return root })
	req := httptest.NewRequest(http.MethodGet, "/files?path="+url.QueryEscape(outside), nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestFilesFavicon(t *testing.T) {
	h := Files(func() string { return t.TempDir() })
	req := httptest.NewRequest(http.MethodGet, "/favicon.ico", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
}

func TestStashImage(t *testing.T) {
	const png = "\x89PNG\r\n\x1a\n"
	enc := base64.StdEncoding.EncodeToString([]byte(png))
	dataURL := "data:image/png;base64," + enc

	path, err := StashImage(dataURL, "png")
	if err != nil {
		t.Fatalf("StashImage err: %v", err)
	}
	defer os.Remove(path)
	if !strings.HasSuffix(path, ".png") {
		t.Fatalf("expected .png suffix, got %s", path)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != png {
		t.Fatalf("stashed bytes = %q, want %q", b, png)
	}
}

func TestStashImageRejectsNonDataURL(t *testing.T) {
	if _, err := StashImage("https://example.com/a.png", "png"); err == nil {
		t.Fatal("expected error for non-data URL")
	}
	if _, err := StashImage("data:image/png,raw", "png"); err == nil {
		t.Fatal("expected error for non-base64 data URL")
	}
}
