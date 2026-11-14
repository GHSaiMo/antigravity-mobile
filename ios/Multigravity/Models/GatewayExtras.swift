import Foundation
import UIKit
import CoreGraphics

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

public struct GitHubBranchTarget: Codable, Sendable, Hashable {
    public let host: String?
    public let owner: String
    public let repo: String
    public let branch: String?
}

public struct GitPullRequestItem: Codable, Sendable, Identifiable, Hashable {
    public var id: String { "\(number)-\(url)" }
    public let number: Int
    public let title: String
    public let url: String
    /// OPEN / DRAFT / MERGED / CLOSED
    public let state: String
    public let authorLogin: String?
    public let headRefName: String?
    public let baseRefName: String?
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
    public let githubTarget: GitHubBranchTarget?
    public let pullRequests: [GitPullRequestItem]?
}

// MARK: - 会话章节目录 TOC (Antigravity 2.30.0 GetChapterTitles)

public struct CascadeStepRange: Codable, Sendable, Hashable {
    public let startStepIndex: Int
    public let endStepIndexExclusive: Int

    public init(startStepIndex: Int, endStepIndexExclusive: Int) {
        self.startStepIndex = startStepIndex
        self.endStepIndexExclusive = endStepIndexExclusive
    }
}

public struct CascadeChapterTitleItem: Codable, Sendable, Hashable {
    public let startStepIndex: Int
    public let endStepIndexExclusive: Int
    public let title: String
    public let generatedAt: String?

    enum CodingKeys: String, CodingKey {
        case startStepIndex, endStepIndexExclusive, title, generatedAt
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        startStepIndex = try c.decodeIfPresent(Int.self, forKey: .startStepIndex) ?? 0
        endStepIndexExclusive = try c.decodeIfPresent(Int.self, forKey: .endStepIndexExclusive) ?? 0
        title = try c.decodeIfPresent(String.self, forKey: .title) ?? ""
        generatedAt = try c.decodeIfPresent(String.self, forKey: .generatedAt)
    }
}

struct CascadeChaptersRequest: Encodable {
    let cascadeId: String
    let ranges: [CascadeStepRange]
}

public struct CascadeChaptersResponse: Codable, Sendable {
    public let chapters: [CascadeChapterTitleItem]
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
    /// 检测输入框末尾是否正在键入 "/xxx" 斜杠命令；若命中则返回要筛选的关键字（不含 "/"），否则返回 nil。
    /// 支持在空框开头输入（"/" -> ""，"/pl" -> "pl"），也支持在已有文字、空格、换行或标点后输入（"思路。/" -> ""，"hello /pl" -> "pl"）。
    /// 为避免误触 URL（https://...）、文件路径（/Users/foo）或英文斜杠词组（Go/Rust），"/" 前紧邻 ASCII 字母数字或路径符号时不触发。
    public static func query(of input: String) -> String? {
        guard let slashIdx = trailingSlashIndex(in: input) else { return nil }
        let afterIdx = input.index(after: slashIdx)
        return String(input[afterIdx...])
    }

    /// 选中命令后，仅剥离输入框末尾触发菜单的 "/xxx" 片段（及紧邻其前的水平空格），完整保留此前已输入的正文。
    public static func stripTrailingQuery(from input: String) -> String {
        guard let slashIdx = trailingSlashIndex(in: input) else { return input }
        var prefix = Substring(input[..<slashIdx])
        while let last = prefix.last, last == " " || last == "\t" {
            prefix = prefix.dropLast()
        }
        return String(prefix)
    }

    private static func trailingSlashIndex(in input: String) -> String.Index? {
        guard let slashIdx = input.lastIndex(of: "/") else { return nil }
        let afterIdx = input.index(after: slashIdx)
        let rest = input[afterIdx...]
        if rest.contains(where: { $0.isWhitespace }) { return nil }
        if slashIdx > input.startIndex {
            let prevChar = input[input.index(before: slashIdx)]
            if isDisallowedBeforeSlash(prevChar) { return nil }
        }
        return slashIdx
    }

