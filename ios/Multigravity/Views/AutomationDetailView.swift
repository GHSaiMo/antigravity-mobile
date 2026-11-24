import SwiftUI

/// 定时任务详情与轻量编辑页（对齐桌面端 Antigravity Automation Detail 页面）
struct AutomationDetailView: View {
    let sidecar: AutomationSidecarItem
    let allEvents: [AutomationEventItem]
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
    @State private var isSavingPrompt = false
    @State private var isDeleting = false
    @State private var errorMessage: String? = nil
    @State private var toastMessage: String? = nil
    @State private var noticeToastMessage: String? = nil
    @State private var noticeToastSeq: Int = 0

    // Inline editors
    @State private var isEditingTitle = false
    @State private var draftTitle = ""

    @State private var isEditingSchedule = false
    @State private var draftPreset: SchedulePreset = .custom
    @State private var draftHour: Int = 9
    @State private var draftWeekday: Int = 1 // 1 = Monday ... 0/7 = Sunday
    @State private var draftCustomCron = ""
    @State private var draftTimeZone = ""
    @State private var draftModel = ""

    @State private var isEditingPrompt = false
    @State private var draftPrompt = ""

    // Run history expansion
    @State private var showAllEvents = false

    // Logs sheet
    @State private var showLogsSheet = false
    @State private var logsLoading = false
    @State private var logsText = ""
    @State private var logsError: String? = nil
    @State private var logsTruncated = false

    // Delete confirm
    @State private var showDeleteConfirm = false

    enum SchedulePreset: String, CaseIterable, Identifiable {
        case hourly = "每小时"
        case daily = "每天"
        case weekly = "每周"
        case custom = "自定义"
        var id: String { rawValue }
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

    private var workspaceDisplayName: String? {
        guard let uri = sidecar.workspaceUri, !uri.isEmpty else { return nil }
        let cleaned = uri.replacingOccurrences(of: "file://", with: "").removingPercentEncoding ?? uri
        return (cleaned as NSString).lastPathComponent.isEmpty ? cleaned : (cleaned as NSString).lastPathComponent
    }

