package com.antigravity.mobile.data.model

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class ConversationTurnOutlineTest {

    @Test
    fun buildTurnsWithOutline_preservesTrueTurnNumbersAndUnloadedTurns() {
        val outline = listOf(
            TurnOutlineItem(
                turnIndex = 1,
                firstMessageId = "step-0",
                messageOffset = 0,
                messageCount = 2,
                startStepIndex = 0,
                endStepIndex = 1,
                promptSummary = "帮我做非 X 渠道推广",
                totalTools = 4
            ),
            TurnOutlineItem(
                turnIndex = 2,
                firstMessageId = "step-2",
                messageOffset = 2,
                messageCount = 2,
                startStepIndex = 2,
                endStepIndex = 3,
                promptSummary = "配置先保留不做试跑"
            ),
            TurnOutlineItem(
                turnIndex = 3,
                firstMessageId = "step-4",
                messageOffset = 4,
                messageCount = 2,
                startStepIndex = 4,
                endStepIndex = 5,
                promptSummary = "对了，请你再跑一轮"
            )
        )

        // Tail slice starts mid-turn-2 (agent reply step-3) + full turn-3 (step-4, step-5)
        val loadedTail = listOf(
            GatewayMessageItem(id = "step-3", type = "agent", role = "agent", text = "好的，已为你保留当前配置", stepIndex = 3),
            GatewayMessageItem(id = "step-4", type = "user", role = "user", text = "对了，请你再跑一轮", stepIndex = 4),
            GatewayMessageItem(id = "step-5", type = "agent", role = "agent", text = "正在掘金...", stepIndex = 5)
        )

        val turns = ConversationTurnBuilder.buildTurns(loadedTail, outline, isRunning = true, isAwaitingResponse = false)

        assertEquals(3, turns.size)

        // Turn 1: completely unloaded, retains real offset 0 & anchor id
        assertEquals(1, turns[0].turnIndex)
        assertEquals("step-0", turns[0].firstMessageId)
        assertEquals(0, turns[0].messageOffset)
        assertFalse(turns[0].isLoaded)
        assertEquals(4, turns[0].totalTools)
        assertTrue(turns[0].messages.isEmpty())

        // Turn 2: tail fragment attached so visible scroll tracks Turn 2, but anchor step-2 is not loaded yet
        assertEquals(2, turns[1].turnIndex)
        assertEquals("step-2", turns[1].firstMessageId)
        assertEquals(2, turns[1].messageOffset)
        assertFalse(turns[1].isLoaded)
        assertEquals(listOf("step-3"), turns[1].messages.map { it.id })

        // Turn 3: fully loaded and active
        assertEquals(3, turns[2].turnIndex)
        assertEquals("step-4", turns[2].firstMessageId)
        assertTrue(turns[2].isLoaded)
        assertTrue(turns[2].isActive)
        assertEquals(listOf("step-4", "step-5"), turns[2].messages.map { it.id })
    }

    @Test
    fun buildTurnsWithOutline_appendsOptimisticUserMessageAsNextTurn() {
        val outline = listOf(
            TurnOutlineItem(
                turnIndex = 1,
                firstMessageId = "step-0",
                messageOffset = 0,
                messageCount = 2,
                startStepIndex = 0,
                endStepIndex = 1,
                promptSummary = "第一轮"
            )
        )
        val loaded = listOf(
            GatewayMessageItem(id = "step-0", type = "user", role = "user", text = "第一轮", stepIndex = 0),
            GatewayMessageItem(id = "step-1", type = "agent", role = "agent", text = "回复", stepIndex = 1),
            GatewayMessageItem(id = "opt_123", type = "user", role = "user", text = "新发的消息")
        )

        val turns = ConversationTurnBuilder.buildTurns(loaded, outline, isRunning = false, isAwaitingResponse = true)
        assertEquals(2, turns.size)
        assertEquals(1, turns[0].turnIndex)
        assertFalse(turns[0].isActive)
        assertEquals(2, turns[1].turnIndex)
        assertEquals("opt_123", turns[1].firstMessageId)
        assertTrue(turns[1].isActive)
        assertTrue(turns[1].isLoaded)
    }
}
