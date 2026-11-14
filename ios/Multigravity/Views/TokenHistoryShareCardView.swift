import SwiftUI

/// 专为 Token 历史设计的长图海报渲染视图（支持 iOS ImageRenderer 离屏导出）
public struct TokenHistoryShareCardView: View {
    public let history: TokenHistoryResponse
    public let selectedRecord: DailyTokenRecord?
    public let colorScheme: ColorScheme
    public let exportDate: Date

    public static let designWidth: CGFloat = 390

    public init(
        history: TokenHistoryResponse,
        selectedRecord: DailyTokenRecord? = nil,
        colorScheme: ColorScheme = .dark,
        exportDate: Date = Date()
    ) {
        self.history = history
        self.selectedRecord = selectedRecord
        self.colorScheme = colorScheme
        self.exportDate = exportDate
    }

    private var isDark: Bool {
        colorScheme == .dark
    }

    private var backgroundColor: Color {
        isDark ? Color(red: 0.08, green: 0.09, blue: 0.13) : Color(red: 0.95, green: 0.96, blue: 0.98)
    }

    private var cardBackground: Color {
        isDark ? Color(red: 0.13, green: 0.14, blue: 0.20) : Color.white
    }

    private var cardBorderColor: Color {
        isDark ? Color.white.opacity(0.08) : Color.black.opacity(0.06)
    }

    private var primaryTextColor: Color {
        isDark ? Color.white : Color(red: 0.10, green: 0.11, blue: 0.14)
    }

    private var secondaryTextColor: Color {
        isDark ? Color.white.opacity(0.65) : Color(red: 0.45, green: 0.47, blue: 0.52)
    }

    private var accentBlue: Color {
        Color.blue
    }

