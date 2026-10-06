import Foundation
import UIKit

/// Seed conversations and generated assets (images, PDF, plan) for demo mode.
///
/// The list is designed to show every kind of status the real app can show (running, error,
/// waiting for the user, unread, plan ready to proceed) and every kind of content the agent
/// deals with (images, documents, code, plain Q&A).
extension DemoGateway {
    static let docsPath = "/Users/demo/Documents"
    static let planPath = "/Users/demo/.gemini/antigravity/brain/demo-0005/implementation_plan.md"
    
    func seedBinary(path: String, data: Data) {
        let name = (path as NSString).lastPathComponent
        files[path] = StoredFile(id: Self.fileID(for: data), name: name, data: data, path: path)
    }
    
    func seedAll(now: Date) {
        let csvPath = seedFile(name: "销售数据.csv", text: DemoAssets.salesCSV)
        let mdPath = seedFile(name: "会议纪要.md", text: Self.meetingNotes)
        
        let chartPath = "\(Self.docsPath)/三季度销售趋势.png"
        seedBinary(path: chartPath, data: DemoAssets.salesChartPNG())
        let pdfPath = "\(Self.docsPath)/三季度销售报告.pdf"
        seedBinary(path: pdfPath, data: DemoAssets.reportPDF())
        seedBinary(path: Self.planPath, data: Data(Self.planMarkdown.utf8))
        
        let invoiceA = DemoAssets.invoiceJPEG(title: "增值税电子普通发票", shop: "星河咖啡（国贸店）", items: [("拿铁 ×2", "56.00"), ("可颂 ×1", "18.00")], total: "74.00", no: "No.2410 0583 7712")
        let invoiceB = DemoAssets.invoiceJPEG(title: "出租汽车发票", shop: "北京出租 · 京B·K8392", items: [("起步价", "14.00"), ("里程 12.6km", "31.50")], total: "45.50", no: "No.0092 4471 0310")
        
        // 1 ─ RUNNING: running task card + queued follow-up
        var c1 = Conversation(id: "demo-0001-upload", title: "实现文件附件上传", project: projects[1],
                              createdAt: now.addingTimeInterval(-1800), lastModified: now.addingTimeInterval(-10), messages: [])
        c1.messages = [
            userMsg(0, "给聊天输入框加上文件附件功能：先上传到网关，发送时只带附件 id。", fileLines: []),
            toolMsg(1, count: 4, names: ["搜索代码", "读取文件", "编辑文件", "运行命令"]),
            agentMsg(2, """
            我先把上传逻辑封装成一个独立的方法，核心部分如下：

            ```swift
            func uploadAttachment(fileURL: URL, name: String) async throws -> UploadedAttachment {
                var request = URLRequest(url: baseURL.appendingPathComponent("api/v1/attachments"))
                request.httpMethod = "POST"
                request.setValue(name.addingPercentEncoding(withAllowedCharacters: .urlPathAllowed),
                                 forHTTPHeaderField: "X-File-Name")
                let (data, _) = try await session.upload(for: request, fromFile: fileURL)
                return try JSONDecoder().decode(UploadedAttachment.self, from: data)
            }
            ```

            现在正在构建并运行测试，完成后会继续接入输入框的文件 chip。
            """)
        ]
        c1.forcedStatus = "CASCADE_RUN_STATUS_RUNNING"
        c1.runningTasks = [[
            "id": "task-demo-1", "stepIndex": 3, "toolName": "run_command",
            "commandLine": "xcodebuild -scheme Multigravity build", "toolSummary": "构建 iOS 工程",
            "toolAction": "Running", "startedAt": iso(now.addingTimeInterval(-25))
        ]]
        c1.queued = [[
            "id": "queue-demo-1", "text": "构建通过后，顺便补上附件校验的单元测试",
            "createdAt": iso(now.addingTimeInterval(-8))
        ]]
        conversations[c1.id] = c1
        
        // 2 ─ ERROR: shows the error card and the Continue button
        var c2 = Conversation(id: "demo-0002-login-500", title: "登录接口 500 错误排查", project: projects[2],
                              createdAt: now.addingTimeInterval(-900), lastModified: now.addingTimeInterval(-90), messages: [])
        c2.messages = [
            userMsg(0, "线上登录接口偶发 500，帮我看看日志和代码。", fileLines: []),
            toolMsg(1, count: 3, names: ["读取日志", "搜索代码", "读取文件"]),
            ["id": "step-2", "type": "error", "role": "error",
             "text": "Agent execution terminated due to error.\n模型服务暂时不可用，请点击 Continue 重试。",
             "content": "Agent execution terminated due to error.\n模型服务暂时不可用，请点击 Continue 重试。", "stepIndex": 2]
        ]
        c2.hasError = true
        c2.errorMessage = "模型服务暂时不可用，请点击 Continue 重试。"
        conversations[c2.id] = c2
        
        // 3 ─ ACTION: waiting for the user to approve a command
        var c3 = Conversation(id: "demo-0003-migration", title: "数据库迁移方案评审", project: projects[2],
                              createdAt: now.addingTimeInterval(-1500), lastModified: now.addingTimeInterval(-240), messages: [])
        c3.messages = [
            userMsg(0, "给 orders 表的 user_id 加索引，写好迁移脚本并在测试库跑一下。", fileLines: []),
            toolMsg(1, count: 2, names: ["编辑文件", "读取文件"]),
            agentMsg(2, """
            迁移脚本已写好：

            ```sql
            -- migrations/0042_add_orders_user_id_index.sql
            CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_orders_user_id
                ON orders (user_id);
            ```

            执行前需要你确认几个选项。
            """)
        ]
        c3.forcedStatus = "CASCADE_RUN_STATUS_RUNNING"
        c3.needsInput = true
        // Multi-question prompt: all questions are shown at once and answered together.
        func question(_ text: String, multi: Bool, _ options: [String]) -> [String: Any] {
            [
                "question": text, "isMultiSelect": multi,
                "options": options.enumerated().map { ["id": "\($0.offset + 1)", "text": $0.element] as [String: Any] },
                "defaultOptionId": "1", "hasWriteIn": true,
                "writeInLabel": "Other", "writeInPlaceholder": "(write in your response)"
            ]
        }
        let questions: [[String: Any]] = [
            question("索引创建方式？", multi: false, ["CREATE INDEX CONCURRENTLY（不锁表，推荐）", "普通 CREATE INDEX（会短暂锁表）"]),
            question("在哪些环境执行？", multi: true, ["测试库", "预发库", "生产库（需在审批窗口内）"]),
            question("执行后是否做回滚验证？", multi: false, ["是，执行后验证并保留回滚脚本", "否，仅执行迁移"])
        ]
        c3.pendingInteraction = [
            "type": "ask_question", "trajectoryId": c3.id, "stepIndex": 3,
            "title": "索引创建方式？",
            "options": questions[0]["options"] as Any,
            "isMultiSelect": false, "defaultOptionId": "1", "hasWriteIn": true,
            "writeInLabel": "Other", "writeInPlaceholder": "(write in your response)",
            "questions": questions
        ]
        conversations[c3.id] = c3
        
        // 4 ─ UNREAD: documents in, chart + PDF out
        var c4 = Conversation(id: "demo-0004-sales", title: "整理季度销售报告", project: projects[0],
                              createdAt: now.addingTimeInterval(-3600), lastModified: now.addingTimeInterval(-720), messages: [])
        c4.messages = [
            userMsg(0, "帮我根据附件整理一下三季度的销售情况，给出结论和建议，并生成一份 PDF 报告。", fileLines: [
                fileLine(path: csvPath, name: "销售数据.csv", size: fileSize(csvPath)),
                fileLine(path: mdPath, name: "会议纪要.md", size: fileSize(mdPath))
            ]),
            toolMsg(1, count: 5, names: ["读取文件", "分析数据", "生成图表", "排版 PDF", "校对数字"]),
            agentMsg(2, """
            ## 三季度销售概览

            | 月份 | 销售额（万元） | 环比 |
            | --- | --- | --- |
            | 7 月 | 128 | +3.2% |
            | 8 月 | 141 | +10.2% |
            | 9 月 | 156 | **+10.6%** |

            季度累计 **425 万元**，完成季度目标（400 万元）的 **106.3%**。

            ![三季度销售趋势](data:image/png;base64,\(DemoAssets.salesChartPNG().base64EncodedString()))

            ## 分区域表现

            | 区域 | 季度合计 | 占比 | 9 月环比 |
            | --- | --- | --- | --- |
            | 华东 | 175 | 41.2% | +12.1% |
            | 华北 | 118 | 27.8% | +10.3% |
            | 华南 | 75 | 17.6% | +4.0% |
            | 西部 | 57 | 13.4% | **+15.8%** |

            **结论**

            - 三个月持续增长，9 月单月 **156 万元**，创季度新高
            - 华东贡献最高（41.2%），西部增速最快（+15.8%）
            - 华南环比仅 +4.0%，是唯一低于整体增速一半的区域

            **建议**

            1. 把华东区的打法复制到华南，先在深圳、广州各选 3 家门店试点
            2. 西部增长势头好，建议十月追加 10% 的渠道预算
            3. 十月活动预算尽快确认，避免错过旺季

            完整图文报告（共 4 页）已生成：[三季度销售报告.pdf](file://\(pdfPath))
            """)
        ]
        c4.unread = true
        conversations[c4.id] = c4
        
        // 5 ─ Plan ready: Proceed button + plan document
        var c5 = Conversation(id: "demo-0005-redesign", title: "官网首页改版方案", project: projects[1],
                              createdAt: now.addingTimeInterval(-3000), lastModified: now.addingTimeInterval(-1500), messages: [])
        c5.messages = [
            userMsg(0, "官网首页转化率偏低，帮我出一份改版方案，先别动代码。", fileLines: []),
            toolMsg(1, count: 3, names: ["读取文件", "搜索代码", "编写文档"]),
            agentMsg(2, """
            我梳理了现有首页的结构，并写好了改版方案：[implementation_plan.md](file://\(Self.planPath))

            核心改动有三点：

            - 首屏只保留一个主行动按钮
            - 把客户案例上移到第二屏
            - 价格区块改为对比卡片

            方案确认后点击 **Proceed** 开始实施。
            """)
        ]
        c5.canProceed = true
        c5.proceedUri = "file://\(Self.planPath)"
        conversations[c5.id] = c5
        
        // 6 ─ Images in, table out
        var c6 = Conversation(id: "demo-0006-invoice", title: "发票识别与整理", project: projects[0],
                              createdAt: now.addingTimeInterval(-7200), lastModified: now.addingTimeInterval(-3600), messages: [])
        c6.messages = [
            userMsg(0, "这是本周两张报销发票，帮我整理成表格，并算一下合计。", fileLines: [], media: [invoiceA, invoiceB]),
            toolMsg(1, count: 1, names: ["识别图片"]),
            agentMsg(2, """
            已识别两张发票：

            | 类型 | 商户 | 金额（元） |
            | --- | --- | --- |
            | 餐饮 | 星河咖啡（国贸店） | 74.00 |
            | 交通 | 北京出租 · 京B·K8392 | 45.50 |
            | **合计** | | **119.50** |

            发票号码分别为 `2410 0583 7712` 和 `0092 4471 0310`，需要我生成报销单的 Excel 吗？
            """)
        ]
        conversations[c6.id] = c6
        
        // 7 ─ Code: several languages + a diff
        var c7 = Conversation(id: "demo-0007-python", title: "Python 脚本性能优化", project: projects[2],
                              createdAt: now.addingTimeInterval(-10800), lastModified: now.addingTimeInterval(-7200), messages: [])
        c7.messages = [
            userMsg(0, "这个统计脚本处理 100 万行要 40 秒，怎么优化？", fileLines: []),
            toolMsg(1, count: 2, names: ["读取文件", "运行命令"]),
            agentMsg(2, """
            瓶颈在于逐行 `append` 和重复的字符串拼接。改成向量化处理即可：

            ```diff
            - rows = []
            - for line in open(path):
            -     rows.append(parse(line))
            - df = pd.DataFrame(rows)
            + df = pd.read_csv(path, usecols=["user_id", "amount"], dtype={"user_id": "int32"})
            ```

            聚合部分也可以直接下推到数据库：

            ```sql
            SELECT user_id, SUM(amount) AS total
            FROM orders
            WHERE created_at >= date_trunc('month', now())
            GROUP BY user_id
            ORDER BY total DESC
            LIMIT 20;
            ```

            实测 100 万行从 **40s 降到 1.8s**：

            ```bash
            $ python stats.py orders.csv
            loaded 1,000,000 rows in 0.9s
            top20 computed in 0.9s
            ```
            """)
        ]
        conversations[c7.id] = c7
        
        // 8 ─ CSS fix
        var c8 = Conversation(id: "demo-0008-login-style", title: "修复登录页样式问题", project: projects[1],
                              createdAt: now.addingTimeInterval(-14400), lastModified: now.addingTimeInterval(-10800), messages: [])
        c8.messages = [
            userMsg(0, "登录按钮在小屏幕上被挤出了容器，帮我看看。", fileLines: []),
            toolMsg(1, count: 3, names: ["搜索代码", "读取文件", "编辑文件"]),
            agentMsg(2, """
            已定位问题：`.login-card` 使用了固定宽度，小屏会溢出。

            ```css
            .login-card {
              width: min(420px, 100% - 32px);
              margin-inline: auto;
            }
            ```

            修改后在 320px 宽度下按钮完整显示。
            """)
        ]
        conversations[c8.id] = c8
        
        // 9 ─ Plain Q&A (no project)
        var c9 = Conversation(id: "demo-0009-chat", title: "杭州三日游行程", project: nil,
                              createdAt: now.addingTimeInterval(-86400), lastModified: now.addingTimeInterval(-43200), messages: [])
        c9.messages = [
            userMsg(0, "十月想去杭州玩三天，帮我排个轻松的行程。", fileLines: []),
            agentMsg(1, """
            **第一天**：西湖慢走（断桥 → 白堤 → 平湖秋月），傍晚去河坊街

            **第二天**：灵隐寺 + 法喜寺，下午喝茶放空

            **第三天**：龙井村采茶体验，返程前逛一逛西溪湿地

            需要我按预算再细化吗？
            """)
        ]
        conversations[c9.id] = c9
    }
    
