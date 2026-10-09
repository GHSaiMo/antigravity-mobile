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

// region 模型目录 / 默认模型

/** 网关 /gateway/models 返回的可选模型。provider 为 "gemini" | "claude" | "other"。 */
@Serializable
data class ModelOption(
    val id: String = "",
    val name: String = "",
    val provider: String = "other",
    val enum: String? = null,
    val supportsImages: Boolean = false,
    val thinking: Boolean = false
)

@Serializable
data class ModelsResponse(
    val models: List<ModelOption> = emptyList(),
    /** 网关建议的各厂商默认模型（provider -> model id）。 */
    val defaults: Map<String, String> = emptyMap(),
    /** false 表示网关暂时取不到实时列表，返回的是内置兜底。 */
    val live: Boolean = false
)

/** 默认模型与当前会话模型的解析规则（纯函数，便于单测）。 */
object ModelDefaults {
    const val FALLBACK_GEMINI = "gemini-3.8-flash-high"
    const val FALLBACK_CLAUDE = "claude-opus-4-6-thinking"

    fun isClaude(modelId: String): Boolean =
        modelId.contains("claude", ignoreCase = true) || modelId.contains("m26", ignoreCase = true)

    /** 输入框上方胶囊的文字：只有厂商名，保持简洁。 */
    fun providerLabel(modelId: String): String = when {
        isClaude(modelId) -> "Claude"
        modelId.contains("gpt", ignoreCase = true) -> "GPT"
        else -> "Gemini"
    }

    /** 胶囊点击后要切到的默认模型：Claude → Gemini 默认；其它 → Claude 默认。 */
    fun toggleTarget(current: String, geminiDefault: String, claudeDefault: String): String =
        if (isClaude(current)) geminiDefault else claudeDefault

    /**
     * 网关推送的 activeModel 如何落到界面状态：
     * - 为空：保持当前；
     * - 已是可读模型 id：原样保留，让后续消息继续用该会话实际的模型；
     * - 只拿到未解析的裸枚举（MODEL_...）：退回该厂商的默认模型。
     */
    fun resolveActive(raw: String?, current: String, geminiDefault: String, claudeDefault: String): String {
        val value = raw?.trim().orEmpty()
        if (value.isEmpty()) return current
        if (value.startsWith("MODEL_", ignoreCase = true)) {
            return if (isClaude(value)) claudeDefault else geminiDefault
        }
        return value
    }

    /** 设置里保存的默认模型已不在实时列表时，换成网关建议的默认值。 */
    fun validated(saved: String, provider: String, catalog: ModelsResponse): String {
        if (!catalog.live) return saved
        if (catalog.models.any { it.id == saved && it.provider == provider }) return saved
        return catalog.defaults[provider]
            ?: catalog.models.firstOrNull { it.provider == provider }?.id
            ?: saved
    }
}

