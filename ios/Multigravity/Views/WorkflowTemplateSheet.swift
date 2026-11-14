import SwiftUI
import Observation

/// 语义化图标与颜色映射工具（与 Web / Android / Go 网关保持一致）。
private enum WorkflowTemplateStyle {
    struct IconOption: Identifiable {
        let key: String
        let systemImage: String
        let label: String
        var id: String { key }
    }

    struct ColorOption: Identifiable {
        let key: String
        let color: Color
        let label: String
        var id: String { key }
    }

    static let icons: [IconOption] = [
        IconOption(key: "workflow", systemImage: "point.3.connected.trianglepath.dotted", label: "工作流"),
        IconOption(key: "security", systemImage: "shield.lefthalf.filled", label: "安全审查"),
        IconOption(key: "bug", systemImage: "testtube.2", label: "测试修复"),
        IconOption(key: "search", systemImage: "magnifyingglass.circle.fill", label: "调研分析"),
        IconOption(key: "bolt", systemImage: "bolt.fill", label: "快捷执行"),
        IconOption(key: "code", systemImage: "chevron.left.forwardslash.chevron.right", label: "代码开发"),
        IconOption(key: "terminal", systemImage: "terminal.fill", label: "终端脚本"),
        IconOption(key: "doc", systemImage: "doc.text.fill", label: "文档总结"),
        IconOption(key: "star", systemImage: "star.fill", label: "常用收藏"),
        IconOption(key: "sparkles", systemImage: "sparkles", label: "AI 增强")
    ]

    static let colors: [ColorOption] = [
        ColorOption(key: "indigo", color: .indigo, label: "靛蓝"),
        ColorOption(key: "green", color: .green, label: "翠绿"),
        ColorOption(key: "blue", color: .blue, label: "湛蓝"),
        ColorOption(key: "purple", color: .purple, label: "亮紫"),
        ColorOption(key: "orange", color: .orange, label: "活力橙"),
        ColorOption(key: "red", color: .red, label: "珊瑚红"),
        ColorOption(key: "teal", color: .teal, label: "青碧")
    ]

    static func systemImage(for key: String) -> String {
        icons.first(where: { $0.key == key })?.systemImage ?? "point.3.connected.trianglepath.dotted"
    }

    static func color(for key: String) -> Color {
        colors.first(where: { $0.key == key })?.color ?? .indigo
    }
}

/// 混合模式模板仓库：本地 UserDefaults 毫秒级兜底 + Go 网关跨端同步。
@MainActor
@Observable
public final class WorkflowTemplateStore {
    public static let shared = WorkflowTemplateStore()
    private static let cacheKey = "agy_workflow_templates_v1"

    public private(set) var templates: [WorkflowTemplateRecord]
    public private(set) var isSyncing = false
    public var syncError: String?

    private init() {
        self.templates = Self.loadCache() ?? WorkflowTemplateDefaults.all
    }

    private static func loadCache() -> [WorkflowTemplateRecord]? {
        guard let data = UserDefaults.standard.data(forKey: cacheKey) else { return nil }
        return try? JSONDecoder().decode([WorkflowTemplateRecord].self, from: data)
    }

    private static func saveCache(_ list: [WorkflowTemplateRecord]) {
        if let data = try? JSONEncoder().encode(list) {
            UserDefaults.standard.set(data, forKey: cacheKey)
        }
    }

    public func refresh() async {
        guard let baseURL = AppSettings.shared.serverURL else { return }
        do {
            let resp = try await APIClient.shared.fetchWorkflowTemplates(baseURL: baseURL)
            templates = resp.templates
            Self.saveCache(resp.templates)
            syncError = nil
        } catch {
            // 离线兜底：保留本地缓存，不打断使用
        }
    }

    public func setLocalOrder(_ next: [WorkflowTemplateRecord]) {
        templates = next
    }

    public func applyAndSync(_ next: [WorkflowTemplateRecord]) {
        templates = next
        Self.saveCache(next)
        syncError = nil

        guard let baseURL = AppSettings.shared.serverURL else { return }
        isSyncing = true
        Task {
            defer { isSyncing = false }
            do {
                let resp = try await APIClient.shared.saveWorkflowTemplates(next, baseURL: baseURL)
                templates = resp.templates
                Self.saveCache(resp.templates)
            } catch {
                syncError = "已保存本机缓存，网关同步失败：\(error.localizedDescription)"
            }
        }
    }

