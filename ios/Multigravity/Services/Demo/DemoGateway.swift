import Foundation
import CryptoKit
import UniformTypeIdentifiers

/// In-memory simulated gateway backing Demo Mode. All requests addressed
/// to `DemoGateway.baseURL` are answered locally by `DemoURLProtocol`; nothing leaves the device.
final class DemoGateway: @unchecked Sendable {
    static let host = "127.0.0.1"
    static let port = 9
    static let baseURL = URL(string: "http://\(host):\(port)")!

    private static let enabledLock = NSLock()
    private static var _isEnabled = false
    /// Mirrors `AppSettings.isDemoMode`; readable from any thread (URLProtocol runs off the main actor).
    static var isEnabled: Bool {
        get { enabledLock.lock(); defer { enabledLock.unlock() }; return _isEnabled }
        set { enabledLock.lock(); _isEnabled = newValue; enabledLock.unlock() }
    }

    static let shared = DemoGateway()

    private static var activeLanguageCode: String {
        (Bundle.main.preferredLocalizations.first ?? Locale.preferredLanguages.first ?? "en").lowercased()
    }

    static var isZH: Bool {
        activeLanguageCode.hasPrefix("zh")
    }

    static var isJA: Bool {
        activeLanguageCode.hasPrefix("ja")
    }

    static func t(_ zh: String, _ en: String) -> String {
        isZH ? zh : en
    }

    static func t(_ zh: String, _ en: String, _ ja: String) -> String {
        if isZH { return zh }
        if isJA { return ja }
        return en
    }

    struct Project {
        let id: String
        let name: String
        var alias: String?
        let workspaceUri: String
        let path: String
        let isWorkspace: Bool
    }

    struct DemoFileDiff {
        let fileUri: String
        let fileName: String
        let actionType: String // "MODIFY" | "CREATE" | "DELETE"
        let additions: Int
        let deletions: Int
        let diffLines: [(type: String, text: String)] // "INSERT" | "DELETE" | "UNCHANGED"

        func toJSON() -> [String: Any] {
            [
                "fileUri": fileUri,
                "fileName": fileName,
                "actionType": actionType,
                "additions": additions,
                "deletions": deletions,
                "diffLines": diffLines.map { ["type": $0.type, "text": $0.text] }
            ]
        }
    }

    struct DemoGitState {
        let repoName: String
        var branch: String
        let upstream: String?
        var ahead: Int
        let behind: Int
        let detached: Bool
        var files: [[String: Any]]
        let githubTarget: [String: Any]?
        var pullRequests: [[String: Any]]

        var isClean: Bool { files.isEmpty }

        func toJSON() -> [String: Any] {
            var dict: [String: Any] = [
                "repoName": repoName,
                "branch": branch,
                "ahead": ahead,
                "behind": behind,
                "detached": detached,
                "files": files,
                "clean": files.isEmpty,
                "pullRequests": pullRequests
            ]
            if let upstream { dict["upstream"] = upstream }
            if let githubTarget { dict["githubTarget"] = githubTarget }
            return dict
        }
    }

    struct Conversation {
        let id: String
        var title: String
        let workspaceName: String
        let workspaceUri: String
        var status: String
        var duration: String
        var messages: [Message]
        var pendingInteraction: [String: Any]?
        var canProceed: Bool
        var proceedArtifactUri: String?
        var runningTasks: [[String: Any]]
        var queuedMessages: [[String: Any]]
        var lastModified: Date
        var unread: Bool
        var pinned: Bool
        var modelId: String
        var modelName: String
        var startedAt: String
        var contextMeter: ContextMeterItem?
        var swarm: [String: Any]?
        var subagents: [[String: Any]]
        var parentConversationId: String?
        var subagentRole: String?
        var gitStatus: DemoGitState?
        var changes: [DemoFileDiff]
        var chapters: [[String: Any]]
        var activeFocus: String? = nil
        var activeFocusStartedAt: String? = nil
        var activePlan: [String: Any]? = nil

        var isSubagent: Bool {
            guard let parent = parentConversationId else { return false }
            return !parent.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
        }
    }

    struct Message {
        let id: String
        var type: String // "user" | "agent" | "tools" | "error" | "subagent" | "workflow"
        var text: String
        var thinking: String? = nil
        var thinkingDuration: String? = nil
        var toolCount: Int?
        var toolNames: [String]?
        var toolDetails: [[String: Any]]? = nil
        var imageUrls: [String]?
        var artifacts: [[String: Any]]?
        var stepIndex: Int
        var attemptCount: Int?
        var maxAttempts: Int?
        var model: String?
        var modelName: String?
        var timestamp: String?
        var subagent: [String: Any]?
        var workflow: [String: Any]?
        var stepPlan: [String: Any]? = nil
        var focusText: String? = nil
        var startedAt: String? = nil
        var slashCommands: [[String: Any]]?
        var clientMessageId: String?
        var errorKind: String?
        var isOverflow: Bool?

        func toJSON() -> [String: Any] {
            var dict: [String: Any] = [
                "id": id,
                "type": type,
                "text": text,
                "stepIndex": stepIndex
            ]
            if let thinking { dict["thinking"] = thinking }
            if let thinkingDuration { dict["thinkingDuration"] = thinkingDuration }
            if let toolCount { dict["toolCount"] = toolCount }
            if let toolNames { dict["toolNames"] = toolNames }
            if let toolDetails { dict["toolDetails"] = toolDetails }
            if let imageUrls { dict["imageUrls"] = imageUrls }
            if let artifacts { dict["artifacts"] = artifacts }
            if let attemptCount { dict["attemptCount"] = attemptCount }
            if let maxAttempts { dict["maxAttempts"] = maxAttempts }
            if let model { dict["model"] = model }
            if let modelName { dict["modelName"] = modelName }
            if let timestamp { dict["timestamp"] = timestamp }
            if let subagent { dict["subagent"] = subagent }
            if let workflow { dict["workflow"] = workflow }
            if let stepPlan { dict["stepPlan"] = stepPlan }
            if let focusText { dict["focusText"] = focusText }
            if let startedAt { dict["startedAt"] = startedAt }
            if let slashCommands { dict["slashCommands"] = slashCommands }
            if let clientMessageId { dict["clientMessageId"] = clientMessageId }
            if let errorKind { dict["errorKind"] = errorKind }
            if let isOverflow { dict["isOverflow"] = isOverflow }
            return dict
        }
    }

    struct StoredFile {
        let id: String
        let name: String
        let data: Data
        let path: String
    }

    static let inboxPath = "/Users/demo/Multigravity/Inbox/2026-11"

    struct QuotaAccount {
        let id: String
        let email: String
        let name: String
        let tier: String
        var claude5h: Double
        var claudeWeekly: Double
        var gemini5h: Double
        var geminiWeekly: Double
        var argon: Double?
        var claude5hOffsetHours: Double
        var claudeWeeklyOffsetDays: Double
        var gemini5hOffsetHours: Double
        var geminiWeeklyOffsetDays: Double
    }

    let lock = NSLock()
    var projects: [Project] = []
    var conversations: [Conversation] = []
    var files: [String: StoredFile] = [:]
    var accounts: [QuotaAccount] = []
    var activeAccountId: String = "acc-ultra"
    var activeGlobalModelId: String = "gemini-4-neon-high"
    var nextIdCounter = 100
    var plugins: [[String: Any]] = []
    var pluginProviders: [[String: Any]] = []
    var automations: [[String: Any]] = []
    var automationEvents: [[String: Any]] = []
    var workflowTemplates: [WorkflowTemplateRecord] = []
    var globalDaemonTasks: [[String: Any]] = []
    var taskLogs: [String: String] = [:]
    var mcpServers: [McpServerState] = []
    var skills: [SkillInfo] = []
    var nextSwarmSeq: Int = 10

    private init() {
        seed()
    }

    func reset() {
        lock.lock()
        projects = []
        conversations = []
        files = [:]
        accounts = []
        activeAccountId = "acc-ultra"
        activeGlobalModelId = "gemini-4-neon-high"
        nextIdCounter = 100
        plugins = []
        pluginProviders = []
        automations = []
        automationEvents = []
        workflowTemplates = []
        globalDaemonTasks = []
        taskLogs = [:]
        mcpServers = []
        skills = []
        nextSwarmSeq = 10
        lock.unlock()
        seed()
    }

    // MARK: - Payloads

    func trajectoriesPayload() -> [String: Any] {
        lock.lock()
        defer { lock.unlock() }
        var summaries: [String: Any] = [:]
        let nowIso = Self.iso(Date())
        for c in conversations where !c.isSubagent {
            var annotations: [String: Any] = [
                "title": c.title,
                "markedAsUnread": c.unread
            ]
            if c.pinned {
                annotations["pinned"] = true
            }
            if !c.unread {
                annotations["lastUserViewTime"] = nowIso
            }

            let isPureChat = c.workspaceUri.isEmpty || c.workspaceUri == "chat"
            var meta: [String: Any] = ["createdAt": c.startedAt]
            var workspaces: [[String: Any]] = []
            if !isPureChat {
                meta["workspaceUris"] = [c.workspaceUri]
                workspaces = [["workspaceFolderAbsoluteUri": c.workspaceUri]]
            }

            let blockingTasksCount = c.runningTasks.filter { ($0["isDaemon"] as? Bool) != true }.count
            let hasRunningSubagents = c.subagents.contains { ($0["status"] as? String) == "running" }
            let isErr = c.status == "CASCADE_RUN_STATUS_ERROR"

            var entry: [String: Any] = [
                "trajectoryId": c.id,
                "trajectoryType": "CORTEX_TRAJECTORY_TYPE_CASCADE",
                "summary": c.title,
                "status": c.status,
                "stepCount": c.messages.count,
                "createdTime": c.startedAt,
                "lastModifiedTime": Self.iso(c.lastModified),
                "workspaceName": isPureChat ? "Chat" : c.workspaceName,
                "annotations": annotations,
                "trajectoryMetadata": meta,
                "needsInput": c.pendingInteraction != nil,
                "hasError": isErr,
                "hasRunningTasks": blockingTasksCount > 0,
                "runningTaskCount": blockingTasksCount,
                "backgroundTaskCount": c.runningTasks.count,
                "hasRunningSubagents": hasRunningSubagents
            ]
            if !workspaces.isEmpty {
                entry["workspaces"] = workspaces
            }
            if let swarm = c.swarm {
                entry["hasSwarm"] = true
                if let brief = swarm["brief"] as? [String: Any] {
                    if let wc = brief["workingCount"] as? Int, wc > 0 {
                        entry["swarmWorkingCount"] = wc
                    }
                    if let tm = brief["totalMembers"] as? Int, tm > 0 {
                        entry["swarmTotalMembers"] = tm
                    }
                }
            }
            if isErr, let lastErr = c.messages.last(where: { $0.type == "error" }) {
                entry["errorMessage"] = lastErr.text
            }
            summaries[c.id] = entry
        }
        return ["trajectorySummaries": summaries]
    }

    func projectsPayload() -> [[String: Any]] {
        lock.lock()
        defer { lock.unlock() }
        return projects.map { p in
            let matches: (Conversation) -> Bool = { c in
                guard !c.isSubagent else { return false }
                if !p.isWorkspace {
                    return c.workspaceUri.isEmpty || c.workspaceUri == "chat"
                }
                return c.workspaceUri == p.workspaceUri
            }
            let matching = conversations.filter(matches)
            let count = matching.count
            let latest = matching.map(\.lastModified).max() ?? Date().addingTimeInterval(-3600)
            var dict: [String: Any] = [
                "id": p.id,
                "name": p.name,
                "uri": p.workspaceUri,
                "path": p.path,
                "isWorkspace": p.isWorkspace,
                "sessionCount": count,
                "lastActive": Self.iso(latest)
            ]
            if let alias = p.alias, !alias.isEmpty {
                dict["alias"] = alias
            }
            return dict
        }
    }

    func updateProjectAlias(path: String, alias: String) {
        lock.lock()
        defer { lock.unlock() }
        let cleanPath = path.trimmingCharacters(in: .whitespacesAndNewlines)
        let cleanAlias = alias.trimmingCharacters(in: .whitespacesAndNewlines)
        for i in projects.indices {
            if projects[i].path == cleanPath || projects[i].workspaceUri == cleanPath || projects[i].id == cleanPath {
                projects[i].alias = cleanAlias.isEmpty ? nil : cleanAlias
            }
        }
    }

    func createProject(path: String, name: String?) -> [String: Any] {
        lock.lock()
        defer { lock.unlock() }
        let clean = path.trimmingCharacters(in: .whitespacesAndNewlines)
        let uri = clean.hasPrefix("file://") ? clean : "file://\(clean)"
        if let existing = projects.first(where: { $0.path == clean || $0.workspaceUri == uri }) {
            return [
                "ok": true,
                "alreadyExists": true,
                "project": [
                    "id": existing.id,
                    "name": existing.name,
                    "uri": existing.workspaceUri,
                    "path": existing.path,
                    "isWorkspace": existing.isWorkspace,
                    "sessionCount": 0,
                    "lastActive": Self.iso(Date())
                ]
            ]
        }
        nextIdCounter += 1
        let id = "demo-proj-\(nextIdCounter)"
        let resolvedName = (name?.trimmingCharacters(in: .whitespacesAndNewlines)).flatMap { $0.isEmpty ? nil : $0 }
            ?? URL(fileURLWithPath: clean).lastPathComponent
        let newProj = Project(id: id, name: resolvedName, alias: nil, workspaceUri: uri, path: clean, isWorkspace: true)
        projects.insert(newProj, at: 0)
        return [
            "ok": true,
            "alreadyExists": false,
            "project": [
                "id": newProj.id,
                "name": newProj.name,
                "uri": newProj.workspaceUri,
                "path": newProj.path,
                "isWorkspace": newProj.isWorkspace,
                "sessionCount": 0,
                "lastActive": Self.iso(Date())
            ]
        ]
    }

