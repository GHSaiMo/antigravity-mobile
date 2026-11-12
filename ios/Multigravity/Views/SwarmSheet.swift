import SwiftUI

public struct SwarmSheet: View {
    @Environment(\.dismiss) private var dismiss
    @State private var settings = AppSettings.shared

    public let cascadeId: String
    public let onSelectConversation: ((String) -> Void)?

    @State private var selectedTab: Tab = .board
    @State private var snapshot: SwarmSnapshot? = nil
    @State private var isLoading: Bool = false
    @State private var errorMessage: String? = nil

    // Overview feedback
    @State private var feedbackText: String = ""
    @State private var isSendingFeedback: Bool = false
    @State private var nudgingMembers: Set<String> = []
    @State private var toastMessage: String? = nil

    // Board filters & expand state
    @State private var boardSearch: String = ""
    @State private var boardAgentFilter: String? = nil // nil = all, "@all" = broadcast, "@none" = unaddressed, or agent name
    @State private var expandedSeqs: Set<Int> = []

    // Members search & retired disclosure
    @State private var memberSearch: String = ""
    @State private var showRetiredMembers: Bool = false
    @State private var selectedMemberForActions: String? = nil

    public enum Tab: String, CaseIterable, Identifiable {
        case overview = "概览"
        case board = "协作板"
        case members = "成员"
        case graph = "拓扑"
        public var id: String { rawValue }
    }

    public init(cascadeId: String, onSelectConversation: ((String) -> Void)? = nil) {
        self.cascadeId = cascadeId
        self.onSelectConversation = onSelectConversation
    }

    public var body: some View {
        VStack(spacing: 0) {
            swarmHeader
            swarmTabBar

            Divider()
                .overlay(SwarmTheme.border)

            if isLoading && snapshot == nil {
                Spacer()
                ProgressView("正在同步蜂群看板与成员状态...")
                    .font(.system(size: 13.5))
                    .foregroundColor(SwarmTheme.muted)
                    .tint(SwarmTheme.accent)
                Spacer()
            } else if let error = errorMessage, snapshot == nil {
                errorView(error)
            } else if let snap = snapshot {
                switch selectedTab {
                case .overview:
                    overviewView(snap)
                case .board:
                    boardView(snap)
                case .members:
                    membersView(snap)
                case .graph:
                    SwarmGraphTabView(
                        snapshot: snap,
                        onSelectConversation: onSelectConversation,
                        onDismissSheet: { dismiss() }
                    )
                }
            } else {
                emptyStateView
            }
        }
        .background(SwarmTheme.bg.ignoresSafeArea())
        .overlay(alignment: .bottom) {
            if let toast = toastMessage {
                Text(toast)
                    .font(.system(size: 12.5, weight: .medium))
                    .foregroundColor(.white)
                    .padding(.horizontal, 16)
                    .padding(.vertical, 8)
                    .background(Color.black.opacity(0.84))
                    .clipShape(Capsule())
                    .shadow(radius: 6)
                    .padding(.bottom, 22)
                    .transition(.move(edge: .bottom).combined(with: .opacity))
            }
        }
        .animation(.easeInOut(duration: 0.2), value: toastMessage)
        .task {
            await loadSnapshot(silent: false)
            // Live auto-refresh every 3 seconds while sheet is open
            while !Task.isCancelled {
                try? await Task.sleep(nanoseconds: 3_000_000_000)
                if Task.isCancelled { break }
                await loadSnapshot(silent: true)
            }
        }
    }

    // MARK: - Header & Desktop-Style Underline Tab Bar (No Top-Right "完成")

    private var swarmHeader: some View {
        HStack(alignment: .center, spacing: 10) {
            Text(snapshot?.title.isEmpty == false ? snapshot!.title : "蜂群协同 (Swarm)")
                .font(.system(size: 16, weight: .semibold))
                .foregroundColor(SwarmTheme.ink)
                .lineLimit(2)
                .multilineTextAlignment(.leading)

            Spacer(minLength: 8)

            Button(action: {
                UIImpactFeedbackGenerator(style: .light).impactOccurred()
                Task { await loadSnapshot(silent: false) }
            }) {
                HStack(spacing: 5) {
                    Circle()
                        .fill(isLoading ? SwarmTheme.warnAmber : SwarmTheme.liveGreen)
                        .frame(width: 7, height: 7)
                    Text("Live")
                        .font(.system(size: 12, weight: .medium))
                        .foregroundColor(SwarmTheme.muted)
                }
                .padding(.horizontal, 8)
                .padding(.vertical, 4)
                .background(SwarmTheme.cardBg)
                .clipShape(Capsule())
            }
            .buttonStyle(.plain)
        }
        .padding(.horizontal, 16)
        .padding(.top, 18)
        .padding(.bottom, 8)
    }

