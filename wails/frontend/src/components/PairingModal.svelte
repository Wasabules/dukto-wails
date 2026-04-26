<script lang="ts">
  // Minimal Noise XXpsk2 pairing modal. Both peers can either generate a
  // 5-word EFF passphrase (responder) or type one a peer just generated
  // (initiator). On success the backend auto-pins the peer's long-term
  // fingerprint, so subsequent transfers run encrypted with no further
  // prompts.

  import { startPairing, cancelPairing, pairWithPassphrase, pairingCodeQR, type Peer } from '../lib/dukto';

  export let peer: Peer;
  export let onClose: () => void = () => {};

  let mode: 'menu' | 'generate' | 'enter' = 'menu';
  let generated: string = '';
  let qrDataUrl: string = '';
  let entered: string = '';
  let busy = false;
  let error: string = '';

  async function generate() {
    busy = true; error = '';
    try {
      generated = await startPairing();
      mode = 'generate';
      // Best-effort: ask the backend for a QR encoding so the Android
      // pairing dialog can scan it instead of typing 5 words. Failure
      // is non-fatal — we still show the text fallback.
      try {
        qrDataUrl = await pairingCodeQR(generated);
      } catch {
        qrDataUrl = '';
      }
    } catch (err: any) {
      error = String(err?.message ?? err);
    } finally {
      busy = false;
    }
  }

  async function submitCode() {
    if (!entered.trim()) return;
    busy = true; error = '';
    try {
      const addrPort = `${peer.address}:${peer.port}`;
      await pairWithPassphrase(addrPort, entered);
      onClose();
    } catch (err: any) {
      error = String(err?.message ?? err);
    } finally {
      busy = false;
    }
  }

  function cancel() {
    if (mode === 'generate') {
      void cancelPairing();
    }
    onClose();
  }
</script>

<div class="modal-backdrop" on:click|self={cancel}>
  <div class="modal" role="dialog" aria-modal="true">
    <h2>Pair with {peer.signature}</h2>

    {#if mode === 'menu'}
      <p>Both peers must enter the same 5-word code. Either generate one and read
      it out, or type the code the other peer just generated.</p>
      <div class="actions">
        <button on:click={generate} disabled={busy}>Generate a code</button>
        <button on:click={() => (mode = 'enter')} disabled={busy}>Enter a code</button>
      </div>
    {:else if mode === 'generate'}
      <p>Read this code out to the other peer (valid 60 seconds):</p>
      <pre class="code">{generated}</pre>
      {#if qrDataUrl}
        <div class="qr">
          <img src={qrDataUrl} alt="Pairing QR code" />
          <p class="hint">…or scan with the other peer's Dukto camera.</p>
        </div>
      {/if}
      <p class="hint">Once their handshake lands the badge on this peer flips to 🔒.</p>
    {:else if mode === 'enter'}
      <p>Type the 5-word code the other peer just generated:</p>
      <input
        bind:value={entered}
        placeholder="apple-tiger-river-ocean-music"
        autofocus
        on:keydown={(e) => { if (e.key === 'Enter') submitCode(); }}
      />
      <div class="actions">
        <button on:click={submitCode} disabled={busy || !entered.trim()}>Pair</button>
      </div>
    {/if}

    {#if error}
      <p class="error">{error}</p>
    {/if}

    <button class="close" on:click={cancel}>Close</button>
  </div>
</div>

<style>
  .modal-backdrop {
    position: fixed; inset: 0;
    background: rgba(0, 0, 0, 0.45);
    display: flex; align-items: center; justify-content: center;
    z-index: 100;
  }
  .modal {
    background: var(--panel-bg);
    color: var(--text);
    border: 1px solid var(--panel-border);
    border-radius: 8px;
    padding: 18px 22px;
    width: min(440px, 92vw);
    box-shadow: 0 8px 24px rgba(0, 0, 0, 0.25);
  }
  h2 { margin: 0 0 12px; font-size: 1rem; }
  p { margin: 6px 0; font-size: 0.92rem; }
  .actions { display: flex; gap: 8px; margin: 12px 0; }
  .actions button { flex: 1; padding: 8px; border-radius: 6px; cursor: pointer;
    border: 1px solid var(--accent); background: var(--accent); color: var(--accent-on); }
  .code {
    background: var(--code-bg);
    color: var(--text);
    border-radius: 6px;
    padding: 12px;
    font-size: 1.15rem;
    font-weight: 600;
    text-align: center;
    user-select: all;
  }
  input {
    width: 100%; padding: 8px; border-radius: 6px;
    border: 1px solid var(--input-border);
    background: var(--input-bg); color: var(--text);
    font-family: monospace;
  }
  .hint { color: var(--text-faint); font-size: 0.85rem; }
  .qr { display: flex; flex-direction: column; align-items: center; gap: 4px; margin: 10px 0; }
  .qr img { width: 220px; height: 220px; image-rendering: pixelated; background: #fff; padding: 8px; border-radius: 6px; }
  .error { color: var(--danger); font-size: 0.85rem; }
  .close {
    margin-top: 8px;
    padding: 6px 12px;
    border-radius: 6px;
    border: 1px solid var(--panel-border);
    background: transparent;
    color: var(--text);
    cursor: pointer;
  }
</style>
