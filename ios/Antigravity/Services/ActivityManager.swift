import Foundation
@preconcurrency import ActivityKit

@MainActor
public final class ActivityManager {
    public static let shared = ActivityManager()
    
    private var currentActivity: Activity<AgentActivityAttributes>?
    
    public init() {}
    
    public var isLiveActivityEnabled: Bool {
        ActivityAuthorizationInfo().areActivitiesEnabled
    }
    
    public func startActivity(title: String, cascadeId: String) {
        guard isLiveActivityEnabled else { return }
        
        // End any existing activity first
        if currentActivity != nil {
            endActivity()
        }
        
        let attributes = AgentActivityAttributes(conversationTitle: title, cascadeId: cascadeId)
        let initialContentState = AgentActivityAttributes.ContentState(
            status: "RUNNING",
            stepCount: 1,
            latestAction: "开始执行任务...",
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
    
    public func updateActivity(status: String, stepCount: Int, latestAction: String) {
        guard let activity = currentActivity else { return }
        
        let updatedState = AgentActivityAttributes.ContentState(
            status: status,
            stepCount: stepCount,
            latestAction: latestAction,
            lastUpdated: Date()
        )
        
        Task {
            await activity.update(.init(state: updatedState, staleDate: nil))
        }
    }
    
    public func endActivity(finalStatus: String = "COMPLETED") {
        guard let activity = currentActivity else { return }
        
        let finalState = AgentActivityAttributes.ContentState(
            status: finalStatus,
            stepCount: activity.content.state.stepCount,
            latestAction: finalStatus == "COMPLETED" ? "任务已完成" : "任务已终止",
            lastUpdated: Date()
        )
        
        Task {
            await activity.end(.init(state: finalState, staleDate: nil), dismissalPolicy: .after(Date().addingTimeInterval(5)))
            self.currentActivity = nil
        }
    }
}
