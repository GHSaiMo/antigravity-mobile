import Foundation
import Observation
import UIKit
import SwiftUI

public struct MarkdownFileViewerData: Identifiable, Sendable, Equatable {
    public let id: String
    public var title: String
    public let uri: String
    public var content: String
    public var summary: String?
    public var isLoading: Bool
    public var isRefreshing: Bool
    public var errorMessage: String?
    public var canProceed: Bool
    public var cachedFileURL: URL?
    public var isCached: Bool
    
    public init(
        id: String,
        title: String,
        uri: String,
        content: String = "",
        summary: String? = nil,
        isLoading: Bool = false,
        isRefreshing: Bool = false,
        errorMessage: String? = nil,
        canProceed: Bool = false,
        cachedFileURL: URL? = nil,
        isCached: Bool = false
    ) {
        self.id = id
        self.title = title
        self.uri = uri
        self.content = content
        self.summary = summary
        self.isLoading = isLoading
        self.isRefreshing = isRefreshing
        self.errorMessage = errorMessage
        self.canProceed = canProceed
        self.cachedFileURL = cachedFileURL
        self.isCached = isCached
    }
}

public struct DocumentActionItem: Identifiable, Sendable, Equatable {
    public let id: String
    public let uri: String
    public let fileName: String
    public let isPresentation: Bool
    public let isSpreadsheet: Bool
    
    public init(uri: String, fileName: String) {
        self.id = uri
        self.uri = uri
        self.fileName = fileName
        let lower = fileName.lowercased()
        self.isPresentation = lower.hasSuffix(".pptx") || lower.hasSuffix(".ppt") || lower.hasSuffix(".key")
        self.isSpreadsheet = lower.hasSuffix(".xlsx") || lower.hasSuffix(".xls") || lower.hasSuffix(".numbers") || lower.hasSuffix(".csv")
    }
}

@Observable

@MainActor
public final class ChatViewModel {
    public static var activeCascadeId: String = ""

    public var cascadeId: String {
        didSet {
            if !cascadeId.isEmpty {
                Self.activeCascadeId = cascadeId
            }
        }
    }
    public let initialTitle: String
    public var currentTitle: String
    public var workspaceName: String = "Chat"
    public var workspaceUri: String? = nil
    public var isNewConversation: Bool
    /// 仅当尚未在服务端创建真实 cascadeId（即本地未发送草稿）时为 true；服务端已有但 0 步的空会话仍需对齐桌面端模型。
    public var isDraftConversation: Bool {
        cascadeId.isEmpty || cascadeId.hasPrefix("local_draft_") || cascadeId.hasPrefix("draft_")
    }
    public var draftProject: ProjectItem?
    public var draftSession: LocalDraftSession?
    public var originDraftId: String? = nil
    var isInitializing: Bool = true
    
    public var messages: [ChatMessage] = [] {
        didSet { messagesRevision &+= 1 }
    }
    public var turnOutline: [TurnOutlineItem] = [] {
        didSet { turnOutlineRevision &+= 1 }
    }
    
    /// messages / turnOutline / chapterTitles 每次赋值（含整体替换与原地修改）都会递增的版本号，
    /// 用作 turns / chapters 缓存键：O(1) 比较，且不会漏掉列表中间消息或同名章节标题的变化。
    @ObservationIgnored private var messagesRevision: UInt64 = 0
    @ObservationIgnored private var turnOutlineRevision: UInt64 = 0
    @ObservationIgnored private var chapterTitlesRevision: UInt64 = 0
    @ObservationIgnored private var _cachedTurnsKey: String = ""
    @ObservationIgnored private var _cachedTurns: [ConversationTurn] = []
    @ObservationIgnored private var _cachedChaptersKey: String = ""
    @ObservationIgnored private var _cachedChapters: [ConversationChapter] = []
    @ObservationIgnored private var draftSaveTask: Task<Void, Never>? = nil
    @ObservationIgnored private var draftRestoreTask: Task<Void, Never>? = nil
    @ObservationIgnored private var draftMutationGeneration: UInt64 = 0
    
    /// 当前会话按问答轮次自动聚合的结构列表（融合服务端全量目录索引与本地已加载消息）
    public var turns: [ConversationTurn] {
        // 读取 messages 与 turnOutline 以保留 Observation 依赖追踪，版本号本身不被观察
        let currentMessages = messages
        let currentOutline = turnOutline
        let key = "\(messagesRevision)|\(turnOutlineRevision)|\(isRunning)|\(isAwaitingResponse)"
        if key == _cachedTurnsKey {
            return _cachedTurns
        }
        let built = ConversationTurnBuilder.buildTurns(
            from: currentMessages,
            outline: currentOutline.isEmpty ? nil : currentOutline,
            isRunning: isRunning,
            isAwaitingResponse: isAwaitingResponse
        )
        _cachedTurnsKey = key
        _cachedTurns = built
        return built
    }
    
    /// 超长会话自动拆分的章节列表（合并 Antigravity 2.30.0 GetChapterTitles 语义标题）
    public var chapters: [ConversationChapter] {
        let currentTurns = turns
        let titles = chapterTitles
        let key = "\(_cachedTurnsKey)|\(chapterTitlesRevision)"
        if key == _cachedChaptersKey {
            return _cachedChapters
        }
        let built = ConversationTurnBuilder.buildChapters(from: currentTurns, aiTitles: titles)
        _cachedChaptersKey = key
        _cachedChapters = built
        return built
    }
    
    public var selectedImageData: [Data] = []
    /// Non-image attachments (documents, archives, source files) of the current draft.
    public var selectedFiles: [DraftFile] = []
    /// One-shot message about rejected / failed attachments, shown as an alert by the view.
    public var attachmentNotice: String?
    @ObservationIgnored var fileUploadTasks: [String: Task<Void, Never>] = [:]
    /// One-shot flag to select the first `[placeholder]` after inserting a workflow template.
    public var pendingSelectFirstPlaceholder: Bool = false
    public var inputText: String = "" {
        didSet {
            guard !isInitializing else { return }
            markDraftMutated()
            scheduleDebouncedDraftSave()
        }
    }
    
    public var draftKey: String {
        if !cascadeId.isEmpty {
            return cascadeId
        } else if let draftSession {
            return draftSession.id
        } else if let draftProject {
            return "draft_project_\(draftProject.id)"
        } else if let originDraftId {
            return originDraftId
        }
        return ""
    }
    
    public var emptyConversationTitle: String {
        if isPureChat {
            return String(localized: "新对话")
        }
        if !workspaceName.isEmpty && workspaceName != "Chat" {
            return workspaceName
        }
        if let draftProject, !draftProject.isPureChat {
            return draftProject.name
        }
        if let draftSession, !draftSession.project.isPureChat {
            return draftSession.project.name
        }
        return String(localized: "新对话")
    }
    
    public var isPureChat: Bool {
        if let draftProject {
            return draftProject.isPureChat
        }
        if let draftSession {
            return draftSession.project.isPureChat
        }
        if !workspaceName.isEmpty && workspaceName != "Chat" {
            return false
        }
        return true
    }
    
    @MainActor
    func markDraftMutated() {
        draftMutationGeneration &+= 1
        draftRestoreTask?.cancel()
        draftRestoreTask = nil
    }

    public func updateDraftImages(_ images: [Data]) {
        markDraftMutated()
        self.selectedImageData = images
        if var updated = draftSession {
            updated.draftImages = images
            updated.updatedAt = Date()
            self.draftSession = updated
            cacheManager.saveLocalDraftSession(updated)
        }
        saveCurrentDraft()
    }
    
    public func appendDraftImages(_ newImages: [Data]) {
        guard !newImages.isEmpty else { return }
        markDraftMutated()
        var current = self.selectedImageData
        current.append(contentsOf: newImages)
        if current.count > 5 {
            current = Array(current.prefix(5))
        }
        self.selectedImageData = current
        if var updated = draftSession {
            updated.draftImages = current
            updated.updatedAt = Date()
            self.draftSession = updated
            cacheManager.saveLocalDraftSession(updated)
        }
        saveCurrentDraft()
    }
    
