import AppIntents
import Foundation
import SwiftUI
import WidgetKit

// MARK: - Timeline Entry & Provider

nonisolated struct AgentStatusWidgetEntry: TimelineEntry {
    let date: Date
    let snapshot: AgentConversationsWidgetSnapshot?
    let isPaired: Bool
    let isStale: Bool
}

nonisolated struct AgentStatusTimelineProvider: TimelineProvider {
    func placeholder(in context: Context) -> AgentStatusWidgetEntry {
        let now = Date()
        return AgentStatusWidgetEntry(
            date: now,
            snapshot: AgentConversationsWidgetSnapshot.demoPlaceholder(now: now),
            isPaired: true,
            isStale: false
        )
    }

    func getSnapshot(in context: Context, completion: @escaping (AgentStatusWidgetEntry) -> Void) {
        let now = Date()
        if context.isPreview {
            completion(AgentStatusWidgetEntry(
                date: now,
                snapshot: QuotaWidgetStore.shared.loadConversationsSnapshot()
                    ?? AgentConversationsWidgetSnapshot.demoPlaceholder(now: now),
                isPaired: true,
                isStale: false
            ))
            return
        }
        let config = QuotaWidgetStore.shared.loadConfig()
        let cached = QuotaWidgetStore.shared.loadConversationsSnapshot()
        let paired = (config?.isPaired ?? false) || (cached != nil)
        completion(AgentStatusWidgetEntry(
            date: now,
            snapshot: cached ?? (paired ? AgentConversationsWidgetSnapshot.demoPlaceholder(now: now) : nil),
            isPaired: paired,
            isStale: false
        ))
    }

    func getTimeline(in context: Context, completion: @escaping (Timeline<AgentStatusWidgetEntry>) -> Void) {
        Task {
            let now = Date()
            let config = QuotaWidgetStore.shared.loadConfig()
            let fresh = await QuotaWidgetStore.shared.fetchLatestConversationsFromGateway()

            let entry: AgentStatusWidgetEntry
            if let fresh {
                entry = AgentStatusWidgetEntry(date: now, snapshot: fresh, isPaired: true, isStale: false)
            } else if let cached = QuotaWidgetStore.shared.loadConversationsSnapshot() {
                let stale = now.timeIntervalSince(cached.syncedAt) > 25 * 60
                entry = AgentStatusWidgetEntry(date: now, snapshot: cached, isPaired: true, isStale: stale)
            } else {
                let paired = config?.isPaired ?? false
                entry = AgentStatusWidgetEntry(date: now, snapshot: nil, isPaired: paired, isStale: false)
            }

            let nextRefresh = Calendar.current.date(byAdding: .minute, value: 15, to: now)
                ?? now.addingTimeInterval(15 * 60)
            completion(Timeline(entries: [entry], policy: .after(nextRefresh)))
        }
    }
}

// MARK: - Widget Definition (Small / Medium / Large; Small automatically eligible for CarPlay & StandBy)

public struct AgentStatusWidget: Widget {
    public init() {}

    public var body: some WidgetConfiguration {
        StaticConfiguration(kind: QuotaWidgetConstants.agentWidgetKind, provider: AgentStatusTimelineProvider()) { entry in
            AgentStatusWidgetEntryView(entry: entry)
        }
        .configurationDisplayName(String(localized: "活跃对话"))
        .description(String(localized: "在主屏幕与 CarPlay 随时查看待确认、运行中的 Agent 任务，轻点直达对话。"))
        .supportedFamilies([.systemSmall, .systemMedium, .systemLarge])
    }
}

// MARK: - Root Family Switcher

private struct AgentStatusWidgetEntryView: View {
    let entry: AgentStatusWidgetEntry
    @Environment(\.widgetFamily) private var family

