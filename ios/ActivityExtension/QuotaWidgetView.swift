import AppIntents
import Foundation
import SwiftUI
import WidgetKit

// MARK: - Timeline Entry & Provider (AppIntentConfiguration)

nonisolated struct QuotaWidgetEntry: TimelineEntry {
    let date: Date
    let snapshot: QuotaWidgetSnapshot?
    let conversations: AgentConversationsWidgetSnapshot?
    let providerPreference: QuotaWidgetProviderPreference
    let isPaired: Bool
    let isStale: Bool
}

nonisolated struct QuotaTimelineProvider: AppIntentTimelineProvider {
    func placeholder(in context: Context) -> QuotaWidgetEntry {
        let now = Date()
        return QuotaWidgetEntry(
            date: now,
            snapshot: QuotaWidgetSnapshot.demoPlaceholder(now: now),
            conversations: AgentConversationsWidgetSnapshot.demoPlaceholder(now: now),
            providerPreference: .followApp,
            isPaired: true,
            isStale: false
        )
    }

    func snapshot(for configuration: QuotaWidgetConfigurationIntent, in context: Context) async -> QuotaWidgetEntry {
        let now = Date()
        if context.isPreview {
            return QuotaWidgetEntry(
                date: now,
                snapshot: QuotaWidgetStore.shared.loadSnapshot() ?? QuotaWidgetSnapshot.demoPlaceholder(now: now),
                conversations: QuotaWidgetStore.shared.loadConversationsSnapshot() ?? AgentConversationsWidgetSnapshot.demoPlaceholder(now: now),
                providerPreference: configuration.provider,
                isPaired: true,
                isStale: false
            )
        }
        let config = QuotaWidgetStore.shared.loadConfig()
        let cachedQuota = QuotaWidgetStore.shared.loadSnapshot()
        let cachedConvs = QuotaWidgetStore.shared.loadConversationsSnapshot()
        let paired = (config?.isPaired ?? false) || (cachedQuota != nil)
        return QuotaWidgetEntry(
            date: now,
            snapshot: cachedQuota ?? (paired ? QuotaWidgetSnapshot.demoPlaceholder(now: now) : nil),
            conversations: cachedConvs ?? (paired ? AgentConversationsWidgetSnapshot.demoPlaceholder(now: now) : nil),
            providerPreference: configuration.provider,
            isPaired: paired,
            isStale: false
        )
    }

    func timeline(for configuration: QuotaWidgetConfigurationIntent, in context: Context) async -> Timeline<QuotaWidgetEntry> {
        let now = Date()
        let config = QuotaWidgetStore.shared.loadConfig()
        let needsConversations = (context.family == .systemLarge || context.family == .systemExtraLarge)

        let freshQuota = await QuotaWidgetStore.shared.fetchLatestQuotaFromGateway()
        let freshConvs: AgentConversationsWidgetSnapshot?
        if needsConversations {
            freshConvs = await QuotaWidgetStore.shared.fetchLatestConversationsFromGateway()
        } else {
            freshConvs = QuotaWidgetStore.shared.loadConversationsSnapshot()
        }

        let resolvedConvs = freshConvs ?? QuotaWidgetStore.shared.loadConversationsSnapshot()

        let entry: QuotaWidgetEntry
        if let freshQuota {
            entry = QuotaWidgetEntry(
                date: now,
                snapshot: freshQuota,
                conversations: resolvedConvs,
                providerPreference: configuration.provider,
                isPaired: true,
                isStale: false
            )
        } else if let cached = QuotaWidgetStore.shared.loadSnapshot() {
            let stale = now.timeIntervalSince(cached.syncedAt) > 25 * 60
            entry = QuotaWidgetEntry(
                date: now,
                snapshot: cached,
                conversations: resolvedConvs,
                providerPreference: configuration.provider,
                isPaired: true,
                isStale: stale
            )
        } else {
            let paired = config?.isPaired ?? false
            entry = QuotaWidgetEntry(
                date: now,
                snapshot: nil,
                conversations: resolvedConvs,
                providerPreference: configuration.provider,
                isPaired: paired,
                isStale: false
            )
        }

        let nextRefresh = Calendar.current.date(byAdding: .minute, value: 20, to: now)
            ?? now.addingTimeInterval(20 * 60)
        return Timeline(entries: [entry], policy: .after(nextRefresh))
    }
}

// MARK: - Widget Definition (All 7 Families: Inline / Circular / Rectangular / Small / Medium / Large / iPad ExtraLarge)

public struct QuotaStatusWidget: Widget {
    public init() {}

    public var body: some WidgetConfiguration {
        AppIntentConfiguration(
            kind: QuotaWidgetConstants.widgetKind,
            intent: QuotaWidgetConfigurationIntent.self,
            provider: QuotaTimelineProvider()
        ) { entry in
            QuotaWidgetEntryView(entry: entry)
        }
        .configurationDisplayName(String(localized: "额度监控"))
        .description(String(localized: "在锁屏、主屏幕、iPad 与 CarPlay 查看 5 小时与 7 天额度余量、重置倒计时与活跃对话。"))
        .supportedFamilies([
            .accessoryInline,
            .accessoryCircular,
            .accessoryRectangular,
            .systemSmall,
            .systemMedium,
            .systemLarge,
            .systemExtraLarge,
        ])
    }
}

// MARK: - Root Family Switcher

private struct QuotaWidgetEntryView: View {
    let entry: QuotaWidgetEntry
    @Environment(\.widgetFamily) private var family

    private var usesSystemCardBackground: Bool {
        switch family {
        case .systemSmall, .systemMedium, .systemLarge, .systemExtraLarge:
            return true
        default:
            return false
        }
    }

