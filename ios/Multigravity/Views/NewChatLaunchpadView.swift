import SwiftUI

/// 本地轻量斜杠调用频次记录（作为网关全端历史统计的即时叠加与离线兜底）。
public enum SlashUsageLocalStore {
    private static let key = "mgy_local_slash_usage_v1"

    public static func counts() -> [String: Int] {
        UserDefaults.standard.dictionary(forKey: key) as? [String: Int] ?? [:]
    }

    public static func record(_ name: String) {
        let clean = name.trimmingCharacters(in: .whitespacesAndNewlines)
            .replacingOccurrences(of: "^/+", with: "", options: .regularExpression)
            .lowercased()
        guard !clean.isEmpty else { return }
        var dict = counts()
        dict[clean, default: 0] += 1
        UserDefaults.standard.set(dict, forKey: key)
    }
}

/// 启动台专属 24×24 矢量 SVG 图标目录、调色板与双池常量（与 Android / Web 端对齐；严禁洋红色系）。
enum LaunchpadCatalog {
    // 严禁洋红：全系采用 Indigo / Blue / Teal / Cyan / Amber / Emerald / Coral
    static let tintIndigo = Color.indigo
    static let tintBlue = Color(red: 0.231, green: 0.510, blue: 0.965)     // #3B82F6
    static let tintTeal = Color(red: 0.078, green: 0.722, blue: 0.651)     // #14B8A6
    static let tintCyan = Color(red: 0.024, green: 0.714, blue: 0.831)     // #06B6D4
    static let tintAmber = Color(red: 0.961, green: 0.620, blue: 0.043)    // #F59E0B
    static let tintEmerald = Color(red: 0.133, green: 0.773, blue: 0.369)  // #22C55E
    static let tintCoral = Color(red: 0.937, green: 0.267, blue: 0.267)    // #EF4444

    static func tintColor(_ name: String) -> Color {
        switch name.lowercased() {
        case "indigo": return tintIndigo
        case "blue": return tintBlue
        case "teal": return tintTeal
        case "cyan": return tintCyan
        case "amber", "orange": return tintAmber
        case "emerald", "green": return tintEmerald
        case "coral", "red": return tintCoral
        default: return tintIndigo
        }
    }

    static func workflowTint(_ colorKey: String) -> Color {
        // 与 WorkflowTemplateStyle 原色对齐；严禁洋红，若旧模板保存 "purple" 则映射为靛蓝
        switch colorKey.lowercased() {
        case "green", "emerald": return .green
        case "blue": return .blue
        case "orange", "amber": return .orange
        case "red", "coral": return .red
        case "teal": return .teal
        case "cyan": return tintCyan
        case "indigo", "purple": return .indigo
        default: return .indigo
        }
    }

    static func safeTemplateTint(_ colorKey: String) -> Color {
        workflowTint(colorKey)
    }

    static func safeTintName(_ colorKey: String) -> String {
        switch colorKey.lowercased() {
        case "indigo", "purple": return "indigo"
        case "green", "emerald": return "green"
        case "blue": return "blue"
        case "teal": return "teal"
        case "cyan": return "cyan"
        case "red", "coral": return "red"
        case "orange", "amber": return "orange"
        default: return "indigo"
        }
    }

    static let midChatExcludedSlashes: Set<String> = ["compact", "btw", "schedule", "browser"]

    struct CoreSlashSpec {
        let name: String
        let svgKey: String
        let tintName: String
        let tint: Color
        let titleZh: String
        let titleEn: String
        let titleJa: String
        let subZh: String
        let subEn: String
        let subJa: String
        let fallbackIcon: String
    }

    struct PromptSpec {
        let id: String
        let svgKey: String
        let tintName: String
        let tint: Color
        let titleZh: String
        let titleEn: String
        let titleJa: String
        let subZh: String
        let subEn: String
        let subJa: String
        let promptZh: String
        let promptEn: String
        let promptJa: String
        let requiresDirtyFiles: Bool
    }

    static let coreSlashPool: [CoreSlashSpec] = [
        CoreSlashSpec(
            name: "plan", svgKey: "slash_plan", tintName: "blue", tint: tintBlue,
            titleZh: "/plan 方案规划", titleEn: "/plan Architect", titleJa: "/plan 実装計画",
            subZh: "先出步骤与风险，确认再动手", subEn: "Align steps & risks before editing", subJa: "手順とリスクを固めてから着手",
            fallbackIcon: "ballot"
        ),
        CoreSlashSpec(
            name: "grill-me", svgKey: "slash_grill", tintName: "indigo", tint: tintIndigo,
            titleZh: "/grill-me 追问对齐", titleEn: "/grill-me Clarify", titleJa: "/grill-me 深掘り",
            subZh: "逐轮反向提问，拆清模糊想法", subEn: "Interactive Q&A to sharpen ideas", subJa: "逆質問で曖昧な要件を明確化",
            fallbackIcon: "forum"
        ),
        CoreSlashSpec(
            name: "boost", svgKey: "slash_boost", tintName: "amber", tint: tintAmber,
            titleZh: "/boost 深度攻坚", titleEn: "/boost Deep Think", titleJa: "/boost 徹底推論",
            subZh: "多视角深度推理与严格验证", subEn: "Deep multi-angle reasoning & checks", subJa: "多角的な深い推論と厳格検証",
            fallbackIcon: "bolt"
        ),
        CoreSlashSpec(
            name: "goal", svgKey: "slash_goal", tintName: "emerald", tint: tintEmerald,
            titleZh: "/goal 自律闭环", titleEn: "/goal Autonomous", titleJa: "/goal 自律達成",
            subZh: "持续迭代自检，达标才收工", subEn: "Iterate & self-verify until done", subJa: "目標達成まで自律検証を継続",
            fallbackIcon: "timer"
        ),
        CoreSlashSpec(
            name: "teamwork", svgKey: "slash_teamwork", tintName: "cyan", tint: tintCyan,
            titleZh: "/teamwork 团队协作", titleEn: "/teamwork Agents", titleJa: "/teamwork 連携",
            subZh: "多角色子 Agent 分工推进大任务", subEn: "Coordinate specialist subagents", subJa: "複数エージェントで分担推進",
            fallbackIcon: "graph_2"
        ),
        CoreSlashSpec(
            name: "swarm", svgKey: "slash_swarm", tintName: "amber", tint: tintAmber,
            titleZh: "/swarm 蜂群并发", titleEn: "/swarm Parallel", titleJa: "/swarm 並列群",
            subZh: "共享消息板多 Agent 协同攻坚", subEn: "Shared board for parallel workers", subJa: "共有ボードで並列エージェント実行",
            fallbackIcon: "swarm_bee"
        ),
        CoreSlashSpec(
            name: "workflow", svgKey: "slash_workflow", tintName: "teal", tint: tintTeal,
            titleZh: "/workflow 编排流程", titleEn: "/workflow Pipeline", titleJa: "/workflow 編成",
            subZh: "并行或流水线批量执行多阶段任务", subEn: "Orchestrate staged agent pipelines", subJa: "多段階ワークフローを一括編成",
            fallbackIcon: "graph_2"
        ),
        CoreSlashSpec(
            name: "learn", svgKey: "slash_learn", tintName: "amber", tint: tintAmber,
            titleZh: "/learn 沉淀经验", titleEn: "/learn Persist", titleJa: "/learn 知見定着",
            subZh: "将踩坑与规范固化，后续自动生效", subEn: "Save lessons & rules for future chats", subJa: "学びやルールを今後の会話に定着",
            fallbackIcon: "lightbulb_2"
        ),
        CoreSlashSpec(
            name: "automation", svgKey: "slash_automation", tintName: "blue", tint: tintBlue,
            titleZh: "/automation 定时任务", titleEn: "/automation Cron", titleJa: "/automation 定期",
            subZh: "创建周期巡检或定时提醒任务", subEn: "Create recurring background schedules", subJa: "定期巡回やリマインドを自動化",
            fallbackIcon: "schedule"
        )
    ]

