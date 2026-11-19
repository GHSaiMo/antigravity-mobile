package com.antigravity.mobile.data.service

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class LiveActivityStatusHelperTest {

    @Test
    fun statusRunningRecognizesBothRawAndPrefixed() {
        assertTrue(LiveActivityStatusHelper.isStatusRunning("RUNNING"))
        assertTrue(LiveActivityStatusHelper.isStatusRunning("CASCADE_RUN_STATUS_RUNNING"))
        assertTrue(LiveActivityStatusHelper.isStatusRunning("cascade_run_status_running"))
        assertFalse(LiveActivityStatusHelper.isStatusRunning("CASCADE_RUN_STATUS_IDLE"))
        assertFalse(LiveActivityStatusHelper.isStatusRunning("IDLE"))
        assertFalse(LiveActivityStatusHelper.isStatusRunning("CASCADE_RUN_STATUS_ERROR"))
        assertFalse(LiveActivityStatusHelper.isStatusRunning(null))
        assertFalse(LiveActivityStatusHelper.isStatusRunning(""))
    }

    @Test
    fun statusErrorRecognizesErrorAndFailed() {
        assertTrue(LiveActivityStatusHelper.isStatusError("ERROR"))
        assertTrue(LiveActivityStatusHelper.isStatusError("FAILED"))
        assertTrue(LiveActivityStatusHelper.isStatusError("CASCADE_RUN_STATUS_ERROR"))
        assertTrue(LiveActivityStatusHelper.isStatusError("CASCADE_RUN_STATUS_FAILED"))
        assertFalse(LiveActivityStatusHelper.isStatusError("CASCADE_RUN_STATUS_IDLE"))
        assertFalse(LiveActivityStatusHelper.isStatusError("CASCADE_RUN_STATUS_RUNNING"))
    }

    @Test
    fun awaitingClearsImmediatelyWhenServerConfirmsRunning() {
        // Once server reports RUNNING, statusRunning alone keeps isRunning=true.
        // Clearing awaiting prevents sticky "running" when the task later goes IDLE.
        val awaiting = LiveActivityStatusHelper.resolveAwaitingResponse(
            wasAwaiting = true,
            statusRunning = true,
            isError = false,
            hasObservedServerRunning = true,
            hasSubsequentNonUserMessage = false,
            hasPendingOptimisticMessage = true,
            awaitingElapsedMs = 300L
        )
        assertFalse(awaiting)
    }

    @Test
    fun awaitingSurvivesBriefStaleIdleFrameRightAfterSend() {
        // Immediately after pressing Send, a stale WS frame with IDLE may arrive before POST completes.
        val awaiting = LiveActivityStatusHelper.resolveAwaitingResponse(
            wasAwaiting = true,
            statusRunning = false,
            isError = false,
            hasObservedServerRunning = false,
            hasSubsequentNonUserMessage = false,
            hasPendingOptimisticMessage = true,
            awaitingElapsedMs = 500L
        )
        assertTrue(awaiting)
    }

    @Test
    fun awaitingClearsWhenServerFinishesWithToolsOnlyReply() {
        // Even if the final turn only emitted tool/subagent steps (non-user message) and went IDLE,
        // awaiting must clear so the notification transitions to completed.
        val awaiting = LiveActivityStatusHelper.resolveAwaitingResponse(
            wasAwaiting = true,
            statusRunning = false,
            isError = false,
            hasObservedServerRunning = false,
            hasSubsequentNonUserMessage = true,
            hasPendingOptimisticMessage = false,
            awaitingElapsedMs = 1_200L
        )
        assertFalse(awaiting)
    }

    @Test
    fun awaitingClearsOnTimeoutEvenIfOptimisticStillPending() {
        val awaiting = LiveActivityStatusHelper.resolveAwaitingResponse(
            wasAwaiting = true,
            statusRunning = false,
            isError = false,
            hasObservedServerRunning = false,
            hasSubsequentNonUserMessage = false,
            hasPendingOptimisticMessage = true,
            awaitingElapsedMs = LiveActivityStatusHelper.DEFAULT_AWAITING_TIMEOUT_MS
        )
        assertFalse(awaiting)
    }

    @Test
    fun trackedActivityFinishesImmediatelyOnceObservedRunningThenIdle() {
        assertTrue(
            LiveActivityStatusHelper.shouldFinishTrackedActivity(
                statusRunning = false,
                isError = false,
                canProceed = false,
                hasObservedServerRunning = true,
                baselineStepCount = 10,
                currentStepCount = 10,
                elapsedSinceStartMs = 1_000L
            )
        )
    }

    @Test
    fun trackedActivityWaitsDuringOptimisticGraceWhenServerStillIdleWithSameSteps() {
        assertFalse(
            LiveActivityStatusHelper.shouldFinishTrackedActivity(
                statusRunning = false,
                isError = false,
                canProceed = false,
                hasObservedServerRunning = false,
                baselineStepCount = 10,
                currentStepCount = 10,
                elapsedSinceStartMs = 2_000L
            )
        )
    }

    @Test
    fun trackedActivityFinishesFastTaskWhenStepCountAdvanced() {
        assertTrue(
            LiveActivityStatusHelper.shouldFinishTrackedActivity(
                statusRunning = false,
                isError = false,
                canProceed = false,
                hasObservedServerRunning = false,
                baselineStepCount = 10,
                currentStepCount = 12,
                elapsedSinceStartMs = 1_500L
            )
        )
    }

    @Test
    fun trackedActivityFinishesAfterOptimisticGraceExpires() {
        assertTrue(
            LiveActivityStatusHelper.shouldFinishTrackedActivity(
                statusRunning = false,
                isError = false,
                canProceed = false,
                hasObservedServerRunning = false,
                baselineStepCount = 10,
                currentStepCount = 10,
                elapsedSinceStartMs = LiveActivityStatusHelper.DEFAULT_OPTIMISTIC_GRACE_MS
            )
        )
    }

    @Test
    fun completionAlertFormattingHandlesCompletedProceedFailedAndCancelled() {
        val completed = LiveActivityStatusHelper.formatCompletionAlert("COMPLETED", "修复安卓通知")
        assertNotNull(completed)
        assertEquals("🎉 Antigravity 任务已完成", completed!!.first)
        assertEquals("【修复安卓通知】已完成，点击查看详情。", completed.second)

        val proceed = LiveActivityStatusHelper.formatCompletionAlert("PROCEED", "重构方案")
        assertNotNull(proceed)
        assertEquals("📋 方案已就绪，请审批", proceed!!.first)

        val failed = LiveActivityStatusHelper.formatCompletionAlert("FAILED", "编译任务")
        assertNotNull(failed)
        assertEquals("❌ Antigravity 任务执行失败", failed!!.first)

        // User-cancelled tasks should not trigger a loud failure alert
        assertNull(LiveActivityStatusHelper.formatCompletionAlert("CANCELLED", "任意任务"))
    }
}
