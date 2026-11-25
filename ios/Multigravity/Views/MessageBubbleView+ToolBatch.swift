import SwiftUI
import Photos

// MARK: - Desktop-Parity Tool Summary & Formatting Helpers

enum DesktopToolLocale {
    case zh
    case ja
    case en

    static var current: DesktopToolLocale {
        let lang = (Bundle.main.preferredLocalizations.first ?? Locale.preferredLanguages.first ?? "en").lowercased()
        if lang.hasPrefix("zh") { return .zh }
        if lang.hasPrefix("ja") { return .ja }
        return .en
    }
}

enum DesktopToolSummaryFormatter {
    static func shortThinkingDuration(_ raw: String?) -> String? {
        guard let trimmed = raw?.trimmingCharacters(in: .whitespacesAndNewlines), !trimmed.isEmpty else {
            return nil
        }
        // Proto3 duration like "14.231s" or "75.4s"
        if trimmed.hasSuffix("s"), !trimmed.hasSuffix("ms"), !trimmed.contains("m") {
            let numStr = String(trimmed.dropLast()).trimmingCharacters(in: .whitespaces)
            if let secsDouble = Double(numStr) {
                let totalSecs = max(1, Int(secsDouble.rounded(.down)))
                if totalSecs >= 60 {
                    let m = totalSecs / 60
                    let s = totalSecs % 60
                    return s > 0 ? "\(m)m \(s)s" : "\(m)m"
                }
                return "\(totalSecs)s"
            }
        }
        return trimmed
    }

    static func thinkingTitle(duration: String?, isRunning: Bool) -> String {
        let locale = DesktopToolLocale.current
        if isRunning {
            switch locale {
            case .zh: return "思考中..."
            case .ja: return "思考中..."
            case .en: return "Thinking..."
            }
        }
        if let dur = shortThinkingDuration(duration) {
            switch locale {
            case .zh: return "已思考 \(dur)"
            case .ja: return "\(dur) 思考"
            case .en: return "Thought for \(dur)"
            }
        }
        switch locale {
        case .zh: return "思考过程"
        case .ja: return "思考プロセス"
        case .en: return "Thought Process"
        }
    }

    static func localizedPrefix(_ raw: String?) -> String {
        guard let p = raw?.trimmingCharacters(in: .whitespacesAndNewlines), !p.isEmpty else {
            return ""
        }
        let locale = DesktopToolLocale.current
        if locale == .en { return p }
        switch p.lowercased() {
        case "analyzed": return locale == .zh ? "已分析" : "分析済み"
        case "analyzing": return locale == .zh ? "正在分析" : "分析中"
        case "read": return locale == .zh ? "已读取" : "読み取り済み"
        case "reading": return locale == .zh ? "正在读取" : "読み取り中"
        case "edited": return locale == .zh ? "已编辑" : "編集済み"
        case "editing": return locale == .zh ? "正在编辑" : "編集中"
        case "created": return locale == .zh ? "已创建" : "作成済み"
        case "creating": return locale == .zh ? "正在创建" : "作成中"
        case "deleted": return locale == .zh ? "已删除" : "削除済み"
        case "deleting": return locale == .zh ? "正在删除" : "削除中"
        case "ran": return locale == .zh ? "已运行" : "実行済み"
        case "running": return locale == .zh ? "正在运行" : "実行中"
        case "run": return locale == .zh ? "运行" : "実行"
        case "searched": return locale == .zh ? "已搜索" : "検索済み"
        case "searching": return locale == .zh ? "正在搜索" : "検索中"
        case "listed": return locale == .zh ? "已浏览" : "一覧取得済み"
        case "listing": return locale == .zh ? "正在浏览" : "一覧取得中"
        case "rejected": return locale == .zh ? "已拒绝" : "拒否済み"
        case "canceled": return locale == .zh ? "已取消" : "キャンセル済み"
        case "interrupted": return locale == .zh ? "已中断" : "中断"
        case "errored": return locale == .zh ? "执行出错" : "エラー"
        case "canceled edit to": return locale == .zh ? "已取消编辑" : "編集キャンセル"
        case "canceled creation of": return locale == .zh ? "已取消创建" : "作成キャンセル"
        case "canceled deletion of": return locale == .zh ? "已取消删除" : "削除キャンセル"
        default: return p
        }
    }

    static func resultsBadge(_ count: Int) -> String {
        switch DesktopToolLocale.current {
        case .zh: return "\(count) 条结果"
        case .ja: return "\(count) 件"
        case .en: return count == 1 ? "1 result" : "\(count) results"
        }
    }

