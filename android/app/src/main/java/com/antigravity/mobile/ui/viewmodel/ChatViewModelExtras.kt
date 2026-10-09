package com.antigravity.mobile.ui.viewmodel

import androidx.lifecycle.viewModelScope
import kotlinx.coroutines.launch

private fun String.isLocalDraft(): Boolean = isBlank() || startsWith("local_draft_")

// endregion

// region 导出 Markdown

/** 导出当前会话为 Markdown 文本；成功后在主线程回调，失败时写入 errorMessage。 */
fun ChatViewModel.exportMarkdown(onReady: (title: String, markdown: String) -> Unit) {
    val state = _uiState.value
    if (state.cascadeId.isLocalDraft()) {
        _uiState.value = state.copy(errorMessage = "新会话还没有内容可导出")
        return
    }
    viewModelScope.launch {
        apiClient.exportConversationMarkdown(state.cascadeId)
            .onSuccess { md ->
                if (md.isBlank()) {
                    _uiState.value = _uiState.value.copy(errorMessage = "会话内容为空，无法导出")
                } else {
                    onReady(state.title, md)
                }
            }
            .onFailure { err ->
                _uiState.value = _uiState.value.copy(errorMessage = "导出失败: ${err.message}")
            }
    }
}

