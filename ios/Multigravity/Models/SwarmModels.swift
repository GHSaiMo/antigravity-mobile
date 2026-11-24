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
    public let activityKind: String?
    public let steps: Int
    public let posts: Int
    public let lastPostSeq: Int
    public let workspace: String?
    public let workspaceMode: String?
    public let roleWorker: Bool
    public let unreadCount: Int
    public let isCurrent: Bool

    enum CodingKeys: String, CodingKey {
        case name, conversationId, model, role, wakeMode, waitSince
        case lastReadSeq, lastNudgedSeq, killed, status, lastActivity
        case activityKind, steps, posts, lastPostSeq, workspace, workspaceMode, roleWorker
        case unreadCount, isCurrent
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
        self.activityKind = try c.decodeIfPresent(String.self, forKey: .activityKind)
        self.steps = try c.decodeIfPresent(Int.self, forKey: .steps) ?? 0
        self.posts = try c.decodeIfPresent(Int.self, forKey: .posts) ?? 0
        self.lastPostSeq = try c.decodeIfPresent(Int.self, forKey: .lastPostSeq) ?? 0
        self.workspace = try c.decodeIfPresent(String.self, forKey: .workspace)
        self.workspaceMode = try c.decodeIfPresent(String.self, forKey: .workspaceMode)
        self.roleWorker = try c.decodeIfPresent(Bool.self, forKey: .roleWorker) ?? false
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

public struct SwarmPlanGoal: Codable, Sendable, Equatable {
    public let title: String
    public let line: String?

    public init(title: String = "", line: String? = nil) {
        self.title = title
        self.line = line
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        self.title = try c.decodeIfPresent(String.self, forKey: .title) ?? ""
        self.line = try c.decodeIfPresent(String.self, forKey: .line)
    }
}

public struct SwarmPlanThread: Codable, Sendable, Identifiable, Equatable {
    public let id: String
    public let title: String
    public let goal: String?
    public let tasks: [String]

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        let tVal = try c.decodeIfPresent(String.self, forKey: .title) ?? ""
        self.id = try c.decodeIfPresent(String.self, forKey: .id) ?? tVal
        self.title = tVal
        self.goal = try c.decodeIfPresent(String.self, forKey: .goal)
        self.tasks = try c.decodeIfPresent([String].self, forKey: .tasks) ?? []
    }
}

public struct SwarmPlan: Codable, Sendable, Equatable {
    public let goal: SwarmPlanGoal?
    public let tasks: [String: String]
    public let threads: [SwarmPlanThread]

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        self.goal = try c.decodeIfPresent(SwarmPlanGoal.self, forKey: .goal)
        self.tasks = try c.decodeIfPresent([String: String].self, forKey: .tasks) ?? [:]
        self.threads = try c.decodeIfPresent([SwarmPlanThread].self, forKey: .threads) ?? []
    }
}

public struct SwarmViewThread: Codable, Sendable, Identifiable, Equatable {
    public let id: String
    public let title: String
    public let goal: String?
    public let now: String?
    public let state: String?
    public let workstreams: [String]
    public let attention: [String]

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        let tVal = try c.decodeIfPresent(String.self, forKey: .title) ?? ""
        self.id = try c.decodeIfPresent(String.self, forKey: .id) ?? tVal
        self.title = tVal
        self.goal = try c.decodeIfPresent(String.self, forKey: .goal)
        self.now = try c.decodeIfPresent(String.self, forKey: .now)
        self.state = try c.decodeIfPresent(String.self, forKey: .state)
        self.workstreams = try c.decodeIfPresent([String].self, forKey: .workstreams) ?? []
        self.attention = try c.decodeIfPresent([String].self, forKey: .attention) ?? []
    }
}

public struct SwarmView: Codable, Sendable, Equatable {
    public let version: String?
    public let asOfSeq: Int?
    public let updatedAt: String?
    public let headline: String?
    public let goal: SwarmPlanGoal?
    public let runningSummary: String?
    public let phase: SwarmViewPhase?
    public let attention: [SwarmViewAttention]
    public let workstreams: [SwarmViewWorkstream]
    public let threads: [SwarmViewThread]
    public let now: [String: String]
    public let milestones: [SwarmViewTextItem]
    public let risks: [SwarmViewTextItem]