    static func hiddenLinesText(_ count: Int) -> String {
        switch DesktopToolLocale.current {
        case .zh: return "…（上方已省略 \(count) 行）"
        case .ja: return "…（上部の \(count) 行を省略）"
        case .en: return "… (\(count) \(count == 1 ? "line" : "lines") above hidden)"
        }
    }

    /// Mirrors Antigravity Desktop `KUc` + `D5b` completed tool-group header summary.
    static func completedSummary(
        count: Int,
        tools: [String],
        toolDetails: [ToolDetailItem]?,
        thinking: String?,
        thinkingDuration: String?,
        thinkingBlocks: [ThinkingBlockItem]?
    ) -> String {
        let locale = DesktopToolLocale.current
        let details = toolDetails ?? []

        // Thinking-only batch (no tool operations)
        let effectiveToolCount = max(details.count, max(count, tools.count))
        if effectiveToolCount == 0 {
            let bestDur = thinkingBlocks?.last(where: { !($0.duration ?? "").isEmpty })?.duration ?? thinkingDuration
            if !(thinking ?? "").trimmingCharacters(in: .whitespacesAndNewlines).isEmpty || !(thinkingBlocks?.isEmpty ?? true) {
                return thinkingTitle(duration: bestDur, isRunning: false)
            }
            switch locale {
            case .zh: return "已完成"
            case .ja: return "完了"
            case .en: return "Done"
            }
        }

        // Classify items matching desktop mY / D5b: explored (other/read/list/search), terminal, edit
        var exploredCount = 0
        var exploredFileKeys = Set<String>()
        var exploredFileCount = 0
        var exploredArtifactCount = 0

        var terminalCount = 0

        var editCount = 0
        var editFileKeys = Set<String>()
        var editFileCount = 0
        var editArtifactCount = 0

        if !details.isEmpty {
            for item in details {
                let kind = resolvedKind(for: item)
                if item.status.lowercased() == "error" { continue }
                switch kind {
                case "terminal":
                    terminalCount += 1
                case "edit":
                    editCount += 1
                    let key = (item.fileUri ?? item.fileName ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
                    if !key.isEmpty {
                        if editFileKeys.insert(key).inserted {
                            if item.isArtifact == true { editArtifactCount += 1 } else { editFileCount += 1 }
                        }
                    } else {
                        editFileCount += 1
                    }
                default:
                    exploredCount += 1
                    if kind == "read" || kind == "list" {
                        let key = (item.fileUri ?? item.fileName ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
                        if !key.isEmpty {
                            if exploredFileKeys.insert(key).inserted {
                                if item.isArtifact == true { exploredArtifactCount += 1 } else { exploredFileCount += 1 }
                            }
                        }
                    }
                }
            }
        } else {
            for rawName in tools {
                let lower = rawName.lowercased()
                if lower.contains("command") || lower == "run_command" || lower == "shell_command" {
                    terminalCount += 1
                } else if lower.contains("edit") || lower.contains("write") || lower.contains("replace") || lower.contains("create_file") || lower.contains("delete_file") {
                    editCount += 1
                    editFileCount += 1
                } else {
                    exploredCount += 1
                    if lower.contains("file") || lower.contains("read") || lower.contains("view") || lower.contains("dir") {
                        exploredFileCount += 1
                    }
                }
            }
        }

        func targetNoun(files: Int, artifacts: Int) -> String {
            switch locale {
            case .zh:
                if files > 0 { return "文件" }
                if artifacts > 0 { return "文档" }
                return ""
            case .ja:
                if files > 0 { return "ファイル" }
                if artifacts > 0 { return "ドキュメント" }
                return ""
            case .en:
                if files > 0 { return (files + artifacts) == 1 ? "file" : "files" }
                if artifacts > 0 { return artifacts == 1 ? "artifact" : "artifacts" }
                return ""
            }
        }

        var parts: [String] = []
        if exploredCount > 0 {
            let noun = targetNoun(files: exploredFileCount, artifacts: exploredArtifactCount)
            switch locale {
            case .zh:
                parts.append(noun.isEmpty ? "已探索" : "已浏览\(noun)")
            case .ja:
                parts.append(noun.isEmpty ? "調査済み" : "\(noun)を調査")
            case .en:
                parts.append(noun.isEmpty ? "Explored" : "Explored \(noun)")
            }
        }
        if terminalCount > 0 {
            switch locale {
            case .zh:
                parts.append(parts.isEmpty ? "已运行命令" : "运行命令")
            case .ja:
                parts.append("コマンドを実行")
            case .en:
                let verb = parts.isEmpty ? "Ran" : "ran"
                parts.append("\(verb) \(terminalCount == 1 ? "command" : "commands")")
            }
        }
        if editCount > 0 {
            let noun = targetNoun(files: editFileCount, artifacts: editArtifactCount)
            switch locale {
            case .zh:
                let verb = parts.isEmpty ? "已编辑" : "编辑"
                parts.append(noun.isEmpty ? "\(verb)文件" : "\(verb)\(noun)")
            case .ja:
                parts.append(noun.isEmpty ? "ファイルを編集" : "\(noun)を編集")
            case .en:
                let verb = parts.isEmpty ? "Edited" : "edited"
                parts.append(noun.isEmpty ? "\(verb) files" : "\(verb) \(noun)")
            }
        }

        if !parts.isEmpty {
            switch locale {
            case .zh: return parts.joined(separator: "、")
            case .ja: return parts.joined(separator: "・")
            case .en: return parts.joined(separator: ", ")
            }
        }

        switch locale {
        case .zh: return "已完成 \(effectiveToolCount) 项操作"
        case .ja: return "\(effectiveToolCount) 件の操作を完了"
        case .en: return effectiveToolCount == 1 ? "Completed 1 action" : "Completed \(effectiveToolCount) actions"
        }
    }

    static func resolvedKind(for item: ToolDetailItem) -> String {
        if let k = item.kind, !k.isEmpty { return k }
        let lower = item.name.lowercased()
        if lower == "run_command" || lower == "shell_command" || lower == "command" || lower == "command_status" {
            return "terminal"
        }
        if lower == "read_file" || lower == "view_file" || lower == "read_url_content" {
            return "read"
        }
        if lower == "edit_file" || lower == "write_file" || lower == "write_to_file" ||
            lower == "replace_file_content" || lower == "multi_replace_file_content" ||
            lower == "create_file" || lower == "delete_file" || lower == "code_action" {
            return "edit"
        }
        if lower == "list_dir" || lower == "list_directory" {
            return "list"
        }
        if lower.contains("search") || lower.contains("grep") || lower.contains("find") {
            return "search"
        }
        if lower.hasPrefix("mcp_") || lower == "call_mcp_tool" || lower == "mcp_tool" {
            return "mcp"
        }
        return "other"
    }

    /// Client-side safety fallback for legacy cached `ToolDetailItem` payloads where `summary`
    /// contained raw JSON like `{"EndLine":650,"Path":"/Users/.../StreamPayload.kt"}`.
    static func normalizedLegacyFields(for item: ToolDetailItem) -> (
        kind: String,
        prefix: String,
        title: String,
        fileUri: String?,
        fileName: String?,
        startLine: Int?,
        endLine: Int?,
        command: String?
    ) {
        let kind = resolvedKind(for: item)
        var prefix = item.prefix ?? ""
        var title = item.title ?? ""
        var fileUri = item.fileUri
        var fileName = item.fileName
        var startLine = item.startLine
        var endLine = item.endLine
        var command = item.command

        let rawSummary = item.summary.trimmingCharacters(in: .whitespacesAndNewlines)
        if rawSummary.hasPrefix("{"),
           let data = rawSummary.data(using: .utf8),
           let obj = try? JSONSerialization.jsonObject(with: data) as? [String: Any] {
            if fileName == nil, let p = (obj["Path"] ?? obj["AbsolutePath"] ?? obj["TargetFile"] ?? obj["path"] ?? obj["DirectoryPath"]) as? String, !p.isEmpty {
                let clean = p.hasPrefix("file://") ? p : "file://" + p
                fileUri = fileUri ?? clean
                fileName = URL(fileURLWithPath: p.replacingOccurrences(of: "file://", with: "")).lastPathComponent
            }
            if startLine == nil, let s = obj["StartLine"] as? Int, s > 0 { startLine = s }
            if endLine == nil, let e = obj["EndLine"] as? Int, e > 0 { endLine = e }
            if command == nil, let c = (obj["CommandLine"] ?? obj["commandLine"] ?? obj["command"]) as? String, !c.isEmpty {
                command = c
            }
            if title.isEmpty, let ts = (obj["toolSummary"] ?? obj["Query"] ?? obj["query"] ?? obj["Pattern"]) as? String, !ts.isEmpty {
                title = ts
            }
        } else if title.isEmpty && fileName == nil && command == nil && !rawSummary.isEmpty {
            if kind == "terminal" {
                command = rawSummary
            } else if !rawSummary.hasPrefix("{") {
                title = rawSummary
            }
        }

        if prefix.isEmpty {
            let running = item.status.lowercased() == "running"
            switch kind {
            case "read", "list": prefix = running ? "Analyzing" : "Analyzed"
            case "edit": prefix = running ? "Editing" : "Edited"
            case "terminal": prefix = running ? "Running" : "Ran"
            case "search": prefix = running ? "Searching" : "Searched"
            default: break
            }
        }
        return (kind, prefix, title, fileUri, fileName, startLine, endLine, command)
    }

    static func lineRangeSuffix(start: Int?, end: Int?) -> String {
        let s = start ?? 0
        let e = end ?? 0
        if s > 0 && e > 0 && e != s { return "#L\(s)-\(e)" }
        if s > 0 { return "#L\(s)" }
        if e > 0 { return "#L1-\(e)" }
        return ""
    }
}

// MARK: - Chronological Timeline Entry (Interleaved Thinking + Tools)

enum DesktopTimelineEntry: Identifiable {
    case thinking(id: String, stepIndex: Int, text: String, duration: String?, isRunning: Bool)
    case tool(id: String, stepIndex: Int, detail: ToolDetailItem)
    case fallbackTool(id: String, name: String)

    var id: String {
        switch self {
        case .thinking(let id, _, _, _, _): return id
        case .tool(let id, _, _): return id
        case .fallbackTool(let id, _): return id
        }
    }

    static func build(
        thinking: String?,
        thinkingDuration: String?,
        thinkingBlocks: [ThinkingBlockItem]?,
        toolDetails: [ToolDetailItem]?,
        fallbackTools: [String]
    ) -> [DesktopTimelineEntry] {
        var entries: [(order: Int, subOrder: Int, entry: DesktopTimelineEntry)] = []

        if let blocks = thinkingBlocks, !blocks.isEmpty {
            for (i, blk) in blocks.enumerated() {
                let text = blk.text.trimmingCharacters(in: .whitespacesAndNewlines)
                guard !text.isEmpty else { continue }
                let isRunning = (blk.status ?? "").lowercased() == "running"
                entries.append((
                    order: blk.stepIndex,
                    subOrder: i * 2,
                    entry: .thinking(
                        id: "think-\(blk.stepIndex)-\(i)",
                        stepIndex: blk.stepIndex,
                        text: text,
                        duration: blk.duration,
                        isRunning: isRunning
                    )
                ))
            }
        } else if let legacyThink = thinking?.trimmingCharacters(in: .whitespacesAndNewlines), !legacyThink.isEmpty {
            entries.append((
                order: -1,
                subOrder: 0,
                entry: .thinking(
                    id: "think-legacy",
                    stepIndex: -1,
                    text: legacyThink,
                    duration: thinkingDuration,
                    isRunning: false
                )
            ))
        }

        if let details = toolDetails, !details.isEmpty {
            for (i, detail) in details.enumerated() {
                let stepIdx = detail.stepIndex ?? (10_000 + i)
                entries.append((
                    order: stepIdx,
                    subOrder: i * 2 + 1,
                    entry: .tool(
                        id: "tool-\(stepIdx)-\(i)",
                        stepIndex: stepIdx,
                        detail: detail
                    )
                ))
            }
        } else if !fallbackTools.isEmpty {
            for (i, name) in fallbackTools.enumerated() {
                entries.append((
                    order: 10_000 + i,
                    subOrder: i * 2 + 1,
                    entry: .fallbackTool(id: "fb-\(i)", name: name)
                ))
            }
        }

        entries.sort { lhs, rhs in
            if lhs.order != rhs.order { return lhs.order < rhs.order }
            return lhs.subOrder < rhs.subOrder
        }
        return entries.map(\.entry)
    }
}

// MARK: - Shared Desktop-Parity Expanded Timeline List

public struct DesktopToolTimelineListView: View {
    public let thinking: String?
    public let thinkingDuration: String?
    public let thinkingBlocks: [ThinkingBlockItem]?
    public let toolDetails: [ToolDetailItem]?
    public let fallbackTools: [String]

    public init(
        thinking: String? = nil,
        thinkingDuration: String? = nil,
        thinkingBlocks: [ThinkingBlockItem]? = nil,
        toolDetails: [ToolDetailItem]? = nil,
        fallbackTools: [String] = []
    ) {
        self.thinking = thinking
        self.thinkingDuration = thinkingDuration
        self.thinkingBlocks = thinkingBlocks
        self.toolDetails = toolDetails
        self.fallbackTools = fallbackTools
    }

    private var entries: [DesktopTimelineEntry] {
        DesktopTimelineEntry.build(
            thinking: thinking,
            thinkingDuration: thinkingDuration,
            thinkingBlocks: thinkingBlocks,
            toolDetails: toolDetails,
            fallbackTools: fallbackTools
        )
    }

    private var isThinkingOnly: Bool {
        let hasTools = !(toolDetails?.isEmpty ?? true) || !fallbackTools.isEmpty
        return !hasTools
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: 2) {
            ForEach(entries) { entry in
                switch entry {
                case .thinking(_, _, let text, let duration, let isRunning):
                    DesktopThinkingStepRowView(
                        text: text,
                        duration: duration,
                        isRunning: isRunning,
                        initiallyExpanded: isThinkingOnly
                    )
                case .tool(_, _, let detail):
                    DesktopToolStepRowView(detail: detail)
                case .fallbackTool(_, let name):
                    HStack(spacing: 6) {
                        Text(ToolLocalization.localizedName(for: name))
                            .font(.system(size: 12.5))
                            .foregroundColor(.secondary)
                            .lineLimit(1)
                        Spacer(minLength: 0)
                    }
                    .padding(.vertical, 4)
                    .padding(.horizontal, 4)
                }
            }
        }
        .padding(.leading, 12)
        .frame(maxWidth: .infinity, alignment: .topLeading)
    }
}

// MARK: - Borderless Thinking Step Row (Matches Desktop `rTc`)

private struct DesktopThinkingStepRowView: View {
    let text: String
    let duration: String?
    let isRunning: Bool
    @State private var isExpanded: Bool
    @State private var shimmerOn: Bool = false

