package dev.wasabules.dukto.tunnel

import org.bouncycastle.math.ec.rfc7748.X25519
import java.io.IOException
import java.io.InputStream
import java.io.OutputStream
import java.io.PushbackInputStream
import java.nio.ByteBuffer
import java.nio.ByteOrder
import java.security.MessageDigest
import java.security.SecureRandom
import javax.crypto.Cipher
import javax.crypto.Mac
import javax.crypto.spec.SecretKeySpec

/**
 * Dukto v2 encrypted tunnel — Noise XX over TCP, mirroring the Wails-side
 * `internal/tunnel` package (Go).
 *
 * Wire format:
 *   8-byte magic `DKTOv2\x00\x00`
 *   Three Noise XX handshake messages, each as `len_le_u16 || ciphertext`
 *   Stream of `len_le_u16 || ChaCha20-Poly1305(plaintext)` transport messages
 *
 * Cipher suite: X25519 / ChaCha20-Poly1305 / SHA-256 — same as WireGuard.
 *
 * See docs/SECURITY_v2.md for the full design.
 */

/** 8-byte magic prefix written by every v2 sender. */
val MAGIC: ByteArray = byteArrayOf(
    'D'.code.toByte(), 'K'.code.toByte(), 'T'.code.toByte(), 'O'.code.toByte(),
    'v'.code.toByte(), '2'.code.toByte(), 0x00, 0x00,
)

const val MAGIC_LEN = 8
const val PUBKEY_LEN = 32
const val DH_LEN = 32
const val KEY_LEN = 32
const val TAG_LEN = 16
const val MAX_MSG_LEN = 65535

/** Result of [peekMagic]: did the prefix match, and an InputStream that
 *  replays the consumed bytes when isV2 is false. */
class PeekResult(val isV2: Boolean, val stream: InputStream)

/**
 * Reads exactly 8 bytes from [src] and reports whether they match [MAGIC].
 * Returns a wrapper input stream that replays the consumed bytes on
 * subsequent reads when isV2 is false, so a legacy SessionHeader parser
 * can run on the unmodified byte stream.
 */
@Throws(IOException::class)
fun peekMagic(src: InputStream): PeekResult {
    val buf = ByteArray(MAGIC_LEN)
    var read = 0
    while (read < MAGIC_LEN) {
        val n = src.read(buf, read, MAGIC_LEN - read)
        if (n < 0) throw IOException("peekMagic: short read at $read/$MAGIC_LEN")
        read += n
    }
    val isV2 = buf.contentEquals(MAGIC)
    val out = if (isV2) src else PushbackInputStream(src, MAGIC_LEN).also { it.unread(buf) }
    return PeekResult(isV2, out)
}

enum class HandshakeRole { Initiator, Responder }

/**
 * Encrypted transport returned by [handshake]. Read/write call into the
 * underlying input/output streams via length-prefixed Noise transport
 * messages.
 *
 * Sessions are not safe for concurrent reads or concurrent writes, but
 * one reader and one writer concurrently is fine (matches `net.Conn`
 * semantics on the Go side).
 */
class Session internal constructor(
    private val input: InputStream,
    private val output: OutputStream,
    internal val sendCS: CipherState,
    internal val recvCS: CipherState,
    /** 32-byte X25519 public key advertised by the remote peer. */
    val remoteStatic: ByteArray,
    /** Closer used by [close] — typically the underlying Socket.close. */
    private val closer: () -> Unit,
) {
    private var rxBuf: ByteArray = ByteArray(0)

    @Throws(IOException::class)
    fun write(plaintext: ByteArray, off: Int = 0, len: Int = plaintext.size) {
        var remaining = len
        var pos = off
        val maxChunk = MAX_MSG_LEN - TAG_LEN
        while (remaining > 0) {
            val n = if (remaining > maxChunk) maxChunk else remaining
            val ct = sendCS.encrypt(null, plaintext.copyOfRange(pos, pos + n))
            writeFrame(output, ct)
            pos += n
            remaining -= n
        }
    }

    @Throws(IOException::class)
    fun read(dst: ByteArray, off: Int = 0, len: Int = dst.size): Int {
        if (len == 0) return 0
        if (rxBuf.isEmpty()) {
            val frame = readFrame(input)
            rxBuf = recvCS.decrypt(null, frame)
        }
        val n = minOf(rxBuf.size, len)
        System.arraycopy(rxBuf, 0, dst, off, n)
        rxBuf = rxBuf.copyOfRange(n, rxBuf.size)
        return n
    }

    fun close() = closer()
}

