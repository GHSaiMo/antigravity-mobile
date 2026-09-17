import Foundation
import ActivityKit

nonisolated public struct AgentTaskSnapshot: Codable, Hashable, Sendable, Identifiable {
    public var id: String
    public var title: String
    public var command: String?
    public var isWaiting: Bool
    
    public init(id: String, title: String, command: String? = nil, isWaiting: Bool = false) {
        self.id = id
        self.title = title
        self.command = command
        self.isWaiting = isWaiting
    }
    
    public nonisolated static func == (lhs: AgentTaskSnapshot, rhs: AgentTaskSnapshot) -> Bool {
        lhs.id == rhs.id &&
        lhs.title == rhs.title &&
        lhs.command == rhs.command &&
        lhs.isWaiting == rhs.isWaiting
    }
    
    public nonisolated func hash(into hasher: inout Hasher) {
        hasher.combine(id)
        hasher.combine(title)
        hasher.combine(command)
        hasher.combine(isWaiting)
    }
}

nonisolated public struct AgentActivityAttributes: ActivityAttributes, Sendable {
    public struct ContentState: Codable, Hashable, Sendable {
        public var conversationTitle: String   // Dynamic conversation title (can be updated in real time)
        public var status: String              // "RUNNING", "TASK_RUNNING", "WAITING_APPROVAL", "COMPLETED", "CANCELLED"
        public var stepCount: Int              // e.g. 14
        public var latestAction: String        // Agent's action description
        public var runningTaskCount: Int       // Number of background running tasks (e.g. 1, 2)
        public var runningTasks: [AgentTaskSnapshot] // List of active background tasks (supports multi-task rendering)
        public var activeTaskTitle: String?    // Primary running task summary or action (e.g. "编译 iOS 原生客户端")
        public var activeTaskCommand: String?  // Primary running task command snippet (e.g. "xcodebuild ...")
        public var hasPendingAction: Bool      // Waiting for user approval/interaction
        public var lastUpdated: Date
        
        enum CodingKeys: String, CodingKey {
            case conversationTitle
            case status
            case stepCount
            case latestAction
            case runningTaskCount
            case runningTasks
            case activeTaskTitle
            case activeTaskCommand
            case hasPendingAction
            case lastUpdated
        }
        
        public init(
            conversationTitle: String = "",
            status: String,
            stepCount: Int,
            latestAction: String,
            runningTaskCount: Int = 0,
            runningTasks: [AgentTaskSnapshot] = [],
            activeTaskTitle: String? = nil,
            activeTaskCommand: String? = nil,
            hasPendingAction: Bool = false,
            lastUpdated: Date = Date()
        ) {
            self.conversationTitle = conversationTitle
            self.status = status
            self.stepCount = stepCount
            self.latestAction = latestAction
            self.runningTaskCount = runningTaskCount
            self.runningTasks = runningTasks
            self.activeTaskTitle = activeTaskTitle ?? runningTasks.first?.title
            self.activeTaskCommand = activeTaskCommand ?? runningTasks.first?.command
            self.hasPendingAction = hasPendingAction
            self.lastUpdated = lastUpdated
        }
        
        public init(from decoder: Decoder) throws {
            let container = try decoder.container(keyedBy: CodingKeys.self)
            self.conversationTitle = try container.decodeIfPresent(String.self, forKey: .conversationTitle) ?? ""
            self.status = try container.decode(String.self, forKey: .status)
            self.stepCount = try container.decode(Int.self, forKey: .stepCount)
            self.latestAction = try container.decode(String.self, forKey: .latestAction)
            self.runningTaskCount = try container.decodeIfPresent(Int.self, forKey: .runningTaskCount) ?? 0
            self.runningTasks = try container.decodeIfPresent([AgentTaskSnapshot].self, forKey: .runningTasks) ?? []
            self.activeTaskTitle = try container.decodeIfPresent(String.self, forKey: .activeTaskTitle) ?? self.runningTasks.first?.title
            self.activeTaskCommand = try container.decodeIfPresent(String.self, forKey: .activeTaskCommand) ?? self.runningTasks.first?.command
            self.hasPendingAction = try container.decodeIfPresent(Bool.self, forKey: .hasPendingAction) ?? false
            self.lastUpdated = try container.decodeIfPresent(Date.self, forKey: .lastUpdated) ?? Date()
        }
    }
    
    public var conversationTitle: String
    public var cascadeId: String
    
    public init(conversationTitle: String, cascadeId: String) {
        self.conversationTitle = conversationTitle
        self.cascadeId = cascadeId
    }
}
