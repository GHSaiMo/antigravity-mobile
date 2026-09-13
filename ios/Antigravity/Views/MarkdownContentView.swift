import SwiftUI

public enum TableColumnAlignment: Sendable, Equatable {
    case leading
    case center
    case trailing
    
    public var swiftUIAlignment: Alignment {
        switch self {
        case .leading: return .leading
        case .center: return .center
        case .trailing: return .trailing
        }
    }
}

public enum MarkdownBlock: Identifiable {
    case frontmatter(id: String, rawContent: String, lineCount: Int)
    case heading(id: String, level: Int, text: String)
    case divider(id: String)
    case codeBlock(id: String, lang: String, code: String)
    case table(id: String, headers: [String], rows: [[String]], alignments: [TableColumnAlignment] = [])
    case list(id: String, items: [String])
    case paragraph(id: String, text: String)
    
    public var id: String {
        switch self {
        case .frontmatter(let id, _, _): return id
        case .heading(let id, _, _): return id
        case .divider(let id): return id
        case .codeBlock(let id, _, _): return id
        case .table(let id, _, _, _): return id
        case .list(let id, _): return id
        case .paragraph(let id, _): return id
        }
    }
}

private final class MarkdownBlockCache: @unchecked Sendable {
    static let shared = MarkdownBlockCache()
    private let lock = NSLock()
    private var storage: [Int: [MarkdownBlock]] = [:]
    
    func blocks(for text: String) -> [MarkdownBlock] {
        let hash = text.hashValue
        lock.lock()
        if let cached = storage[hash] {
            lock.unlock()
            return cached
        }
        lock.unlock()
        
        let parsed = MarkdownParser.parse(text)
        lock.lock()
        if storage.count > 150 {
            storage.removeAll(keepingCapacity: true)
        }
        storage[hash] = parsed
        lock.unlock()
        return parsed
    }
}

public struct MarkdownContentView: View {
    public let content: String
    private let blocks: [MarkdownBlock]
    
    public init(content: String) {
        self.content = content
        self.blocks = MarkdownBlockCache.shared.blocks(for: content)
    }
    
