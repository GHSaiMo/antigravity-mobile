package com.antigravity.mobile.data.model

/**
 * 表示对话中的单轮问答（从用户输入提问开始，到下一个用户提问之前的所有 agent/tool 响应）
 */
data class ConversationTurn(
    val id: String,
    /** 轮次编号（从 1 开始：1, 2, 3...） */
    val turnIndex: Int,
    /** 开启本轮的用户消息（如果存在） */
    val userMessage: GatewayMessageItem?,
    /** 本轮包含的所有完整消息 */
    val messages: List<GatewayMessageItem>,
    /** 滚动定位的目标消息 ID（优先定位用户消息，无用户消息时定位本轮首条） */
    val firstMessageId: String,
    /** 提取的用户提问摘要（用于大纲导航，约 40 字符） */
    val promptSummary: String,
    /** 本轮调用的所有工具总数与名称 */
    val totalTools: Int,
    val toolNames: List<String>,
    /** 本轮生成的所有产物 */
    val artifacts: List<ArtifactItem>,
    /** 本轮是否存在错误 */
    val hasError: Boolean,
    /** 本轮是否包含正在进行或等待中的任务 */
    val isActive: Boolean,
    /** 本轮使用的模型名称（若有） */
    val modelName: String?,
    /** 本轮创建时间戳 (RFC3339) */
    val timestamp: String? = null
)

/**
 * 超长对话的章节分段（对齐 Antigravity 2.28.1 Chapters 规范）
 */
data class ConversationChapter(
    val id: String,
    val title: String,
    val rangeText: String,
    val turns: List<ConversationTurn>,
    val dateText: String? = null
)

object ConversationTurnBuilder {
    fun buildTurns(
        messages: List<GatewayMessageItem>,
        isRunning: Boolean = false,
        isAwaitingResponse: Boolean = false
    ): List<ConversationTurn> {
        if (messages.isEmpty()) return emptyList()

        val turns = mutableListOf<ConversationTurn>()
        var currentMessages = mutableListOf<GatewayMessageItem>()
        var currentUserMsg: GatewayMessageItem? = null
        var currentTurnIndex = 1

        fun finalizeCurrentTurn(isLastTurn: Boolean = false) {
            if (currentMessages.isEmpty()) return

            val userMsg = currentUserMsg
            val firstId = userMsg?.id?.takeIf { it.isNotBlank() }
                ?: currentMessages.firstOrNull()?.id?.takeIf { it.isNotBlank() }
                ?: "turn-$currentTurnIndex"
            val turnId = "turn-$firstId"

            val summary: String = run {
                if (userMsg != null) {
                    val (userBody, attachedFiles) = com.antigravity.mobile.data.service.AttachmentRules.parseBlock(userMsg.effectiveText)
                    val clean = userBody.trim()
                    val validImageUrls = userMsg.imageUrls?.filter { com.antigravity.mobile.data.service.FileIconResolver.isImage(it) }
                    if (clean.isNotEmpty()) {
                        val lines = clean.lines()
                            .map { it.trim() }
                            .filter { it.isNotEmpty() }
                        val combined = lines.take(2).joinToString(" ")
                        if (combined.length > 48) {
                            combined.take(45) + "..."
                        } else {
                            combined
                        }
                    } else if (attachedFiles.isNotEmpty()) {
                        val firstFile = attachedFiles.first().name
                        if (attachedFiles.size == 1) {
                            "[附件: $firstFile]"
                        } else {
                            "[附件: $firstFile 等 ${attachedFiles.size} 个文件]"
                        }
                    } else if (userMsg.effectiveImageDataList.isNotEmpty() || !validImageUrls.isNullOrEmpty()) {
                        val count = maxOf(userMsg.effectiveImageDataList.size, validImageUrls?.size ?: 0)
                        "[包含 $count 张图片]"
                    } else {
                        "用户消息"
                    }
                } else {
                    "会话开场与初始化"
                }
            }

            val totalTools = currentMessages.sumOf { it.toolCount ?: 0 }
            val uniqueTools = mutableListOf<String>()
            for (msg in currentMessages) {
                msg.toolNames?.forEach { name ->
                    if (name !in uniqueTools) uniqueTools.add(name)
                }
            }

            val uniqueArtifacts = mutableListOf<ArtifactItem>()
            for (msg in currentMessages) {
                msg.artifacts?.forEach { art ->
                    if (uniqueArtifacts.none { it.uri == art.uri }) uniqueArtifacts.add(art)
                }
            }

            val hasError = currentMessages.any { it.isError }
            val isActive = isLastTurn && (isRunning || isAwaitingResponse)

            val model = currentMessages.asReversed().firstNotNullOfOrNull { it.modelName ?: it.model }
            val turnTimestamp = userMsg?.timestamp ?: currentMessages.firstNotNullOfOrNull { it.timestamp }

            turns.add(
                ConversationTurn(
                    id = turnId,
                    turnIndex = currentTurnIndex,
                    userMessage = userMsg,
                    messages = currentMessages.toList(),
                    firstMessageId = firstId,
                    promptSummary = summary,
                    totalTools = totalTools,
                    toolNames = uniqueTools,
                    artifacts = uniqueArtifacts,
                    hasError = hasError,
                    isActive = isActive,
                    modelName = model,
                    timestamp = turnTimestamp
                )
            )

            currentTurnIndex++
            currentMessages = mutableListOf()
            currentUserMsg = null
        }

        for (message in messages) {
            if (message.isUser) {
                if (currentMessages.isNotEmpty()) {
                    finalizeCurrentTurn()
                }
                currentUserMsg = message
                currentMessages.add(message)
            } else {
                currentMessages.add(message)
            }
        }

        if (currentMessages.isNotEmpty()) {
            finalizeCurrentTurn(isLastTurn = true)
        }

        return turns
    }

