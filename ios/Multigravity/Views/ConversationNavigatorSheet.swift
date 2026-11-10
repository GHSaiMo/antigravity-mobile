import SwiftUI

/// 对话导轨与大纲导航抽屉（对齐 Antigravity 2.28.1 官方时间线导轨规范）
public struct ConversationNavigatorSheet: View {
    @Environment(\.dismiss) private var dismiss
    public let turns: [ConversationTurn]
    public let chapters: [ConversationChapter]
    public let currentTurnIndex: Int?
    public let isLoadingChapterTitles: Bool
    public let onSelectTurn: (ConversationTurn) -> Void
    public let onJumpToTop: () -> Void
    public let onJumpToBottom: () -> Void
    
    @State private var collapsedChapterIds: Set<String> = []
    @State private var selectedChapterId: String? = nil
    
    private struct ScrollTarget: Equatable {
        let id: String
        let anchor: UnitPoint
        let token: UUID = UUID()
    }
    @State private var scrollTarget: ScrollTarget? = nil
    
    private var allCollapsed: Bool {
        guard chapters.count > 1 else { return false }
        return chapters.allSatisfy { collapsedChapterIds.contains($0.id) }
    }
    
    private var activeChapterId: String? {
        if let selected = selectedChapterId, chapters.contains(where: { $0.id == selected }) {
            return selected
        }
        if let cur = currentTurnIndex,
           let matched = chapters.first(where: { ch in ch.turns.contains(where: { $0.turnIndex == cur }) }) {
            return matched.id
        }
        return chapters.first?.id
    }
    
    public init(
        turns: [ConversationTurn],
        chapters: [ConversationChapter],
        currentTurnIndex: Int? = nil,
        isLoadingChapterTitles: Bool = false,
        onSelectTurn: @escaping (ConversationTurn) -> Void,
        onJumpToTop: @escaping () -> Void,
        onJumpToBottom: @escaping () -> Void
    ) {
        self.turns = turns
        self.chapters = chapters
        self.currentTurnIndex = currentTurnIndex
        self.isLoadingChapterTitles = isLoadingChapterTitles
        self.onSelectTurn = onSelectTurn
        self.onJumpToTop = onJumpToTop
        self.onJumpToBottom = onJumpToBottom
    }
    
    public var body: some View {
        NavigationStack {
            VStack(alignment: .leading, spacing: 12) {
                VStack(alignment: .leading, spacing: 12) {
                    headerSummaryBanner
                    
                    if chapters.count > 1 {
                        chapterJumpBar
                    } else if let first = chapters.first, first.isAiTitled {
                        singleChapterAIBanner(first)
                    }
                }
                .padding(.horizontal, 16)
                .padding(.top, 14)
                
                ScrollView(.vertical, showsIndicators: true) {
                    ScrollViewReader { verticalScrollProxy in
                        VStack(alignment: .leading, spacing: 18) {
                            if chapters.count > 1 {
                                // 多章节呈现
                                ForEach(chapters) { chapter in
                                    chapterSection(chapter: chapter)
                                        .id(chapter.id)
                                }
                            } else {
                                // 单章节平铺呈现
                                turnsTimelineList(turns: turns)
                            }
                        }
                        .padding(.horizontal, 16)
                        .padding(.top, 4)
                        .padding(.bottom, 36)
                        .onChange(of: scrollTarget) { _, target in
                            guard let target = target else { return }
                            withAnimation(.easeInOut(duration: 0.28)) {
                                verticalScrollProxy.scrollTo(target.id, anchor: target.anchor)
                            }
                            DispatchQueue.main.asyncAfter(deadline: .now() + 0.08) {
                                withAnimation(.easeOut(duration: 0.18)) {
                                    verticalScrollProxy.scrollTo(target.id, anchor: target.anchor)
                                }
                            }
                        }
                        .onAppear {
                            if selectedChapterId == nil {
                                selectedChapterId = activeChapterId
                            }
                            if let cur = currentTurnIndex, let target = turns.first(where: { $0.turnIndex == cur }) {
                                DispatchQueue.main.asyncAfter(deadline: .now() + 0.08) {
                                    verticalScrollProxy.scrollTo(target.id, anchor: .center)
                                }
                                DispatchQueue.main.asyncAfter(deadline: .now() + 0.22) {
                                    withAnimation(.easeOut(duration: 0.2)) {
                                        verticalScrollProxy.scrollTo(target.id, anchor: .center)
                                    }
                                }
                            }
                        }
                    }
                }
            }
            .background(Color(uiColor: .systemGroupedBackground))
            .navigationTitle("对话大纲与步骤导轨")
            .navigationBarTitleDisplayMode(.inline)
        }
        .presentationDetents([.large])
        .presentationDragIndicator(.visible)
    }
    
