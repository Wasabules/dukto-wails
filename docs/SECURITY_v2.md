# Dukto v2 — encrypted overlay design

This document records the design for adding **end-to-end encryption** on top of the legacy Dukto LAN protocol while keeping full **back-compatibility** with the existing peers (Qt, third-party apps that don't speak v2). The implementation is split across three milestones (M1 → M3); the wire format described in [`PROTOCOL.md`](PROTOCOL.md) is **not** modified — v2 lives in additional, optional layers that older peers ignore.

Status: design frozen. M1 ✅, M2 ✅, M3a ✅ (Noise XX foundation), M3b ✅ (Wails wire-up + manual TOFU). M3c (Android Noise port) and M3d (EFF PSK pairing UI + audit column) still pending.

## Threat model

- **Trust boundary**: a single LAN where the user has already chosen to enable Dukto. We don't try to defend against a malicious user on the same device (root, malware, etc.).
- **Adversary**: a passive eavesdropper or active MitM on the same LAN (rogue Wi-Fi AP, evil twin, ARP spoofing, etc.).
- **What we protect**: confidentiality + integrity of file/text payloads, mutual authentication of paired devices, forward secrecy of past sessions.
- **What we don't try to do**: hide the fact that two devices ran Dukto on the LAN (discovery is opt-in cleartext by spec). Anyone sniffing UDP can still see "alice has a Dukto-capable device at 192.168.1.42".

## Crypto primitives

Same toolkit as WireGuard, Signal, and TLS 1.3:

| Purpose | Primitive |
|---|---|
| Long-term identity | Ed25519 |
| Key exchange (handshake) | X25519 (derived from Ed25519 keypair) |
| Symmetric encryption (transport) | ChaCha20-Poly1305 (AEAD) |
| Key derivation | HKDF-SHA-256 |
| Pairing PSK derivation | HKDF-SHA-256 with the EFF passphrase as IKM |
| Handshake pattern | **Noise XX** (mutual auth, both static keys exchanged in-band) with `psk2` modifier on first pairing |

Equivalent strength to TLS 1.3 (~128-bit security level on X25519 / ChaCha20). What differs is the **authentication model**: TOFU + optional out-of-band passphrase, no CA chain.

## Trust model

- **TOFU** (Trust-On-First-Use): on first encrypted session with a peer, its Ed25519 public key fingerprint is pinned. Subsequent sessions verify the fingerprint matches; mismatch → user-facing "Identity changed" modal (Signal-style safety number).
- **Optional pairing PSK**: to defeat the first-contact MitM window, two devices may bootstrap their pinning through a one-shot pre-shared passphrase. The PSK is **not** a long-term password — it exists for ~30 seconds, is mixed into the Noise handshake (via the `psk2` modifier), and is discarded once pinning succeeds.

## Identity

Each install holds a single Ed25519 keypair generated on first run.

| Platform | Storage |
|---|---|
| Wails (Go) | `<UserConfigDir>/dukto/identity.key` — file mode `0600`. Future: integrate OS keychains (DPAPI on Windows, Keychain on macOS, libsecret on Linux). |
| Android (Kotlin) | `filesDir/identity.key` wrapped by `androidx.security:security-crypto`'s `EncryptedFile` (AES-256-GCM, master key in AndroidKeyStore, hardware-backed where available). |

The fingerprint shown to users:

```
fingerprint = base32(sha256(pub_key)[0..10])  # 16 chars
```

formatted in groups of four for readability (`K6X4-MYZB-T3Q9-J7EW`). Surfaces in Settings → Profile, copyable. Eventually exportable as a QR code (M2/M3).

## Capability advertisement (M2)

To know which peers can speak v2 without breaking legacy parsers, two new UDP message types extend the discovery protocol:

| Type | Name | Carries |
|---|---|---|
| `0x06` | `HelloPortKeyBroadcast` | port (LE u16) ‖ pub_key (32 B) ‖ sig (64 B) ‖ utf-8 signature |
| `0x07` | `HelloPortKeyUnicast` | same as above, sent in reply to a `0x06` |

The signature `sig` is `Ed25519.sign(priv, port ‖ utf-8(signature_string))` — proves the holder of `pub_key` actually controls this port + name, prevents trivial spoofing of the announcement.

Legacy peers (Qt original) reject any type byte outside `0x01..0x05` and ignore the datagram silently — already spec'd in `PROTOCOL.md` §2.1. So a v2 peer broadcasts **both** `0x04` (legacy) **and** `0x06` (v2) every HELLO interval; legacy peers see the legacy one, v2 peers see both.

## Session upgrade (M3)

A v2 peer that knows the destination is also v2 (saw a `0x06` from it within the last few minutes) initiates a TCP session by sending a magic prefix instead of the legacy `SessionHeader`:

```
[8 bytes "DKTOv2\x00\x00"]   # magic
[Noise XX handshake, possibly XXpsk2 if first contact + PSK]
[Noise transport messages] = stream of legacy SessionHeader + ElementHeader + payload
```

The receiver `peek()`s the first 8 bytes:
- Match → enters Noise handshake mode, all bytes thereafter are framed inside Noise transport messages.
- No match → falls back to parsing as a legacy `SessionHeader`. Cleartext session, exactly as today.

This makes the upgrade **per-connection opt-in**: a v2 sender can talk both languages depending on who it's targeting, without the receiver having to decide upfront.

## Pairing flow (M3)

When sender and receiver have never paired (no pinned fingerprint for each other), three behaviours are possible:

| Setting | Behaviour |
|---|---|
| Default | Prompt: "Pair `bob@phone` to enable encrypted transfers." Two paths: **Pair now** (PSK flow) or **Send unencrypted** (warning). |
| "Refuse cleartext transfers" enabled | Same prompt without the cleartext escape hatch — paired-only. |
| Peer is v1 only (no `0x06` ever seen) | Banner: "Peer doesn't support encryption. Continue in cleartext?" — single click. |

The PSK pairing flow:

1. Device A taps "Pair new device" → generates 5 random EFF wordlist words (~50 bits entropy: e.g. `apple-tiger-river-ocean-music`) + matching QR.
2. Device B taps "I have a pairing code" → enters/scans the code.
3. B opens TCP to A, sends `DKTOv2\x00\x00` + Noise XXpsk2 handshake using `HKDF("DUKTO-PSK-v1", passphrase)` as the PSK.
4. Handshake completes only if A and B used the exact same passphrase (the PSK is mixed into the AEAD authentication — wrong PSK → first message MAC fails → connection drops).
5. On success, A and B have mutually authenticated each other's long-term Ed25519 pubkey. Both store the peer's fingerprint as **pinned/trusted** in their TOFU table.
6. The PSK is discarded.
7. Subsequent sessions use plain Noise XX (no PSK), authenticated by the pinned fingerprints.

### Why the PSK is one-shot

Once both sides have pinned each other's long-term key, the trust is permanent for that key. Re-using the PSK would only matter if a key is lost (factory reset, app reinstall, hardware change), in which case **a new pairing session is required** — a fresh PSK is generated. This avoids the "shared password reuse" failure mode and matches Magic Wormhole / WireGuard pairing flows.

### Mitigating offline brute force

Because the handshake messages are exchanged on the LAN, an attacker who captures them could try to brute-force the PSK offline. The 50-bit EFF wordlist gives ~13 days of GPU-cluster work to brute-force one pairing — sufficient for LAN-private use. Two future hardenings if more is wanted:

- **Switch to a true PAKE** (CPace or SPAKE2) — same UX, but offline brute-force becomes mathematically impossible. Costs: less mature library support in Go/Kotlin, more code to audit.
- **Rate-limit pairing attempts** — server-side; doesn't defeat offline attack on a captured handshake but does defeat live brute-force.

## Discovery stays public

The UDP discovery layer is **not encrypted**. v1 and v2 peers all keep appearing in each other's peer lists — that's the spec, and it's how Dukto stays interoperable. What changes in v2 is the UI affordance:

```
┌─ Buddies on your network ────────────┐
│ 🔒  alice@laptop (Linux)         •  │  ← paired + v2 (encrypted available)
│ 🔓  bob@phone (Android)             │  ← v2-capable but not paired yet
│ ⚠   xuzhen-test (Windows)          │  ← v1 only — cleartext only
└──────────────────────────────────────┘
```

The badges drive the connect-time UX described in the previous section.

## Compatibility matrix

| Initiator | Responder | Outcome |
|---|---|---|
| v1 (Qt original) | v1 | Legacy session (today's behaviour) |
| v1 | v2 | Receiver peeks 8 bytes, no `DKTOv2\x00` prefix → legacy fallback. Cleartext. |
| v2 | v1 | Initiator hasn't seen any `0x06` from this peer → falls back to legacy. Cleartext. |
| v2 | v2, never paired | Connect-time modal asks user to pair (PSK) or send cleartext. |
| v2 | v2, paired & matching fingerprint | Silent encrypted Noise XX session. |
| v2 | v2, paired but fingerprint mismatch | Modal "Identity changed" — Trust new key (re-pair) / Reject. |

## Milestones

### M1 — Identity (in progress)

- Generate Ed25519 keypair on first run, persist privately (`identity.key`).
- Surface fingerprint in Settings → Profile (copyable).
- No protocol change, no UI badges, no networking change.
- Useful in isolation: future block lists / TOFU records can key on fingerprint instead of the trivially-spoofed signature string.

### M2 — Capability advertisement

- Send `0x06`/`0x07` UDP datagrams in addition to `0x04`/`0x05`.
- Parse them on receive; populate a "v2-capable" flag per peer.
- UI: 🔒/🔓/⚠ badges on the peer list, "Encrypted" column in the audit log.
- Still no encryption — peers just announce that they could.

### M3 — Encrypted tunnel + pairing

- TCP magic prefix `DKTOv2\x00\x00`, Noise XX (XXpsk2 first time).
- Pairing flow: 5-word EFF passphrase + QR, fingerprint pinning in TOFU.
- TOFU mismatch modal.
- Settings: paired-peers list with Forget, "Refuse cleartext" toggle.
- Audit log records `kind=ENCRYPTED` vs `kind=CLEARTEXT` per session.

## Implementation notes

- Wails (Go): `crypto/ed25519` and `golang.org/x/crypto/curve25519` are stdlib. Noise XX: `github.com/flynn/noise` (the Noise reference impl). PSK derivation: `golang.org/x/crypto/hkdf`.
- Android (Kotlin): `com.google.crypto.tink:tink-android` for Ed25519 sign/verify, X25519, AEAD. `androidx.security:security-crypto` for `EncryptedFile`. PSK / Noise: `com.southernstorm.noise.protocol` (java port of Noise) or roll-your-own using Tink primitives.
- EFF wordlist: bundled as a static asset on both sides. Verify via the published checksum at first ship to avoid wordlist drift between peers.
