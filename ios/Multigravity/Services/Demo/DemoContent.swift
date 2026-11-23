import Foundation
import UIKit

extension DemoGateway {
    // MARK: - Demo Seed Data & Rich Assets

    func seed() {
        lock.lock()
        defer { lock.unlock() }

        let now = Date()
        func ago(minutes m: Double) -> Date { now.addingTimeInterval(-m * 60) }
        func isoAgo(minutes m: Double) -> String { Self.iso(ago(minutes: m)) }

        // MARK: Projects
        // Pure chat MUST use id="outside-of-project", name="Chat", workspaceUri="" so ProjectItem.isPureChat matches cleanly.
        projects = [
            Project(
                id: "outside-of-project",
                name: "Chat",
                alias: nil,
                workspaceUri: "",
                path: Self.t("新对话 · 不关联任何工作区", "New Chat · No Workspace Attached", "新規チャット · ワークスペース未指定"),
                isWorkspace: false
            ),
            Project(
                id: "proj-multigravity",
                name: "antigravity-mobile",
                alias: "Multigravity",
                workspaceUri: "file:///Users/demo/Projects/antigravity-mobile",
                path: "/Users/demo/Projects/antigravity-mobile",
                isWorkspace: true
            ),
            Project(
                id: "proj-risk",
                name: "risk-engine",
                alias: Self.t("风控引擎", "Risk Engine", "リスクエンジン"),
                workspaceUri: "file:///Users/demo/Projects/risk-engine",
                path: "/Users/demo/Projects/risk-engine",
                isWorkspace: true
            ),
            Project(
                id: "proj-analytics",
                name: "data-analytics",
                alias: Self.t("数据分析台", "Analytics Hub", "データ分析ハブ"),
                workspaceUri: "file:///Users/demo/Projects/data-analytics",
                path: "/Users/demo/Projects/data-analytics",
                isWorkspace: true
            ),
            Project(
                id: "proj-deploy",
                name: "deploy-scripts",
                alias: Self.t("部署脚本", "Deploy Scripts", "デプロイスクリプト"),
                workspaceUri: "file:///Users/demo/Projects/deploy-scripts",
                path: "/Users/demo/Projects/deploy-scripts",
                isWorkspace: true
            )
        ]

        // MARK: Accounts & Quotas (Aligned with real Cockpit 5h / Weekly + independent Argon pool)
        accounts = [
            QuotaAccount(
                id: "acc-ultra",
                email: "alex@antigravity.dev",
                name: "Alex · Ultra",
                tier: "ULTRA",
                claude5h: 84.6,
                claudeWeekly: 91.5,
                gemini5h: 76.4,
                geminiWeekly: 89.2,
                argon: 96.2,
                claude5hOffsetHours: 4.2,
                claudeWeeklyOffsetDays: 6.1,
                gemini5hOffsetHours: 3.8,
                geminiWeeklyOffsetDays: 5.6
            ),
            QuotaAccount(
                id: "acc-pro",
                email: "alex-work@antigravity.dev",
                name: "Alex Work",
                tier: "PRO",
                claude5h: 41.8,
                claudeWeekly: 67.4,
                gemini5h: 54.2,
                geminiWeekly: 78.6,
                argon: 24.6,
                claude5hOffsetHours: 1.7,
                claudeWeeklyOffsetDays: 2.4,
                gemini5hOffsetHours: 2.4,
                geminiWeeklyOffsetDays: 3.2
            ),
            QuotaAccount(
                id: "acc-free",
                email: "alex-lab@antigravity.dev",
                name: "Alex Lab",
                tier: "FREE",
                claude5h: 18.0,
                claudeWeekly: 22.0,
                gemini5h: 31.0,
                geminiWeekly: 38.5,
                argon: nil,
                claude5hOffsetHours: 0.7,
                claudeWeeklyOffsetDays: 1.2,
                gemini5hOffsetHours: 0.9,
                geminiWeeklyOffsetDays: 1.5
            )
        ]
        activeAccountId = "acc-ultra"

        // MARK: Demo Files (PDF, CSV, Markdown, Charts, Invoices, Plan, Jupyter Notebook)
        func registerFile(path: String, name: String, data: Data) {
            let id = Self.fileID(for: data)
            files[path] = StoredFile(id: id, name: name, data: data, path: path)
        }

        let csvData = Data(Self.salesCSV.utf8)
        let notesData = Data(Self.meetingNotes.utf8)
        let planData = Data(Self.planMarkdown.utf8)
        let chartData = Self.salesChartPNG()
        let invoiceCoffeeData = Self.invoiceJPEG(
            title: Self.t("电子发票（餐饮服务）", "Electronic Receipt · Cafe", "電子領収書（カフェ飲食）"),
            shop: Self.t("星河精品咖啡（北京国贸店）", "Starburst Specialty Coffee · Downtown", "スターバースト珈琲 渋谷スクランブル店"),
            items: [
                (Self.t("手冲埃塞俄比亚 × 2", "Pour-over Ethiopia × 2", "ハンドドリップ エチオピア × 2"), Self.t("¥76.00", "$11.00", "¥1,320")),
                (Self.t("燕麦拿铁（大杯）× 1", "Oat Milk Latte (L) × 1", "オーツミルクラテ (L) × 1"), Self.t("¥38.00", "$5.50", "¥680")),
                (Self.t("牛角可颂套餐 × 1", "Croissant Combo × 1", "クロワッサンセット × 1"), Self.t("¥44.00", "$6.50", "¥850"))
            ],
            total: Self.t("¥158.00", "$23.00", "¥2,850"),
            no: "NO.20261108-88412"
        )
        let invoiceTaxiData = Self.invoiceJPEG(
            title: Self.t("北京出租车电子客运发票", "Airport Transit E-Receipt", "空港タクシー電子領収書"),
            shop: Self.t("京B·82716 首都机场 → 国贸CBD", "Cab #82716 · SFO Terminal 2 → SoMa", "品川 500 あ 8271 · 羽田空港T2 → 渋谷"),
            items: [
                (Self.t("里程 28.4 km（含高速费）", "Distance 17.6 mi (incl. bridge toll)", "走行距離 21.4 km（首都高料金含む）"), Self.t("¥102.00", "$46.50", "¥7,800")),
                (Self.t("夜间服务加收", "Late-night airport surcharge", "深夜早朝割増"), Self.t("¥14.00", "$4.50", "¥1,200"))
            ],
            total: Self.t("¥116.00", "$51.00", "¥9,000"),
            no: "NO.20261108-51904"
        )
        let pdfData = Self.reportPDF(chartPNG: chartData)
        let notebookData = Data(Self.notebookJSON.utf8)

        registerFile(path: "/Users/demo/Documents/销售数据.csv", name: "销售数据.csv", data: csvData)
        registerFile(path: "/Users/demo/Documents/sales_q3.csv", name: "sales_q3.csv", data: csvData)
        registerFile(path: "/Users/demo/Documents/会议纪要.md", name: "会议纪要.md", data: notesData)
        registerFile(path: "/Users/demo/Documents/meeting_notes.md", name: "meeting_notes.md", data: notesData)
        registerFile(path: "/Users/demo/Documents/sales_chart.png", name: "sales_chart.png", data: chartData)
        registerFile(path: "/Users/demo/Documents/三季度销售报告.pdf", name: "三季度销售报告.pdf", data: pdfData)
        registerFile(path: "/Users/demo/Documents/q3_sales_report.pdf", name: "q3_sales_report.pdf", data: pdfData)
        registerFile(path: "/Users/demo/Documents/invoice_coffee.jpg", name: "invoice_coffee.jpg", data: invoiceCoffeeData)
        registerFile(path: "/Users/demo/Documents/invoice_taxi.jpg", name: "invoice_taxi.jpg", data: invoiceTaxiData)
        registerFile(path: "/Users/demo/.gemini/antigravity/brain/demo-0005/implementation_plan.md", name: "implementation_plan.md", data: planData)
        registerFile(path: "/Users/demo/Documents/analytics.ipynb", name: "analytics.ipynb", data: notebookData)

        // MARK: MCP Servers
        mcpServers = [
            McpServerState(
                spec: McpServerSpec(serverName: "github", command: "npx -y @modelcontextprotocol/server-github"),
                status: "MCP_SERVER_STATUS_READY",
                tools: [
                    McpToolDefinition(name: "create_pull_request", description: Self.t("创建或更新 GitHub Pull Request", "Create or update a GitHub Pull Request", "GitHub Pull Request を作成または更新"), serverName: "github", jsonSchemaString: "{\"properties\":{\"title\":{\"type\":\"string\"},\"base\":{\"type\":\"string\"}},\"required\":[\"title\"]}"),
                    McpToolDefinition(name: "get_issue", description: Self.t("读取指定 Issue 与评论", "Read GitHub issue details and comments", "指定した Issue の詳細とコメントを取得"), serverName: "github", jsonSchemaString: "{\"properties\":{\"issue_number\":{\"type\":\"integer\"}},\"required\":[\"issue_number\"]}"),
                    McpToolDefinition(name: "list_commits", description: Self.t("列出仓库近期提交记录", "List recent commits in a repository", "リポジトリの直近コミット履歴を一覧取得"), serverName: "github", jsonSchemaString: "{\"properties\":{\"sha\":{\"type\":\"string\"}}}"),
                    McpToolDefinition(name: "search_code", description: Self.t("跨仓库全文代码检索", "Full-text code search across repositories", "リポジトリ横断の全文コード検索を実行"), serverName: "github", jsonSchemaString: "{\"properties\":{\"q\":{\"type\":\"string\"}},\"required\":[\"q\"]}")
                ],
                serverInfo: McpServerInfoDetail(name: "github-mcp", version: "2.4.0"),
                instructions: Self.t("GitHub 仓库、Issue 与 PR 自动化工具", "GitHub repositories, issues, and PR automation", "GitHub リポジトリ・Issue・PR 自動化ツール")
            ),
            McpServerState(
                spec: McpServerSpec(serverName: "postgres-prod", command: "uvx mcp-postgres --readonly"),
                status: "MCP_SERVER_STATUS_READY",
                tools: [
                    McpToolDefinition(name: "query", description: Self.t("执行只读 SQL 查询并格式化结果", "Run a read-only SQL query and format rows", "読み取り専用 SQL クエリを実行して整形表示"), serverName: "postgres-prod", jsonSchemaString: "{\"properties\":{\"sql\":{\"type\":\"string\"}},\"required\":[\"sql\"]}"),
                    McpToolDefinition(name: "explain_analyze", description: Self.t("查看 SQL 查询执行计划与索引命中", "Inspect query execution plan and index scans", "SQL 実行計画とインデックス利用状況を診断"), serverName: "postgres-prod", jsonSchemaString: "{\"properties\":{\"sql\":{\"type\":\"string\"}},\"required\":[\"sql\"]}"),
                    McpToolDefinition(name: "describe_table", description: Self.t("检查表字段、索引与外键约束", "Inspect table columns, indexes, and foreign keys", "テーブルのカラム・インデックス・外部キーを確認"), serverName: "postgres-prod", jsonSchemaString: "{\"properties\":{\"table\":{\"type\":\"string\"}},\"required\":[\"table\"]}")
                ],
                serverInfo: McpServerInfoDetail(name: "postgres-readonly", version: "1.3.1"),
                instructions: Self.t("只读数据库诊断与执行计划检查", "Read-only PostgreSQL schema and query diagnostics", "PostgreSQL 読み取り専用スキーマ＆クエリ診断")
            ),
            McpServerState(
                spec: McpServerSpec(serverName: "xcode-mcp", command: "xcrun mcpbridge"),
                status: "MCP_SERVER_STATUS_READY",
                tools: [
                    McpToolDefinition(name: "build_scheme", description: Self.t("触发 Xcode Scheme 增量构建并返回诊断结果", "Build an Xcode scheme incrementally and return diagnostics", "Xcode スキームの差分ビルドを実行し診断結果を返す"), serverName: "xcode-mcp", jsonSchemaString: "{\"properties\":{\"scheme\":{\"type\":\"string\"}},\"required\":[\"scheme\"]}"),
                    McpToolDefinition(name: "list_destinations", description: Self.t("列出可用 iOS 模拟器与无线调试真机", "List available iOS simulators and paired physical devices", "利用可能な iOS シミュレータと実機デバイスを一覧表示"), serverName: "xcode-mcp", jsonSchemaString: "{\"properties\":{\"platform\":{\"type\":\"string\"}}}")
                ],
                serverInfo: McpServerInfoDetail(name: "xcode-mcpbridge", version: "2.2.0"),
                instructions: Self.t("Xcode 工程构建、诊断与测试执行接口", "Xcode build, diagnostics, and simulator bridge", "Xcode ビルド・静的診断・シミュレータ連携ブリッジ")
            ),
            McpServerState(
                spec: McpServerSpec(serverName: "slack-bridge", command: "node slack-mcp.js"),
                status: "MCP_SERVER_STATUS_ERROR",
                tools: [],
                serverInfo: McpServerInfoDetail(name: "slack-bridge", version: "0.9.8"),
                instructions: Self.t("Slack 频道通知推送网关（OAuth Token 已过期）", "Slack channel notification gateway (OAuth token expired)", "Slack チャンネル通知ゲートウェイ（OAuth トークン期限切れ）")
            )
        ]

        // MARK: Skills
        skills = [
            SkillInfo(name: "swiftui-audit", description: Self.t("SwiftUI 列表性能、@Observable 渲染与无障碍审计", "Audit SwiftUI list diffing, @Observable invalidation, and accessibility", "SwiftUI リスト描画性能・@Observable 監視・アクセシビリティ監査"), path: "/Users/demo/.gemini/config/skills/swiftui-audit/SKILL.md", content: Self.t("# SwiftUI Audit\n检查长列表视图复用、主线程隔离与动态字体适配。", "# SwiftUI Audit\nInspect list cell reuse, main-actor isolation, and Dynamic Type support.", "# SwiftUI Audit\n長いリストの再利用・MainActor 分離・Dynamic Type 対応を検査します。")),
            SkillInfo(name: "go-test-race", description: Self.t("执行 go test -race 并定位并发竞态与连接泄漏", "Run go test -race and diagnose data races or goroutine leaks", "go test -race を実行しデータ競合や goroutine リークを特定"), path: "/Users/demo/.gemini/config/skills/go-test-race/SKILL.md", content: Self.t("# Go Test Race\n自动排查 goroutine 泄漏与并发 map 读写竞态。", "# Go Test Race\nAutomatically pinpoint goroutine leaks and concurrent map access races.", "# Go Test Race\ngoroutine リークや並行 map アクセスの競合を自動検出します。")),
            SkillInfo(name: "sql-migration-review", description: Self.t("Zero-downtime PostgreSQL DDL 与回滚脚本审校", "Review zero-downtime PostgreSQL DDL migrations and rollback safety", "無停止 PostgreSQL DDL とロールバック安全性のレビュー"), path: "/Users/demo/.gemini/config/skills/sql-migration-review/SKILL.md", content: Self.t("# SQL Migration Review\n确保所有索引使用 CONCURRENTLY 创建并附带回滚脚本。", "# SQL Migration Review\nEnsure indexes use CONCURRENTLY and include reversible rollback statements.", "# SQL Migration Review\nすべてのインデックスが CONCURRENTLY で作成され、ロールバック手順があるか確認します。")),
            SkillInfo(name: "invoice-reconcile", description: Self.t("发票 OCR 识别、分类与费用对账汇总", "OCR receipt extraction, categorization, and expense reconciliation", "領収書 OCR 抽出・費目分類・経費精算突合"), path: "/Users/demo/.gemini/config/skills/invoice-reconcile/SKILL.md", content: Self.t("# Invoice Reconcile\n识别电子发票商户、单号与价税合计金额。", "# Invoice Reconcile\nExtract merchant names, receipt IDs, and tax-inclusive totals.", "# Invoice Reconcile\n領収書の店舗名・伝票番号・税込合計金額を抽出します。")),
            SkillInfo(name: "release-checklist", description: Self.t("Xcode Archive、签名校验与 TestFlight 灰度检查", "Verify Xcode Archive, signing identity, and TestFlight release notes", "Xcode Archive・署名検証・TestFlight リリース前チェック"), path: "/Users/demo/.gemini/config/skills/release-checklist/SKILL.md", content: Self.t("# Release Checklist\n核验 Bundle Version、签名证书指纹与崩溃符号表上传。", "# Release Checklist\nValidate bundle version, signing certificate fingerprint, and dSYM upload.", "# Release Checklist\nバンドル番号・署名証明書フィンガープリント・dSYM アップロードを確認します。"))
        ]

        // MARK: Plugins & Providers (Aligned with CascadePluginItem & PluginAuthProviderItem)
        plugins = [
            [
                "id": "plugin-git-inspector",
                "name": "Git & PR Inspector",
                "description": Self.t("实时读取分支状态、Diff 与 GitHub Pull Request", "Inspect branch status, diffs, and GitHub Pull Requests in real time", "ブランチ状態・コード差分・GitHub Pull Request をリアルタイム確認"),
                "version": "1.8.2",
                "enabled": true,
                "installed": true,
                "kind": "plugin",
                "icon": "arrow.triangle.branch",
                "authStatus": "authenticated",
                "accounts": ["alex@github.com"]
            ],
            [
                "id": "plugin-xcode-bridge",
                "name": "Xcode Wireless Bridge",
                "description": Self.t("真机无线调试状态监测与构建日志流推送", "Monitor wireless device pairing state and stream build logs", "ワイヤレスデバッグ接続状態の監視とビルドログのリアルタイム配信"),
                "version": "2.1.0",
                "enabled": true,
                "installed": true,
                "kind": "plugin",
                "icon": "hammer",
                "authStatus": "authenticated",
                "accounts": ["Apple Developer Team"]
            ],
            [
                "id": "plugin-notion-sync",
                "name": "Notion Knowledge Sync",
                "description": Self.t("将技术评审与会议纪要一键归档至团队知识库", "Archive architecture reviews and meeting notes to Notion", "設計レビューや議事録をワンタップでチーム Notion にアーカイブ"),
                "version": "0.9.4",
                "enabled": false,
                "installed": false,
                "kind": "plugin",
                "icon": "doc.text",
                "authStatus": "unauthenticated",
                "accounts": []
            ]
        ]

        pluginProviders = [
            [
                "id": "github",
                "name": "GitHub OAuth",
                "signedIn": true
            ],
            [
                "id": "apple",
                "name": "Apple Developer",
                "signedIn": true
            ],
            [
                "id": "notion",
                "name": "Notion Workspace",
                "signedIn": false
            ]
        ]

        // MARK: Automations (Aligned with AutomationSidecarItem & AutomationEventItem)
        let demoTZ = Self.t("Asia/Shanghai", "America/Los_Angeles", "Asia/Tokyo")
        automations = [
            [
                "id": "auto-nightly-ci",
                "name": Self.t("夜间回归测试巡检", "Nightly Regression Suite", "夜間リグレッション自動巡回"),
                "description": Self.t("每日凌晨执行 Go 竞态单测与 iOS 无头编译检查，异常时通过 APNs 推送摘要。", "Run go test -race and headless iOS build checks every night with APNs summaries.", "毎晩 go test -race とヘッドレス iOS ビルド検証を実行し、失敗時に APNs 通知します。"),
                "status": "RUNNING",
                "cronExpression": "0 2 * * *",
                "timeZone": demoTZ,
                "model": "gemini-4-neon-high",
                "prompt": Self.t("运行 go test -race ./... 与 iOS 编译检查，若有失败则推送错误摘要。", "Run go test -race ./... and iOS build checks; notify on failure.", "go test -race ./... と iOS ビルド検証を実行し、失敗時に要約を通知。"),
                "lastRunAt": isoAgo(minutes: 420),
                "nextRunAt": Self.iso(now.addingTimeInterval(6 * 3600)),
                "enabled": true,
                "workspaceUri": "file:///Users/demo/Projects/antigravity-mobile"
            ],
            [
                "id": "auto-quota-alert",
                "name": Self.t("配额水位与重置监测", "Quota Headroom Watch", "クォータ残量＆リセット監視"),
                "description": Self.t("每小时巡检 5h / Weekly 与 Argon 池余量，低于 20% 时推送切号提醒。", "Check 5h, Weekly, and Argon pools hourly and alert below 20%.", "毎時 5h・週間・Argon の各枠残量を監視し、20% 未満でアカウント切替を通知します。"),
                "status": "READY",
                "cronExpression": "0 * * * *",
                "timeZone": demoTZ,
                "model": "gemini-4-hydrogen-high",
                "prompt": Self.t("每小时检查各账号 5h 额度池余量，低于 20% 时提醒切换备用账号。", "Check 5h model quota headroom hourly and alert below 20%.", "1時間ごとに各アカウントの 5h 枠残量を監視し、20% 未満で切替を通知。"),
                "lastRunAt": isoAgo(minutes: 28),
                "nextRunAt": Self.iso(now.addingTimeInterval(32 * 60)),
                "enabled": true,
                "workspaceUri": ""
            ],
            [
                "id": "auto-friday-digest",
                "name": Self.t("周五版本发布周报", "Friday Release Digest", "金曜リリース週次ダイジェスト"),
                "description": Self.t("汇总本周合并 PR、线上 P99 延迟与未闭环 Issue 生成发布周报。", "Summarize merged PRs, P99 latency, and open issues into a Markdown digest.", "今週の PR・P99 レイテンシ・未解決 Issue を Markdown 週報にまとめます。"),
                "status": "STOPPED",
                "cronExpression": "0 17 * * 5",
                "timeZone": demoTZ,
                "model": "claude-opus-5-5-high",
                "prompt": Self.t("汇总本周 PR 合并清单、线上崩溃率与未闭环 Issue，生成 Markdown 周报。", "Summarize merged PRs, crash-free rate, and open issues in Markdown.", "今週マージされた PR・クラッシュフリー率・未解決 Issue を Markdown 週報に集計。"),
                "lastRunAt": isoAgo(minutes: 4320),
                "enabled": false,
                "workspaceUri": "file:///Users/demo/Projects/antigravity-mobile"
            ]
        ]

        automationEvents = [
            ["id": "evt-1", "scheduleId": "auto-nightly-ci", "sidecarId": "auto-nightly-ci", "conversationId": "demo-0001-upload", "status": "SUCCESS", "message": Self.t("41/41 项测试通过 · iOS 编译零警告", "41/41 tests passed · iOS build clean", "41/41 テスト通過 · iOS ビルド警告ゼロ"), "timestamp": isoAgo(minutes: 420)],
            ["id": "evt-2", "scheduleId": "auto-quota-alert", "sidecarId": "auto-quota-alert", "conversationId": "demo-0012-swarm", "status": "SUCCESS", "message": Self.t("Ultra 5h 76.4% · Pro Argon 24.6%（已提示）", "Ultra 5h 76.4% · Pro Argon 24.6% (alert sent)", "Ultra 5h 76.4% · Pro Argon 24.6%（通知済み）"), "timestamp": isoAgo(minutes: 28)],
            ["id": "evt-3", "scheduleId": "auto-nightly-ci", "sidecarId": "auto-nightly-ci", "conversationId": "demo-0014-running", "status": "RUNNING", "message": Self.t("正在执行 iOS xcodebuild 增量校验...", "Running incremental iOS xcodebuild check...", "iOS xcodebuild 差分ビルド検証を実行中..."), "timestamp": isoAgo(minutes: 425)],
            ["id": "evt-4", "scheduleId": "auto-friday-digest", "sidecarId": "auto-friday-digest", "conversationId": "demo-0005-redesign", "status": "SKIPPED", "message": Self.t("定时任务已暂停", "Automation paused", "定期実行は一時停止中"), "timestamp": isoAgo(minutes: 1440)]
        ]

        // MARK: Workflow Templates
        workflowTemplates = Self.defaultDemoWorkflowTemplates()

        // MARK: Global Daemon & Running Tasks + Logs
        globalDaemonTasks = [
            [
                "taskId": "task-demo-upload-build",
                "conversationId": "demo-0001-upload",
                "conversationTitle": Self.t("附件流式上传与 SHA-256 校验", "Streaming attachment upload & SHA-256", "添付ファイルのストリーム送信とSHA-256検証"),
                "commandLine": "go test -v ./internal/gateway -run TestAttachmentUpload",
                "toolSummary": Self.t("运行网关附件上传单测", "Run gateway attachment upload tests", "ゲートウェイ添付アップロード単体テストを実行"),
                "startedAt": isoAgo(minutes: 2),
                "isDaemon": false,
                "elapsedSeconds": 24,
                "running": true
            ],
            [
                "taskId": "task-vite-daemon",
                "conversationId": "demo-0014-running",
                "conversationTitle": Self.t("本地 HMR 守护进程与竞态测试", "Local HMR dev server & race suite", "ローカルHMR常駐サーバーとレース検証"),
                "commandLine": "npm run dev -- --host 0.0.0.0 --port 5173",
                "toolSummary": Self.t("Web 控制台热更新守护进程", "Web Console HMR Dev Server", "Web コンソール HMR 開発サーバー"),
                "startedAt": isoAgo(minutes: 6),
                "isDaemon": true,
                "elapsedSeconds": 360,
                "running": true
            ],
            [
                "taskId": "task-go-race",
                "conversationId": "demo-0014-running",
                "conversationTitle": Self.t("本地 HMR 守护进程与竞态测试", "Local HMR dev server & race suite", "ローカルHMR常駐サーバーとレース検証"),
                "commandLine": "go test -race -count=1 ./internal/cockpit/...",
                "toolSummary": Self.t("执行 Go 并发竞态回归测试", "Run Go race-detector suite", "Go 並行レース検出テストを実行"),
                "startedAt": isoAgo(minutes: 1.5),
                "isDaemon": false,
                "elapsedSeconds": 90,
                "running": true
            ]
        ]

        taskLogs = [
            "task-demo-upload-build": """
            === RUN   TestAttachmentUpload
            === RUN   TestAttachmentUpload/multipart_csv_and_image
            [14:12:01] POST /api/v1/attachments X-File-Name=sales_q3.csv (1842 B) -> 200 OK id=9f2a4c1b
            [14:12:02] POST /api/v1/attachments X-File-Name=screenshot.png (41280 B) -> 200 OK id=3d8e10aa
            [14:12:03] SHA-256 verification passed · inbox file persisted
            --- PASS: TestAttachmentUpload/multipart_csv_and_image (0.14s)
            === RUN   TestAttachmentUpload/oversized_payload_guard
            --- PASS: TestAttachmentUpload/oversized_payload_guard (0.04s)
            PASS
            """,
            "task-vite-daemon": """
            > @multigravity/web-console@1.4.0 dev
            > vite --host 0.0.0.0 --port 5173

              VITE v6.2.1  ready in 312 ms

              ➜  Local:   http://localhost:5173/
              ➜  Network: http://192.168.50.10:5173/
            [hmr] connected 2 clients (desktop-safari, ios-webview)
            [hmr] updated src/components/ArtifactPreview.tsx in 19ms
            """,
            "task-go-race": """
            === RUN   TestCockpitQuotaAggregation
            --- PASS: TestCockpitQuotaAggregation (0.42s)
            === RUN   TestSwarmBoardBroadcastConcurrency
            [trace] 16 goroutines reading/writing SwarmSnapshot simultaneously
            --- PASS: TestSwarmBoardBroadcastConcurrency (1.18s)
            === RUN   TestEnsureAntigravityStateDBs
            ... running sqlite wal verification (12/14 assertions done)
            """
        ]

        // MARK: Subagent Conversations (clickable inside demo-0011-workflow)
        let sub1 = Conversation(
            id: "demo-subagent-audit-1",
            title: Self.t("安全扫描：SQL 注入与越权", "Security Scan: SQL Injection & AuthZ", "セキュリティスキャン：SQLi ＆ 認可検証"),
            workspaceName: Self.t("风控引擎", "Risk Engine", "リスクエンジン"),
            workspaceUri: "file:///Users/demo/Projects/risk-engine",
            status: "CASCADE_RUN_STATUS_IDLE",
            duration: Self.t("14秒", "14s", "14秒"),
            messages: [
                Self.userMsg(
                    id: "sub1-u0",
                    step: 0,
                    text: Self.t("全面扫描 `handlers/rule_eval.go` 与 `repo/score_store.go` 的参数绑定、租户隔离和 Token 校验逻辑。", "Scan `handlers/rule_eval.go` and `repo/score_store.go` for parameter binding, tenant isolation, and auth token validation.", "`handlers/rule_eval.go` と `repo/score_store.go` のパラメータバインド・テナント分離・トークン検証を全面スキャンして。"),
                    timestamp: isoAgo(minutes: 25)
                ),
                Self.toolMsg(
                    id: "sub1-t1",
                    step: 1,
                    tools: ["grep_search", "read_file", "run_command"],
                    thinking: Self.t("检查 14 个风控 handler 的 SQL 拼接与租户条件注入点，重点核对 ORDER BY 和动态过滤条件。", "Auditing SQL string building and tenant isolation across 14 handlers, focusing on dynamic ORDER BY clauses.", "14 個のハンドラで SQL 構築とテナント分離を監査し、動的 ORDER BY 句を中心に検査します。"),
                    thinkingDuration: "4.6s",
                    toolDetails: [
                        ["name": "grep_search", "summary": "ORDER BY|fmt.Sprintf in internal/repo/", "status": "done", "duration": "0.3s", "stepIndex": 1],
                        ["name": "read_file", "summary": "repo/score_store.go (L60-L125)", "status": "done", "duration": "0.2s", "stepIndex": 1],
                        ["name": "run_command", "summary": "gosec -quiet ./handlers/... ./repo/...", "status": "done", "duration": "1.9s", "stepIndex": 1]
                    ]
                ),
                Self.agentMsg(
                    id: "sub1-a2",
                    step: 2,
                    model: "claude-opus-5-5-high",
                    modelName: "Claude Opus 5.5 (High)",
                    text: Self.t(
                        """
                        已完成 **安全扫描**（共检查 14 个 handler，发现 1 处中高危问题）：

                        1. **`repo/score_store.go:88` 动态排序字段未做白名单校验**
                           - 当前直接拼接 `sortBy` 进 `ORDER BY %s`
                           - **修复建议**：映射为允许列字典 `allowedSortCols[sortBy]`。
                        2. **租户隔离检查通过 ✅**
                           - 所有查询均强制注入 `tenant_id = $1`。
                        """,
                        """
                        Completed **Security Scan** (14 handlers inspected, 1 high-severity finding):

                        1. **Unvalidated dynamic sort column in `repo/score_store.go:88`**
                           - Currently interpolates `sortBy` directly into `ORDER BY %s`.
                           - **Recommendation**: Map via `allowedSortCols[sortBy]` allowlist.
                        2. **Tenant isolation checks passed ✅**
                           - All queries enforce `tenant_id = $1`.
                        """,
                        """
                        **セキュリティスキャン**を完了しました（14 ハンドラを検査、重大度高の指摘 1 件）：

                        1. **`repo/score_store.go:88` の動的ソート列にホワイトリスト検証が未実装**
                           - 現在 `sortBy` をそのまま `ORDER BY %s` に結合しています。
                           - **修正案**：許可カラム辞書 `allowedSortCols[sortBy]` 経由でマッピングしてください。
                        2. **テナント分離チェック通過 ✅**
                           - 全クエリで `tenant_id = $1` が強制適用されています。
                        """
                    ),
                    timestamp: isoAgo(minutes: 24)
                )
            ],
            pendingInteraction: nil,
            canProceed: false,
            proceedArtifactUri: nil,
            runningTasks: [],
            queuedMessages: [],
            lastModified: ago(minutes: 24),
            unread: false,
            pinned: false,
            modelId: "claude-opus-5-5-high",
            modelName: "Claude Opus 5.5 (High)",
            startedAt: isoAgo(minutes: 25),
            contextMeter: Self.meter(used: 44_200, maxTokens: 200_000, tps: 66.4, ttft: 3.4),
            swarm: nil,
            subagents: [],
            parentConversationId: "demo-0011-workflow",
            subagentRole: Self.t("安全审计专员", "Security Auditor", "セキュリティ監査担当"),
            gitStatus: nil,
            changes: [],
            chapters: []
        )

        let sub2 = Conversation(
            id: "demo-subagent-perf-2",
            title: Self.t("压测分析：P99 延迟定位", "Load Profile: P99 Latency Hotspots", "負荷解析：P99 レイテンシ特定"),
            workspaceName: Self.t("风控引擎", "Risk Engine", "リスクエンジン"),
            workspaceUri: "file:///Users/demo/Projects/risk-engine",
            status: "CASCADE_RUN_STATUS_IDLE",
            duration: Self.t("11秒", "11s", "11秒"),
            messages: [
                Self.userMsg(
                    id: "sub2-u0",
                    step: 0,
                    text: Self.t("分析 `EvaluateRiskBatch` 在 500 QPS 下的 CPU pprof 火焰图，定位 P99 抖动来源。", "Analyze the CPU pprof flamegraph for `EvaluateRiskBatch` at 500 QPS and locate P99 latency spikes.", "500 QPS 時の `EvaluateRiskBatch` の CPU pprof フレームグラフを解析し、P99 スパイクの原因を特定して。"),
                    timestamp: isoAgo(minutes: 24)
                ),
                Self.toolMsg(
                    id: "sub2-t1",
                    step: 1,
                    tools: ["run_command", "read_file"],
                    thinking: Self.t("运行压测基准并读取 pprof 热点栈，确认 CPU 与内存分配主要落点。", "Running benchmark profile and inspecting top alloc/CPU frames.", "ベンチマークを実行して pprof のホットスポットとメモリ確保箇所を特定します。"),
                    thinkingDuration: "2.8s",
                    toolDetails: [
                        ["name": "run_command", "summary": "go test -bench=BenchmarkEvaluateRiskBatch -cpuprofile=cpu.out", "status": "done", "duration": "3.4s", "stepIndex": 1],
                        ["name": "read_file", "summary": "engine/evaluator.go (L110-L184)", "status": "done", "duration": "0.2s", "stepIndex": 1]
                    ]
                ),
                Self.agentMsg(
                    id: "sub2-a2",
                    step: 2,
                    model: "gemini-4-hydrogen-high",
                    modelName: "Gemini 4 Hydrogen (High)",
                    text: Self.t(
                        """
                        压测采样已完成，P99 耗时 **184ms** 主要集中在两处：

                        - `regexp.Compile` 在规则循环内重复编译（占总 CPU 44%），改用启动期预编译后可降至 **< 1%**。
                        - `sync.Pool` 复用临时评分切片可减少 68% GC 停顿。
                        """,
                        """
                        Profiling complete — **184ms P99** stems from two bottlenecks:

                        - Re-compiling `regexp.Compile` inside the rule loop (44% CPU); pre-compiling at startup drops this to **< 1%**.
                        - Reusing scratch score slices via `sync.Pool` cuts GC pauses by 68%.
                        """,
                        """
                        プロファイリングが完了しました。**P99 184ms** のボトルネックは主に 2 箇所です：

                        - ルール評価ループ内で `regexp.Compile` を毎回再コンパイル（CPU の 44% を消費）。起動時の事前コンパイル化で **1% 未満** に削減できます。
                        - 一時スコアスライスを `sync.Pool` で再利用すると GC 停止時間を 68% 削減可能です。
                        """
                    ),
                    timestamp: isoAgo(minutes: 23)
                )
            ],
            pendingInteraction: nil,
            canProceed: false,
            proceedArtifactUri: nil,
            runningTasks: [],
            queuedMessages: [],
            lastModified: ago(minutes: 23),
            unread: false,
            pinned: false,
            modelId: "gemini-4-hydrogen-high",
            modelName: "Gemini 4 Hydrogen (High)",
            startedAt: isoAgo(minutes: 24),
            contextMeter: Self.meter(used: 68_400, maxTokens: 512_000, tps: 162.5, ttft: 2.1),
            swarm: nil,
            subagents: [],
            parentConversationId: "demo-0011-workflow",
            subagentRole: Self.t("性能压测专家", "Perf Profiler", "性能プロファイラ"),
            gitStatus: nil,
            changes: [],
            chapters: []
        )

        let sub3 = Conversation(
            id: "demo-subagent-live-3",
            title: Self.t("边界单测自动补全", "Edge-Case Unit Test Authoring", "境界値ユニットテスト自動補完"),
            workspaceName: Self.t("风控引擎", "Risk Engine", "リスクエンジン"),
            workspaceUri: "file:///Users/demo/Projects/risk-engine",
            status: "CASCADE_RUN_STATUS_RUNNING",
            duration: Self.t("运行中", "Running", "実行中"),
            messages: [
                Self.userMsg(
                    id: "sub3-u0",
                    step: 0,
                    text: Self.t("根据审计和性能结论，为 `handlers/rule_eval_test.go` 补充白名单校验与批量超时单测。", "Add allowlist validation and batch timeout unit tests to `handlers/rule_eval_test.go` based on audit & perf findings.", "監査と性能解析の結果をもとに、`handlers/rule_eval_test.go` にホワイトリスト検証とバッチタイムアウトの単体テストを追加して。"),
                    timestamp: isoAgo(minutes: 22)
                ),
                Self.toolMsg(
                    id: "sub3-t1",
                    step: 1,
                    tools: ["edit_file", "run_command"],
                    thinking: Self.t("补充 6 组表驱动用例覆盖非法 sortBy 注入和 50ms 上下文取消边界，然后执行 go test -race 验证。", "Adding 6 table-driven cases for invalid sortBy injection and 50ms context deadline, then running go test -race.", "不正な sortBy 入力と 50ms コンテキストキャンセルを網羅する 6 件のテーブル駆動テストを追加し、go test -race を回します。"),
                    thinkingDuration: "3.9s",
                    toolDetails: [
                        ["name": "edit_file", "summary": "handlers/rule_eval_test.go (+84/-6)", "status": "done", "duration": "0.5s", "stepIndex": 1],
                        ["name": "run_command", "summary": "go test -race -v ./handlers -run TestRuleEval", "status": "running", "duration": "4.2s", "stepIndex": 1]
                    ]
                ),
                Self.agentMsg(
                    id: "sub3-a2",
                    step: 2,
                    model: "gemini-4-neon-high",
                    modelName: "Gemini 4 Neon (High)",
                    text: Self.t("正在写入 6 组表驱动测试用例并执行并发竞态检验，完成后会自动向主流程汇报。", "Writing 6 table-driven test cases and running race checks; will report back to the parent workflow once complete.", "6 パターンのテーブル駆動テストを作成し、`-race` 並行検証を実行中です。完了次第メインフローへ報告します。"),
                    timestamp: isoAgo(minutes: 21.5)
                )
            ],
            pendingInteraction: nil,
            canProceed: false,
            proceedArtifactUri: nil,
            runningTasks: [],
            queuedMessages: [],
            lastModified: ago(minutes: 21.5),
            unread: false,
            pinned: false,
            modelId: "gemini-4-neon-high",
            modelName: "Gemini 4 Neon (High)",
            startedAt: isoAgo(minutes: 22),
            contextMeter: Self.meter(used: 92_600, maxTokens: 512_000, tps: 138.2, ttft: 4.5),
            swarm: nil,
            subagents: [],
            parentConversationId: "demo-0011-workflow",
            subagentRole: Self.t("自动化测试工程师", "Test Engineer", "テスト自動化エンジニア"),
            gitStatus: nil,
            changes: [],
            chapters: []
        )

        // MARK: - 17 Authentic Antigravity Conversations

        // 1. Pinned Command Approval (triggers Lock Screen Live Activity & Dynamic Island Allow/Deny buttons!)
        let c17CmdApproval = Conversation(
            id: "demo-0017-cmd-approval",
            title: Self.t("生产集群灰度发布与缓存清理", "Canary rollout & Redis TTL cleanup", "本番カナリア配信とRedisキャッシュ掃除"),
            workspaceName: Self.t("部署脚本", "Deploy Scripts", "デプロイスクリプト"),
            workspaceUri: "file:///Users/demo/Projects/deploy-scripts",
            status: "CASCADE_RUN_STATUS_RUNNING",
            duration: Self.t("等待审批", "Needs Approval", "承認待ち"),
            messages: [
                Self.userMsg(
                    id: "c17-u0",
                    step: 0,
                    text: Self.t("先帮我检查生产 Redis `v1:session:tmp:*` 过期键数量，确认无活跃会话误伤后，再执行灰度清理与网关热重载。", "Dry-run expired `v1:session:tmp:*` keys in production Redis first; once verified safe, run the cleanup and zero-downtime gateway reload.", "本番 Redis の `v1:session:tmp:*` 期限切れキー件数を先にドライラン確認して。稼働中セッションに影響がないと確認できたら掃除と無停止リロードを実行して。"),
                    timestamp: isoAgo(minutes: 6)
                ),
                Self.toolMsg(
                    id: "c17-t1",
                    step: 1,
                    tools: ["read_file", "run_command"],
                    thinking: Self.t("先读取 scripts/prod_cleanup.sh 确认只匹配 TTL 已过期前缀，再跑只读 --dry-run 核验命中键数与预计释放内存。", "Inspecting scripts/prod_cleanup.sh to verify strict TTL prefix filtering before running a read-only --dry-run scan.", "`scripts/prod_cleanup.sh` のフィルタ条件を確認した上で、読み取り専用の `--dry-run` スキャンを実行します。"),
                    thinkingDuration: "5.2s",
                    toolDetails: [
                        ["name": "read_file", "summary": "scripts/prod_cleanup.sh (L1-L78)", "status": "done", "duration": "0.2s", "stepIndex": 1],
                        ["name": "run_command", "summary": "bash scripts/prod_cleanup.sh --dry-run --prefix 'v1:session:tmp:*'", "status": "done", "duration": "1.6s", "stepIndex": 1]
                    ]
                ),
                Self.agentMsg(
                    id: "c17-a2",
                    step: 2,
                    model: "gemini-4-neon-high",
                    modelName: "Gemini 4 Neon (High)",
                    text: Self.t(
                        """
                        只读预检已完成，确认零活跃会话受影响：

                        | 检查项 | 预检结果 | 影响 |
                        | --- | ---: | :---: |
                        | 过期键 | **14,218** | 🟢 零活跃 |
                        | 释放内存 | **412 MB** | 🟢 -18.4% |
                        | 网关重载 | `SIGHUP` | 🟢 零断连 |

                        接下来需要执行带写入与重载权限的生产脚本，请确认是否放行。
                        """,
                        """
                        Read-only preflight passed with zero active sessions affected:

                        | Check | Result | Impact |
                        | --- | ---: | :---: |
                        | Expired keys | **14,218** | 🟢 Safe |
                        | Freeable RAM | **412 MB** | 🟢 -18.4% |
                        | Reload | `SIGHUP` | 🟢 0 drops |

                        Ready to run the production cleanup & reload command — approve below or directly from your Lock Screen / Dynamic Island.
                        """,
                        """
                        読み取り専用の事前チェックが完了しました。稼働中セッションへの影響はありません：

                        | 項目 | 確認結果 | 影響 |
                        | --- | ---: | :---: |
                        | 期限切れキー | **14,218件** | 🟢 安全 |
                        | 解放メモリ | **412 MB** | 🟢 -18.4% |
                        | 再読込 | `SIGHUP` | 🟢 無停止 |

                        本番クリーンアップとリロードの実行許可をお願いします（ロック画面・Dynamic Island からもワンタップ承認可能です）。
                        """
                    ),
                    timestamp: isoAgo(minutes: 1)
                )
            ],
            pendingInteraction: [
                "type": "run_command",
                "trajectoryId": "demo-0017-cmd-approval",
                "stepIndex": 2,
                "title": Self.t("请求执行生产终端命令", "Run Production Command", "本番ターミナルコマンドの実行承認"),
                "target": "bash scripts/prod_cleanup.sh --prefix 'v1:session:tmp:*' --reload-gateway",
                "action": "run_command",
                "description": Self.t("将清理 14,218 个已过期 Redis 键并发送 SIGHUP 平滑重载网关配置。", "Deletes 14,218 expired Redis keys and sends SIGHUP for zero-downtime gateway reload.", "期限切れ Redis キー 14,218 件を削除し、SIGHUP で無停止リロードを実行します。"),
                "options": [
                    ["id": "1", "text": Self.t("允许本次执行", "Allow Once", "今回のみ許可")],
                    ["id": "2", "text": Self.t("本会话始终允许", "Always Allow in Session", "この会話で常に許可")],
                    ["id": "5", "text": Self.t("拒绝执行", "Deny", "実行を拒否"), "isDeny": true]
                ]
            ],
            canProceed: false,
            proceedArtifactUri: nil,
            runningTasks: [],
            queuedMessages: [],
            lastModified: ago(minutes: 1),
            unread: false,
            pinned: true,
            modelId: "gemini-4-neon-high",
            modelName: "Gemini 4 Neon (High)",
            startedAt: isoAgo(minutes: 6),
            contextMeter: Self.meter(used: 118_400, maxTokens: 512_000, tps: 136.8, ttft: 5.1),
            swarm: nil,
            subagents: [],
            parentConversationId: nil,
            subagentRole: nil,
            gitStatus: nil,
            changes: [],
            chapters: []
        )

        // 2. Pinned Streaming Attachment Upload + Running Task + Queued Message (Multigravity)
        let c01Upload = Conversation(
            id: "demo-0001-upload",
            title: Self.t("附件流式上传与 SHA-256 校验", "Streaming attachment upload & SHA-256", "添付ファイルのストリーム送信とSHA-256検証"),
            workspaceName: "Multigravity",
            workspaceUri: "file:///Users/demo/Projects/antigravity-mobile",
            status: "CASCADE_RUN_STATUS_RUNNING",
            duration: Self.t("运行中", "Running", "実行中"),
            messages: [
                Self.userMsg(
                    id: "c01-u0",
                    step: 0,
                    text: Self.t("帮我把移动端大文件附件上传接到本地网关 `Inbox`，落盘按 SHA-256 短 ID 去重，顺手把网关单测也跑一下。", "Wire mobile attachment uploads into the local gateway `Inbox` with SHA-256 short-ID deduplication, and run the gateway upload test suite.", "モバイル側の添付ファイル送信をローカルゲートウェイの `Inbox` につないで、SHA-256 短縮 ID で重複排除しつつゲートウェイの単体テストも回して。"),
                    timestamp: isoAgo(minutes: 9)
                ),
                Self.stepPlanMsg(
                    id: "c01-sp0",
                    step: 1,
                    items: [
                        ["key": "s1", "title": Self.t("检查网关与 iOS 附件上传协议", "Inspect gateway & iOS upload handlers", "ゲートウェイと iOS 添付処理を調査"), "status": "done"],
                        ["key": "s2", "title": Self.t("接入 io.TeeReader 流式 SHA-256 去重", "Wire streaming io.TeeReader SHA-256 dedup", "ストリーム SHA-256 重複排除を実装"), "status": "done"],
                        ["key": "s3", "title": Self.t("适配 iOS 并发直传与引用路径注入", "Adapt iOS concurrent upload & link injection", "iOS 並列送信と参照リンク挿入に対応"), "status": "done"],
                        ["key": "s4", "title": Self.t("运行网关附件上传单元测试", "Run gateway attachment upload unit tests", "添付アップロード単体テストを実行"), "status": "done"]
                    ]
                ),
                Self.toolMsg(
                    id: "c01-t1",
                    step: 1,
                    tools: ["read_file", "edit_file", "run_command"],
                    thinking: Self.t("读取 internal/gateway/attachments.go 和 iOS APIClient+Attachments.swift，改用流式 io.TeeReader 计算 SHA-256 避免大 PDF 占内存。", "Reading gateway & iOS attachment handlers; switching to streaming io.TeeReader SHA-256 hashing so >50MB PDFs never spike RAM.", "ゲートウェイと iOS の添付処理を確認し、大容量 PDF でもメモリを圧迫しないストリーム SHA-256 計算に変更します。"),
                    thinkingDuration: "6.4s",
                    toolDetails: [
                        ["name": "read_file", "summary": "internal/gateway/attachments.go (L1-L140)", "status": "done", "duration": "0.2s", "stepIndex": 1],
                        ["name": "edit_file", "summary": "internal/gateway/attachments.go (+46/-8)", "status": "done", "duration": "0.6s", "stepIndex": 1],
                        ["name": "edit_file", "summary": "ios/Multigravity/Services/APIClient+Attachments.swift (+19/-5)", "status": "done", "duration": "0.4s", "stepIndex": 1],
                        ["name": "run_command", "summary": "go test -v ./internal/gateway -run TestAttachmentUpload", "status": "running", "duration": "24s", "stepIndex": 1]
                    ]
                ),
                Self.agentMsg(
                    id: "c01-a2",
                    step: 2,
                    model: "gemini-4-neon-xhigh",
                    modelName: "Gemini 4 Neon (Extra)",
                    text: Self.t(
                        """
                        已完成前后两端流式附件上传与 SHA-256 去重：

                        1. **网关接收端点 (`POST /api/v1/attachments`)**：通过 `io.TeeReader` 边落盘边计算 8 位 SHA-256 短 ID，写入 `Inbox/2026-11/`，突破云端 20MB 限制。
                        2. **iOS 并发上传与路径注入**：支持相册、PDF、本地文件直传，自动写入引用行：
                           ```markdown
                           - [sales_q3.csv](/Users/demo/Multigravity/Inbox/2026-11/9f2a4c1b-sales_q3.csv) (1.8 KB)
                           ```
                        3. **后台测试运行中**：正在执行 `go test -v ./internal/gateway -run TestAttachmentUpload`，可点顶部任务条查看实时日志。
                        """,
                        """
                        Implemented end-to-end streaming uploads with SHA-256 deduplication:

                        1. **Gateway Endpoint (`POST /api/v1/attachments`)**: Streams via `io.TeeReader` to compute an 8-char SHA-256 ID while writing to `Inbox/2026-11/`, bypassing cloud 20MB caps.
                        2. **iOS Composer Integration**: Uploads Photos, PDFs, and Files directly over local LAN/Tailscale and injects clean local path links:
                           ```markdown
                           - [sales_q3.csv](/Users/demo/Multigravity/Inbox/2026-11/9f2a4c1b-sales_q3.csv) (1.8 KB)
                           ```
                        3. **Tests Running**: `go test -v ./internal/gateway -run TestAttachmentUpload` is active — tap the live task bar above to tail stdout.
                        """,
                        """
                        フロント・バックエンド双方のストリーム添付アップロードと SHA-256 重複排除を実装しました：

                        1. **ゲートウェイ API（`POST /api/v1/attachments`）**：`io.TeeReader` で書き込みと同時に 8 桁 SHA-256 短縮 ID を計算し `Inbox/2026-11/` へ保存（クラウド 20MB 制限なし）。
                        2. **iOS 直結アップロード**：写真・大容量 PDF・ファイルをローカル直結で送信し、参照パスを自動挿入：
                           ```markdown
                           - [sales_q3.csv](/Users/demo/Multigravity/Inbox/2026-11/9f2a4c1b-sales_q3.csv) (1.8 KB)
                           ```
                        3. **単体テスト実行中**：現在 `go test -v ./internal/gateway -run TestAttachmentUpload` が稼働中です。上部タスクバーからライブログを確認できます。
                        """
                    ),
                    timestamp: isoAgo(minutes: 2)
                )
            ],
            pendingInteraction: nil,
            canProceed: false,
            proceedArtifactUri: nil,
            runningTasks: [
                [
                    "id": "task-demo-upload-build",
                    "stepIndex": 1,
                    "commandLine": "go test -v ./internal/gateway -run TestAttachmentUpload",
                    "toolName": "run_command",
                    "toolSummary": Self.t("运行网关附件上传单测", "Run gateway attachment upload tests", "ゲートウェイ添付アップロード単体テストを実行"),
                    "toolAction": "running",
                    "startedAt": isoAgo(minutes: 2),
                    "isDaemon": false
                ]
            ],
            queuedMessages: [
                [
                    "id": "queue-demo-1",
                    "text": Self.t("测试通过后，再帮我把超过 100MB 时的分块提示和 HTTP 413 响应体补齐。", "Once tests pass, also add chunked upload hints and HTTP 413 payload details for >100MB files.", "テスト通過後、100MB 超え時のチャンク案内と HTTP 413 レスポンス仕様も追記して。"),
                    "createdAt": isoAgo(minutes: 1)
                ]
            ],
            lastModified: ago(minutes: 2),
            unread: false,
            pinned: true,
            modelId: "gemini-4-neon-xhigh",
            modelName: "Gemini 4 Neon (Extra)",
            startedAt: isoAgo(minutes: 9),
            contextMeter: Self.meter(used: 186_400, maxTokens: 512_000, tps: 132.9, ttft: 7.4),
            swarm: nil,
            subagents: [],
            parentConversationId: nil,
            subagentRole: nil,
            gitStatus: Self.demoGitState(
                branch: "feat/attachment-inbox",
                ahead: 2,
                modifiedCount: 2,
                prTitle: "feat(attachments): streaming SHA-256 inbox uploads without 20MB cap",
                prNumber: 421,
                prState: "OPEN"
            ),
            changes: [
                DemoFileDiff(
                    fileUri: "file:///Users/demo/Projects/antigravity-mobile/internal/gateway/attachments.go",
                    fileName: "internal/gateway/attachments.go",
                    actionType: "MODIFY",
                    additions: 46,
                    deletions: 8,
                    diffLines: [
                        ("UNCHANGED", "func handleUploadAttachment(w http.ResponseWriter, r *http.Request) {"),
                        ("DELETE", "    name := r.URL.Query().Get(\"name\")"),
                        ("INSERT", "    name := firstNonEmpty(r.Header.Get(\"X-File-Name\"), r.URL.Query().Get(\"name\"))"),
                        ("INSERT", "    id := shortSHA256Stream(r.Body, tmpFile)[:8]"),
                        ("INSERT", "    dest := filepath.Join(inboxDir, fmt.Sprintf(\"%s-%s\", id, sanitizeName(name)))"),
                        ("UNCHANGED", "    writeJSON(w, http.StatusOK, result)")
                    ]
                ),
                DemoFileDiff(
                    fileUri: "file:///Users/demo/Projects/antigravity-mobile/ios/Multigravity/Services/APIClient+Attachments.swift",
                    fileName: "ios/Multigravity/Services/APIClient+Attachments.swift",
                    actionType: "MODIFY",
                    additions: 19,
                    deletions: 5,
                    diffLines: [
                        ("UNCHANGED", "req.setValue(encodedName, forHTTPHeaderField: \"X-File-Name\")"),
                        ("INSERT", "req.setValue(AttachmentRules.mimeType(for: url), forHTTPHeaderField: \"Content-Type\")"),
                        ("INSERT", "return try JSONDecoder().decode(UploadedAttachment.self, from: data)")
                    ]
                )
            ],
            chapters: [
                ["startStepIndex": 0, "endStepIndexExclusive": 2, "title": Self.t("附件流式落盘与去重设计", "Streaming Inbox & SHA-256 Design", "ストリーム保存とSHA-256重複排除"), "generatedAt": isoAgo(minutes: 5)],
                ["startStepIndex": 2, "endStepIndexExclusive": 3, "title": Self.t("网关与 iOS 端实现及单测", "Gateway + iOS Impl & Tests", "ゲートウェイ・iOS実装とテスト"), "generatedAt": isoAgo(minutes: 2)]
            ]
        )

        // 3. Multi-turn Refactor + Chapters TOC + TurnOutline + Changes + PR #412
        let c10Turns = Conversation(
            id: "demo-0010-turns",
            title: Self.t("移动端网络重试与断线重连优化", "Mobile retry backoff & SSE reconnect", "モバイル通信リトライとSSE即時再接続"),
            workspaceName: "Multigravity",
            workspaceUri: "file:///Users/demo/Projects/antigravity-mobile",
            status: "CASCADE_RUN_STATUS_IDLE",
            duration: Self.t("24秒", "24s", "24秒"),
            messages: [
                Self.userMsg(id: "c10-u0", step: 0, text: Self.t("帮我排查一下 `APIClient` 弱网和切 Wi-Fi/5G 时的重试与重连逻辑，看看有哪些隐患。", "Audit `APIClient` retry and Wi-Fi/5G handoff reconnect behavior for reliability gaps.", "弱回線や Wi-Fi / 5G 切替時の `APIClient` リトライ・再接続処理に問題がないか調査して。"), timestamp: isoAgo(minutes: 45)),
                Self.toolMsg(
                    id: "c10-t1",
                    step: 1,
                    tools: ["grep_search", "read_file"],
                    thinking: Self.t("检索所有重试循环和 WebSocket/SSE 重连退避计时器，确认是否存在非幂等 POST 重放。", "Searching retry loops and stream reconnect timers to check for non-idempotent POST replays.", "リトライ処理とストリーム再接続タイマーを検索し、非冪等な POST の再送有無を確認します。"),
                    thinkingDuration: "4.1s",
                    toolDetails: [
                        ["name": "grep_search", "summary": "retry|reconnect in ios/Multigravity/Services/", "status": "done", "duration": "0.2s", "stepIndex": 1],
                        ["name": "read_file", "summary": "Services/StreamWatcher.swift (L1-L160)", "status": "done", "duration": "0.2s", "stepIndex": 1]
                    ]
                ),
                Self.agentMsg(
                    id: "c10-a2",
                    step: 2,
                    model: "gemini-4-neon-high",
                    modelName: "Gemini 4 Neon (High)",
                    text: Self.t(
                        """
                        排查 `Services/APIClient*.swift` 后定位到 3 个核心问题：

                        1. **401 / 502 重试未区分幂等方法**，超时后的 `POST` 发送请求会被重复提交。
                        2. **SSE 断线重连写死 3 秒延迟**，地铁切 5G 或回局域网恢复体感偏慢。
                        3. **大附件预览未复用内存缓存**，每次退出重进都会重新拉流。
                        """,
                        """
                        Found 3 reliability gaps across `Services/APIClient*.swift`:

                        1. **Non-idempotent retries on 401/502**: timed-out `POST` mutations were retried automatically.
                        2. **Fixed 3s SSE reconnect timer**: sluggish recovery when handing off between Wi-Fi and 5G.
                        3. **Uncached attachment previews**: large artifacts re-downloaded on every sheet reopen.
                        """,
                        """
                        `Services/APIClient*.swift` を調査し、3 つの信頼性課題を特定しました：

                        1. **401 / 502 リトライで冪等性を未判定**：タイムアウトした `POST` 更新系も自動再送されてしまいます。
                        2. **SSE 再接続が 3 秒固定**：Wi-Fi ⇔ 5G 切替時の復帰がワンテンポ遅れます。
                        3. **大型添付プレビューのキャッシュ未実装**：シートを開くたびに再取得が発生しています。
                        """
                    ),
                    timestamp: isoAgo(minutes: 44)
                ),
                Self.userMsg(id: "c10-u3", step: 3, text: Self.t("按指数退避 + 仅对 GET 幂等请求重试来改第 1 点，第 2 点接入 NWPathMonitor 立即重连，第 3 点加 64MB LRU 缓存。", "Use exponential backoff on idempotent GETs for #1, NWPathMonitor instant reconnect for #2, and a 64MB LRU cache for #3.", "1 は冪等な GET のみ指数バックオフ、2 は NWPathMonitor で即時再接続、3 は 64MB の LRU キャッシュを入れて進めて。"), timestamp: isoAgo(minutes: 39)),
                Self.toolMsg(
                    id: "c10-t4",
                    step: 4,
                    tools: ["write_file", "edit_file", "run_command"],
                    thinking: Self.t("新建 RetryPolicy.swift 严格限定 GET/HEAD，更新 StreamWatcher 监听 NWPathMonitor，并在 mac99 远端执行无头 xcodebuild 校验。", "Creating RetryPolicy.swift scoped to GET/HEAD, wiring NWPathMonitor in StreamWatcher, and verifying headless xcodebuild.", "GET/HEAD 限定の RetryPolicy.swift を作成し、StreamWatcher へ NWPathMonitor を組み込んでビルド検証します。"),
                    thinkingDuration: "5.8s",
                    toolDetails: [
                        ["name": "write_file", "summary": "ios/Multigravity/Services/RetryPolicy.swift (+42)", "status": "done", "duration": "0.3s", "stepIndex": 4],
                        ["name": "edit_file", "summary": "ios/Multigravity/Services/StreamWatcher.swift (+24/-9)", "status": "done", "duration": "0.4s", "stepIndex": 4],
                        ["name": "run_command", "summary": "ssh mac99 xcodebuild -scheme Multigravity build", "status": "done", "duration": "14.2s", "stepIndex": 4]
                    ]
                ),
                Self.agentMsg(
                    id: "c10-a5",
                    step: 5,
                    model: "gemini-4-neon-high",
                    modelName: "Gemini 4 Neon (High)",
                    text: Self.t(
                        """
                        已完成改造并通过编译校验：

                        - `RetryPolicy.swift`：仅对 `GET` / `HEAD` 启用 `0.4s -> 0.8s -> 1.6s` 退避，`POST` 绝不自动重发。
                        - `StreamWatcher.swift`：监听 `NWPathMonitor` 网络切换，接口恢复后立即重连。
                        - `PreviewCache.swift`：接入 64MB `NSCache` 预览缓存。
                        """,
                        """
                        Implemented all three fixes and verified a clean build:

                        - `RetryPolicy.swift`: `0.4s -> 0.8s -> 1.6s` backoff strictly scoped to `GET` / `HEAD`.
                        - `StreamWatcher.swift`: `NWPathMonitor` triggers instant reconnect on interface handoff.
                        - `PreviewCache.swift`: Added 64MB `NSCache` artifact buffer.
                        """,
                        """
                        3 点の改修とビルド検証を完了しました：

                        - `RetryPolicy.swift`：`GET` / `HEAD` に限定して `0.4s -> 0.8s -> 1.6s` の指数バックオフを適用（`POST` は再送なし）。
                        - `StreamWatcher.swift`：`NWPathMonitor` で回線切替を検知し、SSE を即時再接続。
                        - `PreviewCache.swift`：64MB 上限の `NSCache` プレビューキャッシュを追加。
                        """
                    ),
                    timestamp: isoAgo(minutes: 37)
                ),
                Self.userMsg(id: "c10-u6", step: 6, text: Self.t("再确认一下后台切回前台时，如果网关暂时不可达会不会连弹重复 Toast？", "Double-check foreground resume won't spam duplicate error toasts if the gateway is unreachable.", "バックグラウンドから復帰したとき、ゲートウェイ未接続でも重複トーストが出ないか確認して。"), timestamp: isoAgo(minutes: 31)),
                Self.agentMsg(
                    id: "c10-a7",
                    step: 7,
                    model: "gemini-4-neon-high",
                    modelName: "Gemini 4 Neon (High)",
                    text: Self.t("已加上 8 秒防抖窗口与 `lastErrorSignature` 去重，短时间内同一网关断连最多提示 1 次。本轮改动已合并至 PR `#412`。", "Added an 8s debounce window with `lastErrorSignature` deduplication so repeated failures surface at most once. Changes are merged in PR `#412`.", "8 秒のデバウンス窓と `lastErrorSignature` 重複排除を追加したため、短時間の連続切断でも通知は 1 回のみ表示されます。変更は PR `#412` にマージ済みです。"),
                    timestamp: isoAgo(minutes: 29)
                )
            ],
            pendingInteraction: nil,
            canProceed: false,
            proceedArtifactUri: nil,
            runningTasks: [],
            queuedMessages: [],
            lastModified: ago(minutes: 12),
            unread: false,
            pinned: false,
            modelId: "gemini-4-neon-high",
            modelName: "Gemini 4 Neon (High)",
            startedAt: isoAgo(minutes: 45),
            contextMeter: Self.meter(used: 238_400, maxTokens: 512_000, tps: 141.6, ttft: 4.9),
            swarm: nil,
            subagents: [],
            parentConversationId: nil,
            subagentRole: nil,
            gitStatus: Self.demoGitState(
                branch: "refactor/api-retry-backoff",
                ahead: 0,
                modifiedCount: 3,
                prTitle: "refactor(ios): exponential backoff retry + NWPathMonitor stream recovery",
                prNumber: 412,
                prState: "MERGED"
            ),
            changes: [
                DemoFileDiff(
                    fileUri: "file:///Users/demo/Projects/antigravity-mobile/ios/Multigravity/Services/RetryPolicy.swift",
                    fileName: "ios/Multigravity/Services/RetryPolicy.swift",
                    actionType: "CREATE",
                    additions: 42,
                    deletions: 0,
                    diffLines: [
                        ("INSERT", "struct RetryPolicy {"),
                        ("INSERT", "    static func delay(forAttempt attempt: Int) -> TimeInterval {"),
                        ("INSERT", "        min(0.4 * pow(2.0, Double(attempt)), 3.2)"),
                        ("INSERT", "    }"),
                        ("INSERT", "}")
                    ]
                ),
                DemoFileDiff(
                    fileUri: "file:///Users/demo/Projects/antigravity-mobile/ios/Multigravity/Services/StreamWatcher.swift",
                    fileName: "ios/Multigravity/Services/StreamWatcher.swift",
                    actionType: "MODIFY",
                    additions: 24,
                    deletions: 9,
                    diffLines: [
                        ("DELETE", "try? await Task.sleep(nanoseconds: 3_000_000_000)"),
                        ("INSERT", "pathMonitor.pathUpdateHandler = { [weak self] path in"),
                        ("INSERT", "    if path.status == .satisfied { self?.reconnectImmediately() }"),
                        ("INSERT", "}")
                    ]
                ),
                DemoFileDiff(
                    fileUri: "file:///Users/demo/Projects/antigravity-mobile/ios/Multigravity/Services/PreviewCache.swift",
                    fileName: "ios/Multigravity/Services/PreviewCache.swift",
                    actionType: "MODIFY",
                    additions: 18,
                    deletions: 4,
                    diffLines: [
                        ("INSERT", "cache.totalCostLimit = 64 * 1024 * 1024"),
                        ("UNCHANGED", "return cache.object(forKey: key as NSString)")
                    ]
                )
            ],
            chapters: [
                ["startStepIndex": 0, "endStepIndexExclusive": 3, "title": Self.t("诊断重试与缓存痛点", "Diagnose Retry & Cache Gaps", "リトライ＆キャッシュ課題の診断"), "generatedAt": isoAgo(minutes: 40)],
                ["startStepIndex": 3, "endStepIndexExclusive": 6, "title": Self.t("退避策略与 NWPathMonitor", "Backoff & NWPathMonitor", "指数バックオフと回線監視"), "generatedAt": isoAgo(minutes: 34)],
                ["startStepIndex": 6, "endStepIndexExclusive": 8, "title": Self.t("前台恢复防抖去重", "Foreground Toast Debounce", "復帰時トースト重複排除"), "generatedAt": isoAgo(minutes: 28)]
            ]
        )

        // 4. Dynamic Workflow + Clickable Subagents
        let c11Workflow = Conversation(
            id: "demo-0011-workflow",
            title: Self.t("风控引擎发布前并行质量门禁", "Risk engine pre-release quality gates", "リスクエンジン本番前並列品質ゲート"),
            workspaceName: Self.t("风控引擎", "Risk Engine", "リスクエンジン"),
            workspaceUri: "file:///Users/demo/Projects/risk-engine",
            status: "CASCADE_RUN_STATUS_RUNNING",
            duration: Self.t("运行中", "Running", "実行中"),
            messages: [
                Self.userMsg(
                    id: "c11-u0",
                    step: 0,
                    text: Self.t("/workflow 并发跑一轮风控引擎发布前质量门禁：安全审计、P99 压测分析，以及自动补全边界单测。", "/workflow Run pre-release quality gates on risk-engine in parallel: security audit, P99 profiling, and unit-test coverage.", "/workflow リリース前の品質ゲートをリスクエンジンで並列実行して：セキュリティ監査・P99 負荷解析・境界単体テストの自動補完。"),
                    timestamp: isoAgo(minutes: 26),
                    slashCommands: [["name": "workflow", "description": Self.t("动态编排多阶段子代理流水线", "Orchestrate staged subagent pipelines", "複数段階のサブエージェントパイプラインを動的編成")]]
                ),
                Message(
                    id: "c11-wf-1",
                    type: "workflow",
                    text: "",
                    toolCount: 1,
                    toolNames: ["run_workflow"],
                    stepIndex: 1,
                    model: "gemini-pro-orchestrator-neon",
                    modelName: "Orchestrator (Neon)",
                    timestamp: isoAgo(minutes: 25),
                    workflow: [
                        "description": Self.t("发版前并行门禁流水线：安全扫描 + 压测定位 -> 单测补全", "Pre-release parallel gate: security + profiling -> test authoring", "リリース前並列ゲート：セキュリティ監査＋負荷解析 → 単体テスト補完"),
                        "scriptPath": "scripts/quality_gate_workflow.py",
                        "phases": [
                            [
                                "name": Self.t("Phase 1 · 深度审计与性能采样", "Phase 1 · Security Audit & Profiling", "Phase 1 · セキュリティ監査＆性能プロファイリング"),
                                "status": "done",
                                "logs": [
                                    Self.t("[security-auditor] 参数化 SQL 与 RBAC 鉴权校验通过", "[security-auditor] Parameterized SQL & RBAC checks passed", "[security-auditor] パラメータ化 SQL と RBAC 認可チェック通過"),
                                    Self.t("[perf-profiler] EvaluateRiskBatch P99 = 18.4ms @ 500 QPS", "[perf-profiler] EvaluateRiskBatch P99 = 18.4ms @ 500 QPS", "[perf-profiler] EvaluateRiskBatch P99 = 18.4ms @ 500 QPS")
                                ],
                                "subagents": [
                                    [
                                        "conversationId": "demo-subagent-audit-1",
                                        "role": Self.t("安全审计：SQL 注入与越权", "Security: SQL Injection & AuthZ", "セキュリティ監査：SQLi ＆ 認可"),
                                        "typeName": "security-auditor",
                                        "modelTier": "pro",
                                        "status": "done",
                                        "stepCount": 3
                                    ],
                                    [
                                        "conversationId": "demo-subagent-perf-2",
                                        "role": Self.t("压测定位：P99 延迟火焰图", "Profiling: P99 Flamegraph", "負荷解析：P99 フレームグラフ"),
                                        "typeName": "perf-profiler",
                                        "modelTier": "flash",
                                        "status": "done",
                                        "stepCount": 3
                                    ]
                                ]
                            ],
                            [
                                "name": Self.t("Phase 2 · 回归单测自动补全", "Phase 2 · Automated Test Coverage", "Phase 2 · 回帰単体テスト自動補完"),
                                "status": "running",
                                "logs": [
                                    Self.t("[test-author] 正在写入 rule_eval_test.go 并运行 go test -race...", "[test-author] Writing rule_eval_test.go and running go test -race...", "[test-author] rule_eval_test.go を作成し go test -race を実行中...")
                                ],
                                "subagents": [
                                    [
                                        "conversationId": "demo-subagent-live-3",
                                        "role": Self.t("生成表驱动边界单测并运行 -race", "Write table-driven tests & run -race", "テーブル駆動テスト生成＆ -race 実行"),
                                        "typeName": "test-author",
                                        "modelTier": "inherit",
                                        "status": "running",
                                        "stepCount": 3
                                    ]
                                ]
                            ]
                        ],
                        "logs": [
                            Self.t("Phase 1 完成（2/2 子代理通过），正在推进 Phase 2...", "Phase 1 completed (2/2 passed); running Phase 2...", "Phase 1 完了（2/2 サブエージェント通過）、Phase 2 を進行中...")
                        ],
                        "status": "running",
                        "duration": Self.t("进行中 · 25秒", "Running · 25s", "実行中 · 25秒")
                    ]
                ),
                Self.subagentMsg(
                    id: "c11-s2",
                    step: 2,
                    conversationId: "demo-subagent-audit-1",
                    role: Self.t("安全审计专员", "Security Auditor", "セキュリティ監査担当"),
                    typeName: "security-auditor",
                    prompt: Self.t("检查 handlers/rule_eval.go 与 repo/score_store.go 的参数绑定与越权风险", "Inspect SQL parameter binding and AuthZ checks in handlers/rule_eval.go & repo/score_store.go", "handlers/rule_eval.go と repo/score_store.go の SQL パラメータバインドと認可漏れを検査"),
                    modelTier: "pro",
                    status: "completed",
                    stepCount: 3
                ),
                Self.subagentMsg(
                    id: "c11-s3",
                    step: 3,
                    conversationId: "demo-subagent-perf-2",
                    role: Self.t("性能压测专家", "Perf Profiler", "性能プロファイラ"),
                    typeName: "perf-profiler",
                    prompt: Self.t("分析 EvaluateRiskBatch 在 500 QPS 下的 CPU 热点与 GC 停顿", "Profile EvaluateRiskBatch CPU hotspots & GC pauses at 500 QPS", "500 QPS 時の EvaluateRiskBatch の CPU ホットスポットと GC 停止時間を計測"),
                    modelTier: "flash",
                    status: "completed",
                    stepCount: 3
                ),
                Self.subagentMsg(
                    id: "c11-s4",
                    step: 4,
                    conversationId: "demo-subagent-live-3",
                    role: Self.t("自动化测试工程师", "Test Engineer", "テスト自動化エンジニア"),
                    typeName: "test-author",
                    prompt: Self.t("根据发现的问题补全 rule_eval_test.go 并跑通并发检查", "Extend rule_eval_test.go and verify with race detector", "指摘内容をもとに rule_eval_test.go を拡充しレース検出テストを実行"),
                    modelTier: "inherit",
                    status: "running",
                    stepCount: 3
                ),
                Self.agentMsg(
                    id: "c11-a5",
                    step: 5,
                    model: "gemini-pro-orchestrator-neon",
                    modelName: "Orchestrator (Neon)",
                    text: Self.t("Phase 1 两个并行子代理已完成分析并汇总了根因，点按上方任意子代理条目即可查看完整上下文；Phase 2 正在补齐表驱动单测并运行 `-race` 验证。", "Phase 1 parallel subagents finished and reported root causes — tap any subagent row above to inspect its full transcript. Phase 2 is authoring table-driven tests and running `-race`.", "Phase 1 の並列サブエージェント 2 名が解析と原因特定を完了しました（上の各行をタップすると個別ログを確認できます）。現在 Phase 2 でテーブル駆動テスト追加と `-race` 検証を実行中です。"),
                    timestamp: isoAgo(minutes: 21)
                )
            ],
            pendingInteraction: nil,
            canProceed: false,
            proceedArtifactUri: nil,
            runningTasks: [],
            queuedMessages: [],
            lastModified: ago(minutes: 15),
            unread: false,
            pinned: false,
            modelId: "gemini-pro-orchestrator-neon",
            modelName: "Orchestrator (Neon)",
            startedAt: isoAgo(minutes: 26),
            contextMeter: Self.meter(used: 164_200, maxTokens: 512_000, tps: 134.8, ttft: 5.4),
            swarm: nil,
            subagents: [
                [
                    "conversationId": "demo-subagent-audit-1",
                    "typeName": "security-auditor",
                    "role": Self.t("安全审计专员", "Security Auditor", "セキュリティ監査担当"),
                    "prompt": Self.t("检查参数绑定与越权", "Check parameter binding & AuthZ", "パラメータバインド＆認可チェック"),
                    "modelTier": "pro",
                    "stepIndex": 2,
                    "status": "completed",
                    "stepCount": 3,
                    "title": Self.t("安全扫描：SQL 注入与越权", "Security Scan: SQL Injection & AuthZ", "セキュリティスキャン：SQLi ＆ 認可検証")
                ],
                [
                    "conversationId": "demo-subagent-perf-2",
                    "typeName": "perf-profiler",
                    "role": Self.t("性能压测专家", "Perf Profiler", "性能プロファイラ"),
                    "prompt": Self.t("分析 500 QPS 火焰图", "Profile 500 QPS flamegraph", "500 QPS フレームグラフ解析"),
                    "modelTier": "flash",
                    "stepIndex": 3,
                    "status": "completed",
                    "stepCount": 3,
                    "title": Self.t("压测分析：P99 延迟定位", "Load Profile: P99 Latency Hotspots", "負荷解析：P99 レイテンシ特定")
                ],
                [
                    "conversationId": "demo-subagent-live-3",
                    "typeName": "test-author",
                    "role": Self.t("自动化测试工程师", "Test Engineer", "テスト自動化エンジニア"),
                    "prompt": Self.t("补全边界单测", "Write edge-case tests", "境界値単体テスト補完"),
                    "modelTier": "inherit",
                    "stepIndex": 4,
                    "status": "running",
                    "stepCount": 3,
                    "title": Self.t("边界单测自动补全", "Edge-Case Unit Test Authoring", "境界値ユニットテスト自動補完")
                ]
            ],
            parentConversationId: nil,
            subagentRole: nil,
            gitStatus: nil,
            changes: [],
            chapters: []
        )

        // 5. Active Swarm 4-Tab Collaboration (/teamwork — activates SwarmBannerView + Hive Pixel Bees!)
        let c12Swarm = Conversation(
            id: "demo-0012-swarm",
            title: Self.t("离线草稿冲突三路合并", "Offline draft 3-way conflict merge", "オフライン下書きの3-way競合マージ"),
            workspaceName: "Multigravity",
            workspaceUri: "file:///Users/demo/Projects/antigravity-mobile",
            status: "CASCADE_RUN_STATUS_RUNNING",
            duration: Self.t("运行中", "Running", "実行中"),
            messages: [
                Self.userMsg(
                    id: "c12-u0",
                    step: 0,
                    text: Self.t("/teamwork 组建小队并行推进「离线草稿冲突合并」：后端加 CAS 乐观锁、iOS 补三路合并提示条、独立验证并发边界，所有进展同步到 Swarm 公告板。", "/teamwork Spin up a swarm for offline draft 3-way conflict merging: Go CAS versioning, iOS merge banner, and independent race verification on the shared board.", "/teamwork 「オフライン下書き 3-way 競合マージ」をスウォーム並列開発して：Go 楽観ロック、iOS マージバナー、並行競合テストを共有ボード連携で進めて。"),
                    timestamp: isoAgo(minutes: 18),
                    slashCommands: [["name": "teamwork", "description": Self.t("组建多代理协同团队与共享公告板", "Coordinate multiple agents via shared board", "共有ボードで複数エージェントを並列協調")]]
                ),
                Self.toolMsg(
                    id: "c12-t1",
                    step: 1,
                    tools: ["swarm", "post_message", "read_board"],
                    thinking: Self.t("按模块边界拆分 3 名并行 Worker：@backend 负责 Go 乐观锁，@ios-ui 负责 SwiftUI 三路合并，@verifier 负责交叉审校与 -race 验证。", "Splitting work across 3 isolated workers: @backend (Go CAS), @ios-ui (SwiftUI 3-way merge), and @verifier (cross-review & -race).", "責務を 3 名のワーカーに分割します：@backend（Go 楽観ロック）、@ios-ui（SwiftUI マージ UI）、@verifier（相互レビュー＆レース検証）。"),
                    thinkingDuration: "5.7s",
                    toolDetails: [
                        ["name": "swarm", "summary": "add backend (gemini-4-neon-high, share)", "status": "done", "duration": "0.8s", "stepIndex": 1],
                        ["name": "swarm", "summary": "add ios-ui (claude-opus-5-5-high, share)", "status": "done", "duration": "0.7s", "stepIndex": 1],
                        ["name": "swarm", "summary": "add verifier (gemini-4-hydrogen-high, inherit)", "status": "done", "duration": "0.6s", "stepIndex": 1],
                        ["name": "read_board", "summary": "seq 1..6 (4 active members)", "status": "done", "duration": "0.2s", "stepIndex": 1]
                    ]
                ),
                Self.agentMsg(
                    id: "c12-a2",
                    step: 2,
                    model: "gemini-pro-orchestrator-neon",
                    modelName: "Orchestrator (Neon)",
                    text: Self.t(
                        """
                        已组建 **4-Agent Swarm 协同小队**，各成员正在独立 worktree 并行推进：

                        - **`@backend`（Gemini 4 Neon · High）**：已完成 `/api/v1/drafts` 的 `version` CAS 校验与 `409 Conflict` 快照响应，正在补齐并发压测。
                        - **`@ios-ui`（Claude Opus 5.5 · High）**：正在打磨 `DraftStore.mergeRemote` 三路合并与断连草稿本地持久化，确保未发长文零丢失。
                        - **`@verifier`（Gemini 4 Hydrogen · High）**：已审校网关补丁，等待 iOS 编译通过后合流终验 PR `#418`。

                        顶部蜂群状态栏实时同步各成员状态，点按即可展开蜂巢视图、依赖星图与共享公告板。
                        """,
                        """
                        Spun up a **4-agent Swarm** working in isolated worktrees on the shared board:

                        - **`@backend` (Gemini 4 Neon · High)**: Landed `/api/v1/drafts` optimistic `version` CAS & `409 Conflict` snapshots; finalizing race stress tests.
                        - **`@ios-ui` (Claude Opus 5.5 · High)**: Wiring `DraftStore.mergeRemote` 3-way merge & local persistence so unsent drafts never drop on reconnect.
                        - **`@verifier` (Gemini 4 Hydrogen · High)**: Approved the Go patch; standing by to run cross-platform verification for PR `#418`.

                        Tap the live Swarm banner above to inspect the Hive bees, dependency graph, and live message board.
                        """,
                        """
                        **4 名の Swarm チーム**を独立ワークツリーで起動し、共有ボード上で並列進行中です：

                        - **`@backend`（Gemini 4 Neon · High）**：`/api/v1/drafts` の `version` 楽観ロックと `409 Conflict` スナップショット返却を実装済み。並行ストレステスト実行中。
                        - **`@ios-ui`（Claude Opus 5.5 · High）**：`DraftStore.mergeRemote` の 3-way マージと未送信下書きのローカル永続化を仕上げ中。
                        - **`@verifier`（Gemini 4 Hydrogen · High）**：Go パッチをレビュー承認済み。iOS ビルド通過後に PR `#418` を最終検証します。

                        上部のスウォームバナーをタップすると、ハイブ稼働状況・依存グラフ・共有ボードをリアルタイムで確認できます。
                        """
                    ),
                    timestamp: isoAgo(minutes: 3)
                )
            ],
            pendingInteraction: nil,
            canProceed: false,
            proceedArtifactUri: nil,
            runningTasks: [],
            queuedMessages: [],
            lastModified: ago(minutes: 3),
            unread: false,
            pinned: false,
            modelId: "gemini-pro-orchestrator-neon",
            modelName: "Orchestrator (Neon)",
            startedAt: isoAgo(minutes: 18),
            contextMeter: Self.meter(used: 194_800, maxTokens: 512_000, tps: 139.4, ttft: 6.1),
            swarm: Self.initialSwarmSnapshot(minutesAgo: 3),
            subagents: [],
            parentConversationId: nil,
            subagentRole: nil,
            gitStatus: Self.demoGitState(
                branch: "feat/swarm-draft-merge",
                ahead: 3,
                modifiedCount: 2,
                prTitle: "feat(drafts): optimistic concurrency merge for mobile offline drafts",
                prNumber: 418,
                prState: "OPEN"
            ),
            changes: [
                DemoFileDiff(
                    fileUri: "file:///Users/demo/Projects/antigravity-mobile/ios/Multigravity/Models/DraftStore.swift",
                    fileName: "ios/Multigravity/Models/DraftStore.swift",
                    actionType: "MODIFY",
                    additions: 31,
                    deletions: 7,
                    diffLines: [
                        ("INSERT", "func mergeRemote(_ remote: DraftSnapshot, local: DraftSnapshot) -> DraftMergeResult {"),
                        ("INSERT", "    if remote.version <= local.baseVersion { return .keepLocal(local) }"),
                        ("INSERT", "    return .conflict(local: local, server: remote)"),
                        ("INSERT", "}")
                    ]
                ),
                DemoFileDiff(
                    fileUri: "file:///Users/demo/Projects/antigravity-mobile/internal/proxy/drafts_cas.go",
                    fileName: "internal/proxy/drafts_cas.go",
                    actionType: "CREATE",
                    additions: 58,
                    deletions: 0,
                    diffLines: [
                        ("INSERT", "if req.BaseVersion < current.Version {"),
                        ("INSERT", "    writeJSON(w, http.StatusConflict, current)"),
                        ("INSERT", "    return"),
                        ("INSERT", "}")
                    ]
                )
            ],
            chapters: []
        )

        // 6. Error & Retry Recovery
        let c02Login500 = Conversation(
            id: "demo-0002-login-500",
            title: Self.t("登录接口 500 错误排查", "Auth login 500 incident triage", "ログインAPI 500エラー調査"),
            workspaceName: Self.t("风控引擎", "Risk Engine", "リスクエンジン"),
            workspaceUri: "file:///Users/demo/Projects/risk-engine",
            status: "CASCADE_RUN_STATUS_ERROR",
            duration: Self.t("7秒", "7s", "7秒"),
            messages: [
                Self.userMsg(
                    id: "c02-u0",
                    step: 0,
                    text: Self.t("线上 `/api/v2/auth/login` 在高峰期偶发 HTTP 500，日志里报错 `context deadline exceeded` 和 `pq: sorry, too many clients already`，帮我查下连接池配置。", "Production `/api/v2/auth/login` intermittently returns HTTP 500 with `context deadline exceeded` and `pq: sorry, too many clients already`. Check pool config.", "本番の `/api/v2/auth/login` でピーク時に HTTP 500 が断続発生中。ログに `context deadline exceeded` と `pq: sorry, too many clients already` が出てるのでコネクションプール設定を調べて。"),
                    timestamp: isoAgo(minutes: 28)
                ),
                Self.toolMsg(
                    id: "c02-t1",
                    step: 1,
                    tools: ["grep_search", "read_file", "call_mcp_tool"],
                    thinking: Self.t("检索 session_store 初始化参数并通过 postgres-prod MCP 探测当前连接上限。", "Checking session_store pool initialization and probing staging Postgres max_connections via MCP.", "session_store のプール設定を検索し、postgres-prod MCP で最大接続数を確認します。"),
                    thinkingDuration: "3.9s",
                    toolDetails: [
                        ["name": "grep_search", "summary": "SetMaxOpenConns|create_pool in internal/auth/", "status": "done", "duration": "0.2s", "stepIndex": 1],
                        ["name": "read_file", "summary": "internal/auth/session_store.go (L18-L64)", "status": "done", "duration": "0.1s", "stepIndex": 1],
                        ["name": "call_mcp_tool", "summary": "postgres-prod: query (SHOW max_connections)", "status": "error", "duration": "5.0s", "stepIndex": 1]
                    ]
                ),
                Self.errorMsg(
                    id: "c02-e2",
                    step: 2,
                    text: Self.t("连接远程风控测试库执行 `SHOW max_connections` 超时（已重试 3/3 次）：Postgres 连接池瞬时占满导致探针握手失败，你可以点击下方 **继续 (Continue)** 让我按本地代码分析完成修复补丁。", "Timed out connecting to remote staging DB for `SHOW max_connections` (3/3 retries): pool saturation blocked health probes. Tap **Continue** below to apply the local pool fix.", "ステージング DB への `SHOW max_connections` 実行がタイムアウトしました（リトライ 3/3 回）：Postgres 接続プール飽和でヘルスプローブがブロックされています。下の **Continue** をタップするとローカルのコード解析ベースで修正パッチを適用します。"),
                    attempts: 3,
                    max: 3,
                    errorKind: "network_timeout",
                    isOverflow: false
                )
            ],
            pendingInteraction: nil,
            canProceed: false,
            proceedArtifactUri: nil,
            runningTasks: [],
            queuedMessages: [],
            lastModified: ago(minutes: 21),
            unread: false,
            pinned: false,
            modelId: "claude-opus-5-5-high",
            modelName: "Claude Opus 5.5 (High)",
            startedAt: isoAgo(minutes: 28),
            contextMeter: Self.meter(used: 62_400, maxTokens: 200_000, tps: 68.2, ttft: 3.7),
            swarm: nil,
            subagents: [],
            parentConversationId: nil,
            subagentRole: nil,
            gitStatus: nil,
            changes: [],
            chapters: []
        )

        // 7. Interactive Multi-Question ask_question
        let c03Migration = Conversation(
            id: "demo-0003-migration",
            title: Self.t("订单表无锁复合索引迁移评审", "Zero-downtime orders index migration", "ordersテーブル無停止複合インデックス移行"),
            workspaceName: Self.t("风控引擎", "Risk Engine", "リスクエンジン"),
            workspaceUri: "file:///Users/demo/Projects/risk-engine",
            status: "CASCADE_RUN_STATUS_RUNNING",
            duration: Self.t("等待确认", "Needs Input", "確認待ち"),
            messages: [
                Self.userMsg(
                    id: "c03-u0",
                    step: 0,
                    text: Self.t("`orders` 表目前有 1.8 亿行，要在生产环境加上 `(created_at DESC, status)` 复合索引并迁移历史状态，帮我评估停机窗口和回滚方案。", "The `orders` table has 180M rows. Plan a zero-downtime migration adding `(created_at DESC, status)` plus rollback safeguards.", "`orders` テーブルが 1.8 億行あります。本番無停止で `(created_at DESC, status)` 複合インデックスを追加し履歴ステータスもバックフィルしたい。ダウンタイムとロールバック方針を評価して。"),
                    timestamp: isoAgo(minutes: 33)
                ),
                Self.toolMsg(
                    id: "c03-t1",
                    step: 1,
                    tools: ["read_file", "call_mcp_tool", "ask_question"],
                    thinking: Self.t("检查已有迁移脚本版本与当前索引分布，确认无锁建索引与历史回填并发度需与用户拍板。", "Inspecting existing migration history and index cardinality; preparing 3 rollout decisions for confirmation.", "既存マイグレーション履歴とインデックス構成を確認し、本番適用方針の 3 点をユーザーに確認します。"),
                    thinkingDuration: "4.8s",
                    toolDetails: [
                        ["name": "read_file", "summary": "migrations/0018_orders_status.sql", "status": "done", "duration": "0.1s", "stepIndex": 1],
                        ["name": "call_mcp_tool", "summary": "postgres-prod: describe_table (orders)", "status": "done", "duration": "0.6s", "stepIndex": 1]
                    ]
                ),
                Self.agentMsg(
                    id: "c03-a2",
                    step: 2,
                    model: "gemini-4-carbon-high",
                    modelName: "Gemini 4 Carbon (High)",
                    text: Self.t("我已梳理好建索引与状态回填方案，在生成最终生产 SQL 之前，需要你确认 3 个上线策略（支持多选和底部补充说明）：", "I prepared the indexing and backfill strategy. Please confirm 3 rollout choices below before I generate the final production SQL:", "インデックス作成とバックフィル計画を整理しました。最終的な本番 SQL を生成する前に、以下の 3 つのリリース方針を確認してください（複数選択・自由記述に対応）："),
                    timestamp: isoAgo(minutes: 31)
                )
            ],
            pendingInteraction: [
                "type": "ask_question",
                "trajectoryId": "demo-0003-migration",
                "stepIndex": 2,
                "title": Self.t("确认生产数据库迁移策略", "Confirm Production Migration Strategy", "本番 DB マイグレーション方針の確認"),
                "description": Self.t("请逐项选择线上执行的并发控制与回滚方式", "Select concurrency limits and rollback safeguards", "同時実行制御とロールバック保護ポリシーを選択してください"),
                "questions": [
                    [
                        "question": Self.t("第 1 题（单选）：主表 1.8 亿行建议采用哪种方式创建索引？", "Question 1 (Single): How should we create the index on 180M rows?", "質問 1（単一選択）：1.8 億行テーブルのインデックス作成方式はどれにしますか？"),
                        "isMultiSelect": false,
                        "defaultOptionId": "q1-concurrent",
                        "hasWriteIn": true,
                        "writeInLabel": Self.t("其他指定窗口时间", "Custom maintenance window", "その他のメンテナンス時間帯を指定"),
                        "writeInPlaceholder": Self.t("例如：凌晨 03:00 执行并限流...", "e.g. Run at 03:00 UTC with rate limit...", "例：深夜 03:00 JST にレート制限付きで実行..."),
                        "options": [
                            ["id": "q1-concurrent", "text": Self.t("CREATE INDEX CONCURRENTLY 无锁并发建索引（推荐）", "CREATE INDEX CONCURRENTLY (Recommended)", "CREATE INDEX CONCURRENTLY 無停止並列作成（推奨）")],
                            ["id": "q1-shadow", "text": Self.t("gh-ost 影子表双写切换", "Shadow-table swap via gh-ost", "gh-ost シャドウテーブル切替")],
                            ["id": "q1-window", "text": Self.t("申请凌晨低峰期只读窗口快速构建", "Schedule off-peak read-only window", "深夜低負荷帯の読み取り専用ウィンドウで高速構築")]
                        ]
                    ],
                    [
                        "question": Self.t("第 2 题（多选）：回滚与发布前检查需要包含哪些安全守卫？", "Question 2 (Multi): Which pre-flight & rollback safeguards should be included?", "質問 2（複数選択）：リリース前チェックとロールバック保護に何を含めますか？"),
                        "isMultiSelect": true,
                        "options": [
                            ["id": "q2-snapshot", "text": Self.t("迁移前自动创建 RDS 只读快照", "Take RDS snapshot before migration", "移行前に RDS スナップショットを自動取得")],
                            ["id": "q2-rollback-sql", "text": Self.t("随发布单附带 DROP INDEX 回滚脚本", "Bundle reversible DROP INDEX rollback SQL", "リリース手順に DROP INDEX ロールバック SQL を同梱")],
                            ["id": "q2-dryrun", "text": Self.t("先在灰度库回放近 24 小时慢 SQL", "Replay last 24h slow queries on staging", "ステージングで直近 24 時間のスロークエリを再生検証")]
                        ]
                    ],
                    [
                        "question": Self.t("第 3 题（单选）：历史 status 空值回填批次大小设为多少？", "Question 3 (Single): Backfill batch size for legacy null status rows?", "質問 3（単一選択）：過去の NULL ステータス埋め戻しバッチサイズは？"),
                        "isMultiSelect": false,
                        "defaultOptionId": "q3-5k",
                        "options": [
                            ["id": "q3-5k", "text": Self.t("每批 5,000 行，批间休眠 100ms（平滑 IO）", "5,000 rows/batch with 100ms sleep (smooth I/O)", "5,000 行 / バッチ・100ms 待機（I/O 平準化）")],
                            ["id": "q3-20k", "text": Self.t("每批 20,000 行，限速主从延迟 < 500ms", "20,000 rows/batch capped at <500ms replica lag", "20,000 行 / バッチ・レプリカ遅延 < 500ms 制御")]
                        ]
                    ]
                ]
            ],
            canProceed: false,
            proceedArtifactUri: nil,
            runningTasks: [],
            queuedMessages: [],
            lastModified: ago(minutes: 25),
            unread: false,
            pinned: false,
            modelId: "gemini-4-carbon-high",
            modelName: "Gemini 4 Carbon (High)",
            startedAt: isoAgo(minutes: 33),
            contextMeter: Self.meter(used: 148_500, maxTokens: 512_000, tps: 118.4, ttft: 5.8),
            swarm: nil,
            subagents: [],
            parentConversationId: nil,
            subagentRole: nil,
            gitStatus: nil,
            changes: [],
            chapters: []
        )

        // 8. Data Report: Unread + Attached CSV/MD + Inline Chart PNG + 4-page PDF
        let c04Sales = Conversation(
            id: "demo-0004-sales",
            title: Self.t("Q3 区域经营分析与汇报 PDF", "Q3 regional revenue report & PDF", "Q3 エリア別売上分析と経営陣向けPDF"),
            workspaceName: Self.t("数据分析台", "Analytics Hub", "データ分析ハブ"),
            workspaceUri: "file:///Users/demo/Projects/data-analytics",
            status: "CASCADE_RUN_STATUS_IDLE",
            duration: Self.t("19秒", "19s", "19秒"),
            messages: [
                Self.userMsg(
                    id: "c04-u0",
                    step: 0,
                    text: Self.t(
                        """
                        📎 附件（已上传到本机，可直接读取）：
                        - [销售数据.csv](file:///Users/demo/Documents/销售数据.csv) (512 B)
                        - [会议纪要.md](file:///Users/demo/Documents/会议纪要.md) (1.1 KB)

                        请基于这两份原始数据，对比 Q1-Q3 华东/华南/华北收入趋势，配一张可视化柱状图，并输出一份可直接给管理层汇报的 4 页 PDF 报告。
                        """,
                        """
                        📎 Attachments (uploaded locally):
                        - [sales_q3.csv](file:///Users/demo/Documents/sales_q3.csv) (512 B)
                        - [meeting_notes.md](file:///Users/demo/Documents/meeting_notes.md) (1.1 KB)

                        Compare Q1–Q3 regional revenue trends across West/East/Central, generate a bar chart, and export a 4-page executive PDF report.
                        """,
                        """
                        📎 添付ファイル（ローカルにアップロード済み）：
                        - [sales_q3.csv](file:///Users/demo/Documents/sales_q3.csv) (512 B)
                        - [meeting_notes.md](file:///Users/demo/Documents/meeting_notes.md) (1.1 KB)

                        この 2 つのデータをもとに Q1〜Q3 の関東・関西・中部エリア別売上推移を比較し、棒グラフ付きの経営陣向け 4 ページ PDF レポートを作成して。
                        """
                    ),
                    timestamp: isoAgo(minutes: 62)
                ),
                Self.toolMsg(
                    id: "c04-t1",
                    step: 1,
                    tools: ["read_file", "run_command", "write_file"],
                    thinking: Self.t("读取两份附件原始数据，汇总三季度区域环比与客单价，调用 Python 脚本渲染趋势图并合成 4 页 PDF 报告。", "Parsing attached CSV and review notes, computing QoQ regional deltas, plotting the bar chart, and exporting a 4-page PDF.", "添付 CSV と議事録を読み込み、エリア別四半期推移を集計して棒グラフと 4 ページ PDF を生成します。"),
                    thinkingDuration: "5.1s",
                    toolDetails: [
                        ["name": "read_file", "summary": "Documents/sales_q3.csv & meeting_notes.md", "status": "done", "duration": "0.2s", "stepIndex": 1],
                        ["name": "run_command", "summary": "python3 scripts/build_q3_report.py --chart --pdf", "status": "done", "duration": "2.4s", "stepIndex": 1]
                    ]
                ),
                Self.agentMsg(
                    id: "c04-a2",
                    step: 2,
                    model: "gemini-4-neon-high",
                    modelName: "Gemini 4 Neon (High)",
                    text: Self.t(
                        """
                        已完成 Q1–Q3 销售归因分析并打包了 **4 页高管汇报 PDF**（点击下方文件名或图表即可全屏预览与分享导出）：

                        | 区域 | Q1 (万) | Q2 (万) | Q3 (万) | 环比增速 |
                        | --- | ---: | ---: | ---: | ---: |
                        | 华东区 | 240 | 288 | 356 | **+23.6%** |
                        | 华南区 | 195 | 224 | 271 | +21.0% |
                        | 华北区 | 162 | 175 | 209 | +19.4% |

                        ![Q1-Q3 分区销售增长趋势图](file:///Users/demo/Documents/sales_chart.png)

                        ### 生成产物与原始文档
                        - 📄 [三季度销售报告.pdf](file:///Users/demo/Documents/三季度销售报告.pdf)（共 4 页：封面概览 / 核心趋势图表 / 区域拆解 / Q4 行动建议）
                        - 📊 [sales_chart.png](file:///Users/demo/Documents/sales_chart.png)
                        - 📑 [销售数据.csv](file:///Users/demo/Documents/销售数据.csv) · [会议纪要.md](file:///Users/demo/Documents/会议纪要.md)
                        """,
                        """
                        Completed Q1–Q3 revenue attribution and packaged a **4-page executive PDF report** (tap any file or chart below for full-screen Quick Look):

                        | Region | Q1 ($K) | Q2 ($K) | Q3 ($K) | QoQ Growth |
                        | --- | ---: | ---: | ---: | ---: |
                        | West Coast | 240 | 288 | 356 | **+23.6%** |
                        | East Coast | 195 | 224 | 271 | +21.0% |
                        | Central | 162 | 175 | 209 | +19.4% |

                        ![Q1-Q3 Regional Sales Trend](file:///Users/demo/Documents/sales_chart.png)

                        ### Deliverables & Source Files
                        - 📄 [q3_sales_report.pdf](file:///Users/demo/Documents/q3_sales_report.pdf) (4 pages: Cover, Charts, Regional Breakdown, Q4 Plan)
                        - 📊 [sales_chart.png](file:///Users/demo/Documents/sales_chart.png)
                        - 📑 [sales_q3.csv](file:///Users/demo/Documents/sales_q3.csv) · [meeting_notes.md](file:///Users/demo/Documents/meeting_notes.md)
                        """,
                        """
                        Q1〜Q3 の売上要因分析を完了し、**経営陣向け 4 ページ PDF レポート**を生成しました（ファイル名やグラフをタップするとフルスクリーン表示・共有できます）：

                        | エリア | Q1 (百万円) | Q2 (百万円) | Q3 (百万円) | 前四半期比 |
                        | --- | ---: | ---: | ---: | ---: |
                        | 関東エリア | 240 | 288 | 356 | **+23.6%** |
                        | 関西エリア | 195 | 224 | 271 | +21.0% |
                        | 中部エリア | 162 | 175 | 209 | +19.4% |

                        ![Q1-Q3 エリア別売上推移グラフ](file:///Users/demo/Documents/sales_chart.png)

                        ### 成果物と元データ
                        - 📄 [q3_sales_report.pdf](file:///Users/demo/Documents/q3_sales_report.pdf)（全 4 ページ：サマリー / 推移グラフ / エリア別詳細 / Q4 施策）
                        - 📊 [sales_chart.png](file:///Users/demo/Documents/sales_chart.png)
                        - 📑 [sales_q3.csv](file:///Users/demo/Documents/sales_q3.csv) · [meeting_notes.md](file:///Users/demo/Documents/meeting_notes.md)
                        """
                    ),
                    artifacts: [
                        [
                            "uri": Self.t("file:///Users/demo/Documents/三季度销售报告.pdf", "file:///Users/demo/Documents/q3_sales_report.pdf", "file:///Users/demo/Documents/q3_sales_report.pdf"),
                            "title": Self.t("三季度销售报告.pdf", "q3_sales_report.pdf", "q3_sales_report.pdf"),
                            "summary": Self.t("2026 Q1-Q3 区域经营分析与 Q4 资源排期报告（4 页 PDF）", "2026 Q1–Q3 regional sales analysis and Q4 action plan (4-page PDF)", "2026 Q1〜Q3 エリア別売上分析および Q4 アクション計画（4 ページ PDF）"),
                            "userFacing": true
                        ]
                    ],
                    timestamp: isoAgo(minutes: 58)
                )
            ],
            pendingInteraction: nil,
            canProceed: false,
            proceedArtifactUri: nil,
            runningTasks: [],
            queuedMessages: [],
            lastModified: ago(minutes: 58),
            unread: true,
            pinned: false,
            modelId: "gemini-4-neon-high",
            modelName: "Gemini 4 Neon (High)",
            startedAt: isoAgo(minutes: 62),
            contextMeter: Self.meter(used: 126_800, maxTokens: 512_000, tps: 138.2, ttft: 4.4),
            swarm: nil,
            subagents: [],
            parentConversationId: nil,
            subagentRole: nil,
            gitStatus: nil,
            changes: [],
            chapters: []
        )

        // 9. Authentic Antigravity Implementation Plan + Mermaid + Proceed Approval Card (Multigravity)
        let c05Redesign = Conversation(
            id: "demo-0005-redesign",
            title: Self.t("边缘网关缓存与 SSR 架构重构", "Edge SSR & canary caching plan", "Edge SSR・カナリアキャッシュ刷新計画"),
            workspaceName: "Multigravity",
            workspaceUri: "file:///Users/demo/Projects/antigravity-mobile",
            status: "CASCADE_RUN_STATUS_IDLE",
            duration: Self.t("14秒", "14s", "14秒"),
            messages: [
                Self.userMsg(
                    id: "c05-u0",
                    step: 0,
                    text: Self.t("/plan 帮我先梳理官网边缘缓存与首页 SSR 改造方案，输出 `implementation_plan.md` 列出基线指标、灰度切流拓扑和回滚门禁，我审完点 Proceed 再动手。", "/plan Draft an `implementation_plan.md` for our Edge SSR & canary caching rollout with real LCP baselines, routing topology, and rollback gates before editing code.", "/plan コード変更の前に、公式サイトの Edge SSR とカナリアキャッシュ導入に向けた `implementation_plan.md`（実測ベースライン・切替構成・ロールバック条件）を作って。レビューして Proceed を押したら実装に入ろう。"),
                    timestamp: isoAgo(minutes: 18),
                    slashCommands: [["name": "plan", "description": Self.t("先拆解架构、关键路径与风险，确认后再动手改代码", "Plan carefully before executing a task", "実装前に構成・マイルストーン・リスクを整理して合意形成")]]
                ),
                Self.toolMsg(
                    id: "c05-t1",
                    step: 1,
                    tools: ["read_file", "grep_search", "write_file"],
                    thinking: Self.t("汇总近 7 天 Cloudflare Web Vitals 基线与当前静态打包产物体积，拆解 Edge Worker 灰度路由、ISR 60s 缓存与 5 秒自动回退方案写入 implementation_plan.md。", "Analyzing 7-day Web Vitals baseline and bundle footprint; structuring Edge Worker canary routing, 60s ISR cache, and 5s auto-rollback inside implementation_plan.md.", "直近 7 日間の Web Vitals ベースラインとバンドル構成を分析し、Edge Worker カナリア配信・60秒 ISR・5秒自動ロールバックを `implementation_plan.md` にまとめます。"),
                    thinkingDuration: "7.8s",
                    toolDetails: [
                        ["name": "read_file", "summary": "web/next.config.ts & worker/edge_router.ts", "status": "done", "duration": "0.3s", "stepIndex": 1],
                        ["name": "write_file", "summary": "brain/demo-0005/implementation_plan.md (RequestFeedback=true)", "status": "done", "duration": "0.4s", "stepIndex": 1]
                    ]
                ),
                Self.agentMsg(
                    id: "c05-a2",
                    step: 2,
                    model: "gemini-4-neon-xhigh",
                    modelName: "Gemini 4 Neon (Extra)",
                    text: Self.t(
                        """
                        已完成基线审计并起草实施计划 [implementation_plan.md](file:///Users/demo/.gemini/antigravity/brain/demo-0005/implementation_plan.md)，核心灰度切流拓扑如下：

                        ```mermaid
                        graph TD
                            U[访客流量] --> EDGE[Edge Worker]
                            EDGE -->|10% 灰度| NEW[Next.js 16 SSR]
                            EDGE -->|90% 基线| OLD[静态资源包]
                            NEW -->|LCP 达标| PROMOTE[全量 100%]
                        ```

                        | 指标 | 基线 P75 | 目标 |
                        | --- | ---: | ---: |
                        | 首页 LCP | `2.64 s` | **`1.20 s`** |
                        | 核心 JS | `184 KB` | **`95 KB`** |
                        | 异常回退 | `~8 min` | **`5 s`** |

                        请审阅上方计划文档，确认无误后点击下方 **Proceed** 启动分阶段编码与自动化验收。
                        """,
                        """
                        Baseline audit complete and implementation plan drafted in [implementation_plan.md](file:///Users/demo/.gemini/antigravity/brain/demo-0005/implementation_plan.md). Core canary routing pipeline:

                        ```mermaid
                        graph TD
                            U[Visitor Traffic] --> EDGE[Edge Worker]
                            EDGE -->|10% Canary| NEW[Next.js 16 SSR]
                            EDGE -->|90% Baseline| OLD[Static Bundle]
                            NEW -->|LCP Pass| PROMOTE[Promote 100%]
                        ```

                        | Metric | P75 Now | Target |
                        | --- | ---: | ---: |
                        | Home LCP | `2.64 s` | **`1.20 s`** |
                        | Crit. JS | `184 KB` | **`95 KB`** |
                        | Rollback | `~8 min` | **`5 s`** |

                        Review the artifact above and tap **Proceed** below to begin phased implementation.
                        """,
                        """
                        ベースライン調査を完了し、実装計画書 [implementation_plan.md](file:///Users/demo/.gemini/antigravity/brain/demo-0005/implementation_plan.md) を作成しました。カナリア切替フローは以下のとおりです：

                        ```mermaid
                        graph TD
                            U[アクセス流入] --> EDGE[Edge Worker]
                            EDGE -->|10% カナリア| NEW[Next.js 16 SSR]
                            EDGE -->|90% 現行| OLD[静的バンドル]
                            NEW -->|LCP 合格| PROMOTE[全量 100%]
                        ```

                        | 指標 | 現行 P75 | 目標 |
                        | --- | ---: | ---: |
                        | LCP | `2.64 s` | **`1.20 s`** |
                        | 初期 JS | `184 KB` | **`95 KB`** |
                        | 切り戻し | `約8分` | **`5 s`** |

                        計画書をご確認のうえ、問題なければ下の **Proceed** をタップして段階的実装を開始してください。
                        """
                    ),
                    artifacts: [
                        [
                            "uri": "file:///Users/demo/.gemini/antigravity/brain/demo-0005/implementation_plan.md",
                            "title": "Implementation Plan",
                            "summary": Self.t("官网 Edge Worker 10%→100% 灰度切流、Next.js 16 流式 SSR + 60s ISR 缓存与 5 秒自动熔断回滚实施方案（含验证计划）。", "Phased Edge Worker 10%→100% canary rollout, Next.js 16 streaming SSR + 60s ISR cache, and 5s automated rollback plan.", "Edge Worker 10%→100% カナリア配信・Next.js 16 ストリーミング SSR + 60秒 ISR キャッシュ・5秒自動ロールバック実装計画。"),
                            "requestFeedback": true,
                            "userFacing": true
                        ]
                    ],
                    timestamp: isoAgo(minutes: 14)
                )
            ],
            pendingInteraction: nil,
            canProceed: true,
            proceedArtifactUri: "file:///Users/demo/.gemini/antigravity/brain/demo-0005/implementation_plan.md",
            runningTasks: [],
            queuedMessages: [],
            lastModified: ago(minutes: 14),
            unread: true,
            pinned: false,
            modelId: "gemini-4-neon-xhigh",
            modelName: "Gemini 4 Neon (Extra)",
            startedAt: isoAgo(minutes: 18),
            contextMeter: Self.meter(used: 156_200, maxTokens: 512_000, tps: 131.4, ttft: 7.2),
            swarm: nil,
            subagents: [],
            parentConversationId: nil,
            subagentRole: nil,
            gitStatus: nil,
            changes: [],
            chapters: []
        )

        // 10. Receipt OCR with 2 Image Previews & Reconciliation Table
        let c06Invoice = Conversation(
            id: "demo-0006-invoice",
            title: Self.t("差旅发票 OCR 识别与报销汇总", "Travel receipt OCR & reconciliation", "出張領収書のOCR読取と経費精算"),
            workspaceName: Self.t("数据分析台", "Analytics Hub", "データ分析ハブ"),
            workspaceUri: "file:///Users/demo/Projects/data-analytics",
            status: "CASCADE_RUN_STATUS_IDLE",
            duration: Self.t("9秒", "9s", "9秒"),
            messages: [
                Self.userMsg(
                    id: "c06-u0",
                    step: 0,
                    text: Self.t("帮我识别这两张出差报销发票图片，提取开票商户、发票号和含税金额，做成报销对账表。", "Extract merchants, receipt numbers, and totals from these two travel receipts into a clean expense table.", "この 2 枚の出張領収書を OCR 読み取りして、店舗名・伝票番号・税込金額を経費精算テーブルにまとめて。"),
                    imageUrls: [
                        "file:///Users/demo/Documents/invoice_coffee.jpg",
                        "file:///Users/demo/Documents/invoice_taxi.jpg"
                    ],
                    timestamp: isoAgo(minutes: 95)
                ),
                Self.toolMsg(
                    id: "c06-t1",
                    step: 1,
                    tools: ["read_file", "call_mcp_tool"],
                    thinking: Self.t("调用本地 OCR 解析两张票据图片并核验票号、日期与价税合计字段。", "Running OCR extraction across both receipt photos and validating totals.", "2 枚の領収書画像を OCR 解析し、伝票番号と税込合計を突合します。"),
                    thinkingDuration: "3.1s",
                    toolDetails: [
                        ["name": "read_file", "summary": "invoice_coffee.jpg & invoice_taxi.jpg", "status": "done", "duration": "0.2s", "stepIndex": 1],
                        ["name": "call_mcp_tool", "summary": "xiaomi_ocr: ocr_document (2 images)", "status": "done", "duration": "1.8s", "stepIndex": 1]
                    ]
                ),
                Self.agentMsg(
                    id: "c06-a2",
                    step: 2,
                    model: "gemini-4-neon-high",
                    modelName: "Gemini 4 Neon (High)",
                    text: Self.t(
                        """
                        已完成 **2 张报销发票** OCR 提取与验真（点上方图片可双指缩放查看原票细节）：

                        | 序号 | 报销类别 | 开票商户 / 行程 | 发票号码 | 含税金额 | 验真状态 |
                        | :---: | --- | --- | --- | ---: | :---: |
                        | 1 | 业务招待 / 餐饮 | 星河精品咖啡（北京国贸店） | `NO.20261108-88412` | **¥158.00** | ✅ 已验真 |
                        | 2 | 市内交通 / 出租 | 京B·82716 首都机场 → 国贸CBD | `NO.20261108-51904` | **¥116.00** | ✅ 已验真 |

                        **合计报销金额：¥274.00**。如需导出报销汇总 CSV，直接回复即可。
                        """,
                        """
                        Extracted & verified **2 travel receipts** (tap images above to zoom in):

                        | # | Category | Merchant / Route | Receipt No. | Total | Status |
                        | :---: | --- | --- | --- | ---: | :---: |
                        | 1 | Team Coffee | Starburst Specialty Coffee · Downtown | `NO.20261108-88412` | **$23.00** | ✅ Verified |
                        | 2 | Airport Transit | Cab #82716 · SFO Terminal 2 → SoMa | `NO.20261108-51904` | **$51.00** | ✅ Verified |

                        **Reimbursable Total: $74.00**. Reply "Export CSV" to package for finance.
                        """,
                        """
                        **2 枚の領収書** の OCR 抽出と検証が完了しました（上の画像をタップして拡大確認できます）：

                        | # | 費目 | 店舗 / 乗車区間 | 伝票番号 | 税込金額 | 状態 |
                        | :---: | --- | --- | --- | ---: | :---: |
                        | 1 | 会議費 / カフェ | スターバースト珈琲 渋谷スクランブル店 | `NO.20261108-88412` | **¥2,850** | ✅ 確認済み |
                        | 2 | 旅費交通費 | 品川 500 あ 8271 · 羽田空港T2 → 渋谷 | `NO.20261108-51904` | **¥9,000** | ✅ 確認済み |

                        **精算合計：¥11,850**。経理提出用の CSV 出力が必要ならそのまま返信してください。
                        """
                    ),
                    timestamp: isoAgo(minutes: 92)
                )
            ],
            pendingInteraction: nil,
            canProceed: false,
            proceedArtifactUri: nil,
            runningTasks: [],
            queuedMessages: [],
            lastModified: ago(minutes: 92),
            unread: false,
            pinned: false,
            modelId: "gemini-4-neon-high",
            modelName: "Gemini 4 Neon (High)",
            startedAt: isoAgo(minutes: 95),
            contextMeter: Self.meter(used: 78_200, maxTokens: 512_000, tps: 140.4, ttft: 3.9),
            swarm: nil,
            subagents: [],
            parentConversationId: nil,
            subagentRole: nil,
            gitStatus: nil,
            changes: [],
            chapters: []
        )

        // 11. Pending Workflow Approval Interaction (4-button Workflow Gate)
        let c13WorkflowApproval = Conversation(
            id: "demo-0013-wf-approval",
            title: Self.t("发布前自动化回归与灰度审批", "Pre-release canary pipeline approval", "リリース前カナリア検証ワークフロー承認"),
            workspaceName: Self.t("部署脚本", "Deploy Scripts", "デプロイスクリプト"),
            workspaceUri: "file:///Users/demo/Projects/deploy-scripts",
            status: "CASCADE_RUN_STATUS_RUNNING",
            duration: Self.t("等待审批", "Awaiting Approval", "承認待ち"),
            messages: [
                Self.userMsg(
                    id: "c13-u0",
                    step: 0,
                    text: Self.t("跑一下 `canary_rollout.py` 灰度发布流水线：先扫描过期 Redis 会话键，再切 10% 流量并做冒烟验证，正式执行前向我申请审批。", "Run `canary_rollout.py` to scan expired Redis session keys, shift 10% canary traffic, and execute smoke probes — ask for approval before running.", "`canary_rollout.py` のカナリア配信パイプラインを回したい。期限切れ Redis キー確認・10% 切替・スモーク検証の前に実行承認リクエストを送って。"),
                    timestamp: isoAgo(minutes: 8)
                ),
                Self.toolMsg(
                    id: "c13-t1",
                    step: 1,
                    tools: ["read_file", "run_command"],
                    thinking: Self.t("读取 deploy/canary_rollout.py 验证并发上限与只读 preflight，并封装为 run_workflow 审批请求等待用户放行。", "Inspecting deploy/canary_rollout.py concurrency bounds and staging dry-run output before requesting run_workflow approval.", "`deploy/canary_rollout.py` の並列制限とドライラン結果を確認し、`run_workflow` の実行承認を申請します。"),
                    thinkingDuration: "4.5s",
                    toolDetails: [
                        ["name": "read_file", "summary": "deploy/canary_rollout.py (L1-L46)", "status": "done", "duration": "0.2s", "stepIndex": 1],
                        ["name": "run_command", "summary": "redis-cli --scan --pattern 'v1:session:tmp:*' | wc -l", "status": "done", "duration": "1.1s", "stepIndex": 1]
                    ]
                ),
                Self.agentMsg(
                    id: "c13-a2",
                    step: 2,
                    model: "claude-opus-5-5-high",
                    modelName: "Claude Opus 5.5 (High)",
                    text: Self.t("预检查完成：命中 `v1:session:tmp:*` 过期键 14,200 条，随后将执行 10% 灰度放量与 18 项冒烟断言。请在下方审批卡片中确认是否运行该工作流。", "Preflight complete: 14,200 expired `v1:session:tmp:*` keys matched, followed by 10% canary shift and 18 smoke checks. Approve or deny the workflow below.", "事前チェック完了：`v1:session:tmp:*` 配下の期限切れキー 14,200 件を確認後、10% カナリア配信と 18 項目のスモーク検証を実行します。下の承認カードから許可または拒否を選択してください。"),
                    timestamp: isoAgo(minutes: 6)
                )
            ],
            pendingInteraction: [
                "type": "workflow_approval",
                "trajectoryId": "demo-0013-wf-approval",
                "stepIndex": 2,
                "title": Self.t("高危环境变更 · 请求执行工作流", "Production Change · Workflow Approval", "本番環境変更 · ワークフロー実行承認"),
                "description": Self.t("将清理过期会话缓存前缀、切 10% 灰度流量并触发网关热重载。", "Cleans expired Redis session prefixes, shifts 10% canary traffic, and hot-reloads gateway.", "期限切れ Redis キャッシュを削除し、10% カナリア配信と本番ゲートウェイのホットリロードを実行します。"),
                "commandLine": "python3 deploy/canary_rollout.py --traffic 10% --reload-gateway",
                "workflowApproval": [
                    "description": Self.t("生产网关灰度放量与缓存清理 · 预计影响 14,200 个过期键，活跃 SSE 连接零中断。", "Canary Rollout & Cache Cleanup · Affects 14,200 expired TTL keys; active SSE streams stay connected.", "本番カナリア配信＆キャッシュ掃除 · 対象 14,200 キー、稼働中 SSE 接続は維持されます。"),
                    "scriptPath": "deploy/canary_rollout.py",
                    "script": """
                    #!/usr/bin/env python3
                    from workflow import phase, run_command

                    with phase("preflight"):
                        run_command("redis-cli --scan --pattern 'v1:session:tmp:*' | wc -l")
                    with phase("canary_and_reload"):
                        run_command("bash scripts/prod_cleanup.sh --prefix 'v1:session:tmp:*' --reload-gateway")
                    """,
                    "estimatedPhases": [
                        Self.t("Preflight 扫描 (14,200 keys)", "Preflight Scan (14,200 keys)", "事前スキャン (14,200 keys)"),
                        Self.t("清理缓存并灰度切流", "Clean Cache & Canary Shift", "掃除＆カナリア切替")
                    ],
                    "maxConcurrency": 2
                ],
                "options": [
                    ["id": "1", "text": Self.t("允许本次执行", "Allow Once", "今回のみ許可")],
                    ["id": "2", "text": Self.t("始终信任本工作流", "Always Trust Workflow", "このワークフローを常に許可")],
                    ["id": "5", "text": Self.t("拒绝并中止", "Deny & Abort", "実行を拒否"), "isDeny": true]
                ]
            ],
            canProceed: false,
            proceedArtifactUri: nil,
            runningTasks: [],
            queuedMessages: [],
            lastModified: ago(minutes: 6),
            unread: false,
            pinned: false,
            modelId: "claude-opus-5-5-high",
            modelName: "Claude Opus 5.5 (High)",
            startedAt: isoAgo(minutes: 8),
            contextMeter: Self.meter(used: 54_800, maxTokens: 200_000, tps: 71.2, ttft: 3.6),
            swarm: nil,
            subagents: [],
            parentConversationId: nil,
            subagentRole: nil,
            gitStatus: nil,
            changes: [],
            chapters: []
        )

        // 12. Running Background Tasks + Daemon Logs + Native Desktop-Parity Step Plan & Live Focus Timer
        let c14PlanItems: [[String: Any]] = [
            ["key": "hmr", "title": Self.t("启动 Vite 5173 热更新守护进程", "Start Vite 5173 HMR dev daemon", "Vite 5173 HMR 常駐サーバーを起動"), "status": "done"],
            ["key": "bind", "title": Self.t("验证局域网绑定与端口健康探针", "Verify LAN bind & port health probe", "LAN バインドとポート疎通を検証"), "status": "done"],
            ["key": "race", "title": Self.t("执行 Go -race 并发竞态回归测试", "Run Go -race concurrency test suite", "Go -race 並行レース回帰テストを実行"), "status": "in_progress"],
            ["key": "report", "title": Self.t("汇总测试报告并推送移动端提醒", "Summarize results & send mobile alert", "検証結果を集計しモバイル通知を送信"), "status": "pending"]
        ]
        let c14ActivePlan: [String: Any] = [
            "anchorStepIndex": 1,
            "items": c14PlanItems,
            "doneCount": 2,
            "totalCount": 4,
            "skippedCount": 0,
            "failedCount": 0
        ]
        let c14FocusText = Self.t(
            "正在运行 ./internal/cockpit/... Go 竞态回归测试",
            "Running Go -race concurrency suite on ./internal/cockpit/...",
            "./internal/cockpit/... の Go -race 並行テストを実行中"
        )
        let c14Running = Conversation(
            id: "demo-0014-running",
            title: Self.t("本地 HMR 守护进程与竞态测试", "Local HMR dev server & race suite", "ローカルHMR常駐サーバーとレース検証"),
            workspaceName: Self.t("部署脚本", "Deploy Scripts", "デプロイスクリプト"),
            workspaceUri: "file:///Users/demo/Projects/deploy-scripts",
            status: "CASCADE_RUN_STATUS_RUNNING",
            duration: Self.t("运行中", "Running", "実行中"),
            messages: [
                Self.userMsg(
                    id: "c14-u0",
                    step: 0,
                    text: Self.t("把 Vite 热更新守护进程和 Go `-race` 并发回归测试同时跑起来，我在外面随时看步骤进度和实时日志，不需要的话直接手机停掉。", "Start the Vite HMR dev server daemon and the Go `-race` test suite so I can track step progress, tail logs, and stop tasks individually from mobile.", "Vite の HMR 常駐サーバーと Go の `-race` 並行回帰テストを同時に走らせて。外出先からステップ進捗とライブログ確認・個別停止ができるようにしたい。"),
                    timestamp: isoAgo(minutes: 4)
                ),
                Self.stepPlanMsg(
                    id: "c14-sp1",
                    step: 1,
                    items: c14PlanItems
                ),
                Self.toolMsg(
                    id: "c14-t2",
                    step: 2,
                    tools: ["run_command", "manage_task"],
                    thinking: Self.t("以 IsDaemon=true 拉起 Vite 5173 开发服务并确认端口就绪，现正持续监听 ./internal/cockpit/... 的 Go -race 并发测试输出。", "Started Vite HMR daemon on port 5173 and verified readiness; now streaming ./internal/cockpit/... Go -race test output.", "Vite HMR デーモンをポート 5173 で起動して疎通確認済み。現在 `./internal/cockpit/...` の Go -race テスト出力を監視中です。"),
                    thinkingDuration: "2.3s",
                    toolDetails: [
                        ["name": "run_command", "summary": "npm run dev -- --host 0.0.0.0 --port 5173 (daemon)", "status": "done", "duration": "1.8s", "stepIndex": 1],
                        ["name": "run_command", "summary": "go test -race -count=1 ./internal/cockpit/...", "status": "running", "duration": "48s", "stepIndex": 2]
                    ],
                    focusText: c14FocusText,
                    startedAt: isoAgo(minutes: 0.8)
                )
            ],
            pendingInteraction: nil,
            canProceed: false,
            proceedArtifactUri: nil,
            runningTasks: [
                [
                    "id": "task-vite-daemon",
                    "stepIndex": 1,
                    "commandLine": "npm run dev -- --host 0.0.0.0 --port 5173",
                    "toolName": "run_command",
                    "toolSummary": Self.t("Web 控制台 HMR 守护进程", "Web Console HMR Daemon", "Web コンソール HMR デーモン"),
                    "toolAction": "running",
                    "startedAt": isoAgo(minutes: 6),
                    "isDaemon": true
                ],
                [
                    "id": "task-go-race",
                    "stepIndex": 2,
                    "commandLine": "go test -race -count=1 ./internal/cockpit/...",
                    "toolName": "run_command",
                    "toolSummary": Self.t("Go 并发竞态测试", "Go Race-Detector Suite", "Go 並行レース検出テスト"),
                    "toolAction": "running",
                    "startedAt": isoAgo(minutes: 0.8),
                    "isDaemon": false
                ]
            ],
            queuedMessages: [],
            lastModified: ago(minutes: 2),
            unread: false,
            pinned: false,
            modelId: "gemini-4-hydrogen-high",
            modelName: "Gemini 4 Hydrogen (High)",
            startedAt: isoAgo(minutes: 4),
            contextMeter: Self.meter(used: 71_400, maxTokens: 512_000, tps: 168.0, ttft: 2.2),
            swarm: nil,
            subagents: [],
            parentConversationId: nil,
            subagentRole: nil,
            gitStatus: nil,
            changes: [],
            chapters: [],
            activeFocus: c14FocusText,
            activeFocusStartedAt: isoAgo(minutes: 0.8),
            activePlan: c14ActivePlan
        )

        // 13. Context Overflow + /compact rescue
        let c15Overflow = Conversation(
            id: "demo-0015-overflow",
            title: Self.t("反欺诈规则全量日志字段对比", "Fraud rule schema drift audit", "不正検知ルール全量ログ差分監査"),
            workspaceName: Self.t("风控引擎", "Risk Engine", "リスクエンジン"),
            workspaceUri: "file:///Users/demo/Projects/risk-engine",
            status: "CASCADE_RUN_STATUS_ERROR",
            duration: Self.t("上下文已满", "Context Full", "コンテキスト満杯"),
            messages: [
                Self.userMsg(
                    id: "c15-u0",
                    step: 0,
                    text: Self.t("把 `internal/rules/` 目录下 42 个反欺诈规则脚本和最近一周的压测 JSONL 全量读进来对比字段差异。", "Load all 42 fraud-rule scripts in `internal/rules/` alongside last week's load-test JSONL logs and compare schema drift.", "`internal/rules/` 配下の不正検知ルール 42 本と先週の負荷テスト JSONL ログを全量読み込んでスキーマ差分を比較して。"),
                    timestamp: isoAgo(minutes: 130)
                ),
                Self.toolMsg(
                    id: "c15-t1",
                    step: 1,
                    tools: ["find_by_name", "read_file", "run_command"],
                    thinking: Self.t("批量读取 42 份规则文件与 7 天 JSONL 采样日志，当前上下文 token 接近窗口上限。", "Loading 42 rule files and 7-day JSONL benchmark dumps; context utilization approaching hard ceiling.", "42 本のルール定義と 7 日分の JSONL ログを読み込んだ結果、コンテキスト上限に近づいています。"),
                    thinkingDuration: "6.2s"
                ),
                Self.errorMsg(
                    id: "c15-e2",
                    step: 2,
                    text: Self.t("当前上下文已占用 **96.2% (192,400 / 200,000 tokens)**，触发 `context_overflow` 保护。建议发送 **`/compact`** 压缩早期工具调用日志，即可立刻释放约 70% 上下文继续分析。", "Context usage reached **96.2% (192,400 / 200,000 tokens)** (`context_overflow`). Send **`/compact`** below to compress verbose tool output and reclaim ~70% context immediately.", "コンテキスト使用率が **96.2%（192,400 / 200,000 tokens）** に達し `context_overflow` 保護が発動しました。下で **`/compact`** を送信すると冗長なツール出力を圧縮し、約 70% の枠を即座に解放して続行できます。"),
                    attempts: 1,
                    max: 1,
                    errorKind: "context_overflow",
                    isOverflow: true
                )
            ],
            pendingInteraction: nil,
            canProceed: false,
            proceedArtifactUri: nil,
            runningTasks: [],
            queuedMessages: [],
            lastModified: ago(minutes: 126),
            unread: false,
            pinned: false,
            modelId: "claude-opus-5-5-high",
            modelName: "Claude Opus 5.5 (High)",
            startedAt: isoAgo(minutes: 130),
            contextMeter: Self.meter(used: 192_400, maxTokens: 200_000, tps: 63.5, ttft: 4.8, isOverflow: true),
            swarm: nil,
            subagents: [],
            parentConversationId: nil,
            subagentRole: nil,
            gitStatus: nil,
            changes: [],
            chapters: []
        )

        // 14. Code Blocks (diff / SQL / bash) + Pre-populated Local Draft
        let c07Python = Conversation(
            id: "demo-0007-python",
            title: Self.t("交易流水特征管道向量化提速", "Vectorize 2M-row feature pipeline", "取引ログ200万件の特徴量パイプライン高速化"),
            workspaceName: Self.t("风控引擎", "Risk Engine", "リスクエンジン"),
            workspaceUri: "file:///Users/demo/Projects/risk-engine",
            status: "CASCADE_RUN_STATUS_IDLE",
            duration: Self.t("11秒", "11s", "11秒"),
            messages: [
                Self.userMsg(
                    id: "c07-u0",
                    step: 0,
                    text: Self.t("我们的特征清洗脚本跑 200 万条流水要 18 分钟，帮我用批量 SQL 聚合 + NumPy 向量化改写一下。", "Our feature-prep script takes 18 minutes for 2M ledger rows. Rewrite it with set-based SQL + vectorized NumPy.", "200 万件の取引ログを処理する特徴量スクリプトが 18 分かかってます。SQL ウィンドウ集計＋NumPy ベクトル化で書き直して。"),
                    timestamp: isoAgo(minutes: 165)
                ),
                Self.toolMsg(
                    id: "c07-t1",
                    step: 1,
                    tools: ["read_file", "run_command", "edit_file"],
                    thinking: Self.t("先跑 cProfile 确认 94% 耗时卡在逐行查库，改为单条窗口 SQL 下推加 NumPy 向量化衰减计算。", "Profiling confirms 94% wall time in row-by-row DB fetches; rewriting into window SQL + vectorized NumPy.", "cProfile で 94% が行単位 DB クエリと判明。ウィンドウ SQL 集約＋NumPy ベクトル化へ書き換えます。"),
                    thinkingDuration: "4.3s",
                    toolDetails: [
                        ["name": "read_file", "summary": "pipelines/feature_prep.py (L1-L148)", "status": "done", "duration": "0.1s", "stepIndex": 1],
                        ["name": "run_command", "summary": "pytest tests/test_feature_vectorized.py -v", "status": "done", "duration": "2.6s", "stepIndex": 1]
                    ]
                ),
                Self.agentMsg(
                    id: "c07-a2",
                    step: 2,
                    model: "gemini-4-neon-high",
                    modelName: "Gemini 4 Neon (High)",
                    text: Self.t(
                        """
                        已定位瓶颈在逐行 Python `for` 循环查库（200 万次往返），改造后端到端耗时由 **18 分钟降至 42 秒**：

                        ```sql
                        -- 1. 下推到数据库做窗口预聚合
                        SELECT user_id,
                               COUNT(*) FILTER (WHERE amount > 5000) AS high_amt_cnt,
                               AVG(amount) OVER (PARTITION BY user_id ORDER BY ts ROWS BETWEEN 20 PRECEDING AND CURRENT ROW) AS rolling_avg
                        FROM txn_ledger
                        WHERE ts >= NOW() - INTERVAL '30 days';
                        ```

                        ```diff
                        - for row in rows:
                        -     score += slow_python_decay(row.amount, row.days)
                        + scores = np.exp(-0.08 * days_arr) * np.log1p(amounts_arr)
                        ```

                        ```bash
                        pytest tests/test_feature_vectorized.py -v --benchmark-min-rounds=5
                        ```
                        """,
                        """
                        Eliminated the row-by-row Python loop (2M DB round-trips), cutting runtime from **18m down to 42s**:

                        ```sql
                        -- 1. Push window aggregations into Postgres
                        SELECT user_id,
                               COUNT(*) FILTER (WHERE amount > 5000) AS high_amt_cnt,
                               AVG(amount) OVER (PARTITION BY user_id ORDER BY ts ROWS BETWEEN 20 PRECEDING AND CURRENT ROW) AS rolling_avg
                        FROM txn_ledger
                        WHERE ts >= NOW() - INTERVAL '30 days';
                        ```

                        ```diff
                        - for row in rows:
                        -     score += slow_python_decay(row.amount, row.days)
                        + scores = np.exp(-0.08 * days_arr) * np.log1p(amounts_arr)
                        ```

                        ```bash
                        pytest tests/test_feature_vectorized.py -v --benchmark-min-rounds=5
                        ```
                        """,
                        """
                        ボトルネックだった Python の 1 行ごとの DB 参照ループ（200 万往復）を解消し、処理時間を **18 分 → 42 秒** に短縮しました：

                        ```sql
                        -- 1. ウィンドウ集計を PostgreSQL 側へプッシュダウン
                        SELECT user_id,
                               COUNT(*) FILTER (WHERE amount > 5000) AS high_amt_cnt,
                               AVG(amount) OVER (PARTITION BY user_id ORDER BY ts ROWS BETWEEN 20 PRECEDING AND CURRENT ROW) AS rolling_avg
                        FROM txn_ledger
                        WHERE ts >= NOW() - INTERVAL '30 days';
                        ```

                        ```diff
                        - for row in rows:
                        -     score += slow_python_decay(row.amount, row.days)
                        + scores = np.exp(-0.08 * days_arr) * np.log1p(amounts_arr)
                        ```

                        ```bash
                        pytest tests/test_feature_vectorized.py -v --benchmark-min-rounds=5
                        ```
                        """
                    ),
                    timestamp: isoAgo(minutes: 160)
                )
            ],
            pendingInteraction: nil,
            canProceed: false,
            proceedArtifactUri: nil,
            runningTasks: [],
            queuedMessages: [],
            lastModified: ago(minutes: 160),
            unread: false,
            pinned: false,
            modelId: "gemini-4-neon-high",
            modelName: "Gemini 4 Neon (High)",
            startedAt: isoAgo(minutes: 165),
            contextMeter: Self.meter(used: 114_200, maxTokens: 512_000, tps: 135.4, ttft: 4.1),
            swarm: nil,
            subagents: [],
            parentConversationId: nil,
            subagentRole: nil,
            gitStatus: nil,
            changes: [],
            chapters: []
        )

        // 15. Rich Rendering Showcase: Unread + Jupyter Notebook + LaTeX + Table (Pure Chat)
        let c16Notebook = Conversation(
            id: "demo-0016-notebook",
            title: Self.t("多资产凯利仓位推导与回测脚本", "Multi-asset Kelly sizing & notebook", "マルチアセット・ケリー配分とノートブック"),
            workspaceName: "Chat",
            workspaceUri: "",
            status: "CASCADE_RUN_STATUS_IDLE",
            duration: Self.t("12秒", "12s", "12秒"),
            messages: [
                Self.userMsg(
                    id: "c16-u0",
                    step: 0,
                    text: Self.t("帮我推导一下带交易成本的凯利公式（Kelly Criterion）多资产扩展形式，并整理成一份可直接打开预览的 Jupyter Notebook。", "Derive the multi-asset Kelly Criterion with transaction costs and package it into a previewable Jupyter Notebook.", "取引コスト込みのマルチアセット版ケリー基準（Kelly Criterion）を導出して、そのままプレビューできる Jupyter Notebook にまとめて。"),
                    timestamp: isoAgo(minutes: 210)
                ),
                Self.agentMsg(
                    id: "c16-a1",
                    step: 1,
                    model: "gemini-4-carbon-high",
                    modelName: "Gemini 4 Carbon (High)",
                    text: Self.t(
                        """
                        在存在滑点与单边手续费率 $c$ 时，单资产最优杠杆率 $f^*$ 满足期望对数收益最大化：

                        $$
                        f^* = \\frac{\\mu - r_f - c \\cdot \\tau}{\\sigma^2}
                        $$

                        拓展到相关性矩阵为 $\\Sigma$ 的多资产组合，向量解为：

                        $$
                        \\mathbf{f}^* = \\gamma \\cdot \\Sigma^{-1}(\\boldsymbol{\\mu} - r_f \\mathbf{1} - \\mathbf{c})
                        $$

                        | 参数档位 | 年化期望收益 | 最大回撤估计 | 建议系数 |
                        | --- | ---: | ---: | :---: |
                        | Full Kelly | 24.8% | -34.2% | $\\gamma = 1.0$ |
                        | Half Kelly（推荐） | 18.6% | -15.9% | $\\gamma = 0.5$ |
                        | Quarter Kelly | 11.2% | -7.4% | $\\gamma = 0.25$ |

                        完整分 Cell 回测脚本已写入：
                        📓 [analytics.ipynb](file:///Users/demo/Documents/analytics.ipynb)
                        """,
                        """
                        Accounting for slippage and turnover fee $c$, single-asset optimal leverage $f^*$ maximizes expected log growth:

                        $$
                        f^* = \\frac{\\mu - r_f - c \\cdot \\tau}{\\sigma^2}
                        $$

                        Extending to a correlated multi-asset universe with covariance $\\Sigma$:

                        $$
                        \\mathbf{f}^* = \\gamma \\cdot \\Sigma^{-1}(\\boldsymbol{\\mu} - r_f \\mathbf{1} - \\mathbf{c})
                        $$

                        | Regime | Expected CAGR | Est. Max Drawdown | Scalar |
                        | --- | ---: | ---: | :---: |
                        | Full Kelly | 24.8% | -34.2% | $\\gamma = 1.0$ |
                        | Half Kelly (Rec.) | 18.6% | -15.9% | $\\gamma = 0.5$ |
                        | Quarter Kelly | 11.2% | -7.4% | $\\gamma = 0.25$ |

                        Full notebook ready for native cell preview:
                        📓 [analytics.ipynb](file:///Users/demo/Documents/analytics.ipynb)
                        """,
                        """
                        スリッページと片道手数料率 $c$ を考慮すると、単一資産の最適レバレッジ $f^*$ は期待対数収益を最大化します：

                        $$
                        f^* = \\frac{\\mu - r_f - c \\cdot \\tau}{\\sigma^2}
                        $$

                        共分散行列 $\\Sigma$ を持つマルチアセットへ拡張するとベクトル解は以下になります：

                        $$
                        \\mathbf{f}^* = \\gamma \\cdot \\Sigma^{-1}(\\boldsymbol{\\mu} - r_f \\mathbf{1} - \\mathbf{c})
                        $$

                        | 設定 | 期待年率リターン | 最大ドローダウン予測 | 係数 |
                        | --- | ---: | ---: | :---: |
                        | Full Kelly | 24.8% | -34.2% | $\\gamma = 1.0$ |
                        | Half Kelly（推奨） | 18.6% | -15.9% | $\\gamma = 0.5$ |
                        | Quarter Kelly | 11.2% | -7.4% | $\\gamma = 0.25$ |

                        各セルをネイティブプレビューできるノートブックを保存しました：
                        📓 [analytics.ipynb](file:///Users/demo/Documents/analytics.ipynb)
                        """
                    ),
                    artifacts: [
                        [
                            "uri": "file:///Users/demo/Documents/analytics.ipynb",
                            "title": "analytics.ipynb",
                            "summary": Self.t("多资产凯利公式分档回测 Jupyter Notebook", "Fractional Kelly multi-asset simulation notebook", "マルチアセット・ケリー配分シミュレーション Notebook"),
                            "userFacing": true
                        ]
                    ],
                    timestamp: isoAgo(minutes: 205)
                )
            ],
            pendingInteraction: nil,
            canProceed: false,
            proceedArtifactUri: nil,
            runningTasks: [],
            queuedMessages: [],
            lastModified: ago(minutes: 205),
            unread: false,
            pinned: false,
            modelId: "gemini-4-carbon-high",
            modelName: "Gemini 4 Carbon (High)",
            startedAt: isoAgo(minutes: 210),
            contextMeter: Self.meter(used: 88_400, maxTokens: 512_000, tps: 121.6, ttft: 5.2),
            swarm: nil,
            subagents: [],
            parentConversationId: nil,
            subagentRole: nil,
            gitStatus: nil,
            changes: [],
            chapters: []
        )

        // 16. CSS Safe-Area Bug Fix + Code Diff (Multigravity)
        let c08LoginStyle = Conversation(
            id: "demo-0008-login-style",
            title: Self.t("修复 Web 登录页安全区与暗黑对比度", "Fix mobile safe-area & dark CSS", "Webログイン画面のSafe-Area・ダークCSS修正"),
            workspaceName: "Multigravity",
            workspaceUri: "file:///Users/demo/Projects/antigravity-mobile",
            status: "CASCADE_RUN_STATUS_IDLE",
            duration: Self.t("6秒", "6s", "6秒"),
            messages: [
                Self.userMsg(
                    id: "c08-u0",
                    step: 0,
                    text: Self.t("iPhone 刘海屏 / 灵动岛机型打开 Web 登录页时，底部提交按钮被安全区遮挡，深色模式下输入框边框也太淡，帮忙修一下。", "On Dynamic Island iPhones, the web login CTA overlaps the home indicator and input borders fade out in Dark Mode.", "Dynamic Island 搭載の iPhone で Web ログイン画面を開くと、下部の送信ボタンがホームバーと重なり、ダークモードでは入力枠の線も薄すぎるので直して。"),
                    timestamp: isoAgo(minutes: 280)
                ),
                Self.toolMsg(
                    id: "c08-t1",
                    step: 1,
                    tools: ["read_file", "edit_file"],
                    thinking: Self.t("检查 web/src/styles/login.css 的视口高度与暗黑配色 token，替换为 100dvh 与 safe-area-inset-bottom。", "Inspecting login.css viewport height and dark-mode border contrast; switching to 100dvh and safe-area-inset-bottom.", "`login.css` のビューポート高とダークモード枠線を確認し、`100dvh` と `safe-area-inset-bottom` に修正します。"),
                    thinkingDuration: "1.9s",
                    toolDetails: [
                        ["name": "read_file", "summary": "web/src/styles/login.css (L1-L92)", "status": "done", "duration": "0.1s", "stepIndex": 1],
                        ["name": "edit_file", "summary": "web/src/styles/login.css (+11/-3)", "status": "done", "duration": "0.2s", "stepIndex": 1]
                    ]
                ),
                Self.agentMsg(
                    id: "c08-a2",
                    step: 2,
                    model: "gemini-3.8-flash-high",
                    modelName: "Gemini 3.8 Flash (High)",
                    text: Self.t(
                        """
                        已修复 `web/src/styles/login.css` 两处样式兼容性问题：

                        - **安全区适配**：底部操作栏改为 `padding-bottom: calc(20px + env(safe-area-inset-bottom))`，并开启 `100dvh` 视口高度避免软键盘顶起错位。
                        - **深色模式对比度**：增加 `@media (prefers-color-scheme: dark)` 输入框边框 `rgba(255, 255, 255, 0.22)`，聚焦态使用品牌蓝高光环。
                        """,
                        """
                        Updated `web/src/styles/login.css` to fix both issues:

                        - **Safe-Area Padding**: Applied `padding-bottom: calc(20px + env(safe-area-inset-bottom))` and `min-height: 100dvh` for keyboard resize stability.
                        - **Dark Mode Contrast**: Bumped `@media (prefers-color-scheme: dark)` input border alpha to `0.22` with a crisp focus ring.
                        """,
                        """
                        `web/src/styles/login.css` の 2 点を修正しました：

                        - **セーフエリア対応**：下部バーに `padding-bottom: calc(20px + env(safe-area-inset-bottom))` と `min-height: 100dvh` を適用し、キーボード表示時のズレを防止。
                        - **ダークモード視認性**：`@media (prefers-color-scheme: dark)` の入力枠線を `rgba(255, 255, 255, 0.22)` に引き上げ、フォーカスリングを追加。
                        """
                    ),
                    timestamp: isoAgo(minutes: 277)
                )
            ],
            pendingInteraction: nil,
            canProceed: false,
            proceedArtifactUri: nil,
            runningTasks: [],
            queuedMessages: [],
            lastModified: ago(minutes: 277),
            unread: false,
            pinned: false,
            modelId: "gemini-3.8-flash-high",
            modelName: "Gemini 3.8 Flash (High)",
            startedAt: isoAgo(minutes: 280),
            contextMeter: Self.meter(used: 42_900, maxTokens: 512_000, tps: 184.0, ttft: 1.4),
            swarm: nil,
            subagents: [],
            parentConversationId: nil,
            subagentRole: nil,
            gitStatus: nil,
            changes: [
                DemoFileDiff(
                    fileUri: "file:///Users/demo/Projects/antigravity-mobile/web/src/styles/login.css",
                    fileName: "web/src/styles/login.css",
                    actionType: "MODIFY",
                    additions: 11,
                    deletions: 3,
                    diffLines: [
                        ("DELETE", "  min-height: 100vh;"),
                        ("DELETE", "  padding-bottom: 20px;"),
                        ("INSERT", "  min-height: 100dvh;"),
                        ("INSERT", "  padding-bottom: calc(20px + env(safe-area-inset-bottom));"),
                        ("INSERT", "@media (prefers-color-scheme: dark) { .auth-input { border-color: rgba(255,255,255,0.22); } }")
                    ]
                )
            ],
            chapters: []
        )

        // 17. Pure Chat: Weekend Travel Itinerary
        let c09Chat = Conversation(
            id: "demo-0009-chat",
            title: Self.t("杭州西湖三日慢游行程", "3-day Lake Tahoe & Napa itinerary", "京都・宇治 2泊3日のんびり家族旅行プラン"),
            workspaceName: "Chat",
            workspaceUri: "",
            status: "CASCADE_RUN_STATUS_IDLE",
            duration: Self.t("4秒", "4s", "4秒"),
            messages: [
                Self.userMsg(
                    id: "c09-u0",
                    step: 0,
                    text: Self.t("下周末带爸妈去杭州玩三天两晚，不想太赶，想吃点地道的杭帮菜、逛逛西湖与茶园，帮我排一份舒适行程。", "Plan a relaxed 3-day weekend trip with parents around Lake Tahoe & Napa: easy scenic walks, local dining, and tea/vineyard stops without rushing.", "来週末に両親と京都へ 2 泊 3 日で行きます。歩き疲れすぎないペースで、京料理や静かなお寺、宇治茶を楽しめるゆったりプランを組んで。"),
                    timestamp: isoAgo(minutes: 420)
                ),
                Self.agentMsg(
                    id: "c09-a1",
                    step: 1,
                    model: "gemini-3.8-flash-high",
                    modelName: "Gemini 3.8 Flash (High)",
                    text: Self.t(
                        """
                        为你规划了一份节奏舒缓、兼顾西湖人文与茶山清净的 **杭州 3 天 2 晚舒适版行程**：

                        ### Day 1 · 西湖北线漫步（住湖滨 / 曲院风荷附近）
                        - **上午**：曲院风荷 → 苏堤北段慢慢走，避开中午晒段（可随时搭景区观光车）。
                        - **午餐**：楼外楼或知味观总店（东坡肉、西湖醋鱼、龙井虾仁，建议 11:15 前拿号）。
                        - **下午**：孤山公园 + 西泠印社小坐喝茶，傍晚在断桥看夕阳。

                        ### Day 2 · 灵隐禅意与龙井茶香
                        - **08:30**：趁人少游览飞来峰与灵隐寺，平缓步行 2 小时即可。
                        - **下午**：打车去**梅家坞 / 龙井村**，挑一家茶农小院泡一壶明前狮峰龙井、吃农家竹林土鸡煲。
                        - **晚上**：城隍阁或河坊街走走消化食，不必硬挤网红点。

                        ### Day 3 · 运河慢生活与返程
                        - **上午**：小河直街 / 桥西历史街区喝咖啡散步，逛刀剪剑博物馆。
                        - **伴手礼**：知味观现烤鲜肉月饼、狮峰龙井散茶（按克买更鲜）。

                        **出行贴士**：带一把晴雨两用伞和防滑平底鞋；苏堤、灵隐景区步行石板路偏多，景区观光车按段招手即停，带长辈非常省力。
                        """,
                        """
                        Here is an unhurried **3-Day Lake Tahoe & Napa Weekend Itinerary** tailored for easy walking:

                        ### Day 1 · Emerald Bay & Lakeside Promenade
                        - **Morning**: Drive the West Shore scenic overlook with short flat boardwalk stops at Vikingsholm overlook.
                        - **Lunch**: Book an early 11:30 AM lakefront table in Tahoe City for seasonal California fare.
                        - **Sunset**: Relax on the deck at Commons Beach before checking in.

                        ### Day 2 · Morning Forest Stroll & Valley Tastings
                        - **08:30 AM**: Beat the crowds with an easy paved walk along the Truckee River path.
                        - **Afternoon**: Cross over to Napa Valley for a seated estate tasting and shaded courtyard lunch.
                        - **Evening**: Unhurried dinner in downtown Yountville.

                        ### Day 3 · Roastery Coffee & Departure
                        - **Morning**: Grab pour-over coffee, browse Oxbow Public Market for artisan olive oil and local pastries, then head home.

                        **Travel Tip**: Pack layered jackets for cool lakeside mornings and cushioned walking shoes for cobblestone market walkways.
                        """,
                        """
                        ご両親と無理なく回れる **京都・宇治 2 泊 3 日ゆったりプラン** を作成しました：

                        ### 1日目 · 南禅寺周辺と鴨川さんぽ
                        - **午前**：南禅寺・哲学の道を平坦ルートでゆっくり散策（疲れたらすぐタクシー移動できる動線）。
                        - **昼食**：11:15 早め予約で湯豆腐と季節の京おばんざいランチ。
                        - **夕方**：鴨川沿いの甘味処でひと休みしてからチェックイン。

                        ### 2日目 · 朝の大原と宇治の茶房めぐり
                        - **08:30**：混雑前に三千院の苔庭を 2 時間ほど静かに散策。
                        - **午後**：宇治へ移動し、老舗茶舗で挽きたて抹茶と茶そばを堪能。
                        - **夜**：先斗町の落ち着いた個室で京懐石。

                        ### 3日目 · 朝カフェとお土産選び
                        - **午前**：堀川周辺のロースタリーでハンドドリップ珈琲、京都駅で宇治新茶や生八ツ橋を購入して帰路へ。

                        **旅のメモ**：石畳が多いためクッション性の高い靴と晴雨兼用傘があると安心です。
                        """
                    ),
                    timestamp: isoAgo(minutes: 416)
                )
            ],
            pendingInteraction: nil,
            canProceed: false,
            proceedArtifactUri: nil,
            runningTasks: [],
            queuedMessages: [],
            lastModified: ago(minutes: 416),
            unread: false,
            pinned: false,
            modelId: "gemini-3.8-flash-high",
            modelName: "Gemini 3.8 Flash (High)",
            startedAt: isoAgo(minutes: 420),
            contextMeter: Self.meter(used: 18_400, maxTokens: 512_000, tps: 192.4, ttft: 1.1),
            swarm: nil,
            subagents: [],
            parentConversationId: nil,
            subagentRole: nil,
            gitStatus: nil,
            changes: [],
            chapters: []
        )

        conversations = [
            c17CmdApproval,
            c01Upload,
            c12Swarm,
            c13WorkflowApproval,
            c05Redesign,
            c11Workflow,
            c10Turns,
            c03Migration,
            c02Login500,
            c04Sales,
            c14Running,
            c06Invoice,
            c15Overflow,
            c07Python,
            c16Notebook,
            c08LoginStyle,
            c09Chat,
            sub1,
            sub2,
            sub3
        ]
    }