    // MARK: - Summary Banner
    
    @ViewBuilder
    private var headerSummaryBanner: some View {
        HStack(spacing: 12) {
            Image(systemName: "point.filled.topleft.down.curvedto.point.bottomright.up")
                .font(.system(size: 20))
                .foregroundColor(.indigo)
                .frame(width: 32, height: 32)
                .background(Color.indigo.opacity(0.12))
                .clipShape(Circle())
            
            VStack(alignment: .leading, spacing: 2) {
                HStack(spacing: 6) {
                    Text(chapters.count > 1 ? "共 \(chapters.count) 个章节 · \(turns.count) 轮问答" : "共 \(turns.count) 轮问答步骤")
                        .font(.system(size: 14.5, weight: .semibold))
                        .foregroundColor(.primary)
                    if isLoadingChapterTitles {
                        ProgressView()
                            .scaleEffect(0.65)
                    }
                }
                
                Text("点击任意步骤即可平滑瞬达对应提问与上下文")
                    .font(.system(size: 12))
                    .foregroundColor(.secondary)
            }
            
            Spacer()
            
            if chapters.count > 1 {
                Button(action: {
                    UIImpactFeedbackGenerator(style: .light).impactOccurred()
                    withAnimation(.easeInOut(duration: 0.25)) {
                        if allCollapsed {
                            collapsedChapterIds.removeAll()
                        } else {
                            collapsedChapterIds = Set(chapters.map(\.id))
                        }
                    }
                }) {
                    HStack(spacing: 4) {
                        Image(systemName: allCollapsed ? "chevron.down.2" : "chevron.up.2")
                            .font(.system(size: 10, weight: .semibold))
                        Text(allCollapsed ? "展开全部" : "折叠全部")
                            .font(.system(size: 12, weight: .medium))
                    }
                    .foregroundColor(.indigo)
                    .padding(.horizontal, 9)
                    .padding(.vertical, 5)
                    .background(Color.indigo.opacity(0.1))
                    .clipShape(Capsule())
                }
                .buttonStyle(.plain)
            }
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 10)
        .background(Color(uiColor: .secondarySystemGroupedBackground))
        .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
    }
    
    // MARK: - Chapter TOC Jump Bar & Single Chapter AI Banner

    @ViewBuilder
    private var chapterJumpBar: some View {
        ScrollViewReader { tabScrollProxy in
            ScrollView(.horizontal, showsIndicators: false) {
                HStack(spacing: 8) {
                    ForEach(Array(chapters.enumerated()), id: \.element.id) { idx, chapter in
                        let isSelected = (chapter.id == activeChapterId)
                        Button {
                            UIImpactFeedbackGenerator(style: .light).impactOccurred()
                            selectedChapterId = chapter.id
                            collapsedChapterIds.remove(chapter.id)
                            withAnimation(.easeInOut(duration: 0.22)) {
                                tabScrollProxy.scrollTo("tab-\(chapter.id)", anchor: .center)
                            }
                            scrollTarget = ScrollTarget(id: chapter.id, anchor: .top)
                        } label: {
                            HStack(spacing: 5) {
                                Image(systemName: chapter.isAiTitled ? "sparkles" : "bookmark.fill")
                                    .font(.system(size: 10, weight: .semibold))
                                    .foregroundColor(isSelected ? .white : .indigo)
                                Text("§\(idx + 1) \(chapter.title)")
                                    .font(.system(size: 12, weight: .medium))
                                    .foregroundColor(isSelected ? .white : .primary)
                                    .lineLimit(1)
                            }
                            .padding(.horizontal, 10)
                            .padding(.vertical, 6)
                            .frame(maxWidth: 220)
                            .background(isSelected ? Color.indigo : Color(uiColor: .secondarySystemGroupedBackground))
                            .clipShape(Capsule())
                            .overlay(
                                Capsule()
                                    .stroke(isSelected ? Color.clear : Color.indigo.opacity(0.22), lineWidth: 1)
                            )
                        }
                        .buttonStyle(.plain)
                        .id("tab-\(chapter.id)")
                    }
                }
                .padding(.vertical, 2)
            }
            .onAppear {
                if let activeId = activeChapterId {
                    tabScrollProxy.scrollTo("tab-\(activeId)", anchor: .center)
                }
            }
        }
    }

