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
        let config = QuotaWidgetStore.shared.loadConfig()
        if context.isPreview {
            return QuotaWidgetEntry(
                date: now,
                snapshot: QuotaWidgetStore.shared.loadSnapshot()?.applying(config: config)
                    ?? QuotaWidgetSnapshot.demoPlaceholder(now: now),
                conversations: QuotaWidgetStore.shared.loadConversationsSnapshot()
                    ?? AgentConversationsWidgetSnapshot.demoPlaceholder(now: now),
                providerPreference: configuration.provider,
                isPaired: true,
                isStale: false
            )
        }
        let cachedQuota = QuotaWidgetStore.shared.loadSnapshot()?.applying(config: config)
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

        let cachedQuota = QuotaWidgetStore.shared.loadSnapshot()?.applying(config: config)
        let cachedConvs = QuotaWidgetStore.shared.loadConversationsSnapshot()

        // Fast-path: if the main app or a sibling widget/intent already synced fresh data within 90s,
        // emit immediately (<1ms) to prevent WidgetKit placeholder stalls and duplicate network bursts.
        let quotaIsFresh = cachedQuota.map { now.timeIntervalSince($0.syncedAt) < 90 } ?? false
        let convsIsFresh = !needsConversations || (cachedConvs.map { now.timeIntervalSince($0.syncedAt) < 90 } ?? false)

        let freshQuota: QuotaWidgetSnapshot?
        let freshConvs: AgentConversationsWidgetSnapshot?
        if quotaIsFresh && convsIsFresh {
            freshQuota = cachedQuota
            freshConvs = cachedConvs
        } else if needsConversations {
            async let quotaTask = QuotaWidgetStore.shared.fetchLatestQuotaFromGateway()
            async let convsTask = QuotaWidgetStore.shared.fetchLatestConversationsFromGateway()
            let (q, c) = await (quotaTask, convsTask)
            freshQuota = q?.applying(config: config)
            freshConvs = c
        } else {
            freshQuota = await QuotaWidgetStore.shared.fetchLatestQuotaFromGateway()?.applying(config: config)
            freshConvs = cachedConvs
        }

        let resolvedConvs = freshConvs ?? cachedConvs

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
        } else if let cached = cachedQuota {
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
                let shortReset = panelShortCountdownText(for: primary, now: entry.date)
                if !shortReset.isEmpty {
                    Text("\(resolved.providerTitle) \(primary.intPercent)% · \(shortReset)")
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
                    Text(panelShortCountdownText(for: resolved.primaryWindow, now: entry.date))
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
                HStack(alignment: .center, spacing: 4) {
                    Image(systemName: "bolt.fill")
                        .font(.system(size: 11, weight: .bold))
                        .foregroundStyle(isFullColor ? quotaTintColor(percent: primary.remainingPercent) : .primary)
                        .widgetAccentable()

                    Text(resolved.providerTitle)
                        .font(.system(size: 13, weight: .bold))
                        .foregroundStyle(.primary)
                        .lineLimit(1)
                        .minimumScaleFactor(0.85)

                    if let tier = snap.tierBadgeLabel {
                        WidgetTierBadge(label: tier, compact: true)
                    }

                    Spacer(minLength: 2)

                    WidgetRefreshIconButton(intent: RefreshQuotaWidgetIntent(), size: 20)
                }

                Spacer(minLength: 5)

                HStack(alignment: .lastTextBaseline, spacing: 5) {
                    Text(String(format: "%.1f%%", primary.remainingPercent))
                        .font(.system(size: 25, weight: .bold, design: .rounded))
                        .foregroundStyle(isFullColor ? quotaTintColor(percent: primary.remainingPercent) : .primary)
                        .minimumScaleFactor(0.7)
                        .lineLimit(1)
                        .widgetAccentable()

                    Text(resolved.primaryWindowTag)
                        .font(.system(size: 10.5, weight: .bold, design: .rounded))
                        .foregroundStyle(.secondary)
                        .padding(.horizontal, 5)
                        .padding(.vertical, 1.5)
                        .background(
                            Capsule().fill(isDark ? Color.white.opacity(0.10) : Color.black.opacity(0.06))
                        )
                }

                let resetInfo = panelResetDisplayText(for: primary, now: entry.date)
                Text(resetInfo.text)
                    .font(.system(size: 9.5, weight: .medium))
                    .foregroundStyle(.secondary)
                    .opacity(resetInfo.isMuted ? 0.65 : 1.0)
                    .lineLimit(1)
                    .minimumScaleFactor(0.78)
                    .padding(.top, 2)

                Spacer(minLength: 6)

                VStack(spacing: 5) {
                    if let short = resolved.shortWindow {
                        QuotaProgressRow(
                            tag: "5h",
                            bucket: short,
                            windowHours: 5.0,
                            now: entry.date,
                            showPaceCursor: snap.paceCursorEnabled
                        )
                    }
                    if let weekly = resolved.weeklyWindow {
                        QuotaProgressRow(
                            tag: "7d",
                            bucket: weekly,
                            windowHours: 168.0,
                            now: entry.date,
                            showPaceCursor: snap.paceCursorEnabled
                        )
                    }
                }

                Spacer(minLength: 4)

                WidgetSyncFooter(syncedAt: snap.syncedAt, isDemo: snap.isDemo, isStale: entry.isStale)
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .leading)
        } else {
            QuotaEmptyCardView(isPaired: entry.isPaired, compact: true)
        }
    }
}

