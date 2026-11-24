package com.antigravity.mobile.data.service

import com.antigravity.mobile.data.model.CockpitAccountQuota
import com.antigravity.mobile.data.model.CockpitQuotaBucket
import kotlinx.coroutines.flow.MutableStateFlow

/**
 * 模型选择器行内余量的只读缓存：由首页额度拉取同步当前账号额度，零额外请求。
 * 开关与 5h / Weekly 窗口由 [PreferencesManager] 写入。
 */
object ModelPickerQuotaCache {
    val account = MutableStateFlow<CockpitAccountQuota?>(null)
    val enabled = MutableStateFlow(true)
    val weekly = MutableStateFlow(false)

    /**
     * 返回某个模型行对应额度池剩余百分比（0..100）；无数据或无法归属时返回 null。
     * Gemini / Orchestrator 归 Gemini 池（Argon 若网关单独下发则用 Argon 桶），Claude 归 Claude 池。
     */
    fun remainingPercent(
        account: CockpitAccountQuota?,
        providerKind: String,
        title: String,
        variantIds: List<String>,
        weekly: Boolean
    ): Double? {
        val acc = account ?: return null
        val bucket: CockpitQuotaBucket? = when (providerKind.lowercase()) {
            "claude" -> if (weekly) acc.claudeWeekly else (acc.claude5h ?: acc.claudeWeekly)
            "gemini", "orchestrator" -> {
                val isArgon = title.contains("argon", ignoreCase = true) ||
                    variantIds.any { it.contains("argon", ignoreCase = true) }
                val argon = acc.gemini4Argon
                if (isArgon && argon != null) argon
                else if (weekly) acc.geminiWeekly else (acc.gemini5h ?: acc.geminiWeekly)
            }
            else -> null
        }
        val pct = bucket?.remainingPercent ?: return null
        if (pct.isNaN() || pct.isInfinite()) return null
        return pct.coerceIn(0.0, 100.0)
    }
}
