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
    val cascadeConfigRaw: String? = null
)

class ChatViewModel(
    private val apiClient: ApiClient,
    private val wsClient: StreamWebSocketClient,
    private val prefs: PreferencesManager? = null,
    private val documentCacheManager: DocumentCacheManager? = null,
    private val liveActivityManager: LiveActivityNotificationManager? = null,
    private val cacheManager: CacheManager? = null
) : ViewModel() {

    private val _uiState = MutableStateFlow(ChatUiState())
    val uiState: StateFlow<ChatUiState> = _uiState.asStateFlow()

    private val _inputText = MutableStateFlow("")
    val inputText: StateFlow<String> = _inputText.asStateFlow()

    private val _scrollToBottomTrigger = MutableStateFlow(0)
    val scrollToBottomTrigger: StateFlow<Int> = _scrollToBottomTrigger.asStateFlow()

    var isUnreadOnEntry: Boolean = false
        private set

    var initialConversationStatus: ConversationStatus? = null
        private set

    var currentDraftProject: ProjectItem? = null
        private set

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

    private val _isRefreshing = MutableStateFlow(false)
    val isRefreshing: StateFlow<Boolean> = _isRefreshing.asStateFlow()

    var onConversationUpdated: ((ConversationItem) -> Unit)? = null

    private var wsJob: Job? = null
    private var fetchJob: Job? = null
    private var pendingOptimisticMessageId: String? = null
    private var currentLastModifiedTime: String? = null
    private val pendingOptimisticQueueItems = mutableListOf<PendingOptimisticQueueItem>()
    private val deletedQueueTombstones = mutableListOf<QueuedMessageTombstone>()
    private val inFlightDeletingQueueIds = mutableSetOf<String>()

    private fun extractStepIndex(id: String): Int? {
        if (id.startsWith("step-")) {
            return id.removePrefix("step-").toIntOrNull()
        }
        return null
    }

    private fun isStatusRunning(status: String?): Boolean {
        if (status.isNullOrBlank()) return false
        return status.equals("CASCADE_RUN_STATUS_RUNNING", ignoreCase = true) ||
               status.equals("RUNNING", ignoreCase = true)
    }

    private fun sanitizeMessageOrder(list: List<GatewayMessageItem>): List<GatewayMessageItem> {
        if (list.size < 2) return list

        var dropIndex: Int? = null
        var prevStep = -1

        for (i in list.indices) {
            val msg = list[i]
            val step = msg.stepIndex ?: extractStepIndex(msg.id)
            if (step != null) {
                if (prevStep != -1 && step < prevStep && (prevStep - step) >= 2) {
                    dropIndex = i
                    break
                }
                prevStep = step
            }
        }

        if (dropIndex != null) {
            val head = list.subList(0, dropIndex)
            val tail = list.subList(dropIndex, list.size)
            return tail + head
        }
        return list
    }

    private fun mergeIncomingMessages(incoming: List<GatewayMessageItem>): List<GatewayMessageItem> {
        if (incoming.isEmpty()) return _uiState.value.messages
        val currentMessages = _uiState.value.messages
        if (currentMessages.isEmpty()) return sanitizeMessageOrder(incoming)

        val currentOptId = pendingOptimisticMessageId
        var optimisticMessage: GatewayMessageItem? = null
        if (currentOptId != null) {
            optimisticMessage = currentMessages.firstOrNull { it.id == currentOptId }
            val serverHasUserTurn = incoming.any {
                it.isUser && (it.id != currentOptId && (optimisticMessage == null || it.effectiveText == optimisticMessage.effectiveText))
            }
            if (serverHasUserTurn) {
                pendingOptimisticMessageId = null
                optimisticMessage = null
            }
        }

        var base = currentMessages.filter { msg ->
            if (currentOptId != null && msg.id == currentOptId) return@filter false
            if (msg.id.startsWith("opt_")) return@filter false
            true
        }

        if (base.isEmpty()) {
            base = incoming
        } else {
            var firstBaseMatchIdx: Int? = null
            var incomingMatchIdxForFirstBaseMatch: Int? = null

            for (baseIdx in base.indices) {
                val baseMsg = base[baseIdx]
                val incIdx = incoming.indexOfFirst { it.id == baseMsg.id }
                if (incIdx >= 0) {
                    firstBaseMatchIdx = baseIdx
                    incomingMatchIdxForFirstBaseMatch = incIdx
                    break
                }
            }

            if (firstBaseMatchIdx != null && incomingMatchIdxForFirstBaseMatch != null) {
                val bIdx = firstBaseMatchIdx
                val iIdx = incomingMatchIdxForFirstBaseMatch
                val prefix = base.subList(0, bIdx)
                val incomingPrefix = incoming.subList(0, iIdx)
                val incomingTail = incoming.subList(iIdx, incoming.size)
                base = prefix + incomingPrefix + incomingTail
            } else {
                val baseStep = base.mapNotNull { it.stepIndex ?: extractStepIndex(it.id) }.firstOrNull()
                val incomingStep = incoming.mapNotNull { it.stepIndex ?: extractStepIndex(it.id) }.firstOrNull()

                if (baseStep != null && incomingStep != null && incomingStep < baseStep) {
                    base = incoming + base
                } else {
                    base = base + incoming
                }
            }
        }

        if (optimisticMessage != null) {
            base = base + optimisticMessage
        }

        return sanitizeMessageOrder(base)
    }

    private fun isUserQueuedMessage(text: String): Boolean {
        val trimmed = text.trim()
        if (trimmed.isEmpty()) return false
        if (trimmed.startsWith("Task id \"") || trimmed.startsWith("Task \"") ||
            trimmed.contains("was canceled with result:") || trimmed.contains("completed with result:") ||
            trimmed.contains("Tool execution was canceled")) {
            return false
        }
        return true
    }

    private fun isUserQueuedItem(item: QueuedMessageItem): Boolean {
        val trimmed = item.text.trim()
        if (trimmed.isEmpty() && !item.hasAttachments) return false
        if (trimmed.startsWith("Task id \"") || trimmed.startsWith("Task \"") ||
            trimmed.contains("was canceled with result:") || trimmed.contains("completed with result:") ||
            trimmed.contains("Tool execution was canceled")) {
            return false
        }
        return true
    }

    private fun normalizeForComparison(text: String): String {
        return text.filter { c ->
            !c.isWhitespace() && !c.isISOControl() && c != '\u200B' && c != '\uFEFF' && c != '\u3000'
        }.lowercase()
    }

    private fun isQueuedItemInMessages(
        text: String,
        media: List<String>?,
        imageUrls: List<String>?,
        enqueuedAfterMessageId: String? = null,
        userMessages: List<GatewayMessageItem>
    ): Boolean {
        val normText = normalizeForComparison(text)
        val hasAttachments = !media.isNullOrEmpty() || !imageUrls.isNullOrEmpty()

        for (uMsg in userMessages.reversed()) {
            if (enqueuedAfterMessageId != null && uMsg.id == enqueuedAfterMessageId) {
                break
            }
            val normMsg = normalizeForComparison(uMsg.effectiveText)
            val msgHasAttachments = uMsg.imageDataList.isNotEmpty() || !uMsg.media.isNullOrEmpty()

            if (normText.isNotEmpty()) {
                if (normText == normMsg) {
                    return true
                }
                if (normText.length >= 6 && normMsg.length >= 6 && (normText.contains(normMsg) || normMsg.contains(normText))) {
                    return true
                }
            } else if (hasAttachments && msgHasAttachments) {
                return true
            }
        }
        return false
    }

    private fun syncQueuedMessages(
        serverQueue: List<QueuedMessageItem>?,
        currentMessages: List<GatewayMessageItem>? = null
    ): List<QueuedMessageItem> {
        val now = System.currentTimeMillis()
        // 1. Expire stale optimistic items older than 15 seconds
        pendingOptimisticQueueItems.removeAll { now - it.createdAt > 15_000L }

        // 2. Expire stale tombstones older than 10 seconds
        deletedQueueTombstones.removeAll { now - it.deletedAt > 10_000L }
        val tombstoneIds = deletedQueueTombstones.mapNotNull { it.id }.toSet()
        val tombstoneTexts = deletedQueueTombstones.map { normalizeForComparison(it.text) }.toSet()

        // 3. Identify user messages in the active conversation
        val msgSource = currentMessages ?: _uiState.value.messages
        val userMessages = msgSource.filter { it.isUser }

        // 4. Clear optimistic items if server has incorporated them OR if entered chat OR if tombstoned
        pendingOptimisticQueueItems.removeAll { opt ->
            val trimmed = opt.text.trim()
            val normOpt = normalizeForComparison(opt.text)
            val inServer = serverQueue?.any { s ->
                if (normOpt.isNotEmpty()) {
                    normalizeForComparison(s.text) == normOpt
                } else {
                    s.id == opt.id
                }
            } == true
            val inChat = isQueuedItemInMessages(
                text = opt.text,
                media = opt.media,
                imageUrls = opt.imageUrls,
                enqueuedAfterMessageId = opt.enqueuedAfterMessageId,
                userMessages = userMessages
            )
            val isTombstoned = tombstoneIds.contains(opt.id) || (normOpt.isNotEmpty() && tombstoneTexts.contains(normOpt))
            if (inChat) {
                deletedQueueTombstones.add(QueuedMessageTombstone(id = opt.id, text = trimmed, deletedAt = now))
            }
            inServer || inChat || isTombstoned
        }

        // 5. Compute base queue from server if provided, otherwise filter existing queue
        val baseQueue: List<QueuedMessageItem> = if (serverQueue != null) {
            serverQueue.filter { sItem ->
                val trimmed = sItem.text.trim()
                val normItem = normalizeForComparison(sItem.text)
                val inChat = isQueuedItemInMessages(
                    text = sItem.text,
                    media = sItem.media,
                    imageUrls = sItem.imageUrls,
                    enqueuedAfterMessageId = null,
                    userMessages = userMessages
                )
                val isTombstoned = tombstoneIds.contains(sItem.id) || (normItem.isNotEmpty() && tombstoneTexts.contains(normItem))
                if (inChat) {
                    deletedQueueTombstones.add(QueuedMessageTombstone(id = sItem.id, text = trimmed, deletedAt = now))
                }
                isUserQueuedItem(sItem) && !inChat && !isTombstoned
            }
        } else {
            _uiState.value.queuedMessages.filter { qm ->
                val trimmed = qm.text.trim()
                val normItem = normalizeForComparison(qm.text)
                val inChat = isQueuedItemInMessages(
                    text = qm.text,
                    media = qm.media,
                    imageUrls = qm.imageUrls,
                    enqueuedAfterMessageId = null,
                    userMessages = userMessages
                )
                val isTombstoned = tombstoneIds.contains(qm.id) || (normItem.isNotEmpty() && tombstoneTexts.contains(normItem))
                if (inChat) {
                    deletedQueueTombstones.add(QueuedMessageTombstone(id = qm.id, text = trimmed, deletedAt = now))
                }
                isUserQueuedItem(qm) && !qm.id.startsWith("queue-") && !inChat && !isTombstoned
            }
        }

        // 6. Append unconfirmed optimistic items (not yet in server queue, not entered chat, not tombstoned)
        val remainingOptItems = pendingOptimisticQueueItems.mapNotNull { opt ->
            val normOpt = normalizeForComparison(opt.text)
            if (tombstoneIds.contains(opt.id) || (normOpt.isNotEmpty() && tombstoneTexts.contains(normOpt))) {
                return@mapNotNull null
            }
            if (baseQueue.any { b ->
                if (normOpt.isNotEmpty()) {
                    normalizeForComparison(b.text) == normOpt
                } else {
                    b.id == opt.id
                }
            }) {
                return@mapNotNull null
            }
            val inChat = isQueuedItemInMessages(
                text = opt.text,
                media = opt.media,
                imageUrls = opt.imageUrls,
                enqueuedAfterMessageId = opt.enqueuedAfterMessageId,
                userMessages = userMessages
            )
            if (inChat) {
                return@mapNotNull null
            }
            QueuedMessageItem(
                id = opt.id,
                text = opt.text,
                media = opt.media,
                imageUrls = opt.imageUrls
            )
        }

        return baseQueue + remainingOptItems
    }

    private fun decodeBase64ToAttachment(raw: String): AttachmentImage? {
        return try {
            val cleaned = if (raw.contains(",")) raw.substringAfter(",") else raw
            val bytes = Base64.decode(cleaned, Base64.DEFAULT)
            val bitmap = BitmapFactory.decodeByteArray(bytes, 0, bytes.size) ?: return null
            AttachmentImage(
                uri = Uri.EMPTY,
                bitmap = bitmap,
                byteArray = bytes
            )
        } catch (_: Exception) {
            null
        }
    }

    fun saveSessionToCache() {
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

    fun currentConversationItem(): ConversationItem? {
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
        pendingOptimisticQueueItems.clear()
        deletedQueueTombstones.clear()
        inFlightDeletingQueueIds.clear()
        currentDraftProject = null
        currentLastModifiedTime = null
        _inputText.value = ""
        _uiState.value = ChatUiState()
    }

    fun prepareSession(
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
            draftImages.mapNotNull { bytes ->
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

    fun initSession(
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

            val savedDraftText = prefs?.getDraftText(cascadeId).orEmpty()
            _inputText.value = savedDraftText

            val draftImages = prefs?.loadDraftImages(cascadeId).orEmpty()
            val attachments = if (draftImages.isNotEmpty()) {
                draftImages.mapNotNull { bytes ->
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

                        _uiState.value = _uiState.value.copy(
                            isLoading = false,
                            title = resolvedTitle,
                            workspaceName = wsFromPayload ?: _uiState.value.workspaceName,
                            messages = mergedMsgs,
                            runningTasks = payload.runningTasks ?: emptyList(),
                            queuedMessages = syncQueuedMessages(payload.queuedMessages, mergedMsgs),
                            isRunning = isStatusRunning(payload.status),
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

    fun retryLoadMessages() {
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

                    _uiState.value = _uiState.value.copy(
                        isLoading = false,
                        title = payload.title?.takeIf { it.isNotBlank() } ?: _uiState.value.title,
                        workspaceName = wsFromPayload ?: _uiState.value.workspaceName,
                        messages = msgs,
                        runningTasks = payload.runningTasks ?: emptyList(),
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
                    saveSessionToCache()
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

                            _uiState.value = _uiState.value.copy(
                                isLoading = false,
                                title = payload.title?.takeIf { it.isNotBlank() } ?: _uiState.value.title,
                                workspaceName = wsFromPayload ?: _uiState.value.workspaceName,
                                messages = msgs,
                                runningTasks = payload.runningTasks ?: emptyList(),
                                queuedMessages = syncQueuedMessages(payload.queuedMessages, msgs),
                                isRunning = isStatusRunning(payload.status),
                                isLatestMessageError = isError,
                                hasMore = hasMore,
                                nextOffset = nextOffset,
                                stepCount = payload.totalSteps,
                                totalTools = payload.totalTools,
                                duration = payload.duration ?: _uiState.value.duration,
                                cascadeConfigRaw = payload.cascadeConfigRaw ?: _uiState.value.cascadeConfigRaw
                            )
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

    fun loadOlderMessages() {
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

    fun onInputTextChanged(text: String) {
        _inputText.value = text
        val cid = _uiState.value.cascadeId
        if (cid.isNotBlank()) {
            prefs?.setDraftText(cid, text)
        }
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

    fun removeImage(index: Int) {
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
        if (text.isEmpty() && images.isEmpty()) return
        val cid = _uiState.value.cascadeId
        if (!cid.startsWith("local_draft_")) {
            prefs?.clearDraftText(cid)
            prefs?.clearDraftImages(cid)
        }
        _inputText.value = ""
        _uiState.value = _uiState.value.copy(selectedImages = emptyList())
        sendMessage(text, images)
    }

    fun saveDraftFor(targetCid: String, text: String, imageBytes: List<ByteArray>? = null) {
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

    fun deleteLocalDraftSession(cascadeId: String) {
        prefs?.deleteLocalDraftSession(cascadeId)
        prefs?.clearDraftText(cascadeId)
        prefs?.clearDraftImages(cascadeId)
        if (_uiState.value.cascadeId == cascadeId) {
            currentDraftProject = null
        }
    }

    fun hasDraftImages(cascadeId: String): Boolean {
        return prefs?.hasDraftImages(cascadeId) == true
    }

    fun handleBack(cascadeId: String, inputText: String) {
        val hasImages = _uiState.value.selectedImages.isNotEmpty() || (prefs?.hasDraftImages(cascadeId) == true)
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

    private fun sendMessage(
        text: String,
        attachments: List<AttachmentImage> = emptyList(),
        forceImmediate: Boolean = false
    ) {
        val cascadeId = _uiState.value.cascadeId
        val model = _uiState.value.activeModel

        val isRunningOrAwaiting = _uiState.value.isRunning || _uiState.value.isAwaitingResponse
        val canQueue = !forceImmediate && isRunningOrAwaiting && cascadeId.isNotBlank() && !cascadeId.startsWith("local_draft_")

        if (canQueue) {
            val trimmed = text.trim()
            if (trimmed.isNotEmpty()) {
                val norm = normalizeForComparison(trimmed)
                deletedQueueTombstones.removeAll { normalizeForComparison(it.text) == norm }
            }
            val mediaBase64 = attachments.map { att ->
                Base64.encodeToString(att.byteArray, Base64.NO_WRAP)
            }.takeIf { it.isNotEmpty() }

            val queueItem = QueuedMessageItem(
                id = "queue-${UUID.randomUUID()}",
                text = text,
                media = mediaBase64
            )
            val lastUserMsgId = _uiState.value.messages.lastOrNull { it.isUser }?.id
            pendingOptimisticQueueItems.add(
                PendingOptimisticQueueItem(
                    id = queueItem.id,
                    text = text,
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
                        clientMessageId = queueClientMsgId
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
        saveSessionToCache()

        if (cascadeId.startsWith("local_draft_")) {
            val draftSession = prefs?.getLocalDraftSession(cascadeId)
            val project = currentDraftProject ?: draftSession?.project ?: ProjectItem.PURE_CHAT
            val isPure = project.isPureChat
            val pid = if (isPure) "outside-of-project" else (project.rawId ?: (if (project.id != project.uri) project.id else null))
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
                    currentDraftProject = null
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
                            saveSessionToCache()
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
                clientMessageId = optId
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

    fun cancelExecution() {
        val cascadeId = _uiState.value.cascadeId
        _uiState.value = _uiState.value.copy(isRunning = false, isAwaitingResponse = false)
        liveActivityManager?.endActivity(cascadeId = cascadeId, finalStatus = "CANCELLED")
        notifyConversationUpdated()
        saveSessionToCache()
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
        val cascadeId = _uiState.value.cascadeId
        val trimmedText = item.text.trim()
        val now = System.currentTimeMillis()

        deletedQueueTombstones.add(QueuedMessageTombstone(id = item.id, text = trimmedText, deletedAt = now))
        pendingOptimisticQueueItems.removeAll { it.id == item.id || (trimmedText.isNotEmpty() && normalizeForComparison(it.text) == normalizeForComparison(trimmedText)) }
        _uiState.value = _uiState.value.copy(
            queuedMessages = _uiState.value.queuedMessages.filter { it.id != item.id }
        )

        val attachments = item.media?.mapNotNull { decodeBase64ToAttachment(it) } ?: emptyList()
        sendMessage(item.text, attachments, forceImmediate = true)

        viewModelScope.launch {
            var targetMsgId: String? = if (item.id.startsWith("queue-")) null else item.id
            if (targetMsgId == null) {
                delay(350)
                apiClient.fetchMessages(cascadeId, limit = 15).onSuccess { res ->
                    val match = res.queuedMessages?.firstOrNull { it.text.trim() == trimmedText }
                    if (match != null) {
                        targetMsgId = match.id
                    }
                }
            }
            val msgId = targetMsgId
            if (!msgId.isNullOrBlank() && !msgId.startsWith("queue-")) {
                apiClient.deleteAgentMessage(cascadeId, msgId)
            }
        }
    }

    fun editQueuedMessage(item: QueuedMessageItem) {
        if (inFlightDeletingQueueIds.contains(item.id)) return
        inFlightDeletingQueueIds.add(item.id)

        val cascadeId = _uiState.value.cascadeId
        val trimmedText = item.text.trim()
        val now = System.currentTimeMillis()

        deletedQueueTombstones.add(QueuedMessageTombstone(id = item.id, text = trimmedText, deletedAt = now))
        pendingOptimisticQueueItems.removeAll { it.id == item.id || (trimmedText.isNotEmpty() && normalizeForComparison(it.text) == normalizeForComparison(trimmedText)) }
        _uiState.value = _uiState.value.copy(
            queuedMessages = _uiState.value.queuedMessages.filter { it.id != item.id }
        )
        _scrollToBottomTrigger.value++
        saveSessionToCache()

        _inputText.value = item.text
        if (!item.media.isNullOrEmpty()) {
            val attachments = item.media.mapNotNull { decodeBase64ToAttachment(it) }
            if (attachments.isNotEmpty()) {
                _uiState.value = _uiState.value.copy(selectedImages = attachments)
            }
        }

        if (cascadeId.isNotBlank() && !cascadeId.startsWith("local_draft_")) {
            viewModelScope.launch {
                try {
                    var targetMsgId: String? = if (item.id.startsWith("queue-")) null else item.id
                    if (targetMsgId == null) {
                        delay(350)
                        apiClient.fetchMessages(cascadeId, limit = 15).onSuccess { res ->
                            val match = res.queuedMessages?.firstOrNull { it.text.trim() == trimmedText }
                            if (match != null) {
                                targetMsgId = match.id
                                deletedQueueTombstones.add(QueuedMessageTombstone(id = match.id, text = trimmedText, deletedAt = System.currentTimeMillis()))
                                inFlightDeletingQueueIds.add(match.id)
                            }
                        }
                    }
                    val msgId = targetMsgId
                    if (!msgId.isNullOrBlank() && !msgId.startsWith("queue-")) {
                        apiClient.deleteAgentMessage(cascadeId, msgId)
                        inFlightDeletingQueueIds.remove(msgId)
                    }
                } finally {
                    inFlightDeletingQueueIds.remove(item.id)
                }
            }
        } else {
            inFlightDeletingQueueIds.remove(item.id)
        }
    }

    fun deleteQueuedMessage(item: QueuedMessageItem) {
        if (inFlightDeletingQueueIds.contains(item.id)) return
        inFlightDeletingQueueIds.add(item.id)

        val cascadeId = _uiState.value.cascadeId
        val trimmedText = item.text.trim()
        val now = System.currentTimeMillis()

        deletedQueueTombstones.add(QueuedMessageTombstone(id = item.id, text = trimmedText, deletedAt = now))
        pendingOptimisticQueueItems.removeAll { it.id == item.id || (trimmedText.isNotEmpty() && normalizeForComparison(it.text) == normalizeForComparison(trimmedText)) }
        _uiState.value = _uiState.value.copy(
            queuedMessages = _uiState.value.queuedMessages.filter { it.id != item.id }
        )
        _scrollToBottomTrigger.value++
        saveSessionToCache()

        if (cascadeId.isNotBlank() && !cascadeId.startsWith("local_draft_")) {
            viewModelScope.launch {
                try {
                    var targetMsgId: String? = if (item.id.startsWith("queue-")) null else item.id
                    if (targetMsgId == null) {
                        delay(350)
                        apiClient.fetchMessages(cascadeId, limit = 15).onSuccess { res ->
                            val match = res.queuedMessages?.firstOrNull { it.text.trim() == trimmedText }
                            if (match != null) {
                                targetMsgId = match.id
                                deletedQueueTombstones.add(QueuedMessageTombstone(id = match.id, text = trimmedText, deletedAt = System.currentTimeMillis()))
                                inFlightDeletingQueueIds.add(match.id)
                            }
                        }
                    }
                    val msgId = targetMsgId
                    if (!msgId.isNullOrBlank() && !msgId.startsWith("queue-")) {
                        apiClient.deleteAgentMessage(cascadeId, msgId)
                        inFlightDeletingQueueIds.remove(msgId)
                    }
                } finally {
                    inFlightDeletingQueueIds.remove(item.id)
                }
            }
        } else {
            inFlightDeletingQueueIds.remove(item.id)
        }
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
            saveSessionToCache()
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
            saveSessionToCache()
        }
    }

    fun proceedArtifact() {
        val cascadeId = _uiState.value.cascadeId
        if (cascadeId.isBlank()) return
        val uri = _uiState.value.proceedArtifactUri ?: "implementation_plan.md"
        val model = _uiState.value.activeModel

        _uiState.value = _uiState.value.copy(
            canProceed = false,
            proceedArtifactUri = null,
            isRunning = true,
            isAwaitingResponse = true,
            isLatestMessageError = false
        )
        saveSessionToCache()
        notifyConversationUpdated()
        ensureWebSocketObserving()

        viewModelScope.launch {
            val result = apiClient.proceedArtifact(cascadeId, uri, model)
            if (result.isFailure) {
                _uiState.value = _uiState.value.copy(
                    canProceed = true,
                    proceedArtifactUri = uri,
                    isRunning = false,
                    isAwaitingResponse = false,
                    isLatestMessageError = true
                )
                saveSessionToCache()
                notifyConversationUpdated()
            } else {
                delay(250)
                refresh()
            }
        }
    }

    fun openMarkdownViewer(uri: String, title: String) {
        val isPlan = uri.contains("implementation_plan", ignoreCase = true) || title.contains("implementation_plan", ignoreCase = true)
        val targetUri = if (isPlan && !_uiState.value.proceedArtifactUri.isNullOrBlank()) {
            _uiState.value.proceedArtifactUri!!
        } else {
            uri
        }

        val rawFilename = targetUri.substringAfterLast('/')
        val decodedFilename = try { URLDecoder.decode(rawFilename, "UTF-8") } catch (_: Exception) { rawFilename }
        val decodedTitle = try { URLDecoder.decode(title, "UTF-8") } catch (_: Exception) { title }
        val initialTitle = when {
            isPlan -> "Implementation Plan"
            decodedTitle.isNotBlank() -> decodedTitle
            else -> decodedFilename
        }

        val isProceedActive = _uiState.value.canProceed && isPlan

        _uiState.value = _uiState.value.copy(
            markdownViewerData = MarkdownFileViewerData(
                uri = targetUri,
                title = initialTitle,
                filename = decodedFilename,
                content = "",
                summary = null,
                canProceed = isProceedActive,
                isLoading = true
            )
        )

        viewModelScope.launch {
            val res = apiClient.fetchFileContent(targetUri, _uiState.value.cascadeId)
            res.onSuccess { resp ->
                val serverFilename = try { URLDecoder.decode(resp.filename, "UTF-8") } catch (_: Exception) { resp.filename }
                val resolvedFilename = serverFilename.ifBlank { decodedFilename }
                val resolvedTitle = if (isPlan) {
                    "Implementation Plan"
                } else if (initialTitle.isBlank() || initialTitle == rawFilename || initialTitle.contains("%")) {
                    resolvedFilename
                } else {
                    initialTitle
                }

                val canProceedFromResp = isProceedActive || (resp.requestFeedback == true && _uiState.value.canProceed)

                _uiState.value = _uiState.value.copy(
                    markdownViewerData = _uiState.value.markdownViewerData?.copy(
                        title = resolvedTitle,
                        filename = resolvedFilename,
                        content = resp.content,
                        summary = resp.summary,
                        canProceed = canProceedFromResp,
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
