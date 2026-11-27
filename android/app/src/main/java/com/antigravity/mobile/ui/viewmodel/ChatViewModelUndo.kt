package com.antigravity.mobile.ui.viewmodel

import com.antigravity.mobile.ui.util.loc
import android.net.Uri
import androidx.lifecycle.viewModelScope
import com.antigravity.mobile.data.model.*
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import java.util.UUID

private fun isOptimisticMessageId(id: String): Boolean {
    return id.startsWith("opt_") || id.startsWith("optimistic-")
}

private data class AndroidUndoPriorContext(
    val hasPriorServerMessages: Boolean,
    val minAllowedStep: Int
)

private fun ChatViewModel.captureUndoPriorContext(message: GatewayMessageItem): AndroidUndoPriorContext {
    val all = _uiState.value.messages
    val idx = all.indexOfFirst { it.id == message.id }
    val preceding = if (idx >= 0) all.subList(0, idx) else all.filter { it.id != message.id }
    val priorServer = preceding.filterNot { isOptimisticMessageId(it.id) }
    val maxPriorStep = priorServer.mapNotNull { it.stepIndex ?: extractStepIndex(it.id) }.maxOrNull()
    return AndroidUndoPriorContext(
        hasPriorServerMessages = priorServer.isNotEmpty(),
        minAllowedStep = (maxPriorStep ?: -1) + 1
    )
}

private suspend fun ChatViewModel.waitForReadyCascadeId(maxWaitMs: Long = 6000L): String? {
    var waited = 0L
    while (waited < maxWaitMs) {
        val cid = _uiState.value.cascadeId
        if (cid.isNotBlank() && !cid.startsWith("local_draft_") && !cid.startsWith("draft_")) {
            return cid
        }
        if (!_uiState.value.isRunning && !_uiState.value.isAwaitingResponse) {
            break
        }
        delay(100L)
        waited += 100L
    }
    val finalCid = _uiState.value.cascadeId
    return if (finalCid.isNotBlank() && !finalCid.startsWith("local_draft_") && !finalCid.startsWith("draft_")) finalCid else null
}

private suspend fun ChatViewModel.resolveUndoStepIndex(
    message: GatewayMessageItem,
    prior: AndroidUndoPriorContext,
    cascadeId: String,
    allowReload: Boolean
): Pair<Int?, GatewayMessageItem?> {
    if (!isOptimisticMessageId(message.id)) {
        val direct = message.stepIndex ?: extractStepIndex(message.id)
        if (direct != null) return direct to message
    }

    val targetText = message.effectiveText.trim()
    val targetClientId = message.clientMessageId?.trim().orEmpty()

    fun findIn(list: List<GatewayMessageItem>): Pair<Int, GatewayMessageItem>? {
        val serverUsers = list.filter { !isOptimisticMessageId(it.id) && it.isUser }
        if (targetClientId.isNotEmpty() && !isOptimisticMessageId(targetClientId)) {
            val byCid = serverUsers.lastOrNull { it.clientMessageId == targetClientId }
            val step = byCid?.let { it.stepIndex ?: extractStepIndex(it.id) }
            if (byCid != null && step != null) return step to byCid
        }
        val byTail = serverUsers.lastOrNull { cand ->
            val step = cand.stepIndex ?: extractStepIndex(cand.id)
            step != null && step >= prior.minAllowedStep && cand.effectiveText.trim() == targetText
        }
        val tailStep = byTail?.let { it.stepIndex ?: extractStepIndex(it.id) }
        if (byTail != null && tailStep != null) return tailStep to byTail
        return null
    }

    findIn(_uiState.value.messages)?.let { return it.first to it.second }

    if (allowReload && cascadeId.isNotBlank()) {
        apiClient.fetchMessages(cascadeId, limit = 15).onSuccess { payload ->
            val incoming = sanitizeMessageOrder(payload.messages ?: emptyList())
            if (_uiState.value.cascadeId == cascadeId && incoming.isNotEmpty()) {
                _uiState.value = _uiState.value.copy(
                    messages = incoming,
                    stepCount = payload.totalSteps,
                    totalTools = payload.totalTools,
                    duration = payload.duration ?: _uiState.value.duration
                )
            }
            findIn(incoming)?.let { return it.first to it.second }
        }
    }

    if (!prior.hasPriorServerMessages) {
        return 0 to null
    }
    return null to null
}

