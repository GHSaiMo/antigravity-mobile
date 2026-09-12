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
    public var errorMessage: String?
    public var canProceed: Bool
    
    public init(
        id: String,
        title: String,
        uri: String,
        content: String = "",
        summary: String? = nil,
        isLoading: Bool = false,
        errorMessage: String? = nil,
        canProceed: Bool = false
    ) {
        self.id = id
        self.title = title
        self.uri = uri
        self.content = content
        self.summary = summary
        self.isLoading = isLoading
        self.errorMessage = errorMessage
        self.canProceed = canProceed
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
    public var isNewConversation: Bool
    public var draftProject: ProjectItem?
    public var draftSession: LocalDraftSession?
    
    public var messages: [ChatMessage] = []
    public var selectedImageData: [Data] = []
    public var inputText: String = "" {
        didSet {
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
        }
        return ""
    }
    
    public func updateDraftImages(_ images: [Data]) {
        self.selectedImageData = images
        saveCurrentDraft()
    }
    
    public func removeDraftImage(at index: Int) {
        guard index < selectedImageData.count else { return }
        selectedImageData.remove(at: index)
        saveCurrentDraft()
    }
    
    public func saveCurrentDraft() {
        let key = draftKey
        guard !key.isEmpty else { return }
        cacheManager.saveDraft(key: key, text: inputText)
        cacheManager.saveDraftImages(key: key, images: selectedImageData)
        if let draftSession, key == draftSession.id {
            var updated = draftSession
            updated.draftText = inputText
            updated.updatedAt = Date()
            self.draftSession = updated
            cacheManager.saveLocalDraftSession(updated)
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
    public var canProceed: Bool = false
    public var proceedArtifactUri: String? = nil
    public var pendingInteraction: PendingInteraction? = nil
    public var isSubmittingInteraction: Bool = false
    public var queuedMessages: [QueuedMessageItem] = []
    public var runningTasks: [RunningTaskItem] = []
    public var isSending: Bool = false
    public var viewingMarkdownFile: MarkdownFileViewerData? = nil
    public var isDownloadingDocument: Bool = false
    public var downloadingDocumentName: String = ""
    public var downloadProgress: Double = 0.0
    public var downloadBytesWritten: Int64 = 0
    public var downloadBytesTotal: Int64 = 0
    private var documentDownloadTask: Task<Void, Never>? = nil
    public var quickLookURL: URL? = nil
    public var htmlPreviewURL: URL? = nil
    public var htmlPreviewTitle: String = ""
    
    /// ID of the first message of the latest response turn (e.g., tool batch or agent response following the last user message)
    public var latestTurnStartMessageId: String? {
        guard let lastUserIdx = messages.lastIndex(where: { $0.sender == .user }) else {
            return messages.last(where: { $0.sender != .user })?.id ?? messages.last?.id
        }
        let subsequent = messages.suffix(from: lastUserIdx + 1)
        return subsequent.first?.id ?? messages.last?.id
    }
    
    /// ID of the latest agent response message (the actual text bubble from the agent, not tool batches)
    public var latestAgentMessageId: String? {
        if let lastUserIdx = messages.lastIndex(where: { $0.sender == .user }) {
            let subsequent = messages.suffix(from: lastUserIdx + 1)
            if let agentMsg = subsequent.last(where: { $0.sender == .agent }) {
                return agentMsg.id
            }
        }
        return messages.last(where: { $0.sender == .agent })?.id
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
    
    private var awaitingResponseSince: Date? = nil
    private var pendingOptimisticMessageId: String? = nil
    private struct PendingOptimisticQueueItem {
        let id: String
        let text: String
        let createdAt: Date
    }
    private var pendingOptimisticQueueItems: [PendingOptimisticQueueItem] = []
    private var knownServerMessageIds: Set<String> = []
    private var pollTask: Task<Void, Never>?
    private var hasUserManuallySelectedModel: Bool = false
    public private(set) var streamClient: StreamWebSocketClient
    private let apiClient: APIClient
    private let settings: AppSettings
    private let activityManager: ActivityManager
    private let cacheManager: CacheManager
    
    public init(
        cascadeId: String,
        initialTitle: String,
        isNewConversation: Bool = false,
        apiClient: APIClient? = nil,
        settings: AppSettings? = nil,
        cacheManager: CacheManager? = nil
    ) {
        self.cascadeId = cascadeId
        self.initialTitle = initialTitle
        self.currentTitle = initialTitle
        self.isNewConversation = isNewConversation
        let resolvedApiClient = apiClient ?? .shared
        let resolvedSettings = settings ?? .shared
        let resolvedCacheManager = cacheManager ?? .shared
        self.apiClient = resolvedApiClient
        self.settings = resolvedSettings
        self.activityManager = ActivityManager.shared
        self.cacheManager = resolvedCacheManager
        self.streamClient = StreamWebSocketClient()
        
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
        self.inputText = resolvedCacheManager.getDraft(for: cascadeId)
        self.selectedImageData = resolvedCacheManager.getDraftImages(for: cascadeId)
        
        // Instant restore from local cache
        if let cached = resolvedCacheManager.loadSession(for: cascadeId) {
            let healed = self.sanitizeMessageOrder(cached.messages)
            self.messages = healed
            self.duration = cached.duration
            self.stepCount = cached.stepCount
            self.totalTools = cached.totalTools
            self.hasMore = cached.hasMore
            self.nextOffset = cached.nextOffset
            self.isRunning = (cached.status == "CASCADE_RUN_STATUS_RUNNING")
            let lastUserIdx = healed.lastIndex(where: { $0.isUser }) ?? -1
            let latestTurn = lastUserIdx >= 0 ? healed.suffix(from: lastUserIdx + 1) : healed[...]
            let latestHasErr = latestTurn.contains(where: { $0.isError }) && !(latestTurn.last?.isAgent == true)
            self.hasError = (cached.status == "CASCADE_RUN_STATUS_ERROR" || latestHasErr)
            if self.hasError {
                self.trajectoryErrorMessage = latestTurn.last(where: { $0.isError })?.content
            }
            self.cascadeConfigRaw = cached.cascadeConfigRaw
            self.canProceed = false
            self.proceedArtifactUri = nil
            self.pendingInteraction = cached.pendingInteraction
            let recentUser = Set(healed.filter { $0.sender == .user }.suffix(15).map { $0.content.trimmingCharacters(in: .whitespacesAndNewlines) })
            self.queuedMessages = (cached.queuedMessages ?? []).filter { !recentUser.contains($0.text.trimmingCharacters(in: .whitespacesAndNewlines)) }
            self.knownServerMessageIds = Set(healed.map(\.id))
            if let cachedTitle = cached.title, !cachedTitle.isEmpty, cachedTitle != "未命名会话" {
                self.currentTitle = cachedTitle
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
        self.initialTitle = draftProject.name
        self.currentTitle = draftProject.name
        self.isNewConversation = true
        self.draftProject = draftProject
        let resolvedCacheManager = cacheManager ?? .shared
        let resolvedSettings = settings ?? .shared
        self.apiClient = apiClient ?? .shared
        self.settings = resolvedSettings
        self.activityManager = ActivityManager.shared
        self.cacheManager = resolvedCacheManager
        self.streamClient = StreamWebSocketClient()
        self.activeModel = resolvedSettings.activeModel
        self.inputText = resolvedCacheManager.getDraft(for: "draft_project_\(draftProject.id)")
        self.selectedImageData = resolvedCacheManager.getDraftImages(for: "draft_project_\(draftProject.id)")
        
        self.setupStreamClient()
    }
    
    public init(
        draftSession: LocalDraftSession,
        apiClient: APIClient? = nil,
        settings: AppSettings? = nil,
        cacheManager: CacheManager? = nil
    ) {
        self.cascadeId = ""
        self.initialTitle = draftSession.project.name
        self.currentTitle = draftSession.project.name
        self.isNewConversation = true
        self.draftSession = draftSession
        self.draftProject = draftSession.project
        let resolvedCacheManager = cacheManager ?? .shared
        let resolvedSettings = settings ?? .shared
        self.apiClient = apiClient ?? .shared
        self.settings = resolvedSettings
        self.activityManager = ActivityManager.shared
        self.cacheManager = resolvedCacheManager
        self.streamClient = StreamWebSocketClient()
        self.activeModel = resolvedSettings.activeModel
        let existingDraft = resolvedCacheManager.getDraft(for: draftSession.id)
        self.inputText = existingDraft.isEmpty ? draftSession.draftText : existingDraft
        self.selectedImageData = resolvedCacheManager.getDraftImages(for: draftSession.id)
        
        self.setupStreamClient()
    }
    
    @MainActor
    public func loadMessages(isBackgroundPoll: Bool = false) async {
        guard !cascadeId.isEmpty else { return }
        // Fallback to cache if messages empty
        if messages.isEmpty, let cached = cacheManager.loadSession(for: cascadeId) {
            let healed = self.sanitizeMessageOrder(cached.messages)
            self.messages = healed
            self.duration = cached.duration
            self.stepCount = cached.stepCount
            self.totalTools = cached.totalTools
            self.hasMore = cached.hasMore
            self.nextOffset = cached.nextOffset
            self.isRunning = (cached.status == "CASCADE_RUN_STATUS_RUNNING")
            self.cascadeConfigRaw = cached.cascadeConfigRaw
            self.canProceed = false
            self.proceedArtifactUri = nil
            self.pendingInteraction = cached.pendingInteraction
            let recentUser = Set(healed.filter { $0.sender == .user }.suffix(15).map { $0.content.trimmingCharacters(in: .whitespacesAndNewlines) })
            self.queuedMessages = (cached.queuedMessages ?? []).filter { !recentUser.contains($0.text.trimmingCharacters(in: .whitespacesAndNewlines)) }
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
            
            if let title = result.title?.trimmingCharacters(in: CharacterSet.whitespacesAndNewlines), !title.isEmpty, title != "未命名会话" {
                if self.currentTitle != title {
                    withAnimation(.easeInOut(duration: 0.25)) {
                        self.currentTitle = title
                    }
                    self.cacheManager.updateConversationTitle(cascadeId: self.cascadeId, newTitle: title)
                }
            }
            
            if (isBackgroundPoll || self.pendingOptimisticMessageId != nil) && !self.messages.isEmpty {
                self.mergeIncomingMessages(result.messages)
            } else if !self.messages.isEmpty && self.messages.count > result.messages.count {
                // Preserves cached/expanded history rather than truncating all older messages
                self.mergeIncomingMessages(result.messages)
            } else {
                let healed = self.sanitizeMessageOrder(result.messages)
                self.messages = healed
                self.hasMore = result.hasMore
                self.nextOffset = result.nextOffset
                if self.pendingOptimisticMessageId == nil {
                    self.knownServerMessageIds = Set(healed.map(\.id))
                }
            }
            
            self.stepCount = result.totalSteps
            self.totalTools = result.totalTools
            self.duration = result.duration
            self.isLoading = false
            
            let previouslyRunning = self.isRunning
            let lastUserIdx = self.messages.lastIndex(where: { $0.isUser }) ?? -1
            let latestTurn = lastUserIdx >= 0 ? self.messages.suffix(from: lastUserIdx + 1) : self.messages[...]
            let latestHasErr = latestTurn.contains(where: { $0.isError }) && !(latestTurn.last?.isAgent == true)
            let isTrajectoryError = result.hasError || result.status == "CASCADE_RUN_STATUS_ERROR" || latestHasErr
            self.hasError = isTrajectoryError
            if isTrajectoryError {
                self.trajectoryErrorMessage = result.errorMessage ?? latestTurn.last(where: { $0.isError })?.content
                self.isRunning = false
                self.isAwaitingResponse = false
                self.awaitingResponseSince = nil
            } else if result.status == "CASCADE_RUN_STATUS_RUNNING" {
                self.isRunning = true
            } else if !self.isAwaitingResponse {
                self.isRunning = false
            }
            
            if !self.isRunning && !self.isAwaitingResponse {
                self.canProceed = result.canProceed
                self.proceedArtifactUri = result.proceedArtifactUri
            } else {
                self.canProceed = false
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
                } else if self.isRunning {
                    return .running
                } else {
                    return .idle
                }
            }()
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
                            self.canProceed = result.canProceed
                            self.proceedArtifactUri = result.proceedArtifactUri
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
            if settings.enableLiveActivities {
                if isRunning && !previouslyRunning {
                    activityManager.startActivity(title: currentTitle, cascadeId: cascadeId)
                } else if isRunning {
                    let latestAction = result.messages.last?.content ?? "正在执行..."
                    activityManager.updateActivity(status: "RUNNING", stepCount: result.totalSteps, latestAction: latestAction)
                } else if !isRunning && previouslyRunning {
                    activityManager.endActivity(finalStatus: "COMPLETED")
                }
            }
            
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
            
            // Connect to WebSocket stream for real-time updates
            connectStream()
            
            // Manage background polling fallback: only poll if WS is not actively connected
            let shouldPoll = (self.isRunning || self.isAwaitingResponse) && streamClient.status != .connected
            if shouldPoll && pollTask == nil {
                startPollingFallback()
            } else if !shouldPoll && pollTask != nil {
                stopPollingFallback()
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
                }
            }
            self.messages = result.messages + self.messages
            self.hasMore = result.hasMore
            self.nextOffset = result.nextOffset
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
    
    // MARK: - Message Sequencing and Self-Healing
    
    /// Extracts a numeric step index from a message ID (e.g. "step-12" -> 12).
    private func extractStepIndex(from id: String) -> Int? {
        if id.hasPrefix("step-"), let val = Int(id.dropFirst(5)) {
            return val
        }
        return nil
    }
    
    /// Detects and self-heals inversion anomalies (e.g. latest turn appearing before earliest turn).
    private func sanitizeMessageOrder(_ list: [ChatMessage]) -> [ChatMessage] {
        guard list.count >= 2 else { return list }
        
        // Find if there is a severe step drop point (e.g. index 10 has step-25 and index 11 has step-0)
        var dropIndex: Int? = nil
        var prevStep = -1
        
        for (i, msg) in list.enumerated() {
            if let step = extractStepIndex(from: msg.id) {
                if prevStep != -1 && step < prevStep && (prevStep - step) >= 2 {
                    // Sudden backwards jump in step index detected!
                    dropIndex = i
                    break
                }
                prevStep = step
            }
        }
        
        if let drop = dropIndex {
            // Segment 1 (0..<drop) was placed at top (latest messages)
            // Segment 2 (drop..<count) was appended at bottom (earliest messages)
            let head = Array(list[0..<drop])
            let tail = Array(list[drop...])
            
            // Re-swap: earlier messages should come first
            let healed = tail + head
            return healed
        }
        
        return list
    }
    
    /// Applies a full trajectory snapshot directly without destructive slicing or reverse appends.
    @MainActor
    private func applySnapshotMessages(_ incoming: [ChatMessage], hasMore: Bool = false, nextOffset: Int = 0) {
        guard !incoming.isEmpty else { return }
        
        // 1. Check if server has incorporated the pending optimistic user message
        let currentOptId = self.pendingOptimisticMessageId
        var optimisticMessage: ChatMessage? = nil
        if let optId = currentOptId {
            optimisticMessage = messages.first(where: { $0.id == optId })
            let serverHasNewUserMsg = incoming.contains(where: {
                ($0.sender == .user && !knownServerMessageIds.contains($0.id)) ||
                ($0.sender == .user && optimisticMessage != nil && $0.content == optimisticMessage?.content)
            })
            if serverHasNewUserMsg {
                self.pendingOptimisticMessageId = nil
                optimisticMessage = nil
            }
        }
        
        var fullList = sanitizeMessageOrder(incoming)
        if let opt = optimisticMessage {
            fullList.append(opt)
        }
        
        self.messages = fullList
        self.hasMore = hasMore
        self.nextOffset = nextOffset
        if self.pendingOptimisticMessageId == nil {
            self.knownServerMessageIds = Set(fullList.map(\.id))
        }
    }
    
    private func mergeIncomingMessages(_ incoming: [ChatMessage]) {
        guard !incoming.isEmpty else { return }
        
        // 1. Check if server has incorporated the pending optimistic user message
        let currentOptId = self.pendingOptimisticMessageId
        var optimisticMessage: ChatMessage? = nil
        if let optId = currentOptId {
            optimisticMessage = messages.first(where: { $0.id == optId })
            // Server only incorporates the new turn if incoming contains a user message with a NEW ID not known before sending, or matching text
            let serverHasNewUserMsg = incoming.contains(where: {
                ($0.sender == .user && !knownServerMessageIds.contains($0.id)) ||
                ($0.sender == .user && optimisticMessage != nil && $0.content == optimisticMessage?.content)
            })
            if serverHasNewUserMsg {
                // Server now has incorporated the new turn; clear optimistic tracker
                self.pendingOptimisticMessageId = nil
                optimisticMessage = nil
            }
        }
        
        // 2. Filter out any local optimistic message before merging with server data
        var base = messages.filter { msg in
            if let optId = currentOptId, msg.id == optId {
                return false
            }
            if msg.id.hasPrefix("optimistic-") {
                return false
            }
            return true
        }
        
        if base.isEmpty {
            base = incoming
        } else {
            // Find overlap between incoming and base
            var firstBaseMatchIdx: Int? = nil
            var incomingMatchIdxForFirstBaseMatch: Int? = nil
            
            for (baseIdx, baseMsg) in base.enumerated() {
                if let incIdx = incoming.firstIndex(where: { $0.id == baseMsg.id }) {
                    firstBaseMatchIdx = baseIdx
                    incomingMatchIdxForFirstBaseMatch = incIdx
                    break
                }
            }
            
            if let bIdx = firstBaseMatchIdx, let iIdx = incomingMatchIdxForFirstBaseMatch {
                let prefix = Array(base[0..<bIdx])
                let incomingPrefix = Array(incoming[0..<iIdx])
                let incomingTail = Array(incoming[iIdx...])
                base = prefix + incomingPrefix + incomingTail
            } else {
                // No overlapping ID found. Determine ordering based on step indexes
                let baseStep = base.compactMap { extractStepIndex(from: $0.id) }.first
                let incomingStep = incoming.compactMap { extractStepIndex(from: $0.id) }.first
                
                if let bStep = baseStep, let iStep = incomingStep, iStep < bStep {
                    // incoming contains earlier steps than base
                    base = incoming + base
                } else {
                    // incoming contains later steps than base
                    base = base + incoming
                }
            }
        }
        
        // 3. If server has NOT yet acknowledged the user message, KEEP IT AT THE END!
        if let opt = optimisticMessage {
            base.append(opt)
        }
        
        // 4. Sanitize ordering in case of anomalies
        base = sanitizeMessageOrder(base)
        
        self.messages = base
        if self.pendingOptimisticMessageId == nil {
            self.knownServerMessageIds = Set(base.map(\.id))
        }
    }
    
    // MARK: - Queued Messages Synchronization
    
    private func syncQueuedMessages(serverQueue: [QueuedMessageItem]?) {
        let now = Date()
        // 1. Expire stale optimistic items older than 15 seconds
        pendingOptimisticQueueItems.removeAll(where: { now.timeIntervalSince($0.createdAt) > 15.0 })
        
        // 2. Identify recent user messages in the active conversation
        let recentUserMessages = self.messages
            .filter { $0.sender == .user }
            .suffix(15)
            .map { $0.content.trimmingCharacters(in: .whitespacesAndNewlines) }
        let recentUserMessageSet = Set(recentUserMessages)
        
        // 3. Clear optimistic items if server has incorporated them OR if their text has already entered the conversation
        pendingOptimisticQueueItems.removeAll { opt in
            let trimmed = opt.text.trimmingCharacters(in: .whitespacesAndNewlines)
            let inServer = serverQueue?.contains(where: { $0.text.trimmingCharacters(in: .whitespacesAndNewlines) == trimmed }) == true
            let inChat = recentUserMessageSet.contains(trimmed)
            return inServer || inChat
        }
        
        // 4. Compute base queue from server if provided, otherwise filter existing queue
        var baseQueue: [QueuedMessageItem]
        if let sq = serverQueue {
            baseQueue = sq.filter { sItem in
                !recentUserMessageSet.contains(sItem.text.trimmingCharacters(in: .whitespacesAndNewlines))
            }
        } else {
            baseQueue = self.queuedMessages.filter { qm in
                !qm.id.hasPrefix("queue-") && !recentUserMessageSet.contains(qm.text.trimmingCharacters(in: .whitespacesAndNewlines))
            }
        }
        
        // 5. Append unconfirmed optimistic items (not yet in server queue and not yet entered chat)
        let remainingOptItems = pendingOptimisticQueueItems.compactMap { opt -> QueuedMessageItem? in
            let trimmed = opt.text.trimmingCharacters(in: .whitespacesAndNewlines)
            if baseQueue.contains(where: { $0.text.trimmingCharacters(in: .whitespacesAndNewlines) == trimmed }) {
                return nil
            }
            if recentUserMessageSet.contains(trimmed) {
                return nil
            }
            return QueuedMessageItem(id: opt.id, text: opt.text)
        }
        
        let newQueue = baseQueue + remainingOptItems
        if newQueue != self.queuedMessages {
            withAnimation(.spring(response: 0.35, dampingFraction: 0.8)) {
                self.queuedMessages = newQueue
            }
        }
    }
    
    private func updateCascadeConfigRawModel(_ modelEnum: String, modelName: String) {
        guard let raw = cascadeConfigRaw, let data = raw.data(using: .utf8) else { return }
        guard var json = try? JSONSerialization.jsonObject(with: data) as? [String: Any] else { return }
        var plannerConfig = json["plannerConfig"] as? [String: Any] ?? [:]
        plannerConfig["planModel"] = modelEnum
        plannerConfig["requestedModel"] = ["model": modelEnum]
        plannerConfig["modelName"] = modelName
        json["plannerConfig"] = plannerConfig
        
        var checkpointConfig = json["checkpointConfig"] as? [String: Any] ?? [:]
        let isClaude = modelEnum.lowercased().contains("claude") ||
            modelName.lowercased().contains("claude") ||
            modelEnum == "MODEL_PLACEHOLDER_M26"
        
        if isClaude {
            checkpointConfig["maxTokenLimit"] = 160000
            checkpointConfig["tokenThreshold"] = 50000
            checkpointConfig["isSync"] = false
            checkpointConfig["useLastPlannerModel"] = false
        } else {
            if let limit = checkpointConfig["maxTokenLimit"] as? Int, limit <= 160000 {
                checkpointConfig["maxTokenLimit"] = 256000
            }
            if let thresh = checkpointConfig["tokenThreshold"] as? Int, thresh <= 50000 {
                checkpointConfig["tokenThreshold"] = 140000
            }
            checkpointConfig["isSync"] = true
            checkpointConfig["useLastPlannerModel"] = true
        }
        json["checkpointConfig"] = checkpointConfig
        
        if let patchedData = try? JSONSerialization.data(withJSONObject: json),
           let patchedStr = String(data: patchedData, encoding: .utf8) {
            self.cascadeConfigRaw = patchedStr
        }
    }
    
    @MainActor
    public func toggleModel() {
        UIImpactFeedbackGenerator(style: .medium).impactOccurred()
        hasUserManuallySelectedModel = true
        if isClaudeActive {
            activeModel = "gemini-3.8-flash-high"
        } else {
            activeModel = "claude-opus-4-6-thinking"
        }
        updateCascadeConfigRawModel(activeModelEnum, modelName: activeModel)
        guard let url = settings.serverURL else { return }
        let cid = self.cascadeId
        let currentEnum = self.activeModelEnum
        Task { [weak self] in
            guard let self else { return }
            do {
                try await self.apiClient.switchModel(to: currentEnum, cascadeId: cid.isEmpty ? nil : cid, baseURL: url)
            } catch {
                print("⚠️ Failed to switch model upstream: \(error)")
            }
        }
    }
    
    @discardableResult
    @MainActor
    public func sendMessage(text customText: String? = nil, images: [Data]? = nil) async -> Bool {
        guard !isSending else { return false }
        let text = (customText ?? inputText).trimmingCharacters(in: .whitespacesAndNewlines)
        let hasImages = (images != nil && !images!.isEmpty)
        guard (!text.isEmpty || hasImages), let url = settings.serverURL else { return false }
        
        hasUserManuallySelectedModel = true
        updateCascadeConfigRawModel(activeModelEnum, modelName: activeModel)
        
        isSending = true
        defer { isSending = false }
        
        // If agent is currently running and session already exists, queue follow-up message!
        if (self.isRunning || self.isAwaitingResponse) && !self.cascadeId.isEmpty {
            UIImpactFeedbackGenerator(style: .medium).impactOccurred()
            let queueItem = QueuedMessageItem(id: "queue-\(UUID().uuidString)", text: text)
            self.pendingOptimisticQueueItems.append(PendingOptimisticQueueItem(id: queueItem.id, text: text, createdAt: Date()))
            withAnimation(.spring(response: 0.35, dampingFraction: 0.8)) {
                self.queuedMessages.append(queueItem)
            }
            if customText == nil {
                self.inputText = ""
                cacheManager.clearDraft(key: draftKey)
            }
            
            // Persist to local cache immediately
            let toCache = self.messages.filter { $0.id != self.pendingOptimisticMessageId && !$0.id.hasPrefix("optimistic-") }
            cacheManager.saveSession(CachedChatSession(
                cascadeId: cascadeId,
                status: isRunning ? "CASCADE_RUN_STATUS_RUNNING" : "CASCADE_RUN_STATUS_DONE",
                duration: self.duration,
                stepCount: self.stepCount,
                totalTools: self.totalTools,
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
            
            // Dispatch to server with deliveryStrategy = 2 (WHEN_IDLE)
            let queueClientMsgId = UUID().uuidString
            Task { [weak self] in
                guard let self else { return }
                do {
                    try await self.apiClient.sendMessage(
                        cascadeId: self.cascadeId,
                        text: text,
                        model: self.activeModelEnum,
                        images: images,
                        deliveryStrategy: 2,
                        cascadeConfigRaw: self.cascadeConfigRaw,
                        clientMessageId: queueClientMsgId,
                        baseURL: url
                    )
                } catch {
                    print("⚠️ Failed to deliver queued message upstream: \(error)")
                }
            }
            return true
        }
        
        // Haptic feedback
        UIImpactFeedbackGenerator(style: .medium).impactOccurred()
        
        // Snapshot known server message IDs before sending (excluding any optimistic items)
        self.knownServerMessageIds = Set(messages.filter { $0.id != pendingOptimisticMessageId && !$0.id.hasPrefix("optimistic-") }.map(\.id))
        
        // If this was an uninitiated session, clear the flag upon sending first message
        if self.isNewConversation {
            self.isNewConversation = false
        }
        
        // Optimistic update with unique client message id
        let clientMessageId = UUID().uuidString
        let optId = "optimistic-\(clientMessageId)"
        messages.append(ChatMessage(id: optId, sender: .user, content: text, imageDataList: images ?? []))
        self.pendingOptimisticMessageId = optId
        self.isAwaitingResponse = true
        self.awaitingResponseSince = Date()
        self.isRunning = true
        self.canProceed = false
        self.hasError = false
        self.trajectoryErrorMessage = nil
        inputText = ""
        cacheManager.clearDraft(key: draftKey)
        errorMessage = nil
        
        if !cascadeId.isEmpty {
            if settings.enableLiveActivities {
                activityManager.startActivity(title: currentTitle, cascadeId: cascadeId)
            }
            
            // Ensure WebSocket stream is connected for immediate streaming
            connectStream()
            
            // If stream is not actively connected, use fallback polling
            if streamClient.status != .connected {
                startPollingFallback()
            }
        }
        
        do {
            if cascadeId.isEmpty, let project = draftProject ?? draftSession?.project {
                let pid = project.rawId ?? (project.id != project.uri ? project.id : nil)
                let initialPrompt = (images == nil || images!.isEmpty) ? text : ""
                let newCascadeId = try await apiClient.createCascade(
                    workspaceUri: project.uri,
                    prompt: initialPrompt,
                    model: activeModelEnum,
                    projectId: pid,
                    clientMessageId: clientMessageId,
                    baseURL: url
                )
                if let dSession = draftSession {
                    self.cacheManager.deleteLocalDraftSession(id: dSession.id)
                    self.cacheManager.clearDraft(key: dSession.id)
                    self.cacheManager.clearDraftImages(key: dSession.id)
                }
                self.cacheManager.clearDraft(key: "draft_project_\(project.id)")
                self.cacheManager.clearDraftImages(key: "draft_project_\(project.id)")
                self.cacheManager.clearDraft(key: newCascadeId)
                self.cacheManager.clearDraftImages(key: newCascadeId)
                self.cascadeId = newCascadeId
                self.draftSession = nil
                self.draftProject = nil
                self.isNewConversation = false
                
                // Immediately register new conversation item in cache
                let newConv = ConversationItem(
                    id: newCascadeId,
                    title: project.name,
                    status: .running,
                    stepCount: 1,
                    workspaceName: project.name,
                    lastModified: Date()
                )
                self.cacheManager.upsertConversation(newConv)
                
                if settings.enableLiveActivities {
                    activityManager.startActivity(title: currentTitle, cascadeId: newCascadeId)
                }
                
                // Ensure WebSocket stream is connected for immediate streaming
                connectStream()
                
                // If stream is not actively connected, use fallback polling
                if streamClient.status != .connected {
                    startPollingFallback()
                }
                
                if let imgs = images, !imgs.isEmpty {
                    try await apiClient.sendMessage(
                        cascadeId: newCascadeId,
                        text: text,
                        model: activeModelEnum,
                        images: imgs,
                        cascadeConfigRaw: cascadeConfigRaw,
                        clientMessageId: UUID().uuidString,
                        baseURL: url
                    )
                }
                
                // Allow upstream 250ms to register task and update state before first eager sync
                try? await Task.sleep(nanoseconds: 250_000_000)
                await self.loadMessages(isBackgroundPoll: true)
                await self.checkAndRefreshTitle()
            } else {
                try await apiClient.sendMessage(
                    cascadeId: cascadeId,
                    text: text,
                    model: activeModelEnum,
                    images: images,
                    cascadeConfigRaw: cascadeConfigRaw,
                    clientMessageId: clientMessageId,
                    baseURL: url
                )
                // Allow upstream 250ms to register task and update state before first eager sync
                try? await Task.sleep(nanoseconds: 250_000_000)
                await self.loadMessages(isBackgroundPoll: true)
                self.cacheManager.clearDraftImages(key: cascadeId)
            }
            return true
        } catch {
            print("❌ sendMessage error: \(error)")
            let errorDesc = error.localizedDescription
            errorMessage = errorDesc
            
            let isTimeoutOrDispatched = errorDesc.contains("超时") ||
                                       errorDesc.contains("timed out") ||
                                       errorDesc.contains("送达服务器")
            
            if isTimeoutOrDispatched {
                // Keep the optimistic message in the chat list, but stop active loading spinners so user can see it
                self.isAwaitingResponse = false
                self.awaitingResponseSince = nil
                self.isRunning = false
                // Do NOT restore inputText: keep it empty so user won't duplicate send!
                // Trigger background refresh after a short delay to check if server actually executed it
                Task { [weak self] in
                    try? await Task.sleep(nanoseconds: 2_000_000_000)
                    guard let self else { return }
                    await self.loadMessages(isBackgroundPoll: true)
                }
            } else {
                isRunning = false
                isAwaitingResponse = false
                awaitingResponseSince = nil
                if let optId = pendingOptimisticMessageId {
                    messages.removeAll(where: { $0.id == optId })
                    self.pendingOptimisticMessageId = nil
                }
                // Restore text so user does not lose their input on hard network failure
                inputText = text
                // If session was not yet created, preserve isNewConversation so the user stays in draft mode
                if cascadeId.isEmpty && draftProject != nil {
                    isNewConversation = true
                }
            }
            stopPollingFallback()
            return false
        }
    }
    
    // MARK: - Queued Messages Actions
    
    @MainActor
    public func sendQueuedMessageNow(item: QueuedMessageItem) async {
        guard let url = settings.serverURL, !cascadeId.isEmpty else { return }
        UIImpactFeedbackGenerator(style: .medium).impactOccurred()
        
        self.pendingOptimisticQueueItems.removeAll(where: { $0.id == item.id || $0.text == item.text })
        withAnimation(.spring(response: 0.35, dampingFraction: 0.8)) {
            self.queuedMessages.removeAll(where: { $0.id == item.id })
        }
        
        // Remove from upstream queue asynchronously
        Task { [weak self] in
            guard let self else { return }
            _ = try? await self.apiClient.deleteAgentMessage(messageId: item.id, cascadeId: self.cascadeId, baseURL: url)
        }
        
        // Dispatch with deliveryStrategy = 1 (NEXT_INVOCATION)
        do {
            try await apiClient.sendMessage(
                cascadeId: cascadeId,
                text: item.text,
                model: activeModelEnum,
                deliveryStrategy: 1,
                cascadeConfigRaw: cascadeConfigRaw,
                clientMessageId: UUID().uuidString,
                baseURL: url
            )
        } catch {
            errorMessage = "发送失败: \(error.localizedDescription)"
        }
    }
    
    @MainActor
    public func editQueuedMessage(item: QueuedMessageItem) {
        UIImpactFeedbackGenerator(style: .light).impactOccurred()
        self.pendingOptimisticQueueItems.removeAll(where: { $0.id == item.id || $0.text == item.text })
        withAnimation(.spring(response: 0.35, dampingFraction: 0.8)) {
            self.queuedMessages.removeAll(where: { $0.id == item.id })
        }
        
        if let url = settings.serverURL, !cascadeId.isEmpty {
            Task { [weak self] in
                guard let self else { return }
                _ = try? await self.apiClient.deleteAgentMessage(messageId: item.id, cascadeId: self.cascadeId, baseURL: url)
            }
        }
        
        self.inputText = item.text
    }
    
    @MainActor
    public func deleteQueuedMessage(item: QueuedMessageItem) {
        UIImpactFeedbackGenerator(style: .light).impactOccurred()
        self.pendingOptimisticQueueItems.removeAll(where: { $0.id == item.id || $0.text == item.text })
        withAnimation(.spring(response: 0.35, dampingFraction: 0.8)) {
            self.queuedMessages.removeAll(where: { $0.id == item.id })
        }
        
        if let url = settings.serverURL, !cascadeId.isEmpty {
            Task { [weak self] in
                guard let self else { return }
                _ = try? await self.apiClient.deleteAgentMessage(messageId: item.id, cascadeId: self.cascadeId, baseURL: url)
            }
        }
    }
    
    @MainActor
    public func stopTask(_ task: RunningTaskItem) async {
        guard let url = settings.serverURL, !self.cascadeId.isEmpty else { return }
        UIImpactFeedbackGenerator(style: .medium).impactOccurred()
        
        withAnimation(.spring(response: 0.35, dampingFraction: 0.8)) {
            self.runningTasks.removeAll(where: { $0.id == task.id && $0.stepIndex == task.stepIndex })
        }
        
        do {
            try await apiClient.stopTask(
                cascadeId: self.cascadeId,
                stepIndex: task.stepIndex,
                taskId: task.id,
                baseURL: url
            )
        } catch {
            print("[ChatViewModel] stopTask failed: \(error)")
        }
    }
    
    @MainActor
    public func proceedArtifact() async {
        guard canProceed, let artifactUri = proceedArtifactUri, let url = settings.serverURL else { return }
        
        UIImpactFeedbackGenerator(style: .medium).impactOccurred()
        
        canProceed = false
        proceedArtifactUri = nil
        hasUserManuallySelectedModel = true
        updateCascadeConfigRawModel(activeModelEnum, modelName: activeModel)
        self.isAwaitingResponse = true
        self.awaitingResponseSince = Date()
        self.isRunning = true
        errorMessage = nil
        cacheManager.updateConversationStatus(cascadeId: cascadeId, status: .running)
        
        if settings.enableLiveActivities {
            activityManager.startActivity(title: currentTitle, cascadeId: cascadeId)
        }
        
        connectStream()
        if streamClient.status != .connected {
            startPollingFallback()
        }
        
        do {
            try await apiClient.proceedArtifact(
                cascadeId: cascadeId,
                artifactUri: artifactUri,
                model: activeModelEnum,
                cascadeConfigRaw: cascadeConfigRaw,
                baseURL: url
            )
            try? await Task.sleep(nanoseconds: 250_000_000)
            await self.loadMessages(isBackgroundPoll: true)
        } catch {
            print("❌ proceedArtifact error: \(error)")
            errorMessage = "确认方案失败: \(error.localizedDescription)"
            isRunning = false
            isAwaitingResponse = false
            awaitingResponseSince = nil
            self.canProceed = true
            self.proceedArtifactUri = artifactUri
            stopPollingFallback()
        }
    }
    
    @MainActor
    public func openMarkdownViewer(uri: String, title: String? = nil) {
        let cleanURI = uri.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !cleanURI.isEmpty else { return }
        
        let unescapedURI = cleanURI.removingPercentEncoding ?? cleanURI
        let rawFileName = (unescapedURI as NSString).lastPathComponent
        let fileName = rawFileName.removingPercentEncoding ?? rawFileName
        let isWalkthrough = unescapedURI.lowercased().contains("walkthrough") ||
                            (title?.lowercased().contains("walkthrough") == true)
        let isPlan = !isWalkthrough && (
            unescapedURI.lowercased().contains("implementation_plan") ||
            (title?.lowercased().contains("implementation_plan") == true)
        )
        
        let resolvedTitle: String = {
            if let t = title?.removingPercentEncoding ?? title, !t.isEmpty { return t }
            if isWalkthrough { return "Walkthrough" }
            if isPlan { return "Implementation Plan" }
            if !fileName.isEmpty && fileName != "/" { return fileName }
            return "文档详情"
        }()
        
        let isProceedActive = self.canProceed && isPlan
        
        // Prefer proceedArtifactUri only if it's an implementation_plan
        let targetURI: String = {
            if isPlan, let pUri = self.proceedArtifactUri, !pUri.isEmpty {
                return pUri
            }
            return cleanURI
        }()
        
        let viewer = MarkdownFileViewerData(
            id: targetURI + "_\(Date().timeIntervalSince1970)",
            title: resolvedTitle,
            uri: targetURI,
            content: "",
            summary: nil,
            isLoading: true,
            errorMessage: nil,
            canProceed: isProceedActive
        )
        self.viewingMarkdownFile = viewer
        
        Task {
            guard let url = settings.serverURL else {
                if self.viewingMarkdownFile?.id == viewer.id {
                    self.viewingMarkdownFile?.isLoading = false
                    self.viewingMarkdownFile?.errorMessage = "未连接到网关服务器"
                }
                return
            }
            do {
                let resp = try await apiClient.fetchFileContent(
                    uri: targetURI,
                    cascadeId: self.cascadeId,
                    baseURL: url
                )
                if self.viewingMarkdownFile?.id == viewer.id {
                    self.viewingMarkdownFile?.content = resp.content
                    self.viewingMarkdownFile?.summary = resp.summary
                    self.viewingMarkdownFile?.isLoading = false
                    if !isWalkthrough && !isPlan && !resp.filename.isEmpty {
                        self.viewingMarkdownFile?.title = resp.filename
                    }
                    if resp.requestFeedback == true && self.canProceed {
                        self.viewingMarkdownFile?.canProceed = true
                    }
                }
            } catch {
                if self.viewingMarkdownFile?.id == viewer.id {
                    self.viewingMarkdownFile?.isLoading = false
                    self.viewingMarkdownFile?.errorMessage = "加载文档失败: \(error.localizedDescription)"
                }
            }
        }
    }
    
    @MainActor
    public func closeMarkdownViewer() {
        self.viewingMarkdownFile = nil
    }
    
    @MainActor
    public func proceedFromViewer() {
        self.viewingMarkdownFile = nil
        Task {
            await self.proceedArtifact()
        }
    }
    
    @MainActor
    public func downloadAndPreviewDocument(uri: String, fileName: String, isHTML: Bool) {
        documentDownloadTask?.cancel()
        
        self.isDownloadingDocument = true
        self.downloadingDocumentName = fileName
        self.downloadProgress = 0.0
        self.downloadBytesWritten = 0
        self.downloadBytesTotal = 0
        
        documentDownloadTask = Task {
            guard let url = settings.serverURL else {
                self.isDownloadingDocument = false
                self.errorMessage = "未连接到网关服务器"
                return
            }
            do {
                let (localURL, resolvedName) = try await apiClient.downloadFile(
                    uri: uri,
                    cascadeId: self.cascadeId,
                    baseURL: url
                ) { [weak self] progress, written, total in
                    Task { @MainActor [weak self] in
                        guard let self = self, self.isDownloadingDocument else { return }
                        self.downloadProgress = progress
                        self.downloadBytesWritten = written
                        self.downloadBytesTotal = total
                    }
                }
                
                guard !Task.isCancelled else { return }
                
                withAnimation(.easeInOut(duration: 0.15)) {
                    self.isDownloadingDocument = false
                }
                
                if isHTML {
                    self.htmlPreviewTitle = resolvedName
                    self.htmlPreviewURL = localURL
                } else {
                    self.quickLookURL = localURL
                }
            } catch {
                guard !Task.isCancelled else { return }
                self.isDownloadingDocument = false
                self.errorMessage = "下载文档失败: \(error.localizedDescription)"
            }
        }
    }
    
    @MainActor
    public func cancelDocumentDownload() {
        documentDownloadTask?.cancel()
        documentDownloadTask = nil
        withAnimation(.easeInOut(duration: 0.15)) {
            self.isDownloadingDocument = false
        }
    }
    
    @MainActor
    public func closeQuickLook() {
        self.quickLookURL = nil
    }
    
    @MainActor
    public func closeHTMLPreview() {
        self.htmlPreviewURL = nil
    }


    
    @MainActor
    public func checkAndRefreshTitle() async {
        guard !cascadeId.isEmpty, let url = settings.serverURL else { return }
        do {
            if let title = try await apiClient.fetchConversationTitle(cascadeId: cascadeId, baseURL: url),
               !title.isEmpty, title != "未命名会话", title != self.currentTitle {
                withAnimation(.easeInOut(duration: 0.25)) {
                    self.currentTitle = title
                }
                self.cacheManager.updateConversationTitle(cascadeId: self.cascadeId, newTitle: title)
            }
        } catch {
            // Background title poll failure can be silently ignored
        }
    }
    
    @MainActor
    public func cancelTask() async {
        guard !cascadeId.isEmpty, let url = settings.serverURL else { return }
        
        UIImpactFeedbackGenerator(style: .heavy).impactOccurred()
        isAwaitingResponse = false
        awaitingResponseSince = nil
        isRunning = false
        canProceed = false
        stopPollingFallback()
        
        do {
            try await apiClient.cancelTask(cascadeId: cascadeId, baseURL: url)
            if settings.enableLiveActivities {
                activityManager.endActivity(finalStatus: "CANCELLED")
            }
            await loadMessages(isBackgroundPoll: true)
        } catch {
            errorMessage = "取消任务失败: \(error.localizedDescription)"
        }
    }
    
    // MARK: - WebSocket Stream Integration
    
    private func setupStreamClient() {
        streamClient.onUpdate = { [weak self] payload in
            Task { @MainActor [weak self] in
                guard let self else { return }
                self.handleStreamPayload(payload)
            }
        }
        
        streamClient.onStatusChange = { [weak self] status in
            Task { @MainActor [weak self] in
                guard let self else { return }
                self.handleStreamStatusChange(status)
            }
        }
        
        NotificationCenter.default.addObserver(
            forName: .networkRoutingPreferenceChanged,
            object: nil,
            queue: .main
        ) { [weak self] _ in
            Task { @MainActor [weak self] in
                guard let self = self, !self.cascadeId.isEmpty else { return }
                print("[ChatViewModel] Network routing preference changed, reconnecting stream...")
                self.connectStream(force: true)
                await self.loadMessages(isBackgroundPoll: true)
            }
        }
    }
    
    private func handleStreamStatusChange(_ status: StreamConnectionStatus) {
        switch status {
        case .connected:
            stopPollingFallback()
        case .failed, .disconnected:
            let shouldPoll = self.isRunning || self.isAwaitingResponse
            if shouldPoll && pollTask == nil {
                startPollingFallback()
            }
        default:
            break
        }
    }
    
    private func handleStreamPayload(_ payload: StreamUpdatePayload) {
        if let steps = payload.totalSteps {
            self.stepCount = steps
        }
        if let tools = payload.totalTools {
            self.totalTools = tools
        }
        if let dur = payload.duration {
            self.duration = dur
        }
        if let cfg = payload.cascadeConfigRaw, !cfg.isEmpty {
            self.cascadeConfigRaw = cfg
            if self.hasUserManuallySelectedModel {
                self.updateCascadeConfigRawModel(self.activeModelEnum, modelName: self.activeModel)
            }
        }
        if let activeModel = payload.activeModel, !activeModel.isEmpty {
            // Streaming updates occur during execution; do not override user manual model selection
            if !self.hasUserManuallySelectedModel {
                self.syncModel(from: activeModel)
            }
        }
        
        if let title = payload.title?.trimmingCharacters(in: .whitespacesAndNewlines),
           !title.isEmpty, title != "未命名会话" {
            if self.currentTitle != title {
                withAnimation(.easeInOut(duration: 0.25)) {
                    self.currentTitle = title
                }
                self.cacheManager.updateConversationTitle(cascadeId: self.cascadeId, newTitle: title)
            }
        }
        
        if let rawMessages = payload.messages {
            let parsedMessages = rawMessages.map { item -> ChatMessage in
                let sender: ChatMessage.MessageSender = {
                    switch item.type {
                    case "user": return .user
                    case "agent": return .agent
                    case "error": return .error
                    default: return .toolBatch(count: item.toolCount ?? 1, tools: item.toolNames ?? [])
                    }
                }()
                let imgDataList = (item.media ?? []).compactMap { Data(base64Encoded: $0) }
                return ChatMessage(
                    id: item.id,
                    sender: sender,
                    content: item.text,
                    toolCount: item.toolCount ?? 0,
                    toolNames: item.toolNames ?? [],
                    imageDataList: imgDataList,
                    imageUrls: item.imageUrls ?? []
                )
            }
            let hasExpandedHistory = self.messages.count > parsedMessages.count
            if payload.isFullSnapshot == true && !hasExpandedHistory {
                self.applySnapshotMessages(
                    parsedMessages,
                    hasMore: payload.hasMore ?? false,
                    nextOffset: payload.nextOffset ?? 0
                )
            } else {
                self.mergeIncomingMessages(parsedMessages)
                if let hm = payload.hasMore, !self.isLoadingOlder && !hasExpandedHistory {
                    self.hasMore = hm
                    if let no = payload.nextOffset {
                        self.nextOffset = no
                    }
                }
            }
        }
        
        let previouslyRunning = self.isRunning
        let lastUserIdx = self.messages.lastIndex(where: { $0.isUser }) ?? -1
        let latestTurn = lastUserIdx >= 0 ? self.messages.suffix(from: lastUserIdx + 1) : self.messages[...]
        let latestHasErr = latestTurn.contains(where: { $0.isError }) && !(latestTurn.last?.isAgent == true)
        let isStreamError = payload.hasError == true || payload.status == "CASCADE_RUN_STATUS_ERROR" || latestHasErr
        self.hasError = isStreamError
        let statusString: String
        if isStreamError {
            self.trajectoryErrorMessage = payload.errorMessage ?? latestTurn.last(where: { $0.isError })?.content
            self.isRunning = false
            self.isAwaitingResponse = false
            self.awaitingResponseSince = nil
            statusString = "CASCADE_RUN_STATUS_ERROR"
        } else {
            statusString = payload.status ?? (previouslyRunning ? "CASCADE_RUN_STATUS_RUNNING" : "CASCADE_RUN_STATUS_DONE")
            if statusString == "CASCADE_RUN_STATUS_RUNNING" {
                self.isRunning = true
            } else if !self.isAwaitingResponse {
                self.isRunning = false
            }
        }
        
        if let cp = payload.canProceed {
            if !self.isRunning && !self.isAwaitingResponse {
                self.canProceed = cp
                self.proceedArtifactUri = payload.proceedArtifactUri
            } else {
                self.canProceed = false
            }
        }
        
        if self.isAwaitingResponse && !isStreamError {
            if self.pendingOptimisticMessageId != nil {
                // Keep awaiting until server acknowledges the user message
            } else if let lastUserIdx = self.messages.lastIndex(where: { $0.sender == .user }) {
                let subsequent = self.messages.suffix(from: lastUserIdx + 1)
                let hasAgentResponse = subsequent.contains(where: { $0.sender == .agent })
                let hasErrorResponse = subsequent.contains(where: { $0.isError })
                if hasAgentResponse || hasErrorResponse {
                    self.isAwaitingResponse = false
                    self.awaitingResponseSince = nil
                    if statusString != "CASCADE_RUN_STATUS_RUNNING" {
                        self.isRunning = false
                        if let cp = payload.canProceed {
                            self.canProceed = cp
                            self.proceedArtifactUri = payload.proceedArtifactUri
                        }
                    }
                    UIImpactFeedbackGenerator(style: .light).impactOccurred()
                } else if statusString != "CASCADE_RUN_STATUS_RUNNING" && !subsequent.isEmpty {
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
        
        if settings.enableLiveActivities {
            if isRunning && !previouslyRunning {
                activityManager.startActivity(title: currentTitle, cascadeId: cascadeId)
            } else if isRunning {
                let latestAction = self.messages.last?.content ?? "正在执行..."
                activityManager.updateActivity(status: "RUNNING", stepCount: self.stepCount, latestAction: latestAction)
            } else if !isRunning && previouslyRunning {
                activityManager.endActivity(finalStatus: "COMPLETED")
            }
        }
        
        if self.isRunning {
            if let pi = payload.pendingInteraction {
                self.pendingInteraction = pi
            }
        } else {
            self.pendingInteraction = nil
        }
        
        self.syncQueuedMessages(serverQueue: payload.queuedMessages)
        
        if let tasks = payload.runningTasks {
            self.runningTasks = tasks
        } else if !self.isRunning && !self.isAwaitingResponse {
            self.runningTasks = []
        }
        
        let toCache = self.messages.filter { $0.id != self.pendingOptimisticMessageId && !$0.id.hasPrefix("optimistic-") }
        cacheManager.saveSession(CachedChatSession(
            cascadeId: cascadeId,
            status: statusString,
            duration: self.duration,
            stepCount: self.stepCount,
            totalTools: self.totalTools,
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
            } else if self.isRunning {
                return .running
            } else {
                return .idle
            }
        }()
        cacheManager.updateConversationStatus(cascadeId: cascadeId, status: convStatus)
    }
    
    // Submit user decision on a pending interaction
    @MainActor
    public func submitInteraction(optionId: String, writeInText: String? = nil, target: String? = nil) async {
        guard let interaction = pendingInteraction, let url = settings.serverURL else { return }
        
        UIImpactFeedbackGenerator(style: .medium).impactOccurred()
        isSubmittingInteraction = true
        errorMessage = nil
        
        let selectedOpt = interaction.options.first(where: { $0.id == optionId })
        let scope = selectedOpt?.scope ?? 1
        let isDeny = selectedOpt?.isDeny ?? (optionId == "5" || optionId == "__write_in__")
        let allow = !isDeny
        let writeIn = writeInText?.trimmingCharacters(in: CharacterSet.whitespacesAndNewlines) ?? ""
        
        do {
            try await apiClient.submitInteraction(
                cascadeId: cascadeId,
                trajectoryId: interaction.trajectoryId,
                stepIndex: interaction.stepIndex,
                type: interaction.type,
                optionId: optionId,
                scope: scope,
                allow: allow,
                writeInResponse: writeIn,
                skipped: false,
                target: target ?? interaction.target,
                baseURL: url
            )
            
            withAnimation(.spring(response: 0.35, dampingFraction: 0.8)) {
                self.pendingInteraction = nil
            }
            self.isSubmittingInteraction = false
            
            try? await Task.sleep(nanoseconds: 250_000_000)
            await self.loadMessages(isBackgroundPoll: true)
        } catch {
            isSubmittingInteraction = false
            errorMessage = "提交失败: \(error.localizedDescription)"
        }
    }
    
    // Skip the current interaction
    @MainActor
    public func skipInteraction() async {
        guard let interaction = pendingInteraction, let url = settings.serverURL else { return }
        
        UIImpactFeedbackGenerator(style: .light).impactOccurred()
        isSubmittingInteraction = true
        errorMessage = nil
        
        do {
            try await apiClient.submitInteraction(
                cascadeId: cascadeId,
                trajectoryId: interaction.trajectoryId,
                stepIndex: interaction.stepIndex,
                type: interaction.type,
                optionId: "",
                scope: 1,
                allow: false,
                writeInResponse: "",
                skipped: true,
                target: interaction.target,
                baseURL: url
            )
            
            withAnimation(.spring(response: 0.35, dampingFraction: 0.8)) {
                self.pendingInteraction = nil
            }
            self.isSubmittingInteraction = false
            
            try? await Task.sleep(nanoseconds: 250_000_000)
            await self.loadMessages(isBackgroundPoll: true)
        } catch {
            isSubmittingInteraction = false
            errorMessage = "跳过失败: \(error.localizedDescription)"
        }
    }
    
    public func connectStream(force: Bool = false) {
        guard !cascadeId.isEmpty, let url = settings.serverURL else { return }
        streamClient.connect(baseURL: url, cascadeId: cascadeId, force: force)
    }
    
    private var lastResumeTime: Date = .distantPast
    
    @MainActor
    public func resumeActiveSession() async {
        let now = Date()
        guard now.timeIntervalSince(lastResumeTime) > 1.0 else { return }
        lastResumeTime = now
        
        guard !cascadeId.isEmpty else { return }
        
        // 1. Force reconnect WebSocket stream to discard stale/suspended connection and receive fresh snapshot
        connectStream(force: true)
        
        // 2. Concurrently fetch latest trajectory over HTTP for instantaneous UI refresh
        await loadMessages(isBackgroundPoll: true)
        await checkAndRefreshTitle()
    }
    
    @MainActor
    public func handleAppBackground() {
        disconnectStream()
    }
    
    public func disconnectStream() {
        streamClient.disconnect(intentional: true)
        stopPollingFallback()
    }
    
    public func stopPolling() {
        disconnectStream()
    }
    
    private func startPollingFallback() {
        guard pollTask == nil else { return }
        pollTask = Task { [weak self] in
            while !Task.isCancelled {
                try? await Task.sleep(nanoseconds: 1_500_000_000)
                guard let self else { break }
                await self.loadMessages(isBackgroundPoll: true)
            }
        }
    }
    
    private func stopPollingFallback() {
        pollTask?.cancel()
        pollTask = nil
    }
}
