# Dukto Wire Protocol

Reference specification of the over-the-wire protocol implemented by the current Qt6 Dukto application. This is the **interop contract**: any re-implementation (e.g. the planned Wails/Go port) must reproduce these bytes exactly, because existing Dukto peers on the LAN speak this format and nothing else.

Authoritative sources in the codebase:

- `duktoprotocol.cpp` — port defaults, server setup, session orchestration.
- `network/messenger.cpp`, `network/buddymessage.cpp` — UDP discovery.
- `network/sender.cpp`, `network/receiver.cpp` — TCP transfer.
- `network/filedata.cpp` — file list generation and relative-path naming.
- `miniwebserver.cpp` — out-of-band avatar HTTP endpoint.

If this document and the code ever diverge, the code wins and this file must be updated.

## 1. Ports

| Purpose           | Default port | Protocol | Bind address     |
|-------------------|--------------|----------|------------------|
| Discovery         | 4644         | UDP      | `0.0.0.0`        |
| File/text transfer| 4644         | TCP      | `0.0.0.0`        |
| Avatar HTTP       | 4645         | TCP      | `0.0.0.0`        |

A peer **may** run its UDP/TCP servers on a non-default port; when it does, it advertises the port in its HELLO message (see §2.1, types `0x04`/`0x05`). The avatar HTTP port is always `udp_port + 1` in the current implementation, and is not announced — clients fetch `http://<peer-ip>:<udp_port+1>/dukto/avatar`.

Only IPv4 is used. IPv6 is not implemented anywhere in the protocol stack.

## 2. Discovery (UDP)

### 2.1 Datagram format

A datagram is `type-byte || [port] || [signature]` where the first byte is the message type. No length prefix, no checksum, no framing — datagram boundaries are the frame.

| Type  | Name                      | Port field | Signature field | Direction                          |
|-------|---------------------------|------------|-----------------|-------------------------------------|
| 0x01  | `HELLO_BROADCAST`         | no         | yes             | broadcast; sender listens on 4644  |
| 0x02  | `HELLO_UNICAST`           | no         | yes             | unicast reply to a broadcast       |
| 0x03  | `GOODBYE`                 | no         | no              | broadcast on shutdown              |
| 0x04  | `HELLO_PORT_BROADCAST`    | yes (u16)  | yes             | broadcast; sender listens on custom port |
| 0x05  | `HELLO_PORT_UNICAST`      | yes (u16)  | yes             | unicast reply; sender uses custom port |

Any other first byte ⇒ datagram ignored.

**Port field** (types `0x04`/`0x05` only): 2 bytes, **host-endian 16-bit unsigned**. In the Qt codebase this is a raw `memcpy` of a `quint16`, so on every supported desktop/mobile CPU (x86_64, ARM64, ARMv7, x86) it is **little-endian**. A re-implementation MUST write and read little-endian. If `port == 0`, the datagram is treated as invalid.

**Signature field**: UTF-8 bytes, no terminator, no length prefix — it runs to the end of the datagram. Empty signature on a non-GOODBYE message ⇒ invalid.

### 2.2 Signature format

```
<username> at <hostname> (<platform>)
```

Built in `Messenger::getSystemSignature()`.

- `<username>` — value of the user-configured buddy name if set, otherwise the OS login name (`GetUserName` on Windows, `$USER` on POSIX, the literal `"User"` on Android). Capitalised on the first letter. Defaults to `"Unknown"` if both sources are empty.
- `<hostname>` — `QHostInfo::localHostName()` with trailing `.local` stripped (macOS). On Android, the `device_name` global setting or Build `MODEL`, with spaces replaced by `-`.
- `<platform>` — exactly one of: `Windows`, `Macintosh`, `Android`, `Linux`, `Unknown`. (A Wails/Go desktop port must emit one of these tokens, not new ones, or pre-6.2 peers may render the logo as "unknown".)

The literal ` at ` (space-at-space) between username and hostname is load-bearing — peers do not strictly parse it, but users see the full string in the UI.

### 2.3 Sending behaviour

On startup (`Messenger::sayHello()` / `DuktoProtocol::greeting()`):

1. For every UP IPv4 non-loopback interface, record each local address.
2. For each such interface, send the HELLO datagram to the interface's IPv4 broadcast address:
   - On the default UDP port (4644), and
   - On **every port** that has been observed in received HELLO messages (deduplicated).
3. Message type is `HELLO_BROADCAST` (`0x01`) if the sender is bound to 4644, else `HELLO_PORT_BROADCAST` (`0x04`) carrying the sender's listen port.

