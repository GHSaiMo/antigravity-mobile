package com.antigravity.mobile.data.service

import android.app.PendingIntent
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.util.Log
import androidx.core.app.NotificationManagerCompat
import com.antigravity.mobile.ui.util.loc
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.launch

/**
 * Handles Lock Screen / Notification Shade action buttons ("允许" / "拒绝" / "停止")
 * directly in the background without forcing the user to unlock and open MainActivity.
 */
class NotificationActionReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        val action = intent.action ?: return
        val cascadeId = intent.getStringExtra(EXTRA_CASCADE_ID)?.trim().orEmpty()
        if (cascadeId.isEmpty()) return

        val alertNotifId = intent.getIntExtra(EXTRA_ALERT_NOTIFICATION_ID, 0)
        val appContext = context.applicationContext
        val pendingResult = goAsync()

        CoroutineScope(Dispatchers.IO + SupervisorJob()).launch {
            try {
                if (alertNotifId != 0) {
                    try {
                        NotificationManagerCompat.from(appContext).cancel(alertNotifId)
                    } catch (_: Exception) {}
                }

                val prefs = PreferencesManager(appContext)
                val connMgr = ConnectionManager(appContext)
                val apiClient = ApiClient(appContext, prefs, connMgr)
                val liveMgr = LiveActivityNotificationManager.getOrCreate(appContext, prefs, apiClient)

                when (action) {
                    ACTION_APPROVE -> {
                        val snap = apiClient.fetchMessages(cascadeId, limit = 5).getOrNull() ?: return@launch
                        val interaction = snap.pendingInteraction
                        if (interaction != null) {
                            val kind = interaction.type.lowercase()
                            val isHighRisk = interaction.approvalPreview?.riskLevel.equals("high", ignoreCase = true)
                            if (isHighRisk || kind == "ask_question" || kind == "workflow_approval" || kind == "elicitation" || !interaction.questions.isNullOrEmpty()) {
                                val promptMsg = if (isHighRisk) {
                                    "高危操作，请打开 App 核验后再允许".loc
                                } else {
                                    "请打开 App 查看完整详情并确认".loc
                                }
                                liveMgr.startOrUpdateActivity(
                                    title = snap.title.orEmpty(),
                                    cascadeId = cascadeId,
                                    status = "WAITING_APPROVAL",
                                    stepCount = snap.totalSteps.coerceAtLeast(1),
                                    latestAction = promptMsg,
                                    hasPendingAction = true,
                                    isOptimisticStart = false,
                                    baselineStepCount = snap.totalSteps,
                                    isHighRiskAction = isHighRisk
                                )
                                if (isHighRisk) {
                                    liveMgr.notifyAlert(
                                        title = "🚨 高危操作审批".loc,
                                        body = promptMsg,
                                        deeplink = "antigravity://cascade/$cascadeId?action=review",
                                        isAction = true,
                                        cascadeId = cascadeId,
                                        isHighRisk = true
                                    )
                                }
                                return@launch
                            }
                            val approveOpt = interaction.options.firstOrNull {
                                it.isDeny != true && it.id != "5" && it.id != "__write_in__"
                            } ?: interaction.options.firstOrNull()
                            val optId = approveOpt?.id ?: interaction.defaultOptionId ?: "1"
                            val scopeVal = approveOpt?.scope ?: 1
                            val submitRes = apiClient.submitInteraction(
                                cascadeId = cascadeId,
                                trajectoryId = interaction.trajectoryId,
                                stepIndex = interaction.stepIndex,
                                type = interaction.type,
                                optionId = optId,
                                scope = scopeVal,
                                allow = true,
                                writeInResponse = "",
                                skipped = false,
                                target = interaction.target,
                                questionResponses = null
                            )
                            if (submitRes.isSuccess) {
                                AntigravityMessagingService.dismissAlertNotification(appContext, cascadeId)
                                liveMgr.startOrUpdateActivity(
                                    title = snap.title.orEmpty(),
                                    cascadeId = cascadeId,
                                    status = "RUNNING",
                                    stepCount = snap.totalSteps.coerceAtLeast(1),
                                    latestAction = "已允许，继续执行中...".loc,
                                    hasPendingAction = false,
                                    isOptimisticStart = true,
                                    baselineStepCount = snap.totalSteps
                                )
                            } else {
                                liveMgr.startOrUpdateActivity(
                                    title = snap.title.orEmpty(),
                                    cascadeId = cascadeId,
                                    status = "WAITING_APPROVAL",
                                    stepCount = snap.totalSteps.coerceAtLeast(1),
                                    latestAction = "操作失败，请重试".loc,
                                    hasPendingAction = true,
                                    isOptimisticStart = false,
                                    baselineStepCount = snap.totalSteps
                                )
                            }
                        } else if (snap.canProceed) {
                            val uri = snap.proceedArtifactUri?.takeIf { it.isNotBlank() } ?: "implementation_plan.md"
                            val procRes = apiClient.proceedArtifact(cascadeId, uri, snap.activeModel)
                            if (procRes.isSuccess) {
                                AntigravityMessagingService.dismissAlertNotification(appContext, cascadeId)
                                liveMgr.startOrUpdateActivity(
                                    title = snap.title.orEmpty(),
                                    cascadeId = cascadeId,
                                    status = "RUNNING",
                                    stepCount = snap.totalSteps.coerceAtLeast(1),
                                    latestAction = "已允许，继续执行中...".loc,
                                    hasPendingAction = false,
                                    isOptimisticStart = true,
                                    baselineStepCount = snap.totalSteps
                                )
                            } else {
                                liveMgr.startOrUpdateActivity(
                                    title = snap.title.orEmpty(),
                                    cascadeId = cascadeId,
                                    status = "WAITING_APPROVAL",
                                    stepCount = snap.totalSteps.coerceAtLeast(1),
                                    latestAction = "操作失败，请重试".loc,
                                    hasPendingAction = true,
                                    isOptimisticStart = false,
                                    baselineStepCount = snap.totalSteps
                                )
                            }
                        } else {
                            reconcileStaleActivity(appContext, liveMgr, cascadeId, snap)
                        }
                    }

                    ACTION_REJECT -> {
                        val snap = apiClient.fetchMessages(cascadeId, limit = 5).getOrNull() ?: return@launch
                        val interaction = snap.pendingInteraction
                        if (interaction != null) {
                            val denyOpt = interaction.options.firstOrNull { it.isDeny == true || it.id == "5" }
                            val submitRes = apiClient.submitInteraction(
                                cascadeId = cascadeId,
                                trajectoryId = interaction.trajectoryId,
                                stepIndex = interaction.stepIndex,
                                type = interaction.type,
                                optionId = denyOpt?.id ?: "",
                                scope = denyOpt?.scope ?: 1,
                                allow = false,
                                writeInResponse = "",
                                skipped = denyOpt == null,
                                target = interaction.target,
                                questionResponses = null
                            )
                            if (submitRes.isSuccess) {
                                AntigravityMessagingService.dismissAlertNotification(appContext, cascadeId)
                                liveMgr.startOrUpdateActivity(
                                    title = snap.title.orEmpty(),
                                    cascadeId = cascadeId,
                                    status = "RUNNING",
                                    stepCount = snap.totalSteps.coerceAtLeast(1),
                                    latestAction = "已拒绝该操作".loc,
                                    hasPendingAction = false,
                                    isOptimisticStart = true,
                                    baselineStepCount = snap.totalSteps
                                )
                            } else {
                                liveMgr.startOrUpdateActivity(
                                    title = snap.title.orEmpty(),
                                    cascadeId = cascadeId,
                                    status = "WAITING_APPROVAL",
                                    stepCount = snap.totalSteps.coerceAtLeast(1),
                                    latestAction = "操作失败，请重试".loc,
                                    hasPendingAction = true,
                                    isOptimisticStart = false,
                                    baselineStepCount = snap.totalSteps
                                )
                            }
                        } else {
                            reconcileStaleActivity(appContext, liveMgr, cascadeId, snap)
                        }
                    }

                    ACTION_STOP -> {
                        apiClient.cancelInvocation(cascadeId, killBackgroundTasks = true)
                        AntigravityMessagingService.dismissAlertNotification(appContext, cascadeId)
                        liveMgr.endActivity(cascadeId = cascadeId, finalStatus = "CANCELLED")
                    }
                }
            } catch (e: Exception) {
                Log.w("NotifAction", "Notification action $action failed: ${e.message}")
            } finally {
                pendingResult.finish()
            }
        }
    }

    private fun reconcileStaleActivity(
        appContext: Context,
        liveMgr: LiveActivityNotificationManager,
        cascadeId: String,
        snap: com.antigravity.mobile.data.model.StreamUpdatePayload
    ) {
        AntigravityMessagingService.dismissAlertNotification(appContext, cascadeId)
        val isStillRunning = LiveActivityStatusHelper.isStatusRunning(snap.status) || !snap.runningTasks.isNullOrEmpty()
        if (isStillRunning) {
            liveMgr.startOrUpdateActivity(
                title = snap.title.orEmpty(),
                cascadeId = cascadeId,
                status = "RUNNING",
                stepCount = snap.totalSteps.coerceAtLeast(1),
                latestAction = "已在其他设备处理，继续执行中".loc,
                hasPendingAction = false,
                isOptimisticStart = false,
                baselineStepCount = snap.totalSteps
            )
        } else {
            val upper = snap.status.uppercase()
            val finalStatus = when {
                snap.hasError || LiveActivityStatusHelper.isStatusError(snap.status) -> "FAILED"
                upper.contains("CANCEL") -> "CANCELLED"
                else -> "COMPLETED"
            }
            liveMgr.endActivity(cascadeId = cascadeId, finalStatus = finalStatus, title = snap.title)
        }
    }

    companion object {
        const val ACTION_APPROVE = "com.antigravity.mobile.ACTION_APPROVE_AGENT"
        const val ACTION_REJECT = "com.antigravity.mobile.ACTION_REJECT_AGENT"
        const val ACTION_STOP = "com.antigravity.mobile.ACTION_STOP_AGENT"
        const val EXTRA_CASCADE_ID = "extra_cascade_id"
        const val EXTRA_ALERT_NOTIFICATION_ID = "extra_alert_notification_id"

        fun buildPendingIntent(
            context: Context,
            action: String,
            cascadeId: String,
            alertNotifId: Int = 0
        ): PendingIntent {
            val intent = Intent(context, NotificationActionReceiver::class.java).apply {
                this.action = action
                putExtra(EXTRA_CASCADE_ID, cascadeId)
                if (alertNotifId != 0) {
                    putExtra(EXTRA_ALERT_NOTIFICATION_ID, alertNotifId)
                }
            }
            val requestCode = "$cascadeId:$action:$alertNotifId".hashCode() and 0x7FFFFFFF
            return PendingIntent.getBroadcast(
                context,
                requestCode,
                intent,
                PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
            )
        }
    }
}
