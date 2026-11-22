import SwiftUI

/// Token 历史页三张可滑动切换的图表类型（顺序固定：每日趋势柱状图 → 模型消耗占比 → 每日活跃热力图）
public enum TokenHistoryChartType: String, CaseIterable, Identifiable, Sendable {
    case dailyBars = "daily_bars"
    case modelBreakdown = "model_breakdown"
    case heatmap = "heatmap"

    public var id: String { rawValue }

    /// 顶部状态栏右侧文案（随当前页动态切换）
    public var statusBarLabel: String {
        switch self {
        case .dailyBars:
            return String(localized: "近 14 日趋势")
        case .modelBreakdown:
            return String(localized: "累计模型占比")
        case .heatmap:
            return String(localized: "近 20 周热力图")
        }
    }

    /// 首次打开时智能决定默认图表：
    /// 若用户手动切换过则沿用记忆；否则活跃 < 28 天默认柱状图，≥ 28 天默认热力图。
    public static func resolveDefault(savedRawValue: String, activeDays: Int) -> TokenHistoryChartType {
        if let saved = TokenHistoryChartType(rawValue: savedRawValue) {
            return saved
        }
        return activeDays < 28 ? .dailyBars : .heatmap
    }
}

/// 统一承载 App 内部卡片与离屏长图海报（含多套皮肤）的配色板
public struct TokenHistoryChartPalette {
    public let primaryText: Color
    public let secondaryText: Color
    public let accent: Color
    public let emptyCell: Color
    public let gridLine: Color
    public let isDark: Bool

    public static func app(colorScheme: ColorScheme) -> TokenHistoryChartPalette {
        let isDark = colorScheme == .dark
        return TokenHistoryChartPalette(
            primaryText: .primary,
            secondaryText: .secondary,
            accent: .blue,
            emptyCell: Color.secondary.opacity(isDark ? 0.16 : 0.13),
            gridLine: Color.primary.opacity(isDark ? 0.08 : 0.07),
            isDark: isDark
        )
    }

    public static func shareCard(
        primaryText: Color,
        secondaryText: Color,
        accent: Color,
        isDark: Bool
    ) -> TokenHistoryChartPalette {
        TokenHistoryChartPalette(
            primaryText: primaryText,
            secondaryText: secondaryText,
            accent: accent,
            emptyCell: isDark ? Color.white.opacity(0.09) : Color.black.opacity(0.06),
            gridLine: isDark ? Color.white.opacity(0.08) : Color.black.opacity(0.07),
            isDark: isDark
        )
    }

    public func heatColor(level: Int) -> Color {
        switch level {
        case 0:
            return emptyCell
        case 1:
            return Color(red: 0.61, green: 0.91, blue: 0.66).opacity(0.7)
        case 2:
            return Color(red: 0.25, green: 0.77, blue: 0.39).opacity(0.85)
        case 3:
            return Color(red: 0.19, green: 0.63, blue: 0.31)
        default:
            return Color(red: 0.13, green: 0.43, blue: 0.22)
        }
    }

    public func modelColor(index: Int, isOther: Bool) -> Color {
        if isOther {
            return secondaryText.opacity(0.45)
        }
        switch index {
        case 0:
            return accent
        case 1:
            return Color(red: 0.22, green: 0.74, blue: 0.42)
        case 2:
            return Color(red: 0.96, green: 0.62, blue: 0.04)
        case 3:
            return Color(red: 0.66, green: 0.33, blue: 0.97)
        default:
            return secondaryText.opacity(0.45)
        }
    }
}

// MARK: - Data Builders

public enum TokenHistoryChartData {
    public struct DailyBarSlot: Identifiable {
        public let id: String
        public let date: String
        public let shortLabel: String
        public let total: Int64
        public let record: DailyTokenRecord?
    }

    public struct DailyBarsSeries {
        public let slots: [DailyBarSlot]
        public let maxTotal: Int64
        public let activeAverage: Int64
    }

    public struct ModelSlice: Identifiable {
        public let id: String
        public let name: String
        public let tokens: Int64
        public let percent: Double
        public let colorIndex: Int
        public let isOther: Bool
    }

    public struct ModelBreakdownSeries {
        public let slices: [ModelSlice]
        public let uniqueModelCount: Int
        public let grandTotal: Int64
    }

    public struct HeatmapCell: Identifiable {
        public let id: String
        public let date: String
        public let record: DailyTokenRecord?
        public let level: Int
        public let isFuture: Bool
    }