    static let meetingNotes = """
    # 周会纪要

    **时间**：10 月 8 日 10:00　**参会**：销售、市场、数据组

    ## 结论

    - 9 月销售额 **156 万元**，环比增长 10.6%，Q3 累计 **425 万元**，超额完成目标（400 万元）
    - 华东区贡献最高（175 万元，占 41.2%），华南区环比仅 +4.0%，需要补强
    - 西部区增速最快（+15.8%），渠道铺设初见成效

    ## 待办

    1. 整理季度报告初稿（销售数据组，周五前）
    2. 与市场部确认十月活动预算（市场部，下周二前）
    3. 华南区门店走访与复盘（华南负责人，两周内）
    """
    
    static let planMarkdown = """
    # 官网首页改版方案

    ## 目标

    把首页访客到「申请试用」的转化率从 **2.1%** 提升到 **3.5%**，并把首屏加载时间控制在 **1.5 秒** 以内。

    ## 现状诊断

    | 指标 | 现状 | 行业参考 |
    | --- | --- | --- |
    | 首屏跳出率 | 62% | 45% |
    | 试用按钮点击率 | 4.8% | 9% |
    | 首屏 LCP | 3.2s | 1.5s |

    ## 改动清单

    | 区块 | 现状 | 改动 |
    | --- | --- | --- |
    | 首屏 | 三个并列按钮 | 保留一个主按钮「免费试用」 |
    | 客户案例 | 位于页脚前 | 上移到第二屏 |
    | 价格 | 纯文字列表 | 三档对比卡片 |
    | 图片 | 原图直出 | WebP + 懒加载 |

    ## 页面结构

    ```mermaid
    graph LR
        A[首屏 Hero] --> B[客户案例]
        B --> C[核心功能]
        C --> D[价格对比]
        D --> E[常见问题]
        E --> F[试用 CTA]
    ```

    ## 实施步骤

    1. 抽出 `Hero`、`CaseStudies`、`Pricing` 三个组件
    2. 调整首页路由的区块顺序
    3. 图片转 WebP 并接入懒加载
    4. 接入埋点（`home_cta_click`），对比改版前后转化率

    ## 时间安排

    - [ ] 第 1 天：组件拆分与区块重排
    - [ ] 第 2 天：价格卡片与小屏适配
    - [ ] 第 3 天：埋点、性能优化与灰度发布

    ## 风险

    - 价格卡片在窄屏需要横向滚动，需要补充小屏样式
    - 案例上移后首屏高度增加，需确认 CLS 不回退
    """
}

