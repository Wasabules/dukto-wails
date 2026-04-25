<script context="module" lang="ts">
  export interface ProgressState {
    bytes: number;
    total: number;
    label: string;
    startedAt: number;
    startedBytes: number;
  }
</script>

<script lang="ts">
  import { humanBytes, formatEta } from '../lib/format';

  export let sendProgress: ProgressState | null = null;
  export let receiveProgress: ProgressState | null = null;
  // `now` is a ticking wall-clock timestamp owned by the parent so the ETA
  // refreshes on the same cadence as the rest of the UI.
  export let now: number = Date.now();
  export let onCancel: () => void = () => {};

  function stats(p: ProgressState, nowMs: number): string | null {
    const dt = (nowMs - p.startedAt) / 1000;
    const dBytes = p.bytes - p.startedBytes;
    if (dt < 0.25 || dBytes <= 0) return null;
    const rate = dBytes / dt;
    if (rate <= 0) return null;
    const left = Math.max(0, p.total - p.bytes);
    const eta = left / rate;
    return `${humanBytes(rate)}/s · ${formatEta(eta)} left`;
  }
</script>

{#if sendProgress || receiveProgress}
  <div class="progress-stack">
    {#if sendProgress}
      {@const sp = sendProgress}
      {@const ss = stats(sp, now)}
      <div class="progress" role="status">
        <div class="progress-label">
          ↑ {sp.label} <span class="progress-bytes">{humanBytes(sp.bytes)} / {humanBytes(sp.total)}</span>
        </div>
        <div class="progress-track">
          <div class="progress-fill" style="width: {sp.total > 0 ? (sp.bytes / sp.total) * 100 : 0}%"></div>
        </div>
        {#if ss}
          <div class="progress-sub">{ss}</div>
        {/if}
      </div>
    {/if}
    {#if receiveProgress}
      {@const rp = receiveProgress}
      {@const rs = stats(rp, now)}
      <div class="progress" role="status">
        <div class="progress-label">
          ↓ {rp.label} <span class="progress-bytes">{humanBytes(rp.bytes)} / {humanBytes(rp.total)}</span>
        </div>
        <div class="progress-track">
          <div class="progress-fill" style="width: {rp.total > 0 ? (rp.bytes / rp.total) * 100 : 0}%"></div>
        </div>
        {#if rs}
          <div class="progress-sub">{rs}</div>
        {/if}
      </div>
    {/if}
    <button class="cancel-btn" type="button" on:click={onCancel}>
      Cancel (Esc)
    </button>
  </div>
{/if}

<style>
  .progress-stack {
    position: fixed;
    bottom: 48px;
    left: 50%;
    transform: translateX(-50%);
    width: min(420px, 80%);
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .progress {
    background: rgba(15, 23, 42, 0.92);
    color: var(--panel-bg-2);
    padding: 8px 12px;
    border-radius: 6px;
    font-size: 0.82rem;
  }
  .progress-label {
    display: flex;
    justify-content: space-between;
    margin-bottom: 4px;
  }
  .progress-bytes {
    opacity: 0.75;
  }
  .progress-track {
    background: rgba(255, 255, 255, 0.12);
    border-radius: 3px;
    height: 6px;
    overflow: hidden;
  }
  .progress-fill {
    background: var(--progress-bar);
    height: 100%;
    transition: width 120ms ease-out;
  }
  .progress-sub {
    margin-top: 4px;
    font-size: 0.75rem;
    opacity: 0.75;
  }
  .cancel-btn {
    align-self: center;
    background: var(--error);
    border: 1px solid var(--error);
    color: var(--panel-bg);
    padding: 4px 12px;
    font-size: 0.8rem;
    border-radius: 4px;
    cursor: pointer;
  }
  .cancel-btn:hover {
    background: var(--error);
    border-color: var(--error);
  }
</style>
