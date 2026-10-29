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
    val modelName: String?
)

/**
 * 超长对话的章节分段（对齐 Antigravity 2.28.1 Chapters 规范）
 */
data class ConversationChapter(
    val id: String,
    val title: String,
    val rangeText: String,
    val turns: List<ConversationTurn>
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
                    val clean = userMsg.effectiveText.trim()
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
                    } else if (userMsg.effectiveImageDataList.isNotEmpty() || !userMsg.imageUrls.isNullOrEmpty()) {
                        val count = maxOf(userMsg.effectiveImageDataList.size, userMsg.imageUrls?.size ?: 0)
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
                    modelName = model
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

    fun buildChapters(turns: List<ConversationTurn>, chunkSize: Int = 10): List<ConversationChapter> {
        if (turns.isEmpty()) return emptyList()

        if (turns.size <= 12) {
            val range = if (turns.size == 1) "第 1 轮" else "第 1–${turns.size} 轮"
            return listOf(ConversationChapter("chapter-all", "全部轮次", range, turns))
        }

        val chapters = mutableListOf<ConversationChapter>()
        var startIndex = 0
        var chapterNum = 1

        while (startIndex < turns.size) {
            val endIndex = minOf(startIndex + chunkSize, turns.size)
            val chunkTurns = turns.subList(startIndex, endIndex)
            val firstTurn = chunkTurns.firstOrNull()?.turnIndex ?: (startIndex + 1)
            val lastTurn = chunkTurns.lastOrNull()?.turnIndex ?: endIndex

            val range = "第 ${firstTurn}–${lastTurn} 轮"
            val title = "第 $chapterNum 章节 ($range)"

            chapters.add(
                ConversationChapter(
                    id = "chapter-$chapterNum",
                    title = title,
                    rangeText = range,
                    turns = chunkTurns
                )
            )

            startIndex = endIndex
            chapterNum++
        }

        return chapters
    }
}