    var body: some View {
        Group {
            switch family {
            case .accessoryInline:
                QuotaInlineView(entry: entry)
                    .widgetURL(QuotaWidgetConstants.deepLinkURL)
            case .accessoryCircular:
                QuotaCircularView(entry: entry)
                    .widgetURL(QuotaWidgetConstants.deepLinkURL)
            case .accessoryRectangular:
                QuotaRectangularView(entry: entry)
                    .widgetURL(QuotaWidgetConstants.deepLinkURL)
            case .systemSmall:
                QuotaSmallHomeView(entry: entry)
                    .widgetURL(QuotaWidgetConstants.deepLinkURL)
            case .systemMedium:
                QuotaMediumHomeView(entry: entry)
                    .widgetURL(QuotaWidgetConstants.deepLinkURL)
            case .systemLarge:
                QuotaLargeHomeView(entry: entry)
            case .systemExtraLarge:
                QuotaExtraLargeHomeView(entry: entry)
            default:
                QuotaSmallHomeView(entry: entry)
                    .widgetURL(QuotaWidgetConstants.deepLinkURL)
            }
        }
        .containerBackground(for: .widget) {
            if usesSystemCardBackground {
                WidgetCardBackground()
            } else {
                Color.clear
            }
        }
    }
}

// MARK: - 1. Lock Screen Inline (.accessoryInline)

private struct QuotaInlineView: View {
    let entry: QuotaWidgetEntry

    var body: some View {
        if let snap = entry.snapshot,
           let resolved = snap.resolvedProvider(for: entry.providerPreference),
           let primary = resolved.primaryWindow {
            Label {
                if let resetAt = primary.resetAt, resetAt > entry.date {
                    Text("\(resolved.providerTitle) \(primary.intPercent)% · \(resetAt, style: .relative)")
                } else if let friendly = primary.resetFriendly, !friendly.isEmpty {
                    Text("\(resolved.providerTitle) \(primary.intPercent)% · \(friendly)")
                } else {
                    Text("\(resolved.providerTitle) \(primary.intPercent)%")
                }
            } icon: {
                Image(systemName: "bolt.fill")
            }
        } else {
            Label {
                Text(entry.isPaired ? String(localized: "等待同步额度") : String(localized: "Multigravity 未配对"))
            } icon: {
                Image(systemName: "bolt.slash.fill")
            }
        }
    }
}

// MARK: - 2. Lock Screen Circular (.accessoryCircular)

private struct QuotaCircularView: View {
    let entry: QuotaWidgetEntry

    var body: some View {
        if let snap = entry.snapshot,
           let resolved = snap.resolvedProvider(for: entry.providerPreference),
           let primary = resolved.primaryWindow {
            Gauge(value: primary.remainingPercent, in: 0...100) {
                Text(circularTag(for: resolved))
                    .font(.system(size: 9, weight: .bold, design: .rounded))
            } currentValueLabel: {
                Text("\(primary.intPercent)%")
                    .font(.system(size: 13, weight: .bold, design: .rounded))
                    .minimumScaleFactor(0.75)
            }
            .gaugeStyle(.accessoryCircular)
            .tint(quotaTintColor(percent: primary.remainingPercent))
            .widgetAccentable()
        } else {
            ZStack {
                AccessoryWidgetBackground()
                VStack(spacing: 1) {
                    Image(systemName: "bolt.slash.fill")
                        .font(.system(size: 14, weight: .semibold))
                    Text("--")
                        .font(.system(size: 11, weight: .bold, design: .rounded))
                }
            }
        }
    }

    private func circularTag(for resolved: ResolvedProviderQuota) -> String {
        switch entry.providerPreference {
        case .gemini:
            return "G·\(resolved.primaryWindowTag)"
        case .claude:
            return "C·\(resolved.primaryWindowTag)"
        case .followApp:
            return resolved.primaryWindowTag
        }
    }
}

// MARK: - 3. Lock Screen Rectangular (.accessoryRectangular)

private struct QuotaRectangularView: View {
    let entry: QuotaWidgetEntry

