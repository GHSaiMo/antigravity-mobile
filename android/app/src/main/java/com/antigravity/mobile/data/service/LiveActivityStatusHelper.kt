package com.antigravity.mobile.data.service

import com.antigravity.mobile.ui.util.loc

/**
 * Pure Kotlin status transition and notification formatting rules shared by
 * [LiveActivityNotificationManager], [AgentTaskForegroundService], and ChatViewModel.
 */
object LiveActivityStatusHelper {
    const val DEFAULT_OPTIMISTIC_GRACE_MS = 8_000L
    const val DEFAULT_AWAITING_TIMEOUT_MS = 10_000L
    const val DEFAULT_NON_OPTIMISTIC_AWAITING_GRACE_MS = 3_000L

    fun isStatusRunning(status: String?): Boolean {
        if (status.isNullOrBlank()) return false
        return status.equals("CASCADE_RUN_STATUS_RUNNING", ignoreCase = true) ||
            status.equals("RUNNING", ignoreCase = true)
    }

    fun isStatusError(status: String?): Boolean {
        if (status.isNullOrBlank()) return false
        return status.equals("CASCADE_RUN_STATUS_ERROR", ignoreCase = true) ||
            status.equals("CASCADE_RUN_STATUS_FAILED", ignoreCase = true) ||
            status.equals("ERROR", ignoreCase = true) ||
            status.equals("FAILED", ignoreCase = true)
    }

    /**
     * Resolves whether ChatViewModel should keep `isAwaitingResponse = true`.
     *
     * `isAwaitingResponse` bridges the brief window after sending a message or clicking Proceed
     * before the server transitions to RUNNING. Once the server confirms RUNNING, errors, returns
     * any non-user reply, or exits the grace window, awaiting is cleared so a subsequent IDLE
     * status can never stay stuck in "running".
     */
    fun resolveAwaitingResponse(
        wasAwaiting: Boolean,
        statusRunning: Boolean,
        isError: Boolean,
        hasObservedServerRunning: Boolean,
        hasSubsequentNonUserMessage: Boolean,
        hasPendingOptimisticMessage: Boolean,
        awaitingElapsedMs: Long,
        optimisticGraceMs: Long = DEFAULT_AWAITING_TIMEOUT_MS
    ): Boolean {
        if (!wasAwaiting) return false
        if (isError) return false
        if (statusRunning) return false
        if (hasObservedServerRunning) return false
        if (hasSubsequentNonUserMessage) return false
        if (!hasPendingOptimisticMessage && awaitingElapsedMs >= DEFAULT_NON_OPTIMISTIC_AWAITING_GRACE_MS) {
            return false
        }
        if (awaitingElapsedMs >= optimisticGraceMs) return false
        return true
    }

    /**
     * Decides whether a tracked live activity notification should transition to finished
     * when the server reports a non-running status.
     *
     * Guards against prematurely ending a brand-new optimistic task whose POST hasn't
     * transitioned server status to RUNNING yet, while still ending ultra-fast tasks
     * immediately if stepCount advanced or an error/proceed state appeared.
     */
    fun shouldFinishTrackedActivity(
        statusRunning: Boolean,
        isError: Boolean,
        canProceed: Boolean,
        hasObservedServerRunning: Boolean,
        baselineStepCount: Int,
        currentStepCount: Int,
        elapsedSinceStartMs: Long,
        optimisticGraceMs: Long = DEFAULT_OPTIMISTIC_GRACE_MS
    ): Boolean {
        if (statusRunning) return false
        if (hasObservedServerRunning) return true
        if (isError || canProceed) return true
        if (baselineStepCount >= 0 && currentStepCount > baselineStepCount) return true
        if (elapsedSinceStartMs >= optimisticGraceMs) return true
        return false
    }

    fun resolveFinalStatus(isError: Boolean, canProceed: Boolean): String = when {
        isError -> "FAILED"
        canProceed -> "PROCEED"
        else -> "COMPLETED"
    }

    fun formatEndedSummary(finalStatus: String): String = when (finalStatus) {
        "COMPLETED" -> "任务已完成".loc
        "PROCEED" -> "方案已就绪，请审批".loc
        "CANCELLED" -> "任务已终止".loc
        "FAILED" -> "执行遇到错误".loc
        else -> "执行已结束".loc
    }

    /**
     * Returns (alertTitle, alertBody) for the high-priority ALERTS channel,
     * or null when no loud completion alert should be fired (e.g., user-initiated CANCELLED).
     */
    fun formatCompletionAlert(finalStatus: String, title: String): Pair<String, String>? {
        val cleanTitle = title.trim().takeIf {
            it.isNotBlank() &&
                it != "未命名会话" && it != "未命名会话".loc &&
                it != "会话详情" && it != "会话详情".loc &&
                it != "Agent 任务执行中" && it != "Agent 任务执行中".loc
        }
        return when (finalStatus) {
            "COMPLETED" -> {
                val body = if (cleanTitle != null) "【%s】已完成，点击查看详情。".loc(cleanTitle) else "任务已完成，点击查看详情。".loc
                "🎉 Antigravity 任务已完成".loc to body
            }
            "PROCEED" -> {
                val body = if (cleanTitle != null) "【%s】方案已生成，点击查看或继续执行。".loc(cleanTitle) else "方案已生成，点击查看或继续执行。".loc
                "📋 方案已就绪，请审批".loc to body
            }
            "FAILED" -> {
                val body = if (cleanTitle != null) "【%s】执行遇到错误，点击查看详情。".loc(cleanTitle) else "任务执行遇到错误，点击查看详情。".loc
                "❌ Antigravity 任务执行失败".loc to body
            }
            "CANCELLED" -> null
            else -> {
                val body = if (cleanTitle != null) "【%s】执行已结束，点击查看详情。".loc(cleanTitle) else "任务执行已结束，点击查看详情。".loc
                "Antigravity 任务已结束".loc to body
            }
        }
    }
}
