package com.antigravity.mobile.data.service

import android.util.Log
import com.antigravity.mobile.data.model.StreamUpdatePayload
import kotlinx.coroutines.*
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.serialization.json.Json
import okhttp3.*
import java.net.URLEncoder
import java.util.concurrent.TimeUnit
import kotlin.random.Random

enum class ConnectionStatus {
    DISCONNECTED,
    CONNECTING,
    CONNECTED,
    FAILED
}

class StreamWebSocketClient(
    private val prefs: PreferencesManager,
    private val connectionManager: ConnectionManager? = null
) {
    private val json = JsonConfig.instance

    private val client = OkHttpClient.Builder()
        .dns(CloudflareOptimizedDns)
        .pingInterval(15, TimeUnit.SECONDS)
        .readTimeout(0, TimeUnit.MILLISECONDS) // infinite for websockets
        .addInterceptor(LanCleartextSecurityInterceptor())
        .build()

    private var webSocket: WebSocket? = null
    private var activeCascadeId: String? = null
    private var isIntentionallyClosed = false

    private val _connectionStatus = MutableStateFlow(ConnectionStatus.DISCONNECTED)
    val connectionStatus: StateFlow<ConnectionStatus> = _connectionStatus.asStateFlow()

    private val _streamUpdates = MutableStateFlow<StreamUpdatePayload?>(null)
    val streamUpdates: StateFlow<StreamUpdatePayload?> = _streamUpdates.asStateFlow()

    private val scope = CoroutineScope(Dispatchers.IO + SupervisorJob())
    private var reconnectJob: Job? = null
    private var reconnectAttempt = 0
    @Volatile private var openedAtMs: Long = 0L
    @Volatile private var lastFrameAtMs: Long = 0L
    @Volatile private var lastStreamRev: Long = 0L
    @Volatile private var revCascadeId: String? = null
    private val deltaReassembler = StreamDeltaReassembler()

    fun hasRecentFrame(withinMs: Long = 20_000L): Boolean {
        val last = lastFrameAtMs
        return _connectionStatus.value == ConnectionStatus.CONNECTED &&
            last > 0L &&
            (System.currentTimeMillis() - last) <= withinMs
    }

    init {
        connectionManager?.addOnRouteChangedListener { newUrl ->
            if (activeCascadeId != null && !isIntentionallyClosed) {
                Log.d("StreamWS", "Route changed to $newUrl, reconnecting stream...")
                disconnect(intentional = false)
                startConnection()
            }
        }
    }

    fun reconnect(force: Boolean = true) {
        val cid = activeCascadeId ?: return
        disconnect(intentional = false)
        startConnection()
    }

    fun connect(cascadeId: String) {
        if (_connectionStatus.value == ConnectionStatus.CONNECTED && activeCascadeId == cascadeId) {
            return
        }

        if (revCascadeId != cascadeId) {
            lastStreamRev = 0L
            revCascadeId = cascadeId
            deltaReassembler.reset()
        }

        disconnect(intentional = false)
        reconnectAttempt = 0
        activeCascadeId = cascadeId
        isIntentionallyClosed = false
        startConnection()
    }

    fun disconnect(intentional: Boolean = true) {
        isIntentionallyClosed = intentional
        reconnectAttempt = 0
        reconnectJob?.cancel()
        reconnectJob = null
        try {
            webSocket?.close(1000, "Normal Closure")
        } catch (_: Exception) {}
        webSocket = null
        if (intentional) {
            _connectionStatus.value = ConnectionStatus.DISCONNECTED
        }
    }

    private fun startConnection() {
        val avoidLan = (connectionManager?.isCellular ?: false) || !(connectionManager?.isWifi ?: true)
        val baseUrl = prefs.getEffectiveGatewayUrl(avoidLan, connectionManager) ?: prefs.gatewayBaseUrl ?: run {
            _connectionStatus.value = ConnectionStatus.FAILED
            return
        }
        val cascadeId = activeCascadeId ?: return

        val wsUrl = buildWebSocketUrl(baseUrl, cascadeId)
        _connectionStatus.value = ConnectionStatus.CONNECTING

        val token = prefs.deviceToken?.takeIf { it.isNotBlank() }
        val requestBuilder = Request.Builder()
            .url(wsUrl)

        if (token != null) {
            requestBuilder.header("Authorization", "Bearer $token")
        }

        val request = requestBuilder.build()

        webSocket = E2eeWebSocketHelper.openWebSocket(client, request, prefs, object : WebSocketListener() {
            override fun onOpen(webSocket: WebSocket, response: Response) {
                Log.d("StreamWS", "Connected to cascade stream: $cascadeId")
                openedAtMs = System.currentTimeMillis()
                lastFrameAtMs = openedAtMs
                if (lastStreamRev == 0L) {
                    deltaReassembler.reset()
                }
                _connectionStatus.value = ConnectionStatus.CONNECTED
            }

            override fun onMessage(webSocket: WebSocket, text: String) {
                try {
                    val raw = json.decodeFromString<StreamUpdatePayload>(text)
                    lastFrameAtMs = System.currentTimeMillis()
                    if (raw.type == "noop") {
                        reconnectAttempt = 0
                        if (raw.rev > 0L) {
                            lastStreamRev = raw.rev
                            revCascadeId = cascadeId
                        }
                        return
                    }
                    val payload = deltaReassembler.apply(raw)
                    if (payload == null) {
                        // Delta does not line up with what we hold: drop the socket and let the
                        // normal reconnect path fetch a fresh complete "init" frame.
                        Log.w("StreamWS", "Delta baseline mismatch, resyncing")
                        lastStreamRev = 0L
                        deltaReassembler.reset()
                        webSocket.close(1000, "delta resync")
                        return
                    }
                    reconnectAttempt = 0
                    if (raw.rev > 0L) {
                        lastStreamRev = raw.rev
                        revCascadeId = cascadeId
                    }
                    _streamUpdates.value = payload
                } catch (e: Exception) {
                    Log.e("StreamWS", "Failed to parse stream update: ${e.message}")
                }
            }

            override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
                val httpCode = response?.code ?: 0
                Log.w("StreamWS", "WebSocket failure (http=$httpCode): ${t.message}")
                if (httpCode == 401 || httpCode == 403) {
                    isIntentionallyClosed = true
                    _connectionStatus.value = ConnectionStatus.FAILED
                    return
                }
                if (openedAtMs > 0L && (System.currentTimeMillis() - openedAtMs) >= 15_000L) {
                    reconnectAttempt = 0
                }
                openedAtMs = 0L
                _connectionStatus.value = ConnectionStatus.FAILED
                scheduleReconnect()
            }

            override fun onClosed(webSocket: WebSocket, code: Int, reason: String) {
                Log.d("StreamWS", "WebSocket closed: $code - $reason")
                if (code == 4001 || code == 4003) {
                    isIntentionallyClosed = true
                    _connectionStatus.value = ConnectionStatus.FAILED
                    return
                }
                if (openedAtMs > 0L && (System.currentTimeMillis() - openedAtMs) >= 15_000L) {
                    reconnectAttempt = 0
                }
                openedAtMs = 0L
                if (!isIntentionallyClosed) {
                    _connectionStatus.value = ConnectionStatus.DISCONNECTED
                    scheduleReconnect()
                }
            }
        })
    }

    private fun scheduleReconnect() {
        if (isIntentionallyClosed || activeCascadeId == null) return
        if (reconnectJob?.isActive == true) return

        val attempt = reconnectAttempt
        val isLan = ConnectionManager.isLanHost(ConnectionManager.extractHost(prefs.gatewayBaseUrl ?: ""))
        val baseDelayMs = if (isLan && !prefs.primaryCloudUrl.isNullOrBlank() && attempt == 0) {
            500L
        } else {
            (2500L * (1 shl attempt.coerceAtMost(5))).coerceAtMost(60_000L)
        }
        val jitterFactor = 0.75 + Random.nextDouble() * 0.50
        val delayMs = (baseDelayMs * jitterFactor).toLong().coerceAtLeast(350L)
        reconnectAttempt++

        reconnectJob = scope.launch {
            delay(delayMs)
            if (!isIntentionallyClosed && activeCascadeId != null) {
                connectionManager?.probeEndpoints(prefs)
                Log.d("StreamWS", "Attempting reconnection (attempt #$reconnectAttempt, delay=${delayMs}ms)...")
                startConnection()
            }
        }
    }

    private fun buildWebSocketUrl(baseUrl: String, cascadeId: String, ticket: String? = null): String {
        val cleanBase = baseUrl.trimEnd('/')
        val wsScheme = if (cleanBase.startsWith("https://", ignoreCase = true)) "wss" else "ws"
        val hostAndPort = cleanBase.substringAfter("://")
        val encodedCid = URLEncoder.encode(cascadeId, "UTF-8")

        var url = "$wsScheme://$hostAndPort/gateway/cascade/stream?cascadeId=$encodedCid&client=android&format=messages&delta=1"
        if (lastStreamRev > 0L && revCascadeId == cascadeId) {
            url += "&sinceRev=$lastStreamRev"
        }
        if (!ticket.isNullOrBlank()) {
            url += "&ticket=${URLEncoder.encode(ticket, "UTF-8")}"
        }
        return url
    }
}
