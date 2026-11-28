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

public enum MarkdownAlertType: String, Sendable, CaseIterable {
    case note
    case tip
    case important
    case warning
    case caution
    
    public var title: String {
        switch self {
        case .note: return "Note"
        case .tip: return "Tip"
        case .important: return "Important"
        case .warning: return "Warning"
        case .caution: return "Caution"
        }
    }
    
    public var systemImage: String {
        switch self {
        case .note: return "info.circle.fill"
        case .tip: return "lightbulb.fill"
        case .important: return "exclamationmark.circle.fill"
        case .warning: return "exclamationmark.triangle.fill"
        case .caution: return "octagon.fill"
        }
    }
    
    public var accentColor: Color {
        switch self {
        case .note: return Color(red: 10/255, green: 132/255, blue: 255/255)       // #0A84FF
        case .tip: return Color(red: 48/255, green: 209/255, blue: 88/255)         // #30D158
        case .important: return Color(red: 191/255, green: 90/255, blue: 242/255) // #BF5AF2
        case .warning: return Color(red: 255/255, green: 159/255, blue: 10/255)    // #FF9F0A
        case .caution: return Color(red: 255/255, green: 69/255, blue: 58/255)     // #FF453A
        }
    }
}

public enum MarkdownBlock: Identifiable, Equatable {
    case frontmatter(id: String, rawContent: String, lineCount: Int)
    case heading(id: String, level: Int, text: String)
    case divider(id: String)
    case codeBlock(id: String, lang: String, code: String)
    case mathBlock(id: String, latex: String)
    case table(id: String, headers: [String], rows: [[String]], alignments: [TableColumnAlignment] = [])
    case list(id: String, items: [String], indents: [Int] = [])
    case orderedList(id: String, startIndex: Int, items: [String])
    case alert(id: String, type: MarkdownAlertType, content: String)
    case blockquote(id: String, content: String)
    case paragraph(id: String, text: String)
    case image(id: String, alt: String, url: String)
    case agentEmbed(id: String, src: String)
    case pluginCard(id: String, catalogId: String)
    case carousel(id: String, slides: [MarkdownCarouselSlide])
    
    public var id: String {
        switch self {
        case .frontmatter(let id, _, _): return id
        case .heading(let id, _, _): return id
        case .divider(let id): return id
        case .codeBlock(let id, _, _): return id
        case .mathBlock(let id, _): return id
        case .table(let id, _, _, _): return id
        case .list(let id, _, _): return id
        case .orderedList(let id, _, _): return id
        case .alert(let id, _, _): return id
        case .blockquote(let id, _): return id
        case .paragraph(let id, _): return id
        case .image(let id, _, _): return id
        case .agentEmbed(let id, _): return id
        case .pluginCard(let id, _): return id
        case .carousel(let id, _): return id
        }
    }
}

public struct MarkdownCarouselSlide: Identifiable, Sendable, Equatable {
    public let id: String
    public let title: String?
    public let content: String
    
    public init(id: String, title: String? = nil, content: String) {
        self.id = id
        self.title = title
        self.content = content
    }
}

final class MarkdownBlockCache: @unchecked Sendable {
    static let shared = MarkdownBlockCache()

    private struct CacheEntry {
        let blocks: [MarkdownBlock]
        var generation: UInt64
    }

    private struct PrefixEntry {
        let prefix: String
        let blocks: [MarkdownBlock]
    }

    private let lock = NSLock()
    private let maxEntries = 200
    private let maxPrefixEntries = 32
    private var generationCounter: UInt64 = 0
    private var storage: [String: CacheEntry] = [:]
    private var prefixEntries: [PrefixEntry] = []

