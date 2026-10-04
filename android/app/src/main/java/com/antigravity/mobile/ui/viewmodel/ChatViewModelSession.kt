package com.antigravity.mobile.ui.viewmodel

import android.net.Uri
import androidx.lifecycle.viewModelScope
import com.antigravity.mobile.data.model.*
import kotlinx.coroutines.launch

internal fun ChatViewModel.saveSessionToCache() {
    val state = _uiState.value
    val cid = state.cascadeId
    if (cid.isBlank() || cid.startsWith("local_draft_")) return
    val cm = cacheManager ?: return

    val toCache = state.messages.filter { !it.id.startsWith("opt_") }
    val statusString = when {
        state.isLatestMessageError -> "CASCADE_RUN_STATUS_ERROR"
        state.isRunning -> "CASCADE_RUN_STATUS_RUNNING"
        else -> "CASCADE_RUN_STATUS_IDLE"
    }

    val session = CachedChatSession(
        cascadeId = cid,
        status = statusString,
        duration = state.duration,
        stepCount = if (state.stepCount > 0) state.stepCount else maxOf(0, state.messages.count { !it.isUser && !it.isTools }),
        totalTools = state.totalTools,
        hasMore = state.hasMore,
        nextOffset = state.nextOffset,
        messages = toCache,
        title = state.title.takeIf { it.isNotBlank() && it != "会话详情" && it != "未命名会话" },
        workspaceName = state.workspaceName.takeIf { it.isNotBlank() && it != "Chat" },
        cascadeConfigRaw = state.cascadeConfigRaw,
        canProceed = state.canProceed,
        proceedArtifactUri = state.proceedArtifactUri,
        pendingInteraction = state.pendingInteraction,
        queuedMessages = state.queuedMessages,
        runningTasks = state.runningTasks,
        activeModel = state.activeModel
    )
    cm.saveSession(session)
}

internal fun ChatViewModel.currentConversationItem(): ConversationItem? {
    val state = _uiState.value
    if (state.cascadeId.isBlank()) return null
    if (state.cascadeId.startsWith("local_draft_")) {
        val draft = prefs?.getLocalDraftSession(state.cascadeId)
        val draftText = prefs?.getDraftText(state.cascadeId).orEmpty().ifBlank { _inputText.value }
        val hasImages = prefs?.hasDraftImages(state.cascadeId) == true || state.selectedImages.isNotEmpty()
        if (draftText.isBlank() && !hasImages) return null
        val sessionToUse = draft ?: currentDraftProject?.let {
            LocalDraftSession(id = state.cascadeId, project = it, draftText = draftText)
        }
        return sessionToUse?.copy(draftText = draftText)?.toConversationItem(hasImages)
    }
    val status = when {
        state.pendingInteraction != null || state.canProceed -> ConversationStatus.ACTION
        state.isLatestMessageError -> ConversationStatus.ERROR
        state.isRunning || state.isAwaitingResponse -> ConversationStatus.RUNNING
        else -> ConversationStatus.IDLE
    }
    val stepCount = maxOf(0, state.messages.count { !it.isUser && !it.isTools })
    val wsName = state.workspaceName.takeIf { it.isNotBlank() && it != "Chat" }
        ?: state.workspaceFolder?.trimEnd('/')?.substringAfterLast('/')?.takeIf { it.isNotBlank() }
        ?: "Chat"

    val title = when {
        state.title.isNotBlank() && state.title != "会话详情" && state.title != "未命名会话" && !state.title.startsWith("会话 ") ->
            state.title
        wsName != "Chat" -> wsName
        else -> "新对话"
    }

    return ConversationItem(
        id = state.cascadeId,
        title = title,
        status = status,
        stepCount = stepCount,
        workspaceName = wsName,
        lastModifiedTime = currentLastModifiedTime,
        isSubagent = false,
        isUnread = false
    )
}

internal fun ChatViewModel.notifyConversationUpdated() {
    currentConversationItem()?.let { item ->
        onConversationUpdated?.invoke(item)
    }
}

internal fun ChatViewModel.resetSession() {
    fetchJob?.cancel()
    fetchJob = null
    wsJob?.cancel()
    wsJob = null
    wsClient.disconnect()
    pendingOptimisticQueueItems.clear()
    deletedQueueTombstones.clear()
    inFlightDeletingQueueIds.clear()
    stoppedTaskKeys.clear()
    currentDraftProject = null
    currentLastModifiedTime = null
    _inputText.value = ""
    _uiState.value = ChatUiState()
}

