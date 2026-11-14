import SwiftUI
import Observation

/// 统一自绘 24×24 矢量图标路径（与 Android / Web 端像素级对齐，替代星形为 Xcode 编译锤子，Bug 对齐昆虫甲虫）。
private enum WorkflowTemplateStyle {
    struct IconOption: Identifiable {
        let key: String
        let pathData: String
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
        IconOption(
            key: "workflow",
            pathData: "M 6 3.3 C 7.5 3.3 8.7 4.5 8.7 6 C 8.7 7.5 7.5 8.7 6 8.7 C 4.5 8.7 3.3 7.5 3.3 6 C 3.3 4.5 4.5 3.3 6 3.3 Z M 18 3.3 C 19.5 3.3 20.7 4.5 20.7 6 C 20.7 7.5 19.5 8.7 18 8.7 C 16.5 8.7 15.3 7.5 15.3 6 C 15.3 4.5 16.5 3.3 18 3.3 Z M 12 15.3 C 13.5 15.3 14.7 16.5 14.7 18 C 14.7 19.5 13.5 20.7 12 20.7 C 10.5 20.7 9.3 19.5 9.3 18 C 9.3 16.5 10.5 15.3 12 15.3 Z M 8.7 6 L 15.3 6 M 7.3 8.4 L 10.7 15.6 M 16.7 8.4 L 13.3 15.6",
            label: "工作流"
        ),
        IconOption(
            key: "security",
            pathData: "M 12 2.5 L 19.5 5.8 L 19.5 11.5 C 19.5 16.5 16.2 20.0 12 21.5 C 7.8 20.0 4.5 16.5 4.5 11.5 L 4.5 5.8 L 12 2.5 Z M 9 12 L 11.2 14.2 L 15.4 9.8",
            label: "安全审查"
        ),
        IconOption(
            key: "bug",
            pathData: "M 9 7.5 L 9 6.5 C 9 4.84 10.34 3.5 12 3.5 C 13.66 3.5 15 4.84 15 6.5 L 15 7.5 M 8.2 2.5 L 10 4.2 M 15.8 2.5 L 14 4.2 M 8.5 7.5 L 15.5 7.5 C 16.88 7.5 18 8.62 18 10 L 18 14 C 18 17.31 15.31 20 12 20 C 8.69 20 6 17.31 6 14 L 6 10 C 6 8.62 7.12 7.5 8.5 7.5 Z M 12 11 L 12 20 M 6 9.5 L 3.2 7.5 M 6 13.2 L 2.5 13.2 M 6.4 16.8 L 3.2 19.2 M 18 9.5 L 20.8 7.5 M 18 13.2 L 21.5 13.2 M 17.6 16.8 L 20.8 19.2",
            label: "测试修复"
        ),
        IconOption(
            key: "search",
            pathData: "M 10.8 4 C 14.56 4 17.6 7.04 17.6 10.8 C 17.6 14.56 14.56 17.6 10.8 17.6 C 7.04 17.6 4 14.56 4 10.8 C 4 7.04 7.04 4 10.8 4 Z M 15.8 15.8 L 20.5 20.5",
            label: "调研分析"
        ),
        IconOption(
            key: "bolt",
            pathData: "M 13.2 2.5 L 4.5 13.5 L 11.5 13.5 L 10.2 21.5 L 19.5 10.5 L 12.5 10.5 L 13.2 2.5 Z",
            label: "快捷执行"
        ),
        IconOption(
            key: "code",
            pathData: "M 7.5 7.5 L 3 12 L 7.5 16.5 M 16.5 7.5 L 21 12 L 16.5 16.5 M 13.8 5 L 10.2 19",
            label: "代码开发"
        ),
        IconOption(
            key: "terminal",
            pathData: "M 5.5 4 L 18.5 4 C 19.88 4 21 5.12 21 6.5 L 21 17.5 C 21 18.88 19.88 20 18.5 20 L 5.5 20 C 4.12 20 3 18.88 3 17.5 L 3 6.5 C 3 5.12 4.12 4 5.5 4 Z M 7 9.5 L 10.5 12.5 L 7 15.5 M 12.5 15.5 L 17 15.5",
            label: "终端脚本"
        ),
        IconOption(
            key: "doc",
            pathData: "M 14 2.5 L 6.5 2.5 C 5.4 2.5 4.5 3.4 4.5 4.5 L 4.5 19.5 C 4.5 20.6 5.4 21.5 6.5 21.5 L 17.5 21.5 C 18.6 21.5 19.5 20.6 19.5 19.5 L 19.5 8 L 14 2.5 Z M 14 2.5 L 14 8 L 19.5 8 M 8.5 13 L 15.5 13 M 8.5 17 L 13.5 17",
            label: "文档总结"
        ),
        IconOption(
            key: "build",
            pathData: "M 9 3.2 C 12.0 2.7 14.8 3.7 16.8 5.7 L 21 9.9 L 18.2 12.7 L 14 8.5 C 12.5 7.0 10.8 6.4 9 6.7 L 9 3.2 Z M 13.5 8.1 L 3.4 18.2 C 2.6 19.0 2.6 20.3 3.4 21.1 C 4.2 21.9 5.5 21.9 6.3 21.1 L 16.4 11.0",
            label: "编译构建"
        ),
        IconOption(
            key: "sparkles",
            pathData: "M 11 4 L 12.8 9.7 L 18.5 11.5 L 12.8 13.3 L 11 19 L 9.2 13.3 L 3.5 11.5 L 9.2 9.7 L 11 4 Z M 18.5 2.8 L 18.5 7.2 M 16.3 5 L 20.7 5",
            label: "AI 增强"
        ),
        IconOption(
            key: "git",
            pathData: "M 6 3.5 C 7.38 3.5 8.5 4.62 8.5 6 C 8.5 7.38 7.38 8.5 6 8.5 C 4.62 8.5 3.5 7.38 3.5 6 C 3.5 4.62 4.62 3.5 6 3.5 Z M 6 15.5 C 7.38 15.5 8.5 16.62 8.5 18 C 8.5 19.38 7.38 20.5 6 20.5 C 4.62 20.5 3.5 19.38 3.5 18 C 3.5 16.62 4.62 15.5 6 15.5 Z M 18 5.5 C 19.38 5.5 20.5 6.62 20.5 8 C 20.5 9.38 19.38 10.5 18 10.5 C 16.62 10.5 15.5 9.38 15.5 8 C 15.5 6.62 16.62 5.5 18 5.5 Z M 6 8.5 L 6 15.5 M 18 10.5 C 18 14.5 14.5 15.5 8.5 15.5",
            label: "Git 分支"
        ),
        IconOption(
            key: "merge",
            pathData: "M 6 3.5 C 7.38 3.5 8.5 4.62 8.5 6 C 8.5 7.38 7.38 8.5 6 8.5 C 4.62 8.5 3.5 7.38 3.5 6 C 3.5 4.62 4.62 3.5 6 3.5 Z M 6 15.5 C 7.38 15.5 8.5 16.62 8.5 18 C 8.5 19.38 7.38 20.5 6 20.5 C 4.62 20.5 3.5 19.38 3.5 18 C 3.5 16.62 4.62 15.5 6 15.5 Z M 18 15.5 C 19.38 15.5 20.5 16.62 20.5 18 C 20.5 19.38 19.38 20.5 18 20.5 C 16.62 20.5 15.5 19.38 15.5 18 C 15.5 16.62 16.62 15.5 18 15.5 Z M 6 8.5 L 6 15.5 M 6 8.5 C 6 13.5 11 18 15.5 18",
            label: "分支合并"
        ),
        IconOption(
            key: "pipeline",
            pathData: "M 4.5 11 C 5.0 6.8 8.6 3.8 12.8 4.0 C 15.8 4.1 18.4 5.8 19.6 8.5 M 20 4.5 L 20 8.8 L 15.7 8.8 M 19.5 13 C 19.0 17.2 15.4 20.2 11.2 20.0 C 8.2 19.9 5.6 18.2 4.4 15.5 M 4 19.5 L 4 15.2 L 8.3 15.2",
            label: "流水线"
        ),
        IconOption(
            key: "checklist",
            pathData: "M 3.5 7.5 L 5.8 9.8 L 9.8 5.5 M 13 7.8 L 20.5 7.8 M 3.5 16.5 L 5.8 18.8 L 9.8 14.5 M 13 16.8 L 20.5 16.8",
            label: "流程清单"
        ),
        IconOption(
            key: "report",
            pathData: "M 3.5 4 L 3.5 20 L 20.5 20 M 8 16.5 L 8 12 M 12.8 16.5 L 12.8 8.5 M 17.6 16.5 L 17.6 5.5",
            label: "分析报告"
        )
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

