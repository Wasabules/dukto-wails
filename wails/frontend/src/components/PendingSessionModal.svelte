<script lang="ts">
  import { onDestroy } from 'svelte';
  import { parseSignature, type PendingSessionPayload } from '../lib/dukto';

  export let request: PendingSessionPayload;
  export let onAllow: (id: string) => void = () => {};
  export let onDeny: (id: string) => void = () => {};

  // Countdown mirrors the Go-side sessionConfirmTimeout. The backend is the
  // source of truth — if we drift, the real request will be resolved by the
  // server's select{} branch regardless of what this label reads.
  let remaining = request.timeout;
  const interval = setInterval(() => {
    remaining = Math.max(0, remaining - 1);
    if (remaining === 0) clearInterval(interval);
  }, 1000);
  onDestroy(() => clearInterval(interval));

  $: parsed = parseSignature(request.signature);
</script>

<!-- svelte-ignore a11y-click-events-have-key-events a11y-no-static-element-interactions -->
<div class="pending-backdrop" role="dialog" aria-modal="true" aria-label="Incoming transfer">
  <div class="pending-modal">
    <h2>Incoming transfer</h2>
    <p class="who">
      <strong>{parsed.user || request.signature || 'Unknown peer'}</strong>
      {#if parsed.host}at {parsed.host}{/if}
    </p>
    <p class="meta">{request.remote}</p>
    <p class="note">This peer is connecting for the first time. Allow the transfer?</p>
    <p class="timer">Auto-rejects in {remaining}s</p>
    <div class="actions">
      <button class="deny" type="button" on:click={() => onDeny(request.id)}>Reject</button>
      <button class="allow" type="button" on:click={() => onAllow(request.id)}>Allow</button>
    </div>
  </div>
</div>

<style>
  .pending-backdrop {
    position: fixed;
    inset: 0;
    background: rgba(15, 23, 42, 0.55);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 60;
    padding: 24px;
  }
  .pending-modal {
    background: #fff;
    border-radius: 8px;
    box-shadow: 0 20px 50px rgba(15, 23, 42, 0.3);
    width: min(440px, 100%);
    padding: 20px 24px;
    font-size: 0.95rem;
  }
  h2 {
    margin: 0 0 8px;
    font-size: 1.1rem;
  }
  .who {
    margin: 4px 0;
  }
  .meta {
    color: #64748b;
    font-size: 0.85rem;
    margin: 0 0 12px;
  }
  .note {
    margin: 0 0 8px;
  }
  .timer {
    color: #94a3b8;
    font-size: 0.85rem;
    margin: 0 0 16px;
  }
  .actions {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
  }
  button {
    padding: 8px 14px;
    font: inherit;
    border-radius: 4px;
    border: 1px solid transparent;
    cursor: pointer;
  }
  .allow {
    background: #2563eb;
    border-color: #2563eb;
    color: #fff;
  }
  .deny {
    background: #fff;
    border-color: #cbd5e1;
    color: #0f172a;
  }
</style>