    static let pureChatPromptPool: [PromptSpec] = [
        PromptSpec(
            id: "pure_fact_check", svgKey: "prompt_fact_check", tintName: "blue", tint: tintBlue,
            titleZh: "联网核查资讯", titleEn: "Web Fact-Check", titleJa: "最新情報を検証",
            subZh: "交叉验证最新进展与来源", subEn: "Verify latest updates & sources", subJa: "信頼できる情報源で事実確認",
            promptZh: "请帮我联网核查以下消息的最新进展，并附上权威来源交叉印证：",
            promptEn: "Please verify the latest updates and cross-check reliable sources for: ",
            promptJa: "以下の件の最新状況と信頼できる情報源を突き合わせて確認してください：",
            requiresDirtyFiles: false
        ),
        PromptSpec(
            id: "pure_draft", svgKey: "prompt_draft", tintName: "teal", tint: tintTeal,
            titleZh: "起草与润色", titleEn: "Draft & Polish", titleJa: "下書き・推敲",
            subZh: "文章 / 公告 / 邮件 / 提纲", subEn: "Articles, notes, emails & outlines", subJa: "記事・お知らせ・メール・構成案",
            promptZh: "帮我起草并润色一段清晰自然的内容，主题是：",
            promptEn: "Help me draft and polish a clear, natural piece about: ",
            promptJa: "次のテーマについて自然で分かりやすい文章を作成・推敲してください：",
            requiresDirtyFiles: false
        ),
        PromptSpec(
            id: "pure_compare", svgKey: "prompt_compare", tintName: "amber", tint: tintAmber,
            titleZh: "多维方案对比", titleEn: "Compare Options", titleJa: "多角的に比較",
            subZh: "权衡利弊、成本与风险", subEn: "Weigh pros, cons, cost & risk", subJa: "利点・コスト・リスクを比較整理",
            promptZh: "请从利弊、成本与风险角度对比以下方案，并给出建议：",
            promptEn: "Please compare these options across pros, cons, cost, and risk, then recommend one: ",
            promptJa: "以下の案をメリット・コスト・リスクの観点で比較し、推奨案を示してください：",
            requiresDirtyFiles: false
        ),
        PromptSpec(
            id: "pure_digest", svgKey: "prompt_digest", tintName: "indigo", tint: tintIndigo,
            titleZh: "长文精读提炼", titleEn: "Distill Long Text", titleJa: "長文の要点抽出",
            subZh: "抓核心结论与关键数据", subEn: "Extract key takeaways & figures", subJa: "核心結論と重要データを整理",
            promptZh: "请帮我精读下面内容，提炼核心结论、关键数据与潜在漏洞：",
            promptEn: "Please read the following closely and extract key conclusions, data, and blind spots: ",
            promptJa: "以下の内容を精読し、核心結論・主要データ・見落としを抽出してください：",
            requiresDirtyFiles: false
        ),
        PromptSpec(
            id: "pure_research", svgKey: "prompt_research", tintName: "cyan", tint: tintCyan,
            titleZh: "深度专题调研", titleEn: "Topic Deep Dive", titleJa: "テーマ徹底調査",
            subZh: "梳理来龙去脉与现状趋势", subEn: "Map background, status & trends", subJa: "背景・現状・今後の動向を整理",
            promptZh: "请帮我做一次系统的专题调研，梳理背景、现状与关键趋势：",
            promptEn: "Please run a structured deep dive covering background, current state, and key trends on: ",
            promptJa: "次のテーマについて背景・現状・主要トレンドを体系的に調査してください：",
            requiresDirtyFiles: false
        ),
        PromptSpec(
            id: "pure_brainstorm", svgKey: "prompt_brainstorm", tintName: "emerald", tint: tintEmerald,
            titleZh: "拆解模糊想法", titleEn: "Shape Rough Ideas", titleJa: "アイデア具体化",
            subZh: "把灵感变成可执行清单", subEn: "Turn sparks into action checklists", subJa: "着想を実行可能な手順へ分解",
            promptZh: "我有一个初步想法，请帮我拆解关键问题并整理成行动清单：",
            promptEn: "I have a rough idea—help me break down the key questions into an actionable checklist: ",
            promptJa: "まだ曖昧なアイデアがあります。論点を分解し実行リストに整理してください：",
            requiresDirtyFiles: false
        ),
        PromptSpec(
            id: "pure_translate", svgKey: "prompt_translate", tintName: "blue", tint: tintBlue,
            titleZh: "地道翻译润色", titleEn: "Natural Translation", titleJa: "自然な翻訳推敲",
            subZh: "保留原意与语境的自然表达", subEn: "Idiomatic phrasing with exact nuance", subJa: "ニュアンスを保った自然な表現",
            promptZh: "请将下面内容地道地翻译并润色，保持自然专业语气：",
            promptEn: "Please translate and polish the following naturally while preserving nuance and tone: ",
            promptJa: "以下の内容を元の意図と専門的なトーンを保って自然に翻訳・推敲してください：",
            requiresDirtyFiles: false
        ),
        PromptSpec(
            id: "pure_notes", svgKey: "prompt_notes", tintName: "teal", tint: tintTeal,
            titleZh: "零散记录整理", titleEn: "Organize Notes", titleJa: "メモを構造化",
            subZh: "转成结构化纪要与待办", subEn: "Structure minutes, decisions & TODOs", subJa: "議事録・決定事項・TODO化",
            promptZh: "请把以下零散记录整理成结构化纪要，列出结论与下一步待办：",
            promptEn: "Please turn these rough notes into structured minutes with decisions and next actions: ",
            promptJa: "以下の断片メモを構造化し、結論と次のアクション一覧にまとめてください：",
            requiresDirtyFiles: false
        )
    ]

    static let workspacePromptPool: [PromptSpec] = [
        PromptSpec(
            id: "ws_git_diff", svgKey: "prompt_git_diff", tintName: "amber", tint: tintAmber,
            titleZh: "审查未提交改动", titleEn: "Review Dirty Diff", titleJa: "未コミット変更確認",
            subZh: "检查 git diff 风险与优化", subEn: "Inspect git diff risks & fixes", subJa: "git diff のリスクと改善確認",
            promptZh: "请先检查当前未提交的改动，指出潜在风险并给出优化建议",
            promptEn: "Please review the current uncommitted changes, highlight potential risks, and suggest improvements",
            promptJa: "現在の未コミット変更を確認し、潜在リスクと改善案を教えてください",
            requiresDirtyFiles: true
        ),
        PromptSpec(
            id: "ws_arch", svgKey: "prompt_arch", tintName: "indigo", tint: tintIndigo,
            titleZh: "梳理项目架构", titleEn: "Map Architecture", titleJa: "構成を把握",
            subZh: "快速定位核心模块与入口", subEn: "Find core modules & entry points", subJa: "主要モジュールと入口を整理",
            promptZh: "请先梳理这个工作区的整体架构、核心模块分工与关键入口文件",
            promptEn: "Please map out the overall architecture, core modules, and key entry points of this workspace",
            promptJa: "このワークスペースの全体構造、主要モジュール、エントリポイントを整理してください",
            requiresDirtyFiles: false
        ),
        PromptSpec(
            id: "ws_bug", svgKey: "prompt_bug", tintName: "coral", tint: tintCoral,
            titleZh: "定位排查 Bug", titleEn: "Debug Root Cause", titleJa: "バグ原因調査",
            subZh: "结合代码与日志追查根因", subEn: "Trace root cause from code & logs", subJa: "コードとログから根本原因特定",
            promptZh: "请帮我排查并定位以下问题的根因，先分析再给修复方案：",
            promptEn: "Please investigate and find the root cause of this issue before proposing a fix: ",
            promptJa: "以下の問題の根本原因を分析し、修正案を提示してください：",
            requiresDirtyFiles: false
        ),
        PromptSpec(
            id: "ws_refactor", svgKey: "prompt_refactor", tintName: "teal", tint: tintTeal,
            titleZh: "代码优化重构", titleEn: "Refactor & Polish", titleJa: "最適化・改善",
            subZh: "提升可读性、性能与稳定性", subEn: "Improve clarity, speed & stability", subJa: "可読性・性能・安定性を改善",
            promptZh: "请分析当前模块可优化的设计点，给出轻量低侵入的重构方案：",
            promptEn: "Please identify improvements in the current module and propose a low-impact refactor: ",
            promptJa: "現在のモジュールの改善点を分析し、低侵襲なリファクタ案を提案してください：",
            requiresDirtyFiles: false
        ),
        PromptSpec(
            id: "ws_tests", svgKey: "prompt_tests", tintName: "emerald", tint: tintEmerald,
            titleZh: "补全单元测试", titleEn: "Add Unit Tests", titleJa: "単体テスト追加",
            subZh: "覆盖核心路径与边界条件", subEn: "Cover core flows & edge cases", subJa: "主要経路と境界ケースを網羅",
            promptZh: "请为以下模块补充单元测试，重点覆盖异常分支与边界情况：",
            promptEn: "Please add high-value unit tests focusing on edge cases and error paths for: ",
            promptJa: "以下のモジュールに異常系・境界条件を重視した単体テストを追加してください：",
            requiresDirtyFiles: false
        ),
        PromptSpec(
            id: "ws_git_log", svgKey: "prompt_git_log", tintName: "cyan", tint: tintCyan,
            titleZh: "速览近期提交", titleEn: "Summarize Commits", titleJa: "直近コミット要約",
            subZh: "总结最近变更与影响范围", subEn: "Recap recent git log & blast radius", subJa: "最近の変更点と影響範囲を把握",
            promptZh: "请结合最近的 git log 梳理近期核心改动，并评估影响范围：",
            promptEn: "Please summarize recent key commits from git log and assess their impact area: ",
            promptJa: "最近の git log から主な変更点を要約し、影響範囲を評価してください：",
            requiresDirtyFiles: false
        ),
        PromptSpec(
            id: "ws_security", svgKey: "prompt_security", tintName: "amber", tint: tintAmber,
            titleZh: "安全隐患体检", titleEn: "Security Checkup", titleJa: "セキュリティ点検",
            subZh: "排查权限、输入与异常兜底", subEn: "Audit auth, inputs & error handling", subJa: "権限・入力検証・例外処理を確認",
            promptZh: "请对当前工作区关键链路做一次轻量安全与健壮性检查：",
            promptEn: "Please perform a lightweight security and resilience audit on the critical paths here: ",
            promptJa: "このワークスペースの重要経路について軽量なセキュリティ・堅牢性チェックを行ってください：",
            requiresDirtyFiles: false
        ),
        PromptSpec(
            id: "ws_changelog", svgKey: "prompt_changelog", tintName: "blue", tint: tintBlue,
            titleZh: "生成变更说明", titleEn: "Draft Changelog", titleJa: "変更履歴を作成",
            subZh: "整理发版日志或 PR 描述", subEn: "Write release notes or PR summary", subJa: "リリースノートや PR 概要を整形",
            promptZh: "请基于当前分支近期改动，整理一份简洁清晰的变更说明：",
            promptEn: "Please draft clear, concise release notes or a PR summary from recent branch changes: ",
            promptJa: "現在のブランチの直近変更から簡潔な変更履歴・PR説明を作成してください：",
            requiresDirtyFiles: false
        )
    ]

