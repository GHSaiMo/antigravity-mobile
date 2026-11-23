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

public enum QuotaBucketKind: String, CaseIterable, Codable, Sendable {
    case gemini5h = "gemini_5h"
    case geminiWeekly = "gemini_weekly"
    case claude5h = "claude_5h"
    case claudeWeekly = "claude_weekly"

    public var displayTitle: String {
        switch self {
        case .gemini5h: return "Gemini · 5h"
        case .geminiWeekly: return "Gemini · Weekly"
        case .claude5h: return "Claude · 5h"
        case .claudeWeekly: return "Claude · Weekly"
        }
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

    public var isFreeTier: Bool {
        subscriptionTier?.trimmingCharacters(in: .whitespacesAndNewlines).uppercased() == "FREE" ||
            (gemini5h == nil && claude5h == nil)
    }

    public func bucket(for kind: QuotaBucketKind) -> CockpitQuotaBucket? {
        switch kind {
        case .gemini5h: return gemini5h
        case .geminiWeekly: return geminiWeekly
        case .claude5h: return claude5h
        case .claudeWeekly: return claudeWeekly
        }
    }

    /// Resolves the effective quota kind and bucket to display on the home quota bar,
    /// falling back gracefully when the preferred bucket is unavailable (e.g. FREE accounts).
    public func resolvedQuotaDisplay(preferred: QuotaBucketKind) -> (kind: QuotaBucketKind, bucket: CockpitQuotaBucket)? {
        if let selected = bucket(for: preferred) {
            return (preferred, selected)
        }
        let fallbackOrder: [QuotaBucketKind] = isFreeTier
            ? [.geminiWeekly, .claudeWeekly]
            : [.gemini5h, .geminiWeekly, .claude5h, .claudeWeekly]
        for kind in fallbackOrder {
            if let fallback = bucket(for: kind) {
                return (kind, fallback)
            }
        }
        return nil
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
        if lower.contains("carbon") {
            hours = 168.0
        } else if lower.contains("argon") {
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
            status = String(localized: "消耗偏快 (-\(deltaAbsStr))")
        } else if abs(delta) <= 1.0 {
            status = String(localized: "匀速正常 (±\(deltaAbsStr))")
        } else {
            status = String(localized: "节奏充裕 (+\(deltaAbsStr))")
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
            resetCountdownDisplay: resetFriendly ?? String(localized: "计算中")
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
        windowHours <= 5.0 ? String(localized: "5小时") : String(localized: "7天")
    }
}

public struct ModelTokenBreakdown: Codable, Identifiable, Sendable, Equatable {
    public var id: String { model }
    public let model: String
    public let total: Int64
    public let input: Int64
    public let output: Int64
    public let cacheRead: Int64
    public let thinking: Int64
    public let steps: Int
    public let costUsd: Double

    public init(
        model: String,
        total: Int64,
        input: Int64 = 0,
        output: Int64 = 0,
        cacheRead: Int64 = 0,
        thinking: Int64 = 0,
        steps: Int = 0,
        costUsd: Double = 0
    ) {
        self.model = model
        self.total = total
        self.input = input
        self.output = output
        self.cacheRead = cacheRead
        self.thinking = thinking
        self.steps = steps
        self.costUsd = costUsd
    }

    enum CodingKeys: String, CodingKey {
        case model, total, input, output, thinking, steps
        case cacheRead = "cache_read"
        case costUsd = "cost_usd"
    }
}

public struct DailyTokenRecord: Codable, Identifiable, Sendable, Equatable {
    public var id: String { date }
    public let date: String
    public let total: Int64
    public let input: Int64
    public let output: Int64
    public let cacheRead: Int64
    public let thinking: Int64
    public let steps: Int
    public let costUsd: Double
    public let models: [String: Int64]
    public let modelDetails: [String: ModelTokenBreakdown]?

    public init(
        date: String,
        total: Int64,
        input: Int64,
        output: Int64,
        cacheRead: Int64,
        thinking: Int64,
        steps: Int,
        costUsd: Double,
        models: [String: Int64],
        modelDetails: [String: ModelTokenBreakdown]? = nil
    ) {
        self.date = date
        self.total = total
        self.input = input
        self.output = output
        self.cacheRead = cacheRead
        self.thinking = thinking
        self.steps = steps
        self.costUsd = costUsd
        self.models = models
        self.modelDetails = modelDetails
    }

    enum CodingKeys: String, CodingKey {
        case date, total, input, output, thinking, steps, models
        case cacheRead = "cache_read"
        case costUsd = "cost_usd"
        case modelDetails = "model_details"
    }
}

public struct TokenHistorySummary: Codable, Sendable, Equatable {
    public let totalTokens: Int64
    public let todayTokens: Int64
    public let last7dTokens: Int64
    public let desktopTokens: Int64
    public let cliTokens: Int64
    public let estimatedCostUsd: Double
    public let estimatedCostCny: Double
    public let activeDays: Int
    public let totalSteps: Int

    enum CodingKeys: String, CodingKey {
        case totalTokens = "total_tokens"
        case todayTokens = "today_tokens"
        case last7dTokens = "last_7d_tokens"
        case desktopTokens = "desktop_tokens"
        case cliTokens = "cli_tokens"
        case estimatedCostUsd = "estimated_cost_usd"
        case estimatedCostCny = "estimated_cost_cny"
        case activeDays = "active_days"
        case totalSteps = "total_steps"
    }
}

public struct TokenHistoryResponse: Codable, Sendable, Equatable {
    public let updatedAt: Int64
    public let summary: TokenHistorySummary
    public let dailyMatrix: [DailyTokenRecord]
    public let modelSummary: [ModelTokenBreakdown]?

    public init(
        updatedAt: Int64,
        summary: TokenHistorySummary,
        dailyMatrix: [DailyTokenRecord],
        modelSummary: [ModelTokenBreakdown]? = nil
    ) {
        self.updatedAt = updatedAt
        self.summary = summary
        self.dailyMatrix = dailyMatrix
        self.modelSummary = modelSummary
    }

    /// 后端下发毫秒级 Unix 时间戳；兼容秒级历史结构并转为本地可格式化 Date
    public var updatedAtDate: Date {
        let seconds = updatedAt >= 10_000_000_000 ? Double(updatedAt) / 1000.0 : Double(updatedAt)
        return Date(timeIntervalSince1970: seconds)
    }

    /// 返回按总 Token 降序的全量模型明细（兼容旧网关未下发 model_summary 的情况）
    public var resolvedModelSummary: [ModelTokenBreakdown] {
        if let list = modelSummary, !list.isEmpty {
            return list.sorted { lhs, rhs in
                if lhs.total != rhs.total { return lhs.total > rhs.total }
                return lhs.model < rhs.model
            }
        }
        var totals: [String: Int64] = [:]
        for rec in dailyMatrix {
            for (m, cnt) in rec.models where cnt > 0 {
                totals[m, default: 0] += cnt
            }
        }
        return totals.sorted { lhs, rhs in
            if lhs.value != rhs.value { return lhs.value > rhs.value }
            return lhs.key < rhs.key
        }.map { ModelTokenBreakdown(model: $0.key, total: $0.value) }
    }

    enum CodingKeys: String, CodingKey {
        case summary
        case updatedAt = "updated_at"
        case dailyMatrix = "daily_matrix"
        case modelSummary = "model_summary"
    }
}

public struct ContextMeterItem: Codable, Sendable, Equatable {
    public let usedTokens: Int64
    public let maxTokens: Int64
    public let percentage: Double
    public let inputTokens: Int64?
    public let outputTokens: Int64?
    public let cacheReadTokens: Int64?
    public let thinkingTokens: Int64?
    public let timeToFirstTokenSec: Double?
    public let streamingDurationSec: Double?
    public let tokensPerSecond: Double?
    public let isOverflow: Bool?
    public let errorKind: String?
    public let cacheHitRate: Double?
    public let cacheMissStreak: Int?
    public let cacheMissTokens: Int64?
    public let cacheStatus: String?
    public let lastActivityAt: String?
    public let sessionSteps: Int?
    public let sessionInputTokens: Int64?
    public let sessionOutputTokens: Int64?
    public let sessionCacheReadTokens: Int64?
    public let sessionThinkingTokens: Int64?
    public let sessionTotalTokens: Int64?
    public let sessionCacheHitRate: Double?
    public let sessionCostUsd: Double?

    public init(
        usedTokens: Int64,
        maxTokens: Int64,
        percentage: Double,
        inputTokens: Int64? = nil,
        outputTokens: Int64? = nil,
        cacheReadTokens: Int64? = nil,
        thinkingTokens: Int64? = nil,
        timeToFirstTokenSec: Double? = nil,
        streamingDurationSec: Double? = nil,
        tokensPerSecond: Double? = nil,
        isOverflow: Bool? = nil,
        errorKind: String? = nil,
        cacheHitRate: Double? = nil,
        cacheMissStreak: Int? = nil,
        cacheMissTokens: Int64? = nil,
        cacheStatus: String? = nil,
        lastActivityAt: String? = nil,
        sessionSteps: Int? = nil,
        sessionInputTokens: Int64? = nil,
        sessionOutputTokens: Int64? = nil,
        sessionCacheReadTokens: Int64? = nil,
        sessionThinkingTokens: Int64? = nil,
        sessionTotalTokens: Int64? = nil,
        sessionCacheHitRate: Double? = nil,
        sessionCostUsd: Double? = nil
    ) {
        self.usedTokens = usedTokens
        self.maxTokens = maxTokens
        self.percentage = percentage
        self.inputTokens = inputTokens
        self.outputTokens = outputTokens
        self.cacheReadTokens = cacheReadTokens
        self.thinkingTokens = thinkingTokens
        self.timeToFirstTokenSec = timeToFirstTokenSec
        self.streamingDurationSec = streamingDurationSec
        self.tokensPerSecond = tokensPerSecond
        self.isOverflow = isOverflow
        self.errorKind = errorKind
        self.cacheHitRate = cacheHitRate
        self.cacheMissStreak = cacheMissStreak
        self.cacheMissTokens = cacheMissTokens
        self.cacheStatus = cacheStatus
        self.lastActivityAt = lastActivityAt
        self.sessionSteps = sessionSteps
        self.sessionInputTokens = sessionInputTokens
        self.sessionOutputTokens = sessionOutputTokens
        self.sessionCacheReadTokens = sessionCacheReadTokens
        self.sessionThinkingTokens = sessionThinkingTokens
        self.sessionTotalTokens = sessionTotalTokens
        self.sessionCacheHitRate = sessionCacheHitRate
        self.sessionCostUsd = sessionCostUsd
    }

    public var hasSessionTotals: Bool {
        (sessionSteps ?? 0) > 0 ||
            (sessionInputTokens ?? 0) + (sessionOutputTokens ?? 0) + (sessionCacheReadTokens ?? 0) + (sessionThinkingTokens ?? 0) > 0
    }

    public var effectiveSessionCacheHitRate: Double? {
        if let rate = sessionCacheHitRate {
            return min(max(rate, 0), 100)
        }
        let inp = max(sessionInputTokens ?? 0, 0)
        let cr = max(sessionCacheReadTokens ?? 0, 0)
        let total = inp + cr
        guard total > 0 else { return nil }
        return (Double(cr) / Double(total)) * 100.0
    }

    public var isNearCompactionCeiling: Bool {
        isOverflow == true || percentage >= 75
    }

    /// 兼容新旧网关：优先采用网关回传命中率，缺失时基于最近一步 input + cacheRead 本地计算。
    public var effectiveCacheHitRate: Double? {
        if let rate = cacheHitRate {
            return min(max(rate, 0), 100)
        }
        let inp = max(inputTokens ?? 0, 0)
        let cr = max(cacheReadTokens ?? 0, 0)
        let total = inp + cr
        guard total > 0 else { return nil }
        return (Double(cr) / Double(total)) * 100.0
    }

    public var isCacheMissStreak: Bool {
        cacheStatus == "miss_streak" || (cacheMissStreak ?? 0) >= 2
    }

    /// 仅当重上下文（>= 40K）、非运行中且无待审批、距最后活动已闲置 >= 15 分钟时返回闲置分钟数。
    public func staleIdleMinutes(
        isRunning: Bool,
        hasPendingInteraction: Bool,
        now: Date = Date()
    ) -> Int? {
        guard !isRunning, !hasPendingInteraction, usedTokens >= 40_000 else { return nil }
        guard let raw = lastActivityAt, !raw.isEmpty,
              let lastDate = Self.parseISO8601Date(raw) else { return nil }
        let minutes = Int(now.timeIntervalSince(lastDate) / 60.0)
        return minutes >= 15 ? minutes : nil
    }

    public func hasCacheWarning(
        isRunning: Bool,
        hasPendingInteraction: Bool,
        now: Date = Date()
    ) -> Bool {
        isCacheMissStreak || staleIdleMinutes(
            isRunning: isRunning,
            hasPendingInteraction: hasPendingInteraction,
            now: now
        ) != nil
    }

    private static func parseISO8601Date(_ raw: String) -> Date? {
        let withFrac = ISO8601DateFormatter()
        withFrac.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        if let d = withFrac.date(from: raw) { return d }
        let plain = ISO8601DateFormatter()
        plain.formatOptions = [.withInternetDateTime]
        return plain.date(from: raw)
    }
}
