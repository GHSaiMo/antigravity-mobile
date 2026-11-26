import SwiftUI
import UIKit

struct AIAssistSettingsView: View {
    @State private var manager = AIAssistManager.shared
    @State private var settings = AppSettings.shared
    @State private var showRestoreConfirm = false

    private var devLang: DeviceTitleTargetLanguage { manager.deviceLanguage }

    private var activeTargetEnglishName: String {
        if !devLang.isEnglish {
            return devLang.englishDisplayName
        }
        if let name = manager.prefs.titleTranslateLangName, !name.isEmpty {
            return name
        }
        if !manager.resolvedLangName.isEmpty {
            return manager.resolvedLangName
        }
        return "Local Language"
    }

    private var titleSectionHeader: String {
        switch devLang.kind {
        case .simplifiedChinese, .traditionalChinese:
            return "会话标题翻译"
        case .japanese:
            return "会話タイトルの翻訳"
        default:
            return "Conversation Title Translation"
        }
    }

    private var titleToggleLabel: String {
        switch devLang.kind {
        case .simplifiedChinese:
            return "自动翻译为简体中文"
        case .traditionalChinese:
            return "自动翻译为繁体中文"
        case .japanese:
            return "英語タイトルを日本語に自動翻訳"
        default:
            return "Auto-translate to \(activeTargetEnglishName)"
        }
    }

    private var titleToggleSubtitle: String {
        switch devLang.kind {
        case .simplifiedChinese, .traditionalChinese:
            return "Google 生成英文标题后自动翻译并同步到桌面与手机"
        case .japanese:
            return "Google 生成の英語タイトルを自動翻訳しデスクトップと同期"
        default:
            return "Translate English titles into \(activeTargetEnglishName) and sync across desktop & mobile"
        }
    }

    private var batchButtonLabel: String {
        switch devLang.kind {
        case .simplifiedChinese, .traditionalChinese:
            return "立即翻译历史英文标题"
        case .japanese:
            return "過去の英語タイトルを一括翻訳"
        default:
            return "Translate Existing Titles to \(activeTargetEnglishName)"
        }
    }

    private var restoreButtonLabel: String {
        switch devLang.kind {
        case .simplifiedChinese, .traditionalChinese:
            return "还原已翻译标题（\(manager.translatedCount)）"
        case .japanese:
            return "翻訳済みタイトルを元に戻す（\(manager.translatedCount)）"
        default:
            return "Restore Original Titles (\(manager.translatedCount))"
        }
    }

    private var titleSectionFooter: String {
        switch devLang.kind {
        case .simplifiedChinese, .traditionalChinese:
            return "仅翻译未归档活跃会话，不会覆盖手动重命名的标题；翻译结果直接写回 Antigravity，桌面端与手机端同步生效。"
        case .japanese:
            return "アーカイブ済みを除く会話のみ対象。手動変更したタイトルは上書きされず、デスクトップと同期されます。"
        default:
            return "Applies to active non-archived conversations only. Manually renamed titles are preserved."
        }
    }

    private var liveSectionHeader: String {
        switch devLang.kind {
        case .simplifiedChinese, .traditionalChinese:
            return "进度与状态翻译"
        case .japanese:
            return "進捗・ステータスの翻訳"
        default:
            return "Progress & Status Translation"
        }
    }

    private var stepPlanToggleLabel: String {
        switch devLang.kind {
        case .simplifiedChinese, .traditionalChinese:
            return "步骤清单实时翻译"
        case .japanese:
            return "ステップ一覧を自動翻訳"
        default:
            return "Translate Step Checklist"
        }
    }

    private var stepPlanToggleSubtitle: String {
        switch devLang.kind {
        case .simplifiedChinese:
            return "将英文待办步骤翻译为简体中文（含灵动岛）"
        case .traditionalChinese:
            return "将英文待办步骤翻译为繁体中文（含灵动岛）"
        case .japanese:
            return "英語のタスクリストを日本語に翻訳（Dynamic Island 含む）"
        default:
            return "Translate English checklist steps into \(activeTargetEnglishName) (incl. Live Activity)"
        }
    }

