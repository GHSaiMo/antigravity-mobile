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
            .padding(.horizontal, 12)
            .padding(.vertical, 6)
            .background(Color(uiColor: .tertiarySystemBackground).opacity(0.8))
            
            Divider()
            
            // Code text
            ScrollView(.horizontal, showsIndicators: true) {
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
            ScrollView(.horizontal, showsIndicators: true) {
                Grid(alignment: .leading, horizontalSpacing: 0, verticalSpacing: 0) {
                    // Header row
                    if !headers.isEmpty {
                        GridRow {
                            ForEach(0..<columnCount, id: \.self) { colIdx in
                                let headerText = colIdx < headers.count ? headers[colIdx] : ""
                                let colAlignment = colIdx < alignments.count ? alignments[colIdx] : .leading
                                let align = colAlignment.swiftUIAlignment
                                let textAlignment = colAlignment.textAlignment
                                
                                Self.renderRichText(headerText, size: 13, weight: .bold)
                                    .font(.system(size: 13, weight: .bold))
                                    .foregroundColor(.primary)
                                    .multilineTextAlignment(textAlignment)
                                    .padding(.horizontal, 12)
                                    .padding(.vertical, 8)
                                    .frame(minWidth: 80, maxWidth: .infinity, maxHeight: .infinity, alignment: align)
                                    .background(Color(uiColor: .tertiarySystemBackground))
                                    .overlay(alignment: .trailing) {
                                        if colIdx < columnCount - 1 {
                                            Rectangle()
                                                .fill(Color.secondary.opacity(0.2))
                                                .frame(width: 0.5)
                                        }
                                    }
                            }
                        }
                        
                        Rectangle()
                            .fill(Color.secondary.opacity(0.3))
                            .frame(height: 0.5)
                    }
                    
                    // Data rows
                    ForEach(Array(rows.enumerated()), id: \.offset) { rowIdx, row in
                        GridRow {
                            ForEach(0..<columnCount, id: \.self) { colIdx in
                                let cellText = colIdx < row.count ? row[colIdx] : ""
                                let colAlignment = colIdx < alignments.count ? alignments[colIdx] : .leading
                                let align = colAlignment.swiftUIAlignment
                                let textAlignment = colAlignment.textAlignment
                                let isEven = rowIdx % 2 == 0
                                
                                Self.renderRichText(cellText, size: 13)
                                    .font(.system(size: 13))
                                    .foregroundColor(.primary.opacity(0.9))
                                    .multilineTextAlignment(textAlignment)
                                    .lineSpacing(2.5)
                                    .padding(.horizontal, 12)
                                    .padding(.vertical, 8)
                                    .frame(minWidth: 80, maxWidth: .infinity, maxHeight: .infinity, alignment: align)
                                    .background(isEven ? Color.clear : Color(uiColor: .tertiarySystemBackground).opacity(0.3))
                                    .overlay(alignment: .trailing) {
                                        if colIdx < columnCount - 1 {
                                            Rectangle()
                                                .fill(Color.secondary.opacity(0.15))
                                                .frame(width: 0.5)
                                        }
                                    }
                            }
                        }
                        
                        if rowIdx < rows.count - 1 {
                            Rectangle()
                                .fill(Color.secondary.opacity(0.15))
                                .frame(height: 0.5)
                        }
                    }
                }
                .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
                .overlay(
                    RoundedRectangle(cornerRadius: 10, style: .continuous)
                        .stroke(Color.secondary.opacity(0.25), lineWidth: 1)
                )
                .fixedSize(horizontal: false, vertical: true)
            }
            .fixedSize(horizontal: false, vertical: true)
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
