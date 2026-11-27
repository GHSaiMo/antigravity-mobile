import SwiftUI

// MARK: - Automation Detail View (Aligned with Antigravity Desktop 2.35.2 TAd / nzd / pzd / qzd)

/// 定时任务详情与编辑页（对齐 Antigravity 桌面端 Automations Detail）
struct AutomationDetailView: View {
    let sidecar: AutomationSidecarItem
    let allEvents: [AutomationEventItem]
    let initialEditPrompt: Bool
    let onRefresh: () async -> Void
    let onDeleted: () -> Void
    let onSelectConversation: ((String) -> Void)?

    @Environment(\.dismiss) private var dismiss
    @State private var settings = AppSettings.shared

    // Busy / feedback state
    @State private var isToggling = false
    @State private var isRunningNow = false
    @State private var isRestarting = false
    @State private var isSavingTitle = false
    @State private var isSavingSchedule = false
    @State private var isDeleting = false
    @State private var errorMessage: String? = nil
    @State private var toastMessage: String? = nil
    @State private var noticeToastMessage: String? = nil
    @State private var noticeToastSeq: Int = 0

    // Title editor
    @State private var isEditingTitle = false
    @State private var draftTitle = ""

    // Schedule / Model / Project editor
    @State private var isEditingSchedule = false
    @State private var scheduleDraft = AutomationScheduleDraft()
    @State private var draftModel = ""
    @State private var draftProjectId = ""
    @State private var availableProjects: [ProjectItem] = []

    // Prompt viewing & full-screen editor sheet
    @State private var isPromptExpanded = false
    @State private var showPromptEditorSheet = false
    @State private var didApplyInitialEditPrompt = false

    // Run history expansion
    @State private var showAllEvents = false

    // Logs sheet (supports multi-file switching)
    @State private var showLogsSheet = false
    @State private var logsLoading = false
    @State private var logsText = ""
    @State private var logsError: String? = nil
    @State private var logsTruncated = false
    @State private var logFiles: [String] = []
    @State private var selectedLogFile: String = "sidecar.log"

    // Delete confirm
    @State private var showDeleteConfirm = false

    init(
        sidecar: AutomationSidecarItem,
        allEvents: [AutomationEventItem],
        initialEditPrompt: Bool = false,
        onRefresh: @escaping () async -> Void,
        onDeleted: @escaping () -> Void,
        onSelectConversation: ((String) -> Void)? = nil
    ) {
        self.sidecar = sidecar
        self.allEvents = allEvents
        self.initialEditPrompt = initialEditPrompt
        self.onRefresh = onRefresh
        self.onDeleted = onDeleted
        self.onSelectConversation = onSelectConversation
    }

    private var taskEvents: [AutomationEventItem] {
        allEvents.filter { $0.matches(sidecar: sidecar) }
    }

    private var visibleEvents: [AutomationEventItem] {
        if showAllEvents || taskEvents.count <= 8 {
            return taskEvents
        }
        return Array(taskEvents.prefix(8))
    }

    private var isAnyBusy: Bool {
        isToggling || isRunningNow || isRestarting || isDeleting
    }

    private var modelOptions: [ModelOption] {
        let live = ModelCatalogStore.shared.allOptions
        return live.isEmpty ? ModelDefaultsLogic.fallbackOptions : live
    }

    private var currentModelDisplay: String {
        guard let m = sidecar.model, !m.isEmpty else {
            return String(localized: "默认模型")
        }
        return ModelDefaultsLogic.displayName(forID: m, options: modelOptions)
    }

    private var boundProjectDisplayName: String? {
        if let pn = sidecar.projectName, !pn.isEmpty {
            return pn
        }
        if let pid = sidecar.projectId, !pid.isEmpty,
           let matched = availableProjects.first(where: { $0.rawId == pid || $0.id == pid }),
           matched.displayName.caseInsensitiveCompare(sidecar.id) != .orderedSame {
            return matched.displayName
        }
        if let uri = sidecar.workspaceUri, !uri.isEmpty {
            let cleaned = uri.replacingOccurrences(of: "file://", with: "").removingPercentEncoding ?? uri
            let base = (cleaned as NSString).lastPathComponent
            return base.isEmpty ? cleaned : base
        }
        return nil
    }

    private var detectedReadsSummary: String? {
        let info = AutomationDataSourceExtractor.extract(
            prompt: sidecar.prompt ?? "",
            accessGrants: sidecar.accessGrants ?? []
        )
        guard !info.sources.isEmpty else { return nil }
        let joined = info.sources.joined(separator: ", ")
        return info.readOnly ? "\(joined) (\(String(localized: "只读")))" : joined
    }

    private var selectableProjects: [ProjectItem] {
        availableProjects.filter { !$0.isPureChat && ($0.rawId?.isEmpty == false) }
    }

