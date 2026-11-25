import SwiftUI
import PDFKit
import UIKit

// MARK: - Nord Theme Color Tokens (Antigravity 2.33.0 Official Palette)

public enum NordTheme {
    public static let background = Color(red: 46.0 / 255.0, green: 52.0 / 255.0, blue: 64.0 / 255.0)         // #2E3440
    public static let surface = Color(red: 59.0 / 255.0, green: 66.0 / 255.0, blue: 82.0 / 255.0)            // #3B4252
    public static let surfaceVariant = Color(red: 67.0 / 255.0, green: 76.0 / 255.0, blue: 94.0 / 255.0)     // #434C5E
    public static let border = Color(red: 76.0 / 255.0, green: 86.0 / 255.0, blue: 106.0 / 255.0)            // #4C566A
    public static let foreground = Color(red: 216.0 / 255.0, green: 222.0 / 255.0, blue: 233.0 / 255.0)      // #D8DEE9
    public static let accent = Color(red: 136.0 / 255.0, green: 192.0 / 255.0, blue: 208.0 / 255.0)          // #88C0D0
    public static let accentForeground = Color(red: 46.0 / 255.0, green: 52.0 / 255.0, blue: 64.0 / 255.0)  // #2E3440
}

// MARK: - GitHub PR Helpers & Live Status Pills

public struct ParsedGitHubPRRef: Hashable, Identifiable, Sendable {
    public var id: String { url.absoluteString }
    public let url: URL
    public let owner: String
    public let repo: String
    public let number: Int
}

public enum GitHubPRLinkExtractor {
    private static let prURLRegex = try? NSRegularExpression(
        pattern: #"https?://[A-Za-z0-9.-]+/([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+)/pull/(\d+)(?:[^\s)\]`"'>]*)?"#,
        options: []
    )

    public static func extract(from text: String) -> [ParsedGitHubPRRef] {
        guard let regex = prURLRegex, text.contains("/pull/") else { return [] }
        let ns = text as NSString
        let matches = regex.matches(in: text, range: NSRange(location: 0, length: ns.length))
        var seen = Set<String>()
        var out: [ParsedGitHubPRRef] = []
        for m in matches {
            guard m.numberOfRanges >= 4 else { continue }
            let rawURL = ns.substring(with: m.range)
                .trimmingCharacters(in: CharacterSet(charactersIn: ".,;:!?)`\"'<>"))
            guard let u = URL(string: rawURL) else { continue }
            let owner = ns.substring(with: m.range(at: 1))
            let repo = ns.substring(with: m.range(at: 2))
            let num = Int(ns.substring(with: m.range(at: 3))) ?? 0
            guard num > 0, !owner.isEmpty, !repo.isEmpty else { continue }
            let canonical = "\(owner)/\(repo)#\(num)".lowercased()
            if seen.insert(canonical).inserted {
                out.append(ParsedGitHubPRRef(url: u, owner: owner, repo: repo, number: num))
            }
        }
        return out
    }

    public static func parse(url: URL) -> ParsedGitHubPRRef? {
        extract(from: url.absoluteString).first
    }
}

@MainActor
public final class GitHubPRStatusCache {
    public static let shared = GitHubPRStatusCache()
    private var entries: [String: (item: GitPullRequestItem, at: Date)] = [:]
    private let ttl: TimeInterval = 30

    public func cached(for urlString: String) -> GitPullRequestItem? {
        guard let hit = entries[urlString.lowercased()], Date().timeIntervalSince(hit.at) < ttl else {
            return nil
        }
        return hit.item
    }

    public func resolve(urls: [String], baseURL: URL) async -> [String: GitPullRequestItem] {
        var result: [String: GitPullRequestItem] = [:]
        var missing: [String] = []
        for raw in urls {
            let key = raw.lowercased()
            if let hit = cached(for: key) {
                result[raw] = hit
            } else {
                missing.append(raw)
            }
        }
        if !missing.isEmpty {
            if let fetched = try? await APIClient.shared.fetchGitHubPullRequests(urls: missing, baseURL: baseURL) {
                let now = Date()
                for item in fetched {
                    entries[item.url.lowercased()] = (item, now)
                    for raw in missing {
                        if raw.caseInsensitiveCompare(item.url) == .orderedSame ||
                            raw.lowercased().contains("/pull/\(item.number)") {
                            entries[raw.lowercased()] = (item, now)
                            result[raw] = item
                        }
                    }
                }
            }
        }
        return result
    }
}

public struct GitHubPRPillBadge: View {
    public let item: GitPullRequestItem
    public var compact: Bool = true

    public init(item: GitPullRequestItem, compact: Bool = true) {
        self.item = item
        self.compact = compact
    }

    private var stateStyle: (label: String, icon: String, color: Color) {
        switch item.state.uppercased() {
        case "MERGED":
            return ("MERGED", "arrow.triangle.merge", Color(red: 0.54, green: 0.34, blue: 0.90))
        case "DRAFT":
            return ("DRAFT", "pencil.circle.fill", Color.gray)
        case "CLOSED":
            return ("CLOSED", "xmark.circle.fill", Color.red)
        default:
            return ("OPEN", "arrow.triangle.pull", Color.green)
        }
    }

