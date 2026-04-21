// Package osint groups the small OS-integration helpers the main Wails app
// needs but that don't fit any of the transfer/discovery/settings packages:
// path containment checks, hash-on-disk, and the native "reveal" / "open"
// shell-outs. Keeping them here lets app.go stay a thin binding layer.
package osint

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"
)

// IsUnder returns true if target resolves to a path at or below root. Both
// arguments are cleaned first so Windows drive-letter cases work the same as
// POSIX. An empty root always returns false so callers using IsUnder as a
// security gate refuse to serve anything when no destination is configured.
func IsUnder(root, target string) bool {
	if root == "" {
		return false
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(rootAbs, target)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// FileHashUnder computes the SHA-256 of path, but only if path resolves
// inside root. Used as a defensive "hash a received file" RPC that refuses to
// act as a generic "read arbitrary file" oracle.
func FileHashUnder(root, path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if !IsUnder(root, abs) {
		return "", fmt.Errorf("path outside destination")
	}
	f, err := os.Open(abs)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Reveal selects the file or directory in the platform's file manager.
// Errors from explorer.exe are swallowed on Windows because explorer exits
// non-zero even on success.
func Reveal(path string) error {
	switch goruntime.GOOS {
	case "windows":
		_ = exec.Command("explorer.exe", "/select,", path).Run()
		return nil
	case "darwin":
		return exec.Command("open", "-R", path).Run()
	default:
		// xdg-open doesn't support "reveal"; open the parent dir instead.
		return exec.Command("xdg-open", filepath.Dir(path)).Start()
	}
}

// Open launches the default application for the path (file or directory).
func Open(path string) error {
	switch goruntime.GOOS {
	case "windows":
		// rundll32 is more predictable than cmd /c start for arbitrary paths.
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", path).Start()
	case "darwin":
		return exec.Command("open", path).Start()
	default:
		return exec.Command("xdg-open", path).Start()
	}
}
