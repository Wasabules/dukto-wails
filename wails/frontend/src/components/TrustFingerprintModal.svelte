<script lang="ts">
  // Confirmation gate before pinning a peer's fingerprint without any
  // out-of-band verification (the "Trust fingerprint as-is" / TOFU flow).
  //
  // The user has to read why TOFU is the weaker option and explicitly
  // confirm — accidentally pinning the wrong key on a contested LAN is
  // exactly the failure mode this prevents.

  import { parseSignature, type Peer } from '../lib/dukto';

  export let peer: Peer;
  export let onClose: () => void = () => {};
  export let onConfirm: () => void = () => {};

  $: parsed = parseSignature(peer.signature);
</script>

<div class="modal-backdrop" on:click|self={onClose}>
  <div class="modal" role="dialog" aria-modal="true">
    <h2>⚠️ Trust this fingerprint without verification?</h2>

    <p>
      You're about to pin <strong>{parsed.user || peer.signature}</strong>'s
      current Ed25519 fingerprint
      <code class="fp">{peer.fingerprint || '—'}</code>
      without any out-of-band check. This is called <em>TOFU</em>
      (Trust-On-First-Use).
    </p>

    <h3>Why this is weaker than the 5-word code</h3>
    <ul>
      <li>
        <strong>No protection against a first-contact MitM.</strong>
        Anyone impersonating this peer on the LAN <em>right now</em> would
        become the identity you trust forever. There's no way to tell after
        the fact.
      </li>
      <li>
        <strong>One-sided trust.</strong> Pinning here only changes how
        <em>your</em> outbound transfers are encrypted. For the peer to talk
        encrypted back, they need to pin <em>your</em> fingerprint too —
        either with the same TOFU button on their side, or with the 5-word
        code flow.
      </li>
      <li>
        <strong>"Refuse cleartext" mode requires both peers paired.</strong>
        If you turn that toggle on, only mutually-paired peers can
        communicate. TOFU on one side alone won't unblock the channel.
      </li>
    </ul>

    <h3>Recommended</h3>
    <p>
      Use the <strong>Pair via 5-word code</strong> action instead. Both
      peers derive the same one-shot PSK from the passphrase; the Noise
      XXpsk2 handshake authenticates the keys mutually before either
      side commits a pin, so a MitM can't stand in.
    </p>

    <div class="actions">
      <button class="primary" type="button" on:click={onConfirm}>
        I understand — trust anyway
      </button>
      <button class="secondary" type="button" on:click={onClose}>
        Cancel
      </button>
    </div>
  </div>
</div>

<style>
  .modal-backdrop {
    position: fixed; inset: 0;
    background: rgba(0, 0, 0, 0.5);
    display: flex; align-items: center; justify-content: center;
    z-index: 150;
  }
  .modal {
    background: var(--panel-bg);
    color: var(--text);
    border: 1px solid var(--panel-border);
    border-radius: 8px;
    padding: 18px 22px;
    width: min(540px, 92vw);
    max-height: 90vh;
    overflow-y: auto;
    box-shadow: 0 8px 24px rgba(0, 0, 0, 0.3);
  }
  h2 { margin: 0 0 10px; font-size: 1.05rem; color: var(--warn, #d39e00); }
  h3 { margin: 14px 0 4px; font-size: 0.92rem; }
  p { margin: 6px 0; font-size: 0.92rem; line-height: 1.45; }
  ul { margin: 4px 0 4px 1em; padding-left: 0; font-size: 0.9rem; line-height: 1.45; }
  ul li { margin: 4px 0; }
  .fp {
    display: inline-block;
    margin: 0 4px;
    padding: 0 4px;
    background: var(--code-bg);
    border-radius: 4px;
    font-size: 0.85rem;
    user-select: all;
  }
  .actions {
    display: flex; gap: 8px; margin-top: 16px;
  }
  .actions button {
    flex: 1;
    padding: 8px;
    border-radius: 6px;
    cursor: pointer;
    font-size: 0.92rem;
  }
  .actions .primary {
    border: 1px solid var(--warn, #d39e00);
    background: var(--warn, #d39e00);
    color: #000;
  }
  .actions .secondary {
    border: 1px solid var(--panel-border);
    background: transparent;
    color: var(--text);
  }
</style>