    init(text: String, duration: String?, isRunning: Bool, initiallyExpanded: Bool = false) {
        self.text = text
        self.duration = duration
        self.isRunning = isRunning
        self._isExpanded = State(initialValue: initiallyExpanded)
    }

    private var titleText: String {
        DesktopToolSummaryFormatter.thinkingTitle(duration: duration, isRunning: isRunning)
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            Button {
                UIImpactFeedbackGenerator(style: .light).impactOccurred()
                withAnimation(.spring(response: 0.26, dampingFraction: 1.0)) {
                    isExpanded.toggle()
                }
            } label: {
                HStack(spacing: 5) {
                    Text(titleText)
                        .font(.system(size: 12.5, weight: .regular))
                        .foregroundColor(.secondary)
                        .opacity(isRunning && shimmerOn ? 0.55 : 1.0)
                        .animation(
                            isRunning ? .easeInOut(duration: 1.2).repeatForever(autoreverses: true) : .default,
                            value: shimmerOn
                        )
                        .lineLimit(1)

                    Image(systemName: "chevron.right")
                        .font(.system(size: 9.5, weight: .medium))
                        .foregroundColor(.secondary.opacity(0.65))
                        .rotationEffect(.degrees(isExpanded ? 90 : 0))

                    Spacer(minLength: 0)
                }
                .padding(.vertical, 4)
                .padding(.horizontal, 4)
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)

            AccordionRevealContainer(isExpanded: isExpanded, topSpacing: 2) {
                HStack(alignment: .top, spacing: 8) {
                    Rectangle()
                        .fill(Color.secondary.opacity(0.24))
                        .frame(width: 1.5)
                        .padding(.vertical, 2)

                    Text(text)
                        .font(.system(size: 12))
                        .foregroundColor(.secondary)
                        .lineSpacing(3)
                        .fixedSize(horizontal: false, vertical: true)
                        .textSelection(.enabled)
                        .frame(maxWidth: .infinity, alignment: .leading)
                }
                .padding(.leading, 6)
                .padding(.trailing, 4)
                .padding(.bottom, 6)
            }
        }
        .onAppear {
            if isRunning { shimmerOn = true }
        }
    }
}

