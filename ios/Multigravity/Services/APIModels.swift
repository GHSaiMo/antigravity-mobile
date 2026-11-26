import Foundation

public enum APIError: LocalizedError, Sendable {
    case invalidURL
    case upstreamNotConnected(reason: String?)
    case serverError(statusCode: Int, message: String)
    case networkError(String)
    case decodingError(String)

    public static func upstreamUnavailableIfMatched(
        statusCode: Int,
        headers: [AnyHashable: Any],
        data: Data
    ) -> APIError? {
        guard statusCode == 503 else { return nil }
        let mgyErrorHeader = headers.first { key, _ in
            String(describing: key).caseInsensitiveCompare("X-Mgy-Error") == .orderedSame
        }.map { String(describing: $0.value).trimmingCharacters(in: .whitespacesAndNewlines).lowercased() }

        var code: String?
        var reason: String?
        var textHint = ""
        if !data.isEmpty,
           let obj = try? JSONSerialization.jsonObject(with: data) as? [String: Any] {
            code = (obj["code"] as? String)?.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
            reason = (obj["reason"] as? String)?.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
            let errStr = (obj["error"] as? String) ?? ""
            let msgStr = (obj["message"] as? String) ?? ""
            textHint = "\(errStr) \(msgStr)".lowercased()
        } else if !data.isEmpty {
            textHint = (String(data: data.prefix(256), encoding: .utf8) ?? "").lowercased()
        }

        let isUpstreamUnavailable =
            mgyErrorHeader == "upstream_not_connected" ||
            code == "upstream_not_connected" ||
            textHint.contains("antigravity upstream not connected") ||
            textHint.contains("language_server is not connected") ||
            textHint.contains("language_server not connected") ||
            textHint.contains("upstream not connected")
        guard isUpstreamUnavailable else { return nil }
        return .upstreamNotConnected(reason: reason)
    }
    
    public var errorDescription: String? {
        switch self {
        case .invalidURL:
            return String(localized: "无效的服务器地址，请在设置中检查")
        case .upstreamNotConnected(let reason):
            switch reason {
            case "ls_access_denied":
                return String(localized: "检测到电脑端 Antigravity 权限隔离（如管理员权限启动），请以相同权限运行 mgy 或用普通权限重启 Antigravity 后重试")
            case "ls_not_ready":
                return String(localized: "已检测到电脑端 Antigravity，正在等待服务就绪（请确认已登录并进入主界面），请稍后重试")
            default:
                return String(localized: "电脑端 Antigravity 未打开或尚未连接，请先在电脑上启动 Antigravity 后重试")
            }
        case .serverError(let code, let msg):
            return String(localized: "服务器错误 (\(code)): \(msg)")
        case .networkError(let msg):
            return String(localized: "网络连接失败: \(msg)")
        case .decodingError(let msg):
            return String(localized: "数据解析失败: \(msg)")
        }
    }
}

public struct EmptyResponse: Codable, Sendable {
    public init() {}
}

public struct EmptyRequest: Codable, Sendable {
    public init() {}
}

public struct FileContentResponse: Codable, Sendable {
    public let uri: String
    public let filename: String
    public let content: String
    public let summary: String?
    public let requestFeedback: Bool?
    public let userFacing: Bool?
    
    enum CodingKeys: String, CodingKey {
        case uri
        case filename
        case content
        case summary
        case requestFeedback = "request_feedback"
        case userFacing = "user_facing"
    }
}

public struct GatewayStatusResponse: Codable, Sendable {
    public let status: String
    public let os: String?
    public let platform: String?
    public let upstream: UpstreamInfo?
    public var usedInterface: String?
    public var connectionDescription: String?
    public var unifiedCursor: UnifiedCursorInfo?
    
    enum CodingKeys: String, CodingKey {
        case status
        case os
        case platform
        case upstream
        case usedInterface
        case connectionDescription
        case unifiedCursor = "unified_cursor"
    }
    
    public struct UpstreamInfo: Codable, Sendable {
        public let pid: Int?
        public let port: Int?
        public let is_healthy: Bool?
    }
    
    public struct UnifiedCursorInfo: Codable, Sendable {
        public let cascadeId: String
        public let title: String?
        public let source: String
        public let updatedAt: String?
        public let isSticky: Bool?
        public let timeSkewMs: Int64?
        