    func listLocalDirs(path: String?) -> [String: Any] {
        lock.lock()
        let existingPaths = Set(projects.map(\.path))
        lock.unlock()
        let home = "/Users/demo"
        let defaultDir = "/Users/demo/Projects"
        let current = (path?.trimmingCharacters(in: .whitespacesAndNewlines)).flatMap { $0.isEmpty ? nil : $0 } ?? defaultDir
        let normalized = current == "~" ? home : (current.hasPrefix("~/") ? home + "/" + current.dropFirst(2) : current)
        let parent = (normalized == home || normalized == "/") ? "" : URL(fileURLWithPath: normalized).deletingLastPathComponent().path
        let subEntries: [[String: Any]]
        if normalized == defaultDir {
            let names = ["antigravity-mobile", "multigravity-web", "sandbox"]
            subEntries = names.map { name in
                let full = "\(defaultDir)/\(name)"
                return [
                    "name": name,
                    "path": full,
                    "isGitRepo": name != "sandbox",
                    "isProject": existingPaths.contains(full),
                    "isHidden": false
                ]
            }
        } else if normalized == home {
            subEntries = ["Projects", "Websites", "Desktop"].map { name in
                let full = "\(home)/\(name)"
                return [
                    "name": name,
                    "path": full,
                    "isGitRepo": false,
                    "isProject": existingPaths.contains(full),
                    "isHidden": false
                ]
            }
        } else {
            subEntries = []
        }
        let display = normalized == home ? "~" : normalized.replacingOccurrences(of: home, with: "~")
        return [
            "ok": true,
            "platform": "darwin",
            "homePath": home,
            "defaultDir": defaultDir,
            "currentPath": normalized,
            "currentDisplay": display,
            "parentPath": parent,
            "canGoUp": !parent.isEmpty,
            "recommendedDirs": [
                ["path": defaultDir, "name": "Projects", "displayPath": "~/Projects", "projectCount": existingPaths.count, "isDefault": true, "exists": true],
                ["path": "/Users/demo/Websites", "name": "Websites", "displayPath": "~/Websites", "projectCount": 1, "isDefault": false, "exists": true],
                ["path": home, "name": Self.t("主目录 (~)", "Home (~)", "ホーム (~)"), "displayPath": "~", "projectCount": 0, "isDefault": false, "exists": true]
            ],
            "entries": subEntries
        ]
    }

    func createLocalDir(parentPath: String?, name: String?, path: String?) -> [String: Any] {
        let target: String
        if let p = path?.trimmingCharacters(in: .whitespacesAndNewlines), !p.isEmpty {
            target = p.hasPrefix("~/") ? "/Users/demo/" + p.dropFirst(2) : p
        } else {
            let parent = (parentPath?.trimmingCharacters(in: .whitespacesAndNewlines)).flatMap { $0.isEmpty ? nil : $0 } ?? "/Users/demo/Projects"
            let folder = (name?.trimmingCharacters(in: .whitespacesAndNewlines)).flatMap { $0.isEmpty ? nil : $0 } ?? "new-folder"
            target = parent.hasSuffix("/") ? "\(parent)\(folder)" : "\(parent)/\(folder)"
        }
        return [
            "ok": true,
            "path": target,
            "name": URL(fileURLWithPath: target).lastPathComponent,
            "displayPath": target.replacingOccurrences(of: "/Users/demo", with: "~"),
            "alreadyExists": false
        ]
    }

    func cloneGitHubRepo(repoSlug: String, parentDir: String?, folderName: String?) -> [String: Any] {
        let cleaned = repoSlug
            .trimmingCharacters(in: .whitespacesAndNewlines)
            .replacingOccurrences(of: "https://github.com/", with: "")
            .replacingOccurrences(of: "git@github.com:", with: "")
            .replacingOccurrences(of: ".git", with: "")
        let repo = (folderName?.trimmingCharacters(in: .whitespacesAndNewlines)).flatMap { $0.isEmpty ? nil : $0 }
            ?? cleaned.split(separator: "/").last.map(String.init)
            ?? "cloned-repo"
        let parent = (parentDir?.trimmingCharacters(in: .whitespacesAndNewlines)).flatMap { $0.isEmpty ? nil : $0 } ?? "/Users/demo/Projects"
        let target = parent.hasSuffix("/") ? "\(parent)\(repo)" : "\(parent)/\(repo)"
        let created = createProject(path: target, name: repo)
        return [
            "ok": true,
            "async": false,
            "repoSlug": cleaned,
            "status": "done",
            "phase": "done",
            "progress": 100,
            "message": "克隆完成，已加入 Antigravity 项目列表",
            "targetPath": target,
            "path": target,
            "folderUri": "file://\(target)",
            "project": created["project"] as Any,
            "alreadyExists": created["alreadyExists"] as Any
        ]
    }

    func statusPayload() -> [String: Any] {
        lock.lock()
        let first = conversations.first(where: { !$0.isSubagent })
        lock.unlock()
        var payload: [String: Any] = [
            "status": "connected",
            "os": "darwin",
            "platform": Self.t("macOS (Apple M4 Max · 演示网关)", "macOS (Apple M4 Max · Demo Gateway)", "macOS (Apple M4 Max · デモゲートウェイ)"),
            "usedInterface": "DemoLoopback",
            "connectionDescription": Self.t("本机演示模拟网关", "Local On-Device Demo Gateway", "ローカルデモ模擬ゲートウェイ"),
            "upstream": [
                "pid": 8341,
                "port": 65020,
                "is_healthy": true,
                "version": "2.34.0"
            ],
            "compat": [
                "version": "2.34.0",
                "checked": true,
                "coreOk": true,
                "unavailable": [],
                "missingRpcs": []
            ]
        ]
        if let first {
            payload["unified_cursor"] = [
                "cascade_id": first.id,
                "title": first.title,
                "source": "desktop",
                "updated_at": Self.iso(first.lastModified),
                "is_sticky": false,
                "time_skew_ms": 0
            ]
        }
        return payload
    }

    func modelsPayload() -> Data {
        let options: [ModelOption] = [
            // Gemini 4 Neon (current flagship)
            ModelOption(id: "gemini-4-neon-xhigh", name: "Gemini 4 Neon (Extra)", provider: "gemini", enum: "MODEL_PLACEHOLDER_M535", supportsImages: true, thinking: true, tag: "New"),
            ModelOption(id: "gemini-4-neon-high", name: "Gemini 4 Neon (High)", provider: "gemini", enum: "MODEL_PLACEHOLDER_M532", supportsImages: true, thinking: true, tag: "New"),
            ModelOption(id: "gemini-4-neon-medium", name: "Gemini 4 Neon (Medium)", provider: "gemini", enum: "MODEL_PLACEHOLDER_M533", supportsImages: true, thinking: true, tag: "New"),
            ModelOption(id: "gemini-4-neon-low", name: "Gemini 4 Neon (Low)", provider: "gemini", enum: "MODEL_PLACEHOLDER_M534", supportsImages: true, thinking: true, tag: "New"),
            // Gemini 4 Carbon
            ModelOption(id: "gemini-4-carbon-high", name: "Gemini 4 Carbon (High)", provider: "gemini", enum: "MODEL_PLACEHOLDER_M427", supportsImages: true, thinking: true, tag: "New"),
            ModelOption(id: "gemini-4-carbon-medium", name: "Gemini 4 Carbon (Medium)", provider: "gemini", enum: "MODEL_PLACEHOLDER_M437", supportsImages: true, thinking: true, tag: "New"),
            ModelOption(id: "gemini-4-carbon-low", name: "Gemini 4 Carbon (Low)", provider: "gemini", enum: "MODEL_PLACEHOLDER_M438", supportsImages: true, thinking: true, tag: "New"),
            // Gemini 4 Argon (independent quota pool)
            ModelOption(id: "gemini-4-argon-high", name: "Gemini 4 Argon (High)", provider: "gemini", enum: "MODEL_PLACEHOLDER_M417", supportsImages: true, thinking: true, tag: "New"),
            ModelOption(id: "gemini-4-argon-medium", name: "Gemini 4 Argon (Medium)", provider: "gemini", enum: "MODEL_PLACEHOLDER_M418", supportsImages: true, thinking: true, tag: "New"),
            ModelOption(id: "gemini-4-argon-low", name: "Gemini 4 Argon (Low)", provider: "gemini", enum: "MODEL_PLACEHOLDER_M419", supportsImages: true, thinking: true, tag: "New"),
            // Gemini 4 Hydrogen (fast worker / coding)
            ModelOption(id: "gemini-4-hydrogen-xhigh", name: "Gemini 4 Hydrogen (Extra)", provider: "gemini", enum: "MODEL_PLACEHOLDER_M528", supportsImages: true, thinking: true, tag: "New"),
            ModelOption(id: "gemini-4-hydrogen-high", name: "Gemini 4 Hydrogen (High)", provider: "gemini", enum: "MODEL_PLACEHOLDER_M525", supportsImages: true, thinking: true, tag: "New"),
            ModelOption(id: "gemini-4-hydrogen-medium", name: "Gemini 4 Hydrogen (Medium)", provider: "gemini", enum: "MODEL_PLACEHOLDER_M526", supportsImages: true, thinking: true, tag: "New"),
            ModelOption(id: "gemini-4-hydrogen-low", name: "Gemini 4 Hydrogen (Low)", provider: "gemini", enum: "MODEL_PLACEHOLDER_M527", supportsImages: true, thinking: true, tag: "New"),
            // Gemini 3.8 Flash
            ModelOption(id: "gemini-3.8-flash-high", name: "Gemini 3.8 Flash (High)", provider: "gemini", enum: "MODEL_PLACEHOLDER_M318", supportsImages: true, thinking: true, tag: nil),
            ModelOption(id: "gemini-3.8-flash-medium", name: "Gemini 3.8 Flash (Medium)", provider: "gemini", enum: "MODEL_PLACEHOLDER_M319", supportsImages: true, thinking: true, tag: nil),
            ModelOption(id: "gemini-3.8-flash-low", name: "Gemini 3.8 Flash (Low)", provider: "gemini", enum: "MODEL_PLACEHOLDER_M320", supportsImages: true, thinking: true, tag: nil),
            // Claude 5.5
            ModelOption(id: "claude-opus-5-5-high", name: "Claude Opus 5.5 (High)", provider: "claude", enum: "MODEL_PLACEHOLDER_M402", supportsImages: true, thinking: true, tag: "New"),
            ModelOption(id: "claude-opus-5-5-medium", name: "Claude Opus 5.5 (Medium)", provider: "claude", enum: "MODEL_PLACEHOLDER_M401", supportsImages: true, thinking: true, tag: "New"),
            ModelOption(id: "claude-opus-5-5-low", name: "Claude Opus 5.5 (Low)", provider: "claude", enum: "MODEL_PLACEHOLDER_M400", supportsImages: true, thinking: true, tag: "New"),
            ModelOption(id: "claude-sonnet-5-5-high", name: "Claude Sonnet 5.5 (High)", provider: "claude", enum: "MODEL_PLACEHOLDER_M405", supportsImages: true, thinking: true, tag: "New"),
            ModelOption(id: "claude-sonnet-5-5-medium", name: "Claude Sonnet 5.5 (Medium)", provider: "claude", enum: "MODEL_PLACEHOLDER_M404", supportsImages: true, thinking: true, tag: "New"),
            ModelOption(id: "claude-sonnet-5-5-low", name: "Claude Sonnet 5.5 (Low)", provider: "claude", enum: "MODEL_PLACEHOLDER_M403", supportsImages: true, thinking: true, tag: "New"),
            // Orchestrator
            ModelOption(id: "gemini-pro-orchestrator-neon", name: "Orchestrator (Neon)", provider: "orchestrator", enum: "MODEL_PLACEHOLDER_M537", supportsImages: true, thinking: true, tag: "New"),
            ModelOption(id: "gemini-pro-orchestrator", name: "Orchestrator (Carbon)", provider: "orchestrator", enum: "MODEL_PLACEHOLDER_M667", supportsImages: true, thinking: true, tag: "New"),
            ModelOption(id: "gemini-pro-orchestrator-argon", name: "Orchestrator (Argon)", provider: "orchestrator", enum: "MODEL_PLACEHOLDER_M681", supportsImages: true, thinking: true, tag: "New")
        ]
        let response = ModelsResponse(
            models: options,
            defaults: [
                "gemini": "gemini-4-neon-high",
                "claude": "claude-opus-5-5-high",
                "orchestrator": "gemini-pro-orchestrator-neon"
            ],
            live: true
        )
        return (try? JSONEncoder().encode(response)) ?? Data("{\"models\":[],\"defaults\":{},\"live\":true}".utf8)
    }

