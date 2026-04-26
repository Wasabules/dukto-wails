package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"dukto/internal/avatar"
	"dukto/internal/discovery"
	"dukto/internal/history"
	"dukto/internal/platform"
	"dukto/internal/protocol"
	"dukto/internal/settings"
	"dukto/internal/transfer"
)

// helloInterval is how often the app re-broadcasts HELLO so other peers keep
// us in their lists. Matches the Qt timer in GuiBehind (10s).
const helloInterval = 10 * time.Second

// startup is invoked by Wails after the window/runtime are ready.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	a.messenger = discovery.New(discovery.Config{
		Port:               protocol.DefaultPort,
		SignatureFunc:      a.currentSignature,
		HelloCooldown:      time.Duration(a.settings.Values().UDPHelloCooldownSeconds) * time.Second,
		IdentityPub:        a.identity.Public,
		IdentityPriv:       a.identity.Private,
		HideFromDiscovery:  a.settings.Values().HideFromDiscovery,
		OnIdentityRotation: a.onPeerIdentityRotation,
		IsPubKeyPinned:     a.isEd25519PubKeyPinned,
	})
	if err := a.messenger.Start(ctx); err != nil {
		log.Printf("dukto: discovery start: %v", err)
		return
	}

	a.eventsStop = make(chan struct{})
	go a.pumpDiscoveryEvents()

	if err := a.startTCPServer(ctx); err != nil {
		log.Printf("dukto: tcp server start: %v", err)
	}

	a.avatarServer = avatar.New(a.currentAvatarRenderer())
	if err := a.avatarServer.Start(protocol.DefaultPort); err != nil {
		log.Printf("dukto: avatar server start: %v", err)
	}

	// Initialise notifications. macOS prompts for permission the first time
	// InitializeNotifications is called; on Windows/Linux it's a no-op.
	if err := runtime.InitializeNotifications(ctx); err != nil {
		log.Printf("dukto: init notifications: %v", err)
	}

	// Native drag-and-drop: forward dropped paths *with cursor coordinates*
	// to the frontend. The UI uses elementFromPoint(x, y) to detect whether
	// the drop landed on a peer card and short-circuit the queue.
	runtime.OnFileDrop(ctx, func(x, y int, paths []string) {
		runtime.EventsEmit(a.ctx, evtFileDrop, map[string]any{
			"paths": paths,
			"x":     x,
			"y":     y,
		})
	})

	// Kick off the first HELLO and the re-broadcast timer.
	if err := a.messenger.SayHello(); err != nil {
		log.Printf("dukto: first hello: %v", err)
	}
	a.pokeManualPeers()
	a.helloStop = make(chan struct{})
	go a.pumpHelloTicker()

	// Seed the idle clock so the auto-disable timer counts from startup,
	// not from epoch. Then start the ticker even if the feature is off —
	// the goroutine is cheap and reading the setting each tick means a
	// runtime toggle takes effect without having to bounce it.
	a.markReceiveActivity()
	a.idleStop = make(chan struct{})
	go a.pumpIdleTicker()

	// Restore the window placement we captured at last shutdown. The window
	// is already visible by the time startup runs, so SetSize/SetPosition
	// will briefly flash — acceptable tradeoff for staying restartable.
	if w := a.settings.Values().Window; w != nil && w.Width > 0 && w.Height > 0 {
		runtime.WindowSetSize(ctx, w.Width, w.Height)
		runtime.WindowSetPosition(ctx, w.X, w.Y)
	}
}

// onBeforeClose fires when the user asks to close the window. With
// CloseToTray enabled, we hide the window instead of quitting — the discovery
// goroutine and the TCP server keep running so incoming transfers still work.
// Re-launching Dukto activates the hidden window via the single-instance
// handler in main.go, which acts as a pragmatic substitute for an actual
// tray icon until one is wired up.
func (a *App) onBeforeClose(ctx context.Context) bool {
	if !a.settings.Values().CloseToTray {
		return false
	}
	runtime.WindowHide(ctx)
	return true
}

