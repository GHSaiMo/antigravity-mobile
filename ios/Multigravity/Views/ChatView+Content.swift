import SwiftUI
import PhotosUI
import AVFoundation

extension ChatView {
    @ViewBuilder
    var contentArea: some View {
        Group {
            if viewModel.isLoading && viewModel.messages.isEmpty && !viewModel.isNewConversation {
                loadingStateView
            } else if let err = viewModel.errorMessage, viewModel.messages.isEmpty && !viewModel.isNewConversation {
                errorStateView(err: err)
            } else {
                GeometryReader { geometry in
                    ScrollViewReader { proxy in
                        messagesScrollView(proxy: proxy, viewportWidth: geometry.size.width, viewportHeight: geometry.size.height)
                            .onAppear {
                                currentViewportHeight = geometry.size.height
                            }
                            .onChange(of: geometry.size.height) { oldHeight, newHeight in
                                currentViewportHeight = newHeight
                                if newHeight != oldHeight {
                                    if !hasInitiallyAligned {
                                        alignMessages(proxy: proxy, animated: false)
                                    } else if isNearBottom && !hasUserInteracted {
                                        scrollToBottom(proxy: proxy, animated: true)
                                    }
                                }
                            }
                    }
                }
            }
        }
        .sheet(isPresented: $viewModel.showConfirmUndoSheet) {
            ConfirmUndoSheet(viewModel: viewModel)
                .presentationDetents([.medium, .large])
                .presentationDragIndicator(.visible)
        }
    }
    
