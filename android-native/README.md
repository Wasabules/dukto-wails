# Dukto — Native Android port (work in progress)

Parallel rewrite of the Android app in **Kotlin + Jetpack Compose**, sharing nothing with the Qt6 build at the repo root. Both APKs install side-by-side on the same device:

| Build           | `applicationId`                | Source       |
|-----------------|--------------------------------|--------------|
| Qt6 / QML       | `com.github.xuzhen.dukto`      | repo root    |
| Native (this)   | `dev.wasabules.dukto`          | `android-native/` |

The Qt build keeps shipping until this port reaches feature parity. Goal once it does: delete the entire Qt tree (it's already off the desktop, and the desktop now ships from `wails/`).

## Why the rewrite

- APK size: ~19–22 MB → ~3–5 MB (no Qt6 + QtQuick + Quick Controls bundled)
- Material You / dynamic colors / edge-to-edge / predictive back / proper insets
- Storage Access Framework for the destination directory and the share-with-Dukto flow (no more fighting Android scoped storage)
- WorkManager / foreground service for in-flight transfers (Doze-friendly, survives backgrounding)
- Native runtime permissions (`POST_NOTIFICATIONS` on Android 13+, etc.)
- First-class Android Studio profiler / layout inspector / tracing
- Lets us delete `qml/`, `androidutils.{h,cpp}`, `android/`, `dukto.pro`, `CMakeLists.txt`, `modules/SingleApplication` from the repo once parity is reached

## Stack

- AGP 8.7 + Kotlin 2.1 (configured via `gradle/libs.versions.toml`)
- Jetpack Compose (BOM 2024.12) + Material 3
- `compileSdk = 36`, `minSdk = 24` (Android 7.0+), `targetSdk = 36`
- JVM toolchain 17

## First-time setup

There's no `gradlew` shipped — bootstrap the wrapper once with whichever path is convenient:

**Option A — Android Studio** (recommended): open `android-native/` in Android Studio. It will prompt to install the matching Gradle distribution and generate the wrapper for you, then the project syncs.

**Option B — system Gradle**: install Gradle (e.g. `sdk install gradle 8.10` via SDKMAN, or `apt install gradle`) and run once at the repo root of this folder:

```sh
cd android-native
gradle wrapper --gradle-version 8.10
```

That writes `gradlew`, `gradlew.bat`, and `gradle/wrapper/gradle-wrapper.{jar,properties}`. From then on use `./gradlew` for everything.

## Common commands (after the wrapper exists)

```sh
./gradlew assembleDebug           # debug APK → app/build/outputs/apk/debug/
./gradlew installDebug            # install on the connected device/emulator
./gradlew test                    # JVM unit tests
./gradlew connectedDebugAndroidTest   # instrumentation tests on a device
./gradlew assembleRelease         # unsigned release APK
```

The dev/Qt builds use distinct `applicationId`s, so `installDebug` here will not touch the Qt APK (and vice versa).

## Layout

```
android-native/
├── app/
│   ├── build.gradle.kts
│   └── src/main/
│       ├── AndroidManifest.xml
│       ├── kotlin/dev/wasabules/dukto/
│       │   ├── MainActivity.kt          — Compose entry, edge-to-edge
│       │   ├── ui/theme/                — Material 3 + dynamic colors
│       │   ├── protocol/                — wire format port (TODO)
│       │   ├── discovery/               — UDP messenger (TODO)
│       │   ├── transfer/                — TCP receiver/sender (TODO)
│       │   └── platform/                — OS identity / device name
│       └── res/
└── README.md
```

## Migration checklist

Sources of truth to mirror:

- Wire format: [`docs/PROTOCOL.md`](../docs/PROTOCOL.md) (frozen — interop with the Wails desktop and third-party Dukto peers).
- Reference Go port: [`wails/internal/protocol`](../wails/internal/protocol), [`discovery`](../wails/internal/discovery), [`transfer`](../wails/internal/transfer).
- Original Qt Android code: `network/`, `androidutils.cpp`, `qml/new/`.

### Phase 1 — wire format port

- [x] `protocol`: `BuddyMessage` encode/decode (UDP datagrams 0x01–0x05)
- [x] `protocol`: `SessionHeader` + `ElementHeader` streaming codec (TCP)
- [x] `protocol`: `buildSignature("<user> at <host> (Android)")`
- [x] JVM round-trip + invalid-input tests (18/18)
- [ ] Cross-stack fixture tests: feed the same `tests/fixtures/*.bin` the Go side uses — left for follow-up once Qt fixture generator runs in CI.

### Phase 2 — networking

- [x] `discovery.Messenger`: HELLO/GOODBYE, self-echo suppression, periodic broadcast, `WifiManager.MulticastLock`
- [x] `transfer.Server`: TCP server on port 4644, accept loop, per-session coroutines
- [x] `transfer.Receiver`: streaming parse → files under `getExternalFilesDir(DIRECTORY_DOWNLOADS)/dukto-<ts>-<src>/`
- [x] `transfer.Sender`: text snippet + multi-URI files
- [ ] Per-source HELLO cooldown / broadcast-storm guard (security hardening, can ride on top later)
- [ ] Avatar HTTP side-channel on `udp_port + 1`
- [ ] SAF tree picker for destination (currently uses app-private external storage — works without permissions but less discoverable)
- [ ] Folder send (recursive directory traversal of a SAF tree)

### Phase 3 — UI (Compose)

- [x] Top-level `DuktoScreen` with peer list + recent activity + in-flight progress bar
- [x] Settings bottom sheet (display name)
- [x] Send bottom sheet (text snippet + file picker entry point)
- [x] Material 3 + Material You dynamic colors (Android 12+)
- [ ] Profile avatar (camera/gallery picker, expose via the existing avatar HTTP endpoint once Phase 2 ships it)
- [ ] Cancel in-flight transfer
- [ ] About / terms-of-use first-run screen

### Phase 4 — Android plumbing

- [x] Notification channel created at app start
- [x] Notification permission request on Android 13+
- [x] Share intent: `ACTION_SEND` / `ACTION_SEND_MULTIPLE` → URIs surfaced as a "ready to send" banner
- [ ] Foreground service for active transfers (`FOREGROUND_SERVICE_DATA_SYNC`) — current implementation runs server in a process-scope coroutine, fine for foreground, fragile in background; add the service when Phase 2 SAF tree-picker lands
- [ ] Per-transfer progress notifications

### Phase 5 — release

- [x] `.github/workflows/build-android-native.yml` — debug + unsigned-release APKs as artifacts on push
- [x] `release.yml`: `android-native` job that ships `dukto-android-native-x.y.z-unsigned.apk` alongside the Qt APKs on tag push
- [ ] Signing keystore (out-of-band, like the Qt APKs)
- [ ] Once usage confirms parity → drop the Qt build (see [`docs/PORT_SCOPE.md`](../docs/PORT_SCOPE.md) "Scope of the Qt6 codebase after the port")

## Testing alongside the Qt APK

1. Install the Qt APK on the phone (current release): `adb install dukto-android-6.2.0-arm64_v8a.apk`
2. Build + install the native one: `./gradlew installDebug`
3. Both apps appear separately on the home screen — labeled "Dukto" (Qt) and "Dukto Native" (this).
4. Discover each other on the same Wi-Fi as if they were two distinct peers — that's the parity smoke test.
