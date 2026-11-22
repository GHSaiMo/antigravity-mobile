import AppIntents
import Foundation
import os
import WidgetKit

// MARK: - Shared Constants

nonisolated public enum QuotaWidgetConstants {
    public static let appGroupID = "group.com.ghsaimo.antigravity"
    public static let widgetKind = "QuotaStatusWidget"
    public static let agentWidgetKind = "AgentStatusWidget"
    public static let deepLinkURL = URL(string: "antigravity://quota")!
    public static let conversationsDeepLinkURL = URL(string: "antigravity://conversations")!
    public static let snapshotFileName = "quota_widget_snapshot.json"
    public static let conversationsSnapshotFileName = "agent_conversations_widget_snapshot.json"
    public static let configFileName = "quota_widget_config.json"

    public static func cascadeDeepLinkURL(id: String) -> URL {
        let encoded = id.addingPercentEncoding(withAllowedCharacters: .urlPathAllowed) ?? id
        return URL(string: "antigravity://cascade/\(encoded)") ?? conversationsDeepLinkURL
    }
}

// MARK: - Widget Configuration Intent (Extension-Only to prevent cross-process appintentsd stalls)

#if ACTIVITY_EXTENSION
nonisolated public enum QuotaWidgetProviderPreference: String, AppEnum, Sendable, CaseIterable {
    case followApp = "follow_app"
    case gemini = "gemini"
    case claude = "claude"

    public static let typeDisplayRepresentation: TypeDisplayRepresentation = "监控模型"

    public static let caseDisplayRepresentations: [QuotaWidgetProviderPreference: DisplayRepresentation] = [
        .followApp: DisplayRepresentation(title: "跟随 App"),
        .gemini: DisplayRepresentation(title: "Gemini"),
        .claude: DisplayRepresentation(title: "Claude"),
    ]
}

nonisolated public struct QuotaWidgetConfigurationIntent: WidgetConfigurationIntent {
    public static let title: LocalizedStringResource = "额度监控设置"
    public static let description = IntentDescription("选择小组件优先展示的模型额度。")

    @Parameter(title: "监控模型", default: .followApp)
    public var provider: QuotaWidgetProviderPreference

    public init() {
        self.provider = .followApp
    }

    public init(provider: QuotaWidgetProviderPreference) {
        self.provider = provider
    }
}
#else
nonisolated public enum QuotaWidgetProviderPreference: String, Sendable, CaseIterable {
    case followApp = "follow_app"
    case gemini = "gemini"
    case claude = "claude"
}
#endif

// MARK: - Shared Quota Snapshot Models

nonisolated public struct QuotaWidgetPaceSummary: Hashable, Sendable {
    public let expectedRemainingPercent: Double
    public let deltaPercent: Double
    public let isOver: Bool
    public let isNormal: Bool

    public var formattedDelta: String {
        let absVal = Int(abs(deltaPercent).rounded())
        if isNormal {
            return "±\(absVal)%"
        }
        return isOver ? "-\(absVal)%" : "+\(absVal)%"
    }
}

nonisolated public struct QuotaWidgetBucketSnapshot: Codable, Hashable, Sendable {
    public let remainingPercent: Double
    public let resetAt: Date?
    public let resetFriendly: String?

    public init(remainingPercent: Double, resetAt: Date?, resetFriendly: String?) {
        self.remainingPercent = max(0, min(100, remainingPercent))
        self.resetAt = resetAt
        self.resetFriendly = resetFriendly
    }

    public var clampedFraction: Double {
        max(0, min(1, remainingPercent / 100.0))
    }

    public var intPercent: Int {
        Int(remainingPercent.rounded())
    }

    /// Evaluates current consumption pace against linear burn across the window (default 7d = 168h).
    public func evaluatePace(windowHours: Double = 168.0, now: Date = Date()) -> QuotaWidgetPaceSummary? {
        guard let resetAt else { return nil }
        let remainingSeconds = resetAt.timeIntervalSince(now)
        guard remainingSeconds > 0, windowHours > 0 else { return nil }
        let totalSeconds = windowHours * 3600.0
        let expectedRemaining = min(100.0, max(0.0, (remainingSeconds / totalSeconds) * 100.0))
        let delta = remainingPercent - expectedRemaining
        return QuotaWidgetPaceSummary(
            expectedRemainingPercent: expectedRemaining,
            deltaPercent: delta,
            isOver: delta < -1.0,
            isNormal: abs(delta) <= 1.0
        )
    }
}

nonisolated public struct ResolvedProviderQuota: Hashable, Sendable {
    public let providerTitle: String
    public let isClaude: Bool
    public let shortWindow: QuotaWidgetBucketSnapshot?
    public let weeklyWindow: QuotaWidgetBucketSnapshot?
    public let preferWeekly: Bool

    public init(
        providerTitle: String,
        isClaude: Bool,
        shortWindow: QuotaWidgetBucketSnapshot?,
        weeklyWindow: QuotaWidgetBucketSnapshot?,
        preferWeekly: Bool = false
    ) {
        self.providerTitle = providerTitle
        self.isClaude = isClaude
        self.shortWindow = shortWindow
        self.weeklyWindow = weeklyWindow
        self.preferWeekly = preferWeekly
    }

    public var primaryWindow: QuotaWidgetBucketSnapshot? {
        if preferWeekly {
            return weeklyWindow ?? shortWindow
        }
        return shortWindow ?? weeklyWindow
    }

    public var primaryWindowTag: String {
        if preferWeekly && weeklyWindow != nil {
            return "7d"
        }
        return shortWindow != nil ? "5h" : "7d"
    }
}

