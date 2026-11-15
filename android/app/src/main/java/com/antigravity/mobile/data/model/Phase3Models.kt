package com.antigravity.mobile.data.model

import kotlinx.serialization.Serializable
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject

@Serializable
data class ToolDetailItem(
    val name: String = "",
    val detail: String = "",
    val status: String = "",
    val duration: String? = null
)

@Serializable
data class ContextMentionItem(
    val id: String = "",
    val type: String = "file", // "file" | "directory" | "conversation"
    val label: String = "",
    val detail: String? = null,
    val path: String? = null,
    val uri: String? = null,
    val cascadeId: String? = null,
    val scopeItem: JsonElement? = null
)

@Serializable
data class ContextMentionsResponse(
    val ok: Boolean = false,
    val items: List<ContextMentionItem> = emptyList()
)

@Serializable
data class ForkConversationResponse(
    val ok: Boolean = false,
    val cascadeId: String? = null,
    val worktreePath: String? = null,
    val error: String? = null
)

@Serializable
data class WorktreeCreateResponse(
    val ok: Boolean = false,
    val worktreePath: String? = null,
    val worktreeName: String? = null,
    val branch: String? = null,
    val cascadeId: String? = null,
    val error: String? = null
)

@Serializable
data class CloneRepoResponse(
    val ok: Boolean = false,
    val path: String? = null,
    val folderUri: String? = null,
    val cascadeId: String? = null,
    val error: String? = null
)

@Serializable
data class PermissionStateResponse(
    val ok: Boolean = true,
    val preset: String = "strict", // "strict" | "sandbox" | "auto"
    val label: String = "严格审批",
    val isUnlocked: Boolean = false
)

@Serializable
data class MemoryRuleItem(
    val id: String = "",
    val scope: String = "memory", // "global" | "workspace" | "memory"
    val title: String = "",
    val path: String = "",
    val content: String = "",
    val readOnly: Boolean = false
)

@Serializable
data class MemoriesResponse(
    val ok: Boolean = true,
    val items: List<MemoryRuleItem> = emptyList()
)

@Serializable
data class TerminalItem(
    val terminalId: String = "",
    val id: String = "",
    val name: String? = null,
    val title: String? = null,
    val command: String? = null,
    val status: String? = null
) {
    val effectiveId: String get() = terminalId.ifBlank { id }
    val displayTitle: String get() = name?.takeIf { it.isNotBlank() }
        ?: title?.takeIf { it.isNotBlank() }
        ?: command?.takeIf { it.isNotBlank() }
        ?: effectiveId.take(10)
}

@Serializable
data class TerminalsResponse(
    val ok: Boolean = true,
    val terminals: List<TerminalItem> = emptyList()
)

@Serializable
data class BrowserSubagentResponse(
    val ok: Boolean = true,
    val url: String? = null,
    val imageBase64: String? = null,
    val screenshotUri: String? = null
)

@Serializable
data class GitBranchesResponse(
    val ok: Boolean = true,
    val current: String = "",
    val branches: List<String> = emptyList()
)