    static func seedDrafts() {
        CacheManager.shared.saveDraft(
            key: "demo-0007-python",
            text: Self.t("另外如果把 pandas 预处理也换成 polars，内存峰值还能压低多少？", "How much lower would peak memory go if we also switch pandas preprocessing to Polars?", "ちなみに pandas の前処理も Polars に置き換えたら、ピークメモリはどれくらい下がる？")
        )
        CacheManager.shared.setPinned(cascadeId: "demo-0017-cmd-approval", pinned: true)
        CacheManager.shared.setPinned(cascadeId: "demo-0001-upload", pinned: true)
    }

    // MARK: - Helpers & Generated Sample Assets

    private static func meter(
        used: Int64,
        maxTokens: Int64 = 512_000,
        tps: Double,
        ttft: Double,
        isOverflow: Bool = false
    ) -> ContextMeterItem {
        let pct = min(100.0, (Double(used) / Double(maxTokens) * 1000.0).rounded() / 10.0)
        // Authentic Antigravity prompt-cache split (~88% cache read, ~9% fresh input, ~2% output, ~1% thinking)
        let cacheRead = Int64(Double(used) * 0.88)
        let input = Int64(Double(used) * 0.092)
        let output = Int64(Double(used) * 0.020)
        let thinking = max(240, used - cacheRead - input - output)
        return ContextMeterItem(
            usedTokens: used,
            maxTokens: maxTokens,
            percentage: pct,
            inputTokens: input,
            outputTokens: output,
            cacheReadTokens: cacheRead,
            thinkingTokens: thinking,
            timeToFirstTokenSec: ttft,
            streamingDurationSec: max(2.2, Double(output) / max(1.0, tps)),
            tokensPerSecond: tps,
            isOverflow: isOverflow,
            errorKind: isOverflow ? "context_overflow" : "none"
        )
    }