    @ViewBuilder
    private func singleChapterAIBanner(_ chapter: ConversationChapter) -> some View {
        HStack(spacing: 8) {
            Image(systemName: "sparkles")
                .font(.system(size: 12, weight: .semibold))
                .foregroundColor(.indigo)
            Text(chapter.title)
                .font(.system(size: 13, weight: .semibold))
                .foregroundColor(.primary)
                .lineLimit(2)
            Spacer()
        }
        .padding(.horizontal, 12)
        .padding(.vertical, 8)
        .background(Color.indigo.opacity(0.08))
        .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
    }

    // MARK: - Chapter Section
    
    @ViewBuilder
    private func chapterSection(chapter: ConversationChapter) -> some View {
        let isCollapsed = collapsedChapterIds.contains(chapter.id)
        let containsCurrent = chapter.turns.contains(where: { $0.turnIndex == currentTurnIndex })
        let isSelectedChapter = (chapter.id == activeChapterId)
        let containsError = chapter.turns.contains(where: { $0.hasError })
        let containsActive = chapter.turns.contains(where: { $0.isActive })
        
        VStack(alignment: .leading, spacing: 8) {
            // 可折叠章节卡片头部
            Button(action: {
                UIImpactFeedbackGenerator(style: .light).impactOccurred()
                withAnimation(.easeInOut(duration: 0.22)) {
                    if isCollapsed {
                        selectedChapterId = chapter.id
                        collapsedChapterIds.remove(chapter.id)
                    } else {
                        collapsedChapterIds.insert(chapter.id)
                    }
                }
            }) {
                HStack(spacing: 8) {
                    Image(systemName: chapter.isAiTitled ? "sparkles" : (chapter.dateText != nil ? "calendar" : "bookmark.fill"))
                        .font(.system(size: 11, weight: .semibold))
                        .foregroundColor(chapter.isAiTitled || containsCurrent || isSelectedChapter ? .indigo : .secondary)
                    
                    VStack(alignment: .leading, spacing: 1.5) {
                        Text(chapter.title)
                            .font(.system(size: 13, weight: .bold))
                            .foregroundColor(containsCurrent || isSelectedChapter ? .primary : .secondary)
                            .lineLimit(1)
                        if let sub = chapter.subtitle, !sub.isEmpty {
                            Text(sub)
                                .font(.system(size: 10.5))
                                .foregroundColor(.secondary)
                                .lineLimit(1)
                        }
                    }
                    
                    Spacer()
                    
                    // 状态与轮次胶囊
                    HStack(spacing: 5) {
                        if containsCurrent {
                            Text("当前")
                                .font(.system(size: 10, weight: .semibold))
                                .foregroundColor(.white)
                                .padding(.horizontal, 6)
                                .padding(.vertical, 2)
                                .background(Color.indigo)
                                .clipShape(Capsule())
                        }
                        if containsActive {
                            Text("进行中")
                                .font(.system(size: 10, weight: .semibold))
                                .foregroundColor(.blue)
                                .padding(.horizontal, 6)
                                .padding(.vertical, 2)
                                .background(Color.blue.opacity(0.12))
                                .clipShape(Capsule())
                        }
                        if containsError {
                            Text("异常")
                                .font(.system(size: 10, weight: .semibold))
                                .foregroundColor(.orange)
                                .padding(.horizontal, 6)
                                .padding(.vertical, 2)
                                .background(Color.orange.opacity(0.12))
                                .clipShape(Capsule())
                        }
                        
                        Text("\(chapter.turns.count) 轮")
                            .font(.system(size: 11, weight: .medium))
                            .foregroundColor(.secondary)
                            .padding(.horizontal, 7)
                            .padding(.vertical, 2)
                            .background(Color(uiColor: .tertiarySystemGroupedBackground))
                            .clipShape(Capsule())
                    }
                    
                    Image(systemName: "chevron.right")
                        .font(.system(size: 11, weight: .semibold))
                        .foregroundColor(.secondary)
                        .rotationEffect(.degrees(isCollapsed ? 0 : 90))
                        .animation(.easeInOut(duration: 0.22), value: isCollapsed)
                }
                .padding(.horizontal, 10)
                .padding(.vertical, 8)
                .background(Color(uiColor: .secondarySystemGroupedBackground).opacity(isCollapsed ? 0.9 : 0.6))
                .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
            }
            .buttonStyle(.plain)
            
            if !isCollapsed {
                turnsTimelineList(turns: chapter.turns)
                    .clipped()
                    .transition(.opacity)
            }
        }
        .clipped()
    }
    
