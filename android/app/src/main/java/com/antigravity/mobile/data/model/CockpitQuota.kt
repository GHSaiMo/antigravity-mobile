package com.antigravity.mobile.data.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import java.text.SimpleDateFormat
import java.util.*

@Serializable
data class CockpitQuotaBucket(
    @SerialName("remaining_fraction") val remainingFraction: Double = 0.0,
    @SerialName("remaining_percent") val remainingPercent: Double = 0.0,
    @SerialName("reset_time") val resetTime: String? = null,
    @SerialName("reset_friendly") val resetFriendly: String? = null
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
            if (friendly == "已就绪" || friendly == "未知") return friendly
            val clock = formattedResetClockTime
            return if (clock != null) "$friendly $clock" else friendly
        }
}

@Serializable
data class CockpitAccountQuota(
    val id: String,
    val email: String = "",
    val name: String = "",
    @SerialName("is_current") val isCurrent: Boolean = false,
    @SerialName("claude_5h") val claude5h: CockpitQuotaBucket? = null,
    @SerialName("claude_weekly") val claudeWeekly: CockpitQuotaBucket? = null,
    @SerialName("gemini_5h") val gemini5h: CockpitQuotaBucket? = null,
    @SerialName("gemini_weekly") val geminiWeekly: CockpitQuotaBucket? = null,
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
}

@Serializable
data class CockpitQuotaResponse(
    @SerialName("current_account") val currentAccount: CockpitAccountQuota? = null,
    val accounts: List<CockpitAccountQuota> = emptyList(),
    @SerialName("updated_at") val updatedAt: Long = 0
)
