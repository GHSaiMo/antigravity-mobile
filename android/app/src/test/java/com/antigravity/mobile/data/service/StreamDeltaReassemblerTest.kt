package com.antigravity.mobile.data.service

import com.antigravity.mobile.data.model.GatewayMessageItem
import com.antigravity.mobile.data.model.StreamUpdatePayload
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Test

class StreamDeltaReassemblerTest {
    private fun m(id: String, text: String) = GatewayMessageItem(id = id, text = text)

    private fun init(vararg msgs: GatewayMessageItem) =
        StreamUpdatePayload(type = "init", messages = msgs.toList())

    private fun delta(ids: List<String>, vararg changed: GatewayMessageItem) =
        StreamUpdatePayload(type = "update", delta = true, messageIds = ids, messages = changed.toList())

    @Test
    fun fullFramesPassThroughUnchanged() {
        val r = StreamDeltaReassembler()
        val p = init(m("a", "1"))
        assertEquals(p, r.apply(p))
    }

    @Test
    fun rebuildsWindowFromTailChange() {
        val r = StreamDeltaReassembler()
        r.apply(init(m("a", "1"), m("b", "2")))
        val out = r.apply(delta(listOf("a", "b"), m("b", "2 more")))!!
        assertEquals(listOf("a", "b"), out.messages!!.map { it.id })
        assertEquals("2 more", out.messages!![1].text)
        assertFalse(out.delta)
        assertNull(out.messageIds)
    }

    @Test
    fun slidingWindowDropsOldAndAddsNew() {
        val r = StreamDeltaReassembler()
        r.apply(init(m("a", "1"), m("b", "2")))
        val out = r.apply(delta(listOf("b", "c"), m("c", "3")))!!
        assertEquals(listOf("b", "c"), out.messages!!.map { it.id })
        // The rebuilt window becomes the new baseline for the next delta.
        val next = r.apply(delta(listOf("b", "c"), m("c", "3!")))!!
        assertEquals("3!", next.messages!![1].text)
        assertEquals("2", next.messages!![0].text)
    }

    @Test
    fun deltaAfterEmptyInitWorksAgainstEmptyBaseline() {
        // Server's placeholder init (no messages) is followed by a delta carrying everything.
        val r = StreamDeltaReassembler()
        r.apply(StreamUpdatePayload(type = "init", messages = null))
        val out = r.apply(delta(listOf("a"), m("a", "1")))
        assertNotNull(out)
        assertEquals(listOf("a"), out!!.messages!!.map { it.id })
    }

    @Test
    fun unknownIdSignalsResync() {
        val r = StreamDeltaReassembler()
        r.apply(init(m("a", "1")))
        assertNull(r.apply(delta(listOf("a", "zzz"))))
    }

    @Test
    fun resetClearsBaseline() {
        val r = StreamDeltaReassembler()
        r.apply(init(m("a", "1")))
        r.reset()
        assertNull(r.apply(delta(listOf("a"))))
    }
}
