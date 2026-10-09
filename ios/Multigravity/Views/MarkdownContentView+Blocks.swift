import SwiftUI

extension MarkdownContentView {
    // MARK: - Subviews
    
    @ViewBuilder
    func headingView(level: Int, text: String) -> some View {
        let size: CGFloat = {
            switch level {
            case 1: return 20
            case 2: return 18
            case 3: return 16
            case 4: return 15
            default: return 14
            }
        }()
        
        Self.renderRichText(text, size: size, weight: .bold)
            .font(.system(size: size, weight: .bold))
            .foregroundColor(.primary)
            .padding(.top, level <= 2 ? 6 : 2)
            .padding(.bottom, 2)
    }
    
    @ViewBuilder
    func codeBlockView(lang: String, code: String) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            // Header bar
            HStack {
                Text(lang.isEmpty ? "CODE" : lang.uppercased())
                    .font(.system(size: 10.5, weight: .bold, design: .monospaced))
                    .foregroundColor(.secondary)
                
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
            ExportAwareHorizontalScroll {
                Text(code)
                    .font(.system(size: 12.5, design: .monospaced))
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
    
    @ViewBuilder
    func listView(items: [String]) -> some View {
        VStack(alignment: .leading, spacing: 4) {
            ForEach(Array(items.enumerated()), id: \.offset) { _, item in
                HStack(alignment: .top, spacing: 6) {
                    Text("•")
                        .font(.system(size: 14, weight: .bold))
                        .foregroundColor(.secondary)
                        .padding(.top, 2)
                    paragraphView(text: item, size: 15)
                        .frame(maxWidth: .infinity, alignment: .leading)
                }
            }
        }
    }
    
    @ViewBuilder
    func orderedListView(startIndex: Int, items: [String]) -> some View {
        VStack(alignment: .leading, spacing: 4) {
            ForEach(Array(items.enumerated()), id: \.offset) { idx, item in
                HStack(alignment: .top, spacing: 6) {
                    Text("\(startIndex + idx).")
                        .font(.system(size: 13.5, weight: .semibold, design: .monospaced))
                        .foregroundColor(.secondary)
                        .padding(.top, 2)
                    paragraphView(text: item, size: 15)
                        .frame(maxWidth: .infinity, alignment: .leading)
                }
            }
        }
    }
    
}


/// 表格：按内容自适应列宽，在能完整显示文字的前提下占用面积尽量小。
/// 容器装得下时，每列取内容的单行宽度；装不下时在「最长单词宽度」与「单行宽度」之间按比例分配；
/// 即使每列都取最小宽度仍超出容器，则保持最小宽度并横向滚动。
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
                AutoTableLayout(columnCount: columnCount, availableWidth: availableWidth) {
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
    let availableWidth: CGFloat

    private func columnWidths(_ subviews: Subviews, limit: CGFloat?) -> [CGFloat] {
        var minW = [CGFloat](repeating: 0, count: columnCount)
        var maxW = [CGFloat](repeating: 0, count: columnCount)
        for (i, v) in subviews.enumerated() {
            let c = i % columnCount
            minW[c] = max(minW[c], v.sizeThatFits(ProposedViewSize(width: 0, height: nil)).width)
            maxW[c] = max(maxW[c], v.sizeThatFits(.unspecified).width)
        }
        for c in 0..<columnCount { maxW[c] = max(maxW[c], minW[c]) }
        let sumMin = minW.reduce(0, +)
        let sumMax = maxW.reduce(0, +)
        guard let limit, limit > 0 else { return maxW }
        if sumMax <= limit { return maxW }
        if sumMin >= limit { return minW }
        let t = (limit - sumMin) / (sumMax - sumMin)
        return (0..<columnCount).map { minW[$0] + (maxW[$0] - minW[$0]) * t }
    }

    private func limit(for proposal: ProposedViewSize) -> CGFloat? {
        if let w = proposal.width, w.isFinite, w > 0 { return w }
        return availableWidth > 0 ? availableWidth : nil
    }

    private func rowHeights(_ subviews: Subviews, widths: [CGFloat]) -> [CGFloat] {
        let rows = (subviews.count + columnCount - 1) / columnCount
        return (0..<rows).map { r in
            (0..<columnCount).compactMap { c -> CGFloat? in
                let i = r * columnCount + c
                guard i < subviews.count else { return nil }
                return subviews[i].sizeThatFits(ProposedViewSize(width: widths[c], height: nil)).height
            }.max() ?? 0
        }
    }

    func sizeThatFits(proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) -> CGSize {
        guard columnCount > 0, !subviews.isEmpty else { return .zero }
        let widths = columnWidths(subviews, limit: limit(for: proposal))
        return CGSize(width: widths.reduce(0, +), height: rowHeights(subviews, widths: widths).reduce(0, +))
    }

    func placeSubviews(in bounds: CGRect, proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) {
        guard columnCount > 0, !subviews.isEmpty else { return }
        let widths = columnWidths(subviews, limit: bounds.width)
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
