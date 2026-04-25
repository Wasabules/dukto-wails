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
    HelloPortUnicast(0x05),
    /**
     * v2 broadcast HELLO with embedded Ed25519 pubkey + signature over
     * `port_le ‖ utf-8(signature)`. Carries the same information as
     * [HelloPortBroadcast] plus the long-term identity. Legacy peers reject
     * any byte > 0x05 and ignore the datagram silently, so a v2 peer sends
     * both 0x04 and 0x06 every HELLO interval.
     */
    HelloPortKeyBroadcast(0x06),
    /** v2 unicast reply, same payload shape as [HelloPortKeyBroadcast]. */
    HelloPortKeyUnicast(0x07);

    val hasPort: Boolean
        get() = this == HelloPortBroadcast || this == HelloPortUnicast ||
            this == HelloPortKeyBroadcast || this == HelloPortKeyUnicast
    val hasSignature: Boolean get() = this != Goodbye
    val hasKey: Boolean
        get() = this == HelloPortKeyBroadcast || this == HelloPortKeyUnicast

    companion object {
        fun fromCode(b: Byte): MessageType? = entries.firstOrNull { it.code == b }
    }
}

/** On-the-wire Ed25519 pubkey length carried in 0x06/0x07. */
const val ED25519_PUBLIC_KEY_SIZE: Int = 32

/** On-the-wire Ed25519 signature length carried in 0x06/0x07. */
const val ED25519_SIGNATURE_SIZE: Int = 64

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
    /** Raw Ed25519 public key (32 B). Populated only for v2 types. */
    val pubKey: ByteArray? = null,
    /** Ed25519 signature (64 B) over [signedPayload]. Populated only for v2 types. */
    val sig: ByteArray? = null,
) {
    /**
     * Encode to wire bytes. Does not validate; call [validate] first if needed.
     *
     * v2 wire layout (0x06/0x07):
     *   type (1B) ‖ port (LE u16, 2B) ‖ pub_key (32B) ‖ sig (64B) ‖ utf-8 signature
     */
    fun serialize(): ByteArray {
        val sigBytes = if (type.hasSignature) signature.toByteArray(Charsets.UTF_8) else ByteArray(0)
        val keyBytes = if (type.hasKey) (pubKey ?: ByteArray(0)) else ByteArray(0)
        val sigField = if (type.hasKey) (sig ?: ByteArray(0)) else ByteArray(0)
        val size = 1 + (if (type.hasPort) 2 else 0) + keyBytes.size + sigField.size + sigBytes.size
        val buf = ByteBuffer.allocate(size).order(ByteOrder.LITTLE_ENDIAN)
        buf.put(type.code)
        if (type.hasPort) buf.putShort(port.toShort())
        if (type.hasKey) {
            buf.put(keyBytes)
            buf.put(sigField)
        }
        if (type.hasSignature) buf.put(sigBytes)
        return buf.array()
    }

    /**
     * Bytes covered by the Ed25519 signature in v2 HELLOs: little-endian port
     * followed by the utf-8 signature string.
     */
    fun signedPayload(): ByteArray {
        val sigBytes = signature.toByteArray(Charsets.UTF_8)
        val buf = ByteBuffer.allocate(2 + sigBytes.size).order(ByteOrder.LITTLE_ENDIAN)
        buf.putShort(port.toShort())
        buf.put(sigBytes)
        return buf.array()
    }

    /**
     * Reject obviously malformed messages before sending. Matches the Qt
     * BuddyMessage::parse rules: HelloPort* with port=0 is invalid;
     * non-Goodbye with empty signature is invalid; key-bearing types must
     * carry a 32-byte pubkey + 64-byte sig.
     */
    fun validate() {
        if (type.hasPort && port == 0) {
            throw InvalidMessageException("port-carrying type ${typeHex()} with zero port")
        }
        if (type.hasSignature && signature.isEmpty()) {
            throw InvalidMessageException("type ${typeHex()} requires a signature")
        }
        if (type.hasKey) {
            if (pubKey?.size != ED25519_PUBLIC_KEY_SIZE) {
                throw InvalidMessageException("type ${typeHex()} pubkey must be $ED25519_PUBLIC_KEY_SIZE bytes")
            }
            if (sig?.size != ED25519_SIGNATURE_SIZE) {
                throw InvalidMessageException("type ${typeHex()} sig must be $ED25519_SIGNATURE_SIZE bytes")
            }
        }
    }

    private fun typeHex(): String = "0x%02x".format(type.code)

    // Generated equals/hashCode aren't safe for ByteArray fields — override
    // to compare contents. Without this, two semantically-equal BuddyMessages
    // would diverge based on identity.
    override fun equals(other: Any?): Boolean {
        if (this === other) return true
        if (other !is BuddyMessage) return false
        if (type != other.type || port != other.port || signature != other.signature) return false
        val a = pubKey; val b = other.pubKey
        if ((a == null) != (b == null)) return false
        if (a != null && b != null && !a.contentEquals(b)) return false
        val c = sig; val d = other.sig
        if ((c == null) != (d == null)) return false
        if (c != null && d != null && !c.contentEquals(d)) return false
        return true
    }

    override fun hashCode(): Int {
        var h = type.hashCode()
        h = 31 * h + port
        h = 31 * h + signature.hashCode()
        h = 31 * h + (pubKey?.contentHashCode() ?: 0)
        h = 31 * h + (sig?.contentHashCode() ?: 0)
        return h
    }

    companion object {
        fun goodbye(): BuddyMessage = BuddyMessage(MessageType.Goodbye)

        /**
         * Decode a UDP datagram. Throws [InvalidMessageException] on empty input,
         * unknown type byte, port-carrying type with port=0, or non-goodbye with
         * empty signature. Mirrors the Go ParseBuddyMessage rules. v2 messages
         * (0x06/0x07) are accepted with raw pubkey + sig fields populated;
         * callers must invoke [verifyKey] before trusting them.
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
            var pub: ByteArray? = null
            var sigBytes: ByteArray? = null
            if (type.hasKey) {
                val need = ED25519_PUBLIC_KEY_SIZE + ED25519_SIGNATURE_SIZE
                if (data.size < off + need) {
                    throw InvalidMessageException("type 0x%02x truncated key/sig".format(type.code))
                }
                pub = data.copyOfRange(off, off + ED25519_PUBLIC_KEY_SIZE)
                off += ED25519_PUBLIC_KEY_SIZE
                sigBytes = data.copyOfRange(off, off + ED25519_SIGNATURE_SIZE)
                off += ED25519_SIGNATURE_SIZE
            }
            var sig = ""
            if (type.hasSignature) {
                if (off >= data.size) {
                    throw InvalidMessageException("missing signature in type 0x%02x".format(type.code))
                }
                sig = String(data, off, data.size - off, Charsets.UTF_8)
            }
            return BuddyMessage(type, port, sig, pub, sigBytes)
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