    var body: some View {
        List {
            // MARK: - Top Status & Primary Controls
            Section {
                VStack(alignment: .leading, spacing: 12) {
                    if isEditingTitle {
                        HStack(spacing: 8) {
                            TextField("任务名称", text: $draftTitle)
                                .font(.system(size: 17, weight: .semibold))
                                .textFieldStyle(.roundedBorder)
                            Button("保存") {
                                saveTitle()
                            }
                            .font(.system(size: 13, weight: .semibold))
                            .buttonStyle(.borderless)
                            .disabled(isSavingTitle || draftTitle.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
                            Button("取消") {
                                isEditingTitle = false
                            }
                            .font(.system(size: 13))
                            .foregroundColor(.secondary)
                            .buttonStyle(.borderless)
                        }
                    } else {
                        HStack(alignment: .center, spacing: 8) {
                            Text(sidecar.name)
                                .font(.system(size: 19, weight: .bold))
                                .fixedSize(horizontal: false, vertical: true)
                            Button {
                                draftTitle = sidecar.name
                                isEditingTitle = true
                            } label: {
                                Image(systemName: "pencil")
                                    .font(.system(size: 13, weight: .medium))
                                    .foregroundColor(.secondary)
                            }
                            .buttonStyle(.borderless)
                            .accessibilityLabel("编辑任务名称")
                            Spacer()
                        }
                    }

                    // Status + Run Now + Toggle
                    HStack(spacing: 10) {
                        statusBadge
                            .lineLimit(1)
                            .fixedSize(horizontal: true, vertical: false)

                        Spacer()

                        Button {
                            runNow()
                        } label: {
                            HStack(spacing: 4) {
                                if isRunningNow {
                                    ProgressView().scaleEffect(0.75)
                                } else {
                                    Image(systemName: "play.fill")
                                        .font(.system(size: 11))
                                }
                                Text("立即运行")
                                    .font(.system(size: 13, weight: .semibold))
                                    .lineLimit(1)
                            }
                            .fixedSize(horizontal: true, vertical: false)
                            .padding(.horizontal, 12)
                            .padding(.vertical, 6)
                            .background(sidecar.isRunning ? Color.indigo.opacity(0.14) : Color.secondary.opacity(0.1))
                            .foregroundColor(sidecar.isRunning ? .indigo : .secondary)
                            .clipShape(Capsule())
                        }
                        .buttonStyle(.borderless)
                        .disabled(!sidecar.isRunning || isAnyBusy)

                        Toggle(isOn: Binding(
                            get: { sidecar.isRunning },
                            set: { newVal in
                                toggleEnabled(newVal)
                            }
                        )) {
                            EmptyView()
                        }
                        .labelsHidden()
                        .disabled(isAnyBusy)
                    }

                    if let lastErr = sidecar.lastError, !lastErr.isEmpty {
                        HStack(alignment: .top, spacing: 6) {
                            Image(systemName: "exclamationmark.triangle.fill")
                                .font(.system(size: 12))
                                .foregroundColor(.red)
                                .padding(.top, 2)
                            Text(lastErr)
                                .font(.system(size: 12))
                                .foregroundColor(.red)
                        }
                        .padding(8)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .background(Color.red.opacity(0.08))
                        .clipShape(RoundedRectangle(cornerRadius: 8))
                    }

                    if let toast = toastMessage {
                        HStack(spacing: 5) {
                            Image(systemName: "checkmark.circle.fill")
                                .font(.system(size: 12))
                            Text(toast)
                                .font(.system(size: 12, weight: .medium))
                        }
                        .foregroundColor(.green)
                    }
                }
                .padding(.vertical, 4)
            }

            // MARK: - Schedule, Model & Project Pipeline Config (Desktop `nzd` equivalent)
            Section {
                if isEditingSchedule {
                    scheduleEditorView
                } else {
                    scheduleSummaryView
                }
            } header: {
                HStack {
                    Text("调度与配置")
                    Spacer()
                    if !isEditingSchedule {
                        Button {
                            beginEditingSchedule()
                        } label: {
                            Label("编辑", systemImage: "pencil")
                                .font(.system(size: 12.5, weight: .semibold))
                                .foregroundColor(.indigo)
                        }
                        .textCase(nil)
                    }
                }
            }

            // MARK: - Instructions / Prompt (Desktop `pzd` + `ozd` equivalent, full-screen sheet editor)
            Section {
                VStack(alignment: .leading, spacing: 10) {
                    if let desc = sidecar.description?.trimmingCharacters(in: .whitespacesAndNewlines), !desc.isEmpty {
                        Text(desc)
                            .font(.system(size: 12.5))
                            .foregroundColor(.secondary)
                        Divider()
                    }
                    if let reads = detectedReadsSummary {
                        HStack(spacing: 5) {
                            Image(systemName: "doc.text.magnifyingglass")
                                .font(.system(size: 11.5))
                            Text("读取范围：\(reads)")
                                .font(.system(size: 12))
                        }
                        .foregroundColor(.secondary)
                    }

                    let promptText = (sidecar.prompt ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
                    if promptText.isEmpty {
                        Button {
                            showPromptEditorSheet = true
                        } label: {
                            HStack(spacing: 6) {
                                Image(systemName: "plus.circle")
                                Text("暂无执行指令，点击编写")
                            }
                            .font(.system(size: 13.5))
                            .foregroundColor(.indigo)
                            .padding(.vertical, 4)
                        }
                        .buttonStyle(.borderless)
                    } else {
                        let isLongPrompt = promptText.count > 480
                        ZStack(alignment: .bottom) {
                            MarkdownContentView(content: promptText)
                                .environment(\.markdownFontScaleMultiplier, 0.92)
                                .frame(maxWidth: .infinity, alignment: .leading)
                                .frame(maxHeight: (isLongPrompt && !isPromptExpanded) ? 200 : nil, alignment: .top)
                                .clipped()

                            if isLongPrompt && !isPromptExpanded {
                                LinearGradient(
                                    colors: [Color(.secondarySystemGroupedBackground).opacity(0), Color(.secondarySystemGroupedBackground)],
                                    startPoint: .top,
                                    endPoint: .bottom
                                )
                                .frame(height: 44)
                                .allowsHitTesting(false)
                            }
                        }

                        if isLongPrompt {
                            Button {
                                withAnimation(.easeInOut(duration: 0.2)) {
                                    isPromptExpanded.toggle()
                                }
                            } label: {
                                HStack(spacing: 4) {
                                    Text(isPromptExpanded ? String(localized: "收起") : String(localized: "展开全部"))
                                        .font(.system(size: 12.5, weight: .semibold))
                                    Image(systemName: isPromptExpanded ? "chevron.up" : "chevron.down")
                                        .font(.system(size: 10, weight: .semibold))
                                }
                                .foregroundColor(.indigo)
                            }
                            .buttonStyle(.borderless)
                            .padding(.top, 2)
                        }
                    }
                }
                .padding(.vertical, 4)
            } header: {
                HStack {
                    Text("执行指令")
                    Spacer()
                    Button {
                        UIImpactFeedbackGenerator(style: .light).impactOccurred()
                        showPromptEditorSheet = true
                    } label: {
                        Label("编辑", systemImage: "pencil")
                            .font(.system(size: 12.5, weight: .semibold))
                            .foregroundColor(.indigo)
                    }
                    .textCase(nil)
                }
            }

            // MARK: - Run History (Desktop `qzd` equivalent with live runStatus & conversationTitle)
            Section(header: Text("运行历史 (\(taskEvents.count))")) {
                if taskEvents.isEmpty {
                    Text(sidecar.isRunning ? String(localized: "暂无运行记录") : String(localized: "任务已暂停，启用后触发记录将显示在此处"))
                        .font(.system(size: 13))
                        .foregroundColor(.secondary)
                        .padding(.vertical, 4)
                } else {
                    ForEach(visibleEvents) { ev in
                        Button {
                            guard let cid = ev.conversationId, !cid.isEmpty else { return }
                            if ev.conversationExists == false {
                                UINotificationFeedbackGenerator().notificationOccurred(.warning)
                                showNoticeToast(String(localized: "该触发记录对应的会话已删除"))
                                return
                            }
                            onSelectConversation?(cid)
                        } label: {
                            HStack(alignment: .top, spacing: 10) {
                                runStatusIcon(for: ev)
                                    .frame(width: 16, height: 16)
                                    .padding(.top, 2)

                                VStack(alignment: .leading, spacing: 3) {
                                    HStack(spacing: 6) {
                                        Text(AutomationCronHelper.formatTimestamp(ev.timestamp))
                                            .font(.system(size: 12, weight: .medium, design: .monospaced))
                                            .foregroundColor(.primary)
                                        if let badge = runStatusPillText(for: ev) {
                                            Text(badge.text)
                                                .font(.system(size: 10, weight: .semibold))
                                                .foregroundColor(badge.color)
                                                .padding(.horizontal, 5)
                                                .padding(.vertical, 1)
                                                .background(badge.color.opacity(0.12))
                                                .clipShape(Capsule())
                                        }
                                    }
                                    if let summary = eventSummaryLine(for: ev) {
                                        Text(summary)
                                            .font(.system(size: 12))
                                            .foregroundColor(ev.effectiveRunStatus == "failed" ? .red : .secondary)
                                            .lineLimit(2)
                                    }
                                }
                                Spacer(minLength: 4)
                                if ev.conversationId?.isEmpty == false {
                                    Image(systemName: "chevron.right")
                                        .font(.system(size: 11, weight: .semibold))
                                        .foregroundColor(.secondary.opacity(0.6))
                                        .padding(.top, 4)
                                }
                            }
                            .padding(.vertical, 2)
                        }
                        .disabled(ev.conversationId?.isEmpty != false)
                    }

                    if taskEvents.count > 8 {
                        Button {
                            showAllEvents.toggle()
                        } label: {
                            Text(showAllEvents ? String(localized: "收起记录") : String(localized: "查看全部 (\(taskEvents.count))"))
                                .font(.system(size: 13, weight: .medium))
                                .foregroundColor(.indigo)
                                .frame(maxWidth: .infinity, alignment: .center)
                        }
                    }
                }
            }
        }
        .listStyle(.insetGrouped)
        .overlay(alignment: .center) {
            if let msg = noticeToastMessage {
                HStack(spacing: 8) {
                    Image(systemName: "info.circle.fill")
                        .font(.system(size: 14, weight: .semibold))
                        .foregroundColor(.orange)
                    Text(msg)
                        .font(.system(size: 13.5, weight: .medium))
                        .foregroundColor(.primary)
                        .lineLimit(2)
                }
                .padding(.horizontal, 16)
                .padding(.vertical, 10)
                .background(.ultraThinMaterial)
                .clipShape(Capsule())
                .overlay(
                    Capsule()
                        .stroke(Color.primary.opacity(0.1), lineWidth: 0.7)
                )
                .shadow(color: Color.black.opacity(0.18), radius: 14, x: 0, y: 6)
                .padding(.horizontal, 24)
                .allowsHitTesting(false)
                .transition(.scale(scale: 0.88).combined(with: .opacity))
            }
        }
        .animation(.spring(response: 0.3, dampingFraction: 0.82), value: noticeToastMessage)
        .navigationTitle("任务详情")
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            ToolbarItem(placement: .topBarTrailing) {
                Menu {
                    Button {
                        showPromptEditorSheet = true
                    } label: {
                        Label("编辑执行指令", systemImage: "pencil")
                    }

                    Button {
                        restartTask()
                    } label: {
                        Label("重启任务", systemImage: "arrow.clockwise")
                    }
                    .disabled(!sidecar.isRunning || isAnyBusy)

                    Button {
                        selectedLogFile = "sidecar.log"
                        showLogsSheet = true
                        loadLogs()
                    } label: {
                        Label("查看日志", systemImage: "doc.text")
                    }

                    Divider()

                    Button(role: .destructive) {
                        showDeleteConfirm = true
                    } label: {
                        Label("删除任务", systemImage: "trash")
                    }
                    .disabled(isAnyBusy)
                } label: {
                    Image(systemName: "ellipsis.circle")
                        .font(.system(size: 17))
                }
            }
        }
        .task {
            await loadProjectsIfNeeded()
            if initialEditPrompt && !didApplyInitialEditPrompt {
                didApplyInitialEditPrompt = true
                DispatchQueue.main.asyncAfter(deadline: .now() + 0.15) {
                    showPromptEditorSheet = true
                }
            }
        }
        .refreshable {
            await onRefresh()
        }
        .sheet(isPresented: $showPromptEditorSheet) {
            AutomationPromptEditorSheet(
                taskName: sidecar.name,
                initialPrompt: sidecar.prompt ?? "",
                onSave: { updatedPrompt in
                    guard let url = settings.serverURL else { return }
                    try await APIClient.shared.updateAutomation(
                        sidecarId: sidecar.id,
                        prompt: updatedPrompt,
                        baseURL: url
                    )
                    await onRefresh()
                    showToast(String(localized: "执行指令已保存"))
                }
            )
        }
        .sheet(isPresented: $showLogsSheet) {
            logsSheetView
        }
        .confirmationDialog(
            "确认删除定时任务？",
            isPresented: $showDeleteConfirm,
            titleVisibility: .visible
        ) {
            Button("删除", role: .destructive) {
                deleteTask()
            }
            Button("取消", role: .cancel) {}
        } message: {
            Text("确定要删除定时任务「\(sidecar.name)」吗？删除后将停止调度并移除该任务配置。")
        }
        .alert("操作失败", isPresented: Binding(
            get: { errorMessage != nil },
            set: { if !$0 { errorMessage = nil } }
        )) {
            Button("确定", role: .cancel) { errorMessage = nil }
        } message: {
            Text(errorMessage ?? "")
        }
    }

    // MARK: - Subviews

    private var statusBadge: some View {
        let isErr = sidecar.status.uppercased() == "ERROR"
        let label = isErr ? String(localized: "异常") : (sidecar.isRunning ? String(localized: "运行中") : String(localized: "已暂停"))
        let color: Color = isErr ? .red : (sidecar.isRunning ? .green : .secondary)
        return HStack(spacing: 5) {
            Circle()
                .fill(color)
                .frame(width: 7, height: 7)
            Text(label)
                .font(.system(size: 12, weight: .semibold))
                .foregroundColor(color)
        }
        .padding(.horizontal, 9)
        .padding(.vertical, 4)
        .background(color.opacity(0.12))
        .clipShape(Capsule())
    }

    @ViewBuilder
    private func runStatusIcon(for ev: AutomationEventItem) -> some View {
        switch ev.effectiveRunStatus {
        case "running":
            ProgressView().scaleEffect(0.7)
        case "needs_input":
            Circle()
                .fill(Color.orange)
                .frame(width: 9, height: 9)
                .overlay(Circle().stroke(Color.orange.opacity(0.35), lineWidth: 3))
        case "failed":
            Image(systemName: "exclamationmark.circle.fill")
                .font(.system(size: 14))
                .foregroundColor(.red)
        default:
            Image(systemName: "checkmark.circle.fill")
                .font(.system(size: 14))
                .foregroundColor(.green)
        }
    }

    private func runStatusPillText(for ev: AutomationEventItem) -> (text: String, color: Color)? {
        switch ev.effectiveRunStatus {
        case "running":
            return (String(localized: "运行中"), .indigo)
        case "needs_input":
            return (String(localized: "等待确认"), .orange)
        default:
            return nil
        }
    }

    private func eventSummaryLine(for ev: AutomationEventItem) -> String? {
        if ev.effectiveRunStatus == "failed" {
            return ev.message?.isEmpty == false ? ev.message : String(localized: "执行失败")
        }
        if let title = ev.conversationTitle?.trimmingCharacters(in: .whitespacesAndNewlines),
           !title.isEmpty,
           title.caseInsensitiveCompare(sidecar.name.trimmingCharacters(in: .whitespacesAndNewlines)) != .orderedSame {
            return title
        }
        if let msg = ev.message?.trimmingCharacters(in: .whitespacesAndNewlines), !msg.isEmpty {
            return msg
        }
        return nil
    }

    @ViewBuilder
    private var scheduleSummaryView: some View {
        let rawCron = sidecar.cronExpression ?? ""
        let readable = AutomationCronHelper.humanSummary(rawCron)

        Button {
            beginEditingSchedule()
        } label: {
            VStack(alignment: .leading, spacing: 10) {
                HStack(spacing: 10) {
                    Image(systemName: "clock")
                        .foregroundColor(.indigo)
                        .frame(width: 20)
                    VStack(alignment: .leading, spacing: 2) {
                        Text(readable)
                            .font(.system(size: 14, weight: .medium))
                            .foregroundColor(.primary)
                        if !rawCron.isEmpty {
                            Text(rawCron)
                                .font(.system(size: 11.5, design: .monospaced))
                                .foregroundColor(.secondary)
                        }
                    }
                    Spacer()
                    Image(systemName: "chevron.right")
                        .font(.system(size: 11, weight: .semibold))
                        .foregroundColor(.secondary.opacity(0.45))
                }

                Divider()

                HStack(spacing: 10) {
                    Image(systemName: "sparkles")
                        .foregroundColor(.indigo)
                        .frame(width: 20)
                    Text("执行模型")
                        .font(.system(size: 13.5))
                        .foregroundColor(.secondary)
                    Spacer()
                    Text(currentModelDisplay)
                        .font(.system(size: 13.5, weight: .medium))
                        .foregroundColor(.primary)
                        .lineLimit(1)
                }

                if let projectDisplay = boundProjectDisplayName {
                    Divider()
                    HStack(spacing: 10) {
                        Image(systemName: "folder")
                            .foregroundColor(.indigo)
                            .frame(width: 20)
                        Text("绑定工作区")
                            .font(.system(size: 13.5))
                            .foregroundColor(.secondary)
                        Spacer()
                        Text(projectDisplay)
                            .font(.system(size: 13.5, weight: .medium))
                            .foregroundColor(.primary)
                            .lineLimit(1)
                    }
                }
            }
            .padding(.vertical, 2)
        }
        .buttonStyle(.plain)

        HStack(spacing: 10) {
            Image(systemName: "calendar.badge.clock")
                .foregroundColor(.indigo)
                .frame(width: 20)
            Text("下次运行")
                .font(.system(size: 13.5))
                .foregroundColor(.secondary)
            Spacer()
            if !sidecar.isRunning {
                Text("已暂停")
                    .font(.system(size: 13))
                    .foregroundColor(.secondary)
            } else if let next = sidecar.nextRunAt, !next.isEmpty {
                Text(AutomationCronHelper.formatTimestamp(next))
                    .font(.system(size: 12.5, design: .monospaced))
                    .foregroundColor(.primary)
            } else {
                Text("等待调度")
                    .font(.system(size: 13))
                    .foregroundColor(.secondary)
            }
        }

        if let last = sidecar.lastRunAt, !last.isEmpty {
            HStack(spacing: 10) {
                Image(systemName: "clock.arrow.circlepath")
                    .foregroundColor(.secondary)
                    .frame(width: 20)
                Text("上次运行")
                    .font(.system(size: 13.5))
                    .foregroundColor(.secondary)
                Spacer()
                Text(AutomationCronHelper.formatTimestamp(last))
                    .font(.system(size: 12.5, design: .monospaced))
                    .foregroundColor(.secondary)
            }
        }
    }

    @ViewBuilder
    private var scheduleEditorView: some View {
        VStack(alignment: .leading, spacing: 12) {
            AutomationScheduleEditorView(draft: $scheduleDraft)

            Divider()

            Picker("执行模型", selection: $draftModel) {
                Text("默认模型").tag("")
                ForEach(modelOptions) { opt in
                    Text(opt.name).tag(opt.id)
                }
                if !draftModel.isEmpty && !modelOptions.contains(where: { $0.id == draftModel }) {
                    Text(draftModel).tag(draftModel)
                }
            }

            if !selectableProjects.isEmpty {
                Picker("绑定项目", selection: $draftProjectId) {
                    Text("默认 / 不指定").tag("")
                    ForEach(selectableProjects) { proj in
                        if let pid = proj.rawId {
                            Text(proj.displayName).tag(pid)
                        }
                    }
                    if !draftProjectId.isEmpty && !selectableProjects.contains(where: { $0.rawId == draftProjectId }) {
                        Text(boundProjectDisplayName ?? draftProjectId).tag(draftProjectId)
                    }
                }
            }

            HStack(spacing: 12) {
                Button {
                    saveScheduleAndModel()
                } label: {
                    Text(isSavingSchedule ? String(localized: "保存中...") : String(localized: "保存配置"))
                        .font(.system(size: 13, weight: .semibold))
                        .padding(.horizontal, 14)
                        .padding(.vertical, 7)
                        .background(Color.indigo)
                        .foregroundColor(.white)
                        .clipShape(Capsule())
                }
                .buttonStyle(.borderless)
                .disabled(isSavingSchedule || scheduleDraft.resolvedCron().isEmpty)

                Button("取消") {
                    isEditingSchedule = false
                }
                .font(.system(size: 13))
                .foregroundColor(.secondary)
                .buttonStyle(.borderless)
                Spacer()
            }
        }
        .padding(.vertical, 4)
    }

    private var logsSheetView: some View {
        NavigationStack {
            VStack(spacing: 0) {
                if logFiles.count > 1 {
                    ScrollView(.horizontal, showsIndicators: false) {
                        HStack(spacing: 8) {
                            ForEach(logFiles, id: \.self) { file in
                                let active = file == selectedLogFile
                                Button {
                                    selectedLogFile = file
                                    loadLogs()
                                } label: {
                                    Text(file)
                                        .font(.system(size: 12, weight: active ? .semibold : .regular, design: .monospaced))
                                        .padding(.horizontal, 10)
                                        .padding(.vertical, 5)
                                        .background(active ? Color.indigo.opacity(0.16) : Color(.secondarySystemBackground))
                                        .foregroundColor(active ? .indigo : .secondary)
                                        .clipShape(Capsule())
                                }
                                .buttonStyle(.plain)
                            }
                        }
                        .padding(.horizontal, 14)
                        .padding(.vertical, 8)
                    }
                    Divider()
                }

                Group {
                    if logsLoading && logsText.isEmpty {
                        VStack(spacing: 10) {
                            Spacer()
                            ProgressView()
                            Text("正在加载运行日志...")
                                .font(.system(size: 13))
                                .foregroundColor(.secondary)
                            Spacer()
                        }
                    } else if let err = logsError {
                        VStack(spacing: 8) {
                            Spacer()
                            Image(systemName: "exclamationmark.triangle")
                                .font(.system(size: 28))
                                .foregroundColor(.orange)
                            Text(err)
                                .font(.system(size: 13))
                                .foregroundColor(.secondary)
                                .multilineTextAlignment(.center)
                                .padding(.horizontal, 24)
                            Spacer()
                        }
                    } else if logsText.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                        VStack {
                            Spacer()
                            Text("暂无日志输出")
                                .font(.system(size: 14))
                                .foregroundColor(.secondary)
                            Spacer()
                        }
                    } else {
                        ScrollView {
                            VStack(alignment: .leading, spacing: 6) {
                                if logsTruncated {
                                    Text("仅展示最近 200 行日志")
                                        .font(.system(size: 11))
                                        .foregroundColor(.secondary)
                                }
                                Text(logsText)
                                    .font(.system(size: 11.5, design: .monospaced))
                                    .textSelection(.enabled)
                                    .frame(maxWidth: .infinity, alignment: .leading)
                            }
                            .padding(14)
                        }
                    }
                }
                .frame(maxWidth: .infinity, maxHeight: .infinity)
            }
            .navigationTitle("运行日志")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    Button("关闭") { showLogsSheet = false }
                }
                ToolbarItemGroup(placement: .topBarTrailing) {
                    if !logsText.isEmpty {
                        Button {
                            UIPasteboard.general.string = logsText
                        } label: {
                            Image(systemName: "doc.on.doc")
                        }
                    }
                    Button {
                        loadLogs()
                    } label: {
                        Image(systemName: "arrow.clockwise")
                    }
                    .disabled(logsLoading)
                }
            }
        }
        .presentationDetents([.medium, .large])
    }

    // MARK: - Actions

    private func loadProjectsIfNeeded() async {
        let cached = ProjectCacheManager.shared.loadProjects()
        if !cached.isEmpty {
            availableProjects = cached
        }
        if let url = settings.serverURL, let live = try? await APIClient.shared.fetchProjects(baseURL: url), !live.isEmpty {
            availableProjects = live
        }
    }

    private func beginEditingSchedule() {
        scheduleDraft = AutomationScheduleDraft.from(
            cronExpression: sidecar.cronExpression ?? "0 9 * * *",
            fallbackTimeZone: sidecar.timeZone
        )
        draftModel = sidecar.model ?? ""
        draftProjectId = sidecar.projectId ?? ""
        isEditingSchedule = true
    }

    private func showToast(_ msg: String) {
        toastMessage = msg
        DispatchQueue.main.asyncAfter(deadline: .now() + 2.5) {
            if toastMessage == msg { toastMessage = nil }
        }
    }

    private func showNoticeToast(_ msg: String) {
        noticeToastSeq += 1
        let seq = noticeToastSeq
        withAnimation(.spring(response: 0.3, dampingFraction: 0.82)) {
            noticeToastMessage = msg
        }
        DispatchQueue.main.asyncAfter(deadline: .now() + 2.2) {
            if noticeToastSeq == seq {
                withAnimation(.easeOut(duration: 0.2)) {
                    noticeToastMessage = nil
                }
            }
        }
    }

    private func toggleEnabled(_ enabled: Bool) {
        guard let url = settings.serverURL else { return }
        isToggling = true
        UIImpactFeedbackGenerator(style: .medium).impactOccurred()
        Task {
            defer { isToggling = false }
            do {
                try await APIClient.shared.manageAutomation(action: enabled ? "start" : "stop", sidecarId: sidecar.id, baseURL: url)
                await onRefresh()
                showToast(enabled ? String(localized: "已启用定时任务") : String(localized: "已暂停定时任务"))
            } catch {
                errorMessage = error.localizedDescription
            }
        }
    }

    private func runNow() {
        guard sidecar.isRunning, let url = settings.serverURL else { return }
        isRunningNow = true
        UIImpactFeedbackGenerator(style: .medium).impactOccurred()
        Task {
            defer { isRunningNow = false }
            do {
                try await APIClient.shared.manageAutomation(action: "run_now", sidecarId: sidecar.id, baseURL: url)
                await onRefresh()
                showToast(String(localized: "已触发立即运行"))
            } catch {
                errorMessage = error.localizedDescription
            }
        }
    }

    private func restartTask() {
        guard sidecar.isRunning, let url = settings.serverURL else { return }
        isRestarting = true
        Task {
            defer { isRestarting = false }
            do {
                try await APIClient.shared.manageAutomation(action: "restart", sidecarId: sidecar.id, baseURL: url)
                await onRefresh()
                showToast(String(localized: "已重启任务"))
            } catch {
                errorMessage = error.localizedDescription
            }
        }
    }

    private func saveTitle() {
        guard let url = settings.serverURL else { return }
        let trimmed = draftTitle.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return }
        isSavingTitle = true
        Task {
            defer { isSavingTitle = false }
            do {
                try await APIClient.shared.updateAutomation(sidecarId: sidecar.id, name: trimmed, baseURL: url)
                await onRefresh()
                isEditingTitle = false
                showToast(String(localized: "已更新任务名称"))
            } catch {
                errorMessage = error.localizedDescription
            }
        }
    }

    private func saveScheduleAndModel() {
        guard let url = settings.serverURL else { return }
        let cronStr = scheduleDraft.resolvedCron()
        guard !cronStr.isEmpty else { return }
        let origProject = sidecar.projectId ?? ""
        let changedProject: String? = (draftProjectId != origProject) ? draftProjectId : nil
        isSavingSchedule = true
        Task {
            defer { isSavingSchedule = false }
            do {
                try await APIClient.shared.updateAutomation(
                    sidecarId: sidecar.id,
                    cron: cronStr,
                    model: draftModel,
                    projectId: changedProject,
                    baseURL: url
                )
                await onRefresh()
                isEditingSchedule = false
                showToast(String(localized: "调度配置已保存"))
            } catch {
                errorMessage = error.localizedDescription
            }
        }
    }

    private func deleteTask() {
        guard let url = settings.serverURL else { return }
        isDeleting = true
        Task {
            defer { isDeleting = false }
            do {
                try await APIClient.shared.deleteAutomation(sidecarId: sidecar.id, baseURL: url)
                await onRefresh()
                onDeleted()
                dismiss()
            } catch {
                errorMessage = error.localizedDescription
            }
        }
    }

    private func loadLogs() {
        guard let url = settings.serverURL else { return }
        logsLoading = true
        logsError = nil
        let targetFile = selectedLogFile
        Task {
            defer { logsLoading = false }
            do {
                let res = try await APIClient.shared.fetchAutomationLogs(
                    sidecarId: sidecar.id,
                    tail: 200,
                    file: targetFile,
                    baseURL: url
                )
                logsText = res.logs
                logsTruncated = res.truncated
                if !res.files.isEmpty {
                    logFiles = res.files
                }
                if !res.filename.isEmpty {
                    selectedLogFile = res.filename
                }
            } catch {
                logsError = error.localizedDescription
            }
        }
    }
}

