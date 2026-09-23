package com.antigravity.mobile.data.service

import android.app.PendingIntent
import android.content.Intent
import android.net.Uri
import android.util.Log
import androidx.core.app.NotificationCompat
import androidx.core.app.NotificationManagerCompat
import com.antigravity.mobile.MainActivity
import com.antigravity.mobile.R
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
        Log.d("FCM", "New FCM registration token received: $token")
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
        Log.d("FCM", "Remote message received: ${remoteMessage.data}")

        val prefs = PreferencesManager(applicationContext)
        if (!prefs.enableLiveNotifications) return

        NotificationChannelManager.createNotificationChannels(applicationContext)

        val data = remoteMessage.data
        val title = remoteMessage.notification?.title ?: data["title"] ?: "Antigravity 任务通知"
        val body = remoteMessage.notification?.body ?: data["body"] ?: ""
        val deeplink = data["deeplink"] ?: data["url"] ?: "antigravity://cascade/"
        val level = data["level"] ?: "active"
        val category = data["category"] ?: ""

        val isAction = level == "timeSensitive" || category.contains("action") || category.contains("proceed") || category.contains("error")
        val channelId = if (isAction) NotificationChannelManager.CHANNEL_ALERTS else NotificationChannelManager.CHANNEL_LIVE_ACTIVITY

        val intent = Intent(applicationContext, MainActivity::class.java).apply {
            action = Intent.ACTION_VIEW
            data = Uri.parse(deeplink)
            flags = Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TOP
        }

        val notifId = (deeplink.hashCode() and 0x7FFFFFFF)
        val pendingIntent = PendingIntent.getActivity(
            applicationContext,
            notifId,
            intent,
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        )

        val builder = NotificationCompat.Builder(applicationContext, channelId)
            .setContentTitle(title)
            .setContentText(body)
            .setSmallIcon(R.drawable.ic_stat_antigravity)
            .setColor(if (isAction) 0xFFF59E0B.toInt() else 0xFF4F46E5.toInt())
            .setContentIntent(pendingIntent)
            .setAutoCancel(true)
            .setPriority(if (isAction) NotificationCompat.PRIORITY_HIGH else NotificationCompat.PRIORITY_DEFAULT)
            .setCategory(if (isAction) NotificationCompat.CATEGORY_ALARM else NotificationCompat.CATEGORY_EVENT)
            .setDefaults(NotificationCompat.DEFAULT_ALL)

        try {
            NotificationManagerCompat.from(applicationContext).notify(notifId, builder.build())
        } catch (e: SecurityException) {
            Log.w("FCM", "Cannot post notification: permission not granted: ${e.message}")
        }
    }
}
