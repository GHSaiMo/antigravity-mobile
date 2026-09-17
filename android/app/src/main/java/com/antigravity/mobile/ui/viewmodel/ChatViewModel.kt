package com.antigravity.mobile.ui.viewmodel

import android.content.Context
import android.graphics.Bitmap
import android.graphics.BitmapFactory
import android.net.Uri
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.antigravity.mobile.data.model.*
import com.antigravity.mobile.data.service.ApiClient
import com.antigravity.mobile.data.service.ConnectionStatus
import com.antigravity.mobile.data.service.DocumentCacheManager
import com.antigravity.mobile.data.service.LiveActivityNotificationManager
import com.antigravity.mobile.data.service.PreferencesManager
import com.antigravity.mobile.data.service.StreamWebSocketClient
import com.antigravity.mobile.ui.components.ImageViewerData
import com.antigravity.mobile.ui.components.ImageViewerItem
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import kotlinx.coroutines.delay
import java.io.ByteArrayOutputStream
import java.net.URLDecoder
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale
import java.util.TimeZone
import java.util.UUID
import kotlin.math.roundToInt

data class AttachmentImage(
    val id: String = UUID.randomUUID().toString(),
    val uri: Uri,
    val bitmap: Bitmap,
    val byteArray: ByteArray,
    val mimeType: String = "image/jpeg"
) {
    override fun equals(other: Any?): Boolean {
        if (this === other) return true
        if (javaClass != other?.javaClass) return false
        other as AttachmentImage
        return id == other.id
    }

    override fun hashCode(): Int = id.hashCode()
}

data class ChatUiState(
    val cascadeId: String = "",
    val title: String = "会话详情",
    val workspaceName: String = "",
    val workspaceFolder: String? = null,
    val messages: List<GatewayMessageItem> = emptyList(),
    val runningTasks: List<RunningTaskItem> = emptyList(),
    val queuedMessages: List<QueuedMessageItem> = emptyList(),
    val isLoading: Boolean = false,
    val isNewConversation: Boolean = false,
    val isRunning: Boolean = false,
    val isAwaitingResponse: Boolean = false,
    val selectedImages: List<AttachmentImage> = emptyList(),
    val canProceed: Boolean = false,
    val proceedArtifactUri: String? = null,
    val pendingInteraction: PendingInteraction? = null,
    val activeModel: String = "gemini-3.8-flash-high",
    val connectionStatus: ConnectionStatus = ConnectionStatus.DISCONNECTED,
    val errorMessage: String? = null,
    val isLatestMessageError: Boolean = false,
    val markdownViewerData: MarkdownFileViewerData? = null,
    val imageViewerData: ImageViewerData? = null,
    val isDownloadingDocument: Boolean = false,
    val downloadingDocumentName: String = "",
    val downloadProgress: Float = 0f,
    val previewDocumentFile: java.io.File? = null,
    val previewDocumentTitle: String = ""
)