// MARK: - Full-Height Native Prompt Editor Sheet

struct AutomationPromptEditorSheet: View {
    let taskName: String
    let initialPrompt: String
    let onSave: (String) async throws -> Void

    @Environment(\.dismiss) private var dismiss
    @State private var draftPrompt: String
    @State private var isSaving = false
    @State private var errorMessage: String? = nil
    @FocusState private var isFocused: Bool

    init(taskName: String, initialPrompt: String, onSave: @escaping (String) async throws -> Void) {
        self.taskName = taskName
        self.initialPrompt = initialPrompt
        self.onSave = onSave
        self._draftPrompt = State(initialValue: initialPrompt)
    }

    private var trimmedDraft: String {
        draftPrompt.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    private var isDirty: Bool {
        trimmedDraft != initialPrompt.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    var body: some View {
        NavigationStack {
            VStack(spacing: 0) {
                HStack(spacing: 6) {
                    Image(systemName: "clock.badge.checkmark")
                        .font(.system(size: 12))
                        .foregroundColor(.indigo)
                    Text(taskName)
                        .font(.system(size: 12.5, weight: .medium))
                        .foregroundColor(.secondary)
                        .lineLimit(1)
                    Spacer()
                    Text("\(trimmedDraft.count) \(String(localized: "字符"))")
                        .font(.system(size: 11.5, design: .monospaced))
                        .foregroundColor(.secondary)
                }
                .padding(.horizontal, 16)
                .padding(.vertical, 8)
                .background(Color(.secondarySystemBackground))

                Divider()

                ZStack(alignment: .topLeading) {
                    if draftPrompt.isEmpty {
                        Text("输入每次触发时交给 Agent 执行的完整指令…")
                            .font(.system(size: 14.5))
                            .foregroundColor(Color(.placeholderText))
                            .padding(.horizontal, 16)
                            .padding(.vertical, 14)
                            .allowsHitTesting(false)
                    }
                    TextEditor(text: $draftPrompt)
                        .font(.system(size: 14.5))
                        .padding(.horizontal, 12)
                        .padding(.vertical, 8)
                        .scrollContentBackground(.hidden)
                        .focused($isFocused)
                }
                .frame(maxWidth: .infinity, maxHeight: .infinity)
            }
            .navigationTitle("编辑执行指令")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("取消") { dismiss() }
                        .disabled(isSaving)
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button(isSaving ? String(localized: "保存中...") : String(localized: "保存")) {
                        submit()
                    }
                    .fontWeight(.semibold)
                    .disabled(isSaving || trimmedDraft.isEmpty || !isDirty)
                }
            }
            .alert("保存失败", isPresented: Binding(
                get: { errorMessage != nil },
                set: { if !$0 { errorMessage = nil } }
            )) {
                Button("确定", role: .cancel) { errorMessage = nil }
            } message: {
                Text(errorMessage ?? "")
            }
            .onAppear {
                DispatchQueue.main.asyncAfter(deadline: .now() + 0.2) {
                    isFocused = true
                }
            }
        }
        .presentationDetents([.large])
        .presentationDragIndicator(.visible)
    }

