import Foundation

// MARK: - Swarm Data Models (Aligned with Go Gateway internal/proxy/swarm.go)

public struct SwarmBrief: Codable, Sendable, Equatable {
    public let active: Bool
    public let coordinatorId: String
    public let title: String
    public let phase: String
    public let headline: String
    public let workingCount: Int
    public let totalMembers: Int
    public let latestSeq: Int
    public let attentionCount: Int

    enum CodingKeys: String, CodingKey {
        case active
        case coordinatorId
        case title
        case phase
        case headline
        case workingCount
        case totalMembers
        case latestSeq
        case attentionCount
    }

    public init(
        active: Bool = false,
        coordinatorId: String = "",
        title: String = "",
        phase: String = "",
        headline: String = "",
        workingCount: Int = 0,
        totalMembers: Int = 0,
        latestSeq: Int = 0,
        attentionCount: Int = 0
    ) {
        self.active = active
        self.coordinatorId = coordinatorId
        self.title = title
        self.phase = phase
        self.headline = headline
        self.workingCount = workingCount
        self.totalMembers = totalMembers
        self.latestSeq = latestSeq
        self.attentionCount = attentionCount
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        self.active = try c.decodeIfPresent(Bool.self, forKey: .active) ?? false
        self.coordinatorId = try c.decodeIfPresent(String.self, forKey: .coordinatorId) ?? ""
        self.title = try c.decodeIfPresent(String.self, forKey: .title) ?? ""
        self.phase = try c.decodeIfPresent(String.self, forKey: .phase) ?? ""
        self.headline = try c.decodeIfPresent(String.self, forKey: .headline) ?? ""
        self.workingCount = try c.decodeIfPresent(Int.self, forKey: .workingCount) ?? 0
        self.totalMembers = try c.decodeIfPresent(Int.self, forKey: .totalMembers) ?? 0
        self.latestSeq = try c.decodeIfPresent(Int.self, forKey: .latestSeq) ?? 0
        self.attentionCount = try c.decodeIfPresent(Int.self, forKey: .attentionCount) ?? 0
    }
}

public struct SwarmMemberInfo: Codable, Sendable, Identifiable, Equatable {
    public var id: String { name }
    public let name: String
    public let conversationId: String
    public let model: String
    public let role: String
    public let wakeMode: String
    public let waitSince: String?
    public let lastReadSeq: Int
    public let lastNudgedSeq: Int
    public let killed: Bool
    public let status: String // "working", "waiting", "killed", "error", "stuck"
    public let lastActivity: String?
    public let unreadCount: Int
    public let isCurrent: Bool

    enum CodingKeys: String, CodingKey {
        case name, conversationId, model, role, wakeMode, waitSince
        case lastReadSeq, lastNudgedSeq, killed, status, lastActivity, unreadCount, isCurrent
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        self.name = try c.decodeIfPresent(String.self, forKey: .name) ?? ""
        self.conversationId = try c.decodeIfPresent(String.self, forKey: .conversationId) ?? ""
        self.model = try c.decodeIfPresent(String.self, forKey: .model) ?? ""
        self.role = try c.decodeIfPresent(String.self, forKey: .role) ?? ""
        self.wakeMode = try c.decodeIfPresent(String.self, forKey: .wakeMode) ?? ""
        self.waitSince = try c.decodeIfPresent(String.self, forKey: .waitSince)
        self.lastReadSeq = try c.decodeIfPresent(Int.self, forKey: .lastReadSeq) ?? 0
        self.lastNudgedSeq = try c.decodeIfPresent(Int.self, forKey: .lastNudgedSeq) ?? 0
        self.killed = try c.decodeIfPresent(Bool.self, forKey: .killed) ?? false
        self.status = try c.decodeIfPresent(String.self, forKey: .status) ?? "waiting"
        self.lastActivity = try c.decodeIfPresent(String.self, forKey: .lastActivity)
        self.unreadCount = try c.decodeIfPresent(Int.self, forKey: .unreadCount) ?? 0
        self.isCurrent = try c.decodeIfPresent(Bool.self, forKey: .isCurrent) ?? false
    }
}

