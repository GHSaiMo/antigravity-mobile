import Foundation
import Observation
import UIKit
import SwiftUI

extension ChatViewModel {
    // MARK: - Undo / Revert Operations
    
    public func cancelUndoSheet() {
        undoPreviewTask?.cancel()
        undoPreviewTask = nil
        isLoadingRevertPreview = false
        if !isReverting {
            showConfirmUndoSheet = false
            activeUndoMessage = nil
            revertPreview = nil
            undoErrorMessage = nil
        }
    }

    public func requestUndo(for message: ChatMessage) {
        guard let url = settings.serverURL, (!cascadeId.isEmpty || isSending) else { return }

        undoPreviewTask?.cancel()
        self.activeUndoMessage = message
        self.revertPreview = nil
        self.undoErrorMessage = nil
        self.isLoadingRevertPreview = true
        self.showConfirmUndoSheet = true

        let priorSnapshot = captureUndoPrecedingContext(for: message)

        undoPreviewTask = Task { @MainActor [weak self] in
            guard let self else { return }
            guard let activeCid = await self.waitForCascadeIdIfNeeded(maxWaitSeconds: 6.0), !Task.isCancelled else {
                if !Task.isCancelled {
                    self.isLoadingRevertPreview = false
                    self.revertPreview = RevertPreviewResponse(
                        cascadeId: self.cascadeId,
                        stepIndex: 0,
                        targetStepIndex: -1,
                        files: [],
                        hasCodeChanges: false
                    )
                }
                return
            }

            let resolved = await self.resolveServerUndoStepIndex(
                for: message,
                priorContext: priorSnapshot,
                allowReload: true
            )
            guard !Task.isCancelled else { return }

            guard let validStepIndex = resolved.stepIndex else {
                self.isLoadingRevertPreview = false
                self.revertPreview = RevertPreviewResponse(
                    cascadeId: activeCid,
                    stepIndex: max(0, priorSnapshot.minAllowedStep),
                    targetStepIndex: max(-1, priorSnapshot.minAllowedStep - 1),
                    files: [],
                    hasCodeChanges: false
                )
                return
            }

            let updatedTarget = resolved.matchedMessage ?? self.cloneMessageWithStepIndex(message, stepIndex: validStepIndex)
            self.activeUndoMessage = updatedTarget

            do {
                let preview = try await self.apiClient.fetchRevertPreview(cascadeId: activeCid, stepIndex: validStepIndex, baseURL: url)
                guard !Task.isCancelled else { return }
                self.revertPreview = preview
                self.isLoadingRevertPreview = false
            } catch {
                guard !Task.isCancelled else { return }
                self.isLoadingRevertPreview = false
                self.revertPreview = RevertPreviewResponse(
                    cascadeId: activeCid,
                    stepIndex: validStepIndex,
                    targetStepIndex: max(-1, validStepIndex - 1),
                    files: [],
                    hasCodeChanges: false
                )
            }
        }
    }

