import SwiftUI

public struct QuotaStatusBarView: View {
    public let account: CockpitAccountQuota?
    public let onTap: () -> Void
    
    public init(account: CockpitAccountQuota?, onTap: @escaping () -> Void) {
        self.account = account
        self.onTap = onTap
    }
    
    private var bucket: CockpitQuotaBucket? {
        account?.gemini5h
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
                    
                    Text("Gemini 5h")
                        .font(.system(size: 12, weight: .medium))
                        .foregroundColor(.primary)
                        .lineLimit(1)
                        .layoutPriority(1)
                    
                    // Mini progress bar - stretched to fill available width
                    GeometryReader { geo in
                        ZStack(alignment: .leading) {
                            Capsule()
                                .fill(Color.primary.opacity(0.08))
                            Capsule()
                                .fill(progressColor)
                                .frame(width: max(0, min(geo.size.width, geo.size.width * CGFloat(percent / 100.0))))
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
