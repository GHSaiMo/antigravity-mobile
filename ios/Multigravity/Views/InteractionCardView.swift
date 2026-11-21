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
    
    // Multi-question state: questionIndex -> selectedOptionId, questionIndex -> writeInText
    @State private var selectedOptionsByQuestion: [Int: String] = [:]
    @State private var writeInByQuestion: [Int: String] = [:]
    
    // Expandable card state (default: adaptive height up to ~half chat area, expandable to full chat area without overlapping nav bar)
    @State private var isExpanded: Bool = false
    @State private var scrollContentHeight: CGFloat = 0
    @State private var headerHeight: CGFloat = 28
    
    private var isPermissionType: Bool {
        interaction.type == "permission" || interaction.type == "file_permission"
    }
    
    private var hasMultipleQuestions: Bool {
        if let questions = interaction.questions, questions.count > 1 {
            return true
        }
        return false
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
        return max(320, UIScreen.main.bounds.height - 220)
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
        if let questions = interaction.questions {
            for (idx, q) in questions.enumerated() {
                let qFallback = (q.hasWriteIn == true) ? "__write_in__" : "1"
                qMap[idx] = q.defaultOptionId ?? q.options.first?.id ?? qFallback
            }
        }
        _selectedOptionsByQuestion = State(initialValue: qMap)
    }
    
    private var headerIconName: String {
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
        if selectedOptionId == "__write_in__" { return true }
        guard interaction.hasWriteIn == true else { return false }
        if let opt = singleDisplayOptions.first(where: { $0.id == selectedOptionId }) {
            return opt.isDeny == true || opt.id == "5" || opt.id.lowercased() == "other"
        }
        return selectedOptionId == "5"
    }
    
    private func isQuestionDenySelected(qIdx: Int, q: InteractionQuestion) -> Bool {
        let optId = selectedOptionsByQuestion[qIdx] ?? ""
        let opts = displayOptions(for: q)
        if let opt = opts.first(where: { $0.id == optId }) {
            return opt.isDeny == true || opt.id == "5" || opt.id == "__write_in__" || opt.id.lowercased() == "other"
        }
        return optId == "5" || optId == "__write_in__" || optId.lowercased() == "other"
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
        dismissKeyboard()
        if hasMultipleQuestions, let questions = interaction.questions {
            let responses = questions.enumerated().map { idx, q in
                let qFallback = (q.hasWriteIn == true) ? "__write_in__" : "1"
                let optId = selectedOptionsByQuestion[idx] ?? q.defaultOptionId ?? q.options.first?.id ?? qFallback
                let text = writeInByQuestion[idx]
                return QuestionResponse(questionIndex: idx, selectedOptionIds: [optId], writeInResponse: text, skipped: false)
            }
            onSubmit(selectedOptionId, nil, interaction.target, responses)
        } else {
            let text = (isWriteInSelected || isDenySelected) ? writeInText : nil
            onSubmit(selectedOptionId, text, interaction.target, nil)
        }
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
                Text(compactCategoryTitle)
                    .font(.system(size: 14.5, weight: .semibold))
                    .foregroundColor(.primary)
                    .lineLimit(1)
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
    private var targetBoxView: some View {
        if let pg = interaction.permissionGrant {
            VStack(alignment: .leading, spacing: 6) {
                HStack(spacing: 6) {
                    Text(pg.binary.uppercased())
                        .font(.system(size: 10, weight: .bold))
                        .foregroundColor(.orange)
                        .padding(.horizontal, 6)
                        .padding(.vertical, 2)
                        .background(Color.orange.opacity(0.18))
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
        } else if let target = interaction.target, !target.isEmpty {
            VStack(alignment: .leading, spacing: 4) {
                Text(target)
                    .font(.system(size: 11.5, design: .monospaced))
                    .foregroundColor(.secondary)
                    .lineLimit(isExpanded ? 8 : 3)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .padding(.horizontal, 10)
                    .padding(.vertical, 7)
                    .background(Color(uiColor: .tertiarySystemFill))
                    .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
            }
        }
    }
    
    @ViewBuilder
    private func multiQuestionSection(qIdx: Int, q: InteractionQuestion) -> some View {
        let opts = displayOptions(for: q)
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
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .contentShape(Rectangle())
            .onTapGesture {
                dismissKeyboard()
            }
            
            VStack(spacing: 6) {
                ForEach(Array(opts.enumerated()), id: \.element.id) { optIdx, option in
                    let isSelected = (selectedOptionsByQuestion[qIdx] ?? q.defaultOptionId ?? "1") == option.id
                    let isWriteIn = (option.id == "__write_in__" || (q.hasWriteIn == true && (option.isDeny == true || option.id == "5" || option.id.lowercased() == "other")))
                    
                    HStack(alignment: .top, spacing: 8) {
                        Text(badgeText(for: option, in: opts, index: optIdx))
                            .font(.system(size: 11, weight: .bold, design: .monospaced))
                            .foregroundColor(isSelected ? .white : .secondary)
                            .frame(width: 20, height: 20)
                            .background(isSelected ? Color.blue : Color(uiColor: .tertiarySystemFill))
                            .clipShape(RoundedRectangle(cornerRadius: 5, style: .continuous))
                            .padding(.top, 1)
                        
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
                        
                        Image(systemName: isSelected ? "largecircle.fill.circle" : "circle")
                            .font(.system(size: 14))
                            .foregroundColor(isSelected ? .blue : Color(uiColor: .tertiaryLabel))
                            .padding(.top, 2)
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
                        UISelectionFeedbackGenerator().selectionChanged()
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
        .padding(.bottom, 4)
    }
    
    @ViewBuilder
    private var singleQuestionOptions: some View {
        let opts = singleDisplayOptions
        VStack(spacing: 6) {
            ForEach(Array(opts.enumerated()), id: \.element.id) { optIdx, option in
                let isSelected = selectedOptionId == option.id
                let isWriteIn = (option.id == "__write_in__" || (interaction.hasWriteIn == true && (option.isDeny == true || option.id == "5")))
                
                HStack(alignment: .top, spacing: 9) {
                    Text(badgeText(for: option, in: opts, index: optIdx))
                        .font(.system(size: 11, weight: .bold, design: .monospaced))
                        .foregroundColor(isSelected ? .white : .secondary)
                        .frame(width: 20, height: 20)
                        .background(isSelected ? Color.blue : Color(uiColor: .tertiarySystemFill))
                        .clipShape(RoundedRectangle(cornerRadius: 5, style: .continuous))
                        .padding(.top, 1)
                    
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
                    
                    Image(systemName: isSelected ? "largecircle.fill.circle" : "circle")
                        .font(.system(size: 15))
                        .foregroundColor(isSelected ? .blue : Color(uiColor: .tertiaryLabel))
                        .padding(.top, 2)
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
                    UISelectionFeedbackGenerator().selectionChanged()
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
                            Text("提交")
                                .font(.system(size: 14, weight: .semibold))
                                .foregroundColor(.white)
                        }
                    }
                    .frame(maxWidth: .infinity)
                    .padding(.vertical, 8)
                    .background(Color.blue)
                    .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
                    .shadow(color: Color.blue.opacity(0.3), radius: 3, x: 0, y: 1.5)
                }
                .buttonStyle(.plain)
                .disabled(isSubmitting)
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

                        targetBoxView

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
        }
    }
}