    static func normalizedIconKey(_ key: String) -> String {
        if key == "star" { return "build" }
        return icons.contains(where: { $0.key == key }) ? key : "workflow"
    }

    static func pathData(for key: String) -> String {
        let normalized = normalizedIconKey(key)
        return icons.first(where: { $0.key == normalized })?.pathData ?? icons[0].pathData
    }

    static func color(for key: String) -> Color {
        colors.first(where: { $0.key == key })?.color ?? .indigo
    }

    private static let cgPathCache = NSCache<NSString, CGPathBox>()

    private final class CGPathBox: NSObject {
        let path: CGPath
        init(path: CGPath) { self.path = path }
    }

    static func cachedCGPath(for key: String) -> CGPath {
        let resolved = normalizedIconKey(key)
        if let hit = cgPathCache.object(forKey: resolved as NSString) {
            return hit.path
        }
        let parsed = parseSVGPath(pathData(for: resolved))
        cgPathCache.setObject(CGPathBox(path: parsed), forKey: resolved as NSString)
        return parsed
    }

    private static func parseSVGPath(_ d: String) -> CGPath {
        let path = CGMutablePath()
        let scanner = Scanner(string: d)
        scanner.charactersToBeSkipped = CharacterSet.whitespacesAndNewlines.union(CharacterSet(charactersIn: ","))
        let cmdSet = CharacterSet(charactersIn: "MLCQZmlcqz")
        var cmd = ""

        while !scanner.isAtEnd {
            if let scannedCmd = scanner.scanCharacters(from: cmdSet) {
                for ch in scannedCmd {
                    cmd = String(ch)
                    if cmd.uppercased() == "Z" {
                        path.closeSubpath()
                    }
                }
                continue
            }
            switch cmd.uppercased() {
            case "M":
                if let x = scanner.scanDouble(), let y = scanner.scanDouble() {
                    path.move(to: CGPoint(x: x, y: y))
                    cmd = "L"
                } else {
                    return path
                }
            case "L":
                if let x = scanner.scanDouble(), let y = scanner.scanDouble() {
                    path.addLine(to: CGPoint(x: x, y: y))
                } else {
                    return path
                }
            case "C":
                if let x1 = scanner.scanDouble(), let y1 = scanner.scanDouble(),
                   let x2 = scanner.scanDouble(), let y2 = scanner.scanDouble(),
                   let x = scanner.scanDouble(), let y = scanner.scanDouble() {
                    path.addCurve(
                        to: CGPoint(x: x, y: y),
                        control1: CGPoint(x: x1, y: y1),
                        control2: CGPoint(x: x2, y: y2)
                    )
                } else {
                    return path
                }
            case "Q":
                if let x1 = scanner.scanDouble(), let y1 = scanner.scanDouble(),
                   let x = scanner.scanDouble(), let y = scanner.scanDouble() {
                    path.addQuadCurve(to: CGPoint(x: x, y: y), control: CGPoint(x: x1, y: y1))
                } else {
                    return path
                }
            default:
                return path
            }
        }
        return path
    }
}

private struct WorkflowVectorIconShape: Shape {
    let iconKey: String

