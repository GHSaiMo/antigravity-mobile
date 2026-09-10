import Foundation

/// Represents a user follow-up message queued during an active Agent execution.
public struct QueuedMessageItem: Identifiable, Sendable, Codable, Hashable {
    public let id: String
    public let text: String
    public let createdAt: String?
    
    public init(id: String = "queue-\(UUID().uuidString)", text: String, createdAt: String? = nil) {
        self.id = id
        self.text = text
        self.createdAt = createdAt ?? ISO8601DateFormatter().string(from: Date())
    }
}