    private static let dayFormatter: DateFormatter = {
        let df = DateFormatter()
        df.locale = Locale(identifier: "en_US_POSIX")
        df.calendar = Calendar(identifier: .gregorian)
        df.dateFormat = "yyyy-MM-dd"
        return df
    }()

    private static let shortDayFormatter: DateFormatter = {
        let df = DateFormatter()
        df.locale = Locale(identifier: "en_US_POSIX")
        df.calendar = Calendar(identifier: .gregorian)
        df.dateFormat = "MM/dd"
        return df
    }()

    private static func resolveAnchorDate(matrix: [DailyTokenRecord]) -> Date {
        let cal = Calendar(identifier: .gregorian)
        let today = cal.startOfDay(for: Date())
        if let lastKey = matrix.last?.date, let parsed = dayFormatter.date(from: lastKey) {
            let lastDay = cal.startOfDay(for: parsed)
            return max(today, lastDay)
        }
        return today
    }

    /// 构造最近 N 个自然日（默认 14 天）的连续柱状图序列
    public static func buildDailyBars(matrix: [DailyTokenRecord], days: Int = 14) -> DailyBarsSeries {
        let cal = Calendar(identifier: .gregorian)
        let anchor = resolveAnchorDate(matrix: matrix)
        let byDate = Dictionary(uniqueKeysWithValues: matrix.map { ($0.date, $0) })

        var slots: [DailyBarSlot] = []
        slots.reserveCapacity(days)
        var maxTotal: Int64 = 0
        var activeSum: Int64 = 0
        var activeCount: Int64 = 0

        for offset in stride(from: -(days - 1), through: 0, by: 1) {
            guard let d = cal.date(byAdding: .day, value: offset, to: anchor) else { continue }
            let key = dayFormatter.string(from: d)
            let shortLabel = shortDayFormatter.string(from: d)
            let rec = byDate[key]
            let total = rec?.total ?? 0
            if total > maxTotal { maxTotal = total }
            if total > 0 {
                activeSum += total
                activeCount += 1
            }
            slots.append(DailyBarSlot(
                id: key,
                date: key,
                shortLabel: shortLabel,
                total: total,
                record: rec
            ))
        }

        let avg = activeCount > 0 ? activeSum / activeCount : 0
        return DailyBarsSeries(slots: slots, maxTotal: maxTotal, activeAverage: avg)
    }

    /// 汇总全历史各模型 Token 消耗（超过 4 款模型时合并为 Top 3 + 其他，保证卡片高度稳定）
    public static func buildModelBreakdown(matrix: [DailyTokenRecord]) -> ModelBreakdownSeries {
        var totals: [String: Int64] = [:]
        for rec in matrix {
            for (model, count) in rec.models where count > 0 {
                totals[model, default: 0] += count
            }
        }

        let sorted = totals.sorted { lhs, rhs in
            if lhs.value != rhs.value { return lhs.value > rhs.value }
            return lhs.key < rhs.key
        }

        let grandTotal = sorted.reduce(Int64(0)) { $0 + $1.value }
        guard grandTotal > 0 else {
            return ModelBreakdownSeries(slices: [], uniqueModelCount: 0, grandTotal: 0)
        }

        var slices: [ModelSlice] = []
        if sorted.count <= 4 {
            for (idx, pair) in sorted.enumerated() {
                let pct = Double(pair.value) * 100.0 / Double(grandTotal)
                slices.append(ModelSlice(
                    id: pair.key,
                    name: pair.key,
                    tokens: pair.value,
                    percent: pct,
                    colorIndex: idx,
                    isOther: false
                ))
            }
        } else {
            for idx in 0..<3 {
                let pair = sorted[idx]
                let pct = Double(pair.value) * 100.0 / Double(grandTotal)
                slices.append(ModelSlice(
                    id: pair.key,
                    name: pair.key,
                    tokens: pair.value,
                    percent: pct,
                    colorIndex: idx,
                    isOther: false
                ))
            }
            let otherTokens = sorted.dropFirst(3).reduce(Int64(0)) { $0 + $1.value }
            let otherPct = Double(otherTokens) * 100.0 / Double(grandTotal)
            slices.append(ModelSlice(
                id: "__other__",
                name: String(localized: "其他"),
                tokens: otherTokens,
                percent: otherPct,
                colorIndex: 4,
                isOther: true
            ))
        }

        return ModelBreakdownSeries(
            slices: slices,
            uniqueModelCount: sorted.count,
            grandTotal: grandTotal
        )
    }

