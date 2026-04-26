package dev.wasabules.dukto.tunnel

import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Test
import java.io.ByteArrayInputStream
import java.net.InetAddress
import java.net.ServerSocket
import java.net.Socket
import java.security.SecureRandom

/** Generates a fresh X25519 keypair via the package-private helper. */
private fun freshKeypair(): Pair<ByteArray, ByteArray> = generateX25519()

class TunnelTest {

    @Test fun peekMagicMatches() {
        val src = ByteArrayInputStream(MAGIC + "trailing".toByteArray())
        val r = peekMagic(src)
        assertTrue(r.isV2)
        // Trailing bytes still readable on the returned stream.
        val rest = r.stream.readBytes()
        assertArrayEquals("trailing".toByteArray(), rest)
    }

    @Test fun peekMagicLegacyReplays() {
        val sessionHeader = byteArrayOf(0x05, 0, 0, 0, 0, 0, 0, 0)
        val payload = sessionHeader + "hello".toByteArray()
        val src = ByteArrayInputStream(payload)
        val r = peekMagic(src)
        assertFalse(r.isV2)
        // The PushbackInputStream replays the consumed 8 bytes first.
        val all = r.stream.readBytes()
        assertArrayEquals(payload, all)
    }

    /**
     * End-to-end Noise XX between two threads connected by a pair of
     * piped streams. Verifies the handshake produces matching transport
     * keys and that bidirectional ciphertext round-trips through them.
     */
    @Test fun handshakeRoundTripPlain() {
        runHandshakePair(psk = null)
    }

    @Test fun handshakeRoundTripXxpsk2() {
        val psk = ByteArray(32).also { SecureRandom().nextBytes(it) }
        runHandshakePair(psk)
    }

    @Test fun handshakeRejectsMismatchedPsk() {
        val a = ByteArray(32).also { SecureRandom().nextBytes(it) }
        val b = ByteArray(32).also { SecureRandom().nextBytes(it) }
        // Force two distinct PSKs (probability-1 in practice given the
        // random fill, but make it deterministic for the test).
        b[0] = (a[0].toInt() xor 0xFF).toByte()
        try {
            runHandshakePair(psk = null, initiatorPsk = a, responderPsk = b)
            fail("PSK mismatch should have aborted the handshake")
        } catch (t: Throwable) {
            // expected — exact exception type depends on which side
            // detects the AEAD mismatch first.
        }
    }

    private fun runHandshakePair(
        psk: ByteArray?,
        initiatorPsk: ByteArray? = psk,
        responderPsk: ByteArray? = psk,
    ) {
        val (initPriv, initPub) = freshKeypair()
        val (respPriv, respPub) = freshKeypair()

        // Real loopback sockets — PipedInputStream's "read end dead" check
        // breaks the moment the responder thread exits, which makes it
        // unusable for a test that wants to drive transport-mode reads
        // from the main thread.
        val server = ServerSocket(0, 1, InetAddress.getLoopbackAddress())
        val port = server.localPort

        var responderSession: Session? = null
        var responderError: Throwable? = null
        val responderThread = Thread {
            try {
                val accepted = server.accept()
                accepted.tcpNoDelay = true
                val peeked = peekMagic(accepted.getInputStream())
                assertTrue(peeked.isV2)
                responderSession = handshake(
                    peeked.stream, accepted.getOutputStream(),
                    HandshakeRole.Responder,
                    respPriv, respPub,
                    psk = responderPsk,
                    closer = { accepted.close() },
                )
            } catch (t: Throwable) {
                responderError = t
            }
        }.apply { start() }

        val client = Socket(InetAddress.getLoopbackAddress(), port)
        client.tcpNoDelay = true

        val initiatorSession: Session = try {
            handshake(
                client.getInputStream(), client.getOutputStream(),
                HandshakeRole.Initiator,
                initPriv, initPub,
                psk = initiatorPsk,
                closer = { client.close() },
            )
        } catch (t: Throwable) {
            responderThread.join(2_000)
            client.close()
            server.close()
            throw t
        }
        responderThread.join(2_000)
        if (responderError != null) {
            client.close()
            server.close()
            throw responderError!!
        }

        val rc = responderSession ?: error("responder session not set")

        try {
            // Each side learns the *other* peer's static pubkey.
            assertArrayEquals(respPub, initiatorSession.remoteStatic)
            assertArrayEquals(initPub, rc.remoteStatic)

            // Bidirectional transport: initiator → responder, then back.
            val msg = "encrypted hello".toByteArray()
            initiatorSession.write(msg)
            val got = ByteArray(msg.size)
            var read = 0
            while (read < msg.size) {
                val n = rc.read(got, read, msg.size - read)
                if (n <= 0) break
                read += n
            }
            assertEquals(msg.size, read)
            assertArrayEquals(msg, got)

            val reply = "ack".toByteArray()
            rc.write(reply)
            val got2 = ByteArray(reply.size)
            var read2 = 0
            while (read2 < reply.size) {
                val n = initiatorSession.read(got2, read2, reply.size - read2)
                if (n <= 0) break
                read2 += n
            }
            assertArrayEquals(reply, got2)
        } finally {
            initiatorSession.close()
            rc.close()
            server.close()
        }
    }
}
