package com.antigravity.mobile.data.service

import com.antigravity.mobile.data.model.CockpitAccountQuota
import com.antigravity.mobile.data.model.CockpitQuotaBucket
import com.antigravity.mobile.data.model.ModelDefaults
import kotlinx.coroutines.flow.MutableStateFlow

enum class ModelQuotaPoolKind {
    GEMINI,
    CLAUDE
}

/**
 * 模型选择器右侧额度伴随卡的只读缓存：由首页额度拉取同步当前账号额度，零额外请求。
 * 开关与匀速游标状态由 [PreferencesManager] 写入。
 */
object ModelPickerQuotaCache {
    val account = MutableStateFlow<CockpitAccountQuota?>(null)
    val enabled = MutableStateFlow(true)
    val weekly = MutableStateFlow(false)
    val weeklyPaceCursorEnabled = MutableStateFlow(true)

    private val CLAUDE_ORCHESTRATOR_TOKENS = listOf("claude", "opus", "sonnet", "haiku", "anthropic")

    fun usesClaudeQuotaPool(
        title: String,
        variantIds: List<String>,
        variantRawNames: List<String>,
        quotaPools: List<String?>
    ): Boolean {
        val explicit = quotaPools.asSequence()
            .mapNotNull { it?.trim()?.lowercase()?.takeIf(String::isNotEmpty) }
            .firstOrNull()
        if (explicit == "claude") return true
        if (explicit == "gemini") return false

        val titleLower = title.lowercase()
        if (CLAUDE_ORCHESTRATOR_TOKENS.any { titleLower.contains(it) }) return true
        return variantIds.any { id ->
            val lower = id.lowercase()
            ModelDefaults.isClaude(lower) || CLAUDE_ORCHESTRATOR_TOKENS.any { lower.contains(it) }
        } || variantRawNames.any { raw ->
            val lower = raw.lowercase()
            CLAUDE_ORCHESTRATOR_TOKENS.any { lower.contains(it) }
        }
    }

    fun quotaPool(
        providerKind: String,
        title: String,
        variantIds: List<String>,
        variantRawNames: List<String> = emptyList(),
        quotaPools: List<String?> = emptyList()
    ): ModelQuotaPoolKind? = when (providerKind.lowercase()) {
        "claude" -> ModelQuotaPoolKind.CLAUDE
        "gemini" -> ModelQuotaPoolKind.GEMINI
        "orchestrator" -> {
            if (usesClaudeQuotaPool(title, variantIds, variantRawNames, quotaPools)) {
                ModelQuotaPoolKind.CLAUDE
            } else {
                ModelQuotaPoolKind.GEMINI
            }
        }
        else -> null
    }

    /**
     * 兼容保留：返回某个模型行对应共享池剩余百分比（0..100，仅 Gemini / Claude 双池）。
     */
    fun remainingPercent(
        account: CockpitAccountQuota?,
        providerKind: String,
        title: String,
        variantIds: List<String>,
        weekly: Boolean,
        variantRawNames: List<String> = emptyList(),
        quotaPools: List<String?> = emptyList()
    ): Double? {
        val acc = account ?: return null
        val pool = quotaPool(providerKind, title, variantIds, variantRawNames, quotaPools) ?: return null
        val bucket: CockpitQuotaBucket? = when (pool) {
            ModelQuotaPoolKind.CLAUDE -> if (weekly) acc.claudeWeekly else (acc.claude5h ?: acc.claudeWeekly)
            ModelQuotaPoolKind.GEMINI -> if (weekly) acc.geminiWeekly else (acc.gemini5h ?: acc.geminiWeekly)
        }
        val pct = bucket?.remainingPercent ?: return null
        if (pct.isNaN() || pct.isInfinite()) return null
        return pct.coerceIn(0.0, 100.0)
    }
}
