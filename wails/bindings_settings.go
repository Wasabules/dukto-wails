package main

import (
	"encoding/base64"
	"fmt"
	"os"

	"github.com/skip2/go-qrcode"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"dukto/internal/avatar"
	"dukto/internal/settings"
)

// DestDir returns the current destination directory.
func (a *App) DestDir() string {
	return a.settings.Values().DestPath
}

// PickDestDir opens a native folder picker and, if the user confirms, persists
// the result as the destination directory. Returns the selected path (empty
// string if the user cancelled). The picker is anchored at the current
// destination so a typical "adjust slightly" flow doesn't start from scratch.
func (a *App) PickDestDir() (string, error) {
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title:            "Choose destination folder",
		DefaultDirectory: a.settings.Values().DestPath,
	})
	if err != nil {
		return "", err
	}
	if dir == "" {
		return "", nil
	}
	if err := a.SetDestDir(dir); err != nil {
		return "", err
	}
	return dir, nil
}

// SetDestDir updates the destination directory. Validation is cheap — the
// frontend typically gets the path from a native folder picker, so the path
// is already known to exist.
func (a *App) SetDestDir(dir string) error {
	if dir == "" {
		return fmt.Errorf("destination cannot be empty")
	}
	info, err := os.Stat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("not a directory: %s", dir)
	}
	return a.settings.Update(func(v *settings.Values) { v.DestPath = dir })
}

// BuddyName returns the user-facing buddy name (may be empty, meaning "use
// OS login name").
func (a *App) BuddyName() string {
	return a.settings.Values().BuddyName
}

// SetBuddyName updates the buddy name. Changes propagate on the next HELLO
// and invalidate the cached avatar so the PNG reflects the new initials.
func (a *App) SetBuddyName(name string) error {
	if err := a.settings.Update(func(v *settings.Values) { v.BuddyName = name }); err != nil {
		return err
	}
	if a.avatarServer != nil {
		a.avatarServer.SetRenderer(avatar.DefaultRenderer(a.currentSignature()))
	}
	return nil
}

// Notifications returns whether desktop notifications are enabled.
func (a *App) Notifications() bool {
	return a.settings.Values().Notifications
}

// SetNotifications toggles desktop notifications.
func (a *App) SetNotifications(enabled bool) error {
	return a.settings.Update(func(v *settings.Values) { v.Notifications = enabled })
}

// CloseToTray reports whether closing the window keeps the app running.
func (a *App) CloseToTray() bool {
	return a.settings.Values().CloseToTray
}

// SetCloseToTray toggles close-to-tray. When on, closing the window hides it
// rather than quitting; relaunching Dukto raises the hidden window via the
// single-instance handler.
func (a *App) SetCloseToTray(enabled bool) error {
	return a.settings.Update(func(v *settings.Values) { v.CloseToTray = enabled })
}

// LargeFileThresholdMB / SetLargeFileThresholdMB expose the receive-size cap.
func (a *App) LargeFileThresholdMB() int { return a.settings.Values().LargeFileThresholdMB }
func (a *App) SetLargeFileThresholdMB(mb int) error {
	if mb < 0 {
		mb = 0
	}
	return a.settings.Update(func(v *settings.Values) { v.LargeFileThresholdMB = mb })
}

// QRCodeSignature returns a PNG QR code (as a data URL) encoding the current
// signature. Handy for peers who want to visually confirm identity —
// especially on a mobile Dukto sender that can scan the screen.
func (a *App) QRCodeSignature() (string, error) {
	png, err := qrcode.Encode(a.currentSignature(), qrcode.Medium, 220)
	if err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(png), nil
}
