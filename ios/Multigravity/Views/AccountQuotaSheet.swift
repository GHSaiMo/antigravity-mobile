import SwiftUI

public struct AccountQuotaSheet: View {
    @Environment(\.dismiss) private var dismiss
    @Environment(\.colorScheme) private var colorScheme
    @AppStorage("cockpit_email_masked") private var isMasked: Bool = false
    @State private var switchingAccountId: String? = nil
    @State private var pendingSwitch: CockpitAccountQuota? = nil
    @State private var showSwitchConfirm: Bool = false
    @State private var isRefreshing: Bool = false
    @State private var errorMessage: String? = nil
    @State private var alertTitle: String = "操作失败"
    @State private var paceDetailItem: WeeklyPaceDetailItem? = nil
    @State private var selectedTab: Int = 0
    @State private var selectedHeatmapDate: String? = nil
    @Binding public var quotaResponse: CockpitQuotaResponse?
    public let tokenHistory: TokenHistoryResponse?
    public let isLoadingTokenHistory: Bool
    public let onLoadTokenHistory: ((Bool) async -> Void)?
    public let onSwitch: ((String) async throws -> Void)?
    public let onRefresh: (() async throws -> Void)?
    public let onAppearFetch: (() async -> Void)?
    
    /// 是否显示账号切换按钮。网关会先退出电脑上的 Antigravity 再注入新账号。
    public let enableSwitchButton: Bool
    /// Live gateway link monitor; when set the sheet shows a connection detail card with a reconnect button.
    public let linkMonitor: ConversationEventsClient?
    
    public init(
        quotaResponse: Binding<CockpitQuotaResponse?>,
        tokenHistory: TokenHistoryResponse? = nil,
        isLoadingTokenHistory: Bool = false,
        onLoadTokenHistory: ((Bool) async -> Void)? = nil,
        onSwitch: ((String) async throws -> Void)? = nil,
        onRefresh: (() async throws -> Void)? = nil,
        onAppearFetch: (() async -> Void)? = nil,
        enableSwitchButton: Bool = true,
        linkMonitor: ConversationEventsClient? = nil
    ) {
        self.linkMonitor = linkMonitor
        self._quotaResponse = quotaResponse
        self.tokenHistory = tokenHistory
        self.isLoadingTokenHistory = isLoadingTokenHistory
        self.onLoadTokenHistory = onLoadTokenHistory
        self.onSwitch = onSwitch
        self.onRefresh = onRefresh
        self.onAppearFetch = onAppearFetch
        self.enableSwitchButton = enableSwitchButton
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
    
    private var cockpitInstallHintCard: some View {
        VStack(alignment: .leading, spacing: 10) {
            Text("备用账号")
                .font(.system(size: 13, weight: .semibold))
                .foregroundColor(.secondary)
                .padding(.horizontal, 4)
            VStack(alignment: .leading, spacing: 10) {
                Label("未检测到 Cockpit Tools", systemImage: "info.circle")
                    .font(.system(size: 14, weight: .semibold))
                Text("请安装 Cockpit Tools,并在网关里用 mgy cockpit 进行配置,即可享受备用账号额度总览与一键切换等功能。")
                    .font(.system(size: 13))
                    .foregroundColor(.secondary)
                Button(action: { UIPasteboard.general.string = "mgy cockpit" }) {
                    Label("复制命令 mgy cockpit", systemImage: "doc.on.doc")
                        .font(.system(size: 12, weight: .medium))
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(14)
            .background(Color(uiColor: .secondarySystemGroupedBackground))
            .clipShape(RoundedRectangle(cornerRadius: 14))
        }
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
            VStack(spacing: 0) {
                Picker("视图模式", selection: $selectedTab) {
                    Text("实时配额").tag(0)
                    Text("Token 历史").tag(1)
                }
                .pickerStyle(.segmented)
                .padding(.horizontal, 16)
                .padding(.vertical, 10)
                .background(Color(uiColor: .systemGroupedBackground))
                .onChange(of: selectedTab) { _, newTab in
                    if newTab == 1 && tokenHistory == nil {
                        Task { await onLoadTokenHistory?(false) }
                    }
                }

                ScrollView {
                    VStack(spacing: 18) {
                        if selectedTab == 1 {
                            tokenHistorySection
                        } else {
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
                            
                            if quotaResponse?.needsCockpitInstall == true {
                                cockpitInstallHintCard
                            } else if !otherAccounts.isEmpty {
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
                    }
                    .padding(16)
                }
            }
            .background(Color(uiColor: .systemGroupedBackground))
            .navigationTitle("账户与配额")
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
                    Button(action: {
                        Task {
                            if selectedTab == 0 {
                                await performRefresh()
                            } else {
                                await onLoadTokenHistory?(true)
                            }
                        }
                    }) {
                        let spinning = selectedTab == 0 ? isRefreshing : isLoadingTokenHistory
                        if #available(iOS 18.0, *) {
                            Image(systemName: "arrow.clockwise")
                                .font(.system(size: 15, weight: .semibold))
                                .symbolEffect(.rotate.clockwise, isActive: spinning)
                        } else {
                            Image(systemName: "arrow.clockwise")
                                .font(.system(size: 15, weight: .semibold))
                                .rotationEffect(.degrees(spinning ? 360 : 0))
                                .animation(
                                    spinning
                                        ? .linear(duration: 1).repeatForever(autoreverses: false)
                                        : .default,
                                    value: spinning
                                )
                        }
                    }
                    .disabled((selectedTab == 0 ? isRefreshing : isLoadingTokenHistory) || switchingAccountId != nil)
                }
            }
            .presentationDragIndicator(.visible)
            .task {
                guard let onAppearFetch = onAppearFetch else { return }
                await onAppearFetch()
                while !Task.isCancelled {
                    try? await Task.sleep(nanoseconds: 10_000_000_000)
                    guard !Task.isCancelled else { break }
                    await onAppearFetch()
                }
            }
            .alert(alertTitle, isPresented: Binding(get: { errorMessage != nil }, set: { if !$0 { errorMessage = nil } })) {
                Button("好", role: .cancel) { errorMessage = nil }
            } message: {
                if let msg = errorMessage {
                    Text(msg)
                }
            }
            .confirmationDialog(
                "确认切换账号",
                isPresented: $showSwitchConfirm,
                titleVisibility: .visible
            ) {
                Button("切换并重启 Antigravity") {
                    if let acc = pendingSwitch {
                        Task { await performSwitch(to: acc) }
                    }
                    pendingSwitch = nil
                }
                Button("取消", role: .cancel) {
                    pendingSwitch = nil
                }
            } message: {
                if let acc = pendingSwitch {
                    Text("切换到 \(acc.email) 将关闭并重启电脑上的 Antigravity。")
                } else {
                    Text("切换账号将关闭并重启电脑上的 Antigravity。")
                }
            }
            .sheet(item: $paceDetailItem) { item in
                WeeklyPaceDetailSheet(item: item)
                    .presentationDetents([.fraction(0.48), .medium])
                    .presentationDragIndicator(.visible)
            }
        }
    }
    
    private func performRefresh() async {
        guard !isRefreshing else { return }
        isRefreshing = true
        UIImpactFeedbackGenerator(style: .medium).impactOccurred()
        do {
            if let onRefresh = onRefresh {
                try await onRefresh()
            }
            UINotificationFeedbackGenerator().notificationOccurred(.success)
        } catch {
            UINotificationFeedbackGenerator().notificationOccurred(.error)
            alertTitle = "刷新配额失败"
            errorMessage = error.localizedDescription
        }
        isRefreshing = false
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
            alertTitle = "切换账号失败"
            errorMessage = error.localizedDescription
        }
        switchingAccountId = nil
    }
    
