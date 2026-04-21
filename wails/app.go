package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"dukto/internal/avatar"
	"dukto/internal/discovery"
	"dukto/internal/platform"
	"dukto/internal/protocol"
	"dukto/internal/settings"
	"dukto/internal/transfer"
)

// helloInterval is how often the app re-broadcasts HELLO so other peers keep
// us in their lists. Matches the Qt timer in GuiBehind (10s).
const helloInterval = 10 * time.Second

// Wails event names consumed by the Svelte frontend.
const (
	evtPeerFound    = "peer:found"
	evtPeerGone     = "peer:gone"
	evtReceiveStart = "receive:start"
	evtReceiveDir   = "receive:dir"
	evtReceiveFile  = "receive:file"
	evtReceiveText  = "receive:text"
	evtReceiveDone  = "receive:done"
	evtReceiveError = "receive:error"
	evtSendError    = "send:error"
	evtFileDrop     = "file:drop"
)

// PeerView is the Wails-facing projection of a discovered peer. Unlike
// discovery.Peer it carries string fields only, so Wails can marshal it.
type PeerView struct {
	Address   string `json:"address"`
	Port      uint16 `json:"port"`
	Signature string `json:"signature"`
}

// App is the Wails application struct bound to the Svelte frontend.
type App struct {
	ctx context.Context

	messenger    *discovery.Messenger
	tcpServer    *transfer.Server
	tcpLn        net.Listener
	avatarServer *avatar.Server
	helloStop    chan struct{}
	eventsStop   chan struct{}

	settings *settings.Store
}

// NewApp creates the App and loads persistent settings. If loading fails
// (malformed file on disk) the app continues with an in-memory default store
// so the user can at least see the error and reset via the UI.
func NewApp() *App {
	a := &App{}
	path, err := settings.DefaultPath()
	if err != nil {
		log.Printf("dukto: settings path: %v — falling back to in-memory store", err)
		a.settings = mustMemStore()
		return a
	}
	store, migrated, err := settings.OpenWithMigration(path)
	if err != nil {
		log.Printf("dukto: load settings: %v — falling back to in-memory store", err)
		a.settings = mustMemStore()
		return a
	}
	if migrated {
		log.Printf("dukto: migrated settings from Qt build (%s)", path)
	}
	a.settings = store
	// Seed the destination directory if the user never picked one.
	if store.Values().DestPath == "" {
		_ = store.Update(func(v *settings.Values) { v.DestPath = defaultDestDir() })
	}
	return a
}

// mustMemStore returns a store rooted at a throwaway path. Used only as a
// last-resort fallback so NewApp can always succeed.
func mustMemStore() *settings.Store {
	s, _ := settings.Open(filepath.Join(os.TempDir(), "dukto-fallback-settings.json"))
	_ = s.Update(func(v *settings.Values) { v.DestPath = defaultDestDir() })
	return s
}

// startup is invoked by Wails after the window/runtime are ready.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	sig := a.currentSignature

	a.messenger = discovery.New(discovery.Config{
		Port:          protocol.DefaultPort,
		SignatureFunc: sig,
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

	a.avatarServer = avatar.New(avatar.DefaultRenderer(a.currentSignature()))
	if err := a.avatarServer.Start(protocol.DefaultPort); err != nil {
		log.Printf("dukto: avatar server start: %v", err)
	}

	// Initialise notifications. macOS prompts for permission the first time
	// InitializeNotifications is called; on Windows/Linux it's a no-op.
	if err := runtime.InitializeNotifications(ctx); err != nil {
		log.Printf("dukto: init notifications: %v", err)
	}

	// Native drag-and-drop: forward dropped paths to the frontend so the user
	// doesn't have to type them. Requires DragAndDrop.EnableFileDrop in options.
	runtime.OnFileDrop(ctx, func(_, _ int, paths []string) {
		runtime.EventsEmit(a.ctx, evtFileDrop, paths)
	})

	// Kick off the first HELLO and the re-broadcast timer.
	if err := a.messenger.SayHello(); err != nil {
		log.Printf("dukto: first hello: %v", err)
	}
	a.helloStop = make(chan struct{})
	go a.pumpHelloTicker()

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
			return &transfer.Receiver{
				Dest:    a.settings.Values().DestPath,
				OnEvent: a.handleReceiveEvent,
			}
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
		view := PeerView{
			Address:   ev.Peer.Addr.String(),
			Port:      ev.Peer.Port,
			Signature: ev.Peer.Signature,
		}
		switch ev.Kind {
		case discovery.EventFound:
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
		}
	}
}

