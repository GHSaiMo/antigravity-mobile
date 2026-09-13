import Foundation
import Network
import os

extension Notification.Name {
    public static let deviceTokenRevoked = Notification.Name("antigravity.device_token_revoked")
}

public enum NetworkTransportError: Error, LocalizedError, Sendable {
    case requestAlreadyDispatched(any Error)
    
    public var errorDescription: String? {
        switch self {
        case .requestAlreadyDispatched(let err):
            return "指令已成功送达服务器，但等待响应超时 (\(err.localizedDescription))"
        }
    }
}

public final class NetworkTransport: Sendable {
    public static let shared = NetworkTransport()
    
    private let fallbackSession: URLSession
    private let pathMonitor = NWPathMonitor()
    private let monitorQueue = DispatchQueue(label: "antigravity.network_transport_monitor", qos: .utility)
    private let _isCellular = OSAllocatedUnfairLock(initialState: false)
    private let _isWifi = OSAllocatedUnfairLock(initialState: false)
    
    public var isCellular: Bool {
        _isCellular.withLock { $0 }
    }
    
    public var isWifi: Bool {
        _isWifi.withLock { $0 }
    }
    
    public init() {
        let config = URLSessionConfiguration.default
        config.timeoutIntervalForRequest = 15
        config.timeoutIntervalForResource = 30
        config.httpShouldSetCookies = true
        config.httpCookieAcceptPolicy = .always
        config.httpCookieStorage = HTTPCookieStorage.shared
        self.fallbackSession = URLSession(configuration: config)
        
        pathMonitor.pathUpdateHandler = { [weak self] path in
            guard let self = self else { return }
            let cellular = path.usesInterfaceType(.cellular) || path.isExpensive
            let wifi = path.usesInterfaceType(.wifi)
            self._isCellular.withLock { $0 = cellular }
            self._isWifi.withLock { $0 = wifi }
        }
        pathMonitor.start(queue: monitorQueue)
        let initialPath = pathMonitor.currentPath
        let cellular = initialPath.usesInterfaceType(.cellular) || initialPath.isExpensive
        let wifi = initialPath.usesInterfaceType(.wifi)
        _isCellular.withLock { $0 = cellular }
        _isWifi.withLock { $0 = wifi }
    }
    
    /// Decorates HTTPURLResponse with X-Antigravity-Interface header to indicate actual interface used
    private func decorateResponse(_ response: URLResponse, url: URL?, forcedCellular: Bool = false) -> URLResponse {
        guard let http = response as? HTTPURLResponse, let targetURL = url ?? http.url else {
            return response
        }
        var fields: [String: String] = [:]
        for (k, v) in http.allHeaderFields {
            fields["\(k)"] = "\(v)"
        }
        // If low-level NWConnection pinned cellular was used, or phone is on cellular network without Wi-Fi
        let usedCellular = forcedCellular || (isCellular && !isWifi)
        fields["X-Antigravity-Interface"] = usedCellular ? "cellular" : "wifi"
        return HTTPURLResponse(
            url: targetURL,
            statusCode: http.statusCode,
            httpVersion: "HTTP/1.1",
            headerFields: fields
        ) ?? response
    }
    
    /// Sends a request prioritizing the cellular interface (IPv6 direct) if requested and available,
    nonisolated public static func isLocalOrPrivateHost(_ host: String) -> Bool {
        let clean = host.trimmingCharacters(in: CharacterSet(charactersIn: "[]")).lowercased()
        if clean == "127.0.0.1" || clean == "localhost" || clean == "::1" || clean.hasSuffix(".local") {
            return true
        }
        // IPv4 private ranges (10.0.0.0/8, 127.0.0.0/8, 192.168.0.0/16)
        if clean.hasPrefix("192.168.") || clean.hasPrefix("10.") || clean.hasPrefix("127.") {
            return true
        }
        // Class B private range (172.16.0.0/12)
        if clean.hasPrefix("172.") {
            let parts = clean.split(separator: ".")
            if parts.count >= 2, let second = Int(parts[1]), second >= 16 && second <= 31 {
                return true
            }
        }
        // Tailscale CGNAT range (100.64.0.0/10) or any Tailscale virtual node
        if clean.hasPrefix("100.") {
            return true
        }
        // IPv6 Link-Local (fe80::/10) and Unique Local Address ULA (fc00::/7, fd00::/8)
        if clean.hasPrefix("fe80:") || clean.hasPrefix("fc") || clean.hasPrefix("fd") {
            return true
        }
        return false
     }

    /// Determines whether a request mutates server state and must never be silently retried on timeout/failure
    nonisolated public static func isNonIdempotentRequest(method: String, path: String) -> Bool {
        let m = method.uppercased()
        guard m == "POST" || m == "PUT" || m == "DELETE" || m == "PATCH" else {
            return false
        }
        if path.contains("SendUserCascadeMessage") ||
           path.contains("gateway/cascade/new") ||
           path.contains("gateway/interaction/submit") {
            return true
        }
        return false
    }

    /// Sends a request using high-performance URLSession.
    public func send(request: URLRequest, preferCellular: Bool = false) async throws -> (Data, URLResponse) {
        var req = request
        if let token = KeychainHelper.shared.read(key: .deviceToken), !token.isEmpty {
            if req.value(forHTTPHeaderField: "Authorization") == nil {
                req.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
            }
        }
        
        let (data, response) = try await fallbackSession.data(for: req)
        return (data, decorateResponse(response, url: req.url))
    }
}