    /// 构造铺满最近 `weekCount` 周的固定网格（右对齐至今天，历史无记录填 0 级浅灰格，未来日期留空）
    public static func buildFixedHeatmapWeeks(
        matrix: [DailyTokenRecord],
        weekCount: Int = 20
    ) -> [[HeatmapCell]] {
        let cal = Calendar(identifier: .gregorian)
        let anchor = resolveAnchorDate(matrix: matrix)
        let weekday = cal.component(.weekday, from: anchor) // 1 = Sun ... 7 = Sat
        guard let currentWeekSunday = cal.date(byAdding: .day, value: -(weekday - 1), to: anchor),
              let startSunday = cal.date(byAdding: .day, value: -((weekCount - 1) * 7), to: currentWeekSunday) else {
            return []
        }

        let byDate = Dictionary(uniqueKeysWithValues: matrix.map { ($0.date, $0) })
        let maxTotal = max(Int64(1), matrix.map(\.total).max() ?? 1)

        var weeks: [[HeatmapCell]] = []
        weeks.reserveCapacity(weekCount)

        for w in 0..<weekCount {
            var col: [HeatmapCell] = []
            col.reserveCapacity(7)
            for d in 0..<7 {
                guard let day = cal.date(byAdding: .day, value: w * 7 + d, to: startSunday) else { continue }
                let key = dayFormatter.string(from: day)
                if day > anchor {
                    col.append(HeatmapCell(id: key, date: key, record: nil, level: 0, isFuture: true))
                } else {
                    let rec = byDate[key]
                    let total = rec?.total ?? 0
                    let ratio = Double(total) / Double(maxTotal)
                    let level: Int = {
                        if total <= 0 { return 0 }
                        if ratio < 0.2 { return 1 }
                        if ratio < 0.45 { return 2 }
                        if ratio < 0.75 { return 3 }
                        return 4
                    }()
                    col.append(HeatmapCell(id: key, date: key, record: rec, level: level, isFuture: false))
                }
            }
            weeks.append(col)
        }
        return weeks
    }
}

// MARK: - Chart 1: 14-Day Daily Bar Chart

public struct TokenDailyBarsChartView: View {
    public let matrix: [DailyTokenRecord]
    public let selectedDate: String?
    public let palette: TokenHistoryChartPalette
    public let onSelectDate: ((String) -> Void)?

    public static let contentHeight: CGFloat = 112
    private let plotHeight: CGFloat = 92

    public init(
        matrix: [DailyTokenRecord],
        selectedDate: String?,
        palette: TokenHistoryChartPalette,
        onSelectDate: ((String) -> Void)? = nil
    ) {
        self.matrix = matrix
        self.selectedDate = selectedDate
        self.palette = palette
        self.onSelectDate = onSelectDate
    }

