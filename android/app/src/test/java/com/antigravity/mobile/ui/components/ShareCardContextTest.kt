package com.antigravity.mobile.ui.components

import com.antigravity.mobile.data.model.GatewayMessageItem
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.ZoneId

class ShareCardContextTest {
    private fun user(id: String) = GatewayMessageItem(id = id, type = "user", text = "q")
    private fun agent(id: String, model: String? = null, name: String? = null) =
        GatewayMessageItem(id = id, type = "agent", text = "a", model = model, modelName = name)

    @Test
    fun agentReplyUsesItsOwnModelNotTheCurrentOne() {
        val msgs = listOf(user("u1"), agent("a1", "gemini-3.8-flash-high", "Gemini 3.8 Flash (High)"), user("u2"), agent("a2", "claude-opus-4-6-thinking", "Claude Opus 4.6 (Thinking)"))
        val (name, claude) = ShareCardContext.resolveModel(msgs[1], msgs, activeModel = "claude-opus-4-6-thinking", activeModelName = "Claude Opus 4.6 (Thinking)")
        assertEquals("Gemini 3.8 Flash (High)", name)
        assertFalse(claude)
    }

    @Test
    fun userQuestionUsesTheReplyThatFollowedIt() {
        val msgs = listOf(user("u1"), agent("a1", "gemini-3.7-flash-low", "Gemini 3.7 Flash (Low)"), user("u2"), agent("a2", "claude-sonnet-4-6", "Claude Sonnet 4.6 (Thinking)"))
        val (n1, c1) = ShareCardContext.resolveModel(msgs[0], msgs, "claude-sonnet-4-6", null)
        assertEquals("Gemini 3.7 Flash (Low)", n1); assertFalse(c1)
        val (n2, c2) = ShareCardContext.resolveModel(msgs[2], msgs, "gemini-3.8-flash-high", null)
        assertEquals("Claude Sonnet 4.6 (Thinking)", n2); assertTrue(c2)
    }

    @Test
    fun userQuestionDoesNotBorrowALaterTurnsModel() {
        // u1 之后还没有回复，下一轮回复属于 u2，不能拿来给 u1 用
        val msgs = listOf(user("u1"), user("u2"), agent("a2", "claude-sonnet-4-6", "Claude Sonnet 4.6 (Thinking)"))
        val (name, _) = ShareCardContext.resolveModel(msgs[0], msgs, "gemini-3.8-flash-high", "Gemini 3.8 Flash (High)")
        assertEquals("Gemini 3.8 Flash (High)", name)
    }

    @Test
    fun fallsBackToConversationModel() {
        val msgs = listOf(user("u1"), agent("a1"))
        assertEquals("Gemini 3.8 Flash (High)" to false, ShareCardContext.resolveModel(msgs[1], msgs, "gemini-3.8-flash-high", "Gemini 3.8 Flash (High)"))
        // 没有展示名时由 id 推断
        val (name, claude) = ShareCardContext.resolveModel(msgs[1], msgs, "claude-opus-4-6-thinking", null)
        assertEquals("Opus 4.6 Thinking", name)
        assertTrue(claude)
    }

    @Test
    fun idOnlyMessageStillGetsAReadableName() {
        val msgs = listOf(user("u1"), agent("a1", model = "gemini-3.8-flash-high"))
        val (name, _) = ShareCardContext.resolveModel(msgs[1], msgs, "claude-opus-4-6-thinking", null)
        assertTrue(name.startsWith("Gemini 3.8"))
    }

    @Test
    fun startedAtFormatsInLocalZoneNotShareTime() {
        val shanghai = ZoneId.of("Asia/Shanghai")
        assertEquals("2026-10-08 12:39", ShareCardContext.formatStartedAt("2026-10-08T04:39:13.864926Z", shanghai))
        assertEquals("2026-10-07 21:39", ShareCardContext.formatStartedAt("2026-10-08T04:39:13Z", ZoneId.of("America/Los_Angeles")))
        assertEquals("2026-10-08 12:00", ShareCardContext.formatStartedAt("2026-10-08T12:00:00+08:00", shanghai))
    }

    @Test
    fun startedAtMissingOrInvalidShowsNothing() {
        assertNull(ShareCardContext.formatStartedAt(null))
        assertNull(ShareCardContext.formatStartedAt(""))
        assertNull(ShareCardContext.formatStartedAt("yesterday"))
    }
}
