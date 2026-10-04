import Foundation
import Observation
import UIKit
import SwiftUI

extension ChatViewModel {
    // MARK: - Queued Messages Actions
    
    @MainActor
    public func sendQueuedMessageNow(item: QueuedMessageItem) async {
        guard let url = settings.serverURL, !cascadeId.isEmpty else { return }
        guard !isSending else { return }
        isSending = true
        defer { isSending = false }
        
        UIImpactFeedbackGenerator(style: .medium).impactOccurred()
        
        // Save original states for rollback on failure
        let originalQueued = self.queuedMessages
        let originalPending = self.pendingOptimisticQueueItems
        let trimmedText = item.text.trimmingCharacters(in: .whitespacesAndNewlines)
        self.deletedQueueItemTombstones.append(QueuedMessageTombstone(id: item.id, text: trimmedText, deletedAt: Date()))
        
        // Remove from optimistic queue list
        self.pendingOptimisticQueueItems.removeAll(where: { $0.id == item.id || $0.text.trimmingCharacters(in: .whitespacesAndNewlines) == trimmedText })
        withAnimation(.spring(response: 0.35, dampingFraction: 0.8)) {
            self.queuedMessages.removeAll(where: { $0.id == item.id })
        }
        
        // Snapshot known server message IDs before sending (excluding any optimistic items)
        self.knownServerMessageIds = Set(messages.filter { $0.id != pendingOptimisticMessageId && !$0.id.hasPrefix("optimistic-") }.map(\.id))
        
        let imgDataList = (item.media ?? []).compactMap { raw -> Data? in
            let cleaned: String
            if let commaIndex = raw.firstIndex(of: ",") {
                cleaned = String(raw[raw.index(after: commaIndex)...])
            } else {
                cleaned = raw
            }
            return Data(base64Encoded: cleaned)
        }
        
        // Optimistic user chat bubble
        let clientMessageId = UUID().uuidString
        let optId = "optimistic-\(clientMessageId)"
        withAnimation(.spring(response: 0.35, dampingFraction: 0.8)) {
            self.messages.append(ChatMessage(id: optId, sender: .user, content: item.text, imageDataList: imgDataList))
        }
        triggerScrollToBottom()
        self.pendingOptimisticMessageId = optId
        self.isAwaitingResponse = true
        self.awaitingResponseSince = Date()
        self.isRunning = true
        self.canProceed = false
        self.hasError = false
        self.trajectoryErrorMessage = nil
        self.errorMessage = nil
        
        // Persist to local cache immediately
        let toCache = self.messages.filter { $0.id != self.pendingOptimisticMessageId && !$0.id.hasPrefix("optimistic-") }
        cacheManager.saveSession(CachedChatSession(
            cascadeId: cascadeId,
            status: "CASCADE_RUN_STATUS_RUNNING",
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
        
        // Ensure stream is actively connected
        connectStream()
        if streamClient.status != .connected {
            startPollingFallback()
        }
        
        // Dispatch with deliveryStrategy = 1 (NEXT_INVOCATION)
        do {
            try await apiClient.sendMessage(
                cascadeId: cascadeId,
                text: item.text,
                model: activeModelEnum,
                images: imgDataList.isEmpty ? nil : imgDataList,
                deliveryStrategy: 1,
                cascadeConfigRaw: cascadeConfigRaw,
                clientMessageId: clientMessageId,
                baseURL: url
            )
            
            // On successful dispatch: remove from upstream queue if it had a real server message ID
            Task { [weak self] in
                guard let self else { return }
                var targetMsgId: String? = item.id.hasPrefix("queue-") ? nil : item.id
                if targetMsgId == nil {
                    try? await Task.sleep(nanoseconds: 350_000_000)
                    if let currentMsgs = try? await self.apiClient.fetchMessages(cascadeId: self.cascadeId, limit: 15, offset: nil, baseURL: url) {
                        let trimmedTarget = item.text.trimmingCharacters(in: .whitespacesAndNewlines)
                        if let match = currentMsgs.queuedMessages.first(where: { $0.text.trimmingCharacters(in: .whitespacesAndNewlines) == trimmedTarget }) {
                            targetMsgId = match.id
                        }
                    }
                }
                if let msgId = targetMsgId, !msgId.hasPrefix("queue-") {
                    _ = try? await self.apiClient.deleteAgentMessage(messageId: msgId, cascadeId: self.cascadeId, baseURL: url)
                }
            }
            
            // Allow upstream 250ms to register task and update state before first eager sync
            try? await Task.sleep(nanoseconds: 250_000_000)
            await self.loadMessages(isBackgroundPoll: true)
        } catch {
            print("❌ sendQueuedMessageNow error: \(error)")
            self.deletedQueueItemTombstones.removeAll(where: { $0.id == item.id || $0.text == trimmedText })
            withAnimation(.spring(response: 0.35, dampingFraction: 0.8)) {
                self.queuedMessages = originalQueued
                self.pendingOptimisticQueueItems = originalPending
                self.messages.removeAll(where: { $0.id == optId })
                if self.pendingOptimisticMessageId == optId {
                    self.pendingOptimisticMessageId = nil
                }
            }
            self.errorMessage = "发送失败: \(error.localizedDescription)"
        }
    }
    
    @MainActor
    public func editQueuedMessage(item: QueuedMessageItem) {
        guard !inFlightDeletingQueueIds.contains(item.id) else { return }
        inFlightDeletingQueueIds.insert(item.id)
        
        UIImpactFeedbackGenerator(style: .light).impactOccurred()
        let trimmedText = item.text.trimmingCharacters(in: .whitespacesAndNewlines)
        self.deletedQueueItemTombstones.append(QueuedMessageTombstone(id: item.id, text: trimmedText, deletedAt: Date()))
        self.pendingOptimisticQueueItems.removeAll(where: { $0.id == item.id || $0.text.trimmingCharacters(in: .whitespacesAndNewlines) == trimmedText })
        withAnimation(.spring(response: 0.35, dampingFraction: 0.8)) {
            self.queuedMessages.removeAll(where: { $0.id == item.id })
        }
        triggerScrollToBottom()
        
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
        
        if let url = settings.serverURL, !cascadeId.isEmpty {
            Task { @MainActor [weak self] in
                defer {
                    self?.inFlightDeletingQueueIds.remove(item.id)
                }
                guard let self else { return }
                var targetMsgId: String? = item.id.hasPrefix("queue-") ? nil : item.id
                if targetMsgId == nil {
                    try? await Task.sleep(nanoseconds: 350_000_000)
                    if let currentMsgs = try? await self.apiClient.fetchMessages(cascadeId: self.cascadeId, limit: 15, offset: nil, baseURL: url) {
                        if let match = currentMsgs.queuedMessages.first(where: { $0.text.trimmingCharacters(in: .whitespacesAndNewlines) == trimmedText }) {
                            targetMsgId = match.id
                            self.deletedQueueItemTombstones.append(QueuedMessageTombstone(id: match.id, text: trimmedText, deletedAt: Date()))
                            self.inFlightDeletingQueueIds.insert(match.id)
                        }
                    }
                }
                if let msgId = targetMsgId, !msgId.hasPrefix("queue-") {
                    _ = try? await self.apiClient.deleteAgentMessage(messageId: msgId, cascadeId: self.cascadeId, baseURL: url)
                    self.inFlightDeletingQueueIds.remove(msgId)
                }
            }
        } else {
            inFlightDeletingQueueIds.remove(item.id)
        }
        
        self.inputText = item.text
        if let media = item.media, !media.isEmpty {
            let images = media.compactMap { raw -> Data? in
                let cleaned: String
                if let commaIndex = raw.firstIndex(of: ",") {
                    cleaned = String(raw[raw.index(after: commaIndex)...])
                } else {
                    cleaned = raw
                }
                return Data(base64Encoded: cleaned)
            }
            if !images.isEmpty {
                self.selectedImageData = images
            }
        }
    }
    
    @MainActor
    public func deleteQueuedMessage(item: QueuedMessageItem) {
        guard !inFlightDeletingQueueIds.contains(item.id) else { return }
        inFlightDeletingQueueIds.insert(item.id)
        
        UIImpactFeedbackGenerator(style: .light).impactOccurred()
        let trimmedText = item.text.trimmingCharacters(in: .whitespacesAndNewlines)
        self.deletedQueueItemTombstones.append(QueuedMessageTombstone(id: item.id, text: trimmedText, deletedAt: Date()))
        self.pendingOptimisticQueueItems.removeAll(where: { $0.id == item.id || $0.text.trimmingCharacters(in: .whitespacesAndNewlines) == trimmedText })
        withAnimation(.spring(response: 0.35, dampingFraction: 0.8)) {
            self.queuedMessages.removeAll(where: { $0.id == item.id })
        }
        triggerScrollToBottom()
        
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
        
        if let url = settings.serverURL, !cascadeId.isEmpty {
            Task { @MainActor [weak self] in
                defer {
                    self?.inFlightDeletingQueueIds.remove(item.id)
                }
                guard let self else { return }
                var targetMsgId: String? = item.id.hasPrefix("queue-") ? nil : item.id
                if targetMsgId == nil {
                    try? await Task.sleep(nanoseconds: 350_000_000)
                    if let currentMsgs = try? await self.apiClient.fetchMessages(cascadeId: self.cascadeId, limit: 15, offset: nil, baseURL: url) {
                        if let match = currentMsgs.queuedMessages.first(where: { $0.text.trimmingCharacters(in: .whitespacesAndNewlines) == trimmedText }) {
                            targetMsgId = match.id
                            self.deletedQueueItemTombstones.append(QueuedMessageTombstone(id: match.id, text: trimmedText, deletedAt: Date()))
                            self.inFlightDeletingQueueIds.insert(match.id)
                        }
                    }
                }
                if let msgId = targetMsgId, !msgId.hasPrefix("queue-") {
                    _ = try? await self.apiClient.deleteAgentMessage(messageId: msgId, cascadeId: self.cascadeId, baseURL: url)
                    self.inFlightDeletingQueueIds.remove(msgId)
                }
            }
        } else {
            inFlightDeletingQueueIds.remove(item.id)
        }
    }
    
    @MainActor
    public func stopTask(_ task: RunningTaskItem) async {
        guard let url = settings.serverURL, !self.cascadeId.isEmpty else { return }
        UIImpactFeedbackGenerator(style: .medium).impactOccurred()
        
        withAnimation(.spring(response: 0.35, dampingFraction: 0.8)) {
            self.runningTasks.removeAll(where: { $0.id == task.id && $0.stepIndex == task.stepIndex })
        }
        syncLiveActivity()
        
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
        
        syncLiveActivity()
        
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
    public func openMarkdownViewer(uri: String, title: String? = nil, forceRefresh: Bool = false) {
        let cleanURI = uri.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !cleanURI.isEmpty else { return }
        
        let unescapedURI = cleanURI.removingPercentEncoding ?? cleanURI
        let rawFileName = (unescapedURI as NSString).lastPathComponent
        var fileName = rawFileName.removingPercentEncoding ?? rawFileName
        if fileName.isEmpty || fileName == "/" {
            fileName = "document.md"
        } else if !(fileName as NSString).pathExtension.lowercased().contains("md") {
            fileName = "\(fileName).md"
        }
        
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
        
        // Fast-path: Check persistent local disk cache first (unless force refresh requested)
        let cached = (!forceRefresh) ? DocumentCacheManager.shared.getCachedMarkdown(
            for: targetURI,
            fileName: fileName,
            cascadeId: self.cascadeId
        ) : nil
        
        let hasCache = cached != nil
        let initialContent = cached?.response.content ?? ""
        let initialSummary = cached?.response.summary
        let initialProceed = isProceedActive || (cached?.response.requestFeedback == true && self.canProceed)
        let initialFileURL = cached?.fileURL
        let initialTitle: String = {
            if !isWalkthrough && !isPlan, let fn = cached?.response.filename, !fn.isEmpty {
                return fn
            }
            return resolvedTitle
        }()
        
        let viewerId = targetURI + "_\(Date().timeIntervalSince1970)"
        let viewer = MarkdownFileViewerData(
            id: viewerId,
            title: initialTitle,
            uri: targetURI,
            content: initialContent,
            summary: initialSummary,
            isLoading: !hasCache,
            isRefreshing: hasCache,
            errorMessage: nil,
            canProceed: initialProceed,
            cachedFileURL: initialFileURL,
            isCached: hasCache
        )
        self.viewingMarkdownFile = viewer
        
        Task { [weak self] in
            guard let self else { return }
            guard let url = settings.serverURL else {
                if self.viewingMarkdownFile?.id == viewerId {
                    self.viewingMarkdownFile?.isRefreshing = false
                    if !hasCache {
                        self.viewingMarkdownFile?.isLoading = false
                        self.viewingMarkdownFile?.errorMessage = "未连接到网关服务器"
                    }
                }
                return
            }
            do {
                let resp = try await apiClient.fetchFileContent(
                    uri: targetURI,
                    cascadeId: self.cascadeId,
                    baseURL: url
                )
                
                // Save persistently to local cache on phone
                let (savedURL, _) = (try? DocumentCacheManager.shared.saveMarkdownToCache(
                    response: resp,
                    for: targetURI,
                    fileName: fileName,
                    cascadeId: self.cascadeId
                )) ?? (initialFileURL ?? DocumentCacheManager.shared.cacheFileURL(for: targetURI, fileName: fileName, cascadeId: self.cascadeId), resp)
                
                if self.viewingMarkdownFile?.id == viewerId {
                    self.viewingMarkdownFile?.content = resp.content
                    self.viewingMarkdownFile?.summary = resp.summary
                    self.viewingMarkdownFile?.isLoading = false
                    self.viewingMarkdownFile?.isRefreshing = false
                    self.viewingMarkdownFile?.isCached = true
                    self.viewingMarkdownFile?.cachedFileURL = savedURL
                    self.viewingMarkdownFile?.errorMessage = nil
                    if !isWalkthrough && !isPlan && !resp.filename.isEmpty {
                        self.viewingMarkdownFile?.title = resp.filename
                    }
                    if resp.requestFeedback == true && self.canProceed {
                        self.viewingMarkdownFile?.canProceed = true
                    }
                }
            } catch {
                if self.viewingMarkdownFile?.id == viewerId {
                    self.viewingMarkdownFile?.isRefreshing = false
                    if !hasCache {
                        self.viewingMarkdownFile?.isLoading = false
                        self.viewingMarkdownFile?.errorMessage = "加载文档失败: \(error.localizedDescription)"
                    }
                }
            }
        }
    }
    
    @MainActor
    public func prefetchMarkdownArtifact(uri: String) {
        let cleanURI = uri.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !cleanURI.isEmpty, let url = settings.serverURL else { return }
        
        let unescapedURI = cleanURI.removingPercentEncoding ?? cleanURI
        let rawFileName = (unescapedURI as NSString).lastPathComponent
        var fileName = rawFileName.removingPercentEncoding ?? rawFileName
        if fileName.isEmpty || fileName == "/" {
            fileName = "document.md"
        } else if !(fileName as NSString).pathExtension.lowercased().contains("md") {
            fileName = "\(fileName).md"
        }
        
        Task { [weak self] in
            guard let self else { return }
            do {
                let resp = try await apiClient.fetchFileContent(
                    uri: cleanURI,
                    cascadeId: self.cascadeId,
                    baseURL: url
                )
                _ = try? DocumentCacheManager.shared.saveMarkdownToCache(
                    response: resp,
                    for: cleanURI,
                    fileName: fileName,
                    cascadeId: self.cascadeId
                )
            } catch {
                // Background prefetch error is non-fatal
            }
        }
    }
    
    @MainActor
    func updateProceedState(canProceed: Bool, artifactUri: String?) {
        self.canProceed = canProceed
        self.proceedArtifactUri = artifactUri
        if canProceed, let uri = artifactUri, !uri.isEmpty {
            self.prefetchMarkdownArtifact(uri: uri)
        }
    }
    
    @MainActor
    public func closeMarkdownViewer() {
        self.viewingMarkdownFile = nil
    }
    
    @MainActor
    public func proceedFromViewer() {
        self.viewingMarkdownFile = nil
        Task { [weak self] in
            await self?.proceedArtifact()
        }
    }
    
    @MainActor
    public func downloadAndPreviewDocument(uri: String, fileName: String, isHTML: Bool) {
        documentDownloadTask?.cancel()
        
        // 1. Fast-path: Check persistent local cache first
        if let cachedURL = DocumentCacheManager.shared.getCachedFile(for: uri, fileName: fileName) {
            if isHTML {
                self.htmlPreviewTitle = fileName
                self.htmlPreviewURL = cachedURL
            } else {
                self.quickLookTitle = fileName
                self.quickLookURL = cachedURL
            }
            return
        }
        
        self.isDownloadingDocument = true
        self.downloadingDocumentName = fileName
        self.downloadProgress = 0.0
        self.downloadBytesWritten = 0
        self.downloadBytesTotal = 0
        
        documentDownloadTask = Task { [weak self] in
            guard let self else { return }
            guard let url = self.settings.serverURL else {
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
                    self.quickLookTitle = resolvedName
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
                self.syncLiveActivity()
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
        let tasksToStop = self.runningTasks
        withAnimation(.spring(response: 0.35, dampingFraction: 0.8)) {
            self.runningTasks = []
        }
        stopPollingFallback()
        
        do {
            try await apiClient.cancelTask(cascadeId: cascadeId, baseURL: url)
            for task in tasksToStop {
                try? await apiClient.stopTask(cascadeId: cascadeId, stepIndex: task.stepIndex, taskId: task.id, baseURL: url)
            }
            if settings.enableLiveActivities {
                activityManager.endActivity(finalStatus: "CANCELLED")
            }
            await loadMessages(isBackgroundPoll: true)
        } catch {
            errorMessage = "取消任务失败: \(error.localizedDescription)"
        }
    }
    
}
