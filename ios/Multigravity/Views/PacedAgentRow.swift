import SwiftUI

/// 流式输出的“匀速播放”：网关推送的文本增量可能一次来很多（网络好时一帧几百字），
/// 直接渲染会让内容疯狂上涨、自动跟随滚动飞快，什么都看不清。
/// 这里把 Agent 消息文本的显示进度和网络到达速度解耦，按稳定的速度逐步追上目标文本：
/// 1. 内部按 O(1) UTF-8 偏移推进并自动对齐到完整 Unicode 字素与已到达的 Markdown 链接/行首标记边界，
///    绝不把已经到齐的 `[文件名](file:///...)` 故意切碎成 20 帧撑高又坍缩；
/// 2. 当外层已挂载 `ChatScrollBottomPinner` 时，滚动跟随由 `UIScrollView.contentSize` KVO 同帧原子完成，
///    不再每 33ms 盲目派发异步 `proxy.scrollTo`。
struct PacedAgentRow<Content: View>: View {
    let message: ChatMessage
    let isActive: Bool
    let scrollHUD: ChatScrollHUDState
    /// 每次显示长度增长后的兜底回调（仅当无法挂接底层 UIScrollView 时触发）。
    let onGrow: () -> Void
    @ViewBuilder let content: (ChatMessage, Bool) -> Content

    @State private var shownUTF8: Int = -1
    @State private var targetUTF8: Int = 0
    @State private var latestContent: String = ""
    @State private var ticker: Task<Void, Never>?

    private static var tickNanos: UInt64 { 33_000_000 }
    /// 积压按此帧数追平：越大越平缓。
    private static var catchUpTicks: Int { 18 }

    var body: some View {
        let full = message.content
        let totalUTF8 = full.utf8.count
        let isPacing = shownUTF8 >= 0 && shownUTF8 < totalUTF8
        let visible: ChatMessage = {
            guard isPacing else { return message }
            let endIdx = Self.clampedCharacterIndex(utf8Offset: shownUTF8, in: full)
            return message.withContent(String(full[..<endIdx]))
        }()
        content(visible, isActive || isPacing)
            .onAppear { sync(full: full, totalUTF8: totalUTF8, initial: true) }
            .onChange(of: totalUTF8) { _, newTotal in sync(full: message.content, totalUTF8: newTotal, initial: false) }
            .onDisappear {
                ticker?.cancel()
                ticker = nil
                scrollHUD.isPacingActive = false
                shownUTF8 = -1
                latestContent = ""
            }
    }

    private func sync(full: String, totalUTF8: Int, initial: Bool) {
        latestContent = full
        targetUTF8 = totalUTF8
        if initial || shownUTF8 < 0 {
            // 首次出现（含历史加载、列表回收后重现）直接显示完整内容，只对之后的增长做节奏控制。
            shownUTF8 = totalUTF8
            return
        }
        if totalUTF8 < shownUTF8 {
            shownUTF8 = totalUTF8
            return
        }
        guard shownUTF8 < totalUTF8, ticker == nil else { return }
        scrollHUD.isPacingActive = true
        ticker = Task { @MainActor in
            defer {
                ticker = nil
                scrollHUD.isPacingActive = false
            }
            while !Task.isCancelled {
                let currentFull = latestContent
                let liveTarget = currentFull.utf8.count
                targetUTF8 = liveTarget
                let backlog = liveTarget - shownUTF8
                if backlog <= 0 { break }
                // 最小步长 3 字节（约 1 个中文字符或 3 个英文字符），避免逐字节切片
                let step = max(3, Int((Double(backlog) / Double(Self.catchUpTicks)).rounded(.up)))
                let rawNext = min(shownUTF8 + step, liveTarget)
                let snappedNext = Self.snapForwardPastCompleteSyntax(utf8Offset: rawNext, in: currentFull)
                shownUTF8 = min(snappedNext, liveTarget)
                if scrollHUD.underlyingScrollView == nil {
                    onGrow()
                }
                if shownUTF8 >= liveTarget { break }
                try? await Task.sleep(nanoseconds: Self.tickNanos)
            }
        }
    }

    /// O(1) 将 UTF-8 偏移对齐到不截断多字节字符与组合字素（如 Emoji ZWJ）的安全 `String.Index`。
    static func clampedCharacterIndex(utf8Offset: Int, in text: String) -> String.Index {
        let utf8 = text.utf8
        guard utf8Offset < utf8.count else { return text.endIndex }
        guard utf8Offset > 0 else { return text.startIndex }
        var idx = utf8.index(utf8.startIndex, offsetBy: utf8Offset)
        while idx < text.endIndex && idx.samePosition(in: text) == nil {
            idx = utf8.index(after: idx)
        }
        if idx < text.endIndex {
            let range = text.rangeOfComposedCharacterSequence(at: idx)
            if range.lowerBound < idx {
                idx = range.upperBound
            }
        }
        return idx
    }