    var body: some View {
        Group {
            switch family {
            case .systemSmall:
                AgentStatusSmallView(entry: entry)
                    .widgetURL(smallDeepLinkURL)
            case .systemMedium:
                AgentStatusMediumView(entry: entry)
            case .systemLarge:
                AgentStatusLargeView(entry: entry)
            default:
                AgentStatusSmallView(entry: entry)
                    .widgetURL(smallDeepLinkURL)
            }
        }
        .containerBackground(for: .widget) {
            WidgetCardBackground()
        }
    }

    private var smallDeepLinkURL: URL {
        if let top = entry.snapshot?.topItem {
            return QuotaWidgetConstants.cascadeDeepLinkURL(id: top.id)
        }
        return QuotaWidgetConstants.conversationsDeepLinkURL
    }
}

// MARK: - 1. Small (.systemSmall — CarPlay / StandBy / Home Glance Card)

private struct AgentStatusSmallView: View {
    let entry: AgentStatusWidgetEntry
    @Environment(\.colorScheme) private var colorScheme
    @Environment(\.widgetRenderingMode) private var renderingMode

    private var isDark: Bool { colorScheme == .dark }
    private var isFullColor: Bool { renderingMode == .fullColor }

    var body: some View {
        if let snap = entry.snapshot {
            VStack(alignment: .leading, spacing: 0) {
                // Header: Brand icon + Primary Status Pill + Refresh
                HStack(alignment: .center, spacing: 5) {
                    Image("AppLogoTransparent")
                        .renderingMode(.original)
                        .resizable()
                        .aspectRatio(contentMode: .fit)
                        .frame(width: 20, height: 13)

                    AgentStatusCompactSummaryPills(snapshot: snap, maxPills: 1)

                    Spacer(minLength: 2)

                    WidgetRefreshIconButton(intent: RefreshAgentStatusWidgetIntent())
                }

                Spacer(minLength: 8)

                if let top = snap.topItem {
                    VStack(alignment: .leading, spacing: 5) {
                        Text(top.title)
                            .font(.system(size: 14, weight: .bold))
                            .foregroundStyle(.primary)
                            .lineLimit(2)
                            .lineSpacing(1.5)

                        HStack(spacing: 4) {
                            Image(systemName: top.displayWorkspace == "Chat" ? "bubble.left" : "folder")
                                .font(.system(size: 9.5, weight: .semibold))
                            Text(top.displayWorkspace)
                                .font(.system(size: 10.5, weight: .semibold))
                                .lineLimit(1)
                        }
                        .foregroundStyle(.secondary)
                        .padding(.horizontal, 6)
                        .padding(.vertical, 2)
                        .background(
                            Capsule().fill(isDark ? Color.white.opacity(0.08) : Color.black.opacity(0.05))
                        )
                    }

                    Spacer(minLength: 8)

                    HStack(spacing: 4) {
                        if top.stepCount > 0 {
                            Text(String(localized: "第 \(top.stepCount) 步"))
                                .font(.system(size: 10, weight: .semibold, design: .rounded))
                                .foregroundStyle(accentColor(for: top))
                                .widgetAccentable()
                            Text("·")
                                .font(.system(size: 10))
                                .foregroundStyle(.secondary)
                        }
                        if let mod = top.lastModified {
                            Text(mod, style: .relative)
                                .font(.system(size: 10, weight: .medium))
                                .foregroundStyle(.secondary)
                                .lineLimit(1)
                        } else {
                            WidgetSyncFooter(syncedAt: snap.syncedAt, isDemo: snap.isDemo, isStale: entry.isStale)
                        }
                        Spacer(minLength: 0)
                    }
                } else {
                    VStack(alignment: .leading, spacing: 4) {
                        Text(String(localized: "暂无进行中的任务"))
                            .font(.system(size: 13.5, weight: .semibold))
                            .foregroundStyle(.primary)
                        Text(String(localized: "轻点打开 Multigravity 开始新对话"))
                            .font(.system(size: 11))
                            .foregroundStyle(.secondary)
                            .lineLimit(2)
                    }

                    Spacer(minLength: 6)

                    WidgetSyncFooter(syncedAt: snap.syncedAt, isDemo: snap.isDemo, isStale: entry.isStale)
                }
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .leading)
        } else {
            AgentStatusEmptyCardView(isPaired: entry.isPaired, compact: true)
        }
    }

