package dev.wasabules.dukto.protocol

import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertThrows
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.ByteArrayInputStream
import java.io.ByteArrayOutputStream

class ProtocolTest {

    @Test fun defaultPortMatchesSpec() = assertEquals(4644, DEFAULT_PORT)

    @Test fun textMagicAndSentinelMatchSpec() {
        assertEquals("___DUKTO___TEXT___", TEXT_ELEMENT_NAME)
        assertEquals(-1L, DIRECTORY_SIZE_MARKER)
    }

    @Test fun signatureFormatIsLoadBearing() {
        assertEquals(
            "alice at host (Linux)",
            buildSignature("alice", "host", PLATFORM_LINUX),
        )
    }

    // ── BuddyMessage ─────────────────────────────────────────────────────────

    @Test fun helloBroadcastRoundTrip() {
        val m = BuddyMessage(MessageType.HelloBroadcast, signature = "alice at host (Android)")
        val data = m.serialize()
        // type byte + utf8 signature, no port
        assertEquals(0x01.toByte(), data[0])
        assertEquals(m, BuddyMessage.parse(data))
    }

    @Test fun helloPortBroadcastIsLittleEndian() {
        val m = BuddyMessage(MessageType.HelloPortBroadcast, port = 0x1234, signature = "x at y (Linux)")
        val data = m.serialize()
        // After the type byte, the next two bytes are the LE port.
        assertEquals(0x34.toByte(), data[1])
        assertEquals(0x12.toByte(), data[2])
        assertEquals(m, BuddyMessage.parse(data))
    }

    @Test fun goodbyeHasNoPortNorSignature() {
        val data = BuddyMessage.goodbye().serialize()
        assertArrayEquals(byteArrayOf(0x03), data)
        assertEquals(BuddyMessage(MessageType.Goodbye), BuddyMessage.parse(data))
    }

    @Test fun parseRejectsEmpty() {
        assertThrows(InvalidMessageException::class.java) { BuddyMessage.parse(ByteArray(0)) }
    }

    @Test fun parseRejectsUnknownType() {
        assertThrows(InvalidMessageException::class.java) {
            // 0x09 is reserved — anything outside the 0x01..0x07 set must error.
            BuddyMessage.parse(byteArrayOf(0x09, 0x10, 0x00, 0x61))
        }
    }

    @Test fun parseRejectsZeroPortInPortType() {
        // 0x04 with port=0 must be rejected — matches Qt BuddyMessage::parse.
        assertThrows(InvalidMessageException::class.java) {
            BuddyMessage.parse(byteArrayOf(0x04, 0x00, 0x00, 0x61))
        }
    }

    @Test fun parseRejectsEmptySignatureForHello() {
        assertThrows(InvalidMessageException::class.java) {
            BuddyMessage.parse(byteArrayOf(0x01))
        }
    }

    @Test fun helloPortKeyRoundTripPreservesFields() {
        val pub = ByteArray(ED25519_PUBLIC_KEY_SIZE) { it.toByte() }
        val sig = ByteArray(ED25519_SIGNATURE_SIZE) { (0x40 + it).toByte() }
        val m = BuddyMessage(
            MessageType.HelloPortKeyBroadcast,
            port = 5000,
            signature = "alice at host (Linux)",
            pubKey = pub,
            sig = sig,
        )
        val data = m.serialize()
        // type + port_le + pub + sig + utf8(signature)
        assertEquals(0x06.toByte(), data[0])
        assertEquals(0x88.toByte(), data[1])
        assertEquals(0x13.toByte(), data[2])
        // The pubkey starts at offset 3.
        assertEquals(pub[0], data[3])
        assertEquals(pub[31], data[3 + 31])
        // The signature follows.
        assertEquals(sig[0], data[3 + 32])
        assertEquals(sig[63], data[3 + 32 + 63])
        // Round-trip preserves everything.
        val parsed = BuddyMessage.parse(data)
        assertEquals(m, parsed)
    }

