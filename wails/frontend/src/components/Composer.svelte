<script lang="ts">
  import { parseSignature, type Peer } from '../lib/dukto';
  import { basename, fileIcon } from '../lib/format';

  // The middle-right panel. Three modes:
  //   - broadcast: file queue + fan-out button (targets multi-picked peers)
  //   - selected: text compose + file queue + both send buttons
  //   - empty: "pick a peer" message
  //
  // State (composeText, draggedFiles, selected peer, broadcast set) is owned
  // upstream so the Wails event handlers in App.svelte can mutate it from
  // non-UI code paths (e.g. a drop event → queue files).
  export let selectedPeer: Peer | null = null;
  export let broadcastMode = false;
  export let broadcastSelectedCount = 0;
  export let draggedFiles: string[] = [];
  export let composeText = '';
  export let aliases: Record<string, string> = {};

  export let onSendText: () => void = () => {};
  export let onSendClipboard: () => void = () => {};
  export let onSendFiles: () => void = () => {};
  export let onSendFilesBroadcast: () => void = () => {};
  export let onRemoveQueued: (path: string) => void = () => {};
  export let onClearQueued: () => void = () => {};
  export let onComposeTextChange: (text: string) => void = () => {};
  export let onComposePaste: (event: ClipboardEvent) => void = () => {};

  function peerLabel(p: Peer): string {
    const alias = aliases[p.signature];
    if (alias) return alias;
    const ps = parseSignature(p.signature);
    return ps.user || p.address;
  }

  function handleTextInput(e: Event) {
    onComposeTextChange((e.currentTarget as HTMLTextAreaElement).value);
  }
</script>

<section class="compose">
  <h2>Send</h2>
  {#if broadcastMode}
    <p class="target">
      Broadcast to <strong>{broadcastSelectedCount}</strong> peer{broadcastSelectedCount === 1 ? '' : 's'}
    </p>
    <div class="dropzone" class:has-items={draggedFiles.length > 0}>
      {#if draggedFiles.length === 0}
        <p>Drop files here, then click Send to fan out.</p>
      {:else}
        <ul class="queued">
          {#each draggedFiles as path (path)}
            <li>
              <span class="file-icon">{fileIcon(basename(path))}</span>
              <code title={path}>{basename(path)}</code>
              <button type="button" class="mini" on:click={() => onRemoveQueued(path)}>×</button>
            </li>
          {/each}
        </ul>
      {/if}
    </div>
    <div class="row">
      <button
        on:click={onSendFilesBroadcast}
        disabled={broadcastSelectedCount === 0 || draggedFiles.length === 0}
      >
        Fan-out {draggedFiles.length || ''} item(s)
      </button>
      {#if draggedFiles.length > 0}
        <button class="ghost" type="button" on:click={onClearQueued}>Clear</button>
      {/if}
    </div>
  {:else if selectedPeer}
    <p class="target">
      To <strong>{peerLabel(selectedPeer)}</strong>
    </p>
    <label>
      Text snippet
      <textarea
        value={composeText}
        rows="4"
        placeholder="Type a message, or paste an image…"
        on:input={handleTextInput}
        on:paste={onComposePaste}
      ></textarea>
    </label>
    <div class="row">
      <button on:click={onSendText} disabled={!composeText.trim()}>Send text</button>
      <button class="ghost" type="button" on:click={onSendClipboard}>Send clipboard</button>
    </div>

    <div class="dropzone" class:has-items={draggedFiles.length > 0}>
      {#if draggedFiles.length === 0}
        <p>Drop files or folders here to queue them — or drop directly onto a peer to send immediately.</p>
      {:else}
        <ul class="queued">
          {#each draggedFiles as path (path)}
            <li>
              <span class="file-icon">{fileIcon(basename(path))}</span>
              <code title={path}>{basename(path)}</code>
              <button type="button" class="mini" on:click={() => onRemoveQueued(path)}>×</button>
            </li>
          {/each}
        </ul>
      {/if}
    </div>
    <div class="row">
      <button on:click={onSendFiles} disabled={draggedFiles.length === 0}>Send {draggedFiles.length || ''} item(s)</button>
      {#if draggedFiles.length > 0}
        <button class="ghost" type="button" on:click={onClearQueued}>Clear</button>
      {/if}
    </div>
  {:else}
    <p class="empty">Pick a peer on the left to start a transfer.</p>
  {/if}
</section>

<style>
  section.compose {
    grid-area: compose;
    background: #ffffff;
    border: 1px solid #e2e8f0;
    border-radius: 6px;
    padding: 10px 14px;
    overflow: auto;
  }
  h2 {
    margin: 0 0 8px;
    font-size: 0.9rem;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: #475569;
  }
  .empty {
    color: #94a3b8;
    font-size: 0.9rem;
  }
  label {
    display: block;
    font-size: 0.85rem;
    color: #334155;
    margin: 8px 0;
  }
  textarea {
    width: 100%;
    box-sizing: border-box;
    font: inherit;
    padding: 6px 8px;
    border: 1px solid #cbd5e1;
    border-radius: 4px;
  }
  button {
    padding: 6px 12px;
    font: inherit;
    border: 1px solid #2563eb;
    background: #2563eb;
    color: #fff;
    border-radius: 4px;
    cursor: pointer;
  }
  button:disabled {
    opacity: 0.4;
    cursor: not-allowed;
  }
  .row {
    display: flex;
    gap: 8px;
    align-items: center;
  }
  .mini {
    padding: 0 6px;
    font-size: 0.9rem;
    line-height: 1;
    background: #94a3b8;
    border-color: #94a3b8;
  }
  .ghost {
    background: transparent;
    color: #2563eb;
  }
  .dropzone {
    --wails-drop-target: drop;
    margin: 8px 0;
    padding: 12px;
    border: 2px dashed #cbd5e1;
    border-radius: 6px;
    background: #f8fafc;
    color: #64748b;
    font-size: 0.9rem;
    min-height: 70px;
  }
  .dropzone.has-items {
    border-style: solid;
    background: #eef2ff;
    color: #1e293b;
  }
  .dropzone p {
    margin: 0;
    text-align: center;
  }
  .queued {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .queued li {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .queued code {
    flex: 1;
    font-size: 0.8rem;
    color: #334155;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .file-icon {
    display: inline-block;
    width: 20px;
    text-align: center;
  }
  .dropzone:global(.wails-drop-target-active) {
    border-color: #2563eb;
    border-style: solid;
    background: #eff6ff;
    color: #1e293b;
  }
</style>