    public func confirmUndo() {
        guard let message = activeUndoMessage, let url = settings.serverURL, !isReverting else { return }
        AppReviewManager.shared.recordFriction()

        undoPreviewTask?.cancel()
        undoPreviewTask = nil
        isLoadingRevertPreview = false
        isReverting = true
        undoErrorMessage = nil

        let priorSnapshot = captureUndoPrecedingContext(for: message)

        Task { @MainActor [weak self] in
            guard let self else { return }

            let readyCascadeId = await self.waitForCascadeIdIfNeeded(maxWaitSeconds: 6.0)

            // If StartCascade failed before creating a server session, clean up the local optimistic bubble directly.
            if (readyCascadeId == nil || readyCascadeId?.isEmpty == true) && !self.isSending {
                self.restoreUndoneDraft(from: message)
                self.messages.removeAll { $0.id == message.id || $0.id.hasPrefix("optimistic-") }
                self.pendingOptimisticMessageId = nil
                self.isRunning = false
                self.isAwaitingResponse = false
                self.awaitingResponseSince = nil
                self.isReverting = false
                self.cancelUndoSheet()
                self.focusInputTrigger &+= 1
                return
            }

            guard let activeCid = readyCascadeId, !activeCid.isEmpty else {
                self.isReverting = false
                self.undoErrorMessage = String(localized: "会话仍在创建中，请稍等片刻再试")
                return
            }

            var resolved = await self.resolveServerUndoStepIndex(
                for: message,
                priorContext: priorSnapshot,
                allowReload: true
            )
            if resolved.stepIndex == nil && (self.isRunning || self.isSending || message.id.hasPrefix("optimistic-")) {
                try? await Task.sleep(nanoseconds: 400_000_000)
                resolved = await self.resolveServerUndoStepIndex(
                    for: message,
                    priorContext: priorSnapshot,
                    allowReload: true
                )
            }

            guard let stepIndex = resolved.stepIndex else {
                self.isReverting = false
                self.undoErrorMessage = String(localized: "消息刚发出尚未同步步骤编号，请稍等 1 秒后重试")
                return
            }

            let isFirstUserMessage = !priorSnapshot.hasPriorServerMessages && stepIndex <= 0

            do {
                try await self.apiClient.executeRevert(cascadeId: activeCid, stepIndex: stepIndex, conversationOnly: false, baseURL: url)

                self.restoreUndoneDraft(from: message)
                self.pendingOptimisticMessageId = nil
                self.isRunning = false
                self.isAwaitingResponse = false
                self.awaitingResponseSince = nil

                self.isReverting = false
                self.cancelUndoSheet()

                self.cacheManager.recordConversationTouch(cascadeId: activeCid)
                self.cacheManager.recordDraftDate(key: activeCid)
                self.saveCurrentDraft()

                if isFirstUserMessage {
                    withAnimation(.easeInOut(duration: 0.25)) {
                        self.messages = []
                        self.knownServerMessageIds = []
                        self.hasMore = false
                        self.nextOffset = 0
                        self.stepCount = 0
                        self.totalTools = 0
                        self.duration = String(localized: "0秒")
                        self.isNewConversation = true
                        self.currentTitle = self.emptyConversationTitle
                    }
                    self.saveEmptySessionToCache()
                } else {
                    withAnimation(.easeInOut(duration: 0.2)) {
                        self.messages.removeAll(where: { msg in
                            if msg.id == message.id || msg.id.hasPrefix("optimistic-") {
                                return true
                            }
                            if let idx = msg.effectiveStepIndex ?? self.extractStepIndex(from: msg.id) {
                                return idx >= stepIndex
                            }
                            return false
                        })
                    }
                }

                await self.loadMessages()
                self.focusInputTrigger &+= 1
            } catch {
                self.isReverting = false
                self.undoErrorMessage = String(localized: "撤回失败: \(error.localizedDescription)")
            }
        }
    }

    // MARK: - Private Undo Helpers

    private struct UndoPrecedingContext {
        let hasPriorServerMessages: Bool
        let minAllowedStep: Int
    }

    private func captureUndoPrecedingContext(for message: ChatMessage) -> UndoPrecedingContext {
        let msgIdx = messages.firstIndex(where: { $0.id == message.id })
        let precedingSlice: [ChatMessage] = {
            if let idx = msgIdx {
                return Array(messages[..<idx])
            }
            return messages.filter { $0.id != message.id }
        }()
        let priorServerMsgs = precedingSlice.filter { !$0.id.hasPrefix("optimistic-") }
        let maxPriorStep = priorServerMsgs.compactMap { $0.effectiveStepIndex ?? extractStepIndex(from: $0.id) }.max()
        return UndoPrecedingContext(
            hasPriorServerMessages: !priorServerMsgs.isEmpty,
            minAllowedStep: (maxPriorStep ?? -1) + 1
        )
    }