    private static func isDisallowedBeforeSlash(_ ch: Character) -> Bool {
        if ch.isASCII && (ch.isLetter || ch.isNumber) { return true }
        switch ch {
        case "/", ":", ".", "~", "_", "-":
            return true
        default:
            return false
        }
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

/// 用户消息中引用的斜杠命令 / 技能（由网关从 trajectory.userInput.items 提取）。
public struct SlashCommandMention: Codable, Sendable, Hashable {
    public let name: String
    public let icon: String?
    public let kind: String

    public init(name: String, icon: String? = nil, kind: String = "skill") {
        self.name = name
        self.icon = icon
        self.kind = kind
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        name = try c.decode(String.self, forKey: .name)
        icon = try c.decodeIfPresent(String.self, forKey: .icon)
        kind = try c.decodeIfPresent(String.self, forKey: .kind) ?? "skill"
    }
    enum CodingKeys: String, CodingKey { case name, icon, kind }
}

/// 桌面端（Antigravity Desktop）斜杠命令与技能图标矢量目录（统一归一化到 0..960 坐标系，仅含 M/L/Q/Z 指令）。
public enum SlashCommandIconCatalog {
    public static let defaultSystemIcons: [String: String] = [
        "grill-me": "forum",
        "plan": "ballot",
        "goal": "timer",
        "compact": "collapse_all",
        "schedule": "schedule",
        "automation": "schedule",
        "browser": "language",
        "teamwork": "graph_2",
        "learn": "lightbulb_2",
        "boost": "bolt",
        "btw": "chat_add_on"
    ]

    public static let svgPaths: [String: String] = [
        "forum": "M852.31 826.15 L710.77 684.61 L320 684.61 Q290.81 684.61 270.02 663.82 Q249.23 643.03 249.23 613.84 L249.23 589.23 L684.62 589.23 Q714.45 589.23 735.69 567.99 Q756.93 546.75 756.93 516.92 L756.93 240 L781.54 240 Q810.73 240 831.52 260.79 Q852.31 281.58 852.31 310.77 L852.31 826.15 Z M167.69 505.85 L224.31 449.23 L604.62 449.23 Q610.01 449.23 613.47 445.77 Q616.93 442.31 616.93 436.92 L616.93 172.31 Q616.93 166.92 613.47 163.46 Q610.01 160 604.62 160 L180 160 Q174.61 160 171.15 163.46 Q167.69 166.92 167.69 172.31 L167.69 505.85 Z M107.69 650.77 L107.69 172.31 Q107.69 142.48 128.93 121.24 Q150.17 100 180 100 L604.62 100 Q634.45 100 655.69 121.24 Q676.93 142.48 676.93 172.31 L676.93 436.92 Q676.93 466.75 655.69 487.99 Q634.45 509.23 604.62 509.23 L249.23 509.23 L107.69 650.77 Z",
        "code_blocks": "M384 610.15 L426.15 567.38 L338.77 480 L425.77 393 L383.62 350.23 L253.85 480 L384 610.15 Z M576 610.15 L706.15 480 L576 349.85 L533.85 392.62 L621.23 480 L533.85 567.38 L576 610.15 Z M212.31 820 Q182 820 161 799 Q140 778 140 747.69 L140 212.31 Q140 182 161 161 Q182 140 212.31 140 L747.69 140 Q778 140 799 161 Q820 182 820 212.31 L820 747.69 Q820 778 799 799 Q778 820 747.69 820 L212.31 820 Z M212.31 760 L747.69 760 Q752.31 760 756.15 756.15 Q759.99 752.3 760 747.69 L760 212.31 Q760 207.69 756.15 203.85 Q752.3 200.01 747.69 200 L212.31 200 Q207.69 200 203.85 203.85 Q200.01 207.7 200 212.31 L200 747.69 Q200 752.31 203.85 756.15 Q207.7 759.99 212.31 760 Z",
        "ballot": "M480 400 L672.31 400 L672.31 340 L480 340 L480 400 Z M480 620 L672.31 620 L672.31 560 L480 560 L480 620 Z M405.73 415.73 Q424.61 396.85 424.61 370 Q424.61 343.15 405.73 324.27 Q386.85 305.39 360 305.39 Q333.15 305.39 314.27 324.27 Q295.39 343.15 295.39 370 Q295.39 396.85 314.27 415.73 Q333.15 434.61 360 434.61 Q386.85 434.61 405.73 415.73 Z M405.73 635.73 Q424.61 616.85 424.61 590 Q424.61 563.15 405.73 544.27 Q386.85 525.39 360 525.39 Q333.15 525.39 314.27 544.27 Q295.39 563.15 295.39 590 Q295.39 616.85 314.27 635.73 Q333.15 654.61 360 654.61 Q386.85 654.61 405.73 635.73 Z M212.31 820 Q182 820 161 799 Q140 778 140 747.69 L140 212.31 Q140 182 161 161 Q182 140 212.31 140 L747.69 140 Q778 140 799 161 Q820 182 820 212.31 L820 747.69 Q820 778 799 799 Q778 820 747.69 820 L212.31 820 Z M212.31 760 L747.69 760 Q752.31 760 756.15 756.15 Q759.99 752.3 760 747.69 L760 212.31 Q760 207.69 756.15 203.85 Q752.3 200.01 747.69 200 L212.31 200 Q207.69 200 203.85 203.85 Q200.01 207.7 200 212.31 L200 747.69 Q200 752.31 203.85 756.15 Q207.7 759.99 212.31 760 Z",
        "timer": "M367.69 110 L367.69 50 L592.31 50 L592.31 110 L367.69 110 Z M450 552.31 L510 552.31 L510 327.69 L450 327.69 L450 552.3 Z M348 833.23 Q286.15 806.46 239.85 760.15 Q193.55 713.84 166.77 652 Q139.99 590.16 140 520 Q140.01 449.84 166.77 388 Q193.53 326.16 239.85 279.85 Q286.17 233.54 348 206.77 Q409.83 180 480 180 Q540.08 180 595.73 200.38 Q651.38 220.76 699.08 258.76 L748.93 208.91 L791.08 251.06 L741.23 300.91 Q779.23 348.6 799.61 404.26 Q819.99 459.92 820 520 Q820 590.15 793.23 652 Q766.46 713.85 720.15 760.15 Q673.84 806.45 612 833.23 Q550.16 860.01 480 860 Q409.84 859.99 348 833.23 Z M678 718 Q760 636 760 520 Q760 404 678 322 Q596 240 480 240 Q364 240 282 322 Q200 404 200 520 Q200 636 282 718 Q364 800 480 800 Q596 800 678 718 Z",
        "collapse_all": "M296 866.15 L253.85 824 L480 597.85 L706.15 824 L664 866.15 L480 682.15 L296 866.15 Z M480 362.15 L253.85 136 L296 93.85 L480 277.85 L664 93.85 L706.15 136 L480 362.15 Z",
        "schedule": "M618.92 661.08 L661.07 618.93 L510 467.84 L510 280 L450 280 L450 492.15 L618.92 661.08 Z M480.07 860 Q401.23 860 331.87 830.08 Q262.51 800.16 211.18 748.87 Q159.85 697.58 129.93 628.24 Q100.01 558.9 100 480.07 Q99.99 401.24 129.92 331.87 Q159.85 262.5 211.13 211.19 Q262.41 159.88 331.76 129.94 Q401.11 100 479.93 100 Q558.75 100 628.13 129.92 Q697.51 159.84 748.81 211.13 Q800.11 262.42 830.06 331.76 Q860.01 401.1 860 479.93 Q859.99 558.76 830.08 628.13 Q800.17 697.5 748.87 748.82 Q697.57 800.14 628.24 830.07 Q558.91 860 480.07 860 Z M480 800 Q613 800 706.5 706.5 Q800 613 800 480 Q800 347 706.5 253.5 Q613 160 480 160 Q347 160 253.5 253.5 Q160 347 160 480 Q160 613 253.5 706.5 Q347 800 480 800 Z",
        "language": "M332.5 830.04 Q263.15 800.08 211.54 748.46 Q159.93 696.84 129.96 627.5 Q99.99 558.16 100 479.69 Q100.01 401.22 129.96 332.19 Q159.91 263.16 211.54 211.54 Q263.17 159.92 332.5 129.96 Q401.83 100 480.31 100 Q558.79 100 627.81 129.96 Q696.83 159.92 748.46 211.54 Q800.09 263.16 830.04 332.19 Q859.99 401.22 860 479.69 Q860.01 558.16 830.04 627.5 Q800.07 696.84 748.46 748.46 Q696.85 800.08 627.81 830.04 Q558.77 860 480.31 860 Q401.85 860 332.5 830.04 Z M480 799.15 Q510.62 758.53 531.54 717.23 Q552.46 675.93 565.62 626.92 L394.38 626.92 Q408.3 677.46 428.84 718.77 Q449.38 760.08 480 799.15 Z M402.54 788.15 Q379.54 755.15 361.23 713.11 Q342.92 671.07 332.77 626.92 L197.08 626.92 Q228.77 689.23 282.08 731.61 Q335.39 773.99 402.54 788.15 Z M557.46 788.15 Q624.61 774 677.92 731.62 Q731.23 689.24 762.92 626.93 L627.23 626.93 Q615.15 671.47 596.84 713.51 Q578.53 755.55 557.45 788.16 Z M171.92 566.92 L320.62 566.92 Q316.85 544.61 315.16 523.19 Q313.47 501.77 313.46 480 Q313.45 458.23 315.15 436.81 Q316.85 415.39 320.61 393.08 L171.92 393.08 Q166.15 413.46 163.07 435.46 Q159.99 457.46 160 480 Q160.01 502.54 163.08 524.54 Q166.15 546.54 171.93 566.92 Z M380.61 566.92 L579.39 566.92 Q583.16 544.61 584.85 523.57 Q586.54 502.53 586.54 480 Q586.54 457.47 584.85 436.42 Q583.16 415.37 579.39 393.07 L380.61 393.07 Q376.84 415.38 375.15 436.42 Q373.46 457.46 373.46 480 Q373.46 502.54 375.15 523.58 Q376.84 544.62 380.61 566.93 Z M639.38 566.92 L788.08 566.92 Q793.85 546.54 796.93 524.54 Q800.01 502.54 800 480 Q799.99 457.46 796.92 435.46 Q793.85 413.46 788.07 393.08 L639.38 393.08 Q643.15 415.39 644.84 436.81 Q646.53 458.23 646.54 480 Q646.55 501.77 644.85 523.19 Q643.15 544.61 639.39 566.92 Z M627.23 333.08 L762.92 333.08 Q730.85 270 678.5 228.38 Q626.15 186.76 557.46 171.46 Q580.46 206.38 598.38 247.85 Q616.3 289.32 627.23 333.08 Z M394.38 333.08 L565.62 333.08 Q551.7 282.93 530.58 240.66 Q509.46 198.39 480 160.85 Q450.54 198.39 429.42 240.66 Q408.3 282.93 394.38 333.08 Z M197.07 333.08 L332.77 333.08 Q343.69 289.31 361.62 247.85 Q379.55 206.39 402.54 171.46 Q333.46 186.77 281.31 228.58 Q229.16 270.39 197.08 333.08 Z",
        "graph_2": "M206.97 860 Q162.4 860 131.2 828.7 Q100 797.4 100 752.69 Q100 716.38 121.92 687.88 Q143.84 659.38 177.3 650.07 L177.3 557.68 Q177.3 513.07 208.84 481.53 Q240.38 449.99 285 450 L450 450 L450 309.92 Q416.54 300.61 394.62 272.11 Q372.7 243.61 372.7 207.3 Q372.7 162.59 403.9 131.29 Q435.1 99.99 479.66 100 Q524.22 100.01 555.77 131.3 Q587.32 162.59 587.31 207.31 Q587.31 243.62 565.39 272.12 Q543.47 300.62 510 309.92 L510 450 L675 450 Q719.61 450 751.15 481.54 Q782.69 513.08 782.69 557.69 L782.69 650.08 Q816.15 659.39 838.07 687.89 Q859.99 716.39 860 752.69 Q860 797.4 828.46 828.7 Q796.92 860 752.35 860 Q707.78 860 676.58 828.7 Q645.38 797.4 645.38 752.69 Q645.38 716.38 667.3 687.88 Q689.22 659.38 722.68 650.07 L722.68 557.68 Q722.68 537.41 708.97 523.7 Q695.26 509.99 675 510 L510 510 L510 650.08 Q543.46 659.39 565.38 687.89 Q587.3 716.39 587.3 752.7 Q587.3 797.41 555.76 828.71 Q524.22 860.01 479.66 860 Q435.1 859.99 403.89 828.7 Q372.68 797.41 372.69 752.69 Q372.69 716.38 394.61 687.88 Q416.53 659.38 450 650.08 L450 510 L285 510 Q264.73 510 251.02 523.71 Q237.31 537.42 237.31 557.69 L237.31 650.08 Q270.77 659.39 292.69 687.89 Q314.61 716.39 314.61 752.7 Q314.61 797.41 283.07 828.71 Q251.53 860.01 206.97 860 Z M207.12 800 Q227 800 240.81 786.4 Q254.62 772.8 254.62 752.71 Q254.62 732.62 240.9 719 Q227.18 705.38 206.92 705.38 Q187.23 705.38 173.61 719.19 Q159.99 733 160 752.69 Q160.01 772.38 173.62 786.19 Q187.23 800 207.12 800 Z M479.81 800 Q499.69 800 513.5 786.4 Q527.31 772.8 527.31 752.71 Q527.31 732.62 513.6 719 Q499.89 705.38 479.62 705.38 Q459.93 705.38 446.31 719.19 Q432.69 733 432.69 752.69 Q432.69 772.38 446.31 786.19 Q459.93 800 479.81 800 Z M752.5 800 Q772.38 800 786.19 786.4 Q800 772.8 800 752.71 Q800 732.62 786.29 719 Q772.58 705.38 752.31 705.38 Q732.62 705.38 719 719.19 Q705.38 733 705.38 752.69 Q705.38 772.38 719 786.19 Q732.62 800 752.5 800 Z M479.81 254.62 Q499.69 254.62 513.5 241.02 Q527.31 227.42 527.31 207.33 Q527.31 187.24 513.6 173.62 Q499.89 160 479.62 160 Q459.93 160 446.31 173.81 Q432.69 187.62 432.69 207.31 Q432.69 227 446.31 240.81 Q459.93 254.62 479.81 254.62 Z",
        "lightbulb_2": "M407.31 712.31 Q377.39 712.31 356.2 691.12 Q335.01 669.93 335 640 L335 576.15 Q280.69 539.84 250.35 482.69 Q220.01 425.54 220 360.38 Q220 251.84 295.73 175.92 Q371.46 100 480 100 Q588.54 100 664.27 175.73 Q740 251.46 740 360 Q740 425.54 709.65 482.38 Q679.3 539.22 625 576.15 L625 640 Q625 669.92 603.81 691.11 Q582.62 712.3 552.7 712.3 L407.31 712.3 Z M407.31 652.31 L552.69 652.31 Q558.08 652.31 561.54 648.85 Q565 645.39 565 640 L565 544.54 L594 524 Q634.62 495.62 657.31 452.31 Q680 409 680 360 Q680 277 621.5 218.5 Q563 160 480 160 Q397 160 338.5 218.5 Q280 277 280 360 Q280 409 302.69 452.31 Q325.38 495.62 366 524 L395 544.54 L395 640 Q395 645.39 398.46 648.85 Q401.92 652.31 407.31 652.31 Z M403.46 860 Q390.69 860 382.08 851.38 Q373.47 842.76 373.46 830 L373.46 800 L586.54 800 L586.54 830 Q586.54 842.77 577.92 851.38 Q569.3 859.99 556.54 860 L403.46 860 Z",
        "bolt": "M422 728 L629 480 L469 480 L498 253 L313 520 L452 520 L422 728 Z M343.08 857.69 L383.08 580 L198.08 580 L527.69 104.23 L576.92 104.23 L537.31 420 L757.31 420 L392.31 857.69 L343.08 857.69 Z",
        "code": "M320 706.15 L93.85 480 L320 253.85 L362.77 296.62 L178.77 480.62 L362.15 664 L320 706.15 Z M640 706.15 L597.23 663.38 L781.23 479.38 L597.85 296 L640 253.85 L866.15 480 L640 706.15 Z",
        "chat_add_on": "M130 775.77 L130 202.31 Q130 172.48 151.24 151.24 Q172.48 130 202.31 130 L677.69 130 Q707.52 130 728.76 151.24 Q750 172.48 750 202.31 L750 401.85 Q742.5 400.62 735 400.31 Q727.5 400 720 400 Q712.5 400 705 400.31 Q697.5 400.62 690 401.85 L690 202.31 Q690 196.92 686.54 193.46 Q683.08 190 677.69 190 L202.31 190 Q196.92 190 193.46 193.46 Q190 196.92 190 202.31 L190 610 L481.85 610 Q480.62 617.5 480.31 625 Q480 632.5 480 640 Q480 647.5 480.31 655 Q480.62 662.5 481.85 670 L235.77 670 L130 775.77 Z M282.31 350 L597.69 350 L597.69 290 L282.31 290 L282.31 350 Z M282.31 510 L477.69 510 L477.69 450 L282.31 450 L282.31 510 Z M690 790 L690 670 L570 670 L570 610 L690 610 L690 490 L750 490 L750 610 L870 610 L870 670 L750 670 L750 790 L690 790 Z"
    ]

    public static func resolveIcon(name: String, icon: String?, kind: String?) -> String {
        if let trimmed = icon?.trimmingCharacters(in: .whitespacesAndNewlines),
           !trimmed.isEmpty,
           svgPaths[trimmed] != nil {
            return trimmed
        }
        let cleanName = name.trimmingCharacters(in: .whitespacesAndNewlines)
        if let sys = defaultSystemIcons[cleanName] {
            return sys
        }
        if kind == "skill" {
            return "code_blocks"
        }
        return "code"
    }

    private static let cgPathCache = NSCache<NSString, AnyObject>()
    private static let imageCache = NSCache<NSString, UIImage>()

    private final class CGPathBox: @unchecked Sendable {
        let path: CGPath
        init(path: CGPath) { self.path = path }
    }

    public static func cachedCGPath(for iconKey: String) -> CGPath {
        let key = svgPaths[iconKey] != nil ? iconKey : "code_blocks"
        if let cached = cgPathCache.object(forKey: key as NSString) as? CGPathBox {
            return cached.path
        }
        let d = svgPaths[key] ?? svgPaths["code_blocks"]!
        let path = parseSvgPath(d)
        cgPathCache.setObject(CGPathBox(path: path), forKey: key as NSString)
        return path
    }

    public static func cachedImage(iconKey: String, size: CGFloat = 16, color: UIColor = .white) -> UIImage {
        let resolvedKey = svgPaths[iconKey] != nil ? iconKey : "code_blocks"
        var r: CGFloat = 0, g: CGFloat = 0, b: CGFloat = 0, a: CGFloat = 0
        color.getRed(&r, green: &g, blue: &b, alpha: &a)
        let colorKey = "\(Int(r * 255))_\(Int(g * 255))_\(Int(b * 255))_\(Int(a * 255))"
        let cacheKey = "\(resolvedKey)_\(Int(size * 10))_\(colorKey)" as NSString
        if let hit = imageCache.object(forKey: cacheKey) {
            return hit
        }
        let cgPath = cachedCGPath(for: resolvedKey)
        let scale = size / 960.0
        var transform = CGAffineTransform(scaleX: scale, y: scale)
        let targetPath = cgPath.copy(using: &transform) ?? cgPath

        let renderer = UIGraphicsImageRenderer(size: CGSize(width: size, height: size))
        let img = renderer.image { ctx in
            let cgCtx = ctx.cgContext
            cgCtx.addPath(targetPath)
            cgCtx.setFillColor(color.cgColor)
            cgCtx.fillPath(using: .winding)
        }
        imageCache.setObject(img, forKey: cacheKey)
        return img
    }

    private static func parseSvgPath(_ d: String) -> CGPath {
        let path = CGMutablePath()
        let scanner = Scanner(string: d)
        var cmd: String = ""
        while !scanner.isAtEnd {
            if let c = scanner.scanCharacters(from: CharacterSet(charactersIn: "MLQZmlqz")) {
                cmd = c
                if cmd.uppercased() == "Z" {
                    path.closeSubpath()
                    continue
                }
            }
            if cmd.uppercased() == "M" {
                if let x = scanner.scanDouble(), let y = scanner.scanDouble() {
                    path.move(to: CGPoint(x: x, y: y))
                }
            } else if cmd.uppercased() == "L" {
                if let x = scanner.scanDouble(), let y = scanner.scanDouble() {
                    path.addLine(to: CGPoint(x: x, y: y))
                }
            } else if cmd.uppercased() == "Q" {
                if let x1 = scanner.scanDouble(), let y1 = scanner.scanDouble(),
                   let x = scanner.scanDouble(), let y = scanner.scanDouble() {
                    path.addQuadCurve(to: CGPoint(x: x, y: y), control: CGPoint(x: x1, y: y1))
                }
            }
        }
        return path
    }
}

public enum SlashTextSegment: Hashable, Sendable {
    case text(String)
    case slash(SlashCommandMention)
}

/// 将用户消息文本拆分为普通文本段与斜杠命令胶囊段（与桌面端 ContextScopeItemPill 对齐）。
public enum SlashCommandTextTokenizer {
    private static let unixRootDirs: Set<String> = [
        "tmp", "var", "usr", "etc", "opt", "bin", "sbin", "dev",
        "home", "root", "proc", "sys", "vol1", "mnt", "media",
        "srv", "run", "users", "library", "system", "applications",
        "volumes", "private", "cores"
    ]

    public static func tokenize(
        text: String,
        mentions: [SlashCommandMention] = [],
        availableCommands: [SlashCommandOption] = []
    ) -> [SlashTextSegment] {
        guard !text.isEmpty, text.contains("/") else {
            return text.isEmpty ? [] : [.text(text)]
        }

        var lookup: [String: SlashCommandMention] = [:]
        for (sysName, sysIcon) in SlashCommandIconCatalog.defaultSystemIcons {
            lookup[sysName] = SlashCommandMention(name: sysName, icon: sysIcon, kind: "system")
        }
        for cmd in availableCommands {
            let icon = SlashCommandIconCatalog.resolveIcon(name: cmd.name, icon: cmd.icon, kind: cmd.kind)
            lookup[cmd.name] = SlashCommandMention(name: cmd.name, icon: icon, kind: cmd.kind)
        }
        for m in mentions {
            let icon = SlashCommandIconCatalog.resolveIcon(name: m.name, icon: m.icon, kind: m.kind)
            lookup[m.name] = SlashCommandMention(name: m.name, icon: icon, kind: m.kind)
        }

        guard let regex = try? NSRegularExpression(pattern: #"(^|\s)\/([A-Za-z0-9_-]+)(?=$|\s)"#) else {
            return [.text(text)]
        }

        let ns = text as NSString
        let matches = regex.matches(in: text, range: NSRange(location: 0, length: ns.length))
        guard !matches.isEmpty else { return [.text(text)] }

        var segments: [SlashTextSegment] = []
        var lastLocation = 0

        for match in matches {
            let wsRange = match.range(at: 1)
            let nameRange = match.range(at: 2)
            let cmdName = ns.substring(with: nameRange)
            let slashLocation = wsRange.location + wsRange.length

            let resolved: SlashCommandMention? = {
                if let known = lookup[cmdName] {
                    return known
                }
                // 离线缓存兜底：若位于消息最开头（index 0）且符合命令/技能命名（非根目录名），按技能图标兜底
                if slashLocation == 0,
                   !unixRootDirs.contains(cmdName.lowercased()),
                   cmdName.first?.isLetter == true {
                    return SlashCommandMention(name: cmdName, icon: "code_blocks", kind: "skill")
                }
                return nil
            }()

            guard let mention = resolved else { continue }

            if slashLocation > lastLocation {
                let prefix = ns.substring(with: NSRange(location: lastLocation, length: slashLocation - lastLocation))
                if !prefix.isEmpty {
                    segments.append(.text(prefix))
                }
            }
            segments.append(.slash(mention))

            var tokenEnd = nameRange.location + nameRange.length
            // 若 /cmd 后紧跟水平空白 + 换行（如 "/grill-me \n/Users/..."），吞掉换行前的多余空格
            while tokenEnd < ns.length {
                let ch = ns.character(at: tokenEnd)
                if ch == 0x20 || ch == 0x09 {
                    var peek = tokenEnd + 1
                    while peek < ns.length && (ns.character(at: peek) == 0x20 || ns.character(at: peek) == 0x09) {
                        peek += 1
                    }
                    if peek < ns.length && (ns.character(at: peek) == 0x0A || ns.character(at: peek) == 0x0D) {
                        tokenEnd = peek
                    }
                }
                break
            }
            lastLocation = tokenEnd
        }

        if lastLocation == 0 {
            return [.text(text)]
        }
        if lastLocation < ns.length {
            let tail = ns.substring(from: lastLocation)
            if !tail.isEmpty {
                segments.append(.text(tail))
            }
        }
        return segments
    }
}

// MARK: - 全局后台任务中心 (ListBackgroundTasks)

public extension Notification.Name {
    static let openConversationFromTaskCenter = Notification.Name("openConversationFromTaskCenter")
    static let startNewSessionForWorkspace = Notification.Name("startNewSessionForWorkspace")
}

public struct GlobalBackgroundTaskItem: Codable, Sendable, Identifiable, Equatable {
    public var id: String { "\(conversationId):\(taskId):\(stepIndex)" }
    public let conversationId: String
    public let conversationTitle: String
    public let workspaceName: String?
    public let taskId: String
    public let stepIndex: Int
    public let toolName: String?
    public let toolSummary: String?
    public let commandLine: String
    public let startTime: String?
    public let logUri: String?
    public let isDaemon: Bool

    public init(
        conversationId: String,
        conversationTitle: String = "未命名会话",
        workspaceName: String? = nil,
        taskId: String,
        stepIndex: Int,
        toolName: String? = nil,
        toolSummary: String? = nil,
        commandLine: String = "",
        startTime: String? = nil,
        logUri: String? = nil,
        isDaemon: Bool = false
    ) {
        self.conversationId = conversationId
        self.conversationTitle = conversationTitle
        self.workspaceName = workspaceName
        self.taskId = taskId
        self.stepIndex = stepIndex
        self.toolName = toolName
        self.toolSummary = toolSummary
        self.commandLine = commandLine
        self.startTime = startTime
        self.logUri = logUri
        self.isDaemon = isDaemon
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        conversationId = try c.decodeIfPresent(String.self, forKey: .conversationId) ?? ""
        let rawTitle = try c.decodeIfPresent(String.self, forKey: .conversationTitle)?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        conversationTitle = rawTitle.isEmpty ? "未命名会话" : rawTitle
        let rawWs = try c.decodeIfPresent(String.self, forKey: .workspaceName)?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        workspaceName = rawWs.isEmpty ? nil : rawWs
        taskId = try c.decodeIfPresent(String.self, forKey: .taskId) ?? ""
        stepIndex = try c.decodeIfPresent(Int.self, forKey: .stepIndex) ?? 0
        toolName = try c.decodeIfPresent(String.self, forKey: .toolName)
        toolSummary = try c.decodeIfPresent(String.self, forKey: .toolSummary)
        commandLine = try c.decodeIfPresent(String.self, forKey: .commandLine) ?? ""
        startTime = try c.decodeIfPresent(String.self, forKey: .startTime)
        logUri = try c.decodeIfPresent(String.self, forKey: .logUri)
        isDaemon = try c.decodeIfPresent(Bool.self, forKey: .isDaemon) ?? false
    }

    enum CodingKeys: String, CodingKey {
        case conversationId, conversationTitle, workspaceName, taskId, stepIndex
        case toolName, toolSummary, commandLine, startTime, logUri, isDaemon
    }

    public var displayTitle: String {
        if let s = toolSummary?.trimmingCharacters(in: .whitespacesAndNewlines), !s.isEmpty {
            return s
        }
        if toolName == "schedule" {
            return "定时任务"
        }
        return isDaemon ? "常驻守护进程" : "后台终端命令"
    }

    public var elapsedText: String? {
        guard let raw = startTime, !raw.isEmpty else { return nil }
        let fmtFrac = ISO8601DateFormatter()
        fmtFrac.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        let fmtStd = ISO8601DateFormatter()
        fmtStd.formatOptions = [.withInternetDateTime]
        guard let date = fmtFrac.date(from: raw) ?? fmtStd.date(from: raw) else { return nil }
        let sec = max(0, Int(Date().timeIntervalSince(date)))
        if sec < 60 { return "\(sec)s" }
        let min = sec / 60
        if min < 60 { return "\(min)m \(sec % 60)s" }
        let hr = min / 60
        return "\(hr)h \(min % 60)m"
    }
}

struct GlobalBackgroundTasksResponse: Decodable {
    let tasks: [GlobalBackgroundTaskItem]

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        tasks = try c.decodeIfPresent([GlobalBackgroundTaskItem].self, forKey: .tasks) ?? []
    }
    enum CodingKeys: String, CodingKey { case tasks }
}

struct BackgroundTaskLogRequest: Encodable {
    let conversationId: String
    let taskId: String
    let logUri: String?
}

public struct BackgroundTaskLogResponse: Decodable, Sendable {
    public let taskId: String
    public let logUri: String
    public let content: String
    public let truncated: Bool
    public let sizeBytes: Int64

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        taskId = try c.decodeIfPresent(String.self, forKey: .taskId) ?? ""
        logUri = try c.decodeIfPresent(String.self, forKey: .logUri) ?? ""
        content = try c.decodeIfPresent(String.self, forKey: .content) ?? ""
        truncated = try c.decodeIfPresent(Bool.self, forKey: .truncated) ?? false
        sizeBytes = try c.decodeIfPresent(Int64.self, forKey: .sizeBytes) ?? 0
    }
    enum CodingKeys: String, CodingKey { case taskId, logUri, content, truncated, sizeBytes }
}

