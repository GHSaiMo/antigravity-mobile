package com.antigravity.mobile.ui.viewmodel

import com.antigravity.mobile.ui.util.loc
import android.content.Context
import android.graphics.Bitmap
import android.graphics.BitmapFactory
import android.net.Uri
import android.util.Base64
import android.util.Log
import androidx.lifecycle.viewModelScope
import com.antigravity.mobile.data.model.*
import com.antigravity.mobile.data.service.AttachmentRules
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.delay
import java.io.ByteArrayOutputStream
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale
import java.util.TimeZone
import java.util.UUID
import kotlin.math.roundToInt

internal fun ChatViewModel.decodeBase64ToAttachment(raw: String): AttachmentImage? {
    return try {
        val cleaned = if (raw.contains(",")) raw.substringAfter(",") else raw
        val bytes = Base64.decode(cleaned, Base64.DEFAULT)
        if (bytes.isEmpty()) return null
        AttachmentImage(
            uri = Uri.EMPTY,
            bitmap = null,
            byteArray = bytes
        )
    } catch (_: Exception) {
        null
    }
}

internal fun ChatViewModel.compressAndResizeImage(bytes: ByteArray, maxDim: Int = 2048): ByteArray? {
    return try {
        val boundsOptions = BitmapFactory.Options().apply { inJustDecodeBounds = true }
        BitmapFactory.decodeByteArray(bytes, 0, bytes.size, boundsOptions)
        val width = boundsOptions.outWidth
        val height = boundsOptions.outHeight
        if (width <= 0 || height <= 0) return null

        var sampleSize = 1
        val largest = maxOf(width, height)
        if (largest > maxDim) {
            while ((largest / (sampleSize * 2)) >= maxDim) {
                sampleSize *= 2
            }
        }

        val decodeOptions = BitmapFactory.Options().apply {
            inSampleSize = sampleSize
            inPreferredConfig = Bitmap.Config.ARGB_8888
        }
        val decodedBitmap = BitmapFactory.decodeByteArray(bytes, 0, bytes.size, decodeOptions) ?: return null

        val currentLargest = maxOf(decodedBitmap.width, decodedBitmap.height)
        val finalBitmap = if (currentLargest > maxDim) {
            val ratio = maxDim.toFloat() / currentLargest
            val targetWidth = (decodedBitmap.width * ratio).roundToInt()
            val targetHeight = (decodedBitmap.height * ratio).roundToInt()
            Bitmap.createScaledBitmap(decodedBitmap, targetWidth, targetHeight, true)
        } else {
            decodedBitmap
        }

        val outputStream = ByteArrayOutputStream()
        finalBitmap.compress(Bitmap.CompressFormat.JPEG, 85, outputStream)
        val compressedBytes = outputStream.toByteArray()
        if (finalBitmap != decodedBitmap) {
            finalBitmap.recycle()
        }
        decodedBitmap.recycle()
        compressedBytes
    } catch (_: Exception) {
        null
    }
}

internal fun ChatViewModel.addImagesFromUris(context: Context, uris: List<Uri>) {
    viewModelScope.launch(Dispatchers.IO) {
        val newAttachments = mutableListOf<AttachmentImage>()
        for (uri in uris) {
            try {
                val bytes = context.contentResolver.openInputStream(uri)?.use { it.readBytes() } ?: continue
                val processed = compressAndResizeImage(bytes)
                if (processed != null) {
                    newAttachments.add(
                        AttachmentImage(
                            uri = uri,
                            bitmap = null,
                            byteArray = processed,
                            mimeType = "image/jpeg"
                        )
                    )
                } else {
                    newAttachments.add(
                        AttachmentImage(
                            uri = uri,
                            bitmap = null,
                            byteArray = bytes,
                            mimeType = context.contentResolver.getType(uri) ?: "image/jpeg"
                        )
                    )
                }
            } catch (e: Exception) {
                // Ignore unreadable image
            }
        }
        if (newAttachments.isNotEmpty()) {
            val updatedImages = _uiState.value.selectedImages + newAttachments
            _uiState.value = _uiState.value.copy(
                selectedImages = updatedImages
            )
            val cid = _uiState.value.cascadeId
            if (cid.isNotBlank()) {
                prefs?.saveDraftImages(cid, updatedImages.map { it.byteArray })
                if (cid.startsWith("local_draft_")) {
                    val existing = prefs?.getLocalDraftSession(cid)
                    val project = currentDraftProject ?: existing?.project
                    if (project != null) {
                        val session = existing ?: LocalDraftSession(id = cid, project = project)
                        session.draftText = _inputText.value
                        session.updatedAtEpochMs = System.currentTimeMillis()
                        prefs?.saveLocalDraftSession(session)
                    }
                }
            }
        }
    }
}

