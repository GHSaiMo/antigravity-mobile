import SwiftUI
import PhotosUI
import AVFoundation

private struct ChatBottomAnchorOffsetPreferenceKey: PreferenceKey {
    static var defaultValue: CGFloat = .infinity
    static func reduce(value: inout CGFloat, nextValue: () -> CGFloat) {
        value = nextValue()
    }
}

public struct ChatView: View {
    @Environment(\.dismiss) private var dismiss
    @Environment(\.scenePhase) private var scenePhase
    @State private var viewModel: ChatViewModel
    @FocusState private var isInputFocused: Bool
    @State private var hasInitiallyAligned = false
    @State private var hasUserInteracted = false
    @State private var isNearBottom = true
    @State private var cardToggleTrigger = 0
    @State private var showCameraPicker = false
    @State private var showCameraUnavailableAlert = false
    @State private var showCameraPermissionAlert = false
    @State private var previewDraftGallery: ImageGalleryData? = nil
    private let shouldAutoFocus: Bool
    private let initialConversation: ConversationItem?
    private let initialIsUnread: Bool
    private let initialStatus: ConversationItem.ConversationStatus?
    @State private var hasAutoFocused = false
    @State private var isViewAppeared = false
    @State private var autoFocusTask: Task<Void, Never>? = nil
    
    public init(conversation: ConversationItem, isNewConversation: Bool = false) {
        self.initialConversation = conversation
        self.initialIsUnread = conversation.isUnread
        self.initialStatus = conversation.status
        if conversation.isDraft {
            self.shouldAutoFocus = true
            if var draftSession = CacheManager.shared.getLocalDraftSession(id: conversation.id) {
                if draftSession.draftImages.isEmpty {
                    draftSession.draftImages = CacheManager.shared.getDraftImages(for: conversation.id)
                }
                _viewModel = State(initialValue: ChatViewModel(draftSession: draftSession))
            } else if let project = conversation.draftProject {
                var session = LocalDraftSession(
                    id: conversation.id,
                    project: project,
                    draftText: CacheManager.shared.getDraft(for: conversation.id)
                )
                session.draftImages = CacheManager.shared.getDraftImages(for: conversation.id)
                _viewModel = State(initialValue: ChatViewModel(draftSession: session))
            } else {
                let isNewOrEmpty = isNewConversation || conversation.stepCount == 0
                _viewModel = State(initialValue: ChatViewModel(
                    cascadeId: conversation.id,
                    initialTitle: conversation.title,
                    isNewConversation: isNewOrEmpty,
                    isUnread: conversation.isUnread,
                    conversationStatus: conversation.status
                ))
            }
        } else {
            let isNewOrEmpty = isNewConversation || conversation.stepCount == 0
            self.shouldAutoFocus = isNewOrEmpty
            _viewModel = State(initialValue: ChatViewModel(
                cascadeId: conversation.id,
                initialTitle: conversation.title,
                isNewConversation: isNewOrEmpty,
                isUnread: conversation.isUnread,
                conversationStatus: conversation.status
            ))
        }
    }
    
    public init(draftSession: LocalDraftSession) {
        self.initialConversation = nil
        self.initialIsUnread = false
        self.initialStatus = nil
        self.shouldAutoFocus = true
        _viewModel = State(initialValue: ChatViewModel(
            draftSession: draftSession
        ))
    }
    
    public init(draftProject: ProjectItem) {
        self.initialConversation = nil
        self.initialIsUnread = false
        self.initialStatus = nil
        self.shouldAutoFocus = true
        _viewModel = State(initialValue: ChatViewModel(
            draftProject: draftProject
        ))
    }
    
