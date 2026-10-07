import Foundation

public enum APIError: LocalizedError, Sendable {
    case invalidURL
    case serverError(statusCode: Int, message: String)
    case networkError(String)
    case decodingError(String)
    
    public var errorDescription: String? {
        switch self {
        case .invalidURL:
            return "无效的服务器地址，请在设置中检查"
        case .serverError(let code, let msg):
            return "服务器错误 (\(code)): \(msg)"
        case .networkError(let msg):
            return "网络连接失败: \(msg)"
        case .decodingError(let msg):
            return "数据解析失败: \(msg)"
        }
    }
}

public struct EmptyResponse: Codable, Sendable {
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
    public let messages: [GatewayMessageItem]
    public let cascadeConfigRaw: String?
    public let canProceed: Bool?
    public let proceedArtifactUri: String?
    public let pendingInteraction: PendingInteraction?
    public let queuedMessages: [QueuedMessageItem]?
    public let runningTasks: [RunningTaskItem]?
    public let activeModel: String?
    public let modelDisplayName: String?
    public let hasError: Bool?
    public let errorMessage: String?
    
    public struct GatewayMessageItem: Codable, Sendable {
        public let id: String
        public let type: String
        public let text: String
        public let toolCount: Int?
        public let toolNames: [String]?
        public let media: [String]?
        public let imageUrls: [String]?
        public let artifacts: [ArtifactItem]?
        public let stepIndex: Int?
        public let attemptCount: Int?
        public let maxAttempts: Int?
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
    public let cascadeConfigRaw: String?
    public let title: String?
    public let canProceed: Bool
    public let proceedArtifactUri: String?
    public let pendingInteraction: PendingInteraction?
    public let queuedMessages: [QueuedMessageItem]
    public let runningTasks: [RunningTaskItem]
    public let activeModel: String?
    public let modelDisplayName: String?
    public let hasError: Bool
    public let errorMessage: String?
}
