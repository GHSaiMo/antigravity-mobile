import SwiftUI

extension MarkdownContentView {
    // MARK: - Subviews
    
    @ViewBuilder
    func headingView(level: Int, text: String) -> some View {
        let baseSize: CGFloat = {
            switch level {
            case 1: return 20
            case 2: return 18
            case 3: return 16
            case 4: return 15
            default: return 14
            }
        }()
        let size = baseSize * (isShareExport ? 1.0 : AppSettings.shared.fontSizeScale.bodyScale)
        
        Self.renderRichText(text, size: size, weight: .bold)
            .font(.system(size: size, weight: .bold))
            .foregroundColor(.primary)
            .padding(.top, level <= 2 ? 6 : 2)
            .padding(.bottom, 2)
    }
    
    @ViewBuilder
    func codeBlockView(lang: String, code: String) -> some View {
        let info = FileIconResolver.resolve(for: lang)
        
        return VStack(alignment: .leading, spacing: 0) {
            // Header bar
            HStack(spacing: 8) {
                HStack(spacing: 6) {
                    Circle()
                        .fill(info.color)
                        .frame(width: 7, height: 7)
                    if !info.glyph.isEmpty {
                        Text(info.glyph)
                            .font(.system(size: 11))
                    }
                    Text(info.displayName)
                        .font(.system(size: 11, weight: .bold, design: .monospaced))
                        .foregroundColor(info.color)
                }
                
                Spacer()
                
                if !isShareExport {
                Button(action: {
                    let formatted = code.hasSuffix("\n") ? code : "\(code)\n"
                    UIPasteboard.general.string = formatted
                    UIImpactFeedbackGenerator(style: .light).impactOccurred()
                    CopiedHUD.show("已复制代码")
                }) {
                    HStack(spacing: 4) {
                        Image(systemName: "doc.on.doc")
                            .font(.system(size: 10))
                        Text("复制")
                            .font(.system(size: 11))
                    }
                    .foregroundColor(.secondary)
                }
                }
            }
            .padding(.horizontal, 12)
            .padding(.vertical, 6)
            .background(Color(uiColor: .tertiarySystemBackground).opacity(0.8))
            
            Divider()
            
            // Code text
            let codeScale = isShareExport ? 1.0 : AppSettings.shared.fontSizeScale.codeScale
            ExportAwareHorizontalScroll {
                Text(code)
                    .font(.system(size: 12.5 * codeScale, design: .monospaced))
                    .padding(10)
            }
            .fixedSize(horizontal: false, vertical: true)
        }
        .background(Color(uiColor: .tertiarySystemBackground).opacity(0.5))
        .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
        .overlay(
            RoundedRectangle(cornerRadius: 10, style: .continuous)
                .stroke(Color.secondary.opacity(0.2), lineWidth: 1)
        )
    }
    