    public var body: some View {
        let series = TokenHistoryChartData.buildDailyBars(matrix: matrix, days: 14)

        VStack(alignment: .leading, spacing: 8) {
            HStack {
                Text("近 14 日趋势")
                    .font(.system(size: 12, weight: .semibold))
                    .foregroundColor(palette.primaryText)
                Spacer()
                if series.maxTotal > 0 {
                    Text(String(localized: "峰值 \(AccountQuotaSheet.formatTokensCompact(series.maxTotal))"))
                        .font(.system(size: 10.5, weight: .medium, design: .monospaced))
                        .foregroundColor(palette.secondaryText)
                } else {
                    Text("暂无用量")
                        .font(.system(size: 10.5))
                        .foregroundColor(palette.secondaryText)
                }
            }

            VStack(spacing: 6) {
                ZStack(alignment: .bottom) {
                    // Subtle horizontal guide lines
                    VStack(spacing: 0) {
                        Rectangle()
                            .fill(palette.gridLine)
                            .frame(height: 0.5)
                        Spacer()
                        Rectangle()
                            .fill(palette.gridLine)
                            .frame(height: 0.5)
                        Spacer()
                        Rectangle()
                            .fill(palette.gridLine)
                            .frame(height: 0.5)
                    }
                    .frame(height: plotHeight)

                    // Active-day average reference line
                    if series.maxTotal > 0, series.activeAverage > 0, series.activeAverage < series.maxTotal {
                        let avgRatio = CGFloat(Double(series.activeAverage) / Double(series.maxTotal))
                        let yOffset = max(4, min(plotHeight - 4, plotHeight * avgRatio))
                        HStack(spacing: 3) {
                            ForEach(0..<24, id: \.self) { _ in
                                Capsule()
                                    .fill(palette.secondaryText.opacity(0.24))
                                    .frame(height: 1)
                            }
                        }
                        .offset(y: -yOffset)
                    }

                    // 14 vertical bars
                    HStack(alignment: .bottom, spacing: 5) {
                        ForEach(series.slots) { slot in
                            let isSelected = (slot.date == selectedDate) && (slot.total > 0)
                            let barHeight: CGFloat = {
                                guard series.maxTotal > 0, slot.total > 0 else { return 3 }
                                let ratio = CGFloat(Double(slot.total) / Double(series.maxTotal))
                                return max(6, min(plotHeight, plotHeight * ratio))
                            }()

                            VStack(spacing: 0) {
                                Spacer(minLength: 0)
                                UnevenRoundedRectangle(
                                    topLeadingRadius: 3.5,
                                    bottomLeadingRadius: 1.5,
                                    bottomTrailingRadius: 1.5,
                                    topTrailingRadius: 3.5,
                                    style: .continuous
                                )
                                .fill(barFill(total: slot.total, isSelected: isSelected))
                                .frame(height: barHeight)
                            }
                            .frame(maxWidth: .infinity)
                            .frame(height: plotHeight)
                            .contentShape(Rectangle())
                            .onTapGesture {
                                guard let rec = slot.record, rec.total > 0, let onSelectDate else { return }
                                UIImpactFeedbackGenerator(style: .light).impactOccurred()
                                onSelectDate(rec.date)
                            }
                        }
                    }
                }
                .frame(height: plotHeight)

                // Crisp 3-anchor X-axis labels (start / mid / end)
                HStack {
                    Text(series.slots.first?.shortLabel ?? "")
                    Spacer()
                    if series.slots.count > 7 {
                        Text(series.slots[series.slots.count / 2].shortLabel)
                    }
                    Spacer()
                    Text(series.slots.last?.shortLabel ?? "")
                }
                .font(.system(size: 9.5, design: .monospaced))
                .foregroundColor(palette.secondaryText.opacity(0.85))
                .frame(height: 14)
            }
            .frame(height: Self.contentHeight)
        }
    }

    private func barFill(total: Int64, isSelected: Bool) -> AnyShapeStyle {
        if total <= 0 {
            return AnyShapeStyle(palette.emptyCell)
        }
        if isSelected {
            return AnyShapeStyle(palette.accent)
        }
        return AnyShapeStyle(
            LinearGradient(
                colors: [
                    Color(red: 0.25, green: 0.77, blue: 0.39),
                    Color(red: 0.19, green: 0.63, blue: 0.31)
                ],
                startPoint: .top,
                endPoint: .bottom
            )
        )
    }
}

// MARK: - Chart 2: Model Breakdown Chart

public struct TokenModelBreakdownChartView: View {
    public let matrix: [DailyTokenRecord]
    public let palette: TokenHistoryChartPalette
    public let onOpenAllModels: (() -> Void)?

    public static let contentHeight: CGFloat = 112

    public init(
        matrix: [DailyTokenRecord],
        palette: TokenHistoryChartPalette,
        onOpenAllModels: (() -> Void)? = nil
    ) {
        self.matrix = matrix
        self.palette = palette
        self.onOpenAllModels = onOpenAllModels
    }

