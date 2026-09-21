package com.antigravity.mobile.data.service

import android.content.Context
import android.net.ConnectivityManager
import android.net.Network
import android.net.NetworkCapabilities
import android.util.Log
import com.antigravity.mobile.data.model.PairingInfo
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitAll
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import okhttp3.OkHttpClient
import okhttp3.Request
import org.json.JSONObject
import java.net.URI
import java.util.concurrent.TimeUnit

data class EndpointHealthStatus(
    val urlString: String,
    val isReachable: Boolean,
    val latencyMs: Long,
    val errorMessage: String? = null
)

class ConnectionManager(private val context: Context) {
    private val connectivityManager = context.getSystemService(Context.CONNECTIVITY_SERVICE) as? ConnectivityManager

    private val httpClient = OkHttpClient.Builder()
        .connectTimeout(2500, TimeUnit.MILLISECONDS)
        .readTimeout(2500, TimeUnit.MILLISECONDS)
        .writeTimeout(2500, TimeUnit.MILLISECONDS)
        .build()

    private val _endpointStatuses = MutableStateFlow<Map<String, EndpointHealthStatus>>(emptyMap())
    val endpointStatuses: StateFlow<Map<String, EndpointHealthStatus>> = _endpointStatuses.asStateFlow()

    private val _isProbing = MutableStateFlow(false)
    val isProbing: StateFlow<Boolean> = _isProbing.asStateFlow()

    private val _lastProbeTime = MutableStateFlow<Long?>(null)
    val lastProbeTime: StateFlow<Long?> = _lastProbeTime.asStateFlow()

    private var isMonitoring = false
    private var lastWasCellular: Boolean? = null
    private var networkCallback: ConnectivityManager.NetworkCallback? = null
    private val routeChangeListeners = java.util.concurrent.CopyOnWriteArrayList<(String) -> Unit>()

    fun addOnRouteChangedListener(listener: (String) -> Unit) {
        routeChangeListeners.add(listener)
    }

    fun removeOnRouteChangedListener(listener: (String) -> Unit) {
        routeChangeListeners.remove(listener)
    }

    fun startMonitoring(
        prefs: PreferencesManager,
        scope: CoroutineScope = CoroutineScope(Dispatchers.IO + SupervisorJob())
    ) {
        if (isMonitoring) return
        val cm = connectivityManager ?: return
        isMonitoring = true
        lastWasCellular = isCellular

        val callback = object : ConnectivityManager.NetworkCallback() {
            override fun onCapabilitiesChanged(network: Network, networkCapabilities: NetworkCapabilities) {
                val cellular = networkCapabilities.hasTransport(NetworkCapabilities.TRANSPORT_CELLULAR)
                val hasInternet = networkCapabilities.hasCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET)
                if (hasInternet && lastWasCellular != cellular) {
                    Log.d("ConnectionManager", "Network transport changed: cellular=$cellular, triggering probe")
                    lastWasCellular = cellular
                    scope.launch {
                        probeEndpoints(prefs)
                    }
                }
            }

            override fun onAvailable(network: Network) {
                val cellular = isCellular
                Log.d("ConnectionManager", "Network available: cellular=$cellular, probing...")
                lastWasCellular = cellular
                scope.launch {
                    probeEndpoints(prefs)
                }
            }

            override fun onLost(network: Network) {
                Log.d("ConnectionManager", "Network lost, waiting for new interface...")
                scope.launch {
                    delay(300)
                    probeEndpoints(prefs)
                }
            }
        }