internal fun ChatViewModel.removeImage(index: Int) {
    val current = _uiState.value.selectedImages.toMutableList()
    if (index in current.indices) {
        current.removeAt(index)
        _uiState.value = _uiState.value.copy(selectedImages = current)
        val cid = _uiState.value.cascadeId
        if (cid.isNotBlank()) {
            prefs?.saveDraftImages(cid, current.map { it.byteArray })
            if (cid.startsWith("local_draft_")) {
                val existing = prefs?.getLocalDraftSession(cid)
                val project = currentDraftProject ?: existing?.project
                if (project != null) {
                    val session = existing ?: LocalDraftSession(id = cid, project = project)
                    session.draftText = _inputText.value
                    session.updatedAtEpochMs = System.currentTimeMillis()
                    prefs?.saveLocalDraftSession(session)
                }
            }
        }
    }
}

internal fun ChatViewModel.saveDraftFor(targetCid: String, text: String, imageBytes: List<ByteArray>? = null) {
    if (targetCid.isBlank()) return
    prefs?.let { p ->
        p.setDraftText(targetCid, text)
        if (imageBytes != null) {
            p.saveDraftImages(targetCid, imageBytes)
        }
        if (targetCid.startsWith("local_draft_")) {
            val session = p.getLocalDraftSession(targetCid)
            if (session != null) {
                session.draftText = text
                session.updatedAtEpochMs = System.currentTimeMillis()
                p.saveLocalDraftSession(session)
            } else {
                val project = currentDraftProject
                if (project != null) {
                    val newSession = LocalDraftSession(
                        id = targetCid,
                        project = project,
                        draftText = text,
                        createdAtEpochMs = System.currentTimeMillis(),
                        updatedAtEpochMs = System.currentTimeMillis()
                    )
                    p.saveLocalDraftSession(newSession)
                }
            }
        }
    }
    if (!targetCid.startsWith("local_draft_") && targetCid == _uiState.value.cascadeId) {
        saveSessionToCache()
    }
}

