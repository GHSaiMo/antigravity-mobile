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
        let csvPath = seedFile(name: "销售数据.csv", text: "月份,销售额(万元),环比\n7月,128,+3.2%\n8月,141,+10.2%\n9月,156,+10.6%\n")
        let mdPath = seedFile(name: "会议纪要.md", text: "# 周会纪要\n\n## 结论\n\n- 9 月销售额 **156 万元**，环比增长 10.6%\n- 华东区贡献最高，华南区需要补强\n\n## 待办\n\n1. 整理季度报告初稿\n2. 与市场部确认十月活动预算\n")
        
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
            toolMsg(2 - 1, count: 3, names: ["读取文件", "分析数据", "生成图表"]),
            agentMsg(2, """
            ## 三季度销售概览

            | 月份 | 销售额（万元） | 环比 |
            | --- | --- | --- |
            | 7 月 | 128 | +3.2% |
            | 8 月 | 141 | +10.2% |
            | 9 月 | 156 | **+10.6%** |

            ![三季度销售趋势](data:image/png;base64,\(DemoAssets.salesChartPNG().base64EncodedString()))

            **结论**

            - 季度累计 **425 万元**，三个月持续增长
            - 华东区贡献最高，华南区增速偏低

            **建议**

            1. 把华东区的打法复制到华南
            2. 十月活动预算尽快确认，避免错过旺季

            完整报告已生成：[三季度销售报告.pdf](file://\(pdfPath))
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
    
    static let planMarkdown = """
    # 官网首页改版方案

    ## 目标

    把首页访客到「申请试用」的转化率从 **2.1%** 提升到 **3.5%**。

    ## 改动清单

    | 区块 | 现状 | 改动 |
    | --- | --- | --- |
    | 首屏 | 三个并列按钮 | 保留一个主按钮「免费试用」 |
    | 客户案例 | 位于页脚前 | 上移到第二屏 |
    | 价格 | 纯文字列表 | 三档对比卡片 |

    ## 实施步骤

    1. 抽出 `Hero`、`CaseStudies`、`Pricing` 三个组件
    2. 调整首页路由的区块顺序
    3. 接入埋点，对比改版前后转化率

    ## 风险

    - 价格卡片在窄屏需要横向滚动，需要补充小屏样式
    """
}

/// Programmatically drawn images and documents for the demo content.
enum DemoAssets {
    static func salesChartPNG() -> Data {
        let size = CGSize(width: 600, height: 346)
        let img = UIGraphicsImageRenderer(size: size).image { ctx in
            UIColor.white.setFill(); ctx.fill(CGRect(origin: .zero, size: size))
            ("三季度销售额（万元）" as NSString).draw(at: CGPoint(x: 28, y: 18), withAttributes: [.font: UIFont.boldSystemFont(ofSize: 20), .foregroundColor: UIColor.black])
            let data: [(String, CGFloat)] = [("7月", 128), ("8月", 141), ("9月", 156)]
            let baseY: CGFloat = 300, maxH: CGFloat = 210, barW: CGFloat = 100
            for (i, d) in data.enumerated() {
                let h = maxH * d.1 / 160
                let x = 100 + CGFloat(i) * 153
                UIColor(red: 0.35, green: 0.34, blue: 0.84, alpha: 1).setFill()
                UIBezierPath(roundedRect: CGRect(x: x, y: baseY - h, width: barW, height: h), cornerRadius: 8).fill()
                ("\(Int(d.1))" as NSString).draw(at: CGPoint(x: x + 32, y: baseY - h - 26), withAttributes: [.font: UIFont.boldSystemFont(ofSize: 18), .foregroundColor: UIColor.darkGray])
                (d.0 as NSString).draw(at: CGPoint(x: x + 34, y: baseY + 8), withAttributes: [.font: UIFont.systemFont(ofSize: 16), .foregroundColor: UIColor.gray])
            }
            UIColor.lightGray.setStroke()
            let axis = UIBezierPath(); axis.move(to: CGPoint(x: 66, y: baseY)); axis.addLine(to: CGPoint(x: 570, y: baseY)); axis.lineWidth = 1.5; axis.stroke()
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
    
    static func reportPDF() -> Data {
        let bounds = CGRect(x: 0, y: 0, width: 595, height: 842)
        return UIGraphicsPDFRenderer(bounds: bounds).pdfData { ctx in
            func page(_ heading: String, _ lines: [String]) {
                ctx.beginPage()
                (heading as NSString).draw(at: CGPoint(x: 50, y: 60), withAttributes: [.font: UIFont.boldSystemFont(ofSize: 26)])
                var y: CGFloat = 120
                for l in lines {
                    (l as NSString).draw(in: CGRect(x: 50, y: y, width: 495, height: 60), withAttributes: [.font: UIFont.systemFont(ofSize: 15), .foregroundColor: UIColor.darkGray])
                    y += 34
                }
            }
            page("三季度销售报告", ["7 月：128 万元（环比 +3.2%）", "8 月：141 万元（环比 +10.2%）", "9 月：156 万元（环比 +10.6%）", "", "季度累计 425 万元，三个月持续增长。"])
            page("结论与建议", ["1. 华东区贡献最高，可总结打法并复制到华南区。", "2. 十月活动预算需尽快确认，避免错过旺季。", "", "（本文档由演示模式生成，仅用于展示。）"])
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
