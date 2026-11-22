package com.antigravity.mobile.data.model

import com.antigravity.mobile.ui.util.loc
import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

@Serializable
data class ModelTokenBreakdown(
    val model: String = "",
    val total: Long = 0,
    val input: Long = 0,
    val output: Long = 0,
    @SerialName("cache_read") val cacheRead: Long = 0,
    val thinking: Long = 0,
    val steps: Int = 0,
    @SerialName("cost_usd") val costUsd: Double = 0.0
)

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
    val models: Map<String, Long> = emptyMap(),
    @SerialName("model_details") val modelDetails: Map<String, ModelTokenBreakdown> = emptyMap()
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
    @SerialName("daily_matrix") val dailyMatrix: List<DailyTokenRecord> = emptyList(),
    @SerialName("model_summary") val modelSummary: List<ModelTokenBreakdown> = emptyList()
) {
    /** 后端下发毫秒级 Unix 时间戳；兼容秒级历史结构并转为毫秒 */
    val updatedAtEpochMs: Long
        get() = if (updatedAt in 1 until 10_000_000_000L) updatedAt * 1000L else updatedAt

    /** 返回按总 Token 降序的全量模型明细（兼容旧网关未下发 model_summary 的情况） */
    fun resolvedModelSummary(): List<ModelTokenBreakdown> {
        if (modelSummary.isNotEmpty()) {
            return modelSummary.sortedWith(
                compareByDescending<ModelTokenBreakdown> { it.total }.thenBy { it.model }
            )
        }
        val totals = LinkedHashMap<String, Long>()
        for (rec in dailyMatrix) {
            for ((m, cnt) in rec.models) {
                if (cnt > 0L) totals[m] = (totals[m] ?: 0L) + cnt
            }
        }
        return totals.entries
            .sortedWith(compareByDescending<Map.Entry<String, Long>> { it.value }.thenBy { it.key })
            .map { (m, cnt) -> ModelTokenBreakdown(model = m, total = cnt) }
    }
}
