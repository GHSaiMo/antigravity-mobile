package com.antigravity.mobile.ui.viewmodel

import androidx.lifecycle.viewModelScope
import com.antigravity.mobile.data.model.*
import kotlinx.coroutines.launch

internal fun ChatViewModel.filterRunningTasks(tasks: List<RunningTaskItem>?): List<RunningTaskItem> {
    if (tasks.isNullOrEmpty()) {
        stoppedTaskKeys.clear()
        return emptyList()
    }
    if (stoppedTaskKeys.isEmpty()) return tasks
    return tasks.filterNot { task ->
        stoppedTaskKeys.contains("${task.id}:${task.stepIndex}") ||
        (task.id.isNotBlank() && stoppedTaskKeys.contains(task.id))
    }
}

internal fun ChatViewModel.extractStepIndex(id: String): Int? {
    if (id.startsWith("step-")) {
        return id.removePrefix("step-").toIntOrNull()
    }
    return null
}

internal fun ChatViewModel.isStatusRunning(status: String?): Boolean {
    if (status.isNullOrBlank()) return false
    return status.equals("CASCADE_RUN_STATUS_RUNNING", ignoreCase = true) ||
           status.equals("RUNNING", ignoreCase = true)
}

internal fun ChatViewModel.sanitizeMessageOrder(list: List<GatewayMessageItem>): List<GatewayMessageItem> {
    if (list.size < 2) return list

    var dropIndex: Int? = null
    var prevStep = -1

    for (i in list.indices) {
        val msg = list[i]
        val step = msg.stepIndex ?: extractStepIndex(msg.id)
        if (step != null) {
            if (prevStep != -1 && step < prevStep && (prevStep - step) >= 2) {
                dropIndex = i
                break
            }
            prevStep = step
        }
    }

    if (dropIndex != null) {
        val head = list.subList(0, dropIndex)
        val tail = list.subList(dropIndex, list.size)
        return tail + head
    }
    return list
}

internal fun ChatViewModel.mergeIncomingMessages(incoming: List<GatewayMessageItem>): List<GatewayMessageItem> {
    if (incoming.isEmpty()) return _uiState.value.messages
    val currentMessages = _uiState.value.messages
    if (currentMessages.isEmpty()) return sanitizeMessageOrder(incoming)

    val currentOptId = pendingOptimisticMessageId
    var optimisticMessage: GatewayMessageItem? = null
    if (currentOptId != null) {
        optimisticMessage = currentMessages.firstOrNull { it.id == currentOptId }
        val serverHasUserTurn = incoming.any {
            it.isUser && (it.id != currentOptId && (optimisticMessage == null || it.effectiveText == optimisticMessage.effectiveText))
        }
        if (serverHasUserTurn) {
            pendingOptimisticMessageId = null
            optimisticMessage = null
        }
    }

    var base = currentMessages.filter { msg ->
        if (currentOptId != null && msg.id == currentOptId) return@filter false
        if (msg.id.startsWith("opt_")) return@filter false
        true
    }

    if (base.isEmpty()) {
        base = incoming
    } else {
        var firstBaseMatchIdx: Int? = null
        var incomingMatchIdxForFirstBaseMatch: Int? = null

        for (baseIdx in base.indices) {
            val baseMsg = base[baseIdx]
            val incIdx = incoming.indexOfFirst { it.id == baseMsg.id }
            if (incIdx >= 0) {
                firstBaseMatchIdx = baseIdx
                incomingMatchIdxForFirstBaseMatch = incIdx
                break
            }
        }

        if (firstBaseMatchIdx != null && incomingMatchIdxForFirstBaseMatch != null) {
            val bIdx = firstBaseMatchIdx
            val iIdx = incomingMatchIdxForFirstBaseMatch
            val prefix = base.subList(0, bIdx)
            val incomingPrefix = incoming.subList(0, iIdx)
            val incomingTail = incoming.subList(iIdx, incoming.size)
            base = prefix + incomingPrefix + incomingTail
        } else {
            val baseStep = base.mapNotNull { it.stepIndex ?: extractStepIndex(it.id) }.firstOrNull()
            val incomingStep = incoming.mapNotNull { it.stepIndex ?: extractStepIndex(it.id) }.firstOrNull()

            if (baseStep != null && incomingStep != null && incomingStep < baseStep) {
                base = incoming + base
            } else {
                base = base + incoming
            }
        }
    }

    if (optimisticMessage != null) {
        base = base + optimisticMessage
    }

    return sanitizeMessageOrder(base)
}

internal fun ChatViewModel.refreshMessages() {
    val cascadeId = _uiState.value.cascadeId
    if (cascadeId.isBlank()) return
    viewModelScope.launch {
        apiClient.fetchMessages(cascadeId, limit = 15).onSuccess { payload ->
            if (_uiState.value.cascadeId == cascadeId) {
                val incoming = payload.messages ?: emptyList()
                if (incoming.isEmpty()) {
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
                    val sanitized = sanitizeMessageOrder(incoming)
                    _uiState.value = _uiState.value.copy(
                        messages = sanitized,
                        stepCount = payload.totalSteps,
                        totalTools = payload.totalTools,
                        duration = payload.duration ?: _uiState.value.duration
                    )
                }
            }
        }
    }
}