public struct SwarmBoardMessage: Codable, Sendable, Identifiable, Equatable {
    public var id: Int { seq }
    public let seq: Int
    public let agent: String
    public let text: String
    public let time: String
    public let channel: String?
    public let mentions: [String]

    enum CodingKeys: String, CodingKey {
        case seq, agent, text, time, channel, mentions
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        self.seq = try c.decodeIfPresent(Int.self, forKey: .seq) ?? 0
        self.agent = try c.decodeIfPresent(String.self, forKey: .agent) ?? ""
        self.text = try c.decodeIfPresent(String.self, forKey: .text) ?? ""
        self.time = try c.decodeIfPresent(String.self, forKey: .time) ?? ""
        self.channel = try c.decodeIfPresent(String.self, forKey: .channel)
        self.mentions = try c.decodeIfPresent([String].self, forKey: .mentions) ?? []
    }
}

public struct SwarmViewPhase: Codable, Sendable, Equatable {
    public let label: String
    public let index: Int
    public let total: Int

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        self.label = try c.decodeIfPresent(String.self, forKey: .label) ?? ""
        self.index = try c.decodeIfPresent(Int.self, forKey: .index) ?? 0
        self.total = try c.decodeIfPresent(Int.self, forKey: .total) ?? 0
    }
}

public struct SwarmViewAttention: Codable, Sendable, Identifiable, Equatable {
    public let id: String
    public let kind: String
    public let severity: String
    public let title: String
    public let why: String?

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        let titleVal = try c.decodeIfPresent(String.self, forKey: .title) ?? ""
        self.id = try c.decodeIfPresent(String.self, forKey: .id) ?? titleVal
        self.kind = try c.decodeIfPresent(String.self, forKey: .kind) ?? "fyi"
        self.severity = try c.decodeIfPresent(String.self, forKey: .severity) ?? "med"
        self.title = titleVal
        self.why = try c.decodeIfPresent(String.self, forKey: .why)
    }
}

public struct SwarmViewWorkstream: Codable, Sendable, Identifiable, Equatable {
    public let id: String
    public let title: String
    public let owners: [String]
    public let state: String
    public let note: String?

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        let titleVal = try c.decodeIfPresent(String.self, forKey: .title) ?? ""
        self.id = try c.decodeIfPresent(String.self, forKey: .id) ?? titleVal
        self.title = titleVal
        self.owners = try c.decodeIfPresent([String].self, forKey: .owners) ?? []
        self.state = try c.decodeIfPresent(String.self, forKey: .state) ?? "active"
        self.note = try c.decodeIfPresent(String.self, forKey: .note)
    }
}

public struct SwarmViewTextItem: Codable, Sendable, Identifiable, Equatable {
    public let id: String
    public let text: String

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        let textVal = try c.decodeIfPresent(String.self, forKey: .text) ?? ""
        self.id = try c.decodeIfPresent(String.self, forKey: .id) ?? textVal
        self.text = textVal
    }
}

public struct SwarmView: Codable, Sendable, Equatable {
    public let version: String?
    public let asOfSeq: Int?
    public let updatedAt: String?
    public let headline: String?
    public let phase: SwarmViewPhase?
    public let attention: [SwarmViewAttention]
    public let workstreams: [SwarmViewWorkstream]
    public let milestones: [SwarmViewTextItem]
    public let risks: [SwarmViewTextItem]

