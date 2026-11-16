import Foundation

private struct P3GatewayErrorBody: Decodable {
    let error: String?
}

private struct P3OkResponse: Decodable {
    let ok: Bool?
    let error: String?
}

private struct P3TerminalCreateResponse: Decodable {
    let ok: Bool?
    let terminalId: String?
}

extension APIClient {
    // MARK: - Phase 3 Gateway HTTP & WebSocket Helpers

    private func p3Get<Resp: Decodable>(pathAndQuery: String, baseURL: URL, timeout: TimeInterval = 15) async throws -> Resp {
        guard let url = URL(string: pathAndQuery, relativeTo: baseURL) ?? URL(string: baseURL.absoluteString.trimmingCharacters(in: CharacterSet(charactersIn: "/")) + "/" + pathAndQuery.trimmingCharacters(in: CharacterSet(charactersIn: "/"))) else {
            throw APIError.invalidURL
        }
        var request = URLRequest(url: url)
        request.httpMethod = "GET"
        request.timeoutInterval = timeout

        let (data, response): (Data, URLResponse)
        do {
            (data, response) = try await transport.send(request: request)
        } catch {
            throw APIError.networkError(error.localizedDescription)
        }
        guard let http = response as? HTTPURLResponse else {
            throw APIError.networkError("Invalid response type")
        }
        guard (200...299).contains(http.statusCode) else {
            if http.statusCode == 401 {
                NotificationCenter.default.post(name: .deviceTokenRevoked, object: nil)
            }
            let msg = (try? Self.sharedDecoder.decode(P3GatewayErrorBody.self, from: data))?.error
                ?? String(data: data, encoding: .utf8)
                ?? "HTTP \(http.statusCode)"
            throw APIError.serverError(statusCode: http.statusCode, message: msg)
        }
        return try Self.sharedDecoder.decode(Resp.self, from: data)
    }

    private func p3PostJSON<Resp: Decodable>(
        path: String,
        payload: [String: JSONValue],
        baseURL: URL,
        timeout: TimeInterval = 25
    ) async throws -> Resp {
        let endpoint = baseURL.appendingPathComponent(path.trimmingCharacters(in: CharacterSet(charactersIn: "/")))
        var request = URLRequest(url: endpoint)
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.httpBody = try Self.sharedEncoder.encode(payload)
        request.timeoutInterval = timeout

        let (data, response): (Data, URLResponse)
        do {
            (data, response) = try await transport.send(request: request)
        } catch {
            throw APIError.networkError(error.localizedDescription)
        }
        guard let http = response as? HTTPURLResponse else {
            throw APIError.networkError("Invalid response type")
        }
        guard (200...299).contains(http.statusCode) else {
            if http.statusCode == 401 {
                NotificationCenter.default.post(name: .deviceTokenRevoked, object: nil)
            }
            let msg = (try? Self.sharedDecoder.decode(P3GatewayErrorBody.self, from: data))?.error
                ?? String(data: data, encoding: .utf8)
                ?? "HTTP \(http.statusCode)"
            throw APIError.serverError(statusCode: http.statusCode, message: msg)
        }
        return try Self.sharedDecoder.decode(Resp.self, from: data)
    }

    public func authorizedWebSocketRequest(pathAndQuery: String, baseURL: URL) -> URLRequest? {
        var components = URLComponents(url: baseURL, resolvingAgainstBaseURL: false)
        components?.scheme = (baseURL.scheme?.lowercased() == "https") ? "wss" : "ws"
        let cleanPath = pathAndQuery.split(separator: "?", maxSplits: 1, omittingEmptySubsequences: false)
        let pathPart = String(cleanPath.first ?? "")
        let existingPath = components?.path.trimmingCharacters(in: CharacterSet(charactersIn: "/")) ?? ""
        let relPath = pathPart.trimmingCharacters(in: CharacterSet(charactersIn: "/"))
        components?.path = "/" + [existingPath, relPath].filter { !$0.isEmpty }.joined(separator: "/")
        if cleanPath.count == 2 {
            components?.percentEncodedQuery = String(cleanPath[1])
        }
        guard let wsURL = components?.url else { return nil }
        var req = URLRequest(url: wsURL)
        req.timeoutInterval = 15
        let hostStr = (wsURL.host ?? "").lowercased()
        if CloudflareAnycastAccelerator.isCloudflareTunnelHost(hostStr) {
            req.setValue(hostStr, forHTTPHeaderField: "Host")
        }
        let token = KeychainHelper.shared.read(key: .deviceToken) ?? AppSettings.shared.deviceToken
        if let token, !token.isEmpty {
            req.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
            req.setValue(token, forHTTPHeaderField: "x-device-token")
        }
        return req
    }