nonisolated public struct QuotaWidgetSnapshot: Codable, Hashable, Sendable {
    public let providerTitle: String
    public let isClaude: Bool
    public let accountLabel: String
    public let subscriptionTier: String?
    public let isFreeTier: Bool
    public let isDemo: Bool
    public let shortWindow: QuotaWidgetBucketSnapshot?
    public let weeklyWindow: QuotaWidgetBucketSnapshot?
    public let geminiShort: QuotaWidgetBucketSnapshot?
    public let geminiWeekly: QuotaWidgetBucketSnapshot?
    public let claudeShort: QuotaWidgetBucketSnapshot?
    public let claudeWeekly: QuotaWidgetBucketSnapshot?
    public let selectedBucketRaw: String?
    public let paceCursorEnabled: Bool
    public let isEmailMasked: Bool
    public let syncedAt: Date

    public init(
        providerTitle: String,
        isClaude: Bool,
        accountLabel: String,
        subscriptionTier: String? = nil,
        isFreeTier: Bool,
        isDemo: Bool,
        shortWindow: QuotaWidgetBucketSnapshot?,
        weeklyWindow: QuotaWidgetBucketSnapshot?,
        geminiShort: QuotaWidgetBucketSnapshot? = nil,
        geminiWeekly: QuotaWidgetBucketSnapshot? = nil,
        claudeShort: QuotaWidgetBucketSnapshot? = nil,
        claudeWeekly: QuotaWidgetBucketSnapshot? = nil,
        selectedBucketRaw: String? = nil,
        paceCursorEnabled: Bool = true,
        isEmailMasked: Bool = false,
        syncedAt: Date
    ) {
        self.providerTitle = providerTitle
        self.isClaude = isClaude
        self.accountLabel = accountLabel
        self.subscriptionTier = subscriptionTier
        self.isFreeTier = isFreeTier
        self.isDemo = isDemo
        self.shortWindow = shortWindow
        self.weeklyWindow = weeklyWindow
        self.geminiShort = geminiShort ?? (!isClaude ? shortWindow : nil)
        self.geminiWeekly = geminiWeekly ?? (!isClaude ? weeklyWindow : nil)
        self.claudeShort = claudeShort ?? (isClaude ? shortWindow : nil)
        self.claudeWeekly = claudeWeekly ?? (isClaude ? weeklyWindow : nil)
        self.selectedBucketRaw = selectedBucketRaw
        self.paceCursorEnabled = paceCursorEnabled
        self.isEmailMasked = isEmailMasked
        self.syncedAt = syncedAt
    }

    enum CodingKeys: String, CodingKey {
        case providerTitle, isClaude, accountLabel, subscriptionTier, isFreeTier, isDemo
        case shortWindow, weeklyWindow
        case geminiShort, geminiWeekly, claudeShort, claudeWeekly
        case selectedBucketRaw, paceCursorEnabled, isEmailMasked
        case syncedAt
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        let providerTitle = try c.decode(String.self, forKey: .providerTitle)
        let isClaude = try c.decode(Bool.self, forKey: .isClaude)
        let accountLabel = try c.decodeIfPresent(String.self, forKey: .accountLabel) ?? ""
        let subscriptionTier = try c.decodeIfPresent(String.self, forKey: .subscriptionTier)
        let isFreeTier = try c.decodeIfPresent(Bool.self, forKey: .isFreeTier) ?? false
        let isDemo = try c.decodeIfPresent(Bool.self, forKey: .isDemo) ?? false
        let shortWindow = try c.decodeIfPresent(QuotaWidgetBucketSnapshot.self, forKey: .shortWindow)
        let weeklyWindow = try c.decodeIfPresent(QuotaWidgetBucketSnapshot.self, forKey: .weeklyWindow)
        let geminiShort = try c.decodeIfPresent(QuotaWidgetBucketSnapshot.self, forKey: .geminiShort)
        let geminiWeekly = try c.decodeIfPresent(QuotaWidgetBucketSnapshot.self, forKey: .geminiWeekly)
        let claudeShort = try c.decodeIfPresent(QuotaWidgetBucketSnapshot.self, forKey: .claudeShort)
        let claudeWeekly = try c.decodeIfPresent(QuotaWidgetBucketSnapshot.self, forKey: .claudeWeekly)
        let selectedBucketRaw = try c.decodeIfPresent(String.self, forKey: .selectedBucketRaw)
        let paceCursorEnabled = try c.decodeIfPresent(Bool.self, forKey: .paceCursorEnabled) ?? true
        let isEmailMasked = try c.decodeIfPresent(Bool.self, forKey: .isEmailMasked) ?? false
        let syncedAt = try c.decode(Date.self, forKey: .syncedAt)

        self.init(
            providerTitle: providerTitle,
            isClaude: isClaude,
            accountLabel: accountLabel,
            subscriptionTier: subscriptionTier,
            isFreeTier: isFreeTier,
            isDemo: isDemo,
            shortWindow: shortWindow,
            weeklyWindow: weeklyWindow,
            geminiShort: geminiShort,
            geminiWeekly: geminiWeekly,
            claudeShort: claudeShort,
            claudeWeekly: claudeWeekly,
            selectedBucketRaw: selectedBucketRaw,
            paceCursorEnabled: paceCursorEnabled,
            isEmailMasked: isEmailMasked,
            syncedAt: syncedAt
        )
    }

    /// Applies the latest app-side display preferences (`selectedBucketRaw`, `paceCursorEnabled`, `isEmailMasked`)
    /// from `QuotaWidgetEndpointConfig` so toggling settings in the app immediately updates cached timelines.
    public func applying(config: QuotaWidgetEndpointConfig?) -> QuotaWidgetSnapshot {
        guard let config else { return self }
        let nextBucket = config.selectedBucketRaw ?? selectedBucketRaw
        let nextClaude = config.preferClaude
        return QuotaWidgetSnapshot(
            providerTitle: nextClaude ? "Claude" : "Gemini",
            isClaude: nextClaude,
            accountLabel: accountLabel,
            subscriptionTier: subscriptionTier,
            isFreeTier: isFreeTier,
            isDemo: isDemo,
            shortWindow: nextClaude ? (claudeShort ?? geminiShort) : (geminiShort ?? claudeShort),
            weeklyWindow: nextClaude ? (claudeWeekly ?? geminiWeekly) : (geminiWeekly ?? claudeWeekly),
            geminiShort: geminiShort,
            geminiWeekly: geminiWeekly,
            claudeShort: claudeShort,
            claudeWeekly: claudeWeekly,
            selectedBucketRaw: nextBucket,
            paceCursorEnabled: config.paceCursorEnabled,
            isEmailMasked: config.isEmailMasked,
            syncedAt: syncedAt
        )
    }

    public var primaryWindow: QuotaWidgetBucketSnapshot? {
        shortWindow ?? weeklyWindow
    }

    public var primaryWindowTag: String {
        shortWindow != nil ? "5h" : "7d"
    }

    private var prefersWeeklyInApp: Bool {
        selectedBucketRaw?.hasSuffix("_weekly") == true
    }

    /// Resolves the effective highlighted bucket key matching `AccountQuotaSheet`'s selection logic.
    public var resolvedSelectedBucketRaw: String? {
        let preferred = selectedBucketRaw ?? (isClaude ? "claude_5h" : "gemini_5h")
        let hasBucket: (String) -> Bool = { key in
            switch key {
            case "gemini_5h": return !isFreeTier && geminiShort != nil
            case "gemini_weekly": return geminiWeekly != nil
            case "claude_5h": return !isFreeTier && claudeShort != nil
            case "claude_weekly": return claudeWeekly != nil
            default: return false
            }
        }
        if hasBucket(preferred) {
            return preferred
        }
        let fallbackOrder = isFreeTier
            ? ["gemini_weekly", "claude_weekly"]
            : ["gemini_5h", "gemini_weekly", "claude_5h", "claude_weekly"]
        return fallbackOrder.first(where: hasBucket)
    }

    public var geminiBundle: ResolvedProviderQuota? {
        guard geminiShort != nil || geminiWeekly != nil else { return nil }
        return ResolvedProviderQuota(
            providerTitle: "Gemini",
            isClaude: false,
            shortWindow: geminiShort,
            weeklyWindow: geminiWeekly,
            preferWeekly: selectedBucketRaw == "gemini_weekly"
        )
    }

    public var claudeBundle: ResolvedProviderQuota? {
        guard claudeShort != nil || claudeWeekly != nil else { return nil }
        return ResolvedProviderQuota(
            providerTitle: "Claude",
            isClaude: true,
            shortWindow: claudeShort,
            weeklyWindow: claudeWeekly,
            preferWeekly: selectedBucketRaw == "claude_weekly"
        )
    }

    public var hasBothProviders: Bool {
        geminiBundle != nil && claudeBundle != nil
    }

    public var tierBadgeLabel: String? {
        let trimmed = subscriptionTier?.trimmingCharacters(in: .whitespacesAndNewlines).uppercased() ?? ""
        if trimmed.isEmpty {
            return isFreeTier ? "FREE" : nil
        }
        if trimmed.contains("ULTRA") { return "ULTRA" }
        if trimmed.contains("PRO") { return "PRO" }
        if trimmed.contains("FREE") { return "FREE" }
        return trimmed
    }

    public var displayAccountTitle: String {
        let trimmed = accountLabel.trimmingCharacters(in: .whitespacesAndNewlines)
        if trimmed.isEmpty { return "Multigravity" }
        if isEmailMasked, trimmed.contains("@") {
            return Self.maskEmail(trimmed)
        }
        if trimmed.count > 22, let atIdx = trimmed.firstIndex(of: "@") {
            let prefix = String(trimmed[..<atIdx])
            if !prefix.isEmpty { return prefix }
        }
        return trimmed
    }

    private static func maskEmail(_ email: String) -> String {
        let parts = email.split(separator: "@", maxSplits: 1, omittingEmptySubsequences: false)
        guard parts.count == 2 else { return email }
        let local = String(parts[0])
        let domain = String(parts[1])

        func maskPart(_ str: String) -> String {
            let chars = Array(str)
            guard !chars.isEmpty else { return "" }
            if chars.count == 1 { return String(chars[0]) + "*" }
            if chars.count == 2 { return String(chars[0]) + "*" + String(chars[1]) }
            return String(chars[0]) + String(repeating: "*", count: chars.count - 2) + String(chars[chars.count - 1])
        }

        if let dotIdx = domain.lastIndex(of: ".") {
            let domainName = String(domain[..<dotIdx])
            let domainExt = String(domain[dotIdx...])
            return "\(maskPart(local))@\(maskPart(domainName))\(domainExt)"
        }
        return "\(maskPart(local))@\(maskPart(domain))"
    }

    /// Resolves which single provider to display given the user's per-widget configuration.
    public func resolvedProvider(for preference: QuotaWidgetProviderPreference) -> ResolvedProviderQuota? {
        switch preference {
        case .gemini:
            if let g = geminiBundle {
                return ResolvedProviderQuota(
                    providerTitle: g.providerTitle,
                    isClaude: false,
                    shortWindow: g.shortWindow,
                    weeklyWindow: g.weeklyWindow,
                    preferWeekly: false
                )
            }
            return claudeBundle
        case .claude:
            if let c = claudeBundle {
                return ResolvedProviderQuota(
                    providerTitle: c.providerTitle,
                    isClaude: true,
                    shortWindow: c.shortWindow,
                    weeklyWindow: c.weeklyWindow,
                    preferWeekly: false
                )
            }
            return geminiBundle
        case .followApp:
            if isClaude {
                return claudeBundle ?? geminiBundle
            }
            return geminiBundle ?? claudeBundle
        }
    }

    public static func demoPlaceholder(now: Date = Date()) -> QuotaWidgetSnapshot {
        let gShort = QuotaWidgetBucketSnapshot(
            remainingPercent: 78.4,
            resetAt: now.addingTimeInterval(2 * 3600 + 14 * 60),
            resetFriendly: "2h 14m"
        )
        let gWeekly = QuotaWidgetBucketSnapshot(
            remainingPercent: 86.2,
            resetAt: now.addingTimeInterval(5 * 86400 + 11 * 3600),
            resetFriendly: "5d 11h"
        )
        let cShort = QuotaWidgetBucketSnapshot(
            remainingPercent: 62.5,
            resetAt: now.addingTimeInterval(3 * 3600 + 40 * 60),
            resetFriendly: "3h 40m"
        )
        let cWeekly = QuotaWidgetBucketSnapshot(
            remainingPercent: 91.0,
            resetAt: now.addingTimeInterval(6 * 86400 + 4 * 3600),
            resetFriendly: "6d 4h"
        )
        return QuotaWidgetSnapshot(
            providerTitle: "Gemini",
            isClaude: false,
            accountLabel: "Demo",
            subscriptionTier: "ULTRA",
            isFreeTier: false,
            isDemo: true,
            shortWindow: gShort,
            weeklyWindow: gWeekly,
            geminiShort: gShort,
            geminiWeekly: gWeekly,
            claudeShort: cShort,
            claudeWeekly: cWeekly,
            selectedBucketRaw: "gemini_5h",
            paceCursorEnabled: true,
            isEmailMasked: false,
            syncedAt: now
        )
    }
}

