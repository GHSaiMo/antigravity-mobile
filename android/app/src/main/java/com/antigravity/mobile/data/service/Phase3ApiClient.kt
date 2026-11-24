package com.antigravity.mobile.data.service

import com.antigravity.mobile.data.model.*
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import kotlinx.serialization.json.putJsonArray
import okhttp3.Call
import okhttp3.Callback
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import java.io.IOException
import java.net.URLEncoder
import kotlin.coroutines.resume
import kotlin.coroutines.resumeWithException

private val p3JsonMediaType = "application/json; charset=utf-8".toMediaType()

private suspend fun Call.awaitP3(): Response = suspendCancellableCoroutine { cont ->
    cont.invokeOnCancellation {
        try { cancel() } catch (_: Throwable) {}
    }
    enqueue(object : Callback {
        override fun onResponse(call: Call, response: Response) {
            if (cont.isCancelled) response.close() else cont.resume(response)
        }
        override fun onFailure(call: Call, e: IOException) {
            cont.resumeWithException(e)
        }
    })
}

internal suspend fun ApiClient.p3Get(path: String): Result<String> = withContext(Dispatchers.IO) {
    val base = currentBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址"))
    try {
        val req = buildAuthorizedRequest("$base$path").get().build()
        okHttpClient.newCall(req).awaitP3().use { resp ->
            val body = resp.body?.string().orEmpty()
            if (resp.isSuccessful) Result.success(body)
            else Result.failure(RuntimeException("HTTP ${resp.code}: ${body.take(180)}"))
        }
    } catch (e: Exception) {
        Result.failure(e)
    }
}

internal suspend fun ApiClient.p3PostJson(path: String, payload: JsonObject): Result<String> = withContext(Dispatchers.IO) {
    val base = currentBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址"))
    try {
        val req = buildAuthorizedRequest("$base$path")
            .post(payload.toString().toRequestBody(p3JsonMediaType))
            .build()
        okHttpClient.newCall(req).awaitP3().use { resp ->
            val body = resp.body?.string().orEmpty()
            if (resp.isSuccessful) Result.success(body)
            else Result.failure(RuntimeException("HTTP ${resp.code}: ${body.take(180)}"))
        }
    } catch (e: Exception) {
        Result.failure(e)
    }
}

suspend fun ApiClient.fetchContextMentions(query: String, cascadeId: String, workspaceUri: String?): Result<List<ContextMentionItem>> {
    val payload = buildJsonObject {
        put("query", query)
        put("cascadeId", cascadeId)
        if (!workspaceUri.isNullOrBlank()) put("workspaceUri", workspaceUri)
        put("limit", 25)
    }
    return p3PostJson("/gateway/context/mentions", payload).mapCatching { body ->
        json.decodeFromString<ContextMentionsResponse>(body).items
    }
}

suspend fun ApiClient.forkConversation(cascadeId: String, stepIndex: Int, createWorktree: Boolean): Result<ForkConversationResponse> {
    val payload = buildJsonObject {
        put("cascadeId", cascadeId)
        put("stepIndex", stepIndex)
        put("createWorktree", createWorktree)
    }
    return p3PostJson("/gateway/cascade/fork", payload).mapCatching { body ->
        json.decodeFromString<ForkConversationResponse>(body)
    }
}

suspend fun ApiClient.createWorktreeSession(repoUri: String, workspacePath: String, startSession: Boolean = true): Result<WorktreeCreateResponse> {
    val payload = buildJsonObject {
        put("repoUri", repoUri)
        put("workspacePath", workspacePath)
        put("startSession", startSession)
    }
    return p3PostJson("/gateway/worktree/create", payload).mapCatching { body ->
        json.decodeFromString<WorktreeCreateResponse>(body)
    }
}

suspend fun ApiClient.cloneGitHubRepo(
    repoSlug: String,
    parentDir: String? = null,
    folderName: String? = null,
    startSession: Boolean = true
): Result<CloneRepoResponse> = startCloneGitHubRepo(
    repoSlug = repoSlug,
    parentDir = parentDir,
    folderName = folderName,
    asyncMode = false,
    importExisting = false,
    startSession = startSession
)

