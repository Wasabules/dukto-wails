<script lang="ts">
  // TOFU mismatch alert. Triggered when an inbound v2 handshake produces
  // a remote_static that doesn't match the X25519 derived from the peer's
  // already-pinned Ed25519 fingerprint — i.e. the peer ID changed since
  // last time we paired with that LAN address.
  //
  // Two ways to react:
  //   - Reject: keep the existing pinning, treat the new key as hostile.
  //   - Re-pair: open the pairing modal so the user can verify the new
  //              key out-of-band via the 5-word passphrase.

  import { unpinPeer, type TOFUMismatch } from '../lib/dukto';

  export let mismatch: TOFUMismatch;
  export let onClose: () => void = () => {};
  export let onRepair: () => void = () => {};

  async function unpinAndClose() {
    try {
      await unpinPeer(mismatch.oldFingerprint);
    } catch (err) {
      console.warn('unpin failed', err);
    }
    onClose();
  }
</script>

<div class="modal-backdrop" on:click|self={onClose}>
  <div class="modal" role="dialog" aria-modal="true">
    <h2>⚠️ Identity changed</h2>
    <p>
      Peer <strong>{mismatch.label || mismatch.address}</strong>
      ({mismatch.address}) just presented a different long-term key
      than the one you previously paired with.
    </p>
    <dl class="fps">
      <dt>Pinned</dt>
      <dd><code>{mismatch.oldFingerprint}</code></dd>
      <dt>New key</dt>
      <dd><code>{mismatch.newFingerprint}</code></dd>
    </dl>
    <p class="hint">
      This usually means the peer reinstalled Dukto or the key file was
      reset. It can also mean someone is impersonating them on the LAN.
      Verify the new fingerprint with the peer out-of-band before trusting it.
    </p>
    <div class="actions">
      <button class="primary" type="button" on:click={onRepair}>Re-pair (verify with code)</button>
      <button class="secondary" type="button" on:click={unpinAndClose}>Unpin &amp; reject</button>
    </div>
    <button class="close" type="button" on:click={onClose}>Close</button>
  </div>
</div>

<style>
  .modal-backdrop {
    position: fixed; inset: 0;
    background: rgba(0, 0, 0, 0.55);
    display: flex; align-items: center; justify-content: center;
    z-index: 200;
  }
  .modal {
    background: var(--panel-bg);
    color: var(--text);
    border: 1px solid var(--danger, #c0392b);
    border-radius: 8px;
    padding: 18px 22px;
    width: min(480px, 92vw);
    box-shadow: 0 8px 24px rgba(0, 0, 0, 0.35);
  }
  h2 { margin: 0 0 10px; font-size: 1.05rem; color: var(--danger, #c0392b); }
  p { margin: 6px 0; font-size: 0.92rem; }
  dl.fps {
    margin: 10px 0;
    display: grid;
    grid-template-columns: max-content 1fr;
    gap: 4px 10px;
  }
  dl.fps dt { font-weight: 600; font-size: 0.85rem; color: var(--text-dim); }
  dl.fps dd { margin: 0; }
  dl.fps code { font-size: 0.85rem; user-select: all; }
  .hint { color: var(--text-dim); font-size: 0.85rem; }
  .actions { display: flex; gap: 8px; margin: 14px 0 6px; }
  .actions button { flex: 1; padding: 8px; border-radius: 6px; cursor: pointer;
    border: 1px solid var(--accent); background: var(--accent); color: var(--accent-on); }
  .actions .secondary { background: transparent; color: var(--text); border-color: var(--panel-border); }
  .close {
    margin-top: 4px;
    padding: 6px 12px;
    border-radius: 6px;
    border: 1px solid var(--panel-border);
    background: transparent;
    color: var(--text);
    cursor: pointer;
  }
</style>
