package com.antigravity.mobile.ui.viewmodel

import android.content.Context
import android.graphics.Bitmap
import android.graphics.BitmapFactory
import android.net.Uri
import android.util.Base64
import android.util.Log
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.antigravity.mobile.data.model.*
import com.antigravity.mobile.data.service.ApiClient
import com.antigravity.mobile.data.service.CacheManager
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
    val bitmap: Bitmap? = null,
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

data class PendingOptimisticQueueItem(
    val id: String,
    val text: String,
    val media: List<String>? = null,
    val imageUrls: List<String>? = null,
    val createdAt: Long = System.currentTimeMillis(),
    val enqueuedAfterMessageId: String? = null
)

data class QueuedMessageTombstone(
    val id: String?,
    val text: String,
    val deletedAt: Long = System.currentTimeMillis()
)

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
    val selectedFiles: List<AttachmentFile> = emptyList(),
    val attachmentNotice: String? = null,
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
    val previewDocumentTitle: String = "",
    val hasMore: Boolean = false,
    val nextOffset: Int = 0,
    val isLoadingOlder: Boolean = false,
    val stepCount: Int = 0,
    val totalTools: Int = 0,
    val duration: String = "",
    val cascadeConfigRaw: String? = null,
    val revertPreview: RevertPreviewResponse? = null,
    val activeUndoMessage: GatewayMessageItem? = null,
    val showConfirmUndoSheet: Boolean = false,
    val isReverting: Boolean = false,
    val isLoadingRevertPreview: Boolean = false,
    val gitSheet: GitSheetState? = null,
    val focusInputTrigger: Int = 0
)