// MARK: - Shared Active Conversations Snapshot Models

nonisolated public enum AgentWidgetConversationStatus: String, Codable, Hashable, Sendable {
    case action = "ACTION"
    case running = "RUNNING"
    case error = "ERROR"
    case idle = "IDLE"
    case unknown = "UNKNOWN"
}

nonisolated public struct AgentWidgetConversationItem: Codable, Hashable, Identifiable, Sendable {
    public let id: String
    public let title: String
    public let workspaceName: String
    public let status: AgentWidgetConversationStatus
    public let stepCount: Int
    public let lastModified: Date?
    public let isUnread: Bool
    public let hasRunningTasks: Bool
    public let isPinned: Bool

    public init(
        id: String,
        title: String,
        workspaceName: String,
        status: AgentWidgetConversationStatus,
        stepCount: Int,
        lastModified: Date?,
        isUnread: Bool,
        hasRunningTasks: Bool,
        isPinned: Bool
    ) {
        self.id = id
        self.title = title
        self.workspaceName = workspaceName
        self.status = status
        self.stepCount = stepCount
        self.lastModified = lastModified
        self.isUnread = isUnread
        self.hasRunningTasks = hasRunningTasks
        self.isPinned = isPinned
    }

    public var isWorking: Bool {
        status == .running || hasRunningTasks
    }

    public var needsAction: Bool {
        status == .action
    }

    /// Priority ordering for desktop / CarPlay glanceability:
    /// Pending approval > Actively running > Error > Unread > Recent idle.
    public var priorityRank: Int {
        if needsAction { return 0 }
        if isWorking { return 1 }
        if status == .error { return 2 }
        if isUnread { return 3 }
        return 4
    }

    public var displayWorkspace: String {
        let trimmed = workspaceName.trimmingCharacters(in: .whitespacesAndNewlines)
        return trimmed.isEmpty ? "Chat" : trimmed
    }
}