/// Programmatically drawn images and documents for the demo content.
enum DemoAssets {
    // MARK: - Shared sales data (万元). Months 7/8/9 × regions.
    static let regionNames = ["华东", "华北", "华南", "西部"]
    static let monthNames = ["7月", "8月", "9月"]
    static let regionSales: [[Int]] = [   // [region][month]
        [52, 58, 65],
        [36, 39, 43],
        [24, 25, 26],
        [16, 19, 22]
    ]
    static let regionColors: [UIColor] = [
        UIColor(red: 0.35, green: 0.34, blue: 0.84, alpha: 1),
        UIColor(red: 0.04, green: 0.52, blue: 1.00, alpha: 1),
        UIColor(red: 1.00, green: 0.62, blue: 0.04, alpha: 1),
        UIColor(red: 0.19, green: 0.82, blue: 0.35, alpha: 1)
    ]
    static let accent = UIColor(red: 0.35, green: 0.34, blue: 0.84, alpha: 1)
    
    static var salesCSV: String {
        var out = "月份,区域,销售额(万元)\n"
        for (m, month) in monthNames.enumerated() {
            for (r, region) in regionNames.enumerated() { out += "\(month),\(region),\(regionSales[r][m])\n" }
        }
        return out
    }
    
    private static func monthTotal(_ m: Int) -> Int { regionSales.reduce(0) { $0 + $1[m] } }
    
