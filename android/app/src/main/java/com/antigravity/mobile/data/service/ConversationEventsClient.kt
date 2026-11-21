package com.antigravity.mobile.data.service

import android.util.Log
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asSharedFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.longOrNull
import java.util.concurrent.TimeUnit
import kotlin.random.Random

/** One frame of the gateway's `/gateway/events` channel. Frames carry a revision, never list data. */
internal data class ConversationEventFrame(
    val type: String,
    val rev: Long,
    val ts: Long? = null,
    /** Desktop language_server state from the gateway: ok / down / unknown. */
    val desktop: String? = null,
    /** Set on `hello` when `?sinceRev=` matches the gateway's current revision. */
    val unchanged: Boolean = false
)

internal fun parseConversationEventFrame(text: String): ConversationEventFrame? = try {
    val obj = Json.parseToJsonElement(text).jsonObject
    val type = obj["type"]?.jsonPrimitive?.content
    if (type == null) null else ConversationEventFrame(
        type,
        obj["rev"]?.jsonPrimitive?.longOrNull ?: 0L,
        obj["ts"]?.jsonPrimitive?.longOrNull,
        obj["desktop"]?.jsonPrimitive?.content,
        obj["unchanged"]?.jsonPrimitive?.booleanOrNull ?: false
    )
} catch (_: Exception) {
    null
}

/**
 * Listens for conversation-list change hints from the gateway so the list screen does not have to
 * poll every few seconds. Every `hello` (connect/reconnect) and `changed` frame emits on [changes];
 * the owner then refetches the list through the normal list API. [connected] tells the owner when
 * it may relax its own polling, and flips back to false as soon as the socket is lost.
 *
 * Gateways without `/gateway/events` simply never connect (the handshake fails and backs off), so
 * callers keep their existing polling in that case.
 */
