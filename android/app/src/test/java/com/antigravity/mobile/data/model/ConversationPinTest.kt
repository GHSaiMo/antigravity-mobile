package com.antigravity.mobile.data.model

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class ConversationPinTest {

    @Test
    fun fromSummary_whenPinnedTrue_isPinnedIsTrue() {
        val summary = TrajectorySummary(
            summary = "Test Conversation",
            annotations = Annotations(title = "Test", pinned = true)
        )
        val item = ConversationItem.fromSummary("conv-1", summary)
        assertTrue(item.isPinned)
    }

    @Test
    fun fromSummary_whenPinnedFalse_isPinnedIsFalse() {
        val summary = TrajectorySummary(
            summary = "Test Conversation",
            annotations = Annotations(title = "Test", pinned = false)
        )
        val item = ConversationItem.fromSummary("conv-2", summary)
        assertFalse(item.isPinned)
    }

    @Test
    fun fromSummary_whenPinnedNull_isPinnedIsFalseByDefault() {
        val summary = TrajectorySummary(
            summary = "Test Conversation",
            annotations = Annotations(title = "Test", pinned = null)
        )
        val item = ConversationItem.fromSummary("conv-3", summary)
        assertFalse(item.isPinned)
    }

    @Test
    fun fromSummary_withExplicitIsPinnedParam_respectsParam() {
        val summary = TrajectorySummary(
            summary = "Test Conversation",
            annotations = Annotations(title = "Test", pinned = false)
        )
        val item = ConversationItem.fromSummary("conv-4", summary, isPinned = true)
        assertTrue(item.isPinned)

        val item2 = ConversationItem.fromSummary("conv-5", summary, isPinned = false)
        assertFalse(item2.isPinned)
    }

    @Test
    fun sortOrder_pinnedItemsComeFirst() {
        val itemUnpinnedRecent = ConversationItem(
            id = "c1",
            title = "Recent Unpinned",
            status = ConversationStatus.IDLE,
            stepCount = 10,
            lastModifiedTime = "2026-10-31T10:00:00Z",
            isPinned = false
        )
        val itemPinnedOlder = ConversationItem(
            id = "c2",
            title = "Older Pinned",
            status = ConversationStatus.IDLE,
            stepCount = 5,
            lastModifiedTime = "2026-10-30T10:00:00Z",
            isPinned = true
        )
        val itemUnpinnedOld = ConversationItem(
            id = "c3",
            title = "Old Unpinned",
            status = ConversationStatus.IDLE,
            stepCount = 1,
            lastModifiedTime = "2026-10-29T10:00:00Z",
            isPinned = false
        )

        val list = listOf(itemUnpinnedOld, itemUnpinnedRecent, itemPinnedOlder)
        val sorted = list.sortedWith(
            compareByDescending<ConversationItem> { it.isPinned }
                .thenByDescending { it.lastModifiedEpochMs }
        )

        // Pinned item must come first regardless of timestamps
        assertEquals("c2", sorted[0].id)
        assertEquals("c1", sorted[1].id)
        assertEquals("c3", sorted[2].id)
    }

    @Test
    fun unpinningSynchronizesCorrectlyInSorting() {
        // Simulates unpinning: an item previously pinned changes to isPinned = false
        val itemNowUnpinned = ConversationItem(
            id = "troubleshooting",
            title = "Troubleshooting",
            status = ConversationStatus.IDLE,
            stepCount = 501,
            lastModifiedTime = "2026-10-31T01:57:43Z",
            isPinned = false
        )
        val itemOtherRunning = ConversationItem(
            id = "xiaomi-ocr",
            title = "Xiaomi OCR",
            status = ConversationStatus.RUNNING,
            stepCount = 256,
            lastModifiedTime = "2026-10-31T02:10:00Z",
            isPinned = false
        )

        val list = listOf(itemNowUnpinned, itemOtherRunning)
        val sorted = list.sortedWith(
            compareByDescending<ConversationItem> { it.isPinned }
                .thenByDescending { it.lastModifiedEpochMs }
        )

        // Since itemNowUnpinned is unpinned, the more recent item comes first
        assertEquals("xiaomi-ocr", sorted[0].id)
        assertEquals("troubleshooting", sorted[1].id)
    }

    @Test
    fun fromSummary_swarmMetadataAndRunningOrder() {
        val activeSwarmSummary = TrajectorySummary(
            summary = "iPhone Duo 适配开发",
            status = "CASCADE_RUN_STATUS_IDLE",
            lastModifiedTime = "2026-11-24T05:27:13Z",
            hasSwarm = true,
            swarmWorkingCount = 2,
            swarmTotalMembers = 5
        )
        val activeSwarm = ConversationItem.fromSummary("swarm-1", activeSwarmSummary)
        assertTrue(activeSwarm.hasSwarm)
        assertEquals(2, activeSwarm.swarmWorkingCount)
        assertEquals(5, activeSwarm.swarmTotalMembers)
        assertEquals(ConversationStatus.RUNNING, activeSwarm.status)
        assertTrue(activeSwarm.isSwarmWorking)

        val idleRecent = ConversationItem(
            id = "idle-recent",
            title = "Recent Idle",
            status = ConversationStatus.IDLE,
            lastModifiedTime = "2026-11-24T06:10:00Z"
        )
        val pinnedOld = ConversationItem(
            id = "pinned-old",
            title = "Pinned Old",
            status = ConversationStatus.IDLE,
            lastModifiedTime = "2026-11-24T01:00:00Z",
            isPinned = true
        )
        val sorted = listOf(idleRecent, activeSwarm, pinnedOld).sortedWith(
            compareByDescending<ConversationItem> { it.status == ConversationStatus.ACTION }
                .thenByDescending { it.isPinned }
                .thenByDescending { it.status == ConversationStatus.RUNNING }
                .thenByDescending { it.lastModifiedEpochMs }
        )
        assertEquals(listOf("pinned-old", "swarm-1", "idle-recent"), sorted.map { it.id })
    }
}