    public var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            ForEach(blocks) { block in
                switch block {
                case .frontmatter(_, let rawContent, let lineCount):
                    FrontmatterCollapseView(content: rawContent, lineCount: lineCount)
                    
                case .heading(_, let level, let text):
                    headingView(level: level, text: text)
                    
                case .divider:
                    Divider()
                        .background(Color.secondary.opacity(0.3))
                        .padding(.vertical, 4)
                    
                case .codeBlock(_, let lang, let code):
                    codeBlockView(lang: lang, code: code)
                    
                case .table(_, let headers, let rows, let alignments):
                    tableView(headers: headers, rows: rows, alignments: alignments)
                    
                case .list(_, let items):
                    listView(items: items)
                    
                case .paragraph(_, let text):
                    paragraphView(text: text, size: 15)
                }
            }
        }
    }
    
    // MARK: - Rich Text with File Icons and Inline Code Styler
    
    private final class InlineMarkdownCache: @unchecked Sendable {
        static let shared = InlineMarkdownCache()
        private let lock = NSLock()
        private var cache: [Int: AttributedString] = [:]
        
        func get(_ key: Int) -> AttributedString? {
            lock.lock()
            defer { lock.unlock() }
            return cache[key]
        }
        
        func set(_ key: Int, value: AttributedString) {
            lock.lock()
            defer { lock.unlock() }
            if cache.count > 500 {
                cache.removeAll(keepingCapacity: true)
            }
            cache[key] = value
        }
    }
    
    private static let linkRegex = try? NSRegularExpression(pattern: #"(?<!\!)\[([^\]]+)\]\(([^)]+)\)"#)
    
    public static func renderRichText(_ rawText: String, size: CGFloat = 15, weight: Font.Weight = .regular) -> Text {
        var text = MathSymbolProcessor.process(rawText)
        
        // Auto-link bare implementation_plan.md, walkthrough.md, task.md if not already in markdown link
        if text.contains("implementation_plan.md") && !text.contains("[implementation_plan.md]") && !text.contains("](implementation_plan.md)") {
            text = text.replacingOccurrences(of: "implementation_plan.md", with: "[implementation_plan.md](implementation_plan.md)")
        }
        if text.contains("walkthrough.md") && !text.contains("[walkthrough.md]") && !text.contains("](walkthrough.md)") {
            text = text.replacingOccurrences(of: "walkthrough.md", with: "[walkthrough.md](walkthrough.md)")
        }
        if text.contains("task.md") && !text.contains("[task.md]") && !text.contains("](task.md)") {
            text = text.replacingOccurrences(of: "task.md", with: "[task.md](task.md)")
        }
        
        // Fast-path: If text does not contain markdown link signature `](`, avoid regex inspection entirely
        guard text.contains("[") && text.contains("](") else {
            return Text(renderInlineMarkdown(text, size: size, weight: weight))
        }
        
        guard let regex = Self.linkRegex else {
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
                // Vector file icons are 13.5pt tall. By default, SwiftUI aligns the icon bottom
                // with the font baseline, causing the icon to float above lowercase/uppercase text.
                // A negative baseline offset vertically centers the icon with the text.
                let iconOffset: CGFloat = (size <= 13) ? -2.2 : -2.0
                combined = combined + Text(Image(icon)).baselineOffset(iconOffset) + Text("\u{2009}")
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
    
    private static let cjkDelimiterMarker = "\u{FE50}" // Small comma (Unicode category Po - Punctuation, other)
    
    private static let cjkDelimiterPatterns: [NSRegularExpression] = {
        let patternStrings = [
            (#"\*\*\*"#, #"(?:[^\*]|\*(?!\*\*))+?"#, #"\*\*\*"#),
            (#"\*\*"#, #"(?:[^\*]|\*(?!\*))+?"#, #"\*\*"#),
            (#"~~"#, #"(?:[^~]|~(?!~))+?"#, #"~~"#),
            (#"__"#, #"(?:[^_]|_(?!_))+?"#, #"__"#),
            (#"(?<!\*)\*(?!\*)"#, #"[^\*\n]+?"#, #"(?<!\*)\*(?!\*)"#),
            (#"(?<!_)_(?!_)"#, #"[^_\n]+?"#, #"(?<!_)_(?!_)"#)
        ]
        return patternStrings.compactMap { (openPat, innerPat, closePat) in
            try? NSRegularExpression(pattern: "(\(openPat))(\(innerPat))(\(closePat))")
        }
    }()
    
    /// Fixes CommonMark delimiter bounding rules for CJK text.
    /// Under CommonMark 0.30 specification, delimiter runs adjacent to punctuation
    /// (e.g. **“bold”** or **(bold)**text) fail left-flanking or right-flanking checks
    /// because CJK characters preceding or following the delimiters are letters (not whitespace or punctuation).
    /// Inserting a temporary Unicode punctuation marker (U+FE50) on the non-punctuation side
    /// satisfies CommonMark flanking rules, and the marker is cleanly stripped from the AttributedString.
    private static func fixCJKDelimiters(in text: String) -> (fixed: String, hasMarkers: Bool) {
        guard text.contains("*") || text.contains("_") || text.contains("~") else {
            return (text, false)
        }
        
        // Protect inline code spans from modification
        var segments: [(content: String, isCode: Bool)] = []
        let chars = Array(text)
        var i = 0
        var lastIdx = 0
        
        while i < chars.count {
            if chars[i] == "`" {
                let tickStart = i
                while i < chars.count && chars[i] == "`" {
                    i += 1
                }
                let tickLen = i - tickStart
                
                if tickStart > lastIdx {
                    segments.append((String(chars[lastIdx..<tickStart]), false))
                }
                
                var closeFound = false
                var j = i
                while j < chars.count {
                    if chars[j] == "`" {
                        let cStart = j
                        while j < chars.count && chars[j] == "`" {
                            j += 1
                        }
                        if (j - cStart) == tickLen {
                            closeFound = true
                            segments.append((String(chars[tickStart..<j]), true))
                            i = j
                            lastIdx = j
                            break
                        }
                    } else {
                        j += 1
                    }
                }
                if !closeFound {
                    i = tickStart + 1
                }
            } else {
                i += 1
            }
        }
        if lastIdx < chars.count {
            segments.append((String(chars[lastIdx..<chars.count]), false))
        }
        
        var hasMarkers = false
        var processedSegments: [String] = []
        
        for segment in segments {
            if segment.isCode {
                processedSegments.append(segment.content)
                continue
            }
            
            let (processed, marked) = processDelimitersInSegment(segment.content)
            if marked { hasMarkers = true }
            processedSegments.append(processed)
        }
        
        return (processedSegments.joined(), hasMarkers)
    }
    
    private static func processDelimitersInSegment(_ text: String) -> (String, Bool) {
        var hasMarkers = false
        var result = text
        
        func isPunctOrSymbol(_ c: Character) -> Bool {
            return c.isPunctuation || c.isSymbol
        }
        
        for regex in cjkDelimiterPatterns {
            let nsText = result as NSString
            let matches = regex.matches(in: result, range: NSRange(location: 0, length: nsText.length))
            guard !matches.isEmpty else { continue }
            
            var modified = ""
            var lastEnd = 0
            
            for match in matches {
                let fullRange = match.range
                if fullRange.location > lastEnd {
                    modified += nsText.substring(with: NSRange(location: lastEnd, length: fullRange.location - lastEnd))
                }
                
                let openDelim = nsText.substring(with: match.range(at: 1))
                let innerText = nsText.substring(with: match.range(at: 2))
                let closeDelim = nsText.substring(with: match.range(at: 3))
                
                let prevChar: Character? = fullRange.location > 0 ? (nsText.substring(with: NSRange(location: fullRange.location - 1, length: 1)).first) : nil
                let nextCharIndex = fullRange.location + fullRange.length
                let nextChar: Character? = nextCharIndex < nsText.length ? (nsText.substring(with: NSRange(location: nextCharIndex, length: 1)).first) : nil
                
                let firstInner = innerText.first
                let lastInner = innerText.last
                
                var prefixMarker = ""
                var suffixMarker = ""
                
                // Opening rule: if firstInner is punct, and prevChar is non-punct non-whitespace
                if let fi = firstInner, isPunctOrSymbol(fi) {
                    if let p = prevChar, !p.isWhitespace && !isPunctOrSymbol(p) {
                        prefixMarker = cjkDelimiterMarker
                        hasMarkers = true
                    }
                }
                
                // Closing rule: if lastInner is punct, and nextChar is non-punct non-whitespace
                if let li = lastInner, isPunctOrSymbol(li) {
                    if let n = nextChar, !n.isWhitespace && !isPunctOrSymbol(n) {
                        suffixMarker = cjkDelimiterMarker
                        hasMarkers = true
                    }
                }
                
                modified += prefixMarker + openDelim + innerText + closeDelim + suffixMarker
                lastEnd = fullRange.location + fullRange.length
            }
            
            if lastEnd < nsText.length {
                modified += nsText.substring(with: NSRange(location: lastEnd, length: nsText.length - lastEnd))
            }
            
            result = modified
        }
        
        return (result, hasMarkers)
    }
    
    public static func renderInlineMarkdown(_ text: String, size: CGFloat = 15, weight: Font.Weight = .regular) -> AttributedString {
        let cacheKey = text.hashValue ^ (Int(size * 100) << 2) ^ weight.hashValue
        if let cached = InlineMarkdownCache.shared.get(cacheKey) {
            return cached
        }
        
        let (preprocessedText, hasMarkers) = fixCJKDelimiters(in: text)
        
        var options = AttributedString.MarkdownParsingOptions()
        options.interpretedSyntax = .inlineOnlyPreservingWhitespace
        guard var attr = try? AttributedString(markdown: preprocessedText, options: options) else {
            let fallback = AttributedString(text)
            InlineMarkdownCache.shared.set(cacheKey, value: fallback)
            return fallback
        }
        
        if hasMarkers {
            while let range = attr.range(of: cjkDelimiterMarker) {
                attr.removeSubrange(range)
            }
        }
        
        // Antigravity Desktop Code Amber/Yellow color: #E5C07B (RGB: 229, 192, 123)
        let codeFgColor = Color(red: 229/255, green: 192/255, blue: 123/255)
        
        for run in attr.runs {
            if let intent = run.inlinePresentationIntent, intent.contains(.code) {
                attr[run.range].foregroundColor = codeFgColor
                attr[run.range].font = .system(size: size * 0.9, weight: .medium, design: .monospaced)
            } else if run.link != nil {
                attr[run.range].font = .system(size: size * 0.9, weight: .medium, design: .monospaced)
                attr[run.range].foregroundColor = Color.blue
                let linkStr = (run.link?.absoluteString ?? "").lowercased()
                if linkStr.contains("implementation_plan") || linkStr.contains("walkthrough") {
                    attr[run.range].backgroundColor = Color.blue.opacity(0.12)
                }
            }
        }
        InlineMarkdownCache.shared.set(cacheKey, value: attr)
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
    private func tableView(headers: [String], rows: [[String]], alignments: [TableColumnAlignment] = []) -> some View {
        let columnCount = max(headers.count, rows.map(\.count).max() ?? 0)
        
        if columnCount > 0 {
            ScrollView(.horizontal, showsIndicators: true) {
                Grid(alignment: .leading, horizontalSpacing: 0, verticalSpacing: 0) {
                    // Header row
                    if !headers.isEmpty {
                        GridRow {
                            ForEach(0..<columnCount, id: \.self) { colIdx in
                                let headerText = colIdx < headers.count ? headers[colIdx] : ""
                                let align = (colIdx < alignments.count ? alignments[colIdx] : .leading).swiftUIAlignment
                                
                                Self.renderRichText(headerText, size: 13, weight: .bold)
                                    .font(.system(size: 13, weight: .bold))
                                    .foregroundColor(.primary)
                                    .padding(.horizontal, 12)
                                    .padding(.vertical, 8)
                                    .frame(minWidth: 80, maxWidth: .infinity, maxHeight: .infinity, alignment: align)
                                    .gridCellUnsizedAxes(.vertical)
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
                                let align = (colIdx < alignments.count ? alignments[colIdx] : .leading).swiftUIAlignment
                                let isEven = rowIdx % 2 == 0
                                
                                Self.renderRichText(cellText, size: 13)
                                    .font(.system(size: 13))
                                    .foregroundColor(.primary.opacity(0.9))
                                    .padding(.horizontal, 12)
                                    .padding(.vertical, 8)
                                    .frame(minWidth: 80, maxWidth: .infinity, maxHeight: .infinity, alignment: align)
                                    .gridCellUnsizedAxes(.vertical)
                                    .background(isEven ? Color.clear : Color(uiColor: .tertiarySystemBackground).opacity(0.3))
                                    .overlay(alignment: .trailing) {
                                        if colIdx < columnCount - 1 {
                                            Rectangle()
                                                .fill(Color.secondary.opacity(0.15))
                                                .frame(width: 0.5)
                                        }
                                    }
                                    .textSelection(.enabled)
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
            }
            .fixedSize(horizontal: false, vertical: true)
            .padding(.vertical, 4)
        }
    }
    
    @ViewBuilder
    private func listView(items: [String]) -> some View {
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
    
    // MARK: - Plan Link Segmentation & Paragraph Flow
    
    @ViewBuilder
    private func paragraphView(text: String, size: CGFloat = 15) -> some View {
        let cleanText = text.trimmingCharacters(in: .whitespacesAndNewlines)
        let segments = Self.parsePlanSegments(cleanText)
        if segments.count <= 1 && (segments.first?.isPlanButton != true) {
            Self.renderRichText(cleanText, size: size)
                .font(.system(size: size))
                .textSelection(.enabled)
                .lineSpacing(3)
                .fixedSize(horizontal: false, vertical: true)
        } else {
            FlowLayout(horizontalSpacing: 4, verticalSpacing: 6) {
                ForEach(segments) { segment in
                    switch segment {
                    case .text(_, let content):
                        let trimmed = content.trimmingCharacters(in: .whitespacesAndNewlines)
                        if !trimmed.isEmpty {
                            Self.renderRichText(trimmed, size: size)
                                .font(.system(size: size))
                                .textSelection(.enabled)
                                .lineSpacing(3)
                        }
                    case .planButton(_, let title, let uri):
                        PlanButtonView(title: title, uri: uri)
                    }
                }
            }
            .fixedSize(horizontal: false, vertical: true)
        }
    }
    
    private static let planRegex = try? NSRegularExpression(
        pattern: #"(?:(?<!\!)\[([^\]]+)\]\(([^)]*(?:implementation_plan|walkthrough)\.md[^)]*)\)|(?<![a-zA-Z0-9_\-\.\/])((?:implementation_plan|walkthrough)\.md)(?![a-zA-Z0-9_\-\.\/]))"#,
        options: [.caseInsensitive]
    )
    
    public static func parsePlanSegments(_ rawText: String) -> [PlanSegment] {
        guard rawText.contains("implementation_plan") || rawText.contains("walkthrough") else {
            return [.text(id: "text-0", content: rawText)]
        }
        
        guard let regex = planRegex else {
            return [.text(id: "text-0", content: rawText)]
        }
        
        let nsText = rawText as NSString
        let matches = regex.matches(in: rawText, range: NSRange(location: 0, length: nsText.length))
        guard !matches.isEmpty else {
            return [.text(id: "text-0", content: rawText)]
        }
        
        var segments: [PlanSegment] = []
        var lastEnd = 0
        var segIdx = 0
        
        for match in matches {
            let matchRange = match.range
            if matchRange.location > lastEnd {
                let prefix = nsText.substring(with: NSRange(location: lastEnd, length: matchRange.location - lastEnd))
                segments.append(.text(id: "seg-\(segIdx)", content: prefix))
                segIdx += 1
            }
            
            var title = "implementation_plan.md"
            var uri = "implementation_plan.md"
            
            if match.range(at: 1).location != NSNotFound && match.range(at: 2).location != NSNotFound {
                title = nsText.substring(with: match.range(at: 1)).trimmingCharacters(in: .whitespaces)
                uri = nsText.substring(with: match.range(at: 2)).trimmingCharacters(in: .whitespaces)
            } else if match.range(at: 3).location != NSNotFound {
                let fn = nsText.substring(with: match.range(at: 3)).trimmingCharacters(in: .whitespaces)
                title = fn
                uri = fn
            }
            
            segments.append(.planButton(id: "seg-\(segIdx)", title: title, uri: uri))
            segIdx += 1
            
            lastEnd = matchRange.location + matchRange.length
        }
        
        if lastEnd < nsText.length {
            let suffix = nsText.substring(with: NSRange(location: lastEnd, length: nsText.length - lastEnd))
            let punctChars: Set<Character> = ["。", ".", "，", ",", "！", "!", "？", "?", "；", ";", "：", ":"]
            if let firstChar = suffix.first, punctChars.contains(firstChar) {
                segments.append(.text(id: "seg-\(segIdx)", content: String(firstChar)))
                segIdx += 1
                let rest = String(suffix.dropFirst())
                if !rest.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                    segments.append(.text(id: "seg-\(segIdx)", content: rest))
                    segIdx += 1
                }
            } else {
                segments.append(.text(id: "seg-\(segIdx)", content: suffix))
                segIdx += 1
            }
        }
        
        return segments
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
            
            // 0. YAML Frontmatter / Marp configuration at beginning of document
            if blockIdx == 0 {
                if trimmed == "---" {
                    var endIdx = i + 1
                    var foundEnd = false
                    while endIdx < lines.count {
                        let t = lines[endIdx].trimmingCharacters(in: .whitespaces)
                        if t == "---" || t == "..." {
                            foundEnd = true
                            break
                        }
                        endIdx += 1
                    }
                    if foundEnd && endIdx > i + 1 {
                        let fmLines = lines[(i + 1)..<endIdx]
                        let raw = fmLines.joined(separator: "\n")
                        blocks.append(.frontmatter(id: "block-\(blockIdx)", rawContent: raw, lineCount: endIdx - i + 1))
                        blockIdx += 1
                        i = endIdx + 1
                        continue
                    }
                } else if trimmed.hasPrefix("marp:") || (trimmed.contains(":") && (trimmed.hasPrefix("theme:") || trimmed.hasPrefix("style:"))) {
                    var endIdx = i + 1
                    var foundEnd = false
                    while endIdx < min(lines.count, i + 100) {
                        let t = lines[endIdx].trimmingCharacters(in: .whitespaces)
                        if t == "---" {
                            foundEnd = true
                            break
                        }
                        if t.hasPrefix("# ") || t.hasPrefix("## ") {
                            break
                        }
                        endIdx += 1
                    }
                    if foundEnd {
                        let fmLines = lines[i..<endIdx]
                        let raw = fmLines.joined(separator: "\n")
                        blocks.append(.frontmatter(id: "block-\(blockIdx)", rawContent: raw, lineCount: endIdx - i + 1))
                        blockIdx += 1
                        i = endIdx + 1
                        continue
                    }
                }
            }
            
            // HTML <style>...</style> Block
            if trimmed.lowercased().hasPrefix("<style") {
                var styleLines: [String] = []
                var endFound = false
                while i < lines.count {
                    let sLine = lines[i]
                    styleLines.append(sLine)
                    if sLine.lowercased().contains("</style>") {
                        endFound = true
                        i += 1
                        break
                    }
                    i += 1
                }
                if endFound {
                    let raw = styleLines.joined(separator: "\n")
                    blocks.append(.frontmatter(id: "block-\(blockIdx)", rawContent: raw, lineCount: styleLines.count))
                    blockIdx += 1
                    continue
                }
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
                        let placeholder = "\u{E000}"
                        let sanitized = rowStr.replacingOccurrences(of: "\\|", with: placeholder)
                        let parts = sanitized.split(separator: "|", omittingEmptySubsequences: false)
                        guard parts.count >= 2 else { return [] }
                        let inner = parts[1..<(parts.count - 1)]
                        return inner.map {
                            String($0)
                                .replacingOccurrences(of: placeholder, with: "|")
                                .trimmingCharacters(in: .whitespaces)
                        }
                    }
                    let isSeparatorRow = { (cells: [String]) -> Bool in
                        guard !cells.isEmpty else { return false }
                        return cells.allSatisfy { cell in
                            let t = cell.trimmingCharacters(in: .whitespaces)
                            guard !t.isEmpty else { return false }
                            return t.allSatisfy { $0 == "-" || $0 == ":" }
                        }
                    }
                    let parseAlignments = { (cells: [String]) -> [TableColumnAlignment] in
                        return cells.map { cell in
                            let t = cell.trimmingCharacters(in: .whitespaces)
                            let hasLeft = t.hasPrefix(":")
                            let hasRight = t.hasSuffix(":")
                            if hasLeft && hasRight {
                                return .center
                            } else if hasRight {
                                return .trailing
                            } else {
                                return .leading
                            }
                        }
                    }
                    
                    let headers = parseRow(tableLines[0])
                    var alignments: [TableColumnAlignment] = []
                    var rows: [[String]] = []
                    for rowIdx in 1..<tableLines.count {
                        let r = parseRow(tableLines[rowIdx])
                        if isSeparatorRow(r) {
                            if alignments.isEmpty {
                                alignments = parseAlignments(r)
                            }
                            continue
                        }
                        rows.append(r)
                    }
                    blocks.append(.table(id: "block-\(blockIdx)", headers: headers, rows: rows, alignments: alignments))
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

// MARK: - Plan Segment Models & Flow Layout

public enum PlanSegment: Identifiable {
    case text(id: String, content: String)
    case planButton(id: String, title: String, uri: String)
    
    public var id: String {
        switch self {
        case .text(let id, _): return id
        case .planButton(let id, _, _): return id
        }
    }
    
    public var isPlanButton: Bool {
        if case .planButton = self { return true }
        return false
    }
}

public struct PlanButtonStyle: ButtonStyle {
    public init() {}
    
    public func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .scaleEffect(configuration.isPressed ? 0.96 : 1.0)
            .opacity(configuration.isPressed ? 0.75 : 1.0)
            .animation(.easeInOut(duration: 0.12), value: configuration.isPressed)
    }
}

public struct PlanButtonView: View {
    @Environment(\.openURL) private var openURL
    
    public let title: String
    public let uri: String
    
    public init(title: String, uri: String) {
        self.title = title
        self.uri = uri
    }
    
    public var body: some View {
        Button(action: {
            UIImpactFeedbackGenerator(style: .light).impactOccurred()
            let target = uri.trimmingCharacters(in: .whitespacesAndNewlines)
            if let url = URL(string: target) ?? URL(string: target.addingPercentEncoding(withAllowedCharacters: .urlPathAllowed) ?? target) {
                openURL(url)
            }
        }) {
            HStack(spacing: 5) {
                if let iconName = FileIconResolver.resolveIcon(for: uri) ?? FileIconResolver.resolveIcon(for: title) {
                    Image(iconName)
                        .resizable()
                        .scaledToFit()
                        .frame(width: 14, height: 14)
                } else {
                    Image(systemName: "doc.text.magnifyingglass")
                        .font(.system(size: 12, weight: .semibold))
                        .foregroundColor(.blue)
                }
                
                Text(title)
                    .font(.system(size: 13, weight: .semibold, design: .monospaced))
                    .foregroundColor(.blue)
                    .lineLimit(1)
                
                Image(systemName: "arrow.up.forward.app")
                    .font(.system(size: 10.5, weight: .medium))
                    .foregroundColor(.blue.opacity(0.65))
            }
            .padding(.horizontal, 9)
            .padding(.vertical, 4.5)
            .background(Color.blue.opacity(0.12))
            .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
            .overlay(
                RoundedRectangle(cornerRadius: 8, style: .continuous)
                    .stroke(Color.blue.opacity(0.35), lineWidth: 1)
            )
        }
        .buttonStyle(PlanButtonStyle())
    }
}

public struct FlowLayout: Layout {
    public var horizontalSpacing: CGFloat
    public var verticalSpacing: CGFloat
    
    public init(horizontalSpacing: CGFloat = 4, verticalSpacing: CGFloat = 6) {
        self.horizontalSpacing = horizontalSpacing
        self.verticalSpacing = verticalSpacing
    }
    
    public func sizeThatFits(proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) -> CGSize {
        let maxAvailableWidth = proposal.width ?? .infinity
        var currentX: CGFloat = 0
        var currentY: CGFloat = 0
        var lineHeight: CGFloat = 0
        var maxWidth: CGFloat = 0
        
        for subview in subviews {
            let size = subview.sizeThatFits(ProposedViewSize(width: maxAvailableWidth, height: nil))
            if currentX + size.width > maxAvailableWidth && currentX > 0 {
                currentX = 0
                currentY += lineHeight + verticalSpacing
                lineHeight = 0
            }
            lineHeight = max(lineHeight, size.height)
            currentX += size.width + horizontalSpacing
            maxWidth = max(maxWidth, currentX - horizontalSpacing)
        }
        
        return CGSize(width: min(maxWidth, maxAvailableWidth), height: currentY + lineHeight)
    }
    
    public func placeSubviews(in bounds: CGRect, proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) {
        let maxAvailableWidth = bounds.width
        var currentX: CGFloat = bounds.minX
        var currentY: CGFloat = bounds.minY
        var lineHeight: CGFloat = 0
        var lineSubviews: [(subview: LayoutSubview, size: CGSize, x: CGFloat)] = []
        
        func flushLine() {
            for item in lineSubviews {
                let y = currentY + (lineHeight - item.size.height) / 2
                item.subview.place(at: CGPoint(x: item.x, y: y), proposal: ProposedViewSize(item.size))
            }
            lineSubviews.removeAll()
        }
        
        for subview in subviews {
            let size = subview.sizeThatFits(ProposedViewSize(width: maxAvailableWidth, height: nil))
            if currentX + size.width > bounds.maxX && currentX > bounds.minX {
                flushLine()
                currentX = bounds.minX
                currentY += lineHeight + verticalSpacing
                lineHeight = 0
            }
            lineSubviews.append((subview: subview, size: size, x: currentX))
            lineHeight = max(lineHeight, size.height)
            currentX += size.width + horizontalSpacing
        }
        flushLine()
    }
}

// MARK: - Frontmatter Collapse View

public struct FrontmatterCollapseView: View {
    public let content: String
    public let lineCount: Int
    @State private var isExpanded: Bool = false
    
    public init(content: String, lineCount: Int) {
        self.content = content
        self.lineCount = lineCount
    }
    
    public var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Button(action: {
                withAnimation(.spring(response: 0.3, dampingFraction: 0.8)) {
                    isExpanded.toggle()
                }
            }) {
                HStack(spacing: 7) {
                    Image(systemName: "slider.horizontal.3")
                        .font(.system(size: 11.5, weight: .semibold))
                        .foregroundColor(.secondary)
                    
                    Text("已自动隐藏文档配置与排版样式 (\(lineCount)行)")
                        .font(.system(size: 12, weight: .medium))
                        .foregroundColor(.secondary)
                    
                    Spacer()
                    
                    Image(systemName: isExpanded ? "chevron.up" : "chevron.down")
                        .font(.system(size: 10.5, weight: .semibold))
                        .foregroundColor(.secondary.opacity(0.8))
                }
                .padding(.horizontal, 11)
                .padding(.vertical, 7)
                .background(Color(uiColor: .tertiarySystemFill))
                .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
            }
            .buttonStyle(.plain)
            .zIndex(10)
            
            if isExpanded {
                ScrollView(.horizontal, showsIndicators: false) {
                    Text(content)
                        .font(.system(size: 11, design: .monospaced))
                        .foregroundColor(.primary.opacity(0.85))
                        .padding(10)
                }
                .fixedSize(horizontal: false, vertical: true)
                .background(Color(uiColor: .secondarySystemBackground))
                .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
                .overlay(
                    RoundedRectangle(cornerRadius: 8, style: .continuous)
                        .stroke(Color.secondary.opacity(0.2), lineWidth: 0.8)
                )
                .clipped()
                .zIndex(1)
                .transition(
                    .asymmetric(
                        insertion: .opacity.combined(with: .scale(scale: 0.98, anchor: .top)),
                        removal: .opacity
                    )
                )
            }
        }
        .frame(maxWidth: .infinity, alignment: .topLeading)
        .clipped()
        .padding(.bottom, 4)
    }
}