    enum CodingKeys: String, CodingKey {
        case version
        case asOfSeq = "as_of_seq"
        case updatedAt = "updated_at"
        case headline
        case phase
        case attention
        case workstreams
        case milestones
        case risks
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        self.version = try c.decodeIfPresent(String.self, forKey: .version)
        self.asOfSeq = try c.decodeIfPresent(Int.self, forKey: .asOfSeq)
        self.updatedAt = try c.decodeIfPresent(String.self, forKey: .updatedAt)
        self.headline = try c.decodeIfPresent(String.self, forKey: .headline)
        self.phase = try c.decodeIfPresent(SwarmViewPhase.self, forKey: .phase)
        self.attention = try c.decodeIfPresent([SwarmViewAttention].self, forKey: .attention) ?? []
        self.workstreams = try c.decodeIfPresent([SwarmViewWorkstream].self, forKey: .workstreams) ?? []
        self.milestones = try c.decodeIfPresent([SwarmViewTextItem].self, forKey: .milestones) ?? []
        self.risks = try c.decodeIfPresent([SwarmViewTextItem].self, forKey: .risks) ?? []
    }
}

public struct SwarmSnapshot: Codable, Sendable, Equatable {
    public let cascadeId: String
    public let coordinatorId: String
    public let title: String
    public let etag: String?
    public let brief: SwarmBrief
    public let members: [SwarmMemberInfo]
    public let messages: [SwarmBoardMessage]
    public let view: SwarmView?

    enum CodingKeys: String, CodingKey {
        case cascadeId, coordinatorId, title, etag, brief, members, messages, view
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        self.cascadeId = try c.decodeIfPresent(String.self, forKey: .cascadeId) ?? ""
        self.coordinatorId = try c.decodeIfPresent(String.self, forKey: .coordinatorId) ?? ""
        self.title = try c.decodeIfPresent(String.self, forKey: .title) ?? ""
        self.etag = try c.decodeIfPresent(String.self, forKey: .etag)
        self.brief = try c.decodeIfPresent(SwarmBrief.self, forKey: .brief) ?? SwarmBrief()
        self.members = try c.decodeIfPresent([SwarmMemberInfo].self, forKey: .members) ?? []
        self.messages = try c.decodeIfPresent([SwarmBoardMessage].self, forKey: .messages) ?? []
        self.view = try c.decodeIfPresent(SwarmView.self, forKey: .view)
    }
}

public struct SwarmFeedbackRequest: Codable, Sendable {
    public let cascadeId: String
    public let coordinatorId: String?
    public let feedback: String
}

public struct SwarmNudgeRequest: Codable, Sendable {
    public let cascadeId: String
    public let coordinatorId: String?
    public let workerName: String
}

// MARK: - Plugins & Automations Models (Aligned with internal/proxy/plugins.go & automations.go)

public struct CascadePluginItem: Codable, Sendable, Identifiable, Equatable {
    public let id: String
    public let name: String
    public let description: String
    public let version: String?
    public let enabled: Bool
    public let installed: Bool
    public let kind: String?
    public let icon: String?
    public let authStatus: String?
    public let accounts: [String]

    enum CodingKeys: String, CodingKey {
        case id, name, description, version, enabled, installed, kind, icon, authStatus, accounts
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        let rawId = (try c.decodeIfPresent(String.self, forKey: .id) ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
        let rawName = (try c.decodeIfPresent(String.self, forKey: .name) ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
        self.id = !rawId.isEmpty ? rawId : (!rawName.isEmpty ? rawName : UUID().uuidString)
        self.name = !rawName.isEmpty ? rawName : self.id
        self.description = try c.decodeIfPresent(String.self, forKey: .description) ?? ""
        self.version = try c.decodeIfPresent(String.self, forKey: .version)
        self.enabled = try c.decodeIfPresent(Bool.self, forKey: .enabled) ?? false
        self.installed = try c.decodeIfPresent(Bool.self, forKey: .installed) ?? false
        self.kind = try c.decodeIfPresent(String.self, forKey: .kind)
        self.icon = try c.decodeIfPresent(String.self, forKey: .icon)
        self.authStatus = try c.decodeIfPresent(String.self, forKey: .authStatus)
        self.accounts = try c.decodeIfPresent([String].self, forKey: .accounts) ?? []
    }
}

public struct PluginAuthProviderItem: Codable, Sendable, Identifiable, Equatable {
    public let id: String
    public let name: String
    public let signedIn: Bool

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        self.id = try c.decodeIfPresent(String.self, forKey: .id) ?? ""
        self.name = try c.decodeIfPresent(String.self, forKey: .name) ?? self.id
        self.signedIn = try c.decodeIfPresent(Bool.self, forKey: .signedIn) ?? false
    }
}

public struct CascadePluginsResponse: Codable, Sendable {
    public let plugins: [CascadePluginItem]
    public let providers: [PluginAuthProviderItem]

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        self.plugins = try c.decodeIfPresent([CascadePluginItem].self, forKey: .plugins) ?? []
        self.providers = try c.decodeIfPresent([PluginAuthProviderItem].self, forKey: .providers) ?? []
    }
}