    private static func demoGitState(
        branch: String,
        ahead: Int,
        modifiedCount: Int,
        prTitle: String,
        prNumber: Int,
        prState: String
    ) -> DemoGitState {
        let paths = [
            "ios/Multigravity/Services/RetryPolicy.swift",
            "internal/gateway/attachments.go",
            "ios/Multigravity/Models/DraftStore.swift"
        ]
        let modifiedFiles: [[String: Any]] = (0..<modifiedCount).map { idx in
            [
                "path": paths[idx % paths.count],
                "index": " ",
                "worktree": "M",
                "status": "MODIFIED",
                "staged": false
            ]
        }
        return DemoGitState(
            repoName: "antigravity-mobile",
            branch: branch,
            upstream: "origin/\(branch)",
            ahead: ahead,
            behind: 0,
            detached: false,
            files: modifiedFiles,
            githubTarget: [
                "host": "github.com",
                "owner": "multigravity-demo",
                "repo": "antigravity-mobile",
                "branch": branch
            ],
            pullRequests: [
                [
                    "host": "github.com",
                    "owner": "multigravity-demo",
                    "repo": "antigravity-mobile",
                    "number": prNumber,
                    "title": prTitle,
                    "state": prState,
                    "url": "https://github.com/multigravity-demo/antigravity-mobile/pull/\(prNumber)",
                    "authorLogin": "alex-dev",
                    "headRefName": branch,
                    "baseRefName": "main"
                ]
            ]
        )
    }