    private func submit() {
        guard !trimmedDraft.isEmpty else { return }
        isSaving = true
        Task {
            defer { isSaving = false }
            do {
                try await onSave(trimmedDraft)
                dismiss()
            } catch {
                errorMessage = error.localizedDescription
            }
        }
    }
}

// MARK: - Structured Create Automation Sheet (Desktop `tBd` equivalent)

struct CreateAutomationSheet: View {
    let existingIds: Set<String>
    let onCreated: () async -> Void

    @Environment(\.dismiss) private var dismiss
    @State private var settings = AppSettings.shared

    @State private var name: String = ""
    @State private var scheduleDraft: AutomationScheduleDraft = {
        var d = AutomationScheduleDraft()
        d.timeZone = TimeZone.current.identifier
        return d
    }()
    @State private var selectedModel: String = ""
    @State private var selectedProjectId: String = ""
    @State private var prompt: String = ""
    @State private var availableProjects: [ProjectItem] = []
    @State private var isCreating: Bool = false
    @State private var errorMessage: String? = nil

    private var modelOptions: [ModelOption] {
        let live = ModelCatalogStore.shared.allOptions
        return live.isEmpty ? ModelDefaultsLogic.fallbackOptions : live
    }

    private var selectableProjects: [ProjectItem] {
        availableProjects.filter { !$0.isPureChat && ($0.rawId?.isEmpty == false) }
    }