    // MARK: - @ Context Mentions

    public func fetchContextMentions(
        query: String,
        cascadeId: String,
        workspaceUri: String?,
        baseURL: URL
    ) async throws -> [ContextMentionItem] {
        var payload: [String: JSONValue] = [
            "query": .string(query),
            "cascadeId": .string(cascadeId),
            "limit": .int(25)
        ]
        if let workspaceUri, !workspaceUri.isEmpty {
            payload["workspaceUri"] = .string(workspaceUri)
        }
        let resp: ContextMentionsResponse = try await p3PostJSON(
            path: "gateway/context/mentions",
            payload: payload,
            baseURL: baseURL
        )
        return resp.items ?? []
    }

    // MARK: - Fork / Worktree / Clone

    public func forkConversation(
        cascadeId: String,
        stepIndex: Int,
        createWorktree: Bool,
        baseURL: URL
    ) async throws -> ForkConversationResponse {
        try await p3PostJSON(
            path: "gateway/cascade/fork",
            payload: [
                "cascadeId": .string(cascadeId),
                "stepIndex": .int(stepIndex),
                "createWorktree": .bool(createWorktree)
            ],
            baseURL: baseURL
        )
    }

    public func createWorktreeSession(
        repoUri: String,
        workspacePath: String,
        startSession: Bool = true,
        baseURL: URL
    ) async throws -> WorktreeCreateResponse {
        try await p3PostJSON(
            path: "gateway/worktree/create",
            payload: [
                "repoUri": .string(repoUri),
                "workspacePath": .string(workspacePath),
                "startSession": .bool(startSession)
            ],
            baseURL: baseURL,
            timeout: 45
        )
    }

    public func cloneGitHubRepo(
        repoSlug: String,
        startSession: Bool = true,
        baseURL: URL
    ) async throws -> CloneRepoResponse {
        try await p3PostJSON(
            path: "gateway/projects/clone",
            payload: [
                "repoSlug": .string(repoSlug),
                "startSession": .bool(startSession)
            ],
            baseURL: baseURL,
            timeout: 120
        )
    }

    // MARK: - Permissions

    public func fetchPermissionSettings(baseURL: URL) async throws -> PermissionStateResponse {
        try await p3Get(pathAndQuery: "gateway/permissions", baseURL: baseURL)
    }

    public func updatePermissionPreset(preset: String, baseURL: URL) async throws -> PermissionStateResponse {
        try await p3PostJSON(
            path: "gateway/permissions",
            payload: ["preset": .string(preset)],
            baseURL: baseURL
        )
    }

    // MARK: - Agent Memories

    public func fetchMemories(workspacePath: String? = nil, baseURL: URL) async throws -> [MemoryRuleItem] {
        var path = "gateway/memories"
        if let workspacePath, !workspacePath.isEmpty,
           let encoded = workspacePath.addingPercentEncoding(withAllowedCharacters: .urlQueryAllowed) {
            path += "?workspacePath=\(encoded)"
        }
        let resp: MemoriesResponse = try await p3Get(pathAndQuery: path, baseURL: baseURL)
        return resp.items ?? []
    }

