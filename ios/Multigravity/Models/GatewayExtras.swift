import Foundation

// MARK: - 会话内容搜索 / 导出

/// 命中区间，偏移量按 Unicode 码点（scalar）计，不是 UTF-16 单元。
public struct SearchMatchRange: Codable, Sendable, Hashable {
    public let startOffset: Int
    public let endOffsetExclusive: Int
}

public struct ConversationSearchResult: Codable, Sendable, Identifiable, Hashable {
    public var id: String { cascadeId }
    public let cascadeId: String
    public let title: String
    public let workspaceName: String?
    public let lastModifiedTime: String?
    public let snippet: String
    public let snippetMatchRanges: [SearchMatchRange]
    public let matchSource: String?
    public let matchedStepIndex: Int?

    enum CodingKeys: String, CodingKey {
        case cascadeId, title, workspaceName, lastModifiedTime, snippet, snippetMatchRanges, matchSource, matchedStepIndex
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        cascadeId = try c.decode(String.self, forKey: .cascadeId)
        title = try c.decodeIfPresent(String.self, forKey: .title) ?? ""
        workspaceName = try c.decodeIfPresent(String.self, forKey: .workspaceName)
        lastModifiedTime = try c.decodeIfPresent(String.self, forKey: .lastModifiedTime)
        snippet = try c.decodeIfPresent(String.self, forKey: .snippet) ?? ""
        snippetMatchRanges = try c.decodeIfPresent([SearchMatchRange].self, forKey: .snippetMatchRanges) ?? []
        matchSource = try c.decodeIfPresent(String.self, forKey: .matchSource)
        matchedStepIndex = try c.decodeIfPresent(Int.self, forKey: .matchedStepIndex)
    }
}

struct ConversationSearchRequest: Encodable {
    let query: String
}

struct ConversationSearchResponse: Decodable {
    let results: [ConversationSearchResult]

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        results = try c.decodeIfPresent([ConversationSearchResult].self, forKey: .results) ?? []
    }
    enum CodingKeys: String, CodingKey { case results }
}

struct MarkdownExportRequest: Encodable {
    let conversationId: String
}

struct MarkdownExportResponse: Decodable {
    let markdown: String
}

/// 搜索片段高亮：把码点区间换算成 `AttributedString`。越界或重叠的区间会被丢弃。
public enum SearchSnippetHighlighter {
    /// 返回 `snippet.unicodeScalars` 上的合法区间（已按起点排序、互不重叠）。
    static func scalarRanges(snippetScalarCount total: Int, ranges: [SearchMatchRange]) -> [Range<Int>] {
        var out: [Range<Int>] = []
        var lastEnd = 0
        for r in ranges.sorted(by: { $0.startOffset < $1.startOffset }) {
            let s = min(max(r.startOffset, 0), total)
            let e = min(max(r.endOffsetExclusive, 0), total)
            if e <= s || s < lastEnd { continue }
            out.append(s..<e)
            lastEnd = e
        }
        return out
    }

    public static func attributed(snippet: String, ranges: [SearchMatchRange], highlight: AttributeContainer) -> AttributedString {
        var result = AttributedString(snippet)
        guard !snippet.isEmpty, !ranges.isEmpty else { return result }
        let scalars = Array(snippet.unicodeScalars)
        for r in scalarRanges(snippetScalarCount: scalars.count, ranges: ranges) {
            var start = snippet.unicodeScalars.startIndex
            snippet.unicodeScalars.formIndex(&start, offsetBy: r.lowerBound)
            var end = start
            snippet.unicodeScalars.formIndex(&end, offsetBy: r.count)
            guard let lower = AttributedString.Index(start, within: result),
                  let upper = AttributedString.Index(end, within: result) else { continue }
            result[lower..<upper].mergeAttributes(highlight)
        }
        return result
    }
}


