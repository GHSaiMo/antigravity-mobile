package com.antigravity.mobile.data.service

import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.net.Uri
import android.os.Handler
import android.os.Looper
import androidx.core.app.NotificationCompat
import androidx.core.app.NotificationManagerCompat
import com.antigravity.mobile.MainActivity
import com.antigravity.mobile.R

class LiveActivityNotificationManager(
    private val context: Context,
    private val prefs: PreferencesManager? = null
) {
    private val notificationManager = context.getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
    private val handler = Handler(Looper.getMainLooper())
    private var dismissRunnable: Runnable? = null

    private var activeCascadeId: String? = null
    private var isNotificationActive: Boolean = false
    private var lastActionNotifiedForCascade: String? = null

    init {
        NotificationChannelManager.createNotificationChannels(context)
    }

    private fun resolveNotificationId(cascadeId: String): Int {
        val hash = cascadeId.hashCode() and 0x7FFFFFFF
        return if (hash == 0) 1001 else hash
    }

    private fun resolveAlertNotificationId(cascadeId: String): Int {
        val hash = (cascadeId + "_alert").hashCode() and 0x7FFFFFFF
        return if (hash == 0) 2001 else hash
    }

    fun startOrUpdateActivity(
        title: String,
        cascadeId: String,
        status: String = "RUNNING",
        stepCount: Int = 1,
        latestAction: String = "正在执行任务...",
        runningTaskCount: Int = 0,
        hasPendingAction: Boolean = false
    ) {
        if (prefs?.enableLiveNotifications == false) {
            cancelActivity()
            return
        }

        dismissRunnable?.let { handler.removeCallbacks(it) }
        dismissRunnable = null

        activeCascadeId = cascadeId
        isNotificationActive = true

        val notifId = resolveNotificationId(cascadeId)

        val intent = Intent(context, MainActivity::class.java).apply {
            action = Intent.ACTION_VIEW
            data = Uri.parse("antigravity://cascade/$cascadeId")
            flags = Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TOP
        }

        val pendingIntent = PendingIntent.getActivity(
            context,
            notifId,
            intent,
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        )

        val resolvedTitle = title.ifBlank { "Agent 任务执行中" }
        val actionText = when {
            latestAction.isNotBlank() -> latestAction
            runningTaskCount > 0 -> "正在执行后台任务 ($runningTaskCount)..."
            stepCount > 0 -> "正在执行第 $stepCount 步..."
            else -> "正在执行..."
        }

        val subText = if (hasPendingAction) "需要用户审批" else "Antigravity Agent"

        // 1. Update persistent Live Activity progress notification
        val builder = NotificationCompat.Builder(context, NotificationChannelManager.CHANNEL_LIVE_ACTIVITY)
            .setContentTitle(resolvedTitle)
            .setContentText(actionText)
            .setSubText(subText)
            .setSmallIcon(R.drawable.ic_stat_antigravity)
            .setColor(0xFF4F46E5.toInt())
            .setContentIntent(pendingIntent)
            .setOngoing(true)
            .setOnlyAlertOnce(true)
            .setProgress(0, 0, true)
            .setCategory(NotificationCompat.CATEGORY_PROGRESS)
            .setPriority(NotificationCompat.PRIORITY_LOW)

        try {
            NotificationManagerCompat.from(context).notify(notifId, builder.build())
        } catch (_: SecurityException) {
            // Handled on Android 13+ when permission not yet granted
        }

        // 2. If user interaction / approval is required, post a high-priority heads-up alert with sound & vibration
        if (hasPendingAction && lastActionNotifiedForCascade != cascadeId) {
            lastActionNotifiedForCascade = cascadeId
            notifyAlert(
                title = "⚠️ Antigravity 需要审批",
                body = "【$resolvedTitle】$actionText",
                deeplink = "antigravity://cascade/$cascadeId?action=review",
                isAction = true
            )
        } else if (!hasPendingAction) {
            lastActionNotifiedForCascade = null
        }
    }

    fun endActivity(
        cascadeId: String? = null,
        finalStatus: String = "COMPLETED"
    ) {
        if (!isNotificationActive) return
        val targetCascadeId = cascadeId ?: activeCascadeId ?: return
        if (activeCascadeId != null && targetCascadeId != activeCascadeId) return

        val notifId = resolveNotificationId(targetCascadeId)

        val summary = when (finalStatus) {
            "COMPLETED" -> "任务已完成"
            "CANCELLED" -> "任务已终止"
            "FAILED" -> "执行遇到错误"
            else -> "执行已结束"
        }

        val builder = NotificationCompat.Builder(context, NotificationChannelManager.CHANNEL_LIVE_ACTIVITY)
            .setContentTitle("任务已结束")
            .setContentText(summary)
            .setSubText("Antigravity Agent")
            .setSmallIcon(R.drawable.ic_stat_antigravity)
            .setColor(if (finalStatus == "COMPLETED") 0xFF10B981.toInt() else 0xFF6B7280.toInt())
            .setOngoing(false)
            .setAutoCancel(true)
            .setProgress(0, 0, false)
            .setOnlyAlertOnce(true)
            .setPriority(NotificationCompat.PRIORITY_LOW)

        try {
            NotificationManagerCompat.from(context).notify(notifId, builder.build())
        } catch (_: SecurityException) {}

        // Post a persistent completion or failure alert to the ALERTS channel so the user is informed
        if (prefs?.enableLiveNotifications != false) {
            val alertTitle = if (finalStatus == "COMPLETED") "🎉 Antigravity 任务已完成" else "❌ Antigravity 任务执行失败"
            notifyAlert(
                title = alertTitle,
                body = "任务 $summary，点击查看详情。",
                deeplink = "antigravity://cascade/$targetCascadeId",
                isAction = false
            )
        }

        isNotificationActive = false
        activeCascadeId = null
        lastActionNotifiedForCascade = null

        // Auto-dismiss the live progress bar after 4 seconds (parity with iOS Live Activity dismissal)
        dismissRunnable?.let { handler.removeCallbacks(it) }
        val run = Runnable {
            try {
                notificationManager.cancel(notifId)
            } catch (_: Exception) {}
        }
        dismissRunnable = run
        handler.postDelayed(run, 4000L)
    }

    fun notifyAlert(
        title: String,
        body: String,
        deeplink: String,
        isAction: Boolean = false
    ) {
        if (prefs?.enableLiveNotifications == false) return

        val intent = Intent(context, MainActivity::class.java).apply {
            action = Intent.ACTION_VIEW
            data = Uri.parse(deeplink)
            flags = Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TOP
        }

        val alertNotifId = (deeplink.hashCode() and 0x7FFFFFFF)

        val pendingIntent = PendingIntent.getActivity(
            context,
            alertNotifId,
            intent,
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        )

        val alertBuilder = NotificationCompat.Builder(context, NotificationChannelManager.CHANNEL_ALERTS)
            .setContentTitle(title)
            .setContentText(body)
            .setSmallIcon(R.drawable.ic_stat_antigravity)
            .setColor(if (isAction) 0xFFF59E0B.toInt() else 0xFF4F46E5.toInt())
            .setContentIntent(pendingIntent)
            .setAutoCancel(true)
            .setPriority(NotificationCompat.PRIORITY_HIGH)
            .setCategory(if (isAction) NotificationCompat.CATEGORY_ALARM else NotificationCompat.CATEGORY_EVENT)
            .setDefaults(NotificationCompat.DEFAULT_ALL)

        try {
            NotificationManagerCompat.from(context).notify(alertNotifId, alertBuilder.build())
        } catch (_: SecurityException) {}
    }

    fun cancelActivity() {
        dismissRunnable?.let { handler.removeCallbacks(it) }
        dismissRunnable = null
        isNotificationActive = false
        val cid = activeCascadeId
        activeCascadeId = null
        lastActionNotifiedForCascade = null
        try {
            if (cid != null) {
                notificationManager.cancel(resolveNotificationId(cid))
            } else {
                notificationManager.cancel(NOTIFICATION_ID)
            }
        } catch (_: Exception) {}
    }

    companion object {
        const val CHANNEL_ID = NotificationChannelManager.CHANNEL_LIVE_ACTIVITY
        const val NOTIFICATION_ID = 1001
    }
}