// MARK: - Borderless Desktop-Parity Tool Step Row (Matches Desktop `n4` + `q4` + `P7c`/`T7c`)

private struct DesktopToolStepRowView: View {
    @Environment(\.openURL) private var openURL
    let detail: ToolDetailItem
    @State private var isExpanded: Bool = false
    @State private var shimmerOn: Bool = false

    private var norm: (
        kind: String,
        prefix: String,
        title: String,
        fileUri: String?,
        fileName: String?,
        startLine: Int?,
        endLine: Int?,
        command: String?
    ) {
        DesktopToolSummaryFormatter.normalizedLegacyFields(for: detail)
    }

    private var isRunning: Bool {
        detail.status.lowercased() == "running"
    }

    private var isError: Bool {
        let st = detail.status.lowercased()
        if st.contains("err") || st.contains("fail") { return true }
        if let ec = detail.exitCode, ec != 0 { return true }
        return false
    }

    private var hasTerminalCard: Bool {
        let cmd = norm.command?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        let out = detail.output?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        return !cmd.isEmpty || !out.isEmpty || (detail.numLinesAbove ?? 0) > 0
    }

    private var hasMarkdownBody: Bool {
        !(detail.markdown?.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty ?? true)
    }

    private var hasErrorBody: Bool {
        !(detail.errorText?.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty ?? true)
    }