    private var swarmTabBar: some View {
        let boardCount = snapshot?.messages.count ?? 0
        let memberCount = snapshot?.members.count ?? 0

        return HStack(spacing: 20) {
            ForEach(Tab.allCases) { tab in
                let isSelected = (selectedTab == tab)
                let countText: String? = {
                    switch tab {
                    case .board where boardCount > 0: return "\(boardCount)"
                    case .members where memberCount > 0: return "\(memberCount)"
                    default: return nil
                    }
                }()

                Button(action: {
                    UIImpactFeedbackGenerator(style: .light).impactOccurred()
                    withAnimation(.easeInOut(duration: 0.16)) {
                        selectedTab = tab
                    }
                }) {
                    VStack(spacing: 7) {
                        HStack(spacing: 4) {
                            Text(tab.rawValue)
                                .font(.system(size: 14, weight: isSelected ? .semibold : .medium))
                                .foregroundColor(isSelected ? SwarmTheme.ink : SwarmTheme.muted)

                            if let countText {
                                Text(countText)
                                    .font(.system(size: 12.5, weight: .regular, design: .monospaced))
                                    .foregroundColor(isSelected ? SwarmTheme.muted : SwarmTheme.faint)
                            }
                        }

                        Rectangle()
                            .fill(isSelected ? SwarmTheme.accent : Color.clear)
                            .frame(height: 2)
                            .clipShape(Capsule())
                    }
                    .padding(.horizontal, 2)
                }
                .buttonStyle(.plain)
            }

            Spacer()
        }
        .padding(.horizontal, 16)
        .padding(.top, 2)
    }

    // MARK: - Overview Tab (Warm Palette, Zero Magenta)