internal fun ChatViewModel.sendMessage(
    text: String,
    attachments: List<AttachmentImage> = emptyList(),
    forceImmediate: Boolean = false,
    files: List<AttachmentFile> = emptyList(),
    slashCommands: List<String> = emptyList(),
    slashOptions: List<SlashCommandOption> = emptyList()
) {
    val cascadeId = _uiState.value.cascadeId
    // The gateway appends the attachment block to the text; mirror it locally so optimistic
    // bubbles and queued items match what the server will echo back.
    val bodyText = AttachmentRules.appendBlock(text, files)
    val cleanSlashes = slashCommands.map { it.trim() }.filter { it.isNotEmpty() }
    // language_server 把斜杠命令的显示文本生成为 "/name1 /name2 内容"，本地气泡与队列条目保持一致
    val displayText = SlashCommandFilter.displayText(cleanSlashes, bodyText)
    val attachmentIds = files.mapNotNull { it.attachmentId }
    val ctxItems = _uiState.value.selectedContextItems
    if (ctxItems.isNotEmpty() || _uiState.value.suggestedResponses.isNotEmpty()) {
        _uiState.value = _uiState.value.copy(
            selectedContextItems = emptyList(),
            suggestedResponses = emptyList(),
            suggestedResponseSource = "",
            dismissedPredictedSuggestion = null
        )
    }
    val model = _uiState.value.activeModel

    val isRunningOrAwaiting = _uiState.value.isRunning || _uiState.value.isAwaitingResponse
    val canQueue = !forceImmediate && isRunningOrAwaiting && cascadeId.isNotBlank() && !cascadeId.startsWith("local_draft_")

    if (canQueue) {
        val trimmed = displayText.trim()
        if (trimmed.isNotEmpty()) {
            val norm = normalizeForComparison(trimmed)
            deletedQueueTombstones.removeAll { normalizeForComparison(it.text) == norm }
        }
        val mediaBase64 = attachments.map { att ->
            Base64.encodeToString(att.byteArray, Base64.NO_WRAP)
        }.takeIf { it.isNotEmpty() }

        // 同一个 uuid 既做本地乐观条目的 id，也随消息发给网关（写进 tags），之后在服务端队列里按它精确对应
        val queueClientMsgId = UUID.randomUUID().toString()
        val queueItem = QueuedMessageItem(
            id = "queue-$queueClientMsgId",
            text = displayText,
            media = mediaBase64,
            clientMessageId = queueClientMsgId
        )
        val lastUserMsgId = _uiState.value.messages.lastOrNull { it.isUser }?.id
        pendingOptimisticQueueItems.add(
            PendingOptimisticQueueItem(
                id = queueItem.id,
                text = displayText,
                media = mediaBase64,
                imageUrls = null,
                createdAt = System.currentTimeMillis(),
                enqueuedAfterMessageId = lastUserMsgId,
                clientMessageId = queueClientMsgId
            )
        )
        _uiState.value = _uiState.value.copy(
            queuedMessages = syncQueuedMessages(_uiState.value.queuedMessages)
        )
        _scrollToBottomTrigger.value++
        saveSessionToCache()
        notifyConversationUpdated()

        val imagePayloads = attachments.map { Pair(it.byteArray, it.mimeType) }
        val targetCascadeId = cascadeId
        viewModelScope.launch {
            suspend fun attemptSend(): Result<Unit> = runCatching {
                apiClient.sendMessage(
                    cascadeId = targetCascadeId,
                    text = text,
                    model = model,
                    images = imagePayloads,
                    deliveryStrategy = 2,
                    clientMessageId = queueClientMsgId,
                    attachmentIds = attachmentIds,
                    slashCommands = cleanSlashes,
                    contextItems = ctxItems
                )
            }.fold(
                onSuccess = { it },
                onFailure = { Result.failure(it) }
            )

            fun isTimeoutOrDispatched(msg: String): Boolean {
                val lower = msg.lowercase()
                return msg.contains("超时") || lower.contains("timed out") || lower.contains("timeout") || msg.contains("送达服务器")
            }

            var result = attemptSend()
            if (result.isFailure) {
                val errMsg = result.exceptionOrNull()?.message.orEmpty()
                if (!isTimeoutOrDispatched(errMsg)) {
                    delay(800L)
                    result = attemptSend()
                }
            }

            result.onFailure { err ->
                val errMsg = err.message.orEmpty()
                Log.w("ChatViewModel", "Failed to deliver queued message upstream: $errMsg", err)
                if (isTimeoutOrDispatched(errMsg)) {
                    // Server may have received the queued message; reconcile after a short delay without duplicating draft.
                    delay(2000L)
                    if (_uiState.value.cascadeId == targetCascadeId) {
                        apiClient.fetchMessages(targetCascadeId, limit = 15).onSuccess { payload ->
                            if (_uiState.value.cascadeId == targetCascadeId) {
                                val msgs = payload.messages?.let { mergeIncomingMessages(it) } ?: _uiState.value.messages
                                _uiState.value = _uiState.value.copy(
                                    queuedMessages = syncQueuedMessages(payload.queuedMessages, msgs)
                                )
                            }
                        }
                    }
                    return@onFailure
                }

                // Hard delivery failure: remove unconfirmed optimistic card & restore unsent text into input/draft.
                pendingOptimisticQueueItems.removeAll { it.id == queueItem.id }
                if (_uiState.value.cascadeId == targetCascadeId) {
                    val remainingQueue = _uiState.value.queuedMessages.filterNot { it.id == queueItem.id }
                    val restoredText = if (text.isNotBlank()) {
                        val existing = _inputText.value
                        if (existing.isBlank()) text else "$existing\n$text"
                    } else {
                        _inputText.value
                    }
                    if (text.isNotBlank()) {
                        _inputText.value = restoredText
                        prefs?.setDraftText(targetCascadeId, restoredText)
                    }
                    val restoredCtx = if (_uiState.value.selectedContextItems.isEmpty() && ctxItems.isNotEmpty()) {
                        ctxItems
                    } else {
                        _uiState.value.selectedContextItems
                    }
                    val restoredSlashes = if (_uiState.value.selectedSlashCommands.isEmpty() && slashOptions.isNotEmpty()) {
                        slashOptions
                    } else {
                        _uiState.value.selectedSlashCommands
                    }
                    _uiState.value = _uiState.value.copy(
                        queuedMessages = remainingQueue,
                        selectedContextItems = restoredCtx,
                        selectedSlashCommands = restoredSlashes,
                        errorMessage = "排队消息发送失败，已恢复到输入框".loc
                    )
                    saveSessionToCache()
                    notifyConversationUpdated()
                } else if (text.isNotBlank()) {
                    val saved = prefs?.getDraftText(targetCascadeId).orEmpty()
                    val merged = if (saved.isBlank()) text else "$saved\n$text"
                    prefs?.setDraftText(targetCascadeId, merged)
                }
            }
        }
        return
    }

    val imageBytesList = attachments.map { it.byteArray }

    // Snapshot known server message IDs & max step before appending the optimistic user bubble
    val confirmedMessages = _uiState.value.messages.filterNot {
        it.id == pendingOptimisticMessageId || it.id.startsWith("opt_") || it.id.startsWith("optimistic-")
    }
    knownServerMessageIds = confirmedMessages.map { it.id }.toSet()
    preSendMaxStepIndex = maxStepIndexOf(confirmedMessages)
    lastLocalSendAtMs = System.currentTimeMillis()

    // Optimistically add user bubble
    val optId = "opt_${System.currentTimeMillis()}"
    pendingOptimisticMessageId = optId
    val optSlashMentions = if (cleanSlashes.isNotEmpty()) {
        cleanSlashes.map { cmdName ->
            val matched = slashOptions.firstOrNull { it.name == cmdName }
                ?: _uiState.value.slashCommands.firstOrNull { it.name == cmdName }
            val kind = matched?.kind ?: if (com.antigravity.mobile.data.model.SlashCommandIconCatalog.defaultSystemIcons.containsKey(cmdName)) "system" else "skill"
            val icon = com.antigravity.mobile.data.model.SlashCommandIconCatalog.resolveIcon(cmdName, matched?.icon, kind)
            com.antigravity.mobile.data.model.SlashCommandMention(name = cmdName, icon = icon, kind = kind, definitionPath = matched?.definitionPath)
        }
    } else null
    val optimisticUserMsg = GatewayMessageItem(
        id = optId,
        type = "user",
        role = "user",
        text = displayText,
        content = displayText,
        imageDataList = imageBytesList,
        slashCommands = optSlashMentions,
        clientMessageId = optId
    )
    awaitingStartedAtMs = System.currentTimeMillis()
    hasObservedServerRunningForAwaiting = false
    val baselineStepCount = _uiState.value.stepCount
    _uiState.value = _uiState.value.copy(
        messages = _uiState.value.messages + optimisticUserMsg,
        isRunning = true,
        isAwaitingResponse = true,
        isLatestMessageError = false
    )
    _scrollToBottomTrigger.value++
    notifyConversationUpdated()
    saveSessionToCache()

    if (cascadeId.startsWith("local_draft_")) {
        val draftSession = prefs?.getLocalDraftSession(cascadeId)
        val project = currentDraftProject ?: draftSession?.project ?: ProjectItem.PURE_CHAT
        val isPure = project.isPureChat
        val pid = if (isPure) "outside-of-project" else (project.rawId ?: (if (project.id != project.uri) project.id else null))
        val wsUri = if (isPure) "" else project.uri
        val initialPrompt = if (attachments.isEmpty() && files.isEmpty() && cleanSlashes.isEmpty() && ctxItems.isEmpty()) text else ""

        viewModelScope.launch {
            val createRes = apiClient.createCascade(
                workspaceUri = wsUri,
                prompt = initialPrompt,
                model = model,
                projectId = pid
            )
            createRes.onSuccess { newCascadeId ->
                val oldDraftId = cascadeId
                currentDraftProject = null
                prefs?.let { p ->
                    p.deleteLocalDraftSession(oldDraftId)
                    p.clearDraftText(oldDraftId)
                    p.clearDraftImages(oldDraftId)
                    p.clearDraftFiles(oldDraftId)
                    p.clearDraftText(newCascadeId)
                    p.clearDraftImages(newCascadeId)
                }

                val title = if (isPure) "新对话".loc else project.name
                val wsName = if (isPure) "Chat" else project.name
                _uiState.value = _uiState.value.copy(
                    cascadeId = newCascadeId,
                    title = title,
                    workspaceName = wsName,
                    isNewConversation = false
                )

                val nowIso = SimpleDateFormat("yyyy-MM-dd'T'HH:mm:ss'Z'", Locale.US).apply {
                    timeZone = TimeZone.getTimeZone("UTC")
                }.format(Date())

                val newConv = ConversationItem(
                    id = newCascadeId,
                    title = title,
                    status = ConversationStatus.RUNNING,
                    stepCount = 1,
                    workspaceName = wsName,
                    lastModifiedTime = nowIso,
                    isSubagent = false,
                    isUnread = false
                )
                onConversationUpdated?.invoke(newConv)

                liveActivityManager?.startOrUpdateActivity(
                    title = title,
                    cascadeId = newCascadeId,
                    status = "RUNNING",
                    stepCount = 1,
                    latestAction = "开始执行任务...".loc,
                    isOptimisticStart = true,
                    baselineStepCount = 0
                )

                wsClient.connect(newCascadeId)
                ensureWebSocketObserving()
                ensureRunningFallbackPolling()

                if (attachments.isNotEmpty() || files.isNotEmpty() || cleanSlashes.isNotEmpty() || ctxItems.isNotEmpty()) {
                    val imagePayloads = attachments.map { Pair(it.byteArray, it.mimeType) }
                    val sendResult = apiClient.sendMessage(
                        newCascadeId, text, model, imagePayloads,
                        attachmentIds = attachmentIds,
                        slashCommands = cleanSlashes,
                        contextItems = ctxItems
                    )
                    sendResult.onFailure { err ->
                        awaitingStartedAtMs = 0L
                        lastLocalSendAtMs = 0L
                        hasObservedServerRunningForAwaiting = false
                        stopRunningFallbackPolling()
                        _uiState.value = _uiState.value.copy(
                            errorMessage = "发送失败: %s".loc(err.message.orEmpty()),
                            isRunning = false,
                            isAwaitingResponse = false
                        )
                        liveActivityManager?.endActivity(cascadeId = newCascadeId, finalStatus = "FAILED", title = title)
                        notifyConversationUpdated()
                        saveSessionToCache()
                    }
                }

                delay(250)
                apiClient.fetchMessages(newCascadeId, limit = 15).onSuccess { payload ->
                    if (_uiState.value.cascadeId == newCascadeId) {
                        val incoming = payload.messages ?: emptyList()
                        val msgs = if (incoming.isNotEmpty()) {
                            mergeIncomingMessages(incoming)
                        } else {
                            _uiState.value.messages
                        }
                        val lastMsg = msgs.lastOrNull()
                        val isError = lastMsg?.status.equals("error", ignoreCase = true) ||
                            payload.hasError ||
                            com.antigravity.mobile.data.service.LiveActivityStatusHelper.isStatusError(payload.status)
                        val statusRunning = isStatusRunning(payload.status)
                        val awaiting = resolveAwaitingForPayload(msgs, statusRunning, isError)
                        val isRunning = statusRunning || awaiting
                        val resolvedTitle = payload.title?.takeIf { it.isNotBlank() } ?: _uiState.value.title
                        _uiState.value = _uiState.value.copy(
                            title = resolvedTitle,
                            workspaceName = payload.workspaceUri?.trimEnd('/')?.substringAfterLast('/')?.takeIf { it.isNotBlank() } ?: _uiState.value.workspaceName,
                            messages = msgs,
                            runningTasks = filterRunningTasks(payload.runningTasks),
                            subagents = payload.subagents.orEmpty(),
                            parentConversationId = payload.parentConversationId?.takeIf { it.isNotBlank() },
                            subagentRole = payload.subagentRole,
                            queuedMessages = syncQueuedMessages(payload.queuedMessages, msgs),
                            isRunning = isRunning,
                            isAwaitingResponse = awaiting,
                            canProceed = payload.canProceed,
                            proceedArtifactUri = payload.proceedArtifactUri,
                            pendingInteraction = payload.pendingInteraction,
                            errorMessage = if (payload.hasError) payload.errorMessage else null,
                            isLatestMessageError = isError,
                            hasMore = payload.hasMore,
                            nextOffset = payload.nextOffset,
                            stepCount = payload.totalSteps,
                            totalTools = payload.totalTools,
                            duration = payload.duration ?: _uiState.value.duration,
                            contextMeter = payload.contextMeter ?: _uiState.value.contextMeter
                        )
                        if (!isRunning && liveActivityManager?.hasNotification(newCascadeId) == true) {
                            val finalStatus = com.antigravity.mobile.data.service.LiveActivityStatusHelper.resolveFinalStatus(
                                isError = isError,
                                canProceed = payload.canProceed
                            )
                            liveActivityManager?.endActivity(cascadeId = newCascadeId, finalStatus = finalStatus, title = resolvedTitle)
                            stopRunningFallbackPolling()
                        }
                        _scrollToBottomTrigger.value++
                        notifyConversationUpdated()
                        saveSessionToCache()
                    }
                }
            }.onFailure { err ->
                awaitingStartedAtMs = 0L
                lastLocalSendAtMs = 0L
                hasObservedServerRunningForAwaiting = false
                stopRunningFallbackPolling()
                _uiState.value = _uiState.value.copy(
                    errorMessage = "创建会话失败: %s".loc(err.message.orEmpty()),
                    isRunning = false,
                    isAwaitingResponse = false
                )
                liveActivityManager?.endActivity(cascadeId = cascadeId, finalStatus = "FAILED")
                notifyConversationUpdated()
            }
        }
        return
    }

    liveActivityManager?.startOrUpdateActivity(
        title = _uiState.value.title,
        cascadeId = cascadeId,
        status = "RUNNING",
        stepCount = maxOf(1, _uiState.value.messages.count { !it.isUser } + 1),
        latestAction = "开始执行任务...".loc,
        isOptimisticStart = true,
        baselineStepCount = baselineStepCount
    )
    ensureRunningFallbackPolling()

    val imagePayloads = attachments.map { Pair(it.byteArray, it.mimeType) }
    val deliveryStrategy = if (forceImmediate) 1 else null

    viewModelScope.launch {
        val result = apiClient.sendMessage(
            cascadeId = cascadeId,
            text = text,
            model = model,
            images = imagePayloads,
            deliveryStrategy = deliveryStrategy,
            clientMessageId = optId,
            attachmentIds = attachmentIds,
            slashCommands = cleanSlashes,
            contextItems = ctxItems
        )
        result.onFailure { err ->
            awaitingStartedAtMs = 0L
            lastLocalSendAtMs = 0L
            hasObservedServerRunningForAwaiting = false
            stopRunningFallbackPolling()
            _uiState.value = _uiState.value.copy(
                errorMessage = "发送失败: %s".loc(err.message.orEmpty()),
                isRunning = false,
                isAwaitingResponse = false
            )
            liveActivityManager?.endActivity(cascadeId = cascadeId, finalStatus = "FAILED", title = _uiState.value.title)
            notifyConversationUpdated()
            saveSessionToCache()
        }
    }
}