    public func removeDraftImage(at index: Int) {
        guard index < selectedImageData.count else { return }
        markDraftMutated()
        selectedImageData.remove(at: index)
        if var updated = draftSession {
            updated.draftImages = selectedImageData
            updated.updatedAt = Date()
            self.draftSession = updated
            cacheManager.saveLocalDraftSession(updated)
        }
        saveCurrentDraft()
    }
    
    private func scheduleDebouncedDraftSave() {
        draftSaveTask?.cancel()
        if inputText.isEmpty {
            saveCurrentDraft(includeImages: false)
            return
        }
        draftSaveTask = Task { @MainActor [weak self] in
            try? await Task.sleep(nanoseconds: 350_000_000)
            guard !Task.isCancelled else { return }
            self?.saveCurrentDraft(includeImages: false)
        }
    }

    public func saveCurrentDraft(includeImages: Bool = true) {
        draftSaveTask?.cancel()
        draftSaveTask = nil
        guard !isInitializing else { return }
        let key = draftKey
        guard !key.isEmpty else { return }
        if includeImages {
            cacheManager.saveDraftImages(key: key, images: selectedImageData)
        }
        cacheManager.saveDraft(key: key, text: inputText)
        cacheManager.recordDraftDate(key: key)
        if !cascadeId.isEmpty && (messages.isEmpty || isNewConversation) {
            cacheManager.recordConversationTouch(cascadeId: cascadeId)
        }
        if let draftSession, key == draftSession.id {
            var updated = draftSession
            updated.draftText = inputText
            updated.draftImages = selectedImageData
            updated.updatedAt = Date()
            self.draftSession = updated
            cacheManager.saveLocalDraftSession(updated)
        }
    }
    
    @MainActor
    public func restoreDraftsIfNeeded() {
        guard !isSending else { return }
        let key = draftKey
        guard !key.isEmpty else { return }
        let currentDraftSession = self.draftSession
        let capturedGen = self.draftMutationGeneration
        draftRestoreTask?.cancel()
        draftRestoreTask = Task { [weak self, key, currentDraftSession, capturedGen] in
            let restored = await Task.detached(priority: .userInitiated) { () -> (images: [Data], draft: String, files: [DraftFile]) in
                var cachedImages = await CacheManager.shared.getDraftImagesAsync(for: key)
                if cachedImages.isEmpty, let dSession = currentDraftSession, !dSession.draftImages.isEmpty {
                    cachedImages = dSession.draftImages
                    CacheManager.shared.saveDraftImages(key: key, images: cachedImages)
                }
                let cachedDraft = CacheManager.shared.getDraft(for: key)
                let resolvedDraft = !cachedDraft.isEmpty ? cachedDraft : (currentDraftSession?.draftText ?? "")
                let savedFiles = DraftFileStore.shared.load(key: key)
                return (cachedImages, resolvedDraft, savedFiles)
            }.value
            guard !Task.isCancelled, let self else { return }
            guard !self.isSending, self.draftKey == key, self.draftMutationGeneration == capturedGen else { return }
            if !restored.images.isEmpty && self.selectedImageData.isEmpty {
                self.selectedImageData = restored.images
            }
            if !restored.files.isEmpty {
                self.applyRestoredDraftFiles(restored.files)
            }
            if !restored.draft.isEmpty && self.inputText.isEmpty {
                self.inputText = restored.draft
            }
        }
    }
    
    public var isLoading: Bool = false
    public var isRunning: Bool = false
    public var stepCount: Int = 0
    public var totalTools: Int = 0
    public var duration: String = String(localized: "0秒")
    public var hasMore: Bool = false
    public var nextOffset: Int = 0
    public var isLoadingOlder: Bool = false
    public var errorMessage: String? = nil
    public var hasError: Bool = false
    public var trajectoryErrorMessage: String? = nil
    public var cascadeConfigRaw: String? = nil
    public var contextMeter: ContextMeterItem? = nil
    public var isAwaitingResponse: Bool = false
    public var scrollToTurnStartTrigger: Int = 0
    public var scrollToBottomTrigger: Int = 0
    
    public func triggerScrollToBottom() {
        scrollToBottomTrigger &+= 1
    }
    
    public var canProceed: Bool = false
    public var proceedArtifactUri: String? = nil
    public var pendingInteraction: PendingInteraction? = nil
    public var isSubmittingInteraction: Bool = false
    public var queuedMessages: [QueuedMessageItem] = []
    public var runningTasks: [RunningTaskItem] = []
    public var globalBackgroundTasks: [GlobalBackgroundTaskItem] = []
    public var hasAnyBackgroundTasks: Bool { !runningTasks.isEmpty || !globalBackgroundTasks.isEmpty }
    /// 本会话派发的子代理（来自 stream，不入缓存）。
    public var subagents: [SubagentItem] = []
    /// 本会话关联的 Swarm 蜂群状态（来自 stream）。
    public var swarmBrief: SwarmBrief? = nil
    public var isSwarmActive: Bool { swarmBrief?.active == true }
    /// 非空说明当前会话本身就是子代理，界面只读。
    public var parentConversationId: String? = nil
    public var subagentRole: String? = nil
    public var isSubagentSession: Bool { !(parentConversationId ?? "").isEmpty }
    public var isSending: Bool = false
    public var viewingMarkdownFile: MarkdownFileViewerData? = nil
    public var isDownloadingDocument: Bool = false
    public var downloadingDocumentName: String = ""
    public var downloadProgress: Double = 0.0
    public var downloadBytesWritten: Int64 = 0
    public var downloadBytesTotal: Int64 = 0
    var documentDownloadTask: Task<Void, Never>? = nil
    public var quickLookURL: URL? = nil
    public var quickLookTitle: String = ""
    public var htmlPreviewURL: URL? = nil
    public var htmlPreviewTitle: String = ""
    public var weChatArchiveItem: WeChatArchiveSessionItem? = nil
    /// 应用内沉浸式安全浏览器 URL (SFSafariViewController)
    public var inAppBrowserURL: URL? = nil

    // MARK: - Phase 3 State
    public var suggestedResponses: [String] = []
    public var activeFocus: String? = nil
    public var activeFocusStartedAt: String? = nil
    public var activePlan: StepPlanItem? = nil
    public var selectedContextItems: [ContextMentionItem] = []
    public var mentionCandidates: [ContextMentionItem] = []
    public var isLoadingMentions: Bool = false
    public var showMentionPicker: Bool = false
    public var mentionQuery: String = ""
    public var permissionState: PermissionStateResponse = PermissionStateResponse()
    public var trustStatus: ProjectTrustStatusResponse? = nil
    public var isUpdatingTrust: Bool = false
    public var btwEntries: [BtwSideQuestionEntry] = []
    public var branchOrigin: SideQuestionOrigin? = nil
    public var sideQuestions: [SideQuestionCardItem] = []
    public var isStartingBtw: Bool = false
    public var isSideQuestionSession: Bool { branchOrigin?.isSideQuestion == true }
    public var activeTerminals: [TerminalItem] = []
    public var isForkingConversation: Bool = false
    @ObservationIgnored var mentionSearchTask: Task<Void, Never>? = nil
    
    @MainActor
    public func openInAppBrowser(url: URL) {
        self.inAppBrowserURL = url
    }
    
    @MainActor
    public func closeInAppBrowser() {
        self.inAppBrowserURL = nil
    }
    
    // Revert / Undo State
    public var revertPreview: RevertPreviewResponse? = nil
    public var activeUndoMessage: ChatMessage? = nil
    public var showConfirmUndoSheet: Bool = false
    public var isReverting: Bool = false
    public var isLoadingRevertPreview: Bool = false
    public var undoErrorMessage: String? = nil
    @ObservationIgnored var undoPreviewTask: Task<Void, Never>? = nil
    public var focusInputTrigger: Int = 0

