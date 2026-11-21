import SwiftUI
import PhotosUI
import AVFoundation

extension ChatView {
    @ViewBuilder
    var contentArea: some View {
        VStack(spacing: 0) {
            if let trust = viewModel.trustStatus, !trust.isProjectTrusted {
                WorkspaceTrustBanner(
                    status: trust,
                    isUpdating: viewModel.isUpdatingTrust,
                    onTrust: {
                        Task { await viewModel.trustCurrentProject() }
                    }
                )
            }
            Group {
                if viewModel.isLoading && viewModel.messages.isEmpty && !viewModel.isNewConversation {
                    loadingStateView
                } else if let err = viewModel.errorMessage, viewModel.messages.isEmpty && !viewModel.isNewConversation {
                    errorStateView(err: err)
                } else {
                GeometryReader { geometry in
                    ScrollViewReader { proxy in
                        ZStack(alignment: .bottomTrailing) {
                            messagesScrollView(proxy: proxy, viewportWidth: geometry.size.width, viewportHeight: geometry.size.height)
                            
                            // 浮动跳跃双钮导航条 (Prompt Jump Stepper)
                            ChatStepperOverlayView(
                                scrollHUD: scrollHUD,
                                totalTurns: viewModel.turns.count,
                                onJumpPrevious: {
                                    wakeStepper()
                                    jumpToPreviousTurn()
                                },
                                onJumpNextOrBottom: {
                                    wakeStepper()
                                    jumpToNextTurnOrBottom()
                                },
                                onForceJumpBottom: {
                                    wakeStepper()
                                    forceJumpToBottom()
                                }
                            )
                            .padding(.trailing, 14)
                            .padding(.bottom, 12)
                        }
                        .onAppear {
                            currentViewportHeight = geometry.size.height
                        }
                        .onChange(of: geometry.size.height) { oldHeight, newHeight in
                            currentViewportHeight = newHeight
                            if newHeight != oldHeight {
                                if !hasInitiallyAligned {
                                    alignMessages(proxy: proxy, animated: false)
                                } else if scrollHUD.isNearBottom && !hasUserInteracted {
                                    scrollToBottom(proxy: proxy, animated: true)
                                }
                            }
                        }
                    }
                }
            }
        }
        }
        // 会话切换、Agent 运行状态翻转（跑完了）、提交/改动面板关闭后重新探测 Commit / Changes 是否该显示
        .task(id: "\(viewModel.cascadeId)|\(viewModel.isRunning)|\(viewModel.gitSheet == nil)") {
            await viewModel.refreshActionAvailability()
        }
        .sheet(isPresented: $viewModel.showConfirmUndoSheet, onDismiss: {
            viewModel.cancelUndoSheet()
        }) {
            ConfirmUndoSheet(viewModel: viewModel)
                .presentationDetents([.medium, .large])
                .presentationDragIndicator(.visible)
        }
        .sheet(isPresented: Binding(
            get: { viewModel.changesSheet != nil },
            set: { if !$0 { viewModel.closeChangesSheet() } }
        )) {
            CascadeChangesSheet(viewModel: viewModel)
                .presentationDetents([.medium, .large])
                .presentationDragIndicator(.visible)
        }
        .sheet(isPresented: Binding(
            get: { viewModel.gitSheet != nil },
            set: { if !$0 { viewModel.closeGitSheet() } }
        )) {
            GitCommitSheet(viewModel: viewModel) {
                viewModel.closeGitSheet()
                insertCommitAndPush()
            }
        }
        .sheet(item: $viewModel.exportedMarkdownFile) { file in
            ShareSheetView(activityItems: [file.url])
                .presentationDetents([.medium, .large])
        }
        .sheet(item: $selectedSubagentDetail) { sa in
            SubagentDetailSheet(
                subagent: sa,
                onStopSubagent: { item in
                    await viewModel.stopSubagent(item)
                },
                onAskBtw: { quote in
                    btwInitialQuote = quote
                    showBtwSheet = true
                }
            )
        }
        .sheet(isPresented: $showWorkflowTemplateSheet, onDismiss: {
            workflowSheetStartInEditMode = false
        }) {
            WorkflowTemplateSheet(initialEditing: workflowSheetStartInEditMode) { templatePrompt in
                viewModel.inputText = templatePrompt
                viewModel.pendingSelectFirstPlaceholder = true
                isInputFocused = true
            }
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
    
    func messagesScrollView(proxy: ScrollViewProxy, viewportWidth: CGFloat, viewportHeight: CGFloat) -> some View {
        let step1 = messagesScrollCore(proxy: proxy, viewportWidth: viewportWidth, viewportHeight: viewportHeight)
        let step2 = applyScrollGesturesAndLifecycle(step1, proxy: proxy)
        let step3 = applyScrollLoadingAndTriggerHandlers(step2, proxy: proxy)
        return applyScrollMessageAndFocusHandlers(step3, proxy: proxy)
    }

    private func messagesScrollCore(proxy: ScrollViewProxy, viewportWidth: CGFloat, viewportHeight: CGFloat) -> some View {
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
        .onPreferenceChange(ChatVisibleMessagesPreferenceKey.self) { visibleGeometries in
            scrollHUD.lastVisibleMessageGeometries = visibleGeometries
            updateCurrentVisibleTurn(viewportHeight: viewportHeight)
        }
        .onChange(of: viewModel.turns.count) { _, _ in
            updateCurrentVisibleTurn(viewportHeight: viewportHeight)
        }
    }

    private func applyScrollGesturesAndLifecycle<Content: View>(_ content: Content, proxy: ScrollViewProxy) -> some View {
        content
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
                DragGesture(minimumDistance: 10)
                    .onChanged { _ in
                        hasUserInteracted = true
                        isUserDragging = true
                        if !hasInitiallyAligned && !viewModel.messages.isEmpty {
                            hasInitiallyAligned = true
                        }
                        if scrollHUD.lockedTurnIndex != nil {
                            scrollHUD.lockedTurnIndex = nil
                        }
                    }
                    .onEnded { _ in isUserDragging = false }
            )
            .scrollDismissesKeyboard(.interactively)
            // 直接基于 ScrollView 几何状态计算距底距离与越界，避免 LazyVStack 中 BOTTOM_ANCHOR 离屏卸载后失灵
            .onScrollGeometryChange(for: ChatScrollMetrics.self) { geo in
                let vh = geo.containerSize.height
                let ch = geo.contentSize.height
                let dist = ch - (geo.contentOffset.y + vh)
                let qDist = (dist / 16).rounded() * 16
                let overshoot: CGFloat = (ch > vh) ? (geo.contentOffset.y + vh - ch) : 0
                let qOvershoot: CGFloat = overshoot > 120 ? (overshoot / 16).rounded() * 16 : 0
                return ChatScrollMetrics(
                    quantizedDistanceFromBottom: qDist,
                    viewportHeight: vh.rounded(),
                    contentHeight: ch.rounded(),
                    overshoot: qOvershoot
                )
            } action: { _, metrics in
                if metrics.contentHeight > 0 && abs(messagesContentHeight - metrics.contentHeight) > 1 {
                    messagesContentHeight = metrics.contentHeight
                }
                scheduleOvershootHeal(overshoot: metrics.overshoot, proxy: proxy)
                guard metrics.viewportHeight > 0, metrics.contentHeight > 0 else { return }
                let effectiveVH = currentViewportHeight > 0 ? currentViewportHeight : metrics.viewportHeight
                let anchorMaxY = effectiveVH + metrics.quantizedDistanceFromBottom
                let isContentFitting = metrics.contentHeight <= effectiveVH
                let near = isContentFitting || (metrics.quantizedDistanceFromBottom <= 90)
                if near && hasUserInteracted {
                    hasUserInteracted = false
                }
                if near != scrollHUD.isNearBottom {
                    scrollHUD.isNearBottom = near
                    updateCurrentVisibleTurn(viewportHeight: effectiveVH)
                }
                scheduleBlankSpaceHeal(anchorMaxY: anchorMaxY, viewportHeight: effectiveVH, proxy: proxy)
                handleStepperScrollProgress(anchorMaxY: anchorMaxY, isNear: near)
            }
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
                // 第 3 阶段：贴底后底部的行仍没被 LazyVStack 创建（整屏空白）时兜底
                for delay in [0.7, 1.4] {
                    DispatchQueue.main.asyncAfter(deadline: .now() + delay) {
                        healUnmaterializedBottom(proxy: proxy)
                    }
                }
            }
    }

