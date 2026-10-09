package com.antigravity.mobile.ui.viewmodel

import androidx.lifecycle.viewModelScope
import com.antigravity.mobile.data.model.*
import kotlinx.coroutines.launch
import kotlinx.coroutines.delay

internal fun ChatViewModel.isUserQueuedMessage(text: String): Boolean {
    val trimmed = text.trim()
    if (trimmed.isEmpty()) return false
    if (trimmed.startsWith("Task id \"") || trimmed.startsWith("Task \"") ||
        trimmed.contains("was canceled with result:") || trimmed.contains("completed with result:") ||
        trimmed.contains("Tool execution was canceled")) {
        return false
    }
    return true
}

internal fun ChatViewModel.isUserQueuedItem(item: QueuedMessageItem): Boolean {
    val trimmed = item.text.trim()
    if (trimmed.isEmpty() && !item.hasAttachments) return false
    if (trimmed.startsWith("Task id \"") || trimmed.startsWith("Task \"") ||
        trimmed.contains("was canceled with result:") || trimmed.contains("completed with result:") ||
        trimmed.contains("Tool execution was canceled")) {
        return false
    }
    return true
}

internal fun ChatViewModel.normalizeForComparison(text: String): String {
    return text.filter { c ->
        !c.isWhitespace() && !c.isISOControl() && c != '\u200B' && c != '\uFEFF' && c != '\u3000'
    }.lowercase()
}

internal fun ChatViewModel.isQueuedItemInMessages(
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
        val msgHasAttachments = uMsg.imageDataList.isNotEmpty() || !uMsg.media.isNullOrEmpty() || !uMsg.imageUrls.isNullOrEmpty()

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

internal fun ChatViewModel.syncQueuedMessages(
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
            when {
                // 网关把客户端 id 写进了消息 tags，服务端队列里能精确认出这一条
                !opt.clientMessageId.isNullOrEmpty() && s.clientMessageId == opt.clientMessageId -> true
                normOpt.isNotEmpty() -> normalizeForComparison(s.text) == normOpt
                else -> s.id == opt.id
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
            when {
                !opt.clientMessageId.isNullOrEmpty() && b.clientMessageId == opt.clientMessageId -> true
                normOpt.isNotEmpty() -> normalizeForComparison(b.text) == normOpt
                else -> b.id == opt.id
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
            imageUrls = opt.imageUrls,
            clientMessageId = opt.clientMessageId
        )
    }

    return baseQueue + remainingOptItems
}

internal fun ChatViewModel.sendQueuedMessageNow(item: QueuedMessageItem) {
    val cascadeId = _uiState.value.cascadeId
    val trimmedText = item.text.trim()
    val now = System.currentTimeMillis()

    deletedQueueTombstones.add(QueuedMessageTombstone(id = item.id, text = trimmedText, deletedAt = now))
    pendingOptimisticQueueItems.removeAll { it.id == item.id || (trimmedText.isNotEmpty() && normalizeForComparison(it.text) == normalizeForComparison(trimmedText)) }
    _uiState.value = _uiState.value.copy(
        queuedMessages = _uiState.value.queuedMessages.filter { it.id != item.id }
    )

    val attachments = item.media?.mapNotNull { decodeBase64ToAttachment(it) } ?: emptyList()
    // 队列里显示的是 "/plan 内容"：还原成「命令 + 文本」再发，否则会变成一条以斜杠开头的普通文字
    val (slash, rest) = SlashCommandFilter.splitPrefix(item.text, _uiState.value.slashCommands)
    sendMessage(rest, attachments, forceImmediate = true, slashCommand = slash?.name)

    viewModelScope.launch {
        var targetMsgId: String? = if (item.id.startsWith("queue-")) null else item.id
        if (targetMsgId == null) {
            delay(350)
            apiClient.fetchMessages(cascadeId, limit = 15).onSuccess { res ->
                val match = findServerQueueItem(res.queuedMessages, item, trimmedText)
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

internal fun ChatViewModel.editQueuedMessage(item: QueuedMessageItem) {
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

    val (slash, rest) = SlashCommandFilter.splitPrefix(item.text, _uiState.value.slashCommands)
    _inputText.value = rest
    if (slash != null) {
        _uiState.value = _uiState.value.copy(selectedSlashCommand = slash)
    }
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
                        val match = findServerQueueItem(res.queuedMessages, item, trimmedText)
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

internal fun ChatViewModel.deleteQueuedMessage(item: QueuedMessageItem) {
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
                        val match = findServerQueueItem(res.queuedMessages, item, trimmedText)
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

/**
 * 在服务端队列里找到与本地条目对应的那一条：优先按客户端 id（精确），没有 id（旧网关、非本机发出）再按文字。
 */
internal fun findServerQueueItem(
    serverQueue: List<QueuedMessageItem>?,
    item: QueuedMessageItem,
    trimmedText: String
): QueuedMessageItem? {
    if (serverQueue.isNullOrEmpty()) return null
    val cid = item.clientMessageId
    if (!cid.isNullOrEmpty()) {
        serverQueue.firstOrNull { it.clientMessageId == cid }?.let { return it }
        // 网关已支持 id 标注（队列里有带 id 的条目）却找不到：说明服务端还没收到这一条，
        // 此时按文字回退可能误删另一条同文字消息，宁可返回空让调用方稍后重试
        if (serverQueue.any { !it.clientMessageId.isNullOrEmpty() }) return null
    }
    return serverQueue.firstOrNull { it.text.trim() == trimmedText }
}
