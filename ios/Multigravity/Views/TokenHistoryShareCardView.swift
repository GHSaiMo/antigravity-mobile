import SwiftUI

/// 专为 Token 历史设计的长图海报渲染视图（支持 iOS ImageRenderer 离屏导出）
public struct TokenHistoryShareCardView: View {
    public let history: TokenHistoryResponse
    public let selectedRecord: DailyTokenRecord?
    public let chartType: TokenHistoryChartType
    public let colorScheme: ColorScheme
    public let skin: ThemePreset?
    public let exportDate: Date

    public static let designWidth: CGFloat = 390

    public init(
        history: TokenHistoryResponse,
        selectedRecord: DailyTokenRecord? = nil,
        chartType: TokenHistoryChartType = .heatmap,
        colorScheme: ColorScheme = .dark,
        skin: ThemePreset? = nil,
        exportDate: Date = Date()
    ) {
        self.history = history
        self.selectedRecord = selectedRecord
        self.chartType = chartType
        self.colorScheme = colorScheme
        self.skin = skin
        self.exportDate = exportDate
    }

    private var resolvedSkin: ThemePreset {
        skin ?? (colorScheme == .dark ? ThemePresetCatalog.defaultDark : ThemePresetCatalog.defaultLight)
    }

    private var effectiveColorScheme: ColorScheme {
        skin != nil ? (resolvedSkin.isDark ? .dark : .light) : colorScheme
    }

    private var isDark: Bool {
        effectiveColorScheme == .dark
    }

    @ViewBuilder
    private var backgroundView: some View {
        if resolvedSkin.isDefaultStyle {
            isDark ? Color(red: 0.08, green: 0.09, blue: 0.13) : Color(red: 0.95, green: 0.96, blue: 0.98)
        } else {
            ShareCardTheme(preset: resolvedSkin).background
        }
    }

    private var cardBackground: Color {
        if resolvedSkin.isDefaultStyle {
            return isDark ? Color(red: 0.13, green: 0.14, blue: 0.20) : Color.white
        }
        return resolvedSkin.surface.opacity(isDark ? 0.92 : 0.96)
    }

    private var cardBorderColor: Color {
        if resolvedSkin.isDefaultStyle {
            return isDark ? Color.white.opacity(0.08) : Color.black.opacity(0.06)
        }
        return resolvedSkin.border
    }

    private var primaryTextColor: Color {
        if resolvedSkin.isDefaultStyle {
            return isDark ? Color.white : Color(red: 0.10, green: 0.11, blue: 0.14)
        }
        return resolvedSkin.fg
    }

    private var secondaryTextColor: Color {
        if resolvedSkin.isDefaultStyle {
            return isDark ? Color.white.opacity(0.65) : Color(red: 0.45, green: 0.47, blue: 0.52)
        }
        return resolvedSkin.secondaryFg
    }

    private var accentBlue: Color {
        resolvedSkin.isDefaultStyle ? Color.blue : resolvedSkin.accent
    }

    private var secondaryRatioAccent: Color {
        (resolvedSkin.isDefaultStyle ? Color.purple : Color(uiColor: resolvedSkin.syntax.keyword)).opacity(0.6)
    }

    public var body: some View {
        VStack(spacing: 14) {
            header
            kpiGrid
            bookEquivalentBanner
            sourceRatioBar
            chartCard
            if let record = selectedRecord {
                inspectorCard(record: record)
            }
            footer
        }
        .padding(16)
        .frame(width: Self.designWidth)
        .background(backgroundView)
        .environment(\.colorScheme, effectiveColorScheme)
    }