Any peer receiving a broadcast HELLO replies with a **unicast** HELLO (`0x02` or `0x05`) straight to the sender's `(address, port)`. This is what populates the peer list on a newly joined node before the next broadcast tick.

On shutdown: one `GOODBYE` (`0x03`) is broadcast on the same set of (interface × port) pairs.

There is **no periodic keepalive in the core protocol**. The UI layer (`GuiBehind`) re-triggers `greeting()` on a timer, which is what keeps peer lists fresh — a Wails port must do the same or it will drop out of other peers' lists.

### 2.4 Receive-side quirks

- **Self-echo suppression**: `Messenger` remembers all local IPv4 addresses (from the broadcast pass). Datagrams whose source address matches a local address are ignored.
- **Broadcast-storm guard**: if a single source address sends > 5 datagrams between two broadcast passes, its address is added to a permanent `badAddrs` list and ignored for the rest of the session. This was added to tolerate buggy VPN interfaces on Android that echo broadcasts. A re-implementation should keep this guard; its absence will appear as crashes/CPU spikes in pathological network environments.
- A valid HELLO (`0x01` or `0x04`) **always** emits a `buddyFound` event, even if the peer was already known — duplicates are deduplicated upstream in the UI list model by IP address, not by the messenger.
- `GOODBYE` only emits `buddyGone` if the sender was previously in the peers map. A GOODBYE from an unknown peer is silently dropped.

## 3. Transfer (TCP)

One transfer = one TCP connection. The sender connects, writes the stream below, and half-closes / disconnects on completion. The receiver detects end-of-transfer by byte accounting, not by FIN.

Only one transfer may be in progress at a time per side (`DuktoProtocol::newIncomingConnection` rejects a second inbound connection; `sendFile`/`sendText` early-return if a send or receive is already active).

### 3.1 Stream layout

```
session_header := total_elements (u64 LE) || total_size (i64 LE)
element        := name_utf8 || 0x00 || element_size (i64 LE) || element_data
stream         := session_header || element[0] || element[1] || ... || element[total_elements - 1]
```

- `total_elements` (`quint64`) — number of top-level and nested items (files + directories + the synthetic text element). Must be > 0; receiver rejects the session otherwise.
- `total_size` (`qint64`) — sum of file sizes in bytes. Directories contribute 0. Text snippets contribute `utf8_length(text)`. Must be ≥ 0. Used only for progress bars.
- `name_utf8` — UTF-8 bytes, NUL-terminated. Empty name ⇒ session aborted.
- `element_size` (`qint64`) — one of:
  - `-1` → the element is a **directory**; no data bytes follow.
  - `0` → empty file; no data bytes follow.
  - `> 0` → file size in bytes; exactly that many bytes of raw file content follow.
  - The magic text element (see §3.3) sends the UTF-8 byte length of the text in this field.
  - Values `< -1` ⇒ session aborted as invalid.

All multi-byte integers are **host-endian**, which in practice means **little-endian** (same rationale as §2.1). A re-implementation MUST write `<u64|i64>_LE`.

### 3.2 Directory layout

File paths are flattened and transmitted as relative paths with `/` separators, regardless of host OS. The top-level name of a selected file or folder is the `QFileInfo::fileName()` of the user-picked path (no leading directories). Nested entries are `"<top>/subdir/file.txt"`, `"<top>/subdir/nested/file2.txt"`, etc.

Order: depth-first, parent directory before its children, as produced by `QDir::entryList(AllEntries | Hidden | System | NoDotAndDotDot)`. Empty directories are emitted as a single `i64 = -1` element. Files of size 0 are emitted with `i64 = 0` and no data payload.

On the receive side, `Receiver::prepareFilesystem` collision-handles by renaming the **top-level** name: if `foo` exists, `foo (2)`, `foo (3)`, … (starting at 2). The mapping is remembered for the session so that later entries under `foo/…` land under the renamed `foo (2)/…`. Intermediate subdirectories are `mkdir -p`'d; they are **not** collision-renamed.

### 3.3 Text snippets

A text snippet is transported as a single element with the magic name:

```
___DUKTO___TEXT___
```

(`Receiver::textElementName` / `Sender::textElementName`.) The `element_size` field is the UTF-8 byte length of the text, and the data bytes are the UTF-8 text itself — no NUL terminator, no length prefix beyond `element_size`. On the receive side this bypasses the filesystem entirely and is surfaced via `textReceived`.

A session that contains the text magic name MUST contain exactly that one element; mixing text with files in the same session is not implemented.

### 3.4 Screen capture

`sendScreen` is a plain file transfer of a PNG/JPG with the hard-coded name `Screenshot.jpg`. No special framing — the wire is indistinguishable from a single-file send of that name.