class ChatViewModel(
    internal val apiClient: ApiClient,
    internal val wsClient: StreamWebSocketClient,
    internal val prefs: PreferencesManager? = null,
    internal val documentCacheManager: DocumentCacheManager? = null,
    internal val liveActivityManager: LiveActivityNotificationManager? = null,
    internal val cacheManager: CacheManager? = null
) : ViewModel() {

    internal val _uiState = MutableStateFlow(ChatUiState())
    val uiState: StateFlow<ChatUiState> = _uiState.asStateFlow()

    internal val _inputText = MutableStateFlow("")
    val inputText: StateFlow<String> = _inputText.asStateFlow()

    internal val _scrollToBottomTrigger = MutableStateFlow(0)
    val scrollToBottomTrigger: StateFlow<Int> = _scrollToBottomTrigger.asStateFlow()

    var isUnreadOnEntry: Boolean = false
        internal set

    var initialConversationStatus: ConversationStatus? = null
        internal set

    var currentDraftProject: ProjectItem? = null
        internal set

    val latestAgentMessageId: String?
        get() {
            val msgs = _uiState.value.messages
            val lastUserIdx = msgs.indexOfLast { it.isUser }
            if (lastUserIdx >= 0) {
                val subsequent = msgs.subList(lastUserIdx + 1, msgs.size)
                return subsequent.firstOrNull { it.isAgent }?.id
            }
            return msgs.firstOrNull { it.isAgent }?.id
        }

    val latestTurnStartMessageId: String?
        get() {
            val msgs = _uiState.value.messages
            val lastUserIdx = msgs.indexOfLast { it.isUser }
            if (lastUserIdx >= 0) {
                val subsequent = msgs.subList(lastUserIdx + 1, msgs.size)
                return subsequent.firstOrNull()?.id
            }
            return msgs.firstOrNull { !it.isUser }?.id ?: msgs.firstOrNull()?.id
        }

    val shouldScrollToTurnStartOnEntry: Boolean
        get() {
            val state = _uiState.value
            val isActivelyRunning = state.isRunning || state.isAwaitingResponse || state.runningTasks.isNotEmpty() || (initialConversationStatus?.isRunning == true)
            if (isActivelyRunning) return false
            if (state.messages.lastOrNull()?.isUser == true) return false
            if (latestAgentMessageId == null && latestTurnStartMessageId == null) return false
            val hasErrorState = state.isLatestMessageError || (initialConversationStatus?.isError == true)
            val hasActionState = state.canProceed || state.pendingInteraction != null || (initialConversationStatus?.needsAction == true)
            return isUnreadOnEntry || hasErrorState || hasActionState
        }

    internal val _isRefreshing = MutableStateFlow(false)
    val isRefreshing: StateFlow<Boolean> = _isRefreshing.asStateFlow()

    var onConversationUpdated: ((ConversationItem) -> Unit)? = null

    internal var wsJob: Job? = null
    internal var fetchJob: Job? = null
    internal var pendingOptimisticMessageId: String? = null
    internal var currentLastModifiedTime: String? = null
    internal val pendingOptimisticQueueItems = mutableListOf<PendingOptimisticQueueItem>()
    internal val deletedQueueTombstones = mutableListOf<QueuedMessageTombstone>()
    internal val inFlightDeletingQueueIds = mutableSetOf<String>()
    internal val stoppedTaskKeys = java.util.Collections.synchronizedSet(mutableSetOf<String>())
    internal val fileUploadJobs = java.util.concurrent.ConcurrentHashMap<String, kotlinx.coroutines.Job>()

    fun onInputTextChanged(text: String) {
        _inputText.value = text
        val cid = _uiState.value.cascadeId
        if (cid.isNotBlank()) {
            prefs?.setDraftText(cid, text)
        }
    }

    fun clearImages() {
        _uiState.value = _uiState.value.copy(selectedImages = emptyList())
        val cid = _uiState.value.cascadeId
        if (cid.isNotBlank()) {
            prefs?.clearDraftImages(cid)
        }
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
        val files = _uiState.value.selectedFiles
        if (text.isEmpty() && images.isEmpty() && files.isEmpty()) return
        if (files.any { it.state == UploadState.FAILED }) {
            _uiState.value = _uiState.value.copy(attachmentNotice = "有文件上传失败，请重试或移除后再发送")
            return
        }
        if (files.any { !it.isUploaded }) {
            _uiState.value = _uiState.value.copy(attachmentNotice = "文件仍在上传，请稍候")
            return
        }
        val cid = _uiState.value.cascadeId
        if (!cid.startsWith("local_draft_")) {
            prefs?.clearDraftText(cid)
            prefs?.clearDraftImages(cid)
            prefs?.clearDraftFiles(cid)
        }
        _inputText.value = ""
        _uiState.value = _uiState.value.copy(selectedImages = emptyList(), selectedFiles = emptyList())
        sendMessage(text, images, files = files)
    }

    fun deleteLocalDraftSession(cascadeId: String) {
        prefs?.deleteLocalDraftSession(cascadeId)
        prefs?.clearDraftText(cascadeId)
        prefs?.clearDraftImages(cascadeId)
        prefs?.clearDraftFiles(cascadeId)
        if (_uiState.value.cascadeId == cascadeId) {
            currentDraftProject = null
        }
    }

    fun hasDraftImages(cascadeId: String): Boolean {
        return prefs?.hasDraftImages(cascadeId) == true
    }

    fun handleBack(cascadeId: String, inputText: String) {
        val hasImages = _uiState.value.selectedImages.isNotEmpty() || _uiState.value.selectedFiles.isNotEmpty() ||
            (prefs?.hasDraftImages(cascadeId) == true) || (prefs?.hasDraftFiles(cascadeId) == true)
        if (cascadeId.startsWith("local_draft_") && inputText.isBlank() && !hasImages) {
            deleteLocalDraftSession(cascadeId)
        } else {
            saveDraftFor(cascadeId, inputText, _uiState.value.selectedImages.map { it.byteArray })
        }
    }

    fun saveDraft() {
        val cid = _uiState.value.cascadeId
        if (cid.isBlank()) return
        saveDraftFor(cid, _inputText.value, _uiState.value.selectedImages.map { it.byteArray })
    }

    fun approveInteraction() {
        val interaction = _uiState.value.pendingInteraction ?: return
        val defaultOpt = interaction.defaultOptionId ?: interaction.options.firstOrNull()?.id ?: "1"
        submitInteraction(optionId = defaultOpt)
    }

    fun rejectInteraction() {
        val interaction = _uiState.value.pendingInteraction ?: return
        val denyOpt = interaction.options.firstOrNull { it.isDeny }?.id ?: "5"
        submitInteraction(optionId = denyOpt)
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

    fun closeImageViewer() {
        _uiState.value = _uiState.value.copy(imageViewerData = null)
    }

    fun resolveMediaUrl(raw: String): String {
        return apiClient.resolveMediaURL(raw)
    }

    internal var documentDownloadJob: Job? = null

    fun closeDocumentPreview() {
        _uiState.value = _uiState.value.copy(previewDocumentFile = null)
    }

    fun cancelDocumentDownload() {
        documentDownloadJob?.cancel()
        documentDownloadJob = null
        _uiState.value = _uiState.value.copy(isDownloadingDocument = false)
    }

    // MARK: - Revert / Undo Operations

    fun dismissConfirmUndo() {
        _uiState.value = _uiState.value.copy(
            showConfirmUndoSheet = false,
            activeUndoMessage = null,
            revertPreview = null,
            isLoadingRevertPreview = false
        )
    }

    override fun onCleared() {
        super.onCleared()
        liveActivityManager?.cancelActivity()
        wsJob?.cancel()
        fetchJob?.cancel()
        wsClient.disconnect()
    }
}
