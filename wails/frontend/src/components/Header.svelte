<script lang="ts">
  import { parseSignature } from '../lib/dukto';
  import AddressPill from './AddressPill.svelte';
  import ReceivingPill from './ReceivingPill.svelte';

  // Top strip: who-you-are-to-other-peers on the left, the address pill +
  // settings gear on the right. The parent owns both the signature and the
  // addresses; we just render.
  export let signature: string = '';
  export let addresses: string[] = [];
  export let receivingEnabled: boolean = true;
  // Same-origin URL of our own avatar (proxied via the Wails AssetServer).
  // Cache-busted by the parent on changes so the <img> reloads.
  export let avatarUrl: string = '';
  export let onToggleReceiving: () => void = () => {};
  export let onOpenSettings: () => void = () => {};

  $: parsed = parseSignature(signature);
</script>

<header>
  <div class="me">
    {#if avatarUrl}
      <img class="avatar" src={avatarUrl} alt="Your avatar" />
    {/if}
    <div class="me-text">
      <strong>{parsed.user || '…'}</strong>
      <span class="host">{parsed.host} · {parsed.platform}</span>
    </div>
  </div>
  <div class="header-actions">
    <ReceivingPill enabled={receivingEnabled} onToggle={onToggleReceiving} />
    <AddressPill {addresses} />
    <button class="icon" title="Settings" on:click={onOpenSettings}>⚙</button>
  </div>
</header>

<style>
  header {
    grid-area: head;
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 8px 12px;
    background: var(--accent); /* Dukto brand green */
    color: var(--header-text);
    border-radius: 6px;
  }
  .me {
    display: flex;
    align-items: center;
    gap: 12px;
    min-width: 0;
  }
  .avatar {
    width: 36px;
    height: 36px;
    border-radius: 8px;
    object-fit: cover;
    flex-shrink: 0;
  }
  .me-text {
    display: flex;
    align-items: baseline;
    gap: 8px;
    min-width: 0;
  }
  .me-text strong {
    font-size: 1.1rem;
  }
  .host {
    opacity: 0.7;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .header-actions {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .icon {
    background: none;
    border: 0;
    color: inherit;
    font-size: 1.4rem;
    cursor: pointer;
  }
</style>