    private func accentColor(for item: AgentWidgetConversationItem) -> Color {
        if !isFullColor { return .primary }
        if item.needsAction { return .orange }
        if item.isWorking { return Color(red: 0.20, green: 0.58, blue: 1.0) }
        if item.status == .error { return .red }
        return .secondary
    }
}

// MARK: - 2. Medium (.systemMedium — 3 Compact Direct-Link Conversations)

private struct AgentStatusMediumView: View {
    let entry: AgentStatusWidgetEntry
    @Environment(\.colorScheme) private var colorScheme

    private var isDark: Bool { colorScheme == .dark }

    var body: some View {
        if let snap = entry.snapshot {
            VStack(alignment: .leading, spacing: 7) {
                HStack(alignment: .center, spacing: 6) {
                    Image("AppLogoTransparent")
                        .renderingMode(.original)
                        .resizable()
                        .aspectRatio(contentMode: .fit)
                        .frame(width: 20, height: 13)

                    Text(String(localized: "活跃对话"))
                        .font(.system(size: 13.5, weight: .bold))
                        .foregroundStyle(.primary)
                        .lineLimit(1)

                    AgentStatusCompactSummaryPills(snapshot: snap, maxPills: 2)

                    Spacer(minLength: 4)

                    WidgetRefreshIconButton(intent: RefreshAgentStatusWidgetIntent())
                }

                if !snap.items.isEmpty {
                    VStack(spacing: 5) {
                        ForEach(Array(snap.items.prefix(3))) { item in
                            CompactMediumConversationRow(item: item)
                        }
                    }
                    Spacer(minLength: 0)
                } else {
                    Spacer()
                    Link(destination: QuotaWidgetConstants.conversationsDeepLinkURL) {
                        HStack(spacing: 8) {
                            Image(systemName: "checkmark.circle")
                                .font(.system(size: 16, weight: .semibold))
                                .foregroundStyle(.secondary)
                            VStack(alignment: .leading, spacing: 2) {
                                Text(String(localized: "暂无进行中的任务"))
                                    .font(.system(size: 13, weight: .semibold))
                                    .foregroundStyle(.primary)
                                Text(String(localized: "轻点打开 Multigravity 查看历史或新建对话"))
                                    .font(.system(size: 11))
                                    .foregroundStyle(.secondary)
                                    .lineLimit(1)
                            }
                            Spacer()
                        }
                        .padding(.horizontal, 10)
                        .padding(.vertical, 10)
                        .background(
                            RoundedRectangle(cornerRadius: 10, style: .continuous)
                                .fill(isDark ? Color.white.opacity(0.05) : Color.black.opacity(0.035))
                        )
                    }
                    .buttonStyle(.plain)
                    Spacer()
                }
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
        } else {
            AgentStatusEmptyCardView(isPaired: entry.isPaired, compact: false)
                .widgetURL(QuotaWidgetConstants.conversationsDeepLinkURL)
        }
    }
}

private struct CompactMediumConversationRow: View {
    let item: AgentWidgetConversationItem
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
            HStack(spacing: 7) {
                Image(systemName: statusIconName)
                    .font(.system(size: item.isUnread && !item.isWorking && !item.needsAction ? 6.5 : 11.5, weight: .semibold))
                    .foregroundStyle(statusColor)
                    .frame(width: 13, alignment: .center)
                    .widgetAccentable()

                Text(item.title)
                    .font(.system(size: 12, weight: .semibold))
                    .foregroundStyle(.primary)
                    .lineLimit(1)

                Text(item.displayWorkspace)
                    .font(.system(size: 10, weight: .medium))
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
                    .layoutPriority(-1)

                Spacer(minLength: 4)

                if let badge = statusBadgeText {
                    Text(badge)
                        .font(.system(size: 9.5, weight: .bold, design: .rounded))
                        .foregroundStyle(statusColor)
                        .padding(.horizontal, 5.5)
                        .padding(.vertical, 1.5)
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
            .padding(.horizontal, 8)
            .padding(.vertical, 6)
            .background(
                RoundedRectangle(cornerRadius: 8.5, style: .continuous)
                    .fill(isDark ? Color.white.opacity(0.055) : Color.black.opacity(0.035))
            )
        }
        .buttonStyle(.plain)
    }
}

// MARK: - 3. Large (.systemLarge — 5 Detailed Direct-Link Conversations + Summary)

private struct AgentStatusLargeView: View {
    let entry: AgentStatusWidgetEntry
    @Environment(\.colorScheme) private var colorScheme

