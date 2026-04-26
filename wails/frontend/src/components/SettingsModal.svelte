<script context="module" lang="ts">
  export type SettingsTab = 'general' | 'security' | 'limits' | 'network' | 'audit';
</script>

<script lang="ts">
  import type { AuditEntry, NetIfaceView } from '../lib/dukto';
  import GeneralTab from './settings/GeneralTab.svelte';
  import SecurityTab from './settings/SecurityTab.svelte';
  import LimitsTab from './settings/LimitsTab.svelte';
  import NetworkTab from './settings/NetworkTab.svelte';
  import AuditTab from './settings/AuditTab.svelte';

  export let tab: SettingsTab = 'general';

  // General
  export let buddyName = '';
  export let destDir = '';
  export let notificationsOn = false;
  export let trayOn = false;
  export let qrData: string | null = null;

  // Security
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
  export let pinned: import('../lib/dukto').PinnedPeer[] = [];

  // Limits
  export let maxFiles = 0;
  export let maxDepth = 0;
  export let minDiskPct = 0;
  export let tcpCooldown = 0;
  export let udpCooldown = 0;

  // Network
  export let manualList: string[] = [];
  export let manualInput = '';
  export let myAddrs: string[] = [];
  export let ifaces: NetIfaceView[] = [];
  export let allowedIfaces: string[] = [];

  // Audit
  export let auditRows: AuditEntry[] = [];

  export let onClose: () => void = () => {};
  export let onTabChange: (t: SettingsTab) => void = () => {};

  // General callbacks
  export let onBuddyNameChange: (v: string) => void = () => {};
  export let onCommitBuddyName: () => void = () => {};
  export let onPickDest: () => void = () => {};
  export let avatarUrl: string = '';
  export let hasCustomAvatar: boolean = false;
  export let onPickAvatar: () => void = () => {};
  export let onClearAvatar: () => void = () => {};
  export let themeMode: 'system' | 'light' | 'dark' = 'system';
  export let onThemeModeChange: (mode: 'system' | 'light' | 'dark') => void = () => {};
  export let fingerprint: string = '';
  export let onToggleNotifications: (on: boolean) => void = () => {};
  export let onToggleTray: (on: boolean) => void = () => {};

  // Security callbacks
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

  // Limits callbacks
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

  // Network callbacks
  export let onAddManual: () => void = () => {};
  export let onRemoveManual: (addr: string) => void = () => {};
  export let onManualInputChange: (v: string) => void = () => {};
  export let onToggleIface: (name: string, on: boolean) => void = () => {};

  // Audit callbacks
  export let onAuditRefresh: () => void = () => {};
  export let onAuditClear: () => void = () => {};
</script>

<!-- svelte-ignore a11y-click-events-have-key-events a11y-no-static-element-interactions -->
<div
  class="settings-backdrop"
  on:click|self={onClose}
  role="dialog"
  aria-modal="true"
  aria-label="Settings"