// MARK: - 5. Home Screen Medium (.systemMedium — Replicates AccountQuotaSheet 2×2 Metric Grid)

private struct QuotaMediumHomeView: View {
    let entry: QuotaWidgetEntry
    @Environment(\.widgetRenderingMode) private var renderingMode

    private var isFullColor: Bool { renderingMode == .fullColor }

    var body: some View {
        if let snap = entry.snapshot {
            VStack(alignment: .leading, spacing: 5.5) {
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

                    WidgetRefreshIconButton(intent: RefreshQuotaWidgetIntent(), size: 20)
                }

                WidgetQuotaPanelGrid(
                    snapshot: snap,
                    providerPreference: entry.providerPreference,
                    now: entry.date,
                    density: .compact
                )

                Spacer(minLength: 0)
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
        } else {
            QuotaEmptyCardView(isPaired: entry.isPaired, compact: false)
        }
    }
}

// MARK: - 6. Home Screen Large (.systemLarge — AccountQuotaSheet 2×2 Grid + Active Conversations)

private struct QuotaLargeHomeView: View {
    let entry: QuotaWidgetEntry
    @Environment(\.colorScheme) private var colorScheme
    @Environment(\.widgetRenderingMode) private var renderingMode

    private var isDark: Bool { colorScheme == .dark }
    private var isFullColor: Bool { renderingMode == .fullColor }