    @ViewBuilder
    var loadingStateView: some View {
        VStack(spacing: 14) {
            ProgressView()
                .scaleEffect(1.2)
            Text("正在同步会话历史与步骤...")
                .font(.system(size: 13.5))
                .foregroundColor(.secondary)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }
    
    @ViewBuilder
    func errorStateView(err: String) -> some View {
        VStack(spacing: 14) {
            Image(systemName: "exclamationmark.triangle")
                .font(.system(size: 36))
                .foregroundColor(.orange)
            Text(err)
                .font(.system(size: 13.5))
                .foregroundColor(.secondary)
                .multilineTextAlignment(.center)
                .padding(.horizontal, 24)
            Button("点击重试") {
                Task { await viewModel.loadMessages() }
            }
            .buttonStyle(.borderedProminent)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }
    
    @ViewBuilder
    func messagesScrollView(proxy: ScrollViewProxy, viewportWidth: CGFloat, viewportHeight: CGFloat) -> some View {
        ScrollView(.vertical, showsIndicators: true) {
            messagesList(proxy: proxy)
                .frame(width: horizontalSizeClass == .regular ? min(viewportWidth, chatReadableMaxWidth) : viewportWidth)
                .frame(maxWidth: .infinity)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .coordinateSpace(name: "ChatScrollViewSpace")
        .onPreferenceChange(ChatContentHeightPreferenceKey.self) { height in
            guard height > 0 else { return }
            if abs(messagesContentHeight - height) > 1 {
                messagesContentHeight = height
            }
        }
        .onPreferenceChange(ChatBottomAnchorOffsetPreferenceKey.self) { anchorMaxY in
            guard anchorMaxY.isFinite else { return }
            let isContentFitting = messagesContentHeight > 0 && currentViewportHeight > 0 && messagesContentHeight <= currentViewportHeight
            let near = isContentFitting || (anchorMaxY <= viewportHeight + 90)
            if near && hasUserInteracted {
                hasUserInteracted = false
            }
            if near != isNearBottom {
                isNearBottom = near
            }
        }
        .scrollBounceBehavior(.basedOnSize, axes: .horizontal)
        .background(Color(uiColor: .systemBackground))
        .contentShape(Rectangle())
        .simultaneousGesture(
            TapGesture().onEnded {
                if isInputFocused {
                    isInputFocused = false
                }
            }
        )
        .simultaneousGesture(
            DragGesture(minimumDistance: 10).onChanged { _ in
                hasUserInteracted = true
            }
        )
        .scrollDismissesKeyboard(.interactively)
        .refreshable {
            await viewModel.loadMessages()
        }
        .onAppear {
            // 第 1 阶段：快速非动画初位定位（0.05s），避免看到历史顶部闪动
            DispatchQueue.main.asyncAfter(deadline: .now() + 0.05) {
                alignMessages(proxy: proxy, animated: false)
            }
            // 第 2 阶段：等待 NavigationStack 转场动画完全完成（约 0.35s），视口展开至最终真实高度后二次对齐
            DispatchQueue.main.asyncAfter(deadline: .now() + 0.35) {
                alignMessages(proxy: proxy, animated: false)
            }
        }
        .onChange(of: viewModel.isLoading) { _, loading in
            if !loading {
                // 网络会话历史同步结算后校准对齐
                DispatchQueue.main.asyncAfter(deadline: .now() + 0.05) {
                    alignMessages(proxy: proxy, animated: false)
                }
                if canScheduleAutoFocus {
                    scheduleAutoFocus(delay: 0.2)
                }
            }
        }
        .onChange(of: cardToggleTrigger) { _, _ in
            performAdaptiveCardScroll(proxy: proxy)
        }
        .onChange(of: viewModel.runningTasks.count) { oldVal, newVal in
            if newVal != oldVal && hasInitiallyAligned {
                if isNearBottom || !hasUserInteracted {
                    scrollToBottom(proxy: proxy, animated: true)
                }
            }
        }
        .onChange(of: viewModel.scrollToTurnStartTrigger) { _, _ in
            scrollToTurnStart(proxy: proxy, animated: true)
        }
        .onChange(of: viewModel.scrollToBottomTrigger) { _, _ in
            scrollToBottom(proxy: proxy, animated: true)
        }
        .onChange(of: viewModel.messages.last?.id) { _, lastId in
            guard lastId != nil else { return }
            if !hasInitiallyAligned {
                alignMessages(proxy: proxy, animated: false)
                return
            }
            if !hasUserInteracted || isNearBottom {
                scrollToBottom(proxy: proxy, animated: true)
            }
        }
        .onChange(of: viewModel.messages.last) { _, lastMsg in
            guard lastMsg != nil else { return }
            guard hasInitiallyAligned else { return }
            if !hasUserInteracted || isNearBottom {
                scrollToBottom(proxy: proxy, animated: true)
            }
        }
        .onChange(of: viewModel.queuedMessages) { oldVal, newVal in
            if newVal != oldVal && hasInitiallyAligned {
                if isNearBottom || !hasUserInteracted {
                    scrollToBottom(proxy: proxy, animated: true)
                }
            }
        }
        .onChange(of: isInputFocused) { _, focused in
            if focused {
                DispatchQueue.main.asyncAfter(deadline: .now() + 0.1) {
                    scrollToBottom(proxy: proxy, animated: true)
                }
            } else {
                // 等待键盘完全收起并恢复完整视口高度后，做底部对齐校准，消除悬空留白
                DispatchQueue.main.asyncAfter(deadline: .now() + 0.28) {
                    scrollToBottom(proxy: proxy, animated: true)
                }
            }
        }
        .onChange(of: viewModel.focusInputTrigger) {
            Task { @MainActor in
                try? await Task.sleep(nanoseconds: 100_000_000)
                isInputFocused = true
            }
        }
    }
    
    var canScheduleAutoFocus: Bool {
        guard !hasAutoFocused, autoFocusTask == nil else { return false }
        if shouldAutoFocus { return true }
        return viewModel.messages.isEmpty && viewModel.stepCount == 0
    }
    
    @ViewBuilder
    func messagesList(proxy: ScrollViewProxy) -> some View {
        LazyVStack(spacing: 8) {
            if viewModel.hasMore && !viewModel.messages.contains(where: { $0.id == "step-0" || $0.effectiveStepIndex == 0 }) {
                loadOlderMessagesButton(proxy: proxy)
            }
            
            if viewModel.messages.isEmpty {
                emptyStateView
            }
            
            ForEach(Array(viewModel.messages.enumerated()), id: \.element.id) { index, message in
                messageRow(index: index, message: message)
            }
            
            if shouldShowThinkingBubble {
                AgentThinkingBubbleView()
                    .id("THINKING_INDICATOR")
                    .transition(.opacity)
            }
            
            Color.clear
                .frame(maxWidth: .infinity)
                .frame(height: 1)
                .id("BOTTOM_ANCHOR")
                .background(
                    GeometryReader { geo in
                        Color.clear.preference(
                            key: ChatBottomAnchorOffsetPreferenceKey.self,
                            value: geo.frame(in: .named("ChatScrollViewSpace")).maxY
                        )
                    }
                )
        }
        .padding(.top, 12)
        .padding(.bottom, 4)
        .frame(maxWidth: .infinity)
        .background(
            GeometryReader { geo in
                Color.clear.preference(
                    key: ChatContentHeightPreferenceKey.self,
                    value: geo.size.height
                )
            }
        )
    }
    
    var shouldShowThinkingBubble: Bool {
        (viewModel.isAwaitingResponse || viewModel.isRunning) && (viewModel.messages.last?.isToolBatch != true)
    }
    
    @ViewBuilder
    func messageRow(index: Int, message: ChatMessage) -> some View {
        let isLast = (index == viewModel.messages.count - 1)
        let isActive = isLast && (viewModel.isRunning || viewModel.isAwaitingResponse)
        MessageBubbleView(
            message: message,
            isActiveToolBatch: isActive,
            onUndo: { msg in
                viewModel.requestUndo(for: msg)
            }
        )
        .id(message.id)
    }
    
    @ViewBuilder
    func loadOlderMessagesButton(proxy: ScrollViewProxy) -> some View {
        Button(action: {
            let currentTopId = viewModel.messages.first?.id
            Task {
                await viewModel.loadOlderMessages()
                if let topId = currentTopId {
                    withAnimation(.easeOut(duration: 0.2)) {
                        proxy.scrollTo(topId, anchor: .top)
                    }
                }
            }
        }) {
            if viewModel.isLoadingOlder {
                ProgressView()
                    .scaleEffect(0.9)
                    .frame(maxWidth: .infinity)
                    .padding(.vertical, 8)
            } else {
                HStack(spacing: 6) {
                    Image(systemName: "arrow.up.circle.fill")
                        .font(.system(size: 13))
                    Text("查看更早的消息")
                        .font(.system(size: 12.5, weight: .medium))
                }
                .foregroundColor(.secondary)
                .padding(.horizontal, 14)
                .padding(.vertical, 7)
                .background(Color(uiColor: .secondarySystemBackground).opacity(0.8))
                .clipShape(Capsule())
                .frame(maxWidth: .infinity)
                .padding(.vertical, 4)
            }
        }
        .buttonStyle(.plain)
    }
    

    var emptyStateView: some View {
        VStack(spacing: 16) {
            Spacer(minLength: 60)
            
            ZStack {
                Circle()
                    .fill(Color.accentColor.opacity(0.12))
                    .frame(width: 58, height: 58)
                Image(systemName: "sparkles")
                    .font(.system(size: 24, weight: .medium))
                    .foregroundColor(.accentColor)
            }
            
            VStack(spacing: 6) {
                Text(viewModel.currentTitle)
                    .font(.system(size: 16.5, weight: .semibold))
                    .foregroundColor(.primary)
                    .lineLimit(1)
                
                Text((viewModel.isPureChat || viewModel.currentTitle == "新对话") ? "新对话模式，在下方输入指令开启对话" : "已连接工作区，在下方输入指令开启对话")
                    .font(.system(size: 13))
                    .foregroundColor(.secondary)
            }
            
            Spacer(minLength: 60)
        }
        .frame(maxWidth: .infinity)
        .padding(.horizontal, 32)
    }
}
