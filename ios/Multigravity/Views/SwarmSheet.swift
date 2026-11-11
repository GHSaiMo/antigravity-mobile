import SwiftUI

public struct SwarmSheet: View {
    @Environment(\.dismiss) private var dismiss
    @State private var settings = AppSettings.shared

    public let cascadeId: String
    public let onSelectConversation: ((String) -> Void)?

    @State private var selectedTab: Tab = .overview
    @State private var snapshot: SwarmSnapshot? = nil
    @State private var isLoading: Bool = false
    @State private var errorMessage: String? = nil
    @State private var feedbackText: String = ""
    @State private var isSendingFeedback: Bool = false
    @State private var nudgingMembers: Set<String> = []
    @State private var toastMessage: String? = nil

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
        NavigationStack {
            VStack(spacing: 0) {
                Picker("视角", selection: $selectedTab) {
                    ForEach(Tab.allCases) { tab in
                        Text(tab.rawValue).tag(tab)
                    }
                }
                .pickerStyle(.segmented)
                .padding(.horizontal, 16)
                .padding(.top, 10)
                .padding(.bottom, 8)

                if isLoading && snapshot == nil {
                    Spacer()
                    ProgressView("正在同步蜂群看板与成员状态...")
                        .font(.system(size: 14))
                        .foregroundColor(.secondary)
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
                        graphView(snap)
                    }
                } else {
                    emptyStateView
                }
            }
            .navigationTitle(snapshot?.title.isEmpty == false ? snapshot!.title : "蜂群协同 (Swarm)")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Button("完成") {
                        dismiss()
                    }
                }
            }
            .overlay(alignment: .bottom) {
                if let toast = toastMessage {
                    Text(toast)
                        .font(.system(size: 13, weight: .medium))
                        .foregroundColor(.white)
                        .padding(.horizontal, 16)
                        .padding(.vertical, 8)
                        .background(Color.black.opacity(0.85))
                        .clipShape(Capsule())
                        .shadow(radius: 6)
                        .padding(.bottom, 24)
                        .transition(.move(edge: .bottom).combined(with: .opacity))
                }
            }
            .animation(.easeInOut(duration: 0.2), value: toastMessage)
            .task {
                await loadSnapshot()
            }
            .refreshable {
                await loadSnapshot()
            }
        }
    }

    // MARK: - Overview Tab

    @ViewBuilder
    private func overviewView(_ snap: SwarmSnapshot) -> some View {
        let phaseLabel = snap.view?.phase?.label ?? snap.brief.phase
        ScrollView {
            VStack(alignment: .leading, spacing: 16) {
                // Header card
                VStack(alignment: .leading, spacing: 10) {
                    HStack {
                        Text(snap.title.isEmpty ? "蜂群协同任务" : snap.title)
                            .font(.system(size: 17, weight: .bold))
                            .foregroundColor(.primary)

                        Spacer()

                        if !phaseLabel.isEmpty {
                            Text(phaseLabel)
                                .font(.system(size: 12, weight: .bold))
                                .foregroundColor(.accentColor)
                                .padding(.horizontal, 8)
                                .padding(.vertical, 3)
                                .background(Color.accentColor.opacity(0.12))
                                .clipShape(Capsule())
                        }
                    }

                    let headline = snap.view?.headline ?? snap.brief.headline
                    if !headline.isEmpty {
                        Text(headline)
                            .font(.system(size: 13.5))
                            .foregroundColor(.secondary)
                            .lineSpacing(2)
                    }

                    HStack(spacing: 12) {
                        statBox(title: "活跃成员", value: "\(snap.brief.workingCount) / \(snap.brief.totalMembers)", icon: "person.2.fill", color: .blue)
                        statBox(title: "协作消息", value: "\(max(snap.messages.count, snap.brief.latestSeq))", icon: "bubble.left.and.bubble.right.fill", color: .purple)
                        statBox(title: "待关注", value: "\(snap.brief.attentionCount)", icon: "exclamationmark.triangle.fill", color: snap.brief.attentionCount > 0 ? .orange : .gray)
                    }
                    .padding(.top, 4)
                }
                .padding(14)
                .background(Color(uiColor: .secondarySystemGroupedBackground))
                .clipShape(RoundedRectangle(cornerRadius: 14, style: .continuous))

                // User Feedback Relay Section
                VStack(alignment: .leading, spacing: 10) {
                    HStack {
                        Image(systemName: "megaphone.fill")
                            .font(.system(size: 14))
                            .foregroundColor(.accentColor)
                        Text("协同指挥 / 用户反馈")
                            .font(.system(size: 15, weight: .semibold))
                    }

                    Text("直接向蜂群公共留言板发送人类反馈，协调各成员或给出方向指令（以 @narrator 格式发送至协调员）。")
                        .font(.system(size: 12))
                        .foregroundColor(.secondary)

                    HStack(spacing: 8) {
                        TextField("输入对全体成员或协调者的反馈...", text: $feedbackText)
                            .textFieldStyle(.roundedBorder)
                            .font(.system(size: 13))

                        Button(action: sendFeedback) {
                            if isSendingFeedback {
                                ProgressView()
                                    .scaleEffect(0.8)
                            } else {
                                Text("发板")
                                    .font(.system(size: 13, weight: .semibold))
                            }
                        }
                        .buttonStyle(.borderedProminent)
                        .disabled(feedbackText.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty || isSendingFeedback)
                    }
                }
                .padding(14)
                .background(Color(uiColor: .secondarySystemGroupedBackground))
                .clipShape(RoundedRectangle(cornerRadius: 14, style: .continuous))

                // Attention items from view.json
                if let attentions = snap.view?.attention, !attentions.isEmpty {
                    VStack(alignment: .leading, spacing: 10) {
                        HStack {
                            Image(systemName: "exclamationmark.triangle.fill")
                                .font(.system(size: 13))
                                .foregroundColor(.orange)
                            Text("待处理事项 (\(attentions.count))")
                                .font(.system(size: 14, weight: .semibold))
                                .foregroundColor(.orange)
                        }

                        ForEach(attentions) { item in
                            VStack(alignment: .leading, spacing: 4) {
                                HStack {
                                    Text(item.title)
                                        .font(.system(size: 13.5, weight: .semibold))
                                        .foregroundColor(.primary)
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
                                        .foregroundColor(.secondary)
                                }
                            }
                            .padding(10)
                            .frame(maxWidth: .infinity, alignment: .leading)
                            .background(attentionColor(item.severity).opacity(0.08))
                            .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
                        }
                    }
                    .padding(14)
                    .background(Color(uiColor: .secondarySystemGroupedBackground))
                    .clipShape(RoundedRectangle(cornerRadius: 14, style: .continuous))
                }

                // Workstreams from view.json
                if let workstreams = snap.view?.workstreams, !workstreams.isEmpty {
                    VStack(alignment: .leading, spacing: 10) {
                        Text("工作流推进 (\(workstreams.count))")
                            .font(.system(size: 14, weight: .semibold))
                            .foregroundColor(.secondary)

                        ForEach(workstreams) { ws in
                            VStack(alignment: .leading, spacing: 5) {
                                HStack {
                                    Text(ws.title)
                                        .font(.system(size: 13.5, weight: .semibold))
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
                                        .foregroundColor(.secondary)
                                }
                                if !ws.owners.isEmpty {
                                    Text("负责人: \(ws.owners.joined(separator: ", "))")
                                        .font(.system(size: 11))
                                        .foregroundColor(.secondary)
                                }
                            }
                            .padding(10)
                            .frame(maxWidth: .infinity, alignment: .leading)
                            .background(Color(uiColor: .tertiarySystemGroupedBackground))
                            .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
                        }
                    }
                    .padding(14)
                    .background(Color(uiColor: .secondarySystemGroupedBackground))
                    .clipShape(RoundedRectangle(cornerRadius: 14, style: .continuous))
                }
            }
            .padding(16)
        }
    }

    private func statBox(title: String, value: String, icon: String, color: Color) -> some View {
        VStack(spacing: 4) {
            Image(systemName: icon)
                .font(.system(size: 16))
                .foregroundColor(color)
            Text(value)
                .font(.system(size: 15, weight: .bold))
                .foregroundColor(.primary)
            Text(title)
                .font(.system(size: 11))
                .foregroundColor(.secondary)
        }
        .frame(maxWidth: .infinity)
        .padding(.vertical, 10)
        .background(color.opacity(0.08))
        .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
    }

    // MARK: - Board Tab

    @ViewBuilder
    private func boardView(_ snap: SwarmSnapshot) -> some View {
        if snap.messages.isEmpty {
            VStack(spacing: 12) {
                Spacer()
                Image(systemName: "bubble.left.and.bubble.right")
                    .font(.system(size: 40))
                    .foregroundColor(.secondary)
                Text("暂无协作留言")
                    .font(.system(size: 15, weight: .medium))
                    .foregroundColor(.secondary)
                Spacer()
            }
        } else {
            List(snap.messages) { msg in
                VStack(alignment: .leading, spacing: 8) {
                    HStack(spacing: 6) {
                        HStack(spacing: 4) {
                            Image(systemName: msg.agent == "narrator" ? "person.crop.circle.badge.checkmark" : "person.crop.circle")
                                .font(.system(size: 12))
                            Text(msg.agent)
                                .font(.system(size: 12, weight: .semibold))
                        }
                        .foregroundColor(msg.agent == "narrator" ? .blue : .primary)
                        .padding(.horizontal, 7)
                        .padding(.vertical, 2.5)
                        .background(msg.agent == "narrator" ? Color.blue.opacity(0.12) : Color.secondary.opacity(0.12))
                        .clipShape(Capsule())

                        Text("#\(msg.seq)")
                            .font(.system(size: 11, design: .monospaced))
                            .foregroundColor(.secondary)

                        if let ch = msg.channel, !ch.isEmpty {
                            Text(ch)
                                .font(.system(size: 10.5))
                                .foregroundColor(.purple)
                                .padding(.horizontal, 5)
                                .padding(.vertical, 1.5)
                                .background(Color.purple.opacity(0.1))
                                .clipShape(Capsule())
                        }

                        Spacer()

                        if !msg.time.isEmpty {
                            Text(msg.time)
                                .font(.system(size: 10.5))
                                .foregroundColor(.secondary)
                                .lineLimit(1)
                        }
                    }

                    if !msg.mentions.isEmpty {
                        FlowLayout(horizontalSpacing: 4, verticalSpacing: 4) {
                            ForEach(msg.mentions, id: \.self) { mention in
                                Text("@\(mention)")
                                    .font(.system(size: 11, weight: .medium))
                                    .foregroundColor(.accentColor)
                                    .padding(.horizontal, 5)
                                    .padding(.vertical, 2)
                                    .background(Color.accentColor.opacity(0.1))
                                    .clipShape(Capsule())
                            }
                        }
                    }

                    Text(msg.text)
                        .font(.system(size: 13.5))
                        .foregroundColor(.primary)
                        .lineSpacing(2)
                }
                .padding(.vertical, 4)
            }
            .listStyle(.insetGrouped)
        }
    }

    // MARK: - Members Tab

    @ViewBuilder
    private func membersView(_ snap: SwarmSnapshot) -> some View {
        List(snap.members) { member in
            VStack(alignment: .leading, spacing: 8) {
                HStack(alignment: .center, spacing: 8) {
                    Circle()
                        .fill(statusColor(member.status))
                        .frame(width: 8, height: 8)

                    Text(member.name)
                        .font(.system(size: 15, weight: .bold))

                    Text(statusText(member.status))
                        .font(.system(size: 11, weight: .medium))
                        .foregroundColor(statusColor(member.status))
                        .padding(.horizontal, 6)
                        .padding(.vertical, 2)
                        .background(statusColor(member.status).opacity(0.12))
                        .clipShape(Capsule())

                    if member.isCurrent {
                        Text("当前会话")
                            .font(.system(size: 10, weight: .semibold))
                            .foregroundColor(.purple)
                            .padding(.horizontal, 5)
                            .padding(.vertical, 2)
                            .background(Color.purple.opacity(0.12))
                            .clipShape(Capsule())
                    }

                    Spacer()

                    if !member.model.isEmpty {
                        Text(member.model)
                            .font(.system(size: 10.5, weight: .medium, design: .monospaced))
                            .foregroundColor(.secondary)
                            .padding(.horizontal, 6)
                            .padding(.vertical, 2)
                            .background(Color.secondary.opacity(0.1))
                            .clipShape(Capsule())
                            .lineLimit(1)
                    }
                }

                if !member.role.isEmpty {
                    Text(member.role)
                        .font(.system(size: 12.5, weight: .medium))
                        .foregroundColor(.primary)
                }

                if let activity = member.lastActivity, !activity.isEmpty {
                    Text("最近动态: \(activity)")
                        .font(.system(size: 12))
                        .foregroundColor(.secondary)
                        .lineLimit(2)
                }

                HStack(spacing: 10) {
                    Button(action: {
                        nudgeMember(member.name)
                    }) {
                        HStack(spacing: 4) {
                            if nudgingMembers.contains(member.name) {
                                ProgressView()
                                    .scaleEffect(0.7)
                            } else {
                                Image(systemName: "bolt.fill")
                                    .font(.system(size: 11))
                            }
                            Text("催促 / 唤醒")
                                .font(.system(size: 12, weight: .semibold))
                        }
                        .foregroundColor(.orange)
                        .padding(.horizontal, 10)
                        .padding(.vertical, 5)
                        .background(Color.orange.opacity(0.12))
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
                                Image(systemName: "arrow.up.right.square")
                                    .font(.system(size: 11))
                                Text("进入子会话")
                                    .font(.system(size: 12, weight: .medium))
                            }
                            .foregroundColor(.accentColor)
                            .padding(.horizontal, 10)
                            .padding(.vertical, 5)
                            .background(Color.accentColor.opacity(0.12))
                            .clipShape(Capsule())
                        }
                        .buttonStyle(.plain)
                    }

                    Spacer()

                    if member.unreadCount > 0 {
                        Text("未读 \(member.unreadCount)")
                            .font(.system(size: 11))
                            .foregroundColor(.secondary)
                    }
                }
                .padding(.top, 4)
            }
            .padding(.vertical, 6)
        }
        .listStyle(.insetGrouped)
    }

    // MARK: - Graph Tab

    @ViewBuilder
    private func graphView(_ snap: SwarmSnapshot) -> some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 16) {
                Text("协作网络与提及关系")
                    .font(.system(size: 15, weight: .semibold))

                ForEach(snap.members) { m in
                    let mentionsOut = snap.messages.filter { $0.agent == m.name }.flatMap(\.mentions)
                    let mentionsIn = snap.messages.filter { $0.mentions.contains(m.name) }.map(\.agent)

                    VStack(alignment: .leading, spacing: 6) {
                        HStack {
                            Circle()
                                .fill(statusColor(m.status))
                                .frame(width: 8, height: 8)
                            Text(m.name)
                                .font(.system(size: 14, weight: .bold))
                            Spacer()
                            Text(m.role)
                                .font(.system(size: 12))
                                .foregroundColor(.secondary)
                        }

                        if !mentionsOut.isEmpty {
                            HStack(spacing: 4) {
                                Text("→ 提及:")
                                    .font(.system(size: 11))
                                    .foregroundColor(.secondary)
                                ForEach(Array(Set(mentionsOut)).sorted(), id: \.self) { target in
                                    Text("@\(target)")
                                        .font(.system(size: 11, weight: .medium))
                                        .foregroundColor(.accentColor)
                                        .padding(.horizontal, 5)
                                        .padding(.vertical, 1.5)
                                        .background(Color.accentColor.opacity(0.1))
                                        .clipShape(Capsule())
                                }
                            }
                        }

                        if !mentionsIn.isEmpty {
                            HStack(spacing: 4) {
                                Text("← 被提及:")
                                    .font(.system(size: 11))
                                    .foregroundColor(.secondary)
                                ForEach(Array(Set(mentionsIn)).sorted(), id: \.self) { from in
                                    Text("@\(from)")
                                        .font(.system(size: 11, weight: .medium))
                                        .foregroundColor(.purple)
                                        .padding(.horizontal, 5)
                                        .padding(.vertical, 1.5)
                                        .background(Color.purple.opacity(0.1))
                                        .clipShape(Capsule())
                                }
                            }
                        }
                    }
                    .padding(12)
                    .background(Color(uiColor: .secondarySystemGroupedBackground))
                    .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
                }
            }
            .padding(16)
        }
    }

    // MARK: - Helpers

    private var emptyStateView: some View {
        VStack(spacing: 12) {
            Spacer()
            Image(systemName: "circle.hexagongrid")
                .font(.system(size: 44))
                .foregroundColor(.secondary)
            Text("未检测到当前会话的蜂群协作信息")
                .font(.system(size: 15, weight: .medium))
                .foregroundColor(.secondary)
            Spacer()
        }
    }

    private func errorView(_ msg: String) -> some View {
        VStack(spacing: 12) {
            Spacer()
            Image(systemName: "exclamationmark.triangle")
                .font(.system(size: 40))
                .foregroundColor(.orange)
            Text("加载蜂群快照失败")
                .font(.system(size: 16, weight: .semibold))
            Text(msg)
                .font(.system(size: 13))
                .foregroundColor(.secondary)
                .multilineTextAlignment(.center)
                .padding(.horizontal, 32)
            Button("重试") {
                Task { await loadSnapshot() }
            }
            .buttonStyle(.borderedProminent)
            Spacer()
        }
    }

    private func statusColor(_ status: String) -> Color {
        switch status.lowercased() {
        case "working": return .green
        case "waiting": return .orange
        case "error", "stuck": return .red
        case "killed": return .gray
        default: return .secondary
        }
    }

    private func statusText(_ status: String) -> String {
        switch status.lowercased() {
        case "working": return "工作中"
        case "waiting": return "等待中"
        case "error", "stuck": return "报错/卡住"
        case "killed": return "已终止"
        default: return status
        }
    }

    private func attentionColor(_ severity: String) -> Color {
        switch severity.lowercased() {
        case "high": return .red
        case "low": return .blue
        default: return .orange
        }
    }

    private func workstreamColor(_ state: String) -> Color {
        switch state.lowercased() {
        case "done": return .green
        case "blocked", "failed": return .red
        case "review": return .purple
        default: return .blue
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

    // MARK: - Actions

    private func loadSnapshot() async {
        guard let url = settings.serverURL else {
            errorMessage = "未连接网关"
            return
        }
        isLoading = true
        errorMessage = nil
        do {
            let snap = try await APIClient.shared.fetchSwarmSnapshot(cascadeId: cascadeId, baseURL: url)
            self.snapshot = snap
        } catch {
            self.errorMessage = error.localizedDescription
        }
        isLoading = false
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
                await loadSnapshot()
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
                await loadSnapshot()
            } catch {
                showToast("催促失败: \(error.localizedDescription)")
            }
            nudgingMembers.remove(member)
        }
    }
}
