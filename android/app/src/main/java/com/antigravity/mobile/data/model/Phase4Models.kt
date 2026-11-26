package com.antigravity.mobile.data.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

// region 1. BrowserViewerService (Remote Live Frame & Touch Takeover)

@Serializable
data class BrowserViewerTab(
    val tabId: String = "",
    val title: String? = null,
    val url: String? = null,
    val active: Boolean? = null,
    val agentOwned: Boolean? = null,
    val loading: Boolean? = null,
    val widthPx: Int? = null,
    val heightPx: Int? = null,
    val rootConversationId: String? = null
) {
    val displayTitle: String
        get() = title?.trim()?.takeIf { it.isNotEmpty() }
            ?: url?.trim()?.takeIf { it.isNotEmpty() }
            ?: tabId
}

@Serializable
data class BrowserTabsResponse(
    val ok: Boolean = false,
    val hasBrowserHost: Boolean = false,
    val tabs: List<BrowserViewerTab> = emptyList(),
    val error: String? = null
)

@Serializable
data class BrowserOpenViewResponse(
    val ok: Boolean = false,
    val sessionId: String? = null,
    val streamId: String? = null,
    val expiresUnixMs: Long? = null,
    val tab: BrowserViewerTab? = null,
    val error: String? = null
)

@Serializable
data class BrowserFrameResponse(
    val ok: Boolean = false,
    val imageData: String? = null,
    val mimeType: String? = null,
    val widthPx: Int? = null,
    val heightPx: Int? = null,
    val tab: BrowserViewerTab? = null,
    val error: String? = null
)

@Serializable
data class BrowserTakeoverResponse(
    val ok: Boolean = false,
    val controlToken: String? = null,
    val controlExpiresUnixMs: Long? = null,
    val tab: BrowserViewerTab? = null,
    val error: String? = null
)

@Serializable
data class BrowserControlResponse(
    val ok: Boolean = false,
    val tab: BrowserViewerTab? = null,
    val error: String? = null
)

// endregion

// region 2. RemotingService (SSH Remote Hosts & Interactive Prompts)

@Serializable
data class RemoteHostHop(
    val host: String? = null,
    val user: String? = null,
    val port: Int? = null
)

@Serializable
data class RemoteMultiHop(
    val hops: List<RemoteHostHop> = emptyList()
)

@Serializable
data class RemoteHostDescriptor(
    val remoteId: String? = null,
    val name: String? = null,
    val description: String? = null,
    val type: String? = null,
    val remoteMultiHop: RemoteMultiHop? = null
) {
    val stableId: String
        get() = remoteId?.takeIf { it.isNotBlank() }
            ?: name?.takeIf { it.isNotBlank() }
            ?: remoteMultiHop?.hops?.firstOrNull()?.host?.takeIf { it.isNotBlank() }
            ?: hashCode().toString()

    val displayName: String
        get() {
            name?.trim()?.takeIf { it.isNotEmpty() }?.let { return it }
            val hop = remoteMultiHop?.hops?.firstOrNull()
            val h = hop?.host?.trim().orEmpty()
            if (h.isNotEmpty()) {
                val u = hop?.user?.trim().orEmpty()
                return if (u.isNotEmpty()) "$u@$h" else h
            }
            return remoteId?.takeIf { it.isNotBlank() } ?: "SSH Host"
        }

    val hostSummary: String
        get() {
            val hop = remoteMultiHop?.hops?.firstOrNull()
            val h = hop?.host?.trim().orEmpty()
            if (h.isNotEmpty()) {
                val u = hop?.user?.trim().orEmpty()
                val userPrefix = if (u.isNotEmpty()) "$u@" else ""
                val portSuffix = if (hop?.port != null && hop.port != 22 && hop.port != 0) ":${hop.port}" else ""
                return "$userPrefix$h$portSuffix"
            }
            return description.orEmpty()
        }
}

@Serializable
data class RemoteConnectPrompt(
    val message: String = "",
    val isSecret: Boolean = false
)

@Serializable
data class RemoteConnectJob(
    val jobId: String = "",
    val name: String? = null,
    val remoteId: String? = null,
    val status: String = "", // "connecting" | "prompt" | "connected" | "error"
    val message: String? = null,
    val error: String? = null,
    val prompt: RemoteConnectPrompt? = null,
    val done: Boolean = false,
    val updatedAt: String? = null
)

@Serializable
data class EnvironmentItem(
    @SerialName("id") val envId: String? = null,
    val name: String? = null,
    val displayName: String? = null,
    val state: String? = null,
    val status: String? = null
) {
    val stableId: String get() = envId?.takeIf { it.isNotBlank() } ?: name?.takeIf { it.isNotBlank() } ?: hashCode().toString()
}

@Serializable
data class RemotesListResponse(
    val ok: Boolean = false,
    val remotes: List<RemoteHostDescriptor> = emptyList(),
    val connected: List<RemoteHostDescriptor> = emptyList(),
    val environments: List<EnvironmentItem> = emptyList(),
    val jobs: List<RemoteConnectJob> = emptyList(),
    val error: String? = null
)

@Serializable
data class RemoteConnectJobResponse(
    val ok: Boolean = false,
    val job: RemoteConnectJob? = null,
    val error: String? = null
)

// endregion

// region 3. Marketplace, Plugin Uninstall & Plugin/MCP OAuth

