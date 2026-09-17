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
import com.antigravity.mobile.data.service.StreamWebSocketClient
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import java.util.UUID

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
    val markdownViewerData: MarkdownFileViewerData? = null
)

class ChatViewModel(
    private val apiClient: ApiClient,
    private val wsClient: StreamWebSocketClient
) : ViewModel() {

    private val _uiState = MutableStateFlow(ChatUiState())
    val uiState: StateFlow<ChatUiState> = _uiState.asStateFlow()

    private val _inputText = MutableStateFlow("")
    val inputText: StateFlow<String> = _inputText.asStateFlow()

    private val _scrollToBottomTrigger = MutableStateFlow(0)
    val scrollToBottomTrigger: StateFlow<Int> = _scrollToBottomTrigger.asStateFlow()

    private var wsJob: Job? = null
    private var fetchJob: Job? = null

    fun resetSession() {
        fetchJob?.cancel()
        fetchJob = null
        wsJob?.cancel()
        wsJob = null
        wsClient.disconnect()
        _inputText.value = ""
        _uiState.value = ChatUiState()
    }

    fun initSession(cascadeId: String, initialTitle: String? = null, isNewConversation: Boolean = false) {
        val isDifferentSession = _uiState.value.cascadeId != cascadeId
        val shouldLoad = !isNewConversation
        if (isDifferentSession) {
            fetchJob?.cancel()
            fetchJob = null
            wsClient.disconnect()
            _inputText.value = ""
            _uiState.value = ChatUiState(
                cascadeId = cascadeId,
                title = initialTitle?.takeIf { it.isNotBlank() } ?: "会话 $cascadeId",
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
                isNewConversation = isNewConversation,
                isLoading = if (_uiState.value.messages.isEmpty() && !isNewConversation) true else _uiState.value.isLoading
            )
        }

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
                        _uiState.value = _uiState.value.copy(
                            isLoading = false,
                            title = payload.title?.takeIf { it.isNotBlank() } ?: _uiState.value.title,
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
                    val previousMsgCount = _uiState.value.messages.size

                    _uiState.value = _uiState.value.copy(
                        title = payload.title?.takeIf { it.isNotBlank() } ?: _uiState.value.title,
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

                    if (msgs.size != previousMsgCount || isRunning) {
                        _scrollToBottomTrigger.value++
                    }
                }
            }
        }
    }

    fun onInputTextChanged(text: String) {
        _inputText.value = text
    }

    fun addImagesFromUris(context: Context, uris: List<Uri>) {
        viewModelScope.launch(Dispatchers.IO) {
            val newAttachments = mutableListOf<AttachmentImage>()
            for (uri in uris) {
                try {
                    val bytes = context.contentResolver.openInputStream(uri)?.use { it.readBytes() } ?: continue
                    val mimeType = context.contentResolver.getType(uri) ?: "image/jpeg"
                    val options = BitmapFactory.Options().apply {
                        inJustDecodeBounds = true
                    }
                    BitmapFactory.decodeByteArray(bytes, 0, bytes.size, options)
                    val sampleSize = maxOf(1, maxOf(options.outWidth, options.outHeight) / 256)
                    val decodeOptions = BitmapFactory.Options().apply {
                        inSampleSize = sampleSize
                    }
                    val bitmap = BitmapFactory.decodeByteArray(bytes, 0, bytes.size, decodeOptions)
                    if (bitmap != null) {
                        newAttachments.add(
                            AttachmentImage(
                                uri = uri,
                                bitmap = bitmap,
                                byteArray = bytes,
                                mimeType = mimeType
                            )
                        )
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
        _inputText.value = ""
        _uiState.value = _uiState.value.copy(selectedImages = emptyList())
        sendMessage(text, images)
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

        val imagePayloads = attachments.map { Pair(it.byteArray, it.mimeType) }

        viewModelScope.launch {
            val result = apiClient.sendMessage(cascadeId, text, model, imagePayloads)
            result.onFailure { err ->
                _uiState.value = _uiState.value.copy(
                    errorMessage = "发送失败: ${err.message}",
                    isRunning = false,
                    isAwaitingResponse = false
                )
            }
        }
    }

    fun cancelExecution() {
        val cascadeId = _uiState.value.cascadeId
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
        _uiState.value = _uiState.value.copy(
            markdownViewerData = MarkdownFileViewerData(
                uri = uri,
                title = title,
                filename = uri.substringAfterLast('/'),
                content = "",
                summary = null,
                canProceed = _uiState.value.canProceed,
                isLoading = true
            )
        )

        viewModelScope.launch {
            val res = apiClient.fetchFileContent(uri, _uiState.value.cascadeId)
            res.onSuccess { resp ->
                _uiState.value = _uiState.value.copy(
                    markdownViewerData = _uiState.value.markdownViewerData?.copy(
                        filename = resp.filename.ifBlank { uri.substringAfterLast('/') },
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

    override fun onCleared() {
        super.onCleared()
        wsJob?.cancel()
        fetchJob?.cancel()
        wsClient.disconnect()
    }
}
