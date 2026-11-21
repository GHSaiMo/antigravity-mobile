import AppIntents
import Foundation
import WidgetKit

// MARK: - Shared Constants

nonisolated public enum QuotaWidgetConstants {
    public static let appGroupID = "group.com.ghsaimo.antigravity"
    public static let widgetKind = "QuotaStatusWidget"
    public static let deepLinkURL = URL(string: "antigravity://quota")!
    public static let snapshotFileName = "quota_widget_snapshot.json"
    public static let configFileName = "quota_widget_config.json"
}

// MARK: - Shared Snapshot Models

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
}

nonisolated public struct QuotaWidgetSnapshot: Codable, Hashable, Sendable {
    public let providerTitle: String
    public let isClaude: Bool
    public let accountLabel: String
    public let isFreeTier: Bool
    public let isDemo: Bool
    public let shortWindow: QuotaWidgetBucketSnapshot?
    public let weeklyWindow: QuotaWidgetBucketSnapshot?
    public let syncedAt: Date

    public init(
        providerTitle: String,
        isClaude: Bool,
        accountLabel: String,
        isFreeTier: Bool,
        isDemo: Bool,
        shortWindow: QuotaWidgetBucketSnapshot?,
        weeklyWindow: QuotaWidgetBucketSnapshot?,
        syncedAt: Date
    ) {
        self.providerTitle = providerTitle
        self.isClaude = isClaude
        self.accountLabel = accountLabel
        self.isFreeTier = isFreeTier
        self.isDemo = isDemo
        self.shortWindow = shortWindow
        self.weeklyWindow = weeklyWindow
        self.syncedAt = syncedAt
    }

    public var primaryWindow: QuotaWidgetBucketSnapshot? {
        shortWindow ?? weeklyWindow
    }

    public var primaryWindowTag: String {
        shortWindow != nil ? "5h" : "7d"
    }

    public static func demoPlaceholder(now: Date = Date()) -> QuotaWidgetSnapshot {
        QuotaWidgetSnapshot(
            providerTitle: "Gemini",
            isClaude: false,
            accountLabel: "Demo",
            isFreeTier: false,
            isDemo: true,
            shortWindow: QuotaWidgetBucketSnapshot(
                remainingPercent: 78,
                resetAt: now.addingTimeInterval(2 * 3600 + 14 * 60),
                resetFriendly: "2h 14m"
            ),
            weeklyWindow: QuotaWidgetBucketSnapshot(
                remainingPercent: 64,
                resetAt: now.addingTimeInterval(4 * 86400),
                resetFriendly: "4d"
            ),
            syncedAt: now
        )
    }
}

nonisolated public struct QuotaWidgetEndpointConfig: Codable, Sendable {
    public let candidateBaseURLs: [String]
    public let deviceToken: String?
    public let preferClaude: Bool
    public let isDemoMode: Bool
    public let isPaired: Bool
    public let updatedAt: Date

    public init(
        candidateBaseURLs: [String],
        deviceToken: String?,
        preferClaude: Bool,
        isDemoMode: Bool,
        isPaired: Bool,
        updatedAt: Date = Date()
    ) {
        self.candidateBaseURLs = candidateBaseURLs
        self.deviceToken = deviceToken
        self.preferClaude = preferClaude
        self.isDemoMode = isDemoMode
        self.isPaired = isPaired
        self.updatedAt = updatedAt
    }
}

// MARK: - Lightweight Wire DTO (decoded by both Main App and ActivityExtension)

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

// MARK: - Shared App Group Store & Background Fetcher

