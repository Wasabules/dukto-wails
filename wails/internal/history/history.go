// Package history owns the persisted receive log. Previously these helpers
// lived inline in app.go; extracting them keeps the Wails binding layer
// focused on event wiring and gives the append / export / payload logic a
// natural unit-test home.
package history

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"dukto/internal/settings"
)

// Cap is the maximum number of HistoryItem entries retained in
// settings.Values.History. Older entries fall off the end. The cap matches
// the previous in-memory cap from App.svelte so the UI sees the same window.
const Cap = 50

// Append prepends item to the persisted history and trims to Cap.
func Append(store *settings.Store, item settings.HistoryItem) error {
	return store.Update(func(v *settings.Values) {
		list := append([]settings.HistoryItem{item}, v.History...)
		if len(list) > Cap {
			list = list[:Cap]
		}
		v.History = list
	})
}

// Clear wipes the persisted receive log. Does not affect files on disk.
func Clear(store *settings.Store) error {
	return store.Update(func(v *settings.Values) { v.History = nil })
}

// Payload shapes a HistoryItem into the JSON the frontend consumes on live
// receive events so the Svelte code path stays uniform between the initial
// bulk load and the incremental append event.
func Payload(it settings.HistoryItem) map[string]any {
	return map[string]any{
		"kind":      it.Kind,
		"name":      it.Name,
		"path":      it.Path,
		"text":      it.Text,
		"at":        it.At.UnixMilli(),
		"from":      it.From,
		"encrypted": it.Encrypted,
	}
}

// All returns every persisted entry as Payload maps, newest first.
func All(store *settings.Store) []map[string]any {
	items := store.Values().History
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		out = append(out, Payload(it))
	}
	return out
}

// Export writes the persisted history to path in either "csv" or "json"
// format and returns the path on success so the UI can offer a "reveal"
// action. Overwrites any existing file at the target path.
func Export(store *settings.Store, format, path string) (string, error) {
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if err := write(f, strings.ToLower(format), store.Values().History); err != nil {
		return "", err
	}
	return path, nil
}

func write(w io.Writer, format string, items []settings.HistoryItem) error {
	switch format {
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(items)
	case "csv":
		cw := csv.NewWriter(w)
		if err := cw.Write([]string{"kind", "name", "path", "text", "at", "from"}); err != nil {
			return err
		}
		for _, it := range items {
			if err := cw.Write([]string{
				it.Kind, it.Name, it.Path, it.Text,
				it.At.UTC().Format(time.RFC3339), it.From,
			}); err != nil {
				return err
			}
		}
		cw.Flush()
		return cw.Error()
	default:
		return fmt.Errorf("unknown format %q", format)
	}
}
