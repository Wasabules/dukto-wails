package dev.wasabules.dukto.protocol

import java.io.IOException
import java.io.InputStream
import java.io.OutputStream
import java.nio.ByteBuffer
import java.nio.ByteOrder

/**
 * Wire format for Dukto's UDP discovery and TCP transfer.
 *
 * Authoritative spec: docs/PROTOCOL.md
 * Reference Go port:  wails/internal/protocol/{discovery,transfer,protocol}.go
 *
 * Constraints (frozen — do not change without coordinating across stacks):
 *  - Little-endian integer I/O regardless of host byte order.
 *  - Platform token in signature: one of PlatformWindows/Macintosh/Linux/Android/Unknown.
 *  - Text-snippet magic: ___DUKTO___TEXT___
 *  - Directory sentinel: element_size == -1
 *  - Default port: 4644 UDP + TCP, avatar HTTP at port + 1.
 */

const val DEFAULT_PORT: Int = 4644
const val AVATAR_PORT_OFFSET: Int = 1

const val PLATFORM_WINDOWS = "Windows"
const val PLATFORM_MACINTOSH = "Macintosh"
const val PLATFORM_LINUX = "Linux"
const val PLATFORM_ANDROID = "Android"
const val PLATFORM_UNKNOWN = "Unknown"

const val TEXT_ELEMENT_NAME = "___DUKTO___TEXT___"
const val DIRECTORY_SIZE_MARKER: Long = -1L

/**
 * Builds the UDP HELLO signature. The literal " at " and the parens around the
 * platform token are load-bearing — legacy peers display the full string in
 * their UI and (older builds) parse it heuristically.
 */
fun buildSignature(user: String, host: String, platform: String): String =
    "$user at $host ($platform)"

// ─────────────────────────────────────────────────────────────────────────────
// UDP discovery
// ─────────────────────────────────────────────────────────────────────────────

enum class MessageType(val code: Byte) {
    HelloBroadcast(0x01),
    HelloUnicast(0x02),
    Goodbye(0x03),
    HelloPortBroadcast(0x04),
    HelloPortUnicast(0x05);

    val hasPort: Boolean get() = this == HelloPortBroadcast || this == HelloPortUnicast
    val hasSignature: Boolean get() = this != Goodbye

    companion object {
        fun fromCode(b: Byte): MessageType? = entries.firstOrNull { it.code == b }
    }
}

/**
 * Hello broadcast type for a peer listening on [localPort]. Default port → 0x01,
 * non-default → 0x04 (carries the port). Mirrors Qt's selection rule.
 */
fun helloBroadcastType(localPort: Int): MessageType =
    if (localPort == DEFAULT_PORT) MessageType.HelloBroadcast else MessageType.HelloPortBroadcast

fun helloUnicastType(localPort: Int): MessageType =
    if (localPort == DEFAULT_PORT) MessageType.HelloUnicast else MessageType.HelloPortUnicast

class InvalidMessageException(message: String) : IOException(message)

/**
 * A decoded UDP discovery datagram.
 *
 * @property port meaningful only for HelloPort* types; ignored for others on serialize, 0 after parse.
 * @property signature meaningful for every type except Goodbye.
 */
data class BuddyMessage(
    val type: MessageType,
    val port: Int = 0,
    val signature: String = "",
) {
    /**
     * Encode to wire bytes. Does not validate; call [validate] first if needed.
     */
    fun serialize(): ByteArray {
        val sigBytes = if (type.hasSignature) signature.toByteArray(Charsets.UTF_8) else ByteArray(0)
        val size = 1 + (if (type.hasPort) 2 else 0) + sigBytes.size
        val buf = ByteBuffer.allocate(size).order(ByteOrder.LITTLE_ENDIAN)
        buf.put(type.code)
        if (type.hasPort) buf.putShort(port.toShort())
        if (type.hasSignature) buf.put(sigBytes)
        return buf.array()
    }

    /**
     * Reject obviously malformed messages before sending. Matches the Qt
     * BuddyMessage::parse rules: HelloPort* with port=0 is invalid;
     * non-Goodbye with empty signature is invalid.
     */
    fun validate() {
        if (type.hasPort && port == 0) {
            throw InvalidMessageException("port-carrying type ${typeHex()} with zero port")
        }
        if (type.hasSignature && signature.isEmpty()) {
            throw InvalidMessageException("type ${typeHex()} requires a signature")
        }
    }

    private fun typeHex(): String = "0x%02x".format(type.code)

    companion object {
        fun goodbye(): BuddyMessage = BuddyMessage(MessageType.Goodbye)

        /**
         * Decode a UDP datagram. Throws [InvalidMessageException] on empty input,
         * unknown type byte, port-carrying type with port=0, or non-goodbye with
         * empty signature. Mirrors the Go ParseBuddyMessage rules.
         */
        @Throws(InvalidMessageException::class)
        fun parse(data: ByteArray): BuddyMessage {
            if (data.isEmpty()) throw InvalidMessageException("empty datagram")
            val type = MessageType.fromCode(data[0])
                ?: throw InvalidMessageException("unknown type 0x%02x".format(data[0]))
            var off = 1
            var port = 0
            if (type.hasPort) {
                if (data.size < off + 2) throw InvalidMessageException("port-carrying type truncated")
                port = ByteBuffer.wrap(data, off, 2).order(ByteOrder.LITTLE_ENDIAN).short.toInt() and 0xFFFF
                off += 2
                if (port == 0) throw InvalidMessageException("zero port in type 0x%02x".format(type.code))
            }
            var sig = ""
            if (type.hasSignature) {
                if (off >= data.size) {
                    throw InvalidMessageException("missing signature in type 0x%02x".format(type.code))
                }
                sig = String(data, off, data.size - off, Charsets.UTF_8)
            }
            return BuddyMessage(type, port, sig)
        }
    }
}