    public func upsert(_ item: WorkflowTemplateRecord) {
        var next = templates
        if let idx = next.firstIndex(where: { $0.id == item.id }) {
            next[idx] = item
        } else {
            next.append(item)
        }
        applyAndSync(next)
    }

    public func delete(id: String) {
        let next = templates.filter { $0.id != id }
        applyAndSync(next)
    }

    public func resetToDefaults() {
        let defaults = WorkflowTemplateDefaults.all
        templates = defaults
        Self.saveCache(defaults)
        syncError = nil

        guard let baseURL = AppSettings.shared.serverURL else { return }
        isSyncing = true
        Task {
            defer { isSyncing = false }
            do {
                let resp = try await APIClient.shared.resetWorkflowTemplates(baseURL: baseURL)
                templates = resp.templates
                Self.saveCache(resp.templates)
            } catch {
                syncError = "已重置本机列表，网关同步失败：\(error.localizedDescription)"
            }
        }
    }
}

private struct EditorDraft: Identifiable {
    let id: String
    let isNew: Bool
    var record: WorkflowTemplateRecord
}

/// 动态多智能体工作流模板选择与管理抽屉 (WorkflowTemplateSheet)。
public struct WorkflowTemplateSheet: View {
    @Environment(\.dismiss) private var dismiss
    @State private var store = WorkflowTemplateStore.shared
    @State private var isEditing = false
    @State private var editorDraft: EditorDraft?
    @State private var showResetConfirm = false

    // Drag-to-reorder state (leftmost handle)
    @State private var draggingId: String?
    @State private var dragOffsetY: CGFloat = 0
    @State private var lastDragTranslationY: CGFloat = 0

    public let onSelectTemplate: (String) -> Void

    public init(onSelectTemplate: @escaping (String) -> Void) {
        self.onSelectTemplate = onSelectTemplate
    }

    /// 仿照 ConversationListView 的 38pt 红色圆形垃圾桶左滑删除按钮
    private static let trashActionImage: UIImage = {
        let size: CGFloat = 40
        let circleDiameter: CGFloat = 38
        let format = UIGraphicsImageRendererFormat()
        format.opaque = false
        let renderer = UIGraphicsImageRenderer(size: CGSize(width: size, height: size), format: format)
        return renderer.image { ctx in
            let offset = (size - circleDiameter) / 2
            let rect = CGRect(x: offset, y: offset, width: circleDiameter, height: circleDiameter)
            UIColor.systemRed.setFill()
            ctx.cgContext.fillEllipse(in: rect)

            let config = UIImage.SymbolConfiguration(pointSize: 16, weight: .semibold)
            if let baseSymbol = UIImage(systemName: "trash", withConfiguration: config) {
                let symbol = baseSymbol.withTintColor(.white, renderingMode: .alwaysOriginal)
                let symSize = symbol.size
                let symRect = CGRect(
                    x: (size - symSize.width) / 2,
                    y: (size - symSize.height) / 2,
                    width: symSize.width,
                    height: symSize.height
                )
                symbol.draw(in: symRect)
            }
        }
    }()