/**
 * Drives the Noise XX handshake on (input, output) and returns the
 * transport [Session] on success. The initiator writes [MAGIC] before the
 * first handshake message; the responder must already have consumed it
 * via [peekMagic]. Use [Session.remoteStatic] to verify the peer's
 * pubkey against a pinned identity before trusting the session.
 *
 * @param psk optional 32-byte pre-shared key — when non-null, runs Noise
 *            XXpsk2 instead of plain XX. Used for first-pairing only.
 */
@Throws(IOException::class)
fun handshake(
    input: InputStream,
    output: OutputStream,
    role: HandshakeRole,
    staticPriv: ByteArray,
    staticPub: ByteArray,
    psk: ByteArray? = null,
    closer: () -> Unit = {},
): Session {
    require(staticPriv.size == DH_LEN && staticPub.size == DH_LEN) { "static key length must be $DH_LEN" }
    if (psk != null) require(psk.size == KEY_LEN) { "psk length must be $KEY_LEN (or null)" }

    val hs = HandshakeState.initialize(role, staticPriv, staticPub, psk, MAGIC)

    if (role == HandshakeRole.Initiator) {
        output.write(MAGIC)
        output.flush()
    }

    // XX has 3 messages: → e ; ← e,ee,s,es ; → s,se. Initiator drives 0,2;
    // responder drives 1.
    val initiatorSteps = arrayOf(true, false, true)
    val initiator = role == HandshakeRole.Initiator
    var sendCS: CipherState? = null
    var recvCS: CipherState? = null
    for ((i, isInitiatorStep) in initiatorSteps.withIndex()) {
        val writing = (initiator && isInitiatorStep) || (!initiator && !isInitiatorStep)
        if (writing) {
            val msg = hs.writeMessage(ByteArray(0))
            writeFrame(output, msg)
            if (hs.completed) {
                val (cs0, cs1) = hs.split()
                if (initiator) { sendCS = cs0; recvCS = cs1 } else { sendCS = cs1; recvCS = cs0 }
            }
        } else {
            val msg = readFrame(input)
            hs.readMessage(msg)
            if (hs.completed) {
                val (cs0, cs1) = hs.split()
                if (initiator) { sendCS = cs0; recvCS = cs1 } else { sendCS = cs1; recvCS = cs0 }
            }
        }
        // Suppress unused-i warning while keeping the loop variable for readability.
        @Suppress("UNUSED_EXPRESSION") i
    }
    val send = sendCS ?: throw IOException("noise: handshake produced no transport keys")
    val recv = recvCS ?: throw IOException("noise: handshake produced no transport keys")
    val remote = hs.remoteStatic ?: throw IOException("noise: peer static key not received")
    return Session(input, output, send, recv, remote, closer)
}

// ─────────────────────────────────────────────────────────────────────────
// Noise primitives — internal but unit-testable (package-private).
// ─────────────────────────────────────────────────────────────────────────

/** Per-direction cipher state: 32-byte key + 64-bit nonce starting at 0. */
internal class CipherState(internal var key: ByteArray, internal var nonce: Long = 0) {
    @Throws(IOException::class)
    fun encrypt(ad: ByteArray?, plaintext: ByteArray): ByteArray {
        val iv = nonceBytes(nonce)
        nonce++
        return chachaEncrypt(key, iv, ad ?: ByteArray(0), plaintext)
    }

    @Throws(IOException::class)
    fun decrypt(ad: ByteArray?, ciphertext: ByteArray): ByteArray {
        val iv = nonceBytes(nonce)
        nonce++
        return chachaDecrypt(key, iv, ad ?: ByteArray(0), ciphertext)
    }

    /** Build the 12-byte ChaCha20-Poly1305 nonce: 4 bytes zero || 8-byte LE n. */
    private fun nonceBytes(n: Long): ByteArray {
        val buf = ByteBuffer.allocate(12).order(ByteOrder.LITTLE_ENDIAN)
        buf.putInt(0)
        buf.putLong(n)
        return buf.array()
    }
}

internal class SymmetricState(
    var ck: ByteArray,
    var h: ByteArray,
    var cipher: CipherState? = null,
) {
    fun mixHash(data: ByteArray) {
        h = sha256(h + data)
    }

    fun mixKey(input: ByteArray) {
        val outs = hkdf(ck, input, 2)
        ck = outs[0]
        cipher = CipherState(outs[1].copyOfRange(0, KEY_LEN))
    }

    fun mixKeyAndHash(input: ByteArray) {
        val outs = hkdf(ck, input, 3)
        ck = outs[0]
        mixHash(outs[1])
        cipher = CipherState(outs[2].copyOfRange(0, KEY_LEN))
    }

    fun encryptAndHash(plaintext: ByteArray): ByteArray {
        val cs = cipher
        val out = if (cs != null) cs.encrypt(h, plaintext) else plaintext
        mixHash(out)
        return out
    }

    fun decryptAndHash(ciphertext: ByteArray): ByteArray {
        val cs = cipher
        val pt = if (cs != null) cs.decrypt(h, ciphertext) else ciphertext
        mixHash(ciphertext)
        return pt
    }

    fun split(): Pair<CipherState, CipherState> {
        val outs = hkdf(ck, ByteArray(0), 2)
        return CipherState(outs[0].copyOfRange(0, KEY_LEN)) to
            CipherState(outs[1].copyOfRange(0, KEY_LEN))
    }
}