    // 本会话改动 / Git 提交 / Markdown 导出
    /// 斜杠命令菜单（首次键入 "/" 时懒加载）与输入框里已选中的命令标签。
    public var slashCommands: [SlashCommandOption] = []
    public var isLoadingSlashCommands: Bool = false
    public var selectedSlashCommand: SlashCommandOption? = nil

    public var changesSheet: ChangesSheetState? = nil
    public var gitSheet: GitSheetState? = nil
    /// Commit / Changes 快捷按钮的动态可见性：会话没有 Git 工作区、或没有文件变动及 PR 时不显示。
    public var hasGitChanges: Bool = false
    public var hasUncommittedFiles: Bool = false
    public var gitChipLabel: String = "Commit"
    public var hasSessionChanges: Bool = false
    public var exportedMarkdownFile: ExportedMarkdownFile? = nil
    /// Antigravity 2.30.0 GetChapterTitles 生成的章节标题缓存（key: "start:end" 或 "start:s"）
    public var chapterTitles: [String: String] = [:] {
        didSet { chapterTitlesRevision &+= 1 }
    }
    public var isLoadingChapterTitles: Bool = false
    
    /// ID of the first message of the latest response turn (e.g., tool batch or agent response following the last user message)
    public var latestTurnStartMessageId: String? {
        guard let lastUserIdx = messages.lastIndex(where: { $0.sender == .user }) else {
            return messages.first(where: { $0.sender != .user })?.id ?? messages.first?.id
        }
        let subsequent = messages.suffix(from: lastUserIdx + 1)
        return subsequent.first?.id
    }
    
    public var isUnreadOnEntry: Bool = false
    public var initialConversationStatus: ConversationItem.ConversationStatus? = nil
    
    /// Whether this conversation has an error (trajectory error, error status, or latest turn error)
    public var hasErrorState: Bool {
        hasError || (initialConversationStatus?.isError == true) || (messages.last?.isError == true)
    }
    
    /// Whether the latest message from the Agent in this conversation is an unrecovered error message
    public var isLatestMessageError: Bool {
        // Hide Continue button while actively running, sending, or awaiting response
        guard !isActivelyRunning, !isSending else { return false }
        
        // 1. Direct check: if the very last message in the chat is an error
        if let last = messages.last {
            return last.isError
        }
        
        // 2. Turn check: in the latest turn (after the last user message),
        // check if the latest event was an unrecovered error
        if let lastUserIdx = messages.lastIndex(where: { $0.isUser }) {
            let subsequent = messages.suffix(from: lastUserIdx + 1)
            if let lastTurnMsg = subsequent.last {
                return lastTurnMsg.isError
            }
        } else if let lastMsg = messages.last {
            return lastMsg.isError
        }
        
        return false
    }
    
    /// Whether this conversation has an action requiring user input/interaction
    public var hasActionState: Bool {
        canProceed || pendingInteraction != nil || (initialConversationStatus?.needsAction == true)
    }
    
    /// Whether the conversation is currently actively running, executing tasks, or awaiting responses
    public var isActivelyRunning: Bool {
        isRunning || isAwaitingResponse || runningTasks.hasBlockingTasks || (initialConversationStatus?.isRunning == true)
    }
    
    /// Whether this conversation on entry has unread messages, error messages, or action messages
    public var shouldScrollToTurnStartOnEntry: Bool {
        guard !isActivelyRunning else { return false }
        if messages.last?.sender == .user { return false }
        if latestAgentMessageId == nil { return false }
        return isUnreadOnEntry || hasErrorState || hasActionState
    }
    
    /// ID of the latest agent response message (the actual text bubble from the agent, not tool batches).
    /// If there are user messages in the chat, this strictly checks for an agent message AFTER the latest user message.
    /// It NEVER falls back to an agent message from an earlier turn before the latest user message.
    public var latestAgentMessageId: String? {
        if let lastUserIdx = messages.lastIndex(where: { $0.sender == .user }) {
            let subsequent = messages.suffix(from: lastUserIdx + 1)
            if let agentMsg = subsequent.first(where: { $0.sender == .agent }) {
                return agentMsg.id
            }
            return nil
        }
        return messages.first(where: { $0.sender == .agent })?.id
    }
    
    public var activeModel: String {
        didSet {
            settings.activeModel = activeModel
        }
    }
    
    /// 网关给出的模型完整展示名（如 "Gemini 3.8 Flash (High)"）；为空时界面回退到由 id 推断的名称。
    public var activeModelName: String? = nil
    /// 会话发起时间（ISO-8601），用于长图分享等场景。
    public var startedAt: String? = nil

    public var isClaudeActive: Bool {
        ModelDefaultsLogic.isClaude(activeModel)
    }
    
    /// 发给需要底层枚举的场景（网关 API 优先直接传 `activeModel` 规范/合成 ID）：
    /// 优先用模型目录里的枚举，合成 Orchestrator 回溯到对应 planner 枚举，最后再回退到内置枚举。
    public var activeModelEnum: String {
        if let known = ModelCatalogStore.shared.modelEnum(forID: activeModel) { return known }
        if let plannerID = ModelDefaultsLogic.resolvedPlannerModelID(forOrchestratorID: activeModel),
           let knownPlanner = ModelCatalogStore.shared.modelEnum(forID: plannerID) {
            return knownPlanner
        }
        if let staticPlanner = ModelDefaultsLogic.resolvedPlannerEnum(forID: activeModel) {
            return staticPlanner
        }
        switch activeModelProviderKind {
        case .claude: return "MODEL_PLACEHOLDER_M26"
        case .orchestrator: return "MODEL_PLACEHOLDER_M537"
        case .gemini, .other: return "MODEL_PLACEHOLDER_M532"
        }
    }
    
    /// 输入框上方胶囊的文字：紧凑短名展示（如 Carbon · High、Opus 5.5 · High 等）。
    public var activeModelDisplayName: String {
        ModelCatalogStore.shared.compactDisplayName(forID: activeModel)
    }

    public var activeModelProviderKind: ModelProviderKind {
        ModelDefaultsLogic.providerKind(forID: activeModel)
    }
    
    public func syncModel(from raw: String?) {
        guard !hasUserManuallySelectedModel else { return }
        guard let raw = raw, !raw.isEmpty else { return }
        let target = ModelDefaultsLogic.resolveActive(
            raw: raw,
            current: activeModel,
            geminiDefault: settings.favoriteModel1,
            claudeDefault: settings.favoriteModel2
        )
        if activeModel != target {
            activeModel = target
        }
    }
    
    var awaitingResponseSince: Date? = nil
    var pendingOptimisticMessageId: String? = nil
    struct PendingOptimisticQueueItem {
        let id: String
        let text: String
        let media: [String]?
        let imageUrls: [String]?
        let createdAt: Date
        let enqueuedAfterMessageId: String?
        /// 随消息发给网关的客户端 id，用来在服务端队列里精确认出这一条。
        var clientMessageId: String? = nil
    }
    var pendingOptimisticQueueItems: [PendingOptimisticQueueItem] = []
    
    struct QueuedMessageTombstone: Equatable, Sendable {
        let id: String?
        let text: String
        let deletedAt: Date
    }
    var deletedQueueItemTombstones: [QueuedMessageTombstone] = []
    var inFlightDeletingQueueIds: Set<String> = []
    var knownServerMessageIds: Set<String> = []
    var pollTask: Task<Void, Never>?
    var hasUserManuallySelectedModel: Bool = false
    var hasResolvedSessionModel: Bool = false
    public private(set) var streamClient: StreamWebSocketClient
    let apiClient: APIClient
    let settings: AppSettings
    let activityManager: ActivityManager
    let cacheManager: CacheManager
    var lastResumeTime: Date = .distantPast
    @ObservationIgnored var backgroundTaskId: UIBackgroundTaskIdentifier = .invalid
    @ObservationIgnored var backgroundDisconnectTask: Task<Void, Never>? = nil
    @ObservationIgnored var decodedStreamMediaCache: [String: (rawMedia: [String], decoded: [Data])] = [:]
    @ObservationIgnored var lastUserCancelledAt: Date = .distantPast
    @ObservationIgnored var currentTurnHadRemoteApproval: Bool = false
    
