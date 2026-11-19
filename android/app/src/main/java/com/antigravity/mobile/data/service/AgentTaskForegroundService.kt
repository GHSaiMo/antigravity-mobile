package com.antigravity.mobile.data.service

import android.app.Notification
import android.app.Service
import android.content.Context
import android.content.Intent
import android.content.pm.ServiceInfo
import android.os.Build
import android.os.IBinder
import android.util.Log
import androidx.core.app.NotificationCompat
import androidx.core.app.ServiceCompat
import androidx.core.content.ContextCompat
import com.antigravity.mobile.R
import com.antigravity.mobile.ui.util.loc
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch

/**
 * Data-sync Foreground Service that keeps active Agent task status synchronization alive
 * while the app is in the background or screen is locked, preventing OEM/Android process
 * freezing from leaving the "running" notification stuck after a task finishes.
 *
 * Binds directly to the active task's notification ID (zero duplicate notifications) and
 * immediately detaches + stops itself as soon as all active tasks reach a terminal state.
 */
class AgentTaskForegroundService : Service() {

    private val serviceScope = CoroutineScope(Dispatchers.IO + SupervisorJob())
    private var watcherJob: Job? = null

    @Volatile
    private var isInForeground: Boolean = false

    override fun onCreate() {
        super.onCreate()
        instance = this
        NotificationChannelManager.createNotificationChannels(applicationContext)
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        val action = intent?.action ?: ACTION_START_OR_UPDATE
        if (action == ACTION_STOP) {
            stopForegroundAndSelf(removeNotification = false)
            return START_NOT_STICKY
        }

        val manager = LiveActivityNotificationManager.getOrCreate(applicationContext)
        val requestedCascadeId = intent?.getStringExtra(EXTRA_CASCADE_ID)
        val binding = manager.resolveForegroundBinding(preferredCascadeId = requestedCascadeId)

        if (binding == null) {
            // Fast-task race condition: task already completed between startForegroundService()
            // and onStartCommand(). Always satisfy Android's startForeground() contract before
            // stopping to prevent ForegroundServiceDidNotStartInTimeException.
            val transientNotification = buildTransientNotification()
            val promoted = promoteToForegroundInternal(
                cascadeId = null,
                notifId = TRANSIENT_NOTIF_ID,
                notification = transientNotification
            )
            stopForegroundAndSelf(removeNotification = promoted)
            return START_NOT_STICKY
        }

        val promoted = promoteToForegroundInternal(
            cascadeId = binding.cascadeId,
            notifId = binding.notifId,
            notification = binding.notification
        )
        if (!promoted) {
            stopSelf()
            return START_NOT_STICKY
        }

        ensureWatcherRunning(manager)
        return START_NOT_STICKY
    }