    /// 100% aligned with `SwarmSnapshot` (`SwarmModels.swift` & `internal/proxy/swarm.go`)
    /// Activates `SwarmBannerView`, `SwarmPixelBeeView` (Hive), `SwarmGraphTabView`, and live board.
    private static func initialSwarmSnapshot(minutesAgo m: Double) -> [String: Any] {
        let cid = "demo-0012-swarm"
        let title = Self.t("离线草稿冲突合并小队", "Offline Draft Merge Squad", "オフライン下書き競合マージ小隊")
        let headline = Self.t("Go CAS 乐观锁 + iOS 三路合并并行攻坚", "Go CAS versioning + iOS 3-way draft merge", "Go 楽観ロック＋iOS 3-way マージ並列進行中")
        return [
            "cascadeId": cid,
            "coordinatorId": cid,
            "title": title,
            "etag": "swarm-demo-v6",
            "brief": [
                "active": true,
                "coordinatorId": cid,
                "title": title,
                "phase": Self.t("阶段 2/3 · 并行实现与验证", "Phase 2/3 · Parallel Build & Verify", "フェーズ 2/3 · 並列実装＆検証"),
                "headline": headline,
                "workingCount": 2,
                "totalMembers": 4,
                "latestSeq": 6,
                "attentionCount": 0
            ],
            "members": [
                [
                    "name": "coordinator",
                    "conversationId": cid,
                    "model": "gemini-pro-orchestrator-neon",
                    "role": "coordinator",
                    "wakeMode": "mentions",
                    "lastReadSeq": 6,
                    "lastNudgedSeq": 6,
                    "killed": false,
                    "status": "waiting",
                    "lastActivity": Self.t("等待 @ios-ui 合流后统一验收", "Waiting for @ios-ui patch before final merge", "@ios-ui の完了待ちで最終検証準備中"),
                    "activityKind": "board",
                    "steps": 18,
                    "posts": 3,
                    "lastPostSeq": 5,
                    "workspaceMode": "inherit",
                    "roleWorker": false,
                    "unreadCount": 0,
                    "isCurrent": true
                ],
                [
                    "name": "backend",
                    "conversationId": "demo-swarm-backend",
                    "model": "gemini-4-neon-high",
                    "role": "worker",
                    "wakeMode": "working",
                    "lastReadSeq": 5,
                    "lastNudgedSeq": 5,
                    "killed": false,
                    "status": "working",
                    "lastActivity": Self.t("运行 go test -race ./internal/proxy/...", "Running go test -race ./internal/proxy/...", "go test -race ./internal/proxy/... 実行中"),
                    "activityKind": "tool",
                    "steps": 14,
                    "posts": 2,
                    "lastPostSeq": 4,
                    "workspace": "/Users/demo/.gemini/antigravity/worktrees/swarm-backend",
                    "workspaceMode": "share",
                    "roleWorker": true,
                    "unreadCount": 1,
                    "isCurrent": false
                ],
                [
                    "name": "ios-ui",
                    "conversationId": "demo-swarm-ios-ui",
                    "model": "claude-opus-5-5-high",
                    "role": "worker",
                    "wakeMode": "working",
                    "lastReadSeq": 5,
                    "lastNudgedSeq": 5,
                    "killed": false,
                    "status": "working",
                    "lastActivity": Self.t("完善 DraftStore.mergeRemote 三路合并 UI", "Refining DraftStore.mergeRemote 3-way merge banner", "DraftStore.mergeRemote 3-way マージ UI を実装中"),
                    "activityKind": "edit",
                    "steps": 11,
                    "posts": 1,
                    "lastPostSeq": 3,
                    "workspace": "/Users/demo/.gemini/antigravity/worktrees/swarm-ios-ui",
                    "workspaceMode": "share",
                    "roleWorker": true,
                    "unreadCount": 1,
                    "isCurrent": false
                ],
                [
                    "name": "verifier",
                    "conversationId": "demo-swarm-verifier",
                    "model": "gemini-4-hydrogen-high",
                    "role": "worker",
                    "wakeMode": "mentions",
                    "lastReadSeq": 6,
                    "lastNudgedSeq": 6,
                    "killed": false,
                    "status": "waiting",
                    "lastActivity": Self.t("Go 补丁交叉审校通过，待测 iOS 合并流", "Go CAS diff approved; ready to verify iOS merge", "Go 差分レビュー通過・iOS 側検証待機中"),
                    "activityKind": "review",
                    "steps": 9,
                    "posts": 1,
                    "lastPostSeq": 6,
                    "workspaceMode": "inherit",
                    "roleWorker": true,
                    "unreadCount": 0,
                    "isCurrent": false
                ]
            ],
            "messages": [
                [
                    "seq": 1,
                    "agent": "coordinator",
                    "text": Self.t("@backend 负责在 `/api/v1/drafts` 加入 `version` 乐观锁与 409 快照；@ios-ui 负责 `DraftStore` 三路合并横幅；@verifier 负责双端交叉审计。", "@backend owns `/api/v1/drafts` optimistic `version` CAS + 409 snapshots; @ios-ui owns `DraftStore` 3-way merge banner; @verifier runs independent audit.", "@backend は `/api/v1/drafts` の `version` 楽観ロックと 409 スナップショット、@ios-ui は `DraftStore` 3-way マージ UI、@verifier は相互監査をお願いします。"),
                    "time": iso(Date().addingTimeInterval(-(m + 14) * 60)),
                    "channel": "board",
                    "mentions": ["backend", "ios-ui", "verifier"]
                ],
                [
                    "seq": 2,
                    "agent": "backend",
                    "text": Self.t("已完成 `drafts_cas.go` 版本自增校验与 `409 Conflict` 快照载荷，12 项单测全绿，请 @verifier 先行审校。", "Implemented `drafts_cas.go` CAS check & `409 Conflict` payload (12/12 tests green). Ready for @verifier review.", "`drafts_cas.go` の楽観ロックと `409 Conflict` 応答を実装（12/12 テスト通過）。@verifier 先行レビューお願いします。"),
                    "time": iso(Date().addingTimeInterval(-(m + 10) * 60)),
                    "channel": "board",
                    "mentions": ["coordinator", "verifier"]
                ],
                [
                    "seq": 3,
                    "agent": "ios-ui",
                    "text": Self.t("已接入 `DraftStore.mergeRemote`，断网重连自动保留未发送长文草稿，正在补齐暗黑模式对比度细节。", "Wired `DraftStore.mergeRemote` so unsent long drafts survive disconnects; polishing dark-mode merge banner contrast.", "`DraftStore.mergeRemote` を接続し、切断復帰時も未送信長文を保持できるようにしました。ダークモード視認性を仕上げ中です。"),
                    "time": iso(Date().addingTimeInterval(-(m + 7) * 60)),
                    "channel": "board",
                    "mentions": ["coordinator"]
                ],
                [
                    "seq": 4,
                    "agent": "backend",
                    "text": Self.t("正在补充 32 并发 goroutine 写入的 `-race` 压测，确保零脏读。", "Adding a 32-goroutine concurrent write `-race` stress test to rule out lost updates.", "32 goroutine 同時書き込みの `-race` ストレステストを追加検証中です。"),
                    "time": iso(Date().addingTimeInterval(-(m + 5) * 60)),
                    "channel": "board",
                    "mentions": ["coordinator"]
                ],
                [
                    "seq": 5,
                    "agent": "coordinator",
                    "text": Self.t("@verifier 收到，Go 压测与 @ios-ui 合并横幅就绪后合入 PR #418 跑全量回归。", "@verifier Copy that — once Go stress tests and @ios-ui banner land, we'll run full regression on PR #418.", "@verifier 了解です。Go 負荷テストと @ios-ui の改修が揃い次第 PR #418 で全体回帰を回します。"),
                    "time": iso(Date().addingTimeInterval(-(m + 3) * 60)),
                    "channel": "board",
                    "mentions": ["verifier", "ios-ui"]
                ],
                [
                    "seq": 6,
                    "agent": "verifier",
                    "text": Self.t("Go 乐观锁补丁审校通过 ✅ 无锁泄漏、边界条件完备；正在待命验收 iOS 侧增量 Diff。", "Go CAS patch verified ✅ Zero lock leaks and solid edge-case handling; standing by for iOS diff.", "Go 楽観ロック差分を承認 ✅ デッドロック・境界値とも問題なし。iOS 側の最終差分待ちです。"),
                    "time": iso(Date().addingTimeInterval(-m * 60)),
                    "channel": "board",
                    "mentions": ["coordinator"]
                ]
            ],
            "plan": [
                "goal": [
                    "title": title,
                    "line": headline
                ],
                "tasks": [
                    "backend": Self.t("实现 /api/v1/drafts 乐观锁与 -race 并发验证", "Implement /api/v1/drafts CAS & -race stress tests", "/api/v1/drafts 楽観ロックと -race 負荷検証"),
                    "ios-ui": Self.t("接入 DraftStore 三路合并与断连持久草稿", "Wire DraftStore 3-way merge & offline persistence", "DraftStore 3-way マージと下書き永続化"),
                    "verifier": Self.t("双端交叉代码审计与 PR #418 门禁核验", "Cross-review backend + iOS patches for PR #418", "両端クロスレビューと PR #418 品質確認")
                ],
                "threads": [
                    [
                        "id": "t-cas-protocol",
                        "title": Self.t("协议与服务端 CAS", "Gateway CAS Protocol", "ゲートウェイ楽観ロック"),
                        "goal": Self.t("防止多端同时编辑互相覆盖未发草稿", "Prevent cross-device draft overwrites", "複数端末間の下書き上書きを防止"),
                        "tasks": ["backend", "verifier"]
                    ],
                    [
                        "id": "t-ios-merge",
                        "title": Self.t("iOS 三路合并体验", "iOS 3-Way Merge UX", "iOS 3-way マージ UX"),
                        "goal": Self.t("断网重连零丢字、轻触合并冲突", "Zero lost drafts on reconnect with 1-tap merge", "切断復帰時の消失ゼロとワンタップ統合"),
                        "tasks": ["ios-ui", "verifier"]
                    ]
                ]
            ],
            "view": [
                "version": "1",
                "as_of_seq": 6,
                "updated_at": iso(Date().addingTimeInterval(-m * 60)),
                "headline": headline,
                "goal": [
                    "title": title,
                    "line": headline
                ],
                "running_summary": Self.t("2 名成员并行编码验证中，服务端 CAS 已过审", "2 workers actively building & verifying; Go CAS approved", "2 名が並列実装・検証中、Go CAS はレビュー承認済み"),
                "phase": [
                    "label": Self.t("并行实现与验证", "Parallel Build & Verify", "並列実装＆検証"),
                    "index": 2,
                    "total": 3
                ],
                "attention": [],
                "workstreams": [
                    [
                        "id": "ws-backend",
                        "title": Self.t("Go CAS 乐观锁与并发压测", "Go CAS Locking & Race Tests", "Go 楽観ロック＆レーステスト"),
                        "owners": ["backend"],
                        "state": "active",
                        "note": Self.t("12/12 单元测试通过，正在跑 32 并发压测", "12/12 unit tests passing; running 32-goroutine race test", "12/12 単体テスト通過、32 並行テスト実行中")
                    ],
                    [
                        "id": "ws-ios",
                        "title": Self.t("iOS DraftStore 三路合并", "iOS DraftStore 3-Way Merge", "iOS DraftStore 3-way マージ"),
                        "owners": ["ios-ui"],
                        "state": "active",
                        "note": Self.t("合并逻辑已接入，正在优化暗黑对比度", "Merge wired; polishing dark-mode banner contrast", "マージ実装済み、ダークモード調整中")
                    ],
                    [
                        "id": "ws-verify",
                        "title": Self.t("双端交叉审查与门禁", "Cross-Review & Gate Check", "クロスレビュー＆品質ゲート"),
                        "owners": ["verifier"],
                        "state": "done",
                        "note": Self.t("Go 补丁已审校通过", "Go patch verified clean", "Go パッチ監査完了")
                    ]
                ],
                "threads": [
                    [
                        "id": "t-cas-protocol",
                        "title": Self.t("协议与服务端 CAS", "Gateway CAS Protocol", "ゲートウェイ楽観ロック"),
                        "goal": Self.t("乐观锁版本校验与 409 快照", "Optimistic versioning + 409 snapshots", "バージョン検証と 409 スナップショット"),
                        "now": Self.t("运行 -race 并发测试", "Running -race stress suite", "-race 負荷テスト実行中"),
                        "state": "active",
                        "workstreams": ["ws-backend", "ws-verify"],
                        "attention": []
                    ],
                    [
                        "id": "t-ios-merge",
                        "title": Self.t("iOS 三路合并体验", "iOS 3-Way Merge UX", "iOS 3-way マージ UX"),
                        "goal": Self.t("本地持久化防闪退丢稿", "Crash-safe local draft persistence", "クラッシュ耐性のある下書き永続化"),
                        "now": Self.t("完善暗黑模式冲突横幅", "Polishing dark-mode conflict banner", "ダークモード競合バナー仕上げ中"),
                        "state": "active",
                        "workstreams": ["ws-ios"],
                        "attention": []
                    ]
                ],
                "now": [
                    "backend": Self.t("go test -race ./internal/proxy/...", "go test -race ./internal/proxy/...", "go test -race ./internal/proxy/..."),
                    "ios-ui": Self.t("编辑 DraftStore.swift", "Editing DraftStore.swift", "DraftStore.swift を編集中")
                ],
                "milestones": [
                    ["id": "m1", "text": Self.t("服务端 /api/v1/drafts CAS 协议落地并通过交叉审计", "Server /api/v1/drafts CAS landed & cross-reviewed", "サーバー側 /api/v1/drafts CAS 実装＆監査通過")],
                    ["id": "m2", "text": Self.t("iOS DraftStore 本地持久化与三路合并引擎接入", "iOS DraftStore persistence & 3-way merge wired", "iOS DraftStore 永続化と 3-way マージ接続完了")]
                ],
                "risks": []
            ]
        ]
    }

