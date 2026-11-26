package com.antigravity.mobile.data.service

import com.antigravity.mobile.data.model.CockpitAccountQuota
import com.antigravity.mobile.data.model.CockpitQuotaBucket
import com.antigravity.mobile.data.model.ModelDefaults
import kotlinx.coroutines.flow.MutableStateFlow

/**
 * 模型选择器行内余量的只读缓存：由首页额度拉取同步当前账号额度，零额外请求。
 * 开关与 5h / Weekly 窗口由 [PreferencesManager] 写入。
 */
object ModelPickerQuotaCache {
    val account = MutableStateFlow<CockpitAccountQuota?>(null)
    val enabled = MutableStateFlow(true)
    val weekly = MutableStateFlow(false)

    private val CLAUDE_ORCHESTRATOR_TOKENS = listOf("claude", "opus", "sonnet", "haiku", "anthropic")

    private fun usesClaudeQuotaPool(
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

    private fun geminiBucket(
        acc: CockpitAccountQuota,
        title: String,
        variantIds: List<String>,
        variantRawNames: List<String>,
        weekly: Boolean
    ): CockpitQuotaBucket? {
        val isArgon = title.contains("argon", ignoreCase = true) ||
            variantIds.any { it.contains("argon", ignoreCase = true) } ||
            variantRawNames.any { it.contains("argon", ignoreCase = true) }
        val argon = acc.gemini4Argon
        return if (isArgon && argon != null) argon
        else if (weekly) acc.geminiWeekly else (acc.gemini5h ?: acc.geminiWeekly)
    }

    /**
     * 返回某个模型行对应额度池剩余百分比（0..100）；无数据或无法归属时返回 null。
     * Claude 及由 Claude 驱动的 Orchestrator（如 Orch Opus / Sonnet）归 Claude 池；
     * 其余 Gemini / Orchestrator 归 Gemini 池（Argon 若网关单独下发则用 Argon 桶）。
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
        val bucket: CockpitQuotaBucket? = when (providerKind.lowercase()) {
            "claude" -> if (weekly) acc.claudeWeekly else (acc.claude5h ?: acc.claudeWeekly)
            "gemini" -> geminiBucket(acc, title, variantIds, variantRawNames, weekly)
            "orchestrator" -> {
                if (usesClaudeQuotaPool(title, variantIds, variantRawNames, quotaPools)) {
                    if (weekly) acc.claudeWeekly else (acc.claude5h ?: acc.claudeWeekly)
                } else {
                    geminiBucket(acc, title, variantIds, variantRawNames, weekly)
                }
            }
            else -> null
        }
        val pct = bucket?.remainingPercent ?: return null
        if (pct.isNaN() || pct.isInfinite()) return null
        return pct.coerceIn(0.0, 100.0)
    }
}
