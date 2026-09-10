import Foundation

public enum APIError: LocalizedError, Sendable {
    case invalidURL
    case serverError(statusCode: Int, message: String)
    case networkError(String)
    case decodingError(String)
    
    public var errorDescription: String? {
        switch self {
        case .invalidURL:
            return "无效的服务器地址，请在设置中检查"
        case .serverError(let code, let msg):
            return "服务器错误 (\(code)): \(msg)"
        case .networkError(let msg):
            return "网络连接失败: \(msg)"
        case .decodingError(let msg):
            return "数据解析失败: \(msg)"
        }
    }
}

public struct EmptyResponse: Codable, Sendable {
    public init() {}
}

public struct GatewayStatusResponse: Codable, Sendable {
    public let status: String
    public let upstream: UpstreamInfo?
    public var usedInterface: String?
    
    public struct UpstreamInfo: Codable, Sendable {
        public let pid: Int?
        public let port: Int?
        public let is_healthy: Bool?
    }
}

public struct PaginatedMessagesResponse: Codable, Sendable {
    public let cascadeId: String
    public let title: String?
    public let status: String
    public let duration: String
    public let totalSteps: Int
    public let totalTools: Int
    public let totalMessages: Int
    public let hasMore: Bool
    public let nextOffset: Int
    public let messages: [GatewayMessageItem]
    public let cascadeConfigRaw: String?
    public let canProceed: Bool?
    public let proceedArtifactUri: String?
    public let pendingInteraction: PendingInteraction?
    public let queuedMessages: [QueuedMessageItem]?
    
    public struct GatewayMessageItem: Codable, Sendable {
        public let id: String
        public let type: String
        public let text: String
        public let toolCount: Int?
        public let toolNames: [String]?
        public let media: [String]?
        public let imageUrls: [String]?
    }
}

public struct FetchMessagesResult: Sendable {
    public let status: String
    public let messages: [ChatMessage]
    public let totalSteps: Int
    public let totalTools: Int
    public let duration: String
    public let hasMore: Bool
    public let nextOffset: Int
    public let cascadeConfigRaw: String?
    public let title: String?
    public let canProceed: Bool
    public let proceedArtifactUri: String?
    public let pendingInteraction: PendingInteraction?
    public let queuedMessages: [QueuedMessageItem]
}

public final class APIClient: Sendable {
    public static let shared = APIClient()
    
    private let transport: NetworkTransport
    
    public init(transport: NetworkTransport = .shared) {
        self.transport = transport
    }
    
    // Core ConnectRPC POST request
    public func rpc<Req: Encodable, Resp: Decodable>(
        method: String,
        body: Req,
        baseURL: URL
    ) async throws -> Resp {
        let endpoint = baseURL.appendingPathComponent("api/exa.language_server_pb.LanguageServerService/\(method)")
        var request = URLRequest(url: endpoint)
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.setValue("1", forHTTPHeaderField: "Connect-Protocol-Version")
        request.httpBody = try JSONEncoder().encode(body)
        request.timeoutInterval = 15.0
        
        let data: Data
        let response: URLResponse
        do {
            (data, response) = try await transport.send(
                request: request,
                preferCellular: AppSettings.shared.preferCellularNetwork
            )
        } catch {
            throw APIError.networkError(error.localizedDescription)
        }
        
        guard let httpResp = response as? HTTPURLResponse else {
            throw APIError.networkError("Invalid response type")
        }
        
        guard (200...299).contains(httpResp.statusCode) else {
            if httpResp.statusCode == 401 {
                NotificationCenter.default.post(name: .deviceTokenRevoked, object: nil)
            }
            let errorMsg = String(data: data, encoding: .utf8) ?? httpResp.description
            throw APIError.serverError(statusCode: httpResp.statusCode, message: errorMsg)
        }
        
        // Handle void/empty responses safely (ConnectRPC void methods like SendUserCascadeMessage)
        if Resp.self == EmptyResponse.self {
            return EmptyResponse() as! Resp
        }
        if data.isEmpty {
            if let empty = EmptyResponse() as? Resp {
                return empty
            }
        }
        
        do {
            return try JSONDecoder().decode(Resp.self, from: data)
        } catch {
            throw APIError.decodingError(error.localizedDescription)
        }
    }
    
