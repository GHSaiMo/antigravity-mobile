package com.antigravity.mobile.data.service

import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.net.Uri
import android.os.Build
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

    init {
        createNotificationChannel()
    }

    private fun createNotificationChannel() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            val channel = NotificationChannel(
                CHANNEL_ID,
                "任务实时进展",
                NotificationManager.IMPORTANCE_LOW
            ).apply {
                description = "在锁屏与通知栏常驻展示 Agent 任务执行进度与后台指令状态"
                setShowBadge(false)
                enableLights(false)
                enableVibration(false)
            }
            notificationManager.createNotificationChannel(channel)
        }
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

        val intent = Intent(context, MainActivity::class.java).apply {
            action = Intent.ACTION_VIEW
            data = Uri.parse("antigravity://session/$cascadeId")
            flags = Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TOP
        }

        val pendingIntent = PendingIntent.getActivity(
            context,
            cascadeId.hashCode(),
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

        val builder = NotificationCompat.Builder(context, CHANNEL_ID)
            .setContentTitle(resolvedTitle)
            .setContentText(actionText)
            .setSubText(subText)
            .setSmallIcon(android.R.drawable.stat_notify_sync)
            .setColor(0xFF4F46E5.toInt())
            .setContentIntent(pendingIntent)
            .setOngoing(true)
            .setOnlyAlertOnce(true)
            .setProgress(0, 0, true)
            .setCategory(NotificationCompat.CATEGORY_PROGRESS)
            .setPriority(NotificationCompat.PRIORITY_LOW)

        try {
            NotificationManagerCompat.from(context).notify(NOTIFICATION_ID, builder.build())
        } catch (_: SecurityException) {
            // Permission not granted on Android 13+
        }
    }

    fun endActivity(
        cascadeId: String? = null,
        finalStatus: String = "COMPLETED"
    ) {
        if (!isNotificationActive) return
        if (cascadeId != null && activeCascadeId != null && cascadeId != activeCascadeId) return

        val summary = when (finalStatus) {
            "COMPLETED" -> "任务已完成"
            "CANCELLED" -> "任务已终止"
            "FAILED" -> "执行遇到错误"
            else -> "执行已结束"
        }

        val builder = NotificationCompat.Builder(context, CHANNEL_ID)
            .setContentTitle("任务已结束")
            .setContentText(summary)
            .setSubText("Antigravity Agent")
            .setSmallIcon(android.R.drawable.stat_notify_sync)
            .setColor(if (finalStatus == "COMPLETED") 0xFF10B981.toInt() else 0xFF6B7280.toInt())
            .setOngoing(false)
            .setAutoCancel(true)
            .setProgress(0, 0, false)
            .setOnlyAlertOnce(true)
            .setPriority(NotificationCompat.PRIORITY_LOW)

        try {
            NotificationManagerCompat.from(context).notify(NOTIFICATION_ID, builder.build())
        } catch (_: SecurityException) {}

        isNotificationActive = false
        activeCascadeId = null

        // Auto-dismiss notification after 4 seconds (1:1 parity with iOS Live Activity dismissal policy)
        dismissRunnable?.let { handler.removeCallbacks(it) }
        val run = Runnable {
            try {
                notificationManager.cancel(NOTIFICATION_ID)
            } catch (_: Exception) {}
        }
        dismissRunnable = run
        handler.postDelayed(run, 4000L)
    }

    fun cancelActivity() {
        dismissRunnable?.let { handler.removeCallbacks(it) }
        dismissRunnable = null
        isNotificationActive = false
        activeCascadeId = null
        try {
            notificationManager.cancel(NOTIFICATION_ID)
        } catch (_: Exception) {}
    }

    companion object {
        const val CHANNEL_ID = "antigravity_live_activity"
        const val NOTIFICATION_ID = 1001
    }
}