    public func upsertMemory(
        scope: String,
        title: String,
        content: String,
        path: String? = nil,
        workspacePath: String? = nil,
        baseURL: URL
    ) async throws -> [MemoryRuleItem] {
        var payload: [String: JSONValue] = [
            "action": .string("upsert"),
            "scope": .string(scope),
            "title": .string(title),
            "content": .string(content)
        ]
        if let path, !path.isEmpty { payload["path"] = .string(path) }
        if let workspacePath, !workspacePath.isEmpty { payload["workspacePath"] = .string(workspacePath) }
        let resp: MemoriesResponse = try await p3PostJSON(
            path: "gateway/memories",
            payload: payload,
            baseURL: baseURL
        )
        return resp.items ?? []
    }

    public func deleteMemory(path: String, baseURL: URL) async throws -> [MemoryRuleItem] {
        let resp: MemoriesResponse = try await p3PostJSON(
            path: "gateway/memories",
            payload: [
                "action": .string("delete"),
                "path": .string(path)
            ],
            baseURL: baseURL
        )
        return resp.items ?? []
    }

    // MARK: - Terminals & Background Tasks

    public func listTerminals(baseURL: URL) async throws -> [TerminalItem] {
        let resp: TerminalsResponse = try await p3Get(pathAndQuery: "gateway/terminals", baseURL: baseURL)
        return resp.terminals ?? []
    }

    public func createTerminal(baseURL: URL) async throws -> TerminalItem {
        let resp: P3TerminalCreateResponse = try await p3PostJSON(
            path: "gateway/terminal/create",
            payload: [:],
            baseURL: baseURL
        )
        let tid = resp.terminalId ?? ""
        return TerminalItem(terminalId: tid, rawId: tid, name: "Terminal", title: nil, command: nil, status: "running")
    }

    public func sendTerminalInput(terminalId: String, input: String, conversationId: String = "", baseURL: URL) async throws {
        var payload: [String: JSONValue] = [
            "terminalId": .string(terminalId),
            "input": .string(input)
        ]
        if !conversationId.isEmpty {
            payload["conversationId"] = .string(conversationId)
        }
        let _: P3OkResponse = try await p3PostJSON(
            path: "gateway/terminal/input",
            payload: payload,
            baseURL: baseURL
        )
    }

    public func closeTerminal(terminalId: String, baseURL: URL) async throws {
        let _: P3OkResponse = try await p3PostJSON(
            path: "gateway/terminal/close",
            payload: ["terminalId": .string(terminalId)],
            baseURL: baseURL
        )
    }

    public func sendStepsToBackground(cascadeId: String, stepIndices: [Int], baseURL: URL) async throws -> Bool {
        let resp: P3OkResponse = try await p3PostJSON(
            path: "gateway/cascade/task/background",
            payload: [
                "cascadeId": .string(cascadeId),
                "stepIndices": .array(stepIndices.map { .int($0) })
            ],
            baseURL: baseURL
        )
        return resp.ok ?? false
    }

    // MARK: - Subagents & Browser

    public func sendSubagentMessage(recipient: String, content: String, baseURL: URL) async throws {
        let _: P3OkResponse = try await p3PostJSON(
            path: "gateway/subagent/message",
            payload: [
                "recipient": .string(recipient),
                "content": .string(content)
            ],
            baseURL: baseURL
        )
    }

    public func fetchBrowserSubagent(cascadeId: String, baseURL: URL) async throws -> BrowserSubagentResponse {
        let encoded = cascadeId.addingPercentEncoding(withAllowedCharacters: .urlQueryAllowed) ?? cascadeId
        return try await p3Get(pathAndQuery: "gateway/subagent/browser?cascadeId=\(encoded)", baseURL: baseURL)
    }

    public func skipBrowserSubagent(cascadeId: String, baseURL: URL) async throws {
        let _: P3OkResponse = try await p3PostJSON(
            path: "gateway/subagent/browser/skip",
            payload: ["cascadeId": .string(cascadeId)],
            baseURL: baseURL
        )
    }

    public func whitelistBrowserUrl(url: String, baseURL: URL) async throws {
        let _: P3OkResponse = try await p3PostJSON(
            path: "gateway/subagent/browser/whitelist",
            payload: ["url": .string(url)],
            baseURL: baseURL
        )
    }

    // MARK: - MCP & MCP Apps