// MARK: - Workflow 模板 (跨端同步)

public struct WorkflowTemplateRecord: Codable, Sendable, Identifiable, Equatable {
    public var id: String
    public var title: String
    public var subtitle: String
    public var icon: String
    public var color: String
    public var templatePrompt: String

    public init(
        id: String = UUID().uuidString,
        title: String,
        subtitle: String = "",
        icon: String = "workflow",
        color: String = "indigo",
        templatePrompt: String
    ) {
        self.id = id
        self.title = title
        self.subtitle = subtitle
        self.icon = icon
        self.color = color
        self.templatePrompt = templatePrompt
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = try c.decodeIfPresent(String.self, forKey: .id) ?? UUID().uuidString
        title = try c.decodeIfPresent(String.self, forKey: .title) ?? ""
        subtitle = try c.decodeIfPresent(String.self, forKey: .subtitle) ?? ""
        icon = try c.decodeIfPresent(String.self, forKey: .icon) ?? "workflow"
        color = try c.decodeIfPresent(String.self, forKey: .color) ?? "indigo"
        templatePrompt = try c.decodeIfPresent(String.self, forKey: .templatePrompt) ?? ""
    }

    enum CodingKeys: String, CodingKey {
        case id, title, subtitle, icon, color, templatePrompt
    }
}