internal fun ChatViewModel.prepareSession(
    cascadeId: String,
    initialTitle: String? = null,
    isNewConversation: Boolean = false,
    workspaceName: String? = null,
    isUnread: Boolean = false,
    conversationStatus: ConversationStatus? = null,
    draftProject: ProjectItem? = null,
    lastModifiedTime: String? = null
) {
    this.currentLastModifiedTime = lastModifiedTime
    this.isUnreadOnEntry = isUnread
    this.initialConversationStatus = conversationStatus

    fetchJob?.cancel()
    fetchJob = null
    wsJob?.cancel()
    wsJob = null
    wsClient.disconnect()
    pendingOptimisticQueueItems.clear()
    deletedQueueTombstones.clear()
    inFlightDeletingQueueIds.clear()
    stoppedTaskKeys.clear()

    val isDraft = cascadeId.startsWith("local_draft_")
    val resolvedDraftProject = draftProject ?: (if (isDraft) prefs?.getLocalDraftSession(cascadeId)?.project else null)
    this.currentDraftProject = resolvedDraftProject

    val resolvedWs = workspaceName?.takeIf { it.isNotBlank() }
        ?: (if (resolvedDraftProject?.isPureChat == true) "Chat" else resolvedDraftProject?.name.orEmpty())
    val defaultTitle = if (isDraft) (if (resolvedWs.isNotBlank() && resolvedWs != "Chat") resolvedWs else "新对话") else "会话 $cascadeId"

    // Restore draft text and images immediately
    val savedDraftText = prefs?.getDraftText(cascadeId).orEmpty()
    _inputText.value = savedDraftText

    val draftImages = prefs?.loadDraftImages(cascadeId).orEmpty()
    val attachments = if (draftImages.isNotEmpty()) {
        draftImages.map { bytes ->
            AttachmentImage(
                uri = Uri.EMPTY,
                bitmap = null,
                byteArray = bytes
            )
        }
    } else emptyList()

    // Restore instant cache if available
    val cached = if (!isDraft) cacheManager?.loadSession(cascadeId) else null
    if (cached != null) {
        val healed = sanitizeMessageOrder(cached.messages)
        val hasEarliest = healed.any { (it.stepIndex ?: extractStepIndex(it.id)) == 0 }
        val hasMore = if (hasEarliest) false else cached.hasMore
        val nextOffset = if (hasEarliest) 0 else cached.nextOffset
        val isRunning = cached.status.equals("CASCADE_RUN_STATUS_RUNNING", ignoreCase = true) || cached.status.equals("RUNNING", ignoreCase = true)
        val resolvedTitle = cached.title?.takeIf { it.isNotBlank() && it != "未命名会话" && it != "会话详情" }
            ?: initialTitle?.takeIf { it.isNotBlank() }
            ?: defaultTitle

        _uiState.value = ChatUiState(
            cascadeId = cascadeId,
            title = resolvedTitle,
            workspaceName = cached.workspaceName?.takeIf { it.isNotBlank() } ?: resolvedWs,
            messages = healed,
            runningTasks = cached.runningTasks ?: emptyList(),
            queuedMessages = (cached.queuedMessages ?: emptyList()).filter { isUserQueuedItem(it) },
            isLoading = false,
            isNewConversation = false,
            isRunning = isRunning,
            isAwaitingResponse = false,
            selectedImages = attachments,
            canProceed = cached.canProceed ?: false,
            proceedArtifactUri = cached.proceedArtifactUri,
            pendingInteraction = cached.pendingInteraction,
            activeModel = cached.activeModel ?: _uiState.value.activeModel,
            errorMessage = null,
            isLatestMessageError = cached.status.equals("CASCADE_RUN_STATUS_ERROR", ignoreCase = true) || cached.status.equals("ERROR", ignoreCase = true),
            hasMore = hasMore,
            nextOffset = nextOffset,
            stepCount = cached.stepCount,
            totalTools = cached.totalTools,
            duration = cached.duration,
            cascadeConfigRaw = cached.cascadeConfigRaw
        )
    } else {
        _uiState.value = ChatUiState(
            cascadeId = cascadeId,
            title = initialTitle?.takeIf { it.isNotBlank() } ?: defaultTitle,
            workspaceName = resolvedWs,
            messages = emptyList(),
            runningTasks = emptyList(),
            queuedMessages = emptyList(),
            isLoading = !isNewConversation && !isDraft,
            isNewConversation = isNewConversation,
            isRunning = false,
            isAwaitingResponse = false,
            selectedImages = attachments,
            canProceed = false,
            proceedArtifactUri = null,
            pendingInteraction = null,
            activeModel = _uiState.value.activeModel,
            errorMessage = null,
            isLatestMessageError = false,
            markdownViewerData = null
        )
    }
}

