<script lang="ts">
  export let maxFiles = 0;
  export let maxDepth = 0;
  export let minDiskPct = 0;
  export let tcpCooldown = 0;
  export let udpCooldown = 0;

  export let onMaxFilesChange: (n: number) => void = () => {};
  export let onCommitMaxFiles: () => void = () => {};
  export let onMaxDepthChange: (n: number) => void = () => {};
  export let onCommitMaxDepth: () => void = () => {};
  export let onMinDiskPctChange: (n: number) => void = () => {};
  export let onCommitMinDiskPct: () => void = () => {};
  export let onTCPCooldownChange: (n: number) => void = () => {};
  export let onCommitTCPCooldown: () => void = () => {};
  export let onUDPCooldownChange: (n: number) => void = () => {};
  export let onCommitUDPCooldown: () => void = () => {};

  function numInput(cb: (n: number) => void) {
    return (e: Event) => cb(Number((e.currentTarget as HTMLInputElement).value));
  }
</script>

<div class="drawer-section first">
  <div class="addrs-title">Files per session</div>
  <div class="row">
    <input
      type="number"
      min="0"
      value={maxFiles}
      on:input={numInput(onMaxFilesChange)}
      on:change={onCommitMaxFiles}
      style="width: 120px"
    />
    <span>files (0 = unlimited)</span>
  </div>
  <p class="hint">Caps the element count per incoming session. Protects against floods of tiny files.</p>
</div>

<div class="drawer-section">
  <div class="addrs-title">Maximum path depth</div>
  <div class="row">
    <input
      type="number"
      min="0"
      max="64"
      value={maxDepth}
      on:input={numInput(onMaxDepthChange)}
      on:change={onCommitMaxDepth}
      style="width: 120px"
    />
    <span>segments (0 = unlimited)</span>
  </div>
  <p class="hint">Refuses elements with more than this many “/” segments.</p>
</div>

<div class="drawer-section">
  <div class="addrs-title">Keep free disk space above</div>
  <div class="row">
    <input
      type="number"
      min="0"
      max="99"
      value={minDiskPct}
      on:input={numInput(onMinDiskPctChange)}
      on:change={onCommitMinDiskPct}
      style="width: 120px"
    />
    <span>% (0 = disabled)</span>
  </div>
  <p class="hint">Refuses a session if completing it would leave less free space than this fraction of the volume.</p>
</div>

<div class="drawer-section">
  <div class="addrs-title">TCP accept cooldown per IP</div>
  <div class="row">
    <input
      type="number"
      min="0"
      max="600"
      value={tcpCooldown}
      on:input={numInput(onTCPCooldownChange)}
      on:change={onCommitTCPCooldown}
      style="width: 120px"
    />
    <span>seconds (0 = disabled)</span>
  </div>
  <p class="hint">Drops successive connections from the same source inside this window.</p>
</div>

<div class="drawer-section">
  <div class="addrs-title">UDP HELLO cooldown per IP</div>
  <div class="row">
    <input
      type="number"
      min="0"
      max="600"
      value={udpCooldown}
      on:input={numInput(onUDPCooldownChange)}
      on:change={onCommitUDPCooldown}
      style="width: 120px"
    />
    <span>seconds (0 = disabled)</span>
  </div>
  <p class="hint">Rate-limits incoming discovery broadcasts from one peer. 1–2 s mitigates broadcast storms without blocking well-behaved peers.</p>
</div>

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
  .hint {
    margin: 6px 0 0;
    color: #64748b;
    font-size: 0.8rem;
  }
  input[type='number'] {
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
</style>
