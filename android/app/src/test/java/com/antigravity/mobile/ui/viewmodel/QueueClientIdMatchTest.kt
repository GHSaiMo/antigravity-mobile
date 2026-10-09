package com.antigravity.mobile.ui.viewmodel

import com.antigravity.mobile.data.model.QueuedMessageItem
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class QueueClientIdMatchTest {
    private fun server(id: String, text: String, cid: String? = null) = QueuedMessageItem(id = id, text = text, clientMessageId = cid)
    private fun local(text: String, cid: String?) = QueuedMessageItem(id = "queue-$cid", text = text, clientMessageId = cid)

    @Test
    fun matchesByClientIdEvenWhenTextIsIdentical() {
        val queue = listOf(server("s1", "继续", "aaa"), server("s2", "继续", "bbb"))
        assertEquals("s2", findServerQueueItem(queue, local("继续", "bbb"), "继续")?.id)
        assertEquals("s1", findServerQueueItem(queue, local("继续", "aaa"), "继续")?.id)
    }

    @Test
    fun doesNotFallBackToTextWhenGatewayTagsButItemNotYetThere() {
        // 队列里已有带 id 的条目（网关支持标注），但自己这一条还没到：不能按文字误删另一条同文字消息
        val queue = listOf(server("s1", "继续", "aaa"))
        assertNull(findServerQueueItem(queue, local("继续", "ccc"), "继续"))
    }

    @Test
    fun fallsBackToTextForOldGatewayOrForeignMessages() {
        val queue = listOf(server("s1", "来自桌面端的排队消息"))
        assertEquals("s1", findServerQueueItem(queue, local("来自桌面端的排队消息", "ddd"), "来自桌面端的排队消息")?.id)
        assertEquals("s1", findServerQueueItem(queue, QueuedMessageItem(id = "queue-x", text = "来自桌面端的排队消息"), "来自桌面端的排队消息")?.id)
    }

    @Test
    fun emptyQueueReturnsNull() {
        assertNull(findServerQueueItem(null, local("x", "a"), "x"))
        assertNull(findServerQueueItem(emptyList(), local("x", "a"), "x"))
    }
}
