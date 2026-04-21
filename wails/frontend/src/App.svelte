<script lang="ts">
  import { onDestroy, onMount } from 'svelte';
  import {
    avatarUrl,
    buddyName,
    closeToTray as fetchCloseToTray,
    copyToClipboard,
    destDir,
    localAddresses as fetchLocalAddresses,
    notifications as fetchNotifications,
    onFileDrop,
    onPeerFound,
    onPeerGone,
    onReceiveDone,
    onReceiveError,
    onReceiveFile,
    onReceiveStart,
    onReceiveText,
    onSendError,
    parseSignature,
    peerKey,
    peers as fetchPeers,
    pickDestDir,
    sendFiles as rpcSendFiles,
    sendText as rpcSendText,
    setBuddyName,
    setCloseToTray,
    setNotifications,
    signature as fetchSignature,
    type Peer,
    type ReceivePayload,
  } from './lib/dukto';

  // Peer list. Keyed by "ip:port" so duplicates from repeated HELLO bursts
  // replace rather than stack.
  let peersByKey = new Map<string, Peer>();
  let peerList: Peer[] = [];
  let selectedKey: string | null = null;
  $: selectedPeer = selectedKey ? peersByKey.get(selectedKey) ?? null : null;

  let mySig = '';
  let myBuddy = '';
  let myDest = '';
  let notifyOn = false;
  let trayOn = false;
  let myAddrs: string[] = [];

  let composeText = '';
  let draggedFiles: string[] = [];

  interface ReceivedItem {
    id: number;
    kind: 'file' | 'text';
    name: string;
    path: string;
    text: string;
    at: Date;
  }
  let received: ReceivedItem[] = [];
  let nextReceivedId = 1;

  let toast: string | null = null;
  let toastTimer: ReturnType<typeof setTimeout> | null = null;
  function showToast(msg: string) {
    toast = msg;
    if (toastTimer) clearTimeout(toastTimer);
    toastTimer = setTimeout(() => (toast = null), 4000);
  }

  let settingsOpen = false;

  function hideBrokenAvatar(event: Event) {
    const img = event.currentTarget as HTMLImageElement;
    img.style.visibility = 'hidden';
  }

  const unsubs: Array<() => void> = [];

  onMount(async () => {
    [mySig, myBuddy, myDest, notifyOn, trayOn, myAddrs] = await Promise.all([
      fetchSignature(),
      buddyName(),
      destDir(),
      fetchNotifications(),
      fetchCloseToTray(),
      fetchLocalAddresses(),
    ]);
    const initial = await fetchPeers();
    for (const p of initial) peersByKey.set(peerKey(p), p);
    peerList = Array.from(peersByKey.values());

    unsubs.push(
      onPeerFound((p) => {
        peersByKey.set(peerKey(p), p);
        peerList = Array.from(peersByKey.values());
      }),
      onPeerGone((p) => {
        peersByKey.delete(peerKey(p));
        peerList = Array.from(peersByKey.values());
        if (selectedKey === peerKey(p)) selectedKey = null;
      }),
      onReceiveStart((p) => showToast(`Receiving ${p.total} element${p.total === 1 ? '' : 's'}…`)),
      onReceiveFile((p) => pushReceived(p, 'file')),
      onReceiveText((p) => pushReceived(p, 'text')),
      onReceiveDone(() => showToast('Transfer complete.')),
      onReceiveError((m) => showToast(`Receive error: ${m}`)),
      onSendError((m) => showToast(`Send error: ${m}`)),
      onFileDrop((paths) => {
        if (!paths || paths.length === 0) return;
        const seen = new Set(draggedFiles);
        const merged = draggedFiles.slice();
        for (const p of paths) {
          if (!seen.has(p)) {
            seen.add(p);
            merged.push(p);
          }
        }
        draggedFiles = merged;
        showToast(`Queued ${paths.length} item(s). Pick a peer and hit Send.`);
      }),
    );
  });

  onDestroy(() => {
    unsubs.forEach((fn) => fn());
  });

  function pushReceived(p: ReceivePayload, kind: 'file' | 'text') {
    received = [
      {
        id: nextReceivedId++,
        kind,
        name: p.name,
        path: p.path,
        text: p.text,
        at: new Date(),
      },
      ...received,
    ].slice(0, 50);
  }

  async function sendText() {
    if (!selectedPeer || !composeText.trim()) return;
    try {
      await rpcSendText(peerKey(selectedPeer), composeText);
      showToast(`Text sent to ${parseSignature(selectedPeer.signature).user}.`);
      composeText = '';
    } catch (e) {
      showToast(`Failed: ${e}`);
    }
  }

  async function sendFiles() {
    if (!selectedPeer || draggedFiles.length === 0) return;
    try {
      await rpcSendFiles(peerKey(selectedPeer), draggedFiles);
      showToast(`Sending ${draggedFiles.length} item(s).`);
      draggedFiles = [];
    } catch (e) {
      showToast(`Failed: ${e}`);
    }
  }

  function removeQueued(path: string) {
    draggedFiles = draggedFiles.filter((p) => p !== path);
  }

  function clearQueued() {
    draggedFiles = [];
  }

  async function commitBuddyName() {
    await setBuddyName(myBuddy);
    mySig = await fetchSignature();
    showToast('Buddy name updated.');
  }

  async function pickDest() {
    try {
      const picked = await pickDestDir();
      if (picked) {
        myDest = picked;
        showToast('Destination updated.');
      }
    } catch (e) {
      showToast(`Failed: ${e}`);
    }
  }

  async function toggleNotifications(event: Event) {
    const on = (event.currentTarget as HTMLInputElement).checked;
    try {
      await setNotifications(on);
      notifyOn = on;
    } catch (e) {
      showToast(`Failed: ${e}`);
    }
  }

  async function toggleTray(event: Event) {
    const on = (event.currentTarget as HTMLInputElement).checked;
    try {
      await setCloseToTray(on);
      trayOn = on;
    } catch (e) {
      showToast(`Failed: ${e}`);
    }
  }

  async function copyText(text: string) {
    try {
      await copyToClipboard(text);
      showToast('Copied to clipboard.');
    } catch (e) {
      showToast(`Failed: ${e}`);
    }
  }