    static var allPromptPool: [PromptSpec] {
        pureChatPromptPool + workspacePromptPool
    }

    static let svgPaths: [String: String] = [
        // 9 大系统核心斜杠命令
        "slash_plan": "M 8 4.5 L 16 4.5 C 17.1 4.5 18 5.4 18 6.5 L 18 19 C 18 20.1 17.1 21 16 21 L 8 21 C 6.9 21 6 20.1 6 19 L 6 6.5 C 6 5.4 6.9 4.5 8 4.5 Z M 9.5 3 L 14.5 3 L 14.5 6 L 9.5 6 Z M 9 11 L 10.6 12.6 L 13.5 9.7 M 9 16 L 15 16",
        "slash_grill": "M 4 6.5 C 4 5.1 5.1 4 6.5 4 L 14.5 4 C 15.9 4 17 5.1 17 6.5 L 17 11.5 C 17 12.9 15.9 14 14.5 14 L 9 14 L 5.5 16.8 L 5.5 14 C 4.6 13.5 4 12.6 4 11.5 Z M 17 9.5 L 18 9.5 C 19.1 9.5 20 10.4 20 11.5 L 20 16 C 20 17.1 19.1 18 18 18 L 17.5 18 L 17.5 20.5 L 14.3 18 L 11 18",
        "slash_boost": "M 13.2 2.5 L 5.2 13 L 11.6 13 L 10.4 21.5 L 18.8 11 L 12.4 11 Z",
        "slash_goal": "M 12 3.5 C 16.7 3.5 20.5 7.3 20.5 12 C 20.5 16.7 16.7 20.5 12 20.5 C 7.3 20.5 3.5 16.7 3.5 12 C 3.5 7.3 7.3 3.5 12 3.5 Z M 12 7.5 C 14.5 7.5 16.5 9.5 16.5 12 C 16.5 14.5 14.5 16.5 12 16.5 C 9.5 16.5 7.5 14.5 7.5 12 C 7.5 9.5 9.5 7.5 12 7.5 Z M 12 10.8 C 12.7 10.8 13.2 11.3 13.2 12 C 13.2 12.7 12.7 13.2 12 13.2 C 11.3 13.2 10.8 12.7 10.8 12 C 10.8 11.3 11.3 10.8 12 10.8 Z",
        "slash_teamwork": "M 12 3.5 C 13.6 3.5 14.8 4.7 14.8 6.3 C 14.8 7.9 13.6 9.1 12 9.1 C 10.4 9.1 9.2 7.9 9.2 6.3 C 9.2 4.7 10.4 3.5 12 3.5 Z M 5.8 15 C 7.2 15 8.3 16.1 8.3 17.5 C 8.3 18.9 7.2 20 5.8 20 C 4.4 20 3.3 18.9 3.3 17.5 C 3.3 16.1 4.4 15 5.8 15 Z M 18.2 15 C 19.6 15 20.7 16.1 20.7 17.5 C 20.7 18.9 19.6 20 18.2 20 C 16.8 20 15.7 18.9 15.7 17.5 C 15.7 16.1 16.8 15 18.2 15 Z M 10.4 8.8 L 7.2 15.2 M 13.6 8.8 L 16.8 15.2 M 8.3 17.5 L 15.7 17.5",
        "slash_swarm": "M 12 3 L 16.2 5.4 L 16.2 10.2 L 12 12.6 L 7.8 10.2 L 7.8 5.4 Z M 7.8 10.2 L 12 12.6 L 12 17.4 L 7.8 19.8 L 3.6 17.4 L 3.6 12.6 Z M 16.2 10.2 L 20.4 12.6 L 20.4 17.4 L 16.2 19.8 L 12 17.4 L 12 12.6 Z",
        "slash_workflow": "M 4 5.5 C 4 4.7 4.7 4 5.5 4 L 9.5 4 C 10.3 4 11 4.7 11 5.5 L 11 8.5 C 11 9.3 10.3 10 9.5 10 L 5.5 10 C 4.7 10 4 9.3 4 8.5 Z M 13 15.5 C 13 14.7 13.7 14 14.5 14 L 18.5 14 C 19.3 14 20 14.7 20 15.5 L 20 18.5 C 20 19.3 19.3 20 18.5 20 L 14.5 20 C 13.7 20 13 19.3 13 18.5 Z M 11 7 L 15.5 7 C 16.3 7 16.5 7.5 16.5 8.5 L 16.5 14 M 7.5 10 L 7.5 15.5 C 7.5 16.5 8.2 17 9 17 L 13 17",
        "slash_learn": "M 9 17.5 L 15 17.5 M 9.8 20.5 L 14.2 20.5 M 9.5 14.5 C 7.2 13.2 6 11 6 8.8 C 6 5.5 8.7 3 12 3 C 15.3 3 18 5.5 18 8.8 C 18 11 16.8 13.2 14.5 14.5 L 14.5 15.5 L 9.5 15.5 Z",
        "slash_automation": "M 12 4 C 16.4 4 20 7.6 20 12 C 20 16.4 16.4 20 12 20 C 7.6 20 4 16.4 4 12 C 4 9.4 5.2 7.1 7.2 5.6 M 4.2 5.2 L 7.6 5.2 L 7.6 8.6 M 12 7.5 L 12 12 L 15.2 13.8",
        // 技能专属图标：模块化拼图徽章（替代旧 </> 代码括号）
        "slash_skill": "M 6.2 7.2 L 9.2 7.2 C 7.6 3.4 14.0 3.4 12.4 7.2 L 15.4 7.2 C 16.2 7.2 16.8 7.8 16.8 8.6 L 16.8 11.6 C 20.6 10.0 20.6 16.4 16.8 14.8 L 16.8 17.8 C 16.8 18.6 16.2 19.2 15.4 19.2 L 6.2 19.2 C 5.4 19.2 4.8 18.6 4.8 17.8 L 4.8 8.6 C 4.8 7.8 5.4 7.2 6.2 7.2 Z",
        "custom_pin": "M 9 4 L 15 4 L 14.2 10 L 17.2 12.8 L 17.2 14.5 L 6.8 14.5 L 6.8 12.8 L 9.8 10 Z M 12 14.5 L 12 20.5",
        "shuffle_refresh": "M 4 7.5 L 8.2 7.5 C 10.2 7.5 11.4 9.2 12.4 11.2 L 13.2 12.8 C 14.2 14.8 15.4 16.5 17.4 16.5 L 20 16.5 M 17.5 14 L 20 16.5 L 17.5 19 M 4 16.5 L 8.2 16.5 C 9.6 16.5 10.6 15.6 11.4 14.3 M 13.8 9.7 C 14.6 8.4 15.6 7.5 17.4 7.5 L 20 7.5 M 17.5 5 L 20 7.5 L 17.5 10",

        // 8 项纯对话场景指令
        "prompt_fact_check": "M 12 3.5 C 16.7 3.5 20.5 7.3 20.5 12 C 20.5 16.7 16.7 20.5 12 20.5 C 7.3 20.5 3.5 16.7 3.5 12 C 3.5 7.3 7.3 3.5 12 3.5 Z M 3.8 12 L 20.2 12 M 12 3.5 C 14.4 6 15.6 8.8 15.6 12 C 15.6 15.2 14.4 18 12 20.5 C 9.6 18 8.4 15.2 8.4 12 C 8.4 8.8 9.6 6 12 3.5 Z",
        "prompt_draft": "M 15.8 4.2 L 19.8 8.2 L 9.2 18.8 L 4.5 19.5 L 5.2 14.8 Z M 13.4 6.6 L 17.4 10.6 M 13.5 19.5 L 20 19.5",
        "prompt_compare": "M 12 4 L 12 19.5 M 8 19.5 L 16 19.5 M 5 7.5 L 19 7.5 M 6.2 7.5 L 3.8 13.2 C 4.5 14.4 7.9 14.4 8.6 13.2 Z M 17.8 7.5 L 15.4 13.2 C 16.1 14.4 19.5 14.4 20.2 13.2 Z",
        "prompt_digest": "M 5.5 3.5 L 14.5 3.5 L 18.5 7.5 L 18.5 20.5 L 5.5 20.5 Z M 14.5 3.5 L 14.5 7.5 L 18.5 7.5 M 8.5 11.5 L 15.5 11.5 M 8.5 15 L 15.5 15 M 8.5 18 L 12.2 18",
        "prompt_research": "M 12 3.5 C 16.7 3.5 20.5 7.3 20.5 12 C 20.5 16.7 16.7 20.5 12 20.5 C 7.3 20.5 3.5 16.7 3.5 12 C 3.5 7.3 7.3 3.5 12 3.5 Z M 15.6 8.4 L 13.6 13.6 L 8.4 15.6 L 10.4 10.4 Z",
        "prompt_brainstorm": "M 4.5 6.5 L 6.3 8.3 L 9.5 5.1 M 12 7 L 19.5 7 M 4.5 12 L 6.3 13.8 L 9.5 10.6 M 12 12.5 L 19.5 12.5 M 4.5 17.5 L 6.3 19.3 L 9.5 16.1 M 12 18 L 17 18",
        "prompt_translate": "M 4 6.5 L 11.5 6.5 M 7.8 4.2 L 7.8 6.5 M 9.8 6.5 C 9.2 9.8 7.2 12.4 4.5 14 M 6.2 9.2 C 7.5 11 9.3 12.6 11.2 13.6 M 13.2 19.5 L 16.4 11 L 19.6 19.5 M 14.3 16.8 L 18.5 16.8",
        "prompt_notes": "M 4.5 5.5 L 19.5 5.5 L 19.5 9.5 L 4.5 9.5 Z M 6.5 9.5 L 6.5 18.5 C 6.5 19.3 7.2 20 8 20 L 16 20 C 16.8 20 17.5 19.3 17.5 18.5 L 17.5 9.5 M 9.5 13.5 L 14.5 13.5 M 9.5 16.5 L 13 16.5",

        // 8 项代码工作区场景指令
        "prompt_git_diff": "M 10.5 4.2 C 14 4.2 16.8 7 16.8 10.5 C 16.8 14 14 16.8 10.5 16.8 C 7 16.8 4.2 14 4.2 10.5 C 4.2 7 7 4.2 10.5 4.2 Z M 15.2 15.2 L 20 20 M 8.3 10.5 L 12.7 10.5 M 10.5 8.3 L 10.5 12.7",
        "prompt_arch": "M 12 3.8 L 20 8 L 12 12.2 L 4 8 Z M 4 12.2 L 12 16.4 L 20 12.2 M 4 16.2 L 12 20.4 L 20 16.2",
        "prompt_bug": "M 9 7.5 L 9 6.5 C 9 4.8 10.3 3.5 12 3.5 C 13.7 3.5 15 4.8 15 6.5 L 15 7.5 M 8.5 7.5 L 15.5 7.5 C 16.9 7.5 18 8.6 18 10 L 18 14 C 18 17.3 15.3 20 12 20 C 8.7 20 6 17.3 6 14 L 6 10 C 6 8.6 7.1 7.5 8.5 7.5 Z M 12 10.8 L 12 20 M 6 10.5 L 3 9 M 6 14 L 2.8 14 M 18 10.5 L 21 9 M 18 14 L 21.2 14",
        "prompt_refactor": "M 4.5 19.5 L 15.5 8.5 M 14.2 6.8 L 17.2 9.8 M 12 3.2 L 12 5.5 M 18.5 12 L 20.8 12 M 17.5 4.5 L 19 3 M 18.5 6.5 L 19.8 5.2 M 7 5 L 7.7 7.1 L 9.8 7.8 L 7.7 8.5 L 7 10.6 L 6.3 8.5 L 4.2 7.8 L 6.3 7.1 Z",
        "prompt_tests": "M 9.2 3.5 L 14.8 3.5 M 10.2 3.5 L 10.2 9.2 L 5.4 17.8 C 4.8 18.9 5.6 20.2 6.8 20.2 L 17.2 20.2 C 18.4 20.2 19.2 18.9 18.6 17.8 L 13.8 9.2 L 13.8 3.5 M 7.6 15.2 L 16.4 15.2",
        "prompt_git_log": "M 7 4 C 8.4 4 9.5 5.1 9.5 6.5 C 9.5 7.9 8.4 9 7 9 C 5.6 9 4.5 7.9 4.5 6.5 C 4.5 5.1 5.6 4 7 4 Z M 7 15 C 8.4 15 9.5 16.1 9.5 17.5 C 9.5 18.9 8.4 20 7 20 C 5.6 20 4.5 18.9 4.5 17.5 C 4.5 16.1 5.6 15 7 15 Z M 17.5 7 C 18.9 7 20 8.1 20 9.5 C 20 10.9 18.9 12 17.5 12 C 16.1 12 15 10.9 15 9.5 C 15 8.1 16.1 7 17.5 7 Z M 7 9 L 7 15 M 17.5 12 C 17.5 15.2 14.2 17.5 9.5 17.5",
        "prompt_security": "M 12 3 L 19 6 L 19 11.8 C 19 16.4 15.9 19.7 12 21 C 8.1 19.7 5 16.4 5 11.8 L 5 6 Z M 9.2 12.2 L 11.2 14.2 L 15.2 10",
        "prompt_changelog": "M 4.5 12.4 L 12.4 4.5 L 19.5 4.5 L 19.5 11.6 L 11.6 19.5 Z M 15.2 8.2 C 15.8 8.2 16.3 8.7 16.3 9.3 C 16.3 9.9 15.8 10.4 15.2 10.4 C 14.6 10.4 14.1 9.9 14.1 9.3 C 14.1 8.7 14.6 8.2 15.2 8.2 Z"
    ]