    @ViewBuilder
    func tableView(headers: [String], rows: [[String]], alignments: [TableColumnAlignment] = []) -> some View {
        let columnCount = max(headers.count, rows.map(\.count).max() ?? 0)
        
        if columnCount > 0 {
            MarkdownAutoTable(columnCount: columnCount, hasHeader: !headers.isEmpty) {
                // Header row
                if !headers.isEmpty {
                    ForEach(0..<columnCount, id: \.self) { colIdx in
                        let colAlignment = colIdx < alignments.count ? alignments[colIdx] : .leading
                        Self.renderRichText(colIdx < headers.count ? headers[colIdx] : "", size: 13, weight: .bold)
                            .font(.system(size: 13, weight: .bold))
                            .foregroundColor(.primary)
                            .multilineTextAlignment(colAlignment.textAlignment)
                            .padding(.horizontal, 12)
                            .padding(.vertical, 8)
                            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: colAlignment.swiftUIAlignment)
                            .background(Color(uiColor: .tertiarySystemBackground))
                            .overlay(alignment: .trailing) {
                                if colIdx < columnCount - 1 {
                                    Rectangle().fill(Color.secondary.opacity(0.2)).frame(width: 0.5)
                                }
                            }
                            .overlay(alignment: .bottom) {
                                Rectangle().fill(Color.secondary.opacity(0.3)).frame(height: 0.5)
                            }
                    }
                }
                
                // Data rows
                ForEach(Array(rows.enumerated()), id: \.offset) { rowIdx, row in
                    ForEach(0..<columnCount, id: \.self) { colIdx in
                        let colAlignment = colIdx < alignments.count ? alignments[colIdx] : .leading
                        Self.renderRichText(colIdx < row.count ? row[colIdx] : "", size: 13)
                            .font(.system(size: 13))
                            .foregroundColor(.primary.opacity(0.9))
                            .multilineTextAlignment(colAlignment.textAlignment)
                            .lineSpacing(2.5)
                            .padding(.horizontal, 12)
                            .padding(.vertical, 8)
                            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: colAlignment.swiftUIAlignment)
                            .background(rowIdx % 2 == 0 ? Color.clear : Color(uiColor: .tertiarySystemBackground).opacity(0.3))
                            .overlay(alignment: .trailing) {
                                if colIdx < columnCount - 1 {
                                    Rectangle().fill(Color.secondary.opacity(0.15)).frame(width: 0.5)
                                }
                            }
                            .overlay(alignment: .bottom) {
                                if rowIdx < rows.count - 1 {
                                    Rectangle().fill(Color.secondary.opacity(0.15)).frame(height: 0.5)
                                }
                            }
                    }
                }
            }
            .padding(.vertical, 4)
        }
    }
    
    private static func parseTaskListItem(_ raw: String) -> (marker: String?, isChecked: Bool, content: String) {
        if raw.hasPrefix("[ ] ") {
            return ("☐", false, String(raw.dropFirst(4)))
        } else if raw.hasPrefix("[x] ") || raw.hasPrefix("[X] ") {
            return ("☑", true, String(raw.dropFirst(4)))
        } else if raw.hasPrefix("[/] ") || raw.hasPrefix("[-] ") {
            return ("◐", false, String(raw.dropFirst(4)))
        }
        return (nil, false, raw)
    }
    
    @ViewBuilder
    func listView(items: [String]) -> some View {
        let bodyScale = isShareExport ? 1.0 : AppSettings.shared.fontSizeScale.bodyScale
        VStack(alignment: .leading, spacing: 4) {
            ForEach(Array(items.enumerated()), id: \.offset) { _, item in
                let parsed = Self.parseTaskListItem(item)
                HStack(alignment: .top, spacing: 6) {
                    Text(parsed.marker ?? "•")
                        .font(.system(size: (parsed.marker != nil ? 14.5 : 14) * bodyScale, weight: .bold))
                        .foregroundColor(parsed.isChecked ? .blue : .secondary)
                        .padding(.top, 2)
                    paragraphView(text: parsed.content, size: 15 * bodyScale)
                        .frame(maxWidth: .infinity, alignment: .leading)
                }
            }
        }
    }
    
    @ViewBuilder
    func orderedListView(startIndex: Int, items: [String]) -> some View {
        let bodyScale = isShareExport ? 1.0 : AppSettings.shared.fontSizeScale.bodyScale
        VStack(alignment: .leading, spacing: 4) {
            ForEach(Array(items.enumerated()), id: \.offset) { idx, item in
                let parsed = Self.parseTaskListItem(item)
                let displayContent = parsed.marker != nil ? "\(parsed.marker!) \(parsed.content)" : item
                HStack(alignment: .top, spacing: 6) {
                    Text("\(startIndex + idx).")
                        .font(.system(size: 13.5 * bodyScale, weight: .semibold, design: .monospaced))
                        .foregroundColor(.secondary)
                        .padding(.top, 2)
                    paragraphView(text: displayContent, size: 15 * bodyScale)
                        .frame(maxWidth: .infinity, alignment: .leading)
                }
            }
        }
    }
    
    @ViewBuilder
    func alertView(type: MarkdownAlertType, content: String) -> some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack(spacing: 6) {
                Image(systemName: type.systemImage)
                    .font(.system(size: 13, weight: .semibold))
                Text(type.title)
                    .font(.system(size: 12.5, weight: .bold))
            }
            .foregroundColor(type.accentColor)
            
            if !content.isEmpty {
                MarkdownContentView(content: content, onImageTap: onImageTap)
            }
        }
        .padding(.leading, 12)
        .padding(.trailing, 10)
        .padding(.vertical, 9)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(type.accentColor.opacity(0.08))
        .overlay(alignment: .leading) {
            Rectangle()
                .fill(type.accentColor)
                .frame(width: 3.5)
        }
        .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
    }
    
    @ViewBuilder
    func blockquoteView(content: String) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            if !content.isEmpty {
                MarkdownContentView(content: content, onImageTap: onImageTap)
            }
        }
        .padding(.leading, 12)
        .padding(.trailing, 10)
        .padding(.vertical, 8)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(Color.secondary.opacity(0.06))
        .overlay(alignment: .leading) {
            Rectangle()
                .fill(Color.secondary.opacity(0.4))
                .frame(width: 3.5)
        }
        .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
    }
}


/// 表格：按内容自适应列宽并支持横向滚动，不强行压缩列宽。
/// 短列保持单行完整显示；长文本列按舒适阅读宽度自动换行，总宽超出容器时交由外层横向滚动。
struct MarkdownAutoTable<Content: View>: View {
    @Environment(\.isShareExport) private var isShareExport
    let columnCount: Int
    let hasHeader: Bool
    @ViewBuilder let content: () -> Content
    @State private var availableWidth: CGFloat = 0

