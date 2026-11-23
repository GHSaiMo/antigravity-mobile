import Foundation
import Observation
import UIKit
import SwiftUI

extension ChatViewModel {
    // MARK: - Live Activity Sync
    
    @MainActor
    public func syncLiveActivity() {
        // Subagents share the parent conversation's Live Activity and never spawn separate cards.
        guard settings.enableLiveActivities, !cascadeId.isEmpty, parentConversationId == nil else { return }
        
        let isAgentBusy = self.isRunning || self.isAwaitingResponse
        // Standing daemons (IsDaemon=true) never finish on their own; keep them out of the Live Activity
        // so it can end once the turn completes.
        let blockingTasks = self.runningTasks.blockingTasks
        let hasRunningTasks = !blockingTasks.isEmpty
        let hasPendingAction = self.canProceed || self.pendingInteraction != nil
        let shouldBeActive = isAgentBusy || hasRunningTasks || hasPendingAction
        
        if shouldBeActive {
            let primaryTask = blockingTasks.first
            let taskTitle: String? = primaryTask.map { task in
                task.toolSummary ?? task.toolAction ?? ToolLocalization.localizedName(for: task.toolName)
            }
            let taskCommand = primaryTask?.commandLine
            
            let status: String
            if hasPendingAction {
                status = "WAITING_APPROVAL"
            } else if hasRunningTasks && !isAgentBusy {
                status = "TASK_RUNNING"
            } else {
                status = "RUNNING"
            }
            
            let pendingStepIndex: Int? = self.pendingInteraction?.stepIndex ?? (self.canProceed ? self.stepCount : nil)
            let pendingActionKind: String? = self.pendingInteraction?.type ?? (self.canProceed ? "proceed" : nil)

            var actionText = ""
            if let interaction = self.pendingInteraction {
                let target = interaction.target?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
                let title = interaction.title.trimmingCharacters(in: .whitespacesAndNewlines)
                let summary = !target.isEmpty ? target : title
                actionText = summary.isEmpty ? String(localized: "等待用户审批操作") : String(localized: "待审批: \(summary)")
            } else if self.canProceed {
                actionText = String(localized: "方案待确认，可点击 Proceed")
            } else if let title = taskTitle, !title.isEmpty {
                actionText = title
            } else if let planOrFocus = liveActivityPlanOrFocusSummary(), !planOrFocus.isEmpty {
                actionText = planOrFocus
            } else if let lastMsg = self.messages.last, !lastMsg.content.isEmpty {
                let trimmed = lastMsg.content.trimmingCharacters(in: .whitespacesAndNewlines)
                actionText = trimmed.count > 500 ? String(trimmed.prefix(500)) : trimmed
            } else if isAgentBusy {
                actionText = String(localized: "Agent 正在执行...")
            } else {
                actionText = String(localized: "任务运行中...")
            }
            
            let taskSnapshots: [AgentTaskSnapshot] = blockingTasks.prefix(3).map { task in
                let title = task.toolSummary ?? task.toolAction ?? ToolLocalization.localizedName(for: task.toolName)
                let cmd = task.commandLine.trimmingCharacters(in: .whitespacesAndNewlines)
                return AgentTaskSnapshot(
                    id: task.id,
                    title: title,
                    command: cmd.isEmpty ? nil : cmd,
                    isWaiting: false
                )
            }
            
            if activityManager.hasActiveActivity(for: cascadeId) {
                activityManager.updateActivity(
                    cascadeId: cascadeId,
                    title: currentTitle,
                    status: status,
                    stepCount: self.stepCount,
                    latestAction: actionText,
                    runningTaskCount: blockingTasks.count,
                    runningTasks: taskSnapshots,
                    activeTaskTitle: taskTitle,
                    activeTaskCommand: taskCommand,
                    hasPendingAction: hasPendingAction,
                    pendingStepIndex: pendingStepIndex,
                    pendingActionKind: pendingActionKind
                )
            } else {
                activityManager.startActivity(
                    title: currentTitle,
                    cascadeId: cascadeId,
                    status: status,
                    stepCount: self.stepCount,
                    latestAction: actionText,
                    runningTaskCount: blockingTasks.count,
                    runningTasks: taskSnapshots,
                    activeTaskTitle: taskTitle,
                    activeTaskCommand: taskCommand,
                    hasPendingAction: hasPendingAction,
                    pendingStepIndex: pendingStepIndex,
                    pendingActionKind: pendingActionKind
                )
            }
        } else {
            if activityManager.hasActiveActivity(for: cascadeId) {
                let finalStatus = self.hasErrorState ? "FAILED" : "COMPLETED"
                activityManager.endActivity(for: cascadeId, finalStatus: finalStatus)
            }
        }
    }
    
    @MainActor
    private func liveActivityPlanOrFocusSummary() -> String? {
        let focus = activeFocus?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        if let plan = activePlan, !plan.items.isEmpty {
            let prefix = plan.totalCount > 0 ? "\(plan.doneCount)/\(plan.totalCount)" : ""
            if !focus.isEmpty {
                return prefix.isEmpty ? focus : "\(prefix) · \(focus)"
            }
            if let current = plan.items.first(where: { $0.status == "in_progress" })?.title.trimmingCharacters(in: .whitespacesAndNewlines),
               !current.isEmpty {
                return prefix.isEmpty ? current : "\(prefix) · \(current)"
            }
        }
        return focus.isEmpty ? nil : focus
    }

    @MainActor
    func endBackgroundGraceTask() {
        backgroundDisconnectTask?.cancel()
        backgroundDisconnectTask = nil
        if backgroundTaskId != .invalid {
            UIApplication.shared.endBackgroundTask(backgroundTaskId)
            backgroundTaskId = .invalid
        }
    }

    @MainActor
    public func disconnectStream() {
        endBackgroundGraceTask()
        streamClient.disconnect(intentional: true)
        stopPollingFallback()
    }
    
    @MainActor
    public func stopPolling() {
        disconnectStream()
    }
    
    func startPollingFallback() {
        guard pollTask == nil else { return }
        pollTask = Task { [weak self] in
            var idleTicks = 0
            while !Task.isCancelled {
                guard let self else { break }
                if self.streamClient.status == .connected && !DemoGateway.isEnabled {
                    self.pollTask = nil
                    break
                }
                let isBusy = self.isRunning || self.isAwaitingResponse || !self.runningTasks.blockingTasks.isEmpty
                let intervalSec: UInt64
                if isBusy {
                    idleTicks = 0
                    intervalSec = 3
                } else {
                    idleTicks += 1
                    switch idleTicks {
                    case 1...2: intervalSec = 6
                    case 3...5: intervalSec = 15
                    case 6...10: intervalSec = 30
                    default: intervalSec = 60
                    }
                }
                try? await Task.sleep(nanoseconds: intervalSec * 1_000_000_000)
                guard !Task.isCancelled else { break }
                if self.streamClient.status == .connected && !DemoGateway.isEnabled {
                    self.pollTask = nil
                    break
                }
                await self.loadMessages(isBackgroundPoll: true)
            }
        }
    }
    
    func stopPollingFallback() {
        pollTask?.cancel()
        pollTask = nil
    }
    
}
