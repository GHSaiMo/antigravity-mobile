import Foundation

/// 父会话通过 invoke_subagent 派发的一个子代理（网关 stream 的 `subagents`）。
public struct SubagentItem: Identifiable, Sendable, Codable, Hashable {
    public var id: String { conversationId }
    public let conversationId: String
    public let typeName: String?
    public let role: String?
    public let prompt: String?
    public let modelTier: String?
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
        self.stepIndex = stepIndex
        self.status = status
        self.stepCount = stepCount
        self.title = title
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
        case "running": return "运行中"
        case "gone": return "已清理"
        case "done": return "已结束"
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
            workspaceName: "子代理",
            isSubagent: true
        )
    }
}
