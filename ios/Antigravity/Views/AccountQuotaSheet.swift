import SwiftUI

public struct AccountQuotaSheet: View {
    @Environment(\.dismiss) private var dismiss
    public let quotaResponse: CockpitQuotaResponse?
    public let isRefreshing: Bool
    public let onRefresh: () -> Void
    
    public init(
        quotaResponse: CockpitQuotaResponse?,
        isRefreshing: Bool,
        onRefresh: @escaping () -> Void
    ) {
        self.quotaResponse = quotaResponse
        self.isRefreshing = isRefreshing
        self.onRefresh = onRefresh
    }
    
    private var currentAccount: CockpitAccountQuota? {
        quotaResponse?.currentAccount
    }
    
    private var otherAccounts: [CockpitAccountQuota] {
        guard let all = quotaResponse?.accounts else { return [] }
        if let cur = currentAccount {
            return all.filter { $0.id != cur.id }
        }
        return all
    }
    
    public var body: some View {
        NavigationStack {
            ScrollView {
                VStack(spacing: 18) {
                    if let current = currentAccount {
                        VStack(alignment: .leading, spacing: 10) {
                            HStack {
                                Text("当前使用账号")
                                    .font(.system(size: 13, weight: .semibold))
                                    .foregroundColor(.secondary)
                                    .textCase(.uppercase)
                                
                                Spacer()
                                
                                HStack(spacing: 4) {
                                    Circle()
                                        .fill(Color.blue)
                                        .frame(width: 6, height: 6)
                                    Text("使用中")
                                        .font(.system(size: 11, weight: .semibold))
                                        .foregroundColor(.blue)
                                }
                                .padding(.horizontal, 8)
                                .padding(.vertical, 3)
                                .background(Color.blue.opacity(0.12))
                                .clipShape(Capsule())
                            }
                            .padding(.horizontal, 4)
                            
                            accountCard(for: current, isCurrent: true)
                        }
                    }
                    
                    if !otherAccounts.isEmpty {
                        VStack(alignment: .leading, spacing: 10) {
                            Text("备用账号 (\(otherAccounts.count))")
                                .font(.system(size: 13, weight: .semibold))
                                .foregroundColor(.secondary)
                                .textCase(.uppercase)
                                .padding(.horizontal, 4)
                            
                            ForEach(otherAccounts) { acc in
                                accountCard(for: acc, isCurrent: false)
                            }
                        }
                    }
                    
                    if let updated = quotaResponse?.updatedAt, updated > 0 {
                        let date = Date(timeIntervalSince1970: Double(updated) / 1000.0)
                        let timeStr = date.formatted(date: .omitted, time: .standard)
                        Text("配额数据更新于 \(timeStr)")
                            .font(.system(size: 11))
                            .foregroundColor(.secondary.opacity(0.7))
                            .padding(.top, 4)
                    }
                }
                .padding(16)
            }
            .background(Color(uiColor: .systemGroupedBackground))
            .navigationTitle("Cockpit 账号配额")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    Button("完成") {
                        dismiss()
                    }
                }
                ToolbarItem(placement: .topBarTrailing) {
                    Button(action: onRefresh) {
                        Image(systemName: "arrow.clockwise")
                            .font(.system(size: 14, weight: .medium))
                            .rotationEffect(.degrees(isRefreshing ? 360 : 0))
                            .animation(isRefreshing ? .linear(duration: 1).repeatForever(autoreverses: false) : .default, value: isRefreshing)
                    }
                    .disabled(isRefreshing)
                }
            }
        }
    }
    
    @ViewBuilder
    private func accountCard(for acc: CockpitAccountQuota, isCurrent: Bool) -> some View {
        VStack(alignment: .leading, spacing: 12) {
            // Header
            HStack {
                VStack(alignment: .leading, spacing: 2) {
                    Text(acc.email)
                        .font(.system(size: 15, weight: .semibold))
                        .foregroundColor(.primary)
                        .lineLimit(1)
                    
                    if !acc.name.isEmpty {
                        Text(acc.name)
                            .font(.system(size: 12))
                            .foregroundColor(.secondary)
                            .lineLimit(1)
                    }
                }
                Spacer()
            }
            
            Divider()
                .opacity(0.6)
            
            // 4 Metrics Grid
            LazyVGrid(columns: [
                GridItem(.flexible(), spacing: 10),
                GridItem(.flexible(), spacing: 10)
            ], spacing: 10) {
                metricCell(title: "Claude 5小时", bucket: acc.claude5h)
                metricCell(title: "Claude 一周", bucket: acc.claudeWeekly)
                metricCell(title: "Gemini 5小时", bucket: acc.gemini5h)
                metricCell(title: "Gemini 一周", bucket: acc.geminiWeekly)
            }
        }
        .padding(14)
        .background(
            RoundedRectangle(cornerRadius: 14, style: .continuous)
                .fill(Color(uiColor: .secondarySystemGroupedBackground))
                .shadow(color: Color.black.opacity(0.03), radius: 4, x: 0, y: 1)
        )
        .overlay(
            RoundedRectangle(cornerRadius: 14, style: .continuous)
                .stroke(isCurrent ? Color.blue.opacity(0.35) : Color.primary.opacity(0.06), lineWidth: isCurrent ? 1.5 : 0.5)
        )
    }
    
    @ViewBuilder
    private func metricCell(title: String, bucket: CockpitQuotaBucket?) -> some View {
        let percent = bucket?.remainingPercent ?? 0.0
        let color = colorForPercent(percent)
        
        VStack(alignment: .leading, spacing: 4) {
            HStack {
                Text(title)
                    .font(.system(size: 11, weight: .medium))
                    .foregroundColor(.secondary)
                    .lineLimit(1)
                Spacer()
                Text(String(format: "%.1f%%", percent))
                    .font(.system(size: 12, weight: .bold, design: .monospaced))
                    .foregroundColor(color)
            }
            
            // Mini progress bar
            GeometryReader { geo in
                ZStack(alignment: .leading) {
                    Capsule()
                        .fill(Color.primary.opacity(0.08))
                    Capsule()
                        .fill(color)
                        .frame(width: max(0, min(geo.size.width, geo.size.width * CGFloat(percent / 100.0))))
                }
            }
            .frame(height: 5)
            
            if let friendly = bucket?.resetFriendly, !friendly.isEmpty {
                Text(friendly)
                    .font(.system(size: 9.5))
                    .foregroundColor(.secondary)
                    .lineLimit(1)
            } else {
                Text("配额充足")
                    .font(.system(size: 9.5))
                    .foregroundColor(.secondary.opacity(0.6))
                    .lineLimit(1)
            }
        }
        .padding(9)
        .background(
            RoundedRectangle(cornerRadius: 8, style: .continuous)
                .fill(Color(uiColor: .tertiarySystemGroupedBackground))
        )
    }
    
    private func colorForPercent(_ p: Double) -> Color {
        if p >= 50 {
            return .green
        } else if p >= 20 {
            return .orange
        } else {
            return .red
        }
    }
}