nonisolated public final class QuotaWidgetStore: Sendable {
    public static let shared = QuotaWidgetStore()

    private init() {}

    private var containerURL: URL? {
        FileManager.default.containerURL(forSecurityApplicationGroupIdentifier: QuotaWidgetConstants.appGroupID)
    }

    private var snapshotURL: URL? {
        containerURL?.appendingPathComponent(QuotaWidgetConstants.snapshotFileName)
    }

    private var configURL: URL? {
        containerURL?.appendingPathComponent(QuotaWidgetConstants.configFileName)
    }

    public func loadSnapshot() -> QuotaWidgetSnapshot? {
        guard let url = snapshotURL,
              let data = try? Data(contentsOf: url) else {
            return nil
        }
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601
        return try? decoder.decode(QuotaWidgetSnapshot.self, from: data)
    }

    public func saveSnapshot(_ snapshot: QuotaWidgetSnapshot) {
        guard let url = snapshotURL else { return }
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        guard let data = try? encoder.encode(snapshot) else { return }
        try? data.write(to: url, options: [.atomic, .completeFileProtectionUntilFirstUserAuthentication])
    }

    public func clearSnapshot() {
        guard let url = snapshotURL else { return }
        try? FileManager.default.removeItem(at: url)
    }

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
    }

    public func clearAll() {
        clearSnapshot()
        if let url = configURL {
            try? FileManager.default.removeItem(at: url)
        }
    }

    public func reloadWidgetTimelines() {
        WidgetCenter.shared.reloadTimelines(ofKind: QuotaWidgetConstants.widgetKind)
    }

    /// Parses gateway ISO8601 reset timestamps into a `Date` for native WidgetKit ticking countdowns.
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

    /// Lightweight background fetch used by WidgetKit timelines inside `ActivityExtension`.
    /// Tries saved candidate gateway URLs in order with a tight timeout and caches on success.
    public func fetchLatestQuotaFromGateway() async -> QuotaWidgetSnapshot? {
        guard let config = loadConfig() else {
            return nil
        }
        if config.isDemoMode {
            let demo = QuotaWidgetSnapshot.demoPlaceholder(now: Date())
            saveSnapshot(demo)
            return demo
        }
        guard config.isPaired, !config.candidateBaseURLs.isEmpty else {
            return nil
        }

        let sessionConfig = URLSessionConfiguration.ephemeral
        sessionConfig.timeoutIntervalForRequest = 6
        sessionConfig.timeoutIntervalForResource = 8
        sessionConfig.waitsForConnectivity = false
        let session = URLSession(configuration: sessionConfig)
        defer { session.invalidateAndCancel() }

        for rawBase in config.candidateBaseURLs {
            guard let baseURL = URL(string: rawBase) else { continue }
            let endpoint = baseURL.appendingPathComponent("api/v1/cockpit/quotas")
            var req = URLRequest(url: endpoint)
            req.httpMethod = "GET"
            req.timeoutInterval = 6
            if let token = config.deviceToken, !token.isEmpty {
                req.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
            }

            do {
                let (data, response) = try await session.data(for: req)
                guard let http = response as? HTTPURLResponse, (200...299).contains(http.statusCode) else {
                    continue
                }
                let wire = try JSONDecoder().decode(QuotaWireResponse.self, from: data)
                if let snap = Self.buildSnapshot(from: wire, preferClaude: config.preferClaude, isDemo: false) {
                    saveSnapshot(snap)
                    return snap
                }
            } catch {
                continue
            }
        }
        return nil
    }

    private static func buildSnapshot(from wire: QuotaWireResponse, preferClaude: Bool, isDemo: Bool) -> QuotaWidgetSnapshot? {
        guard let acc = wire.currentAccount else { return nil }
        let isFree = acc.subscriptionTier?.trimmingCharacters(in: .whitespacesAndNewlines).uppercased() == "FREE"
            || (acc.gemini5h == nil && acc.claude5h == nil)

        let hasClaude = acc.claude5h != nil || acc.claudeWeekly != nil
        let hasGemini = acc.gemini5h != nil || acc.geminiWeekly != nil
        let useClaude = preferClaude ? hasClaude : !hasGemini && hasClaude

        let rawShort = useClaude ? acc.claude5h : acc.gemini5h
        let rawWeekly = useClaude ? acc.claudeWeekly : acc.geminiWeekly
        guard rawShort != nil || rawWeekly != nil else { return nil }

        let shortSnap = rawShort.map {
            QuotaWidgetBucketSnapshot(
                remainingPercent: $0.resolvedPercent,
                resetAt: parseResetDate($0.resetTime),
                resetFriendly: $0.resetFriendly
            )
        }
        let weeklySnap = rawWeekly.map {
            QuotaWidgetBucketSnapshot(
                remainingPercent: $0.resolvedPercent,
                resetAt: parseResetDate($0.resetTime),
                resetFriendly: $0.resetFriendly
            )
        }

        let name = (acc.name ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
        let email = (acc.email ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
        let label = !name.isEmpty ? name : email

        return QuotaWidgetSnapshot(
            providerTitle: useClaude ? "Claude" : "Gemini",
            isClaude: useClaude,
            accountLabel: label,
            isFreeTier: isFree,
            isDemo: isDemo,
            shortWindow: shortSnap,
            weeklyWindow: weeklySnap,
            syncedAt: Date()
        )
    }

    #if !ACTIVITY_EXTENSION
    /// Converts a main-app `CockpitQuotaResponse` directly into a shared widget snapshot and triggers reload.
    public func publishFromApp(response: CockpitQuotaResponse?, preferredBucket: QuotaBucketKind, isDemo: Bool) {
        guard let acc = response?.currentAccount else { return }
        let preferClaude = (preferredBucket == .claude5h || preferredBucket == .claudeWeekly)
        let hasClaude = acc.claude5h != nil || acc.claudeWeekly != nil
        let hasGemini = acc.gemini5h != nil || acc.geminiWeekly != nil
        let useClaude = preferClaude ? hasClaude : !hasGemini && hasClaude

        let rawShort = useClaude ? acc.claude5h : acc.gemini5h
        let rawWeekly = useClaude ? acc.claudeWeekly : acc.geminiWeekly
        guard rawShort != nil || rawWeekly != nil else { return }

        let shortSnap = rawShort.map {
            QuotaWidgetBucketSnapshot(
                remainingPercent: $0.remainingPercent,
                resetAt: Self.parseResetDate($0.resetTime),
                resetFriendly: $0.resetFriendly
            )
        }
        let weeklySnap = rawWeekly.map {
            QuotaWidgetBucketSnapshot(
                remainingPercent: $0.remainingPercent,
                resetAt: Self.parseResetDate($0.resetTime),
                resetFriendly: $0.resetFriendly
            )
        }

        let name = acc.name.trimmingCharacters(in: .whitespacesAndNewlines)
        let label = !name.isEmpty ? name : acc.email
        let isFree = acc.subscriptionTier?.trimmingCharacters(in: .whitespacesAndNewlines).uppercased() == "FREE"
            || (acc.gemini5h == nil && acc.claude5h == nil)

        let snap = QuotaWidgetSnapshot(
            providerTitle: useClaude ? "Claude" : "Gemini",
            isClaude: useClaude,
            accountLabel: label,
            isFreeTier: isFree,
            isDemo: isDemo,
            shortWindow: shortSnap,
            weeklyWindow: weeklySnap,
            syncedAt: Date()
        )
        saveSnapshot(snap)
        reloadWidgetTimelines()
    }
    #endif
}

// MARK: - Interactive Widget Refresh Intent (Home Screen tap-to-refresh without opening App)

nonisolated public struct RefreshQuotaWidgetIntent: AppIntent {
    public static let title: LocalizedStringResource = "刷新额度"
    public static let description = IntentDescription("从本地 Multigravity 网关拉取最新 5 小时与 7 天额度。")
    public static let openAppWhenRun: Bool = false

    public init() {}

    public func perform() async throws -> some IntentResult {
        #if !ACTIVITY_EXTENSION
        let (isPaired, isDemo, serverURL, preferredBucket) = await MainActor.run {
            let s = AppSettings.shared
            s.syncWidgetConfiguration(reloadWidget: false)
            return (s.isPaired, s.isDemoMode, s.serverURL, s.selectedQuotaBucket)
        }
        if isDemo {
            let demo = QuotaWidgetSnapshot.demoPlaceholder(now: Date())
            QuotaWidgetStore.shared.saveSnapshot(demo)
            QuotaWidgetStore.shared.reloadWidgetTimelines()
            return .result()
        }
        if isPaired, let url = serverURL {
            if let response = try? await APIClient.shared.fetchCockpitQuotas(baseURL: url) {
                QuotaWidgetStore.shared.publishFromApp(
                    response: response,
                    preferredBucket: preferredBucket,
                    isDemo: false
                )
                return .result()
            }
        }
        QuotaWidgetStore.shared.reloadWidgetTimelines()
        #else
        _ = await QuotaWidgetStore.shared.fetchLatestQuotaFromGateway()
        QuotaWidgetStore.shared.reloadWidgetTimelines()
        #endif
        return .result()
    }
}
