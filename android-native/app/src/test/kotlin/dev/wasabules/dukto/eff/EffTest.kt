package dev.wasabules.dukto.eff

import org.junit.Assert.assertEquals
import org.junit.Test

class EffTest {

    @Test fun canonicaliseHandlesPunctuationAndCase() {
        val cases = mapOf(
            "Apple-Tiger River_OCEAN, music" to "apple-tiger-river-ocean-music",
            "  apple  tiger  river  " to "apple-tiger-river",
            "apple--tiger___river" to "apple-tiger-river",
        )
        for ((input, expected) in cases) {
            assertEquals(expected, Eff.canonicalise(input))
        }
    }

    /**
     * Cross-stack lock-in: the Wails-side TestDerivePSKMatchesKnownVector
     * pins the same 32-byte output for the same input. If either side
     * drifts (canonicaliser, HKDF wiring, salt convention) the hexes go
     * out of sync immediately.
     */
    @Test fun derivePSKMatchesKnownVector() {
        val psk = Eff.derivePSK("Apple-Tiger-River-Ocean-Music")
        assertEquals(32, psk.size)
        val expected = "1d5dc6f079bb2b26de421d51dfb3524a83519a819f7060cb0e39dfbc619b91dc"
        assertEquals(expected, hexEncode(psk))

        // Whitespace / case / punctuation drift on the same passphrase
        // must yield the identical PSK.
        val psk2 = Eff.derivePSK(" apple tiger  river_ocean,Music ")
        assertEquals(hexEncode(psk), hexEncode(psk2))

        // A different passphrase must yield a different PSK.
        val other = Eff.derivePSK("apple-tiger-river-ocean-musics")
        assert(hexEncode(other) != hexEncode(psk))
    }

    private fun hexEncode(b: ByteArray): String =
        b.joinToString("") { "%02x".format(it) }
}