    public init(
        cascadeId: String,
        initialTitle: String,
        workspaceName: String = "Chat",
        isNewConversation: Bool = false,
        apiClient: APIClient? = nil,
        settings: AppSettings? = nil,
        cacheManager: CacheManager? = nil,
        isUnread: Bool = false,
        conversationStatus: ConversationItem.ConversationStatus? = nil,
        initialBranchOrigin: SideQuestionOrigin? = nil
    ) {
        self.cascadeId = cascadeId
        self.initialTitle = initialTitle
        self.currentTitle = initialTitle
        self.workspaceName = workspaceName
        self.isNewConversation = isNewConversation
        self.isUnreadOnEntry = isUnread
        self.initialConversationStatus = conversationStatus
        self.branchOrigin = initialBranchOrigin
        let resolvedApiClient = apiClient ?? .shared
        let resolvedSettings = settings ?? .shared
        let resolvedCacheManager = cacheManager ?? .shared
        self.apiClient = resolvedApiClient
        self.settings = resolvedSettings
        self.activityManager = ActivityManager.shared
        self.cacheManager = resolvedCacheManager
        self.streamClient = StreamWebSocketClient()
        
        let isDraftCascade = cascadeId.hasPrefix("local_draft_") || cascadeId.hasPrefix("draft_")
        if isDraftCascade {
            self.originDraftId = cascadeId
            if let session = resolvedCacheManager.getLocalDraftSession(id: cascadeId) {
                self.draftSession = session
                self.draftProject = session.project
            }
            self.cascadeId = ""
            self.isNewConversation = true
        }
        
        let memCachedSession: CachedChatSession? = (!cascadeId.isEmpty && !isDraftCascade)
            ? resolvedCacheManager.loadMemoryCachedSession(for: cascadeId)
            : nil

        var initialModel = resolvedSettings.favoriteModel1
        var initialModelName: String? = nil
        var resolvedFromSessionCache = false
        if !cascadeId.isEmpty && !isDraftCascade {
            if let cached = memCachedSession,
               let cfg = cached.cascadeConfigRaw {
                if let enumName = ModelDefaultsLogic.extractModelEnum(fromConfig: cfg),
                   let knownID = ModelCatalogStore.shared.id(forEnum: enumName) {
                    initialModel = knownID
                    resolvedFromSessionCache = true
                } else {
                    let lower = cfg.lowercased()
                    if ModelDefaultsLogic.isClaude(lower) {
                        initialModel = ModelDefaultsLogic.providerKind(forID: resolvedSettings.favoriteModel2) == .claude
                            ? resolvedSettings.favoriteModel2
                            : ModelDefaultsLogic.fallbackClaude
                        resolvedFromSessionCache = true
                    } else if lower.contains("m667") || lower.contains("m681") || lower.contains("orch") {
                        initialModel = ModelDefaultsLogic.providerKind(forID: resolvedSettings.favoriteModel2) == .orchestrator
                            ? resolvedSettings.favoriteModel2
                            : ModelDefaultsLogic.fallbackOrchestrator
                        resolvedFromSessionCache = true
                    } else if lower.contains("m318") || lower.contains("m417") || lower.contains("m418") || lower.contains("m419") || lower.contains("gemini") {
                        initialModel = ModelDefaultsLogic.providerKind(forID: resolvedSettings.favoriteModel1) == .gemini
                            ? resolvedSettings.favoriteModel1
                            : ModelDefaultsLogic.fallbackGemini
                        resolvedFromSessionCache = true
                    }
                }
            }
            if !resolvedFromSessionCache,
               let desktopModel = ModelCatalogStore.shared.catalog?.desktopActiveModel, !desktopModel.isEmpty {
                initialModel = ModelDefaultsLogic.resolveActive(
                    raw: desktopModel,
                    current: initialModel,
                    geminiDefault: resolvedSettings.favoriteModel1,
                    claudeDefault: resolvedSettings.favoriteModel2
                )
                if let dName = ModelCatalogStore.shared.catalog?.desktopActiveModelName, !dName.isEmpty {
                    initialModelName = dName
                }
            }
        }
        self.hasResolvedSessionModel = resolvedFromSessionCache
        self.activeModel = initialModel
        self.activeModelName = initialModelName
        
        self.setupStreamClient()
        
        // Restore draft from memory cache without blocking @MainActor on disk I/O
        if let memDraftImgs = resolvedCacheManager.getMemoryDraftImages(for: cascadeId) {
            self.selectedImageData = memDraftImgs
        } else {
            self.selectedImageData = []
        }
        self.inputText = resolvedCacheManager.getDraft(for: cascadeId)
        self.isInitializing = false
        self.restoreDraftsIfNeeded()
        
        // Instant restore from memory cache if warm; otherwise hydrate off-main-thread
        if let cached = memCachedSession {
            self.applyCachedSessionSnapshot(cached, cascadeId: cascadeId, resolvedCacheManager: resolvedCacheManager)
        } else if !cascadeId.isEmpty && !isDraftCascade && !isNewConversation {
            Task { @MainActor [weak self] in
                guard let self else { return }
                guard let coldCached = await resolvedCacheManager.loadSessionAsync(for: cascadeId) else { return }
                guard self.cascadeId == cascadeId, self.messages.isEmpty, self.knownServerMessageIds.isEmpty else { return }
                self.applyCachedSessionSnapshot(coldCached, cascadeId: cascadeId, resolvedCacheManager: resolvedCacheManager)
                if !self.hasUserManuallySelectedModel, !self.hasResolvedSessionModel, let cfg = coldCached.cascadeConfigRaw {
                    self.syncModel(from: ModelDefaultsLogic.extractModelEnum(fromConfig: cfg) ?? cfg)
                    self.hasResolvedSessionModel = true
                }
            }
        }
    }

    private func applyCachedSessionSnapshot(
        _ cached: CachedChatSession,
        cascadeId: String,
        resolvedCacheManager: CacheManager
    ) {
        if let ws = cached.workspaceName, !ws.isEmpty, ws != "Chat" {
            self.workspaceName = ws
        }
        let healed = self.sanitizeMessageOrder(cached.messages)
        self.messages = healed
        if let outline = cached.turnOutline, !outline.isEmpty {
            self.turnOutline = outline
        }
        self.duration = cached.duration
        self.stepCount = cached.stepCount
        self.totalTools = cached.totalTools
        let hasEarliest = healed.contains(where: { self.extractStepIndex(from: $0) == 0 })
        self.hasMore = hasEarliest ? false : cached.hasMore
        self.nextOffset = hasEarliest ? 0 : cached.nextOffset
        self.isRunning = (cached.status == "CASCADE_RUN_STATUS_RUNNING")
        let lastUserIdx = healed.lastIndex(where: { $0.isUser }) ?? -1
        let latestTurn = lastUserIdx >= 0 ? healed.suffix(from: lastUserIdx + 1) : healed[...]
        let latestEndedInError = latestTurn.last?.isError == true
        self.hasError = (cached.status == "CASCADE_RUN_STATUS_ERROR" || (!self.isRunning && latestEndedInError))
        if self.hasError {
            self.trajectoryErrorMessage = latestTurn.last(where: { $0.isError })?.content
        }
        self.cascadeConfigRaw = cached.cascadeConfigRaw
        self.canProceed = cached.canProceed ?? false
        self.proceedArtifactUri = cached.proceedArtifactUri
        self.pendingInteraction = cached.pendingInteraction
        if let origin = cached.branchOrigin {
            self.branchOrigin = origin
        }
        if let sides = cached.sideQuestions {
            self.sideQuestions = sides
        }
        let userMsgs = healed.filter { $0.isUser }
        self.queuedMessages = (cached.queuedMessages ?? []).filter { qm in
            Self.isUserQueuedItem(qm) && !Self.isQueuedItemInMessages(qm, userMessages: userMsgs)
        }
        self.knownServerMessageIds = Set(healed.map(\.id))
        if let cachedTitle = cached.title?.trimmingCharacters(in: CharacterSet.whitespacesAndNewlines), !cachedTitle.isEmpty, cachedTitle != "未命名会话" {
            self.currentTitle = cachedTitle
            resolvedCacheManager.updateConversationTitle(cascadeId: cascadeId, newTitle: cachedTitle)
        } else if self.currentTitle == "未命名会话" || self.currentTitle.isEmpty {
            if let firstUserMsg = healed.first(where: { $0.isUser }),
               let prompt = firstUserMsg.content.trimmingCharacters(in: CharacterSet.whitespacesAndNewlines).components(separatedBy: CharacterSet.newlines).first(where: { !$0.trimmingCharacters(in: CharacterSet.whitespacesAndNewlines).isEmpty }) {
                let derived = String(prompt.trimmingCharacters(in: CharacterSet.whitespacesAndNewlines).prefix(36))
                if !derived.isEmpty {
                    self.currentTitle = derived
                    resolvedCacheManager.updateConversationTitle(cascadeId: cascadeId, newTitle: derived)
                }
            }
        }
        if healed.count != cached.messages.count {
            resolvedCacheManager.saveSession(CachedChatSession(
                cascadeId: cached.cascadeId,
                status: cached.status,
                duration: cached.duration,
                stepCount: cached.stepCount,
                totalTools: cached.totalTools,
                hasMore: self.hasMore,
                nextOffset: self.nextOffset,
                messages: healed,
                turnOutline: self.turnOutline.isEmpty ? cached.turnOutline : self.turnOutline,
                title: self.currentTitle,
                cascadeConfigRaw: cached.cascadeConfigRaw,
                canProceed: cached.canProceed,
                proceedArtifactUri: cached.proceedArtifactUri,
                pendingInteraction: cached.pendingInteraction,
                queuedMessages: cached.queuedMessages,
                runningTasks: cached.runningTasks,
                workspaceName: cached.workspaceName,
                savedAt: cached.savedAt
            ))
        }
    }
    