internal fun ChatViewModel.cancelExecution() {
    val cascadeId = _uiState.value.cascadeId
    // 只停止阻塞回合的任务；常驻守护任务（IsDaemon=true）保持运行并留在浮层中供手动停止。
    val hasDaemon = _uiState.value.runningTasks.any { it.isDaemon }
    val tasksToStop = _uiState.value.runningTasks.blockingTasks
    tasksToStop.forEach { task ->
        stoppedTaskKeys.add("${task.id}:${task.stepIndex}")
        if (task.id.isNotBlank()) stoppedTaskKeys.add(task.id)
    }
    awaitingStartedAtMs = 0L
    hasObservedServerRunningForAwaiting = false
    stopRunningFallbackPolling()
    _uiState.value = _uiState.value.copy(
        isRunning = false,
        isAwaitingResponse = false,
        runningTasks = _uiState.value.runningTasks.filter { !it.blocksTurn }
    )
    liveActivityManager?.endActivity(cascadeId = cascadeId, finalStatus = "CANCELLED", title = _uiState.value.title)
    notifyConversationUpdated()
    saveSessionToCache()
    viewModelScope.launch {
        apiClient.cancelInvocation(cascadeId, killBackgroundTasks = !hasDaemon)
        tasksToStop.forEach { task ->
            apiClient.stopTask(cascadeId, task.id, task.stepIndex)
        }
    }
}

