import { writable } from 'svelte/store';

// `receiving` mirrors the Go-side ReceivingEnabled setting. The value is
// pushed in via events.ts when the backend emits `receiving:changed`, and
// App.svelte binds to it so the header pill stays in sync whether the
// change came from the user or the idle auto-disable ticker.
export const receiving = writable(true);