    private var isExpandable: Bool {
        hasTerminalCard || hasMarkdownBody || hasErrorBody
    }

    private var openableFileURL: URL? {
        guard !isExpandable, let raw = norm.fileUri?.trimmingCharacters(in: .whitespacesAndNewlines), !raw.isEmpty else {
            return nil
        }
        if let u = URL(string: raw), u.scheme != nil {
            return u
        }
        if raw.hasPrefix("/") {
            return URL(fileURLWithPath: raw)
        }
        return nil
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            if isExpandable {
                Button {
                    UIImpactFeedbackGenerator(style: .light).impactOccurred()
                    withAnimation(.spring(response: 0.26, dampingFraction: 1.0)) {
                        isExpanded.toggle()
                    }
                } label: {
                    headerLine
                }
                .buttonStyle(.plain)

                AccordionRevealContainer(isExpanded: isExpanded, topSpacing: 4) {
                    VStack(alignment: .leading, spacing: 6) {
                        if hasTerminalCard {
                            DesktopTerminalCardView(
                                command: norm.command ?? "",
                                cwd: detail.cwd,
                                output: detail.output,
                                numLinesAbove: detail.numLinesAbove ?? 0,
                                exitCode: detail.exitCode
                            )
                        }
                        if let md = detail.markdown?.trimmingCharacters(in: .whitespacesAndNewlines), !md.isEmpty {
                            Text(md)
                                .font(.system(size: 12))
                                .foregroundColor(.secondary)
                                .fixedSize(horizontal: false, vertical: true)
                                .textSelection(.enabled)
                                .padding(.leading, 8)
                                .padding(.vertical, 4)
                                .frame(maxWidth: .infinity, alignment: .leading)
                        }
                        if let err = detail.errorText?.trimmingCharacters(in: .whitespacesAndNewlines), !err.isEmpty {
                            HStack(alignment: .top, spacing: 6) {
                                Image(systemName: "exclamationmark.circle.fill")
                                    .font(.system(size: 12))
                                    .foregroundColor(.red)
                                    .padding(.top, 1)
                                Text(err)
                                    .font(.system(size: 11.5))
                                    .foregroundColor(.red.opacity(0.9))
                                    .fixedSize(horizontal: false, vertical: true)
                                    .textSelection(.enabled)
                            }
                            .padding(.horizontal, 10)
                            .padding(.vertical, 7)
                            .frame(maxWidth: .infinity, alignment: .leading)
                            .background(Color.red.opacity(0.08))
                            .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
                        }
                    }
                    .padding(.bottom, 4)
                }
            } else if let url = openableFileURL {
                Button {
                    UIImpactFeedbackGenerator(style: .light).impactOccurred()
                    openURL(url)
                } label: {
                    headerLine
                }
                .buttonStyle(.plain)
            } else {
                headerLine
            }
        }
        .onAppear {
            if isRunning { shimmerOn = true }
        }
    }

    @ViewBuilder
    private var headerLine: some View {
        let n = norm
        let prefixLabel = DesktopToolSummaryFormatter.localizedPrefix(n.prefix)

        HStack(spacing: 5) {
            if !prefixLabel.isEmpty {
                Text(prefixLabel)
                    .font(.system(size: 12.5, weight: .regular))
                    .foregroundColor(.secondary)
                    .fixedSize()
            }

            if let fileName = n.fileName, !fileName.isEmpty {
                HStack(spacing: 3.5) {
                    Image(systemName: n.kind == "list" ? "folder" : "doc.text")
                        .font(.system(size: 11, weight: .regular))
                        .foregroundColor(.secondary.opacity(0.85))
                    Text(fileName + DesktopToolSummaryFormatter.lineRangeSuffix(start: n.startLine, end: n.endLine))
                        .font(.system(size: 12.5, weight: .medium))
                        .foregroundColor(.secondary)
                        .lineLimit(1)
                        .truncationMode(.middle)
                }

                if detail.hasDiffStats == true || (detail.additions ?? 0) > 0 || (detail.deletions ?? 0) > 0 {
                    HStack(spacing: 3) {
                        Text("+\(detail.additions ?? 0)")
                            .foregroundColor(Color(red: 0.16, green: 0.72, blue: 0.36))
                        Text("-\(detail.deletions ?? 0)")
                            .foregroundColor(Color(red: 0.90, green: 0.28, blue: 0.28))
                    }
                    .font(.system(size: 11.5, weight: .medium, design: .monospaced))
                    .fixedSize()
                }
            } else if !n.title.isEmpty {
                Text(n.title)
                    .font(.system(size: 12.5, weight: .regular))
                    .foregroundColor(.secondary)
                    .lineLimit(1)
                    .truncationMode(.tail)
            } else if let cmd = n.command, !cmd.isEmpty {
                Text(cmd)
                    .font(.system(size: 11.5, weight: .regular, design: .monospaced))
                    .foregroundColor(.secondary)
                    .lineLimit(1)
                    .truncationMode(.tail)
            } else if prefixLabel.isEmpty {
                Text(ToolLocalization.localizedName(for: detail.name))
                    .font(.system(size: 12.5, weight: .regular))
                    .foregroundColor(.secondary)
                    .lineLimit(1)
            }

            if let results = detail.numResults, results > 0, n.kind == "search" {
                Text(DesktopToolSummaryFormatter.resultsBadge(results))
                    .font(.system(size: 10.5, weight: .regular))
                    .foregroundColor(.secondary)
                    .padding(.horizontal, 6)
                    .padding(.vertical, 1.5)
                    .background(Color.secondary.opacity(0.12))
                    .clipShape(Capsule())
                    .fixedSize()
            }

            if isError {
                Circle()
                    .fill(Color.red)
                    .frame(width: 6, height: 6)
            }

            if isExpandable {
                Image(systemName: "chevron.right")
                    .font(.system(size: 9.5, weight: .medium))
                    .foregroundColor(.secondary.opacity(0.65))
                    .rotationEffect(.degrees(isExpanded ? 90 : 0))
            }

            Spacer(minLength: 0)
        }
        .opacity(isRunning && shimmerOn ? 0.55 : 1.0)
        .animation(
            isRunning ? .easeInOut(duration: 1.2).repeatForever(autoreverses: true) : .default,
            value: shimmerOn
        )
        .padding(.vertical, 4)
        .padding(.horizontal, 4)
        .contentShape(Rectangle())
    }
}

