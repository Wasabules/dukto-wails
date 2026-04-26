# Dukto — Native Android port

Parallel rewrite of the Android app in **Kotlin + Jetpack Compose**, sharing nothing with the Qt6 build at the repo root. Both APKs install side-by-side on the same device:

| Build           | `applicationId`                | Source            |
|-----------------|--------------------------------|-------------------|
| Qt6 / QML       | `com.github.xuzhen.dukto`      | repo root         |
| Native (this)   | `dev.wasabules.dukto`          | `android-native/` |

The Qt build keeps shipping until this port replaces it for real users. Goal once it does: delete the Qt-Android pieces (`qml/`, `androidutils.{h,cpp}`, `android/`, `dukto.pro`, `CMakeLists.txt`, `modules/SingleApplication`) — the desktop is already off Qt, on Wails.

## Why the rewrite

- APK size: ~22 MB → **~6–8 MB release / ~10–12 MB debug** (no Qt6 + QtQuick + Quick Controls bundled, even with the v2 crypto stack added).
- Material 3 with the **Dukto brand palette** (fixed `#248b00` green; Material You dynamic colors disabled on purpose so peers look consistent across devices) + edge-to-edge / predictive back / proper insets.
- Storage Access Framework (SAF) for the destination directory and the share-with-Dukto flow — files land in a folder visible from any file manager.
- Foreground service (`FOREGROUND_SERVICE_DATA_SYNC`) for in-flight transfers — Doze-friendly, survives backgrounding.
- Native runtime permissions (`POST_NOTIFICATIONS` on Android 13+, `CAMERA` on demand for QR pair, etc.).
- **Encrypted overlay (v2)** — Noise XX over TCP, 5-word EFF passphrase pairing, biometric unlock at app launch. See [`../docs/SECURITY_v2.md`](../docs/SECURITY_v2.md).
- First-class Android Studio profiler / layout inspector / tracing.
- Lets us delete the Qt-Android tree once parity is confirmed.

## Stack

- AGP 8.7 + Kotlin 2.1 (configured via `gradle/libs.versions.toml`).
- Jetpack Compose (BOM 2024.12) + Material 3 + Coil 2.7 (image loading).
- `compileSdk = 36`, `minSdk = 24` (Android 7.0+), `targetSdk = 36`.
- JVM toolchain 17.
- Crypto stack: BouncyCastle (X25519, ChaCha20-Poly1305 primitives), `net.i2p.crypto:eddsa` (Ed25519 sign/verify on API 24+), `androidx.security:security-crypto` (`EncryptedFile` for the long-term identity keystore-backed at rest), ZXing (QR encode/decode for the pairing flow).

## First-time setup