    var body: some View {
        List {
            // MARK: - Top Status & Primary Controls
            Section {
                VStack(alignment: .leading, spacing: 12) {
                    // Title row (inline editable)
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

                    // Status + Toggle + Run Now
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
                        Text(toast)
                            .font(.system(size: 12, weight: .medium))
                            .foregroundColor(.green)
                    }
                }
                .padding(.vertical, 4)
            }

            // MARK: - Schedule & Model Config (Desktop `nrd` equivalent)
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
                        Button("编辑") {
                            beginEditingSchedule()
                        }
                        .font(.system(size: 12.5, weight: .medium))
                        .textCase(nil)
                    }
                }
            }

            // MARK: - Instructions (Desktop `prd` equivalent)
            Section {
                if isEditingPrompt {
                    VStack(alignment: .leading, spacing: 10) {
                        TextEditor(text: $draftPrompt)
                            .font(.system(size: 13.5))
                            .frame(minHeight: 160)
                        HStack(spacing: 12) {
                            Button {
                                savePrompt()
                            } label: {
                                Text(isSavingPrompt ? String(localized: "保存中...") : String(localized: "保存指令"))
                                    .font(.system(size: 13, weight: .semibold))
                                    .padding(.horizontal, 14)
                                    .padding(.vertical, 7)
                                    .background(Color.indigo)
                                    .foregroundColor(.white)
                                    .clipShape(Capsule())
                            }
                            .buttonStyle(.borderless)
                            .disabled(isSavingPrompt || draftPrompt.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)

                            Button("取消") {
                                isEditingPrompt = false
                            }
                            .font(.system(size: 13))
                            .foregroundColor(.secondary)
                            .buttonStyle(.borderless)
                            Spacer()
                        }
                    }
                    .padding(.vertical, 4)
                } else {
                    VStack(alignment: .leading, spacing: 8) {
                        if let desc = sidecar.description, !desc.isEmpty {
                            Text(desc)
                                .font(.system(size: 12.5))
                                .foregroundColor(.secondary)
                            Divider()
                        }
                        let promptText = (sidecar.prompt ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
                        Text(promptText.isEmpty ? String(localized: "暂无执行指令") : promptText)
                            .font(.system(size: 13.5))
                            .foregroundColor(promptText.isEmpty ? .secondary : .primary)
                            .textSelection(.enabled)
                            .fixedSize(horizontal: false, vertical: true)
                    }
                    .padding(.vertical, 4)
                }
            } header: {
                HStack {
                    Text("执行指令")
                    Spacer()
                    if !isEditingPrompt {
                        Button("编辑") {
                            draftPrompt = sidecar.prompt ?? ""
                            isEditingPrompt = true
                        }
                        .font(.system(size: 12.5, weight: .medium))
                        .textCase(nil)
                    }
                }
            }

            // MARK: - Run History (Desktop `qrd` equivalent)
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
                                Image(systemName: ev.status == "error" ? "exclamationmark.circle.fill" : "checkmark.circle.fill")
                                    .font(.system(size: 14))
                                    .foregroundColor(ev.status == "error" ? .red : .green)
                                    .padding(.top, 2)

                                VStack(alignment: .leading, spacing: 3) {
                                    Text(ev.timestamp)
                                        .font(.system(size: 12, weight: .medium, design: .monospaced))
                                        .foregroundColor(.primary)
                                    if let msg = ev.message, !msg.isEmpty {
                                        Text(msg)
                                            .font(.system(size: 12))
                                            .foregroundColor(ev.status == "error" ? .red : .secondary)
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
                        restartTask()
                    } label: {
                        Label("重启任务", systemImage: "arrow.clockwise")
                    }
                    .disabled(!sidecar.isRunning || isAnyBusy)

                    Button {
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
        .refreshable {
            await onRefresh()
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
    private var scheduleSummaryView: some View {
        let rawCron = sidecar.cronExpression ?? ""
        let readable = AutomationCronHelper.humanSummary(rawCron)

        HStack(spacing: 10) {
            Image(systemName: "clock")
                .foregroundColor(.indigo)
                .frame(width: 20)
            VStack(alignment: .leading, spacing: 2) {
                Text(readable)
                    .font(.system(size: 14, weight: .medium))
                if !rawCron.isEmpty {
                    Text(rawCron)
                        .font(.system(size: 11.5, design: .monospaced))
                        .foregroundColor(.secondary)
                }
            }
        }

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
                .lineLimit(1)
        }

        if let wsName = workspaceDisplayName {
            HStack(spacing: 10) {
                Image(systemName: "folder")
                    .foregroundColor(.indigo)
                    .frame(width: 20)
                Text("绑定工作区")
                    .font(.system(size: 13.5))
                    .foregroundColor(.secondary)
                Spacer()
                Text(wsName)
                    .font(.system(size: 13.5, weight: .medium))
                    .lineLimit(1)
            }
        }

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
            Picker("执行频率", selection: $draftPreset) {
                ForEach(SchedulePreset.allCases) { p in
                    Text(LocalizedStringKey(p.rawValue)).tag(p)
                }
            }
            .pickerStyle(.segmented)

            if draftPreset == .daily || draftPreset == .weekly {
                if draftPreset == .weekly {
                    Picker("星期", selection: $draftWeekday) {
                        Text("周一").tag(1)
                        Text("周二").tag(2)
                        Text("周三").tag(3)
                        Text("周四").tag(4)
                        Text("周五").tag(5)
                        Text("周六").tag(6)
                        Text("周日").tag(0)
                    }
                }
                Picker("触发时间", selection: $draftHour) {
                    ForEach(0..<24, id: \.self) { h in
                        Text(String(format: "%02d:00", h)).tag(h)
                    }
                }
            } else if draftPreset == .custom {
                VStack(alignment: .leading, spacing: 4) {
                    Text("Cron 表达式")
                        .font(.system(size: 12))
                        .foregroundColor(.secondary)
                    TextField("例如 0 9 * * *", text: $draftCustomCron)
                        .font(.system(size: 13.5, design: .monospaced))
                        .textFieldStyle(.roundedBorder)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                }
            } else {
                Text("每小时整点自动执行（0 * * * *）")
                    .font(.system(size: 12.5))
                    .foregroundColor(.secondary)
            }

            if !draftTimeZone.isEmpty && draftPreset != .custom {
                Text("保留时区：\(draftTimeZone)")
                    .font(.system(size: 11.5))
                    .foregroundColor(.secondary)
            }

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
                .disabled(isSavingSchedule || resolvedDraftCron().isEmpty)

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
            Group {
                if logsLoading && logsText.isEmpty {
                    VStack(spacing: 10) {
                        ProgressView()
                        Text("正在加载运行日志...")
                            .font(.system(size: 13))
                            .foregroundColor(.secondary)
                    }
                } else if let err = logsError {
                    VStack(spacing: 8) {
                        Image(systemName: "exclamationmark.triangle")
                            .font(.system(size: 28))
                            .foregroundColor(.orange)
                        Text(err)
                            .font(.system(size: 13))
                            .foregroundColor(.secondary)
                            .multilineTextAlignment(.center)
                            .padding(.horizontal, 24)
                    }
                } else if logsText.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                    Text("暂无日志输出")
                        .font(.system(size: 14))
                        .foregroundColor(.secondary)
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

    private func beginEditingSchedule() {
        let parsed = AutomationCronHelper.splitTimeZone(sidecar.cronExpression ?? "")
        draftTimeZone = parsed.timeZone ?? (sidecar.timeZone ?? "")
        draftCustomCron = sidecar.cronExpression ?? "0 9 * * *"
        draftModel = sidecar.model ?? ""

        let parts = parsed.cron.split(whereSeparator: \.isWhitespace).map(String.init)
        if parts.count == 5 && parts[0] == "0" && parts[1] == "*" && parts[2] == "*" && parts[3] == "*" && parts[4] == "*" {
            draftPreset = .hourly
        } else if parts.count == 5 && parts[0] == "0", let h = Int(parts[1]), (0...23).contains(h), parts[2] == "*" && parts[3] == "*" && parts[4] == "*" {
            draftPreset = .daily
            draftHour = h
        } else if parts.count == 5 && parts[0] == "0", let h = Int(parts[1]), (0...23).contains(h), parts[2] == "*" && parts[3] == "*", let d = Int(parts[4]), (0...7).contains(d) {
            draftPreset = .weekly
            draftHour = h
            draftWeekday = (d == 7) ? 0 : d
        } else {
            draftPreset = .custom
        }
        isEditingSchedule = true
    }

    private func resolvedDraftCron() -> String {
        switch draftPreset {
        case .custom:
            return draftCustomCron.trimmingCharacters(in: .whitespacesAndNewlines)
        case .hourly:
            return AutomationCronHelper.combine(timeZone: draftTimeZone, cron: "0 * * * *")
        case .daily:
            return AutomationCronHelper.combine(timeZone: draftTimeZone, cron: "0 \(draftHour) * * *")
        case .weekly:
            return AutomationCronHelper.combine(timeZone: draftTimeZone, cron: "0 \(draftHour) * * \(draftWeekday)")
        }
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
        let cronStr = resolvedDraftCron()
        guard !cronStr.isEmpty else { return }
        isSavingSchedule = true
        Task {
            defer { isSavingSchedule = false }
            do {
                try await APIClient.shared.updateAutomation(
                    sidecarId: sidecar.id,
                    cron: cronStr,
                    model: draftModel,
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

    private func savePrompt() {
        guard let url = settings.serverURL else { return }
        let trimmed = draftPrompt.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return }
        isSavingPrompt = true
        Task {
            defer { isSavingPrompt = false }
            do {
                try await APIClient.shared.updateAutomation(sidecarId: sidecar.id, prompt: trimmed, baseURL: url)
                await onRefresh()
                isEditingPrompt = false
                showToast(String(localized: "执行指令已保存"))
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
        Task {
            defer { logsLoading = false }
            do {
                let res = try await APIClient.shared.fetchAutomationLogs(sidecarId: sidecar.id, tail: 200, baseURL: url)
                logsText = res.logs
                logsTruncated = res.truncated
            } catch {
                logsError = error.localizedDescription
            }
        }
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

    /// Formats ISO-8601 UTC or SQL timestamps into compact local `MM/dd HH:mm` for cron cards & details.
    static func formatTimestamp(_ raw: String) -> String {
        let trimmed = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return raw }
        if let date = isoFractional.date(from: trimmed)
            ?? isoPlain.date(from: trimmed)
            ?? sqlParser.date(from: trimmed) {
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
            } else if let m = Int(min), let h = Int(hour), (0...59).contains(m), (0...23).contains(h), dom == "*" && mon == "*" {
                let hm = String(format: "%02d:%02d", h, m)
                if dow == "*" {
                    base = String(localized: "每天 \(hm)")
                } else if let d = Int(dow), (0...7).contains(d) {
                    let names = [
                        String(localized: "周日"),
                        String(localized: "周一"),
                        String(localized: "周二"),
                        String(localized: "周三"),
                        String(localized: "周四"),
                        String(localized: "周五"),
                        String(localized: "周六"),
                        String(localized: "周日")
                    ]
                    base = String(localized: "每\(names[d]) \(hm)")
                }
            }
        }
        if let tz, !tz.isEmpty {
            return "\(base) (\(tz))"
        }
        return base
    }
}
