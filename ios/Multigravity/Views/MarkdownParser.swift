import SwiftUI

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
            
            // Ordered list: 1. or 1)
            if let match = parseOrderedListItem(trimmed) {
                var listItems: [String] = [match.content]
                let startNumber = match.number
                i += 1
                while i < lines.count {
                    let lLine = lines[i].trimmingCharacters(in: .whitespaces)
                    if let nextMatch = parseOrderedListItem(lLine) {
                        listItems.append(nextMatch.content)
                        i += 1
                    } else {
                        break
                    }
                }
                appendOrderedListOrSplitImages(startIndex: startNumber, listItems: listItems, into: &blocks, blockIdx: &blockIdx)
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
                if nTrimmed.isEmpty || nTrimmed.hasPrefix("```") || nTrimmed.hasPrefix("#") || nTrimmed == "---" || (nTrimmed.hasPrefix("|") && nTrimmed.hasSuffix("|")) || nTrimmed.hasPrefix("- ") || nTrimmed.hasPrefix("* ") || nTrimmed.hasPrefix("• ") || parseOrderedListItem(nTrimmed) != nil || parseStandaloneImage(nTrimmed) != nil {
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
        MarkdownContentView.codeSpanIndexSet(in: text)
    }
    
    static func findImages(in text: String) -> [(alt: String, url: String, range: NSRange)] {
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
    
    static func parseStandaloneImage(_ trimmed: String) -> (alt: String, url: String)? {
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
    
    static func splitParagraphIntoBlocks(text: String, blockIdx: inout Int) -> [MarkdownBlock] {
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
    
    private static let orderedListRegex = try? NSRegularExpression(
        pattern: #"^(\d{1,9})[\.\)]\s+(.*)$"#
    )
    
    static func parseOrderedListItem(_ line: String) -> (number: Int, content: String)? {
        guard let regex = orderedListRegex else { return nil }
        let ns = line as NSString
        guard let match = regex.firstMatch(in: line, range: NSRange(location: 0, length: ns.length)) else {
            return nil
        }
        let numStr = ns.substring(with: match.range(at: 1))
        let contentStr = ns.substring(with: match.range(at: 2))
        guard let num = Int(numStr) else { return nil }
        return (number: num, content: contentStr)
    }
    
    private static func appendOrderedListOrSplitImages(startIndex: Int, listItems: [String], into blocks: inout [MarkdownBlock], blockIdx: inout Int) {
        var currentPlain: [(offset: Int, text: String)] = []
        
        func flushPlainOrderedList() {
            guard !currentPlain.isEmpty else { return }
            let firstOffset = currentPlain.first!.offset
            let items = currentPlain.map(\.text)
            blocks.append(.orderedList(id: "block-\(blockIdx)", startIndex: startIndex + firstOffset, items: items))
            blockIdx += 1
            currentPlain.removeAll(keepingCapacity: true)
        }
        
        for (idx, item) in listItems.enumerated() {
            if findImages(in: item).isEmpty {
                currentPlain.append((offset: idx, text: item))
            } else {
                flushPlainOrderedList()
                let split = splitParagraphIntoBlocks(text: item, blockIdx: &blockIdx)
                blocks.append(contentsOf: split)
            }
        }
        flushPlainOrderedList()
    }
}
