package com.antigravity.mobile.ui.viewmodel

import android.net.Uri
import androidx.lifecycle.viewModelScope
import com.antigravity.mobile.data.model.*
import kotlinx.coroutines.launch
import java.util.UUID

internal fun ChatViewModel.requestUndo(message: GatewayMessageItem) {
    val cascadeId = _uiState.value.cascadeId
    if (cascadeId.isBlank()) return

    var targetMsg = message
    val initialStepIndex = targetMsg.stepIndex ?: targetMsg.id.removePrefix("step-").toIntOrNull()

    _uiState.value = _uiState.value.copy(
        activeUndoMessage = targetMsg,
        showConfirmUndoSheet = true,
        isLoadingRevertPreview = true,
        revertPreview = null
    )

    viewModelScope.launch {
        val validStepIndex = initialStepIndex ?: 0
        val res = apiClient.getRevertPreview(cascadeId = cascadeId, stepIndex = validStepIndex)
        res.onSuccess { preview ->
            _uiState.value = _uiState.value.copy(
                revertPreview = preview,
                isLoadingRevertPreview = false
            )
        }.onFailure {
            _uiState.value = _uiState.value.copy(
                isLoadingRevertPreview = false,
                revertPreview = RevertPreviewResponse(
                    cascadeId = cascadeId,
                    stepIndex = validStepIndex,
                    targetStepIndex = (validStepIndex - 1).coerceAtLeast(-1),
                    files = emptyList(),
                    hasCodeChanges = false
                )
            )
        }
    }
}

internal fun ChatViewModel.confirmUndo() {
    val cascadeId = _uiState.value.cascadeId
    val message = _uiState.value.activeUndoMessage ?: return
    val stepIndex = message.stepIndex ?: message.id.removePrefix("step-").toIntOrNull() ?: 0
    val isFirstUserMessage = stepIndex <= 0 || message.id == _uiState.value.messages.firstOrNull { it.isUser }?.id
    if (cascadeId.isBlank()) return

    _uiState.value = _uiState.value.copy(isReverting = true)

    viewModelScope.launch {
        val res = apiClient.executeRevert(cascadeId = cascadeId, stepIndex = stepIndex, conversationOnly = false)
        res.onSuccess {
            // Put undone message text back into input
            _inputText.value = message.effectiveText

            // Restore image attachments if any
            val images = message.effectiveImageDataList
            if (images.isNotEmpty()) {
                val restoredAttachments = images.map { bytes ->
                    AttachmentImage(
                        id = UUID.randomUUID().toString(),
                        uri = Uri.EMPTY,
                        bitmap = null,
                        byteArray = bytes,
                        mimeType = "image/jpeg"
                    )
                }
                _uiState.value = _uiState.value.copy(selectedImages = restoredAttachments)
            }

            _uiState.value = _uiState.value.copy(
                showConfirmUndoSheet = false,
                isReverting = false,
                activeUndoMessage = null,
                revertPreview = null,
                focusInputTrigger = _uiState.value.focusInputTrigger + 1
            )

            if (isFirstUserMessage) {
                val emptyTitle = if (_uiState.value.workspaceName.isNotBlank() && _uiState.value.workspaceName != "Chat") {
                    _uiState.value.workspaceName
                } else {
                    "新对话"
                }
                _uiState.value = _uiState.value.copy(
                    messages = emptyList(),
                    stepCount = 0,
                    totalTools = 0,
                    duration = "0秒",
                    isNewConversation = true,
                    title = emptyTitle
                )
                cacheManager?.updateSessionTitle(cascadeId, emptyTitle)
            } else {
                val filtered = _uiState.value.messages.filter { msg ->
                    val idx = msg.stepIndex ?: msg.id.removePrefix("step-").toIntOrNull()
                    idx == null || idx < stepIndex
                }
                _uiState.value = _uiState.value.copy(messages = filtered)
            }

            refreshMessages()
        }.onFailure { err ->
            _uiState.value = _uiState.value.copy(
                isReverting = false,
                errorMessage = "撤回失败: ${err.message}"
            )
        }
    }
}