nonisolated public struct AgentConversationsWidgetSnapshot: Codable, Hashable, Sendable {
    public let items: [AgentWidgetConversationItem]
    public let runningCount: Int
    public let pendingActionCount: Int
    public let totalCount: Int
    public let isDemo: Bool
    public let syncedAt: Date

    public init(
        items: [AgentWidgetConversationItem],
        runningCount: Int,
        pendingActionCount: Int,
        totalCount: Int,
        isDemo: Bool,
        syncedAt: Date
    ) {
        self.items = items
        self.runningCount = runningCount
        self.pendingActionCount = pendingActionCount
        self.totalCount = totalCount
        self.isDemo = isDemo
        self.syncedAt = syncedAt
    }

    public var topItem: AgentWidgetConversationItem? {
        items.first
    }

    public func isSemanticallyEqual(to other: AgentConversationsWidgetSnapshot) -> Bool {
        items == other.items
            && runningCount == other.runningCount
            && pendingActionCount == other.pendingActionCount
            && totalCount == other.totalCount
            && isDemo == other.isDemo
    }

    /// Compares visible conversation structure & status while ignoring per-step `stepCount` / `lastModified` ticks,
    /// preventing WidgetKit `reloadTimelines` storms during multi-step Agent execution.
    public func isStructurallyEqual(to other: AgentConversationsWidgetSnapshot) -> Bool {
        guard runningCount == other.runningCount,
              pendingActionCount == other.pendingActionCount,
              isDemo == other.isDemo,
              items.count == other.items.count else {
            return false
        }
        for (lhs, rhs) in zip(items, other.items) {
            if lhs.id != rhs.id
                || lhs.title != rhs.title
                || lhs.workspaceName != rhs.workspaceName
                || lhs.status != rhs.status
                || lhs.isUnread != rhs.isUnread
                || lhs.hasRunningTasks != rhs.hasRunningTasks {
                return false
            }
        }
        return true
    }

    public static func demoPlaceholder(now: Date = Date()) -> AgentConversationsWidgetSnapshot {
        let demoItems: [AgentWidgetConversationItem] = [
            AgentWidgetConversationItem(
                id: "demo-1",
                title: String(localized: "修复支付回调超时"),
                workspaceName: "api-server",
                status: .action,
                stepCount: 14,
                lastModified: now.addingTimeInterval(-90),
                isUnread: false,
                hasRunningTasks: false,
                isPinned: true
            ),
            AgentWidgetConversationItem(
                id: "demo-2",
                title: String(localized: "补齐 iOS 全尺寸小组件"),
                workspaceName: "antigravity-mobile",
                status: .running,
                stepCount: 28,
                lastModified: now.addingTimeInterval(-25),
                isUnread: false,
                hasRunningTasks: true,
                isPinned: false
            ),
            AgentWidgetConversationItem(
                id: "demo-3",
                title: String(localized: "核对本周发布日志与翻译"),
                workspaceName: "docs-site",
                status: .idle,
                stepCount: 9,
                lastModified: now.addingTimeInterval(-12 * 60),
                isUnread: true,
                hasRunningTasks: false,
                isPinned: false
            ),
            AgentWidgetConversationItem(
                id: "demo-4",
                title: String(localized: "优化数据库索引查询耗时"),
                workspaceName: "billing-core",
                status: .idle,
                stepCount: 19,
                lastModified: now.addingTimeInterval(-47 * 60),
                isUnread: false,
                hasRunningTasks: false,
                isPinned: false
            ),
            AgentWidgetConversationItem(
                id: "demo-5",
                title: String(localized: "整理周会架构讨论纪要"),
                workspaceName: "Chat",
                status: .idle,
                stepCount: 6,
                lastModified: now.addingTimeInterval(-2 * 3600),
                isUnread: false,
                hasRunningTasks: false,
                isPinned: false
            ),
        ]
        return AgentConversationsWidgetSnapshot(
            items: demoItems,
            runningCount: 1,
            pendingActionCount: 1,
            totalCount: demoItems.count,
            isDemo: true,
            syncedAt: now
        )
    }
}

// MARK: - Shared Endpoint Config

nonisolated public struct QuotaWidgetEndpointConfig: Codable, Sendable {
    public let candidateBaseURLs: [String]
    public let deviceToken: String?
    public let preferClaude: Bool
    public let selectedBucketRaw: String?
    public let paceCursorEnabled: Bool
    public let isEmailMasked: Bool
    public let isDemoMode: Bool
    public let isPaired: Bool
    public let updatedAt: Date

    public init(
        candidateBaseURLs: [String],
        deviceToken: String?,
        preferClaude: Bool,
        selectedBucketRaw: String? = nil,
        paceCursorEnabled: Bool = true,
        isEmailMasked: Bool = false,
        isDemoMode: Bool,
        isPaired: Bool,
        updatedAt: Date = Date()
    ) {
        self.candidateBaseURLs = candidateBaseURLs
        self.deviceToken = deviceToken
        self.preferClaude = preferClaude
        self.selectedBucketRaw = selectedBucketRaw
        self.paceCursorEnabled = paceCursorEnabled
        self.isEmailMasked = isEmailMasked
        self.isDemoMode = isDemoMode
        self.isPaired = isPaired
        self.updatedAt = updatedAt
    }

    enum CodingKeys: String, CodingKey {
        case candidateBaseURLs, deviceToken, preferClaude
        case selectedBucketRaw, paceCursorEnabled, isEmailMasked
        case isDemoMode, isPaired, updatedAt
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        candidateBaseURLs = try c.decode([String].self, forKey: .candidateBaseURLs)
        deviceToken = try c.decodeIfPresent(String.self, forKey: .deviceToken)
        preferClaude = try c.decodeIfPresent(Bool.self, forKey: .preferClaude) ?? false
        selectedBucketRaw = try c.decodeIfPresent(String.self, forKey: .selectedBucketRaw)
        paceCursorEnabled = try c.decodeIfPresent(Bool.self, forKey: .paceCursorEnabled) ?? true
        isEmailMasked = try c.decodeIfPresent(Bool.self, forKey: .isEmailMasked) ?? false
        isDemoMode = try c.decodeIfPresent(Bool.self, forKey: .isDemoMode) ?? false
        isPaired = try c.decodeIfPresent(Bool.self, forKey: .isPaired) ?? false
        updatedAt = try c.decodeIfPresent(Date.self, forKey: .updatedAt) ?? Date()
    }
}

// MARK: - Lightweight Wire DTOs (decoded by both Main App and ActivityExtension)

nonisolated private struct QuotaWireResponse: Decodable, Sendable {
    let currentAccount: QuotaWireAccount?
    let updatedAt: Int64?

    enum CodingKeys: String, CodingKey {
        case currentAccount = "current_account"
        case updatedAt = "updated_at"
    }
}

