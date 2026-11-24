package com.antigravity.mobile.ui.viewmodel

import com.antigravity.mobile.ui.util.loc
import androidx.lifecycle.viewModelScope
import com.antigravity.mobile.data.model.ContextMentionItem
import com.antigravity.mobile.data.model.ConversationItem
import com.antigravity.mobile.data.model.ConversationStatus
import com.antigravity.mobile.data.model.SideQuestionCardItem
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

internal fun ChatViewModel.refreshProjectTrust() {
    val cid = _uiState.value.cascadeId
    if (cid.isBlank() || cid.startsWith("local_draft_")) return
    viewModelScope.launch {
        apiClient.fetchProjectTrustStatus(cascadeId = cid).onSuccess { st ->
            _uiState.value = _uiState.value.copy(trustStatus = st)
        }
    }
}

internal fun ChatViewModel.trustCurrentProject(onDone: ((String) -> Unit)? = null) {
    val st = _uiState.value.trustStatus
    val cid = _uiState.value.cascadeId
    if (_uiState.value.isUpdatingTrust) return
    _uiState.value = _uiState.value.copy(isUpdatingTrust = true)
    viewModelScope.launch {
        apiClient.setWorkspaceTrust(
            cascadeId = cid.ifBlank { null },
            projectId = st?.projectId,
            trusted = true
        ).onSuccess { updated ->
            _uiState.value = _uiState.value.copy(trustStatus = updated, isUpdatingTrust = false)
            onDone?.invoke("已信任工作区".loc)
        }.onFailure { err ->
            _uiState.value = _uiState.value.copy(
                isUpdatingTrust = false,
                attachmentNotice = "授予工作区信任失败: ${err.message}".loc
            )
        }
    }
}

internal fun ChatViewModel.setPermissionPreset(preset: String, onDone: ((String) -> Unit)? = null) {
    viewModelScope.launch {
        apiClient.updatePermissionPreset(preset).onSuccess { st ->
            _uiState.value = _uiState.value.copy(permissionState = st)
            onDone?.invoke("权限档位已切换为「${st.label}」".loc)
        }.onFailure { err ->
            _uiState.value = _uiState.value.copy(attachmentNotice = "切换权限失败: ${err.message}".loc)
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
                _uiState.value = _uiState.value.copy(attachmentNotice = resp.error ?: "分叉未返回新会话".loc)
            }
        }.onFailure { err ->
            _uiState.value = _uiState.value.copy(attachmentNotice = "分叉失败: ${err.message}".loc)
        }
    }
}

internal fun ChatViewModel.sendTaskToBackground(stepIndex: Int, onNotice: (String) -> Unit) {
    val cid = _uiState.value.cascadeId
    if (cid.isBlank()) return
    viewModelScope.launch {
        apiClient.sendStepsToBackground(cid, listOf(stepIndex)).onSuccess { ok ->
            onNotice(if (ok) "已转入后台执行".loc else "当前上游版本暂不支持步骤级转后台".loc)
        }.onFailure { err ->
            onNotice("转后台失败: ${err.message}".loc)
        }
    }
}

/** 发起旁路快问分支（可选 stepIndex 精准继承至该条消息及之前上下文），成功后立即进入完整分支会话。 */
internal suspend fun ChatViewModel.startBtwAndOpen(
    question: String,
    stepIndex: Int? = null,
    quote: String? = null,
    onOpened: (cascadeId: String, title: String) -> Unit
): Boolean {
    val trimmedQ = question.trim()
    val trimmedQuote = quote?.trim().orEmpty()
    if (trimmedQ.isEmpty() && trimmedQuote.isEmpty()) return false
    val state = _uiState.value
    val parentId = state.cascadeId
    if (parentId.isBlank() || parentId.startsWith("local_draft_") || parentId.startsWith("draft_")) {
        _uiState.value = state.copy(attachmentNotice = "请先在主会话发送至少一条消息后再发起旁路快问".loc)
        return false
    }
    if (state.isStartingBtw) return false
    _uiState.value = state.copy(isStartingBtw = true)
    return try {
        val res = apiClient.startBtwSideQuestion(
            parentCascadeId = parentId,
            question = trimmedQ,
            quote = trimmedQuote.ifEmpty { null },
            stepIndex = stepIndex,
            model = state.activeModel
        ).getOrElse { err ->
            _uiState.value = _uiState.value.copy(
                isStartingBtw = false,
                attachmentNotice = "发起旁路快问失败: ${err.message}".loc
            )
            return false
        }
        val resolvedTitle = res.title?.trim()?.takeIf { it.isNotEmpty() }
            ?: ConversationItem.sanitizeTitle(trimmedQ.ifEmpty { trimmedQuote.ifEmpty { "旁路快问".loc } })
        val effectiveForkStep = res.forkStepIndex ?: stepIndex ?: maxOf(0, state.stepCount - 1)
        val newItem = ConversationItem(
            id = res.cascadeId,
            title = resolvedTitle,
            status = ConversationStatus.RUNNING,
            stepCount = 1,
            workspaceName = state.workspaceName.ifBlank { "Chat" },
            lastModifiedTime = res.createdAt,
            isSubagent = false,
            isUnread = false,
            isSideQuestion = true,
            branchParentId = parentId,
            branchParentTitle = state.title,
            forkStepIndex = effectiveForkStep
        )
        onConversationUpdated?.invoke(newItem)

        val anchorId = if (stepIndex != null) {
            state.messages.lastOrNull { (it.stepIndex ?: -1) <= stepIndex }?.id ?: state.messages.lastOrNull()?.id
        } else {
            state.messages.lastOrNull()?.id
        }
        val optimisticCard = SideQuestionCardItem(
            cascadeId = res.cascadeId,
            parentCascadeId = parentId,
            title = resolvedTitle,
            question = trimmedQ,
            answerPreview = null,
            status = "CASCADE_RUN_STATUS_RUNNING",
            forkStepIndex = effectiveForkStep,
            anchorMessageId = anchorId,
            turnCount = 1,
            createdAt = res.createdAt
        )
        val updatedCards = if (_uiState.value.sideQuestions.any { it.cascadeId == res.cascadeId }) {
            _uiState.value.sideQuestions
        } else {
            _uiState.value.sideQuestions + optimisticCard
        }
        _uiState.value = _uiState.value.copy(
            isStartingBtw = false,
            sideQuestions = updatedCards
        )
        onOpened(res.cascadeId, resolvedTitle)
        true
    } catch (e: Exception) {
        _uiState.value = _uiState.value.copy(
            isStartingBtw = false,
            attachmentNotice = "发起旁路快问失败: ${e.message}".loc
        )
        false
    }
}

/** 在旁路分支内一键将回复结论插入父会话草稿，并切回父会话。 */
internal fun ChatViewModel.insertConclusionIntoParent(
    text: String,
    onOpenParent: (parentId: String, parentTitle: String) -> Unit
) {
    val origin = _uiState.value.branchOrigin ?: return
    val parentId = origin.parentCascadeId.trim()
    if (parentId.isEmpty()) return
    val trimmed = text.trim()
    if (trimmed.isEmpty()) return
    val snippet = trimmed.take(1600)
    val existing = prefs?.getDraftText(parentId)?.trim().orEmpty()
    val merged = if (existing.isEmpty()) snippet else "$existing\n\n$snippet"
    prefs?.setDraftText(parentId, merged)
    onOpenParent(parentId, origin.parentTitle.orEmpty())
}