    // MARK: - Header
    private var header: some View {
        HStack(spacing: 10) {
            Image("AppLogo")
                .resizable()
                .scaledToFit()
                .frame(width: 36, height: 36)
                .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
                .overlay(
                    RoundedRectangle(cornerRadius: 8, style: .continuous)
                        .stroke(cardBorderColor, lineWidth: 0.6)
                )

            VStack(alignment: .leading, spacing: 2) {
                Text("Multigravity")
                    .font(.system(size: 16, weight: .bold))
                    .foregroundColor(primaryTextColor)
                Text("Token 消耗分析报告")
                    .font(.system(size: 11, weight: .medium))
                    .foregroundColor(secondaryTextColor)
            }

            Spacer()

            let timeStr = Self.dateFormatter.string(from: exportDate)
            HStack(spacing: 4) {
                Circle()
                    .fill(Color.green)
                    .frame(width: 5, height: 5)
                Text(timeStr)
                    .font(.system(size: 10, weight: .medium, design: .monospaced))
                    .foregroundColor(secondaryTextColor)
            }
            .padding(.horizontal, 8)
            .padding(.vertical, 4)
            .background(cardBackground)
            .clipShape(Capsule())
            .overlay(Capsule().stroke(cardBorderColor, lineWidth: 0.8))
        }
        .padding(.top, 4)
    }

    // MARK: - KPI Grid
    private var kpiGrid: some View {
        let s = history.summary
        return LazyVGrid(
            columns: [GridItem(.flexible(), spacing: 8), GridItem(.flexible(), spacing: 8)],
            spacing: 8
        ) {
            kpiCell(title: String(localized: "今日消耗"), value: AccountQuotaSheet.formatTokensCompact(s.todayTokens), sub: String(localized: "\(s.activeDays) 活跃日"))
            kpiCell(title: String(localized: "近 7 日"), value: AccountQuotaSheet.formatTokensCompact(s.last7dTokens), sub: String(localized: "\(s.totalSteps) 轮采样"))
            kpiCell(title: String(localized: "累计总 Token"), value: AccountQuotaSheet.formatTokensCompact(s.totalTokens), sub: "IDE \(AccountQuotaSheet.formatTokensCompact(s.desktopTokens)) · CLI \(AccountQuotaSheet.formatTokensCompact(s.cliTokens))")
            kpiCell(title: String(localized: "API 折算"), value: AccountQuotaSheet.formatUSD(s.estimatedCostUsd), sub: AccountQuotaSheet.formatSecondaryCost(cny: s.estimatedCostCny))
        }
    }

