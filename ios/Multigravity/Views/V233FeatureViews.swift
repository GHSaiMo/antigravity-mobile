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
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Button("关闭") { dismiss() }
                }
            }
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
                }
            }
            .buttonStyle(.borderedProminent)
            .tint(.orange)
            .controlSize(.small)
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

// MARK: - /btw Side Question Sheet

public struct BtwSideQuestionSheet: View {
    @Environment(\.dismiss) private var dismiss
    public let parentCascadeId: String
    public let activeModel: String
    public let initialQuote: String?
    public let onInsertIntoMain: (String) -> Void
    public let onOpenStandalone: ((String) -> Void)?

    @State private var questionText: String = ""
    @State private var quoteText: String = ""
    @State private var entries: [BtwSideQuestionEntry] = []
    @State private var activeSideId: String? = nil
    @State private var activeStatus: BtwStatusResponse? = nil
    @State private var isStarting: Bool = false
    @State private var errorMsg: String? = nil
    @State private var showThinking: Bool = false

    public init(
        parentCascadeId: String,
        activeModel: String,
        initialQuote: String? = nil,
        onInsertIntoMain: @escaping (String) -> Void,
        onOpenStandalone: ((String) -> Void)? = nil
    ) {
        self.parentCascadeId = parentCascadeId
        self.activeModel = activeModel
        self.initialQuote = initialQuote
        self.onInsertIntoMain = onInsertIntoMain
        self.onOpenStandalone = onOpenStandalone
    }

    public var body: some View {
        NavigationStack {
            VStack(spacing: 0) {
                ScrollView {
                    VStack(alignment: .leading, spacing: 14) {
                        if !quoteText.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                            VStack(alignment: .leading, spacing: 6) {
                                HStack {
                                    Label("引用片段", systemImage: "text.quote")
                                        .font(.system(size: 11.5, weight: .semibold))
                                        .foregroundColor(.indigo)
                                    Spacer()
                                    Button("清除") { quoteText = "" }
                                        .font(.system(size: 11.5))
                                        .foregroundColor(.secondary)
                                }
                                Text(quoteText)
                                    .font(.system(size: 12.5))
                                    .foregroundColor(.secondary)
                                    .lineLimit(5)
                                    .padding(10)
                                    .frame(maxWidth: .infinity, alignment: .leading)
                                    .background(Color(uiColor: .secondarySystemBackground))
                                    .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
                            }
                        }

                        if !entries.isEmpty {
                            ScrollView(.horizontal, showsIndicators: false) {
                                HStack(spacing: 8) {
                                    ForEach(entries) { entry in
                                        Button {
                                            activeSideId = entry.cascadeId
                                        } label: {
                                            HStack(spacing: 5) {
                                                Circle()
                                                    .fill(entry.status == "CASCADE_RUN_STATUS_RUNNING" ? Color.green : Color.indigo.opacity(0.6))
                                                    .frame(width: 6, height: 6)
                                                Text(entry.title ?? entry.question ?? "Side question")
                                                    .font(.system(size: 12, weight: .medium))
                                                    .lineLimit(1)
                                                    .frame(maxWidth: 160, alignment: .leading)
                                            }
                                            .padding(.horizontal, 10)
                                            .padding(.vertical, 6)
                                            .background(activeSideId == entry.cascadeId ? Color.indigo.opacity(0.18) : Color(uiColor: .secondarySystemBackground))
                                            .clipShape(Capsule())
                                        }
                                        .buttonStyle(.plain)
                                    }
                                }
                            }
                        }

                        if let st = activeStatus {
                            VStack(alignment: .leading, spacing: 10) {
                                if let q = st.question, !q.isEmpty {
                                    Text(q)
                                        .font(.system(size: 14, weight: .semibold))
                                        .padding(10)
                                        .frame(maxWidth: .infinity, alignment: .leading)
                                        .background(Color.indigo.opacity(0.10))
                                        .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
                                }

                                if let think = st.thinking, !think.isEmpty {
                                    DisclosureGroup("深度思考", isExpanded: $showThinking) {
                                        Text(think)
                                            .font(.system(size: 12))
                                            .foregroundColor(.secondary)
                                            .padding(.top, 4)
                                    }
                                    .font(.system(size: 12.5, weight: .medium))
                                    .padding(10)
                                    .background(Color(uiColor: .secondarySystemBackground))
                                    .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
                                }

                                if let err = st.errorMessage, st.hasError == true, !err.isEmpty {
                                    Text(err)
                                        .font(.system(size: 13))
                                        .foregroundColor(.red)
                                        .padding(10)
                                        .background(Color.red.opacity(0.10))
                                        .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
                                }

                                if let ans = st.answer, !ans.isEmpty {
                                    MarkdownContentView(content: ans)
                                        .padding(12)
                                        .frame(maxWidth: .infinity, alignment: .leading)
                                        .background(Color(uiColor: .secondarySystemBackground))
                                        .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))

                                    HStack(spacing: 10) {
                                        Button {
                                            let text = (st.insertText?.isEmpty == false ? st.insertText! : ans)
                                            dismiss()
                                            onInsertIntoMain(text)
                                        } label: {
                                            Label("插入主对话", systemImage: "arrow.turn.down.left")
                                                .font(.system(size: 13.5, weight: .semibold))
                                                .frame(maxWidth: .infinity)
                                        }
                                        .buttonStyle(.borderedProminent)
                                        .tint(.indigo)

                                        if let onOpenStandalone {
                                            Button {
                                                dismiss()
                                                onOpenStandalone(st.cascadeId)
                                            } label: {
                                                Image(systemName: "arrow.up.right.square")
                                            }
                                            .buttonStyle(.bordered)
                                            .accessibilityLabel("在独立会话打开")
                                        }
                                    }
                                } else if st.isRunning == true {
                                    HStack(spacing: 8) {
                                        ProgressView().controlSize(.small)
                                        Text("旁路快问正在生成回复...")
                                            .font(.system(size: 13))
                                            .foregroundColor(.secondary)
                                    }
                                    .padding(.vertical, 8)
                                }
                            }
                        } else if !isStarting && entries.isEmpty {
                            VStack(alignment: .leading, spacing: 6) {
                                Label("旁路快问（不打断主对话流）", systemImage: "bubble.left.and.exclamationmark.bubble.right")
                                    .font(.system(size: 13.5, weight: .semibold))
                                    .foregroundColor(.primary)
                                Text("随时就当前上下文发起独立快问，回答完成后可一键将结论引用插入主对话输入框。")
                                    .font(.system(size: 12.5))
                                    .foregroundColor(.secondary)
                            }
                            .padding(12)
                            .frame(maxWidth: .infinity, alignment: .leading)
                            .background(Color(uiColor: .secondarySystemBackground))
                            .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
                        }

                        if let err = errorMsg {
                            Text(err)
                                .font(.system(size: 12.5))
                                .foregroundColor(.red)
                        }
                    }
                    .padding(16)
                }

