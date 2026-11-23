import Foundation

/// 父会话通过 invoke_subagent 派发的一个子代理（网关 stream 的 `subagents`）。
public struct SubagentItem: Identifiable, Sendable, Codable, Hashable {
    public var id: String { conversationId }
    public let conversationId: String
    public let typeName: String?
    public let role: String?
    public let prompt: String?
    public let modelTier: String?
    public let runtimeModel: String?
    public let stepIndex: Int?
    /// "running" | "done" | "gone"；网关还没补全时为 nil。
    public let status: String?
    public let stepCount: Int?
    public let title: String?

    public init(
        conversationId: String,
        typeName: String? = nil,
        role: String? = nil,
        prompt: String? = nil,
        modelTier: String? = nil,
        runtimeModel: String? = nil,
        stepIndex: Int? = nil,
        status: String? = nil,
        stepCount: Int? = nil,
        title: String? = nil
    ) {
        self.conversationId = conversationId
        self.typeName = typeName
        self.role = role
        self.prompt = prompt
        self.modelTier = modelTier
        self.runtimeModel = runtimeModel
        self.stepIndex = stepIndex
        self.status = status
        self.stepCount = stepCount
        self.title = title
    }

    /// 规范化请求档位（去掉 MODEL_TIER_ 前缀），便于详情页与实际落盘模型对照。
    public var requestedTierDisplay: String? {
        guard let raw = modelTier?.trimmingCharacters(in: .whitespacesAndNewlines), !raw.isEmpty else {
            return nil
        }
        let cleaned = raw
            .replacingOccurrences(of: "MODEL_TIER_", with: "", options: [.caseInsensitive, .anchored])
            .replacingOccurrences(of: "_", with: " ")
            .trimmingCharacters(in: .whitespacesAndNewlines)
        return cleaned.isEmpty ? nil : cleaned.lowercased()
    }

    /// 优先展示真实落盘模型（紧凑格式，如 `Hydrogen · Orch` / `Neon · High`），未落盘时回退请求档位。
    public var modelBadgeText: String? {
        if let rt = runtimeModel?.trimmingCharacters(in: .whitespacesAndNewlines), !rt.isEmpty {
            let compact = ModelDefaultsLogic.formatFullNameToCompact(rt)
                .replacingOccurrences(of: "· Orchestrator", with: "· Orch")
            return compact.isEmpty ? rt : compact
        }
        guard let tier = requestedTierDisplay else { return nil }
        return tier.split(separator: " ").map { $0.capitalized }.joined(separator: " ")
    }

    public var isRunning: Bool { status == "running" }
    public var isGone: Bool { status == "gone" }

    /// 列表里展示的名字：优先角色，其次类型，最后用会话 ID 前 8 位兜底。
    public var displayName: String {
        if let r = role?.trimmingCharacters(in: .whitespacesAndNewlines), !r.isEmpty { return r }
        if let t = typeName?.trimmingCharacters(in: .whitespacesAndNewlines), !t.isEmpty { return t }
        return String(conversationId.prefix(8))
    }

    public var statusText: String {
        switch status {
        case "running": return String(localized: "运行中")
        case "gone": return String(localized: "已清理")
        case "done": return String(localized: "已结束")
        default: return ""
        }
    }

    /// 点开后进入的子会话（只读）。
    public var conversationItem: ConversationItem {
        ConversationItem(
            id: conversationId,
            title: displayName,
            status: isRunning ? .running : .idle,
            stepCount: stepCount ?? 0,
            workspaceName: String(localized: "子代理"),
            isSubagent: true
        )
    }
}
