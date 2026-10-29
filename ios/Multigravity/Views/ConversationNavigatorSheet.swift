import SwiftUI

/// 对话导轨与大纲导航抽屉（对齐 Antigravity 2.28.1 官方时间线导轨规范）
public struct ConversationNavigatorSheet: View {
    @Environment(\.dismiss) private var dismiss
    public let turns: [ConversationTurn]
    public let chapters: [ConversationChapter]
    public let currentTurnIndex: Int?
    public let onSelectTurn: (ConversationTurn) -> Void
    public let onJumpToTop: () -> Void
    public let onJumpToBottom: () -> Void
    
    public init(
        turns: [ConversationTurn],
        chapters: [ConversationChapter],
        currentTurnIndex: Int? = nil,
        onSelectTurn: @escaping (ConversationTurn) -> Void,
        onJumpToTop: @escaping () -> Void,
        onJumpToBottom: @escaping () -> Void
    ) {
        self.turns = turns
        self.chapters = chapters
        self.currentTurnIndex = currentTurnIndex
        self.onSelectTurn = onSelectTurn
        self.onJumpToTop = onJumpToTop
        self.onJumpToBottom = onJumpToBottom
    }
    
    public var body: some View {
        NavigationStack {
            ScrollViewReader { scrollProxy in
                ScrollView(.vertical, showsIndicators: true) {
                    VStack(alignment: .leading, spacing: 18) {
                        headerSummaryBanner
                        
                        if chapters.count > 1 {
                            // 多章节呈现
                            ForEach(chapters) { chapter in
                                chapterSection(chapter: chapter)
                            }
                        } else {
                            // 单章节平铺呈现
                            turnsTimelineList(turns: turns)
                        }
                    }
                    .padding(.horizontal, 16)
                    .padding(.top, 14)
                    .padding(.bottom, 36)
                }
                .background(Color(uiColor: .systemGroupedBackground))
                .onAppear {
                    if let cur = currentTurnIndex, let target = turns.first(where: { $0.turnIndex == cur }) {
                        DispatchQueue.main.asyncAfter(deadline: .now() + 0.15) {
                            withAnimation(.easeOut(duration: 0.2)) {
                                scrollProxy.scrollTo(target.id, anchor: .center)
                            }
                        }
                    }
                }
            }
            .navigationTitle("对话大纲与步骤导轨")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    Button("关闭") {
                        dismiss()
                    }
                    .font(.system(size: 15))
                }
                
                ToolbarItem(placement: .topBarTrailing) {
                    HStack(spacing: 12) {
                        Button(action: {
                            UIImpactFeedbackGenerator(style: .light).impactOccurred()
                            dismiss()
                            onJumpToTop()
                        }) {
                            Image(systemName: "arrow.up.to.line")
                                .font(.system(size: 14, weight: .semibold))
                        }
                        .accessibilityLabel("跳到会话开头")
                        
                        Button(action: {
                            UIImpactFeedbackGenerator(style: .light).impactOccurred()
                            dismiss()
                            onJumpToBottom()
                        }) {
                            Image(systemName: "arrow.down.to.line")
                                .font(.system(size: 14, weight: .semibold))
                        }
                        .accessibilityLabel("跳到最新回复")
                    }
                }
            }
        }
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
                Text(chapters.count > 1 ? "共 \(chapters.count) 个章节 · \(turns.count) 轮问答" : "共 \(turns.count) 轮问答步骤")
                    .font(.system(size: 14.5, weight: .semibold))
                    .foregroundColor(.primary)
                
                Text("点击任意步骤即可平滑瞬达对应提问与上下文")
                    .font(.system(size: 12))
                    .foregroundColor(.secondary)
            }
            
            Spacer()
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 10)
        .background(Color(uiColor: .secondarySystemGroupedBackground))
        .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
    }
    
    // MARK: - Chapter Section
    
    @ViewBuilder
    private func chapterSection(chapter: ConversationChapter) -> some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack(spacing: 6) {
                Image(systemName: "bookmark.fill")
                    .font(.system(size: 11))
                    .foregroundColor(.indigo)
                Text(chapter.title)
                    .font(.system(size: 13, weight: .bold))
                    .foregroundColor(.secondary)
                    .textCase(.uppercase)
                Spacer()
            }
            .padding(.leading, 4)
            .padding(.top, 4)
            
            turnsTimelineList(turns: chapter.turns)
        }
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
                
                // Prompt 摘要
                Text(turn.promptSummary)
                    .font(.system(size: 14, weight: isCurrent ? .semibold : .medium))
                    .foregroundColor(.primary)
                    .lineLimit(2)
                    .multilineTextAlignment(.leading)
                
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
                            .foregroundColor(.purple)
                            .padding(.horizontal, 6)
                            .padding(.vertical, 2)
                            .background(Color.purple.opacity(0.1))
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