    // MARK: - Timeline List
    
    @ViewBuilder
    private func turnsTimelineList(turns: [ConversationTurn]) -> some View {
        VStack(spacing: 0) {
            ForEach(Array(turns.enumerated()), id: \.element.id) { index, turn in
                let isCurrent = (turn.turnIndex == currentTurnIndex)
                let isFirst = (index == 0)
                let isLast = (index == turns.count - 1)
                
                HStack(alignment: .top, spacing: 14) {
                    // 左侧垂直时间线导轨
                    timelineNode(turn: turn, isCurrent: isCurrent, isFirst: isFirst, isLast: isLast)
                    
                    // 右侧内容卡片
                    turnCard(turn: turn, isCurrent: isCurrent)
                        .id(turn.id)
                }
            }
        }
    }
    
    // MARK: - Timeline Node & Track
    
    @ViewBuilder
    private func timelineNode(turn: ConversationTurn, isCurrent: Bool, isFirst: Bool, isLast: Bool) -> some View {
        VStack(spacing: 0) {
            // 上半段连接线
            Rectangle()
                .fill(isFirst ? Color.clear : Color.secondary.opacity(0.25))
                .frame(width: 2, height: 14)
            
            // 节点指示器
            ZStack {
                if isCurrent {
                    Circle()
                        .fill(Color.indigo.opacity(0.2))
                        .frame(width: 22, height: 22)
                }
                
                Circle()
                    .fill(nodeColor(for: turn, isCurrent: isCurrent))
                    .frame(width: isCurrent ? 14 : 10, height: isCurrent ? 14 : 10)
                    .overlay(
                        Circle()
                            .stroke(Color(uiColor: .systemGroupedBackground), lineWidth: isCurrent ? 2 : 1.5)
                    )
                
                if turn.hasError {
                    Image(systemName: "exclamationmark")
                        .font(.system(size: 7, weight: .bold))
                        .foregroundColor(.white)
                }
            }
            .frame(width: 22, height: 22)
            
            // 下半段连接线
            Rectangle()
                .fill(isLast ? Color.clear : Color.secondary.opacity(0.25))
                .frame(width: 2)
                .frame(maxHeight: .infinity)
        }
        .frame(width: 22)
    }
    
    private func nodeColor(for turn: ConversationTurn, isCurrent: Bool) -> Color {
        if turn.hasError {
            return .orange
        }
        if turn.isActive {
            return .blue
        }
        if isCurrent {
            return .indigo
        }
        return Color.secondary.opacity(0.7)
    }
    
    // MARK: - Turn Card
    
