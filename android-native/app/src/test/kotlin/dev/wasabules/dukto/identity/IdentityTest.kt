package dev.wasabules.dukto.identity

import net.i2p.crypto.eddsa.EdDSAPrivateKey
import net.i2p.crypto.eddsa.EdDSAPublicKey
import net.i2p.crypto.eddsa.spec.EdDSANamedCurveTable
import net.i2p.crypto.eddsa.spec.EdDSAPrivateKeySpec
import net.i2p.crypto.eddsa.spec.EdDSAPublicKeySpec
import org.bouncycastle.math.ec.rfc7748.X25519
import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.security.MessageDigest

class IdentityTest {

    @Test fun fingerprintIsDeterministicAndFormatted() {
        val pub = ByteArray(32) { it.toByte() }
        val a = fingerprintOf(pub)
        val b = fingerprintOf(pub)
        assertEquals(a, b)
        // 16 base32 chars + 3 dashes
        assertEquals(19, a.length)
        // Dash positions: 4, 9, 14
        assertEquals('-', a[4])
        assertEquals('-', a[9])
        assertEquals('-', a[14])
        assertTrue("base32 alphabet only", a.replace("-", "").all { it in 'A'..'Z' || it in '2'..'7' })
    }

    /**
     * Mirrors the Go-side TestX25519DerivationMatchesEd25519PubConversion:
     * the X25519 public key derived directly from the seed must equal the
     * Edwards-to-Montgomery projection of the Ed25519 public key. Without
     * this invariant, peers that pin a Ed25519 fingerprint (UDP 0x06/0x07)
     * couldn't validate the Noise XX remote_static.
     */
    @Test fun x25519DerivationMatchesEd25519PubConversion() {
        // Build a deterministic identity in-memory (no Android Context).
        val seed = ByteArray(32) { (it * 7 + 1).toByte() }
        val spec = EdDSANamedCurveTable.getByName(EdDSANamedCurveTable.ED_25519)
        val privSpec = EdDSAPrivateKeySpec(seed, spec)
        val priv = EdDSAPrivateKey(privSpec)
        val pub = EdDSAPublicKey(EdDSAPublicKeySpec(privSpec.a, spec)).abyte
        val id = Identity(pub, priv, seed)

        val fromSeed = id.x25519Public()
        val fromPub = ed25519PubToX25519Pub(pub)
            ?: error("ed25519→x25519 conversion failed for a valid pubkey")
        assertArrayEquals(fromSeed, fromPub)
    }

    @Test fun fingerprintMatchesGoSideAlgorithm() {
        // Ground truth: SHA-256("hello"), first 10 bytes, RFC4648 base32 no
        // padding. The Go side computes the same thing — keep them in lockstep.
        val input = "hello".toByteArray()
        val pub = ByteArray(32).also {
            // Pad to 32 bytes; algorithm only cares about feeding the same
            // input to both sides.
            input.copyInto(it)
        }
        val expectedTruncated = MessageDigest.getInstance("SHA-256").digest(pub).copyOf(10)
        val fp = fingerprintOf(pub)
        // First 4 chars correspond to bits 0..19 of the digest.
        val firstByte = expectedTruncated[0].toInt() and 0xFF
        val firstFiveBits = (firstByte shr 3) and 0x1F
        val expectedFirstChar = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"[firstFiveBits]
        assertEquals(expectedFirstChar, fp[0])
    }
}