    // Fetch all conversations (excluding internal subagents)
    public func fetchConversations(baseURL: URL) async throws -> [ConversationItem] {
        struct EmptyBody: Encodable {}
        let resp: GetAllCascadeTrajectoriesResponse = try await rpc(
            method: "GetAllCascadeTrajectories",
            body: EmptyBody(),
            baseURL: baseURL
        )
        
        guard let summaries = resp.trajectorySummaries else { return [] }
        
        return summaries.compactMap { id, summary in
            if summary.isSubagent {
                return nil
            }
            let item = ConversationItem(id: id, summary: summary)
            if item.isSubagent {
                return nil
            }
            return item
        }.sorted { a, b in
            (a.lastModified ?? .distantPast) > (b.lastModified ?? .distantPast)
        }
    }
    
    // Direct title lookup from GetAllCascadeTrajectories
    public func fetchConversationTitle(cascadeId: String, baseURL: URL) async throws -> String? {
        struct EmptyBody: Encodable {}
        let resp: GetAllCascadeTrajectoriesResponse = try await rpc(
            method: "GetAllCascadeTrajectories",
            body: EmptyBody(),
            baseURL: baseURL
        )
        guard let summaries = resp.trajectorySummaries, let summary = summaries[cascadeId] else {
            return nil
        }
        if let t = summary.annotations?.title?.trimmingCharacters(in: .whitespacesAndNewlines), !t.isEmpty {
            return t
        }
        if let s = summary.summary?.trimmingCharacters(in: .whitespacesAndNewlines), !s.isEmpty {
            return s
        }
        return nil
    }
    
    // Mark conversation as read both locally and report to upstream language_server
    public func markConversationAsRead(cascadeId: String, baseURL: URL) async {
        CacheManager.shared.markConversationAsRead(cascadeId: cascadeId)
        
        struct AnnotationsPayload: Encodable {
            let markedAsUnread: Bool
            let lastUserViewTime: String
        }
        struct UpdateReq: Encodable {
            let cascadeIds: [String]
            let annotations: AnnotationsPayload
            let mergeAnnotations: Bool
        }
        
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        let nowStr = formatter.string(from: Date())
        
        let body = UpdateReq(
            cascadeIds: [cascadeId],
            annotations: AnnotationsPayload(markedAsUnread: false, lastUserViewTime: nowStr),
            mergeAnnotations: true
        )
        
        struct EmptyResp: Decodable {}
        _ = try? await rpc(method: "UpdateConversationAnnotations", body: body, baseURL: baseURL) as EmptyResp
    }
    
    // Fast lightweight paginated messages endpoint served by Go gateway
    public func fetchMessages(
        cascadeId: String,
        limit: Int = 10,
        offset: Int? = nil,
        baseURL: URL
    ) async throws -> FetchMessagesResult {
        var comps = URLComponents(url: baseURL.appendingPathComponent("gateway/cascade/messages"), resolvingAgainstBaseURL: true)
        var queryItems = [
            URLQueryItem(name: "cascadeId", value: cascadeId),
            URLQueryItem(name: "limit", value: "\(limit)")
        ]
        if let o = offset {
            queryItems.append(URLQueryItem(name: "offset", value: "\(o)"))
        }
        comps?.queryItems = queryItems
        
        guard let url = comps?.url else { throw APIError.invalidURL }
        
        var request = URLRequest(url: url)
        request.timeoutInterval = 10
        
        do {
            let (data, response) = try await transport.send(
                request: request,
                preferCellular: AppSettings.shared.preferCellularNetwork
            )
            if let httpResp = response as? HTTPURLResponse, httpResp.statusCode == 200 {
                let decoded = try JSONDecoder().decode(PaginatedMessagesResponse.self, from: data)
                let chatMessages = decoded.messages.map { item -> ChatMessage in
                    let sender: ChatMessage.MessageSender = {
                        switch item.type {
                        case "user": return .user
                        case "agent": return .agent
                        default: return .toolBatch(count: item.toolCount ?? 1, tools: item.toolNames ?? [])
                        }
                    }()
                    
                    let imgDataList = (item.media ?? []).compactMap { Data(base64Encoded: $0) }
                    
                    return ChatMessage(
                        id: item.id,
                        sender: sender,
                        content: item.text,
                        toolCount: item.toolCount ?? 0,
                        toolNames: item.toolNames ?? [],
                        imageDataList: imgDataList,
                        imageUrls: item.imageUrls ?? []
                    )
                }
                
                return FetchMessagesResult(
                    status: decoded.status,
                    messages: chatMessages,
                    totalSteps: decoded.totalSteps,
                    totalTools: decoded.totalTools,
                    duration: decoded.duration,
                    hasMore: decoded.hasMore,
                    nextOffset: decoded.nextOffset,
                    cascadeConfigRaw: decoded.cascadeConfigRaw,
                    title: decoded.title,
                    canProceed: decoded.canProceed ?? false,
                    proceedArtifactUri: decoded.proceedArtifactUri,
                    pendingInteraction: decoded.pendingInteraction,
                    queuedMessages: decoded.queuedMessages ?? []
                )
            }
        } catch {
            // Fallback to full trajectory fetch if gateway custom endpoint fails
        }
        
        let (status, msgs, steps, tools, dur, title) = try await fetchTrajectory(cascadeId: cascadeId, baseURL: baseURL)
        return FetchMessagesResult(
            status: status,
            messages: msgs,
            totalSteps: steps,
            totalTools: tools,
            duration: dur,
            hasMore: false,
            nextOffset: 0,
            cascadeConfigRaw: nil,
            title: title,
            canProceed: false,
            proceedArtifactUri: nil,
            pendingInteraction: nil,
            queuedMessages: []
        )
    }
    
