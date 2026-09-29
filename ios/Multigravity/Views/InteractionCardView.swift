import SwiftUI

public struct InteractionCardView: View {
    public let interaction: PendingInteraction
    public let isSubmitting: Bool
    public let onSubmit: (String, String?, String?, [QuestionResponse]?) -> Void
    public let onSkip: ([QuestionResponse]?) -> Void
    
    @State private var selectedOptionId: String
    @State private var writeInText: String = ""
    @State private var targetText: String
    @State private var settings = AppSettings.shared
    
    // Multi-question state: questionIndex -> selectedOptionId, questionIndex -> writeInText
    @State private var selectedOptionsByQuestion: [Int: String] = [:]
    @State private var writeInByQuestion: [Int: String] = [:]
    
    // Expandable sheet state (default: half screen, draggable to full screen)
    @State private var isFullScreen: Bool = false
    @State private var dragOffset: CGFloat = 0
    
    private var isPermissionType: Bool {
        interaction.type == "permission" || interaction.type == "file_permission"
    }
    
    private var hasMultipleQuestions: Bool {
        if let questions = interaction.questions, questions.count > 1 {
            return true
        }
        return false
    }
    
    public init(
        interaction: PendingInteraction,
        isSubmitting: Bool,
        onSubmit: @escaping (String, String?, String?, [QuestionResponse]?) -> Void,
        onSkip: @escaping ([QuestionResponse]?) -> Void
    ) {
        self.interaction = interaction
        self.isSubmitting = isSubmitting
        self.onSubmit = onSubmit
        self.onSkip = onSkip
        
        var initialSelectedId = "1"
        if AppSettings.shared.autoApprovePermissions && (interaction.type == "permission" || interaction.type == "file_permission") {
            if let opt4 = interaction.options.first(where: { $0.id == "4" || $0.scope == 4 || $0.text.localizedCaseInsensitiveContains("always allow") }) {
                initialSelectedId = opt4.id
            } else {
                initialSelectedId = interaction.defaultOptionId ?? interaction.options.first?.id ?? "1"
            }
        } else {
            initialSelectedId = interaction.defaultOptionId ?? interaction.options.first?.id ?? "1"
        }
        _selectedOptionId = State(initialValue: initialSelectedId)
        _targetText = State(initialValue: interaction.target ?? "")
        
        var qMap: [Int: String] = [:]
        if let questions = interaction.questions {
            for (idx, q) in questions.enumerated() {
                qMap[idx] = q.defaultOptionId ?? q.options.first?.id ?? "1"
            }
        }
        _selectedOptionsByQuestion = State(initialValue: qMap)
    }
    
    private var headerIconName: String {
        switch interaction.type {
        case "permission", "file_permission":
            return "hand.raised.fill"
        case "ask_question":
            return "questionmark.bubble.fill"
        case "run_command":
            return "terminal.fill"
        default:
            return "questionmark.circle.fill"
        }
    }
    
    private var headerIconColor: Color {
        switch interaction.type {
        case "permission", "file_permission":
            return .blue
        case "ask_question":
            return .purple
        case "run_command":
            return .orange
        default:
            return .blue
        }
    }
    
    private var isDenySelected: Bool {
        if let opt = interaction.options.first(where: { $0.id == selectedOptionId }) {
            return opt.isDeny == true || opt.id == "5" || opt.id == "__write_in__"
        }
        return selectedOptionId == "5" || selectedOptionId == "__write_in__"
    }
    
    private func isQuestionDenySelected(qIdx: Int, q: InteractionQuestion) -> Bool {
        let optId = selectedOptionsByQuestion[qIdx] ?? ""
        if let opt = q.options.first(where: { $0.id == optId }) {
            return opt.isDeny == true || opt.id == "5" || opt.id == "__write_in__" || opt.id.lowercased() == "other"
        }
        return optId == "5" || optId == "__write_in__" || optId.lowercased() == "other"
    }
    
