import Foundation
import Observation
import UIKit
import SwiftUI

extension ChatViewModel {
    // MARK: - Message Sequencing and Self-Healing
    
    /// Extracts a numeric step index from a message (priority: effectiveStepIndex, then ID string prefix).
    func extractStepIndex(from message: ChatMessage) -> Int? {
        message.effectiveStepIndex ?? extractStepIndex(from: message.id)
    }
    
    /// Extracts a numeric step index from a message ID (e.g. "step-12" -> 12).
    func extractStepIndex(from id: String) -> Int? {
        if id.hasPrefix("step-"), let val = Int(id.dropFirst(5)) {
            return val
        }
        return nil
    }
    
    /// Detects and self-heals inversion anomalies (e.g. latest turn appearing before earliest turn).
    func sanitizeMessageOrder(_ list: [ChatMessage]) -> [ChatMessage] {
        guard list.count >= 2 else { return list }
        
        // Find if there is a severe step drop point (e.g. index 10 has step-25 and index 11 has step-0)
        var dropIndex: Int? = nil
        var prevStep = -1
        
        for (i, msg) in list.enumerated() {
            if let step = extractStepIndex(from: msg) {
                if prevStep != -1 && step < prevStep && (prevStep - step) >= 2 {
                    // Sudden backwards jump in step index detected!
                    dropIndex = i
                    break
                }
                prevStep = step
            }
        }
        
        let ordered: [ChatMessage]
        if let drop = dropIndex {
            // Segment 1 (0..<drop) was placed at top (latest messages)
            // Segment 2 (drop..<count) was appended at bottom (earliest messages)
            let head = Array(list[0..<drop])
            let tail = Array(list[drop...])
            
            // Re-swap: earlier messages should come first
            ordered = tail + head
        } else {
            ordered = list
        }
        
        return collapseAttemptErrors(ordered)
    }
    
    /// Collapses consecutive attempt errors into a single error card (e.g. attempt 1..3 -> attempt 3/9 · 已重试 3 次).
    func collapseAttemptErrors(_ list: [ChatMessage]) -> [ChatMessage] {
        guard list.count >= 2 else { return list }
        var result: [ChatMessage] = []
        
        for msg in list {
            if msg.isError, let last = result.last, last.isError {
                let parsed = APIClient.parseAttemptError(msg.content)
                let prevParsed = APIClient.parseAttemptError(last.content)
                
                // Case 1: Both are attempt-based errors with matching base error
                if parsed.isAttempt && prevParsed.isAttempt && APIClient.normalizeErrKey(prevParsed.baseError) == APIClient.normalizeErrKey(parsed.baseError) {
                    let newAttempt = max(parsed.attempt, (last.attemptCount ?? prevParsed.attempt) + 1)
                    let maxAttempts = parsed.maxAttempts > 0 ? parsed.maxAttempts : (last.maxAttempts ?? 9)
                    let formatted = "\(parsed.prefix) (attempt \(newAttempt)/\(maxAttempts) · 已重试 \(newAttempt) 次): \(parsed.baseError)"
                    result[result.count - 1] = ChatMessage(
                        id: last.id,
                        sender: .error,
                        content: formatted,
                        stepIndex: msg.stepIndex ?? last.stepIndex,
                        attemptCount: newAttempt,
                        maxAttempts: maxAttempts
                    )
                    continue
                }
                
                // Case 2: Curr is attempt error, but prev was exact same base error without attempt marker yet
                if parsed.isAttempt && APIClient.normalizeErrKey(last.content) == APIClient.normalizeErrKey(parsed.baseError) {
                    let newAttempt = max(parsed.attempt, (last.attemptCount ?? 1) + 1)
                    let maxAttempts = parsed.maxAttempts > 0 ? parsed.maxAttempts : 9
                    let formatted = "\(parsed.prefix) (attempt \(newAttempt)/\(maxAttempts) · 已重试 \(newAttempt) 次): \(parsed.baseError)"
                    result[result.count - 1] = ChatMessage(
                        id: last.id,
                        sender: .error,
                        content: formatted,
                        stepIndex: msg.stepIndex ?? last.stepIndex,
                        attemptCount: newAttempt,
                        maxAttempts: maxAttempts
                    )
                    continue
                }
                
                // Case 3: Both are non-attempt errors, but have identical normalized content (consecutive duplicate errors)
                if !parsed.isAttempt && !prevParsed.isAttempt && APIClient.normalizeErrKey(last.content) == APIClient.normalizeErrKey(msg.content) {
                    let count = (last.attemptCount ?? 1) + 1
                    let formatted = "\(msg.content) (已重试 \(count) 次)"
                    result[result.count - 1] = ChatMessage(
                        id: last.id,
                        sender: .error,
                        content: formatted,
                        stepIndex: msg.stepIndex ?? last.stepIndex,
                        attemptCount: count,
                        maxAttempts: nil
                    )
                    continue
                }
            }
            result.append(msg)
        }
        return result
    }
    
