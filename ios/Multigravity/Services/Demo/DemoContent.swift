import Foundation
import UIKit

/// Seed data and programmatic sample assets (CSV, PNG chart, JPG invoice, PDF report, Jupyter Notebook) for Demo Mode.
extension DemoGateway {
    func seed() {
        let now = Date()
        let pAnalytics = Project(id: "proj-analytics", name: "数据分析台", alias: "📊 数据分析台", workspaceUri: "file:///Users/demo/Projects/analytics", path: "/Users/demo/Projects/analytics", isWorkspace: true)
        let pMobile = Project(id: "proj-mobile", name: "Multigravity", alias: nil, workspaceUri: "file:///Users/demo/Projects/multigravity", path: "/Users/demo/Projects/multigravity", isWorkspace: true)
        let pRisk = Project(id: "proj-risk", name: "风控引擎", alias: nil, workspaceUri: "file:///Users/demo/Projects/risk-engine", path: "/Users/demo/Projects/risk-engine", isWorkspace: true)
        let pDeploy = Project(id: "proj-deploy", name: "部署脚本", alias: nil, workspaceUri: "file:///Users/demo/Projects/deploy-infra", path: "/Users/demo/Projects/deploy-infra", isWorkspace: true)
        let pChat = Project(id: "proj-chat", name: "纯对话", alias: nil, workspaceUri: "chat", path: "chat", isWorkspace: false)
        projects = [pAnalytics, pMobile, pRisk, pDeploy, pChat]

        accounts = [
            QuotaAccount(
                id: "acc-ultra",
                email: "demo.ultra@antigravity.ai",
                name: "Demo Ultra",
                tier: "ULTRA",
                claude5h: 82,
                claudeWeekly: 74,
                gemini5h: 91,
                geminiWeekly: 85,
                argon: 88,
                claude5hOffsetHours: 3.4,
                claudeWeeklyOffsetDays: 4.2,
                gemini5hOffsetHours: 4.1,
                geminiWeeklyOffsetDays: 4.2
            ),
            QuotaAccount(
                id: "acc-pro",
                email: "backup.pro@antigravity.ai",
                name: "Backup Pro",
                tier: "PRO",
                claude5h: 96,
                claudeWeekly: 92,
                gemini5h: 88,
                geminiWeekly: 94,
                argon: 76,
                claude5hOffsetHours: 4.6,
                claudeWeeklyOffsetDays: 5.8,
                gemini5hOffsetHours: 2.7,
                geminiWeeklyOffsetDays: 5.8
            ),
            QuotaAccount(
                id: "acc-free",
                email: "sandbox.free@antigravity.ai",
                name: "Sandbox Free",
                tier: "FREE",
                claude5h: 0,
                claudeWeekly: 65,
                gemini5h: 0,
                geminiWeekly: 70,
                argon: nil,
                claude5hOffsetHours: 0,
                claudeWeeklyOffsetDays: 3.5,
                gemini5hOffsetHours: 0,
                geminiWeeklyOffsetDays: 3.5
            )
        ]

        // Sample downloadable / previewable assets
        let csvData = Data(Self.salesCSV.utf8)
        let chartData = Self.salesChartPNG()
        let invoiceData = Self.invoiceJPEG()
        let reportData = Self.reportPDF()
        let notesData = Data(Self.meetingNotes.utf8)
        let planData = Data(Self.planMarkdown.utf8)
        let notebookData = Data(Self.notebookJSON.utf8)
        files["sales_q3.csv"] = StoredFile(name: "sales_q3.csv", data: csvData)
        files["revenue_chart.png"] = StoredFile(name: "revenue_chart.png", data: chartData)
        files["invoice_demo.jpg"] = StoredFile(name: "invoice_demo.jpg", data: invoiceData)
        files["report.pdf"] = StoredFile(name: "report.pdf", data: reportData)
        files["meeting_notes.md"] = StoredFile(name: "meeting_notes.md", data: notesData)
        files["plan.md"] = StoredFile(name: "plan.md", data: planData)
        files["analytics.ipynb"] = StoredFile(name: "analytics.ipynb", data: notebookData)

        // Global catalogs
        mcpServers = [
            McpServerState(
                spec: McpServerSpec(serverName: "ifind", command: "python3 -m ifind_mcp.server run"),
                status: "MCP_SERVER_STATUS_READY",
                tools: [
                    McpToolDefinition(name: "get_realtime_quotation", description: "查询证券实时行情、最新价、涨跌幅、盘口及资金流", serverName: "ifind", jsonSchemaString: "{\"properties\":{\"codes\":{\"type\":\"string\",\"description\":\"证券代码，如 600519.SH\"}},\"required\":[\"codes\"]}"),
                    McpToolDefinition(name: "get_history_quotation", description: "查询历史K线行情与序列数据", serverName: "ifind", jsonSchemaString: "{\"properties\":{\"codes\":{\"type\":\"string\"},\"period\":{\"type\":\"string\",\"default\":\"D\"}},\"required\":[\"codes\"]}")
                ],
                serverInfo: McpServerInfoDetail(name: "ifind-mcp", version: "4.0.0"),
                instructions: "iFinD 金融数据终端服务接口"
            ),
            McpServerState(
                spec: McpServerSpec(serverName: "weather", command: "uv run weather-server"),
                status: "MCP_SERVER_STATUS_READY",
                tools: [
                    McpToolDefinition(name: "get_current_weather", description: "获取指定城市的当前实时天气情况", serverName: "weather", jsonSchemaString: "{\"properties\":{\"city\":{\"type\":\"string\",\"description\":\"城市名称\"}},\"required\":[\"city\"]}")
                ],
                serverInfo: McpServerInfoDetail(name: "weather-service", version: "1.2.0"),
                instructions: "全球实时与未来天气查询"
            ),
            McpServerState(
                spec: McpServerSpec(serverName: "message_hub", command: "node message_hub.js"),
                status: "MCP_SERVER_STATUS_READY",
                tools: [
                    McpToolDefinition(name: "send_message", description: "统一向微信或元宝渠道发送通知与长图", serverName: "message_hub", jsonSchemaString: "{\"properties\":{\"target\":{\"type\":\"string\"},\"text\":{\"type\":\"string\"}},\"required\":[\"target\",\"text\"]}")
                ],
                serverInfo: McpServerInfoDetail(name: "message-hub", version: "2.1.0"),
                instructions: "统一消息与多端通信路由"
            ),
            McpServerState(
                spec: McpServerSpec(serverName: "xcode-mcp", command: "xcrun mcpbridge"),
                status: "MCP_SERVER_STATUS_READY",
                tools: [
                    McpToolDefinition(name: "build_and_test", description: "触发 iOS / macOS Scheme 编译与 XCTest 校验", serverName: "xcode-mcp", jsonSchemaString: "{\"properties\":{\"scheme\":{\"type\":\"string\",\"default\":\"Multigravity\"}},\"required\":[\"scheme\"]}")
                ],
                serverInfo: McpServerInfoDetail(name: "xcode-mcp", version: "1.8.0"),
                instructions: "本地 Xcode 构建诊断桥梁"
            )
        ]

        skills = [
            SkillInfo(name: "chart-generator", description: "自动化图表生成与多维度可视化分析", path: "builtin/skills/chart-generator/SKILL.md", content: "# Chart Generator\n自动根据输入数据渲染交互图表并导出高清 PNG。"),
            SkillInfo(name: "sql-inspector", description: "SQL 查询调优与执行计划诊断", path: "builtin/skills/sql-inspector/SKILL.md", content: "# SQL Inspector\n分析 SQL 慢查询、EXPLAIN 计划并提出索引优化方案。"),
            SkillInfo(name: "jiuge-market-analysis", description: "首席宏观研判与市场逻辑观点体系", path: ".gemini/config/skills/jiuge-market-analysis/SKILL.md", content: "# Jiuge Market Analysis\n宏观流动性、大宗商品与行业景气度核心驱动逻辑。"),
            SkillInfo(name: "human-writing", description: "去模型腔长文与报告写作规范", path: ".gemini/config/skills/human-writing/SKILL.md", content: "# Human Writing\n自然中文韵律、事实核验与高信息密度叙事。"),
            SkillInfo(name: "swift-refactor", description: "SwiftUI 视图拆分与 Swift 6 并发重构规范", path: "workspace/skills/swift-refactor/SKILL.md", content: "# Swift Refactor\n保持 @Observable 单向数据流与严格 @MainActor 隔离。")
        ]

        plugins = [
            [
                "id": "github-enterprise",
                "name": "GitHub Actions & PR Bridge",
                "description": "关联仓库 Pull Request、CI 检查日志与代码评审评论同步",
                "version": "2.4.1",
                "enabled": true,
                "installed": true,
                "kind": "connector",
                "authStatus": "authenticated",
                "accounts": ["demo.ultra@antigravity.ai"]
            ],
            [
                "id": "figma-design",
                "name": "Figma Design Inspector",
                "description": "直接提取 Figma Frame Token、组件间距与设计变量生成 SwiftUI 代码",
                "version": "1.9.0",
                "enabled": true,
                "installed": true,
                "kind": "design",
                "authStatus": "needs_login",
                "accounts": []
            ],
            [
                "id": "linear-tracker",
                "name": "Linear Issue Sync",
                "description": "自动同步团队 Issue 状态，支持从聊天流一键创建研发缺陷单",
                "version": "1.5.2",
                "enabled": false,
                "installed": false,
                "kind": "productivity",
                "authStatus": "unauthenticated",
                "accounts": []
            ]
        ]
        pluginProviders = [
            ["id": "github", "name": "GitHub OAuth", "signedIn": true],
            ["id": "figma", "name": "Figma Account", "signedIn": false],
            ["id": "linear", "name": "Linear Workspace", "signedIn": false]
        ]

        automations = [
            [
                "id": "auto-morning-brief",
                "name": "早盘宏观与持仓巡检",
                "description": "每个交易日早 8:00 拉取隔夜美股、美债与大宗异动并生成简报",
                "status": "RUNNING",
                "cronExpression": "0 8 * * 1-5",
                "prompt": "/jiuge-market-analysis 汇总隔夜核心资产波动与当日重要宏观日程",
                "lastRunAt": Self.iso(now.addingTimeInterval(-3 * 3600)),
                "nextRunAt": Self.iso(now.addingTimeInterval(21 * 3600)),
                "enabled": true,
                "workspaceUri": pAnalytics.workspaceUri
            ],
            [
                "id": "auto-ci-watch",
                "name": "PR 回归与构建看守",
                "description": "每 2 小时巡检 Multigravity 主分支与未合入 PR 的测试状态",
                "status": "RUNNING",
                "cronExpression": "0 */2 * * *",
                "prompt": "检查 open PRs 的 CI 状态并汇总异常构建日志",
                "lastRunAt": Self.iso(now.addingTimeInterval(-42 * 60)),
                "nextRunAt": Self.iso(now.addingTimeInterval(78 * 60)),
                "enabled": true,
                "workspaceUri": pMobile.workspaceUri
            ],
            [
                "id": "auto-sec-audit",
                "name": "依赖漏洞周报审计",
                "description": "每周日凌晨扫描 Go/Swift/NPM 依赖树中的高危 CVE",
                "status": "STOPPED",
                "cronExpression": "0 3 * * 0",
                "prompt": "/workflow 运行安全合规依赖审计流水线",
                "lastRunAt": Self.iso(now.addingTimeInterval(-5 * 86400)),
                "nextRunAt": Self.iso(now.addingTimeInterval(2 * 86400)),
                "enabled": false,
                "workspaceUri": pRisk.workspaceUri
            ]
        ]
        automationEvents = [
            [
                "id": "evt-101",
                "scheduleId": "早盘宏观与持仓巡检",
                "conversationId": "demo-0001-upload",
                "timestamp": Self.iso(now.addingTimeInterval(-3 * 3600)),
                "status": "SUCCESS",
                "message": "已生成今日营收与指标巡检摘要，触发 0 条异常告警。"
            ],
            [
                "id": "evt-102",
                "scheduleId": "PR 回归与构建看守",
                "conversationId": "demo-0002-turns",
                "timestamp": Self.iso(now.addingTimeInterval(-42 * 60)),
                "status": "SUCCESS",
                "message": "PR #412 增量编译通过，等待人工 Review。"
            ]
        ]

        workflowTemplates = Self.defaultDemoWorkflowTemplates()

        // Running tasks (also referenced in demo-0008-running)
        let t1Start = Self.iso(now.addingTimeInterval(-195))
        let t2Start = Self.iso(now.addingTimeInterval(-48))
        let daemonTask1: [String: Any] = [
            "conversationId": "demo-0008-running",
            "conversationTitle": "压测联调服务与守护进程（可看日志）",
            "workspaceName": pDeploy.name,
            "taskId": "task-vite-daemon",
            "stepIndex": 1,
            "toolName": "run_command",
            "toolSummary": "启动本地网关调试控制台",
            "commandLine": "npm run dev -- --host 0.0.0.0 --port 5173",
            "startTime": t1Start,
            "logUri": "demo://logs/task-vite-daemon.log",
            "isDaemon": true
        ]
        let runningTask2: [String: Any] = [
            "conversationId": "demo-0008-running",
            "conversationTitle": "压测联调服务与守护进程（可看日志）",
            "workspaceName": pDeploy.name,
            "taskId": "task-go-race",
            "stepIndex": 2,
            "toolName": "run_command",
            "toolSummary": "全量竞态集成测试",
            "commandLine": "go test -race -count=1 ./internal/proxy ./internal/cockpit",
            "startTime": t2Start,
            "logUri": "demo://logs/task-go-race.log",
            "isDaemon": false
        ]
        globalDaemonTasks = [daemonTask1, runningTask2]
        taskLogs["task-vite-daemon"] = """
        VITE v6.2.1  ready in 312 ms

          ➜  Local:   http://localhost:5173/
          ➜  Network: http://192.168.50.12:5173/
        [proxy] WebSocket /gateway/events mounted (heartbeat=5s)
        [cockpit] quota cache warm: 3 accounts loaded
        [metrics] 200 GET /api/v1/cockpit/tokens/history (4.2ms)
        """
        taskLogs["task-go-race"] = """
        === RUN   TestProxySwarmSnapshotETag
        --- PASS: TestProxySwarmSnapshotETag (0.04s)
        === RUN   TestCockpitTokenHistoryAggregation
        --- PASS: TestCockpitTokenHistoryAggregation (0.12s)
        === RUN   TestWorkflowApprovalLifecycle
        --- PASS: TestWorkflowApprovalLifecycle (0.08s)
        running remaining suites in ./internal/cockpit ...
        """

        // 1) Pinned Multimodal & Artifact Conversation (Healthy ContextMeter)
        let c1 = Conversation(
            id: "demo-0001-upload",
            title: "季度营收图表、发票与 PDF 预览（置顶）",
            workspaceName: pAnalytics.name,
            workspaceUri: pAnalytics.workspaceUri,
            status: "CASCADE_RUN_STATUS_IDLE",
            duration: "8秒",
            messages: [
                Self.userMsg(
                    id: "c1-m0",
                    step: 0,
                    text: "请结合上传的报销发票和 Q3 销售表格，生成汇总图和复盘报告。",
                    imageUrls: ["/api/v1/files/raw?path=invoice_demo.jpg"],
                    timestamp: Self.iso(now.addingTimeInterval(-720))
                ),
                Self.toolMsg(id: "c1-m1", step: 1, tools: ["read_file", "run_command", "write_to_file"]),
                Self.agentMsg(
                    id: "c1-m2",
                    step: 2,
                    model: "gemini-4-carbon-high",
                    modelName: "Gemini 4 Carbon (High)",
                    text: """
                    已处理完成（演示模式内置样例）：

                    - 发票核对：**¥1,280.00**（办公设备采购 · 已验真）
                    - 三季度销售额合计：**¥486,400**（华东 42% / 华南 28% / 华北 19% / 西南 11%）
                    - 以下 5 类文件都可以直接点击打开预览或分享：
                      - **CSV 表格**：[sales_q3.csv](/api/v1/files/raw?path=sales_q3.csv)
                      - **高清图表 PNG**：[revenue_chart.png](/api/v1/files/raw?path=revenue_chart.png)
                      - **PDF 正式报告**：[report.pdf](/api/v1/files/raw?path=report.pdf)
                      - **Markdown 纪要**：[meeting_notes.md](/api/v1/files/raw?path=meeting_notes.md)
                    """,
                    artifacts: [
                        ["uri": "sales_q3.csv", "title": "Q3 各大区销售明细 CSV"],
                        ["uri": "revenue_chart.png", "title": "Q3 销售额柱状图 PNG"],
                        ["uri": "report.pdf", "title": "三季度销售复盘报告 PDF"],
                        ["uri": "meeting_notes.md", "title": "运营会议纪要 MD"]
                    ],
                    timestamp: Self.iso(now.addingTimeInterval(-690))
                )
            ],
            pendingInteraction: nil,
            canProceed: false,
            proceedArtifactUri: nil,
            runningTasks: [],
            queuedMessages: [],
            lastModified: now.addingTimeInterval(-5 * 60),
            unread: false,
            pinned: true,
            modelId: "gemini-4-carbon-high",
            modelName: "Gemini 4 Carbon (High)",
            startedAt: Self.iso(now.addingTimeInterval(-750)),
            contextMeter: ContextMeterItem(
                usedTokens: 48_200,
                maxTokens: 200_000,
                percentage: 24.1,
                inputTokens: 16_800,
                outputTokens: 2_340,
                cacheReadTokens: 28_400,
                thinkingTokens: 660,
                timeToFirstTokenSec: 0.62,
                streamingDurationSec: 3.1,
                tokensPerSecond: 46.2,
                isOverflow: false,
                errorKind: nil
            ),
            swarm: nil,
            subagents: [],
            parentConversationId: nil,
            subagentRole: nil,
            gitStatus: nil,
            changes: [],
            chapters: []
        )

        // 2) Long Multi-Turn Architecture Conversation (Turns, Chapters TOC, ContextMeter, Commit · #412, Changes, Revert)
        let yesterday = now.addingTimeInterval(-26 * 3600)
        let turnsMsgs: [Message] = [
            Self.userMsg(id: "c2-s0", step: 0, text: "先帮我梳理 iOS 端 Context Meter 上下文水位的接口契约，应该展示哪些核心指标？", timestamp: Self.iso(yesterday)),
            Self.toolMsg(id: "c2-s1", step: 1, tools: ["grep_search", "read_file"]),
            Self.agentMsg(id: "c2-s2", step: 2, model: "gemini-4-carbon-high", modelName: "Gemini 4 Carbon (High)", text: "建议在 `ContextMeterItem` 里统一暴露 `usedTokens / maxTokens` 占比、`tokensPerSecond` 输出速度与 `timeToFirstTokenSec` 首字延迟，让移动端顶部一眼看到窗口压力与生成性能。", timestamp: Self.iso(yesterday.addingTimeInterval(45))),
            Self.userMsg(id: "c2-s3", step: 3, text: "第二轮：如果上下文超过 90%，界面上怎么引导用户做压缩而不影响现有发送框？", timestamp: Self.iso(yesterday.addingTimeInterval(3600))),
            Self.toolMsg(id: "c2-s4", step: 4, tools: ["read_file", "find_by_name"]),
            Self.agentMsg(id: "c2-s5", step: 5, model: "claude-opus-5-5-high", modelName: "Claude Opus 5.5 (High)", text: "可以在输入栏左侧做成动态环形水位胶囊：80% 变琥珀色提示，92% 或出现 `context_overflow` 时弹出「点击 `/compact` 压缩上下文」一键救援按钮。", timestamp: Self.iso(yesterday.addingTimeInterval(3650))),
            Self.userMsg(id: "c2-s6", step: 6, text: "第三轮：长对话的轮次跳转和 AI 章节目录 TOC 怎么配合使用？", timestamp: Self.iso(now.addingTimeInterval(-3600))),
            Self.toolMsg(id: "c2-s7", step: 7, tools: ["read_file", "grep_search"]),
            Self.agentMsg(id: "c2-s8", step: 8, model: "claude-opus-5-5-high", modelName: "Claude Opus 5.5 (High)", text: "已接入 `GetChapterTitles`：左侧悬浮 **轮次/章节导轨 (`PromptJumpStepper`)** 支持快速上下切轮，点开导轨目录即可按 AI 总结的章节标题秒级定位！", timestamp: Self.iso(now.addingTimeInterval(-3540))),
            Self.userMsg(id: "c2-s9", step: 9, text: "第四轮：顺便把 52 周 Token 历史热力图与分享长图海报的数据结构核对一下。", timestamp: Self.iso(now.addingTimeInterval(-2400))),
            Self.toolMsg(id: "c2-s10", step: 10, tools: ["read_file", "write_to_file"]),
            Self.agentMsg(id: "c2-s11", step: 11, model: "gemini-3.8-flash-high", modelName: "Gemini 3.8 Flash (High)", text: "已对齐 `/api/v1/cockpit/tokens/history`：`daily_matrix` 覆盖过去 365 天，含 `input / output / cache_read / thinking / models` 明细；`summary` 含 `desktop_tokens / cli_tokens / estimated_cost_usd`。", timestamp: Self.iso(now.addingTimeInterval(-2320))),
            Self.userMsg(id: "c2-s12", step: 12, text: "第五轮：再确认一下 PR #412 的本地 Diff 和 Git 状态，准备提交前做最终核验。", timestamp: Self.iso(now.addingTimeInterval(-900))),
            Self.toolMsg(id: "c2-s13", step: 13, tools: ["write_to_file", "run_command"]),
            Self.agentMsg(id: "c2-s14", step: 14, model: "gemini-4-carbon-high", modelName: "Gemini 4 Carbon (High)", text: """
            已完成本轮实现并通过 `xcodebuild` 校验：

            - 新增 `ContextMeter` 环形水位 + 速度/首字延迟弹窗
            - 补齐 5 轮以上长对话 **章节 TOC** 与 **PromptJumpStepper** 快速跳转
            - 改动已关联 PR **#412**，可点击顶部 **`Changes`** 查看 3 个文件累计 Diff，或点击 **`Commit · #412`** 体验提交与推送流程，也可在消息菜单里体验 **撤回 / 导出 Markdown**。
            """, timestamp: Self.iso(now.addingTimeInterval(-840)))
        ]
        let c2Diffs = [
            DemoFileDiff(
                fileUri: "file:///Users/demo/Projects/multigravity/ios/Multigravity/Views/ContextMeterView.swift",
                fileName: "ContextMeterView.swift",
                actionType: "MODIFY",
                additions: 14,
                deletions: 3,
                diffLines: [
                    ("UNCHANGED", "    public var body: some View {"),
                    ("DELETE", "        Text(\"\\(Int(meter.percentage))%\")"),
                    ("INSERT", "        HStack(spacing: 6) {"),
                    ("INSERT", "            ProgressRing(progress: meter.percentage / 100.0)"),
                    ("INSERT", "            if let tps = meter.tokensPerSecond {"),
                    ("INSERT", "                Text(String(format: \"%.1f t/s\", tps))"),
                    ("INSERT", "            }"),
                    ("INSERT", "        }"),
                    ("UNCHANGED", "    }")
                ]
            ),
            DemoFileDiff(
                fileUri: "file:///Users/demo/Projects/multigravity/ios/Multigravity/Views/TokenHeatmapPosterView.swift",
                fileName: "TokenHeatmapPosterView.swift",
                actionType: "CREATE",
                additions: 42,
                deletions: 0,
                diffLines: [
                    ("INSERT", "public struct TokenHeatmapPosterView: View {"),
                    ("INSERT", "    public let history: TokenHistoryResponse"),
                    ("INSERT", "    public var body: some View {"),
                    ("INSERT", "        VStack(alignment: .leading, spacing: 12) {"),
                    ("INSERT", "            HeatmapGrid52Weeks(matrix: history.dailyMatrix)"),
                    ("INSERT", "        }"),
                    ("INSERT", "    }"),
                    ("INSERT", "}")
                ]
            ),
            DemoFileDiff(
                fileUri: "file:///Users/demo/Projects/multigravity/ios/Multigravity/ViewModels/ChatViewModel+Extras.swift",
                fileName: "ChatViewModel+Extras.swift",
                actionType: "MODIFY",
                additions: 8,
                deletions: 2,
                diffLines: [
                    ("UNCHANGED", "    public func refreshChapterTitles() {"),
                    ("DELETE", "        // old flat scroll"),
                    ("INSERT", "        guard !ranges.isEmpty else { return }"),
                    ("INSERT", "        // fetch AI-generated chapter TOC"),
                    ("UNCHANGED", "    }")
                ]
            )
        ]
        let c2 = Conversation(
            id: "demo-0002-turns",
            title: "多轮架构重构：上下文水位、TOC 与 PR #412",
            workspaceName: pMobile.name,
            workspaceUri: pMobile.workspaceUri,
            status: "CASCADE_RUN_STATUS_IDLE",
            duration: "24秒",
            messages: turnsMsgs,
            pendingInteraction: nil,
            canProceed: false,
            proceedArtifactUri: nil,
            runningTasks: [],
            queuedMessages: [],
            lastModified: now.addingTimeInterval(-12 * 60),
            unread: false,
            pinned: false,
            modelId: "gemini-4-carbon-high",
            modelName: "Gemini 4 Carbon (High)",
            startedAt: Self.iso(yesterday),
            contextMeter: ContextMeterItem(
                usedTokens: 156_800,
                maxTokens: 200_000,
                percentage: 78.4,
                inputTokens: 48_200,
                outputTokens: 9_640,
                cacheReadTokens: 95_800,
                thinkingTokens: 3_160,
                timeToFirstTokenSec: 0.78,
                streamingDurationSec: 4.8,
                tokensPerSecond: 52.8,
                isOverflow: false,
                errorKind: nil
            ),
            swarm: nil,
            subagents: [],
            parentConversationId: nil,
            subagentRole: nil,
            gitStatus: DemoGitState(
                repoName: "multigravity",
                branch: "feat/context-meter-ui",
                upstream: "origin/feat/context-meter-ui",
                ahead: 2,
                behind: 0,
                detached: false,
                files: [
                    ["path": "ios/Multigravity/Views/ContextMeterView.swift", "index": "M", "worktree": " ", "staged": true, "status": "MODIFIED"],
                    ["path": "ios/Multigravity/Views/TokenHeatmapPosterView.swift", "index": "A", "worktree": " ", "staged": true, "status": "ADDED"],
                    ["path": "ios/Multigravity/ViewModels/ChatViewModel+Extras.swift", "index": " ", "worktree": "M", "staged": false, "status": "MODIFIED"]
                ],
                githubTarget: ["host": "github.com", "owner": "hal9000", "repo": "multigravity", "branch": "feat/context-meter-ui"],
                pullRequests: [
                    [
                        "number": 412,
                        "title": "feat(ios): context meter, workflow rail & 52-week token heatmap",
                        "url": "https://github.com/hal9000/multigravity/pull/412",
                        "state": "OPEN",
                        "authorLogin": "hal9000",
                        "headRefName": "feat/context-meter-ui",
                        "baseRefName": "main"
                    ]
                ]
            ),
            changes: c2Diffs,
            chapters: [
                ["startStepIndex": 0, "endStepIndexExclusive": 3, "title": "梳理 Context Meter 接口指标", "generatedAt": Self.iso(now)],
                ["startStepIndex": 3, "endStepIndexExclusive": 6, "title": "设计 92% 水位告警与压缩引导", "generatedAt": Self.iso(now)],
                ["startStepIndex": 6, "endStepIndexExclusive": 9, "title": "对接长对话章节 TOC 与导轨", "generatedAt": Self.iso(now)],
                ["startStepIndex": 9, "endStepIndexExclusive": 12, "title": "对齐 52 周 Token 热力图结构", "generatedAt": Self.iso(now)],
                ["startStepIndex": 12, "endStepIndexExclusive": 15, "title": "核验 Diff 与 PR #412 提交状态", "generatedAt": Self.iso(now)]
            ]
        )

        // 3) Dynamic Multi-Agent Workflow + Clickable Subagents
        let subAudit: [String: Any] = [
            "conversationId": "demo-subagent-audit-1",
            "typeName": "research",
            "role": "安全与鉴权审计官",
            "prompt": "审计风控网关 Token 校验与并发锁边界",
            "modelTier": "pro",
            "stepIndex": 1,
            "status": "done",
            "stepCount": 6,
            "title": "安全与鉴权审计官"
        ]
        let subPerf: [String: Any] = [
            "conversationId": "demo-subagent-perf-2",
            "typeName": "research",
            "role": "并发压测子代理",
            "prompt": "验证 10k QPS 下限流器 P99 抖动",
            "modelTier": "flash",
            "stepIndex": 1,
            "status": "done",
            "stepCount": 4,
            "title": "并发压测子代理"
        ]
        let subLive: [String: Any] = [
            "conversationId": "demo-subagent-live-3",
            "typeName": "research",
            "role": "竞态复现观察员",
            "prompt": "持续观测异步回调顺序与幂等校验",
            "modelTier": "flash",
            "stepIndex": 2,
            "status": "running",
            "stepCount": 3,
            "title": "竞态复现观察员"
        ]
        let c3Workflow: [String: Any] = [
            "description": "多智能体并行审计：安全鉴权 × 性能压测 × 结论收敛",
            "scriptPath": "workflows/risk_audit_pipeline.py",
            "script": "parallel(\n  agent('security_auditor', ...),\n  agent('perf_bench', ...)\n)\nfinish(summary=aggregate())",
            "phases": [
                [
                    "name": "阶段 1 · 并发安全与性能采集",
                    "status": "done",
                    "logs": [
                        "[00:00.3] parallel fan-out: 2 subagents started",
                        "[00:02.4] security_auditor -> 1 low-severity header issue",
                        "[00:03.1] perf_bench -> P99=11.4ms @ 10,000 QPS"
                    ],
                    "subagents": [subAudit, subPerf]
                ],
                [
                    "name": "阶段 2 · 补丁验证与汇总收敛",
                    "status": "done",
                    "logs": [
                        "[00:04.2] applied idempotency guard & verified tests",
                        "[00:05.0] emitted structured finish contract"
                    ],
                    "subagents": [subLive]
                ]
            ],
            "logs": [
                "run_workflow started with max_workers=4",
                "Phase 1 converged (3.1s)",
                "Phase 2 converged (1.9s)"
            ],
            "output": "安全与压测双通道验证通过；P99 延迟 11.4ms，并发幂等锁已加固。",
            "resultJson": "{\"passed\":true,\"p99_ms\":11.4,\"issues_fixed\":1,\"subagents\":3}",
            "status": "done",
            "duration": "5.0s"
        ]
        let c3 = Conversation(
            id: "demo-0003-workflow",
            title: "动态多智能体工作流与子代理拓扑",
            workspaceName: pRisk.name,
            workspaceUri: pRisk.workspaceUri,
            status: "CASCADE_RUN_STATUS_IDLE",
            duration: "5秒",
            messages: [
                Self.userMsg(
                    id: "c3-m0",
                    step: 0,
                    text: "/workflow 请使用 run_workflow 并发对风控核心网关进行安全鉴权与高并发压测审计。",
                    timestamp: Self.iso(now.addingTimeInterval(-18 * 60)),
                    slashCommands: [["name": "workflow", "icon": "graph_2", "kind": "system"]]
                ),
                Message(
                    id: "c3-m1",
                    type: "workflow",
                    text: "",
                    toolCount: 1,
                    toolNames: ["run_workflow"],
                    stepIndex: 1,
                    workflow: c3Workflow
                ),
                Self.subagentMsg(
                    id: "c3-m2",
                    step: 2,
                    conversationId: "demo-subagent-live-3",
                    role: "竞态复现观察员",
                    prompt: "持续观测异步回调顺序与幂等校验",
                    modelTier: "flash",
                    status: "running",
                    stepCount: 3
                ),
                Self.agentMsg(
                    id: "c3-m3",
                    step: 3,
                    model: "gemini-4-carbon-high",
                    modelName: "Gemini 4 Carbon (High)",
                    text: """
                    工作流编排已完成 **2 个阶段** 并汇聚了各 Sub-Agent 结论：

                    - 点开上方 **工作流拓扑卡片** 可切换阶段、查看实时日志与结构化 JSON 输出
                    - 点击任意 **子代理卡片** 可深入查看只读子代理会话详情；顶部状态栏还保留了 1 个运行中观察员可供测试 **手动停止**。
                    """,
                    timestamp: Self.iso(now.addingTimeInterval(-17 * 60))
                )
            ],
            pendingInteraction: nil,
            canProceed: false,
            proceedArtifactUri: nil,
            runningTasks: [],
            queuedMessages: [],
            lastModified: now.addingTimeInterval(-16 * 60),
            unread: false,
            pinned: false,
            modelId: "gemini-4-carbon-high",
            modelName: "Gemini 4 Carbon (High)",
            startedAt: Self.iso(now.addingTimeInterval(-19 * 60)),
            contextMeter: ContextMeterItem(
                usedTokens: 64_400,
                maxTokens: 200_000,
                percentage: 32.2,
                inputTokens: 21_000,
                outputTokens: 3_120,
                cacheReadTokens: 39_100,
                thinkingTokens: 1_180,
                timeToFirstTokenSec: 0.51,
                streamingDurationSec: 5.0,
                tokensPerSecond: 61.5,
                isOverflow: false,
                errorKind: nil
            ),
            swarm: nil,
            subagents: [subLive],
            parentConversationId: nil,
            subagentRole: nil,
            gitStatus: nil,
            changes: [],
            chapters: []
        )

        // Hidden readonly child subagent conversations (tap from demo-0003-workflow!)
        let childAudit = Conversation(
            id: "demo-subagent-audit-1",
            title: "安全与鉴权审计官",
            workspaceName: "子代理",
            workspaceUri: pRisk.workspaceUri,
            status: "CASCADE_RUN_STATUS_IDLE",
            duration: "2秒",
            messages: [
                Self.userMsg(id: "sa1-m0", step: 0, text: "审计风控网关 Token 校验与并发锁边界", timestamp: Self.iso(now.addingTimeInterval(-18 * 60))),
                Self.toolMsg(id: "sa1-m1", step: 1, tools: ["read_file", "grep_search"]),
                Self.agentMsg(id: "sa1-m2", step: 2, model: "claude-opus-5-5-high", modelName: "Claude Opus 5.5 (High)", text: "发现 `NonceCache` 在极端并发窗口存在微秒级竞态，建议改为 `sync/atomic` + 分片锁，已输出补丁建议。")
            ],
            pendingInteraction: nil,
            canProceed: false,
            proceedArtifactUri: nil,
            runningTasks: [],
            queuedMessages: [],
            lastModified: now.addingTimeInterval(-17 * 60),
            unread: false,
            pinned: false,
            modelId: "claude-opus-5-5-high",
            modelName: "Claude Opus 5.5 (High)",
            startedAt: Self.iso(now.addingTimeInterval(-18 * 60)),
            contextMeter: nil,
            swarm: nil,
            subagents: [],
            parentConversationId: "demo-0003-workflow",
            subagentRole: "安全与鉴权审计官",
            gitStatus: nil,
            changes: [],
            chapters: []
        )
        let childPerf = Conversation(
            id: "demo-subagent-perf-2",
            title: "并发压测子代理",
            workspaceName: "子代理",
            workspaceUri: pRisk.workspaceUri,
            status: "CASCADE_RUN_STATUS_IDLE",
            duration: "3秒",
            messages: [
                Self.userMsg(id: "sa2-m0", step: 0, text: "验证 10k QPS 下限流器 P99 抖动", timestamp: Self.iso(now.addingTimeInterval(-18 * 60))),
                Self.toolMsg(id: "sa2-m1", step: 1, tools: ["run_command"]),
                Self.agentMsg(id: "sa2-m2", step: 2, model: "gemini-3.8-flash-high", modelName: "Gemini 3.8 Flash (High)", text: "连续压测 60 秒结果：吞吐 10,420 QPS，P50 = 3.2ms，P99 = 11.4ms，零超时请求。")
            ],
            pendingInteraction: nil,
            canProceed: false,
            proceedArtifactUri: nil,
            runningTasks: [],
            queuedMessages: [],
            lastModified: now.addingTimeInterval(-17 * 60),
            unread: false,
            pinned: false,
            modelId: "gemini-3.8-flash-high",
            modelName: "Gemini 3.8 Flash (High)",
            startedAt: Self.iso(now.addingTimeInterval(-18 * 60)),
            contextMeter: nil,
            swarm: nil,
            subagents: [],
            parentConversationId: "demo-0003-workflow",
            subagentRole: "并发压测子代理",
            gitStatus: nil,
            changes: [],
            chapters: []
        )
        let childLive = Conversation(
            id: "demo-subagent-live-3",
            title: "竞态复现观察员",
            workspaceName: "子代理",
            workspaceUri: pRisk.workspaceUri,
            status: "CASCADE_RUN_STATUS_RUNNING",
            duration: "运行中",
            messages: [
                Self.userMsg(id: "sa3-m0", step: 0, text: "持续观测异步回调顺序与幂等校验", timestamp: Self.iso(now.addingTimeInterval(-17 * 60))),
                Self.toolMsg(id: "sa3-m1", step: 1, tools: ["run_command"]),
                Self.agentMsg(id: "sa3-m2", step: 2, model: "gemini-3.8-flash-high", modelName: "Gemini 3.8 Flash (High)", text: "正在第 3 轮幂等回放采样（目前 0 例乱序异常）...")
            ],
            pendingInteraction: nil,
            canProceed: false,
            proceedArtifactUri: nil,
            runningTasks: [],
            queuedMessages: [],
            lastModified: now.addingTimeInterval(-16 * 60),
            unread: false,
            pinned: false,
            modelId: "gemini-3.8-flash-high",
            modelName: "Gemini 3.8 Flash (High)",
            startedAt: Self.iso(now.addingTimeInterval(-17 * 60)),
            contextMeter: nil,
            swarm: nil,
            subagents: [],
            parentConversationId: "demo-0003-workflow",
            subagentRole: "竞态复现观察员",
            gitStatus: nil,
            changes: [],
            chapters: []
        )

        // 4) Swarm Collaboration Conversation (Banner + 4-tab SwarmSheet)
        let swarmBriefDict: [String: Any] = [
            "active": true,
            "coordinatorId": "demo-0004-swarm",
            "title": "重构多端状态同步蜂群",
            "phase": "Stage 2 · 联调验证",
            "headline": "coordinator 调度 scanner / fixer / tester 并行推进中",
            "workingCount": 2,
            "totalMembers": 4,
            "latestSeq": 6,
            "attentionCount": 1
        ]
        let swarmSnapshotDict: [String: Any] = [
            "cascadeId": "demo-0004-swarm",
            "coordinatorId": "demo-0004-swarm",
            "title": "重构多端状态同步蜂群",
            "etag": "demo-swarm-v6",
            "brief": swarmBriefDict,
            "members": [
                [
                    "name": "coordinator",
                    "conversationId": "demo-0004-swarm",
                    "model": "Orchestrator Carbon",
                    "role": "总协调官",
                    "wakeMode": "event",
                    "lastReadSeq": 6,
                    "lastNudgedSeq": 0,
                    "killed": false,
                    "status": "working",
                    "lastActivity": "正在分配 Stage 2 验收任务",
                    "unreadCount": 0,
                    "isCurrent": true
                ],
                [
                    "name": "scanner",
                    "conversationId": "demo-subagent-audit-1",
                    "model": "Gemini 3.8 Flash",
                    "role": "依赖与边界扫描员",
                    "wakeMode": "inherit",
                    "lastReadSeq": 5,
                    "lastNudgedSeq": 0,
                    "killed": false,
                    "status": "waiting",
                    "waitSince": Self.iso(now.addingTimeInterval(-180)),
                    "lastActivity": "已完成模块调用图扫描",
                    "unreadCount": 1,
                    "isCurrent": false
                ],
                [
                    "name": "fixer",
                    "conversationId": "demo-subagent-perf-2",
                    "model": "Claude Opus 5.5",
                    "role": "独立 Worktree 补丁作者",
                    "wakeMode": "share",
                    "lastReadSeq": 6,
                    "lastNudgedSeq": 0,
                    "killed": false,
                    "status": "working",
                    "lastActivity": "正在修复 ETag 条件竞态",
                    "unreadCount": 0,
                    "isCurrent": false
                ],
                [
                    "name": "reviewer",
                    "conversationId": "demo-subagent-live-3",
                    "model": "Gemini 4 Argon",
                    "role": "回归验证员",
                    "wakeMode": "inherit",
                    "lastReadSeq": 4,
                    "lastNudgedSeq": 0,
                    "killed": true,
                    "status": "killed",
                    "lastActivity": "初版巡检完成已归档",
                    "unreadCount": 0,
                    "isCurrent": false
                ]
            ],
            "messages": [
                ["seq": 1, "agent": "coordinator", "text": "@scanner @fixer 本次任务拆分为依赖扫描与 ETag 竞态修复两路并行推进。", "time": Self.iso(now.addingTimeInterval(-900)), "channel": "board", "mentions": ["scanner", "fixer"]],
                ["seq": 2, "agent": "scanner", "text": "已完成 14 个视图与 ViewModel 的数据流梳理，发现 `refreshActionAvailability` 存在重复探测。", "time": Self.iso(now.addingTimeInterval(-720)), "channel": "board", "mentions": []],
                ["seq": 3, "agent": "fixer", "text": "@coordinator 已在独立 share 工作区修改完增量合并逻辑，准备跑单元测试。", "time": Self.iso(now.addingTimeInterval(-480)), "channel": "board", "mentions": ["coordinator"]],
                ["seq": 4, "agent": "reviewer", "text": "第一阶段 Diff 复核通过，无多余锁嵌套。", "time": Self.iso(now.addingTimeInterval(-360)), "channel": "review", "mentions": []],
                ["seq": 5, "agent": "coordinator", "text": "@fixer 补充一条断网重连的边界回归断言即可收敛。", "time": Self.iso(now.addingTimeInterval(-180)), "channel": "board", "mentions": ["fixer"]],
                ["seq": 6, "agent": "fixer", "text": "收到，正在运行最终构建核验，完成后回报看板。", "time": Self.iso(now.addingTimeInterval(-90)), "channel": "board", "mentions": []]
            ],
            "view": [
                "version": "1",
                "as_of_seq": 6,
                "updated_at": Self.iso(now.addingTimeInterval(-90)),
                "headline": "双工作流并发推进中，等待 fixer 提交终审验证",
                "phase": ["label": "联调与回归测试", "index": 2, "total": 3],
                "attention": [
                    ["id": "att-1", "kind": "decision", "severity": "med", "title": "确认是否将扫描员结果合入主分支", "why": "scanner 处于等待态（waiting），可在协作板发送反馈或轻推唤醒。"]
                ],
                "workstreams": [
                    ["id": "ws-1", "title": "状态同步与防抖合并", "owners": ["fixer"], "state": "active", "note": "正在跑竞态回归"],
                    ["id": "ws-2", "title": "依赖边界静态扫描", "owners": ["scanner"], "state": "done", "note": "14 处引用已校验"]
                ],
                "milestones": [
                    ["id": "ms-1", "text": "完成全部 ViewModel 数据流标注"],
                    ["id": "ms-2", "text": "通过 ETag 与 WebSocket 断线重连回归"]
                ],
                "risks": [
                    ["id": "rk-1", "text": "需关注高频率轮询与心跳重连时的内存抖动"]
                ]
            ]
        ]
        let c4 = Conversation(
            id: "demo-0004-swarm",
            title: "蜂群协同看板：多端状态同步架构升级",
            workspaceName: pMobile.name,
            workspaceUri: pMobile.workspaceUri,
            status: "CASCADE_RUN_STATUS_IDLE",
            duration: "14秒",
            messages: [
                Self.userMsg(
                    id: "c4-m0",
                    step: 0,
                    text: "/teamwork 组建 Swarm 蜂群并发完成状态同步边界扫描、增量补丁修复与交叉评审。",
                    timestamp: Self.iso(now.addingTimeInterval(-960)),
                    slashCommands: [["name": "teamwork", "icon": "graph_2", "kind": "system"]]
                ),
                Self.toolMsg(id: "c4-m1", step: 1, tools: ["swarm", "read_file"]),
                Self.agentMsg(
                    id: "c4-m2",
                    step: 2,
                    model: "gemini-pro-orchestrator",
                    modelName: "Orchestrator Carbon",
                    text: """
                    已拉起 **4 成员协作蜂群（Swarm）** 并同步至顶部横幅：

                    - 点击上方 **蜂群协作横幅** 打开完整 4 标签页看板（**概览 / 协作板 / 成员 / 拓扑**）
                    - 支持在看板发送实时 **协作反馈**、点击 **轻推（Nudge）** 唤醒等待中的 Worker，以及点击节点跳转对应成员会话！
                    """,
                    timestamp: Self.iso(now.addingTimeInterval(-120))
                )
            ],
            pendingInteraction: nil,
            canProceed: false,
            proceedArtifactUri: nil,
            runningTasks: [],
            queuedMessages: [],
            lastModified: now.addingTimeInterval(-22 * 60),
            unread: false,
            pinned: false,
            modelId: "gemini-pro-orchestrator",
            modelName: "Orchestrator Carbon",
            startedAt: Self.iso(now.addingTimeInterval(-960)),
            contextMeter: ContextMeterItem(
                usedTokens: 54_000,
                maxTokens: 200_000,
                percentage: 27.0,
                inputTokens: 18_000,
                outputTokens: 2_890,
                cacheReadTokens: 31_900,
                thinkingTokens: 1_210,
                timeToFirstTokenSec: 0.69,
                streamingDurationSec: 3.9,
                tokensPerSecond: 57.4,
                isOverflow: false,
                errorKind: nil
            ),
            swarm: swarmSnapshotDict,
            subagents: [],
            parentConversationId: nil,
            subagentRole: nil,
            gitStatus: DemoGitState(
                repoName: "multigravity",
                branch: "swarm/worker-fixer",
                upstream: "origin/swarm/worker-fixer",
                ahead: 1,
                behind: 0,
                detached: false,
                files: [],
                githubTarget: ["host": "github.com", "owner": "hal9000", "repo": "multigravity", "branch": "swarm/worker-fixer"],
                pullRequests: [
                    [
                        "number": 418,
                        "title": "wip(swarm): sync etag merge & backoff",
                        "url": "https://github.com/hal9000/multigravity/pull/418",
                        "state": "DRAFT",
                        "authorLogin": "hal9000",
                        "headRefName": "swarm/worker-fixer",
                        "baseRefName": "main"
                    ]
                ]
            ),
            changes: [],
            chapters: []
        )

        // 5) Pending Workflow Approval (.action)
        let wfApprovalInteraction: [String: Any] = [
            "type": "workflow_approval",
            "trajectoryId": "demo-0005-wf-approval",
            "stepIndex": 2,
            "title": "审批动态多智能体工作流执行",
            "description": "即将并发编排生产环境灰度发布脚本（含 Docker 镜像构建、10% 切流与 18 项冒烟检查）。",
            "workflowApproval": [
                "description": "灰度发布与冒烟验证流水线（Max Concurrency = 4）",
                "scriptPath": "deploy/canary_rollout.py",
                "script": "phase('build', lambda: run('docker buildx build --push ...'))\nparallel([\n  agent('canary-deployer', role='10%切流'),\n  agent('smoke-verifier', role='核心接口冒烟')\n])",
                "estimatedPhases": ["构建镜像与推送", "灰度发布 10% 流量", "冒烟验证"],
                "maxConcurrency": 4
            ],
            "options": [
                ["id": "approve", "label": "批准并运行工作流", "description": "立即启动 3 阶段编排流水线"],
                ["id": "reject", "label": "暂不执行", "description": "保持现有线上环境不变"]
            ]
        ]
        let c5 = Conversation(
            id: "demo-0005-wf-approval",
            title: "待审批：生产灰度发布工作流（Workflow Approval）",
            workspaceName: pDeploy.name,
            workspaceUri: pDeploy.workspaceUri,
            status: "CASCADE_RUN_STATUS_RUNNING",
            duration: "2秒",
            messages: [
                Self.userMsg(id: "c5-m0", step: 0, text: "准备发布 v2.35.0 灰度版本，先通过动态工作流编排一下发布与冒烟验证流程。", timestamp: Self.iso(now.addingTimeInterval(-35 * 60))),
                Self.toolMsg(id: "c5-m1", step: 1, tools: ["read_file", "run_workflow"])
            ],
            pendingInteraction: wfApprovalInteraction,
            canProceed: false,
            proceedArtifactUri: nil,
            runningTasks: [],
            queuedMessages: [],
            lastModified: now.addingTimeInterval(-34 * 60),
            unread: false,
            pinned: false,
            modelId: "claude-opus-5-5-high",
            modelName: "Claude Opus 5.5 (High)",
            startedAt: Self.iso(now.addingTimeInterval(-35 * 60)),
            contextMeter: ContextMeterItem(usedTokens: 29_400, maxTokens: 200_000, percentage: 14.7, inputTokens: 11_200, outputTokens: 980, cacheReadTokens: 16_800, thinkingTokens: 420, timeToFirstTokenSec: 0.72, streamingDurationSec: 2.0, tokensPerSecond: 44.8, isOverflow: false, errorKind: nil),
            swarm: nil,
            subagents: [],
            parentConversationId: nil,
            subagentRole: nil,
            gitStatus: nil,
            changes: [],
            chapters: []
        )

        // 6) Multi-Question Interactive Confirmation (.action)
        let actionInteraction: [String: Any] = [
            "type": "user_question",
            "trajectoryId": "demo-0006-action",
            "stepIndex": 2,
            "title": "请确认接口改造范围与验收选项",
            "description": "演示模式内置交互卡：支持单选、多选与自定义补充说明。",
            "questions": [
                [
                    "header": "改造阶段（多选）",
                    "question": "本次优先落地的网关增强能力有哪些？",
                    "isMultiSelect": true,
                    "hasWriteIn": true,
                    "writeInPlaceholder": "可补充其他具体模块...",
                    "options": [
                        ["id": "opt-metrics", "label": "Token 与速度遥测", "description": "采集 TTFT 与输出吞吐 t/s"],
                        ["id": "opt-chapters", "label": "AI 章节目录 TOC", "description": "为长对话生成阶段书签"],
                        ["id": "opt-git", "label": "一键 Git 提交与 PR 联动", "description": "直接查看未提交 Diff 并推送"]
                    ]
                ],
                [
                    "header": "兼容性策略（单选）",
                    "question": "旧版本客户端接口如何兼容？",
                    "isMultiSelect": false,
                    "options": [
                        ["id": "opt-dual", "label": "双字段平滑兼容（推荐）", "description": "保留旧字段一个大版本再下线"],
                        ["id": "opt-strict", "label": "仅保留新协议字段", "description": "强制客户端跟随升级"]
                    ]
                ]
            ]
        ]
        let c6 = Conversation(
            id: "demo-0006-action",
            title: "待选择：多题交互确认卡（Action）",
            workspaceName: pMobile.name,
            workspaceUri: pMobile.workspaceUri,
            status: "CASCADE_RUN_STATUS_RUNNING",
            duration: "2秒",
            messages: [
                Self.userMsg(id: "c6-m0", step: 0, text: "/grill-me 帮我对齐移动端下一批特性的交互决策项。", timestamp: Self.iso(now.addingTimeInterval(-50 * 60)), slashCommands: [["name": "grill-me", "icon": "forum", "kind": "system"]]),
                Self.toolMsg(id: "c6-m1", step: 1, tools: ["read_file", "grep_search"])
            ],
            pendingInteraction: actionInteraction,
            canProceed: false,
            proceedArtifactUri: nil,
            runningTasks: [],
            queuedMessages: [],
            lastModified: now.addingTimeInterval(-48 * 60),
            unread: false,
            pinned: false,
            modelId: "gemini-4-argon-high",
            modelName: "Gemini 4 Argon (High)",
            startedAt: Self.iso(now.addingTimeInterval(-50 * 60)),
            contextMeter: nil,
            swarm: nil,
            subagents: [],
            parentConversationId: nil,
            subagentRole: nil,
            gitStatus: nil,
            changes: [],
            chapters: []
        )

        // 7) Plan Proceed + Mermaid + Draft Tag
        let c7 = Conversation(
            id: "demo-0007-plan",
            title: "计划待确认：数据看板架构重构（含草稿）",
            workspaceName: pAnalytics.name,
            workspaceUri: pAnalytics.workspaceUri,
            status: "CASCADE_RUN_STATUS_IDLE",
            duration: "3秒",
            messages: [
                Self.userMsg(id: "c7-m0", step: 0, text: "/plan 设计一个支持离线缓存与增量更新的实时报表架构。", timestamp: Self.iso(now.addingTimeInterval(-2 * 3600)), slashCommands: [["name": "plan", "icon": "ballot", "kind": "system"]]),
                Self.toolMsg(id: "c7-m1", step: 1, tools: ["find_by_name", "write_to_file"]),
                Self.agentMsg(
                    id: "c7-m2",
                    step: 2,
                    model: "claude-sonnet-5-5-high",
                    modelName: "Claude Sonnet 5.5 (High)",
                    text: "我已拟定完整实施方案（含 Mermaid 架构流向图），请审阅 **[plan.md](/api/v1/files/raw?path=plan.md)**，确认无误后点击下方 **继续执行（Proceed）** 开始落地代码。",
                    artifacts: [["uri": "plan.md", "title": "实时报表架构实施计划", "requestFeedback": true, "userFacing": true]]
                )
            ],
            pendingInteraction: nil,
            canProceed: true,
            proceedArtifactUri: "plan.md",
            runningTasks: [],
            queuedMessages: [],
            lastModified: now.addingTimeInterval(-95 * 60),
            unread: false,
            pinned: false,
            modelId: "claude-sonnet-5-5-high",
            modelName: "Claude Sonnet 5.5 (High)",
            startedAt: Self.iso(now.addingTimeInterval(-2 * 3600)),
            contextMeter: nil,
            swarm: nil,
            subagents: [],
            parentConversationId: nil,
            subagentRole: nil,
            gitStatus: nil,
            changes: [],
            chapters: []
        )

        // 8) Running Background Tasks (Daemon + Command + Live Log Tail + Global Task Capsule)
        let c8 = Conversation(
            id: "demo-0008-running",
            title: "压测联调服务与守护进程（可看日志）",
            workspaceName: pDeploy.name,
            workspaceUri: pDeploy.workspaceUri,
            status: "CASCADE_RUN_STATUS_RUNNING",
            duration: "运行中",
            messages: [
                Self.userMsg(id: "c8-m0", step: 0, text: "在后台同时拉起本地控制台守护进程和 Go 竞态集成测试，实时把日志接进来。", timestamp: Self.iso(now.addingTimeInterval(-4 * 60))),
                Self.toolMsg(id: "c8-m1", step: 1, tools: ["run_command", "run_command"]),
                Self.agentMsg(id: "c8-m2", step: 2, model: "gemini-3.8-flash-high", modelName: "Gemini 3.8 Flash (High)", text: "已将 `vite` 常驻进程与 `go test -race` 注册至全局后台任务中心！点击右上角 **任务 / 工具图标** 或首页后台胶囊，可直接展开终端尾部日志或一键停止任务。")
            ],
            pendingInteraction: nil,
            canProceed: false,
            proceedArtifactUri: nil,
            runningTasks: [
                ["taskId": "task-vite-daemon", "toolName": "run_command", "commandLine": "npm run dev -- --host 0.0.0.0 --port 5173", "isDaemon": true, "stepIndex": 1],
                ["taskId": "task-go-race", "toolName": "run_command", "commandLine": "go test -race -count=1 ./internal/proxy ./internal/cockpit", "isDaemon": false, "stepIndex": 2]
            ],
            queuedMessages: [],
            lastModified: now.addingTimeInterval(-3 * 60),
            unread: false,
            pinned: false,
            modelId: "gemini-3.8-flash-high",
            modelName: "Gemini 3.8 Flash (High)",
            startedAt: Self.iso(now.addingTimeInterval(-4 * 60)),
            contextMeter: nil,
            swarm: nil,
            subagents: [],
            parentConversationId: nil,
            subagentRole: nil,
            gitStatus: nil,
            changes: [],
            chapters: []
        )

        // 9) Context Overflow Error + /compact Rescue + Slash Pills
        let c9 = Conversation(
            id: "demo-0009-overflow",
            title: "异常演示：上下文接近上限（体验 /compact）",
            workspaceName: pRisk.name,
            workspaceUri: pRisk.workspaceUri,
            status: "CASCADE_RUN_STATUS_ERROR",
            duration: "6秒",
            messages: [
                Self.userMsg(
                    id: "c9-m0",
                    step: 0,
                    text: "/grill-me /jiuge-market-analysis 全量解析近 6 个月风控拒付原始日志堆栈并定位异常波峰。",
                    timestamp: Self.iso(now.addingTimeInterval(-3 * 3600)),
                    slashCommands: [
                        ["name": "grill-me", "icon": "forum", "kind": "system"],
                        ["name": "jiuge-market-analysis", "icon": "lightbulb_2", "kind": "skill"]
                    ]
                ),
                Self.toolMsg(id: "c9-m1", step: 1, tools: ["read_file", "grep_search", "run_command"]),
                Self.errorMsg(
                    id: "c9-m2",
                    step: 2,
                    text: "上下文窗口已达到 96.2%（192.4K / 200K tokens），继续注入全量日志触发了 context_overflow 保护。可点击底部或 `/compact` 压缩历史后恢复！",
                    attempts: 3,
                    max: 3,
                    errorKind: "context_overflow",
                    isOverflow: true
                )
            ],
            pendingInteraction: nil,
            canProceed: false,
            proceedArtifactUri: nil,
            runningTasks: [],
            queuedMessages: [],
            lastModified: now.addingTimeInterval(-3 * 3600),
            unread: false,
            pinned: false,
            modelId: "claude-opus-5-5-high",
            modelName: "Claude Opus 5.5 (High)",
            startedAt: Self.iso(now.addingTimeInterval(-3 * 3600)),
            contextMeter: ContextMeterItem(
                usedTokens: 192_400,
                maxTokens: 200_000,
                percentage: 96.2,
                inputTokens: 141_200,
                outputTokens: 6_800,
                cacheReadTokens: 42_100,
                thinkingTokens: 2_300,
                timeToFirstTokenSec: 1.42,
                streamingDurationSec: 5.8,
                tokensPerSecond: 38.4,
                isOverflow: true,
                errorKind: "context_overflow"
            ),
            swarm: nil,
            subagents: [],
            parentConversationId: nil,
            subagentRole: nil,
            gitStatus: nil,
            changes: [],
            chapters: []
        )

        // 10) Unread Conversation + Jupyter Notebook (.ipynb) + Math/Markdown Export
        let c10 = Conversation(
            id: "demo-0010-notebook",
            title: "未读：Notebook 实验与长公式导出（Unread）",
            workspaceName: "Chat",
            workspaceUri: pChat.workspaceUri,
            status: "CASCADE_RUN_STATUS_IDLE",
            duration: "4秒",
            messages: [
                Self.userMsg(id: "c10-m0", step: 0, text: "帮我整理一份包含代码单元、表格和公式的策略回测 Notebook，方便手机查看和导出 Markdown。", timestamp: Self.iso(now.addingTimeInterval(-5 * 3600))),
                Self.agentMsg(
                    id: "c10-m1",
                    step: 1,
                    model: "gemini-4-hydrogen-high",
                    modelName: "Gemini 4 Hydrogen (High)",
                    text: """
                    已生成交互式实验记录 **[analytics.ipynb](/api/v1/files/raw?path=analytics.ipynb)**：

                    ### 核心回测指标对比

                    | 策略分支 | 年化收益 | 最大回撤 | 夏普比率 |
                    | :--- | ---: | ---: | ---: |
                    | Baseline 等权 | 11.4% | -14.2% | 0.92 |
                    | 波动率倒数加权 | **16.8%** | **-7.6%** | **1.84** |

                    组合夏普比定义：$$S_p = \\frac{E[R_p - R_f]}{\\sigma_p}$$

                    可点击任意消息菜单中的 **导出为 Markdown** 体验完整分享导出！
                    """,
                    artifacts: [["uri": "analytics.ipynb", "title": "波动率倒数回测 Notebook"]]
                )
            ],
            pendingInteraction: nil,
            canProceed: false,
            proceedArtifactUri: nil,
            runningTasks: [],
            queuedMessages: [],
            lastModified: now.addingTimeInterval(-5 * 3600),
            unread: true,
            pinned: false,
            modelId: "gemini-4-hydrogen-high",
            modelName: "Gemini 4 Hydrogen (High)",
            startedAt: Self.iso(now.addingTimeInterval(-5 * 3600)),
            contextMeter: ContextMeterItem(
                usedTokens: 21_500,
                maxTokens: 200_000,
                percentage: 10.75,
                inputTokens: 6_200,
                outputTokens: 1_840,
                cacheReadTokens: 13_100,
                thinkingTokens: 360,
                timeToFirstTokenSec: 0.39,
                streamingDurationSec: 2.2,
                tokensPerSecond: 78.6,
                isOverflow: false,
                errorKind: nil
            ),
            swarm: nil,
            subagents: [],
            parentConversationId: nil,
            subagentRole: nil,
            gitStatus: nil,
            changes: [],
            chapters: []
        )

        conversations = [c1, c2, c3, c4, c5, c6, c7, c8, c9, c10, childAudit, childPerf, childLive]
    }
}