// MARK: - Expandable Terminal Output Card (Matches Desktop `T7c`)

private struct DesktopTerminalCardView: View {
    let command: String
    let cwd: String?
    let output: String?
    let numLinesAbove: Int
    let exitCode: Int?
    @State private var copied: Bool = false

    private var hasOutput: Bool {
        !(output?.isEmpty ?? true) || numLinesAbove > 0
    }

    private var isNonZeroExit: Bool {
        if let ec = exitCode, ec != 0 { return true }
        return false
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            if !command.isEmpty {
                HStack(alignment: .top, spacing: 6) {
                    Group {
                        if let dir = cwd, !dir.isEmpty {
                            Text("\(dir) $")
                        } else {
                            Text("$")
                        }
                    }
                    .font(.system(size: 11, design: .monospaced))
                    .foregroundColor(.secondary)
                    .fixedSize()

                    Text(command)
                        .font(.system(size: 11.5, design: .monospaced))
                        .foregroundColor(.primary.opacity(0.9))
                        .fixedSize(horizontal: false, vertical: true)
                        .textSelection(.enabled)
                        .frame(maxWidth: .infinity, alignment: .leading)

                    HStack(spacing: 6) {
                        Button {
                            UIPasteboard.general.string = command
                            UIImpactFeedbackGenerator(style: .light).impactOccurred()
                            copied = true
                            DispatchQueue.main.asyncAfter(deadline: .now() + 1.5) {
                                copied = false
                            }
                        } label: {
                            Image(systemName: copied ? "checkmark" : "doc.on.doc")
                                .font(.system(size: 11, weight: .medium))
                                .foregroundColor(copied ? .green : .secondary)
                                .frame(width: 22, height: 22)
                                .contentShape(Rectangle())
                        }
                        .buttonStyle(.plain)

                        if isNonZeroExit {
                            Image(systemName: "exclamationmark.circle.fill")
                                .font(.system(size: 12))
                                .foregroundColor(.red)
                        }
                    }
                }
                .padding(.horizontal, 10)
                .padding(.vertical, 7)
            }

            if hasOutput {
                if !command.isEmpty {
                    Divider().opacity(0.6)
                }
                ScrollView(.vertical, showsIndicators: true) {
                    VStack(alignment: .leading, spacing: 4) {
                        if numLinesAbove > 0 {
                            Text(DesktopToolSummaryFormatter.hiddenLinesText(numLinesAbove))
                                .font(.system(size: 10.5, design: .monospaced))
                                .foregroundColor(.secondary)
                        }
                        if let out = output, !out.isEmpty {
                            Text(out)
                                .font(.system(size: 11, design: .monospaced))
                                .foregroundColor(.primary.opacity(0.85))
                                .fixedSize(horizontal: false, vertical: true)
                                .textSelection(.enabled)
                                .frame(maxWidth: .infinity, alignment: .leading)
                        }
                    }
                    .padding(.horizontal, 10)
                    .padding(.vertical, 7)
                    .frame(maxWidth: .infinity, alignment: .leading)
                }
                .frame(maxHeight: 200)
            }
        }
        .background(Color(uiColor: .secondarySystemBackground).opacity(0.75))
        .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
        .overlay(
            RoundedRectangle(cornerRadius: 10, style: .continuous)
                .strokeBorder(Color.primary.opacity(0.10), lineWidth: 0.8)
        )
    }
}

