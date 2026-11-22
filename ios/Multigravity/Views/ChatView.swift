import SwiftUI
import PhotosUI
import AVFoundation

public struct ChatView: View {
    @Environment(\.dismiss) var dismiss
    @Environment(\.scenePhase) var scenePhase
    @State var viewModel: ChatViewModel
    @Environment(\.isSplitDetail) var isSplitDetail
    @Environment(\.horizontalSizeClass) var horizontalSizeClass
    @State var isInputFocused: Bool = false
    @State var hasInitiallyAligned = false
    /// 进入子代理等子页面再返回时，保持原滚动位置，不重新对齐到底部。
    @State var preserveScrollOnReturn = false
    @State var scrollHUD = ChatScrollHUDState()
    @State var currentViewportHeight: CGFloat = 0
    @State var cardToggleTrigger = 0
    /// 锁定当前会话列表容器类型（VStack vs LazyVStack），避免流式跨过阈值瞬间整棵子树与 WKWebView 被销毁重建。
    @State var useLazyMessagesStack = false
    @State var showCameraPicker = false
    @State var showDocumentScanner = false
    @State var showCameraUnavailableAlert = false
    @State var showCameraPermissionAlert = false
    @State var showMicrophonePermissionAlert = false
    @State var showOtherSessionRecordingAlert = false
    @State var showSendQueuedNowConfirmAlert = false
    @State var queuedItemToSendNow: QueuedMessageItem? = nil
    @ObservedObject var audioRecorder = AudioRecorderManager.shared
    @ObservedObject var voiceCall = VoiceCallManager.shared
    @State var dictationManager = AudioDictationManager()
    @State var narrationPlayer = NarrationAudioPlayer()
    @State var showVoiceTroubleshootBubble = false
    @State var lastVoiceTroubleshootBubbleAt: Date = .distantPast
    @State var previewDraftGallery: ImageGalleryData? = nil
    @State var showAttachmentSheet = false
    @State var showNavigatorSheet = false
    @State var showMcpSkillsSheet = false
    @State var mcpSkillsInitialTabOverride: McpSkillsSheet.Tab? = nil
    @State var showSwarmSheet = false
    @State var showContextAuditSheet = false
    @State var showWorkflowTemplateSheet = false
    @State var showAutoPermissionConfirm = false
    @State var workflowSheetStartInEditMode = false
    @State var modelPickerState = ModelSlidePickerState()
    @State var workflowPickerState = WorkflowSlidePickerState()
    @State var commitPickerState = CommitSlidePickerState()
    @State var selectedSubagentDetail: SubagentItem? = nil
    @State var showBtwSheet: Bool = false
    @State var btwInitialQuote: String? = nil
    @State var selectedGitHubPRItem: IdentifiableURL? = nil
    @State var isSlashPickerDismissed: Bool = false
    @State var highlightedTurnMessageId: String? = nil
    @State var targetScrollTurnId: String? = nil
    @State var jumpingToTurnIndex: Int? = nil
    @State var jumpTransitionDimmed: Bool = false
    @State var activeTurnJumpTask: Task<Void, Never>? = nil
    @State var chatAreaHeight: CGFloat = 0
    let shouldAutoFocus: Bool
    let initialConversation: ConversationItem?
    let initialIsUnread: Bool
    let initialStatus: ConversationItem.ConversationStatus?
    @State var hasAutoFocused = false
    @State var isViewAppeared = false
    @State var autoFocusTask: Task<Void, Never>? = nil
    
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
                let vm = ChatViewModel(
                    cascadeId: conversation.id,
                    initialTitle: conversation.title,
                    workspaceName: conversation.workspaceName,
                    isNewConversation: isNewOrEmpty,
                    isUnread: conversation.isUnread,
                    conversationStatus: conversation.status
                )
                _viewModel = State(initialValue: vm)
                _useLazyMessagesStack = State(initialValue: Self.shouldUseLazyStack(for: vm.messages))
            }
        } else {
            let isNewOrEmpty = isNewConversation || conversation.stepCount == 0
            self.shouldAutoFocus = isNewOrEmpty
            let vm = ChatViewModel(
                cascadeId: conversation.id,
                initialTitle: conversation.title,
                workspaceName: conversation.workspaceName,
                isNewConversation: isNewOrEmpty,
                isUnread: conversation.isUnread,
                conversationStatus: conversation.status
            )
            _viewModel = State(initialValue: vm)
            _useLazyMessagesStack = State(initialValue: Self.shouldUseLazyStack(for: vm.messages))
        }
    }

    /// 根据消息数量、文本总字符数与工具调用总数判定是否启用 LazyVStack：
    /// 中短会话保持确定性零估算误差的 VStack（配合 EquatableMessageRow 与 Markdown 缓存开销极低），
    /// 仅在超长历史或重度工具会话中启用带 scrollTargetLayout 锚点保护的 LazyVStack。
    static func shouldUseLazyStack(for messages: [ChatMessage]) -> Bool {
        if messages.count > 80 {
            return true
        }
        var totalChars = 0
        var totalTools = 0
        for msg in messages {
            totalChars += msg.content.count
            totalTools += msg.toolCount
            if totalChars > 60_000 || totalTools > 120 {
                return true
            }
        }
        return false
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
        applyPresentationModifiers(
            applyLifecycleModifiers(rootChatLayout)
        )
    }

    @ViewBuilder
    private var rootChatLayout: some View {
        VStack(spacing: 0) {
            ZStack(alignment: .bottom) {
                VStack(spacing: 0) {
                    if let brief = viewModel.swarmBrief, brief.active {
                        SwarmBannerView(brief: brief) {
                            showSwarmSheet = true
                        }
                        .padding(.horizontal, 16)
                        .padding(.vertical, 6)
                    }
                    contentArea
                    errorBanner
                    floatingCards
                        .readableChatWidth(horizontalSizeClass == .regular)
                }
                
                floatingSlashCommandPopup
                    .readableChatWidth(horizontalSizeClass == .regular)
                
                floatingMentionPopup
                    .readableChatWidth(horizontalSizeClass == .regular)
            }
            .onGeometryChange(for: CGFloat.self) { proxy in
                proxy.size.height
            } action: { newHeight in
                if newHeight > 0, abs(chatAreaHeight - newHeight) > 1 {
                    chatAreaHeight = newHeight
                }
            }
            if viewModel.isSubagentSession || initialConversation?.isSubagent == true {
                SubagentReadOnlyBar(
                    role: viewModel.subagentRole,
                    isRunning: viewModel.isRunning,
                    canStop: GatewayCompatStore.shared.isAvailable(GatewayFeature.subagents),
                    onStop: {
                        let me = SubagentItem(conversationId: viewModel.cascadeId, status: "running")
                        Task { await viewModel.stopSubagent(me) }
                    }
                )
            } else {
                inputBar
            }
        }
        .coordinateSpace(name: "chatRoot")
        .onGeometryChange(for: CGSize.self) { proxy in
            proxy.size
        } action: { newSize in
            modelPickerState.containerSize = newSize
            workflowPickerState.containerSize = newSize
            commitPickerState.containerSize = newSize
        }
        .overlay {
            ModelSlidePickerOverlay(state: modelPickerState) { modelId in
                withAnimation(.easeInOut(duration: 0.15)) {
                    viewModel.selectModel(modelId)
                }
            }
            WorkflowSlidePickerOverlay(
                state: workflowPickerState,
                onSelectTemplate: { templatePrompt in
                    viewModel.inputText = templatePrompt
                    viewModel.pendingSelectFirstPlaceholder = true
                    isInputFocused = true
                },
                onOpenSetup: {
                    isInputFocused = false
                    workflowSheetStartInEditMode = true
                    showWorkflowTemplateSheet = true
                }
            )
            CommitSlidePickerOverlay(
                state: commitPickerState,
                onSelect: { action in
                    viewModel.inputText = action
                    isInputFocused = true
                }
            )
        }
        .onChange(of: modelPickerState.isPresented) { _, presented in
            if presented {
                if workflowPickerState.isPresented { workflowPickerState.dismiss() }
                if commitPickerState.isPresented { commitPickerState.dismiss() }
            }
        }
        .onChange(of: workflowPickerState.isPresented) { _, presented in
            if presented {
                if modelPickerState.isPresented { modelPickerState.dismiss() }
                if commitPickerState.isPresented { commitPickerState.dismiss() }
            }
        }
        .onChange(of: commitPickerState.isPresented) { _, presented in
            if presented {
                if modelPickerState.isPresented { modelPickerState.dismiss() }
                if workflowPickerState.isPresented { workflowPickerState.dismiss() }
            }
        }
        .navigationTitle(viewModel.currentTitle)
        .navigationBarTitleDisplayMode(.inline)
        .navigationBarBackButtonHidden(true)
        .toolbar {
            chatToolbarContent
        }
    }

    @ToolbarContentBuilder
    private var chatToolbarContent: some ToolbarContent {
        // In the iPad split view the detail column has no back navigation.
        if !isSplitDetail {
            ToolbarItem(placement: .topBarLeading) {
                Button(action: {
                    dismiss()
                }) {
                    Image(systemName: "chevron.left")
                        .font(.system(size: 16, weight: .semibold))
                }
            }
        }

        ToolbarItem(placement: .principal) {
            ChatToolbarPrincipalView(
                viewModel: viewModel,
                scrollHUD: scrollHUD,
                onOpenNavigator: {
                    UIImpactFeedbackGenerator(style: .light).impactOccurred()
                    showNavigatorSheet = true
                },
                onOpenContextAudit: {
                    UIImpactFeedbackGenerator(style: .light).impactOccurred()
                    showContextAuditSheet = true
                }
            )
        }

        ToolbarItem(placement: .topBarTrailing) {
            let isAutoPermission = viewModel.permissionState.preset == "auto"
            Menu {
                Section("全局权限放行模式 (\(viewModel.permissionState.localizedLabel))") {
                    Button {
                        Task { await viewModel.updatePermissionPreset("strict") }
                    } label: {
                        Label("严格审批", systemImage: viewModel.permissionState.preset == "strict" ? "checkmark" : "lock.shield")
                    }
                    Button {
                        Task { await viewModel.updatePermissionPreset("sandbox") }
                    } label: {
                        Label("离席沙箱放行", systemImage: viewModel.permissionState.preset == "sandbox" ? "checkmark" : "shield.lefthalf.filled")
                    }
                    Button {
                        if isAutoPermission {
                            Task { await viewModel.updatePermissionPreset("auto") }
                        } else {
                            showAutoPermissionConfirm = true
                        }
                    } label: {
                        Label("全自动放行", systemImage: isAutoPermission ? "checkmark" : "bolt.shield")
                    }
                }
                Divider()
                Button {
                    btwInitialQuote = nil
                    showBtwSheet = true
                } label: {
                    Label("旁路快问", systemImage: "bubble.left.and.exclamationmark.bubble.right")
                }
                Button {
                    showMcpSkillsSheet = true
                } label: {
                    Label("打开工具与任务看板", systemImage: "wrench.and.screwdriver")
                }
            } label: {
                let iconName: String = {
                    if viewModel.hasAnyBackgroundTasks { return "terminal" }
                    return isAutoPermission ? "bolt.shield.fill" : "wrench.and.screwdriver"
                }()
                Image(systemName: iconName)
                    .font(.system(size: 14, weight: .medium))
                    .foregroundColor(isAutoPermission ? .orange : .indigo)
            } primaryAction: {
                UIImpactFeedbackGenerator(style: .light).impactOccurred()
                showMcpSkillsSheet = true
            }
            .accessibilityLabel(viewModel.hasAnyBackgroundTasks ? String(localized: "打开后台任务与工具看板") : String(localized: "打开已连接工具与技能看板"))
            .accessibilityValue(viewModel.permissionState.localizedLabel)
            .accessibilityHint("短按打开工具与任务看板，长按切换全局权限模式")
        }
    }

    private func runInitialChatTask() async {
        ChatViewModel.activeCascadeId = viewModel.cascadeId
        if let url = AppSettings.shared.serverURL {
            Task { @MainActor in
                await ModelCatalogStore.shared.refreshIfNeeded(settings: AppSettings.shared, baseURL: url)
                if !viewModel.isDraftConversation && !viewModel.hasUserManuallySelectedModel && !viewModel.hasResolvedSessionModel,
                   let desktopModel = ModelCatalogStore.shared.catalog?.desktopActiveModel,
                   !desktopModel.isEmpty {
                    viewModel.syncModel(from: desktopModel)
                    if let dName = ModelCatalogStore.shared.catalog?.desktopActiveModelName, !dName.isEmpty {
                        viewModel.activeModelName = dName
                    }
                }
            }
            Task {
                await WorkflowTemplateStore.shared.refresh()
            }
        }
        Task {
            await viewModel.refreshPermissionSettings()
            await viewModel.refreshProjectTrust()
        }
        await viewModel.loadMessages()
        viewModel.connectStream()
    }

    @ViewBuilder
    private func applyLifecycleModifiers<Content: View>(_ content: Content) -> some View {
        content
            .background(SwipeBackEnabler())
            .onChange(of: ShareInbox.shared.deliveryTick) { _, _ in
                consumeSharedFiles()
            }
            .onAppear {
                handleViewAppear()
            }
            .task {
                await runInitialChatTask()
            }
            .onReceive(NotificationCenter.default.publisher(for: .permissionPresetDidChange)) { notif in
                if let state = notif.object as? PermissionStateResponse {
                    viewModel.permissionState = state
                }
            }
            .onChange(of: viewModel.pendingInteraction != nil) { _, hasPending in
                if hasPending {
                    isInputFocused = false
                    UIApplication.shared.sendAction(#selector(UIResponder.resignFirstResponder), to: nil, from: nil, for: nil)
                }
            }
            .onChange(of: scenePhase) { _, newPhase in
                handleScenePhaseChange(newPhase)
            }
            .onReceive(NotificationCenter.default.publisher(for: UIApplication.didBecomeActiveNotification)) { _ in
                handleAppDidBecomeActive()
            }
            .onReceive(NotificationCenter.default.publisher(for: UIApplication.didEnterBackgroundNotification)) { _ in
                handleAppDidEnterBackground()
            }
            .onReceive(NotificationCenter.default.publisher(for: .audioRecordingDidFinishFromLiveActivity)) { notif in
                handleLiveActivityRecordingFinished(notif)
            }
            .onDisappear {
                handleViewDisappear()
            }
            .onChange(of: voiceCall.callState) { _, newState in
                handleVoiceCallStateChange(newState)
            }
            .onChange(of: voiceCall.isInCall) { _, inCall in
                if inCall { showVoiceTroubleshootBubble = false }
            }
            .onChange(of: viewModel.activeModel) { _, _ in
                if voiceCall.isInCall {
                    voiceCall.updatePlanModel(viewModel.activeModelEnum)
                }
            }
            .environment(\.openURL, OpenURLAction { url in
                handleURLTap(url)
            })
            .environment(\.currentCascadeId, viewModel.cascadeId)
    }

    @ViewBuilder
    private func applyPresentationModifiers<Content: View>(_ content: Content) -> some View {
        applySecondarySheetsAndAlerts(applyPrimarySheets(content))
    }

    @ViewBuilder
    private func applyPrimarySheets<Content: View>(_ content: Content) -> some View {
        content
            .sheet(isPresented: $showAttachmentSheet) {
                AttachmentPickerSheet(
                    remainingImageSlots: max(0, 5 - viewModel.selectedImageData.count),
                    onPhotosPicked: { assets in handlePickedAssets(assets) },
                    onOpenAudioRecorder: {
                        DispatchQueue.main.asyncAfter(deadline: .now() + 0.3) { handleAudioRecordAction() }
                    },
                    onOpenCamera: {
                        // Wait for the sheet to finish dismissing before presenting the camera.
                        DispatchQueue.main.asyncAfter(deadline: .now() + 0.4) { handleCameraAction() }
                    },
                    onOpenDocumentScanner: {
                        DispatchQueue.main.asyncAfter(deadline: .now() + 0.4) { handleDocumentScannerAction() }
                    },
                    onFilesPicked: { urls in viewModel.addFiles(from: urls) }
                )
            }
            .alert("无法添加文件", isPresented: isAttachmentNoticePresented) {
                Button("好", role: .cancel) {}
            } message: {
                Text(viewModel.attachmentNotice ?? "")
            }
            .sheet(item: $viewModel.viewingMarkdownFile, onDismiss: {
                viewModel.closeMarkdownViewer()
            }) { (item: MarkdownFileViewerData) in
                renderMarkdownViewer(data: item)
                    .presentationDragIndicator(.visible)
            }
            .sheet(isPresented: isQuickLookPresented) {
                if let qlURL = viewModel.quickLookURL {
                    QuickLookPreviewSheet(url: qlURL, title: viewModel.quickLookTitle) {
                        viewModel.closeQuickLook()
                    }
                    .presentationDragIndicator(.visible)
                }
            }
            .sheet(isPresented: isHtmlPreviewPresented) {
                if let htmlURL = viewModel.htmlPreviewURL {
                    HTMLPreviewSheet(url: htmlURL, title: viewModel.htmlPreviewTitle) {
                        viewModel.closeHTMLPreview()
                    }
                    .presentationDragIndicator(.visible)
                }
            }
            .fullScreenCover(item: $viewModel.weChatArchiveItem) { item in
                WeChatChatArchiveSheet(
                    item: item,
                    apiClient: viewModel.apiClient,
                    baseURL: viewModel.settings.serverURL,
                    cascadeId: viewModel.cascadeId,
                    onFallbackToQuickLook: {
                        DispatchQueue.main.asyncAfter(deadline: .now() + 0.35) {
                            viewModel.downloadAndPreviewDocument(uri: item.uri, fileName: item.fileName, isHTML: false)
                        }
                    },
                    onDismiss: {
                        viewModel.closeWeChatArchive()
                    }
                )
                .presentationBackground(.clear)
            }
            .sheet(isPresented: $showNavigatorSheet) {
                ConversationNavigatorSheet(
                    turns: viewModel.turns,
                    chapters: viewModel.chapters,
                    currentTurnIndex: scrollHUD.currentVisibleTurnIndex,
                    isLoadingChapterTitles: viewModel.isLoadingChapterTitles,
                    onSelectTurn: { turn in
                        jumpToTurn(turn)
                    },
                    onJumpToTop: {
                        if let firstTurn = viewModel.turns.first {
                            jumpToTurn(firstTurn)
                        } else if let firstId = viewModel.messages.first?.id {
                            scrollHUD.lockedTurnIndex = 1
                            scrollHUD.currentVisibleTurnIndex = 1
                            targetScrollTurnId = firstId
                        }
                    },
                    onJumpToBottom: {
                        let lastTurn = viewModel.turns.last?.turnIndex ?? 1
                        scrollHUD.lockedTurnIndex = nil
                        scrollHUD.currentVisibleTurnIndex = lastTurn
                        targetScrollTurnId = "BOTTOM_ANCHOR"
                    }
                )
                .onAppear {
                    viewModel.refreshChapterTitles()
                }
                .presentationDetents([.large])
                .presentationDragIndicator(.visible)
            }
    }

    @ViewBuilder
    private func applySecondarySheetsAndAlerts<Content: View>(_ content: Content) -> some View {
        content
            .sheet(item: inAppBrowserItem) { item in
                SafariWebViewSheet(url: item.url) {
                    viewModel.closeInAppBrowser()
                }
                .ignoresSafeArea()
            }
            .sheet(item: $selectedGitHubPRItem) { item in
                GitHubPRDetailSheet(url: item.url) { browserURL in
                    viewModel.openInAppBrowser(url: browserURL)
                }
            }
            .sheet(isPresented: $showBtwSheet, onDismiss: {
                btwInitialQuote = nil
            }) {
                BtwSideQuestionSheet(
                    parentCascadeId: viewModel.cascadeId,
                    activeModel: viewModel.activeModel,
                    initialQuote: btwInitialQuote,
                    onInsertIntoMain: { inserted in
                        let existing = viewModel.inputText.trimmingCharacters(in: .whitespacesAndNewlines)
                        viewModel.inputText = existing.isEmpty ? inserted : (existing + "\n\n" + inserted)
                        viewModel.focusInputTrigger &+= 1
                    },
                    onOpenStandalone: { targetId in
                        NotificationCenter.default.post(
                            name: .openConversationFromTaskCenter,
                            object: targetId
                        )
                    }
                )
            }
            .sheet(isPresented: $showMcpSkillsSheet, onDismiss: {
                mcpSkillsInitialTabOverride = nil
            }) {
                McpSkillsSheet(
                    initialTab: mcpSkillsInitialTabOverride ?? (viewModel.hasAnyBackgroundTasks ? .tasks : .mcp),
                    currentConversationId: viewModel.cascadeId,
                    hasActiveSwarm: viewModel.isSwarmActive,
                    onSelectConversation: { targetId in
                        NotificationCenter.default.post(
                            name: .openConversationFromTaskCenter,
                            object: targetId
                        )
                    },
                    onTasksChanged: { updated in
                        viewModel.globalBackgroundTasks = updated
                    },
                    onOpenSwarm: {
                        DispatchQueue.main.asyncAfter(deadline: .now() + 0.25) {
                            showSwarmSheet = true
                        }
                    }
                )
                .presentationDetents([.large])
                .presentationDragIndicator(.visible)
            }
            .sheet(isPresented: $showSwarmSheet) {
                SwarmSheet(
                    cascadeId: viewModel.cascadeId,
                    onSelectConversation: { targetId in
                        NotificationCenter.default.post(
                            name: .openConversationFromTaskCenter,
                            object: targetId
                        )
                    }
                )
                .presentationDetents([.large])
                .presentationDragIndicator(.visible)
            }
            .sheet(isPresented: $showContextAuditSheet) {
                if let meter = viewModel.contextMeter {
                    ContextAuditSheetView(
                        meter: meter,
                        canCompact: !viewModel.cascadeId.hasPrefix("local_draft_") && !viewModel.isSubagentSession,
                        onCompact: {
                            Task { _ = await viewModel.sendMessage(text: "/compact") }
                        },
                        onStartNewSession: {
                            NotificationCenter.default.post(
                                name: .startNewSessionForWorkspace,
                                object: viewModel.workspaceName
                            )
                        }
                    )
                    .presentationDetents([.fraction(0.42), .medium])
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

                            ZStack {
                                if viewModel.downloadBytesTotal > 0 {
                                    ProgressView(value: viewModel.downloadProgress)
                                        .progressViewStyle(.linear)
                                        .tint(.accentColor)
                                        .animation(.easeInOut(duration: 0.15), value: viewModel.downloadProgress)
                                } else {
                                    ProgressView()
                                        .progressViewStyle(.linear)
                                        .tint(.accentColor)
                                }
                            }
                            .frame(width: 150, height: 4)
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
            .fullScreenCover(isPresented: $showDocumentScanner) {
                DocumentScannerView(
                    onScanned: { scannedImages in
                        handleScannedImages(scannedImages)
                    },
                    onCancel: {
                        showDocumentScanner = false
                    }
                )
                .ignoresSafeArea()
            }
            .modifier(ChatPermissionAlertsModifier(
                showCameraUnavailableAlert: $showCameraUnavailableAlert,
                showCameraPermissionAlert: $showCameraPermissionAlert,
                showMicrophonePermissionAlert: $showMicrophonePermissionAlert,
                showOtherSessionRecordingAlert: $showOtherSessionRecordingAlert,
                audioRecorder: audioRecorder
            ))
            .alert("后台任务正在执行", isPresented: $showSendQueuedNowConfirmAlert) {
                Button("取消", role: .cancel) {
                    queuedItemToSendNow = nil
                }
                Button("仍要立即发送", role: .destructive) {
                    if let item = queuedItemToSendNow {
                        Task {
                            await viewModel.sendQueuedMessageNow(item: item)
                        }
                    }
                    queuedItemToSendNow = nil
                }
            } message: {
                Text("当前有正在运行的后台命令或子代理。立即发送将提前触发下一轮对话，可能会导致后台任务被中断或产生冲突。确定仍要立即发送吗？")
            }
            .alert("开启全自动放行？", isPresented: $showAutoPermissionConfirm) {
                Button("取消", role: .cancel) {}
                Button("确认开启", role: .destructive) {
                    Task { await viewModel.updatePermissionPreset("auto") }
                }
            } message: {
                Text("开启后 Agent 将自动执行终端命令并访问工作区外文件（同步作用于桌面端全部会话）。建议仅在可信或隔离环境中使用。")
            }
            .onReceive(NotificationCenter.default.publisher(for: .dismissAllPresentedModals)) { _ in
                dismissChatModals()
            }
    }

    private func dismissChatModals() {
        showAttachmentSheet = false
        showNavigatorSheet = false
        showMcpSkillsSheet = false
        mcpSkillsInitialTabOverride = nil
        showSwarmSheet = false
        showContextAuditSheet = false
        showWorkflowTemplateSheet = false
        workflowSheetStartInEditMode = false
        showCameraPicker = false
        showDocumentScanner = false
        showBtwSheet = false
        btwInitialQuote = nil
        selectedSubagentDetail = nil
        selectedGitHubPRItem = nil
        previewDraftGallery = nil
        showCameraUnavailableAlert = false
        showCameraPermissionAlert = false
        showMicrophonePermissionAlert = false
        showOtherSessionRecordingAlert = false
        showSendQueuedNowConfirmAlert = false
        queuedItemToSendNow = nil
        showAutoPermissionConfirm = false

        viewModel.closeMarkdownViewer()
        viewModel.closeQuickLook()
        viewModel.closeHTMLPreview()
        viewModel.closeWeChatArchive()
        viewModel.closeInAppBrowser()
        viewModel.closeChangesSheet()
        viewModel.closeGitSheet()
        viewModel.cancelUndoSheet()
        viewModel.exportedMarkdownFile = nil
    }
    
    private func checkPendingLiveRecording() {
        if let pending = audioRecorder.pendingLiveRecording, pending.cascadeId == viewModel.draftKey {
            audioRecorder.pendingLiveRecording = nil
            viewModel.addFiles(from: [pending.url])
            viewModel.saveCurrentDraft()
            if pending.sendImmediately {
                sendRecordedAudioWhenReady()
            }
        }
    }

    private func handleScenePhaseChange(_ newPhase: ScenePhase) {
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

    private func handleAppDidBecomeActive() {
        viewModel.restoreDraftsIfNeeded()
        Task {
            await viewModel.resumeActiveSession()
        }
    }

    private func handleAppDidEnterBackground() {
        viewModel.saveCurrentDraft()
        viewModel.handleAppBackground()
    }

    private func handleLiveActivityRecordingFinished(_ notif: Notification) {
        guard let userInfo = notif.userInfo,
              let targetCascadeId = userInfo["cascadeId"] as? String,
              targetCascadeId == viewModel.draftKey,
              let fileURL = userInfo["url"] as? URL else { return }
        
        audioRecorder.pendingLiveRecording = nil
        let sendImmediately = userInfo["sendImmediately"] as? Bool ?? false
        viewModel.addFiles(from: [fileURL])
        viewModel.saveCurrentDraft()
        if sendImmediately {
            sendRecordedAudioWhenReady()
        }
    }

    private func sendRecordedAudioWhenReady() {
        Task {
            for _ in 0..<100 {
                if viewModel.selectedFiles.allSatisfy({ $0.isUploaded }) { break }
                try? await Task.sleep(nanoseconds: 100_000_000)
            }
            handleSend()
        }
    }

    private var isAttachmentNoticePresented: Binding<Bool> {
        Binding(
            get: { viewModel.attachmentNotice != nil },
            set: { if !$0 { viewModel.attachmentNotice = nil } }
        )
    }

    private var isQuickLookPresented: Binding<Bool> {
        Binding(
            get: { viewModel.quickLookURL != nil },
            set: { if !$0 { viewModel.closeQuickLook() } }
        )
    }

    private var isHtmlPreviewPresented: Binding<Bool> {
        Binding(
            get: { viewModel.htmlPreviewURL != nil },
            set: { if !$0 { viewModel.closeHTMLPreview() } }
        )
    }

    private var inAppBrowserItem: Binding<IdentifiableURL?> {
        Binding<IdentifiableURL?>(
            get: { viewModel.inAppBrowserURL.map { IdentifiableURL(url: $0) } },
            set: { _ in viewModel.closeInAppBrowser() }
        )
    }

    private func handleViewAppear() {
        isViewAppeared = true
        if preserveScrollOnReturn {
            DispatchQueue.main.asyncAfter(deadline: .now() + 1.5) { preserveScrollOnReturn = false }
        }
        viewModel.restoreDraftsIfNeeded()
        consumeSharedFiles()
        checkPendingLiveRecording()
        if shouldAutoFocus && viewModel.pendingInteraction == nil {
            scheduleAutoFocus(delay: 0.45)
        }
        if !viewModel.messages.isEmpty {
            Task {
                await viewModel.resumeActiveSession()
            }
        }
    }

    private func handleViewDisappear() {
        isViewAppeared = false
        if hasInitiallyAligned && !isSplitDetail {
            // 压入子页面（如子代理会话）：保留滚动状态，返回时留在原位置
            preserveScrollOnReturn = true
        } else {
            hasInitiallyAligned = false
            scrollHUD.hasUserInteracted = false
            scrollHUD.isNearBottom = true
            scrollHUD.messagesContentHeight = 0
            scrollHUD.lastKnownVisibleMessageId = nil
            currentViewportHeight = 0
        }
        scrollHUD.isUserDragging = false
        scrollHUD.isScrollMoving = false
        scrollHUD.cancelScrollHealAndJumpTasks()
        activeTurnJumpTask?.cancel()
        activeTurnJumpTask = nil
        jumpTransitionDimmed = false
        jumpingToTurnIndex = nil
        scrollHUD.stepperDimTask?.cancel()
        scrollHUD.stepperDimTask = nil
        scrollHUD.stepperHideTask?.cancel()
        scrollHUD.stepperHideTask = nil
        autoFocusTask?.cancel()
        autoFocusTask = nil
        if voiceCall.isInCall {
            voiceCall.endCall()
        }
        viewModel.saveCurrentDraft()
        viewModel.disconnectStream()
    }
}

private struct ChatPermissionAlertsModifier: ViewModifier {
    @Binding var showCameraUnavailableAlert: Bool
    @Binding var showCameraPermissionAlert: Bool
    @Binding var showMicrophonePermissionAlert: Bool
    @Binding var showOtherSessionRecordingAlert: Bool
    let audioRecorder: AudioRecorderManager

    func body(content: Content) -> some View {
        content
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
            .alert("需要麦克风权限", isPresented: $showMicrophonePermissionAlert) {
                Button("前往设置") {
                    if let url = URL(string: UIApplication.openSettingsURLString) {
                        UIApplication.shared.open(url)
                    }
                }
                Button("取消", role: .cancel) {}
            } message: {
                Text("请在系统设置中允许 Multigravity 访问麦克风以录制语音。")
            }
            .alert("已有会话正在录音", isPresented: $showOtherSessionRecordingAlert) {
                Button("前往该会话") {
                    let targetId = audioRecorder.currentCascadeId
                    if !targetId.isEmpty, let url = URL(string: "antigravity://cascade/\(targetId)") {
                        UIApplication.shared.open(url)
                    }
                }
                Button("取消", role: .cancel) {}
            } message: {
                Text("当前已有另一个会话正在使用麦克风录音，请先结束该录音后再开启新录音。")
            }
    }
}


// MARK: - iPad split view support

private struct SplitDetailKey: EnvironmentKey {
    static let defaultValue = false
}

extension EnvironmentValues {
    /// True when the chat is shown in the detail column of the iPad split view.
    var isSplitDetail: Bool {
        get { self[SplitDetailKey.self] }
        set { self[SplitDetailKey.self] = newValue }
    }
}

/// Maximum width of chat content on wide screens (keeps lines readable, like Messages / Notes on iPad).
let chatReadableMaxWidth: CGFloat = 820

extension View {
    /// Caps the content at the readable chat width and centers it (no-op on compact widths).
    @ViewBuilder
    func readableChatWidth(_ enabled: Bool) -> some View {
        if enabled {
            self
                .frame(maxWidth: chatReadableMaxWidth)
                .frame(maxWidth: .infinity)
        } else {
            self
        }
    }
}

struct ContextRingCapsuleView: View {
    let meter: ContextMeterItem
    let isRunning: Bool
    let onTap: () -> Void
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @State private var isPulsing = false

    private var clampedPct: Double {
        min(max(meter.percentage, 0), 100)
    }

    private var shouldPulse: Bool {
        !reduceMotion && isRunning && (meter.isOverflow == true || clampedPct >= 90)
    }

    private var ringColor: Color {
        if meter.isOverflow == true || clampedPct >= 90 { return .red }
        if clampedPct >= 70 { return .orange }
        return .green
    }

    private func syncPulseAnimation() {
        if shouldPulse {
            withAnimation(.easeInOut(duration: 0.7).repeatForever(autoreverses: true)) {
                isPulsing = true
            }
        } else {
            withAnimation(.default) {
                isPulsing = false
            }
        }
    }

    var body: some View {
        Button(action: onTap) {
            HStack(spacing: 4) {
                ZStack {
                    Circle()
                        .stroke(ringColor.opacity(0.22), lineWidth: 2)
                    Circle()
                        .trim(from: 0, to: clampedPct / 100.0)
                        .stroke(ringColor, style: StrokeStyle(lineWidth: 2, lineCap: .round))
                        .rotationEffect(.degrees(-90))
                }
                .frame(width: 12, height: 12)

                Text("\(Int(clampedPct))%")
                    .font(.system(size: 10, weight: .bold, design: .monospaced))
                    .foregroundColor(ringColor)

                if let tps = meter.tokensPerSecond, tps > 0.1,
                   (isRunning || (meter.streamingDurationSec ?? 0) > 0) {
                    HStack(spacing: 1) {
                        Image(systemName: "bolt.fill")
                            .font(.system(size: 8.5))
                            .foregroundColor(.orange)
                        Text(String(format: "%.0f t/s", tps))
                            .font(.system(size: 10, weight: .semibold, design: .monospaced))
                            .foregroundColor(.secondary)
                    }
                }
            }
            .padding(.horizontal, 6)
            .padding(.vertical, 1.5)
            .background(ringColor.opacity(0.12))
            .overlay(
                Capsule().stroke(ringColor.opacity(0.35), lineWidth: 0.6)
            )
            .clipShape(Capsule())
            .opacity(shouldPulse && isPulsing ? 0.55 : 1.0)
            .onAppear { syncPulseAnimation() }
            .onChange(of: shouldPulse) { _, _ in syncPulseAnimation() }
        }
        .buttonStyle(.plain)
        .accessibilityLabel(String(localized: "上下文占用 \(Int(clampedPct))%，点击查看详情与压缩"))
    }
}

/// 客户端风格的上下收缩双箭头图标（一键压缩）
public struct CompactChevronsIcon: View {
    var size: CGFloat = 8
    var weight: Font.Weight = .bold

    public init(size: CGFloat = 8, weight: Font.Weight = .bold) {
        self.size = size
        self.weight = weight
    }

    public var body: some View {
        VStack(spacing: 1.5) {
            Image(systemName: "chevron.down")
                .font(.system(size: size, weight: weight))
            Image(systemName: "chevron.up")
                .font(.system(size: size, weight: weight))
        }
    }
}

struct ContextAuditSheetView: View {
    @Environment(\.dismiss) private var dismiss
    let meter: ContextMeterItem
    let canCompact: Bool
    let onCompact: () -> Void
    let onStartNewSession: () -> Void

    private var clampedPct: Double {
        min(max(meter.percentage, 0), 100)
    }

    private var accentColor: Color {
        if meter.isOverflow == true || clampedPct >= 90 { return .red }
        if clampedPct >= 70 { return .orange }
        return .green
    }

    var body: some View {
        VStack(spacing: 16) {
            HStack {
                VStack(alignment: .leading, spacing: 2) {
                    Text("实时上下文")
                        .font(.system(size: 17, weight: .bold))
                    Text("\(AccountQuotaSheet.formatTokensCompact(meter.usedTokens)) / \(AccountQuotaSheet.formatTokensCompact(meter.maxTokens)) Tokens")
                        .font(.system(size: 12, design: .monospaced))
                        .foregroundColor(.secondary)
                }
                Spacer()
                Text(String(format: "%.1f%%", clampedPct))
                    .font(.system(size: 14, weight: .bold, design: .monospaced))
                    .foregroundColor(accentColor)
                    .padding(.horizontal, 10)
                    .padding(.vertical, 4)
                    .background(accentColor.opacity(0.14))
                    .clipShape(Capsule())
            }

            ProgressView(value: clampedPct, total: 100)
                .tint(accentColor)

            HStack(spacing: 8) {
                auditTile(title: String(localized: "输出速度"), value: (meter.tokensPerSecond ?? 0) > 0 ? String(format: "%.1f t/s", meter.tokensPerSecond ?? 0) : "--")
                auditTile(title: String(localized: "首字延迟 (TTFT)"), value: (meter.timeToFirstTokenSec ?? 0) > 0 ? String(format: "%.2fs", meter.timeToFirstTokenSec ?? 0) : "--")
                auditTile(title: String(localized: "流式耗时"), value: (meter.streamingDurationSec ?? 0) > 0 ? String(format: "%.1fs", meter.streamingDurationSec ?? 0) : "--")
            }

            VStack(alignment: .leading, spacing: 8) {
                Text("最近一步 Token 构成")
                    .font(.system(size: 12, weight: .semibold))
                    .foregroundColor(.secondary)
                HStack {
                    auditCell("Input", AccountQuotaSheet.formatTokensCompact(meter.inputTokens ?? 0))
                    Spacer()
                    auditCell("Cache Hit", AccountQuotaSheet.formatTokensCompact(meter.cacheReadTokens ?? 0))
                    Spacer()
                    auditCell("Output", AccountQuotaSheet.formatTokensCompact(meter.outputTokens ?? 0))
                    Spacer()
                    auditCell("Thinking", AccountQuotaSheet.formatTokensCompact(meter.thinkingTokens ?? 0))
                }
            }
            .padding(14)
            .background(Color(uiColor: .secondarySystemGroupedBackground))
            .clipShape(RoundedRectangle(cornerRadius: 12))

            HStack(spacing: 10) {
                if canCompact {
                    Button(action: {
                        dismiss()
                        onCompact()
                    }) {
                        HStack(spacing: 6) {
                            CompactChevronsIcon(size: 8, weight: .bold)
                            Text("一键压缩")
                        }
                        .font(.system(size: 13, weight: .semibold))
                        .frame(maxWidth: .infinity)
                        .padding(.vertical, 8)
                    }
                    .buttonStyle(.borderedProminent)
                    .tint(.orange)
                }
                Button(action: {
                    dismiss()
                    onStartNewSession()
                }) {
                    Label("开启新会话", systemImage: "plus.circle")
                        .font(.system(size: 13, weight: .semibold))
                        .frame(maxWidth: .infinity)
                        .padding(.vertical, 8)
                }
                .buttonStyle(.bordered)
            }
        }
        .padding(.horizontal, 20)
        .padding(.top, 52)
        .padding(.bottom, 28)
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
        .background(Color(uiColor: .systemGroupedBackground))
    }

    private func auditTile(title: String, value: String) -> some View {
        VStack(alignment: .leading, spacing: 3) {
            Text(title).font(.system(size: 10)).foregroundColor(.secondary).lineLimit(1)
            Text(value).font(.system(size: 14, weight: .bold, design: .monospaced)).lineLimit(1)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(.horizontal, 10)
        .padding(.vertical, 8)
        .background(Color(uiColor: .secondarySystemGroupedBackground))
        .clipShape(RoundedRectangle(cornerRadius: 10))
    }

    private func auditCell(_ label: String, _ value: String) -> some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(label).font(.system(size: 10)).foregroundColor(.secondary)
            Text(value).font(.system(size: 13, weight: .semibold, design: .monospaced))
        }
    }
}

/// 独立观察 `scrollHUD.currentVisibleTurnIndex` 的顶部导航栏标题组件，
/// 避免滚动切换可见轮次时触发 `ChatView.body` 与整条消息列表重新求值。
private struct ChatToolbarPrincipalView: View {
    let viewModel: ChatViewModel
    let scrollHUD: ChatScrollHUDState
    let onOpenNavigator: () -> Void
    let onOpenContextAudit: () -> Void
    @State private var settings = AppSettings.shared

    var body: some View {
        if viewModel.turns.count >= 2 {
            VStack(spacing: 2) {
                Button(action: onOpenNavigator) {
                    Text(viewModel.currentTitle)
                        .font(.system(size: 14.5, weight: .semibold))
                        .foregroundColor(.primary)
                        .lineLimit(1)
                }
                .buttonStyle(.plain)

                HStack(spacing: 10) {
                    Button(action: onOpenNavigator) {
                        HStack(spacing: 3) {
                            Text("第 \(scrollHUD.currentVisibleTurnIndex)/\(viewModel.turns.count) 轮")
                                .font(.system(size: 10, weight: .semibold))
                                .foregroundColor(.indigo)
                            Image(systemName: "chevron.down")
                                .font(.system(size: 7.5, weight: .bold))
                                .foregroundColor(.indigo.opacity(0.8))
                        }
                        .padding(.horizontal, 6)
                        .padding(.vertical, 1.5)
                        .background(Color.indigo.opacity(0.09))
                        .clipShape(Capsule())
                    }
                    .buttonStyle(.plain)
                    .accessibilityLabel("当前第 \(scrollHUD.currentVisibleTurnIndex) 轮，共 \(viewModel.turns.count) 轮，点击打开步骤大纲")

                    if let meter = viewModel.contextMeter, settings.shouldShowContextMeter(meter) {
                        ContextRingCapsuleView(
                            meter: meter,
                            isRunning: viewModel.isRunning || viewModel.isAwaitingResponse
                        ) {
                            onOpenContextAudit()
                        }
                    }
                }
            }
        } else {
            VStack(spacing: 2) {
                Text(viewModel.currentTitle)
                    .font(.system(size: 15.5, weight: .semibold))
                    .lineLimit(1)
                if let meter = viewModel.contextMeter, settings.shouldShowContextMeter(meter) {
                    ContextRingCapsuleView(
                        meter: meter,
                        isRunning: viewModel.isRunning || viewModel.isAwaitingResponse
                    ) {
                        onOpenContextAudit()
                    }
                }
            }
        }
    }
}