    public var body: some View {
        let style = stateStyle
        HStack(spacing: 4) {
            Image(systemName: style.icon)
                .font(.system(size: compact ? 9.5 : 11, weight: .bold))
            Text("#\(item.number)")
                .font(.system(size: compact ? 10.5 : 11.5, weight: .bold, design: .monospaced))
            if !compact || !item.title.isEmpty {
                Text("· \(style.label)")
                    .font(.system(size: compact ? 9.5 : 10.5, weight: .semibold))
            }
        }
        .foregroundColor(style.color)
        .padding(.horizontal, compact ? 6 : 8)
        .padding(.vertical, compact ? 2 : 4)
        .background(style.color.opacity(0.14))
        .overlay(
            Capsule().stroke(style.color.opacity(0.35), lineWidth: 0.7)
        )
        .clipShape(Capsule())
    }
}

public struct InlineGitHubPRPillsSection: View {
    @Environment(\.openURL) private var openURL
    public let content: String

    @State private var resolved: [String: GitPullRequestItem] = [:]

    private var refs: [ParsedGitHubPRRef] {
        Array(GitHubPRLinkExtractor.extract(from: content).prefix(4))
    }

    public init(content: String) {
        self.content = content
    }

    public var body: some View {
        if !refs.isEmpty {
            ScrollView(.horizontal, showsIndicators: false) {
                HStack(spacing: 6) {
                    ForEach(refs) { ref in
                        let item = resolved[ref.url.absoluteString] ?? GitPullRequestItem(
                            host: ref.url.host,
                            owner: ref.owner,
                            repo: ref.repo,
                            number: ref.number,
                            title: "\(ref.owner)/\(ref.repo) #\(ref.number)",
                            url: ref.url.absoluteString,
                            state: "OPEN",
                            authorLogin: nil,
                            headRefName: nil,
                            baseRefName: nil,
                            body: nil,
                            error: nil
                        )
                        Button {
                            UIImpactFeedbackGenerator(style: .light).impactOccurred()
                            openURL(ref.url)
                        } label: {
                            HStack(spacing: 5) {
                                GitHubPRPillBadge(item: item, compact: true)
                                if let fetched = resolved[ref.url.absoluteString], !fetched.title.isEmpty {
                                    Text(fetched.title)
                                        .font(.system(size: 11.5, weight: .medium))
                                        .foregroundColor(.primary)
                                        .lineLimit(1)
                                        .frame(maxWidth: 180, alignment: .leading)
                                } else {
                                    Text("\(ref.owner)/\(ref.repo)")
                                        .font(.system(size: 11.5))
                                        .foregroundColor(.secondary)
                                        .lineLimit(1)
                                }
                            }
                            .padding(.horizontal, 8)
                            .padding(.vertical, 5)
                            .background(Color(uiColor: .tertiarySystemBackground))
                            .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
                            .overlay(
                                RoundedRectangle(cornerRadius: 10, style: .continuous)
                                    .stroke(Color.secondary.opacity(0.22), lineWidth: 0.7)
                            )
                        }
                        .buttonStyle(.plain)
                    }
                }
            }
            .task(id: refs.map(\.id).joined(separator: "|")) {
                guard let baseURL = AppSettings.shared.gatewayURL else { return }
                let map = await GitHubPRStatusCache.shared.resolve(urls: refs.map(\.url.absoluteString), baseURL: baseURL)
                if !map.isEmpty {
                    resolved = map
                }
            }
        }
    }
}

public struct GitHubPRDetailSheet: View {
    @Environment(\.dismiss) private var dismiss
    public let url: URL
    public let onOpenInBrowser: (URL) -> Void

    @State private var item: GitPullRequestItem? = nil
    @State private var isLoading: Bool = true

    public init(url: URL, onOpenInBrowser: @escaping (URL) -> Void) {
        self.url = url
        self.onOpenInBrowser = onOpenInBrowser
    }