    private static var salesCSV: String {
        Self.t(
            """
            quarter,region,revenue_wan,orders,avg_ticket_yuan,yoy_growth
            2026-Q1,华东区,240,1820,1318,+14.2%
            2026-Q1,华南区,195,1510,1291,+11.8%
            2026-Q1,华北区,162,1290,1255,+9.4%
            2026-Q2,华东区,288,2140,1345,+20.0%
            2026-Q2,华南区,224,1705,1313,+14.9%
            2026-Q2,华北区,175,1380,1268,+8.0%
            2026-Q3,华东区,356,2590,1374,+23.6%
            2026-Q3,华南区,271,2030,1334,+21.0%
            2026-Q3,华北区,209,1610,1298,+19.4%
            """,
            """
            quarter,region,revenue_k_usd,orders,avg_order_usd,yoy_growth
            2026-Q1,West Coast,240,1820,1318,+14.2%
            2026-Q1,East Coast,195,1510,1291,+11.8%
            2026-Q1,Central,162,1290,1255,+9.4%
            2026-Q2,West Coast,288,2140,1345,+20.0%
            2026-Q2,East Coast,224,1705,1313,+14.9%
            2026-Q2,Central,175,1380,1268,+8.0%
            2026-Q3,West Coast,356,2590,1374,+23.6%
            2026-Q3,East Coast,271,2030,1334,+21.0%
            2026-Q3,Central,209,1610,1298,+19.4%
            """,
            """
            quarter,region,revenue_m_jpy,orders,avg_order_jpy,yoy_growth
            2026-Q1,関東エリア,240,1820,13180,+14.2%
            2026-Q1,関西エリア,195,1510,12910,+11.8%
            2026-Q1,中部エリア,162,1290,12550,+9.4%
            2026-Q2,関東エリア,288,2140,13450,+20.0%
            2026-Q2,関西エリア,224,1705,13130,+14.9%
            2026-Q2,中部エリア,175,1380,12680,+8.0%
            2026-Q3,関東エリア,356,2590,13740,+23.6%
            2026-Q3,関西エリア,271,2030,13340,+21.0%
            2026-Q3,中部エリア,209,1610,12980,+19.4%
            """
        )
    }

