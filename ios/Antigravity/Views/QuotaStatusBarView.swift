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
    
    public var body: some View {
        guard let b = bucket else { return AnyView(EmptyView()) }
        
        return AnyView(
            Button(action: onTap) {
                HStack(spacing: 8) {
                    Image(systemName: "bolt.fill")
                        .font(.system(size: 11, weight: .bold))
                        .foregroundColor(progressColor)
                    
                    Text("5h 额度:")
                        .font(.system(size: 12, weight: .medium))
                        .foregroundColor(.primary)
                    
                    // Mini progress bar
                    GeometryReader { geo in
                        ZStack(alignment: .leading) {
                            Capsule()
                                .fill(Color.primary.opacity(0.08))
                            Capsule()
                                .fill(progressColor)
                                .frame(width: max(0, min(geo.size.width, geo.size.width * CGFloat(percent / 100.0))))
                        }
                    }
                    .frame(width: 50, height: 6)
                    
                    Text(String(format: "%.0f%%", percent))
                        .font(.system(size: 12, weight: .semibold, design: .monospaced))
                        .foregroundColor(progressColor)
                    
                    if let friendly = b.resetFriendly, !friendly.isEmpty {
                        Text(friendly)
                            .font(.system(size: 10))
                            .foregroundColor(.secondary)
                            .lineLimit(1)
                    }
                    
                    Spacer(minLength: 0)
                    
                    Image(systemName: "chevron.right")
                        .font(.system(size: 11, weight: .semibold))
                        .foregroundColor(.secondary.opacity(0.6))
                }
                .padding(.horizontal, 12)
                .padding(.vertical, 7)
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
            .padding(.horizontal, 16)
            .padding(.vertical, 4)
        )
    }
}