    var body: some View {
        if let snap = entry.snapshot,
           let resolved = snap.resolvedProvider(for: entry.providerPreference) {
            VStack(alignment: .leading, spacing: 3.5) {
                HStack(spacing: 4) {
                    Image(systemName: "bolt.fill")
                        .font(.system(size: 10, weight: .bold))
                        .widgetAccentable()
                    Text(resolved.providerTitle)
                        .font(.system(size: 12, weight: .bold))
                        .lineLimit(1)
                    Spacer(minLength: 4)
                    resetCountdownView(for: resolved.primaryWindow, now: entry.date, compact: true)
                        .font(.system(size: 10.5, weight: .medium, design: .rounded))
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                }

                if let short = resolved.shortWindow {
                    rectangularBarRow(tag: "5h", bucket: short)
                }
                if let weekly = resolved.weeklyWindow {
                    rectangularBarRow(tag: "7d", bucket: weekly)
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
        } else {
            VStack(alignment: .leading, spacing: 4) {
                HStack(spacing: 4) {
                    Image(systemName: "bolt.fill")
                        .font(.system(size: 11, weight: .bold))
                    Text("Multigravity")
                        .font(.system(size: 12, weight: .bold))
                }
                Text(entry.isPaired ? String(localized: "打开 App 同步额度") : String(localized: "请先打开 App 配对"))
                    .font(.system(size: 11))
                    .foregroundStyle(.secondary)
                    .lineLimit(2)
            }
            .frame(maxWidth: .infinity, alignment: .leading)
        }
    }

    private func rectangularBarRow(tag: String, bucket: QuotaWidgetBucketSnapshot) -> some View {
        HStack(spacing: 5) {
            Text(tag)
                .font(.system(size: 10, weight: .semibold, design: .monospaced))
                .foregroundStyle(.secondary)
                .frame(width: 18, alignment: .leading)

            ProgressView(value: bucket.clampedFraction)
                .progressViewStyle(.linear)
                .tint(quotaTintColor(percent: bucket.remainingPercent))
                .widgetAccentable()

            Text("\(bucket.intPercent)%")
                .font(.system(size: 10.5, weight: .bold, design: .monospaced))
                .frame(width: 30, alignment: .trailing)
        }
    }
}

// MARK: - 4. Home Screen / StandBy / CarPlay Small (.systemSmall)

private struct QuotaSmallHomeView: View {
    let entry: QuotaWidgetEntry
    @Environment(\.colorScheme) private var colorScheme
    @Environment(\.widgetRenderingMode) private var renderingMode

    private var isDark: Bool { colorScheme == .dark }
    private var isFullColor: Bool { renderingMode == .fullColor }

    var body: some View {
        if let snap = entry.snapshot,
           let resolved = snap.resolvedProvider(for: entry.providerPreference),
           let primary = resolved.primaryWindow {
            VStack(alignment: .leading, spacing: 0) {
                HStack(alignment: .center, spacing: 5) {
                    Image(systemName: "bolt.fill")
                        .font(.system(size: 11.5, weight: .bold))
                        .foregroundStyle(isFullColor ? quotaTintColor(percent: primary.remainingPercent) : .primary)
                        .widgetAccentable()

                    Text(resolved.providerTitle)
                        .font(.system(size: 13.5, weight: .bold))
                        .foregroundStyle(.primary)
                        .lineLimit(1)

                    Spacer(minLength: 4)

                    WidgetRefreshIconButton(intent: RefreshQuotaWidgetIntent())
                }

                Spacer(minLength: 6)

                HStack(alignment: .lastTextBaseline, spacing: 5) {
                    Text("\(primary.intPercent)%")
                        .font(.system(size: 30, weight: .bold, design: .rounded))
                        .foregroundStyle(isFullColor ? quotaTintColor(percent: primary.remainingPercent) : .primary)
                        .minimumScaleFactor(0.75)
                        .lineLimit(1)
                        .widgetAccentable()

                    Text(resolved.primaryWindowTag)
                        .font(.system(size: 11, weight: .bold, design: .rounded))
                        .foregroundStyle(.secondary)
                        .padding(.horizontal, 5.5)
                        .padding(.vertical, 1.5)
                        .background(
                            Capsule().fill(isDark ? Color.white.opacity(0.10) : Color.black.opacity(0.06))
                        )
                }

                HStack(spacing: 3) {
                    Image(systemName: "clock")
                        .font(.system(size: 9.5, weight: .medium))
                    resetCountdownView(for: primary, now: entry.date, compact: false)
                        .font(.system(size: 10.5, weight: .medium, design: .rounded))
                        .lineLimit(1)
                }
                .foregroundStyle(.secondary)
                .padding(.top, 2)

                Spacer(minLength: 8)

                VStack(spacing: 5.5) {
                    if let short = resolved.shortWindow {
                        QuotaProgressRow(tag: "5h", bucket: short)
                    }
                    if let weekly = resolved.weeklyWindow {
                        QuotaProgressRow(tag: "7d", bucket: weekly)
                    }
                }

                Spacer(minLength: 5)

                WidgetSyncFooter(syncedAt: snap.syncedAt, isDemo: snap.isDemo, isStale: entry.isStale)
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .leading)
        } else {
            QuotaEmptyCardView(isPaired: entry.isPaired, compact: true)
        }
    }
}

// MARK: - 5. Home Screen Medium (.systemMedium — Dual-Model Comparison or Rich Single-Model Fallback)

private struct QuotaMediumHomeView: View {
    let entry: QuotaWidgetEntry
    @Environment(\.colorScheme) private var colorScheme
    @Environment(\.widgetRenderingMode) private var renderingMode

    private var isDark: Bool { colorScheme == .dark }
    private var isFullColor: Bool { renderingMode == .fullColor }

    var body: some View {
        if let snap = entry.snapshot {
            VStack(alignment: .leading, spacing: 8) {
                // Header: Account + Tier + Sync Time + Refresh
                HStack(alignment: .center, spacing: 6) {
                    Image(systemName: "bolt.fill")
                        .font(.system(size: 11.5, weight: .bold))
                        .foregroundStyle(isFullColor ? Color.accentColor : .primary)
                        .widgetAccentable()

                    Text(snap.displayAccountTitle)
                        .font(.system(size: 13, weight: .bold))
                        .foregroundStyle(.primary)
                        .lineLimit(1)

                    if let tier = snap.tierBadgeLabel {
                        WidgetTierBadge(label: tier)
                    }

                    Spacer(minLength: 6)

                    WidgetSyncFooter(syncedAt: snap.syncedAt, isDemo: snap.isDemo, isStale: entry.isStale)
                        .fixedSize()

                    WidgetRefreshIconButton(intent: RefreshQuotaWidgetIntent())
                }

                if snap.hasBothProviders,
                   let gemini = snap.geminiBundle,
                   let claude = snap.claudeBundle {
                    HStack(alignment: .top, spacing: 12) {
                        ProviderQuotaColumn(provider: gemini, now: entry.date, compact: false)
                            .frame(maxWidth: .infinity, alignment: .leading)

                        Rectangle()
                            .fill(isDark ? Color.white.opacity(0.10) : Color.black.opacity(0.08))
                            .frame(width: 1)
                            .padding(.vertical, 2)

                        ProviderQuotaColumn(provider: claude, now: entry.date, compact: false)
                            .frame(maxWidth: .infinity, alignment: .leading)
                    }
                    .frame(maxHeight: .infinity)
                } else if let resolved = snap.resolvedProvider(for: entry.providerPreference),
                          let primary = resolved.primaryWindow {
                    // Graceful single-provider wide layout with weekly pace pill
                    HStack(alignment: .center, spacing: 14) {
                        VStack(alignment: .leading, spacing: 4) {
                            HStack(alignment: .lastTextBaseline, spacing: 5) {
                                Text("\(primary.intPercent)%")
                                    .font(.system(size: 34, weight: .bold, design: .rounded))
                                    .foregroundStyle(isFullColor ? quotaTintColor(percent: primary.remainingPercent) : .primary)
                                    .minimumScaleFactor(0.75)
                                    .lineLimit(1)
                                    .widgetAccentable()

                                Text(resolved.primaryWindowTag)
                                    .font(.system(size: 11, weight: .bold, design: .rounded))
                                    .foregroundStyle(.secondary)
                                    .padding(.horizontal, 5.5)
                                    .padding(.vertical, 1.5)
                                    .background(
                                        Capsule().fill(isDark ? Color.white.opacity(0.10) : Color.black.opacity(0.06))
                                    )
                            }

                            Text(resolved.providerTitle)
                                .font(.system(size: 12, weight: .semibold))
                                .foregroundStyle(.primary)

                            HStack(spacing: 3) {
                                Image(systemName: "clock")
                                    .font(.system(size: 9.5, weight: .medium))
                                resetCountdownView(for: primary, now: entry.date, compact: false)
                                    .font(.system(size: 10.5, weight: .medium, design: .rounded))
                                    .lineLimit(1)
                            }
                            .foregroundStyle(.secondary)
                        }
                        .frame(maxWidth: .infinity, alignment: .leading)

                        VStack(alignment: .leading, spacing: 8) {
                            if let short = resolved.shortWindow {
                                QuotaDetailedBarRow(tag: "5h", bucket: short, now: entry.date)
                            }
                            if let weekly = resolved.weeklyWindow {
                                QuotaDetailedBarRow(tag: "7d", bucket: weekly, now: entry.date)
                                if let pace = weekly.evaluatePace(windowHours: 168.0, now: entry.date) {
                                    WeeklyPacePill(pace: pace)
                                }
                            }
                        }
                        .frame(maxWidth: .infinity, alignment: .leading)
                    }
                    .frame(maxHeight: .infinity)
                }
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .leading)
        } else {
            QuotaEmptyCardView(isPaired: entry.isPaired, compact: false)
        }
    }
}

// MARK: - 6. Home Screen Large (.systemLarge — Hybrid Cockpit: Dual Quota + Active Conversations)

private struct QuotaLargeHomeView: View {
    let entry: QuotaWidgetEntry
    @Environment(\.colorScheme) private var colorScheme
    @Environment(\.widgetRenderingMode) private var renderingMode

    private var isDark: Bool { colorScheme == .dark }
    private var isFullColor: Bool { renderingMode == .fullColor }

    var body: some View {
        if let snap = entry.snapshot {
            VStack(alignment: .leading, spacing: 10) {
                // Header: Account + Tier + Agent Status Summary + Refresh
                HStack(alignment: .center, spacing: 6) {
                    Image(systemName: "bolt.fill")
                        .font(.system(size: 12, weight: .bold))
                        .foregroundStyle(isFullColor ? Color.accentColor : .primary)
                        .widgetAccentable()

                    Text(snap.displayAccountTitle)
                        .font(.system(size: 13.5, weight: .bold))
                        .foregroundStyle(.primary)
                        .lineLimit(1)

                    if let tier = snap.tierBadgeLabel {
                        WidgetTierBadge(label: tier)
                    }

                    Spacer(minLength: 4)

                    if let convs = entry.conversations {
                        AgentStatusCompactSummaryPills(snapshot: convs, maxPills: 1)
                    }

                    WidgetRefreshIconButton(intent: RefreshQuotaWidgetIntent())
                }

                // Top Half: Dual-Model Quota Card (tapping opens Quota Sheet)
                Link(destination: QuotaWidgetConstants.deepLinkURL) {
                    Group {
                        if snap.hasBothProviders,
                           let gemini = snap.geminiBundle,
                           let claude = snap.claudeBundle {
                            HStack(alignment: .top, spacing: 12) {
                                ProviderQuotaColumn(provider: gemini, now: entry.date, compact: true)
                                    .frame(maxWidth: .infinity, alignment: .leading)

                                Rectangle()
                                    .fill(isDark ? Color.white.opacity(0.10) : Color.black.opacity(0.08))
                                    .frame(width: 1)
                                    .padding(.vertical, 2)

                                ProviderQuotaColumn(provider: claude, now: entry.date, compact: true)
                                    .frame(maxWidth: .infinity, alignment: .leading)
                            }
                        } else if let resolved = snap.resolvedProvider(for: entry.providerPreference) {
                            ProviderQuotaColumn(provider: resolved, now: entry.date, compact: false)
                                .frame(maxWidth: .infinity, alignment: .leading)
                        }
                    }
                    .padding(.horizontal, 11)
                    .padding(.vertical, 9)
                    .background(
                        RoundedRectangle(cornerRadius: 12, style: .continuous)
                            .fill(isDark ? Color.white.opacity(0.05) : Color.black.opacity(0.035))
                    )
                }
                .buttonStyle(.plain)

                // Bottom Half: Top 3 Priority Active Conversations (each individually tappable)
                VStack(alignment: .leading, spacing: 6) {
                    HStack(alignment: .center) {
                        Text(String(localized: "活跃对话"))
                            .font(.system(size: 11.5, weight: .bold))
                            .foregroundStyle(.secondary)

                        Spacer()

                        WidgetSyncFooter(syncedAt: snap.syncedAt, isDemo: snap.isDemo, isStale: entry.isStale)
                    }

                    if let convs = entry.conversations, !convs.items.isEmpty {
                        VStack(spacing: 5.5) {
                            ForEach(Array(convs.items.prefix(3))) { item in
                                WidgetConversationRow(item: item, now: entry.date, showStepCount: false)
                            }
                        }
                    } else {
                        Link(destination: QuotaWidgetConstants.conversationsDeepLinkURL) {
                            HStack(spacing: 6) {
                                Image(systemName: "bubble.left.and.text.bubble.right")
                                    .font(.system(size: 12, weight: .semibold))
                                    .foregroundStyle(.secondary)
                                Text(String(localized: "暂无活跃对话，轻点打开 Multigravity"))
                                    .font(.system(size: 12, weight: .medium))
                                    .foregroundStyle(.secondary)
                                    .lineLimit(1)
                                Spacer()
                            }
                            .padding(.vertical, 10)
                            .padding(.horizontal, 10)
                            .background(
                                RoundedRectangle(cornerRadius: 10, style: .continuous)
                                    .fill(isDark ? Color.white.opacity(0.04) : Color.black.opacity(0.03))
                            )
                        }
                        .buttonStyle(.plain)
                    }
                }

                Spacer(minLength: 0)
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
        } else {
            QuotaEmptyCardView(isPaired: entry.isPaired, compact: false)
                .widgetURL(QuotaWidgetConstants.deepLinkURL)
        }
    }
}

// MARK: - 7. iPad Extra Large (.systemExtraLarge — Left Dual Quota + Pace, Right 5 Active Conversations)

private struct QuotaExtraLargeHomeView: View {
    let entry: QuotaWidgetEntry
    @Environment(\.colorScheme) private var colorScheme
    @Environment(\.widgetRenderingMode) private var renderingMode

    private var isDark: Bool { colorScheme == .dark }
    private var isFullColor: Bool { renderingMode == .fullColor }

    var body: some View {
        if let snap = entry.snapshot {
            HStack(alignment: .top, spacing: 14) {
                // Left Pane (~44%): Account Quota Dashboard + Weekly Pace
                Link(destination: QuotaWidgetConstants.deepLinkURL) {
                    VStack(alignment: .leading, spacing: 10) {
                        HStack(spacing: 6) {
                            Image(systemName: "bolt.fill")
                                .font(.system(size: 13, weight: .bold))
                                .foregroundStyle(isFullColor ? Color.accentColor : .primary)
                                .widgetAccentable()

                            Text(snap.displayAccountTitle)
                                .font(.system(size: 14.5, weight: .bold))
                                .foregroundStyle(.primary)
                                .lineLimit(1)

                            if let tier = snap.tierBadgeLabel {
                                WidgetTierBadge(label: tier)
                            }
                            Spacer(minLength: 4)
                        }

                        if snap.hasBothProviders,
                           let gemini = snap.geminiBundle,
                           let claude = snap.claudeBundle {
                            VStack(spacing: 9) {
                                ExtraLargeProviderCard(provider: gemini, now: entry.date)
                                ExtraLargeProviderCard(provider: claude, now: entry.date)
                            }
                        } else if let resolved = snap.resolvedProvider(for: entry.providerPreference) {
                            ExtraLargeProviderCard(provider: resolved, now: entry.date)
                        }

                        Spacer(minLength: 0)

                        WidgetSyncFooter(syncedAt: snap.syncedAt, isDemo: snap.isDemo, isStale: entry.isStale)
                    }
                }
                .buttonStyle(.plain)
                .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)

                Rectangle()
                    .fill(isDark ? Color.white.opacity(0.10) : Color.black.opacity(0.08))
                    .frame(width: 1)
                    .padding(.vertical, 2)

                // Right Pane (~56%): Active Conversations Queue (up to 5 items)
                VStack(alignment: .leading, spacing: 8) {
                    HStack(alignment: .center, spacing: 6) {
                        Image("AppLogoTransparent")
                            .renderingMode(.original)
                            .resizable()
                            .aspectRatio(contentMode: .fit)
                            .frame(width: 20, height: 13)

                        Text(String(localized: "活跃对话"))
                            .font(.system(size: 14, weight: .bold))
                            .foregroundStyle(.primary)

                        Spacer(minLength: 4)

                        if let convs = entry.conversations {
                            AgentStatusCompactSummaryPills(snapshot: convs, maxPills: 2)
                        }

                        WidgetRefreshIconButton(intent: RefreshQuotaWidgetIntent())
                    }

                    if let convs = entry.conversations, !convs.items.isEmpty {
                        VStack(spacing: 6) {
                            ForEach(Array(convs.items.prefix(5))) { item in
                                WidgetConversationRow(item: item, now: entry.date, showStepCount: true)
                            }
                        }
                    } else {
                        Spacer()
                        HStack {
                            Spacer()
                            VStack(spacing: 6) {
                                Image(systemName: "checkmark.circle")
                                    .font(.system(size: 22, weight: .medium))
                                    .foregroundStyle(.secondary)
                                Text(String(localized: "暂无进行中的任务"))
                                    .font(.system(size: 13, weight: .semibold))
                                    .foregroundStyle(.secondary)
                            }
                            Spacer()
                        }
                        Spacer()
                    }

                    Spacer(minLength: 0)
                }
                .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
                .layoutPriority(1.2)
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
        } else {
            QuotaEmptyCardView(isPaired: entry.isPaired, compact: false)
                .widgetURL(QuotaWidgetConstants.deepLinkURL)
        }
    }
}

// MARK: - Quota Subcomponents

private struct ProviderQuotaColumn: View {
    let provider: ResolvedProviderQuota
    let now: Date
    let compact: Bool
    @Environment(\.colorScheme) private var colorScheme
    @Environment(\.widgetRenderingMode) private var renderingMode

    private var isDark: Bool { colorScheme == .dark }
    private var isFullColor: Bool { renderingMode == .fullColor }

    var body: some View {
        VStack(alignment: .leading, spacing: compact ? 4.5 : 5.5) {
            HStack(alignment: .lastTextBaseline, spacing: 4) {
                Text(provider.providerTitle)
                    .font(.system(size: compact ? 12.5 : 13, weight: .bold))
                    .foregroundStyle(.primary)
                    .lineLimit(1)

                Spacer(minLength: 2)

                if let primary = provider.primaryWindow {
                    Text("\(primary.intPercent)%")
                        .font(.system(size: compact ? 19 : 22, weight: .bold, design: .rounded))
                        .foregroundStyle(isFullColor ? quotaTintColor(percent: primary.remainingPercent) : .primary)
                        .minimumScaleFactor(0.75)
                        .lineLimit(1)
                        .widgetAccentable()

                    Text(provider.primaryWindowTag)
                        .font(.system(size: 9.5, weight: .bold, design: .rounded))
                        .foregroundStyle(.secondary)
                        .padding(.horizontal, 4)
                        .padding(.vertical, 1)
                        .background(
                            Capsule().fill(isDark ? Color.white.opacity(0.10) : Color.black.opacity(0.06))
                        )
                }
            }

            if let primary = provider.primaryWindow {
                HStack(spacing: 3) {
                    Image(systemName: "clock")
                        .font(.system(size: 9, weight: .medium))
                    resetCountdownView(for: primary, now: now, compact: false)
                        .font(.system(size: 10, weight: .medium, design: .rounded))
                        .lineLimit(1)
                }
                .foregroundStyle(.secondary)
            }

            VStack(spacing: compact ? 4 : 5) {
                if let short = provider.shortWindow {
                    QuotaProgressRow(tag: "5h", bucket: short)
                }
                if let weekly = provider.weeklyWindow {
                    QuotaProgressRow(tag: "7d", bucket: weekly)
                }
            }
            .padding(.top, 1)
        }
    }
}

private struct ExtraLargeProviderCard: View {
    let provider: ResolvedProviderQuota
    let now: Date
    @Environment(\.colorScheme) private var colorScheme
    @Environment(\.widgetRenderingMode) private var renderingMode

    private var isDark: Bool { colorScheme == .dark }
    private var isFullColor: Bool { renderingMode == .fullColor }

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack(alignment: .center, spacing: 6) {
                Text(provider.providerTitle)
                    .font(.system(size: 13.5, weight: .bold))
                    .foregroundStyle(.primary)

                if let weekly = provider.weeklyWindow,
                   let pace = weekly.evaluatePace(windowHours: 168.0, now: now) {
                    WeeklyPacePill(pace: pace)
                }

                Spacer(minLength: 4)

                if let primary = provider.primaryWindow {
                    Text("\(primary.intPercent)%")
                        .font(.system(size: 20, weight: .bold, design: .rounded))
                        .foregroundStyle(isFullColor ? quotaTintColor(percent: primary.remainingPercent) : .primary)
                        .widgetAccentable()
                }
            }

            if let short = provider.shortWindow {
                QuotaDetailedBarRow(tag: "5h", bucket: short, now: now)
            }
            if let weekly = provider.weeklyWindow {
                QuotaDetailedBarRow(tag: "7d", bucket: weekly, now: now)
            }
        }
        .padding(.horizontal, 11)
        .padding(.vertical, 9)
        .background(
            RoundedRectangle(cornerRadius: 12, style: .continuous)
                .fill(isDark ? Color.white.opacity(0.055) : Color.black.opacity(0.035))
        )
    }
}

private struct QuotaProgressRow: View {
    let tag: String
    let bucket: QuotaWidgetBucketSnapshot
    @Environment(\.colorScheme) private var colorScheme
    @Environment(\.widgetRenderingMode) private var renderingMode

