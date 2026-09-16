package com.antigravity.mobile.data.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

@Serializable
data class ConversationItem(
    @SerialName("cascade_id") val cascadeId: String,
    val title: String? = null,
    val summary: String? = null,
    @SerialName("step_count") val stepCount: Int = 0,
    @SerialName("created_at") val createdAt: String? = null,
    @SerialName("updated_at") val updatedAt: String? = null,
    @SerialName("last_message_snippet") val lastMessageSnippet: String? = null,
    @SerialName("workspace_folder") val workspaceFolder: String? = null,
    val status: String = "IDLE", // RUNNING, COMPLETED, ERROR, ACTION, IDLE
    @SerialName("has_error") val hasError: Boolean = false,
    @SerialName("error_message") val errorMessage: String? = null,
    @SerialName("can_proceed") val canProceed: Boolean = false,
    @SerialName("pending_interaction") val pendingInteraction: PendingInteraction? = null,
    @SerialName("has_unread") val hasUnread: Boolean = false
) {
    val displayTitle: String
        get() = title?.takeIf { it.isNotBlank() } ?: "未命名会话"

    val displayStatus: SessionStatus
        get() = when {
            hasError -> SessionStatus.ERROR
            pendingInteraction != null || canProceed -> SessionStatus.ACTION
            status.equals("RUNNING", ignoreCase = true) -> SessionStatus.RUNNING
            else -> SessionStatus.IDLE
        }
}

enum class SessionStatus {
    RUNNING,
    ACTION,
    ERROR,
    IDLE
}

@Serializable
data class ConversationListResponse(
    val conversations: List<ConversationItem> = emptyList()
)