    func slashCommandsPayload() -> [String: Any] {
        let commands: [[String: Any]] = [
            ["name": "compact", "title": Self.t("压缩上下文", "Compact Context", "コンテキスト圧縮"), "description": Self.t("提炼当前对话关键事实并大幅释放上下文窗口占用", "Summarize key facts and free context window capacity", "現在の会話の要点を抽出しコンテキスト枠を大きく解放"), "icon": "collapse_all", "kind": "system"],
            ["name": "plan", "title": Self.t("制定实施计划", "Create Plan", "実装計画の作成"), "description": Self.t("先拆解架构、关键路径与风险，确认后再动手改代码", "Break down architecture, milestones, and risks before coding", "実装前に構成・マイルストーン・リスクを整理して合意形成"), "icon": "ballot", "kind": "system"],
            ["name": "grill-me", "title": Self.t("深度追问对齐", "Deep Clarify", "深掘りヒアリング"), "description": Self.t("通过多轮聚焦提问暴露隐含前提并消除设计歧义", "Ask focused follow-up questions to resolve design ambiguity", "集中的な質問で暗黙の前提を洗い出し設計の曖昧さを解消"), "icon": "forum", "kind": "system"],
            ["name": "workflow", "title": Self.t("动态多智能体工作流", "Multi-Agent Workflow", "マルチエージェントワークフロー"), "description": Self.t("编写并执行 Python 并发/流水线编排工作流脚本", "Write and run a concurrent/pipelined multi-agent Python workflow", "並列・パイプライン構成の Python ワークフローを作成・実行"), "icon": "graph_2", "kind": "system"],
            ["name": "teamwork", "title": Self.t("蜂群协同攻坚", "Swarm Teamwork", "スウォーム協調開発"), "description": Self.t("组建多角色 Swarm Agent 协作板并发推进大型任务", "Coordinate multiple Swarm agents via a shared board", "共有ボード上で複数 Swarm エージェントを並列連携"), "icon": "graph_2", "kind": "system"],
            ["name": "goal", "title": Self.t("长程目标模式", "Goal Mode", "自律ゴールモード"), "description": Self.t("持续自主验证与迭代直至完全达成验收目标", "Iterate and self-verify autonomously until acceptance criteria pass", "受け入れ基準を満たすまで自律的に検証と反復を継続"), "icon": "timer", "kind": "system"],
            ["name": "boost", "title": Self.t("深度思考强化", "Deep Reasoning", "ディープ推論強化"), "description": Self.t("调用多视角严格验证与高算力推理链路", "Enable multi-perspective reasoning and rigorous verification", "多角的な検証と高精度推論パスを有効化"), "icon": "bolt", "kind": "system"],
            ["name": "schedule", "title": Self.t("定时自动化", "Schedule Automation", "スケジュール自動化"), "description": Self.t("创建 Cron 定时巡检或延时唤醒触发器", "Create a recurring cron job or delayed timer trigger", "定期実行 Cron または遅延タイマーのトリガーを作成"), "icon": "schedule", "kind": "system"],
            ["name": "btw", "title": Self.t("旁路快问", "Side Question", "サイド質問"), "description": Self.t("不打断主线上下文的轻量临时提问", "Ask a lightweight side question without altering main context", "メインの文脈を崩さず軽い確認質問を投げる"), "icon": "chat_add_on", "kind": "system"],
            ["name": "chart-generator", "title": "chart-generator", "description": Self.t("自动化图表生成与多维度可视化分析", "Automated data visualization and chart generation", "データ可視化とインタラクティブグラフの自動生成"), "icon": "code_blocks", "kind": "skill"],
            ["name": "sql-inspector", "title": "sql-inspector", "description": Self.t("SQL 查询调优与执行计划诊断", "SQL query tuning and execution-plan diagnostics", "SQL クエリ最適化と実行計画の診断"), "icon": "code_blocks", "kind": "skill"],
            ["name": "swift-concurrency-audit", "title": "swift-concurrency-audit", "description": Self.t("Swift 6 并发模型与 Actor 隔离静态审查", "Swift 6 concurrency and actor-isolation static audit", "Swift 6 並行処理モデルと Actor 分離の静的監査"), "icon": "lightbulb_2", "kind": "skill"],
            ["name": "swift-refactor", "title": "swift-refactor", "description": Self.t("SwiftUI / Swift 并发与视图拆分重构指南", "SwiftUI view decomposition and concurrency refactoring guide", "SwiftUI ビュー分割と並行処理リファクタリング指針"), "icon": "code", "kind": "skill"]
        ]
        return ["commands": commands]
    }

    func messagesPayload(cascadeId: String, limit: Int, offset: Int?) -> [String: Any]? {
        lock.lock()
        defer { lock.unlock() }
        guard let c = conversations.first(where: { $0.id == cascadeId }) else { return nil }
        let total = c.messages.count
        let clampedLimit = max(1, limit)
        let end = min(total, offset ?? total)
        let start = max(0, end - clampedLimit)
        let slice = Array(c.messages[start..<end])
        let toolCount = c.messages.reduce(0) { $0 + ($1.toolCount ?? 0) }

        // Build full-session TurnOutline aligned with live Antigravity gateway (4cf30438)
        var turnOutline: [[String: Any]] = []
        var turnIdx = 0
        var i = 0
        while i < c.messages.count {
            let msg = c.messages[i]
            if msg.type == "user" || turnOutline.isEmpty {
                turnIdx += 1
                var endIdx = i + 1
                while endIdx < c.messages.count && c.messages[endIdx].type != "user" {
                    endIdx += 1
                }
                let turnSlice = c.messages[i..<endIdx]
                let turnTools = turnSlice.reduce(0) { $0 + ($1.toolCount ?? 0) }
                let turnArtifacts = turnSlice.flatMap { $0.artifacts ?? [] }
                let turnHasError = turnSlice.contains { $0.type == "error" }
                let turnModel = turnSlice.last(where: { $0.modelName != nil })?.modelName ?? c.modelName
                let rawPrompt = msg.text.replacingOccurrences(of: "\n", with: " ").trimmingCharacters(in: .whitespacesAndNewlines)
                let summary = rawPrompt.count > 48 ? String(rawPrompt.prefix(48)) + "…" : rawPrompt
                var item: [String: Any] = [
                    "turnIndex": turnIdx,
                    "firstMessageId": msg.id,
                    "messageOffset": i,
                    "messageCount": endIdx - i,
                    "startStepIndex": msg.stepIndex,
                    "endStepIndex": turnSlice.last?.stepIndex ?? msg.stepIndex,
                    "promptSummary": summary,
                    "totalTools": turnTools,
                    "hasError": turnHasError,
                    "modelName": turnModel
                ]
                if !turnArtifacts.isEmpty { item["artifacts"] = turnArtifacts }
                if let ts = msg.timestamp { item["timestamp"] = ts }
                turnOutline.append(item)
                i = endIdx
            } else {
                i += 1
            }
        }

        var payload: [String: Any] = [
            "cascadeId": c.id,
            "title": c.title,
            "status": c.status,
            "duration": c.duration,
            "totalSteps": total,
            "totalTools": toolCount,
            "totalMessages": total,
            "hasMore": start > 0,
            "nextOffset": start,
            "canProceed": c.canProceed,
            "runningTasks": c.runningTasks,
            "queuedMessages": c.queuedMessages,
            "activeModel": c.modelId,
            "activeModelName": c.modelName,
            "modelDisplayName": c.modelName,
            "desktopActiveModel": c.modelId,
            "desktopActiveModelName": c.modelName,
            "startedAt": c.startedAt,
            "messages": slice.map { $0.toJSON() },
            "turnOutline": turnOutline
        ]
        if let uri = c.proceedArtifactUri { payload["proceedArtifactUri"] = uri }
        if let pi = c.pendingInteraction { payload["pendingInteraction"] = pi }
        if let cm = c.contextMeter,
           let cmData = try? JSONEncoder().encode(cm),
           let cmObj = try? JSONSerialization.jsonObject(with: cmData) {
            payload["contextMeter"] = cmObj
        }
        if let swarm = c.swarm, let brief = swarm["brief"] {
            payload["swarm"] = brief
        }
        if !c.subagents.isEmpty {
            payload["subagents"] = c.subagents
        }
        if let parentId = c.parentConversationId {
            payload["parentConversationId"] = parentId
        }
        if let role = c.subagentRole {
            payload["subagentRole"] = role
        }
        if let focus = c.activeFocus {
            payload["activeFocus"] = focus
        }
        if let focusStart = c.activeFocusStartedAt {
            payload["activeFocusStartedAt"] = focusStart
        }
        if let plan = c.activePlan {
            payload["activePlan"] = plan
        }
        if c.status == "CASCADE_RUN_STATUS_ERROR", let lastErr = c.messages.last(where: { $0.type == "error" }) {
            payload["hasError"] = true
            payload["errorMessage"] = lastErr.text
        }
        return payload
    }

    func quotasPayload() -> [String: Any] {
        lock.lock()
        defer { lock.unlock() }
        let now = Date()
        let updatedMs = Int64(now.timeIntervalSince1970 * 1000)
        func bucket(_ pct: Double, _ reset: Date) -> [String: Any] {
            let rounded = (pct * 10.0).rounded() / 10.0
            return [
                "remaining_fraction": rounded / 100.0,
                "remaining_percent": rounded,
                "reset_time": Self.iso(reset),
                "reset_friendly": friendlyCountdown(from: now, to: reset)
            ]
        }
        func accountDict(_ acc: QuotaAccount) -> [String: Any] {
            let claude5hReset = now.addingTimeInterval(acc.claude5hOffsetHours * 3600)
            let claudeWeeklyReset = now.addingTimeInterval(acc.claudeWeeklyOffsetDays * 86400)
            let gemini5hReset = now.addingTimeInterval(acc.gemini5hOffsetHours * 3600)
            let geminiWeeklyReset = now.addingTimeInterval(acc.geminiWeeklyOffsetDays * 86400)

            var d: [String: Any] = [
                "id": acc.id,
                "email": acc.email,
                "name": acc.name,
                "subscription_tier": acc.tier,
                "is_current": acc.id == activeAccountId,
                "updated_at": updatedMs,
                "claude_weekly": bucket(acc.claudeWeekly, claudeWeeklyReset),
                "gemini_weekly": bucket(acc.geminiWeekly, geminiWeeklyReset)
            ]
            if acc.tier.uppercased() != "FREE" {
                d["claude_5h"] = bucket(acc.claude5h, claude5hReset)
                d["gemini_5h"] = bucket(acc.gemini5h, gemini5hReset)
            }
            if let argon = acc.argon {
                d["gemini_4_argon"] = bucket(argon, gemini5hReset)
            }
            return d
        }
        let current = accounts.first { $0.id == activeAccountId } ?? accounts.first
        return [
            "source": "cockpit",
            "cockpit_installed": true,
            "current_account": current.map(accountDict) as Any,
            "accounts": accounts.map(accountDict),
            "updated_at": updatedMs
        ]
    }

    private func friendlyCountdown(from now: Date, to target: Date) -> String {
        let secs = max(60, Int(target.timeIntervalSince(now)))
        let days = secs / 86400
        let hours = (secs % 86400) / 3600
        let mins = (secs % 3600) / 60
        if days > 0 { return Self.t("\(days)天\(hours)小时", "\(days)d \(hours)h", "\(days)日\(hours)時間") }
        if hours > 0 { return Self.t("\(hours)小时\(mins)分", "\(hours)h \(mins)m", "\(hours)時間\(mins)分") }
        return Self.t("\(mins)分钟", "\(mins)m", "\(mins)分")
    }

    func tokenHistoryPayload() -> Data {
        let cal = Calendar(identifier: .gregorian)
        let today = cal.startOfDay(for: Date())
        let df = DateFormatter()
        df.dateFormat = "yyyy-MM-dd"
        df.timeZone = TimeZone.current

        struct ModelSpec {
            let name: String
            let weight: Double
            let inputPricePerM: Double
            let cachePricePerM: Double
            let outputPricePerM: Double
        }

        let specs: [ModelSpec] = [
            ModelSpec(name: "Gemini 4 Neon (High)", weight: 0.35, inputPricePerM: 2.00, cachePricePerM: 0.20, outputPricePerM: 12.0),
            ModelSpec(name: "Gemini 4 Carbon (High)", weight: 0.22, inputPricePerM: 2.50, cachePricePerM: 0.25, outputPricePerM: 15.0),
            ModelSpec(name: "Gemini 4 Hydrogen (High)", weight: 0.17, inputPricePerM: 0.60, cachePricePerM: 0.06, outputPricePerM: 3.0),
            ModelSpec(name: "Claude Opus 5.5 (High)", weight: 0.13, inputPricePerM: 5.00, cachePricePerM: 0.50, outputPricePerM: 25.0),
            ModelSpec(name: "Gemini 4 Neon (XHigh)", weight: 0.08, inputPricePerM: 2.00, cachePricePerM: 0.20, outputPricePerM: 12.0),
            ModelSpec(name: "Gemini 3.8 Flash (High)", weight: 0.05, inputPricePerM: 0.15, cachePricePerM: 0.015, outputPricePerM: 0.60)
        ]

        var matrix: [DailyTokenRecord] = []
        var totalTokens: Int64 = 0
        var todayTokens: Int64 = 0
        var last7dTokens: Int64 = 0
        var totalCostUsd: Double = 0
        var activeDays = 0
        var totalSteps = 0

        var aggTotals: [String: (total: Int64, input: Int64, output: Int64, cacheRead: Int64, thinking: Int64, steps: Int, cost: Double)] = [:]

        let totalDays = 364
        for offset in stride(from: -totalDays, through: 0, by: 1) {
            guard let date = cal.date(byAdding: .day, value: offset, to: today) else { continue }
            let dayStr = df.string(from: date)
            let weekday = cal.component(.weekday, from: date)
            let seed = abs(offset * 73 + weekday * 19) % 100
            let isRecent = offset >= -21
            let isIdleDay = (offset < -3) && ((weekday == 1 && seed < 50) || seed < 12)

            if isIdleDay {
                matrix.append(DailyTokenRecord(
                    date: dayStr,
                    total: 0,
                    input: 0,
                    output: 0,
                    cacheRead: 0,
                    thinking: 0,
                    steps: 0,
                    costUsd: 0,
                    models: [:],
                    modelDetails: [:]
                ))
                continue
            }

            let intensityMultiplier: Int64 = {
                if offset == 0 { return 11 }
                if isRecent { return Int64(5 + (seed % 7)) }
                if seed > 85 { return 10 }
                if seed > 60 { return 6 }
                if seed > 28 { return 3 }
                return 1
            }()

            // Authentic Antigravity prompt-cache profile (~87% cache read, ~10% fresh input, ~2% output, ~1% thinking)
            let baseTotal = 4_800_000 * intensityMultiplier + Int64((seed % 19) * 320_000)
            var dayInput: Int64 = 0
            var dayCacheRead: Int64 = 0
            var dayOutput: Int64 = 0
            var dayThinking: Int64 = 0
            var daySteps = 0
            var dayCost = 0.0
            var dayModels: [String: Int64] = [:]
            var dayDetails: [String: ModelTokenBreakdown] = [:]

            for (idx, sp) in specs.enumerated() {
                // Slight realistic daily variation per model
                let jitter = 0.85 + Double((seed + idx * 17) % 31) / 100.0
                let mTotal = Int64(Double(baseTotal) * sp.weight * jitter)
                guard mTotal > 0 else { continue }
                let mCache = Int64(Double(mTotal) * 0.872)
                let mInput = Int64(Double(mTotal) * 0.098)
                let mOutput = Int64(Double(mTotal) * 0.021)
                let mThink = max(0, mTotal - mCache - mInput - mOutput)
                let mSteps = max(4, Int(Double(intensityMultiplier * 28) * sp.weight * jitter))
                let mCost = (
                    Double(mInput) * sp.inputPricePerM +
                    Double(mCache) * sp.cachePricePerM +
                    Double(mOutput + mThink) * sp.outputPricePerM
                ) / 1_000_000.0

                dayInput += mInput
                dayCacheRead += mCache
                dayOutput += mOutput
                dayThinking += mThink
                daySteps += mSteps
                dayCost += mCost
                dayModels[sp.name] = mTotal
                dayDetails[sp.name] = ModelTokenBreakdown(
                    model: sp.name,
                    total: mTotal,
                    input: mInput,
                    output: mOutput,
                    cacheRead: mCache,
                    thinking: mThink,
                    steps: mSteps,
                    costUsd: (mCost * 10000).rounded() / 10000
                )

                var cur = aggTotals[sp.name] ?? (0, 0, 0, 0, 0, 0, 0)
                cur.total += mTotal
                cur.input += mInput
                cur.output += mOutput
                cur.cacheRead += mCache
                cur.thinking += mThink
                cur.steps += mSteps
                cur.cost += mCost
                aggTotals[sp.name] = cur
            }

            let dayTotal = dayInput + dayCacheRead + dayOutput + dayThinking
            matrix.append(DailyTokenRecord(
                date: dayStr,
                total: dayTotal,
                input: dayInput,
                output: dayOutput,
                cacheRead: dayCacheRead,
                thinking: dayThinking,
                steps: daySteps,
                costUsd: (dayCost * 100).rounded() / 100,
                models: dayModels,
                modelDetails: dayDetails
            ))

            totalTokens += dayTotal
            if offset == 0 { todayTokens = dayTotal }
            if offset >= -6 { last7dTokens += dayTotal }
            totalCostUsd += dayCost
            activeDays += 1
            totalSteps += daySteps
        }

        let modelSummary: [ModelTokenBreakdown] = aggTotals
            .map { key, v in
                ModelTokenBreakdown(
                    model: key,
                    total: v.total,
                    input: v.input,
                    output: v.output,
                    cacheRead: v.cacheRead,
                    thinking: v.thinking,
                    steps: v.steps,
                    costUsd: (v.cost * 100).rounded() / 100
                )
            }
            .sorted { $0.total > $1.total }

        let desktopTokens = Int64(Double(totalTokens) * 0.78)
        let cliTokens = totalTokens - desktopTokens
        let summary = TokenHistorySummary(
            totalTokens: totalTokens,
            todayTokens: todayTokens,
            last7dTokens: last7dTokens,
            desktopTokens: desktopTokens,
            cliTokens: cliTokens,
            estimatedCostUsd: (totalCostUsd * 100).rounded() / 100,
            estimatedCostCny: (totalCostUsd * 7.25 * 100).rounded() / 100,
            activeDays: activeDays,
            totalSteps: totalSteps
        )
        let resp = TokenHistoryResponse(
            updatedAt: Int64(Date().timeIntervalSince1970 * 1000),
            summary: summary,
            dailyMatrix: matrix,
            modelSummary: modelSummary
        )
        return (try? JSONEncoder().encode(resp)) ?? Data("{}".utf8)
    }

