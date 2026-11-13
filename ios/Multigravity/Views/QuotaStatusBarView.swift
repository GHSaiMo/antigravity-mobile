import SwiftUI

public struct QuotaStatusBarView: View {
    public let account: CockpitAccountQuota?
    public let onTap: () -> Void
    /// When set, a small connection-heartbeat dot is drawn at the far right of the bar.
    public let linkMonitor: ConversationEventsClient?
    
    public init(account: CockpitAccountQuota?, linkMonitor: ConversationEventsClient? = nil, onTap: @escaping () -> Void) {
        self.account = account
        self.linkMonitor = linkMonitor
        self.onTap = onTap
    }
    
    private var isFree: Bool {
        account?.subscriptionTier?.trimmingCharacters(in: .whitespacesAndNewlines).uppercased() == "FREE" ||
            (account?.gemini5h == nil && account?.claude5h == nil)
    }
    
    private var bucket: CockpitQuotaBucket? {
        if isFree {
            return account?.geminiWeekly ?? account?.claudeWeekly
        }
        return account?.gemini5h ?? account?.geminiWeekly
    }
    
    private var titleText: String {
        if isFree {
            return account?.geminiWeekly != nil ? "Gemini Weekly" : "Claude Weekly"
        }
        return account?.gemini5h != nil ? "Gemini 5h" : "Gemini Weekly"
    }
    
    private var percent: Double {
        bucket?.remainingPercent ?? 0.0
    }
    
    private var progressColor: Color {
        if percent >= 50 {
            return .green
        } else if percent >= 20 {
            return .orange
        } else {
            return .red
        }
    }
    
    private var formattedResetClockTime: String? {
        guard let resetTimeStr = bucket?.resetTime, !resetTimeStr.isEmpty else {
            return nil
        }
        
        let date: Date?
        let fmtFraction = ISO8601DateFormatter()
        fmtFraction.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        if let d = fmtFraction.date(from: resetTimeStr) {
            date = d
        } else {
            let fmtStandard = ISO8601DateFormatter()
            fmtStandard.formatOptions = [.withInternetDateTime]
            if let d = fmtStandard.date(from: resetTimeStr) {
                date = d
            } else {
                let df = DateFormatter()
                df.locale = Locale(identifier: "en_US_POSIX")
                df.dateFormat = "yyyy-MM-dd'T'HH:mm:ssZ"
                date = df.date(from: resetTimeStr)
            }
        }
        
        guard let validDate = date else {
            return nil
        }
        
        let outputFormatter = DateFormatter()
        outputFormatter.locale = Locale(identifier: "en_US_POSIX")
        outputFormatter.timeZone = TimeZone.current
        outputFormatter.dateFormat = "MM/dd HH:mm"
        return "(\(outputFormatter.string(from: validDate)))"
    }
    
    private var resetCountdownDisplay: String? {
        guard let friendly = bucket?.resetFriendly, !friendly.isEmpty else {
            return nil
        }
        if friendly == "已就绪" || friendly == "未知" {
            return friendly
        }
        if let clock = formattedResetClockTime {
            return "\(friendly) \(clock)"
        }
        return friendly
    }
    
    public var body: some View {
        guard let _ = bucket else { return AnyView(EmptyView()) }
        
        return AnyView(
            Button(action: onTap) {
                HStack(spacing: 6) {
                    Image(systemName: "bolt.fill")
                        .font(.system(size: 11, weight: .bold))
                        .foregroundColor(progressColor)
                    
                    Text(titleText)
                        .font(.system(size: 12, weight: .medium))
                        .foregroundColor(.primary)
                        .lineLimit(1)
                        .layoutPriority(1)
                    
                    // Mini progress bar - stretched to fill available width with pace needle
                    let paceInfo = bucket?.calculatePaceForTitle(titleText, tier: account?.subscriptionTier)
                    let showCursor = AppSettings.shared.weeklyPaceCursorEnabled && paceInfo != nil

                    GeometryReader { geo in
                        ZStack(alignment: .leading) {
                            Capsule()
                                .fill(Color.primary.opacity(0.08))
                            Capsule()
                                .fill(progressColor)
                                .frame(width: max(0, min(geo.size.width, geo.size.width * CGFloat(percent / 100.0))))

                            if showCursor, let info = paceInfo {
                                let expectedFrac = CGFloat(info.expectedRemainingPercent / 100.0)
                                let needleOffset = max(0, min(geo.size.width - 1.5, geo.size.width * expectedFrac - 0.75))
                                let needleColor = info.isOverPacing ? Color.orange : Color.white.opacity(0.92)

                                RoundedRectangle(cornerRadius: 0.75, style: .continuous)
                                    .fill(needleColor)
                                    .frame(width: 1.5, height: 9.0)
                                    .shadow(color: Color.black.opacity(0.35), radius: 1, x: 0, y: 0.5)
                                    .offset(x: needleOffset, y: (6.0 - 9.0) / 2.0)
                            }
                        }
                    }
                    .frame(height: 6)
                    .frame(minWidth: 24)
                    
                    Text(String(format: "%.0f%%", percent))
                        .font(.system(size: 12, weight: .semibold, design: .monospaced))
                        .foregroundColor(progressColor)
                        .lineLimit(1)
                        .layoutPriority(1)
                    
                    if let countdownText = resetCountdownDisplay {
                        Text(countdownText)
                            .font(.system(size: 10))
                            .foregroundColor(.secondary)
                            .lineLimit(1)
                            .minimumScaleFactor(0.85)
                            .layoutPriority(1)
                    }
                    
                    if let monitor = linkMonitor {
                        LinkHealthDot(monitor: monitor)
                    }
                }
                .frame(maxWidth: .infinity)
                .padding(.horizontal, 12)
                .padding(.vertical, 8)
                .background(
                    RoundedRectangle(cornerRadius: 12, style: .continuous)
                        .fill(Color(uiColor: .secondarySystemGroupedBackground))
                        .shadow(color: Color.black.opacity(0.04), radius: 3, x: 0, y: 1)
                )
                .overlay(
                    RoundedRectangle(cornerRadius: 12, style: .continuous)
                        .stroke(Color.primary.opacity(0.06), lineWidth: 0.5)
                )
            }
            .buttonStyle(.plain)
            .frame(maxWidth: .infinity)
        )
    }
}
