import Foundation
import ActivityKit

public struct AgentActivityAttributes: ActivityAttributes {
    public struct ContentState: Codable, Hashable, Sendable {
        public var status: String       // "RUNNING", "IDLE", "COMPLETED"
        public var stepCount: Int       // e.g. 14
        public var latestAction: String // e.g. "正在查看文件..."
        public var lastUpdated: Date
        
        public init(status: String, stepCount: Int, latestAction: String, lastUpdated: Date = Date()) {
            self.status = status
            self.stepCount = stepCount
            self.latestAction = latestAction
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
