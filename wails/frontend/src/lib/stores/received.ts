import { writable } from 'svelte/store';
import { mediaKindFromName, type HistoryEntry } from '../dukto';
import type { ReceivedItem } from '../../components/ReceivedRow.svelte';

// Capped history of incoming transfers, newest first. 50 is the backend cap.
export const received = writable<ReceivedItem[]>([]);
export const hashByPath = writable<Map<string, string>>(new Map());

let nextId = 1;

export function pushHistory(h: HistoryEntry, opts: { prepend?: boolean } = {}) {
  const item: ReceivedItem = {
    id: nextId++,
    kind: h.kind,
    name: h.name ?? '',
    path: h.path ?? '',
    text: h.text ?? '',
    at: new Date(h.at),
    media: h.kind === 'file' ? mediaKindFromName(h.name ?? '') : 'other',
    from: h.from ?? '',
    encrypted: !!h.encrypted,
  };
  received.update((cur) => (opts.prepend ? [item, ...cur] : [...cur, item]).slice(0, 50));
}

export function clearReceived() {
  received.set([]);
}

export function cacheHash(path: string, hash: string) {
  hashByPath.update((m) => {
    m.set(path, hash);
    return new Map(m);
  });
}
