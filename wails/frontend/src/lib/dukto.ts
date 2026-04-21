// Thin typed wrapper over the Wails-generated bindings in ../wailsjs. Keeps
// the Svelte components free of ambient window['go'] lookups and gives us
// one place to adapt when the Go-side API evolves.

import * as App from '../../wailsjs/go/main/App.js';
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime.js';

export interface Peer {
  address: string;
  port: number;
  signature: string;
}

export interface ReceivePayload {
  name: string;
  size: number;
  path: string;
  text: string;
  total: number;
  bytes: number;
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
  // The avatar HTTP endpoint lives on the peer's UDP port + 1 by convention.
  // Cache-bust on the signature so a rename is immediately visible.
  const port = peer.port + 1;
  const bust = encodeURIComponent(peer.signature);
  return `http://${peer.address}:${port}/dukto/avatar?v=${bust}`;
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

// Event wrappers ------------------------------------------------------------

export const onPeerFound = (cb: (p: Peer) => void) => on('peer:found', cb);
export const onPeerGone = (cb: (p: Peer) => void) => on('peer:gone', cb);
export const onReceiveStart = (cb: (p: ReceivePayload) => void) => on('receive:start', cb);
export const onReceiveFile = (cb: (p: ReceivePayload) => void) => on('receive:file', cb);
export const onReceiveText = (cb: (p: ReceivePayload) => void) => on('receive:text', cb);
export const onReceiveDone = (cb: (p: ReceivePayload) => void) => on('receive:done', cb);
export const onReceiveError = (cb: (msg: string) => void) => on('receive:error', cb);
export const onSendError = (cb: (msg: string) => void) => on('send:error', cb);
export const onFileDrop = (cb: (paths: string[]) => void) => on('file:drop', cb);

function on<T>(event: string, cb: (p: T) => void): () => void {
  EventsOn(event, cb as (...args: unknown[]) => void);
  return () => EventsOff(event);
}
