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
        
        let decoder = Self.makeProjectsDecoder()
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

    private static func makeProjectsDecoder() -> JSONDecoder {
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
        return decoder
    }

    /// Register a local folder path on the gateway host as a new Antigravity workspace project.
    public func createProjectWorkspace(
        path: String,
        name: String? = nil,
        startSession: Bool = false,
        baseURL: URL
    ) async throws -> CreateProjectResponse {
        let endpoint = baseURL.appendingPathComponent("gateway/projects/create")
        var request = URLRequest(url: endpoint)
        request.httpMethod = "POST"
        request.timeoutInterval = 25
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")

        struct Payload: Encodable {
            let path: String
            let name: String?
            let startSession: Bool
        }
        request.httpBody = try Self.sharedEncoder.encode(Payload(
            path: path.trimmingCharacters(in: .whitespacesAndNewlines),
            name: name?.trimmingCharacters(in: .whitespacesAndNewlines),
            startSession: startSession
        ))

        let reqEpoch = GatewayRuntimeState.shared.epoch
        let (data, response) = try await transport.send(request: request)
        guard let httpResp = response as? HTTPURLResponse else {
            throw APIError.networkError("Invalid response type")
        }
        guard (200...299).contains(httpResp.statusCode) else {
            if httpResp.statusCode == 401 {
                postTokenRevokedIfCurrentEpoch(reqEpoch, url: request.url, data: data)
            }
            let msg = (try? Self.makeProjectsDecoder().decode(CreateProjectResponse.self, from: data))?.error
                ?? String(data: data, encoding: .utf8)
                ?? "HTTP \(httpResp.statusCode)"
            throw APIError.serverError(statusCode: httpResp.statusCode, message: msg)
        }
        return try Self.makeProjectsDecoder().decode(CreateProjectResponse.self, from: data)
    }

    // MARK: - Local Filesystem Directory Picker & Async GitHub Clone

    public func fetchLocalDirs(
        path: String? = nil,
        showHidden: Bool = false,
        baseURL: URL
    ) async throws -> LocalDirsResponse {
        var components = URLComponents(url: baseURL.appendingPathComponent("gateway/fs/dirs"), resolvingAgainstBaseURL: false)
        var queryItems: [URLQueryItem] = []
        if let path, !path.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
            queryItems.append(URLQueryItem(name: "path", value: path.trimmingCharacters(in: .whitespacesAndNewlines)))
        }
        if showHidden {
            queryItems.append(URLQueryItem(name: "showHidden", value: "true"))
        }
        if !queryItems.isEmpty {
            components?.queryItems = queryItems
        }
        guard let url = components?.url else {
            throw APIError.invalidURL
        }
        var request = URLRequest(url: url)
        request.httpMethod = "GET"
        request.timeoutInterval = 15

        let reqEpoch = GatewayRuntimeState.shared.epoch
        let (data, response) = try await transport.send(request: request)
        guard let httpResp = response as? HTTPURLResponse else {
            throw APIError.networkError("Invalid response type")
        }
        guard (200...299).contains(httpResp.statusCode) else {
            if httpResp.statusCode == 401 {
                postTokenRevokedIfCurrentEpoch(reqEpoch, url: request.url, data: data)
            }
            let msg = (try? JSONDecoder().decode(CreateLocalDirResponse.self, from: data))?.error
                ?? String(data: data, encoding: .utf8)
                ?? "HTTP \(httpResp.statusCode)"
            throw APIError.serverError(statusCode: httpResp.statusCode, message: msg)
        }
        let decoded = try Self.makeProjectsDecoder().decode(LocalDirsResponse.self, from: data)
        if let platform = decoded.platform, !platform.isEmpty {
            AppSettings.shared.gatewayPlatform = platform
        }
        return decoded
    }

    public func createLocalDir(
        parentPath: String? = nil,
        name: String? = nil,
        path: String? = nil,
        baseURL: URL
    ) async throws -> CreateLocalDirResponse {
        let endpoint = baseURL.appendingPathComponent("gateway/fs/mkdir")
        var request = URLRequest(url: endpoint)
        request.httpMethod = "POST"
        request.timeoutInterval = 15
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")

        struct Payload: Encodable {
            let parentPath: String?
            let name: String?
            let path: String?
        }
        request.httpBody = try Self.sharedEncoder.encode(Payload(
            parentPath: parentPath?.trimmingCharacters(in: .whitespacesAndNewlines),
            name: name?.trimmingCharacters(in: .whitespacesAndNewlines),
            path: path?.trimmingCharacters(in: .whitespacesAndNewlines)
        ))

        let reqEpoch = GatewayRuntimeState.shared.epoch
        let (data, response) = try await transport.send(request: request)
        guard let httpResp = response as? HTTPURLResponse else {
            throw APIError.networkError("Invalid response type")
        }
        guard (200...299).contains(httpResp.statusCode) else {
            if httpResp.statusCode == 401 {
                postTokenRevokedIfCurrentEpoch(reqEpoch, url: request.url, data: data)
            }
            let msg = (try? JSONDecoder().decode(CreateLocalDirResponse.self, from: data))?.error
                ?? String(data: data, encoding: .utf8)
                ?? "HTTP \(httpResp.statusCode)"
            throw APIError.serverError(statusCode: httpResp.statusCode, message: msg)
        }
        return try JSONDecoder().decode(CreateLocalDirResponse.self, from: data)
    }

    /// Starts (or imports existing) a GitHub repository clone job and returns structured status including 409 conflicts.
    public func startCloneGitHubRepo(
        repoSlug: String,
        parentDir: String? = nil,
        folderName: String? = nil,
        asyncMode: Bool = true,
        importExisting: Bool = false,
        startSession: Bool = false,
        baseURL: URL
    ) async throws -> CloneRepoResponse {
        let endpoint = baseURL.appendingPathComponent("gateway/projects/clone")
        var request = URLRequest(url: endpoint)
        request.httpMethod = "POST"
        request.timeoutInterval = asyncMode ? 25 : 180
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")

        struct Payload: Encodable {
            let repoSlug: String
            let parentDir: String?
            let folderName: String?
            let async: Bool
            let importExisting: Bool
            let startSession: Bool
        }
        request.httpBody = try Self.sharedEncoder.encode(Payload(
            repoSlug: repoSlug.trimmingCharacters(in: .whitespacesAndNewlines),
            parentDir: parentDir?.trimmingCharacters(in: .whitespacesAndNewlines),
            folderName: folderName?.trimmingCharacters(in: .whitespacesAndNewlines),
            async: asyncMode,
            importExisting: importExisting,
            startSession: startSession
        ))

        let reqEpoch = GatewayRuntimeState.shared.epoch
        let (data, response) = try await transport.send(request: request)
        guard let httpResp = response as? HTTPURLResponse else {
            throw APIError.networkError("Invalid response type")
        }
        let decoder = Self.makeProjectsDecoder()
        if httpResp.statusCode == 409, let conflict = try? decoder.decode(CloneRepoResponse.self, from: data) {
            return conflict
        }
        guard (200...299).contains(httpResp.statusCode) else {
            if httpResp.statusCode == 401 {
                postTokenRevokedIfCurrentEpoch(reqEpoch, url: request.url, data: data)
            }
            let msg = (try? decoder.decode(CloneRepoResponse.self, from: data))?.error
                ?? String(data: data, encoding: .utf8)
                ?? "HTTP \(httpResp.statusCode)"
            throw APIError.serverError(statusCode: httpResp.statusCode, message: msg)
        }
        return try decoder.decode(CloneRepoResponse.self, from: data)
    }

    public func fetchCloneGitHubRepoStatus(
        jobId: String,
        baseURL: URL
    ) async throws -> CloneRepoResponse {
        var components = URLComponents(url: baseURL.appendingPathComponent("gateway/projects/clone/status"), resolvingAgainstBaseURL: false)
        components?.queryItems = [URLQueryItem(name: "jobId", value: jobId)]
        guard let url = components?.url else {
            throw APIError.invalidURL
        }
        var request = URLRequest(url: url)
        request.httpMethod = "GET"
        request.timeoutInterval = 15

        let reqEpoch = GatewayRuntimeState.shared.epoch
        let (data, response) = try await transport.send(request: request)
        guard let httpResp = response as? HTTPURLResponse else {
            throw APIError.networkError("Invalid response type")
        }
        let decoder = Self.makeProjectsDecoder()
        guard (200...299).contains(httpResp.statusCode) else {
            if httpResp.statusCode == 401 {
                postTokenRevokedIfCurrentEpoch(reqEpoch, url: request.url, data: data)
            }
            let msg = (try? decoder.decode(CloneRepoResponse.self, from: data))?.error
                ?? String(data: data, encoding: .utf8)
                ?? "HTTP \(httpResp.statusCode)"
            throw APIError.serverError(statusCode: httpResp.statusCode, message: msg)
        }
        return try decoder.decode(CloneRepoResponse.self, from: data)
    }

    public func cancelCloneGitHubRepo(
        jobId: String,
        baseURL: URL
    ) async throws -> CloneRepoResponse {
        let endpoint = baseURL.appendingPathComponent("gateway/projects/clone/cancel")
        var request = URLRequest(url: endpoint)
        request.httpMethod = "POST"
        request.timeoutInterval = 15
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.httpBody = try Self.sharedEncoder.encode(["jobId": jobId])

        let (data, response) = try await transport.send(request: request)
        guard let httpResp = response as? HTTPURLResponse else {
            throw APIError.networkError("Invalid response type")
        }
        let decoder = Self.makeProjectsDecoder()
        guard (200...299).contains(httpResp.statusCode) else {
            let msg = (try? decoder.decode(CloneRepoResponse.self, from: data))?.error
                ?? String(data: data, encoding: .utf8)
                ?? "HTTP \(httpResp.statusCode)"
            throw APIError.serverError(statusCode: httpResp.statusCode, message: msg)
        }
        return try decoder.decode(CloneRepoResponse.self, from: data)
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
            throw APIError.serverError(statusCode: httpResp.statusCode, message: res.error ?? String(localized: "未能成功生成会话 ID"))
        }
        
        return cascadeId
    }
    
}
