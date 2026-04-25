// Thin typed wrapper over the Wails-generated bindings in ../wailsjs. Keeps
// the Svelte components free of ambient window['go'] lookups and gives us
// one place to adapt when the Go-side API evolves.

import * as App from '../../wailsjs/go/main/App.js';
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime.js';

export interface Peer {
  address: string;
  port: number;
  signature: string;
  // True when the peer has produced at least one verified v2 HELLO (0x06/0x07).
  // Drives the encrypted-capability badge in the peer list.
  v2Capable?: boolean;
  // Long-term Ed25519 fingerprint, base32(sha256(pubkey)[:10]) formatted as
  // four 4-char groups. Empty for v1-only peers.
  fingerprint?: string;
  // True when the peer's fingerprint is in our TOFU table (pinned). Outbound
  // sends to a paired peer run over Noise XX automatically.
  paired?: boolean;
}

export interface PinnedPeer {
  fingerprint: string;
  ed25519PubHex: string;
  label?: string;
  pinnedAt: string;
}

export interface ReceivePayload {
  name: string;
  size: number;
  path: string;
  text: string;
  total: number;
  bytes: number;
  // "ip:port" of the sender. Empty for events emitted before a conn was
  // attached (none in the current wiring, but keeping the field optional
  // keeps the types honest).
  from?: string;
}

// Parsed fields pulled out of a signature string "User at Host (Platform)".
// Missing parts fall back to the full signature so the UI still has something
// to render.
export interface ParsedSignature {
  user: string;
  host: string;
  platform: string;
  raw: string;
}

export function parseSignature(sig: string): ParsedSignature {
  const raw = sig ?? '';
  const atIdx = raw.indexOf(' at ');
  if (atIdx < 0) return { user: raw, host: '', platform: '', raw };
  const user = raw.slice(0, atIdx);
  const rest = raw.slice(atIdx + 4);
  const m = /^(.*?)\s*\(([^)]+)\)\s*$/.exec(rest);
  if (!m) return { user, host: rest, platform: '', raw };
  return { user, host: m[1], platform: m[2], raw };
}

export function avatarUrl(peer: Peer): string {
  // We can't fetch http://<peer>:4645/dukto/avatar directly: the WebView
  // serves the page over the wails:// scheme and treats raw http:// as
  // mixed-content, so the request is silently blocked. Route through the
  // backend's AssetServer proxy (/avatar/peer/<ip>?port=N) which fetches
  // it server-side and re-serves the bytes same-origin.
  const port = peer.port + 1;
  const bust = encodeURIComponent(peer.signature);
  return `/avatar/peer/${peer.address}.png?port=${port}&v=${bust}`;
}

export function peerKey(peer: Peer): string {
  return `${peer.address}:${peer.port}`;
}

// Backend API ---------------------------------------------------------------

export const peers = () => App.Peers() as Promise<Peer[]>;
export const signature = () => App.Signature();
export const localAddresses = () => App.LocalAddresses() as Promise<string[]>;
export const destDir = () => App.DestDir();
export const pickDestDir = () => App.PickDestDir();
export const pickFilesToSend = () => App.PickFilesToSend() as Promise<string[]>;
export const pickFolderToSend = () => App.PickFolderToSend() as Promise<string>;
export const localAvatarDataURL = () => App.LocalAvatarDataURL() as Promise<string>;
export const pickAndSetCustomAvatar = () => App.PickAndSetCustomAvatar() as Promise<string>;
export const clearCustomAvatar = () => App.ClearCustomAvatar();
export const hasCustomAvatar = () => App.HasCustomAvatar() as Promise<boolean>;
export type ThemeMode = 'system' | 'light' | 'dark';
export const getTheme = () => App.Theme() as Promise<ThemeMode>;
export const setTheme = (mode: ThemeMode) => App.SetTheme(mode);
export const getFingerprint = () => App.Fingerprint() as Promise<string>;

// v2 pinning — TOFU table over the Wails RPCs. PinPeer takes a fingerprint
// already verified via the UDP discovery layer; the backend re-derives the
// pubkey from the address and rejects mismatches.
export const pinPeer = (fingerprint: string, address: string) =>
  App.PinPeer(fingerprint, address) as Promise<PinnedPeer>;
export const unpinPeer = (fingerprint: string) =>
  App.UnpinPeer(fingerprint) as Promise<void>;
