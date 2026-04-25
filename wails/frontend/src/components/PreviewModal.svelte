<script context="module" lang="ts">
  import type { MediaKind } from '../lib/dukto';
  export interface PreviewItem {
    name: string;
    path: string;
    media: MediaKind;
  }
</script>

<script lang="ts">
  import { fileUrl } from '../lib/dukto';

  export let item: PreviewItem | null = null;
  export let onClose: () => void = () => {};
  export let onOpenExternal: (path: string) => void = () => {};
  export let onReveal: (path: string) => void = () => {};
</script>

{#if item}
  {@const pv = item}
  <!-- svelte-ignore a11y-click-events-have-key-events a11y-no-static-element-interactions -->
  <div
    class="preview-backdrop"
    on:click|self={onClose}
    role="dialog"
    aria-modal="true"
    aria-label="File preview"
  >
    <div class="preview-box">
      <div class="preview-head">
        <span class="preview-name" title={pv.path}>{pv.name}</span>
        <div class="preview-head-actions">
          <button class="mini ghost" type="button" on:click={() => onOpenExternal(pv.path)}>Open externally</button>
          <button class="mini ghost" type="button" on:click={() => onReveal(pv.path)}>Show in folder</button>
          <button class="mini ghost" type="button" on:click={onClose} title="Close (Esc)">✕</button>
        </div>
      </div>
      <div class="preview-body">
        {#if pv.media === 'image'}
          <img src={fileUrl(pv.path)} alt={pv.name} />
        {:else if pv.media === 'video'}
          <!-- svelte-ignore a11y-media-has-caption -->
          <video controls autoplay src={fileUrl(pv.path)}></video>
        {:else if pv.media === 'audio'}
          <div class="audio-wrap">
            <span class="thumb-icon-big">🎵</span>
            <audio controls autoplay src={fileUrl(pv.path)}></audio>
          </div>
        {/if}
      </div>
    </div>
  </div>
{/if}

<style>
  .preview-backdrop {
    position: fixed;
    inset: 0;
    background: rgba(15, 23, 42, 0.72);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 50;
    padding: 24px;
  }
  .preview-box {
    background: var(--text-strong);
    border-radius: 8px;
    box-shadow: 0 20px 50px rgba(0, 0, 0, 0.5);
    max-width: min(1200px, 95vw);
    max-height: 95vh;
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }
  .preview-head {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 8px 12px;
    background: var(--text-strong);
    color: var(--panel-bg-2);
    border-bottom: 1px solid var(--text);
  }
  .preview-name {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-weight: 600;
  }
  .preview-head-actions {
    display: flex;
    gap: 6px;
    flex-shrink: 0;
  }
  /* Dark ghost buttons: the global .ghost is blue-on-blue and invisible on
     this slate backdrop, so the preview header gets its own palette. */
  .mini {
    padding: 0 6px;
    font-size: 0.9rem;
    line-height: 1;
    border-radius: 4px;
    cursor: pointer;
  }
  .mini.ghost {
    background: transparent;
    color: var(--panel-border);
    border: 1px solid var(--text);
  }
  .mini.ghost:hover {
    background: var(--text);
    color: var(--panel-bg-2);
    border-color: var(--text-dim);
  }
  .preview-body {
    flex: 1;
    min-height: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 12px;
    background: var(--text-strong);
    overflow: auto;
  }
  .preview-body img,
  .preview-body video {
    max-width: 100%;
    max-height: calc(95vh - 80px);
    object-fit: contain;
    display: block;
  }
  .audio-wrap {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 16px;
    padding: 20px;
  }
  .audio-wrap audio {
    width: min(480px, 80vw);
  }
  .thumb-icon-big {
    font-size: 5rem;
    color: var(--panel-bg-2);
  }
</style>
