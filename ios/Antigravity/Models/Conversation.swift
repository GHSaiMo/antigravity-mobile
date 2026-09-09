import Foundation

public struct GetAllCascadeTrajectoriesResponse: Codable, Sendable {
    public let trajectorySummaries: [String: TrajectorySummary]?
}

public struct TrajectorySummary: Codable, Sendable {
    public let summary: String?
    public let stepCount: Int?
    public let lastModifiedTime: String?
    public let trajectoryId: String?
    public let status: String?
    public let workspaces: [WorkspaceItem]?
    public let annotations: Annotations?
    public let trajectoryMetadata: TrajectoryMetadata?
    public let needsInput: Bool?
}

public struct WorkspaceItem: Codable, Sendable {
    public let workspaceFolderAbsoluteUri: String?
}

public struct Annotations: Codable, Sendable {
    public let title: String?
    public let lastUserViewTime: String?
}

public struct TrajectoryMetadata: Codable, Sendable {
    public let workspaceUris: [String]?
    public let projectId: String?
    public let createdAt: String?
}

public struct ConversationItem: Identifiable, Hashable, Sendable, Codable {
    public let id: String
    public let title: String
    public let status: ConversationStatus
    public let stepCount: Int
    public let workspaceName: String
    public let lastModified: Date?
    
    public enum ConversationStatus: String, Sendable, Codable {
        case running = "RUNNING"
        case action = "ACTION"
        case idle = "IDLE"
        case unknown = "UNKNOWN"
        
        public var isRunning: Bool {
            self == .running
        }
        
        public var needsAction: Bool {
            self == .action
        }
    }
    
    public init(id: String, summary: TrajectorySummary) {
        self.id = id
        
        if let t = summary.annotations?.title, !t.isEmpty {
            self.title = t
        } else if let s = summary.summary, !s.isEmpty {
            self.title = s
        } else {
            self.title = "未命名会话"
        }
        
        if summary.needsInput == true {
            self.status = .action
        } else if summary.status == "CASCADE_RUN_STATUS_RUNNING" {
            self.status = .running
        } else if summary.status == "CASCADE_RUN_STATUS_IDLE" {
            self.status = .idle
        } else {
            self.status = .unknown
        }
        
        self.stepCount = summary.stepCount ?? 0
        
        let wsUri = summary.trajectoryMetadata?.workspaceUris?.first
            ?? summary.workspaces?.first?.workspaceFolderAbsoluteUri
            ?? ""
        let parts = wsUri.split(separator: "/").filter { !$0.isEmpty }
        self.workspaceName = parts.last.map(String.init) ?? "workspace"
        
        if let timeStr = summary.lastModifiedTime {
            let formatter = ISO8601DateFormatter()
            formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
            self.lastModified = formatter.date(from: timeStr) ?? ISO8601DateFormatter().date(from: timeStr)
        } else {
            self.lastModified = nil
        }
    }
    
    public init(
        id: String,
        title: String,
        status: ConversationStatus = .idle,
        stepCount: Int = 0,
        workspaceName: String = "workspace",
        lastModified: Date? = Date()
    ) {
        self.id = id
        self.title = title
        self.status = status
        self.stepCount = stepCount
        self.workspaceName = workspaceName
        self.lastModified = lastModified
    }
    
    public var relativeTimeString: String {
        guard let date = lastModified else { return "" }
        let diff = Date().timeIntervalSince(date)
        if diff < 60 { return "刚刚" }
        if diff < 3600 { return "\(Int(diff / 60))分钟前" }
        if diff < 86400 { return "\(Int(diff / 3600))小时前" }
        return "\(Int(diff / 86400))天前"
    }
}
