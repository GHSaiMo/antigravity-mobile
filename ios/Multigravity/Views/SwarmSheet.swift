import SwiftUI

public struct SwarmSheet: View {
    @Environment(\.dismiss) private var dismiss
    @Environment(\.scenePhase) private var scenePhase
    @Namespace private var tabIndicatorNamespace
    @State private var settings = AppSettings.shared

    public let cascadeId: String
    public let onSelectConversation: ((String) -> Void)?

    @State private var selectedTab: Tab = .overview
    @State private var snapshot: SwarmSnapshot? = nil
    @State private var isLoading: Bool = false
    @State private var isLoadingEarlier: Bool = false
    @State private var lastETag: String? = nil
    @State private var errorMessage: String? = nil
    private let swarmPageSize = 80

    // Overview feedback & interactive state
    @State private var feedbackText: String = ""
    @State private var isSendingFeedback: Bool = false
    @State private var nudgingMembers: Set<String> = []
    @State private var toastMessage: String? = nil
    @State private var overviewFilterMember: String? = nil
    @State private var showAllRunningTasks: Bool = false
    @State private var showDoneTasks: Bool = false
    @State private var expandedThreadIds: Set<String> = []

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
        public var localizedTitle: String {
            switch self {
            case .overview: return String(localized: "概览")
            case .board: return String(localized: "协作板")
            case .members: return String(localized: "成员")
            case .graph: return String(localized: "拓扑")
            }
        }
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
            // Live auto-refresh every 3 seconds while sheet is open and app is foreground active
            while !Task.isCancelled {
                try? await Task.sleep(nanoseconds: 3_000_000_000)
                if Task.isCancelled { break }
                if scenePhase != .active { continue }
                await loadSnapshot(silent: true)
            }
        }
    }

    // MARK: - Header & Desktop-Style Underline Tab Bar (No Top-Right "完成")

    private var swarmHeader: some View {
        HStack(alignment: .center, spacing: 10) {
            Text(snapshot?.title.isEmpty == false ? snapshot!.title : String(localized: "蜂群协同 (Swarm)"))
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
        .padding(.top, 28)
        .padding(.bottom, 14)
    }

    private var swarmTabBar: some View {
        let boardCount = snapshot?.messages.filter { !$0.text.hasPrefix("[channel]") }.count ?? 0
        let memberCount = snapshot?.members.filter { !$0.roleWorker && !SwarmHelpers.isRoleWorker(name: $0.name, role: $0.role) }.count ?? 0

        return HStack(spacing: 0) {
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
                    withAnimation(.easeInOut(duration: 0.18)) {
                        selectedTab = tab
                    }
                }) {
                    HStack(spacing: 4) {
                        Text(tab.localizedTitle)
                            .font(.system(size: 14, weight: isSelected ? .semibold : .medium))
                            .foregroundColor(isSelected ? SwarmTheme.ink : SwarmTheme.muted)
                            .lineLimit(1)
                            .minimumScaleFactor(0.8)

                        if let countText {
                            Text(countText)
                                .font(.system(size: 12.5, weight: .regular, design: .monospaced))
                                .foregroundColor(isSelected ? SwarmTheme.muted : SwarmTheme.faint)
                                .lineLimit(1)
                                .minimumScaleFactor(0.8)
                                .layoutPriority(1)
                        }
                    }
                    .padding(.horizontal, 4)
                    .padding(.top, 4)
                    .padding(.bottom, 12)
                    .overlay(alignment: .bottom) {
                        if isSelected {
                            Capsule()
                                .fill(SwarmTheme.accent)
                                .frame(height: 2)
                                .matchedGeometryEffect(id: "swarmActiveTabIndicator", in: tabIndicatorNamespace)
                        }
                    }
                    .frame(minWidth: 0, maxWidth: .infinity)
                    .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
            }
        }
        .padding(.horizontal, 4)
    }

    // MARK: - Overview Tab (Goal, Running, Progress Ring, Threads, The Team + Feedback)

    @ViewBuilder
    private func overviewView(_ snap: SwarmSnapshot) -> some View {
        let synth = SwarmHiveEngine.synthesize(snap, filterMember: overviewFilterMember)
        let runningItems = synth.tasks.filter { $0.state == .running || $0.state == .blocked }
            .sorted { ($0.state == .blocked ? 0 : 1) < ($1.state == .blocked ? 0 : 1) }
        let doneItems = synth.tasks.filter { $0.state == .done }.sorted { $0.seq > $1.seq }
        let queuedItems = synth.tasks.filter { $0.state == .queued }

        ScrollView {
            VStack(alignment: .leading, spacing: 14) {
                // 1. Stall Banner (if @monitor flagged [stall])
                if let stallText = synth.stallText, let stallSeq = synth.stallSeq {
                    VStack(alignment: .leading, spacing: 5) {
                        HStack(spacing: 7) {
                            Circle()
                                .fill(SwarmTheme.warnAmber)
                                .frame(width: 8, height: 8)
                            Text("Swarm stalled — no agents working")
                                .font(.system(size: 13, weight: .semibold))
                                .foregroundColor(SwarmTheme.ink)
                            Spacer()
                            Button(action: {
                                selectedTab = .board
                            }) {
                                Text("@\(synth.stallAgent ?? "monitor") #\(stallSeq)")
                                    .font(.system(size: 11, weight: .medium, design: .monospaced))
                                    .foregroundColor(SwarmTheme.warnAmber)
                                    .padding(.horizontal, 7)
                                    .padding(.vertical, 2)
                                    .background(SwarmTheme.warnAmber.opacity(0.16))
                                    .clipShape(Capsule())
                            }
                            .buttonStyle(.plain)
                        }
                        Text(stallText)
                            .font(.system(size: 12))
                            .foregroundColor(SwarmTheme.muted)
                            .padding(.leading, 15)
                    }
                    .padding(12)
                    .background(SwarmTheme.warnAmber.opacity(0.10))
                    .clipShape(RoundedRectangle(cornerRadius: 11, style: .continuous))
                    .overlay(RoundedRectangle(cornerRadius: 11, style: .continuous).stroke(SwarmTheme.warnAmber.opacity(0.45), lineWidth: 0.8))
                }

                // 2. GOAL Card
                if !synth.goalTitle.isEmpty || !synth.goalLine.isEmpty {
                    VStack(alignment: .leading, spacing: 4) {
                        Text("GOAL")
                            .font(.system(size: 11, weight: .semibold))
                            .tracking(0.6)
                            .foregroundColor(SwarmTheme.muted)
                        if !synth.goalTitle.isEmpty {
                            Text(synth.goalTitle)
                                .font(.system(size: 15.5, weight: .semibold))
                                .foregroundColor(SwarmTheme.ink)
                                .lineSpacing(2)
                        }
                        if !synth.goalLine.isEmpty {
                            Text(synth.goalLine)
                                .font(.system(size: 13))
                                .foregroundColor(SwarmTheme.muted)
                                .lineSpacing(2)
                        }
                    }
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .padding(.horizontal, 14)
                    .padding(.vertical, 12)
                    .background(SwarmTheme.surfaceBg)
                    .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
                    .overlay(RoundedRectangle(cornerRadius: 12, style: .continuous).stroke(SwarmTheme.border, lineWidth: 0.8))
                }

                // 3. Running Tasks Card
                if !runningItems.isEmpty && !synth.finished {
                    let visibleRun = showAllRunningTasks ? runningItems : Array(runningItems.prefix(8))
                    let extraRun = max(0, runningItems.count - visibleRun.count)

                    VStack(alignment: .leading, spacing: 0) {
                        HStack(spacing: 8) {
                            Circle()
                                .fill(SwarmTheme.accent)
                                .frame(width: 8, height: 8)
                                .shadow(color: SwarmTheme.accent.opacity(0.45), radius: 3)
                            Text("Running")
                                .font(.system(size: 13.5, weight: .semibold))
                                .foregroundColor(SwarmTheme.ink)
                            Text("\(runningItems.count)")
                                .font(.system(size: 12.5, weight: .regular, design: .monospaced))
                                .foregroundColor(SwarmTheme.muted)
                            Spacer()
                            if let ph = synth.phaseLabel, !ph.isEmpty {
                                Text(ph)
                                    .font(.system(size: 11, weight: .medium))
                                    .foregroundColor(SwarmTheme.muted)
                                    .padding(.horizontal, 8)
                                    .padding(.vertical, 2)
                                    .background(SwarmTheme.fieldBg)
                                    .clipShape(Capsule())
                            }
                        }
                        .padding(.bottom, 8)

                        if !synth.runningSummary.isEmpty {
                            Text(synth.runningSummary)
                                .font(.system(size: 12.5))
                                .foregroundColor(SwarmTheme.muted)
                                .padding(.bottom, 8)
                        }

                        ForEach(Array(visibleRun.enumerated()), id: \.element.id) { idx, task in
                            if idx > 0 {
                                Divider()
                                    .overlay(SwarmTheme.border.opacity(0.65))
                            }
                            overviewRunningTaskRow(task, synth: synth, snap: snap)
                                .padding(.vertical, 8.5)
                        }

                        if extraRun > 0 {
                            Button(action: {
                                withAnimation(.easeInOut(duration: 0.18)) { showAllRunningTasks = true }
                            }) {
                                Text("+\(extraRun) more")
                                    .font(.system(size: 12, weight: .medium))
                                    .foregroundColor(SwarmTheme.muted)
                                    .padding(.top, 4)
                            }
                            .buttonStyle(.plain)
                        }
                    }
                    .padding(.horizontal, 14)
                    .padding(.vertical, 12)
                    .background(SwarmTheme.surfaceBg)
                    .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
                    .overlay(RoundedRectangle(cornerRadius: 12, style: .continuous).stroke(SwarmTheme.border, lineWidth: 0.8))
                }

                // 4. Compact Expandable "X of Y tasks done" Progress Card
                if synth.totalTasksCount > 0 {
                    VStack(alignment: .leading, spacing: 0) {
                        Button(action: {
                            UIImpactFeedbackGenerator(style: .light).impactOccurred()
                            withAnimation(.easeInOut(duration: 0.18)) {
                                showDoneTasks.toggle()
                            }
                        }) {
                            HStack(spacing: 9) {
                                OverviewProgressRingView(
                                    done: synth.doneCount,
                                    running: synth.runningCount,
                                    blocked: synth.blockedCount,
                                    total: synth.totalTasksCount
                                )

                                HStack(spacing: 4) {
                                    Text("\(synth.doneCount) of \(synth.totalTasksCount)")
                                        .font(.system(size: 13, weight: .semibold, design: .rounded))
                                        .foregroundColor(SwarmTheme.ink)
                                    Text("tasks done")
                                        .font(.system(size: 13))
                                        .foregroundColor(SwarmTheme.muted)
                                }

                                Spacer(minLength: 6)

                                HStack(spacing: 8) {
                                    if synth.stuckCount > 0 {
                                        progressStatusChip(text: String(localized: "\(synth.stuckCount) stuck"), color: SwarmTheme.errorRed)
                                    } else if let waitFirst = synth.waitingChips.first {
                                        progressStatusChip(text: waitFirst, color: SwarmTheme.warnAmber)
                                    }
                                    if synth.reviewCount > 0 {
                                        progressStatusChip(text: String(localized: "\(synth.reviewCount) in review"), color: SwarmTheme.accent)
                                    }
                                    if synth.openThreadsCount > 0 {
                                        Text(synth.openThreadsCount == 1 ? String(localized: "1 thread open") : String(localized: "\(synth.openThreadsCount) threads open"))
                                            .font(.system(size: 12))
                                            .foregroundColor(SwarmTheme.muted)
                                            .lineLimit(1)
                                    }
                                    Image(systemName: showDoneTasks ? "chevron.up" : "chevron.down")
                                        .font(.system(size: 10.5, weight: .semibold))
                                        .foregroundColor(SwarmTheme.faint)
                                }
                            }
                            .padding(.horizontal, 13)
                            .padding(.vertical, 10.5)
                            .contentShape(Rectangle())
                        }
                        .buttonStyle(.plain)

                        if showDoneTasks {
                            Divider()
                                .overlay(SwarmTheme.border)
                            VStack(alignment: .leading, spacing: 6) {
                                if doneItems.isEmpty {
                                    Text("No tasks completed yet.")
                                        .font(.system(size: 12.5))
                                        .foregroundColor(SwarmTheme.muted)
                                        .padding(.vertical, 4)
                                } else {
                                    ForEach(doneItems) { item in
                                        HStack(spacing: 8) {
                                            OverviewTaskIndicator(state: .done)
                                            VStack(alignment: .leading, spacing: 1) {
                                                Text(item.title)
                                                    .font(.system(size: 12.5, weight: .medium))
                                                    .foregroundColor(SwarmTheme.ink)
                                                    .lineLimit(2)
                                                if !item.note.isEmpty {
                                                    Text(item.note)
                                                        .font(.system(size: 11.5))
                                                        .foregroundColor(SwarmTheme.muted)
                                                        .lineLimit(1)
                                                }
                                            }
                                            Spacer(minLength: 4)
                                            workerBeeChips(owners: item.owners, synth: synth, snap: snap)
                                        }
                                        .padding(.vertical, 3)
                                    }
                                }

                                if !queuedItems.isEmpty {
                                    Divider()
                                        .overlay(SwarmTheme.border.opacity(0.6))
                                        .padding(.vertical, 2)
                                    Text("UP NEXT · \(queuedItems.count)")
                                        .font(.system(size: 10.5, weight: .semibold))
                                        .tracking(0.5)
                                        .foregroundColor(SwarmTheme.muted)
                                    ForEach(queuedItems.prefix(6)) { item in
                                        HStack(spacing: 8) {
                                            OverviewTaskIndicator(state: .queued)
                                            Text(item.title)
                                                .font(.system(size: 12.5, weight: .medium))
                                                .foregroundColor(SwarmTheme.ink)
                                                .lineLimit(1)
                                            Spacer(minLength: 4)
                                            workerBeeChips(owners: item.owners, synth: synth, snap: snap)
                                        }
                                        .padding(.vertical, 2)
                                    }
                                }
                            }
                            .padding(.horizontal, 13)
                            .padding(.vertical, 10)
                        }
                    }
                    .background(SwarmTheme.surfaceBg)
                    .clipShape(RoundedRectangle(cornerRadius: 11, style: .continuous))
                    .overlay(RoundedRectangle(cornerRadius: 11, style: .continuous).stroke(SwarmTheme.border, lineWidth: 0.8))
                }

                // 5. Threads Section
                if !synth.threads.isEmpty {
                    VStack(alignment: .leading, spacing: 8) {
                        Text("Threads")
                            .font(.system(size: 13.5, weight: .semibold))
                            .foregroundColor(SwarmTheme.ink)
                            .padding(.top, 2)

                        ForEach(synth.threads) { thread in
                            overviewThreadCard(thread, synth: synth, snap: snap)
                        }
                    }
                }

                // 6. Attention items from view.json (if present)
                if let attentions = snap.view?.attention, !attentions.isEmpty {
                    VStack(alignment: .leading, spacing: 8) {
                        HStack(spacing: 6) {
                            Image(systemName: "exclamationmark.triangle.fill")
                                .font(.system(size: 12.5))
                                .foregroundColor(SwarmTheme.warnAmber)
                            Text("Attention (\(attentions.count))")
                                .font(.system(size: 13.5, weight: .semibold))
                                .foregroundColor(SwarmTheme.ink)
                        }
                        ForEach(attentions) { item in
                            VStack(alignment: .leading, spacing: 3) {
                                HStack {
                                    Text(item.title)
                                        .font(.system(size: 13, weight: .semibold))
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
                            .background(SwarmTheme.surfaceBg)
                            .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
                            .overlay(RoundedRectangle(cornerRadius: 10, style: .continuous).stroke(SwarmTheme.border, lineWidth: 0.7))
                        }
                    }
                }

                // 7. The Team Section (Coordinator Lead Card + Pixel Bee Crew Filter Strip)
                overviewTeamSection(synth: synth, snap: snap)
                    .padding(.top, 2)

                // 8. Mobile Feedback Composer
                VStack(alignment: .leading, spacing: 8) {
                    HStack(spacing: 6) {
                        Image(systemName: "megaphone.fill")
                            .font(.system(size: 12.5))
                            .foregroundColor(SwarmTheme.accent)
                        Text("协同指挥 / 发送反馈")
                            .font(.system(size: 13.5, weight: .semibold))
                            .foregroundColor(SwarmTheme.ink)
                    }

                    HStack(spacing: 8) {
                        TextField("向协调者发送补充指示或反馈...", text: $feedbackText)
                            .font(.system(size: 13))
                            .foregroundColor(SwarmTheme.ink)
                            .padding(.horizontal, 10)
                            .padding(.vertical, 8)
                            .background(SwarmTheme.fieldBg)
                            .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))

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
                .padding(13)
                .background(SwarmTheme.surfaceBg)
                .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
                .overlay(RoundedRectangle(cornerRadius: 12, style: .continuous).stroke(SwarmTheme.border, lineWidth: 0.8))
            }
            .padding(16)
        }
        .refreshable {
            await loadSnapshot(silent: false)
        }
    }

    private func overviewRunningTaskRow(_ task: SwarmSynthesizedTask, synth: SwarmSynthesizedModel, snap: SwarmSnapshot) -> some View {
        let subtitle = task.liveActivity ?? task.note
        return HStack(alignment: .top, spacing: 9) {
            OverviewTaskIndicator(state: task.state)
                .padding(.top, 3)

            VStack(alignment: .leading, spacing: 3) {
                HStack(spacing: 6) {
                    Text(task.title)
                        .font(.system(size: 13.5, weight: .semibold))
                        .foregroundColor(SwarmTheme.ink)
                        .lineLimit(1)

                    Spacer(minLength: 4)

                    if task.rawState == "review" {
                        Text("In review")
                            .font(.system(size: 11, weight: .medium))
                            .foregroundColor(SwarmTheme.accent)
                            .padding(.horizontal, 6.5)
                            .padding(.vertical, 2)
                            .background(SwarmTheme.accent.opacity(0.12))
                            .clipShape(Capsule())
                    } else if let prog = task.progressLabel {
                        Text(prog)
                            .font(.system(size: 11, weight: .medium, design: .monospaced))
                            .foregroundColor(SwarmTheme.muted)
                            .padding(.horizontal, 6)
                            .padding(.vertical, 2)
                            .background(SwarmTheme.fieldBg)
                            .clipShape(Capsule())
                    }

                    workerBeeChips(owners: task.owners, synth: synth, snap: snap)
                }

                if let waitTarget = task.waitingOn {
                    Text("Waiting on @\(waitTarget)")
                        .font(.system(size: 12, weight: .medium))
                        .foregroundColor(SwarmTheme.warnAmber)
                        .lineLimit(1)
                } else if !subtitle.isEmpty {
                    Text(subtitle)
                        .font(.system(size: 12))
                        .foregroundColor(task.liveActivity != nil ? SwarmTheme.muted : SwarmTheme.faint)
                        .lineLimit(1)
                }
            }
        }
    }

    private func overviewThreadCard(_ thread: SwarmSynthesizedThread, synth: SwarmSynthesizedModel, snap: SwarmSnapshot) -> some View {
        let defaultOpen = (thread.state == .needs || thread.state == .waiting || thread.state == .stuck)
        let isOpen = expandedThreadIds.contains(thread.id) || (defaultOpen && !expandedThreadIds.contains("closed:\(thread.id)"))
        let dotColor: Color = {
            switch thread.state {
            case .needs, .waiting: return SwarmTheme.warnAmber
            case .stuck: return SwarmTheme.errorRed
            case .running: return SwarmTheme.accent
            case .done: return SwarmTheme.liveGreen
            case .queued: return SwarmTheme.faint
            }
        }()
        let borderColor: Color = {
            switch thread.state {
            case .needs, .waiting: return SwarmTheme.warnAmber.opacity(0.45)
            case .stuck: return SwarmTheme.errorRed.opacity(0.40)
            default: return SwarmTheme.border
            }
        }()

        return VStack(alignment: .leading, spacing: 0) {
            Button(action: {
                UIImpactFeedbackGenerator(style: .light).impactOccurred()
                withAnimation(.easeInOut(duration: 0.18)) {
                    if isOpen {
                        expandedThreadIds.remove(thread.id)
                        if defaultOpen { expandedThreadIds.insert("closed:\(thread.id)") }
                    } else {
                        expandedThreadIds.remove("closed:\(thread.id)")
                        expandedThreadIds.insert(thread.id)
                    }
                }
            }) {
                HStack(spacing: 8) {
                    Circle()
                        .fill(dotColor)
                        .frame(width: 8, height: 8)

                    HStack(alignment: .firstTextBaseline, spacing: 6) {
                        Text(thread.title)
                            .font(.system(size: 14, weight: .semibold))
                            .foregroundColor(SwarmTheme.ink)
                            .lineLimit(1)

                        Text(thread.statusLabel)
                            .font(.system(size: 12))
                            .foregroundColor(thread.state == .waiting || thread.state == .needs ? SwarmTheme.warnAmber : (thread.state == .stuck ? SwarmTheme.errorRed : SwarmTheme.muted))
                            .lineLimit(1)
                    }

                    Spacer(minLength: 6)

                    HStack(spacing: 6) {
                        OverviewMiniSegmentedBar(
                            done: thread.doneCount,
                            running: thread.runningCount,
                            blocked: thread.blockedCount,
                            queued: thread.queuedCount
                        )
                        Text("\(thread.doneCount)/\(thread.totalCount)")
                            .font(.system(size: 12, design: .monospaced))
                            .foregroundColor(SwarmTheme.muted)
                        Image(systemName: isOpen ? "chevron.down" : "chevron.right")
                            .font(.system(size: 10, weight: .semibold))
                            .foregroundColor(SwarmTheme.faint)
                    }
                }
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)

            if !thread.goal.isEmpty {
                Text(thread.goal)
                    .font(.system(size: 13))
                    .foregroundColor(SwarmTheme.muted)
                    .padding(.leading, 16)
                    .padding(.top, 3)
            }

            if !thread.now.isEmpty {
                HStack(spacing: 6) {
                    if thread.state != .done {
                        Text("Now")
                            .font(.system(size: 12, weight: .semibold))
                            .foregroundColor(SwarmTheme.ink.opacity(0.82))
                    }
                    Text(thread.now)
                        .font(.system(size: 12))
                        .foregroundColor(SwarmTheme.muted)
                        .lineLimit(1)
                }
                .padding(.leading, 16)
                .padding(.top, 4)
            }

            if isOpen && !thread.tasks.isEmpty {
                Divider()
                    .overlay(SwarmTheme.border.opacity(0.75))
                    .padding(.leading, 16)
                    .padding(.top, 9)

                VStack(alignment: .leading, spacing: 7) {
                    ForEach(thread.tasks) { task in
                        HStack(spacing: 8) {
                            OverviewTaskIndicator(state: task.state)
                            Text(task.title)
                                .font(.system(size: 12.5))
                                .foregroundColor(task.state == .done ? SwarmTheme.muted : SwarmTheme.ink)
                                .lineLimit(1)
                            Spacer(minLength: 4)
                            if task.rawState == "review" {
                                Text("In review")
                                    .font(.system(size: 10.5, weight: .medium))
                                    .foregroundColor(SwarmTheme.accent)
                                    .padding(.horizontal, 6)
                                    .padding(.vertical, 1.5)
                                    .background(SwarmTheme.accent.opacity(0.12))
                                    .clipShape(Capsule())
                            }
                            workerBeeChips(owners: task.owners, synth: synth, snap: snap)
                        }
                    }
                }
                .padding(.leading, 16)
                .padding(.top, 8)
            }
        }
        .padding(.horizontal, 13)
        .padding(.vertical, 11)
        .background(thread.state == .done ? Color.clear : SwarmTheme.surfaceBg)
        .clipShape(RoundedRectangle(cornerRadius: 11, style: .continuous))
        .overlay(RoundedRectangle(cornerRadius: 11, style: .continuous).stroke(borderColor, lineWidth: 0.8))
    }

    @ViewBuilder
    private func overviewTeamSection(synth: SwarmSynthesizedModel, snap: SwarmSnapshot) -> some View {
        let coord = synth.members["coordinator"]
        let coordPose: PixelBeePose = (!synth.finished && coord?.status == "working") ? .working : .landed
        let crew = synth.orderedCrew
        let maxTiles = 5
        let shownCrew: [SwarmSynthesizedMember] = {
            if crew.count <= maxTiles + 1 { return crew }
            var top = Array(crew.prefix(maxTiles))
            if let sel = overviewFilterMember, !top.contains(where: { $0.name == sel }),
               let match = crew.first(where: { $0.name == sel }) {
                top = Array(top.prefix(maxTiles - 1)) + [match]
            }
            return top
        }()
        let extraCrewCount = max(0, crew.count - shownCrew.count)

        VStack(alignment: .leading, spacing: 10) {
            Text("The team")
                .font(.system(size: 13.5, weight: .semibold))
                .foregroundColor(SwarmTheme.ink)

            // Coordinator Lead Card
            Button(action: {
                if let convId = coord?.conversationId, !convId.isEmpty {
                    dismiss()
                    onSelectConversation?(convId)
                }
            }) {
                HStack(spacing: 12) {
                    SwarmPixelBeeView(
                        name: "coordinator",
                        pose: coordPose,
                        size: 54
                    )

                    VStack(alignment: .leading, spacing: 3) {
                        Text("Coordinator")
                            .font(.system(size: 14, weight: .semibold))
                            .foregroundColor(SwarmTheme.ink)
                        if let now = coord?.now, !now.isEmpty {
                            Text(now)
                                .font(.system(size: 12.5))
                                .foregroundColor(SwarmTheme.muted)
                                .lineLimit(1)
                        }
                    }

                    Spacer()
                }
                .padding(10)
                .background(SwarmTheme.surfaceBg)
                .clipShape(RoundedRectangle(cornerRadius: 11, style: .continuous))
                .overlay(RoundedRectangle(cornerRadius: 11, style: .continuous).stroke(SwarmTheme.border, lineWidth: 0.8))
            }
            .buttonStyle(.plain)

            // Crew Pixel-Bee Filter Tiles (All + Workers + +N)
            if !crew.isEmpty {
                ScrollView(.horizontal, showsIndicators: false) {
                    HStack(spacing: 10) {
                        // "All" 3-bee mini swarm tile
                        let allSelected = (overviewFilterMember == nil)
                        Button(action: {
                            UIImpactFeedbackGenerator(style: .light).impactOccurred()
                            withAnimation(.easeInOut(duration: 0.16)) {
                                overviewFilterMember = nil
                            }
                        }) {
                            VStack(spacing: 4) {
                                SwarmMiniTrioTileView(
                                    names: crew.prefix(3).map(\.name),
                                    size: 46,
                                    isActive: !synth.finished
                                )
                                .overlay(
                                    RoundedRectangle(cornerRadius: 10, style: .continuous)
                                        .stroke(allSelected ? SwarmTheme.ink : SwarmTheme.border, lineWidth: allSelected ? 1.8 : 0.8)
                                )
                                Text("All")
                                    .font(.system(size: 11, weight: allSelected ? .semibold : .regular))
                                    .foregroundColor(allSelected ? SwarmTheme.ink : SwarmTheme.muted)
                            }
                            .frame(width: 52)
                        }
                        .buttonStyle(.plain)

                        ForEach(shownCrew) { member in
                            let isSelected = (overviewFilterMember == member.name)
                            let isWorking = !synth.finished && member.status == "working"
                            let dotColor: Color? = {
                                guard !synth.finished else { return nil }
                                if member.hasIssue || member.status == "needs_you" { return SwarmTheme.errorRed }
                                if isWorking { return SwarmTheme.liveGreen }
                                if member.status == "waiting_on" { return SwarmTheme.warnAmber }
                                return nil
                            }()

                            Button(action: {
                                UIImpactFeedbackGenerator(style: .light).impactOccurred()
                                withAnimation(.easeInOut(duration: 0.16)) {
                                    overviewFilterMember = (overviewFilterMember == member.name) ? nil : member.name
                                }
                            }) {
                                VStack(spacing: 4) {
                                    ZStack(alignment: .topTrailing) {
                                        SwarmPixelBeeView(
                                            name: member.name,
                                            pose: isWorking ? .working : .landed,
                                            size: 46
                                        )
                                        .overlay(
                                            RoundedRectangle(cornerRadius: 11, style: .continuous)
                                                .stroke(isSelected ? SwarmTheme.ink : Color.clear, lineWidth: 1.8)
                                        )

                                        if let dot = dotColor {
                                            Circle()
                                                .fill(dot)
                                                .frame(width: 9.5, height: 9.5)
                                                .overlay(Circle().stroke(SwarmTheme.bg, lineWidth: 1.8))
                                                .offset(x: 2, y: -2)
                                        }
                                    }

                                    Text("@\(member.name)")
                                        .font(.system(size: 11, weight: isSelected ? .semibold : .regular))
                                        .foregroundColor(isSelected ? SwarmTheme.ink : SwarmTheme.muted)
                                        .lineLimit(1)
                                }
                                .frame(width: 54)
                            }
                            .buttonStyle(.plain)
                        }

                        if extraCrewCount > 0 {
                            Button(action: {
                                UIImpactFeedbackGenerator(style: .light).impactOccurred()
                                withAnimation(.easeInOut(duration: 0.16)) {
                                    selectedTab = .members
                                }
                            }) {
                                VStack(spacing: 4) {
                                    Text("+\(extraCrewCount)")
                                        .font(.system(size: 12.5, weight: .semibold))
                                        .foregroundColor(SwarmTheme.muted)
                                        .frame(width: 46, height: 46)
                                        .background(SwarmTheme.fieldBg)
                                        .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
                                        .overlay(RoundedRectangle(cornerRadius: 10, style: .continuous).stroke(SwarmTheme.border, lineWidth: 0.8))
                                    Text(" ")
                                        .font(.system(size: 11))
                                }
                                .frame(width: 50)
                            }
                            .buttonStyle(.plain)
                        }
                    }
                    .padding(.vertical, 3)
                    .padding(.horizontal, 1)
                }
            }
        }
    }

    private func progressStatusChip(text: String, color: Color) -> some View {
        HStack(spacing: 4.5) {
            Circle()
                .fill(color)
                .frame(width: 5.5, height: 5.5)
            Text(text)
                .font(.system(size: 12, weight: .medium))
                .foregroundColor(color)
                .lineLimit(1)
        }
    }

    private func workerBeeChips(owners: [String], synth: SwarmSynthesizedModel, snap: SwarmSnapshot) -> some View {
        let shown = Array(owners.prefix(2))
        let extra = max(0, owners.count - shown.count)
        return HStack(spacing: 4) {
            ForEach(shown, id: \.self) { owner in
                let m = synth.members[owner]
                let working = !synth.finished && !(m?.killed ?? false) && m?.status == "working"
                Button(action: {
                    if let convId = snap.members.first(where: { $0.name == owner })?.conversationId, !convId.isEmpty {
                        dismiss()
                        onSelectConversation?(convId)
                    }
                }) {
                    HStack(spacing: 3.5) {
                        SwarmPixelBeeSpriteView(
                            name: owner,
                            pose: working ? .working : .landed,
                            width: 19,
                            height: 15.5
                        )
                        Text(owner)
                            .font(.system(size: 11, weight: .medium))
                            .foregroundColor(SwarmTheme.ink.opacity(0.85))
                            .lineLimit(1)
                    }
                    .padding(.leading, 4)
                    .padding(.trailing, 7)
                    .frame(height: 22)
                    .background(SwarmTheme.fieldBg)
                    .clipShape(Capsule())
                }
                .buttonStyle(.plain)
            }
            if extra > 0 {
                Text("+\(extra)")
                    .font(.system(size: 11, weight: .medium))
                    .foregroundColor(SwarmTheme.muted)
                    .padding(.horizontal, 6)
                    .frame(height: 22)
                    .background(SwarmTheme.fieldBg)
                    .clipShape(Capsule())
            }
        }
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
            .padding(.horizontal, 16)
            .padding(.top, 14)
            .padding(.bottom, 12)

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
                    Text(snap.messages.isEmpty ? String(localized: "暂无协作留言") : String(localized: "没有匹配筛选条件的消息"))
                        .font(.system(size: 14, weight: .medium))
                        .foregroundColor(SwarmTheme.muted)
                    Spacer()
                }
            } else {
                ScrollView {
                    LazyVStack(alignment: .leading, spacing: 0) {
                        if snap.hasMoreBefore {
                            HStack {
                                Spacer()
                                Button(action: {
                                    Task { await loadEarlierMessages() }
                                }) {
                                    HStack(spacing: 6) {
                                        if isLoadingEarlier {
                                            ProgressView().scaleEffect(0.75)
                                        }
                                        Text(isLoadingEarlier ? String(localized: "正在加载更早消息...") : String(localized: "加载更早消息"))
                                            .font(.system(size: 12, weight: .medium))
                                            .foregroundColor(SwarmTheme.accent)
                                    }
                                    .padding(.horizontal, 12)
                                    .padding(.vertical, 6)
                                    .background(SwarmTheme.cardBg)
                                    .clipShape(Capsule())
                                }
                                .buttonStyle(.plain)
                                .disabled(isLoadingEarlier)
                                Spacer()
                            }
                            .padding(.vertical, 10)
                        }
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
                        Text(isExpanded ? String(localized: "收起") : String(localized: "展开更多"))
                            .font(.system(size: 11.5, weight: .medium))
                            .foregroundColor(SwarmTheme.accent)
                    }
                    .buttonStyle(.plain)
                    .padding(.top, 2)
                }
            }
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 12)
    }

    // MARK: - Tab 3: Members (Native Replication of Desktop Members Roster)

    @ViewBuilder
    private func membersView(_ snap: SwarmSnapshot) -> some View {
        let q = memberSearch.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()

        let matching: [SwarmMemberInfo] = snap.members.filter { m in
            guard !m.roleWorker && !SwarmHelpers.isRoleWorker(name: m.name, role: m.role) else { return false }
            guard !q.isEmpty else { return true }
            let planTask = snap.plan?.tasks[m.name] ?? ""
            let roleHint = !planTask.isEmpty ? planTask : SwarmHelpers.extractRoleHint(for: m.name, role: m.role, messages: snap.messages)
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
            .padding(.horizontal, 16)
            .padding(.top, 14)
            .padding(.bottom, 12)

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
                                memberSectionHeader(title: String(localized: "ACTIVE"), count: activeWorkers.count)
                                ForEach(activeWorkers) { m in
                                    memberRow(m, snap: snap, highlightedCard: true)
                                }
                            }
                            .padding(.horizontal, 14)
                        }

                        // STANDING BY group
                        if !standbyWorkers.isEmpty {
                            VStack(alignment: .leading, spacing: 6) {
                                memberSectionHeader(title: String(localized: "STANDING BY"), count: standbyWorkers.count)
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
        let planTask = snap.plan?.tasks[member.name]?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        let roleHint = !planTask.isEmpty ? planTask : SwarmHelpers.extractRoleHint(for: member.name, role: member.role, messages: snap.messages)
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
                    // Header line: @name + status/role + Current badge
                    HStack(alignment: .firstTextBaseline, spacing: 6) {
                        Text("@\(member.name)")
                            .font(.system(size: 13.5, weight: .semibold))
                            .foregroundColor(member.killed ? SwarmTheme.faint : SwarmTheme.ink)
                            .lineLimit(1)
                            .fixedSize(horizontal: true, vertical: false)
                            .layoutPriority(2)

                        if isCoord {
                            Text(isWorking ? String(localized: "Coordinator · Working") : String(localized: "Coordinator"))
                                .font(.system(size: 12, weight: .medium))
                                .foregroundColor(isWorking ? SwarmTheme.accent : SwarmTheme.muted)
                                .lineLimit(1)
                                .minimumScaleFactor(0.85)
                        } else if member.killed {
                            Text("Retired")
                                .font(.system(size: 11.5, weight: .medium))
                                .foregroundColor(SwarmTheme.faint)
                                .lineLimit(1)
                                .fixedSize(horizontal: true, vertical: false)
                        } else if isWorking {
                            Text("Working")
                                .font(.system(size: 12, weight: .semibold))
                                .foregroundColor(SwarmTheme.accent)
                                .lineLimit(1)
                                .fixedSize(horizontal: true, vertical: false)
                        } else if isError {
                            Text("Error")
                                .font(.system(size: 12, weight: .semibold))
                                .foregroundColor(SwarmTheme.errorRed)
                                .lineLimit(1)
                                .fixedSize(horizontal: true, vertical: false)
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
                                .lineLimit(1)
                                .fixedSize(horizontal: true, vertical: false)
                                .layoutPriority(1)
                                .padding(.horizontal, 5)
                                .padding(.vertical, 1)
                                .background(SwarmTheme.infoBlue.opacity(0.12))
                                .clipShape(Capsule())
                        }

                        Spacer(minLength: 0)
                    }

                    // Model & latest board post sequence on its own compact metadata line
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
                                .lineLimit(1)
                        }
                        .fixedSize(horizontal: true, vertical: false)
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
                                    .lineLimit(1)
                            }
                            .fixedSize(horizontal: true, vertical: false)
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
            if !silent { errorMessage = String(localized: "未连接网关") }
            return
        }
        if !silent {
            isLoading = true
            errorMessage = nil
        }
        do {
            if silent, let existing = self.snapshot, let latestSeq = existing.messages.last?.seq, latestSeq > 0 {
                if let delta = try await APIClient.shared.fetchSwarmSnapshotIfModified(
                    cascadeId: cascadeId,
                    afterSeq: latestSeq,
                    etag: lastETag,
                    baseURL: url
                ) {
                    self.lastETag = delta.etag ?? self.lastETag
                    self.snapshot = existing.mergingMessages(from: delta, isOlderPage: false)
                }
            } else {
                let snap = try await APIClient.shared.fetchSwarmSnapshot(
                    cascadeId: cascadeId,
                    limit: swarmPageSize,
                    baseURL: url
                )
                self.lastETag = snap.etag
                self.snapshot = snap
            }
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

    private func loadEarlierMessages() async {
        guard !isLoadingEarlier,
              let url = settings.serverURL,
              let existing = self.snapshot,
              existing.hasMoreBefore else { return }
        let oldest = existing.oldestSeq > 0 ? existing.oldestSeq : (existing.messages.first?.seq ?? 0)
        guard oldest > 0 else { return }
        isLoadingEarlier = true
        defer { isLoadingEarlier = false }
        do {
            let older = try await APIClient.shared.fetchSwarmSnapshot(
                cascadeId: cascadeId,
                beforeSeq: oldest,
                limit: swarmPageSize,
                baseURL: url
            )
            self.snapshot = existing.mergingMessages(from: older, isOlderPage: true)
        } catch {
            showToast(error.localizedDescription)
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
                showToast(String(localized: "反馈已发送至协调员"))
                await loadSnapshot(silent: true)
            } catch {
                showToast(String(localized: "发送失败: \(error.localizedDescription)"))
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
                showToast(String(localized: "已催促唤醒 @\(member)"))
                await loadSnapshot(silent: true)
            } catch {
                showToast(String(localized: "催促失败: \(error.localizedDescription)"))
            }
            nudgingMembers.remove(member)
        }
    }
}

