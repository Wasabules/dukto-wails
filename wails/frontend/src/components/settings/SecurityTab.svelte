<script lang="ts">
  import { parseSignature, type PinnedPeer } from '../../lib/dukto';

  export let wlEnabled = false;
  export let wlList: string[] = [];
  export let rejectExts: string[] = [];
  export let largeMB = 0;
  export let extInput = '';
  export let idleMinutes = 0;
  export let blockList: string[] = [];
  export let confirmUnknown = false;
  export let refuseCleartext = false;
  export let hideFromDiscovery = false;
  export let pinned: PinnedPeer[] = [];

  export let onToggleWhitelist: (on: boolean) => void = () => {};
  export let onUntrustSig: (sig: string) => void = () => {};
  export let onAddRejectExt: () => void = () => {};
  export let onRemoveRejectExt: (ext: string) => void = () => {};
  export let onCommitLargeMB: () => void = () => {};
  export let onExtInputChange: (v: string) => void = () => {};
  export let onLargeMBChange: (mb: number) => void = () => {};
  export let onIdleMinutesChange: (mins: number) => void = () => {};
  export let onCommitIdleMinutes: () => void = () => {};
  export let onUnblockSig: (sig: string) => void = () => {};
  export let onToggleConfirmUnknown: (on: boolean) => void = () => {};
  export let onForgetApprovals: () => void = () => {};
  export let onToggleRefuseCleartext: (on: boolean) => void = () => {};
  export let onToggleHideFromDiscovery: (on: boolean) => void = () => {};
  export let onUnpinPeer: (fingerprint: string) => void = () => {};

  function handleWhitelistChange(e: Event) {
    onToggleWhitelist((e.currentTarget as HTMLInputElement).checked);
  }
  function handleExtInput(e: Event) {
    onExtInputChange((e.currentTarget as HTMLInputElement).value);
  }
  function handleLargeInput(e: Event) {
    onLargeMBChange(Number((e.currentTarget as HTMLInputElement).value));
  }
  function handleIdleInput(e: Event) {
    onIdleMinutesChange(Number((e.currentTarget as HTMLInputElement).value));
  }
  function handleConfirmUnknownChange(e: Event) {
    onToggleConfirmUnknown((e.currentTarget as HTMLInputElement).checked);
  }
  function handleRefuseCleartextChange(e: Event) {
    onToggleRefuseCleartext((e.currentTarget as HTMLInputElement).checked);
  }
  function handleHideFromDiscoveryChange(e: Event) {
    onToggleHideFromDiscovery((e.currentTarget as HTMLInputElement).checked);
  }

  function fmtPinnedAt(ts: string): string {
    const d = new Date(ts);
    if (isNaN(d.getTime())) return ts;
    return d.toLocaleString();
  }
</script>

