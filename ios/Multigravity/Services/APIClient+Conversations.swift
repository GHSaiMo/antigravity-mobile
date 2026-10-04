import Foundation

extension APIClient {
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
            a.effectiveLastModified > b.effectiveLastModified
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
    
    // Report session focus to gateway immediately on tap (0ms latency, fire-and-forget)
    public func notifySessionFocus(cascadeId: String, baseURL: URL) {
        guard !cascadeId.isEmpty else { return }
        let endpoint = baseURL.appendingPathComponent("gateway/cascade/focus")
        var req = URLRequest(url: endpoint)
        req.httpMethod = "POST"
        req.timeoutInterval = 3.0
        req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        
        let payload: [String: String] = [
            "cascadeId": cascadeId,
            "source": "ios"
        ]
        req.httpBody = try? JSONEncoder().encode(payload)
        
        // Conforms to Swift 6 Sendable concurrency and NetworkTransport
        // Automatically injects Bearer token and handles transport routing
        Task { [transport] in
            _ = try? await transport.send(request: req)
        }
    }
    
    // Mark conversation as read both locally and report to upstream language_server
    public func markConversationAsRead(cascadeId: String, baseURL: URL) async {
        guard !cascadeId.isEmpty else { return }
        if CacheManager.shared.isDeletedConversation(cascadeId: cascadeId) {
            return
        }
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
    
    // Delete conversation from upstream language_server
    public func deleteConversation(cascadeId: String, baseURL: URL) async throws {
        struct DeleteReq: Encodable {
            let cascadeId: String
        }
        struct EmptyResp: Decodable {}
        _ = try await rpc(method: "DeleteCascadeTrajectory", body: DeleteReq(cascadeId: cascadeId), baseURL: baseURL) as EmptyResp
    }
    
    // Rename conversation title in upstream language_server
    public func renameConversation(cascadeId: String, newTitle: String, baseURL: URL) async throws {
        struct AnnotationsPayload: Encodable {
            let title: String
        }
        struct UpdateReq: Encodable {
            let cascadeIds: [String]
            let annotations: AnnotationsPayload
            let mergeAnnotations: Bool
        }
        let body = UpdateReq(
            cascadeIds: [cascadeId],
            annotations: AnnotationsPayload(title: newTitle),
            mergeAnnotations: true
        )
        struct EmptyResp: Decodable {}
        _ = try await rpc(method: "UpdateConversationAnnotations", body: body, baseURL: baseURL) as EmptyResp
    }
    
    /// Unpairs this device from the gateway and cleans up server-side state.
    public func unpair(
        baseURL: URL? = nil,
        candidates: [URL] = [],
        token: String? = nil,
        deviceID: String? = nil
    ) async {
        let primaryURL = baseURL ?? AppSettings.shared.serverURL
        let deviceToken = token ?? KeychainHelper.shared.read(key: .deviceToken) ?? AppSettings.shared.deviceToken
        let targetDeviceID = deviceID ?? KeychainHelper.shared.read(key: .deviceID) ?? AppSettings.shared.deviceID
        
        guard let deviceToken, !deviceToken.isEmpty else { return }
        
        var targetURLs: [URL] = []
        if let primaryURL {
            targetURLs.append(primaryURL)
        }
        for cand in candidates {
            if !targetURLs.contains(cand) {
                targetURLs.append(cand)
            }
        }
        if targetURLs.isEmpty {
            targetURLs = AppSettings.shared.candidateEndpoints.compactMap { URL(string: $0.urlString) }
        }
        guard !targetURLs.isEmpty else { return }
        
        let body: [String: String] = [
            "device_id": targetDeviceID ?? ""
        ]
        let bodyData = (try? JSONEncoder().encode(body)) ?? Data("{}".utf8)
        
        for url in targetURLs {
            let endpoint = url.appendingPathComponent("api/v1/auth/unpair")
            var request = URLRequest(url: endpoint)
            request.httpMethod = "POST"
            request.setValue("application/json", forHTTPHeaderField: "Content-Type")
            request.setValue("Bearer \(deviceToken)", forHTTPHeaderField: "Authorization")
            request.timeoutInterval = 3.5
            request.httpBody = bodyData
            
            do {
                let (_, response) = try await transport.send(request: request)
                if let httpResp = response as? HTTPURLResponse, (200...299).contains(httpResp.statusCode) {
                    return
                }
            } catch {
                // Try next endpoint candidate
            }
        }
    }
    
}
