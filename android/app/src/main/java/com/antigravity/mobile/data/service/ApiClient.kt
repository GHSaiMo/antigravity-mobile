package com.antigravity.mobile.data.service

import com.antigravity.mobile.ui.util.loc
import android.content.Context
import android.os.Build
import android.util.Base64
import android.util.Log
import com.antigravity.mobile.data.model.*
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.withContext
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.*
import okhttp3.Cache
import okhttp3.Call
import okhttp3.Callback
import okhttp3.ConnectionPool
import okhttp3.HttpUrl
import okhttp3.HttpUrl.Companion.toHttpUrlOrNull
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import okhttp3.Response
import okio.source
import java.io.File
import java.io.IOException
import java.net.URLDecoder
import java.net.URLEncoder
import java.time.Instant
import java.util.concurrent.TimeUnit
import kotlin.coroutines.resume
import kotlin.coroutines.resumeWithException

private suspend fun Call.await(): Response = suspendCancellableCoroutine { continuation ->
    continuation.invokeOnCancellation {
        try {
            cancel()
        } catch (_: Throwable) {}
    }
    enqueue(object : Callback {
        override fun onResponse(call: Call, response: Response) {
            if (continuation.isCancelled) {
                response.close()
            } else {
                continuation.resume(response)
            }
        }
        override fun onFailure(call: Call, e: IOException) {
            continuation.resumeWithException(e)
        }
    })
}