    /// Stacked bar chart of monthly sales split by region (also drawn into the PDF).
    private static func drawSalesChart(in rect: CGRect, title: String?) {
        var top = rect.minY
        if let title {
            (title as NSString).draw(at: CGPoint(x: rect.minX, y: top), withAttributes: [.font: UIFont.boldSystemFont(ofSize: rect.width > 500 ? 20 : 13), .foregroundColor: UIColor.black])
            top += rect.width > 500 ? 36 : 24
        }
        let small = rect.width <= 500
        let labelFont = UIFont.systemFont(ofSize: small ? 9 : 14)
        // legend
        var lx = rect.minX
        for (i, name) in regionNames.enumerated() {
            regionColors[i].setFill()
            UIBezierPath(roundedRect: CGRect(x: lx, y: top + 2, width: small ? 8 : 12, height: small ? 8 : 12), cornerRadius: 2).fill()
            (name as NSString).draw(at: CGPoint(x: lx + (small ? 12 : 18), y: top - 1), withAttributes: [.font: labelFont, .foregroundColor: UIColor.darkGray])
            lx += small ? 44 : 70
        }
        top += small ? 22 : 34
        let baseY = rect.maxY - (small ? 20 : 30)
        let plotH = baseY - top - (small ? 14 : 22)
        let axisL = rect.minX + (small ? 26 : 40)
        // gridlines
        for tick in [0, 50, 100, 150] {
            let y = baseY - plotH * CGFloat(tick) / 160
            UIColor(white: 0.9, alpha: 1).setStroke()
            let g = UIBezierPath(); g.move(to: CGPoint(x: axisL, y: y)); g.addLine(to: CGPoint(x: rect.maxX, y: y)); g.lineWidth = 0.8; g.stroke()
            ("\(tick)" as NSString).draw(at: CGPoint(x: rect.minX, y: y - (small ? 5 : 8)), withAttributes: [.font: labelFont, .foregroundColor: UIColor.gray])
        }
        let slot = (rect.maxX - axisL) / 3
        let barW = slot * 0.5
        for m in 0..<3 {
            let x = axisL + slot * CGFloat(m) + (slot - barW) / 2
            var y = baseY
            for r in (0..<4).reversed() {
                let h = plotH * CGFloat(regionSales[r][m]) / 160
                y -= h
                regionColors[r].setFill()
                UIBezierPath(rect: CGRect(x: x, y: y, width: barW, height: h)).fill()
            }
            let total = "\(monthTotal(m))"
            (total as NSString).draw(at: CGPoint(x: x + barW / 2 - CGFloat(total.count) * (small ? 3 : 5), y: y - (small ? 14 : 22)), withAttributes: [.font: UIFont.boldSystemFont(ofSize: small ? 10 : 16), .foregroundColor: UIColor.darkGray])
            (monthNames[m] as NSString).draw(at: CGPoint(x: x + barW / 2 - (small ? 8 : 12), y: baseY + (small ? 4 : 7)), withAttributes: [.font: labelFont, .foregroundColor: UIColor.gray])
        }
    }
    
