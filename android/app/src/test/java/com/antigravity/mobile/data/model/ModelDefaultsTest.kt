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
        assertTrue(ModelDefaults.isClaude("claude-opus-5-5-high"))
        assertTrue(ModelDefaults.isClaude("MODEL_PLACEHOLDER_M26"))
        assertTrue(ModelDefaults.isClaude("MODEL_PLACEHOLDER_M402"))
        assertTrue(ModelDefaults.isClaude("MODEL_PLACEHOLDER_M405"))
        assertFalse(ModelDefaults.isClaude("gemini-3.8-flash-high"))
        assertFalse(ModelDefaults.isClaude("MODEL_PLACEHOLDER_M417"))
        assertEquals("Claude", ModelDefaults.providerLabel("claude-sonnet-4-6"))
        assertEquals("Gemini", ModelDefaults.providerLabel("gemini-3.1-pro-low"))
        assertEquals("GPT", ModelDefaults.providerLabel("gpt-oss-120b-medium"))
    }

    @Test
    fun toggleUsesConfiguredDefaults() {
        assertEquals(g, ModelDefaults.toggleTarget("claude-opus-4-6-thinking", g, c))
        assertEquals(c, ModelDefaults.toggleTarget("gemini-3.7-flash-low", g, c))
        // 非 Gemini（含 Claude 与其他）在旧版双核切换规则中退回 Gemini 默认
        assertEquals(g, ModelDefaults.toggleTarget("gpt-oss-120b-medium", g, c))
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

    @Test
    fun formatPickerLabelFiltersNewFastAndNoticeTags() {
        assertEquals("Claude Opus 5.5 (High)", ModelDefaults.formatPickerLabel("Claude Opus 5.5 (High)", "New"))
        assertEquals("Claude Opus 5.5 (High)", ModelDefaults.formatPickerLabel("Claude Opus 5.5 (High)", "new"))
        assertEquals("Claude Opus 5.5 (High)", ModelDefaults.formatPickerLabel("Claude Opus 5.5 (High)", " NEW "))
        assertEquals("Gemini 3.8 Flash (High)", ModelDefaults.formatPickerLabel("Gemini 3.8 Flash (High)", "Fast"))
        assertEquals("Gemini 3.8 Flash (High)", ModelDefaults.formatPickerLabel("Gemini 3.8 Flash (High)", "FAST"))
        assertEquals("Gemini 3.8 Flash (High)", ModelDefaults.formatPickerLabel("Gemini 3.8 Flash (High)", " fast "))
        assertEquals("Claude Sonnet 4.6 (Thinking)", ModelDefaults.formatPickerLabel("Claude Sonnet 4.6 (Thinking)", "Notice"))
        assertEquals("Claude Sonnet 4.6 (Thinking)", ModelDefaults.formatPickerLabel("Claude Sonnet 4.6 (Thinking)", "notice"))
        assertEquals("Claude Sonnet 4.6 (Thinking)", ModelDefaults.formatPickerLabel("Claude Sonnet 4.6 (Thinking)", " NOTICE "))
        assertEquals("Claude Opus 5.5 (High)", ModelDefaults.formatPickerLabel("Claude Opus 5.5 (High)", null))
        assertEquals("Claude Opus 5.5 (High)", ModelDefaults.formatPickerLabel("Claude Opus 5.5 (High)", "   "))
        assertEquals("GPT-OSS 120B (Medium) · Beta", ModelDefaults.formatPickerLabel("GPT-OSS 120B (Medium)", "Beta"))
    }

    @Test
    fun displayNameUnifiesPresetAndUnknownIds() {
        assertEquals("Gemini 3.8 Flash (High)", ModelDefaults.displayName("gemini-3.8-flash-high"))
        assertEquals("Claude Opus 4.6 (Thinking)", ModelDefaults.displayName("claude-opus-4-6-thinking"))
        assertEquals("Claude Sonnet 4.6 (Thinking)", ModelDefaults.displayName("claude-sonnet-4-6"))
        assertEquals("Claude Opus 6 (High)", ModelDefaults.displayName("claude-opus-6-high"))
    }

    @Test
    fun effortSubmenuTopToBottomIsLowToExtraWithMediumDefault() {
        val options = listOf(
            ModelOption(id = "gemini-4-hydrogen-xhigh", name = "Gemini 4 Hydrogen (Extra)", provider = "gemini"),
            ModelOption(id = "gemini-4-hydrogen-high", name = "Gemini 4 Hydrogen (High)", provider = "gemini"),
            ModelOption(id = "gemini-4-hydrogen-medium", name = "Gemini 4 Hydrogen (Medium)", provider = "gemini"),
            ModelOption(id = "gemini-4-hydrogen-low", name = "Gemini 4 Hydrogen (Low)", provider = "gemini")
        )
        val families = ModelDefaults.modelFamilies(options)
        assertEquals(1, families.size)
        val hydro = families.first()
        assertEquals(
            listOf("gemini-4-hydrogen-low", "gemini-4-hydrogen-medium", "gemini-4-hydrogen-high", "gemini-4-hydrogen-xhigh"),
            hydro.variantsTopToBottom.map { it.id }
        )
        assertEquals("gemini-4-hydrogen-medium", hydro.defaultVariant?.id)
        assertEquals(1, hydro.defaultVariantIndexTopToBottom)
    }

    @Test
    fun favoriteToggleAndValidationBehavior() {
        val fav1 = "gemini-4-carbon-medium"
        val fav2 = "claude-opus-5-5-high"
        // Fav1 -> Fav2 -> Fav1
        assertEquals(fav2, ModelDefaults.toggleFavoriteTarget(fav1, fav1, fav2))
        assertEquals(fav1, ModelDefaults.toggleFavoriteTarget(fav2, fav1, fav2))
        // Third/custom model -> back to Fav1
        assertEquals(fav1, ModelDefaults.toggleFavoriteTarget("gemini-pro-orchestrator", fav1, fav2))

        val catalog = ModelsResponse(
            models = listOf(
                ModelOption(id = "gemini-4-carbon-medium", name = "Gemini 4 Carbon (Medium)", provider = "gemini"),
                ModelOption(id = "claude-sonnet-4-6", name = "Claude Sonnet 4.6", provider = "claude")
            ),
            defaults = mapOf("gemini" to "gemini-4-carbon-medium", "claude" to "claude-sonnet-4-6"),
            live = true
        )
        assertEquals("claude-sonnet-4-6", ModelDefaults.validatedFavorite("gemini-pro-orchestrator", ModelDefaults.FALLBACK_CLAUDE, avoid = "gemini-4-carbon-medium", catalog = catalog))
    }

    @Test
    fun modelFamiliesGroupsGeminiClaudeOrchestratorInOrder() {
        val mixed = listOf(
            ModelOption(id = "gemini-pro-orchestrator", name = "Orchestrator (Carbon)", provider = "orchestrator"),
            ModelOption(id = "claude-opus-5-5-medium", name = "Claude Opus 5.5 (Medium)", provider = "claude"),
            ModelOption(id = "gemini-4-carbon-medium", name = "Gemini 4 Carbon (Medium)", provider = "gemini")
        )
        val families = ModelDefaults.modelFamilies(mixed)
        assertEquals(listOf("gemini", "claude", "orchestrator"), families.map { it.provider })
    }

    @Test
    fun formatSlidePickerTitleStripsProviderPrefixAndShortensOrchestrator() {
        assertEquals("4 Carbon", ModelDefaults.formatSlidePickerTitle("Gemini 4 Carbon"))
        assertEquals("3.8 Flash", ModelDefaults.formatSlidePickerTitle("Gemini 3.8 Flash"))
        assertEquals("Opus 5.5", ModelDefaults.formatSlidePickerTitle("Claude Opus 5.5"))
        assertEquals("Sonnet 4.6", ModelDefaults.formatSlidePickerTitle("Claude Sonnet 4.6"))
        assertEquals("Orch · Carbon", ModelDefaults.formatSlidePickerTitle("Orchestrator (Carbon)"))
        assertEquals("Orch · Argon", ModelDefaults.formatSlidePickerTitle("Orchestrator (Argon)"))
        assertEquals("Orch", ModelDefaults.formatSlidePickerTitle("Orchestrator"))
    }
}