    @ViewBuilder
    private func accountCard(for acc: CockpitAccountQuota, isCurrent: Bool) -> some View {
        VStack(alignment: .leading, spacing: 12) {
            // Header: Email + Tier Badge + Switch button for non-current accounts
            HStack(spacing: 6) {
                let displayEmail = isMasked ? maskEmail(acc.email) : acc.email
                Text(displayEmail)
                    .font(.system(size: 15, weight: .semibold))
                    .foregroundColor(.primary)
                    .lineLimit(1)
                
                tierBadge(for: acc.subscriptionTier)
                
                Spacer(minLength: 4)
                
                if !isCurrent && enableSwitchButton && onSwitch != nil {
                    Button(action: {
                        pendingSwitch = acc
                        showSwitchConfirm = true
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
            
            let isFree = (acc.subscriptionTier?.trimmingCharacters(in: .whitespacesAndNewlines).uppercased() == "FREE") ||
                (acc.claude5h == nil && acc.gemini5h == nil)
            
            VStack(spacing: 10) {
                if isFree {
                    // Free 账号仅展示两个 weekly 数值：左 Claude Weekly，右 Gemini Weekly
                    LazyVGrid(columns: [
                        GridItem(.flexible(), spacing: 10),
                        GridItem(.flexible(), spacing: 10)
                    ], spacing: 10) {
                        metricCell(title: "Claude Weekly", bucket: acc.claudeWeekly, tier: acc.subscriptionTier)
                        metricCell(title: "Gemini Weekly", bucket: acc.geminiWeekly, tier: acc.subscriptionTier)
                    }
                } else {
                    // 4 Metrics Grid: Claude on Left, Gemini on Right, 5h on top, Weekly on bottom
                    LazyVGrid(columns: [
                        GridItem(.flexible(), spacing: 10),
                        GridItem(.flexible(), spacing: 10)
                    ], spacing: 10) {
                        // Row 1: Left Claude 5h, Right Gemini 5h
                        metricCell(title: "Claude 5h", bucket: acc.claude5h, tier: acc.subscriptionTier)
                        metricCell(title: "Gemini 5h", bucket: acc.gemini5h, tier: acc.subscriptionTier)
                        
                        // Row 2: Left Claude Weekly, Right Gemini Weekly
                        metricCell(title: "Claude Weekly", bucket: acc.claudeWeekly, tier: acc.subscriptionTier)
                        metricCell(title: "Gemini Weekly", bucket: acc.geminiWeekly, tier: acc.subscriptionTier)
                    }
                }
                
                let isUltra = (acc.subscriptionTier?.trimmingCharacters(in: .whitespacesAndNewlines).uppercased() == "ULTRA")
                if isUltra {
                    // Ultra 专属通栏：展示 Gemini 4 Carbon Weekly，数据对齐 geminiWeekly
                    if let carbon = acc.geminiWeekly ?? acc.gemini4Argon {
                        metricCell(title: "Gemini 4 Carbon Weekly", bucket: carbon, isCarbon: true, tier: acc.subscriptionTier)
                    }
                } else {
                    // 非 Ultra（如 Pro）：若具备 Argon 额度，展示 Gemini 4 Argon Weekly
                    if let argon = acc.gemini4Argon {
                        metricCell(title: "Gemini 4 Argon Weekly", bucket: argon, isArgon: true, tier: acc.subscriptionTier)
                    }
                }
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
    
    private func carbonAuroraGradientColors(isDark: Bool) -> [Color] {
        isDark
            ? [
                Color(red: 66.0 / 255.0, green: 133.0 / 255.0, blue: 244.0 / 255.0),
                Color(red: 155.0 / 255.0, green: 114.0 / 255.0, blue: 207.0 / 255.0),
                Color(red: 1.0, green: 117.0 / 255.0, blue: 151.0 / 255.0)
            ]
            : [
                Color(red: 66.0 / 255.0, green: 133.0 / 255.0, blue: 244.0 / 255.0),
                Color(red: 155.0 / 255.0, green: 114.0 / 255.0, blue: 207.0 / 255.0),
                Color(red: 240.0 / 255.0, green: 98.0 / 255.0, blue: 146.0 / 255.0)
            ]
    }
    
    @ViewBuilder
    private func tierBadge(for rawTier: String?) -> some View {
        let tier = rawTier?.trimmingCharacters(in: .whitespacesAndNewlines).uppercased() ?? ""
        if tier == "ULTRA" {
            Text("ULTRA")
                .font(.system(size: 9.5, weight: .heavy))
                .tracking(0.5)
                .foregroundColor(.white)
                .shadow(color: Color(red: 76.0 / 255.0, green: 29.0 / 255.0, blue: 149.0 / 255.0).opacity(0.35), radius: 1, x: 0, y: 0.8)
                .padding(.horizontal, 6)
                .padding(.vertical, 2.5)
                .background(
                    LinearGradient(
                        colors: carbonAuroraGradientColors(isDark: colorScheme == .dark),
                        startPoint: .leading,
                        endPoint: .trailing
                    )
                )
                .clipShape(RoundedRectangle(cornerRadius: 5, style: .continuous))
        } else if tier == "PRO" {
            Text("PRO")
                .font(.system(size: 9.5, weight: .bold))
                .tracking(0.4)
                .foregroundColor(.white)
                .padding(.horizontal, 6)
                .padding(.vertical, 2.5)
                .background(
                    LinearGradient(
                        colors: [
                            Color(red: 29.0 / 255.0, green: 78.0 / 255.0, blue: 216.0 / 255.0),
                            Color(red: 14.0 / 255.0, green: 165.0 / 255.0, blue: 165.0 / 255.0)
                        ],
                        startPoint: .topLeading,
                        endPoint: .bottomTrailing
                    )
                )
                .clipShape(RoundedRectangle(cornerRadius: 5, style: .continuous))
        } else if tier == "FREE" {
            Text("FREE")
                .font(.system(size: 9.5, weight: .bold))
                .tracking(0.4)
                .foregroundColor(.secondary)
                .padding(.horizontal, 6)
                .padding(.vertical, 2.5)
                .background(Color.secondary.opacity(0.16))
                .overlay(
                    RoundedRectangle(cornerRadius: 5, style: .continuous)
                        .stroke(Color.secondary.opacity(0.32), lineWidth: 0.5)
                )
                .clipShape(RoundedRectangle(cornerRadius: 5, style: .continuous))
        }
    }
    
    @ViewBuilder
    private func metricCell(title: String, bucket: CockpitQuotaBucket?, isArgon: Bool = false, isCarbon: Bool = false, tier: String? = nil) -> some View {
        let percent = bucket?.remainingPercent ?? 0.0
        let isDark = colorScheme == .dark
        let argonAccent = isDark
            ? Color(red: 1.0, green: 159.0 / 255.0, blue: 10.0 / 255.0)
            : Color(red: 217.0 / 255.0, green: 119.0 / 255.0, blue: 6.0 / 255.0)
        let carbonAccent = isDark
            ? Color(red: 197.0 / 255.0, green: 138.0 / 255.0, blue: 249.0 / 255.0)
            : Color(red: 124.0 / 255.0, green: 58.0 / 255.0, blue: 237.0 / 255.0)
        let carbonAuroraColors = carbonAuroraGradientColors(isDark: isDark)
        let carbonTitleColors: [Color] = isDark
            ? [
                Color(red: 138.0 / 255.0, green: 180.0 / 255.0, blue: 248.0 / 255.0),
                Color(red: 197.0 / 255.0, green: 138.0 / 255.0, blue: 249.0 / 255.0),
                Color(red: 1.0, green: 139.0 / 255.0, blue: 203.0 / 255.0)
            ]
            : [
                Color(red: 37.0 / 255.0, green: 99.0 / 255.0, blue: 235.0 / 255.0),
                Color(red: 124.0 / 255.0, green: 58.0 / 255.0, blue: 237.0 / 255.0),
                Color(red: 219.0 / 255.0, green: 39.0 / 255.0, blue: 119.0 / 255.0)
            ]
        let isSpecial = isArgon || isCarbon
        let color: Color = {
            if isCarbon {
                return percent < 20.0 ? Color.red : carbonAccent
            } else if isArgon {
                return percent < 20.0 ? Color.red : argonAccent
            } else {
                return colorForPercent(percent)
            }
        }()
        
        let isPaceSupported = title.localizedCaseInsensitiveContains("Weekly") ||
            title.localizedCaseInsensitiveContains("5h") ||
            isArgon ||
            isCarbon ||
            title.localizedCaseInsensitiveContains("Argon") ||
            title.localizedCaseInsensitiveContains("Carbon")
        let paceInfo = isPaceSupported ? bucket?.calculatePaceForTitle(title, tier: tier) : nil
        let showCursor = isPaceSupported && AppSettings.shared.weeklyPaceCursorEnabled && paceInfo != nil
        
        VStack(alignment: .leading, spacing: isSpecial ? 5 : 4) {
            HStack(spacing: 5) {
                if isCarbon {
                    Text(title)
                        .font(.system(size: 11.5, weight: .semibold))
                        .foregroundStyle(
                            LinearGradient(
                                colors: carbonTitleColors,
                                startPoint: .leading,
                                endPoint: .trailing
                            )
                        )
                        .lineLimit(1)
                } else {
                    Text(title)
                        .font(.system(size: isSpecial ? 11.5 : 11, weight: isSpecial ? .semibold : .medium))
                        .foregroundColor(isArgon ? argonAccent : .secondary)
                        .lineLimit(1)
                }
                
                Spacer()
                Text(String(format: "%.1f%%", percent))
                    .font(.system(size: 12, weight: .bold, design: .monospaced))
                    .foregroundColor(color)
            }
            
            // Mini progress bar with weekly pace cursor
            GeometryReader { geo in
                ZStack(alignment: .leading) {
                    Capsule()
                        .fill(
                            isCarbon
                                ? Color(red: 155.0 / 255.0, green: 114.0 / 255.0, blue: 207.0 / 255.0).opacity(isDark ? 0.18 : 0.14)
                                : (isArgon
                                    ? Color(red: 1.0, green: 159.0 / 255.0, blue: 10.0 / 255.0).opacity(isDark ? 0.16 : 0.18)
                                    : Color.primary.opacity(0.08))
                        )
                    if isCarbon && percent >= 20.0 {
                        Capsule()
                            .fill(
                                LinearGradient(
                                    colors: carbonAuroraColors,
                                    startPoint: .leading,
                                    endPoint: .trailing
                                )
                            )
                            .frame(width: max(0, min(geo.size.width, geo.size.width * CGFloat(percent / 100.0))))
                            .shadow(color: Color(red: 155.0 / 255.0, green: 114.0 / 255.0, blue: 207.0 / 255.0).opacity(isDark ? 0.42 : 0.30), radius: 3, x: 0, y: 0)
                    } else if isArgon && percent >= 20.0 {
                        Capsule()
                            .fill(
                                LinearGradient(
                                    colors: [
                                        Color(red: 1.0, green: 159.0 / 255.0, blue: 10.0 / 255.0),
                                        Color(red: 1.0, green: 214.0 / 255.0, blue: 10.0 / 255.0)
                                    ],
                                    startPoint: .leading,
                                    endPoint: .trailing
                                )
                            )
                            .frame(width: max(0, min(geo.size.width, geo.size.width * CGFloat(percent / 100.0))))
                            .shadow(color: Color(red: 1.0, green: 159.0 / 255.0, blue: 10.0 / 255.0).opacity(0.35), radius: 3, x: 0, y: 0)
                    } else {
                        Capsule()
                            .fill(color)
                            .frame(width: max(0, min(geo.size.width, geo.size.width * CGFloat(percent / 100.0))))
                    }
                    
                    if showCursor, let info = paceInfo {
                        let expectedFrac = CGFloat(info.expectedRemainingPercent / 100.0)
                        let needleOffset = max(0, min(geo.size.width - 1.5, geo.size.width * expectedFrac - 0.75))
                        let needleColor = info.isOverPacing
                            ? Color(red: 1.0, green: 159.0 / 255.0, blue: 10.0 / 255.0)
                            : Color.white.opacity(0.92)
                        
                        RoundedRectangle(cornerRadius: 0.75, style: .continuous)
                            .fill(needleColor)
                            .frame(width: 1.5, height: 7.5)
                            .shadow(color: Color.black.opacity(0.35), radius: 1, x: 0, y: 0.5)
                            .offset(x: needleOffset, y: (5.0 - 7.5) / 2.0)
                    }
                }
            }
            .frame(height: 5)
            
            let displayText = resetCountdownDisplay(for: bucket)
            Text(displayText)
                .font(.system(size: 9.5))
                .foregroundColor(displayText == "配额充足" ? .secondary.opacity(0.6) : .secondary)
                .lineLimit(1)
                .minimumScaleFactor(0.75)
        }
        .padding(.horizontal, isSpecial ? 11 : 9)
        .padding(.vertical, 9)
        .background(
            Group {
                if isCarbon {
                    RoundedRectangle(cornerRadius: 10, style: .continuous)
                        .fill(
                            LinearGradient(
                                colors: [
                                    Color(red: 66.0 / 255.0, green: 133.0 / 255.0, blue: 244.0 / 255.0).opacity(isDark ? 0.12 : 0.08),
                                    Color(red: 155.0 / 255.0, green: 114.0 / 255.0, blue: 207.0 / 255.0).opacity(isDark ? 0.10 : 0.07),
                                    Color(red: 240.0 / 255.0, green: 98.0 / 255.0, blue: 146.0 / 255.0).opacity(isDark ? 0.08 : 0.06)
                                ],
                                startPoint: .topLeading,
                                endPoint: .bottomTrailing
                            )
                        )
                } else if isArgon {
                    RoundedRectangle(cornerRadius: 10, style: .continuous)
                        .fill(
                            LinearGradient(
                                colors: [
                                    Color(red: 1.0, green: 159.0 / 255.0, blue: 10.0 / 255.0).opacity(isDark ? 0.13 : 0.11),
                                    Color(red: 249.0 / 255.0, green: 115.0 / 255.0, blue: 22.0 / 255.0).opacity(isDark ? 0.06 : 0.05)
                                ],
                                startPoint: .topLeading,
                                endPoint: .bottomTrailing
                            )
                        )
                } else {
                    RoundedRectangle(cornerRadius: 8, style: .continuous)
                        .fill(Color(uiColor: .tertiarySystemGroupedBackground))
                }
            }
        )
        .overlay(
            Group {
                if isCarbon {
                    RoundedRectangle(cornerRadius: 10, style: .continuous)
                        .stroke(
                            LinearGradient(
                                colors: [
                                    Color(red: 66.0 / 255.0, green: 133.0 / 255.0, blue: 244.0 / 255.0).opacity(isDark ? 0.45 : 0.42),
                                    Color(red: 155.0 / 255.0, green: 114.0 / 255.0, blue: 207.0 / 255.0).opacity(isDark ? 0.42 : 0.38),
                                    Color(red: 240.0 / 255.0, green: 98.0 / 255.0, blue: 146.0 / 255.0).opacity(isDark ? 0.38 : 0.36)
                                ],
                                startPoint: .topLeading,
                                endPoint: .bottomTrailing
                            ),
                            lineWidth: 1
                        )
                } else if isArgon {
                    RoundedRectangle(cornerRadius: 10, style: .continuous)
                        .stroke(
                            LinearGradient(
                                colors: [
                                    Color(red: 1.0, green: 202.0 / 255.0, blue: 40.0 / 255.0).opacity(isDark ? 0.42 : 0.48),
                                    Color(red: 1.0, green: 159.0 / 255.0, blue: 10.0 / 255.0).opacity(isDark ? 0.22 : 0.28)
                                ],
                                startPoint: .topLeading,
                                endPoint: .bottomTrailing
                            ),
                            lineWidth: 1
                        )
                }
            }
        )
        .contentShape(Rectangle())
        .onTapGesture {
            if isPaceSupported, let info = paceInfo {
                UIImpactFeedbackGenerator(style: .light).impactOccurred()
                paceDetailItem = WeeklyPaceDetailItem(title: title, info: info)
            }
        }
    }
    
    private func resetCountdownDisplay(for bucket: CockpitQuotaBucket?) -> String {
        guard let friendly = bucket?.resetFriendly, !friendly.isEmpty else {
            return "配额充足"
        }
        if friendly == "已就绪" || friendly == "未知" || friendly == "配额充足" {
            return friendly
        }
        if let timeStr = formatEstimatedResetTime(bucket?.resetTime) {
            return "\(friendly) \(timeStr)"
        }
        return friendly
    }
    
    private func formatEstimatedResetTime(_ resetTimeStr: String?) -> String? {
        guard let resetTimeStr = resetTimeStr, !resetTimeStr.isEmpty else {
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

public struct WeeklyPaceDetailItem: Identifiable, Sendable {
    public let id = UUID()
    public let title: String
    public let info: WeeklyPaceInfo
}

private struct WeeklyPaceDetailSheet: View {
    let item: WeeklyPaceDetailItem
    @Environment(\.dismiss) private var dismiss
    
    var body: some View {
        NavigationStack {
            VStack(alignment: .leading, spacing: 16) {
                // 状态卡片
                VStack(alignment: .leading, spacing: 6) {
                    HStack(spacing: 8) {
                        Circle()
                            .fill(item.info.isOverPacing ? Color.orange : Color.green)
                            .frame(width: 8, height: 8)
                        Text(item.info.isOverPacing ? "⚠️ 消耗偏快 (超速透支)" : "✅ 节奏健康 (额度充裕)")
                            .font(.system(size: 15, weight: .bold))
                            .foregroundColor(item.info.isOverPacing ? .orange : .green)
                    }
                    Text(item.info.isOverPacing
                         ? "当前额度已消耗 \(String(format: "%.1f%%", item.info.usedPercent))，快于 \(item.info.windowLabel)时间匀速基准线 (\(String(format: "%.1f%%", item.info.expectedUsedPercent)))。若保持此速率可能在周期结束前提前耗尽。"
                         : "当前额度已消耗 \(String(format: "%.1f%%", item.info.usedPercent))，低于或符合 \(item.info.windowLabel)时间匀速基准线 (\(String(format: "%.1f%%", item.info.expectedUsedPercent)))。剩余额度充足，无需担心断粮。")
                        .font(.system(size: 13))
                        .foregroundColor(.secondary)
                        .lineSpacing(2)
                }
                .padding(14)
                .frame(maxWidth: .infinity, alignment: .leading)
                .background(
                    (item.info.isOverPacing ? Color.orange : Color.green).opacity(0.12)
                )
                .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
                .overlay(
                    RoundedRectangle(cornerRadius: 12, style: .continuous)
                        .stroke((item.info.isOverPacing ? Color.orange : Color.green).opacity(0.3), lineWidth: 1)
                )
                
                // 明细指标
                VStack(spacing: 10) {
                    detailRow(label: "当前实际剩余", value: String(format: "%.1f%%", item.info.actualRemainingPercent), isPrimary: true)
                    detailRow(label: "当前实际已用", value: String(format: "%.1f%%", item.info.usedPercent), isPrimary: true)
                    Divider()
                    detailRow(label: "\(item.info.windowLabel)匀速应剩 (游标位)", value: String(format: "%.1f%%", item.info.expectedRemainingPercent), isPrimary: false)
                    detailRow(label: "\(item.info.windowLabel)匀速应耗基准", value: String(format: "%.1f%%", item.info.expectedUsedPercent), isPrimary: false)
                    Divider()
                    detailRow(
                        label: "节奏差值 (实际 - 理论)",
                        value: (item.info.paceDeltaPercent >= 0 ? "+" : "") + String(format: "%.1f%%", item.info.paceDeltaPercent),
                        color: item.info.isOverPacing ? .orange : .green
                    )
                    detailRow(label: "重置倒计时", value: item.info.resetCountdownDisplay, isPrimary: false)
                }
                .padding(14)
                .background(Color(uiColor: .secondarySystemGroupedBackground))
                .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
                
                Spacer()
            }
            .padding(.horizontal, 20)
            .padding(.top, 16)
            .navigationTitle("\(item.title) 消耗节奏")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .confirmationAction) {
                    Button("完成") {
                        dismiss()
                    }
                    .fontWeight(.semibold)
                }
            }
        }
    }
    
    private func detailRow(label: String, value: String, isPrimary: Bool = false, color: Color? = nil) -> some View {
        HStack {
            Text(label)
                .font(.system(size: 13))
                .foregroundColor(.secondary)
            Spacer()
            Text(value)
                .font(.system(size: 13, weight: isPrimary ? .semibold : .medium))
                .foregroundColor(color ?? (isPrimary ? .primary : .secondary))
        }
    }
}

extension AccountQuotaSheet {
    @ViewBuilder
    var tokenHistorySection: some View {
        if tokenHistory == nil && isLoadingTokenHistory {
            VStack(spacing: 12) {
                ProgressView()
                Text("正在统计本地 Transcript Token 历史...")
                    .font(.system(size: 13))
                    .foregroundColor(.secondary)
            }
            .frame(maxWidth: .infinity)
            .padding(.vertical, 48)
        } else if let history = tokenHistory {
            let s = history.summary
            let matrix = history.dailyMatrix
            let activeDate = selectedHeatmapDate
                ?? matrix.last(where: { $0.total > 0 })?.date
                ?? matrix.last?.date
            let selectedRec = matrix.first(where: { $0.date == activeDate })

            VStack(spacing: 14) {
                LazyVGrid(columns: [GridItem(.flexible(), spacing: 8), GridItem(.flexible(), spacing: 8)], spacing: 8) {
                    tokenKpiCard(title: "今日消耗", value: Self.formatTokensCompact(s.todayTokens), sub: "\(s.activeDays) 活跃日")
                    tokenKpiCard(title: "近 7 日", value: Self.formatTokensCompact(s.last7dTokens), sub: "\(s.totalSteps) 轮采样")
                    tokenKpiCard(title: "累计总 Token", value: Self.formatTokensCompact(s.totalTokens), sub: "IDE \(Self.formatTokensCompact(s.desktopTokens)) · CLI \(Self.formatTokensCompact(s.cliTokens))")
                    tokenKpiCard(title: "API 折算", value: String(format: "$%.2f", s.estimatedCostUsd), sub: String(format: "≈ ¥%.1f", s.estimatedCostCny))
                }

                let totalSrc = max(Int64(1), s.desktopTokens + s.cliTokens)
                let desktopPct = Int((Double(s.desktopTokens) * 100.0) / Double(totalSrc))
                let cliPct = s.cliTokens > 0 ? (100 - desktopPct) : 0
                HStack {
                    Text("桌面端 Antigravity \(desktopPct)% · CLI \(cliPct)%")
                        .font(.system(size: 12, weight: .medium))
                        .foregroundColor(.secondary)
                    Spacer()
                    Text("近 52 周热力图")
                        .font(.system(size: 11))
                        .foregroundColor(.secondary.opacity(0.75))
                }
                .padding(.horizontal, 12)
                .padding(.vertical, 9)
                .background(Color(uiColor: .secondarySystemGroupedBackground))
                .clipShape(RoundedRectangle(cornerRadius: 10))

                tokenHeatmapCard(matrix: matrix, selectedDate: activeDate)

                if let rec = selectedRec {
                    tokenInspectorCard(record: rec)
                }

                if history.updatedAt > 0 {
                    let date = Date(timeIntervalSince1970: Double(history.updatedAt))
                    Text("本地历史扫描于 \(date.formatted(date: .omitted, time: .standard))")
                        .font(.system(size: 11))
                        .foregroundColor(.secondary.opacity(0.7))
                        .padding(.top, 2)
                }
            }
        } else {
            VStack(spacing: 12) {
                Text("暂无 Token 历史数据")
                    .font(.system(size: 14, weight: .medium))
                    .foregroundColor(.secondary)
                Button("点击加载") {
                    Task { await onLoadTokenHistory?(true) }
                }
                .buttonStyle(.bordered)
            }
            .frame(maxWidth: .infinity)
            .padding(.vertical, 44)
        }
    }

    private func tokenKpiCard(title: String, value: String, sub: String) -> some View {
        VStack(alignment: .leading, spacing: 3) {
            Text(title)
                .font(.system(size: 11))
                .foregroundColor(.secondary)
                .lineLimit(1)
            Text(value)
                .font(.system(size: 18, weight: .bold, design: .monospaced))
                .foregroundColor(.primary)
                .lineLimit(1)
            Text(sub)
                .font(.system(size: 11))
                .foregroundColor(.secondary.opacity(0.8))
                .lineLimit(1)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(.horizontal, 12)
        .padding(.vertical, 10)
        .background(Color(uiColor: .secondarySystemGroupedBackground))
        .clipShape(RoundedRectangle(cornerRadius: 12))
    }

    private func tokenHeatmapCard(matrix: [DailyTokenRecord], selectedDate: String?) -> some View {
        let maxTotal = max(Int64(1), matrix.map(\.total).max() ?? 1)
        let weeks = Self.buildHeatmapWeeks(matrix)
        return VStack(alignment: .leading, spacing: 8) {
            HStack {
                Text("每日活跃密度")
                    .font(.system(size: 12, weight: .semibold))
                Spacer()
                HStack(spacing: 3) {
                    Text("少").font(.system(size: 10)).foregroundColor(.secondary)
                    ForEach(0..<5, id: \.self) { lvl in
                        RoundedRectangle(cornerRadius: 2)
                            .fill(Self.heatColor(level: lvl))
                            .frame(width: 9, height: 9)
                    }
                    Text("多").font(.system(size: 10)).foregroundColor(.secondary)
                }
            }

            ScrollViewReader { proxy in
                ScrollView(.horizontal, showsIndicators: false) {
                    HStack(spacing: 3) {
                        ForEach(Array(weeks.enumerated()), id: \.offset) { wIdx, week in
                            VStack(spacing: 3) {
                                ForEach(0..<7, id: \.self) { dIdx in
                                    if dIdx < week.count, let rec = week[dIdx] {
                                        let ratio = Double(rec.total) / Double(maxTotal)
                                        let level: Int = {
                                            if rec.total <= 0 { return 0 }
                                            if ratio < 0.2 { return 1 }
                                            if ratio < 0.45 { return 2 }
                                            if ratio < 0.75 { return 3 }
                                            return 4
                                        }()
                                        let isSel = (rec.date == selectedDate)
                                        RoundedRectangle(cornerRadius: 2.5)
                                            .fill(Self.heatColor(level: level))
                                            .frame(width: 12, height: 12)
                                            .overlay(
                                                RoundedRectangle(cornerRadius: 2.5)
                                                    .stroke(isSel ? Color.blue : Color.clear, lineWidth: 1.5)
                                            )
                                            .onTapGesture {
                                                selectedHeatmapDate = rec.date
                                            }
                                    } else {
                                        Color.clear.frame(width: 12, height: 12)
                                    }
                                }
                            }
                            .id("week_\(wIdx)")
                        }
                    }
                    .padding(.vertical, 2)
                }
                .onAppear {
                    if !weeks.isEmpty {
                        proxy.scrollTo("week_\(weeks.count - 1)", anchor: .trailing)
                    }
                }
            }
        }
        .padding(12)
        .background(Color(uiColor: .secondarySystemGroupedBackground))
        .clipShape(RoundedRectangle(cornerRadius: 12))
    }

    private func tokenInspectorCard(record: DailyTokenRecord) -> some View {
        let topModels = record.models.sorted { $0.value > $1.value }.prefix(4)
        return VStack(alignment: .leading, spacing: 8) {
            HStack {
                Text("\(record.date) 用量明细")
                    .font(.system(size: 13, weight: .semibold))
                Spacer()
                Text("\(Self.formatTokensCompact(record.total)) · $\(String(format: "%.3f", record.costUsd))")
                    .font(.system(size: 12, weight: .semibold, design: .monospaced))
                    .foregroundColor(.blue)
            }
            Divider()
            HStack {
                inspectorCell("Input", Self.formatTokensCompact(record.input))
                Spacer()
                inspectorCell("Cache Hit", Self.formatTokensCompact(record.cacheRead))
                Spacer()
                inspectorCell("Output", Self.formatTokensCompact(record.output))
                Spacer()
                inspectorCell("Thinking", Self.formatTokensCompact(record.thinking))
            }
            if !topModels.isEmpty {
                Divider()
                VStack(spacing: 4) {
                    ForEach(Array(topModels), id: \.key) { pair in
                        HStack {
                            Text(pair.key)
                                .font(.system(size: 11))
                                .foregroundColor(.secondary)
                                .lineLimit(1)
                            Spacer()
                            Text(Self.formatTokensCompact(pair.value))
                                .font(.system(size: 11, design: .monospaced))
                        }
                    }
                }
            }
        }
        .padding(12)
        .background(Color(uiColor: .secondarySystemGroupedBackground))
        .clipShape(RoundedRectangle(cornerRadius: 12))
    }

    private func inspectorCell(_ label: String, _ value: String) -> some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(label).font(.system(size: 10)).foregroundColor(.secondary)
            Text(value).font(.system(size: 12, weight: .semibold, design: .monospaced))
        }
    }

    private static func buildHeatmapWeeks(_ matrix: [DailyTokenRecord]) -> [[DailyTokenRecord?]] {
        guard !matrix.isEmpty else { return [] }
        let df = DateFormatter()
        df.locale = Locale(identifier: "en_US_POSIX")
        df.dateFormat = "yyyy-MM-dd"
        let cal = Calendar.current
        var padStart = 0
        if let firstDate = df.date(from: matrix[0].date) {
            padStart = max(0, cal.component(.weekday, from: firstDate) - 1)
        }
        var padded: [DailyTokenRecord?] = Array(repeating: nil, count: padStart)
        padded.append(contentsOf: matrix.map { Optional.some($0) })
        var weeks: [[DailyTokenRecord?]] = []
        var idx = 0
        while idx < padded.count {
            let end = min(idx + 7, padded.count)
            weeks.append(Array(padded[idx..<end]))
            idx = end
        }
        return weeks
    }

    private static func heatColor(level: Int) -> Color {
        switch level {
        case 0: return Color.secondary.opacity(0.14)
        case 1: return Color(red: 0.61, green: 0.91, blue: 0.66).opacity(0.7)
        case 2: return Color(red: 0.25, green: 0.77, blue: 0.39).opacity(0.85)
        case 3: return Color(red: 0.19, green: 0.63, blue: 0.31)
        default: return Color(red: 0.13, green: 0.43, blue: 0.22)
        }
    }

    public static func formatTokensCompact(_ tokens: Int64) -> String {
        if tokens >= 1_000_000_000 { return String(format: "%.2fB", Double(tokens) / 1_000_000_000.0) }
        if tokens >= 1_000_000 { return String(format: "%.2fM", Double(tokens) / 1_000_000.0) }
        if tokens >= 1_000 { return String(format: "%.1fK", Double(tokens) / 1_000.0) }
        return "\(tokens)"
    }
}
