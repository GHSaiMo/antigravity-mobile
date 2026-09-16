package com.antigravity.mobile.data.model

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
    val role: String, // "user", "assistant", "system"
    val content: String = "",
    val timestamp: String? = null,
    val status: String? = null,
    val stepIndex: Int? = null,
    @SerialName("tool_calls") val toolCalls: List<ToolCallItem>? = null,
    @SerialName("reasoning_content") val reasoningContent: String? = null
)

@Serializable
data class QueuedMessageItem(
    val id: String,
    val text: String,
    val createdAt: String? = null
)

@Serializable
data class RunningTaskItem(
    val id: String,
    val type: String,
    val command: String? = null,
    val status: String? = null
)

@Serializable
data class StreamUpdatePayload(
    val type: String, // "init", "update", "error"
    val cascadeId: String,
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
