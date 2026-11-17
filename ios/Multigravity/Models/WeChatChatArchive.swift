import Foundation

public struct WeChatZipFileInfo: Codable, Identifiable, Hashable, Sendable {
    public var id: String { path }
    public let path: String
    public let name: String
    public let size: UInt64
}

public struct WeChatChatMessage: Codable, Identifiable, Hashable, Sendable {
    public let id: Int
    public let sender: String
    public let senderInitial: String
    public let colorIndex: Int
    public let timestamp: String
    public let displayTime: String
    public let kind: String
    public let text: String
    public let mediaEntry: String?
    public let mediaName: String?
    public let mediaSize: UInt64?
    public let durationLabel: String?

    enum CodingKeys: String, CodingKey {
        case id
        case sender
        case senderInitial = "sender_initial"
        case colorIndex = "color_index"
        case timestamp
        case displayTime = "display_time"
        case kind
        case text
        case mediaEntry = "media_entry"
        case mediaName = "media_name"
        case mediaSize = "media_size"
        case durationLabel = "duration_label"
    }
}

public struct WeChatChatArchive: Codable, Sendable {
    public let isWeChatArchive: Bool
    public let title: String
    public let dateRange: String
    public let isMultiDay: Bool
    public let fileName: String
    public let messageCount: Int
    public let rawText: String?
    public let messages: [WeChatChatMessage]?
    public let entries: [WeChatZipFileInfo]?

    enum CodingKeys: String, CodingKey {
        case isWeChatArchive = "is_wechat_archive"
        case title
        case dateRange = "date_range"
        case isMultiDay = "is_multi_day"
        case fileName = "file_name"
        case messageCount = "message_count"
        case rawText = "raw_text"
        case messages
        case entries
    }
}

public struct WeChatArchiveSessionItem: Identifiable, Equatable {
    public let id = UUID()
    public let uri: String
    public let fileName: String
}
