# Dukto

Dukto is an easy file transfer tool for LAN. It was created by Emanuele Colombo, and ported to Qt 5/6 by [xuzhen and other contributors](https://github.com/xuzhen/dukto/graphs/contributors). This fork maintains the Qt Android build **and** ships a Wails v2 / Go / Svelte-TS rewrite of the desktop frontend.

Now it supports Windows, Linux, MacOS and Android.

## Warning
Dukto transfers files and text without encryption and is only designed for use in trusted network environments.

## Repo layout

This repository hosts two codebases that speak the same LAN protocol:

- **Root tree (this directory)** — the original Qt6/QML app. Still the source of truth for the **Android** build.
- **`wails/`** — Wails v2 + Go + Svelte-TS rewrite of the **desktop** frontend (Windows, macOS, Linux). Has feature parity with Qt6 for transfer/discovery, plus extra security hardening. See [`wails/README.md`](wails/README.md).

The wire format both trees implement is documented in [`docs/PROTOCOL.md`](docs/PROTOCOL.md); the port plan and coexistence rules are in [`docs/PORT_SCOPE.md`](docs/PORT_SCOPE.md).

## Specs at a glance

| | |
|---|---|
| **Current version** | 6.2.0 (see `version.h`) |
| **Supported OS** | Windows 10+, macOS 11+, Linux, Android 8.0+ (Qt6 APK) / Android 5.0+ (Qt5 APK) |
| **Network** | IPv4 only, UDP + TCP on port `4644` (configurable); avatar HTTP on `udp_port + 1` |
| **Encryption** | None — trusted LAN only |
| **Discovery** | UDP broadcast on every up IPv4 non-loopback interface |
| **Wire format** | Little-endian, framed per datagram / per element. See `docs/PROTOCOL.md`. |
| **Settings store (Qt)** | `QSettings` under `msec.it/Dukto` (registry / plist / `~/.config/msec.it/Dukto.conf`) |
| **Settings store (Wails)** | JSON under `<UserConfigDir>/dukto/` — one-time migration from the Qt store on first run |
| **Runtime deps (Qt desktop)** | Qt 5.3+ or Qt 6.x; libnotify (optional, Linux) |
| **Runtime deps (Wails)** | WebView2 (Windows), WebKitGTK (Linux), WKWebView (macOS, system-provided) |

## Feature comparison — Qt6 vs Wails port

| Feature                                            | Qt6 (root tree) | Wails (`wails/`)              |
|----------------------------------------------------|:---------------:|:------------------------------:|
| Send/receive files, folders, text                  | ✅              | ✅                             |
| Clipboard text / paste-image-to-send               | ✅              | ✅                             |
| Screen capture send                                 | ✅              | ⏳ not yet ported              |
| Recent activity list (persistent)                  | ✅              | ✅                             |
| Buddy name, avatar, avatar HTTP side-channel       | ✅              | ✅                             |
| Dark/light/auto theme detection (OS-native)        | ✅              | ✅                             |
| Custom theme colour picker                         | ✅              | ❌ fixed palette               |
| System tray + close-to-tray                        | ✅              | ✅                             |
| Receive notifications                               | ✅              | ✅                             |
| Cross-subnet manual peers                           | ❌              | ✅                             |
| Per-interface send/listen allow-list                | ❌              | ✅                             |
| Whitelist (only-approved-buddies mode)             | ❌              | ✅                             |
| Block list (hard-reject by signature)              | ❌              | ✅                             |
| Confirm unknown peers (first-session modal)        | ❌              | ✅ (60 s timeout)              |
| Auto-reject by extension                            | ❌              | ✅                             |
| Large-session size threshold                        | ❌              | ✅                             |
| Max files / max path depth per session              | ❌              | ✅                             |
| Minimum free-disk-space guard                       | ❌              | ✅                             |
| TCP per-IP accept cooldown                          | ❌              | ✅                             |
| UDP HELLO per-IP cooldown                           | ❌              | ✅                             |
| Receiving master switch + idle auto-disable        | ❌              | ✅                             |
| Audit log (append-only, rotated, 0o600)             | ❌              | ✅ viewable in-app             |
| Speed + ETA in progress bar                         | Partial         | ✅                             |
| Cancel transfer mid-session                         | ❌              | ✅                             |
| Keyboard shortcuts                                   | Partial         | ✅                             |
| Single-instance enforcement                          | ✅ (`SingleApplication`) | ⏳ not yet ported  |
| Windows taskbar progress (`ITaskbarList3`)          | ✅              | ⏳ not yet ported              |
| Android target                                       | ✅              | ❌ out of scope                |

The Wails port is the long-term desktop frontend; the Qt tree will be pared down to Android-only once the Wails builds replace the Qt desktop builds for real users. See `docs/PORT_SCOPE.md` for the transition plan.

### Prebuilt Packages

#### Windows
Portable versions can be downloaded from [the releases page](https://github.com/xuzhen/dukto/releases)

The Qt6 version supports Windows 10+ only. If you are still using Windows 7, download the Qt5 version instead.

If you can not open the 7z files, visit https://7-zip.org/ and install 7-zip

If you get `The program can't start because MSVCP140.dll is missing from your computer. Try reinstalling the program to fix this problem` error , download and install the Visual C++ Redistributable packages for VS2015-2022 from [Microsoft](https://learn.microsoft.com/en-US/cpp/windows/latest-supported-vc-redist#visual-studio-2015-2017-2019-and-2022). 
Direct links: [X64](https://aka.ms/vs/17/release/vc_redist.x64.exe) or [X86](https://aka.ms/vs/17/release/vc_redist.x86.exe)

#### macOS
The universal app for macOS can be downloaded from [the releases page](https://github.com/xuzhen/dukto/releases)

Supports macOS 11+

#### Android
APKs can be downloaded from [the releases page](https://github.com/xuzhen/dukto/releases)

The `dukto_*_qt6.apk` supports Android 8.0 (Oreo) and later.

The `dukto_*_qt5.apk` supports Android 5.0 (Lollipop) and later.

#### Ubuntu and derivatives:
Use [this PPA](https://launchpad.net/~xuzhen666/+archive/ubuntu/dukto) 

### Build from source code

The repo has two build systems — pick whichever matches your target.

#### A. Wails desktop app (Windows / macOS / Linux) — recommended for desktop

**Dependencies**

- Go 1.21+ (1.23 is what the module targets).
- [Wails v2 CLI](https://wails.io/docs/gettingstarted/installation): `go install github.com/wailsapp/wails/v2/cmd/wails@latest`
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

See [`wails/README.md`](wails/README.md) for the full developer guide.

#### B. Qt Dukto (Android target; legacy desktop)

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

| Workflow             | Trigger                           | What it does |
|----------------------|-----------------------------------|--------------|
| `ci.yml`             | Every push / PR                   | Fast lane: `go vet`, `go test ./...` and `npm run check` for the Wails port |
| `build-wails.yml`    | Push / PR touching `wails/`       | `wails build` on Ubuntu, Windows and macOS |
| `build-qt6.yml`      | Push / PR touching Qt sources     | Qt 6.8.1 CMake build on Ubuntu, Windows and macOS |
| `build-qt5.yml`      | Push / PR touching Qt sources     | Qt 5.15.2 CMake build on Ubuntu (legacy coverage) |
| `build-android.yml`  | Push / PR touching Qt sources     | Qt6-Android APK for `arm64_v8a` and `armv7` |
| `release.yml`        | Push of tag `v*`, or manual       | Runs tests + every build target, then creates a GitHub release with all packaged artifacts attached |

### Cutting a release

1. Bump `version.h` (both `#define VERSION_*` and the `VERSION=x.y.z` line) and commit.
2. Tag the commit `vX.Y.Z` and push the tag:
   ```sh
   git tag v6.2.0
   git push origin v6.2.0
   ```
3. `release.yml` runs on the tag push: gates on `go test` + `svelte-check`, then builds Wails (3 OS), Qt6 desktop (3 OS), Qt5 desktop (Linux) and Qt6-Android (arm64-v8a + armv7) in parallel. The final `publish` job downloads every artifact and creates a GitHub release titled `Dukto X.Y.Z` with auto-generated notes.
4. Android APKs are currently unsigned — sign them out-of-band before distributing if needed.
5. `workflow_dispatch` is also available from the Actions tab for re-runs; pass the existing tag as input.
