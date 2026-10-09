package com.antigravity.mobile.data.model

import com.antigravity.mobile.data.service.JsonConfig
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class SubagentItemTest {
    private val json = JsonConfig.instance

    @Test
    fun decodesSubagentsFromStreamPayload() {
        val body = """{"type":"update","cascadeId":"p1","status":"CASCADE_RUN_STATUS_IDLE",
            "subagents":[
              {"conversationId":"c295e4d0-c590","typeName":"research","role":"Embodied AI Researcher","prompt":"p","modelTier":"MODEL_TIER_FLASH","stepIndex":6,"status":"running","stepCount":12},
              {"conversationId":"97d4c3fb-6322"},
              {"conversationId":"x1","status":"gone"}
            ]}"""
        val payload = json.decodeFromString<StreamUpdatePayload>(body)
        val subs = payload.subagents!!
        assertEquals(3, subs.size)

        assertTrue(subs[0].isRunning)
        assertEquals("Embodied AI Researcher", subs[0].displayName)
        assertEquals("运行中", subs[0].statusText)
        assertEquals(12, subs[0].stepCount)

        assertFalse(subs[1].isRunning)
        assertEquals("97d4c3fb", subs[1].displayName)
        assertEquals("", subs[1].statusText)

        assertTrue(subs[2].isGone)
        assertEquals("已清理", subs[2].statusText)
        assertNull(payload.parentConversationId)
    }

    @Test
    fun missingSubagentFieldsDecodeToNull() {
        val payload = json.decodeFromString<StreamUpdatePayload>("""{"type":"update","cascadeId":"c"}""")
        assertNull(payload.subagents)
        assertNull(payload.parentConversationId)
        assertNull(payload.subagentRole)
    }

    @Test
    fun subagentSessionIdentityComesFromPayload() {
        val payload = json.decodeFromString<StreamUpdatePayload>(
            """{"type":"init","cascadeId":"c1","parentConversationId":"p1","subagentRole":"AI Hardware Analyst"}"""
        )
        assertEquals("p1", payload.parentConversationId)
        assertEquals("AI Hardware Analyst", payload.subagentRole)
    }

    @Test
    fun displayNameFallsBackToTypeThenId() {
        assertEquals("research", SubagentItem(conversationId = "abcdef123456", typeName = "research").displayName)
        assertEquals("abcdef12", SubagentItem(conversationId = "abcdef123456", role = "  ", typeName = "").displayName)
    }
}
