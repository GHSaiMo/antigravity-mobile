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
        modelName: String?
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
    }
}

/// 超长对话的章节分段（对齐 Antigravity 2.28.1 Chapters 规范）
public struct ConversationChapter: Identifiable, Sendable, Hashable {
    public let id: String
    public let title: String
    public let rangeText: String
    public let turns: [ConversationTurn]
    
    public init(id: String, title: String, rangeText: String, turns: [ConversationTurn]) {
        self.id = id
        self.title = title
        self.rangeText = rangeText
        self.turns = turns
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
            
            // 提纯 Prompt 摘要
            let summary: String = {
                if let u = userMsg {
                    let clean = u.content.trimmingCharacters(in: .whitespacesAndNewlines)
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
                modelName: model
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
    
    public static func buildChapters(from turns: [ConversationTurn], chunkSize: Int = 10) -> [ConversationChapter] {
        guard !turns.isEmpty else { return [] }
        
        // 当总轮次不多时，直接作为一个主章节展示
        if turns.count <= 12 {
            let range = turns.count == 1 ? "第 1 轮" : "第 1–\(turns.count) 轮"
            return [ConversationChapter(id: "chapter-all", title: "全部轮次", rangeText: range, turns: turns)]
        }
        
        // 超过 12 轮时，按 chunkSize 自动拆分为 Chapters
        var chapters: [ConversationChapter] = []
        var startIndex = 0
        var chapterNum = 1
        
        while startIndex < turns.count {
            let endIndex = min(startIndex + chunkSize, turns.count)
            let chunkTurns = Array(turns[startIndex..<endIndex])
            let firstTurn = chunkTurns.first?.turnIndex ?? (startIndex + 1)
            let lastTurn = chunkTurns.last?.turnIndex ?? endIndex
            
            let range = "第 \(firstTurn)–\(lastTurn) 轮"
            let title = "第 \(chapterNum) 章节 (\(range))"
            
            chapters.append(ConversationChapter(
                id: "chapter-\(chapterNum)",
                title: title,
                rangeText: range,
                turns: chunkTurns
            ))
            
            startIndex = endIndex
            chapterNum += 1
        }
        
        return chapters
    }
}