internal fun ChatViewModel.initSession(
    cascadeId: String,
    initialTitle: String? = null,
    isNewConversation: Boolean = false,
    workspaceName: String? = null,
    isUnread: Boolean = false,
    conversationStatus: ConversationStatus? = null,
    draftProject: ProjectItem? = null
) {
    val isDifferentSession = _uiState.value.cascadeId != cascadeId
    if (isDifferentSession) {
        this.isUnreadOnEntry = isUnread
        this.initialConversationStatus = conversationStatus
    } else if (isUnread) {
        this.isUnreadOnEntry = true
    }
    val isDraft = cascadeId.startsWith("local_draft_")
    val resolvedDraftProject = draftProject ?: this.currentDraftProject ?: (if (isDraft) prefs?.getLocalDraftSession(cascadeId)?.project else null)
    this.currentDraftProject = resolvedDraftProject

    val shouldLoad = !isNewConversation && !isDraft
    val fallbackWs = (if (resolvedDraftProject?.isPureChat == true) "Chat" else resolvedDraftProject?.name) ?: _uiState.value.workspaceName
    val resolvedWs = workspaceName?.takeIf { it.isNotBlank() } ?: fallbackWs
    val defaultTitle = if (isDraft) (if (resolvedWs.isNotBlank() && resolvedWs != "Chat") resolvedWs else "新对话") else "会话 $cascadeId"

    if (isDifferentSession) {
        fetchJob?.cancel()
        fetchJob = null
        wsClient.disconnect()
        pendingOptimisticQueueItems.clear()
        deletedQueueTombstones.clear()
        inFlightDeletingQueueIds.clear()
        stoppedTaskKeys.clear()

        val savedDraftText = prefs?.getDraftText(cascadeId).orEmpty()
        _inputText.value = savedDraftText

        val draftImages = prefs?.loadDraftImages(cascadeId).orEmpty()
        val attachments = if (draftImages.isNotEmpty()) {
            draftImages.map { bytes ->
                AttachmentImage(
                    uri = Uri.EMPTY,
                    bitmap = null,
                    byteArray = bytes
                )
            }
        } else emptyList()

        val cached = if (!isDraft) cacheManager?.loadSession(cascadeId) else null
        if (cached != null) {
            val healed = sanitizeMessageOrder(cached.messages)
            val hasEarliest = healed.any { (it.stepIndex ?: extractStepIndex(it.id)) == 0 }
            val hasMore = if (hasEarliest) false else cached.hasMore
            val nextOffset = if (hasEarliest) 0 else cached.nextOffset
            val isRunning = cached.status.equals("CASCADE_RUN_STATUS_RUNNING", ignoreCase = true) || cached.status.equals("RUNNING", ignoreCase = true)
            val resolvedTitle = cached.title?.takeIf { it.isNotBlank() && it != "未命名会话" && it != "会话详情" }
                ?: initialTitle?.takeIf { it.isNotBlank() }
                ?: defaultTitle

            _uiState.value = ChatUiState(
                cascadeId = cascadeId,
                title = resolvedTitle,
                workspaceName = cached.workspaceName?.takeIf { it.isNotBlank() } ?: resolvedWs,
                messages = healed,
                runningTasks = cached.runningTasks ?: emptyList(),
                queuedMessages = (cached.queuedMessages ?: emptyList()).filter { isUserQueuedItem(it) },
                isLoading = false,
                isNewConversation = false,
                isRunning = isRunning,
                isAwaitingResponse = false,
                selectedImages = attachments,
                canProceed = cached.canProceed ?: false,
                proceedArtifactUri = cached.proceedArtifactUri,
                pendingInteraction = cached.pendingInteraction,
                activeModel = cached.activeModel ?: _uiState.value.activeModel,
                errorMessage = null,
                isLatestMessageError = cached.status.equals("CASCADE_RUN_STATUS_ERROR", ignoreCase = true) || cached.status.equals("ERROR", ignoreCase = true),
                hasMore = hasMore,
                nextOffset = nextOffset,
                stepCount = cached.stepCount,
                totalTools = cached.totalTools,
                duration = cached.duration,
                cascadeConfigRaw = cached.cascadeConfigRaw
            )
        } else {
            _uiState.value = ChatUiState(
                cascadeId = cascadeId,
                title = initialTitle?.takeIf { it.isNotBlank() } ?: defaultTitle,
                workspaceName = resolvedWs,
                messages = emptyList(),
                runningTasks = emptyList(),
                queuedMessages = emptyList(),
                isLoading = shouldLoad,
                isNewConversation = isNewConversation,
                isRunning = false,
                isAwaitingResponse = false,
                selectedImages = attachments,
                canProceed = false,
                proceedArtifactUri = null,
                pendingInteraction = null,
                activeModel = _uiState.value.activeModel,
                errorMessage = null,
                isLatestMessageError = false,
                markdownViewerData = null
            )
        }
    } else {
        _uiState.value = _uiState.value.copy(
            title = initialTitle?.takeIf { it.isNotBlank() } ?: _uiState.value.title,
            workspaceName = if (resolvedWs.isNotBlank()) resolvedWs else _uiState.value.workspaceName,
            isNewConversation = isNewConversation,
            isLoading = if (_uiState.value.messages.isEmpty() && !isNewConversation && !isDraft) true else false
        )
    }

    // Restore draft if available
    prefs?.let { p ->
        val draftText = p.getDraftText(cascadeId)
        if (draftText.isNotBlank() && _inputText.value.isBlank()) {
            _inputText.value = draftText
        }
        val draftImages = p.loadDraftImages(cascadeId)
        if (draftImages.isNotEmpty() && _uiState.value.selectedImages.isEmpty()) {
            val attachments = draftImages.map { bytes ->
                AttachmentImage(
                    uri = Uri.EMPTY,
                    bitmap = null,
                    byteArray = bytes
                )
            }
            if (attachments.isNotEmpty()) {
                _uiState.value = _uiState.value.copy(selectedImages = attachments)
            }
        }
    }

    if (isDraft) {
        return
    }

    apiClient.notifySessionFocus(cascadeId)

    viewModelScope.launch {
        apiClient.markConversationAsRead(cascadeId)
    }

    // Fetch cached messages via HTTP so entering session loads instantly without wiping local cache
    fetchJob = viewModelScope.launch {
        if (!isNewConversation && _uiState.value.messages.isEmpty()) {
            _uiState.value = _uiState.value.copy(isLoading = true, errorMessage = null)
        }
        apiClient.fetchMessages(cascadeId, limit = 15)
            .onSuccess { payload ->
                if (_uiState.value.cascadeId == cascadeId) {
                    val incoming = payload.messages ?: emptyList()
                    val mergedMsgs = if (_uiState.value.messages.isNotEmpty()) {
                        mergeIncomingMessages(incoming)
                    } else {
                        sanitizeMessageOrder(incoming)
                    }

                    val lastMsg = mergedMsgs.lastOrNull()
                    val isError = lastMsg?.status.equals("error", ignoreCase = true) || payload.hasError
                    val wsFromPayload = payload.workspaceUri?.trimEnd('/')?.substringAfterLast('/')?.takeIf { it.isNotBlank() }

                    val hasEarliest = mergedMsgs.any { (it.stepIndex ?: extractStepIndex(it.id)) == 0 }
                    val hasMore = if (hasEarliest) false else payload.hasMore
                    val nextOffset = if (hasEarliest) 0 else payload.nextOffset

                    val resolvedTitle = payload.title?.takeIf { it.isNotBlank() && it != "未命名会话" && it != "会话详情" }
                        ?: _uiState.value.title

                    val isRunning = isStatusRunning(payload.status)
                    val activeRunningTasks = filterRunningTasks(payload.runningTasks)
                    _uiState.value = _uiState.value.copy(
                        isLoading = false,
                        title = resolvedTitle,
                        workspaceName = wsFromPayload ?: _uiState.value.workspaceName,
                        messages = mergedMsgs,
                        runningTasks = activeRunningTasks,
                        queuedMessages = syncQueuedMessages(payload.queuedMessages, mergedMsgs),
                        isRunning = isRunning,
                        canProceed = payload.canProceed,
                        proceedArtifactUri = payload.proceedArtifactUri,
                        pendingInteraction = payload.pendingInteraction,
                        activeModel = payload.activeModel?.let { raw ->
                            if (raw.contains("claude", ignoreCase = true) || raw.contains("m26", ignoreCase = true)) {
                                "claude-opus-4-6-thinking"
                            } else {
                                "gemini-3.8-flash-high"
                            }
                        } ?: _uiState.value.activeModel,
                        errorMessage = if (payload.hasError && _uiState.value.messages.isEmpty()) payload.errorMessage else null,
                        isLatestMessageError = isError,
                        hasMore = hasMore,
                        nextOffset = nextOffset,
                        stepCount = payload.totalSteps,
                        totalTools = payload.totalTools,
                        duration = payload.duration ?: _uiState.value.duration,
                        cascadeConfigRaw = payload.cascadeConfigRaw ?: _uiState.value.cascadeConfigRaw
                    )

                    if (isRunning) {
                        val stepCount = mergedMsgs.count { !it.isUser }
                        val latestAction = when {
                            payload.pendingInteraction != null -> payload.pendingInteraction.prompt ?: "需要审批操作"
                            activeRunningTasks.isNotEmpty() -> activeRunningTasks.firstOrNull()?.displayCommand?.ifBlank { "正在执行后台任务..." } ?: "正在执行后台任务..."
                            lastMsg?.toolCalls?.isNotEmpty() == true -> "正在执行: " + (lastMsg.toolCalls.lastOrNull()?.name ?: "操作")
                            else -> "正在执行任务..."
                        }
                        liveActivityManager?.startOrUpdateActivity(
                            title = resolvedTitle,
                            cascadeId = cascadeId,
                            status = payload.status ?: "RUNNING",
                            stepCount = maxOf(1, stepCount),
                            latestAction = latestAction,
                            runningTaskCount = activeRunningTasks.size,
                            hasPendingAction = payload.pendingInteraction != null
                        )
                    } else if (liveActivityManager?.hasNotification(cascadeId) == true) {
                        val finalStatus = if (isError) "FAILED" else "COMPLETED"
                        liveActivityManager?.endActivity(cascadeId = cascadeId, finalStatus = finalStatus)
                    }

                    _scrollToBottomTrigger.value++
                    notifyConversationUpdated()
                    saveSessionToCache()
                }
            }
            .onFailure { error ->
                if (_uiState.value.cascadeId == cascadeId) {
                    _uiState.value = _uiState.value.copy(
                        isLoading = false,
                        errorMessage = if (_uiState.value.messages.isEmpty()) (error.localizedMessage ?: "同步会话历史失败") else null
                    )
                }
            }
    }

    wsClient.connect(cascadeId)
    ensureWebSocketObserving()
}

