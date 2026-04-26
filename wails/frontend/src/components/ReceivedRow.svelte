<script context="module" lang="ts">
  import type { MediaKind } from '../lib/dukto';
  export interface ReceivedItem {
    id: number;
    kind: 'file' | 'text';
    name: string;
    path: string;
    text: string;
    at: Date;
    media: MediaKind;
    from: string;
    /** True when the originating session ran over Noise XX. */
    encrypted?: boolean;
  }
</script>

<script lang="ts">
  import { fileUrl } from '../lib/dukto';
  import { fileIcon } from '../lib/format';

  export let item: ReceivedItem;
  export let hash: string | null = null;

  export let onOpenPreview: (r: ReceivedItem) => void = () => {};
  export let onOpenExternal: (path: string) => void = () => {};
  export let onReveal: (path: string) => void = () => {};
  export let onCopyText: (text: string) => void = () => {};
  export let onHash: (path: string) => void = () => {};
</script>

<li>
  {#if item.kind === 'text'}
    <div class="thumb thumb-text" aria-hidden="true">
      <span class="thumb-icon">✉</span>
    </div>
  {:else if item.media === 'image'}
    <button
      type="button"
      class="thumb thumb-media"
      title="Preview {item.name}"
      on:click={() => onOpenPreview(item)}
    >
      <img src={fileUrl(item.path)} alt={item.name} loading="lazy" />
    </button>
  {:else if item.media === 'video'}
    <button
      type="button"
      class="thumb thumb-media"
      title="Preview {item.name}"
      on:click={() => onOpenPreview(item)}
    >
      <!-- svelte-ignore a11y-media-has-caption -->
      <video preload="metadata" src="{fileUrl(item.path)}#t=0.1" muted></video>
      <span class="thumb-overlay">▶</span>
    </button>
  {:else if item.media === 'audio'}
    <button
      type="button"
      class="thumb thumb-file"
      title="Preview {item.name}"
      on:click={() => onOpenPreview(item)}
    >
      <span class="thumb-icon">{fileIcon(item.name)}</span>
    </button>
  {:else}
    <button
      type="button"
      class="thumb thumb-file"
      title="Open {item.name}"
      on:click={() => onOpenExternal(item.path)}
    >
      <span class="thumb-icon">{fileIcon(item.name)}</span>
    </button>
  {/if}
  <div class="meta">
    <div class="row-head">
      <span class="badge">{item.kind}</span>
      {#if item.encrypted}
        <span class="enc-badge" title="Encrypted (Noise XX)">🔒</span>
      {/if}
      <span class="rname" title={item.name}>{item.name}</span>
      <time>{item.at.toLocaleTimeString()}</time>
    </div>
    {#if item.kind === 'text'}
      <blockquote>{item.text}</blockquote>
      <div class="actions">
        <button class="mini ghost" type="button" on:click={() => onCopyText(item.text)}>Copy</button>
      </div>
    {:else}
      <code title={item.path}>{item.path}</code>
      {#if item.media === 'audio'}
        <audio class="inline-audio" controls preload="metadata" src={fileUrl(item.path)}></audio>
      {/if}
      <div class="actions">
        <button class="mini ghost" type="button" on:click={() => onOpenExternal(item.path)}>Open</button>
        <button class="mini ghost" type="button" on:click={() => onReveal(item.path)}>Show in folder</button>
        <button
          class="mini ghost"
          type="button"
          title={hash ?? 'Compute SHA-256 and copy to clipboard'}
          on:click={() => onHash(item.path)}
        >
          {hash ? '✓ Hash' : 'Hash'}
        </button>
      </div>
      {#if hash}
        <code class="hash" title={hash}>SHA-256: {hash}</code>
      {/if}
    {/if}
  </div>
</li>

<style>
  li {
    padding: 8px 0;
    border-bottom: 1px solid var(--code-bg);
    display: grid;
    grid-template-columns: 88px 1fr;
    gap: 12px;
    align-items: start;
    list-style: none;
  }
  .thumb {
    width: 88px;
    height: 88px;
    border-radius: 6px;
    background: var(--code-bg);
    overflow: hidden;
    display: flex;
    align-items: center;
    justify-content: center;
    position: relative;
    flex-shrink: 0;
    padding: 0;
    border: 1px solid var(--panel-border);
  }
  button.thumb {
    cursor: pointer;
  }
  button.thumb:hover {
    border-color: var(--text-faint);
  }
  .thumb img,
  .thumb video {
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
  }
  .thumb-icon {
    font-size: 2rem;
    line-height: 1;
    opacity: 0.85;
  }
  .thumb-overlay {
    position: absolute;
    inset: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    color: var(--panel-bg);
    font-size: 1.6rem;
    text-shadow: 0 1px 3px rgba(0, 0, 0, 0.6);
    background: rgba(0, 0, 0, 0.15);
    pointer-events: none;
  }
  .meta {
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .row-head {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .rname {
    font-weight: 600;
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  blockquote {
    margin: 0;
    padding: 4px 8px;
    border-left: 3px solid var(--input-border);
    color: var(--text);
    white-space: pre-wrap;
    overflow: hidden;
    display: -webkit-box;
    -webkit-line-clamp: 3;
    -webkit-box-orient: vertical;
    max-height: 4.5em;
  }
  code {
    font-size: 0.8rem;
    color: var(--text);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    display: block;
  }
  .inline-audio {
    width: 100%;
    max-width: 360px;
    height: 32px;
  }
  .actions {
    display: flex;
    gap: 6px;
    margin-top: 2px;
    flex-wrap: wrap;
  }
  .badge {
    font-size: 0.7rem;
    padding: 2px 6px;
    border-radius: 10px;
    background: var(--panel-border);
    text-transform: uppercase;
  }
  time {
    font-size: 0.75rem;
    color: var(--text-faint);
  }
  .hash {
    font-size: 0.7rem;
    color: var(--text-dim);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    display: block;
  }
  .mini {
    padding: 0 6px;
    font-size: 0.9rem;
    line-height: 1;
    border-radius: 4px;
    cursor: pointer;
    border: 1px solid var(--accent);
    background: var(--accent);
    color: var(--accent-on);
  }
  .mini.ghost {
    background: transparent;
    color: var(--accent);
  }
</style>