    public init(
        draftProject: ProjectItem,
        apiClient: APIClient? = nil,
        settings: AppSettings? = nil,
        cacheManager: CacheManager? = nil
    ) {
        self.cascadeId = ""
        let isPure = draftProject.isPureChat
        let title = isPure ? String(localized: "新对话") : draftProject.name
        self.initialTitle = title
        self.currentTitle = title
        self.workspaceName = isPure ? "Chat" : draftProject.name
        self.isNewConversation = true
        self.draftProject = draftProject
        self.originDraftId = "draft_project_\(draftProject.id)"
        let resolvedCacheManager = cacheManager ?? .shared
        let resolvedSettings = settings ?? .shared
        self.apiClient = apiClient ?? .shared
        self.settings = resolvedSettings
        self.activityManager = ActivityManager.shared
        self.cacheManager = resolvedCacheManager
        self.streamClient = StreamWebSocketClient()
        self.activeModel = resolvedSettings.favoriteModel1
        let draftKey = "draft_project_\(draftProject.id)"
        let memImgs = resolvedCacheManager.getMemoryDraftImages(for: draftKey)
        self.selectedImageData = memImgs ?? []
        self.inputText = resolvedCacheManager.getDraft(for: draftKey)
        
        self.setupStreamClient()
        self.isInitializing = false
        self.restoreDraftsIfNeeded()
    }
    
    public init(
        draftSession: LocalDraftSession,
        apiClient: APIClient? = nil,
        settings: AppSettings? = nil,
        cacheManager: CacheManager? = nil
    ) {
        self.cascadeId = ""
        let isPure = draftSession.project.isPureChat
        let title = isPure ? String(localized: "新对话") : draftSession.project.name
        self.initialTitle = title
        self.currentTitle = title
        self.workspaceName = isPure ? "Chat" : draftSession.project.name
        self.isNewConversation = true
        self.draftSession = draftSession
        self.draftProject = draftSession.project
        self.originDraftId = draftSession.id
        let resolvedCacheManager = cacheManager ?? .shared
        let resolvedSettings = settings ?? .shared
        self.apiClient = apiClient ?? .shared
        self.settings = resolvedSettings
        self.activityManager = ActivityManager.shared
        self.cacheManager = resolvedCacheManager
        self.streamClient = StreamWebSocketClient()
        self.activeModel = resolvedSettings.favoriteModel1
        let memImgs = resolvedCacheManager.getMemoryDraftImages(for: draftSession.id)
        let sessionImages = !draftSession.draftImages.isEmpty ? draftSession.draftImages : (memImgs ?? [])
        self.selectedImageData = sessionImages
        if !sessionImages.isEmpty {
            resolvedCacheManager.saveDraftImages(key: draftSession.id, images: sessionImages)
        }
        let existingDraft = resolvedCacheManager.getDraft(for: draftSession.id)
        self.inputText = existingDraft.isEmpty ? draftSession.draftText : existingDraft
        
        self.setupStreamClient()
        self.isInitializing = false
        self.restoreDraftsIfNeeded()
    }
    
    @MainActor
    func saveEmptySessionToCache() {
        let emptyTitle = self.emptyConversationTitle
        self.cacheManager.recordConversationTouch(cascadeId: self.cascadeId)
        self.cacheManager.updateConversationTitle(cascadeId: self.cascadeId, newTitle: emptyTitle)
        self.cacheManager.upsertConversation(ConversationItem(
            id: self.cascadeId,
            title: emptyTitle,
            status: .idle,
            stepCount: 0,
            workspaceName: self.workspaceName,
            lastModified: Date()
        ))
        self.cacheManager.saveSession(CachedChatSession(
            cascadeId: self.cascadeId,
            status: "CASCADE_RUN_STATUS_IDLE",
            duration: String(localized: "0秒"),
            stepCount: 0,
            totalTools: 0,
            hasMore: false,
            nextOffset: 0,
            messages: [],
            title: emptyTitle,
            cascadeConfigRaw: self.cascadeConfigRaw,
            canProceed: false,
            proceedArtifactUri: nil,
            pendingInteraction: nil,
            queuedMessages: [],
            runningTasks: [],
            workspaceName: self.workspaceName
        ))
    }
    