    private static let cache = NSCache<NSString, CGPathBox>()
    private final class CGPathBox: NSObject {
        let path: CGPath
        init(path: CGPath) { self.path = path }
    }

    static func cachedPath(for key: String) -> CGPath {
        if key.hasPrefix("wf:") {
            let wfKey = String(key.dropFirst(3))
            return WorkflowTemplateStyle.cachedCGPath(for: wfKey)
        }
        if let hit = cache.object(forKey: key as NSString) { return hit.path }
        let d = svgPaths[key] ?? svgPaths["slash_skill"]!
        let parsed = parseSVGPath(d)
        cache.setObject(CGPathBox(path: parsed), forKey: key as NSString)
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
                    if cmd.uppercased() == "Z" { path.closeSubpath() }
                }
                continue
            }
            switch cmd.uppercased() {
            case "M":
                if let x = scanner.scanDouble(), let y = scanner.scanDouble() {
                    path.move(to: CGPoint(x: x, y: y))
                    cmd = "L"
                } else { return path }
            case "L":
                if let x = scanner.scanDouble(), let y = scanner.scanDouble() {
                    path.addLine(to: CGPoint(x: x, y: y))
                } else { return path }
            case "C":
                if let x1 = scanner.scanDouble(), let y1 = scanner.scanDouble(),
                   let x2 = scanner.scanDouble(), let y2 = scanner.scanDouble(),
                   let x = scanner.scanDouble(), let y = scanner.scanDouble() {
                    path.addCurve(
                        to: CGPoint(x: x, y: y),
                        control1: CGPoint(x: x1, y: y1),
                        control2: CGPoint(x: x2, y: y2)
                    )
                } else { return path }
            case "Q":
                if let x1 = scanner.scanDouble(), let y1 = scanner.scanDouble(),
                   let x = scanner.scanDouble(), let y = scanner.scanDouble() {
                    path.addQuadCurve(to: CGPoint(x: x, y: y), control: CGPoint(x: x1, y: y1))
                } else { return path }
            default:
                return path
            }
        }
        return path
    }
}