    @MainActor
    private func waitForCascadeIdIfNeeded(maxWaitSeconds: Double) async -> String? {
        if !cascadeId.isEmpty {
            return cascadeId
        }
        let maxTicks = max(1, Int(maxWaitSeconds * 10))
        for _ in 0..<maxTicks {
            if Task.isCancelled { return nil }
            if !cascadeId.isEmpty { return cascadeId }
            if !isSending { break }
            try? await Task.sleep(nanoseconds: 100_000_000)
        }
        return cascadeId.isEmpty ? nil : cascadeId
    }

    @MainActor
    private func resolveServerUndoStepIndex(
        for message: ChatMessage,
        priorContext: UndoPrecedingContext,
        allowReload: Bool
    ) async -> (stepIndex: Int?, matchedMessage: ChatMessage?) {
        if !message.id.hasPrefix("optimistic-"),
           let direct = message.effectiveStepIndex ?? extractStepIndex(from: message.id) {
            return (direct, message)
        }

        let targetTrimmed = message.content.trimmingCharacters(in: .whitespacesAndNewlines)
        let targetClientId = message.clientMessageId?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""

        func findCandidate() -> (Int, ChatMessage)? {
            let serverUsers = self.messages.filter { !$0.id.hasPrefix("optimistic-") && $0.isUser }
            if !targetClientId.isEmpty,
               let byClientId = serverUsers.last(where: { $0.clientMessageId == targetClientId }),
               let idx = byClientId.effectiveStepIndex ?? self.extractStepIndex(from: byClientId.id) {
                return (idx, byClientId)
            }
            if let byTailContent = serverUsers.last(where: { candidate in
                guard let idx = candidate.effectiveStepIndex ?? self.extractStepIndex(from: candidate.id),
                      idx >= priorContext.minAllowedStep else {
                    return false
                }
                return candidate.content.trimmingCharacters(in: .whitespacesAndNewlines) == targetTrimmed
            }), let idx = byTailContent.effectiveStepIndex ?? self.extractStepIndex(from: byTailContent.id) {
                return (idx, byTailContent)
            }
            return nil
        }

        if let found = findCandidate() {
            return (found.0, found.1)
        }

        if allowReload && !cascadeId.isEmpty {
            await self.loadMessages()
            if Task.isCancelled { return (nil, nil) }
            if let found = findCandidate() {
                return (found.0, found.1)
            }
        }

        // Safe fallback to step 0 ONLY when there are genuinely no prior server messages in this conversation.
        // Never fall back to 0 on multi-turn conversations, which would wipe all prior turns!
        if !priorContext.hasPriorServerMessages {
            return (0, nil)
        }
        return (nil, nil)
    }

    private func restoreUndoneDraft(from message: ChatMessage) {
        self.inputText = message.content
        if !message.imageDataList.isEmpty {
            self.selectedImageData = message.imageDataList
        }
    }

    private func cloneMessageWithStepIndex(_ msg: ChatMessage, stepIndex: Int) -> ChatMessage {
        guard msg.stepIndex != stepIndex else { return msg }
        return ChatMessage(
            id: msg.id,
            sender: msg.sender,
            content: msg.content,
            thinking: msg.thinking,
            toolCount: msg.toolCount,
            toolNames: msg.toolNames,
            imageDataList: msg.imageDataList,
            imageUrls: msg.imageUrls,
            artifacts: msg.artifacts,
            stepIndex: stepIndex,
            attemptCount: msg.attemptCount,
            maxAttempts: msg.maxAttempts,
            model: msg.model,
            modelName: msg.modelName,
            subagent: msg.subagent,
            workflow: msg.workflow,
            timestamp: msg.timestamp,
            slashCommands: msg.slashCommands,
            clientMessageId: msg.clientMessageId,
            errorKind: msg.errorKind,
            isOverflow: msg.isOverflow,
            thinkingDuration: msg.thinkingDuration,
            thinkingBlocks: msg.thinkingBlocks,
            toolDetails: msg.toolDetails
        )
    }
}
