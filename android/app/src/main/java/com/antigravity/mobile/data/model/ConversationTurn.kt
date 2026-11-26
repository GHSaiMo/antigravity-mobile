package com.antigravity.mobile.data.model

import com.antigravity.mobile.ui.util.loc
import com.antigravity.mobile.ui.components.VoiceTranscriptCleaner

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
    val timestamp: String? = null,
    /** 本轮是否为语音交互提问 */
    val isVoiceTurn: Boolean = false,
    /** 本轮是否为定时任务自动下发的首条提示词 */
    val isScheduledTask: Boolean = false,
    /** 本轮起始的步骤索引（用于 Antigravity 2.30.0 GetChapterTitles 章节标题生成） */
    val startStepIndex: Int? = null,
    /** 本轮结束的步骤索引（可选，用于未加载历史轮次的章节范围估算） */
    val endStepIndex: Int? = null,
    /** 本轮首条消息在服务端全量消息列表中的 0-based 偏移（用于按需直达加载） */
    val messageOffset: Int? = null,
    /** 本轮首条锚点消息是否已加载到本地消息流中 */
    val isLoaded: Boolean = true
)

/**
 * 超长对话的章节分段（对齐 Antigravity 2.30.0 GetChapterTitles 规范）
 */
data class ConversationChapter(
    val id: String,
    val title: String,
    val rangeText: String,
    val turns: List<ConversationTurn>,
    val dateText: String? = null,
    val subtitle: String? = null,
    val startStepIndex: Int? = null,
    val endStepIndexExclusive: Int? = null,
    val isAiTitled: Boolean = false
)

