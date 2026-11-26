import SwiftUI

public struct AccountQuotaSheet: View {
    @Environment(\.dismiss) private var dismiss
    @Environment(\.colorScheme) private var colorScheme
    @Environment(\.displayScale) private var displayScale
    @Environment(\.scenePhase) private var scenePhase
    @AppStorage("cockpit_email_masked") private var isMasked: Bool = false
    @State private var switchingAccountId: String? = nil
    @State private var pendingSwitch: CockpitAccountQuota? = nil
    @State private var showSwitchConfirm: Bool = false
    @State private var isRefreshing: Bool = false
    @State private var isSavingTokenHistory: Bool = false
    @State private var isSharingTokenHistory: Bool = false
    @State private var errorMessage: String? = nil
    @State private var alertTitle: String = String(localized: "操作失败")
    @State private var paceDetailItem: WeeklyPaceDetailItem? = nil
    @State private var selectedTab: Int = 0
    @State private var selectedHeatmapDate: String? = nil
    @State private var selectedChartType: TokenHistoryChartType
    @State private var hasInitializedChartType: Bool
    @State private var suppressNextChartPersist: Bool = false
    @State private var showTokenHistoryShareSheet: Bool = false
    @State private var showAllModelsSheet: Bool = false
    @State private var expandedInspectorModel: String? = nil
    @State private var isCockpitGuideExpanded: Bool = false
    @State private var copiedFeedbackText: String? = nil
    @State private var autoSwitchResponse: AutoSwitchResponse? = nil
    @State private var isAutoSwitchExpanded: Bool = false
    @State private var isSavingAutoSwitch: Bool = false
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
        if let history = tokenHistory {
            let resolved = TokenHistoryChartType.resolveDefault(
                savedRawValue: AppSettings.shared.tokenHistoryChartType,
                activeDays: history.summary.activeDays
            )
            self._selectedChartType = State(initialValue: resolved)
            self._hasInitializedChartType = State(initialValue: true)
        } else {
            let fallback = TokenHistoryChartType(rawValue: AppSettings.shared.tokenHistoryChartType) ?? .dailyBars
            self._selectedChartType = State(initialValue: fallback)
            self._hasInitializedChartType = State(initialValue: false)
        }
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
            HStack(spacing: 6) {
                Text("备用账号")
                    .font(.system(size: 13, weight: .semibold))
                    .foregroundColor(.secondary)
                Text("多账号联动")
                    .font(.system(size: 11))
                    .foregroundColor(.secondary.opacity(0.8))
            }
            .padding(.horizontal, 4)
            
