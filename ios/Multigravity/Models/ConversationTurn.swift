import Foundation

/// 表示对话中的单轮问答（从用户输入提问开始，到下一个用户提问之前的所有 agent/tool 响应）
public struct ConversationTurn: Identifiable, Sendable, Hashable {
    public let id: String
    /// 轮次编号（从 1 开始：1, 2, 3...）
    public let turnIndex: Int
    /// 开启本轮的用户消息（如果存在）
    public let userMessage: ChatMessage?
    /// 本轮包含的所有完整消息
    public let messages: [ChatMessage]
    /// 滚动定位的目标消息 ID（优先定位用户消息，无用户消息时定位本轮首条）
    public let firstMessageId: String
    /// 提取的用户提问摘要（用于大纲导航，单行或精简前两行，约 40 字符）
    public let promptSummary: String
    /// 本轮调用的所有工具总数与名称
    public let totalTools: Int
    public let toolNames: [String]
    /// 本轮生成的所有产物
    public let artifacts: [ArtifactItem]
    /// 本轮是否存在错误
    public let hasError: Bool
    /// 本轮是否包含正在进行或等待中的任务
    public let isActive: Bool
    /// 本轮使用的模型名称（若有）
    public let modelName: String?
    /// 本轮创建时间戳 (RFC3339)
    public let timestamp: String?
    /// 本轮是否为语音交互提问
    public let isVoiceTurn: Bool
    /// 本轮起始的步骤索引（用于 Antigravity 2.30.0 GetChapterTitles 章节标题生成）
    public let startStepIndex: Int?
    
    public init(
        id: String,
        turnIndex: Int,
        userMessage: ChatMessage?,
        messages: [ChatMessage],
        firstMessageId: String,
        promptSummary: String,
        totalTools: Int,
        toolNames: [String],
        artifacts: [ArtifactItem],
        hasError: Bool,
        isActive: Bool,
        modelName: String?,
        timestamp: String? = nil,
        isVoiceTurn: Bool = false,
        startStepIndex: Int? = nil
    ) {
        self.id = id
        self.turnIndex = turnIndex
        self.userMessage = userMessage
        self.messages = messages
        self.firstMessageId = firstMessageId
        self.promptSummary = promptSummary
        self.totalTools = totalTools
        self.toolNames = toolNames
        self.artifacts = artifacts
        self.hasError = hasError
        self.isActive = isActive
        self.modelName = modelName
        self.timestamp = timestamp
        self.isVoiceTurn = isVoiceTurn
        self.startStepIndex = startStepIndex
    }
}

/// 超长对话的章节分段（对齐 Antigravity 2.30.0 GetChapterTitles 规范）
public struct ConversationChapter: Identifiable, Sendable, Hashable {
    public let id: String
    public let title: String
    public let subtitle: String?
    public let rangeText: String
    public let dateText: String?
    public let turns: [ConversationTurn]
    public let startStepIndex: Int?
    public let endStepIndexExclusive: Int?
    public let isAiTitled: Bool
    
    public init(
        id: String,
        title: String,
        subtitle: String? = nil,
        rangeText: String,
        dateText: String? = nil,
        turns: [ConversationTurn],
        startStepIndex: Int? = nil,
        endStepIndexExclusive: Int? = nil,
        isAiTitled: Bool = false
    ) {
        self.id = id
        self.title = title
        self.subtitle = subtitle
        self.rangeText = rangeText
        self.dateText = dateText
        self.turns = turns
        self.startStepIndex = startStepIndex
        self.endStepIndexExclusive = endStepIndexExclusive
        self.isAiTitled = isAiTitled
    }
}

