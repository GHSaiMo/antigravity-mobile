import Foundation
import ActivityKit

@MainActor
public final class ActivityManager {
    public static let shared = ActivityManager()
    
    private var _currentActivity: Activity<AgentActivityAttributes>?
    
    public init() {
        deduplicateActivities()
    }
    
    public var isLiveActivityEnabled: Bool {
        ActivityAuthorizationInfo().areActivitiesEnabled
    }
    
    public var currentActivity: Activity<AgentActivityAttributes>? {
        if let act = _currentActivity, act.activityState == .active {
            return act
        }
        let active = Activity<AgentActivityAttributes>.activities.first(where: { $0.activityState == .active })
        _currentActivity = active
        return active
    }
    
    public var currentCascadeId: String? {
        currentActivity?.attributes.cascadeId
    }
    
    public var hasActiveActivity: Bool {
        currentActivity != nil
    }
    
    public func findActiveActivity(for cascadeId: String) -> Activity<AgentActivityAttributes>? {
        if let current = currentActivity, current.attributes.cascadeId == cascadeId {
            return current
        }
        return Activity<AgentActivityAttributes>.activities.first {
            $0.activityState == .active && $0.attributes.cascadeId == cascadeId
        }
    }
    
    public func cleanUpOrphanedActivities(keepingId: String? = nil) {
        let activeActivities = Activity<AgentActivityAttributes>.activities.filter { $0.activityState == .active }
        for activity in activeActivities {
            if let keepId = keepingId, activity.id == keepId {
                continue
            }
            Task {
                await activity.end(nil, dismissalPolicy: .immediate)
            }
        }
    }
    
    public func deduplicateActivities() {
        let activeActivities = Activity<AgentActivityAttributes>.activities.filter { $0.activityState == .active }
        guard activeActivities.count > 1 else { return }
        
        let sorted = activeActivities.sorted {
            $0.content.state.lastUpdated > $1.content.state.lastUpdated
        }
        if let primary = sorted.first {
            self._currentActivity = primary
            for duplicate in sorted.dropFirst() {
                Task {
                    await duplicate.end(nil, dismissalPolicy: .immediate)
                }
            }
        }
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
        
        // 1. If an active activity already exists for this conversation, update in-place to avoid flicker & duplicates
        if let existing = findActiveActivity(for: cascadeId) {
            self._currentActivity = existing
            updateActivity(
                title: title,
                status: status,
                stepCount: stepCount,
                latestAction: latestAction,
                runningTaskCount: runningTaskCount,
                activeTaskTitle: activeTaskTitle,
                activeTaskCommand: activeTaskCommand,
                hasPendingAction: hasPendingAction
            )
            cleanUpOrphanedActivities(keepingId: existing.id)
            return
        }
        
        // 2. End any other existing activities before requesting a new one
        cleanUpOrphanedActivities()
        self._currentActivity = nil
        
        let attributes = AgentActivityAttributes(conversationTitle: title, cascadeId: cascadeId)
        let initialContentState = AgentActivityAttributes.ContentState(
            conversationTitle: title,
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
            self._currentActivity = activity
            cleanUpOrphanedActivities(keepingId: activity.id)
        } catch {
            print("[LiveActivity] Failed to start: \(error)")
        }
    }
    
    public func updateActivity(
        title: String? = nil,
        status: String,
        stepCount: Int,
        latestAction: String,
        runningTaskCount: Int = 0,
        activeTaskTitle: String? = nil,
        activeTaskCommand: String? = nil,
        hasPendingAction: Bool = false
    ) {
        guard let activity = currentActivity, activity.activityState == .active else { return }
        
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
        
        let resolvedTitle: String
        if let newTitle = title?.trimmingCharacters(in: .whitespacesAndNewlines), !newTitle.isEmpty {
            resolvedTitle = newTitle
        } else if !activity.content.state.conversationTitle.isEmpty {
            resolvedTitle = activity.content.state.conversationTitle
        } else {
            resolvedTitle = activity.attributes.conversationTitle
        }
        
        let updatedState = AgentActivityAttributes.ContentState(
            conversationTitle: resolvedTitle,
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
    
    public func updateTitle(cascadeId: String, newTitle: String) {
        guard let activity = findActiveActivity(for: cascadeId) else { return }
        updateActivity(
            title: newTitle,
            status: activity.content.state.status,
            stepCount: activity.content.state.stepCount,
            latestAction: activity.content.state.latestAction,
            runningTaskCount: activity.content.state.runningTaskCount,
            activeTaskTitle: activity.content.state.activeTaskTitle,
            activeTaskCommand: activity.content.state.activeTaskCommand,
            hasPendingAction: activity.content.state.hasPendingAction
        )
    }
    
    public func endActivity(
        finalStatus: String = "COMPLETED",
        dismissalPolicy: ActivityUIDismissalPolicy = .after(Date().addingTimeInterval(4))
    ) {
        let activeActivities = Activity<AgentActivityAttributes>.activities.filter { $0.activityState == .active }
        guard !activeActivities.isEmpty else {
            self._currentActivity = nil
            return
        }
        
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
        
        self._currentActivity = nil
        
        for activity in activeActivities {
            let finalTitle = !activity.content.state.conversationTitle.isEmpty ? activity.content.state.conversationTitle : activity.attributes.conversationTitle
            let finalState = AgentActivityAttributes.ContentState(
                conversationTitle: finalTitle,
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
                    dismissalPolicy: dismissalPolicy
                )
            }
        }
    }
}