    @ViewBuilder
    private func overviewView(_ snap: SwarmSnapshot) -> some View {
        let phaseLabel = snap.view?.phase?.label ?? snap.brief.phase
        ScrollView {
            VStack(alignment: .leading, spacing: 14) {
                VStack(alignment: .leading, spacing: 10) {
                    HStack {
                        Text(snap.title.isEmpty ? "蜂群协同任务" : snap.title)
                            .font(.system(size: 16, weight: .bold))
                            .foregroundColor(SwarmTheme.ink)

                        Spacer()

                        if !phaseLabel.isEmpty {
                            Text(phaseLabel)
                                .font(.system(size: 11.5, weight: .semibold))
                                .foregroundColor(SwarmTheme.accent)
                                .padding(.horizontal, 8)
                                .padding(.vertical, 3)
                                .background(SwarmTheme.accent.opacity(0.12))
                                .clipShape(Capsule())
                        }
                    }

                    let headline = snap.view?.headline ?? snap.brief.headline
                    if !headline.isEmpty {
                        Text(headline)
                            .font(.system(size: 13.5))
                            .foregroundColor(SwarmTheme.muted)
                            .lineSpacing(2)
                    }

                    HStack(spacing: 10) {
                        statBox(
                            title: "活跃成员",
                            value: "\(snap.brief.workingCount) / \(snap.brief.totalMembers)",
                            icon: "person.2.fill",
                            color: SwarmTheme.liveGreen
                        )
                        statBox(
                            title: "协作消息",
                            value: "\(max(snap.messages.count, snap.brief.latestSeq))",
                            icon: "bubble.left.and.bubble.right.fill",
                            color: SwarmTheme.infoBlue
                        )
                        statBox(
                            title: "待关注",
                            value: "\(snap.brief.attentionCount)",
                            icon: "exclamationmark.triangle.fill",
                            color: snap.brief.attentionCount > 0 ? SwarmTheme.warnAmber : SwarmTheme.muted
                        )
                    }
                    .padding(.top, 2)
                }
                .padding(14)
                .background(SwarmTheme.cardBg)
                .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
                .overlay(RoundedRectangle(cornerRadius: 12, style: .continuous).stroke(SwarmTheme.border, lineWidth: 0.7))

                // User Feedback Composer
                VStack(alignment: .leading, spacing: 10) {
                    HStack(spacing: 6) {
                        Image(systemName: "megaphone.fill")
                            .font(.system(size: 13))
                            .foregroundColor(SwarmTheme.accent)
                        Text("协同指挥 / 用户反馈")
                            .font(.system(size: 14.5, weight: .semibold))
                            .foregroundColor(SwarmTheme.ink)
                    }

                    Text("直接向蜂群协作板发送人类反馈，协调各成员或给出方向指令（发送至协调者）。")
                        .font(.system(size: 12))
                        .foregroundColor(SwarmTheme.muted)

                    HStack(spacing: 8) {
                        TextField("输入对全体成员或协调者的反馈...", text: $feedbackText)
                            .font(.system(size: 13))
                            .foregroundColor(SwarmTheme.ink)
                            .padding(.horizontal, 10)
                            .padding(.vertical, 8)
                            .background(SwarmTheme.surfaceBg)
                            .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
                            .overlay(RoundedRectangle(cornerRadius: 8, style: .continuous).stroke(SwarmTheme.border, lineWidth: 0.7))

                        Button(action: sendFeedback) {
                            Group {
                                if isSendingFeedback {
                                    ProgressView()
                                        .tint(.white)
                                        .scaleEffect(0.8)
                                } else {
                                    Text("发板")
                                        .font(.system(size: 13, weight: .semibold))
                                }
                            }
                            .foregroundColor(.white)
                            .padding(.horizontal, 14)
                            .padding(.vertical, 8)
                            .background(feedbackText.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty || isSendingFeedback ? SwarmTheme.faint : SwarmTheme.accent)
                            .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))
                        }
                        .buttonStyle(.plain)
                        .disabled(feedbackText.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty || isSendingFeedback)
                    }
                }
                .padding(14)
                .background(SwarmTheme.cardBg)
                .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
                .overlay(RoundedRectangle(cornerRadius: 12, style: .continuous).stroke(SwarmTheme.border, lineWidth: 0.7))

                // Attention items from view.json
                if let attentions = snap.view?.attention, !attentions.isEmpty {
                    VStack(alignment: .leading, spacing: 10) {
                        HStack(spacing: 6) {
                            Image(systemName: "exclamationmark.triangle.fill")
                                .font(.system(size: 13))
                                .foregroundColor(SwarmTheme.warnAmber)
                            Text("待处理事项 (\(attentions.count))")
                                .font(.system(size: 14, weight: .semibold))
                                .foregroundColor(SwarmTheme.ink)
                        }

                        ForEach(attentions) { item in
                            VStack(alignment: .leading, spacing: 4) {
                                HStack {
                                    Text(item.title)
                                        .font(.system(size: 13.5, weight: .semibold))
                                        .foregroundColor(SwarmTheme.ink)
                                    Spacer()
                                    Text(item.kind.uppercased())
                                        .font(.system(size: 10, weight: .bold))
                                        .foregroundColor(attentionColor(item.severity))
                                        .padding(.horizontal, 6)
                                        .padding(.vertical, 2)
                                        .background(attentionColor(item.severity).opacity(0.14))
                                        .clipShape(Capsule())
                                }
                                if let why = item.why, !why.isEmpty {
                                    Text(why)
                                        .font(.system(size: 12))
                                        .foregroundColor(SwarmTheme.muted)
                                }
                            }
                            .padding(10)
                            .frame(maxWidth: .infinity, alignment: .leading)
                            .background(attentionColor(item.severity).opacity(0.08))
                            .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
                        }
                    }
                    .padding(14)
                    .background(SwarmTheme.cardBg)
                    .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
                    .overlay(RoundedRectangle(cornerRadius: 12, style: .continuous).stroke(SwarmTheme.border, lineWidth: 0.7))
                }

                // Workstreams from view.json
                if let workstreams = snap.view?.workstreams, !workstreams.isEmpty {
                    VStack(alignment: .leading, spacing: 10) {
                        Text("工作流推进 (\(workstreams.count))")
                            .font(.system(size: 14, weight: .semibold))
                            .foregroundColor(SwarmTheme.muted)

                        ForEach(workstreams) { ws in
                            VStack(alignment: .leading, spacing: 5) {
                                HStack {
                                    Text(ws.title)
                                        .font(.system(size: 13.5, weight: .semibold))
                                        .foregroundColor(SwarmTheme.ink)
                                    Spacer()
                                    Text(ws.state)
                                        .font(.system(size: 11, weight: .medium))
                                        .foregroundColor(workstreamColor(ws.state))
                                        .padding(.horizontal, 6)
                                        .padding(.vertical, 2)
                                        .background(workstreamColor(ws.state).opacity(0.12))
                                        .clipShape(Capsule())
                                }
                                if let note = ws.note, !note.isEmpty {
                                    Text(note)
                                        .font(.system(size: 12))
                                        .foregroundColor(SwarmTheme.muted)
                                }
                                if !ws.owners.isEmpty {
                                    Text("负责人: \(ws.owners.joined(separator: ", "))")
                                        .font(.system(size: 11))
                                        .foregroundColor(SwarmTheme.faint)
                                }
                            }
                            .padding(10)
                            .frame(maxWidth: .infinity, alignment: .leading)
                            .background(SwarmTheme.surfaceBg)
                            .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
                        }
                    }
                    .padding(14)
                    .background(SwarmTheme.cardBg)
                    .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
                    .overlay(RoundedRectangle(cornerRadius: 12, style: .continuous).stroke(SwarmTheme.border, lineWidth: 0.7))
                }
            }
            .padding(16)
        }
        .refreshable {
            await loadSnapshot(silent: false)
        }
    }

    private func statBox(title: String, value: String, icon: String, color: Color) -> some View {
        VStack(spacing: 4) {
            Image(systemName: icon)
                .font(.system(size: 15))
                .foregroundColor(color)
            Text(value)
                .font(.system(size: 14.5, weight: .bold))
                .foregroundColor(SwarmTheme.ink)
            Text(title)
                .font(.system(size: 11))
                .foregroundColor(SwarmTheme.muted)
        }
        .frame(maxWidth: .infinity)
        .padding(.vertical, 10)
        .background(color.opacity(0.09))
        .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
    }

    // MARK: - Tab 2: Board (Native Replication of Desktop Board Feed)

    @ViewBuilder
    private func boardView(_ snap: SwarmSnapshot) -> some View {
        let filtered = filteredBoardMessages(snap.messages)
        let agents = Array(Set(snap.messages.map(\.agent))).sorted { a, b in
            if a == "coordinator" { return true }
            if b == "coordinator" { return false }
            return a < b
        }

        VStack(spacing: 0) {
            // Search + Filter row matching desktop Board
            HStack(spacing: 8) {
                HStack(spacing: 7) {
                    Image(systemName: "magnifyingglass")
                        .font(.system(size: 13))
                        .foregroundColor(SwarmTheme.faint)
                    TextField("Search", text: $boardSearch)
                        .font(.system(size: 13.5))
                        .foregroundColor(SwarmTheme.ink)
                    if !boardSearch.isEmpty {
                        Button(action: { boardSearch = "" }) {
                            Image(systemName: "xmark.circle.fill")
                                .font(.system(size: 13))
                                .foregroundColor(SwarmTheme.faint)
                        }
                    }
                }
                .padding(.horizontal, 10)
                .padding(.vertical, 7.5)
                .background(SwarmTheme.fieldBg)
                .clipShape(RoundedRectangle(cornerRadius: 9, style: .continuous))

                Menu {
                    Button("全部成员") { boardAgentFilter = nil }
                    Button("仅看 @all 广播") { boardAgentFilter = "@all" }
                    Button("未指定接收者") { boardAgentFilter = "@none" }
                    if !agents.isEmpty {
                        Divider()
                        ForEach(agents, id: \.self) { ag in
                            Button("@\(ag)") { boardAgentFilter = ag }
                        }
                    }
                } label: {
                    Image(systemName: boardAgentFilter == nil ? "line.3.horizontal.decrease" : "line.3.horizontal.decrease.circle.fill")
                        .font(.system(size: 15))
                        .foregroundColor(boardAgentFilter == nil ? SwarmTheme.muted : SwarmTheme.accent)
                        .frame(width: 34, height: 34)
                        .background(SwarmTheme.fieldBg)
                        .clipShape(RoundedRectangle(cornerRadius: 9, style: .continuous))
                }
            }
            .padding(.horizontal, 14)
            .padding(.vertical, 10)

            if let activeFilter = boardAgentFilter {
                HStack(spacing: 6) {
                    Text("筛选: \(activeFilter)")
                        .font(.system(size: 11.5, weight: .medium))
                        .foregroundColor(SwarmTheme.accent)
                    Button("清除") { boardAgentFilter = nil }
                        .font(.system(size: 11.5, weight: .semibold))
                        .foregroundColor(SwarmTheme.muted)
                    Spacer()
                }
                .padding(.horizontal, 16)
                .padding(.bottom, 6)
            }

            if filtered.isEmpty {
                VStack(spacing: 10) {
                    Spacer()
                    Image(systemName: "bubble.left.and.bubble.right")
                        .font(.system(size: 36))
                        .foregroundColor(SwarmTheme.faint)
                    Text(snap.messages.isEmpty ? "暂无协作留言" : "没有匹配筛选条件的消息")
                        .font(.system(size: 14, weight: .medium))
                        .foregroundColor(SwarmTheme.muted)
                    Spacer()
                }
            } else {
                ScrollView {
                    LazyVStack(alignment: .leading, spacing: 0) {
                        ForEach(filtered) { msg in
                            boardMessageRow(msg)
                            Divider()
                                .overlay(SwarmTheme.border.opacity(0.6))
                                .padding(.leading, 56)
                        }
                    }
                    .padding(.bottom, 16)
                }
                .refreshable {
                    await loadSnapshot(silent: false)
                }
            }
        }
    }

    private func filteredBoardMessages(_ messages: [SwarmBoardMessage]) -> [SwarmBoardMessage] {
        let q = boardSearch.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        return messages.filter { msg in
            if let f = boardAgentFilter {
                if f == "@all" {
                    if !SwarmHelpers.isBroadcast(msg) { return false }
                } else if f == "@none" {
                    if !msg.mentions.isEmpty { return false }
                } else {
                    let matchSender = msg.agent.caseInsensitiveCompare(f) == .orderedSame
                    let matchMention = msg.mentions.contains { $0.caseInsensitiveCompare(f) == .orderedSame }
                    if !matchSender && !matchMention { return false }
                }
            }
            if !q.isEmpty {
                let hay = "\(msg.agent) \(msg.mentions.joined(separator: " ")) \(msg.text)".lowercased()
                if !hay.contains(q) { return false }
            }
            return true
        }
    }

    private func boardMessageRow(_ msg: SwarmBoardMessage) -> some View {
        let targets = SwarmHelpers.targets(of: msg)
        let bodyText = targets.isEmpty ? msg.text : SwarmHelpers.stripLeadingMentions(msg.text)
        let isExpanded = expandedSeqs.contains(msg.seq)
        let needsClamp = bodyText.count > 160 || bodyText.filter({ $0 == "\n" }).count >= 4

        return HStack(alignment: .top, spacing: 11) {
            SwarmPixelBeeView(
                name: msg.agent,
                pose: .landed,
                size: 30
            )
            .padding(.top, 2)

            VStack(alignment: .leading, spacing: 4) {
                HStack(alignment: .firstTextBaseline, spacing: 5) {
                    Text("@\(msg.agent)")
                        .font(.system(size: 13.5, weight: .semibold))
                        .foregroundColor(SwarmTheme.ink)

                    if !targets.isEmpty {
                        Text("→")
                            .font(.system(size: 12))
                            .foregroundColor(SwarmTheme.faint)
                        Text(targets.map { "@\($0)" }.joined(separator: " "))
                            .font(.system(size: 13, weight: .regular))
                            .foregroundColor(SwarmTheme.muted)
                            .lineLimit(1)
                    }

                    Spacer(minLength: 6)

                    Text("#\(msg.seq) · \(SwarmHelpers.formatClock(msg.time))")
                        .font(.system(size: 11.5, design: .monospaced))
                        .foregroundColor(SwarmTheme.faint)
                        .lineLimit(1)
                }

                Text(SwarmHelpers.inlineAttributed(bodyText, baseSize: 13))
                    .lineSpacing(2.5)
                    .lineLimit((needsClamp && !isExpanded) ? 4 : nil)
                    .fixedSize(horizontal: false, vertical: true)

                if needsClamp {
                    Button(action: {
                        if isExpanded {
                            expandedSeqs.remove(msg.seq)
                        } else {
                            expandedSeqs.insert(msg.seq)
                        }
                    }) {
                        Text(isExpanded ? "收起" : "展开更多")
                            .font(.system(size: 11.5, weight: .medium))
                            .foregroundColor(SwarmTheme.accent)
                    }
                    .buttonStyle(.plain)
                    .padding(.top, 2)
                }
            }
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 10)
    }

    // MARK: - Tab 3: Members (Native Replication of Desktop Members Roster)

    @ViewBuilder
    private func membersView(_ snap: SwarmSnapshot) -> some View {
        let q = memberSearch.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()

        let matching: [SwarmMemberInfo] = snap.members.filter { m in
            guard !q.isEmpty else { return true }
            let roleHint = SwarmHelpers.extractRoleHint(for: m.name, role: m.role, messages: snap.messages)
            let hay = "\(m.name) \(m.role) \(roleHint) \(m.model) \(m.lastActivity ?? "")".lowercased()
            return hay.contains(q)
        }

        let coordinator = matching.first(where: { $0.name == "coordinator" || $0.role == "coordinator" })
        let workers = matching.filter { $0.name != "coordinator" && $0.role != "coordinator" }
        let liveWorkers = workers.filter { !$0.killed }
        let activeWorkers = liveWorkers.filter {
            let st = $0.status.lowercased()
            return st == "working" || st == "error" || st == "stuck"
        }
        let standbyWorkers = liveWorkers.filter {
            let st = $0.status.lowercased()
            return st != "working" && st != "error" && st != "stuck"
        }.sorted { a, b in
            let seqA = snap.messages.last(where: { $0.agent == a.name })?.seq ?? 0
            let seqB = snap.messages.last(where: { $0.agent == b.name })?.seq ?? 0
            if seqA != seqB { return seqA > seqB }
            return a.name < b.name
        }
        let retiredWorkers = workers.filter { $0.killed }

        VStack(spacing: 0) {
            // Search members bar
            HStack(spacing: 7) {
                Image(systemName: "magnifyingglass")
                    .font(.system(size: 13))
                    .foregroundColor(SwarmTheme.faint)
                TextField("Search members", text: $memberSearch)
                    .font(.system(size: 13.5))
                    .foregroundColor(SwarmTheme.ink)
                if !memberSearch.isEmpty {
                    Button(action: { memberSearch = "" }) {
                        Image(systemName: "xmark.circle.fill")
                            .font(.system(size: 13))
                            .foregroundColor(SwarmTheme.faint)
                    }
                }
            }
            .padding(.horizontal, 10)
            .padding(.vertical, 7.5)
            .background(SwarmTheme.fieldBg)
            .clipShape(RoundedRectangle(cornerRadius: 9, style: .continuous))
            .padding(.horizontal, 14)
            .padding(.vertical, 10)

            if matching.isEmpty {
                VStack(spacing: 10) {
                    Spacer()
                    Image(systemName: "person.2")
                        .font(.system(size: 36))
                        .foregroundColor(SwarmTheme.faint)
                    Text("未找到匹配的成员")
                        .font(.system(size: 14, weight: .medium))
                        .foregroundColor(SwarmTheme.muted)
                    Spacer()
                }
            } else {
                ScrollView {
                    LazyVStack(alignment: .leading, spacing: 14) {
                        // Pinned Coordinator
                        if let coord = coordinator {
                            memberRow(coord, snap: snap, highlightedCard: false)
                                .padding(.horizontal, 14)
                        }

                        // ACTIVE group
                        if !activeWorkers.isEmpty {
                            VStack(alignment: .leading, spacing: 8) {
                                memberSectionHeader(title: "ACTIVE", count: activeWorkers.count)
                                ForEach(activeWorkers) { m in
                                    memberRow(m, snap: snap, highlightedCard: true)
                                }
                            }
                            .padding(.horizontal, 14)
                        }

                        // STANDING BY group
                        if !standbyWorkers.isEmpty {
                            VStack(alignment: .leading, spacing: 6) {
                                memberSectionHeader(title: "STANDING BY", count: standbyWorkers.count)
                                ForEach(standbyWorkers) { m in
                                    memberRow(m, snap: snap, highlightedCard: false)
                                }
                            }
                            .padding(.horizontal, 14)
                        }

                        // RETIRED collapsible group
                        if !retiredWorkers.isEmpty {
                            VStack(alignment: .leading, spacing: 6) {
                                Button(action: {
                                    withAnimation(.easeInOut(duration: 0.18)) {
                                        showRetiredMembers.toggle()
                                    }
                                }) {
                                    HStack(spacing: 6) {
                                        Text("RETIRED")
                                            .font(.system(size: 11, weight: .bold))
                                            .tracking(0.6)
                                            .foregroundColor(SwarmTheme.faint)
                                        Text("\(retiredWorkers.count)")
                                            .font(.system(size: 11, weight: .semibold, design: .monospaced))
                                            .foregroundColor(SwarmTheme.faint)
                                        Image(systemName: showRetiredMembers ? "chevron.down" : "chevron.right")
                                            .font(.system(size: 10, weight: .semibold))
                                            .foregroundColor(SwarmTheme.faint)
                                        Spacer()
                                    }
                                    .padding(.vertical, 4)
                                }
                                .buttonStyle(.plain)

                                if showRetiredMembers {
                                    ForEach(retiredWorkers) { m in
                                        memberRow(m, snap: snap, highlightedCard: false)
                                    }
                                }
                            }
                            .padding(.horizontal, 14)
                        }
                    }
                    .padding(.top, 4)
                    .padding(.bottom, 20)
                }
                .refreshable {
                    await loadSnapshot(silent: false)
                }
            }
        }
    }

    private func memberSectionHeader(title: String, count: Int) -> some View {
        HStack(spacing: 6) {
            Text(title)
                .font(.system(size: 11, weight: .bold))
                .tracking(0.6)
                .foregroundColor(SwarmTheme.faint)
            Text("\(count)")
                .font(.system(size: 11, weight: .medium, design: .monospaced))
                .foregroundColor(SwarmTheme.faint)
            Spacer()
        }
        .padding(.top, 4)
    }

    private func memberRow(_ member: SwarmMemberInfo, snap: SwarmSnapshot, highlightedCard: Bool) -> some View {
        let isCoord = (member.name == "coordinator" || member.role == "coordinator")
        let st = member.status.lowercased()
        let isWorking = !member.killed && st == "working"
        let isError = !member.killed && (st == "error" || st == "stuck")
        let pose: PixelBeePose = member.killed ? .landed : (isWorking ? .working : .asleep)
        let dotColor: Color? = isWorking ? SwarmTheme.liveGreen : (isError ? SwarmTheme.errorRed : nil)

        let lastPost = snap.messages.last(where: { $0.agent == member.name })
        let shortModel = SwarmHelpers.shortModel(member.model)
        let roleHint = SwarmHelpers.extractRoleHint(for: member.name, role: member.role, messages: snap.messages)
        let showActions = selectedMemberForActions == member.name

        let bodyPreview: String = {
            if isWorking, let act = member.lastActivity, !act.isEmpty {
                return act
            }
            if isError, let act = member.lastActivity, !act.isEmpty {
                return act
            }
            if let post = lastPost {
                return SwarmHelpers.stripLeadingMentions(post.text)
                    .replacingOccurrences(of: "\n", with: " ")
            }
            return member.lastActivity ?? ""
        }()

        return VStack(alignment: .leading, spacing: 8) {
            HStack(alignment: .top, spacing: 11) {
                SwarmPixelBeeView(
                    name: member.name,
                    pose: pose,
                    size: 36,
                    statusDotColor: dotColor
                )

                VStack(alignment: .leading, spacing: 3) {
                    // Header line: @name + status/role + right model · #seq
                    HStack(alignment: .firstTextBaseline, spacing: 6) {
                        Text("@\(member.name)")
                            .font(.system(size: 13.5, weight: .semibold))
                            .foregroundColor(member.killed ? SwarmTheme.faint : SwarmTheme.ink)
                            .lineLimit(1)

                        if isCoord {
                            Text(isWorking ? "Coordinator · Working" : "Coordinator")
                                .font(.system(size: 12, weight: .medium))
                                .foregroundColor(isWorking ? SwarmTheme.accent : SwarmTheme.muted)
                                .lineLimit(1)
                        } else if member.killed {
                            Text("Retired")
                                .font(.system(size: 11.5, weight: .medium))
                                .foregroundColor(SwarmTheme.faint)
                        } else if isWorking {
                            Text("Working")
                                .font(.system(size: 12, weight: .semibold))
                                .foregroundColor(SwarmTheme.accent)
                        } else if isError {
                            Text("Error")
                                .font(.system(size: 12, weight: .semibold))
                                .foregroundColor(SwarmTheme.errorRed)
                        } else {
                            Text(roleHint)
                                .font(.system(size: 12))
                                .foregroundColor(SwarmTheme.faint)
                                .lineLimit(1)
                        }

                        if member.isCurrent {
                            Text("当前")
                                .font(.system(size: 10, weight: .semibold))
                                .foregroundColor(SwarmTheme.infoBlue)
                                .padding(.horizontal, 5)
                                .padding(.vertical, 1)
                                .background(SwarmTheme.infoBlue.opacity(0.12))
                                .clipShape(Capsule())
                        }

                        Spacer(minLength: 4)

                        let metaText: String = {
                            var parts: [String] = []
                            if !shortModel.isEmpty { parts.append(shortModel) }
                            if let seq = lastPost?.seq { parts.append("#\(seq)") }
                            return parts.joined(separator: " · ")
                        }()
                        if !metaText.isEmpty {
                            Text(metaText)
                                .font(.system(size: 11, design: .monospaced))
                                .foregroundColor(SwarmTheme.faint)
                                .lineLimit(1)
                        }
                    }

                    // For active workers, show brief ownership line above activity (matching screenshot 2)
                    if (isWorking || isError) && !isCoord {
                        Text(roleHint)
                            .font(.system(size: 12))
                            .foregroundColor(SwarmTheme.faint)
                            .lineLimit(1)
                    }

                    if !bodyPreview.isEmpty {
                        Text(bodyPreview)
                            .font(.system(size: 12.5))
                            .foregroundColor((isWorking || isCoord) ? SwarmTheme.ink.opacity(0.88) : SwarmTheme.muted)
                            .lineLimit(2)
                    }
                }
            }

            if showActions {
                HStack(spacing: 10) {
                    Button(action: { nudgeMember(member.name) }) {
                        HStack(spacing: 4) {
                            if nudgingMembers.contains(member.name) {
                                ProgressView().scaleEffect(0.65)
                            } else {
                                Image(systemName: "bolt.fill").font(.system(size: 10.5))
                            }
                            Text("催促 / 唤醒")
                                .font(.system(size: 11.5, weight: .semibold))
                        }
                        .foregroundColor(SwarmTheme.accent)
                        .padding(.horizontal, 10)
                        .padding(.vertical, 5)
                        .background(SwarmTheme.accent.opacity(0.12))
                        .clipShape(Capsule())
                    }
                    .buttonStyle(.plain)
                    .disabled(nudgingMembers.contains(member.name) || member.killed)

                    if !member.conversationId.isEmpty && !member.isCurrent {
                        Button(action: {
                            dismiss()
                            onSelectConversation?(member.conversationId)
                        }) {
                            HStack(spacing: 4) {
                                Image(systemName: "arrow.up.right.square").font(.system(size: 10.5))
                                Text("进入子会话")
                                    .font(.system(size: 11.5, weight: .semibold))
                            }
                            .foregroundColor(SwarmTheme.infoBlue)
                            .padding(.horizontal, 10)
                            .padding(.vertical, 5)
                            .background(SwarmTheme.infoBlue.opacity(0.12))
                            .clipShape(Capsule())
                        }
                        .buttonStyle(.plain)
                    }

                    Spacer()

                    if member.unreadCount > 0 {
                        Text("未读 \(member.unreadCount)")
                            .font(.system(size: 11))
                            .foregroundColor(SwarmTheme.faint)
                    }
                }
                .padding(.leading, 47)
                .transition(.opacity)
            }
        }
        .padding(.horizontal, highlightedCard ? 12 : 4)
        .padding(.vertical, highlightedCard ? 10 : 6)
        .background(
            RoundedRectangle(cornerRadius: 10, style: .continuous)
                .fill(highlightedCard ? SwarmTheme.cardBg : Color.clear)
        )
        .overlay(
            RoundedRectangle(cornerRadius: 10, style: .continuous)
                .stroke(highlightedCard ? SwarmTheme.border : Color.clear, lineWidth: 0.7)
        )
        .contentShape(Rectangle())
        .onTapGesture {
            UIImpactFeedbackGenerator(style: .light).impactOccurred()
            withAnimation(.easeInOut(duration: 0.15)) {
                selectedMemberForActions = (selectedMemberForActions == member.name) ? nil : member.name
            }
        }
    }

    // MARK: - Helpers & Actions

    private var emptyStateView: some View {
        VStack(spacing: 12) {
            Spacer()
            Image(systemName: "circle.hexagongrid")
                .font(.system(size: 42))
                .foregroundColor(SwarmTheme.faint)
            Text("未检测到当前会话的蜂群协作信息")
                .font(.system(size: 14.5, weight: .medium))
                .foregroundColor(SwarmTheme.muted)
            Spacer()
        }
    }

    private func errorView(_ msg: String) -> some View {
        VStack(spacing: 12) {
            Spacer()
            Image(systemName: "exclamationmark.triangle")
                .font(.system(size: 38))
                .foregroundColor(SwarmTheme.warnAmber)
            Text("加载蜂群快照失败")
                .font(.system(size: 15.5, weight: .semibold))
                .foregroundColor(SwarmTheme.ink)
            Text(msg)
                .font(.system(size: 12.5))
                .foregroundColor(SwarmTheme.muted)
                .multilineTextAlignment(.center)
                .padding(.horizontal, 32)
            Button("重试") {
                Task { await loadSnapshot(silent: false) }
            }
            .foregroundColor(.white)
            .padding(.horizontal, 18)
            .padding(.vertical, 8)
            .background(SwarmTheme.accent)
            .clipShape(Capsule())
            Spacer()
        }
    }

    private func attentionColor(_ severity: String) -> Color {
        switch severity.lowercased() {
        case "high": return SwarmTheme.errorRed
        case "low": return SwarmTheme.infoBlue
        default: return SwarmTheme.warnAmber
        }
    }

    private func workstreamColor(_ state: String) -> Color {
        switch state.lowercased() {
        case "done": return SwarmTheme.liveGreen
        case "blocked", "failed": return SwarmTheme.errorRed
        case "review": return SwarmTheme.warnAmber
        default: return SwarmTheme.infoBlue
        }
    }

    private func showToast(_ msg: String) {
        toastMessage = msg
        DispatchQueue.main.asyncAfter(deadline: .now() + 2.5) {
            if toastMessage == msg {
                toastMessage = nil
            }
        }
    }

    private func loadSnapshot(silent: Bool) async {
        guard let url = settings.serverURL else {
            if !silent { errorMessage = "未连接网关" }
            return
        }
        if !silent {
            isLoading = true
            errorMessage = nil
        }
        do {
            let snap = try await APIClient.shared.fetchSwarmSnapshot(cascadeId: cascadeId, baseURL: url)
            self.snapshot = snap
            self.errorMessage = nil
        } catch {
            if !silent || self.snapshot == nil {
                self.errorMessage = error.localizedDescription
            }
        }
        if !silent {
            isLoading = false
        }
    }

    private func sendFeedback() {
        guard let url = settings.serverURL else { return }
        let text = feedbackText.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !text.isEmpty else { return }

        isSendingFeedback = true
        let coordId = snapshot?.coordinatorId.isEmpty == false ? snapshot?.coordinatorId : nil
        Task {
            do {
                try await APIClient.shared.sendSwarmFeedback(
                    cascadeId: cascadeId,
                    coordinatorId: coordId,
                    content: text,
                    baseURL: url
                )
                UIImpactFeedbackGenerator(style: .medium).impactOccurred()
                feedbackText = ""
                showToast("反馈已发送至协调员")
                await loadSnapshot(silent: true)
            } catch {
                showToast("发送失败: \(error.localizedDescription)")
            }
            isSendingFeedback = false
        }
    }

    private func nudgeMember(_ member: String) {
        guard let url = settings.serverURL else { return }
        nudgingMembers.insert(member)
        UIImpactFeedbackGenerator(style: .medium).impactOccurred()
        let coordId = snapshot?.coordinatorId.isEmpty == false ? snapshot?.coordinatorId : nil

        Task {
            do {
                try await APIClient.shared.nudgeSwarmMember(
                    cascadeId: cascadeId,
                    coordinatorId: coordId,
                    member: member,
                    baseURL: url
                )
                UINotificationFeedbackGenerator().notificationOccurred(.success)
                showToast("已催促唤醒 @\(member)")
                await loadSnapshot(silent: true)
            } catch {
                showToast("催促失败: \(error.localizedDescription)")
            }
            nudgingMembers.remove(member)
        }
    }
}