    func blocks(for text: String, cacheExactResult: Bool = true) -> [MarkdownBlock] {
        if text.isEmpty { return [] }

        var matchedPrefix: PrefixEntry? = nil
        lock.lock()
        if var hit = storage[text] {
            generationCounter &+= 1
            hit.generation = generationCounter
            storage[text] = hit
            lock.unlock()
            return hit.blocks
        }
        if text.count >= 120 {
            for idx in prefixEntries.indices {
                let candidate = prefixEntries[idx]
                if text.count > candidate.prefix.count && text.hasPrefix(candidate.prefix) {
                    matchedPrefix = candidate
                    if idx > 0 {
                        prefixEntries.remove(at: idx)
                        prefixEntries.insert(candidate, at: 0)
                    }
                    break
                }
            }
        }
        lock.unlock()

        let safeSplitIndex = Self.findLastSafeBlankSplit(in: text)
        let parsed: [MarkdownBlock]
        var updatedPrefixEntry: PrefixEntry? = nil

        if let base = matchedPrefix {
            if let splitIdx = safeSplitIndex {
                let safePrefix = String(text[..<splitIdx])
                if safePrefix.count > base.prefix.count && safePrefix.hasPrefix(base.prefix) {
                    let midDelta = String(safePrefix.dropFirst(base.prefix.count))
                    let midBlocks = MarkdownParser.parse(midDelta, startBlockIdx: base.blocks.count)
                    let combinedPrefixBlocks = base.blocks + midBlocks
                    if !combinedPrefixBlocks.isEmpty {
                        updatedPrefixEntry = PrefixEntry(prefix: safePrefix, blocks: combinedPrefixBlocks)
                    }
                    let tail = String(text[splitIdx...])
                    let tailBlocks = MarkdownParser.parse(tail, startBlockIdx: combinedPrefixBlocks.count)
                    parsed = combinedPrefixBlocks + tailBlocks
                } else {
                    let tail = String(text.dropFirst(base.prefix.count))
                    let tailBlocks = MarkdownParser.parse(tail, startBlockIdx: base.blocks.count)
                    parsed = base.blocks + tailBlocks
                }
            } else {
                let tail = String(text.dropFirst(base.prefix.count))
                let tailBlocks = MarkdownParser.parse(tail, startBlockIdx: base.blocks.count)
                parsed = base.blocks + tailBlocks
            }
        } else if let splitIdx = safeSplitIndex {
            let safePrefix = String(text[..<splitIdx])
            let prefixBlocks = MarkdownParser.parse(safePrefix, startBlockIdx: 0)
            if !prefixBlocks.isEmpty {
                updatedPrefixEntry = PrefixEntry(prefix: safePrefix, blocks: prefixBlocks)
            }
            let tail = String(text[splitIdx...])
            let tailBlocks = MarkdownParser.parse(tail, startBlockIdx: prefixBlocks.count)
            parsed = prefixBlocks + tailBlocks
        } else {
            parsed = MarkdownParser.parse(text, startBlockIdx: 0)
        }

        if cacheExactResult || updatedPrefixEntry != nil {
            lock.lock()
            if cacheExactResult {
                if storage.count >= maxEntries, storage[text] == nil {
                    if let oldestKey = storage.min(by: { $0.value.generation < $1.value.generation })?.key {
                        storage.removeValue(forKey: oldestKey)
                    }
                }
                generationCounter &+= 1
                storage[text] = CacheEntry(blocks: parsed, generation: generationCounter)
            }

            if let pEntry = updatedPrefixEntry {
                prefixEntries.removeAll { $0.prefix == pEntry.prefix }
                prefixEntries.insert(pEntry, at: 0)
                if prefixEntries.count > maxPrefixEntries {
                    prefixEntries.removeLast(prefixEntries.count - maxPrefixEntries)
                }
            }
            lock.unlock()
        }
        return parsed
    }

