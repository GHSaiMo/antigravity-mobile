package com.antigravity.mobile.data.service

import android.content.Context
import android.net.ConnectivityManager
import android.net.Network
import android.net.NetworkCapabilities
import android.util.Log
import com.antigravity.mobile.data.model.PairingInfo
import com.antigravity.mobile.ui.util.loc
import kotlinx.coroutines.CompletableDeferred
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
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withContext
import okhttp3.ConnectionPool
import okhttp3.OkHttpClient
import okhttp3.Request
import org.json.JSONObject
import java.net.URI
import java.util.concurrent.TimeUnit

data class EndpointHealthStatus(
    val urlString: String,
    val isReachable: Boolean,
    val latencyMs: Long,
    val errorMessage: String? = null,
    val platform: String? = null
)

/**
 * Strict cleartext enforcement interceptor compliant with Security Audit M-1.
 * Ensures cleartext HTTP traffic is strictly limited to RFC 1918 private LAN IP
 * addresses and loopback. Public domain HTTP traffic is rejected immediately.
 */
class LanCleartextSecurityInterceptor : okhttp3.Interceptor {
    override fun intercept(chain: okhttp3.Interceptor.Chain): okhttp3.Response {
        val request = chain.request()
        val url = request.url
        if (!url.isHttps) {
            val host = url.host
            if (!ConnectionManager.isLanHost(host)) {
                throw java.io.IOException("Cleartext HTTP traffic to public host '$host' is rejected by security policy.")
            }
        }
        return chain.proceed(request)
    }
}

class ConnectionManager(private val context: Context) {
    private val connectivityManager = context.getSystemService(Context.CONNECTIVITY_SERVICE) as? ConnectivityManager

    private val httpConnectionPool = ConnectionPool()

    // Short timeout for LAN probes so off-network Wi-Fi failover stays snappy;
    // longer timeout for public HTTPS / cellular relay-pool discovery & cold TLS handshakes.
    private val httpClient = OkHttpClient.Builder()
        .connectionPool(httpConnectionPool)
        .dns(CloudflareOptimizedDns)
        .connectTimeout(1200, TimeUnit.MILLISECONDS)
        .readTimeout(1200, TimeUnit.MILLISECONDS)
        .writeTimeout(1200, TimeUnit.MILLISECONDS)
        .addInterceptor(CloudflareEdgeFallbackInterceptor(httpConnectionPool))
        .addInterceptor(LanCleartextSecurityInterceptor())
        .build()

    private val fastLanProbeClient = httpClient.newBuilder()
        .connectTimeout(650, TimeUnit.MILLISECONDS)
        .readTimeout(650, TimeUnit.MILLISECONDS)
        .writeTimeout(650, TimeUnit.MILLISECONDS)
        .build()

    private val publicProbeClient = httpClient.newBuilder()
        .connectTimeout(5500, TimeUnit.MILLISECONDS)
        .readTimeout(5500, TimeUnit.MILLISECONDS)
        .writeTimeout(5500, TimeUnit.MILLISECONDS)
        .build()

    private val probeLock = Any()
    @Volatile private var inFlightProbe: CompletableDeferred<String?>? = null
    private val lanProbeLock = Any()
    @Volatile private var inFlightLanProbe: CompletableDeferred<Boolean>? = null
    @Volatile private var lastFailedLanProbeAtMs: Long = 0L
    private val migrationLock = Any()
    @Volatile private var lastFailedMigrationOrigin: String? = null
    @Volatile private var lastFailedMigrationAtMs: Long = 0L

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

    fun isEndpointHealthy(urlString: String?): Boolean? {
        if (urlString.isNullOrBlank()) return null
        val clean = urlString.trim().trimEnd('/')
        val status = _endpointStatuses.value[clean] ?: _endpointStatuses.value.entries.firstOrNull {
            it.key.trimEnd('/') == clean
        }?.value
        return status?.isReachable
    }