    private var isDark: Bool { colorScheme == .dark }
    private var isFullColor: Bool { renderingMode == .fullColor }

    var body: some View {
        let tint = isFullColor ? quotaTintColor(percent: bucket.remainingPercent) : Color.primary
        HStack(spacing: 6) {
            Text(tag)
                .font(.system(size: 10.5, weight: .semibold, design: .monospaced))
                .foregroundStyle(.secondary)
                .frame(width: 18, alignment: .leading)

            GeometryReader { geo in
                ZStack(alignment: .leading) {
                    Capsule()
                        .fill(isDark ? Color.white.opacity(0.12) : Color.black.opacity(0.08))
                    Capsule()
                        .fill(tint)
                        .frame(width: max(3, geo.size.width * bucket.clampedFraction))
                        .widgetAccentable()
                }
            }
            .frame(height: 5.5)

            Text("\(bucket.intPercent)%")
                .font(.system(size: 10.5, weight: .bold, design: .monospaced))
                .foregroundStyle(.primary)
                .frame(width: 31, alignment: .trailing)
        }
    }
}

private struct QuotaDetailedBarRow: View {
    let tag: String
    let bucket: QuotaWidgetBucketSnapshot
    let now: Date
    @Environment(\.colorScheme) private var colorScheme
    @Environment(\.widgetRenderingMode) private var renderingMode