    private static var meetingNotes: String {
        Self.t(
            """
            # Q3 经营分析闭门会纪要

            - **日期**：2026-10-12 14:00 - 15:30
            - **参会人**：张总（华东）、李总（华南）、陈敏（华北）、周敏（财务）

            ## 核心结论
            1. **华东大客户续约率突破 94%**，政企定制版套餐拉升客单价 4.2%。
            2. **华南渠道合作伙伴贡献占比提升至 38%**，但交付实施人力出现阶段性排期紧张。
            3. **华北 Q3 末集中落地 2 家能源集团项目**，带动 Q3 同比反超预期。

            ## Q4 重点动作
            - 启动华南区域实施团队扩编与自动化部署模板下沉。
            - 11 月底前输出年度续费分层折扣手册。
            """,
            """
            # Q3 Business Review Notes

            - **Date**: 2026-10-12 14:00 - 15:30
            - **Attendees**: Alex (West), Maya (East), Sam (Central), Jordan (Finance)

            ## Key Takeaways
            1. **West Coast enterprise net renewal reached 94%**, lifting average contract value by +4.2%.
            2. **East Coast partner channel share grew to 38%**, creating near-term onboarding bandwidth pressure.
            3. **Central closed two energy enterprise deals late in Q3**, pushing YoY growth ahead of forecast.

            ## Q4 Action Items
            - Expand East Coast solutions engineering capacity with automated deployment templates.
            - Publish tiered annual renewal guidelines before November 30.
            """,
            """
            # Q3 経営レビュー議事録

            - **日時**：2026-10-12 14:00 - 15:30
            - **参加者**：佐藤（関東）、鈴木（関西）、高橋（中部）、田中（財務）

            ## 主な結論
            1. **関東エンタープライズ継続率が 94% を突破**、平均契約単価を +4.2% 押し上げ。
            2. **関西パートナー経由比率が 38% に拡大**、導入支援エンジニアの稼働が逼迫。
            3. **中部は Q3 末にエネルギー系大型案件 2 件を成約**、前年比成長率が予想を上回る。

            ## Q4 重点アクション
            - 関西導入チームの増員と自動デプロイテンプレートの横展開を開始。
            - 11 月末までに年間更新ティア別割引ガイドラインを公開。
            """
        )
    }

