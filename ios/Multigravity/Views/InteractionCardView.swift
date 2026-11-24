import SwiftUI

private struct OptionsHeightKey: PreferenceKey {
    static var defaultValue: CGFloat = 0
    static func reduce(value: inout CGFloat, nextValue: () -> CGFloat) {
        value = max(value, nextValue())
    }
}

private struct HeaderHeightKey: PreferenceKey {
    static var defaultValue: CGFloat = 0
    static func reduce(value: inout CGFloat, nextValue: () -> CGFloat) {
        value = max(value, nextValue())
    }
}

public struct InteractionCardView: View {
    public let interaction: PendingInteraction
    public let isSubmitting: Bool
    public let maxAvailableHeight: CGFloat?
    public let onSubmit: (String, String?, String?, [QuestionResponse]?) -> Void
    public let onSkip: ([QuestionResponse]?) -> Void
    public let onToggleExpand: (@MainActor @Sendable (Bool) -> Void)?
    
    @State private var selectedOptionId: String
    @State private var writeInText: String = ""
    @State private var targetText: String
    @State private var settings = AppSettings.shared
    @FocusState private var isSingleWriteInFocused: Bool
    @FocusState private var focusedMultiQuestionIndex: Int?
    @State private var pendingAutoFocusSingleWriteIn: Bool = false
    @State private var pendingAutoFocusQuestionIndex: Int? = nil
    
    // Single-question multi-select state
    @State private var singleMultiSelectedIds: Set<String> = []
    @State private var singleWriteInActive: Bool = false

    // Multi-question state: questionIndex -> selectedOptionId (single) / selectedOptionIds (multi) / writeIn
    @State private var selectedOptionsByQuestion: [Int: String] = [:]
    @State private var multiSelectedOptionsByQuestion: [Int: Set<String>] = [:]
    @State private var writeInActiveByQuestion: [Int: Bool] = [:]
    @State private var writeInByQuestion: [Int: String] = [:]
    
    // Expandable card state (default: adaptive height up to ~half chat area, expandable to full chat area without overlapping nav bar)
    @State private var isExpanded: Bool = false
    @State private var isDiffExpanded: Bool = false
    @State private var scrollContentHeight: CGFloat = 0
    @State private var headerHeight: CGFloat = 28

    private var approvalPreview: ApprovalPreviewDetails? {
        interaction.approvalPreview
    }

    private var isHighRiskApproval: Bool {
        approvalPreview?.riskLevel == "high"
    }

    private var isMediumRiskApproval: Bool {
        approvalPreview?.riskLevel == "medium" && !(approvalPreview?.riskReasons ?? []).isEmpty
    }

    private var diffLines: [String] {
        guard let raw = approvalPreview?.diffPreview, !raw.isEmpty else { return [] }
        return raw.split(separator: "\n", omittingEmptySubsequences: false).map(String.init)
    }

    private func localizedRiskReason(_ code: String) -> String {
        switch code {
        case "destructive_delete": return String(localized: "破坏性删除")
        case "force_git": return String(localized: "Git 强推/硬重置")
        case "privilege_exec": return String(localized: "提权执行")
        case "remote_pipe_exec": return String(localized: "远程脚本执行")
        case "destructive_sql": return String(localized: "高危 SQL 删除")
        case "system_ops": return String(localized: "系统级操作")
        case "sensitive_path": return String(localized: "敏感凭证路径")
        case "outside_workspace": return String(localized: "工作区外路径")
        case "overwrite_file": return String(localized: "覆盖写入")
        case "external_write": return String(localized: "外部写入操作")
        case "git_push": return String(localized: "推送远端")
        default: return code
        }
    }

    private var riskSummaryText: String? {
        let reasons = (approvalPreview?.riskReasons ?? []).map(localizedRiskReason)
        if isHighRiskApproval {
            let prefix = String(localized: "高危操作")
            return reasons.isEmpty ? prefix : "\(prefix) · \(reasons.joined(separator: " · "))"
        }
        if isMediumRiskApproval {
            let prefix = String(localized: "注意")
            return "\(prefix) · \(reasons.joined(separator: " · "))"
        }
        return nil
    }
    
    private var isPermissionType: Bool {
        interaction.type == "permission" || interaction.type == "file_permission"
    }
    
    private var hasMultipleQuestions: Bool {
        if let questions = interaction.questions, questions.count > 1 {
            return true
        }
        return false
    }

    private var isSingleMultiSelect: Bool {
        !hasMultipleQuestions && ((interaction.isMultiSelect == true) || (interaction.questions?.first?.isMultiSelect == true))
    }

    private func isOptionWriteIn(_ option: InteractionOption, hasWriteIn: Bool) -> Bool {
        if option.id == "__write_in__" { return true }
        guard hasWriteIn else { return false }
        if option.isDeny == true { return true }
        if isPermissionType && option.id == "5" { return true }
        return false
    }

    private var singleMultiSelectionCount: Int {
        let trimmed = writeInText.trimmingCharacters(in: .whitespacesAndNewlines)
        return singleMultiSelectedIds.count + (singleWriteInActive && !trimmed.isEmpty ? 1 : 0)
    }

