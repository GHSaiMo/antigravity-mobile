package com.antigravity.mobile.data.model

import com.antigravity.mobile.ui.util.loc
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject

@androidx.compose.runtime.Immutable
@Serializable
data class ThinkingBlockItem(
    val stepIndex: Int = 0,
    val text: String = "",
    val duration: String? = null,
    val status: String? = null
)

@androidx.compose.runtime.Immutable
@Serializable
data class ToolDetailItem(
    val name: String = "",
    val summary: String = "",
    val detail: String = "",
    val status: String = "",
    val duration: String? = null,
    val stepIndex: Int? = null,
    val kind: String? = null,
    val prefix: String? = null,
    val title: String? = null,
    val fileUri: String? = null,
    val fileName: String? = null,
    val startLine: Int? = null,
    val endLine: Int? = null,
    val additions: Int? = null,
    val deletions: Int? = null,
    val hasDiffStats: Boolean? = null,
    val numResults: Int? = null,
    val command: String? = null,
    val cwd: String? = null,
    val output: String? = null,
    val numLinesAbove: Int? = null,
    val exitCode: Int? = null,
    val markdown: String? = null,
    val errorText: String? = null,
    val isArtifact: Boolean? = null
) {
    val effectiveSummary: String get() = summary.ifBlank { detail }
}

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
    val async: Boolean = false,
    val jobId: String? = null,
    val repoSlug: String? = null,
    val status: String? = null,
    val phase: String? = null,
    val progress: Int = 0,
    val message: String? = null,
    val targetPath: String? = null,
    val path: String? = null,
    val folderUri: String? = null,
    val project: ProjectItem? = null,
    val alreadyExists: Boolean = false,
    val cascadeId: String? = null,
    val code: String? = null,
    val existingPath: String? = null,
    val error: String? = null
)

@Serializable
data class LocalDirQuickItem(
    val path: String = "",
    val name: String = "",
    val displayPath: String = "",
    val projectCount: Int = 0,
    val isDefault: Boolean = false,
    val exists: Boolean = true
)

@Serializable
data class LocalDirEntry(
    val name: String = "",
    val path: String = "",
    val isGitRepo: Boolean = false,
    val isProject: Boolean = false,
    val isHidden: Boolean = false
)

@Serializable
data class LocalDirsResponse(
    val ok: Boolean = false,
    val platform: String? = null,
    val homePath: String? = null,
    val defaultDir: String? = null,
    val currentPath: String = "",
    val currentDisplay: String? = null,
    val parentPath: String? = null,
    val canGoUp: Boolean = false,
    val recommendedDirs: List<LocalDirQuickItem> = emptyList(),
    val entries: List<LocalDirEntry> = emptyList(),
    val truncated: Boolean = false,
    val error: String? = null
)

@Serializable
data class CreateLocalDirResponse(
    val ok: Boolean = false,
    val path: String? = null,
    val name: String? = null,
    val displayPath: String? = null,
    val alreadyExists: Boolean = false,
    val error: String? = null
)

@Serializable
data class CreateProjectResponse(
    val ok: Boolean = false,
    val alreadyExists: Boolean = false,
    val project: ProjectItem? = null,
    val cascadeId: String? = null,
    val error: String? = null
)

@Serializable
data class PermissionStateResponse(
    val ok: Boolean = true,
    val preset: String = "strict", // "strict" | "sandbox" | "auto"
    val label: String = "严格审批".loc,
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