class ChatViewModel(
    private val apiClient: ApiClient,
    private val wsClient: StreamWebSocketClient,
    private val prefs: PreferencesManager? = null,
    private val documentCacheManager: DocumentCacheManager? = null,
    private val liveActivityManager: LiveActivityNotificationManager? = null
) : ViewModel() {

    private val _uiState = MutableStateFlow(ChatUiState())
    val uiState: StateFlow<ChatUiState> = _uiState.asStateFlow()

    private val _inputText = MutableStateFlow("")
    val inputText: StateFlow<String> = _inputText.asStateFlow()

    private val _scrollToBottomTrigger = MutableStateFlow(0)
    val scrollToBottomTrigger: StateFlow<Int> = _scrollToBottomTrigger.asStateFlow()

    private val _isRefreshing = MutableStateFlow(false)
    val isRefreshing: StateFlow<Boolean> = _isRefreshing.asStateFlow()

    var onConversationUpdated: ((ConversationItem) -> Unit)? = null

    private var wsJob: Job? = null
    private var fetchJob: Job? = null

    fun currentConversationItem(): ConversationItem? {
        val state = _uiState.value
        if (state.cascadeId.isBlank()) return null
        if (state.cascadeId.startsWith("local_draft_")) {
            val draft = prefs?.getLocalDraftSession(state.cascadeId)
            val draftText = prefs?.getDraftText(state.cascadeId).orEmpty().ifBlank { _inputText.value }
            val hasImages = prefs?.hasDraftImages(state.cascadeId) == true || state.selectedImages.isNotEmpty()
            if (draftText.isBlank() && !hasImages) return null
            return draft?.copy(draftText = draftText)?.toConversationItem(hasImages)
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
        val nowIso = SimpleDateFormat("yyyy-MM-dd'T'HH:mm:ss'Z'", Locale.US).apply {
            timeZone = TimeZone.getTimeZone("UTC")
        }.format(Date())

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
            lastModifiedTime = nowIso,
            isSubagent = false,
            isUnread = false
        )
    }

    private fun notifyConversationUpdated() {
        currentConversationItem()?.let { item ->
            onConversationUpdated?.invoke(item)
        }
    }

    fun resetSession() {
        fetchJob?.cancel()
        fetchJob = null
        wsJob?.cancel()
        wsJob = null
        wsClient.disconnect()
        _inputText.value = ""
        _uiState.value = ChatUiState()
    }

    fun prepareSession(
        cascadeId: String,
        initialTitle: String? = null,
        isNewConversation: Boolean = false,
        workspaceName: String? = null
    ) {
        fetchJob?.cancel()
        fetchJob = null
        wsJob?.cancel()
        wsJob = null
        wsClient.disconnect()
        _inputText.value = ""
        val isDraft = cascadeId.startsWith("local_draft_")
        val draftProject = if (isDraft) prefs?.getLocalDraftSession(cascadeId)?.project else null
        val resolvedWs = workspaceName?.takeIf { it.isNotBlank() }
            ?: (if (draftProject?.isPureChat == true) "Chat" else draftProject?.name.orEmpty())
        val defaultTitle = if (isDraft) (if (resolvedWs.isNotBlank() && resolvedWs != "Chat") resolvedWs else "新对话") else "会话 $cascadeId"
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
            selectedImages = emptyList(),
            canProceed = false,
            proceedArtifactUri = null,
            pendingInteraction = null,
            activeModel = _uiState.value.activeModel,
            errorMessage = null,
            isLatestMessageError = false,
            markdownViewerData = null
        )
    }

    fun initSession(
        cascadeId: String,
        initialTitle: String? = null,
        isNewConversation: Boolean = false,
        workspaceName: String? = null
    ) {
        val isDifferentSession = _uiState.value.cascadeId != cascadeId
        val isDraft = cascadeId.startsWith("local_draft_")
        val shouldLoad = !isNewConversation && !isDraft
        val draftProject = if (isDraft) prefs?.getLocalDraftSession(cascadeId)?.project else null
        val fallbackWs = (if (draftProject?.isPureChat == true) "Chat" else draftProject?.name) ?: _uiState.value.workspaceName
        val resolvedWs = workspaceName?.takeIf { it.isNotBlank() } ?: fallbackWs
        val defaultTitle = if (isDraft) (if (resolvedWs.isNotBlank() && resolvedWs != "Chat") resolvedWs else "新对话") else "会话 $cascadeId"
        if (isDifferentSession) {
            fetchJob?.cancel()
            fetchJob = null
            wsClient.disconnect()
            _inputText.value = ""
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
                selectedImages = emptyList(),
                canProceed = false,
                proceedArtifactUri = null,
                pendingInteraction = null,
                activeModel = _uiState.value.activeModel,
                errorMessage = null,
                isLatestMessageError = false,
                markdownViewerData = null
            )
        } else {
            _uiState.value = _uiState.value.copy(
                title = initialTitle?.takeIf { it.isNotBlank() } ?: _uiState.value.title,
                workspaceName = if (resolvedWs.isNotBlank()) resolvedWs else _uiState.value.workspaceName,
                isNewConversation = isNewConversation,
                isLoading = if (_uiState.value.messages.isEmpty() && !isNewConversation && !isDraft) true else _uiState.value.isLoading
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
                val attachments = draftImages.mapNotNull { bytes ->
                    try {
                        val bitmap = BitmapFactory.decodeByteArray(bytes, 0, bytes.size)
                        if (bitmap != null) {
                            AttachmentImage(
                                uri = Uri.EMPTY,
                                bitmap = bitmap,
                                byteArray = bytes
                            )
                        } else null
                    } catch (_: Exception) { null }
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

        // Fetch cached messages via HTTP so entering session loads instantly
        fetchJob = viewModelScope.launch {
            if (!isNewConversation && _uiState.value.messages.isEmpty()) {
                _uiState.value = _uiState.value.copy(isLoading = true, errorMessage = null)
            }
            apiClient.fetchMessages(cascadeId, limit = 15)
                .onSuccess { payload ->
                    if (_uiState.value.cascadeId == cascadeId) {
                        val msgs = payload.messages ?: emptyList()
                        val lastMsg = msgs.lastOrNull()
                        val isError = lastMsg?.status.equals("error", ignoreCase = true) || payload.hasError
                        val wsFromPayload = payload.workspaceUri?.trimEnd('/')?.substringAfterLast('/')?.takeIf { it.isNotBlank() }
                        _uiState.value = _uiState.value.copy(
                            isLoading = false,
                            title = payload.title?.takeIf { it.isNotBlank() } ?: _uiState.value.title,
                            workspaceName = wsFromPayload ?: _uiState.value.workspaceName,
                            messages = msgs,
                            runningTasks = payload.runningTasks ?: emptyList(),
                            queuedMessages = payload.queuedMessages ?: emptyList(),
                            isRunning = payload.status.equals("RUNNING", ignoreCase = true),
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
                            isLatestMessageError = isError
                        )
                        _scrollToBottomTrigger.value++
                        notifyConversationUpdated()
                    }
                }
                .onFailure { error ->
                    if (_uiState.value.cascadeId == cascadeId) {
                        _uiState.value = _uiState.value.copy(
                            isLoading = false,
                            errorMessage = error.localizedMessage ?: "同步会话历史失败"
                        )
                    }
                }
        }

        wsClient.connect(cascadeId)
        ensureWebSocketObserving()
    }

    fun retryLoadMessages() {
        val cascadeId = _uiState.value.cascadeId
        if (cascadeId.isBlank()) return
        initSession(cascadeId, _uiState.value.title, _uiState.value.isNewConversation)
    }

    private fun ensureWebSocketObserving() {
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

                    val msgs = payload.messages ?: _uiState.value.messages
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

                    val isRunning = payload.status.equals("RUNNING", ignoreCase = true) || awaiting
                    val wasRunning = _uiState.value.isRunning
                    val previousMsgCount = _uiState.value.messages.size
                    val wsFromPayload = payload.workspaceUri?.trimEnd('/')?.substringAfterLast('/')?.takeIf { it.isNotBlank() }

                    _uiState.value = _uiState.value.copy(
                        isLoading = false,
                        title = payload.title?.takeIf { it.isNotBlank() } ?: _uiState.value.title,
                        workspaceName = wsFromPayload ?: _uiState.value.workspaceName,
                        messages = msgs,
                        runningTasks = payload.runningTasks ?: emptyList(),
                        queuedMessages = payload.queuedMessages ?: emptyList(),
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
                        isLatestMessageError = isError
                    )

                    if (isRunning) {
                        val stepCount = msgs.count { !it.isUser }
                        val latestAction = when {
                            payload.pendingInteraction != null -> payload.pendingInteraction.prompt ?: "需要审批操作"
                            !payload.runningTasks.isNullOrEmpty() -> payload.runningTasks.firstOrNull()?.command ?: "正在执行后台任务..."
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
                            runningTaskCount = payload.runningTasks?.size ?: 0,
                            hasPendingAction = payload.pendingInteraction != null
                        )
                    } else if (wasRunning) {
                        val finalStatus = if (isError) "FAILED" else "COMPLETED"
                        liveActivityManager?.endActivity(cascadeId = _uiState.value.cascadeId, finalStatus = finalStatus)
                    }

                    if (msgs.size != previousMsgCount || isRunning) {
                        _scrollToBottomTrigger.value++
                    }

                    notifyConversationUpdated()
                }
            }
        }
    }

    fun refresh(onComplete: (() -> Unit)? = null) {
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
                            val msgs = payload.messages ?: emptyList()
                            val lastMsg = msgs.lastOrNull()
                            val isError = lastMsg?.status.equals("error", ignoreCase = true) || payload.hasError
                            val wsFromPayload = payload.workspaceUri?.trimEnd('/')?.substringAfterLast('/')?.takeIf { it.isNotBlank() }
                            _uiState.value = _uiState.value.copy(
                                isLoading = false,
                                title = payload.title?.takeIf { it.isNotBlank() } ?: _uiState.value.title,
                                workspaceName = wsFromPayload ?: _uiState.value.workspaceName,
                                messages = msgs,
                                runningTasks = payload.runningTasks ?: emptyList(),
                                queuedMessages = payload.queuedMessages ?: emptyList(),
                                isRunning = payload.status.equals("RUNNING", ignoreCase = true),
                                isLatestMessageError = isError
                            )
                            notifyConversationUpdated()
                        }
                    }
            } finally {
                _isRefreshing.value = false
                onComplete?.invoke()
            }
        }
    }

    fun onInputTextChanged(text: String) {
        _inputText.value = text
    }

    private fun compressAndResizeImage(bytes: ByteArray, maxDim: Int = 2048): Pair<Bitmap, ByteArray>? {
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

            Pair(finalBitmap, compressedBytes)
        } catch (_: Exception) {
            null
        }
    }

    fun addImagesFromUris(context: Context, uris: List<Uri>) {
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
                                bitmap = processed.first,
                                byteArray = processed.second,
                                mimeType = "image/jpeg"
                            )
                        )
                    } else {
                        val bitmap = BitmapFactory.decodeByteArray(bytes, 0, bytes.size)
                        if (bitmap != null) {
                            newAttachments.add(
                                AttachmentImage(
                                    uri = uri,
                                    bitmap = bitmap,
                                    byteArray = bytes,
                                    mimeType = context.contentResolver.getType(uri) ?: "image/jpeg"
                                )
                            )
                        }
                    }
                } catch (e: Exception) {
                    // Ignore unreadable image
                }
            }
            if (newAttachments.isNotEmpty()) {
                _uiState.value = _uiState.value.copy(
                    selectedImages = _uiState.value.selectedImages + newAttachments
                )
            }
        }
    }

    fun removeImage(index: Int) {
        val current = _uiState.value.selectedImages.toMutableList()
        if (index in current.indices) {
            current.removeAt(index)
            _uiState.value = _uiState.value.copy(selectedImages = current)
        }
    }

    fun clearImages() {
        _uiState.value = _uiState.value.copy(selectedImages = emptyList())
    }

    fun toggleModel() {
        val nextModel = if (_uiState.value.activeModel.contains("claude", ignoreCase = true)) {
            "gemini-3.8-flash-high"
        } else {
            "claude-opus-4-6-thinking"
        }
        _uiState.value = _uiState.value.copy(activeModel = nextModel)
    }

    fun syncModel(model: String) {
        val lower = model.lowercase()
        val canonical = if (lower.contains("claude") || lower.contains("m26")) {
            "claude-opus-4-6-thinking"
        } else {
            "gemini-3.8-flash-high"
        }
        _uiState.value = _uiState.value.copy(activeModel = canonical)
    }

    fun insertCommitAndPush() {
        val cmd = "Commit and Push"
        val current = _inputText.value.trim()
        _inputText.value = if (current.isEmpty()) cmd else "$current\n$cmd"
    }

    fun handleContinue() {
        sendMessage("Continue")
    }

    fun sendCurrentMessage() {
        val text = _inputText.value.trim()
        val images = _uiState.value.selectedImages
        if (text.isEmpty() && images.isEmpty()) return
        val cid = _uiState.value.cascadeId
        prefs?.clearDraftText(cid)
        prefs?.clearDraftImages(cid)
        _inputText.value = ""
        _uiState.value = _uiState.value.copy(selectedImages = emptyList())
        sendMessage(text, images)
    }

    fun saveDraft() {
        val cid = _uiState.value.cascadeId
        if (cid.isBlank()) return
        prefs?.let { p ->
            p.setDraftText(cid, _inputText.value)
            p.saveDraftImages(cid, _uiState.value.selectedImages.map { it.byteArray })
            if (cid.startsWith("local_draft_")) {
                val session = p.getLocalDraftSession(cid)
                if (session != null) {
                    session.draftText = _inputText.value
                    session.updatedAtEpochMs = System.currentTimeMillis()
                    p.saveLocalDraftSession(session)
                }
            }
        }
    }

    private fun sendMessage(text: String, attachments: List<AttachmentImage> = emptyList()) {
        val cascadeId = _uiState.value.cascadeId
        val model = _uiState.value.activeModel

        val imageBytesList = attachments.map { it.byteArray }

        // Optimistically add user bubble
        val optimisticUserMsg = GatewayMessageItem(
            id = "opt_${System.currentTimeMillis()}",
            type = "user",
            role = "user",
            text = text,
            content = text,
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

        if (cascadeId.startsWith("local_draft_")) {
            val draftSession = prefs?.getLocalDraftSession(cascadeId)
            val project = draftSession?.project ?: ProjectItem.PURE_CHAT
            val isPure = project.isPureChat
            val pid = if (isPure) "outside-of-project" else project.rawId
            val wsUri = if (isPure) "" else project.uri
            val initialPrompt = if (attachments.isEmpty()) text else ""

            viewModelScope.launch {
                val createRes = apiClient.createCascade(
                    workspaceUri = wsUri,
                    prompt = initialPrompt,
                    model = model,
                    projectId = pid
                )
                createRes.onSuccess { newCascadeId ->
                    val oldDraftId = cascadeId
                    prefs?.let { p ->
                        p.deleteLocalDraftSession(oldDraftId)
                        p.clearDraftText(oldDraftId)
                        p.clearDraftImages(oldDraftId)
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

                    if (attachments.isNotEmpty()) {
                        val imagePayloads = attachments.map { Pair(it.byteArray, it.mimeType) }
                        val sendResult = apiClient.sendMessage(newCascadeId, text, model, imagePayloads)
                        sendResult.onFailure { err ->
                            _uiState.value = _uiState.value.copy(
                                errorMessage = "发送失败: ${err.message}",
                                isRunning = false,
                                isAwaitingResponse = false
                            )
                            liveActivityManager?.endActivity(cascadeId = newCascadeId, finalStatus = "FAILED")
                            notifyConversationUpdated()
                        }
                    }

                    delay(250)
                    apiClient.fetchMessages(newCascadeId, limit = 15).onSuccess { payload ->
                        if (_uiState.value.cascadeId == newCascadeId) {
                            val msgs = payload.messages ?: emptyList()
                            val lastMsg = msgs.lastOrNull()
                            val isError = lastMsg?.status.equals("error", ignoreCase = true) || payload.hasError
                            _uiState.value = _uiState.value.copy(
                                title = payload.title?.takeIf { it.isNotBlank() } ?: _uiState.value.title,
                                workspaceName = payload.workspaceUri?.trimEnd('/')?.substringAfterLast('/')?.takeIf { it.isNotBlank() } ?: _uiState.value.workspaceName,
                                messages = if (msgs.isNotEmpty()) msgs else _uiState.value.messages,
                                runningTasks = payload.runningTasks ?: emptyList(),
                                queuedMessages = payload.queuedMessages ?: emptyList(),
                                isRunning = payload.status.equals("RUNNING", ignoreCase = true),
                                canProceed = payload.canProceed,
                                proceedArtifactUri = payload.proceedArtifactUri,
                                pendingInteraction = payload.pendingInteraction,
                                errorMessage = if (payload.hasError) payload.errorMessage else null,
                                isLatestMessageError = isError
                            )
                            _scrollToBottomTrigger.value++
                            notifyConversationUpdated()
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

        viewModelScope.launch {
            val result = apiClient.sendMessage(cascadeId, text, model, imagePayloads)
            result.onFailure { err ->
                _uiState.value = _uiState.value.copy(
                    errorMessage = "发送失败: ${err.message}",
                    isRunning = false,
                    isAwaitingResponse = false
                )
                liveActivityManager?.endActivity(cascadeId = cascadeId, finalStatus = "FAILED")
                notifyConversationUpdated()
            }
        }
    }

    fun cancelExecution() {
        val cascadeId = _uiState.value.cascadeId
        _uiState.value = _uiState.value.copy(isRunning = false, isAwaitingResponse = false)
        liveActivityManager?.endActivity(cascadeId = cascadeId, finalStatus = "CANCELLED")
        notifyConversationUpdated()
        viewModelScope.launch {
            apiClient.cancelInvocation(cascadeId)
        }
    }

    fun stopTask(task: RunningTaskItem) {
        val cascadeId = _uiState.value.cascadeId
        viewModelScope.launch {
            apiClient.stopTask(cascadeId, task.id)
        }
    }

    fun sendQueuedMessageNow(item: QueuedMessageItem) {
        sendMessage(item.text)
        deleteQueuedMessage(item)
    }

    fun editQueuedMessage(item: QueuedMessageItem) {
        _inputText.value = item.text
        deleteQueuedMessage(item)
    }

    fun deleteQueuedMessage(item: QueuedMessageItem) {
        _uiState.value = _uiState.value.copy(
            queuedMessages = _uiState.value.queuedMessages.filter { it.id != item.id }
        )
    }

    fun approveInteraction() {
        val interaction = _uiState.value.pendingInteraction ?: return
        val cascadeId = _uiState.value.cascadeId
        viewModelScope.launch {
            apiClient.submitInteraction(
                cascadeId = cascadeId,
                stepIndex = interaction.stepIndex,
                responseType = interaction.type,
                confirmed = true
            )
            _uiState.value = _uiState.value.copy(pendingInteraction = null)
        }
    }

    fun rejectInteraction() {
        val interaction = _uiState.value.pendingInteraction ?: return
        val cascadeId = _uiState.value.cascadeId
        viewModelScope.launch {
            apiClient.submitInteraction(
                cascadeId = cascadeId,
                stepIndex = interaction.stepIndex,
                responseType = interaction.type,
                confirmed = false
            )
            _uiState.value = _uiState.value.copy(pendingInteraction = null)
        }
    }

    fun proceedArtifact() {
        val cascadeId = _uiState.value.cascadeId
        val uri = _uiState.value.proceedArtifactUri ?: ""
        viewModelScope.launch {
            apiClient.proceedArtifact(cascadeId, uri)
            _uiState.value = _uiState.value.copy(canProceed = false)
        }
    }

    fun openMarkdownViewer(uri: String, title: String) {
        val rawFilename = uri.substringAfterLast('/')
        val decodedFilename = try { URLDecoder.decode(rawFilename, "UTF-8") } catch (_: Exception) { rawFilename }
        val decodedTitle = try { URLDecoder.decode(title, "UTF-8") } catch (_: Exception) { title }
        val initialTitle = if (decodedTitle.isNotBlank()) decodedTitle else decodedFilename

        _uiState.value = _uiState.value.copy(
            markdownViewerData = MarkdownFileViewerData(
                uri = uri,
                title = initialTitle,
                filename = decodedFilename,
                content = "",
                summary = null,
                canProceed = _uiState.value.canProceed,
                isLoading = true
            )
        )

        viewModelScope.launch {
            val res = apiClient.fetchFileContent(uri, _uiState.value.cascadeId)
            res.onSuccess { resp ->
                val serverFilename = try { URLDecoder.decode(resp.filename, "UTF-8") } catch (_: Exception) { resp.filename }
                val resolvedFilename = serverFilename.ifBlank { decodedFilename }
                val resolvedTitle = if (initialTitle.isBlank() || initialTitle == rawFilename || initialTitle.contains("%")) {
                    resolvedFilename
                } else {
                    initialTitle
                }

                _uiState.value = _uiState.value.copy(
                    markdownViewerData = _uiState.value.markdownViewerData?.copy(
                        title = resolvedTitle,
                        filename = resolvedFilename,
                        content = resp.content,
                        summary = resp.summary,
                        isLoading = false
                    )
                )
            }.onFailure { err ->
                _uiState.value = _uiState.value.copy(
                    markdownViewerData = _uiState.value.markdownViewerData?.copy(
                        errorMessage = err.message ?: "无法加载文档内容",
                        isLoading = false
                    )
                )
            }
        }
    }

    fun closeMarkdownViewer() {
        _uiState.value = _uiState.value.copy(markdownViewerData = null)
    }

    fun proceedFromViewer() {
        closeMarkdownViewer()
        proceedArtifact()
    }

    fun openImageViewer(items: List<ImageViewerItem>, initialIndex: Int = 0) {
        val resolvedItems = items.map { item ->
            val resolvedUrl = item.url?.let { apiClient.resolveMediaURL(it) } ?: item.url
            item.copy(url = resolvedUrl)
        }
        _uiState.value = _uiState.value.copy(
            imageViewerData = ImageViewerData(items = resolvedItems, initialIndex = initialIndex)
        )
    }

    fun openImageViewer(bitmap: Bitmap? = null, url: String? = null, title: String? = null) {
        openImageViewer(
            items = listOf(ImageViewerItem(bitmap = bitmap, url = url, title = title)),
            initialIndex = 0
        )
    }

    fun openAttachmentImageViewer(attachment: AttachmentImage) {
        val selected = _uiState.value.selectedImages
        val index = selected.indexOf(attachment).coerceAtLeast(0)
        val items = if (selected.isNotEmpty()) {
            selected.map { ImageViewerItem(bitmap = it.bitmap) }
        } else {
            listOf(ImageViewerItem(bitmap = attachment.bitmap))
        }
        openImageViewer(items = items, initialIndex = index)

        viewModelScope.launch(Dispatchers.IO) {
            try {
                val options = BitmapFactory.Options().apply { inJustDecodeBounds = true }
                BitmapFactory.decodeByteArray(attachment.byteArray, 0, attachment.byteArray.size, options)
                val maxDim = maxOf(options.outWidth, options.outHeight)
                var sampleSize = 1
                if (maxDim > 2560) {
                    while ((maxDim / (sampleSize * 2)) >= 2560) {
                        sampleSize *= 2
                    }
                }
                val decodeOptions = BitmapFactory.Options().apply {
                    inSampleSize = sampleSize
                    inPreferredConfig = Bitmap.Config.ARGB_8888
                }
                val highRes = BitmapFactory.decodeByteArray(attachment.byteArray, 0, attachment.byteArray.size, decodeOptions)
                if (highRes != null) {
                    withContext(Dispatchers.Main) {
                        _uiState.value.imageViewerData?.let { current ->
                            val updatedItems = current.items.toMutableList()
                            if (index in updatedItems.indices) {
                                updatedItems[index] = updatedItems[index].copy(bitmap = highRes)
                                _uiState.value = _uiState.value.copy(
                                    imageViewerData = current.copy(items = updatedItems)
                                )
                            }
                        }
                    }
                }
            } catch (_: Exception) {
                // Keep existing bitmap preview
            }
        }
    }

    fun closeImageViewer() {
        _uiState.value = _uiState.value.copy(imageViewerData = null)
    }

    fun resolveMediaUrl(raw: String): String {
        return apiClient.resolveMediaURL(raw)
    }

    private var documentDownloadJob: Job? = null

    fun downloadAndPreviewDocument(uri: String, fileName: String) {
        documentDownloadJob?.cancel()

        val decodedFileName = try { URLDecoder.decode(fileName, "UTF-8") } catch (_: Exception) { fileName }
        val cascadeId = _uiState.value.cascadeId

        // 1. Check local cache first
        val cached = documentCacheManager?.getCachedFile(uri, decodedFileName, cascadeId)
        if (cached != null && cached.exists() && cached.length() > 0) {
            _uiState.value = _uiState.value.copy(
                previewDocumentFile = cached,
                previewDocumentTitle = decodedFileName
            )
            return
        }

        // 2. Prepare target file in cache
        val target = documentCacheManager?.cacheFile(uri, decodedFileName, cascadeId)
            ?: java.io.File.createTempFile("doc_", "_$decodedFileName")

        _uiState.value = _uiState.value.copy(
            isDownloadingDocument = true,
            downloadingDocumentName = decodedFileName,
            downloadProgress = 0f
        )

        documentDownloadJob = viewModelScope.launch {
            val res = apiClient.downloadFile(
                uri = uri,
                targetFile = target,
                cascadeId = cascadeId,
                onProgress = { progress, _, _ ->
                    _uiState.value = _uiState.value.copy(downloadProgress = progress)
                }
            )

            _uiState.value = _uiState.value.copy(isDownloadingDocument = false)

            res.onSuccess { downloadedFile ->
                _uiState.value = _uiState.value.copy(
                    previewDocumentFile = downloadedFile,
                    previewDocumentTitle = decodedFileName
                )
            }.onFailure { err ->
                _uiState.value = _uiState.value.copy(
                    errorMessage = "下载文档失败: ${err.message}"
                )
            }
        }
    }

    fun closeDocumentPreview() {
        _uiState.value = _uiState.value.copy(previewDocumentFile = null)
    }

    fun cancelDocumentDownload() {
        documentDownloadJob?.cancel()
        documentDownloadJob = null
        _uiState.value = _uiState.value.copy(isDownloadingDocument = false)
    }

    override fun onCleared() {
        super.onCleared()
        liveActivityManager?.cancelActivity()
        wsJob?.cancel()
        fetchJob?.cancel()
        wsClient.disconnect()
    }
}
