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
    
    public var isSubagent: Bool {
        if let meta = trajectoryMetadata {
            if let parent = meta.parentConversationId, !parent.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                return true
            }
            if let depth = meta.nestingDepth, depth > 0 {
                return true
            }
            if meta.hasSubagentSpec == true {
                return true
            }
            if let root = meta.rootConversationId, let tid = trajectoryId, !root.isEmpty, !tid.isEmpty, root != tid {
                return true
            }
        }
        return false
    }
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
    public let parentConversationId: String?
    public let rootConversationId: String?
    public let nestingDepth: Int?
    public let hasSubagentSpec: Bool?
    
    enum CodingKeys: String, CodingKey {
        case workspaceUris
        case projectId
        case createdAt
        case parentConversationId
        case rootConversationId
        case nestingDepth
        case subagentSpec
        case agentScript
    }
    
    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        self.workspaceUris = try container.decodeIfPresent([String].self, forKey: .workspaceUris)
        self.projectId = try container.decodeIfPresent(String.self, forKey: .projectId)
        self.createdAt = try container.decodeIfPresent(String.self, forKey: .createdAt)
        self.parentConversationId = try container.decodeIfPresent(String.self, forKey: .parentConversationId)
        self.rootConversationId = try container.decodeIfPresent(String.self, forKey: .rootConversationId)
        self.nestingDepth = try container.decodeIfPresent(Int.self, forKey: .nestingDepth)
        
        let hasSpec = container.contains(.subagentSpec)
        let hasScript = container.contains(.agentScript)
        self.hasSubagentSpec = hasSpec || hasScript
    }
    
    public func encode(to encoder: Encoder) throws {
        var container = encoder.container(keyedBy: CodingKeys.self)
        try container.encodeIfPresent(workspaceUris, forKey: .workspaceUris)
        try container.encodeIfPresent(projectId, forKey: .projectId)
        try container.encodeIfPresent(createdAt, forKey: .createdAt)
        try container.encodeIfPresent(parentConversationId, forKey: .parentConversationId)
        try container.encodeIfPresent(rootConversationId, forKey: .rootConversationId)
        try container.encodeIfPresent(nestingDepth, forKey: .nestingDepth)
    }
    
    public init(
        workspaceUris: [String]? = nil,
        projectId: String? = nil,
        createdAt: String? = nil,
        parentConversationId: String? = nil,
        rootConversationId: String? = nil,
        nestingDepth: Int? = nil,
        hasSubagentSpec: Bool? = nil
    ) {
        self.workspaceUris = workspaceUris
        self.projectId = projectId
        self.createdAt = createdAt
        self.parentConversationId = parentConversationId
        self.rootConversationId = rootConversationId
        self.nestingDepth = nestingDepth
        self.hasSubagentSpec = hasSubagentSpec
    }
}

public struct ConversationItem: Identifiable, Hashable, Sendable, Codable {
    public let id: String
    public let title: String
    public let status: ConversationStatus
    public let stepCount: Int
    public let workspaceName: String
    public let lastModified: Date?
    public let isSubagent: Bool
    
    enum CodingKeys: String, CodingKey {
        case id
        case title
        case status
        case stepCount
        case workspaceName
        case lastModified
        case isSubagent
    }
    
    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        self.id = try container.decode(String.self, forKey: .id)
        self.title = try container.decode(String.self, forKey: .title)
        self.status = try container.decodeIfPresent(ConversationStatus.self, forKey: .status) ?? .unknown
        self.stepCount = try container.decodeIfPresent(Int.self, forKey: .stepCount) ?? 0
        self.workspaceName = try container.decodeIfPresent(String.self, forKey: .workspaceName) ?? "workspace"
        self.lastModified = try container.decodeIfPresent(Date.self, forKey: .lastModified)
        self.isSubagent = try container.decodeIfPresent(Bool.self, forKey: .isSubagent) ?? false
    }
    
    public func encode(to encoder: Encoder) throws {
        var container = encoder.container(keyedBy: CodingKeys.self)
        try container.encode(id, forKey: .id)
        try container.encode(title, forKey: .title)
        try container.encode(status, forKey: .status)
        try container.encode(stepCount, forKey: .stepCount)
        try container.encode(workspaceName, forKey: .workspaceName)
        try container.encodeIfPresent(lastModified, forKey: .lastModified)
        try container.encode(isSubagent, forKey: .isSubagent)
    }
    
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
        
        var sub = summary.isSubagent
        if !sub, let meta = summary.trajectoryMetadata {
            if let root = meta.rootConversationId, !root.isEmpty, root != id {
                sub = true
            }
        }
        self.isSubagent = sub
    }
    
    public init(
        id: String,
        title: String,
        status: ConversationStatus = .idle,
        stepCount: Int = 0,
        workspaceName: String = "workspace",
        lastModified: Date? = Date(),
        isSubagent: Bool = false
    ) {
        self.id = id
        self.title = title
        self.status = status
        self.stepCount = stepCount
        self.workspaceName = workspaceName
        self.lastModified = lastModified
        self.isSubagent = isSubagent
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