    @MainActor
    public func loadMessages(isBackgroundPoll: Bool = false) async {
        guard !cascadeId.isEmpty else { return }
        if (self.workspaceName.isEmpty || self.workspaceName == "Chat") && !cascadeId.isEmpty {
            if let conv = cacheManager.loadConversations().first(where: { $0.id == cascadeId }),
               !conv.workspaceName.isEmpty, conv.workspaceName != "Chat" {
                self.workspaceName = conv.workspaceName
            }
        }
        // Fallback to cache if messages empty
        if messages.isEmpty, let cached = cacheManager.loadSession(for: cascadeId) {
            if let ws = cached.workspaceName, !ws.isEmpty, ws != "Chat" {
                self.workspaceName = ws
            }
            let healed = self.sanitizeMessageOrder(cached.messages)
            self.messages = healed
            if let outline = cached.turnOutline, !outline.isEmpty {
                self.turnOutline = outline
            }
            self.duration = cached.duration
            self.stepCount = cached.stepCount
            self.totalTools = cached.totalTools
            let hasEarliest = healed.contains(where: { self.extractStepIndex(from: $0) == 0 })
            self.hasMore = hasEarliest ? false : cached.hasMore
            self.nextOffset = hasEarliest ? 0 : cached.nextOffset
            self.isRunning = (cached.status == "CASCADE_RUN_STATUS_RUNNING")
            self.cascadeConfigRaw = cached.cascadeConfigRaw
            self.canProceed = false
            self.proceedArtifactUri = nil
            self.pendingInteraction = cached.pendingInteraction
            if let origin = cached.branchOrigin {
                self.branchOrigin = origin
            }
            if let sides = cached.sideQuestions {
                self.sideQuestions = sides
            }
            let userMsgs = healed.filter { $0.isUser }
            self.queuedMessages = (cached.queuedMessages ?? []).filter { qm in
                Self.isUserQueuedItem(qm) && !Self.isQueuedItemInMessages(qm, userMessages: userMsgs)
            }
            self.runningTasks = cached.runningTasks ?? []
            self.knownServerMessageIds = Set(healed.map(\.id))
            if healed.count != cached.messages.count {
                self.cacheManager.saveSession(CachedChatSession(
                    cascadeId: cached.cascadeId,
                    status: cached.status,
                    duration: cached.duration,
                    stepCount: cached.stepCount,
                    totalTools: cached.totalTools,
                    hasMore: self.hasMore,
                    nextOffset: self.nextOffset,
                    messages: healed,
                    title: cached.title,
                    cascadeConfigRaw: cached.cascadeConfigRaw,
                    canProceed: cached.canProceed,
                    proceedArtifactUri: cached.proceedArtifactUri,
                    pendingInteraction: cached.pendingInteraction,
                    queuedMessages: cached.queuedMessages,
                    runningTasks: cached.runningTasks,
                    workspaceName: cached.workspaceName,
                    savedAt: cached.savedAt
                ))
            }
        }
        
        guard let url = settings.serverURL else {
            if messages.isEmpty {
                errorMessage = String(localized: "未配置服务器地址")
            }
            return
        }
        
        if !isBackgroundPoll && messages.isEmpty && !isNewConversation {
            isLoading = true
        }
        errorMessage = nil
        
        do {
            let result = try await apiClient.fetchMessages(
                cascadeId: cascadeId,
                limit: 15,
                offset: nil,
                baseURL: url
            )
            if let configRaw = result.cascadeConfigRaw, !configRaw.isEmpty {
                self.cascadeConfigRaw = configRaw
                if self.hasUserManuallySelectedModel {
                    self.updateCascadeConfigRawModel(self.activeModelEnum, modelName: self.activeModel)
                }
            }
            ModelCatalogStore.shared.updateDesktopActiveModel(result.desktopActiveModel, name: result.desktopActiveModelName)
            let isDraft = self.isDraftConversation
            let hasSessionModel = result.activeModel?.isEmpty == false
            if hasSessionModel {
                self.hasResolvedSessionModel = true
            }
            let alignedModel = hasSessionModel
                ? result.activeModel
                : (!isDraft ? result.desktopActiveModel : nil)
            let alignedName = hasSessionModel
                ? result.activeModelName
                : (!isDraft ? result.desktopActiveModelName : nil)
            if let alignedModel, !alignedModel.isEmpty {
                // User manual model selection is authoritative and must not be overwritten by background polling
                if !self.hasUserManuallySelectedModel && !isBackgroundPoll {
                    self.syncModel(from: alignedModel)
                }
            }
            if !self.hasUserManuallySelectedModel && !isBackgroundPoll, let name = alignedName, !name.isEmpty { self.activeModelName = name }
            if let started = result.startedAt, !started.isEmpty { self.startedAt = started }
            if let cm = result.contextMeter { self.contextMeter = cm }
            if let swarm = result.swarm {
                self.swarmBrief = swarm
            } else if !isBackgroundPoll {
                self.swarmBrief = nil
            }
            if let subagents = result.subagents {
                if self.subagents != subagents { self.subagents = subagents }
            }
            let newParent = result.parentConversationId?.trimmingCharacters(in: .whitespacesAndNewlines)
            if let newParent, !newParent.isEmpty {
                self.parentConversationId = newParent
            } else if !isBackgroundPoll && result.parentConversationId != nil {
                self.parentConversationId = nil
            }
            if let role = result.subagentRole {
                self.subagentRole = role
            }
            if let suggestions = result.suggestedResponses {
                self.suggestedResponses = suggestions
            }
            self.activeFocus = result.activeFocus
            self.activeFocusStartedAt = result.activeFocusStartedAt
            self.activePlan = result.activePlan
            if let origin = result.branchOrigin {
                if self.branchOrigin != origin {
                    self.branchOrigin = origin
                }
            }
            let incomingSides = result.sideQuestions ?? []
            if self.sideQuestions != incomingSides {
                self.sideQuestions = incomingSides
            }
            
            if let outline = result.turnOutline {
                if self.turnOutline != outline {
                    self.turnOutline = outline
                }
            }
            
            if result.messages.isEmpty {
                if self.pendingOptimisticMessageId == nil {
                    withAnimation(.easeInOut(duration: 0.2)) {
                        self.messages = []
                        self.turnOutline = []
                        self.knownServerMessageIds = []
                        self.hasMore = false
                        self.nextOffset = 0
                        self.stepCount = 0
                        self.totalTools = 0
                        self.duration = String(localized: "0秒")
                        self.isNewConversation = true
                        self.currentTitle = self.emptyConversationTitle
                    }
                    self.saveEmptySessionToCache()
                }
            } else {
                if let title = result.title?.trimmingCharacters(in: CharacterSet.whitespacesAndNewlines), !title.isEmpty, title != "未命名会话" {
                    if self.currentTitle != title {
                        withAnimation(.easeInOut(duration: 0.25)) {
                            self.currentTitle = title
                        }
                        self.cacheManager.updateConversationTitle(cascadeId: self.cascadeId, newTitle: title)
                    }
                }
                
                let clientMaxStep = self.messages.compactMap { self.extractStepIndex(from: $0) }.max() ?? -1
                let serverMaxStep = result.messages.compactMap { self.extractStepIndex(from: $0) }.max() ?? -1
                let isTruncatedOrReverted = serverMaxStep < clientMaxStep
                
                if (isBackgroundPoll || self.pendingOptimisticMessageId != nil) && !self.messages.isEmpty && !isTruncatedOrReverted {
                    self.mergeIncomingMessages(result.messages)
                    if self.messages.contains(where: { self.extractStepIndex(from: $0) == 0 }) {
                        self.hasMore = false
                        self.nextOffset = 0
                    }
                } else if !self.messages.isEmpty && self.messages.count > result.messages.count && !isTruncatedOrReverted {
                    // Preserves cached/expanded history rather than truncating all older messages
                    self.mergeIncomingMessages(result.messages)
                    if self.messages.contains(where: { self.extractStepIndex(from: $0) == 0 }) {
                        self.hasMore = false
                        self.nextOffset = 0
                    }
                } else {
                    let healed = self.sanitizeMessageOrder(result.messages)
                    self.messages = healed
                    let hasEarliest = healed.contains(where: { self.extractStepIndex(from: $0) == 0 })
                    self.hasMore = hasEarliest ? false : result.hasMore
                    self.nextOffset = hasEarliest ? 0 : result.nextOffset
                    if self.pendingOptimisticMessageId == nil {
                        self.knownServerMessageIds = Set(healed.map(\.id))
                    }
                }
            }
            
            if self.currentTitle == "未命名会话" || self.currentTitle.isEmpty {
                if let firstUserMsg = self.messages.first(where: { $0.isUser }),
                   let prompt = firstUserMsg.content.trimmingCharacters(in: CharacterSet.whitespacesAndNewlines).components(separatedBy: CharacterSet.newlines).first(where: { !$0.trimmingCharacters(in: CharacterSet.whitespacesAndNewlines).isEmpty }) {
                    let derived = String(prompt.trimmingCharacters(in: CharacterSet.whitespacesAndNewlines).prefix(36))
                    if !derived.isEmpty {
                        withAnimation(.easeInOut(duration: 0.25)) {
                            self.currentTitle = derived
                        }
                        self.cacheManager.updateConversationTitle(cascadeId: self.cascadeId, newTitle: derived)
                    }
                }
            }
            
            self.stepCount = result.totalSteps
            self.totalTools = result.totalTools
            self.duration = result.duration
            self.isLoading = false
            
            let previouslyRunning = self.isRunning
            let wasAwaiting = self.isAwaitingResponse
            let lastUserIdx = self.messages.lastIndex(where: { $0.isUser }) ?? -1
            let latestTurn = lastUserIdx >= 0 ? self.messages.suffix(from: lastUserIdx + 1) : self.messages[...]
            let latestEndedInError = latestTurn.last?.isError == true
            
            let isTrajectoryError: Bool
            if result.status == "CASCADE_RUN_STATUS_RUNNING" {
                self.isRunning = true
                self.hasError = false
                self.trajectoryErrorMessage = nil
                isTrajectoryError = false
            } else {
                isTrajectoryError = result.hasError || result.status == "CASCADE_RUN_STATUS_ERROR" || latestEndedInError
                self.hasError = isTrajectoryError
                if isTrajectoryError {
                    self.trajectoryErrorMessage = result.errorMessage ?? latestTurn.last(where: { $0.isError })?.content
                    self.isRunning = false
                    self.isAwaitingResponse = false
                    self.awaitingResponseSince = nil
                } else if !self.isAwaitingResponse {
                    self.isRunning = false
                }
            }
            
            if !self.isRunning && !self.isAwaitingResponse {
                self.updateProceedState(canProceed: result.canProceed, artifactUri: result.proceedArtifactUri)
            } else {
                self.updateProceedState(canProceed: false, artifactUri: nil)
            }
            
            if self.isRunning {
                self.pendingInteraction = result.pendingInteraction
            } else {
                self.pendingInteraction = nil
            }
            
            self.syncQueuedMessages(serverQueue: result.queuedMessages)
            
            self.runningTasks = result.runningTasks
            
            // Persist latest state to cache (excluding temporary optimistic message)
            let toCache = self.messages.filter { $0.id != self.pendingOptimisticMessageId && !$0.id.hasPrefix("optimistic-") }
            cacheManager.saveSession(CachedChatSession(
                cascadeId: cascadeId,
                status: isTrajectoryError ? "CASCADE_RUN_STATUS_ERROR" : result.status,
                duration: result.duration,
                stepCount: result.totalSteps,
                totalTools: result.totalTools,
                hasMore: self.hasMore,
                nextOffset: self.nextOffset,
                messages: toCache,
                turnOutline: self.turnOutline.isEmpty ? nil : self.turnOutline,
                title: self.currentTitle,
                cascadeConfigRaw: self.cascadeConfigRaw,
                canProceed: self.canProceed,
                proceedArtifactUri: self.proceedArtifactUri,
                pendingInteraction: self.pendingInteraction,
                queuedMessages: self.queuedMessages,
                runningTasks: self.runningTasks,
                workspaceName: self.workspaceName,
                branchOrigin: self.branchOrigin,
                sideQuestions: self.sideQuestions.isEmpty ? nil : self.sideQuestions
            ))
            if self.pendingOptimisticMessageId == nil {
                self.knownServerMessageIds = Set(toCache.map(\.id))
            }
            
            let convStatus: ConversationItem.ConversationStatus = {
                if self.hasError {
                    return .error
                } else if self.canProceed || self.pendingInteraction != nil {
                    return .action
                } else if self.isRunning || self.runningTasks.hasBlockingTasks {
                    return .running
                } else {
                    return .idle
                }
            }()
            self.initialConversationStatus = convStatus
            cacheManager.updateConversationStatus(cascadeId: cascadeId, status: convStatus)
            
            // If awaiting response, check if agent has completed response
            if self.isAwaitingResponse && !isTrajectoryError {
                // If optimistic user message has not been incorporated by server yet, keep awaiting!
                if self.pendingOptimisticMessageId != nil {
                    // Server has not acknowledged user message yet; keep awaiting
                } else if let lastUserIdx = self.messages.lastIndex(where: { $0.sender == .user }) {
                    // Check if an agent response exists strictly AFTER the latest user message
                    let subsequent = self.messages.suffix(from: lastUserIdx + 1)
                    let hasAgentResponse = subsequent.contains(where: { $0.sender == .agent })
                    let hasErrorResponse = subsequent.contains(where: { $0.isError })
                    
                    if hasAgentResponse || hasErrorResponse {
                        self.isAwaitingResponse = false
                        self.awaitingResponseSince = nil
                        if result.status != "CASCADE_RUN_STATUS_RUNNING" {
                            self.isRunning = false
                            self.updateProceedState(canProceed: result.canProceed, artifactUri: result.proceedArtifactUri)
                            self.pendingInteraction = nil
                        }
                        UIImpactFeedbackGenerator(style: .light).impactOccurred()
                    } else if result.status != "CASCADE_RUN_STATUS_RUNNING" && !subsequent.isEmpty {
                        // Upstream has stopped running and subsequent steps exist; clear awaiting
                        self.isAwaitingResponse = false
                        self.awaitingResponseSince = nil
                        self.isRunning = false
                    } else if !self.isRunning && previouslyRunning {
                        if let since = awaitingResponseSince {
                            let elapsed = Date().timeIntervalSince(since)
                            if elapsed > 15.0 {
                                self.isAwaitingResponse = false
                                self.awaitingResponseSince = nil
                            }
                        }
                    }
                }
            }
            
            // Manage Live Activity
            syncLiveActivity()
            evaluateTurnDelightTransition(
                previouslyRunning: previouslyRunning,
                wasAwaiting: wasAwaiting,
                isError: isTrajectoryError
            )
            
            if !self.isRunning && previouslyRunning {
                // Agent just finished turn; schedule post-turn title verification tasks
                Task { [weak self] in
                    try? await Task.sleep(nanoseconds: 1_200_000_000)
                    guard let self else { return }
                    await self.loadMessages(isBackgroundPoll: true)
                    await self.checkAndRefreshTitle()
                    
                    try? await Task.sleep(nanoseconds: 2_000_000_000)
                    await self.loadMessages(isBackgroundPoll: true)
                    await self.checkAndRefreshTitle()
                }
            }
            
            // Connect to WebSocket stream for real-time updates (only during initial/user-initiated load, not background polling ticks)
            if !isBackgroundPoll {
                connectStream()
                
                // If stream is not yet connected, initiate fallback polling until stream connects
                if streamClient.status != .connected && pollTask == nil {
                    startPollingFallback()
                }
            }
        } catch {
            if !isBackgroundPoll && messages.isEmpty {
                errorMessage = error.localizedDescription
            }
            isLoading = false
        }
    }
    
