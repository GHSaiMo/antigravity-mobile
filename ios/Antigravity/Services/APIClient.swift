import Foundation

public enum APIError: LocalizedError, Sendable {
    case invalidURL
    case serverError(statusCode: Int, message: String)
    case networkError(String)
    case decodingError(String)
    case cloudflareAuthRequired
    
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
        case .cloudflareAuthRequired:
            return "🔒 遇到了 Cloudflare 邮箱验证拦截，请在设置中点击「完成邮箱验证」或配置 Service Token"
        }
    }
}

public struct EmptyResponse: Codable, Sendable {
    public init() {}
}

public struct GatewayStatusResponse: Codable, Sendable {
    public let status: String
    public let upstream: UpstreamInfo?
    
    public struct UpstreamInfo: Codable, Sendable {
        public let pid: Int?
        public let port: Int?
        public let is_healthy: Bool?
    }
}

public struct PaginatedMessagesResponse: Codable, Sendable {
    public let cascadeId: String
    public let status: String
    public let duration: String
    public let totalSteps: Int
    public let totalTools: Int
    public let totalMessages: Int
    public let hasMore: Bool
    public let nextOffset: Int
    public let messages: [GatewayMessageItem]
    public let cascadeConfigRaw: String?
    
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

public final class APIClient: Sendable {
    public static let shared = APIClient()
    
    private let session: URLSession
    