    public var body: some View {
        NavigationStack {
            List {
                // Header Banner
                Section {
                    HStack(spacing: 10) {
                        ZStack {
                            Circle()
                                .fill(Color.indigo.opacity(0.18))
                                .frame(width: 36, height: 36)
                            Image(systemName: "point.3.connected.trianglepath.dotted")
                                .font(.system(size: 17, weight: .bold))
                                .foregroundColor(.indigo)
                        }

                        VStack(alignment: .leading, spacing: 2) {
                            Text("Antigravity 动态工作流")
                                .font(.system(size: 15, weight: .bold))
                                .foregroundColor(.primary)
                            Text(isEditing ? "按住最左侧手柄拖拽排序，左滑可删除模板" : "选择常用模板快速填充，左滑可删除")
                                .font(.system(size: 12))
                                .foregroundColor(.secondary)
                        }
                    }
                    .padding(.vertical, 4)
                    .listRowBackground(Color.clear)
                    .listRowSeparator(.hidden)
                    .listRowInsets(EdgeInsets(top: 6, leading: 16, bottom: 6, trailing: 16))
                }

                if let errorMsg = store.syncError {
                    Section {
                        Text(errorMsg)
                            .font(.system(size: 12.5))
                            .foregroundColor(.red)
                            .padding(.vertical, 4)
                            .listRowBackground(Color.red.opacity(0.1))
                    }
                }

                // Templates List
                if store.templates.isEmpty {
                    Section {
                        VStack(spacing: 12) {
                            Text("暂无 Workflow 模板")
                                .font(.system(size: 14))
                                .foregroundColor(.secondary)
                            HStack(spacing: 12) {
                                Button("+ 新增模板") {
                                    openNewEditor()
                                }
                                .buttonStyle(.borderedProminent)
                                .tint(.indigo)

                                Button("恢复默认") {
                                    store.resetToDefaults()
                                }
                                .buttonStyle(.bordered)
                            }
                        }
                        .frame(maxWidth: .infinity)
                        .padding(.vertical, 28)
                        .listRowBackground(Color.clear)
                        .listRowSeparator(.hidden)
                    }
                } else {
                    Section {
                        ForEach(store.templates) { item in
                            rowView(for: item)
                                .listRowBackground(Color.clear)
                                .listRowSeparator(.hidden)
                                .listRowInsets(EdgeInsets(top: 5, leading: 16, bottom: 5, trailing: 16))
                                .swipeActions(edge: .trailing, allowsFullSwipe: false) {
                                    Button {
                                        UIImpactFeedbackGenerator(style: .medium).impactOccurred()
                                        withAnimation(.easeInOut(duration: 0.2)) {
                                            store.delete(id: item.id)
                                        }
                                    } label: {
                                        Image(uiImage: Self.trashActionImage)
                                    }
                                    .tint(.clear)
                                }
                        }
                    }
                }

                if isEditing {
                    Section {
                        Button(role: .destructive) {
                            showResetConfirm = true
                        } label: {
                            HStack {
                                Spacer()
                                Text("恢复默认模板")
                                    .font(.system(size: 14, weight: .medium))
                                Spacer()
                            }
                        }
                        .listRowBackground(Color.clear)
                        .listRowSeparator(.hidden)
                    }
                }
            }
            .listStyle(.plain)
            .scrollContentBackground(.hidden)
            .coordinateSpace(name: "wfList")
            .navigationTitle("Workflow 模板")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    Button {
                        UIImpactFeedbackGenerator(style: .light).impactOccurred()
                        withAnimation(.easeInOut(duration: 0.2)) {
                            isEditing.toggle()
                        }
                    } label: {
                        Image(systemName: isEditing ? "checkmark" : "pencil")
                            .font(.system(size: 15, weight: .semibold))
                            .foregroundColor(isEditing ? .indigo : .primary)
                    }
                    .accessibilityLabel(isEditing ? "完成" : "编辑")
                }

                ToolbarItem(placement: .topBarTrailing) {
                    Button {
                        openNewEditor()
                    } label: {
                        Image(systemName: "plus")
                            .font(.system(size: 16, weight: .semibold))
                    }
                    .accessibilityLabel("新增 Workflow 模板")
                }
            }
            .sheet(item: $editorDraft) { draft in
                WorkflowTemplateEditorSheet(
                    isNew: draft.isNew,
                    initial: draft.record
                ) { saved in
                    store.upsert(saved)
                }
            }
            .alert("恢复默认模板", isPresented: $showResetConfirm) {
                Button("取消", role: .cancel) {}
                Button("确认恢复", role: .destructive) {
                    store.resetToDefaults()
                }
            } message: {
                Text("将恢复为系统初始的 4 个模板，当前自定义模板和排序会被替换。")
            }
            .task {
                await store.refresh()
            }
        }
        .presentationDetents([.medium, .large])
    }

    @ViewBuilder
    private func rowView(for item: WorkflowTemplateRecord) -> some View {
        let isDragging = draggingId == item.id
        let iconColor = WorkflowTemplateStyle.color(for: item.color)
        let systemImage = WorkflowTemplateStyle.systemImage(for: item.icon)
        let displaySubtitle = item.subtitle.isEmpty ? item.templatePrompt : item.subtitle

        let cardContent = HStack(spacing: 12) {
            if isEditing {
                Image(systemName: "line.3.horizontal")
                    .font(.system(size: 16, weight: .semibold))
                    .foregroundColor(isDragging ? .indigo : .secondary)
                    .frame(width: 28, height: 36)
                    .contentShape(Rectangle())
                    .highPriorityGesture(
                        DragGesture(minimumDistance: 2, coordinateSpace: .named("wfList"))
                            .onChanged { value in
                                if draggingId == nil {
                                    draggingId = item.id
                                    lastDragTranslationY = 0
                                    UIImpactFeedbackGenerator(style: .light).impactOccurred()
                                }
                                let deltaY = value.translation.height - lastDragTranslationY
                                lastDragTranslationY = value.translation.height
                                dragOffsetY += deltaY

                                let stepThreshold: CGFloat = 56
                                guard let currentIdx = store.templates.firstIndex(where: { $0.id == item.id }) else { return }

                                if dragOffsetY > stepThreshold && currentIdx < store.templates.count - 1 {
                                    var next = store.templates
                                    let moved = next.remove(at: currentIdx)
                                    next.insert(moved, at: currentIdx + 1)
                                    withAnimation(.easeInOut(duration: 0.15)) {
                                        store.setLocalOrder(next)
                                    }
                                    dragOffsetY -= stepThreshold
                                    UIImpactFeedbackGenerator(style: .light).impactOccurred()
                                } else if dragOffsetY < -stepThreshold && currentIdx > 0 {
                                    var next = store.templates
                                    let moved = next.remove(at: currentIdx)
                                    next.insert(moved, at: currentIdx - 1)
                                    withAnimation(.easeInOut(duration: 0.15)) {
                                        store.setLocalOrder(next)
                                    }
                                    dragOffsetY += stepThreshold
                                    UIImpactFeedbackGenerator(style: .light).impactOccurred()
                                }
                            }
                            .onEnded { _ in
                                draggingId = nil
                                dragOffsetY = 0
                                lastDragTranslationY = 0
                                store.applyAndSync(store.templates)
                            }
                    )
            }

            ZStack {
                RoundedRectangle(cornerRadius: 10)
                    .fill(iconColor.opacity(0.15))
                    .frame(width: 40, height: 40)
                Image(systemName: systemImage)
                    .font(.system(size: 18, weight: .semibold))
                    .foregroundColor(iconColor)
            }

            VStack(alignment: .leading, spacing: 3) {
                Text(item.title)
                    .font(.system(size: 14, weight: .semibold))
                    .foregroundColor(.primary)
                    .lineLimit(1)
                Text(displaySubtitle)
                    .font(.system(size: 11.5))
                    .foregroundColor(.secondary)
                    .lineLimit(2)
                    .fixedSize(horizontal: false, vertical: true)
            }

            Spacer(minLength: 4)

            if isEditing {
                Button {
                    editorDraft = EditorDraft(id: item.id, isNew: false, record: item)
                } label: {
                    Image(systemName: "pencil.circle.fill")
                        .font(.system(size: 20))
                        .foregroundColor(.indigo)
                }
                .buttonStyle(.plain)
                .accessibilityLabel("编辑 \(item.title)")
            } else {
                Image(systemName: "chevron.right")
                    .font(.system(size: 12, weight: .bold))
                    .foregroundColor(Color(uiColor: .tertiaryLabel))
            }
        }
        .padding(12)
        .background(Color(uiColor: .secondarySystemBackground))
        .cornerRadius(12)
        .overlay(
            RoundedRectangle(cornerRadius: 12)
                .stroke(isDragging ? Color.indigo.opacity(0.75) : Color(uiColor: .separator).opacity(0.4), lineWidth: isDragging ? 1.2 : 0.5)
        )
        .offset(y: isDragging ? dragOffsetY : 0)
        .zIndex(isDragging ? 10 : 0)
        .shadow(color: isDragging ? Color.black.opacity(0.16) : .clear, radius: 8, y: 4)

        if isEditing {
            cardContent
        } else {
            Button {
                UIImpactFeedbackGenerator(style: .medium).impactOccurred()
                onSelectTemplate(item.templatePrompt)
                dismiss()
            } label: {
                cardContent
            }
            .buttonStyle(.plain)
        }
    }

    private func openNewEditor() {
        let newRecord = WorkflowTemplateRecord(
            id: "wf_\(UUID().uuidString.lowercased())",
            title: "",
            subtitle: "",
            icon: "workflow",
            color: "indigo",
            templatePrompt: ""
        )
        editorDraft = EditorDraft(id: newRecord.id, isNew: true, record: newRecord)
    }
}

