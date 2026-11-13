package com.antigravity.mobile.data.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

@Serializable
data class DailyTokenRecord(
    val date: String = "",
    val total: Long = 0,
    val input: Long = 0,
    val output: Long = 0,
    @SerialName("cache_read") val cacheRead: Long = 0,
    val thinking: Long = 0,
    val steps: Int = 0,
    @SerialName("cost_usd") val costUsd: Double = 0.0,
    val models: Map<String, Long> = emptyMap()
)

@Serializable
data class TokenHistorySummary(
    @SerialName("total_tokens") val totalTokens: Long = 0,
    @SerialName("today_tokens") val todayTokens: Long = 0,
    @SerialName("last_7d_tokens") val last7dTokens: Long = 0,
    @SerialName("desktop_tokens") val desktopTokens: Long = 0,
    @SerialName("cli_tokens") val cliTokens: Long = 0,
    @SerialName("estimated_cost_usd") val estimatedCostUsd: Double = 0.0,
    @SerialName("estimated_cost_cny") val estimatedCostCny: Double = 0.0,
    @SerialName("active_days") val activeDays: Int = 0,
    @SerialName("total_steps") val totalSteps: Int = 0
)

@Serializable
data class TokenHistoryResponse(
    @SerialName("updated_at") val updatedAt: Long = 0,
    val summary: TokenHistorySummary = TokenHistorySummary(),
    @SerialName("daily_matrix") val dailyMatrix: List<DailyTokenRecord> = emptyList()
)
