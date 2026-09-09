import Foundation
import Observation
import UIKit

@Observable
@MainActor
public final class ChatViewModel {
    public let cascadeId: String
    public let initialTitle: String
    
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
    
    /// ID of the first message of the latest response turn (e.g., tool batch or agent response following the last user message)
    public var latestTurnStartMessageId: String? {
        guard let lastUserIdx = messages.lastIndex(where: { $0.sender == .user }) else {
            return messages.first(where: { $0.sender != .user })?.id ?? messages.first?.id
        }
        let subsequent = messages.suffix(from: lastUserIdx + 1)
        return subsequent.first?.id
    }
    
    private var awaitingResponseSince: Date? = nil
    private var pendingOptimisticMessageId: String? = nil
    private var pollTask: Task<Void, Never>?
    private let apiClient: APIClient
    private let settings: AppSettings
    private let activityManager: ActivityManager
    private let cacheManager: CacheManager
    
    public init(
        cascadeId: String,
        initialTitle: String,
        apiClient: APIClient? = nil,
        settings: AppSettings? = nil,
        cacheManager: CacheManager? = nil
    ) {
        self.cascadeId = cascadeId
        self.initialTitle = initialTitle
        self.apiClient = apiClient ?? .shared
        self.settings = settings ?? .shared
        self.activityManager = ActivityManager.shared
        self.cacheManager = cacheManager ?? .shared
        
        // Instant restore from local cache
        if let cached = self.cacheManager.loadSession(for: cascadeId) {
            self.messages = cached.messages
            self.duration = cached.duration
            self.stepCount = cached.stepCount
            self.totalTools = cached.totalTools
            self.hasMore = cached.hasMore
            self.nextOffset = cached.nextOffset
            self.isRunning = (cached.status == "CASCADE_RUN_STATUS_RUNNING")
        }
    }
    
    @MainActor
    public func loadMessages(isBackgroundPoll: Bool = false) async {
        // Fallback to cache if messages empty
        if messages.isEmpty, let cached = cacheManager.loadSession(for: cascadeId) {
            self.messages = cached.messages
            self.duration = cached.duration
            self.stepCount = cached.stepCount
            self.totalTools = cached.totalTools
            self.hasMore = cached.hasMore
            self.nextOffset = cached.nextOffset
            self.isRunning = (cached.status == "CASCADE_RUN_STATUS_RUNNING")
        }
        
        guard let url = settings.serverURL else {
            if messages.isEmpty {
                errorMessage = "未配置服务器地址"
            }
            return
        }
        
        if !isBackgroundPoll && messages.isEmpty {
            isLoading = true
        }
        errorMessage = nil
        
        do {
            let (status, parsedMessages, count, toolCount, duration, hasMoreRemaining, nextOff, configRaw) = try await apiClient.fetchMessages(
                cascadeId: cascadeId,
                limit: 10,
                offset: nil,
                baseURL: url
            )
            if let configRaw, !configRaw.isEmpty {
                self.cascadeConfigRaw = configRaw
            }
            
            if (isBackgroundPoll || self.pendingOptimisticMessageId != nil) && !self.messages.isEmpty {
                self.mergeIncomingMessages(parsedMessages)
            } else {
                self.messages = parsedMessages
                self.hasMore = hasMoreRemaining
                self.nextOffset = nextOff
            }
            
            self.stepCount = count
            self.totalTools = toolCount
            self.duration = duration
            self.isLoading = false
            
            // Persist latest state to cache (excluding temporary optimistic message)
            let toCache = self.messages.filter { $0.id != self.pendingOptimisticMessageId }
            cacheManager.saveSession(CachedChatSession(
                cascadeId: cascadeId,
                status: status,
                duration: duration,
                stepCount: count,
                totalTools: toolCount,
                hasMore: self.hasMore,
                nextOffset: self.nextOffset,
                messages: toCache
            ))
            
            let previouslyRunning = self.isRunning
            if status == "CASCADE_RUN_STATUS_RUNNING" {
                self.isRunning = true
            } else if !self.isAwaitingResponse {
                self.isRunning = false
            }
            
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
                        self.scrollToTurnStartTrigger += 1
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
                    activityManager.startActivity(title: initialTitle, cascadeId: cascadeId)
                } else if isRunning {
                    let latestAction = parsedMessages.last?.content ?? "正在执行..."
                    activityManager.updateActivity(status: "RUNNING", stepCount: count, latestAction: latestAction)
                } else if !isRunning && previouslyRunning {
                    activityManager.endActivity(finalStatus: "COMPLETED")
                }
            }
            