<div class="drawer-section first">
  <div class="addrs-title">Allow-list</div>
  <label class="check">
    <input
      type="checkbox"
      checked={wlEnabled}
      on:change={handleWhitelistChange}
    />
    Only accept transfers from approved buddies
  </label>
  {#if wlList.length > 0}
    <ul class="chip-list">
      {#each wlList as sig (sig)}
        <li>
          <span title={sig}>{parseSignature(sig).user || sig}</span>
          <button class="mini" type="button" on:click={() => onUntrustSig(sig)}>×</button>
        </li>
      {/each}
    </ul>
  {:else}
    <p class="empty">No entries yet. Click ＋ on a buddy card to add.</p>
  {/if}
</div>

<div class="drawer-section">
  <div class="addrs-title">Auto-reject extensions</div>
  {#if rejectExts.length > 0}
    <ul class="chip-list">
      {#each rejectExts as ext (ext)}
        <li>
          <span>.{ext}</span>
          <button class="mini" type="button" on:click={() => onRemoveRejectExt(ext)}>×</button>
        </li>
      {/each}
    </ul>
  {/if}
  <div class="row">
    <input
      type="text"
      placeholder="e.g. exe"
      value={extInput}
      on:input={handleExtInput}
      on:keydown={(e) => { if (e.key === 'Enter') { e.preventDefault(); onAddRejectExt(); } }}
    />
    <button on:click={onAddRejectExt}>Add</button>
  </div>
</div>

<div class="drawer-section">
  <div class="addrs-title">Reject sessions larger than</div>
  <div class="row">
    <input
      type="number"
      min="0"
      value={largeMB}
      on:input={handleLargeInput}
      on:change={onCommitLargeMB}
      style="width: 100px"
    />
    <span>MB (0 = disabled)</span>
  </div>
</div>

<div class="drawer-section">
  <div class="addrs-title">Block list</div>
  {#if blockList.length > 0}
    <ul class="chip-list">
      {#each blockList as sig (sig)}
        <li>
          <span title={sig}>{parseSignature(sig).user || sig}</span>
          <button class="mini" type="button" on:click={() => onUnblockSig(sig)}>×</button>
        </li>
      {/each}
    </ul>
  {:else}
    <p class="empty">No blocked peers. Use a peer card’s menu to block a spammer.</p>
  {/if}
</div>

<div class="drawer-section">
  <div class="addrs-title">Confirm unknown peers</div>
  <label class="check">
    <input
      type="checkbox"
      checked={confirmUnknown}
      on:change={handleConfirmUnknownChange}
    />
    Prompt before accepting the first session from a new peer
  </label>
  <div class="row">
    <button class="secondary" type="button" on:click={onForgetApprovals}>Forget approvals</button>
  </div>
  <p class="hint">Approved peers are remembered forever unless you click “Forget approvals”. Rejections auto-close after 60 s.</p>
</div>

<div class="drawer-section">
  <div class="addrs-title">Discovery</div>
  <label class="check">
    <input
      type="checkbox"
      checked={hideFromDiscovery}
      on:change={handleHideFromDiscoveryChange}
    />
    Hide me from network discovery
  </label>
  <p class="hint">When on, Dukto stops broadcasting HELLO and stops replying to probes. Other peers can't see you on the LAN unless they already know your IP (e.g. via Manual peers). You still see them, and incoming transfers from peers who do know your IP still work.</p>
</div>

<div class="drawer-section">
  <div class="addrs-title">Encrypted overlay (v2)</div>
  <label class="check">
    <input
      type="checkbox"
      checked={refuseCleartext}
      on:change={handleRefuseCleartextChange}
    />
    Refuse cleartext transfers (require Noise XX)
  </label>
  <p class="hint">When on, peers must be paired (🔒) — sessions from v1 peers and unpaired v2 peers are dropped. Use the 🤝 button on the peer card to pair via a 5-word passphrase.</p>

  <div class="addrs-title" style="margin-top: 12px">Paired peers</div>
  {#if pinned.length > 0}
    <ul class="pin-list">
      {#each pinned as p (p.fingerprint)}
        <li>
          <div class="pin-row">
            <div class="pin-meta">
              <div class="pin-label" title={p.label}>{parseSignature(p.label).user || p.label || 'unnamed'}</div>
              <code class="pin-fp">{p.fingerprint}</code>
              <div class="pin-when">paired {fmtPinnedAt(p.pinnedAt)}</div>
            </div>
            <button class="secondary" type="button" on:click={() => onUnpinPeer(p.fingerprint)}>Unpin</button>
          </div>
        </li>
      {/each}
    </ul>
  {:else}
    <p class="empty">No paired peers yet. Tap 🤝 on a buddy card to bootstrap mutual trust via a one-shot 5-word code.</p>
  {/if}
</div>

<div class="drawer-section">
  <div class="addrs-title">Auto-disable receiving after inactivity</div>
  <div class="row">
    <input
      type="number"
      min="0"
      value={idleMinutes}
      on:input={handleIdleInput}
      on:change={onCommitIdleMinutes}
      style="width: 100px"
    />
    <span>minutes (0 = never)</span>
  </div>
  <p class="hint">Turns the master switch off after the given number of minutes without any received transfer. Re-enable it from the header pill.</p>
</div>

<style>
  .drawer-section {
    margin-top: 12px;
    padding-top: 8px;
    border-top: 1px solid var(--panel-border);
  }
  .drawer-section.first {
    margin-top: 0;
    padding-top: 0;
    border-top: 0;
  }
  .addrs-title {
    font-size: 0.75rem;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--text-dim);
    margin-bottom: 4px;
  }
  .empty {
    color: var(--text-faint);
    font-size: 0.9rem;
  }
  .pin-list {
    list-style: none;
    padding: 0;
    margin: 6px 0 0;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .pin-list li {
    background: var(--code-bg);
    border-radius: 6px;
    padding: 8px 10px;
  }
  .pin-row {
    display: flex;
    align-items: center;
    gap: 12px;
  }
  .pin-meta {
    flex: 1;
    min-width: 0;
  }
  .pin-label {
    font-weight: 600;
    font-size: 0.9rem;
    color: var(--text);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .pin-fp {
    display: block;
    font-size: 0.78rem;
    color: var(--text-dim);
    user-select: all;
  }
  .pin-when {
    font-size: 0.72rem;
    color: var(--text-faint);
    margin-top: 2px;
  }
  .hint {
    margin: 6px 0 0;
    color: var(--text-dim);
    font-size: 0.8rem;
  }
  label.check {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 0.9rem;
    margin: 8px 0;
  }
  label.check input {
    width: auto;
    margin: 0;
  }
  input[type='text'],
  input[type='number'] {
    box-sizing: border-box;
    font: inherit;
    padding: 6px 8px;
    border: 1px solid var(--input-border);
    border-radius: 4px;
      background-color: var(--input-bg);
      color: var(--text);
  }
  input[type='text'] {
    flex: 1;
      background-color: var(--input-bg);
      color: var(--text);
  }
  .row {
    display: flex;
    gap: 8px;
    align-items: center;
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
  button.secondary {
    background: var(--panel-bg);
    color: var(--accent);
  }
  .chip-list {
    list-style: none;
    padding: 0;
    margin: 4px 0 8px;
    display: flex;
    flex-wrap: wrap;
    gap: 4px;
  }
  .chip-list li {
    display: flex;
    align-items: center;
    gap: 4px;
    padding: 2px 4px 2px 8px;
    background: var(--panel-border);
    border-radius: 12px;
    font-size: 0.8rem;
  }
  .mini {
    padding: 0 6px;
    font-size: 0.9rem;
    line-height: 1;
    background: var(--text-faint);
    border-color: var(--text-faint);
    border-radius: 4px;
    cursor: pointer;
    color: var(--panel-bg);
  }
</style>
