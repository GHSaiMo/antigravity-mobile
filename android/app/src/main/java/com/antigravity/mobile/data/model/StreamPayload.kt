package com.antigravity.mobile.data.model

import android.util.Base64
import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

@Serializable
data class ToolCallItem(
    val id: String = "",
    val type: String = "",
    val name: String = "",
    val input: String? = null,
    val output: String? = null,
    val status: String? = null
)

@Serializable
data class ArtifactItem(
    val uri: String = "",
    val title: String = "",
    val summary: String? = null,
    val requestFeedback: Boolean? = null,
    val userFacing: Boolean? = null
)

@Serializable
data class GatewayMessageItem(
    val id: String = "",
    val type: String = "user", // "user", "agent", "tools", "error", "subagent"
    val role: String = "", // "user", "assistant", "system"
    val text: String = "",
    val content: String = "",
    val toolCount: Int? = null,
    val toolNames: List<String>? = null,
    val media: List<String>? = null,
    val imageUrls: List<String>? = null,
    val artifacts: List<ArtifactItem>? = null,
    val timestamp: String? = null,
    /** 生成这条回复的模型 id 与展示名（仅 Agent 消息；未知时为空）。 */
    val model: String? = null,
    val modelName: String? = null,
    val status: String? = null,
    val stepIndex: Int? = null,
    val attemptCount: Int? = null,
    val maxAttempts: Int? = null,
    @SerialName("tool_calls") val toolCalls: List<ToolCallItem>? = null,
    @SerialName("reasoning_content") val reasoningContent: String? = null,
    /** type == "subagent" 时的子代理卡片数据（状态由网关实时补全）。 */
    val subagent: SubagentItem? = null,
    @kotlinx.serialization.Transient val imageDataList: List<ByteArray> = emptyList()
) {
    val effectiveRole: String
        get() = when {
            type.isNotBlank() -> if (type == "agent") "assistant" else type
            role.isNotBlank() -> role
            else -> "user"
        }

    val effectiveText: String
        get() = when {
            text.isNotBlank() -> text
            content.isNotBlank() -> content
            else -> ""
        }

    val isUser: Boolean
        get() = effectiveRole.equals("user", ignoreCase = true)

    val isTools: Boolean
        get() = type.equals("tools", ignoreCase = true) || !toolNames.isNullOrEmpty() || (toolCount != null && toolCount > 0)

    val isError: Boolean
        get() = status.equals("error", ignoreCase = true) || type.equals("error", ignoreCase = true)

    val isSubagent: Boolean
        get() = type.equals("subagent", ignoreCase = true) && subagent != null

    val isAgent: Boolean
        get() = !isUser && !isTools && !isError && !isSubagent

    val effectiveImageDataList: List<ByteArray>
        get() {
            if (imageDataList.isNotEmpty()) return imageDataList
            if (!media.isNullOrEmpty()) {
                return media.mapNotNull { b64 ->
                    try {
                        Base64.decode(b64, Base64.DEFAULT)
                    } catch (_: Exception) {
                        null
                    }
                }
            }
            return emptyList()
        }
}

@Serializable
data class QueuedMessageItem(
    val id: String = "",
    val text: String = "",
    val createdAt: String? = null,
    val media: List<String>? = null,
    val imageUrls: List<String>? = null,
    /** 发送时附带的客户端消息 id（网关写进 tags，读队列时还原）；不是经网关发的消息为空。 */
    val clientMessageId: String? = null
) {
    val hasAttachments: Boolean
        get() = !media.isNullOrEmpty() || !imageUrls.isNullOrEmpty()
}

@Serializable
data class RunningTaskItem(
    val id: String = "",
    val stepIndex: Int = 0,
    val toolName: String? = null,
    val commandLine: String = "",
    val toolSummary: String? = null,
    val toolAction: String? = null,
    val logUri: String? = null,
    val startedAt: String? = null,
    val type: String = "",
    val command: String? = null,
    val status: String? = null
) {
    val displayTitle: String
        get() = toolSummary?.takeIf { it.isNotBlank() }
            ?: toolAction?.takeIf { it.isNotBlank() }
            ?: toolName?.takeIf { it.isNotBlank() }?.let { com.antigravity.mobile.ui.util.ToolLocalization.localizedName(it) }
            ?: type.takeIf { it.isNotBlank() }?.let { com.antigravity.mobile.ui.util.ToolLocalization.localizedName(it) }
            ?: "运行终端命令"

    val displayCommand: String
        get() = commandLine.takeIf { it.isNotBlank() }
            ?: command?.takeIf { it.isNotBlank() }
            ?: ""
}

