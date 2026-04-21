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
    background: #dcfce7;
    color: #166534;
    border-color: #86efac;
  }
  .recv-pill.on:hover {
    background: #bbf7d0;
    border-color: #4ade80;
  }
  .recv-pill.off {
    background: #fee2e2;
    color: #991b1b;
    border-color: #fca5a5;
  }
  .recv-pill.off:hover {
    background: #fecaca;
    border-color: #f87171;
  }
  .dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: currentColor;
  }
</style>
