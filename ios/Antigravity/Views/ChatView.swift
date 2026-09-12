import SwiftUI
import PhotosUI

public struct ChatView: View {
    @Environment(\.scenePhase) private var scenePhase
    @State private var viewModel: ChatViewModel
    @FocusState private var isInputFocused: Bool
    @State private var hasInitiallyAligned = false
    @State private var selectedPhotoItems: [PhotosPickerItem] = []
    private let shouldAutoFocus: Bool
    @State private var hasAutoFocused = false
    @State private var isViewAppeared = false
    @State private var autoFocusTask: Task<Void, Never>? = nil
    
    public init(conversation: ConversationItem, isNewConversation: Bool = false) {
        if conversation.isDraft {
            self.shouldAutoFocus = true
            if let draftSession = CacheManager.shared.getLocalDraftSession(id: conversation.id) {
                _viewModel = State(initialValue: ChatViewModel(draftSession: draftSession))
            } else if let project = conversation.draftProject {
                let session = LocalDraftSession(
                    id: conversation.id,
                    project: project,
                    draftText: CacheManager.shared.getDraft(for: conversation.id)
                )
                _viewModel = State(initialValue: ChatViewModel(draftSession: session))
            } else {
                let isNewOrEmpty = isNewConversation || conversation.stepCount == 0
                _viewModel = State(initialValue: ChatViewModel(
                    cascadeId: conversation.id,
                    initialTitle: conversation.title,
                    isNewConversation: isNewOrEmpty
                ))
            }
        } else {
            let isNewOrEmpty = isNewConversation || conversation.stepCount == 0
            self.shouldAutoFocus = isNewOrEmpty
            _viewModel = State(initialValue: ChatViewModel(
                cascadeId: conversation.id,
                initialTitle: conversation.title,
                isNewConversation: isNewOrEmpty
            ))
        }
    }
    
    public init(draftSession: LocalDraftSession) {
        self.shouldAutoFocus = true
        _viewModel = State(initialValue: ChatViewModel(
            draftSession: draftSession
        ))
    }
    