    fun notifyRouteChanged(newUrl: String) {
        val clean = newUrl.trim().trimEnd('/')
        routeChangeListeners.forEach { it.invoke(clean) }
    }

    /** Clears recent LAN probe failure cooldown when returning to foreground so walking back onto home Wi-Fi re-checks LAN immediately. */
    fun noteForegroundResume() {
        lastFailedLanProbeAtMs = 0L
    }

    /** Resets cached endpoint health and detaches any in-flight probes when switching gateways. */
    fun resetForGatewaySwitch() {
        synchronized(probeLock) {
            inFlightProbe?.complete(null)
            inFlightProbe = null
        }
        synchronized(lanProbeLock) {
            inFlightLanProbe?.complete(false)
            inFlightLanProbe = null
        }
        synchronized(migrationLock) {
            lastFailedMigrationOrigin = null
            lastFailedMigrationAtMs = 0L
        }
        lastFailedLanProbeAtMs = 0L
        _endpointStatuses.value = emptyMap()
        _lastProbeTime.value = null
        _isProbing.value = false
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
                val now = System.currentTimeMillis()
                val transportChanged = lastWasCellular != cellular
                val probeStale = (now - (_lastProbeTime.value ?: 0L)) > 6000L
                if (hasInternet && (transportChanged || probeStale)) {
                    Log.d("ConnectionManager", "Network capabilities update: cellular=$cellular, transportChanged=$transportChanged, probing...")
                    lastWasCellular = cellular
                    if (transportChanged) {
                        lastFailedLanProbeAtMs = 0L
                    }
                    scope.launch {
                        probeEndpoints(prefs)
                    }
                }
            }

            override fun onAvailable(network: Network) {
                val cellular = isCellular
                Log.d("ConnectionManager", "Network available: cellular=$cellular, probing...")
                lastWasCellular = cellular
                lastFailedLanProbeAtMs = 0L
                scope.launch {
                    probeEndpoints(prefs)
                }
            }

