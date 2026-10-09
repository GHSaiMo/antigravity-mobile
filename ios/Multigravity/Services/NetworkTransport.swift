import Foundation
import Network
import os
import Security

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
    
    public var isCellular: Bool {
        NetworkStatus.shared.isCellular
    }
    
    public var isWifi: Bool {
        NetworkStatus.shared.isWifi
    }
    
    public init() {
        let config = URLSessionConfiguration.default
        config.timeoutIntervalForRequest = 15
        config.timeoutIntervalForResource = 30
        config.httpShouldSetCookies = false
        config.httpCookieAcceptPolicy = .never
        config.httpCookieStorage = nil
        config.protocolClasses = [DemoURLProtocol.self] + (config.protocolClasses ?? [])
        self.fallbackSession = URLSession(configuration: config, delegate: nil, delegateQueue: nil)
    }
    
    /// Decorates HTTPURLResponse with X-Antigravity-Interface header to indicate actual interface used
    private func decorateResponse(_ response: URLResponse, url: URL?, forcedCellular: Bool = false) -> URLResponse {
        guard let http = response as? HTTPURLResponse, let targetURL = url ?? http.url else {
            return response
        }
        // Self-heal primary Cloud URL if advertised by gateway
        if let cloudHeader = http.value(forHTTPHeaderField: "X-Antigravity-Cloud-URL")?.trimmingCharacters(in: .whitespacesAndNewlines),
           !cloudHeader.isEmpty {
            Task { @MainActor in
                if AppSettings.shared.primaryCloudURL != cloudHeader {
                    AppSettings.shared.primaryCloudURL = cloudHeader
                }
            }
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
    
    nonisolated public static func extractHost(from raw: String) -> String {
        let trimmed = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        if trimmed.isEmpty { return "" }
        if let url = URL(string: trimmed.contains("://") ? trimmed : "http://\(trimmed)"),
           let host = url.host {
            return host.trimmingCharacters(in: CharacterSet(charactersIn: "[]")).lowercased()
        }
        var clean = trimmed
        if let schemeRange = clean.range(of: "://") {
            clean = String(clean[schemeRange.upperBound...])
        }
        if let slashIdx = clean.firstIndex(of: "/") {
            clean = String(clean[..<slashIdx])
        }
        if clean.hasPrefix("["), let end = clean.firstIndex(of: "]") {
            return String(clean[clean.index(after: clean.startIndex)..<end]).lowercased()
        }
        if let colonIdx = clean.lastIndex(of: ":") {
            let port = clean[clean.index(after: colonIdx)...]
            if !port.isEmpty && port.allSatisfy({ $0.isNumber }) {
                clean = String(clean[..<colonIdx])
            }
        }
        return clean.trimmingCharacters(in: CharacterSet(charactersIn: "[]")).lowercased()
    }
    
    nonisolated public static func isLocalOrPrivateHost(_ host: String) -> Bool {
        let clean = extractHost(from: host)
        if clean.isEmpty { return false }
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
        if clean.hasPrefix("100.") || clean.hasSuffix(".ts.net") {
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

    /// Sends a request using URLSession, except cleartext HTTP to non-local endpoints
    /// which App Transport Security blocks. Those go through Network.framework.
    public func send(request: URLRequest, preferCellular: Bool = false) async throws -> (Data, URLResponse) {
        var req = request
        if let token = KeychainHelper.shared.read(key: .deviceToken), !token.isEmpty {
            if req.value(forHTTPHeaderField: "Authorization") == nil {
                req.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
            }
        }
        
        if Self.requiresCleartextATSBypass(req.url) {
            let (data, response) = try await sendCleartextHTTP(req)
            return (data, decorateResponse(response, url: req.url, forcedCellular: preferCellular || isCellular))
        }
        
        if let url = req.url, url.scheme?.lowercased() == "https", CloudflareAnycastAccelerator.isCloudflareTunnelHost(url.host ?? "") {
            do {
                let (data, response) = try await sendCloudflareAcceleratedHTTPS(req)
                return (data, decorateResponse(response, url: req.url))
            } catch {
                // If Anycast acceleration fails, smoothly fall back to system URLSession
            }
        }
        
        let (data, response) = try await fallbackSession.data(for: req)
        return (data, decorateResponse(response, url: req.url))
    }
    
    /// ATS blocks cleartext HTTP to public hosts (NSAllowsArbitraryLoads is false).
    /// Cloud HTTP relays must still work, so those go through Network.framework.
    nonisolated public static func requiresCleartextATSBypass(_ url: URL?) -> Bool {
        guard let url, url.scheme?.lowercased() == "http" else { return false }
        let host = (url.host ?? "").trimmingCharacters(in: CharacterSet(charactersIn: "[]"))
        return !isLocalOrPrivateHost(host)
    }
    
    // MARK: - Accelerated Connection Pool
    
    private actor AcceleratedConnectionPool {
        static let shared = AcceleratedConnectionPool()
        
        struct Entry {
            let connection: NWConnection
            let host: String
            let lastUsed: Date
        }
        
        /// Idle keep-alive connections per "\(ip):\(port)", most recently used last. HTTP/1.1 carries one
        /// request at a time per connection, so concurrent requests (list, quotas, status and messages on
        /// launch) each need their own; keeping several avoids a fresh TCP+TLS handshake for each.
        private var pool: [String: [Entry]] = [:]
        private static let maxIdlePerKey = 4
        private static let idleTimeout: TimeInterval = 15.0

        func acquire(ip: String, host: String, port: NWEndpoint.Port, parameters: NWParameters) -> (NWConnection, Bool) {
            let key = "\(ip):\(port.rawValue)"
            var idle = pool[key] ?? []
            var reusable: NWConnection?
            while reusable == nil, let entry = idle.popLast() {
                if entry.connection.state == .ready &&
                   entry.host == host &&
                   Date().timeIntervalSince(entry.lastUsed) < Self.idleTimeout {
                    reusable = entry.connection
                } else {
                    entry.connection.cancel()
                }
            }
            pool[key] = idle.isEmpty ? nil : idle
            if let reusable {
                return (reusable, true)
            }
            let conn = NWConnection(host: NWEndpoint.Host(ip), port: port, using: parameters)
            return (conn, false)
        }

        func release(connection: NWConnection, ip: String, host: String, port: NWEndpoint.Port, canReuse: Bool) {
            let key = "\(ip):\(port.rawValue)"
            guard canReuse && connection.state == .ready else {
                connection.cancel()
                return
            }
            var idle = pool[key] ?? []
            idle.append(Entry(connection: connection, host: host, lastUsed: Date()))
            while idle.count > Self.maxIdlePerKey {
                idle.removeFirst().connection.cancel()
            }
            pool[key] = idle
        }
    }
    
    private func sendCloudflareAcceleratedHTTPS(_ request: URLRequest) async throws -> (Data, URLResponse) {
        guard let url = request.url else {
            throw URLError(.badURL)
        }
        let bareHost = (url.host ?? "").trimmingCharacters(in: CharacterSet(charactersIn: "[]"))
        guard !bareHost.isEmpty else {
            throw URLError(.badURL)
        }
        
        let tlsOptions = NWProtocolTLS.Options()
        sec_protocol_options_set_tls_server_name(tlsOptions.securityProtocolOptions, bareHost)
        let parameters = NWParameters(tls: tlsOptions)
        
        let anycastIP = CloudflareAnycastAccelerator.primaryIP
        let port = NWEndpoint.Port(rawValue: UInt16(url.port ?? 443)) ?? .https
        
        for attempt in 0..<2 {
            let (connection, isReused) = await AcceleratedConnectionPool.shared.acquire(
                ip: anycastIP, host: bareHost, port: port, parameters: parameters
            )
            
            if !isReused {
                do {
                    try await Self.waitUntilReady(connection, timeout: request.timeoutInterval > 0 ? min(request.timeoutInterval, 5) : 5)
                } catch {
                    connection.cancel()
                    throw error
                }
            }
            
            let payload = HTTP11Codec.buildRequest(request, url: url, bareHost: bareHost, keepAlive: true)
            var canReuse = false
            do {
                try await Self.sendAll(connection, payload)
                let raw = try await Self.receiveHTTPMessage(connection)
                let (data, response) = try HTTP11Codec.parseResponse(raw, url: url)
                
                if let http = response as? HTTPURLResponse,
                   let connHeader = http.value(forHTTPHeaderField: "Connection"),
                   connHeader.lowercased().contains("close") {
                    canReuse = false
                } else {
                    canReuse = true
                }
                
                await AcceleratedConnectionPool.shared.release(
                    connection: connection, ip: anycastIP, host: bareHost, port: port, canReuse: canReuse
                )
                return (data, response)
            } catch {
                connection.cancel()
                if isReused && attempt == 0 {
                    continue
                }
                throw error
            }
        }
        
        throw URLError(.cannotConnectToHost)
    }

    private func sendCleartextHTTP(_ request: URLRequest) async throws -> (Data, URLResponse) {
        guard let url = request.url else {
            throw URLError(.badURL)
        }
        let bareHost = (url.host ?? "").trimmingCharacters(in: CharacterSet(charactersIn: "[]"))
        guard !bareHost.isEmpty, let port = NWEndpoint.Port(rawValue: UInt16(url.port ?? 80)) else {
            throw URLError(.badURL)
        }
        
        let connection = NWConnection(host: NWEndpoint.Host(bareHost), port: port, using: .tcp)
        try await Self.waitUntilReady(connection, timeout: request.timeoutInterval > 0 ? request.timeoutInterval : 8)
        defer { connection.cancel() }
        
        let payload = HTTP11Codec.buildRequest(request, url: url, bareHost: bareHost, keepAlive: false)
        try await Self.sendAll(connection, payload)
        let raw = try await Self.receiveHTTPMessage(connection)
        return try HTTP11Codec.parseResponse(raw, url: url)
    }
    
    private static func waitUntilReady(_ connection: NWConnection, timeout: TimeInterval) async throws {
        try await withCheckedThrowingContinuation { (cont: CheckedContinuation<Void, Error>) in
            let finished = OSAllocatedUnfairLock(initialState: false)
            @Sendable func complete(_ result: Result<Void, Error>) {
                let go = finished.withLock { flag -> Bool in
                    if flag { return false }
                    flag = true
                    return true
                }
                guard go else { return }
                cont.resume(with: result)
            }
            
            connection.stateUpdateHandler = { state in
                switch state {
                case .ready:
                    complete(.success(()))
                case .failed(let err):
                    complete(.failure(err))
                case .cancelled:
                    complete(.failure(URLError(.cancelled)))
                default:
                    break
                }
            }
            connection.start(queue: DispatchQueue.global(qos: .userInitiated))
            
            DispatchQueue.global(qos: .userInitiated).asyncAfter(deadline: .now() + timeout) {
                let timedOut = finished.withLock { flag -> Bool in
                    if flag { return false }
                    flag = true
                    return true
                }
                if timedOut {
                    connection.cancel()
                    cont.resume(throwing: URLError(.timedOut))
                }
            }
        }
    }
    
    private static func sendAll(_ connection: NWConnection, _ data: Data) async throws {
        try await withCheckedThrowingContinuation { (cont: CheckedContinuation<Void, Error>) in
            connection.send(content: data, completion: .contentProcessed { error in
                if let error {
                    cont.resume(throwing: error)
                } else {
                    cont.resume()
                }
            })
        }
    }
    
    private static func receiveHTTPMessage(_ connection: NWConnection) async throws -> Data {
        var accumulator = HTTP11Codec.ResponseAccumulator()
        while true {
            let chunk: Data = try await withCheckedThrowingContinuation { cont in
                connection.receive(minimumIncompleteLength: 1, maximumLength: 64 * 1024) { content, _, _, error in
                    if let error {
                        cont.resume(throwing: error)
                        return
                    }
                    cont.resume(returning: content ?? Data())
                }
            }
            if accumulator.append(chunk) {
                return accumulator.buffer
            }
            if chunk.isEmpty {
                if accumulator.buffer.isEmpty {
                    throw URLError(.networkConnectionLost)
                }
                return accumulator.buffer
            }
        }
    }
}