// MARK: - Overview Visual Primitives (Ported from hive.css / hive.js)

private struct OverviewTaskIndicator: View {
    let state: SwarmTaskBucketState
    @State private var spinning: Bool = false

    var body: some View {
        switch state {
        case .done:
            ZStack {
                Circle()
                    .fill(SwarmTheme.liveGreen.opacity(0.22))
                    .frame(width: 15, height: 15)
                Image(systemName: "checkmark")
                    .font(.system(size: 8.5, weight: .bold))
                    .foregroundColor(SwarmTheme.liveGreen)
            }
        case .running:
            Circle()
                .trim(from: 0, to: 0.72)
                .stroke(SwarmTheme.accent, style: StrokeStyle(lineWidth: 2, lineCap: .round))
                .frame(width: 14, height: 14)
                .rotationEffect(.degrees(spinning ? 360 : 0))
                .onAppear {
                    withAnimation(.linear(duration: 1.15).repeatForever(autoreverses: false)) {
                        spinning = true
                    }
                }
        case .blocked:
            Circle()
                .stroke(SwarmTheme.warnAmber, lineWidth: 2)
                .frame(width: 14, height: 14)
        case .queued:
            Circle()
                .stroke(SwarmTheme.faint, style: StrokeStyle(lineWidth: 1.4, dash: [2.5, 2.5]))
                .frame(width: 14, height: 14)
        }
    }
}

