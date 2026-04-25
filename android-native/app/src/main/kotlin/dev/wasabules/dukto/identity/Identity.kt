package dev.wasabules.dukto.identity

import android.content.Context
import androidx.security.crypto.EncryptedFile
import androidx.security.crypto.MasterKey
import net.i2p.crypto.eddsa.EdDSAPrivateKey
import net.i2p.crypto.eddsa.EdDSAPublicKey
import net.i2p.crypto.eddsa.spec.EdDSANamedCurveTable
import net.i2p.crypto.eddsa.spec.EdDSAPrivateKeySpec
import net.i2p.crypto.eddsa.spec.EdDSAPublicKeySpec
import java.io.File
import java.security.MessageDigest
import java.security.SecureRandom

/**
 * This install's long-term Ed25519 keypair.
 *
 * The keypair is generated lazily on first call to [loadOrGenerate] and
 * persisted under `filesDir/identity.key`, wrapped by AndroidX
 * `EncryptedFile` (AES-256-GCM, master key in the AndroidKeyStore — hardware-
 * backed where the device offers a TEE / StrongBox).
 *
 * Used for the v2 encrypted overlay (see docs/SECURITY_v2.md):
 *  - M1 (this milestone): the keypair exists and the fingerprint is shown in
 *    Settings. Not yet on the wire.
 *  - M2: the public key signs UDP discovery datagrams (0x06/0x07).
 *  - M3: the static keypair authenticates Noise XX TCP handshakes.
 */
data class Identity(
    /** 32-byte Ed25519 raw public key. Safe to share. */
    val publicKey: ByteArray,
    /** Underlying EdDSA private key handle — used by future sign() calls.
     *  Treat as opaque — never log, never serialise. */
    internal val privateKey: EdDSAPrivateKey,
) {
    /** Canonical 16-character fingerprint (XXXX-XXXX-XXXX-XXXX). */
    val fingerprint: String get() = fingerprintOf(publicKey)

    // Override equals/hashCode because the auto-generated ones for data
    // classes don't compare ByteArray contents structurally.
    override fun equals(other: Any?): Boolean =
        other is Identity && publicKey.contentEquals(other.publicKey)
    override fun hashCode(): Int = publicKey.contentHashCode()
}

/**
 * Compute the user-visible fingerprint of an Ed25519 public key:
 * RFC4648 base32 (no padding) of the first 10 bytes of SHA-256(pub),
 * grouped as 4-4-4-4 with dashes.
 */
fun fingerprintOf(publicKey: ByteArray): String {
    val digest = MessageDigest.getInstance("SHA-256").digest(publicKey)
    val truncated = digest.copyOf(10)
    val encoded = base32NoPad(truncated)
    return buildString {
        for ((i, c) in encoded.withIndex()) {
            if (i > 0 && i % 4 == 0) append('-')
            append(c)
        }
    }
}

/**
 * Load the Ed25519 keypair from [path], or generate and persist a fresh one
 * on first call.
 *
 * Throws if a non-empty file is unreadable / corrupted: a previous
 * fingerprint may be pinned by paired peers, so we never silently regenerate
 * over a bad file. Caller is expected to surface the error.
 */
fun loadOrGenerate(context: Context, path: File): Identity {
    val masterKey = MasterKey.Builder(context)
        .setKeyScheme(MasterKey.KeyScheme.AES256_GCM)
        .build()
    val encryptedFile = EncryptedFile.Builder(
        context,
        path,
        masterKey,
        EncryptedFile.FileEncryptionScheme.AES256_GCM_HKDF_4KB,
    ).build()

    if (path.exists() && path.length() > 0) {
        return decode(encryptedFile.openFileInput().use { it.readBytes() })
    }
    return generateAndPersist(encryptedFile, path)
}

private fun generateAndPersist(file: EncryptedFile, path: File): Identity {
    val seed = ByteArray(32).also { SecureRandom().nextBytes(it) }
    val spec = EdDSANamedCurveTable.getByName(EdDSANamedCurveTable.ED_25519)
    val privSpec = EdDSAPrivateKeySpec(seed, spec)
    val priv = EdDSAPrivateKey(privSpec)
    val pubSpec = EdDSAPublicKeySpec(privSpec.a, spec)
    val pub = EdDSAPublicKey(pubSpec)
    val publicKey = pub.abyte

    // Atomic-ish write: we let EncryptedFile manage the underlying file
    // descriptor; if it fails halfway the file is left in a state that's
    // either decode-success (good) or decode-failure (caller throws on
    // next launch and we surface "move it aside if you really mean to
    // regenerate" — same shape as the Go side).
    if (path.exists()) path.delete() // EncryptedFile refuses to write over an existing file
    file.openFileOutput().use { it.write(encodePayload(seed, publicKey)) }
    return Identity(publicKey, priv)
}

/**
 * On-disk format: 1 byte version (=1), 32 bytes seed, 32 bytes pubkey.
 * EncryptedFile handles confidentiality + integrity via AES-256-GCM, so the
 * payload itself is plain — keeping it tiny and version-tagged for forward-
 * compatibility (e.g. future M2 might add an X25519 derived key alongside).
 */
private const val VERSION_TAG: Byte = 1

private fun encodePayload(seed: ByteArray, pub: ByteArray): ByteArray {
    require(seed.size == 32 && pub.size == 32) { "Ed25519 seed/pub must be 32 bytes" }
    return ByteArray(1 + 32 + 32).also {
        it[0] = VERSION_TAG
        seed.copyInto(it, 1)
        pub.copyInto(it, 33)
    }
}

private fun decode(data: ByteArray): Identity {
    require(data.size == 1 + 32 + 32) {
        "identity file payload size ${data.size}, expected 65"
    }
    require(data[0] == VERSION_TAG) {
        "identity file version ${data[0]}, expected $VERSION_TAG"
    }
    val seed = data.copyOfRange(1, 33)
    val pub = data.copyOfRange(33, 65)
    val spec = EdDSANamedCurveTable.getByName(EdDSANamedCurveTable.ED_25519)
    val privSpec = EdDSAPrivateKeySpec(seed, spec)
    return Identity(pub, EdDSAPrivateKey(privSpec))
}

/**
 * RFC4648 base32 encoding without padding (so the formatted fingerprint
 * doesn't have stray '=' characters).
 */
private fun base32NoPad(input: ByteArray): String {
    val alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"
    val sb = StringBuilder()
    var buffer = 0
    var bitsLeft = 0
    for (b in input) {
        buffer = (buffer shl 8) or (b.toInt() and 0xFF)
        bitsLeft += 8
        while (bitsLeft >= 5) {
            val idx = (buffer shr (bitsLeft - 5)) and 0x1F
            sb.append(alphabet[idx])
            bitsLeft -= 5
        }
    }
    if (bitsLeft > 0) {
        val idx = (buffer shl (5 - bitsLeft)) and 0x1F
        sb.append(alphabet[idx])
    }
    return sb.toString()
}
