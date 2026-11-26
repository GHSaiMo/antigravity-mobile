import Foundation

private struct P4ErrorBody: Decodable {
    let error: String?
}

private struct P4OkResponse: Decodable {
    let ok: Bool?
    let message: String?
    let error: String?
}

extension APIClient {
    private func p4Get<Resp: Decodable>(pathAndQuery: String, baseURL: URL, timeout: TimeInterval = 15) async throws -> Resp {
        let baseStr = baseURL.absoluteString.trimmingCharacters(in: CharacterSet(charactersIn: "/"))
        let cleanRel = pathAndQuery.trimmingCharacters(in: CharacterSet(charactersIn: "/"))
        guard let url = URL(string: "\(baseStr)/\(cleanRel)") else {
            throw APIError.invalidURL
        }
        var request = URLRequest(url: url)
        request.httpMethod = "GET"
        request.timeoutInterval = timeout

        let reqEpoch = GatewayRuntimeState.shared.epoch
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
            if http.statusCode == 401 { postTokenRevokedIfCurrentEpoch(reqEpoch) }
            let msg = (try? Self.sharedDecoder.decode(P4ErrorBody.self, from: data))?.error
                ?? String(data: data, encoding: .utf8)
                ?? "HTTP \(http.statusCode)"
            throw APIError.serverError(statusCode: http.statusCode, message: msg)
        }
        return try Self.sharedDecoder.decode(Resp.self, from: data)
    }

    private func p4PostJSON<Resp: Decodable>(
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

        let reqEpoch = GatewayRuntimeState.shared.epoch
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
            if http.statusCode == 401 { postTokenRevokedIfCurrentEpoch(reqEpoch) }
            let msg = (try? Self.sharedDecoder.decode(P4ErrorBody.self, from: data))?.error
                ?? String(data: data, encoding: .utf8)
                ?? "HTTP \(http.statusCode)"
            throw APIError.serverError(statusCode: http.statusCode, message: msg)
        }
        return try Self.sharedDecoder.decode(Resp.self, from: data)
    }

    // MARK: - 1. BrowserViewerService

    public func fetchBrowserTabs(cascadeId: String?, baseURL: URL) async throws -> BrowserTabsResponse {
        var path = "gateway/browser/tabs"
        if let cid = cascadeId?.trimmingCharacters(in: .whitespacesAndNewlines), !cid.isEmpty,
           let enc = cid.addingPercentEncoding(withAllowedCharacters: .urlQueryAllowed) {
            path += "?cascadeId=\(enc)"
        }
        return try await p4Get(pathAndQuery: path, baseURL: baseURL, timeout: 10)
    }

    public func openBrowserView(cascadeId: String?, tabId: String?, baseURL: URL) async throws -> BrowserOpenViewResponse {
        var payload: [String: JSONValue] = [:]
        if let cid = cascadeId, !cid.isEmpty { payload["cascadeId"] = .string(cid) }
        if let tid = tabId, !tid.isEmpty { payload["tabId"] = .string(tid) }
        return try await p4PostJSON(path: "gateway/browser/open", payload: payload, baseURL: baseURL, timeout: 15)
    }

    public func readBrowserFrame(
        cascadeId: String?,
        sessionId: String?,
        tabId: String?,
        maxWidthPx: Int = 960,
        maxHeightPx: Int = 1440,
        baseURL: URL
    ) async throws -> BrowserFrameResponse {
        var payload: [String: JSONValue] = [
            "maxWidthPx": .int(maxWidthPx),
            "maxHeightPx": .int(maxHeightPx)
        ]
        if let cid = cascadeId, !cid.isEmpty { payload["cascadeId"] = .string(cid) }
        if let sid = sessionId, !sid.isEmpty { payload["sessionId"] = .string(sid) }
        if let tid = tabId, !tid.isEmpty { payload["tabId"] = .string(tid) }
        return try await p4PostJSON(path: "gateway/browser/frame", payload: payload, baseURL: baseURL, timeout: 12)
    }

    public func takeBrowserControl(cascadeId: String?, sessionId: String?, tabId: String?, baseURL: URL) async throws -> BrowserTakeoverResponse {
        var payload: [String: JSONValue] = [:]
        if let cid = cascadeId, !cid.isEmpty { payload["cascadeId"] = .string(cid) }
        if let sid = sessionId, !sid.isEmpty { payload["sessionId"] = .string(sid) }
        if let tid = tabId, !tid.isEmpty { payload["tabId"] = .string(tid) }
        return try await p4PostJSON(path: "gateway/browser/takeover", payload: payload, baseURL: baseURL, timeout: 12)
    }

    public func controlBrowserTab(
        cascadeId: String?,
        sessionId: String?,
        tabId: String?,
        controlToken: String?,
        action: String,
        url: String? = nil,
        inputEvent: [String: JSONValue]? = nil,
        inputEvents: [[String: JSONValue]]? = nil,
        baseURL: URL
    ) async throws -> BrowserControlResponse {
        var payload: [String: JSONValue] = ["action": .string(action)]
        if let cid = cascadeId, !cid.isEmpty { payload["cascadeId"] = .string(cid) }
        if let sid = sessionId, !sid.isEmpty { payload["sessionId"] = .string(sid) }
        if let tid = tabId, !tid.isEmpty { payload["tabId"] = .string(tid) }
        if let tok = controlToken, !tok.isEmpty { payload["controlToken"] = .string(tok) }
        if let url, !url.isEmpty { payload["url"] = .string(url) }
        if let inputEvent { payload["inputEvent"] = .object(inputEvent) }
        if let inputEvents { payload["inputEvents"] = .array(inputEvents.map { .object($0) }) }
        return try await p4PostJSON(path: "gateway/browser/control", payload: payload, baseURL: baseURL, timeout: 12)
    }

    public func releaseBrowserControl(cascadeId: String?, sessionId: String?, tabId: String?, controlToken: String?, baseURL: URL) async throws {
        var payload: [String: JSONValue] = [:]
        if let cid = cascadeId, !cid.isEmpty { payload["cascadeId"] = .string(cid) }
        if let sid = sessionId, !sid.isEmpty { payload["sessionId"] = .string(sid) }
        if let tid = tabId, !tid.isEmpty { payload["tabId"] = .string(tid) }
        if let tok = controlToken, !tok.isEmpty { payload["controlToken"] = .string(tok) }
        let _: P4OkResponse = try await p4PostJSON(path: "gateway/browser/release", payload: payload, baseURL: baseURL, timeout: 10)
    }

    public func renewBrowserView(sessionId: String, baseURL: URL) async throws {
        let _: P4OkResponse = try await p4PostJSON(path: "gateway/browser/renew", payload: ["sessionId": .string(sessionId)], baseURL: baseURL, timeout: 10)
    }

    public func closeBrowserView(sessionId: String, baseURL: URL) async throws {
        let _: P4OkResponse = try await p4PostJSON(path: "gateway/browser/close", payload: ["sessionId": .string(sessionId)], baseURL: baseURL, timeout: 8)
    }

    // MARK: - 2. RemotingService (SSH Remote Hosts)

    public func fetchRemotes(baseURL: URL) async throws -> RemotesListResponse {
        try await p4Get(pathAndQuery: "gateway/remotes", baseURL: baseURL, timeout: 12)
    }

    public func connectRemote(name: String, host: String, user: String = "", port: Int = 22, baseURL: URL) async throws -> RemoteConnectJob {
        var payload: [String: JSONValue] = [
            "name": .string(name),
            "host": .string(host),
            "port": .int(port)
        ]
        if !user.isEmpty { payload["user"] = .string(user) }
        let resp: RemoteConnectJobResponse = try await p4PostJSON(path: "gateway/remotes/connect", payload: payload, baseURL: baseURL, timeout: 15)
        guard let job = resp.job else {
            throw APIError.serverError(statusCode: 500, message: resp.error ?? String(localized: "启动 SSH 连接失败"))
        }
        return job
    }

    public func fetchRemoteConnectStatus(jobId: String, baseURL: URL) async throws -> RemoteConnectJob {
        let enc = jobId.addingPercentEncoding(withAllowedCharacters: .urlQueryAllowed) ?? jobId
        let resp: RemoteConnectJobResponse = try await p4Get(pathAndQuery: "gateway/remotes/connect/status?jobId=\(enc)", baseURL: baseURL, timeout: 10)
        guard let job = resp.job else {
            throw APIError.serverError(statusCode: 404, message: resp.error ?? String(localized: "任务不存在"))
        }
        return job
    }

    public func respondToRemotePrompt(jobId: String?, remoteId: String?, response: String, baseURL: URL) async throws {
        var payload: [String: JSONValue] = ["response": .string(response)]
        if let jobId, !jobId.isEmpty { payload["jobId"] = .string(jobId) }
        if let remoteId, !remoteId.isEmpty { payload["remoteId"] = .string(remoteId) }
        let _: P4OkResponse = try await p4PostJSON(path: "gateway/remotes/prompt", payload: payload, baseURL: baseURL, timeout: 12)
    }

    public func disconnectRemote(remoteId: String, baseURL: URL) async throws {
        let _: P4OkResponse = try await p4PostJSON(path: "gateway/remotes/disconnect", payload: ["remoteId": .string(remoteId)], baseURL: baseURL, timeout: 12)
    }

    public func saveRemote(name: String, host: String, user: String = "", port: Int = 22, description: String = "", baseURL: URL) async throws {
        var payload: [String: JSONValue] = [
            "name": .string(name),
            "host": .string(host),
            "port": .int(port)
        ]
        if !user.isEmpty { payload["user"] = .string(user) }
        if !description.isEmpty { payload["description"] = .string(description) }
        let _: P4OkResponse = try await p4PostJSON(path: "gateway/remotes/save", payload: payload, baseURL: baseURL, timeout: 12)
    }

    public func deleteRemote(remoteId: String, baseURL: URL) async throws {
        let _: P4OkResponse = try await p4PostJSON(path: "gateway/remotes/delete", payload: ["remoteId": .string(remoteId)], baseURL: baseURL, timeout: 12)
    }

    // MARK: - 3. Marketplace, Plugin Uninstall & Plugin / MCP OAuth

    public func searchMarketplace(query: String, baseURL: URL) async throws -> [MarketplaceItem] {
        let q = query.trimmingCharacters(in: .whitespacesAndNewlines)
        let resp: MarketplaceSearchResponse
        if q.isEmpty {
            resp = try await p4Get(pathAndQuery: "gateway/marketplace/featured", baseURL: baseURL, timeout: 15)
        } else {
            let enc = q.addingPercentEncoding(withAllowedCharacters: .urlQueryAllowed) ?? q
            resp = try await p4Get(pathAndQuery: "gateway/marketplace/search?q=\(enc)", baseURL: baseURL, timeout: 15)
        }
        return resp.items ?? []
    }

    public func deletePlugin(pluginId: String, baseURL: URL) async throws {
        let _: P4OkResponse = try await p4PostJSON(path: "gateway/plugins/delete", payload: ["pluginId": .string(pluginId)], baseURL: baseURL, timeout: 15)
    }

    public func startPluginAuthSignIn(plugin: String, provider: String, accountId: String? = nil, addAccount: Bool = false, baseURL: URL) async throws -> OAuthStartResponse {
        var payload: [String: JSONValue] = [
            "plugin": .string(plugin),
            "provider": .string(provider),
            "addAccount": .bool(addAccount)
        ]
        if let accountId, !accountId.isEmpty { payload["accountId"] = .string(accountId) }
        return try await p4PostJSON(path: "gateway/plugins/auth/start", payload: payload, baseURL: baseURL, timeout: 15)
    }

    public func signOutPluginAuth(plugin: String, provider: String, accountId: String? = nil, baseURL: URL) async throws {
        var payload: [String: JSONValue] = [
            "plugin": .string(plugin),
            "provider": .string(provider)
        ]
        if let accountId, !accountId.isEmpty { payload["accountId"] = .string(accountId) }
        let _: P4OkResponse = try await p4PostJSON(path: "gateway/plugins/auth/signout", payload: payload, baseURL: baseURL, timeout: 15)
    }

    public func setDefaultPluginAuthAccount(plugin: String, provider: String, accountId: String, baseURL: URL) async throws {
        let _: P4OkResponse = try await p4PostJSON(
            path: "gateway/plugins/auth/default",
            payload: [
                "plugin": .string(plugin),
                "provider": .string(provider),
                "accountId": .string(accountId)
            ],
            baseURL: baseURL,
            timeout: 15
        )
    }

    public func startMcpOAuth(serverName: String, baseURL: URL) async throws -> OAuthStartResponse {
        try await p4PostJSON(path: "gateway/mcp/oauth/start", payload: ["serverName": .string(serverName)], baseURL: baseURL, timeout: 15)
    }

    public func completeMcpOAuth(serverName: String, authorizationCode: String, baseURL: URL) async throws {
        let _: P4OkResponse = try await p4PostJSON(
            path: "gateway/mcp/oauth/complete",
            payload: [
                "serverName": .string(serverName),
                "authorizationCode": .string(authorizationCode)
            ],
            baseURL: baseURL,
            timeout: 15
        )
    }

    public func disconnectMcpOAuth(serverName: String, account: String? = nil, baseURL: URL) async throws {
        var payload: [String: JSONValue] = ["serverName": .string(serverName)]
        if let account, !account.isEmpty { payload["account"] = .string(account) }
        let _: P4OkResponse = try await p4PostJSON(path: "gateway/mcp/oauth/disconnect", payload: payload, baseURL: baseURL, timeout: 15)
    }

    public func forwardLoopbackOAuthCallback(callbackUrl: String, baseURL: URL) async throws -> OAuthCallbackForwardResponse {
        try await p4PostJSON(
            path: "gateway/oauth/callback-forward",
            payload: ["callbackUrl": .string(callbackUrl)],
            baseURL: baseURL,
            timeout: 15
        )
    }

    // MARK: - 4. Desktop Self-Update & Changelog

    public func fetchDesktopUpdateStatus(baseURL: URL) async throws -> DesktopUpdateStatus {
        try await p4Get(pathAndQuery: "gateway/desktop/update", baseURL: baseURL, timeout: 10)
    }

    public func fetchDesktopChangelog(version: String?, baseURL: URL) async throws -> DesktopChangelogResponse {
        var path = "gateway/desktop/changelog"
        if let v = version?.trimmingCharacters(in: .whitespacesAndNewlines), !v.isEmpty,
           let enc = v.addingPercentEncoding(withAllowedCharacters: .urlQueryAllowed) {
            path += "?version=\(enc)"
        }
        return try await p4Get(pathAndQuery: path, baseURL: baseURL, timeout: 10)
    }

    public func applyDesktopUpdate(baseURL: URL) async throws -> DesktopApplyUpdateResponse {
        try await p4PostJSON(path: "gateway/desktop/update/apply", payload: ["confirm": .bool(true)], baseURL: baseURL, timeout: 15)
    }

    // MARK: - 5. Workspace Code & File Search

    public func searchWorkspaceCode(
        cascadeId: String,
        query: String,
        isRegex: Bool = false,
        caseSensitive: Bool = false,
        baseURL: URL
    ) async throws -> [CodeSearchHit] {
        let resp: CodeSearchResponse = try await p4PostJSON(
            path: "gateway/search/code",
            payload: [
                "cascadeId": .string(cascadeId),
                "query": .string(query),
                "isRegex": .bool(isRegex),
                "caseSensitive": .bool(caseSensitive),
                "maxResults": .int(60)
            ],
            baseURL: baseURL,
            timeout: 15
        )
        return resp.results ?? []
    }

    public func searchWorkspaceFiles(cascadeId: String, query: String, baseURL: URL) async throws -> [FileSearchHit] {
        let resp: FileSearchResponse = try await p4PostJSON(
            path: "gateway/search/files",
            payload: [
                "cascadeId": .string(cascadeId),
                "query": .string(query),
                "maxResults": .int(60)
            ],
            baseURL: baseURL,
            timeout: 15
        )
        return resp.results ?? []
    }

    // MARK: - 6. Artifact Version Snapshots

    public func fetchArtifactSnapshots(cascadeId: String, uri: String?, artifactName: String?, baseURL: URL) async throws -> [ArtifactSnapshotItem] {
        var queryItems: [String] = []
        if let enc = cascadeId.addingPercentEncoding(withAllowedCharacters: .urlQueryAllowed) {
            queryItems.append("cascadeId=\(enc)")
        }
        if let uri, !uri.isEmpty, let enc = uri.addingPercentEncoding(withAllowedCharacters: .urlQueryAllowed) {
            queryItems.append("uri=\(enc)")
        }
        if let artifactName, !artifactName.isEmpty, let enc = artifactName.addingPercentEncoding(withAllowedCharacters: .urlQueryAllowed) {
            queryItems.append("artifactName=\(enc)")
        }
        let path = "gateway/artifacts/snapshots?\(queryItems.joined(separator: "&"))"
        let resp: ArtifactSnapshotsResponse = try await p4Get(pathAndQuery: path, baseURL: baseURL, timeout: 12)
        return resp.snapshots ?? []
    }

    // MARK: - 7. Worktree PR & Conversation Star

    public func updateWorktreePR(cascadeId: String, title: String, body: String, commitMessage: String, baseURL: URL) async throws -> WorktreePRResponse {
        var payload: [String: JSONValue] = ["cascadeId": .string(cascadeId)]
        if !title.isEmpty { payload["title"] = .string(title) }
        if !body.isEmpty { payload["body"] = .string(body) }
        if !commitMessage.isEmpty { payload["commitMessage"] = .string(commitMessage) }
        return try await p4PostJSON(path: "gateway/worktree/pr", payload: payload, baseURL: baseURL, timeout: 65)
    }

    public func setConversationStarred(cascadeId: String, starred: Bool, baseURL: URL) async throws {
        let _: StarCascadeResponse = try await p4PostJSON(
            path: "gateway/cascade/star",
            payload: [
                "cascadeId": .string(cascadeId),
                "starred": .bool(starred)
            ],
            baseURL: baseURL,
            timeout: 10
        )
    }
}
