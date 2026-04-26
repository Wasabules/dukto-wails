package main

import (
	"context"
	"fmt"
	"net/netip"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"dukto/internal/protocol"
	"dukto/internal/transfer"
)

// SendText sends a text snippet to peer "ip:port".
func (a *App) SendText(addrPort, text string) error {
	peer, err := parseAddrPort(addrPort)
	if err != nil {
		return err
	}
	srcs, hdr := transfer.TextSource(text)
	go a.sendWithProgress(peer, srcs, hdr)
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
	go a.sendWithProgress(peer, srcs, hdr)
	return nil
}

// SendFilesMulti fans a set of files out to multiple peers. Each peer gets
// its own send goroutine via sendWithProgress so one slow peer doesn't
// bottleneck the rest. Errors per-peer surface via the existing send:error
// event stream, identified by peer in its payload.
func (a *App) SendFilesMulti(addrPorts []string, paths []string) error {
	if len(addrPorts) == 0 {
		return fmt.Errorf("no peers")
	}
	srcs, hdr, err := transfer.Sources(paths)
	if err != nil {
		return err
	}
	for _, ap := range addrPorts {
		peer, err := parseAddrPort(ap)
		if err != nil {
			runtime.EventsEmit(a.ctx, evtSendError, fmt.Sprintf("%s: %v", ap, err))
			continue
		}
		go a.sendWithProgress(peer, srcs, hdr)
	}
	return nil
}

// SendClipboard grabs the current clipboard text and sends it to a peer.
// Returns an error (and emits nothing) if the clipboard is empty so the
// frontend can show a precise toast.
func (a *App) SendClipboard(addrPort string) error {
	text, err := runtime.ClipboardGetText(a.ctx)
	if err != nil {
		return err
	}
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("clipboard is empty")
	}
	return a.SendText(addrPort, text)
}

// sendWithProgress dials peer and streams sources, emitting send:start,
// send:progress, and send:done events so the UI can drive a progress bar.
// Runs on its own goroutine; errors become send:error events. The per-send
// cancel fn is published to a.sendCancel so CancelTransfer can abort it.
func (a *App) sendWithProgress(peer netip.AddrPort, srcs []transfer.Source, hdr protocol.SessionHeader) {
	ctx, cancel := context.WithCancel(a.ctx)
	a.cancelMu.Lock()
	a.sendCancel = cancel
	a.cancelMu.Unlock()
	defer func() {
		cancel()
		a.cancelMu.Lock()
		a.sendCancel = nil
		a.cancelMu.Unlock()
	}()

	runtime.EventsEmit(a.ctx, evtSendStart, map[string]any{
		"peer":  peer.String(),
		"total": hdr.TotalSize,
		"count": hdr.TotalElements,
	})
	// If the destination's Ed25519 fingerprint is in our pinned-peers
	// table, install the v2 Upgrade hook so the dialled connection runs
	// Noise XX before any session bytes hit the wire. Cleartext fallback
	// happens implicitly when the peer isn't pinned.
	pinnedFP := a.fingerprintForAddress(peer.Addr().String())
	pinned := pinnedFP != "" && a.IsPeerPinned(pinnedFP)
	if !pinned && a.settings.Values().RefuseCleartext {
		runtime.EventsEmit(a.ctx, evtSendError,
			fmt.Sprintf("refuseCleartext: peer %s is not paired", peer))
		return
	}
	sender := &transfer.Sender{
		OnProgress: func(done, total int64) {
			runtime.EventsEmit(a.ctx, evtSendProgress, map[string]any{
				"bytes": done,
				"total": total,
			})
		},
	}
	if pinned {
		sender.Upgrade = a.senderUpgrade(pinnedFP)
	}
	if err := sender.Dial(ctx, peer, srcs, hdr); err != nil {
		runtime.EventsEmit(a.ctx, evtSendError, err.Error())
		return
	}
	runtime.EventsEmit(a.ctx, evtSendDone, map[string]any{
		"peer":  peer.String(),
		"total": hdr.TotalSize,
	})
}

// CancelTransfer aborts any in-flight send and/or receive session. It's safe
// to call even if nothing is running. Cancelling the send closes our socket
// mid-stream; the peer will see a truncated transfer. Cancelling a receive
// closes the inbound socket, which the Qt sender reports as a write failure.
func (a *App) CancelTransfer() {
	a.cancelMu.Lock()
	sc, rc := a.sendCancel, a.receiveCancel
	a.cancelMu.Unlock()
	if sc != nil {
		sc()
	}
	if rc != nil {
		rc()
	}
}
