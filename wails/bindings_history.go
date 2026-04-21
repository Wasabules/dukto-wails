package main

import (
	"fmt"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"dukto/internal/history"
)

// History returns the persisted receive log, newest first. The frontend
// calls this on startup to populate the Received panel.
func (a *App) History() []map[string]any {
	return history.All(a.settings)
}

// ClearHistory wipes the persisted receive log. Does not affect files on disk.
func (a *App) ClearHistory() error {
	return history.Clear(a.settings)
}

// ExportHistory writes the persisted history to path in "csv" or "json".
// Returns the path on success so the UI can offer a "reveal" action.
func (a *App) ExportHistory(format, path string) (string, error) {
	return history.Export(a.settings, format, path)
}

// PickExportPath opens a save-file dialog anchored at the OS Documents dir
// with an appropriate extension filter. Returns the chosen path or an empty
// string if the user cancelled.
func (a *App) PickExportPath(format string) (string, error) {
	ext := strings.ToLower(format)
	if ext != "csv" && ext != "json" {
		return "", fmt.Errorf("unknown format %q", format)
	}
	return runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "Export history",
		DefaultFilename: "dukto-history." + ext,
		Filters: []runtime.FileFilter{
			{DisplayName: strings.ToUpper(ext), Pattern: "*." + ext},
		},
	})
}