suspend fun ApiClient.fetchLocalDirs(
    path: String? = null,
    showHidden: Boolean = false
): Result<LocalDirsResponse> = withContext(Dispatchers.IO) {
    val base = currentBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址"))
    val params = mutableListOf<String>()
    if (!path.isNullOrBlank()) {
        params.add("path=${URLEncoder.encode(path.trim(), "UTF-8")}")
    }
    if (showHidden) {
        params.add("showHidden=true")
    }
    val query = if (params.isEmpty()) "" else "?" + params.joinToString("&")
    try {
        val req = buildAuthorizedRequest("$base/gateway/fs/dirs$query").get().build()
        okHttpClient.newCall(req).awaitP3().use { resp ->
            val body = resp.body?.string().orEmpty()
            val parsed = runCatching {
                if (body.trimStart().startsWith("{")) json.decodeFromString<LocalDirsResponse>(body) else null
            }.getOrNull()
            if (resp.isSuccessful && parsed != null && parsed.ok) {
                Result.success(parsed)
            } else {
                val errMsg = parsed?.error?.takeIf { it.isNotBlank() }
                    ?: "HTTP ${resp.code}: ${body.take(180)}"
                Result.failure(RuntimeException(errMsg))
            }
        }
    } catch (e: Exception) {
        Result.failure(e)
    }
}

suspend fun ApiClient.createLocalDir(
    parentPath: String? = null,
    name: String? = null,
    path: String? = null
): Result<CreateLocalDirResponse> = withContext(Dispatchers.IO) {
    val base = currentBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址"))
    val payload = buildJsonObject {
        if (!parentPath.isNullOrBlank()) put("parentPath", parentPath.trim())
        if (!name.isNullOrBlank()) put("name", name.trim())
        if (!path.isNullOrBlank()) put("path", path.trim())
    }
    try {
        val req = buildAuthorizedRequest("$base/gateway/fs/mkdir")
            .post(payload.toString().toRequestBody(p3JsonMediaType))
            .build()
        okHttpClient.newCall(req).awaitP3().use { resp ->
            val body = resp.body?.string().orEmpty()
            val parsed = runCatching {
                if (body.trimStart().startsWith("{")) json.decodeFromString<CreateLocalDirResponse>(body) else null
            }.getOrNull()
            if (resp.isSuccessful && parsed != null && parsed.ok) {
                Result.success(parsed)
            } else {
                val errMsg = parsed?.error?.takeIf { it.isNotBlank() }
                    ?: "HTTP ${resp.code}: ${body.take(180)}"
                Result.failure(RuntimeException(errMsg))
            }
        }
    } catch (e: Exception) {
        Result.failure(e)
    }
}

suspend fun ApiClient.startCloneGitHubRepo(
    repoSlug: String,
    parentDir: String? = null,
    folderName: String? = null,
    asyncMode: Boolean = true,
    importExisting: Boolean = false,
    startSession: Boolean = false
): Result<CloneRepoResponse> = withContext(Dispatchers.IO) {
    val base = currentBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址"))
    val payload = buildJsonObject {
        put("repoSlug", repoSlug.trim())
        if (!parentDir.isNullOrBlank()) put("parentDir", parentDir.trim())
        if (!folderName.isNullOrBlank()) put("folderName", folderName.trim())
        put("async", asyncMode)
        put("importExisting", importExisting)
        put("startSession", startSession)
    }
    try {
        val req = buildAuthorizedRequest("$base/gateway/projects/clone")
            .post(payload.toString().toRequestBody(p3JsonMediaType))
            .build()
        okHttpClient.newCall(req).awaitP3().use { resp ->
            val body = resp.body?.string().orEmpty()
            val parsed = runCatching {
                if (body.trimStart().startsWith("{")) json.decodeFromString<CloneRepoResponse>(body) else null
            }.getOrNull()
            if (resp.code == 409 && parsed != null) {
                return@withContext Result.success(parsed)
            }
            if (resp.isSuccessful && parsed != null) {
                Result.success(parsed)
            } else {
                val errMsg = parsed?.error?.takeIf { it.isNotBlank() }
                    ?: "HTTP ${resp.code}: ${body.take(180)}"
                Result.failure(RuntimeException(errMsg))
            }
        }
    } catch (e: Exception) {
        Result.failure(e)
    }
}

