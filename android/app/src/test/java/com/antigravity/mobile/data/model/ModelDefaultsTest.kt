package com.antigravity.mobile.data.model

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class ModelDefaultsTest {
    private val g = "gemini-3.8-flash-medium"
    private val c = "claude-sonnet-4-6"

    @Test
    fun providerDetectionAndLabel() {
        assertTrue(ModelDefaults.isClaude("claude-opus-4-6-thinking"))
        assertTrue(ModelDefaults.isClaude("MODEL_PLACEHOLDER_M26"))
        assertFalse(ModelDefaults.isClaude("gemini-3.8-flash-high"))
        assertEquals("Claude", ModelDefaults.providerLabel("claude-sonnet-4-6"))
        assertEquals("Gemini", ModelDefaults.providerLabel("gemini-3.1-pro-low"))
        assertEquals("GPT", ModelDefaults.providerLabel("gpt-oss-120b-medium"))
    }

    @Test
    fun toggleUsesConfiguredDefaults() {
        assertEquals(g, ModelDefaults.toggleTarget("claude-opus-4-6-thinking", g, c))
        assertEquals(c, ModelDefaults.toggleTarget("gemini-3.7-flash-low", g, c))
        // 非 Claude（含 GPT）都切到 Claude 默认
        assertEquals(c, ModelDefaults.toggleTarget("gpt-oss-120b-medium", g, c))
    }

    @Test
    fun resolveActiveKeepsConversationModel() {
        // 会话实际在用的模型原样保留，而不是被压成两个固定值
        assertEquals("gemini-3.5-flash-lite", ModelDefaults.resolveActive("gemini-3.5-flash-lite", "x", g, c))
        assertEquals("claude-opus-4-6-thinking", ModelDefaults.resolveActive("claude-opus-4-6-thinking", "x", g, c))
    }

    @Test
    fun resolveActiveFallsBackForEmptyOrBareEnum() {
        assertEquals("current", ModelDefaults.resolveActive(null, "current", g, c))
        assertEquals("current", ModelDefaults.resolveActive("  ", "current", g, c))
        assertEquals(c, ModelDefaults.resolveActive("MODEL_PLACEHOLDER_M26", "current", g, c))
        assertEquals(g, ModelDefaults.resolveActive("MODEL_PLACEHOLDER_M99999", "current", g, c))
    }

    @Test
    fun validatedReplacesRetiredDefaults() {
        val catalog = ModelsResponse(
            models = listOf(
                ModelOption(id = "gemini-3.8-flash-high", name = "Gemini 3.8 Flash (High)", provider = "gemini"),
                ModelOption(id = "claude-sonnet-4-6", name = "Claude Sonnet 4.6", provider = "claude")
            ),
            defaults = mapOf("gemini" to "gemini-3.8-flash-high", "claude" to "claude-sonnet-4-6"),
            live = true
        )
        assertEquals("gemini-3.8-flash-high", ModelDefaults.validated("gemini-3.8-flash-high", "gemini", catalog))
        assertEquals("gemini-3.8-flash-high", ModelDefaults.validated("gemini-1.0-gone", "gemini", catalog))
        // 提供商不匹配也视为无效（防止把 Claude 存进 Gemini 槽）
        assertEquals("claude-sonnet-4-6", ModelDefaults.validated("gemini-3.8-flash-high", "claude", catalog))
        // 兜底（非实时）列表不应触发重置
        assertEquals("gemini-1.0-gone", ModelDefaults.validated("gemini-1.0-gone", "gemini", catalog.copy(live = false)))
    }
}
