import Foundation

public final class APIClient: Sendable {
    public static let shared = APIClient()
    
    let transport: NetworkTransport
    
    public init(transport: NetworkTransport = .shared) {
        self.transport = transport
    }
    
    /// True when `candidate` is the same host as the paired gateway (ignore port).
    public static func isSameGatewayHost(_ candidate: URL, gateway: URL) -> Bool {
        func normalizedHost(_ url: URL) -> String {
            (url.host ?? "")
                .trimmingCharacters(in: CharacterSet(charactersIn: "[]"))
                .lowercased()
        }
        let remote = normalizedHost(candidate)
        let local = normalizedHost(gateway)
        return !remote.isEmpty && remote == local
    }
    
    public static let sharedDecoder = JSONDecoder()
    public static let sharedEncoder = JSONEncoder()

    // Core ConnectRPC POST request
    public func rpc<Req: Encodable, Resp: Decodable>(
        method: String,
        body: Req,
        baseURL: URL,
        additionalHeaders: [String: String]? = nil
    ) async throws -> Resp {
        let endpoint = baseURL.appendingPathComponent("api/exa.language_server_pb.LanguageServerService/\(method)")
        var request = URLRequest(url: endpoint)
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.setValue("1", forHTTPHeaderField: "Connect-Protocol-Version")
        if let headers = additionalHeaders {
            for (k, v) in headers {
                request.setValue(v, forHTTPHeaderField: k)
            }
        }
        request.httpBody = try Self.sharedEncoder.encode(body)
        request.timeoutInterval = 15.0
        
        let data: Data
        let response: URLResponse
        do {
            (data, response) = try await transport.send(request: request)
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
            return try Self.sharedDecoder.decode(Resp.self, from: data)
        } catch {
            throw APIError.decodingError(error.localizedDescription)
        }
    }
}
