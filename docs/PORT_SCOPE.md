# Wails/Go/Svelte Port — Scope and Coexistence Plan

Status: **planning**. No Go code written yet. This doc records the scope decisions before work starts so the two codebases stay compatible.

## Target architecture

```
                                  LAN (UDP/TCP 4644)
                                         │
          ┌──────────────────────────────┼──────────────────────────────┐
          │                              │                              │
    Qt6 (Android only)           Wails/Go/Svelte               other Dukto peers
    — this repo,                 (desktop: Win/Mac/Linux)      (legacy, third-party)
      pared down to              — new repo (TBD)
      Android target
```

Both our own builds must interoperate with each other **and** with arbitrary third-party Dukto apps already deployed. The wire format is frozen in `PROTOCOL.md`; don't change it unilaterally in either stack.

## Scope of the Wails port

**In scope** (desktop feature parity with Qt6 6.2.0):

- UDP discovery + TCP transfer (full implementation of `PROTOCOL.md`).
- Send/receive files, folders, text, clipboard text, screen capture.
- Recent-activity list, destination folder chooser.
- Profile (buddy name, avatar, avatar HTTP side-channel on port 4645).
- System tray with close-to-tray, notifications on receive.
- Dark/light/auto theme, custom theme colour.
- Single-instance enforcement (equivalent of `SingleApplication`).
- Windows taskbar progress (`ITaskbarList3`) — parity with `ecwin7.*`.
- Drag-and-drop onto the window.
- Native dark-theme detection per OS (same sources as `platform.cpp`).
- Settings persistence in the **same store as Qt** (see §Interop with existing installs).

**Out of scope** for the Wails port:

- Android — stays on Qt6 in this repo.
- `UPDATER` / `updateschecker.cpp` — already marked broken, don't port it.
- libnotify-specific notification path — use Wails' notification API or `github.com/gen2brain/beeep`, bridged to OS-native.
- The legacy `qml/old/` UI variant — dead on arrival in a Svelte rewrite.

## Scope of the Qt6 codebase after the port

Once the Wails desktop app reaches feature parity, this Qt6 repo becomes **Android-only**. Everything desktop-specific can be deleted:

- `ecwin7.*` (Windows taskbar).
- `platform.cpp` branches for `Q_OS_WIN`, `Q_OS_MAC`, `Q_OS_LINUX` non-Android — keep only the `Q_OS_ANDROID` paths.
- `systemtray.*` — desktop-only, gated by `DESKTOP_APP`.
- `modules/SingleApplication` submodule — desktop-only.
- Linux DBus integration, macOS bundle/icon setup, Windows `.rc`/`.ico`.
- `qml/old/` — Qt6-Android uses the `qml/new/` tree unconditionally.
- `dukto.desktop`, `.png` icon install rules, macOS `.icns`.
- CMake `USE_SINGLE_APP`, `USE_NOTIFY_LIBNOTIFY`, `USE_UPDATER` options.

**Don't touch yet.** Do this cleanup in one go only after the Wails app ships to Android-less users, so bisecting desktop regressions against the old Qt build is still possible during the transition.

## Interop constraints (non-negotiable)

1. **Wire format**: exactly as specified in `PROTOCOL.md`. Test both stacks against the same fixture bytes.
2. **UDP/TCP ports**: 4644 by default, configurable; avatar HTTP at `udp_port + 1`.
3. **Platform token in signature**: use `Windows`, `Macintosh`, `Linux` (not `"Win"`, `"macOS"`, `"Ubuntu"`, etc.) — legacy peers render the platform icon off this string.
4. **Text-snippet magic**: literal `___DUKTO___TEXT___`.
5. **Directory sentinel**: element_size `-1`.
6. **Little-endian** integer I/O regardless of host.

If anything on this list needs to change, it is a protocol version bump and must be coordinated with every implementation (ours + third-party).

## Interop with existing installs (settings migration)

The Qt app stores settings under the vendor/application pair `msec.it` / `Dukto` via `QSettings`. On each OS:

