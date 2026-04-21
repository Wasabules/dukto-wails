package main

import (
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"dukto/internal/settings"
)

// ----- Receiving master switch RPCs -----------------------------------------

// ReceivingEnabled returns whether the app is currently accepting incoming
// transfers. The gate is enforced in allowAddr, so this is the authoritative
// value for the UI to display.
func (a *App) ReceivingEnabled() bool { return a.settings.Values().ReceivingEnabled }

// SetReceivingEnabled flips the master switch. Turning it on also resets the
// idle clock so the user's deliberate action doesn't get stomped on by a
// stale lastReceiveActivity from before they last disabled it.
func (a *App) SetReceivingEnabled(on bool) error {
	if err := a.settings.Update(func(v *settings.Values) { v.ReceivingEnabled = on }); err != nil {
		return err
	}
	if on {
		a.markReceiveActivity()
	}
	runtime.EventsEmit(a.ctx, evtReceivingChanged, map[string]any{
		"enabled": on,
		"reason":  "user",
	})
	return nil
}

// IdleAutoDisableMinutes returns the current inactivity timeout. 0 means
// the feature is off.
func (a *App) IdleAutoDisableMinutes() int {
	return a.settings.Values().IdleAutoDisableMinutes
}

// SetIdleAutoDisableMinutes configures the inactivity timeout. Negative
// values are clamped to 0 (disabled); any positive value arms the ticker.
func (a *App) SetIdleAutoDisableMinutes(mins int) error {
	if mins < 0 {
		mins = 0
	}
	if err := a.settings.Update(func(v *settings.Values) { v.IdleAutoDisableMinutes = mins }); err != nil {
		return err
	}
	// Reset the clock so a freshly-armed timer doesn't fire on old idleness.
	a.markReceiveActivity()
	return nil
}