    private var canSubmit: Bool {
        if hasMultipleQuestions, let questions = interaction.questions {
            for (idx, q) in questions.enumerated() where q.isMultiSelect == true {
                let hasOpts = !(multiSelectedOptionsByQuestion[idx] ?? []).isEmpty
                let trimmed = (writeInByQuestion[idx] ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
                let hasWriteIn = (writeInActiveByQuestion[idx] == true) && !trimmed.isEmpty
                if !hasOpts && !hasWriteIn {
                    return false
                }
            }
            return true
        }
        if isSingleMultiSelect {
            return singleMultiSelectionCount > 0
        }
        return true
    }

    private var submitButtonTitle: String {
        if isSingleMultiSelect && singleMultiSelectionCount > 0 {
            return "\(String(localized: "提交")) (\(singleMultiSelectionCount))"
        }
        return String(localized: "提交")
    }

    private var singleQuestionTitle: String {
        interaction.title.isEmpty ? (interaction.description ?? String(localized: "需要用户审批操作")) : interaction.title
    }

    /// 当单题问题描述较长（含换行或超过 48 字）时，将其移入统一的滚动区域，
    /// 避免顶栏被撑爆而撞上导航栏或挤占选项空间。
    private var isLongSingleQuestion: Bool {
        !hasMultipleQuestions && (singleQuestionTitle.contains("\n") || singleQuestionTitle.count > 48)
    }

    private var compactCategoryTitle: String {
        if interaction.permissionGrant != nil {
            return String(localized: "权限确认")
        }
        switch interaction.type {
        case "permission", "file_permission":
            return String(localized: "权限确认")
        case "browser_download":
            return String(localized: "下载确认")
        case "approval":
            return String(localized: "操作确认")
        case "ask_question":
            return String(localized: "询问用户")
        case "run_command":
            return String(localized: "执行命令")
        case "mcp":
            return String(localized: "MCP 工具确认")
        case "elicitation":
            return String(localized: "补充信息")
        default:
            return String(localized: "需要确认")
        }
    }

    private var effectiveAvailableHeight: CGFloat {
        if let maxAvailableHeight, maxAvailableHeight > 0 {
            return maxAvailableHeight
        }
        return 380
    }

    /// 卡片固定壳层高度（顶栏 + 底部按钮栏 + 内外边距与分割线）
    private var fixedChromeHeight: CGFloat {
        max(headerHeight, 28) + 96
    }

    private var maxCardCollapsedHeight: CGFloat {
        let preferred = min(440, max(260, effectiveAvailableHeight * 0.56))
        return min(max(180, effectiveAvailableHeight - 16), preferred)
    }

    private var maxCardExpandedHeight: CGFloat {
        max(maxCardCollapsedHeight, effectiveAvailableHeight - 16)
    }

    private var maxScrollCollapsedHeight: CGFloat {
        max(72, maxCardCollapsedHeight - fixedChromeHeight)
    }

    private var maxScrollExpandedHeight: CGFloat {
        max(maxScrollCollapsedHeight, maxCardExpandedHeight - fixedChromeHeight)
    }

    private var exceedsCollapsedHeight: Bool {
        scrollContentHeight > maxScrollCollapsedHeight + 2
    }

    private var canExpand: Bool {
        isExpanded || (exceedsCollapsedHeight && maxScrollExpandedHeight > maxScrollCollapsedHeight + 24)
    }

    private var targetScrollHeight: CGFloat {
        let maxH = isExpanded ? maxScrollExpandedHeight : maxScrollCollapsedHeight
        if scrollContentHeight > 0 {
            return min(scrollContentHeight, maxH)
        }
        let optionCount = hasMultipleQuestions ? (interaction.questions?.count ?? 1) * 2 : interaction.options.count
        let fallback = CGFloat(optionCount * 55 + (isLongSingleQuestion ? 160 : 15))
        return min(fallback, maxH)
    }
    
    public init(
        interaction: PendingInteraction,
        isSubmitting: Bool,
        maxAvailableHeight: CGFloat? = nil,
        onSubmit: @escaping (String, String?, String?, [QuestionResponse]?) -> Void,
        onSkip: @escaping ([QuestionResponse]?) -> Void,
        onToggleExpand: (@MainActor @Sendable (Bool) -> Void)? = nil
    ) {
        self.interaction = interaction
        self.isSubmitting = isSubmitting
        self.maxAvailableHeight = maxAvailableHeight
        self.onSubmit = onSubmit
        self.onSkip = onSkip
        self.onToggleExpand = onToggleExpand
        
        let defaultFallback = (interaction.hasWriteIn == true) ? "__write_in__" : "1"
        let initialSelectedId = interaction.defaultOptionId ?? interaction.options.first?.id ?? defaultFallback
        _selectedOptionId = State(initialValue: initialSelectedId)
        _targetText = State(initialValue: interaction.target ?? "")
        
        var qMap: [Int: String] = [:]
        var qMultiMap: [Int: Set<String>] = [:]
        if let questions = interaction.questions {
            for (idx, q) in questions.enumerated() {
                if q.isMultiSelect == true {
                    qMultiMap[idx] = []
                } else {
                    let qFallback = (q.hasWriteIn == true) ? "__write_in__" : "1"
                    qMap[idx] = q.defaultOptionId ?? q.options.first?.id ?? qFallback
                }
            }
        }
        _selectedOptionsByQuestion = State(initialValue: qMap)
        _multiSelectedOptionsByQuestion = State(initialValue: qMultiMap)
    }
    
    private var headerIconName: String {
        if isHighRiskApproval {
            return "exclamationmark.triangle.fill"
        }
        if interaction.permissionGrant != nil {
            return "lock.shield.fill"
        }
        switch interaction.type {
        case "permission", "file_permission":
            return "hand.raised.fill"
        case "browser_download":
            return "arrow.down.doc.fill"
        case "approval":
            return "checkmark.shield.fill"
        case "ask_question":
            return "questionmark.bubble.fill"
        case "run_command":
            return "terminal.fill"
        case "mcp":
            return "puzzlepiece.extension.fill"
        case "elicitation":
            return "rectangle.and.pencil.and.ellipsis"
        default:
            return "questionmark.circle.fill"
        }
    }
    
    private var headerIconColor: Color {
        if isHighRiskApproval {
            return .red
        }
        if interaction.permissionGrant != nil {
            return .orange
        }
        switch interaction.type {
        case "permission", "file_permission":
            return .blue
        case "browser_download":
            return .orange
        case "approval":
            return .indigo
        case "ask_question":
            return .orange
        case "run_command":
            return .orange
        case "mcp":
            return .purple
        case "elicitation":
            return .teal
        default:
            return .orange
        }
    }
    
    private func resolveWriteInLabel(_ raw: String?) -> String {
        guard let raw = raw?.trimmingCharacters(in: .whitespacesAndNewlines), !raw.isEmpty else {
            return String(localized: "自定义输入")
        }
        let lower = raw.lowercased()
        if lower == "other" || lower == "write in" || lower == "write in..." {
            return String(localized: "自定义输入")
        }
        return raw
    }

    private func resolveWriteInPlaceholder(_ raw: String?, isDeny: Bool) -> String {
        guard let raw = raw?.trimmingCharacters(in: .whitespacesAndNewlines), !raw.isEmpty else {
            return isDeny ? String(localized: "告诉 Agent 应该怎么做...") : String(localized: "输入自定义回答...")
        }
        let lower = raw.lowercased()
        if lower.contains("write in your response") || lower.contains("write in") {
            return String(localized: "输入自定义回答...")
        }
        if lower.contains("tell the agent") || lower.contains("what to do instead") {
            return String(localized: "告诉 Agent 应该怎么做...")
        }
        return raw
    }

    private func displayOptions(for q: InteractionQuestion) -> [InteractionOption] {
        let hasWriteIn = (q.hasWriteIn == true) && !q.options.contains(where: { $0.id == "__write_in__" || $0.isDeny == true })
        guard hasWriteIn else { return q.options }
        let label = resolveWriteInLabel(q.writeInLabel)
        return q.options + [InteractionOption(id: "__write_in__", text: label)]
    }

    private var singleDisplayOptions: [InteractionOption] {
        let hasWriteIn = (interaction.hasWriteIn == true) && !interaction.options.contains(where: { $0.id == "__write_in__" || $0.isDeny == true })
        guard hasWriteIn else { return interaction.options }
        let label = resolveWriteInLabel(interaction.writeInLabel)
        return interaction.options + [InteractionOption(id: "__write_in__", text: label)]
    }

    private func badgeText(for option: InteractionOption, in options: [InteractionOption], index: Int) -> String {
        if option.id == "__write_in__" {
            let nonWriteIn = options.filter { $0.id != "__write_in__" }
            let isNumeric = !nonWriteIn.isEmpty && nonWriteIn.allSatisfy { Int($0.id) != nil }
            if isNumeric {
                return "\(index + 1)"
            }
            return "✎"
        }
        return option.id
    }

    private var isDenySelected: Bool {
        if let opt = singleDisplayOptions.first(where: { $0.id == selectedOptionId }) {
            if opt.isDeny == true { return true }
            if isPermissionType && (opt.id == "5" || opt.id == "__write_in__") { return true }
            return false
        }
        return isPermissionType && (selectedOptionId == "5" || selectedOptionId == "__write_in__")
    }

    private var isWriteInSelected: Bool {
        if isSingleMultiSelect {
            return singleWriteInActive
        }
        if selectedOptionId == "__write_in__" { return true }
        guard interaction.hasWriteIn == true else { return false }
        if let opt = singleDisplayOptions.first(where: { $0.id == selectedOptionId }) {
            return isOptionWriteIn(opt, hasWriteIn: true)
        }
        return isPermissionType && selectedOptionId == "5"
    }
    
    private func isQuestionDenySelected(qIdx: Int, q: InteractionQuestion) -> Bool {
        if q.isMultiSelect == true {
            return writeInActiveByQuestion[qIdx] == true
        }
        let optId = selectedOptionsByQuestion[qIdx] ?? ""
        let opts = displayOptions(for: q)
        if let opt = opts.first(where: { $0.id == optId }) {
            return isOptionWriteIn(opt, hasWriteIn: q.hasWriteIn == true)
        }
        return optId == "__write_in__"
    }
    
    private func dismissKeyboard() {
        pendingAutoFocusQuestionIndex = nil
        pendingAutoFocusSingleWriteIn = false
        focusedMultiQuestionIndex = nil
        isSingleWriteInFocused = false
        UIApplication.shared.sendAction(#selector(UIResponder.resignFirstResponder), to: nil, from: nil, for: nil)
    }

    private func toggleExpansion() {
        dismissKeyboard()
        UISelectionFeedbackGenerator().selectionChanged()
        withAnimation(.spring(response: 0.35, dampingFraction: 0.82)) {
            isExpanded.toggle()
        }
        onToggleExpand?(isExpanded)
    }
    
    private func handleSkip() {
        dismissKeyboard()
        if hasMultipleQuestions, let questions = interaction.questions {
            let responses = questions.enumerated().map { idx, _ in
                QuestionResponse(questionIndex: idx, selectedOptionIds: [], writeInResponse: "", skipped: true)
            }
            onSkip(responses)
        } else {
            onSkip(nil)
        }
    }
    
    private func handleSubmit() {
        guard canSubmit else { return }
        dismissKeyboard()
        if hasMultipleQuestions, let questions = interaction.questions {
            let responses = questions.enumerated().map { idx, q -> QuestionResponse in
                if q.isMultiSelect == true {
                    let selectedSet = multiSelectedOptionsByQuestion[idx] ?? []
                    let orderedIds = q.options.map(\.id).filter { selectedSet.contains($0) }
                    let trimmedWriteIn = (writeInActiveByQuestion[idx] == true)
                        ? (writeInByQuestion[idx] ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
                        : ""
                    return QuestionResponse(
                        questionIndex: idx,
                        selectedOptionIds: orderedIds,
                        writeInResponse: trimmedWriteIn.isEmpty ? nil : trimmedWriteIn,
                        skipped: false
                    )
                } else {
                    let qFallback = (q.hasWriteIn == true) ? "__write_in__" : "1"
                    let optId = selectedOptionsByQuestion[idx] ?? q.defaultOptionId ?? q.options.first?.id ?? qFallback
                    let isWriteIn = (optId == "__write_in__") || (q.options.first(where: { $0.id == optId })?.isDeny == true)
                    let ids = isWriteIn ? [] : [optId]
                    let text = isWriteIn ? (writeInByQuestion[idx] ?? "").trimmingCharacters(in: .whitespacesAndNewlines) : ""
                    return QuestionResponse(
                        questionIndex: idx,
                        selectedOptionIds: ids,
                        writeInResponse: text.isEmpty ? nil : text,
                        skipped: false
                    )
                }
            }
            let primaryOptId = responses.first?.selectedOptionIds.first ?? ""
            onSubmit(primaryOptId, nil, interaction.target, responses)
        } else if isSingleMultiSelect {
            let orderedIds = interaction.options.map(\.id).filter { singleMultiSelectedIds.contains($0) }
            let trimmedWriteIn = singleWriteInActive
                ? writeInText.trimmingCharacters(in: .whitespacesAndNewlines)
                : ""
            let response = QuestionResponse(
                questionIndex: 0,
                selectedOptionIds: orderedIds,
                writeInResponse: trimmedWriteIn.isEmpty ? nil : trimmedWriteIn,
                skipped: false
            )
            let primaryOptId = orderedIds.first ?? (trimmedWriteIn.isEmpty ? "" : "__write_in__")
            onSubmit(primaryOptId, trimmedWriteIn.isEmpty ? nil : trimmedWriteIn, interaction.target, [response])
        } else {
            let text = (isWriteInSelected || isDenySelected) ? writeInText : nil
            onSubmit(selectedOptionId, text, interaction.target, nil)
        }
    }

    private var multiSelectBadgeView: some View {
        Text("多选")
            .font(.system(size: 11, weight: .semibold))
            .foregroundColor(.blue)
            .padding(.horizontal, 7)
            .padding(.vertical, 2)
            .background(Color.blue.opacity(0.12))
            .clipShape(Capsule())
            .fixedSize()
    }
    
    @ViewBuilder
    private var headerView: some View {
        HStack(alignment: isLongSingleQuestion || hasMultipleQuestions ? .center : .top, spacing: 8) {
            Image(systemName: headerIconName)
                .font(.system(size: 15, weight: .semibold))
                .foregroundColor(headerIconColor)
                .padding(.top, isLongSingleQuestion || hasMultipleQuestions ? 0 : 2)
            
            if hasMultipleQuestions, let count = interaction.questions?.count {
                Text("需要确认规格 (\(count) 个问题)")
                    .font(.system(size: 14.5, weight: .semibold))
                    .foregroundColor(.primary)
                    .lineLimit(1)
            } else if isLongSingleQuestion {
                HStack(spacing: 6) {
                    Text(compactCategoryTitle)
                        .font(.system(size: 14.5, weight: .semibold))
                        .foregroundColor(.primary)
                        .lineLimit(1)
                    if isSingleMultiSelect {
                        multiSelectBadgeView
                    }
                }
            } else if isSingleMultiSelect {
                ViewThatFits(in: .horizontal) {
                    HStack(alignment: .center, spacing: 6) {
                        Text(singleQuestionTitle)
                            .font(.system(size: 14.5, weight: .semibold))
                            .foregroundColor(.primary)
                            .lineLimit(1)
                            .fixedSize(horizontal: true, vertical: false)
                        multiSelectBadgeView
                    }
                    VStack(alignment: .leading, spacing: 5) {
                        Text(singleQuestionTitle)
                            .font(.system(size: 14.5, weight: .semibold))
                            .foregroundColor(.primary)
                            .lineSpacing(2.5)
                            .lineLimit(3)
                            .multilineTextAlignment(.leading)
                            .fixedSize(horizontal: false, vertical: true)
                        multiSelectBadgeView
                    }
                }
            } else {
                Text(singleQuestionTitle)
                    .font(.system(size: 14.5, weight: .semibold))
                    .foregroundColor(.primary)
                    .lineSpacing(2.5)
                    .lineLimit(3)
                    .multilineTextAlignment(.leading)
                    .fixedSize(horizontal: false, vertical: true)
            }
            
            Spacer(minLength: 4)
            
            if canExpand {
                // 一键展开至会话区域全屏 / 收起回半屏
                Button {
                    toggleExpansion()
                } label: {
                    HStack(spacing: 4) {
                        Text(isExpanded ? String(localized: "收起") : String(localized: "展开"))
                            .font(.system(size: 11.5, weight: .medium))
                        Image(systemName: isExpanded ? "chevron.down" : "chevron.up")
                            .font(.system(size: 11, weight: .semibold))
                    }
                    .foregroundColor(.secondary)
                    .padding(.horizontal, 9)
                    .padding(.vertical, 5)
                    .background(Color(uiColor: .tertiarySystemFill))
                    .clipShape(Capsule())
                }
                .buttonStyle(.plain)
                .help(isExpanded ? String(localized: "收起为半屏") : String(localized: "一键全屏"))
            }
        }
        .background(
            GeometryReader { geo in
                Color.clear.preference(key: HeaderHeightKey.self, value: geo.size.height)
            }
        )
        .contentShape(Rectangle())
        .onTapGesture {
            if canExpand {
                toggleExpansion()
            }
        }
    }
    
    @ViewBuilder
    private var riskSummaryBannerView: some View {
        if let summary = riskSummaryText {
            let accent: Color = isHighRiskApproval ? .red : .orange
            HStack(spacing: 6) {
                Image(systemName: isHighRiskApproval ? "exclamationmark.shield.fill" : "exclamationmark.circle.fill")
                    .font(.system(size: 11.5, weight: .semibold))
                    .foregroundColor(accent)
                Text(summary)
                    .font(.system(size: 11.5, weight: isHighRiskApproval ? .semibold : .medium))
                    .foregroundColor(accent)
                    .lineLimit(2)
                    .fixedSize(horizontal: false, vertical: true)
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(.horizontal, 10)
            .padding(.vertical, 6)
            .background(accent.opacity(isHighRiskApproval ? 0.12 : 0.10))
            .overlay(
                RoundedRectangle(cornerRadius: 8, style: .continuous)
                    .stroke(accent.opacity(isHighRiskApproval ? 0.35 : 0.25), lineWidth: 0.75)
            )
            .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
        }
    }

    @ViewBuilder
    private var targetBoxView: some View {
        if let pg = interaction.permissionGrant {
            VStack(alignment: .leading, spacing: 6) {
                HStack(spacing: 6) {
                    Text(pg.binary.uppercased())
                        .font(.system(size: 10, weight: .bold))
                        .foregroundColor(isHighRiskApproval ? .red : .orange)
                        .padding(.horizontal, 6)
                        .padding(.vertical, 2)
                        .background((isHighRiskApproval ? Color.red : Color.orange).opacity(0.18))
                        .clipShape(RoundedRectangle(cornerRadius: 4, style: .continuous))
                    
                    Text("操作: \(pg.action)")
                        .font(.system(size: 12, weight: .medium))
                        .foregroundColor(.primary)
                }
                
                if let repo = pg.repo, !repo.isEmpty {
                    let repoText = (pg.org != nil && !pg.org!.isEmpty) ? "\(pg.org!)/\(repo)" : repo
                    Text("仓库: \(repoText)")
                        .font(.system(size: 11.5, design: .monospaced))
                        .foregroundColor(.secondary)
                }
                
                if let branch = pg.branch, !branch.isEmpty {
                    Text("目标分支: \(branch)")
                        .font(.system(size: 11.5, design: .monospaced))
                        .foregroundColor(.secondary)
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(.horizontal, 10)
            .padding(.vertical, 8)
            .background(Color(uiColor: .tertiarySystemFill).opacity(0.8))
            .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
        } else {
            let displayTarget: String = {
                if let dp = approvalPreview?.displayPath, !dp.isEmpty, interaction.type != "run_command" {
                    return dp
                }
                return interaction.target ?? ""
            }()
            let cwdText = approvalPreview?.cwd ?? ""
            if !displayTarget.isEmpty || !cwdText.isEmpty {
                VStack(alignment: .leading, spacing: 4) {
                    if !displayTarget.isEmpty {
                        formattedTargetText(displayTarget)
                            .lineLimit(isExpanded ? 8 : 3)
                            .frame(maxWidth: .infinity, alignment: .leading)
                    }
                    if !cwdText.isEmpty {
                        HStack(spacing: 4) {
                            Image(systemName: "folder")
                                .font(.system(size: 10))
                                .foregroundColor(Color(uiColor: .tertiaryLabel))
                            Text(cwdText)
                                .font(.system(size: 10.5, design: .monospaced))
                                .foregroundColor(Color(uiColor: .tertiaryLabel))
                                .lineLimit(1)
                                .truncationMode(.middle)
                        }
                    }
                }
                .frame(maxWidth: .infinity, alignment: .leading)
                .padding(.horizontal, 10)
                .padding(.vertical, 7)
                .background(Color(uiColor: .tertiarySystemFill))
                .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
            }
        }
    }

    /// Antigravity 2.35 parity: bold target path/pattern suffix for faster visual scanning.
    private func formattedTargetText(_ raw: String) -> Text {
        let baseFont = Font.system(size: 11.5, design: .monospaced)
        let boldFont = Font.system(size: 11.5, weight: .bold, design: .monospaced)
        let baseColor: Color = isHighRiskApproval ? .primary : .secondary
        if !raw.contains("\n"), let slashIdx = raw.lastIndex(of: "/"), slashIdx < raw.index(before: raw.endIndex) {
            let splitAfter = raw.index(after: slashIdx)
            let prefix = String(raw[..<splitAfter])
            let suffix = String(raw[splitAfter...])
            return Text(prefix).font(baseFont).foregroundColor(baseColor)
                + Text(suffix).font(boldFont).foregroundColor(.primary)
        }
        return Text(raw).font(boldFont).foregroundColor(.primary)
    }

    @ViewBuilder
    private var compactDiffPreviewView: some View {
        let lines = diffLines
        if !lines.isEmpty {
            let added = approvalPreview?.addedLines ?? 0
            let removed = approvalPreview?.removedLines ?? 0
            let isNew = (approvalPreview?.isNewFile == true) && removed == 0
            let truncated = approvalPreview?.diffTruncated == true

            VStack(alignment: .leading, spacing: 6) {
                Button {
                    UISelectionFeedbackGenerator().selectionChanged()
                    withAnimation(.spring(response: 0.28, dampingFraction: 0.85)) {
                        isDiffExpanded.toggle()
                    }
                } label: {
                    HStack(spacing: 6) {
                        Image(systemName: "doc.text.magnifyingglass")
                            .font(.system(size: 11, weight: .semibold))
                            .foregroundColor(.secondary)

                        Text(isDiffExpanded ? String(localized: "收起改动") : String(localized: "预览改动"))
                            .font(.system(size: 11.5, weight: .medium))
                            .foregroundColor(.primary)

                        if isNew {
                            Text("新建")
                                .font(.system(size: 10, weight: .semibold))
                                .foregroundColor(.blue)
                                .padding(.horizontal, 5)
                                .padding(.vertical, 1)
                                .background(Color.blue.opacity(0.14))
                                .clipShape(RoundedRectangle(cornerRadius: 4, style: .continuous))
                        }

                        if added > 0 {
                            Text("+\(added)")
                                .font(.system(size: 11, weight: .semibold, design: .monospaced))
                                .foregroundColor(.green)
                        }

                        if removed > 0 {
                            Text("-\(removed)")
                                .font(.system(size: 11, weight: .semibold, design: .monospaced))
                                .foregroundColor(.red)
                        }

                        Spacer(minLength: 4)

                        Image(systemName: isDiffExpanded ? "chevron.up" : "chevron.down")
                            .font(.system(size: 10.5, weight: .semibold))
                            .foregroundColor(.secondary)
                    }
                    .padding(.horizontal, 10)
                    .padding(.vertical, 6)
                    .background(Color(uiColor: .tertiarySystemFill).opacity(0.75))
                    .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
                }
                .buttonStyle(.plain)

                if isDiffExpanded {
                    let rowHeight: CGFloat = 17
                    let boxHeight = min(CGFloat(lines.count) * rowHeight + (truncated ? 28 : 14), 180)
                    ScrollView([.vertical, .horizontal], showsIndicators: true) {
                        VStack(alignment: .leading, spacing: 1) {
                            ForEach(Array(lines.enumerated()), id: \.offset) { _, line in
                                diffLineView(line)
                            }
                            if truncated {
                                Text("仅展示前 40 行改动")
                                    .font(.system(size: 10.5))
                                    .foregroundColor(Color(uiColor: .tertiaryLabel))
                                    .padding(.top, 4)
                                    .padding(.horizontal, 6)
                            }
                        }
                        .padding(.vertical, 6)
                        .frame(maxWidth: .infinity, alignment: .leading)
                    }
                    .frame(height: boxHeight)
                    .background(Color(uiColor: .systemBackground).opacity(0.85))
                    .overlay(
                        RoundedRectangle(cornerRadius: 8, style: .continuous)
                            .stroke(Color(uiColor: .separator).opacity(0.45), lineWidth: 0.75)
                    )
                    .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
                }
            }
        }
    }

    @ViewBuilder
    private func diffLineView(_ line: String) -> some View {
        let isAdded = line.hasPrefix("+ ")
        let isRemoved = line.hasPrefix("- ")
        let isHunk = line == "@@"
        let fg: Color = isAdded ? .green : (isRemoved ? .red : (isHunk ? Color(uiColor: .tertiaryLabel) : .secondary))
        let bg: Color = isAdded ? Color.green.opacity(0.11) : (isRemoved ? Color.red.opacity(0.11) : .clear)

        Text(line.isEmpty ? " " : line)
            .font(.system(size: 11, design: .monospaced))
            .foregroundColor(fg)
            .padding(.horizontal, 8)
            .padding(.vertical, 1)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(bg)
    }
    
    @ViewBuilder
    private func multiQuestionSection(qIdx: Int, q: InteractionQuestion) -> some View {
        let opts = displayOptions(for: q)
        let isMulti = (q.isMultiSelect == true)
        let selectedMultiSet = multiSelectedOptionsByQuestion[qIdx] ?? []
        let isQuestionWriteInActive = writeInActiveByQuestion[qIdx] == true
        VStack(alignment: .leading, spacing: 7) {
            HStack(alignment: .top, spacing: 6) {
                Text("Q\(qIdx + 1)")
                    .font(.system(size: 10.5, weight: .bold, design: .monospaced))
                    .foregroundColor(.white)
                    .padding(.horizontal, 5)
                    .padding(.vertical, 2)
                    .background(headerIconColor)
                    .clipShape(RoundedRectangle(cornerRadius: 4, style: .continuous))
                
                Text(q.question)
                    .font(.system(size: 12.5, weight: .semibold))
                    .foregroundColor(.primary)
                    .lineSpacing(2)
                    .multilineTextAlignment(.leading)
                    .fixedSize(horizontal: false, vertical: true)
                    .frame(maxWidth: .infinity, alignment: .leading)

                if isMulti {
                    multiSelectBadgeView
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .contentShape(Rectangle())
            .onTapGesture {
                dismissKeyboard()
            }
            
            VStack(spacing: 6) {
                ForEach(Array(opts.enumerated()), id: \.element.id) { optIdx, option in
                    let isWriteIn = isOptionWriteIn(option, hasWriteIn: q.hasWriteIn == true)
                    let isSelected: Bool = {
                        if isMulti {
                            return isWriteIn ? isQuestionWriteInActive : selectedMultiSet.contains(option.id)
                        }
                        return (selectedOptionsByQuestion[qIdx] ?? q.defaultOptionId ?? "1") == option.id
                    }()
                    let indicatorName = isMulti
                        ? (isSelected ? "checkmark.square.fill" : "square")
                        : (isSelected ? "largecircle.fill.circle" : "circle")
                    let toggleWriteInOff = {
                        UISelectionFeedbackGenerator().selectionChanged()
                        withAnimation(.easeInOut(duration: 0.15)) {
                            writeInActiveByQuestion[qIdx] = false
                        }
                        dismissKeyboard()
                    }
                    
                    HStack(alignment: .top, spacing: 8) {
                        Text(badgeText(for: option, in: opts, index: optIdx))
                            .font(.system(size: 11, weight: .bold, design: .monospaced))
                            .foregroundColor(isSelected ? .white : .secondary)
                            .frame(width: 20, height: 20)
                            .background(isSelected ? Color.blue : Color(uiColor: .tertiarySystemFill))
                            .clipShape(RoundedRectangle(cornerRadius: 5, style: .continuous))
                            .padding(.top, 1)
                            .onTapGesture {
                                if isMulti && isWriteIn && isSelected {
                                    toggleWriteInOff()
                                }
                            }
                        
                        if isWriteIn && isSelected {
                            let binding = Binding<String>(
                                get: { writeInByQuestion[qIdx] ?? "" },
                                set: { writeInByQuestion[qIdx] = $0 }
                            )
                            TextField(
                                resolveWriteInPlaceholder(q.writeInPlaceholder, isDeny: false),
                                text: binding,
                                axis: .vertical
                            )
                            .font(.system(size: 12.5, weight: .regular))
                            .foregroundColor(.primary)
                            .frame(maxWidth: .infinity, alignment: .leading)
                            .focused($focusedMultiQuestionIndex, equals: qIdx)
                            .onAppear {
                                if pendingAutoFocusQuestionIndex == qIdx {
                                    pendingAutoFocusQuestionIndex = nil
                                    focusedMultiQuestionIndex = qIdx
                                }
                            }
                        } else {
                            let currentText = writeInByQuestion[qIdx] ?? ""
                            Text(isWriteIn && !currentText.isEmpty ? currentText : option.text)
                                .font(.system(size: 12.5, weight: isSelected ? .medium : .regular))
                                .foregroundColor(isSelected ? .primary : .secondary)
                                .lineSpacing(2)
                                .multilineTextAlignment(.leading)
                                .frame(maxWidth: .infinity, alignment: .leading)
                        }
                        
                        Image(systemName: indicatorName)
                            .font(.system(size: isMulti ? 15.5 : 14))
                            .foregroundColor(isSelected ? .blue : Color(uiColor: .tertiaryLabel))
                            .padding(.top, isMulti ? 1.5 : 2)
                            .contentShape(Rectangle())
                            .onTapGesture {
                                if isMulti && isWriteIn && isSelected {
                                    toggleWriteInOff()
                                }
                            }
                    }
                    .padding(.horizontal, 9)
                    .padding(.vertical, 8)
                    .background(
                        RoundedRectangle(cornerRadius: 8, style: .continuous)
                            .fill(isSelected ? Color.blue.opacity(0.08) : Color(uiColor: .secondarySystemBackground))
                    )
                    .overlay(
                        RoundedRectangle(cornerRadius: 8, style: .continuous)
                            .stroke(isSelected ? Color.blue.opacity(0.4) : Color.clear, lineWidth: 1)
                    )
                    .contentShape(Rectangle())
                    .onTapGesture {
                        if isMulti && isWriteIn && isSelected {
                            focusedMultiQuestionIndex = qIdx
                            return
                        }
                        UISelectionFeedbackGenerator().selectionChanged()
                        if isMulti {
                            withAnimation(.easeInOut(duration: 0.15)) {
                                if isWriteIn {
                                    writeInActiveByQuestion[qIdx] = true
                                } else {
                                    var set = multiSelectedOptionsByQuestion[qIdx] ?? []
                                    if set.contains(option.id) {
                                        set.remove(option.id)
                                    } else {
                                        set.insert(option.id)
                                    }
                                    multiSelectedOptionsByQuestion[qIdx] = set
                                }
                            }
                            if isWriteIn {
                                pendingAutoFocusQuestionIndex = qIdx
                                focusedMultiQuestionIndex = qIdx
                            } else {
                                dismissKeyboard()
                            }
                        } else {
                            let wasAlreadySelected = isSelected
                            withAnimation(.easeInOut(duration: 0.15)) {
                                selectedOptionsByQuestion[qIdx] = option.id
                            }
                            if isWriteIn {
                                if !wasAlreadySelected {
                                    pendingAutoFocusQuestionIndex = qIdx
                                }
                                focusedMultiQuestionIndex = qIdx
                            } else {
                                dismissKeyboard()
                            }
                        }
                    }
                }
            }
        }
        .padding(.bottom, 4)
    }
    
    @ViewBuilder
    private var singleQuestionOptions: some View {
        let opts = singleDisplayOptions
        let isMulti = isSingleMultiSelect
        VStack(spacing: 6) {
            ForEach(Array(opts.enumerated()), id: \.element.id) { optIdx, option in
                let isWriteIn = isOptionWriteIn(option, hasWriteIn: interaction.hasWriteIn == true)
                let isSelected: Bool = {
                    if isMulti {
                        return isWriteIn ? singleWriteInActive : singleMultiSelectedIds.contains(option.id)
                    }
                    return selectedOptionId == option.id
                }()
                let indicatorName = isMulti
                    ? (isSelected ? "checkmark.square.fill" : "square")
                    : (isSelected ? "largecircle.fill.circle" : "circle")
                let toggleSingleWriteInOff = {
                    UISelectionFeedbackGenerator().selectionChanged()
                    withAnimation(.easeInOut(duration: 0.15)) {
                        singleWriteInActive = false
                    }
                    dismissKeyboard()
                }
                
                HStack(alignment: .top, spacing: 9) {
                    Text(badgeText(for: option, in: opts, index: optIdx))
                        .font(.system(size: 11, weight: .bold, design: .monospaced))
                        .foregroundColor(isSelected ? .white : .secondary)
                        .frame(width: 20, height: 20)
                        .background(isSelected ? Color.blue : Color(uiColor: .tertiarySystemFill))
                        .clipShape(RoundedRectangle(cornerRadius: 5, style: .continuous))
                        .padding(.top, 1)
                        .onTapGesture {
                            if isMulti && isWriteIn && isSelected {
                                toggleSingleWriteInOff()
                            }
                        }
                    
                    if isWriteIn && isSelected {
                        TextField(
                            resolveWriteInPlaceholder(interaction.writeInPlaceholder, isDeny: isDenySelected),
                            text: $writeInText,
                            axis: .vertical
                        )
                        .font(.system(size: 13, weight: .regular))
                        .foregroundColor(.primary)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .focused($isSingleWriteInFocused)
                        .onAppear {
                            if pendingAutoFocusSingleWriteIn {
                                pendingAutoFocusSingleWriteIn = false
                                isSingleWriteInFocused = true
                            }
                        }
                    } else {
                        Text(isWriteIn && !writeInText.isEmpty ? writeInText : option.text)
                            .font(.system(size: 13, weight: isSelected ? .medium : .regular))
                            .foregroundColor(isSelected ? .primary : .secondary)
                            .lineSpacing(2)
                            .multilineTextAlignment(.leading)
                            .frame(maxWidth: .infinity, alignment: .leading)
                    }
                    
                    Image(systemName: indicatorName)
                        .font(.system(size: isMulti ? 16 : 15))
                        .foregroundColor(isSelected ? .blue : Color(uiColor: .tertiaryLabel))
                        .padding(.top, isMulti ? 1.5 : 2)
                        .contentShape(Rectangle())
                        .onTapGesture {
                            if isMulti && isWriteIn && isSelected {
                                toggleSingleWriteInOff()
                            }
                        }
                }
                .padding(.horizontal, 10)
                .padding(.vertical, 8)
                .background(
                    RoundedRectangle(cornerRadius: 9, style: .continuous)
                        .fill(isSelected ? Color.blue.opacity(0.08) : Color(uiColor: .secondarySystemBackground))
                )
                .overlay(
                    RoundedRectangle(cornerRadius: 9, style: .continuous)
                        .stroke(isSelected ? Color.blue.opacity(0.4) : Color.clear, lineWidth: 1)
                )
                .contentShape(Rectangle())
                .onTapGesture {
                    if isMulti && isWriteIn && isSelected {
                        isSingleWriteInFocused = true
                        return
                    }
                    UISelectionFeedbackGenerator().selectionChanged()
                    if isMulti {
                        withAnimation(.easeInOut(duration: 0.15)) {
                            if isWriteIn {
                                singleWriteInActive = true
                            } else if singleMultiSelectedIds.contains(option.id) {
                                singleMultiSelectedIds.remove(option.id)
                            } else {
                                singleMultiSelectedIds.insert(option.id)
                            }
                        }
                        if isWriteIn {
                            pendingAutoFocusSingleWriteIn = true
                            isSingleWriteInFocused = true
                        } else {
                            dismissKeyboard()
                        }
                    } else {
                        let wasAlreadySelected = isSelected
                        withAnimation(.easeInOut(duration: 0.15)) {
                            selectedOptionId = option.id
                        }
                        if isWriteIn {
                            if !wasAlreadySelected {
                                pendingAutoFocusSingleWriteIn = true
                            }
                            isSingleWriteInFocused = true
                        } else {
                            dismissKeyboard()
                        }
                    }
                }
            }
        }
    }
    
    @ViewBuilder
    private var bottomActionBar: some View {
        VStack(spacing: 8) {
            HStack(spacing: 10) {
                Button(action: handleSkip) {
                    Text("跳过")
                        .font(.system(size: 14, weight: .medium))
                        .foregroundColor(.secondary)
                        .frame(maxWidth: .infinity)
                        .padding(.vertical, 8)
                        .background(Color(uiColor: .tertiarySystemFill))
                        .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
                }
                .buttonStyle(.plain)
                .disabled(isSubmitting)
                
                Button(action: handleSubmit) {
                    HStack(spacing: 5) {
                        if isSubmitting {
                            ProgressView()
                                .tint(.white)
                                .scaleEffect(0.8)
                        } else {
                            Text(submitButtonTitle)
                                .font(.system(size: 14, weight: .semibold))
                                .foregroundColor(.white)
                        }
                    }
                    .frame(maxWidth: .infinity)
                    .padding(.vertical, 8)
                    .background(canSubmit ? Color.blue : Color.blue.opacity(0.38))
                    .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
                    .shadow(color: canSubmit ? Color.blue.opacity(0.3) : .clear, radius: 3, x: 0, y: 1.5)
                }
                .buttonStyle(.plain)
                .disabled(isSubmitting || !canSubmit)
            }
        }
        .padding(.top, 2)
    }
    
    public var body: some View {
        if interaction.type == "workflow_approval" || interaction.workflowApproval != nil {
            WorkflowApprovalCardView(
                interaction: interaction,
                isSubmitting: isSubmitting,
                onSubmit: { optionId, text in
                    onSubmit(optionId, text, interaction.target, nil)
                }
            )
            .padding(.horizontal, 12)
            .padding(.bottom, 6)
        } else {
            let showBottomDivider = exceedsCollapsedHeight || (isExpanded && scrollContentHeight > maxScrollExpandedHeight + 2)
            VStack(alignment: .leading, spacing: 8) {
                headerView
                
                if isLongSingleQuestion {
                    Divider().opacity(0.45)
                }
                
                ScrollView(.vertical, showsIndicators: exceedsCollapsedHeight || isExpanded) {
                    VStack(alignment: .leading, spacing: 10) {
                        if isLongSingleQuestion {
                            Text(singleQuestionTitle)
                                .font(.system(size: 13.5, weight: .medium))
                                .foregroundColor(.primary)
                                .lineSpacing(3)
                                .multilineTextAlignment(.leading)
                                .fixedSize(horizontal: false, vertical: true)
                                .textSelection(.enabled)
                                .frame(maxWidth: .infinity, alignment: .leading)
                        }

                        riskSummaryBannerView

                        targetBoxView

                        compactDiffPreviewView

                        if hasMultipleQuestions, let questions = interaction.questions {
                            VStack(alignment: .leading, spacing: 14) {
                                ForEach(Array(questions.enumerated()), id: \.offset) { qIdx, q in
                                    multiQuestionSection(qIdx: qIdx, q: q)
                                }
                            }
                        } else {
                            singleQuestionOptions
                        }
                    }
                    .padding(.vertical, 2)
                    .background(
                        GeometryReader { geo in
                            Color.clear.preference(key: OptionsHeightKey.self, value: geo.size.height)
                        }
                    )
                }
                .scrollDismissesKeyboard(.immediately)
                .frame(height: targetScrollHeight)
                
                if showBottomDivider {
                    Divider().opacity(0.45)
                }
                
                bottomActionBar
            }
            .frame(maxWidth: .infinity, alignment: .topLeading)
            .padding(.horizontal, 16)
            .padding(.vertical, 12)
            .background(Color(uiColor: .secondarySystemGroupedBackground))
            .clipShape(RoundedRectangle(cornerRadius: 16, style: .continuous))
            .overlay(
                RoundedRectangle(cornerRadius: 16, style: .continuous)
                    .stroke(headerIconColor.opacity(0.35), lineWidth: 1)
            )
            .shadow(color: Color.black.opacity(0.08), radius: 6, x: 0, y: 2)
            .padding(.horizontal, 12)
            .padding(.bottom, 6)
            .animation(.spring(response: 0.35, dampingFraction: 0.82), value: isExpanded)
            .onPreferenceChange(OptionsHeightKey.self) { height in
                if height > 0, abs(self.scrollContentHeight - height) > 0.5 {
                    self.scrollContentHeight = height
                }
            }
            .onPreferenceChange(HeaderHeightKey.self) { height in
                if height > 0, abs(self.headerHeight - height) > 0.5 {
                    self.headerHeight = height
                }
            }
            .onChange(of: focusedMultiQuestionIndex) { _, newFocus in
                if newFocus == nil {
                    pendingAutoFocusQuestionIndex = nil
                }
            }
            .onChange(of: isSingleWriteInFocused) { _, newFocus in
                if !newFocus {
                    pendingAutoFocusSingleWriteIn = false
                }
            }
            .onChange(of: interaction.id) { _, _ in
                isDiffExpanded = false
            }
        }
    }
}
