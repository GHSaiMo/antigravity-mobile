package com.antigravity.mobile.ui.viewmodel

import androidx.lifecycle.viewModelScope
import com.antigravity.mobile.data.model.ContextMentionItem
import com.antigravity.mobile.data.service.*
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

internal fun ChatViewModel.refreshPermissionSettings() {
    viewModelScope.launch {
        apiClient.fetchPermissionSettings().onSuccess { st ->
            _uiState.value = _uiState.value.copy(permissionState = st)
        }
    }
}

internal fun ChatViewModel.setPermissionPreset(preset: String, onDone: ((String) -> Unit)? = null) {
    viewModelScope.launch {
        apiClient.updatePermissionPreset(preset).onSuccess { st ->
            _uiState.value = _uiState.value.copy(permissionState = st)
            onDone?.invoke("权限档位已切换为「${st.label}」")
        }.onFailure { err ->
            _uiState.value = _uiState.value.copy(attachmentNotice = "切换权限失败: ${err.message}")
        }
    }
}

internal fun ChatViewModel.openMentionPicker(query: String = "") {
    _uiState.value = _uiState.value.copy(showMentionPicker = true, mentionQuery = query)
    searchContextMentions(query)
}

internal fun ChatViewModel.closeMentionPicker() {
    mentionSearchJob?.cancel()
    mentionSearchJob = null
    _uiState.value = _uiState.value.copy(showMentionPicker = false, isLoadingMentions = false)
}

internal fun ChatViewModel.searchContextMentions(query: String) {
    val cid = _uiState.value.cascadeId
    val wsFolder = _uiState.value.workspaceFolder
    val wsUri = wsFolder?.takeIf { it.isNotBlank() }?.let { if (it.startsWith("file://")) it else "file://$it" }
    _uiState.value = _uiState.value.copy(mentionQuery = query, isLoadingMentions = true)
    mentionSearchJob?.cancel()
    mentionSearchJob = viewModelScope.launch {
        delay(150L)
        apiClient.fetchContextMentions(query, cid, wsUri)
            .onSuccess { items ->
                _uiState.value = _uiState.value.copy(mentionCandidates = items, isLoadingMentions = false)
            }
            .onFailure {
                _uiState.value = _uiState.value.copy(mentionCandidates = emptyList(), isLoadingMentions = false)
            }
    }
}

internal fun ChatViewModel.addContextMention(item: ContextMentionItem) {
    val cur = _uiState.value.selectedContextItems
    if (cur.none { it.id == item.id }) {
        _uiState.value = _uiState.value.copy(
            selectedContextItems = cur + item,
            showMentionPicker = false
        )
    } else {
        _uiState.value = _uiState.value.copy(showMentionPicker = false)
    }
}

internal fun ChatViewModel.removeContextMention(id: String) {
    _uiState.value = _uiState.value.copy(
        selectedContextItems = _uiState.value.selectedContextItems.filterNot { it.id == id }
    )
}

internal fun ChatViewModel.forkConversationAt(
    stepIndex: Int,
    createWorktree: Boolean,
    onForked: (String) -> Unit
) {
    val cid = _uiState.value.cascadeId
    if (cid.isBlank() || cid.startsWith("local_draft_")) return
    viewModelScope.launch {
        apiClient.forkConversation(cid, stepIndex, createWorktree).onSuccess { resp ->
            val newId = resp.cascadeId
            if (!newId.isNullOrBlank()) {
                onForked(newId)
            } else {
                _uiState.value = _uiState.value.copy(attachmentNotice = resp.error ?: "分叉未返回新会话")
            }
        }.onFailure { err ->
            _uiState.value = _uiState.value.copy(attachmentNotice = "分叉失败: ${err.message}")
        }
    }
}

internal fun ChatViewModel.sendTaskToBackground(stepIndex: Int, onNotice: (String) -> Unit) {
    val cid = _uiState.value.cascadeId
    if (cid.isBlank()) return
    viewModelScope.launch {
        apiClient.sendStepsToBackground(cid, listOf(stepIndex)).onSuccess { ok ->
            onNotice(if (ok) "已转入后台执行" else "当前上游版本暂不支持步骤级转后台")
        }.onFailure { err ->
            onNotice("转后台失败: ${err.message}")
        }
    }
}
