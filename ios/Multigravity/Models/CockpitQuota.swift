import Foundation

public struct CockpitQuotaResponse: Codable, Sendable {
    public let currentAccount: CockpitAccountQuota?
    public let accounts: [CockpitAccountQuota]
    public let updatedAt: Int64
    
    enum CodingKeys: String, CodingKey {
        case currentAccount = "current_account"
        case accounts
        case updatedAt = "updated_at"
    }
}

public struct CockpitAccountQuota: Codable, Identifiable, Sendable {
    public let id: String
    public let email: String
    public let name: String
    public let isCurrent: Bool
    public let claude5h: CockpitQuotaBucket?
    public let claudeWeekly: CockpitQuotaBucket?
    public let gemini5h: CockpitQuotaBucket?
    public let geminiWeekly: CockpitQuotaBucket?
    public let updatedAt: Int64
    
    enum CodingKeys: String, CodingKey {
        case id, email, name
        case isCurrent = "is_current"
        case claude5h = "claude_5h"
        case claudeWeekly = "claude_weekly"
        case gemini5h = "gemini_5h"
        case geminiWeekly = "gemini_weekly"
        case updatedAt = "updated_at"
    }
    
    public var displayName: String {
        if !name.isEmpty {
            return "\(name) (\(email))"
        }
        return email
    }
}

public struct CockpitQuotaBucket: Codable, Sendable {
    public let remainingFraction: Double
    public let remainingPercent: Double
    public let resetTime: String?
    public let resetFriendly: String?
    
    enum CodingKeys: String, CodingKey {
        case remainingFraction = "remaining_fraction"
        case remainingPercent = "remaining_percent"
        case resetTime = "reset_time"
        case resetFriendly = "reset_friendly"
    }
}
