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
        clientMessageId: String? = nil,
        userMessages: [ChatMessage]
    ) -> Bool {
        if let cid = clientMessageId, !cid.isEmpty {
            if userMessages.contains(where: { $0.clientMessageId == cid }) {
                return true
            }
        }
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
            clientMessageId: item.clientMessageId,
            userMessages: userMessages
        )
    }
    
    /// 在服务端队列里找到与本地条目对应的那一条：优先按客户端 id（精确），没有 id（旧网关、非本机发出）再按文字。
    public static func findServerQueueItem(in serverQueue: [QueuedMessageItem], for item: QueuedMessageItem, trimmedText: String) -> QueuedMessageItem? {
        if serverQueue.isEmpty { return nil }
        if let cid = item.clientMessageId, !cid.isEmpty {
            if let hit = serverQueue.first(where: { $0.clientMessageId == cid }) { return hit }
            // 网关已支持 id 标注（队列里有带 id 的条目）却找不到：说明服务端还没收到这一条，
            // 此时按文字回退可能误删另一条同文字消息，宁可返回空让调用方稍后重试
            if serverQueue.contains(where: { !($0.clientMessageId ?? "").isEmpty }) { return nil }
        }
        return serverQueue.first(where: { $0.text.trimmingCharacters(in: .whitespacesAndNewlines) == trimmedText })
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
                // 网关把客户端 id 写进了消息 tags，服务端队列里能精确认出这一条
                if let cid = opt.clientMessageId, !cid.isEmpty, $0.clientMessageId == cid { return true }
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
                clientMessageId: opt.clientMessageId,
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
                    clientMessageId: sItem.clientMessageId,
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
                    clientMessageId: qm.clientMessageId,
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
                if let cid = opt.clientMessageId, !cid.isEmpty, $0.clientMessageId == cid { return true }
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
                clientMessageId: opt.clientMessageId,
                userMessages: userMessages
            )
            if inChat {
                return nil
            }
            return QueuedMessageItem(
                id: opt.id,
                text: opt.text,
                media: opt.media,
                imageUrls: opt.imageUrls,
                clientMessageId: opt.clientMessageId
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

        let isOrchestrator = ModelDefaultsLogic.providerKind(forID: modelName) == .orchestrator
        var effectiveEnum = modelEnum
        if isOrchestrator {
            // Do not overwrite planModel/requestedModel with a bare Orchestrator anchor enum
            // (e.g. MODEL_PLACEHOLDER_M537); resolve the real planner enum when known or let
            // the gateway populate planModel + planExecuteConfig from the synthetic model ID.
            let bareAnchors: Set<String> = ["MODEL_PLACEHOLDER_M537", "MODEL_PLACEHOLDER_M667", "MODEL_PLACEHOLDER_M681"]
            if let plannerID = ModelDefaultsLogic.resolvedPlannerModelID(forOrchestratorID: modelName),
               let knownPlanner = ModelCatalogStore.shared.modelEnum(forID: plannerID) ?? ModelDefaultsLogic.staticEnumByID[plannerID] {
                effectiveEnum = knownPlanner
                plannerConfig["planModel"] = knownPlanner
                plannerConfig["requestedModel"] = ["model": knownPlanner]
            } else if !modelEnum.isEmpty && !bareAnchors.contains(modelEnum.uppercased()) {
                plannerConfig["planModel"] = modelEnum
                plannerConfig["requestedModel"] = ["model": modelEnum]
            }
            plannerConfig["modelName"] = modelName
        } else {
            plannerConfig["planModel"] = modelEnum
            plannerConfig["requestedModel"] = ["model": modelEnum]
            plannerConfig["modelName"] = modelName
        }
        json["plannerConfig"] = plannerConfig
        
        var checkpointConfig = json["checkpointConfig"] as? [String: Any] ?? [:]
        let isClaude = ModelDefaultsLogic.isClaude(effectiveEnum) || ModelDefaultsLogic.isClaude(modelName)
        
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
        // 单击胶囊在「设置 › 常用模型」的两个常用模型之间快速切换（当前非常用模型 1 时回切常用模型 1）
        activeModel = ModelDefaultsLogic.toggleFavoriteTarget(
            current: activeModel,
            favorite1: settings.favoriteModel1,
            favorite2: settings.favoriteModel2
        )
        activeModelName = nil
        updateCascadeConfigRawModel(activeModelEnum, modelName: activeModel)
        // 本地切换即刻生效，不立即发起网络请求，在发送消息时统一提交到桌面网关
    }

    @MainActor
    public func selectModel(_ modelId: String) {
        guard activeModel != modelId else { return }
        UIImpactFeedbackGenerator(style: .light).impactOccurred()
        hasUserManuallySelectedModel = true
        activeModel = modelId
        activeModelName = nil
        updateCascadeConfigRawModel(activeModelEnum, modelName: activeModel)
        // 本地选择即刻生效，在发送消息时统一提交到桌面网关
    }
    
    /// Clears both in-memory and persisted draft inputs (text, images, files, slash, mentions)
    /// synchronously when submitting the user's draft, preventing sent attachments from being
    /// restored on re-entry or app foregrounding.
    @MainActor
    public func clearCurrentDraftForSubmission() {
        markDraftMutated()
        for task in fileUploadTasks.values {
            task.cancel()
        }
        fileUploadTasks.removeAll()
        let key = draftKey
        inputText = ""
        selectedImageData = []
        selectedFiles = []
        selectedSlashCommand = nil
        selectedContextItems = []
        suggestedResponses = []
        suggestedResponseSource = ""
        dismissedPredictedSuggestion = nil
        if !key.isEmpty {
            cacheManager.clearDraft(key: key)
        }
    }

    @MainActor
    private func restoreDraftAfterFailure(
        targetDraftKey: String,
        isCurrentSession: Bool,
        text: String,
        images: [Data],
        files: [DraftFile],
        slashOption: SlashCommandOption?,
        contextItems: [ContextMentionItem]
    ) {
        if isCurrentSession {
            markDraftMutated()
            if !text.isEmpty {
                let existing = self.inputText.trimmingCharacters(in: .whitespacesAndNewlines)
                self.inputText = existing.isEmpty ? text : "\(self.inputText)\n\(text)"
            }
            if !images.isEmpty {
                var mergedImages = self.selectedImageData
                mergedImages.append(contentsOf: images)
                if mergedImages.count > 5 {
                    mergedImages = Array(mergedImages.prefix(5))
                }
                self.selectedImageData = mergedImages
            }
            if !files.isEmpty {
                let existingIds = Set(self.selectedFiles.map(\.id))
                let missing = files.filter { !existingIds.contains($0.id) }
                if !missing.isEmpty {
                    self.selectedFiles.append(contentsOf: missing)
                }
                self.persistDraftFiles()
            }
            if self.selectedSlashCommand == nil, let slashOption {
                self.selectedSlashCommand = slashOption
            }
            if !contextItems.isEmpty {
                for item in contextItems where !self.selectedContextItems.contains(where: { $0.id == item.id && $0.label == item.label }) {
                    self.selectedContextItems.append(item)
                }
            }
            self.saveCurrentDraft(includeImages: true)
            if !files.isEmpty {
                NotificationCenter.default.post(name: .conversationDraftChanged, object: self.draftKey)
            }
        } else if !targetDraftKey.isEmpty {
            if !text.isEmpty {
                let saved = self.cacheManager.getDraft(for: targetDraftKey)
                let merged = saved.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty ? text : "\(saved)\n\(text)"
                self.cacheManager.saveDraft(key: targetDraftKey, text: merged)
            }
            if !images.isEmpty {
                var savedImages = self.cacheManager.getDraftImages(for: targetDraftKey)
                savedImages.append(contentsOf: images)
                if savedImages.count > 5 {
                    savedImages = Array(savedImages.prefix(5))
                }
                self.cacheManager.saveDraftImages(key: targetDraftKey, images: savedImages)
            }
            if !files.isEmpty {
                let savedFiles = DraftFileStore.shared.load(key: targetDraftKey)
                let existingIds = Set(savedFiles.map(\.id))
                let mergedFiles = savedFiles + files.filter { !existingIds.contains($0.id) }
                DraftFileStore.shared.save(key: targetDraftKey, files: mergedFiles)
                NotificationCenter.default.post(name: .conversationDraftChanged, object: targetDraftKey)
            }
        }
    }

    @discardableResult
    @MainActor
    public func sendMessage(
        text customText: String? = nil,
        images: [Data]? = nil,
        files: [DraftFile]? = nil,
        slashCommand: String? = nil,
        slashOption: SlashCommandOption? = nil,
        contextItems: [ContextMentionItem]? = nil,
        clearsDraft: Bool = false
    ) async -> Bool {
        guard !isSending else { return false }
        let isDraftSend = clearsDraft || customText == nil
        let text = (customText ?? inputText).trimmingCharacters(in: .whitespacesAndNewlines)
        let sendImages = images ?? []
        let hasImages = !sendImages.isEmpty
        let sendFiles = files ?? []
        let hasFiles = !sendFiles.isEmpty
        let hasSlash = !(slashCommand ?? "").isEmpty
        let resolvedSlashOption = slashOption ?? (isDraftSend ? self.selectedSlashCommand : nil)
        let currentContextItems = contextItems ?? self.selectedContextItems
        let hasContextItems = !currentContextItems.isEmpty
        guard (!text.isEmpty || hasImages || hasFiles || hasSlash || hasContextItems), let url = settings.serverURL else { return false }
        // The gateway appends the attachment block to the text; mirror it locally so optimistic
        // bubbles and queued items match what the server will echo back.
        let bodyText = AttachmentRules.appendBlock(to: text, files: sendFiles)
        // language_server 把斜杠命令的显示文本生成为 "/name 内容"，本地气泡与队列条目保持一致
        let displayText = hasSlash ? "/\(slashCommand!)" + (bodyText.isEmpty ? "" : " " + bodyText) : bodyText
        let attachmentIds = sendFiles.compactMap(\.attachmentId)
        
        hasUserManuallySelectedModel = true
        updateCascadeConfigRawModel(activeModelEnum, modelName: activeModel)
        
        // 发送时将当前选定的模型同步提交到网关（上游 Language Server）；排队分支会在发送成功后再同步，避免与排包竞态
        let cid = self.cascadeId
        let currentModel = self.activeModel
        let capturedConfigRaw = self.cascadeConfigRaw
        let syncUpstreamModelSelection: () -> Void = { [weak self] in
            Task { [weak self] in
                guard let self else { return }
                do {
                    try await self.apiClient.switchModel(to: currentModel, cascadeId: cid.isEmpty ? nil : cid, baseURL: url)
                } catch {
                    print("⚠️ Failed to switch model upstream on send: \(error)")
                }
            }
        }
        
        isSending = true
        defer { isSending = false }
        currentTurnHadRemoteApproval = false
        AppReviewManager.shared.cancelScheduledPrompt()
        
        // If agent is currently running and session already exists, queue follow-up message!
        if (self.isRunning || self.isAwaitingResponse) && !self.cascadeId.isEmpty {
            UIImpactFeedbackGenerator(style: .medium).impactOccurred()
            let targetCascadeId = self.cascadeId
            let targetDraftKey = self.draftKey
            if isDraftSend {
                clearCurrentDraftForSubmission()
            }
            if !displayText.isEmpty {
                self.deletedQueueItemTombstones.removeAll(where: { $0.text == displayText })
            }
            let mediaBase64 = sendImages.isEmpty ? nil : sendImages.map { $0.base64EncodedString() }
            // 同一个 uuid 既做本地乐观条目的 id，也随消息发给网关（写进 tags），之后在服务端队列里按它精确对应
            let queueClientMsgId = UUID().uuidString
            let queueItem = QueuedMessageItem(
                id: "queue-\(queueClientMsgId)",
                text: displayText,
                media: mediaBase64,
                clientMessageId: queueClientMsgId
            )
            let lastUserMsgId = self.messages.last(where: { $0.isUser })?.id
            self.pendingOptimisticQueueItems.append(PendingOptimisticQueueItem(
                id: queueItem.id,
                text: displayText,
                media: mediaBase64,
                imageUrls: nil,
                createdAt: Date(),
                enqueuedAfterMessageId: lastUserMsgId,
                clientMessageId: queueClientMsgId
            ))
            withAnimation(.spring(response: 0.35, dampingFraction: 0.8)) {
                self.queuedMessages.append(queueItem)
            }
            triggerScrollToBottom()
            
            // Persist to local cache immediately
            let persistQueueCache: @MainActor () -> Void = { [weak self] in
                guard let self, self.cascadeId == targetCascadeId else { return }
                let toCache = self.messages.filter { $0.id != self.pendingOptimisticMessageId && !$0.id.hasPrefix("optimistic-") }
                self.cacheManager.saveSession(CachedChatSession(
                    cascadeId: targetCascadeId,
                    status: self.isRunning ? "CASCADE_RUN_STATUS_RUNNING" : "CASCADE_RUN_STATUS_DONE",
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
            }
            persistQueueCache()
            
            // Dispatch to server with deliveryStrategy = 2 (WHEN_IDLE).
            // Retry once on transient network drop using the same clientMessageId (gateway deduplicates);
            // if still rejected/offline, drop the optimistic card and restore unsent draft safely.
            Task { @MainActor [weak self] in
                guard let self else { return }
                let isTimeoutError: (Error) -> Bool = { err in
                    let desc = err.localizedDescription
                    return desc.contains("超时") || desc.contains("timed out") || desc.contains("送达服务器")
                }
                let attemptSend: () async throws -> Void = {
                    try await self.apiClient.sendMessage(
                        cascadeId: targetCascadeId,
                        text: text,
                        model: currentModel,
                        images: hasImages ? sendImages : nil,
                        deliveryStrategy: 2,
                        cascadeConfigRaw: capturedConfigRaw,
                        clientMessageId: queueClientMsgId,
                        attachmentIds: attachmentIds,
                        slashCommand: slashCommand,
                        contextItems: hasContextItems ? currentContextItems : nil,
                        baseURL: url
                    )
                }

                var finalError: Error? = nil
                do {
                    try await attemptSend()
                } catch {
                    if isTimeoutError(error) {
                        finalError = error
                    } else {
                        try? await Task.sleep(nanoseconds: 800_000_000)
                        do {
                            try await attemptSend()
                        } catch {
                            finalError = error
                        }
                    }
                }

                guard let err = finalError else {
                    syncUpstreamModelSelection()
                    return
                }

                print("⚠️ Failed to deliver queued message upstream: \(err)")
                if isTimeoutError(err) {
                    // Server may have received the queued message; refresh after a short pause without duplicating input.
                    try? await Task.sleep(nanoseconds: 2_000_000_000)
                    if self.cascadeId == targetCascadeId {
                        await self.loadMessages(isBackgroundPoll: true)
                    }
                    return
                }

                // Hard delivery failure: remove phantom optimistic queue item & restore unsent draft safely.
                self.pendingOptimisticQueueItems.removeAll(where: { $0.id == queueItem.id })
                let isSameSession = (self.cascadeId == targetCascadeId)
                if isSameSession {
                    withAnimation(.spring(response: 0.3, dampingFraction: 0.85)) {
                        self.queuedMessages.removeAll(where: { $0.id == queueItem.id })
                    }
                    persistQueueCache()
                }
                if isDraftSend {
                    self.restoreDraftAfterFailure(
                        targetDraftKey: targetDraftKey,
                        isCurrentSession: isSameSession,
                        text: text,
                        images: sendImages,
                        files: sendFiles,
                        slashOption: resolvedSlashOption,
                        contextItems: currentContextItems
                    )
                }
                if isSameSession {
                    self.errorMessage = String(localized: "排队消息发送失败，已恢复到输入框")
                    UINotificationFeedbackGenerator().notificationOccurred(.error)
                }
            }
            return true
        }
        syncUpstreamModelSelection()
        
        // Haptic feedback
        UIImpactFeedbackGenerator(style: .medium).impactOccurred()
        
        // Snapshot known server message IDs before sending (excluding any optimistic items)
        self.knownServerMessageIds = Set(messages.filter { $0.id != pendingOptimisticMessageId && !$0.id.hasPrefix("optimistic-") }.map(\.id))
        
        // If this was an uninitiated session, clear the flag upon sending first message
        if self.isNewConversation {
            self.isNewConversation = false
        }
        
        // Optimistic update with unique client message id
        let targetDraftKey = self.draftKey
        let clientMessageId = UUID().uuidString
        let optId = "optimistic-\(clientMessageId)"
        let optSlashMentions: [SlashCommandMention]? = {
            guard let slash = slashCommand, !slash.isEmpty else { return nil }
            let matched = self.slashCommands.first(where: { $0.name == slash })
            let kind = matched?.kind ?? (SlashCommandIconCatalog.defaultSystemIcons[slash] != nil ? "system" : "skill")
            let icon = SlashCommandIconCatalog.resolveIcon(name: slash, icon: matched?.icon, kind: kind)
            return [SlashCommandMention(name: slash, icon: icon, kind: kind, definitionPath: matched?.definitionPath)]
        }()
        messages.append(ChatMessage(id: optId, sender: .user, content: displayText, imageDataList: sendImages, slashCommands: optSlashMentions, clientMessageId: clientMessageId))
        triggerScrollToBottom()
        self.pendingOptimisticMessageId = optId
        self.isAwaitingResponse = true
        self.awaitingResponseSince = Date()
        self.isRunning = true
        self.canProceed = false
        self.hasError = false
        self.trajectoryErrorMessage = nil
        if isDraftSend {
            clearCurrentDraftForSubmission()
        }
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
                let initialPrompt = !hasImages && !hasFiles && !hasSlash && !hasContextItems ? text : ""
                let newCascadeId = try await apiClient.createCascade(
                    workspaceUri: wsUri,
                    prompt: initialPrompt,
                    model: activeModel,
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
                DraftFileStore.shared.clear(key: newCascadeId)
                self.originDraftId = nil
                self.cascadeId = newCascadeId
                self.draftSession = nil
                self.draftProject = nil
                self.isNewConversation = false
                self.workspaceName = isPure ? "Chat" : project.name
                
                // Immediately register new conversation item in cache
                let newConv = ConversationItem(
                    id: newCascadeId,
                    title: isPure ? String(localized: "新对话") : project.name,
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
                
                if hasImages || hasFiles || hasSlash || hasContextItems {
                    try await apiClient.sendMessage(
                        cascadeId: newCascadeId,
                        text: text,
                        model: activeModel,
                        images: hasImages ? sendImages : nil,
                        cascadeConfigRaw: cascadeConfigRaw,
                        clientMessageId: UUID().uuidString,
                        attachmentIds: attachmentIds,
                        slashCommand: slashCommand,
                        contextItems: hasContextItems ? currentContextItems : nil,
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
                    model: activeModel,
                    images: hasImages ? sendImages : nil,
                    cascadeConfigRaw: cascadeConfigRaw,
                    clientMessageId: clientMessageId,
                    attachmentIds: attachmentIds,
                    slashCommand: slashCommand,
                    contextItems: hasContextItems ? currentContextItems : nil,
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
                // Do NOT restore draft text/images/files: keep input empty so user won't duplicate send!
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
                // If session was not yet created, preserve isNewConversation so the user stays in draft mode
                if cascadeId.isEmpty && (draftProject != nil || draftSession != nil) {
                    isNewConversation = true
                }
                // Restore draft so user does not lose their input/attachments on hard network failure
                if isDraftSend {
                    restoreDraftAfterFailure(
                        targetDraftKey: targetDraftKey,
                        isCurrentSession: true,
                        text: text,
                        images: sendImages,
                        files: sendFiles,
                        slashOption: resolvedSlashOption,
                        contextItems: currentContextItems
                    )
                }
            }
            stopPollingFallback()
            return false
        }
    }

    @MainActor
    public func ensureCascadeForVoiceCall() async throws -> String {
        if !cascadeId.isEmpty {
            return cascadeId
        }
        guard let url = settings.serverURL else {
            throw APIError.invalidURL
        }
        let project = draftProject ?? draftSession?.project ?? ProjectItem.pureChat
        let isPure = project.isPureChat
        let pid = isPure ? "outside-of-project" : (project.rawId ?? (project.id != project.uri ? project.id : nil))
        let wsUri = isPure ? "" : project.uri
        let newCascadeId = try await apiClient.createCascade(
            workspaceUri: wsUri,
            prompt: "",
            model: activeModel,
            projectId: pid,
            clientMessageId: UUID().uuidString,
            baseURL: url
        )
        bindVoiceCascadeId(newCascadeId, project: project)
        return newCascadeId
    }

    @MainActor
    public func bindVoiceCascadeId(_ newCascadeId: String, project: ProjectItem? = nil) {
        guard !newCascadeId.isEmpty, cascadeId != newCascadeId else { return }
        let resolvedProject = project ?? draftProject ?? draftSession?.project ?? ProjectItem.pureChat
        let isPure = resolvedProject.isPureChat

        var draftKeysToPurge = Set<String>()
        if let orig = self.originDraftId, !orig.isEmpty {
            draftKeysToPurge.insert(orig)
        }
        if let dSession = draftSession {
            draftKeysToPurge.insert(dSession.id)
        }
        draftKeysToPurge.insert("draft_project_\(resolvedProject.id)")

        for key in draftKeysToPurge {
            self.cacheManager.deleteLocalDraftSession(id: key)
            self.cacheManager.clearDraft(key: key)
            self.cacheManager.clearDraftImages(key: key)
            DraftFileStore.shared.clear(key: key)
            NotificationCenter.default.post(name: .conversationDraftDeleted, object: key)
        }

        self.originDraftId = nil
        self.cascadeId = newCascadeId
        self.draftSession = nil
        self.draftProject = nil
        self.isNewConversation = false
        self.workspaceName = isPure ? "Chat" : resolvedProject.name

        let newConv = ConversationItem(
            id: newCascadeId,
            title: isPure ? String(localized: "新对话") : resolvedProject.name,
            status: .idle,
            stepCount: 0,
            workspaceName: isPure ? "Chat" : resolvedProject.name,
            lastModified: Date()
        )
        self.cacheManager.upsertConversation(newConv)
        connectStream()
    }

}