private struct WorkflowTemplateEditorSheet: View {
    @Environment(\.dismiss) private var dismiss

    let isNew: Bool
    let onSave: (WorkflowTemplateRecord) -> Void

    @State private var id: String
    @State private var title: String
    @State private var subtitle: String
    @State private var icon: String
    @State private var color: String
    @State private var templatePrompt: String

    init(isNew: Bool, initial: WorkflowTemplateRecord, onSave: @escaping (WorkflowTemplateRecord) -> Void) {
        self.isNew = isNew
        self.onSave = onSave
        _id = State(initialValue: initial.id)
        _title = State(initialValue: initial.title)
        _subtitle = State(initialValue: initial.subtitle)
        _icon = State(initialValue: initial.icon)
        _color = State(initialValue: initial.color)
        _templatePrompt = State(initialValue: initial.templatePrompt)
    }

    private var canSave: Bool {
        !title.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty &&
        !templatePrompt.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
    }

    var body: some View {
        NavigationStack {
            Form {
                Section("基本信息") {
                    TextField("标题（必填，例如：每日代码巡检）", text: $title)
                    TextField("简介（可选，方便快速辨识）", text: $subtitle)
                }

                Section("图标与主题色") {
                    ScrollView(.horizontal, showsIndicators: false) {
                        HStack(spacing: 14) {
                            ForEach(WorkflowTemplateStyle.colors) { c in
                                Button {
                                    color = c.key
                                } label: {
                                    Circle()
                                        .fill(c.color)
                                        .frame(width: 28, height: 28)
                                        .overlay(
                                            Circle()
                                                .stroke(Color.primary, lineWidth: color == c.key ? 2.5 : 0)
                                                .padding(-3)
                                        )
                                }
                                .buttonStyle(.plain)
                                .accessibilityLabel(c.label)
                            }
                        }
                        .padding(.vertical, 6)
                    }

                    let selectedColor = WorkflowTemplateStyle.color(for: color)
                    LazyVGrid(columns: Array(repeating: GridItem(.flexible(), spacing: 10), count: 5), spacing: 10) {
                        ForEach(WorkflowTemplateStyle.icons) { ic in
                            let active = icon == ic.key
                            Button {
                                icon = ic.key
                            } label: {
                                Image(systemName: ic.systemImage)
                                    .font(.system(size: 17, weight: .semibold))
                                    .foregroundColor(active ? selectedColor : .secondary)
                                    .frame(maxWidth: .infinity)
                                    .frame(height: 40)
                                    .background(active ? selectedColor.opacity(0.16) : Color(uiColor: .tertiarySystemFill))
                                    .cornerRadius(10)
                                    .overlay(
                                        RoundedRectangle(cornerRadius: 10)
                                            .stroke(active ? selectedColor : .clear, lineWidth: 1.5)
                                    )
                            }
                            .buttonStyle(.plain)
                            .accessibilityLabel(ic.label)
                        }
                    }
                    .padding(.vertical, 4)
                }

                Section {
                    ZStack(alignment: .topLeading) {
                        if templatePrompt.isEmpty {
                            Text("输入提示词或 /workflow、/plan 等命令模板...")
                                .font(.system(size: 14))
                                .foregroundColor(Color(uiColor: .placeholderText))
                                .padding(.top, 8)
                                .padding(.leading, 4)
                        }
                        TextEditor(text: $templatePrompt)
                            .font(.system(size: 14))
                            .frame(minHeight: 130)
                    }
                } header: {
                    Text("提示词 / 命令模板（必填）")
                } footer: {
                    Text("支持任意 Prompt 或斜杠命令，可用 [占位符] 提示自己在发送前补充参数。")
                }
            }
            .navigationTitle(isNew ? "新增 Workflow" : "编辑 Workflow")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    Button("取消") {
                        dismiss()
                    }
                }
                ToolbarItem(placement: .topBarTrailing) {
                    Button("保存") {
                        let saved = WorkflowTemplateRecord(
                            id: id,
                            title: title.trimmingCharacters(in: .whitespacesAndNewlines),
                            subtitle: subtitle.trimmingCharacters(in: .whitespacesAndNewlines),
                            icon: icon,
                            color: color,
                            templatePrompt: templatePrompt.trimmingCharacters(in: .whitespacesAndNewlines)
                        )
                        onSave(saved)
                        dismiss()
                    }
                    .fontWeight(.semibold)
                    .disabled(!canSave)
                }
            }
        }
    }
}