                Divider()
                HStack(spacing: 8) {
                    TextField("输入旁路快问（/btw）...", text: $questionText, axis: .vertical)
                        .lineLimit(1...4)
                        .font(.system(size: 14.5))
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
                                .background(Color.indigo)
                                .clipShape(Circle())
                        }
                    }
                    .buttonStyle(.plain)
                    .disabled(isStarting || (questionText.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty && quoteText.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty))
                }
                .padding(.horizontal, 14)
                .padding(.vertical, 10)
                .background(Color(uiColor: .systemBackground))
            }
            .navigationTitle("/btw 旁路快问")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    Button("关闭") { dismiss() }
                }
            }
            .onAppear {
                if let q = initialQuote, !q.isEmpty {
                    quoteText = q
                }
            }
            .task(id: parentCascadeId) {
                guard let baseURL = AppSettings.shared.gatewayURL, !parentCascadeId.isEmpty else { return }
                if let list = try? await APIClient.shared.listBtwSideQuestions(cascadeId: parentCascadeId, baseURL: baseURL) {
                    entries = list
                    if activeSideId == nil {
                        activeSideId = list.last?.cascadeId
                    }
                }
            }
            .task(id: activeSideId) {
                guard let sid = activeSideId, let baseURL = AppSettings.shared.gatewayURL else { return }
                while !Task.isCancelled {
                    if let st = try? await APIClient.shared.fetchBtwStatus(cascadeId: sid, baseURL: baseURL) {
                        activeStatus = st
                        if st.isRunning != true {
                            break
                        }
                    }
                    try? await Task.sleep(nanoseconds: 850_000_000)
                }
            }
        }
        .presentationDetents([.medium, .large])
        .presentationDragIndicator(.visible)
    }

    private func submitQuestion() async {
        guard let baseURL = AppSettings.shared.gatewayURL else { return }
        let q = questionText.trimmingCharacters(in: .whitespacesAndNewlines)
        let qt = quoteText.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !q.isEmpty || !qt.isEmpty else { return }

        isStarting = true
        errorMsg = nil
        defer { isStarting = false }

        do {
            let res = try await APIClient.shared.startBtwSideQuestion(
                parentCascadeId: parentCascadeId,
                question: q,
                quote: qt.isEmpty ? nil : qt,
                model: activeModel,
                baseURL: baseURL
            )
            questionText = ""
            quoteText = ""
            let entry = BtwSideQuestionEntry(
                cascadeId: res.cascadeId,
                parentCascadeId: parentCascadeId,
                title: res.title,
                question: res.question,
                quote: res.quote,
                status: "CASCADE_RUN_STATUS_RUNNING",
                createdAt: res.createdAt
            )
            entries.append(entry)
            activeSideId = res.cascadeId
        } catch {
            errorMsg = error.localizedDescription
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
