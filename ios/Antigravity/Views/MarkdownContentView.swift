import SwiftUI

public enum MarkdownBlock: Identifiable {
    case heading(id: String, level: Int, text: String)
    case divider(id: String)
    case codeBlock(id: String, lang: String, code: String)
    case table(id: String, headers: [String], rows: [[String]])
    case list(id: String, items: [String])
    case paragraph(id: String, text: String)
    
    public var id: String {
        switch self {
        case .heading(let id, _, _): return id
        case .divider(let id): return id
        case .codeBlock(let id, _, _): return id
        case .table(let id, _, _): return id
        case .list(let id, _): return id
        case .paragraph(let id, _): return id
        }
    }
}

public struct MarkdownContentView: View {
    public let content: String
    private let blocks: [MarkdownBlock]
    
    public init(content: String) {
        self.content = content
        self.blocks = MarkdownParser.parse(content)
    }
    
    public var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            ForEach(blocks) { block in
                switch block {
                case .heading(_, let level, let text):
                    headingView(level: level, text: text)
                    
                case .divider:
                    Divider()
                        .background(Color.secondary.opacity(0.3))
                        .padding(.vertical, 4)
                    
                case .codeBlock(_, let lang, let code):
                    codeBlockView(lang: lang, code: code)
                    
                case .table(_, let headers, let rows):
                    tableView(headers: headers, rows: rows)
                    
                case .list(_, let items):
                    listView(items: items)
                    
                case .paragraph(_, let text):
                    Self.renderRichText(text, size: 15)
                        .font(.system(size: 15))
                        .textSelection(.enabled)
                        .lineSpacing(3)
                        .fixedSize(horizontal: false, vertical: true)
                }
            }
        }
    }
    
    // MARK: - Rich Text with File Icons and Inline Code Styler
    
    public static func renderRichText(_ rawText: String, size: CGFloat = 15, weight: Font.Weight = .regular) -> Text {
        let text = MathSymbolProcessor.process(rawText)
        let pattern = #"(?<!\!)\[([^\]]+)\]\(([^)]+)\)"#
        guard let regex = try? NSRegularExpression(pattern: pattern) else {
            return Text(renderInlineMarkdown(text, size: size, weight: weight))
        }
        
        let nsText = text as NSString
        let matches = regex.matches(in: text, range: NSRange(location: 0, length: nsText.length))
        
        if matches.isEmpty {
            return Text(renderInlineMarkdown(text, size: size, weight: weight))
        }
        
        var combined = Text("")
        var lastEnd = 0
        
        for match in matches {
            let matchRange = match.range
            if matchRange.location > lastEnd {
                let leading = nsText.substring(with: NSRange(location: lastEnd, length: matchRange.location - lastEnd))
                combined = combined + Text(renderInlineMarkdown(leading, size: size, weight: weight))
            }
            
            let linkText = nsText.substring(with: match.range(at: 1))
            let linkUrl = nsText.substring(with: match.range(at: 2))
            let fullLinkMarkdown = nsText.substring(with: matchRange)
            
            if let icon = FileIconResolver.resolveIcon(for: linkText) ?? FileIconResolver.resolveIcon(for: linkUrl) {
                combined = combined + Text(Image(icon)) + Text("\u{2009}")
            }
            
            combined = combined + Text(renderInlineMarkdown(fullLinkMarkdown, size: size, weight: weight))
            lastEnd = matchRange.location + matchRange.length
        }
        
        if lastEnd < nsText.length {
            let trailing = nsText.substring(with: NSRange(location: lastEnd, length: nsText.length - lastEnd))
            combined = combined + Text(renderInlineMarkdown(trailing, size: size, weight: weight))
        }
        
        return combined
    }
    
    // MARK: - Markdown Attributed String Styler
    
    public static func renderInlineMarkdown(_ text: String, size: CGFloat = 15, weight: Font.Weight = .regular) -> AttributedString {
        var options = AttributedString.MarkdownParsingOptions()
        options.interpretedSyntax = .inlineOnlyPreservingWhitespace
        guard var attr = try? AttributedString(markdown: text, options: options) else {
            return AttributedString(text)
        }
        
        // Antigravity Desktop Code Amber/Yellow color: #E5C07B (RGB: 229, 192, 123)
        let codeFgColor = Color(red: 229/255, green: 192/255, blue: 123/255)
        
        for run in attr.runs {
            if let intent = run.inlinePresentationIntent, intent.contains(.code) {
                attr[run.range].foregroundColor = codeFgColor
                attr[run.range].font = .system(size: size * 0.9, weight: .medium, design: .monospaced)
            } else if run.link != nil {
                attr[run.range].font = .system(size: size * 0.9, weight: .medium, design: .monospaced)
            }
        }
        return attr
    }
    
    // MARK: - Subviews
    
    @ViewBuilder
    private func headingView(level: Int, text: String) -> some View {
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
            .textSelection(.enabled)
    }
    
    @ViewBuilder
    private func codeBlockView(lang: String, code: String) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            // Header bar
            HStack {
                Text(lang.isEmpty ? "CODE" : lang.uppercased())
                    .font(.system(size: 10.5, weight: .bold, design: .monospaced))
                    .foregroundColor(.secondary)
                
                Spacer()
                
                Button(action: {
                    UIPasteboard.general.string = code
                    UIImpactFeedbackGenerator(style: .light).impactOccurred()
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
                    .textSelection(.enabled)
                    .padding(10)
            }
        }
        .background(Color(uiColor: .tertiarySystemBackground).opacity(0.5))
        .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
        .overlay(
            RoundedRectangle(cornerRadius: 10, style: .continuous)
                .stroke(Color.secondary.opacity(0.2), lineWidth: 1)
        )
    }
    
    @ViewBuilder
    private func tableView(headers: [String], rows: [[String]]) -> some View {
        ScrollView(.horizontal, showsIndicators: true) {
            VStack(alignment: .leading, spacing: 0) {
                // Header row
                HStack(spacing: 0) {
                    ForEach(Array(headers.enumerated()), id: \.offset) { colIdx, header in
                        Self.renderRichText(header, size: 13, weight: .bold)
                            .font(.system(size: 13, weight: .bold))
                            .foregroundColor(.primary)
                            .padding(.horizontal, 12)
                            .padding(.vertical, 8)
                            .frame(minWidth: 100, alignment: .leading)
                        
                        if colIdx < headers.count - 1 {
                            Divider()
                                .frame(height: 20)
                        }
                    }
                }
                .background(Color(uiColor: .tertiarySystemBackground))
                
                Divider()
                
                // Data rows
                ForEach(Array(rows.enumerated()), id: \.offset) { rowIdx, row in
                    HStack(spacing: 0) {
                        ForEach(Array(headers.indices), id: \.self) { colIdx in
                            let cellText = colIdx < row.count ? row[colIdx] : ""
                            Self.renderRichText(cellText, size: 13)
                                .font(.system(size: 13))
                                .foregroundColor(.primary.opacity(0.9))
                                .padding(.horizontal, 12)
                                .padding(.vertical, 8)
                                .frame(minWidth: 100, alignment: .leading)
                                .textSelection(.enabled)
                            
                            if colIdx < headers.count - 1 {
                                Divider()
                                .frame(height: 20)
                            }
                        }
                    }
                    .background(rowIdx % 2 == 0 ? Color.clear : Color(uiColor: .tertiarySystemBackground).opacity(0.3))
                    
                    if rowIdx < rows.count - 1 {
                        Divider()
                            .background(Color.secondary.opacity(0.15))
                    }
                }
            }
            .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
            .overlay(
                RoundedRectangle(cornerRadius: 10, style: .continuous)
                    .stroke(Color.secondary.opacity(0.25), lineWidth: 1)
            )
        }
        .padding(.vertical, 4)
    }
    
    @ViewBuilder
    private func listView(items: [String]) -> some View {
        VStack(alignment: .leading, spacing: 4) {
            ForEach(Array(items.enumerated()), id: \.offset) { _, item in
                HStack(alignment: .top, spacing: 6) {
                    Text("•")
                        .font(.system(size: 14, weight: .bold))
                        .foregroundColor(.secondary)
                    Self.renderRichText(item, size: 15)
                        .font(.system(size: 15))
                        .textSelection(.enabled)
                        .lineSpacing(2)
                        .fixedSize(horizontal: false, vertical: true)
                }
            }
        }
    }
}

