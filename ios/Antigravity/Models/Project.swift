import Foundation

public struct ProjectItem: Identifiable, Hashable, Codable, Sendable {
    public var id: String { uri }
    public let name: String
    public let uri: String
    public let path: String
    public let isWorkspace: Bool
    public let sessionCount: Int
    public let lastActive: Date?
    
    public init(name: String, uri: String, path: String, isWorkspace: Bool, sessionCount: Int, lastActive: Date? = nil) {
        self.name = name
        self.uri = uri
        self.path = path
        self.isWorkspace = isWorkspace
        self.sessionCount = sessionCount
        self.lastActive = lastActive
    }
    
    public var relativeTimeString: String {
        guard let date = lastActive else { return "" }
        let diff = Date().timeIntervalSince(date)
        if diff < 60 { return "刚刚" }
        if diff < 3600 { return "\(Int(diff / 60))分钟前" }
        if diff < 86400 { return "\(Int(diff / 3600))小时前" }
        return "\(Int(diff / 86400))天前"
    }
}

public struct CreateCascadeResponsePayload: Codable, Sendable {
    public let cascadeId: String?
    public let status: String?
    public let error: String?
}
