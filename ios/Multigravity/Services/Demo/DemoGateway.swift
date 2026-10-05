import Foundation
import CryptoKit

/// In-app simulated gateway used by "演示模式" (demo mode).
///
/// Demo mode lets App Review (and anyone without a desktop gateway) explore the whole app: the
/// conversation list, chat, sending messages, attaching photos / files and previewing them. Requests
/// to `DemoGateway.baseURL` are answered locally by `DemoURLProtocol`; nothing leaves the device.
///
/// Replies are scripted: a sent message first shows the agent as running (with a tool step) and then
/// an agent reply appears. State lives in memory and is reset every time demo mode is entered.
final class DemoGateway: @unchecked Sendable {
    static let shared = DemoGateway()
    
    static let host = "127.0.0.1"
    static let port = 9
    static let baseURL = URL(string: "http://\(host):\(port)")!
    static let inboxPath = "/Users/demo/Multigravity/Inbox/2026-10"
    
    private static let flagLock = NSLock()
    nonisolated(unsafe) private static var enabledFlag = false
    /// Mirrors `AppSettings.isDemoMode`; readable from any thread (URLProtocol runs off the main actor).
    static var isEnabled: Bool {
        get { flagLock.lock(); defer { flagLock.unlock() }; return enabledFlag }
        set { flagLock.lock(); enabledFlag = newValue; flagLock.unlock() }
    }
    
    // MARK: - State
    
    struct Project {
        let id: String
        let name: String
        let uri: String
    }
    
    struct Conversation {
        let id: String
        var title: String
        var project: Project?
        let createdAt: Date
        var lastModified: Date
        var messages: [[String: Any]]
        var pending: Pending?
        var unread = false
    }
    
    struct Pending {
        let startedAt: Date
        let userText: String
        let fileNames: [String]
        let imageCount: Int
        var toolAdded = false
    }
    
    private struct StoredFile {
        let id: String
        let name: String
        let data: Data
        let path: String
    }
    
    private let lock = NSRecursiveLock()
    private var conversations: [String: Conversation] = [:]
    private var files: [String: StoredFile] = [:]      // keyed by path
    private var counter = 0
    
    /// Simulated Cockpit Tools quota accounts. Percentages are 0...100; reset offsets are seconds from "now".
    struct QuotaAccount {
        let id: String
        let email: String
        let name: String
        var claude5h: Double
        var claudeWeekly: Double
        var gemini5h: Double
        var geminiWeekly: Double
        let reset5h: TimeInterval
        let resetWeekly: TimeInterval
    }
    private var quotaAccounts: [QuotaAccount] = []
    private var currentQuotaId = ""
    private var quotaUpdatedAt: Int64 = 0
    
    let projects: [Project] = [
        Project(id: "demo-project-sales", name: "销售分析", uri: "file:///Users/demo/Projects/sales-analysis"),
        Project(id: "demo-project-web", name: "官网前端", uri: "file:///Users/demo/Projects/website")
    ]
    
    private init() { reset() }
    
    // MARK: - Lifecycle
    
