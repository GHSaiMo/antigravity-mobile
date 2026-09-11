import SwiftUI
import PhotosUI

public struct ChatView: View {
    @Environment(\.scenePhase) private var scenePhase
    @State private var viewModel: ChatViewModel
    @FocusState private var isInputFocused: Bool
    @State private var hasInitiallyAligned = false
    @State private var selectedPhotoItems: [PhotosPickerItem] = []
    @State private var selectedImageData: [Data] = []
    private let shouldAutoFocus: Bool
    @State private var hasAutoFocused = false
    @State private var isViewAppeared = false
    @State private var autoFocusTask: Task<Void, Never>? = nil
    
    public init(conversation: ConversationItem, isNewConversation: Bool = false) {
        let isNewOrEmpty = isNewConversation || conversation.stepCount == 0
        self.shouldAutoFocus = isNewOrEmpty
        _viewModel = State(initialValue: ChatViewModel(
            cascadeId: conversation.id,
            initialTitle: conversation.title,
            isNewConversation: isNewOrEmpty
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
            // Content area
            if viewModel.isLoading && viewModel.messages.isEmpty && !viewModel.isNewConversation {
                VStack(spacing: 14) {
                    ProgressView()
                        .scaleEffect(1.2)
                    Text("正在同步会话历史与步骤...")
                        .font(.system(size: 13.5))
                        .foregroundColor(.secondary)
                }
                .frame(maxWidth: .infinity, maxHeight: .infinity)
            } else if let err = viewModel.errorMessage, viewModel.messages.isEmpty && !viewModel.isNewConversation {
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
            } else {
                // Message stream
                ScrollViewReader { proxy in
                    ScrollView {
                        VStack(spacing: 8) {
                            // Load older messages button
                            if viewModel.hasMore {
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
                            
                            if viewModel.messages.isEmpty {
                                emptyStateView
                            }
                            
                            ForEach(Array(viewModel.messages.enumerated()), id: \.element.id) { index, message in
                                let isLast = (index == viewModel.messages.count - 1)
                                let isActive = isLast && (viewModel.isRunning || viewModel.isAwaitingResponse)
                                MessageBubbleView(message: message, isActiveToolBatch: isActive)
                                    .id(message.id)
                            }
                            
                            // Agent thinking & executing indicator animation (shown while awaiting before tools/response arrive)
                            if (viewModel.isAwaitingResponse || viewModel.isRunning) && (viewModel.messages.last?.isToolBatch != true) {
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
                        // Stage 1: Quick pre-alignment for cached messages before push completes (80ms)
                        DispatchQueue.main.asyncAfter(deadline: .now() + 0.08) {
                            initialAlignmentIfNeeded(proxy: proxy)
                        }
                        // Stage 2: Fallback calibration after push finishes if Stage 1 missed (320ms)
                        DispatchQueue.main.asyncAfter(deadline: .now() + 0.32) {
                            initialAlignmentIfNeeded(proxy: proxy)
                        }
                    }
                    .onChange(of: viewModel.isLoading) { _, loading in
                        if !loading {
                            // If initial alignment was waiting on network load, perform it now
                            DispatchQueue.main.asyncAfter(deadline: .now() + 0.08) {
                                initialAlignmentIfNeeded(proxy: proxy)
                            }
                            if !hasAutoFocused && autoFocusTask == nil && (shouldAutoFocus || (viewModel.messages.isEmpty && viewModel.stepCount == 0)) {
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
                        if focused {
                            // 软键盘弹起时平滑过渡贴合底部
                            DispatchQueue.main.asyncAfter(deadline: .now() + 0.1) {
                                scrollToBottom(proxy: proxy, animated: true)
                            }
                        } else {
                            // 输入法收起时校准底部对齐，消除视口悬空留白
                            DispatchQueue.main.asyncAfter(deadline: .now() + 0.1) {
                                scrollToBottom(proxy: proxy, animated: true)
                            }
                        }
                    }
                }
            }
            
            // Error banner for active chats or drafting new conversations
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
            
            // Floating Interaction Card (Permissions / Prompts / Decision)
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
            
            // Floating Running Tasks Card (Desktop Parity)
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
            
            // Floating Queued Messages Card
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
            
            // Bottom input bar
            inputBar
        }
        .navigationTitle(viewModel.currentTitle)
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            if viewModel.hasError {
                ToolbarItem(placement: .topBarTrailing) {
                    HStack(spacing: 4) {
                        Image(systemName: "exclamationmark.circle.fill")
                            .font(.system(size: 11, weight: .bold))
                        Text("error")
                            .font(.system(size: 11, weight: .bold, design: .monospaced))
                    }
                    .foregroundColor(.red)
                    .padding(.horizontal, 8)
                    .padding(.vertical, 3)
                    .background(Color.red.opacity(0.15))
                    .clipShape(Capsule())
                }
            }
        }
        .onAppear {
            isViewAppeared = true
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
                Task {
                    await viewModel.resumeActiveSession()
                }
            } else if newPhase == .background {
                viewModel.handleAppBackground()
            }
        }
        .onReceive(NotificationCenter.default.publisher(for: UIApplication.didBecomeActiveNotification)) { _ in
            Task {
                await viewModel.resumeActiveSession()
            }
        }
        .onReceive(NotificationCenter.default.publisher(for: UIApplication.didEnterBackgroundNotification)) { _ in
            viewModel.handleAppBackground()
        }
        .onChange(of: selectedPhotoItems) { _, items in
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
                selectedImageData = loaded
            }
        }
        .onDisappear {
            isViewAppeared = false
            autoFocusTask?.cancel()
            autoFocusTask = nil
            viewModel.disconnectStream()
        }
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
                        Task {
                            await viewModel.toggleModel()
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
                    
                    // 4. Proceed Button
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
            if !selectedImageData.isEmpty {
                ScrollView(.horizontal, showsIndicators: false) {
                    HStack(spacing: 10) {
                        ForEach(Array(selectedImageData.enumerated()), id: \.offset) { index, data in
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
        viewModel.isSending || (viewModel.inputText.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty && selectedImageData.isEmpty)
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
        let images = selectedImageData
        guard !text.isEmpty || !images.isEmpty else { return }
        viewModel.inputText = ""
        selectedImageData = []
        selectedPhotoItems = []
        Task {
            let success = await viewModel.sendMessage(text: text, images: images)
            if !success && !images.isEmpty {
                selectedImageData = images
            }
        }
    }
    
    private func removeImage(at index: Int) {
        guard index < selectedImageData.count else { return }
        selectedImageData.remove(at: index)
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
