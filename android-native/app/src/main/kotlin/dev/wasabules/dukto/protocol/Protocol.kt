package dev.wasabules.dukto.protocol

/**
 * Wire format for Dukto's UDP discovery and TCP transfer.
 *
 * Reference implementations (must stay byte-compatible):
 *  - Spec: docs/PROTOCOL.md
 *  - Go: wails/internal/protocol/{discovery,transfer,protocol}.go
 *  - C++: network/buddymessage.cpp, network/sender.cpp, network/receiver.cpp
 *
 * Constraints (don't break):
 *  - Little-endian integer I/O regardless of host byte order.
 *  - Platform token in signature is one of the literals
 *    "Windows" / "Macintosh" / "Android" / "Linux" / "Unknown".
 *  - Text-snippet magic: ___DUKTO___TEXT___
 *  - Directory sentinel: element_size == -1
 *  - Default port: 4644 UDP + TCP, avatar HTTP at port + 1.
 *
 * TODO: port BuddyMessage encode/decode (UDP), SessionHeader and
 * ElementHeader streaming codec (TCP), and BuildSignature(username,
 * hostname, platform). Mirror the Go round-trip and parity tests under
 * src/test/kotlin/dev/wasabules/dukto/protocol/.
 */
const val DEFAULT_PORT: UShort = 4644u

const val TEXT_SNIPPET_MAGIC: String = "___DUKTO___TEXT___"
const val DIRECTORY_SENTINEL: Long = -1L