// MARK: - Sample Assets & Drafts

extension DemoGateway {
    static let draftedConversationId = "demo-0007-plan"
    static let pinnedConversationId = "demo-0001-upload"

    /// Seeds an unsent follow-up typed into an existing conversation so the list shows the DRAFT tag.
    static func seedDrafts() {
        CacheManager.shared.saveDraft(key: draftedConversationId, text: "顺便补充 SQLite WAL 模式的并发写入限流说明")
        CacheManager.shared.setPinned(cascadeId: pinnedConversationId, pinned: true)
    }

    static let salesCSV = """
    region,category,q2_revenue,q3_revenue,growth_pct,orders
    华东,企业服务,148200,204300,37.9,312
    华南,企业服务,98500,136200,38.3,218
    华北,消费硬件,79400,92400,16.4,154
    西南,渠道分销,46100,53500,16.1,97
    合计,全部品类,372200,486400,30.7,781
    """

    static let meetingNotes = """
    # 三季度经营复盘会议纪要

    - **时间**：周四 14:00 – 15:10
    - **参会人**：产品、数据、华东/华南业务负责人

    ## 核心结论
    1. 华东与华南区 Q3 企业服务续费率提升至 **91.4%**，带动整体同比 +30.7%。
    2. 西南大区渠道新签节奏略慢，下周二前完成政策调整方案。

    ## 待办事项
    - [x] 输出大区对比看板与 PDF 复盘报告
    - [ ] 财务核对 10 月市场推广预算（@市场部）
    """