object ConversationTurnBuilder {
    /**
     * 结合服务端全量轻量目录索引 `outline` 与本地已加载 `messages` 构建完整 1..N 轮次列表。
     * 若 `outline` 为空（旧版网关或本地草稿），自动退回仅基于 `messages` 构建。
     */
    fun buildTurns(
        messages: List<GatewayMessageItem>,
        outline: List<TurnOutlineItem>?,
        isRunning: Boolean = false,
        isAwaitingResponse: Boolean = false
    ): List<ConversationTurn> {
        if (outline.isNullOrEmpty()) {
            return buildTurns(messages, isRunning, isAwaitingResponse)
        }

        val firstIdToIdx = HashMap<String, Int>(outline.size)
        outline.forEachIndexed { idx, item ->
            if (item.firstMessageId.isNotBlank()) {
                firstIdToIdx[item.firstMessageId] = idx
            }
        }

        var firstMatchedMsgPos: Int? = null
        var firstMatchedOutlineIdx: Int? = null
        for (pos in messages.indices) {
            val oIdx = firstIdToIdx[messages[pos].id]
            if (oIdx != null) {
                firstMatchedMsgPos = pos
                firstMatchedOutlineIdx = oIdx
                break
            }
        }

        val msgsByOutlineIdx = HashMap<Int, MutableList<GatewayMessageItem>>()
        val trailingOptimisticTurns = mutableListOf<MutableList<GatewayMessageItem>>()

        if (firstMatchedMsgPos != null && firstMatchedOutlineIdx != null) {
            if (firstMatchedMsgPos > 0) {
                val prefixIdx = (firstMatchedOutlineIdx - 1).coerceAtLeast(0)
                val bucket = msgsByOutlineIdx.getOrPut(prefixIdx) { mutableListOf() }
                for (p in 0 until firstMatchedMsgPos) {
                    bucket.add(messages[p])
                }
            }
            var activeOutlineIdx: Int? = firstMatchedOutlineIdx
            for (pos in firstMatchedMsgPos until messages.size) {
                val msg = messages[pos]
                val oIdx = firstIdToIdx[msg.id]
                if (oIdx != null) {
                    activeOutlineIdx = oIdx
                    msgsByOutlineIdx.getOrPut(oIdx) { mutableListOf() }.add(msg)
                } else if (msg.isUser && (msg.id.startsWith("opt_") || msg.id.startsWith("optimistic-") || activeOutlineIdx == outline.size - 1)) {
                    trailingOptimisticTurns.add(mutableListOf(msg))
                    activeOutlineIdx = null
                } else if (activeOutlineIdx != null) {
                    msgsByOutlineIdx.getOrPut(activeOutlineIdx) { mutableListOf() }.add(msg)
                } else if (trailingOptimisticTurns.isNotEmpty()) {
                    trailingOptimisticTurns.last().add(msg)
                }
            }
        } else if (messages.isNotEmpty()) {
            val lastOutlineIdx = outline.size - 1
            for (msg in messages) {
                if (msg.isUser && (msg.id.startsWith("opt_") || msg.id.startsWith("optimistic-"))) {
                    trailingOptimisticTurns.add(mutableListOf(msg))
                } else if (trailingOptimisticTurns.isNotEmpty()) {
                    trailingOptimisticTurns.last().add(msg)
                } else {
                    msgsByOutlineIdx.getOrPut(lastOutlineIdx) { mutableListOf() }.add(msg)
                }
            }
        }

        val turns = ArrayList<ConversationTurn>(outline.size + trailingOptimisticTurns.size)

        outline.forEachIndexed { idx, item ->
            val turnMessages = msgsByOutlineIdx[idx]?.toList() ?: emptyList()
            val firstId = item.firstMessageId.takeIf { it.isNotBlank() }
                ?: turnMessages.firstOrNull()?.id?.takeIf { it.isNotBlank() }
                ?: "turn-${item.turnIndex}"
            val turnId = "turn-$firstId"
            val isLoaded = turnMessages.any { it.id == firstId }
            val userMsg = turnMessages.firstOrNull { it.isUser }

            val (userBody, attachedFiles) = if (userMsg != null) {
                com.antigravity.mobile.data.service.AttachmentRules.parseBlock(userMsg.effectiveText)
            } else {
                "" to emptyList()
            }
            val isVoice = if (userMsg != null) {
                VoiceTranscriptCleaner.isVoiceMessage(userBody, attachedFiles)
            } else {
                item.isVoiceTurn
            }

            val summary = if (userMsg != null) {
                summarizeUserMessage(userMsg, userBody, attachedFiles, isVoice)
            } else {
                when (item.promptSummary) {
                    "会话开场与初始化" -> "会话开场与初始化".loc
                    "用户消息" -> "用户消息".loc
                    else -> item.promptSummary
                }
            }

            val localTools = turnMessages.sumOf { it.toolCount ?: 0 }
            val totalTools = if (isLoaded) maxOf(localTools, item.totalTools) else if (item.totalTools > 0) item.totalTools else localTools

            val uniqueTools = mutableListOf<String>()
            for (msg in turnMessages) {
                msg.toolNames?.forEach { name ->
                    if (name !in uniqueTools) uniqueTools.add(name)
                }
            }

            val uniqueArtifacts = mutableListOf<ArtifactItem>()
            for (msg in turnMessages) {
                msg.artifacts?.forEach { art ->
                    if (uniqueArtifacts.none { it.uri == art.uri }) uniqueArtifacts.add(art)
                }
            }
            if (uniqueArtifacts.isEmpty() && !item.artifacts.isNullOrEmpty()) {
                uniqueArtifacts.addAll(item.artifacts)
            }

            val hasError = item.hasError || turnMessages.any { it.isError }
            val isLastOverall = (idx == outline.size - 1) && trailingOptimisticTurns.isEmpty()
            val isActive = isLastOverall && (isRunning || isAwaitingResponse)

            val model = turnMessages.asReversed().firstNotNullOfOrNull { msg ->
                msg.modelName?.takeIf { it.isNotBlank() } ?: msg.model?.takeIf { it.isNotBlank() }?.let { ModelDefaults.displayName(it) }
            } ?: item.modelName?.takeIf { it.isNotBlank() }

            val turnTimestamp = userMsg?.timestamp
                ?: turnMessages.firstNotNullOfOrNull { it.timestamp }
                ?: item.timestamp?.takeIf { it.isNotBlank() }

            val startStep = item.startStepIndex
                ?: userMsg?.stepIndex
                ?: turnMessages.mapNotNull { it.stepIndex }.minOrNull()

            val endStep = item.endStepIndex
                ?: turnMessages.mapNotNull { it.stepIndex }.maxOrNull()

            turns.add(
                ConversationTurn(
                    id = turnId,
                    turnIndex = item.turnIndex,
                    userMessage = userMsg,
                    messages = turnMessages,
                    firstMessageId = firstId,
                    promptSummary = summary,
                    totalTools = totalTools,
                    toolNames = uniqueTools,
                    artifacts = uniqueArtifacts,
                    hasError = hasError,
                    isActive = isActive,
                    modelName = model,
                    timestamp = turnTimestamp,
                    isVoiceTurn = isVoice,
                    isScheduledTask = !isVoice && ((userMsg?.isScheduledTask == true) || item.isScheduledTask),
                    startStepIndex = startStep,
                    endStepIndex = endStep,
                    messageOffset = item.messageOffset,
                    isLoaded = isLoaded
                )
            )
        }

        var nextTurnIndex = (turns.lastOrNull()?.turnIndex ?: 0) + 1
        trailingOptimisticTurns.forEachIndexed { tIdx, optMsgs ->
            val builtOpt = buildTurns(optMsgs, isRunning, isAwaitingResponse)
            val isLastOpt = (tIdx == trailingOptimisticTurns.size - 1)
            for (b in builtOpt) {
                turns.add(
                    b.copy(
                        turnIndex = nextTurnIndex,
                        isActive = isLastOpt && (isRunning || isAwaitingResponse),
                        messageOffset = null,
                        isLoaded = true
                    )
                )
                nextTurnIndex++
            }
        }

        return turns
    }