    private func kpiCell(title: String, value: String, sub: String) -> some View {
        VStack(alignment: .leading, spacing: 3) {
            Text(title)
                .font(.system(size: 11))
                .foregroundColor(secondaryTextColor)
                .lineLimit(1)
            Text(value)
                .font(.system(size: 17, weight: .bold, design: .monospaced))
                .foregroundColor(primaryTextColor)
                .lineLimit(1)
            Text(sub)
                .font(.system(size: 10.5))
                .foregroundColor(secondaryTextColor.opacity(0.85))
                .lineLimit(1)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(.horizontal, 12)
        .padding(.vertical, 10)
        .background(cardBackground)
        .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: 12, style: .continuous).stroke(cardBorderColor, lineWidth: 0.8))
    }

    // MARK: - Book Equivalent Banner
    @ViewBuilder
    private var bookEquivalentBanner: some View {
        if let eq = TokenBookEquivalent.resolve(from: history) {
            HStack(spacing: 10) {
                ZStack {
                    RoundedRectangle(cornerRadius: 8, style: .continuous)
                        .fill(accentBlue.opacity(0.14))
                        .frame(width: 28, height: 28)
                    Image(systemName: "books.vertical.fill")
                        .font(.system(size: 12, weight: .semibold))
                        .foregroundColor(accentBlue)
                }
                VStack(alignment: .leading, spacing: 2) {
                    Text(eq.sentence)
                        .font(.system(size: 12, weight: .semibold))
                        .foregroundColor(primaryTextColor)
                        .lineLimit(2)
                    Text(eq.authorOrigin)
                        .font(.system(size: 10))
                        .foregroundColor(secondaryTextColor)
                        .lineLimit(1)
                }
                Spacer(minLength: 4)
            }
            .padding(.horizontal, 12)
            .padding(.vertical, 8)
            .background(cardBackground)
            .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
            .overlay(RoundedRectangle(cornerRadius: 10, style: .continuous).stroke(accentBlue.opacity(0.25), lineWidth: 0.8))
        }
    }

    // MARK: - Source Ratio Bar
    private var sourceRatioBar: some View {
        let s = history.summary
        let totalSrc = max(Int64(1), s.desktopTokens + s.cliTokens)
        let desktopPct = Int((Double(s.desktopTokens) * 100.0) / Double(totalSrc))
        let cliPct = s.cliTokens > 0 ? (100 - desktopPct) : 0

        return VStack(spacing: 6) {
            HStack {
                Text("桌面客户端 \(desktopPct)% · CLI \(cliPct)%")
                    .font(.system(size: 11.5, weight: .medium))
                    .foregroundColor(secondaryTextColor)
                Spacer()
                Text(chartType.statusBarLabel)
                    .font(.system(size: 10.5))
                    .foregroundColor(secondaryTextColor.opacity(0.8))
            }

            GeometryReader { geo in
                ZStack(alignment: .leading) {
                    RoundedRectangle(cornerRadius: 3)
                        .fill(secondaryRatioAccent)
                        .frame(height: 6)
                    RoundedRectangle(cornerRadius: 3)
                        .fill(accentBlue)
                        .frame(width: geo.size.width * CGFloat(desktopPct) / 100.0, height: 6)
                }
            }
            .frame(height: 6)
        }
        .padding(.horizontal, 12)
        .padding(.vertical, 9)
        .background(cardBackground)
        .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: 10, style: .continuous).stroke(cardBorderColor, lineWidth: 0.8))
    }

    // MARK: - Chart Card (所见即所得，跟随当前所选图表)
    private var chartCard: some View {
        let palette = TokenHistoryChartPalette.shareCard(
            primaryText: primaryTextColor,
            secondaryText: secondaryTextColor,
            accent: accentBlue,
            isDark: isDark
        )
        let innerWidth = Self.designWidth - 32 - 24

        return Group {
            switch chartType {
            case .dailyBars:
                TokenDailyBarsChartView(
                    matrix: history.dailyMatrix,
                    selectedDate: selectedRecord?.date,
                    palette: palette
                )
            case .modelBreakdown:
                TokenModelBreakdownChartView(
                    matrix: history.dailyMatrix,
                    palette: palette
                )
            case .heatmap:
                TokenHeatmapGridChartView(
                    matrix: history.dailyMatrix,
                    selectedDate: selectedRecord?.date,
                    palette: palette,
                    fixedWidth: innerWidth
                )
            }
        }
        .padding(12)
        .background(cardBackground)
        .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: 12, style: .continuous).stroke(cardBorderColor, lineWidth: 0.8))
    }

    // MARK: - Inspector Card
    private func inspectorCard(record: DailyTokenRecord) -> some View {
        let allDayModels = record.models.sorted { lhs, rhs in
            if lhs.value != rhs.value { return lhs.value > rhs.value }
            return lhs.key < rhs.key
        }
        return VStack(alignment: .leading, spacing: 8) {
            HStack {
                Text("\(record.date) 用量明细")
                    .font(.system(size: 13, weight: .semibold))
                    .foregroundColor(primaryTextColor)
                Spacer()
                Text("\(AccountQuotaSheet.formatTokensCompact(record.total)) · $\(String(format: "%.3f", record.costUsd))")
                    .font(.system(size: 12, weight: .semibold, design: .monospaced))
                    .foregroundColor(accentBlue)
            }
            Divider().overlay(cardBorderColor)
            HStack {
                inspectorCell(String(localized: "Input"), AccountQuotaSheet.formatTokensCompact(record.input))
                Spacer()
                inspectorCell(String(localized: "Cache Hit"), AccountQuotaSheet.formatTokensCompact(record.cacheRead))
                Spacer()
                inspectorCell(String(localized: "Output"), AccountQuotaSheet.formatTokensCompact(record.output))
                Spacer()
                inspectorCell(String(localized: "Thinking"), AccountQuotaSheet.formatTokensCompact(record.thinking))
            }
            if !allDayModels.isEmpty {
                Divider().overlay(cardBorderColor)
                VStack(spacing: 4) {
                    ForEach(allDayModels, id: \.key) { pair in
                        HStack {
                            Text(pair.key)
                                .font(.system(size: 11))
                                .foregroundColor(secondaryTextColor)
                                .lineLimit(1)
                            Spacer(minLength: 8)
                            Text(AccountQuotaSheet.formatTokensCompact(pair.value))
                                .font(.system(size: 11, design: .monospaced))
                                .foregroundColor(primaryTextColor)
                                .lineLimit(1)
                        }
                    }
                }
            }
        }
        .padding(12)
        .background(cardBackground)
        .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: 12, style: .continuous).stroke(cardBorderColor, lineWidth: 0.8))
    }

    private func inspectorCell(_ label: String, _ value: String) -> some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(label).font(.system(size: 10)).foregroundColor(secondaryTextColor)
            Text(value).font(.system(size: 12, weight: .semibold, design: .monospaced)).foregroundColor(primaryTextColor)
        }
    }

    // MARK: - Footer
    private var footer: some View {
        VStack(spacing: 4) {
            Divider().overlay(cardBorderColor)
                .padding(.bottom, 2)
            Text("由 Multigravity 自动统计生成 · 跨端协同 AI 控制台")
                .font(.system(size: 10, weight: .medium))
                .foregroundColor(secondaryTextColor.opacity(0.85))
            if history.updatedAt > 0 {
                Text("本地数据扫描于 \(Self.scanTimeFormatter.string(from: history.updatedAtDate))")
                    .font(.system(size: 9.5))
                    .foregroundColor(secondaryTextColor.opacity(0.6))
            }
        }
        .padding(.horizontal, 4)
        .padding(.top, 2)
    }

    /// 离屏渲染长图（含 8192px 像素高度上限保护，避免模型明细过多时内存峰值过高）
    @MainActor
    public func renderImage(displayScale: CGFloat) -> UIImage? {
        let renderer = ImageRenderer(content: self)
        renderer.isOpaque = true
        var scale = max(displayScale, 2.0)
        renderer.scale = scale
        var image = renderer.uiImage
        let maxPixelHeight: CGFloat = 8192
        if let current = image, current.size.height * scale > maxPixelHeight {
            scale = max(1.0, maxPixelHeight / current.size.height)
            renderer.scale = scale
            image = renderer.uiImage
        }
        return image
    }

    // MARK: - Helpers
    private static let dateFormatter: DateFormatter = {
        let f = DateFormatter()
        f.locale = Locale(identifier: "en_US_POSIX")
        f.timeZone = .autoupdatingCurrent
        f.dateFormat = "yyyy-MM-dd HH:mm"
        return f
    }()

    private static let scanTimeFormatter: DateFormatter = {
        let f = DateFormatter()
        f.locale = Locale(identifier: "en_US_POSIX")
        f.timeZone = .autoupdatingCurrent
        f.dateFormat = "HH:mm:ss"
        return f
    }()
}

