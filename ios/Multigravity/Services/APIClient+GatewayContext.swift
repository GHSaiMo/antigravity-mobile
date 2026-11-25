import Foundation

/// Side-effect-free gateway API helpers using an explicit `GatewayRequestContext`,
/// enabling lock-screen notification actions and Live Activity intents to act on ANY paired gateway
/// (even an inactive one) without disturbing the foreground active gateway.
extension APIClient {
    private func resolveReachableBaseURL(for context: GatewayRequestContext) -> URL? {
        let isCell = NetworkStatus.shared.isCellular || !NetworkStatus.shared.isWifi
        return context.candidateURLs(isCellular: isCell).first
    }

    public func fetchMessages(
        cascadeId: String,
        limit: Int = 5,
        context: GatewayRequestContext
    ) async throws -> FetchMessagesResult {
        let isCell = NetworkStatus.shared.isCellular || !NetworkStatus.shared.isWifi
        let candidates = context.candidateURLs(isCellular: isCell)
        guard !candidates.isEmpty else {
            throw APIError.networkError("No gateway endpoint available")
        }

        var lastError: Error = APIError.networkError("No gateway endpoint available")
        for baseURL in candidates {
            var comps = URLComponents(url: baseURL.appendingPathComponent("gateway/cascade/messages"), resolvingAgainstBaseURL: false)
            comps?.queryItems = [
                URLQueryItem(name: "cascadeId", value: cascadeId),
                URLQueryItem(name: "limit", value: "\(limit)")
            ]
            guard let url = comps?.url else { continue }
            var req = URLRequest(url: url)
            req.httpMethod = "GET"
            req.timeoutInterval = NetworkTransport.isLanOnlyHost(baseURL.host ?? "") ? 2.0 : 8.0

            do {
                let (data, resp) = try await transport.send(request: req, context: context)
                guard let http = resp as? HTTPURLResponse, (200...299).contains(http.statusCode) else {
                    if let http = resp as? HTTPURLResponse {
                        lastError = APIError.serverError(statusCode: http.statusCode, message: "HTTP \(http.statusCode)")
                    }
                    continue
                }
                return try self.decodeFetchMessagesResult(from: data, baseURL: baseURL)
            } catch {
                lastError = error
            }
        }
        throw lastError
    }

    public func submitInteraction(
        cascadeId: String,
        trajectoryId: String,
        stepIndex: Int,
        type: String,
        optionId: String,
        scope: Int,
        allow: Bool,
        writeInResponse: String,
        skipped: Bool,
        target: String?,
        questionResponses: [QuestionResponse]?,
        context: GatewayRequestContext
    ) async throws {
        let isCell = NetworkStatus.shared.isCellular || !NetworkStatus.shared.isWifi
        let candidates = context.candidateURLs(isCellular: isCell)
        guard !candidates.isEmpty else {
            throw APIError.networkError("No gateway endpoint available")
        }

        struct InteractionReq: Encodable {
            let cascadeId: String
            let trajectoryId: String
            let stepIndex: Int
            let type: String
            let optionId: String
            let scope: Int
            let allow: Bool
            let writeInResponse: String
            let skipped: Bool
            let target: String?
            let questionResponses: [QuestionResponse]?
        }
        let body = InteractionReq(
            cascadeId: cascadeId,
            trajectoryId: trajectoryId,
            stepIndex: stepIndex,
            type: type,
            optionId: optionId,
            scope: scope,
            allow: allow,
            writeInResponse: writeInResponse,
            skipped: skipped,
            target: target,
            questionResponses: questionResponses
        )
        let bodyData = try Self.sharedEncoder.encode(body)

        var lastError: Error = APIError.networkError("Submit interaction failed")
        for baseURL in candidates {
            var req = URLRequest(url: baseURL.appendingPathComponent("gateway/cascade/interaction"))
            req.httpMethod = "POST"
            req.setValue("application/json", forHTTPHeaderField: "Content-Type")
            req.httpBody = bodyData
            req.timeoutInterval = NetworkTransport.isLanOnlyHost(baseURL.host ?? "") ? 2.5 : 10.0

            do {
                let (data, resp) = try await transport.send(request: req, context: context)
                if let http = resp as? HTTPURLResponse, (200...299).contains(http.statusCode) {
                    return
                }
                let code = (resp as? HTTPURLResponse)?.statusCode ?? 0
                lastError = APIError.serverError(statusCode: code, message: String(data: data, encoding: .utf8) ?? "HTTP \(code)")
            } catch {
                lastError = error
            }
        }
        throw lastError
    }

    public func proceedArtifact(
        cascadeId: String,
        artifactUri: String,
        model: String?,
        cascadeConfigRaw: String? = nil,
        context: GatewayRequestContext
    ) async throws {
        let isCell = NetworkStatus.shared.isCellular || !NetworkStatus.shared.isWifi
        let candidates = context.candidateURLs(isCellular: isCell)
        guard !candidates.isEmpty else {
            throw APIError.networkError("No gateway endpoint available")
        }

        var payload: [String: Any] = [
            "cascadeId": cascadeId,
            "artifactUri": artifactUri
        ]
        if let m = model, !m.isEmpty { payload["model"] = m }
        if let cfg = cascadeConfigRaw, !cfg.isEmpty { payload["cascadeConfigRaw"] = cfg }
        let bodyData = try JSONSerialization.data(withJSONObject: payload)

        var lastError: Error = APIError.networkError("Proceed artifact failed")
        for baseURL in candidates {
            var req = URLRequest(url: baseURL.appendingPathComponent("gateway/cascade/proceed"))
            req.httpMethod = "POST"
            req.setValue("application/json", forHTTPHeaderField: "Content-Type")
            req.httpBody = bodyData
            req.timeoutInterval = NetworkTransport.isLanOnlyHost(baseURL.host ?? "") ? 2.5 : 10.0

            do {
                let (data, resp) = try await transport.send(request: req, context: context)
                if let http = resp as? HTTPURLResponse, (200...299).contains(http.statusCode) {
                    return
                }
                let code = (resp as? HTTPURLResponse)?.statusCode ?? 0
                lastError = APIError.serverError(statusCode: code, message: String(data: data, encoding: .utf8) ?? "HTTP \(code)")
            } catch {
                lastError = error
            }
        }
        throw lastError
    }

    public func cancelTask(
        cascadeId: String,
        killBackgroundTasks: Bool = true,
        context: GatewayRequestContext
    ) async throws {
        let isCell = NetworkStatus.shared.isCellular || !NetworkStatus.shared.isWifi
        let candidates = context.candidateURLs(isCellular: isCell)
        guard !candidates.isEmpty else { return }

        let bodyData = try JSONSerialization.data(withJSONObject: [
            "cascadeId": cascadeId,
            "killBackgroundTasks": killBackgroundTasks
        ])
        for baseURL in candidates {
            var req = URLRequest(url: baseURL.appendingPathComponent("gateway/cascade/task/stop"))
            req.httpMethod = "POST"
            req.setValue("application/json", forHTTPHeaderField: "Content-Type")
            req.httpBody = bodyData
            req.timeoutInterval = NetworkTransport.isLanOnlyHost(baseURL.host ?? "") ? 2.5 : 8.0
            if let (_, resp) = try? await transport.send(request: req, context: context),
               let http = resp as? HTTPURLResponse,
               (200...299).contains(http.statusCode) {
                return
            }
        }
    }
}
