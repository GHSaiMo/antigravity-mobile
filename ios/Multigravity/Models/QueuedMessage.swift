import Foundation
import UIKit

/// Represents a user follow-up message queued during an active Agent execution.
public struct QueuedMessageItem: Identifiable, Sendable, Codable, Hashable {
    public let id: String
    public let text: String
    public let createdAt: String?
    public let media: [String]?
    public let imageUrls: [String]?
    
    public init(
        id: String = "queue-\(UUID().uuidString)",
        text: String,
        createdAt: String? = nil,
        media: [String]? = nil,
        imageUrls: [String]? = nil
    ) {
        self.id = id
        self.text = text
        self.createdAt = createdAt ?? ISO8601DateFormatter().string(from: Date())
        self.media = media
        self.imageUrls = imageUrls
    }
    
    public var hasAttachments: Bool {
        (media != nil && !media!.isEmpty) || (imageUrls != nil && !imageUrls!.isEmpty)
    }
    
    public func decodedImages() -> [UIImage] {
        guard let media = media, !media.isEmpty else { return [] }
        return media.compactMap { raw in
            let cleaned: String
            if let commaIndex = raw.firstIndex(of: ",") {
                cleaned = String(raw[raw.index(after: commaIndex)...])
            } else {
                cleaned = raw
            }
            guard let data = Data(base64Encoded: cleaned) else { return nil }
            return UIImage(data: data)
        }
    }
}