suspend fun ApiClient.fetchCloneGitHubRepoStatus(jobId: String): Result<CloneRepoResponse> = withContext(Dispatchers.IO) {
    val base = currentBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址"))
    val encoded = URLEncoder.encode(jobId.trim(), "UTF-8")
    try {
        val req = buildAuthorizedRequest("$base/gateway/projects/clone/status?jobId=$encoded").get().build()
        okHttpClient.newCall(req).awaitP3().use { resp ->
            val body = resp.body?.string().orEmpty()
            val parsed = runCatching {
                if (body.trimStart().startsWith("{")) json.decodeFromString<CloneRepoResponse>(body) else null
            }.getOrNull()
            if (resp.isSuccessful && parsed != null) {
                Result.success(parsed)
            } else {
                val errMsg = parsed?.error?.takeIf { it.isNotBlank() }
                    ?: "HTTP ${resp.code}: ${body.take(180)}"
                Result.failure(RuntimeException(errMsg))
            }
        }
    } catch (e: Exception) {
        Result.failure(e)
    }
}

suspend fun ApiClient.cancelCloneGitHubRepo(jobId: String): Result<CloneRepoResponse> {
    val payload = buildJsonObject {
        put("jobId", jobId.trim())
    }
    return p3PostJson("/gateway/projects/clone/cancel", payload).mapCatching { body ->
        json.decodeFromString<CloneRepoResponse>(body)
    }
}

suspend fun ApiClient.createProjectWorkspace(
    path: String,
    name: String? = null,
    startSession: Boolean = false
): Result<CreateProjectResponse> = withContext(Dispatchers.IO) {
    val base = currentBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址"))
    val payload = buildJsonObject {
        put("path", path)
        if (!name.isNullOrBlank()) put("name", name)
        put("startSession", startSession)
    }
    try {
        val req = buildAuthorizedRequest("$base/gateway/projects/create")
            .post(payload.toString().toRequestBody(p3JsonMediaType))
            .build()
        okHttpClient.newCall(req).awaitP3().use { resp ->
            val body = resp.body?.string().orEmpty()
            val parsed = runCatching {
                if (body.trimStart().startsWith("{")) json.decodeFromString<CreateProjectResponse>(body) else null
            }.getOrNull()
            if (resp.isSuccessful && parsed != null && parsed.ok) {
                Result.success(parsed)
            } else {
                val errMsg = parsed?.error?.takeIf { it.isNotBlank() }
                    ?: "HTTP ${resp.code}: ${body.take(180)}"
                Result.failure(RuntimeException(errMsg))
            }
        }
    } catch (e: Exception) {
        Result.failure(e)
    }
}

suspend fun ApiClient.fetchPermissionSettings(): Result<PermissionStateResponse> {
    return p3Get("/gateway/permissions").mapCatching { body ->
        json.decodeFromString<PermissionStateResponse>(body)
    }
}

suspend fun ApiClient.updatePermissionPreset(preset: String): Result<PermissionStateResponse> {
    return p3PostJson("/gateway/permissions", buildJsonObject { put("preset", preset) }).mapCatching { body ->
        json.decodeFromString<PermissionStateResponse>(body)
    }
}

suspend fun ApiClient.fetchMemories(workspacePath: String? = null): Result<List<MemoryRuleItem>> {
    val q = if (!workspacePath.isNullOrBlank()) "?workspacePath=${URLEncoder.encode(workspacePath, "UTF-8")}" else ""
    return p3Get("/gateway/memories$q").mapCatching { body ->
        json.decodeFromString<MemoriesResponse>(body).items
    }
}

