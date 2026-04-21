package httpserve

import (
	"encoding/base64"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// StashImage persists a data-URL-encoded image to a new temp file and
// returns its path. Used by the frontend to convert a clipboard-pasted image
// into a file path that can be queued alongside other sends.
//
// extHint is the preferred extension (e.g. "png"); it names the temp file
// when the data URL's media type is missing. If the data URL does carry a
// recognised media type, that wins.
func StashImage(dataURL, extHint string) (string, error) {
	const prefix = "data:"
	if !strings.HasPrefix(dataURL, prefix) {
		return "", fmt.Errorf("not a data URL")
	}
	comma := strings.Index(dataURL, ",")
	if comma < 0 {
		return "", fmt.Errorf("malformed data URL")
	}
	header := dataURL[len(prefix):comma]
	body := dataURL[comma+1:]
	if !strings.Contains(header, ";base64") {
		return "", fmt.Errorf("only base64 data URLs supported")
	}
	ctype := strings.TrimSuffix(header, ";base64")
	if ctype == "" {
		ctype = "image/" + extHint
	}
	data, err := base64.StdEncoding.DecodeString(body)
	if err != nil {
		return "", fmt.Errorf("decode data URL: %w", err)
	}
	ext := extHint
	if exts, _ := mime.ExtensionsByType(ctype); len(exts) > 0 {
		ext = strings.TrimPrefix(exts[0], ".")
	}
	if ext == "" {
		ext = "bin"
	}
	name := fmt.Sprintf("dukto-paste-%d.%s", time.Now().UnixNano(), ext)
	path := filepath.Join(os.TempDir(), name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", fmt.Errorf("write temp: %w", err)
	}
	return path, nil
}