    public var body: some View {
        VStack(spacing: 0) {
            contentArea
            errorBanner
            floatingCards
            inputBar
        }
        .navigationTitle(viewModel.currentTitle)
        .navigationBarTitleDisplayMode(.inline)
        .navigationBarBackButtonHidden(true)
        .toolbar {
            ToolbarItem(placement: .topBarLeading) {
                Button(action: {
                    dismiss()
                }) {
                    Image(systemName: "chevron.left")
                        .font(.system(size: 16, weight: .semibold))
                }
            }
        }
        .onAppear {
            isViewAppeared = true
            viewModel.restoreDraftsIfNeeded()
            if shouldAutoFocus {
                scheduleAutoFocus(delay: 0.45)
            }
            if !viewModel.messages.isEmpty {
                Task {
                    await viewModel.resumeActiveSession()
                }
            }
        }
        .task {
            await viewModel.loadMessages()
            viewModel.connectStream()
        }
        .onChange(of: scenePhase) { _, newPhase in
            if newPhase == .active {
                viewModel.restoreDraftsIfNeeded()
                Task {
                    await viewModel.resumeActiveSession()
                }
            } else if newPhase == .background || newPhase == .inactive {
                viewModel.saveCurrentDraft()
                if newPhase == .background {
                    viewModel.handleAppBackground()
                }
            }
        }
        .onReceive(NotificationCenter.default.publisher(for: UIApplication.didBecomeActiveNotification)) { _ in
            viewModel.restoreDraftsIfNeeded()
            Task {
                await viewModel.resumeActiveSession()
            }
        }
        .onReceive(NotificationCenter.default.publisher(for: UIApplication.didEnterBackgroundNotification)) { _ in
            viewModel.saveCurrentDraft()
            viewModel.handleAppBackground()
        }
        .onDisappear {
            isViewAppeared = false
            hasInitiallyAligned = false
            hasUserInteracted = false
            isNearBottom = true
            autoFocusTask?.cancel()
            autoFocusTask = nil
            viewModel.saveCurrentDraft()
            viewModel.disconnectStream()
        }
        .environment(\.openURL, OpenURLAction { url in
            handleURLTap(url)
        })
        .sheet(item: $viewModel.viewingMarkdownFile, onDismiss: {
            viewModel.closeMarkdownViewer()
        }) { (item: MarkdownFileViewerData) in
            renderMarkdownViewer(data: item)
                .presentationDragIndicator(.visible)
        }
        .sheet(isPresented: Binding(
            get: { viewModel.quickLookURL != nil },
            set: { if !$0 { viewModel.closeQuickLook() } }
        )) {
            if let qlURL = viewModel.quickLookURL {
                QuickLookPreviewSheet(url: qlURL, title: viewModel.quickLookTitle) {
                    viewModel.closeQuickLook()
                }
                .presentationDragIndicator(.visible)
            }
        }
        .sheet(isPresented: Binding(
            get: { viewModel.htmlPreviewURL != nil },
            set: { if !$0 { viewModel.closeHTMLPreview() } }
        )) {
            if let htmlURL = viewModel.htmlPreviewURL {
                HTMLPreviewSheet(url: htmlURL, title: viewModel.htmlPreviewTitle) {
                    viewModel.closeHTMLPreview()
                }
                .presentationDragIndicator(.visible)
            }
        }
        .overlay {
            if viewModel.isDownloadingDocument {
                ZStack {
                    Color.black.opacity(0.12)
                        .ignoresSafeArea()
                        .onTapGesture {
                            viewModel.cancelDocumentDownload()
                        }
                    
                    VStack(spacing: 12) {
                        Text("正在下载文件")
                            .font(.system(size: 14, weight: .medium))
                            .foregroundColor(.primary)
                        
                        if viewModel.downloadBytesTotal > 0 {
                            ProgressView(value: viewModel.downloadProgress)
                                .progressViewStyle(.linear)
                                .tint(.accentColor)
                                .frame(width: 150)
                        } else {
                            ProgressView()
                                .progressViewStyle(.circular)
                        }
                    }
                    .padding(.horizontal, 24)
                    .padding(.vertical, 16)
                    .background(.ultraThinMaterial)
                    .clipShape(RoundedRectangle(cornerRadius: 16, style: .continuous))
                    .shadow(color: .black.opacity(0.12), radius: 12, y: 4)
                }
                .transition(.opacity)
                .animation(.easeInOut(duration: 0.15), value: viewModel.isDownloadingDocument)
            }
        }
        .fullScreenCover(isPresented: $showCameraPicker) {
            CameraPickerView { capturedImage in
                handleCapturedImage(capturedImage)
            }
            .ignoresSafeArea()
        }
        .alert("无法使用相机", isPresented: $showCameraUnavailableAlert) {
            Button("好", role: .cancel) {}
        } message: {
            Text("当前设备或模拟器未检测到可用相机。")
        }
        .alert("需要相机权限", isPresented: $showCameraPermissionAlert) {
            Button("前往设置") {
                if let url = URL(string: UIApplication.openSettingsURLString) {
                    UIApplication.shared.open(url)
                }
            }
            Button("取消", role: .cancel) {}
        } message: {
            Text("请在系统设置中允许 Multigravity 访问相机以拍照。")
        }
    }
    
    @ViewBuilder
    private var contentArea: some View {
        if viewModel.isLoading && viewModel.messages.isEmpty && !viewModel.isNewConversation {
            loadingStateView
        } else if let err = viewModel.errorMessage, viewModel.messages.isEmpty && !viewModel.isNewConversation {
            errorStateView(err: err)
        } else {
            GeometryReader { geometry in
                ScrollViewReader { proxy in
                    messagesScrollView(proxy: proxy, viewportWidth: geometry.size.width, viewportHeight: geometry.size.height)
                        .onChange(of: geometry.size.height) { oldHeight, newHeight in
                            if newHeight != oldHeight {
                                if isNearBottom {
                                    performAdaptiveCardScroll(proxy: proxy)
                                } else if !hasInitiallyAligned {
                                    alignMessages(proxy: proxy, animated: false)
                                }
                            }
                        }
                }
            }
        }
    }
    
