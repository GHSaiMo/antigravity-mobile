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
    public let annotations: Annotations?
    public let summary: String?
    public let hasError: Bool?
    public let errorMessage: String?
}

public struct CortexStep: Codable, Sendable {
    public let type: String?
    public let status: String?
    public let metadata: CortexStepMetadata?
    public let userInput: UserInputPayload?
    public let plannerResponse: PlannerResponsePayload?
    public let errorMessage: ErrorMessagePayload?
    public let error: ErrorPayload?
}

public struct ErrorMessagePayload: Codable, Sendable {
    public let message: String?
    public let shortError: String?
    public let userErrorMessage: String?
}

public struct ErrorPayload: Codable, Sendable {
    public let message: String?
    public let detail: String?
}

public struct CortexStepMetadata: Codable, Sendable {
    public let createdAt: String?
}

public struct UserInputPayload: Codable, Sendable {
    public let items: [TextItem]?
    public let userResponse: String?
    public let media: [UserMediaItem]?
    public let images: [UserImageItem]?
}

public struct UserImageItem: Codable, Sendable {
    public let base64Data: String?
    public let mimeType: String?
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

public struct ArtifactCommentPayload: Codable, Sendable {
    public let artifactUri: String
    public let fullFile: [String: String]
    public let approvalStatus: Int
    public let comment: String
    
    public init(artifactUri: String, approvalStatus: Int = 1, comment: String = "") {
        self.artifactUri = artifactUri
        self.fullFile = [:]
        self.approvalStatus = approvalStatus
        self.comment = comment
    }
}

public struct InteractionOption: Codable, Sendable, Identifiable, Hashable {
    public let id: String
    public let text: String
    public let scope: Int?
    public let isDeny: Bool?
    
    public init(id: String, text: String, scope: Int? = nil, isDeny: Bool? = nil) {
        self.id = id
        self.text = text
        self.scope = scope
        self.isDeny = isDeny
    }
}

public struct PendingInteraction: Codable, Sendable, Identifiable, Hashable {
    public var id: String { "\(trajectoryId):\(stepIndex)" }
    public let type: String
    public let trajectoryId: String
    public let stepIndex: Int
    public let title: String
    public let target: String?
    public let action: String?
    public let description: String?
    public let options: [InteractionOption]
    public let isMultiSelect: Bool?
    public let defaultOptionId: String?
    public let hasWriteIn: Bool?
    public let writeInLabel: String?
    public let writeInPlaceholder: String?
    
    public init(
        type: String,
        trajectoryId: String,
        stepIndex: Int,
        title: String,
        target: String? = nil,
        action: String? = nil,
        description: String? = nil,
        options: [InteractionOption] = [],
        isMultiSelect: Bool? = nil,
        defaultOptionId: String? = nil,
        hasWriteIn: Bool? = nil,
        writeInLabel: String? = nil,
        writeInPlaceholder: String? = nil
    ) {
        self.type = type
        self.trajectoryId = trajectoryId
        self.stepIndex = stepIndex
        self.title = title
        self.target = target
        self.action = action
        self.description = description
        self.options = options
        self.isMultiSelect = isMultiSelect
        self.defaultOptionId = defaultOptionId
        self.hasWriteIn = hasWriteIn
        self.writeInLabel = writeInLabel
        self.writeInPlaceholder = writeInPlaceholder
    }
}

public struct InteractionSubmitRequest: Codable, Sendable {
    public let cascadeId: String
    public let trajectoryId: String
    public let stepIndex: Int
    public let type: String
    public let optionId: String
    public let scope: Int
    public let allow: Bool
    public let writeInResponse: String
    public let skipped: Bool
    public let target: String?
    
    public init(
        cascadeId: String,
        trajectoryId: String,
        stepIndex: Int,
        type: String,
        optionId: String,
        scope: Int = 1,
        allow: Bool = true,
        writeInResponse: String = "",
        skipped: Bool = false,
        target: String? = nil
    ) {
        self.cascadeId = cascadeId
        self.trajectoryId = trajectoryId
        self.stepIndex = stepIndex
        self.type = type
        self.optionId = optionId
        self.scope = scope
        self.allow = allow
        self.writeInResponse = writeInResponse
        self.skipped = skipped
        self.target = target
    }
}

public struct ImageDataPayload: Codable, Sendable {
    public let base64Data: String
    public let mimeType: String
    
    public init(base64Data: String, mimeType: String = "image/jpeg") {
        self.base64Data = base64Data
        self.mimeType = mimeType
    }
}

public struct MediaDataPayload: Codable, Sendable {
    public let inlineData: String
    public let mimeType: String
    
    public init(inlineData: String, mimeType: String = "image/jpeg") {
        self.inlineData = inlineData
        self.mimeType = mimeType
    }
}

// Request to send a message
public struct SendUserCascadeMessageRequest: Codable, Sendable {
    public let cascadeId: String
    public let model: String?
    public let items: [TextItem]
    public let images: [ImageDataPayload]?
    public let media: [MediaDataPayload]?
    public let cascadeConfigRaw: String?
    public let artifactComments: [ArtifactCommentPayload]?
    public let deliveryStrategy: Int?
    
    public init(cascadeId: String, text: String, model: String? = nil, images: [ImageDataPayload]? = nil, media: [MediaDataPayload]? = nil, deliveryStrategy: Int? = nil, cascadeConfigRaw: String? = nil) {
        self.cascadeId = cascadeId
        self.model = model
        self.items = text.isEmpty ? [] : [TextItem(text: text)]
        self.images = images
        self.media = media
        self.cascadeConfigRaw = cascadeConfigRaw
        self.artifactComments = nil
        self.deliveryStrategy = deliveryStrategy
    }
    
    public init(cascadeId: String, items: [TextItem] = [], model: String? = nil, images: [ImageDataPayload]? = nil, media: [MediaDataPayload]? = nil, deliveryStrategy: Int? = nil, cascadeConfigRaw: String? = nil, artifactComments: [ArtifactCommentPayload]? = nil) {
        self.cascadeId = cascadeId
        self.model = model
        self.items = items
        self.images = images
        self.media = media
        self.cascadeConfigRaw = cascadeConfigRaw
        self.artifactComments = artifactComments
        self.deliveryStrategy = deliveryStrategy
    }
}

// Request to delete a queued agent message
public struct DeleteAgentMessageRequest: Codable, Sendable {
    public let messageId: String
    public let recipient: String
    
    public init(messageId: String, recipient: String) {
        self.messageId = messageId
        self.recipient = recipient
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
        case error
        
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
            case .error:
                try container.encode("error", forKey: .type)
            }
        }
        
        public init(from decoder: Decoder) throws {
            let container = try decoder.container(keyedBy: CodingKeys.self)
            let type = try container.decode(String.self, forKey: .type)
            switch type {
            case "agent":
                self = .agent
            case "error":
                self = .error
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
    
    public var isUser: Bool {
        if case .user = sender { return true }
        return false
    }

    public var isAgent: Bool {
        if case .agent = sender { return true }
        return false
    }

    public var isToolBatch: Bool {
        if case .toolBatch = sender { return true }
        return false
    }

    public var isError: Bool {
        if case .error = sender { return true }
        return false
    }
}