    // Fetch trajectory steps and parse into streamlined ChatMessage array
    public func fetchTrajectory(cascadeId: String, baseURL: URL) async throws -> (status: String, messages: [ChatMessage], totalSteps: Int, totalTools: Int, duration: String, title: String?) {
        let req = GetCascadeTrajectoryRequest(cascadeId: cascadeId)
        let resp: GetCascadeTrajectoryResponse = try await rpc(
            method: "GetCascadeTrajectory",
            body: req,
            baseURL: baseURL
        )
        
        guard let traj = resp.trajectory, let steps = traj.steps else {
            return ("UNKNOWN", [], 0, 0, "0秒", nil)
        }
        
        var messages: [ChatMessage] = []
        var pendingTools: [String] = []
        var totalToolsCount = 0
        
        func flushTools() {
            guard !pendingTools.isEmpty else { return }
            let count = pendingTools.count
            totalToolsCount += count
            let uniqueNames = Array(NSOrderedSet(array: pendingTools)).compactMap { $0 as? String }
            messages.append(ChatMessage(
                sender: .toolBatch(count: count, tools: uniqueNames),
                content: "已执行 \(count) 次工具调用",
                toolCount: count,
                toolNames: uniqueNames
            ))
            pendingTools.removeAll()
        }
        
        for step in steps {
            let type = step.type ?? ""
            
            if type == "CORTEX_STEP_TYPE_USER_INPUT" {
                flushTools()
                let text = step.userInput?.userResponse ?? step.userInput?.items?.first?.text ?? ""
                
                var images: [Data] = []
                if let mediaList = step.userInput?.media {
                    for media in mediaList {
                        if let thumb = media.thumbnail, !thumb.isEmpty, let data = Data(base64Encoded: thumb) {
                            images.append(data)
                        } else if let inline = media.inlineData, !inline.isEmpty, let data = Data(base64Encoded: inline) {
                            images.append(data)
                        }
                    }
                }
                
                if !text.isEmpty || !images.isEmpty {
                    messages.append(ChatMessage(
                        sender: .user,
                        content: text,
                        imageDataList: images
                    ))
                }
            } else if type == "CORTEX_STEP_TYPE_PLANNER_RESPONSE" {
                let response = step.plannerResponse?.response?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
                let thinking = step.plannerResponse?.thinking?.trimmingCharacters(in: .whitespacesAndNewlines)
                
                if !response.isEmpty || (thinking != nil && !thinking!.isEmpty) {
                    flushTools()
                    
                    // Extract any image URLs in response markdown
                    var imageUrls: [String] = []
                    if let regex = try? NSRegularExpression(pattern: #"!\[.*?\]\((https?://[^\s\)]+|/static/[^\s\)]+)\)"#) {
                        let nsRange = NSRange(response.startIndex..<response.endIndex, in: response)
                        let matches = regex.matches(in: response, range: nsRange)
                        for match in matches {
                            if match.numberOfRanges > 1, let range = Range(match.range(at: 1), in: response) {
                                var imgUrl = String(response[range])
                                if imgUrl.hasPrefix("/static/") {
                                    imgUrl = baseURL.absoluteString.trimmingCharacters(in: CharacterSet(charactersIn: "/")) + imgUrl
                                }
                                imageUrls.append(imgUrl)
                            }
                        }
                    }
                    
                    messages.append(ChatMessage(
                        sender: .agent,
                        content: response.isEmpty ? "（已完成思考，准备下发指令）" : response,
                        thinking: thinking?.isEmpty == false ? thinking : nil,
                        imageUrls: imageUrls
                    ))
                }
            } else if type.hasPrefix("CORTEX_STEP_TYPE_") && type != "CORTEX_STEP_TYPE_SYSTEM_MESSAGE" {
                let toolName = type
                    .replacingOccurrences(of: "CORTEX_STEP_TYPE_", with: "")
                    .lowercased()
                pendingTools.append(toolName)
            }
        }
        
        flushTools()
        
        // Calculate total duration
        var durationString = "0秒"
        let firstCreated = steps.first(where: { $0.metadata?.createdAt != nil })?.metadata?.createdAt
        let lastCreated = steps.last(where: { $0.metadata?.createdAt != nil })?.metadata?.createdAt
        
        if let firstStr = firstCreated, let lastStr = lastCreated {
            let fmtWithFraction = ISO8601DateFormatter()
            fmtWithFraction.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
            let fmtStandard = ISO8601DateFormatter()
            
            let startDate = fmtWithFraction.date(from: firstStr) ?? fmtStandard.date(from: firstStr)
            let endDate = fmtWithFraction.date(from: lastStr) ?? fmtStandard.date(from: lastStr)
            
            if let start = startDate, let end = endDate {
                let diff = max(0, Int(end.timeIntervalSince(start)))
                let hours = diff / 3600
                let minutes = (diff % 3600) / 60
                let secs = diff % 60
                if hours > 0 {
                    durationString = "\(hours)小时\(minutes)分"
                } else if minutes > 0 {
                    durationString = "\(minutes)分\(secs)秒"
                } else {
                    durationString = "\(secs)秒"
                }
            }
        }
        
        let runStatus = resp.status ?? "DONE"
        let title = traj.annotations?.title ?? traj.summary
        return (runStatus, messages, steps.count, totalToolsCount, durationString, title)
    }
    
