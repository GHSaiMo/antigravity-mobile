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
    public var cascadeId: String
    public let initialTitle: String
    public var currentTitle: String
    public var workspaceName: String = "Chat"
    public var isNewConversation: Bool
    public var draftProject: ProjectItem?
    public var draftSession: LocalDraftSession?
    public var originDraftId: String? = nil
    var isInitializing: Bool = true
    
    public var messages: [ChatMessage] = []
    public var selectedImageData: [Data] = []
    /// Non-image attachments (documents, archives, source files) of the current draft.
    public var selectedFiles: [DraftFile] = []
    /// One-shot message about rejected / failed attachments, shown as an alert by the view.
    public var attachmentNotice: String?
    @ObservationIgnored var fileUploadTasks: [String: Task<Void, Never>] = [:]
    public var inputText: String = "" {
        didSet {
            guard !isInitializing else { return }
            saveCurrentDraft()
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
            return "新对话"
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
        return "新对话"
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
    
    public func updateDraftImages(_ images: [Data]) {
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
        selectedImageData.remove(at: index)
        if var updated = draftSession {
            updated.draftImages = selectedImageData
            updated.updatedAt = Date()
            self.draftSession = updated
            cacheManager.saveLocalDraftSession(updated)
        }
        saveCurrentDraft()
    }
    
    public func saveCurrentDraft() {
        guard !isInitializing else { return }
        let key = draftKey
        guard !key.isEmpty else { return }
        cacheManager.saveDraftImages(key: key, images: selectedImageData)
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
        let key = draftKey
        guard !key.isEmpty else { return }
        let currentDraftSession = self.draftSession
        
        Task.detached(priority: .userInitiated) { [weak self, key, currentDraftSession] in
            guard let self else { return }
            var cachedImages = await CacheManager.shared.getDraftImagesAsync(for: key)
            if cachedImages.isEmpty, let dSession = currentDraftSession, !dSession.draftImages.isEmpty {
                cachedImages = dSession.draftImages
                CacheManager.shared.saveDraftImages(key: key, images: cachedImages)
            }
            let cachedDraft = CacheManager.shared.getDraft(for: key)
            let resolvedDraft = !cachedDraft.isEmpty ? cachedDraft : (currentDraftSession?.draftText ?? "")
            
            await MainActor.run { [weak self] in
                guard let self else { return }
                guard self.draftKey == key else { return }
                if !cachedImages.isEmpty && self.selectedImageData.isEmpty {
                    self.selectedImageData = cachedImages
                }
                self.restoreDraftFiles()
                if !resolvedDraft.isEmpty && self.inputText.isEmpty {
                    self.inputText = resolvedDraft
                }
            }
        }
    }
    
    public var isLoading: Bool = false
    public var isRunning: Bool = false
    public var stepCount: Int = 0
    public var totalTools: Int = 0
    public var duration: String = "0秒"
    public var hasMore: Bool = false
    public var nextOffset: Int = 0
    public var isLoadingOlder: Bool = false
    public var errorMessage: String? = nil
    public var hasError: Bool = false
    public var trajectoryErrorMessage: String? = nil
    public var cascadeConfigRaw: String? = nil
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
    var lastAutoApprovedInteractionId: String? = nil
    public var queuedMessages: [QueuedMessageItem] = []
    public var runningTasks: [RunningTaskItem] = []
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
    
    // Revert / Undo State
    public var revertPreview: RevertPreviewResponse? = nil
    public var activeUndoMessage: ChatMessage? = nil
    public var showConfirmUndoSheet: Bool = false
    public var isReverting: Bool = false
    public var isLoadingRevertPreview: Bool = false
    public var focusInputTrigger: Int = 0
    
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
        isRunning || isAwaitingResponse || !runningTasks.isEmpty || (initialConversationStatus?.isRunning == true)
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
    
    public var isClaudeActive: Bool {
        activeModel.lowercased().contains("claude") || activeModel == "MODEL_PLACEHOLDER_M26"
    }
    
    public var activeModelEnum: String {
        isClaudeActive ? "MODEL_PLACEHOLDER_M26" : "MODEL_PLACEHOLDER_M318"
    }
    
    public var activeModelDisplayName: String {
        isClaudeActive ? "Claude" : "Gemini"
    }
    
    public func syncModel(from raw: String?) {
        guard let raw = raw, !raw.isEmpty else { return }
        let lower = raw.lowercased()
        let target = (lower.contains("claude") || lower.contains("m26")) ? "claude-opus-4-6-thinking" : "gemini-3.8-flash-high"
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
    public private(set) var streamClient: StreamWebSocketClient
    let apiClient: APIClient
    let settings: AppSettings
    let activityManager: ActivityManager
    let cacheManager: CacheManager
    var lastResumeTime: Date = .distantPast
    
    public init(
        cascadeId: String,
        initialTitle: String,
        workspaceName: String = "Chat",
        isNewConversation: Bool = false,
        apiClient: APIClient? = nil,
        settings: AppSettings? = nil,
        cacheManager: CacheManager? = nil,
        isUnread: Bool = false,
        conversationStatus: ConversationItem.ConversationStatus? = nil
    ) {
        self.cascadeId = cascadeId
        self.initialTitle = initialTitle
        self.currentTitle = initialTitle
        self.workspaceName = workspaceName
        self.isNewConversation = isNewConversation
        self.isUnreadOnEntry = isUnread
        self.initialConversationStatus = conversationStatus
        let resolvedApiClient = apiClient ?? .shared
        let resolvedSettings = settings ?? .shared
        let resolvedCacheManager = cacheManager ?? .shared
        self.apiClient = resolvedApiClient
        self.settings = resolvedSettings
        self.activityManager = ActivityManager.shared
        self.cacheManager = resolvedCacheManager
        self.streamClient = StreamWebSocketClient()
        
        if cascadeId.hasPrefix("local_draft_") || cascadeId.hasPrefix("draft_") {
            self.originDraftId = cascadeId
            if let session = resolvedCacheManager.getLocalDraftSession(id: cascadeId) {
                self.draftSession = session
                self.draftProject = session.project
            }
            self.cascadeId = ""
            self.isNewConversation = true
        }
        
        var initialModel = resolvedSettings.activeModel
        if let cached = resolvedCacheManager.loadSession(for: cascadeId),
           let cfg = cached.cascadeConfigRaw {
            let lower = cfg.lowercased()
            if lower.contains("m26") || lower.contains("claude") {
                initialModel = "claude-opus-4-6-thinking"
            } else if lower.contains("m318") || lower.contains("gemini") {
                initialModel = "gemini-3.8-flash-high"
            }
        }
        self.activeModel = initialModel
        
        self.setupStreamClient()
        
        // Restore draft from local cache
        self.selectedImageData = resolvedCacheManager.getDraftImages(for: cascadeId)
        self.inputText = resolvedCacheManager.getDraft(for: cascadeId)
        self.isInitializing = false
        
        // Instant restore from local cache
        if let cached = resolvedCacheManager.loadSession(for: cascadeId) {
            if let ws = cached.workspaceName, !ws.isEmpty, ws != "Chat" {
                self.workspaceName = ws
            }
            let healed = self.sanitizeMessageOrder(cached.messages)
            self.messages = healed
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
        let title = isPure ? "新对话" : draftProject.name
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
        self.activeModel = resolvedSettings.activeModel
        let draftKey = "draft_project_\(draftProject.id)"
        self.selectedImageData = resolvedCacheManager.getDraftImages(for: draftKey)
        self.inputText = resolvedCacheManager.getDraft(for: draftKey)
        
        self.setupStreamClient()
        self.isInitializing = false
    }
    
    public init(
        draftSession: LocalDraftSession,
        apiClient: APIClient? = nil,
        settings: AppSettings? = nil,
        cacheManager: CacheManager? = nil
    ) {
        self.cascadeId = ""
        let isPure = draftSession.project.isPureChat
        let title = isPure ? "新对话" : draftSession.project.name
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
        self.activeModel = resolvedSettings.activeModel
        let sessionImages = !draftSession.draftImages.isEmpty ? draftSession.draftImages : resolvedCacheManager.getDraftImages(for: draftSession.id)
        self.selectedImageData = sessionImages
        if !sessionImages.isEmpty {
            resolvedCacheManager.saveDraftImages(key: draftSession.id, images: sessionImages)
        }
        let existingDraft = resolvedCacheManager.getDraft(for: draftSession.id)
        self.inputText = existingDraft.isEmpty ? draftSession.draftText : existingDraft
        
        self.setupStreamClient()
        self.isInitializing = false
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
            duration: "0秒",
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
            let userMsgs = healed.filter { $0.isUser }
            self.queuedMessages = (cached.queuedMessages ?? []).filter { qm in
                Self.isUserQueuedItem(qm) && !Self.isQueuedItemInMessages(qm, userMessages: userMsgs)
            }
            self.runningTasks = cached.runningTasks ?? []
            self.knownServerMessageIds = Set(healed.map(\.id))
        }
        
        guard let url = settings.serverURL else {
            if messages.isEmpty {
                errorMessage = "未配置服务器地址"
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
            if let activeModel = result.activeModel, !activeModel.isEmpty {
                // User manual model selection is authoritative and must not be overwritten by background polling
                if !self.hasUserManuallySelectedModel && !isBackgroundPoll {
                    self.syncModel(from: activeModel)
                }
            }
            
            if result.messages.isEmpty {
                if self.pendingOptimisticMessageId == nil {
                    withAnimation(.easeInOut(duration: 0.2)) {
                        self.messages = []
                        self.knownServerMessageIds = []
                        self.hasMore = false
                        self.nextOffset = 0
                        self.stepCount = 0
                        self.totalTools = 0
                        self.duration = "0秒"
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
                self.checkAutoApproveInteractionIfNeeded(result.pendingInteraction)
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
                title: self.currentTitle,
                cascadeConfigRaw: self.cascadeConfigRaw,
                canProceed: self.canProceed,
                proceedArtifactUri: self.proceedArtifactUri,
                pendingInteraction: self.pendingInteraction,
                queuedMessages: self.queuedMessages,
                runningTasks: self.runningTasks
            ))
            if self.pendingOptimisticMessageId == nil {
                self.knownServerMessageIds = Set(toCache.map(\.id))
            }
            
            let convStatus: ConversationItem.ConversationStatus = {
                if self.hasError {
                    return .error
                } else if self.canProceed || self.pendingInteraction != nil {
                    return .action
                } else if self.isRunning || !self.runningTasks.isEmpty {
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
                title: self.currentTitle,
                cascadeConfigRaw: self.cascadeConfigRaw,
                canProceed: self.canProceed,
                proceedArtifactUri: self.proceedArtifactUri,
                pendingInteraction: self.pendingInteraction,
                queuedMessages: self.queuedMessages,
                runningTasks: self.runningTasks
            ))
            
            UIImpactFeedbackGenerator(style: .light).impactOccurred()
        } catch {
            self.isLoadingOlder = false
        }
    }
    
}