internal fun ChatViewModel.retryLoadMessages() {
    val cascadeId = _uiState.value.cascadeId
    if (cascadeId.isBlank()) return
    initSession(
        cascadeId = cascadeId,
        initialTitle = _uiState.value.title,
        isNewConversation = _uiState.value.isNewConversation,
        workspaceName = _uiState.value.workspaceName,
        isUnread = isUnreadOnEntry,
        conversationStatus = initialConversationStatus
    )
}

internal fun ChatViewModel.ensureWebSocketObserving() {
    if (wsJob?.isActive == true) return

    wsJob = viewModelScope.launch {
        launch {
            wsClient.connectionStatus.collect { status ->
                _uiState.value = _uiState.value.copy(connectionStatus = status)
            }
        }

        launch {
            wsClient.streamUpdates.collect { payload ->
                if (payload == null) return@collect
                if (payload.cascadeId != _uiState.value.cascadeId) return@collect

                val rawMsgs = payload.messages
                val msgs = if (rawMsgs != null) {
                    mergeIncomingMessages(rawMsgs)
                } else {
                    _uiState.value.messages
                }
                val lastMsg = msgs.lastOrNull()
                val isError = lastMsg?.status.equals("error", ignoreCase = true) || payload.hasError

                var awaiting = _uiState.value.isAwaitingResponse
                if (isError) {
                    awaiting = false
                } else if (awaiting) {
                    val lastUserIdx = msgs.indexOfLast { it.isUser }
                    if (lastUserIdx >= 0) {
                        val subsequent = msgs.subList(lastUserIdx + 1, msgs.size)
                        val hasAgent = subsequent.any { !it.isUser && !it.isTools }
                        val hasSubsequentError = subsequent.any { it.status.equals("error", ignoreCase = true) }
                        if (hasAgent || hasSubsequentError) {
                            awaiting = false
                        }
                    }
                }

                val isRunning = isStatusRunning(payload.status) || awaiting
                val wasRunning = _uiState.value.isRunning
                val previousMsgCount = _uiState.value.messages.size
                val wsFromPayload = payload.workspaceUri?.trimEnd('/')?.substringAfterLast('/')?.takeIf { it.isNotBlank() }

                val hasEarliest = msgs.any { (it.stepIndex ?: extractStepIndex(it.id)) == 0 }
                val hasMore = if (hasEarliest) false else (if (payload.hasMore) true else _uiState.value.hasMore)
                val nextOffset = if (hasEarliest) 0 else (if (payload.nextOffset > 0) payload.nextOffset else _uiState.value.nextOffset)

                val activeRunningTasks = filterRunningTasks(payload.runningTasks)
                _uiState.value = _uiState.value.copy(
                    isLoading = false,
                    title = payload.title?.takeIf { it.isNotBlank() } ?: _uiState.value.title,
                    workspaceName = wsFromPayload ?: _uiState.value.workspaceName,
                    messages = msgs,
                    runningTasks = activeRunningTasks,
                    queuedMessages = syncQueuedMessages(payload.queuedMessages, msgs),
                    isRunning = isRunning,
                    isAwaitingResponse = awaiting,
                    canProceed = payload.canProceed,
                    proceedArtifactUri = payload.proceedArtifactUri,
                    pendingInteraction = payload.pendingInteraction,
                    activeModel = payload.activeModel?.let { raw ->
                        if (raw.contains("claude", ignoreCase = true) || raw.contains("m26", ignoreCase = true)) {
                            "claude-opus-4-6-thinking"
                        } else {
                            "gemini-3.8-flash-high"
                        }
                    } ?: _uiState.value.activeModel,
                    errorMessage = if (payload.hasError) payload.errorMessage else null,
                    isLatestMessageError = isError,
                    hasMore = hasMore,
                    nextOffset = nextOffset,
                    stepCount = if (payload.totalSteps > 0) payload.totalSteps else _uiState.value.stepCount,
                    totalTools = if (payload.totalTools > 0) payload.totalTools else _uiState.value.totalTools,
                    duration = payload.duration ?: _uiState.value.duration,
                    cascadeConfigRaw = payload.cascadeConfigRaw ?: _uiState.value.cascadeConfigRaw
                )

                if (isRunning) {
                    val stepCount = msgs.count { !it.isUser }
                    val latestAction = when {
                        payload.pendingInteraction != null -> payload.pendingInteraction.prompt ?: "需要审批操作"
                        activeRunningTasks.isNotEmpty() -> activeRunningTasks.firstOrNull()?.displayCommand?.ifBlank { "正在执行后台任务..." } ?: "正在执行后台任务..."
                        lastMsg?.toolCalls?.isNotEmpty() == true -> "正在执行: " + (lastMsg.toolCalls.lastOrNull()?.name ?: "操作")
                        awaiting -> "正在思考并组织回复..."
                        else -> "正在执行任务..."
                    }
                    liveActivityManager?.startOrUpdateActivity(
                        title = _uiState.value.title,
                        cascadeId = _uiState.value.cascadeId,
                        status = payload.status,
                        stepCount = maxOf(1, stepCount),
                        latestAction = latestAction,
                        runningTaskCount = activeRunningTasks.size,
                        hasPendingAction = payload.pendingInteraction != null
                    )
                } else if (wasRunning || liveActivityManager?.hasNotification(_uiState.value.cascadeId) == true) {
                    val finalStatus = if (isError) "FAILED" else "COMPLETED"
                    liveActivityManager?.endActivity(cascadeId = _uiState.value.cascadeId, finalStatus = finalStatus)
                }

                if (msgs.size != previousMsgCount || isRunning) {
                    _scrollToBottomTrigger.value++
                }

                notifyConversationUpdated()
                saveSessionToCache()
            }
        }
    }
}

