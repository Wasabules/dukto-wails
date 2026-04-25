<script lang="ts">
  // Master switch for incoming transfers, shown in the header next to the
  // address pill. Green = accepting, red = blocked. The state is mirrored
  // from the Go side — flipping here calls SetReceivingEnabled and trusts
  // the receiving:changed event to roll the visual back if the backend
  // refuses.
  export let enabled: boolean = true;
  export let onToggle: () => void = () => {};
</script>

<button
  type="button"
  class="recv-pill"
  class:on={enabled}
  class:off={!enabled}
  title={enabled ? 'Receiving enabled — click to disable' : 'Receiving disabled — click to enable'}
  aria-pressed={enabled}
  on:click={onToggle}
>
  <span class="dot" aria-hidden="true"></span>
  <span class="label">{enabled ? 'Receiving: ON' : 'Receiving: OFF'}</span>
</button>

<style>
  .recv-pill {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 4px 10px;
    border-radius: 999px;
    font-size: 0.8rem;
    cursor: pointer;
    border: 1px solid transparent;
    font-variant-numeric: tabular-nums;
  }
  .recv-pill.on {
    background: var(--accent-soft);
    color: var(--accent-strong);
    border-color: var(--accent-soft-border);
  }
  .recv-pill.on:hover {
    background: var(--accent-soft);
    border-color: var(--accent-soft-border);
  }
  .recv-pill.off {
    background: var(--error-bg);
    color: var(--error-strong);
    border-color: var(--error-border);
  }
  .recv-pill.off:hover {
    background: var(--error-border);
    border-color: var(--error);
  }
  .dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: currentColor;
  }
</style>
