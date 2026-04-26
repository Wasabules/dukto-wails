# Dukto

Dukto is an easy file transfer tool for LAN. It was created by Emanuele Colombo, ported to Qt 5/6 by [xuzhen and other contributors](https://github.com/xuzhen/dukto/graphs/contributors), and this fork adds a Wails v2 / Go / Svelte-TS rewrite of the **desktop** frontend, a Kotlin / Jetpack Compose rewrite of the **Android** app, **and an opt-in encrypted overlay (v2)** that runs Noise XX over the legacy wire format while staying interoperable with v1 peers.

Now it supports Windows, Linux, MacOS and Android.

## Warning

The legacy v1 wire format transfers files and text **in cleartext** and is only designed for use in trusted network environments. The Wails port and the native Android app (this fork's `wails/` and `android-native/` trees) ship a **v2 encrypted overlay** documented in [`docs/SECURITY_v2.md`](docs/SECURITY_v2.md): once two peers have paired (via a one-shot 5-word EFF passphrase or by trusting a fingerprint), every subsequent transfer between them runs over Noise XX (X25519 / ChaCha20-Poly1305 / SHA-256). v1 peers (Qt original, third-party clients) keep working unchanged — the overlay only kicks in between two v2-capable peers, and only after explicit user pairing.

## Repo layout

This repository hosts three codebases that speak the same LAN protocol:

| Tree | Stack | Targets | Status |
|---|---|---|---|
| `./` (root) | Qt6 / QML, C++ | Android (still ships) + legacy Qt desktop | Source-of-truth for the Qt-Android APK; desktop will be retired |
| [`wails/`](wails/README.md) | Wails v2, Go, Svelte-TS | Windows / macOS / Linux desktop | Production |
| [`android-native/`](android-native/README.md) | Kotlin, Jetpack Compose, AGP | Android | Functional, packaging-ready (debug-signed APK shipped, release APK unsigned) |

The wire format every tree implements is documented in [`docs/PROTOCOL.md`](docs/PROTOCOL.md); the port plan and coexistence rules are in [`docs/PORT_SCOPE.md`](docs/PORT_SCOPE.md).

The native Android APK uses `applicationId = dev.wasabules.dukto`, distinct from the Qt build's `com.github.xuzhen.dukto`, so both can be installed on the same device for parity testing.

## Specs at a glance

| | |
|---|---|
| **Current version** | 6.4.0 (see `version.h`) |
| **Supported OS** | Windows 10+, macOS 11+, Linux, Android 7.0+ (native APK) / Android 8.0+ (Qt6 APK) / Android 5.0+ (Qt5 APK) |
| **Network** | IPv4 only, UDP + TCP on port `4644` (configurable); avatar HTTP on `udp_port + 1` |
| **Encryption (v1)** | None — trusted LAN only |
| **Encryption (v2)** | Noise XX over TCP (X25519 + ChaCha20-Poly1305 + SHA-256) once two peers are paired. Bootstrap: 5-word EFF passphrase + Noise XXpsk2, or manual TOFU pin. Mutual fingerprint authentication, forward secrecy via ephemeral keys. See [`docs/SECURITY_v2.md`](docs/SECURITY_v2.md). |
| **Discovery** | UDP broadcast on every up IPv4 non-loopback interface; types `0x01..0x05` (v1) and `0x06..0x07` (v2 capability + Ed25519 identity). |
| **Wire format** | Little-endian, framed per datagram / per element. See [`docs/PROTOCOL.md`](docs/PROTOCOL.md). |
| **Settings store (Qt)** | `QSettings` under `msec.it/Dukto` (registry / plist / `~/.config/msec.it/Dukto.conf`) |
| **Settings store (Wails)** | JSON under `<UserConfigDir>/dukto/` — one-time migration from the Qt store on first run |
| **Settings store (Android native)** | `SharedPreferences` (`dukto.xml`) under the app's data dir; receive destination tree URI persisted via `takePersistableUriPermission` |
| **Runtime deps (Qt desktop)** | Qt 5.3+ or Qt 6.x; libnotify (optional, Linux) |
| **Runtime deps (Wails)** | WebView2 (Windows), WebKitGTK 4.1 (Linux), WKWebView (macOS, system-provided) |
| **Runtime deps (Android native)** | None beyond the OS — pure-Kotlin, no NDK |

## Encrypted overlay (v2) at a glance

The Wails desktop and the native Android app ship an **opt-in** encrypted overlay that piggy-backs on the existing wire format. Nothing is encrypted by default — the user explicitly pairs each peer first.

| Step | What happens |
|---|---|
| Identity | Each install generates a long-term **Ed25519 keypair** at first launch. Persisted at file mode 0600 (Wails) or AES-256-GCM `EncryptedFile` backed by AndroidKeyStore (Android). User-visible **fingerprint**: 16-char base32 `XXXX-XXXX-XXXX-XXXX`. |
| Discovery | UDP HELLO `0x06`/`0x07` adds the Ed25519 pubkey + a signature over `port \|\| signature_string`. Legacy peers (Qt original, third-party) ignore types ≥ `0x06` silently — interop preserved. |
| Pairing | One-shot **5-word EFF passphrase** (~52 bits, e.g. `apple-tiger-river-ocean-music`); HKDF-SHA256 derives a 32-byte PSK; **Noise XXpsk2** authenticates both sides mutually before either pin commits. QR-code variant available; Android can scan, Wails generates. |
| TOFU fallback | Manual "Trust fingerprint as-is" with a warning modal explaining the first-contact MitM trade-off. |
| Mismatch detection | If a previously-pinned peer's identity changes (legitimate reinstall *or* impersonation attempt), the session is killed and an "Identity changed" modal asks the user to re-pair or unpin. |
| Transport | **Noise XX** static keypair = X25519 derived from the Ed25519 seed. Cipher: **ChaCha20-Poly1305**, hash: **SHA-256** — same suite as WireGuard. Forward secrecy via ephemeral keys. The legacy `SessionHeader`/`ElementHeader` stream is wrapped inside Noise transport messages once the handshake completes. |
| Stealth mode | "Hide me from network discovery" toggle suppresses every outbound HELLO. **Paired peers stay reachable**: the auto-reply path bypasses stealth for inbound `0x06`/`0x07` whose Ed25519 fingerprint is in the local TOFU table, and the unicast probe loop folds in pinned peers' last-known IPs (TTL 7 days). Strangers see nothing. |
| Manual peers | Cross-subnet IPs (or VPN endpoints) can be added by hand and get the same unicast HELLO poke as paired peers. |
| Refuse cleartext | Per-peer setting that blocks every legacy and every unpaired-v2 session; only mutually paired peers can communicate. |
| Audit | Every accept / reject / pair / mismatch decision lands in an append-only audit log (10 MiB rotation, mode 0600), viewable in Settings. |

The v2 layer is fully described in [`docs/SECURITY_v2.md`](docs/SECURITY_v2.md), including the threat model, exact wire layout for the new HELLO types, and the Noise handshake flow.

## Feature comparison — Desktop (Qt6 ↔ Wails port)

| Feature                                              | Qt6 (root tree) | Wails (`wails/`)              |
|------------------------------------------------------|:---------------:|:-----------------------------:|
| Send/receive files, folders, text                    | ✅              | ✅                            |
| Clipboard text / paste-image-to-send                 | ✅              | ✅                            |
| Screen capture send                                   | ✅              | ⏳ not yet ported             |
| Recent activity list (persistent)                    | ✅              | ✅                            |
| Buddy name, avatar, avatar HTTP side-channel         | ✅              | ✅                            |
| Custom avatar (image picker → 64×64 PNG)             | ❌              | ✅                            |
| Dark/light/auto theme detection (OS-native)          | ✅              | ✅                            |
| Manual theme override (System / Light / Dark)        | ❌              | ✅                            |
| Custom theme colour picker                           | ✅              | ❌ fixed Dukto green palette  |
| System tray + close-to-tray                          | ✅              | ✅                            |
| Receive notifications                                 | ✅              | ✅                            |
| Cross-subnet manual peers                             | ❌              | ✅                            |
| Per-interface send/listen allow-list                  | ❌              | ✅                            |
| Whitelist (only-approved-buddies mode)               | ❌              | ✅                            |
| Block list (hard-reject by signature)                | ❌              | ✅                            |
| Confirm unknown peers (first-session modal)          | ❌              | ✅ (60 s timeout)             |
| Auto-reject by extension                              | ❌              | ✅                            |
| Large-session size threshold                          | ❌              | ✅                            |
| Max files / max path depth per session                | ❌              | ✅                            |
| Minimum free-disk-space guard                         | ❌              | ✅                            |
| TCP per-IP accept cooldown                            | ❌              | ✅                            |
| UDP HELLO per-IP cooldown                             | ❌              | ✅                            |
| Receiving master switch + idle auto-disable          | ❌              | ✅                            |
| Audit log (append-only, rotated, 0o600)               | ❌              | ✅ viewable in-app            |
| Speed + ETA in progress bar                           | Partial         | ✅                            |
| Cancel transfer mid-session                           | ❌              | ✅                            |
| Pick file / pick folder buttons + drag-drop          | Partial         | ✅                            |
| Keyboard shortcuts                                    | Partial         | ✅                            |
| Single-instance enforcement                           | ✅ (`SingleApplication`) | ✅ (Wails `SingleInstanceLock`) |
| Windows taskbar progress (`ITaskbarList3`)            | ✅              | ⏳ not yet ported             |
| Android target                                        | ✅              | ❌ out of scope               |
| **v2 — UDP capability advertise (`0x06`/`0x07`)**     | ❌              | ✅                            |
| **v2 — Persistent Ed25519 identity + fingerprint**    | ❌              | ✅                            |
| **v2 — PSK pairing (5-word EFF + XXpsk2)**            | ❌              | ✅ (with QR generator)        |
| **v2 — Manual TOFU pin (with warning modal)**         | ❌              | ✅                            |
| **v2 — Noise XX encrypted transfers**                 | ❌              | ✅                            |
| **v2 — TOFU mismatch detection (handshake + IP rotation)** | ❌         | ✅                            |
| **v2 — Refuse cleartext mode**                        | ❌              | ✅                            |
| **v2 — Hide from network discovery (stealth)**        | ❌              | ✅                            |
| **v2 — Paired-peer probe loop (stealth-friendly)**    | ❌              | ✅                            |

The Wails port is the long-term desktop frontend; the Qt tree will be pared down to Android-only once the Wails builds replace the Qt desktop builds for real users. See `docs/PORT_SCOPE.md` for the transition plan.

## Feature comparison — Android (Qt6 ↔ Native Kotlin/Compose)

| Feature                                              | Qt6-Android (root) | Native (`android-native/`)    |
|------------------------------------------------------|:------------------:|:-----------------------------:|
| Send/receive files, text                             | ✅                 | ✅                            |
| Folder send (recursive walk)                          | ✅                 | ✅ (via SAF tree picker)      |
| Buddy name, avatar HTTP side-channel                 | ✅                 | ✅                            |
| Custom avatar (gallery picker, 64×64 PNG)            | ❌                 | ✅                            |
| Receive into user-chosen folder (SAF tree URI)       | Partial            | ✅                            |
| File preview (in-app + system viewer fallback)       | Partial            | ✅ (image thumbs, text snippet, ACTION_VIEW chooser) |
| Activity log persistent across restarts              | ❌                 | ✅ (SharedPreferences JSON)   |
| Activity log entry cap + Clear                        | ❌                 | ✅                            |
| Manual theme override (System / Light / Dark)        | Partial            | ✅                            |
| Material 3 (Material You disabled — fixed Dukto palette) | ❌             | ✅                            |
| Brand identity (Dukto green logo + colour scheme)    | ✅                 | ✅                            |
| Foreground service for in-flight transfers (Doze-safe) | Partial          | ✅ (`FOREGROUND_SERVICE_DATA_SYNC`) |
| Per-transfer notification with progress              | Partial            | ✅                            |
| `POST_NOTIFICATIONS` runtime permission flow         | n/a (older API)    | ✅                            |
| `ACTION_SEND` / `ACTION_SEND_MULTIPLE` share target  | ❌                 | ✅                            |
| Receiving master switch                              | ❌                 | ✅                            |
| Block list (hard-reject by signature)                | ❌                 | ✅                            |
| Confirm unknown peers (60 s modal)                   | ❌                 | ✅                            |
| Auto-reject by extension                              | ❌                 | ✅                            |
| Max session size                                      | ❌                 | ✅                            |
| Audit log (1 MiB rotated, viewable in Settings)      | ❌                 | ✅                            |
| Cancel transfer mid-session                           | ❌                 | ✅                            |
| Biometric unlock at app launch                        | ❌                 | ✅                            |
| Wireless ADB pairing tested                           | n/a                | ✅                            |
| Manual peers (cross-subnet IPs, VPN endpoints)       | ❌                 | ✅                            |
| `compileSdk` / `minSdk` / `targetSdk`                 | (Qt-managed)       | 36 / 24 / 36                  |
| APK size (release, unsigned)                          | ~22 MB (arm64-v8a) | ~6–8 MB                       |
| APK size (debug-signed, single ABI)                   | ~22 MB             | ~10–12 MB                     |
| Wire format compat with Qt / Wails peers              | ✅                 | ✅ (Kotlin port + JVM tests)  |
| **v2 — UDP capability advertise (`0x06`/`0x07`)**     | ❌                 | ✅                            |
| **v2 — Persistent Ed25519 identity + fingerprint**    | ❌                 | ✅                            |
| **v2 — PSK pairing (5-word EFF + XXpsk2)**            | ❌                 | ✅ (with QR generator + scanner) |
| **v2 — Manual TOFU pin (with warning modal)**         | ❌                 | ✅                            |
| **v2 — Noise XX encrypted transfers**                 | ❌                 | ✅                            |
| **v2 — TOFU mismatch detection**                      | ❌                 | ✅                            |
| **v2 — Refuse cleartext mode**                        | ❌                 | ✅                            |
| **v2 — Hide from network discovery (stealth)**        | ❌                 | ✅                            |
| **v2 — Paired-peer probe loop (stealth-friendly)**    | ❌                 | ✅                            |

The native APK has reached functional parity with the Qt one for the day-to-day flows (discover → send/receive text, files, folders, with thumbnails and a preview screen), and it ships extra security gates that mirror the Wails desktop. Once it's been used in the wild for a release cycle, the Qt-Android tree can be retired alongside the Qt-desktop tree (see [`docs/PORT_SCOPE.md`](docs/PORT_SCOPE.md)).

### Prebuilt Packages

Every release on the [GitHub releases page](https://github.com/Wasabules/dukto-wails/releases) of this fork ships nine artifacts:

| File pattern | What it is |
|---|---|
| `dukto-wails-X.Y.Z-linux-amd64.tar.gz` | Wails Linux build — extract and run `./wails` (binary inside the tarball) |
| `dukto-wails-X.Y.Z-windows-amd64.zip` | Wails Windows build — unzip and run `wails.exe` |
| `dukto-wails-X.Y.Z-darwin-universal.zip` | Wails macOS bundle — unzip and run `dukto.app` |
| `dukto-qt6-X.Y.Z-linux-amd64.tar.gz` | Qt6 desktop, Linux — extract and run `./<dir>/AppRun` |
| `dukto-qt6-X.Y.Z-windows-amd64.zip` | Qt6 desktop, Windows — unzip and run `dukto.exe` (Qt DLLs deployed alongside via `windeployqt`) |
| `dukto-qt6-X.Y.Z-darwin-universal.zip` | Qt6 desktop, macOS — unzip and run `dukto.app` (frameworks deployed via `macdeployqt`) |
| `dukto-qt5-X.Y.Z-linux-amd64.tar.gz` | Qt5 desktop, Linux — same shape as Qt6, kept for legacy distros |
| `dukto-android-X.Y.Z-arm64_v8a.apk` / `…-armv7.apk` | Qt6-Android APKs (unsigned) |
| `dukto-android-native-X.Y.Z-debug-signed.apk` | Native Android APK signed with the debug keystore — installable directly on any device for testing |
| `dukto-android-native-X.Y.Z-release-unsigned.apk` | Native Android APK release build — sign out-of-band before distribution |

> **Note.** Both the Qt-Android APKs and the native release APK are unsigned by the workflow; sign them with your own keystore before distributing. The native debug APK is signed with the standard Android debug keystore so it installs on any device for testing without further setup.

### Build from source code

The repo has three build systems — pick whichever matches your target.

#### A. Wails desktop app (Windows / macOS / Linux) — recommended for desktop

**Dependencies**

- Go 1.21+ (1.23 is what the module targets).
- [Wails v2 CLI](https://wails.io/docs/gettingstarted/installation): `go install github.com/wailsapp/wails/v2/cmd/wails@v2.12.0`.
- Node.js 18+.
- Platform runtime libs: `webkit2gtk-4.1-dev libgtk-3-dev` on Linux; nothing extra on Windows (WebView2 is auto-installed by modern Windows) or macOS.
- Run `wails doctor` to verify the toolchain.

**Build**

```sh
cd wails
wails build              # release build → wails/build/bin/
wails build -debug       # with debug symbols
wails dev                # Vite HMR + live Go reload
go test ./...            # Go unit tests (incl. wire-format parity)
cd frontend && npm run check   # TypeScript / Svelte check
```

The `webkit2_41` build tag is pinned in `wails.json`, so `wails build` and `wails dev` automatically link against `libwebkit2gtk-4.1` (Ubuntu 24.04+ no longer ships the 4.0 series). On Windows/macOS the tag is a no-op.

See [`wails/README.md`](wails/README.md) for the full developer guide.

#### B. Native Android app (Kotlin + Compose) — recommended for Android

**Dependencies**

- JDK 17 (Temurin / OpenJDK).
- Android SDK with platform 36 + build-tools 36 (install via Android Studio's SDK Manager, or `sdkmanager` CLI).
- Optional but recommended: Android Studio (Hedgehog or newer).

**Build**

```sh
cd android-native
./gradlew assembleDebug                  # debug APK → app/build/outputs/apk/debug/
./gradlew installDebug                   # install on the connected device/emulator
./gradlew assembleRelease                # unsigned release APK
./gradlew test                           # JVM unit tests (incl. wire-format round-trip)
```

The native app installs alongside the Qt APK without conflict — they have distinct `applicationId`s. Wireless ADB pairing works fine for development:

```sh
adb pair  <PHONE_IP>:<PAIR_PORT>          # enter the 6-digit code from the phone
adb connect <PHONE_IP>:<CONNECT_PORT>     # main connect port (visible on the Wireless debugging screen)
adb install -r app/build/outputs/apk/debug/app-debug.apk
```

See [`android-native/README.md`](android-native/README.md) for the full migration checklist and developer guide.

#### C. Qt Dukto (Android target; legacy desktop)

**Dependencies**

- Qt 5.3+ (desktop) or Qt 6.x
- libnotify (optional, Linux only)
- Android SDK and NDK (Android only)

Initialise the submodule before building with `SINGLE_APP`:

```sh
git submodule update --init --recursive
```

##### For Windows, Linux, MacOS

Run the following command in the source code directory to build:

* QMake 
```sh
mkdir build && cd build && qmake .. && make
```

* CMake 
```sh
mkdir build && cd build && cmake .. && make
```

##### For Android

* Build with Qt6:
```sh
mkdir build && cd build
/path/to/qt6/bin/qt-cmake -DANDROID_NDK_ROOT=/path/to/ndk -DANDROID_SDK_ROOT=/path/to/sdk ..
make
```

* Build with Qt5:
```sh
mkdir build && cd build
export ANDROID_NDK_ROOT=/path/to/ndk ANDROID_SDK_ROOT=/path/to/sdk
cmake -DCMAKE_SYSTEM_NAME=Android -DCMAKE_ANDROID_ARCH_ABI=arm64-v8a -DQT_CMAKE_ROOT=/path/to/qt/cmake ..
make
```

## Continuous integration

GitHub Actions workflows under `.github/workflows/`:

| Workflow                       | Trigger                                          | What it does |
|--------------------------------|--------------------------------------------------|--------------|
| `ci.yml`                       | Every push / PR                                  | Fast lane: `go vet`, `go test ./...` and `npm run check` for the Wails port |
| `build-wails.yml`              | Push / PR touching `wails/`                      | `wails build` on Ubuntu, Windows and macOS |
| `build-android-native.yml`     | Push / PR touching `android-native/`             | Gradle `test` + `assembleDebug` + `assembleRelease` (Ubuntu, JDK 17) — debug + unsigned-release APKs uploaded as artifacts |
| `build-qt6.yml`                | Push / PR touching Qt sources                    | Qt 6.8.1 CMake build on Ubuntu, Windows and macOS |
| `build-qt5.yml`                | Push / PR touching Qt sources                    | Qt 5.15.2 CMake build on Ubuntu (legacy coverage) |
| `build-android.yml`            | Push / PR touching Qt sources                    | Qt6-Android APK for `arm64_v8a` and `armv7` |
| `release.yml`                  | Push of tag `v*`, or manual                      | Runs tests + every build target (Wails ×3, Qt6 ×3, Qt5 Linux, Qt6-Android ×2 ABI, native Android), then publishes a GitHub release with all packaged artifacts attached |

The Qt workflows have `paths-ignore` for `wails/**` and `android-native/**` so changes scoped to one of the rewrites don't trigger Qt rebuilds.

### Cutting a release

1. Bump `version.h` (both the `#define VERSION_*` macros and the `VERSION=x.y.z` line read by `dukto.pro`) and commit.
2. Tag the commit `vX.Y.Z` and push the tag:
   ```sh
   git tag v6.4.0
   git push origin v6.4.0
   ```
3. `release.yml` runs on the tag push: gates on `go test` + `svelte-check`, then builds in parallel:
   - Wails desktop (Linux / Windows / macOS)
   - Qt6 desktop (Linux / Windows / macOS, with `linuxdeploy` / `windeployqt` / `macdeployqt` so binaries are runnable without a system Qt install)
   - Qt5 desktop (Linux only)
   - Qt6-Android (`arm64_v8a` + `armv7`)
   - Native Android (Kotlin/Compose, `debug-signed` + `release-unsigned` APKs)
   The final `publish` job downloads every artifact and creates a GitHub release titled `Dukto X.Y.Z` with auto-generated notes.
4. **Signing**: the Qt-Android APKs are unsigned; the native-Android `release-unsigned.apk` is unsigned. Sign them with your own keystore before distributing — the `debug-signed.apk` is for testing only. Desktop binaries don't need code-signing for distribution but you may want to sign them on macOS / Windows to avoid Gatekeeper / SmartScreen prompts.
5. `workflow_dispatch` is also available from the Actions tab for re-runs; pass the existing tag as input.
