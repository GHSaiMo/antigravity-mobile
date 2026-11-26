package com.antigravity.mobile.data.service

import com.antigravity.mobile.data.model.*
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import java.net.URLEncoder

private fun urlEnc(s: String): String = URLEncoder.encode(s, "UTF-8")

// region 1. BrowserViewerService

suspend fun ApiClient.fetchBrowserTabs(): Result<BrowserTabsResponse> =
    p3Get("/gateway/browser/tabs").mapCatching { json.decodeFromString(it) }

suspend fun ApiClient.openBrowserView(
    tabId: String? = null,
    widthPx: Int = 0,
    heightPx: Int = 0,
    leaseSeconds: Int = 60
): Result<BrowserOpenViewResponse> {
    val payload = buildJsonObject {
        if (!tabId.isNullOrBlank()) put("tabId", tabId)
        if (widthPx > 0) put("widthPx", widthPx)
        if (heightPx > 0) put("heightPx", heightPx)
        put("leaseSeconds", leaseSeconds)
    }
    return p3PostJson("/gateway/browser/open", payload).mapCatching { json.decodeFromString(it) }
}

suspend fun ApiClient.fetchBrowserFrame(sessionId: String, tabId: String): Result<BrowserFrameResponse> {
    val path = "/gateway/browser/frame?sessionId=${urlEnc(sessionId)}&tabId=${urlEnc(tabId)}"
    return p3Get(path).mapCatching { json.decodeFromString(it) }
}

suspend fun ApiClient.takeoverBrowserTab(
    sessionId: String,
    tabId: String,
    leaseSeconds: Int = 60
): Result<BrowserTakeoverResponse> {
    val payload = buildJsonObject {
        put("sessionId", sessionId)
        put("tabId", tabId)
        put("leaseSeconds", leaseSeconds)
    }
    return p3PostJson("/gateway/browser/takeover", payload).mapCatching { json.decodeFromString(it) }
}

suspend fun ApiClient.controlBrowserTab(
    sessionId: String,
    tabId: String,
    controlToken: String,
    action: String,
    url: String? = null,
    inputKind: String? = null,
    xPx: Int? = null,
    yPx: Int? = null,
    deltaX: Double? = null,
    deltaY: Double? = null,
    key: String? = null,
    text: String? = null
): Result<BrowserControlResponse> {
    val payload = buildJsonObject {
        put("sessionId", sessionId)
        put("tabId", tabId)
        put("controlToken", controlToken)
        put("action", action)
        if (!url.isNullOrBlank()) put("url", url)
        if (!inputKind.isNullOrBlank()) {
            put("inputEvent", buildJsonObject {
                put("kind", inputKind)
                if (xPx != null) put("xPx", xPx)
                if (yPx != null) put("yPx", yPx)
                if (deltaX != null) put("deltaX", deltaX)
                if (deltaY != null) put("deltaY", deltaY)
                if (!key.isNullOrBlank()) put("key", key)
                if (!text.isNullOrBlank()) put("text", text)
                if (inputKind == "KIND_POINTER_DOWN" || inputKind == "KIND_POINTER_UP") {
                    put("button", "MOUSE_BUTTON_LEFT")
                }
            })
        }
    }
    return p3PostJson("/gateway/browser/control", payload).mapCatching { json.decodeFromString(it) }
}

suspend fun ApiClient.releaseBrowserControl(sessionId: String, tabId: String, controlToken: String): Result<Unit> =
    p3PostJson("/gateway/browser/release", buildJsonObject {
        put("sessionId", sessionId)
        put("tabId", tabId)
        put("controlToken", controlToken)
    }).map { }

suspend fun ApiClient.renewBrowserView(sessionId: String, tabId: String, leaseSeconds: Int = 60): Result<Unit> =
    p3PostJson("/gateway/browser/renew", buildJsonObject {
        put("sessionId", sessionId)
        put("tabId", tabId)
        put("leaseSeconds", leaseSeconds)
    }).map { }

suspend fun ApiClient.closeBrowserView(sessionId: String, tabId: String): Result<Unit> =
    p3PostJson("/gateway/browser/close", buildJsonObject {
        put("sessionId", sessionId)
        put("tabId", tabId)
    }).map { }

// endregion

// region 2. RemotingService (SSH Remote Hosts)

suspend fun ApiClient.fetchRemotes(): Result<RemotesListResponse> =
    p3Get("/gateway/remotes").mapCatching { json.decodeFromString(it) }