// MARK: - Completed Tool Group Accordion (Desktop-Aligned Borderless Header)

extension MessageBubbleView {
    struct NoTapAnimationButtonStyle: ButtonStyle {
        func makeBody(configuration: Configuration) -> some View {
            configuration.label
        }
    }

    public struct ToolBatchAccordionView: View {
        public let count: Int
        public let tools: [String]
        public let thinking: String?
        public let thinkingDuration: String?
        public let thinkingBlocks: [ThinkingBlockItem]?
        public let toolDetails: [ToolDetailItem]?
        @State private var isExpanded: Bool = false

        public init(
            count: Int,
            tools: [String],
            thinking: String? = nil,
            thinkingDuration: String? = nil,
            thinkingBlocks: [ThinkingBlockItem]? = nil,
            toolDetails: [ToolDetailItem]? = nil
        ) {
            self.count = count
            self.tools = tools
            self.thinking = thinking
            self.thinkingDuration = thinkingDuration
            self.thinkingBlocks = thinkingBlocks
            self.toolDetails = toolDetails
        }

        private var resolvedTools: [String] {
            if !tools.isEmpty {
                return tools
            }
            let actualCount = max(count, 0)
            return actualCount > 0 ? (0..<actualCount).map { _ in "tool_call" } : []
        }