private struct LaunchpadSVGShape: Shape {
    let iconKey: String
    func path(in rect: CGRect) -> Path {
        let base = LaunchpadCatalog.cachedPath(for: iconKey)
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

struct LaunchpadSVGIconView: View {
    let iconKey: String
    let tint: Color
    var size: CGFloat = 15

    var body: some View {
        LaunchpadSVGShape(iconKey: iconKey)
            .stroke(
                tint,
                style: StrokeStyle(
                    lineWidth: max(1.45, size * (1.9 / 24.0)),
                    lineCap: .round,
                    lineJoin: .round
                )
            )
            .frame(width: size, height: size)
    }
}

/// 手机版双态新建对话启动台：
/// - 状态一（默认 · 速发聚焦态，键盘展开）：单行工作区/Git 条 + 用户自选 2×2 快捷起手网格（彻底移除问候语与 7 日 Token 彩蛋）
/// - 状态二（全屏 · 探索与配置态，收起键盘）：单行工作区/Git 条 + 2×2 配置页入口 + 2×4 学习区（上 4 随机系统斜杠 + 下 4 场景提示词，长按一键固定到 2×2）
/// - 宽屏物理键盘（iPad 妙控键盘等无虚拟键盘遮挡时）：保持输入框可即打即发，并在顶部提供「快捷 2×2 / 探索 2×4」轻量切换
/// - 全程严禁使用洋红/亮紫红系配色（遵守 AGENTS.md 第 9 节红线）。
public struct NewChatLaunchpadView: View {
    public let viewModel: ChatViewModel
    public let isKeyboardFocused: Bool
    public let isAwaitingInitialAutoFocus: Bool
    public let onFillPrompt: (String) -> Void
    public let onSelectSlashCommand: (SlashCommandOption) -> Void
    public let onSwitchWorkspace: () -> Void
    public let onTapDirtyFiles: () -> Void
    public let onDismissKeyboard: () -> Void
    public let onOpenWorkflowTemplates: () -> Void

    @Environment(\.horizontalSizeClass) private var horizontalSizeClass

    @State private var workflowStore = WorkflowTemplateStore.shared
    @State private var shortcutStore = LaunchpadShortcutsStore.shared
    @State private var shuffleSeed: Int = Int.random(in: 1...999_999)
    @State private var softwareKeyboardHeight: CGFloat = 0
    @State private var wideScreenExploreMode: Bool = false
    @State private var showConfiguratorSheet: Bool = false
    @State private var pendingPinItem: LaunchpadShortcutItem?
    @State private var toastMessage: String?

    private enum CardAction {
        case fillPrompt(String)
        case selectSlash(SlashCommandOption)
    }

    private struct LaunchCard: Identifiable {
        let id: String
        let svgKey: String
        let tint: Color
        let title: String
        let subtitle: String
        let action: CardAction
        let shortcutItem: LaunchpadShortcutItem?
    }

    public init(
        viewModel: ChatViewModel,
        isKeyboardFocused: Bool,
        isAwaitingInitialAutoFocus: Bool = false,
        onFillPrompt: @escaping (String) -> Void,
        onSelectSlashCommand: @escaping (SlashCommandOption) -> Void,
        onSwitchWorkspace: @escaping () -> Void,
        onTapDirtyFiles: @escaping () -> Void,
        onDismissKeyboard: @escaping () -> Void,
        onOpenWorkflowTemplates: @escaping () -> Void
    ) {
        self.viewModel = viewModel
        self.isKeyboardFocused = isKeyboardFocused
        self.isAwaitingInitialAutoFocus = isAwaitingInitialAutoFocus
        self.onFillPrompt = onFillPrompt
        self.onSelectSlashCommand = onSelectSlashCommand
        self.onSwitchWorkspace = onSwitchWorkspace
        self.onTapDirtyFiles = onTapDirtyFiles
        self.onDismissKeyboard = onDismissKeyboard
        self.onOpenWorkflowTemplates = onOpenWorkflowTemplates
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

    private var isHardwareKeyboardWideMode: Bool {
        horizontalSizeClass == .regular && softwareKeyboardHeight < 100
    }

    /// 紧凑屏幕（手机 / iPad 软键盘弹出）由焦点与首帧锁存直入速发态；宽屏无虚拟键盘时由顶部轻量开关切换。
    private var isShowingFocusState: Bool {
        if isHardwareKeyboardWideMode {
            return !wideScreenExploreMode
        }
        return isKeyboardFocused || isAwaitingInitialAutoFocus
    }

    private var workspaceDisplayName: String {
        if let project = viewModel.draftProject, !project.isPureChat {
            return project.displayName
        }
        let ws = viewModel.workspaceName.trimmingCharacters(in: .whitespacesAndNewlines)
        if !ws.isEmpty && ws != "Chat" {
            return ws
        }
        return viewModel.currentTitle
    }

    private func deterministicShuffle<T>(_ items: [T], seed: Int) -> [T] {
        guard items.count > 1 else { return items }
        var arr = items
        var state = UInt64(bitPattern: Int64(seed == 0 ? 1 : seed)) &* 6364136223846793005 &+ 1442695040888963407
        for i in stride(from: arr.count - 1, through: 1, by: -1) {
            state = state &* 6364136223846793005 &+ 1442695040888963407
            let j = Int((state >> 33) % UInt64(i + 1))
            if i != j { arr.swapAt(i, j) }
        }
        return arr
    }

    private func makeCoreSlashCard(
        _ spec: LaunchpadCatalog.CoreSlashSpec,
        gatewayByName: [String: SlashCommandOption],
        idPrefix: String
    ) -> LaunchCard {
        let option = gatewayByName[spec.name] ?? SlashCommandOption(
            name: spec.name,
            title: t3(spec.titleZh, spec.titleEn, spec.titleJa),
            description: t3(spec.subZh, spec.subEn, spec.subJa),
            icon: spec.fallbackIcon,
            kind: "system"
        )
        return LaunchCard(
            id: "\(idPrefix)_\(spec.name)",
            svgKey: spec.svgKey,
            tint: spec.tint,
            title: t3(spec.titleZh, spec.titleEn, spec.titleJa),
            subtitle: t3(spec.subZh, spec.subEn, spec.subJa),
            action: .selectSlash(option),
            shortcutItem: LaunchpadShortcutItem(
                kind: "slash",
                ref: spec.name,
                title: t3(spec.titleZh, spec.titleEn, spec.titleJa),
                subtitle: t3(spec.subZh, spec.subEn, spec.subJa),
                svgKey: spec.svgKey,
                tint: spec.tintName
            )
        )
    }

    private func makeDynamicSlashCard(
        _ opt: SlashCommandOption,
        coreByName: [String: LaunchpadCatalog.CoreSlashSpec],
        idPrefix: String
    ) -> LaunchCard {
        let lower = opt.name.lowercased()
        if let spec = coreByName[lower] {
            return LaunchCard(
                id: "\(idPrefix)_\(opt.name)",
                svgKey: spec.svgKey,
                tint: spec.tint,
                title: t3(spec.titleZh, spec.titleEn, spec.titleJa),
                subtitle: t3(spec.subZh, spec.subEn, spec.subJa),
                action: .selectSlash(opt),
                shortcutItem: LaunchpadShortcutItem(
                    kind: "slash",
                    ref: spec.name,
                    title: t3(spec.titleZh, spec.titleEn, spec.titleJa),
                    subtitle: t3(spec.subZh, spec.subEn, spec.subJa),
                    svgKey: spec.svgKey,
                    tint: spec.tintName
                )
            )
        }
        let sub = opt.description.trimmingCharacters(in: .whitespacesAndNewlines)
        let kind = opt.kind.lowercased() == "system" ? "slash" : "skill"
        return LaunchCard(
            id: "\(idPrefix)_\(opt.name)",
            svgKey: "slash_skill",
            tint: LaunchpadCatalog.tintTeal,
            title: "/\(opt.name)",
            subtitle: sub.isEmpty ? t3("高频常用技能指令", "Frequently used skill", "よく使うスキルコマンド") : sub,
            action: .selectSlash(opt),
            shortcutItem: LaunchpadShortcutItem(
                kind: kind,
                ref: opt.name,
                title: "/\(opt.name)",
                subtitle: sub,
                svgKey: "slash_skill",
                tint: "teal"
            )
        )
    }

    private func makePromptCard(
        _ spec: LaunchpadCatalog.PromptSpec,
        dirtyCount: Int,
        idPrefix: String
    ) -> LaunchCard {
        let title: String
        if spec.id == "ws_git_diff" && dirtyCount > 0 {
            title = t3("审查 \(dirtyCount) 个改动", "Review \(dirtyCount) changes", "変更 \(dirtyCount) 件を確認")
        } else {
            title = t3(spec.titleZh, spec.titleEn, spec.titleJa)
        }
        return LaunchCard(
            id: "\(idPrefix)_\(spec.id)",
            svgKey: spec.svgKey,
            tint: spec.tint,
            title: title,
            subtitle: t3(spec.subZh, spec.subEn, spec.subJa),
            action: .fillPrompt(t3(spec.promptZh, spec.promptEn, spec.promptJa)),
            shortcutItem: LaunchpadShortcutItem(
                kind: "prompt",
                ref: spec.id,
                title: t3(spec.titleZh, spec.titleEn, spec.titleJa),
                subtitle: t3(spec.subZh, spec.subEn, spec.subJa),
                svgKey: spec.svgKey,
                tint: spec.tintName
            )
        )
    }

    private func resolvePinnedCard(
        _ item: LaunchpadShortcutItem,
        gatewayByName: [String: SlashCommandOption],
        coreByName: [String: LaunchpadCatalog.CoreSlashSpec],
        dirtyCount: Int
    ) -> LaunchCard? {
        switch item.kind {
        case "workflow":
            guard let tpl = workflowStore.templates.first(where: { $0.id == item.ref }) else {
                return nil
            }
            return LaunchCard(
                id: "pin_wf_\(tpl.id)",
                svgKey: "wf:\(WorkflowTemplateStyle.normalizedIconKey(tpl.icon))",
                tint: LaunchpadCatalog.workflowTint(tpl.color),
                title: tpl.title,
                subtitle: tpl.subtitle.isEmpty ? t3("自定义快捷模板", "Custom quick template", "カスタムショートカット") : tpl.subtitle,
                action: .fillPrompt(tpl.templatePrompt),
                shortcutItem: item
            )
        case "slash", "skill":
            let lower = item.ref.lowercased()
            if let spec = coreByName[lower] {
                return makeCoreSlashCard(spec, gatewayByName: gatewayByName, idPrefix: "pin_slash")
            }
            if let live = gatewayByName[lower] {
                return makeDynamicSlashCard(live, coreByName: coreByName, idPrefix: "pin_slash")
            }
            let fallbackOpt = SlashCommandOption(
                name: lower,
                title: item.title ?? "/\(lower)",
                description: item.subtitle ?? "",
                icon: "terminal",
                kind: item.kind == "slash" ? "system" : "skill"
            )
            return LaunchCard(
                id: "pin_slash_\(lower)",
                svgKey: item.svgKey ?? "slash_skill",
                tint: LaunchpadCatalog.tintColor(item.tint ?? "teal"),
                title: item.title ?? "/\(lower)",
                subtitle: (item.subtitle ?? "").isEmpty ? t3("自定义斜杠指令", "Custom slash command", "カスタムコマンド") : (item.subtitle ?? ""),
                action: .selectSlash(fallbackOpt),
                shortcutItem: item
            )
        case "prompt":
            if let spec = LaunchpadCatalog.allPromptPool.first(where: { $0.id == item.ref }) {
                return makePromptCard(spec, dirtyCount: dirtyCount, idPrefix: "pin_prompt")
            }
            return nil
        default:
            return nil
        }
    }

    /// 状态一：2×2 快捷起手卡片（永远饱满 4 格）：
    /// - 用户已自定义：先放用户选定项；若不足 2 格用当前模式随机模板补满前排，若不足 4 格用随机系统核心斜杠补满后排；
    /// - 用户未自定义：不自动套用自定义 Workflow，上排 2 个纯对话/代码区随机模板 + 下排 2 个随机系统核心斜杠。
    private var quickFocusCards: [LaunchCard] {
        let gatewayCommands = viewModel.slashCommands
        var gatewayByName: [String: SlashCommandOption] = [:]
        for cmd in gatewayCommands {
            gatewayByName[cmd.name.lowercased()] = cmd
        }
        var coreByName: [String: LaunchpadCatalog.CoreSlashSpec] = [:]
        for spec in LaunchpadCatalog.coreSlashPool {
            coreByName[spec.name] = spec
        }

        let isPure = viewModel.isPureChat
        let dirtyCount = viewModel.uncommittedFileCount
        var cards: [LaunchCard] = []
        var usedSlashNames = Set<String>()
        var usedPromptIds = Set<String>()

        // 1. 用户显式固定项（最多 4 个）
        if shortcutStore.customized {
            for item in shortcutStore.items.prefix(4) {
                guard let card = resolvePinnedCard(item, gatewayByName: gatewayByName, coreByName: coreByName, dirtyCount: dirtyCount) else {
                    continue
                }
                cards.append(card)
                switch item.kind {
                case "slash", "skill": usedSlashNames.insert(item.ref.lowercased())
                case "prompt": usedPromptIds.insert(item.ref)
                default: break
                }
            }
        }

        // 2. 上排补齐（最多补至 2 格）：基于纯对话或代码工作区的随机提示词模板
        if cards.count < 2 {
            let modePool: [LaunchpadCatalog.PromptSpec] = isPure
                ? LaunchpadCatalog.pureChatPromptPool
                : LaunchpadCatalog.workspacePromptPool.filter { !$0.requiresDirtyFiles || dirtyCount > 0 }
            let filteredPrompts = modePool.filter { !usedPromptIds.contains($0.id) }
            for spec in deterministicShuffle(filteredPrompts, seed: shuffleSeed ^ 0x7E4B21) where cards.count < 2 {
                guard usedPromptIds.insert(spec.id).inserted else { continue }
                cards.append(makePromptCard(spec, dirtyCount: dirtyCount, idPrefix: "auto_prompt"))
            }
        }

        // 3. 下排补齐（补满至 4 格）：先对固定 9 项系统核心斜杠池做确定性洗牌再过滤，避免网关异步回包改变数组长度引发卡片重排闪烁
        if cards.count < 4 {
            let shuffledCore = deterministicShuffle(LaunchpadCatalog.coreSlashPool, seed: shuffleSeed ^ 0x13579B)
            let unusedCore = shuffledCore.filter { !usedSlashNames.contains($0.name) }
            let supportedUnused = gatewayCommands.isEmpty
                ? unusedCore
                : unusedCore.filter { gatewayByName[$0.name] != nil }
            let slashPool = supportedUnused.count >= (4 - cards.count) ? supportedUnused : unusedCore
            for spec in slashPool where cards.count < 4 {
                guard usedSlashNames.insert(spec.name).inserted else { continue }
                cards.append(makeCoreSlashCard(spec, gatewayByName: gatewayByName, idPrefix: "auto_slash"))
            }
        }

        return Array(cards.prefix(4))
    }

    /// 状态二上半 2×2（4 张）：随机系统核心斜杠教学池。
    private var exploreSlashCards: [LaunchCard] {
        let gatewayCommands = viewModel.slashCommands
        var gatewayByName: [String: SlashCommandOption] = [:]
        for cmd in gatewayCommands {
            gatewayByName[cmd.name.lowercased()] = cmd
        }
        let shuffledCore = deterministicShuffle(LaunchpadCatalog.coreSlashPool, seed: shuffleSeed ^ 0x5A3C91)
        let supported = gatewayCommands.isEmpty
            ? shuffledCore
            : shuffledCore.filter { gatewayByName[$0.name] != nil }
        let pool = supported.count >= 4 ? supported : shuffledCore
        return pool
            .prefix(4)
            .map { makeCoreSlashCard($0, gatewayByName: gatewayByName, idPrefix: "exp_slash") }
    }

    /// 状态二下半 2×2（4 张）：按当前模式随机展示的任务表达模板池（纯对话 8 条 / 代码区 8 条）。
    private var explorePromptCards: [LaunchCard] {
        let isPure = viewModel.isPureChat
        let dirtyCount = viewModel.uncommittedFileCount
        let pool: [LaunchpadCatalog.PromptSpec] = isPure
            ? LaunchpadCatalog.pureChatPromptPool
            : LaunchpadCatalog.workspacePromptPool.filter { !$0.requiresDirtyFiles || dirtyCount > 0 }
        return deterministicShuffle(pool, seed: shuffleSeed ^ 0x2F8B4D)
            .prefix(4)
            .map { makePromptCard($0, dirtyCount: dirtyCount, idPrefix: "exp_prompt") }
    }

    public var body: some View {
        ZStack(alignment: .bottom) {
            VStack(alignment: .leading, spacing: 0) {
                // 顶部工作区选择栏：固定锚定偏上方位置与恒定高度
                workspacePulseBar

                // 下方双态内容区：直接切换，无闪烁动效
                Group {
                    if isShowingFocusState {
                        focusQuickLaunchSection
                    } else {
                        exploreAndConfigureSection
                    }
                }
                .padding(.top, 22)
            }
            .transaction { $0.animation = nil }
            .padding(.horizontal, 18)
            .padding(.top, 16)
            .padding(.bottom, 20)
            .frame(maxWidth: 540, alignment: .top)
            .frame(maxWidth: .infinity, alignment: .top)

            if let msg = toastMessage {
                Text(msg)
                    .font(.system(size: 12.5, weight: .semibold))
                    .foregroundColor(.white)
                    .padding(.horizontal, 14)
                    .padding(.vertical, 8.5)
                    .background(Color.black.opacity(0.82), in: Capsule())
                    .padding(.bottom, 12)
                    .transition(.move(edge: .bottom).combined(with: .opacity))
                    .allowsHitTesting(false)
            }
        }
        .contentShape(Rectangle())
        .onTapGesture {
            if isKeyboardFocused && !isHardwareKeyboardWideMode {
                onDismissKeyboard()
            }
        }
        .onReceive(NotificationCenter.default.publisher(for: UIResponder.keyboardWillChangeFrameNotification)) { notif in
            // 仅宽屏分栏/外接键盘模式需要监测软件键盘高度，手机端直接跳过避免键盘弹出时额外触发视图重绘
            guard horizontalSizeClass == .regular else { return }
            guard let frame = notif.userInfo?[UIResponder.keyboardFrameEndUserInfoKey] as? CGRect else { return }
            let screenH = UIScreen.main.bounds.height
            let visibleH = max(0, screenH - frame.origin.y)
            if abs(softwareKeyboardHeight - visibleH) > 1 {
                softwareKeyboardHeight = visibleH
            }
        }
        .onReceive(NotificationCenter.default.publisher(for: UIResponder.keyboardWillHideNotification)) { _ in
            guard horizontalSizeClass == .regular else { return }
            if softwareKeyboardHeight != 0 {
                softwareKeyboardHeight = 0
            }
        }
        .sheet(isPresented: $showConfiguratorSheet, onDismiss: {
            pendingPinItem = nil
        }) {
            LaunchpadShortcutConfiguratorSheet(
                slashCommands: viewModel.slashCommands,
                pendingPinItem: pendingPinItem,
                onOpenWorkflowManager: onOpenWorkflowTemplates
            )
            .presentationDetents([.large])
            .presentationDragIndicator(.visible)
        }
        .task(id: viewModel.cascadeId) {
            // 不再在 .task 里重洗 shuffleSeed（@State 初始化已给出每页独立随机种子，避免横向推入中途卡片重排闪烁）；
            // 新建草稿页让路给 350ms 横向推入动画后再静默同步网关数据。
            if viewModel.isDraftConversation {
                try? await Task.sleep(nanoseconds: 420_000_000)
                guard !Task.isCancelled else { return }
            }
            viewModel.ensureSlashCommands()
            await shortcutStore.refresh()
            await workflowStore.refresh()
        }
    }

    private func triggerCard(_ card: LaunchCard) {
        UIImpactFeedbackGenerator(style: .light).impactOccurred()
        switch card.action {
        case .fillPrompt(let prompt):
            onFillPrompt(prompt)
        case .selectSlash(let opt):
            SlashUsageLocalStore.record(opt.name)
            onSelectSlashCommand(opt)
        }
    }

    private func handleLongPressPin(_ item: LaunchpadShortcutItem) {
        if shortcutStore.isPinned(kind: item.kind, ref: item.ref) {
            UIImpactFeedbackGenerator(style: .light).impactOccurred()
            shortcutStore.unpin(kind: item.kind, ref: item.ref)
            showToast(t3("已从 2×2 移除", "Removed from 2×2", "2×2 から削除しました"))
            return
        }
        switch shortcutStore.pin(item) {
        case .added(let count):
            UIImpactFeedbackGenerator(style: .medium).impactOccurred()
            showToast(t3("已固定到新建页 2×2 (\(count)/4)", "Pinned to 2×2 (\(count)/4)", "2×2 に固定しました (\(count)/4)"))
        case .alreadyPinned:
            break
        case .full:
            UIImpactFeedbackGenerator(style: .medium).impactOccurred()
            pendingPinItem = item
            showConfiguratorSheet = true
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

    // MARK: - 状态一：默认「速发聚焦态」（舒展间距，防误触）

    private var focusQuickLaunchSection: some View {
        VStack(alignment: .leading, spacing: 14) {
            HStack(alignment: .center) {
                Text(t3("快捷起手", "Quick Start", "クイック起動"))
                    .font(.system(size: 12.5, weight: .medium))
                    .foregroundColor(.secondary)
                    .padding(.leading, 2)
                Spacer()
                if isHardwareKeyboardWideMode {
                    wideModeTogglePill
                } else {
                    Button {
                        UIImpactFeedbackGenerator(style: .light).impactOccurred()
                        onDismissKeyboard()
                    } label: {
                        HStack(spacing: 4) {
                            Text(t3("探索与配置", "Explore & Edit", "探索と設定"))
                                .font(.system(size: 12.5, weight: .medium))
                            Image(systemName: "chevron.right")
                                .font(.system(size: 10, weight: .semibold))
                        }
                        .foregroundColor(.secondary)
                        .padding(.horizontal, 10)
                        .padding(.vertical, 6)
                        .background(Color(uiColor: .secondarySystemBackground).opacity(0.65), in: Capsule())
                        .contentShape(Capsule())
                    }
                    .buttonStyle(.plain)
                }
            }

            LazyVGrid(
                columns: [
                    GridItem(.flexible(), spacing: 12),
                    GridItem(.flexible(), spacing: 12)
                ],
                spacing: 12
            ) {
                ForEach(quickFocusCards) { card in
                    quickCardView(card, allowLongPressPin: false)
                }
            }
        }
    }

    // MARK: - 状态二：全屏「探索与配置态」（组件展开）

    private var exploreAndConfigureSection: some View {
        VStack(alignment: .leading, spacing: 18) {
            // 1. 自定义 2×2 配置入口条
            HStack(spacing: 10) {
                Button {
                    UIImpactFeedbackGenerator(style: .light).impactOccurred()
                    pendingPinItem = nil
                    showConfiguratorSheet = true
                } label: {
                    HStack(spacing: 9) {
                        ZStack {
                            RoundedRectangle(cornerRadius: 7, style: .continuous)
                                .fill(Color.indigo.opacity(0.14))
                                .frame(width: 26, height: 26)
                            Image(systemName: "square.grid.2x2")
                                .font(.system(size: 12.5, weight: .semibold))
                                .foregroundColor(.indigo)
                        }
                        VStack(alignment: .leading, spacing: 1.5) {
                            Text(t3("自定义 2×2 快捷指令", "Customize 2×2 Shortcuts", "2×2 ショートカット設定"))
                                .font(.system(size: 13.5, weight: .semibold))
                                .foregroundColor(.primary)
                            Text(shortcutStore.customized
                                 ? t3("已固定 \(shortcutStore.items.count)/4 项 · 长按下方卡片亦可一键添加", "\(shortcutStore.items.count)/4 pinned · Long-press cards below to pin", "\(shortcutStore.items.count)/4 固定済み · 下のカードを長押しで追加")
                                 : t3("当前为随机推荐 · 长按下方卡片可一键固定", "Random discovery · Long-press any card below to pin", "ランダム表示中 · カード長押しで固定可能"))
                                .font(.system(size: 11))
                                .foregroundColor(.secondary)
                                .lineLimit(1)
                        }
                        Spacer(minLength: 4)
                        Image(systemName: "chevron.right")
                            .font(.system(size: 11, weight: .semibold))
                            .foregroundColor(Color(uiColor: .tertiaryLabel))
                    }
                    .padding(.horizontal, 13)
                    .padding(.vertical, 11)
                    .background(Color(uiColor: .secondarySystemBackground))
                    .clipShape(RoundedRectangle(cornerRadius: 14, style: .continuous))
                    .overlay(
                        RoundedRectangle(cornerRadius: 14, style: .continuous)
                            .stroke(Color.indigo.opacity(0.18), lineWidth: 0.9)
                    )
                }
                .buttonStyle(.plain)

                if isHardwareKeyboardWideMode {
                    wideModeTogglePill
                }
            }

            // 2. 上半 2×2（4 张）：斜杠指令用法探索 + 换一批
            VStack(alignment: .leading, spacing: 10) {
                HStack(alignment: .center) {
                    Text(t3("探索斜杠指令", "Discover Slash Commands", "スラッシュコマンド探索"))
                        .font(.system(size: 12.5, weight: .medium))
                        .foregroundColor(.secondary)
                        .padding(.leading, 2)
                    Spacer()
                    Button {
                        UIImpactFeedbackGenerator(style: .light).impactOccurred()
                        withAnimation(.easeInOut(duration: 0.18)) {
                            shuffleSeed &+= 1
                        }
                    } label: {
                        HStack(spacing: 4) {
                            LaunchpadSVGIconView(iconKey: "shuffle_refresh", tint: .secondary, size: 12.5)
                            Text(t3("换一批", "Shuffle", "入れ替え"))
                                .font(.system(size: 12, weight: .medium))
                                .foregroundColor(.secondary)
                        }
                        .padding(.horizontal, 8)
                        .padding(.vertical, 4)
                        .contentShape(Rectangle())
                    }
                    .buttonStyle(.plain)
                }

                LazyVGrid(
                    columns: [
                        GridItem(.flexible(), spacing: 12),
                        GridItem(.flexible(), spacing: 12)
                    ],
                    spacing: 12
                ) {
                    ForEach(exploreSlashCards) { card in
                        quickCardView(card, allowLongPressPin: true)
                    }
                }
            }

            // 3. 下半 2×2（4 张）：任务表达提示词池（纯对话 vs 代码工作区自动切换）
            VStack(alignment: .leading, spacing: 10) {
                Text(viewModel.isPureChat
                     ? t3("纯对话任务表达", "Chat Prompt Ideas", "チャット向けプロンプト例")
                     : t3("代码工作区任务表达", "Workspace Prompt Ideas", "開発ワークスペース向け例"))
                    .font(.system(size: 12.5, weight: .medium))
                    .foregroundColor(.secondary)
                    .padding(.leading, 2)

                LazyVGrid(
                    columns: [
                        GridItem(.flexible(), spacing: 12),
                        GridItem(.flexible(), spacing: 12)
                    ],
                    spacing: 12
                ) {
                    ForEach(explorePromptCards) { card in
                        quickCardView(card, allowLongPressPin: true)
                    }
                }
            }
        }
    }

    private var wideModeTogglePill: some View {
        Button {
            UIImpactFeedbackGenerator(style: .light).impactOccurred()
            withAnimation(.snappy(duration: 0.28, extraBounce: 0)) {
                wideScreenExploreMode.toggle()
            }
        } label: {
            HStack(spacing: 4) {
                Image(systemName: wideScreenExploreMode ? "bolt.fill" : "safari")
                    .font(.system(size: 11, weight: .semibold))
                Text(wideScreenExploreMode ? t3("快捷 2×2", "Quick 2×2", "2×2") : t3("探索与配置", "Explore", "探索"))
                    .font(.system(size: 12, weight: .semibold))
            }
            .foregroundColor(.indigo)
            .padding(.horizontal, 10)
            .padding(.vertical, 6)
            .background(Color.indigo.opacity(0.12), in: Capsule())
        }
        .buttonStyle(.plain)
    }

    /// 恒定高度、恒定文案的工作区选择条（键盘展开与收起时完全静止不动）
    private var workspacePulseBar: some View {
        HStack(spacing: 8) {
            if viewModel.isPureChat {
                Image(systemName: "bubble.left.and.bubble.right.fill")
                    .font(.system(size: 13))
                    .foregroundColor(.indigo)
                Text(t3("纯对话模式 · 不关联代码工作区", "Pure chat · No workspace linked", "チャット専用 · ワークスペース未連携"))
                    .font(.system(size: 13))
                    .foregroundColor(.secondary)
                    .lineLimit(1)
                    .minimumScaleFactor(0.85)
                Spacer(minLength: 6)
                if viewModel.isDraftConversation {
                    Button {
                        UIImpactFeedbackGenerator(style: .light).impactOccurred()
                        onSwitchWorkspace()
                    } label: {
                        Text(t3("选工作区", "Workspace", "選択"))
                            .font(.system(size: 13, weight: .semibold))
                            .foregroundColor(.indigo)
                            .padding(.horizontal, 6)
                            .padding(.vertical, 4)
                            .contentShape(Rectangle())
                    }
                    .buttonStyle(.plain)
                }
            } else {
                Image(systemName: "folder.fill")
                    .font(.system(size: 13))
                    .foregroundColor(.indigo)
                Text(workspaceDisplayName)
                    .font(.system(size: 13.5, weight: .semibold))
                    .foregroundColor(.primary)
                    .lineLimit(1)
                    .truncationMode(.tail)

                if let branch = viewModel.gitBranch, !branch.isEmpty {
                    Text(branch)
                        .font(.system(size: 11, weight: .medium, design: .monospaced))
                        .foregroundColor(.secondary)
                        .padding(.horizontal, 6)
                        .padding(.vertical, 2)
                        .background(Color.primary.opacity(0.06))
                        .clipShape(RoundedRectangle(cornerRadius: 5, style: .continuous))
                        .lineLimit(1)
                        .fixedSize(horizontal: true, vertical: false)
                }

                if viewModel.uncommittedFileCount > 0 {
                    Button {
                        UIImpactFeedbackGenerator(style: .light).impactOccurred()
                        onTapDirtyFiles()
                    } label: {
                        Text(t3("\(viewModel.uncommittedFileCount) 改动", "\(viewModel.uncommittedFileCount) dirty", "\(viewModel.uncommittedFileCount) 変更"))
                            .font(.system(size: 11, weight: .semibold))
                            .foregroundColor(LaunchpadCatalog.tintAmber)
                            .padding(.horizontal, 6)
                            .padding(.vertical, 2)
                            .background(LaunchpadCatalog.tintAmber.opacity(0.14))
                            .clipShape(RoundedRectangle(cornerRadius: 5, style: .continuous))
                            .fixedSize(horizontal: true, vertical: false)
                    }
                    .buttonStyle(.plain)
                } else if viewModel.gitBranch != nil {
                    Text(t3("干净", "Clean", "クリーン"))
                        .font(.system(size: 11))
                        .foregroundColor(.secondary)
                        .fixedSize(horizontal: true, vertical: false)
                }

                Spacer(minLength: 4)

                if viewModel.isDraftConversation {
                    Button {
                        UIImpactFeedbackGenerator(style: .light).impactOccurred()
                        onSwitchWorkspace()
                    } label: {
                        Text(t3("切换", "Switch", "切替"))
                            .font(.system(size: 13, weight: .semibold))
                            .foregroundColor(.indigo)
                            .padding(.horizontal, 6)
                            .padding(.vertical, 4)
                            .contentShape(Rectangle())
                            .fixedSize(horizontal: true, vertical: false)
                    }
                    .buttonStyle(.plain)
                }
            }
        }
        .padding(.horizontal, 14)
        .frame(height: 46)
        .background(Color(uiColor: .secondarySystemBackground))
        .clipShape(RoundedRectangle(cornerRadius: 14, style: .continuous))
        .overlay(
            RoundedRectangle(cornerRadius: 14, style: .continuous)
                .stroke(Color.primary.opacity(0.07), lineWidth: 0.8)
        )
    }

    @ViewBuilder
    private func quickCardView(_ card: LaunchCard, allowLongPressPin: Bool) -> some View {
        let isPinnedInCustom = allowLongPressPin && (card.shortcutItem.map { shortcutStore.isPinned(kind: $0.kind, ref: $0.ref) } ?? false)
        let cardContent = Button {
            triggerCard(card)
        } label: {
            VStack(alignment: .leading, spacing: 6.5) {
                HStack(spacing: 7.5) {
                    ZStack {
                        RoundedRectangle(cornerRadius: 6.5, style: .continuous)
                            .fill(card.tint.opacity(0.13))
                            .frame(width: 22, height: 22)
                        LaunchpadSVGIconView(iconKey: card.svgKey, tint: card.tint, size: 13.5)
                    }
                    Text(card.title)
                        .font(.system(size: 13, weight: .semibold))
                        .foregroundColor(.primary)
                        .lineLimit(1)
                    Spacer(minLength: 2)
                    if isPinnedInCustom {
                        Image(systemName: "pin.fill")
                            .font(.system(size: 9.5))
                            .foregroundColor(.indigo.opacity(0.85))
                    }
                }
                Text(card.subtitle)
                    .font(.system(size: 11.5))
                    .foregroundColor(.secondary)
                    .lineLimit(1)
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(.horizontal, 13)
            .padding(.vertical, 13.5)
            .background(Color(uiColor: .secondarySystemBackground))
            .clipShape(RoundedRectangle(cornerRadius: 14, style: .continuous))
            .overlay(
                RoundedRectangle(cornerRadius: 14, style: .continuous)
                    .stroke(isPinnedInCustom ? Color.indigo.opacity(0.28) : Color.primary.opacity(0.07), lineWidth: 0.8)
            )
        }
        .buttonStyle(.plain)

        if allowLongPressPin, let shortcutItem = card.shortcutItem {
            cardContent.contextMenu {
                Button {
                    handleLongPressPin(shortcutItem)
                } label: {
                    Label(
                        isPinnedInCustom
                            ? t3("从 2×2 快捷指令移除", "Remove from 2×2 Shortcuts", "2×2 から削除")
                            : t3("固定到新建页 2×2 快捷指令", "Pin to 2×2 Quick Launch", "2×2 ショートカットに固定"),
                        systemImage: isPinnedInCustom ? "pin.slash" : "pin"
                    )
                }
                Button {
                    triggerCard(card)
                } label: {
                    Label(t3("立即试用", "Try Now", "今すぐ試す"), systemImage: "arrow.up.right.circle")
                }
            }
        } else {
            cardContent
        }
    }
}