internal class HandshakeState private constructor(
    val role: HandshakeRole,
    val s_priv: ByteArray,
    val s_pub: ByteArray,
    val psk: ByteArray?,
    val sym: SymmetricState,
) {
    private var e_priv: ByteArray? = null
    private var e_pub: ByteArray? = null
    private var re: ByteArray? = null
    var remoteStatic: ByteArray? = null
        private set

    private var msgIdx = 0
    val completed: Boolean get() = msgIdx >= 3

    /** XX message tokens by message index, identical for both sides. */
    private val patterns: List<List<String>> = listOf(
        listOf("e"),
        listOf("e", "ee", "s", "es"),
        // XX's third message is `s, se`. With XXpsk2 we prepend a `psk`
        // token so the PSK is mixed before the static-key encryption.
        if (psk == null) listOf("s", "se") else listOf("psk", "s", "se"),
    )

    fun writeMessage(payload: ByteArray): ByteArray {
        val tokens = patterns[msgIdx]
        val out = java.io.ByteArrayOutputStream()
        for (token in tokens) {
            when (token) {
                "e" -> {
                    val (priv, pub) = generateX25519()
                    e_priv = priv; e_pub = pub
                    out.write(pub)
                    sym.mixHash(pub)
                    if (psk != null) sym.mixKey(pub)
                }
                "s" -> {
                    val ct = sym.encryptAndHash(s_pub)
                    out.write(ct)
                }
                "ee" -> sym.mixKey(dh(e_priv!!, re!!))
                "es" -> sym.mixKey(if (role == HandshakeRole.Initiator) dh(e_priv!!, remoteStatic!!) else dh(s_priv, re!!))
                "se" -> sym.mixKey(if (role == HandshakeRole.Initiator) dh(s_priv, re!!) else dh(e_priv!!, remoteStatic!!))
                "psk" -> sym.mixKeyAndHash(psk!!)
                else -> error("noise: unknown token $token")
            }
        }
        out.write(sym.encryptAndHash(payload))
        msgIdx++
        return out.toByteArray()
    }

    fun readMessage(message: ByteArray): ByteArray {
        val tokens = patterns[msgIdx]
        var off = 0
        for (token in tokens) {
            when (token) {
                "e" -> {
                    val pub = message.copyOfRange(off, off + DH_LEN)
                    re = pub; off += DH_LEN
                    sym.mixHash(pub)
                    if (psk != null) sym.mixKey(pub)
                }
                "s" -> {
                    val len = if (sym.cipher != null) DH_LEN + TAG_LEN else DH_LEN
                    val raw = message.copyOfRange(off, off + len)
                    val pt = sym.decryptAndHash(raw)
                    remoteStatic = pt
                    off += len
                }
                "ee" -> sym.mixKey(dh(e_priv!!, re!!))
                "es" -> sym.mixKey(if (role == HandshakeRole.Initiator) dh(e_priv!!, remoteStatic!!) else dh(s_priv, re!!))
                "se" -> sym.mixKey(if (role == HandshakeRole.Initiator) dh(s_priv, re!!) else dh(e_priv!!, remoteStatic!!))
                "psk" -> sym.mixKeyAndHash(psk!!)
                else -> error("noise: unknown token $token")
            }
        }
        val payload = sym.decryptAndHash(message.copyOfRange(off, message.size))
        msgIdx++
        return payload
    }

    fun split() = sym.split()

    companion object {
        fun initialize(
            role: HandshakeRole,
            staticPriv: ByteArray,
            staticPub: ByteArray,
            psk: ByteArray?,
            prologue: ByteArray,
        ): HandshakeState {
            // Protocol name must match the Go side bytewise — this is the
            // fundamental cross-stack contract.
            val protocolName = if (psk == null)
                "Noise_XX_25519_ChaChaPoly_SHA256"
            else
                "Noise_XXpsk2_25519_ChaChaPoly_SHA256"
            val nameBytes = protocolName.toByteArray(Charsets.US_ASCII)
            val h0 = if (nameBytes.size <= 32)
                nameBytes + ByteArray(32 - nameBytes.size)
            else
                sha256(nameBytes)
            val ck0 = h0.copyOf()
            val sym = SymmetricState(ck0, h0)
            sym.mixHash(prologue)
            return HandshakeState(role, staticPriv, staticPub, psk, sym)
        }
    }
}