        private var summaryText: String {
            DesktopToolSummaryFormatter.completedSummary(
                count: count,
                tools: tools,
                toolDetails: toolDetails,
                thinking: thinking,
                thinkingDuration: thinkingDuration,
                thinkingBlocks: thinkingBlocks
            )
        }

        public var body: some View {
            VStack(alignment: .leading, spacing: 0) {
                Button(action: {
                    UIImpactFeedbackGenerator(style: .light).impactOccurred()
                    withAnimation(.spring(response: 0.28, dampingFraction: 1.0)) {
                        isExpanded.toggle()
                    }
                }) {
                    HStack(spacing: 6) {
                        Text(summaryText)
                            .font(.system(size: 13, weight: .regular))
                            .foregroundColor(.secondary)
                            .lineLimit(1)
                            .truncationMode(.tail)

                        Image(systemName: "chevron.right")
                            .font(.system(size: 10, weight: .medium))
                            .foregroundColor(.secondary.opacity(0.65))
                            .rotationEffect(.degrees(isExpanded ? 90 : 0))

                        Spacer(minLength: 0)
                    }
                    .padding(.horizontal, 4)
                    .padding(.vertical, 3)
                    .contentShape(Rectangle())
                }
                .buttonStyle(NoTapAnimationButtonStyle())
                .zIndex(2)

                AccordionRevealContainer(isExpanded: isExpanded, topSpacing: 4) {
                    DesktopToolTimelineListView(
                        thinking: thinking,
                        thinkingDuration: thinkingDuration,
                        thinkingBlocks: thinkingBlocks,
                        toolDetails: toolDetails,
                        fallbackTools: resolvedTools
                    )
                }
                .zIndex(1)
            }
            .frame(maxWidth: .infinity, alignment: .topLeading)
        }
    }
}
