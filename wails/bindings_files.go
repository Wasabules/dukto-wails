package main

import (
	"fmt"
	"net/http"
	"path/filepath"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"dukto/internal/httpserve"
	"dukto/internal/osint"
)

// CopyToClipboard puts the given text on the system clipboard so the user
// can paste a received snippet with one click.
func (a *App) CopyToClipboard(text string) error {
	return runtime.ClipboardSetText(a.ctx, text)
}

// FileHash computes the SHA-256 of a file under the destination directory.
// Same path-escape guard as RevealInFolder: we refuse anything outside the
// dest dir to avoid being used as a generic "read arbitrary file" oracle.
func (a *App) FileHash(path string) (string, error) {
	return osint.FileHashUnder(a.settings.Values().DestPath, path)
}

// RevealInFolder opens the system file manager with the given file or
// directory selected. Used by the "Show" button on received items.
func (a *App) RevealInFolder(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if !osint.IsUnder(a.settings.Values().DestPath, abs) {
		return fmt.Errorf("path outside destination")
	}
	return osint.Reveal(abs)
}

// OpenPath launches the default application for a file or directory. Same
// security check as RevealInFolder.
func (a *App) OpenPath(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if !osint.IsUnder(a.settings.Values().DestPath, abs) {
		return fmt.Errorf("path outside destination")
	}
	return osint.Open(abs)
}

// StashPastedImage persists a data-URL-encoded image to a temp file and
// returns its path, so the frontend can queue it alongside other files for
// sending.
func (a *App) StashPastedImage(dataURL, extHint string) (string, error) {
	return httpserve.StashImage(dataURL, extHint)
}

// serveFile is the http.Handler plugged into Wails' AssetServer. It serves
// files under the current destination directory so the frontend can render
// <img>/<video>/<audio> previews of received content without leaking access
// to arbitrary files on disk.
//
// Unexported so Wails doesn't auto-bind it as a JS-callable RPC (the
// generator mangles http.ResponseWriter/*http.Request into broken bindings);
// main.go references it via app.serveFile when building AssetServer options.
func (a *App) serveFile(w http.ResponseWriter, r *http.Request) {
	httpserve.Files(func() string { return a.settings.Values().DestPath }).ServeHTTP(w, r)
}