export const pinnedPeers = () => App.PinnedPeers() as Promise<PinnedPeer[]>;
export const setDestDir = (dir: string) => App.SetDestDir(dir);
export const buddyName = () => App.BuddyName();
export const setBuddyName = (name: string) => App.SetBuddyName(name);
export const notifications = () => App.Notifications();
export const setNotifications = (enabled: boolean) => App.SetNotifications(enabled);
export const closeToTray = () => App.CloseToTray();
export const setCloseToTray = (enabled: boolean) => App.SetCloseToTray(enabled);
export const copyToClipboard = (text: string) => App.CopyToClipboard(text);
export const sendText = (addrPort: string, text: string) => App.SendText(addrPort, text);
export const sendFiles = (addrPort: string, paths: string[]) => App.SendFiles(addrPort, paths);
export const sendClipboard = (addrPort: string) => App.SendClipboard(addrPort);
export const revealInFolder = (path: string) => App.RevealInFolder(path);
export const openPath = (path: string) => App.OpenPath(path);
export const qrCodeSignature = () => App.QRCodeSignature() as Promise<string>;
export const cancelTransfer = () => App.CancelTransfer();
export const stashPastedImage = (dataUrl: string, extHint: string) =>
  App.StashPastedImage(dataUrl, extHint) as Promise<string>;

// Whitelist / auto-reject ---------------------------------------------------

export const whitelistEnabled = () => App.WhitelistEnabled();
export const setWhitelistEnabled = (on: boolean) => App.SetWhitelistEnabled(on);
export const whitelist = () => App.Whitelist() as Promise<string[]>;
export const addWhitelist = (sig: string) => App.AddWhitelist(sig);
export const removeWhitelist = (sig: string) => App.RemoveWhitelist(sig);
export const rejectedExtensions = () => App.RejectedExtensions() as Promise<string[]>;
export const setRejectedExtensions = (exts: string[]) => App.SetRejectedExtensions(exts);
export const largeFileThresholdMB = () => App.LargeFileThresholdMB() as Promise<number>;
export const setLargeFileThresholdMB = (mb: number) => App.SetLargeFileThresholdMB(mb);

// Receiving master switch --------------------------------------------------

export const receivingEnabled = () => App.ReceivingEnabled() as Promise<boolean>;
export const setReceivingEnabled = (on: boolean) => App.SetReceivingEnabled(on);
export const idleAutoDisableMinutes = () =>
  App.IdleAutoDisableMinutes() as Promise<number>;
export const setIdleAutoDisableMinutes = (mins: number) =>
  App.SetIdleAutoDisableMinutes(mins);

// Block list ----------------------------------------------------------------

export const blockedPeers = () => App.BlockedPeers() as Promise<string[]>;
export const blockPeer = (sig: string) => App.BlockPeer(sig);
export const unblockPeer = (sig: string) => App.UnblockPeer(sig);

// Rate limits ---------------------------------------------------------------

export const tcpAcceptCooldownSeconds = () =>
  App.TCPAcceptCooldownSeconds() as Promise<number>;
export const setTCPAcceptCooldownSeconds = (s: number) =>
  App.SetTCPAcceptCooldownSeconds(s);
export const udpHelloCooldownSeconds = () =>
  App.UDPHelloCooldownSeconds() as Promise<number>;
export const setUDPHelloCooldownSeconds = (s: number) =>
  App.SetUDPHelloCooldownSeconds(s);

// Confirm-unknown gate ------------------------------------------------------

export const confirmUnknownPeers = () =>
  App.ConfirmUnknownPeers() as Promise<boolean>;
export const setConfirmUnknownPeers = (on: boolean) =>
  App.SetConfirmUnknownPeers(on);
export const forgetApprovedPeers = () => App.ForgetApprovedPeers();
export const resolvePendingSession = (id: string, allow: boolean) =>
  App.ResolvePendingSession(id, allow);

// Session caps --------------------------------------------------------------

export const maxFilesPerSession = () =>
  App.MaxFilesPerSession() as Promise<number>;
export const setMaxFilesPerSession = (n: number) => App.SetMaxFilesPerSession(n);
export const maxPathDepth = () => App.MaxPathDepth() as Promise<number>;
export const setMaxPathDepth = (n: number) => App.SetMaxPathDepth(n);
export const minFreeDiskPercent = () =>
  App.MinFreeDiskPercent() as Promise<number>;
export const setMinFreeDiskPercent = (n: number) => App.SetMinFreeDiskPercent(n);

// Interface allow-list ------------------------------------------------------

export interface NetIfaceView {
  name: string;
  active: boolean;
  addresses: string[];
}
export const allowedInterfaces = () =>
  App.AllowedInterfaces() as Promise<string[]>;
export const setAllowedInterfaces = (names: string[]) =>
  App.SetAllowedInterfaces(names);
export const availableInterfaces = () =>
  App.AvailableInterfaces() as unknown as Promise<NetIfaceView[]>;

// Audit log -----------------------------------------------------------------

export interface AuditEntry {
  time: string;
  kind: string;
  reason?: string;
  remote?: string;
  peer?: string;
  detail?: string;
}
export const auditEntries = () =>
  App.AuditEntries() as unknown as Promise<AuditEntry[]>;
export const clearAudit = () => App.ClearAudit();

// Aliases / manual peers ----------------------------------------------------

export const aliases = () => App.Aliases() as Promise<Record<string, string>>;
export const setAlias = (sig: string, alias: string) => App.SetAlias(sig, alias);
export const manualPeers = () => App.ManualPeers() as Promise<string[]>;
export const addManualPeer = (addr: string) => App.AddManualPeer(addr);
export const removeManualPeer = (addr: string) => App.RemoveManualPeer(addr);

