import Foundation

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