private struct OverviewProgressRingView: View {
    let done: Int
    let running: Int
    let blocked: Int
    let total: Int

    var body: some View {
        if total > 0 && done == total {
            ZStack {
                Circle()
                    .fill(SwarmTheme.liveGreen.opacity(0.22))
                    .frame(width: 18, height: 18)
                Image(systemName: "checkmark")
                    .font(.system(size: 9.5, weight: .bold))
                    .foregroundColor(SwarmTheme.liveGreen)
            }
        } else {
            let safeTotal = Double(max(1, total))
            let fDone = Double(done) / safeTotal
            let fRun = Double(running) / safeTotal
            let fBlk = Double(blocked) / safeTotal
            ZStack {
                Circle()
                    .stroke(SwarmTheme.fieldBg, lineWidth: 2.3)
                if fDone > 0 {
                    Circle()
                        .trim(from: 0, to: fDone)
                        .stroke(SwarmTheme.liveGreen, style: StrokeStyle(lineWidth: 2.3, lineCap: .butt))
                        .rotationEffect(.degrees(-90))
                }
                if fRun > 0 {
                    Circle()
                        .trim(from: fDone, to: fDone + fRun)
                        .stroke(SwarmTheme.accent, style: StrokeStyle(lineWidth: 2.3, lineCap: .butt))
                        .rotationEffect(.degrees(-90))
                }
                if fBlk > 0 {
                    Circle()
                        .trim(from: fDone + fRun, to: fDone + fRun + fBlk)
                        .stroke(SwarmTheme.warnAmber, style: StrokeStyle(lineWidth: 2.3, lineCap: .butt))
                        .rotationEffect(.degrees(-90))
                }
            }
            .frame(width: 18, height: 18)
        }
    }
}

private struct OverviewMiniSegmentedBar: View {
    let done: Int
    let running: Int
    let blocked: Int
    let queued: Int

    var body: some View {
        let total = max(1, done + running + blocked + queued)
        GeometryReader { geo in
            let w = geo.size.width
            HStack(spacing: 1) {
                if done > 0 {
                    Rectangle()
                        .fill(SwarmTheme.liveGreen)
                        .frame(width: max(2, w * CGFloat(done) / CGFloat(total)))
                }
                if running > 0 {
                    Rectangle()
                        .fill(SwarmTheme.accent)
                        .frame(width: max(2, w * CGFloat(running) / CGFloat(total)))
                }
                if blocked > 0 {
                    Rectangle()
                        .fill(SwarmTheme.warnAmber)
                        .frame(width: max(2, w * CGFloat(blocked) / CGFloat(total)))
                }
                if queued > 0 {
                    Rectangle()
                        .fill(SwarmTheme.faint.opacity(0.45))
                        .frame(width: max(2, w * CGFloat(queued) / CGFloat(total)))
                }
            }
        }
        .frame(width: 28, height: 4.5)
        .background(SwarmTheme.fieldBg)
        .clipShape(Capsule())
    }
}