    // Send a message to cascade
    public func sendMessage(
        cascadeId: String,
        text: String,
        deliveryStrategy: Int? = nil,
        cascadeConfigRaw: String? = nil,
        baseURL: URL
    ) async throws {
        let req = SendUserCascadeMessageRequest(
            cascadeId: cascadeId,
            text: text,
            deliveryStrategy: deliveryStrategy,
            cascadeConfigRaw: cascadeConfigRaw
        )
        let _: EmptyResponse = try await rpc(
            method: "SendUserCascadeMessage",
            body: req,
            baseURL: baseURL
        )
    }
    
    // Proceed with an artifact review/plan
    public func proceedArtifact(cascadeId: String, artifactUri: String, cascadeConfigRaw: String? = nil, baseURL: URL) async throws {
        let comment = ArtifactCommentPayload(artifactUri: artifactUri, approvalStatus: 1, comment: "")
        let req = SendUserCascadeMessageRequest(
            cascadeId: cascadeId,
            items: [],
            cascadeConfigRaw: cascadeConfigRaw,
            artifactComments: [comment]
        )
        let _: EmptyResponse = try await rpc(
            method: "SendUserCascadeMessage",
            body: req,
            baseURL: baseURL
        )
    }
    
    // Submit user decision on a pending interaction
    public func submitInteraction(
        cascadeId: String,
        trajectoryId: String,
        stepIndex: Int,
        type: String,
        optionId: String,
        scope: Int = 1,
        allow: Bool = true,
        writeInResponse: String = "",
        skipped: Bool = false,
        target: String? = nil,
        baseURL: URL
    ) async throws {
        let endpoint = baseURL.appendingPathComponent("gateway/cascade/interaction")
        var request = URLRequest(url: endpoint)
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.timeoutInterval = 10
        
        let payload = InteractionSubmitRequest(
            cascadeId: cascadeId,
            trajectoryId: trajectoryId,
            stepIndex: stepIndex,
            type: type,
            optionId: optionId,
            scope: scope,
            allow: allow,
            writeInResponse: writeInResponse,
            skipped: skipped,
            target: target
        )
        
        request.httpBody = try JSONEncoder().encode(payload)
        
        let (data, response) = try await transport.send(
            request: request,
            preferCellular: AppSettings.shared.preferCellularNetwork
        )
        guard let httpResp = response as? HTTPURLResponse else {
            throw APIError.networkError("Invalid HTTP response")
        }
        guard httpResp.statusCode == 200 else {
            let msg = String(data: data, encoding: .utf8) ?? "HTTP \(httpResp.statusCode)"
            throw APIError.networkError("提交选项失败: \(msg)")
        }
    }
    
    // Cancel task execution
    public func cancelTask(cascadeId: String, baseURL: URL) async throws {
        let req = CancelCascadeInvocationRequest(cascadeId: cascadeId)
        let _: EmptyResponse = try await rpc(
            method: "CancelCascadeInvocation",
            body: req,
            baseURL: baseURL
        )
    }
    
