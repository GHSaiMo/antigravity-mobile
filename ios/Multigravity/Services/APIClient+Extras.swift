import Foundation

private struct EmptyBody: Encodable {}

/// 网关统一错误体 {"error": "..."}。
private struct GatewayErrorBody: Decodable { let error: String? }

extension APIClient {
    // MARK: - 改动 diff / Git / 搜索 / 导出

    /// POST JSON 到网关的非 RPC 接口，失败时优先取网关 {"error": "..."} 里的说明。
    private func gatewayPost<Req: Encodable, Resp: Decodable>(
        path: String,
        body: Req,
        baseURL: URL,
        timeout: TimeInterval = 20,
        failurePrefix: String
    ) async throws -> Resp {
        var request = URLRequest(url: baseURL.appendingPathComponent(path))
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.httpBody = try JSONEncoder().encode(body)
        request.timeoutInterval = timeout

        let data: Data
        let response: URLResponse
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
            let msg = (try? JSONDecoder().decode(GatewayErrorBody.self, from: data))?.error
                ?? String(data: data, encoding: .utf8)
                ?? "HTTP \(http.statusCode)"
            throw APIError.serverError(statusCode: http.statusCode, message: "\(failurePrefix): \(msg)")
        }
        do {
            return try JSONDecoder().decode(Resp.self, from: data)
        } catch {
            throw APIError.decodingError(error.localizedDescription)
        }
    }

    /// 可选模型目录（网关从 language_server 实时取，失败时返回内置兜底）。
    public func fetchModels(baseURL: URL, refresh: Bool = false) async throws -> ModelsResponse {
        var components = URLComponents(url: baseURL.appendingPathComponent("gateway/models"), resolvingAgainstBaseURL: false)
        if refresh { components?.queryItems = [URLQueryItem(name: "refresh", value: "1")] }
        guard let url = components?.url else { throw APIError.invalidURL }
        var request = URLRequest(url: url)
        request.httpMethod = "GET"
        request.timeoutInterval = 15
        let data: Data
        let response: URLResponse
        do {
            (data, response) = try await transport.send(request: request)
        } catch {
            throw APIError.networkError(error.localizedDescription)
        }
        guard let http = response as? HTTPURLResponse, (200...299).contains(http.statusCode) else {
            let code = (response as? HTTPURLResponse)?.statusCode ?? -1
            throw APIError.serverError(statusCode: code, message: "获取模型列表失败")
        }
        do {
            return try JSONDecoder().decode(ModelsResponse.self, from: data)
        } catch {
            throw APIError.decodingError(error.localizedDescription)
        }
    }

    /// 刷新网关的升级自检结果（GET /gateway/status 的 compat 字段）。失败时保持上一次的结果不变。
    @MainActor
    public func refreshGatewayCompat(baseURL: URL) async {
        var request = URLRequest(url: baseURL.appendingPathComponent("gateway/status"))
        request.httpMethod = "GET"
        request.timeoutInterval = 8
        guard let (data, response) = try? await transport.send(request: request),
              let http = response as? HTTPURLResponse, (200...299).contains(http.statusCode) else { return }
        GatewayCompatStore.shared.update(GatewayCompat.fromStatusData(data))
    }

    /// 手机端 "/" 菜单（系统命令 + 技能）。
    public func fetchSlashCommands(baseURL: URL) async throws -> [SlashCommandOption] {
        let resp: SlashCommandsResponse = try await gatewayPost(
            path: "gateway/slash-commands",
            body: EmptyBody(),
            baseURL: baseURL,
            failurePrefix: "获取斜杠命令失败"
        )
        return resp.commands
    }

    public func fetchCascadeChanges(cascadeId: String, fromStepIndex: Int? = nil, baseURL: URL) async throws -> CascadeChangesResponse {
        try await gatewayPost(
            path: "gateway/cascade/changes",
            body: CascadeChangesRequest(cascadeId: cascadeId, fromStepIndex: fromStepIndex),
            baseURL: baseURL,
            timeout: 30,
            failurePrefix: "获取本会话改动失败"
        )
    }

    public func fetchGitStatus(cascadeId: String, baseURL: URL) async throws -> GitStatusResponse {
        try await gatewayPost(
            path: "gateway/git/status",
            body: GitCascadeRequest(cascadeId: cascadeId),
            baseURL: baseURL,
            failurePrefix: "获取 Git 状态失败"
        )
    }

    public func gitCommit(cascadeId: String, message: String, paths: [String], push: Bool, baseURL: URL) async throws -> GitCommitResponse {
        try await gatewayPost(
            path: "gateway/git/commit",
            body: GitCommitRequest(cascadeId: cascadeId, message: message, paths: paths, push: push),
            baseURL: baseURL,
            timeout: push ? 150 : 70,
            failurePrefix: "提交失败"
        )
    }

    public func gitPush(cascadeId: String, baseURL: URL) async throws -> GitCommitResponse {
        try await gatewayPost(
            path: "gateway/git/push",
            body: GitCascadeRequest(cascadeId: cascadeId),
            baseURL: baseURL,
            timeout: 150,
            failurePrefix: "推送失败"
        )
    }

    /// 会话内容全文搜索（language_server SearchConversations，经网关透传）。
    public func searchConversations(query: String, baseURL: URL) async throws -> [ConversationSearchResult] {
        let resp: ConversationSearchResponse = try await rpc(
            method: "SearchConversations",
            body: ConversationSearchRequest(query: query),
            baseURL: baseURL
        )
        return resp.results
    }

    /// 导出会话为 Markdown（language_server ConvertTrajectoryToMarkdown，经网关透传）。
    public func exportConversationMarkdown(cascadeId: String, baseURL: URL) async throws -> String {
        let resp: MarkdownExportResponse = try await rpc(
            method: "ConvertTrajectoryToMarkdown",
            body: MarkdownExportRequest(conversationId: cascadeId),
            baseURL: baseURL
        )
        return resp.markdown
    }
}
