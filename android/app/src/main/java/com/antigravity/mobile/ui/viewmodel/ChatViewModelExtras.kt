package com.antigravity.mobile.ui.viewmodel

import androidx.lifecycle.viewModelScope
import com.antigravity.mobile.data.model.CascadeChangesResponse
import com.antigravity.mobile.data.model.CascadeStepRange
import com.antigravity.mobile.data.model.ConversationChapter
import com.antigravity.mobile.data.model.GitCommitResponse
import com.antigravity.mobile.data.model.GitStatusResponse
import com.antigravity.mobile.data.model.SlashCommandOption
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

// region 快捷按钮可见性

/** 探测当前会话是否有 Git 改动、是否有本会话文件改动及全局后台任务；探测失败（纯对话、非 Git 目录、接口缺失）一律视为没有。 */
suspend fun ChatViewModel.refreshActionAvailability(changesSupported: Boolean) {
    val cascadeId = _uiState.value.cascadeId
    val bgTasks = apiClient.fetchGlobalBackgroundTasks().getOrNull()
    if (cascadeId.isLocalDraft()) {
        _uiState.value = _uiState.value.copy(
            hasGitChanges = false,
            hasSessionChanges = false,
            globalBackgroundTasks = bgTasks ?: _uiState.value.globalBackgroundTasks
        )
        return
    }
    val git = apiClient.getGitStatus(cascadeId).getOrNull()
    val changes = if (changesSupported) apiClient.getCascadeChanges(cascadeId).getOrNull() else null
    if (_uiState.value.cascadeId != cascadeId) return
    val hasFiles = git?.files?.isNotEmpty() == true
    val prs = git?.pullRequests.orEmpty()
    val firstPrNum = prs.firstOrNull()?.number ?: 0
    val chipLabel = when {
        firstPrNum > 0 && hasFiles -> "Commit · #$firstPrNum"
        firstPrNum > 0 -> "PR #$firstPrNum"
        else -> "Commit"
    }
    _uiState.value = _uiState.value.copy(
        hasGitChanges = hasFiles || prs.isNotEmpty(),
        gitChipLabel = chipLabel,
        hasSessionChanges = changes?.hasChanges == true,
        globalBackgroundTasks = bgTasks ?: _uiState.value.globalBackgroundTasks
    )
}

fun ChatViewModel.updateGlobalBackgroundTasks(tasks: List<com.antigravity.mobile.data.model.GlobalBackgroundTaskItem>) {
    _uiState.value = _uiState.value.copy(globalBackgroundTasks = tasks)
}

/** 异步拉取 Antigravity 2.30.0 GetChapterTitles 章节语义标题并更新到 UI 状态。 */
fun ChatViewModel.refreshChapterTitles(chapters: List<ConversationChapter>) {
    val state = _uiState.value
    val cascadeId = state.cascadeId
    if (cascadeId.isLocalDraft() || state.isLoadingChapterTitles || chapters.isEmpty()) return
    val ranges = chapters.mapIndexedNotNull { idx, ch ->
        val s = ch.startStepIndex ?: return@mapIndexedNotNull null
        val isLast = idx == chapters.lastIndex
        val rawEnd = if (isLast) maxOf(ch.endStepIndexExclusive ?: 0, state.stepCount) else (ch.endStepIndexExclusive ?: state.stepCount)
        if (rawEnd <= s) null else CascadeStepRange(startStepIndex = s, endStepIndexExclusive = rawEnd)
    }
    if (ranges.isEmpty()) return

    _uiState.value = _uiState.value.copy(isLoadingChapterTitles = true)
    viewModelScope.launch {
        apiClient.getChapterTitles(cascadeId, ranges)
            .onSuccess { items ->
                if (_uiState.value.cascadeId != cascadeId) return@onSuccess
                val updated = _uiState.value.chapterTitles.toMutableMap()
                for (item in items) {
                    val title = item.title.trim()
                    if (title.isNotEmpty()) {
                        updated["${item.startStepIndex}:${item.endStepIndexExclusive}"] = title
                        updated["start:${item.startStepIndex}"] = title
                    }
                }
                _uiState.value = _uiState.value.copy(
                    chapterTitles = updated,
                    isLoadingChapterTitles = false
                )
            }
            .onFailure {
                if (_uiState.value.cascadeId == cascadeId) {
                    _uiState.value = _uiState.value.copy(isLoadingChapterTitles = false)
                }
            }
    }
}

// endregion

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

// endregion

// region 斜杠命令

/** 首次键入 "/" 时懒加载命令列表；已加载或正在加载时什么都不做。失败后下次键入 "/" 会重试。 */
fun ChatViewModel.ensureSlashCommands() {
    val st = _uiState.value
    if (st.slashCommands.isNotEmpty() || st.isLoadingSlashCommands) return
    _uiState.value = st.copy(isLoadingSlashCommands = true)
    viewModelScope.launch {
        apiClient.getSlashCommands()
            .onSuccess { list ->
                _uiState.value = _uiState.value.copy(slashCommands = list, isLoadingSlashCommands = false)
            }
            .onFailure {
                _uiState.value = _uiState.value.copy(isLoadingSlashCommands = false)
            }
    }
}

/** 选中命令：仅剥离输入框末尾正在输入的 "/xx"，保留此前已输入的正文，命令变成快捷栏按钮。 */
fun ChatViewModel.selectSlashCommand(command: SlashCommandOption) {
    _inputText.value = com.antigravity.mobile.data.model.SlashCommandFilter.stripTrailingQuery(_inputText.value)
    _uiState.value = _uiState.value.copy(selectedSlashCommand = command, focusInputTrigger = _uiState.value.focusInputTrigger + 1)
}

fun ChatViewModel.clearSlashCommand() {
    _uiState.value = _uiState.value.copy(selectedSlashCommand = null)
}

// endregion