            override fun onLost(network: Network) {
                Log.d("ConnectionManager", "Network lost, waiting for new interface...")
                lastFailedLanProbeAtMs = 0L
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

        if (isConnectedToNetwork) {
            scope.launch {
                probeEndpoints(prefs)
            }
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
        if (urlString.isBlank()) return if (cellular) "蜂窝网络".loc else "Wi-Fi"
        val host = extractHost(urlString)

        val isLan = isLanHost(host)
        val isTailscale = isTailscaleHost(host)

        return when {
            isLan -> "Wi-Fi 局域网直连".loc
            isTailscale -> if (cellular) "蜂窝网络 (Tailscale)".loc else "Wi-Fi (Tailscale)".loc
            urlString.startsWith("https://") -> if (cellular) "蜂窝网络 (专属公网 HTTPS)".loc else "Wi-Fi (专属公网 HTTPS)".loc
            else -> if (cellular) "蜂窝网络 (公网直连)".loc else "Wi-Fi (公网直连)".loc
        }
    }

    suspend fun testSingleEndpoint(
        urlString: String,
        overrideClient: OkHttpClient? = null
    ): EndpointHealthStatus = withContext(Dispatchers.IO) {
        val clean = urlString.trim().trimEnd('/')
        if (clean.isBlank()) {
            return@withContext EndpointHealthStatus(
                urlString = urlString,
                isReachable = false,
                latencyMs = 0,
                errorMessage = "无效地址".loc
            )
        }
        val targetUrl = if (!clean.startsWith("http://") && !clean.startsWith("https://")) {
            "http://$clean/healthz"
        } else {
            "$clean/healthz"
        }

        val targetClient = overrideClient ?: if (isLanHost(extractHost(clean))) httpClient else publicProbeClient
        val start = System.currentTimeMillis()
        try {
            val req = Request.Builder().url(targetUrl).build()
            targetClient.newCall(req).execute().use { resp ->
                val took = System.currentTimeMillis() - start
                if (resp.isSuccessful) {
                    E2eeCrypto.observeServerCapabilities(resp, resp.request.url.host)
                    val body = resp.body?.string() ?: ""
                    var platform: String? = null
                    try {
                        val json = JSONObject(body)
                        platform = json.optString("platform").takeIf { it.isNotBlank() }
                            ?: json.optString("os").takeIf { it.isNotBlank() }
                    } catch (_: Exception) {}
                    EndpointHealthStatus(
                        urlString = urlString,
                        isReachable = true,
                        latencyMs = took,
                        errorMessage = null,
                        platform = platform
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
                errorMessage = e.message ?: "连接超时".loc
            )
        }
    }

    /**
     * Synchronous single-flight relay migration for OkHttp interceptors when an active cloud/relay
     * endpoint fails with HTTP 502/503 (`gateway_offline`) or a network error.
     * Deduplicates concurrent failures so only one thread queries `/api/relay/pool` and probes peers.
     */
    fun migrateUnreachableCloudBlocking(prefs: PreferencesManager, failedOrigin: String): String? {
        val cleanFailed = failedOrigin.trim().trimEnd('/').lowercase()
        if (cleanFailed.isEmpty() || !E2eeCrypto.isReady(prefs)) return null

        synchronized(migrationLock) {
            // Fast path: another thread already migrated away from cleanFailed.
            val currentCloud = prefs.primaryCloudUrl?.trim()?.trimEnd('/')
            if (!currentCloud.isNullOrBlank() &&
                !currentCloud.equals(cleanFailed, ignoreCase = true) &&
                !isLanHost(extractHost(currentCloud))
            ) {
                val curActive = prefs.gatewayBaseUrl?.trim()?.trimEnd('/')
                if (curActive.isNullOrBlank() || curActive.equals(cleanFailed, ignoreCase = true)) {
                    prefs.gatewayBaseUrl = currentCloud
                }
                return currentCloud
            }

            val now = System.currentTimeMillis()
            if (lastFailedMigrationOrigin == cleanFailed && (now - lastFailedMigrationAtMs) < 10_000L) {
                return null
            }

            val migratedUrl = try {
                runBlocking(Dispatchers.IO) {
                    RelayPoolResolver.resolveReachableCloudUrl(
                        currentCloudUrl = cleanFailed,
                        prefs = prefs,
                        connectionManager = this@ConnectionManager,
                        httpClient = publicProbeClient
                    )
                }
            } catch (e: Exception) {
                Log.w("ConnectionManager", "Blocking relay migration failed for $cleanFailed: ${e.message}")
                null
            }

            if (!migratedUrl.isNullOrBlank() && !migratedUrl.equals(cleanFailed, ignoreCase = true)) {
                lastFailedMigrationOrigin = null
                lastFailedMigrationAtMs = 0L
                val cleanMigrated = migratedUrl.trim().trimEnd('/')
                val oldBase = prefs.gatewayBaseUrl
                prefs.primaryCloudUrl = cleanMigrated
                if (extractHost(cleanMigrated).startsWith("r-")) {
                    prefs.relayServerUrl = cleanMigrated
                }
                prefs.gatewayBaseUrl = cleanMigrated
                Log.i("ConnectionManager", "Migrated cloud relay from $cleanFailed to $cleanMigrated")
                if (!oldBase.equals(cleanMigrated, ignoreCase = true)) {
                    notifyRouteChanged(cleanMigrated)
                }
                return cleanMigrated
            }

            lastFailedMigrationOrigin = cleanFailed
            lastFailedMigrationAtMs = System.currentTimeMillis()
            return null
        }
    }

    /**
     * Ultra-fast LAN-only pre-probe on Wi-Fi when the active gateway is not yet LAN.
     * Returns true if LAN answered HTTP 200 and `gatewayBaseUrl` switched to LAN.
     */
    suspend fun probeLanFastIfNeeded(prefs: PreferencesManager, force: Boolean = false): Boolean {
        if (!prefs.isPaired()) return false
        val isCellularNow = isCellular || !isWifi
        if (isCellularNow) return false
        val lan = prefs.lanServerUrl?.trim()?.trimEnd('/')?.takeIf { it.isNotBlank() } ?: return false
        if (isSameEndpoint(prefs.gatewayBaseUrl, lan)) return false
        val now = System.currentTimeMillis()
        if (!force && lastFailedLanProbeAtMs > 0L && (now - lastFailedLanProbeAtMs) < 30_000L) {
            return false
        }

        val existing: CompletableDeferred<Boolean>?
        val owned: CompletableDeferred<Boolean>?
        synchronized(lanProbeLock) {
            val current = inFlightLanProbe
            if (current != null && !current.isCompleted) {
                existing = current
                owned = null
            } else {
                val created = CompletableDeferred<Boolean>()
                inFlightLanProbe = created
                existing = null
                owned = created
            }
        }
        if (existing != null) {
            return existing.await()
        }
        val startEpoch = com.antigravity.mobile.data.model.GatewayRuntimeState.epoch
        val deferred = owned!!
        return try {
            val status = testSingleEndpoint(lan, overrideClient = fastLanProbeClient)
            if (com.antigravity.mobile.data.model.GatewayRuntimeState.epoch != startEpoch) {
                deferred.complete(false)
                return false
            }
            _endpointStatuses.value = _endpointStatuses.value + (status.urlString to status)
            val switched = if (status.isReachable) {
                lastFailedLanProbeAtMs = 0L
                _lastProbeTime.value = System.currentTimeMillis()
                status.platform?.takeIf { it.isNotBlank() }?.let { prefs.gatewayPlatform = it }
                val oldBase = prefs.gatewayBaseUrl
                prefs.gatewayBaseUrl = lan
                val changed = !isSameEndpoint(oldBase, lan)
                if (changed) {
                    notifyRouteChanged(lan)
                }
                changed
            } else {
                lastFailedLanProbeAtMs = System.currentTimeMillis()
                false
            }
            deferred.complete(switched)
            switched
        } catch (t: Throwable) {
            deferred.complete(false)
            false
        } finally {
            synchronized(lanProbeLock) {
                if (inFlightLanProbe === deferred) {
                    inFlightLanProbe = null
                }
            }
        }
    }

    suspend fun probeEndpoints(prefs: PreferencesManager): String? {
        val existing: CompletableDeferred<String?>?
        val owned: CompletableDeferred<String?>?
        synchronized(probeLock) {
            val current = inFlightProbe
            if (current != null && !current.isCompleted) {
                existing = current
                owned = null
            } else {
                val created = CompletableDeferred<String?>()
                inFlightProbe = created
                existing = null
                owned = created
            }
        }
        if (existing != null) {
            return existing.await()
        }
        val deferred = owned!!
        return try {
            val result = performProbeEndpoints(prefs)
            deferred.complete(result)
            result
        } catch (t: Throwable) {
            deferred.complete(prefs.gatewayBaseUrl)
            throw t
        } finally {
            synchronized(probeLock) {
                if (inFlightProbe === deferred) {
                    inFlightProbe = null
                }
            }
        }
    }

    private suspend fun performProbeEndpoints(prefs: PreferencesManager): String? = withContext(Dispatchers.IO) {
        val startEpoch = com.antigravity.mobile.data.model.GatewayRuntimeState.epoch
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
            val distinctEndpoints = endpointsToTest.distinct()
            val cleanLan = if (!isCellularNow) lan else null
            val results = mutableListOf<EndpointHealthStatus>()
            coroutineScope {
                val channel = kotlinx.coroutines.channels.Channel<EndpointHealthStatus>(distinctEndpoints.size)
                val jobs = distinctEndpoints.map { url ->
                    launch {
                        channel.send(testSingleEndpoint(url))
                    }
                }
                var received = 0
                while (received < distinctEndpoints.size) {
                    val status = channel.receive()
                    results.add(status)
                    received++
                    // LAN short-circuit: LAN always wins on Wi-Fi, so elect immediately instead of
                    // waiting up to 5.5s for a slow or unreachable cloud endpoint.
                    if (cleanLan != null && status.isReachable && isSameEndpoint(status.urlString, cleanLan)) {
                        jobs.forEach { it.cancel() }
                        break
                    }
                }
                channel.close()
            }

            if (com.antigravity.mobile.data.model.GatewayRuntimeState.epoch != startEpoch) {
                return@withContext null
            }

            if (cleanLan != null) {
                val lanRes = results.firstOrNull { isSameEndpoint(it.urlString, cleanLan) }
                if (lanRes != null) {
                    lastFailedLanProbeAtMs = if (lanRes.isReachable) 0L else System.currentTimeMillis()
                }
            }

            val map = results.associateBy { it.urlString }
            _endpointStatuses.value = _endpointStatuses.value + map
            _lastProbeTime.value = System.currentTimeMillis()

            val reachable = results.filter { it.isReachable }

            // Extract and update gateway platform from reachable endpoints
            reachable.firstOrNull { !it.platform.isNullOrBlank() }?.platform?.let { plat ->
                prefs.gatewayPlatform = plat
            }

            // Silent legacy-to-E2EE key negotiation for pre-1.2.5 paired devices (zero user interaction)
            if (!prefs.deviceToken.isNullOrBlank() && !E2eeCrypto.isReady(prefs) && reachable.isNotEmpty()) {
                val reachableBase = reachable.first().urlString
                val upgradeClient = if (isLanHost(extractHost(reachableBase))) httpClient else publicProbeClient
                E2eeCrypto.silentUpgradeIfNeeded(reachableBase, prefs, upgradeClient)
            }

            // Stateless multi-VPS pool auto-discovery when all known endpoints are unreachable
            val seedCloudUrl = prefs.primaryCloudUrl?.trim()?.trimEnd('/')?.takeIf { it.isNotBlank() }
                ?: prefs.gatewayBaseUrl?.trim()?.trimEnd('/')?.takeIf { it.isNotBlank() && !isLanHost(extractHost(it)) }
            if (reachable.isEmpty() && E2eeCrypto.isReady(prefs) && !seedCloudUrl.isNullOrBlank()) {
                val migratedUrl = RelayPoolResolver.resolveReachableCloudUrl(
                    currentCloudUrl = seedCloudUrl,
                    prefs = prefs,
                    connectionManager = this@ConnectionManager,
                    httpClient = publicProbeClient
                )
                if (!migratedUrl.isNullOrBlank()) {
                    val cleanMigrated = migratedUrl.trim().trimEnd('/')
                    val oldBase = prefs.gatewayBaseUrl
                    prefs.primaryCloudUrl = cleanMigrated
                    if (extractHost(cleanMigrated).startsWith("r-")) {
                        prefs.relayServerUrl = cleanMigrated
                    }
                    prefs.gatewayBaseUrl = cleanMigrated
                    _endpointStatuses.value = _endpointStatuses.value + (
                        cleanMigrated to EndpointHealthStatus(
                            urlString = cleanMigrated,
                            isReachable = true,
                            latencyMs = 0L
                        )
                    )
                    if (!oldBase.equals(cleanMigrated, ignoreCase = true)) {
                        notifyRouteChanged(cleanMigrated)
                    }
                    return@withContext cleanMigrated
                }
            }

            // Self-heal: if primaryCloudUrl is missing, discover it from /api/v1/auth/endpoints on reachable gateway
            if (prefs.primaryCloudUrl.isNullOrBlank() && reachable.isNotEmpty()) {
                val token = prefs.deviceToken?.trim()?.takeIf { it.isNotBlank() }
                for (ep in reachable) {
                    val cleanBase = ep.urlString.trim().trimEnd('/')
                    val probeUrl = "$cleanBase/api/v1/auth/endpoints"
                    val epClient = if (isLanHost(extractHost(cleanBase))) httpClient else publicProbeClient
                    try {
                        val reqBuilder = Request.Builder().url(probeUrl).get()
                        if (token != null) {
                            reqBuilder.header("Authorization", "Bearer $token")
                        }
                        epClient.newCall(reqBuilder.build()).execute().use { resp ->
                            if (resp.isSuccessful) {
                                applyAdvertisedCloudUrl(
                                    rawHeader = resp.header("X-Antigravity-Cloud-URL"),
                                    prefs = prefs,
                                    connectionManager = this@ConnectionManager
                                )
                                if (prefs.primaryCloudUrl.isNullOrBlank()) {
                                    val bodyStr = resp.body?.string().orEmpty()
                                    val json = JSONObject(bodyStr)
                                    val eps = json.optJSONArray("endpoints")
                                    if (eps != null) {
                                        for (i in 0 until eps.length()) {
                                            val item = eps.optJSONObject(i) ?: continue
                                            val type = item.optString("type").trim().lowercase()
                                            val epUrl = item.optString("url").trim().trimEnd('/')
                                            if ((type == "cloudflare" || type == "relay") &&
                                                isValidAdvertisedCloudUrl(epUrl, prefs)
                                            ) {
                                                prefs.primaryCloudUrl = epUrl
                                                break
                                            }
                                        }
                                    }
                                }
                            }
                        }
                    } catch (_: Exception) {}
                    if (!prefs.primaryCloudUrl.isNullOrBlank()) {
                        break
                    }
                }
            }

            val effectiveCloud = prefs.primaryCloudUrl?.trim()?.trimEnd('/')?.takeIf { it.isNotBlank() } ?: cloud

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
            if (selected == null && effectiveCloud != null) {
                selected = reachable.firstOrNull { it.urlString.trimEnd('/') == effectiveCloud }?.urlString ?: effectiveCloud
            }

            if (selected != null) {
                if (com.antigravity.mobile.data.model.GatewayRuntimeState.epoch != startEpoch) {
                    return@withContext null
                }
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
        if (clean.isBlank()) return@withContext Result.failure(IllegalArgumentException("网关地址为空".loc))

        val statusUrl = if (!clean.startsWith("http://") && !clean.startsWith("https://")) {
            "http://$clean/gateway/status"
        } else {
            "$clean/gateway/status"
        }

        val start = System.currentTimeMillis()
        val prefs = PreferencesManager(context)
        try {
            val req = Request.Builder().url(statusUrl).build()
            httpClient.newCall(req).execute().use { resp ->
                val took = System.currentTimeMillis() - start
                if (resp.isSuccessful) {
                    E2eeCrypto.observeServerCapabilities(resp, resp.request.url.host)
                    val body = resp.body?.string() ?: ""
                    var pidInfo = ""
                    try {
                        val json = JSONObject(body)
                        val upstream = json.optJSONObject("upstream")
                        val pid = upstream?.optInt("pid", 0) ?: 0
                        if (pid > 0) pidInfo = " PID $pid ·"
                        val platform = json.optString("platform").takeIf { it.isNotBlank() }
                            ?: json.optString("os").takeIf { it.isNotBlank() }
                        if (!platform.isNullOrBlank()) {
                            prefs.gatewayPlatform = platform
                        }
                    } catch (_: Exception) {}

                    val ifaceDesc = describeEndpoint(clean)
                    Result.success("连接成功:%1\$s %2\$dms (%3\$s)".loc(pidInfo, took, ifaceDesc))
                } else {
                    // Fallback to /healthz
                    val healthUrl = statusUrl.replace("/gateway/status", "/healthz")
                    val healthReq = Request.Builder().url(healthUrl).build()
                    httpClient.newCall(healthReq).execute().use { hResp ->
                        val took2 = System.currentTimeMillis() - start
                        if (hResp.isSuccessful) {
                            E2eeCrypto.observeServerCapabilities(hResp, hResp.request.url.host)
                            val hBody = hResp.body?.string() ?: ""
                            try {
                                val hJson = JSONObject(hBody)
                                val platform = hJson.optString("platform").takeIf { it.isNotBlank() }
                                    ?: hJson.optString("os").takeIf { it.isNotBlank() }
                                if (!platform.isNullOrBlank()) {
                                    prefs.gatewayPlatform = platform
                                }
                            } catch (_: Exception) {}
                            val ifaceDesc = describeEndpoint(clean)
                            Result.success("网关在线 (%1\$dms · %2\$s)".loc(took2, ifaceDesc))
                        } else {
                            Result.failure(RuntimeException("网关响应异常 (HTTP %d)".loc(resp.code)))
                        }
                    }
                }
            }
        } catch (e: Exception) {
            val took = System.currentTimeMillis() - start
            val desc = e.message ?: "连接失败".loc
            if (desc.contains("SSL") || desc.contains("cert")) {
                Result.failure(RuntimeException("SSL握手失败，网关默认使用 HTTP 协议".loc))
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

        fun isRelayHost(host: String): Boolean {
            val clean = host.trim().trim('[', ']').lowercase()
            return clean.endsWith(".mgy.jiuge.space") ||
                clean.endsWith(".jiuge.space") ||
                clean.endsWith(".trycloudflare.com")
        }

        fun isTailscaleHost(host: String): Boolean {
            val clean = host.trim().trim('[', ']').lowercase()
            if (clean == "ts.net" || clean.endsWith(".ts.net")) return true
            if (clean.startsWith("100.")) {
                val parts = clean.split(".")
                if (parts.size == 4 && parts.all { p -> p.toIntOrNull()?.let { it in 0..255 } == true }) {
                    val second = parts[1].toIntOrNull() ?: -1
                    if (second in 64..127) return true
                }
            }
            return false
        }

        fun isTrustedEndpoint(urlString: String, info: PairingInfo, usedBase: String): Boolean {
            val trimmed = urlString.trim()
            val isHttps = trimmed.startsWith("https://", ignoreCase = true)
            val isHttp = trimmed.startsWith("http://", ignoreCase = true)
            if (!isHttps && !isHttp) return false

            val host = extractHost(trimmed)
            if (host.isBlank()) return false
            val clean = host.trim('[', ']').lowercase()
            if (isHttp && !isLanHost(clean) && !isTailscaleHost(clean)) {
                return false
            }

            val allowed = mutableSetOf<String>()
            allowed.add(info.host.trim('[', ']').lowercase())
            info.lanHost?.takeIf { it.isNotBlank() }?.let { allowed.add(it.trim('[', ']').lowercase()) }
            info.ddnsHost?.takeIf { it.isNotBlank() }?.let { allowed.add(it.trim('[', ']').lowercase()) }
            info.relayHost?.takeIf { it.isNotBlank() }?.let { allowed.add(it.trim('[', ']').lowercase()) }

            val usedHost = extractHost(usedBase).trim('[', ']').lowercase()
            if (usedHost.isNotBlank()) allowed.add(usedHost)

            if (allowed.contains(clean)) return true
            if (isHttps && (clean.endsWith(".mgy.jiuge.space") ||
                    clean.endsWith(".jiuge.space") ||
                    clean.endsWith(".trycloudflare.com") ||
                    clean.endsWith(".antigravity.internal"))
            ) {
                return true
            }
            val parts = clean.split(".")
            if (isHttps && parts.size >= 3) {
                val parentSuffix = "." + parts.drop(1).joinToString(".")
                if (allowed.any { it.endsWith(parentSuffix) }) return true
            }
            return isLanHost(clean) || isTailscaleHost(clean)
        }

        fun isSameEndpoint(url1: String?, url2: String?): Boolean {
            if (url1.isNullOrBlank() || url2.isNullOrBlank()) return false
            return url1.trim().trimEnd('/') == url2.trim().trimEnd('/')
        }

        /**
         * Validates an advertised public cloud/relay URL (from X-Antigravity-Cloud-URL or /api/v1/auth/endpoints),
         * mirroring iOS NetworkTransport.isValidAdvertisedCloudURL to prevent untrusted origin hijacking.
         */
        fun isValidAdvertisedCloudUrl(
            raw: String,
            prefs: PreferencesManager?,
            trustedByE2ee: Boolean = false
        ): Boolean {
            val trimmed = raw.trim().trimEnd('/')
            if (trimmed.isEmpty()) return false
            val uri = try {
                URI(trimmed)
            } catch (_: Exception) {
                return false
            }
            if (!"https".equals(uri.scheme, ignoreCase = true)) return false
            if (uri.userInfo != null) return false
            val host = uri.host?.trim('[', ']')?.lowercase().orEmpty()
            if (host.isEmpty() || isLanHost(host) || isTailscaleHost(host)) return false

            if (trustedByE2ee || RelayPoolResolver.isPoolDomainHost(host)) {
                return true
            }
            if (host.endsWith(".trycloudflare.com") || host.endsWith(".jiuge.space")) {
                return true
            }

            val configuredHosts = buildSet {
                if (prefs != null) {
                    listOfNotNull(
                        prefs.gatewayBaseUrl,
                        prefs.primaryCloudUrl,
                        prefs.lanServerUrl,
                        prefs.customServerUrl,
                        prefs.relayServerUrl
                    ).forEach { rawEp ->
                        val h = extractHost(rawEp)
                        if (h.isNotEmpty()) add(h)
                    }
                }
            }
            if (configuredHosts.contains(host)) {
                return true
            }
            val parts = host.split(".")
            if (parts.size >= 3) {
                val parentSuffix = "." + parts.drop(1).joinToString(".")
                if (configuredHosts.any { it.endsWith(parentSuffix) }) {
                    return true
                }
            }
            return false
        }

        /**
         * Silently hot-updates primaryCloudUrl when a trusted paired gateway advertises a migrated cloud/relay URL.
         * Also syncs gatewayBaseUrl when the device is currently using the previous cloud/relay endpoint
         * (while never clobbering active LAN, Tailscale, or user-configured custom routes).
         */
        fun applyAdvertisedCloudUrl(
            rawHeader: String?,
            prefs: PreferencesManager,
            trustedByE2ee: Boolean = false,
            connectionManager: ConnectionManager? = null
        ) {
            val clean = rawHeader?.trim()?.trimEnd('/')?.takeIf { it.isNotBlank() } ?: return
            if (!isValidAdvertisedCloudUrl(clean, prefs, trustedByE2ee = trustedByE2ee)) return
            val host = extractHost(clean)
            if (host.startsWith("r-")) {
                E2eeCrypto.markHostRequiresE2ee(host)
                if (prefs.relayServerUrl?.trim()?.trimEnd('/') != clean) {
                    prefs.relayServerUrl = clean
                }
            }
            val current = prefs.primaryCloudUrl?.trim()?.trimEnd('/')
            if (current != clean) {
                prefs.primaryCloudUrl = clean
            }
            val active = prefs.gatewayBaseUrl?.trim()?.trimEnd('/')
            if (!active.isNullOrBlank() && !active.equals(clean, ignoreCase = true)) {
                val activeHost = extractHost(active)
                val custom = prefs.customServerUrl?.trim()?.trimEnd('/')
                val isUsingOldCloud = (!current.isNullOrBlank() && active.equals(current, ignoreCase = true)) ||
                    (!isLanHost(activeHost) &&
                        !isTailscaleHost(activeHost) &&
                        !active.equals(custom, ignoreCase = true) &&
                        isRelayHost(activeHost))
                if (isUsingOldCloud) {
                    prefs.gatewayBaseUrl = clean
                    connectionManager?.notifyRouteChanged(clean)
                }
            }
        }
    }
}
