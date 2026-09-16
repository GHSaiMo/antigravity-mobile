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
    
    public var textAlignment: TextAlignment {
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
    case image(id: String, alt: String, url: String)
    
    public var id: String {
        switch self {
        case .frontmatter(let id, _, _): return id
        case .heading(let id, _, _): return id
        case .divider(let id): return id
        case .codeBlock(let id, _, _): return id
        case .table(let id, _, _, _): return id
        case .list(let id, _): return id
        case .paragraph(let id, _): return id
        case .image(let id, _, _): return id
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
    public let onImageTap: ((URL) -> Void)?
    private let blocks: [MarkdownBlock]
    
    @State private var internalPreviewImage: IdentifiableImage? = nil
    
    public init(content: String, onImageTap: ((URL) -> Void)? = nil) {
        self.content = content
        self.onImageTap = onImageTap
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
                    if lang.trimmingCharacters(in: .whitespaces).lowercased() == "mermaid" {
                        MermaidDiagramView(code: code)
                    } else {
                        codeBlockView(lang: lang, code: code)
                    }
                    
                case .table(_, let headers, let rows, let alignments):
                    tableView(headers: headers, rows: rows, alignments: alignments)
                    
                case .list(_, let items):
                    listView(items: items)
                    
                case .paragraph(_, let text):
                    paragraphView(text: text, size: 15)
                    
                case .image(_, let alt, let url):
                    markdownImageView(alt: alt, urlString: url)
                }
            }
        }
        .fullScreenCover(item: $internalPreviewImage) { item in
            ImageViewerSheet(item: item)
        }
    }
    
    // MARK: - Embedded Markdown Images
    
    private func resolvedImageURL(from raw: String) -> URL? {
        if let base = AppSettings.shared.gatewayURL {
            let resolved = APIClient.shared.resolveMediaURL(raw, baseURL: base)
            if let url = URL(string: resolved) {
                return url
            }
        }
        return URL(string: raw)
    }
    
    private func handleImageTap(url: URL) {
        UIImpactFeedbackGenerator(style: .light).impactOccurred()
        if let onImageTap {
            onImageTap(url)
        } else {
            internalPreviewImage = IdentifiableImage(url: url)
        }
    }
    
    @ViewBuilder
    private func markdownImageView(alt: String, urlString: String) -> some View {
        if let url = resolvedImageURL(from: urlString) {
            AsyncImage(url: url) { phase in
                switch phase {
                case .empty:
                    ProgressView()
                        .frame(maxWidth: .infinity, minHeight: 120, maxHeight: 160, alignment: .leading)
                case .success(let image):
                    Button {
                        handleImageTap(url: url)
                    } label: {
                        image
                            .resizable()
                            .scaledToFit()
                            .frame(maxWidth: .infinity, maxHeight: 280, alignment: .leading)
                    }
                    .buttonStyle(.plain)
                    .accessibilityLabel(alt.isEmpty ? "图片" : alt)
                case .failure:
                    markdownImageFailure(alt: alt)
                @unknown default:
                    EmptyView()
                }
            }
        } else {
            markdownImageFailure(alt: alt)
        }
    }
    
    private func markdownImageFailure(alt: String) -> some View {
        HStack(spacing: 8) {
            Image(systemName: "photo")
            Text(alt.isEmpty ? "图片加载失败" : alt)
                .lineLimit(2)
        }
        .font(.footnote)
        .foregroundColor(.secondary)
        .frame(maxWidth: .infinity, alignment: .leading)
        .accessibilityLabel(alt.isEmpty ? "图片加载失败" : alt)
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
    
    public static func renderRichText(_ rawText: String, size: CGFloat = 15, weight: Font.Weight = .regular) -> Text {
        var text = MathSymbolProcessor.process(rawText)
        text = replaceHtmlBreaks(in: text)
        
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
        
        // Auto-link MEDIA: paths (e.g. MEDIA:/path/to/image.png) into clickable image links
        if text.contains("MEDIA:") {
            let mediaPattern = #"(?:^|\s|<br\s*/?>)MEDIA:\s*([^\s\)\<\>\"\'\`]+)"#
            if let regex = try? NSRegularExpression(pattern: mediaPattern) {
                let nsText = text as NSString
                let matches = regex.matches(in: text, range: NSRange(location: 0, length: nsText.length))
                if !matches.isEmpty {
                    var replaced = ""
                    var lastEnd = 0
                    for match in matches {
                        if match.range.location > lastEnd {
                            replaced += nsText.substring(with: NSRange(location: lastEnd, length: match.range.location - lastEnd))
                        }
                        let rawPath = nsText.substring(with: match.range(at: 1))
                        let clean = rawPath.trimmingCharacters(in: CharacterSet(charactersIn: "`\"'()[]<>"))
                        let fn = (clean as NSString).lastPathComponent
                        let linkTarget = (clean.hasPrefix("file://") || clean.hasPrefix("http://") || clean.hasPrefix("https://")) ? clean : "file://\(clean)"
                        replaced += "\n[点击放大查看图片 (\(fn))](\(linkTarget))"
                        lastEnd = match.range.location + match.range.length
                    }
                    if lastEnd < nsText.length {
                        replaced += nsText.substring(with: NSRange(location: lastEnd, length: nsText.length - lastEnd))
                    }
                    text = replaced
                }
            }
        }
        
        let attr = renderInlineMarkdown(text, size: size, weight: weight)
        
        // Fast-path: If text does not contain markdown link signature `](`, return Text(attr) directly
        guard text.contains("[") && text.contains("](") else {
            return Text(attr)
        }
        
        var iconInsertions: [(icon: String, index: AttributedString.Index)] = []
        var currentLinkUrl: URL? = nil
        var currentLinkRunsText = ""
        var currentLinkStartIndex: AttributedString.Index? = nil
        
        func finishCurrentLink() {
            guard let url = currentLinkUrl, let startIdx = currentLinkStartIndex else { return }
            let linkText = currentLinkRunsText.trimmingCharacters(in: .whitespacesAndNewlines)
            if let icon = FileIconResolver.resolveIcon(for: linkText) ?? FileIconResolver.resolveIcon(for: url.absoluteString) {
                iconInsertions.append((icon: icon, index: startIdx))
            }
            currentLinkUrl = nil
            currentLinkRunsText = ""
            currentLinkStartIndex = nil
        }
        
        for run in attr.runs {
            if let link = run.link {
                if let activeUrl = currentLinkUrl, activeUrl == link {
                    currentLinkRunsText += String(attr[run.range].characters)
                } else {
                    finishCurrentLink()
                    currentLinkUrl = link
                    currentLinkRunsText = String(attr[run.range].characters)
                    currentLinkStartIndex = run.range.lowerBound
                }
            } else {
                finishCurrentLink()
            }
        }
        finishCurrentLink()
        
        if iconInsertions.isEmpty {
            return Text(attr)
        }
        
        var combined = Text("")
        var currentIndex = attr.startIndex
        let iconOffset: CGFloat = (size <= 13) ? -2.2 : -2.0
        
        for ins in iconInsertions {
            if ins.index > currentIndex {
                let leading = AttributedString(attr[currentIndex..<ins.index])
                combined = combined + Text(leading)
            }
            combined = combined + Text(Image(ins.icon)).baselineOffset(iconOffset) + Text("\u{2009}")
            currentIndex = ins.index
        }
        
        if currentIndex < attr.endIndex {
            let trailing = AttributedString(attr[currentIndex..<attr.endIndex])
            combined = combined + Text(trailing)
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
    
    // MARK: - HTML Line Break & Inline Code Processor
    
    private static let htmlBreakRegex = try? NSRegularExpression(
        pattern: #"[ \t]*<(?:\/br|br\b[^>]*\/?)>[ \t]*\n?"#,
        options: [.caseInsensitive]
    )
    
    /// Splits text into inline code segments (wrapped in backticks) and regular markdown text segments.
    public static func splitCodeSpans(in text: String) -> [(content: String, isCode: Bool)] {
        guard text.contains("`") else {
            return [(content: text, isCode: false)]
        }
        
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
        return segments
    }
    
    /// Replaces HTML line breaks (<br>, <br/>, <br />, </br>) with newlines while protecting inline code spans.
    public static func replaceHtmlBreaks(in text: String) -> String {
        guard text.localizedCaseInsensitiveContains("<br") else {
            return text
        }
        
        let replaceInString = { (s: String) -> String in
            guard let regex = htmlBreakRegex else {
                return s.replacingOccurrences(of: "<br>", with: "\n", options: .caseInsensitive)
                        .replacingOccurrences(of: "<br/>", with: "\n", options: .caseInsensitive)
                        .replacingOccurrences(of: "<br />", with: "\n", options: .caseInsensitive)
                        .replacingOccurrences(of: "</br>", with: "\n", options: .caseInsensitive)
            }
            let range = NSRange(location: 0, length: (s as NSString).length)
            return regex.stringByReplacingMatches(in: s, options: [], range: range, withTemplate: "\n")
        }
        
        guard text.contains("`") else {
            return replaceInString(text)
        }
        
        let segments = splitCodeSpans(in: text)
        return segments.map { segment in
            segment.isCode ? segment.content : replaceInString(segment.content)
        }.joined()
    }
    
    // Normalizes inner whitespace in bold markdown, e.g. "** text **" -> " **text** "
    private static let boldPairRegex = try? NSRegularExpression(
        pattern: #"(?<!\*)\*\*((?:[^\*]|\*(?!\*))+?)\*\*(?!\*)"#
    )
    
    private static func normalizeBoldSpaces(in text: String) -> String {
        guard text.contains("**"), let regex = boldPairRegex else { return text }
        let nsText = text as NSString
        let matches = regex.matches(in: text, range: NSRange(location: 0, length: nsText.length))
        guard !matches.isEmpty else { return text }
        
        var result = ""
        var lastEnd = 0
        
        for match in matches {
            let fullRange = match.range
            if fullRange.location > lastEnd {
                result += nsText.substring(with: NSRange(location: lastEnd, length: fullRange.location - lastEnd))
            }
            
            let inner = nsText.substring(with: match.range(at: 1))
            
            var leadingSpaces = ""
            var trailingSpaces = ""
            var trimmed = inner
            
            while let first = trimmed.first, first == " " || first == "\t" {
                leadingSpaces.append(first)
                trimmed.removeFirst()
            }
            while let last = trimmed.last, last == " " || last == "\t" {
                trailingSpaces.append(last)
                trimmed.removeLast()
            }
            
            if trimmed.isEmpty {
                result += nsText.substring(with: fullRange)
            } else {
                result += leadingSpaces + "**" + trimmed + "**" + trailingSpaces
            }
            
            lastEnd = fullRange.location + fullRange.length
        }
        
        if lastEnd < nsText.length {
            result += nsText.substring(with: NSRange(location: lastEnd, length: nsText.length - lastEnd))
        }
        
        return result
    }

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
        
        let segments = splitCodeSpans(in: text)
        var hasMarkers = false
        var processedSegments: [String] = []
        
        for segment in segments {
            if segment.isCode {
                processedSegments.append(segment.content)
                continue
            }
            
            let normalizedContent = normalizeBoldSpaces(in: segment.content)
            let (processed, marked) = processDelimitersInSegment(normalizedContent)
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
        
        let textWithBreaks = replaceHtmlBreaks(in: text)
        let (preprocessedText, hasMarkers) = fixCJKDelimiters(in: textWithBreaks)
        
        var options = AttributedString.MarkdownParsingOptions()
        options.interpretedSyntax = .inlineOnlyPreservingWhitespace
        guard var attr = try? AttributedString(markdown: preprocessedText, options: options) else {
            let fallback = AttributedString(textWithBreaks)
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
            let isBold = (run.inlinePresentationIntent?.contains(.stronglyEmphasized) == true) || weight == .bold
            let runWeight: Font.Weight = isBold ? .bold : .medium
            
            if let intent = run.inlinePresentationIntent, intent.contains(.code) {
                attr[run.range].foregroundColor = codeFgColor
                attr[run.range].font = .system(size: size * 0.9, weight: runWeight, design: .monospaced)
            } else if run.link != nil {
                attr[run.range].font = .system(size: size * 0.9, weight: runWeight, design: .monospaced)
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
                                let colAlignment = colIdx < alignments.count ? alignments[colIdx] : .leading
                                let align = colAlignment.swiftUIAlignment
                                let textAlignment = colAlignment.textAlignment
                                
                                Self.renderRichText(headerText, size: 13, weight: .bold)
                                    .font(.system(size: 13, weight: .bold))
                                    .foregroundColor(.primary)
                                    .multilineTextAlignment(textAlignment)
                                    .padding(.horizontal, 12)
                                    .padding(.vertical, 8)
                                    .frame(minWidth: 80, alignment: align)
                                    .frame(maxHeight: .infinity, alignment: align)
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
                                    .frame(minWidth: 80, alignment: align)
                                    .frame(maxHeight: .infinity, alignment: align)
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
                .fixedSize(horizontal: false, vertical: true)
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
        let cleanText = Self.replaceHtmlBreaks(in: text).trimmingCharacters(in: .whitespacesAndNewlines)
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
        }
    }
    
    private static let planRegex = try? NSRegularExpression(
        pattern: #"(?:(?<!\!)\[([^\]]+)\]\(([^)]+)\)|(?<![a-zA-Z0-9_\-\.\/])((?:implementation_plan|walkthrough)\.md)(?![a-zA-Z0-9_\-\.\/]))"#,
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
        let allMatches = regex.matches(in: rawText, range: NSRange(location: 0, length: nsText.length))
        
        let matches = allMatches.filter { m in
            if m.range(at: 1).location != NSNotFound && m.range(at: 2).location != NSNotFound {
                let g1 = nsText.substring(with: m.range(at: 1)).lowercased()
                let g2 = nsText.substring(with: m.range(at: 2)).lowercased()
                return g1.contains("implementation_plan") || g1.contains("walkthrough") ||
                       g2.contains("implementation_plan") || g2.contains("walkthrough")
            } else if m.range(at: 3).location != NSNotFound {
                return true
            }
            return false
        }
        
        guard !matches.isEmpty else {
            return [.text(id: "text-0", content: rawText)]
        }
        
        var segments: [PlanSegment] = []
        var lastEnd = 0
        var segIdx = 0
        
        for (idx, match) in matches.enumerated() {
            let matchRange = match.range
            var prefix = ""
            if matchRange.location > lastEnd {
                prefix = nsText.substring(with: NSRange(location: lastEnd, length: matchRange.location - lastEnd))
            }
            
            let nextIndex = matchRange.location + matchRange.length
            let suffixLength = (idx + 1 < matches.count) ? (matches[idx + 1].range.location - nextIndex) : (nsText.length - nextIndex)
            let suffixPreview = nsText.substring(with: NSRange(location: nextIndex, length: suffixLength))
            
            // If the plan link was wrapped in markdown delimiters (e.g. **[implementation_plan.md](...)**),
            // strip them from prefix and suffix so no dangling asterisks/ticks surround the button.
            var strippedLength = 0
            if prefix.hasSuffix("**") && suffixPreview.hasPrefix("**") {
                prefix = String(prefix.dropLast(2))
                strippedLength = 2
            } else if prefix.hasSuffix("*") && suffixPreview.hasPrefix("*") {
                prefix = String(prefix.dropLast(1))
                strippedLength = 1
            } else if prefix.hasSuffix("__") && suffixPreview.hasPrefix("__") {
                prefix = String(prefix.dropLast(2))
                strippedLength = 2
            } else if prefix.hasSuffix("`") && suffixPreview.hasPrefix("`") {
                prefix = String(prefix.dropLast(1))
                strippedLength = 1
            }
            
            let trimmedPrefix = prefix.trimmingCharacters(in: .whitespacesAndNewlines)
            if !trimmedPrefix.isEmpty {
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
            
            lastEnd = matchRange.location + matchRange.length + strippedLength
        }
        
        if lastEnd < nsText.length {
            let suffix = nsText.substring(with: NSRange(location: lastEnd, length: nsText.length - lastEnd))
            let punctChars: Set<Character> = ["。", ".", "，", ",", "！", "!", "？", "?", "；", ";", "：", ":"]
            let trimmedSuffix = suffix.trimmingCharacters(in: .whitespacesAndNewlines)
            // If suffix only consists of trailing punctuation and whitespace, drop it to avoid dangling orphan punctuation.
            // If it contains meaningful text, preserve it as a unified text segment.
            let hasMeaningfulContent = trimmedSuffix.contains { !punctChars.contains($0) }
            if hasMeaningfulContent {
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
                appendListOrSplitImages(listItems, into: &blocks, blockIdx: &blockIdx)
                continue
            }
            
            // Standalone image line: ![alt](url), [![alt](thumb)](url), MEDIA:url
            if let img = parseStandaloneImage(trimmed) {
                blocks.append(.image(id: "block-\(blockIdx)", alt: img.alt, url: img.url))
                blockIdx += 1
                i += 1
                continue
            }
            
            // Paragraph
            var paraLines: [String] = [line]
            i += 1
            while i < lines.count {
                let nextLine = lines[i]
                let nTrimmed = nextLine.trimmingCharacters(in: .whitespaces)
                if nTrimmed.isEmpty || nTrimmed.hasPrefix("```") || nTrimmed.hasPrefix("#") || nTrimmed == "---" || (nTrimmed.hasPrefix("|") && nTrimmed.hasSuffix("|")) || nTrimmed.hasPrefix("- ") || nTrimmed.hasPrefix("* ") || nTrimmed.hasPrefix("• ") || parseStandaloneImage(nTrimmed) != nil {
                    break
                }
                paraLines.append(nextLine)
                i += 1
            }
            let paraText = paraLines.joined(separator: "\n")
            let split = splitParagraphIntoBlocks(text: paraText, blockIdx: &blockIdx)
            blocks.append(contentsOf: split)
        }
        
        return blocks
    }
    
    // MARK: - Image syntax
    
    /// `[![alt](thumb)](orig)` — linked thumbnail; capture alt, thumb, original.
    private static let linkedImageRegex = try? NSRegularExpression(
        pattern: #"\[!\[(.*?)\]\(([^\s\)]+)(?:\s+"[^"]*")?\)\]\(([^\s\)]+)(?:\s+"[^"]*")?\)"#
    )
    
    /// `![alt](url)` — standard markdown image.
    private static let markdownImageRegex = try? NSRegularExpression(
        pattern: #"!\[(.*?)\]\(([^\s\)]+)(?:\s+"[^"]*")?\)"#
    )
    
    /// `MEDIA:/path/to/file.png` (optionally preceded by whitespace or `<br>`).
    private static let mediaPrefixRegex = try? NSRegularExpression(
        pattern: #"(?:^|\s|<br\s*/?>)MEDIA:\s*([^\s)<>"'`]+)"#,
        options: [.caseInsensitive]
    )
    
    private static let wrappingURLChars = CharacterSet(charactersIn: "`\"'()[]<>")
    
    private static func cleanImageURL(_ raw: String) -> String {
        raw.trimmingCharacters(in: .whitespacesAndNewlines)
            .trimmingCharacters(in: wrappingURLChars)
    }
    
    private static func filenameAlt(for url: String) -> String {
        let trimmed = cleanImageURL(url)
        if let parsed = URL(string: trimmed), !parsed.lastPathComponent.isEmpty {
            return parsed.lastPathComponent
        }
        return (trimmed as NSString).lastPathComponent
    }
    
    private static func codeSpanIndexSet(in text: String) -> IndexSet {
        var occupied = IndexSet()
        var loc = 0
        for seg in MarkdownContentView.splitCodeSpans(in: text) {
            let len = (seg.content as NSString).length
            if seg.isCode && len > 0 {
                occupied.insert(integersIn: loc ..< (loc + len))
            }
            loc += len
        }
        return occupied
    }
    
    fileprivate static func findImages(in text: String) -> [(alt: String, url: String, range: NSRange)] {
        guard !text.isEmpty else { return [] }
        let ns = text as NSString
        let full = NSRange(location: 0, length: ns.length)
        let codeSpans = text.contains("`") ? codeSpanIndexSet(in: text) : IndexSet()
        var occupied = IndexSet()
        var results: [(alt: String, url: String, range: NSRange)] = []
        
        func overlapsCode(_ range: NSRange) -> Bool {
            guard range.length > 0 else { return false }
            return !codeSpans.intersection(IndexSet(integersIn: range.location ..< (range.location + range.length))).isEmpty
        }
        
        func add(alt: String, url: String, range: NSRange) {
            guard range.location != NSNotFound, range.length > 0 else { return }
            let cleaned = cleanImageURL(url)
            guard !cleaned.isEmpty else { return }
            if overlapsCode(range) { return }
            let indices = IndexSet(integersIn: range.location ..< (range.location + range.length))
            guard occupied.intersection(indices).isEmpty else { return }
            occupied.formUnion(indices)
            results.append((alt.trimmingCharacters(in: .whitespacesAndNewlines), cleaned, range))
        }
        
        if let regex = linkedImageRegex {
            for match in regex.matches(in: text, range: full) {
                let alt = match.range(at: 1).location != NSNotFound ? ns.substring(with: match.range(at: 1)) : ""
                let orig = match.range(at: 3).location != NSNotFound ? ns.substring(with: match.range(at: 3)) : ""
                add(alt: alt, url: orig, range: match.range)
            }
        }
        
        if let regex = markdownImageRegex {
            for match in regex.matches(in: text, range: full) {
                let alt = match.range(at: 1).location != NSNotFound ? ns.substring(with: match.range(at: 1)) : ""
                let url = match.range(at: 2).location != NSNotFound ? ns.substring(with: match.range(at: 2)) : ""
                add(alt: alt, url: url, range: match.range)
            }
        }
        
        if let regex = mediaPrefixRegex {
            for match in regex.matches(in: text, range: full) {
                let url = match.range(at: 1).location != NSNotFound ? ns.substring(with: match.range(at: 1)) : ""
                add(alt: filenameAlt(for: url), url: url, range: match.range)
            }
        }
        
        results.sort { $0.range.location < $1.range.location }
        return results
    }
    
    fileprivate static func parseStandaloneImage(_ trimmed: String) -> (alt: String, url: String)? {
        let images = findImages(in: trimmed)
        guard images.count == 1 else { return nil }
        let img = images[0]
        let ns = trimmed as NSString
        let before = ns.substring(to: img.range.location)
        let afterStart = img.range.location + img.range.length
        let after = afterStart < ns.length ? ns.substring(from: afterStart) : ""
        guard before.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty,
              after.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else {
            return nil
        }
        return (img.alt, img.url)
    }
    
    fileprivate static func splitParagraphIntoBlocks(text: String, blockIdx: inout Int) -> [MarkdownBlock] {
        let images = findImages(in: text)
        if images.isEmpty {
            let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
            guard !trimmed.isEmpty else { return [] }
            let block = MarkdownBlock.paragraph(id: "block-\(blockIdx)", text: text)
            blockIdx += 1
            return [block]
        }
        
        var blocks: [MarkdownBlock] = []
        let ns = text as NSString
        var cursor = 0
        
        for img in images {
            if img.range.location > cursor {
                let prefix = ns.substring(with: NSRange(location: cursor, length: img.range.location - cursor))
                if !prefix.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                    blocks.append(.paragraph(id: "block-\(blockIdx)", text: prefix.trimmingCharacters(in: .whitespacesAndNewlines)))
                    blockIdx += 1
                }
            }
            blocks.append(.image(id: "block-\(blockIdx)", alt: img.alt, url: img.url))
            blockIdx += 1
            cursor = img.range.location + img.range.length
        }
        
        if cursor < ns.length {
            let suffix = ns.substring(from: cursor)
            if !suffix.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                blocks.append(.paragraph(id: "block-\(blockIdx)", text: suffix.trimmingCharacters(in: .whitespacesAndNewlines)))
                blockIdx += 1
            }
        }
        
        return blocks
    }
    
    private static func appendListOrSplitImages(_ listItems: [String], into blocks: inout [MarkdownBlock], blockIdx: inout Int) {
        var currentPlain: [String] = []
        
        func flushPlainList() {
            guard !currentPlain.isEmpty else { return }
            blocks.append(.list(id: "block-\(blockIdx)", items: currentPlain))
            blockIdx += 1
            currentPlain.removeAll(keepingCapacity: true)
        }
        
        for item in listItems {
            if findImages(in: item).isEmpty {
                currentPlain.append(item)
            } else {
                flushPlainList()
                let split = splitParagraphIntoBlocks(text: item, blockIdx: &blockIdx)
                blocks.append(contentsOf: split)
            }
        }
        flushPlainList()
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
    
    public struct LayoutRow {
        public var elements: [(subview: LayoutSubview, size: CGSize, origin: CGPoint)]
        public var frame: CGRect
    }
    
    public struct LayoutResult {
        public var size: CGSize
        public var rows: [LayoutRow]
    }
    
    private func computeLayout(proposal: ProposedViewSize, subviews: Subviews) -> LayoutResult {
        let maxAvailableWidth = proposal.width ?? .infinity
        var rows: [LayoutRow] = []
        var currentRowElements: [(subview: LayoutSubview, size: CGSize, origin: CGPoint)] = []
        
        var currentX: CGFloat = 0
        var currentY: CGFloat = 0
        var currentLineHeight: CGFloat = 0
        var maxWidth: CGFloat = 0
        
        for subview in subviews {
            let size = subview.sizeThatFits(ProposedViewSize(width: maxAvailableWidth, height: nil))
            
            if currentX + size.width > maxAvailableWidth + 0.5 && currentX > 0 {
                let rowFrame = CGRect(
                    x: 0,
                    y: currentY,
                    width: max(0, currentX - horizontalSpacing),
                    height: currentLineHeight
                )
                rows.append(LayoutRow(elements: currentRowElements, frame: rowFrame))
                
                currentRowElements = []
                currentX = 0
                currentY += currentLineHeight + verticalSpacing
                currentLineHeight = 0
            }
            
            currentRowElements.append((subview: subview, size: size, origin: CGPoint(x: currentX, y: 0)))
            currentLineHeight = max(currentLineHeight, size.height)
            currentX += size.width + horizontalSpacing
            maxWidth = max(maxWidth, currentX - horizontalSpacing)
        }
        
        if !currentRowElements.isEmpty {
            let rowFrame = CGRect(
                x: 0,
                y: currentY,
                width: max(0, currentX - horizontalSpacing),
                height: currentLineHeight
            )
            rows.append(LayoutRow(elements: currentRowElements, frame: rowFrame))
            currentY += currentLineHeight
        }
        
        let totalWidth = min(maxWidth, maxAvailableWidth)
        let totalHeight = currentY
        
        return LayoutResult(size: CGSize(width: totalWidth, height: totalHeight), rows: rows)
    }
    
    public func sizeThatFits(proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) -> CGSize {
        let result = computeLayout(proposal: proposal, subviews: subviews)
        return result.size
    }
    
    public func placeSubviews(in bounds: CGRect, proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) {
        let result = computeLayout(
            proposal: ProposedViewSize(width: bounds.width, height: bounds.height),
            subviews: subviews
        )
        
        for row in result.rows {
            let rowY = bounds.minY + row.frame.origin.y
            let rowHeight = row.frame.height
            for item in row.elements {
                let x = bounds.minX + item.origin.x
                let y = rowY + (rowHeight - item.size.height) / 2
                item.subview.place(at: CGPoint(x: x, y: y), proposal: ProposedViewSize(item.size))
            }
        }
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
