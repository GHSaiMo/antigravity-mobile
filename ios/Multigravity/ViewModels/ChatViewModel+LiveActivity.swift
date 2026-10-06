import Foundation
import Observation
import UIKit
import SwiftUI

extension ChatViewModel {
    // MARK: - Live Activity Sync
    
    @MainActor
    public func syncLiveActivity() {
        guard settings.enableLiveActivities, !cascadeId.isEmpty else { return }
        
        let isAgentBusy = self.isRunning || self.isAwaitingResponse
        let hasRunningTasks = !self.runningTasks.isEmpty
        let hasPendingAction = self.canProceed || self.pendingInteraction != nil
        let shouldBeActive = isAgentBusy || hasRunningTasks || hasPendingAction
        
        if shouldBeActive {
            let primaryTask = self.runningTasks.first
            let taskTitle = primaryTask?.toolSummary ?? primaryTask?.toolAction ?? primaryTask?.toolName
            let taskCommand = primaryTask?.commandLine
            
            let status: String
            if hasPendingAction {
                status = "WAITING_APPROVAL"
            } else if hasRunningTasks && !isAgentBusy {
                status = "TASK_RUNNING"
            } else {
                status = "RUNNING"
            }
            
            var actionText = ""
            if hasPendingAction {
                actionText = "等待用户审批操作"
            } else if let title = taskTitle, !title.isEmpty {
                actionText = title
            } else if let lastMsg = self.messages.last, !lastMsg.content.isEmpty {
                actionText = lastMsg.content
            } else if isAgentBusy {
                actionText = "Agent 正在执行..."
            } else {
                actionText = "任务运行中..."
            }
            
            let taskSnapshots: [AgentTaskSnapshot] = self.runningTasks.prefix(3).map { task in
                let title = task.toolSummary ?? task.toolAction ?? task.toolName ?? "后台任务"
                let cmd = task.commandLine.trimmingCharacters(in: .whitespacesAndNewlines)
                return AgentTaskSnapshot(
                    id: task.id,
                    title: title,
                    command: cmd.isEmpty ? nil : cmd,
                    isWaiting: false
                )
            }
            
            if activityManager.hasActiveActivity && activityManager.currentCascadeId == cascadeId {
                activityManager.updateActivity(
                    title: currentTitle,
                    status: status,
                    stepCount: self.stepCount,
                    latestAction: actionText,
                    runningTaskCount: self.runningTasks.count,
                    runningTasks: taskSnapshots,
                    activeTaskTitle: taskTitle,
                    activeTaskCommand: taskCommand,
                    hasPendingAction: hasPendingAction
                )
            } else {
                activityManager.startActivity(
                    title: currentTitle,
                    cascadeId: cascadeId,
                    status: status,
                    stepCount: self.stepCount,
                    latestAction: actionText,
                    runningTaskCount: self.runningTasks.count,
                    runningTasks: taskSnapshots,
                    activeTaskTitle: taskTitle,
                    activeTaskCommand: taskCommand,
                    hasPendingAction: hasPendingAction
                )
            }
        } else {
            if activityManager.hasActiveActivity && activityManager.currentCascadeId == cascadeId {
                let finalStatus = self.hasErrorState ? "FAILED" : "COMPLETED"
                activityManager.endActivity(finalStatus: finalStatus)
            }
        }
    }
    
    public func disconnectStream() {
        streamClient.disconnect(intentional: true)
        stopPollingFallback()
    }
    
    public func stopPolling() {
        disconnectStream()
    }
    
    func startPollingFallback() {
        guard pollTask == nil else { return }
        pollTask = Task { [weak self] in
            while !Task.isCancelled {
                guard let self else { break }
                let interval: UInt64 = (self.isRunning || self.isAwaitingResponse) ? 2_500_000_000 : 6_000_000_000
                try? await Task.sleep(nanoseconds: interval)
                guard !Task.isCancelled else { break }
                await self.loadMessages(isBackgroundPoll: true)
            }
        }
    }
    
    func stopPollingFallback() {
        pollTask?.cancel()
        pollTask = nil
    }
    
}