    public init(draftProject: ProjectItem) {
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
        .onChange(of: selectedPhotoItems) { _, items in
            guard !items.isEmpty else { return }
            Task {
                var loaded: [Data] = []
                for item in items {
                    if let data = try? await item.loadTransferable(type: Data.self) {
                        if let uiImage = UIImage(data: data) {
                            let maxDim: CGFloat = 2048
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
                            if let jpeg = targetImage.jpegData(compressionQuality: 0.8) {
                                loaded.append(jpeg)
                                continue
                            }
                        }
                        loaded.append(data)
                    }
                }
                guard !loaded.isEmpty else { return }
                viewModel.appendDraftImages(loaded)
                selectedPhotoItems = []
            }
        }
        .onDisappear {
            isViewAppeared = false
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
                .presentationDragIndicator(.hidden)
        }
        .sheet(isPresented: Binding(
            get: { viewModel.quickLookURL != nil },
            set: { if !$0 { viewModel.closeQuickLook() } }
        )) {
            if let qlURL = viewModel.quickLookURL {
                QuickLookPreviewSheet(url: qlURL, title: viewModel.quickLookTitle) {
                    viewModel.closeQuickLook()
                }
                .presentationDragIndicator(.hidden)
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
                .presentationDragIndicator(.hidden)
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
    }
    
    @ViewBuilder
    private var contentArea: some View {
        if viewModel.isLoading && viewModel.messages.isEmpty && !viewModel.isNewConversation {
            loadingStateView
        } else if let err = viewModel.errorMessage, viewModel.messages.isEmpty && !viewModel.isNewConversation {
            errorStateView(err: err)
        } else {
            ScrollViewReader { proxy in
                messagesScrollView(proxy: proxy)
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
    private func messagesScrollView(proxy: ScrollViewProxy) -> some View {
        ScrollView {
            messagesList(proxy: proxy)
        }
        .defaultScrollAnchor(.bottom)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .background(Color(uiColor: .systemBackground))
        .contentShape(Rectangle())
        .simultaneousGesture(
            TapGesture().onEnded {
                if isInputFocused {
                    isInputFocused = false
                }
            }
        )
        .scrollDismissesKeyboard(.interactively)
        .refreshable {
            await viewModel.loadMessages()
        }
        .onAppear {
            DispatchQueue.main.asyncAfter(deadline: .now() + 0.08) {
                initialAlignmentIfNeeded(proxy: proxy)
            }
            DispatchQueue.main.asyncAfter(deadline: .now() + 0.32) {
                initialAlignmentIfNeeded(proxy: proxy)
            }
        }
        .onChange(of: viewModel.isLoading) { _, loading in
            if !loading {
                DispatchQueue.main.asyncAfter(deadline: .now() + 0.08) {
                    initialAlignmentIfNeeded(proxy: proxy)
                }
                if canScheduleAutoFocus {
                    scheduleAutoFocus(delay: 0.2)
                }
            }
        }
        .onChange(of: viewModel.scrollToTurnStartTrigger) { _, _ in
            scrollToTurnStart(proxy: proxy, animated: true)
        }
        .onChange(of: viewModel.messages.last?.id) { _, lastId in
            guard lastId != nil else { return }
            if !hasInitiallyAligned {
                initialAlignmentIfNeeded(proxy: proxy)
                return
            }
            scrollToBottom(proxy: proxy, animated: true)
        }
        .onChange(of: isInputFocused) { _, focused in
            DispatchQueue.main.asyncAfter(deadline: .now() + 0.1) {
                scrollToBottom(proxy: proxy, animated: true)
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
        VStack(spacing: 8) {
            if viewModel.hasMore {
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
                    .transition(.opacity.combined(with: .scale(scale: 0.95, anchor: .topLeading)))
            }
            
            Color.clear
                .frame(height: 4)
                .id("BOTTOM_ANCHOR")
        }
        .padding(.vertical, 12)
        .frame(maxWidth: .infinity)
        .scrollTargetLayout()
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
        
        let decodedFileName: String = {
            if let last = url.lastPathComponent.removingPercentEncoding, !last.isEmpty {
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
        
        // 2. Presentations & Office documents (PPTX, PPT, KEY, DOCX, XLSX, PDF, HTML, etc.)
        let pathLower = url.path.lowercased()
        let isHTML = lower.hasSuffix(".html") || lower.hasSuffix(".htm") || pathLower.hasSuffix(".html") || pathLower.hasSuffix(".htm")
        let documentExtensions = [".pptx", ".ppt", ".key", ".pdf", ".docx", ".doc", ".xlsx", ".xls", ".numbers", ".pages", ".html", ".htm"]
        if documentExtensions.contains(where: { lower.hasSuffix($0) || pathLower.hasSuffix($0) }) {
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
                viewModel.openMarkdownViewer(uri: data.uri, title: data.title)
            }
        )
    }
    
    private var inputBar: some View {
        VStack(alignment: .leading, spacing: 8) {
            // Quick action chips at top of input box (➕ and Model Switch placed in front of Commit and Push)
            ScrollView(.horizontal, showsIndicators: false) {
                HStack(spacing: 8) {
                    // 1. Add Image ➕ (PhotosPicker)
                    PhotosPicker(selection: $selectedPhotoItems, maxSelectionCount: 5, matching: .images) {
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
                    
                    // 4. Proceed Button (Plan is opened directly via implementation_plan.md button in chat)
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
            .animation(.easeInOut(duration: 0.2), value: viewModel.canProceed)
            
            // Image previews strip
            if !viewModel.selectedImageData.isEmpty {
                ScrollView(.horizontal, showsIndicators: false) {
                    HStack(spacing: 10) {
                        ForEach(Array(viewModel.selectedImageData.enumerated()), id: \.offset) { index, data in
                            if let uiImage = UIImage(data: data) {
                                ZStack(alignment: .topTrailing) {
                                    Image(uiImage: uiImage)
                                        .resizable()
                                        .scaledToFill()
                                        .frame(width: 52, height: 52)
                                        .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
                                        .overlay(
                                            RoundedRectangle(cornerRadius: 10, style: .continuous)
                                                .stroke(Color.secondary.opacity(0.25), lineWidth: 1)
                                        )
                                    
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
                TextField((viewModel.isRunning || viewModel.isAwaitingResponse) ? "向队列添加指令..." : "发送对 Agent 的指令...", text: $viewModel.inputText, axis: .vertical)
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
        viewModel.inputText = ""
        viewModel.selectedImageData = []
        selectedPhotoItems = []
        Task {
            let success = await viewModel.sendMessage(text: text, images: images)
            if !success && !images.isEmpty {
                viewModel.selectedImageData = images
            }
        }
    }
    
    private func removeImage(at index: Int) {
        viewModel.removeDraftImage(at: index)
        if index < selectedPhotoItems.count {
            selectedPhotoItems.remove(at: index)
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
    
    private func initialAlignmentIfNeeded(proxy: ScrollViewProxy) {
        guard !hasInitiallyAligned, !viewModel.messages.isEmpty else { return }
        hasInitiallyAligned = true
        smartScroll(proxy: proxy, animated: false)
    }
    
    private func smartScroll(proxy: ScrollViewProxy, animated: Bool = false) {
        if viewModel.isAwaitingResponse || viewModel.isRunning {
            scrollToBottom(proxy: proxy, animated: animated)
        } else if let _ = viewModel.latestAgentMessageId {
            scrollToTurnStart(proxy: proxy, animated: animated)
        } else {
            scrollToBottom(proxy: proxy, animated: animated)
        }
    }
    
    private func scrollToTurnStart(proxy: ScrollViewProxy, animated: Bool = true) {
        // Only target agent response message start when opening a completed chat
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
                
                Text("已连接工作区，在下方输入指令开启对话")
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
    public let onDismiss: () -> Void
    public let onProceed: (() -> Void)?
    public let onRetry: (() -> Void)?
    
    public init(
        data: MarkdownFileViewerData,
        onDismiss: @escaping () -> Void,
        onProceed: (() -> Void)? = nil,
        onRetry: (() -> Void)? = nil
    ) {
        self.data = data
        self.onDismiss = onDismiss
        self.onProceed = onProceed
        self.onRetry = onRetry
    }
    
    public var body: some View {
        VStack(spacing: 0) {
            // Floating grab handle hinting pull-down dismissal
            Capsule()
                .fill(Color(uiColor: .tertiaryLabel))
                .frame(width: 38, height: 5)
                .padding(.top, 10)
                .padding(.bottom, 20)
            
            // Header title (clean centered, dismiss via pull-down gesture)
            Text(data.title.isEmpty ? "文档详情" : data.title)
                .font(.system(size: 17, weight: .bold))
                .foregroundColor(.primary)
                .lineLimit(1)
                .frame(maxWidth: .infinity)
                .padding(.horizontal, 16)
                .padding(.bottom, 12)
            
            Divider()
            
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
                            MarkdownContentView(content: data.content)
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
        }
        .presentationDragIndicator(.hidden)
    }
}

