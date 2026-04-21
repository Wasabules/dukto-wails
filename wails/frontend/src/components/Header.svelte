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
  export let onToggleReceiving: () => void = () => {};
  export let onOpenSettings: () => void = () => {};

  $: parsed = parseSignature(signature);
</script>

<header>
  <div class="me">
    <strong>{parsed.user || '…'}</strong>
    <span class="host">{parsed.host} · {parsed.platform}</span>
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
    background: #1f2937;
    color: #e5e7eb;
    border-radius: 6px;
  }
  .me strong {
    font-size: 1.1rem;
  }
  .host {
    margin-left: 8px;
    opacity: 0.7;
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
