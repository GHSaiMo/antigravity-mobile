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
data class GatewayMessageItem(
    val id: String = "",
    val type: String = "user", // "user", "agent", "tools", "error"
    val role: String = "", // "user", "assistant", "system"
    val text: String = "",
    val content: String = "",
    val toolCount: Int? = null,
    val toolNames: List<String>? = null,
    val media: List<String>? = null,
    val imageUrls: List<String>? = null,
    val timestamp: String? = null,
    val status: String? = null,
    val stepIndex: Int? = null,
    @SerialName("tool_calls") val toolCalls: List<ToolCallItem>? = null,
    @SerialName("reasoning_content") val reasoningContent: String? = null,
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

    val isAgent: Boolean
        get() = !isUser && !isTools && !isError

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
    val imageUrls: List<String>? = null
) {
    val hasAttachments: Boolean
        get() = !media.isNullOrEmpty() || !imageUrls.isNullOrEmpty()
}

@Serializable
data class RunningTaskItem(
    val id: String = "",
    val type: String = "",
    val command: String? = null,
    val status: String? = null
)

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
    val isFullSnapshot: Boolean = false,
    val cascadeConfigRaw: String? = null,
    val canProceed: Boolean = false,
    val proceedArtifactUri: String? = null,
    val pendingInteraction: PendingInteraction? = null,
    val activeModel: String? = null,
    val modelDisplayName: String? = null
)
