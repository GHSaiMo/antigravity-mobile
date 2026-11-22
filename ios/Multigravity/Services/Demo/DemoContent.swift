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

        // MARK: Accounts & Quotas
        accounts = [
            QuotaAccount(
                id: "acc-ultra",
                email: "demo-ultra@antigravity.dev",
                name: "Demo Ultra",
                tier: "ULTRA",
                claude5h: 86,
                claudeWeekly: 92,
                gemini5h: 74,
                geminiWeekly: 88,
                argon: 100,
                claude5hOffsetHours: 3.6,
                claudeWeeklyOffsetDays: 4.8,
                gemini5hOffsetHours: 4.2,
                geminiWeeklyOffsetDays: 5.5
            ),
            QuotaAccount(
                id: "acc-pro",
                email: "demo-pro@antigravity.dev",
                name: "Demo Pro",
                tier: "PRO",
                claude5h: 42,
                claudeWeekly: 68,
                gemini5h: 55,
                geminiWeekly: 79,
                argon: 62,
                claude5hOffsetHours: 1.8,
                claudeWeeklyOffsetDays: 2.3,
                gemini5hOffsetHours: 2.4,
                geminiWeeklyOffsetDays: 3.1
            ),
            QuotaAccount(
                id: "acc-free",
                email: "demo-free@antigravity.dev",
                name: "Demo Free",
                tier: "FREE",
                claude5h: 18,
                claudeWeekly: 25,
                gemini5h: 30,
                geminiWeekly: 40,
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

        // MARK: Plugins & Providers
        plugins = [
            [
                "id": "plugin-git-inspector",
                "name": "Git & PR Inspector",
                "description": Self.t("实时读取分支状态、Diff 与 GitHub Pull Request", "Inspect branch status, diffs, and GitHub Pull Requests in real time", "ブランチ状態・コード差分・GitHub Pull Request をリアルタイム確認"),
                "version": "1.8.2",
                "enabled": true,
                "canToggle": true,
                "scope": "global",
                "providerName": "GitHub",
                "signedIn": true,
                "accountIdentity": "demo-dev@github.com",
                "tools": ["git_status", "git_diff", "pr_checks"]
            ],
            [
                "id": "plugin-xcode-bridge",
                "name": "Xcode Wireless Bridge",
                "description": Self.t("真机无线调试状态监测与构建日志流推送", "Monitor wireless device pairing state and stream build logs", "ワイヤレスデバッグ接続状態の監視とビルドログのリアルタイム配信"),
                "version": "2.1.0",
                "enabled": true,
                "canToggle": true,
                "scope": "global",
                "providerName": "Apple Developer",
                "signedIn": true,
                "accountIdentity": "Demo Team (8A92KQ)",
                "tools": ["xcode_build", "device_pair"]
            ],
            [
                "id": "plugin-notion-sync",
                "name": "Notion Knowledge Sync",
                "description": Self.t("将技术评审与会议纪要一键归档至团队知识库", "Archive architecture reviews and meeting notes to Notion", "設計レビューや議事録をワンタップでチーム Notion にアーカイブ"),
                "version": "0.9.4",
                "enabled": false,
                "canToggle": true,
                "scope": "workspace",
                "providerName": "Notion",
                "signedIn": false,
                "accountIdentity": "",
                "tools": ["notion_export"]
            ]
        ]

        pluginProviders = [
            [
                "provider": "github",
                "pluginId": "plugin-git-inspector",
                "pluginName": "Git & PR Inspector",
                "displayName": "GitHub OAuth",
                "signedIn": true,
                "accounts": [
                    ["name": "demo-main", "label": Self.t("工作账号", "Work", "仕事用"), "email": "demo-dev@github.com", "isDefault": true]
                ]
            ],
            [
                "provider": "apple",
                "pluginId": "plugin-xcode-bridge",
                "pluginName": "Xcode Wireless Bridge",
                "displayName": "Apple Developer",
                "signedIn": true,
                "accounts": [
                    ["name": "team-demo", "label": "Demo Team", "email": "dev@antigravity.dev", "isDefault": true]
                ]
            ],
            [
                "provider": "notion",
                "pluginId": "plugin-notion-sync",
                "pluginName": "Notion Knowledge Sync",
                "displayName": "Notion Workspace",
                "signedIn": false,
                "accounts": []
            ]
        ]

        // MARK: Automations
        automations = [
            [
                "id": "auto-nightly-ci",
                "name": Self.t("夜间回归测试巡检", "Nightly Regression Suite", "夜間リグレッション自動巡回"),
                "prompt": Self.t("运行 go test -race ./... 与 iOS 编译检查，若有失败则推送错误摘要。", "Run go test -race ./... and iOS build checks; notify on failure.", "go test -race ./... と iOS ビルド検証を実行し、失敗時に要約を通知。"),
                "scheduleType": "cron",
                "cron": "0 2 * * *",
                "scheduleSummary": Self.t("每天 02:00", "Daily at 02:00", "毎日 02:00"),
                "model": "gemini-4-carbon-high",
                "workspace": "/Users/demo/Projects/antigravity-mobile",
                "enabled": true,
                "runCount": 42,
                "lastRunAt": isoAgo(minutes: 420),
                "nextRunAt": Self.iso(now.addingTimeInterval(6 * 3600)),
                "lastResult": "success"
            ],
            [
                "id": "auto-quota-alert",
                "name": Self.t("配额水位预警", "Quota Threshold Watch", "クォータ残量アラート"),
                "prompt": Self.t("每小时检查各账号 5h 额度池余量，低于 20% 时提醒切换备用账号。", "Check 5h model quota headroom hourly and alert below 20%.", "1時間ごとに各アカウントの 5h 枠残量を監視し、20% 未満で切替を通知。"),
                "scheduleType": "interval",
                "intervalSeconds": 3600,
                "scheduleSummary": Self.t("每 1 小时", "Every 1 hour", "1時間ごと"),
                "model": "gemini-3.8-flash",
                "workspace": "",
                "enabled": true,
                "runCount": 118,
                "lastRunAt": isoAgo(minutes: 28),
                "nextRunAt": Self.iso(now.addingTimeInterval(32 * 60)),
                "lastResult": "success"
            ],
            [
                "id": "auto-friday-digest",
                "name": Self.t("周五版本发布周报", "Friday Release Digest", "金曜リリース週次ダイジェスト"),
                "prompt": Self.t("汇总本周 PR 合并清单、线上崩溃率与未闭环 Issue，生成 Markdown 周报。", "Summarize merged PRs, crash-free rate, and open issues in Markdown.", "今週マージされた PR・クラッシュフリー率・未解決 Issue を Markdown 週報に集計。"),
                "scheduleType": "cron",
                "cron": "0 17 * * 5",
                "scheduleSummary": Self.t("每周五 17:00", "Fridays at 17:00", "毎週金曜 17:00"),
                "model": "claude-opus-5-5-thinking",
                "workspace": "/Users/demo/Projects/antigravity-mobile",
                "enabled": false,
                "runCount": 9,
                "lastRunAt": isoAgo(minutes: 4320),
                "lastResult": "paused"
            ]
        ]

        automationEvents = [
            ["automationId": "auto-nightly-ci", "automationName": Self.t("夜间回归测试巡检", "Nightly Regression Suite", "夜間リグレッション自動巡回"), "status": "completed", "message": "41/41 tests passed", "timestamp": isoAgo(minutes: 420)],
            ["automationId": "auto-quota-alert", "automationName": Self.t("配额水位预警", "Quota Threshold Watch", "クォータ残量アラート"), "status": "completed", "message": Self.t("Demo Ultra 86% · Pro 42% · Free 18% (已提示)", "Demo Ultra 86% · Pro 42% · Free 18% (Alert sent)", "Demo Ultra 86% · Pro 42% · Free 18%（通知済み）"), "timestamp": isoAgo(minutes: 28)],
            ["automationId": "auto-nightly-ci", "automationName": Self.t("夜间回归测试巡检", "Nightly Regression Suite", "夜間リグレッション自動巡回"), "status": "running", "message": Self.t("正在执行 iOS xcodebuild 增量检查...", "Running incremental iOS xcodebuild check...", "iOS xcodebuild 差分ビルド検証を実行中..."), "timestamp": isoAgo(minutes: 425)],
            ["automationId": "auto-friday-digest", "automationName": Self.t("周五版本发布周报", "Friday Release Digest", "金曜リリース週次ダイジェスト"), "status": "skipped", "message": Self.t("自动化已暂停", "Automation paused", "自動化を一時停止中"), "timestamp": isoAgo(minutes: 1440)]
        ]

        // MARK: Workflow Templates
        workflowTemplates = Self.defaultDemoWorkflowTemplates()

        // MARK: Global Daemon & Running Tasks + Logs
        globalDaemonTasks = [
            [
                "taskId": "task-demo-upload-build",
                "conversationId": "demo-0001-upload",
                "conversationTitle": Self.t("实现文件附件上传", "File Attachment Upload Flow", "ファイル添付アップロード実装"),
                "commandLine": "go test -v ./internal/gateway -run TestAttachmentUpload",
                "toolSummary": Self.t("验证多端附件上传测试", "Verify attachment upload tests", "マルチ端末の添付アップロードテストを検証"),
                "startedAt": isoAgo(minutes: 2),
                "isDaemon": false,
                "elapsedSeconds": 24,
                "running": true
            ],
            [
                "taskId": "task-vite-daemon",
                "conversationId": "demo-0014-running",
                "conversationTitle": Self.t("实时联调与守护进程监控", "Live Dev Server & Daemon Logs", "開発サーバー＆デーモンログ監視"),
                "commandLine": "npm run dev -- --host 0.0.0.0 --port 5173",
                "toolSummary": Self.t("启动 Web 控制台热更新守护进程", "Web Console HMR Dev Server", "Web コンソール HMR 開発サーバーを起動"),
                "startedAt": isoAgo(minutes: 6),
                "isDaemon": true,
                "elapsedSeconds": 360,
                "running": true
            ],
            [
                "taskId": "task-go-race",
                "conversationId": "demo-0014-running",
                "conversationTitle": Self.t("实时联调与守护进程监控", "Live Dev Server & Daemon Logs", "開発サーバー＆デーモンログ監視"),
                "commandLine": "go test -race -count=1 ./internal/cockpit/...",
                "toolSummary": Self.t("运行 Go 并发竞态回归测试", "Run Go race-detector suite", "Go 並行レース検出テストを実行"),
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
                    text: Self.t("全面扫描 `handlers/rule_eval.go` 与 `repo/score_store.go` 的参数绑定、租户隔离和 Token 校验逻辑。", "Scan `handlers/rule_eval.go` and `repo/score_store.go` for parameter binding, tenant isolation, and auth token validation.", "handlers/rule_eval.go と repo/score_store.go のパラメータバインド・テナント分離・トークン検証を全面スキャンして。"),
                    timestamp: isoAgo(minutes: 25)
                ),
                Self.toolMsg(id: "sub1-t1", step: 1, tools: ["GrepSearch", "ReadFile", "RunStaticAudit"]),
                Self.agentMsg(
                    id: "sub1-a2",
                    step: 2,
                    model: "claude-opus-5-5-thinking",
                    modelName: "Claude Opus 5.5 (Thinking)",
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
            modelId: "claude-opus-5-5-thinking",
            modelName: "Claude Opus 5.5 (Thinking)",
            startedAt: isoAgo(minutes: 25),
            contextMeter: Self.meter(input: 18_400, output: 1_250, tps: 49.4, ttft: 0.72),
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
                    text: Self.t("分析 `EvaluateRiskBatch` 在 500 QPS 下的 CPU pprof 火焰图，定位 P99 抖动来源。", "Analyze the CPU pprof flamegraph for `EvaluateRiskBatch` at 500 QPS and locate P99 latency spikes.", "500 QPS 時の EvaluateRiskBatch の CPU pprof フレームグラフを解析し、P99 スパイクの原因を特定して。"),
                    timestamp: isoAgo(minutes: 24)
                ),
                Self.toolMsg(id: "sub2-t1", step: 1, tools: ["RunBenchmark", "InspectProfile"]),
                Self.agentMsg(
                    id: "sub2-a2",
                    step: 2,
                    model: "gemini-3.8-flash",
                    modelName: "Gemini 3.8 Flash",
                    text: Self.t(
                        """
                        压测采样已完成，P99 耗时 **184ms** 主要集中在两处：

                        - `regexp.Compile` 在规则循环内重复编译（占总 CPU 44%），改用启动期预编译后可降至 **< 1%**.
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
            modelId: "gemini-3.8-flash",
            modelName: "Gemini 3.8 Flash",
            startedAt: isoAgo(minutes: 24),
            contextMeter: Self.meter(input: 14_200, output: 940, tps: 118.5, ttft: 0.24),
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
                    text: Self.t("根据审计和性能结论，为 `handlers/rule_eval_test.go` 补充白名单校验与批量超时单测。", "Add allowlist validation and batch timeout unit tests to `handlers/rule_eval_test.go` based on audit & perf findings.", "監査と性能解析の結果をもとに、handlers/rule_eval_test.go にホワイトリスト検証とバッチタイムアウトの単体テストを追加して。"),
                    timestamp: isoAgo(minutes: 22)
                ),
                Self.toolMsg(id: "sub3-t1", step: 1, tools: ["EditFile", "GoTest"]),
                Self.agentMsg(
                    id: "sub3-a2",
                    step: 2,
                    model: "gpt-6-astra",
                    modelName: "GPT-6 Astra",
                    text: Self.t("正在写入 6 组表驱动测试用例并执行并发竞争检验，完成后会自动向主流程汇报。", "Writing 6 table-driven test cases and running race checks; will report back to the parent workflow once complete.", "6 パターンのテーブル駆動テストを作成し、-race 並行検証を実行中です。完了次第メインフローへ報告します。"),
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
            modelId: "gpt-6-astra",
            modelName: "GPT-6 Astra",
            startedAt: isoAgo(minutes: 22),
            contextMeter: Self.meter(input: 11_800, output: 520, tps: 78.2, ttft: 0.45),
            swarm: nil,
            subagents: [],
            parentConversationId: "demo-0011-workflow",
            subagentRole: Self.t("自动化测试工程师", "Test Engineer", "テスト自動化エンジニア"),
            gitStatus: nil,
            changes: [],
            chapters: []
        )

        // MARK: - 16 Showcase + Everyday Conversations

        // 1. Pinned Classic + Running Task + Queued Message (Multigravity)
        let c01Upload = Conversation(
            id: "demo-0001-upload",
            title: Self.t("实现文件附件上传", "File Attachment Upload Flow", "ファイル添付アップロード実装"),
            workspaceName: "Multigravity",
            workspaceUri: "file:///Users/demo/Projects/antigravity-mobile",
            status: "CASCADE_RUN_STATUS_RUNNING",
            duration: Self.t("运行中", "Running", "実行中"),
            messages: [
                Self.userMsg(
                    id: "c01-u0",
                    step: 0,
                    text: Self.t("帮我给移动端接入“📎 文件附件上传”功能：选中的文档或图片先上传到网关 `Inbox`，再把路径一行写入消息里，并同时跑一下网关测试。", "Wire up \"📎 File Attachment Uploads\" on mobile: upload selected files/images to the gateway `Inbox`, insert attachment links into the prompt, and run gateway tests.", "モバイル側に「📎 ファイル添付アップロード」を実装して。選んだ書類や画像をゲートウェイの `Inbox` に先上げし、パスをプロンプトに自動挿入してテストも回してほしい。"),
                    timestamp: isoAgo(minutes: 9)
                ),
                Self.toolMsg(id: "c01-t1", step: 1, tools: ["ReadFile", "EditFile", "RunCommand"]),
                Self.agentMsg(
                    id: "c01-a2",
                    step: 2,
                    model: "claude-opus-5-5-thinking",
                    modelName: "Claude Opus 5.5 (Thinking)",
                    text: Self.t(
                        """
                        已完成前后两端附件上传链路与校验保护：

                        1. **网关接收端点 (`POST /api/v1/attachments`)**：按 `X-File-Name` 读取文件流，计算 SHA-256 生成 8 位文件短 ID，落盘至工作区 `Inbox/2026-11/` 并返回标准 `UploadedAttachment` 元数据。
                        2. **iOS 客户端发送拼接**：选中文件或从“图库 / 文件 / 粘贴板”导入后先并发上传，再生成固定格式引用行：
                           ```markdown
                           📎 附件（已上传到本机，可直接读取）：
                           - [sales_q3.csv](/Users/demo/Multigravity/Inbox/2026-11/9f2a4c1b-sales_q3.csv) (1.8 KB)
                           ```
                        3. **后台测试运行中**：正在执行 `go test -v ./internal/gateway -run TestAttachmentUpload`，你也可以随时在顶部卡片点击**查看实时日志**或**停止任务**。
                        """,
                        """
                        Implemented end-to-end attachment uploads with validation:

                        1. **Gateway Upload Endpoint (`POST /api/v1/attachments`)**: Reads filename via `X-File-Name`, computes an 8-char SHA-256 ID, saves to `Inbox/2026-11/`, and returns `UploadedAttachment` metadata.
                        2. **iOS Composer Integration**: Uploads selected Photos, Files, or clipboard items concurrently and injects formatted lines:
                           ```markdown
                           📎 Attachments (uploaded locally):
                           - [sales_q3.csv](/Users/demo/Multigravity/Inbox/2026-11/9f2a4c1b-sales_q3.csv) (1.8 KB)
                           ```
                        3. **Background Tests Running**: `go test -v ./internal/gateway -run TestAttachmentUpload` is active — tap the **Live Tasks** bar above to stream logs or stop it anytime.
                        """,
                        """
                        フロント・バックエンド双方の添付アップロード経路とバリデーションを実装しました：

                        1. **ゲートウェイ受信 API（`POST /api/v1/attachments`）**：`X-File-Name` からファイル名を取得し、SHA-256 の先頭 8 桁 ID で `Inbox/2026-11/` へ保存、`UploadedAttachment` メタデータを返却。
                        2. **iOS 入力欄との連携**：写真・ファイル・クリップボードから選択した添付を並列アップロードし、定型リンクを自動挿入：
                           ```markdown
                           📎 添付ファイル（ローカルにアップロード済み）：
                           - [sales_q3.csv](/Users/demo/Multigravity/Inbox/2026-11/9f2a4c1b-sales_q3.csv) (1.8 KB)
                           ```
                        3. **バックグラウンドテスト実行中**：現在 `go test -v ./internal/gateway -run TestAttachmentUpload` が稼働中です。上部の **実行中タスクバー** からライブログ閲覧や停止ができます。
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
                    "toolName": "RunCommand",
                    "toolSummary": Self.t("验证多端附件上传测试", "Verify attachment upload tests", "マルチ端末の添付アップロードテストを検証"),
                    "toolAction": "running",
                    "startedAt": isoAgo(minutes: 2),
                    "isDaemon": false
                ]
            ],
            queuedMessages: [
                [
                    "id": "queue-demo-1",
                    "text": Self.t("测试全部通过后，再帮我把大于 25MB 的文件拦截提示补一条中文错误码文档。", "Once tests pass, document the HTTP 413 error code for >25MB attachments.", "テストが全部通ったら、25MB 超えファイル拒否時の HTTP 413 エラーコード仕様もドキュメントに追記して。"),
                    "createdAt": isoAgo(minutes: 1)
                ]
            ],
            lastModified: ago(minutes: 1),
            unread: false,
            pinned: true,
            modelId: "claude-opus-5-5-thinking",
            modelName: "Claude Opus 5.5 (Thinking)",
            startedAt: isoAgo(minutes: 9),
            contextMeter: Self.meter(input: 48_600, output: 3_420, tps: 56.4, ttft: 0.62),
            swarm: nil,
            subagents: [],
            parentConversationId: nil,
            subagentRole: nil,
            gitStatus: Self.demoGitState(
                branch: "feat/attachment-inbox",
                ahead: 2,
                modifiedCount: 2,
                prTitle: "feat(attachments): persist uploads under Inbox with sha256 short id",
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
                        ("INSERT", "    id := shortSHA256(payload)[:8]"),
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
                ["id": "c01-ch1", "title": Self.t("需求与 Inbox 路径规范", "Requirements & Inbox Spec", "要件定義と Inbox 保存規約"), "stepIndex": 0, "summary": Self.t("定义多端附件落盘规则与短 ID 格式", "Define upload layout & SHA-256 short ID", "アップロード保存先と SHA-256 短縮 ID 形式を定義")],
                ["id": "c01-ch2", "title": Self.t("网关上传接口与单测", "Gateway Endpoint & Tests", "ゲートウェイ API と単体テスト"), "stepIndex": 2, "summary": Self.t("实现 SHA-256 持久化并运行 go test", "Persist files and run go test", "SHA-256 永続化を実装し go test を実行")]
            ]
        )

        // 2. Multi-turn Refactor + Chapters TOC + Changes + PR #412 + ContextMeter
        let c10Turns = Conversation(
            id: "demo-0010-turns",
            title: Self.t("多轮重构与代码审校（Chapters · Diff）", "Multi-Turn Refactor (Chapters · Diff)", "マルチターンリファクタ（目次 · Diff）"),
            workspaceName: "Multigravity",
            workspaceUri: "file:///Users/demo/Projects/antigravity-mobile",
            status: "CASCADE_RUN_STATUS_IDLE",
            duration: Self.t("24秒", "24s", "24秒"),
            messages: [
                Self.userMsg(id: "c10-u0", step: 0, text: Self.t("先帮我梳理 `APIClient` 目前有哪些网络重试痛点，给出改造方案。", "Audit current retry & reconnect pain points in `APIClient` and propose a clean fix.", "現在の `APIClient` にあるリトライ・再接続まわりの課題を洗い出して、きれいな改善案を出して。"), timestamp: isoAgo(minutes: 45)),
                Self.toolMsg(id: "c10-t1", step: 1, tools: ["GrepSearch", "ReadFile"]),
                Self.agentMsg(
                    id: "c10-a2",
                    step: 2,
                    model: "gemini-4-carbon-high",
                    modelName: "Gemini 4 Carbon (High)",
                    text: Self.t(
                        """
                        排查 `Services/APIClient*.swift` 后发现 3 个主要痛点：

                        1. **401 / 502 重试未区分幂等方法**，POST 请求超时也被重复发送。
                        2. **SSE 断线重连延迟固定 3s**，切换 Wi-Fi / 5G 时恢复偏慢。
                        3. **大附件预览未复用内存缓存**，每次返回详情都重新拉流。
                        """,
                        """
                        Found 3 reliability gaps across `Services/APIClient*.swift`:

                        1. **Non-idempotent retries on 401/502**: timed-out `POST` mutations were retried automatically.
                        2. **Fixed 3s SSE reconnect timer**: slow recovery when switching between Wi-Fi and 5G.
                        3. **Uncached attachment previews**: large artifacts re-downloaded on every sheet open.
                        """,
                        """
                        `Services/APIClient*.swift` を調査したところ、主に 3 つの信頼性課題が見つかりました：

                        1. **401 / 502 リトライで冪等性を未判定**：タイムアウトした `POST` 更新系も自動再送されてしまいます。
                        2. **SSE 再接続が 3 秒固定**：Wi-Fi ⇔ 5G 切替時の復帰がワンテンポ遅れます。
                        3. **大型添付プレビューのメモリキャッシュ未実装**：シートを開くたびに再取得が発生しています。
                        """
                    ),
                    timestamp: isoAgo(minutes: 44)
                ),
                Self.userMsg(id: "c10-u3", step: 3, text: Self.t("好，第 1 和第 2 点先按指数退避 + 仅对 GET 幂等请求重试来改，第 3 点加 LRU 缓存。", "Apply exponential backoff only on idempotent GETs for #1, NWPathMonitor reconnect for #2, and an LRU cache for #3.", "いいね。1 は冪等な GET のみ指数バックオフ、2 は NWPathMonitor で即時再接続、3 は LRU キャッシュを入れて進めて。"), timestamp: isoAgo(minutes: 39)),
                Self.toolMsg(id: "c10-t4", step: 4, tools: ["EditFile", "EditFile", "XcodeBuild"]),
                Self.agentMsg(
                    id: "c10-a5",
                    step: 5,
                    model: "gemini-4-carbon-high",
                    modelName: "Gemini 4 Carbon (High)",
                    text: Self.t(
                        """
                        已完成第 1 阶段改造并验证编译通过：

                        - `RetryPolicy.swift`：对 `GET` / `HEAD` 开启 `0.4s -> 0.8s -> 1.6s` 退避，POST 严格不自动重发。
                        - `StreamWatcher.swift`：监听 `NWPathMonitor` 切换事件，网络切换后立即重连 SSE。
                        - `PreviewCache.swift`：新增 64MB `NSCache` 预览缓存。
                        """,
                        """
                        Phase 1 landed and compiles cleanly:

                        - `RetryPolicy.swift`: `0.4s -> 0.8s -> 1.6s` backoff scoped strictly to `GET` / `HEAD`.
                        - `StreamWatcher.swift`: `NWPathMonitor` triggers instant SSE reconnect on interface handoff.
                        - `PreviewCache.swift`: Added 64MB `NSCache` artifact buffer.
                        """,
                        """
                        第 1 フェーズの改修とビルド検証を完了しました：

                        - `RetryPolicy.swift`：`GET` / `HEAD` に限定して `0.4s -> 0.8s -> 1.6s` の指数バックオフを適用（`POST` は再送なし）。
                        - `StreamWatcher.swift`：`NWPathMonitor` で回線切替を検知し、SSE を即時再接続。
                        - `PreviewCache.swift`：64MB 上限の `NSCache` プレビューバッファを追加。
                        """
                    ),
                    timestamp: isoAgo(minutes: 37)
                ),
                Self.userMsg(id: "c10-u6", step: 6, text: Self.t("帮我再确认下后台切回前台时，如果网关重连失败会不会弹出重复 Toast？", "Verify foreground transitions won't spam duplicate error toasts if the gateway is unreachable.", "バックグラウンドから復帰したとき、ゲートウェイ未接続でも重複トーストが出ないか念のため確認して。"), timestamp: isoAgo(minutes: 31)),
                Self.agentMsg(
                    id: "c10-a7",
                    step: 7,
                    model: "gemini-4-carbon-high",
                    modelName: "Gemini 4 Carbon (High)",
                    text: Self.t("已补充 8 秒防抖窗口与错误签名去重（`lastErrorSignature`），短时间内同一网关断连只会提示 1 次。右上角可点击 **Changes / Git** 直接查看本轮全部文件 Diff 与已合并 PR `#412`。", "Added an 8s debounce window with `lastErrorSignature` deduplication so repeated failures surface at most once. Open **Changes / Git** in the toolbar to inspect the full diff and merged PR `#412`.", "8 秒のデバウンス窓とエラー署名重複排除（`lastErrorSignature`）を追加したため、短時間の連続切断でも通知は 1 回のみ表示されます。上部バーの **Changes / Git** から全ファイル差分とマージ済み PR `#412` を確認できます。"),
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
            modelId: "gemini-4-carbon-high",
            modelName: "Gemini 4 Carbon (High)",
            startedAt: isoAgo(minutes: 45),
            contextMeter: Self.meter(input: 112_400, output: 8_910, tps: 84.3, ttft: 0.36),
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
                ["id": "ch-1", "title": Self.t("诊断重试与缓存痛点", "Diagnose Retry & Cache Gaps", "リトライ＆キャッシュ課題の診断"), "stepIndex": 0, "summary": Self.t("梳理 401/502 误重试、固定 3s 重连与缓存缺失", "Identify non-idempotent retries & uncached assets", "非冪等リトライ・固定 3s 再接続・キャッシュ欠落を特定")],
                ["id": "ch-2", "title": Self.t("实现退避策略与路径监听", "Backoff & NWPathMonitor", "指数バックオフと回線監視の実装"), "stepIndex": 3, "summary": Self.t("新增 RetryPolicy 与 NWPathMonitor 立即重连", "Add RetryPolicy and instant SSE reconnect", "RetryPolicy と NWPathMonitor 即時再接続を追加")],
                ["id": "ch-3", "title": Self.t("Toast 防抖与错误去重", "Toast Debounce & Dedup", "トースト重複排除とデバウンス"), "stepIndex": 6, "summary": Self.t("8 秒窗口防抖避免反复弹窗", "8s debounce to suppress duplicate alerts", "8秒ウィンドウで連続エラー通知を抑制")]
            ]
        )

        // 3. Dynamic Workflow + Clickable Subagents
        let c11Workflow = Conversation(
            id: "demo-0011-workflow",
            title: Self.t("动态多 Agent 质量门禁巡检", "Parallel Quality Gate Workflow", "動的マルチ Agent 品質ゲート巡回"),
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
                    stepIndex: 1,
                    model: "claude-opus-5-5-thinking",
                    modelName: "Claude Opus 5.5 (Thinking)",
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
                        "duration": Self.t("进行中 · 25s", "Running · 25s", "実行中 · 25秒")
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
                    model: "claude-opus-5-5-thinking",
                    modelName: "Claude Opus 5.5 (Thinking)",
                    text: Self.t("前两个并行子代理已完成分析（点击上方**工作流卡片**或**子代理胶囊**可直接点进去查看各自完整对话详情），第 3 个测试子代理正在补齐单测中。", "Phase 1 subagents finished (tap any **workflow step** or **subagent pill** above to open its full transcript); the test-author subagent is wrapping up coverage.", "Phase 1 の並列サブエージェント 2 名が完了しました（上の**ワークフローカード**や**サブエージェントバッジ**をタップすると個別の詳細ログを開けます）。現在 3 人目のテスト担当がカバレッジを補完中です。"),
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
            modelId: "claude-opus-5-5-thinking",
            modelName: "Claude Opus 5.5 (Thinking)",
            startedAt: isoAgo(minutes: 26),
            contextMeter: Self.meter(input: 64_200, output: 4_120, tps: 52.8, ttft: 0.68),
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

        // 4. Swarm 4-Tab Collaboration (/teamwork)
        let c12Swarm = Conversation(
            id: "demo-0012-swarm",
            title: Self.t("Swarm 协同开发 · 离线草稿同步（/teamwork）", "Swarm Teamwork · Offline Draft Merge", "Swarm 協調開発 · オフライン下書き同期（/teamwork）"),
            workspaceName: "Multigravity",
            workspaceUri: "file:///Users/demo/Projects/antigravity-mobile",
            status: "CASCADE_RUN_STATUS_IDLE",
            duration: Self.t("38秒", "38s", "38秒"),
            messages: [
                Self.userMsg(
                    id: "c12-u0",
                    step: 0,
                    text: Self.t("/teamwork 组建一个小队同时完成「离线草稿冲突合并」的后端协议、iOS 持久化以及 E2E 校验，并在面板实时同步进度。", "/teamwork Spin up a squad to implement offline draft conflict merging across backend CAS, iOS persistence, and cross-review.", "/teamwork チームを編成して「オフライン下書き競合マージ」のバックエンド楽観ロック・iOS 永続化・相互レビューを並列で進めて、ボードに進捗を同期して。"),
                    timestamp: isoAgo(minutes: 50),
                    slashCommands: [["name": "teamwork", "description": Self.t("组建多代理协同团队与共享公告板", "Coordinate multiple agents via shared board", "共有ボードで複数エージェントを並列協調")]]
                ),
                Self.agentMsg(
                    id: "c12-a1",
                    step: 1,
                    model: "claude-opus-5-5-thinking",
                    modelName: "Claude Opus 5.5 (Thinking)",
                    text: Self.t(
                        """
                        已拉起 **Swarm 协同团队（4 位成员）** 并在独立工作区共享公告板上完成协同交付：

                        - **`@backend`（GPT-6 Astra）**：新增 `version` 乐观锁字段，冲突时返回 `409 Conflict` 与服务端快照。
                        - **`@ios-ui`（Claude Opus 5.5）**：在 `DraftStore` 实现三路合并提示条，保留用户未发文字。
                        - **`@reviewer`（Gemini 4 Carbon）**：已评审全部 3 个文件，未发现死锁或数据覆盖缺陷。

                        点击顶部或工具栏中的 **Swarm** 按钮可切换 4 个标签页（总览 / 成员 / 公告板 / 互动发帖），可直接在公告板向任意 `@成员` 下发新指令。
                        """,
                        """
                        Launched a **4-agent Swarm** on the shared coordination board:

                        - **`@backend` (GPT-6 Astra)**: Added optimistic `version` CAS checks returning `409 Conflict` snapshots.
                        - **`@ios-ui` (Claude Opus 5.5)**: Implemented 3-way draft merge prompts in `DraftStore` preserving unsent edits.
                        - **`@reviewer` (Gemini 4 Carbon)**: Verified all 3 modified files with zero race or overwrite risks.

                        Tap **Swarm** in the toolbar to explore Summary, Members, Message Board, and live Feedback.
                        """,
                        """
                        **4 名の Swarm チーム**を起動し、共有ボード上で協調開発を完了しました：

                        - **`@backend`（GPT-6 Astra）**：`version` 楽観ロックを追加し、競合時に `409 Conflict` とサーバースナップショットを返却。
                        - **`@ios-ui`（Claude Opus 5.5）**：`DraftStore` に 3-way マージ通知バーを実装し、未送信テキストを保護。
                        - **`@reviewer`（Gemini 4 Carbon）**：変更 3 ファイルを全件レビュー済み。レースや上書きリスクなし。

                        上部ツールバーの **Swarm** ボタンから 4 タブ（概要 / メンバー / ボード / フィードバック）を切り替え、任意の `@メンバー` に直接追加指示を出せます。
                        """
                    ),
                    timestamp: isoAgo(minutes: 40)
                )
            ],
            pendingInteraction: nil,
            canProceed: false,
            proceedArtifactUri: nil,
            runningTasks: [],
            queuedMessages: [],
            lastModified: ago(minutes: 18),
            unread: false,
            pinned: false,
            modelId: "claude-opus-5-5-thinking",
            modelName: "Claude Opus 5.5 (Thinking)",
            startedAt: isoAgo(minutes: 50),
            contextMeter: Self.meter(input: 79_300, output: 5_240, tps: 54.0, ttft: 0.64),
            swarm: Self.initialSwarmSnapshot(minutesAgo: 40),
            subagents: [],
            parentConversationId: nil,
            subagentRole: nil,
            gitStatus: Self.demoGitState(
                branch: "feat/swarm-draft-merge",
                ahead: 3,
                modifiedCount: 2,
                prTitle: "feat(drafts): optimistic concurrency merge for mobile offline drafts",
                prNumber: 418,
                prState: "DRAFT"
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
                )
            ],
            chapters: []
        )

        // 5. Classic Error & Retry Recovery
        let c02Login500 = Conversation(
            id: "demo-0002-login-500",
            title: Self.t("登录接口 500 错误排查", "Auth Login 500 Incident Triage", "ログイン API 500 エラー調査"),
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
                Self.toolMsg(id: "c02-t1", step: 1, tools: ["GrepSearch", "ReadFile", "RunPostgresExplain"]),
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
            modelId: "claude-opus-5-5-thinking",
            modelName: "Claude Opus 5.5 (Thinking)",
            startedAt: isoAgo(minutes: 28),
            contextMeter: Self.meter(input: 32_100, output: 1_840, tps: 50.2, ttft: 0.70),
            swarm: nil,
            subagents: [],
            parentConversationId: nil,
            subagentRole: nil,
            gitStatus: nil,
            changes: [],
            chapters: []
        )

        // 6. Classic Interactive Multi-Question ask_question
        let c03Migration = Conversation(
            id: "demo-0003-migration",
            title: Self.t("数据库迁移方案评审", "Zero-Downtime DB Migration Review", "無停止 DB マイグレーション設計レビュー"),
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
                Self.toolMsg(id: "c03-t1", step: 1, tools: ["ReadFile", "InspectMigrations"]),
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
            contextMeter: Self.meter(input: 41_500, output: 2_380, tps: 76.0, ttft: 0.41),
            swarm: nil,
            subagents: [],
            parentConversationId: nil,
            subagentRole: nil,
            gitStatus: nil,
            changes: [],
            chapters: []
        )

        // 7. Classic Data Report: Unread + Attached CSV/MD + Inline Chart PNG + 4-page PDF
        let chartBase64 = chartData.base64EncodedString()
        let c04Sales = Conversation(
            id: "demo-0004-sales",
            title: Self.t("整理季度销售报告（4页 PDF）", "Q3 Sales Report & Charts (4-Page PDF)", "Q3 売上分析レポート作成（4ページ PDF）"),
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
                Self.toolMsg(id: "c04-t1", step: 1, tools: ["ReadCSV", "GenerateChart", "BuildPDF"]),
                Self.agentMsg(
                    id: "c04-a2",
                    step: 2,
                    model: "gemini-4-carbon-high",
                    modelName: "Gemini 4 Carbon (High)",
                    text: Self.t(
                        """
                        已完成 Q1–Q3 销售归因分析并打包了 **4 页高管汇报 PDF**（点击下方文件名或图表即可全屏预览与分享导出）：

                        | 区域 | Q1 (万) | Q2 (万) | Q3 (万) | 环比增速 |
                        | --- | ---: | ---: | ---: | ---: |
                        | 华东区 | 240 | 288 | 356 | **+23.6%** |
                        | 华南区 | 195 | 224 | 271 | +21.0% |
                        | 华北区 | 162 | 175 | 209 | +19.4% |

                        ![Q1-Q3 分区销售增长趋势图](data:image/png;base64,\(chartBase64))

                        ### 生成产物与原始文档（点击预览）
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

                        ![Q1-Q3 Regional Sales Trend](data:image/png;base64,\(chartBase64))

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

                        ![Q1-Q3 エリア別売上推移グラフ](data:image/png;base64,\(chartBase64))

                        ### 成果物と元データ（タップでプレビュー）
                        - 📄 [q3_sales_report.pdf](file:///Users/demo/Documents/q3_sales_report.pdf)（全 4 ページ：サマリー / 推移グラフ / エリア別詳細 / Q4 施策）
                        - 📊 [sales_chart.png](file:///Users/demo/Documents/sales_chart.png)
                        - 📑 [sales_q3.csv](file:///Users/demo/Documents/sales_q3.csv) · [meeting_notes.md](file:///Users/demo/Documents/meeting_notes.md)
                        """
                    ),
                    artifacts: [
                        [
                            "uri": Self.t("file:///Users/demo/Documents/三季度销售报告.pdf", "file:///Users/demo/Documents/q3_sales_report.pdf", "file:///Users/demo/Documents/q3_sales_report.pdf"),
                            "title": Self.t("三季度销售报告.pdf", "q3_sales_report.pdf", "q3_sales_report.pdf")
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
            modelId: "gemini-4-carbon-high",
            modelName: "Gemini 4 Carbon (High)",
            startedAt: isoAgo(minutes: 62),
            contextMeter: Self.meter(input: 36_800, output: 3_110, tps: 81.2, ttft: 0.39),
            swarm: nil,
            subagents: [],
            parentConversationId: nil,
            subagentRole: nil,
            gitStatus: nil,
            changes: [],
            chapters: []
        )

        // 8. Classic Implementation Plan with Mermaid + Proceed Approval Card (Multigravity)
        let c05Redesign = Conversation(
            id: "demo-0005-redesign",
            title: Self.t("官网首页改版方案（Proceed 审批）", "Homepage Redesign Plan (Proceed)", "公式サイトトップ刷新計画（Proceed 承認）"),
            workspaceName: "Multigravity",
            workspaceUri: "file:///Users/demo/Projects/antigravity-mobile",
            status: "CASCADE_RUN_STATUS_IDLE",
            duration: Self.t("14秒", "14s", "14秒"),
            messages: [
                Self.userMsg(
                    id: "c05-u0",
                    step: 0,
                    text: Self.t("帮我先写一份官网首页改版的架构与迁移 `implementation_plan.md`，支持渐进发布与性能指标观测，我审批后再动手改代码。", "Draft an `implementation_plan.md` for our homepage redesign with progressive canary rollout and telemetry before editing code.", "コードを書く前に、公式サイトトップページ刷新の段階的カナリア配信と指標監視を含む `implementation_plan.md` を作成して。承認したら実装に進もう。"),
                    timestamp: isoAgo(minutes: 75)
                ),
                Self.toolMsg(id: "c05-t1", step: 1, tools: ["ReadWorkspace", "WriteArtifact"]),
                Self.agentMsg(
                    id: "c05-a2",
                    step: 2,
                    model: "claude-opus-5-5-thinking",
                    modelName: "Claude Opus 5.5 (Thinking)",
                    text: Self.t(
                        """
                        已起草新版首页架构与发布方案，核心流量切换链路如下：

                        ```mermaid
                        flowchart LR
                            U["访客流量"] --> EDGE["CDN Edge Worker"]
                            EDGE -->|"10% 灰度"| NEW["Next.js 16 SSR 首页"]
                            EDGE -->|"90% 保底"| OLD["旧版静态页面"]
                            NEW -->|"LCP < 1.2s"| PROMOTE["全量放量 100%"]
                        ```

                        完整实施清单见 Artifact：[implementation_plan.md](file:///Users/demo/.gemini/antigravity/brain/demo-0005/implementation_plan.md)。你可以点击文档查看全文，或直接点击下方 **Proceed** 审批通过并启动编码。
                        """,
                        """
                        Drafted the homepage architecture & canary rollout pipeline:

                        ```mermaid
                        flowchart LR
                            U["Visitor Traffic"] --> EDGE["CDN Edge Worker"]
                            EDGE -->|"10% Canary"| NEW["Next.js 16 SSR Home"]
                            EDGE -->|"90% Baseline"| OLD["Legacy Static Bundle"]
                            NEW -->|"LCP < 1.2s"| PROMOTE["Promote 100%"]
                        ```

                        Full spec artifact: [implementation_plan.md](file:///Users/demo/.gemini/antigravity/brain/demo-0005/implementation_plan.md). Open the document or tap **Proceed** below to start implementation.
                        """,
                        """
                        新トップページのアーキテクチャとカナリア配信計画をまとめました：

                        ```mermaid
                        flowchart LR
                            U["アクセス流入"] --> EDGE["CDN Edge Worker"]
                            EDGE -->|"10% カナリア"| NEW["Next.js 16 SSR 新トップ"]
                            EDGE -->|"90% 現行"| OLD["旧静的バンドル"]
                            NEW -->|"LCP < 1.2s"| PROMOTE["100% 全量切替"]
                        ```

                        詳細な実装計画はこちら：[implementation_plan.md](file:///Users/demo/.gemini/antigravity/brain/demo-0005/implementation_plan.md)。内容を確認のうえ、下の **Proceed** をタップすると実装を開始します。
                        """
                    ),
                    artifacts: [
                        [
                            "uri": "file:///Users/demo/.gemini/antigravity/brain/demo-0005/implementation_plan.md",
                            "title": "implementation_plan.md",
                            "request_feedback": true
                        ]
                    ],
                    timestamp: isoAgo(minutes: 70)
                )
            ],
            pendingInteraction: nil,
            canProceed: true,
            proceedArtifactUri: "file:///Users/demo/.gemini/antigravity/brain/demo-0005/implementation_plan.md",
            runningTasks: [],
            queuedMessages: [],
            lastModified: ago(minutes: 70),
            unread: false,
            pinned: false,
            modelId: "claude-opus-5-5-thinking",
            modelName: "Claude Opus 5.5 (Thinking)",
            startedAt: isoAgo(minutes: 75),
            contextMeter: Self.meter(input: 29_500, output: 2_640, tps: 53.7, ttft: 0.65),
            swarm: nil,
            subagents: [],
            parentConversationId: nil,
            subagentRole: nil,
            gitStatus: nil,
            changes: [],
            chapters: []
        )

        // 9. Classic Receipt OCR with 2 Image Previews & Reconciliation Table
        let c06Invoice = Conversation(
            id: "demo-0006-invoice",
            title: Self.t("发票识别与整理（多图识别）", "Receipt OCR & Expense Table", "領収書 OCR 読取＆経費精算表（複数画像）"),
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
                Self.toolMsg(id: "c06-t1", step: 1, tools: ["OCRDocument", "VerifyTaxInvoice"]),
                Self.agentMsg(
                    id: "c06-a2",
                    step: 2,
                    model: "gemini-4-carbon-high",
                    modelName: "Gemini 4 Carbon (High)",
                    text: Self.t(
                        """
                        已完成 **2 张报销发票** OCR 提取与验真（点上方图片可双指缩放查看原票细节）：

                        | 序号 | 报销类别 | 开票商户 / 行程 | 发票号码 | 含税金额 | 验真状态 |
                        | :---: | --- | --- | --- | ---: | :---: |
                        | 1 | 业务招待 / 餐饮 | 星河精品咖啡（北京国贸店） | `NO.20261108-88412` | **¥158.00** | ✅ 已验真 |
                        | 2 | 市内交通 / 出租 | 京B·82716 首都机场 → 国贸CBD | `NO.20261108-51904` | **¥116.00** | ✅ 已验真 |

                        **合计报销金额：¥274.00**。如需自动导出报销汇总 CSV，直接回复「导出 CSV」即可。
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

                        **精算合計：¥11,850**。経理提出用の CSV 出力が必要なら「CSV を出力」と返信してください。
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
            modelId: "gemini-4-carbon-high",
            modelName: "Gemini 4 Carbon (High)",
            startedAt: isoAgo(minutes: 95),
            contextMeter: Self.meter(input: 22_800, output: 1_510, tps: 79.4, ttft: 0.34),
            swarm: nil,
            subagents: [],
            parentConversationId: nil,
            subagentRole: nil,
            gitStatus: nil,
            changes: [],
            chapters: []
        )

        // 10. Pending Workflow Approval Interaction
        let c13WorkflowApproval = Conversation(
            id: "demo-0013-wf-approval",
            title: Self.t("高危发布脚本审批（Workflow Approval）", "Production Script Approval Gate", "本番デプロイスクリプト承認（Workflow 実行承認）"),
            workspaceName: Self.t("部署脚本", "Deploy Scripts", "デプロイスクリプト"),
            workspaceUri: "file:///Users/demo/Projects/deploy-scripts",
            status: "CASCADE_RUN_STATUS_RUNNING",
            duration: Self.t("等待审批", "Awaiting Approval", "承認待ち"),
            messages: [
                Self.userMsg(
                    id: "c13-u0",
                    step: 0,
                    text: Self.t("执行生产 Redis 缓存前缀清理并重载 Gateway 配置，先跑一次预检查然后向我申请执行确认。", "Run a preflight dry-run for cleaning expired Redis session keys and reloading the gateway config, then request my approval.", "本番 Redis の期限切れセッションキー掃除と Gateway 設定リロードをやりたい。まずドライランで影響件数を確認して、実行前に承認リクエストを送って。"),
                    timestamp: isoAgo(minutes: 105)
                ),
                Self.toolMsg(id: "c13-t1", step: 1, tools: ["InspectDeployScript", "DryRunRedisCleanup"]),
                Self.agentMsg(
                    id: "c13-a2",
                    step: 2,
                    model: "claude-opus-5-5-thinking",
                    modelName: "Claude Opus 5.5 (Thinking)",
                    text: Self.t("预检查完成：将扫描并删除 `v1:session:tmp:*` 下约 14,200 个过期键，随后向本机集群发送 `SIGHUP` 热重载。请在下方授权卡片中批准或拒绝本次命令执行。", "Dry-run complete: ~14,200 expired `v1:session:tmp:*` keys matched, followed by zero-downtime `SIGHUP` config reload. Approve or deny below.", "事前チェック完了：`v1:session:tmp:*` 配下の期限切れキー約 14,200 件を削除し、その後ローカルクラスタへ無停止 `SIGHUP` リロードを実行します。下の承認カードから許可または拒否を選択してください。"),
                    timestamp: isoAgo(minutes: 103)
                )
            ],
            pendingInteraction: [
                "type": "workflow_approval",
                "trajectoryId": "demo-0013-wf-approval",
                "stepIndex": 2,
                "title": Self.t("高危环境变更 · 请求执行工作流命令", "Production Change · Command Approval Requested", "本番環境変更 · ワークフロー実行承認リクエスト"),
                "description": Self.t("该脚本将清理过期会话缓存前缀并触发生产网关配置重载。", "Cleans expired Redis session prefixes and hot-reloads production gateway.", "期限切れ Redis セッションキャッシュを削除し、本番ゲートウェイ設定をホットリロードします。"),
                "commandLine": "bash scripts/prod_cleanup.sh --prefix 'v1:session:tmp:*' --reload-gateway",
                "workflowApproval": [
                    "description": Self.t("生产网关滚动重载与前缀清理 · 预计影响 14,200 个临时键，网关平滑重载不中断活跃 SSE 连接。", "Canary Cache Cleanup & Gateway Reload · Affects ~14,200 TTL keys; active SSE streams stay connected.", "本番ゲートウェイ無停止リロード＆キャッシュ掃除 · 対象約 14,200 キー、稼働中の SSE 接続は維持されます。"),
                    "scriptPath": "scripts/prod_cleanup.sh",
                    "script": """
                    #!/usr/bin/env python3
                    from workflow import phase, run_command

                    with phase("preflight"):
                        run_command("redis-cli --scan --pattern 'v1:session:tmp:*' | wc -l")
                    with phase("cleanup_and_reload"):
                        run_command("bash scripts/prod_cleanup.sh --prefix 'v1:session:tmp:*' --reload-gateway")
                    """,
                    "estimatedPhases": [
                        Self.t("Preflight 扫描 (14,200 keys)", "Preflight Scan (14,200 keys)", "事前スキャン (14,200 keys)"),
                        Self.t("清理前缀并热重载", "Clean Keys & Reload", "キー削除＆ホットリロード")
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
            lastModified: ago(minutes: 103),
            unread: false,
            pinned: false,
            modelId: "claude-opus-5-5-thinking",
            modelName: "Claude Opus 5.5 (Thinking)",
            startedAt: isoAgo(minutes: 105),
            contextMeter: Self.meter(input: 19_400, output: 1_120, tps: 51.6, ttft: 0.61),
            swarm: nil,
            subagents: [],
            parentConversationId: nil,
            subagentRole: nil,
            gitStatus: nil,
            changes: [],
            chapters: []
        )

        // 11. Running Background Tasks + Daemon Logs
        let c14Running = Conversation(
            id: "demo-0014-running",
            title: Self.t("实时联调与守护进程监控（Daemon Logs）", "Live Dev Server & Daemon Logs", "開発サーバー＆デーモンログ監視（Daemon Logs）"),
            workspaceName: Self.t("部署脚本", "Deploy Scripts", "デプロイスクリプト"),
            workspaceUri: "file:///Users/demo/Projects/deploy-scripts",
            status: "CASCADE_RUN_STATUS_RUNNING",
            duration: Self.t("运行中", "Running", "実行中"),
            messages: [
                Self.userMsg(
                    id: "c14-u0",
                    step: 0,
                    text: Self.t("同时把 Vite 热更新前端守护进程和 Go `-race` 并发回归测试跑起来，方便我在手机上实时看日志和按需中止。", "Start the Vite HMR dev server daemon and the Go `-race` test suite so I can tail logs and manage tasks from mobile.", "Vite の HMR 開発サーバーと Go の `-race` 並行回帰テストを同時に走らせて。スマホからリアルタイムログ確認と個別停止ができるようにしたい。"),
                    timestamp: isoAgo(minutes: 112)
                ),
                Self.toolMsg(id: "c14-t1", step: 1, tools: ["StartDaemon", "RunCommand"]),
                Self.agentMsg(
                    id: "c14-a2",
                    step: 2,
                    model: "gpt-6-astra",
                    modelName: "GPT-6 Astra",
                    text: Self.t(
                        """
                        已同时启动 2 个后台进程：

                        1. **守护进程 `task-vite-daemon`**：监听 `0.0.0.0:5173`，供局域网浏览器实时预览。
                        2. **阻塞任务 `task-go-race`**：正在执行 `./internal/cockpit/...` 并发竞态测试。

                        点击对话顶部的**后台任务栏**（或左下角运行指示器）可展开实时输出日志，或单独停止其中某个任务。
                        """,
                        """
                        Started 2 background tasks:

                        1. **Daemon `task-vite-daemon`**: Listening on `0.0.0.0:5173` for LAN live preview.
                        2. **Blocking Task `task-go-race`**: Running `-race` tests across `./internal/cockpit/...`.

                        Tap the **Live Tasks** bar above (or the status pill) to tail live logs or stop either process individually.
                        """,
                        """
                        バックグラウンドタスクを 2 件起動しました：

                        1. **常駐デーモン `task-vite-daemon`**：`0.0.0.0:5173` で待機中（LAN プレビュー対応）。
                        2. **検証タスク `task-go-race`**：`./internal/cockpit/...` の `-race` テストを実行中。

                        画面上部の **実行中タスクバー**（またはステータス表示）からライブログ確認や個別停止ができます。
                        """
                    ),
                    timestamp: isoAgo(minutes: 110)
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
                    "toolName": "RunCommand",
                    "toolSummary": Self.t("Web 控制台 HMR 守护进程", "Web Console HMR Daemon", "Web コンソール HMR デーモン"),
                    "toolAction": "running",
                    "startedAt": isoAgo(minutes: 6),
                    "isDaemon": true
                ],
                [
                    "id": "task-go-race",
                    "stepIndex": 1,
                    "commandLine": "go test -race -count=1 ./internal/cockpit/...",
                    "toolName": "RunCommand",
                    "toolSummary": Self.t("Go 并发竞态测试", "Go Race-Detector Suite", "Go 並行レース検出テスト"),
                    "toolAction": "running",
                    "startedAt": isoAgo(minutes: 1.5),
                    "isDaemon": false
                ]
            ],
            queuedMessages: [],
            lastModified: ago(minutes: 110),
            unread: false,
            pinned: false,
            modelId: "gpt-6-astra",
            modelName: "GPT-6 Astra",
            startedAt: isoAgo(minutes: 112),
            contextMeter: Self.meter(input: 27_400, output: 1_960, tps: 88.0, ttft: 0.37),
            swarm: nil,
            subagents: [],
            parentConversationId: nil,
            subagentRole: nil,
            gitStatus: nil,
            changes: [],
            chapters: []
        )

        // 12. Context Overflow + /compact rescue
        let c15Overflow = Conversation(
            id: "demo-0015-overflow",
            title: Self.t("上下文超限自救演示（Context Overflow · /compact）", "Context Overflow Recovery (/compact)", "コンテキスト上限リカバリ（Context Overflow · /compact）"),
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
                Self.toolMsg(id: "c15-t1", step: 1, tools: ["ReadFileBatch", "DiffJSONL"]),
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
            modelId: "claude-opus-5-5-thinking",
            modelName: "Claude Opus 5.5 (Thinking)",
            startedAt: isoAgo(minutes: 130),
            contextMeter: Self.meter(input: 184_200, output: 8_200, tps: 47.5, ttft: 0.85, isOverflow: true),
            swarm: nil,
            subagents: [],
            parentConversationId: nil,
            subagentRole: nil,
            gitStatus: nil,
            changes: [],
            chapters: []
        )

        // 13. Classic Code Blocks (diff / SQL / bash) + Pre-populated Local Draft
        let c07Python = Conversation(
            id: "demo-0007-python",
            title: Self.t("Python 脚本性能优化（带草稿）", "Python Feature Pipeline Optimization", "Python 特徴量パイプライン高速化（下書きあり）"),
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
                Self.toolMsg(id: "c07-t1", step: 1, tools: ["ReadFile", "RunProfiler"]),
                Self.agentMsg(
                    id: "c07-a2",
                    step: 2,
                    model: "gpt-6-astra",
                    modelName: "GPT-6 Astra",
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

                        （底部输入框已预置一条“追问草稿”，可以直接体验局部草稿持久化与继续编辑。）
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

                        *(A follow-up draft is pre-filled in the composer below to preview persistent per-thread drafts.)*
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

                        *（下の入力欄に続きの質問下書きが入っています。スレッド別下書きの自動保存をお試しいただけます）*
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
            modelId: "gpt-6-astra",
            modelName: "GPT-6 Astra",
            startedAt: isoAgo(minutes: 165),
            contextMeter: Self.meter(input: 31_200, output: 2_410, tps: 83.4, ttft: 0.38),
            swarm: nil,
            subagents: [],
            parentConversationId: nil,
            subagentRole: nil,
            gitStatus: nil,
            changes: [],
            chapters: []
        )

        // 14. Rich Rendering Showcase: Unread + Jupyter Notebook + LaTeX + Table (Pure Chat)
        let c16Notebook = Conversation(
            id: "demo-0016-notebook",
            title: Self.t("量化策略笔记本与 LaTeX 推导（Notebook）", "Kelly Allocation & Jupyter Notebook", "ケリー基準と LaTeX 数式展開（Jupyter Notebook）"),
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

                        点击下方链接可直接在 iOS 原生预览器中查看完整分 Cell 渲染效果：
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

                        Tap below to inspect rendered Markdown & code cells natively:
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

                        下をタップすると iOS ネイティブビューアで Markdown とコードセルをそのまま確認できます：
                        📓 [analytics.ipynb](file:///Users/demo/Documents/analytics.ipynb)
                        """
                    ),
                    artifacts: [
                        [
                            "uri": "file:///Users/demo/Documents/analytics.ipynb",
                            "title": "analytics.ipynb"
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
            unread: true,
            pinned: false,
            modelId: "gemini-4-carbon-high",
            modelName: "Gemini 4 Carbon (High)",
            startedAt: isoAgo(minutes: 210),
            contextMeter: Self.meter(input: 16_400, output: 1_890, tps: 77.6, ttft: 0.42),
            swarm: nil,
            subagents: [],
            parentConversationId: nil,
            subagentRole: nil,
            gitStatus: nil,
            changes: [],
            chapters: []
        )

        // 15. Classic CSS Bug Fix + Code Diff (Multigravity)
        let c08LoginStyle = Conversation(
            id: "demo-0008-login-style",
            title: Self.t("修复登录页样式问题", "Fix Mobile Safe-Area & Dark CSS", "ログイン画面の Safe-Area ＆ダークCSS修正"),
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
                Self.toolMsg(id: "c08-t1", step: 1, tools: ["ReadFile", "EditFile"]),
                Self.agentMsg(
                    id: "c08-a2",
                    step: 2,
                    model: "gemini-3.8-flash",
                    modelName: "Gemini 3.8 Flash",
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
            modelId: "gemini-3.8-flash",
            modelName: "Gemini 3.8 Flash",
            startedAt: isoAgo(minutes: 280),
            contextMeter: Self.meter(input: 12_900, output: 980, tps: 112.0, ttft: 0.25),
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

        // 16. Classic Pure Chat: Weekend Travel Itinerary
        let c09Chat = Conversation(
            id: "demo-0009-chat",
            title: Self.t("杭州三日游行程", "3-Day Kyoto & Lake Itinerary", "京都・宇治 2泊3日のんびり家族旅行プラン"),
            workspaceName: "Chat",
            workspaceUri: "",
            status: "CASCADE_RUN_STATUS_IDLE",
            duration: Self.t("4秒", "4s", "4秒"),
            messages: [
                Self.userMsg(
                    id: "c09-u0",
                    step: 0,
                    text: Self.t("下周末带爸妈去杭州玩三天两晚，不想太赶，想吃点地道的杭帮菜、逛逛西湖与茶园，帮我排一份舒适行程。", "Plan a relaxed 3-day weekend trip with parents: scenic lake walks, local cuisine, and tea gardens without rushing.", "来週末に両親と京都へ 2 泊 3 日で行きます。歩き疲れすぎないペースで、京料理や静かなお寺、宇治茶を楽しめるゆったりプランを組んで。"),
                    timestamp: isoAgo(minutes: 420)
                ),
                Self.agentMsg(
                    id: "c09-a1",
                    step: 1,
                    model: "gemini-3.8-flash",
                    modelName: "Gemini 3.8 Flash",
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
                        Here is a unhurried **3-Day Scenic & Tea Garden Itinerary** designed with gentle walking paces:

                        ### Day 1 · Waterfront Promenade & Old Quarter
                        - **Morning**: Stroll along the shaded lakeside path and botanical gardens (hop on park shuttles anytime).
                        - **Lunch**: Book an early 11:15 AM table for seasonal local specialties and braised delicacies.
                        - **Sunset**: Relax at an overlook teahouse along the western causeway.

                        ### Day 2 · Morning Temple Walk & Highland Tea Fields
                        - **08:30 AM**: Beat the crowds with an easy 2-hour flat walk through ancient cedar groves.
                        - **Afternoon**: Head up to the hillside tea terraces for freshly roasted green tea tasting and farm-to-table lunch.
                        - **Evening**: Casual dusk stroll through the lantern-lit historic lanes.

                        ### Day 3 · Canal Cafe & Departure
                        - **Morning**: Grab pour-over coffee by the canal, browse artisan craft shops, and pick up fresh leaf tea gifts.

                        **Travel Tip**: Pack cushioned walking shoes and a compact travel umbrella; point-to-point park shuttles save knees on stone walkways.
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
            modelId: "gemini-3.8-flash",
            modelName: "Gemini 3.8 Flash",
            startedAt: isoAgo(minutes: 420),
            contextMeter: Self.meter(input: 4_800, output: 1_120, tps: 121.4, ttft: 0.22),
            swarm: nil,
            subagents: [],
            parentConversationId: nil,
            subagentRole: nil,
            gitStatus: nil,
            changes: [],
            chapters: []
        )

        conversations = [
            c01Upload,
            c10Turns,
            c11Workflow,
            c12Swarm,
            c02Login500,
            c03Migration,
            c04Sales,
            c05Redesign,
            c06Invoice,
            c13WorkflowApproval,
            c14Running,
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
        CacheManager.shared.setPinned(cascadeId: "demo-0001-upload", pinned: true)
    }

    // MARK: - Helpers & Generated Sample Assets

    private static func meter(
        input: Int64,
        output: Int64,
        tps: Double,
        ttft: Double,
        isOverflow: Bool = false
    ) -> ContextMeterItem {
        let maxTokens: Int64 = 200_000
        let used = input + output
        let pct = min(100.0, Double(used) / Double(maxTokens) * 100.0)
        return ContextMeterItem(
            usedTokens: used,
            maxTokens: maxTokens,
            percentage: pct,
            inputTokens: input,
            outputTokens: output,
            cacheReadTokens: input / 2,
            thinkingTokens: output / 3,
            timeToFirstTokenSec: ttft,
            streamingDurationSec: max(1.0, Double(output) / max(1.0, tps)),
            tokensPerSecond: tps,
            isOverflow: isOverflow,
            errorKind: isOverflow ? "context_overflow" : nil
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
        let modifiedFiles: [[String: Any]] = (0..<modifiedCount).map { idx in
            [
                "path": idx == 0 ? "ios/Multigravity/Services/RetryPolicy.swift" : "internal/gateway/attachments.go",
                "status": "M",
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
                "owner": "multigravity-demo",
                "repo": "antigravity-mobile",
                "baseBranch": "main"
            ],
            pullRequests: [
                [
                    "number": prNumber,
                    "title": prTitle,
                    "state": prState,
                    "url": "https://github.com/multigravity-demo/antigravity-mobile/pull/\(prNumber)",
                    "headBranch": branch,
                    "baseBranch": "main",
                    "draft": prState == "DRAFT",
                    "updatedAt": iso(Date().addingTimeInterval(-900))
                ]
            ]
        )
    }

    private static func initialSwarmSnapshot(minutesAgo m: Double) -> [String: Any] {
        let t = iso(Date().addingTimeInterval(-m * 60))
        return [
            "swarmId": "swarm-demo-draft-merge",
            "title": Self.t("离线草稿冲突合并小队", "Offline Draft Merge Squad", "オフライン下書き競合マージ小隊"),
            "active": true,
            "members": [
                [
                    "name": "coordinator",
                    "role": Self.t("主控协调者", "Coordinator", "コーディネーター"),
                    "model": "claude-opus-5-5-thinking",
                    "state": "waiting",
                    "isCoordinator": true
                ],
                [
                    "name": "backend",
                    "role": Self.t("Go 协议开发", "Go Backend Engineer", "Go バックエンド担当"),
                    "model": "gpt-6-astra",
                    "state": "waiting",
                    "workspace": "share"
                ],
                [
                    "name": "ios-ui",
                    "role": Self.t("iOS 客户端开发", "iOS UI Engineer", "iOS UI 担当"),
                    "model": "claude-opus-5-5-thinking",
                    "state": "waiting",
                    "workspace": "share"
                ],
                [
                    "name": "reviewer",
                    "role": Self.t("交叉代码评审", "Code Reviewer", "クロスコードレビュー"),
                    "model": "gemini-4-carbon-high",
                    "state": "waiting",
                    "workspace": "inherit"
                ]
            ],
            "messages": [
                [
                    "seq": 1,
                    "sender": "coordinator",
                    "mentions": ["backend", "ios-ui"],
                    "body": Self.t("@backend 负责在 `/api/v1/drafts` 加入 `version` 乐观锁；@ios-ui 负责客户端冲突三路合并 UI。", "@backend please add optimistic `version` locking to `/api/v1/drafts`; @ios-ui build the 3-way conflict banner.", "@backend は `/api/v1/drafts` に `version` 楽観ロックを実装、@ios-ui は 3-way 競合マージバナーをお願いします。"),
                    "timestamp": iso(Date().addingTimeInterval(-(m + 8) * 60))
                ],
                [
                    "seq": 2,
                    "sender": "backend",
                    "mentions": ["coordinator", "reviewer"],
                    "body": Self.t("服务端已完成 `version` 自增校验与 `409 Conflict` 快照响应，所有 Go 单元测试通过，请 @reviewer 核验。", "Server `version` CAS check and `409 Conflict` snapshot payload are ready; Go unit tests green. Over to @reviewer.", "サーバー側の `version` CAS 検証と `409 Conflict` スナップショット返却を実装済み。Go 単体テスト全緑です。@reviewer 確認お願いします。"),
                    "timestamp": iso(Date().addingTimeInterval(-(m + 5) * 60))
                ],
                [
                    "seq": 3,
                    "sender": "ios-ui",
                    "mentions": ["coordinator", "reviewer"],
                    "body": Self.t("客户端 `DraftStore.mergeRemote` 与底部合并提示横幅已接入，本地保留未发送草稿不会丢字。", "`DraftStore.mergeRemote` and the conflict banner are wired up; unsent local text is preserved.", "`DraftStore.mergeRemote` と下部の競合マージバナーを接続しました。未送信テキストが消えないようローカル保持しています。"),
                    "timestamp": iso(Date().addingTimeInterval(-(m + 3) * 60))
                ],
                [
                    "seq": 4,
                    "sender": "reviewer",
                    "mentions": ["coordinator"],
                    "body": Self.t("已完成两端交叉审校 ✅ 边界条件与并发读写均无问题，PR #418 已就绪。", "Cross-reviewed both patches ✅ Edge cases and concurrent mutations look solid. PR #418 is ready.", "両側のパッチを相互レビュー完了 ✅ 境界条件と並行書き込みともに問題なし。PR #418 マージ準備 OK です。"),
                    "timestamp": t
                ]
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
            # 官网首页改版技术实施方案

            ## 1. 目标与关键指标
            - 将桌面端与移动端首页 **LCP（最大内容绘制）** 从 2.6s 压降至 **< 1.2s**。
            - 首屏核心包体控制在 95KB gzip 以内，支持零停机灰度切流与一键回滚。

            ## 2. 架构拆分
            - **Edge Worker 层**：负责基于 Cookie / 地理区域做 10% → 50% → 100% 金丝雀分流。
            - **Next.js 16 App Router**：关键 Hero 区采用流式 SSR + 边缘静态缓存（ISR 60s）。
            - **Web Vitals 监控**：实时上报 LCP、INP、CLS 至 Prometheus。

            ## 3. 回滚预案
            - 任意时刻若 LCP P95 > 1.8s 或 5xx > 0.2%，Edge 开关 5 秒内自动切回旧版静态资源包。
            """,
            """
            # Homepage Redesign Implementation Plan

            ## 1. Goals & Core Metrics
            - Reduce desktop & mobile homepage **LCP** from 2.6s to **< 1.2s**.
            - Keep critical above-the-fold JS under 95KB gzipped with zero-downtime canary routing.

            ## 2. Architecture Breakdown
            - **Edge Worker Layer**: Deterministic 10% -> 50% -> 100% canary traffic splitting by cookie/region.
            - **Next.js 16 App Router**: Streaming SSR for the Hero section paired with 60s edge ISR cache.
            - **Web Vitals Telemetry**: Export real-time LCP, INP, and CLS metrics to Prometheus.

            ## 3. Rollback Plan
            - If LCP P95 exceeds 1.8s or 5xx > 0.2%, the edge flag reverts to the static bundle within 5 seconds.
            """,
            """
            # 公式サイトトップ刷新 実装計画書

            ## 1. 目標と主要指標
            - PC・モバイル双方のトップページ **LCP** を 2.6s から **1.2s 未満** に短縮。
            - ファーストビューの主要 JS を gzip 95KB 以下に抑え、無停止カナリア切替と即時ロールバックに対応。

            ## 2. アーキテクチャ分割
            - **Edge Worker 層**：Cookie・リージョン別に 10% → 50% → 100% のカナリア配信を制御。
            - **Next.js 16 App Router**：Hero 領域にストリーミング SSR と 60 秒 Edge ISR キャッシュを併用。
            - **Web Vitals 計測**：LCP / INP / CLS を Prometheus へリアルタイム送信。

            ## 3. ロールバック手順
            - LCP P95 > 1.8s または 5xx > 0.2% を検知した場合、Edge フラグで 5 秒以内に旧静的バンドルへ自動復帰。
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
