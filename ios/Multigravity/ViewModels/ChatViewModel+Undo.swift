import Foundation
import Observation
import UIKit
import SwiftUI

extension ChatViewModel {
    // MARK: - Undo / Revert Operations
    
    public func requestUndo(for message: ChatMessage) {
        guard let url = settings.serverURL, !cascadeId.isEmpty else { return }
        
        // Immediately present the confirm sheet so UI feedback is instant
        self.activeUndoMessage = message
        self.revertPreview = nil
        self.isLoadingRevertPreview = true
        self.showConfirmUndoSheet = true
        
        Task { @MainActor [weak self] in
            guard let self else { return }
            var targetMsg = message
            var stepIndex = targetMsg.effectiveStepIndex ?? self.extractStepIndex(from: targetMsg.id)
            
            if stepIndex == nil {
                await self.loadMessages()
                if let matched = self.messages.first(where: { $0.id == message.id || ($0.isUser && $0.content == message.content) }) {
                    targetMsg = matched
                    stepIndex = matched.effectiveStepIndex ?? self.extractStepIndex(from: matched.id)
                }
            }
            
            guard let validStepIndex = stepIndex else {
                self.isLoadingRevertPreview = false
                self.revertPreview = RevertPreviewResponse(
                    cascadeId: self.cascadeId,
                    stepIndex: 0,
                    targetStepIndex: 0,
                    files: [],
                    hasCodeChanges: false
                )
                return
            }
            
            if targetMsg.stepIndex != validStepIndex {
                targetMsg = ChatMessage(
                    id: targetMsg.id,
                    sender: targetMsg.sender,
                    content: targetMsg.content,
                    thinking: targetMsg.thinking,
                    toolCount: targetMsg.toolCount,
                    toolNames: targetMsg.toolNames,
                    imageDataList: targetMsg.imageDataList,
                    imageUrls: targetMsg.imageUrls,
                    stepIndex: validStepIndex
                )
                self.activeUndoMessage = targetMsg
            }
            
            do {
                let preview = try await apiClient.fetchRevertPreview(cascadeId: self.cascadeId, stepIndex: validStepIndex, baseURL: url)
                self.revertPreview = preview
                self.isLoadingRevertPreview = false
            } catch {
                self.isLoadingRevertPreview = false
                self.revertPreview = RevertPreviewResponse(
                    cascadeId: self.cascadeId,
                    stepIndex: validStepIndex,
                    targetStepIndex: max(-1, validStepIndex - 1),
                    files: [],
                    hasCodeChanges: false
                )
            }
        }
    }
    
    public func confirmUndo() {
        guard let message = activeUndoMessage, let url = settings.serverURL, !cascadeId.isEmpty else { return }
        let stepIndex = message.effectiveStepIndex ?? extractStepIndex(from: message.id) ?? 0
        let isFirstUserMessage = (stepIndex <= 0) || (message.id == self.messages.first(where: { $0.isUser })?.id)
        isReverting = true
        
        Task { @MainActor [weak self] in
            guard let self else { return }
            do {
                try await apiClient.executeRevert(cascadeId: self.cascadeId, stepIndex: stepIndex, conversationOnly: false, baseURL: url)
                
                self.inputText = message.content
                if !message.imageDataList.isEmpty {
                    self.selectedImageData = message.imageDataList
                }
                
                self.showConfirmUndoSheet = false
                self.isReverting = false
                self.activeUndoMessage = nil
                self.revertPreview = nil
                
                self.cacheManager.recordConversationTouch(cascadeId: self.cascadeId)
                self.cacheManager.recordDraftDate(key: self.cascadeId)
                self.saveCurrentDraft()
                
                if isFirstUserMessage {
                    withAnimation(.easeInOut(duration: 0.25)) {
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
                } else {
                    withAnimation(.easeInOut(duration: 0.2)) {
                        self.messages.removeAll(where: { msg in
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
                self.errorMessage = "撤回失败: \(error.localizedDescription)"
            }
        }
    }
}