>
  <div class="settings-modal">
    <div class="settings-head">
      <h2>Settings</h2>
      <button class="mini ghost" type="button" title="Close (Esc)" on:click={onClose}>✕</button>
    </div>
    <div class="settings-tabs" role="tablist">
      <button
        type="button"
        role="tab"
        class="tab"
        class:active={tab === 'general'}
        aria-selected={tab === 'general'}
        on:click={() => onTabChange('general')}
      >General</button>
      <button
        type="button"
        role="tab"
        class="tab"
        class:active={tab === 'security'}
        aria-selected={tab === 'security'}
        on:click={() => onTabChange('security')}
      >Security</button>
      <button
        type="button"
        role="tab"
        class="tab"
        class:active={tab === 'limits'}
        aria-selected={tab === 'limits'}
        on:click={() => onTabChange('limits')}
      >Limits</button>
      <button
        type="button"
        role="tab"
        class="tab"
        class:active={tab === 'network'}
        aria-selected={tab === 'network'}
        on:click={() => onTabChange('network')}
      >Network</button>
      <button
        type="button"
        role="tab"
        class="tab"
        class:active={tab === 'audit'}
        aria-selected={tab === 'audit'}
        on:click={() => onTabChange('audit')}
      >Audit</button>
    </div>

    <div class="settings-body" role="tabpanel">
      {#if tab === 'general'}
        <GeneralTab
          {buddyName}
          {destDir}
          {notificationsOn}
          {trayOn}
          {qrData}
          {avatarUrl}
          {hasCustomAvatar}
          {themeMode}
          {fingerprint}
          {onBuddyNameChange}
          {onCommitBuddyName}
          {onPickDest}
          {onPickAvatar}
          {onClearAvatar}
          {onThemeModeChange}
          {onToggleNotifications}
          {onToggleTray}
        />
      {:else if tab === 'security'}
        <SecurityTab
          {wlEnabled}
          {wlList}
          {rejectExts}
          {largeMB}
          {extInput}
          {idleMinutes}
          {blockList}
          {confirmUnknown}
          {onToggleWhitelist}
          {onUntrustSig}
          {onAddRejectExt}
          {onRemoveRejectExt}
          {onCommitLargeMB}
          {onExtInputChange}
          {onLargeMBChange}
          {onIdleMinutesChange}
          {onCommitIdleMinutes}
          {onUnblockSig}
          {onToggleConfirmUnknown}
          {onForgetApprovals}
          {refuseCleartext}
          {hideFromDiscovery}
          {pinned}
          {onToggleRefuseCleartext}
          {onToggleHideFromDiscovery}
          {onUnpinPeer}
        />
      {:else if tab === 'limits'}
        <LimitsTab
          {maxFiles}
          {maxDepth}
          {minDiskPct}
          {tcpCooldown}
          {udpCooldown}
          {onMaxFilesChange}
          {onCommitMaxFiles}
          {onMaxDepthChange}
          {onCommitMaxDepth}
          {onMinDiskPctChange}
          {onCommitMinDiskPct}
          {onTCPCooldownChange}
          {onCommitTCPCooldown}
          {onUDPCooldownChange}
          {onCommitUDPCooldown}
        />
      {:else if tab === 'network'}
        <NetworkTab
          {manualList}
          {manualInput}
          {myAddrs}
          {ifaces}
          {allowedIfaces}
          {onAddManual}
          {onRemoveManual}
          {onManualInputChange}
          {onToggleIface}
        />
      {:else}
        <AuditTab
          entries={auditRows}
          onRefresh={onAuditRefresh}
          onClear={onAuditClear}
        />
      {/if}
    </div>
  </div>
</div>

<style>
  .settings-backdrop {
    position: fixed;
    inset: 0;
    background: rgba(15, 23, 42, 0.45);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 40;
    padding: 24px;
  }
  .settings-modal {
    background: var(--panel-bg);
    border-radius: 8px;
    box-shadow: 0 20px 50px rgba(15, 23, 42, 0.25);
    width: min(640px, 100%);
    max-height: 90vh;
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }
  .settings-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 12px 16px;
    border-bottom: 1px solid var(--panel-border);
  }
  .settings-head h2 {
    margin: 0;
    font-size: 1rem;
  }
  .settings-tabs {
    display: flex;
    gap: 2px;
    padding: 0 16px;
    border-bottom: 1px solid var(--panel-border);
    background: var(--panel-bg-2);
    flex-wrap: wrap;
  }
  .settings-tabs .tab {
    background: transparent;
    color: var(--text);
    border: 0;
    border-bottom: 2px solid transparent;
    border-radius: 0;
    padding: 10px 14px;
    font-weight: 500;
    cursor: pointer;
  }
  .settings-tabs .tab:hover {
    color: var(--text-strong);
    background: var(--accent-soft);
  }
  .settings-tabs .tab.active {
    color: var(--accent);
    border-bottom-color: var(--accent);
    background: var(--panel-bg);
  }
  .settings-body {
    padding: 16px 20px;
    overflow-y: auto;
  }
  .mini {
    padding: 0 6px;
    font-size: 0.9rem;
    line-height: 1;
    border-radius: 4px;
    cursor: pointer;
    border: 1px solid var(--accent);
    background: var(--accent);
    color: var(--accent-on);
  }
  .mini.ghost {
    background: transparent;
    color: var(--accent);
  }
</style>