    public var body: some View {
        let series = TokenHistoryChartData.buildModelBreakdown(matrix: matrix)
        let canOpen = onOpenAllModels != nil && !series.slices.isEmpty

        VStack(alignment: .leading, spacing: 8) {
            HStack {
                Text("模型消耗占比")
                    .font(.system(size: 12, weight: .semibold))
                    .foregroundColor(palette.primaryText)
                Spacer()
                if series.uniqueModelCount > 0 {
                    let baseLabel = String(localized: "\(series.uniqueModelCount) 款模型")
                    Text(canOpen ? "\(baseLabel) ›" : baseLabel)
                        .font(.system(size: 10.5, weight: .medium, design: .monospaced))
                        .foregroundColor(canOpen ? palette.accent : palette.secondaryText)
                } else {
                    Text("暂无数据")
                        .font(.system(size: 10.5))
                        .foregroundColor(palette.secondaryText)
                }
            }

            if series.slices.isEmpty {
                VStack {
                    Spacer()
                    Text("暂无模型明细数据")
                        .font(.system(size: 12))
                        .foregroundColor(palette.secondaryText)
                    Spacer()
                }
                .frame(maxWidth: .infinity)
                .frame(height: Self.contentHeight)
            } else {
                VStack(alignment: .leading, spacing: 10) {
                    // Top segmented proportional bar
                    GeometryReader { geo in
                        let gap: CGFloat = 2
                        let totalGap = gap * CGFloat(max(0, series.slices.count - 1))
                        let usableWidth = max(1, geo.size.width - totalGap)

                        HStack(spacing: gap) {
                            ForEach(series.slices) { slice in
                                let width = max(4, usableWidth * CGFloat(slice.percent / 100.0))
                                RoundedRectangle(cornerRadius: 3, style: .continuous)
                                    .fill(palette.modelColor(index: slice.colorIndex, isOther: slice.isOther))
                                    .frame(width: width, height: 10)
                            }
                        }
                    }
                    .frame(height: 10)
                    .clipShape(Capsule())

                    // Top 4 model rows (full width for long model names)
                    VStack(spacing: 7) {
                        ForEach(series.slices) { slice in
                            HStack(spacing: 6) {
                                Circle()
                                    .fill(palette.modelColor(index: slice.colorIndex, isOther: slice.isOther))
                                    .frame(width: 7, height: 7)
                                Text(slice.name)
                                    .font(.system(size: 11.5, weight: .medium))
                                    .foregroundColor(palette.primaryText)
                                    .lineLimit(1)
                                    .truncationMode(.tail)
                                Spacer(minLength: 6)
                                Text(AccountQuotaSheet.formatTokensCompact(slice.tokens))
                                    .font(.system(size: 11, weight: .semibold, design: .monospaced))
                                    .foregroundColor(palette.primaryText)
                                Text(String(format: "%.1f%%", slice.percent))
                                    .font(.system(size: 10.5, design: .monospaced))
                                    .foregroundColor(palette.secondaryText)
                                    .frame(width: 42, alignment: .trailing)
                            }
                        }
                    }

                    Spacer(minLength: 0)
                }
                .frame(height: Self.contentHeight)
            }
        }
        .contentShape(Rectangle())
        .onTapGesture {
            guard canOpen else { return }
            UIImpactFeedbackGenerator(style: .light).impactOccurred()
            onOpenAllModels?()
        }
    }
}

// MARK: - Chart 3: Fixed 20-Week Calendar Heatmap

public struct TokenHeatmapGridChartView: View {
    public let matrix: [DailyTokenRecord]
    public let selectedDate: String?
    public let palette: TokenHistoryChartPalette
    /// 为 nil 时在 App 内自适应卡片宽度；离屏海报传入固定内容宽度避免依赖 GeometryReader 尺寸波动
    public let fixedWidth: CGFloat?
    public let onSelectDate: ((String) -> Void)?

    public static let contentHeight: CGFloat = 112
    private let cellSpacing: CGFloat = 3

    public init(
        matrix: [DailyTokenRecord],
        selectedDate: String?,
        palette: TokenHistoryChartPalette,
        fixedWidth: CGFloat? = nil,
        onSelectDate: ((String) -> Void)? = nil
    ) {
        self.matrix = matrix
        self.selectedDate = selectedDate
        self.palette = palette
        self.fixedWidth = fixedWidth
        self.onSelectDate = onSelectDate
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack {
                Text("每日活跃密度")
                    .font(.system(size: 12, weight: .semibold))
                    .foregroundColor(palette.primaryText)
                Spacer()
                HStack(spacing: 3) {
                    Text("少").font(.system(size: 10)).foregroundColor(palette.secondaryText)
                    ForEach(0..<5, id: \.self) { lvl in
                        RoundedRectangle(cornerRadius: 2)
                            .fill(palette.heatColor(level: lvl))
                            .frame(width: 8.5, height: 8.5)
                    }
                    Text("多").font(.system(size: 10)).foregroundColor(palette.secondaryText)
                }
            }

            if let fixedWidth {
                heatmapGrid(availableWidth: fixedWidth)
                    .frame(maxWidth: .infinity)
                    .frame(height: Self.contentHeight, alignment: .center)
            } else {
                GeometryReader { geo in
                    heatmapGrid(availableWidth: geo.size.width)
                        .frame(width: geo.size.width, height: Self.contentHeight, alignment: .center)
                }
                .frame(height: Self.contentHeight)
            }
        }
    }

    @ViewBuilder
    private func heatmapGrid(availableWidth: CGFloat) -> some View {
        let weekCount = availableWidth < 290 ? 18 : 20
        let rawCell = floor((availableWidth - cellSpacing * CGFloat(weekCount - 1)) / CGFloat(weekCount))
        let cellSize = max(9, min(13.5, rawCell))
        let weeks = TokenHistoryChartData.buildFixedHeatmapWeeks(matrix: matrix, weekCount: weekCount)

        HStack(spacing: cellSpacing) {
            ForEach(Array(weeks.enumerated()), id: \.offset) { _, week in
                VStack(spacing: cellSpacing) {
                    ForEach(week) { cell in
                        if cell.isFuture {
                            Color.clear.frame(width: cellSize, height: cellSize)
                        } else {
                            let isSel = (cell.date == selectedDate)
                            RoundedRectangle(cornerRadius: 2.5, style: .continuous)
                                .fill(palette.heatColor(level: cell.level))
                                .frame(width: cellSize, height: cellSize)
                                .overlay(
                                    RoundedRectangle(cornerRadius: 2.5, style: .continuous)
                                        .stroke(isSel ? palette.accent : Color.clear, lineWidth: 1.5)
                                )
                                .onTapGesture {
                                    guard let rec = cell.record, let onSelectDate else { return }
                                    UIImpactFeedbackGenerator(style: .light).impactOccurred()
                                    onSelectDate(rec.date)
                                }
                        }
                    }
                }
            }
        }
    }
}