    // MARK: - Actions

    func createConversation(workspace: String?, model: String?) -> String {
        lock.lock()
        defer { lock.unlock() }
        nextIdCounter += 1
        let id = String(format: "demo-%04d-live", nextIdCounter)
        let matched = projects.first { $0.workspaceUri == workspace || $0.name == workspace || $0.path == workspace }
        let wsName = matched?.name ?? "Chat"
        let wsUri = matched?.workspaceUri ?? "chat"
        let chosenModel = (model?.isEmpty == false) ? model! : activeGlobalModelId
        let conv = Conversation(
            id: id,
            title: Self.t("新对话", "New Chat", "新規チャット"),
            workspaceName: wsName,
            workspaceUri: wsUri,
            status: "CASCADE_RUN_STATUS_IDLE",
            duration: Self.t("0秒", "0s", "0秒"),
            messages: [],
            pendingInteraction: nil,
            canProceed: false,
            proceedArtifactUri: nil,
            runningTasks: [],
            queuedMessages: [],
            lastModified: Date(),
            unread: false,
            pinned: false,
            modelId: chosenModel,
            modelName: ModelDefaultsLogic.displayName(forID: chosenModel),
            startedAt: Self.iso(Date()),
            contextMeter: ContextMeterItem(
                usedTokens: 0,
                maxTokens: 512_000,
                percentage: 0,
                inputTokens: 0,
                outputTokens: 0,
                cacheReadTokens: 0,
                thinkingTokens: 0,
                timeToFirstTokenSec: nil,
                streamingDurationSec: nil,
                tokensPerSecond: nil,
                isOverflow: false,
                errorKind: "none"
            ),
            swarm: nil,
            subagents: [],
            parentConversationId: nil,
            subagentRole: nil,
            gitStatus: matched?.isWorkspace == true ? DemoGitState(
                repoName: wsName,
                branch: "main",
                upstream: "origin/main",
                ahead: 0,
                behind: 0,
                detached: false,
                files: [],
                githubTarget: ["host": "github.com", "owner": "demo-dev", "repo": wsName, "branch": "main"],
                pullRequests: []
            ) : nil,
            changes: [],
            chapters: []
        )
        conversations.insert(conv, at: 0)
        return id
    }

    func switchModel(cascadeId: String?, modelEnum: String) {
        lock.lock()
        defer { lock.unlock() }
        let resolvedId = ModelCatalogStore.shared.id(forEnum: modelEnum)
            ?? ModelDefaultsLogic.staticIDByEnum[modelEnum]
            ?? activeGlobalModelId
        activeGlobalModelId = resolvedId
        if let cid = cascadeId, !cid.isEmpty,
           let idx = conversations.firstIndex(where: { $0.id == cid }) {
            conversations[idx].modelId = resolvedId
            conversations[idx].modelName = ModelDefaultsLogic.displayName(forID: resolvedId)
        }
    }

    @discardableResult
    func send(
        cascadeId: String,
        text: String,
        clientMessageId: String? = nil,
        media: [String]? = nil,
        attachmentIds: [String] = [],
        deliveryStrategy: Int? = nil,
        slashCommands: [[String: Any]]? = nil,
        isArtifactApproval: Bool = false
    ) -> String? {
        lock.lock()
        guard let idx = conversations.firstIndex(where: { $0.id == cascadeId }) else {
            lock.unlock()
            return nil
        }

        // Resolve uploaded attachment IDs to file lines
        var attachmentLines: [String] = []
        var attachmentNames: [String] = []
        for aid in attachmentIds {
            guard let f = files.values.first(where: { $0.id == aid }) else {
                lock.unlock()
                return Self.t("附件不存在或已过期", "Attachment not found or expired", "添付ファイルが見つからないか有効期限切れです")
            }
            attachmentLines.append(Self.attachmentLine(path: f.path, name: f.name, size: Int64(f.data.count)))
            attachmentNames.append(f.name)
        }

        if isArtifactApproval {
            let isRedesign = cascadeId == "demo-0005-redesign"
            conversations[idx].canProceed = false
            conversations[idx].proceedArtifactUri = nil
            conversations[idx].status = "CASCADE_RUN_STATUS_RUNNING"
            conversations[idx].lastModified = Date()
            lock.unlock()

            DispatchQueue.global().asyncAfter(deadline: .now() + 0.9) {
                self.lock.lock()
                defer { self.lock.unlock() }
                guard let i = self.conversations.firstIndex(where: { $0.id == cascadeId }) else { return }
                let s1 = self.conversations[i].messages.count
                self.conversations[i].messages.append(Self.toolMsg(id: "\(cascadeId)-tool-\(s1)", step: s1, tools: ["write_file", "run_command", "read_file"]))
                let s2 = self.conversations[i].messages.count
                let replyText = isRedesign
                    ? Self.t(
                        "已按方案完成首页改版：\n\n- ✅ 抽出 `Hero`、`CaseStudies`、`Pricing` 三个组件\n- ✅ 首屏只保留「免费试用」主按钮\n- ✅ 客户案例上移到第二屏\n- ✅ 价格区块改为三档对比卡片\n- ✅ 接入转化埋点（`home_cta_click`）\n\n测试全部通过，改动已提交到 `feat/home-redesign` 分支。",
                        "Completed the homepage redesign as planned:\n\n- ✅ Extracted `Hero`, `CaseStudies`, and `Pricing` components\n- ✅ Kept a single primary \"Start Free Trial\" CTA above the fold\n- ✅ Moved Customer Case Studies into Section 2\n- ✅ Switched Pricing to a 3-tier comparison grid\n- ✅ Wired conversion telemetry (`home_cta_click`)\n\nAll tests passed and changes are committed to `feat/home-redesign`.",
                        "計画どおりトップページの刷新を完了しました：\n\n- ✅ `Hero`・`CaseStudies`・`Pricing` の3コンポーネントを分離\n- ✅ ファーストビューの主ボタンを「無料で試す」に一本化\n- ✅ 導入事例を第2セクションへ引き上げ\n- ✅ 料金セクションを3プラン比較カードに変更\n- ✅ CV計測イベント（`home_cta_click`）を接続\n\n全テスト通過済み、`feat/home-redesign` ブランチへコミット済みです。"
                    )
                    : Self.t(
                        "已按审批通过的方案完成实施与本地验证：\n\n1. 更新核心模块化边界，保留旧接口向后兼容。\n2. 全部单元测试与编译校验均已通过 ✅。\n\n如需查看本次修改的增量 Diff 或直接 Commit，可点击顶部栏的 **Changes** / **Commit**。",
                        "Implemented the approved plan and completed local verification:\n\n1. Updated core module boundaries while keeping legacy API compatibility.\n2. All unit tests and build checks passed ✅.\n\nTap **Changes** or **Commit** in the top bar to inspect the incremental diff.",
                        "承認された計画に沿って実装とローカル検証を完了しました：\n\n1. 既存 API の後方互換を保ちつつコアモジュール境界を整理。\n2. 単体テストおよびビルド検証をすべてパス ✅。\n\n差分確認やコミットは上部バーの **Changes** / **Commit** から確認できます。"
                    )
                self.conversations[i].messages.append(Self.agentMsg(
                    id: "\(cascadeId)-agent-\(s2)",
                    step: s2,
                    model: self.conversations[i].modelId,
                    modelName: self.conversations[i].modelName,
                    text: replyText
                ))
                self.conversations[i].status = "CASCADE_RUN_STATUS_IDLE"
                self.conversations[i].duration = Self.t("4秒", "4s", "4秒")
                self.conversations[i].lastModified = Date()
                self.bumpContextMeter(at: i, addInput: 3_200, addOutput: 410, speed: 54.6, ttft: 0.61)
            }
            return nil
        }

        let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)

        // If already running, queue the message briefly then drain it
        if conversations[idx].status == "CASCADE_RUN_STATUS_RUNNING" && !trimmed.isEmpty {
            let qid = "q-\(UUID().uuidString.prefix(8))"
            conversations[idx].queuedMessages.append([
                "id": qid,
                "text": trimmed,
                "createdAt": Self.iso(Date()),
                "clientMessageId": clientMessageId ?? ""
            ])
            conversations[idx].lastModified = Date()
            let shouldAutoDrain = deliveryStrategy != 2
            lock.unlock()

            if shouldAutoDrain {
                DispatchQueue.global().asyncAfter(deadline: .now() + 1.1) {
                    self.lock.lock()
                    guard let i = self.conversations.firstIndex(where: { $0.id == cascadeId }) else {
                        self.lock.unlock()
                        return
                    }
                    self.conversations[i].queuedMessages.removeAll { ($0["id"] as? String) == qid }
                    self.conversations[i].status = "CASCADE_RUN_STATUS_IDLE"
                    self.lock.unlock()
                    self.send(cascadeId: cascadeId, text: trimmed, clientMessageId: clientMessageId, media: media, attachmentIds: attachmentIds, slashCommands: slashCommands)
                }
            }
            return nil
        }

        if conversations[idx].title == "新对话" || conversations[idx].title == "New Chat" || conversations[idx].title == "新規チャット", !trimmed.isEmpty {
            conversations[idx].title = String(trimmed.prefix(24))
        }

        let imageUrls: [String]? = media?.enumerated().map { offset, b64 in
            let name = "photo-\(UUID().uuidString.prefix(6))-\(offset + 1).jpg"
            let cleanB64 = b64.components(separatedBy: ",").last ?? b64
            let data = Data(base64Encoded: cleanB64) ?? Data()
            let path = "\(Self.inboxPath)/\(name)"
            files[path] = StoredFile(id: Self.fileID(for: data), name: name, data: data, path: path)
            return "file://\(path)"
        }

        // Clear any trailing error steps when continuing
        if trimmed == "Continue" || conversations[idx].status == "CASCADE_RUN_STATUS_ERROR" {
            conversations[idx].messages.removeAll { $0.type == "error" }
        }

        let step = conversations[idx].messages.count
        var displayUserText = trimmed.isEmpty ? "Continue" : trimmed
        if !attachmentLines.isEmpty {
            let block = AttachmentRules.blockHeader + "\n" + attachmentLines.joined(separator: "\n")
            displayUserText = displayUserText == "Continue" ? block : "\(displayUserText)\n\n\(block)"
        }
        var userM = Self.userMsg(
            id: "\(cascadeId)-u-\(step)",
            step: step,
            text: displayUserText,
            imageUrls: imageUrls,
            slashCommands: slashCommands
        )
        userM.clientMessageId = clientMessageId
        if trimmed != "Continue" || !attachmentLines.isEmpty {
            conversations[idx].messages.append(userM)
        }
        conversations[idx].canProceed = false
        conversations[idx].pendingInteraction = nil
        conversations[idx].status = "CASCADE_RUN_STATUS_RUNNING"
        conversations[idx].lastModified = Date()
        let activeModelId = conversations[idx].modelId
        let activeModelName = conversations[idx].modelName
        let isLogin500 = cascadeId == "demo-0002-login-500"
        lock.unlock()