    private var focusToggleLabel: String {
        switch devLang.kind {
        case .simplifiedChinese, .traditionalChinese:
            return "实时状态行翻译"
        case .japanese:
            return "リアルタイム状況を自動翻訳"
        default:
            return "Translate Live Status Line"
        }
    }

    private var focusToggleSubtitle: String {
        switch devLang.kind {
        case .simplifiedChinese:
            return "将执行中的英文状态摘要翻译为简体中文（含灵动岛）"
        case .traditionalChinese:
            return "将执行中的英文状态摘要翻译为繁体中文（含灵动岛）"
        case .japanese:
            return "実行中の英語ステータスを日本語に翻訳（Dynamic Island 含む）"
        default:
            return "Translate live working status into \(activeTargetEnglishName) (incl. Live Activity)"
        }
    }

    private var liveSectionFooter: String {
        switch devLang.kind {
        case .simplifiedChinese, .traditionalChinese:
            return "仅在客户端与灵动岛展示译文，不修改桌面端原始记录；相同条目本地缓存复用，随时可独立开关。"
        case .japanese:
            return "クライアントと Dynamic Island の表示のみ翻訳され、デスクトップの元記録は変更されません。同じ項目はキャッシュで再利用されます。"
        default:
            return "Translates display on mobile, Web, and Live Activity only without modifying desktop history. Cached locally for reuse."
        }
    }

