<script lang="ts">
  import { onDestroy, onMount } from 'svelte';
  import {
    addManualPeer as rpcAddManualPeer,
    addWhitelist as rpcAddWhitelist,
    aliases as fetchAliases,
    allowedInterfaces as fetchAllowedInterfaces,
    auditEntries as fetchAuditEntries,
    availableInterfaces as fetchAvailableInterfaces,
    blockedPeers as fetchBlockedPeers,
    buddyName,
    cancelTransfer,
    clearAudit as rpcClearAudit,
    clearHistory,
    closeToTray as fetchCloseToTray,
    confirmUnknownPeers as fetchConfirmUnknownPeers,
    copyToClipboard,
    destDir,
    exportHistory as rpcExportHistory,
    fileHash,
    forgetApprovedPeers as rpcForgetApprovedPeers,
    history as fetchHistory,
    idleAutoDisableMinutes as fetchIdleAutoDisableMinutes,
    largeFileThresholdMB as fetchLargeFileThresholdMB,
    localAddresses as fetchLocalAddresses,
    manualPeers as fetchManualPeers,
    maxFilesPerSession as fetchMaxFilesPerSession,
    maxPathDepth as fetchMaxPathDepth,
    minFreeDiskPercent as fetchMinFreeDiskPercent,
    notifications as fetchNotifications,
    openPath,
    parseSignature,
    peerKey,
    peers as fetchPeers,
    pickDestDir,
    pickExportPath,
    qrCodeSignature,
    receivingEnabled as fetchReceivingEnabled,
    rejectedExtensions as fetchRejectedExtensions,
    removeManualPeer as rpcRemoveManualPeer,
    removeWhitelist as rpcRemoveWhitelist,
    resolvePendingSession as rpcResolvePendingSession,
    revealInFolder,
    sendClipboard as rpcSendClipboard,
    sendFiles as rpcSendFiles,
    sendFilesMulti as rpcSendFilesMulti,
    sendText as rpcSendText,
    setAlias as rpcSetAlias,
    setAllowedInterfaces as rpcSetAllowedInterfaces,
    setBuddyName,
    setCloseToTray,
    setConfirmUnknownPeers as rpcSetConfirmUnknownPeers,
    setIdleAutoDisableMinutes as rpcSetIdleAutoDisableMinutes,
    setLargeFileThresholdMB as rpcSetLargeFileThresholdMB,
    setMaxFilesPerSession as rpcSetMaxFilesPerSession,
    setMaxPathDepth as rpcSetMaxPathDepth,
    setMinFreeDiskPercent as rpcSetMinFreeDiskPercent,
    setNotifications,
    setReceivingEnabled as rpcSetReceivingEnabled,
    setRejectedExtensions as rpcSetRejectedExtensions,
    setTCPAcceptCooldownSeconds as rpcSetTCPAcceptCooldownSeconds,
    setUDPHelloCooldownSeconds as rpcSetUDPHelloCooldownSeconds,
    setWhitelistEnabled as rpcSetWhitelistEnabled,
    signature as fetchSignature,
    stashPastedImage,
    tcpAcceptCooldownSeconds as fetchTCPAcceptCooldownSeconds,
    type AuditEntry,
    type NetIfaceView,
    type Peer,
    udpHelloCooldownSeconds as fetchUDPHelloCooldownSeconds,
    unblockPeer as rpcUnblockPeer,
    whitelist as fetchWhitelist,
    whitelistEnabled as fetchWhitelistEnabled,
  } from './lib/dukto';
  import { OnFileDrop as WailsOnFileDrop, OnFileDropOff as WailsOnFileDropOff } from '../wailsjs/runtime/runtime.js';

  import Header from './components/Header.svelte';
  import PeerList from './components/PeerList.svelte';
  import Composer from './components/Composer.svelte';
  import ReceivedList from './components/ReceivedList.svelte';
  import type { ReceivedItem } from './components/ReceivedRow.svelte';
  import SettingsModal, { type SettingsTab } from './components/SettingsModal.svelte';
  import PreviewModal from './components/PreviewModal.svelte';
  import PendingSessionModal from './components/PendingSessionModal.svelte';
  import ProgressStack from './components/ProgressStack.svelte';
  import Toast from './components/Toast.svelte';

  import { wireEvents } from './lib/events';
  import {
    broadcastMode,
    broadcastSelected,
    lastSeen,
    peerList,
    peersByKey,
    selectedKey,
    sortedPeers,
    upsertPeer,
  } from './lib/stores/peers';
  import {
    cacheHash,
    clearReceived,
    hashByPath,
    pushHistory,
    received,
  } from './lib/stores/received';
  import { now, stopNowClock } from './lib/stores/now';
  import {
    receiveProgress,
    sendByPeer,
    sendProgress,
  } from './lib/stores/progress';
  import { receiving } from './lib/stores/receiving';
  import { pendingSession } from './lib/stores/pending';
  import { showToast, toast } from './lib/stores/toast';

  // Identity + settings mirror (single-owner state — stays local).
  let mySig = '';
  let myBuddy = '';
  let myDest = '';
  let notifyOn = false;
  let trayOn = false;
  let myAddrs: string[] = [];
  let wlEnabled = false;
  let wlList: string[] = [];
  let rejectExts: string[] = [];
  let largeMB = 0;
  let idleMinutes = 0;
  let manualList: string[] = [];
  let manualInput = '';
  let extInput = '';

  // New security state.
  let blockList: string[] = [];
  let confirmUnknown = false;
  let maxFiles = 0;
  let maxDepth = 0;
  let minDiskPct = 0;
  let tcpCooldown = 0;
  let udpCooldown = 0;
  let ifaces: NetIfaceView[] = [];
  let allowedIfaces: string[] = [];
  let auditRows: AuditEntry[] = [];

  // Compose.
  let composeText = '';
  let draggedFiles: string[] = [];

  // Aliases stay local because they feed component props directly.
  let aliases: Record<string, string> = {};

  // Preview lightbox + settings modal state.
  let previewItem: ReceivedItem | null = null;
  let settingsOpen = false;
  let settingsTab: SettingsTab = 'general';
  let qrData: string | null = null;
  $: if (settingsOpen && qrData === null) void loadQrCode();
  async function loadQrCode() {
    try {
      qrData = await qrCodeSignature();
    } catch (e) {
      qrData = null;
      showToast(`QR code failed: ${e}`);
    }
  }
  function invalidateQr() {
    qrData = null;
    if (settingsOpen) void loadQrCode();
  }

  function peerLabel(p: Peer): string {
    const alias = aliases[p.signature];
    if (alias) return alias;
    const ps = parseSignature(p.signature);
    return ps.user || p.address;
  }

  let unwire: (() => void) | null = null;

  onMount(async () => {
    let initialReceiving = true;
    [
      mySig, myBuddy, myDest, notifyOn, trayOn, myAddrs,
      aliases, wlEnabled, wlList, rejectExts, largeMB, manualList,
      initialReceiving, idleMinutes,
      blockList, confirmUnknown,
      maxFiles, maxDepth, minDiskPct,
      tcpCooldown, udpCooldown,
      ifaces, allowedIfaces,
    ] = await Promise.all([
      fetchSignature(),
      buddyName(),
      destDir(),
      fetchNotifications(),
      fetchCloseToTray(),
      fetchLocalAddresses(),
      fetchAliases(),
      fetchWhitelistEnabled(),
      fetchWhitelist(),
      fetchRejectedExtensions(),
      fetchLargeFileThresholdMB(),
      fetchManualPeers(),
      fetchReceivingEnabled(),
      fetchIdleAutoDisableMinutes(),
      fetchBlockedPeers(),
      fetchConfirmUnknownPeers(),
      fetchMaxFilesPerSession(),
      fetchMaxPathDepth(),
      fetchMinFreeDiskPercent(),
      fetchTCPAcceptCooldownSeconds(),
      fetchUDPHelloCooldownSeconds(),
      fetchAvailableInterfaces(),
      fetchAllowedInterfaces(),
    ]);
    receiving.set(initialReceiving);
    const initial = await fetchPeers();
    for (const p of initial) upsertPeer(p);

    try {
      const saved = await fetchHistory();
      for (const h of saved) pushHistory(h);
    } catch (e) {
      console.warn('dukto: failed to load history', e);
    }

    unwire = wireEvents(({ paths, x, y }) => {
      if (!paths || paths.length === 0) return;
      const hit = hitPeerAt(x, y);
      if (hit) {
        const key = peerKey(hit);
        rpcSendFiles(key, paths)
          .then(() => showToast(`Sending ${paths.length} item(s) to ${peerLabel(hit)}…`))
          .catch((e) => showToast(`Failed: ${e}`));
        return;
      }
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
    });

    WailsOnFileDrop(() => {}, true);
  });

  function hitPeerAt(x: number, y: number): Peer | null {
    if (typeof x !== 'number' || typeof y !== 'number') return null;
    const el = document.elementFromPoint(x, y);
    if (!el) return null;
    const li = (el as HTMLElement).closest('li[data-peer-key]') as HTMLElement | null;
    if (!li) return null;
    const key = li.dataset.peerKey;
    if (!key) return null;
    return $peersByKey.get(key) ?? null;
  }

  onDestroy(() => {
    if (unwire) unwire();
    stopNowClock();
    WailsOnFileDropOff();
  });

  $: selectedPeer = $selectedKey ? $peersByKey.get($selectedKey) ?? null : null;

  function openPreview(r: ReceivedItem) {
    if (r.kind !== 'file') return;
    if (r.media === 'other') {
      void openReceived(r.path);
      return;
    }
    previewItem = r;
  }
  function closePreview() {
    previewItem = null;
  }

  async function onClearHistory() {
    try {
      await clearHistory();
      clearReceived();
      showToast('History cleared.');
    } catch (e) {
      showToast(`Failed: ${e}`);
    }
  }

  async function onCancelTransfer() {
    try {
      await cancelTransfer();
    } catch (e) {
      showToast(`Cancel failed: ${e}`);
    }
  }

  function onKeyDown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      if ($sendProgress || $receiveProgress) {
        e.preventDefault();
        void onCancelTransfer();
        return;
      }
      if (previewItem) {
        e.preventDefault();
        closePreview();
        return;
      }
      if (settingsOpen) {
        e.preventDefault();
        settingsOpen = false;
        return;
      }
    }
    if ((e.ctrlKey || e.metaKey) && e.key === 'Enter' && composeText.trim() && selectedPeer) {
      e.preventDefault();
      void sendText();
      return;
    }
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      const t = e.target as HTMLElement | null;
      if (t && (t.tagName === 'TEXTAREA' || t.tagName === 'INPUT')) return;
      if ($sortedPeers.length === 0) return;
      e.preventDefault();
      const idx = $selectedKey ? $sortedPeers.findIndex((p) => peerKey(p) === $selectedKey) : -1;
      const delta = e.key === 'ArrowDown' ? 1 : -1;
      const next = (idx + delta + $sortedPeers.length) % $sortedPeers.length;
      selectedKey.set(peerKey($sortedPeers[next < 0 ? 0 : next]));
    }
  }

  // --- Aliases -------------------------------------------------------------

  async function promptRenamePeer(p: Peer) {
    const current = aliases[p.signature] ?? '';
    const next = window.prompt(`Local nickname for ${parseSignature(p.signature).user || p.address}`, current);
    if (next === null) return;
    try {
      await rpcSetAlias(p.signature, next);
      aliases = { ...aliases, [p.signature]: next };
      if (!next) delete aliases[p.signature];
      // Force a re-render of peer labels by re-publishing the map.
      peersByKey.update((m) => new Map(m));
    } catch (e) {
      showToast(`Failed: ${e}`);
    }
  }

  // --- Whitelist -----------------------------------------------------------

  async function toggleWhitelist(on: boolean) {
    if (on && wlList.length === 0) {
      if (!window.confirm('The allow-list is empty. Turning it on will block every incoming transfer until you add entries. Continue?')) {
        return;
      }
    }
    try {
      await rpcSetWhitelistEnabled(on);
      wlEnabled = on;
    } catch (e) {
      showToast(`Failed: ${e}`);
    }
  }

  async function trustPeer(p: Peer) {
    try {
      await rpcAddWhitelist(p.signature);
      if (!wlList.includes(p.signature)) wlList = [...wlList, p.signature];
      showToast(`${peerLabel(p)} added to allow-list.`);
    } catch (e) {
      showToast(`Failed: ${e}`);
    }
  }

  async function untrustSig(sig: string) {
    try {
      await rpcRemoveWhitelist(sig);
      wlList = wlList.filter((s) => s !== sig);
    } catch (e) {
      showToast(`Failed: ${e}`);
    }
  }

  // --- Extension policy / threshold ---------------------------------------

  async function addRejectExt() {
    const v = extInput.trim().toLowerCase().replace(/^\./, '');
    if (!v) return;
    if (rejectExts.includes(v)) { extInput = ''; return; }
    const next = [...rejectExts, v];
    try {
      await rpcSetRejectedExtensions(next);
      rejectExts = next;
      extInput = '';
    } catch (e) {
      showToast(`Failed: ${e}`);
    }
  }

  async function removeRejectExt(ext: string) {
    const next = rejectExts.filter((e) => e !== ext);
    try {
      await rpcSetRejectedExtensions(next);
      rejectExts = next;
    } catch (e) {
      showToast(`Failed: ${e}`);
    }
  }

  async function commitLargeMB() {
    const mb = Math.max(0, Math.floor(Number(largeMB) || 0));
    try {
      await rpcSetLargeFileThresholdMB(mb);
      largeMB = mb;
    } catch (e) {
      showToast(`Failed: ${e}`);
    }
  }

  async function commitIdleMinutes() {
    const mins = Math.max(0, Math.floor(Number(idleMinutes) || 0));
    try {
      await rpcSetIdleAutoDisableMinutes(mins);
      idleMinutes = mins;
    } catch (e) {
      showToast(`Failed: ${e}`);
    }
  }

  // --- Block list ---------------------------------------------------------

  async function unblockSig(sig: string) {
    try {
      await rpcUnblockPeer(sig);
      blockList = blockList.filter((s) => s !== sig);
    } catch (e) {
      showToast(`Failed: ${e}`);
    }
  }

  async function toggleConfirmUnknown(on: boolean) {
    try {
      await rpcSetConfirmUnknownPeers(on);
      confirmUnknown = on;
    } catch (e) {
      showToast(`Failed: ${e}`);
    }
  }

  async function forgetApprovals() {
    try {
      await rpcForgetApprovedPeers();
      showToast('Forgot all approvals.');
    } catch (e) {
      showToast(`Failed: ${e}`);
    }
  }

  // --- Limits -------------------------------------------------------------

  async function commitMaxFiles() {
    const n = Math.max(0, Math.floor(Number(maxFiles) || 0));
    try {
      await rpcSetMaxFilesPerSession(n);
      maxFiles = n;
    } catch (e) {
      showToast(`Failed: ${e}`);
    }
  }

  async function commitMaxDepth() {
    const n = Math.max(0, Math.floor(Number(maxDepth) || 0));
    try {
      await rpcSetMaxPathDepth(n);
      maxDepth = n;
    } catch (e) {
      showToast(`Failed: ${e}`);
    }
  }

  async function commitMinDiskPct() {
    const n = Math.max(0, Math.floor(Number(minDiskPct) || 0));
    try {
      await rpcSetMinFreeDiskPercent(n);
      minDiskPct = n;
    } catch (e) {
      showToast(`Failed: ${e}`);
    }
  }

  async function commitTCPCooldown() {
    const n = Math.max(0, Math.floor(Number(tcpCooldown) || 0));
    try {
      await rpcSetTCPAcceptCooldownSeconds(n);
      tcpCooldown = n;
    } catch (e) {
      showToast(`Failed: ${e}`);
    }
  }

  async function commitUDPCooldown() {
    const n = Math.max(0, Math.floor(Number(udpCooldown) || 0));
    try {
      await rpcSetUDPHelloCooldownSeconds(n);
      udpCooldown = n;
    } catch (e) {
      showToast(`Failed: ${e}`);
    }
  }

  // --- Interfaces ---------------------------------------------------------

  async function toggleIface(name: string, on: boolean) {
    const next = on
      ? (allowedIfaces.includes(name) ? allowedIfaces : [...allowedIfaces, name])
      : allowedIfaces.filter((n) => n !== name);
    try {
      await rpcSetAllowedInterfaces(next);
      allowedIfaces = next;
    } catch (e) {
      showToast(`Failed: ${e}`);
    }
  }

  // --- Audit --------------------------------------------------------------

  async function refreshAudit() {
    try {
      auditRows = (await fetchAuditEntries()) ?? [];
    } catch (e) {
      showToast(`Failed: ${e}`);
    }
  }

  async function clearAuditLog() {
    try {
      await rpcClearAudit();
      auditRows = [];
    } catch (e) {
      showToast(`Failed: ${e}`);
    }
  }

  $: if (settingsOpen && settingsTab === 'audit') void refreshAudit();

  // --- Pending session resolution ----------------------------------------

  async function resolvePending(id: string, allow: boolean) {
    try {
      await rpcResolvePendingSession(id, allow);
    } catch (e) {
      showToast(`Failed: ${e}`);
    }
    pendingSession.set(null);
  }

  async function toggleReceiving() {
    const next = !$receiving;
    try {
      await rpcSetReceivingEnabled(next);
      // The backend emits `receiving:changed`, which wireEvents() pushes
      // into the store — set it here too so the pill flips without
      // waiting for the round-trip.
      receiving.set(next);
      showToast(next ? 'Receiving enabled.' : 'Receiving disabled.');
    } catch (e) {
      showToast(`Failed: ${e}`);
    }
  }

  // --- Manual peers --------------------------------------------------------

  async function addManual() {
    const v = manualInput.trim();
    if (!v) return;
    try {
      await rpcAddManualPeer(v);
      if (!manualList.includes(v)) manualList = [...manualList, v];
      manualInput = '';
      showToast(`Probing ${v}…`);
    } catch (e) {
      showToast(`Failed: ${e}`);
    }
  }

  async function removeManual(addr: string) {
    try {
      await rpcRemoveManualPeer(addr);
      manualList = manualList.filter((a) => a !== addr);
    } catch (e) {
      showToast(`Failed: ${e}`);
    }
  }

  // --- Hash -----------------------------------------------------------------

  async function showHash(path: string) {
    const cached = $hashByPath.get(path);
    if (cached) {
      void copyToClipboard(cached);
      showToast('Hash copied.');
      return;
    }
    showToast('Hashing…');
    try {
      const h = await fileHash(path);
      cacheHash(path, h);
      await copyToClipboard(h);
      showToast('SHA-256 copied to clipboard.');
    } catch (e) {
      showToast(`Hash failed: ${e}`);
    }
  }

  // --- Export ---------------------------------------------------------------

  async function exportAs(format: 'csv' | 'json') {
    try {
      const path = await pickExportPath(format);
      if (!path) return;
      await rpcExportHistory(format, path);
      showToast(`Exported to ${path}.`);
    } catch (e) {
      showToast(`Export failed: ${e}`);
    }
  }

  // --- Broadcast ------------------------------------------------------------

  function toggleBroadcastPick(key: string) {
    broadcastSelected.update((s) => {
      const next = new Set(s);
      if (next.has(key)) next.delete(key); else next.add(key);
      return next;
    });
  }

  async function sendFilesBroadcast() {
    if ($broadcastSelected.size === 0 || draggedFiles.length === 0) return;
    try {
      await rpcSendFilesMulti(Array.from($broadcastSelected), draggedFiles);
      showToast(`Fan-out to ${$broadcastSelected.size} peer(s).`);
      draggedFiles = [];
    } catch (e) {
      showToast(`Failed: ${e}`);
    }
  }

  // --- Paste image ---------------------------------------------------------

  async function onComposePaste(event: ClipboardEvent) {
    const items = event.clipboardData?.items;
    if (!items) return;
    for (const item of items) {
      if (item.kind !== 'file' || !item.type.startsWith('image/')) continue;
      const file = item.getAsFile();
      if (!file) continue;
      event.preventDefault();
      try {
        const dataUrl = await blobToDataUrl(file);
        const ext = (item.type.split('/')[1] || 'png').split(';')[0];
        const path = await stashPastedImage(dataUrl, ext);
        if (!draggedFiles.includes(path)) draggedFiles = [...draggedFiles, path];
        showToast('Pasted image queued.');
      } catch (e) {
        showToast(`Paste failed: ${e}`);
      }
      return;
    }
  }

  function blobToDataUrl(b: Blob): Promise<string> {
    return new Promise((resolve, reject) => {
      const fr = new FileReader();
      fr.onload = () => resolve(fr.result as string);
      fr.onerror = () => reject(fr.error);
      fr.readAsDataURL(b);
    });
  }

  // --- Send + open + reveal ------------------------------------------------

  async function sendClipboardNow() {
    if (!selectedPeer) return;
    try {
      await rpcSendClipboard(peerKey(selectedPeer));
      showToast('Clipboard sent.');
    } catch (e) {
      showToast(`Failed: ${e}`);
    }
  }

  async function sendText() {
    if (!selectedPeer || !composeText.trim()) return;
    try {
      await rpcSendText(peerKey(selectedPeer), composeText);
      showToast(`Text sent to ${peerLabel(selectedPeer)}.`);
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

  async function openReceived(path: string) {
    try {
      await openPath(path);
    } catch (e) {
      showToast(`Open failed: ${e}`);
    }
  }

  async function revealReceived(path: string) {
    try {
      await revealInFolder(path);
    } catch (e) {
      showToast(`Reveal failed: ${e}`);
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

  // --- Settings callbacks --------------------------------------------------

  async function commitBuddyName() {
    await setBuddyName(myBuddy);
    mySig = await fetchSignature();
    invalidateQr();
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

  async function toggleNotifications(on: boolean) {
    try {
      await setNotifications(on);
      notifyOn = on;
    } catch (e) {
      showToast(`Failed: ${e}`);
    }
  }

  async function toggleTray(on: boolean) {
    try {
      await setCloseToTray(on);
      trayOn = on;
    } catch (e) {
      showToast(`Failed: ${e}`);
    }
  }
</script>

<svelte:window on:keydown={onKeyDown} />

<main>
  <Header
    signature={mySig}
    addresses={myAddrs}
    receivingEnabled={$receiving}
    onToggleReceiving={toggleReceiving}
    onOpenSettings={() => (settingsOpen = !settingsOpen)}
  />

  <PeerList
    peers={$sortedPeers}
    selectedKey={$selectedKey}
    broadcastMode={$broadcastMode}
    broadcastSelected={$broadcastSelected}
    sendByPeer={$sendByPeer}
    whitelistEnabled={wlEnabled}
    whitelist={wlList}
    {aliases}
    now={$now}
    lastSeen={$lastSeen}
    onSelect={(k) => selectedKey.set(k)}
    onToggleBroadcastPick={toggleBroadcastPick}
    onRenamePeer={promptRenamePeer}
    onTrustPeer={trustPeer}
    onToggleBroadcastMode={(on) => broadcastMode.set(on)}
  />

  <Composer
    {selectedPeer}
    broadcastMode={$broadcastMode}
    broadcastSelectedCount={$broadcastSelected.size}
    {draggedFiles}
    {composeText}
    {aliases}
    onSendText={sendText}
    onSendClipboard={sendClipboardNow}
    onSendFiles={sendFiles}
    onSendFilesBroadcast={sendFilesBroadcast}
    onRemoveQueued={removeQueued}
    onClearQueued={clearQueued}
    onComposeTextChange={(v) => (composeText = v)}
    onComposePaste={onComposePaste}
  />

  <ReceivedList
    items={$received}
    peers={$peerList}
    {aliases}
    hashByPath={$hashByPath}
    onOpenPreview={openPreview}
    onOpenExternal={openReceived}
    onReveal={revealReceived}
    onCopyText={copyText}
    onHash={showHash}
    onExport={exportAs}
    onClear={onClearHistory}
  />

  {#if settingsOpen}
    <SettingsModal
      tab={settingsTab}
      buddyName={myBuddy}
      destDir={myDest}
      notificationsOn={notifyOn}
      {trayOn}
      {qrData}
      {wlEnabled}
      {wlList}
      {rejectExts}
      {largeMB}
      {extInput}
      {idleMinutes}
      {blockList}
      {confirmUnknown}
      {maxFiles}
      {maxDepth}
      {minDiskPct}
      {tcpCooldown}
      {udpCooldown}
      {manualList}
      {manualInput}
      {myAddrs}
      {ifaces}
      {allowedIfaces}
      {auditRows}
      onClose={() => (settingsOpen = false)}
      onTabChange={(t) => (settingsTab = t)}
      onBuddyNameChange={(v) => (myBuddy = v)}
      onCommitBuddyName={commitBuddyName}
      onPickDest={pickDest}
      onToggleNotifications={toggleNotifications}
      onToggleTray={toggleTray}
      onToggleWhitelist={toggleWhitelist}
      onUntrustSig={untrustSig}
      onAddRejectExt={addRejectExt}
      onRemoveRejectExt={removeRejectExt}
      onCommitLargeMB={commitLargeMB}
      onExtInputChange={(v) => (extInput = v)}
      onLargeMBChange={(mb) => (largeMB = mb)}
      onIdleMinutesChange={(m) => (idleMinutes = m)}
      onCommitIdleMinutes={commitIdleMinutes}
      onUnblockSig={unblockSig}
      onToggleConfirmUnknown={toggleConfirmUnknown}
      onForgetApprovals={forgetApprovals}
      onMaxFilesChange={(n) => (maxFiles = n)}
      onCommitMaxFiles={commitMaxFiles}
      onMaxDepthChange={(n) => (maxDepth = n)}
      onCommitMaxDepth={commitMaxDepth}
      onMinDiskPctChange={(n) => (minDiskPct = n)}
      onCommitMinDiskPct={commitMinDiskPct}
      onTCPCooldownChange={(n) => (tcpCooldown = n)}
      onCommitTCPCooldown={commitTCPCooldown}
      onUDPCooldownChange={(n) => (udpCooldown = n)}
      onCommitUDPCooldown={commitUDPCooldown}
      onAddManual={addManual}
      onRemoveManual={removeManual}
      onManualInputChange={(v) => (manualInput = v)}
      onToggleIface={toggleIface}
      onAuditRefresh={refreshAudit}
      onAuditClear={clearAuditLog}
    />
  {/if}

  {#if $pendingSession}
    <PendingSessionModal
      request={$pendingSession}
      onAllow={(id) => resolvePending(id, true)}
      onDeny={(id) => resolvePending(id, false)}
    />
  {/if}

  <ProgressStack
    sendProgress={$sendProgress}
    receiveProgress={$receiveProgress}
    now={$now}
    onCancel={onCancelTransfer}
  />

  <PreviewModal
    item={previewItem}
    onClose={closePreview}
    onOpenExternal={openReceived}
    onReveal={revealReceived}
  />

  <Toast message={$toast} />
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
</style>