    public var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 14) {
                    if let ref = GitHubPRLinkExtractor.parse(url: url) {
                        HStack(spacing: 8) {
                            if let item {
                                GitHubPRPillBadge(item: item, compact: false)
                            }
                            Text("\(ref.owner)/\(ref.repo) #\(ref.number)")
                                .font(.system(size: 13, weight: .semibold, design: .monospaced))
                                .foregroundColor(.secondary)
                            Spacer()
                            if isLoading {
                                ProgressView().controlSize(.small)
                            }
                        }
                    }

                    if let item {
                        Text(item.title.isEmpty ? url.absoluteString : item.title)
                            .font(.system(size: 17, weight: .bold))
                            .foregroundColor(.primary)

                        if let head = item.headRefName, !head.isEmpty {
                            HStack(spacing: 6) {
                                Text(head)
                                    .font(.system(size: 12, design: .monospaced))
                                    .padding(.horizontal, 7)
                                    .padding(.vertical, 3)
                                    .background(Color.secondary.opacity(0.14))
                                    .clipShape(RoundedRectangle(cornerRadius: 6, style: .continuous))
                                Image(systemName: "arrow.right")
                                    .font(.system(size: 11, weight: .semibold))
                                    .foregroundColor(.secondary)
                                Text(item.baseRefName ?? "main")
                                    .font(.system(size: 12, design: .monospaced))
                                    .padding(.horizontal, 7)
                                    .padding(.vertical, 3)
                                    .background(Color.secondary.opacity(0.14))
                                    .clipShape(RoundedRectangle(cornerRadius: 6, style: .continuous))
                            }
                        }

                        if let author = item.authorLogin, !author.isEmpty {
                            Label("@\(author)", systemImage: "person.circle")
                                .font(.system(size: 13))
                                .foregroundColor(.secondary)
                        }

                        if let body = item.body, !body.isEmpty {
                            Divider()
                            Text(body)
                                .font(.system(size: 13.5))
                                .foregroundColor(.primary)
                                .textSelection(.enabled)
                        }
                    } else if !isLoading {
                        Text(url.absoluteString)
                            .font(.system(size: 13, design: .monospaced))
                            .foregroundColor(.secondary)
                    }

                    Button {
                        let target = URL(string: item?.url ?? "") ?? url
                        dismiss()
                        onOpenInBrowser(target)
                    } label: {
                        Label("在浏览器中打开 Pull Request", systemImage: "safari")
                            .font(.system(size: 14.5, weight: .semibold))
                            .frame(maxWidth: .infinity)
                            .padding(.vertical, 10)
                    }
                    .buttonStyle(.borderedProminent)
                    .tint(.indigo)
                }
                .padding(16)
            }
            .navigationTitle("GitHub Pull Request")
            .navigationBarTitleDisplayMode(.inline)
            .task {
                isLoading = true
                defer { isLoading = false }
                guard let baseURL = AppSettings.shared.gatewayURL else { return }
                let map = await GitHubPRStatusCache.shared.resolve(urls: [url.absoluteString], baseURL: baseURL)
                item = map[url.absoluteString] ?? map.values.first
            }
        }
        .presentationDetents([.medium, .large])
        .presentationDragIndicator(.visible)
    }
}

// MARK: - Workspace Trust Banner

public struct WorkspaceTrustBanner: View {
    public let status: ProjectTrustStatusResponse
    public let isUpdating: Bool
    public let onTrust: () -> Void

    public var body: some View {
        HStack(spacing: 10) {
            Image(systemName: "exclamationmark.shield.fill")
                .font(.system(size: 15, weight: .semibold))
                .foregroundColor(.orange)

            VStack(alignment: .leading, spacing: 2) {
                Text("当前工作区尚未受信任")
                    .font(.system(size: 12.5, weight: .semibold))
                    .foregroundColor(.primary)
                Text("部分文件访问或工具执行会受限，可一键授予信任。")
                    .font(.system(size: 11))
                    .foregroundColor(.secondary)
                    .lineLimit(2)
            }

            Spacer(minLength: 6)

            Button(action: onTrust) {
                if isUpdating {
                    ProgressView().controlSize(.small)
                } else {
                    Text("信任工作区")
                        .font(.system(size: 12, weight: .semibold))
                        .lineLimit(1)
                        .fixedSize(horizontal: true, vertical: false)
                }
            }
            .buttonStyle(.borderedProminent)
            .tint(.orange)
            .controlSize(.small)
            .layoutPriority(1)
            .disabled(isUpdating)
        }
        .padding(.horizontal, 12)
        .padding(.vertical, 9)
        .background(Color.orange.opacity(0.11))
        .overlay(
            RoundedRectangle(cornerRadius: 12, style: .continuous)
                .stroke(Color.orange.opacity(0.32), lineWidth: 0.8)
        )
        .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
        .padding(.horizontal, 12)
        .padding(.top, 6)
    }
}

// MARK: - /btw Side Question Branch UI Components

