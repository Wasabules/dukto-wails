import { writable } from 'svelte/store';
import type { PendingSessionPayload } from '../dukto';

// pendingSession holds the currently-awaiting session-confirm request, if
// any. The backend blocks on the Go side until ResolvePendingSession fires,
// so this store is a 0-or-1 element queue — a second request arriving while
// one is already pending overwrites it (the first call will time out
// server-side after 60 s, which is the desired fallback).
export const pendingSession = writable<PendingSessionPayload | null>(null);
