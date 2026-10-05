import Foundation

extension APIClient {
    // Send a message to cascade
    public func sendMessage(
        cascadeId: String,
        text: String,
        model: String? = nil,
        images: [Data]? = nil,
        deliveryStrategy: Int? = nil,
        cascadeConfigRaw: String? = nil,
        clientMessageId: String? = nil,
        attachmentIds: [String]? = nil,
        baseURL: URL
    ) async throws {
        let imagePayloads = images?.map { ImageDataPayload(base64Data: $0.base64EncodedString(), mimeType: "image/jpeg") }
        let mediaPayloads = images?.map { MediaDataPayload(inlineData: $0.base64EncodedString(), mimeType: "image/jpeg") }
        let req = SendUserCascadeMessageRequest(
            cascadeId: cascadeId,
            text: text,
            model: model,
            images: imagePayloads,
            media: mediaPayloads,
            deliveryStrategy: deliveryStrategy,
            cascadeConfigRaw: cascadeConfigRaw,
            attachments: attachmentIds?.isEmpty == false ? attachmentIds?.map { AttachmentRef(id: $0) } : nil
        )
        var headers: [String: String] = [:]
        if let cmid = clientMessageId, !cmid.isEmpty {
            headers["X-Client-Message-Id"] = cmid
        }
        if let m = model, !m.isEmpty {
            headers["X-Antigravity-Model"] = m
        }
        let _: EmptyResponse = try await rpc(
            method: "SendUserCascadeMessage",
            body: req,
            baseURL: baseURL,
            additionalHeaders: headers.isEmpty ? nil : headers
        )
    }
    
    // Switch active model in upstream Language Server via JetboxWriteState
    public func switchModel(to modelEnum: String, cascadeId: String? = nil, baseURL: URL) async throws {
        struct JetboxWriteStateRequest: Encodable {
            struct AppState: Encodable {
                let lastSelectedAgentModel: String
            }
            let appState: AppState
        }
        var headers: [String: String] = [:]
        if let cid = cascadeId, !cid.isEmpty {
            headers["X-Cascade-Id"] = cid
        }
        let req = JetboxWriteStateRequest(appState: .init(lastSelectedAgentModel: modelEnum))
        let _: EmptyResponse = try await rpc(
            method: "JetboxWriteState",
            body: req,
            baseURL: baseURL,
            additionalHeaders: headers.isEmpty ? nil : headers
        )
    }
    
    // Proceed with an artifact review/plan
    public func proceedArtifact(cascadeId: String, artifactUri: String, model: String? = nil, cascadeConfigRaw: String? = nil, baseURL: URL) async throws {
        let comment = ArtifactCommentPayload(artifactUri: artifactUri, approvalStatus: 1, comment: "")
        let req = SendUserCascadeMessageRequest(
            cascadeId: cascadeId,
            items: [],
            model: model,
            cascadeConfigRaw: cascadeConfigRaw,
            artifactComments: [comment]
        )
        var headers: [String: String]? = nil
        if let m = model, !m.isEmpty {
            headers = ["X-Antigravity-Model": m]
        }
        let _: EmptyResponse = try await rpc(
            method: "SendUserCascadeMessage",
            body: req,
            baseURL: baseURL,
            additionalHeaders: headers
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
        questionResponses: [QuestionResponse]? = nil,
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
            target: target,
            questionResponses: questionResponses
        )
        
        request.httpBody = try JSONEncoder().encode(payload)
        
        let (data, response) = try await transport.send(request: request)
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
    

    // Stop / cancel a running background task step
    public func stopTask(cascadeId: String, stepIndex: Int, taskId: String, baseURL: URL) async throws {
        let endpoint = baseURL.appendingPathComponent("gateway/cascade/task/stop")
        var request = URLRequest(url: endpoint)
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        
        let body: [String: Any] = [
            "cascadeId": cascadeId,
            "stepIndex": stepIndex,
            "taskId": taskId
        ]
        request.httpBody = try JSONSerialization.data(withJSONObject: body)
        request.timeoutInterval = 10
        
        let (data, response) = try await transport.send(request: request)
        guard let httpResp = response as? HTTPURLResponse else {
            throw APIError.networkError("Invalid response type")
        }
        guard (200...299).contains(httpResp.statusCode) else {
            let msg = String(data: data, encoding: .utf8) ?? "HTTP \(httpResp.statusCode)"
            throw APIError.serverError(statusCode: httpResp.statusCode, message: msg)
        }
    }
    
}
