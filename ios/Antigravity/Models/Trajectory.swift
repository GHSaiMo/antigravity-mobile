import Foundation

public struct GetCascadeTrajectoryRequest: Codable, Sendable {
    public let cascadeId: String
    
    public init(cascadeId: String) {
        self.cascadeId = cascadeId
    }
}

public struct GetCascadeTrajectoryResponse: Codable, Sendable {
    public let trajectory: TrajectoryDetails?
    public let status: String?
}

public struct TrajectoryDetails: Codable, Sendable {
    public let trajectoryId: String?
    public let cascadeId: String?
    public let steps: [CortexStep]?
    public let workspaceUris: [String]?
}

public struct CortexStep: Codable, Sendable {
    public let type: String?
    public let status: String?
    public let metadata: CortexStepMetadata?
    public let userInput: UserInputPayload?
    public let plannerResponse: PlannerResponsePayload?
}

public struct CortexStepMetadata: Codable, Sendable {
    public let createdAt: String?
}

public struct UserInputPayload: Codable, Sendable {
    public let items: [TextItem]?
    public let userResponse: String?
    public let media: [UserMediaItem]?
}

public struct UserMediaItem: Codable, Sendable {
    public let mimeType: String?
    public let description: String?
    public let thumbnail: String?
    public let inlineData: String?
    public let uri: String?
}

public struct TextItem: Codable, Sendable {
    public let text: String?
}

public struct PlannerResponsePayload: Codable, Sendable {
    public let response: String?
    public let thinking: String?
}

// Request to send a message
public struct SendUserCascadeMessageRequest: Codable, Sendable {
    public let cascadeId: String
    public let items: [TextItem]
    public let cascadeConfigRaw: String?
    
    public init(cascadeId: String, text: String, cascadeConfigRaw: String? = nil) {
        self.cascadeId = cascadeId
        self.items = [TextItem(text: text)]
        self.cascadeConfigRaw = cascadeConfigRaw
    }
}

// Request to cancel a task
public struct CancelCascadeInvocationRequest: Codable, Sendable {
    public let cascadeId: String
    
    public init(cascadeId: String) {
        self.cascadeId = cascadeId
    }
}

// Clean model for SwiftUI chat view
public struct ChatMessage: Identifiable, Hashable, Sendable, Codable {
    public let id: String
    public let sender: MessageSender
    public let content: String
    public let thinking: String?
    public let toolCount: Int
    public let toolNames: [String]
    public let imageDataList: [Data]
    public let imageUrls: [String]
    
    public enum MessageSender: Hashable, Sendable, Codable {
        case user
        case agent
        case toolBatch(count: Int, tools: [String])
        
        private enum CodingKeys: String, CodingKey {
            case type, count, tools
        }
        
        public func encode(to encoder: Encoder) throws {
            var container = encoder.container(keyedBy: CodingKeys.self)
            switch self {
            case .user:
                try container.encode("user", forKey: .type)
            case .agent:
                try container.encode("agent", forKey: .type)
            case .toolBatch(let count, let tools):
                try container.encode("toolBatch", forKey: .type)
                try container.encode(count, forKey: .count)
                try container.encode(tools, forKey: .tools)
            }
        }
        
        public init(from decoder: Decoder) throws {
            let container = try decoder.container(keyedBy: CodingKeys.self)
            let type = try container.decode(String.self, forKey: .type)
            switch type {
            case "agent":
                self = .agent
            case "toolBatch":
                let count = try container.decodeIfPresent(Int.self, forKey: .count) ?? 1
                let tools = try container.decodeIfPresent([String].self, forKey: .tools) ?? []
                self = .toolBatch(count: count, tools: tools)
            default:
                self = .user
            }
        }
    }
    
    public init(
        id: String = UUID().uuidString,
        sender: MessageSender,
        content: String,
        thinking: String? = nil,
        toolCount: Int = 0,
        toolNames: [String] = [],
        imageDataList: [Data] = [],
        imageUrls: [String] = []
    ) {
        self.id = id
        self.sender = sender
        self.content = content
        self.thinking = thinking
        self.toolCount = toolCount
        self.toolNames = toolNames
        self.imageDataList = imageDataList
        self.imageUrls = imageUrls
    }
}
