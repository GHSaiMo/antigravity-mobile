package com.antigravity.mobile.ui.viewmodel

import com.antigravity.mobile.ui.util.loc
import android.net.Uri
import androidx.lifecycle.viewModelScope
import com.antigravity.mobile.data.model.*
import com.antigravity.mobile.data.service.ConnectionStatus
import com.antigravity.mobile.data.service.LiveActivityStatusHelper
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

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
        stepCount = if (state.stepCount > 0) state.stepCount else maxOf(0, state.messages.count { !it.isUser && !it.isTools && !it.isSubagent }),
        totalTools = state.totalTools,
        hasMore = state.hasMore,
        nextOffset = state.nextOffset,
        messages = toCache,
        title = state.title.takeIf { it.isNotBlank() && it != "会话详情".loc && it != "未命名会话".loc },
        workspaceName = state.workspaceName.takeIf { it.isNotBlank() && it != "Chat" },
        cascadeConfigRaw = state.cascadeConfigRaw,
        canProceed = state.canProceed,
        proceedArtifactUri = state.proceedArtifactUri,
        pendingInteraction = state.pendingInteraction,
        queuedMessages = state.queuedMessages,
        runningTasks = state.runningTasks,
        activeModel = state.activeModel,
        activeModelName = state.activeModelName,
        startedAt = state.startedAt
    )
    cm.saveSession(session)
}

