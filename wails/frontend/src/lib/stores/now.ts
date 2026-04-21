import { writable } from 'svelte/store';

// `now` is a ticking wall-clock timestamp. The idle greyout, the peer-sort
// order, and the progress ETA all read it so they refresh on the same
// cadence — 500ms when a transfer is live, 5s otherwise.
export const now = writable(Date.now());

let timer: ReturnType<typeof setInterval> | null = null;
let fast = false;

export function retuneNowClock(active: boolean) {
  if (timer !== null && active === fast) return;
  if (timer !== null) clearInterval(timer);
  fast = active;
  timer = setInterval(() => now.set(Date.now()), active ? 500 : 5000);
}

export function stopNowClock() {
  if (timer !== null) {
    clearInterval(timer);
    timer = null;
  }
}