// Hash / broadcast / export -------------------------------------------------

export const fileHash = (path: string) => App.FileHash(path) as Promise<string>;
export const sendFilesMulti = (addrs: string[], paths: string[]) =>
  App.SendFilesMulti(addrs, paths);
export const exportHistory = (format: 'csv' | 'json', path: string) =>
  App.ExportHistory(format, path) as Promise<string>;
export const pickExportPath = (format: 'csv' | 'json') =>
  App.PickExportPath(format) as Promise<string>;
export const history = () => App.History() as Promise<HistoryEntry[]>;
export const clearHistory = () => App.ClearHistory();

// HistoryEntry is the persisted shape emitted by history:append and returned
// by history(). `at` is a unix milli timestamp so JSON survives round-trip
// without a Date parsing step.
export interface HistoryEntry {
  kind: 'file' | 'text';
  name?: string;
  path?: string;
  text?: string;
  at: number;
  from?: string;
}

// Preview helpers ----------------------------------------------------------

// Media kinds the webview can render inline. "other" means "offer a link only".
export type MediaKind = 'image' | 'video' | 'audio' | 'other';

const imageExts = new Set(['png', 'jpg', 'jpeg', 'gif', 'webp', 'bmp', 'svg', 'avif', 'ico']);
const videoExts = new Set(['mp4', 'webm', 'mov', 'm4v', 'ogv']);
const audioExts = new Set(['mp3', 'wav', 'ogg', 'oga', 'flac', 'm4a', 'aac', 'opus']);

export function mediaKindFromName(name: string): MediaKind {
  const i = name.lastIndexOf('.');
  if (i < 0) return 'other';
  const ext = name.slice(i + 1).toLowerCase();
  if (imageExts.has(ext)) return 'image';
  if (videoExts.has(ext)) return 'video';
  if (audioExts.has(ext)) return 'audio';
  return 'other';
}

// fileUrl points the webview at the Go-side fall-through HTTP handler (see
// app.serveFile). Relative URL so it stays on the same origin as the SPA,
// which is what Wails' AssetServer expects.
export function fileUrl(path: string): string {
  return `/files?path=${encodeURIComponent(path)}`;
}

// Event wrappers ------------------------------------------------------------

export const onPeerFound = (cb: (p: Peer) => void) => on('peer:found', cb);
export const onPeerGone = (cb: (p: Peer) => void) => on('peer:gone', cb);
export const onReceiveStart = (cb: (p: ReceivePayload) => void) => on('receive:start', cb);
export const onReceiveFile = (cb: (p: ReceivePayload) => void) => on('receive:file', cb);
export const onReceiveText = (cb: (p: ReceivePayload) => void) => on('receive:text', cb);
export const onReceiveDone = (cb: (p: ReceivePayload) => void) => on('receive:done', cb);
export const onReceiveError = (cb: (msg: string) => void) => on('receive:error', cb);
export const onSendError = (cb: (msg: string) => void) => on('send:error', cb);
export interface FileDropPayload { paths: string[]; x: number; y: number }
export const onFileDrop = (cb: (p: FileDropPayload) => void) => on('file:drop', cb);

export interface ProgressPayload {
  bytes: number;
  total: number;
}
export interface SendLifecyclePayload {
  peer: string;
  total: number;
  count?: number;
}
export const onReceiveProgress = (cb: (p: ProgressPayload) => void) =>
  on('receive:progress', cb);
export const onSendStart = (cb: (p: SendLifecyclePayload) => void) => on('send:start', cb);
export const onSendProgress = (cb: (p: ProgressPayload) => void) => on('send:progress', cb);
export const onSendDone = (cb: (p: SendLifecyclePayload) => void) => on('send:done', cb);
export const onHistoryAppend = (cb: (p: HistoryEntry) => void) => on('history:append', cb);

export interface ReceivingChangedPayload {
  enabled: boolean;
  // "user" when flipped via SetReceivingEnabled, "idle" when the Go-side
  // inactivity ticker auto-disabled it. UI uses this to decide whether to
  // show a toast/notification.
  reason: 'user' | 'idle';
}
export const onReceivingChanged = (cb: (p: ReceivingChangedPayload) => void) =>
  on('receiving:changed', cb);

export interface ElementRejectedPayload {
  name: string;
  reason: string;
  from?: string;
}
export const onElementRejected = (cb: (p: ElementRejectedPayload) => void) =>
  on('receive:rejected', cb);

export interface PendingSessionPayload {
  id: string;
  remote: string;
  signature: string;
  timeout: number;
}
export const onPendingSession = (cb: (p: PendingSessionPayload) => void) =>
  on('session:pending', cb);

function on<T>(event: string, cb: (p: T) => void): () => void {
  EventsOn(event, cb as (...args: unknown[]) => void);
  return () => EventsOff(event);
}
