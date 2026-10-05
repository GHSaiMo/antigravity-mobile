import Foundation
import Observation
import UIKit
import SwiftUI

extension ChatViewModel {
    // MARK: - Queued Messages Synchronization
    
    public static func isUserQueuedMessage(_ text: String) -> Bool {
        let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
        if trimmed.isEmpty { return false }
        if trimmed.hasPrefix("Task id \"") || trimmed.hasPrefix("Task \"") ||
           trimmed.contains("was canceled with result:") || trimmed.contains("completed with result:") ||
           trimmed.contains("Tool execution was canceled") {
            return false
        }
        return true
    }
    
    public static func isUserQueuedItem(_ item: QueuedMessageItem) -> Bool {
        let trimmed = item.text.trimmingCharacters(in: .whitespacesAndNewlines)
        if trimmed.isEmpty && !item.hasAttachments { return false }
        if trimmed.hasPrefix("Task id \"") || trimmed.hasPrefix("Task \"") ||
           trimmed.contains("was canceled with result:") || trimmed.contains("completed with result:") ||
           trimmed.contains("Tool execution was canceled") {
            return false
        }
        return true
    }
    
    public static func normalizeForComparison(_ text: String) -> String {
        let stripped = text.unicodeScalars.filter { scalar in
            !CharacterSet.whitespacesAndNewlines.contains(scalar) &&
            !CharacterSet.controlCharacters.contains(scalar) &&
            scalar.value != 0x200B && // zero-width space
            scalar.value != 0xFEFF && // zero-width no-break space
            scalar.value != 0x3000    // ideographic space
        }
        return String(String.UnicodeScalarView(stripped)).lowercased()
    }
    
    public static func isQueuedItemInMessages(
        text: String,
        media: [String]?,
        imageUrls: [String]?,
        enqueuedAfterMessageId: String? = nil,
        userMessages: [ChatMessage]
    ) -> Bool {
        let normText = normalizeForComparison(text)
        let hasAttachments = (media != nil && !media!.isEmpty) || (imageUrls != nil && !imageUrls!.isEmpty)
        
        for uMsg in userMessages.reversed() {
            if let afterId = enqueuedAfterMessageId, uMsg.id == afterId {
                break
            }
            
            let normMsg = normalizeForComparison(uMsg.content)
            let msgHasAttachments = !uMsg.imageDataList.isEmpty || !uMsg.imageUrls.isEmpty
            
            if !normText.isEmpty {
                if normText == normMsg {
                    return true
                }
                if normText.count >= 6 && normMsg.count >= 6 && (normText.contains(normMsg) || normMsg.contains(normText)) {
                    return true
                }
            } else if hasAttachments && msgHasAttachments {
                return true
            }
        }
        return false
    }
    
    public static func isQueuedItemInMessages(_ item: QueuedMessageItem, userMessages: [ChatMessage]) -> Bool {
        return isQueuedItemInMessages(
            text: item.text,
            media: item.media,
            imageUrls: item.imageUrls,
            enqueuedAfterMessageId: nil,
            userMessages: userMessages
        )
    }
    