    public init() {
        let config = URLSessionConfiguration.default
        config.timeoutIntervalForRequest = 15
        config.timeoutIntervalForResource = 30
        config.httpShouldSetCookies = true
        config.httpCookieAcceptPolicy = .always
        config.httpCookieStorage = HTTPCookieStorage.shared
        self.session = URLSession(configuration: config)
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
        
        // Inject Cloudflare Access Service Token headers if configured
        let settings = AppSettings.shared
        if !settings.cfAccessClientId.isEmpty {
            request.setValue(settings.cfAccessClientId, forHTTPHeaderField: "CF-Access-Client-Id")
        }
        if !settings.cfAccessClientSecret.isEmpty {
            request.setValue(settings.cfAccessClientSecret, forHTTPHeaderField: "CF-Access-Client-Secret")
        }
        
        request.httpBody = try JSONEncoder().encode(body)
        
        let data: Data
        let response: URLResponse
        do {
            (data, response) = try await session.data(for: request)
        } catch {
            throw APIError.networkError(error.localizedDescription)
        }
        
        guard let httpResp = response as? HTTPURLResponse else {
            throw APIError.networkError("Invalid response type")
        }
        
        // Detect Cloudflare Zero Trust Access challenge
        let contentType = httpResp.value(forHTTPHeaderField: "Content-Type") ?? ""
        if contentType.contains("text/html") || httpResp.statusCode == 403 || httpResp.statusCode == 302 {
            let bodyStr = String(data: data, encoding: .utf8) ?? ""
            if bodyStr.contains("cloudflareaccess.com") || bodyStr.contains("Cloudflare") || bodyStr.contains("<!DOCTYPE html") {
                throw APIError.cloudflareAuthRequired
            }
        }
        
        guard (200...299).contains(httpResp.statusCode) else {
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
    
    // Fetch all conversations
    public func fetchConversations(baseURL: URL) async throws -> [ConversationItem] {
        struct EmptyBody: Encodable {}
        let resp: GetAllCascadeTrajectoriesResponse = try await rpc(
            method: "GetAllCascadeTrajectories",
            body: EmptyBody(),
            baseURL: baseURL
        )
        
        guard let summaries = resp.trajectorySummaries else { return [] }
        
        return summaries.map { id, summary in
            ConversationItem(id: id, summary: summary)
        }.sorted { a, b in
            (a.lastModified ?? .distantPast) > (b.lastModified ?? .distantPast)
        }
    }
    
    // Fast lightweight paginated messages endpoint served by Go gateway
    public func fetchMessages(
        cascadeId: String,
        limit: Int = 10,
        offset: Int? = nil,
        baseURL: URL
    ) async throws -> (status: String, messages: [ChatMessage], totalSteps: Int, totalTools: Int, duration: String, hasMore: Bool, nextOffset: Int, cascadeConfigRaw: String?) {
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
        
        let settings = AppSettings.shared
        if !settings.cfAccessClientId.isEmpty && !settings.cfAccessClientSecret.isEmpty {
            request.setValue(settings.cfAccessClientId, forHTTPHeaderField: "CF-Access-Client-Id")
            request.setValue(settings.cfAccessClientSecret, forHTTPHeaderField: "CF-Access-Client-Secret")
        }
        
        do {
            let (data, response) = try await session.data(for: request)
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
                
                return (
                    decoded.status,
                    chatMessages,
                    decoded.totalSteps,
                    decoded.totalTools,
                    decoded.duration,
                    decoded.hasMore,
                    decoded.nextOffset,
                    decoded.cascadeConfigRaw
                )
            }
        } catch {
            // Fallback to full trajectory fetch if gateway custom endpoint fails
        }
        
        let (status, msgs, steps, tools, dur) = try await fetchTrajectory(cascadeId: cascadeId, baseURL: baseURL)
        return (status, msgs, steps, tools, dur, false, 0, nil)
    }
    
    // Fetch trajectory steps and parse into streamlined ChatMessage array
    public func fetchTrajectory(cascadeId: String, baseURL: URL) async throws -> (status: String, messages: [ChatMessage], totalSteps: Int, totalTools: Int, duration: String) {
        let req = GetCascadeTrajectoryRequest(cascadeId: cascadeId)
        let resp: GetCascadeTrajectoryResponse = try await rpc(
            method: "GetCascadeTrajectory",
            body: req,
            baseURL: baseURL
        )
        
        guard let traj = resp.trajectory, let steps = traj.steps else {
            return ("UNKNOWN", [], 0, 0, "0秒")
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
        return (runStatus, messages, steps.count, totalToolsCount, durationString)
    }
    
    // Send a message to cascade
    public func sendMessage(cascadeId: String, text: String, cascadeConfigRaw: String? = nil, baseURL: URL) async throws {
        let req = SendUserCascadeMessageRequest(cascadeId: cascadeId, text: text, cascadeConfigRaw: cascadeConfigRaw)
        let _: EmptyResponse = try await rpc(
            method: "SendUserCascadeMessage",
            body: req,
            baseURL: baseURL
        )
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
    
    // Test gateway connection
    public func testConnection(baseURL: URL) async throws -> GatewayStatusResponse {
        let endpoint = baseURL.appendingPathComponent("gateway/status")
        var request = URLRequest(url: endpoint)
        request.httpMethod = "GET"
        
        let settings = AppSettings.shared
        if !settings.cfAccessClientId.isEmpty {
            request.setValue(settings.cfAccessClientId, forHTTPHeaderField: "CF-Access-Client-Id")
        }
        if !settings.cfAccessClientSecret.isEmpty {
            request.setValue(settings.cfAccessClientSecret, forHTTPHeaderField: "CF-Access-Client-Secret")
        }
        
        let (data, response) = try await session.data(for: request)
        guard let httpResp = response as? HTTPURLResponse else {
            throw APIError.networkError("Invalid response type")
        }
        
        let contentType = httpResp.value(forHTTPHeaderField: "Content-Type") ?? ""
        if contentType.contains("text/html") || httpResp.statusCode == 403 || httpResp.statusCode == 302 {
            throw APIError.cloudflareAuthRequired
        }
        
        guard httpResp.statusCode == 200 else {
            throw APIError.networkError("网关未返回 200 (HTTP \(httpResp.statusCode))")
        }
        
        return try JSONDecoder().decode(GatewayStatusResponse.self, from: data)
    }
}
