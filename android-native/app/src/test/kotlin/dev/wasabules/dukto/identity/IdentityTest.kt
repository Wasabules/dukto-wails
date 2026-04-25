package dev.wasabules.dukto.identity

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