        DispatchQueue.global().asyncAfter(deadline: .now() + 0.85) {
            self.lock.lock()
            defer { self.lock.unlock() }
            guard let i = self.conversations.firstIndex(where: { $0.id == cascadeId }),
                  self.conversations[i].status == "CASCADE_RUN_STATUS_RUNNING" else { return }

            let promptText = trimmed.isEmpty ? "Continue" : trimmed
            let lower = promptText.lowercased()
            if lower.hasPrefix("/compact") {
                let s = self.conversations[i].messages.count
                let prevPct = self.conversations[i].contextMeter?.percentage ?? 88.0
                let newUsed: Int64 = 124_600
                let maxT: Int64 = 512_000
                let newPct = Double(newUsed) / Double(maxT) * 100.0
                self.conversations[i].contextMeter = ContextMeterItem(
                    usedTokens: newUsed,
                    maxTokens: maxT,
                    percentage: newPct,
                    inputTokens: 12_400,
                    outputTokens: 640,
                    cacheReadTokens: 31_200,
                    thinkingTokens: 480,
                    timeToFirstTokenSec: 0.44,
                    streamingDurationSec: 2.1,
                    tokensPerSecond: 61.4,
                    isOverflow: false,
                    errorKind: nil
                )
                self.conversations[i].messages.append(Self.agentMsg(
                    id: "\(cascadeId)-a-\(s)",
                    step: s,
                    model: activeModelId,
                    modelName: activeModelName,
                    text: String(
                        format: Self.t(
                            "已完成上下文压缩 **`/compact`** ✅\n\n- 上下文占用从 **%.1f%%** 降至 **%.1f%%**（48.6K / 200K tokens）\n- 保留了核心需求、文件路径、改动 Diff 与验收约束，移除了冗余中间工具输出\n\n现在可以继续畅通对话了。",
                            "Compacted conversation context via **`/compact`** ✅\n\n- Context usage dropped from **%.1f%%** to **%.1f%%** (48.6K / 200K tokens)\n- Preserved key requirements, file paths, code diffs, and constraints while trimming intermediate tool noise",
                            "コンテキスト圧縮 **`/compact`** を完了しました ✅\n\n- コンテキスト使用率：**%.1f%%** → **%.1f%%**（48.6K / 200K tokens）\n- 主要要件・ファイルパス・差分・制約を保持し、冗長な中間ツール出力を整理\n\nこのままスムーズに会話を続けられます。"
                        ),
                        prevPct, newPct
                    )
                ))
            } else if trimmed == "Continue" && isLogin500 {
                let s1 = self.conversations[i].messages.count
                self.conversations[i].messages.append(Self.toolMsg(id: "\(cascadeId)-t-\(s1)", step: s1, tools: ["read_file", "edit_file"]))
                let s2 = self.conversations[i].messages.count
                self.conversations[i].messages.append(Self.agentMsg(
                    id: "\(cascadeId)-a-\(s2)",
                    step: s2,
                    model: activeModelId,
                    modelName: activeModelName,
                    text: Self.t(
                        """
                        已恢复。根据日志，500 来自 `session_store` 的连接池耗尽：高峰期登录请求并发超过了连接池上限（20）。

                        ```diff
                        - pool = create_pool(max_size=20)
                        + pool = create_pool(max_size=100, acquire_timeout=3)
                        ```

                        我已调大连接池并加上获取超时，超时会返回 503 而不是挂起。建议上线后观察 `pool_wait_ms` 指标。
                        """,
                        """
                        Recovered. Logs show the 500 errors came from `session_store` connection pool exhaustion under peak concurrent logins (pool limit: 20).

                        ```diff
                        - pool = create_pool(max_size=20)
                        + pool = create_pool(max_size=100, acquire_timeout=3)
                        ```

                        I raised the pool size to 100 and added an acquire timeout so overload fails fast with HTTP 503 instead of hanging. Monitor `pool_wait_ms` after rollout.
                        """,
                        """
                        復旧しました。ログを確認したところ、500 エラーの原因はピーク時の同時ログイン集中による `session_store` コネクションプール枯渇（上限 20）でした。

                        ```diff
                        - pool = create_pool(max_size=20)
                        + pool = create_pool(max_size=100, acquire_timeout=3)
                        ```

                        プール上限を 100 に引き上げ、取得タイムアウト（3秒）を設定してハングせず HTTP 503 で即時フェイルするよう修正しました。リリース後は `pool_wait_ms` 指標の監視をおすすめします。
                        """
                    )
                ))
                self.bumpContextMeter(at: i, addInput: 2_100, addOutput: 310, speed: 52.4, ttft: 0.58)
            } else if lower.hasPrefix("/workflow") || lower.contains("run_workflow") || lower.contains("工作流") {
                let wfSubId = "demo-subagent-live-wf-\(self.nextIdCounter)"
                self.nextIdCounter += 1
                let wfObj: [String: Any] = [
                    "description": Self.t("实时触发的多智能体并发工作流", "On-Demand Multi-Agent Parallel Workflow", "オンデマンド並列マルチエージェントワークフロー"),
                    "scriptPath": "workflows/live_pipeline.py",
                    "phases": [
                        [
                            "name": Self.t("Phase 1 · 并发采集与静态检查", "Phase 1 · Parallel Collection & Static Audit", "Phase 1 · 並列収集＆静的チェック"),
                            "status": "done",
                            "logs": [
                                "[00:00.2] spawn parallel workers (n=2)",
                                "[00:01.1] scanner finished: 0 critical issues"
                            ],
                            "subagents": [
                                [
                                    "conversationId": "demo-subagent-audit-1",
                                    "typeName": "research",
                                    "role": Self.t("依赖与静态审计官", "Dependency & Static Auditor", "依存関係・静的監査担当"),
                                    "modelTier": "flash",
                                    "status": "done",
                                    "stepCount": 4,
                                    "title": Self.t("依赖与静态审计官", "Dependency & Static Auditor", "依存関係・静的監査担当")
                                ]
                            ]
                        ],
                        [
                            "name": Self.t("Phase 2 · 方案综合与收敛", "Phase 2 · Synthesis & Verification", "Phase 2 · 統合レビュー＆検証"),
                            "status": "done",
                            "logs": ["[00:02.0] aggregated summary & emitted JSON finish contract"],
                            "subagents": [
                                [
                                    "conversationId": wfSubId,
                                    "typeName": "research",
                                    "role": Self.t("结论校验子代理", "Verification Subagent", "検証サブエージェント"),
                                    "modelTier": "pro",
                                    "status": "done",
                                    "stepCount": 3,
                                    "title": Self.t("结论校验子代理", "Verification Subagent", "検証サブエージェント")
                                ]
                            ]
                        ]
                    ],
                    "logs": [
                        "workflow engine initialized (max_parallel=4)",
                        "all phases converged cleanly in 2.3s"
                    ],
                    "output": Self.t("全部 2 个阶段已完成，无阻断风险。", "All 2 phases completed with zero blocking findings.", "全 2 フェーズが完了し、ブロッキングリスクはありません。"),
                    "resultJson": "{\"ok\":true,\"phases\":2,\"findings\":0,\"recommendedNextStep\":\"ready_to_commit\"}",
                    "status": "done",
                    "duration": "2.3s"
                ]
                let s1 = self.conversations[i].messages.count
                self.conversations[i].messages.append(Message(
                    id: "\(cascadeId)-wf-\(s1)",
                    type: "workflow",
                    text: "",
                    toolCount: 1,
                    toolNames: ["run_workflow"],
                    stepIndex: s1,
                    workflow: wfObj
                ))
                let s2 = self.conversations[i].messages.count
                self.conversations[i].messages.append(Self.agentMsg(
                    id: "\(cascadeId)-a-\(s2)",
                    step: s2,
                    model: activeModelId,
                    modelName: activeModelName,
                    text: Self.t("动态工作流已执行完毕，详见上方工作流编排卡片中的阶段日志与 JSON 产物。", "Dynamic workflow completed. Inspect phase logs and JSON artifacts in the workflow card above.", "動的ワークフローの実行が完了しました。上のワークフローカードから各フェーズのログと JSON 成果物を確認できます。")
                ))
                self.bumpContextMeter(at: i, addInput: 4_800, addOutput: 520, speed: 58.2, ttft: 0.55)
            } else {
                let s1 = self.conversations[i].messages.count
                self.conversations[i].messages.append(Self.toolMsg(id: "\(cascadeId)-t-\(s1)", step: s1, tools: ["read_file", "grep_search"]))
                let s2 = self.conversations[i].messages.count
                let reply = self.demoReply(for: promptText, attachmentNames: attachmentNames, hasMedia: !(imageUrls?.isEmpty ?? true))
                self.conversations[i].messages.append(Self.agentMsg(
                    id: "\(cascadeId)-a-\(s2)",
                    step: s2,
                    model: activeModelId,
                    modelName: activeModelName,
                    text: reply
                ))
                self.bumpContextMeter(at: i, addInput: 2_600, addOutput: 340, speed: 49.5, ttft: 0.64)
            }

            self.conversations[i].status = "CASCADE_RUN_STATUS_IDLE"
            self.conversations[i].duration = Self.t("2秒", "2s", "2秒")
            self.conversations[i].lastModified = Date()
        }
        return nil
    }

    private func bumpContextMeter(at index: Int, addInput: Int64, addOutput: Int64, speed: Double, ttft: Double) {
        let prev = conversations[index].contextMeter
        let maxT = prev?.maxTokens ?? 512_000
        let newUsed = min(maxT - 1_200, (prev?.usedTokens ?? 48_400) + addInput + addOutput)
        let pct = Double(newUsed) / Double(maxT) * 100.0
        conversations[index].contextMeter = ContextMeterItem(
            usedTokens: newUsed,
            maxTokens: maxT,
            percentage: pct,
            inputTokens: (prev?.inputTokens ?? 8_200) + addInput,
            outputTokens: (prev?.outputTokens ?? 1_100) + addOutput,
            cacheReadTokens: (prev?.cacheReadTokens ?? 40_000) + addInput * 3,
            thinkingTokens: (prev?.thinkingTokens ?? 620) + addOutput / 2,
            timeToFirstTokenSec: ttft,
            streamingDurationSec: max(1.2, Double(addOutput) / speed),
            tokensPerSecond: speed,
            isOverflow: pct >= 95.0,
            errorKind: pct >= 95.0 ? "context_overflow" : "none"
        )
    }

    func respond(
        cascadeId: String,
        stepIndex: Int,
        allow: Bool?,
        optionId: String?,
        writeInResponse: String? = nil,
        skipped: Bool = false,
        answers: [[String: Any]]?
    ) {
        lock.lock()
        guard let idx = conversations.firstIndex(where: { $0.id == cascadeId }) else {
            lock.unlock()
            return
        }
        let interactionType = conversations[idx].pendingInteraction?["type"] as? String ?? ""
        let pendingQuestions = conversations[idx].pendingInteraction?["questions"] as? [[String: Any]]
        let pendingOptions = conversations[idx].pendingInteraction?["options"] as? [[String: Any]]
        conversations[idx].pendingInteraction = nil
        conversations[idx].status = "CASCADE_RUN_STATUS_RUNNING"
        conversations[idx].lastModified = Date()
        let activeModelId = conversations[idx].modelId
        let activeModelName = conversations[idx].modelName
        lock.unlock()

        DispatchQueue.global().asyncAfter(deadline: .now() + 0.75) {
            self.lock.lock()
            defer { self.lock.unlock() }
            guard let i = self.conversations.firstIndex(where: { $0.id == cascadeId }) else { return }
            let trimmedWriteIn = writeInResponse?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
            if skipped || allow == false {
                let s = self.conversations[i].messages.count
                let note = trimmedWriteIn.isEmpty
                    ? Self.t("好的，已按你的选择取消/跳过该操作，工作区保持原状。", "Got it — operation skipped/cancelled. Workspace left unchanged.", "了解しました。この操作をキャンセル・スキップし、ワークスペースは変更していません。")
                    : Self.t("已收到你的说明「\(trimmedWriteIn)」，暂不执行该操作，等待你的进一步指示。", "Received your note \"\(trimmedWriteIn)\". Holding execution pending your next instruction.", "メモ「\(trimmedWriteIn)」を受け取りました。実行を保留し、次の指示をお待ちしています。")
                self.conversations[i].messages.append(Self.agentMsg(
                    id: "\(cascadeId)-a-\(s)",
                    step: s,
                    model: activeModelId,
                    modelName: activeModelName,
                    text: note
                ))
            } else if interactionType == "run_command" {
                let s1 = self.conversations[i].messages.count
                self.conversations[i].messages.append(Self.toolMsg(
                    id: "\(cascadeId)-t-\(s1)",
                    step: s1,
                    tools: ["run_command"],
                    thinking: Self.t("已获得批准，正在执行生产缓存清理与网关平滑重载，并核验健康检查端点。", "Approval received. Running TTL cleanup, issuing zero-downtime SIGHUP reload, and verifying health probes.", "承認を確認しました。期限切れキー掃除と無停止 SIGHUP リロードを実行し、ヘルスチェックを検証します。"),
                    thinkingDuration: "2.1s",
                    toolDetails: [[
                        "name": "run_command",
                        "summary": "bash scripts/prod_cleanup.sh --prefix 'v1:session:tmp:*' --reload-gateway",
                        "status": "done",
                        "duration": "1.8s",
                        "stepIndex": s1
                    ]]
                ))
                let s2 = self.conversations[i].messages.count
                self.conversations[i].messages.append(Self.agentMsg(
                    id: "\(cascadeId)-a-\(s2)",
                    step: s2,
                    model: activeModelId,
                    modelName: activeModelName,
                    text: Self.t(
                        "已获批并执行完毕 ✅\n\n- 清理 `v1:session:tmp:*` 过期键 **14,218** 条，释放 Redis 内存 **412 MB**\n- 网关已完成 `SIGHUP` 热重载，活跃 SSE / WebSocket 连接 **0 中断**\n- `/healthz` 探针返回 `200 OK`（P99 = **38ms**）",
                        "Approved and executed cleanly ✅\n\n- Purged **14,218** expired `v1:session:tmp:*` keys (**412 MB** Redis memory reclaimed)\n- Completed zero-downtime `SIGHUP` reload (**0** dropped SSE / WebSocket streams)\n- `/healthz` check passed `200 OK` (P99 = **38ms**)",
                        "承認を受けて実行完了しました ✅\n\n- `v1:session:tmp:*` の期限切れキー **14,218 件**を削除（Redis メモリ **412 MB** 解放）\n- `SIGHUP` ホットリロード完了（稼働中 SSE / WebSocket 切断 **0 件**）\n- `/healthz` 正常応答 `200 OK`（P99 = **38ms**）"
                    )
                ))
            } else if interactionType == "workflow_approval" {
                let s1 = self.conversations[i].messages.count
                self.conversations[i].messages.append(Message(
                    id: "\(cascadeId)-wf-\(s1)",
                    type: "workflow",
                    text: "",
                    toolCount: 1,
                    toolNames: ["run_workflow"],
                    stepIndex: s1,
                    workflow: [
                        "description": Self.t("灰度发布与冒烟验证流水线", "Canary Rollout & Smoke Verification Pipeline", "カナリアリリース＆スモーク検証パイプライン"),
                        "scriptPath": "deploy/canary_rollout.py",
                        "phases": [
                            [
                                "name": Self.t("构建镜像与推送", "Build & Push Image", "イメージビルド＆プッシュ"),
                                "status": "done",
                                "logs": ["docker buildx ok", "pushed tag v2.35.0-canary"],
                                "subagents": []
                            ],
                            [
                                "name": Self.t("灰度发布 10% 流量", "Canary Rollout (10% Traffic)", "カナリア配信（10% トラフィック）"),
                                "status": "done",
                                "logs": ["canary pods 2/2 ready", "error rate 0.00%"],
                                "subagents": []
                            ],
                            [
                                "name": Self.t("冒烟验证", "Smoke Verification", "スモーク検証"),
                                "status": "done",
                                "logs": ["18/18 smoke assertions passed"],
                                "subagents": []
                            ]
                        ],
                        "logs": ["workflow completed after approval"],
                        "output": Self.t("灰度实例健康检查通过，可随时全量切流。", "Canary health checks passed; ready for full traffic promotion.", "カナリア環境のヘルスチェック通過。いつでも全量切替可能です。"),
                        "resultJson": "{\"canary\":\"healthy\",\"smokePassed\":18,\"p99Ms\":42}",
                        "status": "done",
                        "duration": "6.4s"
                    ]
                ))
                let s2 = self.conversations[i].messages.count
                self.conversations[i].messages.append(Self.agentMsg(
                    id: "\(cascadeId)-a-\(s2)",
                    step: s2,
                    model: activeModelId,
                    modelName: activeModelName,
                    text: Self.t("已批准并完成灰度发布工作流！18 项冒烟断言全部通过（P99 = 42ms）。", "Approved and completed canary rollout workflow! All 18 smoke assertions passed (P99 = 42ms).", "承認を受けカナリアリリースワークフローを完了しました！18 件のスモーク検証がすべて成功（P99 = 42ms）。")
                ))
            } else {
                let summary: String
                if let answers, !answers.isEmpty {
                    var parts: [String] = []
                    for ans in answers {
                        let qIdx = ans["questionIndex"] as? Int ?? parts.count
                        let ids = ans["selectedOptionIds"] as? [String] ?? []
                        let qWriteIn = (ans["writeInResponse"] as? String ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
                        let qSkipped = ans["skipped"] as? Bool ?? false
                        if qSkipped {
                            parts.append(Self.t("- 第 \(qIdx + 1) 题：已跳过", "- Question \(qIdx + 1): Skipped", "- 質問 \(qIdx + 1)：スキップ"))
                            continue
                        }
                        var chosenTexts: [String] = []
                        if let qList = pendingQuestions, qIdx >= 0, qIdx < qList.count,
                           let opts = qList[qIdx]["options"] as? [[String: Any]] {
                            chosenTexts = ids.map { id in
                                opts.first(where: { ($0["id"] as? String) == id })?["text"] as? String ?? id
                            }
                        } else if let opts = pendingOptions {
                            chosenTexts = ids.map { id in
                                opts.first(where: { ($0["id"] as? String) == id })?["text"] as? String ?? id
                            }
                        } else {
                            chosenTexts = ids
                        }
                        if !qWriteIn.isEmpty { chosenTexts.append(Self.t("补充：\(qWriteIn)", "Note: \(qWriteIn)", "補足：\(qWriteIn)")) }
                        let sep = (Self.isZH || Self.isJA) ? "、" : ", "
                        parts.append(Self.t(
                            "- 第 \(qIdx + 1) 题：\(chosenTexts.isEmpty ? "已确认" : chosenTexts.joined(separator: sep))",
                            "- Question \(qIdx + 1): \(chosenTexts.isEmpty ? "Confirmed" : chosenTexts.joined(separator: sep))",
                            "- 質問 \(qIdx + 1)：\(chosenTexts.isEmpty ? "確認済み" : chosenTexts.joined(separator: sep))"
                        ))
                    }
                    if cascadeId == "demo-0003-migration" {
                        summary = Self.t(
                            """
                            已按你的确认方案生成最终迁移脚本与回滚预案：

                            \(parts.joined(separator: "\n"))

                            ```sql
                            -- 0019_orders_created_idx.sql（无锁并发创建 + 状态回填）
                            CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_orders_created_status
                            ON orders (created_at DESC, status)
                            WHERE deleted_at IS NULL;

                            -- 回滚脚本：DROP INDEX CONCURRENTLY IF EXISTS idx_orders_created_status;
                            ```

                            ✅ 预估锁表时间 `< 10ms`，对线上订单写入零阻塞，可直接并入本次发布单。
                            """,
                            """
                            Generated the final lock-free migration script and rollback plan based on your selections:

                            \(parts.joined(separator: "\n"))

                            ```sql
                            -- 0019_orders_created_idx.sql (lock-free concurrent index)
                            CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_orders_created_status
                            ON orders (created_at DESC, status)
                            WHERE deleted_at IS NULL;

                            -- Rollback: DROP INDEX CONCURRENTLY IF EXISTS idx_orders_created_status;
                            ```

                            ✅ Estimated lock hold `< 10ms`, zero write blocking on production orders. Ready to merge into this release.
                            """,
                            """
                            ご選択いただいた方針で最終マイグレーションSQLとロールバック手順を生成しました：

                            \(parts.joined(separator: "\n"))

                            ```sql
                            -- 0019_orders_created_idx.sql（ロックフリー並列インデックス作成）
                            CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_orders_created_status
                            ON orders (created_at DESC, status)
                            WHERE deleted_at IS NULL;

                            -- ロールバック: DROP INDEX CONCURRENTLY IF EXISTS idx_orders_created_status;
                            ```

                            ✅ 推定ロック保持時間は `< 10ms`、本番注文の書き込みブロックなし。そのまま今回リリースに組み込めます。
                            """
                        )
                    } else {
                        summary = Self.t(
                            "已收到你的确认选择：\n\n\(parts.joined(separator: "\n"))\n\n1. 已更新配置并生成变更摘要。\n2. 全部检查通过，随时可继续下一轮对话。",
                            "Confirmed your selections:\n\n\(parts.joined(separator: "\n"))\n\n1. Updated configuration and generated change summary.\n2. All checks passed; ready for your next instruction.",
                            "選択内容を確認しました：\n\n\(parts.joined(separator: "\n"))\n\n1. 設定を更新し変更サマリーを生成しました。\n2. 全チェック通過済み。続けて次の指示をどうぞ。"
                        )
                    }
                } else if !trimmedWriteIn.isEmpty {
                    summary = Self.t("已收到你的自定义回复「\(trimmedWriteIn)」，并按新要求继续执行完成 ✅。", "Received custom reply \"\(trimmedWriteIn)\" and completed execution ✅.", "カスタム指示「\(trimmedWriteIn)」を反映し、実行を完了しました ✅。")
                } else if let optionId, !optionId.isEmpty {
                    let chosenText = pendingOptions?.first(where: { ($0["id"] as? String) == optionId })?["text"] as? String ?? optionId
                    summary = Self.t("已按所选方案（\(chosenText)）执行完毕 ✅。", "Executed with selected option (\(chosenText)) ✅.", "選択された案（\(chosenText)）で実行を完了しました ✅。")
                } else {
                    summary = Self.t("已获得执行权限并顺利完成命令运行 ✅。", "Permission granted and command completed ✅.", "実行許可を確認し、コマンドを正常終了しました ✅。")
                }
                let s = self.conversations[i].messages.count
                self.conversations[i].messages.append(Self.agentMsg(
                    id: "\(cascadeId)-a-\(s)",
                    step: s,
                    model: activeModelId,
                    modelName: activeModelName,
                    text: summary
                ))
            }
            self.conversations[i].status = "CASCADE_RUN_STATUS_IDLE"
            self.conversations[i].duration = Self.t("3秒", "3s", "3秒")
            self.conversations[i].lastModified = Date()
            self.bumpContextMeter(at: i, addInput: 1_900, addOutput: 260, speed: 53.1, ttft: 0.58)
        }
    }

    func stop(cascadeId: String, killBackgroundTasks: Bool = false) {
        lock.lock()
        defer { lock.unlock() }
        guard let idx = conversations.firstIndex(where: { $0.id == cascadeId }) else { return }
        conversations[idx].status = "CASCADE_RUN_STATUS_IDLE"
        if killBackgroundTasks {
            conversations[idx].runningTasks = []
            globalDaemonTasks.removeAll { ($0["conversationId"] as? String) == cascadeId }
        }
        conversations[idx].lastModified = Date()
    }

    func stopTask(cascadeId: String, taskId: String) {
        lock.lock()
        defer { lock.unlock() }
        if let idx = conversations.firstIndex(where: { $0.id == cascadeId }) {
            conversations[idx].runningTasks.removeAll {
                ($0["id"] as? String) == taskId || ($0["taskId"] as? String) == taskId
            }
            if conversations[idx].runningTasks.isEmpty && conversations[idx].status == "CASCADE_RUN_STATUS_RUNNING" {
                conversations[idx].status = "CASCADE_RUN_STATUS_IDLE"
            }
        }
        globalDaemonTasks.removeAll {
            ($0["taskId"] as? String) == taskId || ($0["id"] as? String) == taskId
        }
    }

    func stopSubagent(conversationId: String) {
        lock.lock()
        defer { lock.unlock() }
        if let childIdx = conversations.firstIndex(where: { $0.id == conversationId }) {
            conversations[childIdx].status = "CASCADE_RUN_STATUS_IDLE"
            conversations[childIdx].runningTasks = []
        }
        for i in conversations.indices {
            conversations[i].subagents = conversations[i].subagents.map { item in
                guard (item["conversationId"] as? String) == conversationId else { return item }
                var copy = item
                copy["status"] = "done"
                return copy
            }
        }
    }

    func deleteQueuedMessage(cascadeId: String, messageId: String) {
        lock.lock()
        defer { lock.unlock() }
        guard let idx = conversations.firstIndex(where: { $0.id == cascadeId }) else { return }
        conversations[idx].queuedMessages.removeAll { ($0["id"] as? String) == messageId }
    }

    func updateAnnotations(cascadeId: String, title: String?, markedAsUnread: Bool?, pinned: Bool?) {
        lock.lock()
        defer { lock.unlock() }
        guard let idx = conversations.firstIndex(where: { $0.id == cascadeId }) else { return }
        if let title, !title.isEmpty { conversations[idx].title = title }
        if let markedAsUnread { conversations[idx].unread = markedAsUnread }
        if let pinned { conversations[idx].pinned = pinned }
    }

    func deleteConversation(cascadeId: String) {
        lock.lock()
        defer { lock.unlock() }
        conversations.removeAll { $0.id == cascadeId }
    }

    // MARK: - Changes / Revert / Git / Chapters / Search / Export

    func changesPayload(cascadeId: String) -> [String: Any] {
        lock.lock()
        defer { lock.unlock() }
        guard let conv = conversations.first(where: { $0.id == cascadeId }) else {
            return ["cascadeId": cascadeId, "files": [], "additions": 0, "deletions": 0, "hasChanges": false]
        }
        let additions = conv.changes.reduce(0) { $0 + $1.additions }
        let deletions = conv.changes.reduce(0) { $0 + $1.deletions }
        return [
            "cascadeId": conv.id,
            "files": conv.changes.map { $0.toJSON() },
            "additions": additions,
            "deletions": deletions,
            "hasChanges": !conv.changes.isEmpty
        ]
    }

    func revertPreviewPayload(cascadeId: String, stepIndex: Int) -> [String: Any] {
        lock.lock()
        defer { lock.unlock() }
        guard let conv = conversations.first(where: { $0.id == cascadeId }) else {
            return ["cascadeId": cascadeId, "stepIndex": stepIndex, "targetStepIndex": stepIndex, "files": [], "hasCodeChanges": false]
        }
        return [
            "cascadeId": conv.id,
            "stepIndex": stepIndex,
            "targetStepIndex": max(0, stepIndex),
            "files": conv.changes.map { $0.toJSON() },
            "hasCodeChanges": !conv.changes.isEmpty
        ]
    }

    func revertExecute(cascadeId: String, stepIndex: Int, conversationOnly: Bool) -> [String: Any] {
        lock.lock()
        defer { lock.unlock() }
        if let idx = conversations.firstIndex(where: { $0.id == cascadeId }) {
            let kept = conversations[idx].messages.filter { $0.stepIndex < stepIndex }
            conversations[idx].messages = kept.isEmpty ? Array(conversations[idx].messages.prefix(2)) : kept
            conversations[idx].status = "CASCADE_RUN_STATUS_IDLE"
            conversations[idx].pendingInteraction = nil
            if !conversationOnly {
                conversations[idx].changes = []
            }
            conversations[idx].lastModified = Date()
        }
        return ["ok": true, "cascadeId": cascadeId, "stepIndex": stepIndex]
    }

    func gitStatusPayload(cascadeId: String) -> [String: Any]? {
        lock.lock()
        defer { lock.unlock() }
        return conversations.first(where: { $0.id == cascadeId })?.gitStatus?.toJSON()
    }

    func gitCommit(cascadeId: String, message: String, paths: [String], push: Bool) -> [String: Any] {
        lock.lock()
        defer { lock.unlock() }
        let sha = String(UUID().uuidString.replacingOccurrences(of: "-", with: "").prefix(7)).lowercased()
        if let idx = conversations.firstIndex(where: { $0.id == cascadeId }), var git = conversations[idx].gitStatus {
            if paths.isEmpty {
                git.files.removeAll()
            } else {
                let targetSet = Set(paths)
                git.files.removeAll { targetSet.contains(($0["path"] as? String) ?? "") }
            }
            git.ahead = push ? 0 : (git.ahead + 1)
            conversations[idx].gitStatus = git
            conversations[idx].lastModified = Date()
        }
        return [
            "committed": true,
            "pushed": push,
            "commitId": sha,
            "output": "[demo-branch \(sha)] \(message)\n \(max(1, paths.count)) file(s) changed"
        ]
    }

    func gitPush(cascadeId: String) -> [String: Any] {
        lock.lock()
        defer { lock.unlock() }
        if let idx = conversations.firstIndex(where: { $0.id == cascadeId }), var git = conversations[idx].gitStatus {
            git.ahead = 0
            conversations[idx].gitStatus = git
        }
        return ["pushed": true, "output": "To origin (demo): everything up-to-date"]
    }

    func chaptersPayload(cascadeId: String, ranges: [[String: Any]]) -> [String: Any] {
        lock.lock()
        defer { lock.unlock() }
        guard let conv = conversations.first(where: { $0.id == cascadeId }) else {
            return ["chapters": []]
        }
        if !conv.chapters.isEmpty && ranges.isEmpty {
            return ["chapters": conv.chapters]
        }
        var out: [[String: Any]] = []
        for (i, r) in ranges.enumerated() {
            let s = (r["startStepIndex"] as? Int) ?? 0
            let e = (r["endStepIndexExclusive"] as? Int) ?? s + 1
            if let preset = conv.chapters.first(where: { ($0["startStepIndex"] as? Int) == s }) {
                out.append(preset)
            } else {
                let firstUser = conv.messages.first(where: { $0.stepIndex >= s && $0.stepIndex < e && $0.type == "user" })?.text ?? Self.t("阶段 \(i + 1)", "Phase \(i + 1)", "フェーズ \(i + 1)")
                let shortTitle = String(firstUser.replacingOccurrences(of: "\n", with: " ").prefix(22))
                out.append([
                    "startStepIndex": s,
                    "endStepIndexExclusive": e,
                    "title": shortTitle.isEmpty ? Self.t("第 \(i + 1) 轮对话", "Turn \(i + 1)", "ターン \(i + 1)") : shortTitle,
                    "generatedAt": Self.iso(Date())
                ])
            }
        }
        return ["chapters": out]
    }

    func searchConversationsPayload(query: String) -> [String: Any] {
        lock.lock()
        defer { lock.unlock() }
        let q = query.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !q.isEmpty else { return ["results": []] }
        var results: [[String: Any]] = []

        for conv in conversations where !conv.isSubagent {
            if let range = conv.title.range(of: q, options: .caseInsensitive) {
                let sOff = conv.title.unicodeScalars.distance(from: conv.title.unicodeScalars.startIndex, to: range.lowerBound)
                let eOff = conv.title.unicodeScalars.distance(from: conv.title.unicodeScalars.startIndex, to: range.upperBound)
                results.append([
                    "cascadeId": conv.id,
                    "title": conv.title,
                    "workspaceName": conv.workspaceName,
                    "lastModifiedTime": Self.iso(conv.lastModified),
                    "snippet": conv.title,
                    "snippetMatchRanges": [["startOffset": sOff, "endOffsetExclusive": eOff]],
                    "matchSource": "title",
                    "matchedStepIndex": 0
                ])
                continue
            }
            for msg in conv.messages where !msg.text.isEmpty {
                if let range = msg.text.range(of: q, options: .caseInsensitive) {
                    let prefixStart = msg.text.index(range.lowerBound, offsetBy: -24, limitedBy: msg.text.startIndex) ?? msg.text.startIndex
                    let suffixEnd = msg.text.index(range.upperBound, offsetBy: 60, limitedBy: msg.text.endIndex) ?? msg.text.endIndex
                    let snippet = String(msg.text[prefixStart..<suffixEnd]).replacingOccurrences(of: "\n", with: " ")
                    var matchRanges: [[String: Any]] = []
                    if let sRange = snippet.range(of: q, options: .caseInsensitive) {
                        let sOff = snippet.unicodeScalars.distance(from: snippet.unicodeScalars.startIndex, to: sRange.lowerBound)
                        let eOff = snippet.unicodeScalars.distance(from: snippet.unicodeScalars.startIndex, to: sRange.upperBound)
                        matchRanges.append(["startOffset": sOff, "endOffsetExclusive": eOff])
                    }
                    results.append([
                        "cascadeId": conv.id,
                        "title": conv.title,
                        "workspaceName": conv.workspaceName,
                        "lastModifiedTime": Self.iso(conv.lastModified),
                        "snippet": snippet,
                        "snippetMatchRanges": matchRanges,
                        "matchSource": msg.type == "user" ? "user" : "agent",
                        "matchedStepIndex": msg.stepIndex
                    ])
                    break
                }
            }
        }
        return ["results": results]
    }

    func exportMarkdownPayload(cascadeId: String) -> [String: Any] {
        lock.lock()
        defer { lock.unlock() }
        guard let conv = conversations.first(where: { $0.id == cascadeId }) else {
            return ["markdown": Self.t("# 会话导出\n\n（未找到会话内容）", "# Conversation Export\n\n(No content found)", "# 会話エクスポート\n\n（会話内容が見つかりません）")]
        }
        var lines: [String] = [
            "# \(conv.title)",
            "",
            "- **\(Self.t("工作区", "Workspace", "ワークスペース"))**: \(conv.workspaceName)",
            "- **\(Self.t("模型", "Model", "モデル"))**: \(conv.modelName)",
            "- **\(Self.t("发起时间", "Started At", "開始時刻"))**: \(conv.startedAt)",
            ""
        ]
        for msg in conv.messages {
            switch msg.type {
            case "user":
                lines.append("## User (Step \(msg.stepIndex))")
                lines.append("")
                lines.append(msg.text)
                lines.append("")
            case "agent":
                let tag = msg.modelName.map { " (\($0))" } ?? ""
                lines.append("## Assistant\(tag)")
                lines.append("")
                lines.append(msg.text)
                lines.append("")
            case "workflow":
                lines.append("> **[Workflow]** \((msg.workflow?["description"] as? String) ?? Self.t("动态工作流", "Dynamic Workflow", "動的ワークフロー"))")
                lines.append("")
            case "tools":
                let names = (msg.toolNames ?? []).joined(separator: ", ")
                lines.append("> **[Tools x\(msg.toolCount ?? 1)]** \(names)")
                lines.append("")
            case "error":
                lines.append("> **[Error]** \(msg.text)")
                lines.append("")
            default:
                break
            }
        }
        return ["markdown": lines.joined(separator: "\n")]
    }

    // MARK: - Swarm

    func swarmSnapshotPayload(cascadeId: String) -> [String: Any] {
        lock.lock()
        defer { lock.unlock() }
        if let swarm = conversations.first(where: { $0.id == cascadeId })?.swarm {
            return swarm
        }
        return [
            "cascadeId": cascadeId,
            "coordinatorId": cascadeId,
            "title": Self.t("蜂群协同", "Agent Swarm", "エージェントスウォーム"),
            "brief": ["active": false, "coordinatorId": cascadeId, "title": Self.t("蜂群协同", "Agent Swarm", "エージェントスウォーム"), "phase": "", "headline": "", "workingCount": 0, "totalMembers": 0, "latestSeq": 0, "attentionCount": 0],
            "members": [],
            "messages": []
        ]
    }

    func sendSwarmFeedback(cascadeId: String, feedback: String) -> [String: Any] {
        lock.lock()
        defer { lock.unlock() }
        let trimmed = feedback.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else { return ["ok": true] }
        if let idx = conversations.firstIndex(where: { $0.id == cascadeId }), var swarm = conversations[idx].swarm {
            nextSwarmSeq += 1
            let seq = nextSwarmSeq
            var messages = (swarm["messages"] as? [[String: Any]]) ?? []
            messages.append([
                "seq": seq,
                "agent": "user",
                "text": trimmed,
                "time": Self.iso(Date()),
                "channel": "feedback",
                "mentions": ["coordinator"]
            ])
            nextSwarmSeq += 1
            messages.append([
                "seq": nextSwarmSeq,
                "agent": "coordinator",
                "text": Self.t("@reviewer @tester 已同步用户反馈「\(String(trimmed.prefix(36)))」，优先核查并汇总结果。", "@reviewer @tester Synced user feedback \"\(String(trimmed.prefix(36)))\" — please verify and report back.", "@reviewer @tester ユーザーフィードバック「\(String(trimmed.prefix(36)))」を共有しました。優先的に確認して報告してください。"),
                "time": Self.iso(Date()),
                "channel": "board",
                "mentions": ["reviewer", "tester"]
            ])
            swarm["messages"] = messages
            if var brief = swarm["brief"] as? [String: Any] {
                brief["latestSeq"] = nextSwarmSeq
                swarm["brief"] = brief
            }
            conversations[idx].swarm = swarm
        }
        return ["ok": true]
    }

    func nudgeSwarmMember(cascadeId: String, workerName: String) -> [String: Any] {
        lock.lock()
        defer { lock.unlock() }
        if let idx = conversations.firstIndex(where: { $0.id == cascadeId }), var swarm = conversations[idx].swarm {
            nextSwarmSeq += 1
            var messages = (swarm["messages"] as? [[String: Any]]) ?? []
            messages.append([
                "seq": nextSwarmSeq,
                "agent": "coordinator",
                "text": Self.t("@\(workerName) 收到前端轻推（nudge），请回报当前阶段进展与阻塞项。", "@\(workerName) Nudge received from mobile — please report current progress and blockers.", "@\(workerName) モバイルからリマインド（nudge）を受信しました。現在の進捗とブロッカーを報告してください。"),
                "time": Self.iso(Date()),
                "channel": "nudge",
                "mentions": [workerName]
            ])
            var members = (swarm["members"] as? [[String: Any]]) ?? []
            for mIdx in members.indices where (members[mIdx]["name"] as? String) == workerName {
                members[mIdx]["lastNudgedSeq"] = nextSwarmSeq
                members[mIdx]["lastActivity"] = Self.t("刚刚轻推唤醒", "Nudged just now", "たった今リマインド送信")
                if (members[mIdx]["status"] as? String) == "waiting" {
                    members[mIdx]["status"] = "working"
                }
            }
            swarm["messages"] = messages
            swarm["members"] = members
            if var brief = swarm["brief"] as? [String: Any] {
                brief["latestSeq"] = nextSwarmSeq
                brief["workingCount"] = members.filter { ($0["status"] as? String) == "working" }.count
                swarm["brief"] = brief
            }
            conversations[idx].swarm = swarm
        }
        return ["ok": true]
    }

    // MARK: - MCP, Skills, Plugins, Automations, Background Tasks, Workflow Templates

    func mcpStatesPayload() -> Data {
        lock.lock()
        let resp = GetMcpServerStatesResponse(states: mcpServers)
        lock.unlock()
        return (try? JSONEncoder().encode(resp)) ?? Data("{\"states\":[]}".utf8)
    }

    func skillsPayload() -> Data {
        lock.lock()
        let resp = GetAllSkillsResponse(skills: skills)
        lock.unlock()
        return (try? JSONEncoder().encode(resp)) ?? Data("{\"skills\":[]}".utf8)
    }

    func pluginsPayload() -> [String: Any] {
        lock.lock()
        defer { lock.unlock() }
        return ["plugins": plugins, "providers": pluginProviders]
    }

    func installPlugin(pluginId: String) -> [String: Any] {
        lock.lock()
        defer { lock.unlock() }
        for i in plugins.indices where (plugins[i]["id"] as? String) == pluginId {
            plugins[i]["installed"] = true
            plugins[i]["enabled"] = true
            plugins[i]["authStatus"] = "authenticated"
        }
        return [
            "status": "ok",
            "success": true,
            "pluginId": pluginId,
            "installed": true,
            "resumed": true,
            "message": Self.t("演示模式已模拟安装插件 \(pluginId)", "Simulated plugin install for \(pluginId) in Demo Mode", "デモモードでプラグイン \(pluginId) のインストールをシミュレートしました")
        ]
    }

    func automationsPayload() -> [String: Any] {
        lock.lock()
        defer { lock.unlock() }
        return ["automations": automations, "events": automationEvents]
    }

    func manageAutomation(sidecarId: String, action: String) -> [String: Any] {
        lock.lock()
        defer { lock.unlock() }
        let nowIso = Self.iso(Date())
        for i in automations.indices where (automations[i]["id"] as? String) == sidecarId {
            switch action {
            case "stop":
                automations[i]["status"] = "STOPPED"
                automations[i]["enabled"] = false
            case "start":
                automations[i]["status"] = "RUNNING"
                automations[i]["enabled"] = true
                automations[i]["nextRunAt"] = Self.iso(Date().addingTimeInterval(3600))
            case "run_now":
                automations[i]["lastRunAt"] = nowIso
                let linkedConvId = conversations.first(where: { !$0.isSubagent })?.id ?? "demo-0001-upload"
                automationEvents.insert([
                    "id": "evt-\(UUID().uuidString.prefix(6))",
                    "scheduleId": automations[i]["name"] as? String ?? sidecarId,
                    "conversationId": linkedConvId,
                    "timestamp": nowIso,
                    "status": "SUCCESS",
                    "message": Self.t("手动触发执行完成：检查通过 4 项，生成简报摘要。", "Manual trigger completed: 4 checks passed and summary generated.", "手動トリガー実行完了：4 項目のチェックを通過しサマリーを生成しました。")
                ], at: 0)
            default:
                break
            }
        }
        return ["ok": true]
    }

    func workflowTemplatesPayload() -> Data {
        lock.lock()
        let resp = WorkflowTemplatesResponse(templates: workflowTemplates, updatedAt: Self.iso(Date()))
        lock.unlock()
        return (try? JSONEncoder().encode(resp)) ?? Data("{\"templates\":[]}".utf8)
    }

    func saveWorkflowTemplates(data: Data) -> Data {
        struct Payload: Decodable { let templates: [WorkflowTemplateRecord] }
        if let decoded = try? JSONDecoder().decode(Payload.self, from: data) {
            lock.lock()
            workflowTemplates = WorkflowTemplateRecord.partitioned(decoded.templates)
            let resp = WorkflowTemplatesResponse(templates: workflowTemplates, updatedAt: Self.iso(Date()))
            lock.unlock()
            return (try? JSONEncoder().encode(resp)) ?? Data("{\"templates\":[]}".utf8)
        }
        return workflowTemplatesPayload()
    }

    func resetWorkflowTemplates() -> Data {
        lock.lock()
        workflowTemplates = Self.defaultDemoWorkflowTemplates()
        let resp = WorkflowTemplatesResponse(templates: workflowTemplates, updatedAt: Self.iso(Date()))
        lock.unlock()
        return (try? JSONEncoder().encode(resp)) ?? Data("{\"templates\":[]}".utf8)
    }

    static func defaultDemoWorkflowTemplates() -> [WorkflowTemplateRecord] {
        var list = WorkflowTemplateDefaults.all
        if let first = list.first {
            list[0] = WorkflowTemplateRecord(
                id: first.id,
                title: first.title,
                subtitle: first.subtitle,
                icon: first.icon,
                color: first.color,
                templatePrompt: first.templatePrompt,
                inQuickMenu: true
            )
        }
        list.insert(
            WorkflowTemplateRecord(
                id: "fullstack_diagnose",
                title: Self.t("全栈故障排查与修复", "Full-Stack Incident Triage & Fix", "フルスタック障害切り分け＆修正"),
                subtitle: Self.t("并发分析网关日志、前端请求链路与单测回归，给出最小修复补丁", "Concurrently inspect gateway logs, client request traces, and regressions to propose a minimal patch", "ゲートウェイログ・クライアント通信・単体テストを並列調査し最小修正パッチを提案"),
                icon: "bolt",
                color: "orange",
                templatePrompt: Self.t(
                    "/workflow 请使用 run_workflow 编排并发排查：Worker-1 分析服务端日志与超时链路，Worker-2 验证接口契约与单测，汇总修复方案。",
                    "/workflow Orchestrate parallel diagnostics with run_workflow: Worker-1 inspects server logs & timeout paths, Worker-2 verifies API contracts & unit tests, then synthesize a minimal fix.",
                    "/workflow run_workflow で並列調査を実行：Worker-1 がサーバーログとタイムアウト経路を解析し、Worker-2 が API 契約と単体テストを検証して最小修正案をまとめて。"
                ),
                inQuickMenu: true
            ),
            at: 1
        )
        return WorkflowTemplateRecord.partitioned(list)
    }

    func globalBackgroundTasksPayload() -> [String: Any] {
        lock.lock()
        defer { lock.unlock() }
        return ["tasks": globalDaemonTasks]
    }

    func backgroundTaskLogPayload(conversationId: String, taskId: String) -> [String: Any] {
        lock.lock()
        defer { lock.unlock() }
        let content = taskLogs[taskId]
            ?? taskLogs["\(conversationId):\(taskId)"]
            ?? "[\(Self.iso(Date()))] task \(taskId) running normally...\nListening on loopback demo worker\n0 warnings, 0 errors."
        return [
            "taskId": taskId,
            "logUri": "demo://logs/\(taskId).log",
            "content": content,
            "truncated": false,
            "sizeBytes": Int64(content.utf8.count)
        ]
    }

    // MARK: - Accounts & Files

    func refreshQuotas() {
        lock.lock()
        defer { lock.unlock() }
        for i in accounts.indices {
            if accounts[i].tier.uppercased() != "FREE" {
                accounts[i].claude5h = min(100.0, accounts[i].claude5h + 1.5)
                accounts[i].gemini5h = min(100.0, accounts[i].gemini5h + 1.5)
            }
        }
    }

    func switchAccount(to target: String) {
        lock.lock()
        defer { lock.unlock() }
        if let acc = accounts.first(where: { $0.id == target || $0.email == target }) {
            activeAccountId = acc.id
        }
    }

    func storeUpload(name: String, data: Data) -> (status: Int, json: [String: Any]) {
        lock.lock()
        defer { lock.unlock() }
        let safe = (name as NSString).lastPathComponent
        let cleanName = safe.isEmpty ? "upload-\(UUID().uuidString.prefix(6)).bin" : safe
        let id = Self.fileID(for: data)
        let path = "\(Self.inboxPath)/\(id)-\(cleanName)"
        let stored = StoredFile(id: id, name: cleanName, data: data, path: path)
        files[path] = stored
        let ext = (cleanName as NSString).pathExtension.lowercased()
        let isImage = ["png", "jpg", "jpeg", "webp", "gif", "heic"].contains(ext)
        let mime: String = {
            switch ext {
            case "png": return "image/png"
            case "jpg", "jpeg": return "image/jpeg"
            case "webp": return "image/webp"
            case "gif": return "image/gif"
            case "pdf": return "application/pdf"
            case "csv": return "text/csv"
            case "md": return "text/markdown"
            case "json", "ipynb": return "application/json"
            default: return "application/octet-stream"
            }
        }()
        let digest = SHA256.hash(data: data).map { String(format: "%02x", $0) }.joined()
        let line = Self.attachmentLine(path: path, name: cleanName, size: Int64(data.count))
        return (200, [
            "id": id,
            "path": path,
            "name": cleanName,
            "size": data.count,
            "mime": mime,
            "sha256": digest,
            "kind": isImage ? "image" : "file",
            "line": line
        ])
    }

    static func fileID(for data: Data) -> String {
        let digest = SHA256.hash(data: data)
        return digest.prefix(4).map { String(format: "%02x", $0) }.joined()
    }

    static func humanSize(_ n: Int64) -> String {
        if n < 1024 { return "\(n) B" }
        let kb = Double(n) / 1024.0
        if kb < 1024 { return String(format: "%.1f KB", kb) }
        return String(format: "%.2f MB", kb / 1024.0)
    }

    static func attachmentLine(path: String, name: String, size: Int64) -> String {
        "- [\(name)](\(path)) (\(humanSize(size)))"
    }

    func fileData(forPath rawPath: String) -> (data: Data, name: String)? {
        lock.lock()
        defer { lock.unlock() }
        var normalized = rawPath.trimmingCharacters(in: .whitespacesAndNewlines)
        if let comps = URLComponents(string: normalized),
           let q = comps.queryItems?.first(where: { $0.name == "uri" || $0.name == "path" })?.value {
            normalized = q
        }
        if normalized.hasPrefix("file://") {
            if let url = URL(string: normalized) {
                normalized = url.path
            } else {
                normalized = String(normalized.dropFirst("file://".count))
            }
        }
        normalized = normalized.removingPercentEncoding ?? normalized
        let key = normalized.split(separator: "/").last.map(String.init) ?? normalized
        if let f = files[normalized] ?? files[rawPath] {
            return (f.data, f.name)
        }
        if let match = files.values.first(where: { $0.id == key || $0.name == key || $0.path == normalized }) {
            return (match.data, match.name)
        }
        return nil
    }

    // MARK: - Replies & Message Builders

    private func demoReply(for prompt: String, attachmentNames: [String], hasMedia: Bool) -> String {
        if hasMedia || !attachmentNames.isEmpty {
            let sep = (Self.isZH || Self.isJA) ? "、" : ", "
            let fileSummary = attachmentNames.isEmpty
                ? Self.t("随消息发送的图片 / 文件", "attached image / file", "添付された画像 / ファイル")
                : attachmentNames.map { "`\($0)`" }.joined(separator: sep)
            return Self.t(
                "已收到你上传的附件（\(fileSummary)，演示模式模拟分析）：\n\n- 已完成格式解析、字节校验与内容结构提取\n- 如需继续分析或导出报告，可直接追问下一步要求",
                "Received your attachment (\(fileSummary)) in Demo Mode:\n\n- Parsed format headers, validated byte integrity, and extracted structure\n- Ask a follow-up question to analyze further or export a report",
                "添付ファイル（\(fileSummary)）を受信しました（デモモード模擬解析）：\n\n- フォーマット解析・整合性チェック・構造抽出を完了\n- 追加の分析やレポート出力が必要ならそのまま指示してください"
            )
        }
        return Self.t(
            "这是**演示模式**下的本地模拟回复（未连接真实电脑网关）：\n\n已收到你的消息：「\(String(prompt.prefix(60)))」。\n\n你可以继续体验：\n- 左滑或长按消息触发 **撤回 / 导出 Markdown**\n- 在输入框键入 **`/`** 呼出斜杠命令与技能，或长按 **Workflow** 胶囊体验快捷模板\n- 点击右上角导轨切换 **章节 TOC / 轮次跳转**，或点顶部 **Changes / Commit** 查看代码与 PR",
            "This is an offline simulated reply in **Demo Mode** (no desktop gateway connected):\n\nReceived your prompt: \"\(String(prompt.prefix(60)))\".\n\nTry exploring:\n- Long-press messages to **Export Markdown** or **Copy**\n- Type **`/`** in the composer for slash commands & skills, or open **Workflow** templates\n- Tap the outline rail in the top-right to jump between **TOC Chapters / Turns**, or tap **Changes / Commit** to inspect diffs & PRs",
            "これは **デモモード** のローカル模擬応答です（実機 PC ゲートウェイ未接続）：\n\n受信したプロンプト：「\(String(prompt.prefix(60)))」。\n\n以下の機能もお試しいただけます：\n- メッセージ長押しで **Markdown エクスポート / コピー**\n- 入力欄で **`/`** を打ってスラッシュコマンド・スキルを呼び出し、または **Workflow** テンプレートを実行\n- 右上ガイドから **目次（TOC）/ ターンジャンプ**、上部の **Changes / Commit** で差分・PR を確認"
        )
    }

    static func iso(_ date: Date) -> String {
        let f = ISO8601DateFormatter()
        f.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return f.string(from: date)
    }

    static func userMsg(
        id: String,
        step: Int,
        text: String,
        imageUrls: [String]? = nil,
        timestamp: String? = nil,
        slashCommands: [[String: Any]]? = nil
    ) -> Message {
        Message(
            id: id,
            type: "user",
            text: text,
            imageUrls: imageUrls,
            stepIndex: step,
            timestamp: timestamp,
            slashCommands: slashCommands
        )
    }

    static func agentMsg(
        id: String,
        step: Int,
        model: String? = "gemini-4-neon-high",
        modelName: String? = "Gemini 4 Neon (High)",
        text: String,
        artifacts: [[String: Any]]? = nil,
        timestamp: String? = nil
    ) -> Message {
        Message(
            id: id,
            type: "agent",
            text: text,
            artifacts: artifacts,
            stepIndex: step,
            model: model,
            modelName: modelName,
            timestamp: timestamp
        )
    }

    static func toolMsg(
        id: String,
        step: Int,
        tools: [String],
        thinking: String? = nil,
        thinkingDuration: String? = nil,
        toolDetails: [[String: Any]]? = nil,
        focusText: String? = nil,
        startedAt: String? = nil
    ) -> Message {
        Message(
            id: id,
            type: "tools",
            text: "",
            thinking: thinking,
            thinkingDuration: thinkingDuration,
            toolCount: tools.count,
            toolNames: tools,
            toolDetails: toolDetails,
            stepIndex: step,
            focusText: focusText,
            startedAt: startedAt
        )
    }

    static func stepPlanMsg(
        id: String,
        step: Int,
        items: [[String: Any]]
    ) -> Message {
        let doneCount = items.filter { ($0["status"] as? String) == "done" }.count
        let skippedCount = items.filter { ($0["status"] as? String) == "skipped" }.count
        let failedCount = items.filter { ($0["status"] as? String) == "failed" }.count
        let planDict: [String: Any] = [
            "anchorStepIndex": step,
            "items": items,
            "doneCount": doneCount,
            "totalCount": max(0, items.count - skippedCount),
            "skippedCount": skippedCount,
            "failedCount": failedCount
        ]
        return Message(
            id: id,
            type: "step_plan",
            text: "",
            stepIndex: step,
            stepPlan: planDict
        )
    }

    static func errorMsg(
        id: String,
        step: Int,
        text: String,
        attempts: Int,
        max: Int,
        errorKind: String? = nil,
        isOverflow: Bool? = nil
    ) -> Message {
        Message(
            id: id,
            type: "error",
            text: text,
            stepIndex: step,
            attemptCount: attempts,
            maxAttempts: max,
            errorKind: errorKind,
            isOverflow: isOverflow
        )
    }

    static func subagentMsg(
        id: String,
        step: Int,
        conversationId: String,
        role: String,
        typeName: String = "research",
        prompt: String,
        modelTier: String = "flash",
        status: String = "done",
        stepCount: Int = 5
    ) -> Message {
        Message(
            id: id,
            type: "subagent",
            text: "",
            stepIndex: step,
            subagent: [
                "conversationId": conversationId,
                "typeName": typeName,
                "role": role,
                "prompt": prompt,
                "modelTier": modelTier,
                "stepIndex": step,
                "status": status,
                "stepCount": stepCount,
                "title": role
            ]
        )
    }

    static func mime(for name: String) -> String {
        let ext = (name as NSString).pathExtension
        if let ut = UTType(filenameExtension: ext), let m = ut.preferredMIMEType {
            return m
        }
        return "application/octet-stream"
    }
}