    enum CodingKeys: String, CodingKey {
        case version
        case asOfSeq = "as_of_seq"
        case updatedAt = "updated_at"
        case headline
        case goal
        case runningSummary = "running_summary"
        case phase
        case attention
        case workstreams
        case threads
        case now
        case milestones
        case risks
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        self.version = try c.decodeIfPresent(String.self, forKey: .version)
        self.asOfSeq = try c.decodeIfPresent(Int.self, forKey: .asOfSeq)
        self.updatedAt = try c.decodeIfPresent(String.self, forKey: .updatedAt)
        self.headline = try c.decodeIfPresent(String.self, forKey: .headline)
        self.goal = try c.decodeIfPresent(SwarmPlanGoal.self, forKey: .goal)
        self.runningSummary = try c.decodeIfPresent(String.self, forKey: .runningSummary)
        self.phase = try c.decodeIfPresent(SwarmViewPhase.self, forKey: .phase)
        self.attention = try c.decodeIfPresent([SwarmViewAttention].self, forKey: .attention) ?? []
        self.workstreams = try c.decodeIfPresent([SwarmViewWorkstream].self, forKey: .workstreams) ?? []
        self.threads = try c.decodeIfPresent([SwarmViewThread].self, forKey: .threads) ?? []
        self.now = try c.decodeIfPresent([String: String].self, forKey: .now) ?? [:]
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
    public let totalMessages: Int
    public let hasMoreBefore: Bool
    public let oldestSeq: Int
    public let view: SwarmView?
    public let plan: SwarmPlan?

    enum CodingKeys: String, CodingKey {
        case cascadeId, coordinatorId, title, etag, brief, members, messages, totalMessages, hasMoreBefore, oldestSeq, view, plan
    }

    public init(
        cascadeId: String,
        coordinatorId: String,
        title: String,
        etag: String?,
        brief: SwarmBrief,
        members: [SwarmMemberInfo],
        messages: [SwarmBoardMessage],
        totalMessages: Int = 0,
        hasMoreBefore: Bool = false,
        oldestSeq: Int = 0,
        view: SwarmView?,
        plan: SwarmPlan?
    ) {
        self.cascadeId = cascadeId
        self.coordinatorId = coordinatorId
        self.title = title
        self.etag = etag
        self.brief = brief
        self.members = members
        self.messages = messages
        self.totalMessages = totalMessages
        self.hasMoreBefore = hasMoreBefore
        self.oldestSeq = oldestSeq
        self.view = view
        self.plan = plan
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
        self.totalMessages = try c.decodeIfPresent(Int.self, forKey: .totalMessages) ?? self.messages.count
        self.hasMoreBefore = try c.decodeIfPresent(Bool.self, forKey: .hasMoreBefore) ?? false
        self.oldestSeq = try c.decodeIfPresent(Int.self, forKey: .oldestSeq) ?? (self.messages.first?.seq ?? 0)
        self.view = try c.decodeIfPresent(SwarmView.self, forKey: .view)
        self.plan = try c.decodeIfPresent(SwarmPlan.self, forKey: .plan)
    }