    private fun summarizeUserMessage(
        userMsg: GatewayMessageItem,
        userBody: String,
        attachedFiles: List<com.antigravity.mobile.data.service.AttachmentRules.ParsedFile>,
        isVoice: Boolean
    ): String {
        if (isVoice) {
            val cleaned = VoiceTranscriptCleaner.clean(userBody)
            val cleanPrompt = cleaned
                .replace(Regex("^(我|user|human|me)\\s*[:：]\\s*", RegexOption.IGNORE_CASE), "")
                .trim()
            val lines = cleanPrompt.lines().map { it.trim() }.filter { it.isNotEmpty() }
            val combined = lines.take(2).joinToString(" ")
            return if (combined.length > 48) combined.take(45) + "..." else combined
        }
        val clean = userBody.trim()
        val validImageUrls = userMsg.imageUrls?.filter { com.antigravity.mobile.data.service.FileIconResolver.isImage(it) }
        return if (clean.isNotEmpty()) {
            val lines = clean.lines().map { it.trim() }.filter { it.isNotEmpty() }
            val combined = lines.take(2).joinToString(" ")
            if (combined.length > 48) combined.take(45) + "..." else combined
        } else if (attachedFiles.isNotEmpty()) {
            val firstFile = attachedFiles.first().name
            if (attachedFiles.size == 1) "[附件: %s]".loc(firstFile) else "[附件: %1\$s 等 %2\$d 个文件]".loc(firstFile, attachedFiles.size)
        } else if (userMsg.effectiveImageDataList.isNotEmpty() || !validImageUrls.isNullOrEmpty()) {
            val count = maxOf(userMsg.effectiveImageDataList.size, validImageUrls?.size ?: 0)
            "[包含 $count 张图片]".loc
        } else {
            "用户消息".loc
        }
    }

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

            val (userBody, attachedFiles) = if (userMsg != null) {
                com.antigravity.mobile.data.service.AttachmentRules.parseBlock(userMsg.effectiveText)
            } else {
                "" to emptyList()
            }
            val isVoice = userMsg != null && VoiceTranscriptCleaner.isVoiceMessage(userBody, attachedFiles)

