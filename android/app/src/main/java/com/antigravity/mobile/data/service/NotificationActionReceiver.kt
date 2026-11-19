package com.antigravity.mobile.data.service

import android.app.PendingIntent
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.util.Log
import androidx.core.app.NotificationManagerCompat
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
                        val snap = apiClient.fetchMessages(cascadeId, limit = 5).getOrNull()
                        val interaction = snap?.pendingInteraction
                        if (interaction != null) {
                            val approveOpt = interaction.options.firstOrNull {
                                it.isDeny != true && it.id != "5" && it.id != "__write_in__"
                            } ?: interaction.options.firstOrNull()
                            val optId = approveOpt?.id ?: interaction.defaultOptionId ?: "1"
                            val scopeVal = approveOpt?.scope ?: 1
                            apiClient.submitInteraction(
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
                        } else if (snap?.canProceed == true) {
                            val uri = snap.proceedArtifactUri?.takeIf { it.isNotBlank() } ?: "implementation_plan.md"
                            apiClient.proceedArtifact(cascadeId, uri, snap.activeModel)
                        }
                        liveMgr.startOrUpdateActivity(
                            title = snap?.title.orEmpty(),
                            cascadeId = cascadeId,
                            status = "RUNNING",
                            stepCount = (snap?.totalSteps ?: 1).coerceAtLeast(1),
                            latestAction = "已允许，继续执行中...",
                            hasPendingAction = false,
                            isOptimisticStart = true,
                            baselineStepCount = snap?.totalSteps ?: -1
                        )
                    }

                    ACTION_REJECT -> {
                        val snap = apiClient.fetchMessages(cascadeId, limit = 5).getOrNull()
                        val interaction = snap?.pendingInteraction
                        if (interaction != null) {
                            val denyOpt = interaction.options.firstOrNull { it.isDeny == true || it.id == "5" }
                            apiClient.submitInteraction(
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
                        }
                        liveMgr.startOrUpdateActivity(
                            title = snap?.title.orEmpty(),
                            cascadeId = cascadeId,
                            status = "RUNNING",
                            stepCount = (snap?.totalSteps ?: 1).coerceAtLeast(1),
                            latestAction = "已拒绝该操作",
                            hasPendingAction = false,
                            isOptimisticStart = true,
                            baselineStepCount = snap?.totalSteps ?: -1
                        )
                    }

                    ACTION_STOP -> {
                        apiClient.cancelInvocation(cascadeId, killBackgroundTasks = true)
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
