import Foundation
import Observation
import UIKit
import SwiftUI

@Observable
@MainActor
public final class ChatViewModel {
    public var cascadeId: String
    public let initialTitle: String
    public var currentTitle: String
    public var isNewConversation: Bool
    public var draftProject: ProjectItem?
    
    public var messages: [ChatMessage] = []
    public var inputText: String = ""
    public var isLoading: Bool = false
    public var isRunning: Bool = false
    public var stepCount: Int = 0
    public var totalTools: Int = 0
    public var duration: String = "0秒"
    public var hasMore: Bool = false
    public var nextOffset: Int = 0
    public var isLoadingOlder: Bool = false
    public var errorMessage: String? = nil
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
        settings.activeModel
    }
    
    public var isClaudeActive: Bool {
        settings.isClaudeActive
    }
    
    private var awaitingResponseSince: Date? = nil
    private var pendingOptimisticMessageId: String? = nil
    private var knownServerMessageIds: Set<String> = []
    private var pollTask: Task<Void, Never>?
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
        self.apiClient = apiClient ?? .shared
        self.settings = settings ?? .shared
        self.activityManager = ActivityManager.shared
        self.cacheManager = cacheManager ?? .shared
        self.streamClient = StreamWebSocketClient()
        
        self.setupStreamClient()
        
        // Instant restore from local cache
        if let cached = self.cacheManager.loadSession(for: cascadeId) {
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
            self.queuedMessages = cached.queuedMessages ?? []
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
        self.apiClient = apiClient ?? .shared
        self.settings = settings ?? .shared
        self.activityManager = ActivityManager.shared
        self.cacheManager = cacheManager ?? .shared
        self.streamClient = StreamWebSocketClient()
        
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
            self.queuedMessages = cached.queuedMessages ?? []
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
            if result.status == "CASCADE_RUN_STATUS_RUNNING" {
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
            
            if !result.queuedMessages.isEmpty {
                self.queuedMessages = result.queuedMessages
            } else if !self.isRunning && !self.isAwaitingResponse {
                self.queuedMessages = []
            }
            
            self.runningTasks = result.runningTasks
            
            // Persist latest state to cache (excluding temporary optimistic message)
            let toCache = self.messages.filter { $0.id != self.pendingOptimisticMessageId && !$0.id.hasPrefix("optimistic-") }
            cacheManager.saveSession(CachedChatSession(
                cascadeId: cascadeId,
                status: result.status,
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
                if self.canProceed || self.pendingInteraction != nil {
                    return .action
                } else if self.isRunning {
                    return .running
                } else {
                    return .idle
                }
            }()
            cacheManager.updateConversationStatus(cascadeId: cascadeId, status: convStatus)
            
            // If awaiting response, check if agent has completed response
            if self.isAwaitingResponse {
                // If optimistic user message has not been incorporated by server yet, keep awaiting!
                if self.pendingOptimisticMessageId != nil {
                    // Server has not acknowledged user message yet; keep awaiting
                } else if let lastUserIdx = self.messages.lastIndex(where: { $0.sender == .user }) {
                    // Check if an agent response exists strictly AFTER the latest user message
                    let subsequent = self.messages.suffix(from: lastUserIdx + 1)
                    let hasAgentResponse = subsequent.contains(where: { $0.sender == .agent })
                    
                    if hasAgentResponse {
                        self.isAwaitingResponse = false
                        self.awaitingResponseSince = nil
                        if result.status != "CASCADE_RUN_STATUS_RUNNING" {
                            self.isRunning = false
                            self.canProceed = result.canProceed
                            self.proceedArtifactUri = result.proceedArtifactUri
                            self.pendingInteraction = nil
                        }
                        UIImpactFeedbackGenerator(style: .light).impactOccurred()
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
    
    @MainActor
    public func toggleModel() async {
        settings.toggleActiveModel()
        UIImpactFeedbackGenerator(style: .medium).impactOccurred()
        guard let url = settings.serverURL else { return }
        do {
            try await apiClient.switchModel(to: settings.activeModelEnum, baseURL: url)
        } catch {
            print("⚠️ Failed to switch model upstream: \(error)")
        }
    }
    
    @discardableResult
    @MainActor
    public func sendMessage(text customText: String? = nil, images: [Data]? = nil) async -> Bool {
        guard !isSending else { return false }
        let text = (customText ?? inputText).trimmingCharacters(in: .whitespacesAndNewlines)
        let hasImages = (images != nil && !images!.isEmpty)
        guard (!text.isEmpty || hasImages), let url = settings.serverURL else { return false }
        
        isSending = true
        defer { isSending = false }
        
        // If agent is currently running and session already exists, queue follow-up message!
        if (self.isRunning || self.isAwaitingResponse) && !self.cascadeId.isEmpty {
            UIImpactFeedbackGenerator(style: .medium).impactOccurred()
            let queueItem = QueuedMessageItem(id: "queue-\(UUID().uuidString)", text: text)
            withAnimation(.spring(response: 0.35, dampingFraction: 0.8)) {
                self.queuedMessages.append(queueItem)
            }
            if customText == nil {
                self.inputText = ""
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
        inputText = ""
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
            if cascadeId.isEmpty, let project = draftProject {
                let pid = project.rawId ?? (project.id != project.uri ? project.id : nil)
                let initialPrompt = (images == nil || images!.isEmpty) ? text : ""
                let newCascadeId = try await apiClient.createCascade(
                    workspaceUri: project.uri,
                    prompt: initialPrompt,
                    model: settings.activeModelEnum,
                    projectId: pid,
                    clientMessageId: clientMessageId,
                    baseURL: url
                )
                self.cascadeId = newCascadeId
                self.draftProject = nil
                
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
                    images: images,
                    cascadeConfigRaw: cascadeConfigRaw,
                    clientMessageId: clientMessageId,
                    baseURL: url
                )
                // Allow upstream 250ms to register task and update state before first eager sync
                try? await Task.sleep(nanoseconds: 250_000_000)
                await self.loadMessages(isBackgroundPoll: true)
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
            guard let self else { return }
            self.handleStreamPayload(payload)
        }
        
        streamClient.onStatusChange = { [weak self] status in
            guard let self else { return }
            self.handleStreamStatusChange(status)
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
        let statusString = payload.status ?? (previouslyRunning ? "CASCADE_RUN_STATUS_RUNNING" : "CASCADE_RUN_STATUS_DONE")
        if statusString == "CASCADE_RUN_STATUS_RUNNING" {
            self.isRunning = true
        } else if !self.isAwaitingResponse {
            self.isRunning = false
        }
        
        if let cp = payload.canProceed {
            if !self.isRunning && !self.isAwaitingResponse {
                self.canProceed = cp
                self.proceedArtifactUri = payload.proceedArtifactUri
            } else {
                self.canProceed = false
            }
        }
        
        if self.isAwaitingResponse {
            if self.pendingOptimisticMessageId != nil {
                // Keep awaiting until server acknowledges the user message
            } else if let lastUserIdx = self.messages.lastIndex(where: { $0.sender == .user }) {
                let subsequent = self.messages.suffix(from: lastUserIdx + 1)
                let hasAgentResponse = subsequent.contains(where: { $0.sender == .agent })
                if hasAgentResponse {
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
        
        if let qm = payload.queuedMessages {
            if !qm.isEmpty {
                self.queuedMessages = qm
            } else if !self.isRunning && !self.isAwaitingResponse {
                self.queuedMessages = []
            }
        }
        
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
            if self.canProceed || self.pendingInteraction != nil {
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
