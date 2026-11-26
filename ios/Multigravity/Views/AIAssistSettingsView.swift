import SwiftUI
import UIKit

struct AIAssistSettingsView: View {
    @State private var manager = AIAssistManager.shared
    @State private var settings = AppSettings.shared

    private var devLang: DeviceTitleTargetLanguage { manager.deviceLanguage }

    private var translationSectionHeader: String {
        switch devLang.kind {
        case .simplifiedChinese, .traditionalChinese:
            return "智能翻译"
        case .japanese:
            return "スマート翻訳"
        default:
            return "Smart Translation"
        }
    }

    private var titleToggleLabel: String {
        switch devLang.kind {
        case .simplifiedChinese, .traditionalChinese:
            return "会话标题翻译"
        case .japanese:
            return "会話タイトルの翻訳"
        default:
            return "Conversation Title Translation"
        }
    }

    private var titleToggleSubtitle: String {
        switch devLang.kind {
        case .simplifiedChinese:
            return "新会话生成英文标题后自动翻译为简体中文并覆盖"
        case .traditionalChinese:
            return "新会话生成英文标题后自动翻译为繁体中文并覆盖"
        case .japanese:
            return "新しい会話の英語タイトルを日本語に自動翻訳して上書き"
        default:
            return "Auto-translate new English titles into \(devLang.englishDisplayName)"
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
            return "Translate English checklist steps into \(devLang.englishDisplayName) (incl. Live Activity)"
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
            return "Translate live working status into \(devLang.englishDisplayName) (incl. Live Activity)"
        }
    }

    private var translationSectionFooter: String {
        switch devLang.kind {
        case .simplifiedChinese, .traditionalChinese:
            return "标题翻译仅覆盖开启后的新会话，不改动已有会话；步骤与状态仅在客户端与灵动岛展示译文，不修改桌面端原始记录。"
        case .japanese:
            return "タイトル翻訳はオン後の新しい会話のみ上書き同期されます（既存会話は変更しません）。ステップ・状況は表示のみ翻訳され、元の記録は保持されます。"
        default:
            return "Title translation overwrites new conversations going forward without modifying existing ones. Step and status translations are display-only."
        }
    }

    var body: some View {
        Form {
            // MARK: - 1. 智能翻译 (仅非英文用户可见，统一收拢三个翻译开关)
            if manager.shouldShowSmartTranslation {
                Section(
                    header: Text(translationSectionHeader),
                    footer: Text(translationSectionFooter)
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
    }
}