    static let planMarkdown = """
    # 实时数据看板离线缓存与增量更新方案

    > 目标：让移动端在弱网或离线状态下毫秒级首屏渲染，同时支持增量 Merge 与多端同步。

    ## 1. 数据流架构

    ```mermaid
    flowchart LR
        A["网关 /gateway/events"] -->|WebSocket Heartbeat| B["ConversationEventsClient"]
        B -->|增量变更事件| C["本地 SQLite + CacheManager"]
        C -->|乐观合并| D["SwiftUI @Observable ViewModel"]
        D -->|Context / Swarm| E["UI 实时状态卡片"]
    ```

    ## 2. 落地里程碑
    - **M1**：引入 ETag 与本地快照持久化，降低重复全量拉取流量 75%。
    - **M2**：补齐弱网下操作队列与幂等重发 (`clientMessageId`)。
    - **M3**：上线 52 周 Token 历史热力图与每日模型消耗穿透明细。
    """

    static let notebookJSON = #"""
    {
      "cells": [
        {
          "cell_type": "markdown",
          "metadata": {},
          "source": [
            "# 波动率倒数加权策略实验 (Demo Notebook)\n",
            "\n",
            "本 Notebook 用于演示移动端内嵌 `.ipynb` 单元格、表格及计算输出渲染。"
          ]
        },
        {
          "cell_type": "code",
          "execution_count": 1,
          "metadata": {},
          "outputs": [
            {
              "name": "stdout",
              "output_type": "stream",
              "text": [
                "Sharpe Ratio = 1.84 | Max Drawdown = -7.6%\n",
                "Weights: {'沪深300': 0.28, '中证红利': 0.42, '黄金ETF': 0.30}\n"
              ]
            }
          ],
          "source": [
            "import numpy as np\n",
            "vols = np.array([0.18, 0.12, 0.17])\n",
            "weights = (1 / vols) / np.sum(1 / vols)\n",
            "print('Weights:', np.round(weights, 2))"
          ]
        }
      ],
      "metadata": {
        "kernelspec": { "display_name": "Python 3", "language": "python", "name": "python3" }
      },
      "nbformat": 4,
      "nbformat_minor": 5
    }
    """#

    /// Programmatic 640x400 bar chart PNG showing Q2 vs Q3 regional revenue.
    static func salesChartPNG() -> Data {
        let size = CGSize(width: 640, height: 400)
        let format = UIGraphicsImageRendererFormat()
        format.scale = 2
        let renderer = UIGraphicsImageRenderer(size: size, format: format)
        let image = renderer.image { ctx in
            let g = ctx.cgContext
            UIColor.white.setFill()
            g.fill(CGRect(origin: .zero, size: size))

            let titleAttr: [NSAttributedString.Key: Any] = [
                .font: UIFont.boldSystemFont(ofSize: 20),
                .foregroundColor: UIColor(white: 0.12, alpha: 1)
            ]
            NSString(string: "Q2 vs Q3 各大区营收对比（千元）").draw(at: CGPoint(x: 32, y: 24), withAttributes: titleAttr)

            let subAttr: [NSAttributedString.Key: Any] = [
                .font: UIFont.systemFont(ofSize: 12),
                .foregroundColor: UIColor(white: 0.45, alpha: 1)
            ]
            NSString(string: "演示数据 · 蓝色为 Q2，靛蓝为 Q3").draw(at: CGPoint(x: 32, y: 52), withAttributes: subAttr)

            let regions: [(String, CGFloat, CGFloat)] = [
                ("华东", 148.2, 204.3),
                ("华南", 98.5, 136.2),
                ("华北", 79.4, 92.4),
                ("西南", 46.1, 53.5)
            ]
            let baselineY: CGFloat = 335
            let chartH: CGFloat = 225
            let maxVal: CGFloat = 220

            g.setStrokeColor(UIColor(white: 0.9, alpha: 1).cgColor)
            g.setLineWidth(1)
            for step in 0...4 {
                let y = baselineY - chartH * CGFloat(step) / 4.0
                g.move(to: CGPoint(x: 40, y: y))
                g.addLine(to: CGPoint(x: 600, y: y))
            }
            g.strokePath()

            let q2Color = UIColor(red: 0.56, green: 0.76, blue: 0.98, alpha: 1)
            let q3Color = UIColor(red: 0.24, green: 0.42, blue: 0.92, alpha: 1)
            let labelAttr: [NSAttributedString.Key: Any] = [
                .font: UIFont.systemFont(ofSize: 13, weight: .medium),
                .foregroundColor: UIColor(white: 0.2, alpha: 1)
            ]
            let valAttr: [NSAttributedString.Key: Any] = [
                .font: UIFont.monospacedDigitSystemFont(ofSize: 11, weight: .semibold),
                .foregroundColor: UIColor(white: 0.25, alpha: 1)
            ]

            for (i, item) in regions.enumerated() {
                let groupX: CGFloat = 68 + CGFloat(i) * 136
                let h2 = chartH * (item.1 / maxVal)
                let h3 = chartH * (item.2 / maxVal)

                q2Color.setFill()
                UIBezierPath(roundedRect: CGRect(x: groupX, y: baselineY - h2, width: 38, height: h2), cornerRadius: 5).fill()
                q3Color.setFill()
                UIBezierPath(roundedRect: CGRect(x: groupX + 44, y: baselineY - h3, width: 38, height: h3), cornerRadius: 5).fill()

                NSString(string: String(format: "%.0f", item.1)).draw(at: CGPoint(x: groupX + 4, y: baselineY - h2 - 16), withAttributes: valAttr)
                NSString(string: String(format: "%.0f", item.2)).draw(at: CGPoint(x: groupX + 48, y: baselineY - h3 - 16), withAttributes: valAttr)
                NSString(string: item.0).draw(at: CGPoint(x: groupX + 24, y: baselineY + 10), withAttributes: labelAttr)
            }
        }
        return image.pngData() ?? Data()
    }

    /// Programmatic receipt/invoice image encoded as JPEG.
    static func invoiceJPEG() -> Data {
        let size = CGSize(width: 520, height: 680)
        let renderer = UIGraphicsImageRenderer(size: size)
        let image = renderer.image { ctx in
            let g = ctx.cgContext
            UIColor(red: 0.96, green: 0.97, blue: 0.99, alpha: 1).setFill()
            g.fill(CGRect(origin: .zero, size: size))

            let card = CGRect(x: 28, y: 28, width: size.width - 56, height: size.height - 56)
            UIColor.white.setFill()
            let cardPath = UIBezierPath(roundedRect: card, cornerRadius: 14)
            cardPath.fill()
            UIColor(red: 0.85, green: 0.88, blue: 0.93, alpha: 1).setStroke()
            cardPath.lineWidth = 1.5
            cardPath.stroke()

            let headAttr: [NSAttributedString.Key: Any] = [
                .font: UIFont.boldSystemFont(ofSize: 22),
                .foregroundColor: UIColor(red: 0.12, green: 0.25, blue: 0.55, alpha: 1)
            ]
            NSString(string: "电子发票（演示样例）").draw(at: CGPoint(x: 52, y: 56), withAttributes: headAttr)

            let bodyAttr: [NSAttributedString.Key: Any] = [
                .font: UIFont.systemFont(ofSize: 15),
                .foregroundColor: UIColor(white: 0.22, alpha: 1)
            ]
            let rows = [
                "发票号码：DEMO-2026-0849102",
                "销售方：星图办公用品有限公司",
                "项目：无线机械键盘 × 1，USB-C 扩展坞 × 2",
                "金额合计：¥1,280.00",
                "备注：演示模式自动生成，非真实票据"
            ]
            for (idx, line) in rows.enumerated() {
                NSString(string: line).draw(at: CGPoint(x: 52, y: 120 + CGFloat(idx) * 38), withAttributes: bodyAttr)
            }
        }
        return image.jpegData(compressionQuality: 0.88) ?? Data()
    }

    /// Programmatic single-page PDF quarterly report.
    static func reportPDF() -> Data {
        let pageRect = CGRect(x: 0, y: 0, width: 595, height: 842)
        let renderer = UIGraphicsPDFRenderer(bounds: pageRect)
        return renderer.pdfData { ctx in
            ctx.beginPage()
            func text(_ str: String, _ x: CGFloat, _ y: CGFloat, _ size: CGFloat, bold: Bool = false, color: UIColor = UIColor(white: 0.15, alpha: 1)) {
                let font = bold ? UIFont.boldSystemFont(ofSize: size) : UIFont.systemFont(ofSize: size)
                NSString(string: str).draw(at: CGPoint(x: x, y: y), withAttributes: [.font: font, .foregroundColor: color])
            }
            func table(_ rows: [[String]], x: CGFloat, y: CGFloat, widths: [CGFloat]) {
                var cy = y
                for (rIdx, row) in rows.enumerated() {
                    var cx = x
                    for (cIdx, cell) in row.enumerated() {
                        let w = widths[cIdx]
                        let rect = CGRect(x: cx, y: cy, width: w, height: 28)
                        (rIdx == 0 ? UIColor(red: 0.93, green: 0.95, blue: 0.99, alpha: 1) : UIColor.white).setFill()
                        UIBezierPath(rect: rect).fill()
                        UIColor(white: 0.82, alpha: 1).setStroke()
                        UIBezierPath(rect: rect).stroke()
                        text(cell, cx + 8, cy + 7, 12, bold: rIdx == 0)
                        cx += w
                    }
                    cy += 28
                }
            }
            text("三季度销售复盘报告（演示样例）", 50, 56, 22, bold: true)
            text("摘要：Q3 总体营收 ¥486,400，同比增长 30.7%，各区域表现稳健。", 50, 96, 13, color: UIColor.darkGray)
            table([
                ["大区", "Q2 营收", "Q3 营收", "增长率"],
                ["华东", "¥148,200", "¥204,300", "+37.9%"],
                ["华南", "¥98,500", "¥136,200", "+38.3%"],
                ["华北", "¥79,400", "¥92,400", "+16.4%"],
                ["西南", "¥46,100", "¥53,500", "+16.1%"]
            ], x: 50, y: 140, widths: [120, 125, 125, 125])
            let y: CGFloat = 320
            text("后续待办事项：", 50, y, 15, bold: true)
            table([
                ["事项", "负责团队", "截止日期"],
                ["确认十月活动预算", "市场部", "下周二前"],
                ["华南区门店走访复盘", "华南负责人", "两周内"]
            ], x: 50, y: y + 30, widths: [235, 140, 120])
            text("本文档由演示模式生成，数据均为虚构，仅用于展示。", 50, 790, 10, color: UIColor.gray)
        }
    }
}