suspend fun ApiClient.upsertMemory(
    scope: String,
    title: String,
    content: String,
    path: String? = null,
    workspacePath: String? = null
): Result<List<MemoryRuleItem>> {
    val payload = buildJsonObject {
        put("action", "upsert")
        put("scope", scope)
        put("title", title)
        put("content", content)
        if (!path.isNullOrBlank()) put("path", path)
        if (!workspacePath.isNullOrBlank()) put("workspacePath", workspacePath)
    }
    return p3PostJson("/gateway/memories", payload).mapCatching { body ->
        json.decodeFromString<MemoriesResponse>(body).items
    }
}

suspend fun ApiClient.deleteMemory(path: String): Result<List<MemoryRuleItem>> {
    val payload = buildJsonObject {
        put("action", "delete")
        put("path", path)
    }
    return p3PostJson("/gateway/memories", payload).mapCatching { body ->
        json.decodeFromString<MemoriesResponse>(body).items
    }
}

suspend fun ApiClient.listTerminals(): Result<List<TerminalItem>> {
    return p3Get("/gateway/terminals").mapCatching { body ->
        json.decodeFromString<TerminalsResponse>(body).terminals
    }
}

suspend fun ApiClient.createTerminal(): Result<TerminalItem> {
    return p3PostJson("/gateway/terminal/create", buildJsonObject {}).mapCatching { body ->
        val obj = json.parseToJsonElement(body) as? JsonObject
        val tid = obj?.get("terminalId")?.let { (it as? JsonPrimitive)?.content }.orEmpty()
        TerminalItem(terminalId = tid, id = tid, name = "Terminal")
    }
}

suspend fun ApiClient.sendTerminalInput(terminalId: String, input: String, conversationId: String = ""): Result<Unit> {
    val payload = buildJsonObject {
        put("terminalId", terminalId)
        put("input", input)
        if (conversationId.isNotBlank()) put("conversationId", conversationId)
    }
    return p3PostJson("/gateway/terminal/input", payload).map { }
}

suspend fun ApiClient.closeTerminal(terminalId: String): Result<Unit> {
    return p3PostJson("/gateway/terminal/close", buildJsonObject { put("terminalId", terminalId) }).map { }
}

fun ApiClient.openTerminalWebSocket(terminalId: String, listener: WebSocketListener): WebSocket? {
    val base = currentBaseUrl ?: return null
    val wsBase = base.replaceFirst("https://", "wss://").replaceFirst("http://", "ws://").trimEnd('/')
    val req = buildAuthorizedRequest("$wsBase/gateway/terminal/stream?terminalId=${URLEncoder.encode(terminalId, "UTF-8")}").build()
    return openWebSocket(req, listener)
}

suspend fun ApiClient.sendStepsToBackground(cascadeId: String, stepIndices: List<Int>): Result<Boolean> {
    val payload = buildJsonObject {
        put("cascadeId", cascadeId)
        putJsonArray("stepIndices") { stepIndices.forEach { add(JsonPrimitive(it)) } }
    }
    return p3PostJson("/gateway/cascade/task/background", payload).mapCatching { body ->
        val obj = json.parseToJsonElement(body) as? JsonObject
        (obj?.get("ok") as? JsonPrimitive)?.content == "true"
    }
}

suspend fun ApiClient.sendSubagentMessage(recipient: String, content: String): Result<Unit> {
    val payload = buildJsonObject {
        put("recipient", recipient)
        put("content", content)
    }
    return p3PostJson("/gateway/subagent/message", payload).map { }
}

suspend fun ApiClient.fetchBrowserSubagent(cascadeId: String): Result<BrowserSubagentResponse> {
    return p3Get("/gateway/subagent/browser?cascadeId=${URLEncoder.encode(cascadeId, "UTF-8")}").mapCatching { body ->
        json.decodeFromString<BrowserSubagentResponse>(body)
    }
}

suspend fun ApiClient.skipBrowserSubagent(cascadeId: String): Result<Unit> {
    return p3PostJson("/gateway/subagent/browser/skip", buildJsonObject { put("cascadeId", cascadeId) }).map { }
}

suspend fun ApiClient.whitelistBrowserUrl(url: String): Result<Unit> {
    return p3PostJson("/gateway/subagent/browser/whitelist", buildJsonObject { put("url", url) }).map { }
}