    @MainActor
    public func loadOlderMessages() async {
        guard hasMore, !isLoadingOlder, let url = settings.serverURL else { return }
        if messages.contains(where: { extractStepIndex(from: $0) == 0 }) {
            self.hasMore = false
            self.nextOffset = 0
            return
        }
        isLoadingOlder = true
        do {
            let result = try await apiClient.fetchMessages(
                cascadeId: cascadeId,
                limit: 15,
                offset: nextOffset,
                baseURL: url
            )
            if let configRaw = result.cascadeConfigRaw, !configRaw.isEmpty {
                self.cascadeConfigRaw = configRaw
            }
            if let title = result.title?.trimmingCharacters(in: CharacterSet.whitespacesAndNewlines), !title.isEmpty, title != "未命名会话" {
                if self.currentTitle != title {
                    self.currentTitle = title
                    self.cacheManager.updateConversationTitle(cascadeId: self.cascadeId, newTitle: title)
                }
            }
            if let outline = result.turnOutline, !outline.isEmpty, self.turnOutline != outline {
                self.turnOutline = outline
            }
            let existingIds = Set(self.messages.map(\.id))
            let uniqueOlder = result.messages.filter { !existingIds.contains($0.id) }
            let combined = uniqueOlder + self.messages
            self.messages = sanitizeMessageOrder(combined)
            
            let hasEarliest = self.messages.contains(where: { self.extractStepIndex(from: $0) == 0 })
            if !result.hasMore || uniqueOlder.isEmpty || result.nextOffset <= 0 || hasEarliest {
                self.hasMore = false
                self.nextOffset = 0
            } else {
                self.hasMore = result.hasMore
                self.nextOffset = result.nextOffset
            }
            self.isLoadingOlder = false
            
            // Persist expanded message stream to cache
            cacheManager.saveSession(CachedChatSession(
                cascadeId: cascadeId,
                status: result.status,
                duration: result.duration,
                stepCount: result.totalSteps,
                totalTools: result.totalTools,
                hasMore: self.hasMore,
                nextOffset: self.nextOffset,
                messages: self.messages,
                turnOutline: self.turnOutline.isEmpty ? nil : self.turnOutline,
                title: self.currentTitle,
                cascadeConfigRaw: self.cascadeConfigRaw,
                canProceed: self.canProceed,
                proceedArtifactUri: self.proceedArtifactUri,
                pendingInteraction: self.pendingInteraction,
                queuedMessages: self.queuedMessages,
                runningTasks: self.runningTasks,
                workspaceName: self.workspaceName,
                branchOrigin: self.branchOrigin,
                sideQuestions: self.sideQuestions.isEmpty ? nil : self.sideQuestions
            ))
            
            UIImpactFeedbackGenerator(style: .light).impactOccurred()
        } catch {
            self.isLoadingOlder = false
        }
    }