    private var isDark: Bool { colorScheme == .dark }
    private var isFullColor: Bool { renderingMode == .fullColor }

    var body: some View {
        let tint = isFullColor ? quotaTintColor(percent: bucket.remainingPercent) : Color.primary
        VStack(alignment: .leading, spacing: 3) {
            HStack(spacing: 6) {
                Text(tag)
                    .font(.system(size: 10.5, weight: .bold, design: .monospaced))
                    .foregroundStyle(.secondary)

                resetCountdownView(for: bucket, now: now, compact: true)
                    .font(.system(size: 10, weight: .medium, design: .rounded))
                    .foregroundStyle(.secondary)
                    .lineLimit(1)

                Spacer()

                Text("\(bucket.intPercent)%")
                    .font(.system(size: 11, weight: .bold, design: .monospaced))
                    .foregroundStyle(.primary)
            }

            GeometryReader { geo in
                ZStack(alignment: .leading) {
                    Capsule()
                        .fill(isDark ? Color.white.opacity(0.12) : Color.black.opacity(0.08))
                    Capsule()
                        .fill(tint)
                        .frame(width: max(3, geo.size.width * bucket.clampedFraction))
                        .widgetAccentable()
                }
            }
            .frame(height: 5.5)
        }
    }
}

private struct WeeklyPacePill: View {
    let pace: QuotaWidgetPaceSummary
    @Environment(\.colorScheme) private var colorScheme
    @Environment(\.widgetRenderingMode) private var renderingMode

