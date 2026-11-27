import SwiftUI

/// 手机版新建对话窗口 · 方案 A（混合型轻启动台）
/// - 问候头（带账号昵称）
/// - 单行状态微条（实时配额 + 今日用量 + 缓存命中率，点击直达「账号与配额」）
/// - 工作区 Git 脉搏（分支 + 未提交改动数 + 草稿就地切换）
/// - 2×2 情境自适应快捷发起卡（动态感知脏工作区 + 置顶自定义模板 + 高频轻指令）
/// - 底部全球经典书目阶梯折算彩蛋（点击直达「Token 历史」）
/// - 键盘弹出时自动平滑折叠为单行紧凑态；iPad / 宽屏居中限宽 520pt。
public struct NewChatLaunchpadView: View {
    public let viewModel: ChatViewModel
    public let isKeyboardVisible: Bool
    public let onFillPrompt: (String) -> Void
    public let onSwitchWorkspace: () -> Void
    public let onOpenQuota: () -> Void
    public let onOpenTokenHistory: () -> Void
    public let onTapDirtyFiles: () -> Void

    @State private var settings = AppSettings.shared
    @State private var quotaCache = ModelPickerQuotaCache.shared
    @State private var tokenCache = TokenHistoryCache.shared
    @State private var workflowStore = WorkflowTemplateStore.shared

    private struct LaunchCard: Identifiable {
        let id: String
        let icon: String
        let tint: Color
        let title: String
        let subtitle: String
        let prompt: String
    }

    public init(
        viewModel: ChatViewModel,
        isKeyboardVisible: Bool,
        onFillPrompt: @escaping (String) -> Void,
        onSwitchWorkspace: @escaping () -> Void,
        onOpenQuota: @escaping () -> Void,
        onOpenTokenHistory: @escaping () -> Void,
        onTapDirtyFiles: @escaping () -> Void
    ) {
        self.viewModel = viewModel
        self.isKeyboardVisible = isKeyboardVisible
        self.onFillPrompt = onFillPrompt
        self.onSwitchWorkspace = onSwitchWorkspace
        self.onOpenQuota = onOpenQuota
        self.onOpenTokenHistory = onOpenTokenHistory
        self.onTapDirtyFiles = onTapDirtyFiles
    }

    private var account: CockpitAccountQuota? {
        quotaCache.account
    }

    private var tokenHistory: TokenHistoryResponse? {
        tokenCache.history
    }

    private var resolvedQuota: (kind: QuotaBucketKind, bucket: CockpitQuotaBucket)? {
        account?.resolvedQuotaDisplay(preferred: settings.selectedQuotaBucket)
    }

    private var greetingText: String {
        let hour = Calendar.current.component(.hour, from: Date())
        let period: String
        switch hour {
        case 5..<12: period = String(localized: "上午好")
        case 12..<18: period = String(localized: "下午好")
        default: period = String(localized: "晚上好")
        }
        let rawName = account?.name.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        let shortName: String = {
            guard !rawName.isEmpty else { return "" }
            if let firstPart = rawName.split(separator: " ").first {
                return String(firstPart)
            }
            return rawName
        }()
        if isKeyboardVisible {
            return shortName.isEmpty ? period : "\(period)，\(shortName)"
        }
        if shortName.isEmpty {
            return String(localized: "\(period)，接下来做什么？")
        }
        return String(localized: "\(period)，接下来做什么，\(shortName)？")
    }

    private var quotaPercent: Int? {
        guard let pct = resolvedQuota?.bucket.remainingPercent else { return nil }
        return max(0, min(100, Int(pct.rounded())))
    }

    private var quotaColor: Color {
        guard let pct = quotaPercent else { return .secondary }
        if pct >= 50 { return .green }
        if pct >= 20 { return .orange }
        return .red
    }

    private var quotaLabelShort: String {
        guard let kind = resolvedQuota?.kind else { return "" }
        switch kind {
        case .auto: return ""
        case .gemini5h: return "Gemini 5h"
        case .geminiWeekly: return "Gemini"
        case .claude5h: return "Claude 5h"
        case .claudeWeekly: return "Claude"
        }
    }

    private var todayTokensText: String? {
        guard let s = tokenHistory?.summary, s.todayTokens > 0 else { return nil }
        return AccountQuotaSheet.formatTokensCompact(s.todayTokens)
    }

    private var cacheHitPercent: Int? {
        guard let h = tokenHistory else { return nil }
        return TokenBookEquivalent.cacheHitRatePercent(from: h)
    }

