import Foundation

public struct CockpitQuotaResponse: Codable, Sendable {
    public let currentAccount: CockpitAccountQuota?
    public let accounts: [CockpitAccountQuota]
    public let updatedAt: Int64
    /// "cockpit" or "live"; nil from older gateways.
    public let source: String?
    /// nil from older gateways (treated as installed).
    public let cockpitInstalled: Bool?
    public let cockpitHint: String?
    
    /// 网关未配置 Cockpit 时为 true,此时只有当前账号的实时额度。
    public var needsCockpitInstall: Bool { cockpitInstalled == false }
    
    enum CodingKeys: String, CodingKey {
        case currentAccount = "current_account"
        case accounts
        case updatedAt = "updated_at"
        case source
        case cockpitInstalled = "cockpit_installed"
        case cockpitHint = "cockpit_hint"
    }
}

public struct CockpitAccountQuota: Codable, Identifiable, Sendable {
    public let id: String
    public let email: String
    public let name: String
    public let subscriptionTier: String?
    public let isCurrent: Bool
    public let claude5h: CockpitQuotaBucket?
    public let claudeWeekly: CockpitQuotaBucket?
    public let gemini5h: CockpitQuotaBucket?
    public let geminiWeekly: CockpitQuotaBucket?
    public let gemini4Argon: CockpitQuotaBucket?
    public let updatedAt: Int64
    
    enum CodingKeys: String, CodingKey {
        case id, email, name
        case subscriptionTier = "subscription_tier"
        case isCurrent = "is_current"
        case claude5h = "claude_5h"
        case claudeWeekly = "claude_weekly"
        case gemini5h = "gemini_5h"
        case geminiWeekly = "gemini_weekly"
        case gemini4Argon = "gemini_4_argon"
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

    public func calculateWeeklyPace(totalWindowHours: Double = 168.0) -> WeeklyPaceInfo? {
        guard let rt = resetTime?.trimmingCharacters(in: .whitespacesAndNewlines), !rt.isEmpty else { return nil }
        let cleanRt = rt.components(separatedBy: ".").first ?? rt
        let isoFmt = ISO8601DateFormatter()
        isoFmt.formatOptions = [.withFullDate, .withTime, .withColonSeparatorInTime]
        if let date = isoFmt.date(from: cleanRt) ?? isoFmt.date(from: cleanRt + "Z") {
            return evaluatePace(date: date, totalWindowHours: totalWindowHours)
        }
        let df = DateFormatter()
        df.locale = Locale(identifier: "en_US_POSIX")
        df.timeZone = TimeZone(secondsFromGMT: 0)
        df.dateFormat = "yyyy-MM-dd'T'HH:mm:ss"
        if let fallbackDate = df.date(from: cleanRt) {
            return evaluatePace(date: fallbackDate, totalWindowHours: totalWindowHours)
        }
        return nil
    }

    public func calculatePaceForTitle(_ title: String, tier: String? = nil) -> WeeklyPaceInfo? {
        let lower = title.lowercased()
        let hours: Double
        if lower.contains("argon") {
            let isUltra = tier?.trimmingCharacters(in: .whitespacesAndNewlines).uppercased().contains("ULTRA") == true
            hours = isUltra ? 5.0 : 168.0
        } else if lower.contains("5h") {
            hours = 5.0
        } else if lower.contains("weekly") {
            hours = 168.0
        } else {
            hours = 168.0
        }
        return calculateWeeklyPace(totalWindowHours: hours)
    }

    private func evaluatePace(date: Date, totalWindowHours: Double) -> WeeklyPaceInfo? {
        let remainingSeconds = date.timeIntervalSinceNow
        guard remainingSeconds > 0 else { return nil }
        let totalSeconds = totalWindowHours * 3600.0
        let expectedRemaining = min(100.0, max(0.0, (remainingSeconds / totalSeconds) * 100.0))
        let actualRemaining = min(100.0, max(0.0, remainingPercent))
        let used = min(100.0, max(0.0, 100.0 - actualRemaining))
        let expectedUsed = min(100.0, max(0.0, 100.0 - expectedRemaining))
        let delta = actualRemaining - expectedRemaining
        let isOver = delta < -1.0

        let deltaAbsStr = String(format: "%.1f%%", abs(delta))
        let status: String
        if isOver {
            status = "消耗偏快 (-\(deltaAbsStr))"
        } else if abs(delta) <= 1.0 {
            status = "匀速正常 (±\(deltaAbsStr))"
        } else {
            status = "节奏充裕 (+\(deltaAbsStr))"
        }

        return WeeklyPaceInfo(
            actualRemainingPercent: actualRemaining,
            expectedRemainingPercent: expectedRemaining,
            usedPercent: used,
            expectedUsedPercent: expectedUsed,
            paceDeltaPercent: delta,
            isOverPacing: isOver,
            statusText: status,
            windowHours: totalWindowHours,
            resetCountdownDisplay: resetFriendly ?? "计算中"
        )
    }
}

public struct WeeklyPaceInfo: Sendable {
    public let actualRemainingPercent: Double
    public let expectedRemainingPercent: Double
    public let usedPercent: Double
    public let expectedUsedPercent: Double
    public let paceDeltaPercent: Double
    public let isOverPacing: Bool
    public let statusText: String
    public let windowHours: Double
    public let resetCountdownDisplay: String
    
    public var windowLabel: String {
        windowHours <= 5.0 ? "5小时" : "7天"
    }
}
