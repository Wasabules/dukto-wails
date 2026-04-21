<script lang="ts">
  import type { NetIfaceView } from '../../lib/dukto';

  export let manualList: string[] = [];
  export let manualInput = '';
  export let myAddrs: string[] = [];
  export let ifaces: NetIfaceView[] = [];
  export let allowedIfaces: string[] = [];

  export let onAddManual: () => void = () => {};
  export let onRemoveManual: (addr: string) => void = () => {};
  export let onManualInputChange: (v: string) => void = () => {};
  export let onToggleIface: (name: string, on: boolean) => void = () => {};

  function handleManualInput(e: Event) {
    onManualInputChange((e.currentTarget as HTMLInputElement).value);
  }
  function handleIfaceChange(name: string) {
    return (e: Event) => onToggleIface(name, (e.currentTarget as HTMLInputElement).checked);
  }
</script>

<div class="drawer-section first">
  <div class="addrs-title">Cross-subnet peers</div>
  {#if manualList.length > 0}
    <ul class="chip-list">
      {#each manualList as addr (addr)}
        <li>
          <span>{addr}</span>
          <button class="mini" type="button" on:click={() => onRemoveManual(addr)}>×</button>
        </li>
      {/each}
    </ul>
  {/if}
  <div class="row">
    <input
      type="text"
      placeholder="192.168.2.42 or 10.0.0.3:4644"
      value={manualInput}
      on:input={handleManualInput}
      on:keydown={(e) => { if (e.key === 'Enter') { e.preventDefault(); onAddManual(); } }}
    />
    <button on:click={onAddManual}>Add</button>
  </div>
</div>

{#if ifaces.length > 0}
  <div class="drawer-section">
    <div class="addrs-title">Allowed network interfaces</div>
    <ul class="iface-list">
      {#each ifaces as iface (iface.name)}
        <li>
          <label class="iface">
            <input
              type="checkbox"
              checked={allowedIfaces.includes(iface.name)}
              on:change={handleIfaceChange(iface.name)}
              disabled={!iface.active}
            />
            <span class="name">{iface.name}</span>
            {#if !iface.active}<span class="tag">down</span>{/if}
            {#if iface.addresses && iface.addresses.length > 0}
              <span class="addrs">{iface.addresses.join(', ')}</span>
            {/if}
          </label>
        </li>
      {/each}
    </ul>
    <p class="hint">Unchecked interfaces are ignored by the TCP server. Leave everything unchecked to accept on every interface.</p>
  </div>
{/if}

{#if myAddrs.length > 0}
  <div class="drawer-section">
    <div class="addrs-title">Listening on</div>
    <ul class="addr-list">
      {#each myAddrs as a}
        <li><code>{a}</code></li>
      {/each}
    </ul>
  </div>
{/if}

<style>
  .drawer-section {
    margin-top: 12px;
    padding-top: 8px;
    border-top: 1px solid #e2e8f0;
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
    color: #64748b;
    margin-bottom: 4px;
  }
  .addr-list {
    list-style: none;
    padding: 0;
    margin: 0;
  }
  .addr-list li {
    font-size: 0.85rem;
    padding: 2px 0;
  }
  .iface-list {
    list-style: none;
    padding: 0;
    margin: 0;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .iface {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 0.85rem;
  }
  .iface input {
    flex: none;
    margin: 0;
  }
  .iface .name {
    font-weight: 600;
  }
  .iface .tag {
    background: #e2e8f0;
    color: #475569;
    padding: 1px 6px;
    font-size: 0.7rem;
    border-radius: 3px;
  }
  .iface .addrs {
    color: #64748b;
    font-family: ui-monospace, monospace;
    font-size: 0.75rem;
  }
  .hint {
    margin: 6px 0 0;
    color: #64748b;
    font-size: 0.8rem;
  }
  input {
    flex: 1;
    box-sizing: border-box;
    font: inherit;
    padding: 6px 8px;
    border: 1px solid #cbd5e1;
    border-radius: 4px;
  }
  .row {
    display: flex;
    gap: 8px;
    align-items: center;
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
    background: #e2e8f0;
    border-radius: 12px;
    font-size: 0.8rem;
  }
  .mini {
    padding: 0 6px;
    font-size: 0.9rem;
    line-height: 1;
    background: #94a3b8;
    border-color: #94a3b8;
    border-radius: 4px;
    cursor: pointer;
    color: #fff;
  }
</style>
