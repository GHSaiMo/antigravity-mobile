package com.antigravity.mobile.data.service

import android.content.Context
import android.net.ConnectivityManager
import android.net.NetworkCapabilities
import android.util.Log
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitAll
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
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
        val host = try {
            val uri = URI(if (!urlString.contains("://")) "http://$urlString" else urlString)
            uri.host ?: urlString
        } catch (_: Exception) {
            urlString
        }.trim().trim('[', ']').lowercase()

        val isLan = host == "127.0.0.1" || host == "localhost" || host == "::1" ||
                host.endsWith(".local") || host.startsWith("192.168.") ||
                host.startsWith("10.") || (host.startsWith("172.") && run {
                    val parts = host.split(".")
                    if (parts.size >= 2) {
                        val second = parts[1].toIntOrNull() ?: 0
                        second in 16..31
                    } else false
                })
        val isIPv6 = host.contains(":") && !host.startsWith("fe80") && !host.startsWith("fc") && !host.startsWith("fd")
        val isTailscale = host.startsWith("100.") || host.contains("ts.net")
        val isRelay = host.contains("relay")

        return when {
            isLan -> "Wi-Fi 局域网"
            isIPv6 -> if (cellular) "蜂窝网络 IPv6 直连" else "Wi-Fi IPv6 直连"
            isRelay -> if (cellular) "蜂窝网络 (云中继)" else "Wi-Fi (云中继)"
            isTailscale -> if (cellular) "蜂窝网络 (Tailscale)" else "Wi-Fi (Tailscale)"
            else -> if (cellular) "蜂窝网络 (公网)" else "Wi-Fi (公网)"
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

        val candidates = listOfNotNull(
            prefs.lanServerUrl?.takeIf { it.isNotBlank() },
            prefs.ipv6ServerUrl?.takeIf { it.isNotBlank() },
            prefs.relayServerUrl?.takeIf { it.isNotBlank() },
            prefs.customServerUrl?.takeIf { it.isNotBlank() },
            prefs.gatewayBaseUrl?.takeIf { it.isNotBlank() }
        ).distinct()

        if (candidates.isEmpty()) return@withContext null

        _isProbing.value = true
        try {
            val results = coroutineScope {
                candidates.map { url ->
                    async { testSingleEndpoint(url) }
                }.awaitAll()
            }

            val map = results.associateBy { it.urlString }
            _endpointStatuses.value = map
            _lastProbeTime.value = System.currentTimeMillis()

            val reachable = results.filter { it.isReachable }
            val cellular = isCellular

            val lanEp = reachable.firstOrNull { ep ->
                val clean = ep.urlString.lowercase()
                clean.contains("192.168.") || clean.contains("10.") || clean.contains("172.")
            }

            val winner = when {
                !cellular && lanEp != null -> lanEp
                cellular -> {
                    val v6Ep = reachable.firstOrNull { ep ->
                        val clean = ep.urlString.lowercase()
                        clean.contains("[") || clean.contains("::")
                    }
                    v6Ep ?: reachable.minByOrNull { it.latencyMs }
                }
                else -> reachable.minByOrNull { it.latencyMs }
            }

            if (winner != null) {
                prefs.gatewayBaseUrl = winner.urlString
                return@withContext winner.urlString
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
}
