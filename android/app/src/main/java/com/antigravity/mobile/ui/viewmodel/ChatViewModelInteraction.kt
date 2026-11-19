package com.antigravity.mobile.ui.viewmodel

import androidx.lifecycle.viewModelScope
import com.antigravity.mobile.data.model.*
import kotlinx.coroutines.launch
import kotlinx.coroutines.delay

internal fun ChatViewModel.submitInteraction(
    optionId: String,
    writeInText: String? = null,
    target: String? = null,
    questionResponses: List<QuestionResponse>? = null
) {
    val interaction = _uiState.value.pendingInteraction ?: return
    val cascadeId = _uiState.value.cascadeId
    val selectedOpt = interaction.options.find { it.id == optionId }
    val scope = selectedOpt?.scope ?: 1
    val isPermissionType = interaction.type == "permission" || interaction.type == "file_permission"
    val isDeny = selectedOpt?.isDeny == true || optionId == "5" || (isPermissionType && optionId == "__write_in__")
    val allow = !isDeny

    viewModelScope.launch {
        apiClient.submitInteraction(
            cascadeId = cascadeId,
            trajectoryId = interaction.trajectoryId,
            stepIndex = interaction.stepIndex,
            type = interaction.type,
            optionId = optionId,
            scope = scope,
            allow = allow,
            writeInResponse = writeInText ?: "",
            skipped = false,
            target = target ?: interaction.target,
            questionResponses = questionResponses
        )
        _uiState.value = _uiState.value.copy(pendingInteraction = null)
        saveSessionToCache()
        refreshMessages()
    }
}

internal fun ChatViewModel.skipInteraction(questionResponses: List<QuestionResponse>? = null) {
    val interaction = _uiState.value.pendingInteraction ?: return
    val cascadeId = _uiState.value.cascadeId
    viewModelScope.launch {
        apiClient.submitInteraction(
            cascadeId = cascadeId,
            trajectoryId = interaction.trajectoryId,
            stepIndex = interaction.stepIndex,
            type = interaction.type,
            optionId = "",
            scope = 1,
            allow = false,
            writeInResponse = "",
            skipped = true,
            target = interaction.target,
            questionResponses = questionResponses
        )
        _uiState.value = _uiState.value.copy(pendingInteraction = null)
        saveSessionToCache()
        refreshMessages()
    }
}

internal fun ChatViewModel.proceedArtifact() {
    val cascadeId = _uiState.value.cascadeId
    if (cascadeId.isBlank()) return
    val uri = _uiState.value.proceedArtifactUri ?: "implementation_plan.md"
    val model = _uiState.value.activeModel

    awaitingStartedAtMs = System.currentTimeMillis()
    hasObservedServerRunningForAwaiting = false
    _uiState.value = _uiState.value.copy(
        canProceed = false,
        proceedArtifactUri = null,
        isRunning = true,
        isAwaitingResponse = true,
        isLatestMessageError = false
    )
    liveActivityManager?.startOrUpdateActivity(
        title = _uiState.value.title,
        cascadeId = cascadeId,
        status = "RUNNING",
        stepCount = maxOf(1, _uiState.value.stepCount),
        latestAction = "已确认方案，继续执行中...",
        isOptimisticStart = true,
        baselineStepCount = _uiState.value.stepCount
    )
    saveSessionToCache()
    notifyConversationUpdated()
    ensureWebSocketObserving()
    ensureRunningFallbackPolling()

    viewModelScope.launch {
        val result = apiClient.proceedArtifact(cascadeId, uri, model)
        if (result.isFailure) {
            awaitingStartedAtMs = 0L
            hasObservedServerRunningForAwaiting = false
            _uiState.value = _uiState.value.copy(
                canProceed = true,
                proceedArtifactUri = uri,
                isRunning = false,
                isAwaitingResponse = false,
                isLatestMessageError = true
            )
            liveActivityManager?.endActivity(cascadeId = cascadeId, finalStatus = "FAILED", title = _uiState.value.title)
            saveSessionToCache()
            notifyConversationUpdated()
        } else {
            delay(250)
            refresh()
        }
    }
}
