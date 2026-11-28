import SwiftUI

/// 新建对话 2×2 快捷指令仓库：本地 UserDefaults 毫秒级兜底 + Go 网关 `/gateway/launchpad-shortcuts` 跨端同步。
@MainActor
@Observable
public final class LaunchpadShortcutsStore {
    public static let shared = LaunchpadShortcutsStore()
    public static let maxSlots = 4
    private static let cacheKey = "mgy_launchpad_shortcuts_v1"

    public enum PinResult: Equatable {
        case added(Int)
        case alreadyPinned
        case full
    }

    public private(set) var customized: Bool = false
    public private(set) var items: [LaunchpadShortcutItem] = []
    public private(set) var isSyncing: Bool = false
    public var syncError: String?

    private struct LocalCache: Codable {
        let customized: Bool
        let items: [LaunchpadShortcutItem]
    }

    private init() {
        if let cached = Self.loadCache() {
            self.customized = cached.customized && !cached.items.isEmpty
            self.items = Array(cached.items.prefix(Self.maxSlots))
        }
    }

    private static func loadCache() -> LocalCache? {
        guard let data = UserDefaults.standard.data(forKey: cacheKey) else { return nil }
        return try? JSONDecoder().decode(LocalCache.self, from: data)
    }

    private static func saveCache(customized: Bool, items: [LaunchpadShortcutItem]) {
        let payload = LocalCache(customized: customized && !items.isEmpty, items: Array(items.prefix(Self.maxSlots)))
        if let data = try? JSONEncoder().encode(payload) {
            UserDefaults.standard.set(data, forKey: cacheKey)
        }
    }

    public func isPinned(kind: String, ref: String) -> Bool {
        let probe = LaunchpadShortcutItem(kind: kind, ref: ref)
        return items.contains(where: { $0.id == probe.id })
    }

    public func refresh() async {
        guard let baseURL = AppSettings.shared.serverURL else { return }
        do {
            let resp = try await APIClient.shared.fetchLaunchpadShortcuts(baseURL: baseURL)
            let cleaned = Array(resp.items.prefix(Self.maxSlots))
            let nextCustom = resp.customized && !cleaned.isEmpty
            if nextCustom != customized || cleaned != items {
                customized = nextCustom
                items = cleaned
                Self.saveCache(customized: nextCustom, items: cleaned)
            }
            if syncError != nil { syncError = nil }
        } catch {
            // 离线或旧版本网关 404 时静默保留本地缓存
        }
    }

    @discardableResult
    public func pin(_ item: LaunchpadShortcutItem) -> PinResult {
        if items.contains(where: { $0.id == item.id }) {
            return .alreadyPinned
        }
        guard items.count < Self.maxSlots else {
            return .full
        }
        var next = items
        next.append(item)
        applyAndSync(next, customized: true)
        return .added(next.count)
    }

    public func unpin(kind: String, ref: String) {
        let probe = LaunchpadShortcutItem(kind: kind, ref: ref)
        let next = items.filter { $0.id != probe.id }
        applyAndSync(next, customized: !next.isEmpty)
    }

    public func remove(at index: Int) {
        guard items.indices.contains(index) else { return }
        var next = items
        next.remove(at: index)
        applyAndSync(next, customized: !next.isEmpty)
    }

    public func replace(at index: Int, with item: LaunchpadShortcutItem) {
        guard items.indices.contains(index) else { return }
        var next = items.filter { $0.id != item.id }
        let safeIdx = min(index, next.count)
        if next.count >= Self.maxSlots && safeIdx < next.count {
            next[safeIdx] = item
        } else if next.count < Self.maxSlots {
            next.insert(item, at: safeIdx)
        }
        applyAndSync(Array(next.prefix(Self.maxSlots)), customized: !next.isEmpty)
    }

    public func move(from sourceIndex: Int, to destIndex: Int) {
        guard items.indices.contains(sourceIndex),
              items.indices.contains(destIndex),
              sourceIndex != destIndex else { return }
        var next = items
        let elem = next.remove(at: sourceIndex)
        next.insert(elem, at: destIndex)
        applyAndSync(next, customized: true)
    }

