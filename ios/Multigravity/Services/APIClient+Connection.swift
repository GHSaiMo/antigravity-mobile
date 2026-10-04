import Foundation

extension APIClient {
    public struct WSTicketResponse: Codable, Sendable {
        public let ticket: String
        public let expiresIn: Int?
        
        enum CodingKeys: String, CodingKey {
            case ticket
            case expiresIn = "expires_in"
        }
    }
    
    /// Requests a short-lived (30s) one-time WebSocket ticket via POST /api/v1/auth/ws-ticket.
    public func fetchWSTicket(baseURL: URL) async throws -> String {
        let endpoint = baseURL.appendingPathComponent("api/v1/auth/ws-ticket")
        var request = URLRequest(url: endpoint)
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        if let token = KeychainHelper.shared.read(key: .deviceToken) ?? AppSettings.shared.deviceToken, !token.isEmpty {
            request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
            request.setValue(token, forHTTPHeaderField: "x-device-token")
        }
        request.httpBody = "{}".data(using: .utf8)
        let (data, response) = try await transport.send(request: request)
        guard let httpResp = response as? HTTPURLResponse, (200...299).contains(httpResp.statusCode) else {
            let code = (response as? HTTPURLResponse)?.statusCode ?? 500
            throw APIError.serverError(statusCode: code, message: "Failed to obtain ws-ticket")
        }
        let ticketResp = try JSONDecoder().decode(WSTicketResponse.self, from: data)
        return ticketResp.ticket
    }
    

    // Test gateway connection
    public func testConnection(baseURL: URL) async throws -> GatewayStatusResponse {
        let endpoint = baseURL.appendingPathComponent("gateway/status")
        var request = URLRequest(url: endpoint)
        request.httpMethod = "GET"
        request.timeoutInterval = 5.0
        
        do {
            let (data, response) = try await transport.send(request: request)
            guard let httpResp = response as? HTTPURLResponse else {
                throw APIError.networkError("Invalid response type")
            }
            
            guard httpResp.statusCode == 200 else {
                throw APIError.networkError("网关未返回 200 (HTTP \(httpResp.statusCode))")
            }
            
            var decoded = try JSONDecoder().decode(GatewayStatusResponse.self, from: data)
            if let plat = decoded.platform ?? decoded.os {
                AppSettings.shared.gatewayPlatform = plat
            }
            let ifaceHeader = (httpResp.allHeaderFields["X-Antigravity-Interface"] as? String) ??
                              (httpResp.allHeaderFields["x-antigravity-interface"] as? String)
            let isCellular: Bool
            if let iface = ifaceHeader {
                isCellular = (iface == "cellular")
            } else {
                isCellular = NetworkTransport.shared.isCellular && !NetworkTransport.shared.isWifi
            }
            decoded.usedInterface = isCellular ? "cellular" : "wifi"
            decoded.connectionDescription = AppSettings.describeEndpoint(url: baseURL, isCellular: isCellular)
            return decoded
        } catch {
            let desc = error.localizedDescription
            if desc.contains("SSL") || desc.contains("certificate") || desc.contains("TLS") || desc.contains("secure connection") {
                throw APIError.networkError("SSL握手失败。网关默认运行在 HTTP 协议，请检查地址是否误填了 https://")
            }
            throw error
        }
    }
    
}