    private var isDark: Bool { colorScheme == .dark }

    var body: some View {
        if let snap = entry.snapshot {
            VStack(alignment: .leading, spacing: 8) {
                HStack(alignment: .center, spacing: 6) {
                    Image("AppLogoTransparent")
                        .renderingMode(.original)
                        .resizable()
                        .aspectRatio(contentMode: .fit)
                        .frame(width: 22, height: 14)

                    Text(String(localized: "活跃对话"))
                        .font(.system(size: 14.5, weight: .bold))
                        .foregroundStyle(.primary)
                        .lineLimit(1)

                    Spacer(minLength: 4)

                    AgentStatusCompactSummaryPills(snapshot: snap, maxPills: 2)

                    WidgetRefreshIconButton(intent: RefreshAgentStatusWidgetIntent())
                }

                if !snap.items.isEmpty {
                    VStack(spacing: 6) {
                        ForEach(Array(snap.items.prefix(5))) { item in
                            WidgetConversationRow(item: item, now: entry.date, showStepCount: true)
                        }
                    }
                } else {
                    Spacer()
                    Link(destination: QuotaWidgetConstants.conversationsDeepLinkURL) {
                        HStack {
                            Spacer()
                            VStack(spacing: 8) {
                                Image(systemName: "checkmark.circle")
                                    .font(.system(size: 26, weight: .medium))
                                    .foregroundStyle(.secondary)
                                Text(String(localized: "暂无进行中的任务"))
                                    .font(.system(size: 14, weight: .semibold))
                                    .foregroundStyle(.primary)
                                Text(String(localized: "轻点打开 Multigravity 开始新对话"))
                                    .font(.system(size: 12))
                                    .foregroundStyle(.secondary)
                            }
                            Spacer()
                        }
                        .padding(.vertical, 24)
                        .background(
                            RoundedRectangle(cornerRadius: 12, style: .continuous)
                                .fill(isDark ? Color.white.opacity(0.04) : Color.black.opacity(0.03))
                        )
                    }
                    .buttonStyle(.plain)
                    Spacer()
                }

                Spacer(minLength: 0)

                HStack {
                    if snap.totalCount > 0 {
                        Text(String(localized: "共 \(snap.totalCount) 个会话"))
                            .font(.system(size: 10, weight: .medium))
                            .foregroundStyle(.secondary)
                    }
                    Spacer()
                    WidgetSyncFooter(syncedAt: snap.syncedAt, isDemo: snap.isDemo, isStale: entry.isStale)
                }
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
        } else {
            AgentStatusEmptyCardView(isPaired: entry.isPaired, compact: false)
                .widgetURL(QuotaWidgetConstants.conversationsDeepLinkURL)
        }
    }
}

// MARK: - Empty / Unpaired State

private struct AgentStatusEmptyCardView: View {
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

            Text(isPaired ? String(localized: "等待同步对话") : String(localized: "尚未配对网关"))
                .font(.system(size: compact ? 14 : 16, weight: .semibold))
                .foregroundStyle(.primary)

            Text(isPaired ? String(localized: "点击打开 App 同步最近活跃对话与任务状态") : String(localized: "点击打开 App 扫码配对本机网关"))
                .font(.system(size: compact ? 11 : 12.5))
                .foregroundStyle(.secondary)
                .lineLimit(2)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .leading)
    }
}
