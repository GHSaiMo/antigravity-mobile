package com.antigravity.mobile.data.service

import com.antigravity.mobile.data.model.CockpitAccountQuota
import com.antigravity.mobile.data.model.CockpitQuotaBucket
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class ModelPickerQuotaCacheTest {
    private val account = CockpitAccountQuota(
        id = "acc-1",
        email = "user@example.com",
        claude5h = CockpitQuotaBucket(remainingPercent = 100.0),
        claudeWeekly = CockpitQuotaBucket(remainingPercent = 92.0),
        gemini5h = CockpitQuotaBucket(remainingPercent = 88.0),
        geminiWeekly = CockpitQuotaBucket(remainingPercent = 74.0),
        gemini4Argon = CockpitQuotaBucket(remainingPercent = 63.0)
    )

    @Test
    fun claudeBackedOrchestratorUsesClaudeQuotaPool() {
        // Synthetic Opus 5.5 orchestrator without explicit quotaPool (legacy gateway / cached catalog)
        assertEquals(
            100.0,
            ModelPickerQuotaCache.remainingPercent(
                account = account,
                providerKind = "orchestrator",
                title = "Orch Opus 5.5",
                variantIds = listOf(
                    "gemini-pro-orchestrator-opus-5-5-low",
                    "gemini-pro-orchestrator-opus-5-5-medium",
                    "gemini-pro-orchestrator-opus-5-5-high"
                ),
                weekly = false,
                variantRawNames = listOf("Orchestrator (Opus 5.5) (High)")
            )
        )
        assertEquals(
            92.0,
            ModelPickerQuotaCache.remainingPercent(
                account = account,
                providerKind = "orchestrator",
                title = "Orch Opus 5.5",
                variantIds = listOf("gemini-pro-orchestrator-opus-5-5-high"),
                weekly = true
            )
        )
        // Sonnet / Haiku orchestrators also map to Claude
        assertEquals(
            100.0,
            ModelPickerQuotaCache.remainingPercent(
                account = account,
                providerKind = "orchestrator",
                title = "Orch Sonnet 5.5",
                variantIds = listOf("gemini-pro-orchestrator-sonnet-5-5-high"),
                weekly = false
            )
        )
        // Future unknown Anthropic codename resolved via gateway quotaPool="claude"
        assertEquals(
            100.0,
            ModelPickerQuotaCache.remainingPercent(
                account = account,
                providerKind = "orchestrator",
                title = "Orch Fable 5.2",
                variantIds = listOf("gemini-pro-orchestrator-fable-5-2-high"),
                weekly = false,
                quotaPools = listOf("claude")
            )
        )
    }

    @Test
    fun geminiBackedOrchestratorsKeepGeminiAndArgonPools() {
        assertEquals(
            88.0,
            ModelPickerQuotaCache.remainingPercent(
                account = account,
                providerKind = "orchestrator",
                title = "Orch Neon",
                variantIds = listOf("gemini-pro-orchestrator-neon-high"),
                weekly = false,
                quotaPools = listOf("gemini")
            )
        )
        assertEquals(
            74.0,
            ModelPickerQuotaCache.remainingPercent(
                account = account,
                providerKind = "orchestrator",
                title = "Orch Carbon",
                variantIds = listOf("gemini-pro-orchestrator-carbon-high"),
                weekly = true,
                quotaPools = listOf("gemini")
            )
        )
        assertEquals(
            63.0,
            ModelPickerQuotaCache.remainingPercent(
                account = account,
                providerKind = "orchestrator",
                title = "Orch Argon",
                variantIds = listOf("gemini-pro-orchestrator-argon-high"),
                weekly = false,
                quotaPools = listOf("gemini")
            )
        )
    }

    @Test
    fun directProvidersAndOtherBehaveAsExpected() {
        assertEquals(
            100.0,
            ModelPickerQuotaCache.remainingPercent(
                account = account,
                providerKind = "claude",
                title = "Opus 5.5",
                variantIds = listOf("claude-opus-5-5-high"),
                weekly = false
            )
        )
        assertEquals(
            88.0,
            ModelPickerQuotaCache.remainingPercent(
                account = account,
                providerKind = "gemini",
                title = "4 Neon",
                variantIds = listOf("gemini-4-neon-high"),
                weekly = false
            )
        )
        assertNull(
            ModelPickerQuotaCache.remainingPercent(
                account = account,
                providerKind = "other",
                title = "GPT-OSS",
                variantIds = listOf("gpt-oss-120b-medium"),
                weekly = false
            )
        )
    }
}
