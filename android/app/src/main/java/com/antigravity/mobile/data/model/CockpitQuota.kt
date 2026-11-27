package com.antigravity.mobile.data.model

import com.antigravity.mobile.ui.util.loc
import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import java.text.SimpleDateFormat
import java.util.*

@Serializable
data class CockpitQuotaBucket(
    @SerialName("remaining_fraction") val remainingFraction: Double = 0.0,
    @SerialName("remaining_percent") val remainingPercent: Double = 0.0,
    @SerialName("reset_time") val resetTime: String? = null,
    @SerialName("reset_friendly") val resetFriendly: String? = null,
    @SerialName("est_cycle_used_tokens") val estCycleUsedTokens: Long = 0L,
    @SerialName("est_remaining_tokens") val estRemainingTokens: Long = 0L,
    @SerialName("est_total_tokens") val estTotalTokens: Long = 0L,
    @SerialName("est_reliable") val estReliable: Boolean = false
) {
    val formattedResetClockTime: String?
        get() {
            val rt = resetTime?.takeIf { it.isNotBlank() } ?: return null
            return try {
                val inputFmt = SimpleDateFormat("yyyy-MM-dd'T'HH:mm:ss", Locale.US)
                inputFmt.timeZone = TimeZone.getTimeZone("UTC")
                val date = inputFmt.parse(rt.substringBefore('.')) ?: return null
                val outputFmt = SimpleDateFormat("MM/dd HH:mm", Locale.getDefault())
                "(${outputFmt.format(date)})"
            } catch (_: Exception) {
                null
            }
        }

    val resetCountdownDisplay: String?
        get() {
            val friendly = resetFriendly?.takeIf { it.isNotBlank() } ?: return null
            val localizedFriendly = friendly.loc
            if (
                friendly == "已就绪" || friendly == "未知" ||
                friendly.equals("ready", ignoreCase = true) || friendly.equals("unknown", ignoreCase = true) ||
                localizedFriendly == "已就绪".loc || localizedFriendly == "未知".loc
            ) return localizedFriendly
            val clock = formattedResetClockTime
            return if (clock != null) "$localizedFriendly $clock" else localizedFriendly
        }

    fun calculateWeeklyPace(totalWindowHours: Double = 168.0): WeeklyPaceInfo? {
        return calculatePace(totalWindowHours)
    }

    fun calculatePaceForTitle(title: String, tier: String? = null): WeeklyPaceInfo? {
        val hours = when {
            title.contains("carbon", ignoreCase = true) -> 168.0
            title.contains("argon", ignoreCase = true) -> {
                val isUltra = tier?.trim()?.contains("ULTRA", ignoreCase = true) == true
                if (isUltra) 5.0 else 168.0
            }
            title.contains("5h", ignoreCase = true) -> 5.0
            title.contains("Weekly", ignoreCase = true) -> 168.0
            else -> 168.0
        }
        return calculatePace(hours)
    }

    fun calculatePace(totalWindowHours: Double): WeeklyPaceInfo? {
        val rt = resetTime?.takeIf { it.isNotBlank() } ?: return null
        return try {
            val inputFmt = SimpleDateFormat("yyyy-MM-dd'T'HH:mm:ss", Locale.US)
            inputFmt.timeZone = TimeZone.getTimeZone("UTC")
            val date = inputFmt.parse(rt.substringBefore('.')) ?: return null
            val remainingMillis = date.time - System.currentTimeMillis()
            if (remainingMillis <= 0) return null
            val totalMillis = totalWindowHours * 3600.0 * 1000.0
            val expectedRemaining = ((remainingMillis / totalMillis) * 100.0).coerceIn(0.0, 100.0)
            val actualRemaining = remainingPercent.coerceIn(0.0, 100.0)
            val used = (100.0 - actualRemaining).coerceIn(0.0, 100.0)
            val expectedUsed = (100.0 - expectedRemaining).coerceIn(0.0, 100.0)
            val delta = actualRemaining - expectedRemaining
            val isOver = delta < -1.0 // 实际剩余比理论应剩少 1% 以上判定为超速

            val deltaAbsStr = String.format(Locale.US, "%.1f%%", Math.abs(delta))
            val status = if (isOver) {
                "消耗偏快 (-%s)".loc(deltaAbsStr)
            } else if (Math.abs(delta) <= 1.0) {
                "匀速正常 (±%s)".loc(deltaAbsStr)
            } else {
                "节奏充裕 (+%s)".loc(deltaAbsStr)
            }

            WeeklyPaceInfo(
                actualRemainingPercent = actualRemaining,
                expectedRemainingPercent = expectedRemaining,
                usedPercent = used,
                expectedUsedPercent = expectedUsed,
                paceDeltaPercent = delta,
                isOverPacing = isOver,
                statusText = status,
                windowHours = totalWindowHours,
                resetCountdownDisplay = resetCountdownDisplay ?: "计算中".loc
            )
        } catch (_: Exception) {
            null
        }
    }
}

@Serializable
data class WeeklyPaceInfo(
    val actualRemainingPercent: Double,
    val expectedRemainingPercent: Double,
    val usedPercent: Double,
    val expectedUsedPercent: Double,
    val paceDeltaPercent: Double,
    val isOverPacing: Boolean,
    val statusText: String,
    val windowHours: Double = 168.0,
    val resetCountdownDisplay: String
) {
    val windowLabel: String
        get() = if (windowHours <= 5.0) "5小时".loc else "7天".loc
}

