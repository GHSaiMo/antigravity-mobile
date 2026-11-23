package com.antigravity.mobile.data.service

import android.app.Notification
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.net.Uri
import android.os.Build
import android.os.Handler
import android.os.Looper
import android.util.Log
import androidx.core.app.NotificationCompat
import androidx.core.app.NotificationManagerCompat
import com.antigravity.mobile.MainActivity
import com.antigravity.mobile.R
import com.antigravity.mobile.data.model.ConversationItem
import com.antigravity.mobile.data.model.ConversationStatus
import com.antigravity.mobile.data.model.StreamUpdatePayload
import com.antigravity.mobile.data.model.blockingTasks
import com.antigravity.mobile.data.model.hasBlockingTasks
import com.antigravity.mobile.ui.util.ToolLocalization
import com.antigravity.mobile.ui.util.loc
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import kotlinx.serialization.encodeToString
import java.util.concurrent.ConcurrentHashMap

internal data class ForegroundNotificationBinding(
    val cascadeId: String,
    val notifId: Int,
    val notification: Notification
)

class LiveActivityNotificationManager(
    context: Context,
    private val prefs: PreferencesManager? = null,
    @Volatile private var apiClient: ApiClient? = null
) {
    private val context: Context = context.applicationContext
    private val notificationManager = this.context.getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
    private val handler = Handler(Looper.getMainLooper())
    private val scope = CoroutineScope(Dispatchers.IO + SupervisorJob())
    private val dismissRunnables = ConcurrentHashMap<Int, Runnable>()

    // Track active cascade IDs that currently have a running notification
    private val activeCascadeIds = ConcurrentHashMap.newKeySet<String>()
    private val cascadeTitles = ConcurrentHashMap<String, String>()
    private val activityStartedAtMs = ConcurrentHashMap<String, Long>()
    private val activityBaselineSteps = ConcurrentHashMap<String, Int>()
    private val observedServerRunningIds = ConcurrentHashMap.newKeySet<String>()
    private val lastBuiltNotifications = ConcurrentHashMap<String, Notification>()
    private val recentlyEndedAtMs = ConcurrentHashMap<String, Long>()
    private val consecutivePollFailures = ConcurrentHashMap<String, Int>()

    @Volatile private var activeCascadeId: String? = null
    @Volatile private var isNotificationActive: Boolean = false
    @Volatile private var lastActionNotifiedForCascade: String? = null
    private var fallbackWatcherJob: Job? = null

    /**
     * Optional callback invoked on Dispatchers.IO when the background status watcher
     * observes a tracked task reach a terminal state, so in-memory ViewModels can refresh.
     */
    @Volatile
    var onBackgroundTaskEnded: ((cascadeId: String, payload: StreamUpdatePayload) -> Unit)? = null

    init {
        NotificationChannelManager.createNotificationChannels(this.context)
        synchronized(companionLock) {
            if (sharedInstance == null || apiClient != null) {
                sharedInstance = this
            }
        }
        cleanUpOrphanedActivities()
    }

    fun attachApiClient(client: ApiClient) {
        apiClient = client
    }

    private fun resolveApiClient(): ApiClient? {
        apiClient?.let { return it }
        val p = prefs ?: PreferencesManager(context)
        if (!p.isPaired()) return null
        val connMgr = ConnectionManager(context)
        return ApiClient(context, p, connMgr).also { apiClient = it }
    }

    fun hasActiveActivities(): Boolean = activeCascadeIds.isNotEmpty()

    private fun resolveNotificationId(cascadeId: String): Int {
        val hash = cascadeId.hashCode() and 0x7FFFFFFF
        return if (hash == 0) 1001 else hash
    }

    private fun resolveAlertNotificationId(cascadeId: String): Int {
        val hash = (cascadeId + "_alert").hashCode() and 0x7FFFFFFF
        return if (hash == 0) 2001 else hash
    }

    private fun hasSystemNotification(notifId: Int): Boolean {
        return try {
            notificationManager.activeNotifications.any { it.id == notifId }
        } catch (_: Exception) {
            false
        }
    }

    fun hasNotification(cascadeId: String): Boolean {
        val notifId = resolveNotificationId(cascadeId)
        if (activeCascadeIds.contains(cascadeId)) return true
        return hasSystemNotification(notifId)
    }

    internal fun resolveForegroundBinding(preferredCascadeId: String? = null): ForegroundNotificationBinding? {
        val currentActive = activeCascadeId
        val targetCid = when {
            !preferredCascadeId.isNullOrBlank() && activeCascadeIds.contains(preferredCascadeId) -> preferredCascadeId
            currentActive != null && activeCascadeIds.contains(currentActive) -> currentActive
            else -> activeCascadeIds.firstOrNull()
        } ?: return null

        val notifId = resolveNotificationId(targetCid)
        val notif = lastBuiltNotifications[targetCid] ?: buildOngoingNotification(
            title = cascadeTitles[targetCid].orEmpty(),
            cascadeId = targetCid,
            status = "RUNNING",
            stepCount = 1,
            latestAction = "正在执行任务...",
            runningTaskCount = 0,
            hasPendingAction = false
        ).also { lastBuiltNotifications[targetCid] = it }

        return ForegroundNotificationBinding(
            cascadeId = targetCid,
            notifId = notifId,
            notification = notif
        )
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

    private fun updateCachedConversationStatus(cascadeId: String, finalStatus: String) {
        val p = prefs ?: return
        scope.launch {
            try {
                val jsonStr = p.cachedConversationsJson ?: return@launch
                val items = JsonConfig.instance.decodeFromString<List<ConversationItem>>(jsonStr)
                val targetStatus = when (finalStatus) {
                    "FAILED" -> ConversationStatus.ERROR
                    "PROCEED" -> ConversationStatus.ACTION
                    else -> ConversationStatus.IDLE
                }
                var changed = false
                val updated = items.map { item ->
                    if (item.id == cascadeId && item.status != targetStatus) {
                        changed = true
                        item.copy(status = targetStatus)
                    } else {
                        item
                    }
                }
                if (changed) {
                    p.cachedConversationsJson = JsonConfig.instance.encodeToString(updated)
                }
            } catch (_: Exception) {}
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
            // Keep currently tracked active tasks when running without an authoritative server list,
            // so newly started tasks whose status hasn't persisted to cache yet are never wiped out.
            allowedRunningIds.addAll(activeCascadeIds)
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

        if (activeCascadeIds.isEmpty()) {
            stopFallbackWatcher()
            AgentTaskForegroundService.detachAndStop(context, removeNotification = false)
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
                // Preserve 4-second completion summary cards and any active foreground-bound notification.
                val isPendingDismissCard = dismissRunnables.containsKey(sbn.id)
                val isForegroundPinned = AgentTaskForegroundService.isForegroundFor(sbn.id)
                if (isLiveActivityChannel && !allowedNotifIds.contains(sbn.id) && !isPendingDismissCard && !isForegroundPinned) {
                    dismissRunnables[sbn.id]?.let { handler.removeCallbacks(it) }
                    dismissRunnables.remove(sbn.id)
                    notificationManager.cancel(sbn.tag, sbn.id)
                }
            }
        } catch (_: Exception) {}
    }

    /**
     * Synchronize live activity notifications with the latest authoritative list of conversations.
     * Transitions finished tasks cleanly via [endActivity] instead of silently cancelling them.
     */
    fun syncWithConversations(conversations: List<ConversationItem>) {
        val convById = conversations.associateBy { it.id }
        val nowMs = System.currentTimeMillis()

        for (cid in activeCascadeIds.toList()) {
            val item = convById[cid] ?: continue
            if (item.title.isNotBlank()) {
                cascadeTitles[cid] = item.title
            }
            if (item.status.isRunning) {
                observedServerRunningIds.add(cid)
            } else if (!item.status.needsAction) {
                val startedAt = activityStartedAtMs[cid] ?: 0L
                val baselineSteps = activityBaselineSteps[cid] ?: -1
                val shouldFinish = LiveActivityStatusHelper.shouldFinishTrackedActivity(
                    statusRunning = false,
                    isError = item.status.isError,
                    canProceed = false,
                    hasObservedServerRunning = observedServerRunningIds.contains(cid),
                    baselineStepCount = baselineSteps,
                    currentStepCount = item.stepCount,
                    elapsedSinceStartMs = if (startedAt > 0L) (nowMs - startedAt) else Long.MAX_VALUE
                )
                if (shouldFinish) {
                    val finalStatus = if (item.status.isError) "FAILED" else "COMPLETED"
                    endActivity(cascadeId = cid, finalStatus = finalStatus, title = item.displayTitle)
                }
            }
        }

        val runningIds = conversations
            .filter { it.status.isRunning || it.status.needsAction }
            .map { it.id }
            .toMutableSet()
        // Keep still-grace-protected active tasks from being silently cancelled before server catches up.
        runningIds.addAll(activeCascadeIds)
        cleanUpOrphanedActivities(runningCascadeIds = runningIds)
    }

    private fun buildOngoingNotification(
        title: String,
        cascadeId: String,
        status: String,
        stepCount: Int,
        latestAction: String,
        runningTaskCount: Int,
        hasPendingAction: Boolean
    ): Notification {
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

        val resolvedTitle = title.ifBlank { cascadeTitles[cascadeId].orEmpty().ifBlank { "Agent 任务执行中".loc } }
        val actionText = when {
            latestAction.isNotBlank() -> latestAction.loc
            runningTaskCount > 0 -> "正在执行后台任务 ($runningTaskCount)...".loc
            stepCount > 0 -> "正在执行第 $stepCount 步...".loc
            else -> "正在执行...".loc
        }

        val redactedActionText = redactNotificationSecrets(actionText)
        val subText = if (hasPendingAction) "需要用户审批".loc else "Antigravity Agent"

        val publicOngoing = NotificationCompat.Builder(context, NotificationChannelManager.CHANNEL_LIVE_ACTIVITY)
            .setContentTitle("Multigravity")
            .setContentText(if (hasPendingAction) "需要用户审批".loc else "Agent 任务执行中".loc)
            .setSmallIcon(R.drawable.ic_stat_antigravity)
            .build()

        val builder = NotificationCompat.Builder(context, NotificationChannelManager.CHANNEL_LIVE_ACTIVITY)
            .setContentTitle(resolvedTitle)
            .setContentText(redactedActionText)
            .setSubText(subText)
            .setSmallIcon(R.drawable.ic_stat_antigravity)
            .setColor(0xFF4F46E5.toInt())
            .setContentIntent(pendingIntent)
            .setOngoing(true)
            .setOnlyAlertOnce(true)
            .setVisibility(NotificationCompat.VISIBILITY_PRIVATE)
            .setPublicVersion(publicOngoing)
            .setProgress(0, 0, true)
            .setCategory(NotificationCompat.CATEGORY_PROGRESS)
            .setPriority(NotificationCompat.PRIORITY_LOW)

        if (hasPendingAction) {
            builder.addAction(
                0,
                "允许".loc,
                NotificationActionReceiver.buildPendingIntent(
                    context,
                    NotificationActionReceiver.ACTION_APPROVE,
                    cascadeId
                )
            )
            builder.addAction(
                0,
                "拒绝".loc,
                NotificationActionReceiver.buildPendingIntent(
                    context,
                    NotificationActionReceiver.ACTION_REJECT,
                    cascadeId
                )
            )
        } else if (LiveActivityStatusHelper.isStatusRunning(status)) {
            builder.addAction(
                0,
                "停止".loc,
                NotificationActionReceiver.buildPendingIntent(
                    context,
                    NotificationActionReceiver.ACTION_STOP,
                    cascadeId
                )
            )
        }

        return builder.build()
    }

    fun startOrUpdateActivity(
        title: String,
        cascadeId: String,
        status: String = "RUNNING",
        stepCount: Int = 1,
        latestAction: String = "正在执行任务...",
        runningTaskCount: Int = 0,
        hasPendingAction: Boolean = false,
        isOptimisticStart: Boolean = false,
        baselineStepCount: Int = -1
    ) {
        if (cascadeId.isBlank() || cascadeId.startsWith("local_draft_") || cascadeId.startsWith("draft_")) {
            return
        }
        if (prefs?.enableLiveNotifications == false) {
            cancelActivity()
            return
        }

        recentlyEndedAtMs.remove(cascadeId)
        consecutivePollFailures.remove(cascadeId)

        val wasAlreadyTracked = activeCascadeIds.contains(cascadeId)
        activeCascadeId = cascadeId
        isNotificationActive = true
        activeCascadeIds.add(cascadeId)

        val cleanedTitle = title.trim().takeIf { it.isNotBlank() && it != "未命名会话" && it != "会话详情" }
        if (cleanedTitle != null) {
            cascadeTitles[cascadeId] = cleanedTitle
        }
        if (!wasAlreadyTracked) {
            activityStartedAtMs[cascadeId] = System.currentTimeMillis()
            if (baselineStepCount >= 0) {
                activityBaselineSteps[cascadeId] = baselineStepCount
            }
        } else {
            activityStartedAtMs.putIfAbsent(cascadeId, System.currentTimeMillis())
            if (baselineStepCount >= 0) {
                activityBaselineSteps.putIfAbsent(cascadeId, baselineStepCount)
            }
        }
        if (!isOptimisticStart && LiveActivityStatusHelper.isStatusRunning(status)) {
            observedServerRunningIds.add(cascadeId)
        }

        val notifId = resolveNotificationId(cascadeId)
        dismissRunnables[notifId]?.let { handler.removeCallbacks(it) }
        dismissRunnables.remove(notifId)

        val notification = buildOngoingNotification(
            title = cleanedTitle ?: cascadeTitles[cascadeId].orEmpty(),
            cascadeId = cascadeId,
            status = status,
            stepCount = stepCount,
            latestAction = latestAction,
            runningTaskCount = runningTaskCount,
            hasPendingAction = hasPendingAction
        )
        lastBuiltNotifications[cascadeId] = notification

        // 1. Update persistent Live Activity progress notification
        try {
            NotificationManagerCompat.from(context).notify(notifId, notification)
        } catch (_: SecurityException) {
            // Handled on Android 13+ when permission not yet granted
        }

        // 2. Keep active status sync alive in the background via our dataSync Foreground Service
        val fgsStarted = AgentTaskForegroundService.startOrEnsure(
            context = context,
            cascadeId = cascadeId,
            notifId = notifId,
            notification = notification
        )
        if (fgsStarted) {
            stopFallbackWatcher()
        } else {
            ensureFallbackWatcherRunning()
        }

        // Clean up any truly orphaned activities from previous dead tasks
        cleanUpOrphanedActivities(keepingCascadeId = cascadeId)

        // 3. If user interaction / approval is required, post a high-priority heads-up alert with sound & vibration
        val resolvedTitle = cleanedTitle ?: cascadeTitles[cascadeId].orEmpty().ifBlank { "Agent 任务执行中".loc }
        val actionText = when {
            latestAction.isNotBlank() -> latestAction.loc
            runningTaskCount > 0 -> "正在执行后台任务 ($runningTaskCount)...".loc
            stepCount > 0 -> "正在执行第 $stepCount 步...".loc
            else -> "正在执行...".loc
        }
        if (hasPendingAction && lastActionNotifiedForCascade != cascadeId) {
            lastActionNotifiedForCascade = cascadeId
            notifyAlert(
                title = "⚠️ Antigravity 需要审批".loc,
                body = "【$resolvedTitle】$actionText",
                deeplink = "antigravity://cascade/$cascadeId?action=review",
                isAction = true,
                cascadeId = cascadeId
            )
        } else if (!hasPendingAction && lastActionNotifiedForCascade == cascadeId) {
            lastActionNotifiedForCascade = null
            dismissAlert(cascadeId)
        }
    }

    fun dismissAlert(cascadeId: String) {
        if (cascadeId.isBlank()) return
        AntigravityMessagingService.dismissAlertNotification(context, cascadeId)
    }

    fun endActivity(
        cascadeId: String? = null,
        finalStatus: String = "COMPLETED",
        title: String? = null
    ) {
        val targetCascadeId = cascadeId ?: activeCascadeId ?: return
        val notifId = resolveNotificationId(targetCascadeId)

        val wasTracked = activeCascadeIds.remove(targetCascadeId)
        val hadSystemNotif = hasSystemNotification(notifId)
        val nowMs = System.currentTimeMillis()
        val lastEnded = recentlyEndedAtMs[targetCascadeId] ?: 0L
        if (!wasTracked && !hadSystemNotif && (nowMs - lastEnded) < 5_000L) {
            return
        }
        recentlyEndedAtMs[targetCascadeId] = nowMs

        observedServerRunningIds.remove(targetCascadeId)
        activityStartedAtMs.remove(targetCascadeId)
        activityBaselineSteps.remove(targetCascadeId)
        lastBuiltNotifications.remove(targetCascadeId)
        consecutivePollFailures.remove(targetCascadeId)
        val cachedTitle = cascadeTitles.remove(targetCascadeId).orEmpty()
        val resolvedTitle = title?.trim()?.takeIf { it.isNotBlank() && it != "未命名会话" && it != "会话详情" }
            ?: cachedTitle

        if (activeCascadeId == targetCascadeId) {
            activeCascadeId = activeCascadeIds.firstOrNull()
            isNotificationActive = activeCascadeId != null
        }

        // Release foreground service binding BEFORE posting the non-ongoing finished notification;
        // otherwise Android keeps FLAG_FOREGROUND_SERVICE / FLAG_ONGOING_EVENT locked on notifId.
        if (activeCascadeIds.isEmpty()) {
            stopFallbackWatcher()
            AgentTaskForegroundService.detachAndStop(context, removeNotification = false)
        } else if (AgentTaskForegroundService.isForegroundFor(notifId)) {
            val nextBinding = resolveForegroundBinding()
            if (nextBinding != null) {
                AgentTaskForegroundService.rebindForeground(
                    cascadeId = nextBinding.cascadeId,
                    notifId = nextBinding.notifId,
                    notification = nextBinding.notification
                )
            } else {
                AgentTaskForegroundService.detachAndStop(context, removeNotification = false)
            }
        }

        updateCachedConversationStatus(targetCascadeId, finalStatus)

        val summary = LiveActivityStatusHelper.formatEndedSummary(finalStatus)
        val contentTitle = if (resolvedTitle.isNotBlank()) resolvedTitle else "任务已结束".loc
        val intent = Intent(context, MainActivity::class.java).apply {
            action = Intent.ACTION_VIEW
            data = Uri.parse("antigravity://cascade/$targetCascadeId")
            flags = Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TOP
        }
        val pendingIntent = PendingIntent.getActivity(
            context,
            notifId,
            intent,
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        )

        val accentColor = when (finalStatus) {
            "COMPLETED" -> 0xFF10B981.toInt()
            "PROCEED" -> 0xFFF59E0B.toInt()
            "FAILED" -> 0xFFEF4444.toInt()
            else -> 0xFF6B7280.toInt()
        }

        val builder = NotificationCompat.Builder(context, NotificationChannelManager.CHANNEL_LIVE_ACTIVITY)
            .setContentTitle(contentTitle)
            .setContentText(redactNotificationSecrets(summary))
            .setSubText("Antigravity Agent")
            .setSmallIcon(R.drawable.ic_stat_antigravity)
            .setColor(accentColor)
            .setContentIntent(pendingIntent)
            .setOngoing(false)
            .setAutoCancel(true)
            .setVisibility(NotificationCompat.VISIBILITY_PRIVATE)
            .setProgress(0, 0, false)
            .setOnlyAlertOnce(true)
            .setPriority(NotificationCompat.PRIORITY_LOW)

        try {
            NotificationManagerCompat.from(context).notify(notifId, builder.build())
        } catch (_: SecurityException) {}

        // Post a persistent completion / approval / failure alert to the ALERTS channel so the user is informed
        if (prefs?.enableLiveNotifications != false) {
            LiveActivityStatusHelper.formatCompletionAlert(finalStatus, resolvedTitle)?.let { (alertTitle, alertBody) ->
                notifyAlert(
                    title = alertTitle,
                    body = alertBody,
                    deeplink = "antigravity://cascade/$targetCascadeId",
                    isAction = finalStatus == "PROCEED",
                    cascadeId = targetCascadeId
                )
            }
        }

        if (activeCascadeId == null || lastActionNotifiedForCascade == targetCascadeId) {
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

    /**
     * Polls every currently active tracked cascade once via HTTP and synchronizes its notification
     * state (updating progress/approval or transitioning to completed/failed).
     */
    suspend fun pollActiveActivitiesOnce() {
        val ids = activeCascadeIds.toList()
        if (ids.isEmpty()) {
            stopFallbackWatcher()
            AgentTaskForegroundService.detachAndStop(context, removeNotification = false)
            return
        }

        val client = resolveApiClient() ?: return
        val nowMs = System.currentTimeMillis()

        for (cid in ids) {
            if (!activeCascadeIds.contains(cid)) continue
            client.fetchMessages(cid, limit = 5)
                .onSuccess { payload ->
                    consecutivePollFailures.remove(cid)
                    if (!activeCascadeIds.contains(cid)) return@onSuccess

                    val statusRunning = LiveActivityStatusHelper.isStatusRunning(payload.status)
                    val msgs = payload.messages.orEmpty()
                    val lastMsg = msgs.lastOrNull()
                    val isError = lastMsg?.status.equals("error", ignoreCase = true) ||
                        payload.hasError ||
                        LiveActivityStatusHelper.isStatusError(payload.status)
                    val resolvedTitle = payload.title?.takeIf {
                        it.isNotBlank() && it != "未命名会话" && it != "会话详情"
                    } ?: cascadeTitles[cid].orEmpty()

                    if (statusRunning) {
                        observedServerRunningIds.add(cid)
                        val runningTasks = payload.runningTasks.orEmpty()
                        val latestAction = when {
                            payload.pendingInteraction != null -> payload.pendingInteraction.prompt ?: "需要审批操作".loc
                            runningTasks.hasBlockingTasks -> runningTasks.blockingTasks.firstOrNull()?.displayCommand?.ifBlank { "正在执行后台任务...".loc } ?: "正在执行后台任务...".loc
                            lastMsg?.toolCalls?.isNotEmpty() == true -> "正在执行: ".loc + ToolLocalization.localizedName(lastMsg.toolCalls.lastOrNull()?.name ?: "操作")
                            else -> "正在执行任务...".loc
                        }
                        val stepCount = maxOf(1, payload.totalSteps, msgs.count { !it.isUser })
                        startOrUpdateActivity(
                            title = resolvedTitle,
                            cascadeId = cid,
                            status = payload.status,
                            stepCount = stepCount,
                            latestAction = latestAction,
                            runningTaskCount = runningTasks.blockingTasks.size,
                            hasPendingAction = payload.pendingInteraction != null,
                            isOptimisticStart = false
                        )
                    } else {
                        val startedAt = activityStartedAtMs[cid] ?: 0L
                        val baselineSteps = activityBaselineSteps[cid] ?: -1
                        val currentSteps = maxOf(payload.totalSteps, msgs.size)
                        val shouldFinish = LiveActivityStatusHelper.shouldFinishTrackedActivity(
                            statusRunning = false,
                            isError = isError,
                            canProceed = payload.canProceed,
                            hasObservedServerRunning = observedServerRunningIds.contains(cid),
                            baselineStepCount = baselineSteps,
                            currentStepCount = currentSteps,
                            elapsedSinceStartMs = if (startedAt > 0L) (nowMs - startedAt) else Long.MAX_VALUE
                        )
                        if (shouldFinish) {
                            val finalStatus = LiveActivityStatusHelper.resolveFinalStatus(
                                isError = isError,
                                canProceed = payload.canProceed
                            )
                            endActivity(
                                cascadeId = cid,
                                finalStatus = finalStatus,
                                title = resolvedTitle
                            )
                            onBackgroundTaskEnded?.invoke(cid, payload)
                        }
                    }
                }
                .onFailure { err ->
                    val failCount = (consecutivePollFailures[cid] ?: 0) + 1
                    consecutivePollFailures[cid] = failCount
                    if (failCount == 1 || failCount % 10 == 0) {
                        Log.w(TAG, "Background poll failed for $cid (#$failCount): ${err.message}")
                    }
                    if (failCount >= MAX_CONSECUTIVE_POLL_FAILURES) {
                        Log.w(TAG, "Giving up background poll for unreachable task $cid")
                        cancelActivity(cid)
                    }
                }
        }
    }

    /**
     * Triggers an immediate asynchronous status check for all active live activities
     * (e.g., when MainActivity returns to foreground).
     */
    fun refreshActiveActivitiesNow() {
        if (activeCascadeIds.isEmpty()) return
        scope.launch {
            try {
                pollActiveActivitiesOnce()
            } catch (e: Exception) {
                Log.w(TAG, "Immediate active activity refresh failed: ${e.message}")
            }
        }
    }

    private fun ensureFallbackWatcherRunning() {
        if (fallbackWatcherJob?.isActive == true) return
        fallbackWatcherJob = scope.launch {
            delay(2_000L)
            while (isActive && activeCascadeIds.isNotEmpty() && !AgentTaskForegroundService.isRunning) {
                try {
                    pollActiveActivitiesOnce()
                } catch (_: Exception) {}
                if (activeCascadeIds.isEmpty() || AgentTaskForegroundService.isRunning) break
                delay(3_000L)
            }
        }
    }

    private fun stopFallbackWatcher() {
        fallbackWatcherJob?.cancel()
        fallbackWatcherJob = null
    }

    fun notifyAlert(
        title: String,
        body: String,
        deeplink: String,
        isAction: Boolean = false,
        cascadeId: String? = null
    ) {
        if (prefs?.enableLiveNotifications == false) return

        val intent = Intent(context, MainActivity::class.java).apply {
            action = Intent.ACTION_VIEW
            data = Uri.parse(deeplink)
            flags = Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TOP
        }

        val alertNotifId = if (!cascadeId.isNullOrBlank()) {
            AntigravityMessagingService.alertNotificationIdForCascade(cascadeId)
        } else {
            (deeplink.hashCode() and 0x7FFFFFFF)
        }

        val pendingIntent = PendingIntent.getActivity(
            context,
            alertNotifId,
            intent,
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        )

        val publicAlert = NotificationCompat.Builder(context, NotificationChannelManager.CHANNEL_ALERTS)
            .setContentTitle("Multigravity")
            .setContentText(if (isAction) "需要用户审批".loc else "有新的任务进展通知".loc)
            .setSmallIcon(R.drawable.ic_stat_antigravity)
            .build()

        val alertBuilder = NotificationCompat.Builder(context, NotificationChannelManager.CHANNEL_ALERTS)
            .setContentTitle(title)
            .setContentText(redactNotificationSecrets(body))
            .setSmallIcon(R.drawable.ic_stat_antigravity)
            .setColor(if (isAction) 0xFFF59E0B.toInt() else 0xFF4F46E5.toInt())
            .setContentIntent(pendingIntent)
            .setAutoCancel(true)
            .setVisibility(NotificationCompat.VISIBILITY_PRIVATE)
            .setPublicVersion(publicAlert)
            .setPriority(NotificationCompat.PRIORITY_HIGH)
            .setCategory(if (isAction) NotificationCompat.CATEGORY_ALARM else NotificationCompat.CATEGORY_EVENT)
            .setDefaults(NotificationCompat.DEFAULT_ALL)

        if (isAction && !cascadeId.isNullOrBlank()) {
            alertBuilder.addAction(
                0,
                "允许".loc,
                NotificationActionReceiver.buildPendingIntent(
                    context,
                    NotificationActionReceiver.ACTION_APPROVE,
                    cascadeId,
                    alertNotifId = alertNotifId
                )
            )
            alertBuilder.addAction(
                0,
                "拒绝".loc,
                NotificationActionReceiver.buildPendingIntent(
                    context,
                    NotificationActionReceiver.ACTION_REJECT,
                    cascadeId,
                    alertNotifId = alertNotifId
                )
            )
        }

        try {
            NotificationManagerCompat.from(context).notify(alertNotifId, alertBuilder.build())
        } catch (_: SecurityException) {}
    }

    fun cancelActivity(cascadeId: String? = null) {
        if (cascadeId != null) {
            activeCascadeIds.remove(cascadeId)
            cascadeTitles.remove(cascadeId)
            activityStartedAtMs.remove(cascadeId)
            activityBaselineSteps.remove(cascadeId)
            observedServerRunningIds.remove(cascadeId)
            lastBuiltNotifications.remove(cascadeId)
            consecutivePollFailures.remove(cascadeId)
            if (activeCascadeId == cascadeId) {
                activeCascadeId = activeCascadeIds.firstOrNull()
                isNotificationActive = activeCascadeId != null
            }
            val notifId = resolveNotificationId(cascadeId)
            if (activeCascadeIds.isEmpty()) {
                stopFallbackWatcher()
                AgentTaskForegroundService.detachAndStop(context, removeNotification = true)
            } else if (AgentTaskForegroundService.isForegroundFor(notifId)) {
                val nextBinding = resolveForegroundBinding()
                if (nextBinding != null) {
                    AgentTaskForegroundService.rebindForeground(
                        cascadeId = nextBinding.cascadeId,
                        notifId = nextBinding.notifId,
                        notification = nextBinding.notification
                    )
                } else {
                    AgentTaskForegroundService.detachAndStop(context, removeNotification = true)
                }
            }
            dismissRunnables[notifId]?.let { handler.removeCallbacks(it) }
            dismissRunnables.remove(notifId)
            try {
                notificationManager.cancel(notifId)
            } catch (_: Exception) {}
        } else {
            stopFallbackWatcher()
            AgentTaskForegroundService.detachAndStop(context, removeNotification = true)
            for (cid in activeCascadeIds.toList()) {
                val nid = resolveNotificationId(cid)
                dismissRunnables[nid]?.let { handler.removeCallbacks(it) }
                try {
                    notificationManager.cancel(nid)
                } catch (_: Exception) {}
            }
            activeCascadeIds.clear()
            cascadeTitles.clear()
            activityStartedAtMs.clear()
            activityBaselineSteps.clear()
            observedServerRunningIds.clear()
            lastBuiltNotifications.clear()
            consecutivePollFailures.clear()
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
        private const val TAG = "LiveActivityNotif"
        private const val MAX_CONSECUTIVE_POLL_FAILURES = 60
        const val CHANNEL_ID = NotificationChannelManager.CHANNEL_LIVE_ACTIVITY
        const val NOTIFICATION_ID = 1001

        private val companionLock = Any()

        @Volatile
        private var sharedInstance: LiveActivityNotificationManager? = null

        private val BEARER_SECRET_REGEX = Regex("(?i)(Bearer\\s+)[A-Za-z0-9._\\-~+/=]+")
        private val KV_SECRET_REGEX = Regex("(?i)((?:token|api[_-]?key|password|passwd|secret|auth_token)=)[^\\s&\"']+")
        private val FLAG_SECRET_REGEX = Regex("(-w\\s+)[^\\s\"']+")

        fun redactNotificationSecrets(raw: String): String {
            if (raw.isEmpty()) return raw
            var out = BEARER_SECRET_REGEX.replace(raw, "$1***")
            out = KV_SECRET_REGEX.replace(out, "$1***")
            out = FLAG_SECRET_REGEX.replace(out, "$1***")
            return out
        }

        fun sharedInstanceOrNull(): LiveActivityNotificationManager? = sharedInstance

        fun getOrCreate(
            context: Context,
            prefs: PreferencesManager? = null,
            apiClient: ApiClient? = null
        ): LiveActivityNotificationManager {
            sharedInstance?.let { existing ->
                if (apiClient != null) {
                    existing.attachApiClient(apiClient)
                }
                return existing
            }
            return synchronized(companionLock) {
                sharedInstance?.let { existing ->
                    if (apiClient != null) {
                        existing.attachApiClient(apiClient)
                    }
                    existing
                } ?: LiveActivityNotificationManager(
                    context = context.applicationContext,
                    prefs = prefs ?: PreferencesManager(context.applicationContext),
                    apiClient = apiClient
                ).also { sharedInstance = it }
            }
        }
    }
}