nonisolated private struct QuotaWireAccount: Decodable, Sendable {
    let email: String?
    let name: String?
    let subscriptionTier: String?
    let claude5h: QuotaWireBucket?
    let claudeWeekly: QuotaWireBucket?
    let gemini5h: QuotaWireBucket?
    let geminiWeekly: QuotaWireBucket?

    enum CodingKeys: String, CodingKey {
        case email, name
        case subscriptionTier = "subscription_tier"
        case claude5h = "claude_5h"
        case claudeWeekly = "claude_weekly"
        case gemini5h = "gemini_5h"
        case geminiWeekly = "gemini_weekly"
    }
}

nonisolated private struct QuotaWireBucket: Decodable, Sendable {
    let remainingFraction: Double?
    let remainingPercent: Double?
    let resetTime: String?
    let resetFriendly: String?

    enum CodingKeys: String, CodingKey {
        case remainingFraction = "remaining_fraction"
        case remainingPercent = "remaining_percent"
        case resetTime = "reset_time"
        case resetFriendly = "reset_friendly"
    }

    var resolvedPercent: Double {
        if let p = remainingPercent {
            return p
        }
        if let f = remainingFraction {
            return f * 100.0
        }
        return 0
    }
}

nonisolated private struct TrajectoriesWireResponse: Decodable, Sendable {
    let trajectorySummaries: [String: TrajectoryWireSummary]?
}

nonisolated private struct TrajectoryWireSummary: Decodable, Sendable {
    let summary: String?
    let stepCount: Int?
    let lastModifiedTime: String?
    let status: String?
    let workspaces: [TrajectoryWireWorkspace]?
    let annotations: TrajectoryWireAnnotations?
    let trajectoryMetadata: TrajectoryWireMetadata?
    let needsInput: Bool?
    let hasError: Bool?
    let parentConversationId: String?
    let nestingDepth: Int?
    let hasRunningTasks: Bool?
    let runningTaskCount: Int?
    let hasRunningSubagents: Bool?
    let notFullyIdle: Bool?
    let backgroundTaskCount: Int?
    let workspaceName: String?

    var isSubagent: Bool {
        if let parent = parentConversationId?.trimmingCharacters(in: .whitespacesAndNewlines), !parent.isEmpty {
            return true
        }
        if let depth = nestingDepth, depth > 0 {
            return true
        }
        if let meta = trajectoryMetadata {
            if let parent = meta.parentConversationId?.trimmingCharacters(in: .whitespacesAndNewlines), !parent.isEmpty {
                return true
            }
            if meta.isBattleModeFork == true {
                return true
            }
            if let depth = meta.nestingDepth, depth > 0 {
                return true
            }
            if meta.hasSubagentSpec == true {
                return true
            }
        }
        return false
    }
}

nonisolated private struct TrajectoryWireWorkspace: Decodable, Sendable {
    let workspaceFolderAbsoluteUri: String?
}

nonisolated private struct TrajectoryWireAnnotations: Decodable, Sendable {
    let title: String?
    let markedAsUnread: Bool?
    let archived: Bool?
    let pinned: Bool?
}

nonisolated private struct TrajectoryWireMetadata: Decodable, Sendable {
    let workspaceUris: [String]?
    let parentConversationId: String?
    let nestingDepth: Int?
    let isBattleModeFork: Bool?
    let hasSubagentSpec: Bool?

    enum CodingKeys: String, CodingKey {
        case workspaceUris, parentConversationId, nestingDepth, isBattleModeFork
        case subagentSpec, agentScript
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        workspaceUris = try c.decodeIfPresent([String].self, forKey: .workspaceUris)
        parentConversationId = try c.decodeIfPresent(String.self, forKey: .parentConversationId)
        nestingDepth = try c.decodeIfPresent(Int.self, forKey: .nestingDepth)
        isBattleModeFork = try c.decodeIfPresent(Bool.self, forKey: .isBattleModeFork)
        hasSubagentSpec = c.contains(.subagentSpec) || c.contains(.agentScript)
    }
}

// MARK: - Shared App Group Store & Background Fetcher

