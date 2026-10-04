import Foundation

extension APIClient {
    // MARK: - Revert / Undo Operations
    
    public func fetchRevertPreview(cascadeId: String, stepIndex: Int, baseURL: URL) async throws -> RevertPreviewResponse {
        let endpoint = baseURL.appendingPathComponent("gateway/cascade/revert/preview")
        var request = URLRequest(url: endpoint)
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        let reqBody = RevertPreviewRequest(cascadeId: cascadeId, stepIndex: stepIndex)
        request.httpBody = try JSONEncoder().encode(reqBody)
        request.timeoutInterval = 15.0
        
        let (data, response) = try await transport.send(request: request)
        guard let httpResp = response as? HTTPURLResponse, (200...299).contains(httpResp.statusCode) else {
            let code = (response as? HTTPURLResponse)?.statusCode ?? -1
            let msg = String(data: data, encoding: .utf8) ?? "Failed to fetch revert preview"
            throw APIError.serverError(statusCode: code, message: msg)
        }
        return try JSONDecoder().decode(RevertPreviewResponse.self, from: data)
    }
    
    public func executeRevert(cascadeId: String, stepIndex: Int, conversationOnly: Bool = false, baseURL: URL) async throws {
        let endpoint = baseURL.appendingPathComponent("gateway/cascade/revert/execute")
        var request = URLRequest(url: endpoint)
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        let reqBody = RevertExecuteRequest(cascadeId: cascadeId, stepIndex: stepIndex, conversationOnly: conversationOnly)
        request.httpBody = try JSONEncoder().encode(reqBody)
        request.timeoutInterval = 20.0
        
        let (data, response) = try await transport.send(request: request)
        guard let httpResp = response as? HTTPURLResponse, (200...299).contains(httpResp.statusCode) else {
            let code = (response as? HTTPURLResponse)?.statusCode ?? -1
            let msg = String(data: data, encoding: .utf8) ?? "Failed to execute revert"
            throw APIError.serverError(statusCode: code, message: msg)
        }
    }
}