            val summary: String = run {
                if (userMsg != null) {
                    if (isVoice) {
                        val cleaned = VoiceTranscriptCleaner.clean(userBody)
                        val cleanPrompt = cleaned
                            .replace(Regex("^(我|user|human|me)\\s*[:：]\\s*", RegexOption.IGNORE_CASE), "")
                            .trim()
                        val lines = cleanPrompt.lines().map { it.trim() }.filter { it.isNotEmpty() }
                        val combined = lines.take(2).joinToString(" ")
                        if (combined.length > 48) combined.take(45) + "..." else combined
                    } else {
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
                                "[附件: %s]".loc(firstFile)
                            } else {
                                "[附件: %1\$s 等 %2\$d 个文件]".loc(firstFile, attachedFiles.size)
                            }
                        } else if (userMsg.effectiveImageDataList.isNotEmpty() || !validImageUrls.isNullOrEmpty()) {
                            val count = maxOf(userMsg.effectiveImageDataList.size, validImageUrls?.size ?: 0)
                            "[包含 $count 张图片]".loc
                        } else {
                            "用户消息".loc
                        }
                    }
                } else {
                    "会话开场与初始化".loc
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

            val model = currentMessages.asReversed().firstNotNullOfOrNull { msg ->
                msg.modelName?.takeIf { it.isNotBlank() } ?: msg.model?.takeIf { it.isNotBlank() }?.let { ModelDefaults.displayName(it) }
            }
            val turnTimestamp = userMsg?.timestamp ?: currentMessages.firstNotNullOfOrNull { it.timestamp }
            val turnStartStep = userMsg?.stepIndex ?: currentMessages.mapNotNull { it.stepIndex }.minOrNull()
            val turnEndStep = currentMessages.mapNotNull { it.stepIndex }.maxOrNull()

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
                    timestamp = turnTimestamp,
                    isVoiceTurn = isVoice,
                    isScheduledTask = !isVoice && (userMsg?.isScheduledTask == true),
                    startStepIndex = turnStartStep,
                    endStepIndex = turnEndStep
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
            date == today -> "今天".loc
            date == today.minusDays(1) -> "昨天".loc
            date.year == today.year -> "%1\$d月%2\$d日".loc(date.monthValue, date.dayOfMonth)
            else -> "%1\$d年%2\$d月%3\$d日".loc(date.year, date.monthValue, date.dayOfMonth)
        }
    }

    fun buildChapters(
        turns: List<ConversationTurn>,
        chunkSize: Int = 6,
        aiTitles: Map<String, String> = emptyMap()
    ): List<ConversationChapter> {
        if (turns.isEmpty()) return emptyList()
        val raw = buildRawChapters(turns, chunkSize.coerceAtLeast(3))
        return enrichChaptersWithStepRangesAndAiTitles(raw, aiTitles)
    }

    private fun enrichChaptersWithStepRangesAndAiTitles(
        raw: List<ConversationChapter>,
        aiTitles: Map<String, String>
    ): List<ConversationChapter> {
        if (raw.isEmpty()) return emptyList()

        val starts: List<Int?> = raw.mapIndexed { idx, ch ->
            ch.turns.mapNotNull { it.startStepIndex }.minOrNull()
                ?: ch.turns.flatMap { it.messages }.mapNotNull { it.stepIndex }.minOrNull()
                ?: if (idx == 0) 0 else null
        }

        return raw.mapIndexed { idx, ch ->
            val s = starts[idx]
            val e: Int? = if (idx + 1 < raw.size && starts[idx + 1] != null) {
                starts[idx + 1]
            } else {
                (ch.turns.mapNotNull { it.endStepIndex }.maxOrNull()
                    ?: ch.turns.flatMap { it.messages }.mapNotNull { it.stepIndex }.maxOrNull())?.let { it + 1 }
            }

            var resolvedTitle = ch.title
            var resolvedSubtitle: String? = ch.subtitle
            var isAi = false

            if (s != null) {
                val exactKey = e?.let { "$s:$it" }
                val startKey = "start:$s"
                val matched = (exactKey?.let { aiTitles[it] } ?: aiTitles[startKey])?.trim()
                if (!matched.isNullOrEmpty()) {
                    resolvedTitle = matched
                    resolvedSubtitle = ch.title
                    isAi = true
                }
            }

            ch.copy(
                title = resolvedTitle,
                subtitle = resolvedSubtitle,
                startStepIndex = s,
                endStepIndexExclusive = e,
                isAiTitled = isAi
            )
        }
    }

    private fun buildRawChapters(turns: List<ConversationTurn>, chunkSize: Int): List<ConversationChapter> {
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

                if (currentDayTurns.size <= maxOf(chunkSize, 8)) {
                    val firstTurn = currentDayTurns.firstOrNull()?.turnIndex ?: 1
                    val lastTurn = currentDayTurns.lastOrNull()?.turnIndex ?: firstTurn
                    val range = if (firstTurn == lastTurn) "第 ${firstTurn} 轮".loc else "第 %1\$d–%2\$d 轮".loc(firstTurn, lastTurn)
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
                    // 单日轮数较多时，在日期内按 chunkSize 分段
                    var start = 0
                    var subIdx = 1
                    while (start < currentDayTurns.size) {
                        val end = minOf(start + chunkSize, currentDayTurns.size)
                        val subTurns = currentDayTurns.subList(start, end)
                        val firstTurn = subTurns.firstOrNull()?.turnIndex ?: (start + 1)
                        val lastTurn = subTurns.lastOrNull()?.turnIndex ?: end
                        val range = "第 %1\$d–%2\$d 轮".loc(firstTurn, lastTurn)
                        val title = "%1\$s (第 %2\$d 段) · %3\$s".loc(label, subIdx, range)
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
        if (turns.size <= chunkSize) {
            val range = if (turns.size == 1) "第 1 轮".loc else "第 %1\$d–%2\$d 轮".loc(1, turns.size)
            val dateLabel = turns.firstOrNull()?.timestamp?.let { parseTurnDate(it) }?.let { formatDateLabel(it) }
            val title = if (dateLabel != null) "$dateLabel · $range" else "全部轮次".loc
            return listOf(ConversationChapter("chapter-all", title, range, turns, dateLabel))
        }

        // 超过 chunkSize 轮时，按 chunkSize 自动拆分为 Chapters
        val chapters = mutableListOf<ConversationChapter>()
        var startIndex = 0
        var chapterNum = 1

        while (startIndex < turns.size) {
            val endIndex = minOf(startIndex + chunkSize, turns.size)
            val chunkTurns = turns.subList(startIndex, endIndex)
            val firstTurn = chunkTurns.firstOrNull()?.turnIndex ?: (startIndex + 1)
            val lastTurn = chunkTurns.lastOrNull()?.turnIndex ?: endIndex

            val range = "第 %1\$d–%2\$d 轮".loc(firstTurn, lastTurn)
            val chunkDate = chunkTurns.firstOrNull()?.timestamp?.let { parseTurnDate(it) }?.let { formatDateLabel(it) }
            val title = if (chunkDate != null) "%1\$s · 第 %2\$d 章节 (%3\$s)".loc(chunkDate, chapterNum, range) else "第 %1\$d 章节 (%2\$s)".loc(chapterNum, range)

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