    private var dragGesture: some Gesture {
        DragGesture()
            .onChanged { value in
                dragOffset = value.translation.height
            }
            .onEnded { value in
                let verticalMove = value.translation.height
                withAnimation(.spring(response: 0.35, dampingFraction: 0.8)) {
                    if isFullScreen {
                        if verticalMove > 50 {
                            isFullScreen = false
                        }
                    } else {
                        if verticalMove < -50 {
                            isFullScreen = true
                        }
                    }
                    dragOffset = 0
                }
            }
    }
    
    private func toggleAutoApprove() {
        UIImpactFeedbackGenerator(style: .light).impactOccurred()
        withAnimation(.easeInOut(duration: 0.2)) {
            settings.autoApprovePermissions.toggle()
            if settings.autoApprovePermissions {
                let opt = interaction.options.first { opt in
                    opt.id == "4" || opt.scope == 4 || opt.text.localizedCaseInsensitiveContains("always allow")
                }
                if let opt = opt {
                    selectedOptionId = opt.id
                }
            }
        }
    }
    
    private func handleSkip() {
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
        if hasMultipleQuestions, let questions = interaction.questions {
            let responses = questions.enumerated().map { idx, q in
                let optId = selectedOptionsByQuestion[idx] ?? q.defaultOptionId ?? q.options.first?.id ?? "1"
                let text = writeInByQuestion[idx]
                return QuestionResponse(questionIndex: idx, selectedOptionIds: [optId], writeInResponse: text, skipped: false)
            }
            onSubmit(selectedOptionId, nil, interaction.target, responses)
        } else {
            let text = isDenySelected ? writeInText : nil
            onSubmit(selectedOptionId, text, interaction.target, nil)
        }
    }
    
    @ViewBuilder
    private var dragHandleView: some View {
        VStack(spacing: 0) {
            Capsule()
                .fill(Color.secondary.opacity(0.35))
                .frame(width: 38, height: 4.5)
                .padding(.top, 8)
                .padding(.bottom, 4)
        }
        .frame(maxWidth: .infinity)
        .contentShape(Rectangle())
        .onTapGesture {
            UISelectionFeedbackGenerator().selectionChanged()
            withAnimation(.spring(response: 0.35, dampingFraction: 0.8)) {
                isFullScreen.toggle()
            }
        }
        .gesture(dragGesture)
    }
    
    @ViewBuilder
    private var headerView: some View {
        HStack(alignment: .center, spacing: 8) {
            Image(systemName: headerIconName)
                .font(.system(size: 15, weight: .semibold))
                .foregroundColor(headerIconColor)
            
            if hasMultipleQuestions, let count = interaction.questions?.count {
                Text("需要确认规格 (\(count) 个问题)")
                    .font(.system(size: 14.5, weight: .semibold))
                    .foregroundColor(.primary)
            } else {
                Text(interaction.title.isEmpty ? "需要用户审批操作" : interaction.title)
                    .font(.system(size: 14.5, weight: .semibold))
                    .foregroundColor(.primary)
                    .fixedSize(horizontal: false, vertical: true)
            }
            
            Spacer()
            
            Button {
                UISelectionFeedbackGenerator().selectionChanged()
                withAnimation(.spring(response: 0.35, dampingFraction: 0.8)) {
                    isFullScreen.toggle()
                }
            } label: {
                Image(systemName: isFullScreen ? "chevron.down" : "chevron.up")
                    .font(.system(size: 13, weight: .semibold))
                    .foregroundColor(.secondary)
                    .frame(width: 28, height: 28)
                    .background(Color(uiColor: .tertiarySystemFill))
                    .clipShape(Circle())
            }
            .buttonStyle(.plain)
        }
        .padding(.horizontal, 14)
        .gesture(dragGesture)
    }
    
