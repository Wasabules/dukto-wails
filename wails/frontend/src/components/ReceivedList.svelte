<script lang="ts">
  import { parseSignature, type Peer } from '../lib/dukto';
  import ReceivedRow, { type ReceivedItem } from './ReceivedRow.svelte';

  // Bottom-right panel. Groups items into sender-keyed threads and lets the
  // user search, export, and clear. Thread expand/collapse is local state
  // because it's purely presentational.
  export let items: ReceivedItem[] = [];
  export let peers: Peer[] = [];
  export let aliases: Record<string, string> = {};
  export let hashByPath: Map<string, string> = new Map();

  export let onOpenPreview: (r: ReceivedItem) => void = () => {};
  export let onOpenExternal: (path: string) => void = () => {};
  export let onReveal: (path: string) => void = () => {};
  export let onCopyText: (text: string) => void = () => {};
  export let onHash: (path: string) => void = () => {};
  export let onExport: (format: 'csv' | 'json') => void = () => {};
  export let onClear: () => void = () => {};

  let searchTerm = '';
  let collapsed = new Set<string>();

  function toggleThread(key: string) {
    const next = new Set(collapsed);
    if (next.has(key)) next.delete(key); else next.add(key);
    collapsed = next;
  }

  function peerLabel(p: Peer): string {
    const alias = aliases[p.signature];
    if (alias) return alias;
    const ps = parseSignature(p.signature);
    return ps.user || p.address;
  }

  interface Thread {
    fromIp: string;
    label: string;
    items: ReceivedItem[];
    latest: Date;
  }
  $: threads = (function buildThreads(): Thread[] {
    const q = searchTerm.trim().toLowerCase();
    const match = (r: ReceivedItem): boolean => {
      if (!q) return true;
      return (
        r.name.toLowerCase().includes(q) ||
        r.text.toLowerCase().includes(q) ||
        r.from.toLowerCase().includes(q)
      );
    };
    const byIp = new Map<string, ReceivedItem[]>();
    for (const r of items) {
      if (!match(r)) continue;
      const ip = (r.from || 'unknown').split(':')[0];
      const bucket = byIp.get(ip) ?? [];
      bucket.push(r);
      byIp.set(ip, bucket);
    }
    const out: Thread[] = [];
    for (const [ip, bucket] of byIp) {
      const peer = peers.find((p) => p.address === ip);
      const label = peer ? peerLabel(peer) : ip;
      out.push({ fromIp: ip, label, items: bucket, latest: bucket[0].at });
    }
    out.sort((a, b) => b.latest.getTime() - a.latest.getTime());
    return out;
  })();
</script>

<section class="received">
  <div class="section-head">
    <h2>Received</h2>
    <div class="received-tools">
      {#if items.length > 0}
        <input
          type="search"
          class="search"
          placeholder="Search…"
          bind:value={searchTerm}
        />
        <button class="mini ghost" type="button" on:click={() => onExport('csv')}>Export CSV</button>
        <button class="mini ghost" type="button" on:click={() => onExport('json')}>Export JSON</button>
        <button class="mini ghost" type="button" on:click={onClear}>Clear</button>
      {/if}
    </div>
  </div>
  {#if items.length === 0}
    <p class="empty">Nothing yet. Incoming transfers will appear here.</p>
  {:else}
    {#each threads as t (t.fromIp)}
      <div class="thread">
        <button
          type="button"
          class="thread-head"
          on:click={() => toggleThread(t.fromIp)}
          aria-expanded={!collapsed.has(t.fromIp)}
        >
          <span class="thread-caret">{collapsed.has(t.fromIp) ? '▸' : '▾'}</span>
          <span class="thread-label">{t.label}</span>
          <span class="thread-count">{t.items.length}</span>
        </button>
        {#if !collapsed.has(t.fromIp)}
          <ul>
            {#each t.items as r (r.id)}
              <ReceivedRow
                item={r}
                hash={hashByPath.get(r.path) ?? null}
                {onOpenPreview}
                {onOpenExternal}
                {onReveal}
                {onCopyText}
                {onHash}
              />
            {/each}
          </ul>
        {/if}
      </div>
    {/each}
  {/if}
</section>

<style>
  section.received {
    grid-area: received;
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
  .thread {
    margin-bottom: 10px;
  }
  .thread-head {
    display: flex;
    align-items: center;
    gap: 6px;
    width: 100%;
    padding: 4px 8px;
    background: #f1f5f9;
    border: 0;
    border-radius: 4px;
    color: #334155;
    font-weight: 600;
    cursor: pointer;
    text-align: left;
  }
  .thread-head:hover {
    background: #e2e8f0;
  }
  .thread-caret {
    font-size: 0.75rem;
    opacity: 0.6;
    width: 12px;
  }
  .thread-label {
    flex: 1;
  }
  .thread-count {
    font-size: 0.75rem;
    padding: 1px 8px;
    border-radius: 10px;
    background: #cbd5e1;
    color: #1e293b;
    font-weight: 500;
  }
  .section-head {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    margin-bottom: 8px;
    gap: 8px;
  }
  .received-tools {
    display: flex;
    gap: 6px;
    align-items: center;
    flex-wrap: wrap;
  }
  .search {
    font-size: 0.8rem;
    padding: 4px 8px;
    width: 140px;
    box-sizing: border-box;
    font-family: inherit;
    border: 1px solid #cbd5e1;
    border-radius: 4px;
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
