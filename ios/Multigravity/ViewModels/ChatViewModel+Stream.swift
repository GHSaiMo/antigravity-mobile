import Foundation
import Observation
import UIKit
import SwiftUI

extension ChatViewModel {
    // MARK: - WebSocket Stream Integration
    
    func setupStreamClient() {
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
    
    func handleStreamStatusChange(_ status: StreamConnectionStatus) {
        switch status {
        case .connected:
            stopPollingFallback()
        case .failed, .disconnected, .connecting:
            if pollTask == nil {
                startPollingFallback()
            }
        }
    }
    
    func handleStreamPayload(_ payload: StreamUpdatePayload) {
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
            if self.currentTitle != title && !(payload.messages?.isEmpty == true) {
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
                let resolvedImageUrls: [String] = {
                    let rawList = item.imageUrls ?? []
                    guard let baseURL = self.settings.gatewayURL else { return rawList }
                    return rawList.map { self.apiClient.resolveMediaURL($0, baseURL: baseURL) }
                }()
                return ChatMessage(
                    id: item.id,
                    sender: sender,
                    content: item.text,
                    toolCount: item.toolCount ?? 0,
                    toolNames: item.toolNames ?? [],
                    imageDataList: imgDataList,
                    imageUrls: resolvedImageUrls,
                    stepIndex: item.stepIndex ?? (item.id.hasPrefix("step-") ? Int(item.id.dropFirst(5)) : nil)
                )
            }
            let clientMaxStep = self.messages.compactMap { self.extractStepIndex(from: $0) }.max() ?? -1
            let serverMaxStep = parsedMessages.compactMap { self.extractStepIndex(from: $0) }.max() ?? -1
            let isTruncatedOrReverted = serverMaxStep < clientMaxStep || parsedMessages.isEmpty
            let hasExpandedHistory = self.messages.count > parsedMessages.count && !isTruncatedOrReverted
            
            if parsedMessages.isEmpty {
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
            } else if (payload.isFullSnapshot == true || isTruncatedOrReverted) && !hasExpandedHistory {
                self.applySnapshotMessages(
                    parsedMessages,
                    hasMore: payload.hasMore ?? false,
                    nextOffset: payload.nextOffset ?? 0
                )
            } else {
                self.mergeIncomingMessages(parsedMessages)
                if self.messages.contains(where: { self.extractStepIndex(from: $0) == 0 }) {
                    self.hasMore = false
                    self.nextOffset = 0
                } else if let hm = payload.hasMore, !self.isLoadingOlder && !hasExpandedHistory {
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
        let latestEndedInError = latestTurn.last?.isError == true
        
        let statusString: String
        let isStreamError: Bool
        if payload.status == "CASCADE_RUN_STATUS_RUNNING" {
            self.isRunning = true
            self.hasError = false
            self.trajectoryErrorMessage = nil
            isStreamError = false
            statusString = "CASCADE_RUN_STATUS_RUNNING"
        } else {
            isStreamError = payload.hasError == true || payload.status == "CASCADE_RUN_STATUS_ERROR" || latestEndedInError
            self.hasError = isStreamError
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
        }
        
        if let cp = payload.canProceed {
            if !self.isRunning && !self.isAwaitingResponse {
                self.updateProceedState(canProceed: cp, artifactUri: payload.proceedArtifactUri)
            } else {
                self.updateProceedState(canProceed: false, artifactUri: nil)
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
                            self.updateProceedState(canProceed: cp, artifactUri: payload.proceedArtifactUri)
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
        
        
        if self.isRunning {
            if let pi = payload.pendingInteraction {
                self.pendingInteraction = pi
                self.checkAutoApproveInteractionIfNeeded(pi)
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
        
        syncLiveActivity()
        
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
            } else if self.isRunning || !self.runningTasks.isEmpty {
                return .running
            } else {
                return .idle
            }
        }()
        self.initialConversationStatus = convStatus
        cacheManager.updateConversationStatus(cascadeId: cascadeId, status: convStatus)
    }
    
    // Submit user decision on a pending interaction
    @MainActor
    public func submitInteraction(
        optionId: String,
        writeInText: String? = nil,
        target: String? = nil,
        questionResponses: [QuestionResponse]? = nil
    ) async {
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
                questionResponses: questionResponses,
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
    public func skipInteraction(questionResponses: [QuestionResponse]? = nil) async {
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
                questionResponses: questionResponses,
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
    
    // Auto-approve permission requests with "Yes, and always allow" (Scope 4) if enabled in settings
    @MainActor
    func checkAutoApproveInteractionIfNeeded(_ interaction: PendingInteraction?) {
        guard let interaction = interaction else { return }
        guard AppSettings.shared.autoApprovePermissions else { return }
        guard !isSubmittingInteraction else { return }
        guard lastAutoApprovedInteractionId != interaction.id else { return }
        
        let isPermissionType = interaction.type == "permission" || interaction.type == "file_permission"
        guard isPermissionType else { return }
        
        // Find option 4 or option with scope 4 or text containing "always allow"
        guard let opt = interaction.options.first(where: {
            $0.scope == 4 || $0.id == "4" || $0.text.localizedCaseInsensitiveContains("always allow")
        }) else {
            return
        }
        
        lastAutoApprovedInteractionId = interaction.id
        
        Task { @MainActor [weak self] in
            // Small delay to allow any current render cycle to settle
            try? await Task.sleep(nanoseconds: 200_000_000)
            guard let self, self.pendingInteraction?.id == interaction.id, !self.isSubmittingInteraction else { return }
            await self.submitInteraction(optionId: opt.id, writeInText: nil, target: interaction.target)
        }
    }
    
    public func connectStream(force: Bool = false) {
        guard !cascadeId.isEmpty, let url = settings.serverURL else { return }
        streamClient.connect(baseURL: url, cascadeId: cascadeId, force: force)
    }
    
    
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
    
}