/** 父会话通过 invoke_subagent 派发的一个子代理（网关 stream 的 `subagents`）。 */
@Serializable
data class SubagentItem(
    val conversationId: String = "",
    val typeName: String? = null,
    val role: String? = null,
    val prompt: String? = null,
    val modelTier: String? = null,
    val stepIndex: Int? = null,
    /** "running" | "done" | "gone"；网关尚未补全时为 null。 */
    val status: String? = null,
    val stepCount: Int? = null,
    val title: String? = null
) {
    val isRunning: Boolean get() = status == "running"
    val isGone: Boolean get() = status == "gone"

    /** 列表里展示的名字：优先角色，其次类型，最后用会话 ID 前 8 位兜底。 */
    val displayName: String
        get() = role?.trim()?.takeIf { it.isNotEmpty() }
            ?: typeName?.trim()?.takeIf { it.isNotEmpty() }
            ?: conversationId.take(8)

    val statusText: String
        get() = when (status) {
            "running" -> "运行中"
            "gone" -> "已清理"
            "done" -> "已结束"
            else -> ""
        }
}

@Serializable
data class StreamUpdatePayload(
    val type: String = "update", // "init", "update", "error"
    val cascadeId: String = "",
    val title: String? = null,
    val status: String = "",
    val hasError: Boolean = false,
    val errorMessage: String? = null,
    val duration: String? = null,
    val totalSteps: Int = 0,
    val totalTools: Int = 0,
    val totalMessages: Int = 0,
    val hasMore: Boolean = false,
    val nextOffset: Int = 0,
    val workspaceUri: String? = null,
    val messages: List<GatewayMessageItem>? = null,
    val queuedMessages: List<QueuedMessageItem>? = null,
    val runningTasks: List<RunningTaskItem>? = null,
    val subagents: List<SubagentItem>? = null,
    /** 非空说明该会话本身是子代理。 */
    val parentConversationId: String? = null,
    val subagentRole: String? = null,
    val isFullSnapshot: Boolean = false,
    val cascadeConfigRaw: String? = null,
    val canProceed: Boolean = false,
    val proceedArtifactUri: String? = null,
    val pendingInteraction: PendingInteraction? = null,
    val activeModel: String? = null,
    /** 完整展示名，如 "Gemini 3.8 Flash (High)"。 */
    val activeModelName: String? = null,
    val modelDisplayName: String? = null,
    /** 会话发起时间（ISO-8601）。 */
    val startedAt: String? = null,
    /** delta=1 增量帧：messages 仅含新增/变化项，messageIds 为当前窗口完整有序 ID（见 StreamDeltaReassembler）。 */
    val delta: Boolean = false,
    val messageIds: List<String>? = null
)

@Serializable
data class RevertDiffLine(
    val text: String = "",
    val type: String = "UNCHANGED" // "INSERT", "DELETE", "UNCHANGED"
)

@Serializable
data class RevertPreviewFile(
    val fileUri: String = "",
    val fileName: String = "",
    val actionType: String = "MODIFY", // "MODIFY", "CREATE", "DELETE"
    val additions: Int = 0,
    val deletions: Int = 0,
    val diffLines: List<RevertDiffLine> = emptyList()
)

@Serializable
data class RevertPreviewResponse(
    val cascadeId: String = "",
    val stepIndex: Int = 0,
    val targetStepIndex: Int = 0,
    val files: List<RevertPreviewFile> = emptyList(),
    val hasCodeChanges: Boolean = false
)

@Serializable
data class RevertPreviewRequest(
    val cascadeId: String = "",
    val stepIndex: Int = 0
)

@Serializable
data class RevertExecuteRequest(
    val cascadeId: String = "",
    val stepIndex: Int = 0,
    val conversationOnly: Boolean = false
)

