import AppIntents
import Foundation
import SwiftUI
import WidgetKit

// MARK: - Timeline Entry & Provider

nonisolated struct QuotaWidgetEntry: TimelineEntry {
    let date: Date
    let snapshot: QuotaWidgetSnapshot?
    let isPaired: Bool
    let isStale: Bool
}

nonisolated struct QuotaTimelineProvider: TimelineProvider {
    func placeholder(in context: Context) -> QuotaWidgetEntry {
        QuotaWidgetEntry(
            date: Date(),
            snapshot: QuotaWidgetSnapshot.demoPlaceholder(),
            isPaired: true,
            isStale: false
        )
    }

    func getSnapshot(in context: Context, completion: @escaping (QuotaWidgetEntry) -> Void) {
        let now = Date()
        if context.isPreview {
            completion(QuotaWidgetEntry(
                date: now,
                snapshot: QuotaWidgetStore.shared.loadSnapshot() ?? QuotaWidgetSnapshot.demoPlaceholder(now: now),
                isPaired: true,
                isStale: false
            ))
            return
        }
        let config = QuotaWidgetStore.shared.loadConfig()
        let cached = QuotaWidgetStore.shared.loadSnapshot()
        let paired = (config?.isPaired ?? false) || (cached != nil)
        completion(QuotaWidgetEntry(
            date: now,
            snapshot: cached ?? (paired ? QuotaWidgetSnapshot.demoPlaceholder(now: now) : nil),
            isPaired: paired,
            isStale: false
        ))
    }

    func getTimeline(in context: Context, completion: @escaping (Timeline<QuotaWidgetEntry>) -> Void) {
        Task {
            let now = Date()
            let config = QuotaWidgetStore.shared.loadConfig()
            let fresh = await QuotaWidgetStore.shared.fetchLatestQuotaFromGateway()

            let entry: QuotaWidgetEntry
            if let fresh {
                entry = QuotaWidgetEntry(date: now, snapshot: fresh, isPaired: true, isStale: false)
            } else if let cached = QuotaWidgetStore.shared.loadSnapshot() {
                let stale = now.timeIntervalSince(cached.syncedAt) > 25 * 60
                entry = QuotaWidgetEntry(date: now, snapshot: cached, isPaired: true, isStale: stale)
            } else {
                let paired = config?.isPaired ?? false
                entry = QuotaWidgetEntry(date: now, snapshot: nil, isPaired: paired, isStale: false)
            }

            // Refresh every 20 minutes within iOS WidgetKit budget; App foreground & tap refresh reload immediately.
            let nextRefresh = Calendar.current.date(byAdding: .minute, value: 20, to: now)
                ?? now.addingTimeInterval(20 * 60)
            completion(Timeline(entries: [entry], policy: .after(nextRefresh)))
        }
    }
}

// MARK: - Widget Definition (Lock Screen Circular/Rectangular + Home/StandBy/CarPlay/Mac Small)

public struct QuotaStatusWidget: Widget {
    public init() {}

    public var body: some WidgetConfiguration {
        StaticConfiguration(kind: QuotaWidgetConstants.widgetKind, provider: QuotaTimelineProvider()) { entry in
            QuotaWidgetEntryView(entry: entry)
                .widgetURL(QuotaWidgetConstants.deepLinkURL)
        }
        .configurationDisplayName(String(localized: "额度监控"))
        .description(String(localized: "在锁屏、主屏幕与 CarPlay 随时查看 5 小时与 7 天额度余量及重置倒计时。"))
        .supportedFamilies([.accessoryCircular, .accessoryRectangular, .systemSmall])
    }
}

// MARK: - Root Family Switcher

private struct QuotaWidgetEntryView: View {
    let entry: QuotaWidgetEntry
    @Environment(\.widgetFamily) private var family

    var body: some View {
        Group {
            switch family {
            case .accessoryCircular:
                QuotaCircularView(entry: entry)
            case .accessoryRectangular:
                QuotaRectangularView(entry: entry)
            default:
                QuotaSmallHomeView(entry: entry)
            }
        }
        .containerBackground(for: .widget) {
            if family == .systemSmall {
                QuotaSmallCardBackground()
            } else {
                Color.clear
            }
        }
    }
}

// MARK: - 1. Lock Screen Circular (.accessoryCircular)

private struct QuotaCircularView: View {
    let entry: QuotaWidgetEntry