// MARK: - Markdown Parser

public enum MarkdownParser {
    public static func parse(_ text: String) -> [MarkdownBlock] {
        var blocks: [MarkdownBlock] = []
        let lines = text.components(separatedBy: "\n")
        var i = 0
        var blockIdx = 0
        
        while i < lines.count {
            let line = lines[i]
            let trimmed = line.trimmingCharacters(in: .whitespaces)
            
            if trimmed.isEmpty {
                i += 1
                continue
            }
            
            // Fenced code block
            if trimmed.hasPrefix("```") {
                let lang = String(trimmed.dropFirst(3)).trimmingCharacters(in: .whitespaces)
                var codeLines: [String] = []
                i += 1
                while i < lines.count {
                    if lines[i].trimmingCharacters(in: .whitespaces).hasPrefix("```") {
                        i += 1
                        break
                    }
                    codeLines.append(lines[i])
                    i += 1
                }
                blocks.append(.codeBlock(id: "block-\(blockIdx)", lang: lang, code: codeLines.joined(separator: "\n")))
                blockIdx += 1
                continue
            }
            
            // Divider
            if trimmed == "---" || trimmed == "***" || trimmed == "___" {
                blocks.append(.divider(id: "block-\(blockIdx)"))
                blockIdx += 1
                i += 1
                continue
            }
            
            // Headings
            if trimmed.hasPrefix("#") {
                var level = 0
                for ch in trimmed {
                    if ch == "#" { level += 1 } else { break }
                }
                if level <= 6 && trimmed.count > level && trimmed[trimmed.index(trimmed.startIndex, offsetBy: level)] == " " {
                    let hText = String(trimmed.dropFirst(level + 1)).trimmingCharacters(in: .whitespaces)
                    blocks.append(.heading(id: "block-\(blockIdx)", level: level, text: hText))
                    blockIdx += 1
                    i += 1
                    continue
                }
            }
            
            // Table
            if trimmed.hasPrefix("|") && trimmed.hasSuffix("|") && trimmed.contains("|") {
                var tableLines: [String] = []
                while i < lines.count {
                    let tLine = lines[i].trimmingCharacters(in: .whitespaces)
                    if tLine.hasPrefix("|") && tLine.hasSuffix("|") {
                        tableLines.append(tLine)
                        i += 1
                    } else {
                        break
                    }
                }
                if tableLines.count >= 2 {
                    let parseRow = { (rowStr: String) -> [String] in
                        let parts = rowStr.split(separator: "|", omittingEmptySubsequences: false)
                        guard parts.count >= 2 else { return [] }
                        let inner = parts[1..<(parts.count - 1)]
                        return inner.map { String($0).trimmingCharacters(in: .whitespaces) }
                    }
                    let headers = parseRow(tableLines[0])
                    var rows: [[String]] = []
                    for rowIdx in 1..<tableLines.count {
                        let r = parseRow(tableLines[rowIdx])
                        // Skip separator row (| --- | --- |)
                        if r.allSatisfy({ $0.allSatisfy({ $0 == "-" || $0 == ":" || $0.isWhitespace }) }) {
                            continue
                        }
                        rows.append(r)
                    }
                    blocks.append(.table(id: "block-\(blockIdx)", headers: headers, rows: rows))
                    blockIdx += 1
                    continue
                }
            }
            
            // Bullet or list
            if trimmed.hasPrefix("- ") || trimmed.hasPrefix("* ") || trimmed.hasPrefix("• ") {
                var listItems: [String] = []
                while i < lines.count {
                    let lLine = lines[i].trimmingCharacters(in: .whitespaces)
                    if lLine.hasPrefix("- ") || lLine.hasPrefix("* ") || lLine.hasPrefix("• ") {
                        listItems.append(String(lLine.dropFirst(2)).trimmingCharacters(in: .whitespaces))
                        i += 1
                    } else {
                        break
                    }
                }
                blocks.append(.list(id: "block-\(blockIdx)", items: listItems))
                blockIdx += 1
                continue
            }
            
            // Paragraph
            var paraLines: [String] = [line]
            i += 1
            while i < lines.count {
                let nextLine = lines[i]
                let nTrimmed = nextLine.trimmingCharacters(in: .whitespaces)
                if nTrimmed.isEmpty || nTrimmed.hasPrefix("```") || nTrimmed.hasPrefix("#") || nTrimmed == "---" || (nTrimmed.hasPrefix("|") && nTrimmed.hasSuffix("|")) || nTrimmed.hasPrefix("- ") || nTrimmed.hasPrefix("* ") {
                    break
                }
                paraLines.append(nextLine)
                i += 1
            }
            blocks.append(.paragraph(id: "block-\(blockIdx)", text: paraLines.joined(separator: "\n")))
            blockIdx += 1
        }
        
        return blocks
    }
}
