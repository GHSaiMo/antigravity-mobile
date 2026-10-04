import SwiftUI
import PhotosUI
import AVFoundation

extension ChatView {
    func scheduleAutoFocus(delay: Double = 0.45) {
        guard !hasAutoFocused else { return }
        autoFocusTask?.cancel()
        autoFocusTask = Task { @MainActor in
            if delay > 0 {
                try? await Task.sleep(nanoseconds: UInt64(delay * 1_000_000_000))
            }
            guard !Task.isCancelled, isViewAppeared, !hasAutoFocused, viewModel.messages.isEmpty, viewModel.pendingInteraction == nil else { return }
            hasAutoFocused = true
            isInputFocused = true
        }
    }
    
    func alignMessages(proxy: ScrollViewProxy, animated: Bool = false) {
        guard !viewModel.messages.isEmpty else { return }
        guard !hasUserInteracted else { return }
        hasInitiallyAligned = true
        smartScroll(proxy: proxy, animated: animated)
    }
    
    func smartScroll(proxy: ScrollViewProxy, animated: Bool = false) {
        // 1. 若会话正在运行、等待回复、或有活跃后台任务，必须保持在底部展示最新任务卡片与进展
        let isActivelyRunning = viewModel.isActivelyRunning || (initialStatus?.isRunning == true)
        if isActivelyRunning {
            scrollToBottom(proxy: proxy, animated: animated)
            return
        }
        
        // 2. 若最后一条消息是用户发送的，或最新一轮对话中尚无 Agent 文本回复，直接滚动到底部展示最新内容
        if viewModel.messages.last?.sender == .user || viewModel.latestAgentMessageId == nil {
            scrollToBottom(proxy: proxy, animated: animated)
            return
        }
        
        // 3. 判断是否需要从本轮 Agent 回复开头展示：
        // 仅在会话处于未读、报错或等待用户交互，且存在最新的 Agent 回复时，才定位到该回复开头
        let shouldScrollToTurnStart = viewModel.shouldScrollToTurnStartOnEntry
            || (initialIsUnread && viewModel.latestAgentMessageId != nil)
            || (initialStatus?.isError == true && viewModel.latestAgentMessageId != nil)
            || (initialStatus?.needsAction == true && viewModel.latestAgentMessageId != nil)
        
        if shouldScrollToTurnStart {
            scrollToTurnStart(proxy: proxy, animated: animated)
        } else {
            scrollToBottom(proxy: proxy, animated: animated)
        }
    }
    
    func scrollToTurnStart(proxy: ScrollViewProxy, animated: Bool = true) {
        // Target agent response message start when opening a chat with unread messages, error, or pending action
        guard let targetId = viewModel.latestAgentMessageId else {
            scrollToBottom(proxy: proxy, animated: animated)
            return
        }
        
        if animated {
            withAnimation(.easeOut(duration: 0.22)) {
                proxy.scrollTo(targetId, anchor: .top)
            }
        } else {
            proxy.scrollTo(targetId, anchor: .top)
        }
    }
    
    func scrollToBottom(proxy: ScrollViewProxy, animated: Bool = true) {
        let isThinkingActive = (viewModel.isAwaitingResponse || viewModel.isRunning) && (viewModel.messages.last?.isToolBatch != true)
        // 空会话且无活跃思考卡片时，无需强行滚动到底部，避免 emptyStateView 被挤出视口顶部
        if viewModel.messages.isEmpty && !isThinkingActive {
            return
        }
        
        // 当内容总高度小于等于视口可用高度时，所有消息均完整在可视区域内。
        // 此时强行向底部锚点滚动会促使 UIScrollView 产生负偏移越界，触发橡皮筋反弹、抖动与闪烁漂移。
        if messagesContentHeight > 0 && currentViewportHeight > 0 && messagesContentHeight <= currentViewportHeight {
            return
        }
        
        let target = "BOTTOM_ANCHOR"
        if animated {
            withAnimation(.easeOut(duration: 0.2)) {
                proxy.scrollTo(target, anchor: .bottom)
            }
        } else {
            proxy.scrollTo(target, anchor: .bottom)
        }
    }
    
    func handleFloatingCardToggle(isExpanded: Bool) {
        hasUserInteracted = false
        isNearBottom = true
        cardToggleTrigger &+= 1
    }
    
    func performAdaptiveCardScroll(proxy: ScrollViewProxy) {
        hasUserInteracted = false
        isNearBottom = true
        guard messagesContentHeight == 0 || currentViewportHeight == 0 || messagesContentHeight > currentViewportHeight else {
            return
        }
        
        // 1. 同步使用完全一致的弹性阻尼动画启动滚动，卡片展开向上弹起，卡片折叠直接贴着卡片边缘回弹
        withAnimation(.spring(response: 0.35, dampingFraction: 0.8)) {
            scrollToBottom(proxy: proxy, animated: false)
        }
        
        // 2. 连续多阶段布局微调吸附，消除折叠卡片后的悬空留白，贴边回弹
        DispatchQueue.main.asyncAfter(deadline: .now() + 0.12) {
            withAnimation(.spring(response: 0.30, dampingFraction: 0.82)) {
                scrollToBottom(proxy: proxy, animated: false)
            }
        }
        DispatchQueue.main.asyncAfter(deadline: .now() + 0.30) {
            scrollToBottom(proxy: proxy, animated: false)
        }
    }
    
    func performAutoScrollToBottom(proxy: ScrollViewProxy, animated: Bool = true) {
        performAdaptiveCardScroll(proxy: proxy)
    }
    
}