    // Delete a queued agent message
    public func deleteAgentMessage(messageId: String, cascadeId: String, baseURL: URL) async throws {
        let req = DeleteAgentMessageRequest(messageId: messageId, recipient: cascadeId)
        let _: EmptyResponse = try await rpc(
            method: "DeleteAgentMessage",
            body: req,
            baseURL: baseURL
        )
    }
    
    // Test gateway connection
    public func testConnection(baseURL: URL) async throws -> GatewayStatusResponse {
        let endpoint = baseURL.appendingPathComponent("gateway/status")
        var request = URLRequest(url: endpoint)
        request.httpMethod = "GET"
        request.timeoutInterval = 5.0
        
        do {
            let (data, response) = try await transport.send(
                request: request,
                preferCellular: AppSettings.shared.preferCellularNetwork
            )
            guard let httpResp = response as? HTTPURLResponse else {
                throw APIError.networkError("Invalid response type")
            }
            
            guard httpResp.statusCode == 200 else {
                throw APIError.networkError("网关未返回 200 (HTTP \(httpResp.statusCode))")
            }
            
            var decoded = try JSONDecoder().decode(GatewayStatusResponse.self, from: data)
            let ifaceHeader = (httpResp.allHeaderFields["X-Antigravity-Interface"] as? String) ??
                              (httpResp.allHeaderFields["x-antigravity-interface"] as? String)
            decoded.usedInterface = (ifaceHeader == "cellular") ? "cellular" : "wifi"
            return decoded
        } catch {
            let desc = error.localizedDescription
            if desc.contains("SSL") || desc.contains("certificate") || desc.contains("TLS") || desc.contains("secure connection") {
                throw APIError.networkError("SSL握手失败。网关默认运行在 HTTP 协议，请检查地址是否误填了 https://")
            }
            throw error
        }
    }
    
    // Fetch discovered upstream projects
    public func fetchProjects(baseURL: URL) async throws -> [ProjectItem] {
        let endpoint = baseURL.appendingPathComponent("gateway/projects")
        var request = URLRequest(url: endpoint)
        request.httpMethod = "GET"
        request.timeoutInterval = 10
        
        let (data, response) = try await transport.send(
            request: request,
            preferCellular: AppSettings.shared.preferCellularNetwork
        )
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
        
        return try decoder.decode([ProjectItem].self, from: data)
    }
    
    // Create a new cascade and optionally send initial prompt
    public func createCascade(
        workspaceUri: String,
        prompt: String,
        model: String? = nil,
        projectId: String? = nil,
        baseURL: URL
    ) async throws -> String {
        let endpoint = baseURL.appendingPathComponent("gateway/cascade/new")
        var request = URLRequest(url: endpoint)
        request.httpMethod = "POST"
        request.timeoutInterval = 15
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        
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
        
        let (data, response) = try await transport.send(
            request: request,
            preferCellular: AppSettings.shared.preferCellularNetwork
        )
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
    
    // Fetch Cockpit Quotas
    public func fetchCockpitQuotas(baseURL: URL) async throws -> CockpitQuotaResponse {
        let endpoint = baseURL.appendingPathComponent("api/v1/cockpit/quotas")
        var request = URLRequest(url: endpoint)
        request.httpMethod = "GET"
        request.timeoutInterval = 10
        
        let (data, response) = try await transport.send(
            request: request,
            preferCellular: AppSettings.shared.preferCellularNetwork
        )
        guard let httpResp = response as? HTTPURLResponse else {
            throw APIError.networkError("Invalid response type")
        }
        guard (200...299).contains(httpResp.statusCode) else {
            let msg = String(data: data, encoding: .utf8) ?? "HTTP \(httpResp.statusCode)"
            throw APIError.serverError(statusCode: httpResp.statusCode, message: msg)
        }
        return try JSONDecoder().decode(CockpitQuotaResponse.self, from: data)
    }
    
    // Trigger Cockpit Quota Refresh
    public func refreshCockpitQuotas(baseURL: URL) async throws {
        let endpoint = baseURL.appendingPathComponent("api/v1/cockpit/refresh")
        var request = URLRequest(url: endpoint)
        request.httpMethod = "POST"
        request.timeoutInterval = 10
        
        let (data, response) = try await transport.send(
            request: request,
            preferCellular: AppSettings.shared.preferCellularNetwork
        )
        guard let httpResp = response as? HTTPURLResponse else {
            throw APIError.networkError("Invalid response type")
        }
        guard (200...299).contains(httpResp.statusCode) else {
            let msg = String(data: data, encoding: .utf8) ?? "HTTP \(httpResp.statusCode)"
            throw APIError.serverError(statusCode: httpResp.statusCode, message: msg)
        }
    }
}
