import SwiftUI

extension MarkdownContentView {
    // MARK: - Rich Text with File Icons and Inline Code Styler
    
    final class InlineMarkdownCache: @unchecked Sendable {
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
        text = unwrapBacktickBold(in: text)
        
        // Auto-link bare implementation_plan.md, walkthrough.md, task.md outside code spans
        func autoLinkPlans(in s: String) -> String {
            var res = s
            if res.contains("implementation_plan.md") && !res.contains("[implementation_plan.md]") && !res.contains("](implementation_plan.md)") {
                res = res.replacingOccurrences(of: "implementation_plan.md", with: "[implementation_plan.md](implementation_plan.md)")
            }
            if res.contains("walkthrough.md") && !res.contains("[walkthrough.md]") && !res.contains("](walkthrough.md)") {
                res = res.replacingOccurrences(of: "walkthrough.md", with: "[walkthrough.md](walkthrough.md)")
            }
            if res.contains("task.md") && !res.contains("[task.md]") && !res.contains("](task.md)") {
                res = res.replacingOccurrences(of: "task.md", with: "[task.md](task.md)")
            }
            return res
        }
        
        if text.contains("implementation_plan.md") || text.contains("walkthrough.md") || text.contains("task.md") {
            if text.contains("`") {
                let segments = splitCodeSpans(in: text)
                text = segments.map { segment in
                    segment.isCode ? segment.content : autoLinkPlans(in: segment.content)
                }.joined()
            } else {
                text = autoLinkPlans(in: text)
            }
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
    
    static let cjkDelimiterMarker = "\u{FE50}" // Small comma (Unicode category Po - Punctuation, other)
    
    static let cjkDelimiterPatterns: [NSRegularExpression] = {
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
    
    static let htmlBreakRegex = try? NSRegularExpression(
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
    
    public static func codeSpanIndexSet(in text: String) -> IndexSet {
        guard text.contains("`") else { return IndexSet() }
        var occupied = IndexSet()
        var loc = 0
        for seg in splitCodeSpans(in: text) {
            let len = (seg.content as NSString).length
            if seg.isCode && len > 0 {
                occupied.insert(integersIn: loc ..< (loc + len))
            }
            loc += len
        }
        return occupied
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
    
    // Unwraps bold text mistakenly wrapped in inline code backticks,
    // e.g. "`** BUILD SUCCEEDED **`" -> "**BUILD SUCCEEDED**"
    // so it parses and renders as true bold text rather than raw code with asterisks.
    static let backtickBoldRegex = try? NSRegularExpression(
        pattern: #"`+(\*{2,3})\s*([^\*`\n]+?)\s*\1`+"#
    )
    
    public static func unwrapBacktickBold(in text: String) -> String {
        guard text.contains("`") && text.contains("**"), let regex = backtickBoldRegex else { return text }
        let nsText = text as NSString
        let range = NSRange(location: 0, length: nsText.length)
        return regex.stringByReplacingMatches(in: text, options: [], range: range, withTemplate: "$1$2$1")
    }
    
    // Normalizes inner whitespace in bold markdown, e.g. "** text **" -> " **text** "
    static let boldPairRegex = try? NSRegularExpression(
        pattern: #"(?<!\*)(\*{2,3})((?:[^\*]|\*(?!\*))+?)\1(?!\*)"#
    )
    
    static func normalizeBoldSpaces(in text: String) -> String {
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
            
            let stars = nsText.substring(with: match.range(at: 1))
            let inner = nsText.substring(with: match.range(at: 2))
            
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
                result += leadingSpaces + stars + trimmed + stars + trailingSpaces
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
    static func fixCJKDelimiters(in text: String) -> (fixed: String, hasMarkers: Bool) {
        guard text.contains("*") || text.contains("_") || text.contains("~") else {
            return (text, false)
        }
        
        let segments = splitCodeSpans(in: text)

        // Mask each code span with a single placeholder so emphasis that wraps inline code
        // (e.g. 设定为**`Code`（说明）**) is processed as one run instead of being cut apart.
        var codeSpans: [String] = []
        var masked = ""
        for segment in segments {
            if segment.isCode {
                codeSpans.append(segment.content)
                masked.append(codeSpanPlaceholder)
            } else {
                masked += segment.content
            }
        }
        if !codeSpans.isEmpty && text.contains(codeSpanPlaceholder) {
            return (text, false)
        }

        let normalized = normalizeBoldSpaces(in: masked)
        let (processed, hasMarkers) = processDelimitersInSegment(normalized)

        var result = ""
        var index = 0
        for ch in processed {
            if ch == codeSpanPlaceholder, index < codeSpans.count {
                result += codeSpans[index]
                index += 1
            } else {
                result.append(ch)
            }
        }
        return (result, hasMarkers)
    }

    /// Stand-in for an inline code span while delimiter fixing runs (private-use, treated as punctuation).
    static let codeSpanPlaceholder: Character = "\u{E000}"
    
    static func processDelimitersInSegment(_ text: String) -> (String, Bool) {
        var hasMarkers = false
        var result = text
        
        func isPunctOrSymbol(_ c: Character) -> Bool {
            return c.isPunctuation || c.isSymbol || c == codeSpanPlaceholder
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
        let unwrappedText = unwrapBacktickBold(in: textWithBreaks)
        let (preprocessedText, hasMarkers) = fixCJKDelimiters(in: unwrappedText)
        
        var options = AttributedString.MarkdownParsingOptions()
        options.interpretedSyntax = .inlineOnlyPreservingWhitespace
        guard var attr = try? AttributedString(markdown: preprocessedText, options: options) else {
            let fallback = AttributedString(unwrappedText)
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
    
}