    private var isDark: Bool { colorScheme == .dark }
    private var isFullColor: Bool { renderingMode == .fullColor }

    private var tint: Color {
        if !isFullColor { return .primary }
        if pace.isOver { return .orange }
        if pace.isNormal { return .blue }
        return Color(red: 0.20, green: 0.78, blue: 0.42)
    }

    private var labelText: String {
        if pace.isOver {
            return String(localized: "偏快 \(pace.formattedDelta)")
        } else if pace.isNormal {
            return String(localized: "匀速 \(pace.formattedDelta)")
        } else {
            return String(localized: "充裕 \(pace.formattedDelta)")
        }
    }

    var body: some View {
        Text(labelText)
            .font(.system(size: 9.5, weight: .bold, design: .rounded))
            .foregroundStyle(tint)
            .padding(.horizontal, 6)
            .padding(.vertical, 2)
            .background(
                Capsule().fill(tint.opacity(isDark ? 0.20 : 0.13))
            )
            .lineLimit(1)
            .widgetAccentable()
    }
}

private struct QuotaEmptyCardView: View {
    let isPaired: Bool
    let compact: Bool

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack(spacing: 6) {
                Image("AppLogoTransparent")
                    .renderingMode(.original)
                    .resizable()
                    .aspectRatio(contentMode: .fit)
                    .frame(width: 22, height: 15)
                Text("Multigravity")
                    .font(.system(size: 13.5, weight: .bold))
                    .lineLimit(1)
            }