    @Test fun helloPortKeyRejectsTruncatedKeyOrSig() {
        val pub = ByteArray(ED25519_PUBLIC_KEY_SIZE) { it.toByte() }
        val sig = ByteArray(ED25519_SIGNATURE_SIZE) { (0x40 + it).toByte() }
        val data = BuddyMessage(
            MessageType.HelloPortKeyBroadcast,
            port = 5000,
            signature = "x at y (Linux)",
            pubKey = pub,
            sig = sig,
        ).serialize()
        // Inside the pubkey field (header 1 + 2 = 3, pubkey 32 — cut at 19).
        assertThrows(InvalidMessageException::class.java) {
            BuddyMessage.parse(data.copyOfRange(0, 19))
        }
        // Inside the sig field (header 1+2+32 = 35, sig 64 — cut at 65).
        assertThrows(InvalidMessageException::class.java) {
            BuddyMessage.parse(data.copyOfRange(0, 65))
        }
    }

    @Test fun signedPayloadIsLittleEndianPortPlusSignature() {
        val m = BuddyMessage(
            MessageType.HelloPortKeyBroadcast,
            port = 0xABCD,
            signature = "x",
        )
        val payload = m.signedPayload()
        // 0xABCD LE = 0xCD 0xAB
        assertEquals(0xCD.toByte(), payload[0])
        assertEquals(0xAB.toByte(), payload[1])
        assertEquals('x'.code.toByte(), payload[2])
    }

    @Test fun helloBroadcastTypeFollowsPort() {
        assertEquals(MessageType.HelloBroadcast, helloBroadcastType(DEFAULT_PORT))
        assertEquals(MessageType.HelloPortBroadcast, helloBroadcastType(5000))
        assertEquals(MessageType.HelloUnicast, helloUnicastType(DEFAULT_PORT))
        assertEquals(MessageType.HelloPortUnicast, helloUnicastType(5000))
    }

    // ── Session/Element headers ──────────────────────────────────────────────

    @Test fun sessionHeaderRoundTrip() {
        val h = SessionHeader(totalElements = 7, totalSize = 1234567)
        val out = ByteArrayOutputStream()
        writeSessionHeader(out, h)
        assertEquals(16, out.size())
        val read = readSessionHeader(ByteArrayInputStream(out.toByteArray()))
        assertEquals(h, read)
    }

    @Test fun sessionHeaderRejectsZeroElements() {
        val out = ByteArrayOutputStream()
        writeSessionHeader(out, SessionHeader(totalElements = 0, totalSize = 0))
        assertThrows(InvalidStreamException::class.java) {
            readSessionHeader(ByteArrayInputStream(out.toByteArray()))
        }
    }

    @Test fun elementHeaderRoundTripFile() {
        val h = ElementHeader("foo/bar.txt", 42)
        val out = ByteArrayOutputStream()
        writeElementHeader(out, h)
        // bytes: utf8(name) + NUL + 8 bytes
        assertEquals(h.name.length + 1 + 8, out.size())
        val read = readElementHeader(ByteArrayInputStream(out.toByteArray()))
        assertEquals(h, read)
        assertTrue(!read.isDirectory && !read.isText)
    }

    @Test fun elementHeaderRoundTripDirectory() {
        val h = ElementHeader("subdir", DIRECTORY_SIZE_MARKER)
        val out = ByteArrayOutputStream()
        writeElementHeader(out, h)
        val read = readElementHeader(ByteArrayInputStream(out.toByteArray()))
        assertEquals(h, read)
        assertTrue(read.isDirectory)
    }

    @Test fun elementHeaderTextMagicIsRecognised() {
        val h = ElementHeader(TEXT_ELEMENT_NAME, 11)
        val out = ByteArrayOutputStream()
        writeElementHeader(out, h)
        val read = readElementHeader(ByteArrayInputStream(out.toByteArray()))
        assertTrue(read.isText)
    }

    @Test fun elementHeaderRejectsEmptyName() {
        // NUL at offset 0 makes the name empty — protocol violation.
        val data = byteArrayOf(0x00, 0, 0, 0, 0, 0, 0, 0, 0)
        assertThrows(InvalidStreamException::class.java) {
            readElementHeader(ByteArrayInputStream(data))
        }
    }

    @Test fun elementHeaderRejectsSizeBelowMinusOne() {
        // -2 little-endian
        val name = "x".toByteArray()
        val out = ByteArrayOutputStream()
        out.write(name)
        out.write(0)
        out.write(byteArrayOf(0xFE.toByte(), 0xFF.toByte(), 0xFF.toByte(), 0xFF.toByte(),
            0xFF.toByte(), 0xFF.toByte(), 0xFF.toByte(), 0xFF.toByte()))
        assertThrows(InvalidStreamException::class.java) {
            readElementHeader(ByteArrayInputStream(out.toByteArray()))
        }
    }
}