    private var slugPreview: String {
        AutomationSlugHelper.slug(for: name)
    }

    private var isDuplicateName: Bool {
        let slug = slugPreview
        guard !slug.isEmpty else { return false }
        return existingIds.contains(slug) || existingIds.contains("schedule-\(slug)")
    }

    private var canSubmit: Bool {
        !isCreating &&
        !name.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty &&
        !isDuplicateName &&
        !scheduleDraft.resolvedCron().isEmpty &&
        !prompt.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
    }

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    TextField("例如：晨间 PR 汇总", text: $name)
                    if isDuplicateName {
                        Label("已存在同名任务（ID: \(slugPreview)），请更换名称", systemImage: "exclamationmark.triangle.fill")
                            .font(.system(size: 12))
                            .foregroundColor(.red)
                    } else if !slugPreview.isEmpty {
                        Text("ID: \(slugPreview)")
                            .font(.system(size: 11.5, design: .monospaced))
                            .foregroundColor(.secondary)
                    }
                } header: {
                    Text("任务名称")
                }

                Section {
                    AutomationScheduleEditorView(draft: $scheduleDraft)
                    let cronPreview = scheduleDraft.resolvedCron()
                    if !cronPreview.isEmpty {
                        HStack(spacing: 6) {
                            Image(systemName: "clock")
                                .font(.system(size: 11.5))
                                .foregroundColor(.indigo)
                            Text(AutomationCronHelper.humanSummary(cronPreview))
                                .font(.system(size: 12.5, weight: .medium))
                            Spacer()
                        }
                        .padding(.top, 2)
                    }
                } header: {
                    Text("执行频率与时间")
                }

                Section {
                    Picker("执行模型", selection: $selectedModel) {
                        Text("默认模型").tag("")
                        ForEach(modelOptions) { opt in
                            Text(opt.name).tag(opt.id)
                        }
                    }
                    if !selectableProjects.isEmpty {
                        Picker("绑定项目", selection: $selectedProjectId) {
                            Text("默认 / 不指定").tag("")
                            ForEach(selectableProjects) { proj in
                                if let pid = proj.rawId {
                                    Text(proj.displayName).tag(pid)
                                }
                            }
                        }
                    }
                } header: {
                    Text("运行环境")
                }

                Section {
                    ZStack(alignment: .topLeading) {
                        if prompt.isEmpty {
                            Text("描述每次触发时 Agent 需要完成的任务、数据源和输出格式…")
                                .font(.system(size: 14))
                                .foregroundColor(Color(.placeholderText))
                                .padding(.top, 8)
                                .padding(.leading, 4)
                                .allowsHitTesting(false)
                        }
                        TextEditor(text: $prompt)
                            .font(.system(size: 14))
                            .frame(minHeight: 150)
                    }
                } header: {
                    Text("执行指令")
                }
            }
            .navigationTitle("新建定时任务")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("取消") { dismiss() }
                        .disabled(isCreating)
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button(isCreating ? String(localized: "创建中...") : String(localized: "创建")) {
                        submitCreate()
                    }
                    .fontWeight(.semibold)
                    .disabled(!canSubmit)
                }
            }
            .task {
                let cached = ProjectCacheManager.shared.loadProjects()
                if !cached.isEmpty { availableProjects = cached }
                if let url = settings.serverURL, let live = try? await APIClient.shared.fetchProjects(baseURL: url), !live.isEmpty {
                    availableProjects = live
                }
            }
            .alert("创建失败", isPresented: Binding(
                get: { errorMessage != nil },
                set: { if !$0 { errorMessage = nil } }
            )) {
                Button("确定", role: .cancel) { errorMessage = nil }
            } message: {
                Text(errorMessage ?? "")
            }
        }
        .presentationDetents([.large])
        .presentationDragIndicator(.visible)
    }

    private func submitCreate() {
        guard let url = settings.gatewayURL else { return }
        let trimmedName = name.trimmingCharacters(in: .whitespacesAndNewlines)
        let cronStr = scheduleDraft.resolvedCron()
        let trimmedPrompt = prompt.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmedName.isEmpty, !cronStr.isEmpty, !trimmedPrompt.isEmpty else { return }
        isCreating = true
        Task {
            defer { isCreating = false }
            do {
                try await APIClient.shared.createAutomation(
                    name: trimmedName,
                    schedule: cronStr,
                    prompt: trimmedPrompt,
                    model: selectedModel,
                    projectId: selectedProjectId,
                    baseURL: url
                )
                await onCreated()
                dismiss()
            } catch {
                errorMessage = error.localizedDescription
            }
        }
    }
}

