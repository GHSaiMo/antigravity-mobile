import Foundation
import SwiftUI

extension Notification.Name {
    public static let openConversationById = Notification.Name("multigravity.openConversationById")
    public static let permissionPresetDidChange = Notification.Name("multigravity.permissionPresetDidChange")
}

extension ChatViewModel {
    // MARK: - Phase 3: Permissions

    @MainActor
    public func refreshPermissionSettings() async {
        guard let url = settings.gatewayURL else { return }
        if let state = try? await apiClient.fetchPermissionSettings(baseURL: url) {
            permissionState = state
        }
    }

    @MainActor
    public func updatePermissionPreset(_ preset: String) async {
        guard let url = settings.gatewayURL else { return }
        do {
            let state = try await apiClient.updatePermissionPreset(preset: preset, baseURL: url)
            permissionState = state
            NotificationCenter.default.post(name: .permissionPresetDidChange, object: state)
        } catch {
            errorMessage = "权限模式切换失败: \(error.localizedDescription)"
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
                errorMessage = resp.error ?? "分叉会话失败"
            }
        } catch {
            errorMessage = "分叉会话失败: \(error.localizedDescription)"
        }
    }

    @MainActor
    public func sendTaskToBackground(stepIndices: [Int]) async {
        guard let url = settings.gatewayURL, !cascadeId.isEmpty else { return }
        do {
            _ = try await apiClient.sendStepsToBackground(cascadeId: cascadeId, stepIndices: stepIndices, baseURL: url)
            await loadMessages(isBackgroundPoll: true)
        } catch {
            errorMessage = "转入后台失败: \(error.localizedDescription)"
        }
    }

    @MainActor
    public func refreshTerminals() async {
        guard let url = settings.gatewayURL else { return }
        if let list = try? await apiClient.listTerminals(baseURL: url) {
            activeTerminals = list
        }
    }
}
