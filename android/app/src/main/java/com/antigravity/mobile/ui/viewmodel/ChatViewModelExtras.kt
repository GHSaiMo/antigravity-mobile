package com.antigravity.mobile.ui.viewmodel

import androidx.lifecycle.viewModelScope
import com.antigravity.mobile.data.model.CascadeChangesResponse
import com.antigravity.mobile.data.model.GitCommitResponse
import com.antigravity.mobile.data.model.GitStatusResponse
import kotlinx.coroutines.launch

/** 「本会话改动」浮窗状态。 */
data class ChangesSheetState(
    val isLoading: Boolean = true,
    val data: CascadeChangesResponse? = null,
    val error: String? = null
)

/** Git 提交浮窗状态。 */
data class GitSheetState(
    val isLoading: Boolean = true,
    val status: GitStatusResponse? = null,
    val error: String? = null,
    val isWorking: Boolean = false,
    val result: GitCommitResponse? = null,
    val actionError: String? = null
)

private fun String.isLocalDraft(): Boolean = isBlank() || startsWith("local_draft_")

// region 本会话改动

fun ChatViewModel.openChangesSheet() {
    val cascadeId = _uiState.value.cascadeId
    if (cascadeId.isLocalDraft()) {
        _uiState.value = _uiState.value.copy(errorMessage = "新会话还没有任何改动")
        return
    }
    _uiState.value = _uiState.value.copy(changesSheet = ChangesSheetState())
    viewModelScope.launch {
        apiClient.getCascadeChanges(cascadeId)
            .onSuccess { res ->
                _uiState.value = _uiState.value.copy(changesSheet = ChangesSheetState(isLoading = false, data = res))
            }
            .onFailure { err ->
                _uiState.value = _uiState.value.copy(
                    changesSheet = ChangesSheetState(isLoading = false, error = err.message ?: "加载失败")
                )
            }
    }
}

fun ChatViewModel.closeChangesSheet() {
    _uiState.value = _uiState.value.copy(changesSheet = null)
}

// endregion

// region Git 直接提交

fun ChatViewModel.openGitSheet() {
    val cascadeId = _uiState.value.cascadeId
    if (cascadeId.isLocalDraft()) {
        _uiState.value = _uiState.value.copy(errorMessage = "新会话还没有关联的工作区")
        return
    }
    _uiState.value = _uiState.value.copy(gitSheet = GitSheetState())
    refreshGitStatus()
}

fun ChatViewModel.refreshGitStatus() {
    val cascadeId = _uiState.value.cascadeId
    val current = _uiState.value.gitSheet ?: return
    _uiState.value = _uiState.value.copy(gitSheet = current.copy(isLoading = true, error = null))
    viewModelScope.launch {
        apiClient.getGitStatus(cascadeId)
            .onSuccess { st ->
                val sheet = _uiState.value.gitSheet ?: return@onSuccess
                _uiState.value = _uiState.value.copy(gitSheet = sheet.copy(isLoading = false, status = st, error = null))
            }
            .onFailure { err ->
                val sheet = _uiState.value.gitSheet ?: return@onFailure
                _uiState.value = _uiState.value.copy(
                    gitSheet = sheet.copy(isLoading = false, error = err.message ?: "加载失败")
                )
            }
    }
}

/** 直接调用网关提交（可选推送）。paths 为空表示提交全部改动。 */
fun ChatViewModel.commitGit(message: String, paths: List<String>, push: Boolean) {
    val cascadeId = _uiState.value.cascadeId
    val sheet = _uiState.value.gitSheet ?: return
    if (sheet.isWorking) return
    _uiState.value = _uiState.value.copy(gitSheet = sheet.copy(isWorking = true, actionError = null))
    viewModelScope.launch {
        apiClient.gitCommit(cascadeId, message, paths, push)
            .onSuccess { res ->
                val s = _uiState.value.gitSheet ?: return@onSuccess
                _uiState.value = _uiState.value.copy(gitSheet = s.copy(isWorking = false, result = res))
            }
            .onFailure { err ->
                val s = _uiState.value.gitSheet ?: return@onFailure
                _uiState.value = _uiState.value.copy(
                    gitSheet = s.copy(isWorking = false, actionError = err.message ?: "提交失败")
                )
            }
    }
}

/** 提交已成功但推送失败时，单独重试推送。 */
fun ChatViewModel.retryGitPush() {
    val cascadeId = _uiState.value.cascadeId
    val sheet = _uiState.value.gitSheet ?: return
    if (sheet.isWorking) return
    _uiState.value = _uiState.value.copy(gitSheet = sheet.copy(isWorking = true, actionError = null))
    viewModelScope.launch {
        apiClient.gitPush(cascadeId)
            .onSuccess {
                val s = _uiState.value.gitSheet ?: return@onSuccess
                _uiState.value = _uiState.value.copy(
                    gitSheet = s.copy(isWorking = false, result = s.result?.copy(pushed = true, pushError = null))
                )
            }
            .onFailure { err ->
                val s = _uiState.value.gitSheet ?: return@onFailure
                _uiState.value = _uiState.value.copy(
                    gitSheet = s.copy(isWorking = false, actionError = err.message ?: "推送失败")
                )
            }
    }
}

fun ChatViewModel.closeGitSheet() {
    _uiState.value = _uiState.value.copy(gitSheet = null)
}

/** 回退到「让 Agent 来提交」：保留原有行为，适合需要 Agent 理解改动并写提交信息的场景。 */
fun ChatViewModel.delegateCommitToAgent() {
    closeGitSheet()
    insertCommitAndPush()
}

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