    private fun parseTurnDate(str: String?): java.time.LocalDate? {
        if (str.isNullOrBlank()) return null
        return try {
            java.time.OffsetDateTime.parse(str).toLocalDate()
        } catch (_: Exception) {
            try {
                java.time.Instant.parse(str).atZone(java.time.ZoneId.systemDefault()).toLocalDate()
            } catch (_: Exception) {
                null
            }
        }
    }

    private fun formatDateLabel(date: java.time.LocalDate): String {
        val today = java.time.LocalDate.now()
        return when {
            date == today -> "今天"
            date == today.minusDays(1) -> "昨天"
            date.year == today.year -> "${date.monthValue}月${date.dayOfMonth}日"
            else -> "${date.year}年${date.monthValue}月${date.dayOfMonth}日"
        }
    }

    fun buildChapters(turns: List<ConversationTurn>, chunkSize: Int = 10): List<ConversationChapter> {
        if (turns.isEmpty()) return emptyList()

        // 尝试按日期跨度分组（跨天长会话自动激活日期章节）
        val turnsWithDates = turns.mapNotNull { turn ->
            val date = parseTurnDate(turn.timestamp)
            if (date != null) Triple(turn, date, date.toString()) else null
        }

        val uniqueDays = turnsWithDates.map { it.third }.toSet()
        if (uniqueDays.size > 1 && turnsWithDates.size == turns.size) {
            val dateChapters = mutableListOf<ConversationChapter>()
            var currentDayKey = ""
            var currentDayTurns = mutableListOf<ConversationTurn>()
            var currentDayDate: java.time.LocalDate? = null
            var chapterIdx = 1

            fun flushDay() {
                val d = currentDayDate ?: return
                if (currentDayTurns.isEmpty()) return
                val label = formatDateLabel(d)

                if (currentDayTurns.size <= 12) {
                    val firstTurn = currentDayTurns.firstOrNull()?.turnIndex ?: 1
                    val lastTurn = currentDayTurns.lastOrNull()?.turnIndex ?: firstTurn
                    val range = if (firstTurn == lastTurn) "第 ${firstTurn} 轮" else "第 ${firstTurn}–${lastTurn} 轮"
                    val title = "$label · $range"
                    dateChapters.add(
                        ConversationChapter(
                            id = "chapter-date-$chapterIdx",
                            title = title,
                            rangeText = range,
                            turns = currentDayTurns.toList(),
                            dateText = label
                        )
                    )
                    chapterIdx++
                } else {
                    // 单日超过 12 轮时，在日期内按 chunkSize 分段
                    var start = 0
                    var subIdx = 1
                    while (start < currentDayTurns.size) {
                        val end = minOf(start + chunkSize, currentDayTurns.size)
                        val subTurns = currentDayTurns.subList(start, end)
                        val firstTurn = subTurns.firstOrNull()?.turnIndex ?: (start + 1)
                        val lastTurn = subTurns.lastOrNull()?.turnIndex ?: end
                        val range = "第 ${firstTurn}–${lastTurn} 轮"
                        val title = "$label (第 $subIdx 段) · $range"
                        dateChapters.add(
                            ConversationChapter(
                                id = "chapter-date-$chapterIdx-$subIdx",
                                title = title,
                                rangeText = range,
                                turns = subTurns.toList(),
                                dateText = label
                            )
                        )
                        start = end
                        subIdx++
                    }
                    chapterIdx++
                }
                currentDayTurns = mutableListOf()
                currentDayDate = null
            }

            for (item in turnsWithDates) {
                if (item.third != currentDayKey) {
                    flushDay()
                    currentDayKey = item.third
                    currentDayDate = item.second
                }
                currentDayTurns.add(item.first)
            }
            flushDay()

            if (dateChapters.isNotEmpty()) {
                return dateChapters
            }
        }

        // 当总轮次不多时，直接作为一个主章节展示
        if (turns.size <= 12) {
            val range = if (turns.size == 1) "第 1 轮" else "第 1–${turns.size} 轮"
            val dateLabel = turns.firstOrNull()?.timestamp?.let { parseTurnDate(it) }?.let { formatDateLabel(it) }
            val title = if (dateLabel != null) "$dateLabel · $range" else "全部轮次"
            return listOf(ConversationChapter("chapter-all", title, range, turns, dateLabel))
        }

        // 超过 12 轮时，按 chunkSize 自动拆分为 Chapters
        val chapters = mutableListOf<ConversationChapter>()
        var startIndex = 0
        var chapterNum = 1

        while (startIndex < turns.size) {
            val endIndex = minOf(startIndex + chunkSize, turns.size)
            val chunkTurns = turns.subList(startIndex, endIndex)
            val firstTurn = chunkTurns.firstOrNull()?.turnIndex ?: (startIndex + 1)
            val lastTurn = chunkTurns.lastOrNull()?.turnIndex ?: endIndex

            val range = "第 ${firstTurn}–${lastTurn} 轮"
            val chunkDate = chunkTurns.firstOrNull()?.timestamp?.let { parseTurnDate(it) }?.let { formatDateLabel(it) }
            val title = if (chunkDate != null) "$chunkDate · 第 $chapterNum 章节 ($range)" else "第 $chapterNum 章节 ($range)"

            chapters.add(
                ConversationChapter(
                    id = "chapter-$chapterNum",
                    title = title,
                    rangeText = range,
                    turns = chunkTurns,
                    dateText = chunkDate
                )
            )

            startIndex = endIndex
            chapterNum++
        }

        return chapters
    }
}