// ─────────────────────────────────────────────────────────────────────────────
// TCP transfer
// ─────────────────────────────────────────────────────────────────────────────

class InvalidStreamException(message: String) : IOException(message)

/**
 * 16-byte session header at the start of a TCP transfer stream.
 *
 * @property totalElements files + directories + the synthetic text element if any. Must be > 0.
 * @property totalSize sum of file sizes; directories contribute 0; text snippets contribute UTF-8 byte length. Must be >= 0.
 */
data class SessionHeader(val totalElements: Long, val totalSize: Long)

/**
 * Per-element header in a TCP transfer stream: <utf8-name>\x00<size-le-i64>.
 *
 * @property name path with '/' separator (regardless of host OS). Must be non-empty.
 * @property size -1 = directory marker (no bytes follow); 0 = empty file or zero-length text; >0 = byte count.
 *               <-1 is invalid and aborts the session.
 */
data class ElementHeader(val name: String, val size: Long) {
    val isDirectory: Boolean get() = size == DIRECTORY_SIZE_MARKER
    val isText: Boolean get() = name == TEXT_ELEMENT_NAME
}

@Throws(IOException::class)
fun writeSessionHeader(out: OutputStream, h: SessionHeader) {
    val buf = ByteBuffer.allocate(16).order(ByteOrder.LITTLE_ENDIAN)
    buf.putLong(h.totalElements)
    buf.putLong(h.totalSize)
    out.write(buf.array())
}

@Throws(IOException::class)
fun readSessionHeader(input: InputStream): SessionHeader {
    val data = readFully(input, 16)
    val buf = ByteBuffer.wrap(data).order(ByteOrder.LITTLE_ENDIAN)
    val h = SessionHeader(buf.long, buf.long)
    if (h.totalElements <= 0L) throw InvalidStreamException("zero or negative TotalElements")
    if (h.totalSize < 0L) throw InvalidStreamException("negative TotalSize ${h.totalSize}")
    return h
}

@Throws(IOException::class)
fun writeElementHeader(out: OutputStream, h: ElementHeader) {
    if (h.name.isEmpty()) throw InvalidStreamException("empty element name")
    out.write(h.name.toByteArray(Charsets.UTF_8))
    out.write(0)
    val buf = ByteBuffer.allocate(8).order(ByteOrder.LITTLE_ENDIAN)
    buf.putLong(h.size)
    out.write(buf.array())
}

/**
 * Read a NUL-terminated UTF-8 name followed by the 8-byte little-endian size.
 * Caller-supplied [readByte] returns -1 at EOF and is expected to be a buffered
 * source (e.g. BufferedInputStream or DataInputStream wrapping a Socket).
 */
@Throws(IOException::class)
fun readElementHeader(input: InputStream): ElementHeader {
    // Read until NUL; cap at a sane limit to bound memory if peer is hostile.
    val sb = ByteArrayOutputStreamSized(256)
    while (true) {
        val b = input.read()
        if (b < 0) throw IOException("unexpected EOF in element name")
        if (b == 0) break
        sb.write(b)
        if (sb.size() > 4096) throw InvalidStreamException("element name exceeds 4096 bytes")
    }
    val name = String(sb.toByteArray(), Charsets.UTF_8)
    if (name.isEmpty()) throw InvalidStreamException("empty element name")

    val sz = readFully(input, 8)
    val size = ByteBuffer.wrap(sz).order(ByteOrder.LITTLE_ENDIAN).long
    if (size < DIRECTORY_SIZE_MARKER) {
        throw InvalidStreamException("element size $size below -1")
    }
    return ElementHeader(name, size)
}

// ─────────────────────────────────────────────────────────────────────────────
// I/O helpers
// ─────────────────────────────────────────────────────────────────────────────

/** Read exactly [n] bytes or throw EOF. */
@Throws(IOException::class)
internal fun readFully(input: InputStream, n: Int): ByteArray {
    val out = ByteArray(n)
    var off = 0
    while (off < n) {
        val r = input.read(out, off, n - off)
        if (r < 0) throw IOException("unexpected EOF after $off/$n bytes")
        off += r
    }
    return out
}

/**
 * Tiny resizable byte buffer (avoids pulling in java.io.ByteArrayOutputStream's
 * synchronisation overhead in hot inner loops).
 */
internal class ByteArrayOutputStreamSized(initial: Int) {
    private var buf = ByteArray(initial)
    private var count = 0
    fun write(b: Int) {
        if (count == buf.size) buf = buf.copyOf(buf.size * 2)
        buf[count++] = b.toByte()
    }
    fun size(): Int = count
    fun toByteArray(): ByteArray = buf.copyOf(count)
}