    private func applyScrollLoadingAndTriggerHandlers<Content: View>(_ content: Content, proxy: ScrollViewProxy) -> some View {
        content
            .onChange(of: viewModel.isLoading) { _, loading in
                if !loading {
                    if !useLazyMessagesStack && !hasInitiallyAligned && ChatView.shouldUseLazyStack(for: viewModel.messages) {
                        useLazyMessagesStack = true
                    }
                    // 网络会话历史同步结算后校准对齐
                    DispatchQueue.main.asyncAfter(deadline: .now() + 0.05) {
                        alignMessages(proxy: proxy, animated: false)
                    }
                    if canScheduleAutoFocus {
                        scheduleAutoFocus(delay: 0.2)
                    }
                }
            }
            .onChange(of: viewModel.isLoadingOlder) { _, loadingOlder in
                if !loadingOlder && !useLazyMessagesStack && !viewModel.isActivelyRunning && ChatView.shouldUseLazyStack(for: viewModel.messages) {
                    useLazyMessagesStack = true
                }
            }
            .onChange(of: cardToggleTrigger) { _, _ in
                performAdaptiveCardScroll(proxy: proxy)
            }
            .onChange(of: viewModel.runningTasks.count) { oldVal, newVal in
                if newVal != oldVal && hasInitiallyAligned {
                    if scrollHUD.isNearBottom || !hasUserInteracted {
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
    }

    private func applyScrollMessageAndFocusHandlers<Content: View>(_ content: Content, proxy: ScrollViewProxy) -> some View {
        content
            .onChange(of: viewModel.messages.last?.id) { _, lastId in
                guard lastId != nil else { return }
                if !hasInitiallyAligned {
                    alignMessages(proxy: proxy, animated: false)
                    return
                }
                if !hasUserInteracted || scrollHUD.isNearBottom {
                    // 新 Agent 气泡进入流式传输时无动画贴底，避免与 PacedAgentRow 的 followStreamGrowth 动画冲突抖动
                    let shouldAnimate = !(viewModel.messages.last?.isAgent == true && viewModel.isActivelyRunning)
                    scrollToBottom(proxy: proxy, animated: shouldAnimate)
                }
            }
            .onChange(of: viewModel.messages.last) { oldMsg, lastMsg in
                guard let lastMsg else { return }
                guard hasInitiallyAligned else { return }
                // 新消息 ID 切换已由上方 last?.id 处理；同一条 Agent 消息的文本增长由 PacedAgentRow 按节奏驱动滚动
                guard oldMsg?.id == lastMsg.id, !lastMsg.isAgent else { return }
                if !hasUserInteracted || scrollHUD.isNearBottom {
                    scrollToBottom(proxy: proxy, animated: true)
                }
            }
            .onChange(of: viewModel.queuedMessages) { oldVal, newVal in
                if newVal != oldVal && hasInitiallyAligned {
                    if scrollHUD.isNearBottom || !hasUserInteracted {
                        scrollToBottom(proxy: proxy, animated: true)
                    }
                }
            }
            .onChange(of: isInputFocused) { _, focused in
                if focused {
                    hideStepperImmediately()
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
            .onChange(of: targetScrollTurnId) { _, targetId in
                guard let targetId = targetId else { return }
                if targetId == "BOTTOM_ANCHOR" {
                    hasUserInteracted = false
                    scrollHUD.isNearBottom = true
                    withAnimation(.easeOut(duration: 0.2)) {
                        proxy.scrollTo("BOTTOM_ANCHOR", anchor: .bottom)
                    }
                } else {
                    hasUserInteracted = true
                    scrollHUD.isNearBottom = false
                    blankHealTask?.cancel()
                    blankHealTask = nil

                    // 1. 若目标为首条消息或首轮，先对齐顶部锚点唤醒顶部区域渲染
                    if targetId == viewModel.messages.first?.id || targetId == "TOP_ANCHOR" {
                        proxy.scrollTo("TOP_ANCHOR", anchor: .top)
                    }

                    // 2. 立即跳转至目标消息行
                    proxy.scrollTo(targetId, anchor: .top)

                    // 3. 在 Sheet 转场关闭及 Lazy 视图实例化阶段做多级微调校准，避免大跨度跳转下的空白
                    for delay in [0.08, 0.22, 0.4] {
                        DispatchQueue.main.asyncAfter(deadline: .now() + delay) {
                            proxy.scrollTo(targetId, anchor: .top)
                        }
                    }
                }
                highlightedTurnMessageId = targetId
                DispatchQueue.main.asyncAfter(deadline: .now() + 0.5) {
                    targetScrollTurnId = nil
                }
                DispatchQueue.main.asyncAfter(deadline: .now() + 1.8) {
                    if highlightedTurnMessageId == targetId {
                        withAnimation(.easeOut(duration: 0.3)) {
                            highlightedTurnMessageId = nil
                        }
                    }
                }
            }
    }
    
    var canScheduleAutoFocus: Bool {
        guard !hasAutoFocused, autoFocusTask == nil else { return false }
        if shouldAutoFocus { return true }
        return viewModel.messages.isEmpty && viewModel.stepCount == 0
    }
    
    /// 消息极少且内容轻量时用普通 VStack：LazyVStack 在程序化大幅跳转或滚到底后可能不创建行，整屏空白。
    /// 通过锁存的 `useLazyMessagesStack` 控制容器身份（综合消息条数、总文本量、工具调用数），
    /// 绝不在流式输出跨过阈值瞬间切换容器，防止整屏视图与内嵌 WKWebView 销毁重挂载。
    @ViewBuilder
    func messagesStack<C: View>(@ViewBuilder _ content: () -> C) -> some View {
        if !useLazyMessagesStack {
            VStack(spacing: 8) { content() }
        } else {
            LazyVStack(spacing: 8) { content() }
        }
    }

    @ViewBuilder
    func messagesList(proxy: ScrollViewProxy) -> some View {
        let lastId = viewModel.messages.last?.id
        let isStreamingOrRunning = viewModel.isRunning || viewModel.isAwaitingResponse
        let shouldTrackVisibility = viewModel.turns.count >= 2
        let canRevert = GatewayCompatStore.shared.isAvailable(GatewayFeature.revert)
        let canExport = GatewayCompatStore.shared.isAvailable(GatewayFeature.export)
        let highlightedId = highlightedTurnMessageId

        messagesStack {
            Color.clear
                .frame(maxWidth: .infinity)
                .frame(height: 1)
                .id("TOP_ANCHOR")
            
            if viewModel.hasMore && !viewModel.messages.contains(where: { $0.id == "step-0" || $0.effectiveStepIndex == 0 }) {
                loadOlderMessagesButton(proxy: proxy)
            }
            
            if viewModel.messages.isEmpty {
                emptyStateView
            }
            
            ForEach(viewModel.messages) { message in
                messageRow(
                    message: message,
                    isLast: message.id == lastId,
                    isActive: (message.id == lastId) && isStreamingOrRunning,
                    isHighlighted: highlightedId == message.id,
                    shouldTrackVisibility: shouldTrackVisibility,
                    canRevert: canRevert,
                    canExport: canExport,
                    proxy: proxy
                )
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
    func messageRow(
        message: ChatMessage,
        isLast: Bool,
        isActive: Bool,
        isHighlighted: Bool,
        shouldTrackVisibility: Bool,
        canRevert: Bool,
        canExport: Bool,
        proxy: ScrollViewProxy
    ) -> some View {
        EquatableMessageRow(
            message: message,
            isActive: isActive,
            isHighlighted: isHighlighted,
            canRevert: canRevert,
            canExport: canExport,
            onFollowStreamGrowth: { followStreamGrowth(proxy: proxy) },
            onUndo: { msg in viewModel.requestUndo(for: msg) },
            onExportMarkdown: { viewModel.exportMarkdown() },
            onCompact: {
                Task {
                    _ = await viewModel.sendMessage(text: "/compact")
                }
            },
            onStartNewSession: {
                NotificationCenter.default.post(
                    name: .startNewSessionForWorkspace,
                    object: viewModel.workspaceName
                )
            },
            onStopWorkflow: {
                Task {
                    await viewModel.cancelTask()
                }
            },
            onSelectSubagent: { sa in
                selectedSubagentDetail = sa
            },
            onFork: { target in
                if let stepIdx = target.effectiveStepIndex {
                    Task { await viewModel.forkConversation(at: stepIdx, createWorktree: false) }
                }
            },
            onNarrate: { text in
                narrationPlayer.play(text: text, baseURL: AppSettings.shared.gatewayURL)
            },
            onAskBtw: { quote in
                btwInitialQuote = quote
                showBtwSheet = true
            },
            makeShareContext: { shareContext(for: message) }
        )
        .equatable()
        .id(message.id)
        .background(
            Group {
                if shouldTrackVisibility {
                    GeometryReader { geo in
                        let frame = geo.frame(in: .named("ChatScrollViewSpace"))
                        let isPossiblyVisible = frame.maxY >= -40 && frame.minY <= 1600
                        let qMinY = (frame.minY / 32).rounded() * 32
                        let qMaxY = (frame.maxY / 32).rounded() * 32
                        Color.clear.preference(
                            key: ChatVisibleMessagesPreferenceKey.self,
                            value: isPossiblyVisible ? [VisibleMessageGeometry(id: message.id, minY: qMinY, maxY: qMaxY)] : []
                        )
                    }
                }
            }
        )
        .onAppear {
            if isLast { lastRowMaterialized = true }
        }
        .onDisappear { if isLast { lastRowMaterialized = false } }
    }
    
    func updateCurrentVisibleTurn(viewportHeight: CGFloat) {
        let turns = viewModel.turns
        guard !turns.isEmpty else {
            if scrollHUD.currentVisibleTurnIndex != 1 { scrollHUD.currentVisibleTurnIndex = 1 }
            return
        }
        
        // 1. 若用户刚从大纲导轨手动点选了某一轮，且尚未手动拖动滚动条，保持所选轮次
        if let locked = scrollHUD.lockedTurnIndex, turns.contains(where: { $0.turnIndex == locked }) {
            if scrollHUD.currentVisibleTurnIndex != locked {
                scrollHUD.currentVisibleTurnIndex = locked
            }
            return
        }
        
        // 2. 若初始尚未完成滚动对齐，或当前位于会话底部区域，直接对应最新一轮
        if !hasInitiallyAligned || scrollHUD.isNearBottom {
            let lastIdx = turns.last?.turnIndex ?? 1
            if scrollHUD.currentVisibleTurnIndex != lastIdx {
                scrollHUD.currentVisibleTurnIndex = lastIdx
            }
            return
        }
        
        // 3. 根据当前视口内可见的消息几何位置，计算处于阅读视觉重心处的消息所属轮次
        let effectiveHeight = viewportHeight > 0 ? viewportHeight : (currentViewportHeight > 0 ? currentViewportHeight : 600)
        let visibleInViewport = scrollHUD.lastVisibleMessageGeometries.filter { $0.maxY > 8 && $0.minY < effectiveHeight - 8 }
        guard !visibleInViewport.isEmpty else { return }
        
        // 若首条消息已在视口顶部附近，判定为第 1 轮
        if let firstMsgId = viewModel.messages.first?.id,
           let firstGeo = visibleInViewport.first(where: { $0.id == firstMsgId }),
           firstGeo.minY >= -24 {
            let firstIdx = turns.first?.turnIndex ?? 1
            if scrollHUD.currentVisibleTurnIndex != firstIdx {
                scrollHUD.currentVisibleTurnIndex = firstIdx
            }
            return
        }
        
        // 取视口中上部（45% 高度处）作为阅读锚点，优先选择覆盖该锚点的消息，否则选择距离锚点最近的消息
        let anchorY = effectiveHeight * 0.45
        let bestGeo = visibleInViewport.min { a, b in
            let distA: CGFloat = (anchorY >= a.minY && anchorY <= a.maxY) ? 0 : min(abs(a.minY - anchorY), abs(a.maxY - anchorY))
            let distB: CGFloat = (anchorY >= b.minY && anchorY <= b.maxY) ? 0 : min(abs(b.minY - anchorY), abs(b.maxY - anchorY))
            if abs(distA - distB) > 1 {
                return distA < distB
            }
            return a.minY > b.minY
        }
        
        if let targetId = bestGeo?.id,
           let matchedTurn = turns.first(where: { $0.messages.contains(where: { $0.id == targetId }) }) {
            if scrollHUD.currentVisibleTurnIndex != matchedTurn.turnIndex {
                scrollHUD.currentVisibleTurnIndex = matchedTurn.turnIndex
            }
        }
    }
    
    func shareContext(for message: ChatMessage) -> ShareCardContext {
        var question: ChatMessage?
        if let idx = viewModel.messages.firstIndex(where: { $0.id == message.id }) {
            for candidate in viewModel.messages[..<idx].reversed() {
                if case .user = candidate.sender {
                    question = candidate.hasQuestionContent ? candidate : nil
                    break
                }
            }
        }
        let badge = ShareCardContext.resolveModel(
            for: message,
            in: viewModel.messages,
            activeModel: viewModel.activeModel,
            activeModelName: viewModel.activeModelName
        )
        return ShareCardContext(
            sessionTitle: viewModel.currentTitle,
            modelName: badge.name,
            modelIsClaude: badge.isClaude,
            startedAtISO: viewModel.startedAt,
            previousMessage: question
        )
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
                
                Text((viewModel.isPureChat || viewModel.currentTitle == String(localized: "新对话") || viewModel.currentTitle == "新对话") ? String(localized: "新对话模式，在下方输入指令开启对话") : String(localized: "已连接工作区，在下方输入指令开启对话"))
                    .font(.system(size: 13))
                    .foregroundColor(.secondary)
            }
            
            Spacer(minLength: 60)
        }
        .frame(maxWidth: .infinity)
        .padding(.horizontal, 32)
    }
    
    // MARK: - Prompt Jump Stepper Helpers
    
    func handleStepperScrollProgress(anchorMaxY: CGFloat, isNear: Bool) {
        if isNear || isInputFocused || viewModel.turns.count < 2 {
            hideStepperImmediately()
            scrollHUD.lastAnchorMaxY = anchorMaxY
            return
        }
        
        if scrollHUD.lastAnchorMaxY == 0 {
            scrollHUD.lastAnchorMaxY = anchorMaxY
            return
        }
        
        let delta = anchorMaxY - scrollHUD.lastAnchorMaxY
        scrollHUD.lastAnchorMaxY = anchorMaxY
        
        // 极端布局跳变防护（允许快速惯性滑动正常唤醒跳轮条）
        guard abs(delta) < 2000, abs(delta) > 2 else { return }
        
        if delta > 0 {
            // 上滑（翻看旧历史 / 内容向下，anchorMaxY 增加）-> 唤醒完全可见 (1.0)，重置 3s 渐隐与 10s 全隐定时器
            wakeStepper()
        } else {
            // 下滑（滑向底部新消息 / 内容向上，anchorMaxY 减小）-> 立即渐隐至 30% 半隐藏，重置 10s 全隐定时器
            dimStepperOnScrollDown()
        }
    }
    
    func wakeStepper() {
        guard viewModel.turns.count >= 2, !scrollHUD.isNearBottom, !isInputFocused else { return }
        let needsReveal = scrollHUD.stepperOpacity < 0.99
        if needsReveal {
            withAnimation(.easeInOut(duration: 0.2)) {
                scrollHUD.stepperOpacity = 1.0
            }
        }
        let now = Date()
        if needsReveal || now.timeIntervalSince(scrollHUD.lastStepperScheduleAt) >= 0.35 {
            scrollHUD.lastStepperScheduleAt = now
            scheduleStepperTimers()
        }
    }
    
    func dimStepperOnScrollDown() {
        guard viewModel.turns.count >= 2, !scrollHUD.isNearBottom, !isInputFocused else { return }
        let needsDim = scrollHUD.stepperOpacity > 0.3
        if needsDim {
            withAnimation(.easeInOut(duration: 0.25)) {
                scrollHUD.stepperOpacity = 0.3
            }
        }
        let now = Date()
        guard needsDim || scrollHUD.stepperHideTask == nil || now.timeIntervalSince(scrollHUD.lastStepperScheduleAt) >= 0.35 else {
            return
        }
        scrollHUD.lastStepperScheduleAt = now
        scrollHUD.stepperDimTask?.cancel()
        scrollHUD.stepperHideTask?.cancel()
        scrollHUD.stepperHideTask = Task { @MainActor in
            try? await Task.sleep(nanoseconds: 10_000_000_000)
            guard !Task.isCancelled else { return }
            withAnimation(.easeInOut(duration: 0.35)) {
                scrollHUD.stepperOpacity = 0.0
            }
        }
    }
    
    func scheduleStepperTimers() {
        scrollHUD.stepperDimTask?.cancel()
        scrollHUD.stepperHideTask?.cancel()
        
        // 停止滑动 3s 后，渐隐至 30% 半隐藏
        scrollHUD.stepperDimTask = Task { @MainActor in
            try? await Task.sleep(nanoseconds: 3_000_000_000)
            guard !Task.isCancelled else { return }
            withAnimation(.easeInOut(duration: 0.35)) {
                scrollHUD.stepperOpacity = 0.3
            }
        }
        
        // 停止滑动 10s 后，完全隐藏 (0.0)
        scrollHUD.stepperHideTask = Task { @MainActor in
            try? await Task.sleep(nanoseconds: 10_000_000_000)
            guard !Task.isCancelled else { return }
            withAnimation(.easeInOut(duration: 0.35)) {
                scrollHUD.stepperOpacity = 0.0
            }
        }
    }
    
    func hideStepperImmediately() {
        scrollHUD.stepperDimTask?.cancel()
        scrollHUD.stepperHideTask?.cancel()
        if scrollHUD.stepperOpacity > 0 {
            withAnimation(.easeInOut(duration: 0.2)) {
                scrollHUD.stepperOpacity = 0.0
            }
        }
    }
    
    private func jumpToPreviousTurn() {
        let turns = viewModel.turns
        guard !turns.isEmpty else { return }
        let targetIdx = max(1, scrollHUD.currentVisibleTurnIndex - 1)
        scrollHUD.lockedTurnIndex = targetIdx
        scrollHUD.currentVisibleTurnIndex = targetIdx
        if let match = turns.first(where: { $0.turnIndex == targetIdx }) {
            targetScrollTurnId = match.firstMessageId
        }
    }
    
    private func jumpToNextTurnOrBottom() {
        let turns = viewModel.turns
        guard !turns.isEmpty else { return }
        let lastIdx = turns.last?.turnIndex ?? 1
        if scrollHUD.currentVisibleTurnIndex < lastIdx {
            let targetIdx = scrollHUD.currentVisibleTurnIndex + 1
            scrollHUD.lockedTurnIndex = targetIdx
            scrollHUD.currentVisibleTurnIndex = targetIdx
            if let match = turns.first(where: { $0.turnIndex == targetIdx }) {
                targetScrollTurnId = match.firstMessageId
            }
        } else {
            forceJumpToBottom()
        }
    }
    
    private func forceJumpToBottom() {
        scrollHUD.lockedTurnIndex = nil
        targetScrollTurnId = "BOTTOM_ANCHOR"
    }
}

/// 独立观察 `scrollHUD` 的浮动跳轮条容器，避免 `stepperOpacity` / `currentVisibleTurnIndex` 变化触发整页消息列表重算。
private struct ChatStepperOverlayView: View {
    let scrollHUD: ChatScrollHUDState
    let totalTurns: Int
    let onJumpPrevious: () -> Void
    let onJumpNextOrBottom: () -> Void
    let onForceJumpBottom: () -> Void

    var body: some View {
        let opacity = scrollHUD.stepperOpacity
        let currentTurn = scrollHUD.currentVisibleTurnIndex
        if opacity > 0.01 {
            PromptJumpStepperView(
                currentTurn: currentTurn,
                totalTurns: totalTurns,
                isAtTop: currentTurn <= 1,
                isAtLastTurn: currentTurn >= totalTurns,
                onJumpPrevious: onJumpPrevious,
                onJumpNextOrBottom: onJumpNextOrBottom,
                onForceJumpBottom: onForceJumpBottom
            )
            .opacity(opacity)
            .allowsHitTesting(opacity > 0.05)
            .transition(.opacity)
        }
    }
}

/// 通过 `Equatable` 隔离每条历史消息行：滚动、输入框打字或顶部状态变更时，内容未变的消息行跳过 body 重建与 Markdown 块求值。
private struct EquatableMessageRow: View, Equatable {
    let message: ChatMessage
    let isActive: Bool
    let isHighlighted: Bool
    let canRevert: Bool
    let canExport: Bool
    let onFollowStreamGrowth: () -> Void
    let onUndo: (ChatMessage) -> Void
    let onExportMarkdown: () -> Void
    let onCompact: () -> Void
    let onStartNewSession: () -> Void
    let onStopWorkflow: () -> Void
    let onSelectSubagent: (SubagentItem) -> Void
    let onFork: (ChatMessage) -> Void
    let onNarrate: (String) -> Void
    let onAskBtw: (String) -> Void
    let makeShareContext: () -> ShareCardContext

    static func == (lhs: EquatableMessageRow, rhs: EquatableMessageRow) -> Bool {
        lhs.message == rhs.message
            && lhs.isActive == rhs.isActive
            && lhs.isHighlighted == rhs.isHighlighted
            && lhs.canRevert == rhs.canRevert
            && lhs.canExport == rhs.canExport
    }

    var body: some View {
        let bubble: (ChatMessage) -> MessageBubbleView = { msg in
            MessageBubbleView(
                message: msg,
                isActiveToolBatch: isActive,
                onUndo: canRevert ? onUndo : nil,
                onExportMarkdown: canExport ? onExportMarkdown : nil,
                onCompact: onCompact,
                onStartNewSession: onStartNewSession,
                onStopWorkflow: onStopWorkflow,
                onSelectSubagent: onSelectSubagent,
                onFork: onFork,
                onNarrate: onNarrate,
                onAskBtw: onAskBtw,
                makeShareContext: makeShareContext
            )
        }
        Group {
            if message.isAgent {
                // 流式文本按稳定速度播放，并在播放过程中平滑跟随到底部
                PacedAgentRow(message: message, onGrow: onFollowStreamGrowth, content: bubble)
            } else {
                bubble(message)
            }
        }
        .overlay(
            Group {
                if isHighlighted {
                    RoundedRectangle(cornerRadius: 18, style: .continuous)
                        .stroke(Color.indigo, lineWidth: 2)
                        .shadow(color: Color.indigo.opacity(0.35), radius: 6)
                        .padding(.horizontal, 2)
                        .transition(.opacity)
                }
            }
        )
    }
}