// shutdown is invoked by Wails when the window is closing. Registered in main.go.
func (a *App) shutdown(ctx context.Context) {
	// Capture window placement before the frontend tears down. Guards against
	// platforms that return (0,0)/negative dimensions when minimised or
	// hidden — in those cases we keep the previous saved state.
	if w, h := runtime.WindowGetSize(ctx); w > 0 && h > 0 {
		x, y := runtime.WindowGetPosition(ctx)
		_ = a.settings.Update(func(v *settings.Values) {
			v.Window = &settings.WindowState{X: x, Y: y, Width: w, Height: h}
		})
	}
	if a.helloStop != nil {
		close(a.helloStop)
	}
	if a.idleStop != nil {
		close(a.idleStop)
	}
	if a.messenger != nil {
		_ = a.messenger.Stop()
	}
	if a.tcpServer != nil {
		_ = a.tcpServer.Close()
	}
	if a.avatarServer != nil {
		_ = a.avatarServer.Stop()
	}
	if a.eventsStop != nil {
		<-a.eventsStop
	}
	// Close any D-Bus connection the notification service held open.
	runtime.CleanupNotifications(ctx)
}

func (a *App) startTCPServer(ctx context.Context) error {
	ln, err := net.Listen("tcp4", ":"+strconv.Itoa(int(protocol.DefaultPort)))
	if err != nil {
		return err
	}
	a.tcpLn = ln
	a.tcpServer = &transfer.Server{
		NewReceiver: func() *transfer.Receiver {
			v := a.settings.Values()
			// Build the extension reject set fresh per connection so updates
			// to the policy take effect without bouncing the server.
			reject := make(map[string]struct{}, len(v.RejectedExtensions))
			for _, ext := range v.RejectedExtensions {
				reject[strings.ToLower(strings.TrimPrefix(ext, "."))] = struct{}{}
			}
			return &transfer.Receiver{
				Dest:               v.DestPath,
				OnEvent:            a.handleReceiveEvent,
				RejectExtensions:   reject,
				MaxSessionBytes:    int64(v.LargeFileThresholdMB) * 1024 * 1024,
				MaxFilesPerSession: v.MaxFilesPerSession,
				MaxPathDepth:       v.MaxPathDepth,
				AllowSession:       a.checkDiskFree(v.DestPath, v.MinFreeDiskPercent),
				OnProgress: func(done, total int64) {
					runtime.EventsEmit(a.ctx, evtReceiveProgress, map[string]any{
						"bytes": done,
						"total": total,
					})
				},
			}
		},
		Allow:         a.allowConn,
		Upgrade:       a.upgradeServerConn,
		OnSessionMode: a.recordSessionMode,
		OnSessionStart: func(cancel context.CancelFunc) {
			a.cancelMu.Lock()
			a.receiveCancel = cancel
			a.cancelMu.Unlock()
		},
		OnSessionEnd: func() {
			a.cancelMu.Lock()
			a.receiveCancel = nil
			a.cancelMu.Unlock()
			// Clear the latched session-mode flag so the *next* session
			// starts with the default (cleartext) until OnSessionMode
			// reasserts it.
			a.recordSessionMode(false)
		},
		OnAcceptError: func(err error) {
			runtime.EventsEmit(a.ctx, evtReceiveError, err.Error())
		},
	}
	go func() {
		if err := a.tcpServer.Serve(ctx, ln); err != nil {
			log.Printf("dukto: tcp serve: %v", err)
		}
	}()
	return nil
}

