import Foundation

extension APIClient {
    // Fetch Cockpit Quotas
    public func fetchCockpitQuotas(baseURL: URL) async throws -> CockpitQuotaResponse {
        let endpoint = baseURL.appendingPathComponent("api/v1/cockpit/quotas")
        var request = URLRequest(url: endpoint)
        request.httpMethod = "GET"
        request.timeoutInterval = 10
        
        let (data, response) = try await transport.send(request: request)
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
    @discardableResult
    public func refreshCockpitQuotas(baseURL: URL) async throws -> CockpitQuotaResponse? {
        let endpoint = baseURL.appendingPathComponent("api/v1/cockpit/refresh")
        var request = URLRequest(url: endpoint)
        request.httpMethod = "POST"
        request.timeoutInterval = 12
        
        let (data, response) = try await transport.send(request: request)
        guard let httpResp = response as? HTTPURLResponse else {
            throw APIError.networkError("Invalid response type")
        }
        guard (200...299).contains(httpResp.statusCode) else {
            let msg = String(data: data, encoding: .utf8) ?? "HTTP \(httpResp.statusCode)"
            throw APIError.serverError(statusCode: httpResp.statusCode, message: msg)
        }
        return try? JSONDecoder().decode(CockpitQuotaResponse.self, from: data)
    }
    
    // Switch Cockpit Active Account
    public func switchCockpitAccount(baseURL: URL, accountId: String) async throws {
        let endpoint = baseURL.appendingPathComponent("api/v1/cockpit/switch")
        var request = URLRequest(url: endpoint)
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.httpBody = try JSONSerialization.data(withJSONObject: ["account_id": accountId])
        request.timeoutInterval = 120
        
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
