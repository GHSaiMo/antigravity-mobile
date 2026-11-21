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
        guard !scrollHUD.hasUserInteracted, !preserveScrollOnReturn else { return }
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
        if scrollHUD.messagesContentHeight > 0 && currentViewportHeight > 0 && scrollHUD.messagesContentHeight <= currentViewportHeight {
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
        scrollHUD.hasUserInteracted = false
        scrollHUD.isNearBottom = true
        cardToggleTrigger &+= 1
    }
    
    func performAdaptiveCardScroll(proxy: ScrollViewProxy) {
        scrollHUD.hasUserInteracted = false
        scrollHUD.isNearBottom = true
        guard scrollHUD.messagesContentHeight == 0 || currentViewportHeight == 0 || scrollHUD.messagesContentHeight > currentViewportHeight else {
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
    
    /// 流式文本每播放一小段后调用：用户没有手动翻看时，无动画贴底，避免动画叠加造成的抖动。
    func followStreamGrowth(proxy: ScrollViewProxy) {
        guard hasInitiallyAligned, !scrollHUD.isUserDragging else { return }
        guard scrollHUD.isNearBottom || !scrollHUD.hasUserInteracted else { return }
        let now = Date()
        guard now.timeIntervalSince(scrollHUD.lastFollowScrollAt) >= 0.06 else { return }
        scrollHUD.lastFollowScrollAt = now
        DispatchQueue.main.async {
            scrollToBottom(proxy: proxy, animated: false)
        }
    }
    
    /// 长会话里 LazyVStack 用估算高度滚到底，随后真实行高落定，偏移可能停在内容末尾之后导致底部留白。
    /// 统一使用 ScrollGeometry 同源的越界量，且严格限定仅在用户未手动翻阅历史、且滚动已静止时执行底部贴合，
    /// 绝不在用户浏览历史轮次或惯性减速途中强拉回底部。
    func scheduleOvershootHeal(overshoot: CGFloat, proxy: ScrollViewProxy) {
        guard !scrollHUD.hasUserInteracted, !preserveScrollOnReturn else {
            scrollHUD.blankHealTask?.cancel()
            scrollHUD.blankHealTask = nil
            return
        }
        guard overshoot > 64 else {
            scrollHUD.blankHealTask?.cancel()
            scrollHUD.blankHealTask = nil
            return
        }
        scrollHUD.blankHealTask?.cancel()
        scrollHUD.blankHealTask = Task { @MainActor in
            try? await Task.sleep(nanoseconds: 220_000_000)
            guard !Task.isCancelled,
                  !scrollHUD.isUserDragging,
                  !scrollHUD.isScrollMoving,
                  !scrollHUD.hasUserInteracted,
                  !preserveScrollOnReturn else { return }
            scrollToBottom(proxy: proxy, animated: false)
        }
    }

    /// 长会话（LazyVStack）程序化滚到底后，底部的行有时不被创建，整屏空白。
    /// 稳定后若最后一行仍未出现，先滚到最后一行再贴底，逼它创建。
    func healUnmaterializedBottom(proxy: ScrollViewProxy) {
        guard !scrollHUD.hasUserInteracted,
              !scrollHUD.isUserDragging,
              !scrollHUD.isScrollMoving,
              !scrollHUD.lastRowMaterialized,
              !preserveScrollOnReturn,
              let lastId = viewModel.messages.last?.id else { return }
        proxy.scrollTo(lastId, anchor: .bottom)
        DispatchQueue.main.async {
            scrollToBottom(proxy: proxy, animated: false)
        }
    }

    /// 会话中途在 LazyVStack 里翻阅超长气泡后，若出现极端偏移失步导致视口内零可见消息（整屏纯白），
    /// 在滚动完全静止（idle）后就地唤醒刚才阅读的消息行，绝不篡改用户当前阅读轮次或强跳回底部。
    func scheduleMidScrollBlankHealIfNeeded(proxy: ScrollViewProxy, viewportHeight: CGFloat) {
        guard useLazyMessagesStack,
              hasInitiallyAligned,
              !preserveScrollOnReturn,
              !viewModel.isLoading,
              !viewModel.messages.isEmpty,
              !scrollHUD.isUserDragging,
              !scrollHUD.isScrollMoving,
              targetScrollTurnId == nil else {
            scrollHUD.midScrollBlankHealTask?.cancel()
            scrollHUD.midScrollBlankHealTask = nil
            return
        }
        let effectiveVH = viewportHeight > 0 ? viewportHeight : (currentViewportHeight > 0 ? currentViewportHeight : 600)
        let hasAnyRenderedInViewport = scrollHUD.lastVisibleMessageGeometries.contains {
            $0.maxY >= -80 && $0.minY <= effectiveVH + 80
        }
        guard !hasAnyRenderedInViewport else {
            scrollHUD.midScrollBlankHealTask?.cancel()
            scrollHUD.midScrollBlankHealTask = nil
            return
        }
        guard scrollHUD.midScrollBlankHealTask == nil else { return }
        scrollHUD.midScrollBlankHealTask = Task { @MainActor in
            defer { scrollHUD.midScrollBlankHealTask = nil }
            try? await Task.sleep(nanoseconds: 180_000_000)
            guard !Task.isCancelled,
                  useLazyMessagesStack,
                  hasInitiallyAligned,
                  !preserveScrollOnReturn,
                  !viewModel.isLoading,
                  !viewModel.messages.isEmpty,
                  !scrollHUD.isUserDragging,
                  !scrollHUD.isScrollMoving,
                  targetScrollTurnId == nil else { return }
            let vh = currentViewportHeight > 0 ? currentViewportHeight : effectiveVH
            let stillEmpty = !scrollHUD.lastVisibleMessageGeometries.contains {
                $0.maxY >= -80 && $0.minY <= vh + 80
            }
            guard stillEmpty else { return }
            let now = Date()
            guard now.timeIntervalSince(scrollHUD.lastMidBlankHealAt) >= 1.2 else { return }
            scrollHUD.lastMidBlankHealAt = now

            if !scrollHUD.hasUserInteracted || scrollHUD.isNearBottom {
                if let lastId = viewModel.messages.last?.id {
                    proxy.scrollTo(lastId, anchor: .bottom)
                }
                DispatchQueue.main.async {
                    scrollToBottom(proxy: proxy, animated: false)
                }
            } else if let anchorId = scrollHUD.lastKnownVisibleMessageId,
                      viewModel.messages.contains(where: { $0.id == anchorId }) {
                proxy.scrollTo(anchorId, anchor: .center)
            } else {
                let ch = max(scrollHUD.messagesContentHeight, 1)
                let ratio = min(max(scrollHUD.lastQuantizedOffsetY / ch, 0), 1)
                let approxIdx = min(max(Int((CGFloat(viewModel.messages.count - 1) * ratio).rounded()), 0), viewModel.messages.count - 1)
                let fallbackId = viewModel.messages[approxIdx].id
                proxy.scrollTo(fallbackId, anchor: .center)
            }
        }
    }
}