func (a *App) pumpDiscoveryEvents() {
	defer close(a.eventsStop)
	for ev := range a.messenger.Events() {
		view := a.peerViewWith(ev.Peer)
		switch ev.Kind {
		case discovery.EventFound:
			// Refresh LastSeenAddr on every verified v2 sighting of a
			// pinned peer — gives the unicast probe loop a fresh
			// target IP without manual config.
			if ev.Peer.V2Capable && len(ev.Peer.PubKey) > 0 {
				a.notePinnedPeerSeen(ev.Peer)
			}
			runtime.EventsEmit(a.ctx, evtPeerFound, view)
		case discovery.EventGone:
			runtime.EventsEmit(a.ctx, evtPeerGone, view)
		}
	}
}

func (a *App) pumpHelloTicker() {
	t := time.NewTicker(helloInterval)
	defer t.Stop()
	for {
		select {
		case <-a.helloStop:
			return
		case <-t.C:
			if err := a.messenger.SayHello(); err != nil {
				log.Printf("dukto: periodic hello: %v", err)
			}
			a.pokeManualPeers()
		}
	}
}

func (a *App) handleReceiveEvent(ev transfer.ReceiveEvent) error {
	payload := map[string]any{
		"name":  ev.Name,
		"size":      ev.Size,
		"path":      ev.LocalPath,
		"text":      ev.Text,
		"total":     ev.Header.TotalElements,
		"bytes":     ev.Header.TotalSize,
		"from":      ev.RemoteAddr,
		"encrypted": a.sessionEncrypted(),
	}
	switch ev.Kind {
	case transfer.EventSessionStart:
		a.markReceiveActivity()
		runtime.EventsEmit(a.ctx, evtReceiveStart, payload)
	case transfer.EventDirectoryCreated:
		runtime.EventsEmit(a.ctx, evtReceiveDir, payload)
	case transfer.EventFileReceived:
		a.markReceiveActivity()
		runtime.EventsEmit(a.ctx, evtReceiveFile, payload)
		a.appendHistory(settings.HistoryItem{
			Kind: "file", Name: ev.Name, Path: ev.LocalPath,
			At: time.Now(), From: ev.RemoteAddr, Encrypted: a.sessionEncrypted(),
		})
	case transfer.EventTextReceived:
		a.markReceiveActivity()
		runtime.EventsEmit(a.ctx, evtReceiveText, payload)
		a.appendHistory(settings.HistoryItem{
			Kind: "text", Text: ev.Text,
			At: time.Now(), From: ev.RemoteAddr, Encrypted: a.sessionEncrypted(),
		})
	case transfer.EventSessionComplete:
		runtime.EventsEmit(a.ctx, evtReceiveDone, payload)
		a.notifySessionComplete(ev)
	case transfer.EventElementRejected:
		runtime.EventsEmit(a.ctx, evtElementRejected, map[string]any{
			"name":   ev.Name,
			"reason": ev.Text,
			"from":   ev.RemoteAddr,
		})
	}
	return nil
}

// appendHistory persists item via the history package and emits a
// history:append event so the frontend can update without re-reading the
// full list.
func (a *App) appendHistory(item settings.HistoryItem) {
	_ = history.Append(a.settings, item)
	runtime.EventsEmit(a.ctx, evtHistoryAppend, history.Payload(item))
}

// notifySessionComplete fires a desktop notification when a transfer finishes,
// if the user has enabled it. Failures are logged but never bubble up — the
// UI toast is the primary signal, notifications are secondary.
func (a *App) notifySessionComplete(ev transfer.ReceiveEvent) {
	if !a.settings.Values().Notifications {
		return
	}
	body := "Transfer complete."
	if ev.Header.TotalElements > 0 {
		body = fmt.Sprintf("Received %d item(s) (%s).", ev.Header.TotalElements, humanSize(ev.Header.TotalSize))
	}
	err := runtime.SendNotification(a.ctx, runtime.NotificationOptions{
		ID:    fmt.Sprintf("dukto-recv-%d", time.Now().UnixNano()),
		Title: "Dukto",
		Body:  body,
	})
	if err != nil {
		log.Printf("dukto: send notification: %v", err)
	}
}