suspend fun ApiClient.toggleMcpServer(serverName: String, enabled: Boolean): Result<Unit> {
    return p3PostJson("/gateway/mcp/toggle", buildJsonObject {
        put("serverName", serverName)
        put("enabled", enabled)
    }).map { }
}

suspend fun ApiClient.refreshMcpServers(): Result<Unit> {
    return p3PostJson("/gateway/mcp/refresh", buildJsonObject {}).map { }
}

suspend fun ApiClient.callMcpAppTool(
    toolName: String,
    arguments: JsonElement?,
    appUri: String = "",
    serverName: String = ""
): Result<String> {
    val payload = buildJsonObject {
        put("toolName", toolName)
        if (arguments != null) put("arguments", arguments) else putJsonObject("arguments") {}
        if (appUri.isNotBlank()) put("appUri", appUri)
        if (serverName.isNotBlank()) put("serverName", serverName)
    }
    return p3PostJson("/gateway/mcp/app/call", payload)
}

suspend fun ApiClient.getMcpAppResource(uri: String, serverName: String = ""): Result<String> {
    val payload = buildJsonObject {
        put("uri", uri)
        if (serverName.isNotBlank()) put("serverName", serverName)
    }
    return p3PostJson("/gateway/mcp/app/resource", payload)
}

suspend fun ApiClient.createAutomation(
    name: String,
    schedule: String,
    prompt: String,
    model: String = "",
    workspace: String = ""
): Result<Unit> {
    val payload = buildJsonObject {
        put("name", name)
        put("cron", schedule)
        put("schedule", schedule)
        put("prompt", prompt)
        if (model.isNotBlank()) put("model", model)
        if (workspace.isNotBlank()) put("workspace", workspace)
    }
    return p3PostJson("/gateway/automations/create", payload).map { }
}

suspend fun ApiClient.updateAutomation(
    sidecarId: String,
    name: String? = null,
    description: String? = null,
    cron: String? = null,
    model: String? = null,
    prompt: String? = null
): Result<Unit> {
    val payload = buildJsonObject {
        put("sidecarId", sidecarId)
        if (name != null) put("name", name)
        if (description != null) put("description", description)
        if (cron != null) put("cron", cron)
        if (model != null) put("model", model)
        if (prompt != null) put("prompt", prompt)
    }
    return p3PostJson("/gateway/automations/update", payload).map { }
}

suspend fun ApiClient.fetchAutomationLogs(
    sidecarId: String,
    tail: Int = 200
): Result<AutomationLogsResponse> {
    val encodedId = URLEncoder.encode(sidecarId, "UTF-8")
    return p3Get("/gateway/automations/logs?sidecarId=$encodedId&tail=$tail").mapCatching { body ->
        json.decodeFromString<AutomationLogsResponse>(body)
    }
}

suspend fun ApiClient.deleteAutomation(sidecarId: String): Result<Unit> {
    return p3PostJson("/gateway/automations/delete", buildJsonObject { put("sidecarId", sidecarId) }).map { }
}

suspend fun ApiClient.gitListBranches(cascadeId: String): Result<GitBranchesResponse> {
    return p3PostJson("/gateway/git/branches", buildJsonObject { put("cascadeId", cascadeId) }).mapCatching { body ->
        json.decodeFromString<GitBranchesResponse>(body)
    }
}

suspend fun ApiClient.gitCheckoutBranch(cascadeId: String, branch: String, create: Boolean = false): Result<GitBranchesResponse> {
    return p3PostJson("/gateway/git/checkout", buildJsonObject {
        put("cascadeId", cascadeId)
        put("branch", branch)
        put("create", create)
    }).mapCatching { body ->
        json.decodeFromString<GitBranchesResponse>(body)
    }
}

suspend fun ApiClient.gitDiscardFile(cascadeId: String, path: String): Result<Unit> {
    return p3PostJson("/gateway/git/discard", buildJsonObject {
        put("cascadeId", cascadeId)
        put("path", path)
    }).map { }
}

