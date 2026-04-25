package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/gif"  // register decoder for image.Decode
	_ "image/jpeg" // register decoder for image.Decode
	"image/png"
	"os"
	"path/filepath"

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
// and refresh the avatar renderer so a name change re-runs the initials
// generator (a custom avatar, if any, is preserved — currentAvatarRenderer
// returns BytesRenderer when avatar.png exists on disk).
func (a *App) SetBuddyName(name string) error {
	if err := a.settings.Update(func(v *settings.Values) { v.BuddyName = name }); err != nil {
		return err
	}
	if a.avatarServer != nil {
		a.avatarServer.SetRenderer(a.currentAvatarRenderer())
	}
	return nil
}

// avatarFilePath is where a user-uploaded avatar lives on disk. Persisted
// alongside settings so it survives restarts and is shared by every
// per-process Renderer call.
func (a *App) avatarFilePath() string {
	dir := filepath.Dir(a.settings.Path())
	return filepath.Join(dir, "avatar.png")
}

// currentAvatarRenderer picks the right Renderer based on whether the user
// has uploaded a custom avatar. When avatar.png exists and is readable, we
// serve those bytes; otherwise we fall back to the deterministic initials
// tile keyed by the current signature.
//
// Side effect: if an on-disk avatar isn't already 64×64, we transparently
// resize it. Older builds (pre-resize fix) persisted the user's file at
// original dimensions, which produced ugly crops on receivers — this pass
// migrates them on next start without needing the user to re-pick.
func (a *App) currentAvatarRenderer() avatar.Renderer {
	data, err := os.ReadFile(a.avatarFilePath())
	if err == nil && len(data) > 0 {
		if normalised, ok := normaliseAvatar64(data); ok {
			// Best-effort overwrite; failure here just means we'll re-do the
			// normalisation next launch.
			_ = os.WriteFile(a.avatarFilePath(), normalised, 0o644)
			return avatar.BytesRenderer(normalised)
		}
		return avatar.BytesRenderer(data)
	}
	return avatar.DefaultRenderer(a.currentSignature())
}

// normaliseAvatar64 returns 64×64 PNG bytes plus true if [data] decoded but
// wasn't already at the canonical size. Returns false on decode failure (the
// caller serves the raw bytes — a broken upload still gets a chance, even
// if the receivers reject it).
func normaliseAvatar64(data []byte) ([]byte, bool) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, false
	}
	b := img.Bounds()
	if b.Dx() == 64 && b.Dy() == 64 {
		return nil, false
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, resizeAvatarTo64(img)); err != nil {
		return nil, false
	}
	return buf.Bytes(), true
}

// HasCustomAvatar reports whether avatar.png exists. The UI uses this to
// decide whether to show the "Reset to initials" button.
func (a *App) HasCustomAvatar() bool {
	info, err := os.Stat(a.avatarFilePath())
	return err == nil && !info.IsDir() && info.Size() > 0
}

// LocalAvatarDataURL returns the current avatar (custom or initials) as a
// data: URL the frontend can drop straight into <img src=...>. Recomputed on
// every call — cheap relative to the network round-trip the alternative
// (fetching localhost:4645) would require.
func (a *App) LocalAvatarDataURL() (string, error) {
	r := a.currentAvatarRenderer()
	data, err := r()
	if err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(data), nil
}

// PickAndSetCustomAvatar opens a native image picker. On confirm, it
// validates that the file decodes as a real image, re-encodes it as PNG (so
// peers always get a uniform format regardless of input), persists it to
// avatar.png, and swaps the renderer.
//
// Returns the new data URL (or "" if the user cancelled). Big images are
// accepted as-is — peers will pay the bandwidth on every fetch but no
// resampling library is pulled in for an MVP.
func (a *App) PickAndSetCustomAvatar() (string, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Pick an avatar (PNG / JPEG)",
		Filters: []runtime.FileFilter{{
			DisplayName: "Images",
			Pattern:     "*.png;*.jpg;*.jpeg;*.gif;*.webp",
		}},
	})
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read avatar: %w", err)
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return "", fmt.Errorf("decode avatar (%s not a recognised image): %w", filepath.Base(path), err)
	}
	resized := resizeAvatarTo64(img)
	var buf bytes.Buffer
	if err := png.Encode(&buf, resized); err != nil {
		return "", fmt.Errorf("re-encode avatar: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(a.avatarFilePath()), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(a.avatarFilePath(), buf.Bytes(), 0o644); err != nil {
		return "", err
	}
	if a.avatarServer != nil {
		a.avatarServer.SetRenderer(avatar.BytesRenderer(buf.Bytes()))
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

// resizeAvatarTo64 center-crops an arbitrary image to a square then
// nearest-neighbor scales it to 64×64. Mirrors the Android side and the
// protocol's canonical avatar size — keeps every peer's tile uniform and
// the bytes-on-the-wire small (high-aspect uploads were producing slivers
// inside the receiver's circular crop).
func resizeAvatarTo64(src image.Image) *image.RGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	side := w
	if h < side {
		side = h
	}
	cx := b.Min.X + (w-side)/2
	cy := b.Min.Y + (h-side)/2
	out := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		srcY := cy + y*side/64
		for x := 0; x < 64; x++ {
			srcX := cx + x*side/64
			out.Set(x, y, src.At(srcX, srcY))
		}
	}
	return out
}

// ClearCustomAvatar removes avatar.png and reverts to the initials renderer.
func (a *App) ClearCustomAvatar() error {
	if err := os.Remove(a.avatarFilePath()); err != nil && !os.IsNotExist(err) {
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

// Fingerprint returns the user-visible 16-character identity fingerprint
// (XXXX-XXXX-XXXX-XXXX) derived from this install's long-term Ed25519
// public key. Empty string if the identity failed to load (logged at
// startup) — UI should hide the field rather than show a placeholder.
//
// The keypair is generated once on first run and persisted under
// <UserConfigDir>/dukto/identity.key. See docs/SECURITY_v2.md.
func (a *App) Fingerprint() string {
	if len(a.identity.Public) == 0 {
		return ""
	}
	return a.identity.Fingerprint()
}

// Theme returns the current theme override as one of "system", "light",
// "dark". Maps the legacy AutoTheme + DarkMode boolean pair onto a single
// tri-state so the frontend doesn't have to deal with two flags.
func (a *App) Theme() string {
	v := a.settings.Values()
	if v.AutoTheme {
		return "system"
	}
	if v.DarkMode {
		return "dark"
	}
	return "light"
}

// SetTheme accepts "system" | "light" | "dark". Anything else returns an
// error rather than silently snapping to a default — surfaces typos in the
// frontend instead of mysterious behaviour.
func (a *App) SetTheme(mode string) error {
	switch mode {
	case "system":
		return a.settings.Update(func(v *settings.Values) { v.AutoTheme = true })
	case "light":
		return a.settings.Update(func(v *settings.Values) {
			v.AutoTheme = false
			v.DarkMode = false
		})
	case "dark":
		return a.settings.Update(func(v *settings.Values) {
			v.AutoTheme = false
			v.DarkMode = true
		})
	default:
		return fmt.Errorf("dukto: unknown theme mode %q", mode)
	}
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