    private var bookEquivalent: TokenBookEquivalent? {
        guard let h = tokenHistory else { return nil }
        return TokenBookEquivalent.resolve(from: h)
    }

    private var hasStatusStripData: Bool {
        quotaPercent != nil || todayTokensText != nil || cacheHitPercent != nil
    }

    private var workspaceDisplayName: String {
        if let project = viewModel.draftProject, !project.isPureChat {
            return project.displayName
        }
        let ws = viewModel.workspaceName.trimmingCharacters(in: .whitespacesAndNewlines)
        if !ws.isEmpty && ws != "Chat" {
            return ws
        }
        return viewModel.currentTitle
    }

    private var launchCards: [LaunchCard] {
        var result: [LaunchCard] = []
        let isPure = viewModel.isPureChat
        let dirtyCount = viewModel.uncommittedFileCount

        if !isPure {
            if dirtyCount > 0 {
                result.append(LaunchCard(
                    id: "dirty_review",
                    icon: "magnifyingglass.circle.fill",
                    tint: .orange,
                    title: String(localized: "审查当前 \(dirtyCount) 个改动"),
                    subtitle: String(localized: "检查 git diff 风险与优化"),
                    prompt: String(localized: "请先检查当前未提交的改动，指出潜在风险并给出优化建议")
                ))
            } else {
                result.append(LaunchCard(
                    id: "arch_overview",
                    icon: "map.fill",
                    tint: .indigo,
                    title: String(localized: "梳理项目结构"),
                    subtitle: String(localized: "快速定位核心模块与入口"),
                    prompt: String(localized: "请先梳理这个工作区的核心模块与关键入口文件")
                ))
            }
        }

        // 优先承接用户放入快捷菜单的自定义 Workflow 模板（跳过未改动的内置默认重型模板）
        let defaultIds: Set<String> = ["code_review", "test_and_fix", "research", "custom"]
        let maxPinned = isPure ? 1 : 2
        let pinnedTemplates = workflowStore.templates.filter { $0.inQuickMenu && !defaultIds.contains($0.id) }
        for tpl in pinnedTemplates.prefix(maxPinned) {
            result.append(LaunchCard(
                id: "pinned_\(tpl.id)",
                icon: "pin.fill",
                tint: .purple,
                title: tpl.title,
                subtitle: tpl.subtitle.isEmpty ? String(localized: "置顶模板") : tpl.subtitle,
                prompt: tpl.templatePrompt
            ))
        }

        let fallbackPool: [LaunchCard] = isPure ? [
            LaunchCard(
                id: "chat_grill",
                icon: "bubble.left.and.text.bubble.right.fill",
                tint: .indigo,
                title: String(localized: "/grill-me 探讨"),
                subtitle: String(localized: "逐项拆解模糊想法"),
                prompt: "/grill-me "
            ),
            LaunchCard(
                id: "chat_search",
                icon: "globe",
                tint: .blue,
                title: String(localized: "联网核查资讯"),
                subtitle: String(localized: "交叉验证最新进展"),
                prompt: String(localized: "请帮我联网核查以下消息的最新进展与可靠来源：")
            ),
            LaunchCard(
                id: "chat_draft",
                icon: "pencil.and.outline",
                tint: .teal,
                title: String(localized: "起草与润色"),
                subtitle: String(localized: "文章 / 公告 / 提纲"),
                prompt: String(localized: "帮我起草一份清晰自然的说明：")
            ),
            LaunchCard(
                id: "chat_compare",
                icon: "scale.3d",
                tint: .orange,
                title: String(localized: "多维方案对比"),
                subtitle: String(localized: "权衡利弊与取舍"),
                prompt: String(localized: "请从利弊、成本与风险角度对比以下方案：")
            )
        ] : [
            LaunchCard(
                id: "ws_plan",
                icon: "list.bullet.clipboard.fill",
                tint: .blue,
                title: String(localized: "/plan 方案规划"),
                subtitle: String(localized: "先对齐步骤再动手"),
                prompt: "/plan "
            ),
            LaunchCard(
                id: "ws_grill",
                icon: "bubble.left.and.text.bubble.right.fill",
                tint: .indigo,
                title: String(localized: "/grill-me 探讨"),
                subtitle: String(localized: "逐项拆解需求决策"),
                prompt: "/grill-me "
            ),
            LaunchCard(
                id: "ws_bug",
                icon: "ladybug.fill",
                tint: .red,
                title: String(localized: "定位排查 Bug"),
                subtitle: String(localized: "结合上下文与日志定位"),
                prompt: String(localized: "请帮我排查并定位以下问题的根因：")
            ),
            LaunchCard(
                id: "ws_refactor",
                icon: "wand.and.stars",
                tint: .teal,
                title: String(localized: "优化与重构"),
                subtitle: String(localized: "提升可读性与稳定性"),
                prompt: String(localized: "请分析当前模块可优化的设计点并给出轻量重构方案：")
            )
        ]

        for item in fallbackPool where result.count < 4 {
            result.append(item)
        }
        return Array(result.prefix(4))
    }