    /// Applies a full trajectory snapshot directly without destructive slicing or reverse appends.
    @MainActor
    func applySnapshotMessages(_ incoming: [ChatMessage], hasMore: Bool = false, nextOffset: Int = 0) {
        guard !incoming.isEmpty else {
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
            return
        }
        
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
        let hasEarliest = fullList.contains(where: { self.extractStepIndex(from: $0) == 0 })
        self.hasMore = hasEarliest ? false : hasMore
        self.nextOffset = hasEarliest ? 0 : nextOffset
        if self.pendingOptimisticMessageId == nil {
            self.knownServerMessageIds = Set(fullList.map(\.id))
        }
    }
    
    func mergeIncomingMessages(_ incoming: [ChatMessage]) {
        guard !incoming.isEmpty else {
            if self.pendingOptimisticMessageId == nil {
                self.messages = []
                self.knownServerMessageIds = []
                self.hasMore = false
                self.nextOffset = 0
            }
            return
        }
        
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
            // Find overlap between incoming and base using multi-dimensional matching:
            // 1. Exact ID match
            // 2. Non-nil effectiveStepIndex match with identical sender
            // 3. User message identical content match
            var firstBaseMatchIdx: Int? = nil
            var incomingMatchIdxForFirstBaseMatch: Int? = nil
            
            for (baseIdx, baseMsg) in base.enumerated() {
                if let incIdx = incoming.firstIndex(where: { inc in
                    if inc.id == baseMsg.id { return true }
                    if let bs = self.extractStepIndex(from: baseMsg),
                       let is_ = self.extractStepIndex(from: inc),
                       bs == is_ && baseMsg.sender == inc.sender {
                        return true
                    }
                    if baseMsg.isUser && inc.isUser &&
                       !baseMsg.content.isEmpty && baseMsg.content == inc.content {
                        return true
                    }
                    return false
                }) {
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
                let baseHasStart = base.contains(where: { self.extractStepIndex(from: $0) == 0 })
                let incomingHasStart = incoming.contains(where: { self.extractStepIndex(from: $0) == 0 })
                
                if baseHasStart && incomingHasStart {
                    // Both base and incoming represent the conversation from the beginning (step 0).
                    // Never concatenate them end-to-end, which would duplicate the entire conversation.
                    // Prefer incoming as the authoritative server snapshot.
                    base = incoming
                } else {
                    // Determine ordering based on step indexes
                    let baseStep = base.compactMap { self.extractStepIndex(from: $0) }.first
                    let incomingStep = incoming.compactMap { self.extractStepIndex(from: $0) }.first
                    
                    if let bStep = baseStep, let iStep = incomingStep, iStep < bStep {
                        // incoming contains earlier steps than base
                        base = incoming + base
                    } else if baseStep != nil || incomingStep != nil {
                        // incoming contains later steps than base
                        base = base + incoming
                    } else {
                        // Neither base nor incoming have step indices; default to incoming snapshot
                        base = incoming
                    }
                }
            }
        }
        
        // 3. If server has NOT yet acknowledged the user message, KEEP IT AT THE END!
        if let opt = optimisticMessage {
            base.append(opt)
        }
        
        // 4. Sanitize ordering in case of anomalies
        base = sanitizeMessageOrder(base)
        
        // 5. Final safety deduplication: ensure no duplicate IDs or identical (stepIndex, sender) pairs exist
        var seenIds = Set<String>()
        var seenSteps = Set<Int>()
        base = base.filter { msg in
            if !seenIds.insert(msg.id).inserted {
                return false
            }
            if let step = self.extractStepIndex(from: msg), step >= 0 {
                if msg.isUser || msg.isAgent {
                    return seenSteps.insert(step).inserted
                }
            }
            return true
        }
        
        self.messages = base
        if self.pendingOptimisticMessageId == nil {
            self.knownServerMessageIds = Set(base.map(\.id))
        }
    }
    
}