    var body: some View {
        Form {
            // MARK: - 1. 会话标题翻译 (英文区默认隐藏；若网关已开启则兜底展示以便关闭)
            if manager.shouldShowTitleTranslation {
                Section(
                    header: Text(titleSectionHeader),
                    footer: Text(titleSectionFooter)
                ) {
                    Toggle(isOn: Binding(
                        get: { manager.prefs.titleTranslateEnabled },
                        set: { newVal in
                            UIImpactFeedbackGenerator(style: .light).impactOccurred()
                            Task { await manager.setTitleTranslateEnabled(newVal) }
                        }
                    )) {
                        VStack(alignment: .leading, spacing: 2) {
                            Text(titleToggleLabel)
                            Text(titleToggleSubtitle)
                                .font(.caption)
                                .foregroundColor(.secondary)
                        }
                    }
                    .disabled(settings.isDemoMode || !manager.isAvailable)

                    if !devLang.isEnglish {
                        Button {
                            UIImpactFeedbackGenerator(style: .light).impactOccurred()
                            Task { await manager.translateExistingTitles() }
                        } label: {
                            HStack {
                                Text(batchButtonLabel)
                                    .foregroundColor((settings.isDemoMode || !manager.isAvailable || manager.isTranslatingBatch) ? .secondary : .indigo)
                                Spacer()
                                if manager.isTranslatingBatch {
                                    ProgressView()
                                        .scaleEffect(0.85)
                                }
                            }
                        }
                        .disabled(settings.isDemoMode || !manager.isAvailable || manager.isTranslatingBatch)
                    }

                    if manager.translatedCount > 0 {
                        Button(role: .destructive) {
                            showRestoreConfirm = true
                        } label: {
                            HStack {
                                Text(restoreButtonLabel)
                                Spacer()
                                if manager.isRestoringTitles {
                                    ProgressView()
                                        .scaleEffect(0.85)
                                }
                            }
                        }
                        .disabled(settings.isDemoMode || manager.isRestoringTitles || manager.isTranslatingBatch)
                    }

                    if let feedback = manager.batchFeedback, !feedback.isEmpty {
                        Text(feedback)
                            .font(.caption)
                            .foregroundColor(manager.batchFeedbackIsError ? .red : .secondary)
                    }
                }
            }

            // MARK: - 1.5 进度与状态翻译 (英文区默认隐藏；若网关已开启则兜底展示以便关闭)
            if manager.shouldShowLiveTranslation {
                Section(
                    header: Text(liveSectionHeader),
                    footer: Text(liveSectionFooter)
                ) {
                    Toggle(isOn: Binding(
                        get: { manager.prefs.stepPlanTranslateEnabled },
                        set: { newVal in
                            UIImpactFeedbackGenerator(style: .light).impactOccurred()
                            Task { await manager.setStepPlanTranslateEnabled(newVal) }
                        }
                    )) {
                        VStack(alignment: .leading, spacing: 2) {
                            Text(stepPlanToggleLabel)
                            Text(stepPlanToggleSubtitle)
                                .font(.caption)
                                .foregroundColor(.secondary)
                        }
                    }
                    .disabled(settings.isDemoMode || !manager.isAvailable)

                    Toggle(isOn: Binding(
                        get: { manager.prefs.focusTranslateEnabled },
                        set: { newVal in
                            UIImpactFeedbackGenerator(style: .light).impactOccurred()
                            Task { await manager.setFocusTranslateEnabled(newVal) }
                        }
                    )) {
                        VStack(alignment: .leading, spacing: 2) {
                            Text(focusToggleLabel)
                            Text(focusToggleSubtitle)
                                .font(.caption)
                                .foregroundColor(.secondary)
                        }
                    }
                    .disabled(settings.isDemoMode || !manager.isAvailable)
                }
            }

            // MARK: - 2. 下一步提示词建议 (全语种可见)
            Section(
                header: Text("提示词建议"),
                footer: Text("存在审批卡片、Proceed/Continue 按钮或输入框已有草稿时自动隐藏，避免打扰。")
            ) {
                Toggle(isOn: Binding(
                    get: { manager.prefs.nextPromptEnabled },
                    set: { newVal in
                        UIImpactFeedbackGenerator(style: .light).impactOccurred()
                        Task { await manager.setNextPromptEnabled(newVal) }
                    }
                )) {
                    VStack(alignment: .leading, spacing: 2) {
                        Text("下一步提示词建议")
                        Text("每轮回复完成后轻量预测 1 条建议胶囊")
                            .font(.caption)
                            .foregroundColor(.secondary)
                    }
                }
                .disabled(settings.isDemoMode || !manager.isAvailable)

                if manager.prefs.nextPromptEnabled {
                    Picker("点击建议胶囊", selection: Binding(
                        get: { manager.prefs.nextPromptTapAction },
                        set: { newVal in
                            UIImpactFeedbackGenerator(style: .light).impactOccurred()
                            Task { await manager.setNextPromptTapAction(newVal) }
                        }
                    )) {
                        Text("填入输入框").tag("fill")
                        Text("直接发送").tag("send")
                    }
                    .disabled(settings.isDemoMode)
                }
            }

            // MARK: - 3. 底层引擎信息
            Section(
                header: Text("推理引擎"),
                footer: Text(
                    manager.syncError
                        ?? (settings.isDemoMode
                            ? String(localized: "演示模式下仅预览开关状态，配对网关后实时生效。")
                            : (!manager.isAvailable
                                ? String(localized: "当前 Antigravity 版本不支持原生 GetModelResponse 接口。")
                                : String(localized: "通过本机 Antigravity 原生接口无状态执行，不产生额外会话，微量共享当前账号 Gemini 额度，开关跨设备同步至网关。")))
                )
            ) {
                HStack {
                    Text("轻量模型")
                    Spacer()
                    Text(manager.modelName)
                        .font(.subheadline)
                        .foregroundColor(.secondary)
                }
            }
        }
        .navigationTitle("智能辅助")
        .navigationBarTitleDisplayMode(.inline)
        .task {
            await manager.fetchStatus()
        }
        .alert(String(localized: "还原原始标题"), isPresented: $showRestoreConfirm) {
            Button(String(localized: "取消"), role: .cancel) {}
            Button(String(localized: "还原"), role: .destructive) {
                Task { await manager.restoreOriginalTitles() }
            }
        } message: {
            Text("将把此前自动或批量翻译的会话标题恢复为原始标题（手动重命名的会话不受影响）。")
        }
    }
}