class ApiClient(
    private val context: Context,
    private val prefs: PreferencesManager,
    private val connectionManager: ConnectionManager? = null
) {
    init {
        defaultInstance = this
    }

    companion object {
        const val IMAGE_PREVIEW_THUMB = "thumb"
        const val IMAGE_PREVIEW_FULL = "1"

        /**
         * Appends or replaces `?preview=<mode>` on gateway `/api/v1/files/raw` URLs.
         * Non-gateway / external / static URLs are returned unchanged.
         */
        fun withImagePreview(url: String, mode: String): String {
            val parsed = url.toHttpUrlOrNull() ?: return url
            if (!parsed.encodedPath.endsWith("/api/v1/files/raw")) return url
            return parsed.newBuilder()
                .removeAllQueryParameters("preview")
                .addQueryParameter("preview", mode)
                .build()
                .toString()
        }

        /**
         * Removes any `preview` query parameter from gateway `/api/v1/files/raw` URLs so
         * independent document/image download sheets always fetch the untouched original file.
         */
        fun stripImagePreview(url: String): String {
            val parsed = url.toHttpUrlOrNull() ?: return url
            if (!parsed.encodedPath.endsWith("/api/v1/files/raw")) return url
            if (parsed.queryParameter("preview") == null) return url
            return parsed.newBuilder()
                .removeAllQueryParameters("preview")
                .build()
                .toString()
        }

        /**
         * Stable Coil cache key for gateway media URLs (includes query params like `uri` and `preview`
         * so bubble thumbnails and fullscreen previews never collide in cache).
         */
        fun mediaCacheKey(url: String): String {
            return if (url.contains("/api/v1/files/raw")) {
                url.substringAfter("/api/v1/files/raw")
            } else {
                url
            }
        }

        @Volatile
        var defaultInstance: ApiClient? = null

        @Volatile
        var currentCascadeId: String? = null

        suspend fun fetchFileContent(uri: String, cascadeId: String? = null): Result<FileContentResponse> {
            val inst = defaultInstance ?: return Result.failure(IllegalStateException("ApiClient not initialized"))
            return inst.fetchFileContent(uri, cascadeId)
        }

        val currentBaseUrl: String?
            get() = defaultInstance?.currentBaseUrl

        /**
         * Strictly verifies that [url]'s `(scheme, host, port)` matches one of the explicitly paired
         * gateway endpoints stored in [prefs], preventing Bearer/device-token leakage to third-party origins.
         */
        fun isConfiguredGatewayOrigin(url: HttpUrl, prefs: PreferencesManager?): Boolean {
            if (prefs == null) return false
            val endpoints = listOfNotNull(
                prefs.gatewayBaseUrl,
                prefs.primaryCloudUrl,
                prefs.lanServerUrl,
                prefs.customServerUrl,
                prefs.relayServerUrl
            )
            for (raw in endpoints) {
                val clean = raw.trim().trimEnd('/')
                if (clean.isEmpty()) continue
                val candidate = clean.toHttpUrlOrNull() ?: continue
                if (candidate.scheme.equals(url.scheme, ignoreCase = true) &&
                    candidate.host.equals(url.host, ignoreCase = true) &&
                    candidate.port == url.port
                ) {
                    return true
                }
            }
            return false
        }
    }

    val currentBaseUrl: String?
        get() {
            val avoidLan = (connectionManager?.isCellular ?: false) || !(connectionManager?.isWifi ?: true)
            return prefs.getEffectiveGatewayUrl(avoidLan, connectionManager) ?: prefs.gatewayBaseUrl
        }

    val json = JsonConfig.instance

    val okHttpClient: OkHttpClient
        get() = client

    fun openWebSocket(request: Request, listener: okhttp3.WebSocketListener): okhttp3.WebSocket =
        E2eeWebSocketHelper.openWebSocket(client, request, prefs, listener)

    private val connectionPool = ConnectionPool()

    private val client = OkHttpClient.Builder()
        .connectionPool(connectionPool)
        .dns(CloudflareOptimizedDns)
        .cache(Cache(File(context.cacheDir, "http_cache"), 10L * 1024L * 1024L))
        .addInterceptor(RouteFailoverInterceptor(prefs, connectionManager))
        .addInterceptor(E2eeTransportInterceptor(prefs, connectionManager))
        .addInterceptor(CloudflareEdgeFallbackInterceptor(connectionPool))
        .addInterceptor(LanCleartextSecurityInterceptor())
        .retryOnConnectionFailure(true)
        .connectTimeout(10, TimeUnit.SECONDS)
        .readTimeout(30, TimeUnit.SECONDS)
        .writeTimeout(30, TimeUnit.SECONDS)
        .build()

    private val jsonMediaType = "application/json; charset=utf-8".toMediaType()
    private val messagesEtagCache = java.util.concurrent.ConcurrentHashMap<String, Pair<String, StreamUpdatePayload>>()

    /**
     * Attempts pairing with candidates from PairingInfo.
     * Supports encrypted zero-knowledge pairing (`POST /api/v1/e2ee/pair`) when QR carries `&pk=...`
     * over public relay hosts, as well as inline E2EE key negotiation during LAN/CF pairing.
     */
    suspend fun pair(info: PairingInfo): Result<PairResponse> = withContext(Dispatchers.IO) {
        val candidates = info.candidateBaseUrls()
        if (candidates.isEmpty()) {
            return@withContext Result.failure(IllegalArgumentException("无可用候选网关地址".loc))
        }

        val deviceName = "${Build.MANUFACTURER} ${Build.MODEL}"
        val (ephemKp, ephemPubB64) = E2eeCrypto.generateEphemeralKeyPair()
        val pairReq = PairRequest(
            pairingCode = info.code,
            deviceName = deviceName,
            platform = "android",
            e2eePub = ephemPubB64
        )
        val bodyStr = json.encodeToString(pairReq)

        var lastException: Exception? = null
        for (candidate in candidates) {
            val candHost = ConnectionManager.extractHost(candidate)
            val gwPub = info.e2eePubKey?.takeIf { it.isNotBlank() }
            val useEncryptedPair = gwPub != null && !ConnectionManager.isLanHost(candHost) && !ConnectionManager.isTailscaleHost(candHost)
            val endpoint = if (useEncryptedPair) "$candidate/api/v1/e2ee/pair" else "$candidate/api/v1/auth/pair"
            try {
                var activePriv = ephemKp.private
                var activeClientPub = ephemPubB64
                val postBody = if (useEncryptedPair) {
                    val (encKp, encPub, envJson) = E2eeCrypto.encryptPairEnvelope(bodyStr.toByteArray(Charsets.UTF_8), gwPub!!)
                    activePriv = encKp.private
                    activeClientPub = encPub
                    envJson
                } else {
                    bodyStr
                }

                val request = Request.Builder()
                    .url(endpoint)
                    .post(postBody.toRequestBody(jsonMediaType))
                    .build()

                client.newCall(request).await().use { response ->
                    val rawRespStr = response.body?.string() ?: ""
                    val (effectiveCode, respStr) = if (useEncryptedPair && response.isSuccessful) {
                        E2eeCrypto.decryptPairEnvelope(rawRespStr, activePriv, activeClientPub, gwPub!!)
                    } else {
                        response.code to rawRespStr
                    }
                    if (effectiveCode !in 200..299) {
                        val errMsg = respStr.ifBlank { "HTTP $effectiveCode" }
                        lastException = RuntimeException("配对响应失败: %s".loc(errMsg))
                        return@use
                    }
                    val pairResp = json.decodeFromString<PairResponse>(respStr)

                    // Derive E2EE session key in memory without mutating active prefs prematurely
                    // (protects existing gateways if adding a 2nd host).
                    var derivedE2eeKeyB64: String? = null
                    if (!pairResp.e2eeGatewayPub.isNullOrBlank() && !pairResp.e2eeSalt.isNullOrBlank()) {
                        try {
                            val salt = E2eeCrypto.decodeBase64Flexible(pairResp.e2eeSalt)
                            if (salt != null && salt.isNotEmpty()) {
                                val shared = E2eeCrypto.sharedSecret(activePriv, pairResp.e2eeGatewayPub)
                                val devKey = E2eeCrypto.deriveDeviceKey(shared, salt, pairResp.deviceId)
                                derivedE2eeKeyB64 = E2eeCrypto.base64UrlEncode(devKey)
                            }
                        } catch (e: Exception) {
                            Log.w("ApiClient", "Derive E2EE key failed: ${e.message}")
                        }
                    }

                    var lanUrl: String? = info.lanBaseUrl
                    var relayUrl: String? = info.relayBaseUrl
                    var cloudUrl: String? = null
                    val e2eeHosts = mutableListOf<String>()
                    relayUrl?.let { r ->
                        val h = ConnectionManager.extractHost(r)
                        if (h.isNotEmpty()) e2eeHosts.add(h)
                    }

                    if (info.ssl || !ConnectionManager.isLanHost(ConnectionManager.extractHost(info.host))) {
                        cloudUrl = info.serverBaseUrl
                    }

                    if (pairResp.endpoints != null) {
                        for (ep in pairResp.endpoints) {
                            val epUrl = ep.url.trim().trimEnd('/')
                            val trusted = ConnectionManager.isTrustedEndpoint(epUrl, info, candidate) ||
                                (useEncryptedPair && ConnectionManager.isValidAdvertisedCloudUrl(epUrl, prefs, trustedByE2ee = true))
                            if (!trusted) {
                                continue
                            }
                            when (ep.type.lowercase()) {
                                "lan" -> lanUrl = epUrl
                                "relay" -> {
                                    relayUrl = epUrl
                                    cloudUrl = epUrl
                                    val rHost = ConnectionManager.extractHost(epUrl)
                                    if (rHost.isNotEmpty()) e2eeHosts.add(rHost)
                                }
                                "cloudflare" -> {
                                    if (cloudUrl.isNullOrBlank() || relayUrl.isNullOrBlank()) {
                                        cloudUrl = epUrl
                                    }
                                }
                                "primary" -> {
                                    val h = ConnectionManager.extractHost(epUrl)
                                    when {
                                        ConnectionManager.isLanHost(h) && lanUrl.isNullOrBlank() -> lanUrl = epUrl
                                        ConnectionManager.isRelayHost(h) && relayUrl.isNullOrBlank() -> relayUrl = epUrl
                                        else -> if (cloudUrl.isNullOrBlank()) cloudUrl = epUrl
                                    }
                                }
                            }
                        }
                    }

                    val dualRelay = pairResp.relayCloudUrl?.trim()?.trimEnd('/')
                    if (!dualRelay.isNullOrBlank() &&
                        !derivedE2eeKeyB64.isNullOrBlank() &&
                        ConnectionManager.isValidAdvertisedCloudUrl(dualRelay, prefs, trustedByE2ee = true)
                    ) {
                        relayUrl = dualRelay
                        cloudUrl = dualRelay
                        val rHost = ConnectionManager.extractHost(dualRelay)
                        if (rHost.isNotEmpty()) e2eeHosts.add(rHost)
                    }

                    // Fallback classify the successful candidate into its slot if still unassigned
                    val candClean = candidate.trim().trimEnd('/')
                    when {
                        ConnectionManager.isLanHost(candHost) -> {
                            if (lanUrl.isNullOrBlank()) lanUrl = candClean
                        }
                        ConnectionManager.isRelayHost(candHost) -> {
                            if (relayUrl.isNullOrBlank()) relayUrl = candClean
                            if (cloudUrl.isNullOrBlank()) cloudUrl = candClean
                        }
                        else -> {
                            // Public allocated domain (e.g. Cloudflare tunnel) strictly serves as background fallback, NEVER custom!
                            if (cloudUrl.isNullOrBlank()) cloudUrl = candClean
                        }
                    }

                    val plat = pairResp.platform ?: pairResp.os ?: info.platform ?: info.os
                    val resolvedGatewayId = pairResp.gatewayId?.trim()?.takeIf { it.isNotEmpty() }
                        ?: GatewayProfileUtils.deriveGatewayId(pairResp.e2eeGatewayPub ?: info.e2eePubKey)
                    val resolvedGatewayName = pairResp.gatewayName?.trim()?.takeIf { it.isNotEmpty() }

                    val store = GatewayProfileStore.getOrCreate(context, prefs)
                    val hadActiveBefore = store.activeProfile != null && prefs.isPaired()
                    val (profile, _) = store.upsertPairedProfile(
                        gatewayId = resolvedGatewayId,
                        gatewayName = resolvedGatewayName,
                        platform = plat,
                        deviceId = pairResp.deviceId,
                        deviceToken = pairResp.deviceToken,
                        e2eeKeyB64 = derivedE2eeKeyB64,
                        lanUrl = lanUrl,
                        relayUrl = relayUrl,
                        cloudUrl = cloudUrl,
                        activeUrl = candClean,
                        e2eeRequiredHosts = e2eeHosts
                    )

                    if (hadActiveBefore && store.activeProfileId != profile.id) {
                        GatewaySwitcher.switchTo(
                            profileId = profile.id,
                            showToast = true,
                            force = true
                        )
                    } else {
                        store.setActiveProfileId(profile.id)
                        E2eeCrypto.resetForGatewaySwitch(profile.e2eeRequiredHosts)
                        store.runtimeCacheManager()?.switchStorageNamespace(profile.storageNamespace)
                        store.runtimeDocumentCacheManager()?.switchStorageNamespace(profile.storageNamespace)
                        prefs.applyProfile(
                            profile = profile,
                            profileDeviceToken = pairResp.deviceToken,
                            profileDeviceId = pairResp.deviceId,
                            profileE2eeKeyB64 = derivedE2eeKeyB64
                        )
                    }

                    prefs.fcmPushToken?.takeIf { it.isNotBlank() }?.let { fcm ->
                        syncPushTokenToAllGateways(fcm)
                    }

                    return@withContext Result.success(pairResp)
                }
            } catch (e: Exception) {
                Log.w("ApiClient", "Failed candidate $candidate: ${e.message}")
                lastException = e
            }
        }

        Result.failure(lastException ?: RuntimeException("所有候选地址配对均超时或失败".loc))
    }

    /**
     * Fetch all conversations via ConnectRPC GetAllCascadeTrajectories
     */
    suspend fun fetchConversations(): Result<List<ConversationItem>> = withContext(Dispatchers.IO) {
        val baseUrl = currentBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址".loc))
        val url = "$baseUrl/api/exa.language_server_pb.LanguageServerService/GetAllCascadeTrajectories"

        try {
            val req = buildAuthorizedRequest(url)
                .post("{}".toRequestBody(jsonMediaType))
                .build()

            client.newCall(req).await().use { response ->
                if (!response.isSuccessful) {
                    return@withContext Result.failure(RuntimeException("获取会话列表失败 (%d)".loc(response.code)))
                }
                val bodyStr = response.body?.string() ?: "{}"
                val resp = json.decodeFromString<GetAllCascadeTrajectoriesResponse>(bodyStr)
                val summaries = resp.trajectorySummaries ?: emptyMap()
                val localPinnedIds = prefs.getPinnedConversationIds()

                val list = summaries.mapNotNull { (id, summary) ->
                    if (summary.isSubagent) return@mapNotNull null
                    val localViewTime = prefs.getLastViewTime(id)
                    val serverPinned = summary.annotations?.pinned
                    val isPinned = if (serverPinned != null) {
                        if (serverPinned != localPinnedIds.contains(id)) {
                            prefs.setPinned(id, serverPinned)
                        }
                        serverPinned
                    } else {
                        localPinnedIds.contains(id)
                    }
                    val item = ConversationItem.fromSummary(
                        id = id,
                        summary = summary,
                        localViewTime = localViewTime,
                        isPinned = isPinned
                    )
                    if (item.isSubagent) return@mapNotNull null
                    item
                }.sortedWith(
                    compareByDescending<ConversationItem> { it.lastModifiedEpochMs }
                        .thenByDescending { it.id }
                )

                Result.success(list)
            }
        } catch (e: Exception) {
            Result.failure(e)
        }
    }

    /**
     * Fetch Cockpit Quota status
     */
    suspend fun fetchCockpitQuotas(): Result<CockpitQuotaResponse> = withContext(Dispatchers.IO) {
        val baseUrl = currentBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址".loc))
        val url = "$baseUrl/api/v1/cockpit/quotas"

        try {
            val req = buildAuthorizedRequest(url).get().build()
            client.newCall(req).await().use { response ->
                if (!response.isSuccessful) {
                    return@withContext Result.failure(RuntimeException("获取配额失败 (%d)".loc(response.code)))
                }
                val bodyStr = response.body?.string() ?: "{}"
                val quotaResp = json.decodeFromString<CockpitQuotaResponse>(bodyStr)
                Result.success(quotaResp)
            }
        } catch (e: Exception) {
            Result.failure(e)
        }
    }

    /**
     * Fetch Cockpit Token History & 52-week Heatmap
     */
    suspend fun fetchTokenHistory(force: Boolean = false): Result<TokenHistoryResponse> = withContext(Dispatchers.IO) {
        val baseUrl = currentBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址".loc))
        val url = "$baseUrl/api/v1/cockpit/tokens/history" + if (force) "?force=true" else ""

        try {
            val req = buildAuthorizedRequest(url).get().build()
            client.newCall(req).await().use { response ->
                if (!response.isSuccessful) {
                    return@withContext Result.failure(RuntimeException("获取 Token 历史失败 (%d)".loc(response.code)))
                }
                val bodyStr = response.body?.string() ?: "{}"
                val historyResp = json.decodeFromString<TokenHistoryResponse>(bodyStr)
                Result.success(historyResp)
            }
        } catch (e: Exception) {
            Result.failure(e)
        }
    }

    /**
     * Refresh Cockpit Quota
     */
    suspend fun refreshCockpitQuotas(): Result<CockpitQuotaResponse> = withContext(Dispatchers.IO) {
        val baseUrl = currentBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址".loc))
        val url = "$baseUrl/api/v1/cockpit/refresh"

        try {
            val req = buildAuthorizedRequest(url).post("{}".toRequestBody(jsonMediaType)).build()
            client.newCall(req).await().use { response ->
                if (!response.isSuccessful) {
                    return@withContext Result.failure(RuntimeException("刷新配额失败 (%d)".loc(response.code)))
                }
                val bodyStr = response.body?.string() ?: "{}"
                val quotaResp = json.decodeFromString<CockpitQuotaResponse>(bodyStr)
                Result.success(quotaResp)
            }
        } catch (e: Exception) {
            Result.failure(e)
        }
    }

    /**
     * Switch active Cockpit Account
     */
    suspend fun switchCockpitAccount(accountId: String): Result<Unit> = withContext(Dispatchers.IO) {
        val baseUrl = currentBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址".loc))
        val url = "$baseUrl/api/v1/cockpit/switch"

        val body = buildJsonObject {
            put("account_id", accountId)
        }.toString().toRequestBody(jsonMediaType)

        try {
            val req = buildAuthorizedRequest(url).post(body).build()
            client.newCall(req).await().use { response ->
                if (response.isSuccessful) {
                    Result.success(Unit)
                } else {
                    Result.failure(RuntimeException("切换账号失败: HTTP %d".loc(response.code)))
                }
            }
        } catch (e: Exception) {
            Result.failure(e)
        }
    }

    /**
     * Fetch workspace projects from gateway
     */
    suspend fun fetchProjects(): Result<List<ProjectItem>> = withContext(Dispatchers.IO) {
        val baseUrl = currentBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址".loc))
        val url = "$baseUrl/gateway/projects"

        try {
            val req = buildAuthorizedRequest(url).get().build()
            client.newCall(req).await().use { response ->
                if (!response.isSuccessful) {
                    val errMsg = response.body?.string()?.take(200) ?: ""
                    Log.e("ApiClient", "fetchProjects failed (${response.code}): $errMsg")
                    return@withContext Result.failure(RuntimeException("获取工作区列表失败 (%d)".loc(response.code)))
                }
                val bodyStr = response.body?.string() ?: "[]"
                val list = json.decodeFromString<List<ProjectItem>>(bodyStr)
                Log.d("ApiClient", "fetchProjects successfully retrieved ${list.size} projects")
                if (prefs.gatewayPlatform == null) {
                    for (item in list) {
                        val p = item.path.ifBlank { item.uri }
                        if (p.contains(":\\\\") || p.contains(":/") || p.contains(":\\") || (p.length >= 2 && p[1] == ':')) {
                            prefs.gatewayPlatform = "windows"
                            break
                        } else if (p.startsWith("/Users/")) {
                            prefs.gatewayPlatform = "darwin"
                            break
                        } else if (p.startsWith("/home/")) {
                            prefs.gatewayPlatform = "linux"
                            break
                        }
                    }
                }
                Result.success(list)
            }
        } catch (e: Exception) {
            Log.e("ApiClient", "fetchProjects error: ${e.message}", e)
            Result.failure(e)
        }
    }

    /**
     * Update or clear custom alias for a workspace/project path
     */
    suspend fun updateProjectAlias(path: String, alias: String): Result<Unit> = withContext(Dispatchers.IO) {
        val baseUrl = currentBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址".loc))
        val url = "$baseUrl/gateway/projects/alias"

        val payload = buildJsonObject {
            put("path", path.trim())
            put("alias", alias.trim())
        }

        try {
            val req = buildAuthorizedRequest(url)
                .post(payload.toString().toRequestBody(jsonMediaType))
                .build()

            client.newCall(req).await().use { response ->
                if (response.isSuccessful) {
                    Result.success(Unit)
                } else {
                    Result.failure(RuntimeException("修改工作区备注失败: HTTP %d".loc(response.code)))
                }
            }
        } catch (e: Exception) {
            Result.failure(e)
        }
    }

    /**
     * Create a new cascade session
     */
    suspend fun createCascade(
        workspaceUri: String,
        prompt: String,
        model: String? = null,
        projectId: String? = null
    ): Result<String> = withContext(Dispatchers.IO) {
        val baseUrl = currentBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址".loc))
        val url = "$baseUrl/gateway/cascade/new"

        val payload = buildJsonObject {
            put("workspaceUri", workspaceUri)
            put("prompt", prompt)
            model?.let { put("model", it) }
            projectId?.let { put("projectId", it) }
        }

        try {
            val req = buildAuthorizedRequest(url)
                .post(payload.toString().toRequestBody(jsonMediaType))
                .build()

            client.newCall(req).await().use { response ->
                if (!response.isSuccessful) {
                    val errMsg = formatUpstreamUnavailableError(response)
                        ?: "创建会话失败: HTTP %d".loc(response.code)
                    return@withContext Result.failure(RuntimeException(errMsg))
                }
                val bodyStr = response.body?.string() ?: "{}"
                val resObj = json.parseToJsonElement(bodyStr).jsonObject
                val cascadeId = resObj["cascadeId"]?.jsonPrimitive?.contentOrNull
                if (!cascadeId.isNullOrBlank()) {
                    Result.success(cascadeId)
                } else {
                    val err = resObj["error"]?.jsonPrimitive?.contentOrNull ?: "未能生成会话 ID".loc
                    Result.failure(RuntimeException(err))
                }
            }
        } catch (e: Exception) {
            Result.failure(e)
        }
    }

    /**
     * Delete a conversation
     */
    suspend fun deleteConversation(cascadeId: String): Result<Unit> = withContext(Dispatchers.IO) {
        val baseUrl = currentBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址".loc))
        val url = "$baseUrl/api/exa.language_server_pb.LanguageServerService/DeleteCascadeTrajectory"

        val payload = buildJsonObject {
            put("cascadeId", cascadeId)
        }

        try {
            val req = buildAuthorizedRequest(url)
                .post(payload.toString().toRequestBody(jsonMediaType))
                .build()

            client.newCall(req).await().use { response ->
                if (response.isSuccessful) {
                    Result.success(Unit)
                } else {
                    Result.failure(RuntimeException("删除会话失败: HTTP %d".loc(response.code)))
                }
            }
        } catch (e: Exception) {
            Result.failure(e)
        }
    }

    /**
     * Rename conversation
     */
    suspend fun renameConversation(cascadeId: String, newTitle: String): Result<Unit> = withContext(Dispatchers.IO) {
        val baseUrl = currentBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址".loc))
        val url = "$baseUrl/api/exa.language_server_pb.LanguageServerService/UpdateConversationAnnotations"

        val payload = buildJsonObject {
            put("cascadeIds", buildJsonArray { add(JsonPrimitive(cascadeId)) })
            put("annotations", buildJsonObject {
                put("title", newTitle.trim())
            })
            put("mergeAnnotations", true)
        }

        try {
            val req = buildAuthorizedRequest(url)
                .post(payload.toString().toRequestBody(jsonMediaType))
                .build()

            client.newCall(req).await().use { response ->
                if (response.isSuccessful) {
                    Result.success(Unit)
                } else {
                    Result.failure(RuntimeException("重命名会话失败: HTTP %d".loc(response.code)))
                }
            }
        } catch (e: Exception) {
            Result.failure(e)
        }
    }

    /**
     * Update pinned state of a conversation in upstream language_server
     */
    suspend fun setConversationPinned(cascadeId: String, pinned: Boolean): Result<Unit> = withContext(Dispatchers.IO) {
        val baseUrl = currentBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址".loc))
        val url = "$baseUrl/api/exa.language_server_pb.LanguageServerService/UpdateConversationAnnotations"

        val payload = buildJsonObject {
            put("cascadeIds", buildJsonArray { add(JsonPrimitive(cascadeId)) })
            put("annotations", buildJsonObject {
                put("pinned", pinned)
            })
            put("mergeAnnotations", true)
        }

        try {
            val req = buildAuthorizedRequest(url)
                .post(payload.toString().toRequestBody(jsonMediaType))
                .build()

            client.newCall(req).await().use { response ->
                if (response.isSuccessful) {
                    Result.success(Unit)
                } else {
                    Result.failure(RuntimeException("设置置顶状态失败: HTTP %d".loc(response.code)))
                }
            }
        } catch (e: Exception) {
            Result.failure(e)
        }
    }

    private fun formatUpstreamUnavailableError(response: okhttp3.Response): String? {
        if (response.code != 503) return null
        val mgyErr = response.header("X-Mgy-Error")?.trim()?.lowercase()
        val bodyStr = try {
            response.peekBody(1024).string()
        } catch (_: Exception) {
            ""
        }
        var code: String? = null
        var reason: String? = null
        var hint = bodyStr.lowercase()
        if (bodyStr.isNotBlank()) {
            try {
                val obj = json.parseToJsonElement(bodyStr).jsonObject
                code = obj["code"]?.jsonPrimitive?.contentOrNull?.trim()?.lowercase()
                reason = obj["reason"]?.jsonPrimitive?.contentOrNull?.trim()?.lowercase()
                val errPart = obj["error"]?.jsonPrimitive?.contentOrNull.orEmpty()
                val msgPart = obj["message"]?.jsonPrimitive?.contentOrNull.orEmpty()
                hint = "$errPart $msgPart".lowercase()
            } catch (_: Exception) {
            }
        }
        val matched = mgyErr == "upstream_not_connected" ||
            code == "upstream_not_connected" ||
            hint.contains("antigravity upstream not connected") ||
            hint.contains("language_server is not connected") ||
            hint.contains("language_server not connected") ||
            hint.contains("upstream not connected")
        if (!matched) return null
        return when (reason) {
            "ls_access_denied" -> "检测到电脑端 Antigravity 权限隔离（如管理员权限启动），请以相同权限运行 mgy 或用普通权限重启 Antigravity 后重试".loc
            "ls_not_ready" -> "已检测到电脑端 Antigravity，正在等待服务就绪（请确认已登录并进入主界面），请稍后重试".loc
            else -> "电脑端 Antigravity 未打开或尚未连接，请先在电脑上启动 Antigravity 后重试".loc
        }
    }

    /**
     * Fetch paginated messages for a cascade session
     */
    suspend fun fetchMessages(
        cascadeId: String,
        limit: Int = 15,
        offset: Int? = null,
        fromOffset: Int? = null
    ): Result<StreamUpdatePayload> = withContext(Dispatchers.IO) {
        if (!CacheManager.isValidCascadeId(cascadeId)) {
            return@withContext Result.failure(IllegalArgumentException("无效的会话 ID".loc))
        }
        val baseUrl = currentBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址".loc))
        val encodedCid = URLEncoder.encode(cascadeId, "UTF-8")
        val urlBuilder = StringBuilder("$baseUrl/gateway/cascade/messages?cascadeId=$encodedCid&limit=$limit")
        offset?.let { urlBuilder.append("&offset=$it") }
        fromOffset?.let { urlBuilder.append("&fromOffset=$it") }

        try {
            val urlStr = urlBuilder.toString()
            val cached = messagesEtagCache[urlStr]
            val reqBuilder = buildAuthorizedRequest(urlStr).get()
            if (cached != null && cached.first.isNotBlank()) {
                reqBuilder.header("If-None-Match", cached.first)
            }

            client.newCall(reqBuilder.build()).await().use { response ->
                if (response.code == 304 && cached != null) {
                    return@withContext Result.success(cached.second)
                }
                if (!response.isSuccessful) {
                    val errMsg = formatUpstreamUnavailableError(response)
                        ?: "获取消息失败: HTTP %d".loc(response.code)
                    return@withContext Result.failure(RuntimeException(errMsg))
                }
                if (response.networkResponse?.code == 304 && cached != null) {
                    return@withContext Result.success(cached.second)
                }
                val bodyStr = response.body?.string() ?: "{}"
                val payload = json.decodeFromString<StreamUpdatePayload>(bodyStr)
                val etag = response.header("ETag")?.trim()
                if (!etag.isNullOrEmpty()) {
                    if (messagesEtagCache.size >= 32 && !messagesEtagCache.containsKey(urlStr)) {
                        messagesEtagCache.clear()
                    }
                    messagesEtagCache[urlStr] = etag to payload
                }
                Result.success(payload)
            }
        } catch (e: Exception) {
            Result.failure(e)
        }
    }

    /**
     * Send user message to a cascade session
     */
    suspend fun sendMessage(
        cascadeId: String,
        text: String,
        model: String? = null,
        images: List<Pair<ByteArray, String>> = emptyList(),
        deliveryStrategy: Int? = null,
        clientMessageId: String? = null,
        attachmentIds: List<String> = emptyList(),
        /** 斜杠命令名（如 "plan"）。网关会补全权威定义；命令条目排在文本条目之前。 */
        slashCommand: String? = null,
        contextItems: List<ContextMentionItem> = emptyList()
    ): Result<Unit> = withContext(Dispatchers.IO) {
        val baseUrl = currentBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址".loc))
        val url = "$baseUrl/api/exa.language_server_pb.LanguageServerService/SendUserCascadeMessage"

        val payload = buildJsonObject {
            put("cascadeId", cascadeId)
            putJsonArray("items") {
                if (!slashCommand.isNullOrBlank()) {
                    addJsonObject {
                        putJsonObject("item") {
                            putJsonObject("slashCommand") {
                                putJsonObject("info") { put("name", slashCommand) }
                            }
                        }
                    }
                }
                // 带命令时文本前补一个空格，让服务端生成的显示文本是 "/plan 内容"；只有命令没有文本时不带文本条目
                if (slashCommand.isNullOrBlank() || text.isNotEmpty()) {
                    addJsonObject {
                        put("text", if (slashCommand.isNullOrBlank()) text else " $text")
                    }
                }
            }
            put("text", text)
            if (images.isNotEmpty()) {
                // Only media[] is stored by language_server (and read by the model); sending the
                // legacy images[] copy as well would double the upload.
                putJsonArray("media") {
                    images.forEach { (bytes, mime) ->
                        addJsonObject {
                            put("inlineData", Base64.encodeToString(bytes, Base64.NO_WRAP))
                            put("mimeType", mime)
                        }
                    }
                }
            } else {
                put("media", JsonArray(emptyList()))
            }
            if (attachmentIds.isNotEmpty()) {
                putJsonArray("attachments") {
                    attachmentIds.forEach { id -> addJsonObject { put("id", id) } }
                }
            }
            if (contextItems.isNotEmpty()) {
                put("contextItems", json.encodeToJsonElement(contextItems))
            }
            deliveryStrategy?.let { put("deliveryStrategy", it) }
            model?.let { put("model", it) }
        }

        try {
            val reqBuilder = buildAuthorizedRequest(url)
                .post(payload.toString().toRequestBody(jsonMediaType))

            if (!clientMessageId.isNullOrBlank()) {
                reqBuilder.header("X-Client-Message-Id", clientMessageId)
            }
            if (!model.isNullOrBlank()) {
                reqBuilder.header("X-Antigravity-Model", model)
            }

            val req = reqBuilder.build()

            client.newCall(req).await().use { response ->
                if (response.isSuccessful) {
                    Result.success(Unit)
                } else {
                    val errMsg = formatUpstreamUnavailableError(response)
                        ?: "发送失败: HTTP %d".loc(response.code)
                    Result.failure(RuntimeException(errMsg))
                }
            }
        } catch (e: Exception) {
            Result.failure(e)
        }
    }

    /**
     * 删除队列中的消息 (DeleteAgentMessage)
     */
    suspend fun deleteAgentMessage(
        cascadeId: String,
        messageId: String
    ): Result<Unit> = withContext(Dispatchers.IO) {
        val baseUrl = currentBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址".loc))
        val url = "$baseUrl/api/exa.language_server_pb.LanguageServerService/DeleteAgentMessage"

        val payload = buildJsonObject {
            put("messageId", messageId)
            put("recipient", cascadeId)
        }

        try {
            val req = buildAuthorizedRequest(url)
                .post(payload.toString().toRequestBody(jsonMediaType))
                .build()

            client.newCall(req).await().use { response ->
                if (response.isSuccessful) {
                    Result.success(Unit)
                } else {
                    Result.failure(RuntimeException("删除队列消息失败: HTTP %d".loc(response.code)))
                }
            }
        } catch (e: Exception) {
            Result.failure(e)
        }
    }

    /**
     * Cancel ongoing invocation
     */
    suspend fun cancelInvocation(cascadeId: String, killBackgroundTasks: Boolean? = null): Result<Unit> = withContext(Dispatchers.IO) {
        val baseUrl = currentBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址".loc))
        val url = "$baseUrl/api/exa.language_server_pb.LanguageServerService/CancelCascadeInvocation"

        val payload = buildJsonObject {
            put("cascadeId", cascadeId)
            if (killBackgroundTasks != null) {
                put("killBackgroundTasks", killBackgroundTasks)
            }
        }

        try {
            val req = buildAuthorizedRequest(url)
                .post(payload.toString().toRequestBody(jsonMediaType))
                .build()

            client.newCall(req).await().use { response ->
                if (response.isSuccessful) {
                    Result.success(Unit)
                } else {
                    Result.failure(RuntimeException("取消失败: HTTP %d".loc(response.code)))
                }
            }
        } catch (e: Exception) {
            Result.failure(e)
        }
    }

    /**
     * 关停一个子代理（网关只接受子代理会话，不会误停主会话）。
     */
    suspend fun stopSubagent(conversationId: String): Result<Unit> = withContext(Dispatchers.IO) {
        val baseUrl = currentBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址".loc))
        val payload = buildJsonObject { put("conversationId", conversationId) }
        try {
            val req = buildAuthorizedRequest("$baseUrl/gateway/subagent/stop")
                .post(payload.toString().toRequestBody(jsonMediaType))
                .build()
            client.newCall(req).await().use { response ->
                if (response.isSuccessful) Result.success(Unit)
                else Result.failure(RuntimeException("关停子代理失败: HTTP %d".loc(response.code)))
            }
        } catch (e: Exception) {
            Result.failure(e)
        }
    }

    /**
     * Stop background running command task
     */
    suspend fun stopTask(cascadeId: String, taskId: String, stepIndex: Int = 0): Result<Unit> = withContext(Dispatchers.IO) {
        val baseUrl = currentBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址".loc))
        val url = "$baseUrl/gateway/cascade/task/stop"

        val payload = buildJsonObject {
            put("cascadeId", cascadeId)
            put("taskId", taskId)
            put("stepIndex", stepIndex)
            put("cascade_id", cascadeId)
            put("task_id", taskId)
            put("step_index", stepIndex)
        }

        try {
            val req = buildAuthorizedRequest(url)
                .post(payload.toString().toRequestBody(jsonMediaType))
                .build()

            client.newCall(req).await().use { response ->
                if (response.isSuccessful) {
                    Result.success(Unit)
                } else {
                    Result.failure(RuntimeException("停止任务失败: HTTP %d".loc(response.code)))
                }
            }
        } catch (e: Exception) {
            Result.failure(e)
        }
    }

    /**
     * Submit user interaction response (e.g. command approval, permission, ask_question)
     */
    suspend fun submitInteraction(
        cascadeId: String,
        trajectoryId: String = "",
        stepIndex: Int = 0,
        type: String = "",
        optionId: String? = null,
        scope: Int = 1,
        allow: Boolean = true,
        writeInResponse: String = "",
        skipped: Boolean = false,
        target: String? = null,
        questionResponses: List<QuestionResponse>? = null
    ): Result<Unit> = withContext(Dispatchers.IO) {
        val baseUrl = currentBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址".loc))
        val url = "$baseUrl/gateway/cascade/interaction"

        val reqObj = InteractionSubmitRequest(
            cascadeId = cascadeId,
            trajectoryId = trajectoryId,
            stepIndex = stepIndex,
            type = type,
            optionId = optionId,
            scope = scope,
            allow = allow,
            writeInResponse = writeInResponse,
            skipped = skipped,
            target = target,
            questionResponses = questionResponses
        )
        val bodyStr = json.encodeToString(reqObj)

        try {
            val req = buildAuthorizedRequest(url)
                .post(bodyStr.toRequestBody(jsonMediaType))
                .build()

            client.newCall(req).await().use { response ->
                if (response.isSuccessful) {
                    Result.success(Unit)
                } else {
                    Result.failure(RuntimeException("交互提交失败: HTTP %d".loc(response.code)))
                }
            }
        } catch (e: Exception) {
            Result.failure(e)
        }
    }

    /**
     * Proceed with artifact plan execution (aligned 1:1 with iOS SendUserCascadeMessage protocol)
     */
    suspend fun proceedArtifact(
        cascadeId: String,
        artifactUri: String,
        model: String? = null
    ): Result<Unit> = withContext(Dispatchers.IO) {
        val baseUrl = currentBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址".loc))
        val url = "$baseUrl/api/exa.language_server_pb.LanguageServerService/SendUserCascadeMessage"

        val comment = buildJsonObject {
            put("artifactUri", artifactUri)
            put("scope", buildJsonObject {
                put("case", "fullFile")
                put("value", buildJsonObject {})
            })
            put("approvalStatus", 1)
            put("comment", "")
        }

        val payload = buildJsonObject {
            put("cascadeId", cascadeId)
            put("items", JsonArray(emptyList()))
            put("artifactComments", buildJsonArray { add(comment) })
            if (!model.isNullOrBlank()) {
                put("model", model)
            }
        }

        try {
            val reqBuilder = buildAuthorizedRequest(url)
                .post(payload.toString().toRequestBody(jsonMediaType))
            if (!model.isNullOrBlank()) {
                reqBuilder.addHeader("X-Antigravity-Model", model)
            }
            val req = reqBuilder.build()

            client.newCall(req).await().use { response ->
                if (response.isSuccessful) {
                    Result.success(Unit)
                } else {
                    Result.failure(RuntimeException("执行方案推进失败: HTTP %d".loc(response.code)))
                }
            }
        } catch (e: Exception) {
            Result.failure(e)
        }
    }

    /**
     * Fetch document or artifact content with companion metadata
     */
    suspend fun fetchFileContent(uri: String, cascadeId: String? = null): Result<FileContentResponse> = withContext(Dispatchers.IO) {
        val baseUrl = currentBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址".loc))
        val unescapedUri = try { URLDecoder.decode(uri, "UTF-8") } catch (_: Exception) { uri }
        val encodedUri = URLEncoder.encode(unescapedUri, "UTF-8")
        val cidParam = cascadeId?.let { "&cascade_id=${URLEncoder.encode(it, "UTF-8")}" } ?: ""
        val url = "$baseUrl/api/v1/files/content?uri=$encodedUri$cidParam"

        try {
            val req = buildAuthorizedRequest(url).get().build()
            client.newCall(req).await().use { response ->
                if (!response.isSuccessful) {
                    return@withContext Result.failure(RuntimeException("获取文件失败: HTTP %d".loc(response.code)))
                }
                val bodyStr = response.body?.string() ?: "{}"
                val fileResp = json.decodeFromString<FileContentResponse>(bodyStr)
                Result.success(fileResp)
            }
        } catch (e: Exception) {
            Result.failure(e)
        }
    }

    suspend fun fetchWeChatChatArchive(uri: String, cascadeId: String? = null): Result<WeChatChatArchive> = withContext(Dispatchers.IO) {
        val baseUrl = currentBaseUrl?.trimEnd('/') ?: return@withContext Result.failure(IllegalStateException("未配置网关地址".loc))
        val cleanUri = normalizeUriForGateway(uri)
        val encodedUri = URLEncoder.encode(cleanUri, "UTF-8")
        val cidParam = cascadeId?.takeIf { it.isNotBlank() }?.let { "&cascade_id=${URLEncoder.encode(it, "UTF-8")}" } ?: ""
        val url = "$baseUrl/api/v1/files/wechat-chat?uri=$encodedUri$cidParam"
        try {
            val req = buildAuthorizedRequest(url).get().build()
            client.newCall(req).await().use { response ->
                if (!response.isSuccessful) {
                    return@withContext Result.failure(RuntimeException("解析聊天记录失败: HTTP %d".loc(response.code)))
                }
                val bodyStr = response.body?.string() ?: "{}"
                val parsed = json.decodeFromString<WeChatChatArchive>(bodyStr)
                Result.success(parsed)
            }
        } catch (e: Exception) {
            Result.failure(e)
        }
    }

    fun buildZipEntryUrl(uri: String, entry: String, cascadeId: String? = null, thumb: Boolean = false): String {
        val baseUrl = currentBaseUrl?.trimEnd('/') ?: return ""
        val cleanUri = normalizeUriForGateway(uri)
        val encodedUri = URLEncoder.encode(cleanUri, "UTF-8")
        val encodedEntry = URLEncoder.encode(entry, "UTF-8")
        val cidParam = cascadeId?.takeIf { it.isNotBlank() }?.let { "&cascade_id=${URLEncoder.encode(it, "UTF-8")}" } ?: ""
        val thumbParam = if (thumb) "&thumb=1" else ""
        return "$baseUrl/api/v1/files/zip-entry?uri=$encodedUri&entry=$encodedEntry$cidParam$thumbParam"
    }

    private fun normalizeUriForGateway(raw: String): String {
        var clean = raw.trim()
        if (clean.startsWith("MEDIA:", ignoreCase = true)) {
            clean = clean.substring(6).trim()
        }
        if (clean.startsWith("file://")) {
            clean = clean.removePrefix("file://")
        }
        clean = clean.trim('`', '"', '\'', '(', ')', '[', ']', '<', '>')
        return try { URLDecoder.decode(clean, "UTF-8") } catch (_: Exception) { clean }
    }

    /**
     * Report session focus to gateway immediately on tap (0ms latency, fire-and-forget).
     * Synchronizes Mac desktop / hardware cascading cursor state.
     */
    fun notifySessionFocus(cascadeId: String) {
        if (cascadeId.isBlank()) return
        val baseUrl = currentBaseUrl ?: return
        val url = "$baseUrl/gateway/cascade/focus"
        val payload = buildJsonObject {
            put("cascadeId", cascadeId)
            put("source", "android")
        }
        try {
            val req = buildAuthorizedRequest(url)
                .post(payload.toString().toRequestBody(jsonMediaType))
                .build()
            client.newCall(req).enqueue(object : okhttp3.Callback {
                override fun onFailure(call: okhttp3.Call, e: java.io.IOException) {}
                override fun onResponse(call: okhttp3.Call, response: okhttp3.Response) {
                    response.close()
                }
            })
        } catch (_: Exception) {}
    }

    /**
     * Mark conversation as read locally and report upstream
     */
    suspend fun markConversationAsRead(cascadeId: String, customViewTime: Long? = null) = withContext(Dispatchers.IO) {
        val now = customViewTime ?: System.currentTimeMillis()
        prefs.setLastViewTime(cascadeId, now)

        // 1. Notify focus
        notifySessionFocus(cascadeId)

        // 2. Report upstream to LanguageServerService/UpdateConversationAnnotations
        val baseUrl = currentBaseUrl ?: return@withContext
        val url = "$baseUrl/api/exa.language_server_pb.LanguageServerService/UpdateConversationAnnotations"
        val nowIso = Instant.ofEpochMilli(now).toString()

        val payload = buildJsonObject {
            put("cascadeIds", buildJsonArray { add(JsonPrimitive(cascadeId)) })
            put("annotations", buildJsonObject {
                put("markedAsUnread", false)
                put("lastUserViewTime", nowIso)
            })
            put("mergeAnnotations", true)
        }

        try {
            val req = buildAuthorizedRequest(url)
                .post(payload.toString().toRequestBody(jsonMediaType))
                .build()
            client.newCall(req).await().close()
        } catch (_: Exception) {}
    }

    /**
     * Resolves a raw media/image URI into an authenticated, loadable HTTP URL for Coil / Image loaders.
     * Note: Image authentication is handled via Authorization Header (configured in Coil ImageLoader)
     * avoiding deprecated query tokens.
     */
    fun resolveMediaURL(raw: String): String {
        var clean = raw.trim()
        if (clean.startsWith("MEDIA:", ignoreCase = true)) {
            clean = clean.substring(6).trim()
        }
        clean = clean.trim('`', '"', '\'', '(', ')', '[', ']', '<', '>')

        val baseUrl = currentBaseUrl?.trimEnd('/') ?: ""

        if (clean.startsWith("http://") || clean.startsWith("https://")) {
            return clean
        }

        if (clean.startsWith("/static/")) {
            return "$baseUrl$clean"
        }

        if (clean.startsWith("file://")) {
            clean = clean.removePrefix("file://")
        }

        if (baseUrl.isBlank()) return clean

        val unescaped = try {
            URLDecoder.decode(clean, "UTF-8")
        } catch (_: Exception) {
            clean
        }

        val encodedUri = try {
            URLEncoder.encode(unescaped, "UTF-8")
        } catch (_: Exception) {
            unescaped
        }
        return "$baseUrl/api/v1/files/raw?uri=$encodedUri"
    }

    suspend fun downloadFile(
        uri: String,
        targetFile: java.io.File,
        cascadeId: String? = null,
        onProgress: ((progress: Float, written: Long, total: Long) -> Unit)? = null
    ): Result<java.io.File> = withContext(Dispatchers.IO) {
        try {
            // Document/image download sheet (DocumentPreviewSheet) always downloads the untouched original file.
            val resolvedUrl = stripImagePreview(resolveMediaURL(uri))
            val parsedTarget = resolvedUrl.toHttpUrlOrNull()
                ?: return@withContext Result.failure(IllegalArgumentException("无效的文件链接".loc))
            val isGatewayOrigin = isConfiguredGatewayOrigin(parsedTarget, prefs)
            val request = Request.Builder()
                .url(parsedTarget)
                .apply {
                    if (isGatewayOrigin) {
                        prefs.deviceToken?.takeIf { it.isNotBlank() }?.let { token ->
                            header("Authorization", "Bearer $token")
                        }
                    }
                }
                .build()

            val call = client.newCall(request)
            val cancelHandle = coroutineContext[Job]?.invokeOnCompletion { cause ->
                if (cause != null) {
                    try {
                        call.cancel()
                    } catch (_: Throwable) {}
                }
            }
            var tempFile: java.io.File? = null
            try {
                call.await().use { response ->
                    if (!response.isSuccessful) {
                        return@withContext Result.failure(RuntimeException("下载文件失败: HTTP %d".loc(response.code)))
                    }
                    val body = response.body ?: return@withContext Result.failure(RuntimeException("空响应体".loc))
                    val totalLength = body.contentLength()

                    targetFile.parentFile?.mkdirs()
                    val candidateTemp = java.io.File(targetFile.parentFile, "${targetFile.name}.${System.nanoTime()}.tmp")
                    tempFile = candidateTemp
                    candidateTemp.outputStream().use { output ->
                        body.byteStream().use { input ->
                            val buffer = ByteArray(8192)
                            var bytesWritten = 0L
                            var lastEmittedProgress = 0f
                            var lastEmitMs = 0L
                            var read: Int
                            while (input.read(buffer).also { read = it } != -1) {
                                coroutineContext.ensureActive()
                                output.write(buffer, 0, read)
                                bytesWritten += read
                                if (totalLength > 0 && onProgress != null) {
                                    val rawProgress = (bytesWritten.toFloat() / totalLength.toFloat()).coerceIn(0f, 1f)
                                    val monotonicProgress = maxOf(lastEmittedProgress, rawProgress)
                                    val now = android.os.SystemClock.elapsedRealtime()
                                    val shouldEmit = monotonicProgress >= 1f ||
                                        lastEmittedProgress == 0f ||
                                        (now - lastEmitMs >= 50L && monotonicProgress - lastEmittedProgress >= 0.005f) ||
                                        (monotonicProgress - lastEmittedProgress >= 0.05f)
                                    if (shouldEmit) {
                                        lastEmittedProgress = monotonicProgress
                                        lastEmitMs = now
                                        onProgress(monotonicProgress, bytesWritten, totalLength)
                                    }
                                }
                            }
                            output.flush()
                        }
                    }
                    coroutineContext.ensureActive()
                    if (targetFile.exists()) targetFile.delete()
                    if (!candidateTemp.renameTo(targetFile)) {
                        candidateTemp.copyTo(targetFile, overwrite = true)
                        candidateTemp.delete()
                    }
                    tempFile = null
                    Result.success(targetFile)
                }
            } catch (e: CancellationException) {
                tempFile?.delete()
                throw e
            } finally {
                cancelHandle?.dispose()
                tempFile?.delete()
            }
        } catch (e: CancellationException) {
            throw e
        } catch (e: Exception) {
            Result.failure(e)
        }
    }

    /**
     * Unpairs this device from the gateway and cleans up server-side state.
     */
    suspend fun unpair(): Result<Unit> = withContext(Dispatchers.IO) {
        val baseUrl = currentBaseUrl?.trim()?.trimEnd('/')
        val token = prefs.deviceToken
        if (baseUrl.isNullOrBlank() || token.isNullOrBlank()) {
            return@withContext Result.success(Unit)
        }

        try {
            val endpoint = "$baseUrl/api/v1/auth/unpair"
            val body = "{}".toRequestBody(jsonMediaType)
            val request = buildAuthorizedRequest(endpoint)
                .post(body)
                .build()

            client.newCall(request).await().use { response ->
                Log.d("ApiClient", "Unpair response code: ${response.code}")
            }
            Result.success(Unit)
        } catch (e: Exception) {
            Log.w("ApiClient", "Unpair call failed: ${e.message}")
            Result.failure(e)
        }
    }

    /**
     * Fetches revert preview for a cascade step.
     */
    suspend fun getRevertPreview(cascadeId: String, stepIndex: Int): Result<RevertPreviewResponse> = withContext(Dispatchers.IO) {
        val baseUrl = currentBaseUrl?.trim()?.trimEnd('/')
            ?: return@withContext Result.failure(IllegalStateException("网关地址未配置".loc))

        try {
            val endpoint = "$baseUrl/gateway/cascade/revert/preview"
            val reqPayload = json.encodeToString(RevertPreviewRequest(cascadeId = cascadeId, stepIndex = stepIndex))
            val request = buildAuthorizedRequest(endpoint)
                .post(reqPayload.toRequestBody(jsonMediaType))
                .build()

            client.newCall(request).await().use { response ->
                if (!response.isSuccessful) {
                    val errMsg = response.body?.string() ?: "HTTP ${response.code}"
                    return@withContext Result.failure(RuntimeException("获取撤回预览失败: %s".loc(errMsg)))
                }
                val respStr = response.body?.string() ?: ""
                val preview = json.decodeFromString<RevertPreviewResponse>(respStr)
                Result.success(preview)
            }
        } catch (e: Exception) {
            Result.failure(e)
        }
    }

    /**
     * Executes revert for a cascade step.
     */
    suspend fun executeRevert(cascadeId: String, stepIndex: Int, conversationOnly: Boolean = false): Result<Unit> = withContext(Dispatchers.IO) {
        val baseUrl = currentBaseUrl?.trim()?.trimEnd('/')
            ?: return@withContext Result.failure(IllegalStateException("网关地址未配置".loc))

        try {
            val endpoint = "$baseUrl/gateway/cascade/revert/execute"
            val reqPayload = json.encodeToString(RevertExecuteRequest(cascadeId = cascadeId, stepIndex = stepIndex, conversationOnly = conversationOnly))
            val request = buildAuthorizedRequest(endpoint)
                .post(reqPayload.toRequestBody(jsonMediaType))
                .build()

            client.newCall(request).await().use { response ->
                if (!response.isSuccessful) {
                    val errMsg = response.body?.string() ?: "HTTP ${response.code}"
                    return@withContext Result.failure(RuntimeException("执行撤回失败: %s".loc(errMsg)))
                }
                Result.success(Unit)
            }
        } catch (e: Exception) {
            Result.failure(e)
        }
    }

    // region 改动 diff / Git / 搜索 / 导出

    /** POST 一个 JSON 到网关并返回响应文本；失败时优先取网关 {"error": "..."} 里的说明。 */
    private suspend fun postGatewayJson(path: String, payload: String, failurePrefix: String): Result<String> =
        withContext(Dispatchers.IO) {
            val baseUrl = currentBaseUrl?.trim()?.trimEnd('/')
                ?: return@withContext Result.failure(IllegalStateException("网关地址未配置".loc))
            try {
                val request = buildAuthorizedRequest("$baseUrl$path")
                    .post(payload.toRequestBody(jsonMediaType))
                    .build()
                client.newCall(request).await().use { response ->
                    val body = response.body?.string().orEmpty()
                    if (!response.isSuccessful) {
                        val msg = runCatching {
                            json.parseToJsonElement(body).jsonObject["error"]?.jsonPrimitive?.contentOrNull
                                ?: json.parseToJsonElement(body).jsonObject["message"]?.jsonPrimitive?.contentOrNull
                        }.getOrNull() ?: body.ifBlank { "HTTP ${response.code}" }
                        return@withContext Result.failure(RuntimeException("$failurePrefix: $msg"))
                    }
                    Result.success(body)
                }
            } catch (e: Exception) {
                Result.failure(e)
            }
        }

    @Volatile
    private var lastCompatRefreshAt = 0L

    /**
     * 刷新网关的升级自检结果（GET /gateway/status 的 compat 字段）。失败时保持上一次的结果不变。
     */
    suspend fun refreshGatewayCompat(force: Boolean = false): Result<GatewayCompat> = withContext(Dispatchers.IO) {
        val baseUrl = currentBaseUrl?.trim()?.trimEnd('/')
            ?: return@withContext Result.failure(IllegalStateException("网关地址未配置".loc))
        // 列表会被频繁轮询；自检结果只在 Antigravity 重启/升级时才会变，30 秒内不重复请求
        val now = System.currentTimeMillis()
        if (!force && now - lastCompatRefreshAt < 30_000L) return@withContext Result.success(GatewayCompatStore.state.value)
        lastCompatRefreshAt = now
        val reqEpoch = GatewayRuntimeState.epoch
        try {
            val request = buildAuthorizedRequest("$baseUrl/gateway/status").get().build()
            client.newCall(request).await().use { response ->
                if (!response.isSuccessful) return@withContext Result.failure(RuntimeException("HTTP ${response.code}"))
                if (GatewayRuntimeState.epoch != reqEpoch) return@withContext Result.success(GatewayCompatStore.state.value)
                val bodyText = response.body?.string().orEmpty()
                val compat = GatewayCompat.fromStatusJson(bodyText)
                GatewayCompatStore.update(compat)
                runCatching {
                    val obj = org.json.JSONObject(bodyText)
                    val gwId = obj.optString("gateway_id").takeIf { it.isNotBlank() }
                    val gwName = obj.optString("gateway_name").takeIf { it.isNotBlank() }
                    val plat = obj.optString("platform").takeIf { it.isNotBlank() }
                    val store = GatewayProfileStore.sharedOrNull()
                    val activeId = store?.activeProfileId
                    if (store != null && activeId != null) {
                        store.backfillGatewayMetadata(activeId, gwId, gwName, plat)
                    }
                    if (!plat.isNullOrBlank() && prefs.gatewayPlatform != plat) {
                        prefs.gatewayPlatform = plat
                    }
                }
                Result.success(compat)
            }
        } catch (e: Exception) {
            Result.failure(e)
        }
    }

    /** 手机端 "/" 菜单（系统命令 + 技能）。 */
    suspend fun getSlashCommands(refresh: Boolean = false): Result<List<SlashCommandOption>> =
        postGatewayJson(
            if (refresh) "/gateway/slash-commands?refresh=1" else "/gateway/slash-commands",
            "{}",
            "获取斜杠命令失败".loc
        ).mapCatching { json.decodeFromString<SlashCommandsResponse>(it).commands }

    /** 可选模型目录（网关从 language_server 实时取，失败时返回内置兜底）。 */
    suspend fun getModels(refresh: Boolean = false): Result<ModelsResponse> =
        postGatewayJson(
            if (refresh) "/gateway/models?refresh=1" else "/gateway/models",
            "{}",
            "获取模型列表失败".loc
        ).mapCatching { json.decodeFromString<ModelsResponse>(it) }

    /** 本会话累计改动（正向 diff）。fromStepIndex 非空时只取该步骤起的改动。 */
    suspend fun getCascadeChanges(cascadeId: String, fromStepIndex: Int? = null): Result<CascadeChangesResponse> =
        postGatewayJson(
            "/gateway/cascade/changes",
            json.encodeToString(CascadeChangesRequest(cascadeId = cascadeId, fromStepIndex = fromStepIndex)),
            "获取本会话改动失败".loc
        ).mapCatching { json.decodeFromString<CascadeChangesResponse>(it) }

    /** 会话章节语义标题（Antigravity 2.30.0 GetChapterTitles，经网关 /gateway/cascade/chapters 聚合并轮询收敛）。 */
    suspend fun getChapterTitles(
        cascadeId: String,
        ranges: List<CascadeStepRange>
    ): Result<List<CascadeChapterTitleItem>> =
        postGatewayJson(
            "/gateway/cascade/chapters",
            json.encodeToString(CascadeChaptersRequest(cascadeId = cascadeId, ranges = ranges)),
            "获取章节标题失败".loc
        ).mapCatching { json.decodeFromString<CascadeChaptersResponse>(it).chapters }

    suspend fun getGitStatus(cascadeId: String): Result<GitStatusResponse> =
        postGatewayJson(
            "/gateway/git/status",
            json.encodeToString(GitCascadeRequest(cascadeId = cascadeId)),
            "获取 Git 状态失败".loc
        ).mapCatching { json.decodeFromString<GitStatusResponse>(it) }

    suspend fun gitCommit(
        cascadeId: String,
        message: String,
        paths: List<String>,
        push: Boolean
    ): Result<GitCommitResponse> =
        postGatewayJson(
            "/gateway/git/commit",
            json.encodeToString(GitCommitRequest(cascadeId = cascadeId, message = message, paths = paths, push = push)),
            "提交失败".loc
        ).mapCatching { json.decodeFromString<GitCommitResponse>(it) }

    suspend fun gitPush(cascadeId: String): Result<GitCommitResponse> =
        postGatewayJson(
            "/gateway/git/push",
            json.encodeToString(GitCascadeRequest(cascadeId = cascadeId)),
            "推送失败".loc
        ).mapCatching { json.decodeFromString<GitCommitResponse>(it) }

    /** 会话内容全文搜索（language_server SearchConversations，经网关透传）。 */
    suspend fun searchConversations(query: String): Result<List<ConversationSearchResult>> =
        postGatewayJson(
            "/api/exa.language_server_pb.LanguageServerService/SearchConversations",
            buildJsonObject { put("query", query) }.toString(),
            "搜索失败".loc
        ).mapCatching { json.decodeFromString<ConversationSearchResponse>(it).results }

    /** 导出会话为 Markdown（language_server ConvertTrajectoryToMarkdown，经网关透传）。 */
    suspend fun exportConversationMarkdown(cascadeId: String): Result<String> =
        postGatewayJson(
            "/api/exa.language_server_pb.LanguageServerService/ConvertTrajectoryToMarkdown",
            buildJsonObject { put("conversationId", cascadeId) }.toString(),
            "导出失败".loc
        ).mapCatching { json.decodeFromString<MarkdownExportResponse>(it).markdown }

    /** 获取当前挂载的所有 MCP 服务状态及工具清单（language_server GetMcpServerStates，经网关透传）。 */
    suspend fun fetchMcpServerStates(): Result<List<McpServerState>> =
        postGatewayJson(
            "/api/exa.language_server_pb.LanguageServerService/GetMcpServerStates",
            "{}",
            "获取 MCP 服务状态失败".loc
        ).mapCatching { json.decodeFromString<GetMcpServerStatesResponse>(it).states ?: emptyList() }

    /** 获取当前挂载的所有技能清单（language_server GetAllSkills，经网关透传）。 */
    suspend fun fetchAllSkills(): Result<List<SkillInfo>> =
        postGatewayJson(
            "/api/exa.language_server_pb.LanguageServerService/GetAllSkills",
            "{}",
            "获取技能清单失败".loc
        ).mapCatching { json.decodeFromString<GetAllSkillsResponse>(it).skills ?: emptyList() }

    // endregion

    @kotlinx.serialization.Serializable
    data class AlertPreferencesPayload(
        @kotlinx.serialization.SerialName("task_completion") val taskCompletion: Boolean = true,
        @kotlinx.serialization.SerialName("task_stall") val taskStall: Boolean = true,
        @kotlinx.serialization.SerialName("cache_miss") val cacheMiss: Boolean = true,
        @kotlinx.serialization.SerialName("cache_miss_mode") val cacheMissMode: String = "shadow",
        @kotlinx.serialization.SerialName("quota_low") val quotaLow: Boolean = true,
        @kotlinx.serialization.SerialName("quota_bonus_reset") val quotaBonusReset: Boolean = true,
        @kotlinx.serialization.SerialName("quota_anomaly") val quotaAnomaly: Boolean = true,
        @kotlinx.serialization.SerialName("stall_minutes") val stallMinutes: Int = 8
    ) {
        val isCacheMissLive: Boolean
            get() = cacheMissMode.equals("live", ignoreCase = true)

        val visibleTotalCount: Int
            get() = if (isCacheMissLive) 6 else 5

        val enabledCount: Int
            get() = buildList {
                add(taskCompletion)
                add(taskStall)
                if (isCacheMissLive) add(cacheMiss)
                add(quotaLow)
                add(quotaBonusReset)
                add(quotaAnomaly)
            }.count { it }

        fun withUpdated(key: String, value: Boolean): AlertPreferencesPayload = when (key) {
            "task_completion" -> copy(taskCompletion = value)
            "task_stall" -> copy(taskStall = value)
            "cache_miss" -> copy(cacheMiss = value)
            "quota_low" -> copy(quotaLow = value)
            "quota_bonus_reset" -> copy(quotaBonusReset = value)
            "quota_anomaly" -> copy(quotaAnomaly = value)
            else -> this
        }
    }

    suspend fun fetchAlertPreferences(): Result<AlertPreferencesPayload> = withContext(Dispatchers.IO) {
        val baseUrl = currentBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址".loc))
        val url = "$baseUrl/api/v1/push/preferences"
        try {
            val req = buildAuthorizedRequest(url).get().build()
            client.newCall(req).await().use { response ->
                val bodyStr = response.body?.string().orEmpty()
                if (!response.isSuccessful) {
                    return@withContext Result.failure(RuntimeException("获取提醒设置失败 (%d)".loc(response.code)))
                }
                Result.success(json.decodeFromString<AlertPreferencesPayload>(bodyStr))
            }
        } catch (e: Exception) {
            Log.w("ApiClient", "fetchAlertPreferences error: ${e.message}")
            Result.failure(e)
        }
    }

    suspend fun updateAlertPreference(key: String, value: Boolean): Result<AlertPreferencesPayload> = withContext(Dispatchers.IO) {
        val baseUrl = currentBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址".loc))
        val url = "$baseUrl/api/v1/push/preferences"
        val payload = buildJsonObject { put(key, value) }
        try {
            val req = buildAuthorizedRequest(url)
                .post(payload.toString().toRequestBody(jsonMediaType))
                .build()
            client.newCall(req).await().use { response ->
                val bodyStr = response.body?.string().orEmpty()
                if (!response.isSuccessful) {
                    return@withContext Result.failure(RuntimeException("保存提醒设置失败 (%d)".loc(response.code)))
                }
                Result.success(json.decodeFromString<AlertPreferencesPayload>(bodyStr))
            }
        } catch (e: Exception) {
            Log.w("ApiClient", "updateAlertPreference error: ${e.message}")
            Result.failure(e)
        }
    }

    suspend fun fetchAIAssistStatus(): Result<AIAssistStatusPayload> = withContext(Dispatchers.IO) {
        val baseUrl = currentBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址".loc))
        val url = "$baseUrl/gateway/ai-assist/prefs"
        try {
            val req = buildAuthorizedRequest(url).get().build()
            client.newCall(req).await().use { response ->
                val bodyStr = response.body?.string().orEmpty()
                if (!response.isSuccessful) {
                    return@withContext Result.failure(RuntimeException("HTTP ${response.code}"))
                }
                Result.success(json.decodeFromString<AIAssistStatusPayload>(bodyStr))
            }
        } catch (e: Exception) {
            Log.w("ApiClient", "fetchAIAssistStatus error: ${e.message}")
            Result.failure(e)
        }
    }

    suspend fun patchAIAssistPrefs(patch: JsonObject): Result<AIAssistStatusPayload> = withContext(Dispatchers.IO) {
        val baseUrl = currentBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址".loc))
        val url = "$baseUrl/gateway/ai-assist/prefs"
        try {
            val req = buildAuthorizedRequest(url)
                .post(patch.toString().toRequestBody(jsonMediaType))
                .build()
            client.newCall(req).await().use { response ->
                val bodyStr = response.body?.string().orEmpty()
                if (!response.isSuccessful) {
                    return@withContext Result.failure(RuntimeException("HTTP ${response.code}"))
                }
                Result.success(json.decodeFromString<AIAssistStatusPayload>(bodyStr))
            }
        } catch (e: Exception) {
            Log.w("ApiClient", "patchAIAssistPrefs error: ${e.message}")
            Result.failure(e)
        }
    }

    suspend fun translateExistingTitles(targetLang: String, targetLangName: String): Result<AIAssistTranslateResultPayload> = withContext(Dispatchers.IO) {
        val baseUrl = currentBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址".loc))
        val url = "$baseUrl/gateway/ai-assist/translate-titles"
        val payload = buildJsonObject {
            put("targetLang", targetLang)
            put("targetLangName", targetLangName)
            put("includeArchived", false)
        }
        try {
            val batchClient = client.newBuilder()
                .readTimeout(60, TimeUnit.SECONDS)
                .writeTimeout(15, TimeUnit.SECONDS)
                .build()
            val req = buildAuthorizedRequest(url)
                .post(payload.toString().toRequestBody(jsonMediaType))
                .build()
            batchClient.newCall(req).await().use { response ->
                val bodyStr = response.body?.string().orEmpty()
                if (!response.isSuccessful) {
                    return@withContext Result.failure(RuntimeException("HTTP ${response.code}"))
                }
                Result.success(json.decodeFromString<AIAssistTranslateResultPayload>(bodyStr))
            }
        } catch (e: Exception) {
            Log.w("ApiClient", "translateExistingTitles error: ${e.message}")
            Result.failure(e)
        }
    }

    suspend fun restoreOriginalTitles(): Result<AIAssistRestoreResultPayload> = withContext(Dispatchers.IO) {
        val baseUrl = currentBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址".loc))
        val url = "$baseUrl/gateway/ai-assist/restore-titles"
        try {
            val batchClient = client.newBuilder()
                .readTimeout(30, TimeUnit.SECONDS)
                .build()
            val req = buildAuthorizedRequest(url)
                .post("{}".toRequestBody(jsonMediaType))
                .build()
            batchClient.newCall(req).await().use { response ->
                val bodyStr = response.body?.string().orEmpty()
                if (!response.isSuccessful) {
                    return@withContext Result.failure(RuntimeException("HTTP ${response.code}"))
                }
                Result.success(json.decodeFromString<AIAssistRestoreResultPayload>(bodyStr))
            }
        } catch (e: Exception) {
            Log.w("ApiClient", "restoreOriginalTitles error: ${e.message}")
            Result.failure(e)
        }
    }

    data class GatewayStatusMeta(
        val gatewayId: String? = null,
        val gatewayName: String? = null,
        val platform: String? = null
    )

    private fun currentPushLangCode(): String = when (com.antigravity.mobile.ui.util.L10n.currentLang) {
        com.antigravity.mobile.ui.util.L10n.AppLang.ZH -> "zh"
        com.antigravity.mobile.ui.util.L10n.AppLang.JA -> "ja"
        com.antigravity.mobile.ui.util.L10n.AppLang.EN -> "en"
    }

    private fun buildContextRequest(url: String, context: GatewayRequestContext): Request.Builder {
        val builder = Request.Builder()
            .url(url)
            .tag(E2eeContextTag::class.java, E2eeContextTag(context))
        context.deviceToken?.trim()?.takeIf { it.isNotEmpty() }?.let { tok ->
            builder.header("Authorization", "Bearer $tok")
            builder.header("x-device-token", tok)
        }
        return builder
    }

    private fun isAvoidLanNow(): Boolean =
        (connectionManager?.isCellular ?: false) || !(connectionManager?.isWifi ?: true)

    /**
     * Registers the device push token (FCM / remote push) with the active gateway (or fans out across all paired gateways).
     */
    suspend fun registerPushToken(token: String, platform: String = "android"): Result<Unit> = withContext(Dispatchers.IO) {
        val store = GatewayProfileStore.sharedOrNull()
        if (store != null && store.profiles.isNotEmpty()) {
            syncPushTokenToAllGateways(token, platform)
            return@withContext Result.success(Unit)
        }
        val baseUrl = currentBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址".loc))
        val url = "$baseUrl/api/v1/device/push-token"
        val payload = buildJsonObject {
            put("platform", platform)
            put("token", token)
            put("fcm_token", token)
            put("lang", currentPushLangCode())
            prefs.deviceId?.let { put("device_id", it) }
        }

        try {
            val req = buildAuthorizedRequest(url)
                .post(payload.toString().toRequestBody(jsonMediaType))
                .build()

            client.newCall(req).await().use { response ->
                if (!response.isSuccessful) {
                    return@withContext Result.failure(RuntimeException("注册推送 Token 失败 (%d)".loc(response.code)))
                }
                Result.success(Unit)
            }
        } catch (e: Exception) {
            Log.w("ApiClient", "registerPushToken error: ${e.message}")
            Result.failure(e)
        }
    }

    suspend fun syncPushTokenToAllGateways(fcmToken: String, platform: String = "android") = withContext(Dispatchers.IO) {
        val cleanToken = fcmToken.trim()
        if (cleanToken.isEmpty()) return@withContext
        val store = GatewayProfileStore.getOrCreate(context, prefs)
        val contexts = store.allRequestContexts()
        if (contexts.isEmpty()) return@withContext
        val showLabel = contexts.size > 1
        for (ctx in contexts) {
            registerPushTokenForContext(
                fcmToken = cleanToken,
                context = ctx,
                showGatewayLabel = showLabel,
                gatewayLabel = ctx.displayName,
                platform = platform
            )
        }
    }

    suspend fun registerPushTokenForContext(
        fcmToken: String,
        context: GatewayRequestContext,
        showGatewayLabel: Boolean,
        gatewayLabel: String,
        platform: String = "android"
    ): Result<Unit> = withContext(Dispatchers.IO) {
        val candidates = context.candidateUrls(isAvoidLanNow())
        if (candidates.isEmpty()) return@withContext Result.failure(IllegalStateException("No gateway URL"))
        val payload = buildJsonObject {
            put("platform", platform)
            put("token", fcmToken)
            put("fcm_token", fcmToken)
            put("lang", currentPushLangCode())
            put("show_gateway_label", showGatewayLabel)
            put("gateway_label", gatewayLabel)
            context.deviceId?.takeIf { it.isNotBlank() }?.let { put("device_id", it) }
        }.toString()

        var lastErr: Exception? = null
        for (base in candidates) {
            try {
                val req = buildContextRequest("$base/api/v1/device/push-token", context)
                    .post(payload.toRequestBody(jsonMediaType))
                    .build()
                val callClient = if (ConnectionManager.isLanHost(ConnectionManager.extractHost(base))) {
                    client.newBuilder().connectTimeout(2, TimeUnit.SECONDS).readTimeout(4, TimeUnit.SECONDS).build()
                } else {
                    client
                }
                callClient.newCall(req).await().use { resp ->
                    if (resp.isSuccessful) {
                        return@withContext Result.success(Unit)
                    }
                    lastErr = RuntimeException("HTTP ${resp.code}")
                }
            } catch (e: Exception) {
                lastErr = e
            }
        }
        Result.failure(lastErr ?: RuntimeException("registerPushTokenForContext failed"))
    }

    suspend fun unregisterPushTokenForContext(context: GatewayRequestContext): Result<Unit> = withContext(Dispatchers.IO) {
        val candidates = context.candidateUrls(isAvoidLanNow())
        if (candidates.isEmpty()) return@withContext Result.success(Unit)
        val payload = buildJsonObject {
            put("platform", "android")
            context.deviceId?.takeIf { it.isNotBlank() }?.let { put("device_id", it) }
        }.toString()
        for (base in candidates) {
            try {
                val req = buildContextRequest("$base/api/v1/device/push-token", context)
                    .delete(payload.toRequestBody(jsonMediaType))
                    .build()
                client.newCall(req).await().use { resp ->
                    if (resp.isSuccessful) return@withContext Result.success(Unit)
                }
            } catch (_: Exception) {}
        }
        Result.success(Unit)
    }

    suspend fun unpairContext(context: GatewayRequestContext): Result<Unit> = withContext(Dispatchers.IO) {
        val candidates = context.candidateUrls(isAvoidLanNow())
        for (base in candidates) {
            try {
                val req = buildContextRequest("$base/api/v1/auth/unpair", context)
                    .post("{}".toRequestBody(jsonMediaType))
                    .build()
                client.newCall(req).await().use { resp ->
                    if (resp.isSuccessful) return@withContext Result.success(Unit)
                }
            } catch (_: Exception) {}
        }
        Result.success(Unit)
    }

    suspend fun fetchGatewaySummary(
        context: GatewayRequestContext
    ): Pair<GatewaySummarySnapshot, GatewayStatusMeta?> = withContext(Dispatchers.IO) {
        val candidates = context.candidateUrls(isAvoidLanNow())
        for (base in candidates) {
            val isLan = ConnectionManager.isLanHost(ConnectionManager.extractHost(base))
            val timeoutMs = if (isLan) 1500L else 3500L
            val summaryClient = client.newBuilder()
                .connectTimeout(timeoutMs, TimeUnit.MILLISECONDS)
                .readTimeout(timeoutMs, TimeUnit.MILLISECONDS)
                .build()
            try {
                val req = buildContextRequest("$base/gateway/status?summary=1", context).get().build()
                summaryClient.newCall(req).await().use { resp ->
                    if (resp.isSuccessful) {
                        val text = resp.body?.string().orEmpty()
                        val obj = runCatching { org.json.JSONObject(text) }.getOrNull()
                        val gwId = obj?.optString("gateway_id")?.takeIf { it.isNotBlank() }
                        val gwName = obj?.optString("gateway_name")?.takeIf { it.isNotBlank() }
                        val plat = obj?.optString("platform")?.takeIf { it.isNotBlank() }
                        val meta = GatewayStatusMeta(gwId, gwName, plat)
                        val sumObj = obj?.optJSONObject("summary")
                        if (sumObj != null) {
                            return@withContext GatewaySummarySnapshot(
                                profileId = context.profileId,
                                runningCount = sumObj.optInt("running_count", 0),
                                actionCount = sumObj.optInt("action_count", 0),
                                unreadCount = sumObj.optInt("unread_count", 0),
                                totalActive = sumObj.optInt("total_active", 0),
                                isReachable = true,
                                isRefreshing = false,
                                gatewayName = gwName ?: context.displayName,
                                updatedAtMs = System.currentTimeMillis()
                            ) to meta
                        }
                        // Fallback for older gateways without `summary`: fetch trajectory list once.
                        val fallbackSnap = fetchLegacySummarySnapshot(base, context, summaryClient, gwName)
                        if (fallbackSnap != null) {
                            return@withContext fallbackSnap to meta
                        }
                        return@withContext GatewaySummarySnapshot(
                            profileId = context.profileId,
                            isReachable = true,
                            isRefreshing = false,
                            gatewayName = gwName ?: context.displayName,
                            updatedAtMs = System.currentTimeMillis()
                        ) to meta
                    }
                }
            } catch (_: Exception) {}
        }
        GatewaySummarySnapshot(
            profileId = context.profileId,
            isReachable = false,
            isRefreshing = false,
            gatewayName = context.displayName,
            updatedAtMs = System.currentTimeMillis()
        ) to null
    }

    private suspend fun fetchLegacySummarySnapshot(
        base: String,
        context: GatewayRequestContext,
        summaryClient: OkHttpClient,
        gwName: String?
    ): GatewaySummarySnapshot? {
        return try {
            val req = buildContextRequest(
                "$base/api/exa.language_server_pb.LanguageServerService/GetAllCascadeTrajectories",
                context
            ).post("{}".toRequestBody(jsonMediaType)).build()
            summaryClient.newCall(req).await().use { resp ->
                if (!resp.isSuccessful) return null
                val body = resp.body?.string().orEmpty()
                val parsed = json.decodeFromString<GetAllCascadeTrajectoriesResponse>(body)
                val map = parsed.trajectorySummaries ?: emptyMap()
                var actionCount = 0
                var runningCount = 0
                var totalActive = 0
                for ((_, s) in map) {
                    if (s.isSubagent || s.annotations?.archived == true) continue
                    totalActive++
                    val st = s.status?.uppercase().orEmpty()
                    if (st.contains("WAITING") || st.contains("ACTION")) {
                        actionCount++
                    } else if (st.contains("RUNNING")) {
                        runningCount++
                    }
                }
                GatewaySummarySnapshot(
                    profileId = context.profileId,
                    runningCount = runningCount,
                    actionCount = actionCount,
                    unreadCount = 0,
                    totalActive = totalActive,
                    isReachable = true,
                    isRefreshing = false,
                    gatewayName = gwName ?: context.displayName,
                    updatedAtMs = System.currentTimeMillis()
                )
            }
        } catch (_: Exception) {
            null
        }
    }

    suspend fun fetchMessages(
        cascadeId: String,
        limit: Int = 5,
        context: GatewayRequestContext
    ): Result<StreamUpdatePayload> = withContext(Dispatchers.IO) {
        if (!CacheManager.isValidCascadeId(cascadeId)) {
            return@withContext Result.failure(IllegalArgumentException("无效的会话 ID".loc))
        }
        val candidates = context.candidateUrls(isAvoidLanNow())
        if (candidates.isEmpty()) return@withContext Result.failure(IllegalStateException("未配置网关地址".loc))
        val encodedCid = URLEncoder.encode(cascadeId, "UTF-8")
        var lastErr: Exception? = null
        for (base in candidates) {
            try {
                val req = buildContextRequest("$base/gateway/cascade/messages?cascadeId=$encodedCid&limit=$limit", context)
                    .get()
                    .build()
                client.newCall(req).await().use { resp ->
                    if (resp.isSuccessful) {
                        val body = resp.body?.string() ?: "{}"
                        return@withContext Result.success(json.decodeFromString<StreamUpdatePayload>(body))
                    }
                    lastErr = RuntimeException("HTTP ${resp.code}")
                }
            } catch (e: Exception) {
                lastErr = e
            }
        }
        Result.failure(lastErr ?: RuntimeException("fetchMessages failed"))
    }

    suspend fun submitInteraction(
        cascadeId: String,
        trajectoryId: String = "",
        stepIndex: Int = 0,
        type: String = "",
        optionId: String? = null,
        scope: Int = 1,
        allow: Boolean = true,
        writeInResponse: String = "",
        skipped: Boolean = false,
        target: String? = null,
        questionResponses: List<QuestionResponse>? = null,
        context: GatewayRequestContext
    ): Result<Unit> = withContext(Dispatchers.IO) {
        val candidates = context.candidateUrls(isAvoidLanNow())
        if (candidates.isEmpty()) return@withContext Result.failure(IllegalStateException("未配置网关地址".loc))
        val bodyStr = json.encodeToString(
            InteractionSubmitRequest(
                cascadeId = cascadeId,
                trajectoryId = trajectoryId,
                stepIndex = stepIndex,
                type = type,
                optionId = optionId,
                scope = scope,
                allow = allow,
                writeInResponse = writeInResponse,
                skipped = skipped,
                target = target,
                questionResponses = questionResponses
            )
        )
        var lastErr: Exception? = null
        for (base in candidates) {
            try {
                val req = buildContextRequest("$base/gateway/cascade/interaction", context)
                    .post(bodyStr.toRequestBody(jsonMediaType))
                    .build()
                client.newCall(req).await().use { resp ->
                    if (resp.isSuccessful) return@withContext Result.success(Unit)
                    lastErr = RuntimeException("HTTP ${resp.code}")
                }
            } catch (e: Exception) {
                lastErr = e
            }
        }
        Result.failure(lastErr ?: RuntimeException("submitInteraction failed"))
    }

    suspend fun proceedArtifact(
        cascadeId: String,
        artifactUri: String,
        model: String? = null,
        context: GatewayRequestContext
    ): Result<Unit> = withContext(Dispatchers.IO) {
        val candidates = context.candidateUrls(isAvoidLanNow())
        if (candidates.isEmpty()) return@withContext Result.failure(IllegalStateException("未配置网关地址".loc))
        val comment = buildJsonObject {
            put("artifactUri", artifactUri)
            put("scope", buildJsonObject {
                put("case", "fullFile")
                put("value", buildJsonObject {})
            })
            put("approvalStatus", 1)
            put("comment", "")
        }
        val payload = buildJsonObject {
            put("cascadeId", cascadeId)
            put("items", JsonArray(emptyList()))
            put("artifactComments", buildJsonArray { add(comment) })
            if (!model.isNullOrBlank()) put("model", model)
        }.toString()
        var lastErr: Exception? = null
        for (base in candidates) {
            try {
                val reqBuilder = buildContextRequest("$base/api/exa.language_server_pb.LanguageServerService/SendUserCascadeMessage", context)
                    .post(payload.toRequestBody(jsonMediaType))
                if (!model.isNullOrBlank()) reqBuilder.addHeader("X-Antigravity-Model", model)
                client.newCall(reqBuilder.build()).await().use { resp ->
                    if (resp.isSuccessful) return@withContext Result.success(Unit)
                    lastErr = RuntimeException("HTTP ${resp.code}")
                }
            } catch (e: Exception) {
                lastErr = e
            }
        }
        Result.failure(lastErr ?: RuntimeException("proceedArtifact failed"))
    }

    suspend fun cancelInvocation(
        cascadeId: String,
        killBackgroundTasks: Boolean? = null,
        context: GatewayRequestContext
    ): Result<Unit> = withContext(Dispatchers.IO) {
        val candidates = context.candidateUrls(isAvoidLanNow())
        if (candidates.isEmpty()) return@withContext Result.failure(IllegalStateException("未配置网关地址".loc))
        val payload = buildJsonObject {
            put("cascadeId", cascadeId)
            if (killBackgroundTasks != null) put("killBackgroundTasks", killBackgroundTasks)
        }.toString()
        var lastErr: Exception? = null
        for (base in candidates) {
            try {
                val req = buildContextRequest("$base/api/exa.language_server_pb.LanguageServerService/CancelCascadeInvocation", context)
                    .post(payload.toRequestBody(jsonMediaType))
                    .build()
                client.newCall(req).await().use { resp ->
                    if (resp.isSuccessful) return@withContext Result.success(Unit)
                    lastErr = RuntimeException("HTTP ${resp.code}")
                }
            } catch (e: Exception) {
                lastErr = e
            }
        }
        Result.failure(lastErr ?: RuntimeException("cancelInvocation failed"))
    }

    /**
     * Exchanges the device token for a short-lived one-time ticket via POST /api/v1/auth/ws-ticket.
     */
    suspend fun getWsTicket(): Result<String> = withContext(Dispatchers.IO) {
        val baseUrl = currentBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址".loc))
        val url = "$baseUrl/api/v1/auth/ws-ticket"
        try {
            val req = buildAuthorizedRequest(url)
                .post("{}".toRequestBody("application/json".toMediaType()))
                .build()

            client.newCall(req).await().use { response ->
                if (!response.isSuccessful) {
                    return@withContext Result.failure(RuntimeException("获取 ws-ticket 失败 (%d)".loc(response.code)))
                }
                val body = response.body?.string() ?: return@withContext Result.failure(RuntimeException("Empty body"))
                val ticketResp = JsonConfig.instance.decodeFromString<WsTicketResponse>(body)
                Result.success(ticketResp.ticket)
            }
        } catch (e: Exception) {
            Log.w("ApiClient", "getWsTicket error: ${e.message}")
            Result.failure(e)
        }
    }

    /**
     * Uploads a local file to the gateway (POST /api/v1/attachments) as a raw streamed body.
     * [onProgress] receives 0f..1f as bytes are written.
     */
    suspend fun uploadAttachment(
        file: File,
        displayName: String,
        onProgress: (Float) -> Unit = {}
    ): Result<UploadedAttachment> = withContext(Dispatchers.IO) {
        val baseUrl = currentBaseUrl?.trim()?.trimEnd('/')
            ?: return@withContext Result.failure(IllegalStateException("未配置网关地址".loc))
        try {
            val uploadUrl = "$baseUrl/api/v1/attachments"
            val parsedUploadUrl = runCatching { java.net.URI(uploadUrl) }.getOrNull()
            val willEncryptE2ee = parsedUploadUrl != null && E2eeCrypto.shouldEncrypt(
                parsedUploadUrl.scheme ?: "",
                parsedUploadUrl.host ?: "",
                parsedUploadUrl.rawPath ?: "/api/v1/attachments",
                prefs
            )
            val total = file.length()
            val body = object : okhttp3.RequestBody() {
                override fun contentType() = "application/octet-stream".toMediaType()
                override fun contentLength() = total
                override fun writeTo(sink: okio.BufferedSink) {
                    file.source().use { source ->
                        var written = 0L
                        val buf = okio.Buffer()
                        while (true) {
                            val n = source.read(buf, 64 * 1024L)
                            if (n == -1L) break
                            sink.write(buf, n)
                            written += n
                            // When E2EE wraps the request into an in-memory buffer first, defer progress to
                            // E2eeUploadProgressTag so progress tracks actual encrypted wire transmission.
                            if (!willEncryptE2ee && total > 0) {
                                onProgress(((written.toFloat() / total) * 0.95f).coerceIn(0f, 0.95f))
                            }
                        }
                    }
                }
            }
            val request = buildAuthorizedRequest(uploadUrl)
                .header("X-File-Name", android.net.Uri.encode(displayName))
                .tag(E2eeUploadProgressTag::class.java, E2eeUploadProgressTag(onProgress))
                .post(body)
                .build()
            val uploadClient = client.newBuilder()
                .writeTimeout(120, TimeUnit.SECONDS)
                .readTimeout(120, TimeUnit.SECONDS)
                .build()
            uploadClient.newCall(request).await().use { response ->
                val respStr = response.body?.string().orEmpty()
                if (!response.isSuccessful) {
                    val msg = runCatching {
                        json.parseToJsonElement(respStr).jsonObject["message"]?.jsonPrimitive?.content
                    }.getOrNull() ?: "HTTP ${response.code}"
                    return@withContext Result.failure(RuntimeException(msg))
                }
                Result.success(json.decodeFromString<UploadedAttachment>(respStr))
            }
        } catch (e: Exception) {
            Result.failure(e)
        }
    }

    /**
     * 获取全局所有正在运行的后台任务（language_server ListBackgroundTasks，经网关补全会话与工作区信息）
     */
    suspend fun fetchGlobalBackgroundTasks(): Result<List<GlobalBackgroundTaskItem>> = withContext(Dispatchers.IO) {
        val baseUrl = currentBaseUrl ?: return@withContext Result.failure(RuntimeException("未配置网关地址".loc))
        try {
            val request = buildAuthorizedRequest("$baseUrl/gateway/tasks/background")
                .get()
                .build()
            client.newCall(request).await().use { response ->
                val respStr = response.body?.string().orEmpty()
                if (!response.isSuccessful) {
                    return@withContext Result.failure(RuntimeException("HTTP ${response.code}"))
                }
                val parsed = json.decodeFromString<GlobalBackgroundTasksResponse>(respStr)
                Result.success(parsed.tasks)
            }
        } catch (e: Exception) {
            Result.failure(e)
        }
    }

    /**
     * 读取后台任务日志文件的最新尾部内容（/gateway/tasks/background/log）
     */
    suspend fun fetchBackgroundTaskLog(
        conversationId: String,
        taskId: String,
        logUri: String?
    ): Result<BackgroundTaskLogResponse> = withContext(Dispatchers.IO) {
        val baseUrl = currentBaseUrl ?: return@withContext Result.failure(RuntimeException("未配置网关地址".loc))
        try {
            val payload = buildJsonObject {
                put("conversationId", conversationId)
                put("taskId", taskId)
                if (!logUri.isNullOrBlank()) {
                    put("logUri", logUri)
                }
            }
            val request = buildAuthorizedRequest("$baseUrl/gateway/tasks/background/log")
                .post(payload.toString().toRequestBody(jsonMediaType))
                .build()
            client.newCall(request).await().use { response ->
                val respStr = response.body?.string().orEmpty()
                if (!response.isSuccessful) {
                    return@withContext Result.failure(RuntimeException("HTTP ${response.code}"))
                }
                Result.success(json.decodeFromString<BackgroundTaskLogResponse>(respStr))
            }
        } catch (e: Exception) {
            Result.failure(e)
        }
    }

    // MARK: - Swarm 蜂群协同接口

    suspend fun fetchSwarmSnapshot(
        cascadeId: String,
        afterSeq: Int? = null,
        beforeSeq: Int? = null,
        limit: Int? = null
    ): Result<SwarmSnapshot> = fetchSwarmSnapshotIfModified(
        cascadeId = cascadeId,
        afterSeq = afterSeq,
        beforeSeq = beforeSeq,
        limit = limit,
        etag = null
    ).mapCatching { it ?: throw RuntimeException("Not Modified") }

    suspend fun fetchSwarmSnapshotIfModified(
        cascadeId: String,
        afterSeq: Int? = null,
        beforeSeq: Int? = null,
        limit: Int? = null,
        etag: String? = null
    ): Result<SwarmSnapshot?> = withContext(Dispatchers.IO) {
        val baseUrl = currentBaseUrl ?: return@withContext Result.failure(RuntimeException("未配置网关地址".loc))
        try {
            val query = buildString {
                append("cascadeId=").append(URLEncoder.encode(cascadeId, "UTF-8"))
                if (afterSeq != null && afterSeq > 0) append("&afterSeq=").append(afterSeq)
                if (beforeSeq != null && beforeSeq > 0) append("&beforeSeq=").append(beforeSeq)
                if (limit != null && limit > 0) append("&limit=").append(limit)
            }
            val url = "$baseUrl/gateway/swarm?$query"
            val builder = buildAuthorizedRequest(url).get()
            if (!etag.isNullOrBlank()) {
                builder.header("If-None-Match", etag)
            }
            client.newCall(builder.build()).await().use { response ->
                if (response.code == 304) {
                    return@withContext Result.success(null)
                }
                val respStr = response.body?.string().orEmpty()
                if (!response.isSuccessful) {
                    return@withContext Result.failure(RuntimeException("HTTP ${response.code}"))
                }
                Result.success(json.decodeFromString<SwarmSnapshot>(respStr))
            }
        } catch (e: Exception) {
            Result.failure(e)
        }
    }

    suspend fun sendSwarmFeedback(cascadeId: String, content: String, coordinatorId: String? = null): Result<Unit> = withContext(Dispatchers.IO) {
        val baseUrl = currentBaseUrl ?: return@withContext Result.failure(RuntimeException("未配置网关地址".loc))
        try {
            val payload = json.encodeToString(
                SwarmFeedbackRequest(
                    cascadeId = cascadeId,
                    coordinatorId = coordinatorId?.takeIf { it.isNotBlank() },
                    feedback = content
                )
            )
            val request = buildAuthorizedRequest("$baseUrl/gateway/swarm/feedback")
                .post(payload.toRequestBody(jsonMediaType))
                .build()
            client.newCall(request).await().use { response ->
                if (!response.isSuccessful) {
                    return@withContext Result.failure(RuntimeException("HTTP ${response.code}"))
                }
                Result.success(Unit)
            }
        } catch (e: Exception) {
            Result.failure(e)
        }
    }

    suspend fun nudgeSwarmMember(cascadeId: String, member: String, coordinatorId: String? = null): Result<Unit> = withContext(Dispatchers.IO) {
        val baseUrl = currentBaseUrl ?: return@withContext Result.failure(RuntimeException("未配置网关地址".loc))
        try {
            val payload = json.encodeToString(
                SwarmNudgeRequest(
                    cascadeId = cascadeId,
                    coordinatorId = coordinatorId?.takeIf { it.isNotBlank() },
                    workerName = member
                )
            )
            val request = buildAuthorizedRequest("$baseUrl/gateway/swarm/nudge")
                .post(payload.toRequestBody(jsonMediaType))
                .build()
            client.newCall(request).await().use { response ->
                if (!response.isSuccessful) {
                    return@withContext Result.failure(RuntimeException("HTTP ${response.code}"))
                }
                Result.success(Unit)
            }
        } catch (e: Exception) {
            Result.failure(e)
        }
    }

    // MARK: - Plugins 插件市场接口

    suspend fun fetchPluginsCatalog(): Result<CascadePluginsResponse> = withContext(Dispatchers.IO) {
        val baseUrl = currentBaseUrl ?: return@withContext Result.failure(RuntimeException("未配置网关地址".loc))
        try {
            val request = buildAuthorizedRequest("$baseUrl/gateway/plugins").get().build()
            client.newCall(request).await().use { response ->
                val respStr = response.body?.string().orEmpty()
                if (!response.isSuccessful) {
                    return@withContext Result.failure(RuntimeException("HTTP ${response.code}"))
                }
                val parsed = json.decodeFromString<CascadePluginsResponse>(respStr)
                Result.success(parsed)
            }
        } catch (e: Exception) {
            Result.failure(e)
        }
    }

    suspend fun fetchPlugins(): Result<List<CascadePluginItem>> =
        fetchPluginsCatalog().map { it.plugins }

    suspend fun installPlugin(id: String, conversationId: String? = null, autoResume: Boolean = true): Result<PluginInstallResponse> = withContext(Dispatchers.IO) {
        val baseUrl = currentBaseUrl ?: return@withContext Result.failure(RuntimeException("未配置网关地址".loc))
        try {
            val payload = json.encodeToString(
                PluginInstallRequest(
                    pluginId = id,
                    cascadeId = conversationId?.takeIf { it.isNotBlank() },
                    autoResume = autoResume
                )
            )
            val request = buildAuthorizedRequest("$baseUrl/gateway/plugins/install")
                .post(payload.toRequestBody(jsonMediaType))
                .build()
            client.newCall(request).await().use { response ->
                val respStr = response.body?.string().orEmpty()
                if (!response.isSuccessful) {
                    return@withContext Result.failure(RuntimeException("HTTP ${response.code}"))
                }
                Result.success(json.decodeFromString<PluginInstallResponse>(respStr))
            }
        } catch (e: Exception) {
            Result.failure(e)
        }
    }

    suspend fun revokePluginPermissionGrant(item: PluginPermissionGrantItem): Result<Unit> = withContext(Dispatchers.IO) {
        val baseUrl = currentBaseUrl ?: return@withContext Result.failure(RuntimeException("未配置网关地址".loc))
        try {
            val payload = json.encodeToString(
                RevokePluginPermissionGrantRequest(
                    grant = item.grant,
                    scope = item.scope,
                    projectId = item.projectId?.takeIf { it.isNotBlank() }
                )
            )
            val request = buildAuthorizedRequest("$baseUrl/gateway/plugins/grants/revoke")
                .post(payload.toRequestBody(jsonMediaType))
                .build()
            client.newCall(request).await().use { response ->
                if (!response.isSuccessful) {
                    return@withContext Result.failure(RuntimeException("HTTP ${response.code}"))
                }
                Result.success(Unit)
            }
        } catch (e: Exception) {
            Result.failure(e)
        }
    }

    suspend fun setConversationArchived(cascadeId: String, archived: Boolean): Result<Unit> = withContext(Dispatchers.IO) {
        val baseUrl = currentBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址".loc))
        val url = "$baseUrl/api/exa.language_server_pb.LanguageServerService/UpdateConversationAnnotations"
        val payload = buildJsonObject {
            put("cascadeIds", buildJsonArray { add(JsonPrimitive(cascadeId)) })
            put("annotations", buildJsonObject {
                put("archived", archived)
            })
            put("mergeAnnotations", true)
        }
        try {
            val request = buildAuthorizedRequest(url)
                .post(payload.toString().toRequestBody(jsonMediaType))
                .header("Connect-Protocol-Version", "1")
                .build()
            client.newCall(request).await().use { response ->
                if (response.isSuccessful) Result.success(Unit)
                else Result.failure(RuntimeException("归档操作失败: HTTP %d".loc(response.code)))
            }
        } catch (e: Exception) {
            Result.failure(e)
        }
    }

    // MARK: - Automations 定时与守护任务接口

    suspend fun fetchAutomationsData(): Result<AutomationsResponse> = withContext(Dispatchers.IO) {
        val baseUrl = currentBaseUrl ?: return@withContext Result.failure(RuntimeException("未配置网关地址".loc))
        try {
            val request = buildAuthorizedRequest("$baseUrl/gateway/automations").get().build()
            client.newCall(request).await().use { response ->
                val respStr = response.body?.string().orEmpty()
                if (!response.isSuccessful) {
                    return@withContext Result.failure(RuntimeException("HTTP ${response.code}"))
                }
                val parsed = json.decodeFromString<AutomationsResponse>(respStr)
                Result.success(parsed)
            }
        } catch (e: Exception) {
            Result.failure(e)
        }
    }

    suspend fun fetchAutomations(): Result<List<AutomationSidecarItem>> =
        fetchAutomationsData().map { it.automations }

    suspend fun manageAutomation(action: String, sidecarId: String): Result<Unit> = withContext(Dispatchers.IO) {
        val baseUrl = currentBaseUrl ?: return@withContext Result.failure(RuntimeException("未配置网关地址".loc))
        try {
            val payload = json.encodeToString(AutomationManageRequest(sidecarId = sidecarId, action = action))
            val request = buildAuthorizedRequest("$baseUrl/gateway/automations/manage")
                .post(payload.toRequestBody(jsonMediaType))
                .build()
            client.newCall(request).await().use { response ->
                if (!response.isSuccessful) {
                    return@withContext Result.failure(RuntimeException("HTTP ${response.code}"))
                }
                Result.success(Unit)
            }
        } catch (e: Exception) {
            Result.failure(e)
        }
    }

    // region Workflow 模板同步接口

    suspend fun fetchWorkflowTemplates(): Result<WorkflowTemplatesResponse> = withContext(Dispatchers.IO) {
        val baseUrl = currentBaseUrl ?: return@withContext Result.failure(RuntimeException("未配置网关地址".loc))
        try {
            val request = buildAuthorizedRequest("$baseUrl/gateway/workflow-templates")
                .get()
                .build()
            client.newCall(request).await().use { response ->
                val body = response.body?.string().orEmpty()
                if (!response.isSuccessful) {
                    return@withContext Result.failure(RuntimeException("HTTP ${response.code}: $body"))
                }
                Result.success(json.decodeFromString<WorkflowTemplatesResponse>(body))
            }
        } catch (e: Exception) {
            Result.failure(e)
        }
    }

    suspend fun saveWorkflowTemplates(templates: List<WorkflowTemplateRecord>): Result<WorkflowTemplatesResponse> = withContext(Dispatchers.IO) {
        val baseUrl = currentBaseUrl ?: return@withContext Result.failure(RuntimeException("未配置网关地址".loc))
        try {
            val payload = json.encodeToString(WorkflowTemplatesUpdateRequest(templates = templates))
            val request = buildAuthorizedRequest("$baseUrl/gateway/workflow-templates")
                .put(payload.toRequestBody(jsonMediaType))
                .build()
            client.newCall(request).await().use { response ->
                val body = response.body?.string().orEmpty()
                if (!response.isSuccessful) {
                    return@withContext Result.failure(RuntimeException("HTTP ${response.code}: $body"))
                }
                Result.success(json.decodeFromString<WorkflowTemplatesResponse>(body))
            }
        } catch (e: Exception) {
            Result.failure(e)
        }
    }

    suspend fun resetWorkflowTemplates(): Result<WorkflowTemplatesResponse> = withContext(Dispatchers.IO) {
        val baseUrl = currentBaseUrl ?: return@withContext Result.failure(RuntimeException("未配置网关地址".loc))
        try {
            val request = buildAuthorizedRequest("$baseUrl/gateway/workflow-templates")
                .delete()
                .build()
            client.newCall(request).await().use { response ->
                val body = response.body?.string().orEmpty()
                if (!response.isSuccessful) {
                    return@withContext Result.failure(RuntimeException("HTTP ${response.code}: $body"))
                }
                Result.success(json.decodeFromString<WorkflowTemplatesResponse>(body))
            }
        } catch (e: Exception) {
            Result.failure(e)
        }
    }

    // endregion

    internal val prefsRef: PreferencesManager
        get() = prefs

    internal fun buildAuthorizedRequest(url: String): Request.Builder {
        val builder = Request.Builder()
            .url(url)
            .header("Content-Type", "application/json")
            .header("Connect-Protocol-Version", "1")

        prefs.deviceToken?.takeIf { it.isNotBlank() }?.let { token ->
            builder.header("Authorization", "Bearer $token")
        }
        return builder
    }
}

