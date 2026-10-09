package com.antigravity.mobile.data.model

import kotlinx.serialization.Serializable

/** 本会话改动（正向累计 diff）。文件结构与撤回预览一致，直接复用 [RevertPreviewFile] 与 Diff 渲染。 */
@Serializable
data class CascadeChangesRequest(
    val cascadeId: String = "",
    val fromStepIndex: Int? = null
)

@Serializable
data class CascadeChangesResponse(
    val cascadeId: String = "",
    val files: List<RevertPreviewFile> = emptyList(),
    val additions: Int = 0,
    val deletions: Int = 0,
    val hasChanges: Boolean = false
)

/** Git 工作区状态与提交。 */
@Serializable
data class GitFileStatus(
    val path: String = "",
    val origPath: String? = null,
    val index: String = "",
    val worktree: String = "",
    val staged: Boolean = false,
    val status: String = "MODIFIED" // MODIFIED / ADDED / DELETED / RENAMED / UNTRACKED / CONFLICT
)

@Serializable
data class GitStatusResponse(
    val repoName: String = "",
    val branch: String = "",
    val upstream: String? = null,
    val ahead: Int = 0,
    val behind: Int = 0,
    val detached: Boolean = false,
    val files: List<GitFileStatus> = emptyList(),
    val clean: Boolean = false
)

@Serializable
data class GitCommitRequest(
    val cascadeId: String = "",
    val message: String = "",
    val paths: List<String> = emptyList(),
    val push: Boolean = false
)

@Serializable
data class GitCommitResponse(
    val committed: Boolean = false,
    val pushed: Boolean = false,
    val commitId: String? = null,
    val output: String? = null,
    val pushError: String? = null
)

@Serializable
data class GitCascadeRequest(val cascadeId: String = "")

/** 会话内容搜索（language_server SearchConversations）。偏移量按 Unicode 码点计，不是 UTF-16 单元。 */
@Serializable
data class SearchMatchRange(
    val startOffset: Int = 0,
    val endOffsetExclusive: Int = 0
)

@Serializable
data class ConversationSearchResult(
    val cascadeId: String = "",
    val title: String = "",
    val workspaceName: String? = null,
    val lastModifiedTime: String? = null,
    val snippet: String = "",
    val snippetMatchRanges: List<SearchMatchRange> = emptyList(),
    val matchSource: String = "",
    val matchedStepIndex: Int? = null
)

@Serializable
data class ConversationSearchResponse(
    val results: List<ConversationSearchResult> = emptyList()
)

@Serializable
data class MarkdownExportResponse(val markdown: String = "")