            // Manage background polling: poll while running or awaiting agent response
            let shouldPoll = self.isRunning || self.isAwaitingResponse
            if shouldPoll && pollTask == nil {
                startPolling()
            } else if !shouldPoll && pollTask != nil {
                stopPolling()
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
            let (status, olderMessages, count, toolCount, duration, hasMoreRemaining, nextOff, configRaw) = try await apiClient.fetchMessages(
                cascadeId: cascadeId,
                limit: 10,
                offset: nextOffset,
                baseURL: url
            )
            if let configRaw, !configRaw.isEmpty {
                self.cascadeConfigRaw = configRaw
            }
            self.messages = olderMessages + self.messages
            self.hasMore = hasMoreRemaining
            self.nextOffset = nextOff
            self.isLoadingOlder = false
            
            // Persist expanded message stream to cache
            cacheManager.saveSession(CachedChatSession(
                cascadeId: cascadeId,
                status: status,
                duration: duration,
                stepCount: count,
                totalTools: toolCount,
                hasMore: self.hasMore,
                nextOffset: self.nextOffset,
                messages: self.messages
            ))
            
            UIImpactFeedbackGenerator(style: .light).impactOccurred()
        } catch {
            self.isLoadingOlder = false
        }
    }
    
    private func mergeIncomingMessages(_ incoming: [ChatMessage]) {
        guard !incoming.isEmpty else { return }
        
        // 1. Check if server has incorporated the pending optimistic user message
        var optimisticMessage: ChatMessage? = nil
        if let optId = pendingOptimisticMessageId {
            optimisticMessage = messages.first(where: { $0.id == optId })
            let serverHasUserMsg = incoming.contains(where: {
                $0.sender == .user && ($0.content == optimisticMessage?.content || optimisticMessage == nil)
            })
            if serverHasUserMsg {
                // Server now has it; clear optimistic tracker
                self.pendingOptimisticMessageId = nil
                optimisticMessage = nil
            }
        }
        
        // 2. Filter out any local optimistic message before merging with server data
        var base = messages.filter { $0.id != pendingOptimisticMessageId }
        
        if base.isEmpty {
            base = incoming
        } else if let firstIncoming = incoming.first, let matchIdx = base.firstIndex(where: { $0.id == firstIncoming.id }) {
            base = Array(base[0..<matchIdx]) + incoming
        } else {
            var updated = base
            for inc in incoming {
                if let idx = updated.firstIndex(where: { $0.id == inc.id }) {
                    updated[idx] = inc
                } else {
                    updated.append(inc)
                }
            }
            base = updated
        }
        
        // 3. If server has NOT yet acknowledged the user message, KEEP IT AT THE END!
        if let opt = optimisticMessage {
            base.append(opt)
        }
        
        self.messages = base
    }
    
    @MainActor
    public func sendMessage(text customText: String? = nil) async {
        let text = (customText ?? inputText).trimmingCharacters(in: .whitespacesAndNewlines)
        guard !text.isEmpty, let url = settings.serverURL else { return }
        
        // Haptic feedback
        UIImpactFeedbackGenerator(style: .medium).impactOccurred()
        
        // Optimistic update
        let optId = "optimistic-\(UUID().uuidString)"
        messages.append(ChatMessage(id: optId, sender: .user, content: text))
        self.pendingOptimisticMessageId = optId
        self.isAwaitingResponse = true
        self.awaitingResponseSince = Date()
        self.isRunning = true
        inputText = ""
        errorMessage = nil
        
        if settings.enableLiveActivities {
            activityManager.startActivity(title: initialTitle, cascadeId: cascadeId)
        }
        
        // Start active polling immediately
        startPolling()
        
        do {
            try await apiClient.sendMessage(cascadeId: cascadeId, text: text, cascadeConfigRaw: cascadeConfigRaw, baseURL: url)
            // Allow upstream 250ms to register task and update state before first eager poll
            try? await Task.sleep(nanoseconds: 250_000_000)
            await self.loadMessages(isBackgroundPoll: true)
        } catch {
            print("❌ sendMessage error: \(error)")
            errorMessage = error.localizedDescription
            isRunning = false
            isAwaitingResponse = false
            awaitingResponseSince = nil
            if let optId = pendingOptimisticMessageId {
                messages.removeAll(where: { $0.id == optId })
                self.pendingOptimisticMessageId = nil
            }
            // Restore text so user does not lose their input
            inputText = text
            stopPolling()
        }
    }
    
    @MainActor
    public func cancelTask() async {
        guard let url = settings.serverURL else { return }
        
        UIImpactFeedbackGenerator(style: .heavy).impactOccurred()
        isAwaitingResponse = false
        awaitingResponseSince = nil
        isRunning = false
        stopPolling()
        
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
    
    private func startPolling() {
        pollTask?.cancel()
        pollTask = Task { [weak self] in
            while !Task.isCancelled {
                try? await Task.sleep(nanoseconds: 1_000_000_000) // 1 second
                guard let self else { break }
                await self.loadMessages(isBackgroundPoll: true)
            }
        }
    }
    
    public func stopPolling() {
        pollTask?.cancel()
        pollTask = nil
    }
}
