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

- [ ] `protocol`: `BuddyMessage` encode/decode (UDP datagrams 0x01–0x05)
- [ ] `protocol`: `SessionHeader` + `ElementHeader` streaming codec (TCP)
- [ ] `protocol`: `BuildSignature("<user> at <host> (Android)")`
- [ ] Cross-stack fixture tests: feed the same `tests/fixtures/*.bin` the Go side uses (see `docs/PROTOCOL.md` §7), assert byte-for-byte parity.

### Phase 2 — networking

- [ ] `discovery.Messenger`: HELLO/GOODBYE, self-echo suppression, per-source HELLO cooldown, broadcast-storm guard, `WifiManager.MulticastLock`
- [ ] `transfer.Server`: TCP server bound to `0.0.0.0:4644`, accept loop with policy hook
- [ ] `transfer.Receiver`: streaming parse → `DocumentFile` outputs in user-selected destination tree (SAF)
- [ ] `transfer.Sender`: send files / folders / text / clipboard
- [ ] Avatar HTTP side-channel on `udp_port + 1`

### Phase 3 — UI (Compose)

- [ ] Buddies / peer list (mirror `qml/new/BuddiesPage.qml`)
- [ ] Recent activity list
- [ ] Profile: editable buddy name + avatar (camera / gallery picker)
- [ ] Send composer: file picker (SAF), folder picker, text input, clipboard
- [ ] In-progress transfer screen: speed / ETA / cancel
- [ ] Settings: destination tree (SAF), notifications, theme
- [ ] About / terms-of-use first-run screen

### Phase 4 — Android plumbing

- [ ] Foreground service for active transfers (`FOREGROUND_SERVICE_DATA_SYNC`)
- [ ] Notification channel + rich notifications with progress
- [ ] Share intent: `ACTION_SEND` / `ACTION_SEND_MULTIPLE` → preselect peer
- [ ] Notification permission flow (Android 13+)
- [ ] System dark mode + Material You dynamic colors (already wired)

### Phase 5 — release

- [ ] CI workflow under `.github/workflows/build-android-native.yml` (parallel of `build-android.yml`)
- [ ] Sign with a release keystore (out-of-band, like the Qt APKs are today)
- [ ] Update `release.yml` to ship `dukto-android-native-x.y.z-{abi}.apk`
- [ ] Once usage confirms parity → drop the Qt build (see [`docs/PORT_SCOPE.md`](../docs/PORT_SCOPE.md) "Scope of the Qt6 codebase after the port")

## Testing alongside the Qt APK

1. Install the Qt APK on the phone (current release): `adb install dukto-android-6.2.0-arm64_v8a.apk`
2. Build + install the native one: `./gradlew installDebug`
3. Both apps appear separately on the home screen — labeled "Dukto" (Qt) and "Dukto Native" (this).
4. Discover each other on the same Wi-Fi as if they were two distinct peers — that's the parity smoke test.