The Gradle wrapper is bundled — `./gradlew` works out of the box once you have JDK 17 and an Android SDK with platform 36 (install via Android Studio's SDK Manager, or `sdkmanager` CLI).

```sh
cd android-native
./gradlew assembleDebug                  # debug APK → app/build/outputs/apk/debug/
./gradlew installDebug                   # install on the connected device/emulator
./gradlew assembleRelease                # unsigned release APK
./gradlew test                           # JVM unit tests (incl. wire-format round-trip)
```

For wireless ADB:

```sh
adb pair  <PHONE_IP>:<PAIR_PORT>          # 6-digit code from the phone's pairing dialog
adb connect <PHONE_IP>:<CONNECT_PORT>     # main connect port (visible on the Wireless debugging screen)
adb install -r app/build/outputs/apk/debug/app-debug.apk
adb shell am start -n dev.wasabules.dukto/.MainActivity
```

## Layout

```
android-native/
├── app/
│   ├── build.gradle.kts
│   └── src/main/
│       ├── AndroidManifest.xml
│       ├── kotlin/dev/wasabules/dukto/
│       │   ├── MainActivity.kt          — Compose entry, edge-to-edge, share-intent capture
│       │   ├── DuktoApp.kt              — Application; engine ownership, notif channel
│       │   ├── DuktoEngine.kt           — process-wide orchestration (Messenger / Server / Sender / AvatarServer)
│       │   ├── ui/                      — Compose surfaces:
│       │   │   ├── DuktoScreen.kt       — top bar (avatar + status), peer list, activity list, send sheet
│       │   │   ├── SettingsSheet.kt     — Profile / Appearance / Destination / Security / Activity / Audit
│       │   │   ├── PendingPeerDialog.kt — confirm-unknown 60 s modal
│       │   │   ├── PreviewScreen.kt     — file detail / thumbnail grid
│       │   │   ├── FileMeta.kt          — content-resolver metadata helpers
│       │   │   └── theme/Theme.kt       — fixed Dukto green palette (light + dark)
│       │   ├── protocol/                — wire format (BuddyMessage, SessionHeader, ElementHeader, signature)
│       │   ├── discovery/               — UDP messenger + MulticastLock
│       │   ├── transfer/                — TCP server + receiver + sender + foreground service
│       │   ├── policy/                  — SessionPolicy: master switch / block list / confirm-unknown / size cap / extension reject
│       │   ├── audit/                   — append-only JSON-per-line log, 1 MiB rotation
│       │   ├── settings/                — SharedPreferences-backed Settings + ThemeMode
│       │   ├── avatar/                  — HTTP side-channel (port 4645) + initials renderer
│       │   └── platform/                — OS identity / device name
│       └── res/                         — Material theme, mipmap-* launcher icons (Dukto pipe on green)
└── README.md
```

## Status — what's in vs not yet

### Wire format
- ✅ `protocol`: `BuddyMessage` encode/decode (UDP datagrams `0x01..0x05` legacy + `0x06..0x07` v2 with embedded Ed25519 pubkey + signature).
- ✅ `protocol`: `SessionHeader` + `ElementHeader` streaming codec (TCP).
- ✅ `protocol`: `buildSignature("<user> at <host> (Android)")`.
- ✅ JVM round-trip + invalid-input tests (covers Qt-compatible rejection rules + v2 truncation / signature tampering).
- ⏳ Cross-stack fixture tests: feed the `.bin` fixtures the Go side will eventually generate. Left for follow-up once Qt fixture generator runs in CI.

### v2 encrypted overlay
- ✅ `identity`: long-term Ed25519 keypair, `EncryptedFile` (AES-256-GCM, MasterKey from AndroidKeyStore — hardware-backed where TEE/StrongBox is available). X25519 derivation from the Ed25519 seed; Edwards-to-Montgomery projection for cross-binding.
- ✅ `tunnel`: hand-rolled Noise XX (X25519 / ChaCha20-Poly1305 / SHA-256) using BouncyCastle primitives, with XXpsk2 mode for first-pairing. 8-byte magic prefix `DKTOv2\x00\x00`. Cross-stack tested against `flynn/noise` on the Wails side.
- ✅ `eff`: bundled EFF Short Wordlist 1 (1296 words), 5-word passphrase generator + HKDF-SHA256 PSK derivation. Known-vector test pinned to the Wails side.
- ✅ TOFU pinning persisted in `SharedPreferences`. PSK pairing auto-pins both sides on success.
- ✅ TOFU mismatch detector (handshake-time and IP-rotation-time, both surface the same modal).
- ✅ RefuseCleartext mode: drops every legacy session and every unpaired-v2 session.
- ✅ HideFromDiscovery (stealth) + paired-peer probe loop: paired peers stay reachable via unicast HELLO even when broadcast is off.
- ✅ Pair UI: generate code (5-word + QR) / enter code (text + camera scan via `zxing-android-embedded`).
- ✅ Biometric unlock at app launch (`androidx.biometric`).

### Networking
- ✅ `discovery.Messenger`: HELLO/GOODBYE, self-echo suppression, periodic broadcast, `WifiManager.MulticastLock`, GOODBYE on stop, unicast reply on broadcast.
- ✅ `transfer.Server`: TCP server on port 4644, accept loop, per-session coroutine, policy-gated.
- ✅ `transfer.Receiver`: streaming parse → SAF tree (when configured) or `getExternalFilesDir(DOWNLOADS)/dukto-<ts>-<src>/` fallback. Captures per-file URIs for the preview UI.
- ✅ `transfer.Sender`: text snippet, multi-URI files, recursive folder send via SAF tree URI.
- ✅ Cancel in-flight transfer (closes active socket on either side).
- ✅ `httpserve` / `avatar.AvatarServer`: avatar HTTP side-channel on `udp_port + 1` (port 4645) — initials tile by default, custom user-picked PNG when set (auto-resized to 64×64).
- ⏳ Per-source HELLO cooldown / broadcast-storm guard. Lower priority — can layer on top later.

### UI (Compose)
- ✅ `DuktoScreen` — top bar (Dukto logo avatar + receiving status + settings cog), peer list with per-peer avatar (Coil-fetched from the peer's avatar HTTP endpoint, falls back to initials), recent activity, in-flight progress bar with cancel.
- ✅ `SettingsSheet` — Profile (avatar pick + display name), Appearance (System / Light / Dark theme override), Destination (SAF folder picker), Security (master switch, confirm-unknown, blocked extensions, max session size, blocked & approved peers, forget approvals), Recent activity (max-entries cap + Clear), Audit log (last 15 entries + Clear).
- ✅ Send sheet — text snippet + multi-file picker + folder picker.
- ✅ Custom avatar: gallery picker → `BitmapFactory` decode → centre-crop + scale to 64×64 → re-encode PNG → persist to `filesDir/avatar.png` → live update of the AvatarServer's payload.
- ✅ Material 3 with **fixed Dukto green palette** (light + dark) — keyed off the original Qt `theme.cpp`. Theme can be system / light / dark.
- ✅ Pending-peer modal (60 s countdown, Allow once / Allow always / Reject / Block forever).
- ✅ `PreviewScreen` — image thumbnails via Coil + text snippet rendering + ACTION_VIEW chooser fallback.
- ⏳ Speed / ETA in the in-flight progress bar (currently shows bytes / total).
- ⏳ About / terms-of-use first-run screen.

### Android plumbing
- ✅ Notification channel created at app start (`dukto.transfers`).
- ✅ `POST_NOTIFICATIONS` runtime request on Android 13+.
- ✅ Share intent: `ACTION_SEND` / `ACTION_SEND_MULTIPLE` → URIs surfaced as a "ready to send" banner.
- ✅ `FOREGROUND_SERVICE_DATA_SYNC` foreground service for active transfers — keeps the OS from killing the process under Doze.
- ✅ Per-transfer progress notification with content intent back to the activity.
- ✅ `usesCleartextTraffic="true"` declared (the Dukto protocol is cleartext HTTP/TCP by design).
- ✅ Adaptive launcher icon: Dukto pipe on Dukto-green background (`mipmap-anydpi-v26/ic_launcher.xml` + bitmap fallbacks at every density).

### Security parity (matches the Wails desktop's defense-in-depth chain)
- ✅ Master switch (Receiving on/off).
- ✅ Block list (signature-based hard reject).
- ✅ Confirm unknown peers (60 s modal, AllowOnce / AllowAlways → adds to approvedPeers / Reject / Block forever → adds to blockedPeers).
- ✅ Auto-reject by extension (default list mirrors Wails: `exe,bat,cmd,com,scr,msi,ps1,vbs,jse,lnk`).
- ✅ Max session size cap (MB).
- ✅ Audit log (append-only JSON-per-line, 1 MiB rotation, viewable in Settings).
- ⏳ Whitelist-only mode (overlap with confirm-unknown — pick one for now).
- ⏳ Per-IP TCP / UDP cooldowns (low priority on phones with one wifi iface).
- ⏳ Free-disk percent guard.

### Release
- ✅ `.github/workflows/build-android-native.yml` — debug + unsigned-release APKs as artifacts on every push.
- ✅ `release.yml` — `android-native` job ships `dukto-android-native-x.y.z-debug-signed.apk` + `dukto-android-native-x.y.z-release-unsigned.apk` alongside Qt / Wails artifacts on tag push.
- ⏳ Real signing keystore for the release APK (out-of-band, like the Qt APKs).
- ⏳ Once usage confirms parity → drop the Qt-Android tree (see [`docs/PORT_SCOPE.md`](../docs/PORT_SCOPE.md)).

## Testing alongside the Qt APK

1. Install the Qt APK on the phone (current release): `adb install dukto-android-6.2.0-arm64_v8a.apk`.
2. Build + install the native one: `./gradlew installDebug`.
3. Both apps appear separately on the home screen — labelled "Dukto" (Qt) and "Dukto Native" (this), with distinct icons (the native one carries the Dukto pipe-on-green adaptive icon).
4. Discover each other on the same Wi-Fi as two distinct peers — that's the parity smoke test.
5. The native app also interoperates with the Wails desktop (`192.168.x.x` from `wails build`'s output) and any third-party Dukto peer, since all three speak the protocol in [`docs/PROTOCOL.md`](../docs/PROTOCOL.md).