    /// Finds the `String.Index` right after the last top-level blank line (`\n\n`) that is guaranteed
    /// not to be inside YAML/Marp frontmatter, `<style>`, fenced code blocks, or display math blocks.
    private static func findLastSafeBlankSplit(in text: String) -> String.Index? {
        guard text.count >= 120 else { return nil }

        var firstNonEmptySeen = false
        var inFrontmatter = false
        var frontmatterDashOnly = false
        var frontmatterBodyLines = 0
        var inStyle = false
        var inFence = false
        var fenceChar: Character = "`"
        var fenceLen = 0
        var inMath = false
        var mathCloseToken = "$$"
        var hasCompletedContent = false
        var lastSafeIndex: String.Index? = nil

        var lineStart = text.startIndex
        while lineStart < text.endIndex {
            let newlineIdx = text[lineStart...].firstIndex(of: "\n")
            let lineEnd = newlineIdx ?? text.endIndex
            let afterLine = newlineIdx.map { text.index(after: $0) } ?? text.endIndex
            let lineSub = text[lineStart..<lineEnd]
            let trimmed = lineSub.trimmingCharacters(in: .whitespaces)

            if !firstNonEmptySeen {
                if !trimmed.isEmpty {
                    firstNonEmptySeen = true
                    if trimmed == "---" {
                        inFrontmatter = true
                        frontmatterDashOnly = false
                        frontmatterBodyLines = 0
                        lineStart = afterLine
                        continue
                    } else if trimmed.hasPrefix("marp:") || (trimmed.contains(":") && (trimmed.hasPrefix("theme:") || trimmed.hasPrefix("style:"))) {
                        inFrontmatter = true
                        frontmatterDashOnly = true
                        frontmatterBodyLines = 1
                        lineStart = afterLine
                        continue
                    }
                }
            } else if inFrontmatter {
                if frontmatterDashOnly {
                    if trimmed == "---" {
                        inFrontmatter = false
                        hasCompletedContent = true
                    } else if trimmed.hasPrefix("# ") || trimmed.hasPrefix("## ") {
                        // Unclosed Marp header fallback — bail out of incremental split for safety
                        return nil
                    } else {
                        frontmatterBodyLines += 1
                    }
                } else {
                    if trimmed == "---" || trimmed == "..." {
                        if frontmatterBodyLines > 0 {
                            inFrontmatter = false
                            hasCompletedContent = true
                        } else {
                            // Leading `---` followed immediately by `---` is not frontmatter; skip incremental split
                            return nil
                        }
                    } else {
                        frontmatterBodyLines += 1
                    }
                }
                lineStart = afterLine
                continue
            }

            if inStyle {
                if trimmed.lowercased().contains("</style>") {
                    inStyle = false
                    hasCompletedContent = true
                }
                lineStart = afterLine
                continue
            }

            if inFence {
                if trimmed.hasPrefix(String(repeating: fenceChar, count: fenceLen)) {
                    inFence = false
                    hasCompletedContent = true
                }
                lineStart = afterLine
                continue
            }

            if inMath {
                if trimmed.contains(mathCloseToken) {
                    inMath = false
                    hasCompletedContent = true
                }
                lineStart = afterLine
                continue
            }

            if trimmed.isEmpty {
                if hasCompletedContent && newlineIdx != nil && afterLine < text.endIndex {
                    lastSafeIndex = afterLine
                }
                lineStart = afterLine
                continue
            }

            let lower = trimmed.lowercased()
            if lower.hasPrefix("<style") {
                if !lower.contains("</style>") {
                    inStyle = true
                } else {
                    hasCompletedContent = true
                }
                lineStart = afterLine
                continue
            }

            if trimmed.hasPrefix("```") || trimmed.hasPrefix("~~~") {
                let ch = trimmed.first!
                var count = 0
                for c in trimmed {
                    if c == ch { count += 1 } else { break }
                }
                if count >= 3 {
                    inFence = true
                    fenceChar = ch
                    fenceLen = count
                    lineStart = afterLine
                    continue
                }
            }

            if (trimmed.hasPrefix("$$") && (trimmed == "$$" || !trimmed.dropFirst(2).contains("$$"))) ||
               (trimmed.hasPrefix(#"\["#) && !trimmed.dropFirst(2).contains(#"\]"#)) {
                inMath = true
                mathCloseToken = trimmed.hasPrefix("$$") ? "$$" : #"\]"#
                lineStart = afterLine
                continue
            }

            hasCompletedContent = true
            lineStart = afterLine
        }

        if inFrontmatter {
            return nil
        }
        return lastSafeIndex
    }
}

/// 流式渲染尾行稳定器：仅在消息仍处于活跃流式输出时启用，用于消除以下导致气泡高度剧烈暴涨后瞬间坍缩的闪烁源：
/// 1. 尚未闭合的 Markdown 长链接 `[label](file:///...` 先撑高 3~4 行、闭合 `)` 瞬间回弹坍缩；
/// 2. 尚未闭合的 Markdown 图片 `![alt](https://...`；
/// 3. 表格逐行流式输出时，未闭合的 `| col1 | col2` 先被解析成独立段落出现在表格下方、闭合 `|` 后又消失并入表格；
/// 4. 行首半截块级标记（`#`、`##`、`-`、`*`、`1.`、`>`）短暂生成占位段落随后变形。
/// 处于未闭合代码块（``` / ~~~）或数学块内时完全不做干预；流式结束瞬间回落到 100% 原始文本。
enum StreamingMarkdownStabilizer {
    static func stabilize(_ text: String) -> String {
        guard !text.isEmpty else { return text }
        if isInsideOpenFenceOrMath(text) {
            return text
        }

        let lastNewlineIdx = text.lastIndex(of: "\n")
        let lastLineStart = lastNewlineIdx.map { text.index(after: $0) } ?? text.startIndex
        let lastLine = String(text[lastLineStart...])
        let trimmedLast = lastLine.trimmingCharacters(in: .whitespaces)

        // 1. 行首仅有未写完的块级语法前缀（尚无正文）：暂隐该半截尾行，避免先出现一段原始 "##"/"-"/"1." 再变形跳动
        if isBareBlockMarkerStub(trimmedLast) {
            if let nl = lastNewlineIdx {
                return String(text[..<nl])
            }
            return text
        }

        // 2. 紧跟在已完成管道表格行后的未闭合表格行 `| ...`：暂缓至收尾 `|` 到达后再整行并入表格
        if trimmedLast.hasPrefix("|") && !trimmedLast.hasSuffix("|"),
           let nl = lastNewlineIdx,
           previousNonEmptyLineIsPipeTable(in: text[..<nl]) {
            return String(text[..<nl])
        }

        // 3. 尾行末尾存在尚未闭合的 `[label](partial_url` 或 `![alt](partial_url`
        if lastLine.contains("]("),
           let stabilizedTail = stabilizeUnclosedTrailingLink(in: lastLine) {
            let trimmedTail = stabilizedTail.trimmingCharacters(in: .whitespaces)
            if trimmedTail.isEmpty || isBareBlockMarkerStub(trimmedTail) {
                if let nl = lastNewlineIdx {
                    return String(text[..<nl])
                }
            }
            return String(text[..<lastLineStart]) + stabilizedTail
        }

        return text
    }

