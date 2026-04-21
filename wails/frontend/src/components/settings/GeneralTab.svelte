<script lang="ts">
  export let buddyName = '';
  export let destDir = '';
  export let notificationsOn = false;
  export let trayOn = false;
  // Data-URL PNG, null until the parent lazily loads it.
  export let qrData: string | null = null;

  export let onBuddyNameChange: (name: string) => void = () => {};
  export let onCommitBuddyName: () => void = () => {};
  export let onPickDest: () => void = () => {};
  export let onToggleNotifications: (on: boolean) => void = () => {};
  export let onToggleTray: (on: boolean) => void = () => {};

  function handleBuddyInput(e: Event) {
    onBuddyNameChange((e.currentTarget as HTMLInputElement).value);
  }
  function handleNotifChange(e: Event) {
    onToggleNotifications((e.currentTarget as HTMLInputElement).checked);
  }
  function handleTrayChange(e: Event) {
    onToggleTray((e.currentTarget as HTMLInputElement).checked);
  }
</script>

<label>
  Buddy name
  <div class="row">
    <input
      value={buddyName}
      placeholder="Your display name"
      on:input={handleBuddyInput}
    />
    <button on:click={onCommitBuddyName}>Save</button>
  </div>
</label>
<label>
  Destination directory
  <div class="dest-row">
    <code class="dest">{destDir || '(not set)'}</code>
    <button on:click={onPickDest}>Browse…</button>
  </div>
</label>
<label class="check">
  <input
    type="checkbox"
    checked={notificationsOn}
    on:change={handleNotifChange}
  />
  Desktop notification on completed transfer
</label>
<label class="check">
  <input
    type="checkbox"
    checked={trayOn}
    on:change={handleTrayChange}
  />
  Keep running when window closes (relaunch to show)
</label>
{#if qrData}
  <div class="qr">
    <div class="addrs-title">Your signature</div>
    <img src={qrData} alt="QR code of this device's signature" />
  </div>
{/if}

<style>
  label {
    display: block;
    margin: 8px 0;
  }
  label.check {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 0.9rem;
  }
  label.check input {
    width: auto;
    margin: 0;
  }
  input {
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
  .row {
    display: flex;
    gap: 8px;
    align-items: center;
  }
  .dest-row {
    display: flex;
    gap: 8px;
    align-items: center;
    margin-top: 4px;
  }
  .dest {
    flex: 1;
    font-size: 0.85rem;
    padding: 6px 8px;
    background: #f1f5f9;
    border-radius: 4px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .addrs-title {
    font-size: 0.75rem;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: #64748b;
    margin-bottom: 4px;
  }
  .qr {
    margin-top: 12px;
    padding-top: 8px;
    border-top: 1px solid #e2e8f0;
    text-align: center;
  }
  .qr img {
    margin-top: 6px;
    max-width: 220px;
    image-rendering: pixelated;
  }
</style>
