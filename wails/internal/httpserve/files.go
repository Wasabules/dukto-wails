// Package httpserve implements the small HTTP handlers the Wails webview
// calls directly: the /files fall-through that streams received media for
// inline preview, and the /favicon.ico silencer. The handlers take their
// "destination root" from a callback so callers can reflect live settings
// updates without rebuilding the handler.
package httpserve

import (
	"mime"
	"net/http"
	"os"
	"path/filepath"

	"dukto/internal/osint"
)

// Files returns an http.Handler that serves files under the directory
// returned by destFn, and answers /favicon.ico with 204. Any request outside
// the destination is rejected with 403.
func Files(destFn func() string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Browsers auto-request /favicon.ico and pollute the devtools console
		// with a 404 when we don't answer it. Hand back an empty 204 so the
		// request resolves silently; the webview doesn't use the icon.
		if r.URL.Path == "/favicon.ico" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.URL.Path != "/files" {
			http.NotFound(w, r)
			return
		}
		p := r.URL.Query().Get("path")
		if p == "" {
			http.Error(w, "missing path", http.StatusBadRequest)
			return
		}
		abs, err := filepath.Abs(p)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if !osint.IsUnder(destFn(), abs) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		f, err := os.Open(abs)
		if err != nil {
			if os.IsNotExist(err) {
				http.NotFound(w, r)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer f.Close()
		info, err := f.Stat()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if ctype := mime.TypeByExtension(filepath.Ext(abs)); ctype != "" {
			w.Header().Set("Content-Type", ctype)
		}
		// http.ServeContent handles Range, If-Modified-Since, and HEAD for us — a
		// win for videos, which the webview streams in byte-range chunks.
		http.ServeContent(w, r, filepath.Base(abs), info.ModTime(), f)
	})
}
