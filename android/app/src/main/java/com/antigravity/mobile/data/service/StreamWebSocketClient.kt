package com.antigravity.mobile.data.service

import android.util.Log
import com.antigravity.mobile.data.model.StreamUpdatePayload
import kotlinx.coroutines.*
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.serialization.json.Json
import okhttp3.*
import java.util.concurrent.TimeUnit

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
            requestBuilder.header("x-device-token", token)
        }

        val request = requestBuilder.build()

        webSocket = client.newWebSocket(request, object : WebSocketListener() {
            override fun onOpen(webSocket: WebSocket, response: Response) {
                Log.d("StreamWS", "Connected to cascade stream: $cascadeId")
                reconnectAttempt = 0
                _connectionStatus.value = ConnectionStatus.CONNECTED
            }

            override fun onMessage(webSocket: WebSocket, text: String) {
                try {
                    val payload = json.decodeFromString<StreamUpdatePayload>(text)
                    _streamUpdates.value = payload
                } catch (e: Exception) {
                    Log.e("StreamWS", "Failed to parse stream update: ${e.message}")
                }
            }

            override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
                Log.w("StreamWS", "WebSocket failure: ${t.message}")
                _connectionStatus.value = ConnectionStatus.FAILED
                scheduleReconnect()
            }

            override fun onClosed(webSocket: WebSocket, code: Int, reason: String) {
                Log.d("StreamWS", "WebSocket closed: $code - $reason")
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
        val delayMs = if (isLan && !prefs.primaryCloudUrl.isNullOrBlank() && attempt == 0) {
            500L
        } else {
            (2500L * (1 shl attempt.coerceAtMost(5))).coerceAtMost(60_000L)
        }
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

        var url = "$wsScheme://$hostAndPort/gateway/cascade/stream?cascadeId=$cascadeId&client=android&format=messages"
        if (!ticket.isNullOrBlank()) {
            url += "&ticket=$ticket"
        }
        return url
    }
}