// MARK: - Token History Share Sheet (with curated skin switcher)

public struct TokenHistoryShareCardSheet: View {
    public let history: TokenHistoryResponse
    public let selectedRecord: DailyTokenRecord?
    public let chartType: TokenHistoryChartType

    @Environment(\.colorScheme) private var systemScheme
    @Environment(\.displayScale) private var displayScale

    @State private var selectedSkin: ThemePreset = ThemePresetCatalog.defaultDark
    @State private var hasInitializedSkin = false
    @State private var showSkinPicker = false
    @State private var renderedImage: UIImage?
    @State private var isRendering = false
    @State private var renderFailed = false

    public init(
        history: TokenHistoryResponse,
        selectedRecord: DailyTokenRecord? = nil,
        chartType: TokenHistoryChartType = .heatmap
    ) {
        self.history = history
        self.selectedRecord = selectedRecord
        self.chartType = chartType
    }

    public var body: some View {
        NavigationStack {
            VStack(spacing: 8) {
                ScrollView {
                    Group {
                        if let renderedImage {
                            Image(uiImage: renderedImage)
                                .resizable()
                                .scaledToFit()
                                .clipShape(RoundedRectangle(cornerRadius: 14, style: .continuous))
                                .shadow(color: .black.opacity(0.15), radius: 8, y: 2)
                                .opacity(isRendering ? 0.5 : 1)
                        } else if renderFailed {
                            Text("长图生成失败，请稍后重试")
                                .font(.footnote)
                                .foregroundColor(.secondary)
                                .frame(maxWidth: .infinity, minHeight: 240)
                        } else {
                            ProgressView()
                                .frame(maxWidth: .infinity, minHeight: 240)
                        }
                    }
                    .padding(.horizontal, 16)
                    .padding(.top, 8)
                    .padding(.bottom, 80)
                }
            }
            .safeAreaInset(edge: .bottom) {
                GlassEffectContainer(spacing: 12) {
                    HStack {
                        Button {
                            UIImpactFeedbackGenerator(style: .light).impactOccurred()
                            showSkinPicker = true
                        } label: {
                            HStack(spacing: 7) {
                                ThemeSwatchDotsView(preset: selectedSkin)
                                Text(selectedSkin.name)
                                    .font(.system(size: 14, weight: .semibold))
                                    .lineLimit(1)
                                Image(systemName: "chevron.up.chevron.down")
                                    .font(.system(size: 11, weight: .semibold))
                                    .foregroundColor(.secondary)
                            }
                            .padding(.horizontal, 14)
                            .frame(height: 44)
                        }
                        .buttonStyle(.plain)
                        .glassEffect(.regular.interactive(), in: .capsule)

                        Spacer()
                    }
                    .padding(.horizontal, 16)
                    .padding(.bottom, 8)
                }
            }
            .navigationTitle("分享 Token 报告")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    Button {
                        guard let image = renderedImage else { return }
                        Task { await ImageActions.saveWithFeedback(image) }
                    } label: {
                        Image(systemName: "square.and.arrow.down")
                    }
                    .accessibilityLabel("保存到相册")
                    .disabled(renderedImage == nil || isRendering)
                }
                ToolbarItem(placement: .topBarTrailing) {
                    Button {
                        guard let image = renderedImage else { return }
                        ImageActions.share(image)
                    } label: {
                        Image(systemName: "square.and.arrow.up")
                    }
                    .accessibilityLabel("分享")
                    .disabled(renderedImage == nil || isRendering)
                }
            }
        }
        .onAppear {
            if !hasInitializedSkin {
                selectedSkin = AppSettings.shared.resolvedShareCardSkin(for: systemScheme)
                hasInitializedSkin = true
            }
        }
        .task(id: selectedSkin.storageKey) {
            await render()
        }
        .sheet(isPresented: $showSkinPicker) {
            ShareCardSkinPickerSheet(selectedPreset: selectedSkin) { picked in
                AppSettings.shared.shareCardSkinID = picked.id
                selectedSkin = picked
                showSkinPicker = false
            }
            .presentationDetents([.medium, .large])
            .presentationDragIndicator(.visible)
        }
    }

    @MainActor
    private func render() async {
        isRendering = true
        renderFailed = false
        defer { isRendering = false }

        let card = TokenHistoryShareCardView(
            history: history,
            selectedRecord: selectedRecord,
            chartType: chartType,
            colorScheme: systemScheme,
            skin: selectedSkin,
            exportDate: Date()
        )
        if let uiImage = card.renderImage(displayScale: displayScale) {
            renderedImage = uiImage
        } else {
            renderFailed = true
        }
    }
}