internal fun ChatViewModel.requestUndo(message: GatewayMessageItem) {
    val rawCid = _uiState.value.cascadeId
    val isPendingDraftSend = (rawCid.startsWith("local_draft_") || rawCid.startsWith("draft_")) && _uiState.value.isRunning
    if (rawCid.isBlank() || ((rawCid.startsWith("local_draft_") || rawCid.startsWith("draft_")) && !isPendingDraftSend)) return

    undoPreviewJob?.cancel()
    val prior = captureUndoPriorContext(message)

    _uiState.value = _uiState.value.copy(
        activeUndoMessage = message,
        showConfirmUndoSheet = true,
        isLoadingRevertPreview = true,
        revertPreview = null,
        undoErrorMessage = null
    )

    undoPreviewJob = viewModelScope.launch {
        val activeCid = waitForReadyCascadeId()
        if (!isActive) return@launch
        if (activeCid == null) {
            _uiState.value = _uiState.value.copy(
                isLoadingRevertPreview = false,
                revertPreview = RevertPreviewResponse(
                    cascadeId = rawCid,
                    stepIndex = 0,
                    targetStepIndex = -1,
                    files = emptyList(),
                    hasCodeChanges = false
                )
            )
            return@launch
        }

        val (resolvedStep, matchedMsg) = resolveUndoStepIndex(
            message = message,
            prior = prior,
            cascadeId = activeCid,
            allowReload = true
        )
        if (!isActive) return@launch

        if (resolvedStep == null) {
            _uiState.value = _uiState.value.copy(
                isLoadingRevertPreview = false,
                revertPreview = RevertPreviewResponse(
                    cascadeId = activeCid,
                    stepIndex = prior.minAllowedStep.coerceAtLeast(0),
                    targetStepIndex = (prior.minAllowedStep - 1).coerceAtLeast(-1),
                    files = emptyList(),
                    hasCodeChanges = false
                )
            )
            return@launch
        }

        val updatedTarget = matchedMsg ?: message.copy(stepIndex = resolvedStep)
        _uiState.value = _uiState.value.copy(activeUndoMessage = updatedTarget)

        val res = apiClient.getRevertPreview(cascadeId = activeCid, stepIndex = resolvedStep)
        if (!isActive) return@launch
        res.onSuccess { preview ->
            _uiState.value = _uiState.value.copy(
                revertPreview = preview,
                isLoadingRevertPreview = false
            )
        }.onFailure {
            _uiState.value = _uiState.value.copy(
                isLoadingRevertPreview = false,
                revertPreview = RevertPreviewResponse(
                    cascadeId = activeCid,
                    stepIndex = resolvedStep,
                    targetStepIndex = (resolvedStep - 1).coerceAtLeast(-1),
                    files = emptyList(),
                    hasCodeChanges = false
                )
            )
        }
    }
}

internal fun ChatViewModel.confirmUndo() {
    val message = _uiState.value.activeUndoMessage ?: return
    if (_uiState.value.isReverting) return

    undoPreviewJob?.cancel()
    undoPreviewJob = null
    val prior = captureUndoPriorContext(message)

    _uiState.value = _uiState.value.copy(
        isReverting = true,
        isLoadingRevertPreview = false,
        undoErrorMessage = null
    )

    viewModelScope.launch {
        val activeCid = waitForReadyCascadeId()
        if (activeCid == null) {
            if (!_uiState.value.isRunning && !_uiState.value.isAwaitingResponse) {
                _inputText.value = message.effectiveText
                pendingOptimisticMessageId = null
                _uiState.value = _uiState.value.copy(
                    messages = _uiState.value.messages.filterNot { it.id == message.id || isOptimisticMessageId(it.id) },
                    showConfirmUndoSheet = false,
                    isReverting = false,
                    activeUndoMessage = null,
                    revertPreview = null,
                    undoErrorMessage = null,
                    focusInputTrigger = _uiState.value.focusInputTrigger + 1
                )
            } else {
                _uiState.value = _uiState.value.copy(
                    isReverting = false,
                    undoErrorMessage = "会话仍在创建中，请稍等片刻再试".loc
                )
            }
            return@launch
        }

        var (stepIndex, _) = resolveUndoStepIndex(
            message = message,
            prior = prior,
            cascadeId = activeCid,
            allowReload = true
        )
        if (stepIndex == null && (_uiState.value.isRunning || isOptimisticMessageId(message.id))) {
            delay(400L)
            stepIndex = resolveUndoStepIndex(
                message = message,
                prior = prior,
                cascadeId = activeCid,
                allowReload = true
            ).first
        }

        if (stepIndex == null) {
            _uiState.value = _uiState.value.copy(
                isReverting = false,
                undoErrorMessage = "消息刚发出尚未同步步骤编号，请稍等 1 秒后重试".loc
            )
            return@launch
        }

        val isFirstUserMessage = !prior.hasPriorServerMessages && stepIndex <= 0

        val res = apiClient.executeRevert(cascadeId = activeCid, stepIndex = stepIndex, conversationOnly = false)
        res.onSuccess {
            lastLocalSendAtMs = 0L
            preSendMaxStepIndex = -1
            knownServerMessageIds = emptySet()
            _inputText.value = message.effectiveText
            pendingOptimisticMessageId = null

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
                isRunning = false,
                isAwaitingResponse = false,
                activeUndoMessage = null,
                revertPreview = null,
                undoErrorMessage = null,
                focusInputTrigger = _uiState.value.focusInputTrigger + 1
            )

            if (isFirstUserMessage) {
                val emptyTitle = if (_uiState.value.workspaceName.isNotBlank() && _uiState.value.workspaceName != "Chat") {
                    _uiState.value.workspaceName
                } else {
                    "新对话".loc
                }
                _uiState.value = _uiState.value.copy(
                    messages = emptyList(),
                    stepCount = 0,
                    totalTools = 0,
                    duration = "0秒".loc,
                    isNewConversation = true,
                    title = emptyTitle
                )
                cacheManager?.updateSessionTitle(activeCid, emptyTitle)
            } else {
                val filtered = _uiState.value.messages.filterNot { msg ->
                    if (msg.id == message.id || isOptimisticMessageId(msg.id)) {
                        true
                    } else {
                        val idx = msg.stepIndex ?: extractStepIndex(msg.id)
                        idx != null && idx >= stepIndex
                    }
                }
                _uiState.value = _uiState.value.copy(messages = filtered)
            }

            refreshMessages()
        }.onFailure { err ->
            _uiState.value = _uiState.value.copy(
                isReverting = false,
                undoErrorMessage = "撤回失败: %s".loc(err.message.orEmpty())
            )
        }
    }
}