    public func applyAndSync(_ raw: [LaunchpadShortcutItem], customized customFlag: Bool) {
        var dedup: [LaunchpadShortcutItem] = []
        var seen = Set<String>()
        for item in raw {
            if seen.insert(item.id).inserted {
                dedup.append(item)
                if dedup.count == Self.maxSlots { break }
            }
        }
        let effectiveCustom = customFlag && !dedup.isEmpty
        self.customized = effectiveCustom
        self.items = effectiveCustom ? dedup : []
        Self.saveCache(customized: self.customized, items: self.items)
        self.syncError = nil

        guard let baseURL = AppSettings.shared.serverURL else { return }
        isSyncing = true
        let payloadItems = self.items
        let payloadCustom = self.customized
        Task {
            defer { isSyncing = false }
            do {
                let resp: LaunchpadShortcutsResponse
                if payloadCustom {
                    resp = try await APIClient.shared.saveLaunchpadShortcuts(payloadItems, customized: true, baseURL: baseURL)
                } else {
                    resp = try await APIClient.shared.resetLaunchpadShortcuts(baseURL: baseURL)
                }
                let serverItems = Array(resp.items.prefix(Self.maxSlots))
                let serverCustom = resp.customized && !serverItems.isEmpty
                if serverCustom != self.customized || serverItems != self.items {
                    self.customized = serverCustom
                    self.items = serverItems
                    Self.saveCache(customized: serverCustom, items: serverItems)
                }
            } catch {
                // 静默兜底本地保存，兼容旧网关
            }
        }
    }

    public func resetToSmartDefaults() {
        applyAndSync([], customized: false)
    }
}

// MARK: - 2×2 快捷指令配置 Sheet

struct LaunchpadShortcutConfiguratorSheet: View {
    let slashCommands: [SlashCommandOption]
    let pendingPinItem: LaunchpadShortcutItem?
    let onOpenWorkflowManager: () -> Void

    @Environment(\.dismiss) private var dismiss
    @State private var shortcutStore = LaunchpadShortcutsStore.shared
    @State private var workflowStore = WorkflowTemplateStore.shared
    @State private var activeTab: PickerTab = .workflow
    @State private var searchQuery: String = ""
    @State private var activePendingItem: LaunchpadShortcutItem?
    @State private var toastMessage: String?

    private enum PickerTab: Int, CaseIterable, Identifiable {
        case workflow = 0
        case slash = 1
        case skill = 2
        case prompt = 3

        var id: Int { rawValue }
    }

    private var uiLangCode: String {
        let lang = (Bundle.main.preferredLocalizations.first ?? Locale.preferredLanguages.first ?? "zh").lowercased()
        if lang.hasPrefix("ja") { return "ja" }
        if lang.hasPrefix("en") { return "en" }
        return "zh"
    }

    private func t3(_ zh: String, _ en: String, _ ja: String) -> String {
        switch uiLangCode {
        case "en": return en
        case "ja": return ja
        default: return zh
        }
    }

    private func tabTitle(_ tab: PickerTab) -> String {
        switch tab {
        case .workflow: return "Workflow"
        case .slash: return t3("斜杠", "Slash", "スラッシュ")
        case .skill: return t3("技能", "Skills", "スキル")
        case .prompt: return t3("模板", "Prompts", "テンプレート")
        }
    }

    private func showToast(_ text: String) {
        withAnimation(.easeOut(duration: 0.18)) {
            toastMessage = text
        }
        DispatchQueue.main.asyncAfter(deadline: .now() + 1.8) {
            withAnimation(.easeOut(duration: 0.2)) {
                if toastMessage == text { toastMessage = nil }
            }
        }
    }