enum class QuotaBucketKind(val key: String, val displayTitle: String) {
    AUTO("auto", "自动最紧"),
    GEMINI_5H("gemini_5h", "Gemini · 5h"),
    GEMINI_WEEKLY("gemini_weekly", "Gemini · Weekly"),
    CLAUDE_5H("claude_5h", "Claude · 5h"),
    CLAUDE_WEEKLY("claude_weekly", "Claude · Weekly");

    companion object {
        fun fromKey(key: String?): QuotaBucketKind {
            return entries.firstOrNull { it.key == key } ?: GEMINI_5H
        }
    }
}

@Serializable
data class CockpitAccountQuota(
    val id: String,
    val email: String = "",
    val name: String = "",
    @SerialName("subscription_tier") val subscriptionTier: String? = null,
    @SerialName("is_current") val isCurrent: Boolean = false,
    @SerialName("claude_5h") val claude5h: CockpitQuotaBucket? = null,
    @SerialName("claude_weekly") val claudeWeekly: CockpitQuotaBucket? = null,
    @SerialName("gemini_5h") val gemini5h: CockpitQuotaBucket? = null,
    @SerialName("gemini_weekly") val geminiWeekly: CockpitQuotaBucket? = null,
    @SerialName("gemini_4_argon") val gemini4Argon: CockpitQuotaBucket? = null,
    @SerialName("updated_at") val updatedAt: Long = 0
) {
    val displayName: String
        get() = if (name.isNotBlank()) "$name ($email)" else email

    val maskedEmail: String
        get() {
            val atIdx = email.indexOf('@')
            if (atIdx <= 1) return email
            val prefix = email.substring(0, 1)
            val domain = email.substring(atIdx)
            return "$prefix***$domain"
        }

    val isFreeTier: Boolean
        get() = subscriptionTier?.trim()?.equals("FREE", ignoreCase = true) == true ||
            (gemini5h == null && claude5h == null)

    fun bucket(kind: QuotaBucketKind): CockpitQuotaBucket? = when (kind) {
        QuotaBucketKind.AUTO -> null
        QuotaBucketKind.GEMINI_5H -> gemini5h
        QuotaBucketKind.GEMINI_WEEKLY -> geminiWeekly
        QuotaBucketKind.CLAUDE_5H -> claude5h
        QuotaBucketKind.CLAUDE_WEEKLY -> claudeWeekly
    }

    fun resolvedQuotaDisplay(preferred: QuotaBucketKind): Pair<QuotaBucketKind, CockpitQuotaBucket>? {
        val candidatesOrder = if (isFreeTier) {
            listOf(QuotaBucketKind.GEMINI_WEEKLY, QuotaBucketKind.CLAUDE_WEEKLY)
        } else {
            listOf(
                QuotaBucketKind.GEMINI_5H,
                QuotaBucketKind.GEMINI_WEEKLY,
                QuotaBucketKind.CLAUDE_5H,
                QuotaBucketKind.CLAUDE_WEEKLY
            )
        }
        if (preferred == QuotaBucketKind.AUTO) {
            var tightest: Pair<QuotaBucketKind, CockpitQuotaBucket>? = null
            for (kind in candidatesOrder) {
                val b = bucket(kind) ?: continue
                if (tightest == null || b.remainingPercent < tightest.second.remainingPercent) {
                    tightest = kind to b
                }
            }
            return tightest
        }
        bucket(preferred)?.let { return preferred to it }
        for (kind in candidatesOrder) {
            bucket(kind)?.let { return kind to it }
        }
        return null
    }
}

@Serializable
data class AutoSwitchConfig(
    val enabled: Boolean = false,
    @SerialName("target_bucket") val targetBucket: String = "gemini_5h",
    @SerialName("threshold_percent") val thresholdPercent: Double = 10.0,
    @SerialName("wait_until_idle") val waitUntilIdle: Boolean = true,
    val strategy: String = "highest_quota",
    val candidates: List<String> = emptyList(),
    @SerialName("debounce_sec") val debounceSec: Int = 15,
    @SerialName("cooldown_sec") val cooldownSec: Int = 180
)

@Serializable
data class AutoSwitchStatus(
    val state: String = "disabled",
    val reason: String? = null,
    @SerialName("last_check_at") val lastCheckAt: Long = 0L,
    @SerialName("last_switch_at") val lastSwitchAt: Long = 0L,
    @SerialName("last_from_email") val lastFromEmail: String? = null,
    @SerialName("last_to_email") val lastToEmail: String? = null,
    @SerialName("last_to_remaining_pct") val lastToRemainingPct: Double = 0.0,
    @SerialName("last_switch_notice_id") val lastSwitchNoticeId: String? = null
)

@Serializable
data class AutoSwitchResponse(
    val config: AutoSwitchConfig = AutoSwitchConfig(),
    val status: AutoSwitchStatus = AutoSwitchStatus(),
    @SerialName("cockpit_installed") val cockpitInstalled: Boolean = false
)

@Serializable
data class CockpitQuotaResponse(
    @SerialName("current_account") val currentAccount: CockpitAccountQuota? = null,
    val accounts: List<CockpitAccountQuota> = emptyList(),
    @SerialName("updated_at") val updatedAt: Long = 0,
    val source: String? = null,
    // null from older gateways (treated as installed)
    @SerialName("cockpit_installed") val cockpitInstalled: Boolean? = null,
    @SerialName("cockpit_hint") val cockpitHint: String? = null
) {
    /** 网关未配置 Cockpit 时为 true,此时只有当前账号的实时额度。 */
    val needsCockpitInstall: Boolean get() = cockpitInstalled == false
}