func (a *App) handleReceiveEvent(ev transfer.ReceiveEvent) error {
	payload := map[string]any{
		"name":  ev.Name,
		"size":  ev.Size,
		"path":  ev.LocalPath,
		"text":  ev.Text,
		"total": ev.Header.TotalElements,
		"bytes": ev.Header.TotalSize,
	}
	switch ev.Kind {
	case transfer.EventSessionStart:
		runtime.EventsEmit(a.ctx, evtReceiveStart, payload)
	case transfer.EventDirectoryCreated:
		runtime.EventsEmit(a.ctx, evtReceiveDir, payload)
	case transfer.EventFileReceived:
		runtime.EventsEmit(a.ctx, evtReceiveFile, payload)
	case transfer.EventTextReceived:
		runtime.EventsEmit(a.ctx, evtReceiveText, payload)
	case transfer.EventSessionComplete:
		runtime.EventsEmit(a.ctx, evtReceiveDone, payload)
		a.notifySessionComplete(ev)
	}
	return nil
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

// humanSize renders a byte count as a short "1.2 MB" style string. Kept local
// because it's used only by the notification body.
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

// -----------------------------------------------------------------------------
// Frontend-bindable methods
// -----------------------------------------------------------------------------

// Peers returns the current peer snapshot.
func (a *App) Peers() []PeerView {
	if a.messenger == nil {
		return nil
	}
	raw := a.messenger.Peers()
	out := make([]PeerView, 0, len(raw))
	for _, p := range raw {
		out = append(out, PeerView{
			Address:   p.Addr.String(),
			Port:      p.Port,
			Signature: p.Signature,
		})
	}
	return out
}

// Signature returns the signature this app currently advertises. Useful for
// the UI to show "you appear to others as …".
func (a *App) Signature() string {
	return a.currentSignature()
}

// LocalAddresses returns the IPv4 addresses we're listening on. Mirrors Qt's
// "your IPs" debug panel and is invaluable when peers don't see each other:
// they're usually on different subnets or VPNs.
func (a *App) LocalAddresses() []string {
	ifs, err := discovery.SystemInterfaces()
	if err != nil {
		log.Printf("dukto: enumerate interfaces: %v", err)
		return nil
	}
	out := make([]string, 0, len(ifs))
	for _, iface := range ifs {
		out = append(out, fmt.Sprintf("%s (%s)", iface.IP.String(), iface.Name))
	}
	return out
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

// CopyToClipboard puts the given text on the system clipboard so the user
// can paste a received snippet with one click.
func (a *App) CopyToClipboard(text string) error {
	return runtime.ClipboardSetText(a.ctx, text)
}

// SendText sends a text snippet to peer "ip:port".
func (a *App) SendText(addrPort, text string) error {
	peer, err := parseAddrPort(addrPort)
	if err != nil {
		return err
	}
	go func() {
		if err := transfer.SendText(a.ctx, peer, text); err != nil {
			runtime.EventsEmit(a.ctx, evtSendError, err.Error())
		}
	}()
	return nil
}

// SendFiles sends one or more local filesystem paths to peer "ip:port".
// Directories are recursively flattened. The send runs on a background
// goroutine; errors surface via the send:error Wails event.
func (a *App) SendFiles(addrPort string, paths []string) error {
	peer, err := parseAddrPort(addrPort)
	if err != nil {
		return err
	}
	srcs, hdr, err := transfer.Sources(paths)
	if err != nil {
		return err
	}
	go func() {
		if err := transfer.Dial(a.ctx, peer, srcs, hdr); err != nil {
			runtime.EventsEmit(a.ctx, evtSendError, err.Error())
		}
	}()
	return nil
}

// parseAddrPort coerces an "ip:port" string into a netip.AddrPort, falling
// back to the default TCP port if only an address is given.
func parseAddrPort(s string) (netip.AddrPort, error) {
	if ap, err := netip.ParseAddrPort(s); err == nil {
		return ap, nil
	}
	if addr, err := netip.ParseAddr(s); err == nil {
		return netip.AddrPortFrom(addr, protocol.DefaultPort), nil
	}
	return netip.AddrPort{}, fmt.Errorf("invalid address %q", s)
}

// defaultDestDir picks a sensible default: ~/Downloads if it exists, else the
// user's home directory, else the current working directory.
func defaultDestDir() string {
	home, err := os.UserHomeDir()
	if err == nil {
		dl := filepath.Join(home, "Downloads")
		if info, err := os.Stat(dl); err == nil && info.IsDir() {
			return dl
		}
		return home
	}
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return "."
}