public enum ConversationTurnBuilder {
    public static func buildTurns(from messages: [ChatMessage], isRunning: Bool = false, isAwaitingResponse: Bool = false) -> [ConversationTurn] {
        guard !messages.isEmpty else { return [] }
        
        var turns: [ConversationTurn] = []
        var currentMessages: [ChatMessage] = []
        var currentUserMsg: ChatMessage? = nil
        var currentTurnIndex = 1
        
        func finalizeCurrentTurn(isLastTurn: Bool = false) {
            guard !currentMessages.isEmpty else { return }
            
            let userMsg = currentUserMsg
            let firstId = userMsg?.id ?? currentMessages.first?.id ?? "turn-\(currentTurnIndex)"
            let turnId = "turn-\(firstId)"
            
            let parsed = userMsg.map { AttachmentRules.parseBlock($0.content) }
            let isVoice = userMsg.map { VoiceTranscriptCleaner.isVoiceMessage(body: parsed?.body ?? $0.content, files: parsed?.files ?? []) } ?? false
            
            // 提纯 Prompt 摘要
            let summary: String = {
                if let u = userMsg {
                    if isVoice {
                        let cleaned = VoiceTranscriptCleaner.clean(parsed?.body ?? u.content)
                        let cleanPrompt = cleaned
                            .replacingOccurrences(of: #"^(我|user|human|me)\s*[:：]\s*"#, with: "", options: [.regularExpression, .caseInsensitive])
                            .trimmingCharacters(in: .whitespacesAndNewlines)
                        let lines = cleanPrompt.components(separatedBy: .newlines)
                            .map { $0.trimmingCharacters(in: .whitespaces) }
                            .filter { !$0.isEmpty }
                        let combined = lines.prefix(2).joined(separator: " ")
                        if combined.count > 48 {
                            return String(combined.prefix(45)) + "..."
                        }
                        return combined
                    }
                    let clean = (parsed?.body ?? u.content).trimmingCharacters(in: .whitespacesAndNewlines)
                    if !clean.isEmpty {
                        let lines = clean.components(separatedBy: .newlines)
                            .map { $0.trimmingCharacters(in: .whitespaces) }
                            .filter { !$0.isEmpty }
                        let combined = lines.prefix(2).joined(separator: " ")
                        if combined.count > 48 {
                            return String(combined.prefix(45)) + "..."
                        }
                        return combined
                    }
                    if let files = parsed?.files, !files.isEmpty {
                        let firstFile = files.first?.name ?? "文件"
                        if files.count == 1 {
                            return "[附件: \(firstFile)]"
                        } else {
                            return "[附件: \(firstFile) 等 \(files.count) 个文件]"
                        }
                    }
                    if !u.imageDataList.isEmpty || !u.imageUrls.isEmpty {
                        let count = max(u.imageDataList.count, u.imageUrls.count)
                        return "[包含 \(count) 张图片]"
                    }
                }
                // 无用户消息的开场轮
                return "会话开场与初始化"
            }()
            
            let totalTools = currentMessages.reduce(0) { $0 + $1.toolCount }
            var uniqueTools: [String] = []
            for msg in currentMessages {
                for name in msg.toolNames where !uniqueTools.contains(name) {
                    uniqueTools.append(name)
                }
            }
            
            var uniqueArtifacts: [ArtifactItem] = []
            for msg in currentMessages {
                for art in msg.artifacts where !uniqueArtifacts.contains(where: { $0.uri == art.uri }) {
                    uniqueArtifacts.append(art)
                }
            }
            
            let hasError = currentMessages.contains(where: { $0.isError })
            let isActive = isLastTurn && (isRunning || isAwaitingResponse)
            
            let model = currentMessages.reversed().compactMap(\.modelName).first
                ?? currentMessages.reversed().compactMap(\.model).first
            
            let turnTimestamp = userMsg?.timestamp ?? currentMessages.compactMap(\.timestamp).first
            let turnStartStep = userMsg?.stepIndex ?? currentMessages.compactMap(\.stepIndex).min()
            
            turns.append(ConversationTurn(
                id: turnId,
                turnIndex: currentTurnIndex,
                userMessage: userMsg,
                messages: currentMessages,
                firstMessageId: firstId,
                promptSummary: summary,
                totalTools: totalTools,
                toolNames: uniqueTools,
                artifacts: uniqueArtifacts,
                hasError: hasError,
                isActive: isActive,
                modelName: model,
                timestamp: turnTimestamp,
                isVoiceTurn: isVoice,
                startStepIndex: turnStartStep
            ))
            
            currentTurnIndex += 1
            currentMessages = []
            currentUserMsg = nil
        }
        
        for message in messages {
            if message.isUser {
                // 如果之前已经有累积的消息，结算上一轮
                if !currentMessages.isEmpty {
                    finalizeCurrentTurn()
                }
                currentUserMsg = message
                currentMessages.append(message)
            } else {
                currentMessages.append(message)
            }
        }
        
        if !currentMessages.isEmpty {
            finalizeCurrentTurn(isLastTurn: true)
        }
        
        return turns
    }
    
    private static let isoFormatterWithFraction: ISO8601DateFormatter = {
        let f = ISO8601DateFormatter()
        f.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return f
    }()

    private static let isoFormatterBasic: ISO8601DateFormatter = {
        let f = ISO8601DateFormatter()
        f.formatOptions = [.withInternetDateTime]
        return f
    }()

    private static func parseTurnDate(from string: String?) -> Date? {
        guard let s = string, !s.isEmpty else { return nil }
        return isoFormatterWithFraction.date(from: s) ?? isoFormatterBasic.date(from: s)
    }

    private static func formatDateLabel(_ date: Date) -> String {
        let calendar = Calendar.current
        if calendar.isDateInToday(date) {
            return "今天"
        } else if calendar.isDateInYesterday(date) {
            return "昨天"
        }
        
        let nowYear = calendar.component(.year, from: Date())
        let dateYear = calendar.component(.year, from: date)
        let formatter = DateFormatter()
        if nowYear == dateYear {
            formatter.dateFormat = "M月d日"
        } else {
            formatter.dateFormat = "yyyy年M月d日"
        }
        return formatter.string(from: date)
    }

    public static func buildChapters(
        from turns: [ConversationTurn],
        chunkSize: Int = 6,
        aiTitles: [String: String] = [:]
    ) -> [ConversationChapter] {
        guard !turns.isEmpty else { return [] }
        let rawChapters = buildRawChapters(from: turns, chunkSize: max(chunkSize, 3))
        return enrichChaptersWithStepRangesAndAITitles(rawChapters, aiTitles: aiTitles)
    }

    private static func enrichChaptersWithStepRangesAndAITitles(
        _ raw: [ConversationChapter],
        aiTitles: [String: String]
    ) -> [ConversationChapter] {
        guard !raw.isEmpty else { return [] }

        let starts: [Int?] = raw.enumerated().map { idx, ch in
            let explicit = ch.turns.compactMap(\.startStepIndex).min()
                ?? ch.turns.flatMap(\.messages).compactMap(\.stepIndex).min()
            if let s = explicit { return s }
            return idx == 0 ? 0 : nil
        }

        return raw.enumerated().map { idx, ch in
            let s = starts[idx]
            let e: Int? = {
                if idx + 1 < raw.count, let nextStart = starts[idx + 1] {
                    return nextStart
                }
                if let maxStep = ch.turns.flatMap(\.messages).compactMap(\.stepIndex).max() {
                    return maxStep + 1
                }
                return nil
            }()

            var resolvedTitle = ch.title
            var resolvedSubtitle: String? = ch.subtitle
            var isAI = false

            if let s = s {
                let exactKey = e.map { "\(s):\($0)" }
                let startKey = "start:\(s)"
                let matched = (exactKey.flatMap { aiTitles[$0] }) ?? aiTitles[startKey]
                if let ai = matched?.trimmingCharacters(in: .whitespacesAndNewlines), !ai.isEmpty {
                    resolvedTitle = ai
                    resolvedSubtitle = ch.title
                    isAI = true
                }
            }

            return ConversationChapter(
                id: ch.id,
                title: resolvedTitle,
                subtitle: resolvedSubtitle,
                rangeText: ch.rangeText,
                dateText: ch.dateText,
                turns: ch.turns,
                startStepIndex: s,
                endStepIndexExclusive: e,
                isAiTitled: isAI
            )
        }
    }

    private static func buildRawChapters(from turns: [ConversationTurn], chunkSize: Int) -> [ConversationChapter] {
        guard !turns.isEmpty else { return [] }
        
        // 尝试按日期跨度分组（跨天长会话自动激活日期章节）
        var turnsWithDates: [(turn: ConversationTurn, date: Date, dayKey: String)] = []
        let dayKeyFormatter = DateFormatter()
        dayKeyFormatter.dateFormat = "yyyy-MM-dd"
        
        for turn in turns {
            if let date = parseTurnDate(from: turn.timestamp) {
                turnsWithDates.append((turn, date, dayKeyFormatter.string(from: date)))
            }
        }
        
        // 如果有时间戳且存在跨天（多个不同日期）
        let uniqueDays = Set(turnsWithDates.map(\.dayKey))
        if uniqueDays.count > 1 && turnsWithDates.count == turns.count {
            var dateChapters: [ConversationChapter] = []
            var currentDayKey = ""
            var currentDayTurns: [ConversationTurn] = []
            var currentDayDate: Date? = nil
            var chapterIdx = 1
            
            func flushDay() {
                guard !currentDayTurns.isEmpty, let d = currentDayDate else { return }
                let label = formatDateLabel(d)
                
                if currentDayTurns.count <= max(chunkSize, 8) {
                    let firstTurn = currentDayTurns.first?.turnIndex ?? 1
                    let lastTurn = currentDayTurns.last?.turnIndex ?? firstTurn
                    let range = firstTurn == lastTurn ? "第 \(firstTurn) 轮" : "第 \(firstTurn)–\(lastTurn) 轮"
                    let title = "\(label) · \(range)"
                    dateChapters.append(ConversationChapter(
                        id: "chapter-date-\(chapterIdx)",
                        title: title,
                        rangeText: range,
                        dateText: label,
                        turns: currentDayTurns
                    ))
                    chapterIdx += 1
                } else {
                    // 单日轮数较多时，在日期内按 chunkSize 分段
                    var start = 0
                    var subIdx = 1
                    while start < currentDayTurns.count {
                        let end = min(start + chunkSize, currentDayTurns.count)
                        let subTurns = Array(currentDayTurns[start..<end])
                        let firstTurn = subTurns.first?.turnIndex ?? (start + 1)
                        let lastTurn = subTurns.last?.turnIndex ?? end
                        let range = "第 \(firstTurn)–\(lastTurn) 轮"
                        let title = "\(label) (第 \(subIdx) 段) · \(range)"
                        dateChapters.append(ConversationChapter(
                            id: "chapter-date-\(chapterIdx)-\(subIdx)",
                            title: title,
                            rangeText: range,
                            dateText: label,
                            turns: subTurns
                        ))
                        start = end
                        subIdx += 1
                    }
                    chapterIdx += 1
                }
                currentDayTurns = []
                currentDayDate = nil
            }
            
            for item in turnsWithDates {
                if item.dayKey != currentDayKey {
                    flushDay()
                    currentDayKey = item.dayKey
                    currentDayDate = item.date
                }
                currentDayTurns.append(item.turn)
            }
            flushDay()
            
            if !dateChapters.isEmpty {
                return dateChapters
            }
        }
        
        // 当总轮次不多时，直接作为一个主章节展示
        if turns.count <= chunkSize {
            let range = turns.count == 1 ? "第 1 轮" : "第 1–\(turns.count) 轮"
            let dateLabel = turns.first?.timestamp.flatMap(parseTurnDate).map(formatDateLabel)
            let title = dateLabel != nil ? "\(dateLabel!) · \(range)" : "全部轮次"
            return [ConversationChapter(id: "chapter-all", title: title, rangeText: range, dateText: dateLabel, turns: turns)]
        }
        
        // 超过 chunkSize 轮时，按 chunkSize 自动拆分为 Chapters
        var chapters: [ConversationChapter] = []
        var startIndex = 0
        var chapterNum = 1
        
        while startIndex < turns.count {
            let endIndex = min(startIndex + chunkSize, turns.count)
            let chunkTurns = Array(turns[startIndex..<endIndex])
            let firstTurn = chunkTurns.first?.turnIndex ?? (startIndex + 1)
            let lastTurn = chunkTurns.last?.turnIndex ?? endIndex
            
            let range = "第 \(firstTurn)–\(lastTurn) 轮"
            let chunkDate = chunkTurns.first?.timestamp.flatMap(parseTurnDate).map(formatDateLabel)
            let title = chunkDate != nil ? "\(chunkDate!) · 第 \(chapterNum) 章节 (\(range))" : "第 \(chapterNum) 章节 (\(range))"
            
            chapters.append(ConversationChapter(
                id: "chapter-\(chapterNum)",
                title: title,
                rangeText: range,
                dateText: chunkDate,
                turns: chunkTurns
            ))
            
            startIndex = endIndex
            chapterNum += 1
        }
        
        return chapters
    }
}
