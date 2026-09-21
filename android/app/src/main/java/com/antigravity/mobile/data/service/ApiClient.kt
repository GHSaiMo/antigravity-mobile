package com.antigravity.mobile.data.service

import android.os.Build
import android.util.Base64
import android.util.Log
import com.antigravity.mobile.data.model.*
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.*
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import java.net.URLDecoder
import java.net.URLEncoder
import java.time.Instant
import java.util.concurrent.TimeUnit

class ApiClient(
    private val prefs: PreferencesManager,
    private val connectionManager: ConnectionManager? = null
) {
    val currentBaseUrl: String?
        get() {
            val isCell = connectionManager?.isCellular ?: false
            return prefs.getEffectiveGatewayUrl(isCell) ?: prefs.gatewayBaseUrl
        }

    val json = Json {
        ignoreUnknownKeys = true
        isLenient = true
        encodeDefaults = true
        coerceInputValues = true
    }

    private val client = OkHttpClient.Builder()
        .connectTimeout(10, TimeUnit.SECONDS)
        .readTimeout(30, TimeUnit.SECONDS)
        .writeTimeout(30, TimeUnit.SECONDS)
        .build()

    private val jsonMediaType = "application/json; charset=utf-8".toMediaType()

    /**
     * Attempts pairing with candidates from PairingInfo.
     */
    suspend fun pair(info: PairingInfo): Result<PairResponse> = withContext(Dispatchers.IO) {
        val candidates = info.candidateBaseUrls()
        if (candidates.isEmpty()) {
            return@withContext Result.failure(IllegalArgumentException("无可用候选网关地址"))
        }

        val deviceName = "${Build.MANUFACTURER} ${Build.MODEL}"
        val pairReq = PairRequest(
            pairingCode = info.code,
            deviceName = deviceName,
            platform = "android"
        )
        val bodyStr = json.encodeToString(pairReq)

        var lastException: Exception? = null
        for (candidate in candidates) {
            val endpoint = "$candidate/api/v1/auth/pair"
            try {
                val request = Request.Builder()
                    .url(endpoint)
                    .post(bodyStr.toRequestBody(jsonMediaType))
                    .build()

                client.newCall(request).execute().use { response ->
                    if (!response.isSuccessful) {
                        val errMsg = response.body?.string() ?: "HTTP ${response.code}"
                        lastException = RuntimeException("配对响应失败: $errMsg")
                        return@use
                    }
                    val respStr = response.body?.string() ?: ""
                    val pairResp = json.decodeFromString<PairResponse>(respStr)

                    var lanUrl: String? = info.lanBaseUrl
                    var ipv6Url: String? = info.ipv6BaseUrl
                    var relayUrl: String? = info.relayBaseUrl
                    var cloudUrl: String? = null

                    if (info.ssl || !ConnectionManager.isLanHost(ConnectionManager.extractHost(info.host))) {
                        cloudUrl = info.serverBaseUrl
                    }

                    if (pairResp.endpoints != null) {
                        for (ep in pairResp.endpoints) {
                            val epUrl = ep.url.trim().trimEnd('/')
                            if (!ConnectionManager.isTrustedEndpoint(epUrl, info, candidate)) {
                                continue
                            }
                            when (ep.type.lowercase()) {
                                "lan" -> lanUrl = epUrl
                                "ipv6" -> ipv6Url = epUrl
                                "relay" -> relayUrl = epUrl
                                "cloudflare" -> cloudUrl = epUrl
                                "primary" -> {
                                    val h = ConnectionManager.extractHost(epUrl)
                                    when {
                                        ConnectionManager.isLanHost(h) && lanUrl.isNullOrBlank() -> lanUrl = epUrl
                                        ConnectionManager.isIpv6Host(h) && ipv6Url.isNullOrBlank() -> ipv6Url = epUrl
                                        ConnectionManager.isRelayHost(h) && relayUrl.isNullOrBlank() -> relayUrl = epUrl
                                        else -> if (cloudUrl.isNullOrBlank()) cloudUrl = epUrl
                                    }
                                }
                            }
                        }
                    }

                    // Fallback classify the successful candidate into its slot if still unassigned
                    val candClean = candidate.trim().trimEnd('/')
                    val candHost = ConnectionManager.extractHost(candClean)
                    when {
                        ConnectionManager.isLanHost(candHost) -> {
                            if (lanUrl.isNullOrBlank()) lanUrl = candClean
                        }
                        ConnectionManager.isIpv6Host(candHost) -> {
                            if (ipv6Url.isNullOrBlank()) ipv6Url = candClean
                        }
                        ConnectionManager.isRelayHost(candHost) -> {
                            if (relayUrl.isNullOrBlank()) relayUrl = candClean
                        }
                        else -> {
                            // Public allocated domain (e.g. Cloudflare tunnel) strictly serves as background fallback, NEVER custom!
                            if (cloudUrl.isNullOrBlank()) cloudUrl = candClean
                        }
                    }

                    prefs.updateEndpoints(
                        lan = lanUrl,
                        ipv6 = ipv6Url,
                        relay = relayUrl,
                        custom = null, // Strictly null: custom is left for manual user configuration only
                        active = candidate,
                        primaryCloud = cloudUrl
                    )
                    prefs.deviceToken = pairResp.deviceToken
                    prefs.deviceId = pairResp.deviceId

                    return@withContext Result.success(pairResp)
                }
            } catch (e: Exception) {
                Log.w("ApiClient", "Failed candidate $candidate: ${e.message}")
                lastException = e
            }
        }

        Result.failure(lastException ?: RuntimeException("所有候选地址配对均超时或失败"))
    }

    /**
     * Fetch all conversations via ConnectRPC GetAllCascadeTrajectories
     */
    suspend fun fetchConversations(): Result<List<ConversationItem>> = withContext(Dispatchers.IO) {
        val baseUrl = currentBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址"))
        val url = "$baseUrl/api/exa.language_server_pb.LanguageServerService/GetAllCascadeTrajectories"

        try {
            val req = buildAuthorizedRequest(url)
                .post("{}".toRequestBody(jsonMediaType))
                .build()

            client.newCall(req).execute().use { response ->
                if (!response.isSuccessful) {
                    return@withContext Result.failure(RuntimeException("获取会话列表失败 (${response.code})"))
                }
                val bodyStr = response.body?.string() ?: "{}"
                val resp = json.decodeFromString<GetAllCascadeTrajectoriesResponse>(bodyStr)
                val summaries = resp.trajectorySummaries ?: emptyMap()

                val list = summaries.mapNotNull { (id, summary) ->
                    if (summary.isSubagent) return@mapNotNull null
                    val localViewTime = prefs.getLastViewTime(id)
                    val item = ConversationItem.fromSummary(id, summary, localViewTime = localViewTime)
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
        val baseUrl = currentBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址"))
        val url = "$baseUrl/api/v1/cockpit/quotas"

        try {
            val req = buildAuthorizedRequest(url).get().build()
            client.newCall(req).execute().use { response ->
                if (!response.isSuccessful) {
                    return@withContext Result.failure(RuntimeException("获取配额失败 (${response.code})"))
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
     * Refresh Cockpit Quota
     */
    suspend fun refreshCockpitQuotas(): Result<CockpitQuotaResponse> = withContext(Dispatchers.IO) {
        val baseUrl = currentBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址"))
        val url = "$baseUrl/api/v1/cockpit/refresh"

        try {
            val req = buildAuthorizedRequest(url).post("{}".toRequestBody(jsonMediaType)).build()
            client.newCall(req).execute().use { response ->
                if (!response.isSuccessful) {
                    return@withContext Result.failure(RuntimeException("刷新配额失败 (${response.code})"))
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
        val baseUrl = currentBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址"))
        val url = "$baseUrl/api/v1/cockpit/switch"

        val body = buildJsonObject {
            put("account_id", accountId)
        }.toString().toRequestBody(jsonMediaType)

        try {
            val req = buildAuthorizedRequest(url).post(body).build()
            client.newCall(req).execute().use { response ->
                if (response.isSuccessful) {
                    Result.success(Unit)
                } else {
                    Result.failure(RuntimeException("切换账号失败: HTTP ${response.code}"))
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
        val baseUrl = currentBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址"))
        val url = "$baseUrl/gateway/projects"

        try {
            val req = buildAuthorizedRequest(url).get().build()
            client.newCall(req).execute().use { response ->
                if (!response.isSuccessful) {
                    val errMsg = response.body?.string()?.take(200) ?: ""
                    Log.e("ApiClient", "fetchProjects failed (${response.code}): $errMsg")
                    return@withContext Result.failure(RuntimeException("获取工作区列表失败 (${response.code})"))
                }
                val bodyStr = response.body?.string() ?: "[]"
                val list = json.decodeFromString<List<ProjectItem>>(bodyStr)
                Log.d("ApiClient", "fetchProjects successfully retrieved ${list.size} projects")
                Result.success(list)
            }
        } catch (e: Exception) {
            Log.e("ApiClient", "fetchProjects error: ${e.message}", e)
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
        val baseUrl = currentBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址"))
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

            client.newCall(req).execute().use { response ->
                if (!response.isSuccessful) {
                    return@withContext Result.failure(RuntimeException("创建会话失败: HTTP ${response.code}"))
                }
                val bodyStr = response.body?.string() ?: "{}"
                val resObj = json.parseToJsonElement(bodyStr).jsonObject
                val cascadeId = resObj["cascadeId"]?.jsonPrimitive?.contentOrNull
                if (!cascadeId.isNullOrBlank()) {
                    Result.success(cascadeId)
                } else {
                    val err = resObj["error"]?.jsonPrimitive?.contentOrNull ?: "未能生成会话 ID"
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
        val baseUrl = currentBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址"))
        val url = "$baseUrl/api/exa.language_server_pb.LanguageServerService/DeleteCascadeTrajectory"

        val payload = buildJsonObject {
            put("cascadeId", cascadeId)
        }

        try {
            val req = buildAuthorizedRequest(url)
                .post(payload.toString().toRequestBody(jsonMediaType))
                .build()

            client.newCall(req).execute().use { response ->
                if (response.isSuccessful) {
                    Result.success(Unit)
                } else {
                    Result.failure(RuntimeException("删除会话失败: HTTP ${response.code}"))
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
        val baseUrl = currentBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址"))
        val url = "$baseUrl/api/exa.language_server_pb.LanguageServerService/SetCascadeTrajectoryMetadata"

        val payload = buildJsonObject {
            put("cascadeId", cascadeId)
            put("annotations", buildJsonObject {
                put("title", newTitle.trim())
            })
        }

        try {
            val req = buildAuthorizedRequest(url)
                .post(payload.toString().toRequestBody(jsonMediaType))
                .build()

            client.newCall(req).execute().use { response ->
                if (response.isSuccessful) {
                    Result.success(Unit)
                } else {
                    Result.failure(RuntimeException("重命名会话失败: HTTP ${response.code}"))
                }
            }
        } catch (e: Exception) {
            Result.failure(e)
        }
    }

    /**
     * Fetch paginated messages for a cascade session
     */
    suspend fun fetchMessages(
        cascadeId: String,
        limit: Int = 15,
        offset: Int? = null
    ): Result<StreamUpdatePayload> = withContext(Dispatchers.IO) {
        val baseUrl = currentBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址"))
        val urlBuilder = StringBuilder("$baseUrl/gateway/cascade/messages?cascadeId=$cascadeId&limit=$limit")
        offset?.let { urlBuilder.append("&offset=$it") }

        try {
            val req = buildAuthorizedRequest(urlBuilder.toString())
                .get()
                .build()

            client.newCall(req).execute().use { response ->
                if (!response.isSuccessful) {
                    return@withContext Result.failure(RuntimeException("获取消息失败: HTTP ${response.code}"))
                }
                val bodyStr = response.body?.string() ?: "{}"
                val payload = json.decodeFromString<StreamUpdatePayload>(bodyStr)
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
        clientMessageId: String? = null
    ): Result<Unit> = withContext(Dispatchers.IO) {
        val baseUrl = currentBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址"))
        val url = "$baseUrl/api/exa.language_server_pb.LanguageServerService/SendUserCascadeMessage"

        val payload = buildJsonObject {
            put("cascadeId", cascadeId)
            putJsonArray("items") {
                addJsonObject {
                    put("text", text)
                }
            }
            put("text", text)
            if (images.isNotEmpty()) {
                putJsonArray("images") {
                    images.forEach { (bytes, mime) ->
                        addJsonObject {
                            put("base64Data", Base64.encodeToString(bytes, Base64.NO_WRAP))
                            put("mimeType", mime)
                        }
                    }
                }
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

            client.newCall(req).execute().use { response ->
                if (response.isSuccessful) {
                    Result.success(Unit)
                } else {
                    Result.failure(RuntimeException("发送失败: HTTP ${response.code}"))
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
        val baseUrl = currentBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址"))
        val url = "$baseUrl/api/exa.language_server_pb.LanguageServerService/DeleteAgentMessage"

        val payload = buildJsonObject {
            put("messageId", messageId)
            put("recipient", cascadeId)
        }

        try {
            val req = buildAuthorizedRequest(url)
                .post(payload.toString().toRequestBody(jsonMediaType))
                .build()

            client.newCall(req).execute().use { response ->
                if (response.isSuccessful) {
                    Result.success(Unit)
                } else {
                    Result.failure(RuntimeException("删除队列消息失败: HTTP ${response.code}"))
                }
            }
        } catch (e: Exception) {
            Result.failure(e)
        }
    }

    /**
     * Cancel ongoing invocation
     */
    suspend fun cancelInvocation(cascadeId: String): Result<Unit> = withContext(Dispatchers.IO) {
        val baseUrl = currentBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址"))
        val url = "$baseUrl/api/exa.language_server_pb.LanguageServerService/CancelCascadeInvocation"

        val payload = buildJsonObject {
            put("cascadeId", cascadeId)
        }

        try {
            val req = buildAuthorizedRequest(url)
                .post(payload.toString().toRequestBody(jsonMediaType))
                .build()

            client.newCall(req).execute().use { response ->
                if (response.isSuccessful) {
                    Result.success(Unit)
                } else {
                    Result.failure(RuntimeException("取消失败: HTTP ${response.code}"))
                }
            }
        } catch (e: Exception) {
            Result.failure(e)
        }
    }

    /**
     * Stop background running command task
     */
    suspend fun stopTask(cascadeId: String, taskId: String, stepIndex: Int = 0): Result<Unit> = withContext(Dispatchers.IO) {
        val baseUrl = currentBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址"))
        val url = "$baseUrl/gateway/cascade/task/stop"

        val payload = buildJsonObject {
            put("cascade_id", cascadeId)
            put("task_id", taskId)
            put("step_index", stepIndex)
        }

        try {
            val req = buildAuthorizedRequest(url)
                .post(payload.toString().toRequestBody(jsonMediaType))
                .build()

            client.newCall(req).execute().use { response ->
                if (response.isSuccessful) {
                    Result.success(Unit)
                } else {
                    Result.failure(RuntimeException("停止任务失败: HTTP ${response.code}"))
                }
            }
        } catch (e: Exception) {
            Result.failure(e)
        }
    }

    /**
     * Submit user interaction response (e.g. command approval)
     */
    suspend fun submitInteraction(
        cascadeId: String,
        stepIndex: Int,
        responseType: String,
        selectedOptionId: String? = null,
        confirmed: Boolean? = null,
        customText: String? = null
    ): Result<Unit> = withContext(Dispatchers.IO) {
        val baseUrl = currentBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址"))
        val url = "$baseUrl/gateway/cascade/interaction"

        val reqObj = InteractionRespondRequest(
            cascadeId = cascadeId,
            stepIndex = stepIndex,
            responseType = responseType,
            selectedOptionId = selectedOptionId,
            confirmed = confirmed,
            customText = customText
        )
        val bodyStr = json.encodeToString(reqObj)

        try {
            val req = buildAuthorizedRequest(url)
                .post(bodyStr.toRequestBody(jsonMediaType))
                .build()

            client.newCall(req).execute().use { response ->
                if (response.isSuccessful) {
                    Result.success(Unit)
                } else {
                    Result.failure(RuntimeException("交互提交失败: HTTP ${response.code}"))
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
        val baseUrl = currentBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址"))
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

            client.newCall(req).execute().use { response ->
                if (response.isSuccessful) {
                    Result.success(Unit)
                } else {
                    Result.failure(RuntimeException("执行方案推进失败: HTTP ${response.code}"))
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
        val baseUrl = currentBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址"))
        val unescapedUri = try { URLDecoder.decode(uri, "UTF-8") } catch (_: Exception) { uri }
        val encodedUri = URLEncoder.encode(unescapedUri, "UTF-8")
        val cidParam = cascadeId?.let { "&cascade_id=${URLEncoder.encode(it, "UTF-8")}" } ?: ""
        val url = "$baseUrl/api/v1/files/content?uri=$encodedUri$cidParam"

        try {
            val req = buildAuthorizedRequest(url).get().build()
            client.newCall(req).execute().use { response ->
                if (!response.isSuccessful) {
                    return@withContext Result.failure(RuntimeException("获取文件失败: HTTP ${response.code}"))
                }
                val bodyStr = response.body?.string() ?: "{}"
                val fileResp = json.decodeFromString<FileContentResponse>(bodyStr)
                Result.success(fileResp)
            }
        } catch (e: Exception) {
            Result.failure(e)
        }
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
            client.newCall(req).execute().close()
        } catch (_: Exception) {}
    }

    /**
     * Resolves a raw media/image URI into an authenticated, loadable HTTP URL for Coil / Image loaders.
     */
    fun resolveMediaURL(raw: String): String {
        var clean = raw.trim()
        if (clean.startsWith("MEDIA:", ignoreCase = true)) {
            clean = clean.substring(6).trim()
        }
        clean = clean.trim('`', '"', '\'', '(', ')', '[', ']', '<', '>')

        val token = prefs.deviceToken ?: ""
        val baseUrl = currentBaseUrl?.trimEnd('/') ?: ""

        if (clean.startsWith("http://") || clean.startsWith("https://")) {
            if (baseUrl.isNotBlank() && clean.contains("/api/v1/files/raw") && !clean.contains("auth_token=") && !clean.contains("token=") && token.isNotBlank()) {
                val separator = if (clean.contains("?")) "&" else "?"
                return "$clean${separator}auth_token=$token"
            }
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
        val tokenParam = if (token.isNotBlank()) "&auth_token=$token" else ""
        return "$baseUrl/api/v1/files/raw?uri=$encodedUri$tokenParam"
    }

    suspend fun downloadFile(
        uri: String,
        targetFile: java.io.File,
        cascadeId: String? = null,
        onProgress: ((progress: Float, written: Long, total: Long) -> Unit)? = null
    ): Result<java.io.File> = withContext(Dispatchers.IO) {
        try {
            val resolvedUrl = resolveMediaURL(uri)
            val request = Request.Builder()
                .url(resolvedUrl)
                .apply {
                    prefs.deviceToken?.takeIf { it.isNotBlank() }?.let { token ->
                        header("Authorization", "Bearer $token")
                    }
                }
                .build()

            client.newCall(request).execute().use { response ->
                if (!response.isSuccessful) {
                    return@withContext Result.failure(RuntimeException("下载文件失败: HTTP ${response.code}"))
                }
                val body = response.body ?: return@withContext Result.failure(RuntimeException("空响应体"))
                val totalLength = body.contentLength()

                targetFile.parentFile?.mkdirs()
                val tempFile = java.io.File(targetFile.parentFile, "${targetFile.name}.tmp")
                tempFile.outputStream().use { output ->
                    body.byteStream().use { input ->
                        val buffer = ByteArray(8192)
                        var bytesWritten = 0L
                        var read: Int
                        while (input.read(buffer).also { read = it } != -1) {
                            output.write(buffer, 0, read)
                            bytesWritten += read
                            if (totalLength > 0 && onProgress != null) {
                                val progress = (bytesWritten.toFloat() / totalLength.toFloat()).coerceIn(0f, 1f)
                                onProgress(progress, bytesWritten, totalLength)
                            }
                        }
                        output.flush()
                    }
                }
                if (targetFile.exists()) targetFile.delete()
                tempFile.renameTo(targetFile)
                Result.success(targetFile)
            }
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

            client.newCall(request).execute().use { response ->
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
            ?: return@withContext Result.failure(IllegalStateException("网关地址未配置"))

        try {
            val endpoint = "$baseUrl/gateway/cascade/revert/preview"
            val reqPayload = json.encodeToString(RevertPreviewRequest(cascadeId = cascadeId, stepIndex = stepIndex))
            val request = buildAuthorizedRequest(endpoint)
                .post(reqPayload.toRequestBody(jsonMediaType))
                .build()

            client.newCall(request).execute().use { response ->
                if (!response.isSuccessful) {
                    val errMsg = response.body?.string() ?: "HTTP ${response.code}"
                    return@withContext Result.failure(RuntimeException("获取撤回预览失败: $errMsg"))
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
            ?: return@withContext Result.failure(IllegalStateException("网关地址未配置"))

        try {
            val endpoint = "$baseUrl/gateway/cascade/revert/execute"
            val reqPayload = json.encodeToString(RevertExecuteRequest(cascadeId = cascadeId, stepIndex = stepIndex, conversationOnly = conversationOnly))
            val request = buildAuthorizedRequest(endpoint)
                .post(reqPayload.toRequestBody(jsonMediaType))
                .build()

            client.newCall(request).execute().use { response ->
                if (!response.isSuccessful) {
                    val errMsg = response.body?.string() ?: "HTTP ${response.code}"
                    return@withContext Result.failure(RuntimeException("执行撤回失败: $errMsg"))
                }
                Result.success(Unit)
            }
        } catch (e: Exception) {
            Result.failure(e)
        }
    }

    private fun buildAuthorizedRequest(url: String): Request.Builder {
        val builder = Request.Builder()
            .url(url)
            .header("Content-Type", "application/json")
            .header("Connect-Protocol-Version", "1")

        prefs.deviceToken?.takeIf { it.isNotBlank() }?.let { token ->
            builder.header("Authorization", "Bearer $token")
            builder.header("x-device-token", token)
        }
        return builder
    }
}