    func path(in rect: CGRect) -> Path {
        let base = WorkflowTemplateStyle.cachedCGPath(for: iconKey)
        let scale = min(rect.width, rect.height) / 24.0
        var transform = CGAffineTransform(scaleX: scale, y: scale)
        let offsetX = (rect.width - 24.0 * scale) / 2.0
        let offsetY = (rect.height - 24.0 * scale) / 2.0
        transform = transform.concatenating(CGAffineTransform(translationX: offsetX, y: offsetY))
        if let transformed = base.copy(using: &transform) {
            return Path(transformed)
        }
        return Path()
    }
}

struct WorkflowVectorIconView: View {
    let iconKey: String
    var size: CGFloat = 18
    var color: Color = .primary

    var body: some View {
        WorkflowVectorIconShape(iconKey: iconKey)
            .stroke(
                color,
                style: StrokeStyle(
                    lineWidth: max(1.4, size * (2.1 / 24.0)),
                    lineCap: .round,
                    lineJoin: .round
                )
            )
            .frame(width: size, height: size)
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
        let cached = Self.loadCache() ?? WorkflowTemplateDefaults.all
        self.templates = WorkflowTemplateRecord.partitioned(cached)
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
            let partitioned = WorkflowTemplateRecord.partitioned(resp.templates)
            if partitioned != templates {
                templates = partitioned
                Self.saveCache(partitioned)
            }
            if syncError != nil { syncError = nil }
        } catch {
            // 离线兜底：保留本地缓存，不打断使用
        }
    }