    @ViewBuilder
    private func turnCard(turn: ConversationTurn, isCurrent: Bool) -> some View {
        Button(action: {
            UIImpactFeedbackGenerator(style: .medium).impactOccurred()
            dismiss()
            onSelectTurn(turn)
        }) {
            VStack(alignment: .leading, spacing: 8) {
                // 顶部元数据栏
                HStack(alignment: .center, spacing: 8) {
                    Text("第 \(turn.turnIndex) 轮")
                        .font(.system(size: 11.5, weight: .semibold))
                        .foregroundColor(isCurrent ? .white : .indigo)
                        .padding(.horizontal, 7)
                        .padding(.vertical, 2.5)
                        .background(isCurrent ? Color.indigo : Color.indigo.opacity(0.12))
                        .clipShape(Capsule())
                    
                    if let model = turn.modelName {
                        Text(model)
                            .font(.system(size: 11, weight: .medium))
                            .foregroundColor(.secondary)
                            .lineLimit(1)
                    }
                    
                    Spacer()
                    
                    if turn.isActive {
                        HStack(spacing: 4) {
                            ProgressView()
                                .scaleEffect(0.65)
                            Text("处理中")
                                .font(.system(size: 11, weight: .medium))
                                .foregroundColor(.blue)
                        }
                    } else if turn.hasError {
                        HStack(spacing: 3) {
                            Image(systemName: "exclamationmark.triangle.fill")
                                .font(.system(size: 10))
                            Text("异常")
                                .font(.system(size: 11, weight: .medium))
                        }
                        .foregroundColor(.orange)
                    }
                    
                    Image(systemName: "chevron.right")
                        .font(.system(size: 11, weight: .semibold))
                        .foregroundColor(Color.secondary.opacity(0.4))
                }
                
                // Prompt 摘要或语音会话气泡
                if turn.isVoiceTurn {
                    HStack(alignment: .center, spacing: 8) {
                        HStack(spacing: 5) {
                            AntigravityVoiceShape()
                                .fill(Color.white)
                                .frame(width: 13, height: 13)
                            Text("语音交互")
                                .font(.system(size: 11.5, weight: .semibold))
                                .foregroundColor(.white)
                        }
                        .padding(.horizontal, 7.5)
                        .padding(.vertical, 3.5)
                        .background(Color.indigo)
                        .clipShape(Capsule())
                        
                        if !turn.promptSummary.isEmpty {
                            Text(turn.promptSummary)
                                .font(.system(size: 13.5, weight: isCurrent ? .semibold : .medium))
                                .foregroundColor(.primary)
                                .lineLimit(1)
                        }
                    }
                } else {
                    Text(turn.promptSummary)
                        .font(.system(size: 14, weight: isCurrent ? .semibold : .medium))
                        .foregroundColor(.primary)
                        .lineLimit(2)
                        .multilineTextAlignment(.leading)
                }
                
                // 底部产物与工具 Chips
                if !turn.artifacts.isEmpty || turn.totalTools > 0 {
                    HStack(spacing: 6) {
                        if !turn.artifacts.isEmpty {
                            HStack(spacing: 3) {
                                Image(systemName: "doc.text.fill")
                                    .font(.system(size: 9.5))
                                Text("\(turn.artifacts.count) 个产物")
                                    .font(.system(size: 10.5, weight: .medium))
                            }
                            .foregroundColor(.teal)
                            .padding(.horizontal, 6)
                            .padding(.vertical, 2)
                            .background(Color.teal.opacity(0.1))
                            .clipShape(RoundedRectangle(cornerRadius: 4, style: .continuous))
                        }
                        
                        if turn.totalTools > 0 {
                            HStack(spacing: 3) {
                                Image(systemName: "wrench.and.screwdriver.fill")
                                    .font(.system(size: 9))
                                Text("\(turn.totalTools) 次工具调用")
                                    .font(.system(size: 10.5, weight: .medium))
                            }
                            .foregroundColor(.green)
                            .padding(.horizontal, 6)
                            .padding(.vertical, 2)
                            .background(Color.green.opacity(0.12))
                            .clipShape(RoundedRectangle(cornerRadius: 4, style: .continuous))
                        }
                        
                        Spacer()
                    }
                }
            }
            .padding(12)
            .background(Color(uiColor: .secondarySystemGroupedBackground))
            .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
            .overlay(
                RoundedRectangle(cornerRadius: 12, style: .continuous)
                    .stroke(isCurrent ? Color.indigo.opacity(0.6) : Color.clear, lineWidth: 1.5)
            )
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .padding(.bottom, 12)
    }
}