    static func salesChartPNG() -> Data {
        let size = CGSize(width: 640, height: 400)
        let img = UIGraphicsImageRenderer(size: size).image { _ in
            UIColor.white.setFill(); UIRectFill(CGRect(origin: .zero, size: size))
            drawSalesChart(in: CGRect(x: 28, y: 20, width: 590, height: 360), title: "三季度销售额（万元）· 分区域")
        }
        return img.pngData() ?? Data()
    }
    
    /// Returns a base64 JPEG of a receipt-like image (used as a user-sent photo).
    static func invoiceJPEG(title: String, shop: String, items: [(String, String)], total: String, no: String) -> String {
        let size = CGSize(width: 600, height: 760)
        let img = UIGraphicsImageRenderer(size: size).image { ctx in
            UIColor(white: 0.96, alpha: 1).setFill(); ctx.fill(CGRect(origin: .zero, size: size))
            UIColor.white.setFill(); UIBezierPath(roundedRect: CGRect(x: 30, y: 30, width: 540, height: 700), cornerRadius: 12).fill()
            func draw(_ t: String, _ x: CGFloat, _ y: CGFloat, _ font: UIFont, _ color: UIColor = .black) {
                (t as NSString).draw(at: CGPoint(x: x, y: y), withAttributes: [.font: font, .foregroundColor: color])
            }
            draw(title, 70, 70, .boldSystemFont(ofSize: 32))
            draw(no, 70, 120, .monospacedSystemFont(ofSize: 20, weight: .regular), .gray)
            draw(shop, 70, 190, .systemFont(ofSize: 28))
            var y: CGFloat = 270
            for (name, price) in items {
                draw(name, 70, y, .systemFont(ofSize: 26)); draw("¥ " + price, 400, y, .systemFont(ofSize: 26)); y += 56
            }
            UIColor.lightGray.setStroke(); let line = UIBezierPath(); line.move(to: CGPoint(x: 70, y: y + 10)); line.addLine(to: CGPoint(x: 530, y: y + 10)); line.stroke()
            draw("合计", 70, y + 40, .boldSystemFont(ofSize: 30)); draw("¥ " + total, 380, y + 40, .boldSystemFont(ofSize: 34), UIColor(red: 0.85, green: 0.15, blue: 0.15, alpha: 1))
            draw("发票专用章", 360, 600, .boldSystemFont(ofSize: 26), UIColor.red.withAlphaComponent(0.6))
        }
        return (img.jpegData(compressionQuality: 0.8) ?? Data()).base64EncodedString()
    }
    
