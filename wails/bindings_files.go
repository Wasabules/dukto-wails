package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"dukto/internal/httpserve"
	"dukto/internal/osint"
	"dukto/internal/protocol"
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

// PickFilesToSend opens a native multi-select file picker and returns the
// absolute paths the user chose. Empty slice on cancel.
//
// Anchors the dialog at the user's home directory rather than DestPath:
// users almost always send files from somewhere other than the
// receive dir, and Wails on Linux doesn't restore last-used dialog state.
func (a *App) PickFilesToSend() ([]string, error) {
	paths, err := runtime.OpenMultipleFilesDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select files to send",
	})
	if err != nil {
		return nil, err
	}
	return paths, nil
}

// PickFolderToSend opens a native folder picker and returns the selected
// directory's absolute path, or empty string on cancel. The send-side
// `transfer.Sources` walker will recurse into it.
func (a *App) PickFolderToSend() (string, error) {
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select folder to send",
	})
	if err != nil {
		return "", err
	}
	return dir, nil
}

// serveFile is the http.Handler plugged into Wails' AssetServer. It serves
// two same-origin endpoints over the embedded AssetServer so the WebView
// doesn't run into mixed-content blocking:
//
//   /avatar/peer/<ip>          → proxies http://<ip>:4645/dukto/avatar
//   <anything else>            → file under the current destination dir
//
// Unexported so Wails doesn't auto-bind it as a JS-callable RPC (the
// generator mangles http.ResponseWriter/*http.Request into broken bindings);
// main.go references it via app.serveFile when building AssetServer options.
func (a *App) serveFile(w http.ResponseWriter, r *http.Request) {
	// .png suffix on the avatar paths is load-bearing: Wails' AssetServer
	// otherwise routes any extension-less GET to index.html (SPA fallback)
	// before our handler ever runs.
	switch {
	case r.URL.Path == "/avatar/me.png":
		a.proxyOwnAvatar(w, r)
		return
	case strings.HasPrefix(r.URL.Path, "/avatar/peer/") && strings.HasSuffix(r.URL.Path, ".png"):
		a.proxyPeerAvatar(w, r)
		return
	}
	httpserve.Files(func() string { return a.settings.Values().DestPath }).ServeHTTP(w, r)
}

// proxyOwnAvatar streams our own AvatarServer's PNG back through the
// same-origin AssetServer. Avoids the data: URL plumbing and works whether
// or not the renderer was generated synchronously.
func (a *App) proxyOwnAvatar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	port := int(protocol.DefaultPort + protocol.AvatarPortOffset)
	upstream := fmt.Sprintf("http://127.0.0.1:%d/dukto/avatar", port)
	client := &http.Client{Timeout: 4 * time.Second}
	resp, err := client.Get(upstream)
	if err != nil {
		http.Error(w, "upstream: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		http.Error(w, "upstream status "+resp.Status, http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == http.MethodHead {
		return
	}
	_, _ = io.Copy(w, &io.LimitedReader{R: resp.Body, N: 256 * 1024})
}

// proxyPeerAvatar fetches a peer's avatar from its HTTP side-channel and
// streams the bytes back over the embedded AssetServer so the page can
// reference it via a relative URL (same-origin, no mixed-content blocking by
// WebKitGTK).
//
// URL format: /avatar/peer/<ip>[?port=N]. Port defaults to the avatar offset
// from the protocol default (4645). Anything other than GET → 405; bogus IP
// → 400; upstream timeout/failure → 502.
func (a *App) proxyPeerAvatar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ip := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/avatar/peer/"), ".png")
	if ip == "" || strings.ContainsAny(ip, "/?#") {
		http.Error(w, "bad peer ip", http.StatusBadRequest)
		return
	}
	if parsed := net.ParseIP(ip); parsed == nil {
		http.Error(w, "bad peer ip", http.StatusBadRequest)
		return
	}
	port := int(protocol.DefaultPort + protocol.AvatarPortOffset)
	if p := r.URL.Query().Get("port"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v > 0 && v < 65536 {
			port = v
		}
	}
	upstream := fmt.Sprintf("http://%s:%d/dukto/avatar", ip, port)
	client := &http.Client{Timeout: 4 * time.Second}
	resp, err := client.Get(upstream)
	if err != nil {
		http.Error(w, "upstream: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		http.Error(w, "upstream status "+resp.Status, http.StatusBadGateway)
		return
	}
	// Force PNG content-type even if the upstream lies; cap copy at 256 KB so
	// a hostile peer can't make us buffer arbitrary bytes.
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == http.MethodHead {
		return
	}
	_, _ = io.Copy(w, &io.LimitedReader{R: resp.Body, N: 256 * 1024})
}
