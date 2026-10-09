package com.antigravity.mobile.ui.viewmodel

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
    slashCommand: String? = null
) {
    val cascadeId = _uiState.value.cascadeId
    // The gateway appends the attachment block to the text; mirror it locally so optimistic
    // bubbles and queued items match what the server will echo back.
    val bodyText = AttachmentRules.appendBlock(text, files)
    // language_server 把斜杠命令的显示文本生成为 "/name 内容"，本地气泡与队列条目保持一致
    val displayText = if (slashCommand.isNullOrBlank()) bodyText else "/$slashCommand" + if (bodyText.isNotEmpty()) " $bodyText" else ""
    val attachmentIds = files.mapNotNull { it.attachmentId }
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

        val queueItem = QueuedMessageItem(
            id = "queue-${UUID.randomUUID()}",
            text = displayText,
            media = mediaBase64
        )
        val lastUserMsgId = _uiState.value.messages.lastOrNull { it.isUser }?.id
        pendingOptimisticQueueItems.add(
            PendingOptimisticQueueItem(
                id = queueItem.id,
                text = displayText,
                media = mediaBase64,
                imageUrls = null,
                createdAt = System.currentTimeMillis(),
                enqueuedAfterMessageId = lastUserMsgId
            )
        )
        _uiState.value = _uiState.value.copy(
            queuedMessages = syncQueuedMessages(_uiState.value.queuedMessages)
        )
        _scrollToBottomTrigger.value++
        saveSessionToCache()
        notifyConversationUpdated()

        val queueClientMsgId = UUID.randomUUID().toString()
        val imagePayloads = attachments.map { Pair(it.byteArray, it.mimeType) }
        viewModelScope.launch {
            try {
                apiClient.sendMessage(
                    cascadeId = cascadeId,
                    text = text,
                    model = model,
                    images = imagePayloads,
                    deliveryStrategy = 2,
                    clientMessageId = queueClientMsgId,
                    attachmentIds = attachmentIds,
                    slashCommand = slashCommand
                )
            } catch (e: Exception) {
                Log.w("ChatViewModel", "Failed to deliver queued message upstream", e)
            }
        }
        return
    }

    val imageBytesList = attachments.map { it.byteArray }

    // Optimistically add user bubble
    val optId = "opt_${System.currentTimeMillis()}"
    pendingOptimisticMessageId = optId
    val optimisticUserMsg = GatewayMessageItem(
        id = optId,
        type = "user",
        role = "user",
        text = displayText,
        content = displayText,
        imageDataList = imageBytesList
    )
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
        val initialPrompt = if (attachments.isEmpty() && files.isEmpty() && slashCommand.isNullOrBlank()) text else ""

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

                val title = if (isPure) "新对话" else project.name
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
                    latestAction = "开始执行任务..."
                )

                wsClient.connect(newCascadeId)
                ensureWebSocketObserving()

                if (attachments.isNotEmpty() || files.isNotEmpty() || !slashCommand.isNullOrBlank()) {
                    val imagePayloads = attachments.map { Pair(it.byteArray, it.mimeType) }
                    val sendResult = apiClient.sendMessage(
                        newCascadeId, text, model, imagePayloads,
                        attachmentIds = attachmentIds,
                        slashCommand = slashCommand
                    )
                    sendResult.onFailure { err ->
                        _uiState.value = _uiState.value.copy(
                            errorMessage = "发送失败: ${err.message}",
                            isRunning = false,
                            isAwaitingResponse = false
                        )
                        liveActivityManager?.endActivity(cascadeId = newCascadeId, finalStatus = "FAILED")
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
                        val isError = lastMsg?.status.equals("error", ignoreCase = true) || payload.hasError
                        _uiState.value = _uiState.value.copy(
                            title = payload.title?.takeIf { it.isNotBlank() } ?: _uiState.value.title,
                            workspaceName = payload.workspaceUri?.trimEnd('/')?.substringAfterLast('/')?.takeIf { it.isNotBlank() } ?: _uiState.value.workspaceName,
                            messages = msgs,
                            runningTasks = filterRunningTasks(payload.runningTasks),
                            queuedMessages = syncQueuedMessages(payload.queuedMessages, msgs),
                            isRunning = isStatusRunning(payload.status),
                            canProceed = payload.canProceed,
                            proceedArtifactUri = payload.proceedArtifactUri,
                            pendingInteraction = payload.pendingInteraction,
                            errorMessage = if (payload.hasError) payload.errorMessage else null,
                            isLatestMessageError = isError,
                            hasMore = payload.hasMore,
                            nextOffset = payload.nextOffset,
                            stepCount = payload.totalSteps,
                            totalTools = payload.totalTools,
                            duration = payload.duration ?: _uiState.value.duration
                        )
                        _scrollToBottomTrigger.value++
                        notifyConversationUpdated()
                        saveSessionToCache()
                    }
                }
            }.onFailure { err ->
                _uiState.value = _uiState.value.copy(
                    errorMessage = "创建会话失败: ${err.message}",
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
        latestAction = "开始执行任务..."
    )

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
            slashCommand = slashCommand
        )
        result.onFailure { err ->
            _uiState.value = _uiState.value.copy(
                errorMessage = "发送失败: ${err.message}",
                isRunning = false,
                isAwaitingResponse = false
            )
            liveActivityManager?.endActivity(cascadeId = cascadeId, finalStatus = "FAILED")
            notifyConversationUpdated()
            saveSessionToCache()
        }
    }
}

internal fun ChatViewModel.cancelExecution() {
    val cascadeId = _uiState.value.cascadeId
    val tasksToStop = _uiState.value.runningTasks
    tasksToStop.forEach { task ->
        stoppedTaskKeys.add("${task.id}:${task.stepIndex}")
        if (task.id.isNotBlank()) stoppedTaskKeys.add(task.id)
    }
    _uiState.value = _uiState.value.copy(isRunning = false, isAwaitingResponse = false, runningTasks = emptyList())
    liveActivityManager?.endActivity(cascadeId = cascadeId, finalStatus = "CANCELLED")
    notifyConversationUpdated()
    saveSessionToCache()
    viewModelScope.launch {
        apiClient.cancelInvocation(cascadeId)
        tasksToStop.forEach { task ->
            apiClient.stopTask(cascadeId, task.id, task.stepIndex)
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