    public func applyAndSync(_ next: [WorkflowTemplateRecord]) {
        let partitioned = WorkflowTemplateRecord.partitioned(next)
        templates = partitioned
        Self.saveCache(partitioned)
        syncError = nil

        guard let baseURL = AppSettings.shared.serverURL else { return }
        isSyncing = true
        Task {
            defer { isSyncing = false }
            do {
                let resp = try await APIClient.shared.saveWorkflowTemplates(partitioned, baseURL: baseURL)
                let serverPartitioned = WorkflowTemplateRecord.partitioned(resp.templates)
                if serverPartitioned != templates {
                    templates = serverPartitioned
                    Self.saveCache(serverPartitioned)
                }
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
        let defaults = WorkflowTemplateRecord.partitioned(WorkflowTemplateDefaults.all)
        templates = defaults
        Self.saveCache(defaults)
        syncError = nil

        guard let baseURL = AppSettings.shared.serverURL else { return }
        isSyncing = true
        Task {
            defer { isSyncing = false }
            do {
                let resp = try await APIClient.shared.resetWorkflowTemplates(baseURL: baseURL)
                let serverPartitioned = WorkflowTemplateRecord.partitioned(resp.templates)
                if serverPartitioned != templates {
                    templates = serverPartitioned
                    Self.saveCache(serverPartitioned)
                }
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

private struct RowFramePreferenceKey: PreferenceKey {
    static var defaultValue: [String: CGRect] { [:] }
    static func reduce(value: inout [String: CGRect], nextValue: () -> [String: CGRect]) {
        value.merge(nextValue(), uniquingKeysWith: { _, new in new })
    }
}

private struct MoreBoundaryMinYKey: PreferenceKey {
    static var defaultValue: CGFloat? { nil }
    static func reduce(value: inout CGFloat?, nextValue: () -> CGFloat?) {
        if let v = nextValue() { value = v }
    }
}

/// 动态多智能体工作流模板选择与管理抽屉（直接满屏、双分区配置、编辑模式点击整卡编辑）。
public struct WorkflowTemplateSheet: View {
    private static let guideDismissedKey = "agy_wf_quick_guide_dismissed_v1"

    @Environment(\.dismiss) private var dismiss
    @State private var store = WorkflowTemplateStore.shared
    @State private var isEditing: Bool
    @State private var editorDraft: EditorDraft?
    @State private var showResetConfirm = false
    @State private var swipedId: String?
    @State private var horizontalSwipingId: String?
    @State private var guideDismissed: Bool = UserDefaults.standard.bool(forKey: guideDismissedKey)

    // Cross-section drag-and-drop state
    @State private var draggingId: String?
    @State private var dragStartIndex: Int?
    @State private var dragTargetInQuickMenu: Bool?
    @State private var dragInsertInTargetSection: Int = 0
    @State private var dragOffsetY: CGFloat = 0
    @State private var rowFrames: [String: CGRect] = [:]
    @State private var moreBoundaryMinY: CGFloat = 0

    private let cardSpacing: CGFloat = 10

    public let onSelectTemplate: (String) -> Void

    public init(initialEditing: Bool = false, onSelectTemplate: @escaping (String) -> Void) {
        self._isEditing = State(initialValue: initialEditing)
        self.onSelectTemplate = onSelectTemplate
    }

    private var quickItems: [(index: Int, item: WorkflowTemplateRecord)] {
        store.templates.enumerated().compactMap { $0.element.inQuickMenu ? ($0.offset, $0.element) : nil }
    }

    private var moreItems: [(index: Int, item: WorkflowTemplateRecord)] {
        store.templates.enumerated().compactMap { !$0.element.inQuickMenu ? ($0.offset, $0.element) : nil }
    }

    private var draggingInQuickMenu: Bool? {
        guard let start = dragStartIndex, store.templates.indices.contains(start) else { return nil }
        return store.templates[start].inQuickMenu
    }

    public var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 14) {
                    if !guideDismissed {
                        WorkflowQuickGuideBanner(
                            isEditing: isEditing,
                            onEnterEdit: {
                                UIImpactFeedbackGenerator(style: .light).impactOccurred()
                                withAnimation(.easeInOut(duration: 0.2)) {
                                    isEditing = true
                                }
                            },
                            onDismiss: {
                                UIImpactFeedbackGenerator(style: .light).impactOccurred()
                                UserDefaults.standard.set(true, forKey: Self.guideDismissedKey)
                                withAnimation(.easeOut(duration: 0.18)) {
                                    guideDismissed = true
                                }
                            }
                        )
                    }

                    if let errorMsg = store.syncError {
                        Text(errorMsg)
                            .font(.system(size: 12.5))
                            .foregroundColor(.red)
                            .padding(10)
                            .frame(maxWidth: .infinity, alignment: .leading)
                            .background(Color.red.opacity(0.1))
                            .cornerRadius(10)
                    }

                    if store.templates.isEmpty {
                        VStack(spacing: 12) {
                            Text("暂无 Workflow 模板")
                                .font(.system(size: 14))
                                .foregroundColor(.secondary)
                            HStack(spacing: 12) {
                                Button("+ 新增模板") {
                                    openNewEditor(inQuickMenu: false)
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
                    } else {
                        // Section 1: 长按快捷菜单
                        sectionHeader(
                            title: "长按快捷菜单",
                            subtitle: isEditing ? "拖拽至此置入长按上滑菜单" : "长按聊天栏 Workflow 上滑直达",
                            count: quickItems.count
                        )

                        if quickItems.isEmpty {
                            let highlightDrop = draggingId != nil && dragTargetInQuickMenu == true
                            emptyQuickMenuDropZone(isHighlighted: highlightDrop)
                        } else {
                            VStack(spacing: cardSpacing) {
                                ForEach(quickItems, id: \.item.id) { entry in
                                    templateRowView(index: entry.index, item: entry.item)
                                }
                            }
                            .zIndex(draggingInQuickMenu == true ? 10 : 0)
                        }

                        // Section 2: 更多模板
                        sectionHeader(
                            title: "更多模板",
                            subtitle: isEditing ? "点击卡片编辑，按住 ≡ 可拖入上方快捷菜单" : "点击模板直接填入输入框",
                            count: moreItems.count
                        )
                        .padding(.top, 4)
                        .background(
                            GeometryReader { geo in
                                Color.clear.preference(
                                    key: MoreBoundaryMinYKey.self,
                                    value: geo.frame(in: .named("wfScrollContent")).minY
                                )
                            }
                        )

                        if moreItems.isEmpty {
                            if isEditing {
                                let highlightDrop = draggingId != nil && dragTargetInQuickMenu == false
                                emptyMoreTemplatesDropZone(isHighlighted: highlightDrop)
                            } else {
                                Text("全部模板已加入长按快捷菜单")
                                    .font(.system(size: 12.5))
                                    .foregroundColor(.secondary)
                                    .frame(maxWidth: .infinity, alignment: .center)
                                    .padding(.vertical, 16)
                            }
                        } else {
                            VStack(spacing: cardSpacing) {
                                ForEach(moreItems, id: \.item.id) { entry in
                                    templateRowView(index: entry.index, item: entry.item)
                                }
                            }
                            .zIndex(draggingInQuickMenu == false ? 10 : 0)
                        }
                    }

                    if isEditing {
                        Button(role: .destructive) {
                            showResetConfirm = true
                        } label: {
                            Text("恢复默认模板")
                                .font(.system(size: 14, weight: .medium))
                                .frame(maxWidth: .infinity)
                                .padding(.vertical, 8)
                        }
                        .padding(.top, 4)
                    }
                }
                .padding(16)
                .coordinateSpace(name: "wfScrollContent")
                .onPreferenceChange(RowFramePreferenceKey.self) { frames in
                    rowFrames.merge(frames, uniquingKeysWith: { _, new in new })
                }
                .onPreferenceChange(MoreBoundaryMinYKey.self) { minY in
                    if let minY {
                        moreBoundaryMinY = minY
                    }
                }
            }
            .scrollDisabled(draggingId != nil || horizontalSwipingId != nil)
            .navigationTitle("Workflow 模板")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    Button {
                        UIImpactFeedbackGenerator(style: .light).impactOccurred()
                        swipedId = nil
                        withAnimation(.easeInOut(duration: 0.2)) {
                            isEditing.toggle()
                        }
                    } label: {
                        Image(systemName: isEditing ? "checkmark" : "pencil")
                            .font(.system(size: 15, weight: .semibold))
                    }
                    .accessibilityLabel(isEditing ? "完成" : "编辑")
                }

                ToolbarItem(placement: .topBarTrailing) {
                    Button {
                        swipedId = nil
                        openNewEditor(inQuickMenu: false)
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
                Text("将恢复为系统初始的 4 个模板，当前自定义模板、快捷菜单配置和排序会被替换。")
            }
            .task {
                await store.refresh()
            }
        }
        .presentationDetents([.large])
        .presentationDragIndicator(.visible)
    }

    @ViewBuilder
    private func sectionHeader(title: String, subtitle: String, count: Int) -> some View {
        HStack(alignment: .firstTextBaseline, spacing: 6) {
            Text(title)
                .font(.system(size: 13.5, weight: .bold))
                .foregroundColor(.primary)
            Text("\(count)")
                .font(.system(size: 11.5, weight: .semibold))
                .foregroundColor(.secondary)
                .padding(.horizontal, 6)
                .padding(.vertical, 1)
                .background(Color(uiColor: .tertiarySystemFill))
                .clipShape(Capsule())
            Spacer(minLength: 8)
            Text(subtitle)
                .font(.system(size: 11))
                .foregroundColor(.secondary)
                .lineLimit(1)
        }
        .padding(.horizontal, 2)
    }

    @ViewBuilder
    private func emptyQuickMenuDropZone(isHighlighted: Bool) -> some View {
        let content = VStack(spacing: 5) {
            Text(isEditing ? "按住下方模板左侧 ≡ 向上拖入此处" : "暂未添加快捷模板")
                .font(.system(size: 13, weight: .semibold))
                .foregroundColor(isHighlighted ? .indigo : .primary)
            Text(isEditing ? "拖入后即可在聊天栏长按 Workflow 上滑直达" : "点击进入编辑，或编辑单个模板开启「加入长按快捷菜单」")
                .font(.system(size: 11.5))
                .foregroundColor(.secondary)
                .multilineTextAlignment(.center)
        }
        .frame(maxWidth: .infinity)
        .padding(.horizontal, 14)
        .padding(.vertical, 18)
        .background(isHighlighted ? Color.indigo.opacity(0.10) : Color(uiColor: .secondarySystemBackground).opacity(0.55))
        .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
        .overlay(
            RoundedRectangle(cornerRadius: 12, style: .continuous)
                .strokeBorder(
                    isHighlighted ? Color.indigo : Color(uiColor: .separator),
                    style: StrokeStyle(lineWidth: isHighlighted ? 1.5 : 1, dash: [6, 4])
                )
        )

        if isEditing {
            content
        } else {
            Button {
                UIImpactFeedbackGenerator(style: .light).impactOccurred()
                withAnimation(.easeInOut(duration: 0.2)) {
                    isEditing = true
                }
            } label: {
                content
            }
            .buttonStyle(.plain)
        }
    }

    @ViewBuilder
    private func emptyMoreTemplatesDropZone(isHighlighted: Bool) -> some View {
        VStack(spacing: 4) {
            Text("拖至此处可移出长按快捷菜单")
                .font(.system(size: 13, weight: .semibold))
                .foregroundColor(isHighlighted ? .indigo : .secondary)
        }
        .frame(maxWidth: .infinity)
        .padding(.horizontal, 14)
        .padding(.vertical, 16)
        .background(isHighlighted ? Color.indigo.opacity(0.08) : Color.clear)
        .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
        .overlay(
            RoundedRectangle(cornerRadius: 12, style: .continuous)
                .strokeBorder(
                    isHighlighted ? Color.indigo : Color(uiColor: .separator),
                    style: StrokeStyle(lineWidth: isHighlighted ? 1.5 : 1, dash: [6, 4])
                )
        )
    }

    @ViewBuilder
    private func templateRowView(index: Int, item: WorkflowTemplateRecord) -> some View {
        let isDragging = draggingId == item.id
        let displacement = siblingDisplacement(for: index)

        WorkflowSwipeableRow(
            item: item,
            isEditing: isEditing,
            isDragging: isDragging,
            isSwiped: Binding(
                get: { swipedId == item.id },
                set: { open in
                    if open {
                        swipedId = item.id
                    } else if swipedId == item.id {
                        swipedId = nil
                    }
                }
            ),
            onHorizontalSwipeChanged: { active in
                if active {
                    horizontalSwipingId = item.id
                } else if horizontalSwipingId == item.id {
                    horizontalSwipingId = nil
                }
            },
            onSelect: {
                swipedId = nil
                UIImpactFeedbackGenerator(style: .medium).impactOccurred()
                onSelectTemplate(item.templatePrompt)
                dismiss()
            },
            onEdit: {
                swipedId = nil
                editorDraft = EditorDraft(id: item.id, isNew: false, record: item)
            },
            onDelete: {
                UIImpactFeedbackGenerator(style: .medium).impactOccurred()
                withAnimation(.snappy(duration: 0.2)) {
                    store.delete(id: item.id)
                }
                swipedId = nil
                if horizontalSwipingId == item.id {
                    horizontalSwipingId = nil
                }
            },
            dragHandleGesture: dragGesture(for: item, index: index)
        )
        .background(
            GeometryReader { geo in
                Color.clear.preference(
                    key: RowFramePreferenceKey.self,
                    value: [item.id: geo.frame(in: .named("wfScrollContent"))]
                )
            }
        )
        .offset(y: isDragging ? dragOffsetY : displacement)
        .zIndex(isDragging ? 10 : 0)
        .animation(isDragging ? nil : .snappy(duration: 0.18), value: displacement)
    }

    private func siblingDisplacement(for index: Int) -> CGFloat {
        guard let dId = draggingId,
              let start = dragStartIndex,
              let targetQuick = dragTargetInQuickMenu,
              index != start,
              store.templates.indices.contains(start),
              store.templates.indices.contains(index) else {
            return 0
        }
        let startItem = store.templates[start]
        let thisItem = store.templates[index]
        let draggedHeight = (rowFrames[dId]?.height ?? 68) + cardSpacing

        if startItem.inQuickMenu == targetQuick {
            // Same-section reorder
            guard thisItem.inQuickMenu == targetQuick else { return 0 }
            let sectionOthers = store.templates.enumerated().filter {
                $0.offset != start && $0.element.inQuickMenu == targetQuick
            }
            let startPosInSection = store.templates[..<start].filter { $0.inQuickMenu == targetQuick }.count
            guard let thisPosAmongOthers = sectionOthers.firstIndex(where: { $0.offset == index }) else { return 0 }
            // In original coordinates before removal:
            let origPos = thisPosAmongOthers >= startPosInSection ? thisPosAmongOthers + 1 : thisPosAmongOthers
            if dragInsertInTargetSection > startPosInSection && origPos > startPosInSection && origPos <= dragInsertInTargetSection {
                return -draggedHeight
            }
            if dragInsertInTargetSection < startPosInSection && origPos >= dragInsertInTargetSection && origPos < startPosInSection {
                return draggedHeight
            }
            return 0
        } else {
            // Cross-section drag
            if thisItem.inQuickMenu == startItem.inQuickMenu {
                return index > start ? -draggedHeight : 0
            } else if thisItem.inQuickMenu == targetQuick {
                let destOthers = store.templates.enumerated().filter {
                    $0.offset != start && $0.element.inQuickMenu == targetQuick
                }
                guard let posInDest = destOthers.firstIndex(where: { $0.offset == index }) else { return 0 }
                return posInDest >= dragInsertInTargetSection ? draggedHeight : 0
            }
            return 0
        }
    }

    private func updateDragTarget(startIndex: Int, offsetY: CGFloat) {
        let list = store.templates
        guard list.indices.contains(startIndex) else { return }
        let draggedItem = list[startIndex]
        let baseMidY = rowFrames[draggedItem.id]?.midY ?? 0
        let currentMidY = baseMidY + offsetY

        let nextInQuick = currentMidY < moreBoundaryMinY
        let destOthers = list.enumerated().filter {
            $0.offset != startIndex && $0.element.inQuickMenu == nextInQuick
        }
        var insertAt = 0
        for entry in destOthers {
            let mid = rowFrames[entry.element.id]?.midY ?? 0
            if currentMidY > mid {
                insertAt += 1
            } else {
                break
            }
        }

        if nextInQuick != dragTargetInQuickMenu || insertAt != dragInsertInTargetSection {
            dragTargetInQuickMenu = nextInQuick
            dragInsertInTargetSection = insertAt
            UIImpactFeedbackGenerator(style: .light).impactOccurred()
        }
    }

    private func dragGesture(for item: WorkflowTemplateRecord, index: Int) -> some Gesture {
        DragGesture(minimumDistance: 1, coordinateSpace: .named("wfScrollContent"))
            .onChanged { value in
                if draggingId == nil {
                    swipedId = nil
                    draggingId = item.id
                    dragStartIndex = index
                    dragTargetInQuickMenu = item.inQuickMenu
                    let posInSection = store.templates[..<index].filter { $0.inQuickMenu == item.inQuickMenu }.count
                    dragInsertInTargetSection = posInSection
                    UIImpactFeedbackGenerator(style: .light).impactOccurred()
                }
                dragOffsetY = value.translation.height
                if let start = dragStartIndex {
                    updateDragTarget(startIndex: start, offsetY: dragOffsetY)
                }
            }
            .onEnded { _ in
                let start = dragStartIndex
                let targetQuick = dragTargetInQuickMenu
                let insertSlot = dragInsertInTargetSection
                var tx = Transaction()
                tx.disablesAnimations = true
                withTransaction(tx) {
                    if let start, let targetQuick,
                       store.templates.indices.contains(start) {
                        var next = store.templates
                        var moved = next.remove(at: start)
                        let oldInQuick = moved.inQuickMenu
                        moved.inQuickMenu = targetQuick
                        let quickOthersCount = next.filter(\.inQuickMenu).count
                        let destGlobalIdx = targetQuick
                            ? min(max(0, insertSlot), quickOthersCount)
                            : min(max(quickOthersCount, quickOthersCount + insertSlot), next.count)
                        next.insert(moved, at: destGlobalIdx)
                        if targetQuick != oldInQuick || destGlobalIdx != start {
                            store.applyAndSync(next)
                        }
                    }
                    draggingId = nil
                    dragStartIndex = nil
                    dragTargetInQuickMenu = nil
                    dragInsertInTargetSection = 0
                    dragOffsetY = 0
                }
            }
    }

    private func openNewEditor(inQuickMenu: Bool) {
        let newRecord = WorkflowTemplateRecord(
            id: "wf_\(UUID().uuidString.lowercased())",
            title: "",
            subtitle: "",
            icon: "workflow",
            color: "indigo",
            templatePrompt: "",
            inQuickMenu: inQuickMenu
        )
        editorDraft = EditorDraft(id: newRecord.id, isNew: true, record: newRecord)
    }
}

/// 顶部可关闭的「微缩手势示意引导卡」
private struct WorkflowQuickGuideBanner: View {
    let isEditing: Bool
    let onEnterEdit: () -> Void
    let onDismiss: () -> Void

    var body: some View {
        HStack(alignment: .center, spacing: 12) {
            // 左侧：微缩长按上滑手势示意图
            VStack(spacing: 3) {
                VStack(spacing: 2) {
                    RoundedRectangle(cornerRadius: 2.5)
                        .fill(Color.primary.opacity(0.18))
                        .frame(width: 44, height: 6)
                    RoundedRectangle(cornerRadius: 2.5)
                        .fill(Color.indigo.opacity(0.55))
                        .frame(width: 44, height: 7)
                }
                .padding(4)
                .background(Color(uiColor: .systemBackground).opacity(0.85))
                .clipShape(RoundedRectangle(cornerRadius: 5, style: .continuous))
                .overlay(
                    RoundedRectangle(cornerRadius: 5, style: .continuous)
                        .stroke(Color.primary.opacity(0.12), lineWidth: 0.7)
                )

                Image(systemName: "arrow.up")
                    .font(.system(size: 8.5, weight: .bold))
                    .foregroundColor(.indigo)

                Text("Workflow")
                    .font(.system(size: 7.5, weight: .semibold))
                    .foregroundColor(.primary)
                    .padding(.horizontal, 6)
                    .padding(.vertical, 2)
                    .background(Color(uiColor: .systemBackground))
                    .clipShape(RoundedRectangle(cornerRadius: 4, style: .continuous))
                    .overlay(
                        RoundedRectangle(cornerRadius: 4, style: .continuous)
                            .stroke(Color.secondary.opacity(0.35), lineWidth: 0.7)
                    )
            }
            .padding(6)
            .background(Color.indigo.opacity(0.08))
            .clipShape(RoundedRectangle(cornerRadius: 9, style: .continuous))

            VStack(alignment: .leading, spacing: 4) {
                HStack(alignment: .top) {
                    Text("长按上滑，快捷输入常用模板")
                        .font(.system(size: 13.5, weight: .bold))
                        .foregroundColor(.primary)
                    Spacer(minLength: 4)
                    Button(action: onDismiss) {
                        Image(systemName: "xmark")
                            .font(.system(size: 11, weight: .semibold))
                            .foregroundColor(.secondary)
                            .frame(width: 22, height: 22)
                            .contentShape(Rectangle())
                    }
                    .buttonStyle(.plain)
                    .accessibilityLabel("关闭引导")
                }

                Text("聊天栏长按 Workflow 上滑松手即可填入；编辑后按住 ≡ 可拖入「长按快捷菜单」")
                    .font(.system(size: 11.5))
                    .foregroundColor(.secondary)
                    .fixedSize(horizontal: false, vertical: true)

                if !isEditing {
                    Button(action: onEnterEdit) {
                        Text("去配置 ›")
                            .font(.system(size: 11.5, weight: .semibold))
                            .foregroundColor(.indigo)
                            .padding(.horizontal, 10)
                            .padding(.vertical, 4)
                            .background(Color.indigo.opacity(0.14))
                            .clipShape(Capsule())
                    }
                    .buttonStyle(.plain)
                    .padding(.top, 2)
                }
            }
        }
        .padding(12)
        .background(Color(uiColor: .secondarySystemBackground))
        .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
        .overlay(
            RoundedRectangle(cornerRadius: 12, style: .continuous)
                .strokeBorder(Color.indigo.opacity(0.22), lineWidth: 0.8)
        )
    }
}

/// 支持左滑删除与编辑模式短按直接编辑的单行卡片。
private struct WorkflowSwipeableRow<DragHandleGesture: Gesture>: View {
    let item: WorkflowTemplateRecord
    let isEditing: Bool
    let isDragging: Bool
    @Binding var isSwiped: Bool
    let onHorizontalSwipeChanged: (Bool) -> Void
    let onSelect: () -> Void
    let onEdit: () -> Void
    let onDelete: () -> Void
    let dragHandleGesture: DragHandleGesture

    @State private var swipeDragX: CGFloat = 0
    @State private var swipeActivationDx: CGFloat = 0
    @State private var isHorizontalSwiping = false

    private let maxRevealWidth: CGFloat = 68

    private var currentOffsetX: CGFloat {
        let base: CGFloat = isSwiped ? -maxRevealWidth : 0
        return min(0, max(-maxRevealWidth, base + swipeDragX))
    }

    private var deleteRevealProgress: CGFloat {
        min(1, max(0, (-currentOffsetX - 6) / 36))
    }

    var body: some View {
        let iconColor = WorkflowTemplateStyle.color(for: item.color)
        let displaySubtitle = item.subtitle.isEmpty ? item.templatePrompt : item.subtitle

        ZStack(alignment: .trailing) {
            Button(action: onDelete) {
                ZStack {
                    Circle()
                        .fill(Color.red)
                        .frame(width: 38, height: 38)
                    Image(systemName: "trash")
                        .font(.system(size: 16, weight: .semibold))
                        .foregroundColor(.white)
                }
                .frame(width: maxRevealWidth)
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .opacity(deleteRevealProgress)
            .scaleEffect(0.72 + 0.28 * deleteRevealProgress)
            .allowsHitTesting(isSwiped)
            .accessibilityLabel("删除 \(item.title)")

            HStack(spacing: 12) {
                if isEditing {
                    Image(systemName: "line.3.horizontal")
                        .font(.system(size: 16, weight: .semibold))
                        .foregroundColor(isDragging ? .indigo : .secondary)
                        .frame(width: 28, height: 36)
                        .contentShape(Rectangle())
                        .highPriorityGesture(dragHandleGesture)
                }

                ZStack {
                    RoundedRectangle(cornerRadius: 10)
                        .fill(iconColor.opacity(0.15))
                        .frame(width: 40, height: 40)
                    WorkflowVectorIconView(iconKey: item.icon, size: 20, color: iconColor)
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

                if !isEditing {
                    Image(systemName: "chevron.right")
                        .font(.system(size: 12, weight: .bold))
                        .foregroundColor(Color(uiColor: .tertiaryLabel))
                }
            }
            .padding(12)
            .background(Color(uiColor: .secondarySystemBackground))
            .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
            .overlay(
                RoundedRectangle(cornerRadius: 12, style: .continuous)
                    .strokeBorder(
                        isDragging ? Color.indigo.opacity(0.85) : Color(uiColor: .separator).opacity(0.4),
                        lineWidth: isDragging ? 1.5 : 0.5
                    )
            )
            .contentShape(Rectangle())
            .offset(x: currentOffsetX)
            .onTapGesture {
                if isSwiped {
                    withAnimation(.snappy(duration: 0.2)) {
                        isSwiped = false
                    }
                } else if isEditing {
                    UIImpactFeedbackGenerator(style: .light).impactOccurred()
                    onEdit()
                } else {
                    onSelect()
                }
            }
        }
        .simultaneousGesture(
            DragGesture(minimumDistance: 12, coordinateSpace: .global)
                .onChanged { value in
                    guard !isDragging else { return }
                    let dx = value.translation.width
                    let dy = value.translation.height
                    if !isHorizontalSwiping {
                        guard abs(dx) > 12, abs(dx) > abs(dy) * 1.4 else { return }
                        isHorizontalSwiping = true
                        swipeActivationDx = dx
                        onHorizontalSwipeChanged(true)
                    }
                    swipeDragX = dx - swipeActivationDx
                }
                .onEnded { _ in
                    let finalDelta = swipeDragX
                    let wasSwiping = isHorizontalSwiping
                    swipeDragX = 0
                    swipeActivationDx = 0
                    isHorizontalSwiping = false
                    onHorizontalSwipeChanged(false)
                    guard wasSwiping else { return }
                    let projected = (isSwiped ? -maxRevealWidth : 0) + finalDelta
                    withAnimation(.snappy(duration: 0.2)) {
                        isSwiped = projected < -maxRevealWidth * 0.45
                    }
                }
        )
        .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
        .scaleEffect(isDragging ? 1.015 : 1.0)
        .shadow(color: isDragging ? Color.black.opacity(0.12) : .clear, radius: 8, y: 4)
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
    @State private var inQuickMenu: Bool

    init(isNew: Bool, initial: WorkflowTemplateRecord, onSave: @escaping (WorkflowTemplateRecord) -> Void) {
        self.isNew = isNew
        self.onSave = onSave
        _id = State(initialValue: initial.id)
        _title = State(initialValue: initial.title)
        _subtitle = State(initialValue: initial.subtitle)
        _icon = State(initialValue: WorkflowTemplateStyle.normalizedIconKey(initial.icon))
        _color = State(initialValue: initial.color)
        _templatePrompt = State(initialValue: initial.templatePrompt)
        _inQuickMenu = State(initialValue: initial.inQuickMenu)
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

                Section {
                    Toggle("加入长按快捷菜单", isOn: $inQuickMenu)
                } footer: {
                    Text("开启后，在聊天栏长按 Workflow 按钮向上滑动即可直接快速填入该模板。")
                }

                Section("图标与主题色") {
                    ScrollView(.horizontal, showsIndicators: false) {
                        HStack(spacing: 12) {
                            ForEach(WorkflowTemplateStyle.colors) { c in
                                let selected = color == c.key
                                Button {
                                    color = c.key
                                } label: {
                                    ZStack {
                                        if selected {
                                            Circle()
                                                .strokeBorder(Color.primary, lineWidth: 2.2)
                                                .frame(width: 34, height: 34)
                                        }
                                        Circle()
                                            .fill(c.color)
                                            .frame(width: 26, height: 26)
                                    }
                                    .frame(width: 36, height: 36)
                                    .contentShape(Circle())
                                }
                                .buttonStyle(.plain)
                                .accessibilityLabel(c.label)
                            }
                        }
                        .padding(.horizontal, 4)
                        .padding(.vertical, 4)
                    }

                    let selectedColor = WorkflowTemplateStyle.color(for: color)
                    LazyVGrid(columns: Array(repeating: GridItem(.flexible(), spacing: 10), count: 5), spacing: 10) {
                        ForEach(WorkflowTemplateStyle.icons) { ic in
                            let active = icon == ic.key
                            Button {
                                icon = ic.key
                            } label: {
                                WorkflowVectorIconView(
                                    iconKey: ic.key,
                                    size: 20,
                                    color: active ? selectedColor : .secondary
                                )
                                .frame(maxWidth: .infinity)
                                .frame(height: 40)
                                .background(active ? selectedColor.opacity(0.16) : Color(uiColor: .tertiarySystemFill))
                                .cornerRadius(10)
                                .overlay(
                                    RoundedRectangle(cornerRadius: 10)
                                        .strokeBorder(active ? selectedColor : .clear, lineWidth: 1.5)
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
                            templatePrompt: templatePrompt.trimmingCharacters(in: .whitespacesAndNewlines),
                            inQuickMenu: inQuickMenu
                        )
                        onSave(saved)
                        dismiss()
                    }
                    .fontWeight(.semibold)
                    .disabled(!canSave)
                }
            }
        }
        .presentationDetents([.large])
        .presentationDragIndicator(.visible)
    }
}