    public var body: some View {
        Group {
            if isKeyboardVisible {
                compactKeyboardView
            } else {
                fullLaunchpadView
            }
        }
        .frame(maxWidth: 520)
        .frame(maxWidth: .infinity)
        .animation(.easeOut(duration: 0.2), value: isKeyboardVisible)
        .task {
            await tokenCache.prefetchIfNeeded()
        }
    }

    // MARK: - 键盘弹出时的单行紧凑态

    private var compactKeyboardView: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack(spacing: 8) {
                brandBadge(size: 26, fontSize: 12.5)
                Text(greetingText)
                    .font(.system(size: 15, weight: .semibold))
                    .foregroundColor(.primary)
                    .lineLimit(1)
                Spacer(minLength: 6)
                if hasStatusStripData {
                    Button {
                        UIImpactFeedbackGenerator(style: .light).impactOccurred()
                        onOpenQuota()
                    } label: {
                        HStack(spacing: 5) {
                            if let pct = quotaPercent {
                                Image(systemName: "bolt.fill")
                                    .font(.system(size: 10, weight: .bold))
                                    .foregroundColor(quotaColor)
                                Text("\(pct)%")
                                    .font(.system(size: 11.5, weight: .semibold, design: .monospaced))
                                    .foregroundColor(quotaColor)
                            }
                            if let today = todayTokensText {
                                if quotaPercent != nil {
                                    Text("·")
                                        .font(.system(size: 11))
                                        .foregroundColor(.secondary)
                                }
                                Text(today)
                                    .font(.system(size: 11.5, design: .monospaced))
                                    .foregroundColor(.secondary)
                            }
                        }
                        .padding(.horizontal, 9)
                        .padding(.vertical, 5)
                        .background(Color(uiColor: .secondarySystemBackground))
                        .clipShape(Capsule())
                        .overlay(Capsule().stroke(Color.primary.opacity(0.08), lineWidth: 0.8))
                    }
                    .buttonStyle(.plain)
                }
            }