    public func toggleMcpServer(serverName: String, enabled: Bool, baseURL: URL) async throws {
        let _: P3OkResponse = try await p3PostJSON(
            path: "gateway/mcp/toggle",
            payload: [
                "serverName": .string(serverName),
                "enabled": .bool(enabled)
            ],
            baseURL: baseURL
        )
    }

    public func refreshMcpServers(baseURL: URL) async throws {
        let _: P3OkResponse = try await p3PostJSON(
            path: "gateway/mcp/refresh",
            payload: [:],
            baseURL: baseURL,
            timeout: 30
        )
    }

    public func callMcpAppTool(
        toolName: String,
        arguments: JSONValue?,
        appUri: String = "",
        baseURL: URL
    ) async throws -> String {
        var payload: [String: JSONValue] = [
            "toolName": .string(toolName),
            "arguments": arguments ?? .object([:])
        ]
        if !appUri.isEmpty {
            payload["appUri"] = .string(appUri)
        }
        let endpoint = baseURL.appendingPathComponent("gateway/mcp/app/call")
        var request = URLRequest(url: endpoint)
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.httpBody = try Self.sharedEncoder.encode(payload)
        request.timeoutInterval = 30
        let (data, response) = try await transport.send(request: request)
        guard let http = response as? HTTPURLResponse, (200...299).contains(http.statusCode) else {
            let code = (response as? HTTPURLResponse)?.statusCode ?? -1
            let text = String(data: data, encoding: .utf8) ?? "HTTP \(code)"
            throw APIError.serverError(statusCode: code, message: text)
        }
        return String(data: data, encoding: .utf8) ?? "{}"
    }

    // MARK: - Automations CRUD

    public func createAutomation(name: String, schedule: String, prompt: String, workspace: String = "", baseURL: URL) async throws {
        var payload: [String: JSONValue] = [
            "name": .string(name),
            "schedule": .string(schedule),
            "prompt": .string(prompt)
        ]
        if !workspace.isEmpty { payload["workspace"] = .string(workspace) }
        let _: P3OkResponse = try await p3PostJSON(path: "gateway/automations/create", payload: payload, baseURL: baseURL)
    }

    public func deleteAutomation(name: String, baseURL: URL) async throws {
        let _: P3OkResponse = try await p3PostJSON(
            path: "gateway/automations/delete",
            payload: ["name": .string(name)],
            baseURL: baseURL
        )
    }

    // MARK: - Git Branches & Discard

    public func gitListBranches(cascadeId: String, baseURL: URL) async throws -> GitBranchesResponse {
        try await p3PostJSON(
            path: "gateway/git/branches",
            payload: ["cascadeId": .string(cascadeId)],
            baseURL: baseURL
        )
    }

    public func gitCheckoutBranch(cascadeId: String, branch: String, create: Bool = false, baseURL: URL) async throws -> GitBranchesResponse {
        try await p3PostJSON(
            path: "gateway/git/checkout",
            payload: [
                "cascadeId": .string(cascadeId),
                "branch": .string(branch),
                "create": .bool(create)
            ],
            baseURL: baseURL
        )
    }

    public func gitDiscardFile(cascadeId: String, path: String, baseURL: URL) async throws {
        let _: P3OkResponse = try await p3PostJSON(
            path: "gateway/git/discard",
            payload: [
                "cascadeId": .string(cascadeId),
                "path": .string(path)
            ],
            baseURL: baseURL
        )
    }

    // MARK: - Narration Synthesis

    public func synthesizeNarrationAudio(text: String, baseURL: URL) async throws -> Data {
        let endpoint = baseURL.appendingPathComponent("gateway/narration")
        var request = URLRequest(url: endpoint)
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.httpBody = try Self.sharedEncoder.encode(["text": text])
        request.timeoutInterval = 30
        let (data, response) = try await transport.send(request: request)
        guard let http = response as? HTTPURLResponse, (200...299).contains(http.statusCode), !data.isEmpty else {
            let code = (response as? HTTPURLResponse)?.statusCode ?? -1
            throw APIError.serverError(statusCode: code, message: String(localized: "语音合成失败"))
        }
        return data
    }
}