    /// 若推进落点刚好切在“全文里已经完整到达”的 Markdown 链接 `[label](url)`、
    /// 行首块级标记（`## ` / `- ` / `1. `）或管道表格行 `| ... |` 中间，则顺势对齐到该语法闭合处，
    /// 避免人为制造多帧半截原始 URL 撑高再坍缩。
    static func snapForwardPastCompleteSyntax(utf8Offset: Int, in text: String) -> Int {
        let total = text.utf8.count
        guard utf8Offset > 0, utf8Offset < total else { return utf8Offset }

        let idx = clampedCharacterIndex(utf8Offset: utf8Offset, in: text)
        guard idx < text.endIndex else { return total }

        let lineStart: String.Index = {
            if let nl = text[..<idx].lastIndex(of: "\n") {
                return text.index(after: nl)
            }
            return text.startIndex
        }()
        let lineEnd = text[idx...].firstIndex(of: "\n") ?? text.endIndex

        // 1. 落点位于已闭合的 Markdown 链接 `](...)` URL 内部：直接跳过到 `)` 后
        if let linkClose = findEnclosingClosedLinkEnd(at: idx, lineStart: lineStart, lineEnd: lineEnd, in: text) {
            return text.utf8.distance(from: text.startIndex, to: linkClose)
        }

        // 2. 落点位于完整管道表格行 `| ... |` 中间（且该行在全文中已闭合 `|`）：整行一步到位
        let fullLineTrimmed = text[lineStart..<lineEnd].trimmingCharacters(in: .whitespaces)
        if fullLineTrimmed.hasPrefix("|") && fullLineTrimmed.hasSuffix("|") && fullLineTrimmed.count >= 3 {
            return text.utf8.distance(from: text.startIndex, to: lineEnd)
        }

        // 3. 落点刚好停在行首块标记（`#`/`##`/`-`/`*`/`+`/`>`/`1.`）而后续正文已到达：跳过标记和空格
        let prefixOnLine = text[lineStart..<idx].trimmingCharacters(in: .whitespaces)
        if isIncompleteLeadingBlockMarker(prefixOnLine) {
            var cursor = idx
            while cursor < lineEnd && (text[cursor] == "#" || text[cursor] == " " || text[cursor] == "." || text[cursor] == ")") {
                cursor = text.index(after: cursor)
            }
            if cursor < lineEnd {
                cursor = text.index(after: cursor)
            }
            return text.utf8.distance(from: text.startIndex, to: cursor)
        }

        return text.utf8.distance(from: text.startIndex, to: idx)
    }

    private static func findEnclosingClosedLinkEnd(
        at idx: String.Index,
        lineStart: String.Index,
        lineEnd: String.Index,
        in text: String
    ) -> String.Index? {
        let before = text[lineStart..<idx]
        guard let openRange = before.range(of: "](", options: .backwards) else { return nil }
        let destStart = openRange.upperBound
        // 检查从 `](` 到当前落点之间是否还没闭合 `)`
        var depth = 1
        var cursor = destStart
        while cursor < idx {
            let ch = text[cursor]
            if ch == "(" { depth += 1 }
            else if ch == ")" {
                depth -= 1
                if depth == 0 { return nil }
            }
            cursor = text.index(after: cursor)
        }
        // 继续向后查找同行内已到达的闭合 `)`（限长 512 字符防止误吞超长括号文本）
        var scanned = 0
        while cursor < lineEnd && scanned < 512 {
            let ch = text[cursor]
            if ch == "(" { depth += 1 }
            else if ch == ")" {
                depth -= 1
                if depth == 0 {
                    return text.index(after: cursor)
                }
            }
            cursor = text.index(after: cursor)
            scanned += 1
        }
        return nil
    }

    private static func isIncompleteLeadingBlockMarker(_ trimmed: String) -> Bool {
        guard !trimmed.isEmpty, trimmed.count <= 8 else { return false }
        if trimmed == "-" || trimmed == "*" || trimmed == "+" || trimmed == ">" || trimmed == "•" {
            return true
        }
        if trimmed.allSatisfy({ $0 == "#" }) && trimmed.count <= 6 {
            return true
        }
        if (trimmed.hasSuffix(".") || trimmed.hasSuffix(")")) && trimmed.count >= 2 {
            let digits = trimmed.dropLast()
            return !digits.isEmpty && digits.allSatisfy({ $0.isASCII && $0.isNumber })
        }
        return false
    }
}

extension ChatMessage {
    func withContent(_ text: String) -> ChatMessage {
        ChatMessage(
            id: id,
            sender: sender,
            content: text,
            thinking: thinking,
            toolCount: toolCount,
            toolNames: toolNames,
            imageDataList: imageDataList,
            imageUrls: imageUrls,
            artifacts: artifacts,
            stepIndex: stepIndex,
            attemptCount: attemptCount,
            maxAttempts: maxAttempts,
            model: model,
            modelName: modelName,
            subagent: subagent,
            workflow: workflow,
            stepPlan: stepPlan,
            focusText: focusText,
            startedAt: startedAt,
            timestamp: timestamp,
            slashCommands: slashCommands,
            clientMessageId: clientMessageId,
            errorKind: errorKind,
            isOverflow: isOverflow,
            thinkingDuration: thinkingDuration,
            thinkingBlocks: thinkingBlocks,
            toolDetails: toolDetails,
            isScheduledTask: isScheduledTask
        )
    }
}