            ScrollView(.horizontal, showsIndicators: false) {
                HStack(spacing: 8) {
                    ForEach(launchCards) { card in
                        Button {
                            UIImpactFeedbackGenerator(style: .light).impactOccurred()
                            onFillPrompt(card.prompt)
                        } label: {
                            HStack(spacing: 6) {
                                Image(systemName: card.icon)
                                    .font(.system(size: 11.5, weight: .semibold))
                                    .foregroundColor(card.tint)
                                Text(card.title)
                                    .font(.system(size: 12.5, weight: .medium))
                                    .foregroundColor(.primary)
                                    .lineLimit(1)
                            }
                            .padding(.horizontal, 11)
                            .padding(.vertical, 8)
                            .background(Color(uiColor: .secondarySystemBackground))
                            .clipShape(RoundedRectangle(cornerRadius: 11, style: .continuous))
                            .overlay(
                                RoundedRectangle(cornerRadius: 11, style: .continuous)
                                    .stroke(Color.primary.opacity(0.08), lineWidth: 0.8)
                            )
                        }
                        .buttonStyle(.plain)
                    }
                }
            }
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 18)
    }

    // MARK: - 键盘收起时的完整启动台

    private var fullLaunchpadView: some View {
        VStack(alignment: .leading, spacing: 14) {
            Spacer(minLength: 14)

            // 1. 个性化问候头
            HStack(spacing: 10) {
                brandBadge(size: 32, fontSize: 15)
                Text(greetingText)
                    .font(.system(size: 18, weight: .semibold))
                    .foregroundColor(.primary)
                    .lineLimit(2)
                    .minimumScaleFactor(0.88)
            }
            .padding(.bottom, 2)

            // 2. 单行状态微条（零闪烁降级）
            if hasStatusStripData {
                statusMicroStrip
            }

            // 3. 工作区 Git 脉搏栏（兼就地切换）
            workspacePulseBar

            // 4. 2×2 情境自适应快捷发起卡
            VStack(alignment: .leading, spacing: 8) {
                Text("快捷发起")
                    .font(.system(size: 12, weight: .medium))
                    .foregroundColor(.secondary)
                    .padding(.leading, 2)

                LazyVGrid(
                    columns: [
                        GridItem(.flexible(), spacing: 9),
                        GridItem(.flexible(), spacing: 9)
                    ],
                    spacing: 9
                ) {
                    ForEach(launchCards) { card in
                        quickCardView(card)
                    }
                }
            }
            .padding(.top, 2)

            // 5. 底部全球经典书目阶梯折算彩蛋
            if let eq = bookEquivalent {
                Button {
                    UIImpactFeedbackGenerator(style: .light).impactOccurred()
                    onOpenTokenHistory()
                } label: {
                    HStack(spacing: 5) {
                        Image(systemName: "sparkles")
                            .font(.system(size: 11, weight: .medium))
                            .foregroundColor(.indigo)
                        Text(eq.sentence)
                            .font(.system(size: 12, weight: .medium))
                            .foregroundColor(.secondary)
                            .lineLimit(1)
                            .minimumScaleFactor(0.85)
                        Image(systemName: "chevron.right")
                            .font(.system(size: 10, weight: .semibold))
                            .foregroundColor(Color(uiColor: .tertiaryLabel))
                    }
                    .frame(maxWidth: .infinity)
                    .padding(.vertical, 8)
                    .padding(.horizontal, 10)
                    .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                .padding(.top, 2)
            }

            Spacer(minLength: 10)
        }
        .padding(.horizontal, 16)
    }

    private func brandBadge(size: CGFloat, fontSize: CGFloat) -> some View {
        ZStack {
            RoundedRectangle(cornerRadius: size * 0.3, style: .continuous)
                .fill(Color.indigo.opacity(0.14))
                .frame(width: size, height: size)
            Image(systemName: "sparkles")
                .font(.system(size: fontSize, weight: .semibold))
                .foregroundColor(.indigo)
        }
    }

    private var statusMicroStrip: some View {
        Button {
            UIImpactFeedbackGenerator(style: .light).impactOccurred()
            onOpenQuota()
        } label: {
            HStack(spacing: 8) {
                if let pct = quotaPercent {
                    HStack(spacing: 4) {
                        Image(systemName: "bolt.fill")
                            .font(.system(size: 11, weight: .bold))
                            .foregroundColor(quotaColor)
                        if !quotaLabelShort.isEmpty {
                            Text(quotaLabelShort)
                                .font(.system(size: 12, weight: .medium))
                                .foregroundColor(.primary)
                                .lineLimit(1)
                        }
                        Text("\(pct)%")
                            .font(.system(size: 12, weight: .bold, design: .monospaced))
                            .foregroundColor(quotaColor)
                    }
                    .fixedSize(horizontal: true, vertical: false)

                    GeometryReader { geo in
                        ZStack(alignment: .leading) {
                            Capsule()
                                .fill(Color.primary.opacity(0.08))
                            Capsule()
                                .fill(quotaColor)
                                .frame(width: max(2, geo.size.width * CGFloat(pct) / 100.0))
                        }
                    }
                    .frame(width: 34, height: 5)
                }

                HStack(spacing: 5) {
                    if let today = todayTokensText {
                        Text("今日 \(today)")
                            .font(.system(size: 12))
                            .foregroundColor(.secondary)
                            .lineLimit(1)
                    }
                    if let hit = cacheHitPercent {
                        if todayTokensText != nil {
                            Text("·")
                                .font(.system(size: 11))
                                .foregroundColor(Color(uiColor: .tertiaryLabel))
                        }
                        Text("缓存 \(hit)%")
                            .font(.system(size: 12))
                            .foregroundColor(.secondary)
                            .lineLimit(1)
                    }
                }
                .minimumScaleFactor(0.85)

                Spacer(minLength: 4)

                Image(systemName: "chevron.right")
                    .font(.system(size: 11, weight: .semibold))
                    .foregroundColor(Color(uiColor: .tertiaryLabel))
            }
            .padding(.horizontal, 12)
            .padding(.vertical, 10)
            .background(Color(uiColor: .secondarySystemBackground))
            .clipShape(RoundedRectangle(cornerRadius: 13, style: .continuous))
            .overlay(
                RoundedRectangle(cornerRadius: 13, style: .continuous)
                    .stroke(Color.primary.opacity(0.07), lineWidth: 0.8)
            )
        }
        .buttonStyle(.plain)
    }

    private var workspacePulseBar: some View {
        HStack(spacing: 8) {
            if viewModel.isPureChat {
                Image(systemName: "bubble.left.and.bubble.right.fill")
                    .font(.system(size: 12.5))
                    .foregroundColor(.indigo)
                Text("纯对话模式 · 不关联代码工作区")
                    .font(.system(size: 12.5))
                    .foregroundColor(.secondary)
                    .lineLimit(1)
                    .minimumScaleFactor(0.85)
                Spacer(minLength: 6)
                if viewModel.isDraftConversation {
                    Button {
                        UIImpactFeedbackGenerator(style: .light).impactOccurred()
                        onSwitchWorkspace()
                    } label: {
                        Text("选工作区")
                            .font(.system(size: 12.5, weight: .semibold))
                            .foregroundColor(.indigo)
                    }
                    .buttonStyle(.plain)
                }
            } else {
                Image(systemName: "folder.fill")
                    .font(.system(size: 12.5))
                    .foregroundColor(.accentColor)
                Text(workspaceDisplayName)
                    .font(.system(size: 13, weight: .semibold))
                    .foregroundColor(.primary)
                    .lineLimit(1)
                    .truncationMode(.tail)

                if let branch = viewModel.gitBranch, !branch.isEmpty {
                    Text(branch)
                        .font(.system(size: 11, weight: .medium, design: .monospaced))
                        .foregroundColor(.secondary)
                        .padding(.horizontal, 6)
                        .padding(.vertical, 2)
                        .background(Color.primary.opacity(0.06))
                        .clipShape(RoundedRectangle(cornerRadius: 5, style: .continuous))
                        .lineLimit(1)
                        .fixedSize(horizontal: true, vertical: false)
                }

                if viewModel.uncommittedFileCount > 0 {
                    Button {
                        UIImpactFeedbackGenerator(style: .light).impactOccurred()
                        onTapDirtyFiles()
                    } label: {
                        Text("\(viewModel.uncommittedFileCount) 改动")
                            .font(.system(size: 11, weight: .semibold))
                            .foregroundColor(.orange)
                            .padding(.horizontal, 6)
                            .padding(.vertical, 2)
                            .background(Color.orange.opacity(0.14))
                            .clipShape(RoundedRectangle(cornerRadius: 5, style: .continuous))
                            .fixedSize(horizontal: true, vertical: false)
                    }
                    .buttonStyle(.plain)
                } else if viewModel.gitBranch != nil {
                    Text("干净")
                        .font(.system(size: 11))
                        .foregroundColor(.secondary)
                        .fixedSize(horizontal: true, vertical: false)
                }

                Spacer(minLength: 4)

                if viewModel.isDraftConversation {
                    Button {
                        UIImpactFeedbackGenerator(style: .light).impactOccurred()
                        onSwitchWorkspace()
                    } label: {
                        Text("切换")
                            .font(.system(size: 12.5, weight: .semibold))
                            .foregroundColor(.indigo)
                            .fixedSize(horizontal: true, vertical: false)
                    }
                    .buttonStyle(.plain)
                }
            }
        }
        .padding(.horizontal, 12)
        .padding(.vertical, 10)
        .background(Color(uiColor: .secondarySystemBackground))
        .clipShape(RoundedRectangle(cornerRadius: 13, style: .continuous))
        .overlay(
            RoundedRectangle(cornerRadius: 13, style: .continuous)
                .stroke(Color.primary.opacity(0.07), lineWidth: 0.8)
        )
    }

    private func quickCardView(_ card: LaunchCard) -> some View {
        Button {
            UIImpactFeedbackGenerator(style: .light).impactOccurred()
            onFillPrompt(card.prompt)
        } label: {
            VStack(alignment: .leading, spacing: 5) {
                HStack(spacing: 6) {
                    Image(systemName: card.icon)
                        .font(.system(size: 12.5, weight: .semibold))
                        .foregroundColor(card.tint)
                    Text(card.title)
                        .font(.system(size: 13, weight: .semibold))
                        .foregroundColor(.primary)
                        .lineLimit(1)
                }
                Text(card.subtitle)
                    .font(.system(size: 11.5))
                    .foregroundColor(.secondary)
                    .lineLimit(1)
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(.horizontal, 12)
            .padding(.vertical, 11)
            .background(Color(uiColor: .secondarySystemBackground))
            .clipShape(RoundedRectangle(cornerRadius: 13, style: .continuous))
            .overlay(
                RoundedRectangle(cornerRadius: 13, style: .continuous)
                    .stroke(Color.primary.opacity(0.07), lineWidth: 0.8)
            )
        }
        .buttonStyle(.plain)
    }
}