    private static var planMarkdown: String {
        Self.t(
            """
            # 官网边缘缓存与首页 SSR 重构实施方案

            > [!IMPORTANT]
            > 本次改造保持旧版静态包 100% 在线兜底，所有灰度流量均通过 Edge Worker 按 Cookie 哈希切流，发现 P95 LCP 超标或 5xx 升高可在 5 秒内自动熔断回滚。

            ## 一、线上实测基线与目标指标

            | 观测指标 | 当前基线（近 7 天 P75） | 目标验收值 |
            | --- | ---: | ---: |
            | 首页 LCP | `2.64 s` | **`< 1.20 s`** |
            | 首屏关键 JS | `184 KB gzip` | **`< 95 KB gzip`** |
            | 边缘缓存命中率 | `61.4%` | **`>= 92.0%`** |
            | 异常回滚耗时 | 手动切流 ~8 min | **`< 5 s` 自动回退** |

            ## 二、分模块改动清单

            ### 1. Edge Worker 灰度路由层
            - **[MODIFY] `worker/edge_router.ts`**：实现 `10% → 50% → 100%` 确定性灰度桶分配，注入 `X-Mgy-Variant` 响应头便于观测。
            - **[NEW] `worker/circuit_breaker.ts`**：采样边缘 5xx 比率与 TTFB，超阈值自动将流量切回静态源站。

            ### 2. Next.js 16 App Router 流式渲染
            - **[MODIFY] `web/app/page.tsx`**：将 Hero 与实时特性对比拆分为独立 Server Component，开启 `revalidate = 60` 边缘 ISR。
            - **[MODIFY] `web/next.config.ts`**：剔除非首屏多语言包与冗余字体预载，压缩核心包至 `< 95 KB gzip`。

            > [!CAUTION]
            > 切流前务必核对 CDN 缓存键包含 `Accept-Language`（`zh` / `en` / `ja`），避免跨语种 HTML 互相污染缓存。

            ## 三、Verification Plan（验证与验收）
            1. **自动化回归**：执行 `pnpm build && pnpm test:e2e`，验证三语首屏零 Hydration Warning。
            2. **灰度实测**：放量 10% 观察 30 分钟 Prometheus Web Vitals（`LCP < 1.2s`、`5xx < 0.1%`）后再推全量。
            """,
            """
            # Edge SSR & Canary Caching Implementation Plan

            > [!IMPORTANT]
            > The legacy static bundle stays warm as a 100% fallback. Canary buckets route deterministically via Edge Worker cookie hashing with automated 5-second circuit-breaker rollback.

            ## 1. Baseline Metrics & Acceptance Gates

            | Metric | Current 7d P75 | Target Gate |
            | --- | ---: | ---: |
            | Home LCP | `2.64 s` | **`< 1.20 s`** |
            | Critical JS | `184 KB gzip` | **`< 95 KB gzip`** |
            | Edge Cache Hit Ratio | `61.4%` | **`>= 92.0%`** |
            | Rollback Time | Manual ~8 min | **`< 5 s` auto-revert** |

            ## 2. Proposed Changes

            ### Edge Worker Routing Layer
            - **[MODIFY] `worker/edge_router.ts`**: Deterministic `10% → 50% → 100%` canary routing with `X-Mgy-Variant` telemetry headers.
            - **[NEW] `worker/circuit_breaker.ts`**: Auto-reverts traffic to static origin if 5xx > 0.2% or P95 LCP > 1.8s.

            ### Next.js 16 Streaming SSR
            - **[MODIFY] `web/app/page.tsx`**: Split Hero and live feature matrix into streaming Server Components with `revalidate = 60` ISR.
            - **[MODIFY] `web/next.config.ts`**: Trim off-fold locale bundles to keep critical JS under `95 KB gzip`.

            > [!CAUTION]
            > Ensure the CDN cache key varies by `Accept-Language` (`en` / `ja` / `zh`) before shifting traffic so localized HTML never collides in edge cache.

            ## 3. Verification Plan
            1. **Automated Suite**: Run `pnpm build && pnpm test:e2e` across all 3 locales with zero hydration warnings.
            2. **Canary Soak**: Hold 10% traffic for 30 minutes while verifying Prometheus Web Vitals (`LCP < 1.2s`, `5xx < 0.1%`) before full promotion.
            """,
            """
            # Edge SSR・カナリアキャッシュ刷新 実装計画書

            > [!IMPORTANT]
            > 現行の静的バンドルは 100% フォールバック待機させたまま、Edge Worker の Cookie ハッシュで段階切替します。P95 LCP 悪化や 5xx 増加時は 5 秒以内に自動ロールバックします。

            ## 1. 現行実測ベースラインと目標ゲート

            | 観測指標 | 現行 7 日間 P75 | 目標受け入れ基準 |
            | --- | ---: | ---: |
            | トップ LCP | `2.64 s` | **`< 1.20 s`** |
            | 初期 JS 容量 | `184 KB gzip` | **`< 95 KB gzip`** |
            | エッジキャッシュヒット率 | `61.4%` | **`>= 92.0%`** |
            | 異常時ロールバック | 手動 約8分 | **`< 5 s` 自動復帰** |

            ## 2. モジュール別変更計画

            ### Edge Worker ルーティング層
            - **[MODIFY] `worker/edge_router.ts`**：`10% → 50% → 100%` の決定的カナリア振り分けと `X-Mgy-Variant` 計測ヘッダを追加。
            - **[NEW] `worker/circuit_breaker.ts`**：5xx > 0.2% または P95 LCP > 1.8s を検知した際、5 秒以内に静的オリジンへ自動切替。

            ### Next.js 16 ストリーミング SSR
            - **[MODIFY] `web/app/page.tsx`**：Hero と機能比較を Server Component 分離し、`revalidate = 60` のエッジ ISR を適用。
            - **[MODIFY] `web/next.config.ts`**：初期バンドルから非表示ロケールを除外し `95 KB gzip` 未満を維持。

            > [!CAUTION]
            > 配信開始前に CDN キャッシュキーへ `Accept-Language`（`ja` / `en` / `zh`）を含め、多言語 HTML のキャッシュ混在を防止してください。

            ## 3. 検証計画（Verification Plan）
            1. **自動テスト**：`pnpm build && pnpm test:e2e` を 3 言語で実行し Hydration 警告ゼロを確認。
            2. **カナリア監視**：10% 配信を 30 分継続し Prometheus で `LCP < 1.2s`・`5xx < 0.1%` を確認後に全量切替。
            """
        )
    }