    private func resolveDisplay(for item: LaunchpadShortcutItem) -> (svgKey: String, tint: Color, title: String, subtitle: String) {
        switch item.kind {
        case "workflow":
            if let wf = workflowStore.templates.first(where: { $0.id == item.ref }) {
                return (
                    "wf:\(WorkflowTemplateStyle.normalizedIconKey(wf.icon))",
                    LaunchpadCatalog.workflowTint(wf.color),
                    wf.title,
                    wf.subtitle.isEmpty ? t3("自定义 Workflow", "Custom Workflow", "カスタム Workflow") : wf.subtitle
                )
            }
            let fallbackKey = (item.svgKey ?? "").hasPrefix("wf:") ? (item.svgKey ?? "wf:workflow") : "wf:workflow"
            return (
                fallbackKey,
                LaunchpadCatalog.workflowTint(item.tint ?? "indigo"),
                item.title ?? item.ref,
                item.subtitle ?? t3("Workflow 模板", "Workflow template", "Workflow テンプレート")
            )
        case "slash", "skill":
            if let spec = LaunchpadCatalog.coreSlashPool.first(where: { $0.name == item.ref.lowercased() }) {
                return (spec.svgKey, spec.tint, t3(spec.titleZh, spec.titleEn, spec.titleJa), t3(spec.subZh, spec.subEn, spec.subJa))
            }
            if let live = slashCommands.first(where: { $0.name.lowercased() == item.ref.lowercased() }) {
                let desc = live.description.trimmingCharacters(in: .whitespacesAndNewlines)
                return (
                    "slash_skill",
                    LaunchpadCatalog.tintTeal,
                    "/\(live.name)",
                    desc.isEmpty ? t3("已安装技能指令", "Installed skill", "スキルコマンド") : desc
                )
            }
            return (
                item.svgKey ?? "slash_skill",
                LaunchpadCatalog.tintColor(item.tint ?? "teal"),
                item.title ?? "/\(item.ref)",
                item.subtitle ?? t3("斜杠 / 技能指令", "Slash / Skill", "スラッシュ / スキル")
            )
        case "prompt":
            if let spec = LaunchpadCatalog.allPromptPool.first(where: { $0.id == item.ref }) {
                return (spec.svgKey, spec.tint, t3(spec.titleZh, spec.titleEn, spec.titleJa), t3(spec.subZh, spec.subEn, spec.subJa))
            }
            return (
                item.svgKey ?? "prompt_draft",
                LaunchpadCatalog.tintColor(item.tint ?? "blue"),
                item.title ?? item.ref,
                item.subtitle ?? t3("场景提示词模板", "Prompt template", "プロンプトテンプレート")
            )
        default:
            return ("slash_skill", LaunchpadCatalog.tintIndigo, item.title ?? item.ref, item.subtitle ?? "")
        }
    }

    private func toggleItem(_ item: LaunchpadShortcutItem) {
        if shortcutStore.isPinned(kind: item.kind, ref: item.ref) {
            UIImpactFeedbackGenerator(style: .light).impactOccurred()
            shortcutStore.unpin(kind: item.kind, ref: item.ref)
            return
        }
        switch shortcutStore.pin(item) {
        case .added(let count):
            UIImpactFeedbackGenerator(style: .light).impactOccurred()
            showToast(t3("已固定到 2×2 (\(count)/4)", "Pinned to 2×2 (\(count)/4)", "2×2 に固定しました (\(count)/4)"))
        case .alreadyPinned:
            break
        case .full:
            UINotificationFeedbackGenerator().notificationOccurred(.warning)
            showToast(t3("已选满 4 项，请先在上方移除一项", "4/4 slots full — remove one above first", "4枠が埋まっています。上から1つ削除してください"))
        }
    }

