import Foundation

public enum JSONValue: Codable, Hashable, Sendable {
    case string(String)
    case int(Int)
    case double(Double)
    case bool(Bool)
    case object([String: JSONValue])
    case array([JSONValue])
    case null

    public init(from decoder: Decoder) throws {
        let container = try decoder.singleValueContainer()
        if container.decodeNil() {
            self = .null
        } else if let b = try? container.decode(Bool.self) {
            self = .bool(b)
        } else if let i = try? container.decode(Int.self) {
            self = .int(i)
        } else if let d = try? container.decode(Double.self) {
            self = .double(d)
        } else if let s = try? container.decode(String.self) {
            self = .string(s)
        } else if let arr = try? container.decode([JSONValue].self) {
            self = .array(arr)
        } else if let obj = try? container.decode([String: JSONValue].self) {
            self = .object(obj)
        } else {
            self = .null
        }
    }

    public func encode(to encoder: Encoder) throws {
        var container = encoder.singleValueContainer()
        switch self {
        case .string(let s): try container.encode(s)
        case .int(let i): try container.encode(i)
        case .double(let d): try container.encode(d)
        case .bool(let b): try container.encode(b)
        case .object(let o): try container.encode(o)
        case .array(let a): try container.encode(a)
        case .null: try container.encodeNil()
        }
    }
}

public struct ToolDetailItem: Codable, Hashable, Sendable {
    public let name: String
    public let summary: String
    public let status: String
    public let duration: String?
    public let stepIndex: Int?

    /// UI display alias; gateway JSON sends `summary` (and omits empty fields).
    public var detail: String { summary }

    enum CodingKeys: String, CodingKey {
        case name
        case summary
        case detail
        case status
        case duration
        case stepIndex
    }

    public init(
        name: String = "",
        summary: String = "",
        detail: String = "",
        status: String = "",
        duration: String? = nil,
        stepIndex: Int? = nil
    ) {
        self.name = name
        self.summary = !summary.isEmpty ? summary : detail
        self.status = status
        self.duration = duration
        self.stepIndex = stepIndex
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        self.name = try c.decodeIfPresent(String.self, forKey: .name) ?? ""
        let decodedSummary = try c.decodeIfPresent(String.self, forKey: .summary)
        let decodedDetail = try c.decodeIfPresent(String.self, forKey: .detail)
        self.summary = decodedSummary ?? decodedDetail ?? ""
        self.status = try c.decodeIfPresent(String.self, forKey: .status) ?? ""
        self.duration = try c.decodeIfPresent(String.self, forKey: .duration)
        self.stepIndex = try c.decodeIfPresent(Int.self, forKey: .stepIndex)
    }

    public func encode(to encoder: Encoder) throws {
        var c = encoder.container(keyedBy: CodingKeys.self)
        try c.encode(name, forKey: .name)
        if !summary.isEmpty {
            try c.encode(summary, forKey: .summary)
        }
        if !status.isEmpty {
            try c.encode(status, forKey: .status)
        }
        try c.encodeIfPresent(duration, forKey: .duration)
        try c.encodeIfPresent(stepIndex, forKey: .stepIndex)
    }
}

public struct ContextMentionItem: Codable, Hashable, Sendable, Identifiable {
    public let id: String
    public let type: String // "file" | "directory" | "conversation"
    public let label: String
    public let detail: String?
    public let path: String?
    public let uri: String?
    public let cascadeId: String?
    public let scopeItem: JSONValue?

    public init(
        id: String = UUID().uuidString,
        type: String = "file",
        label: String = "",
        detail: String? = nil,
        path: String? = nil,
        uri: String? = nil,
        cascadeId: String? = nil,
        scopeItem: JSONValue? = nil
    ) {
        self.id = id
        self.type = type
        self.label = label
        self.detail = detail
        self.path = path
        self.uri = uri
        self.cascadeId = cascadeId
        self.scopeItem = scopeItem
    }
}

public struct ContextMentionsResponse: Codable, Sendable {
    public let ok: Bool?
    public let items: [ContextMentionItem]?
}

public struct ForkConversationResponse: Codable, Sendable {
    public let ok: Bool?
    public let cascadeId: String?
    public let worktreePath: String?
    public let error: String?
}

public struct WorktreeCreateResponse: Codable, Sendable {
    public let ok: Bool?
    public let worktreePath: String?
    public let worktreeName: String?
    public let branch: String?
    public let cascadeId: String?
    public let error: String?
}

