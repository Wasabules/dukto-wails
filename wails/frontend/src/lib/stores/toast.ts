import { writable } from 'svelte/store';

// Single-line notification pinned to the bottom of the viewport.
// A new call cancels the previous timer so bursts show only the latest.
export const toast = writable<string | null>(null);

let timer: ReturnType<typeof setTimeout> | null = null;

export function showToast(msg: string, durationMs = 4000) {
  toast.set(msg);
  if (timer) clearTimeout(timer);
  timer = setTimeout(() => toast.set(null), durationMs);
}