        networkCallback = callback
        try {
            cm.registerDefaultNetworkCallback(callback)
            Log.d("ConnectionManager", "Default network callback registered successfully")
        } catch (e: Exception) {
            Log.w("ConnectionManager", "Failed to register default network callback: ${e.message}")
        }
    }

    fun stopMonitoring() {
        if (!isMonitoring) return
        networkCallback?.let {
            try {
                connectivityManager?.unregisterNetworkCallback(it)
            } catch (_: Exception) {}
        }
        networkCallback = null
        isMonitoring = false
    }

    val isCellular: Boolean
        get() {
            val net = connectivityManager?.activeNetwork ?: return false
            val caps = connectivityManager.getNetworkCapabilities(net) ?: return false
            return caps.hasTransport(NetworkCapabilities.TRANSPORT_CELLULAR)
        }

    val isWifi: Boolean
        get() {
            val net = connectivityManager?.activeNetwork ?: return false
            val caps = connectivityManager.getNetworkCapabilities(net) ?: return false
            return caps.hasTransport(NetworkCapabilities.TRANSPORT_WIFI)
        }

    val isConnectedToNetwork: Boolean
        get() {
            val net = connectivityManager?.activeNetwork ?: return false
            val caps = connectivityManager.getNetworkCapabilities(net) ?: return false
            return caps.hasCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET)
        }

    fun describeEndpoint(urlString: String, cellular: Boolean = isCellular): String {
        if (urlString.isBlank()) return if (cellular) "蜂窝网络" else "Wi-Fi"
        val host = extractHost(urlString)

        val isLan = isLanHost(host)
        val isTailscale = isTailscaleHost(host)

        return when {
            isLan -> "Wi-Fi 局域网直连"
            isTailscale -> if (cellular) "蜂窝网络 (Tailscale)" else "Wi-Fi (Tailscale)"
            urlString.startsWith("https://") -> if (cellular) "蜂窝网络 (专属公网 HTTPS)" else "Wi-Fi (专属公网 HTTPS)"
            else -> if (cellular) "蜂窝网络 (公网直连)" else "Wi-Fi (公网直连)"
        }
    }

    suspend fun testSingleEndpoint(urlString: String): EndpointHealthStatus = withContext(Dispatchers.IO) {
        val clean = urlString.trim().trimEnd('/')
        if (clean.isBlank()) {
            return@withContext EndpointHealthStatus(
                urlString = urlString,
                isReachable = false,
                latencyMs = 0,
                errorMessage = "无效地址"
            )
        }
        val targetUrl = if (!clean.startsWith("http://") && !clean.startsWith("https://")) {
            "http://$clean/healthz"
        } else {
            "$clean/healthz"
        }

        val start = System.currentTimeMillis()
        try {
            val req = Request.Builder().url(targetUrl).build()
            httpClient.newCall(req).execute().use { resp ->
                val took = System.currentTimeMillis() - start
                if (resp.isSuccessful) {
                    EndpointHealthStatus(
                        urlString = urlString,
                        isReachable = true,
                        latencyMs = took,
                        errorMessage = null
                    )
                } else {
                    EndpointHealthStatus(
                        urlString = urlString,
                        isReachable = false,
                        latencyMs = took,
                        errorMessage = "HTTP ${resp.code}"
                    )
                }
            }
        } catch (e: Exception) {
            val took = System.currentTimeMillis() - start
            EndpointHealthStatus(
                urlString = urlString,
                isReachable = false,
                latencyMs = took,
                errorMessage = e.message ?: "连接超时"
            )
        }
    }

    suspend fun probeEndpoints(prefs: PreferencesManager): String? = withContext(Dispatchers.IO) {
        if (_isProbing.value) return@withContext prefs.gatewayBaseUrl

        val isCellularNow = isCellular || !isWifi

        // 智能路由候选集：
        // 1. 局域网：若为蜂窝网络状态，直接跳过！
        // 2. 自定义：若未配置，直接跳过！
        // 3. 主域名：最终兜底
        val endpointsToTest = mutableListOf<String>()
        val lan = prefs.lanServerUrl?.trim()?.trimEnd('/')?.takeIf { it.isNotBlank() }
        val custom = prefs.customServerUrl?.trim()?.trimEnd('/')?.takeIf { it.isNotBlank() }
        val cloud = prefs.primaryCloudUrl?.trim()?.trimEnd('/')?.takeIf { it.isNotBlank() }

        if (!isCellularNow && lan != null) {
            endpointsToTest.add(lan)
        }
        if (custom != null) {
            val isLan = isLanHost(extractHost(custom))
            if (!isCellularNow || !isLan) {
                endpointsToTest.add(custom)
            }
        }
        if (cloud != null) {
            endpointsToTest.add(cloud)
        }
        if (endpointsToTest.isEmpty() && !prefs.gatewayBaseUrl.isNullOrBlank()) {
            val base = prefs.gatewayBaseUrl!!.trim().trimEnd('/')
            val isLan = isLanHost(extractHost(base))
            if (!isCellularNow || !isLan) {
                endpointsToTest.add(base)
            }
        }

        if (endpointsToTest.isEmpty()) return@withContext null

        _isProbing.value = true
        try {
            val results = coroutineScope {
                endpointsToTest.distinct().map { url ->
                    async { testSingleEndpoint(url) }
                }.awaitAll()
            }

            val map = results.associateBy { it.urlString }
            _endpointStatuses.value = map
            _lastProbeTime.value = System.currentTimeMillis()

            val reachable = results.filter { it.isReachable }

            // 智能路由选举：
            // 顺序：局域网 -> 自定义 -> 主域名兜底
            var selected: String? = null

            // 1. 局域网（非蜂窝网络且在线）
            if (!isCellularNow && lan != null) {
                if (reachable.any { it.urlString.trimEnd('/') == lan }) {
                    selected = lan
                }
            }

            // 2. 自定义（已配置且在线）
            if (selected == null && custom != null) {
                if (reachable.any { it.urlString.trimEnd('/') == custom }) {
                    selected = custom
                }
            }

            // 3. 主域名（最终兜底）
            if (selected == null && cloud != null) {
                selected = reachable.firstOrNull { it.urlString.trimEnd('/') == cloud }?.urlString ?: cloud
            }

            if (selected != null) {
                val oldBase = prefs.gatewayBaseUrl
                prefs.gatewayBaseUrl = selected
                if (!oldBase.equals(selected, ignoreCase = true)) {
                    routeChangeListeners.forEach { it.invoke(selected) }
                }
                return@withContext selected
            }
            return@withContext prefs.gatewayBaseUrl
        } finally {
            _isProbing.value = false
        }
    }

    suspend fun testGatewayStatus(baseUrl: String): Result<String> = withContext(Dispatchers.IO) {
        val clean = baseUrl.trim().trimEnd('/')
        if (clean.isBlank()) return@withContext Result.failure(IllegalArgumentException("网关地址为空"))

        val statusUrl = if (!clean.startsWith("http://") && !clean.startsWith("https://")) {
            "http://$clean/gateway/status"
        } else {
            "$clean/gateway/status"
        }

        val start = System.currentTimeMillis()
        try {
            val req = Request.Builder().url(statusUrl).build()
            httpClient.newCall(req).execute().use { resp ->
                val took = System.currentTimeMillis() - start
                if (resp.isSuccessful) {
                    val body = resp.body?.string() ?: ""
                    var pidInfo = ""
                    try {
                        val json = JSONObject(body)
                        val upstream = json.optJSONObject("upstream")
                        val pid = upstream?.optInt("pid", 0) ?: 0
                        if (pid > 0) pidInfo = " PID $pid ·"
                    } catch (_: Exception) {}

                    val ifaceDesc = describeEndpoint(clean)
                    Result.success("连接成功:$pidInfo ${took}ms ($ifaceDesc)")
                } else {
                    // Fallback to /healthz
                    val healthUrl = statusUrl.replace("/gateway/status", "/healthz")
                    val healthReq = Request.Builder().url(healthUrl).build()
                    httpClient.newCall(healthReq).execute().use { hResp ->
                        val took2 = System.currentTimeMillis() - start
                        if (hResp.isSuccessful) {
                            val ifaceDesc = describeEndpoint(clean)
                            Result.success("网关在线 (${took2}ms · $ifaceDesc)")
                        } else {
                            Result.failure(RuntimeException("网关响应异常 (HTTP ${resp.code})"))
                        }
                    }
                }
            }
        } catch (e: Exception) {
            val took = System.currentTimeMillis() - start
            val desc = e.message ?: "连接失败"
            if (desc.contains("SSL") || desc.contains("cert")) {
                Result.failure(RuntimeException("SSL握手失败，网关默认使用 HTTP 协议"))
            } else {
                Result.failure(RuntimeException("$desc (${took}ms)"))
            }
        }
    }

    companion object {
        fun extractHost(urlString: String): String {
            return try {
                val uri = URI(if (!urlString.contains("://")) "http://$urlString" else urlString)
                uri.host ?: urlString
            } catch (_: Exception) {
                urlString
            }.trim().trim('[', ']').lowercase()
        }

        fun isLanHost(host: String): Boolean {
            val clean = host.trim().trim('[', ']').lowercase()
            if (clean == "127.0.0.1" || clean == "localhost" || clean == "::1" || clean.endsWith(".local")) {
                return true
            }
            if (clean.startsWith("192.168.") || clean.startsWith("10.") || clean.startsWith("127.")) {
                return true
            }
            if (clean.startsWith("172.")) {
                val parts = clean.split(".")
                if (parts.size >= 2) {
                    val second = parts[1].toIntOrNull() ?: 0
                    if (second in 16..31) return true
                }
            }
            return false
        }

        fun isIpv6Host(host: String): Boolean {
            val clean = host.trim().trim('[', ']').lowercase()
            return clean.contains(":") && !clean.startsWith("fe80") && !clean.startsWith("fc") && !clean.startsWith("fd")
        }

        fun isRelayHost(host: String): Boolean {
            val clean = host.trim().trim('[', ']').lowercase()
            return clean.contains("relay")
        }

        fun isTailscaleHost(host: String): Boolean {
            val clean = host.trim().trim('[', ']').lowercase()
            return clean.startsWith("100.") || clean.contains("ts.net")
        }

        fun isTrustedEndpoint(urlString: String, info: PairingInfo, usedBase: String): Boolean {
            val host = extractHost(urlString)
            if (host.isBlank()) return false
            val clean = host.trim('[', ']').lowercase()
            val allowed = mutableListOf<String>()
            allowed.add(info.host.trim('[', ']').lowercase())
            info.lanHost?.takeIf { it.isNotBlank() }?.let { allowed.add(it.trim('[', ']').lowercase()) }
            info.ipv6Host?.takeIf { it.isNotBlank() }?.let { allowed.add(it.trim('[', ']').lowercase()) }
            info.ddnsHost?.takeIf { it.isNotBlank() }?.let { allowed.add(it.trim('[', ']').lowercase()) }
            info.relayHost?.takeIf { it.isNotBlank() }?.let { allowed.add(it.trim('[', ']').lowercase()) }

            val usedHost = extractHost(usedBase).trim('[', ']').lowercase()
            if (usedHost.isNotBlank()) allowed.add(usedHost)

            if (allowed.contains(clean)) return true
            if (clean.endsWith(".jiuge.space") || clean.endsWith(".antigravity.internal")) return true
            if (urlString.startsWith("https://", ignoreCase = true)) return true
            return isLanHost(clean) || isTailscaleHost(clean) || isRelayHost(clean)
        }

        fun isSameEndpoint(url1: String?, url2: String?): Boolean {
            if (url1.isNullOrBlank() || url2.isNullOrBlank()) return false
            return url1.trim().trimEnd('/') == url2.trim().trimEnd('/')
        }
    }
}
