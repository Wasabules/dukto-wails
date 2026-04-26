<script lang="ts">
  export let buddyName = '';
  export let destDir = '';
  export let notificationsOn = false;
  export let trayOn = false;
  // Data-URL PNG, null until the parent lazily loads it.
  export let qrData: string | null = null;
  // Same-origin URL for our own avatar (custom upload or initials), cache-busted
  // by the parent so a pick / reset / rename reloads the image.
  export let avatarUrl: string = '';
  export let hasCustomAvatar = false;

  // 'system' follows the OS pref via prefers-color-scheme; 'light' / 'dark'
  // force the matching theme regardless of OS.
  export let themeMode: 'system' | 'light' | 'dark' = 'system';

  // Long-term identity fingerprint (16 base32 chars, formatted as
  // XXXX-XXXX-XXXX-XXXX). Empty if the keypair couldn't be loaded.
  export let fingerprint: string = '';

  async function copyFingerprint() {
    if (!fingerprint) return;
    try { await navigator.clipboard.writeText(fingerprint); } catch (e) { /* no-op */ }
  }

  export let onBuddyNameChange: (name: string) => void = () => {};
  export let onCommitBuddyName: () => void = () => {};
  export let onPickDest: () => void = () => {};
  export let onPickAvatar: () => void = () => {};
  export let onClearAvatar: () => void = () => {};
  export let onThemeModeChange: (mode: 'system' | 'light' | 'dark') => void = () => {};
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
  Avatar
  <div class="avatar-row">
    {#if avatarUrl}
      <img class="avatar-preview" src={avatarUrl} alt="Your avatar" />
    {:else}
      <div class="avatar-preview placeholder" aria-hidden="true">…</div>
    {/if}
    <div class="avatar-actions">
      <button on:click={onPickAvatar}>Pick image…</button>
      {#if hasCustomAvatar}
        <button class="ghost" type="button" on:click={onClearAvatar}>Reset to initials</button>
      {/if}
    </div>
  </div>
  <small class="hint">
    {#if hasCustomAvatar}
      Custom image — peers fetch it from this device's avatar HTTP endpoint.
    {:else}
      Auto-generated from your buddy name. Pick a PNG / JPEG to override.
    {/if}
  </small>
</label>
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
  <small class="hint">
    Empty = use the OS user name, which often is your real first/last name.
    Set an explicit pseudonym for privacy on shared networks — every peer on
    the LAN sees this string in your HELLO broadcasts.
  </small>
</label>
<label>
  Destination directory
  <div class="dest-row">
    <code class="dest">{destDir || '(not set)'}</code>
    <button on:click={onPickDest}>Browse…</button>
  </div>
</label>
<label>
  Theme
  <div class="theme-row">
    <button
      type="button"
      class:active={themeMode === 'system'}
      on:click={() => onThemeModeChange('system')}
    >System</button>
    <button
      type="button"
      class:active={themeMode === 'light'}
      on:click={() => onThemeModeChange('light')}
    >Light</button>
    <button
      type="button"
      class:active={themeMode === 'dark'}
      on:click={() => onThemeModeChange('dark')}
    >Dark</button>
  </div>
  <small class="hint">
    System follows your OS dark/light setting. Light/Dark force a specific theme regardless of the OS.
  </small>
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
{#if fingerprint}
  <label>
    Identity fingerprint
    <div class="dest-row">
      <code class="dest" title={fingerprint}>{fingerprint}</code>
      <button class="ghost" type="button" on:click={copyFingerprint}>Copy</button>
    </div>
    <small class="hint">
      A long-term Ed25519 key generated for this install. Used today only as a stable
      ID; will anchor encrypted transfers in a future release. Different from the buddy
      name — surviving a rename. See docs/SECURITY_v2.md.
    </small>
  </label>
{/if}
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
  .row {
    display: flex;
    gap: 8px;
    align-items: center;
  }
  .avatar-row {
    display: flex;
    gap: 12px;
    align-items: center;
    margin-top: 4px;
  }
  .avatar-preview {
    width: 56px;
    height: 56px;
    border-radius: 8px;
    object-fit: cover;
    background: var(--code-bg);
    border: 1px solid var(--panel-border);
    flex-shrink: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    color: var(--text-faint);
    font-size: 1.2rem;
  }
  .avatar-preview.placeholder {
    line-height: 56px;
    text-align: center;
  }
  .avatar-actions {
    display: flex;
    flex-direction: column;
    gap: 6px;
    align-items: flex-start;
  }
  .avatar-actions button {
    padding: 4px 12px;
    font-size: 0.85rem;
  }
  .avatar-actions button.ghost {
    background: transparent;
    color: var(--accent);
  }
  .hint {
    display: block;
    margin-top: 6px;
    font-size: 0.75rem;
    color: var(--text-dim);
  }
  .theme-row {
    display: flex;
    gap: 6px;
    margin-top: 4px;
  }
  .theme-row button {
    /* Outlined "ghost" tone unless active. Visually mirrors the chip-row
       on the Android settings panel so the two apps feel related. */
    background: transparent;
    color: var(--accent);
    border: 1px solid var(--accent);
    padding: 4px 12px;
    font-size: 0.85rem;
  }
  .theme-row button.active {
    background: var(--accent);
    color: var(--header-text);
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
    background: var(--code-bg);
    border-radius: 4px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .addrs-title {
    font-size: 0.75rem;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--text-dim);
    margin-bottom: 4px;
  }
  .qr {
    margin-top: 12px;
    padding-top: 8px;
    border-top: 1px solid var(--panel-border);
    text-align: center;
  }
  .qr img {
    margin-top: 6px;
    max-width: 220px;
    image-rendering: pixelated;
  }
</style>