// MARK: - Shared Visual Schedule Editor (Desktop `Cyd` equivalent)

struct AutomationScheduleDraft: Equatable {
    enum Frequency: String, CaseIterable, Identifiable {
        case hourly = "每小时"
        case daily = "每天"
        case weekdays = "工作日"
        case weekly = "每周"
        case custom = "自定义"
        var id: String { rawValue }
    }

    enum CustomUnit: String, CaseIterable, Identifiable {
        case everyHours = "每隔几小时"
        case multiWeekday = "每周多选"
        case monthly = "每月"
        case rawCron = "Cron 表达式"
        var id: String { rawValue }
    }

    var frequency: Frequency = .daily
    var hour: Int = 9
    var minute: Int = 0
    var weekday: Int = 1 // 1=Mon ... 6=Sat, 0=Sun
    var customUnit: CustomUnit = .everyHours
    var everyHours: Int = 2
    var multiWeekdays: Set<Int> = [1, 3, 5]
    var dayOfMonth: Int = 1
    var rawCron: String = "0 9 * * *"
    var timeZone: String = ""

    static let everyHourChoices = [2, 3, 4, 6, 8, 12]
    static let orderedWeekdays = [1, 2, 3, 4, 5, 6, 0]

    static func from(cronExpression: String, fallbackTimeZone: String? = nil) -> AutomationScheduleDraft {
        var d = AutomationScheduleDraft()
        let parsed = AutomationCronHelper.splitTimeZone(cronExpression)
        d.timeZone = parsed.timeZone ?? (fallbackTimeZone ?? "")
        d.rawCron = cronExpression.trimmingCharacters(in: .whitespacesAndNewlines)

        let parts = parsed.cron.split(whereSeparator: \.isWhitespace).map(String.init)
        guard parts.count == 5 else {
            d.frequency = .custom
            d.customUnit = .rawCron
            return d
        }
        let minStr = parts[0], hourStr = parts[1], domStr = parts[2], monStr = parts[3], dowStr = parts[4]

        if minStr == "0" && hourStr == "*" && domStr == "*" && monStr == "*" && dowStr == "*" {
            d.frequency = .hourly
            return d
        }
        if minStr == "0", hourStr.hasPrefix("*/"),
           let step = Int(hourStr.dropFirst(2)), everyHourChoices.contains(step),
           domStr == "*" && monStr == "*" && dowStr == "*" {
            d.frequency = .custom
            d.customUnit = .everyHours
            d.everyHours = step
            return d
        }
        if let m = Int(minStr), (0...59).contains(m),
           let h = Int(hourStr), (0...23).contains(h),
           monStr == "*" {
            d.hour = h
            d.minute = m
            if domStr == "*" && dowStr == "*" {
                d.frequency = .daily
                return d
            }
            if domStr == "*" && dowStr == "1-5" {
                d.frequency = .weekdays
                return d
            }
            if domStr == "*", let dow = Int(dowStr), (0...7).contains(dow) {
                d.frequency = .weekly
                d.weekday = (dow == 7) ? 0 : dow
                return d
            }
            if domStr == "*", dowStr.contains(",") {
                let rawDays = dowStr.split(separator: ",").compactMap { Int($0) }.filter { (0...7).contains($0) }.map { $0 == 7 ? 0 : $0 }
                if !rawDays.isEmpty {
                    d.frequency = .custom
                    d.customUnit = .multiWeekday
                    d.multiWeekdays = Set(rawDays)
                    return d
                }
            }
            if dowStr == "*", let dom = Int(domStr), (1...28).contains(dom) {
                d.frequency = .custom
                d.customUnit = .monthly
                d.dayOfMonth = dom
                return d
            }
        }

        d.frequency = .custom
        d.customUnit = .rawCron
        return d
    }