// ─────────────────────────────────────────────────────────────────────────
// Crypto primitives — wrappers around BouncyCastle / java.security.
// ─────────────────────────────────────────────────────────────────────────

internal fun sha256(data: ByteArray): ByteArray =
    MessageDigest.getInstance("SHA-256").digest(data)

internal fun hmacSha256(key: ByteArray, data: ByteArray): ByteArray {
    val mac = Mac.getInstance("HmacSHA256")
    mac.init(SecretKeySpec(key, "HmacSHA256"))
    return mac.doFinal(data)
}

/**
 * RFC 5869 HKDF specialised for the Noise spec: returns [n] 32-byte
 * outputs. Uses [salt] as the HKDF salt and [ikm] as the key material.
 */
internal fun hkdf(salt: ByteArray, ikm: ByteArray, n: Int): List<ByteArray> {
    require(n in 1..3) { "hkdf: noise requires 1..3 outputs" }
    val tempKey = hmacSha256(salt, ikm) // extract
    val outs = mutableListOf<ByteArray>()
    var prev = ByteArray(0)
    for (i in 1..n) {
        prev = hmacSha256(tempKey, prev + byteArrayOf(i.toByte()))
        outs += prev
    }
    return outs
}

internal fun generateX25519(): Pair<ByteArray, ByteArray> {
    val priv = ByteArray(32).also { SecureRandom().nextBytes(it) }
    // Standard X25519 clamping per RFC 7748 — BouncyCastle's X25519
    // applies it internally too, but doing it on the stored scalar keeps
    // the public side derived from a canonical scalar.
    priv[0] = (priv[0].toInt() and 248).toByte()
    priv[31] = ((priv[31].toInt() and 127) or 64).toByte()
    val pub = ByteArray(32)
    X25519.scalarMultBase(priv, 0, pub, 0)
    return priv to pub
}

internal fun dh(priv: ByteArray, peerPub: ByteArray): ByteArray {
    val shared = ByteArray(32)
    X25519.scalarMult(priv, 0, peerPub, 0, shared, 0)
    return shared
}

private fun chachaEncrypt(key: ByteArray, iv: ByteArray, ad: ByteArray, plaintext: ByteArray): ByteArray {
    val cipher = Cipher.getInstance("ChaCha20-Poly1305")
    val keySpec = SecretKeySpec(key, "ChaCha20")
    cipher.init(
        Cipher.ENCRYPT_MODE,
        keySpec,
        javax.crypto.spec.IvParameterSpec(iv),
    )
    cipher.updateAAD(ad)
    return cipher.doFinal(plaintext)
}

private fun chachaDecrypt(key: ByteArray, iv: ByteArray, ad: ByteArray, ciphertext: ByteArray): ByteArray {
    val cipher = Cipher.getInstance("ChaCha20-Poly1305")
    val keySpec = SecretKeySpec(key, "ChaCha20")
    cipher.init(
        Cipher.DECRYPT_MODE,
        keySpec,
        javax.crypto.spec.IvParameterSpec(iv),
    )
    cipher.updateAAD(ad)
    return cipher.doFinal(ciphertext)
}

// ─────────────────────────────────────────────────────────────────────────
// Frame helpers — 16-bit LE length prefix.
// ─────────────────────────────────────────────────────────────────────────

@Throws(IOException::class)
private fun writeFrame(out: OutputStream, data: ByteArray) {
    require(data.size <= MAX_MSG_LEN) { "tunnel: frame ${data.size} > $MAX_MSG_LEN" }
    val hdr = ByteBuffer.allocate(2).order(ByteOrder.LITTLE_ENDIAN).putShort(data.size.toShort()).array()
    out.write(hdr)
    out.write(data)
    out.flush()
}

@Throws(IOException::class)
private fun readFrame(input: InputStream): ByteArray {
    val hdr = readFully(input, 2)
    val n = ByteBuffer.wrap(hdr).order(ByteOrder.LITTLE_ENDIAN).short.toInt() and 0xFFFF
    return readFully(input, n)
}

@Throws(IOException::class)
private fun readFully(input: InputStream, n: Int): ByteArray {
    val out = ByteArray(n)
    var read = 0
    while (read < n) {
        val r = input.read(out, read, n - read)
        if (r < 0) throw IOException("tunnel: short read at $read/$n")
        read += r
    }
    return out
}