    private static func isInsideOpenFenceOrMath(_ text: String) -> Bool {
        var inFence = false
        var fenceChar: Character = "`"
        var fenceLen = 0
        var inMath = false
        var mathClose = "$$"

        var lineStart = text.startIndex
        while lineStart < text.endIndex {
            let nl = text[lineStart...].firstIndex(of: "\n")
            let lineEnd = nl ?? text.endIndex
            let trimmed = text[lineStart..<lineEnd].trimmingCharacters(in: .whitespaces)

            if inFence {
                if trimmed.hasPrefix(String(repeating: fenceChar, count: fenceLen)) {
                    inFence = false
                }
            } else if inMath {
                if trimmed.contains(mathClose) {
                    inMath = false
                }
            } else if trimmed.hasPrefix("```") || trimmed.hasPrefix("~~~") {
                let ch = trimmed.first!
                var count = 0
                for c in trimmed {
                    if c == ch { count += 1 } else { break }
                }
                if count >= 3 {
                    inFence = true
                    fenceChar = ch
                    fenceLen = count
                }
            } else if (trimmed.hasPrefix("$$") && (trimmed == "$$" || !trimmed.dropFirst(2).contains("$$"))) ||
                      (trimmed.hasPrefix(#"\["#) && !trimmed.dropFirst(2).contains(#"\]"#)) {
                inMath = true
                mathClose = trimmed.hasPrefix("$$") ? "$$" : #"\]"#
            }

            guard let newlineIdx = nl else { break }
            lineStart = text.index(after: newlineIdx)
        }
        return inFence || inMath
    }

    private static func isBareBlockMarkerStub(_ trimmed: String) -> Bool {
        guard !trimmed.isEmpty, trimmed.count <= 10 else { return false }
        if trimmed == "-" || trimmed == "*" || trimmed == "+" || trimmed == ">" || trimmed == "•" || trimmed == "|" {
            return true
        }
        if trimmed.allSatisfy({ $0 == "#" }) && trimmed.count <= 6 {
            return true
        }
        if (trimmed.hasSuffix(".") || trimmed.hasSuffix(")")) && trimmed.count >= 2 {
            let digits = trimmed.dropLast()
            if !digits.isEmpty && digits.allSatisfy({ $0.isASCII && $0.isNumber }) {
                return true
            }
        }
        return false
    }

    private static func previousNonEmptyLineIsPipeTable(in prefix: Substring) -> Bool {
        var end = prefix.endIndex
        while end > prefix.startIndex {
            let searchSlice = prefix[..<end]
            let prevNl = searchSlice.lastIndex(of: "\n")
            let lineStart = prevNl.map { prefix.index(after: $0) } ?? prefix.startIndex
            let trimmed = prefix[lineStart..<end].trimmingCharacters(in: .whitespaces)
            if !trimmed.isEmpty {
                return trimmed.hasPrefix("|") && trimmed.hasSuffix("|") && trimmed.count >= 3
            }
            if let nl = prevNl {
                end = nl
            } else {
                break
            }
        }
        return false
    }

    /// 扫描单行内容（跳过反引号行内代码与转义字符），仅替换末尾未闭合的 `[label](url...` / `![alt](url...`。
    private static func stabilizeUnclosedTrailingLink(in line: String) -> String? {
        let chars = Array(line)
        let n = chars.count
        var i = 0

        while i < n {
            let ch = chars[i]
            if ch == "\\", i + 1 < n {
                i += 2
                continue
            }
            if ch == "`" {
                let tickStart = i
                while i < n && chars[i] == "`" { i += 1 }
                let tickLen = i - tickStart
                var j = i
                var closed = false
                while j < n {
                    if chars[j] == "`" {
                        let cStart = j
                        while j < n && chars[j] == "`" { j += 1 }
                        if j - cStart == tickLen {
                            i = j
                            closed = true
                            break
                        }
                    } else {
                        j += 1
                    }
                }
                if !closed { return nil }
                continue
            }

            let isImage = (ch == "!" && i + 1 < n && chars[i + 1] == "[")
            let isLink = (ch == "[")
            if isImage || isLink {
                let bracketStart = isImage ? i + 1 : i
                var j = bracketStart + 1
                var bracketDepth = 1
                while j < n {
                    if chars[j] == "\\", j + 1 < n {
                        j += 2
                        continue
                    }
                    if chars[j] == "[" {
                        bracketDepth += 1
                    } else if chars[j] == "]" {
                        bracketDepth -= 1
                        if bracketDepth == 0 { break }
                    }
                    j += 1
                }
                if bracketDepth == 0, j + 1 < n, chars[j + 1] == "(" {
                    var k = j + 2
                    var parenDepth = 1
                    while k < n {
                        if chars[k] == "\\", k + 1 < n {
                            k += 2
                            continue
                        }
                        if chars[k] == "(" {
                            parenDepth += 1
                        } else if chars[k] == ")" {
                            parenDepth -= 1
                            if parenDepth == 0 { break }
                        }
                        k += 1
                    }
                    if parenDepth == 0 {
                        // 已闭合的完整链接，原样跳过
                        i = k + 1
                        continue
                    } else {
                        // 命中文末未闭合链接：链接保留可读 label，图片暂隐未完成的 URL
                        let prefix = String(chars[0..<i])
                        if isImage {
                            return prefix
                        } else {
                            let label = String(chars[(bracketStart + 1)..<j])
                            return prefix + label
                        }
                    }
                }
            }
            i += 1
        }
        return nil
    }
}