internal fun ChatViewModel.refresh(onComplete: (() -> Unit)? = null) {
    val cid = _uiState.value.cascadeId
    if (cid.isBlank()) {
        onComplete?.invoke()
        return
    }
    _isRefreshing.value = true
    viewModelScope.launch {
        try {
            apiClient.fetchMessages(cid, limit = 25)
                .onSuccess { payload ->
                    if (_uiState.value.cascadeId == cid) {
                        val incoming = payload.messages ?: emptyList()
                        val msgs = if (_uiState.value.messages.isNotEmpty()) {
                            mergeIncomingMessages(incoming)
                        } else {
                            sanitizeMessageOrder(incoming)
                        }
                        val lastMsg = msgs.lastOrNull()
                        val isError = lastMsg?.status.equals("error", ignoreCase = true) || payload.hasError
                        val wsFromPayload = payload.workspaceUri?.trimEnd('/')?.substringAfterLast('/')?.takeIf { it.isNotBlank() }

                        val hasEarliest = msgs.any { (it.stepIndex ?: extractStepIndex(it.id)) == 0 }
                        val hasMore = if (hasEarliest) false else payload.hasMore
                        val nextOffset = if (hasEarliest) 0 else payload.nextOffset

                        val isRunning = isStatusRunning(payload.status)
                        _uiState.value = _uiState.value.copy(
                            isLoading = false,
                            title = payload.title?.takeIf { it.isNotBlank() } ?: _uiState.value.title,
                            workspaceName = wsFromPayload ?: _uiState.value.workspaceName,
                            messages = msgs,
                            runningTasks = filterRunningTasks(payload.runningTasks),
                            queuedMessages = syncQueuedMessages(payload.queuedMessages, msgs),
                            isRunning = isRunning,
                            isLatestMessageError = isError,
                            hasMore = hasMore,
                            nextOffset = nextOffset,
                            stepCount = payload.totalSteps,
                            totalTools = payload.totalTools,
                            duration = payload.duration ?: _uiState.value.duration,
                            cascadeConfigRaw = payload.cascadeConfigRaw ?: _uiState.value.cascadeConfigRaw
                        )
                        if (!isRunning && liveActivityManager?.hasNotification(cid) == true) {
                            val finalStatus = if (isError) "FAILED" else "COMPLETED"
                            liveActivityManager?.endActivity(cascadeId = cid, finalStatus = finalStatus)
                        }
                        notifyConversationUpdated()
                        saveSessionToCache()
                    }
                }
        } finally {
            _isRefreshing.value = false
            onComplete?.invoke()
        }
    }
}