            Spacer()

            Text(isPaired ? String(localized: "等待同步额度") : String(localized: "尚未配对网关"))
                .font(.system(size: compact ? 14 : 16, weight: .semibold))
                .foregroundStyle(.primary)

            Text(isPaired ? String(localized: "点击打开 App 获取最新 5h/7d 额度") : String(localized: "点击打开 App 扫码配对本机网关"))
                .font(.system(size: compact ? 11 : 12.5))
                .foregroundStyle(.secondary)
                .lineLimit(2)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .leading)
    }
}

// MARK: - Shared Visual Components (reused by QuotaStatusWidget & AgentStatusWidget)

struct WidgetCardBackground: View {
    @Environment(\.colorScheme) private var colorScheme

    var body: some View {
        if colorScheme == .dark {
            LinearGradient(
                colors: [
                    Color(red: 0.11, green: 0.12, blue: 0.16),
                    Color(red: 0.07, green: 0.08, blue: 0.11),
                ],
                startPoint: .topLeading,
                endPoint: .bottomTrailing
            )
        } else {
            LinearGradient(
                colors: [
                    Color(red: 0.99, green: 0.99, blue: 1.0),
                    Color(red: 0.95, green: 0.96, blue: 0.98),
                ],
                startPoint: .topLeading,
                endPoint: .bottomTrailing
            )
        }
    }
}

struct WidgetTierBadge: View {
    let label: String
    @Environment(\.colorScheme) private var colorScheme
    @Environment(\.widgetRenderingMode) private var renderingMode

    private var isDark: Bool { colorScheme == .dark }
    private var isFullColor: Bool { renderingMode == .fullColor }

    var body: some View {
        let tint: Color = {
            if !isFullColor { return .secondary }
            if label == "ULTRA" { return .purple }
            if label == "PRO" { return .blue }
            return .secondary
        }()
        Text(label)
            .font(.system(size: 9, weight: .bold, design: .rounded))
            .foregroundStyle(tint)
            .padding(.horizontal, 5)
            .padding(.vertical, 1.5)
            .background(
                Capsule().fill(tint.opacity(isDark ? 0.22 : 0.12))
            )
            .lineLimit(1)
    }
}

struct WidgetRefreshIconButton<IntentType: AppIntent>: View {
    let intent: IntentType
    @Environment(\.colorScheme) private var colorScheme

    private var isDark: Bool { colorScheme == .dark }

    var body: some View {
        Button(intent: intent) {
            Image(systemName: "arrow.clockwise")
                .font(.system(size: 10.5, weight: .semibold))
                .foregroundStyle(.secondary)
                .frame(width: 22, height: 22)
                .background(
                    Circle().fill(isDark ? Color.white.opacity(0.08) : Color.black.opacity(0.05))
                )
        }
        .buttonStyle(.plain)
    }
}

struct WidgetSyncFooter: View {
    let syncedAt: Date
    let isDemo: Bool
    let isStale: Bool

    var body: some View {
        HStack(spacing: 4) {
            if isDemo {
                Text("Demo")
                    .font(.system(size: 9.5, weight: .semibold))
                    .foregroundStyle(.secondary)
            } else {
                if isStale {
                    Image(systemName: "icloud.slash")
                        .font(.system(size: 9))
                        .foregroundStyle(.secondary)
                }
                Text(syncedAt, style: .relative)
                    .font(.system(size: 9.5, weight: .medium))
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
            }
        }
    }
}

struct AgentStatusCompactSummaryPills: View {
    let snapshot: AgentConversationsWidgetSnapshot
    let maxPills: Int
    @Environment(\.colorScheme) private var colorScheme
    @Environment(\.widgetRenderingMode) private var renderingMode