        enum CodingKeys: String, CodingKey {
            case cascadeId = "cascade_id"
            case title
            case source
            case updatedAt = "updated_at"
            case isSticky = "is_sticky"
            case timeSkewMs = "time_skew_ms"
        }
    }
}

public struct PaginatedMessagesResponse: Codable, Sendable {
    public let cascadeId: String
    public let title: String?
    public let status: String
    public let duration: String
    public let totalSteps: Int
    public let totalTools: Int
    public let totalMessages: Int
    public let hasMore: Bool
    public let nextOffset: Int
    public let messages: [GatewayMessageItem]?
    public let turnOutline: [TurnOutlineItem]?
    public let cascadeConfigRaw: String?
    public let canProceed: Bool?
    public let proceedArtifactUri: String?
    public let pendingInteraction: PendingInteraction?
    public let queuedMessages: [QueuedMessageItem]?
    public let runningTasks: [RunningTaskItem]?
    public let activeModel: String?
    public let activeModelName: String?
    public let modelDisplayName: String?
    public let desktopActiveModel: String?
    public let desktopActiveModelName: String?
    public let startedAt: String?
    public let hasError: Bool?
    public let errorMessage: String?
    public let swarm: SwarmBrief?
    public let contextMeter: ContextMeterItem?
    public let subagents: [SubagentItem]?
    public let parentConversationId: String?
    public let subagentRole: String?
    public let suggestedResponses: [String]?
    public let activeFocus: String?
    public let activeFocusStartedAt: String?
    public let activePlan: StepPlanItem?
    public let branchOrigin: SideQuestionOrigin?
    public let sideQuestions: [SideQuestionCardItem]?
    
    public struct GatewayMessageItem: Codable, Sendable {
        public let id: String
        public let type: String
        public let text: String
        public let thinking: String?
        public let thinkingDuration: String?
        public let thinkingBlocks: [ThinkingBlockItem]?
        public let focusText: String?
        public let startedAt: String?
        public let toolCount: Int?
        public let toolNames: [String]?
        public let toolDetails: [ToolDetailItem]?
        public let media: [String]?
        public let imageUrls: [String]?
        public let artifacts: [ArtifactItem]?
        public let stepIndex: Int?
        public let attemptCount: Int?
        public let maxAttempts: Int?
        /// 生成这条回复的模型 id 与展示名（仅 Agent 消息；未知时为空）。
        public let model: String?
        public let modelName: String?
        public let timestamp: String?
        /// type == "subagent" 时的子代理卡片数据（状态由网关实时补全）。
        public let subagent: SubagentItem?
        /// type == "workflow" 时的动态多智能体工作流数据 (Antigravity 2.26+)。
        public let workflow: WorkflowItem?
        /// type == "step_plan" 时的实时步骤清单卡片数据 (update_steps)。
        public let stepPlan: StepPlanItem?
        /// type == "user" 时引用的斜杠命令 / 技能列表。
        public let slashCommands: [SlashCommandMention]?
        /// 客户端发送时生成的幂等标识（Antigravity 2.30+）。
        public let clientMessageId: String?
        public let errorKind: String?
        public let isOverflow: Bool?
    }
}

public struct FetchMessagesResult: Sendable {
    public let status: String
    public let messages: [ChatMessage]
    public let totalSteps: Int
    public let totalTools: Int
    public let duration: String
    public let hasMore: Bool
    public let nextOffset: Int
    public let turnOutline: [TurnOutlineItem]?
    public let cascadeConfigRaw: String?
    public let title: String?
    public let canProceed: Bool
    public let proceedArtifactUri: String?
    public let pendingInteraction: PendingInteraction?
    public let queuedMessages: [QueuedMessageItem]
    public let runningTasks: [RunningTaskItem]
    public let activeModel: String?
    public let activeModelName: String?
    public let modelDisplayName: String?
    public let desktopActiveModel: String?
    public let desktopActiveModelName: String?
    public let startedAt: String?
    public let hasError: Bool
    public let errorMessage: String?
    public let swarm: SwarmBrief?
    public let contextMeter: ContextMeterItem?
    public let subagents: [SubagentItem]?
    public let parentConversationId: String?
    public let subagentRole: String?
    public let suggestedResponses: [String]?
    public let activeFocus: String?
    public let activeFocusStartedAt: String?
    public let activePlan: StepPlanItem?
    public let branchOrigin: SideQuestionOrigin?
    public let sideQuestions: [SideQuestionCardItem]?
}