    func resolvedCron() -> String {
        switch frequency {
        case .hourly:
            return AutomationCronHelper.combine(timeZone: timeZone, cron: "0 * * * *")
        case .daily:
            return AutomationCronHelper.combine(timeZone: timeZone, cron: "\(minute) \(hour) * * *")
        case .weekdays:
            return AutomationCronHelper.combine(timeZone: timeZone, cron: "\(minute) \(hour) * * 1-5")
        case .weekly:
            return AutomationCronHelper.combine(timeZone: timeZone, cron: "\(minute) \(hour) * * \(weekday)")
        case .custom:
            switch customUnit {
            case .everyHours:
                return AutomationCronHelper.combine(timeZone: timeZone, cron: "0 */\(everyHours) * * *")
            case .multiWeekday:
                let sorted = Self.orderedWeekdays.filter { multiWeekdays.contains($0) }
                let list = sorted.isEmpty ? "1" : sorted.map(String.init).joined(separator: ",")
                return AutomationCronHelper.combine(timeZone: timeZone, cron: "\(minute) \(hour) * * \(list)")
            case .monthly:
                return AutomationCronHelper.combine(timeZone: timeZone, cron: "\(minute) \(hour) \(dayOfMonth) * *")
            case .rawCron:
                return rawCron.trimmingCharacters(in: .whitespacesAndNewlines)
            }
        }
    }
}

struct AutomationScheduleEditorView: View {
    @Binding var draft: AutomationScheduleDraft

    private var needsTimePicker: Bool {
        switch draft.frequency {
        case .hourly: return false
        case .daily, .weekdays, .weekly: return true
        case .custom: return draft.customUnit == .multiWeekday || draft.customUnit == .monthly
        }
    }

    private var timeBinding: Binding<Date> {
        Binding(
            get: {
                var comps = Calendar.current.dateComponents([.year, .month, .day], from: Date())
                comps.hour = draft.hour
                comps.minute = draft.minute
                return Calendar.current.date(from: comps) ?? Date()
            },
            set: { newDate in
                let comps = Calendar.current.dateComponents([.hour, .minute], from: newDate)
                draft.hour = comps.hour ?? 9
                draft.minute = comps.minute ?? 0
            }
        )
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Picker("执行频率", selection: $draft.frequency) {
                ForEach(AutomationScheduleDraft.Frequency.allCases) { f in
                    Text(LocalizedStringKey(f.rawValue)).tag(f)
                }
            }
            .pickerStyle(.menu)

            if draft.frequency == .hourly {
                Text("每小时整点自动执行（0 * * * *）")
                    .font(.system(size: 12.5))
                    .foregroundColor(.secondary)
            } else if draft.frequency == .weekly {
                Picker("星期", selection: $draft.weekday) {
                    Text("周一").tag(1)
                    Text("周二").tag(2)
                    Text("周三").tag(3)
                    Text("周四").tag(4)
                    Text("周五").tag(5)
                    Text("周六").tag(6)
                    Text("周日").tag(0)
                }
            } else if draft.frequency == .custom {
                Picker("重复方式", selection: $draft.customUnit) {
                    ForEach(AutomationScheduleDraft.CustomUnit.allCases) { u in
                        Text(LocalizedStringKey(u.rawValue)).tag(u)
                    }
                }
                .pickerStyle(.menu)

                switch draft.customUnit {
                case .everyHours:
                    Picker("间隔小时", selection: $draft.everyHours) {
                        ForEach(AutomationScheduleDraft.everyHourChoices, id: \.self) { n in
                            Text("每 \(n) 小时").tag(n)
                        }
                    }
                case .multiWeekday:
                    HStack(spacing: 6) {
                        ForEach(AutomationScheduleDraft.orderedWeekdays, id: \.self) { day in
                            let selected = draft.multiWeekdays.contains(day)
                            Button {
                                if selected && draft.multiWeekdays.count > 1 {
                                    draft.multiWeekdays.remove(day)
                                } else {
                                    draft.multiWeekdays.insert(day)
                                }
                            } label: {
                                Text(AutomationCronHelper.shortWeekdayName(day))
                                    .font(.system(size: 12, weight: selected ? .semibold : .medium))
                                    .frame(maxWidth: .infinity)
                                    .padding(.vertical, 6)
                                    .background(selected ? Color.indigo : Color(.tertiarySystemFill))
                                    .foregroundColor(selected ? .white : .primary)
                                    .clipShape(RoundedRectangle(cornerRadius: 7))
                            }
                            .buttonStyle(.borderless)
                        }
                    }
                case .monthly:
                    Picker("每月日期", selection: $draft.dayOfMonth) {
                        ForEach(1...28, id: \.self) { day in
                            Text("每月 \(day) 日").tag(day)
                        }
                    }
                case .rawCron:
                    VStack(alignment: .leading, spacing: 4) {
                        Text("Cron 表达式")
                            .font(.system(size: 12))
                            .foregroundColor(.secondary)
                        TextField("例如 0 9 * * *", text: $draft.rawCron)
                            .font(.system(size: 13.5, design: .monospaced))
                            .textFieldStyle(.roundedBorder)
                            .textInputAutocapitalization(.never)
                            .autocorrectionDisabled()
                    }
                }
            }

            if needsTimePicker {
                DatePicker("触发时间", selection: timeBinding, displayedComponents: .hourAndMinute)
                    .datePickerStyle(.compact)
            }

            if !draft.timeZone.isEmpty && !(draft.frequency == .custom && draft.customUnit == .rawCron) {
                Text("时区：\(draft.timeZone)")
                    .font(.system(size: 11.5))
                    .foregroundColor(.secondary)
            }
        }
    }
}

