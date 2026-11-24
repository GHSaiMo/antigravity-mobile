import Foundation
import SwiftUI

extension Notification.Name {
    public static let openConversationById = Notification.Name("multigravity.openConversationById")
    public static let permissionPresetDidChange = Notification.Name("multigravity.permissionPresetDidChange")
    public static let btwInsertedIntoParentDraft = Notification.Name("multigravity.btwInsertedIntoParentDraft")
}

extension ChatViewModel {
    // MARK: - Phase 3: Permissions

    @MainActor
    public func refreshPermissionSettings() async {
        guard let url = settings.gatewayURL else { return }
        if let state = try? await apiClient.fetchPermissionSettings(baseURL: url) {
            permissionState = state
            settings.cachedPermissionPreset = state.preset
        }
    }

    @MainActor
    public func updatePermissionPreset(_ preset: String) async {
        guard let url = settings.gatewayURL else { return }
        do {
            let state = try await apiClient.updatePermissionPreset(preset: preset, baseURL: url)
            permissionState = state
            settings.cachedPermissionPreset = state.preset
            NotificationCenter.default.post(name: .permissionPresetDidChange, object: state)
        } catch {
            errorMessage = String(localized: "权限模式切换失败: \(error.localizedDescription)")
        }
    }

    // MARK: - Phase 3: @ Context Mentions

    @MainActor
    public var effectiveWorkspaceUri: String? {
        if let workspaceUri, !workspaceUri.isEmpty { return workspaceUri }
        if let uri = draftProject?.uri, !uri.isEmpty { return uri }
        if let uri = draftSession?.project.uri, !uri.isEmpty { return uri }
        return nil
    }

    @MainActor
    public func openMentionPicker(query: String = "") {
        showMentionPicker = true
        mentionQuery = query
        scheduleMentionSearch(query: query)
    }

    @MainActor
    public func closeMentionPicker() {
        showMentionPicker = false
        mentionSearchTask?.cancel()
        mentionSearchTask = nil
    }

    @MainActor
    public func scheduleMentionSearch(query: String) {
        mentionQuery = query
        mentionSearchTask?.cancel()
        mentionSearchTask = Task { [weak self] in
            try? await Task.sleep(nanoseconds: 150_000_000)
            guard let self, !Task.isCancelled else { return }
            await self.fetchMentionCandidates(query: query)
        }
    }

    @MainActor
    public func fetchMentionCandidates(query: String) async {
        guard let url = settings.gatewayURL else { return }
        isLoadingMentions = true
        defer { isLoadingMentions = false }
        do {
            let items = try await apiClient.fetchContextMentions(
                query: query,
                cascadeId: cascadeId,
                workspaceUri: effectiveWorkspaceUri,
                baseURL: url
            )
            mentionCandidates = items
        } catch {
            mentionCandidates = []
        }
    }

    @MainActor
    public func selectMentionItem(_ item: ContextMentionItem) {
        if !selectedContextItems.contains(where: { $0.id == item.id && $0.label == item.label }) {
            selectedContextItems.append(item)
        }
        // Remove trailing "@..." token from inputText when triggered via inline typing
        if let regex = try? NSRegularExpression(pattern: "(?:^|\\s)@[^\\s@]*$", options: []) {
            let ns = inputText as NSString
            let range = NSRange(location: 0, length: ns.length)
            if let match = regex.firstMatch(in: inputText, options: [], range: range) {
                let matchedStr = ns.substring(with: match.range)
                let leadingSpace = matchedStr.hasPrefix(" ") || matchedStr.hasPrefix("\n") ? String(matchedStr.prefix(1)) : ""
                inputText = ns.replacingCharacters(in: match.range, with: leadingSpace)
            }
        }
        closeMentionPicker()
    }

    @MainActor
    public func removeContextItem(id: String) {
        selectedContextItems.removeAll(where: { $0.id == id })
    }

    // MARK: - Phase 3: Fork Conversation & Background Tasks

    @MainActor
    public func forkConversation(at stepIndex: Int, createWorktree: Bool = false) async {
        guard let url = settings.gatewayURL, !cascadeId.isEmpty, !isForkingConversation else { return }
        isForkingConversation = true
        defer { isForkingConversation = false }
        do {
            let resp = try await apiClient.forkConversation(
                cascadeId: cascadeId,
                stepIndex: stepIndex,
                createWorktree: createWorktree,
                baseURL: url
            )
            if let newId = resp.cascadeId, !newId.isEmpty {
                UIImpactFeedbackGenerator(style: .medium).impactOccurred()
                NotificationCenter.default.post(name: .openConversationById, object: newId)
            } else {
                errorMessage = resp.error ?? String(localized: "分叉会话失败")
            }
        } catch {
            errorMessage = String(localized: "分叉会话失败: \(error.localizedDescription)")
        }
    }

    @MainActor
    public func sendTaskToBackground(stepIndices: [Int]) async {
        guard let url = settings.gatewayURL, !cascadeId.isEmpty else { return }
        do {
            _ = try await apiClient.sendStepsToBackground(cascadeId: cascadeId, stepIndices: stepIndices, baseURL: url)
            await loadMessages(isBackgroundPoll: true)
        } catch {
            errorMessage = String(localized: "转入后台失败: \(error.localizedDescription)")
        }
    }

    @MainActor
    public func refreshTerminals() async {
        guard let url = settings.gatewayURL else { return }
        if let list = try? await apiClient.listTerminals(baseURL: url) {
            activeTerminals = list
        }
    }

    @MainActor
    public func refreshProjectTrust() async {
        guard let url = settings.gatewayURL, !isDraftConversation, !cascadeId.isEmpty else { return }
        if let res = try? await apiClient.fetchProjectTrustStatus(cascadeId: cascadeId, baseURL: url) {
            trustStatus = res
        }
    }

