package com.antigravity.mobile.data.service

import android.app.NotificationChannel
import android.app.NotificationManager
import android.content.Context
import android.graphics.Color
import android.media.AudioAttributes
import android.media.RingtoneManager
import android.os.Build

object NotificationChannelManager {
    const val CHANNEL_LIVE_ACTIVITY = "antigravity_live_activity"
    const val CHANNEL_ALERTS = "antigravity_alerts"

    fun createNotificationChannels(context: Context) {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O) return

        val notificationManager = context.getSystemService(Context.NOTIFICATION_SERVICE) as? NotificationManager ?: return

        // 1. Live Activity Ongoing Channel (Low importance: silent progress in drawer/lockscreen)
        val liveChannel = NotificationChannel(
            CHANNEL_LIVE_ACTIVITY,
            "任务实时进展",
            NotificationManager.IMPORTANCE_LOW
        ).apply {
            description = "在锁屏与通知栏常驻展示 Agent 任务执行进度与后台指令状态"
            setShowBadge(false)
            enableLights(false)
            enableVibration(false)
        }

        // 2. High-Priority Alerts Channel (High importance: heads-up pop-up, sound, vibration for approvals & completions)
        val alertChannel = NotificationChannel(
            CHANNEL_ALERTS,
            "重要通知与审批",
            NotificationManager.IMPORTANCE_HIGH
        ).apply {
            description = "接收 Agent 用户审批申请、方案就绪提示、任务完成与异常终止等高优先级提醒"
            setShowBadge(true)
            enableLights(true)
            lightColor = Color.parseColor("#4F46E5")
            enableVibration(true)
            vibrationPattern = longArrayOf(0, 250, 150, 250)

            val soundUri = RingtoneManager.getDefaultUri(RingtoneManager.TYPE_NOTIFICATION)
            val audioAttributes = AudioAttributes.Builder()
                .setContentType(AudioAttributes.CONTENT_TYPE_SONIFICATION)
                .setUsage(AudioAttributes.USAGE_NOTIFICATION)
                .build()
            setSound(soundUri, audioAttributes)
        }

        notificationManager.createNotificationChannel(liveChannel)
        notificationManager.createNotificationChannel(alertChannel)
    }
}