suspend fun ApiClient.synthesizeNarrationAudio(text: String): Result<ByteArray> = withContext(Dispatchers.IO) {
    val base = currentBaseUrl ?: return@withContext Result.failure(IllegalStateException("未配置网关地址"))
    try {
        val req = buildAuthorizedRequest("$base/gateway/narration")
            .post(buildJsonObject { put("text", text) }.toString().toRequestBody(p3JsonMediaType))
            .build()
        okHttpClient.newCall(req).awaitP3().use { resp ->
            val bytes = resp.body?.bytes()
            if (resp.isSuccessful && bytes != null && bytes.isNotEmpty()) Result.success(bytes)
            else Result.failure(RuntimeException("语音合成失败: HTTP ${resp.code}"))
        }
    } catch (e: Exception) {
        Result.failure(e)
    }
}

private inline fun kotlinx.serialization.json.JsonObjectBuilder.putJsonObject(
    key: String,
    builderAction: kotlinx.serialization.json.JsonObjectBuilder.() -> Unit
) {
    put(key, buildJsonObject(builderAction))
}

suspend fun ApiClient.fetchProjectTrustStatus(cascadeId: String? = null, projectId: String? = null): Result<ProjectTrustStatusResponse> {
    val params = mutableListOf<String>()
    if (!projectId.isNullOrBlank()) params.add("projectId=${URLEncoder.encode(projectId, "UTF-8")}")
    if (!cascadeId.isNullOrBlank()) params.add("cascadeId=${URLEncoder.encode(cascadeId, "UTF-8")}")
    val path = if (params.isEmpty()) "/gateway/projects/trust" else "/gateway/projects/trust?${params.joinToString("&")}"
    return p3Get(path).mapCatching { body ->
        json.decodeFromString<ProjectTrustStatusResponse>(body)
    }
}

suspend fun ApiClient.setWorkspaceTrust(cascadeId: String? = null, projectId: String? = null, trusted: Boolean = true): Result<ProjectTrustStatusResponse> {
    val payload = buildJsonObject {
        put("trusted", trusted)
        if (!projectId.isNullOrBlank()) put("projectId", projectId)
        if (!cascadeId.isNullOrBlank()) put("cascadeId", cascadeId)
    }
    return p3PostJson("/gateway/projects/trust", payload).mapCatching { body ->
        json.decodeFromString<ProjectTrustStatusResponse>(body)
    }
}

suspend fun ApiClient.fetchGitHubPullRequests(urls: List<String>): Result<List<GitPullRequestItem>> {
    if (urls.isEmpty()) return Result.success(emptyList())
    val payload = buildJsonObject {
        putJsonArray("urls") { urls.forEach { add(JsonPrimitive(it)) } }
    }
    return p3PostJson("/gateway/github/pull-requests", payload).mapCatching { body ->
        json.decodeFromString<GitHubPullRequestsBatchResponse>(body).pullRequests
    }
}

suspend fun ApiClient.startBtwSideQuestion(
    parentCascadeId: String,
    question: String,
    quote: String? = null,
    model: String? = null
): Result<BtwStartResponse> {
    val payload = buildJsonObject {
        put("parentCascadeId", parentCascadeId)
        put("question", question)
        if (!quote.isNullOrBlank()) put("quote", quote)
        if (!model.isNullOrBlank()) put("model", model)
    }
    return p3PostJson("/gateway/btw/start", payload).mapCatching { body ->
        json.decodeFromString<BtwStartResponse>(body)
    }
}

suspend fun ApiClient.listBtwSideQuestions(cascadeId: String): Result<List<BtwSideQuestionEntry>> {
    val q = URLEncoder.encode(cascadeId, "UTF-8")
    return p3Get("/gateway/btw/list?cascadeId=$q").mapCatching { body ->
        json.decodeFromString<BtwListResponse>(body).items
    }
}

suspend fun ApiClient.fetchBtwStatus(cascadeId: String): Result<BtwStatusResponse> {
    val q = URLEncoder.encode(cascadeId, "UTF-8")
    return p3Get("/gateway/btw/status?cascadeId=$q").mapCatching { body ->
        json.decodeFromString<BtwStatusResponse>(body)
    }
}