            VStack(alignment: .leading, spacing: 12) {
                HStack {
                    Label("未检测到 Cockpit Tools", systemImage: "info.circle")
                        .font(.system(size: 14, weight: .semibold))
                    Spacer()
                    Text("可选功能")
                        .font(.system(size: 10, weight: .medium))
                        .foregroundColor(.secondary)
                        .padding(.horizontal, 6)
                        .padding(.vertical, 2)
                        .background(Color(uiColor: .tertiarySystemFill))
                        .clipShape(RoundedRectangle(cornerRadius: 4))
                }
                
                VStack(alignment: .leading, spacing: 4) {
                    Text("• 单账号用户无需配置，当前额度已正常监控。")
                        .font(.system(size: 12))
                        .foregroundColor(.secondary)
                    Text("• 若需管理多个 Antigravity 账号并在手机上一键切号，可配合开源工具 Cockpit Tools 与网关联动。")
                        .font(.system(size: 12))
                        .foregroundColor(.secondary)
                }
                
                HStack(spacing: 12) {
                    Button(action: {
                        UIPasteboard.general.string = "https://github.com/jlcodes99/cockpit-tools"
                        let generator = UINotificationFeedbackGenerator()
                        generator.notificationOccurred(.success)
                        copiedFeedbackText = "github"
                        DispatchQueue.main.asyncAfter(deadline: .now() + 2.0) {
                            if copiedFeedbackText == "github" {
                                copiedFeedbackText = nil
                            }
                        }
                    }) {
                        Label(copiedFeedbackText == "github" ? String(localized: "已复制链接") : String(localized: "复制 GitHub 链接"),
                              systemImage: copiedFeedbackText == "github" ? "checkmark" : "link")
                            .font(.system(size: 12, weight: .medium))
                    }
                    
                    Button(action: {
                        UIPasteboard.general.string = "mgy cockpit"
                        let generator = UINotificationFeedbackGenerator()
                        generator.notificationOccurred(.success)
                        copiedFeedbackText = "cmd"
                        DispatchQueue.main.asyncAfter(deadline: .now() + 2.0) {
                            if copiedFeedbackText == "cmd" {
                                copiedFeedbackText = nil
                            }
                        }
                    }) {
                        Label(copiedFeedbackText == "cmd" ? String(localized: "已复制命令") : String(localized: "复制 mgy cockpit 命令"),
                              systemImage: copiedFeedbackText == "cmd" ? "checkmark" : "terminal")
                            .font(.system(size: 12, weight: .medium))
                    }
                }
                
                Button(action: {
                    withAnimation(.easeInOut(duration: 0.2)) {
                        isCockpitGuideExpanded.toggle()
                    }
                }) {
                    HStack(spacing: 4) {
                        Text(isCockpitGuideExpanded ? String(localized: "收起配置步骤") : String(localized: "查看详细配置步骤"))
                            .font(.system(size: 11, weight: .medium))
                        Image(systemName: isCockpitGuideExpanded ? "chevron.up" : "chevron.down")
                            .font(.system(size: 10, weight: .semibold))
                    }
                    .foregroundColor(.secondary)
                }
                .buttonStyle(.plain)
                
                if isCockpitGuideExpanded {
                    VStack(alignment: .leading, spacing: 6) {
                        Text("1. 下载安装：从 GitHub Releases 下载 Cockpit Tools 桌面端，在应用中添加/登录备用账号；")
                        Text("2. 网关联动：在网关电脑终端运行 mgy cockpit，按回车一键开启 HTTP 报表服务与 Token 并重启；")
                        Text("3. 手机刷新：在手机端下拉刷新，即可查看到所有备用账号的实时额度并自由切换。")
                    }
                    .font(.system(size: 11))
                    .foregroundColor(.secondary)
                    .lineSpacing(2)
                    .padding(10)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .background(Color(uiColor: .tertiarySystemGroupedBackground))
                    .clipShape(RoundedRectangle(cornerRadius: 8))
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

                                        let isAutoTightest = AppSettings.shared.selectedQuotaBucket == .auto
                                        Button(action: {
                                            UIImpactFeedbackGenerator(style: .light).impactOccurred()
                                            withAnimation(.easeInOut(duration: 0.18)) {
                                                AppSettings.shared.selectedQuotaBucket = isAutoTightest ? .gemini5h : .auto
                                            }
                                        }) {
                                            HStack(spacing: 3) {
                                                Image(systemName: isAutoTightest ? "sparkles" : "scope")
                                                    .font(.system(size: 9.5, weight: .semibold))
                                                Text(isAutoTightest ? "顶栏：自动最紧" : "顶栏：自动最紧")
                                                    .font(.system(size: 10.5, weight: .semibold))
                                            }
                                            .foregroundColor(isAutoTightest ? .blue : .secondary)
                                            .padding(.horizontal, 7)
                                            .padding(.vertical, 3)
                                            .background(isAutoTightest ? Color.blue.opacity(0.14) : Color(uiColor: .tertiarySystemFill))
                                            .clipShape(Capsule())
                                        }
                                        .buttonStyle(.plain)
                                        
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

                                    autoSwitchSectionCard
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
                    if selectedTab == 0 {
                        Button(action: {
                            isMasked.toggle()
                            AppSettings.shared.syncWidgetConfiguration()
                        }) {
                            Image(systemName: isMasked ? "eye.slash.fill" : "eye.slash")
                                .font(.system(size: 15, weight: .medium))
                                .foregroundColor(isMasked ? .blue : .secondary)
                        }
                    } else {
                        Button(action: {
                            Task {
                                await saveTokenHistory()
                            }
                        }) {
                            if isSavingTokenHistory {
                                ProgressView()
                                    .controlSize(.small)
                            } else {
                                Image(systemName: "square.and.arrow.down")
                                    .font(.system(size: 15, weight: .semibold))
                            }
                        }
                        .accessibilityLabel("保存到相册")
                        .disabled(tokenHistory == nil || isLoadingTokenHistory || isSavingTokenHistory || isSharingTokenHistory)
                    }
                }
                ToolbarItem(placement: .topBarTrailing) {
                    if selectedTab == 0 {
                        Button(action: {
                            Task {
                                await performRefresh()
                            }
                        }) {
                            if #available(iOS 18.0, *) {
                                Image(systemName: "arrow.clockwise")
                                    .font(.system(size: 15, weight: .semibold))
                                    .symbolEffect(.rotate.clockwise, isActive: isRefreshing)
                            } else {
                                Image(systemName: "arrow.clockwise")
                                    .font(.system(size: 15, weight: .semibold))
                                    .rotationEffect(.degrees(isRefreshing ? 360 : 0))
                                    .animation(
                                        isRefreshing
                                            ? .linear(duration: 1).repeatForever(autoreverses: false)
                                            : .default,
                                        value: isRefreshing
                                    )
                            }
                        }
                        .disabled(isRefreshing || switchingAccountId != nil)
                    } else {
                        Button(action: {
                            shareTokenHistory()
                        }) {
                            if isLoadingTokenHistory || isSharingTokenHistory {
                                ProgressView()
                                    .controlSize(.small)
                            } else {
                                Image(systemName: "square.and.arrow.up")
                                    .font(.system(size: 15, weight: .semibold))
                            }
                        }
                        .accessibilityLabel("分享")
                        .disabled(tokenHistory == nil || isLoadingTokenHistory || isSavingTokenHistory || isSharingTokenHistory)
                    }
                }
            }
            .presentationDragIndicator(.visible)
            .task {
                await loadAutoSwitchConfig()
                guard let onAppearFetch = onAppearFetch else { return }
                await onAppearFetch()
                while !Task.isCancelled {
                    try? await Task.sleep(nanoseconds: 10_000_000_000)
                    guard !Task.isCancelled else { break }
                    await onAppearFetch()
                }
            }
            .task(id: selectedTab) {
                guard selectedTab == 1 else { return }
                await onLoadTokenHistory?(false)
                while !Task.isCancelled {
                    try? await Task.sleep(nanoseconds: 60_000_000_000)
                    guard !Task.isCancelled else { break }
                    if scenePhase == .active {
                        await onLoadTokenHistory?(false)
                    }
                }
            }
            .onChange(of: scenePhase) { _, newPhase in
                if newPhase == .active && selectedTab == 1 {
                    Task { await onLoadTokenHistory?(false) }
                }
            }
            .onReceive(NotificationCenter.default.publisher(for: UIApplication.didBecomeActiveNotification)) { _ in
                if selectedTab == 1 {
                    Task { await onLoadTokenHistory?(false) }
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
                Button("切换并重启桌面端") {
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
                    Text("切换到 \(acc.email) 将关闭并重启电脑上的桌面客户端。")
                } else {
                    Text("切换账号将关闭并重启电脑上的桌面客户端。")
                }
            }
            .sheet(item: $paceDetailItem) { item in
                WeeklyPaceDetailSheet(item: item)
                    .presentationDetents([.fraction(0.48), .medium])
                    .presentationDragIndicator(.visible)
            }
            .sheet(isPresented: $showTokenHistoryShareSheet) {
                if let history = tokenHistory {
                    let activeDate = selectedHeatmapDate
                        ?? history.dailyMatrix.last(where: { $0.total > 0 })?.date
                        ?? history.dailyMatrix.last?.date
                    let selectedRec = history.dailyMatrix.first(where: { $0.date == activeDate })
                    TokenHistoryShareCardSheet(
                        history: history,
                        selectedRecord: selectedRec,
                        chartType: selectedChartType
                    )
                }
            }
            .sheet(isPresented: $showAllModelsSheet) {
                if let history = tokenHistory {
                    let allModels = history.resolvedModelSummary
                    if !allModels.isEmpty {
                        ModelUsageBreakdownSheet(models: allModels)
                            .presentationDetents([.large])
                            .presentationDragIndicator(.visible)
                    }
                }
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
            alertTitle = String(localized: "刷新配额失败")
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
            alertTitle = String(localized: "切换账号失败")
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
                    .fixedSize(horizontal: true, vertical: false)
                    .layoutPriority(1)
                
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
                                .lineLimit(1)
                                .fixedSize(horizontal: true, vertical: false)
                                .foregroundColor(.white)
                                .padding(.horizontal, 10)
                                .padding(.vertical, 4)
                                .background(Color.blue)
                                .clipShape(Capsule())
                        }
                    }
                    .buttonStyle(.plain)
                    .layoutPriority(1)
                    .disabled(switchingAccountId != nil)
                }
            }
            
            Divider()
                .opacity(0.6)
            
            let isFree = acc.isFreeTier
            let resolvedSelectedKind = isCurrent
                ? acc.resolvedQuotaDisplay(preferred: AppSettings.shared.selectedQuotaBucket)?.kind
                : nil
            
            VStack(spacing: 10) {
                if isFree {
                    // Free 账号仅展示两个 weekly 数值：左 Gemini · Weekly，右 Claude · Weekly
                    LazyVGrid(columns: [
                        GridItem(.flexible(), spacing: 10),
                        GridItem(.flexible(), spacing: 10)
                    ], spacing: 10) {
                        metricCell(
                            title: "Gemini · Weekly",
                            bucket: acc.geminiWeekly,
                            tier: acc.subscriptionTier,
                            kind: .geminiWeekly,
                            isCurrentAccount: isCurrent,
                            isSelected: isCurrent && resolvedSelectedKind == .geminiWeekly
                        )
                        metricCell(
                            title: "Claude · Weekly",
                            bucket: acc.claudeWeekly,
                            tier: acc.subscriptionTier,
                            kind: .claudeWeekly,
                            isCurrentAccount: isCurrent,
                            isSelected: isCurrent && resolvedSelectedKind == .claudeWeekly
                        )
                    }
                } else {
                    // 4 Metrics Grid: 5h in left, Weekly in right, Gemini on top, Claude on bottom
                    LazyVGrid(columns: [
                        GridItem(.flexible(), spacing: 10),
                        GridItem(.flexible(), spacing: 10)
                    ], spacing: 10) {
                        // Row 1: Left Gemini · 5h, Right Gemini · Weekly
                        metricCell(
                            title: "Gemini · 5h",
                            bucket: acc.gemini5h,
                            tier: acc.subscriptionTier,
                            kind: .gemini5h,
                            isCurrentAccount: isCurrent,
                            isSelected: isCurrent && resolvedSelectedKind == .gemini5h
                        )
                        metricCell(
                            title: "Gemini · Weekly",
                            bucket: acc.geminiWeekly,
                            tier: acc.subscriptionTier,
                            kind: .geminiWeekly,
                            isCurrentAccount: isCurrent,
                            isSelected: isCurrent && resolvedSelectedKind == .geminiWeekly
                        )
                        
                        // Row 2: Left Claude · 5h, Right Claude · Weekly
                        metricCell(
                            title: "Claude · 5h",
                            bucket: acc.claude5h,
                            tier: acc.subscriptionTier,
                            kind: .claude5h,
                            isCurrentAccount: isCurrent,
                            isSelected: isCurrent && resolvedSelectedKind == .claude5h
                        )
                        metricCell(
                            title: "Claude · Weekly",
                            bucket: acc.claudeWeekly,
                            tier: acc.subscriptionTier,
                            kind: .claudeWeekly,
                            isCurrentAccount: isCurrent,
                            isSelected: isCurrent && resolvedSelectedKind == .claudeWeekly
                        )
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
                .shadow(color: Color(red: 124.0 / 255.0, green: 45.0 / 255.0, blue: 18.0 / 255.0).opacity(0.35), radius: 1, x: 0, y: 0.8)
                .padding(.horizontal, 6)
                .padding(.vertical, 2.5)
                .background(
                    LinearGradient(
                        colors: [
                            Color(red: 217.0 / 255.0, green: 119.0 / 255.0, blue: 6.0 / 255.0),
                            Color(red: 245.0 / 255.0, green: 158.0 / 255.0, blue: 11.0 / 255.0),
                            Color(red: 234.0 / 255.0, green: 88.0 / 255.0, blue: 12.0 / 255.0)
                        ],
                        startPoint: .topLeading,
                        endPoint: .bottomTrailing
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
    private func metricCell(
        title: String,
        bucket: CockpitQuotaBucket?,
        isArgon: Bool = false,
        isCarbon: Bool = false,
        tier: String? = nil,
        kind: QuotaBucketKind? = nil,
        isCurrentAccount: Bool = false,
        isSelected: Bool = false
    ) -> some View {
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
        
        let displayTitle = Self.localizedMetricTitle(title)
        VStack(alignment: .leading, spacing: isSpecial ? 5 : 4) {
            HStack(spacing: 5) {
                if isCarbon {
                    Text(displayTitle)
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
                    Text(displayTitle)
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
                .foregroundColor(displayText == String(localized: "配额充足") ? .secondary.opacity(0.6) : .secondary)
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
                } else if isSelected {
                    RoundedRectangle(cornerRadius: 8, style: .continuous)
                        .stroke(Color.blue.opacity(isDark ? 0.85 : 0.75), lineWidth: 1.5)
                }
            }
        )
        .contentShape(Rectangle())
        .onTapGesture {
            if isCurrentAccount, let kind, !isSelected {
                UIImpactFeedbackGenerator(style: .light).impactOccurred()
                withAnimation(.easeInOut(duration: 0.18)) {
                    AppSettings.shared.selectedQuotaBucket = kind
                }
                return
            }
            if isPaceSupported, let info = paceInfo {
                UIImpactFeedbackGenerator(style: .light).impactOccurred()
                paceDetailItem = WeeklyPaceDetailItem(title: displayTitle, info: info, bucket: bucket)
            }
        }
    }
    
    static func localizedMetricTitle(_ raw: String) -> String {
        switch raw {
        case "Gemini · Weekly": return String(localized: "Gemini · Weekly")
        case "Claude · Weekly": return String(localized: "Claude · Weekly")
        case "Gemini · 5h": return String(localized: "Gemini · 5h")
        case "Claude · 5h": return String(localized: "Claude · 5h")
        default: return raw
        }
    }

    private static let usdFormatter: NumberFormatter = {
        let f = NumberFormatter()
        f.numberStyle = .decimal
        f.locale = Locale(identifier: "en_US_POSIX")
        f.usesGroupingSeparator = true
        f.minimumFractionDigits = 2
        f.maximumFractionDigits = 2
        return f
    }()

    static func formatUSD(_ amount: Double) -> String {
        let formatted = usdFormatter.string(from: NSNumber(value: amount)) ?? String(format: "%.2f", amount)
        return "$\(formatted)"
    }

    static func formatSecondaryCost(cny: Double) -> String {
        let lang = (Bundle.main.preferredLocalizations.first ?? Locale.preferredLanguages.first ?? "en").lowercased()
        if lang.hasPrefix("zh") {
            let formatted = usdFormatter.string(from: NSNumber(value: (cny * 10).rounded() / 10))
            if let formatted, formatted.hasSuffix("0") {
                return "≈ ¥\(String(formatted.dropLast()))"
            }
            return String(format: "≈ ¥%.1f", cny)
        }
        return String(localized: "API 估算 (USD)")
    }

    private func resetCountdownDisplay(for bucket: CockpitQuotaBucket?) -> String {
        guard let friendly = bucket?.resetFriendly, !friendly.isEmpty else {
            return String(localized: "配额充足")
        }
        if friendly == "已就绪" {
            return String(localized: "已就绪")
        }
        if friendly == "未知" {
            return String(localized: "未知")
        }
        if friendly == "配额充足" {
            return String(localized: "配额充足")
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

    // MARK: - Threshold Auto-Switch (Collapsed by default, shown only when backup accounts exist)

    @ViewBuilder
    private var autoSwitchSectionCard: some View {
        let cfg = autoSwitchResponse?.config
        let st = autoSwitchResponse?.status
        let isEnabled = cfg?.enabled == true
        let statusLabel: String = {
            switch st?.state ?? "disabled" {
            case "watching": return String(localized: "监控中")
            case "pending_debounce": return String(localized: "阈值确认中")
            case "waiting_idle": return String(localized: "等待生成结束")
            case "cooldown": return String(localized: "冷却中")
            case "no_candidates": return String(localized: "无充足备用号")
            case "switched": return String(localized: "已切号")
            case "error": return String(localized: "切号失败")
            default: return String(localized: "未开启")
            }
        }()

        DisclosureGroup(isExpanded: $isAutoSwitchExpanded) {
            VStack(alignment: .leading, spacing: 12) {
                Toggle(isOn: Binding(
                    get: { cfg?.enabled ?? false },
                    set: { newVal in
                        UIImpactFeedbackGenerator(style: .light).impactOccurred()
                        Task { await updateAutoSwitch(["enabled": newVal]) }
                    }
                )) {
                    VStack(alignment: .leading, spacing: 2) {
                        Text("额度低于阈值自动切号")
                            .font(.system(size: 13, weight: .medium))
                        Text("默认等当前生成空闲后再切换，避免打断运行中的任务")
                            .font(.system(size: 11))
                            .foregroundColor(.secondary)
                    }
                }
                .disabled(isSavingAutoSwitch || AppSettings.shared.isDemoMode)

                if let cfg {
                    Divider()
                    HStack {
                        Text("监控池窗")
                            .font(.system(size: 12.5))
                            .foregroundColor(.secondary)
                        Spacer()
                        Picker("监控池窗", selection: Binding(
                            get: { cfg.targetBucket },
                            set: { val in Task { await updateAutoSwitch(["target_bucket": val]) } }
                        )) {
                            Text("Gemini · 5h").tag("gemini_5h")
                            Text("Gemini · Weekly").tag("gemini_weekly")
                            Text("Claude · 5h").tag("claude_5h")
                            Text("Claude · Weekly").tag("claude_weekly")
                        }
                        .pickerStyle(.menu)
                        .disabled(isSavingAutoSwitch || AppSettings.shared.isDemoMode)
                    }

                    HStack {
                        Text("触发阈值")
                            .font(.system(size: 12.5))
                            .foregroundColor(.secondary)
                        Spacer()
                        Text("≤ \(Int(cfg.thresholdPercent.rounded()))%")
                            .font(.system(size: 12.5, weight: .semibold, design: .monospaced))
                            .monospacedDigit()
                        Stepper("", value: Binding(
                            get: { Int(cfg.thresholdPercent.rounded()) },
                            set: { val in Task { await updateAutoSwitch(["threshold_percent": Double(val)]) } }
                        ), in: 5...30, step: 5)
                        .labelsHidden()
                        .disabled(isSavingAutoSwitch || AppSettings.shared.isDemoMode)
                    }

                    Toggle(isOn: Binding(
                        get: { cfg.waitUntilIdle },
                        set: { val in Task { await updateAutoSwitch(["wait_until_idle": val]) } }
                    )) {
                        Text("等本轮生成空闲再切")
                            .font(.system(size: 12.5))
                    }
                    .disabled(isSavingAutoSwitch || AppSettings.shared.isDemoMode)

                    if let toEmail = st?.lastToEmail, !toEmail.isEmpty, let ts = st?.lastSwitchAt, ts > 0 {
                        let d = Date(timeIntervalSince1970: Double(ts) / 1000.0)
                        Text("最近切号：\(d.formatted(date: .omitted, time: .shortened)) → \(isMasked ? Self.maskEmailForDisplay(toEmail) : toEmail)")
                            .font(.system(size: 11))
                            .foregroundColor(.secondary)
                    }
                }
            }
            .padding(.top, 10)
        } label: {
            HStack(spacing: 8) {
                Image(systemName: "arrow.triangle.2.circlepath")
                    .font(.system(size: 13, weight: .semibold))
                    .foregroundColor(isEnabled ? .blue : .secondary)
                Text("阈值自动切号")
                    .font(.system(size: 13.5, weight: .semibold))
                    .foregroundColor(.primary)
                Spacer()
                Text(statusLabel)
                    .font(.system(size: 11, weight: .medium))
                    .foregroundColor(isEnabled ? .blue : .secondary)
                    .padding(.horizontal, 7)
                    .padding(.vertical, 2)
                    .background((isEnabled ? Color.blue : Color.secondary).opacity(0.12))
                    .clipShape(Capsule())
            }
        }
        .padding(14)
        .background(Color(uiColor: .secondarySystemGroupedBackground))
        .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
    }

    private static func maskEmailForDisplay(_ email: String) -> String {
        let parts = email.split(separator: "@", maxSplits: 1)
        guard parts.count == 2 else { return email }
        let local = String(parts[0])
        let prefix = local.count <= 2 ? String(local.prefix(1)) : String(local.prefix(2))
        return "\(prefix)***@\(parts[1])"
    }

    @MainActor
    private func loadAutoSwitchConfig() async {
        guard !AppSettings.shared.isDemoMode, let baseURL = AppSettings.shared.serverURL else { return }
        if let resp = try? await APIClient.shared.fetchAutoSwitchConfig(baseURL: baseURL) {
            autoSwitchResponse = resp
        }
    }

    @MainActor
    private func updateAutoSwitch(_ patch: [String: Any]) async {
        guard !AppSettings.shared.isDemoMode, let baseURL = AppSettings.shared.serverURL else { return }
        isSavingAutoSwitch = true
        defer { isSavingAutoSwitch = false }
        if let resp = try? await APIClient.shared.updateAutoSwitchConfig(baseURL: baseURL, body: patch) {
            autoSwitchResponse = resp
        }
    }
}

public struct WeeklyPaceDetailItem: Identifiable, Sendable {
    public let id = UUID()
    public let title: String
    public let info: WeeklyPaceInfo
    public let bucket: CockpitQuotaBucket?

    public init(title: String, info: WeeklyPaceInfo, bucket: CockpitQuotaBucket? = nil) {
        self.title = title
        self.info = info
        self.bucket = bucket
    }
}

private struct WeeklyPaceDetailSheet: View {
    let item: WeeklyPaceDetailItem
    
    var body: some View {
        NavigationStack {
            VStack(alignment: .leading, spacing: 16) {
                // 状态卡片
                VStack(alignment: .leading, spacing: 6) {
                    Text(item.info.isOverPacing ? String(localized: "⚠️ 消耗偏快 (超速透支)") : String(localized: "✅ 节奏健康 (额度充裕)"))
                        .font(.system(size: 15, weight: .bold))
                        .foregroundColor(item.info.isOverPacing ? .orange : .green)
                        .fixedSize(horizontal: false, vertical: true)
                    Text(item.info.isOverPacing
                         ? String(localized: "当前额度已消耗 \(String(format: "%.1f%%", item.info.usedPercent))，快于 \(item.info.windowLabel)时间匀速基准线 (\(String(format: "%.1f%%", item.info.expectedUsedPercent)))。若保持此速率可能在周期结束前提前耗尽。")
                         : String(localized: "当前额度已消耗 \(String(format: "%.1f%%", item.info.usedPercent))，低于或符合 \(item.info.windowLabel)时间匀速基准线 (\(String(format: "%.1f%%", item.info.expectedUsedPercent)))。剩余额度充足，无需担心断粮。"))
                        .font(.system(size: 13))
                        .foregroundColor(.secondary)
                        .lineSpacing(2)
                        .fixedSize(horizontal: false, vertical: true)
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
                    detailRow(label: String(localized: "当前实际剩余"), value: String(format: "%.1f%%", item.info.actualRemainingPercent), isPrimary: true)
                    detailRow(label: String(localized: "当前实际已用"), value: String(format: "%.1f%%", item.info.usedPercent), isPrimary: true)
                    if let b = item.bucket, (b.estCycleUsedTokens ?? 0) > 0 || b.estReliable == true {
                        Divider()
                        if let used = b.estCycleUsedTokens, used > 0 {
                            detailRow(
                                label: String(localized: "本周期观测已耗"),
                                value: "~\(AccountQuotaSheet.formatTokensCompact(used))",
                                isPrimary: false
                            )
                        }
                        if b.estReliable == true, let rem = b.estRemainingTokens, rem > 0 {
                            detailRow(
                                label: String(localized: "本周期估算约剩"),
                                value: "~\(AccountQuotaSheet.formatTokensCompact(rem))",
                                isPrimary: true,
                                color: .blue
                            )
                        }
                    }
                    Divider()
                    detailRow(label: String(localized: "\(item.info.windowLabel)匀速应剩 (游标位)"), value: String(format: "%.1f%%", item.info.expectedRemainingPercent), isPrimary: false)
                    detailRow(label: String(localized: "\(item.info.windowLabel)匀速应耗基准"), value: String(format: "%.1f%%", item.info.expectedUsedPercent), isPrimary: false)
                    Divider()
                    detailRow(
                        label: String(localized: "节奏差值 (实际 - 理论)"),
                        value: (item.info.paceDeltaPercent >= 0 ? "+" : "") + String(format: "%.1f%%", item.info.paceDeltaPercent),
                        color: item.info.isOverPacing ? .orange : .green
                    )
                    detailRow(label: String(localized: "重置倒计时"), value: item.info.resetCountdownDisplay, isPrimary: false)
                }
                .padding(14)
                .background(Color(uiColor: .secondarySystemGroupedBackground))
                .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
                
                Spacer()
            }
            .padding(.horizontal, 20)
            .padding(.top, 16)
            .navigationTitle(String(localized: "\(item.title) 消耗节奏"))
            .navigationBarTitleDisplayMode(.inline)
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
                    tokenKpiCard(title: String(localized: "今日消耗"), value: Self.formatTokensCompact(s.todayTokens), sub: String(localized: "\(s.activeDays) 活跃日"))
                    tokenKpiCard(title: String(localized: "近 7 日"), value: Self.formatTokensCompact(s.last7dTokens), sub: String(localized: "\(s.totalSteps) 轮采样"))
                    tokenKpiCard(title: String(localized: "累计总 Token"), value: Self.formatTokensCompact(s.totalTokens), sub: "IDE \(Self.formatTokensCompact(s.desktopTokens)) · CLI \(Self.formatTokensCompact(s.cliTokens))")
                    let costSubtitle: String = {
                        if let roi = s.monthlyRoiMultiple, roi > 0,
                           let tier = s.planTier, !tier.isEmpty,
                           let monthUsd = s.thisMonthCostUsd, monthUsd > 0 {
                            return String(localized: "本月 \(Self.formatUSD(monthUsd)) · \(tier) \(String(format: "%.1fx", roi))")
                        }
                        return Self.formatSecondaryCost(cny: s.estimatedCostCny)
                    }()
                    tokenKpiCard(title: String(localized: "API 折算"), value: Self.formatUSD(s.estimatedCostUsd), sub: costSubtitle)
                }

                let totalSrc = max(Int64(1), s.desktopTokens + s.cliTokens)
                let desktopPct = Int((Double(s.desktopTokens) * 100.0) / Double(totalSrc))
                let cliPct = s.cliTokens > 0 ? (100 - desktopPct) : 0
                HStack {
                    Text("桌面客户端 \(desktopPct)% · CLI \(cliPct)%")
                        .font(.system(size: 12, weight: .medium))
                        .foregroundColor(.secondary)
                    Spacer()
                    Text(selectedChartType.statusBarLabel)
                        .font(.system(size: 11))
                        .foregroundColor(.secondary.opacity(0.75))
                        .contentTransition(.numericText())
                        .animation(.easeInOut(duration: 0.18), value: selectedChartType)
                }
                .padding(.horizontal, 12)
                .padding(.vertical, 9)
                .background(Color(uiColor: .secondarySystemGroupedBackground))
                .clipShape(RoundedRectangle(cornerRadius: 10))

                let allModels = history.resolvedModelSummary
                TokenHistoryChartCarouselCard(
                    matrix: matrix,
                    modelSummary: allModels,
                    selectedDate: activeDate,
                    selectedChart: $selectedChartType,
                    onSelectDate: { date in
                        selectedHeatmapDate = date
                        expandedInspectorModel = nil
                    },
                    onUserSelectChart: { chart in
                        AppSettings.shared.tokenHistoryChartType = chart.rawValue
                    },
                    onOpenAllModels: allModels.isEmpty ? nil : {
                        showAllModelsSheet = true
                    }
                )
                .onAppear {
                    if !hasInitializedChartType {
                        let resolved = TokenHistoryChartType.resolveDefault(
                            savedRawValue: AppSettings.shared.tokenHistoryChartType,
                            activeDays: s.activeDays
                        )
                        hasInitializedChartType = true
                        if resolved != selectedChartType {
                            suppressNextChartPersist = true
                            selectedChartType = resolved
                        }
                    }
                }
                .onChange(of: selectedChartType) { _, newValue in
                    if suppressNextChartPersist {
                        suppressNextChartPersist = false
                        return
                    }
                    if hasInitializedChartType {
                        AppSettings.shared.tokenHistoryChartType = newValue.rawValue
                    }
                }

                if let rec = selectedRec {
                    tokenInspectorCard(record: rec)
                }

                if history.updatedAt > 0 {
                    Text("本地历史扫描于 \(history.updatedAtDate.formatted(date: .omitted, time: .standard))")
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

    private func tokenInspectorCard(record: DailyTokenRecord) -> some View {
        let allDayModels = record.models.sorted { lhs, rhs in
            if lhs.value != rhs.value { return lhs.value > rhs.value }
            return lhs.key < rhs.key
        }
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
                inspectorCell(String(localized: "Input"), Self.formatTokensCompact(record.input))
                Spacer()
                inspectorCell(String(localized: "Cache Hit"), Self.formatTokensCompact(record.cacheRead))
                Spacer()
                inspectorCell(String(localized: "Output"), Self.formatTokensCompact(record.output))
                Spacer()
                inspectorCell(String(localized: "Thinking"), Self.formatTokensCompact(record.thinking))
            }
            if !allDayModels.isEmpty {
                Divider()
                VStack(spacing: 5) {
                    ForEach(allDayModels, id: \.key) { pair in
                        let detail = record.modelDetails?[pair.key]
                        let canExpand = detail.map { $0.input > 0 || $0.output > 0 || $0.cacheRead > 0 || $0.thinking > 0 } ?? false
                        let isExpanded = canExpand && expandedInspectorModel == pair.key
                        VStack(alignment: .leading, spacing: 4) {
                            HStack {
                                Text(pair.key)
                                    .font(.system(size: 11, weight: isExpanded ? .medium : .regular))
                                    .foregroundColor(isExpanded ? .primary : .secondary)
                                    .lineLimit(1)
                                Spacer()
                                Text(Self.formatTokensCompact(pair.value))
                                    .font(.system(size: 11, design: .monospaced))
                            }
                            if isExpanded, let detail {
                                HStack {
                                    Text("In \(Self.formatTokensCompact(detail.input))")
                                    Spacer()
                                    Text("Cache \(Self.formatTokensCompact(detail.cacheRead))")
                                    Spacer()
                                    Text("Out \(Self.formatTokensCompact(detail.output))")
                                    if detail.thinking > 0 {
                                        Spacer()
                                        Text("Think \(Self.formatTokensCompact(detail.thinking))")
                                    }
                                }
                                .font(.system(size: 10, design: .monospaced))
                                .foregroundColor(.secondary)
                                .padding(.horizontal, 8)
                                .padding(.vertical, 5)
                                .background(Color(uiColor: .systemGroupedBackground).opacity(0.75))
                                .clipShape(RoundedRectangle(cornerRadius: 6, style: .continuous))
                            }
                        }
                        .contentShape(Rectangle())
                        .onTapGesture {
                            guard canExpand else { return }
                            UIImpactFeedbackGenerator(style: .light).impactOccurred()
                            withAnimation(.easeInOut(duration: 0.16)) {
                                expandedInspectorModel = isExpanded ? nil : pair.key
                            }
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

    public static func formatTokensCompact(_ tokens: Int64) -> String {
        if tokens >= 1_000_000_000 { return String(format: "%.2fB", Double(tokens) / 1_000_000_000.0) }
        if tokens >= 1_000_000 { return String(format: "%.2fM", Double(tokens) / 1_000_000.0) }
        if tokens >= 1_000 { return String(format: "%.1fK", Double(tokens) / 1_000.0) }
        return "\(tokens)"
    }

    @MainActor
    private func renderTokenHistoryImage(failureTitle: String) -> UIImage? {
        guard let history = tokenHistory else { return nil }

        let matrix = history.dailyMatrix
        let activeDate = selectedHeatmapDate
            ?? matrix.last(where: { $0.total > 0 })?.date
            ?? matrix.last?.date
        let selectedRec = matrix.first(where: { $0.date == activeDate })

        let card = TokenHistoryShareCardView(
            history: history,
            selectedRecord: selectedRec,
            chartType: selectedChartType,
            colorScheme: colorScheme,
            skin: AppSettings.shared.resolvedShareCardSkin(for: colorScheme),
            exportDate: Date()
        )

        guard let image = card.renderImage(displayScale: displayScale) else {
            alertTitle = failureTitle
            errorMessage = String(localized: "长图生成失败，请稍后重试")
            return nil
        }

        return image
    }

    @MainActor
    private func saveTokenHistory() async {
        guard !isSavingTokenHistory, !isSharingTokenHistory else { return }
        isSavingTokenHistory = true
        defer { isSavingTokenHistory = false }

        guard let image = renderTokenHistoryImage(failureTitle: String(localized: "保存失败")) else {
            return
        }

        await ImageActions.saveWithFeedback(image)
    }

    @MainActor
    private func shareTokenHistory() {
        guard tokenHistory != nil, !isSavingTokenHistory, !isSharingTokenHistory else { return }
        showTokenHistoryShareSheet = true
    }
}

private struct ModelUsageBreakdownSheet: View {
    let models: [ModelTokenBreakdown]

    private var grandTotal: Int64 {
        max(1, models.reduce(Int64(0)) { $0 + $1.total })
    }

    private func dotColor(for idx: Int) -> Color {
        switch idx % 4 {
        case 0: return .blue
        case 1: return Color(red: 0.22, green: 0.74, blue: 0.42)
        case 2: return Color(red: 0.96, green: 0.62, blue: 0.04)
        default: return Color(red: 0.66, green: 0.33, blue: 0.97)
        }
    }

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(spacing: 10) {
                    ForEach(Array(models.enumerated()), id: \.element.id) { idx, item in
                        let pct = Double(item.total) * 100.0 / Double(grandTotal)
                        let hasDetail = item.input > 0 || item.output > 0 || item.cacheRead > 0 || item.thinking > 0
                        VStack(alignment: .leading, spacing: 8) {
                            HStack(spacing: 6) {
                                Circle()
                                    .fill(dotColor(for: idx))
                                    .frame(width: 7, height: 7)
                                Text(item.model)
                                    .font(.system(size: 12.5, weight: .semibold))
                                    .lineLimit(1)
                                    .truncationMode(.tail)
                                Spacer(minLength: 6)
                                Text(AccountQuotaSheet.formatTokensCompact(item.total))
                                    .font(.system(size: 12, weight: .bold, design: .monospaced))
                                    .foregroundColor(.blue)
                                Text(String(format: "%.1f%%", pct))
                                    .font(.system(size: 11, design: .monospaced))
                                    .foregroundColor(.secondary)
                            }
                            if hasDetail {
                                Divider()
                                HStack {
                                    detailCell(String(localized: "Input"), AccountQuotaSheet.formatTokensCompact(item.input))
                                    Spacer()
                                    detailCell(String(localized: "Cache Hit"), AccountQuotaSheet.formatTokensCompact(item.cacheRead))
                                    Spacer()
                                    detailCell(String(localized: "Output"), AccountQuotaSheet.formatTokensCompact(item.output))
                                    Spacer()
                                    detailCell(String(localized: "Thinking"), AccountQuotaSheet.formatTokensCompact(item.thinking))
                                }
                                if item.steps > 0 || item.costUsd > 0 || (item.medianTps ?? 0) > 0 {
                                    HStack {
                                        Text("\(item.steps) 轮采样")
                                            .font(.system(size: 10.5))
                                            .foregroundColor(.secondary)
                                        if let tps = item.medianTps, tps > 0 {
                                            let ttftPart = (item.medianTtftSec ?? 0) > 0
                                                ? String(format: " · %.1fs", item.medianTtftSec ?? 0)
                                                : ""
                                            Text("· \(String(format: "%.1f", tps)) t/s\(ttftPart)")
                                                .font(.system(size: 10.5, design: .monospaced))
                                                .monospacedDigit()
                                                .foregroundColor(.secondary)
                                        }
                                        Spacer()
                                        Text(String(format: "$%.3f", item.costUsd))
                                            .font(.system(size: 10.5, design: .monospaced))
                                            .monospacedDigit()
                                            .foregroundColor(.secondary)
                                    }
                                }
                            }
                        }
                        .padding(12)
                        .background(Color(uiColor: .secondarySystemGroupedBackground))
                        .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
                    }
                }
                .padding(.horizontal, 16)
                .padding(.vertical, 12)
            }
            .background(Color(uiColor: .systemGroupedBackground))
            .navigationTitle("模型用量明细")
            .navigationBarTitleDisplayMode(.inline)
        }
    }

    private func detailCell(_ label: String, _ value: String) -> some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(label).font(.system(size: 10)).foregroundColor(.secondary)
            Text(value).font(.system(size: 12, weight: .semibold, design: .monospaced))
        }
    }
}