@Serializable
data class MarketplaceItem(
    val id: String = "",
    val name: String = "",
    val description: String? = null,
    val author: String? = null,
    val version: String? = null,
    val kind: String? = null,
    val installCount: Long? = null,
    val userCount: Long? = null,
    val homepage: String? = null,
    val repository: String? = null,
    val logo: String? = null,
    val marketplace: String? = null,
    val installed: Boolean = false
)

@Serializable
data class MarketplaceSearchResponse(
    val ok: Boolean = false,
    val items: List<MarketplaceItem> = emptyList(),
    val nextPageToken: String? = null,
    val error: String? = null
)

@Serializable
data class PluginAuthAccountInfo(
    val accountId: String = "",
    val alias: String? = null,
    val identity: String? = null,
    val label: String? = null,
    val isDefault: Boolean = false,
    val needsAuth: Boolean = false,
    val needsAuthReason: String? = null
) {
    val displayLabel: String
        get() = listOf(label, identity, alias)
            .firstOrNull { !it.isNullOrBlank() }
            ?.trim()
            ?: accountId
}

@Serializable
data class OAuthStartResponse(
    val ok: Boolean = false,
    val authUrl: String? = null,
    val accountId: String? = null,
    val mode: String? = null,
    val isLoopbackCallback: Boolean = false,
    val error: String? = null
)

@Serializable
data class OAuthCallbackForwardResponse(
    val ok: Boolean = false,
    val statusCode: Int? = null,
    val message: String? = null,
    val error: String? = null
)

// endregion

// region 4. Desktop Self-Update & Changelog

@Serializable
data class DesktopUpdateStatus(
    val ok: Boolean = false,
    val currentVersion: String? = null,
    val latestVersion: String? = null,
    val updateAvailable: Boolean = false,
    val runningCount: Int = 0,
    val error: String? = null
)

@Serializable
data class DesktopChangelogResponse(
    val ok: Boolean = false,
    val version: String? = null,
    val path: String? = null,
    val markdown: String? = null,
    val error: String? = null
)

@Serializable
data class DesktopApplyUpdateResponse(
    val ok: Boolean = false,
    val notifiedHost: Boolean = false,
    val message: String? = null,
    val error: String? = null
)

@Serializable
data class GatewaySelfUpdateStatus(
    val ok: Boolean = false,
    val currentVersion: String? = null,
    val latestVersion: String? = null,
    val tag: String? = null,
    val updateAvailable: Boolean = false,
    val canSelfUpdate: Boolean = false,
    val cannotUpdateReason: String? = null,
    val restartMode: String? = null,
    val os: String? = null,
    val arch: String? = null,
    val runningCount: Int = 0,
    val state: String = "idle",
    val stageMessage: String? = null,
    val targetVersion: String? = null,
    val error: String? = null,
    val checkError: String? = null,
    val releaseUrl: String? = null
) {
    val isInProgress: Boolean
        get() = state == "checking" || state == "downloading" || state == "verifying" || state == "replacing" || state == "restarting"
}

@Serializable
data class GatewaySelfUpdateApplyResponse(
    val ok: Boolean = false,
    val accepted: Boolean = false,
    val state: String? = null,
    val restartMode: String? = null,
    val message: String? = null,
    val error: String? = null
)

// endregion

// region 5. Workspace Code & File Search

@Serializable
data class CodeSearchHit(
    val path: String = "",
    val relativePath: String = "",
    val uri: String = "",
    val lineNumber: Int = 1,
    val snippet: String = "",
    val matchStartCol: Int? = null,
    val matchEndCol: Int? = null,
    val contextBefore: List<String> = emptyList(),
    val contextAfter: List<String> = emptyList()
) {
    val stableId: String get() = "$path:$lineNumber:${matchStartCol ?: 0}"
}

@Serializable
data class CodeSearchResponse(
    val ok: Boolean = false,
    val workspaceUri: String? = null,
    val source: String? = null,
    val results: List<CodeSearchHit> = emptyList(),
    val error: String? = null
)

@Serializable
data class FileSearchHit(
    val path: String = "",
    val relativePath: String = "",
    val uri: String = "",
    val isDirectory: Boolean = false
)

@Serializable
data class FileSearchResponse(
    val ok: Boolean = false,
    val workspaceUri: String? = null,
    val source: String? = null,
    val results: List<FileSearchHit> = emptyList(),
    val error: String? = null
)

// endregion

// region 6. Artifact Version Snapshots

@Serializable
data class ArtifactSnapshotItem(
    val index: Int = 0,
    val artifactName: String = "",
    val artifactUri: String = "",
    val content: String = "",
    val lastEdited: String? = null,
    val reviewState: String? = null,
    val summary: String? = null
) {
    val stableId: String get() = "$artifactUri#$index"
}

@Serializable
data class ArtifactSnapshotsResponse(
    val ok: Boolean = false,
    val cascadeId: String? = null,
    val snapshots: List<ArtifactSnapshotItem> = emptyList(),
    val error: String? = null
)

// endregion

// region 7. Worktree PR & Conversation Starring

@Serializable
data class WorktreePRResponse(
    val ok: Boolean = false,
    val prUrl: String? = null,
    val error: String? = null
)

@Serializable
data class StarCascadeResponse(
    val ok: Boolean = false,
    val cascadeId: String? = null,
    val starred: Boolean = false,
    val error: String? = null
)

// endregion
