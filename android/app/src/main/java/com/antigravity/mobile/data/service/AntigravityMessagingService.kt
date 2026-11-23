package com.antigravity.mobile.data.service

import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.net.Uri
import android.util.Log
import androidx.core.app.NotificationCompat
import androidx.core.app.NotificationManagerCompat
import com.antigravity.mobile.MainActivity
import com.antigravity.mobile.R
import com.antigravity.mobile.ui.util.loc
import com.google.firebase.messaging.FirebaseMessagingService
import com.google.firebase.messaging.RemoteMessage
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.launch

class AntigravityMessagingService : FirebaseMessagingService() {
    private val scope = CoroutineScope(Dispatchers.IO + SupervisorJob())

    override fun onNewToken(token: String) {
        super.onNewToken(token)
        val prefs = PreferencesManager(applicationContext)
        prefs.fcmPushToken = token

        if (prefs.isPaired()) {
            val connectionManager = ConnectionManager(applicationContext)
            val apiClient = ApiClient(applicationContext, prefs, connectionManager)
            scope.launch {
                apiClient.registerPushToken(token)
            }
        }
    }

    override fun onMessageReceived(remoteMessage: RemoteMessage) {
        super.onMessageReceived(remoteMessage)

        val data = remoteMessage.data
        val pushAction = data["action"] ?: ""
        val category = data["category"] ?: ""
        val deeplink = data["deeplink"]?.takeIf { it.isNotBlank() }
            ?: data["url"]?.takeIf { it.isNotBlank() }
            ?: if (category.equals("quota_alert", ignoreCase = true)) "antigravity://quota" else "antigravity://cascade/"
        val cascadeId = extractCascadeId(data, deeplink)

        if (pushAction.equals("dismiss_approval", ignoreCase = true) ||
            category.equals("dismiss_approval", ignoreCase = true)
        ) {
            if (!cascadeId.isNullOrBlank()) {
                dismissAlertNotification(applicationContext, cascadeId)
            }
            return
        }

        val prefs = PreferencesManager(applicationContext)
        if (!prefs.enableLiveNotifications) return

        NotificationChannelManager.createNotificationChannels(applicationContext)

        val title = remoteMessage.notification?.title ?: data["title"] ?: "Antigravity 任务通知"
        val body = remoteMessage.notification?.body ?: data["body"] ?: ""
        val level = data["level"] ?: "active"

        val isAction = level == "timeSensitive" || category.contains("action") || category.contains("proceed") || category.contains("error")
        val channelId = if (isAction) NotificationChannelManager.CHANNEL_ALERTS else NotificationChannelManager.CHANNEL_LIVE_ACTIVITY

        val intent = Intent(applicationContext, MainActivity::class.java).apply {
            action = Intent.ACTION_VIEW
            setData(Uri.parse(deeplink))
            flags = Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TOP
        }

        val notifId = if (!cascadeId.isNullOrBlank()) {
            alertNotificationIdForCascade(cascadeId)
        } else {
            (deeplink.hashCode() and 0x7FFFFFFF)
        }
        val pendingIntent = PendingIntent.getActivity(
            applicationContext,
            notifId,
            intent,
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        )

        val lowerCat = category.lowercase()
        val isHighRiskApproval = lowerCat == "antigravity_action_high"
        val isStandardApproval = lowerCat == "antigravity_action" || lowerCat == "antigravity_proceed"

        val publicNotif = NotificationCompat.Builder(applicationContext, channelId)
            .setContentTitle("Multigravity")
            .setContentText(if (isAction) "有任务等待确认".loc else "有新的任务进展通知".loc)
            .setSmallIcon(R.drawable.ic_stat_antigravity)
            .build()

        val accentColor = when {
            isHighRiskApproval -> 0xFFEF4444.toInt()
            isAction -> 0xFFF59E0B.toInt()
            else -> 0xFF4F46E5.toInt()
        }

        val builder = NotificationCompat.Builder(applicationContext, channelId)
            .setContentTitle(title)
            .setContentText(LiveActivityNotificationManager.redactNotificationSecrets(body))
            .setSmallIcon(R.drawable.ic_stat_antigravity)
            .setColor(accentColor)
            .setContentIntent(pendingIntent)
            .setAutoCancel(true)
            .setVisibility(NotificationCompat.VISIBILITY_PRIVATE)
            .setPublicVersion(publicNotif)
            .setPriority(if (isAction) NotificationCompat.PRIORITY_HIGH else NotificationCompat.PRIORITY_DEFAULT)
            .setCategory(if (isAction) NotificationCompat.CATEGORY_ALARM else NotificationCompat.CATEGORY_EVENT)
            .setDefaults(NotificationCompat.DEFAULT_ALL)

        val isTerminalPush = lowerCat.contains("complete") || lowerCat.contains("done") ||
            lowerCat.contains("finish") || lowerCat.contains("error") || lowerCat.contains("fail") ||
            lowerCat.contains("proceed")
        if (isTerminalPush && !cascadeId.isNullOrBlank()) {
            // Clear any stale local "running" live notification / foreground service when a terminal push arrives
            LiveActivityNotificationManager.sharedInstanceOrNull()?.cancelActivity(cascadeId)
        }
        if (!cascadeId.isNullOrBlank()) {
            if (isStandardApproval) {
                builder.addAction(
                    0,
                    "允许".loc,
                    NotificationActionReceiver.buildPendingIntent(
                        applicationContext,
                        NotificationActionReceiver.ACTION_APPROVE,
                        cascadeId,
                        alertNotifId = notifId
                    )
                )
                builder.addAction(
                    0,
                    "拒绝".loc,
                    NotificationActionReceiver.buildPendingIntent(
                        applicationContext,
                        NotificationActionReceiver.ACTION_REJECT,
                        cascadeId,
                        alertNotifId = notifId
                    )
                )
            } else if (isHighRiskApproval) {
                // High-risk approvals never expose a blind lock-screen Allow button; open App to review first.
                builder.addAction(0, "打开核验".loc, pendingIntent)
                builder.addAction(
                    0,
                    "拒绝".loc,
                    NotificationActionReceiver.buildPendingIntent(
                        applicationContext,
                        NotificationActionReceiver.ACTION_REJECT,
                        cascadeId,
                        alertNotifId = notifId
                    )
                )
            }
        }

        try {
            NotificationManagerCompat.from(applicationContext).notify(notifId, builder.build())
        } catch (e: SecurityException) {
            Log.w("FCM", "Cannot post notification: permission not granted: ${e.message}")
        }
    }

    companion object {
        fun alertNotificationIdForCascade(cascadeId: String): Int {
            return ("cascade_alert:${cascadeId.trim()}".hashCode() and 0x7FFFFFFF)
        }

        fun extractCascadeId(data: Map<String, String>, deeplink: String): String? {
            return data["cascadeId"]?.trim()?.takeIf { it.isNotBlank() }
                ?: data["cascade_id"]?.trim()?.takeIf { it.isNotBlank() }
                ?: runCatching { Uri.parse(deeplink).lastPathSegment }
                    .getOrNull()
                    ?.trim()
                    ?.takeIf { it.isNotBlank() && it != "cascade" }
        }

        fun dismissAlertNotification(context: Context, cascadeId: String) {
            val cid = cascadeId.trim()
            if (cid.isEmpty()) return
            val nm = NotificationManagerCompat.from(context.applicationContext)
            val ids = setOf(
                alertNotificationIdForCascade(cid),
                ("antigravity://cascade/$cid?action=review".hashCode() and 0x7FFFFFFF),
                ("antigravity://cascade/$cid".hashCode() and 0x7FFFFFFF)
            )
            ids.forEach { id ->
                try {
                    nm.cancel(id)
                } catch (_: Exception) {}
            }
        }
    }
}