class ConversationEventsClient(
    private val prefs: PreferencesManager,
    private val baseUrlProvider: () -> String?
) {
    private val client = OkHttpClient.Builder()
        .dns(CloudflareOptimizedDns)
        .pingInterval(15, TimeUnit.SECONDS)
        .readTimeout(0, TimeUnit.MILLISECONDS) // infinite for websockets
        .addInterceptor(LanCleartextSecurityInterceptor())
        .build()

    private val scope = CoroutineScope(Dispatchers.IO + SupervisorJob())
    private var runJob: Job? = null
    private var webSocket: WebSocket? = null
    @Volatile private var lastRev: Long = 0L
    @Volatile private var lastHost: String? = null

    private val _connected = MutableStateFlow(false)
    val connected: StateFlow<Boolean> = _connected.asStateFlow()

    private val _changes = MutableSharedFlow<Unit>(extraBufferCapacity = 1, onBufferOverflow = kotlinx.coroutines.channels.BufferOverflow.DROP_OLDEST)
    val changes: SharedFlow<Unit> = _changes.asSharedFlow()

    private val _snapshot = MutableStateFlow(LinkSnapshot())
    /** Heartbeat facts for the connection dot in the home quota bar. */
    val snapshot: StateFlow<LinkSnapshot> = _snapshot.asStateFlow()

    /** Drops the current socket (if any) and reconnects immediately. */
    fun reconnect() {
        if (!_snapshot.value.started) return
        stop()
        start()
    }

    /** Foreground check: reconnect right away when the heartbeat went stale instead of trusting an old green dot. */
    fun reconnectIfStale(maxAgeMs: Long = LinkHealth.ATTENTION_AFTER_MS) {
        val s = _snapshot.value
        if (!s.started) return
        val last = s.lastBeatAtMs
        if (!s.connected || (s.supportsBeat && last != null && System.currentTimeMillis() - last > maxAgeMs)) reconnect()
    }

    private enum class SessionOutcome {
        HEALTHY_CLOSED,
        FAILED_OR_PREMATURE,
        AUTH_REJECTED
    }

    @Synchronized
    fun start() {
        if (runJob?.isActive == true) return
        _snapshot.update { it.copy(started = true, startedAtMs = System.currentTimeMillis()) }
        runJob = scope.launch {
            var attempt = 0
            while (isActive) {
                when (connectOnce()) {
                    SessionOutcome.AUTH_REJECTED -> {
                        stop()
                        break
                    }
                    SessionOutcome.HEALTHY_CLOSED -> {
                        attempt = 0
                        val jitter = (1_000L * (0.75 + Random.nextDouble() * 0.50)).toLong()
                        delay(jitter)
                    }
                    SessionOutcome.FAILED_OR_PREMATURE -> {
                        attempt = (attempt + 1).coerceAtMost(6)
                        val baseMs = (2_500L * (1 shl (attempt - 1).coerceAtLeast(0))).coerceAtMost(60_000L)
                        val delayMs = (baseMs * (0.75 + Random.nextDouble() * 0.50)).toLong()
                        delay(delayMs)
                    }
                }
            }
        }
    }

    @Synchronized
    fun stop() {
        runJob?.cancel()
        runJob = null
        try { webSocket?.close(1000, "stop") } catch (_: Exception) {}
        webSocket = null
        _connected.value = false
        _snapshot.update { it.copy(started = false, connected = false) }
    }

    fun close() {
        stop()
        scope.cancel()
    }

    /** Runs one socket until it closes. */
    private suspend fun connectOnce(): SessionOutcome {
        val base = baseUrlProvider()?.trimEnd('/') ?: return SessionOutcome.FAILED_OR_PREMATURE
        val scheme = if (base.startsWith("https://", ignoreCase = true)) "wss" else "ws"
        val hostPart = base.substringAfter("://")
        if (lastHost != null && lastHost != hostPart) {
            lastRev = 0L
        }
        lastHost = hostPart
        val revSuffix = if (lastRev > 0L) "?sinceRev=$lastRev" else ""
        val url = "$scheme://$hostPart/gateway/events$revSuffix"

        val builder = Request.Builder().url(url)
        prefs.deviceToken?.takeIf { it.isNotBlank() }?.let { token ->
            builder.header("Authorization", "Bearer $token")
            builder.header("x-device-token", token)
        }

        val done = CompletableDeferred<SessionOutcome>()
        val opened = java.util.concurrent.atomic.AtomicBoolean(false)
        val openedAtMs = java.util.concurrent.atomic.AtomicLong(0L)
        val receivedValidFrame = java.util.concurrent.atomic.AtomicBoolean(false)
        val ws = client.newWebSocket(builder.build(), object : WebSocketListener() {
            override fun onOpen(webSocket: WebSocket, response: Response) {
                opened.set(true)
                openedAtMs.set(System.currentTimeMillis())
                _connected.value = true
                _snapshot.update { it.copy(connected = true, hasEverConnected = true) }
            }

            override fun onMessage(webSocket: WebSocket, text: String) {
                val frame = parseConversationEventFrame(text) ?: return
                receivedValidFrame.set(true)
                if (frame.rev > 0L) {
                    lastRev = frame.rev
                }
                if (frame.type == "hello" || frame.type == "beat" || frame.type == "changed") {
                    _snapshot.update {
                        it.copy(
                            lastBeatAtMs = System.currentTimeMillis(),
                            beatCount = it.beatCount + 1,
                            supportsBeat = it.supportsBeat || frame.desktop != null,
                            desktop = frame.desktop ?: it.desktop
                        )
                    }
                }
                if (frame.type == "changed" || (frame.type == "hello" && !frame.unchanged)) {
                    _changes.tryEmit(Unit)
                }
            }

            override fun onClosing(webSocket: WebSocket, code: Int, reason: String) {
                webSocket.close(1000, null)
            }

            override fun onClosed(webSocket: WebSocket, code: Int, reason: String) {
                if (code == 4001 || code == 4003) {
                    done.complete(SessionOutcome.AUTH_REJECTED)
                    return
                }
                val isOpen = opened.get()
                val tOpen = openedAtMs.get()
                val livedLongEnough = isOpen && tOpen > 0L && (System.currentTimeMillis() - tOpen) >= 15_000L
                val outcome = if (isOpen && (receivedValidFrame.get() || livedLongEnough)) {
                    SessionOutcome.HEALTHY_CLOSED
                } else {
                    SessionOutcome.FAILED_OR_PREMATURE
                }
                done.complete(outcome)
            }

            override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
                val httpCode = response?.code ?: 0
                Log.d("ConvEvents", "events socket ended (http=$httpCode): ${t.message}")
                if (httpCode == 401 || httpCode == 403) {
                    done.complete(SessionOutcome.AUTH_REJECTED)
                    return
                }
                val isOpen = opened.get()
                val tOpen = openedAtMs.get()
                val livedLongEnough = isOpen && tOpen > 0L && (System.currentTimeMillis() - tOpen) >= 15_000L
                val outcome = if (isOpen && (receivedValidFrame.get() || livedLongEnough)) {
                    SessionOutcome.HEALTHY_CLOSED
                } else {
                    SessionOutcome.FAILED_OR_PREMATURE
                }
                done.complete(outcome)
            }
        })
        synchronized(this) { webSocket = ws }
        return try {
            done.await()
        } finally {
            _connected.value = false
            _snapshot.update { it.copy(connected = false) }
            synchronized(this) { if (webSocket === ws) webSocket = null }
        }
    }
}
