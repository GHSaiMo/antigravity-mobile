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
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.longOrNull
import java.util.concurrent.TimeUnit

/** One frame of the gateway's `/gateway/events` channel. Frames carry a revision, never list data. */
internal data class ConversationEventFrame(val type: String, val rev: Long)

internal fun parseConversationEventFrame(text: String): ConversationEventFrame? = try {
    val obj = Json.parseToJsonElement(text).jsonObject
    val type = obj["type"]?.jsonPrimitive?.content
    if (type == null) null else ConversationEventFrame(type, obj["rev"]?.jsonPrimitive?.longOrNull ?: 0L)
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

    private val _connected = MutableStateFlow(false)
    val connected: StateFlow<Boolean> = _connected.asStateFlow()

    private val _changes = MutableSharedFlow<Unit>(extraBufferCapacity = 1, onBufferOverflow = kotlinx.coroutines.channels.BufferOverflow.DROP_OLDEST)
    val changes: SharedFlow<Unit> = _changes.asSharedFlow()

    @Synchronized
    fun start() {
        if (runJob?.isActive == true) return
        runJob = scope.launch {
            var attempt = 0
            while (isActive) {
                val opened = connectOnce()
                attempt = if (opened) 0 else (attempt + 1).coerceAtMost(6)
                // Quick retry after a healthy session ended; back off while the gateway is unreachable or lacks the endpoint.
                val delayMs = if (opened) 1_000L else (2_500L * (1 shl (attempt - 1).coerceAtLeast(0))).coerceAtMost(60_000L)
                delay(delayMs)
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
    }

    fun close() {
        stop()
        scope.cancel()
    }

    /** Runs one socket until it closes. Returns true when the handshake succeeded. */
    private suspend fun connectOnce(): Boolean {
        val base = baseUrlProvider()?.trimEnd('/') ?: return false
        val scheme = if (base.startsWith("https://", ignoreCase = true)) "wss" else "ws"
        val url = "$scheme://${base.substringAfter("://")}/gateway/events"

        val builder = Request.Builder().url(url)
        prefs.deviceToken?.takeIf { it.isNotBlank() }?.let { token ->
            builder.header("Authorization", "Bearer $token")
            builder.header("x-device-token", token)
        }

        val done = CompletableDeferred<Boolean>()
        var opened = false
        val ws = client.newWebSocket(builder.build(), object : WebSocketListener() {
            override fun onOpen(webSocket: WebSocket, response: Response) {
                opened = true
                _connected.value = true
            }

            override fun onMessage(webSocket: WebSocket, text: String) {
                val frame = parseConversationEventFrame(text) ?: return
                if (frame.type == "hello" || frame.type == "changed") {
                    _changes.tryEmit(Unit)
                }
            }

            override fun onClosing(webSocket: WebSocket, code: Int, reason: String) {
                webSocket.close(1000, null)
            }

            override fun onClosed(webSocket: WebSocket, code: Int, reason: String) {
                done.complete(opened)
            }

            override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
                Log.d("ConvEvents", "events socket ended: ${t.message}")
                done.complete(opened)
            }
        })
        synchronized(this) { webSocket = ws }
        return try {
            done.await()
        } finally {
            _connected.value = false
            synchronized(this) { if (webSocket === ws) webSocket = null }
        }
    }
}