    var body: some View {
        NavigationStack {
            ZStack(alignment: .bottom) {
                ScrollView {
                    VStack(alignment: .leading, spacing: 16) {
                        if let pending = activePendingItem {
                            pendingSwapBanner(pending)
                        }

                        // 1. 当前 2×2 槽位预览
                        slotsPreviewSection

                        // 2. 分段分类 + 搜索
                        VStack(spacing: 10) {
                            Picker("", selection: $activeTab) {
                                ForEach(PickerTab.allCases) { tab in
                                    Text(tabTitle(tab)).tag(tab)
                                }
                            }
                            .pickerStyle(.segmented)

                            HStack(spacing: 8) {
                                Image(systemName: "magnifyingglass")
                                    .font(.system(size: 13))
                                    .foregroundColor(.secondary)
                                TextField(t3("搜索指令、技能或模板...", "Search commands, skills or prompts...", "コマンド・スキル・テンプレートを検索..."), text: $searchQuery)
                                    .font(.system(size: 14))
                                    .textInputAutocapitalization(.never)
                                    .autocorrectionDisabled()
                                if !searchQuery.isEmpty {
                                    Button {
                                        searchQuery = ""
                                    } label: {
                                        Image(systemName: "xmark.circle.fill")
                                            .font(.system(size: 14))
                                            .foregroundColor(.secondary)
                                    }
                                    .buttonStyle(.plain)
                                }
                            }
                            .padding(.horizontal, 12)
                            .padding(.vertical, 9)
                            .background(Color(uiColor: .secondarySystemBackground))
                            .clipShape(RoundedRectangle(cornerRadius: 11, style: .continuous))
                        }

                        // 3. 候选素材列表
                        candidateListSection
                    }
                    .padding(.horizontal, 16)
                    .padding(.vertical, 12)
                    .padding(.bottom, 36)
                }
                .background(Color(uiColor: .systemGroupedBackground))

                if let msg = toastMessage {
                    Text(msg)
                        .font(.system(size: 12.5, weight: .semibold))
                        .foregroundColor(.white)
                        .padding(.horizontal, 14)
                        .padding(.vertical, 9)
                        .background(Color.black.opacity(0.82), in: Capsule())
                        .padding(.bottom, 16)
                        .transition(.move(edge: .bottom).combined(with: .opacity))
                }
            }
            .navigationTitle(t3("自定义 2×2 快捷指令", "Customize 2×2 Shortcuts", "2×2 ショートカット設定"))
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Button(t3("完成", "Done", "完了")) {
                        dismiss()
                    }
                    .fontWeight(.semibold)
                }
            }
        }
        .onAppear {
            activePendingItem = pendingPinItem
        }
        .task {
            await shortcutStore.refresh()
            await workflowStore.refresh()
        }
    }

    private func pendingSwapBanner(_ pending: LaunchpadShortcutItem) -> some View {
        let info = resolveDisplay(for: pending)
        return HStack(spacing: 10) {
            ZStack {
                RoundedRectangle(cornerRadius: 7, style: .continuous)
                    .fill(info.tint.opacity(0.16))
                    .frame(width: 26, height: 26)
                LaunchpadSVGIconView(iconKey: info.svgKey, tint: info.tint, size: 14)
            }
            VStack(alignment: .leading, spacing: 2) {
                Text(t3("2×2 已满，点选下方任意槽位替换为：", "2×2 is full — tap any slot below to replace with:", "2×2が満枠です。下の枠をタップして置換："))
                    .font(.system(size: 11.5))
                    .foregroundColor(.secondary)
                Text(info.title)
                    .font(.system(size: 13.5, weight: .semibold))
                    .foregroundColor(.primary)
                    .lineLimit(1)
            }
            Spacer(minLength: 4)
            Button(t3("取消", "Cancel", "キャンセル")) {
                withAnimation(.easeOut(duration: 0.18)) {
                    activePendingItem = nil
                }
            }
            .font(.system(size: 12.5, weight: .medium))
            .foregroundColor(.secondary)
        }
        .padding(12)
        .background(LaunchpadCatalog.tintAmber.opacity(0.12))
        .clipShape(RoundedRectangle(cornerRadius: 13, style: .continuous))
        .overlay(
            RoundedRectangle(cornerRadius: 13, style: .continuous)
                .stroke(LaunchpadCatalog.tintAmber.opacity(0.35), lineWidth: 1)
        )
    }

    private var slotsPreviewSection: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack {
                Text(t3("当前已固定 (\(shortcutStore.items.count)/4)", "Pinned Slots (\(shortcutStore.items.count)/4)", "固定済み (\(shortcutStore.items.count)/4)"))
                    .font(.system(size: 13, weight: .semibold))
                    .foregroundColor(.primary)
                Spacer()
                if shortcutStore.customized {
                    Button {
                        UIImpactFeedbackGenerator(style: .light).impactOccurred()
                        shortcutStore.resetToSmartDefaults()
                        showToast(t3("已恢复按常用智能推荐", "Reset to smart recommendations", "自動おすすめに戻しました"))
                    } label: {
                        Text(t3("恢复智能推荐", "Smart Default", "おすすめに戻す"))
                            .font(.system(size: 12, weight: .medium))
                            .foregroundColor(.indigo)
                    }
                    .buttonStyle(.plain)
                }
            }

            LazyVGrid(
                columns: [
                    GridItem(.flexible(), spacing: 10),
                    GridItem(.flexible(), spacing: 10)
                ],
                spacing: 10
            ) {
                ForEach(0..<LaunchpadShortcutsStore.maxSlots, id: \.self) { idx in
                    if idx < shortcutStore.items.count {
                        let item = shortcutStore.items[idx]
                        let info = resolveDisplay(for: item)
                        slotFilledCard(index: idx, item: item, info: info)
                    } else {
                        slotEmptyCard(index: idx)
                    }
                }
            }

            Text(t3(
                "未填满的空位将自动用场景模板与随机斜杠补齐，配置跨端同步。",
                "Unfilled slots auto-fill with prompt templates & random slash commands, synced across devices.",
                "空き枠はテンプレートとランダムなコマンドで自動補完され、全端末に同期されます。"
            ))
            .font(.system(size: 11.5))
            .foregroundColor(.secondary)
        }
    }

    private func slotFilledCard(index: Int, item: LaunchpadShortcutItem, info: (svgKey: String, tint: Color, title: String, subtitle: String)) -> some View {
        let isSwapMode = activePendingItem != nil
        return VStack(alignment: .leading, spacing: 6) {
            HStack(spacing: 6) {
                ZStack {
                    RoundedRectangle(cornerRadius: 6, style: .continuous)
                        .fill(info.tint.opacity(0.14))
                        .frame(width: 22, height: 22)
                    LaunchpadSVGIconView(iconKey: info.svgKey, tint: info.tint, size: 13)
                }
                Text(info.title)
                    .font(.system(size: 13, weight: .semibold))
                    .foregroundColor(.primary)
                    .lineLimit(1)
                Spacer(minLength: 2)
                if !isSwapMode {
                    Button {
                        UIImpactFeedbackGenerator(style: .light).impactOccurred()
                        shortcutStore.remove(at: index)
                    } label: {
                        Image(systemName: "xmark.circle.fill")
                            .font(.system(size: 15))
                            .foregroundColor(Color(uiColor: .tertiaryLabel))
                    }
                    .buttonStyle(.plain)
                }
            }

            HStack(spacing: 6) {
                Text(isSwapMode ? t3("点此替换", "Tap to replace", "タップして置換") : info.subtitle)
                    .font(.system(size: 11))
                    .foregroundColor(isSwapMode ? LaunchpadCatalog.tintAmber : .secondary)
                    .lineLimit(1)
                Spacer(minLength: 2)
                if !isSwapMode && shortcutStore.items.count > 1 {
                    HStack(spacing: 4) {
                        if index > 0 {
                            Button {
                                UIImpactFeedbackGenerator(style: .light).impactOccurred()
                                shortcutStore.move(from: index, to: index - 1)
                            } label: {
                                Image(systemName: "chevron.left")
                                    .font(.system(size: 10, weight: .semibold))
                                    .foregroundColor(.secondary)
                                    .frame(width: 18, height: 18)
                                    .background(Color.primary.opacity(0.06), in: Circle())
                            }
                            .buttonStyle(.plain)
                        }
                        if index < shortcutStore.items.count - 1 {
                            Button {
                                UIImpactFeedbackGenerator(style: .light).impactOccurred()
                                shortcutStore.move(from: index, to: index + 1)
                            } label: {
                                Image(systemName: "chevron.right")
                                    .font(.system(size: 10, weight: .semibold))
                                    .foregroundColor(.secondary)
                                    .frame(width: 18, height: 18)
                                    .background(Color.primary.opacity(0.06), in: Circle())
                            }
                            .buttonStyle(.plain)
                        }
                    }
                }
            }
        }
        .padding(.horizontal, 11)
        .padding(.vertical, 10)
        .background(Color(uiColor: .secondarySystemGroupedBackground))
        .clipShape(RoundedRectangle(cornerRadius: 13, style: .continuous))
        .overlay(
            RoundedRectangle(cornerRadius: 13, style: .continuous)
                .stroke(isSwapMode ? LaunchpadCatalog.tintAmber.opacity(0.55) : Color.primary.opacity(0.08), lineWidth: isSwapMode ? 1.2 : 0.8)
        )
        .contentShape(RoundedRectangle(cornerRadius: 13, style: .continuous))
        .onTapGesture {
            if let pending = activePendingItem {
                UIImpactFeedbackGenerator(style: .medium).impactOccurred()
                shortcutStore.replace(at: index, with: pending)
                activePendingItem = nil
                showToast(t3("已替换槽位 \(index + 1)", "Replaced slot \(index + 1)", "枠 \(index + 1) を置換しました"))
            }
        }
    }

    private func slotEmptyCard(index: Int) -> some View {
        VStack(spacing: 4) {
            Image(systemName: "plus")
                .font(.system(size: 13, weight: .medium))
                .foregroundColor(Color(uiColor: .tertiaryLabel))
            Text(t3("空位 \(index + 1) · 自动补位", "Slot \(index + 1) · Auto", "枠 \(index + 1) · 自動補完"))
                .font(.system(size: 11))
                .foregroundColor(.secondary)
        }
        .frame(maxWidth: .infinity)
        .frame(height: 62)
        .background(Color(uiColor: .secondarySystemGroupedBackground).opacity(0.5))
        .clipShape(RoundedRectangle(cornerRadius: 13, style: .continuous))
        .overlay(
            RoundedRectangle(cornerRadius: 13, style: .continuous)
                .stroke(Color.primary.opacity(0.14), style: StrokeStyle(lineWidth: 1, dash: [4, 4]))
        )
    }

    @ViewBuilder
    private var candidateListSection: some View {
        let q = searchQuery.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        switch activeTab {
        case .workflow:
            VStack(alignment: .leading, spacing: 8) {
                let list = workflowStore.templates.filter { tpl in
                    q.isEmpty || tpl.title.lowercased().contains(q) || tpl.subtitle.lowercased().contains(q)
                }
                ForEach(list) { tpl in
                    let wfIconKey = "wf:\(WorkflowTemplateStyle.normalizedIconKey(tpl.icon))"
                    let shortcut = LaunchpadShortcutItem(
                        kind: "workflow",
                        ref: tpl.id,
                        title: tpl.title,
                        subtitle: tpl.subtitle,
                        svgKey: wfIconKey,
                        tint: LaunchpadCatalog.safeTintName(tpl.color)
                    )
                    candidateRow(
                        item: shortcut,
                        svgKey: wfIconKey,
                        tint: LaunchpadCatalog.workflowTint(tpl.color),
                        title: tpl.title,
                        subtitle: tpl.subtitle.isEmpty ? tpl.templatePrompt : tpl.subtitle,
                        badge: tpl.inQuickMenu ? t3("快选", "Quick", "クイック") : nil
                    )
                }

                Button {
                    dismiss()
                    DispatchQueue.main.asyncAfter(deadline: .now() + 0.25) {
                        onOpenWorkflowManager()
                    }
                } label: {
                    HStack(spacing: 6) {
                        Image(systemName: "slider.horizontal.3")
                            .font(.system(size: 13))
                        Text(t3("新建或编辑 Workflow 模板", "Create or edit Workflow templates", "Workflow テンプレートを管理"))
                            .font(.system(size: 13, weight: .medium))
                    }
                    .foregroundColor(.indigo)
                    .frame(maxWidth: .infinity)
                    .padding(.vertical, 12)
                    .background(Color(uiColor: .secondarySystemGroupedBackground))
                    .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
                }
                .buttonStyle(.plain)
            }

        case .slash:
            VStack(alignment: .leading, spacing: 8) {
                let localCounts = SlashUsageLocalStore.counts()
                let gatewayByName: [String: SlashCommandOption] = Dictionary(
                    slashCommands.map { ($0.name.lowercased(), $0) },
                    uniquingKeysWith: { first, _ in first }
                )
                let coreNames = Set(LaunchpadCatalog.coreSlashPool.map(\.name))

                let coreFiltered = LaunchpadCatalog.coreSlashPool.filter { spec in
                    q.isEmpty
                        || spec.name.contains(q)
                        || spec.titleZh.lowercased().contains(q)
                        || spec.titleEn.lowercased().contains(q)
                        || spec.subZh.lowercased().contains(q)
                }
                ForEach(coreFiltered, id: \.name) { spec in
                    let liveCount = max(gatewayByName[spec.name]?.usageCount ?? 0, localCounts[spec.name] ?? 0)
                    let item = LaunchpadShortcutItem(
                        kind: "slash",
                        ref: spec.name,
                        title: t3(spec.titleZh, spec.titleEn, spec.titleJa),
                        subtitle: t3(spec.subZh, spec.subEn, spec.subJa),
                        svgKey: spec.svgKey,
                        tint: spec.tintName
                    )
                    candidateRow(
                        item: item,
                        svgKey: spec.svgKey,
                        tint: spec.tint,
                        title: t3(spec.titleZh, spec.titleEn, spec.titleJa),
                        subtitle: t3(spec.subZh, spec.subEn, spec.subJa),
                        badge: liveCount > 0 ? t3("用过 \(liveCount) 次", "\(liveCount)×", "\(liveCount)回") : nil
                    )
                }

                let extraSystem = slashCommands.filter { cmd in
                    let lower = cmd.name.lowercased()
                    return cmd.kind.lowercased() == "system"
                        && !coreNames.contains(lower)
                        && !LaunchpadCatalog.midChatExcludedSlashes.contains(lower)
                        && (q.isEmpty || lower.contains(q) || cmd.description.lowercased().contains(q))
                }
                ForEach(extraSystem) { cmd in
                    let liveCount = max(cmd.usageCount, localCounts[cmd.name.lowercased()] ?? 0)
                    let item = LaunchpadShortcutItem(
                        kind: "slash",
                        ref: cmd.name,
                        title: "/\(cmd.name)",
                        subtitle: cmd.description,
                        svgKey: "slash_skill",
                        tint: "blue"
                    )
                    candidateRow(
                        item: item,
                        svgKey: "slash_skill",
                        tint: LaunchpadCatalog.tintBlue,
                        title: "/\(cmd.name)",
                        subtitle: cmd.description.isEmpty ? t3("系统斜杠指令", "System slash command", "システムコマンド") : cmd.description,
                        badge: liveCount > 0 ? t3("用过 \(liveCount) 次", "\(liveCount)×", "\(liveCount)回") : nil
                    )
                }
            }

        case .skill:
            VStack(alignment: .leading, spacing: 8) {
                let localCounts = SlashUsageLocalStore.counts()
                let slashExcludedNames = Set(LaunchpadCatalog.coreSlashPool.map(\.name))
                    .union(LaunchpadCatalog.midChatExcludedSlashes)
                let skills = slashCommands
                    .filter { cmd in
                        let lower = cmd.name.lowercased()
                        return cmd.kind.lowercased() != "system" && !slashExcludedNames.contains(lower)
                    }
                    .filter { cmd in
                        q.isEmpty
                            || cmd.name.lowercased().contains(q)
                            || cmd.title.lowercased().contains(q)
                            || cmd.description.lowercased().contains(q)
                    }
                    .sorted { lhs, rhs in
                        let lc = max(lhs.usageCount, localCounts[lhs.name.lowercased()] ?? 0)
                        let rc = max(rhs.usageCount, localCounts[rhs.name.lowercased()] ?? 0)
                        if lc != rc { return lc > rc }
                        return lhs.name.lowercased() < rhs.name.lowercased()
                    }

                if skills.isEmpty {
                    Text(t3("当前未检测到匹配的已安装技能", "No matching installed skills found", "一致するインストール済みスキルはありません"))
                        .font(.system(size: 13))
                        .foregroundColor(.secondary)
                        .frame(maxWidth: .infinity, alignment: .center)
                        .padding(.vertical, 28)
                } else {
                    ForEach(skills) { cmd in
                        let liveCount = max(cmd.usageCount, localCounts[cmd.name.lowercased()] ?? 0)
                        let item = LaunchpadShortcutItem(
                            kind: "skill",
                            ref: cmd.name,
                            title: "/\(cmd.name)",
                            subtitle: cmd.description,
                            svgKey: "slash_skill",
                            tint: "teal"
                        )
                        candidateRow(
                            item: item,
                            svgKey: "slash_skill",
                            tint: LaunchpadCatalog.tintTeal,
                            title: "/\(cmd.name)",
                            subtitle: cmd.description.isEmpty ? t3("已安装技能", "Installed skill", "インストール済みスキル") : cmd.description,
                            badge: liveCount > 0 ? t3("用过 \(liveCount) 次", "\(liveCount)×", "\(liveCount)回") : nil
                        )
                    }
                }
            }

        case .prompt:
            VStack(alignment: .leading, spacing: 14) {
                let pureFiltered = LaunchpadCatalog.pureChatPromptPool.filter { spec in
                    q.isEmpty || spec.titleZh.lowercased().contains(q) || spec.titleEn.lowercased().contains(q) || spec.subZh.lowercased().contains(q)
                }
                if !pureFiltered.isEmpty {
                    VStack(alignment: .leading, spacing: 8) {
                        Text(t3("纯对话场景模板", "Pure Chat Templates", "チャット専用テンプレート"))
                            .font(.system(size: 12, weight: .semibold))
                            .foregroundColor(.secondary)
                            .padding(.leading, 4)
                        ForEach(pureFiltered, id: \.id) { spec in
                            let item = LaunchpadShortcutItem(
                                kind: "prompt",
                                ref: spec.id,
                                title: t3(spec.titleZh, spec.titleEn, spec.titleJa),
                                subtitle: t3(spec.subZh, spec.subEn, spec.subJa),
                                svgKey: spec.svgKey,
                                tint: spec.tintName
                            )
                            candidateRow(
                                item: item,
                                svgKey: spec.svgKey,
                                tint: spec.tint,
                                title: t3(spec.titleZh, spec.titleEn, spec.titleJa),
                                subtitle: t3(spec.subZh, spec.subEn, spec.subJa),
                                badge: nil
                            )
                        }
                    }
                }

                let wsFiltered = LaunchpadCatalog.workspacePromptPool.filter { spec in
                    q.isEmpty || spec.titleZh.lowercased().contains(q) || spec.titleEn.lowercased().contains(q) || spec.subZh.lowercased().contains(q)
                }
                if !wsFiltered.isEmpty {
                    VStack(alignment: .leading, spacing: 8) {
                        Text(t3("代码工作区模板", "Workspace Templates", "ワークスペース用テンプレート"))
                            .font(.system(size: 12, weight: .semibold))
                            .foregroundColor(.secondary)
                            .padding(.leading, 4)
                        ForEach(wsFiltered, id: \.id) { spec in
                            let item = LaunchpadShortcutItem(
                                kind: "prompt",
                                ref: spec.id,
                                title: t3(spec.titleZh, spec.titleEn, spec.titleJa),
                                subtitle: t3(spec.subZh, spec.subEn, spec.subJa),
                                svgKey: spec.svgKey,
                                tint: spec.tintName
                            )
                            candidateRow(
                                item: item,
                                svgKey: spec.svgKey,
                                tint: spec.tint,
                                title: t3(spec.titleZh, spec.titleEn, spec.titleJa),
                                subtitle: t3(spec.subZh, spec.subEn, spec.subJa),
                                badge: nil
                            )
                        }
                    }
                }
            }
        }
    }

    private func candidateRow(
        item: LaunchpadShortcutItem,
        svgKey: String,
        tint: Color,
        title: String,
        subtitle: String,
        badge: String?
    ) -> some View {
        let pinned = shortcutStore.isPinned(kind: item.kind, ref: item.ref)
        return Button {
            toggleItem(item)
        } label: {
            HStack(spacing: 11) {
                ZStack {
                    RoundedRectangle(cornerRadius: 8, style: .continuous)
                        .fill(tint.opacity(0.14))
                        .frame(width: 30, height: 30)
                    LaunchpadSVGIconView(iconKey: svgKey, tint: tint, size: 15.5)
                }
                VStack(alignment: .leading, spacing: 2.5) {
                    HStack(spacing: 6) {
                        Text(title)
                            .font(.system(size: 14, weight: .semibold))
                            .foregroundColor(.primary)
                            .lineLimit(1)
                        if let badge {
                            Text(badge)
                                .font(.system(size: 10.5, weight: .medium))
                                .foregroundColor(LaunchpadCatalog.tintTeal)
                                .padding(.horizontal, 6)
                                .padding(.vertical, 1.5)
                                .background(LaunchpadCatalog.tintTeal.opacity(0.12))
                                .clipShape(Capsule())
                        }
                    }
                    Text(subtitle)
                        .font(.system(size: 12))
                        .foregroundColor(.secondary)
                        .lineLimit(1)
                }
                Spacer(minLength: 6)
                Image(systemName: pinned ? "checkmark.circle.fill" : "plus.circle")
                    .font(.system(size: 19))
                    .foregroundColor(pinned ? .indigo : Color(uiColor: .tertiaryLabel))
            }
            .padding(.horizontal, 12)
            .padding(.vertical, 10)
            .background(Color(uiColor: .secondarySystemGroupedBackground))
            .clipShape(RoundedRectangle(cornerRadius: 13, style: .continuous))
            .overlay(
                RoundedRectangle(cornerRadius: 13, style: .continuous)
                    .stroke(pinned ? Color.indigo.opacity(0.35) : Color.primary.opacity(0.06), lineWidth: 0.9)
            )
        }
        .buttonStyle(.plain)
    }
}