public struct PluginInstallRequest: Codable, Sendable {
    public let pluginId: String
    public let cascadeId: String?
    public let autoResume: Bool
}

public struct PluginInstallResponse: Codable, Sendable {
    public let status: String?
    public let success: Bool?
    public let pluginId: String?
    public let installed: Bool?
    public let resumed: Bool?
    public let message: String?

    public var isSuccess: Bool {
        success == true || installed == true || status == "ok"
    }
}

public struct AutomationSidecarItem: Codable, Sendable, Identifiable, Equatable {
    public let id: String
    public let name: String
    public let description: String?
    public let status: String // "READY", "RUNNING", "STOPPED", "ERROR"
    public let cronExpression: String?
    public let prompt: String?
    public let lastRunAt: String?
    public let nextRunAt: String?
    public let enabled: Bool
    public let workspaceUri: String?

    public var isRunning: Bool {
        enabled || ["running", "ready"].contains(status.lowercased())
    }

    enum CodingKeys: String, CodingKey {
        case id, name, description, status, cronExpression, prompt, lastRunAt, nextRunAt, enabled, workspaceUri
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        let nameVal = try c.decodeIfPresent(String.self, forKey: .name) ?? ""
        let idVal = try c.decodeIfPresent(String.self, forKey: .id)
        self.id = (idVal?.isEmpty == false) ? idVal! : nameVal
        self.name = nameVal.isEmpty ? self.id : nameVal
        self.description = try c.decodeIfPresent(String.self, forKey: .description)
        self.status = try c.decodeIfPresent(String.self, forKey: .status) ?? "STOPPED"
        self.cronExpression = try c.decodeIfPresent(String.self, forKey: .cronExpression)
        self.prompt = try c.decodeIfPresent(String.self, forKey: .prompt)
        self.lastRunAt = try c.decodeIfPresent(String.self, forKey: .lastRunAt)
        self.nextRunAt = try c.decodeIfPresent(String.self, forKey: .nextRunAt)
        self.enabled = try c.decodeIfPresent(Bool.self, forKey: .enabled) ?? false
        self.workspaceUri = try c.decodeIfPresent(String.self, forKey: .workspaceUri)
    }
}

public struct AutomationEventItem: Codable, Sendable, Identifiable, Equatable {
    public let id: String
    public let scheduleId: String
    public let conversationId: String?
    public let timestamp: String
    public let status: String
    public let message: String?

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        self.id = try c.decodeIfPresent(String.self, forKey: .id) ?? UUID().uuidString
        self.scheduleId = try c.decodeIfPresent(String.self, forKey: .scheduleId) ?? ""
        self.conversationId = try c.decodeIfPresent(String.self, forKey: .conversationId)
        self.timestamp = try c.decodeIfPresent(String.self, forKey: .timestamp) ?? ""
        self.status = try c.decodeIfPresent(String.self, forKey: .status) ?? ""
        self.message = try c.decodeIfPresent(String.self, forKey: .message)
    }
}

public struct AutomationsResponse: Codable, Sendable {
    public let automations: [AutomationSidecarItem]
    public let events: [AutomationEventItem]

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        self.automations = try c.decodeIfPresent([AutomationSidecarItem].self, forKey: .automations) ?? []
        self.events = try c.decodeIfPresent([AutomationEventItem].self, forKey: .events) ?? []
    }
}

public struct AutomationManageRequest: Codable, Sendable {
    public let sidecarId: String
    public let action: String
}
