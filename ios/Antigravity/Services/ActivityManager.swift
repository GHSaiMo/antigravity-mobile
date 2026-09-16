import Foundation
import ActivityKit

@MainActor
public final class ActivityManager {
    public static let shared = ActivityManager()
    
    private var currentActivity: Activity<AgentActivityAttributes>?
    
    public init() {}
    
    public var isLiveActivityEnabled: Bool {
        ActivityAuthorizationInfo().areActivitiesEnabled
    }
    
    public var currentCascadeId: String? {
        currentActivity?.attributes.cascadeId
    }
    
    public var hasActiveActivity: Bool {
        currentActivity != nil
    }
    
    public func startActivity(
        title: String,
        cascadeId: String,
        status: String = "RUNNING",
        stepCount: Int = 1,
        latestAction: String = "开始执行任务...",
        runningTaskCount: Int = 0,
        activeTaskTitle: String? = nil,
        activeTaskCommand: String? = nil,
        hasPendingAction: Bool = false
    ) {
        guard isLiveActivityEnabled else { return }
        
        // If current activity is for the same conversation, update in-place to avoid flicker
        if let current = currentActivity, current.attributes.cascadeId == cascadeId {
            updateActivity(
                status: status,
                stepCount: stepCount,
                latestAction: latestAction,
                runningTaskCount: runningTaskCount,
                activeTaskTitle: activeTaskTitle,
                activeTaskCommand: activeTaskCommand,
                hasPendingAction: hasPendingAction
            )
            return
        }
        
        // End any existing activity first
        if currentActivity != nil {
            endActivity()
        }
        
        let attributes = AgentActivityAttributes(conversationTitle: title, cascadeId: cascadeId)
        let initialContentState = AgentActivityAttributes.ContentState(
            status: status,
            stepCount: max(1, stepCount),
            latestAction: latestAction.isEmpty ? (runningTaskCount > 0 ? "正在执行后台任务..." : "正在执行...") : latestAction,
            runningTaskCount: runningTaskCount,
            activeTaskTitle: activeTaskTitle,
            activeTaskCommand: activeTaskCommand,
            hasPendingAction: hasPendingAction,
            lastUpdated: Date()
        )
        
        do {
            let activity = try Activity.request(
                attributes: attributes,
                content: .init(state: initialContentState, staleDate: nil)
            )
            self.currentActivity = activity
        } catch {
            print("[LiveActivity] Failed to start: \(error)")
        }
    }
    
    public func updateActivity(
        status: String,
        stepCount: Int,
        latestAction: String,
        runningTaskCount: Int = 0,
        activeTaskTitle: String? = nil,
        activeTaskCommand: String? = nil,
        hasPendingAction: Bool = false
    ) {
        guard let activity = currentActivity else { return }
        
        let actionText: String
        if !latestAction.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
            actionText = latestAction
        } else if runningTaskCount > 0 {
            actionText = "正在执行后台任务..."
        } else if stepCount > 0 {
            actionText = "正在执行第 \(stepCount) 步..."
        } else {
            actionText = "正在执行..."
        }
        
        let updatedState = AgentActivityAttributes.ContentState(
            status: status,
            stepCount: stepCount,
            latestAction: actionText,
            runningTaskCount: runningTaskCount,
            activeTaskTitle: activeTaskTitle,
            activeTaskCommand: activeTaskCommand,
            hasPendingAction: hasPendingAction,
            lastUpdated: Date()
        )
        
        Task {
            await activity.update(.init(state: updatedState, staleDate: nil))
        }
    }
    
    public func endActivity(finalStatus: String = "COMPLETED") {
        guard let activity = currentActivity else { return }
        
        let summary: String
        switch finalStatus {
        case "COMPLETED":
            summary = "任务已完成"
        case "CANCELLED":
            summary = "任务已终止"
        case "FAILED":
            summary = "执行遇到错误"
        default:
            summary = "执行已结束"
        }
        
        let finalState = AgentActivityAttributes.ContentState(
            status: finalStatus,
            stepCount: activity.content.state.stepCount,
            latestAction: summary,
            runningTaskCount: 0,
            activeTaskTitle: nil,
            activeTaskCommand: nil,
            hasPendingAction: false,
            lastUpdated: Date()
        )
        
        Task {
            await activity.end(
                .init(state: finalState, staleDate: nil),
                dismissalPolicy: .after(Date().addingTimeInterval(4))
            )
            self.currentActivity = nil
        }
    }
}
