package main

// Wails event names consumed by the Svelte frontend. The list is the
// contract between Go emitters and the TypeScript wrappers in
// src/lib/dukto.ts; keep them aligned when adding new events.
const (
	evtPeerFound       = "peer:found"
	evtPeerGone        = "peer:gone"
	evtReceiveStart    = "receive:start"
	evtReceiveDir      = "receive:dir"
	evtReceiveFile     = "receive:file"
	evtReceiveText     = "receive:text"
	evtReceiveDone     = "receive:done"
	evtReceiveError    = "receive:error"
	evtReceiveProgress = "receive:progress"
	evtSendError       = "send:error"
	evtSendStart       = "send:start"
	evtSendProgress    = "send:progress"
	evtSendDone        = "send:done"
	evtFileDrop         = "file:drop"
	evtHistoryAppend    = "history:append"
	evtReceivingChanged = "receiving:changed"
	evtElementRejected  = "receive:rejected"
	evtPendingSession   = "session:pending"
	evtAuditAppended    = "audit:appended"
)

// PeerView is the Wails-facing projection of a discovered peer. Unlike
// discovery.Peer it carries string fields only, so Wails can marshal it.
type PeerView struct {
	Address   string `json:"address"`
	Port      uint16 `json:"port"`
	Signature string `json:"signature"`
}
