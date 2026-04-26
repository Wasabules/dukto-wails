package dev.wasabules.dukto.eff

import android.content.Context
import java.security.SecureRandom
import javax.crypto.Mac
import javax.crypto.spec.SecretKeySpec

/**
 * EFF Short Wordlist 1 (1296 words, ~10.34 bits/word) bundled as an
 * Android asset. Mirrors the Wails-side `internal/eff` package so peers
 * derive the same PSK from the same passphrase.
 *
 * Wordlist source: https://www.eff.org/files/2016/09/08/eff_short_wordlist_1.txt
 */
object Eff {

    /** Fixed HKDF info string. Bumping this invalidates every pairing. */
    const val PSK_INFO = "DUKTO-PSK-v1"

    /** Single character separating words in the canonical form. */
    const val JOIN_SEPARATOR = "-"

    /** Lazy-loaded wordlist. Subsequent calls read from the cached array. */
    @Volatile private var cached: List<String>? = null

    fun words(context: Context): List<String> {
        cached?.let { return it }
        val list = synchronized(this) {
            cached ?: context.assets.open("eff_short_wordlist.txt").bufferedReader().use { r ->
                r.readLines().map { it.trim() }.filter { it.isNotEmpty() }
            }.also { cached = it }
        }
        check(list.size == 1296) { "EFF wordlist size ${list.size}, expected 1296" }
        return list
    }

    /**
     * Generates [n] random EFF short words joined by [JOIN_SEPARATOR].
     * Default [n] is 5 (~51.7 bits of entropy). [n] must be in [3, 8].
     */
    fun generate(context: Context, n: Int = 5): String {
        require(n in 3..8) { "word count $n out of range [3,8]" }
        val list = words(context)
        val rng = SecureRandom()
        val picks = (0 until n).map {
            // Rejection-sample to avoid modulo bias on the random Int.
            var idx: Int
            do { idx = rng.nextInt() and 0xFFFF } while (idx >= 65536 - (65536 % list.size))
            list[idx % list.size]
        }
        return picks.joinToString(JOIN_SEPARATOR)
    }

    /**
     * Normalises a user-entered passphrase: lowercases, splits on
     * whitespace / dashes / underscores / commas, drops empties,
     * rejoins with [JOIN_SEPARATOR]. Both stacks must canonicalise
     * before [derivePSK] so capitalisation and punctuation drift
     * doesn't break the handshake.
     */
    fun canonicalise(s: String): String {
        return s.lowercase()
            .split(' ', '\t', '-', '_', ',')
            .filter { it.isNotEmpty() }
            .joinToString(JOIN_SEPARATOR)
    }

    /**
     * HKDF-SHA256 with the canonical passphrase as IKM and the fixed
     * [PSK_INFO] string as info. Returns 32 bytes suitable for Noise
     * XXpsk2. Mirrors the formula on the Wails side.
     */
    fun derivePSK(passphrase: String): ByteArray {
        val canon = canonicalise(passphrase)
        require(canon.isNotEmpty()) { "empty passphrase" }
        // HKDF-Extract: PRK = HMAC-SHA256(salt=zeros, IKM=canon)
        val zeroSalt = ByteArray(32)
        val prk = hmacSha256(zeroSalt, canon.toByteArray(Charsets.UTF_8))
        // HKDF-Expand for one block (32 bytes): T(1) = HMAC(PRK, info || 0x01)
        val info = PSK_INFO.toByteArray(Charsets.UTF_8)
        return hmacSha256(prk, info + byteArrayOf(0x01))
    }

    private fun hmacSha256(key: ByteArray, data: ByteArray): ByteArray {
        val mac = Mac.getInstance("HmacSHA256")
        mac.init(SecretKeySpec(key, "HmacSHA256"))
        return mac.doFinal(data)
    }
}