/** 关停一个子代理：先乐观标成「已结束」，失败则恢复并提示；成功后以网关随后推送的状态为准。 */
internal fun ChatViewModel.stopSubagent(conversationId: String) {
    val previous = _uiState.value.subagents
    _uiState.value = _uiState.value.copy(
        subagents = previous.map { if (it.conversationId == conversationId) it.copy(status = "done") else it }
    )
    viewModelScope.launch {
        val result = apiClient.stopSubagent(conversationId)
        if (result.isFailure) {
            Log.e("ChatViewModel", "stopSubagent failed for $conversationId: ${result.exceptionOrNull()?.message}")
            _uiState.value = _uiState.value.copy(
                subagents = previous,
                attachmentNotice = "关停子代理失败，请稍后重试".loc
            )
        }
    }
}

internal fun ChatViewModel.stopTask(task: RunningTaskItem) {
    val cascadeId = _uiState.value.cascadeId
    val taskKey = "${task.id}:${task.stepIndex}"
    stoppedTaskKeys.add(taskKey)
    if (task.id.isNotBlank()) {
        stoppedTaskKeys.add(task.id)
    }
    // 乐观从 UI 移除该任务，带来即时反馈（对齐 iOS 与 Web）
    _uiState.value = _uiState.value.copy(
        runningTasks = _uiState.value.runningTasks.filterNot {
            (it.id == task.id && it.stepIndex == task.stepIndex) ||
            (task.id.isNotBlank() && it.id == task.id)
        }
    )
    viewModelScope.launch {
        val result = apiClient.stopTask(cascadeId, task.id, task.stepIndex)
        if (result.isFailure) {
            Log.e("ChatViewModel", "stopTask failed for task ${task.id} (step ${task.stepIndex}): ${result.exceptionOrNull()?.message}")
        }
    }
}

