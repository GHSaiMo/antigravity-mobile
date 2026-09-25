package com.antigravity.mobile.data.service

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
import com.antigravity.mobile.data.model.ConversationItem
import java.util.concurrent.ConcurrentHashMap

class LiveActivityNotificationManager(
    private val context: Context,
    private val prefs: PreferencesManager? = null
) {
    private val notificationManager = context.getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
    private val handler = Handler(Looper.getMainLooper())
    private val dismissRunnables = ConcurrentHashMap<Int, Runnable>()

    // Track active cascade IDs that currently have a running notification
    private val activeCascadeIds = ConcurrentHashMap.newKeySet<String>()
    @Volatile private var activeCascadeId: String? = null
    @Volatile private var isNotificationActive: Boolean = false
    private var lastActionNotifiedForCascade: String? = null

    init {
        NotificationChannelManager.createNotificationChannels(context)
        cleanUpOrphanedActivities()
    }

    private fun resolveNotificationId(cascadeId: String): Int {
        val hash = cascadeId.hashCode() and 0x7FFFFFFF
        return if (hash == 0) 1001 else hash
    }

    private fun resolveAlertNotificationId(cascadeId: String): Int {
        val hash = (cascadeId + "_alert").hashCode() and 0x7FFFFFFF
        return if (hash == 0) 2001 else hash
    }

    fun hasNotification(cascadeId: String): Boolean {
        val notifId = resolveNotificationId(cascadeId)
        if (activeCascadeIds.contains(cascadeId)) return true
        return try {
            notificationManager.activeNotifications.any { it.id == notifId }
        } catch (_: Exception) {
            false
        }
    }

    private fun getRunningCascadeIdsFromCache(): Set<String> {
        val jsonStr = prefs?.cachedConversationsJson ?: return emptySet()
        return try {
            val items = JsonConfig.instance.decodeFromString<List<ConversationItem>>(jsonStr)
            items.filter { it.status.isRunning || it.status.needsAction }.map { it.id }.toSet()
        } catch (_: Exception) {
            emptySet()
        }
    }

    /**
     * Clean up any zombie/orphaned Live Activity notifications for cascades that are not actively running.
     * Matches iOS ActivityManager.cleanUpOrphanedActivities behavior.
     */
    fun cleanUpOrphanedActivities(keepingCascadeId: String? = null, runningCascadeIds: Set<String>? = null) {
        val allowedRunningIds = mutableSetOf<String>()
        if (runningCascadeIds != null) {
            allowedRunningIds.addAll(runningCascadeIds)
        } else {
            allowedRunningIds.addAll(getRunningCascadeIdsFromCache())
        }
        if (keepingCascadeId != null) {
            allowedRunningIds.add(keepingCascadeId)
        }

        val allowedNotifIds = allowedRunningIds.map { resolveNotificationId(it) }.toSet()

        // Clean up internal tracking set
        activeCascadeIds.retainAll(allowedRunningIds)
        if (activeCascadeId != null && !allowedRunningIds.contains(activeCascadeId)) {
            activeCascadeId = activeCascadeIds.firstOrNull()
            isNotificationActive = activeCascadeId != null
        }

        // Clean up actual system notifications in Android notification drawer
        try {
            val activeNotifs = notificationManager.activeNotifications
            for (sbn in activeNotifs) {
                val isLiveActivityChannel = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
                    sbn.notification.channelId == NotificationChannelManager.CHANNEL_LIVE_ACTIVITY
                } else {
                    true
                }
                if (isLiveActivityChannel && !allowedNotifIds.contains(sbn.id)) {
                    dismissRunnables[sbn.id]?.let { handler.removeCallbacks(it) }
                    dismissRunnables.remove(sbn.id)
                    notificationManager.cancel(sbn.tag, sbn.id)
                }
            }
        } catch (_: Exception) {}
    }

    /**
     * Synchronize live activity notifications with the latest authoritative list of conversations.
     * Automatically dismisses notifications for any tasks that have finished or become idle.
     */
    fun syncWithConversations(conversations: List<ConversationItem>) {
        val runningIds = conversations
            .filter { it.status.isRunning || it.status.needsAction }
            .map { it.id }
            .toSet()
        cleanUpOrphanedActivities(runningCascadeIds = runningIds)
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

        activeCascadeId = cascadeId
        isNotificationActive = true
        activeCascadeIds.add(cascadeId)

        val notifId = resolveNotificationId(cascadeId)
        dismissRunnables[notifId]?.let { handler.removeCallbacks(it) }
        dismissRunnables.remove(notifId)

        // Clean up any orphaned activities from previous tasks that are no longer running
        cleanUpOrphanedActivities(keepingCascadeId = cascadeId)

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
        val targetCascadeId = cascadeId ?: activeCascadeId ?: return
        val notifId = resolveNotificationId(targetCascadeId)

        activeCascadeIds.remove(targetCascadeId)
        if (activeCascadeId == targetCascadeId) {
            activeCascadeId = activeCascadeIds.firstOrNull()
            isNotificationActive = activeCascadeId != null
        }

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

        if (activeCascadeId == null) {
            lastActionNotifiedForCascade = null
        }

        // Auto-dismiss the live progress bar after 4 seconds (parity with iOS Live Activity dismissal)
        dismissRunnables[notifId]?.let { handler.removeCallbacks(it) }
        val run = Runnable {
            try {
                notificationManager.cancel(notifId)
            } catch (_: Exception) {}
            dismissRunnables.remove(notifId)
        }
        dismissRunnables[notifId] = run
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

    fun cancelActivity(cascadeId: String? = null) {
        if (cascadeId != null) {
            activeCascadeIds.remove(cascadeId)
            if (activeCascadeId == cascadeId) {
                activeCascadeId = activeCascadeIds.firstOrNull()
                isNotificationActive = activeCascadeId != null
            }
            val notifId = resolveNotificationId(cascadeId)
            dismissRunnables[notifId]?.let { handler.removeCallbacks(it) }
            dismissRunnables.remove(notifId)
            try {
                notificationManager.cancel(notifId)
            } catch (_: Exception) {}
        } else {
            for (cid in activeCascadeIds.toList()) {
                val nid = resolveNotificationId(cid)
                dismissRunnables[nid]?.let { handler.removeCallbacks(it) }
                try {
                    notificationManager.cancel(nid)
                } catch (_: Exception) {}
            }
            activeCascadeIds.clear()
            dismissRunnables.clear()
            activeCascadeId = null
            isNotificationActive = false
            lastActionNotifiedForCascade = null
            try {
                val activeNotifs = notificationManager.activeNotifications
                for (sbn in activeNotifs) {
                    val isLiveActivityChannel = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
                        sbn.notification.channelId == NotificationChannelManager.CHANNEL_LIVE_ACTIVITY
                    } else {
                        true
                    }
                    if (isLiveActivityChannel) {
                        notificationManager.cancel(sbn.tag, sbn.id)
                    }
                }
            } catch (_: Exception) {}
        }
    }

    companion object {
        const val CHANNEL_ID = NotificationChannelManager.CHANNEL_LIVE_ACTIVITY
        const val NOTIFICATION_ID = 1001
    }
}
