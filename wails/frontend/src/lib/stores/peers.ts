import { derived, writable } from 'svelte/store';
import { peerKey, peers as fetchPeers, type Peer } from '../dukto';
import { now } from './now';

// Keyed by "ip:port" so repeated HELLO bursts replace rather than stack.
export const peersByKey = writable<Map<string, Peer>>(new Map());
export const lastSeen = writable<Map<string, number>>(new Map());

// `peerList` is an array projection kept in sync with the map so components
// can iterate without recreating the array on every render.
export const peerList = derived(peersByKey, ($m) => Array.from($m.values()));

// Sort stable + idle-aware: active peers first, most-recently-seen at the
// top. Reads `now` so the ordering refreshes as peers go idle.
const idleThresholdMs = 5 * 60 * 1000;
export const sortedPeers = derived(
  [peerList, lastSeen, now],
  ([$peers, $seen, $now]) => {
    const dup = $peers.slice();
    dup.sort((a, b) => {
      const ta = $seen.get(peerKey(a));
      const tb = $seen.get(peerKey(b));
      const ia = ta === undefined ? idleThresholdMs : $now - ta;
      const ib = tb === undefined ? idleThresholdMs : $now - tb;
      return ia - ib;
    });
    return dup;
  },
);

export const selectedKey = writable<string | null>(null);
export const broadcastMode = writable(false);
export const broadcastSelected = writable<Set<string>>(new Set());

export function upsertPeer(p: Peer) {
  const k = peerKey(p);
  const ts = Date.now();
  peersByKey.update((m) => {
    m.set(k, p);
    return new Map(m);
  });
  lastSeen.update((m) => {
    m.set(k, ts);
    return new Map(m);
  });
}

// refreshPeers re-fetches the full peer list from the backend and replaces
// the per-key entries while leaving lastSeen alone. Called after a pair
// toggle so the UI's paired flag flips immediately rather than waiting on
// the next HELLO tick.
export async function refreshPeers() {
  try {
    const list = await fetchPeers();
    peersByKey.update((m) => {
      for (const p of list) {
        m.set(peerKey(p), p);
      }
      return new Map(m);
    });
  } catch (err) {
    console.warn('refreshPeers failed', err);
  }
}

export function removePeer(p: Peer) {
  const k = peerKey(p);
  peersByKey.update((m) => {
    m.delete(k);
    return new Map(m);
  });
  lastSeen.update((m) => {
    m.delete(k);
    return new Map(m);
  });
  selectedKey.update((cur) => (cur === k ? null : cur));
}
