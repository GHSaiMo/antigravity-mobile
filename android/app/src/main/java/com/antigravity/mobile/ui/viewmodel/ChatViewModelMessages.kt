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

    val ordered = if (dropIndex != null) {
        val head = list.subList(0, dropIndex)
        val tail = list.subList(dropIndex, list.size)
        tail + head
    } else {
        list
    }
    return collapseAttemptErrors(ordered)
}

data class ParsedAttemptError(
    val isAttempt: Boolean,
    val prefix: String,
    val attempt: Int,
    val maxAttempts: Int,
    val baseError: String
)

private val attemptPattern = Regex("""(?i)^(.*?)\s*\(attempt\s+(\d+)(?:\s*(?:of|/)\s*(\d+))?(?:\s*[·,]\s*[^)]*)?\)\s*[:：\-]?\s*(.*)$""")

internal fun parseAttemptError(text: String): ParsedAttemptError {
    val trimmed = text.trim()
    val match = attemptPattern.find(trimmed)
    if (match != null) {
        var prefix = match.groupValues[1].trim()
        if (prefix.isBlank()) prefix = "API error"
        val attempt = match.groupValues[2].toIntOrNull() ?: 1
        var maxAttempts = match.groupValues[3].toIntOrNull() ?: 0
        if (maxAttempts == 0) {
            maxAttempts = if (attempt <= 9) 9 else attempt
        }
        val baseError = match.groupValues[4].trim()
        return ParsedAttemptError(
            isAttempt = true,
            prefix = prefix,
            attempt = attempt,
            maxAttempts = maxAttempts,
            baseError = baseError
        )
    }
    return ParsedAttemptError(
        isAttempt = false,
        prefix = "API error",
        attempt = 1,
        maxAttempts = 9,
        baseError = trimmed
    )
}

internal fun normalizeErrKey(text: String): String {
    var s = text.trim().lowercase()
    while (s.endsWith(".") || s.endsWith(":") || s.endsWith(";") || s.endsWith(",")) {
        s = s.substring(0, s.length - 1)
    }
    return s.trim()
}

internal fun collapseAttemptErrors(list: List<GatewayMessageItem>): List<GatewayMessageItem> {
    if (list.size < 2) return list
    val result = ArrayList<GatewayMessageItem>(list.size)

    for (msg in list) {
        val last = result.lastOrNull()
        if (msg.isError && last != null && last.isError) {
            val msgText = msg.effectiveText.ifBlank { "执行遇到错误" }
            val lastText = last.effectiveText.ifBlank { "执行遇到错误" }
            val parsed = parseAttemptError(msgText)
            val prevParsed = parseAttemptError(lastText)

            if (parsed.isAttempt && prevParsed.isAttempt && normalizeErrKey(prevParsed.baseError) == normalizeErrKey(parsed.baseError)) {
                val newAttempt = maxOf(parsed.attempt, (last.attemptCount ?: prevParsed.attempt) + 1)
                val maxAttempts = if (parsed.maxAttempts > 0) parsed.maxAttempts else (last.maxAttempts ?: 9)
                val formatted = "${parsed.prefix} (attempt $newAttempt/$maxAttempts · 已重试 $newAttempt 次): ${parsed.baseError}"
                result[result.size - 1] = last.copy(
                    text = formatted,
                    content = formatted,
                    stepIndex = msg.stepIndex ?: last.stepIndex,
                    attemptCount = newAttempt,
                    maxAttempts = maxAttempts
                )
                continue
            } else if (parsed.isAttempt && normalizeErrKey(lastText) == normalizeErrKey(parsed.baseError)) {
                val newAttempt = maxOf(parsed.attempt, (last.attemptCount ?: 1) + 1)
                val maxAttempts = if (parsed.maxAttempts > 0) parsed.maxAttempts else 9
                val formatted = "${parsed.prefix} (attempt $newAttempt/$maxAttempts · 已重试 $newAttempt 次): ${parsed.baseError}"
                result[result.size - 1] = last.copy(
                    text = formatted,
                    content = formatted,
                    stepIndex = msg.stepIndex ?: last.stepIndex,
                    attemptCount = newAttempt,
                    maxAttempts = maxAttempts
                )
                continue
            } else if (!parsed.isAttempt && !prevParsed.isAttempt && normalizeErrKey(lastText) == normalizeErrKey(msgText)) {
                val count = (last.attemptCount ?: 1) + 1
                val formatted = "$msgText (已重试 $count 次)"
                result[result.size - 1] = last.copy(
                    text = formatted,
                    content = formatted,
                    stepIndex = msg.stepIndex ?: last.stepIndex,
                    attemptCount = count,
                    maxAttempts = null
                )
                continue
            }
        }
        result.add(msg)
    }
    return result
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
