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
import java.net.URLEncoder
import java.time.Instant
import java.util.concurrent.TimeUnit

class ApiClient(private val prefs: PreferencesManager) {
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

                    prefs.gatewayBaseUrl = candidate
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
        val baseUrl = prefs.gatewayBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址"))
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
                }.sortedByDescending { it.lastModifiedTime ?: "" }

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
        val baseUrl = prefs.gatewayBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址"))
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
        val baseUrl = prefs.gatewayBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址"))
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
        val baseUrl = prefs.gatewayBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址"))
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
        val baseUrl = prefs.gatewayBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址"))
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
        val baseUrl = prefs.gatewayBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址"))
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
        val baseUrl = prefs.gatewayBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址"))
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
        val baseUrl = prefs.gatewayBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址"))
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
        val baseUrl = prefs.gatewayBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址"))
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
        images: List<Pair<ByteArray, String>> = emptyList()
    ): Result<Unit> = withContext(Dispatchers.IO) {
        val baseUrl = prefs.gatewayBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址"))
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
            model?.let { put("model", it) }
        }

        try {
            val req = buildAuthorizedRequest(url)
                .post(payload.toString().toRequestBody(jsonMediaType))
                .build()

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
     * Cancel ongoing invocation
     */
    suspend fun cancelInvocation(cascadeId: String): Result<Unit> = withContext(Dispatchers.IO) {
        val baseUrl = prefs.gatewayBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址"))
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
        val baseUrl = prefs.gatewayBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址"))
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
        val baseUrl = prefs.gatewayBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址"))
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
     * Proceed with artifact plan execution
     */
    suspend fun proceedArtifact(cascadeId: String, artifactUri: String): Result<Unit> = withContext(Dispatchers.IO) {
        val baseUrl = prefs.gatewayBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址"))
        val url = "$baseUrl/api/exa.language_server_pb.LanguageServerService/SendUserCascadeMessage"

        val payload = buildJsonObject {
            put("cascadeId", cascadeId)
            put("text", "Proceed with implementation plan.")
            put("media", JsonArray(emptyList()))
        }

        try {
            val req = buildAuthorizedRequest(url)
                .post(payload.toString().toRequestBody(jsonMediaType))
                .build()

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
        val baseUrl = prefs.gatewayBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址"))
        val encodedUri = URLEncoder.encode(uri, "UTF-8")
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
     * Report session focus to gateway immediately on tap
     */
    suspend fun notifySessionFocus(cascadeId: String) = withContext(Dispatchers.IO) {
        val baseUrl = prefs.gatewayBaseUrl ?: return@withContext
        val url = "$baseUrl/gateway/cascade/focus"
        val payload = buildJsonObject {
            put("cascadeId", cascadeId)
            put("source", "android")
        }
        try {
            val req = buildAuthorizedRequest(url)
                .post(payload.toString().toRequestBody(jsonMediaType))
                .build()
            client.newCall(req).execute().close()
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
        val baseUrl = prefs.gatewayBaseUrl ?: return@withContext
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
