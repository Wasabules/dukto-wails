import { derived, writable } from 'svelte/store';
import type { ProgressState } from '../../components/ProgressStack.svelte';
import { retuneNowClock } from './now';

export const sendProgress = writable<ProgressState | null>(null);
export const receiveProgress = writable<ProgressState | null>(null);

// Per-peer send progress for the mini bars under peer cards.
export const sendByPeer = writable<Map<string, { bytes: number; total: number }>>(new Map());

// `anyActive` is derived from the two progress stores and drives the
// global now-clock tick rate.
const anyActive = derived(
  [sendProgress, receiveProgress],
  ([$s, $r]) => $s !== null || $r !== null,
);
anyActive.subscribe((active) => retuneNowClock(active));