public struct WorkflowTemplatesResponse: Codable, Sendable {
    public let templates: [WorkflowTemplateRecord]
    public let updatedAt: String?

    public init(templates: [WorkflowTemplateRecord], updatedAt: String? = nil) {
        self.templates = templates
        self.updatedAt = updatedAt
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        templates = try c.decodeIfPresent([WorkflowTemplateRecord].self, forKey: .templates) ?? []
        updatedAt = try c.decodeIfPresent(String.self, forKey: .updatedAt)
    }

    enum CodingKeys: String, CodingKey { case templates, updatedAt }
}

struct WorkflowTemplatesUpdateRequest: Encodable {
    let templates: [WorkflowTemplateRecord]
}

public enum WorkflowTemplateDefaults {
    public static let all: [WorkflowTemplateRecord] = [
        WorkflowTemplateRecord(
            id: "code_review",
            title: "并发代码审查",
            subtitle: "并行启动安全、性能、类型规范多角度 Sub-Agents 进行深度审计",
            icon: "security",
            color: "indigo",
            templatePrompt: "/workflow 请使用 run_workflow 动态编排工作流，以 parallel 并发对 [模块/文件路径] 进行多维度审计（安全/性能/类型规范），并在最终阶段输出结构化修复建议。"
        ),
        WorkflowTemplateRecord(
            id: "test_and_fix",
            title: "并发单测与故障修复",
            subtitle: "并行运行各模块单元测试，定位失败用例并尝试由子代理生成补丁",
            icon: "bug",
            color: "green",
            templatePrompt: "/workflow 请使用 run_workflow 编排自动化单测，以 parallel 同时运行 [测试目标或路径] 的单元测试，聚合失败信息并生成修复方案。"
        ),
        WorkflowTemplateRecord(
            id: "research",
            title: "多维度综合调研",
            subtitle: "分派多个子代理从技术实现、竞品方案、生产实践分头研究并归纳",
            icon: "search",
            color: "blue",
            templatePrompt: "/workflow 请使用 run_workflow 编排调研工作流，分派子代理分别调查 [调研主题] 的核心技术方案、竞品对比与最佳实践，并汇总为结构化报告。"
        ),
        WorkflowTemplateRecord(
            id: "custom",
            title: "自定义动态工作流",
            subtitle: "自由描述任务，让 Agent 依据依赖自主规划 parallel / pipeline 阶段",
            icon: "workflow",
            color: "purple",
            templatePrompt: "/workflow 请使用 run_workflow 编写 Python 动态工作流脚本，分阶段执行以下任务：[描述具体任务]，要求使用 parallel 并发加速并给出 finish 契约结果。"
        )
    ]
}