    func syncQueuedMessages(serverQueue: [QueuedMessageItem]?) {
        let now = Date()
        // 1. Expire stale optimistic items older than 15 seconds
        pendingOptimisticQueueItems.removeAll(where: { now.timeIntervalSince($0.createdAt) > 15.0 })
        
        // 2. Expire stale tombstones older than 10 seconds
        deletedQueueItemTombstones.removeAll(where: { now.timeIntervalSince($0.deletedAt) > 10.0 })
        let tombstoneIds = Set(deletedQueueItemTombstones.compactMap(\.id))
        let tombstoneTexts = Set(deletedQueueItemTombstones.map { Self.normalizeForComparison($0.text) })
        
        // 3. Identify user messages in the active conversation
        let userMessages = self.messages.filter { $0.isUser }
        
        // 4. Clear optimistic items if server has incorporated them OR if entered chat OR if tombstoned
        pendingOptimisticQueueItems.removeAll { opt in
            let trimmed = opt.text.trimmingCharacters(in: .whitespacesAndNewlines)
            let normOpt = Self.normalizeForComparison(opt.text)
            let inServer = serverQueue?.contains(where: {
                if !normOpt.isEmpty {
                    return Self.normalizeForComparison($0.text) == normOpt
                }
                return $0.id == opt.id
            }) == true
            let inChat = Self.isQueuedItemInMessages(
                text: opt.text,
                media: opt.media,
                imageUrls: opt.imageUrls,
                enqueuedAfterMessageId: opt.enqueuedAfterMessageId,
                userMessages: userMessages
            )
            let isTombstoned = tombstoneIds.contains(opt.id) || (!normOpt.isEmpty && tombstoneTexts.contains(normOpt))
            if inChat {
                self.deletedQueueItemTombstones.append(QueuedMessageTombstone(id: opt.id, text: trimmed, deletedAt: Date()))
            }
            return inServer || inChat || isTombstoned
        }
        
        // 5. Compute base queue from server if provided, otherwise filter existing queue
        var baseQueue: [QueuedMessageItem]
        if let sq = serverQueue {
            baseQueue = sq.filter { sItem in
                let trimmed = sItem.text.trimmingCharacters(in: .whitespacesAndNewlines)
                let normItem = Self.normalizeForComparison(sItem.text)
                let inChat = Self.isQueuedItemInMessages(
                    text: sItem.text,
                    media: sItem.media,
                    imageUrls: sItem.imageUrls,
                    enqueuedAfterMessageId: nil,
                    userMessages: userMessages
                )
                let isTombstoned = tombstoneIds.contains(sItem.id) || (!normItem.isEmpty && tombstoneTexts.contains(normItem))
                if inChat {
                    self.deletedQueueItemTombstones.append(QueuedMessageTombstone(id: sItem.id, text: trimmed, deletedAt: Date()))
                }
                return Self.isUserQueuedItem(sItem) && !inChat && !isTombstoned
            }
        } else {
            baseQueue = self.queuedMessages.filter { qm in
                let trimmed = qm.text.trimmingCharacters(in: .whitespacesAndNewlines)
                let normItem = Self.normalizeForComparison(qm.text)
                let inChat = Self.isQueuedItemInMessages(
                    text: qm.text,
                    media: qm.media,
                    imageUrls: qm.imageUrls,
                    enqueuedAfterMessageId: nil,
                    userMessages: userMessages
                )
                let isTombstoned = tombstoneIds.contains(qm.id) || (!normItem.isEmpty && tombstoneTexts.contains(normItem))
                if inChat {
                    self.deletedQueueItemTombstones.append(QueuedMessageTombstone(id: qm.id, text: trimmed, deletedAt: Date()))
                }
                return Self.isUserQueuedItem(qm) && !qm.id.hasPrefix("queue-") && !inChat && !isTombstoned
            }
        }
        
        // 6. Append unconfirmed optimistic items (not yet in server queue, not entered chat, not tombstoned)
        let remainingOptItems = pendingOptimisticQueueItems.compactMap { opt -> QueuedMessageItem? in
            let normOpt = Self.normalizeForComparison(opt.text)
            if tombstoneIds.contains(opt.id) || (!normOpt.isEmpty && tombstoneTexts.contains(normOpt)) {
                return nil
            }
            if baseQueue.contains(where: {
                if !normOpt.isEmpty {
                    return Self.normalizeForComparison($0.text) == normOpt
                }
                return $0.id == opt.id
            }) {
                return nil
            }
            let inChat = Self.isQueuedItemInMessages(
                text: opt.text,
                media: opt.media,
                imageUrls: opt.imageUrls,
                enqueuedAfterMessageId: opt.enqueuedAfterMessageId,
                userMessages: userMessages
            )
            if inChat {
                return nil
            }
            return QueuedMessageItem(
                id: opt.id,
                text: opt.text,
                media: opt.media,
                imageUrls: opt.imageUrls
            )
        }
        
        let newQueue = baseQueue + remainingOptItems
        if newQueue != self.queuedMessages {
            withAnimation(.spring(response: 0.35, dampingFraction: 0.8)) {
                self.queuedMessages = newQueue
            }
            triggerScrollToBottom()
        }
    }

    
    func updateCascadeConfigRawModel(_ modelEnum: String, modelName: String) {
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
    public func sendMessage(text customText: String? = nil, images: [Data]? = nil, files: [DraftFile]? = nil) async -> Bool {
        guard !isSending else { return false }
        let text = (customText ?? inputText).trimmingCharacters(in: .whitespacesAndNewlines)
        let hasImages = (images != nil && !images!.isEmpty)
        let sendFiles = files ?? []
        let hasFiles = !sendFiles.isEmpty
        guard (!text.isEmpty || hasImages || hasFiles), let url = settings.serverURL else { return false }
        // The gateway appends the attachment block to the text; mirror it locally so optimistic
        // bubbles and queued items match what the server will echo back.
        let displayText = AttachmentRules.appendBlock(to: text, files: sendFiles)
        let attachmentIds = sendFiles.compactMap(\.attachmentId)
        
        hasUserManuallySelectedModel = true
        updateCascadeConfigRawModel(activeModelEnum, modelName: activeModel)
        
        isSending = true
        defer { isSending = false }
        
        // If agent is currently running and session already exists, queue follow-up message!
        if (self.isRunning || self.isAwaitingResponse) && !self.cascadeId.isEmpty {
            UIImpactFeedbackGenerator(style: .medium).impactOccurred()
            if !displayText.isEmpty {
                self.deletedQueueItemTombstones.removeAll(where: { $0.text == displayText })
            }
            let mediaBase64 = images?.map { $0.base64EncodedString() }
            let queueItem = QueuedMessageItem(
                id: "queue-\(UUID().uuidString)",
                text: displayText,
                media: mediaBase64
            )
            let lastUserMsgId = self.messages.last(where: { $0.isUser })?.id
            self.pendingOptimisticQueueItems.append(PendingOptimisticQueueItem(
                id: queueItem.id,
                text: displayText,
                media: mediaBase64,
                imageUrls: nil,
                createdAt: Date(),
                enqueuedAfterMessageId: lastUserMsgId
            ))
            withAnimation(.spring(response: 0.35, dampingFraction: 0.8)) {
                self.queuedMessages.append(queueItem)
            }
            triggerScrollToBottom()
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
                        attachmentIds: attachmentIds,
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
        messages.append(ChatMessage(id: optId, sender: .user, content: displayText, imageDataList: images ?? []))
        triggerScrollToBottom()
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
            syncLiveActivity()
            
            // Ensure WebSocket stream is connected for immediate streaming
            connectStream()
            
            // If stream is not actively connected, use fallback polling
            if streamClient.status != .connected {
                startPollingFallback()
            }
        }
        
        do {
            if cascadeId.isEmpty, let project = draftProject ?? draftSession?.project ?? (originDraftId != nil ? ProjectItem.pureChat : nil) {
                let isPure = project.isPureChat
                let pid = isPure ? "outside-of-project" : (project.rawId ?? (project.id != project.uri ? project.id : nil))
                let wsUri = isPure ? "" : project.uri
                let initialPrompt = (images == nil || images!.isEmpty) && !hasFiles ? text : ""
                let newCascadeId = try await apiClient.createCascade(
                    workspaceUri: wsUri,
                    prompt: initialPrompt,
                    model: activeModelEnum,
                    projectId: pid,
                    clientMessageId: clientMessageId,
                    baseURL: url
                )
                
                var draftKeysToPurge = Set<String>()
                if let orig = self.originDraftId, !orig.isEmpty {
                    draftKeysToPurge.insert(orig)
                }
                if let dSession = draftSession {
                    draftKeysToPurge.insert(dSession.id)
                }
                draftKeysToPurge.insert("draft_project_\(project.id)")
                
                for key in draftKeysToPurge {
                    self.cacheManager.deleteLocalDraftSession(id: key)
                    self.cacheManager.clearDraft(key: key)
                    self.cacheManager.clearDraftImages(key: key)
                    DraftFileStore.shared.clear(key: key)
                    NotificationCenter.default.post(name: .conversationDraftDeleted, object: key)
                }
                
                self.cacheManager.clearDraft(key: newCascadeId)
                self.cacheManager.clearDraftImages(key: newCascadeId)
                self.originDraftId = nil
                self.cascadeId = newCascadeId
                self.draftSession = nil
                self.draftProject = nil
                self.isNewConversation = false
                self.workspaceName = isPure ? "Chat" : project.name
                
                // Immediately register new conversation item in cache
                let newConv = ConversationItem(
                    id: newCascadeId,
                    title: isPure ? "新对话" : project.name,
                    status: .running,
                    stepCount: 1,
                    workspaceName: isPure ? "Chat" : project.name,
                    lastModified: Date()
                )
                self.cacheManager.upsertConversation(newConv)
                
                syncLiveActivity()
                
                // Ensure WebSocket stream is connected for immediate streaming
                connectStream()
                
                // If stream is not actively connected, use fallback polling
                if streamClient.status != .connected {
                    startPollingFallback()
                }
                
                if (images?.isEmpty == false) || hasFiles {
                    try await apiClient.sendMessage(
                        cascadeId: newCascadeId,
                        text: text,
                        model: activeModelEnum,
                        images: images,
                        cascadeConfigRaw: cascadeConfigRaw,
                        clientMessageId: UUID().uuidString,
                        attachmentIds: attachmentIds,
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
                    attachmentIds: attachmentIds,
                    baseURL: url
                )
                // Allow upstream 250ms to register task and update state before first eager sync
                try? await Task.sleep(nanoseconds: 250_000_000)
                await self.loadMessages(isBackgroundPoll: true)
                self.cacheManager.clearDraftImages(key: cascadeId)
                DraftFileStore.shared.clear(key: cascadeId)
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
    
}