### 3.5 Receiver-side completion

The receiver stops reading when `sessionElementsReceived == sessionElements`, emits `completed`, and closes the socket. It does **not** wait for FIN from the sender. The sender, for its part, only calls `disconnectFromHost()` after `bytesToWrite() == 0`. A re-implementation must tolerate the socket being closed by either side once all declared bytes are transferred.

### 3.6 Backpressure

Sender caps its send-buffer backlog at 1 MiB (`if (socket->bytesToWrite() >= 1024 * 1024) return;`) and reads files in 1 MiB chunks. Receiver reads up to 1 MiB per `readyRead`. These numbers are not part of the wire format — they just tune memory usage — but reproducing them will yield similar throughput characteristics.

## 4. Avatar HTTP side-channel

`MiniWebServer` (`miniwebserver.cpp`) listens on TCP port `udp_port + 1` (i.e. 4645 by default) and serves the local user's avatar as a `64×64` PNG. Requests are matched loosely:

- `GET /` → PNG
- `GET /dukto/avatar` → PNG
- `GET /dukto/avatar?<anything>` → PNG

Response: HTTP/1.0, `Content-Type: image/png`, `Content-Length: <n>`, then the PNG bytes. No keep-alive, no compression, no conditional GETs. The server does not start at all if no avatar is resolvable.

This endpoint is discovered purely by convention — peers derive the avatar URL from the peer's IP and the known UDP port + 1. There is no announcement in the HELLO signature.

## 5. Endianness summary

Every multi-byte integer on the wire (UDP port field, TCP session counters, per-element size) is serialised by `reinterpret_cast` over native memory. Every desktop and mobile platform Dukto runs on is little-endian in practice, so the protocol is **de-facto little-endian**. Any re-implementation on a potential big-endian target MUST byte-swap on I/O to interop with existing Dukto peers.

## 6. Things that are NOT in the protocol

The following are enforced by the current app but are **not** visible on the wire — a re-implementation is free to change them:

- Single-transfer-at-a-time (`DuktoProtocol` rejects concurrent sessions).
- The `1 MiB` chunk size in §3.6.
- The `5-datagram` storm threshold in §2.4.
- The 10 s peer-list refresh timer in the UI layer.
- The `(2)`, `(3)` collision-rename scheme on the receive side.

Conversely, the following **are** load-bearing and must be preserved:

- The exact type bytes `0x01..0x05`.
- The `" at "` literal in the signature.
- The `___DUKTO___TEXT___` magic filename.
- The `-1` size-means-directory convention.
- Little-endian integer encoding.
- The avatar port being `udp_port + 1`.

## 7. Generating interop fixtures

The Qt code **is** the reference implementation, so fixtures are produced from it directly rather than captured from the network. Write a small Qt test binary (under `tests/fixture_gen/`) that calls the existing serialisation paths with fixed inputs and writes the raw bytes to files. This yields reproducible, version-controlled fixtures that both the future Wails/Go port and any Qt-side refactoring can test against.

Concretely, emit:

1. **Discovery datagrams** — call `BuddyMessage::serialize()` for each of the five types (`0x01`–`0x05`) with a fixed signature such as `"TestUser at test-host (Linux)"` and a fixed port (e.g. 5000). One file per type: `udp_hello_bcast.bin`, `udp_hello_unicast.bin`, `udp_goodbye.bin`, `udp_hello_port_bcast.bin`, `udp_hello_port_unicast.bin`.
2. **Text snippet** — ASCII (`"hello"`) and multibyte UTF-8 (emoji + CJK). Drive `Sender::sendText` against a `QBuffer` instead of a socket (inject via a minimal shim) and dump the buffer. Files: `tcp_text_ascii.bin`, `tcp_text_utf8.bin`.
3. **Files** — sizes 0 B, small text, 10 MiB binary with a deterministic byte pattern. Files: `tcp_file_empty.bin`, `tcp_file_small.bin`, `tcp_file_10mib.bin`.
4. **Directory tree** — fixed layout with empty dirs and a nested path (≥ 3 levels). File: `tcp_dir_nested.bin`.

For TCP fixtures, the cleanest way is to refactor `Sender` to write to a `QIODevice *` (given in the constructor) rather than hard-coding a `QTcpSocket`. This refactor is worth doing anyway — it makes `Sender` unit-testable — and is a prerequisite for the "extract protocol into a standalone lib" step.

Each fixture file should have a paired `.json` describing the input (filenames, sizes, text content) so decoder tests on either side can assert the reconstructed state.

Wireshark captures remain useful only if we later need to validate against third-party Dukto implementations we don't control. Not required for the Wails/Go port.
