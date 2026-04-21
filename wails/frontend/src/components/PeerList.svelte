<script lang="ts">
  import { avatarUrl, parseSignature, peerKey, type Peer } from '../lib/dukto';

  export let peers: Peer[] = [];
  export let selectedKey: string | null = null;
  export let broadcastMode = false;
  export let broadcastSelected: Set<string> = new Set();
  export let sendByPeer: Map<string, { bytes: number; total: number }> = new Map();
  export let whitelistEnabled = false;
  export let whitelist: string[] = [];
  export let aliases: Record<string, string> = {};
  // `now` ticks so idle peers grey out over time. Owned by App.svelte.
  export let now: number = Date.now();
  // lastSeen: peer-key → ms timestamp. Same owner as `now`.
  export let lastSeen: Map<string, number> = new Map();

  export let onSelect: (key: string) => void = () => {};
  export let onToggleBroadcastPick: (key: string) => void = () => {};
  export let onRenamePeer: (p: Peer) => void = () => {};
  export let onTrustPeer: (p: Peer) => void = () => {};
  export let onToggleBroadcastMode: (on: boolean) => void = () => {};

  const idleThresholdMs = 5 * 60 * 1000;
  function idleFor(p: Peer, nowMs: number): number {
    const t = lastSeen.get(peerKey(p));
    return t === undefined ? idleThresholdMs : nowMs - t;
  }
  function peerLabel(p: Peer): string {
    const alias = aliases[p.signature];
    if (alias) return alias;
    const ps = parseSignature(p.signature);
    return ps.user || p.address;
  }
  function hideBrokenAvatar(event: Event) {
    (event.currentTarget as HTMLImageElement).style.visibility = 'hidden';
  }
  function handleBroadcastModeChange(e: Event) {
    onToggleBroadcastMode((e.currentTarget as HTMLInputElement).checked);
  }
</script>

<section class="peers">
  <div class="section-head">
    <h2>Buddies on your network</h2>
    <label class="inline-toggle">
      <input
        type="checkbox"
        checked={broadcastMode}
        on:change={handleBroadcastModeChange}
      />
      Broadcast
    </label>
  </div>
  {#if peers.length === 0}
    <p class="empty">No peers yet. Make sure Dukto is running on another device in the same LAN.</p>
  {:else}
    <ul>
      {#each peers as p (peerKey(p))}
        {@const ps = parseSignature(p.signature)}
        {@const k = peerKey(p)}
        {@const idle = idleFor(p, now) > idleThresholdMs}
        {@const prog = sendByPeer.get(k)}
        {@const trusted = whitelist.includes(p.signature)}
        <li
          data-peer-key={k}
          class:selected={selectedKey === k}
          class:idle
          class:drop-target={true}
          style="--wails-drop-target: drop"
        >
          <button
            type="button"
            class="peer-btn"
            on:click={() => {
              if (broadcastMode) onToggleBroadcastPick(k);
              else onSelect(k);
            }}
          >
            {#if broadcastMode}
              <input
                type="checkbox"
                class="pick"
                checked={broadcastSelected.has(k)}
                on:click|stopPropagation={() => onToggleBroadcastPick(k)}
              />
            {/if}
            <img src={avatarUrl(p)} alt="" on:error={hideBrokenAvatar} />
            <div class="who">
              <div class="name">
                {peerLabel(p)}
                {#if trusted}<span class="trust-badge" title="In allow-list">✓</span>{/if}
              </div>
              <div class="detail">{ps.host || p.address} · {ps.platform || '–'}</div>
            </div>
          </button>
          <div class="peer-actions">
            <button
              type="button"
              class="mini ghost"
              title="Rename locally"
              on:click|stopPropagation={() => onRenamePeer(p)}
            >✎</button>
            {#if whitelistEnabled && !trusted}
              <button
                type="button"
                class="mini ghost"
                title="Allow to send"
                on:click|stopPropagation={() => onTrustPeer(p)}
              >＋</button>
            {/if}
          </div>
          {#if prog}
            <div class="peer-progress">
              <div
                class="peer-progress-fill"
                style="width: {prog.total > 0 ? (prog.bytes / prog.total) * 100 : 0}%"
              ></div>
            </div>
          {/if}
        </li>
      {/each}
    </ul>
  {/if}
</section>

<style>
  section.peers {
    grid-area: peers;
    background: #ffffff;
    border: 1px solid #e2e8f0;
    border-radius: 6px;
    padding: 10px 14px;
    overflow: auto;
  }
  h2 {
    margin: 0;
    font-size: 0.9rem;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: #475569;
  }
  .empty {
    color: #94a3b8;
    font-size: 0.9rem;
  }
  ul {
    list-style: none;
    padding: 0;
    margin: 0;
  }
  li {
    list-style: none;
    padding: 0;
    margin: 0;
    position: relative;
  }
  .peer-btn {
    display: grid;
    grid-template-columns: 40px 1fr;
    gap: 8px;
    align-items: center;
    padding: 6px;
    border-radius: 4px;
    width: 100%;
    background: transparent;
    border: 0;
    color: inherit;
    text-align: left;
    cursor: pointer;
  }
  .peer-btn:hover {
    background: #f1f5f9;
  }
  li.selected .peer-btn {
    background: #dbeafe;
  }
  img {
    width: 40px;
    height: 40px;
    border-radius: 6px;
    background: #cbd5e1;
  }
  .name {
    font-weight: 600;
  }
  .detail {
    font-size: 0.8rem;
    color: #64748b;
  }
  .peer-actions {
    position: absolute;
    top: 6px;
    right: 6px;
    display: none;
    gap: 4px;
  }
  li:hover .peer-actions {
    display: flex;
  }
  li.idle .peer-btn {
    opacity: 0.5;
  }
  /* Wails attaches `wails-drop-target-active` at runtime while a drag hovers
     the target, so the class isn't in the markup — :global() keeps the rule
     from being stripped as unused. */
  li:global(.wails-drop-target-active) {
    outline: 2px dashed #2563eb;
    outline-offset: -2px;
    background: #eff6ff;
  }
  .pick {
    width: 16px;
    height: 16px;
    margin: 0 6px 0 0;
  }
  .trust-badge {
    display: inline-block;
    margin-left: 4px;
    color: #059669;
    font-weight: 700;
  }
  .peer-progress {
    position: absolute;
    left: 6px;
    right: 6px;
    bottom: 2px;
    height: 3px;
    background: rgba(15, 23, 42, 0.08);
    border-radius: 2px;
    overflow: hidden;
  }
  .peer-progress-fill {
    background: #22d3ee;
    height: 100%;
    transition: width 120ms ease-out;
  }
  .section-head {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    margin-bottom: 8px;
    gap: 8px;
  }
  .inline-toggle {
    font-size: 0.8rem;
    color: #475569;
    display: flex;
    align-items: center;
    gap: 4px;
  }
  .inline-toggle input {
    width: auto;
  }
  .mini {
    padding: 0 6px;
    font-size: 0.9rem;
    line-height: 1;
    border-radius: 4px;
    cursor: pointer;
    border: 1px solid #2563eb;
    background: #2563eb;
    color: #fff;
  }
  .mini.ghost {
    background: transparent;
    color: #2563eb;
  }
</style>