</script>

<main>
  <header>
    <div class="me">
      <strong>{parseSignature(mySig).user || '…'}</strong>
      <span class="host">{parseSignature(mySig).host} · {parseSignature(mySig).platform}</span>
    </div>
    <button class="icon" on:click={() => (settingsOpen = !settingsOpen)}>⚙</button>
  </header>

  <section class="peers">
    <h2>Buddies on your network</h2>
    {#if peerList.length === 0}
      <p class="empty">No peers yet. Make sure Dukto is running on another device in the same LAN.</p>
    {:else}
      <ul>
        {#each peerList as p (peerKey(p))}
          {@const ps = parseSignature(p.signature)}
          <li class:selected={selectedKey === peerKey(p)}>
            <button
              type="button"
              class="peer-btn"
              on:click={() => (selectedKey = peerKey(p))}
            >
              <img src={avatarUrl(p)} alt="" on:error={hideBrokenAvatar} />
              <div class="who">
                <div class="name">{ps.user || p.address}</div>
                <div class="detail">{ps.host || p.address} · {ps.platform || '–'}</div>
              </div>
            </button>
          </li>
        {/each}
      </ul>
    {/if}
  </section>

  <section class="compose">
    <h2>Send</h2>
    {#if selectedPeer}
      <p class="target">
        To <strong>{parseSignature(selectedPeer.signature).user || selectedPeer.address}</strong>
      </p>
      <label>
        Text snippet
        <textarea bind:value={composeText} rows="4" placeholder="Type a message…"></textarea>
      </label>
      <button on:click={sendText} disabled={!composeText.trim()}>Send text</button>

      <div class="dropzone" class:has-items={draggedFiles.length > 0}>
        {#if draggedFiles.length === 0}
          <p>Drop files or folders here to queue them for sending.</p>
        {:else}
          <ul class="queued">
            {#each draggedFiles as path (path)}
              <li>
                <code>{path}</code>
                <button type="button" class="mini" on:click={() => removeQueued(path)}>×</button>
              </li>
            {/each}
          </ul>
        {/if}
      </div>
      <div class="row">
        <button on:click={sendFiles} disabled={draggedFiles.length === 0}>Send {draggedFiles.length || ''} item(s)</button>
        {#if draggedFiles.length > 0}
          <button class="ghost" type="button" on:click={clearQueued}>Clear</button>
        {/if}
      </div>
    {:else}
      <p class="empty">Pick a peer on the left to start a transfer.</p>
    {/if}
  </section>

  <section class="received">
    <h2>Received</h2>
    {#if received.length === 0}
      <p class="empty">Nothing yet. Incoming transfers will appear here.</p>
    {:else}
      <ul>
        {#each received as r (r.id)}
          <li>
            <span class="badge">{r.kind}</span>
            <span class="rname">{r.name}</span>
            {#if r.kind === 'text'}
              <blockquote>{r.text}</blockquote>
              <button class="mini ghost" type="button" on:click={() => copyText(r.text)}>Copy</button>
            {:else}
              <code>{r.path}</code>
            {/if}
            <time>{r.at.toLocaleTimeString()}</time>
          </li>
        {/each}
      </ul>
    {/if}
  </section>

  {#if settingsOpen}
    <div class="drawer" role="dialog">
      <h2>Settings</h2>
      <label>
        Buddy name
        <input bind:value={myBuddy} placeholder="Your display name" />
        <button on:click={commitBuddyName}>Save</button>
      </label>
      <label>
        Destination directory
        <div class="dest-row">
          <code class="dest">{myDest || '(not set)'}</code>
          <button on:click={pickDest}>Browse…</button>
        </div>
      </label>
      <label class="check">
        <input type="checkbox" checked={notifyOn} on:change={toggleNotifications} />
        Desktop notification on completed transfer
      </label>
      <label class="check">
        <input type="checkbox" checked={trayOn} on:change={toggleTray} />
        Keep running when window closes (relaunch to show)
      </label>
      {#if myAddrs.length > 0}
        <div class="addrs">
          <div class="addrs-title">Listening on</div>
          <ul>
            {#each myAddrs as a}
              <li><code>{a}</code></li>
            {/each}
          </ul>
        </div>
      {/if}
      <button class="close" on:click={() => (settingsOpen = false)}>Close</button>
    </div>
  {/if}

  {#if toast}
    <div class="toast" role="status">{toast}</div>
  {/if}
</main>

<style>
  main {
    display: grid;
    grid-template-areas:
      'head head'
      'peers compose'
      'peers received';
    grid-template-columns: 260px 1fr;
    grid-template-rows: auto 1fr 1fr;
    gap: 12px;
    padding: 12px;
    height: 100vh;
    box-sizing: border-box;
  }
  header {
    grid-area: head;
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 8px 12px;
    background: #1f2937;
    color: #e5e7eb;
    border-radius: 6px;
  }
  header .me strong {
    font-size: 1.1rem;
  }
  header .host {
    margin-left: 8px;
    opacity: 0.7;
  }
  header .icon {
    background: none;
    border: 0;
    color: inherit;
    font-size: 1.4rem;
    cursor: pointer;
  }
  section {
    background: #ffffff;
    border: 1px solid #e2e8f0;
    border-radius: 6px;
    padding: 10px 14px;
    overflow: auto;
  }
  section.peers {
    grid-area: peers;
  }
  section.compose {
    grid-area: compose;
  }
  section.received {
    grid-area: received;
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
  .peers ul,
  .received ul {
    list-style: none;
    padding: 0;
    margin: 0;
  }
  .peers li {
    list-style: none;
    padding: 0;
    margin: 0;
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
  .peers li.selected .peer-btn {
    background: #dbeafe;
  }
  .peers img {
    width: 40px;
    height: 40px;
    border-radius: 6px;
    background: #cbd5e1;
  }
  .peers .name {
    font-weight: 600;
  }
  .peers .detail {
    font-size: 0.8rem;
    color: #64748b;
  }
  .compose label {
    display: block;
    font-size: 0.85rem;
    color: #334155;
    margin: 8px 0;
  }
  textarea,
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
  button:disabled {
    opacity: 0.4;
    cursor: not-allowed;
  }
  .received li {
    padding: 6px 0;
    border-bottom: 1px solid #f1f5f9;
    display: grid;
    grid-template-columns: auto 1fr auto;
    gap: 8px;
    align-items: center;
  }
  .received .rname {
    font-weight: 600;
  }
  .received blockquote {
    grid-column: 1 / -1;
    margin: 4px 0 0;
    padding: 4px 8px;
    border-left: 3px solid #cbd5e1;
    color: #334155;
    white-space: pre-wrap;
  }
  .received code {
    grid-column: 1 / -1;
    font-size: 0.8rem;
    color: #475569;
  }
  .badge {
    font-size: 0.7rem;
    padding: 2px 6px;
    border-radius: 10px;
    background: #e2e8f0;
    text-transform: uppercase;
  }
  time {
    font-size: 0.75rem;
    color: #94a3b8;
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
  .drawer {
    position: fixed;
    right: 12px;
    top: 64px;
    width: 320px;
    background: #fff;
    border: 1px solid #cbd5e1;
    box-shadow: 0 8px 24px rgba(15, 23, 42, 0.12);
    border-radius: 6px;
    padding: 14px;
  }
  .drawer label {
    display: block;
    margin: 8px 0;
  }
  .drawer label input {
    margin: 4px 0;
  }
  .drawer label.check {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 0.9rem;
  }
  .drawer label.check input {
    width: auto;
    margin: 0;
  }
  .addrs {
    margin-top: 12px;
    padding-top: 8px;
    border-top: 1px solid #e2e8f0;
  }
  .addrs-title {
    font-size: 0.75rem;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: #64748b;
    margin-bottom: 4px;
  }
  .addrs ul {
    list-style: none;
    padding: 0;
    margin: 0;
  }
  .addrs li {
    font-size: 0.85rem;
    padding: 2px 0;
  }
  .drawer .close {
    background: #64748b;
    border-color: #64748b;
    margin-top: 12px;
  }
  .toast {
    position: fixed;
    bottom: 12px;
    left: 50%;
    transform: translateX(-50%);
    background: #0f172a;
    color: #f8fafc;
    padding: 8px 16px;
    border-radius: 20px;
    font-size: 0.9rem;
  }
</style>
