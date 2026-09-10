import SwiftUI

public struct AccountQuotaSheet: View {
    @Environment(\.dismiss) private var dismiss
    @AppStorage("cockpit_email_masked") private var isMasked: Bool = false
    @State private var switchingAccountId: String? = nil
    @State private var errorMessage: String? = nil
    public let quotaResponse: CockpitQuotaResponse?
    public let onSwitch: ((String) async throws -> Void)?
    
    public init(quotaResponse: CockpitQuotaResponse?, onSwitch: ((String) async throws -> Void)? = nil) {
        self.quotaResponse = quotaResponse
        self.onSwitch = onSwitch
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
    
    private func maskEmail(_ email: String) -> String {
        let parts = email.split(separator: "@", maxSplits: 1, omittingEmptySubsequences: false)
        guard parts.count == 2 else { return email }
        let local = String(parts[0])
        let domain = String(parts[1])
        
        func maskPart(_ str: String) -> String {
            let chars = Array(str)
            guard chars.count > 0 else { return "" }
            if chars.count == 1 { return String(chars[0]) + "*" }
            if chars.count == 2 { return String(chars[0]) + "*" + String(chars[1]) }
            let middleCount = chars.count - 2
            return String(chars[0]) + String(repeating: "*", count: middleCount) + String(chars[chars.count - 1])
        }
        
        if let dotIdx = domain.lastIndex(of: ".") {
            let domainName = String(domain[..<dotIdx])
            let domainExt = String(domain[dotIdx...])
            return "\(maskPart(local))@\(maskPart(domainName))\(domainExt)"
        }
        return "\(maskPart(local))@\(maskPart(domain))"
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
            .navigationTitle("Cockpit Tools")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    Button(action: { isMasked.toggle() }) {
                        Image(systemName: isMasked ? "eye.slash.fill" : "eye.slash")
                            .font(.system(size: 15, weight: .medium))
                            .foregroundColor(isMasked ? .blue : .secondary)
                    }
                }
                ToolbarItem(placement: .topBarTrailing) {
                    Button("完成") {
                        dismiss()
                    }
                }
            }
            .alert("切换账号失败", isPresented: Binding(get: { errorMessage != nil }, set: { if !$0 { errorMessage = nil } })) {
                Button("好", role: .cancel) { errorMessage = nil }
            } message: {
                if let msg = errorMessage {
                    Text(msg)
                }
            }
        }
    }
    
    private func performSwitch(to acc: CockpitAccountQuota) async {
        guard let onSwitch = onSwitch else { return }
        switchingAccountId = acc.id
        UIImpactFeedbackGenerator(style: .medium).impactOccurred()
        do {
            try await onSwitch(acc.id)
            UINotificationFeedbackGenerator().notificationOccurred(.success)
        } catch {
            UINotificationFeedbackGenerator().notificationOccurred(.error)
            errorMessage = error.localizedDescription
        }
        switchingAccountId = nil
    }
    
    @ViewBuilder
    private func accountCard(for acc: CockpitAccountQuota, isCurrent: Bool) -> some View {
        VStack(alignment: .leading, spacing: 12) {
            // Header: Email only (no name) + Switch button for non-current accounts
            HStack {
                let displayEmail = isMasked ? maskEmail(acc.email) : acc.email
                Text(displayEmail)
                    .font(.system(size: 15, weight: .semibold))
                    .foregroundColor(.primary)
                    .lineLimit(1)
                Spacer()
                
                if !isCurrent && onSwitch != nil {
                    Button(action: {
                        Task {
                            await performSwitch(to: acc)
                        }
                    }) {
                        if switchingAccountId == acc.id {
                            ProgressView()
                                .progressViewStyle(CircularProgressViewStyle())
                                .scaleEffect(0.7)
                                .frame(width: 44, height: 22)
                        } else {
                            Text("切换")
                                .font(.system(size: 11, weight: .semibold))
                                .foregroundColor(.white)
                                .padding(.horizontal, 10)
                                .padding(.vertical, 4)
                                .background(Color.blue)
                                .clipShape(Capsule())
                        }
                    }
                    .buttonStyle(.plain)
                    .disabled(switchingAccountId != nil)
                }
            }
            
            Divider()
                .opacity(0.6)
            
            // 4 Metrics Grid: Claude on Left, Gemini on Right, 5h on top, Weekly on bottom
            LazyVGrid(columns: [
                GridItem(.flexible(), spacing: 10),
                GridItem(.flexible(), spacing: 10)
            ], spacing: 10) {
                // Row 1: Left Claude 5h, Right Gemini 5h
                metricCell(title: "Claude 5h", bucket: acc.claude5h)
                metricCell(title: "Gemini 5h", bucket: acc.gemini5h)
                
                // Row 2: Left Claude Weekly, Right Gemini Weekly
                metricCell(title: "Claude Weekly", bucket: acc.claudeWeekly)
                metricCell(title: "Gemini Weekly", bucket: acc.geminiWeekly)
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