    var body: some View {
        if let snap = entry.snapshot {
            let singleRowQuota = snap.usesSingleRowQuotaGrid(for: entry.providerPreference)
            VStack(alignment: .leading, spacing: 8.5) {
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

                // Top Half: Replicated AccountQuotaSheet 2×2 Metric Grid (tapping opens Quota Sheet)
                Link(destination: QuotaWidgetConstants.deepLinkURL) {
                    WidgetQuotaPanelGrid(
                        snapshot: snap,
                        providerPreference: entry.providerPreference,
                        now: entry.date,
                        density: .regular
                    )
                }
                .buttonStyle(.plain)

                // Bottom Half: Active Conversations (3 rows for 2×2 quota grid, 4 rows for single-row Free/filtered grid)
                VStack(alignment: .leading, spacing: 6) {
                    HStack(alignment: .center) {
                        Text(String(localized: "活跃对话"))
                            .font(.system(size: 11.5, weight: .bold))
                            .foregroundStyle(.secondary)

                        Spacer()

                        WidgetSyncFooter(syncedAt: snap.syncedAt, isDemo: snap.isDemo, isStale: entry.isStale)
                    }

                    let maxRows = singleRowQuota ? 4 : 3
                    if let convs = entry.conversations, !convs.items.isEmpty {
                        VStack(spacing: 5.5) {
                            ForEach(Array(convs.items.prefix(maxRows))) { item in
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
                                    .fill(widgetSubCardFill(isDark: isDark))
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

// MARK: - 7. iPad Extra Large (.systemExtraLarge — Left Panel Quota Stack, Right 5 Active Conversations)

private struct QuotaExtraLargeHomeView: View {
    let entry: QuotaWidgetEntry
    @Environment(\.colorScheme) private var colorScheme
    @Environment(\.widgetRenderingMode) private var renderingMode

    private var isDark: Bool { colorScheme == .dark }
    private var isFullColor: Bool { renderingMode == .fullColor }

    var body: some View {
        if let snap = entry.snapshot {
            HStack(alignment: .top, spacing: 14) {
                // Left Pane (~45%): Account Quota Panel Cards
                Link(destination: QuotaWidgetConstants.deepLinkURL) {
                    VStack(alignment: .leading, spacing: 8.5) {
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

                        WidgetQuotaPanelStack(
                            snapshot: snap,
                            providerPreference: entry.providerPreference,
                            now: entry.date
                        )

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

                // Right Pane (~55%): Active Conversations Queue (up to 5 items)
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

// MARK: - Replicated AccountQuotaSheet Panel Grid & Metric Cells

private enum WidgetQuotaCellDensity {
    case compact   // .systemMedium
    case regular   // .systemLarge
    case spacious  // .systemExtraLarge

    var titleFontSize: CGFloat {
        switch self {
        case .compact: return 10.0
        case .regular: return 10.5
        case .spacious: return 11.5
        }
    }

    var percentFontSize: CGFloat {
        switch self {
        case .compact: return 11.0
        case .regular: return 11.5
        case .spacious: return 12.5
        }
    }

    var resetFontSize: CGFloat {
        switch self {
        case .compact: return 9.0
        case .regular: return 9.5
        case .spacious: return 10.0
        }
    }

    var barHeight: CGFloat {
        switch self {
        case .compact: return 4.5
        case .regular: return 5.0
        case .spacious: return 5.5
        }
    }

    var innerSpacing: CGFloat {
        switch self {
        case .compact: return 3.0
        case .regular: return 4.2
        case .spacious: return 4.5
        }
    }

    var horizontalPadding: CGFloat {
        switch self {
        case .compact: return 8.0
        case .regular: return 8.5
        case .spacious: return 10.0
        }
    }

    var verticalPadding: CGFloat {
        switch self {
        case .compact: return 5.5
        case .regular: return 8.2
        case .spacious: return 8.5
        }
    }

    var gridSpacing: CGFloat {
        switch self {
        case .compact: return 5.5
        case .regular: return 7.0
        case .spacious: return 7.0
        }
    }
}

private struct WidgetQuotaCellSpec: Identifiable {
    let id: String
    let title: String
    let bucket: QuotaWidgetBucketSnapshot?
    let windowHours: Double
}

private extension QuotaWidgetSnapshot {
    func panelCellSpecs(for preference: QuotaWidgetProviderPreference) -> [WidgetQuotaCellSpec] {
        switch preference {
        case .followApp:
            if hasBothProviders {
                if isFreeTier {
                    return [
                        WidgetQuotaCellSpec(id: "gemini_weekly", title: "Gemini · Weekly", bucket: geminiWeekly, windowHours: 168.0),
                        WidgetQuotaCellSpec(id: "claude_weekly", title: "Claude · Weekly", bucket: claudeWeekly, windowHours: 168.0),
                    ]
                }
                return [
                    WidgetQuotaCellSpec(id: "gemini_5h", title: "Gemini · 5h", bucket: geminiShort, windowHours: 5.0),
                    WidgetQuotaCellSpec(id: "gemini_weekly", title: "Gemini · Weekly", bucket: geminiWeekly, windowHours: 168.0),
                    WidgetQuotaCellSpec(id: "claude_5h", title: "Claude · 5h", bucket: claudeShort, windowHours: 5.0),
                    WidgetQuotaCellSpec(id: "claude_weekly", title: "Claude · Weekly", bucket: claudeWeekly, windowHours: 168.0),
                ]
            }
            if let single = resolvedProvider(for: .followApp) {
                return singleProviderCellSpecs(for: single)
            }
            return []
        case .gemini, .claude:
            guard let single = resolvedProvider(for: preference) else { return [] }
            return singleProviderCellSpecs(for: single)
        }
    }

    func usesSingleRowQuotaGrid(for preference: QuotaWidgetProviderPreference) -> Bool {
        panelCellSpecs(for: preference).count <= 2
    }

    private func singleProviderCellSpecs(for provider: ResolvedProviderQuota) -> [WidgetQuotaCellSpec] {
        let prefix = provider.isClaude ? "claude" : "gemini"
        var specs: [WidgetQuotaCellSpec] = []
        if !isFreeTier, let short = provider.shortWindow {
            specs.append(WidgetQuotaCellSpec(
                id: "\(prefix)_5h",
                title: "\(provider.providerTitle) · 5h",
                bucket: short,
                windowHours: 5.0
            ))
        }
        if let weekly = provider.weeklyWindow {
            specs.append(WidgetQuotaCellSpec(
                id: "\(prefix)_weekly",
                title: "\(provider.providerTitle) · Weekly",
                bucket: weekly,
                windowHours: 168.0
            ))
        }
        return specs
    }
}

private struct WidgetQuotaPanelGrid: View {
    let snapshot: QuotaWidgetSnapshot
    let providerPreference: QuotaWidgetProviderPreference
    let now: Date
    let density: WidgetQuotaCellDensity

    var body: some View {
        let specs = snapshot.panelCellSpecs(for: providerPreference)
        let selectedKey = snapshot.resolvedSelectedBucketRaw

        if specs.count == 4 {
            VStack(spacing: density.gridSpacing) {
                HStack(spacing: density.gridSpacing) {
                    cellView(specs[0], selectedKey: selectedKey)
                    cellView(specs[1], selectedKey: selectedKey)
                }
                HStack(spacing: density.gridSpacing) {
                    cellView(specs[2], selectedKey: selectedKey)
                    cellView(specs[3], selectedKey: selectedKey)
                }
            }
        } else if specs.count == 2 {
            HStack(spacing: density.gridSpacing) {
                cellView(specs[0], selectedKey: selectedKey)
                cellView(specs[1], selectedKey: selectedKey)
            }
        } else if let first = specs.first {
            cellView(first, selectedKey: selectedKey)
        }
    }

    private func cellView(_ spec: WidgetQuotaCellSpec, selectedKey: String?) -> some View {
        WidgetQuotaMetricCell(
            title: spec.title,
            bucket: spec.bucket,
            windowHours: spec.windowHours,
            now: now,
            showPaceCursor: snapshot.paceCursorEnabled,
            isSelected: selectedKey == spec.id,
            density: density
        )
    }
}

private struct WidgetQuotaPanelStack: View {
    let snapshot: QuotaWidgetSnapshot
    let providerPreference: QuotaWidgetProviderPreference
    let now: Date

    var body: some View {
        let specs = snapshot.panelCellSpecs(for: providerPreference)
        let selectedKey = snapshot.resolvedSelectedBucketRaw

        VStack(spacing: WidgetQuotaCellDensity.spacious.gridSpacing) {
            ForEach(specs) { spec in
                WidgetQuotaMetricCell(
                    title: spec.title,
                    bucket: spec.bucket,
                    windowHours: spec.windowHours,
                    now: now,
                    showPaceCursor: snapshot.paceCursorEnabled,
                    isSelected: selectedKey == spec.id,
                    density: .spacious,
                    showPaceTag: spec.windowHours >= 168.0
                )
            }
        }
    }
}

private struct WidgetQuotaMetricCell: View {
    let title: String
    let bucket: QuotaWidgetBucketSnapshot?
    let windowHours: Double
    let now: Date
    let showPaceCursor: Bool
    let isSelected: Bool
    let density: WidgetQuotaCellDensity
    var showPaceTag: Bool = false

    @Environment(\.colorScheme) private var colorScheme
    @Environment(\.widgetRenderingMode) private var renderingMode

    private var isDark: Bool { colorScheme == .dark }
    private var isFullColor: Bool { renderingMode == .fullColor }

    var body: some View {
        let percent = bucket?.remainingPercent ?? 0.0
        let tint = isFullColor ? quotaTintColor(percent: percent) : Color.primary
        let pace = bucket?.evaluatePace(windowHours: windowHours, now: now)

        VStack(alignment: .leading, spacing: density.innerSpacing) {
            HStack(spacing: 3) {
                Text(title)
                    .font(.system(size: density.titleFontSize, weight: .medium))
                    .tracking(-0.15)
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
                    .minimumScaleFactor(0.85)

                Spacer(minLength: 3)

                Text(String(format: "%.1f%%", percent))
                    .font(.system(size: density.percentFontSize, weight: .bold, design: .monospaced))
                    .foregroundStyle(tint)
                    .lineLimit(1)
                    .fixedSize(horizontal: true, vertical: false)
                    .layoutPriority(1)
                    .widgetAccentable()
            }

            GeometryReader { geo in
                let fillWidth = max(0, min(geo.size.width, geo.size.width * CGFloat(percent / 100.0)))
                ZStack(alignment: .leading) {
                    Capsule()
                        .fill(isDark ? Color.white.opacity(0.10) : Color.black.opacity(0.08))

                    if fillWidth > 0 {
                        Capsule()
                            .fill(tint)
                            .frame(width: fillWidth)
                            .widgetAccentable()
                    }

                    if showPaceCursor, let pace {
                        let expectedFrac = CGFloat(pace.expectedRemainingPercent / 100.0)
                        let needleOffset = max(0, min(geo.size.width - 1.5, geo.size.width * expectedFrac - 0.75))
                        let needleColor: Color = isFullColor
                            ? (pace.isOver
                                ? Color(red: 1.0, green: 159.0 / 255.0, blue: 10.0 / 255.0)
                                : Color.white.opacity(0.94))
                            : Color.primary

                        RoundedRectangle(cornerRadius: 0.75, style: .continuous)
                            .fill(needleColor)
                            .frame(width: 1.5, height: density.barHeight + 2.5)
                            .shadow(color: Color.black.opacity(0.35), radius: 1, x: 0, y: 0.5)
                            .offset(x: needleOffset, y: -1.25)
                    }
                }
            }
            .frame(height: density.barHeight)

            let resetInfo = panelResetDisplayText(for: bucket, now: now)
            HStack(spacing: 4) {
                Text(resetInfo.text)
                    .font(.system(size: density.resetFontSize))
                    .foregroundStyle(.secondary)
                    .opacity(resetInfo.isMuted ? 0.6 : 1.0)
                    .lineLimit(1)
                    .minimumScaleFactor(0.75)

                if showPaceTag, let pace {
                    Spacer(minLength: 4)
                    WeeklyPacePill(pace: pace)
                }
            }
        }
        .padding(.horizontal, density.horizontalPadding)
        .padding(.vertical, density.verticalPadding)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(
            RoundedRectangle(cornerRadius: 8.5, style: .continuous)
                .fill(widgetSubCardFill(isDark: isDark))
        )
        .overlay(
            Group {
                if isSelected {
                    RoundedRectangle(cornerRadius: 8.5, style: .continuous)
                        .stroke(
                            isFullColor
                                ? Color.blue.opacity(isDark ? 0.85 : 0.75)
                                : Color.primary.opacity(0.45),
                            lineWidth: 1.5
                        )
                }
            }
        )
    }
}

private struct QuotaProgressRow: View {
    let tag: String
    let bucket: QuotaWidgetBucketSnapshot
    let windowHours: Double
    let now: Date
    let showPaceCursor: Bool
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
                let fillW = max(0, min(geo.size.width, geo.size.width * bucket.clampedFraction))
                ZStack(alignment: .leading) {
                    Capsule()
                        .fill(isDark ? Color.white.opacity(0.12) : Color.black.opacity(0.08))
                    if fillW > 0 {
                        Capsule()
                            .fill(tint)
                            .frame(width: fillW)
                            .widgetAccentable()
                    }
                    if showPaceCursor, let pace = bucket.evaluatePace(windowHours: windowHours, now: now) {
                        let expectedFrac = CGFloat(pace.expectedRemainingPercent / 100.0)
                        let needleOffset = max(0, min(geo.size.width - 1.5, geo.size.width * expectedFrac - 0.75))
                        let needleColor: Color = isFullColor
                            ? (pace.isOver
                                ? Color(red: 1.0, green: 159.0 / 255.0, blue: 10.0 / 255.0)
                                : Color.white.opacity(0.94))
                            : Color.primary
                        RoundedRectangle(cornerRadius: 0.75, style: .continuous)
                            .fill(needleColor)
                            .frame(width: 1.5, height: 7.5)
                            .shadow(color: Color.black.opacity(0.35), radius: 1, x: 0, y: 0.5)
                            .offset(x: needleOffset, y: -1.0)
                    }
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
            .font(.system(size: 9, weight: .bold, design: .rounded))
            .foregroundStyle(tint)
            .padding(.horizontal, 5.5)
            .padding(.vertical, 1.5)
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

func widgetSubCardFill(isDark: Bool) -> Color {
    isDark
        ? Color.white.opacity(0.065)
        : Color(red: 242.0 / 255.0, green: 242.0 / 255.0, blue: 247.0 / 255.0)
}

struct WidgetCardBackground: View {
    @Environment(\.colorScheme) private var colorScheme

    var body: some View {
        if colorScheme == .dark {
            LinearGradient(
                colors: [
                    Color(red: 0.11, green: 0.12, blue: 0.16),
                    Color(red: 0.08, green: 0.09, blue: 0.12),
                ],
                startPoint: .topLeading,
                endPoint: .bottomTrailing
            )
        } else {
            LinearGradient(
                colors: [
                    Color.white,
                    Color(red: 0.99, green: 0.992, blue: 0.998),
                ],
                startPoint: .topLeading,
                endPoint: .bottomTrailing
            )
        }
    }
}

struct WidgetTierBadge: View {
    let label: String
    var compact: Bool = false
    @Environment(\.widgetRenderingMode) private var renderingMode

    private var isFullColor: Bool { renderingMode == .fullColor }

    var body: some View {
        let normalized = label.trimmingCharacters(in: .whitespacesAndNewlines).uppercased()
        let fontSize: CGFloat = compact ? 8.5 : 9.5
        let hPad: CGFloat = compact ? 4.5 : 6.0
        let vPad: CGFloat = compact ? 1.8 : 2.5

        if isFullColor && normalized == "ULTRA" {
            Text("ULTRA")
                .font(.system(size: fontSize, weight: .heavy))
                .tracking(0.5)
                .foregroundStyle(.white)
                .shadow(
                    color: Color(red: 124.0 / 255.0, green: 45.0 / 255.0, blue: 18.0 / 255.0).opacity(0.35),
                    radius: 1,
                    x: 0,
                    y: 0.8
                )
                .padding(.horizontal, hPad)
                .padding(.vertical, vPad)
                .background(
                    LinearGradient(
                        colors: [
                            Color(red: 217.0 / 255.0, green: 119.0 / 255.0, blue: 6.0 / 255.0),
                            Color(red: 245.0 / 255.0, green: 158.0 / 255.0, blue: 11.0 / 255.0),
                            Color(red: 234.0 / 255.0, green: 88.0 / 255.0, blue: 12.0 / 255.0),
                        ],
                        startPoint: .topLeading,
                        endPoint: .bottomTrailing
                    )
                )
                .clipShape(RoundedRectangle(cornerRadius: 5, style: .continuous))
                .lineLimit(1)
        } else if isFullColor && normalized == "PRO" {
            Text("PRO")
                .font(.system(size: fontSize, weight: .bold))
                .tracking(0.4)
                .foregroundStyle(.white)
                .padding(.horizontal, hPad)
                .padding(.vertical, vPad)
                .background(
                    LinearGradient(
                        colors: [
                            Color(red: 29.0 / 255.0, green: 78.0 / 255.0, blue: 216.0 / 255.0),
                            Color(red: 14.0 / 255.0, green: 165.0 / 255.0, blue: 165.0 / 255.0),
                        ],
                        startPoint: .topLeading,
                        endPoint: .bottomTrailing
                    )
                )
                .clipShape(RoundedRectangle(cornerRadius: 5, style: .continuous))
                .lineLimit(1)
        } else {
            Text(normalized)
                .font(.system(size: fontSize, weight: .bold))
                .tracking(0.4)
                .foregroundStyle(.secondary)
                .padding(.horizontal, hPad)
                .padding(.vertical, vPad)
                .background(Color.secondary.opacity(0.16))
                .overlay(
                    RoundedRectangle(cornerRadius: 5, style: .continuous)
                        .stroke(Color.secondary.opacity(0.32), lineWidth: 0.5)
                )
                .clipShape(RoundedRectangle(cornerRadius: 5, style: .continuous))
                .lineLimit(1)
        }
    }
}

struct WidgetRefreshIconButton<IntentType: AppIntent>: View {
    let intent: IntentType
    var size: CGFloat = 22
    @Environment(\.colorScheme) private var colorScheme

    private var isDark: Bool { colorScheme == .dark }

    var body: some View {
        Button(intent: intent) {
            Image(systemName: "arrow.clockwise")
                .font(.system(size: size * 0.48, weight: .semibold))
                .foregroundStyle(.secondary)
                .frame(width: size, height: size)
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
            .padding(.vertical, 6.5)
            .background(
                RoundedRectangle(cornerRadius: 9, style: .continuous)
                    .fill(widgetSubCardFill(isDark: isDark))
            )
        }
        .buttonStyle(.plain)
    }
}

// MARK: - Shared Formatting & Semantic Helpers

func quotaTintColor(percent: Double) -> Color {
    if percent >= 50 {
        return Color(red: 0.20, green: 0.78, blue: 0.35)
    } else if percent >= 20 {
        return .orange
    } else {
        return .red
    }
}

/// Formats compact remaining duration (`6d 15h 23m`, `2h 49m`, `34m`, `<1m`) matching `AccountQuotaSheet`.
func panelShortCountdownText(for bucket: QuotaWidgetBucketSnapshot?, now: Date = Date()) -> String {
    if let resetAt = bucket?.resetAt {
        let diff = resetAt.timeIntervalSince(now)
        if diff <= 0 {
            return String(localized: "已就绪")
        }
        let totalMinutes = Int(diff / 60.0)
        let days = totalMinutes / (24 * 60)
        let hours = (totalMinutes % (24 * 60)) / 60
        let minutes = totalMinutes % 60
        var parts: [String] = []
        if days > 0 { parts.append("\(days)d") }
        if hours > 0 { parts.append("\(hours)h") }
        if minutes > 0 { parts.append("\(minutes)m") }
        return parts.isEmpty ? "<1m" : parts.joined(separator: " ")
    }
    if let friendly = bucket?.resetFriendly?.trimmingCharacters(in: .whitespacesAndNewlines), !friendly.isEmpty {
        if friendly == "已就绪" || friendly == "就绪" {
            return String(localized: "已就绪")
        }
        if friendly == "未知" {
            return String(localized: "未知")
        }
        if friendly == "配额充足" {
            return String(localized: "配额充足")
        }
        return friendly
    }
    return String(localized: "配额充足")
}

/// Formats full reset display (`2h 49m (11/22 11:11)`, `未知`, `配额充足`) matching `AccountQuotaSheet.resetCountdownDisplay`.
func panelResetDisplayText(for bucket: QuotaWidgetBucketSnapshot?, now: Date = Date()) -> (text: String, isMuted: Bool) {
    if let resetAt = bucket?.resetAt {
        let diff = resetAt.timeIntervalSince(now)
        if diff <= 0 {
            return (String(localized: "已就绪"), false)
        }
        let short = panelShortCountdownText(for: bucket, now: now)
        let df = DateFormatter()
        df.locale = Locale(identifier: "en_US_POSIX")
        df.timeZone = TimeZone.current
        df.dateFormat = "MM/dd HH:mm"
        return ("\(short) (\(df.string(from: resetAt)))", false)
    }
    if let friendly = bucket?.resetFriendly?.trimmingCharacters(in: .whitespacesAndNewlines), !friendly.isEmpty {
        if friendly == "已就绪" || friendly == "就绪" {
            return (String(localized: "已就绪"), false)
        }
        if friendly == "未知" {
            return (String(localized: "未知"), false)
        }
        if friendly == "配额充足" {
            return (String(localized: "配额充足"), true)
        }
        return (friendly, false)
    }
    return (String(localized: "配额充足"), true)
}