    /// 4-page illustrated report: cover + KPIs, monthly trend, regional analysis, conclusions & next steps.
    static func reportPDF() -> Data {
        let bounds = CGRect(x: 0, y: 0, width: 595, height: 842)
        let ink = UIColor(white: 0.1, alpha: 1)
        let sub = UIColor(white: 0.38, alpha: 1)
        let line = UIColor(white: 0.88, alpha: 1)
        let tint = UIColor(red: 0.95, green: 0.95, blue: 0.99, alpha: 1)
        
        func text(_ s: String, _ x: CGFloat, _ y: CGFloat, _ size: CGFloat, bold: Bool = false, color: UIColor? = nil, width: CGFloat = 495) {
            let font = bold ? UIFont.boldSystemFont(ofSize: size) : UIFont.systemFont(ofSize: size)
            (s as NSString).draw(in: CGRect(x: x, y: y, width: width, height: size * 3.2), withAttributes: [.font: font, .foregroundColor: color ?? (bold ? ink : sub)])
        }
        func paragraph(_ s: String, _ x: CGFloat, _ y: CGFloat, width: CGFloat = 495, size: CGFloat = 12) -> CGFloat {
            let style = NSMutableParagraphStyle(); style.lineSpacing = 5
            let attr: [NSAttributedString.Key: Any] = [.font: UIFont.systemFont(ofSize: size), .foregroundColor: sub, .paragraphStyle: style]
            let r = (s as NSString).boundingRect(with: CGSize(width: width, height: 400), options: .usesLineFragmentOrigin, attributes: attr, context: nil)
            (s as NSString).draw(in: CGRect(x: x, y: y, width: width, height: r.height + 2), withAttributes: attr)
            return y + r.height + 10
        }
        func pageHeader(_ ctx: UIGraphicsPDFRendererContext, _ n: Int, _ title: String) {
            ctx.beginPage()
            accent.setFill(); UIRectFill(CGRect(x: 0, y: 0, width: 595, height: 6))
            text("三季度销售报告", 50, 28, 10, color: UIColor.gray)
            text("第 \(n) 页 / 共 4 页", 455, 28, 10, color: UIColor.gray, width: 90)
            text(title, 50, 62, 24, bold: true)
            accent.setFill(); UIBezierPath(roundedRect: CGRect(x: 50, y: 100, width: 36, height: 4), cornerRadius: 2).fill()
        }
        func table(_ rows: [[String]], x: CGFloat, y: CGFloat, widths: [CGFloat], rowH: CGFloat = 30, highlightLast: Bool = false) -> CGFloat {
            var yy = y
            for (i, row) in rows.enumerated() {
                if i == 0 { accent.setFill(); UIBezierPath(roundedRect: CGRect(x: x, y: yy, width: widths.reduce(0, +), height: rowH), cornerRadius: 6).fill() }
                else if i % 2 == 0 { tint.setFill(); UIRectFill(CGRect(x: x, y: yy, width: widths.reduce(0, +), height: rowH)) }
                var cx = x
                for (c, cell) in row.enumerated() {
                    let isLast = highlightLast && i == rows.count - 1
                    text(cell, cx + 12, yy + (rowH - 13) / 2, 12, bold: i == 0 || isLast, color: i == 0 ? .white : (isLast ? ink : sub), width: widths[c] - 16)
                    cx += widths[c]
                }
                yy += rowH
            }
            line.setStroke(); let b = UIBezierPath(); b.move(to: CGPoint(x: x, y: yy)); b.addLine(to: CGPoint(x: x + widths.reduce(0, +), y: yy)); b.stroke()
            return yy
        }
        
        return UIGraphicsPDFRenderer(bounds: bounds).pdfData { ctx in
            // ── Page 1: cover + KPIs
            ctx.beginPage()
            accent.setFill(); UIRectFill(CGRect(x: 0, y: 0, width: 595, height: 250))
            UIColor.white.withAlphaComponent(0.12).setFill()
            UIBezierPath(ovalIn: CGRect(x: 380, y: -80, width: 300, height: 300)).fill()
            UIBezierPath(ovalIn: CGRect(x: 450, y: 110, width: 180, height: 180)).fill()
            text("2026 年 Q3 · 销售分析项目", 50, 80, 13, color: UIColor.white.withAlphaComponent(0.8))
            text("三季度销售报告", 50, 108, 38, bold: true, color: .white)
            text("季度表现、区域结构与下一步行动", 50, 166, 15, color: UIColor.white.withAlphaComponent(0.85))
            text("由 Antigravity Agent 生成 · 2026-10-08", 50, 212, 11, color: UIColor.white.withAlphaComponent(0.7))
            
            let kpis: [(String, String, String, UIColor)] = [
                ("季度累计", "425", "万元", accent),
                ("目标完成率", "106.3", "%", regionColors[3]),
                ("9 月环比", "+10.6", "%", regionColors[1]),
                ("增速最快", "西部", "+15.8%", regionColors[2])
            ]
            for (i, k) in kpis.enumerated() {
                let x = 50 + CGFloat(i % 2) * 255, y: CGFloat = 285 + CGFloat(i / 2) * 110
                tint.setFill(); UIBezierPath(roundedRect: CGRect(x: x, y: y, width: 240, height: 96), cornerRadius: 14).fill()
                k.3.setFill(); UIBezierPath(roundedRect: CGRect(x: x + 16, y: y + 18, width: 4, height: 60), cornerRadius: 2).fill()
                text(k.0, x + 30, y + 16, 12)
                text(k.1, x + 30, y + 38, 34, bold: true, color: ink, width: 150)
                text(k.2, x + 30 + CGFloat(k.1.count) * (k.1.contains("西") ? 34 : 19) + 4, y + 56, 14, color: sub, width: 80)
            }
            text("摘要", 50, 520, 16, bold: true)
            _ = paragraph("三季度累计销售额 425 万元，超出季度目标（400 万元）6.3%，三个月连续增长，9 月单月 156 万元创季度新高。华东区贡献 41.2%，保持领先；西部区环比增速达 15.8%，渠道铺设初见成效；华南区环比仅 +4.0%，是主要短板。", 50, 546)
            text("本报告共 4 页：月度趋势 · 区域分析 · 结论与下一步", 50, 780, 10, color: UIColor.gray)
            
            // ── Page 2: monthly trend
            pageHeader(ctx, 2, "月度趋势")
            var y = paragraph("下图展示 7–9 月销售额及其区域构成。9 月在华东（+12.1%）与西部（+15.8%）的带动下，环比增长 10.6%。", 50, 124)
            drawSalesChart(in: CGRect(x: 50, y: y + 6, width: 495, height: 280), title: nil)
            y += 304
            text("月度明细", 50, y, 15, bold: true)
            y = table([
                ["月份", "销售额（万元）", "环比", "占季度"],
                ["7 月", "128", "+3.2%", "30.1%"],
                ["8 月", "141", "+10.2%", "33.2%"],
                ["9 月", "156", "+10.6%", "36.7%"],
                ["合计", "425", "—", "100%"]
            ], x: 50, y: y + 28, widths: [110, 150, 120, 115], highlightLast: true)
            _ = paragraph("注：环比为相邻月份增长率；7 月环比相对 6 月（124 万元）。", 50, y + 14, size: 10)
            
            // ── Page 3: regional analysis
            pageHeader(ctx, 3, "区域分析")
            y = paragraph("按区域拆分后，华东仍是最大的贡献来源；华南是三个月里唯一增速持续低于整体的区域。", 50, 124)
            y = table([
                ["区域", "7 月", "8 月", "9 月", "合计", "占比"],
                ["华东", "52", "58", "65", "175", "41.2%"],
                ["华北", "36", "39", "43", "118", "27.8%"],
                ["华南", "24", "25", "26", "75", "17.6%"],
                ["西部", "16", "19", "22", "57", "13.4%"],
                ["合计", "128", "141", "156", "425", "100%"]
            ], x: 50, y: y + 8, widths: [95, 75, 75, 75, 85, 90], highlightLast: true)
            text("区域占比", 50, y + 28, 15, bold: true)
            var by = y + 62
            let shares: [Double] = [41.2, 27.8, 17.6, 13.4]
            for (i, name) in regionNames.enumerated() {
                text(name, 50, by, 12)
                line.setFill(); UIBezierPath(roundedRect: CGRect(x: 100, y: by + 2, width: 380, height: 12), cornerRadius: 6).fill()
                regionColors[i].setFill(); UIBezierPath(roundedRect: CGRect(x: 100, y: by + 2, width: 380 * CGFloat(shares[i] / 45), height: 12), cornerRadius: 6).fill()
                text(String(format: "%.1f%%", shares[i]), 492, by, 12, bold: true, color: ink, width: 60)
                by += 30
            }
            text("9 月环比增速", 50, by + 20, 15, bold: true)
            by += 54
            let growth: [Double] = [12.1, 10.3, 4.0, 15.8]
            for (i, name) in regionNames.enumerated() {
                text(name, 50, by, 12)
                line.setFill(); UIBezierPath(roundedRect: CGRect(x: 100, y: by + 2, width: 380, height: 12), cornerRadius: 6).fill()
                regionColors[i].setFill(); UIBezierPath(roundedRect: CGRect(x: 100, y: by + 2, width: 380 * CGFloat(growth[i] / 18), height: 12), cornerRadius: 6).fill()
                text(String(format: "+%.1f%%", growth[i]), 492, by, 12, bold: true, color: i == 2 ? UIColor.systemRed : ink, width: 60)
                by += 30
            }
            
            // ── Page 4: conclusions
            pageHeader(ctx, 4, "结论与下一步")
            y = 126
            let blocks: [(String, UIColor, [String])] = [
                ("核心结论", accent, ["三个月持续增长，季度累计 425 万元，完成目标的 106.3%", "华东贡献最高（41.2%），西部增速最快（+15.8%）", "华南环比仅 +4.0%，低于整体增速一半"]),
                ("行动建议", regionColors[3], ["把华东区的打法复制到华南，深圳、广州各选 3 家门店试点", "西部区十月追加 10% 渠道预算，巩固增长势头", "十月活动预算尽快确认，避免错过旺季"]),
                ("风险提示", regionColors[2], ["华东占比过高，单区域波动会明显影响整体", "十月活动期间库存与履约压力上升"])
            ]
            for blk in blocks {
                blk.1.setFill(); UIBezierPath(roundedRect: CGRect(x: 50, y: y + 3, width: 4, height: 16), cornerRadius: 2).fill()
                text(blk.0, 62, y, 15, bold: true)
                y += 30
                for item in blk.2 {
                    sub.setFill(); UIBezierPath(ovalIn: CGRect(x: 62, y: y + 6, width: 4, height: 4)).fill()
                    y = paragraph(item, 76, y, width: 460) + 2
                }
                y += 10
            }
            text("下一步", 50, y, 15, bold: true)
            y = table([
                ["事项", "负责人", "时间"],
                ["整理季度报告终稿", "销售数据组", "周五前"],
                ["确认十月活动预算", "市场部", "下周二前"],
                ["华南区门店走访复盘", "华南负责人", "两周内"]
            ], x: 50, y: y + 30, widths: [235, 140, 120])
            text("本文档由演示模式生成，数据均为虚构，仅用于展示。", 50, 790, 10, color: UIColor.gray)
        }
    }
}


// MARK: - Demo drafts (DRAFT tag in the conversation list)

extension DemoGateway {
    static let draftedConversationId = "demo-0007-python"
    
    /// Seeds an unsent follow-up typed into an existing conversation so the list shows the DRAFT tag.
    static func seedDrafts() {
        CacheManager.shared.saveDraft(key: draftedConversationId, text: "再对比一下 pandas 和 polars 的耗时，并给出内存占用")
    }
}