internal fun ChatViewModel.currentConversationItem(): ConversationItem? {
    val state = _uiState.value
    if (state.cascadeId.isBlank()) return null
    if (state.cascadeId.startsWith("local_draft_")) {
        val draft = prefs?.getLocalDraftSession(state.cascadeId)
        val draftText = prefs?.getDraftText(state.cascadeId).orEmpty().ifBlank { _inputText.value }
        val hasImages = prefs?.hasDraftImages(state.cascadeId) == true || state.selectedImages.isNotEmpty() ||
            prefs?.hasDraftFiles(state.cascadeId) == true || state.selectedFiles.isNotEmpty()
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
    val stepCount = maxOf(0, state.messages.count { !it.isUser && !it.isTools && !it.isSubagent })
    val wsName = state.workspaceName.takeIf { it.isNotBlank() && it != "Chat" }
        ?: state.workspaceFolder?.trimEnd('/')?.substringAfterLast('/')?.takeIf { it.isNotBlank() }
        ?: "Chat"

    val title = when {
        state.title.isNotBlank() && state.title != "会话详情".loc && state.title != "未命名会话".loc && !state.title.startsWith("会话 ".loc) ->
            state.title
        wsName != "Chat" -> wsName
        else -> "新对话".loc
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
    runningFallbackPollJob?.cancel()
    runningFallbackPollJob = null
    wsJob?.cancel()
    wsJob = null
    wsClient.disconnect()
    pendingOptimisticMessageId = null
    awaitingStartedAtMs = 0L
    hasObservedServerRunningForAwaiting = false
    pendingOptimisticQueueItems.clear()
    deletedQueueTombstones.clear()
    inFlightDeletingQueueIds.clear()
    stoppedTaskKeys.clear()
    currentDraftProject = null
    currentLastModifiedTime = null
    hasUserManuallySelectedModel = false
    _inputText.value = ""
    _uiState.value = ChatUiState(activeModel = favoriteModel1)
}

private fun ChatViewModel.buildStateFromCachedSession(
    cascadeId: String,
    cached: CachedChatSession,
    initialTitle: String?,
    defaultTitle: String,
    resolvedWs: String,
    isDraft: Boolean,
    attachments: List<AttachmentImage>
): ChatUiState {
    val healed = sanitizeMessageOrder(cached.messages)
    if (healed.size != cached.messages.size) {
        cacheManager?.saveSession(cached.copy(messages = healed))
    }
    val hasEarliest = healed.any { (it.stepIndex ?: extractStepIndex(it.id)) == 0 }
    val hasMore = if (hasEarliest) false else cached.hasMore
    val nextOffset = if (hasEarliest) 0 else cached.nextOffset
    val isRunning = cached.status.equals("CASCADE_RUN_STATUS_RUNNING", ignoreCase = true) || cached.status.equals("RUNNING", ignoreCase = true)
    val resolvedTitle = cached.title?.takeIf { it.isNotBlank() && it != "未命名会话".loc && it != "会话详情".loc }
        ?: initialTitle?.takeIf { it.isNotBlank() }
        ?: defaultTitle

    return ChatUiState(
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
        activeModel = cached.activeModel?.takeIf { it.isNotBlank() } ?: if (!isDraft) {
            val desktop = prefs?.cachedModelCatalog?.desktopActiveModel?.takeIf { it.isNotBlank() }
            ModelDefaults.resolveActive(desktop, favoriteModel1, favoriteModel1, favoriteModel2)
        } else {
            favoriteModel1
        },
        activeModelName = if (!cached.activeModel.isNullOrBlank()) {
            cached.activeModelName ?: _uiState.value.activeModelName
        } else if (!isDraft && !prefs?.cachedModelCatalog?.desktopActiveModel.isNullOrBlank()) {
            prefs?.cachedModelCatalog?.desktopActiveModelName ?: _uiState.value.activeModelName
        } else {
            cached.activeModelName ?: _uiState.value.activeModelName
        },
        startedAt = cached.startedAt ?: _uiState.value.startedAt,
        errorMessage = null,
        isLatestMessageError = cached.status.equals("CASCADE_RUN_STATUS_ERROR", ignoreCase = true) || cached.status.equals("ERROR", ignoreCase = true),
        hasMore = hasMore,
        nextOffset = nextOffset,
        stepCount = cached.stepCount,
        totalTools = cached.totalTools,
        duration = cached.duration,
        cascadeConfigRaw = cached.cascadeConfigRaw
    )
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
    com.antigravity.mobile.data.service.ApiClient.currentCascadeId = cascadeId
    this.currentLastModifiedTime = lastModifiedTime
    this.isUnreadOnEntry = isUnread
    this.initialConversationStatus = conversationStatus

    fetchJob?.cancel()
    fetchJob = null
    runningFallbackPollJob?.cancel()
    runningFallbackPollJob = null
    wsJob?.cancel()
    wsJob = null
    wsClient.disconnect()
    pendingOptimisticMessageId = null
    awaitingStartedAtMs = 0L
    hasObservedServerRunningForAwaiting = false
    pendingOptimisticQueueItems.clear()
    deletedQueueTombstones.clear()
    inFlightDeletingQueueIds.clear()
    stoppedTaskKeys.clear()
    hasUserManuallySelectedModel = false

    val isDraft = cascadeId.isBlank() || cascadeId.startsWith("local_draft_") || cascadeId.startsWith("draft_")
    val resolvedDraftProject = draftProject ?: (if (isDraft) prefs?.getLocalDraftSession(cascadeId)?.project else null)
    this.currentDraftProject = resolvedDraftProject

    val resolvedWs = workspaceName?.takeIf { it.isNotBlank() }
        ?: (if (resolvedDraftProject?.isPureChat == true) "Chat" else resolvedDraftProject?.name.orEmpty())
    val defaultTitle = if (isDraft) (if (resolvedWs.isNotBlank() && resolvedWs != "Chat") resolvedWs else "新对话".loc) else "会话 $cascadeId".loc

    // Restore draft text and warm memory images immediately (zero disk I/O on Main)
    val savedDraftText = prefs?.getDraftText(cascadeId).orEmpty()
    _inputText.value = savedDraftText

    val memDraftImages = prefs?.getMemoryDraftImages(cascadeId)
    val attachments = if (!memDraftImages.isNullOrEmpty()) {
        memDraftImages.map { bytes ->
            AttachmentImage(
                uri = Uri.EMPTY,
                bitmap = null,
                byteArray = bytes
            )
        }
    } else emptyList()

    // Restore instant memory cache if warm; hydrate cold disk session/images on Dispatchers.IO
    val memCached = if (!isDraft) cacheManager?.getMemoryCachedSession(cascadeId) else null
    if (memCached != null) {
        _uiState.value = buildStateFromCachedSession(
            cascadeId = cascadeId,
            cached = memCached,
            initialTitle = initialTitle,
            defaultTitle = defaultTitle,
            resolvedWs = resolvedWs,
            isDraft = isDraft,
            attachments = attachments
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
            activeModel = if (!isDraft) {
                val desktop = prefs?.cachedModelCatalog?.desktopActiveModel?.takeIf { it.isNotBlank() }
                ModelDefaults.resolveActive(desktop, favoriteModel1, favoriteModel1, favoriteModel2)
            } else {
                favoriteModel1
            },
            activeModelName = if (!isDraft && !prefs?.cachedModelCatalog?.desktopActiveModel.isNullOrBlank()) {
                prefs?.cachedModelCatalog?.desktopActiveModelName
            } else null,
            errorMessage = null,
            isLatestMessageError = false,
            markdownViewerData = null
        )
    }

    val needsColdSessionLoad = !isDraft && !isNewConversation && memCached == null
    val needsColdDraftImages = memDraftImages == null && cascadeId.isNotBlank()
    if (needsColdSessionLoad || needsColdDraftImages) {
        viewModelScope.launch {
            val (coldSession, coldDraftImgs) = withContext(Dispatchers.IO) {
                val s = if (needsColdSessionLoad) cacheManager?.loadSession(cascadeId) else null
                val imgs = if (needsColdDraftImages) prefs?.loadDraftImages(cascadeId).orEmpty() else emptyList()
                s to imgs
            }
            if (_uiState.value.cascadeId != cascadeId) return@launch
            val resolvedAttachments = if (coldDraftImgs.isNotEmpty() && _uiState.value.selectedImages.isEmpty()) {
                coldDraftImgs.map { bytes ->
                    AttachmentImage(uri = Uri.EMPTY, bitmap = null, byteArray = bytes)
                }
            } else {
                _uiState.value.selectedImages
            }
            if (coldSession != null && _uiState.value.messages.isEmpty()) {
                _uiState.value = buildStateFromCachedSession(
                    cascadeId = cascadeId,
                    cached = coldSession,
                    initialTitle = initialTitle,
                    defaultTitle = defaultTitle,
                    resolvedWs = resolvedWs,
                    isDraft = isDraft,
                    attachments = resolvedAttachments
                )
            } else if (resolvedAttachments !== _uiState.value.selectedImages) {
                _uiState.value = _uiState.value.copy(selectedImages = resolvedAttachments)
            }
        }
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
    val isDraft = cascadeId.isBlank() || cascadeId.startsWith("local_draft_") || cascadeId.startsWith("draft_")
    val resolvedDraftProject = draftProject ?: this.currentDraftProject ?: (if (isDraft) prefs?.getLocalDraftSession(cascadeId)?.project else null)
    this.currentDraftProject = resolvedDraftProject

    val shouldLoad = !isNewConversation && !isDraft
    val fallbackWs = (if (resolvedDraftProject?.isPureChat == true) "Chat" else resolvedDraftProject?.name) ?: _uiState.value.workspaceName
    val resolvedWs = workspaceName?.takeIf { it.isNotBlank() } ?: fallbackWs
    val defaultTitle = if (isDraft) (if (resolvedWs.isNotBlank() && resolvedWs != "Chat") resolvedWs else "新对话".loc) else "会话 $cascadeId".loc

    if (isDifferentSession) {
        fetchJob?.cancel()
        fetchJob = null
        wsClient.disconnect()
        pendingOptimisticQueueItems.clear()
        deletedQueueTombstones.clear()
        inFlightDeletingQueueIds.clear()
        stoppedTaskKeys.clear()
        hasUserManuallySelectedModel = false

        val savedDraftText = prefs?.getDraftText(cascadeId).orEmpty()
        _inputText.value = savedDraftText

        val memDraftImages = prefs?.getMemoryDraftImages(cascadeId)
        val attachments = if (!memDraftImages.isNullOrEmpty()) {
            memDraftImages.map { bytes ->
                AttachmentImage(
                    uri = Uri.EMPTY,
                    bitmap = null,
                    byteArray = bytes
                )
            }
        } else emptyList()

        val memCached = if (!isDraft) cacheManager?.getMemoryCachedSession(cascadeId) else null
        if (memCached != null) {
            _uiState.value = buildStateFromCachedSession(
                cascadeId = cascadeId,
                cached = memCached,
                initialTitle = initialTitle,
                defaultTitle = defaultTitle,
                resolvedWs = resolvedWs,
                isDraft = isDraft,
                attachments = attachments
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
                activeModel = if (!isDraft) {
                    val desktop = prefs?.cachedModelCatalog?.desktopActiveModel?.takeIf { it.isNotBlank() }
                    ModelDefaults.resolveActive(desktop, favoriteModel1, favoriteModel1, favoriteModel2)
                } else {
                    favoriteModel1
                },
                activeModelName = if (!isDraft && !prefs?.cachedModelCatalog?.desktopActiveModel.isNullOrBlank()) {
                    prefs?.cachedModelCatalog?.desktopActiveModelName
                } else null,
                errorMessage = null,
                isLatestMessageError = false,
                markdownViewerData = null
            )
        }
    } else {
        val desktop = if (!isDraft) prefs?.cachedModelCatalog?.desktopActiveModel?.takeIf { it.isNotBlank() } else null
        val syncedModel = if (!hasUserManuallySelectedModel && desktop != null) {
            ModelDefaults.resolveActive(desktop, _uiState.value.activeModel, favoriteModel1, favoriteModel2)
        } else {
            _uiState.value.activeModel
        }
        val syncedName = if (!hasUserManuallySelectedModel && desktop != null) {
            prefs?.cachedModelCatalog?.desktopActiveModelName ?: _uiState.value.activeModelName
        } else {
            _uiState.value.activeModelName
        }
        _uiState.value = _uiState.value.copy(
            title = initialTitle?.takeIf { it.isNotBlank() } ?: _uiState.value.title,
            workspaceName = if (resolvedWs.isNotBlank()) resolvedWs else _uiState.value.workspaceName,
            isNewConversation = isNewConversation,
            isLoading = if (_uiState.value.messages.isEmpty() && !isNewConversation && !isDraft) true else false,
            activeModel = syncedModel,
            activeModelName = syncedName
        )
    }

    restoreDraftFiles(cascadeId)

    // Restore draft text synchronously (SharedPreferences memory map) and draft images off Main thread
    prefs?.let { p ->
        val draftText = p.getDraftText(cascadeId)
        if (draftText.isNotBlank() && _inputText.value.isBlank()) {
            _inputText.value = draftText
        }
        val memDraftImgs = p.getMemoryDraftImages(cascadeId)
        if (memDraftImgs != null) {
            if (memDraftImgs.isNotEmpty() && _uiState.value.selectedImages.isEmpty()) {
                _uiState.value = _uiState.value.copy(
                    selectedImages = memDraftImgs.map { bytes ->
                        AttachmentImage(uri = Uri.EMPTY, bitmap = null, byteArray = bytes)
                    }
                )
            }
        } else if (cascadeId.isNotBlank()) {
            viewModelScope.launch {
                val diskImgs = withContext(Dispatchers.IO) { p.loadDraftImages(cascadeId) }
                if (_uiState.value.cascadeId == cascadeId && diskImgs.isNotEmpty() && _uiState.value.selectedImages.isEmpty()) {
                    _uiState.value = _uiState.value.copy(
                        selectedImages = diskImgs.map { bytes ->
                            AttachmentImage(uri = Uri.EMPTY, bitmap = null, byteArray = bytes)
                        }
                    )
                }
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

    // Hydrate cold disk cache on Dispatchers.IO first, then fetch latest messages via HTTP
    fetchJob = viewModelScope.launch {
        if (!isNewConversation && _uiState.value.messages.isEmpty()) {
            val coldCached = withContext(Dispatchers.IO) { cacheManager?.loadSession(cascadeId) }
            if (coldCached != null && _uiState.value.cascadeId == cascadeId && _uiState.value.messages.isEmpty()) {
                _uiState.value = buildStateFromCachedSession(
                    cascadeId = cascadeId,
                    cached = coldCached,
                    initialTitle = initialTitle,
                    defaultTitle = defaultTitle,
                    resolvedWs = resolvedWs,
                    isDraft = isDraft,
                    attachments = _uiState.value.selectedImages
                )
            }
        }
        if (!isNewConversation && _uiState.value.messages.isEmpty()) {
            _uiState.value = _uiState.value.copy(isLoading = true, errorMessage = null)
        }
        apiClient.fetchMessages(cascadeId, limit = 15)
            .onSuccess { payload ->
                if (_uiState.value.cascadeId == cascadeId) {
                    prefs?.updateDesktopActiveModel(payload.desktopActiveModel, payload.desktopActiveModelName)
                    val incoming = payload.messages ?: emptyList()
                    val mergedMsgs = if (_uiState.value.messages.isNotEmpty()) {
                        mergeIncomingMessages(incoming)
                    } else {
                        sanitizeMessageOrder(incoming)
                    }

                    val lastMsg = mergedMsgs.lastOrNull()
                    val isError = lastMsg?.status.equals("error", ignoreCase = true) ||
                        payload.hasError ||
                        LiveActivityStatusHelper.isStatusError(payload.status)
                    val wsFromPayload = payload.workspaceUri?.trimEnd('/')?.substringAfterLast('/')?.takeIf { it.isNotBlank() }

                    val hasEarliest = mergedMsgs.any { (it.stepIndex ?: extractStepIndex(it.id)) == 0 }
                    val hasMore = if (hasEarliest) false else payload.hasMore
                    val nextOffset = if (hasEarliest) 0 else payload.nextOffset

                    val resolvedTitle = payload.title?.takeIf { it.isNotBlank() && it != "未命名会话".loc && it != "会话详情".loc }
                        ?: _uiState.value.title

                    val statusRunning = isStatusRunning(payload.status)
                    val awaiting = resolveAwaitingForPayload(mergedMsgs, statusRunning, isError)
                    val isRunning = statusRunning || awaiting
                    val wasRunning = _uiState.value.isRunning
                    val activeRunningTasks = filterRunningTasks(payload.runningTasks)
                    _uiState.value = _uiState.value.copy(
                        isLoading = false,
                        title = resolvedTitle,
                        workspaceName = wsFromPayload ?: _uiState.value.workspaceName,
                        messages = mergedMsgs,
                        runningTasks = activeRunningTasks,
                        subagents = payload.subagents.orEmpty(),
                        swarmBrief = payload.swarm ?: _uiState.value.swarmBrief,
                        parentConversationId = payload.parentConversationId?.takeIf { it.isNotBlank() },
                        subagentRole = payload.subagentRole,
                        queuedMessages = syncQueuedMessages(payload.queuedMessages, mergedMsgs),
                        isRunning = isRunning,
                        isAwaitingResponse = awaiting,
                        canProceed = payload.canProceed,
                        proceedArtifactUri = payload.proceedArtifactUri,
                        pendingInteraction = payload.pendingInteraction,
                        activeModel = if (hasUserManuallySelectedModel) _uiState.value.activeModel else {
                            val serverModel = payload.activeModel?.takeIf { it.isNotBlank() }
                                ?: (if (!isDraft) payload.desktopActiveModel?.takeIf { it.isNotBlank() } else null)
                            ModelDefaults.resolveActive(serverModel, _uiState.value.activeModel, favoriteModel1, favoriteModel2)
                        },
                        activeModelName = if (hasUserManuallySelectedModel) _uiState.value.activeModelName else {
                            val serverName = if (!payload.activeModel.isNullOrBlank()) {
                                payload.activeModelName
                            } else if (!isDraft) {
                                payload.desktopActiveModelName
                            } else {
                                payload.activeModelName
                            }
                            serverName ?: _uiState.value.activeModelName
                        },
                        startedAt = payload.startedAt ?: _uiState.value.startedAt,
                        errorMessage = if (payload.hasError && _uiState.value.messages.isEmpty()) payload.errorMessage else null,
                        isLatestMessageError = isError,
                        hasMore = hasMore,
                        nextOffset = nextOffset,
                        stepCount = payload.totalSteps,
                        totalTools = payload.totalTools,
                        duration = payload.duration ?: _uiState.value.duration,
                        cascadeConfigRaw = payload.cascadeConfigRaw ?: _uiState.value.cascadeConfigRaw,
                        contextMeter = payload.contextMeter ?: _uiState.value.contextMeter,
                        suggestedResponses = if (!isRunning) payload.suggestedResponses.orEmpty() else emptyList()
                    )

                    if (isRunning) {
                        val stepCount = mergedMsgs.count { !it.isUser }
                        val latestAction = when {
                            payload.pendingInteraction != null -> payload.pendingInteraction.prompt ?: "需要审批操作".loc
                            activeRunningTasks.hasBlockingTasks -> activeRunningTasks.blockingTasks.firstOrNull()?.displayCommand?.ifBlank { "正在执行后台任务...".loc } ?: "正在执行后台任务...".loc
                            lastMsg?.toolCalls?.isNotEmpty() == true -> "正在执行: ".loc + com.antigravity.mobile.ui.util.ToolLocalization.localizedName(lastMsg.toolCalls.lastOrNull()?.name ?: "操作".loc)
                            else -> "正在执行任务...".loc
                        }
                        liveActivityManager?.startOrUpdateActivity(
                            title = resolvedTitle,
                            cascadeId = cascadeId,
                            status = payload.status.ifBlank { "RUNNING" },
                            stepCount = maxOf(1, stepCount),
                            latestAction = latestAction,
                            runningTaskCount = activeRunningTasks.blockingTasks.size,
                            hasPendingAction = payload.pendingInteraction != null,
                            isOptimisticStart = !statusRunning
                        )
                        ensureRunningFallbackPolling()
                    } else if (wasRunning || liveActivityManager?.hasNotification(cascadeId) == true) {
                        val finalStatus = LiveActivityStatusHelper.resolveFinalStatus(isError, payload.canProceed)
                        liveActivityManager?.endActivity(cascadeId = cascadeId, finalStatus = finalStatus, title = resolvedTitle)
                        stopRunningFallbackPolling()
                    } else {
                        stopRunningFallbackPolling()
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
                        errorMessage = if (_uiState.value.messages.isEmpty()) (error.localizedMessage ?: "同步会话历史失败".loc) else null
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

internal fun ChatViewModel.resolveAwaitingForPayload(
    msgs: List<GatewayMessageItem>,
    statusRunning: Boolean,
    isError: Boolean
): Boolean {
    if (statusRunning) {
        hasObservedServerRunningForAwaiting = true
    }
    val wasAwaiting = _uiState.value.isAwaitingResponse
    if (!wasAwaiting) return false

    val lastUserIdx = msgs.indexOfLast { it.isUser }
    val hasSubsequentNonUser = if (lastUserIdx >= 0) {
        msgs.subList(lastUserIdx + 1, msgs.size).any { !it.isUser }
    } else {
        false
    }
    val hasPendingOptimistic = pendingOptimisticMessageId != null &&
        msgs.any { it.id == pendingOptimisticMessageId }
    val elapsedMs = if (awaitingStartedAtMs > 0L) {
        (System.currentTimeMillis() - awaitingStartedAtMs).coerceAtLeast(0L)
    } else {
        LiveActivityStatusHelper.DEFAULT_AWAITING_TIMEOUT_MS
    }

    val stillAwaiting = LiveActivityStatusHelper.resolveAwaitingResponse(
        wasAwaiting = true,
        statusRunning = statusRunning,
        isError = isError,
        hasObservedServerRunning = hasObservedServerRunningForAwaiting,
        hasSubsequentNonUserMessage = hasSubsequentNonUser,
        hasPendingOptimisticMessage = hasPendingOptimistic,
        awaitingElapsedMs = elapsedMs
    )
    if (!stillAwaiting) {
        awaitingStartedAtMs = 0L
    }
    return stillAwaiting
}

internal fun ChatViewModel.handleAppBackgrounded() {
    isAppInBackground = true
    backgroundDisconnectJob?.cancel()
    val state = _uiState.value
    val graceMs = if (state.isRunning || state.isAwaitingResponse) 25_000L else 8_000L
    backgroundDisconnectJob = viewModelScope.launch {
        delay(graceMs)
        if (isAppInBackground) {
            wsClient.disconnect(intentional = true)
            stopRunningFallbackPolling()
        }
    }
}

internal fun ChatViewModel.handleAppForegrounded() {
    isAppInBackground = false
    backgroundDisconnectJob?.cancel()
    backgroundDisconnectJob = null
    val activeCid = _uiState.value.cascadeId
    if (activeCid.isNotBlank() && !activeCid.startsWith("local_draft_") && !activeCid.startsWith("draft_")) {
        reconnectWebSocketIfDisconnected()
        ensureRunningFallbackPolling()
        refresh()
    }
}

internal fun ChatViewModel.ensureRunningFallbackPolling() {
    if (isAppInBackground) return
    val cid = _uiState.value.cascadeId
    if (cid.isBlank() || cid.startsWith("local_draft_") || cid.startsWith("draft_")) return
    if (runningFallbackPollJob?.isActive == true) return

    runningFallbackPollJob = viewModelScope.launch {
        while (isActive && !isAppInBackground) {
            val state = _uiState.value
            val currentCid = state.cascadeId
            if (currentCid.isBlank() || currentCid.startsWith("local_draft_") || currentCid.startsWith("draft_")) {
                break
            }
            val shouldPoll = state.isRunning ||
                state.isAwaitingResponse ||
                (liveActivityManager?.hasNotification(currentCid) == true)
            if (!shouldPoll) {
                break
            }
            val wsConnected = wsClient.connectionStatus.value == ConnectionStatus.CONNECTED
            delay(if (wsConnected) 10_000L else 3_500L)
            if (isAppInBackground) break
            val afterDelayState = _uiState.value
            if (afterDelayState.cascadeId != currentCid) break
            val stillShouldPoll = afterDelayState.isRunning ||
                afterDelayState.isAwaitingResponse ||
                (liveActivityManager?.hasNotification(currentCid) == true)
            if (!stillShouldPoll) break
            // Suspend redundant HTTP fetchMessages polling while the WebSocket stream is actively delivering frames.
            if (wsClient.hasRecentFrame(withinMs = 20_000L)) {
                continue
            }
            if (!_isRefreshing.value) {
                refresh()
            }
        }
        runningFallbackPollJob = null
    }
}

internal fun ChatViewModel.stopRunningFallbackPolling() {
    runningFallbackPollJob?.cancel()
    runningFallbackPollJob = null
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
                prefs?.updateDesktopActiveModel(payload.desktopActiveModel, payload.desktopActiveModelName)
                val isCurrentDraft = _uiState.value.cascadeId.let {
                    it.isBlank() || it.startsWith("local_draft_") || it.startsWith("draft_")
                }

                val rawMsgs = payload.messages
                val msgs = if (rawMsgs != null) {
                    mergeIncomingMessages(rawMsgs)
                } else {
                    _uiState.value.messages
                }
                val lastMsg = msgs.lastOrNull()
                val isError = lastMsg?.status.equals("error", ignoreCase = true) ||
                    payload.hasError ||
                    LiveActivityStatusHelper.isStatusError(payload.status)

                val statusRunning = isStatusRunning(payload.status)
                val awaiting = resolveAwaitingForPayload(msgs, statusRunning, isError)
                val isRunning = statusRunning || awaiting
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
                    subagents = payload.subagents.orEmpty(),
                    swarmBrief = payload.swarm ?: (if (payload.isFullSnapshot) null else _uiState.value.swarmBrief),
                    parentConversationId = payload.parentConversationId?.takeIf { it.isNotBlank() },
                    subagentRole = payload.subagentRole,
                    queuedMessages = syncQueuedMessages(payload.queuedMessages, msgs),
                    isRunning = isRunning,
                    isAwaitingResponse = awaiting,
                    canProceed = payload.canProceed,
                    proceedArtifactUri = payload.proceedArtifactUri,
                    pendingInteraction = payload.pendingInteraction,
                    activeModel = if (hasUserManuallySelectedModel) _uiState.value.activeModel else {
                        val serverModel = payload.activeModel?.takeIf { it.isNotBlank() }
                            ?: (if (!isCurrentDraft) payload.desktopActiveModel?.takeIf { it.isNotBlank() } else null)
                        ModelDefaults.resolveActive(serverModel, _uiState.value.activeModel, favoriteModel1, favoriteModel2)
                    },
                    activeModelName = if (hasUserManuallySelectedModel) _uiState.value.activeModelName else {
                        val serverName = if (!payload.activeModel.isNullOrBlank()) {
                            payload.activeModelName
                        } else if (!isCurrentDraft) {
                            payload.desktopActiveModelName
                        } else {
                            payload.activeModelName
                        }
                        serverName ?: _uiState.value.activeModelName
                    },
                    startedAt = payload.startedAt ?: _uiState.value.startedAt,
                    errorMessage = if (payload.hasError) payload.errorMessage else null,
                    isLatestMessageError = isError,
                    hasMore = hasMore,
                    nextOffset = nextOffset,
                    stepCount = if (payload.totalSteps > 0) payload.totalSteps else _uiState.value.stepCount,
                    totalTools = if (payload.totalTools > 0) payload.totalTools else _uiState.value.totalTools,
                    duration = payload.duration ?: _uiState.value.duration,
                    cascadeConfigRaw = payload.cascadeConfigRaw ?: _uiState.value.cascadeConfigRaw,
                    contextMeter = payload.contextMeter ?: _uiState.value.contextMeter,
                    suggestedResponses = if (!isRunning) payload.suggestedResponses.orEmpty() else emptyList()
                )

                if (isRunning) {
                    val stepCount = msgs.count { !it.isUser }
                    val latestAction = when {
                        payload.pendingInteraction != null -> payload.pendingInteraction.prompt ?: "需要审批操作".loc
                        activeRunningTasks.hasBlockingTasks -> activeRunningTasks.blockingTasks.firstOrNull()?.displayCommand?.ifBlank { "正在执行后台任务...".loc } ?: "正在执行后台任务...".loc
                        lastMsg?.toolCalls?.isNotEmpty() == true -> "正在执行: ".loc + com.antigravity.mobile.ui.util.ToolLocalization.localizedName(lastMsg.toolCalls.lastOrNull()?.name ?: "操作".loc)
                        awaiting || lastMsg?.isUser == true -> "正在思考并组织回复...".loc
                        else -> "正在执行任务...".loc
                    }
                    liveActivityManager?.startOrUpdateActivity(
                        title = _uiState.value.title,
                        cascadeId = _uiState.value.cascadeId,
                        status = payload.status.ifBlank { "RUNNING" },
                        stepCount = maxOf(1, stepCount),
                        latestAction = latestAction,
                        runningTaskCount = activeRunningTasks.blockingTasks.size,
                        hasPendingAction = payload.pendingInteraction != null,
                        isOptimisticStart = !statusRunning
                    )
                    ensureRunningFallbackPolling()
                } else if (wasRunning || liveActivityManager?.hasNotification(_uiState.value.cascadeId) == true) {
                    val finalStatus = LiveActivityStatusHelper.resolveFinalStatus(isError, payload.canProceed)
                    liveActivityManager?.endActivity(
                        cascadeId = _uiState.value.cascadeId,
                        finalStatus = finalStatus,
                        title = _uiState.value.title
                    )
                    stopRunningFallbackPolling()
                } else {
                    stopRunningFallbackPolling()
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
    if (cid.isBlank() || cid.startsWith("local_draft_") || cid.startsWith("draft_")) {
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
                        val isError = lastMsg?.status.equals("error", ignoreCase = true) ||
                            payload.hasError ||
                            LiveActivityStatusHelper.isStatusError(payload.status)
                        val wsFromPayload = payload.workspaceUri?.trimEnd('/')?.substringAfterLast('/')?.takeIf { it.isNotBlank() }

                        val hasEarliest = msgs.any { (it.stepIndex ?: extractStepIndex(it.id)) == 0 }
                        val hasMore = if (hasEarliest) false else payload.hasMore
                        val nextOffset = if (hasEarliest) 0 else payload.nextOffset

                        val resolvedTitle = payload.title?.takeIf { it.isNotBlank() && it != "未命名会话".loc && it != "会话详情".loc }
                            ?: _uiState.value.title
                        val statusRunning = isStatusRunning(payload.status)
                        val awaiting = resolveAwaitingForPayload(msgs, statusRunning, isError)
                        val isRunning = statusRunning || awaiting
                        val wasRunning = _uiState.value.isRunning
                        val activeRunningTasks = filterRunningTasks(payload.runningTasks)

                        _uiState.value = _uiState.value.copy(
                            isLoading = false,
                            title = resolvedTitle,
                            workspaceName = wsFromPayload ?: _uiState.value.workspaceName,
                            messages = msgs,
                            runningTasks = activeRunningTasks,
                            subagents = payload.subagents.orEmpty(),
                            parentConversationId = payload.parentConversationId?.takeIf { it.isNotBlank() },
                            subagentRole = payload.subagentRole,
                            queuedMessages = syncQueuedMessages(payload.queuedMessages, msgs),
                            isRunning = isRunning,
                            isAwaitingResponse = awaiting,
                            canProceed = payload.canProceed,
                            proceedArtifactUri = payload.proceedArtifactUri,
                            pendingInteraction = payload.pendingInteraction,
                            isLatestMessageError = isError,
                            hasMore = hasMore,
                            nextOffset = nextOffset,
                            stepCount = payload.totalSteps,
                            totalTools = payload.totalTools,
                            duration = payload.duration ?: _uiState.value.duration,
                            cascadeConfigRaw = payload.cascadeConfigRaw ?: _uiState.value.cascadeConfigRaw,
                            suggestedResponses = if (!isRunning) payload.suggestedResponses.orEmpty() else emptyList()
                        )
                        if (isRunning) {
                            val stepCount = msgs.count { !it.isUser }
                            val latestAction = when {
                                payload.pendingInteraction != null -> payload.pendingInteraction.prompt ?: "需要审批操作".loc
                                activeRunningTasks.hasBlockingTasks -> activeRunningTasks.blockingTasks.firstOrNull()?.displayCommand?.ifBlank { "正在执行后台任务...".loc } ?: "正在执行后台任务...".loc
                                lastMsg?.toolCalls?.isNotEmpty() == true -> "正在执行: ".loc + com.antigravity.mobile.ui.util.ToolLocalization.localizedName(lastMsg.toolCalls.lastOrNull()?.name ?: "操作".loc)
                                awaiting || lastMsg?.isUser == true -> "正在思考并组织回复...".loc
                                else -> "正在执行任务...".loc
                            }
                            liveActivityManager?.startOrUpdateActivity(
                                title = resolvedTitle,
                                cascadeId = cid,
                                status = payload.status.ifBlank { "RUNNING" },
                                stepCount = maxOf(1, stepCount),
                                latestAction = latestAction,
                                runningTaskCount = activeRunningTasks.blockingTasks.size,
                                hasPendingAction = payload.pendingInteraction != null,
                                isOptimisticStart = !statusRunning
                            )
                            ensureRunningFallbackPolling()
                        } else if (wasRunning || liveActivityManager?.hasNotification(cid) == true) {
                            val finalStatus = LiveActivityStatusHelper.resolveFinalStatus(isError, payload.canProceed)
                            liveActivityManager?.endActivity(cascadeId = cid, finalStatus = finalStatus, title = resolvedTitle)
                            stopRunningFallbackPolling()
                        } else {
                            stopRunningFallbackPolling()
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
                    prefs?.updateDesktopActiveModel(payload.desktopActiveModel, payload.desktopActiveModelName)
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
