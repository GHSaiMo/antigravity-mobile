import Foundation

// MARK: - 本会话改动（正向累计 diff）

/// 文件结构与撤回预览一致，直接复用 `RevertPreviewFile` 与 `DiffViewerSheet`。
public struct CascadeChangesResponse: Codable, Sendable {
    public let cascadeId: String
    public let files: [RevertPreviewFile]
    public let additions: Int
    public let deletions: Int
    public let hasChanges: Bool
}

struct CascadeChangesRequest: Encodable {
    let cascadeId: String
    var fromStepIndex: Int? = nil
}

// MARK: - Git

public struct GitFileStatus: Codable, Sendable, Identifiable, Hashable {
    public var id: String { path }
    public let path: String
    public let origPath: String?
    public let index: String
    public let worktree: String
    public let staged: Bool
    /// MODIFIED / ADDED / DELETED / RENAMED / UNTRACKED / CONFLICT
    public let status: String
}

public struct GitStatusResponse: Codable, Sendable {
    public let repoName: String
    public let branch: String
    public let upstream: String?
    public let ahead: Int
    public let behind: Int
    public let detached: Bool
    public let files: [GitFileStatus]
    public let clean: Bool
}

struct GitCascadeRequest: Encodable {
    let cascadeId: String
}

struct GitCommitRequest: Encodable {
    let cascadeId: String
    let message: String
    let paths: [String]
    let push: Bool
}

public struct GitCommitResponse: Codable, Sendable {
    public var committed: Bool
    public var pushed: Bool
    public var commitId: String?
    public var output: String?
    /// 提交已成功但推送失败时有值，供客户端单独重试推送。
    public var pushError: String?
}

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


// MARK: - 斜杠命令

/// 网关 /gateway/slash-commands 返回的一项。发送时只需把 `name` 带回去，网关会补全权威定义。
public struct SlashCommandOption: Codable, Sendable, Identifiable, Hashable {
    public var id: String { name }
    public let name: String
    public let title: String
    public let description: String
    public let icon: String?
    /// "system" 系统命令 | "skill" 技能。
    public let kind: String

    public init(name: String, title: String = "", description: String = "", icon: String? = nil, kind: String = "system") {
        self.name = name
        self.title = title
        self.description = description
        self.icon = icon
        self.kind = kind
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        name = try c.decode(String.self, forKey: .name)
        title = try c.decodeIfPresent(String.self, forKey: .title) ?? name
        description = try c.decodeIfPresent(String.self, forKey: .description) ?? ""
        icon = try c.decodeIfPresent(String.self, forKey: .icon)
        kind = try c.decodeIfPresent(String.self, forKey: .kind) ?? "system"
    }
    enum CodingKeys: String, CodingKey { case name, title, description, icon, kind }
}

struct SlashCommandsResponse: Decodable {
    let commands: [SlashCommandOption]

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        commands = try c.decodeIfPresent([SlashCommandOption].self, forKey: .commands) ?? []
    }
    enum CodingKeys: String, CodingKey { case commands }
}

/// 输入框里 "/xxx" 的筛选规则（纯函数，便于单测）。
public enum SlashCommandFilter {
    /// 输入以 "/" 开头且尚未出现空白时，返回要筛选的关键字（不含 "/"）；否则返回 nil，表示不弹出命令列表。
    /// "/" -> ""，"/pl" -> "pl"，"/plan 帮我" -> nil，"hello" -> nil。
    public static func query(of input: String) -> String? {
        guard input.hasPrefix("/") else { return nil }
        let rest = input.dropFirst()
        return rest.contains(where: { $0.isWhitespace }) ? nil : String(rest)
    }

    /// 名称前缀命中优先，其次名称/标题/描述包含；保持原有顺序。
    public static func filter(_ all: [SlashCommandOption], query: String) -> [SlashCommandOption] {
        let q = query.trimmingCharacters(in: .whitespaces).lowercased()
        if q.isEmpty { return all }
        let prefix = all.filter { $0.name.lowercased().hasPrefix(q) }
        let prefixNames = Set(prefix.map(\.name))
        let rest = all.filter {
            !prefixNames.contains($0.name) &&
            ($0.name.lowercased().contains(q) || $0.title.lowercased().contains(q) || $0.description.lowercased().contains(q))
        }
        return prefix + rest
    }

    /// 把 "/plan 帮我规划" 拆成（命令，"帮我规划"）；开头不是已知命令（或命令列表还没加载）时原样返回。
    /// 用于队列里的「编辑 / 立即发送」：队列显示的是服务端生成的显示文本，需要还原成「命令 + 文本」。
    public static func splitPrefix(_ text: String, commands: [SlashCommandOption]) -> (command: SlashCommandOption?, rest: String) {
        guard text.hasPrefix("/"), !commands.isEmpty else { return (nil, text) }
        let afterSlash = text.dropFirst()
        let token = String(afterSlash.prefix(while: { !$0.isWhitespace }))
        guard let command = commands.first(where: { $0.name == token }) else { return (nil, text) }
        let rest = afterSlash.dropFirst(token.count).drop(while: { $0.isWhitespace })
        return (command, String(rest))
    }
}
