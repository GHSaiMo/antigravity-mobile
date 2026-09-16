import Foundation
import ActivityKit

nonisolated public struct AgentActivityAttributes: ActivityAttributes, Sendable {
    public struct ContentState: Codable, Hashable, Sendable {
        public var status: String              // "RUNNING", "TASK_RUNNING", "WAITING_APPROVAL", "COMPLETED", "CANCELLED"
        public var stepCount: Int              // e.g. 14
        public var latestAction: String        // Agent's action description
        public var runningTaskCount: Int       // Number of background running tasks (e.g. 1, 2)
        public var activeTaskTitle: String?    // Primary running task summary or action (e.g. "编译 iOS 原生客户端")
        public var activeTaskCommand: String?  // Primary running task command snippet (e.g. "xcodebuild ...")
        public var hasPendingAction: Bool      // Waiting for user approval/interaction
        public var lastUpdated: Date
        
        public init(
            status: String,
            stepCount: Int,
            latestAction: String,
            runningTaskCount: Int = 0,
            activeTaskTitle: String? = nil,
            activeTaskCommand: String? = nil,
            hasPendingAction: Bool = false,
            lastUpdated: Date = Date()
        ) {
            self.status = status
            self.stepCount = stepCount
            self.latestAction = latestAction
            self.runningTaskCount = runningTaskCount
            self.activeTaskTitle = activeTaskTitle
            self.activeTaskCommand = activeTaskCommand
            self.hasPendingAction = hasPendingAction
            self.lastUpdated = lastUpdated
        }
    }
    
    public var conversationTitle: String
    public var cascadeId: String
    
    public init(conversationTitle: String, cascadeId: String) {
        self.conversationTitle = conversationTitle
        self.cascadeId = cascadeId
    }
}