    /// Merges incremental forward/backward paged messages onto an existing snapshot while keeping seq ordering unique.
    public func mergingMessages(from incoming: SwarmSnapshot, isOlderPage: Bool) -> SwarmSnapshot {
        var bySeq: [Int: SwarmBoardMessage] = [:]
        for m in self.messages { bySeq[m.seq] = m }
        for m in incoming.messages { bySeq[m.seq] = m }
        let merged = bySeq.values.sorted { $0.seq < $1.seq }
        let nextHasMore = isOlderPage ? incoming.hasMoreBefore : self.hasMoreBefore
        let nextOldest = merged.first?.seq ?? (isOlderPage ? incoming.oldestSeq : self.oldestSeq)
        return SwarmSnapshot(
            cascadeId: incoming.cascadeId.isEmpty ? self.cascadeId : incoming.cascadeId,
            coordinatorId: incoming.coordinatorId.isEmpty ? self.coordinatorId : incoming.coordinatorId,
            title: incoming.title.isEmpty ? self.title : incoming.title,
            etag: incoming.etag ?? self.etag,
            brief: incoming.brief,
            members: incoming.members.isEmpty ? self.members : incoming.members,
            messages: merged,
            totalMessages: max(incoming.totalMessages, merged.count),
            hasMoreBefore: nextHasMore,
            oldestSeq: nextOldest,
            view: incoming.view ?? self.view,
            plan: incoming.plan ?? self.plan
        )
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

    enum CodingKeys: String, CodingKey {
        case id, name, signedIn
    }

    private enum FallbackKeys: String, CodingKey {
        case displayName, plugin, pluginId, pluginName, provider, connected
    }

    public init(id: String, name: String, signedIn: Bool) {
        self.id = id
        self.name = name
        self.signedIn = signedIn
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        let extra = try decoder.container(keyedBy: FallbackKeys.self)

        let rawId = (try c.decodeIfPresent(String.self, forKey: .id) ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
        let rawName = (try c.decodeIfPresent(String.self, forKey: .name) ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
        let displayName = (try extra.decodeIfPresent(String.self, forKey: .displayName) ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
        let plugin = (try extra.decodeIfPresent(String.self, forKey: .plugin) ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
        let pluginId = (try extra.decodeIfPresent(String.self, forKey: .pluginId) ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
        let pluginName = (try extra.decodeIfPresent(String.self, forKey: .pluginName) ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
        let provider = (try extra.decodeIfPresent(String.self, forKey: .provider) ?? "").trimmingCharacters(in: .whitespacesAndNewlines)

        let effectivePlugin = !plugin.isEmpty ? plugin : pluginId
        let resolvedId: String
        if !rawId.isEmpty {
            resolvedId = rawId
        } else if !effectivePlugin.isEmpty && !provider.isEmpty && effectivePlugin.caseInsensitiveCompare(provider) != .orderedSame {
            resolvedId = "\(effectivePlugin):\(provider)"
        } else if !effectivePlugin.isEmpty {
            resolvedId = effectivePlugin
        } else if !provider.isEmpty {
            resolvedId = provider
        } else if !rawName.isEmpty {
            resolvedId = rawName
        } else if !displayName.isEmpty {
            resolvedId = displayName
        } else {
            resolvedId = UUID().uuidString
        }
        self.id = resolvedId

        let pluginLabel = !pluginName.isEmpty ? pluginName : effectivePlugin
        if !rawName.isEmpty {
            self.name = rawName
        } else if !displayName.isEmpty {
            self.name = displayName
        } else if !pluginLabel.isEmpty && !provider.isEmpty && pluginLabel.caseInsensitiveCompare(provider) != .orderedSame {
            self.name = "\(pluginLabel) (\(provider))"
        } else if !pluginLabel.isEmpty {
            self.name = pluginLabel
        } else if !provider.isEmpty {
            self.name = provider
        } else {
            self.name = resolvedId
        }

        let signed = try c.decodeIfPresent(Bool.self, forKey: .signedIn)
        let connected = try extra.decodeIfPresent(Bool.self, forKey: .connected)
        self.signedIn = (signed == true) || (connected == true)
    }
}

public struct CascadePluginsResponse: Codable, Sendable {
    public let plugins: [CascadePluginItem]
    public let providers: [PluginAuthProviderItem]

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        self.plugins = try c.decodeIfPresent([CascadePluginItem].self, forKey: .plugins) ?? []
        let rawProviders = try c.decodeIfPresent([PluginAuthProviderItem].self, forKey: .providers) ?? []
        var seenIds = Set<String>()
        self.providers = rawProviders.map { item in
            var candidate = item.id
            if seenIds.contains(candidate) {
                var n = 2
                while seenIds.contains("\(item.id)-\(n)") {
                    n += 1
                }
                candidate = "\(item.id)-\(n)"
            }
            seenIds.insert(candidate)
            return candidate == item.id ? item : PluginAuthProviderItem(id: candidate, name: item.name, signedIn: item.signedIn)
        }
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
    public let timeZone: String?
    public let model: String?
    public let prompt: String?
    public let lastRunAt: String?
    public let nextRunAt: String?
    public let lastError: String?
    public let startTimeMs: String?
    public let enabled: Bool
    public let workspaceUri: String?
    public let workspaceUris: [String]?
    public let projectId: String?
    public let accessGrants: [String]?

    public var isRunning: Bool {
        enabled || ["running", "ready"].contains(status.lowercased())
    }

    enum CodingKeys: String, CodingKey {
        case id, name, description, status, cronExpression, timeZone, model, prompt
        case lastRunAt, nextRunAt, lastError, startTimeMs, enabled
        case workspaceUri, workspaceUris, projectId, accessGrants
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
        self.timeZone = try c.decodeIfPresent(String.self, forKey: .timeZone)
        self.model = try c.decodeIfPresent(String.self, forKey: .model)
        self.prompt = try c.decodeIfPresent(String.self, forKey: .prompt)
        self.lastRunAt = try c.decodeIfPresent(String.self, forKey: .lastRunAt)
        self.nextRunAt = try c.decodeIfPresent(String.self, forKey: .nextRunAt)
        self.lastError = try c.decodeIfPresent(String.self, forKey: .lastError)
        self.startTimeMs = try c.decodeIfPresent(String.self, forKey: .startTimeMs)
        self.enabled = try c.decodeIfPresent(Bool.self, forKey: .enabled) ?? false
        self.workspaceUri = try c.decodeIfPresent(String.self, forKey: .workspaceUri)
        self.workspaceUris = try c.decodeIfPresent([String].self, forKey: .workspaceUris)
        self.projectId = try c.decodeIfPresent(String.self, forKey: .projectId)
        self.accessGrants = try c.decodeIfPresent([String].self, forKey: .accessGrants)
    }
}

public struct AutomationEventItem: Codable, Sendable, Identifiable, Equatable {
    public let id: String
    public let scheduleId: String
    public let sidecarId: String?
    public let conversationId: String?
    public let timestamp: String
    public let status: String
    public let message: String?

    public func matches(sidecar: AutomationSidecarItem) -> Bool {
        if let sid = sidecarId, !sid.isEmpty {
            return sid == sidecar.id
        }
        return scheduleId == sidecar.id || scheduleId == sidecar.name
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        self.id = try c.decodeIfPresent(String.self, forKey: .id) ?? UUID().uuidString
        self.scheduleId = try c.decodeIfPresent(String.self, forKey: .scheduleId) ?? ""
        self.sidecarId = try c.decodeIfPresent(String.self, forKey: .sidecarId)
        self.conversationId = try c.decodeIfPresent(String.self, forKey: .conversationId)
        self.timestamp = try c.decodeIfPresent(String.self, forKey: .timestamp) ?? ""
        self.status = try c.decodeIfPresent(String.self, forKey: .status) ?? ""
        self.message = try c.decodeIfPresent(String.self, forKey: .message)
    }
}

public struct AutomationLogsResponse: Codable, Sendable {
    public let sidecarId: String
    public let filename: String
    public let files: [String]
    public let logs: String
    public let truncated: Bool

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        self.sidecarId = try c.decodeIfPresent(String.self, forKey: .sidecarId) ?? ""
        self.filename = try c.decodeIfPresent(String.self, forKey: .filename) ?? "sidecar.log"
        self.files = try c.decodeIfPresent([String].self, forKey: .files) ?? []
        self.logs = try c.decodeIfPresent(String.self, forKey: .logs) ?? ""
        self.truncated = try c.decodeIfPresent(Bool.self, forKey: .truncated) ?? false
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