/**
 * OkHttp Interceptor that automatically fails over from an unreachable LAN gateway
 * to the primary cloud domain (Cloudflare/E2EE HTTPS tunnel), and transparently migrates
 * across multi-VPS relay nodes when a stale relay returns HTTP 502/503 (`gateway_offline`)
 * or fails with a network error.
 */
class RouteFailoverInterceptor(
    private val prefs: PreferencesManager,
    private val connectionManager: ConnectionManager?
) : okhttp3.Interceptor {
    private fun canSafelyFailover(request: Request, error: IOException): Boolean {
        if (request.body?.isOneShot() == true) {
            return false
        }
        val method = request.method.uppercase()
        if (method == "GET" || method == "HEAD") {
            return true
        }
        // For state-mutating requests (POST/PUT/PATCH/DELETE), only failover on pre-transmission
        // connection establishment errors — never on read SocketTimeoutException after payload dispatch.
        return error is java.net.ConnectException ||
            error is java.net.UnknownHostException ||
            error is java.net.NoRouteToHostException ||
            (error is java.net.SocketTimeoutException && error.message?.contains("connect", ignoreCase = true) == true)
    }

    private fun isSafeReadRequest(request: Request): Boolean {
        if (request.body?.isOneShot() == true) return false
        val method = request.method.uppercase()
        if (method == "GET" || method == "HEAD") return true
        val path = request.url.encodedPath
        return path.endsWith("/api/v1/attachments") ||
            path.endsWith("/GetAllCascadeTrajectories") ||
            path.endsWith("/GetCascadeTrajectory") ||
            path.endsWith("/GetCascadeTrajectorySteps") ||
            path.endsWith("/GetAuthStatus") ||
            path.endsWith("/GetCascadeNuxes") ||
            path.endsWith("/SearchConversations")
    }

    private fun isGatewayUpstreamUnavailableResponse(response: okhttp3.Response): Boolean {
        if (response.code != 503) return false
        if (response.header("X-Mgy-Error")?.trim().equals("upstream_not_connected", ignoreCase = true)) {
            return true
        }
        val snippet = try {
            response.peekBody(320).string().lowercase()
        } catch (_: Exception) {
            ""
        }
        return snippet.contains("upstream_not_connected") ||
            snippet.contains("antigravity upstream not connected") ||
            snippet.contains("language_server is not connected") ||
            snippet.contains("language_server not connected")
    }

    private fun isRelayGatewayOfflineResponse(response: okhttp3.Response): Boolean {
        if (response.code != 503) return false
        val snippet = try {
            response.peekBody(256).string().lowercase()
        } catch (_: Exception) {
            ""
        }
        return snippet.contains("gateway_offline") || snippet.contains("relay_capacity_full")
    }

    private fun originOf(url: HttpUrl): String {
        val defaultPort = if (url.isHttps) 443 else 80
        return if (url.port == defaultPort) {
            "${url.scheme}://${url.host}"
        } else {
            "${url.scheme}://${url.host}:${url.port}"
        }
    }

    private fun retryOnTargetBase(
        chain: okhttp3.Interceptor.Chain,
        request: Request,
        targetBaseUrl: String,
        reqEpoch: Long = GatewayRuntimeState.epoch
    ): okhttp3.Response? {
        val clean = targetBaseUrl.trim().trimEnd('/')
        if (clean.isEmpty()) return null
        val targetUri = java.net.URI(if (!clean.contains("://")) "https://$clean" else clean)
        val newScheme = targetUri.scheme ?: "https"
        val newHost = targetUri.host ?: ConnectionManager.extractHost(clean)
        if (newHost.isBlank()) return null
        if (!newScheme.equals("https", ignoreCase = true) && !ConnectionManager.isLanHost(newHost)) {
            return null
        }
        val newPort = if (targetUri.port != -1) {
            targetUri.port
        } else if (newScheme.equals("https", ignoreCase = true)) {
            443
        } else {
            80
        }

        val fallbackUrl = request.url.newBuilder()
            .scheme(newScheme)
            .host(newHost)
            .port(newPort)
            .build()

        val fallbackRequest = request.newBuilder()
            .url(fallbackUrl)
            .build()

        val fallbackResponse = chain.withConnectTimeout(10, TimeUnit.SECONDS).proceed(fallbackRequest)
        if ((fallbackResponse.isSuccessful || fallbackResponse.code < 500) &&
            GatewayRuntimeState.epoch == reqEpoch
        ) {
            ConnectionManager.applyAdvertisedCloudUrl(
                rawHeader = fallbackResponse.header("X-Antigravity-Cloud-URL"),
                prefs = prefs,
                connectionManager = connectionManager
            )
            if (!prefs.gatewayBaseUrl.equals(clean, ignoreCase = true)) {
                Log.i("ApiClient", "Failover to $clean succeeded! Updating active gateway to $clean")
                prefs.gatewayBaseUrl = clean
                connectionManager?.notifyRouteChanged(clean)
            }
        }
        return fallbackResponse
    }

    override fun intercept(chain: okhttp3.Interceptor.Chain): okhttp3.Response {
        val request = chain.request()
        // Side-effect-free requests targeting an explicit gateway context must never mutate active prefs or failover across global endpoints.
        if (request.tag(E2eeContextTag::class.java) != null) {
            return chain.proceed(request)
        }
        val reqEpoch = GatewayRuntimeState.epoch
        val originalUrl = request.url
        val originalHost = originalUrl.host
        if (!ApiClient.isConfiguredGatewayOrigin(originalUrl, prefs)) {
            return chain.proceed(request)
        }

        // If communicating with LAN host, shorten connect timeout to 1800ms so off-network failover is snappy
        val isLan = ConnectionManager.isLanHost(originalHost)
        val initialChain = if (isLan) {
            chain.withConnectTimeout(1800, TimeUnit.MILLISECONDS)
        } else {
            chain
        }

        var initialError: IOException? = null
        val response = try {
            initialChain.proceed(request)
        } catch (e: IOException) {
            initialError = e
            null
        }

        if (response != null) {
            if (response.code < 500) {
                if (GatewayRuntimeState.epoch == reqEpoch) {
                    ConnectionManager.applyAdvertisedCloudUrl(
                        rawHeader = response.header("X-Antigravity-Cloud-URL"),
                        prefs = prefs,
                        connectionManager = connectionManager
                    )
                }
                return response
            }

            // Multi-VPS relay migration: when an old relay node returns HTTP 503 gateway_offline
            // (or 502/503 on a safe read request), discover the active node via /api/relay/pool
            // and transparently retry once so quota/list loads recover on the first attempt.
            if (!isLan &&
                connectionManager != null &&
                request.body?.isOneShot() != true &&
                (response.code == 502 || response.code == 503) &&
                !isGatewayUpstreamUnavailableResponse(response) &&
                (isRelayGatewayOfflineResponse(response) || isSafeReadRequest(request))
            ) {
                val failedOrigin = originOf(originalUrl)
                val migratedUrl = connectionManager.migrateUnreachableCloudBlocking(prefs, failedOrigin)
                if (!migratedUrl.isNullOrBlank()) {
                    val migratedHost = ConnectionManager.extractHost(migratedUrl)
                    if (migratedHost.isNotBlank() && !migratedHost.equals(originalHost, ignoreCase = true)) {
                        response.close()
                        try {
                            retryOnTargetBase(chain, request, migratedUrl, reqEpoch)?.let { return it }
                        } catch (retryEx: Exception) {
                            Log.w("ApiClient", "Retry on migrated relay $migratedUrl failed: ${retryEx.message}")
                            throw retryEx
                        }
                    }
                }
            }
            return response
        }

        val e = initialError!!
        if (!canSafelyFailover(request, e)) {
            throw e
        }
        val cloud = prefs.primaryCloudUrl?.trim()?.trimEnd('/')
        var failedCloudOrigin: String? = null
        var migrationAttempted = false
        if (!cloud.isNullOrBlank()) {
            val cloudHost = ConnectionManager.extractHost(cloud)
            if (!originalHost.equals(cloudHost, ignoreCase = true)) {
                Log.w("ApiClient", "Request to $originalHost failed (${e.message}). Failing over to cloud domain: $cloud")
                val fallbackResponse = try {
                    retryOnTargetBase(chain, request, cloud, reqEpoch)
                } catch (fallbackEx: Exception) {
                    Log.w("ApiClient", "Failover to $cloud also failed: ${fallbackEx.message}")
                    failedCloudOrigin = cloud
                    null
                }
                if (fallbackResponse != null) {
                    if ((fallbackResponse.code == 502 || fallbackResponse.code == 503) &&
                        !isGatewayUpstreamUnavailableResponse(fallbackResponse) &&
                        connectionManager != null &&
                        (isRelayGatewayOfflineResponse(fallbackResponse) || isSafeReadRequest(request))
                    ) {
                        migrationAttempted = true
                        val migratedUrl = connectionManager.migrateUnreachableCloudBlocking(prefs, cloud)
                        if (!migratedUrl.isNullOrBlank() &&
                            !ConnectionManager.extractHost(migratedUrl).equals(cloudHost, ignoreCase = true)
                        ) {
                            fallbackResponse.close()
                            try {
                                retryOnTargetBase(chain, request, migratedUrl, reqEpoch)?.let { return it }
                            } catch (migEx: Exception) {
                                Log.w("ApiClient", "Failover to migrated relay $migratedUrl failed: ${migEx.message}")
                                throw migEx
                            }
                        }
                    }
                    return fallbackResponse
                }
            } else {
                failedCloudOrigin = originOf(originalUrl)
            }
        } else if (!isLan) {
            failedCloudOrigin = originOf(originalUrl)
        }

        if (!migrationAttempted && !failedCloudOrigin.isNullOrBlank() && connectionManager != null) {
            val migratedUrl = connectionManager.migrateUnreachableCloudBlocking(prefs, failedCloudOrigin)
            if (!migratedUrl.isNullOrBlank() &&
                !ConnectionManager.extractHost(migratedUrl).equals(originalHost, ignoreCase = true)
            ) {
                try {
                    retryOnTargetBase(chain, request, migratedUrl, reqEpoch)?.let { return it }
                } catch (migEx: Exception) {
                    Log.w("ApiClient", "Failover to migrated relay $migratedUrl also failed: ${migEx.message}")
                }
            }
        }
        throw e
    }
}