public struct CloneRepoResponse: Codable, Sendable {
    public let ok: Bool?
    public let async: Bool?
    public let jobId: String?
    public let repoSlug: String?
    public let status: String?
    public let phase: String?
    public let progress: Int?
    public let message: String?
    public let targetPath: String?
    public let path: String?
    public let folderUri: String?
    public let project: ProjectItem?
    public let alreadyExists: Bool?
    public let cascadeId: String?
    public let code: String?
    public let existingPath: String?
    public let error: String?
}

public struct LocalDirQuickItem: Identifiable, Codable, Hashable, Sendable {
    public var id: String { path }
    public let path: String
    public let name: String
    public let displayPath: String
    public let projectCount: Int
    public let isDefault: Bool
    public let exists: Bool
}

public struct LocalDirEntry: Identifiable, Codable, Hashable, Sendable {
    public var id: String { path }
    public let name: String
    public let path: String
    public let isGitRepo: Bool
    public let isProject: Bool
    public let isHidden: Bool
}

public struct LocalDirsResponse: Codable, Sendable {
    public let ok: Bool?
    public let platform: String?
    public let homePath: String?
    public let defaultDir: String?
    public let currentPath: String
    public let currentDisplay: String?
    public let parentPath: String?
    public let canGoUp: Bool
    public let recommendedDirs: [LocalDirQuickItem]
    public let entries: [LocalDirEntry]
    public let truncated: Bool?
    public let error: String?
}

public struct CreateLocalDirResponse: Codable, Sendable {
    public let ok: Bool?
    public let path: String?
    public let name: String?
    public let displayPath: String?
    public let alreadyExists: Bool?
    public let error: String?
}

public struct CreateProjectResponse: Codable, Sendable {
    public let ok: Bool?
    public let alreadyExists: Bool?
    public let project: ProjectItem?
    public let cascadeId: String?
    public let error: String?
}

public struct PermissionStateResponse: Codable, Hashable, Sendable {
    public let ok: Bool?
    public let preset: String
    public let label: String
    public let isUnlocked: Bool

    public init(ok: Bool? = true, preset: String = "strict", label: String = String(localized: "严格审批"), isUnlocked: Bool = false) {
        self.ok = ok
        self.preset = preset
        self.label = label
        self.isUnlocked = isUnlocked
    }

    public var localizedLabel: String {
        switch preset {
        case "strict": return String(localized: "严格审批")
        case "sandbox": return String(localized: "离席沙箱放行")
        case "auto": return String(localized: "全自动放行")
        default: return label
        }
    }
}

public struct MemoryRuleItem: Codable, Hashable, Sendable, Identifiable {
    public let id: String
    public let scope: String // "global" | "workspace" | "memory"
    public let title: String
    public let path: String
    public let content: String
    public let readOnly: Bool?

    public var isReadOnly: Bool { readOnly ?? false }
}

public struct MemoriesResponse: Codable, Sendable {
    public let ok: Bool?
    public let items: [MemoryRuleItem]?
}

public struct TerminalItem: Codable, Hashable, Sendable, Identifiable {
    public let terminalId: String?
    public let rawId: String?
    public let name: String?
    public let title: String?
    public let command: String?
    public let status: String?

    public var id: String { effectiveId }

    public var effectiveId: String {
        if let terminalId, !terminalId.isEmpty { return terminalId }
        if let rawId, !rawId.isEmpty { return rawId }
        return UUID().uuidString
    }

    public var displayTitle: String {
        if let name, !name.isEmpty { return name }
        if let title, !title.isEmpty { return title }
        if let command, !command.isEmpty { return command }
        return String(effectiveId.prefix(10))
    }

    enum CodingKeys: String, CodingKey {
        case terminalId
        case rawId = "id"
        case name
        case title
        case command
        case status
    }
}

public struct TerminalsResponse: Codable, Sendable {
    public let ok: Bool?
    public let terminals: [TerminalItem]?
}

public struct BrowserSubagentResponse: Codable, Sendable {
    public let ok: Bool?
    public let url: String?
    public let imageBase64: String?
    public let screenshotUri: String?
}

public struct GitBranchesResponse: Codable, Hashable, Sendable {
    public let ok: Bool?
    public let current: String?
    public let branches: [String]?
}