    func reset() {
        lock.lock(); defer { lock.unlock() }
        conversations = [:]
        files = [:]
        counter = 0
        quotaAccounts = [
            QuotaAccount(id: "demo-acc-1", email: "alice@multigravity.app", name: "Alice（演示）",
                         claude5h: 100, claudeWeekly: 96, gemini5h: 88, geminiWeekly: 64,
                         reset5h: 4 * 3600 + 12 * 60, resetWeekly: 5 * 86400 + 2 * 3600),
            QuotaAccount(id: "demo-acc-2", email: "bob@multigravity.app", name: "Bob（演示）",
                         claude5h: 12, claudeWeekly: 41, gemini5h: 55, geminiWeekly: 23,
                         reset5h: 1 * 3600 + 5 * 60, resetWeekly: 2 * 86400 + 20 * 3600),
            QuotaAccount(id: "demo-acc-3", email: "carol@multigravity.app", name: "",
                         claude5h: 78, claudeWeekly: 9, gemini5h: 100, geminiWeekly: 100,
                         reset5h: 3 * 3600 + 40 * 60, resetWeekly: 6 * 86400 + 11 * 3600)
        ]
        currentQuotaId = "demo-acc-1"
        quotaUpdatedAt = Int64(Date().timeIntervalSince1970 * 1000)
        
        let now = Date()
        let csvPath = seedFile(name: "销售数据.csv", text: "月份,销售额(万元),环比\n7月,128,+3.2%\n8月,141,+10.2%\n9月,156,+10.6%\n")
        let mdPath = seedFile(name: "会议纪要.md", text: "# 周会纪要\n\n## 结论\n\n- 9 月销售额 **156 万元**，环比增长 10.6%\n- 华东区贡献最高，华南区需要补强\n\n## 待办\n\n1. 整理季度报告初稿\n2. 与市场部确认十月活动预算\n")
        
        var c1 = Conversation(
            id: "demo-0001-sales-report", title: "整理季度销售报告", project: projects[0],
            createdAt: now.addingTimeInterval(-3600), lastModified: now.addingTimeInterval(-600), messages: []
        )
        c1.messages = [
            userMsg(0, "帮我根据附件整理一下三季度的销售情况，给出结论和建议。", fileLines: [
                fileLine(path: csvPath, name: "销售数据.csv", size: fileSize(csvPath)),
                fileLine(path: mdPath, name: "会议纪要.md", size: fileSize(mdPath))
            ]),
            toolMsg(1, count: 2, names: ["读取文件", "分析数据"]),
            agentMsg(2, """
            ## 三季度销售概览

            | 月份 | 销售额（万元） | 环比 |
            | --- | --- | --- |
            | 7 月 | 128 | +3.2% |
            | 8 月 | 141 | +10.2% |
            | 9 月 | 156 | **+10.6%** |

            **结论**

            - 季度累计 **425 万元**，三个月持续增长
            - 华东区贡献最高，华南区增速偏低

            **建议**

            1. 把华东区的打法复制到华南
            2. 十月活动预算尽快确认，避免错过旺季

            > 这是演示模式生成的示例内容。
            """)
        ]
        conversations[c1.id] = c1
        
        var c2 = Conversation(
            id: "demo-0002-login-style", title: "修复登录页样式问题", project: projects[1],
            createdAt: now.addingTimeInterval(-7200), lastModified: now.addingTimeInterval(-1800), messages: []
        )
        c2.messages = [
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
        conversations[c2.id] = c2
        
        var c3 = Conversation(
            id: "demo-0003-chat", title: "旅行计划建议", project: nil,
            createdAt: now.addingTimeInterval(-86400), lastModified: now.addingTimeInterval(-43200), messages: []
        )
        c3.messages = [
            userMsg(0, "十月想去杭州玩三天，帮我排个轻松的行程。", fileLines: []),
            agentMsg(1, """
            **第一天**：西湖慢走（断桥 → 白堤 → 平湖秋月），傍晚去河坊街

            **第二天**：灵隐寺 + 法喜寺，下午喝茶放空

            **第三天**：龙井村采茶体验，返程前逛一逛西溪湿地

            需要我按预算再细化吗？
            """)
        ]
        conversations[c3.id] = c3
    }
    
    // MARK: - Message builders
    
    private func iso(_ date: Date) -> String {
        let f = ISO8601DateFormatter()
        f.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return f.string(from: date)
    }
    
    func userMsg(_ step: Int, _ text: String, fileLines: [String], media: [String] = []) -> [String: Any] {
        var full = text
        if !fileLines.isEmpty {
            full = (text.isEmpty ? "" : text + "\n\n") + AttachmentRules.blockHeader + "\n" + fileLines.joined(separator: "\n")
        }
        var m: [String: Any] = ["id": "step-\(step)", "type": "user", "role": "user", "text": full, "content": full, "stepIndex": step]
        if !media.isEmpty { m["media"] = media }
        return m
    }
    
    func agentMsg(_ step: Int, _ text: String) -> [String: Any] {
        ["id": "step-\(step)", "type": "agent", "role": "agent", "text": text, "content": text, "stepIndex": step]
    }
    
    func toolMsg(_ step: Int, count: Int, names: [String]) -> [String: Any] {
        ["id": "step-\(step)", "type": "tools", "role": "tools", "text": "已思考并执行 \(count) 项操作",
         "content": "已思考并执行 \(count) 项操作", "stepIndex": step, "toolCount": count, "toolNames": names]
    }
    
    // MARK: - Files
    
    private func seedFile(name: String, text: String) -> String {
        let data = Data(text.utf8)
        let id = Self.fileID(for: data)
        let path = "\(Self.inboxPath)/\(id)-\(name)"
        files[path] = StoredFile(id: id, name: name, data: data, path: path)
        return path
    }
    
    private func fileSize(_ path: String) -> Int64 { Int64(files[path]?.data.count ?? 0) }
    
    private func fileLine(path: String, name: String, size: Int64) -> String {
        let ext = (name as NSString).pathExtension.lowercased()
        return "- \(path) (\(ext), \(Self.humanSize(size)))"
    }
    
    static func fileID(for data: Data) -> String {
        SHA256.hash(data: data).map { String(format: "%02x", $0) }.joined().prefix(16).description
    }
    
    static func humanSize(_ n: Int64) -> String {
        if n >= 1 << 20 { return String(format: "%.1f MB", Double(n) / 1_048_576.0) }
        if n >= 1 << 10 { return String(format: "%.0f KB", Double(n) / 1024.0) }
        return "\(n) B"
    }
    
    /// Stores an uploaded attachment and returns the same JSON the real gateway returns.
    func storeUpload(name: String, data: Data) -> (status: Int, json: [String: Any]) {
        guard AttachmentRules.isAllowedName(name) else {
            return (415, ["error": "unsupported_type", "message": "不支持的文件类型: .\((name as NSString).pathExtension)"])
        }
        guard !data.isEmpty else { return (400, ["error": "empty_file", "message": "文件为空"]) }
        guard Int64(data.count) <= AttachmentRules.maxFileBytes else {
            return (413, ["error": "too_large", "message": "文件超过 50MB 上限"])
        }
        lock.lock(); defer { lock.unlock() }
        let sum = SHA256.hash(data: data).map { String(format: "%02x", $0) }.joined()
        let id = String(sum.prefix(16))
        let path = "\(Self.inboxPath)/\(id)-\(name)"
        files[path] = StoredFile(id: id, name: name, data: data, path: path)
        let line = fileLine(path: path, name: name, size: Int64(data.count))
        return (200, [
            "id": id, "path": path, "name": name, "size": data.count,
            "mime": Self.mime(for: name), "sha256": sum, "kind": "document", "line": line
        ])
    }
    
    func fileData(forPath path: String) -> (name: String, data: Data)? {
        lock.lock(); defer { lock.unlock() }
        var p = path
        if p.hasPrefix("file://") { p = String(p.dropFirst(7)) }
        p = p.removingPercentEncoding ?? p
        if let f = files[p] { return (f.name, f.data) }
        return nil
    }
    
    private func attachmentLines(ids: [String]) -> [String] {
        var lines: [String] = []
        for id in ids {
            if let f = files.values.first(where: { $0.id == id }) {
                lines.append(fileLine(path: f.path, name: f.name, size: Int64(f.data.count)))
            }
        }
        return lines
    }
    
    static func mime(for name: String) -> String {
        switch (name as NSString).pathExtension.lowercased() {
        case "md", "markdown": return "text/markdown; charset=utf-8"
        case "txt", "log", "csv", "json", "xml", "html", "yaml", "yml": return "text/plain; charset=utf-8"
        case "pdf": return "application/pdf"
        case "png": return "image/png"
        case "jpg", "jpeg": return "image/jpeg"
        default: return "application/octet-stream"
        }
    }
    
    // MARK: - Conversations
    
    func settle(_ id: String) {
        guard var conv = conversations[id], var p = conv.pending else { return }
        let elapsed = Date().timeIntervalSince(p.startedAt)
        if elapsed >= 1.2 && !p.toolAdded {
            p.toolAdded = true
            conv.messages.append(toolMsg(conv.messages.count, count: 2, names: ["读取文件", "思考"]))
            conv.pending = p
            conversations[id] = conv
        }
        if elapsed >= 3.0 {
            var reply: String
            if !p.fileNames.isEmpty {
                reply = "我已收到 \(p.fileNames.count) 个附件：\n\n" + p.fileNames.map { "- `\($0)`" }.joined(separator: "\n")
                    + "\n\n演示模式不会真正读取文件内容。连接到你电脑上的网关后，我会直接读取这些文件并按你的要求处理。"
            } else if p.imageCount > 0 {
                reply = "我收到了 \(p.imageCount) 张图片。演示模式不会分析图片内容；连接真实网关后即可使用。"
            } else {
                reply = "收到：「\(p.userText.prefix(60))」\n\n这是**演示模式**的模拟回复。连接到你电脑上的网关后，这里会是 Agent 的真实执行结果。"
            }
            conv.messages.append(agentMsg(conv.messages.count, reply))
            conv.pending = nil
            conv.lastModified = Date()
            conv.unread = false
            conversations[id] = conv
        }
    }
    
    func status(of conv: Conversation) -> String {
        conv.pending != nil ? "CASCADE_RUN_STATUS_RUNNING" : "CASCADE_RUN_STATUS_IDLE"
    }
    
    func trajectorySummaries() -> [String: Any] {
        lock.lock(); defer { lock.unlock() }
        var out: [String: Any] = [:]
        for id in conversations.keys { settle(id) }
        for (id, c) in conversations {
            var workspaces: [[String: Any]] = []
            var meta: [String: Any] = ["createdAt": iso(c.createdAt)]
            if let p = c.project {
                workspaces = [["workspaceFolderAbsoluteUri": p.uri]]
                meta["projectId"] = p.id
                meta["workspaceUris"] = [p.uri]
                meta["workspaces"] = workspaces
            }
            var s: [String: Any] = [
                "annotations": ["title": c.title, "markedAsUnread": c.unread, "lastUserViewTime": iso(Date())],
                "createdTime": iso(c.createdAt),
                "lastModifiedTime": iso(c.lastModified),
                "status": status(of: c),
                "stepCount": c.messages.count,
                "summary": c.title,
                "trajectoryId": id,
                "trajectoryMetadata": meta,
                "trajectoryType": "CORTEX_TRAJECTORY_TYPE_CASCADE"
            ]
            if !workspaces.isEmpty { s["workspaces"] = workspaces }
            out[id] = s
        }
        return out
    }
    
    func messagesPayload(cascadeId: String) -> [String: Any]? {
        lock.lock(); defer { lock.unlock() }
        settle(cascadeId)
        guard let c = conversations[cascadeId] else { return nil }
        let toolCount = c.messages.filter { ($0["type"] as? String) == "tools" }.count
        return [
            "cascadeId": c.id, "title": c.title, "status": status(of: c), "hasError": false,
            "duration": "12秒", "totalSteps": c.messages.count, "totalTools": toolCount,
            "totalMessages": c.messages.count, "hasMore": false, "nextOffset": 0,
            "messages": c.messages, "queuedMessages": [], "runningTasks": [],
            "activeModel": "gemini-3.8-flash-high", "modelDisplayName": "Gemini",
            "canProceed": false
        ]
    }
    
    func createConversation(project: Project?, prompt: String) -> String {
        lock.lock(); defer { lock.unlock() }
        counter += 1
        let id = String(format: "demo-%04d-new-%d", 100 + counter, counter)
        var c = Conversation(id: id, title: "新对话", project: project, createdAt: Date(), lastModified: Date(), messages: [])
        conversations[id] = c
        if !prompt.isEmpty {
            c.title = String(prompt.prefix(20))
            c.messages = [userMsg(0, prompt, fileLines: [])]
            c.pending = Pending(startedAt: Date(), userText: prompt, fileNames: [], imageCount: 0)
            conversations[id] = c
        }
        return id
    }
    
    func project(forURI uri: String, id: String?) -> Project? {
        projects.first { $0.id == id || $0.uri == uri }
    }
    
    /// Handles SendUserCascadeMessage. Returns an error message for invalid attachment ids.
    func send(body: [String: Any]) -> String? {
        lock.lock(); defer { lock.unlock() }
        guard let cid = body["cascadeId"] as? String, var c = conversations[cid] else { return nil }
        var text = (body["text"] as? String) ?? ""
        if text.isEmpty, let items = body["items"] as? [[String: Any]], let t = items.first?["text"] as? String { text = t }
        
        var ids: [String] = []
        for a in (body["attachments"] as? [[String: Any]]) ?? [] { if let id = a["id"] as? String { ids.append(id) } }
        let lines = attachmentLines(ids: ids)
        if lines.count != ids.count { return "附件不存在或已过期" }
        
        var media: [String] = []
        for img in (body["images"] as? [[String: Any]]) ?? [] { if let b = img["base64Data"] as? String { media.append(b) } }
        
        let names = lines.map { line -> String in
            let path = line.dropFirst(2).components(separatedBy: " (").first ?? ""
            return files[path]?.name ?? path
        }
        if text.isEmpty && lines.isEmpty && media.isEmpty { return nil }
        c.messages.append(userMsg(c.messages.count, text, fileLines: lines, media: media))
        c.pending = Pending(startedAt: Date(), userText: text, fileNames: names, imageCount: media.count)
        c.lastModified = Date()
        conversations[cid] = c
        return nil
    }
    
    func rename(ids: [String], title: String) {
        lock.lock(); defer { lock.unlock() }
        for id in ids { conversations[id]?.title = title }
    }
    
    func markRead(ids: [String]) {
        lock.lock(); defer { lock.unlock() }
        for id in ids { conversations[id]?.unread = false }
    }
    
    func delete(id: String) {
        lock.lock(); defer { lock.unlock() }
        conversations.removeValue(forKey: id)
    }
    
    func cancel(id: String) {
        lock.lock(); defer { lock.unlock() }
        conversations[id]?.pending = nil
    }
    
    func projectsPayload() -> [[String: Any]] {
        lock.lock(); defer { lock.unlock() }
        return projects.map { p in
            let count = conversations.values.filter { $0.project?.id == p.id }.count
            return [
                "id": p.id, "name": p.name, "uri": p.uri, "path": String(p.uri.dropFirst(7)),
                "isWorkspace": false, "sessionCount": count, "lastActive": iso(Date())
            ] as [String: Any]
        }
    }
    
    private static func friendly(_ seconds: TimeInterval) -> String {
        let total = Int(seconds)
        let d = total / 86400, h = (total % 86400) / 3600, m = (total % 3600) / 60
        if d > 0 { return "\(d)d \(h)h" }
        if h > 0 { return "\(h)h \(m)m" }
        return "\(m)m"
    }
    
    func quotasPayload() -> [String: Any] {
        lock.lock(); defer { lock.unlock() }
        func bucket(_ pct: Double, _ reset: TimeInterval) -> [String: Any] {
            ["remaining_fraction": pct / 100, "remaining_percent": pct,
             "reset_time": iso(Date().addingTimeInterval(reset)), "reset_friendly": Self.friendly(reset)]
        }
        func account(_ a: QuotaAccount) -> [String: Any] {
            ["id": a.id, "email": a.email, "name": a.name, "is_current": a.id == currentQuotaId,
             "claude_5h": bucket(a.claude5h, a.reset5h), "claude_weekly": bucket(a.claudeWeekly, a.resetWeekly),
             "gemini_5h": bucket(a.gemini5h, a.reset5h), "gemini_weekly": bucket(a.geminiWeekly, a.resetWeekly),
             "updated_at": quotaUpdatedAt]
        }
        let all = quotaAccounts.map(account)
        let current = quotaAccounts.first { $0.id == currentQuotaId }.map(account) ?? all.first as Any
        return ["current_account": current, "accounts": all, "updated_at": quotaUpdatedAt]
    }
    
    /// Simulates POST /api/v1/cockpit/switch.
    func switchQuotaAccount(id: String) {
        lock.lock(); defer { lock.unlock() }
        if quotaAccounts.contains(where: { $0.id == id }) { currentQuotaId = id }
        quotaUpdatedAt = Int64(Date().timeIntervalSince1970 * 1000)
    }
    
    /// Simulates POST /api/v1/cockpit/refresh: new timestamp and slightly different numbers.
    func refreshQuotas() {
        lock.lock(); defer { lock.unlock() }
        func jitter(_ v: Double) -> Double { max(0, min(100, (v + Double.random(in: -3...1)).rounded(toPlaces: 1))) }
        quotaAccounts = quotaAccounts.map { a in
            var b = a
            b.claude5h = jitter(a.claude5h); b.claudeWeekly = jitter(a.claudeWeekly)
            b.gemini5h = jitter(a.gemini5h); b.geminiWeekly = jitter(a.geminiWeekly)
            return b
        }
        quotaUpdatedAt = Int64(Date().timeIntervalSince1970 * 1000)
    }
}

private extension Double {
    func rounded(toPlaces places: Int) -> Double {
        let f = pow(10, Double(places))
        return (self * f).rounded() / f
    }
}