    var body: some View {
        if let snap = entry.snapshot, let primary = snap.primaryWindow {
            Gauge(value: primary.remainingPercent, in: 0...100) {
                Text(snap.primaryWindowTag)
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
}

// MARK: - 2. Lock Screen Rectangular (.accessoryRectangular)

private struct QuotaRectangularView: View {
    let entry: QuotaWidgetEntry

    var body: some View {
        if let snap = entry.snapshot {
            VStack(alignment: .leading, spacing: 3.5) {
                // Header: Provider + Live Ticking Reset Countdown
                HStack(spacing: 4) {
                    Image(systemName: "bolt.fill")
                        .font(.system(size: 10, weight: .bold))
                        .widgetAccentable()
                    Text(snap.providerTitle)
                        .font(.system(size: 12, weight: .bold))
                        .lineLimit(1)
                    Spacer(minLength: 4)
                    resetCountdownView(for: snap.primaryWindow, compact: true)
                        .font(.system(size: 10.5, weight: .medium, design: .rounded))
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                }

                if let short = snap.shortWindow {
                    rectangularBarRow(tag: "5h", bucket: short)
                }
                if let weekly = snap.weeklyWindow {
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

// MARK: - 3. Home Screen / StandBy / CarPlay Widgets / Mac Desktop (.systemSmall)

private struct QuotaSmallCardBackground: View {
    @Environment(\.colorScheme) private var colorScheme

    var body: some View {
        if colorScheme == .dark {
            LinearGradient(
                colors: [
                    Color(red: 0.11, green: 0.12, blue: 0.16),
                    Color(red: 0.07, green: 0.08, blue: 0.11)
                ],
                startPoint: .topLeading,
                endPoint: .bottomTrailing
            )
        } else {
            LinearGradient(
                colors: [
                    Color(red: 0.99, green: 0.99, blue: 1.0),
                    Color(red: 0.95, green: 0.96, blue: 0.98)
                ],
                startPoint: .topLeading,
                endPoint: .bottomTrailing
            )
        }
    }
}

private struct QuotaSmallHomeView: View {
    let entry: QuotaWidgetEntry
    @Environment(\.colorScheme) private var colorScheme
    @Environment(\.widgetRenderingMode) private var renderingMode

    private var isDark: Bool { colorScheme == .dark }
    private var isFullColor: Bool { renderingMode == .fullColor }

    var body: some View {
        if let snap = entry.snapshot, let primary = snap.primaryWindow {
            VStack(alignment: .leading, spacing: 0) {
                // Header: Provider + Interactive Refresh Button
                HStack(alignment: .center, spacing: 5) {
                    Image(systemName: "bolt.fill")
                        .font(.system(size: 11.5, weight: .bold))
                        .foregroundStyle(isFullColor ? quotaTintColor(percent: primary.remainingPercent) : .primary)
                        .widgetAccentable()

                    Text(snap.providerTitle)
                        .font(.system(size: 13.5, weight: .bold))
                        .foregroundStyle(.primary)
                        .lineLimit(1)

                    Spacer(minLength: 4)

                    Button(intent: RefreshQuotaWidgetIntent()) {
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

                Spacer(minLength: 6)

                // Hero Percentage + Primary Window Pill & Live Reset Countdown
                HStack(alignment: .lastTextBaseline, spacing: 5) {
                    Text("\(primary.intPercent)%")
                        .font(.system(size: 30, weight: .bold, design: .rounded))
                        .foregroundStyle(isFullColor ? quotaTintColor(percent: primary.remainingPercent) : .primary)
                        .minimumScaleFactor(0.75)
                        .lineLimit(1)
                        .widgetAccentable()

                    Text(snap.primaryWindowTag)
                        .font(.system(size: 11, weight: .bold, design: .rounded))
                        .foregroundStyle(.secondary)
                        .padding(.horizontal, 5.5)
                        .padding(.vertical, 1.5)
                        .background(
                            Capsule().fill(isDark ? Color.white.opacity(0.10) : Color.black.opacity(0.06))
                        )
                }

                // Ticking Reset Countdown
                HStack(spacing: 3) {
                    Image(systemName: "clock")
                        .font(.system(size: 9.5, weight: .medium))
                    resetCountdownView(for: primary, compact: false)
                        .font(.system(size: 10.5, weight: .medium, design: .rounded))
                        .lineLimit(1)
                }
                .foregroundStyle(.secondary)
                .padding(.top, 2)

                Spacer(minLength: 8)

                // Dual Window Bars (5h + 7d)
                VStack(spacing: 5.5) {
                    if let short = snap.shortWindow {
                        smallBarRow(tag: "5h", bucket: short)
                    }
                    if let weekly = snap.weeklyWindow {
                        smallBarRow(tag: "7d", bucket: weekly)
                    }
                }

                Spacer(minLength: 5)

                // Footer: Synced timestamp / Demo indicator
                HStack(spacing: 4) {
                    if snap.isDemo {
                        Text("Demo")
                            .font(.system(size: 9.5, weight: .semibold))
                            .foregroundStyle(.secondary)
                    } else {
                        if entry.isStale {
                            Image(systemName: "icloud.slash")
                                .font(.system(size: 9))
                                .foregroundStyle(.secondary)
                        }
                        Text(snap.syncedAt, style: .relative)
                            .font(.system(size: 9.5, weight: .medium))
                            .foregroundStyle(.secondary)
                            .lineLimit(1)
                    }
                    Spacer()
                }
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .leading)
        } else {
            VStack(alignment: .leading, spacing: 8) {
                HStack(spacing: 6) {
                    Image("AppLogoTransparent")
                        .renderingMode(.original)
                        .resizable()
                        .aspectRatio(contentMode: .fit)
                        .frame(width: 22, height: 15)
                    Text("Multigravity")
                        .font(.system(size: 13, weight: .bold))
                        .lineLimit(1)
                }

                Spacer()

                Text(entry.isPaired ? String(localized: "等待同步额度") : String(localized: "尚未配对网关"))
                    .font(.system(size: 14, weight: .semibold))
                    .foregroundStyle(.primary)

                Text(entry.isPaired ? String(localized: "点击打开 App 获取最新 5h/7d 额度") : String(localized: "点击打开 App 扫码配对本机网关"))
                    .font(.system(size: 11))
                    .foregroundStyle(.secondary)
                    .lineLimit(2)
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .leading)
        }
    }

    private func smallBarRow(tag: String, bucket: QuotaWidgetBucketSnapshot) -> some View {
        let tint = isFullColor ? quotaTintColor(percent: bucket.remainingPercent) : Color.primary
        return HStack(spacing: 6) {
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

// MARK: - Shared Formatting & Semantic Helpers

private func quotaTintColor(percent: Double) -> Color {
    if percent >= 50 {
        return Color(red: 0.20, green: 0.78, blue: 0.42)
    } else if percent >= 20 {
        return .orange
    } else {
        return .red
    }
}

@ViewBuilder
private func resetCountdownView(for bucket: QuotaWidgetBucketSnapshot?, compact: Bool) -> some View {
    if let resetAt = bucket?.resetAt, resetAt > Date() {
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
