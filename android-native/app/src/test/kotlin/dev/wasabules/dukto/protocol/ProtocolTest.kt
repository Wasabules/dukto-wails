package dev.wasabules.dukto.protocol

import org.junit.Assert.assertEquals
import org.junit.Test

class ProtocolTest {
    @Test
    fun defaultPortMatchesSpec() {
        assertEquals(4644.toUShort(), DEFAULT_PORT)
    }

    @Test
    fun textMagicMatchesSpec() {
        // docs/PROTOCOL.md §3.4 — the magic is what receivers identify a
        // text snippet by. Don't change it without coordinating across all
        // implementations.
        assertEquals("___DUKTO___TEXT___", TEXT_SNIPPET_MAGIC)
    }
}
