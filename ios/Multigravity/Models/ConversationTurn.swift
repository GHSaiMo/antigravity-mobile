import Foundation

/// 服务端返回的全会话轻量轮次目录索引（零正文、零 Thinking、零 Base64）
public struct TurnOutlineItem: Codable, Sendable, Hashable {
    public let turnIndex: Int
    public let firstMessageId: String
    public let messageOffset: Int
    public let messageCount: Int
    public let startStepIndex: Int?
    public let endStepIndex: Int?
    public let promptSummary: String
    public let totalTools: Int?
    public let artifacts: [ArtifactItem]?
    public let hasError: Bool?
    public let modelName: String?
    public let timestamp: String?
    public let isVoiceTurn: Bool?

    public init(
        turnIndex: Int,
        firstMessageId: String,
        messageOffset: Int,
        messageCount: Int,
        startStepIndex: Int? = nil,
        endStepIndex: Int? = nil,
        promptSummary: String,
        totalTools: Int? = nil,
        artifacts: [ArtifactItem]? = nil,
        hasError: Bool? = nil,
        modelName: String? = nil,
        timestamp: String? = nil,
        isVoiceTurn: Bool? = nil
    ) {
        self.turnIndex = turnIndex
        self.firstMessageId = firstMessageId
        self.messageOffset = messageOffset
        self.messageCount = messageCount
        self.startStepIndex = startStepIndex
        self.endStepIndex = endStepIndex
        self.promptSummary = promptSummary
        self.totalTools = totalTools
        self.artifacts = artifacts
        self.hasError = hasError
        self.modelName = modelName
        self.timestamp = timestamp
        self.isVoiceTurn = isVoiceTurn
    }
}

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
    /// 本轮结束的步骤索引（可选，用于未加载历史轮次的章节范围估算）
    public let endStepIndex: Int?
    /// 本轮首条消息在服务端全量消息列表中的 0-based 偏移（用于按需直达加载）
    public let messageOffset: Int?
    /// 本轮首条锚点消息是否已加载到本地消息流中
    public let isLoaded: Bool
    
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
        startStepIndex: Int? = nil,
        endStepIndex: Int? = nil,
        messageOffset: Int? = nil,
        isLoaded: Bool = true
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
        self.endStepIndex = endStepIndex
        self.messageOffset = messageOffset
        self.isLoaded = isLoaded
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
    /// 结合服务端全量轻量目录索引 `outline` 与本地已加载 `messages` 构建完整 1..N 轮次列表。
    /// 若 `outline` 为空（旧版网关、本地草稿或演示模式），自动退回仅基于 `messages` 构建。
    public static func buildTurns(
        from messages: [ChatMessage],
        outline: [TurnOutlineItem]?,
        isRunning: Bool = false,
        isAwaitingResponse: Bool = false
    ) -> [ConversationTurn] {
        guard let outline, !outline.isEmpty else {
            return buildTurns(from: messages, isRunning: isRunning, isAwaitingResponse: isAwaitingResponse)
        }

        var firstIdToIdx: [String: Int] = [:]
        firstIdToIdx.reserveCapacity(outline.count)
        for (idx, item) in outline.enumerated() where !item.firstMessageId.isEmpty {
            firstIdToIdx[item.firstMessageId] = idx
        }

        var firstMatchedMsgPos: Int? = nil
        var firstMatchedOutlineIdx: Int? = nil
        for (pos, msg) in messages.enumerated() {
            if let oIdx = firstIdToIdx[msg.id] {
                firstMatchedMsgPos = pos
                firstMatchedOutlineIdx = oIdx
                break
            }
        }

        var msgsByOutlineIdx: [Int: [ChatMessage]] = [:]
        var trailingOptimisticTurns: [[ChatMessage]] = []

        if let firstMatchPos = firstMatchedMsgPos, let firstOIdx = firstMatchedOutlineIdx {
            if firstMatchPos > 0 {
                let prefixIdx = max(0, firstOIdx - 1)
                msgsByOutlineIdx[prefixIdx, default: []].append(contentsOf: messages[0..<firstMatchPos])
            }
            var activeOutlineIdx: Int? = firstOIdx
            for pos in firstMatchPos..<messages.count {
                let msg = messages[pos]
                if let oIdx = firstIdToIdx[msg.id] {
                    activeOutlineIdx = oIdx
                    msgsByOutlineIdx[oIdx, default: []].append(msg)
                } else if msg.isUser && (msg.id.hasPrefix("optimistic-") || activeOutlineIdx == outline.count - 1) {
                    trailingOptimisticTurns.append([msg])
                    activeOutlineIdx = nil
                } else if let idx = activeOutlineIdx {
                    msgsByOutlineIdx[idx, default: []].append(msg)
                } else if !trailingOptimisticTurns.isEmpty {
                    trailingOptimisticTurns[trailingOptimisticTurns.count - 1].append(msg)
                }
            }
        } else if !messages.isEmpty {
            let lastOutlineIdx = outline.count - 1
            for msg in messages {
                if msg.isUser && msg.id.hasPrefix("optimistic-") {
                    trailingOptimisticTurns.append([msg])
                } else if !trailingOptimisticTurns.isEmpty {
                    trailingOptimisticTurns[trailingOptimisticTurns.count - 1].append(msg)
                } else {
                    msgsByOutlineIdx[lastOutlineIdx, default: []].append(msg)
                }
            }
        }

        var turns: [ConversationTurn] = []
        turns.reserveCapacity(outline.count + trailingOptimisticTurns.count)

        for (idx, item) in outline.enumerated() {
            let turnMessages = msgsByOutlineIdx[idx] ?? []
            let firstId = item.firstMessageId.isEmpty
                ? (turnMessages.first?.id ?? "turn-\(item.turnIndex)")
                : item.firstMessageId
            let turnId = "turn-\(firstId)"
            let isLoaded = turnMessages.contains(where: { $0.id == firstId })
            let userMsg = turnMessages.first(where: { $0.isUser })

            let parsed = userMsg.map { AttachmentRules.parseBlock($0.content) }
            let isVoice: Bool = {
                if let u = userMsg {
                    return VoiceTranscriptCleaner.isVoiceMessage(body: parsed?.body ?? u.content, files: parsed?.files ?? [])
                }
                return item.isVoiceTurn ?? false
            }()

            let summary: String = {
                if let u = userMsg {
                    return summarizeUserMessage(u, parsed: parsed, isVoice: isVoice)
                }
                switch item.promptSummary {
                case "会话开场与初始化":
                    return String(localized: "会话开场与初始化")
                case "用户消息":
                    return String(localized: "用户消息")
                default:
                    return item.promptSummary
                }
            }()

            let localTools = turnMessages.reduce(0) { $0 + $1.toolCount }
            let totalTools = isLoaded ? max(localTools, item.totalTools ?? 0) : (item.totalTools ?? localTools)

            var uniqueTools: [String] = []
            for msg in turnMessages {
                for name in msg.toolNames where !uniqueTools.contains(name) {
                    uniqueTools.append(name)
                }
            }

            var uniqueArtifacts: [ArtifactItem] = []
            for msg in turnMessages {
                for art in msg.artifacts where !uniqueArtifacts.contains(where: { $0.uri == art.uri }) {
                    uniqueArtifacts.append(art)
                }
            }
            if uniqueArtifacts.isEmpty, let outlineArts = item.artifacts {
                uniqueArtifacts = outlineArts
            }

            let hasError = (item.hasError ?? false) || turnMessages.contains(where: { $0.isError })
            let isLastOverall = (idx == outline.count - 1) && trailingOptimisticTurns.isEmpty
            let isActive = isLastOverall && (isRunning || isAwaitingResponse)

            let model = turnMessages.reversed().compactMap(\.modelName).first
                ?? turnMessages.reversed().compactMap(\.model).first
                ?? (item.modelName?.isEmpty == false ? item.modelName : nil)

            let turnTimestamp = userMsg?.timestamp
                ?? turnMessages.compactMap(\.timestamp).first
                ?? (item.timestamp?.isEmpty == false ? item.timestamp : nil)

            let startStep = item.startStepIndex
                ?? userMsg?.stepIndex
                ?? turnMessages.compactMap(\.stepIndex).min()

            let endStep = item.endStepIndex
                ?? turnMessages.compactMap(\.stepIndex).max()

            turns.append(ConversationTurn(
                id: turnId,
                turnIndex: item.turnIndex,
                userMessage: userMsg,
                messages: turnMessages,
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
                startStepIndex: startStep,
                endStepIndex: endStep,
                messageOffset: item.messageOffset,
                isLoaded: isLoaded
            ))
        }

        // Append any local optimistic turns not yet echoed in server outline
        var nextTurnIndex = (turns.last?.turnIndex ?? 0) + 1
        for (tIdx, optMsgs) in trailingOptimisticTurns.enumerated() {
            let builtOpt = buildTurns(
                from: optMsgs,
                isRunning: isRunning,
                isAwaitingResponse: isAwaitingResponse
            )
            for b in builtOpt {
                let isLastOpt = (tIdx == trailingOptimisticTurns.count - 1)
                turns.append(ConversationTurn(
                    id: b.id,
                    turnIndex: nextTurnIndex,
                    userMessage: b.userMessage,
                    messages: b.messages,
                    firstMessageId: b.firstMessageId,
                    promptSummary: b.promptSummary,
                    totalTools: b.totalTools,
                    toolNames: b.toolNames,
                    artifacts: b.artifacts,
                    hasError: b.hasError,
                    isActive: isLastOpt && (isRunning || isAwaitingResponse),
                    modelName: b.modelName,
                    timestamp: b.timestamp,
                    isVoiceTurn: b.isVoiceTurn,
                    startStepIndex: b.startStepIndex,
                    endStepIndex: b.endStepIndex,
                    messageOffset: nil,
                    isLoaded: true
                ))
                nextTurnIndex += 1
            }
        }

        return turns
    }

    private static func summarizeUserMessage(
        _ u: ChatMessage,
        parsed: (body: String, files: [AttachmentRules.ParsedFile])?,
        isVoice: Bool
    ) -> String {
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
            let firstFile = files.first?.name ?? String(localized: "file.singular", defaultValue: "文件")
            if files.count == 1 {
                return String(localized: "[附件: \(firstFile)]")
            } else {
                return String(localized: "[附件: \(firstFile) 等 \(files.count) 个文件]")
            }
        }
        if !u.imageDataList.isEmpty || !u.imageUrls.isEmpty {
            let count = max(u.imageDataList.count, u.imageUrls.count)
            return String(localized: "[包含 \(count) 张图片]")
        }
        return String(localized: "会话开场与初始化")
    }

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
                        let firstFile = files.first?.name ?? String(localized: "file.singular", defaultValue: "文件")
                        if files.count == 1 {
                            return String(localized: "[附件: \(firstFile)]")
                        } else {
                            return String(localized: "[附件: \(firstFile) 等 \(files.count) 个文件]")
                        }
                    }
                    if !u.imageDataList.isEmpty || !u.imageUrls.isEmpty {
                        let count = max(u.imageDataList.count, u.imageUrls.count)
                        return String(localized: "[包含 \(count) 张图片]")
                    }
                }
                // 无用户消息的开场轮
                return String(localized: "会话开场与初始化")
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
            let turnEndStep = currentMessages.compactMap(\.stepIndex).max()
            
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
                startStepIndex: turnStartStep,
                endStepIndex: turnEndStep
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
            return String(localized: "今天")
        } else if calendar.isDateInYesterday(date) {
            return String(localized: "昨天")
        }
        
        let nowYear = calendar.component(.year, from: Date())
        let dateYear = calendar.component(.year, from: date)
        let formatter = DateFormatter()
        if nowYear == dateYear {
            formatter.setLocalizedDateFormatFromTemplate("MMMd")
        } else {
            formatter.setLocalizedDateFormatFromTemplate("yyyyMMMd")
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
                if let maxStep = ch.turns.compactMap(\.endStepIndex).max()
                    ?? ch.turns.flatMap(\.messages).compactMap(\.stepIndex).max() {
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
                    let range = firstTurn == lastTurn ? String(localized: "第 \(firstTurn) 轮") : String(localized: "第 \(firstTurn)–\(lastTurn) 轮")
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
                        let range = String(localized: "第 \(firstTurn)–\(lastTurn) 轮")
                        let title = String(localized: "\(label) (第 \(subIdx) 段) · \(range)")
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
            let range = turns.count == 1 ? String(localized: "第 1 轮") : String(localized: "第 1–\(turns.count) 轮")
            let dateLabel = turns.first?.timestamp.flatMap(parseTurnDate).map(formatDateLabel)
            let title = dateLabel != nil ? "\(dateLabel!) · \(range)" : String(localized: "全部轮次")
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
            
            let range = String(localized: "第 \(firstTurn)–\(lastTurn) 轮")
            let chunkDate = chunkTurns.first?.timestamp.flatMap(parseTurnDate).map(formatDateLabel)
            let chapterPart = String(localized: "第 \(chapterNum) 章节 (\(range))")
            let title = chunkDate != nil ? "\(chunkDate!) · \(chapterPart)" : chapterPart
            
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