    @MainActor
    public func trustCurrentProject() async {
        guard let url = settings.gatewayURL, !isUpdatingTrust else { return }
        isUpdatingTrust = true
        defer { isUpdatingTrust = false }
        do {
            trustStatus = try await apiClient.setWorkspaceTrust(
                cascadeId: cascadeId.isEmpty ? nil : cascadeId,
                projectId: trustStatus?.projectId,
                trusted: true,
                baseURL: url
            )
            UIImpactFeedbackGenerator(style: .medium).impactOccurred()
        } catch {
            errorMessage = String(localized: "工作区信任授权失败: \(error.localizedDescription)")
        }
    }

    @MainActor
    public func refreshBtwList() async {
        guard let url = settings.gatewayURL, !isDraftConversation, !cascadeId.isEmpty else { return }
        if let list = try? await apiClient.listBtwSideQuestions(cascadeId: cascadeId, baseURL: url) {
            btwEntries = list
        }
    }

    /// 发起旁路快问分支（可选 stepIndex 精准继承至该条消息及之前上下文），创建后立即进入完整分支会话。
    @MainActor
    public func startBtwAndOpen(question: String, stepIndex: Int? = nil, quote: String? = nil) async -> Bool {
        let trimmedQ = question.trimmingCharacters(in: .whitespacesAndNewlines)
        let trimmedQuote = quote?.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmedQ.isEmpty || !(trimmedQuote ?? "").isEmpty else { return false }
        guard let url = settings.gatewayURL, !isDraftConversation, !cascadeId.isEmpty else {
            errorMessage = String(localized: "请先在主会话发送至少一条消息后再发起旁路快问")
            return false
        }
        guard !isStartingBtw else { return false }
        isStartingBtw = true
        defer { isStartingBtw = false }

        do {
            let requestedModel = activeModelEnum.isEmpty ? activeModel : activeModelEnum
            let res = try await apiClient.startBtwSideQuestion(
                parentCascadeId: cascadeId,
                question: trimmedQ,
                quote: (trimmedQuote?.isEmpty == false) ? trimmedQuote : nil,
                stepIndex: stepIndex,
                model: requestedModel,
                baseURL: url
            )
            let resolvedTitle = (res.title?.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty == false)
                ? res.title!
                : ConversationItem.sanitizeTitle(trimmedQ.isEmpty ? (trimmedQuote ?? String(localized: "旁路快问")) : trimmedQ)
            let effectiveForkStep = res.forkStepIndex ?? stepIndex ?? max(0, stepCount - 1)
            let newItem = ConversationItem(
                id: res.cascadeId,
                title: resolvedTitle,
                status: .running,
                stepCount: 1,
                workspaceName: workspaceName,
                lastModified: Date(),
                isSubagent: false,
                isUnread: false,
                isSideQuestion: true,
                branchParentId: cascadeId,
                branchParentTitle: currentTitle,
                forkStepIndex: effectiveForkStep
            )
            cacheManager.upsertConversation(newItem)
            NotificationCenter.default.post(name: .conversationUpserted, object: newItem)

            let anchorId: String? = {
                if let si = res.forkStepIndex ?? stepIndex {
                    if let match = messages.last(where: { msg in
                        guard let msgStep = msg.effectiveStepIndex else { return false }
                        return msgStep <= si && (msg.isAgent || msg.isUser)
                    }) ?? messages.last(where: { msg in
                        guard let msgStep = msg.effectiveStepIndex else { return false }
                        return msgStep <= si
                    }) {
                        return match.id
                    }
                }
                return messages.last(where: { $0.isAgent || $0.isUser })?.id ?? messages.last?.id
            }()
            let optimisticCard = SideQuestionCardItem(
                cascadeId: res.cascadeId,
                parentCascadeId: cascadeId,
                title: resolvedTitle,
                question: trimmedQ,
                answerPreview: nil,
                status: "CASCADE_RUN_STATUS_RUNNING",
                forkStepIndex: effectiveForkStep,
                anchorMessageId: anchorId,
                turnCount: 1,
                createdAt: res.createdAt
            )
            if !sideQuestions.contains(where: { $0.cascadeId == res.cascadeId }) {
                sideQuestions.append(optimisticCard)
            }
            UIImpactFeedbackGenerator(style: .medium).impactOccurred()
            NotificationCenter.default.post(name: .openConversationById, object: res.cascadeId)
            return true
        } catch {
            errorMessage = String(localized: "发起旁路快问失败: \(error.localizedDescription)")
            return false
        }
    }

    /// 在旁路分支内一键将回复结论插入父会话草稿，并切回父会话聚焦输入栏。
    @MainActor
    public func insertConclusionIntoParent(text: String) {
        guard let parentId = branchOrigin?.parentCascadeId.trimmingCharacters(in: .whitespacesAndNewlines),
              !parentId.isEmpty else { return }
        let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return }
        let snippet = String(trimmed.prefix(1600))
        let existing = cacheManager.getDraft(for: parentId).trimmingCharacters(in: .whitespacesAndNewlines)
        let merged = existing.isEmpty ? snippet : (existing + "\n\n" + snippet)
        cacheManager.saveDraft(key: parentId, text: merged)
        cacheManager.recordDraftDate(key: parentId)
        NotificationCenter.default.post(name: .conversationDraftChanged, object: nil)
        NotificationCenter.default.post(
            name: .btwInsertedIntoParentDraft,
            object: nil,
            userInfo: ["cascadeId": parentId, "text": merged]
        )
        UIImpactFeedbackGenerator(style: .medium).impactOccurred()
        NotificationCenter.default.post(name: .openConversationById, object: parentId)
    }
}