    private var isDark: Bool { colorScheme == .dark }
    private var isFullColor: Bool { renderingMode == .fullColor }

    var body: some View {
        HStack(spacing: 4) {
            if snapshot.pendingActionCount > 0 {
                pill(
                    text: String(localized: "\(snapshot.pendingActionCount) 待确认"),
                    color: .orange
                )
            }
            if snapshot.runningCount > 0 && (snapshot.pendingActionCount == 0 || maxPills >= 2) {
                pill(
                    text: String(localized: "\(snapshot.runningCount) 运行中"),
                    color: Color(red: 0.20, green: 0.56, blue: 1.0)
                )
            }
            if snapshot.pendingActionCount == 0 && snapshot.runningCount == 0 {
                pill(
                    text: String(localized: "已就绪"),
                    color: Color(red: 0.20, green: 0.78, blue: 0.42)
                )
            }
        }
    }

    private func pill(text: String, color: Color) -> some View {
        let fg = isFullColor ? color : Color.primary
        return Text(text)
            .font(.system(size: 10, weight: .bold, design: .rounded))
            .foregroundStyle(fg)
            .padding(.horizontal, 6.5)
            .padding(.vertical, 2)
            .background(
                Capsule().fill(fg.opacity(isDark ? 0.22 : 0.13))
            )
            .lineLimit(1)
            .widgetAccentable()
    }
}

struct WidgetConversationRow: View {
    let item: AgentWidgetConversationItem
    let now: Date
    let showStepCount: Bool
    @Environment(\.colorScheme) private var colorScheme
    @Environment(\.widgetRenderingMode) private var renderingMode

    private var isDark: Bool { colorScheme == .dark }
    private var isFullColor: Bool { renderingMode == .fullColor }

    private var statusColor: Color {
        if !isFullColor { return .primary }
        if item.needsAction { return .orange }
        if item.isWorking { return Color(red: 0.20, green: 0.58, blue: 1.0) }
        if item.status == .error { return .red }
        if item.isUnread { return Color(red: 0.20, green: 0.78, blue: 0.42) }
        return .secondary
    }

    private var statusIconName: String {
        if item.needsAction { return "exclamationmark.circle.fill" }
        if item.isWorking { return "bolt.circle.fill" }
        if item.status == .error { return "xmark.circle.fill" }
        if item.isUnread { return "circle.fill" }
        return "bubble.left.fill"
    }

    private var statusBadgeText: String? {
        if item.needsAction { return String(localized: "待确认") }
        if item.isWorking { return String(localized: "运行中") }
        if item.status == .error { return String(localized: "异常") }
        if item.isUnread { return String(localized: "未读") }
        return nil
    }

    var body: some View {
        Link(destination: QuotaWidgetConstants.cascadeDeepLinkURL(id: item.id)) {
            HStack(alignment: .center, spacing: 8) {
                Image(systemName: statusIconName)
                    .font(.system(size: item.isUnread && !item.isWorking && !item.needsAction ? 7 : 12.5, weight: .semibold))
                    .foregroundStyle(statusColor)
                    .frame(width: 14, alignment: .center)
                    .widgetAccentable()

                VStack(alignment: .leading, spacing: 1.5) {
                    Text(item.title)
                        .font(.system(size: 12, weight: .semibold))
                        .foregroundStyle(.primary)
                        .lineLimit(1)

                    HStack(spacing: 4) {
                        Text(item.displayWorkspace)
                            .font(.system(size: 10, weight: .medium))
                            .foregroundStyle(.secondary)
                            .lineLimit(1)

                        if showStepCount && item.stepCount > 0 {
                            Text("·")
                                .font(.system(size: 10))
                                .foregroundStyle(.secondary)
                            Text(String(localized: "第 \(item.stepCount) 步"))
                                .font(.system(size: 10, weight: .medium, design: .rounded))
                                .foregroundStyle(.secondary)
                                .lineLimit(1)
                        }
                    }
                }

                Spacer(minLength: 6)

                if let badge = statusBadgeText {
                    Text(badge)
                        .font(.system(size: 10, weight: .bold, design: .rounded))
                        .foregroundStyle(statusColor)
                        .padding(.horizontal, 6)
                        .padding(.vertical, 2)
                        .background(
                            Capsule().fill(statusColor.opacity(isDark ? 0.20 : 0.13))
                        )
                        .lineLimit(1)
                        .widgetAccentable()
                } else if let mod = item.lastModified {
                    Text(mod, style: .relative)
                        .font(.system(size: 10, weight: .medium, design: .rounded))
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                }
            }
            .padding(.horizontal, 8.5)
            .padding(.vertical, 6)
            .background(
                RoundedRectangle(cornerRadius: 9, style: .continuous)
                    .fill(isDark ? Color.white.opacity(0.055) : Color.black.opacity(0.035))
            )
        }
        .buttonStyle(.plain)
    }
}

// MARK: - Shared Formatting & Semantic Helpers

func quotaTintColor(percent: Double) -> Color {
    if percent >= 50 {
        return Color(red: 0.20, green: 0.78, blue: 0.42)
    } else if percent >= 20 {
        return .orange
    } else {
        return .red
    }
}

@ViewBuilder
func resetCountdownView(for bucket: QuotaWidgetBucketSnapshot?, now: Date = Date(), compact: Bool) -> some View {
    if let resetAt = bucket?.resetAt, resetAt > now {
        if compact {
            Text(resetAt, style: .relative)
        } else {
            HStack(spacing: 2) {
                Text(resetAt, style: .relative)
                Text(String(localized: "后重置"))
            }
        }
    } else if let friendly = bucket?.resetFriendly, !friendly.isEmpty {
        Text(friendly)
    } else {
        Text(String(localized: "已就绪"))
    }
}