// MARK: - Interactive Swipeable Carousel Card

public struct TokenHistoryChartCarouselCard: View {
    public let matrix: [DailyTokenRecord]
    public let selectedDate: String?
    @Binding public var selectedChart: TokenHistoryChartType
    public let onSelectDate: (String) -> Void
    public let onUserSelectChart: (TokenHistoryChartType) -> Void
    public let onOpenAllModels: (() -> Void)?

    @Environment(\.colorScheme) private var colorScheme

    public init(
        matrix: [DailyTokenRecord],
        selectedDate: String?,
        selectedChart: Binding<TokenHistoryChartType>,
        onSelectDate: @escaping (String) -> Void,
        onUserSelectChart: @escaping (TokenHistoryChartType) -> Void,
        onOpenAllModels: (() -> Void)? = nil
    ) {
        self.matrix = matrix
        self.selectedDate = selectedDate
        self._selectedChart = selectedChart
        self.onSelectDate = onSelectDate
        self.onUserSelectChart = onUserSelectChart
        self.onOpenAllModels = onOpenAllModels
    }

    public var body: some View {
        let palette = TokenHistoryChartPalette.app(colorScheme: colorScheme)

        VStack(spacing: 8) {
            TabView(selection: $selectedChart) {
                TokenDailyBarsChartView(
                    matrix: matrix,
                    selectedDate: selectedDate,
                    palette: palette,
                    onSelectDate: onSelectDate
                )
                .padding(.horizontal, 12)
                .tag(TokenHistoryChartType.dailyBars)

                TokenModelBreakdownChartView(
                    matrix: matrix,
                    palette: palette,
                    onOpenAllModels: onOpenAllModels
                )
                .padding(.horizontal, 12)
                .tag(TokenHistoryChartType.modelBreakdown)

                TokenHeatmapGridChartView(
                    matrix: matrix,
                    selectedDate: selectedDate,
                    palette: palette,
                    fixedWidth: nil,
                    onSelectDate: onSelectDate
                )
                .padding(.horizontal, 12)
                .tag(TokenHistoryChartType.heatmap)
            }
            .tabViewStyle(.page(indexDisplayMode: .never))
            .frame(height: 140)

            // 极简 3 圆点指示器（支持点击切换，也可直观提示可左右滑动）
            HStack(spacing: 6) {
                ForEach(TokenHistoryChartType.allCases) { chart in
                    let isActive = (chart == selectedChart)
                    Capsule()
                        .fill(isActive ? Color.primary.opacity(0.65) : Color.secondary.opacity(0.28))
                        .frame(width: isActive ? 16 : 5, height: 5)
                        .animation(.snappy(duration: 0.22), value: selectedChart)
                        .contentShape(Rectangle().inset(by: -6))
                        .onTapGesture {
                            guard chart != selectedChart else { return }
                            UIImpactFeedbackGenerator(style: .light).impactOccurred()
                            withAnimation(.snappy(duration: 0.25)) {
                                selectedChart = chart
                            }
                            onUserSelectChart(chart)
                        }
                }
            }
            .padding(.bottom, 2)
        }
        .padding(.vertical, 12)
        .background(Color(uiColor: .secondarySystemGroupedBackground))
        .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
    }
}
