package com.antigravity.mobile.data.model

import kotlinx.serialization.Serializable
import java.time.Instant

@Serializable
data class ContextBreakdownCategory(
    val key: String = "",
    val tokens: Long = 0L,
    val percent: Double = 0.0
)

@Serializable
data class ContextTopConsumer(
    val stepIndex: Int = 0,
    val messageId: String? = null,
    val kind: String = "",
    val label: String = "",
    val tokens: Long = 0L
)

@Serializable
data class ContextBreakdownSummary(
    val categories: List<ContextBreakdownCategory> = emptyList(),
    val topConsumers: List<ContextTopConsumer> = emptyList()
)

@Serializable
data class ContextMeterItem(
    val usedTokens: Long = 0,
    val maxTokens: Long = 200000,
    val percentage: Double = 0.0,
    val inputTokens: Long = 0,
    val outputTokens: Long = 0,
    val cacheReadTokens: Long = 0,
    val thinkingTokens: Long = 0,
    val timeToFirstTokenSec: Double = 0.0,
    val streamingDurationSec: Double = 0.0,
    val tokensPerSecond: Double = 0.0,
    val isOverflow: Boolean = false,
    val errorKind: String? = null,
    val cacheHitRate: Double? = null,
    val cacheMissStreak: Int = 0,
    val cacheMissTokens: Long = 0,
    val cacheStatus: String? = null,
    val lastActivityAt: String? = null,
    val sessionSteps: Int = 0,
    val sessionInputTokens: Long = 0,
    val sessionOutputTokens: Long = 0,
    val sessionCacheReadTokens: Long = 0,
    val sessionThinkingTokens: Long = 0,
    val sessionTotalTokens: Long = 0,
    val sessionCacheHitRate: Double? = null,
    val sessionCostUsd: Double = 0.0,
    val compacted: Boolean = false,
    val checkpointCount: Int = 0,
    val lastCheckpointStep: Int? = null,
    val breakdown: ContextBreakdownSummary? = null
) {
    /**
     * 兼容新旧网关：优先使用网关返回的命中率，缺失时基于最近一步 input + cacheRead 本地计算。
     */
    fun effectiveCacheHitRate(): Double? {
        cacheHitRate?.let { return it.coerceIn(0.0, 100.0) }
        val inp = inputTokens.coerceAtLeast(0L)
        val cr = cacheReadTokens.coerceAtLeast(0L)
        val total = inp + cr
        if (total <= 0L) return null
        return (cr.toDouble() / total.toDouble()) * 100.0
    }

    fun hasSessionTotals(): Boolean =
        sessionSteps > 0 ||
            (sessionInputTokens + sessionOutputTokens + sessionCacheReadTokens + sessionThinkingTokens) > 0L

    fun effectiveSessionCacheHitRate(): Double? {
        sessionCacheHitRate?.let { return it.coerceIn(0.0, 100.0) }
        val inp = sessionInputTokens.coerceAtLeast(0L)
        val cr = sessionCacheReadTokens.coerceAtLeast(0L)
        val total = inp + cr
        if (total <= 0L) return null
        return (cr.toDouble() / total.toDouble()) * 100.0
    }

    fun isNearCompactionCeiling(): Boolean =
        isOverflow || percentage >= 75.0

    fun isCacheMissStreak(): Boolean =
        cacheStatus == "miss_streak" || cacheMissStreak >= 2

    /**
     * 仅当重上下文 (>= 40K)、非运行中且无待审批、距最后活动已闲置 >= 15 分钟时返回闲置分钟数。
     */
    fun staleIdleMinutes(
        isRunning: Boolean,
        hasPendingInteraction: Boolean,
        nowMillis: Long = System.currentTimeMillis()
    ): Int? {
        if (isRunning || hasPendingInteraction || usedTokens < 40_000L) return null
        val raw = lastActivityAt?.trim().orEmpty()
        if (raw.isEmpty()) return null
        val lastMillis = runCatching { Instant.parse(raw).toEpochMilli() }.getOrNull() ?: return null
        val mins = ((nowMillis - lastMillis) / 60_000L).toInt()
        return if (mins >= 15) mins else null
    }

    fun hasCacheWarning(
        isRunning: Boolean,
        hasPendingInteraction: Boolean,
        nowMillis: Long = System.currentTimeMillis()
    ): Boolean = isCacheMissStreak() || staleIdleMinutes(isRunning, hasPendingInteraction, nowMillis) != null
}
