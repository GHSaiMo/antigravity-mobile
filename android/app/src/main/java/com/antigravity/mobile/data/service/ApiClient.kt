package com.antigravity.mobile.data.service

import android.os.Build
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
import java.util.concurrent.TimeUnit

class ApiClient(private val prefs: PreferencesManager) {
    private val json = Json {
        ignoreUnknownKeys = true
        isLenient = true
        encodeDefaults = true
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

                    // Persist matched base URL and tokens
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
     * Fetch all conversation trajectories via ConnectRPC
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
                val rootJson = json.parseToJsonElement(bodyStr).jsonObject
                val summaries = rootJson["trajectorySummaries"]?.jsonObject ?: JsonObject(emptyMap())

                val list = mutableListOf<ConversationItem>()
                for ((cascadeId, summaryEl) in summaries) {
                    val summaryObj = summaryEl.jsonObject
                    val isSubagent = summaryObj["isSubagent"]?.jsonPrimitive?.booleanOrNull ?: false
                    if (isSubagent) continue

                    val title = summaryObj["title"]?.jsonPrimitive?.contentOrNull
                    val stepCount = summaryObj["stepCount"]?.jsonPrimitive?.intOrNull ?: 0
                    val lastModified = summaryObj["lastModified"]?.jsonPrimitive?.contentOrNull
                    val workspace = summaryObj["workspaceFolder"]?.jsonPrimitive?.contentOrNull
                    val status = summaryObj["status"]?.jsonPrimitive?.contentOrNull ?: "IDLE"
                    val hasError = summaryObj["hasError"]?.jsonPrimitive?.booleanOrNull ?: false

                    list.add(
                        ConversationItem(
                            cascadeId = cascadeId,
                            title = title,
                            stepCount = stepCount,
                            updatedAt = lastModified,
                            workspaceFolder = workspace,
                            status = status,
                            hasError = hasError
                        )
                    )
                }

                // Sort descending by updated time
                val sorted = list.sortedByDescending { it.updatedAt ?: "" }
                Result.success(sorted)
            }
        } catch (e: Exception) {
            Result.failure(e)
        }
    }

    /**
     * Send user message to a cascade session
     */
    suspend fun sendMessage(cascadeId: String, text: String): Result<Unit> = withContext(Dispatchers.IO) {
        val baseUrl = prefs.gatewayBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址"))
        val url = "$baseUrl/api/exa.language_server_pb.LanguageServerService/SendUserCascadeMessage"

        val payload = buildJsonObject {
            put("cascadeId", cascadeId)
            put("text", text)
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

        // Default Proceed confirmation prompt
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
