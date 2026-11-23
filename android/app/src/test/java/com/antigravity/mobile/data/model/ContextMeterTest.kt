package com.antigravity.mobile.data.model

import com.antigravity.mobile.data.service.JsonConfig
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class ContextMeterTest {
    private val json = JsonConfig.instance

    @Test
    fun decodesSessionTotalsAndCompactionState() {
        val body = """{
            "usedTokens": 160000,
            "maxTokens": 200000,
            "percentage": 80.0,
            "inputTokens": 12000,
            "cacheReadTokens": 145000,
            "outputTokens": 3000,
            "thinkingTokens": 1100,
            "sessionSteps": 7,
            "sessionInputTokens": 84000,
            "sessionCacheReadTokens": 916000,
            "sessionOutputTokens": 22000,
            "sessionThinkingTokens": 9500,
            "sessionTotalTokens": 1022000,
            "sessionCacheHitRate": 91.6,
            "sessionCostUsd": 0.42
        }"""
        val meter = json.decodeFromString<ContextMeterItem>(body)
        assertTrue(meter.hasSessionTotals())
        assertEquals(7, meter.sessionSteps)
        assertEquals(84000L, meter.sessionInputTokens)
        assertEquals(916000L, meter.sessionCacheReadTokens)
        assertEquals(22000L, meter.sessionOutputTokens)
        assertEquals(9500L, meter.sessionThinkingTokens)
        assertEquals(91.6, meter.effectiveSessionCacheHitRate()!!, 0.01)
        assertEquals(0.42, meter.sessionCostUsd, 0.001)
        assertTrue(meter.isNearCompactionCeiling())
    }

    @Test
    fun legacyGatewayPayloadHidesSessionTotalsAndComputesFallbackHitRate() {
        val legacy = """{
            "usedTokens": 45000,
            "maxTokens": 200000,
            "percentage": 22.5,
            "inputTokens": 5000,
            "cacheReadTokens": 40000,
            "outputTokens": 800
        }"""
        val legacyMeter = json.decodeFromString<ContextMeterItem>(legacy)
        assertFalse(legacyMeter.hasSessionTotals())
        assertNull(legacyMeter.effectiveSessionCacheHitRate())
        assertFalse(legacyMeter.isNearCompactionCeiling())

        val partialSession = ContextMeterItem(
            usedTokens = 190000,
            maxTokens = 200000,
            percentage = 95.0,
            sessionInputTokens = 20000,
            sessionCacheReadTokens = 80000,
            sessionOutputTokens = 4000
        )
        assertTrue(partialSession.hasSessionTotals())
        assertEquals(80.0, partialSession.effectiveSessionCacheHitRate()!!, 0.01)
        assertTrue(partialSession.isNearCompactionCeiling())
    }
}
