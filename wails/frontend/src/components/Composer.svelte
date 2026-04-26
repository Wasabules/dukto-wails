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
  export let onPickFiles: () => void = () => {};
  export let onPickFolder: () => void = () => {};
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
        <p>Drop files here, or use the buttons below, then click Send to fan out.</p>
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
      <div class="picker-row">
        <button class="ghost" type="button" on:click={onPickFiles}>Pick file(s)</button>
        <button class="ghost" type="button" on:click={onPickFolder}>Pick folder</button>
      </div>
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
    {#if !selectedPeer.paired}
      <div class="ct-warn">
        ⚠️ <strong>Cleartext.</strong>
        {#if selectedPeer.v2Capable}
          This peer supports encryption but isn't paired yet — pair via the ⋮ menu to encrypt future sends.
        {:else}
          This peer doesn't run a v2 Dukto — bytes go on the wire in clear. Anyone on the LAN can read them.
        {/if}
      </div>
    {/if}
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
        <p>Drop files or folders here, use the buttons below, or drop directly onto a peer to send immediately.</p>
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
      <div class="picker-row">
        <button class="ghost" type="button" on:click={onPickFiles}>Pick file(s)</button>
        <button class="ghost" type="button" on:click={onPickFolder}>Pick folder</button>
      </div>
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
    background: var(--panel-bg);
    border: 1px solid var(--panel-border);
    border-radius: 6px;
    padding: 10px 14px;
    overflow: auto;
  }
  h2 {
    margin: 0 0 8px;
    font-size: 0.9rem;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--text);
  }
  .empty {
    color: var(--text-faint);
    font-size: 0.9rem;
  }
  .ct-warn {
    margin: 6px 0;
    padding: 6px 10px;
    border: 1px solid var(--warn, #d39e00);
    border-radius: 6px;
    background: color-mix(in srgb, var(--warn, #d39e00) 12%, transparent);
    font-size: 0.82rem;
    color: var(--text);
    line-height: 1.35;
  }
  label {
    display: block;
    font-size: 0.85rem;
    color: var(--text);
    margin: 8px 0;
  }
  textarea {
    width: 100%;
    box-sizing: border-box;
    font: inherit;
    padding: 6px 8px;
    border: 1px solid var(--input-border);
    border-radius: 4px;
      background-color: var(--input-bg);
      color: var(--text);
  }
  button {
    padding: 6px 12px;
    font: inherit;
    border: 1px solid var(--accent);
    background: var(--accent);
    color: var(--accent-on);
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
    background: var(--text-faint);
    border-color: var(--text-faint);
  }
  .ghost {
    background: transparent;
    color: var(--accent);
  }
  .dropzone {
    --wails-drop-target: drop;
    margin: 8px 0;
    padding: 12px;
    border: 2px dashed var(--input-border);
    border-radius: 6px;
    background: var(--panel-bg-2);
    color: var(--text-dim);
    font-size: 0.9rem;
    min-height: 70px;
  }
  .dropzone.has-items {
    border-style: solid;
    background: var(--accent-soft);
    color: var(--text-strong);
  }
  .dropzone p {
    margin: 0;
    text-align: center;
  }
  .picker-row {
    display: flex;
    gap: 8px;
    justify-content: center;
    margin-top: 8px;
    /* Stop the picker buttons from claiming the drop event — drops should
       still flow to the dropzone container, not get swallowed mid-button. */
    pointer-events: auto;
  }
  .picker-row button {
    /* Compact, subtle — these are alternatives to the primary action. */
    padding: 4px 12px;
    font-size: 0.85rem;
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
    color: var(--text);
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
    border-color: var(--accent);
    border-style: solid;
    background: var(--accent-soft);
    color: var(--text-strong);
  }
</style>