    private static var notebookJSON: String {
        let heading = Self.t("# 带交易成本的多资产 Kelly 仓位模拟", "# Multi-Asset Kelly Allocation with Transaction Costs", "# 取引コスト込みマルチアセット・ケリー配分シミュレーション")
        let desc = Self.t(
            "测试不同缩放因子 $\\\\gamma \\\\in \\\\{0.25, 0.5, 1.0\\\\}$ 下的净值曲线与最大回撤。",
            "Simulate equity curves and max drawdown across fractional Kelly scalars $\\\\gamma \\\\in \\\\{0.25, 0.5, 1.0\\\\}$.",
            "スケーリング係数 $\\\\gamma \\\\in \\\\{0.25, 0.5, 1.0\\\\}$ ごとの資産推移と最大ドローダウンを比較。"
        )
        return """
        {
          "nbformat": 4,
          "nbformat_minor": 5,
          "metadata": {
            "kernelspec": { "display_name": "Python 3.12 (quant)", "language": "python", "name": "python3" },
            "language_info": { "name": "python", "version": "3.12.4" }
          },
          "cells": [
            {
              "cell_type": "markdown",
              "metadata": {},
              "source": [
                "\(heading)\\n",
                "\(desc)"
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
                    "gamma=0.25 -> CAGR: 11.2% | MaxDD: -7.4% | Sharpe: 1.62\\n",
                    "gamma=0.50 -> CAGR: 18.6% | MaxDD: -15.9% | Sharpe: 1.61\\n",
                    "gamma=1.00 -> CAGR: 24.8% | MaxDD: -34.2% | Sharpe: 1.48\\n"
                  ]
                }
              ],
              "source": [
                "import numpy as np\\n",
                "for gamma in [0.25, 0.5, 1.0]:\\n",
                "    print(f'gamma={gamma:.2f} evaluated')"
              ]
            }
          ]
        }
        """
    }

    private static func salesChartPNG() -> Data {
        let size = CGSize(width: 960, height: 540)
        let format = UIGraphicsImageRendererFormat()
        format.scale = 1
        let renderer = UIGraphicsImageRenderer(size: size, format: format)
        let img = renderer.image { ctx in
            let cg = ctx.cgContext
            UIColor.white.setFill()
            cg.fill(CGRect(origin: .zero, size: size))

            let titleAttrs: [NSAttributedString.Key: Any] = [
                .font: UIFont.boldSystemFont(ofSize: 28),
                .foregroundColor: UIColor(white: 0.12, alpha: 1)
            ]
            Self.t("2026 Q1 - Q3 各区域销售额趋势（万元）", "2026 Q1 - Q3 Regional Revenue ($K USD)", "2026 Q1 - Q3 エリア別売上推移（百万円）").draw(at: CGPoint(x: 48, y: 34), withAttributes: titleAttrs)

            let quarters = ["Q1", "Q2", "Q3"]
            let east: [CGFloat] = [240, 288, 356]
            let south: [CGFloat] = [195, 224, 271]
            let north: [CGFloat] = [162, 175, 209]
            let colors = [
                UIColor(red: 0.18, green: 0.45, blue: 0.92, alpha: 1),
                UIColor(red: 0.16, green: 0.69, blue: 0.56, alpha: 1),
                UIColor(red: 0.96, green: 0.60, blue: 0.18, alpha: 1)
            ]

            let chartRect = CGRect(x: 72, y: 112, width: 820, height: 340)
            cg.setStrokeColor(UIColor(white: 0.9, alpha: 1).cgColor)
            cg.setLineWidth(1)
            for step in 0...4 {
                let y = chartRect.maxY - chartRect.height * CGFloat(step) / 4.0
                cg.move(to: CGPoint(x: chartRect.minX, y: y))
                cg.addLine(to: CGPoint(x: chartRect.maxX, y: y))
                cg.strokePath()
                let label = "\(step * 100)"
                label.draw(
                    at: CGPoint(x: 24, y: y - 10),
                    withAttributes: [.font: UIFont.systemFont(ofSize: 15), .foregroundColor: UIColor.gray]
                )
            }

            let groupW = chartRect.width / CGFloat(quarters.count)
            for (idx, q) in quarters.enumerated() {
                let vals = [east[idx], south[idx], north[idx]]
                let barW: CGFloat = 46
                let startX = chartRect.minX + groupW * CGFloat(idx) + (groupW - barW * 3 - 20) / 2
                for (b, val) in vals.enumerated() {
                    let h = chartRect.height * (val / 400.0)
                    let r = CGRect(x: startX + CGFloat(b) * (barW + 10), y: chartRect.maxY - h, width: barW, height: h)
                    colors[b].setFill()
                    UIBezierPath(roundedRect: r, cornerRadius: 6).fill()
                    "\(Int(val))".draw(
                        at: CGPoint(x: r.minX + 8, y: r.minY - 22),
                        withAttributes: [.font: UIFont.boldSystemFont(ofSize: 14), .foregroundColor: UIColor.darkGray]
                    )
                }
                q.draw(
                    at: CGPoint(x: chartRect.minX + groupW * CGFloat(idx) + groupW / 2 - 14, y: chartRect.maxY + 14),
                    withAttributes: [.font: UIFont.boldSystemFont(ofSize: 18), .foregroundColor: UIColor.darkGray]
                )
            }

            let legends = [Self.t("华东区", "West", "関東"), Self.t("华南区", "East", "関西"), Self.t("华北区", "Central", "中部")]
            for (idx, title) in legends.enumerated() {
                let lx = 600 + CGFloat(idx) * 105
                colors[idx].setFill()
                UIBezierPath(roundedRect: CGRect(x: lx, y: 40, width: 16, height: 16), cornerRadius: 4).fill()
                title.draw(at: CGPoint(x: lx + 22, y: 38), withAttributes: [.font: UIFont.systemFont(ofSize: 16), .foregroundColor: UIColor.darkGray])
            }
        }
        return img.pngData() ?? Data()
    }

    private static func invoiceJPEG(
        title: String,
        shop: String,
        items: [(String, String)],
        total: String,
        no: String
    ) -> Data {
        let size = CGSize(width: 720, height: 480)
        let format = UIGraphicsImageRendererFormat()
        format.scale = 1
        let renderer = UIGraphicsImageRenderer(size: size, format: format)
        let img = renderer.image { ctx in
            let cg = ctx.cgContext
            UIColor(red: 0.99, green: 0.98, blue: 0.95, alpha: 1).setFill()
            cg.fill(CGRect(origin: .zero, size: size))

            let border = CGRect(x: 24, y: 24, width: size.width - 48, height: size.height - 48)
            UIColor(red: 0.72, green: 0.24, blue: 0.22, alpha: 0.85).setStroke()
            let path = UIBezierPath(roundedRect: border, cornerRadius: 10)
            path.lineWidth = 3
            path.stroke()

            title.draw(
                at: CGPoint(x: 48, y: 48),
                withAttributes: [.font: UIFont.boldSystemFont(ofSize: 26), .foregroundColor: UIColor(red: 0.72, green: 0.2, blue: 0.18, alpha: 1)]
            )
            no.draw(
                at: CGPoint(x: 470, y: 54),
                withAttributes: [.font: UIFont.monospacedSystemFont(ofSize: 16, weight: .semibold), .foregroundColor: UIColor.darkGray]
            )
            Self.t("销方：\(shop)", "Merchant: \(shop)", "発行元：\(shop)").draw(
                at: CGPoint(x: 48, y: 102),
                withAttributes: [.font: UIFont.systemFont(ofSize: 20), .foregroundColor: UIColor.black]
            )

            cg.setStrokeColor(UIColor(white: 0.8, alpha: 1).cgColor)
            cg.move(to: CGPoint(x: 48, y: 144))
            cg.addLine(to: CGPoint(x: size.width - 48, y: 144))
            cg.strokePath()

            for (i, (itemName, price)) in items.enumerated() {
                let y = CGFloat(168 + i * 44)
                itemName.draw(at: CGPoint(x: 52, y: y), withAttributes: [.font: UIFont.systemFont(ofSize: 19), .foregroundColor: UIColor.darkGray])
                price.draw(at: CGPoint(x: 560, y: y), withAttributes: [.font: UIFont.monospacedSystemFont(ofSize: 19, weight: .medium), .foregroundColor: UIColor.black])
            }

            cg.move(to: CGPoint(x: 48, y: 344))
            cg.addLine(to: CGPoint(x: size.width - 48, y: 344))
            cg.strokePath()

            Self.t("价税合计：\(total)", "Total (incl. tax): \(total)", "税込合計：\(total)").draw(
                at: CGPoint(x: 48, y: 372),
                withAttributes: [.font: UIFont.boldSystemFont(ofSize: 26), .foregroundColor: UIColor(red: 0.72, green: 0.2, blue: 0.18, alpha: 1)]
            )

            // Receipt verification stamp
            let stampRect = CGRect(x: 520, y: 332, width: 120, height: 84)
            UIColor(red: 0.82, green: 0.18, blue: 0.18, alpha: 0.7).setStroke()
            let oval = UIBezierPath(ovalIn: stampRect)
            oval.lineWidth = 3
            oval.stroke()
            Self.t("发票专用章", "VERIFIED", "領収済").draw(
                at: CGPoint(x: 536, y: 363),
                withAttributes: [.font: UIFont.boldSystemFont(ofSize: 16), .foregroundColor: UIColor(red: 0.82, green: 0.18, blue: 0.18, alpha: 0.8)]
            )
        }
        return img.jpegData(compressionQuality: 0.88) ?? Data()
    }

    private static func reportPDF(chartPNG: Data) -> Data {
        let bounds = CGRect(x: 0, y: 0, width: 612, height: 792)
        let renderer = UIGraphicsPDFRenderer(bounds: bounds)
        let chartImg = UIImage(data: chartPNG)

        return renderer.pdfData { ctx in
            // Page 1: Cover
            ctx.beginPage()
            UIColor(red: 0.10, green: 0.16, blue: 0.30, alpha: 1).setFill()
            ctx.cgContext.fill(CGRect(x: 0, y: 0, width: 612, height: 220))
            Self.t("2026 Q3 季度销售经营分析报告", "2026 Q3 Sales Performance Report", "2026 Q3 四半期売上パフォーマンス報告書").draw(
                at: CGPoint(x: 48, y: 78),
                withAttributes: [.font: UIFont.boldSystemFont(ofSize: 28), .foregroundColor: UIColor.white]
            )
            Self.t("编制部门：数据分析台  |  报告周期：2026.07 - 2026.09", "Prepared by Analytics Hub  |  Period: Jul – Sep 2026", "作成：データ分析ハブ  |  対象期間：2026.07 - 2026.09").draw(
                at: CGPoint(x: 48, y: 126),
                withAttributes: [.font: UIFont.systemFont(ofSize: 15), .foregroundColor: UIColor(white: 0.86, alpha: 1)]
            )
            Self.t(
                """
                执行摘要：

                • 全公司 Q3 总体营收 836 万元，环比 Q2（687 万）增长 +21.7%。
                • 华东大区大客户续约率达到 94%，拉动综合客单价提升 4.2%。
                • 华南渠道合作伙伴贡献占比达到 38%，需于 Q4 扩充交付实施力量。
                • 华北受能源集团项目集中验收带动，Q3 同比增长实现反超。
                """,
                """
                Executive Summary:

                • Total Q3 revenue reached $836K, up +21.7% QoQ vs. Q2 ($687K).
                • West Coast enterprise renewal hit 94%, lifting ACV by +4.2%.
                • East Coast partner channel share reached 38%, motivating Q4 onboarding hires.
                • Central exceeded forecast following two late-quarter energy rollouts.
                """,
                """
                エグゼクティブサマリー：

                • 全社 Q3 売上は 836 百万円、Q2（687 百万円）比で +21.7% 成長。
                • 関東エリアの大口継続率が 94% に達し、平均単価 +4.2% に寄与。
                • 関西パートナー経由比率が 38% へ伸長、Q4 の導入体制拡充が必要。
                • 中部は四半期末の大型案件 2 件により計画を超過達成。
                """
            ).draw(
                in: CGRect(x: 48, y: 268, width: 516, height: 420),
                withAttributes: [.font: UIFont.systemFont(ofSize: 16), .foregroundColor: UIColor.darkGray]
            )

            // Page 2: Visual Chart
            ctx.beginPage()
            Self.t("第二部分 · 区域业绩增长趋势图", "Section 2 · Regional Revenue Trend", "第 2 部 · エリア別売上成長トレンド").draw(
                at: CGPoint(x: 48, y: 54),
                withAttributes: [.font: UIFont.boldSystemFont(ofSize: 22), .foregroundColor: UIColor.black]
            )
            if let chartImg {
                chartImg.draw(in: CGRect(x: 48, y: 112, width: 516, height: 290))
            }
            Self.t(
                "图表解读：三个大区均保持连续两个季度环比正增长，其中华东 Q3 单季营收达 356 万元，创历史新高；华南渠道放量显著。",
                "Takeaway: All three regions grew sequentially for two straight quarters, led by West Coast ($356K) at a new quarterly record.",
                "ポイント：全 3 エリアが 2 四半期連続で増収を達成。特に関東エリアは Q3 単体で 356 百万円と過去最高を更新しました。"
            ).draw(
                in: CGRect(x: 48, y: 428, width: 516, height: 180),
                withAttributes: [.font: UIFont.systemFont(ofSize: 15), .foregroundColor: UIColor.darkGray]
            )

            // Page 3: Regional Breakdown
            ctx.beginPage()
            Self.t("第三部分 · 各区域关键经营指标明细", "Section 3 · Regional KPI Breakdown", "第 3 部 · エリア別主要 KPI 詳細").draw(
                at: CGPoint(x: 48, y: 54),
                withAttributes: [.font: UIFont.boldSystemFont(ofSize: 22), .foregroundColor: UIColor.black]
            )
            Self.t(
                """
                1. 华东区：Q3 营收 356 万，订单数 2,590 单，客单价 ¥1,374（最高）。
                2. 华南区：Q3 营收 271 万，订单数 2,030 单，渠道贡献占比 38%。
                3. 华北区：Q3 营收 209 万，订单数 1,610 单，同比增速回升至 +19.4%。
                """,
                """
                1. West Coast: $356K Q3 revenue, 2,590 orders, $1,374 ACV (highest).
                2. East Coast: $271K Q3 revenue, 2,030 orders, 38% partner contribution.
                3. Central: $209K Q3 revenue, 1,610 orders, YoY growth accelerating to +19.4%.
                """,
                """
                1. 関東エリア：Q3 売上 356 百万円、注文 2,590 件、平均単価 ¥13,740（最高）。
                2. 関西エリア：Q3 売上 271 百万円、注文 2,030 件、パートナー比率 38%。
                3. 中部エリア：Q3 売上 209 百万円、注文 1,610 件、前年比成長率 +19.4% へ加速。
                """
            ).draw(
                in: CGRect(x: 48, y: 112, width: 516, height: 320),
                withAttributes: [.font: UIFont.systemFont(ofSize: 16), .foregroundColor: UIColor.darkGray]
            )

            // Page 4: Q4 Action Plan
            ctx.beginPage()
            Self.t("第四部分 · Q4 行动建议与资源排期", "Section 4 · Q4 Action Plan", "第 4 部 · Q4 アクションプラン").draw(
                at: CGPoint(x: 48, y: 54),
                withAttributes: [.font: UIFont.boldSystemFont(ofSize: 22), .foregroundColor: UIColor.black]
            )
            Self.t(
                """
                • 11 月 15 日前完成华南 4 名专职交付工程师补充，缓解伙伴排期压力。
                • 11 月 30 日前发布《2027 年度大客户分层续约政策手册》。
                • 引入自动化对账仪表盘，压缩月结数据分析耗时 50%。
                """,
                """
                • Hire 4 dedicated solutions engineers in the East region by November 15.
                • Finalize the 2027 tiered enterprise renewal playbook by November 30.
                • Roll out automated reconciliation dashboards to cut month-end reporting time by 50%.
                """,
                """
                • 11 月 15 日までに関西専任ソリューションエンジニア 4 名を増員し導入待ちを解消。
                • 11 月 30 日までに 2027 年度エンタープライズ継続ガイドラインを策定。
                • 自動照合ダッシュボードを導入し、月次レポート集計工数を 50% 削減。
                """
            ).draw(
                in: CGRect(x: 48, y: 112, width: 516, height: 320),
                withAttributes: [.font: UIFont.systemFont(ofSize: 16), .foregroundColor: UIColor.darkGray]
            )
        }
    }
}