    @ViewBuilder
    private var targetBoxView: some View {
        if let target = interaction.target, !target.isEmpty {
            VStack(alignment: .leading, spacing: 4) {
                Text(target)
                    .font(.system(size: 11.5, design: .monospaced))
                    .foregroundColor(.secondary)
                    .lineLimit(3)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .padding(.horizontal, 10)
                    .padding(.vertical, 7)
                    .background(Color(uiColor: .tertiarySystemFill))
                    .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
            }
            .padding(.horizontal, 14)
        }
    }
    
    @ViewBuilder
    private func multiQuestionSection(qIdx: Int, q: InteractionQuestion) -> some View {
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
                    .fixedSize(horizontal: false, vertical: true)
            }
            
            VStack(spacing: 6) {
                ForEach(q.options) { option in
                    let isSelected = (selectedOptionsByQuestion[qIdx] ?? q.defaultOptionId ?? "1") == option.id
                    
                    HStack(alignment: .top, spacing: 8) {
                        Text(option.id)
                            .font(.system(size: 11, weight: .bold, design: .monospaced))
                            .foregroundColor(isSelected ? .white : .secondary)
                            .frame(width: 20, height: 20)
                            .background(isSelected ? Color.blue : Color(uiColor: .tertiarySystemFill))
                            .clipShape(RoundedRectangle(cornerRadius: 5, style: .continuous))
                            .padding(.top, 1)
                        
                        Text(option.text)
                            .font(.system(size: 12.5, weight: isSelected ? .medium : .regular))
                            .foregroundColor(isSelected ? .primary : .secondary)
                            .lineSpacing(2)
                            .multilineTextAlignment(.leading)
                            .frame(maxWidth: .infinity, alignment: .leading)
                        
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
                        withAnimation(.easeInOut(duration: 0.15)) {
                            selectedOptionsByQuestion[qIdx] = option.id
                        }
                    }
                }
            }
            
