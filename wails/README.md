# Dukto (Wails port)

Wails v2 + Go + Svelte-TS rewrite of Dukto's desktop frontend (Windows, macOS, Linux). Interoperates on the LAN with the Qt6 Dukto app in the parent directory and with third-party Dukto peers — see [`../docs/PROTOCOL.md`](../docs/PROTOCOL.md) for the wire format and [`../docs/PORT_SCOPE.md`](../docs/PORT_SCOPE.md) for the port plan.

## Prerequisites

- Go 1.21+.
- [Wails v2 CLI](https://wails.io/docs/gettingstarted/installation) (`go install github.com/wailsapp/wails/v2/cmd/wails@latest`).
- Node.js 18+ (the Svelte/Vite frontend is built through the Wails CLI; you don't need to run `npm` directly except when adding frontend dependencies).
- Platform-specific Wails dependencies — run `wails doctor` once to check.

## Development

From this directory:

```sh
wails dev
```

Starts a Vite dev server with HMR for the Svelte frontend and rebuilds the Go side on change. The dev server also exposes the Go bindings at `http://localhost:34115` so you can call them from browser devtools.

## Build

```sh
wails build           # release build into build/bin/
wails build -debug    # include debug symbols
```

## Tests

The Go side of the port has unit tests; the frontend is validated with `svelte-check`:

```sh
go test ./...                            # all Go tests
go test ./internal/protocol/... -run Xxx # single test in the wire-format package
cd frontend && npm run check             # Svelte/TS type check
```

There is no test runner configured on the frontend beyond `svelte-check`.

## Layout

Top-level Go files in this directory (`package main`):

- `main.go` — Wails app bootstrap.
- `app.go` — `App` struct that owns every subsystem (messenger, TCP server, settings, audit, history, …). This is the type bound to the JS side.
- `lifecycle.go` — startup/shutdown wiring; maps Go-side events (`transfer`/`discovery`) to Wails `EventsEmit` for the frontend.
- `events.go` — canonical event names shared between Go and TS.
- `bindings_*.go` — methods on `*App` exposed as JS RPCs, grouped by concern: `files`, `history`, `peers`, `policy`, `receive`, `security`, `settings`, `transfer`.
- `disk_unix.go` / `disk_windows.go` — build-tagged free-space probe used by the disk-guard policy.

Reusable Go packages under `internal/`:

| Package       | Purpose |
|---------------|---------|
| `protocol`    | Pure-Go encoder/decoder for UDP `BuddyMessage` (types `0x01..0x07`), TCP `SessionHeader`/`ElementHeader`, `BuildSignature`. No Wails deps; the single source of truth for byte-level I/O. Round-trip and parity tests next to it. |
| `discovery`   | UDP messenger: HELLO/GOODBYE, v2 capability advertisement (`0x06`/`0x07` signed Ed25519), self-echo suppression, broadcast-storm guard, per-source HELLO cooldown, IP-level identity-rotation detection, stealth mode with selective reply for paired peers. |
| `transfer`    | TCP server + `Receiver` (streaming parse) + `Sender`. Magic-prefix peek (`DKTOv2\x00\x00`) routes a session through the encrypted Noise XX tunnel when the peer is paired; legacy bytes flow through the unmodified `Receiver` otherwise. Exposes `Allow`, `Upgrade`, and `OnSessionMode` hooks. |
| `tunnel`      | Noise XX (X25519 + ChaCha20-Poly1305 + SHA-256) handshake + transport layer using `github.com/flynn/noise`. XXpsk2 path for first-pairing. Magic-prefix peek with replay-on-fallback so legacy v1 senders see an unmodified byte stream. |
| `identity`    | Long-term Ed25519 keypair persisted at file mode `0600` under `<UserConfigDir>/dukto/identity.key`. X25519 derivation from the Ed25519 seed (RFC 8032 / 7748) and Edwards-to-Montgomery projection so the same long-term identity covers both signing (UDP HELLO) and DH (Noise XX). Fingerprint = `base32(SHA-256(pub)[:10])`. |
| `eff`         | Bundled EFF Short Wordlist 1 (1296 words). 5-word passphrase generator + HKDF-SHA256 PSK derivation (`info="DUKTO-PSK-v1"`). Cross-stack known-vector tests pin the bytewise output. |
| `settings`    | JSON-file-backed settings store with atomic `Update(func(*Values))`. `OpenWithMigration` seeds on first run from the Qt `QSettings` store (registry on Windows, plist on macOS, INI on Linux). Holds `PinnedPeers`, `RefuseCleartext`, `HideFromDiscovery`, `ManualPeers`, … |
| `history`     | Recent-activity list (sent / received) with `Encrypted` flag per entry. |
| `audit`       | Append-only JSON-per-line audit log; 10 MiB rotation (`audit.log` + `audit.log.1`); mode `0o600`. Records accept/reject, peer_pinned, session_encrypted, tofu_mismatch, etc. |
| `platform`    | OS name token for the signature (`Windows`/`Macintosh`/`Linux`), user/hostname, default download path, dark-theme detection. |
| `osint`       | Cross-platform shims kept out of `platform` (e.g. open file in file manager). |
| `httpserve`   | Avatar HTTP side-channel on `udp_port + 1`. |
| `avatar`      | Local + per-peer avatar PNG resolution/cache. |

Frontend (`frontend/`): Svelte + Vite + TypeScript.

- `src/App.svelte` — top-level state orchestration.
- `src/components/` — one file per UI surface (`Header`, `PeerList`, `Composer`, `ReceivedList`, `ProgressStack`, `PreviewModal`, `PendingSessionModal`, `SettingsModal`, …).
- `src/components/settings/` — one file per settings tab (`GeneralTab`, `SecurityTab`, `LimitsTab`, `NetworkTab`, `AuditTab`).
- `src/lib/dukto.ts` — typed wrappers over Wails-generated bindings.
- `src/lib/events.ts` — subscribes to Wails events and fans them out to stores.
- `src/lib/stores/` — Svelte writable stores (toasts, pending-session modal, …).
- `wailsjs/` — generated Go↔JS bindings (committed; regenerated by `wails generate` on binding changes).

## Settings and data location

Settings and the audit log live under `os.UserConfigDir()`:

- Windows: `%AppData%\Dukto\`
- macOS: `~/Library/Application Support/Dukto/`
- Linux: `~/.config/dukto/`

On first run the Go side reads the Qt `QSettings` store (if present) and seeds `settings.json`. The Qt store is left untouched so downgrades to the Qt desktop build stay safe. See `../docs/PORT_SCOPE.md` for the key-by-key mapping.

## Security surface

The Wails port carries extra policy gates not present in the Qt6 app. Ordered as they run on the receive path:

1. master switch (receiving enabled)
2. block list (signature)
3. TCP per-IP accept cooldown
4. allowed-interface filter
5. whitelist (only-approved-buddies mode)
6. confirm-unknown peers modal (60 s timeout)
7. **v2 magic-prefix peek**: if `DKTOv2\x00\x00`, run Noise XX responder; if not, fall back to the legacy `SessionHeader` parser
8. **TOFU mismatch detector**: when the new `remote_static` doesn't match the X25519 derived from the peer's pinned Ed25519 fingerprint, kill the session and emit a UI alert
9. **RefuseCleartext gate**: if on, drop every legacy session and every unpaired-v2 session
10. session-header checks: blocked extensions, large-session size threshold, max files, max path depth, minimum free-disk percent

Every accept/reject decision is written to the audit log (mode `0600`, 10 MiB rotation) and is visible in the **Audit** settings tab.

The full v2 design is in [`../docs/SECURITY_v2.md`](../docs/SECURITY_v2.md): threat model, exact wire layout for the `0x06`/`0x07` HELLO types, the Noise XX / XXpsk2 handshake, EFF passphrase encoding, and the milestone breakdown (M1–M3d, all shipped).
