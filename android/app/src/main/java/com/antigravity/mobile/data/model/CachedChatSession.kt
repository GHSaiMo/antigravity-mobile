package com.antigravity.mobile.data.model

import kotlinx.serialization.Serializable

@Serializable
data class CachedChatSession(
    val cascadeId: String,
    val status: String = "",
    val duration: String = "",
    val stepCount: Int = 0,
    val totalTools: Int = 0,
    val hasMore: Boolean = false,
    val nextOffset: Int = 0,
    val messages: List<GatewayMessageItem> = emptyList(),
    val title: String? = null,
    val workspaceName: String? = null,
    val cascadeConfigRaw: String? = null,
    val canProceed: Boolean? = null,
    val proceedArtifactUri: String? = null,
    val pendingInteraction: PendingInteraction? = null,
    val queuedMessages: List<QueuedMessageItem>? = null,
    val runningTasks: List<RunningTaskItem>? = null,
    val activeModel: String? = null,
    val savedAtEpochMs: Long = System.currentTimeMillis()
)