suspend fun ApiClient.connectRemoteHost(
    name: String,
    host: String? = null,
    user: String? = null,
    port: Int? = null,
    remoteId: String? = null
): Result<RemoteConnectJobResponse> {
    val payload = buildJsonObject {
        if (name.isNotBlank()) put("name", name)
        if (!remoteId.isNullOrBlank()) put("remoteId", remoteId)
        if (!host.isNullOrBlank()) put("host", host)
        if (!user.isNullOrBlank()) put("user", user)
        if (port != null && port > 0) put("port", port)
    }
    return p3PostJson("/gateway/remotes/connect", payload).mapCatching { json.decodeFromString(it) }
}

suspend fun ApiClient.fetchRemoteConnectStatus(jobId: String): Result<RemoteConnectJobResponse> =
    p3Get("/gateway/remotes/connect/status?jobId=${urlEnc(jobId)}").mapCatching { json.decodeFromString(it) }

suspend fun ApiClient.respondRemotePrompt(jobId: String, response: String): Result<Unit> =
    p3PostJson("/gateway/remotes/prompt", buildJsonObject {
        put("jobId", jobId)
        put("response", response)
    }).map { }

suspend fun ApiClient.disconnectRemoteHost(remoteId: String, name: String? = null): Result<Unit> =
    p3PostJson("/gateway/remotes/disconnect", buildJsonObject {
        put("remoteId", remoteId)
        if (!name.isNullOrBlank()) put("name", name)
    }).map { }

suspend fun ApiClient.saveRemoteHost(
    name: String,
    host: String,
    user: String? = null,
    port: Int = 22
): Result<Unit> = p3PostJson("/gateway/remotes/save", buildJsonObject {
    put("name", name)
    put("host", host)
    if (!user.isNullOrBlank()) put("user", user)
    if (port > 0) put("port", port)
}).map { }

suspend fun ApiClient.deleteRemoteHost(remoteId: String, name: String? = null): Result<Unit> =
    p3PostJson("/gateway/remotes/delete", buildJsonObject {
        put("remoteId", remoteId)
        if (!name.isNullOrBlank()) put("name", name)
    }).map { }

// endregion

// region 3. Marketplace, Plugin Uninstall & Plugin/MCP OAuth

suspend fun ApiClient.searchMarketplace(query: String, kind: String? = null, limit: Int = 20): Result<MarketplaceSearchResponse> {
    val params = mutableListOf("q=${urlEnc(query)}", "limit=$limit")
    if (!kind.isNullOrBlank()) params.add("kind=${urlEnc(kind)}")
    return p3Get("/gateway/marketplace/search?${params.joinToString("&")}").mapCatching { json.decodeFromString(it) }
}

suspend fun ApiClient.fetchMarketplaceFeatured(): Result<MarketplaceSearchResponse> =
    p3Get("/gateway/marketplace/featured").mapCatching { json.decodeFromString(it) }

suspend fun ApiClient.deletePlugin(pluginId: String): Result<Unit> =
    p3PostJson("/gateway/plugins/delete", buildJsonObject { put("pluginId", pluginId) }).map { }

suspend fun ApiClient.startPluginAuthSignIn(plugin: String, provider: String, accountId: String? = null): Result<OAuthStartResponse> =
    p3PostJson("/gateway/plugins/auth/start", buildJsonObject {
        put("plugin", plugin)
        put("provider", provider)
        if (!accountId.isNullOrBlank()) put("accountId", accountId)
    }).mapCatching { json.decodeFromString(it) }

suspend fun ApiClient.signOutPluginAuthProvider(plugin: String, provider: String, accountId: String? = null): Result<Unit> =
    p3PostJson("/gateway/plugins/auth/signout", buildJsonObject {
        put("plugin", plugin)
        put("provider", provider)
        if (!accountId.isNullOrBlank()) put("accountId", accountId)
    }).map { }

suspend fun ApiClient.setDefaultPluginAuthAccount(plugin: String, provider: String, accountId: String): Result<Unit> =
    p3PostJson("/gateway/plugins/auth/default", buildJsonObject {
        put("plugin", plugin)
        put("provider", provider)
        put("accountId", accountId)
    }).map { }

suspend fun ApiClient.startMcpOAuthSignIn(serverName: String): Result<OAuthStartResponse> =
    p3PostJson("/gateway/mcp/oauth/start", buildJsonObject {
        put("serverName", serverName)
    }).mapCatching { json.decodeFromString(it) }

suspend fun ApiClient.completeMcpOAuth(serverName: String, code: String, state: String? = null): Result<Unit> =
    p3PostJson("/gateway/mcp/oauth/complete", buildJsonObject {
        put("serverName", serverName)
        put("code", code)
        if (!state.isNullOrBlank()) put("state", state)
    }).map { }