nonisolated public final class QuotaWidgetStore: Sendable {
    public static let shared = QuotaWidgetStore()

    private let lastConversationsReloadLock = OSAllocatedUnfairLock<Date?>(initialState: nil)

    private init() {}

    private var containerURL: URL? {
        FileManager.default.containerURL(forSecurityApplicationGroupIdentifier: QuotaWidgetConstants.appGroupID)
    }

    private var snapshotURL: URL? {
        containerURL?.appendingPathComponent(QuotaWidgetConstants.snapshotFileName)
    }

    private var conversationsSnapshotURL: URL? {
        containerURL?.appendingPathComponent(QuotaWidgetConstants.conversationsSnapshotFileName)
    }

    private var configURL: URL? {
        containerURL?.appendingPathComponent(QuotaWidgetConstants.configFileName)
    }

    // MARK: Quota Snapshot Persistence

    public func loadSnapshot() -> QuotaWidgetSnapshot? {
        guard let url = snapshotURL,
              let data = try? Data(contentsOf: url) else {
            return nil
        }
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601
        return try? decoder.decode(QuotaWidgetSnapshot.self, from: data)
    }

    private static func markExcludedFromBackup(_ url: URL) {
        var mutableURL = url
        var values = URLResourceValues()
        values.isExcludedFromBackup = true
        try? mutableURL.setResourceValues(values)
    }

    public func saveSnapshot(_ snapshot: QuotaWidgetSnapshot) {
        guard let url = snapshotURL else { return }
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        guard let data = try? encoder.encode(snapshot) else { return }
        try? data.write(to: url, options: [.atomic, .completeFileProtectionUntilFirstUserAuthentication])
        Self.markExcludedFromBackup(url)
    }

    public func clearSnapshot() {
        guard let url = snapshotURL else { return }
        try? FileManager.default.removeItem(at: url)
    }

    // MARK: Conversations Snapshot Persistence

    public func loadConversationsSnapshot() -> AgentConversationsWidgetSnapshot? {
        guard let url = conversationsSnapshotURL,
              let data = try? Data(contentsOf: url) else {
            return nil
        }
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601
        return try? decoder.decode(AgentConversationsWidgetSnapshot.self, from: data)
    }

    public func saveConversationsSnapshot(_ snapshot: AgentConversationsWidgetSnapshot) {
        guard let url = conversationsSnapshotURL else { return }
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        guard let data = try? encoder.encode(snapshot) else { return }
        try? data.write(to: url, options: [.atomic, .completeFileProtectionUntilFirstUserAuthentication])
        Self.markExcludedFromBackup(url)
    }

    public func clearConversationsSnapshot() {
        guard let url = conversationsSnapshotURL else { return }
        try? FileManager.default.removeItem(at: url)
    }

    // MARK: Endpoint Config Persistence

    public func loadConfig() -> QuotaWidgetEndpointConfig? {
        guard let url = configURL,
              let data = try? Data(contentsOf: url) else {
            return nil
        }
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601
        return try? decoder.decode(QuotaWidgetEndpointConfig.self, from: data)
    }

    public func saveConfig(_ config: QuotaWidgetEndpointConfig) {
        guard let url = configURL else { return }
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        guard let data = try? encoder.encode(config) else { return }
        try? data.write(to: url, options: [.atomic, .completeFileProtectionUntilFirstUserAuthentication])
        Self.markExcludedFromBackup(url)
    }

    public func clearAll() {
        clearSnapshot()
        clearConversationsSnapshot()
        if let url = configURL {
            try? FileManager.default.removeItem(at: url)
        }
    }

    public func reloadWidgetTimelines() {
        WidgetCenter.shared.reloadTimelines(ofKind: QuotaWidgetConstants.widgetKind)
        WidgetCenter.shared.reloadTimelines(ofKind: QuotaWidgetConstants.agentWidgetKind)
    }

    /// Parses gateway ISO8601 timestamps into a `Date` for native WidgetKit ticking countdowns.
    public static func parseResetDate(_ raw: String?) -> Date? {
        guard let trimmed = raw?.trimmingCharacters(in: .whitespacesAndNewlines), !trimmed.isEmpty else {
            return nil
        }
        let fmtFrac = ISO8601DateFormatter()
        fmtFrac.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        if let d = fmtFrac.date(from: trimmed) {
            return d
        }
        let fmtStd = ISO8601DateFormatter()
        fmtStd.formatOptions = [.withInternetDateTime]
        if let d = fmtStd.date(from: trimmed) {
            return d
        }
        let clean = trimmed.components(separatedBy: ".").first ?? trimmed
        let df = DateFormatter()
        df.locale = Locale(identifier: "en_US_POSIX")
        df.timeZone = TimeZone(secondsFromGMT: 0)
        df.dateFormat = "yyyy-MM-dd'T'HH:mm:ss"
        return df.date(from: clean)
    }

    // MARK: Background Network Fetchers (Widget Extension)

    /// Max raw response size (2.5 MB) decoded inside `ActivityExtension` to stay safely below iOS's ~30 MB Jetsam ceiling.
    private static let maxExtensionPayloadBytes = 2_500_000

    /// Lightweight background quota fetch used by WidgetKit timelines inside `ActivityExtension`.
    /// Races all candidate URLs concurrently with a short 3.2s ceiling so unreachable LAN/tunnel URLs never stall WidgetKit.
    public func fetchLatestQuotaFromGateway() async -> QuotaWidgetSnapshot? {
        guard let config = loadConfig() else {
            return nil
        }
        if config.isDemoMode {
            let demo = QuotaWidgetSnapshot.demoPlaceholder(now: Date())
            saveSnapshot(demo)
            return demo
        }
        let validURLs = config.candidateBaseURLs.compactMap { URL(string: $0) }
        guard config.isPaired, !validURLs.isEmpty else {
            return nil
        }

        let sessionConfig = URLSessionConfiguration.ephemeral
        sessionConfig.timeoutIntervalForRequest = 2.8
        sessionConfig.timeoutIntervalForResource = 3.2
        sessionConfig.waitsForConnectivity = false
        let session = URLSession(configuration: sessionConfig)
        defer { session.invalidateAndCancel() }

        let token = config.deviceToken
        let winner: QuotaWidgetSnapshot? = await withTaskGroup(of: QuotaWidgetSnapshot?.self) { group in
            for baseURL in validURLs {
                group.addTask {
                    let endpoint = baseURL.appendingPathComponent("api/v1/cockpit/quotas")
                    var req = URLRequest(url: endpoint)
                    req.httpMethod = "GET"
                    req.timeoutInterval = 2.8
                    if let token, !token.isEmpty {
                        req.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
                    }
                    do {
                        let (data, response) = try await session.data(for: req)
                        guard let http = response as? HTTPURLResponse,
                              (200...299).contains(http.statusCode),
                              data.count <= Self.maxExtensionPayloadBytes else {
                            return nil
                        }
                        let wire = try JSONDecoder().decode(QuotaWireResponse.self, from: data)
                        return Self.buildSnapshot(from: wire, config: config, isDemo: false)
                    } catch {
                        return nil
                    }
                }
            }

            for await candidate in group {
                if let snap = candidate {
                    group.cancelAll()
                    return snap
                }
            }
            return nil
        }

        if let winner {
            saveSnapshot(winner)
        }
        return winner
    }

    /// Lightweight background conversations fetch used by WidgetKit timelines inside `ActivityExtension`.
    /// Sends `X-Widget-Lite: 1` and races candidate URLs concurrently within 3.2s to avoid timeouts & 30 MB Jetsam kills.
    public func fetchLatestConversationsFromGateway() async -> AgentConversationsWidgetSnapshot? {
        guard let config = loadConfig() else {
            return nil
        }
        if config.isDemoMode {
            let demo = AgentConversationsWidgetSnapshot.demoPlaceholder(now: Date())
            saveConversationsSnapshot(demo)
            return demo
        }
        let validURLs = config.candidateBaseURLs.compactMap { URL(string: $0) }
        guard config.isPaired, !validURLs.isEmpty else {
            return nil
        }

        let sessionConfig = URLSessionConfiguration.ephemeral
        sessionConfig.timeoutIntervalForRequest = 2.8
        sessionConfig.timeoutIntervalForResource = 3.2
        sessionConfig.waitsForConnectivity = false
        let session = URLSession(configuration: sessionConfig)
        defer { session.invalidateAndCancel() }

        let token = config.deviceToken
        let winner: AgentConversationsWidgetSnapshot? = await withTaskGroup(of: AgentConversationsWidgetSnapshot?.self) { group in
            for baseURL in validURLs {
                group.addTask {
                    let endpoint = baseURL.appendingPathComponent("api/exa.language_server_pb.LanguageServerService/GetAllCascadeTrajectories")
                    var req = URLRequest(url: endpoint)
                    req.httpMethod = "POST"
                    req.timeoutInterval = 2.8
                    req.setValue("application/json", forHTTPHeaderField: "Content-Type")
                    req.setValue("1", forHTTPHeaderField: "Connect-Protocol-Version")
                    req.setValue("1", forHTTPHeaderField: "X-Widget-Lite")
                    if let token, !token.isEmpty {
                        req.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
                    }
                    req.httpBody = Data("{}".utf8)

                    do {
                        let (data, response) = try await session.data(for: req)
                        guard let http = response as? HTTPURLResponse,
                              (200...299).contains(http.statusCode),
                              data.count <= Self.maxExtensionPayloadBytes else {
                            return nil
                        }
                        let wire = try JSONDecoder().decode(TrajectoriesWireResponse.self, from: data)
                        return Self.buildConversationsSnapshot(from: wire, isDemo: false)
                    } catch {
                        return nil
                    }
                }
            }

            for await candidate in group {
                if let snap = candidate {
                    group.cancelAll()
                    return snap
                }
            }
            return nil
        }

        if let winner {
            saveConversationsSnapshot(winner)
        }
        return winner
    }

    private static func buildSnapshot(
        from wire: QuotaWireResponse,
        config: QuotaWidgetEndpointConfig,
        isDemo: Bool
    ) -> QuotaWidgetSnapshot? {
        guard let acc = wire.currentAccount else { return nil }
        let isFree = acc.subscriptionTier?.trimmingCharacters(in: .whitespacesAndNewlines).uppercased() == "FREE"
            || (acc.gemini5h == nil && acc.claude5h == nil)

        let gShort = acc.gemini5h.map {
            QuotaWidgetBucketSnapshot(
                remainingPercent: $0.resolvedPercent,
                resetAt: parseResetDate($0.resetTime),
                resetFriendly: $0.resetFriendly
            )
        }
        let gWeekly = acc.geminiWeekly.map {
            QuotaWidgetBucketSnapshot(
                remainingPercent: $0.resolvedPercent,
                resetAt: parseResetDate($0.resetTime),
                resetFriendly: $0.resetFriendly
            )
        }
        let cShort = acc.claude5h.map {
            QuotaWidgetBucketSnapshot(
                remainingPercent: $0.resolvedPercent,
                resetAt: parseResetDate($0.resetTime),
                resetFriendly: $0.resetFriendly
            )
        }
        let cWeekly = acc.claudeWeekly.map {
            QuotaWidgetBucketSnapshot(
                remainingPercent: $0.resolvedPercent,
                resetAt: parseResetDate($0.resetTime),
                resetFriendly: $0.resetFriendly
            )
        }

        let hasClaude = cShort != nil || cWeekly != nil
        let hasGemini = gShort != nil || gWeekly != nil
        guard hasClaude || hasGemini else { return nil }

        let useClaude = config.preferClaude ? hasClaude : !hasGemini && hasClaude
        let shortSnap = useClaude ? cShort : gShort
        let weeklySnap = useClaude ? cWeekly : gWeekly

        let name = (acc.name ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
        let email = (acc.email ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
        let label = !name.isEmpty ? name : email

        return QuotaWidgetSnapshot(
            providerTitle: useClaude ? "Claude" : "Gemini",
            isClaude: useClaude,
            accountLabel: label,
            subscriptionTier: acc.subscriptionTier,
            isFreeTier: isFree,
            isDemo: isDemo,
            shortWindow: shortSnap,
            weeklyWindow: weeklySnap,
            geminiShort: gShort,
            geminiWeekly: gWeekly,
            claudeShort: cShort,
            claudeWeekly: cWeekly,
            selectedBucketRaw: config.selectedBucketRaw,
            paceCursorEnabled: config.paceCursorEnabled,
            isEmailMasked: config.isEmailMasked,
            syncedAt: Date()
        )
    }

    private static func sanitizeWireTitle(_ raw: String?) -> String {
        guard let raw else { return "" }
        let stripped = raw
            .replacingOccurrences(of: "<[^>]+>", with: "", options: .regularExpression)
            .trimmingCharacters(in: .whitespacesAndNewlines)
        guard let firstLine = stripped
            .components(separatedBy: .newlines)
            .first(where: { !$0.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty }) else {
            return ""
        }
        let trimmed = firstLine.trimmingCharacters(in: .whitespacesAndNewlines)
        if trimmed.count > 44 {
            return String(trimmed.prefix(44)) + "…"
        }
        return trimmed
    }

    private static func buildConversationsSnapshot(
        from wire: TrajectoriesWireResponse,
        isDemo: Bool
    ) -> AgentConversationsWidgetSnapshot {
        let summaries = wire.trajectorySummaries ?? [:]
        var mapped: [AgentWidgetConversationItem] = []
        mapped.reserveCapacity(summaries.count)

        let untitled = String(localized: "未命名会话")

        for (id, summary) in summaries {
            if summary.isSubagent || summary.annotations?.archived == true {
                continue
            }

            let annotTitle = sanitizeWireTitle(summary.annotations?.title)
            let sumTitle = sanitizeWireTitle(summary.summary)
            let resolvedTitle: String
            if !annotTitle.isEmpty && annotTitle != "未命名会话" {
                resolvedTitle = annotTitle
            } else if !sumTitle.isEmpty && sumTitle != "未命名会话" {
                resolvedTitle = sumTitle
            } else {
                resolvedTitle = untitled
            }

            let hasRunningTasks = summary.hasRunningTasks == true
                || (summary.runningTaskCount ?? 0) > 0
                || (summary.backgroundTaskCount ?? 0) > 0
                || summary.notFullyIdle == true
            let hasRunningSubagents = summary.hasRunningSubagents == true

            let status: AgentWidgetConversationStatus
            if summary.needsInput == true {
                status = .action
            } else if summary.hasError == true || summary.status == "CASCADE_RUN_STATUS_ERROR" {
                status = .error
            } else if summary.status == "CASCADE_RUN_STATUS_RUNNING" || hasRunningSubagents {
                status = .running
            } else if summary.status == "CASCADE_RUN_STATUS_IDLE" {
                status = .idle
            } else {
                status = .unknown
            }

            let wsUri = summary.trajectoryMetadata?.workspaceUris?.first
                ?? summary.workspaces?.first?.workspaceFolderAbsoluteUri
                ?? ""
            let wsParts = wsUri.split(separator: "/").filter { !$0.isEmpty }
            let workspaceName: String
            if let explicit = summary.workspaceName?.trimmingCharacters(in: .whitespacesAndNewlines),
               !explicit.isEmpty {
                workspaceName = explicit
            } else {
                workspaceName = wsParts.last.map(String.init) ?? "Chat"
            }

            let isUnread = (status != .running && status != .action) && (summary.annotations?.markedAsUnread == true)

            mapped.append(
                AgentWidgetConversationItem(
                    id: id,
                    title: resolvedTitle,
                    workspaceName: workspaceName,
                    status: status,
                    stepCount: summary.stepCount ?? 0,
                    lastModified: parseResetDate(summary.lastModifiedTime),
                    isUnread: isUnread,
                    hasRunningTasks: hasRunningTasks,
                    isPinned: summary.annotations?.pinned == true
                )
            )
        }

        return finalizeConversationsSnapshot(mapped, isDemo: isDemo)
    }

    public static func finalizeConversationsSnapshot(
        _ items: [AgentWidgetConversationItem],
        isDemo: Bool,
        now: Date = Date()
    ) -> AgentConversationsWidgetSnapshot {
        let sorted = items.sorted { a, b in
            if a.priorityRank != b.priorityRank {
                return a.priorityRank < b.priorityRank
            }
            let dateA = a.lastModified ?? .distantPast
            let dateB = b.lastModified ?? .distantPast
            if dateA != dateB {
                return dateA > dateB
            }
            return a.id < b.id
        }
        let runningCount = items.filter { $0.isWorking && !$0.needsAction }.count
        let pendingCount = items.filter { $0.needsAction }.count
        return AgentConversationsWidgetSnapshot(
            items: Array(sorted.prefix(8)),
            runningCount: runningCount,
            pendingActionCount: pendingCount,
            totalCount: items.count,
            isDemo: isDemo,
            syncedAt: now
        )
    }

    #if !ACTIVITY_EXTENSION
    /// Converts a main-app `CockpitQuotaResponse` directly into a shared widget snapshot and triggers reload.
    public func publishFromApp(
        response: CockpitQuotaResponse?,
        preferredBucket: QuotaBucketKind,
        paceCursorEnabled: Bool = true,
        isEmailMasked: Bool = false,
        isDemo: Bool
    ) {
        guard let acc = response?.currentAccount else { return }

        let gShort = acc.gemini5h.map {
            QuotaWidgetBucketSnapshot(
                remainingPercent: $0.remainingPercent,
                resetAt: Self.parseResetDate($0.resetTime),
                resetFriendly: $0.resetFriendly
            )
        }
        let gWeekly = acc.geminiWeekly.map {
            QuotaWidgetBucketSnapshot(
                remainingPercent: $0.remainingPercent,
                resetAt: Self.parseResetDate($0.resetTime),
                resetFriendly: $0.resetFriendly
            )
        }
        let cShort = acc.claude5h.map {
            QuotaWidgetBucketSnapshot(
                remainingPercent: $0.remainingPercent,
                resetAt: Self.parseResetDate($0.resetTime),
                resetFriendly: $0.resetFriendly
            )
        }
        let cWeekly = acc.claudeWeekly.map {
            QuotaWidgetBucketSnapshot(
                remainingPercent: $0.remainingPercent,
                resetAt: Self.parseResetDate($0.resetTime),
                resetFriendly: $0.resetFriendly
            )
        }

        let hasClaude = cShort != nil || cWeekly != nil
        let hasGemini = gShort != nil || gWeekly != nil
        guard hasClaude || hasGemini else { return }

        let preferClaude = (preferredBucket == .claude5h || preferredBucket == .claudeWeekly)
        let useClaude = preferClaude ? hasClaude : !hasGemini && hasClaude
        let shortSnap = useClaude ? cShort : gShort
        let weeklySnap = useClaude ? cWeekly : gWeekly

        let name = acc.name.trimmingCharacters(in: .whitespacesAndNewlines)
        let label = !name.isEmpty ? name : acc.email
        let isFree = acc.subscriptionTier?.trimmingCharacters(in: .whitespacesAndNewlines).uppercased() == "FREE"
            || (acc.gemini5h == nil && acc.claude5h == nil)

        let snap = QuotaWidgetSnapshot(
            providerTitle: useClaude ? "Claude" : "Gemini",
            isClaude: useClaude,
            accountLabel: label,
            subscriptionTier: acc.subscriptionTier,
            isFreeTier: isFree,
            isDemo: isDemo,
            shortWindow: shortSnap,
            weeklyWindow: weeklySnap,
            geminiShort: gShort,
            geminiWeekly: gWeekly,
            claudeShort: cShort,
            claudeWeekly: cWeekly,
            selectedBucketRaw: preferredBucket.rawValue,
            paceCursorEnabled: paceCursorEnabled,
            isEmailMasked: isEmailMasked,
            syncedAt: Date()
        )
        saveSnapshot(snap)
        reloadWidgetTimelines()
    }

    /// Converts live main-app `ConversationItem` list into a shared widget snapshot.
    /// Avoids excessive WidgetKit timeline reloads when status & top items have not changed.
    public func publishConversationsFromApp(items: [ConversationItem], isDemo: Bool) {
        if isDemo {
            let demo = AgentConversationsWidgetSnapshot.demoPlaceholder()
            saveConversationsSnapshot(demo)
            reloadWidgetTimelines()
            return
        }

        let mapped: [AgentWidgetConversationItem] = items.compactMap { item in
            let isDraft = item.id.hasPrefix("local_draft_") || item.id.hasPrefix("draft_") || item.draftProject != nil
            guard !item.isSubagent, !isDraft else { return nil }
            let mappedStatus: AgentWidgetConversationStatus
            switch item.status {
            case .action: mappedStatus = .action
            case .running: mappedStatus = .running
            case .error: mappedStatus = .error
            case .idle: mappedStatus = .idle
            case .unknown: mappedStatus = .unknown
            }
            return AgentWidgetConversationItem(
                id: item.id,
                title: item.title,
                workspaceName: item.workspaceName,
                status: mappedStatus,
                stepCount: item.stepCount,
                lastModified: item.lastModified,
                isUnread: item.isUnread,
                hasRunningTasks: item.hasRunningTasks,
                isPinned: item.isPinned
            )
        }

        let now = Date()
        let nextSnap = Self.finalizeConversationsSnapshot(mapped, isDemo: false, now: now)
        let prev = loadConversationsSnapshot()
        let structurallyUnchanged = prev.map { $0.isStructurallyEqual(to: nextSnap) } ?? false

        saveConversationsSnapshot(nextSnap)

        let shouldReload = lastConversationsReloadLock.withLock { lastReload -> Bool in
            let elapsed = lastReload.map { now.timeIntervalSince($0) } ?? .greatestFiniteMagnitude
            if !structurallyUnchanged && elapsed >= 6 {
                lastReload = now
                return true
            }
            if elapsed >= 120 {
                lastReload = now
                return true
            }
            return false
        }
        if shouldReload {
            reloadWidgetTimelines()
        }
    }
    #endif
}

// MARK: - Interactive Widget Refresh Intents (Extension-Only so taps execute directly inside ActivityExtension)

#if ACTIVITY_EXTENSION
nonisolated public struct RefreshQuotaWidgetIntent: AppIntent {
    public static let title: LocalizedStringResource = "刷新额度"
    public static let description = IntentDescription("从本地 Multigravity 网关拉取最新 5 小时与 7 天额度。")
    public static let openAppWhenRun: Bool = false

    public init() {}

    public func perform() async throws -> some IntentResult {
        async let quotaTask = QuotaWidgetStore.shared.fetchLatestQuotaFromGateway()
        async let convTask = QuotaWidgetStore.shared.fetchLatestConversationsFromGateway()
        _ = await (quotaTask, convTask)
        QuotaWidgetStore.shared.reloadWidgetTimelines()
        return .result()
    }
}

nonisolated public struct RefreshAgentStatusWidgetIntent: AppIntent {
    public static let title: LocalizedStringResource = "刷新活跃对话"
    public static let description = IntentDescription("从本地 Multigravity 网关拉取最新活跃对话与 Agent 状态。")
    public static let openAppWhenRun: Bool = false

    public init() {}

    public func perform() async throws -> some IntentResult {
        _ = await QuotaWidgetStore.shared.fetchLatestConversationsFromGateway()
        WidgetCenter.shared.reloadTimelines(ofKind: QuotaWidgetConstants.agentWidgetKind)
        return .result()
    }
}
#endif