// humanSize renders a byte count as a short "1.2 MB" style string. Local
// because the notification body is the only caller.
func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for n/div >= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

// markReceiveActivity updates the last-activity timestamp that drives the
// idle auto-disable ticker. Called on session start and on each received
// element so multi-file transfers don't trip the timer mid-batch.
func (a *App) markReceiveActivity() {
	a.activityMu.Lock()
	a.lastReceiveActivity = time.Now()
	a.activityMu.Unlock()
}

// idleTickerInterval is how often the auto-disable loop checks for
// inactivity. Small enough to feel snappy when the threshold is a couple of
// minutes, large enough not to churn CPU when the feature is off.
const idleTickerInterval = 30 * time.Second

// pumpIdleTicker periodically evaluates the inactivity threshold. When the
// user has configured IdleAutoDisableMinutes > 0 and no receive activity has
// happened for that long, it flips ReceivingEnabled off, emits a
// receiving:changed event, and fires a desktop notification.
func (a *App) pumpIdleTicker() {
	t := time.NewTicker(idleTickerInterval)
	defer t.Stop()
	for {
		select {
		case <-a.idleStop:
			return
		case <-t.C:
			a.checkIdleAutoDisable()
		}
	}
}

func (a *App) checkIdleAutoDisable() {
	v := a.settings.Values()
	if !v.ReceivingEnabled || v.IdleAutoDisableMinutes <= 0 {
		return
	}
	a.activityMu.Lock()
	last := a.lastReceiveActivity
	a.activityMu.Unlock()
	if last.IsZero() {
		return
	}
	threshold := time.Duration(v.IdleAutoDisableMinutes) * time.Minute
	if time.Since(last) < threshold {
		return
	}
	if err := a.settings.Update(func(v *settings.Values) { v.ReceivingEnabled = false }); err != nil {
		log.Printf("dukto: idle auto-disable persist: %v", err)
		return
	}
	runtime.EventsEmit(a.ctx, evtReceivingChanged, map[string]any{
		"enabled": false,
		"reason":  "idle",
	})
	if a.settings.Values().Notifications {
		err := runtime.SendNotification(a.ctx, runtime.NotificationOptions{
			ID:    fmt.Sprintf("dukto-idle-%d", time.Now().UnixNano()),
			Title: "Dukto",
			Body:  fmt.Sprintf("Reception disabled after %d min of inactivity.", v.IdleAutoDisableMinutes),
		})
		if err != nil {
			log.Printf("dukto: idle notification: %v", err)
		}
	}
}

// checkDiskFree builds the Receiver.AllowSession hook that enforces the
// MinFreeDiskPercent guard. Returns nil (no-op hook) when the setting is off
// or the volume can't be interrogated — erring on the side of "accept" so a
// broken diskFree probe never locks the user out of reception.
func (a *App) checkDiskFree(dest string, minPct int) func(protocol.SessionHeader) error {
	if minPct <= 0 || dest == "" {
		return nil
	}
	return func(h protocol.SessionHeader) error {
		free, total, err := diskFree(dest)
		if err != nil || total == 0 {
			return nil
		}
		needed := uint64(h.TotalSize)
		if free <= needed {
			a.auditDiskReject(dest, free, total, h.TotalSize)
			return fmt.Errorf("transfer: not enough free space (%d needed, %d available)", needed, free)
		}
		remaining := free - needed
		threshold := total * uint64(minPct) / 100
		if remaining < threshold {
			a.auditDiskReject(dest, free, total, h.TotalSize)
			return fmt.Errorf("transfer: would drop below %d%% free (remaining %d, threshold %d)", minPct, remaining, threshold)
		}
		return nil
	}
}

// currentSignature is the single source of truth the discovery layer and the
// Signature RPC both read from.
func (a *App) currentSignature() string {
	user := a.settings.Values().BuddyName
	if user == "" {
		user = platform.Username()
	}
	return protocol.BuildSignature(user, platform.Hostname(), platform.Name())
}
