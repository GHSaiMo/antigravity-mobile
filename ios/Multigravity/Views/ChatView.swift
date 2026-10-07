import SwiftUI
import PhotosUI
import AVFoundation

public struct ChatView: View {
    @Environment(\.dismiss) var dismiss
    @Environment(\.scenePhase) var scenePhase
    @State var viewModel: ChatViewModel
    @Environment(\.isSplitDetail) var isSplitDetail
    @Environment(\.horizontalSizeClass) var horizontalSizeClass
    @FocusState var isInputFocused: Bool
    @State var hasInitiallyAligned = false
    @State var hasUserInteracted = false
    @State var isNearBottom = true
    @State var messagesContentHeight: CGFloat = 0
    @State var currentViewportHeight: CGFloat = 0
    @State var cardToggleTrigger = 0
    @State var showCameraPicker = false
    @State var showCameraUnavailableAlert = false
    @State var showCameraPermissionAlert = false
    @State var previewDraftGallery: ImageGalleryData? = nil
    @State var showAttachmentSheet = false
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
                _viewModel = State(initialValue: ChatViewModel(
                    cascadeId: conversation.id,
                    initialTitle: conversation.title,
                    workspaceName: conversation.workspaceName,
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
                workspaceName: conversation.workspaceName,
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
                .readableChatWidth(horizontalSizeClass == .regular)
            inputBar
        }
        .navigationTitle(viewModel.currentTitle)
        .navigationBarTitleDisplayMode(.inline)
        .navigationBarBackButtonHidden(true)
        .toolbar {
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
        }
        .background(SwipeBackEnabler())
        .onChange(of: ShareInbox.shared.deliveryTick) { _, _ in
            consumeSharedFiles()
        }
        .onAppear {
            isViewAppeared = true
            viewModel.restoreDraftsIfNeeded()
            consumeSharedFiles()
            if shouldAutoFocus && viewModel.pendingInteraction == nil {
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
        .onChange(of: viewModel.pendingInteraction != nil) { _, hasPending in
            if hasPending {
                isInputFocused = false
                UIApplication.shared.sendAction(#selector(UIResponder.resignFirstResponder), to: nil, from: nil, for: nil)
            }
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
            messagesContentHeight = 0
            currentViewportHeight = 0
            autoFocusTask?.cancel()
            autoFocusTask = nil
            viewModel.saveCurrentDraft()
            viewModel.disconnectStream()
        }
        .environment(\.openURL, OpenURLAction { url in
            handleURLTap(url)
        })
        .sheet(isPresented: $showAttachmentSheet) {
            AttachmentPickerSheet(
                remainingImageSlots: max(0, 5 - viewModel.selectedImageData.count),
                onPhotosPicked: { assets in handlePickedAssets(assets) },
                onOpenCamera: {
                    // Wait for the sheet to finish dismissing before presenting the camera.
                    DispatchQueue.main.asyncAfter(deadline: .now() + 0.4) { handleCameraAction() }
                },
                onFilesPicked: { urls in viewModel.addFiles(from: urls) }
            )
        }
        .alert("无法添加文件", isPresented: Binding(
            get: { viewModel.attachmentNotice != nil },
            set: { if !$0 { viewModel.attachmentNotice = nil } }
        )) {
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
