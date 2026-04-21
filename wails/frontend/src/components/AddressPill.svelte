<script lang="ts">
  // Header-level info pill that shows the first local IP and a "+N" counter
  // for additional addresses, with a hover/focus popover listing the rest.
  // Lives next to the ⚙ icon so the user can confirm where the daemon is
  // reachable without opening Settings.
  export let addresses: string[] = [];
  let open = false;
</script>

{#if addresses.length > 0}
  <!-- svelte-ignore a11y-no-static-element-interactions -->
  <div
    class="addr-pill"
    on:mouseenter={() => (open = true)}
    on:mouseleave={() => (open = false)}
    on:focusin={() => (open = true)}
    on:focusout={() => (open = false)}
  >
    <button
      type="button"
      class="addr-pill-btn"
      aria-haspopup="true"
      aria-expanded={open}
      title="Listening addresses"
      on:click={() => (open = !open)}
    >
      <span class="addr-pill-icon" aria-hidden="true">🌐</span>
      <span class="addr-pill-label">{addresses[0]}{addresses.length > 1 ? ` +${addresses.length - 1}` : ''}</span>
    </button>
    {#if open}
      <div class="addr-pop" role="dialog" aria-label="Listening on">
        <div class="addr-pop-title">Listening on</div>
        <ul>
          {#each addresses as a}
            <li><code>{a}</code></li>
          {/each}
        </ul>
      </div>
    {/if}
  </div>
{/if}

<style>
  .addr-pill {
    position: relative;
  }
  .addr-pill-btn {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 4px 10px;
    background: #f1f5f9;
    color: #334155;
    border: 1px solid #cbd5e1;
    border-radius: 999px;
    font-size: 0.8rem;
    cursor: pointer;
  }
  .addr-pill-btn:hover {
    background: #e2e8f0;
    border-color: #94a3b8;
  }
  .addr-pill-icon {
    font-size: 0.9rem;
    line-height: 1;
  }
  .addr-pill-label {
    font-variant-numeric: tabular-nums;
  }
  .addr-pop {
    position: absolute;
    top: calc(100% + 6px);
    right: 0;
    min-width: 200px;
    background: #fff;
    border: 1px solid #cbd5e1;
    border-radius: 6px;
    box-shadow: 0 8px 24px rgba(15, 23, 42, 0.15);
    padding: 10px 12px;
    z-index: 30;
  }
  .addr-pop-title {
    font-size: 0.7rem;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: #334155;
    margin-bottom: 4px;
  }
  .addr-pop ul {
    list-style: none;
    padding: 0;
    margin: 0;
  }
  .addr-pop li {
    font-size: 0.85rem;
    padding: 2px 0;
    color: #0f172a;
  }
  .addr-pop code {
    color: #0f172a;
  }
</style>
