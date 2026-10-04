import Foundation

extension APIClient {
    // Fetch discovered upstream projects
    public func fetchProjects(baseURL: URL) async throws -> [ProjectItem] {
        let endpoint = baseURL.appendingPathComponent("gateway/projects")
        var request = URLRequest(url: endpoint)
        request.httpMethod = "GET"
        request.timeoutInterval = 10
        
        let (data, response) = try await transport.send(request: request)
        guard let httpResp = response as? HTTPURLResponse else {
            throw APIError.networkError("Invalid response type")
        }
        
        guard (200...299).contains(httpResp.statusCode) else {
            throw APIError.serverError(statusCode: httpResp.statusCode, message: String(data: data, encoding: .utf8) ?? "")
        }
        
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .custom { decoder in
            let container = try decoder.singleValueContainer()
            let dateStr = try container.decode(String.self)
            let isoFormatter = ISO8601DateFormatter()
            isoFormatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
            if let date = isoFormatter.date(from: dateStr) {
                return date
            }
            let stdFormatter = ISO8601DateFormatter()
            if let date = stdFormatter.date(from: dateStr) {
                return date
            }
            throw DecodingError.dataCorruptedError(in: container, debugDescription: "Invalid date: \(dateStr)")
        }
        
        let projects = try decoder.decode([ProjectItem].self, from: data)
        if AppSettings.shared.gatewayPlatform == nil {
            for item in projects {
                let p = item.path.isEmpty ? item.uri : item.path
                if p.contains(":\\") || p.contains(":/") || (p.count >= 2 && p.dropFirst().first == ":") {
                    AppSettings.shared.gatewayPlatform = "windows"
                    break
                } else if p.hasPrefix("/Users/") {
                    AppSettings.shared.gatewayPlatform = "darwin"
                    break
                } else if p.hasPrefix("/home/") {
                    AppSettings.shared.gatewayPlatform = "linux"
                    break
                }
            }
        }
        return projects
    }
    
    /// Set or clear (empty alias) the custom display alias of a workspace.
    public func updateProjectAlias(path: String, alias: String, baseURL: URL) async throws {
        let endpoint = baseURL.appendingPathComponent("gateway/projects/alias")
        var request = URLRequest(url: endpoint)
        request.httpMethod = "POST"
        request.timeoutInterval = 10
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        
        struct Payload: Encodable {
            let path: String
            let alias: String
        }
        request.httpBody = try JSONEncoder().encode(Payload(
            path: path.trimmingCharacters(in: .whitespacesAndNewlines),
            alias: alias.trimmingCharacters(in: .whitespacesAndNewlines)
        ))
        
        let (data, response) = try await transport.send(request: request)
        guard let httpResp = response as? HTTPURLResponse else {
            throw APIError.networkError("Invalid response type")
        }
        guard (200...299).contains(httpResp.statusCode) else {
            throw APIError.serverError(statusCode: httpResp.statusCode, message: String(data: data, encoding: .utf8) ?? "")
        }
    }
    
    // Create a new cascade and optionally send initial prompt
    public func createCascade(
        workspaceUri: String,
        prompt: String,
        model: String? = nil,
        projectId: String? = nil,
        clientMessageId: String? = nil,
        baseURL: URL
    ) async throws -> String {
        let endpoint = baseURL.appendingPathComponent("gateway/cascade/new")
        var request = URLRequest(url: endpoint)
        request.httpMethod = "POST"
        request.timeoutInterval = 15
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        if let cmid = clientMessageId, !cmid.isEmpty {
            request.setValue(cmid, forHTTPHeaderField: "X-Client-Message-Id")
        }
        
        struct Payload: Encodable {
            let workspaceUri: String
            let prompt: String
            let model: String?
            let projectId: String?
        }
        
        request.httpBody = try JSONEncoder().encode(Payload(
            workspaceUri: workspaceUri,
            prompt: prompt,
            model: model,
            projectId: projectId
        ))
        
        let (data, response) = try await transport.send(request: request)
        guard let httpResp = response as? HTTPURLResponse else {
            throw APIError.networkError("Invalid response type")
        }
        
        guard (200...299).contains(httpResp.statusCode) else {
            let msg = String(data: data, encoding: .utf8) ?? "HTTP \(httpResp.statusCode)"
            throw APIError.serverError(statusCode: httpResp.statusCode, message: msg)
        }
        
        let res = try JSONDecoder().decode(CreateCascadeResponsePayload.self, from: data)
        guard let cascadeId = res.cascadeId, !cascadeId.isEmpty else {
            throw APIError.serverError(statusCode: httpResp.statusCode, message: res.error ?? "未能成功生成会话 ID")
        }
        
        return cascadeId
    }
    
}