internal fun ChatViewModel.loadOlderMessages() {
    val state = _uiState.value
    val cascadeId = state.cascadeId
    if (cascadeId.isBlank() || !state.hasMore || state.isLoadingOlder) return

    val hasEarliest = state.messages.any { (it.stepIndex ?: extractStepIndex(it.id)) == 0 }
    if (hasEarliest) {
        _uiState.value = state.copy(hasMore = false, nextOffset = 0)
        return
    }

    _uiState.value = state.copy(isLoadingOlder = true)

    viewModelScope.launch {
        apiClient.fetchMessages(cascadeId, limit = 15, offset = state.nextOffset)
            .onSuccess { payload ->
                if (_uiState.value.cascadeId == cascadeId) {
                    val incoming = payload.messages ?: emptyList()
                    val existingIds = _uiState.value.messages.map { it.id }.toSet()
                    val uniqueOlder = incoming.filter { !existingIds.contains(it.id) }
                    val combined = sanitizeMessageOrder(uniqueOlder + _uiState.value.messages)

                    val containsEarliest = combined.any { (it.stepIndex ?: extractStepIndex(it.id)) == 0 }
                    val newHasMore = if (!payload.hasMore || uniqueOlder.isEmpty() || payload.nextOffset <= 0 || containsEarliest) {
                        false
                    } else {
                        payload.hasMore
                    }
                    val newNextOffset = if (!newHasMore) 0 else payload.nextOffset

                    _uiState.value = _uiState.value.copy(
                        messages = combined,
                        isLoadingOlder = false,
                        hasMore = newHasMore,
                        nextOffset = newNextOffset,
                        stepCount = maxOf(_uiState.value.stepCount, payload.totalSteps),
                        totalTools = maxOf(_uiState.value.totalTools, payload.totalTools)
                    )
                    saveSessionToCache()
                }
            }
            .onFailure {
                if (_uiState.value.cascadeId == cascadeId) {
                    _uiState.value = _uiState.value.copy(isLoadingOlder = false)
                }
            }
    }
}