// MARK: - Helpers (Slug, DataSource Extraction, Cron Parsing & Formatting)

enum AutomationSlugHelper {
    static func slug(for raw: String) -> String {
        let lower = raw.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        guard !lower.isEmpty else { return "" }
        var out = ""
        var prevDash = false
        for scalar in lower.unicodeScalars {
            if CharacterSet.alphanumerics.contains(scalar) && scalar.isASCII {
                out.unicodeScalars.append(scalar)
                prevDash = false
            } else if !prevDash && !out.isEmpty {
                out.append("-")
                prevDash = true
            }
        }
        return out.trimmingCharacters(in: CharacterSet(charactersIn: "-"))
    }
}

enum AutomationDataSourceExtractor {
    private static let patterns: [(regex: String, label: String)] = [
        (#"\bgcalendar\b|calendar"#, "Calendar"),
        (#"\bgmail\b"#, "Gmail"),
        (#"\bgchat\b|google chat"#, "Chat"),
        (#"\bgdocs\b|google docs"#, "Docs"),
        (#"\bgdrive\b|google drive"#, "Drive"),
        (#"\bgsheets\b"#, "Sheets"),
        (#"\bgh\s+|github"#, "GitHub")
    ]

    static func extract(prompt: String, accessGrants: [String]) -> (sources: [String], readOnly: Bool) {
        let corpus = ([prompt] + accessGrants).joined(separator: "\n")
        var found: [String] = []
        for entry in patterns {
            if let re = try? NSRegularExpression(pattern: entry.regex, options: [.caseInsensitive]),
               re.firstMatch(in: corpus, range: NSRange(corpus.startIndex..., in: corpus)) != nil {
                found.append(entry.label)
            }
        }
        let readOnly = !accessGrants.isEmpty && accessGrants.allSatisfy { grant in
            grant.hasPrefix("read_file(") || grant.hasPrefix("read_url(")
        }
        return (found, readOnly)
    }
}

enum AutomationCronHelper {
    private static let isoFractional: ISO8601DateFormatter = {
        let f = ISO8601DateFormatter()
        f.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return f
    }()

    private static let isoPlain: ISO8601DateFormatter = {
        let f = ISO8601DateFormatter()
        f.formatOptions = [.withInternetDateTime]
        return f
    }()

    private static let sqlParser: DateFormatter = {
        let f = DateFormatter()
        f.locale = Locale(identifier: "en_US_POSIX")
        f.dateFormat = "yyyy-MM-dd HH:mm:ss"
        return f
    }()

    private static let compactDisplay: DateFormatter = {
        let f = DateFormatter()
        f.locale = Locale(identifier: "en_US_POSIX")
        f.timeZone = .autoupdatingCurrent
        f.dateFormat = "MM/dd HH:mm"
        return f
    }()

    static func shortWeekdayName(_ day: Int) -> String {
        switch day {
        case 1: return String(localized: "周一")
        case 2: return String(localized: "周二")
        case 3: return String(localized: "周三")
        case 4: return String(localized: "周四")
        case 5: return String(localized: "周五")
        case 6: return String(localized: "周六")
        default: return String(localized: "周日")
        }
    }

    static func parseDate(_ raw: String) -> Date? {
        let trimmed = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return nil }
        return isoFractional.date(from: trimmed)
            ?? isoPlain.date(from: trimmed)
            ?? sqlParser.date(from: trimmed)
    }

    /// Formats ISO-8601 UTC or SQL timestamps into compact local `MM/dd HH:mm` for cron cards & details.
    static func formatTimestamp(_ raw: String) -> String {
        let trimmed = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return raw }
        if let date = parseDate(trimmed) {
            return compactDisplay.string(from: date)
        }
        return trimmed
    }

    static func splitTimeZone(_ raw: String) -> (timeZone: String?, cron: String) {
        let trimmed = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        let prefixes = ["CRON_TZ=", "TZ="]
        for pref in prefixes {
            if trimmed.hasPrefix(pref) {
                let rest = String(trimmed.dropFirst(pref.count))
                if let spaceIdx = rest.firstIndex(where: { $0.isWhitespace }) {
                    let tz = String(rest[..<spaceIdx])
                    let cron = rest[spaceIdx...].trimmingCharacters(in: .whitespacesAndNewlines)
                    return (tz, cron)
                }
            }
        }
        return (nil, trimmed)
    }

    static func combine(timeZone: String, cron: String) -> String {
        let tz = timeZone.trimmingCharacters(in: .whitespacesAndNewlines)
        let c = cron.trimmingCharacters(in: .whitespacesAndNewlines)
        return tz.isEmpty ? c : "CRON_TZ=\(tz) \(c)"
    }

    static func humanSummary(_ raw: String) -> String {
        let (tz, cron) = splitTimeZone(raw)
        let parts = cron.split(whereSeparator: \.isWhitespace).map(String.init)
        var base = cron.isEmpty ? String(localized: "未配置调度") : cron
        if parts.count == 5 {
            let min = parts[0], hour = parts[1], dom = parts[2], mon = parts[3], dow = parts[4]
            if min == "0" && hour == "*" && dom == "*" && mon == "*" && dow == "*" {
                base = String(localized: "每小时整点")
            } else if min == "0", hour.hasPrefix("*/"), let step = Int(hour.dropFirst(2)), dom == "*" && mon == "*" && dow == "*" {
                base = String(localized: "每 \(step) 小时")
            } else if let m = Int(min), let h = Int(hour), (0...59).contains(m), (0...23).contains(h), mon == "*" {
                let hm = String(format: "%02d:%02d", h, m)
                if dom == "*" {
                    if dow == "*" {
                        base = String(localized: "每天 \(hm)")
                    } else if dow == "1-5" {
                        base = String(localized: "工作日 \(hm)")
                    } else if let d = Int(dow), (0...7).contains(d) {
                        let name = shortWeekdayName(d == 7 ? 0 : d)
                        base = String(localized: "每\(name) \(hm)")
                    } else if dow.contains(",") {
                        let days = dow.split(separator: ",").compactMap { Int($0) }.filter { (0...7).contains($0) }.map { shortWeekdayName($0 == 7 ? 0 : $0) }
                        if !days.isEmpty {
                            base = "\(days.joined(separator: "、")) \(hm)"
                        }
                    }
                } else if dow == "*", let dayNum = Int(dom), (1...31).contains(dayNum) {
                    base = String(localized: "每月 \(dayNum) 日 \(hm)")
                }
            }
        }
        if let tz, !tz.isEmpty {
            return "\(base) (\(tz))"
        }
        return base
    }
}