    @ViewBuilder
    private var loadingStateView: some View {
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
    private func errorStateView(err: String) -> some View {
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
    private func messagesScrollView(proxy: ScrollViewProxy, viewportWidth: CGFloat, viewportHeight: CGFloat) -> some View {
        ScrollView(.vertical, showsIndicators: true) {
            messagesList(proxy: proxy)
                .frame(width: viewportWidth)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .coordinateSpace(name: "ChatScrollViewSpace")
        .onPreferenceChange(ChatBottomAnchorOffsetPreferenceKey.self) { anchorMaxY in
            guard anchorMaxY.isFinite else { return }
            let near = anchorMaxY <= viewportHeight + 90
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
            // 第 1 阶段：快速非动画初位定位，避免看到历史顶部闪动
            DispatchQueue.main.asyncAfter(deadline: .now() + 0.06) {
                alignMessages(proxy: proxy, animated: false)
            }
            // 第 2 阶段：等待 NavigationStack 转场动画完全完成（约 0.35s），视口展开至最终真实高度后二次对齐
            DispatchQueue.main.asyncAfter(deadline: .now() + 0.35) {
                alignMessages(proxy: proxy, animated: false)
            }
            // 第 3 阶段：兜底针对大篇幅 Markdown / Table 异步渲染完成后的终态贴边校准
            DispatchQueue.main.asyncAfter(deadline: .now() + 0.60) {
                alignMessages(proxy: proxy, animated: false)
            }
        }
        .onChange(of: viewModel.isLoading) { _, loading in
            if !loading {
                // 网络会话历史同步结算后校准对齐
                DispatchQueue.main.asyncAfter(deadline: .now() + 0.08) {
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
            if newVal != oldVal {
                if isNearBottom || !hasUserInteracted {
                    performAdaptiveCardScroll(proxy: proxy)
                }
            }
        }
        .onChange(of: viewModel.scrollToTurnStartTrigger) { _, _ in
            scrollToTurnStart(proxy: proxy, animated: true)
        }
        .onChange(of: viewModel.scrollToBottomTrigger) { _, _ in
            performAdaptiveCardScroll(proxy: proxy)
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
            if newVal != oldVal {
                if isNearBottom || !hasUserInteracted {
                    performAdaptiveCardScroll(proxy: proxy)
                }
            }
        }
        .onChange(of: isInputFocused) { _, focused in
            performAdaptiveCardScroll(proxy: proxy)
            if focused {
                DispatchQueue.main.asyncAfter(deadline: .now() + 0.08) {
                    performAdaptiveCardScroll(proxy: proxy)
                }
                DispatchQueue.main.asyncAfter(deadline: .now() + 0.28) {
                    performAdaptiveCardScroll(proxy: proxy)
                }
            } else {
                // 等待键盘完全收起并恢复完整视口高度后，做底部对齐校准，消除悬空留白
                DispatchQueue.main.asyncAfter(deadline: .now() + 0.28) {
                    performAdaptiveCardScroll(proxy: proxy)
                }
            }
        }
    }
    
    private var canScheduleAutoFocus: Bool {
        guard !hasAutoFocused, autoFocusTask == nil else { return false }
        if shouldAutoFocus { return true }
        return viewModel.messages.isEmpty && viewModel.stepCount == 0
    }
    
    @ViewBuilder
    private func messagesList(proxy: ScrollViewProxy) -> some View {
        LazyVStack(spacing: 8) {
            if viewModel.hasMore && !viewModel.messages.contains(where: { $0.id == "step-0" }) {
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
    }
    
    private var shouldShowThinkingBubble: Bool {
        (viewModel.isAwaitingResponse || viewModel.isRunning) && (viewModel.messages.last?.isToolBatch != true)
    }
    
    @ViewBuilder
    private func messageRow(index: Int, message: ChatMessage) -> some View {
        let isLast = (index == viewModel.messages.count - 1)
        let isActive = isLast && (viewModel.isRunning || viewModel.isAwaitingResponse)
        MessageBubbleView(message: message, isActiveToolBatch: isActive)
            .id(message.id)
    }
    
    @ViewBuilder
    private func loadOlderMessagesButton(proxy: ScrollViewProxy) -> some View {
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
    
    @ViewBuilder
    private var floatingCards: some View {
        if let interaction = viewModel.pendingInteraction {
            InteractionCardView(
                interaction: interaction,
                isSubmitting: viewModel.isSubmittingInteraction,
                onSubmit: { optionId, writeInText, target in
                    Task {
                        await viewModel.submitInteraction(optionId: optionId, writeInText: writeInText, target: target)
                    }
                },
                onSkip: {
                    Task {
                        await viewModel.skipInteraction()
                    }
                }
            )
            .transition(.move(edge: .bottom).combined(with: .opacity))
        }
        
        if !viewModel.runningTasks.isEmpty {
            RunningTasksCardView(
                items: viewModel.runningTasks,
                onStop: { task in
                    Task {
                        await viewModel.stopTask(task)
                    }
                },
                onToggleExpand: { isExpanded in
                    handleFloatingCardToggle(isExpanded: isExpanded)
                }
            )
            .transition(.move(edge: .bottom).combined(with: .opacity))
        }
        
        if !viewModel.queuedMessages.isEmpty {
            QueuedMessagesCardView(
                items: viewModel.queuedMessages,
                onSendNow: { item in
                    Task {
                        await viewModel.sendQueuedMessageNow(item: item)
                    }
                },
                onEdit: { item in
                    viewModel.editQueuedMessage(item: item)
                    isInputFocused = true
                },
                onDelete: { item in
                    viewModel.deleteQueuedMessage(item: item)
                },
                onToggleExpand: { isExpanded in
                    handleFloatingCardToggle(isExpanded: isExpanded)
                }
            )
            .transition(.move(edge: .bottom).combined(with: .opacity))
        }
    }
    
    @ViewBuilder
    private var errorBanner: some View {
        if let err = viewModel.errorMessage, (!viewModel.messages.isEmpty || viewModel.isNewConversation) {
            HStack(spacing: 8) {
                Image(systemName: "exclamationmark.triangle.fill")
                    .foregroundColor(.orange)
                    .font(.system(size: 14))
                Text(err)
                    .font(.system(size: 12.5, weight: .medium))
                    .foregroundColor(.primary)
                    .lineLimit(2)
                Spacer()
                Button(action: {
                    viewModel.errorMessage = nil
                }) {
                    Image(systemName: "xmark.circle.fill")
                        .foregroundColor(.secondary)
                        .font(.system(size: 14))
                }
                .buttonStyle(.plain)
            }
            .padding(.horizontal, 14)
            .padding(.vertical, 8)
            .background(Color(uiColor: .secondarySystemBackground))
            .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
            .padding(.horizontal, 16)
            .padding(.bottom, 6)
        }
    }
    
    private func handleURLTap(_ url: URL) -> OpenURLAction.Result {
        let clean = url.absoluteString.trimmingCharacters(in: .whitespacesAndNewlines)
        let unescaped = clean.removingPercentEncoding ?? clean
        let lower = unescaped.lowercased()
        let pathLower = url.path.lowercased()
        
        let uriParam = URLComponents(url: url, resolvingAgainstBaseURL: false)?.queryItems?.first(where: { $0.name == "uri" })?.value
        let decodedFileName: String = {
            if let param = uriParam, !param.isEmpty {
                let fn = (param as NSString).lastPathComponent.removingPercentEncoding ?? (param as NSString).lastPathComponent
                if !fn.isEmpty && fn != "raw" { return fn }
            }
            if let last = url.lastPathComponent.removingPercentEncoding, !last.isEmpty, last != "raw" {
                return last
            }
            let fn = (unescaped as NSString).lastPathComponent
            return fn.isEmpty ? "文档详情" : fn
        }()
        
        // 1. Markdown & Plan Artifacts
        if lower.hasSuffix(".md") || lower.hasSuffix(".markdown") ||
           lower.contains("/brain/") || lower.contains("/static/artifacts/") ||
           lower.contains("implementation_plan") || lower.contains("walkthrough") {
            let docTitle: String = {
                if lower.contains("walkthrough") {
                    return "Walkthrough"
                } else if lower.contains("implementation_plan") {
                    return "Implementation Plan"
                }
                return decodedFileName
            }()
            viewModel.openMarkdownViewer(uri: unescaped, title: docTitle)
            return .handled
        }
        
        // 2. Images, Presentations & Office documents (PNG, JPG, WEBP, GIF, PPTX, PPT, KEY, DOCX, XLSX, PDF, HTML, etc.)
        let isHTML = lower.hasSuffix(".html") || lower.hasSuffix(".htm") || pathLower.hasSuffix(".html") || pathLower.hasSuffix(".htm")
        let documentExtensions = [".pptx", ".ppt", ".key", ".pdf", ".docx", ".doc", ".xlsx", ".xls", ".numbers", ".pages", ".html", ".htm", ".txt", ".csv", ".json", ".log"]
        let imageExtensions = [".png", ".jpg", ".jpeg", ".webp", ".gif", ".heic", ".heif", ".bmp", ".svg", ".ico", ".tiff", ".tif"]
        let allPreviewExtensions = documentExtensions + imageExtensions
        
        let uriLower = (uriParam ?? "").lowercased()
        let isPreviewableFile = allPreviewExtensions.contains { ext in
            lower.hasSuffix(ext) || pathLower.hasSuffix(ext) || uriLower.hasSuffix(ext) || lower.contains(ext + "?") || lower.contains(ext + "#")
        }
        
        if isPreviewableFile {
            UIImpactFeedbackGenerator(style: .medium).impactOccurred()
            viewModel.downloadAndPreviewDocument(uri: unescaped, fileName: decodedFileName, isHTML: isHTML)
            return .handled
        }
        
        // 3. External Web links
        if url.scheme == "http" || url.scheme == "https" {
            return .systemAction
        }
        
        return .handled
    }
    
    @ViewBuilder
    private func renderMarkdownViewer(data: MarkdownFileViewerData) -> some View {
        MarkdownViewerSheet(
            data: data,
            onDismiss: {
                viewModel.closeMarkdownViewer()
            },
            onProceed: {
                viewModel.proceedFromViewer()
            },
            onRetry: {
                viewModel.openMarkdownViewer(uri: data.uri, title: data.title, forceRefresh: true)
            },
            onRefresh: {
                viewModel.openMarkdownViewer(uri: data.uri, title: data.title, forceRefresh: true)
            }
        )
    }
    
    private var inputBar: some View {
        VStack(alignment: .leading, spacing: 8) {
            // Quick action chips at top of input box (➕ and Model Switch placed in front of Commit and Push)
            ScrollView(.horizontal, showsIndicators: false) {
                HStack(spacing: 8) {
                    // 1. Add Image ➕ Button (Directly opens Photo Library with Camera at index 0)
                    Button {
                        openPhotoLibraryWithCamera()
                    } label: {
                        Image(systemName: "plus")
                            .font(.system(size: 13, weight: .semibold))
                            .foregroundColor(.indigo)
                            .frame(width: 34, height: 32)
                            .background(Color(uiColor: .secondarySystemBackground))
                            .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
                            .overlay(
                                RoundedRectangle(cornerRadius: 10, style: .continuous)
                                    .stroke(Color.secondary.opacity(0.25), lineWidth: 1)
                            )
                    }
                    .buttonStyle(.plain)
                    
                    // 2. Gemini / Claude Model Switch Button
                    Button(action: {
                        withAnimation(.easeInOut(duration: 0.15)) {
                            viewModel.toggleModel()
                        }
                    }) {
                        Text(viewModel.isClaudeActive ? "Claude" : "Gemini")
                            .font(.system(size: 13, weight: .medium))
                            .foregroundColor(viewModel.isClaudeActive ? .orange : .blue)
                            .padding(.horizontal, 11)
                            .frame(height: 32)
                            .background(
                                viewModel.isClaudeActive ? Color.orange.opacity(0.12) : Color.blue.opacity(0.12)
                            )
                            .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
                            .overlay(
                                RoundedRectangle(cornerRadius: 10, style: .continuous)
                                    .stroke(
                                        viewModel.isClaudeActive ? Color.orange.opacity(0.35) : Color.blue.opacity(0.35),
                                        lineWidth: 1
                                    )
                            )
                    }
                    .buttonStyle(.plain)
                    
                    // 3. Commit and Push Button
                    Button(action: insertCommitAndPush) {
                        Text("Commit and Push")
                            .font(.system(size: 13, weight: .medium))
                            .foregroundColor(.primary)
                            .padding(.horizontal, 14)
                            .frame(height: 32)
                            .background(Color(uiColor: .secondarySystemBackground))
                            .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
                            .overlay(
                                RoundedRectangle(cornerRadius: 10, style: .continuous)
                                    .stroke(Color.secondary.opacity(0.25), lineWidth: 1)
                            )
                    }
                    .buttonStyle(.plain)
                    
                    // 4. Continue Button (Shown when Agent's latest message is an error)
                    if viewModel.isLatestMessageError {
                        Button(action: handleContinue) {
                            HStack(spacing: 6) {
                                Image(systemName: "play.fill")
                                    .font(.system(size: 11, weight: .semibold))
                                    .foregroundColor(.white)
                                Text("Continue")
                                    .font(.system(size: 13, weight: .semibold))
                                    .foregroundColor(.white)
                            }
                            .padding(.horizontal, 14)
                            .frame(height: 32)
                            .background(Color.blue)
                            .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
                            .shadow(color: Color.blue.opacity(0.35), radius: 4, x: 0, y: 2)
                        }
                        .buttonStyle(.plain)
                        .transition(.scale(scale: 0.9).combined(with: .opacity))
                        .contextMenu {
                            Button {
                                insertContinue()
                            } label: {
                                Label("填入输入框", systemImage: "square.and.pencil")
                            }
                        }
                    }
                    
                    // 5. Proceed Button (Plan is opened directly via implementation_plan.md button in chat)
                    if viewModel.canProceed {
                        Button(action: handleProceed) {
                            HStack(spacing: 6) {
                                Image(systemName: "play.fill")
                                    .font(.system(size: 11, weight: .semibold))
                                    .foregroundColor(.white)
                                Text("Proceed")
                                    .font(.system(size: 13, weight: .semibold))
                                    .foregroundColor(.white)
                            }
                            .padding(.horizontal, 14)
                            .frame(height: 32)
                            .background(Color.blue)
                            .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
                            .shadow(color: Color.blue.opacity(0.35), radius: 4, x: 0, y: 2)
                        }
                        .buttonStyle(.plain)
                        .transition(.scale(scale: 0.9).combined(with: .opacity))
                    }
                }
                .padding(.horizontal, 16)
                .padding(.top, 8)
            }
            .animation(.easeInOut(duration: 0.2), value: viewModel.isLatestMessageError)
            .animation(.easeInOut(duration: 0.2), value: viewModel.canProceed)
            
            // Image previews strip
            if !viewModel.selectedImageData.isEmpty {
                ScrollView(.horizontal, showsIndicators: false) {
                    HStack(spacing: 10) {
                        ForEach(Array(viewModel.selectedImageData.enumerated()), id: \.offset) { index, data in
                            if let uiImage = UIImage(data: data) {
                                ZStack(alignment: .topTrailing) {
                                    Button(action: {
                                        isInputFocused = false
                                        let draftImages = viewModel.selectedImageData.compactMap { UIImage(data: $0) }.map { IdentifiableImage(image: $0) }
                                        previewDraftGallery = ImageGalleryData(items: draftImages, initialIndex: index)
                                    }) {
                                        Image(uiImage: uiImage)
                                            .resizable()
                                            .scaledToFill()
                                            .frame(width: 52, height: 52)
                                            .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
                                            .contentShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
                                            .overlay(
                                                RoundedRectangle(cornerRadius: 10, style: .continuous)
                                                    .stroke(Color.secondary.opacity(0.25), lineWidth: 1)
                                            )
                                    }
                                    .buttonStyle(.plain)
                                    
                                    Button(action: {
                                        removeImage(at: index)
                                    }) {
                                        Image(systemName: "xmark.circle.fill")
                                            .font(.system(size: 16))
                                            .foregroundColor(.white)
                                            .background(Circle().fill(Color.black.opacity(0.65)))
                                    }
                                    .buttonStyle(.plain)
                                    .offset(x: 4, y: -4)
                                }
                                .padding(.top, 4)
                                .padding(.trailing, 4)
                            }
                        }
                    }
                    .padding(.horizontal, 16)
                    .padding(.vertical, 2)
                }
                .transition(.opacity.combined(with: .move(edge: .bottom)))
            }
            
            // Input field and send/stop button
            HStack(alignment: .bottom, spacing: 10) {
                TextField("", text: $viewModel.inputText, prompt: Text((viewModel.isRunning || viewModel.isAwaitingResponse) ? "向队列添加指令..." : "发送对 Agent 的指令..."), axis: .vertical)
                    .font(.system(size: 16))
                    .padding(.horizontal, 16)
                    .padding(.vertical, 11)
                    .frame(minHeight: 44)
                    .background(Color(uiColor: .secondarySystemBackground))
                    .clipShape(RoundedRectangle(cornerRadius: 22, style: .continuous))
                    .lineLimit(1...5)
                    .focused($isInputFocused)
                
                if (viewModel.isRunning || viewModel.isAwaitingResponse) && viewModel.inputText.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                    Button(action: handleCancel) {
                        ZStack {
                            Circle()
                                .fill(Color(uiColor: .systemGray5))
                                .frame(width: 44, height: 44)
                                .overlay(
                                    Circle()
                                        .stroke(Color.secondary.opacity(0.25), lineWidth: 0.8)
                                )
                            
                            Image(systemName: "stop.fill")
                                .font(.system(size: 15, weight: .bold))
                                .foregroundColor(.red)
                        }
                    }
                    .buttonStyle(.plain)
                    .transition(.scale.combined(with: .opacity))
                } else {
                    Button(action: handleSend) {
                        ZStack {
                            Circle()
                                .fill(isSendDisabled ? Color(uiColor: .systemGray5) : Color.indigo)
                                .frame(width: 44, height: 44)
                                .overlay(
                                    Circle()
                                        .stroke(Color.secondary.opacity(isSendDisabled ? 0.25 : 0), lineWidth: 0.8)
                                )
                            
                            Image(systemName: "arrow.up")
                                .font(.system(size: 18, weight: .semibold))
                                .foregroundColor(isSendDisabled ? .secondary : .white)
                        }
                    }
                    .disabled(isSendDisabled)
                    .buttonStyle(.plain)
                    .transition(.scale.combined(with: .opacity))
                }
            }
            .padding(.horizontal, 16)
            .padding(.bottom, 12)
            .animation(.easeInOut(duration: 0.2), value: viewModel.isRunning || viewModel.isAwaitingResponse)
        }
        .background(Color(uiColor: .systemBackground))
        .overlay(
            Divider(), alignment: .top
        )
        .fullScreenCover(item: $previewDraftGallery) { gallery in
            ImageViewerSheet(gallery: gallery)
                .presentationBackground(.clear)
        }
    }
    
    private var isSendDisabled: Bool {
        viewModel.isSending || (viewModel.inputText.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty && viewModel.selectedImageData.isEmpty)
    }
    
    private func handleCancel() {
        UIImpactFeedbackGenerator(style: .medium).impactOccurred()
        Task {
            await viewModel.cancelTask()
        }
    }
    
    private func handleSend() {
        guard !viewModel.isSending else { return }
        let text = viewModel.inputText.trimmingCharacters(in: .whitespacesAndNewlines)
        let images = viewModel.selectedImageData
        guard !text.isEmpty || !images.isEmpty else { return }
        hasUserInteracted = false
        viewModel.inputText = ""
        viewModel.selectedImageData = []
        Task {
            let success = await viewModel.sendMessage(text: text, images: images)
            if !success && !images.isEmpty {
                viewModel.selectedImageData = images
            }
        }
    }
    
    private func removeImage(at index: Int) {
        viewModel.removeDraftImage(at: index)
    }
    
    private func openPhotoLibraryWithCamera() {
        let maxCount = 5
        let currentCount = viewModel.selectedImageData.count
        let remaining = max(0, maxCount - currentCount)
        guard remaining > 0 else {
            UIImpactFeedbackGenerator(style: .rigid).impactOccurred()
            return
        }
        
        ZLPhotoPickerBridge.shared.present(maxCount: remaining) { pickedImages in
            var compressedList: [Data] = []
            for img in pickedImages {
                if let data = compressAndResizeImage(img) {
                    compressedList.append(data)
                }
            }
            guard !compressedList.isEmpty else { return }
            viewModel.appendDraftImages(compressedList)
        }
    }
    
    private func compressAndResizeImage(_ uiImage: UIImage) -> Data? {
        let maxDim: CGFloat = 1600
        let size = uiImage.size
        let targetImage: UIImage
        if size.width > maxDim || size.height > maxDim {
            let ratio = min(maxDim / size.width, maxDim / size.height)
            let newSize = CGSize(width: size.width * ratio, height: size.height * ratio)
            let format = UIGraphicsImageRendererFormat()
            format.scale = 1.0
            let renderer = UIGraphicsImageRenderer(size: newSize, format: format)
            targetImage = renderer.image { _ in
                uiImage.draw(in: CGRect(origin: .zero, size: newSize))
            }
        } else {
            targetImage = uiImage
        }
        return targetImage.jpegData(compressionQuality: 0.65)
    }
    
    private func handleCapturedImage(_ uiImage: UIImage) {
        if let data = compressAndResizeImage(uiImage) {
            viewModel.appendDraftImages([data])
        }
    }
    
    private func handleCameraAction() {
        guard UIImagePickerController.isSourceTypeAvailable(.camera) else {
            showCameraUnavailableAlert = true
            return
        }
        
        let status = AVCaptureDevice.authorizationStatus(for: .video)
        switch status {
        case .authorized:
            showCameraPicker = true
        case .notDetermined:
            AVCaptureDevice.requestAccess(for: .video) { granted in
                DispatchQueue.main.async {
                    if granted {
                        showCameraPicker = true
                    }
                }
            }
        case .denied, .restricted:
            showCameraPermissionAlert = true
        @unknown default:
            showCameraPicker = true
        }
    }
    
    private func insertCommitAndPush() {
        UIImpactFeedbackGenerator(style: .light).impactOccurred()
        let toAppend = "Commit and Push"
        if viewModel.inputText.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
            viewModel.inputText = toAppend
        } else {
            viewModel.inputText += "\n" + toAppend
        }
        hasUserInteracted = false
        isInputFocused = true
    }
    
    private func handleContinue() {
        guard !viewModel.isSending && !viewModel.isRunning else { return }
        UIImpactFeedbackGenerator(style: .medium).impactOccurred()
        isInputFocused = false
        let text = viewModel.inputText.trimmingCharacters(in: .whitespacesAndNewlines)
        let textToSend = text.isEmpty ? "Continue" : "\(text)\nContinue"
        viewModel.inputText = ""
        Task {
            await viewModel.sendMessage(text: textToSend)
        }
    }
    
    private func insertContinue() {
        UIImpactFeedbackGenerator(style: .light).impactOccurred()
        let toAppend = "Continue"
        if viewModel.inputText.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
            viewModel.inputText = toAppend
        } else {
            viewModel.inputText += "\n" + toAppend
        }
        isInputFocused = true
    }
    
    private func handleProceed() {
        isInputFocused = false
        Task {
            await viewModel.proceedArtifact()
        }
    }
    
    private func scheduleAutoFocus(delay: Double = 0.45) {
        guard !hasAutoFocused else { return }
        autoFocusTask?.cancel()
        autoFocusTask = Task { @MainActor in
            if delay > 0 {
                try? await Task.sleep(nanoseconds: UInt64(delay * 1_000_000_000))
            }
            guard !Task.isCancelled, isViewAppeared, !hasAutoFocused, viewModel.messages.isEmpty else { return }
            hasAutoFocused = true
            isInputFocused = true
        }
    }
    
    private func alignMessages(proxy: ScrollViewProxy, animated: Bool = false) {
        guard !viewModel.messages.isEmpty else { return }
        guard !hasUserInteracted else { return }
        hasInitiallyAligned = true
        smartScroll(proxy: proxy, animated: animated)
    }
    
    private func smartScroll(proxy: ScrollViewProxy, animated: Bool = false) {
        // 1. 若会话正在运行、等待回复、或有活跃后台任务，必须保持在底部展示最新任务卡片与进展
        let isActivelyRunning = viewModel.isActivelyRunning || (initialStatus?.isRunning == true)
        if isActivelyRunning {
            if !viewModel.runningTasks.isEmpty || !viewModel.queuedMessages.isEmpty {
                performAdaptiveCardScroll(proxy: proxy)
            } else {
                scrollToBottom(proxy: proxy, animated: animated)
            }
            return
        }
        
        // 2. 若最后一条消息是用户发送的，或最新一轮对话中尚无 Agent 文本回复，直接滚动到底部展示最新内容
        if viewModel.messages.last?.sender == .user || viewModel.latestAgentMessageId == nil {
            if !viewModel.runningTasks.isEmpty || !viewModel.queuedMessages.isEmpty {
                performAdaptiveCardScroll(proxy: proxy)
            } else {
                scrollToBottom(proxy: proxy, animated: animated)
            }
            return
        }
        
        // 3. 判断是否需要从本轮 Agent 回复开头展示：
        // 仅在会话处于未读、报错或等待用户交互，且存在最新的 Agent 回复时，才定位到该回复开头
        let shouldScrollToTurnStart = viewModel.shouldScrollToTurnStartOnEntry
            || initialIsUnread
            || (initialStatus?.isError == true)
            || (initialStatus?.needsAction == true)
        
        if shouldScrollToTurnStart {
            scrollToTurnStart(proxy: proxy, animated: animated)
        } else {
            // 已读状态下，默认拉到会话最下面
            if !viewModel.runningTasks.isEmpty || !viewModel.queuedMessages.isEmpty {
                performAdaptiveCardScroll(proxy: proxy)
            } else {
                scrollToBottom(proxy: proxy, animated: animated)
            }
        }
    }
    
    private func scrollToTurnStart(proxy: ScrollViewProxy, animated: Bool = true) {
        // Target agent response message start when opening a chat with unread messages, error, or pending action
        guard let targetId = viewModel.latestAgentMessageId else {
            if !viewModel.runningTasks.isEmpty || !viewModel.queuedMessages.isEmpty {
                performAdaptiveCardScroll(proxy: proxy)
            } else {
                scrollToBottom(proxy: proxy, animated: animated)
            }
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
    
    private func scrollToBottom(proxy: ScrollViewProxy, animated: Bool = true) {
        let isThinkingActive = (viewModel.isAwaitingResponse || viewModel.isRunning) && (viewModel.messages.last?.isToolBatch != true)
        // 空会话且无活跃思考卡片时，无需强行滚动到底部，避免 emptyStateView 被挤出视口顶部
        if viewModel.messages.isEmpty && !isThinkingActive {
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
    
    private func handleFloatingCardToggle(isExpanded: Bool) {
        hasUserInteracted = false
        isNearBottom = true
        cardToggleTrigger &+= 1
    }
    
    private func performAdaptiveCardScroll(proxy: ScrollViewProxy) {
        hasUserInteracted = false
        isNearBottom = true
        // 1. 同步使用完全一致的弹性阻尼动画启动滚动，卡片展开向上弹起，卡片折叠直接贴着卡片边缘回弹
        withAnimation(.spring(response: 0.35, dampingFraction: 0.8)) {
            scrollToBottom(proxy: proxy, animated: false)
        }
        
        // 2. 连续多阶段布局微调吸附，消除折叠卡片后的悬空留白，贴边回弹
        DispatchQueue.main.asyncAfter(deadline: .now() + 0.08) {
            withAnimation(.spring(response: 0.30, dampingFraction: 0.82)) {
                scrollToBottom(proxy: proxy, animated: false)
            }
        }
        DispatchQueue.main.asyncAfter(deadline: .now() + 0.20) {
            withAnimation(.spring(response: 0.25, dampingFraction: 0.85)) {
                scrollToBottom(proxy: proxy, animated: false)
            }
        }
        DispatchQueue.main.asyncAfter(deadline: .now() + 0.36) {
            withAnimation(.easeOut(duration: 0.15)) {
                scrollToBottom(proxy: proxy, animated: false)
            }
        }
        DispatchQueue.main.asyncAfter(deadline: .now() + 0.48) {
            scrollToBottom(proxy: proxy, animated: false)
        }
    }
    
    private func performAutoScrollToBottom(proxy: ScrollViewProxy, animated: Bool = true) {
        performAdaptiveCardScroll(proxy: proxy)
    }
    
    private var emptyStateView: some View {
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

// Animated thinking & executing bubble displayed while waiting for Agent response
public struct AgentThinkingBubbleView: View {
    public init() {}
    
    public var body: some View {
        HStack(alignment: .top, spacing: 10) {
            AgentAvatarView()
            
            HStack(spacing: 8) {
                Text("Agent 正在思考与执行")
                    .font(.system(size: 13, weight: .semibold))
                    .foregroundColor(.primary)
                
                AgentActivityDotsView()
            }
            .padding(.horizontal, 12)
            .padding(.vertical, 8)
            .background(Color(uiColor: .secondarySystemBackground))
            .clipShape(RoundedRectangle(cornerRadius: 14, style: .continuous))
            
            Spacer(minLength: 40)
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 2)
    }
}

// MARK: - Markdown Viewer Sheet (Implementation Plan & Markdown Artifacts)
public struct MarkdownViewerSheet: View {
    @Environment(\.dismiss) private var dismiss
    public let data: MarkdownFileViewerData
    public let onDismiss: (() -> Void)?
    public let onProceed: (() -> Void)?
    public let onRetry: (() -> Void)?
    public let onRefresh: (() -> Void)?
    
    @State private var previewImage: IdentifiableImage? = nil
    
    public init(
        data: MarkdownFileViewerData,
        onDismiss: (() -> Void)? = nil,
        onProceed: (() -> Void)? = nil,
        onRetry: (() -> Void)? = nil,
        onRefresh: (() -> Void)? = nil
    ) {
        self.data = data
        self.onDismiss = onDismiss
        self.onProceed = onProceed
        self.onRetry = onRetry
        self.onRefresh = onRefresh
    }
    
    private var shareURL: URL? {
        if let fileURL = data.cachedFileURL, FileManager.default.fileExists(atPath: fileURL.path) {
            return fileURL
        }
        guard !data.content.isEmpty else { return nil }
        let baseName = data.title.isEmpty ? "document" : data.title
        let safeName = baseName.replacingOccurrences(of: "/", with: "_")
        let fileName = safeName.hasSuffix(".md") ? safeName : "\(safeName).md"
        let tempURL = FileManager.default.temporaryDirectory.appendingPathComponent(fileName)
        if !FileManager.default.fileExists(atPath: tempURL.path) {
            try? data.content.write(to: tempURL, atomically: true, encoding: .utf8)
        }
        return tempURL
    }
    
    public var body: some View {
        NavigationStack {
            ZStack(alignment: .bottom) {
                ScrollView {
                    VStack(alignment: .leading, spacing: 14) {
                        // Main Markdown Content Body
                        if data.isLoading {
                            VStack(spacing: 12) {
                                ProgressView()
                                    .scaleEffect(1.1)
                                Text("正在从电脑端拉取文档...")
                                    .font(.system(size: 13.5))
                                    .foregroundColor(.secondary)
                            }
                            .frame(maxWidth: .infinity, minHeight: 240)
                        } else if let err = data.errorMessage {
                            VStack(spacing: 12) {
                                Image(systemName: "exclamationmark.triangle.fill")
                                    .font(.system(size: 32))
                                    .foregroundColor(.orange)
                                Text(err)
                                    .font(.system(size: 14))
                                    .foregroundColor(.secondary)
                                    .multilineTextAlignment(.center)
                                    .padding(.horizontal, 24)
                                if let retry = onRetry {
                                    Button("重新加载", action: retry)
                                        .buttonStyle(.borderedProminent)
                                        .padding(.top, 4)
                                }
                            }
                            .frame(maxWidth: .infinity, minHeight: 240)
                        } else if data.content.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                            VStack(spacing: 8) {
                                Image(systemName: "doc.text")
                                    .font(.system(size: 32))
                                    .foregroundColor(.secondary)
                                Text("文档内容为空")
                                    .font(.system(size: 14))
                                    .foregroundColor(.secondary)
                            }
                            .frame(maxWidth: .infinity, minHeight: 200)
                        } else {
                            MarkdownContentView(content: data.content, onImageTap: { url in
                                previewImage = IdentifiableImage(url: url)
                            })
                            .frame(maxWidth: .infinity, alignment: .leading)
                        }
                    }
                    .padding(.horizontal, 16)
                    .padding(.top, 14)
                    .padding(.bottom, data.canProceed ? 90 : 32)
                }
                
                // Bottom Fixed Proceed Bar (when planning mode artifact needs approval)
                if data.canProceed {
                    VStack(spacing: 0) {
                        Divider()
                        HStack(spacing: 12) {
                            VStack(alignment: .leading, spacing: 2) {
                                Text("方案查阅完毕")
                                    .font(.system(size: 11.5, weight: .medium))
                                    .foregroundColor(.secondary)
                                Text("点击立即进入自动化执行")
                                    .font(.system(size: 12.5, weight: .semibold))
                                    .foregroundColor(.primary)
                            }
                            
                            Spacer()
                            
                            Button(action: {
                                onProceed?()
                            }) {
                                HStack(spacing: 6) {
                                    Image(systemName: "play.fill")
                                        .font(.system(size: 12, weight: .bold))
                                    Text("确认执行 (Proceed)")
                                        .font(.system(size: 13.5, weight: .semibold))
                                }
                                .foregroundColor(.white)
                                .padding(.horizontal, 16)
                                .padding(.vertical, 9)
                                .background(Color.blue)
                                .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
                                .shadow(color: Color.blue.opacity(0.35), radius: 5, x: 0, y: 2.5)
                            }
                            .buttonStyle(.plain)
                        }
                        .padding(.horizontal, 16)
                        .padding(.vertical, 10)
                        .background(Color(uiColor: .systemBackground).opacity(0.95))
                    }
                    .transition(.move(edge: .bottom).combined(with: .opacity))
                }
            }
            .navigationTitle(data.title.isEmpty ? "文档详情" : data.title)
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    if let url = shareURL {
                        ShareLink(item: url, preview: SharePreview(data.title.isEmpty ? "文档详情" : data.title)) {
                            Image(systemName: "square.and.arrow.up")
                                .font(.system(size: 16, weight: .semibold))
                        }
                    }
                }
            }
        }
        .presentationDragIndicator(.visible)
        .fullScreenCover(item: $previewImage) { item in
            ImageViewerSheet(item: item)
                .presentationBackground(.clear)
        }
    }
}


