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

public enum MarkdownBlock: Identifiable {
    case frontmatter(id: String, rawContent: String, lineCount: Int)
    case heading(id: String, level: Int, text: String)
    case divider(id: String)
    case codeBlock(id: String, lang: String, code: String)
    case table(id: String, headers: [String], rows: [[String]], alignments: [TableColumnAlignment] = [])
    case list(id: String, items: [String])
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
        case .table(let id, _, _, _): return id
        case .list(let id, _): return id
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

    func blocks(for text: String) -> [MarkdownBlock] {
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

        lock.lock()
        if storage.count >= maxEntries, storage[text] == nil {
            if let oldestKey = storage.min(by: { $0.value.generation < $1.value.generation })?.key {
                storage.removeValue(forKey: oldestKey)
            }
        }
        generationCounter &+= 1
        storage[text] = CacheEntry(blocks: parsed, generation: generationCounter)

        if let pEntry = updatedPrefixEntry {
            prefixEntries.removeAll { $0.prefix == pEntry.prefix }
            prefixEntries.insert(pEntry, at: 0)
            if prefixEntries.count > maxPrefixEntries {
                prefixEntries.removeLast(prefixEntries.count - maxPrefixEntries)
            }
        }
        lock.unlock()
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