    public var body: some View {
        VStack(spacing: 14) {
            header
            kpiGrid
            sourceRatioBar
            heatmapCard
            if let record = selectedRecord {
                inspectorCard(record: record)
            }
            footer
        }
        .padding(16)
        .frame(width: Self.designWidth)
        .background(backgroundColor)
        .environment(\.colorScheme, colorScheme)
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
            kpiCell(title: "今日消耗", value: AccountQuotaSheet.formatTokensCompact(s.todayTokens), sub: "\(s.activeDays) 活跃日")
            kpiCell(title: "近 7 日", value: AccountQuotaSheet.formatTokensCompact(s.last7dTokens), sub: "\(s.totalSteps) 轮采样")
            kpiCell(title: "累计总 Token", value: AccountQuotaSheet.formatTokensCompact(s.totalTokens), sub: "IDE \(AccountQuotaSheet.formatTokensCompact(s.desktopTokens)) · CLI \(AccountQuotaSheet.formatTokensCompact(s.cliTokens))")
            kpiCell(title: "API 折算", value: String(format: "$%.2f", s.estimatedCostUsd), sub: String(format: "≈ ¥%.1f", s.estimatedCostCny))
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

    // MARK: - Source Ratio Bar
    private var sourceRatioBar: some View {
        let s = history.summary
        let totalSrc = max(Int64(1), s.desktopTokens + s.cliTokens)
        let desktopPct = Int((Double(s.desktopTokens) * 100.0) / Double(totalSrc))
        let cliPct = s.cliTokens > 0 ? (100 - desktopPct) : 0

        return VStack(spacing: 6) {
            HStack {
                Text("桌面端 Antigravity \(desktopPct)% · CLI \(cliPct)%")
                    .font(.system(size: 11.5, weight: .medium))
                    .foregroundColor(secondaryTextColor)
                Spacer()
                Text("近 18 周活跃热力")
                    .font(.system(size: 10.5))
                    .foregroundColor(secondaryTextColor.opacity(0.8))
            }

            GeometryReader { geo in
                ZStack(alignment: .leading) {
                    RoundedRectangle(cornerRadius: 3)
                        .fill(Color.purple.opacity(0.6))
                        .frame(height: 6)
                    RoundedRectangle(cornerRadius: 3)
                        .fill(Color.blue)
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

    // MARK: - Heatmap Card (最近 18 周)
    private var heatmapCard: some View {
        let matrix = history.dailyMatrix
        let maxTotal = max(Int64(1), matrix.map(\.total).max() ?? 1)
        let allWeeks = buildHeatmapWeeks(matrix)
        let displayWeeks = Array(allWeeks.suffix(18))

        return VStack(alignment: .leading, spacing: 8) {
            HStack {
                Text("每日活跃密度")
                    .font(.system(size: 12, weight: .semibold))
                    .foregroundColor(primaryTextColor)
                Spacer()
                HStack(spacing: 3) {
                    Text("少").font(.system(size: 10)).foregroundColor(secondaryTextColor)
                    ForEach(0..<5, id: \.self) { lvl in
                        RoundedRectangle(cornerRadius: 2)
                            .fill(heatColor(level: lvl))
                            .frame(width: 8, height: 8)
                    }
                    Text("多").font(.system(size: 10)).foregroundColor(secondaryTextColor)
                }
            }

            // 平铺 18 周，无需 ScrollView，适应长图海报（左对齐，与 App 内部显示保持一致）
            HStack(spacing: 3) {
                ForEach(Array(displayWeeks.enumerated()), id: \.offset) { _, week in
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
                                let isSel = (rec.date == selectedRecord?.date)
                                RoundedRectangle(cornerRadius: 2.5)
                                    .fill(heatColor(level: level))
                                    .frame(width: 12, height: 12)
                                    .overlay(
                                        RoundedRectangle(cornerRadius: 2.5)
                                            .stroke(isSel ? Color.blue : Color.clear, lineWidth: 1.5)
                                    )
                            } else {
                                Color.clear.frame(width: 12, height: 12)
                            }
                        }
                    }
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(.vertical, 2)
        }
        .padding(12)
        .background(cardBackground)
        .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: 12, style: .continuous).stroke(cardBorderColor, lineWidth: 0.8))
    }

    // MARK: - Inspector Card
    private func inspectorCard(record: DailyTokenRecord) -> some View {
        let topModels = record.models.sorted { $0.value > $1.value }.prefix(4)
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
                inspectorCell("Input", AccountQuotaSheet.formatTokensCompact(record.input))
                Spacer()
                inspectorCell("Cache Hit", AccountQuotaSheet.formatTokensCompact(record.cacheRead))
                Spacer()
                inspectorCell("Output", AccountQuotaSheet.formatTokensCompact(record.output))
                Spacer()
                inspectorCell("Thinking", AccountQuotaSheet.formatTokensCompact(record.thinking))
            }
            if !topModels.isEmpty {
                Divider().overlay(cardBorderColor)
                VStack(spacing: 4) {
                    ForEach(Array(topModels), id: \.key) { pair in
                        HStack {
                            Text(pair.key)
                                .font(.system(size: 11))
                                .foregroundColor(secondaryTextColor)
                                .lineLimit(1)
                            Spacer()
                            Text(AccountQuotaSheet.formatTokensCompact(pair.value))
                                .font(.system(size: 11, design: .monospaced))
                                .foregroundColor(primaryTextColor)
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
                let scanDate = Date(timeIntervalSince1970: Double(history.updatedAt))
                Text("本地数据扫描于 \(Self.scanTimeFormatter.string(from: scanDate))")
                    .font(.system(size: 9.5))
                    .foregroundColor(secondaryTextColor.opacity(0.6))
            }
        }
        .padding(.horizontal, 4)
        .padding(.top, 2)
    }

    // MARK: - Helpers
    private func buildHeatmapWeeks(_ matrix: [DailyTokenRecord]) -> [[DailyTokenRecord?]] {
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

    private func heatColor(level: Int) -> Color {
        switch level {
        case 0:
            return isDark ? Color.white.opacity(0.08) : Color.black.opacity(0.06)
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

    private static let dateFormatter: DateFormatter = {
        let f = DateFormatter()
        f.dateFormat = "yyyy-MM-dd HH:mm"
        return f
    }()

    private static let scanTimeFormatter: DateFormatter = {
        let f = DateFormatter()
        f.dateFormat = "HH:mm:ss"
        return f
    }()
}
