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
    public let hasError: Bool?
    public let errorMessage: String?
    
    public var isSubagent: Bool {
        if let meta = trajectoryMetadata {
            if let parent = meta.parentConversationId, !parent.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                return true
            }
            if meta.isBattleModeFork == true {
                return true
            }
            if let depth = meta.nestingDepth, depth > 0 {
                return true
            }
            if meta.hasSubagentSpec == true {
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
    public let markedAsUnread: Bool?
    public let archived: Bool?
}

public struct TrajectoryMetadata: Codable, Sendable {
    public let workspaceUris: [String]?
    public let projectId: String?
    public let createdAt: String?
    public let parentConversationId: String?
    public let rootConversationId: String?
    public let nestingDepth: Int?
    public let hasSubagentSpec: Bool?
    public let isBattleModeFork: Bool?
    
    enum CodingKeys: String, CodingKey {
        case workspaceUris
        case projectId
        case createdAt
        case parentConversationId
        case rootConversationId
        case nestingDepth
        case subagentSpec
        case agentScript
        case isBattleModeFork
    }
    
    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        self.workspaceUris = try container.decodeIfPresent([String].self, forKey: .workspaceUris)
        self.projectId = try container.decodeIfPresent(String.self, forKey: .projectId)
        self.createdAt = try container.decodeIfPresent(String.self, forKey: .createdAt)
        self.parentConversationId = try container.decodeIfPresent(String.self, forKey: .parentConversationId)
        self.rootConversationId = try container.decodeIfPresent(String.self, forKey: .rootConversationId)
        self.nestingDepth = try container.decodeIfPresent(Int.self, forKey: .nestingDepth)
        self.isBattleModeFork = try container.decodeIfPresent(Bool.self, forKey: .isBattleModeFork)
        
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
        try container.encodeIfPresent(isBattleModeFork, forKey: .isBattleModeFork)
    }
    
    public init(
        workspaceUris: [String]? = nil,
        projectId: String? = nil,
        createdAt: String? = nil,
        parentConversationId: String? = nil,
        rootConversationId: String? = nil,
        nestingDepth: Int? = nil,
        hasSubagentSpec: Bool? = nil,
        isBattleModeFork: Bool? = nil
    ) {
        self.workspaceUris = workspaceUris
        self.projectId = projectId
        self.createdAt = createdAt
        self.parentConversationId = parentConversationId
        self.rootConversationId = rootConversationId
        self.nestingDepth = nestingDepth
        self.hasSubagentSpec = hasSubagentSpec
        self.isBattleModeFork = isBattleModeFork
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
    public let isUnread: Bool
    public let draftProject: ProjectItem?
    
    public var isDraft: Bool {
        id.hasPrefix("local_draft_") || id.hasPrefix("draft_") || draftProject != nil
    }
    
    enum CodingKeys: String, CodingKey {
        case id
        case title
        case status
        case stepCount
        case workspaceName
        case lastModified
        case isSubagent
        case isUnread
        case draftProject
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
        self.isUnread = try container.decodeIfPresent(Bool.self, forKey: .isUnread) ?? false
        self.draftProject = try container.decodeIfPresent(ProjectItem.self, forKey: .draftProject)
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
        try container.encode(isUnread, forKey: .isUnread)
        try container.encodeIfPresent(draftProject, forKey: .draftProject)
    }
    
    public enum ConversationStatus: String, Sendable, Codable {
        case running = "RUNNING"
        case action = "ACTION"
        case idle = "IDLE"
        case unknown = "UNKNOWN"
        case error = "ERROR"
        
        public var isRunning: Bool {
            self == .running
        }
        
        public var needsAction: Bool {
            self == .action
        }
        
        public var isError: Bool {
            self == .error
        }
    }
    
    public init(id: String, summary: TrajectorySummary) {
        self.id = id
        self.draftProject = nil
        
        if let t = summary.annotations?.title, !t.isEmpty {
            self.title = t
        } else if let s = summary.summary, !s.isEmpty {
            self.title = s
        } else {
            self.title = "未命名会话"
        }
        
        if summary.needsInput == true {
            self.status = .action
        } else if summary.hasError == true || summary.status == "CASCADE_RUN_STATUS_ERROR" {
            self.status = .error
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
        
        self.isSubagent = summary.isSubagent
        
        // 未读状态计算：排除运行中、等待交互及归档会话
        var unread = false
        if self.status != .running && self.status != .action && summary.annotations?.archived != true {
            if summary.annotations?.markedAsUnread == true {
                unread = true
            } else if let modDate = self.lastModified {
                var serverViewDate: Date? = nil
                if let uvStr = summary.annotations?.lastUserViewTime {
                    let formatter = ISO8601DateFormatter()
                    formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
                    serverViewDate = formatter.date(from: uvStr) ?? ISO8601DateFormatter().date(from: uvStr)
                }
                let localViewDate = CacheManager.shared.getLastViewDate(for: id)
                let effectiveDate = [serverViewDate, localViewDate].compactMap { $0 }.max()
                if let eff = effectiveDate {
                    unread = modDate > eff
                } else {
                    unread = true
                }
            }
        }
        self.isUnread = unread
    }
    
    public init(
        id: String,
        title: String,
        status: ConversationStatus = .idle,
        stepCount: Int = 0,
        workspaceName: String = "workspace",
        lastModified: Date? = Date(),
        isSubagent: Bool = false,
        isUnread: Bool = false,
        draftProject: ProjectItem? = nil
    ) {
        self.id = id
        self.title = title
        self.status = status
        self.stepCount = stepCount
        self.workspaceName = workspaceName
        self.lastModified = lastModified
        self.isSubagent = isSubagent
        self.isUnread = isUnread
        self.draftProject = draftProject
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

public struct LocalDraftSession: Codable, Sendable, Identifiable, Hashable {
    public let id: String
    public let project: ProjectItem
    public var draftText: String
    public var createdAt: Date
    public var updatedAt: Date
    
    public init(
        id: String = "local_draft_\(UUID().uuidString)",
        project: ProjectItem,
        draftText: String = "",
        createdAt: Date = Date(),
        updatedAt: Date = Date()
    ) {
        self.id = id
        self.project = project
        self.draftText = draftText
        self.createdAt = createdAt
        self.updatedAt = updatedAt
    }
    
    public func toConversationItem() -> ConversationItem {
        let trimmed = draftText.trimmingCharacters(in: .whitespacesAndNewlines)
        let firstLine = trimmed.components(separatedBy: .newlines).first?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        let hasImages = CacheManager.shared.hasDraftImages(for: id)
        let displayTitle: String
        if !firstLine.isEmpty {
            displayTitle = firstLine
        } else if hasImages {
            displayTitle = "[图片] \(project.name)"
        } else {
            displayTitle = project.name
        }
        return ConversationItem(
            id: id,
            title: displayTitle,
            status: .idle,
            stepCount: 0,
            workspaceName: project.name,
            lastModified: updatedAt,
            isSubagent: false,
            isUnread: false,
            draftProject: project
        )
    }
}