suspend fun ApiClient.disconnectMcpOAuth(serverName: String): Result<Unit> =
    p3PostJson("/gateway/mcp/oauth/disconnect", buildJsonObject {
        put("serverName", serverName)
    }).map { }

suspend fun ApiClient.forwardLoopbackOAuthCallback(callbackUrl: String): Result<OAuthCallbackForwardResponse> =
    p3PostJson("/gateway/oauth/callback-forward", buildJsonObject {
        put("callbackUrl", callbackUrl)
    }).mapCatching { json.decodeFromString(it) }

// endregion

// region 4. Desktop Self-Update & Changelog

suspend fun ApiClient.fetchDesktopUpdateStatus(): Result<DesktopUpdateStatus> =
    p3Get("/gateway/desktop/update").mapCatching { json.decodeFromString(it) }

suspend fun ApiClient.fetchDesktopChangelog(version: String? = null): Result<DesktopChangelogResponse> {
    val q = if (!version.isNullOrBlank()) "?version=${urlEnc(version)}" else ""
    return p3Get("/gateway/desktop/changelog$q").mapCatching { json.decodeFromString(it) }
}

suspend fun ApiClient.applyDesktopUpdate(): Result<DesktopApplyUpdateResponse> =
    p3PostJson("/gateway/desktop/update/apply", buildJsonObject { put("confirm", true) })
        .mapCatching { json.decodeFromString(it) }

suspend fun ApiClient.fetchGatewaySelfUpdateStatus(refresh: Boolean = false): Result<GatewaySelfUpdateStatus> {
    val path = if (refresh) "/gateway/self/update?refresh=1" else "/gateway/self/update"
    return p3Get(path).mapCatching { json.decodeFromString(it) }
}

suspend fun ApiClient.applyGatewaySelfUpdate(): Result<GatewaySelfUpdateApplyResponse> =
    p3PostJson("/gateway/self/update/apply", buildJsonObject { put("confirm", true) })
        .mapCatching { json.decodeFromString(it) }

// endregion

// region 5. Workspace Code & File Search

suspend fun ApiClient.searchWorkspaceCode(
    cascadeId: String,
    query: String,
    isRegex: Boolean = false,
    caseSensitive: Boolean = false,
    maxResults: Int = 60
): Result<List<CodeSearchHit>> {
    val payload = buildJsonObject {
        put("cascadeId", cascadeId)
        put("query", query)
        put("isRegex", isRegex)
        put("caseSensitive", caseSensitive)
        put("maxResults", maxResults)
    }
    return p3PostJson("/gateway/search/code", payload).mapCatching {
        json.decodeFromString<CodeSearchResponse>(it).results
    }
}

suspend fun ApiClient.searchWorkspaceFiles(
    cascadeId: String,
    query: String,
    maxResults: Int = 60
): Result<List<FileSearchHit>> {
    val payload = buildJsonObject {
        put("cascadeId", cascadeId)
        put("query", query)
        put("maxResults", maxResults)
    }
    return p3PostJson("/gateway/search/files", payload).mapCatching {
        json.decodeFromString<FileSearchResponse>(it).results
    }
}

// endregion

// region 6. Artifact Version Snapshots

suspend fun ApiClient.fetchArtifactSnapshots(
    cascadeId: String,
    artifactUri: String? = null
): Result<List<ArtifactSnapshotItem>> {
    val params = mutableListOf("cascadeId=${urlEnc(cascadeId)}")
    if (!artifactUri.isNullOrBlank()) params.add("artifactUri=${urlEnc(artifactUri)}")
    return p3Get("/gateway/artifacts/snapshots?${params.joinToString("&")}").mapCatching {
        json.decodeFromString<ArtifactSnapshotsResponse>(it).snapshots
    }
}

// endregion

// region 7. Worktree PR & Conversation Starring

suspend fun ApiClient.updateWorktreePR(
    cascadeId: String,
    title: String? = null,
    body: String? = null,
    draft: Boolean = false
): Result<WorktreePRResponse> =
    p3PostJson("/gateway/worktree/pr", buildJsonObject {
        put("cascadeId", cascadeId)
        if (!title.isNullOrBlank()) put("title", title)
        if (!body.isNullOrBlank()) put("body", body)
        put("draft", draft)
    }).mapCatching { json.decodeFromString(it) }

suspend fun ApiClient.setCascadeStarred(cascadeId: String, starred: Boolean): Result<StarCascadeResponse> =
    p3PostJson("/gateway/cascade/star", buildJsonObject {
        put("cascadeId", cascadeId)
        put("starred", starred)
    }).mapCatching { json.decodeFromString(it) }

// endregion