            if q.hasWriteIn == true && isQuestionDenySelected(qIdx: qIdx, q: q) {
                let binding = Binding<String>(
                    get: { writeInByQuestion[qIdx] ?? "" },
                    set: { writeInByQuestion[qIdx] = $0 }
                )
                TextField(
                    q.writeInPlaceholder ?? "(输入自定义说明)",
                    text: binding
                )
                .font(.system(size: 12))
                .padding(.horizontal, 8)
                .padding(.vertical, 6)
                .background(Color(uiColor: .tertiarySystemFill))
                .clipShape(RoundedRectangle(cornerRadius: 7, style: .continuous))
            }
        }
        .padding(.bottom, 4)
    }
    
    @ViewBuilder
    private var singleQuestionOptions: some View {
        VStack(spacing: 6) {
            ForEach(interaction.options) { option in
                let isSelected = selectedOptionId == option.id
                
                HStack(alignment: .top, spacing: 9) {
                    Text(option.id)
                        .font(.system(size: 11, weight: .bold, design: .monospaced))
                        .foregroundColor(isSelected ? .white : .secondary)
                        .frame(width: 20, height: 20)
                        .background(isSelected ? Color.blue : Color(uiColor: .tertiarySystemFill))
                        .clipShape(RoundedRectangle(cornerRadius: 5, style: .continuous))
                        .padding(.top, 1)
                    
                    Text(option.text)
                        .font(.system(size: 13, weight: isSelected ? .medium : .regular))
                        .foregroundColor(isSelected ? .primary : .secondary)
                        .lineSpacing(2)
                        .multilineTextAlignment(.leading)
                        .frame(maxWidth: .infinity, alignment: .leading)
                    
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
                    withAnimation(.easeInOut(duration: 0.15)) {
                        selectedOptionId = option.id
                    }
                }
            }
        }
        
        if interaction.hasWriteIn == true && isDenySelected {
            VStack(alignment: .leading, spacing: 4) {
                TextField(
                    interaction.writeInPlaceholder ?? "(告诉 Agent 应该怎么做)",
                    text: $writeInText,
                    axis: .vertical
                )
                .font(.system(size: 12.5))
                .padding(.horizontal, 10)
                .padding(.vertical, 7)
                .background(Color(uiColor: .tertiarySystemFill))
                .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
                .overlay(
                    RoundedRectangle(cornerRadius: 8, style: .continuous)
                        .stroke(Color.secondary.opacity(0.25), lineWidth: 1)
                )
            }
            .padding(.horizontal, 10)
            .transition(.opacity.combined(with: .move(edge: .top)))
        }
    }
    
    @ViewBuilder
    private func bottomActionBar(safeAreaBottom: CGFloat) -> some View {
        HStack(spacing: 8) {
            if isPermissionType {
                Button(action: toggleAutoApprove) {
                    HStack(spacing: 4) {
                        Image(systemName: settings.autoApprovePermissions ? "bolt.fill" : "bolt")
                            .font(.system(size: 11, weight: .semibold))
                            .foregroundColor(settings.autoApprovePermissions ? .yellow : .secondary)
                        Text(settings.autoApprovePermissions ? "自动审批: 开" : "自动审批: 关")
                            .font(.system(size: 11.5, weight: .medium))
                            .foregroundColor(settings.autoApprovePermissions ? .primary : .secondary)
                    }
                    .padding(.horizontal, 8)
                    .padding(.vertical, 6)
                    .background(settings.autoApprovePermissions ? Color.yellow.opacity(0.15) : Color(uiColor: .tertiarySystemFill))
                    .clipShape(RoundedRectangle(cornerRadius: 7, style: .continuous))
                }
                .buttonStyle(.plain)
            }
            
            Spacer()
            
            Button(action: handleSkip) {
                Text("Skip (跳过)")
                    .font(.system(size: 13, weight: .medium))
                    .foregroundColor(.secondary)
                    .padding(.horizontal, 14)
                    .padding(.vertical, 7)
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
                        Text("Submit ↵ (提交)")
                            .font(.system(size: 13, weight: .semibold))
                            .foregroundColor(.white)
                    }
                }
                .padding(.horizontal, 16)
                .padding(.vertical, 7)
                .background(Color.blue)
                .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
                .shadow(color: Color.blue.opacity(0.3), radius: 3, x: 0, y: 1.5)
            }
            .buttonStyle(.plain)
            .disabled(isSubmitting)
        }
        .padding(.horizontal, 14)
        .padding(.top, 4)
        .padding(.bottom, max(8, safeAreaBottom))
    }
    
    public var body: some View {
        GeometryReader { geometry in
            let fullHeight = geometry.size.height
            let halfHeight = fullHeight * 0.52
            let baseHeight = isFullScreen ? fullHeight : halfHeight
            let currentHeight = min(fullHeight, max(halfHeight, baseHeight - dragOffset))
            
            VStack(spacing: 0) {
                Spacer(minLength: 0)
                
                VStack(alignment: .leading, spacing: 8) {
                    dragHandleView
                    headerView
                    targetBoxView
                    
                    ScrollView(.vertical, showsIndicators: true) {
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
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
                    .padding(.horizontal, 14)
                    
                    bottomActionBar(safeAreaBottom: geometry.safeAreaInsets.bottom)
                }
                .frame(width: geometry.size.width, height: currentHeight)
                .background(
                    RoundedRectangle(cornerRadius: 18, style: .continuous)
                        .fill(Color(uiColor: .secondarySystemGroupedBackground))
                )
                .overlay(
                    RoundedRectangle(cornerRadius: 18, style: .continuous)
                        .stroke(headerIconColor.opacity(0.35), lineWidth: 1)
                )
                .clipShape(RoundedRectangle(cornerRadius: 18, style: .continuous))
                .shadow(color: Color.black.opacity(0.12), radius: 10, x: 0, y: -2)
            }
            .frame(width: geometry.size.width, height: geometry.size.height, alignment: .bottom)
        }
    }
}