    var body: some View {
        VStack(spacing: 0) {
            Color.clear
                .frame(height: 0)
                .background(GeometryReader { geo in
                    Color.clear.preference(key: TableWidthKey.self, value: geo.size.width)
                })
            ExportAwareHorizontalScroll {
                AutoTableLayout(
                    columnCount: columnCount,
                    hasHeader: hasHeader,
                    availableWidth: availableWidth,
                    isShareExport: isShareExport
                ) {
                    content()
                }
                .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
                .overlay(
                    RoundedRectangle(cornerRadius: 10, style: .continuous)
                        .stroke(Color.secondary.opacity(0.25), lineWidth: 1)
                )
            }
            .fixedSize(horizontal: false, vertical: true)
        }
        .onPreferenceChange(TableWidthKey.self) { availableWidth = $0 }
    }
}

private struct TableWidthKey: PreferenceKey {
    static var defaultValue: CGFloat = 0
    static func reduce(value: inout CGFloat, nextValue: () -> CGFloat) { value = max(value, nextValue()) }
}

private struct AutoTableLayout: Layout {
    let columnCount: Int
    let hasHeader: Bool
    let availableWidth: CGFloat
    let isShareExport: Bool

    private func defaultCap(for column: Int, limit: CGFloat) -> CGFloat {
        switch columnCount {
        case 1:
            return max(limit, 260)
        case 2:
            return column == 0 ? 180 : 320
        default:
            return 260
        }
    }

    private func columnWidths(_ subviews: Subviews, limit: CGFloat) -> [CGFloat] {
        var maxW = [CGFloat](repeating: 44, count: columnCount)
        var headerW = [CGFloat](repeating: 0, count: columnCount)

        for (i, v) in subviews.enumerated() {
            let c = i % columnCount
            let singleW = ceil(v.sizeThatFits(.unspecified).width)
            maxW[c] = max(maxW[c], singleW)
            if hasHeader && i < columnCount {
                headerW[c] = singleW
            }
        }

        let sumMax = maxW.reduce(0, +)
        if limit > 0 && sumMax <= limit {
            return maxW
        }

        var widths = (0..<columnCount).map { c -> CGFloat in
            let cap = max(defaultCap(for: c, limit: limit), min(headerW[c], 280))
            return min(maxW[c], cap)
        }

        let sumWidths = widths.reduce(0, +)
        if limit > 0 && sumWidths < limit {
            let extraNeeded = (0..<columnCount).reduce(CGFloat(0)) { $0 + (maxW[$1] - widths[$1]) }
            if extraNeeded > 0 {
                let grow = min(1.0, (limit - sumWidths) / extraNeeded)
                for c in 0..<columnCount where maxW[c] > widths[c] {
                    widths[c] = floor(widths[c] + (maxW[c] - widths[c]) * grow)
                }
            }
        } else if isShareExport && limit > 0 && sumWidths > limit {
            let floors = (0..<columnCount).map { c in
                min(maxW[c], max(min(headerW[c], 120), 64))
            }
            let sumFloors = floors.reduce(0, +)
            if sumFloors >= limit {
                return floors
            }
            let t = (limit - sumFloors) / (sumWidths - sumFloors)
            widths = (0..<columnCount).map { c in
                floor(floors[c] + (widths[c] - floors[c]) * t)
            }
        }

        return widths
    }

    private func containerLimit(for proposal: ProposedViewSize) -> CGFloat {
        if isShareExport, let w = proposal.width, w.isFinite, w > 0 {
            return w
        }
        if availableWidth > 0 {
            return availableWidth
        }
        if let w = proposal.width, w.isFinite, w > 0 {
            return w
        }
        return max(UIScreen.main.bounds.width - 32, 280)
    }

    private func rowHeights(_ subviews: Subviews, widths: [CGFloat]) -> [CGFloat] {
        let rows = (subviews.count + columnCount - 1) / columnCount
        return (0..<rows).map { r in
            (0..<columnCount).compactMap { c -> CGFloat? in
                let i = r * columnCount + c
                guard i < subviews.count else { return nil }
                return ceil(subviews[i].sizeThatFits(ProposedViewSize(width: widths[c], height: nil)).height)
            }.max() ?? 0
        }
    }

    func sizeThatFits(proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) -> CGSize {
        guard columnCount > 0, !subviews.isEmpty else { return .zero }
        let widths = columnWidths(subviews, limit: containerLimit(for: proposal))
        return CGSize(width: widths.reduce(0, +), height: rowHeights(subviews, widths: widths).reduce(0, +))
    }

    func placeSubviews(in bounds: CGRect, proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) {
        guard columnCount > 0, !subviews.isEmpty else { return }
        let widths = columnWidths(subviews, limit: containerLimit(for: proposal))
        let heights = rowHeights(subviews, widths: widths)
        var y = bounds.minY
        for (r, h) in heights.enumerated() {
            var x = bounds.minX
            for c in 0..<columnCount {
                let i = r * columnCount + c
                if i < subviews.count {
                    subviews[i].place(at: CGPoint(x: x, y: y), anchor: .topLeading,
                                      proposal: ProposedViewSize(width: widths[c], height: h))
                }
                x += widths[c]
            }
            y += h
        }
    }
}