enum CompactMarkdownPreviewRenderer {
    private static let hrRegex = try! NSRegularExpression(pattern: #"^([-*_]\s*){3,}$"#)
    private static let tableSepRegex = try! NSRegularExpression(pattern: #"^\|?[\s:]*-{3,}[\s:]*(?:\|[\s:]*-{3,}[\s:]*)+\|?$"#)
    private static let quoteRegex = try! NSRegularExpression(pattern: #"^(?:>\s*)+"#)
    private static let headingRegex = try! NSRegularExpression(pattern: #"^#{1,6}\s+"#)
    private static let bulletRegex = try! NSRegularExpression(pattern: #"^[-*+]\s+"#)
    private static let imageRegex = try! NSRegularExpression(pattern: #"!\[([^\]]*)\]\([^)]*\)"#)
    private static let htmlBrRegex = try! NSRegularExpression(pattern: #"(?i)<br\s*/?>"#)
    private static let inlineFenceRegex = try! NSRegularExpression(pattern: #"(?:^|\s)(?:```|~~~)[A-Za-z0-9_-]*(?=\s|$)"#)
    private static let brokenImgRegex = try! NSRegularExpression(pattern: #"!\[[^\]]*(?:\]\([^)]*)?$"#)
    private static let brokenLinkRegex = try! NSRegularExpression(pattern: #"\[([^\[\]]+)\]\([^\)]*$"#)
    private static let whitespaceRegex = try! NSRegularExpression(pattern: #"\s+"#)

    static func normalize(_ raw: String) -> String {
        let trimmed = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return "" }

        let unified = trimmed
            .replacingOccurrences(of: "\r\n", with: "\n")
            .replacingOccurrences(of: "\r", with: "\n")
        var cleanedLines: [String] = []
        for line in unified.split(separator: "\n", omittingEmptySubsequences: true) {
            var t = String(line).trimmingCharacters(in: .whitespacesAndNewlines)
            if t.isEmpty || t.hasPrefix("```") || t.hasPrefix("~~~") { continue }
            if matches(hrRegex, t) || matches(tableSepRegex, t) { continue }
            t = replace(quoteRegex, in: t, with: "").trimmingCharacters(in: .whitespaces)
            t = replace(headingRegex, in: t, with: "")
            t = replace(bulletRegex, in: t, with: "• ")
            if t.hasPrefix("|") && t.hasSuffix("|") && t.filter({ $0 == "|" }).count >= 2 {
                let inner = t.trimmingCharacters(in: CharacterSet(charactersIn: "|"))
                t = inner.split(separator: "|")
                    .map { $0.trimmingCharacters(in: .whitespaces) }
                    .filter { !$0.isEmpty }
                    .joined(separator: " · ")
            }
            t = replace(imageRegex, in: t, with: "$1")
            t = replace(htmlBrRegex, in: t, with: " ")
            t = replace(inlineFenceRegex, in: t, with: " ")
            t = t.trimmingCharacters(in: .whitespacesAndNewlines)
            if !t.isEmpty {
                cleanedLines.append(t)
            }
        }
        guard !cleanedLines.isEmpty else { return "" }
        let flat = replace(whitespaceRegex, in: cleanedLines.joined(separator: " "), with: " ")
            .trimmingCharacters(in: .whitespacesAndNewlines)
        return repairTruncatedInlineMarkdown(flat)
    }

    static func attributed(_ raw: String, fontSize: CGFloat) -> AttributedString {
        let normalized = normalize(raw)
        guard !normalized.isEmpty else { return AttributedString() }
        let processed = MathSymbolProcessor.process(normalized)
        var attr = MarkdownContentView.renderInlineMarkdown(processed, size: fontSize)
        // Strip interactive link attributes so tapping anywhere on the compact card opens the branch.
        let linkRanges = attr.runs.compactMap { $0.link != nil ? $0.range : nil }
        for range in linkRanges {
            attr[range].link = nil
        }
        return attr
    }

    private static func repairTruncatedInlineMarkdown(_ text: String) -> String {
        var suffix = ""
        var body = text
        if body.hasSuffix("…") {
            suffix = "…"
            body.removeLast()
        } else if body.hasSuffix("...") {
            suffix = "..."
            body.removeLast(3)
        }

        body = replace(brokenImgRegex, in: body, with: "")
        body = replace(brokenLinkRegex, in: body, with: "$1")

        // Balance an unclosed trailing inline code backtick.
        var chars = Array(body)
        var inCode = false
        var lastBacktick = -1
        var i = 0
        while i < chars.count {
            if chars[i] == "\\", i + 1 < chars.count {
                i += 2
                continue
            }
            if chars[i] == "`" {
                inCode.toggle()
                if inCode { lastBacktick = i }
            }
            i += 1
        }
        if inCode && lastBacktick >= 0 {
            let after = String(chars[(lastBacktick + 1)...]).trimmingCharacters(in: .whitespaces)
            if after.isEmpty {
                body = String(chars[..<lastBacktick]).trimmingCharacters(in: .whitespaces)
            } else {
                body = body.trimmingCharacters(in: .whitespaces) + "`"
            }
        }

        // Balance unclosed ***, **, ~~, * outside code spans.
        chars = Array(body)
        var stack: [(delim: String, pos: Int)] = []
        inCode = false
        i = 0
        while i < chars.count {
            if chars[i] == "\\", i + 1 < chars.count {
                i += 2
                continue
            }
            if chars[i] == "`" {
                inCode.toggle()
                i += 1
                continue
            }
            if inCode {
                i += 1
                continue
            }
            if i + 2 < chars.count && chars[i] == "*" && chars[i + 1] == "*" && chars[i + 2] == "*" {
                if stack.last?.delim == "***" {
                    stack.removeLast()
                } else {
                    stack.append(("***", i))
                }
                i += 3
                continue
            }
            if i + 1 < chars.count && chars[i] == "*" && chars[i + 1] == "*" {
                if stack.last?.delim == "**" {
                    stack.removeLast()
                } else if stack.last?.delim == "***" {
                    stack[stack.count - 1].delim = "*"
                } else {
                    stack.append(("**", i))
                }
                i += 2
                continue
            }
            if i + 1 < chars.count && chars[i] == "~" && chars[i + 1] == "~" {
                if stack.last?.delim == "~~" {
                    stack.removeLast()
                } else {
                    stack.append(("~~", i))
                }
                i += 2
                continue
            }
            if chars[i] == "*" {
                let prevSpace = (i == 0 || chars[i - 1].isWhitespace)
                let nextSpace = (i + 1 >= chars.count || chars[i + 1].isWhitespace)
                if !(prevSpace && nextSpace) {
                    if stack.last?.delim == "*" {
                        stack.removeLast()
                    } else if stack.last?.delim == "***" {
                        stack[stack.count - 1].delim = "**"
                    } else {
                        stack.append(("*", i))
                    }
                }
            }
            i += 1
        }

        while let top = stack.popLast() {
            let curChars = Array(body)
            let afterIdx = top.pos + top.delim.count
            if afterIdx <= curChars.count && String(curChars[afterIdx...]).trimmingCharacters(in: .whitespaces).isEmpty {
                body = String(curChars[..<top.pos]).trimmingCharacters(in: .whitespaces)
            } else {
                body = body.trimmingCharacters(in: .whitespaces) + top.delim
            }
        }

        body = body.trimmingCharacters(in: .whitespacesAndNewlines)
        return body.isEmpty ? "" : body + suffix
    }

    private static func matches(_ regex: NSRegularExpression, _ s: String) -> Bool {
        let range = NSRange(s.startIndex..<s.endIndex, in: s)
        return regex.firstMatch(in: s, range: range) != nil
    }

    private static func replace(_ regex: NSRegularExpression, in s: String, with template: String) -> String {
        let range = NSRange(s.startIndex..<s.endIndex, in: s)
        return regex.stringByReplacingMatches(in: s, range: range, withTemplate: template)
    }
}

/// 挂在父会话源消息卡片正下方的紧凑旁路分支子卡片（点击直接进入完整分支会话）。
public struct SideQuestionInlineCardsSection: View {
    public let items: [SideQuestionCardItem]
    public let onOpen: (String) -> Void

    public init(items: [SideQuestionCardItem], onOpen: @escaping (String) -> Void) {
        self.items = items
        self.onOpen = onOpen
    }

    public var body: some View {
        if !items.isEmpty {
            VStack(alignment: .leading, spacing: 6) {
                ForEach(items) { item in
                    Button {
                        UIImpactFeedbackGenerator(style: .light).impactOccurred()
                        onOpen(item.cascadeId)
                    } label: {
                        HStack(alignment: .top, spacing: 8) {
                            Rectangle()
                                .fill(Color.indigo.opacity(0.45))
                                .frame(width: 2.5)
                                .clipShape(Capsule())

                            VStack(alignment: .leading, spacing: 3) {
                                HStack(spacing: 5) {
                                    Image(systemName: "arrow.triangle.branch")
                                        .font(.system(size: 10.5, weight: .semibold))
                                        .foregroundColor(.indigo)
                                    Text("旁路快问")
                                        .font(.system(size: 10.5, weight: .semibold))
                                        .lineLimit(1)
                                        .fixedSize(horizontal: true, vertical: false)
                                        .foregroundColor(.indigo)
                                    Text("·")
                                        .font(.system(size: 10.5))
                                        .foregroundColor(.secondary)
                                    Text(item.displayTitle)
                                        .font(.system(size: 12.5, weight: .semibold))
                                        .foregroundColor(.primary)
                                        .lineLimit(1)
                                    Spacer(minLength: 4)
                                    if item.isRunning {
                                        ProgressView()
                                            .controlSize(.mini)
                                            .tint(.indigo)
                                    } else if item.isError {
                                        Circle()
                                            .fill(Color.red)
                                            .frame(width: 6, height: 6)
                                    } else {
                                        Circle()
                                            .fill(Color.indigo.opacity(0.65))
                                            .frame(width: 6, height: 6)
                                    }
                                    Image(systemName: "chevron.right")
                                        .font(.system(size: 9.5, weight: .semibold))
                                        .foregroundColor(.secondary.opacity(0.75))
                                }

                                if let preview = item.answerPreview?.trimmingCharacters(in: .whitespacesAndNewlines),
                                   !preview.isEmpty {
                                    Text(CompactMarkdownPreviewRenderer.attributed(preview, fontSize: 12))
                                        .font(.system(size: 12))
                                        .foregroundColor(.secondary)
                                        .lineLimit(2)
                                        .multilineTextAlignment(.leading)
                                } else if item.isRunning {
                                    Text("正在独立生成回复...")
                                        .font(.system(size: 11.5))
                                        .foregroundColor(.secondary)
                                }
                            }
                            .frame(maxWidth: .infinity, alignment: .leading)
                        }
                        .padding(.horizontal, 10)
                        .padding(.vertical, 8)
                        .background(Color.indigo.opacity(0.06))
                        .overlay(
                            RoundedRectangle(cornerRadius: 12, style: .continuous)
                                .stroke(Color.indigo.opacity(0.20), lineWidth: 0.8)
                        )
                        .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
                    }
                    .buttonStyle(.plain)
                }
            }
            .padding(.leading, 22)
            .padding(.trailing, 16)
            .padding(.top, 2)
            .padding(.bottom, 4)
        }
    }
}

/// 旁路分支会话顶部的来源与共享前文折叠横幅。
public struct BranchOriginBannerView: View {
    public let origin: SideQuestionOrigin
    public let hiddenCount: Int
    public let isExpanded: Bool
    public let onToggleExpand: () -> Void
    public let onReturnToParent: () -> Void

    private var originLabel: String {
        let title = origin.parentTitle?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        if !title.isEmpty, let turn = origin.forkTurnNumber, turn > 0 {
            return String(localized: "承接自「\(title)」第 \(turn) 轮")
        }
        if !title.isEmpty {
            return String(localized: "承接自「\(title)」")
        }
        if let turn = origin.forkTurnNumber, turn > 0 {
            return String(localized: "承接自主会话第 \(turn) 轮")
        }
        return String(localized: "承接自主会话上下文")
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack(spacing: 7) {
                Image(systemName: "arrow.triangle.branch")
                    .font(.system(size: 12, weight: .semibold))
                    .foregroundColor(.indigo)
                Text(originLabel)
                    .font(.system(size: 12.5, weight: .semibold))
                    .foregroundColor(.primary)
                    .lineLimit(1)
                Spacer(minLength: 4)
            }

            HStack(spacing: 8) {
                Text(isExpanded ? String(localized: "已展开共享前文") : String(localized: "已共享前文上下文（默认折叠）"))
                    .font(.system(size: 11.5))
                    .foregroundColor(.secondary)
                    .lineLimit(1)
                    .minimumScaleFactor(0.82)

                Spacer(minLength: 4)

                Button {
                    UIImpactFeedbackGenerator(style: .light).impactOccurred()
                    onToggleExpand()
                } label: {
                    HStack(spacing: 3) {
                        Image(systemName: isExpanded ? "chevron.up" : "chevron.down")
                            .font(.system(size: 9.5, weight: .bold))
                        Text(isExpanded ? String(localized: "收起前文") : String(localized: "展开前文"))
                            .font(.system(size: 11.5, weight: .semibold))
                            .lineLimit(1)
                    }
                    .fixedSize(horizontal: true, vertical: false)
                    .foregroundColor(.indigo)
                    .padding(.horizontal, 9)
                    .padding(.vertical, 4)
                    .background(Color.indigo.opacity(0.11))
                    .clipShape(Capsule())
                }
                .buttonStyle(.plain)
                .layoutPriority(1)

                if !origin.parentCascadeId.isEmpty {
                    Button {
                        UIImpactFeedbackGenerator(style: .light).impactOccurred()
                        onReturnToParent()
                    } label: {
                        HStack(spacing: 3) {
                            Image(systemName: "arrow.uturn.backward")
                                .font(.system(size: 9.5, weight: .bold))
                            Text("回主会话")
                                .font(.system(size: 11.5, weight: .semibold))
                                .lineLimit(1)
                        }
                        .fixedSize(horizontal: true, vertical: false)
                        .foregroundColor(.primary)
                        .padding(.horizontal, 9)
                        .padding(.vertical, 4)
                        .background(Color(uiColor: .tertiarySystemFill))
                        .clipShape(Capsule())
                    }
                    .buttonStyle(.plain)
                    .layoutPriority(1)
                }
            }
        }
        .padding(.horizontal, 12)
        .padding(.vertical, 10)
        .background(Color.indigo.opacity(0.06))
        .overlay(
            RoundedRectangle(cornerRadius: 12, style: .continuous)
                .stroke(Color.indigo.opacity(0.22), lineWidth: 0.8)
        )
        .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
        .padding(.horizontal, 16)
        .padding(.bottom, 4)
    }
}

/// 展开共享前文时，区分共享历史与旁路分支消息的分隔线。
public struct BranchForkDividerView: View {
    public let onCollapse: () -> Void

    public var body: some View {
        HStack(spacing: 8) {
            Rectangle()
                .fill(Color.indigo.opacity(0.22))
                .frame(height: 1)

            Button(action: onCollapse) {
                HStack(spacing: 4) {
                    Image(systemName: "arrow.triangle.branch")
                        .font(.system(size: 10, weight: .semibold))
                    Text("以上为共享前文 · 以下为旁路分支")
                        .font(.system(size: 11, weight: .semibold))
                        .lineLimit(1)
                        .minimumScaleFactor(0.78)
                }
                .fixedSize(horizontal: true, vertical: false)
                .foregroundColor(.indigo)
                .padding(.horizontal, 10)
                .padding(.vertical, 4)
                .background(Color.indigo.opacity(0.10))
                .clipShape(Capsule())
            }
            .buttonStyle(.plain)
            .layoutPriority(1)

            Rectangle()
                .fill(Color.indigo.opacity(0.22))
                .frame(height: 1)
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 6)
    }
}

// MARK: - /btw Quick-Ask Composer & Branch Switcher Sheet

public struct BtwSideQuestionSheet: View {
    @Environment(\.dismiss) private var dismiss
    public let parentCascadeId: String
    public let parentTitle: String
    public let anchorStepIndex: Int?
    public let anchorPreview: String?
    public let existingBranches: [SideQuestionCardItem]
    public let onOpenBranch: (String) -> Void
    public let onStartBranch: (String, Int?) async -> Bool

    @State private var questionText: String = ""
    @State private var useAnchorStep: Bool = true
    @State private var isStarting: Bool = false
    @State private var loadedBranches: [SideQuestionCardItem] = []
    @FocusState private var isQuestionFocused: Bool

    public init(
        parentCascadeId: String,
        parentTitle: String,
        anchorStepIndex: Int? = nil,
        anchorPreview: String? = nil,
        existingBranches: [SideQuestionCardItem] = [],
        onOpenBranch: @escaping (String) -> Void,
        onStartBranch: @escaping (String, Int?) async -> Bool
    ) {
        self.parentCascadeId = parentCascadeId
        self.parentTitle = parentTitle
        self.anchorStepIndex = anchorStepIndex
        self.anchorPreview = anchorPreview
        self.existingBranches = existingBranches
        self.onOpenBranch = onOpenBranch
        self.onStartBranch = onStartBranch
        self._useAnchorStep = State(initialValue: anchorStepIndex != nil)
    }

    private var effectiveBranches: [SideQuestionCardItem] {
        loadedBranches.isEmpty ? existingBranches : loadedBranches
    }

    public var body: some View {
        NavigationStack {
            VStack(spacing: 0) {
                ScrollView {
                    VStack(alignment: .leading, spacing: 14) {
                        if let stepIdx = anchorStepIndex, useAnchorStep {
                            VStack(alignment: .leading, spacing: 6) {
                                HStack(spacing: 6) {
                                    Label("承接至所选消息的上下文", systemImage: "arrow.triangle.branch")
                                        .font(.system(size: 12, weight: .semibold))
                                        .foregroundColor(.indigo)
                                    Spacer()
                                    Button("改用最新上下文") {
                                        withAnimation(.easeInOut(duration: 0.15)) {
                                            useAnchorStep = false
                                        }
                                    }
                                    .font(.system(size: 11.5, weight: .medium))
                                    .foregroundColor(.secondary)
                                }
                                if let preview = anchorPreview?.trimmingCharacters(in: .whitespacesAndNewlines), !preview.isEmpty {
                                    Text(CompactMarkdownPreviewRenderer.attributed(preview, fontSize: 12))
                                        .font(.system(size: 12))
                                        .foregroundColor(.secondary)
                                        .lineLimit(2)
                                        .frame(maxWidth: .infinity, alignment: .leading)
                                } else {
                                    Text("将从 Step #\(stepIdx) 处分叉，不携带后续消息。")
                                        .font(.system(size: 12))
                                        .foregroundColor(.secondary)
                                }
                            }
                            .padding(11)
                            .background(Color.indigo.opacity(0.07))
                            .overlay(
                                RoundedRectangle(cornerRadius: 12, style: .continuous)
                                    .stroke(Color.indigo.opacity(0.22), lineWidth: 0.8)
                            )
                            .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
                        } else {
                            HStack(spacing: 8) {
                                Image(systemName: "arrow.triangle.branch")
                                    .font(.system(size: 13, weight: .semibold))
                                    .foregroundColor(.indigo)
                                Text("基于「\(parentTitle)」当前上下文创建独立旁路分支，不打断主对话流，支持多轮追问。")
                                    .font(.system(size: 12.5))
                                    .foregroundColor(.secondary)
                            }
                            .padding(11)
                            .frame(maxWidth: .infinity, alignment: .leading)
                            .background(Color(uiColor: .secondarySystemBackground))
                            .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
                        }

                        if !effectiveBranches.isEmpty {
                            VStack(alignment: .leading, spacing: 8) {
                                Text("已有旁路分支 (\(effectiveBranches.count))")
                                    .font(.system(size: 12, weight: .semibold))
                                    .foregroundColor(.secondary)

                                ForEach(effectiveBranches) { branch in
                                    Button {
                                        dismiss()
                                        onOpenBranch(branch.cascadeId)
                                    } label: {
                                        HStack(spacing: 8) {
                                            Image(systemName: "arrow.triangle.branch")
                                                .font(.system(size: 12, weight: .semibold))
                                                .foregroundColor(.indigo)
                                            VStack(alignment: .leading, spacing: 2) {
                                                Text(branch.displayTitle)
                                                    .font(.system(size: 13.5, weight: .semibold))
                                                    .foregroundColor(.primary)
                                                    .lineLimit(1)
                                                if let ans = branch.answerPreview?.trimmingCharacters(in: .whitespacesAndNewlines), !ans.isEmpty {
                                                    Text(CompactMarkdownPreviewRenderer.attributed(ans, fontSize: 11.5))
                                                        .font(.system(size: 11.5))
                                                        .foregroundColor(.secondary)
                                                        .lineLimit(1)
                                                }
                                            }
                                            Spacer()
                                            if branch.isRunning {
                                                ProgressView().controlSize(.mini).tint(.indigo)
                                            }
                                            Image(systemName: "chevron.right")
                                                .font(.system(size: 11, weight: .semibold))
                                                .foregroundColor(.secondary)
                                        }
                                        .padding(.horizontal, 12)
                                        .padding(.vertical, 10)
                                        .background(Color(uiColor: .secondarySystemBackground))
                                        .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
                                    }
                                    .buttonStyle(.plain)
                                }
                            }
                        }
                    }
                    .padding(16)
                }

                Divider()
                HStack(spacing: 8) {
                    TextField("输入旁路问题（/btw）...", text: $questionText, axis: .vertical)
                        .lineLimit(1...4)
                        .font(.system(size: 14.5))
                        .focused($isQuestionFocused)
                        .padding(.horizontal, 12)
                        .padding(.vertical, 9)
                        .background(Color(uiColor: .secondarySystemBackground))
                        .clipShape(RoundedRectangle(cornerRadius: 14, style: .continuous))

                    Button {
                        Task { await submitQuestion() }
                    } label: {
                        if isStarting {
                            ProgressView().controlSize(.small)
                                .frame(width: 36, height: 36)
                        } else {
                            Image(systemName: "paperplane.fill")
                                .font(.system(size: 15, weight: .semibold))
                                .foregroundColor(.white)
                                .frame(width: 36, height: 36)
                                .background(questionText.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty ? Color.secondary.opacity(0.35) : Color.indigo)
                                .clipShape(Circle())
                        }
                    }
                    .buttonStyle(.plain)
                    .disabled(isStarting || questionText.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
                }
                .padding(.horizontal, 14)
                .padding(.vertical, 10)
                .background(Color(uiColor: .systemBackground))
            }
            .navigationTitle("/btw 旁路快问")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Button("关闭") { dismiss() }
                        .font(.system(size: 14.5))
                }
            }
            .onAppear {
                DispatchQueue.main.asyncAfter(deadline: .now() + 0.18) {
                    isQuestionFocused = true
                }
            }
            .task(id: parentCascadeId) {
                guard let baseURL = AppSettings.shared.gatewayURL, !parentCascadeId.isEmpty else { return }
                if let list = try? await APIClient.shared.listBtwSideQuestions(cascadeId: parentCascadeId, baseURL: baseURL) {
                    loadedBranches = list.map { entry in
                        SideQuestionCardItem(
                            cascadeId: entry.cascadeId,
                            parentCascadeId: entry.parentCascadeId ?? parentCascadeId,
                            title: entry.title,
                            question: entry.question,
                            answerPreview: entry.answerPreview,
                            status: entry.status,
                            forkStepIndex: entry.forkStepIndex,
                            anchorMessageId: nil,
                            turnCount: entry.turnCount,
                            createdAt: entry.createdAt
                        )
                    }
                }
            }
        }
        .presentationDetents([.medium, .large])
        .presentationDragIndicator(.visible)
    }

    private func submitQuestion() async {
        let q = questionText.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !q.isEmpty, !isStarting else { return }
        isStarting = true
        let ok = await onStartBranch(q, useAnchorStep ? anchorStepIndex : nil)
        isStarting = false
        if ok {
            dismiss()
        }
    }
}

// MARK: - Fast Dense PDF Viewer (PDFKit + Find-in-Page + Deferred Text Selection)

public struct PdfDocumentPreviewView: View {
    public let url: URL
    @State private var findTrigger: Int = 0
    @State private var enableTextSelection: Bool = false

    public init(url: URL) {
        self.url = url
    }

    public var body: some View {
        VStack(spacing: 0) {
            HStack(spacing: 10) {
                Toggle(isOn: $enableTextSelection) {
                    Label("文本选择", systemImage: "character.cursor.ibeam")
                        .font(.system(size: 12.5, weight: .medium))
                }
                .toggleStyle(.button)
                .controlSize(.small)
                .tint(.indigo)

                Spacer()

                Button {
                    findTrigger &+= 1
                } label: {
                    Label("页内查找", systemImage: "magnifyingglass")
                        .font(.system(size: 12.5, weight: .medium))
                }
                .buttonStyle(.bordered)
                .controlSize(.small)
            }
            .padding(.horizontal, 12)
            .padding(.vertical, 6)
            .background(Color(uiColor: .secondarySystemBackground))

            Divider()

            PDFKitRepresentable(
                url: url,
                enableTextSelection: enableTextSelection,
                findTrigger: findTrigger
            )
        }
    }
}

private struct PDFKitRepresentable: UIViewRepresentable {
    let url: URL
    let enableTextSelection: Bool
    let findTrigger: Int

    final class Coordinator {
        var lastFindTrigger: Int = 0
    }

    func makeCoordinator() -> Coordinator { Coordinator() }

    func makeUIView(context: Context) -> PDFView {
        let view = PDFView()
        view.autoScales = true
        view.displayMode = .singlePageContinuous
        view.displayDirection = .vertical
        view.displaysPageBreaks = true
        view.backgroundColor = .systemBackground
        view.isFindInteractionEnabled = true
        view.isUserInteractionEnabled = true
        view.document = PDFDocument(url: url)
        return view
    }

    func updateUIView(_ pdfView: PDFView, context: Context) {
        if pdfView.document?.documentURL != url {
            pdfView.document = PDFDocument(url: url)
        }
        // Defer expensive text-layer selection hit-testing on dense multi-page PDFs unless toggled on.
        if let docView = pdfView.documentView {
            docView.isUserInteractionEnabled = enableTextSelection || findTrigger != context.coordinator.lastFindTrigger
        }
        if findTrigger != context.coordinator.lastFindTrigger {
            context.coordinator.lastFindTrigger = findTrigger
            DispatchQueue.main.async {
                pdfView.becomeFirstResponder()
                pdfView.findInteraction.presentFindNavigator(showingReplace: false)
            }
        }
    }
}