internal fun ChatViewModel.ensureCascadeForVoiceCall(onReady: (String, String?, String?) -> Unit) {
    val currentCid = _uiState.value.cascadeId
    val baseUrl = prefs?.getEffectiveGatewayUrl(false, null) ?: prefs?.gatewayBaseUrl
    val token = prefs?.deviceToken

    if (currentCid.isNotBlank() && !currentCid.startsWith("local_draft_")) {
        onReady(currentCid, baseUrl, token)
        return
    }

    val draftSession = prefs?.getLocalDraftSession(currentCid)
    val project = currentDraftProject ?: draftSession?.project ?: ProjectItem.PURE_CHAT
    val isPure = project.isPureChat
    val pid = if (isPure) "outside-of-project" else (project.rawId ?: (if (project.id != project.uri) project.id else null))
    val wsUri = if (isPure) "" else project.uri

    viewModelScope.launch {
        val createRes = apiClient.createCascade(
            workspaceUri = wsUri,
            prompt = "",
            model = _uiState.value.activeModel,
            projectId = pid
        )
        createRes.onSuccess { newCascadeId ->
            bindVoiceCascadeId(newCascadeId, project)
            onReady(newCascadeId, baseUrl, token)
        }.onFailure { err ->
            _uiState.value = _uiState.value.copy(
                errorMessage = "无法启动实时语音会话：%s".loc(err.message ?: "网络异常".loc)
            )
        }
    }
}

internal fun ChatViewModel.bindVoiceCascadeId(newCascadeId: String, projectOverride: ProjectItem? = null) {
    if (newCascadeId.isBlank() || _uiState.value.cascadeId == newCascadeId) return
    val oldDraftId = _uiState.value.cascadeId
    val draftSession = prefs?.getLocalDraftSession(oldDraftId)
    val project = projectOverride ?: currentDraftProject ?: draftSession?.project ?: ProjectItem.PURE_CHAT
    val isPure = project.isPureChat
    currentDraftProject = null
    prefs?.let { p ->
        p.deleteLocalDraftSession(oldDraftId)
        p.clearDraftText(oldDraftId)
        p.clearDraftImages(oldDraftId)
        p.clearDraftFiles(oldDraftId)
    }
    val title = if (isPure) "新对话".loc else project.name
    val wsName = if (isPure) "Chat" else project.name
    _uiState.value = _uiState.value.copy(
        cascadeId = newCascadeId,
        title = title,
        workspaceName = wsName,
        isNewConversation = false
    )
    wsClient.connect(newCascadeId)
    ensureWebSocketObserving()
}
