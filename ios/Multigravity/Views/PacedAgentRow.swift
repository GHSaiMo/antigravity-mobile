import SwiftUI

/// 流式输出的“匀速播放”：网关推送的文本增量可能一次来很多（网络好时一帧几百字），
/// 直接渲染会让内容疯狂上涨、自动跟随滚动飞快，什么都看不清。
/// 这里把 Agent 消息文本的显示进度和网络到达速度解耦，按稳定的速度逐步追上目标文本：
/// 积压越多追得越快（约 0.6 秒内追平），积压很少时保持约 30 字/秒的阅读节奏。
struct PacedAgentRow<Content: View>: View {
    let message: ChatMessage
    /// 每次显示长度增长后回调（用于让外层平滑跟随到底部）。
    let onGrow: () -> Void
    @ViewBuilder let content: (ChatMessage) -> Content

    @State private var shown: Int = -1
    @State private var target: Int = 0
    @State private var ticker: Task<Void, Never>?

    private static var tickNanos: UInt64 { 33_000_000 }
    /// 积压按此帧数追平：越大越平缓。
    private static var catchUpTicks: Int { 20 }

    var body: some View {
        let full = message.content
        let total = full.count
        let visible: ChatMessage = (shown < 0 || shown >= total)
            ? message
            : message.withContent(String(full.prefix(shown)))
        content(visible)
            .onAppear { sync(total: total, initial: true) }
            .onChange(of: total) { _, newTotal in sync(total: newTotal, initial: false) }
            .onDisappear {
                ticker?.cancel()
                ticker = nil
                shown = -1
            }
    }

    private func sync(total: Int, initial: Bool) {
        target = total
        if initial || shown < 0 {
            // 首次出现（含历史加载、列表回收后重现）直接显示完整内容，只对之后的增长做节奏控制。
            shown = total
            return
        }
        if total < shown { shown = total; return }
        guard shown < total, ticker == nil else { return }
        ticker = Task { @MainActor in
            while !Task.isCancelled {
                let backlog = target - shown
                if backlog <= 0 { break }
                let step = max(1, Int((Double(backlog) / Double(Self.catchUpTicks)).rounded(.up)))
                shown = min(shown + step, target)
                onGrow()
                try? await Task.sleep(nanoseconds: Self.tickNanos)
            }
            ticker = nil
        }
    }
}

extension ChatMessage {
    func withContent(_ text: String) -> ChatMessage {
        ChatMessage(
            id: id, sender: sender, content: text, thinking: thinking,
            toolCount: toolCount, toolNames: toolNames,
            imageDataList: imageDataList, imageUrls: imageUrls, artifacts: artifacts,
            stepIndex: stepIndex, attemptCount: attemptCount, maxAttempts: maxAttempts,
            model: model, modelName: modelName, subagent: subagent
        )
    }
}