    /// 按需一次性补齐历史消息直至覆盖目标锚点（支持新网关 `fromOffset` 单请求直达，并自动向下兼容旧网关分页循环）。
    @MainActor
    @discardableResult
    public func loadMessagesThrough(targetMessageId: String, targetOffset: Int?) async -> Bool {
        if messages.contains(where: { $0.id == targetMessageId }) {
            return true
        }
        guard hasMore, let url = settings.serverURL else {
            return messages.contains(where: { $0.id == targetMessageId })
        }
        // 等待可能正在进行中的单页加载收尾，避免并发重复拉取
        for _ in 0..<25 where isLoadingOlder {
            try? await Task.sleep(nanoseconds: 40_000_000)
        }
        if messages.contains(where: { $0.id == targetMessageId }) {
            return true
        }
        guard hasMore, !isLoadingOlder else {
            return messages.contains(where: { $0.id == targetMessageId })
        }
        if messages.contains(where: { extractStepIndex(from: $0) == 0 }) {
            self.hasMore = false
            self.nextOffset = 0
            return messages.contains(where: { $0.id == targetMessageId })
        }

        // 目标靠近会话起点（≤ 15 条）时直接对齐到 0，避免顶部残留极短的「查看更早的消息」按钮
        let rawTarget = max(0, targetOffset ?? 0)
        let desiredStart = rawTarget <= 15 ? 0 : rawTarget

        isLoadingOlder = true
        defer { isLoadingOlder = false }

        var lastResult: FetchMessagesResult? = nil
        do {
            for _ in 0..<12 {
                guard hasMore else { break }
                if messages.contains(where: { $0.id == targetMessageId }) { break }
                let effectiveFrom = (nextOffset <= desiredStart) ? 0 : desiredStart

                let result = try await apiClient.fetchMessages(
                    cascadeId: cascadeId,
                    limit: 50,
                    offset: nextOffset,
                    fromOffset: effectiveFrom,
                    baseURL: url
                )
                lastResult = result

                if let configRaw = result.cascadeConfigRaw, !configRaw.isEmpty {
                    self.cascadeConfigRaw = configRaw
                }
                if let title = result.title?.trimmingCharacters(in: CharacterSet.whitespacesAndNewlines),
                   !title.isEmpty, title != "未命名会话" {
                    if self.currentTitle != title {
                        self.currentTitle = title
                        self.cacheManager.updateConversationTitle(cascadeId: self.cascadeId, newTitle: title)
                    }
                }
                if let outline = result.turnOutline, !outline.isEmpty, self.turnOutline != outline {
                    self.turnOutline = outline
                }

                let existingIds = Set(self.messages.map(\.id))
                let uniqueOlder = result.messages.filter { !existingIds.contains($0.id) }
                if !uniqueOlder.isEmpty {
                    self.messages = sanitizeMessageOrder(uniqueOlder + self.messages)
                }

                let hasEarliest = self.messages.contains(where: { self.extractStepIndex(from: $0) == 0 })
                if !result.hasMore || uniqueOlder.isEmpty || result.nextOffset <= 0 || hasEarliest {
                    self.hasMore = false
                    self.nextOffset = 0
                    break
                } else {
                    self.hasMore = result.hasMore
                    self.nextOffset = result.nextOffset
                }
            }

            if let res = lastResult {
                let toCache = self.messages.filter { $0.id != self.pendingOptimisticMessageId && !$0.id.hasPrefix("optimistic-") }
                cacheManager.saveSession(CachedChatSession(
                    cascadeId: cascadeId,
                    status: res.status,
                    duration: res.duration,
                    stepCount: res.totalSteps,
                    totalTools: res.totalTools,
                    hasMore: self.hasMore,
                    nextOffset: self.nextOffset,
                    messages: toCache,
                    turnOutline: self.turnOutline.isEmpty ? nil : self.turnOutline,
                    title: self.currentTitle,
                    cascadeConfigRaw: self.cascadeConfigRaw,
                    canProceed: self.canProceed,
                    proceedArtifactUri: self.proceedArtifactUri,
                    pendingInteraction: self.pendingInteraction,
                    queuedMessages: self.queuedMessages,
                    runningTasks: self.runningTasks
                ))
            }
        } catch {
            // 保留已成功补齐的部分历史，静默退出加载态
        }

        return messages.contains(where: { $0.id == targetMessageId }) || !hasMore
    }

    /// 评估本轮会话状态翻转是否构成「前台亲眼见证 Agent 顺利交付」的正向爽点，
    /// 或新发生的报错摩擦。WebSocket 流式推送与 HTTP 轮询共用此判定，避免重复计分。
    @MainActor
    func evaluateTurnDelightTransition(previouslyRunning: Bool, wasAwaiting: Bool, isError: Bool) {
        let wasActiveTurn = previouslyRunning || wasAwaiting
        if isError {
            if wasActiveTurn {
                currentTurnHadRemoteApproval = false
                AppReviewManager.shared.recordFriction()
            }
            return
        }

        guard wasActiveTurn else { return }
        guard !isRunning, !isAwaitingResponse, !hasError, errorMessage == nil else { return }
        guard pendingInteraction == nil, !canProceed, runningTasks.blockingTasks.isEmpty else { return }
        guard Date().timeIntervalSince(lastUserCancelledAt) > 10 else { return }
        guard !cascadeId.isEmpty, Self.activeCascadeId == cascadeId else { return }

        let lastUserIdx = messages.lastIndex(where: { $0.isUser }) ?? -1
        let latestTurn = lastUserIdx >= 0 ? messages.suffix(from: lastUserIdx + 1) : messages[...]
        guard !latestTurn.contains(where: { $0.isError }) else { return }

        let meaningfulAgentMsg = latestTurn.last(where: { msg in
            guard msg.sender == .agent else { return false }
            return !msg.content.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
                || !msg.imageUrls.isEmpty
                || !msg.imageDataList.isEmpty
                || !msg.artifacts.isEmpty
        })
        guard let anchorMsg = meaningfulAgentMsg else { return }

        let usedTools = latestTurn.contains { msg in
            if case .toolBatch = msg.sender { return true }
            return msg.toolCount > 0 || msg.sender == .subagent || msg.sender == .workflow
        }
        let afterApproval = currentTurnHadRemoteApproval
        currentTurnHadRemoteApproval = false

        AppReviewManager.shared.recordTurnCompleted(
            cascadeId: cascadeId,
            anchorMessageId: anchorMsg.id,
            usedTools: usedTools,
            afterRemoteApproval: afterApproval
        ) { [weak self] in
            guard let self else { return false }
            return Self.activeCascadeId == self.cascadeId
                && !self.isRunning
                && !self.isAwaitingResponse
                && !self.isSending
                && !self.isSubmittingInteraction
                && !self.hasError
                && self.errorMessage == nil
                && self.pendingInteraction == nil
                && !self.canProceed
                && self.runningTasks.blockingTasks.isEmpty
                && self.inputText.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
        }
    }
    
}