    private fun promoteToForegroundInternal(
        cascadeId: String?,
        notifId: Int,
        notification: Notification
    ): Boolean {
        return try {
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
                ServiceCompat.startForeground(
                    this,
                    notifId,
                    notification,
                    ServiceInfo.FOREGROUND_SERVICE_TYPE_DATA_SYNC
                )
            } else {
                startForeground(notifId, notification)
            }
            isInForeground = true
            foregroundNotifId = notifId
            foregroundCascadeId = cascadeId
            true
        } catch (e: Exception) {
            Log.w(TAG, "Failed to promote AgentTaskForegroundService to foreground: ${e.message}")
            false
        }
    }

    private fun ensureWatcherRunning(manager: LiveActivityNotificationManager) {
        if (watcherJob?.isActive == true) return
        watcherJob = serviceScope.launch {
            val startedAtMs = System.currentTimeMillis()
            // Brief initial delay so a freshly posted command can reach the gateway first.
            delay(INITIAL_POLL_DELAY_MS)
            while (isActive) {
                if (!manager.hasActiveActivities()) {
                    stopForegroundAndSelf(removeNotification = false)
                    break
                }
                if (System.currentTimeMillis() - startedAtMs >= MAX_WATCHER_DURATION_MS) {
                    Log.w(TAG, "Background task watcher reached max safety duration; detaching foreground service")
                    stopForegroundAndSelf(removeNotification = false)
                    break
                }
                try {
                    manager.pollActiveActivitiesOnce()
                } catch (e: Exception) {
                    Log.w(TAG, "Background task status poll error: ${e.message}")
                }
                if (!manager.hasActiveActivities()) {
                    stopForegroundAndSelf(removeNotification = false)
                    break
                }
                delay(POLL_INTERVAL_MS)
            }
        }
    }

    internal fun rebindForegroundInternal(
        cascadeId: String,
        notifId: Int,
        notification: Notification
    ): Boolean {
        val promoted = promoteToForegroundInternal(cascadeId, notifId, notification)
        if (promoted) {
            LiveActivityNotificationManager.sharedInstanceOrNull()?.let { ensureWatcherRunning(it) }
        }
        return promoted
    }

    internal fun stopForegroundAndSelf(removeNotification: Boolean) {
        watcherJob?.cancel()
        watcherJob = null
        if (isInForeground) {
            try {
                val flags = if (removeNotification) {
                    ServiceCompat.STOP_FOREGROUND_REMOVE
                } else {
                    ServiceCompat.STOP_FOREGROUND_DETACH
                }
                ServiceCompat.stopForeground(this, flags)
            } catch (e: Exception) {
                Log.w(TAG, "stopForeground failed: ${e.message}")
            }
            isInForeground = false
        }
        foregroundNotifId = 0
        foregroundCascadeId = null
        try {
            stopSelf()
        } catch (_: Exception) {}
    }

    private fun buildTransientNotification(): Notification {
        return NotificationCompat.Builder(applicationContext, NotificationChannelManager.CHANNEL_LIVE_ACTIVITY)
            .setContentTitle("正在同步任务状态".loc)
            .setContentText("正在检查任务执行结果...".loc)
            .setSubText("Antigravity Agent")
            .setSmallIcon(R.drawable.ic_stat_antigravity)
            .setOnlyAlertOnce(true)
            .setPriority(NotificationCompat.PRIORITY_LOW)
            .build()
    }

    override fun onDestroy() {
        watcherJob?.cancel()
        watcherJob = null
        serviceScope.cancel()
        isInForeground = false
        foregroundNotifId = 0
        foregroundCascadeId = null
        if (instance === this) {
            instance = null
        }
        super.onDestroy()
    }

    override fun onBind(intent: Intent?): IBinder? = null

    companion object {
        private const val TAG = "AgentTaskFGS"
        const val ACTION_START_OR_UPDATE = "com.antigravity.mobile.action.START_OR_UPDATE_TASK_SYNC"
        const val ACTION_STOP = "com.antigravity.mobile.action.STOP_TASK_SYNC"
        const val EXTRA_CASCADE_ID = "extra_cascade_id"

        private const val TRANSIENT_NOTIF_ID = 1099
        private const val INITIAL_POLL_DELAY_MS = 2_000L
        private const val POLL_INTERVAL_MS = 3_000L
        // Guard well below Android 15's 6h/24h dataSync FGS cumulative limit.
        private const val MAX_WATCHER_DURATION_MS = 2L * 60L * 60L * 1000L

        @Volatile
        private var instance: AgentTaskForegroundService? = null

        @Volatile
        private var foregroundNotifId: Int = 0

        @Volatile
        private var foregroundCascadeId: String? = null

        val isRunning: Boolean
            get() = instance?.isInForeground == true

        fun isForegroundFor(notifId: Int): Boolean {
            return instance?.isInForeground == true && foregroundNotifId == notifId
        }

        /**
         * Ensures the foreground service is running and watching active tasks.
         * Returns true if the service is active or successfully requested to start.
         */
        fun startOrEnsure(
            context: Context,
            cascadeId: String,
            notifId: Int,
            notification: Notification
        ): Boolean {
            val runningService = instance
            if (runningService != null) {
                if (!runningService.isInForeground) {
                    return runningService.rebindForegroundInternal(cascadeId, notifId, notification)
                }
                LiveActivityNotificationManager.sharedInstanceOrNull()?.let {
                    runningService.ensureWatcherRunning(it)
                }
                return true
            }

            val appContext = context.applicationContext
            val intent = Intent(appContext, AgentTaskForegroundService::class.java).apply {
                action = ACTION_START_OR_UPDATE
                putExtra(EXTRA_CASCADE_ID, cascadeId)
            }
            return try {
                ContextCompat.startForegroundService(appContext, intent)
                true
            } catch (e: Exception) {
                // On Android 12+, starting a foreground service strictly from background without
                // an exemption throws ForegroundServiceStartNotAllowedException. Fall back cleanly
                // without crashing.
                Log.w(TAG, "Unable to start foreground service right now: ${e.message}")
                false
            }
        }

        fun rebindForeground(
            cascadeId: String,
            notifId: Int,
            notification: Notification
        ): Boolean {
            val runningService = instance ?: return false
            return runningService.rebindForegroundInternal(cascadeId, notifId, notification)
        }

        fun detachAndStop(context: Context, removeNotification: Boolean = false) {
            val runningService = instance
            if (runningService != null) {
                runningService.stopForegroundAndSelf(removeNotification = removeNotification)
                return
            }
            // If service is not currently instantiated, nothing to detach synchronously.
            // (If a startForegroundService intent is still in flight, onStartCommand checks
            // hasActiveActivities() and will transiently promote + immediately stop itself.)
        }
    }
}