- **Windows**: `HKCU\Software\msec.it\Dukto` (registry). Values are typed (`REG_SZ`, `REG_DWORD`, `REG_BINARY`).
- **macOS**: `~/Library/Preferences/com.msec.it.Dukto.plist` (or `it.msec.Dukto` depending on Qt version — verify at migration time).
- **Linux**: `~/.config/msec.it/Dukto.conf` (INI-like text).

Keys currently written (`settings.cpp`):

| Key                    | Type      | Default             | Notes |
|------------------------|-----------|---------------------|-------|
| `DestPath`             | string    | `Platform::getDefaultPath()` | Per-OS default, see `platform.cpp`. |
| `WindowPosAndSize`     | bytearray | unset               | Opaque Qt `saveGeometry()` blob — don't try to reuse, re-derive from screen metrics. |
| `ThemeColor`           | string    | `Theme::DEFAULT_THEME_COLOR` | Hex `#rrggbb`. |
| `AutoMode`             | bool      | `true`              | Follow OS dark/light. |
| `DarkMode`             | bool      | `false`             | Only when `AutoMode=false`. |
| `R5/ShowTermsOnStart`  | bool      | `true`              | First-run ToS modal. |
| `BuddyName`            | string    | `""`                | Empty ⇒ fall back to OS username. |
| `Notification`         | bool      | `false`             | Notify on receive. |
| `CloseToTray`          | bool      | `false`             |       |

**Recommendation for the Wails port**: on first run, read the existing Qt settings store and seed a new Wails-native settings file from it (e.g. JSON under `%AppData%/Dukto/` on Windows, `~/.config/dukto/` on Linux, `~/Library/Application Support/Dukto/` on macOS). Skip `WindowPosAndSize` — its format is Qt-internal. After seeding, only write to the new store; don't mutate the Qt store. This lets a user downgrade back to Qt without losing their Qt settings, at the cost of a one-way fork for any later changes.

Avatar file (`avatar.png` under `QStandardPaths::AppLocalDataLocation`) can be copied as-is to the Wails data dir.

## Test strategy (applies to both stacks)

Fixtures are generated from the Qt code itself (see `PROTOCOL.md` §7) — no Wireshark needed. Sequence:

1. **Refactor `Sender` to write to a `QIODevice *`** instead of an owned `QTcpSocket`. This makes it unit-testable and decouples the serialisation from the transport. Low-risk, localised change.
2. **Write a Qt fixture generator** (`tests/fixture_gen/`) that drives the refactored `Sender` and `BuddyMessage` against fixed inputs and writes `.bin` + `.json` files under `tests/fixtures/`.
3. **Encoder tests** (Qt and Go): serialise the same inputs, assert byte-for-byte equality with fixtures.
4. **Decoder tests** (Qt and Go): feed fixture bytes into the receiver pipeline, assert reconstructed filesystem/text matches the `.json` manifest.
5. **Cross-stack integration** (manual, then CI): Qt sender → Go receiver, Go sender → Qt receiver, across all fixture scenarios.
6. **Fuzz** the Go decoder for the TCP stream (hostile peer or corrupted bytes) — the Qt code has been in production long enough to be resilient; the Go port won't be on day one.

## Decided stack

- **Wails v2** — stable, production-proven. v3 is still beta.
- **Backend**: Go. File I/O + net/ip maps one-for-one to the Qt code; no third-party networking lib needed (stdlib `net` is enough).
- **Frontend**: Svelte. QML → Svelte mapping is roughly per-page (`BuddiesPage.qml` → `Buddies.svelte`, etc.).
- **IPC**: Wails bindings (Go methods callable from JS). Mirror the `GuiBehind` surface as a single bound struct to minimise the mental model shift from the Qt side.

## Open questions (not blocking)

- **Single-instance on Windows**: not Wails-provided. Options: `github.com/juju/mutex` (cross-platform), named mutex via `golang.org/x/sys/windows`, or a TCP socket on a fixed local port. Pick once we get to the polish phase.
- **Screen capture**: Qt uses `QScreen::grabWindow`. Go equivalents: `github.com/kbinani/screenshot` (pure Go, cross-platform).
- **Clipboard**: `github.com/atotto/clipboard` or Wails' built-in clipboard API.
- **Notifications**: `github.com/gen2brain/beeep` is the usual pick, OS-native under the hood.
