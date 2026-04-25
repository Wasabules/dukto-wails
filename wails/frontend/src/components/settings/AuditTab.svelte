<script lang="ts">
  import type { AuditEntry } from '../../lib/dukto';

  export let entries: AuditEntry[] = [];
  export let onRefresh: () => void = () => {};
  export let onClear: () => void = () => {};

  function fmtTime(t: string | undefined): string {
    if (!t) return '';
    const d = new Date(t);
    if (Number.isNaN(d.getTime())) return t;
    return d.toLocaleString();
  }
</script>

<div class="drawer-section first">
  <div class="addrs-title">
    Audit log ({entries.length})
    <span class="spacer"></span>
    <button class="mini" type="button" on:click={onRefresh}>Refresh</button>
    <button class="mini ghost" type="button" on:click={onClear}>Clear</button>
  </div>
  {#if entries.length === 0}
    <p class="empty">No events yet. Policy hits and session accepts/rejects are recorded here.</p>
  {:else}
    <div class="log">
      {#each entries as e, i (i)}
        <div class="entry" class:rejected={e.kind === 'reject'}>
          <div class="row">
            <span class="kind">{e.kind}</span>
            <span class="reason">{e.reason ?? ''}</span>
            <span class="when">{fmtTime(e.time)}</span>
          </div>
          <div class="meta">
            {#if e.peer}<span title={e.peer}>{e.peer}</span>{/if}
            {#if e.remote}<span>{e.remote}</span>{/if}
            {#if e.detail}<span class="detail">{e.detail}</span>{/if}
          </div>
        </div>
      {/each}
    </div>
  {/if}
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
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 0.75rem;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--text-dim);
    margin-bottom: 6px;
  }
  .spacer {
    flex: 1;
  }
  .empty {
    color: var(--text-faint);
    font-size: 0.9rem;
  }
  .log {
    display: flex;
    flex-direction: column;
    gap: 4px;
    max-height: 50vh;
    overflow-y: auto;
    padding: 4px;
    background: var(--panel-bg-2);
    border: 1px solid var(--panel-border);
    border-radius: 4px;
    font-size: 0.8rem;
  }
  .entry {
    padding: 4px 6px;
    border-radius: 3px;
    background: var(--panel-bg);
    border: 1px solid var(--panel-border);
  }
  .entry.rejected {
    border-color: var(--error-border);
    background: var(--error-bg);
  }
  .row {
    display: flex;
    gap: 8px;
    align-items: baseline;
  }
  .kind {
    font-weight: 600;
    text-transform: uppercase;
    font-size: 0.7rem;
    color: var(--text);
  }
  .entry.rejected .kind {
    color: var(--error);
  }
  .reason {
    color: var(--text-strong);
    font-family: ui-monospace, monospace;
    font-size: 0.75rem;
  }
  .when {
    margin-left: auto;
    color: var(--text-faint);
    font-size: 0.7rem;
  }
  .meta {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    color: var(--text-dim);
    font-size: 0.75rem;
  }
  .detail {
    font-family: ui-monospace, monospace;
  }
  button {
    padding: 2px 8px;
    font: inherit;
    border: 1px solid var(--accent);
    background: var(--accent);
    color: var(--accent-on);
    border-radius: 4px;
    cursor: pointer;
    font-size: 0.75rem;
  }
  button.ghost {
    background: transparent;
    color: var(--accent);
  }
</style>
